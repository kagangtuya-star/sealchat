import {
  createDefaultTheaterPresentation,
  normalizeTheaterPresentation,
  resolveTheaterPresentation,
  type ResolvedTheaterPresentation,
} from '../../../types/theaterPresentation'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import {
  createTheaterDialogueQueueState,
  getTheaterDialogueTextLength,
  isTheaterDialogueTyping,
  reduceTheaterDialogueQueue,
  type TheaterDialogueMessage,
  type TheaterDialogueQueueState,
} from '../bridge/theater-dialogue-queue'

export const THEATER_DIALOGUE_CHARACTERS_PER_SECOND = 32
export const THEATER_DIALOGUE_MIN_TYPING_MS = 400
export const THEATER_DIALOGUE_MAX_TYPING_MS = 8_000
export const THEATER_DIALOGUE_HOLD_MS = 900

export type TheaterDialoguePlaybackPhase = 'idle' | 'typing' | 'hold'

export interface TheaterDialogueRuntimeSnapshot {
  queue: TheaterDialogueQueueState
  phase: TheaterDialoguePlaybackPhase
  reducedMotion: boolean
}
interface TheaterDialogueScheduler {
  setTimeout(callback: () => void, delayMs: number): ReturnType<typeof setTimeout>
  clearTimeout(timer: ReturnType<typeof setTimeout>): void
}

interface TheaterDialogueRuntimeOptions {
  reducedMotion?: boolean
  scheduler?: TheaterDialogueScheduler
}

const defaultScheduler: TheaterDialogueScheduler = {
  setTimeout: (callback, delayMs) => globalThis.setTimeout(callback, delayMs),
  clearTimeout: (timer) => globalThis.clearTimeout(timer),
}

export const getTheaterDialogueTypingDuration = (characterCount: number) => {
  if (characterCount <= 0) return 0
  return Math.min(
    THEATER_DIALOGUE_MAX_TYPING_MS,
    Math.max(THEATER_DIALOGUE_MIN_TYPING_MS, characterCount / THEATER_DIALOGUE_CHARACTERS_PER_SECOND * 1_000),
  )
}

export const resolveTheaterDialoguePresentation = (
  message: TheaterDialogueMessage | null | undefined,
  snapshot?: ChatCharactersSnapshotPayload | null,
): ResolvedTheaterPresentation => {
  if (message?.actor.appearance.theaterPresentation) {
    return normalizeTheaterPresentation(message.actor.appearance.theaterPresentation)
  }
  const character = snapshot?.characters.find((item) => item.identityId === message?.actor.identityId)
  if (!character) return createDefaultTheaterPresentation()

  let presentation = character.baseAppearance.theaterPresentation || null
  if (character.activeVariantId === message?.actor.variantId) {
    presentation = character.resolvedAppearance.theaterPresentation || presentation
  } else if (message?.actor.variantId) {
    const variant = character.variants.find((item) => item.variantId === message.actor.variantId)
    const patch = variant?.appearancePatch.theaterPresentation
    if (patch !== undefined) presentation = resolveTheaterPresentation(presentation, patch)
  }
  return presentation ? normalizeTheaterPresentation(presentation) : createDefaultTheaterPresentation()
}

export class TheaterDialogueRuntime {
  private queue = createTheaterDialogueQueueState()
  private phase: TheaterDialoguePlaybackPhase = 'idle'
  private reducedMotion: boolean
  private readonly scheduler: TheaterDialogueScheduler
  private timer: ReturnType<typeof setTimeout> | null = null
  private timerGeneration = 0
  private disposed = false
  private readonly listeners = new Set<(snapshot: TheaterDialogueRuntimeSnapshot) => void>()

  constructor(options: TheaterDialogueRuntimeOptions = {}) {
    this.reducedMotion = options.reducedMotion === true
    this.scheduler = options.scheduler || defaultScheduler
  }

  getSnapshot = (): TheaterDialogueRuntimeSnapshot => ({
    queue: structuredClone(this.queue),
    phase: this.phase,
    reducedMotion: this.reducedMotion,
  })

  subscribe = (listener: (snapshot: TheaterDialogueRuntimeSnapshot) => void) => {
    this.listeners.add(listener)
    listener(this.getSnapshot())
    return () => { this.listeners.delete(listener) }
  }

  created = (message: TheaterDialogueMessage) => {
    if (this.disposed) return
    const previousCurrentId = this.queue.current?.message.messageId
    const next = reduceTheaterDialogueQueue(this.queue, { type: 'created', message: structuredClone(message) })
    if (next === this.queue) return
    this.queue = next
    if (this.queue.current?.message.messageId !== previousCurrentId) this.startCurrent()
    else {
      this.emit()
      if (this.phase === 'hold') this.scheduleAdvance()
    }
  }

  updated = (message: TheaterDialogueMessage) => {
    if (this.disposed) return
    const next = reduceTheaterDialogueQueue(this.queue, { type: 'updated', message: structuredClone(message) })
    if (next === this.queue) return
    const currentChanged = next.current?.message.messageId === message.messageId
    this.queue = next
    if (currentChanged) this.armCurrent()
    else this.emit()
  }

  removed = (messageId: string) => {
    if (this.disposed) return
    const previousCurrentId = this.queue.current?.message.messageId
    const next = reduceTheaterDialogueQueue(this.queue, { type: 'removed', messageId })
    if (next === this.queue) return
    this.queue = next
    if (previousCurrentId === messageId) this.startCurrent()
    else this.emit()
  }

  completeCurrent = () => {
    if (this.disposed || !this.queue.current) return
    this.queue = reduceTheaterDialogueQueue(this.queue, { type: 'complete-current' })
    this.phase = 'hold'
    this.clearTimer()
    this.emit()
    this.scheduleAdvance()
  }

  skip = () => {
    if (this.disposed) return
    const previousCurrentId = this.queue.current?.message.messageId
    const wasTyping = isTheaterDialogueTyping(this.queue.current)
    this.queue = reduceTheaterDialogueQueue(this.queue, { type: 'skip' })
    if (!this.queue.current) {
      this.phase = 'idle'
      this.clearTimer()
      this.emit()
      return
    }
    if (this.queue.current.message.messageId !== previousCurrentId) {
      this.startCurrent()
      return
    }
    if (wasTyping) {
      this.phase = 'hold'
      this.clearTimer()
      this.emit()
      this.scheduleAdvance()
      return
    }
    this.emit()
  }

  close = () => {
    if (this.disposed) return
    this.queue = reduceTheaterDialogueQueue(this.queue, { type: 'close' })
    this.phase = 'idle'
    this.clearTimer()
    this.emit()
  }

  setReducedMotion = (reducedMotion: boolean) => {
    if (this.disposed || this.reducedMotion === reducedMotion) return
    this.reducedMotion = reducedMotion
    this.armCurrent()
  }

  reset = () => {
    if (this.disposed) return
    this.queue = reduceTheaterDialogueQueue(this.queue, { type: 'reset' })
    this.phase = 'idle'
    this.clearTimer()
    this.emit()
  }

  dispose = () => {
    if (this.disposed) return
    this.clearTimer()
    this.queue = createTheaterDialogueQueueState()
    this.phase = 'idle'
    this.disposed = true
    this.listeners.clear()
  }

  private startCurrent() {
    this.clearTimer()
    if (!this.queue.current) {
      this.phase = 'idle'
      this.emit()
      return
    }
    this.queue = {
      ...this.queue,
      current: { ...this.queue.current, revealedCharacters: 0 },
    }
    this.armCurrent()
  }

  private armCurrent() {
    this.clearTimer()
    const current = this.queue.current
    if (!current) {
      this.phase = 'idle'
      this.emit()
      return
    }
    if (this.reducedMotion) {
      this.queue = reduceTheaterDialogueQueue(this.queue, { type: 'complete-current' })
      this.phase = 'hold'
      this.emit()
      this.scheduleAdvance()
      return
    }
    if (!isTheaterDialogueTyping(current)) {
      this.phase = 'hold'
      this.emit()
      this.scheduleAdvance()
      return
    }
    this.phase = 'typing'
    this.emit()
    const length = getTheaterDialogueTextLength(current.message)
    const interval = getTheaterDialogueTypingDuration(length) / Math.max(1, length)
    this.schedule(() => {
      if (!this.queue.current) return
      this.queue = reduceTheaterDialogueQueue(this.queue, {
        type: 'reveal',
        characterCount: this.queue.current.revealedCharacters + 1,
      })
      if (isTheaterDialogueTyping(this.queue.current)) this.armCurrent()
      else {
        this.phase = 'hold'
        this.emit()
        this.scheduleAdvance()
      }
    }, interval)
  }

  private scheduleAdvance() {
    if (!this.queue.current || this.queue.waiting.length === 0 || this.timer !== null) return
    this.schedule(() => {
      this.queue = reduceTheaterDialogueQueue(this.queue, { type: 'advance' })
      this.startCurrent()
    }, THEATER_DIALOGUE_HOLD_MS)
  }

  private schedule(callback: () => void, delayMs: number) {
    const generation = this.timerGeneration
    this.timer = this.scheduler.setTimeout(() => {
      this.timer = null
      if (this.disposed || generation !== this.timerGeneration) return
      callback()
    }, Math.max(0, delayMs))
  }

  private clearTimer() {
    this.timerGeneration += 1
    if (this.timer !== null) this.scheduler.clearTimeout(this.timer)
    this.timer = null
  }

  private emit() {
    if (this.disposed) return
    const snapshot = this.getSnapshot()
    this.listeners.forEach((listener) => listener(snapshot))
  }
}
