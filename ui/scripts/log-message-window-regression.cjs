// node --test scripts/log-message-window-regression.cjs
// 执行真实 composable 与预览组件脚本，模拟消息 API 和 iframe 引擎。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');
const src = path.join(__dirname, '../src');
const read = file => fs.readFileSync(path.join(src, file), 'utf8');
const flush = () => new Promise(setImmediate);
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};
class LogDomainError extends Error {
  constructor(code, message = code) { super(message); this.code = code; }
}
const evaluate = (source, context) => vm.runInNewContext(ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, context);
const page = (start, nextCursor, sourceVersion = 1) => ({
  items: Array.from({ length: 100 }, (_, i) => ({ message: { id: `m${start + i}` }, version: 1 })),
  nextCursor, sourceVersion,
});

function setup(t) {
  const requests = [];
  const cleanup = [];
  const module = { exports: {} };
  const scope = vue.effectScope();
  evaluate(read('views/log/composables/useLogMessageWindow.ts'), {
    module, exports: module.exports, AbortController, setTimeout, clearTimeout,
    document: { addEventListener() {}, removeEventListener() {} },
    require: name => {
      if (name === 'vue') return { ...vue, onBeforeUnmount: fn => cleanup.push(fn) };
      if (name === '@/stores/chat') return { chatEvent: { on() {}, off() {} } };
      if (name === '@/services/log/logApi') return {
        LogDomainError, isLogError: (err, code) => err instanceof LogDomainError && err.code === code,
        describeLogError: err => err.message,
        fetchLogSectionMessages: (...args) => {
          const pending = deferred(); requests.push({ args, ...pending }); return pending.promise;
        },
      };
      throw new Error(name);
    },
  });
  const projectId = vue.ref('p1');
  const section = vue.ref({ id: 's1', source: { channelId: 'c1' } });
  const win = scope.run(() => module.exports.useLogMessageWindow(projectId, section));
  t.after(() => { cleanup.forEach(fn => fn()); scope.stop(); });
  return { requests, projectId, section, win };
}

test('首屏与每次显式追加只请求 100 条，100 → 200 → 300，最后一页才 complete', async t => {
  const { requests, win } = setup(t);
  assert.equal(requests.length, 1);
  assert.deepEqual(requests[0].args.slice(0, 4), ['p1', 's1', null, 100]);
  requests[0].resolve(page(0, 'cursor100'));
  await flush();
  assert.equal(requests.length, 1);
  assert.equal(win.items.value.length, 100);
  assert.equal(win.sourceVersion.value, 1);
  assert.equal(win.complete.value, false);
  for (const [index, cursor, next] of [[1, 'cursor100', 'cursor200'], [2, 'cursor200', null]]) {
    const work = win.loadMore();
    assert.equal(requests.length, index + 1);
    assert.deepEqual(requests[index].args.slice(0, 4), ['p1', 's1', cursor, 100]);
    assert.equal(win.items.value.length, index * 100);
    assert.equal(win.complete.value, false);
    requests[index].resolve(page(index * 100, next));
    await work;
    assert.equal(requests.length, index + 1);
    assert.equal(win.items.value.length, (index + 1) * 100);
    assert.equal(win.items.value.at(-1).message.id, `m${(index + 1) * 100 - 1}`);
    assert.equal(win.nextCursor.value, next);
    assert.equal(win.complete.value, next === null);
  }
  await win.loadMore();
  assert.equal(requests.length, 3);
  const reload = win.load();
  assert.equal(requests[3].args[2], null);
  requests[3].resolve(page(500, 'new-cursor'));
  await reload;
  assert.equal(win.items.value.length, 100);
  assert.equal(win.items.value[0].message.id, 'm500');
});

test('near-end 连续事件与直接 loadMore 都不能并发追加；网络失败保留窗口、游标并可重试', async t => {
  const { requests, section, win } = setup(t);
  requests[0].resolve(page(0, 'cursor100')); await flush();
  // 执行真实工作台处理函数，同时覆盖 UI 与 composable 两层保护。
  const source = read('views/log/components/LogWorkbench.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
  const ast = ts.createSourceFile('workbench.ts', source, ts.ScriptTarget.Latest, true);
  const handler = ast.statements.find(node => ts.isVariableStatement(node)
    && node.declarationList.declarations.some(decl => decl.name.getText(ast) === 'onNearEnd'));
  const ctx = { win, querySection: section, currentSection: section };
  evaluate(`${handler.getText(ast)}\nglobalThis.nearEnd = onNearEnd;`, ctx);
  ctx.nearEnd(); ctx.nearEnd(); ctx.nearEnd();
  await win.loadMore();
  assert.equal(requests.length, 2);
  requests[1].reject(new Error('network failed')); await flush();
  assert.equal(win.error.value, 'network failed');
  assert.equal(win.loading.value, false);
  assert.equal(win.items.value.length, 100);
  assert.equal(win.nextCursor.value, 'cursor100');
  ctx.nearEnd();
  assert.equal(requests.length, 2, '错误后等待手动重试，避免自动重试循环');
  const retry = win.loadMore();
  assert.equal(requests[2].args[2], 'cursor100');
  requests[2].resolve(page(100, null)); await retry;
  assert.equal(win.items.value.length, 200);
  assert.equal(new Set(win.items.value.map(item => item.message.id)).size, 200);
  assert.equal(win.error.value, '');
  assert.equal(win.complete.value, true);
});

for (const changed of ['SOURCE_CHANGED', 'version mismatch']) {
  test(`追加遇到 ${changed} 丢弃旧游标，从第一页重建，不混入旧版本`, async t => {
    const { requests, win } = setup(t);
    requests[0].resolve(page(0, 'old-cursor')); await flush();
    const work = win.loadMore();
    if (changed === 'SOURCE_CHANGED') requests[1].reject(new LogDomainError('SOURCE_CHANGED'));
    else requests[1].resolve(page(100, 'invalid', 2));
    await flush();
    assert.equal(win.items.value.length, 0);
    assert.equal(win.nextCursor.value, null);
    assert.equal(win.sourceVersion.value, null);
    assert.equal(win.complete.value, false);
    assert.equal(requests.length, 3);
    assert.deepEqual(requests[2].args.slice(0, 4), ['p1', 's1', null, 100]);
    requests[2].resolve(page(1000, 'new-cursor', 2)); await work;
    assert.equal(requests.length, 3);
    assert.equal(win.items.value.length, 100);
    assert.equal(win.items.value[0].message.id, 'm1000');
    assert.equal(win.sourceVersion.value, 2);
    assert.equal(win.nextCursor.value, 'new-cursor');
  });
}

for (const change of ['section', 'project', 'source', 'empty section']) {
  test(`${change} 切换取消旧请求，晚到响应不得污染新窗口`, async t => {
    const { requests, section, projectId, win } = setup(t);
    requests[0].resolve(page(0, 'old-cursor')); await flush();
    const oldWork = win.loadMore();
    if (change === 'section') section.value = { id: 's2', source: { channelId: 'c2' } };
    if (change === 'project') projectId.value = 'p2';
    if (change === 'source') section.value.source.channelId = 'c2';
    if (change === 'empty section') section.value = null;
    await vue.nextTick();
    assert.equal(requests[1].args[4].aborted, true);
    assert.equal(win.items.value.length, 0);
    assert.equal(win.sourceVersion.value, null);
    assert.equal(win.nextCursor.value, null);
    requests[1].resolve(page(100, null)); await oldWork;
    assert.equal(win.items.value.length, 0);
    if (change === 'empty section') {
      assert.equal(win.loading.value, false);
      assert.equal(requests.length, 2);
    } else {
      assert.equal(requests[2].args[2], null);
      requests[2].resolve(page(500, 'new-cursor', 2)); await flush();
      assert.equal(win.items.value.length, 100);
      assert.equal(win.items.value[0].message.id, 'm500');
      assert.equal(win.sourceVersion.value, 2);
    }
  });
}

test('预览转发 near-end，并在追加后的 layout 继续传递 lastAnchor；工作台保留手动入口', async () => {
  const layouts = [], events = [], mounted = [];
  const props = { doc: { blocks: [] }, assetTokens: [], selection: [], zoom: 1,
    prepareAssets: async doc => ({ doc, assets: [] }) };
  const source = read('views/log/components/LogPreviewFrame.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
  const ctx = {
    exports: {},
    defineProps: () => props, withDefaults: value => value,
    defineEmits: () => (...args) => events.push(args), defineExpose() {},
    require: name => {
      if (name === 'vue') return { ...vue, watch() {}, onMounted: fn => mounted.push(fn), onBeforeUnmount() {} };
      if (name === '@/services/log/pagination/logFrameHost') return {
        LogFrameEngine: class {
          ready = Promise.resolve();
          zoom() {} proofView() {} proofMarks() {} hasAsset() { return true; } async provideAssets() {}
          async layout(input, revision) { layouts.push(input); return { revision }; }
        },
      };
      throw new Error(name);
    },
  };
  evaluate(`${source}\nglobalThis.hooks = { onEvent, run, iframeRef };`, ctx);
  ctx.hooks.iframeRef.value = {};
  mounted[0](); await flush();
  const anchor = { nodeId: 'm99', fragmentIndex: 0, offsetWithinViewport: -24 };
  ctx.hooks.onEvent({ type: 'anchor', anchor, nearEnd: false });
  assert.equal(events.filter(event => event[0] === 'near-end').length, 0);
  ctx.hooks.onEvent({ type: 'anchor', anchor, nearEnd: true });
  assert.equal(events.filter(event => event[0] === 'near-end').length, 1);
  props.doc = { blocks: [{ id: 'm100' }] };
  await ctx.hooks.run();
  assert.deepEqual(JSON.parse(JSON.stringify(layouts.at(-1).anchor)), anchor);
  const workbench = read('views/log/components/LogWorkbench.vue');
  assert.match(workbench, /@near-end="onNearEnd"/);
  assert.match(workbench, /@click="win\.load\(\)">重试/);
  assert.match(workbench, /@click="win\.loadMore\(\)">加载更多/);
  assert.match(workbench, /composeDesignSection\([^\n]*win\.complete\.value/);
});

test('校对跨章节定位按需加载超过首屏的消息，并在预览及批注就绪后准确定位', async t => {
  const { requests, section, win } = setup(t);
  requests[0].resolve(page(0, null));
  await flush();
  const sectionId = vue.ref('s1');
  const pendingProofReveal = vue.shallowRef(null);
  const proofMarks = vue.shallowRef(null);
  const layoutInfo = vue.shallowRef(null);
  const seen = [], scrolled = [], errors = [];
  const previewRef = vue.shallowRef({ proofReveal: key => seen.push(key), scrollTo: id => scrolled.push(id) });
  const scope = vue.effectScope();
  t.after(() => scope.stop());
  scope.run(() => vue.watch(sectionId, id => { section.value = { id, source: { channelId: `c-${id}` } }; }, { flush: 'sync' }));
  const source = read('views/log/components/LogWorkbench.vue');
  const snippet = source.slice(source.indexOf('/** 预览和批注都已准备好时才定位'), source.indexOf('// ---- 定位搜索'));
  assert.ok(snippet.includes('const continueProofReveal'), '提取真实工作台的导航控制流程');
  const ctx = {
    win, sectionId, querySection: section, proofMarks, layoutInfo, previewRef,
    pendingProofReveal, sections: vue.ref([{ id: 's1' }, { id: 's2' }]),
    draftCtl: { status: vue.ref('saved') },
    toast: { error: msg => errors.push(msg), warning: msg => errors.push(msg) },
    watch: vue.watch, locate: id => scrolled.push(id),
  };
  scope.run(() => evaluate(`${snippet}\nglobalThis.navigation = { revealProof, locateProofMessage, finishProofReveal, continueProofReveal };`, ctx));
  ctx.navigation.revealProof('issue250', 'm250', 's2');
  await vue.nextTick();
  assert.equal(requests.length, 2, '切换章节后读取目标章节第一页');
  assert.equal(requests[1].args[1], 's2');
  requests[1].resolve(page(0, 'cursor100'));
  await vue.nextTick(); await flush();
  assert.equal(requests.length, 3, '未找到目标时才按原游标追加第二页');
  requests[2].resolve(page(100, 'cursor200'));
  await vue.nextTick(); await flush();
  assert.equal(requests.length, 4, '第二页仍不包含目标时继续追加');
  requests[3].resolve(page(200, null));
  await vue.nextTick(); await flush();
  assert.ok(win.byId('m250'));
  assert.deepEqual(seen, [], '消息出现但尚未排版、映射时不提前导航');
  proofMarks.value = [{ nodeId: 'm250', items: [{ key: 'issue250' }] }];
  await vue.nextTick();
  assert.deepEqual(seen, [], '仅有批注数据仍需等待布局完成');
  layoutInfo.value = { map: { m250: [3] } };
  ctx.navigation.finishProofReveal();
  assert.deepEqual(seen, ['issue250']);
  assert.equal(pendingProofReveal.value, null);
  // 点击消息标题定位也可以跨来源工作，不再依赖手动加载更多。
  ctx.navigation.locateProofMessage('m50', 's1');
  await vue.nextTick();
  assert.equal(requests[4].args[1], 's1');
  requests[4].resolve(page(0, null));
  await vue.nextTick(); await flush();
  layoutInfo.value = { map: { m50: [1] } };
  ctx.navigation.finishProofReveal();
  assert.deepEqual(scrolled, ['m50']);
  assert.equal(pendingProofReveal.value, null);
  assert.deepEqual(errors, []);
});
