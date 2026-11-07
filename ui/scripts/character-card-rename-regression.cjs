const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');
const pinia = require('pinia');
const { parse } = require('@vue/compiler-sfc');
const { renderToString } = require('@vue/server-renderer');

const read = path => readFileSync(require.resolve(path), 'utf8');
function load(source, imports = {}, globals = {}) {
  const loaded = { exports: {} };
  const compiled = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText;
  vm.runInNewContext(compiled, {
    module: loaded, exports: loaded.exports, console,
    require(name) {
      if (name in imports) return imports[name];
      if (name === 'vue') return vue;
      if (name === 'pinia') return pinia;
      throw new Error(`Unexpected import: ${name}`);
    },
    ...globals,
  });
  return loaded.exports;
}

function setup() {
  pinia.setActivePinia(pinia.createPinia());
  const channel = { id: 'channel', characterApiEnabled: true, botFeatureEnabled: true };
  const chat = vue.reactive({
    curChannel: channel, channelIdentities: {},
    ensureConnectionReady: async () => {},
    findChannelById: id => id === channel.id ? channel : undefined,
  });
  const display = vue.reactive({ settings: { characterCardAutoSyncBotNickname: true } });
  const user = vue.reactive({ info: { id: 'user' } });
  const template = {
    ensureTemplatesLoaded: async () => {}, ensureBindingsLoaded: async () => {},
    resolveCardTemplate: () => '<html>template</html>',
  };
  let cardStore;
  const persisted = [];
  const sheet = load(read('../src/stores/characterSheet.ts'), {
    '../../../doc/template/sealchat-default-template-v3-coc7th.html?raw': { default: '' },
    '../../../doc/template/sealchat-shinobigami-template-v1.html?raw': { default: '' },
    '../../../doc/template/sealchat-daggerheart-template-v1.html?raw': { default: '' },
    '../../../doc/template/sealchat-triangle-agency-template-v1.html?raw': { default: '' },
    '@/utils/characterSheetType': load(read('../src/utils/characterSheetType.ts')),
    './characterCard': { useCharacterCardStore: () => cardStore },
    './characterCardTemplate': { useCharacterCardTemplateStore: () => template },
    './chat': { useChatStore: () => chat },
    './display': { useDisplayStore: () => display },
  }, {
    localStorage: { setItem: (...args) => persisted.push(args) },
    window: {}, setTimeout: fn => { fn(); return 1; }, clearTimeout() {},
  }).useCharacterSheetStore();
  const nickname = load(read('../src/utils/botNicknameSync.ts'), {
    './botCommand': load(read('../src/utils/botCommand.ts'), { '@/utils/tiptap-render': {} }),
  });
  const cardModule = load(read('../src/stores/characterCard.ts'), {
    './chat': { useChatStore: () => chat, chatEvent: { on() {} } },
    './user': { useUserStore: () => user },
    './display': { useDisplayStore: () => display },
    './characterCardTemplate': { useCharacterCardTemplateStore: () => template },
    './characterSheet': { useCharacterSheetStore: () => sheet },
    './characterCardAvatar': { useCharacterCardAvatarStore: () => ({}) },
    './channelCharacterSnapshot': { useChannelCharacterSnapshotStore: () => ({
      setLocalSnapshotProvider() {}, syncLocalSnapshot: async () => {},
    }) },
    './characterCardDeleteCleanup': {},
    '@/utils/characterCardTemplate': {},
    '@/utils/characterCardNarratorSettings': {},
    '@/utils/botNicknameSync': nickname,
    '@/utils/characterStatMutation': {},
    '@/utils/characterStatDisplay': {},
  });
  cardStore = cardModule.useCharacterCardStore();
  const server = { id: 'card', name: 'Hero', type: 'coc7', attrs: { STR: 70, nested: { HP: 12 } } };
  const api = [];
  let failSet = '';
  let failGet = '';
  chat.sendAPI = async (action, payload) => {
    api.push({ action, payload });
    if (action === 'character.set') {
      if (failSet) return { data: { ok: false, error: failSet } };
      server.name = payload.name;
      if (payload.attrs != null) Object.assign(server.attrs, payload.attrs);
      return { data: { ok: true } };
    }
    if (action === 'character.list') return { data: { ok: true, list: [
      { id: server.id, name: server.name, sheet_type: server.type },
      { id: 'other', name: 'Other', sheet_type: 'coc7' },
    ] } };
    if (action === 'character.get') {
      if (failGet) return { data: { ok: false, error: failGet } };
      return { data: { ok: true, name: server.name, type: server.type, data: server.attrs } };
    }
    throw new Error(`Unexpected API: ${action}`);
  };
  const botCalls = [];
  chat.botInteract = async (...args) => { botCalls.push(args); return { ok: true }; };
  cardStore.cardList = [
    { id: 'card', name: 'Hero', sheetType: 'coc7' },
    { id: 'other', name: 'Other', sheetType: 'coc7' },
  ];
  cardStore.activeCards.channel = { ...server };
  for (const [id, cardId, cardName, readOnly, channelId] of [
    ['main', 'card', 'Hero', false, 'channel'],
    ['second', 'card', 'Hero', false, 'another-channel'],
    ['sameName', 'other', 'Hero', false, 'channel'],
    ['snapshot', 'card', 'Hero', true, 'channel'],
  ]) {
    sheet.windows[id] = {
      id, cardId, cardName, readOnly, channelId, attrs: { localDraft: 42 },
      template: '<html>local</html>', templateId: 'template', avatarUrl: 'avatar',
      syncState: 'editing_locked', hasLocalEditsInLock: true,
    };
    sheet.activeWindowIds.push(id);
  }
  return {
    cardStore, cardModule, sheet, server, api, botCalls, chat, display, persisted,
    failSet: value => { failSet = value; }, failGet: value => { failGet = value; },
  };
}

const panel = parse(read('../src/views/chat/components/CharacterCardPanel.vue')).descriptor;
const panelScript = ts.createSourceFile('panel.ts', panel.scriptSetup.content, ts.ScriptTarget.Latest, true);
const declarations = names => panelScript.statements.filter(statement => ts.isVariableStatement(statement)
  && statement.declarationList.declarations.some(item => names.includes(item.name.getText(panelScript))))
  .map(statement => statement.getText(panelScript)).join('\n');
function panelActions(env) {
  const errors = [], warnings = [];
  const source = `${declarations([
    'renameTarget', 'renameCardName', 'renaming', 'renameModalVisible',
    'canRenameCard', 'openRenameModal', 'handleRenameCard',
  ])}\nexport { renameTarget, renameCardName, renaming, renameModalVisible, canRenameCard, openRenameModal, handleRenameCard };`;
  return { ...load(source, {}, {
    ref: vue.ref, cardStore: env.cardStore,
    resolvedChannelId: vue.computed(() => env.chat.curChannel?.id || ''),
    validateCharacterCardName: env.cardModule.validateCharacterCardName,
    message: { error: value => errors.push(value), warning: value => warnings.push(value), success() {} },
  }), errors, warnings };
}

test('only the current channel API-bound card renders the rename action', async () => {
  const env = setup();
  const actions = panelActions(env);
  const buttonSource = panel.template.content.match(/<n-button\s+v-if="canRenameCard\(card\)"[\s\S]*?<\/n-button>/)?.[0];
  assert.ok(buttonSource, 'the action must use v-if instead of a disabled placeholder');
  assert.ok(!buttonSource.includes('disabled'));
  const render = vue.compile(buttonSource);
  const rendersRename = async cardId => {
    const app = vue.createSSRApp({
      setup: () => ({ card: { id: cardId }, canRenameCard: actions.canRenameCard, Edit: null, openRenameModal() {} }),
      render,
    });
    app.component('NButton', { setup: (_, { slots }) => () => vue.h('button', slots.default?.()) });
    app.component('NIcon', { render: () => null });
    return (await renderToString(app)).includes('重命名');
  };
  assert.equal(await rendersRename('card'), true);
  for (const id of ['other', 'unbound', 'snapshot:channel:identity', 'online:user:identity']) {
    assert.equal(await rendersRename(id), false);
  }
  env.cardStore.activeCards.otherChannel = { id: 'other', name: 'Other' };
  assert.equal(await rendersRename('other'), false);
  assert.equal(env.cardStore.canRenameActiveCard('otherChannel', 'other'), false);
  delete env.cardStore.activeCards.channel.id;
  assert.equal(await rendersRename('card'), false, 'name matching does not prove a binding');
  env.cardStore.activeCards.channel.id = 'card';
  env.chat.observerMode = true;
  assert.equal(await rendersRename('card'), false);
});

test('name-only character.set preserves cardId, attrs and keyed state and refreshes names', async () => {
  const env = setup();
  env.cardStore.identityBindings.identity = 'card';
  const before = JSON.stringify(env.sheet.windows);
  await env.cardStore.renameActiveCard('channel', 'card', '  Renamed  ');
  const set = env.api.find(item => item.action === 'character.set');
  assert.deepEqual(JSON.parse(JSON.stringify(set)), {
    action: 'character.set', payload: { group_id: 'channel', user_id: 'user', name: 'Renamed' },
  });
  assert.equal(Object.hasOwn(set.payload, 'attrs'), false);
  assert.equal(env.server.id, 'card');
  assert.deepEqual(env.server.attrs, { STR: 70, nested: { HP: 12 } });
  assert.equal(env.cardStore.cardList.find(card => card.id === 'card').name, 'Renamed');
  assert.equal(env.cardStore.activeCards.channel.name, 'Renamed');
  assert.equal(env.cardStore.activeCards.channel.id, 'card');
  assert.deepEqual(env.api.map(item => item.action), ['character.get', 'character.set', 'character.list', 'character.get']);
  const expectedWindows = JSON.parse(before);
  expectedWindows.main.cardName = expectedWindows.second.cardName = 'Renamed';
  assert.deepEqual(JSON.parse(JSON.stringify(env.sheet.windows)), expectedWindows);
  assert.equal(env.cardStore.identityBindings.identity, 'card');
  assert.ok(env.persisted.length > 0, 'updated names are persisted');
  assert.equal(env.botCalls.length, 1);
  assert.deepEqual(env.botCalls[0].slice(0, 2), ['channel', '.nn Renamed']);
});

test('successful rename updates local UI state before server refresh completes', async () => {
  const env = setup();
  const sendAPI = env.chat.sendAPI;
  let releaseList;
  const listGate = new Promise(resolve => { releaseList = resolve; });
  let blockNextList = false;
  env.chat.sendAPI = async (action, payload) => {
    if (action === 'character.set') {
      const result = await sendAPI(action, payload);
      blockNextList = true;
      return result;
    }
    if (action === 'character.list' && blockNextList) {
      await listGate;
    }
    return sendAPI(action, payload);
  };

  const pending = env.cardStore.renameActiveCard('channel', 'card', 'Instant');
  while (!blockNextList) await Promise.resolve();
  await new Promise(resolve => setImmediate(resolve));

  assert.equal(env.cardStore.cardList.find(card => card.id === 'card').name, 'Instant');
  assert.equal(env.cardStore.activeCards.channel.name, 'Instant');
  assert.equal(env.sheet.windows.main.cardName, 'Instant');

  releaseList();
  await pending;
});

test('stale character.list after rename does not lose the active binding or revert the visible name', async () => {
  const env = setup();
  const sendAPI = env.chat.sendAPI;
  let renamed = false;
  env.chat.sendAPI = async (action, payload) => {
    if (action === 'character.set') {
      const result = await sendAPI(action, payload);
      renamed = true;
      return result;
    }
    if (action === 'character.list' && renamed) {
      return { data: { ok: true, list: [
        { id: 'card', name: 'Hero', sheet_type: 'coc7' },
        { id: 'other', name: 'Other', sheet_type: 'coc7' },
      ] } };
    }
    return sendAPI(action, payload);
  };

  await env.cardStore.renameActiveCard('channel', 'card', 'Stable');
  assert.equal(env.cardStore.activeCards.channel.id, 'card');
  assert.equal(env.cardStore.activeCards.channel.name, 'Stable');
  assert.equal(env.cardStore.cardList.find(card => card.id === 'card').name, 'Stable');
  assert.equal(env.cardStore.canRenameActiveCard('channel', 'card'), true);

  await env.cardStore.loadCardList('channel', { throwOnError: true });
  await env.cardStore.getActiveCard('channel', { throwOnError: true });
  assert.equal(env.cardStore.activeCards.channel.id, 'card');
  assert.equal(env.cardStore.cardList.find(card => card.id === 'card').name, 'Stable');
});

test('existing nickname preference and channel switch still gate the shared nickname sync', async () => {
  for (const enabled of [false, true]) {
    const env = setup();
    env.display.settings.characterCardAutoSyncBotNickname = enabled;
    await env.cardStore.renameActiveCard('channel', 'card', 'New');
    assert.equal(env.botCalls.length, enabled ? 1 : 0);
    if (enabled) {
      await env.cardStore.syncBotNicknameForCard('channel', 'New');
      assert.equal(env.botCalls.length, 1, 'rename uses the existing sync deduplication');
    }
  }
  const env = setup();
  env.chat.curChannel.botFeatureEnabled = false;
  await env.cardStore.renameActiveCard('channel', 'card', 'New');
  assert.equal(env.botCalls.length, 0);
});

test('API failures surface their error and never change local names or sync nicknames', async () => {
  for (const error of ['NAME_ALREADY_EXISTS', '当前BOT不支持人物卡API、未开启或未启用。']) {
    const env = setup();
    const actions = panelActions(env);
    actions.openRenameModal({ id: 'card', name: 'Hero' });
    actions.renameCardName.value = 'New';
    env.failSet(error);
    assert.equal(await actions.handleRenameCard(), false);
    assert.deepEqual(actions.errors, [error]);
    assert.equal(env.cardStore.cardList[0].name, 'Hero');
    assert.equal(env.cardStore.activeCards.channel.name, 'Hero');
    assert.equal(env.sheet.windows.main.cardName, 'Hero');
    assert.equal(env.botCalls.length, 0);
    assert.equal(actions.renameModalVisible.value, true);
    assert.equal(actions.renaming.value, false);
  }
});

test('validation is shared with creation; empty/long names, changed binding and stale dialogs cannot write', async () => {
  const env = setup();
  for (const name of ['  ', 'x'.repeat(env.cardModule.CHARACTER_CARD_NAME_MAX_LENGTH + 1)]) {
    await assert.rejects(env.cardStore.renameActiveCard('channel', 'card', name));
  }
  assert.equal(env.api.length, 0);
  assert.match(declarations(['handleCreateCard']), /validateCharacterCardName\(newCardName.value\)/);
  assert.equal((panel.template.content.match(/:maxlength="CHARACTER_CARD_NAME_MAX_LENGTH"/g) || []).length, 2);
  await assert.rejects(env.cardStore.renameActiveCard('channel', 'other', 'New'));
  env.server.name = 'Other';
  await assert.rejects(env.cardStore.renameActiveCard('channel', 'card', 'New'));
  assert.ok(env.api.every(item => item.action !== 'character.set'));
  const second = setup();
  const actions = panelActions(second);
  actions.openRenameModal({ id: 'card', name: 'Hero' });
  second.chat.curChannel = { id: 'another-channel' };
  assert.equal(await actions.handleRenameCard(), false);
  assert.equal(second.api.length, 0);
});

test('a failed fresh binding read must stop before character.set', async () => {
  const env = setup();
  env.failGet('BOT_TIMEOUT');
  await assert.rejects(env.cardStore.renameActiveCard('channel', 'card', 'New'), /BOT_TIMEOUT/);
  assert.ok(env.api.every(item => item.action !== 'character.set'));
});

test('switching channels while waiting for the set connection cannot rename the previous card', async () => {
  const env = setup();
  let connections = 0;
  env.chat.ensureConnectionReady = async () => {
    if (++connections === 2) env.chat.curChannel = { id: 'another-channel' };
  };
  await assert.rejects(env.cardStore.renameActiveCard('channel', 'card', 'New'));
  assert.ok(env.api.every(item => item.action !== 'character.set'));
  assert.equal(env.sheet.windows.main.cardName, 'Hero');
});
