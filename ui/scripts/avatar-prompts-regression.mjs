import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';
import { reactive, ref, watch } from 'vue';

const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
}).outputText;
const source = read('../src/views/chat/chat.vue').match(/<script setup[^>]*>([\s\S]*?)<\/script>/)[1];
const ast = ts.createSourceFile('chat.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const names = ['avatarPromptVisible', 'avatarPromptUserId', 'avatarPromptTimer', 'promptContextRevision',
  'promptsDisposed', 'cancelIdentitySyncPrompt', 'handleOpenAvatarPrompt', 'handleAvatarPromptSetup',
  'handleAvatarPromptSkip', 'checkAvatarPromptOnMount', 'identitySyncPromptPending',
  'isInObserverMode', 'canManageIdentities', 'maybePromptIdentitySync'];
const declarations = names.map(name => {
  const statement = ast.statements.find(node => ts.isVariableStatement(node)
    && node.declarationList.declarations.some(entry => entry.name.getText(ast) === name));
  assert.ok(statement, `missing declaration ${name}`);
  return statement.getText(ast);
});
const effects = ast.statements.filter(node => ts.isExpressionStatement(node)
  && ts.isCallExpression(node.expression)
  && ['watch', 'onBeforeUnmount'].includes(node.expression.expression.getText(ast))
  && /promptContextRevision\+\+|promptsDisposed = true|if \(!hasDefaultAvatar\)/.test(node.getText(ast)));
assert.equal(effects.length, 3);
const tick = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };

function harness(storage = new Map(), { blocked = false, userId = 'user-a', worldId = 'world-a' } = {}) {
  const exports = {};
  vm.runInNewContext(compile(read('../src/utils/promptDismissal.ts')), {
    exports,
    localStorage: {
      getItem(key) { if (blocked) throw new Error('storage disabled'); return storage.get(key) ?? null; },
      setItem(key, value) { if (blocked) throw new Error('storage disabled'); storage.set(key, value); },
    },
  });
  const timers = new Map(), cleanup = [], stops = [], dialogs = [], events = [];
  let timerId = 0;
  const user = reactive({ info: { id: userId, avatar: '' }, get hasDefaultAvatar() { return !this.info.avatar; } });
  const chat = reactive({ currentWorldId: worldId, curChannel: { id: 'channel-a' },
    worldDetailMap: { [worldId]: { memberRole: 'member' } }, isObserver: false,
    observerMode: false, observerWorldId: '', channelIdentities: { 'channel-a': [] },
    loadChannelIdentities: async () => {},
  });
  const api = vm.runInNewContext(compile([...declarations, ...effects.map(node => node.getText(ast))].join('\n') + `
    ({ check: checkAvatarPromptOnMount, skip: handleAvatarPromptSkip, setup: handleAvatarPromptSetup,
       open: handleOpenAvatarPrompt, sync: maybePromptIdentitySync, visible: avatarPromptVisible });`), {
    ...exports, user, chat, ref, watch: (...args) => { const stop = watch(...args); stops.push(stop); return stop; },
    onBeforeUnmount: fn => cleanup.push(fn), nextTick: async () => {}, isPrivateChatChannel: () => false,
    identitySyncDialogVisible: ref(false), chatEvent: { emit: name => events.push(name) },
    setTimeout: fn => { timers.set(++timerId, fn); return timerId; }, clearTimeout: id => timers.delete(id),
    dialog: { warning: options => {
      const prompt = { ...options, destroyed: false, destroy() { this.destroyed = true; } };
      dialogs.push(prompt);
      return prompt;
    } },
    openIdentityManager: async () => events.push('manager'),
    openIdentitySyncDialog: async () => events.push('sync'), message: { error() {} }, console,
  });
  return { api, user, chat, dialogs, events, storage, preferences: exports,
    flush() { const pending = [...timers.values()]; timers.clear(); pending.forEach(fn => fn()); },
    dispose() { cleanup.forEach(fn => fn()); stops.forEach(stop => stop()); },
  };
}

test('avatar skip survives reload, remains user-scoped and allows manual opening', () => {
  const h = harness(); h.api.check(); h.flush(); assert.equal(h.api.visible.value, true);
  h.api.skip(); h.dispose();
  const reload = harness(h.storage); reload.api.check(); reload.flush();
  assert.equal(reload.api.visible.value, false);
  reload.api.open(); assert.equal(reload.api.visible.value, true);
  const other = harness(h.storage, { userId: 'user-b' }); other.api.check(); other.flush();
  assert.equal(other.api.visible.value, true);
  reload.dispose(); other.dispose();
});

test('setup acknowledges onboarding without depending on avatar upload', () => {
  const h = harness(); h.api.check(); h.flush(); h.api.setup();
  assert.deepEqual(h.events, ['open-user-profile']); h.dispose();
  const reload = harness(h.storage); reload.api.check(); reload.flush();
  assert.equal(reload.api.visible.value, false); reload.dispose();
});

test('storage failure falls back to memory and empty scopes are never persisted', () => {
  const h = harness(undefined, { blocked: true }); h.api.check(); h.flush(); h.api.skip();
  h.api.check(); h.flush(); assert.equal(h.api.visible.value, false);
  h.preferences.dismissPrompt('avatar', '');
  h.preferences.dismissPrompt('identitySync', 'user-a', '');
  assert.equal(h.preferences.isPromptDismissed('avatar', ''), false);
  assert.equal(h.preferences.isPromptDismissed('identitySync', 'user-a', ''), false);
  h.dispose();
});

test('avatar timer cannot show after observation, upload or unmount', () => {
  for (const change of [h => { h.chat.isObserver = true; }, h => { h.user.info.avatar = 'id:saved'; }, h => h.dispose()]) {
    const h = harness(); h.api.check(); change(h); h.flush();
    assert.equal(h.api.visible.value, false); h.dispose();
  }
});

test('sync decline survives reload but does not suppress another user or world', async () => {
  const h = harness(); const pending = h.api.sync(); await tick();
  assert.equal(h.dialogs.length, 1); h.dialogs[0].onNegativeClick(); await pending; h.dispose();
  const reload = harness(h.storage); await reload.api.sync(); assert.equal(reload.dialogs.length, 0); reload.dispose();
  for (const scope of [{ userId: 'user-b' }, { worldId: 'world-b' }]) {
    const other = harness(h.storage, scope); const pending = other.api.sync(); await tick();
    assert.equal(other.dialogs.length, 1); other.dialogs[0].onNegativeClick(); await pending; other.dispose();
  }
});

test('sync serializes checks and discards stale loads and dialogs', async () => {
  const h = harness(); let resolveLoad;
  h.chat.loadChannelIdentities = () => new Promise(resolve => { resolveLoad = resolve; });
  const pending = h.api.sync(); await tick(); await h.api.sync();
  h.chat.curChannel = { id: 'channel-b' }; resolveLoad(); await pending;
  assert.equal(h.dialogs.length, 0);
  h.chat.loadChannelIdentities = async () => {};
  const next = h.api.sync(); await tick(); assert.equal(h.dialogs.length, 1);
  h.chat.currentWorldId = 'world-b'; await next;
  assert.equal(h.dialogs[0].destroyed, true);
  assert.equal(h.preferences.isPromptDismissed('identitySync', 'user-a', 'world-a'), false);
  h.dispose();
});

test('accepting sync acknowledges onboarding and opens the manual workflow', async () => {
  const h = harness(); const pending = h.api.sync(); await tick();
  h.dialogs[0].onPositiveClick(); await pending;
  assert.deepEqual(h.events, ['manager', 'sync']); h.dispose();
  const reload = harness(h.storage); await reload.api.sync();
  assert.equal(reload.dialogs.length, 0); reload.dispose();
});

test('profile avatar upload reports server failure and ignores responses after account change', async () => {
  const component = read('../src/views/components/user-profile.vue');
  const script = component.match(/<script[^>]*setup[^>]*>([\s\S]*?)<\/script>/)[1];
  const parsed = ts.createSourceFile('profile.tsx', script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const statement = parsed.statements.find(node => ts.isVariableStatement(node)
    && node.declarationList.declarations.some(entry => entry.name.getText(parsed) === 'handleEditorSave'));
  assert.ok(statement);
  for (const switchAccount of [false, true]) {
    const notices = [], user = { info: { id: 'user-a', avatar: 'id:old' }, token: 'test-token' };
    let finish;
    const run = vm.runInNewContext(compile(statement.getText(parsed) + '\nhandleEditorSave;'), {
      user, profileDisposed: false, api: { post: () => new Promise((resolve, reject) => { finish = switchAccount ? resolve : reject; }) },
      FormData: class { append() {} }, showEditor: ref(true), avatarFile: ref({}),
      message: { success: text => notices.push(['success', text]), error: text => notices.push(['error', text]) },
      isAxiosError: error => Boolean(error.response), console: { error() {} },
    });
    const pending = run({ name: 'avatar.png' });
    if (switchAccount) {
      user.info = { id: 'user-b', avatar: 'id:other-user' };
      finish({ status: 200, data: { ids: ['uploaded'] } });
    } else {
      finish({ response: { data: { message: '头像保存失败，请重试' } } });
    }
    await pending;
    assert.equal(user.info.avatar, switchAccount ? 'id:other-user' : 'id:old');
    assert.deepEqual(notices, switchAccount ? [] : [['error', '头像保存失败，请重试']]);
  }
});

test('modal dismissal routes mask, Escape and close-button updates through skip', () => {
  const component = read('../src/components/AvatarSetupPrompt.vue');
  const script = component.match(/<script setup[^>]*>([\s\S]*?)<\/script>/)[1];
  const parsed = ts.createSourceFile('prompt.ts', script, ts.ScriptTarget.Latest, true);
  const statements = parsed.statements.filter(node => ts.isVariableStatement(node)
    && node.declarationList.declarations.some(entry => ['showModal', 'handleSetup', 'handleSkip'].includes(entry.name.getText(parsed))));
  const events = [];
  const api = vm.runInNewContext(compile(statements.map(node => node.getText(parsed)).join('\n')
    + '\n({showModal, handleSetup, handleSkip});'), {
    computed: options => options, props: { show: true }, emit: (...args) => events.push(args),
  });
  api.showModal.set(false); assert.deepEqual(events, [['skip'], ['update:show', false]]);
  events.length = 0; api.handleSetup(); assert.deepEqual(events, [['setup'], ['update:show', false]]);
  events.length = 0; api.handleSkip(); assert.deepEqual(events, [['skip'], ['update:show', false]]);
});
