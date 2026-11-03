// node --test scripts/log-image-layout-regression.cjs
// 真实 renderer/草稿操作及 iframe 手势；最小 DOM 不替代浏览器视觉验证。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const Module = require('node:module');
const vm = require('node:vm');
const ts = require('typescript');
const src = path.join(__dirname, '../src');

// 只模拟模板序列化所需的 img 属性，正文清洗保留既有独立安全测试边界。
function template() {
  let html = '';
  let images = [];
  return {
    appendChild(frag) { this.innerHTML = frag.html(); },
    set innerHTML(value) {
      images = [];
      html = value.replace(/<img\b[^>]*>/g, (tag, offset) => {
        const attrs = new Map(Array.from(tag.matchAll(/([\w-]+)="([^"]*)"/g), m => [m[1], m[2]]));
        images.push({ body: value.slice(0, offset).includes('class="log-body"'),
          getAttribute: key => attrs.get(key), setAttribute: (key, val) => attrs.set(key, val), removeAttribute: key => attrs.delete(key),
          html: () => `<img ${Array.from(attrs, ([key, val]) => `${key}="${val}"`).join(' ')}>` });
        return `__IMG_${images.length - 1}__`;
      });
    },
    get innerHTML() { return html.replace(/__IMG_(\d+)__/g, (_, i) => images[i].html()); },
    content: {
      html: () => html.replace(/__IMG_(\d+)__/g, (_, i) => images[i].html()),
      querySelectorAll: selector => selector === '.log-body img' ? images.filter(img => img.body) : images,
    },
  };
}
globalThis.document = { createElement: () => template() };
const compiler = {
  buildTemplateLibrary: () => ({ templates: new Map([['imagePlate', true], ['cover', true], ['message', true]]), issues: [] }),
  messageTemplateIds: () => ['message'],
  instantiateTemplate: (_lib, id, data) => id === 'message'
    ? `<img data-log-asset="att:avatar"><div class="log-body">${layout.markLogImages(data.bodyHtml, { kind: 'message', id: data.bodyImageLayout.messageId, widths: data.bodyImageLayout.widths })}</div>`
    : `<figure><img data-log-asset="${data.assets.assetUrl}"></figure>`,
  fragmentToHtml: value => value, sanitizeBodyHtml: html => html, templateUsesField: () => false, isSafeTokenValue: () => true,
};
const stubs = {
  './logContent': { renderMessageBody: html => ({ html, plainText: '', assets: [] }), normalizeMentionClasses() {} },
  './logTemplateCompiler': compiler,
  './logFontPlan': { logPlatformFontId: () => null },
};
const resolve = Module._resolveFilename;
Module._resolveFilename = function (request, parent, ...args) {
  if (Object.hasOwn(stubs, request)) return `stub:image-layout:${request}`;
  return resolve.call(this, request.startsWith('@/') ? path.join(src, request.slice(2)) : request, parent, ...args);
};
for (const [name, exports] of Object.entries(stubs)) {
  const filename = `stub:image-layout:${name}`;
  Module._cache[filename] = { id: filename, filename, loaded: true, exports };
}
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename);
const layout = require(path.join(src, 'services/log/logImageLayout.ts'));
const renderer = require(path.join(src, 'services/log/logRenderer.ts'));
const { rebindStyle } = require(path.join(src, 'views/log/logDraftOps.ts'));
const style = { templates: { imagePlate: '', cover: '', message: '' }, assets: [], settingsSchema: {}, rules: [{ templateId: 'message' }], tokens: {} };
const draft = () => ({ schemaVersion: 1, elements: [{ id: 'e1', type: 'imagePlate', templateId: 'imagePlate', content: { assetId: 'bg' }, style: {} }], page: { fontFamily: 'sans-serif' }, themeTokens: {} });
const build = (nodes, extra = {}) => renderer.buildFrameDocument({ style, page: { fontFamily: 'sans-serif', backgroundColor: '#ffffff' }, title: '', sectionTitle: '', nodes, toc: [], ...extra }).doc;

test('不完整窗口保留末尾与未加载消息锚点，只有完整后才插入章节末尾元素', () => {
  const elements = [
    { id: 'start', type: 'pageBreak', anchor: { kind: 'sectionStart', sectionId: 's1' }, order: 0, content: {}, style: {} },
    { id: 'end', type: 'pageBreak', anchor: { kind: 'sectionEnd', sectionId: 's1' }, order: 1, content: {}, style: {} },
    { id: 'future', type: 'pageBreak', anchor: { kind: 'message', sectionId: 's1', messageId: 'm200', side: 'after' }, order: 2, content: {}, style: {} },
  ];
  const partial = renderer.composeDesignSection('s1', '章节', [], elements, false, new Map());
  assert.deepEqual(partial.nodes.map(node => node.id), ['el-start']);
  assert.equal(partial.anchorStates.get('end'), 'unloaded');
  assert.equal(partial.anchorStates.get('future'), 'unloaded');
  const complete = renderer.composeDesignSection('s1', '章节', [], elements, true, new Map());
  assert.deepEqual(complete.nodes.map(node => node.id), ['el-start', 'el-end']);
  assert.equal(complete.anchorStates.get('end'), 'attached');
  assert.equal(complete.anchorStates.get('future'), 'orphaned');
});

test('element 自动/显式比例、非法值、cover 和模板切换', () => {
  const d = draft();
  const render = () => build([renderer.designElementToNode(d.elements[0])]).blocks[0].html;
  assert.match(render(), /data-log-resizable-image="element"/);
  assert.doesNotMatch(render(), /data-log-image-width/);
  layout.updateDraftImageWidth(d, { targetKind: 'element', nodeId: 'el-e1', width: 0.5 });
  assert.match(render(), /data-log-image-width="0.5"/);
  d.elements[0].content.imageWidth = Infinity;
  assert.doesNotMatch(render(), /data-log-image-width/);
  d.elements[0].content.imageWidth = 9;
  assert.match(render(), /data-log-image-width="1"/);
  d.elements[0].content.imageWidth = 0.5;
  d.elements[0].type = 'cover'; d.elements[0].templateId = 'cover';
  assert.match(render(), /data-log-image-width="0.5"/);
  const rebound = rebindStyle(d, { ...style, id: 'new', version: '1', assets: [{ id: 'bg', mime: 'image/png' }] }).next;
  assert.equal(rebound.elements[0].content.imageWidth, 0.5);
  layout.updateDraftImageWidth(d, { targetKind: 'element', nodeId: 'el-e1', width: null });
  assert.ok(!('imageWidth' in d.elements[0].content));
});

test('同消息多图 key 与 URL 无关，仅覆盖指定图片；发布序号映射一致', () => {
  const node = { kind: 'message', id: 'm1', assets: [], clues: [], bodyHtml: '<img data-log-asset="att:first"><img data-log-asset="att:second">', speakerName: '' };
  const key = layout.messageImageKey('m1', 0);
  const html = build([node], { imageWidths: { [key]: 0.4 } }).blocks[0].html;
  assert.match(html, /data-log-image-key="message:m1:image:0" data-log-image-width="0.4"/);
  assert.match(html, /data-log-image-key="message:m1:image:1"/);
  assert.equal((html.match(/data-log-image-width/g) || []).length, 1);
  assert.equal((html.match(/data-log-resizable-image/g) || []).length, 2, '头像不可调整');
  assert.equal(build([{ ...node, bodyHtml: node.bodyHtml.replaceAll('att:', 'pub:') }], { imageWidths: { [key]: 0.4 } }).blocks[0].html.match(/data-log-image-key="[^"]+"/g).join(), html.match(/data-log-image-key="[^"]+"/g).join());
  assert.doesNotMatch(build([node]).blocks[0].html, /data-log-image-width/);
  const publicNode = renderer.publicNodeToRender({ ...node, id: 'public-id', speaker: { key: 'speaker', name: '' }, imageWidths: { '0': 0.4 },
    bodyHtml: `<img src="/api/v1/public/log/slug1234/assets/${'a'.repeat(32)}"><img src="/api/v1/public/log/slug1234/assets/${'b'.repeat(32)}">` });
  assert.equal(publicNode.imageWidths['message:public-id:image:0'], 0.4);
  const pubHtml = build([publicNode]).blocks[0].html;
  assert.match(pubHtml, /data-log-image-key="message:public-id:image:0" data-log-image-width="0.4"/);
  assert.match(pubHtml, /data-log-image-key="message:public-id:image:1"/);
  assert.equal((pubHtml.match(/data-log-image-width/g) || []).length, 1);
});

test('草稿旧字段兼容、clamp、恢复自动和拒绝错误节点 key', () => {
  const d = draft();
  const event = { targetKind: 'message', nodeId: 'm1', imageKey: 'message:m1:image:1', width: 0.01 };
  layout.updateDraftImageWidth(d, event);
  assert.equal(d.imageWidths[event.imageKey], 0.15);
  layout.updateDraftImageWidth(d, { ...event, imageKey: 'message:m2:image:1', width: 0.4 });
  layout.updateDraftImageWidth(d, { ...event, width: NaN });
  assert.deepEqual(d.imageWidths, { [event.imageKey]: 0.15 });
  layout.updateDraftImageWidth(d, { ...event, width: null });
  assert.ok(!('imageWidths' in d));
  assert.equal(layout.normalizeImageWidth('50%'), undefined);
});

function emitter() {
  const listeners = new Map();
  return { listeners,
    addEventListener(type, fn) { if (!listeners.has(type)) listeners.set(type, new Set()); listeners.get(type).add(fn); },
    removeEventListener(type, fn) { listeners.get(type)?.delete(fn); },
    fire(type, event) { for (const fn of listeners.get(type) || []) { fn(event); if (event.immediateStopped) break; } },
  };
}
function runtime() {
  const window = { ...emitter() };
  const root = { ...emitter(), querySelectorAll: () => [], replaceChildren() {}, style: {} };
  const messages = [];
  const context = { window, document: { ...emitter(), getElementById: () => root }, Map, Set, performance, clearInterval, clearTimeout,
    getComputedStyle: el => ({ paddingLeft: '0', paddingRight: '0', maxHeight: el.maxHeight || 'none' }),
  };
  const source = fs.readFileSync(path.join(src, 'services/log/pagination/logFrameRuntime.js'), 'utf8');
  vm.runInNewContext(source.replace(/\}\)\(\);\s*$/, 'globalThis.hooks = { sizeImages, endImageResize, dispose, handle, setPort: (p, canResizeImages = true) => { port = p; current.design = true; current.canResizeImages = canResizeImages; } };})();'), context);
  context.hooks.setPort({ postMessage: event => messages.push(event), close() {} });
  return { ...context.hooks, window, root, messages };
}
function imageFixture(column = 400, naturalWidth = 1200, naturalHeight = 600, zoom = 1) {
  const classes = new Set();
  const node = { clientWidth: column, closest: () => node, getAttribute: key => key === 'data-log-node' ? 'm1' : 'message' };
  const box = { ...emitter(), style: {}, parentElement: { clientWidth: column }, classList: { add: v => classes.add(v), remove: v => classes.delete(v) },
    get offsetWidth() { return parseFloat(this.style.width) || 0; }, getBoundingClientRect() { return { width: this.offsetWidth * zoom }; } };
  const attrs = { 'data-log-resizable-image': 'message', 'data-log-image-key': 'message:m1:image:0' };
  const img = { naturalWidth, naturalHeight, renderedWidth: Math.min(naturalWidth, column), parentElement: box, getAttribute: key => attrs[key] || null,
    getBoundingClientRect() { return { width: this.renderedWidth }; }, closest: () => node };
  box.querySelector = () => img;
  const control = { ...emitter(), parentElement: box, closest: selector => selector === '[data-log-image-action]' ? control : null, getAttribute: () => 'resize', setPointerCapture() {}, hasPointerCapture: () => true, releasePointerCapture() {} };
  node.querySelectorAll = () => [img];
  const pointer = (x, id = 1) => ({ target: control, button: 0, pointerId: id, clientX: x, preventDefault() {}, stopImmediatePropagation() { this.immediateStopped = true; } });
  return { img, node, box, attrs, pointer };
}

test('自动尺寸不放大小图、大图为实际栏宽75%、显式比例及超高图保护', () => {
  const r = runtime();
  for (const [column, natural, expected] of [[400, 1200, 300], [400, 80, 80], [180, 1200, 135]]) {
    const f = imageFixture(column, natural);
    r.sizeImages(f.node);
    assert.equal(parseFloat(f.box.style.width), expected);
    f.attrs['data-log-image-width'] = '0.5';
    r.sizeImages(f.node);
    assert.equal(parseFloat(f.box.style.width), column * 0.5);
  }
  const tall = imageFixture(400, 400, 2000);
  tall.img.maxHeight = '600px';
  tall.img.renderedWidth = 120;
  r.sizeImages(tall.node);
  assert.equal(parseFloat(tall.box.style.width), 120, '高度不超过单页600px');
  const compact = imageFixture(400, 1200);
  compact.img.renderedWidth = 192;
  r.sizeImages(compact.node);
  assert.equal(parseFloat(compact.box.style.width), 192, '自动尺寸尊重样式包既有 max-width');
  compact.attrs['data-log-image-width'] = '0.75';
  compact.box.classList.remove('log-image-box--sized');
  r.sizeImages(compact.node);
  assert.equal(parseFloat(compact.box.style.width), 300, '显式拖动宽度可覆盖样式包宽度约束');
});

test('zoom/双栏拖动只有松手发送一次事件，取消和卸载恢复并清理监听', () => {
  {
    const r = runtime();
    const f = imageFixture(200, 1200, 600);
    r.sizeImages(f.node);
    r.setPort({ postMessage: event => r.messages.push(event), close() {} }, false);
    r.root.fire('pointerdown', f.pointer(0));
    r.window.fire('pointermove', f.pointer(50));
    r.window.fire('pointerup', f.pointer(50));
    assert.equal(f.box.style.width, '150px');
    assert.equal(r.messages.length, 0, '只读设计态不能启动图片缩放');
  }
  for (const zoom of [0.5, 1, 1.75]) {
    const r = runtime();
    const f = imageFixture(200, 1200, 600, zoom);
    r.sizeImages(f.node);
    r.root.fire('pointerdown', f.pointer(0));
    r.window.fire('pointermove', f.pointer(-70 * zoom));
    assert.equal(f.box.style.width, '80px');
    assert.equal(r.messages.length, 0);
    r.window.fire('pointerup', f.pointer(-70 * zoom));
    assert.equal(r.messages.length, 1);
    assert.equal(r.messages[0].width, 0.4);
    assert.equal(r.messages[0].imageKey, 'message:m1:image:0');
    for (const type of ['pointermove', 'pointerup', 'pointercancel', 'blur']) assert.equal(r.window.listeners.get(type).size, 0);
    r.root.fire('pointerdown', f.pointer(0));
    r.window.fire('pointermove', f.pointer(500));
    assert.equal(f.box.style.width, '200px');
    r.window.fire('pointercancel', f.pointer(500));
    assert.equal(f.box.style.width, '80px');
    assert.equal(r.messages.length, 1);
    r.root.fire('pointerdown', f.pointer(0));
    r.window.fire('pointermove', f.pointer(-1000));
    assert.equal(f.box.style.width, '30px');
    r.dispose();
    assert.equal(r.window.listeners.get('pointermove').size, 0);
    assert.equal(f.box.style.width, '80px');
    assert.equal(r.messages.length, 1);
  }
});
