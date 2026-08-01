import {
  resolveTheaterChatEventChannelId,
  serializeTheaterDialogueMessage,
  serializeTheaterDialogueRemoved,
} from '../../../bridge/sealchatBridgeSerializer'
import { shouldEnqueueTheaterDialogue } from './theater-dialogue-queue'
import {
  theaterDialogueMessagePayloadSchema,
  theaterDialogueMessageRemovedPayloadSchema,
  type TheaterBridgeContext,
} from './theater-bridge-protocol'

type ChatMessageEventName = 'message-created' | 'message-updated' | 'message-removed'
type TheaterMessageBridgeEventName = 'chat.message.created' | 'chat.message.updated' | 'chat.message.removed'

interface ChatEventSource {
  on(name: ChatMessageEventName, handler: (event: unknown) => void): void
  off(name: ChatMessageEventName, handler: (event: unknown) => void): void
}

interface TheaterMessageBridgeClient {
  supports(endpoint: 'stage', capability: string): boolean
  emit<T>(target: 'stage', name: TheaterMessageBridgeEventName, payload: T): void
}

interface TheaterChatMessageSubscriptionOptions {
  eventSource: ChatEventSource
  client: TheaterMessageBridgeClient
  bridgeContext: TheaterBridgeContext
  getCurrentContext: () => TheaterBridgeContext
  isInitialized: () => boolean
  resolveAttachmentUrl?: (token?: string) => string
}

const messageFromEvent = (input: unknown): unknown => (
  input && typeof input === 'object' && 'message' in input
    ? (input as { message?: unknown }).message
    : null
)

const matchesCurrentContext = (
  event: unknown,
  expected: TheaterBridgeContext,
  current: TheaterBridgeContext,
) => current.worldId === expected.worldId
  && current.channelId === expected.channelId
  && current.sessionId === expected.sessionId
  && resolveTheaterChatEventChannelId(event) === expected.channelId

export const subscribeTheaterChatMessageEvents = (
  options: TheaterChatMessageSubscriptionOptions,
): (() => void) => {
  const seenCreated = new WeakSet<object>()
  const seenUpdated = new WeakSet<object>()
  const seenRemoved = new WeakSet<object>()

  const alreadyHandled = (event: unknown, seen: WeakSet<object>) => {
    if (!event || typeof event !== 'object') return false
    if (seen.has(event)) return true
    seen.add(event)
    return false
  }

  const canPublish = (event: unknown, capability: TheaterMessageBridgeEventName) => (
    options.isInitialized()
    && options.client.supports('stage', capability)
    && matchesCurrentContext(event, options.bridgeContext, options.getCurrentContext())
  )

  const handleCreated = (event: unknown) => {
    if (alreadyHandled(event, seenCreated) || !canPublish(event, 'chat.message.created')) return
    const payload = serializeTheaterDialogueMessage(messageFromEvent(event), options.resolveAttachmentUrl)
    const parsed = theaterDialogueMessagePayloadSchema.safeParse(payload)
    if (!parsed.success || !shouldEnqueueTheaterDialogue(parsed.data)) return
    options.client.emit('stage', 'chat.message.created', parsed.data)
  }

  const handleUpdated = (event: unknown) => {
    if (alreadyHandled(event, seenUpdated) || !canPublish(event, 'chat.message.updated')) return
    const payload = serializeTheaterDialogueMessage(messageFromEvent(event), options.resolveAttachmentUrl)
    const parsed = theaterDialogueMessagePayloadSchema.safeParse(payload)
    if (!parsed.success) return
    options.client.emit('stage', 'chat.message.updated', parsed.data)
  }

  const handleRemoved = (event: unknown) => {
    if (alreadyHandled(event, seenRemoved) || !canPublish(event, 'chat.message.removed')) return
    const payload = serializeTheaterDialogueRemoved(messageFromEvent(event))
    const parsed = theaterDialogueMessageRemovedPayloadSchema.safeParse(payload)
    if (!parsed.success) return
    options.client.emit('stage', 'chat.message.removed', parsed.data)
  }

  options.eventSource.on('message-created', handleCreated)
  options.eventSource.on('message-updated', handleUpdated)
  options.eventSource.on('message-removed', handleRemoved)

  return () => {
    options.eventSource.off('message-created', handleCreated)
    options.eventSource.off('message-updated', handleUpdated)
    options.eventSource.off('message-removed', handleRemoved)
  }
}
