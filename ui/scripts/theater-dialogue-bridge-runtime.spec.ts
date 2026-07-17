import assert from 'node:assert/strict'
import { reactive } from 'vue'

import {
  buildRoleSnapshot,
  normalizeBridgePlainText,
  serializeTheaterDialogueMessage,
} from '../src/bridge/sealchatBridgeSerializer'
import { createDefaultTheaterPresentation } from '../src/types/theaterPresentation'
import { mergeTheaterBridgePermissions, TheaterHostBridge } from '../src/views/theater/bridge/TheaterHostBridge'
import {
  isTheaterBridgeDebugEnabled,
  setTheaterBridgeDebugEnabled,
} from '../src/views/theater/bridge/theater-bridge-debug'
import { subscribeTheaterChatMessageEvents } from '../src/views/theater/bridge/theater-chat-message-events'
import {
  THEATER_CHAT_MESSAGE_EVENT_NAMES,
  createTheaterBridgeMessage,
  parseTheaterBridgeMessage,
  type TheaterBridgeMessage,
  type TheaterDialogueMessagePayload,
} from '../src/views/theater/bridge/theater-bridge-protocol'
import { MemoryTransport } from '../src/views/theater/bridge/theater-bridge-transport'
import { createTheaterStageStore } from '../src/views/theater/stage/StageStore'

const run = async () => {
const context = { worldId: 'world-1', channelId: 'channel-1', sessionId: 'session-1' }
const presentation = createDefaultTheaterPresentation()
setTheaterBridgeDebugEnabled(true)
assert.equal(isTheaterBridgeDebugEnabled(), true)
setTheaterBridgeDebugEnabled(false)
assert.equal(isTheaterBridgeDebugEnabled(), false)
assert.deepEqual(
  mergeTheaterBridgePermissions(['stage.view', 'stage.object.edit']),
  [
    'stage.view',
    'stage.object.edit',
    'chat.message.send',
    'chat.composer.insert',
    'chat.character.read',
    'chat.character.select',
    'chat.character.variant.select',
  ],
)
assert.equal(mergeTheaterBridgePermissions(['stage.view'], true).includes('stage.control'), true)
const reactiveRoleSnapshot = buildRoleSnapshot({
  identity: reactive({
    id: 'reactive-identity',
    displayName: 'Reactive Actor',
    theaterPresentation: presentation,
  }),
  resolveAttachmentUrl: () => '',
})
assert.doesNotThrow(() => structuredClone(reactiveRoleSnapshot))
assert.deepEqual(reactiveRoleSnapshot.baseAppearance.theaterPresentation, presentation)

const messagePayload = (messageId = 'message-1'): TheaterDialogueMessagePayload => ({
  messageId,
  createdAt: 1_000,
  displayOrder: 1024,
  icMode: 'ic',
  isWhisper: false,
  isArchived: false,
  isDeleted: false,
  contentText: 'hello',
  actor: {
    identityId: 'identity-1',
    variantId: 'variant-1',
    displayName: 'Actor',
    color: '#ffffff',
    appearance: {
      displayName: 'Actor',
      color: '#ffffff',
      avatar: null,
      decorations: [],
      theaterPresentation: presentation,
      extensions: {},
    },
  },
})

for (const [name, payload] of [
  ['chat.message.created', messagePayload('created')],
  ['chat.message.updated', messagePayload('updated')],
  ['chat.message.removed', { messageId: 'removed' }],
] as const) {
  const parsed = parseTheaterBridgeMessage(createTheaterBridgeMessage(context, {
    kind: 'event',
    source: 'chat',
    target: 'stage',
    name,
    payload,
  }))
  assert.deepEqual(parsed.payload, payload)
}

assert.throws(() => parseTheaterBridgeMessage({
  ...createTheaterBridgeMessage(context, {
    kind: 'event',
    source: 'chat',
    target: 'stage',
    name: 'chat.message.created',
    payload: { ...messagePayload(), unknown: true },
  }),
}), /unrecognized|unknown/i)

for (const invalid of [
  { source: 'plugin' },
  { target: 'chat' },
  { worldId: '' },
  { channelId: '' },
  { sessionId: '' },
]) {
  assert.throws(() => parseTheaterBridgeMessage({
    ...createTheaterBridgeMessage(context, {
      kind: 'event',
      source: 'chat',
      target: 'stage',
      name: 'chat.message.created',
      payload: messagePayload(),
    }),
    ...invalid,
  }))
}

const [sizeSender, sizeReceiver] = MemoryTransport.createPair()
await Promise.all([sizeSender.connect(), sizeReceiver.connect()])
assert.throws(() => sizeSender.send(createTheaterBridgeMessage(context, {
  kind: 'event',
  source: 'chat',
  target: 'stage',
  name: 'chat.message.created',
  payload: {
    ...messagePayload(),
    actor: {
      ...messagePayload().actor,
      appearance: {
        ...messagePayload().actor.appearance,
        extensions: { oversized: 'x'.repeat(300_000) },
      },
    },
  },
})), /exceeds 262144 bytes/)
sizeSender.disconnect()
sizeReceiver.disconnect()

const tiptap = JSON.stringify({
  type: 'doc',
  content: [{
    type: 'paragraph',
    content: [
      { type: 'text', text: '第一行😀' },
      { type: 'hardBreak' },
      { type: 'text', text: '第二行' },
    ],
  }],
})
assert.equal(normalizeBridgePlainText(tiptap), '第一行😀\n第二行')
assert.equal(normalizeBridgePlainText('<p>Hello<br>world &amp; 😀</p>'), 'Hello\nworld & 😀')

const frozenPresentation = createDefaultTheaterPresentation()
frozenPresentation.dialogue.textAlign = 'right'
const serialized = serializeTheaterDialogueMessage({
  id: 'frozen-message',
  createdAt: 1234,
  icMode: 'ic',
  content: tiptap,
  identity: {
    id: 'identity-frozen',
    variantId: 'variant-frozen',
    displayName: 'Frozen Actor',
    color: '#123456',
    theaterPresentation: frozenPresentation,
  },
})
assert.ok(serialized)
assert.equal(serialized.actor.displayName, 'Frozen Actor')
assert.equal(serialized.actor.variantId, 'variant-frozen')
assert.equal(serialized.actor.appearance.theaterPresentation?.dialogue.textAlign, 'right')
assert.equal(serialized.contentRichText, tiptap)
assert.equal(serialized.hasPerformanceContent, false)
const performanceTiptap = JSON.stringify({
  type: 'doc',
  content: [{
    type: 'paragraph',
    content: [{ type: 'text', text: '演出', marks: [{ type: 'performance', attrs: { effect: 'wave' } }] }],
  }],
})
assert.equal(serializeTheaterDialogueMessage({
  id: 'performance-message',
  content: performanceTiptap,
  identity: { id: 'identity-frozen' },
})?.hasPerformanceContent, true)
frozenPresentation.dialogue.textAlign = 'left'
assert.equal(serialized.actor.appearance.theaterPresentation?.dialogue.textAlign, 'right')
assert.equal(serializeTheaterDialogueMessage({ id: 'legacy', content: 'old' })?.actor.appearance.theaterPresentation, null)

type EventHandler = (event: unknown) => void
class FakeChatEvents {
  handlers = new Map<string, Set<EventHandler>>()
  on(name: string, handler: EventHandler) {
    const handlers = this.handlers.get(name) || new Set<EventHandler>()
    handlers.add(handler)
    this.handlers.set(name, handlers)
  }
  off(name: string, handler: EventHandler) {
    this.handlers.get(name)?.delete(handler)
  }
  emit(name: string, event: unknown) {
    this.handlers.get(name)?.forEach((handler) => handler(event))
  }
}

const eventSource = new FakeChatEvents()
const emitted: Array<{ name: string; payload: unknown }> = []
let eventSequence = 0
let initialized = true
let currentContext = { ...context }
let capabilities = new Set<string>(THEATER_CHAT_MESSAGE_EVENT_NAMES)
const dispose = subscribeTheaterChatMessageEvents({
  eventSource,
  client: {
    supports: (_endpoint, capability) => capabilities.has(capability),
    emit: (_target, name, payload) => emitted.push({ name, payload }),
  },
  bridgeContext: context,
  getCurrentContext: () => currentContext,
  isInitialized: () => initialized,
})

const eventFor = (overrides: Record<string, unknown> = {}) => ({
  channel: { id: context.channelId },
  message: {
    id: `message-${eventSequence += 1}`,
    createdAt: 1,
    icMode: 'ic',
    content: 'line one\nline two 😀',
    identity: { id: 'identity-1', displayName: 'Actor', color: '#fff' },
    ...overrides,
  },
})

const acceptedCreated = eventFor({ id: 'accepted' })
eventSource.emit('message-created', acceptedCreated)
eventSource.emit('message-created', acceptedCreated)
assert.deepEqual(emitted.map((item) => item.name), ['chat.message.created'])

for (const overrides of [
  { id: 'ooc', icMode: 'ooc' },
  { id: 'whisper', isWhisper: true },
  { id: 'archived', isArchived: true },
  { id: 'deleted', isDeleted: true },
  { id: 'empty', content: ' \n ' },
  { id: 'no-identity', identity: null },
]) eventSource.emit('message-created', eventFor(overrides))
assert.equal(emitted.length, 1)

eventSource.emit('message-updated', eventFor({ id: 'updated', icMode: 'ooc' }))
eventSource.emit('message-removed', eventFor({ id: 'removed' }))
assert.deepEqual(emitted.map((item) => item.name), [
  'chat.message.created',
  'chat.message.updated',
  'chat.message.removed',
])

currentContext = { ...context, channelId: 'channel-2' }
eventSource.emit('message-updated', eventFor({ id: 'old-channel' }))
assert.equal(emitted.length, 3)
currentContext = { ...context, worldId: 'world-2' }
eventSource.emit('message-updated', eventFor({ id: 'old-world' }))
assert.equal(emitted.length, 3)
currentContext = { ...context, sessionId: 'session-2' }
eventSource.emit('message-removed', eventFor({ id: 'old-session' }))
assert.equal(emitted.length, 3)
currentContext = { ...context }
capabilities = new Set(['chat.message.updated'])
eventSource.emit('message-created', eventFor({ id: 'missing-stage-capability' }))
assert.equal(emitted.length, 3)
initialized = false
eventSource.emit('message-updated', eventFor({ id: 'offline' }))
assert.equal(emitted.length, 3)
initialized = true
dispose()
eventSource.emit('message-updated', eventFor({ id: 'disposed' }))
assert.equal(emitted.length, 3)

class FakeWindow extends EventTarget {
  sent: TheaterBridgeMessage[] = []
  localStorage = { getItem: (_key: string) => null, setItem: (_key: string, _value: string) => undefined }
  postMessage(message: TheaterBridgeMessage) { this.sent.push(message) }
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
const hostWindow = new FakeWindow()
const chatWindow = new FakeWindow()
Object.defineProperty(globalThis, 'window', { configurable: true, value: hostWindow })
const hostCreated: string[] = []
const hostUpdated: string[] = []
const hostRemoved: string[] = []
const host = new TheaterHostBridge({
  context,
  stageStore: createTheaterStageStore('theater-dialogue-host-test'),
  getChatWindow: () => chatWindow as unknown as Window,
  origin: 'https://sealchat.test',
  userId: 'user-1',
  permissions: [],
  onChatMessageCreated: (payload) => hostCreated.push(payload.messageId),
  onChatMessageUpdated: (payload) => hostUpdated.push(payload.messageId),
  onChatMessageRemoved: (payload) => hostRemoved.push(payload.messageId),
})
await host.start()
;(host as any).hostClient.setRemoteCapabilities('chat', THEATER_CHAT_MESSAGE_EVENT_NAMES)

const routeChatEvent = (name: typeof THEATER_CHAT_MESSAGE_EVENT_NAMES[number], payload: unknown) => (
  (host as any).routeFromChat(createTheaterBridgeMessage(context, {
    kind: 'event', source: 'chat', target: 'stage', name, payload,
  }))
)

routeChatEvent('chat.message.created', messagePayload('offline-drop'))
assert.equal((host as any).pendingChatMessages.length, 0)
;(host as any).setChatOnline(true)
;(host as any).hostClient.setRemoteCapabilities('chat', [])
routeChatEvent('chat.message.created', messagePayload('source-capability-drop'))
await tick()
assert.equal(hostCreated.length, 0)
;(host as any).hostClient.setRemoteCapabilities('chat', THEATER_CHAT_MESSAGE_EVENT_NAMES)
routeChatEvent('chat.message.created', messagePayload('host-created'))
routeChatEvent('chat.message.updated', messagePayload('host-updated'))
routeChatEvent('chat.message.removed', { messageId: 'host-removed' })
await tick()
assert.deepEqual(hostCreated, ['host-created'])
assert.deepEqual(hostUpdated, ['host-updated'])
assert.deepEqual(hostRemoved, ['host-removed'])

;(host as any).routeFromChat({
  ...createTheaterBridgeMessage(context, {
    kind: 'event', source: 'chat', target: 'stage', name: 'chat.message.updated', payload: messagePayload('bad-context'),
  }),
  sessionId: 'old-session',
})
await tick()
assert.deepEqual(hostUpdated, ['host-updated'])
host.stop()

const missingStageCapability = new TheaterHostBridge({
  context,
  stageStore: createTheaterStageStore('theater-dialogue-capability-test'),
  getChatWindow: () => chatWindow as unknown as Window,
  origin: 'https://sealchat.test',
  userId: 'user-1',
  permissions: [],
  stageCapabilities: ['stage.scene.read'],
  onChatMessageCreated: (payload) => hostCreated.push(payload.messageId),
})
await missingStageCapability.start()
;(missingStageCapability as any).hostClient.setRemoteCapabilities('chat', THEATER_CHAT_MESSAGE_EVENT_NAMES)
;(missingStageCapability as any).setChatOnline(true)
;(missingStageCapability as any).routeFromChat(createTheaterBridgeMessage(context, {
  kind: 'event', source: 'chat', target: 'stage', name: 'chat.message.created', payload: messagePayload('capability-drop'),
}))
await tick()
assert.deepEqual(hostCreated, ['host-created'])
missingStageCapability.stop()

console.log('theater dialogue bridge runtime tests passed')
}

void run()
