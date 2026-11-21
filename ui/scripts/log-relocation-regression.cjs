// node --test scripts/log-relocation-regression.cjs
// 执行工作台真实命令与事件处理，避免依赖浏览器或后端。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');
const read = file => fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');
const source = read('views/log/components/LogWorkbench.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
const ast = ts.createSourceFile('workbench.ts', source, ts.ScriptTarget.Latest, true);

function setup(t) {
  const names = new Set(['placeAt', 'onDrop', 'relocation', 'frameRelocation', 'relocateDialog', 'relocateScrollId',
    'cancelRelocate', 'startRelocate', 'relocateDestination', 'confirmRelocate', 'relocateSummary',
    'pickRelocateTarget', 'onFrameEvent', 'onGlobalKey', 'onEscape', 'windowOrder', 'ctxMenu', 'openContextMenu',
    'batchDialog', 'closeBatchDialog', 'exitMulti', 'batchTarget', 'afterBatchMove']);
  const statements = ast.statements.filter(node => {
    if (ts.isVariableStatement(node)) return node.declarationList.declarations.some(decl => names.has(decl.name.getText(ast)));
    return ts.isExpressionStatement(node) && /^(watch|onBeforeUnmount)\(/.test(node.getText(ast)) && node.getText(ast).includes('cancelRelocate');
  });
  const requests = [], dialogs = [], notices = [], selected = [], drops = [], cleanup = [];
  const items = vue.shallowRef(['a', 'b', 'c'].map(id => ({ message: { id }, version: 1, capabilities: { reorder: true } })));
  const win = {
    items, byId: id => items.value.find(item => item.message.id === id),
    indexOf: id => items.value.findIndex(item => item.message.id === id),
    load: () => { win.loads++; }, loads: 0, markOwnWrite() {},
  };
  const context = {
    ref: vue.ref, computed: vue.computed, watch: vue.watch,
    onBeforeUnmount: fn => cleanup.push(fn), buildTimer: null, themeObserver: null,
    window: { removeEventListener() {} }, onResizeEnd() {},
    sectionId: vue.ref('s1'), projectId: vue.ref('p1'), currentSection: vue.ref({ source: { channelId: 'ch1' } }),
    previewPublish: vue.ref(false), busy: vue.ref(false), win,
    selection: { select: (...args) => selected.push(args), clear: () => selected.push(['clear']), multi: vue.ref(false), exitMulti() {} },
    planBatchRelocate: () => null, relocateMessagesBatch: async () => {}, sourceChannelId: vue.ref('ch1'),
    nodeCache: new Map(['a', 'b', 'c'].map(id => [id, { node: { kind: 'message', speakerName: id, plainText: '<正文> '.repeat(80) } }])),
    toast: { info: text => notices.push(text) },
    dialog: { warning: options => { const entry = { ...options, destroy() { entry.destroyed = true; } }; dialogs.push(entry); return entry; } },
    target: item => ({ messageId: item.message.id }),
    reorderMessage: async (...args) => requests.push(args),
    runCommand: async (_label, command, after) => { context.busy.value = true; try { await command(); after(); } finally { context.busy.value = false; } },
    draft: vue.ref({ elements: [] }), caps: vue.ref({ editDraft: true }), orderedIds: vue.ref(['a', 'b', 'c']),
    editorShow: vue.ref(false), insertShow: vue.ref(false), focusElementId: vue.ref(null),
    openEditor: () => drops.push('edit'), openPanel: () => drops.push('panel'), insertAtMessage: () => drops.push('insert'),
    draftCtl: { update: () => drops.push('resize') },
    proofreadPanelRef: vue.ref(null),
  };
  const scope = vue.effectScope();
  scope.run(() => vm.runInNewContext(ts.transpileModule(statements.map(node => node.getText(ast)).join('\n')
    + '\nglobalThis.hooks = { relocation, frameRelocation, startRelocate, cancelRelocate, pickRelocateTarget, onFrameEvent, onGlobalKey, onDrop };', {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText, context));
  t.after(() => scope.stop());
  return { ...context, ...context.hooks, requests, dialogs, notices, selected, drops, cleanup };
}

test('只允许单条有权限原消息启动；排除自身、元素、未加载消息和等价位置', t => {
  const h = setup(t);
  for (const ids of [[], ['a', 'b'], ['el-x'], ['missing']]) h.startRelocate(ids);
  h.win.items.value[0].capabilities.reorder = false;
  h.startRelocate(['a']);
  assert.equal(h.relocation.value.active, false);
  h.startRelocate(['b']);
  assert.deepEqual(structuredClone(h.frameRelocation.value).sourceMessageIds, ['b']);
  for (const id of [null, 'b', 'el-x', 'missing', 'a']) h.pickRelocateTarget(id);
  assert.equal(h.dialogs.length, 0);
  assert.equal(h.requests.length, 0);
  assert.equal(h.notices.length, 1);
  assert.equal(h.relocation.value.active, true);
});

test('select 选目标不改变选择；确认取消后仍可选其他目标，摘要为短纯文本', t => {
  const h = setup(t);
  h.startRelocate(['a']);
  h.onFrameEvent({ type: 'select', nodeId: 'c', shift: true, additive: true });
  assert.equal(h.selected.length, 1);
  assert.equal(h.dialogs.length, 1);
  const first = h.dialogs[0];
  assert.equal(first.title, '确认重定位');
  assert.ok(first.content.length < 160);
  assert.equal(typeof first.content, 'string');
  first.onAfterLeave();
  assert.equal(h.relocation.value.active, true);
  assert.equal(h.relocation.value.targetMessageId, null);
  h.pickRelocateTarget('b');
  assert.equal(h.dialogs.length, 2);
  assert.equal(h.requests.length, 0);
});

test('固定 after 仅执行一次 reorder，刷新并保持源消息选中；拖动排序仍支持 before', async t => {
  const h = setup(t);
  h.startRelocate(['a']); h.pickRelocateTarget('b');
  await h.dialogs[0].onPositiveClick();
  assert.equal(h.requests.length, 1);
  assert.deepEqual(JSON.parse(JSON.stringify(h.requests[0])), [{ messageId: 'a' }, { afterId: 'b', beforeId: 'c' }]);
  assert.equal(h.win.loads, 1);
  assert.equal(h.relocation.value.active, false);
  assert.equal(h.selected.at(-1)[0], 'a');
  h.onDrop('c', 'a', 'before');
  await Promise.resolve();
  assert.equal(h.requests[1][1].beforeId, 'a');
});

test('分页追加自然提供新目标；确认前顺序变化时重新检查等价位置', async t => {
  const h = setup(t);
  h.startRelocate(['a']);
  h.win.items.value = h.win.items.value.concat({ message: { id: 'd' }, version: 1, capabilities: {} });
  await vue.nextTick();
  assert.equal(h.relocation.value.active, true);
  h.pickRelocateTarget('d');
  assert.equal(h.dialogs.length, 1);
  h.win.items.value = [h.win.byId('b'), h.win.byId('c'), h.win.byId('d'), h.win.byId('a')];
  await h.dialogs[0].onPositiveClick();
  assert.equal(h.requests.length, 0);
  assert.equal(h.relocation.value.active, true);
});

for (const exit of ['section', 'source', 'publish', 'missing', 'permission', 'escape', 'frameEscape', 'cancel', 'unmount']) {
  test(`${exit} 退出重定位并销毁当前确认框`, async t => {
    const h = setup(t);
    h.startRelocate(['a']); h.pickRelocateTarget('c');
    if (exit === 'section') h.sectionId.value = 's2';
    if (exit === 'source') h.currentSection.value = { source: { channelId: 'ch2' } };
    if (exit === 'publish') h.previewPublish.value = true;
    if (exit === 'missing') h.win.items.value = h.win.items.value.slice(1);
    if (exit === 'permission') h.win.items.value = h.win.items.value.map(item => ({ ...item, capabilities: { reorder: false } }));
    if (exit === 'escape') h.onGlobalKey({ key: 'Escape' });
    if (exit === 'frameEscape') h.onFrameEvent({ type: 'key', key: 'Escape' });
    if (exit === 'cancel') h.cancelRelocate();
    if (exit === 'unmount') h.cleanup.forEach(fn => fn());
    await vue.nextTick();
    assert.equal(h.relocation.value.active, false);
    assert.equal(h.dialogs[0].destroyed, true);
  });
}

test('普通点击保持 Shift/additive；重定位期间拦截其他画布写操作', t => {
  const h = setup(t);
  h.onFrameEvent({ type: 'select', nodeId: 'b', shift: true, additive: true });
  assert.deepEqual(JSON.parse(JSON.stringify(h.selected[0])), ['b', { shift: true, additive: true }]);
  h.startRelocate(['a']);
  for (const type of ['open', 'drop', 'action', 'imageResize']) h.onFrameEvent({ type, nodeId: 'b', targetId: 'c', side: 'after', action: 'edit' });
  assert.equal(h.requests.length, 0);
  assert.equal(h.drops.length, 0);
});

test('iframe 设计状态同时标记源与其他原消息，发布模式与退出后无重定位 class', () => {
  const runtime = read('services/log/pagination/logFrameRuntime.js');
  const makeNode = (id, kind) => {
    const classes = new Set();
    return { classes, getAttribute: key => key === 'data-log-kind' ? kind : id,
      classList: { toggle: (name, enabled) => enabled ? classes.add(name) : classes.delete(name) } };
  };
  const nodes = [makeNode('a', 'message'), makeNode('b', 'message'), makeNode('el-x', 'element')];
  const bodyClasses = new Set();
  const root = { addEventListener() {}, querySelectorAll: () => nodes };
  const context = { Set, Map, document: { getElementById: () => root, addEventListener() {}, body: {
    classList: { toggle: (name, enabled) => enabled ? bodyClasses.add(name) : bodyClasses.delete(name) },
  } }, window: { addEventListener() {} } };
  vm.runInNewContext(runtime.replace(/\}\)\(\);\s*$/, 'globalThis.hooks = { current, applyRelocation }; })();'), context);
  const { current, applyRelocation } = context.hooks;
  current.design = true;
  current.relocation = { sourceMessageIds: ['a'] };
  applyRelocation();
  assert.ok(nodes[0].classes.has('log-relocate-source'));
  assert.ok(nodes[1].classes.has('log-relocate-target'));
  assert.equal(nodes[2].classes.size, 0);
  nodes.push(makeNode('c', 'message'));
  applyRelocation();
  assert.ok(nodes[3].classes.has('log-relocate-target'));
  for (const change of ['publish', 'cancel']) {
    if (change === 'publish') current.design = false;
    else { current.design = true; current.relocation = null; }
    applyRelocation();
    assert.equal(bodyClasses.size, 0);
    assert.ok(nodes.every(node => node.classes.size === 0));
  }
});
