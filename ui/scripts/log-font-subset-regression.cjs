// node --test scripts/log-font-subset-regression.cjs
// 不联网、不安装依赖：以现有 TypeScript 编译器加载真实规划/资源代码，替换 API 与 DOM 边界。
const fs = require('node:fs');
const path = require('node:path');
const Module = require('node:module');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const { test } = require('node:test');
const ts = require('typescript');
const src = path.join(__dirname, '../src');
const apiFile = path.join(src, 'services/font/platformFontApi.ts');
const vueFile = require.resolve('vue');
const resolve = Module._resolveFilename;
Module._resolveFilename = function (request, parent, ...args) {
  if (request.startsWith('@/')) request = path.join(src, request.slice(2));
  return resolve.call(this, request, parent, ...args);
};
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename);

const latin = { name: 'latin.woff2', url: 'https://untrusted.example/latin.woff2', unicodeRange: 'U+0000-00FF', mimeType: 'font/woff2' };
const cjk = { name: 'nested/cjk.woff', url: '/untrusted/cjk.woff', unicodeRange: 'U+4E00-9FFF', mimeType: 'font/woff' };
let config;
let requests;
let apiCalls;
let fetchCredentials;
let cleanup;
const api = {
  buildPlatformFontFileUrl: id => `/api/v1/platform-fonts/${id}/file`,
  buildPlatformFontSubsetUrl: (id, name) => `/api/v1/platform-fonts/${id}/subset/${name.split('/').map(encodeURIComponent).join('/')}`,
  getPlatformFontMeta: async () => { apiCalls.push('meta'); return { id: 'font', family: 'Font', weight: '400', style: 'normal', sourceFileName: 'full.ttf', ...config.meta }; },
  getPlatformFontManifest: async () => { apiCalls.push('manifest'); if (config.manifestError) throw Error('manifest failed'); return { chunks: config.chunks }; },
  getPublicPlatformFontMeta: async () => { apiCalls.push('public-meta'); return { id: 'font', family: 'Font', weight: '400', style: 'normal', sourceFileName: 'full.ttf', ...config.meta }; },
  getPublicPlatformFontManifest: async () => { apiCalls.push('public-manifest'); if (config.manifestError) throw Error('manifest failed'); return { chunks: config.chunks }; },
};
Module._cache[apiFile] = { id: apiFile, filename: apiFile, loaded: true, exports: api };
Module._cache[vueFile] = { id: vueFile, filename: vueFile, loaded: true, exports: { onBeforeUnmount: fn => { cleanup = fn; } } };
// logApi 的业务接口不参与字体请求。
const logApiFile = path.join(src, 'services/log/logApi.ts');
Module._cache[logApiFile] = { id: logApiFile, filename: logApiFile, loaded: true, exports: {} };
global.fetch = async (url, options) => {
  assert.ok(options.signal instanceof AbortSignal);
  requests.push(url);
  fetchCredentials.push(options.credentials);
  if (config.fail?.(url)) throw Error('chunk failed');
  const mime = url.endsWith('.woff2') ? 'font/woff2' : url.endsWith('.woff') ? 'font/woff' : 'font/ttf';
  return { ok: true, headers: { get: () => config.mime || mime }, arrayBuffer: async () => new Uint8Array([1, 2, 3]).buffer };
};
// 仅替换浏览器 HTML 解析边界；font plan 仍读取真实 FrameDocument 全部 HTML，而非 plainText。
global.document = { createElement: () => {
  let html = '';
  return { set innerHTML(value) { html = value; }, content: {
    get textContent() { return html.replace(/<[^>]+>/g, '').replace(/&amp;/g, '&'); },
    querySelectorAll: selector => selector.startsWith('[data-platform-font-id]')
      ? Array.from(html.matchAll(/<[^>]+\bdata-(?:platform-font-id|ruby-base-font-asset-id|ruby-rt-font-asset-id)="[^"]+"[^>]*>/g), match => ({
        getAttribute: name => match[0].match(new RegExp(`${name}="([^"]+)"`))?.[1] || null,
      })) : [],
    querySelector: selector => selector.includes('pageNumber') && html.includes('data-log-fill') ? {} : null,
  } };
} };
const subset = require(path.join(src, 'services/font/platformFontSubsetRuntime.ts'));
const sheet = require(path.join(src, 'services/font/characterSheetTemplateFontRuntime.ts'));
const plan = require(path.join(src, 'services/log/logFontPlan.ts'));
const { useLogAssets } = require(path.join(src, 'views/log/composables/useLogAssets.ts'));
const doc = (text = 'A', extra = {}) => ({
  page: { fontFamily: 'platform:font' }, css: '', rootVars: {}, rootAttrs: {},
  blocks: [{ html: `<p>${text}</p>` }], headerHtml: null, footerHtml: null, ...extra,
});
const setup = (value = {}, scope = {}) => {
  config = { meta: { deliveryMode: 'subset' }, chunks: [latin, cjk], ...value };
  requests = []; apiCalls = []; fetchCredentials = [];
  return useLogAssets(scope);
};

test('single 继续获取 full；非平台字体不请求字体 API', async () => {
  const assets = setup({ meta: { deliveryMode: 'single' } });
  const result = await assets.prepare(doc(), [], () => false);
  assert.deepEqual(requests, ['/api/v1/platform-fonts/font/file']);
  assert.deepEqual(apiCalls, ['meta']);
  assert.match(result.doc.css, /__LOGASSET_platform-font:font:full__/);
  assert.match(result.doc.css, /format\("truetype"\)/);
  assert.deepEqual(fetchCredentials, ['include']);
  await assets.prepare(doc('A', { page: { fontFamily: 'serif' } }), [], () => false);
  assert.equal(requests.length, 1);
});

test('subset 只请求命中 chunk；缓存及已投递资源支持增量，CSS 无真实 URL', async () => {
  const assets = setup();
  const first = await assets.prepare(doc(), [], () => false);
  assert.deepEqual(requests, ['/api/v1/platform-fonts/font/subset/latin.woff2']);
  assert.match(first.doc.css, /unicode-range:U\+0000-00FF/);
  assert.doesNotMatch(first.doc.css, /https?:|\/api\/|:full/);
  const delivered = new Set(first.assets.map(asset => asset.token));
  const second = await assets.prepare(doc('A中'), [], token => delivered.has(token));
  assert.deepEqual(requests, ['/api/v1/platform-fonts/font/subset/latin.woff2', '/api/v1/platform-fonts/font/subset/nested/cjk.woff']);
  assert.equal(second.assets.length, 1);
  assert.equal(second.assets[0].mime, 'font/woff');
  assert.match(second.doc.css, /format\("woff"\)/);
  assert.equal((second.doc.css.match(/sc-log-platform-font/g) || []).length, 2);
  await assets.prepare(doc('A中'), [], () => false);
  assert.equal(requests.length, 2, 'iframe 重建仍复用字节缓存');
  assert.deepEqual(apiCalls, ['meta', 'manifest']);
});

test('公开 LOG 平台字体 meta、manifest 与字体字节均不携带登录凭据', async () => {
  const assets = setup({}, { publicSlug: () => 'publicslug' });
  await assets.prepare(doc('A中'), [], () => false);
  assert.deepEqual(apiCalls, ['public-meta', 'public-manifest']);
  assert.deepEqual(fetchCredentials, ['omit', 'omit']);
});

test('正文和 Ruby 平台字体与页面字体合并规划、去重缓存；单字体失败独立回退', async () => {
  const assets = setup({ fail: url => url.includes('/rt/subset/') });
  const input = doc('<span data-platform-font-id="body">A</span><ruby data-ruby-base-font-asset-id="body" data-ruby-rt-font-asset-id="rt">中<rt>A</rt></ruby>');
  const result = await assets.prepare(input, [], () => false);
  for (const id of ['font', 'body', 'rt']) assert.ok(result.doc.css.includes(`sc-log-platform-${id}`));
  assert.match(result.doc.css, /__LOGASSET_platform-font:rt:full__/);
  assert.match(result.doc.css, /__LOGASSET_platform-font:body:chunk:/);
  assert.equal(requests.filter(url => url.includes('/body/')).length, 2);
  const count = requests.length;
  await assets.prepare(input, [], () => false);
  assert.equal(requests.length, count);
  const bodyOnly = setup({ fail: () => true });
  const missing = await bodyOnly.prepare(doc('<span data-platform-font-id="missing">A</span>', { page: { fontFamily: 'serif' } }), [], () => false);
  assert.equal(missing.assets.length, 1);
  assert.equal(missing.assets[0].data, null);
  assert.match(missing.doc.css, /sc-log-platform-missing/);
});

test('最终正文/角色名/引用/线索/自定义元素/标题/页眉页脚均计入需求，字符去重', async () => {
  const assets = setup();
  const input = doc('AA', {
    blocks: [
      { html: '<article><header>角</header><q>引</q><span>线</span></article>' },
      { html: '<h1>标</h1><aside>自</aside>' },
    ], headerHtml: '<header>页眉A</header>', footerHtml: '<footer>页脚</footer>',
  });
  const text = plan.collectLogFontText(input);
  for (const char of '角引线标自页眉脚A') assert.ok(text.text.includes(char));
  assert.equal(text.text.length, new Set(text.text).size);
  await assets.prepare(input, [], () => false);
  assert.equal(requests.length, 2);
  assert.equal(plan.collectLogFontText(doc('', { footerHtml: '<span data-log-fill="pageNumber"></span>' })).text, '0123456789');
});

test('空文档不获取清单或下载字体', async () => {
  const assets = setup();
  const result = await assets.prepare(doc(''), [], () => false);
  assert.deepEqual(requests, []);
  assert.deepEqual(apiCalls, []);
  assert.equal(result.assets.length, 0);
});

test('CSS 静态生成文本计入分片；动态或无法解析的生成文本安全降级 full', async () => {
  global.CSSStyleSheet = class {
    replaceSync(css) {
      this.cssRules = [{ style: { getPropertyValue: () => css.includes('attr(') ? 'attr(data-label)' : '"\\4E2D"' } }];
    }
  };
  try {
    let assets = setup();
    await assets.prepare(doc('A', { css: '.label::before{content:"中"}' }), [], () => false);
    assert.equal(requests.length, 2);
    assets = setup();
    const dynamic = await assets.prepare(doc('A', { css: '.label::before{content:attr(data-label)}' }), [], () => false);
    assert.match(dynamic.doc.css, /:full__/);
    assert.deepEqual(requests, ['/api/v1/platform-fonts/font/file']);
  } finally { delete global.CSSStyleSheet; }
  const assets = setup();
  const result = await assets.prepare(doc('A', { css: '.label::before{content:"中"}' }), [], () => false);
  assert.match(result.doc.css, /:full__/);
});

test('清单获取失败、空/非法 chunks、区间无效、未命中时均降级 full', async () => {
  for (const value of [
    { manifestError: true }, { chunks: [] }, { chunks: [null] },
    { chunks: [{ ...latin, unicodeRange: 'bad;src:url(https://bad)' }] },
    { chunks: [{ ...latin, unicodeRange: '' }] },
    { chunks: [{ ...latin, name: '../evil.woff2' }] },
    { chunks: [{ ...latin, name: '/evil.woff2' }] },
    { chunks: [{ ...latin, name: '%2e%2e/evil.woff2' }] },
    { chunks: [cjk] },
  ]) {
    const assets = setup(value);
    const result = await assets.prepare(doc(), [], () => false);
    assert.deepEqual(requests, ['/api/v1/platform-fonts/font/file']);
    assert.match(result.doc.css, /:full__/);
    assert.equal(result.assets[0].data.byteLength, 3);
  }
});

test('必要 chunk 失败或 MIME 无效时降级 full；不影响排版，full 同样缓存', async () => {
  for (const fail of [url => url.endsWith('.woff'), null]) {
    const assets = setup({ fail, ...(fail ? {} : { mime: 'application/octet-stream' }) });
    const result = await assets.prepare(doc('A中'), [], () => false);
    if (fail) {
      assert.match(result.doc.css, /:full__/);
      assert.equal(result.assets.length, 1);
      assert.equal(requests.length, 3);
      await assets.prepare(doc('A中'), [], () => false);
      assert.equal(requests.length, 3);
    } else {
      assert.deepEqual(result.assets.map(asset => asset.mime), ['font/woff2', 'font/woff']);
    }
  }
  const assets = setup({ mime: 'text/html' });
  const result = await assets.prepare(doc(), [], () => false);
  assert.match(result.doc.css, /:full__/);
  assert.ok(requests.includes('/api/v1/platform-fonts/font/file'));
  cleanup();
});

test('非法 token 不发起请求，不允许 token 注入任意 URL 或路径', async () => {
  const assets = setup();
  const result = await assets.ensure(['platform-font:../bad:full', 'platform-font:font:chunk:https://bad', 'platform-font:font:chunk:deadbeef']);
  assert.ok(result.every(asset => asset.data === null));
  assert.deepEqual(requests, []);
  for (const id of ['platform:../x', 'platform:x/y', 'platform:x:full', `platform:${'a'.repeat(101)}`]) assert.equal(plan.logPlatformFontId(id), '');
});

test('Character Sheet 复用同一选择器，保留 URL 和格式行为，支持通配/非 BMP/空文本', () => {
  assert.equal(sheet.selectPlatformFontChunksForText, subset.selectPlatformFontChunksForText);
  assert.deepEqual(subset.selectPlatformFontChunksForText([latin, cjk], '调查员A'), [latin, cjk]);
  assert.deepEqual(subset.selectPlatformFontChunksForText([latin, cjk], ''), []);
  const emoji = { name: 'emoji.woff2', url: '', unicodeRange: 'U+1F6??, U+20000-2FFFF' };
  assert.deepEqual(subset.selectPlatformFontChunksForText([emoji], '😀𠀀'), [emoji]);
  assert.equal(subset.resolvePlatformFontChunkUrl(latin, name => `local/${name}`), latin.url);
  assert.equal(subset.resolvePlatformFontChunkUrl({ ...latin, url: './latin.woff2' }, name => `local/${name}`), 'local/latin.woff2');
  require(path.join(src, 'services/font/characterSheetTemplateFontRuntime.spec.ts'));
});

test('Character Sheet 实际生成 subset/full CSS 与分片字节缓存保持正常', async () => {
  setup({ chunks: [{ ...latin, url: '' }, { ...cjk, url: '' }] });
  const originalFetch = global.fetch;
  global.fetch = async url => {
    requests.push(url);
    return { ok: true, blob: async () => new Blob([new Uint8Array([1, 2])], { type: url.endsWith('.woff2') ? 'font/woff2' : 'font/woff' }) };
  };
  try {
    const html = '<script type="application/sealchat-fonts+json">{"version":1,"fonts":[{"platformFontId":"font","family":"Test"}]}</script>';
    const first = await sheet.resolveCharacterSheetFontCss(html, 'A');
    assert.match(first, /data:font\/woff2;base64,/);
    assert.doesNotMatch(first, /U\+4E00-9FFF/);
    const next = await sheet.resolveCharacterSheetFontCss(html, 'A中');
    assert.match(next, /unicode-range:U\+4E00-9FFF/);
    assert.deepEqual(requests, ['/api/v1/platform-fonts/font/subset/latin.woff2', '/api/v1/platform-fonts/font/subset/nested/cjk.woff']);
    config.meta.deliveryMode = 'single';
    const full = await sheet.resolveCharacterSheetFontCss(html, 'A');
    assert.match(full, /format\("truetype"\)/);
    assert.equal(requests.at(-1), '/api/v1/platform-fonts/font/file');
  } finally { global.fetch = originalFetch; }
});

test('CSP 保持原值，iframe 只将字体字节转换为 Blob URL', async () => {
  const host = fs.readFileSync(path.join(src, 'services/log/pagination/logFrameHost.ts'), 'utf8');
  assert.ok(host.includes("'font-src blob: data:'"));
  assert.ok(host.includes('"connect-src \'none\'"'));
  const runtime = fs.readFileSync(path.join(src, 'services/log/pagination/logFrameRuntime.js'), 'utf8');
  assert.doesNotMatch(runtime, /fetch\(|platform-fonts\//);
  const context = {
    window: { addEventListener() {} }, document: { addEventListener() {}, getElementById: () => ({ addEventListener() {} }) },
    URL: { createObjectURL: blob => { assert.match(blob.type, /^font\/woff2?$/); return 'blob:font-test'; } },
    Blob, ArrayBuffer, Map, Set,
  };
  vm.runInNewContext(runtime.replace(/\}\)\(\);\s*$/, 'globalThis.hooks = { receiveAssets, cssWithAssets };})();'), context);
  for (const mime of ['font/woff', 'font/woff2']) {
    const token = `platform-font:font:chunk:${mime.endsWith('2') ? 'aa' : 'bb'}`;
    context.hooks.receiveAssets([{ token, data: new ArrayBuffer(3), mime }]);
    const css = context.hooks.cssWithAssets(`@font-face{src:url("__LOGASSET_${token}__")}`);
    assert.equal(css, '@font-face{src:url("blob:font-test")}');
  }
});
