import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import vm from 'node:vm';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const vue = require('vue');
const pinia = require('pinia');
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
function load(path, imports) {
  const module = { exports: {} };
  const source = ts.transpileModule(read(path), {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText;
  vm.runInNewContext(source, {
    module, exports: module.exports, console, setTimeout, clearTimeout,
    require(name) {
      if (name === 'vue') return vue;
      if (name === 'pinia') return pinia;
      if (name in imports) return imports[name];
      throw new Error(`Unexpected import: ${name}`);
    },
  });
  return module.exports;
}
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
};
const settle = () => new Promise(resolve => setImmediate(resolve));
const item = (revision, identityId = 'identity', userId = 'user') => ({
  channelId: 'A', identityId, userId, sourceType: 'client', sourceCardId: 'card-A',
  data: { identity: { id: identityId, userId }, card: { name: 'Hero', sheetType: 'coc', attrs: { hp: revision } } },
  serverRevision: revision, contentHash: `hash-${revision}`, lastSeenAt: revision,
});

function setup(t) {
  const instance = pinia.createPinia();
  pinia.setActivePinia(instance);
  const listeners = new Map();
  const chatEvent = { on(name, callback) {
    listeners.set(name, [...(listeners.get(name) || []), callback]);
  } };
  const emit = (name, payload) => (listeners.get(name) || []).forEach(callback => callback(payload));
  const user = vue.reactive({ info: { id: 'user' } });
  const display = vue.reactive({ settings: {
    characterCardSnapshotUploadEnabled: true, onlineCharacterCardsEnabled: true, characterCardBadgeEnabled: true,
  } });
  const chat = vue.reactive({
    curChannel: { id: 'A', characterApiEnabled: true }, currentWorldId: 'world',
    ensureConnectionReady: async () => {},
    findChannelById: id => ({ id, characterApiEnabled: true }),
    getActiveIdentity: () => ({ id: 'identity', userId: user.info.id, displayName: 'Hero' }),
    getIdentityIcOocMode: () => 'ic', getActiveIdentityVariant: () => null,
  });
  const snapshot = load('../src/stores/channelCharacterSnapshot.ts', {
    './chat': { useChatStore: () => chat, chatEvent }, './user': { useUserStore: () => user },
  }).useChannelCharacterSnapshotStore();
  const template = {
    ensureTemplatesLoaded: async () => {}, ensureBindingsLoaded: async () => {},
    getBinding: () => null, resolveCardTemplate: () => '',
  };
  const cards = load('../src/stores/characterCard.ts', {
    './chat': { useChatStore: () => chat, chatEvent },
    './user': { useUserStore: () => user }, './display': { useDisplayStore: () => display },
    './characterCardTemplate': { useCharacterCardTemplateStore: () => template },
    './characterSheet': { useCharacterSheetStore: () => ({}) },
    './characterCardAvatar': { useCharacterCardAvatarStore: () => ({
      ensureBindingsLoaded: async () => {}, resolveCardAvatar: () => '',
    }) },
    './channelCharacterSnapshot': { useChannelCharacterSnapshotStore: () => snapshot },
    './characterCardDeleteCleanup': {}, '@/utils/characterCardTemplate': {},
    '@/utils/characterCardNarratorSettings': {
      normalizeCharacterCardNarratorSettings: () => ({}), isCharacterCardNarratorIdentity: () => false,
    },
    '@/utils/botNicknameSync': {}, '@/utils/characterStatMutation': {}, '@/utils/characterStatDisplay': {},
  }).useCharacterCardStore();
  const calls = [], handlers = {};
  let revision = 1000;
  const state = { items: [item(100)], active: { ok: true, id: 'card-A', name: 'Hero', type: 'coc', data: { hp: 9 } } };
  chat.sendAPI = async (api, payload) => {
    calls.push({ api, payload });
    if (handlers[api]) return handlers[api](payload);
    if (api === 'character.get') return { data: state.active };
    if (api === 'character.snapshot.list') return { data: { channelId: payload.channelId, items: state.items } };
    if (api === 'character.snapshot.upsert') {
      const value = { ...item(++revision, payload.identityId), channelId: payload.channelId, data: payload.data };
      state.items = [value];
      return { data: { item: value, changed: true } };
    }
    if (api === 'character.snapshot.clear') {
      const value = item(++revision, payload.identityId);
      state.items = [];
      return { data: { item: value, changed: true } };
    }
    if (api === 'character.list') return { data: { ok: true, list: [] } };
    if (api === 'character.snapshot.settings.get' || api === 'character.snapshot.preference.get') return { data: { channelId: payload.channelId, userId: user.info.id } };
    throw new Error(`Unexpected API: ${api}`);
  };
  t.after(() => { cards.$dispose(); snapshot.$dispose(); });
  const update = (value, action = 'update') => emit('character-snapshot-updated', { characterSnapshot: { item: value, action } });
  const probe = hash => emit('character-snapshot-probe', {
    characterSnapshotProbe: { channelId: 'A', items: [{ userId: 'user', contentHash: hash }] },
  });
  const writes = () => calls.filter(call => ['character.snapshot.upsert', 'character.snapshot.clear'].includes(call.api));
  return { chat, user, display, cards, snapshot, handlers, calls, state, update, emit, probe, writes };
}

test('pending and failed loads preserve the published card; successful retry publishes automatically', async t => {
  const s = setup(t), pending = deferred();
  await s.snapshot.refreshChannel('A');
  s.handlers['character.get'] = () => pending.promise;
  const loading = s.cards.getActiveCard('A');
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes().length, 0);
  assert.equal(s.snapshot.getSnapshot('A', 'identity').data.card.name, 'Hero');
  pending.resolve({ data: { ok: false, error: 'TIMEOUT' } });
  await loading;
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.cards.activeCardLoadState.A, 'error');
  assert.equal(s.writes().length, 0);
  delete s.handlers['character.get'];
  s.probe('hash-100');
  await settle();
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.cards.activeCardLoadState.A, 'ready');
  assert.equal(s.writes().length, 1);
  assert.equal(s.writes()[0].payload.cardAction, 'replace');
  assert.equal(s.writes()[0].payload.data.card.attrs.hp, 9);
});

test('successful no-card response clears stale active data; sharing switches retain their separate meanings', async t => {
  const s = setup(t);
  await s.cards.getActiveCard('A');
  await s.snapshot.syncLocalSnapshot('A');
  s.state.active = { ok: true, name: '', type: '', data: {} };
  await s.cards.getActiveCard('A');
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.cards.activeCards.A, undefined);
  assert.equal(s.writes().at(-1).payload.cardAction, 'clear');
  s.state.active = { ok: true, id: 'card-A', name: 'Hero', data: { hp: 9 } };
  await s.cards.getActiveCard('A');
  await s.snapshot.syncLocalSnapshot('A');
  s.display.settings.onlineCharacterCardsEnabled = false;
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes().at(-1).payload.cardAction, 'clear');
  assert.equal(s.writes().at(-1).payload.data.badgeEnabled, true);
  s.display.settings.characterCardSnapshotUploadEnabled = false;
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes().at(-1).api, 'character.snapshot.clear');
  assert.equal(s.snapshot.getSnapshot('A', 'identity'), null);
});

test('missing identity and failed loads do not masquerade as clear; explicit opt-out works while loading', async t => {
  const s = setup(t), pending = deferred();
  await s.snapshot.refreshChannel('A');
  s.handlers['character.get'] = () => pending.promise;
  const loading = s.cards.getActiveCard('A');
  s.chat.getActiveIdentity = () => null;
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes().length, 0);
  s.display.settings.characterCardSnapshotUploadEnabled = false;
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes()[0].api, 'character.snapshot.clear');
  pending.resolve({ data: s.state.active });
  await loading;
});

test('stale list cannot overwrite an incremental update or revive a clear; list events are ignored', async t => {
  const s = setup(t), pending = deferred();
  let requests = 0;
  s.handlers['character.snapshot.list'] = () => ++requests === 1
    ? pending.promise : { data: { channelId: 'A', items: [item(200)] } };
  const refresh = s.snapshot.refreshChannel('A');
  await settle();
  s.update(item(200));
  pending.resolve({ data: { channelId: 'A', items: [item(100)] } });
  await refresh;
  assert.equal(s.snapshot.getSnapshot('A', 'identity').serverRevision, 200);
  assert.equal(requests, 1);
  s.update(item(201), 'clear');
  s.emit('character-snapshot-list', { characterSnapshotList: { channelId: 'A', items: [item(100)] } });
  s.update(item(200));
  assert.equal(s.snapshot.getSnapshot('A', 'identity'), null);
  s.state.items = [item(100)];
  delete s.handlers['character.snapshot.list'];
  await s.snapshot.refreshChannel('A');
  assert.equal(s.snapshot.getSnapshot('A', 'identity'), null);
});

test('old clear cannot remove a newer item and an older list revision cannot roll it back', async t => {
  const s = setup(t);
  s.update(item(200));
  s.update(item(199), 'clear');
  await s.snapshot.refreshChannel('A');
  assert.equal(s.snapshot.getSnapshot('A', 'identity').serverRevision, 200);
});

test('only the newest list request controls data and loading', async t => {
  const s = setup(t), old = deferred(), fresh = deferred();
  let n = 0;
  s.handlers['character.snapshot.list'] = () => ++n === 1 ? old.promise : fresh.promise;
  const first = s.snapshot.refreshChannel('A');
  await settle();
  const second = s.snapshot.refreshChannel('A');
  await settle();
  old.resolve({ data: { channelId: 'A', items: [] } });
  await first;
  assert.equal(s.snapshot.loadingByChannel.A, true);
  fresh.resolve({ data: { channelId: 'A', items: [item(300)] } });
  await second;
  assert.equal(s.snapshot.getSnapshot('A', 'identity').serverRevision, 300);
  assert.equal(s.snapshot.loadingByChannel.A, false);
});

test('late upsert response cannot roll back a newer event', async t => {
  const s = setup(t), pending = deferred();
  s.snapshot.setLocalSnapshotProvider(() => ({ identityId: 'identity', data: item(100).data }));
  s.handlers['character.snapshot.upsert'] = () => pending.promise;
  const syncing = s.snapshot.syncLocalSnapshot('A');
  await settle();
  s.update(item(200));
  pending.resolve({ data: { item: item(100) } });
  await syncing;
  assert.equal(s.snapshot.getSnapshot('A', 'identity').serverRevision, 200);
});

test('late clear response cannot erase a newer reactivated snapshot', async t => {
  const s = setup(t), pending = deferred();
  await s.snapshot.refreshChannel('A');
  s.snapshot.setLocalSnapshotProvider(() => null);
  s.handlers['character.snapshot.clear'] = () => pending.promise;
  const clearing = s.snapshot.syncLocalSnapshot('A');
  await settle();
  s.update(item(300));
  pending.resolve({ data: { item: item(200) } });
  await clearing;
  assert.equal(s.snapshot.getSnapshot('A', 'identity').serverRevision, 300);
});

test('list merges unaffected members while protecting concurrent updates and clears', async t => {
  const s = setup(t), first = deferred(), second = deferred();
  let n = 0;
  s.handlers['character.snapshot.list'] = () => ++n === 1 ? first.promise : second.promise;
  const refresh = s.snapshot.refreshChannel('A');
  await settle();
  s.update(item(200));
  first.resolve({ data: { channelId: 'A', items: [item(100), item(150, 'friend', 'other')] } });
  await refresh;
  assert.equal(s.snapshot.getSnapshot('A', 'identity').serverRevision, 200);
  assert.equal(s.snapshot.getSnapshot('A', 'friend').serverRevision, 150);
  const clearing = s.snapshot.refreshChannel('A');
  await settle();
  s.update(item(300), 'clear');
  second.resolve({ data: { channelId: 'A', items: [item(200), item(150, 'friend', 'other')] } });
  await clearing;
  assert.equal(s.snapshot.getSnapshot('A', 'identity'), null);
  assert.equal(s.snapshot.getSnapshot('A', 'friend').serverRevision, 150);
});

test('list merge preserves a concurrent identity switch without comparing revisions across identities', async t => {
  const s = setup(t), pending = deferred();
  s.update(item(100));
  s.handlers['character.snapshot.list'] = () => pending.promise;
  const refresh = s.snapshot.refreshChannel('A');
  await settle();
  s.update(item(50, 'new-identity'));
  pending.resolve({ data: { channelId: 'A', items: [item(150)] } });
  await refresh;
  assert.equal(s.snapshot.getSnapshot('A', 'identity'), null);
  assert.equal(s.snapshot.getSnapshot('A', 'new-identity').serverRevision, 50);
});

test('list reads do not acknowledge an external overwrite; probe republishes the local card', async t => {
  const s = setup(t);
  s.snapshot.setLocalSnapshotProvider(() => ({ identityId: 'identity', data: item(100).data }));
  await s.snapshot.syncLocalSnapshot('A');
  const degraded = item(2000); delete degraded.data.card;
  s.state.items = [degraded];
  await s.snapshot.refreshChannel('A');
  s.probe(degraded.contentHash);
  await settle();
  assert.equal(s.writes().length, 2);
  assert.ok(s.writes()[1].payload.data.card);
});

test('a provider result captured before a queued opt-out is discarded', async t => {
  const s = setup(t), pending = deferred();
  await s.snapshot.refreshChannel('A');
  let input = pending.promise;
  s.snapshot.setLocalSnapshotProvider(() => input);
  const first = s.snapshot.syncLocalSnapshot('A');
  await settle();
  input = null;
  const second = s.snapshot.syncLocalSnapshot('A', true);
  pending.resolve({ identityId: 'identity', data: item(100).data });
  await Promise.all([first, second]);
  assert.equal(s.writes().length, 1);
  assert.equal(s.writes()[0].api, 'character.snapshot.clear');
});

for (const fail of [false, true]) {
  test(`in-flight upload ${fail ? 'failure' : 'success'} is followed by queued clear without concurrent writes`, async t => {
    const s = setup(t), pending = deferred();
    await s.snapshot.refreshChannel('A');
    let input = { identityId: 'identity', data: item(100).data };
    s.snapshot.setLocalSnapshotProvider(() => input);
    s.handlers['character.snapshot.upsert'] = () => pending.promise;
    const first = s.snapshot.syncLocalSnapshot('A');
    await settle();
    input = null;
    const second = s.snapshot.syncLocalSnapshot('A');
    assert.equal(s.writes().length, 1);
    if (fail) pending.reject(new Error('upload failed'));
    else pending.resolve({ data: { item: item(200) } });
    await Promise.all([first, second]);
    assert.equal(s.writes().at(-1).api, 'character.snapshot.clear');
    assert.equal(s.snapshot.getSnapshot('A', 'identity'), null);
  });
}

test('opt-out queued at any promise boundary is drained before synchronization becomes idle', async t => {
  for (let delay = 0; delay < 12; delay += 1) {
    const s = setup(t);
    await s.snapshot.refreshChannel('A');
    let input = { identityId: 'identity', data: item(100).data };
    s.snapshot.setLocalSnapshotProvider(() => input);
    const first = s.snapshot.syncLocalSnapshot('A');
    const last = (async () => {
      for (let tick = 0; tick < delay; tick += 1) await Promise.resolve();
      input = null;
      await s.snapshot.syncLocalSnapshot('A');
    })();
    await Promise.all([first, last]);
    assert.equal(s.snapshot.getSnapshot('A', 'identity'), null, `lost clear at boundary ${delay}`);
  }
});

test('card lists are isolated by channel and same-channel stale responses are ignored', async t => {
  const s = setup(t), a = deferred(), b = deferred();
  s.handlers['character.list'] = payload => payload.group_id === 'A' ? a.promise : b.promise;
  const first = s.cards.loadCardList('A');
  await settle();
  s.chat.curChannel = { id: 'B', characterApiEnabled: true };
  const second = s.cards.loadCardList('B');
  b.resolve({ data: { ok: true, list: [{ id: 'B', name: 'B card' }] } });
  await second;
  a.resolve({ data: { ok: true, list: [{ id: 'A', name: 'A card' }] } });
  await first;
  assert.equal(s.cards.cardList[0].id, 'B');
  assert.equal(s.cards.getCardsByChannel('A')[0].id, 'A');
  const old = deferred();
  let n = 0;
  s.handlers['character.list'] = () => ++n === 1 ? old.promise : { data: { ok: true, list: [{ id: 'B-new', name: 'New' }] } };
  const stale = s.cards.loadCardList('B');
  await settle();
  await s.cards.loadCardList('B');
  old.resolve({ data: { ok: true, list: [{ id: 'B-old', name: 'Old' }] } });
  await stale;
  assert.equal(s.cards.cardList[0].id, 'B-new');
});

test('active card requests only accept the latest response', async t => {
  const s = setup(t), old = deferred();
  let n = 0;
  s.handlers['character.get'] = () => ++n === 1 ? old.promise : { data: { ...s.state.active, name: 'New' } };
  const first = s.cards.getActiveCard('A');
  await settle();
  await s.cards.getActiveCard('A');
  old.resolve({ data: { ...s.state.active, name: 'Old' } });
  await first;
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.cards.activeCards.A.name, 'New');
  assert.equal(s.writes().at(-1).payload.data.card.name, 'New');
});

test('reconnect preserves the published card until the active card is confirmed again', async t => {
  const s = setup(t), pending = deferred();
  await s.cards.getActiveCard('A');
  await s.snapshot.syncLocalSnapshot('A');
  const before = s.writes().length;
  s.handlers['character.get'] = () => pending.promise;
  s.emit('connected');
  await settle();
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes().length, before);
  assert.equal(s.snapshot.getSnapshot('A', 'identity').data.card.name, 'Hero');
  pending.resolve({ data: { ...s.state.active, data: { hp: 22 } } });
  await settle();
  await s.snapshot.syncLocalSnapshot('A');
  assert.equal(s.writes().at(-1).payload.data.card.attrs.hp, 22);
});

test('account switch invalidates pending lists, active cards and snapshot writes', async t => {
  const s = setup(t), pending = deferred();
  s.snapshot.setLocalSnapshotProvider(() => ({ identityId: 'identity', data: item(100).data }));
  for (const api of ['character.list', 'character.get', 'character.snapshot.list', 'character.snapshot.upsert']) s.handlers[api] = () => pending.promise;
  const requests = [s.cards.loadCardList('A'), s.cards.getActiveCard('A'), s.snapshot.refreshChannel('A'), s.snapshot.syncLocalSnapshot('A')];
  await settle();
  s.user.info.id = 'other-user';
  await vue.nextTick();
  pending.resolve({ data: { ...s.state.active, list: [{ id: 'old', name: 'Old' }], channelId: 'A', items: [item(100)], item: item(100) } });
  await Promise.all(requests);
  assert.equal(s.cards.cardList.length, 0);
  assert.equal(s.cards.activeCards.A, undefined);
  assert.equal(s.snapshot.getChannelItems('A').length, 0);
});

test('theater reconciliation removes a card after explicit card clear', () => {
  const source = read('../src/views/theater/stage/TheaterCharacterStatsOverlay.vue');
  const start = source.indexOf('const reconcileCharacters = () => {');
  const end = source.indexOf('\nconst characters =', start);
  const stable = { value: { user: { item: item(100) } } };
  const run = loadReconcile(source.slice(start, end), stable);
  run();
  assert.equal(Object.keys(stable.value).length, 0);
});

function loadReconcile(source, stableCharactersByUser) {
  const context = {
    overlayReady: { value: true }, stableCharactersByUser,
    snapshotItems: { value: [{ ...item(200), data: { identity: { id: 'identity', userId: 'user' } } }] },
    resolvedCharacters: { value: [] }, ensureSlotOrder() {},
  };
  return vm.runInNewContext(`${ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText}\nreconcileCharacters`, context);
}
