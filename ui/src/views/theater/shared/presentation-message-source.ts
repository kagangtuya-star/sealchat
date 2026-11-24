import type { TheaterDialogueMessage } from '../bridge/theater-dialogue-queue'

export interface PresentationMessageSource {
  // Only live messages are delivered. null invalidates the current message.
  subscribe(listener: (message: TheaterDialogueMessage | null) => void): () => void
}

export const createPresentationMessageSource = () => {
  const listeners = new Set<Parameters<PresentationMessageSource['subscribe']>[0]>()
  return {
    subscribe(listener: Parameters<PresentationMessageSource['subscribe']>[0]) {
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
    publish(message: TheaterDialogueMessage) {
      listeners.forEach((listener) => listener(message))
    },
  }
}

export const rememberPresentationId = (ids: Set<string>, id: string, limit = 256) => {
  if (ids.has(id)) return false
  ids.add(id)
  if (ids.size > limit) ids.delete(ids.values().next().value!)
  return true
}
