const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');
const pinia = require('pinia');

const storePath = '../src/stores/characterSheet.ts';
const sandboxPath = '../src/views/chat/components/character-sheet/IframeSandbox.vue';
const read = path => readFileSync(require.resolve(path), 'utf8');
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};
const tick = () => new Promise(resolve => setImmediate(resolve));

function load(source, imports, globals = {}) {
  const loaded = { exports: {} };
  const compiled = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText;
  vm.runInNewContext(compiled, {
    module: loaded, exports: loaded.exports, console, Error,
    require(name) {
      if (name in imports) return imports[name];
      if (name === 'vue') return vue;
      if (name === 'pinia') return pinia;
      throw new Error(`Unexpected import: ${name}`);
    }, ...globals,
  });
  return loaded.exports;
}

function timers() {
  const pending = new Map();
  let nextId = 0;
  return {
    pending,
    setTimeout(fn) { pending.set(++nextId, fn); return nextId; },
    clearTimeout(id) { pending.delete(id); },
    run() {
      const callbacks = [...pending.values()];
      pending.clear();
      callbacks.forEach(fn => fn());
    },
  };
}

const sheetTypes = load(read('../src/utils/characterSheetType.ts'), {});
const daggerheartHtml = read('../../doc/template/sealchat-daggerheart-template-v1.html');

function setup(update = async () => true, get = async () => ({ name: 'Hero', attrs: { hope: 0 } })) {
  pinia.setActivePinia(pinia.createPinia());
  const clock = timers();
  const calls = [];
  const card = {
    updateCardStrict(...args) { calls.push(args); return update(...args); },
    getActiveCard: get,
  };
  const { useCharacterSheetStore } = load(read(storePath), {
    '../../../doc/template/sealchat-default-template-v3-coc7th.html?raw': { default: '' },
    '../../../doc/template/sealchat-shinobigami-template-v1.html?raw': { default: '' },
    '../../../doc/template/sealchat-daggerheart-template-v1.html?raw': { default: daggerheartHtml },
    '../../../doc/template/sealchat-triangle-agency-template-v1.html?raw': { default: '<html>ta</html>' },
    '@/utils/characterSheetType': sheetTypes,
    './characterCard': { useCharacterCardStore: () => card },
    './characterCardTemplate': { useCharacterCardTemplateStore: () => ({}) },
    './chat': { useChatStore: () => ({}) },
    './display': { useDisplayStore: () => ({}) },
  }, { ...clock, localStorage: { removeItem() {} } });
  const store = useCharacterSheetStore();
  store.windows.sheet = {
    id: 'sheet', cardId: 'card', cardName: 'Hero', channelId: 'channel',
    attrs: { hope: 0 }, syncState: 'normal', hasLocalEditsInLock: false,
    hasSavedAfterEditEnd: false,
  };
  store.activeWindowIds.push('sheet');
  return { store, win: store.windows.sheet, clock, calls };
}

test('rapid button patches debounce to the latest attrs; concurrent flushes share one save', async () => {
  const saving = deferred();
  const { store, win, clock, calls } = setup(() => saving.promise);
  for (let hope = 1; hope <= 6; hope++) store.updateAttrs('sheet', { hope });
  assert.equal(win.syncState, 'normal', 'button protection does not impersonate an input edit lock');
  assert.equal(clock.pending.size, 1);
  const first = store.flushAttrsSync('sheet');
  const second = store.flushAttrsSync('sheet');
  assert.equal(calls.length, 1);
  assert.equal(calls[0][2].hope, 6);
  assert.equal(clock.pending.size, 0);
  let finished = false;
  first.then(() => { finished = true; });
  await tick();
  assert.equal(finished, false);
  saving.resolve(true);
  await Promise.all([first, second]);
  await store.flushAttrsSync('sheet');
  clock.run();
  assert.equal(calls.length, 1);
});

test('normal debounce saves without an explicit flush', async () => {
  const { store, clock, calls } = setup();
  store.updateAttrs('sheet', { hope: 3 });
  clock.run();
  await tick();
  assert.equal(calls.length, 1);
  assert.equal(calls[0][2].hope, 3);
  assert.equal(clock.pending.size, 0);
});

test('old refreshes before/during a local write cannot apply, including after save completion', async () => {
  for (const refreshBeforeWrite of [true, false]) {
    for (const replyAfterSave of [true, false]) {
      const remote = deferred();
      const { store, win, calls } = setup(undefined, () => remote.promise);
      let refresh;
      if (refreshBeforeWrite) refresh = store.refreshWindowAttrs('sheet');
      store.updateAttrs('sheet', { hope: 5 });
      if (!refreshBeforeWrite) refresh = store.refreshWindowAttrs('sheet');
      if (replyAfterSave) await store.flushAttrsSync('sheet');
      remote.resolve({ name: 'Hero', attrs: { hope: 0 } });
      await refresh;
      assert.equal(win.attrs.hope, 5);
      if (!replyAfterSave) await store.flushAttrsSync('sheet');
      assert.equal(calls[0][2].hope, 5);
    }
  }
});

test('after save, new SealDice hope/stress/fear snapshots refresh normally', async () => {
  const { store, win } = setup(undefined, async () => ({ name: 'Hero', attrs: { hope: 4, stress: 2, fear: 1 } }));
  store.updateAttrs('sheet', { hope: 5 });
  await store.flushAttrsSync('sheet');
  await store.refreshWindowAttrs('sheet');
  assert.equal(win.attrs.hope, 4);
  assert.equal(win.attrs.stress, 2);
  assert.equal(win.attrs.fear, 1);
});

test('input focus lock still buffers remote attrs and resumes only after edit/save', async () => {
  const { store, win } = setup();
  store.beginEditLock('sheet');
  await store.refreshWindowAttrs('sheet');
  assert.equal(win.pendingRemoteAttrs.hope, 0);
  store.endEditLock('sheet');
  assert.equal(win.syncState, 'normal');
  store.beginEditLock('sheet');
  store.updateAttrs('sheet', { hope: 3 });
  await store.flushAttrsSync('sheet');
  assert.equal(win.syncState, 'editing_locked');
  await store.refreshWindowAttrs('sheet');
  assert.equal(win.attrs.hope, 3);
  store.endEditLock('sheet');
  assert.equal(win.syncState, 'normal');
  await store.refreshWindowAttrs('sheet');
  assert.equal(win.attrs.hope, 0);
});

test('updates during a save serialize and coalesce; older success cannot unlock newer edits', async () => {
  const requests = [deferred(), deferred()];
  let index = 0;
  const { store, win, clock, calls } = setup(() => requests[index++].promise);
  store.beginEditLock('sheet');
  store.updateAttrs('sheet', { hope: 1 });
  const flushing = store.flushAttrsSync('sheet');
  store.updateAttrs('sheet', { hope: 2 });
  store.updateAttrs('sheet', { hope: 3 });
  store.endEditLock('sheet');
  clock.run(); // a debounce expiring during the request must not duplicate it
  assert.equal(calls.length, 1);
  requests[0].resolve(true);
  await tick();
  assert.equal(calls.length, 2);
  assert.equal(calls[1][2].hope, 3);
  assert.equal(win.syncState, 'resume_pending');
  assert.equal(win.hasSavedAfterEditEnd, false);
  await store.refreshWindowAttrs('sheet');
  assert.equal(win.attrs.hope, 3);
  requests[1].resolve(true);
  await flushing;
  assert.equal(win.syncState, 'normal');
  assert.equal(clock.pending.size, 0);
});

test('false and thrown save failures reject dependent rolls and keep local state for retry', async () => {
  for (const [throws, editing] of [[false, false], [true, false], [false, true], [true, true]]) {
    let fail = true;
    const { store, win } = setup(async () => {
      if (fail && throws) throw new Error('offline');
      return !fail;
    });
    if (editing) store.beginEditLock('sheet');
    store.updateAttrs('sheet', { hope: 5 });
    if (editing) store.endEditLock('sheet');
    let rolled = false;
    await assert.rejects(store.flushAttrsSync('sheet').then(() => { rolled = true; }));
    assert.equal(rolled, false);
    assert.equal(win.syncState, editing ? 'resume_pending' : 'normal');
    assert.equal(win.hasLocalEditsInLock, editing);
    assert.match(win.syncError, /保存失败/);
    await store.refreshWindowAttrs('sheet');
    assert.equal(win.attrs.hope, 5);
    fail = false;
    await store.flushAttrsSync('sheet');
    assert.equal(win.syncState, 'normal');
    assert.equal(win.syncError, '');
  }
});

test('readOnly/online preview never writes; close/reset discard timers and stale completions', async () => {
  const preview = setup();
  preview.win.readOnly = true;
  preview.win.cardId = 'online:card';
  preview.store.updateAttrs('sheet', { hope: 9 });
  await preview.store.flushAttrsSync('sheet');
  assert.equal(preview.win.attrs.hope, 0);
  assert.equal(preview.calls.length, 0);
  for (const close of ['closeSheet', 'reset']) {
    const saving = deferred();
    const { store, win, clock } = setup(() => saving.promise);
    store.updateAttrs('sheet', { hope: 2 });
    const flush = store.flushAttrsSync('sheet');
    store.updateAttrs('sheet', { hope: 3 });
    store[close]('sheet');
    assert.equal(clock.pending.size, 0);
    saving.resolve(true);
    await assert.rejects(flush, /窗口已关闭/);
    assert.equal(win.hasSavedAfterEditEnd, false);
    assert.equal(win.syncError, '');
    await assert.rejects(store.flushAttrsSync('sheet'), /窗口已关闭/);
  }
});

test('iframe flush API validates acknowledgements, rejects failures/timeouts and cleans up on close', async () => {
  const source = read(sandboxPath);
  const hook = source.match(/const SYNC_HOOK_SCRIPT = `<script[^>]*>([\s\S]*?)<\\\/script>`;/)[1];
  const listeners = {};
  const posted = [];
  const clock = timers();
  const parent = { postMessage: message => posted.push(message) };
  const window = { parent, addEventListener: (name, fn) => { listeners[name] = fn; } };
  vm.runInNewContext(hook, { window, ...clock });
  const receive = data => listeners.message({ source: parent, data });
  receive({ type: 'SEALCHAT_UPDATE', payload: { windowId: 'sheet' } });
  let done = false;
  const success = window.sealchat.flushAttrs().then(() => { done = true; });
  const ack = { type: 'SEALCHAT_ATTRS_FLUSHED', windowId: 'sheet', requestId: posted[0].payload.requestId, ok: true };
  listeners.message({ source: {}, data: ack });
  receive({ ...ack, windowId: 'other' });
  await tick();
  assert.equal(done, false);
  receive(ack);
  await success;
  assert.equal(clock.pending.size, 0);
  const failed = window.sealchat.flushAttrs();
  receive({ ...ack, requestId: posted[1].payload.requestId, ok: false, error: '保存失败' });
  await assert.rejects(failed, /保存失败/);
  const timeout = window.sealchat.flushAttrs();
  clock.run();
  await assert.rejects(timeout, /超时/);
  const closed = window.sealchat.flushAttrs();
  listeners.pagehide();
  await assert.rejects(closed, /窗口已关闭/);
  assert.equal(clock.pending.size, 0);
});

test('host only acknowledges its own iframe after save; failures and stale frames cannot report success', async () => {
  const { parse, compileScript } = require('@vue/compiler-sfc');
  const { descriptor } = parse(read(sandboxPath));
  const compiled = compileScript(descriptor, { id: 'sheet-sync-test' });
  let dispose;
  const clock = timers();
  const { default: component } = load(compiled.content, {
    vue: { ...vue, onMounted() {}, onBeforeUnmount(fn) { dispose = fn; } },
    '@/services/font/characterSheetTemplateFontRuntime': {},
  }, { ...clock, window: { removeEventListener() {} } });
  const saved = deferred();
  let saves = 0;
  const props = vue.reactive({
    html: '', data: { name: 'Hero', attrs: {} }, windowId: 'sheet',
    flushAttrs: () => { saves++; return saved.promise; },
  });
  const emitted = [];
  const scope = vue.effectScope();
  const api = scope.run(() => component.setup(props, { expose() {}, emit: (...args) => emitted.push(args) }));
  const replies = [];
  const source = vue.markRaw({ postMessage: message => replies.push(message) });
  api.iframeRef.value = vue.markRaw({ contentWindow: source });
  const data = {
    type: 'SEALCHAT_EVENT', version: 1, windowId: 'sheet',
    action: 'FLUSH_ATTRS', payload: { requestId: '1' },
  };
  await api.handleMessage({ source: {}, data });
  await api.handleMessage({ source, data: { ...data, windowId: 'other' } });
  assert.equal(saves, 0);
  const request = api.handleMessage({ source, data });
  assert.equal(saves, 1);
  assert.equal(replies.length, 0);
  saved.resolve();
  await request;
  assert.equal(replies[0].ok, true);
  assert.equal(replies[0].requestId, '1');
  props.flushAttrs = async () => { throw new Error('offline'); };
  await api.handleMessage({ source, data });
  assert.equal(replies[1].ok, false);
  assert.equal(replies[1].error, 'offline');

  // Existing events, including ordinary weapon damage, do not wait for saves.
  api.iframeRef.value.getBoundingClientRect = () => ({ top: 0, left: 0, width: 10, height: 10 });
  await api.handleMessage({ source, data: { ...data, action: 'ROLL_DICE', payload: { roll: { template: '.r 1d6' } } } });
  assert.equal(emitted[0][1].payload.roll.template, '.r 1d6');
  const stale = deferred();
  props.flushAttrs = () => stale.promise;
  const pending = api.handleMessage({ source, data });
  props.html = '<body>new document</body>';
  await vue.nextTick();
  stale.resolve();
  await pending;
  assert.equal(replies.length, 2);
  dispose();
  assert.equal(clock.pending.size, 0);
  scope.stop();
});

function setupDaggerheart(update) {
  const host = setup(update);
  const clock = timers();
  const rolls = [];
  const elements = new Map();
  const context = vm.createContext({
    ...clock,
    document: {
      addEventListener() {},
      getElementById(id) {
        if (!elements.has(id)) elements.set(id, { dataset: {}, classList: { add() {}, remove() {} } });
        return elements.get(id);
      },
    },
    window: {
      addEventListener() {},
      sealchat: {
        onUpdate() {},
        updateAttrs(patch) { host.store.updateAttrs('sheet', { ...host.win.attrs, ...patch }); },
        flushAttrs() { return host.store.flushAttrsSync('sheet'); },
        roll(...args) { rolls.push(args); },
      },
    },
  });
  const script = [...daggerheartHtml.matchAll(/<script>([\s\S]*?)<\/script>/g)].at(-1)[1];
  vm.runInContext(script, context);
  context.S.model = context.blankModel();
  return { ...host, context, rolls, elements };
}

test('Daggerheart aliases select the same default template', () => {
  const { store } = setup();
  for (const alias of ['daggerheart', 'dh', '匕首之心']) {
    assert.equal(store.getDefaultTemplate(alias), daggerheartHtml.trim());
  }
});

test('Daggerheart dd/ddr flush pending roots and model before rolling, deduplicating rapid submissions', async () => {
  for (const reaction of [false, true]) {
    const saved = deferred();
    const { context, calls, rolls } = setupDaggerheart(() => saved.promise);
    context.setRoot('希望', 4);
    context.setRoot('敏捷', 3);
    context.S.model.conditions.push('隐藏');
    context.touchModel();
    context.openRoll({ trait: '敏捷' });
    context.S.modal.reaction = reaction;
    const pending = context.submitModal();
    await context.submitModal();
    assert.equal(calls.length, 1);
    assert.equal(calls[0][2]['希望'], 4);
    assert.equal(calls[0][2]['敏捷'], 3);
    assert.equal(calls[0][2]['$匕首之心'].conditions[0], '隐藏');
    assert.equal(rolls.length, 0);
    saved.resolve(true);
    await pending;
    assert.equal(rolls.length, 1);
    assert.match(rolls[0][0], reaction ? /^\.ddr / : /^\.dd /);
    assert.equal(context.S.modal, null);
  }
});

test('Daggerheart save failure shows feedback, sends no roll and can retry', async () => {
  for (const fail of [() => false, () => { throw new Error('offline'); }]) {
    let retry = false;
    const { context, rolls, elements } = setupDaggerheart(async () => retry ? true : fail());
    context.setRoot('希望', 5);
    context.openRoll({});
    await context.submitModal();
    assert.equal(rolls.length, 0);
    assert.equal(context.S.modal.submitting, false);
    assert.match(elements.get('modalBox').innerHTML, /保存失败，本次未掷骰/);
    retry = true;
    await context.submitModal();
    assert.equal(rolls.length, 1);
  }
});

test('Daggerheart damage rolls bypass flush; cancelled submissions cannot send a delayed roll', async () => {
  const saved = deferred();
  const { context, rolls } = setupDaggerheart(() => saved.promise);
  context.setRoot('希望', 4);
  context.openRoll({});
  const pending = context.submitModal();
  context.closeModal();
  context.S.modal = { type: 'damage', expr: '2d8+3', reason: '长弓伤害' };
  await context.submitModal();
  assert.equal(rolls.length, 1);
  assert.equal(rolls[0][0], '.r 2d8+3 长弓伤害');
  saved.resolve(true);
  await pending;
  assert.equal(rolls.length, 1);
});
