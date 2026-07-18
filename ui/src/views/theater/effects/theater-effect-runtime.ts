import type { StageObject } from '../shared/stage-types'
import type { TheaterDialogueMessage } from '../bridge/theater-dialogue-queue'
import type { TheaterDialogueRuntime, TheaterDialogueRuntimeSnapshot } from '../dialogue/theater-dialogue-runtime'
import { cloneStageData } from '../stage/stage-editing'
import {
  isTheaterEffectObject,
  theaterEffectConfigFromObject,
  type TheaterEffectConfig,
} from './theater-effect-types'

export interface TheaterEffectPlayback {
  instanceId: string
  effectId: string
  expiresAt: number
  object: StageObject
  config: TheaterEffectConfig
  preview: boolean
}

interface TheaterEffectScheduler {
  setTimeout(callback: () => void, delayMs: number): ReturnType<typeof setTimeout>
  clearTimeout(timer: ReturnType<typeof setTimeout>): void
  now(): number
}

interface TheaterEffectRuntimeOptions {
  dialogueRuntime: TheaterDialogueRuntime
  getObjects: () => StageObject[]
  scheduler?: TheaterEffectScheduler
  maximumActive?: number
  onStart?: (playback: TheaterEffectPlayback) => void
}

const defaultScheduler: TheaterEffectScheduler = {
  setTimeout: (callback, delayMs) => globalThis.setTimeout(callback, delayMs),
  clearTimeout: (timer) => globalThis.clearTimeout(timer),
  now: () => Date.now(),
}

const normalizedKeywordText = (value: string) => value.normalize('NFKC').toLocaleLowerCase()
const normalizedActorName = (value: string) => value.normalize('NFKC').trim()

export const theaterEffectMatchesMessage = (config: TheaterEffectConfig, message: TheaterDialogueMessage) => {
  if (!config.keywords.length) return false
  if (config.targetActorName && normalizedActorName(config.targetActorName) !== normalizedActorName(message.actor.displayName)) return false
  const content = normalizedKeywordText(message.contentText)
  return config.keywords.some((keyword) => content.includes(normalizedKeywordText(keyword)))
}

export class TheaterEffectRuntime {
  private readonly scheduler: TheaterEffectScheduler
  private readonly maximumActive: number
  private readonly listeners = new Set<(playbacks: TheaterEffectPlayback[]) => void>()
  private readonly timers = new Map<string, ReturnType<typeof setTimeout>>()
  private readonly lastTriggeredAt = new Map<string, number>()
  private readonly lastTriggeredMessageId = new Map<string, string>()
  private active: TheaterEffectPlayback[] = []
  private currentMessage: TheaterDialogueMessage | null = null
  private currentMessageId = ''
  private instanceSequence = 0
  private unsubscribeDialogue: (() => void) | null = null
  private disposed = false

  constructor(private readonly options: TheaterEffectRuntimeOptions) {
    this.scheduler = options.scheduler || defaultScheduler
    this.maximumActive = Math.max(1, Math.min(8, options.maximumActive || 4))
    this.unsubscribeDialogue = options.dialogueRuntime.subscribe(this.handleDialogueSnapshot)
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', this.handleVisibilityChange)
  }

  subscribe = (listener: (playbacks: TheaterEffectPlayback[]) => void) => {
    this.listeners.add(listener)
    listener(this.getActive())
    return () => { this.listeners.delete(listener) }
  }

  getActive = () => this.active.map((item) => cloneStageData(item))

  preview = (object: StageObject) => {
    if (!isTheaterEffectObject(object)) return
    this.start(object, true)
  }

  stop = (effectId: string) => {
    const timer = this.timers.get(effectId)
    if (timer) this.scheduler.clearTimeout(timer)
    this.timers.delete(effectId)
    const next = this.active.filter((item) => item.effectId !== effectId)
    if (next.length === this.active.length) return
    this.active = next
    this.emit()
  }

  reconcile = () => {
    const validIds = new Set(this.options.getObjects().filter(isTheaterEffectObject).map((object) => object.id))
    this.active.filter((item) => !validIds.has(item.effectId)).forEach((item) => this.stop(item.effectId))
    for (const effectId of this.lastTriggeredMessageId.keys()) {
      if (!validIds.has(effectId)) this.lastTriggeredMessageId.delete(effectId)
    }
    if (this.currentMessage) this.triggerEffects(this.currentMessage)
  }

  dispose = () => {
    if (this.disposed) return
    this.disposed = true
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', this.handleVisibilityChange)
    this.unsubscribeDialogue?.()
    this.unsubscribeDialogue = null
    this.timers.forEach((timer) => this.scheduler.clearTimeout(timer))
    this.timers.clear()
    this.active = []
    this.listeners.clear()
  }

  private readonly handleDialogueSnapshot = (snapshot: TheaterDialogueRuntimeSnapshot) => {
    if (this.disposed) return
    const message = snapshot.queue.current?.message
    if (!message) {
      this.currentMessage = null
      this.currentMessageId = ''
      return
    }
    if (message.messageId === this.currentMessageId) return
    this.currentMessageId = message.messageId
    this.currentMessage = message
    this.triggerEffects(message)
  }

  private triggerEffects(message: TheaterDialogueMessage) {
    const now = this.scheduler.now()
    this.options.getObjects()
      .filter((object) => isTheaterEffectObject(object) && object.visible)
      .sort((left, right) => left.transform.z - right.transform.z || left.transform.order - right.transform.order)
      .forEach((object) => {
        const config = theaterEffectConfigFromObject(object)
        const lastTriggeredAt = this.lastTriggeredAt.get(object.id)
        if (
          this.lastTriggeredMessageId.get(object.id) === message.messageId
          ||
          !theaterEffectMatchesMessage(config, message)
          || (lastTriggeredAt !== undefined && now - lastTriggeredAt < config.cooldownMs)
        ) return
        this.lastTriggeredAt.set(object.id, now)
        this.lastTriggeredMessageId.set(object.id, message.messageId)
        this.start(object, false)
      })
  }

  private readonly handleVisibilityChange = () => {
    if (document.visibilityState !== 'visible') return
    const now = this.scheduler.now()
    this.active
      .filter((playback) => playback.expiresAt <= now)
      .forEach((playback) => this.stop(playback.effectId))
  }

  private start(object: StageObject, preview: boolean) {
    this.stop(object.id)
    if (this.active.length >= this.maximumActive) this.stop(this.active[0].effectId)
    const config = theaterEffectConfigFromObject(object)
    const playback: TheaterEffectPlayback = {
      instanceId: `${object.id}:${++this.instanceSequence}`,
      effectId: object.id,
      expiresAt: this.scheduler.now() + config.durationMs,
      object: cloneStageData(object),
      config: cloneStageData(config),
      preview,
    }
    this.active = [...this.active, playback]
    this.timers.set(object.id, this.scheduler.setTimeout(() => this.stop(object.id), config.durationMs))
    this.options.onStart?.(cloneStageData(playback))
    this.emit()
  }

  private emit() {
    if (this.disposed) return
    const active = this.getActive()
    this.listeners.forEach((listener) => listener(active))
  }
}
