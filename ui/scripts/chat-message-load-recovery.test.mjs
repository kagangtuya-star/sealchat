import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

// Run the actual initial-load statements and channel-switch declarations.
// Transport, other feature panels and DOM effects are stubbed.
const source = readFileSync(new URL('../src/views/chat/chat.vue', import.meta.url), 'utf8').match(/<script setup[^>]*>([\s\S]*?)<\/script>/)[1];
const ast = ts.createSourceFile('chat.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const declaration = name => {
  const statement = ast.statements.find(item => ts.isVariableStatement(item)
    && item.declarationList.declarations.some(entry => entry.name.getText(ast) === name));
  assert.ok(statement, `missing declaration: ${name}`);
  return statement.getText(ast);
};
const mount = ast.statements.find(item => ts.isExpressionStatement(item)
  && ts.isCallExpression(item.expression) && item.expression.expression.getText(ast) === 'onMounted'
  && item.getText(ast).includes('firstLoad = true'));
const statements = mount.expression.arguments[0].body.statements;
const start = statements.findIndex(item => item.getText(ast).startsWith("chatEvent.off('channel-context-cleared'"));
assert.ok(start >= 0, 'initial load must follow channel event registration');
const initialize = `async function initialize() {\n${statements.slice(start).map(item => item.getText(ast)).join('\n')}\n}`;
const compile = text => ts.transpileModule(text, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS } }).outputText;
const tick = async () => { for (let i = 0; i < 30; i++) await Promise.resolve(); };
const harness = () => {
  const events = new Map(), requests = [];
  const state = {
    reactive: value => value,
    rows: { value: [] }, pinnedRows: { value: [] },
    chat: { curChannel: { id: 'a' }, isObserver: false,
      clearMessageInsertTarget() {}, cancelMultiSelectRelocate() {}, channelRefreshSetup() {},
    },
    chatEvent: { off: name => events.delete(name), on: (name, fn) => events.set(name, fn) },
    handleBattleReportDisplayRefresh() {}, handleBattleReportOpenEditorRequest() {}, handleChannelContextCleared() {},
    initialMessageJumpReady: { value: false }, initCharacterRemark() {}, consumePendingMessageJump() {},
    maybePromptIdentitySync: async () => {}, fetchPinnedMessages: async () => {},
    utils: { configGet: async () => {}, commandsRefresh: async () => {} }, onboarding: { checkAndStartOnboarding() {} },
    messageFilterSignature: { value: '' }, detachedRealtimeRevision: 0, displayOrderRevision: 0,
    detachedRealtimeMessageKeys: new Set(), resetSearchBrowseSession() {}, resetTypingPreview() {},
    nextTick: fn => Promise.resolve().then(fn), INITIAL_MESSAGE_LOAD_LIMIT: 30,
    buildMessageFilterOptions: () => ({}), normalizeMessageList: items => items,
    sortRowsByDisplayOrder() {}, validateMessageInsertTarget() {}, applyCursorUpdate() {}, computeAfterCursorFromRows() {},
    scrollToBottom() {}, showButton: { value: false }, autoFillIfNeeded: async () => {}, tryAutoRestoreSessionDraft() {},
    invalidateDisplayOrderRefresh() { state.displayOrderRevision++; },
    persistOwnedSessionDraft() {}, stopTypingPreviewNow() {}, stopEditingPreviewNow() {}, cancelEditingSession() {},
    textToSend: { value: '' }, draftOwnerChannelKey: { value: '' }, currentChannelKey: { value: '' },
    clearInputModeCache() {}, resetDragState() {}, refreshHistoryEntries() {}, console: { info() {}, warn() {} },
  };
  const names = ['firstLoad', 'messageWindow', 'resetWindowState', 'handleChannelSwitchEvent',
    'latestMessagesFetchEpoch', 'latestMessagesRefetchQueued', 'scheduleLatestMessagesRefetch', 'fetchLatestMessages'];
  const api = vm.runInNewContext(compile(names.map(declaration).join('\n') + '\n' + initialize
    + '\n({initialize, ready:()=>firstLoad});'), state);
  return { state, api, requests, switchTo(channel) { state.chat.curChannel = { id: channel }; state.currentChannelKey.value = channel; events.get('channel-switch-to')?.({ argv: {} }); } };
};

for (const failInitial of [false, true]) {
  test(`initial history ${failInitial ? 'failure' : 'success'} permits later channel switches`, async () => {
    const h = harness(); let initialized = false;
    h.state.utils.configGet = async () => { initialized = true; };
    h.state.chat.messageList = async channel => {
      h.requests.push(channel);
      if (failInitial && channel === 'a') throw new Error('message.list timeout');
      return { data: [{ id: channel + '-message' }], next: '' };
    };
    await h.api.initialize();
    assert.equal(initialized, true, 'remaining initialization must run after the first request');
    assert.equal(h.api.ready(), true);
    assert.equal(h.state.rows.value.length, failInitial ? 0 : 1);
    h.switchTo('b');
    await tick();
    assert.deepEqual(h.requests, ['a', 'b']);
    assert.equal(h.state.rows.value[0]?.id, 'b-message');
  });
}
