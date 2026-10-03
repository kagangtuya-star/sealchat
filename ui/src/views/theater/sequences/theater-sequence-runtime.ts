import type { TheaterDialogueRuntime, TheaterDialogueRuntimeSnapshot } from '../dialogue/theater-dialogue-runtime'
import { theaterEffectMatchesMessage } from '../effects/theater-effect-runtime'
import type { TheaterDialogueMessage } from '../bridge/theater-dialogue-queue'
import type { TheaterSequence, TheaterSequenceTrigger } from './theater-sequence-types'

interface TheaterSequenceRuntimeOptions {
  dialogueRuntime: Pick<TheaterDialogueRuntime, 'subscribe'>
  getSequences: () => readonly TheaterSequence[]
  onTrigger: (sequenceId: string, triggerId: string) => void
  now?: () => number
}

interface TheaterSequenceTriggerState {
  count: number
  lastTriggeredAt: number | null
  fingerprint: string
}

const triggerStateKey = (sequenceId: string, triggerId: string) => `${sequenceId}\u0000${triggerId}`

const triggerFingerprint = (trigger: TheaterSequenceTrigger): string => JSON.stringify(trigger.type === 'message'
  ? [trigger.type, trigger.keywords, trigger.targetActorName, trigger.threshold, trigger.every, trigger.cooldownMs]
  : [trigger.type, trigger.objectId, trigger.threshold, trigger.every, trigger.cooldownMs])

// Hit counts, cooldowns and the current scope are runtime-only and never persisted.
export class TheaterSequenceRuntime {
  private readonly triggerStates = new Map<string, TheaterSequenceTriggerState>()
  private scopeId: string | null = null
  private currentMessageId = ''
  private initialSnapshot = true
  private unsubscribeDialogue: (() => void) | null = null
  private disposed = false

  constructor(private readonly options: TheaterSequenceRuntimeOptions) {
    this.unsubscribeDialogue = options.dialogueRuntime.subscribe(this.handleDialogueSnapshot)
    this.initialSnapshot = false
  }

  notifyComponentClick = (objectId: string) => {
    if (this.disposed || !objectId) return
    this.evaluate((trigger) => trigger.type === 'component.click' && trigger.objectId === objectId)
  }

  // A scope change drops every counter; otherwise prune removed, disabled or changed triggers.
  reconcile = (scopeId: string | null) => {
    if (this.disposed) return
    if (scopeId !== this.scopeId) {
      this.scopeId = scopeId
      this.triggerStates.clear()
      return
    }
    const fingerprints = new Map<string, string>()
    this.options.getSequences().forEach((sequence) => {
      if (!sequence.enabled) return
      sequence.triggers.forEach((trigger) => {
        fingerprints.set(triggerStateKey(sequence.id, trigger.id), triggerFingerprint(trigger))
      })
    })
    for (const [key, state] of this.triggerStates) {
      if (fingerprints.get(key) !== state.fingerprint) this.triggerStates.delete(key)
    }
  }

  dispose = () => {
    if (this.disposed) return
    this.disposed = true
    this.unsubscribeDialogue?.()
    this.unsubscribeDialogue = null
    this.triggerStates.clear()
  }

  private readonly handleDialogueSnapshot = (snapshot: TheaterDialogueRuntimeSnapshot) => {
    if (this.disposed) return
    const message = snapshot.queue.current?.message
    if (!message) {
      this.currentMessageId = ''
      return
    }
    if (message.messageId === this.currentMessageId) return
    this.currentMessageId = message.messageId
    // Do not count a message that was already on screen before this runtime existed.
    if (this.initialSnapshot) return
    this.handleMessage(message)
  }

  private handleMessage(message: TheaterDialogueMessage) {
    this.evaluate((trigger) => trigger.type === 'message' && theaterEffectMatchesMessage(trigger, message))
  }

  private evaluate(matches: (trigger: TheaterSequenceTrigger) => boolean) {
    const now = this.options.now?.() ?? Date.now()
    const fired: Array<{ sequenceId: string, triggerId: string }> = []
    this.options.getSequences().forEach((sequence) => {
      if (!sequence.enabled) return
      let sequenceFired = false
      sequence.triggers.forEach((trigger) => {
        if (!matches(trigger)) return
        const key = triggerStateKey(sequence.id, trigger.id)
        const fingerprint = triggerFingerprint(trigger)
        const previousState = this.triggerStates.get(key)
        const state = previousState?.fingerprint === fingerprint
          ? previousState
          : { count: 0, lastTriggeredAt: null, fingerprint }
        state.count = Math.min(Number.MAX_SAFE_INTEGER, state.count + 1)
        this.triggerStates.set(key, state)
        const due = state.count >= trigger.threshold && (state.count - trigger.threshold) % trigger.every === 0
        if (!due) return
        if (state.lastTriggeredAt !== null && now - state.lastTriggeredAt < trigger.cooldownMs) return
        // Triggers are OR-ed: one event starts a sequence at most once.
        if (sequenceFired) return
        state.lastTriggeredAt = now
        sequenceFired = true
        fired.push({ sequenceId: sequence.id, triggerId: trigger.id })
      })
    })
    fired.forEach(({ sequenceId, triggerId }) => {
      if (!this.disposed) this.options.onTrigger(sequenceId, triggerId)
    })
  }
}
