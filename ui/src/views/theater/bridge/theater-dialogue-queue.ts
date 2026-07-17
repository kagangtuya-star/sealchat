import type { ResolvedTheaterPresentation } from '../../../types/theaterPresentation'

export const THEATER_DIALOGUE_QUEUE_LIMIT = 64
export const THEATER_DIALOGUE_DEDUPE_LIMIT = 512

export interface TheaterDialogueMessage {
  messageId: string
  createdAt: number
  displayOrder?: number
  icMode: 'ic' | 'ooc'
  isWhisper: boolean
  isArchived: boolean
  isDeleted: boolean
  contentText: string
  contentRichText?: string
  hasPerformanceContent?: boolean
  actor: {
    identityId: string | null
    variantId: string | null
    displayName: string
    color: string
    appearance: {
      theaterPresentation?: ResolvedTheaterPresentation | null
      [key: string]: unknown
    }
  }
}

export interface TheaterDialogueQueueItem {
  sequence: number
  message: TheaterDialogueMessage
  revealedCharacters: number
}

export interface TheaterDialogueQueueState {
  current: TheaterDialogueQueueItem | null
  waiting: TheaterDialogueQueueItem[]
  recentMessageIds: string[]
  lastSequence: number
  dismissedThroughSequence: number
}

export type TheaterDialogueQueueAction =
  | { type: 'created'; message: TheaterDialogueMessage }
  | { type: 'updated'; message: TheaterDialogueMessage }
  | { type: 'removed'; messageId: string }
  | { type: 'reveal'; characterCount: number }
  | { type: 'complete-current' }
  | { type: 'advance' }
  | { type: 'skip' }
  | { type: 'close' }
  | { type: 'reset' }

export const createTheaterDialogueQueueState = (): TheaterDialogueQueueState => ({
  current: null,
  waiting: [],
  recentMessageIds: [],
  lastSequence: 0,
  dismissedThroughSequence: 0,
})

export const shouldEnqueueTheaterDialogue = (message: TheaterDialogueMessage) => (
  message.icMode === 'ic'
  && !message.isWhisper
  && !message.isArchived
  && !message.isDeleted
  && message.contentText.trim().length > 0
  && Boolean(message.actor.identityId?.trim())
)

export const getTheaterDialogueTextLength = (message: TheaterDialogueMessage) => (
  Array.from(message.contentText).length
)

export const isTheaterDialogueTyping = (item: TheaterDialogueQueueItem | null) => (
  item !== null && item.revealedCharacters < getTheaterDialogueTextLength(item.message)
)

export const reduceTheaterDialogueQueue = (
  state: TheaterDialogueQueueState,
  action: TheaterDialogueQueueAction,
): TheaterDialogueQueueState => {
  switch (action.type) {
    case 'created':
      return enqueueCreatedMessage(state, action.message)
    case 'updated':
      return updateMessage(state, action.message)
    case 'removed':
      return removeMessage(state, action.messageId)
    case 'reveal': {
      if (!state.current) return state
      if (!Number.isFinite(action.characterCount)) return state
      const revealedCharacters = Math.min(
        getTheaterDialogueTextLength(state.current.message),
        Math.max(state.current.revealedCharacters, Math.floor(action.characterCount)),
      )
      if (revealedCharacters === state.current.revealedCharacters) return state
      return { ...state, current: { ...state.current, revealedCharacters } }
    }
    case 'complete-current':
      return completeCurrent(state)
    case 'advance':
      return promoteNext(state)
    case 'skip':
      return skipDialogue(state)
    case 'close':
      return {
        ...state,
        current: null,
        waiting: [],
        dismissedThroughSequence: state.lastSequence,
      }
    case 'reset':
      return createTheaterDialogueQueueState()
  }
}

const enqueueCreatedMessage = (
  state: TheaterDialogueQueueState,
  message: TheaterDialogueMessage,
): TheaterDialogueQueueState => {
  if (!shouldEnqueueTheaterDialogue(message) || state.recentMessageIds.includes(message.messageId)) return state

  const sequence = state.lastSequence + 1
  const item: TheaterDialogueQueueItem = { sequence, message, revealedCharacters: 0 }
  const recentMessageIds = [...state.recentMessageIds, message.messageId]
    .slice(-THEATER_DIALOGUE_DEDUPE_LIMIT)
  if (!state.current) {
    return { ...state, current: item, recentMessageIds, lastSequence: sequence }
  }

  const waiting = [...state.waiting, item]
  const maximumWaiting = Math.max(0, THEATER_DIALOGUE_QUEUE_LIMIT - 1)
  return {
    ...state,
    waiting: waiting.slice(-maximumWaiting),
    recentMessageIds,
    lastSequence: sequence,
  }
}

const updateMessage = (
  state: TheaterDialogueQueueState,
  message: TheaterDialogueMessage,
): TheaterDialogueQueueState => {
  if (state.current?.message.messageId === message.messageId) {
    return {
      ...state,
      current: {
        ...state.current,
        message,
        revealedCharacters: Math.min(state.current.revealedCharacters, getTheaterDialogueTextLength(message)),
      },
    }
  }
  const index = state.waiting.findIndex((item) => item.message.messageId === message.messageId)
  if (index < 0) return state
  const waiting = state.waiting.slice()
  waiting[index] = { ...waiting[index], message }
  return { ...state, waiting }
}

const removeMessage = (state: TheaterDialogueQueueState, messageId: string): TheaterDialogueQueueState => {
  if (state.current?.message.messageId === messageId) return promoteNext(state)
  const waiting = state.waiting.filter((item) => item.message.messageId !== messageId)
  return waiting.length === state.waiting.length ? state : { ...state, waiting }
}

const promoteNext = (state: TheaterDialogueQueueState): TheaterDialogueQueueState => {
  const nextIndex = state.waiting.findIndex((item) => item.sequence > state.dismissedThroughSequence)
  if (nextIndex < 0) return { ...state, current: null, waiting: [] }
  return {
    ...state,
    current: state.waiting[nextIndex],
    waiting: state.waiting.slice(nextIndex + 1),
  }
}

const completeCurrent = (state: TheaterDialogueQueueState): TheaterDialogueQueueState => {
  if (!state.current) return state
  const revealedCharacters = getTheaterDialogueTextLength(state.current.message)
  if (state.current.revealedCharacters === revealedCharacters) return state
  return { ...state, current: { ...state.current, revealedCharacters } }
}

const skipDialogue = (state: TheaterDialogueQueueState): TheaterDialogueQueueState => {
  if (state.waiting.length > 0) {
    const latest = state.waiting[state.waiting.length - 1]
    return { ...state, current: { ...latest, revealedCharacters: 0 }, waiting: [] }
  }
  if (isTheaterDialogueTyping(state.current)) return completeCurrent(state)
  return state
}
