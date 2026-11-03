// node --test scripts/log-multi-select-regression.cjs
// LOG 多选模式：批量移动规划、局部选择模型、工作台批量命令与 iframe 交互，均执行真实源码，不依赖浏览器或后端。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');
const read = file => fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');
const plain = value => JSON.parse(JSON.stringify(value));

/** 在当前 realm 中加载 TS 模块（数组/对象可直接 deepStrictEqual）。 */
function loadModule(file, modules = {}) {
  const output = ts.transpileModule(read(file), { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText;
  const module = { exports: {} };
  vm.runInThisContext(`(function (module, exports, require) {${output}\n})`)(module, module.exports, id => {
    if (id in modules) return modules[id];
    throw new Error(`unexpected import ${id}`);
  });
  return module.exports;
}

const ops = loadModule('views/log/logBatchOps.ts');
const draftOps = loadModule('views/log/logDraftOps.ts', { '@/services/log/logRenderer': {}, './logAssetRefs': {} });
const { useLogSelection } = loadModule('views/log/composables/useLogSelection.ts', { vue });
const ids = text => text.split('');

test('整体上移/下移：非连续选择逐段与相邻未选消息交换，顺序稳定且可逆', () => {
  const order = ids('abcdefgh');
  const up = ops.planBatchStep(order, new Set(ids('bdeg')), -1);
  assert.deepEqual(up, { placement: 'before', groups: [
    { messageIds: ['b'], targetMessageId: 'a' },
    { messageIds: ['d', 'e'], targetMessageId: 'c' },
    { messageIds: ['g'], targetMessageId: 'f' },
  ] });
  const moved = ops.applyBatchMovePlan(order, up);
  assert.equal(moved.join(''), 'badecgfh');
  assert.deepEqual(moved.filter(id => 'bdeg'.includes(id)), ids('bdeg'));
  const back = ops.applyBatchMovePlan(moved, ops.planBatchStep(moved, new Set(ids('bdeg')), 1));
  assert.equal(back.join(''), 'abcdefgh');
  const down = ops.planBatchStep(order, new Set(ids('acd')), 1);
  assert.equal(down.placement, 'after');
  assert.equal(ops.applyBatchMovePlan(order, down).join(''), 'baecdfgh');
});

test('边界：首段在顶部不可上移、末段在底部不可下移，不做部分移动', () => {
  const order = ids('abcde');
  assert.equal(ops.planBatchStep(order, new Set(ids('ad')), -1), null);
  assert.equal(ops.planBatchStep(order, new Set(ids('be')), 1), null);
  assert.equal(ops.planBatchStep(order, new Set(), -1), null);
  assert.ok(ops.planBatchStep(order, new Set(ids('ad')), 1));
});

test('批量重定位：保持原顺序，拒绝定位到选中集合内部与位置不变的目标', () => {
  const order = ids('abcdef');
  const plan = ops.planBatchRelocate(order, new Set(ids('eb')), 'c');
  assert.deepEqual(plan, { placement: 'after', groups: [{ messageIds: ['b', 'e'], targetMessageId: 'c' }] });
  assert.equal(ops.applyBatchMovePlan(order, plan).join(''), 'acbedf');
  assert.equal(ops.planBatchRelocate(order, new Set(ids('bd')), 'd'), null);
  assert.equal(ops.planBatchRelocate(order, new Set(ids('bc')), 'a'), null);
  assert.equal(ops.planBatchRelocate(order, new Set(ids('bc')), 'missing'), null);
  assert.ok(ops.planBatchRelocate(order, new Set(ids('bd')), 'a'));
});

test('选择模型：单选语义不变；多选点击切换、Shift 与范围模式并入、元素与空白不参与', () => {
  const order = vue.ref(['a', 'el-x', 'b', 'c', 'd', 'e']);
  const s = useLogSelection(order);
  s.select('b');
  s.select('d', { shift: true });
  assert.deepEqual(s.selected.value, ['b', 'c', 'd']);
  s.select('el-x');
  assert.deepEqual(s.selected.value, ['el-x']);
  s.enterMulti();
  assert.equal(s.multi.value, true);
  assert.deepEqual(s.selected.value, []);
  s.exitMulti();
  s.select('a', { additive: true });
  s.select('c', { additive: true });
  s.enterMulti();
  assert.deepEqual(s.selected.value, ['a', 'c']);
  s.enterMulti('d');
  assert.deepEqual(s.selected.value, ['d']);
  for (const id of [null, 'el-x']) s.select(id);
  assert.deepEqual(s.selected.value, ['d']);
  s.select('a');
  s.select('d');
  assert.deepEqual(s.selected.value, ['a']);
  s.select('b');
  s.select('d', { shift: true });
  assert.deepEqual(s.selected.value, ['a', 'b', 'c', 'd']);
  s.toggleRangeMode();
  s.select('e');
  assert.equal(s.rangeAnchor.value, 'e');
  s.select('d');
  assert.deepEqual(s.selected.value, ['a', 'b', 'c', 'd', 'e']);
  assert.equal(s.rangeAnchor.value, null);
  s.selectAll();
  assert.deepEqual(s.selected.value, ['a', 'b', 'c', 'd', 'e']);
  s.prune(new Set(['a', 'c', 'el-x']));
  assert.deepEqual(s.selected.value, ['a', 'c']);
  s.exitMulti();
  assert.equal(s.multi.value, false);
  assert.equal(s.rangeMode.value, false);
  assert.deepEqual(s.selected.value, []);
});

// ---- 工作台批量命令：从 LogWorkbench.vue 抽取真实声明 ----
const source = read('views/log/components/LogWorkbench.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
const ast = ts.createSourceFile('workbench.ts', source, ts.ScriptTarget.Latest, true);
const names = new Set(['selection', 'windowOrder', 'selectedMessages', 'placeAt', 'relocation', 'frameRelocation', 'relocateDialog',
  'relocateScrollId', 'cancelRelocate', 'startRelocate', 'relocateDestination', 'confirmRelocate', 'relocateSummary', 'pickRelocateTarget',
  'multiMode', 'selectedSet', 'multiAtTop', 'multiAtBottom', 'batchDialog', 'closeBatchDialog', 'openBatchDialog', 'enterMulti', 'exitMulti',
  'batchTarget', 'afterBatchMove', 'canBatch', 'moveMulti', 'excludeMulti', 'removeMulti', 'selectAllMulti', 'ctxMenu', 'ctxOptions',
  'openContextMenu', 'onContextSelect', 'sessionExclusions', 'setExcluded', 'excludeSelected', 'onFrameEvent', 'onEscape', 'onGlobalKey', 'locate', 'applyMessageTemplate']);

function setup(t, caps = {}) {
  const statements = ast.statements.filter(node => {
    if (ts.isVariableStatement(node)) return node.declarationList.declarations.some(decl => names.has(decl.name.getText(ast)));
    const text = node.getText(ast);
    return ts.isExpressionStatement(node) && /^(watch|onBeforeUnmount)\(/.test(text) && /cancelRelocate|selection\.prune/.test(text);
  });
  const calls = { relocate: [], remove: [], reorder: [], exclusions: [], dialogs: [], notices: [], opened: [], scrolls: [], cleanup: [], mutations: 0 };
  const make = id => ({ message: { id }, version: 3, capabilities: { reorder: true, remove: true, edit: true, ...(caps[id] || {}) } });
  const items = vue.shallowRef(ids('abcdefg').map(make));
  const win = {
    items, complete: vue.ref(true), loads: 0, reloads: 0, removed: [], own: [],
    byId: id => items.value.find(item => item.message.id === id) || null,
    indexOf: id => items.value.findIndex(item => item.message.id === id),
    load: () => { win.loads++; }, scheduleReload: () => { win.reloads++; },
    removeLocal: id => { win.removed.push(id); items.value = items.value.filter(item => item.message.id !== id); },
    markOwnWrite: (id, v) => win.own.push([id, v]),
  };
  const dialogFactory = kind => options => {
    const entry = { kind, ...options, destroy() { entry.destroyed = true; } };
    calls.dialogs.push(entry);
    return entry;
  };
  const context = {
    ref: vue.ref, computed: vue.computed, watch: vue.watch, shallowRef: vue.shallowRef,
    onBeforeUnmount: fn => calls.cleanup.push(fn), buildTimer: null, themeObserver: null, onResizeEnd() {},
    window: { removeEventListener() {} },
    useLogSelection, planBatchStep: ops.planBatchStep, planBatchRelocate: ops.planBatchRelocate,
    orderedIds: vue.computed(() => ['el-x', ...items.value.map(item => item.message.id)]),
    sectionId: vue.ref('s1'), projectId: vue.ref('p1'), currentSection: vue.ref({ source: { channelId: 'ch1' } }),
    querySection: vue.ref({ id: 's1' }), sourceChannelId: vue.ref('ch1'),
    previewPublish: vue.ref(false), busy: vue.ref(false), win, caps: vue.ref({ editDraft: true, manageSources: true }),
    nodeCache: new Map(), draft: vue.ref({ elements: [] }), editorShow: vue.ref(false), insertShow: vue.ref(false), focusElementId: vue.ref(null),
    selectedElement: vue.ref(null), messageTemplates: vue.ref(['normal', 'gm']), setMessageTemplateOverrides: draftOps.setMessageTemplateOverrides,
    toast: { info: text => calls.notices.push(text), success: text => calls.notices.push(text), error: text => calls.notices.push(text), warning() {} },
    dialog: { warning: dialogFactory('warning'), error: dialogFactory('error') },
    target: item => ({ messageId: item.message.id }),
    reorderMessage: async (...args) => calls.reorder.push(args),
    relocateMessagesBatch: async (...args) => calls.relocate.push(plain(args)),
    removeMessagesBatch: async (...args) => calls.remove.push(plain(args)),
    removeMessage: async () => { throw new Error('single remove must not be used'); },
    updateLogExclusions: async (...args) => { calls.exclusions.push(plain(args)); return { draftVersion: 2 }; },
    draftCtl: { runVersioned: fn => fn(1), update(fn) { calls.mutations++; fn(context.draft.value); } },
    runCommand: async (_label, command, after) => { context.busy.value = true; try { await command(); after(); } finally { context.busy.value = false; } },
    describeLogError: () => 'error', isLogError: () => false,
    openEditor: (...args) => calls.opened.push(args), openPanel() {}, insertAtMessage() {},
    previewRef: vue.ref({ scrollTo: id => calls.scrolls.push(id) }),
  };
  const scope = vue.effectScope();
  scope.run(() => vm.runInNewContext(ts.transpileModule(statements.map(node => node.getText(ast)).join('\n')
    + '\nglobalThis.hooks = { selection, relocation, multiMode, multiAtTop, multiAtBottom, ctxMenu, ctxOptions, enterMulti, exitMulti, moveMulti,'
    + ' excludeMulti, removeMulti, startRelocate, pickRelocateTarget, onFrameEvent, onGlobalKey, onContextSelect, selectAllMulti, locate, applyMessageTemplate };', {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText, context));
  t.after(() => scope.stop());
  return { ...context, ...context.hooks, calls };
}

const pick = (h, id, extra = {}) => h.onFrameEvent({ type: 'select', nodeId: id, kind: 'message', ...extra });

test('精确模板：整个 selected set 一次 mutation，自动删除，权限/元素/200条限制不可绕过', t => {
  const h = setup(t);
  h.enterMulti();
  for (const id of ['a', 'c', 'e']) pick(h, id);
  h.applyMessageTemplate('gm');
  assert.equal(h.calls.mutations, 1);
  assert.deepEqual(plain(h.draft.value.messageTemplateOverrides), { a: 'gm', c: 'gm', e: 'gm' });
  h.applyMessageTemplate('');
  assert.equal(h.calls.mutations, 2);
  assert.deepEqual(plain(h.draft.value.messageTemplateOverrides), {});
  h.applyMessageTemplate('missing');
  h.caps.value.editDraft = false;
  h.applyMessageTemplate('gm');
  h.caps.value.editDraft = true;
  h.selectedElement.value = { id: 'element' };
  h.applyMessageTemplate('gm');
  assert.equal(h.calls.mutations, 2);
  h.selectedElement.value = null;
  h.win.items.value = Array.from({ length: 201 }, (_, i) => ({ message: { id: `m${i}` }, capabilities: {} }));
  h.selection.selectAll();
  h.applyMessageTemplate('gm');
  assert.equal(h.calls.mutations, 2);
});

test('工程规则表单：拒绝空条件，增删/编辑/优先级只写草稿，用户与角色条件独立', () => {
  const source = read('views/log/components/LogBulkPanel.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
  const ast = ts.createSourceFile('rules.ts', source, ts.ScriptTarget.Latest, true);
  const names = new Set(['ruleEdit', 'ruleCanSave', 'editRule', 'setRuleMatch', 'saveRule', 'deleteRule', 'moveRule']);
  const declarations = ast.statements.filter(n => ts.isVariableStatement(n) && n.declarationList.declarations.some(d => names.has(d.name.getText(ast))));
  let mutations = 0;
  const props = { draft: {}, canEditDraft: true, templateIds: ['normal', 'gm'], update(fn) { mutations++; fn(props.draft); return true; } };
  let id = 0;
  const ctx = { props, ref: vue.ref, computed: vue.computed, cloneJson: plain, uuid: () => `rule-${++id}` };
  vm.runInNewContext(ts.transpileModule(declarations.map(n => n.getText(ast)).join('\n') + '\nglobalThis.hooks = {ruleEdit, editRule, setRuleMatch, saveRule, deleteRule, moveRule};', {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText, ctx);
  const h = ctx.hooks;
  h.editRule(); h.saveRule();
  assert.equal(mutations, 0);
  h.setRuleMatch('identityId', 'role'); h.setRuleMatch('userId', 'user'); h.setRuleMatch('variantId', 'variant'); h.saveRule();
  assert.deepEqual(plain(props.draft.displayRules[0].match), { identityId: 'role', userId: 'user', variantId: 'variant' });
  h.editRule(); h.setRuleMatch('messageType', 'ic'); h.saveRule();
  h.moveRule('rule-2', -1);
  assert.deepEqual(plain(props.draft.displayRules.map(r => r.id)), ['rule-2', 'rule-1']);
  h.editRule(props.draft.displayRules[1]); h.setRuleMatch('identityId', 'new-role'); h.saveRule();
  assert.deepEqual(plain(props.draft.displayRules[1].match), { identityId: 'new-role', userId: 'user' });
  h.deleteRule('rule-2');
  assert.equal(props.draft.displayRules.length, 1);
  props.canEditDraft = false;
  const before = mutations;
  h.deleteRule('rule-1'); h.moveRule('rule-1', 1); h.editRule(); h.saveRule();
  assert.equal(mutations, before);
});

test('模板选择状态：自动、指定、混合和失效引用均可识别', () => {
  const source = read('views/log/components/LogMessageTemplateSelect.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
  const ast = ts.createSourceFile('template-select.ts', source, ts.ScriptTarget.Latest, true);
  const declarations = ast.statements.filter(n => ts.isVariableStatement(n) && n.declarationList.declarations.some(d => ['value', 'options'].includes(d.name.getText(ast))));
  const props = { values: [undefined], templateIds: ['gm', 'normal'] };
  const get = () => {
    const ctx = { props, computed: vue.computed };
    vm.runInNewContext(ts.transpileModule(declarations.map(n => n.getText(ast)).join('\n') + '\nglobalThis.state = {value: value.value, options: options.value};', {
      compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
    }).outputText, ctx);
    return plain(ctx.state);
  };
  assert.equal(get().value, '');
  props.values = ['gm', 'gm'];
  assert.equal(get().value, 'gm');
  props.values = ['gm', undefined];
  assert.equal(get().value, null);
  props.values = ['removed', 'gm'];
  assert.equal(get().options.find(o => o.value === 'removed').disabled, true);
  assert.match(get().options.find(o => o.value === 'removed').label, /模板已不存在/);
});

test('右键原消息进入多选并直接选中该消息；多选期间单条交互被拦截', t => {
  const h = setup(t);
  h.onFrameEvent({ type: 'context', nodeId: 'c', x: 10, y: 20 });
  assert.equal(h.ctxMenu.value.show, true);
  assert.equal(h.ctxOptions.value[0].key, 'multi');
  h.onContextSelect('multi');
  assert.equal(h.multiMode.value, true);
  assert.deepEqual(h.selection.selected.value, ['c']);
  pick(h, 'e');
  pick(h, 'el-x');
  pick(h, null);
  pick(h, 'g', { shift: true });
  assert.deepEqual(h.selection.selected.value, ['c', 'e', 'f', 'g']);
  pick(h, 'e');
  assert.deepEqual(h.selection.selected.value, ['c', 'f', 'g']);
  for (const type of ['open', 'drop', 'action', 'imageResize']) h.onFrameEvent({ type, nodeId: 'a', targetId: 'b', side: 'after', action: 'edit' });
  assert.equal(h.calls.opened.length + h.calls.reorder.length, 0);
  h.onFrameEvent({ type: 'context', nodeId: 'f', x: 1, y: 1 });
  assert.equal(h.ctxOptions.value[0].label, '取消选中此消息');
  pick(h, null);
  assert.equal(h.ctxMenu.value.show, false);
  assert.deepEqual(h.selection.selected.value, ['c', 'f', 'g']);
  h.locate('a');
  assert.deepEqual(h.selection.selected.value, ['c', 'f', 'g']);
  assert.deepEqual(h.calls.scrolls, ['a']);
});

test('批量上移/下移一次提交 relocate.batch，携带全部版本，完成后刷新并保留选择', async t => {
  const h = setup(t);
  h.enterMulti('b');
  pick(h, 'd'); pick(h, 'e');
  assert.equal(h.multiAtTop.value, false);
  await h.moveMulti(-1);
  assert.equal(h.calls.relocate.length, 1);
  const [targetArg, plan] = h.calls.relocate[0];
  assert.equal(targetArg.channelId, 'ch1');
  assert.deepEqual(targetArg.items.map(it => it.messageId), ['b', 'd', 'e']);
  assert.ok(targetArg.items.every(it => it.expectedVersion === 3));
  assert.deepEqual(plan, { placement: 'before', groups: [{ messageIds: ['b'], targetMessageId: 'a' }, { messageIds: ['d', 'e'], targetMessageId: 'c' }] });
  assert.equal(h.win.loads, 1);
  assert.deepEqual(h.selection.selected.value, ['b', 'd', 'e']);
  pick(h, 'a');
  assert.equal(h.multiAtTop.value, true);
  await h.moveMulti(-1);
  assert.equal(h.calls.relocate.length, 1);
  await h.moveMulti(1);
  assert.equal(h.calls.relocate[1][1].placement, 'after');
});

test('权限按整个集合计算：任一条不可调整顺序或不可删除即不提交', async t => {
  const h = setup(t, { c: { reorder: false, remove: false } });
  h.enterMulti('b');
  pick(h, 'c');
  await h.moveMulti(1);
  h.startRelocate(h.selection.messageIds.value);
  h.removeMulti();
  assert.equal(h.relocation.value.active, false);
  assert.equal(h.calls.relocate.length + h.calls.dialogs.length, 0);
});

test('批量重定位：目标选择与二次确认，保持原顺序；Esc 分层只取消重定位再退出多选', async t => {
  const h = setup(t);
  h.enterMulti('e');
  pick(h, 'b');
  h.startRelocate(h.selection.messageIds.value);
  assert.deepEqual(plain(h.relocation.value.sourceMessageIds), ['b', 'e']);
  pick(h, 'e');
  pick(h, 'el-x');
  assert.equal(h.calls.dialogs.length, 0);
  assert.deepEqual(h.selection.selected.value, ['b', 'e']);
  h.onGlobalKey({ key: 'Escape' });
  assert.equal(h.relocation.value.active, false);
  assert.equal(h.multiMode.value, true);
  assert.deepEqual(h.selection.selected.value, ['b', 'e']);
  h.startRelocate(h.selection.messageIds.value);
  h.pickRelocateTarget('c');
  assert.equal(h.calls.dialogs.length, 1);
  assert.match(h.calls.dialogs[0].content, /2 条消息按原有顺序/);
  await h.calls.dialogs[0].onPositiveClick();
  assert.deepEqual(h.calls.relocate[0][1], { placement: 'after', groups: [{ messageIds: ['b', 'e'], targetMessageId: 'c' }] });
  assert.equal(h.calls.reorder.length, 0);
  assert.equal(h.relocation.value.active, false);
  assert.equal(h.multiMode.value, true);
  assert.deepEqual(h.selection.selected.value, ['b', 'e']);
  h.onFrameEvent({ type: 'key', key: 'Escape' });
  assert.equal(h.multiMode.value, false);
  assert.deepEqual(h.selection.selected.value, []);
});

test('排除需确认且只调用 LOG 排除接口；删除为危险确认并一次删除全部原消息', async t => {
  const h = setup(t);
  h.enterMulti('a');
  pick(h, 'c');
  h.excludeMulti();
  const exclude = h.calls.dialogs[0];
  assert.equal(exclude.kind, 'warning');
  assert.match(exclude.content, /只从当前 LOG 排除，不影响聊天原消息/);
  h.onGlobalKey({ key: 'Escape' });
  assert.equal(h.multiMode.value, true);
  await exclude.onPositiveClick();
  assert.deepEqual(h.calls.exclusions[0].slice(1), ['s1', 1, ['a', 'c'], true]);
  assert.equal(h.calls.remove.length, 0);
  assert.equal(h.multiMode.value, true);
  assert.deepEqual(h.selection.selected.value, []);
  pick(h, 'd'); pick(h, 'f');
  h.removeMulti();
  const remove = h.calls.dialogs[1];
  assert.equal(remove.kind, 'error');
  assert.match(remove.content, /删除选中的 2 条原消息/);
  assert.match(remove.content, /频道中的这些消息也会被删除/);
  await remove.onPositiveClick();
  assert.equal(h.calls.remove.length, 1);
  assert.deepEqual(h.calls.remove[0][0].items.map(it => it.messageId), ['d', 'f']);
  assert.deepEqual(h.win.removed, ['d', 'f']);
  assert.equal(h.win.reloads, 1);
  assert.deepEqual(h.selection.selected.value, []);
});

for (const change of ['section', 'source', 'project', 'publish', 'unmount']) {
  test(`${change} 切换时退出多选与重定位并销毁未完成的确认框`, async t => {
    const h = setup(t);
    h.enterMulti('a');
    pick(h, 'b');
    h.removeMulti();
    if (change === 'section') h.sectionId.value = 's2';
    if (change === 'source') h.currentSection.value = { source: { channelId: 'ch2' } };
    if (change === 'project') h.projectId.value = 'p2';
    if (change === 'publish') h.previewPublish.value = true;
    if (change === 'unmount') h.calls.cleanup.forEach(fn => fn());
    await vue.nextTick();
    assert.equal(h.calls.dialogs[0].destroyed, true);
    if (change !== 'unmount') {
      assert.equal(h.multiMode.value, false);
      assert.deepEqual(h.selection.selected.value, []);
    }
  });
}

test('全选限定当前片段已加载消息且不含元素；未完整加载时提示', t => {
  const h = setup(t);
  h.win.complete.value = false;
  h.enterMulti();
  h.selectAllMulti();
  assert.deepEqual(h.selection.selected.value, ids('abcdefg'));
  assert.match(h.calls.notices.at(-1), /尚未完整加载/);
});

// ---- 工具条：能力按整个选中集合计算 ----
test('多选工具条：混合权限时禁用对应操作并说明原因', async () => {
  const { compileScript, parse } = require('@vue/compiler-sfc');
  const { renderToString } = require('@vue/server-renderer');
  const { descriptor } = parse(read('views/log/components/LogMultiSelectBar.vue'), { filename: 'LogMultiSelectBar.vue' });
  const compiled = compileScript(descriptor, { id: 'multi', inlineTemplate: true, templateOptions: { ssr: true } });
  const icons = new Proxy({}, { get: () => ({ render: () => null }) });
  const output = ts.transpileModule(compiled.content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText;
  const module = { exports: {} };
  vm.runInThisContext(`(function (module, exports, require) {${output}\n})`)(module, module.exports, id => {
    if (id === 'vue') return vue;
    if (id === 'vue/server-renderer') return require('@vue/server-renderer');
    if (id === '@vicons/tabler') return icons;
    if (id === './LogMessageTemplateSelect.vue') return { default: { render: () => null } };
    throw new Error(`unexpected import ${id}`);
  });
  const message = (id, caps) => ({ message: { id }, version: 1, capabilities: { reorder: true, remove: true, ...caps } });
  const render = props => renderToString(vue.createSSRApp(module.exports.default, {
    total: 5, complete: true, atTop: false, atBottom: false, canManageSources: true, canEditDraft: true, templateIds: [], templateOverrides: {}, busy: false, rangeMode: false, rangeAnchored: false, relocating: false, ...props,
  }).component('n-icon', { props: ['component', 'size'], render: () => null }));
  const mixed = await render({ messages: [message('a'), message('b', { reorder: false, remove: false })] });
  assert.match(mixed, /整体上移一位：所选消息中有 1 条无权调整顺序/);
  assert.match(mixed, /重定位：选择目标消息，按原顺序移到其后：所选消息中有 1 条无权调整顺序/);
  assert.match(mixed, /删除聊天中的原消息：所选消息中有 1 条无权删除/);
  assert.match(mixed, /title="仅从本 LOG 排除（聊天原消息不受影响）"/);
  const ok = await render({ messages: [message('a'), message('b')], atTop: true });
  assert.match(ok, /整体上移一位：已到达本片段已加载范围的顶部/);
  assert.match(ok, /title="整体下移一位"/);
  assert.match(ok, /title="删除聊天中的原消息"/);
  const relocating = await render({ messages: [message('a')], relocating: true });
  assert.match(relocating, /取消重定位/);
  assert.doesNotMatch(relocating, /整体上移/);
});

// ---- iframe 运行时：多选 class、点击与右键 ----
test('iframe 多选：只切换原消息选择，隐藏单条操作；右键原消息上报坐标', () => {
  const runtime = read('services/log/pagination/logFrameRuntime.js');
  const listeners = {};
  const bodyClasses = new Set();
  const posted = [];
  const root = { addEventListener: (type, fn) => { (listeners[type] ||= []).push(fn); }, querySelectorAll: () => [] };
  const context = { Set, Map, document: { getElementById: () => root, addEventListener() {}, body: {
    classList: { toggle: (name, enabled) => enabled ? bodyClasses.add(name) : bodyClasses.delete(name) },
  } }, window: { addEventListener() {}, getSelection: () => '' } };
  vm.runInNewContext(runtime.replace(/\}\)\(\);\s*$/, 'globalThis.hooks = { current, applyMulti, setPost: fn => { post = fn; } }; })();')
    .replace('function post(msg) {', 'var post = function (msg) {'), context);
  const { current, applyMulti, setPost } = context.hooks;
  setPost(msg => posted.push(msg));
  const node = (id, kind) => ({ getAttribute: key => (key === 'data-log-node' ? id : kind) });
  const target = found => ({ closest: selector => (selector.includes('data-log-kind="message"') ? (found && found.getAttribute('data-log-kind') === 'message' ? found : null) : selector === '[data-log-node]' ? found : null) });
  const fire = (type, found, extra = {}) => listeners[type].forEach(fn => fn({ target: target(found), preventDefault() { extra.prevented = true; }, stopPropagation() {}, clientX: 5, clientY: 7, shiftKey: false, ctrlKey: false, metaKey: false, ...extra }));
  current.design = true;
  current.multi = true;
  applyMulti();
  assert.ok(bodyClasses.has('log-multi'));
  fire('click', node('m1', 'message'), { shiftKey: true });
  fire('click', node('el-1', 'element'));
  fire('click', null);
  assert.deepEqual(plain(posted), [
    { type: 'select', nodeId: 'm1', kind: 'message', shift: true, additive: false },
    { type: 'select', nodeId: 'el-1', kind: 'element', shift: false, additive: false },
    { type: 'select', nodeId: null, shift: false, additive: false },
  ]);
  fire('dblclick', node('m1', 'message'));
  assert.equal(posted.length, 3);
  const evt = {};
  fire('contextmenu', node('m2', 'message'), evt);
  assert.deepEqual(plain(posted.at(-1)), { type: 'context', nodeId: 'm2', x: 5, y: 7 });
  fire('contextmenu', node('el-1', 'element'));
  assert.equal(posted.length, 4);
  current.design = false;
  applyMulti();
  assert.equal(bodyClasses.size, 0);
});
