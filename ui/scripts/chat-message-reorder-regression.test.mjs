import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

const source = readFileSync(new URL('../src/views/chat/chat.vue', import.meta.url), 'utf8').match(/<script setup[^>]*>([\s\S]*?)<\/script>/)[1];
const ast = ts.createSourceFile('chat.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const declaration = (name) => {
  let statement;
  const visit = (item) => {
    if (ts.isVariableStatement(item)
      && item.declarationList.declarations.some((entry) => entry.name.getText(ast) === name)) statement = item;
    else ts.forEachChild(item, visit);
  };
  visit(ast);
  assert.ok(statement, `missing declaration: ${name}`);
  return statement.getText(ast);
};
const compile = (text) => ts.transpileModule(text, {
  compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
}).outputText;
const messages = () => ['a', 'b', 'c', 'd'].map((id, index) => ({ id, displayOrder: (index + 1) * 1024, createdAt: index + 1 }));
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((ok, no) => { resolve = ok; reject = no; });
  return { promise, resolve, reject };
};
const names = ['compareByDisplayOrder', 'sortRowsByDisplayOrder', 'getMessageDisplayOrderValue', 'deriveLocalDisplayOrder',
  'restoreMessageDisplayOrder', 'captureMessageReadingPosition', 'displayOrderRevision', 'displayOrderRefresh', 'displayOrderDirty',
  'invalidateDisplayOrderRefresh', 'refreshLoadedDisplayOrders', 'applyReorderPayload', 'finalizeDrag',
  'latestMessagesFetchEpoch', 'latestMessagesRefetchQueued', 'scheduleLatestMessagesRefetch', 'fetchLatestMessages',
  'upsertMessage', 'handleMessageUpdated'];
const harness = () => {
  const errors = [];
  const state = {
    rows: { value: messages() }, pinnedRows: { value: [] },
    chat: { curChannel: { id: 'channel' }, messageReorder: async () => undefined },
    dragState: { activeId: 'b', overId: 'd', position: 'after', clientOpId: 'op' },
    localReorderOps: new Set(), document: { body: { style: {} } },
    detachDragListeners() {}, stopAutoScroll() {}, clearGhost() {},
    resetDragState() {},
    messagesListRef: { value: null }, messageRowRefs: new Map(),
    messageWindow: { hasReachedStart: false }, searchBrowseSession: {},
    updateWindowAnchorsFromRows() {}, isSearchBrowseActive: () => false,
    buildMessageCursor: (item) => `${item.id}:${item.displayOrder}`,
    message: { error: (text) => errors.push(text) }, errors,
    chatEvent: { emit() {} }, nextTick: (fn) => Promise.resolve().then(fn),
    nanoid: () => 'op', console,
    messageFilterSignature: { value: '' }, detachedRealtimeRevision: 0, detachedRealtimeMessageKeys: new Set(),
    resetWindowState() {}, resetTypingPreview() {}, INITIAL_MESSAGE_LOAD_LIMIT: 30,
    buildMessageFilterOptions: () => ({}), normalizeMessageList: (items) => items.map((item) => ({ ...item })),
    normalizeMessageShape: (item) => ({ ...item }), validateMessageInsertTarget() {}, applyCursorUpdate() {},
    computeAfterCursorFromRows() {}, scrollToBottom() {}, showButton: { value: false },
    autoFillIfNeeded: async () => {}, tryAutoRestoreSessionDraft() {},
    upsertPinnedMessage() {}, removePinnedMessage() {}, removeTypingPreview() {},
  };
  Object.defineProperty(state, 'visibleRows', { get: () => state.rows });
  state.api = vm.runInNewContext(compile(names.map(declaration).join('\n') + '\n({' + names.join(',') + '});'), state);
  return state;
};
const ids = (state) => Array.from(state.rows.value, (item) => item.id).join('');

test('an empty initial load retries when its response predates a reset', async () => {
  const state = harness();
  state.rows.value = [];
  const first = deferred();
  const retry = deferred();
  let requests = 0;
  state.chat.messageList = () => ++requests === 1 ? first.promise : retry.promise;
  const loading = state.api.fetchLatestMessages();
  state.api.applyReorderPayload({ reset: true });
  await state.api.refreshLoadedDisplayOrders();
  first.resolve({ data: messages().map((item) => ({ ...item, displayOrder: item.displayOrder + 1768754295000 })) });
  assert.equal(await loading, false);
  assert.equal(requests, 2);
  assert.equal(state.rows.value.length, 0, 'stale rows must never be rendered');
  retry.resolve({ data: messages().map((item) => ({ ...item, displayOrder: item.id === 'd' ? 1536 : item.displayOrder })) });
  await new Promise(setImmediate);
  assert.equal(ids(state), 'adbc');
  assert.equal(state.messageWindow.loadingLatest, false);
  assert.equal(requests, 2, 'the retry must not loop');
});

for (const failedFirst of [false, true]) {
  test(`an edit ${failedFirst ? 'after a failed' : 'during a'} reset preserves row order until synchronization`, async () => {
    const state = harness();
    state.rows.value.forEach((item) => { item.displayOrder += 1768754295000; });
    const read = deferred();
    let requests = 0;
    state.chat.messageDisplayOrders = () => {
      requests++;
      if (failedFirst && requests === 1) return Promise.reject(new Error('offline'));
      return read.promise;
    };
    const initial = state.api.refreshLoadedDisplayOrders();
    if (failedFirst) await initial;
    state.api.handleMessageUpdated({ channel: { id: 'channel' }, message: { id: 'c', displayOrder: 3072, content: 'edited' } });
    assert.equal(ids(state), 'abcd');
    assert.equal(state.rows.value.find((item) => item.id === 'c').content, 'edited');
    assert.equal(requests, failedFirst ? 2 : 1, 'edits must reuse or retry the existing synchronization');
    const syncing = state.api.refreshLoadedDisplayOrders();
    read.resolve(messages().map((item) => ({ id: item.id, display_order: item.id === 'd' ? 1536 : item.displayOrder })));
    await syncing;
    assert.equal(ids(state), 'adbc');
    assert.equal(state.rows.value.find((item) => item.id === 'c').content, 'edited');
  });
}

test('a new message during reset is synchronized before sorting existing rows', async () => {
  const state = harness();
  state.rows.value.forEach((item) => { item.displayOrder += 1768754295000; });
  const first = deferred();
  let requests = 0;
  state.chat.messageDisplayOrders = () => ++requests === 1 ? first.promise : Promise.resolve([
    ...messages().map((item) => ({ id: item.id, display_order: item.displayOrder })),
    { id: 'e', display_order: 1536 },
  ]);
  const syncing = state.api.refreshLoadedDisplayOrders();
  state.api.upsertMessage({ id: 'e', displayOrder: 1536, content: 'inserted' });
  assert.equal(ids(state), 'abcde');
  assert.equal(requests, 1);
  first.resolve(messages().map((item) => ({ id: item.id, display_order: item.displayOrder })));
  await syncing;
  assert.equal(requests, 2, 'the new ID must be included in the complete refresh');
  assert.equal(ids(state), 'aebcd');
});

test('rebalance refresh replaces every loaded order before sorting', async () => {
  const state = harness();
  state.rows.value.forEach((item) => { item.displayOrder += 1768754295000; });
  state.chat.messageDisplayOrders = async () => [
    { id: 'a', display_order: 1024 }, { id: 'b', display_order: 2048 },
    { id: 'c', display_order: 3072 }, { id: 'd', display_order: 1536 },
  ];
  await state.api.refreshLoadedDisplayOrders();
  assert.equal(ids(state), 'adbc');
  assert.equal(state.messageWindow.beforeCursor, 'a:1024');
});

test('a message loaded during refresh is included in a new batch and preserved', async () => {
  const state = harness();
  const first = deferred();
  let requests = 0;
  state.chat.messageDisplayOrders = async (_channel, requested) => {
    requests += 1;
    if (requests === 1) return first.promise;
    return requested.map((id, index) => ({ id, display_order: (index + 1) * 1024 }));
  };
  const pending = state.api.refreshLoadedDisplayOrders();
  state.rows.value.push({ id: 'e', displayOrder: 5000 });
  first.resolve([]);
  await pending;
  assert.equal(requests, 2);
  assert.equal(ids(state), 'abcde');
});

test('a normal reorder event during refresh never mixes old and new scales', async () => {
  const state = harness();
  const first = deferred();
  let requests = 0;
  state.chat.messageDisplayOrders = async () => {
    if (++requests === 1) return first.promise;
    return [{ id: 'a', display_order: 1024 }, { id: 'b', display_order: 2048 }, { id: 'c', display_order: 3072 }, { id: 'd', display_order: 1536 }];
  };
  const pending = state.api.refreshLoadedDisplayOrders();
  state.api.applyReorderPayload({ messageId: 'd', displayOrder: 1536 });
  assert.equal(ids(state), 'abcd');
  first.resolve([]);
  await pending;
  assert.equal(requests, 2);
  assert.equal(ids(state), 'adbc');
});

test('refresh response from an old channel is ignored', async () => {
  const state = harness();
  const request = deferred();
  state.chat.messageDisplayOrders = () => request.promise;
  const pending = state.api.refreshLoadedDisplayOrders();
  state.chat.curChannel = { id: 'another-channel' };
  state.rows.value = [{ id: 'a', displayOrder: 9000 }];
  request.resolve([{ id: 'a', display_order: 1024 }]);
  await pending;
  assert.equal(state.rows.value[0].displayOrder, 9000);
});

test('failed optimistic reorder restores the original scalar order', async () => {
  const state = harness();
  state.chat.messageReorder = async () => { throw new Error('offline'); };
  await state.api.finalizeDrag();
  assert.equal(ids(state), 'abcd');
  assert.equal(state.rows.value.find((item) => item.id === 'b').displayOrder, 2048);
  assert.equal(state.errors.length, 1);
});

test('leaving and reentering the same channel invalidates its old order refresh', async () => {
  const state = harness();
  const request = deferred();
  state.chat.messageDisplayOrders = () => request.promise;
  const pending = state.api.refreshLoadedDisplayOrders();
  state.api.invalidateDisplayOrderRefresh();
  state.chat.curChannel = { id: 'channel' };
  state.rows.value = [{ id: 'a', displayOrder: 9000 }];
  request.resolve([{ id: 'a', display_order: 1024 }]);
  await pending;
  assert.equal(state.rows.value[0].displayOrder, 9000);
});

test('drop uses the current collection, preserving history loaded during the drag', async () => {
  const state = harness();
  state.rows.value.unshift({ id: 'older', displayOrder: 512 });
  await state.api.finalizeDrag();
  assert.equal(ids(state), 'olderacdb');
});

test('late reorder ACK cannot change the next channel', async () => {
  const state = harness();
  const request = deferred();
  state.chat.messageReorder = () => request.promise;
  const pending = state.api.finalizeDrag();
  state.chat.curChannel = { id: 'another-channel' };
  state.rows.value = [{ id: 'b', displayOrder: 7000 }];
  request.resolve({ display_order: 999 });
  await pending;
  assert.equal(state.rows.value[0].displayOrder, 7000);
});

test('an authoritative event prevents a later request error from rolling it back', async () => {
  const state = harness();
  const request = deferred();
  state.chat.messageReorder = () => request.promise;
  const pending = state.api.finalizeDrag();
  state.localReorderOps.delete('op');
  request.reject(new Error('ACK lost'));
  await pending;
  assert.equal(ids(state), 'acdb');
  assert.equal(state.errors.length, 0);
});

test('a rebalance event and its ACK share one order refresh', async () => {
  const state = harness();
  const ack = deferred();
  const read = deferred();
  let requests = 0;
  state.chat.messageReorder = () => ack.promise;
  state.chat.messageDisplayOrders = () => { requests += 1; return read.promise; };
  const pending = state.api.finalizeDrag();
  state.api.applyReorderPayload({ reset: true, clientOpId: 'op' });
  state.localReorderOps.delete('op');
  const refreshing = state.api.refreshLoadedDisplayOrders();
  read.resolve(messages().map((item) => ({ id: item.id, display_order: item.id === 'b' ? 4608 : item.displayOrder })));
  await refreshing;
  ack.resolve({ reset: true, display_order: 4608 });
  await pending;
  assert.equal(requests, 1);
  assert.equal(ids(state), 'acdb');
});

test('a failed full refresh retries before applying any partial order', async () => {
  const state = harness();
  state.rows.value.forEach((item) => { item.displayOrder += 1768754295000; });
  state.chat.messageDisplayOrders = async () => { throw new Error('offline'); };
  await state.api.refreshLoadedDisplayOrders();
  assert.equal(state.errors.length, 1);
  const read = deferred();
  state.chat.messageDisplayOrders = () => read.promise;
  state.api.applyReorderPayload({ messageId: 'd', displayOrder: 1536 });
  assert.equal(ids(state), 'abcd');
  read.resolve([{ id: 'a', display_order: 1024 }, { id: 'b', display_order: 2048 }, { id: 'c', display_order: 3072 }, { id: 'd', display_order: 1536 }]);
  await state.api.refreshLoadedDisplayOrders();
  assert.equal(ids(state), 'adbc');
});

test('leaving and reentering the same channel invalidates its old reorder ACK', async () => {
  const state = harness();
  const ack = deferred();
  state.chat.messageReorder = () => ack.promise;
  const pending = state.api.finalizeDrag();
  state.api.invalidateDisplayOrderRefresh();
  state.chat.curChannel = { id: 'channel' };
  state.rows.value = [{ id: 'b', displayOrder: 9000 }];
  ack.resolve({ display_order: 999 });
  await pending;
  assert.equal(state.rows.value[0].displayOrder, 9000);
});

test('a failed ACK after rebalance cannot restore an obsolete order scale', async () => {
  const state = harness();
  const ack = deferred();
  state.rows.value.forEach((item) => { item.displayOrder += 1768754295000; });
  state.chat.messageReorder = () => ack.promise;
  state.chat.messageDisplayOrders = async () => messages().map((item) => ({ id: item.id, display_order: item.id === 'b' ? 4608 : item.displayOrder }));
  const pending = state.api.finalizeDrag();
  await state.api.refreshLoadedDisplayOrders();
  ack.reject(new Error('ACK lost'));
  await pending;
  await state.api.refreshLoadedDisplayOrders();
  assert.equal(ids(state), 'acdb');
  assert.equal(state.rows.value.find((item) => item.id === 'b').displayOrder, 4608);
});

test('pointerup flushes its final coordinate before committing the drop', () => {
  const calls = [];
  const dragState = { pointerId: 1, pendingClientY: 10, dragRafId: 5 };
  const onUp = vm.runInNewContext(compile(declaration('onDragPointerUp') + '\nonDragPointerUp;'), {
    dragState,
    cancelAnimationFrame: (id) => calls.push(`cancel:${id}`),
    processDragFrame: () => calls.push(`frame:${dragState.pendingClientY}`),
    finalizeDrag: () => calls.push('drop'),
  });
  onUp({ pointerId: 2, clientY: 500, preventDefault() {} });
  assert.equal(calls.length, 0);
  onUp({ pointerId: 1, clientY: 500, preventDefault() {} });
  assert.deepEqual(calls, ['cancel:5', 'frame:500', 'drop']);
});

test('scroll anchor restores the same message offset, and ignores channel changes', () => {
  const state = harness();
  let top = 60;
  const container = { scrollTop: 100, getBoundingClientRect: () => ({ top: 0, bottom: 800 }) };
  state.messagesListRef.value = container;
  state.messageRowRefs.set('a', { getBoundingClientRect: () => ({ top, bottom: top + 50 }) });
  const restore = state.api.captureMessageReadingPosition();
  top = 85;
  restore();
  assert.equal(container.scrollTop, 125);
  state.chat.curChannel = { id: 'another-channel' };
  top = 200;
  restore();
  assert.equal(container.scrollTop, 125);
});
