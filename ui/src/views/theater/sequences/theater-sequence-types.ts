import { normalizeStageSequenceSteps } from '../shared/stage-actions'
import type { StageAtomicAction, StageSequenceStep } from '../shared/stage-types'

// Scene-level sequences live in StageLiveState.serverState under this key.
export const THEATER_SEQUENCES_STATE_KEY = 'theaterSequences'
export const THEATER_SEQUENCE_MAX_COUNT = 64
export const THEATER_SEQUENCE_MAX_TRIGGERS = 32
export const THEATER_SEQUENCE_MAX_KEYWORDS = 32
export const THEATER_SEQUENCE_MAX_HIT_INTERVAL = 65_535
export const THEATER_SEQUENCE_MAX_COOLDOWN_MS = 300_000
export const THEATER_SEQUENCE_MAX_LOOP_COUNT = 100

// First version only exposes idempotent, multi-client safe step actions.
export const theaterSequenceStepActionTypes = ['effect.play', 'scene.apply'] as const satisfies readonly StageAtomicAction['type'][]
export type TheaterSequenceStepActionType = typeof theaterSequenceStepActionTypes[number]

interface TheaterSequenceTriggerCounter {
  id: string
  // Fire on hit N >= threshold where (N - threshold) % every === 0.
  threshold: number
  every: number
  cooldownMs: number
}

export interface TheaterSequenceMessageTrigger extends TheaterSequenceTriggerCounter {
  type: 'message'
  keywords: string[]
  targetActorName: string | null
}

export interface TheaterSequenceComponentClickTrigger extends TheaterSequenceTriggerCounter {
  type: 'component.click'
  objectId: string
}

export type TheaterSequenceTrigger = TheaterSequenceMessageTrigger | TheaterSequenceComponentClickTrigger
export type TheaterSequenceTriggerType = TheaterSequenceTrigger['type']

export interface TheaterSequence {
  version: 1
  id: string
  name: string
  enabled: boolean
  triggers: TheaterSequenceTrigger[]
  loopCount: number
  steps: StageSequenceStep[]
}

const createId = (prefix: string) => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${value}`
}

const truncate = (value: string, maximum: number) => Array.from(value).slice(0, maximum).join('')

const normalizeId = (value: unknown) => (
  typeof value === 'string' ? truncate(value.trim(), 128) : ''
)

const integerRange = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, Math.round(value)))
    : fallback
)

const isRecord = (value: unknown): value is Record<string, unknown> => (
  Boolean(value) && typeof value === 'object' && !Array.isArray(value)
)

export const isTheaterSequenceStepActionType = (type: string): type is TheaterSequenceStepActionType => (
  (theaterSequenceStepActionTypes as readonly string[]).includes(type)
)

export const createTheaterSequenceTrigger = (
  type: TheaterSequenceTriggerType,
  objectId = '',
): TheaterSequenceTrigger => {
  const counter = { id: createId('trigger'), threshold: 1, every: 1, cooldownMs: 0 }
  return type === 'component.click'
    ? { ...counter, type, objectId }
    : { ...counter, type, keywords: [], targetActorName: null }
}

export const createDefaultTheaterSequence = (): TheaterSequence => ({
  version: 1,
  id: createId('sequence'),
  name: '新序列',
  enabled: true,
  triggers: [createTheaterSequenceTrigger('message')],
  loopCount: 1,
  steps: [],
})

const normalizeTrigger = (input: unknown): TheaterSequenceTrigger | null => {
  if (!isRecord(input)) return null
  const id = normalizeId(input.id)
  if (!id) return null
  const counter = {
    id,
    threshold: integerRange(input.threshold, 1, 1, THEATER_SEQUENCE_MAX_HIT_INTERVAL),
    every: integerRange(input.every, 1, 1, THEATER_SEQUENCE_MAX_HIT_INTERVAL),
    cooldownMs: integerRange(input.cooldownMs, 0, 0, THEATER_SEQUENCE_MAX_COOLDOWN_MS),
  }
  if (input.type === 'component.click') {
    const objectId = normalizeId(input.objectId)
    return objectId ? { ...counter, type: 'component.click', objectId } : null
  }
  if (input.type !== 'message') return null
  const keywords = Array.isArray(input.keywords)
    ? [...new Set(input.keywords
      .filter((item): item is string => typeof item === 'string')
      .map((item) => truncate(item.trim(), 128))
      .filter(Boolean))].slice(0, THEATER_SEQUENCE_MAX_KEYWORDS)
    : []
  const targetActorName = typeof input.targetActorName === 'string' && input.targetActorName.trim()
    ? input.targetActorName.trim().slice(0, 512)
    : null
  return { ...counter, type: 'message', keywords, targetActorName }
}

export const normalizeTheaterSequence = (input: unknown): TheaterSequence | null => {
  if (!isRecord(input)) return null
  const id = normalizeId(input.id)
  if (!id) return null
  const seenTriggers = new Set<string>()
  const triggers = (Array.isArray(input.triggers) ? input.triggers : []).flatMap((raw) => {
    const trigger = normalizeTrigger(raw)
    if (!trigger || seenTriggers.has(trigger.id)) return []
    seenTriggers.add(trigger.id)
    return [trigger]
  }).slice(0, THEATER_SEQUENCE_MAX_TRIGGERS)
  const steps = normalizeStageSequenceSteps(input.steps)
    .filter((step) => isTheaterSequenceStepActionType(step.action.type))
  const name = typeof input.name === 'string' ? truncate(input.name.trim(), 128) : ''
  return {
    version: 1,
    id,
    name: name || '未命名序列',
    enabled: input.enabled !== false,
    triggers,
    loopCount: integerRange(input.loopCount, 1, 1, THEATER_SEQUENCE_MAX_LOOP_COUNT),
    steps,
  }
}

export const normalizeTheaterSequences = (input: unknown): TheaterSequence[] => {
  if (!Array.isArray(input)) return []
  const seen = new Set<string>()
  return input.flatMap((raw) => {
    const sequence = normalizeTheaterSequence(raw)
    if (!sequence || seen.has(sequence.id)) return []
    seen.add(sequence.id)
    return [sequence]
  }).slice(0, THEATER_SEQUENCE_MAX_COUNT)
}

export const theaterSequencesFromServerState = (serverState: Record<string, unknown> | undefined) => (
  normalizeTheaterSequences(serverState?.[THEATER_SEQUENCES_STATE_KEY])
)
