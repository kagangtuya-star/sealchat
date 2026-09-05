import { resolveTheaterChatEventChannelId, serializeTheaterDialogueMessage, serializeTheaterDialogueRemoved } from './sealchatBridgeSerializer'
import { shouldEnqueueTheaterDialogue } from '../views/theater/bridge/theater-dialogue-queue'
import { theaterDialogueMessagePayloadSchema, theaterDialogueMessageRemovedPayloadSchema, type TheaterDialogueMessagePayload, type TheaterDialogueMessageRemovedPayload } from '../views/theater/bridge/theater-bridge-protocol'

type EventName = 'message-created' | 'message-updated' | 'message-removed'
interface EventSource {
  on(name: EventName, handler: (event: unknown) => void): void
  off(name: EventName, handler: (event: unknown) => void): void
}
export type ChannelEmbedDialogueEvent =
  | { topic: 'theater.dialogue.created' | 'theater.dialogue.updated'; payload: TheaterDialogueMessagePayload }
  | { topic: 'theater.dialogue.removed'; payload: TheaterDialogueMessageRemovedPayload }

/** One lazy source per host page; no message history or per-instance serialization. */
export const createChannelEmbedTheaterDialogue = (source: EventSource, resolveAttachmentUrl?: (token?: string) => string) => {
  const subscribers = new Set<{ channelId: string; identityId: string; receive: (event: ChannelEmbedDialogueEvent) => void }>()
  const handlers = new Map<EventName, (event: unknown) => void>()
  const start = () => {
    for (const name of ['message-created', 'message-updated', 'message-removed'] as const) {
      const seen = new WeakSet<object>()
      const handler = (event: unknown) => {
        if (!event || typeof event !== 'object' || seen.has(event)) return
        seen.add(event)
        const channelId = resolveTheaterChatEventChannelId(event)
        const targets = [...subscribers].filter(item => item.channelId === channelId)
        if (!targets.length) return
        const message = 'message' in event ? event.message : null
        if (name === 'message-removed') {
          const parsed = theaterDialogueMessageRemovedPayloadSchema.safeParse(serializeTheaterDialogueRemoved(message))
          if (parsed.success) targets.forEach(item => item.receive({ topic: 'theater.dialogue.removed', payload: parsed.data }))
          return
        }
        const parsed = theaterDialogueMessagePayloadSchema.safeParse(serializeTheaterDialogueMessage(message, resolveAttachmentUrl))
        if (!parsed.success || (name === 'message-created' && !shouldEnqueueTheaterDialogue(parsed.data))) return
        const topic = name === 'message-created' ? 'theater.dialogue.created' : 'theater.dialogue.updated'
        targets.filter(item => item.identityId === parsed.data.actor.identityId)
          .forEach(item => item.receive({ topic, payload: parsed.data }))
      }
      handlers.set(name, handler)
      source.on(name, handler)
    }
  }
  return {
    subscribe(channelId: string, identityId: string, receive: (event: ChannelEmbedDialogueEvent) => void) {
      const subscriber = { channelId, identityId, receive }
      subscribers.add(subscriber)
      if (subscribers.size === 1) start()
      return () => {
        subscribers.delete(subscriber)
        if (!subscribers.size) {
          handlers.forEach((handler, name) => source.off(name, handler))
          handlers.clear()
        }
      }
    },
  }
}
