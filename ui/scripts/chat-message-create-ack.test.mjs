import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

const source = readFileSync(new URL('../src/views/chat/messageCreateAck.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const module = { exports: {} };
vm.runInNewContext(compiled, { exports: module.exports, module });
const { shouldApplyMessageCreateAck } = module.exports;

const upsert = (messages, incoming) => {
  const index = messages.findIndex(item => item.id === incoming.id);
  if (index >= 0) {
    messages.splice(index, 1, { ...messages[index], ...incoming });
  } else {
    messages.push(incoming);
  }
};

{
  const optimistic = { id: 'client-1', clientId: 'client-1', content: 'draft' };
  const messages = [optimistic];
  const pending = new Set([optimistic]);
  const ack = { id: 'message-1', clientId: 'client-1', content: 'sent' };
  assert.equal(shouldApplyMessageCreateAck('channel-1', 'channel-1', pending.has(optimistic)), true);
  Object.assign(optimistic, ack);
  pending.delete(optimistic);
  upsert(messages, optimistic);
  upsert(messages, { ...ack });
  assert.deepEqual(messages.map(item => item.id), ['message-1'], 'ACK-first + Event-later must keep one message');
}

{
  const optimistic = { id: 'client-2', clientId: 'client-2', content: 'draft' };
  const messages = [optimistic];
  const pending = new Set([optimistic]);
  const created = { id: 'message-2', clientId: 'client-2', content: 'sent' };
  pending.delete(optimistic);
  Object.assign(optimistic, created);
  upsert(messages, optimistic);
  upsert(messages, { ...created, content: 'edited' });
  const applyLateAck = shouldApplyMessageCreateAck('channel-1', 'channel-1', pending.has(optimistic));
  assert.equal(applyLateAck, false);
  if (applyLateAck) {
    Object.assign(optimistic, created);
    upsert(messages, optimistic);
  }
  assert.deepEqual(messages, [{ ...created, content: 'edited' }], 'Event-first + ACK-later must preserve newer event state');
}

{
  const optimistic = { id: 'client-3', clientId: 'client-3', content: 'draft' };
  const messages = [optimistic];
  const pending = new Set([optimistic]);
  const created = { id: 'message-3', clientId: 'client-3', content: 'sent' };
  pending.delete(optimistic);
  Object.assign(optimistic, created);
  upsert(messages, optimistic);
  messages.splice(messages.findIndex(item => item.id === created.id), 1);
  if (shouldApplyMessageCreateAck('channel-1', 'channel-1', pending.has(optimistic))) {
    upsert(messages, created);
  }
  assert.deepEqual(messages, [], 'a late ACK must not recreate a message deleted after its realtime event');
}

assert.equal(
  shouldApplyMessageCreateAck('channel-1', 'channel-2', true),
  false,
  'a response from an old channel must not update the current channel',
);

console.log('message.create ACK ordering regressions passed');
