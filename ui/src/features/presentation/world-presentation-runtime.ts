import { TheaterEffectRuntime } from '@/views/theater/effects/theater-effect-runtime'
import { TheaterSequenceRuntime } from '@/views/theater/sequences/theater-sequence-runtime'
import type { TheaterSequence } from '@/views/theater/sequences/theater-sequence-types'
import { createPresentationMessageSource } from '@/views/theater/shared/presentation-message-source'
import type { StageSequenceStep } from '@/views/theater/shared/stage-types'
import { runStageActionSequence, STAGE_ACTION_CANCELLED } from '@/views/theater/stage/theater-action-sequence-runtime'
import type { WorldPresentationConfig } from './world-presentation-config'

export type WorldPresentationTarget = 'effect' | 'sequence'

/** Channel live control handed to the console; config is the last saved one (null while it loads). */
export interface WorldPresentationLiveControl {
  config: WorldPresentationConfig | null
  failed: boolean
  play: (kind: WorldPresentationTarget, id: string) => Promise<boolean>
  retry: () => void
}

type EffectStep = StageSequenceStep & { action: Extract<StageSequenceStep['action'], { type: 'effect.play' }> }

export const isWorldEffectStep = (step: StageSequenceStep): step is EffectStep => step.action.type === 'effect.play' && !step.sceneId

export const canPlayWorldSequence = (sequence: TheaterSequence, config: WorldPresentationConfig) => {
  const enabledEffects = new Set(config.effects.filter((effect) => effect.visible).map((effect) => effect.id))
  return sequence.enabled && sequence.steps.some((step) => isWorldEffectStep(step) && enabledEffects.has(step.action.payload.effectId))
}

export const createWorldPresentationAuthority = (
  getConfig: () => WorldPresentationConfig,
  publish: (kind: WorldPresentationTarget, targetId: string, messageId: string) => void,
) => {
  const messages = createPresentationMessageSource()
  const effects = new TheaterEffectRuntime({
    messageSource: messages,
    getObjects: () => getConfig().effects,
    onMessageTrigger: (id, messageId) => publish('effect', id, messageId),
  })
  const sequences = new TheaterSequenceRuntime({
    messageSource: messages,
    getSequences: () => getConfig().sequences,
    onTrigger: (id, _, messageId) => {
      const config = getConfig()
      const sequence = config.sequences.find((item) => item.id === id)
      if (messageId && sequence && canPlayWorldSequence(sequence, config)) publish('sequence', id, messageId)
    },
  })
  return {
    receive: messages.publish,
    dispose() { effects.dispose(); sequences.dispose() },
  }
}

export const playWorldSequence = async (
  sequence: TheaterSequence,
  executionId: string,
  play: (effectId: string, triggerId: string) => void,
  signal: AbortSignal,
) => {
  const steps = sequence.steps.filter(isWorldEffectStep)
  if (!sequence.enabled || !steps.length) return
  for (let loop = 0; loop < sequence.loopCount && !signal.aborted; loop += 1) {
    const outcome = await runStageActionSequence(steps, async (step) => {
      if (signal.aborted) return STAGE_ACTION_CANCELLED
      play(step.action.payload.effectId, `${executionId}:${loop}:${step.id}`)
    }, signal)
    if (outcome === STAGE_ACTION_CANCELLED) return
  }
}
