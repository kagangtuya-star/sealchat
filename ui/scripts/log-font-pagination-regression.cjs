// node --test scripts/log-font-pagination-regression.cjs
// 在最小测量 DOM 中执行真实 iframe runtime，核验字体等待、分页和旧修订丢弃。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

class Element {
  constructor(tag) {
    this.tagName = tag.toUpperCase(); this.childNodes = []; this.attrs = {}; this.className = '';
    this.style = { setProperty() {}, removeProperty() {} };
    this.classList = {
      contains: value => this.className.split(' ').includes(value),
      add: value => { this.className += ` ${value}`; }, remove() {}, toggle() {},
    };
  }
  addEventListener() {}
  setAttribute(key, value) { this.attrs[key] = value; }
  getAttribute(key) { return this.attrs[key] ?? null; }
  removeAttribute(key) { delete this.attrs[key]; }
  appendChild(child) { child.remove(); this.childNodes.push(child); child.parentNode = this; return child; }
  removeChild(child) { this.childNodes.splice(this.childNodes.indexOf(child), 1); child.parentNode = null; }
  replaceChildren(...children) { for (const child of this.childNodes.slice()) child.remove(); children.forEach(child => this.appendChild(child)); }
  remove() { this.parentNode?.removeChild(this); }
  get firstElementChild() { return this.childNodes[0] || null; }
  get offsetHeight() { return this.scrollHeight; }
  get scrollHeight() { return this.classList.contains('log-node') ? Number(this.innerHTML) : this.childNodes.reduce((sum, child) => sum + child.scrollHeight, 0); }
  get clientHeight() { return Number.parseFloat(this.style.height) || 10000; }
  get clientWidth() { return 10000; }
  get scrollWidth() { return 0; }
  getBoundingClientRect() { return { top: 0, bottom: 80 }; }
  matches(selector) {
    const anchor = selector.match(/^\[data-log-node="([^"]+)"\]$/);
    if (anchor) return this.attrs['data-log-node'] === anchor[1];
    if (selector === '[data-log-node]') return 'data-log-node' in this.attrs;
    if (selector.startsWith('.')) return this.classList.contains(selector.slice(1));
    return false;
  }
  querySelectorAll(selector) {
    return this.childNodes.flatMap(child => [...(child.matches(selector) ? [child] : []), ...child.querySelectorAll(selector)]);
  }
  closest(selector) { return this.matches(selector) ? this : this.parentNode?.closest(selector); }
}

const createRuntime = (options = {}) => {
  const root = new Element('main');
  const pkg = new Element('style');
  const body = new Element('body');
  const nodes = { 'log-root': root, 'log-package': pkg, 'log-page-rule': new Element('style') };
  const messages = [];
  const listeners = {};
  const scrolls = [];
  const window = { innerHeight: 800, scrollY: 0,
    addEventListener: (type, fn) => { listeners[type] = fn; },
    scrollBy: (x, y) => { scrolls.push(y); window.scrollY += y; }, CSS: { escape: value => value } };
  let releaseFont = () => {};
  const fontReady = options.neverReady ? new Promise(() => {}) : new Promise(resolve => { releaseFont = resolve; });
  let loadCalls = 0;
  const document = {
    body, documentElement: new Element('html'), addEventListener() {}, getElementById: id => nodes[id],
    createElement: tag => new Element(tag), getElementsByTagName: () => [],
    fonts: { ready: fontReady, forEach: fn => (options.fonts || ['sc-log-platform-font']).forEach(family => fn({ family, load: () => {
      assert.ok(pkg.textContent.includes('blob:chunk'), '@font-face 必须先将占位符替换为 Blob URL');
      assert.ok(body.childNodes[0].childNodes[0].childNodes.length > 0, '字体等待时正文已挂入隐藏 staging');
      loadCalls++; return options.neverReady ? new Promise(() => {}) : fontReady;
    } })) },
  };
  const context = {
    document, window,
    URL: { createObjectURL: () => 'blob:chunk' }, Blob, ArrayBuffer, CSS: { escape: value => value },
    performance,
    setTimeout: options.fastTimeout ? ((fn, ms) => setTimeout(fn, ms === 3000 ? 0 : ms)) : setTimeout,
    clearTimeout,
    requestAnimationFrame: fn => fn(),
    getComputedStyle: () => ({ breakBefore: 'auto', breakAfter: 'auto' }),
  };
  const source = fs.readFileSync(path.join(__dirname, '../src/services/log/pagination/logFrameRuntime.js'), 'utf8');
  vm.runInNewContext(source.replace(/\}\)\(\);\s*$/, 'globalThis.hooks = { render, receiveAssets, setPort: value => { port = value; } };})();'), context);
  context.hooks.setPort({ postMessage: message => messages.push(message) });
  context.hooks.receiveAssets([{ token: 'platform-font:font:chunk:aa', data: new ArrayBuffer(3), mime: 'font/woff2' }]);
  return { ...context.hooks, root, messages, releaseFont, window, document, listeners, scrolls, loadCalls: () => loadCalls };
};

const input = (revision, mode = 'paged') => ({
  revision, mode, selection: [], doc: {
    css: '@font-face{font-family:"sc-log-platform-font";src:url("__LOGASSET_platform-font:font:chunk:aa__")}',
    page: { widthMm: 100, heightMm: 26.4583333, marginsMm: { top: 0, right: 0, bottom: 0, left: 0 }, columns: 1 },
    blocks: ['a', 'b'].map(id => ({ id, kind: 'message', html: '80' })),
    rootVars: {}, rootAttrs: {},
  },
});

test('Blob 字体完成加载后才测量分页，正文分页结果稳定', async () => {
  const runtime = createRuntime();
  const work = runtime.render(input(1));
  await new Promise(setImmediate);
  assert.equal(runtime.loadCalls(), 1);
  assert.equal(runtime.messages.length, 0, '字体未完成时不得返回排版');
  assert.equal(runtime.root.childNodes.length, 0);
  runtime.releaseFont();
  await work;
  const layout = runtime.messages.find(message => message.type === 'layout');
  assert.equal(layout.pageCount, 2);
  assert.deepEqual(JSON.parse(JSON.stringify(layout.map)), { a: [1], b: [2] });
});

test('字体永久 pending 时总超时后继续分页，不无限等待 face.load 或 fonts.ready', async () => {
  const runtime = createRuntime({ neverReady: true, fastTimeout: true });
  await runtime.render(input(3));
  assert.equal(runtime.loadCalls(), 1);
  const layout = runtime.messages.find(message => message.type === 'layout');
  assert.equal(layout.revision, 3);
  assert.equal(layout.pageCount, 2);
});

test('页面、正文和 Ruby 的多个平台字体全部完成后才分页', async () => {
  const runtime = createRuntime({ fonts: ['sc-log-platform-font', 'sc-log-platform-body', 'sc-log-platform-rt'] });
  const work = runtime.render(input(4));
  await new Promise(setImmediate);
  assert.equal(runtime.loadCalls(), 3);
  assert.equal(runtime.messages.length, 0);
  runtime.releaseFont();
  await work;
  assert.equal(runtime.messages.find(message => message.type === 'layout').pageCount, 2);
});

test('字体等待期间新修订覆盖旧修订；flow 与 paged 共用等待链路', async () => {
  const runtime = createRuntime();
  const old = runtime.render(input(1));
  const current = runtime.render(input(2, 'flow'));
  runtime.releaseFont();
  await Promise.all([old, current]);
  const layouts = runtime.messages.filter(message => message.type === 'layout');
  assert.equal(layouts.length, 1);
  assert.equal(layouts[0].revision, 2);
  assert.equal(layouts[0].pageCount, 0);
  assert.equal(runtime.root.childNodes[0].childNodes.length, 2);
});

test('真实 LogFrameEngine 对已投递字体只补充新 chunk，不重复传输字节', async () => {
  const messages = [];
  const globals = {
    crypto: require('node:crypto').webcrypto, performance,
    MessageChannel: class { constructor() {
      this.port1 = { postMessage: message => messages.push(message), close() {} }; this.port2 = {};
    } },
  };
  // LogFrameEngine 复用 logFrameChannel 的通道实现；两者都按真实源码转译加载。
  const load = (file, requireImpl) => {
    const source = fs.readFileSync(path.join(__dirname, '../src/services/log/pagination', file), 'utf8');
    const loaded = { exports: {} };
    vm.runInNewContext(ts.transpileModule(source, {
      compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
    }).outputText, { module: loaded, exports: loaded.exports, require: requireImpl, ...globals });
    return loaded;
  };
  const channel = load('logFrameChannel.ts', name => assert.fail(`unexpected import ${name}`));
  const module = load('logFrameHost.ts', name => {
    if (name === './logFrameChannel') return channel.exports;
    assert.ok(name.endsWith('?raw'));
    return '';
  });
  const iframe = { setAttribute() {}, addEventListener() {}, removeEventListener() {}, contentWindow: { postMessage() {} } };
  const events = [];
  const engine = new module.exports.LogFrameEngine(iframe, event => events.push(event));
  engine.handshake(); engine.receive({ type: 'ready' });
  const a = { token: 'platform-font:font:chunk:aa', data: new ArrayBuffer(3), mime: 'font/woff2' };
  const b = { ...a, token: 'platform-font:font:chunk:bb' };
  await engine.provideAssets([a]);
  await engine.provideAssets([a, b]);
  await engine.provideAssets([a, b]);
  assert.deepEqual(messages.map(message => Array.from(message.items, item => item.token)), [[a.token], [b.token]]);
  assert.equal(a.data.byteLength, 3, '发送后字节缓存不被转移/清空');
  assert.ok(engine.hasAsset(a.token));
  assert.ok(engine.hasAsset(b.token));
  engine.receive({ type: 'anchor', anchor: null, nearEnd: true });
  assert.equal(events[0].nearEnd, true);
  engine.dispose();
});

test('iframe 中间位置不触发 nearEnd；接近底部提前触发并沿用 200ms 节流', async () => {
  const runtime = createRuntime();
  Object.defineProperty(runtime.document.documentElement, 'scrollHeight', { value: 5000 });
  runtime.window.scrollY = 1000;
  runtime.releaseFont();
  await runtime.render(input(1, 'flow'));
  assert.equal(runtime.messages.at(-1).type, 'anchor', 'layout 完成后主动报告视口');
  assert.equal(runtime.messages.at(-1).nearEnd, false);
  runtime.messages.length = 0;
  runtime.window.scrollY = 3500;
  runtime.listeners.scroll(); runtime.listeners.scroll(); runtime.listeners.scroll();
  assert.equal(runtime.messages.length, 0);
  await new Promise(resolve => setTimeout(resolve, 230));
  assert.equal(runtime.messages.length, 1);
  assert.equal(runtime.messages[0].nearEnd, true, '距底部还有 700px，无需滚到最后一个像素');
});

test('首屏不足一屏时 layout 主动报告 nearEnd；追加后恢复 anchor 再检查视口', async () => {
  const runtime = createRuntime();
  runtime.releaseFont();
  await runtime.render(input(1, 'flow'));
  assert.equal(runtime.messages.at(-1).nearEnd, true);
  const next = input(2, 'flow');
  next.doc.blocks.push({ id: 'c', kind: 'message', html: '80' });
  next.anchor = { nodeId: 'b', fragmentIndex: 0, offsetWithinViewport: -24 };
  await runtime.render(next);
  assert.equal(runtime.scrolls.at(-1), 24, '既有 restoreAnchor 修正原节点相对视口偏移');
  assert.equal(runtime.messages.at(-2).type, 'layout');
  assert.equal(runtime.messages.at(-1).nearEnd, true);
});
