import type {
  StageAction,
  StageAtomicAction,
  StageAtomicActionDescriptor,
  StageSequenceAction,
  StageSequenceStep,
  StageSequenceTiming,
} from './stage-types'
import { createDefaultStageActionSchedule, normalizeStageActionSchedule } from './stage-types'

export const STAGE_SEQUENCE_MAX_STEPS = 32
export const STAGE_SEQUENCE_MAX_DELAY_MS = 60_000
export const STAGE_RANDOM_TABLE_MAX_ENTRIES = 1_000
export const STAGE_RANDOM_TABLE_MAX_TEXT_LENGTH = 10_000

const stageSimpleDiceFormulaPattern = /^([1-9][0-9]*)d([1-9][0-9]*)(?:([+-])([0-9]+))?$/i

const id = (prefix: string) => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${value}`
}

export const isStageSequenceAction = (action: StageAction): action is StageSequenceAction => action.type === 'action.sequence'

export const createStageAtomicActionDescriptor = (
  type: StageAtomicAction['type'],
  sceneId: string,
  targetId = '',
): StageAtomicActionDescriptor => {
  if (type === 'chat.send') return { type, payload: { content: '舞台消息' } }
  if (type === 'chat.random-table') return {
    type,
    payload: {
      name: '随机表',
      formula: '1d6',
      entries: Array.from({ length: 6 }, (_, index) => ({
        min: index + 1,
        max: index + 1,
        text: `结果${index + 1}`,
      })),
    },
  }
  if (type === 'chat.insert') return { type, payload: { content: '舞台台词' } }
  if (type === 'scene.apply') return { type, payload: { sceneId } }
  if (type === 'effect.play') return { type, payload: { effectId: targetId } }
  return { type, payload: { objectId: targetId } }
}

export const createStageSequenceStep = (sceneId: string, objectId = ''): StageSequenceStep => ({
  id: id('step'),
  sceneId: sceneId || null,
  timing: { mode: 'after' },
  action: createStageAtomicActionDescriptor('object.toggle', sceneId, objectId),
})

export const createStageSequenceAction = (sceneId: string, objectId = ''): StageSequenceAction => ({
  id: id('action'),
  type: 'action.sequence',
  schedule: createDefaultStageActionSchedule(),
  payload: {
    version: 1,
    name: '点击动作组合',
    steps: [createStageSequenceStep(sceneId, objectId)],
  },
})

const normalizeTiming = (value: unknown): StageSequenceTiming => {
  if (!value || typeof value !== 'object') return { mode: 'after' }
  const timing = value as { mode?: unknown, delayMs?: unknown }
  if (timing.mode === 'sync') return { mode: 'sync' }
  if (timing.mode === 'delay') {
    const delayMs = Number(timing.delayMs)
    return {
      mode: 'delay',
      delayMs: Number.isFinite(delayMs)
        ? Math.min(STAGE_SEQUENCE_MAX_DELAY_MS, Math.max(0, Math.round(delayMs)))
        : 0,
    }
  }
  return { mode: 'after' }
}

export const normalizeStageRandomTablePayload = (value: unknown): Extract<StageAtomicAction, { type: 'chat.random-table' }>['payload'] | null => {
  if (!value || typeof value !== 'object') return null
  const payload = value as { name?: unknown, formula?: unknown, entries?: unknown }
  const name = typeof payload.name === 'string' ? payload.name.trim() : ''
  const formula = typeof payload.formula === 'string' ? payload.formula.replace(/\s+/g, '') : ''
  const formulaMatch = stageSimpleDiceFormulaPattern.exec(formula)
  if (!name || Array.from(name).length > 128 || !formulaMatch) return null
  const diceCount = Number(formulaMatch[1])
  const diceSides = Number(formulaMatch[2])
  const modifier = Number(formulaMatch[4] || 0) * (formulaMatch[3] === '-' ? -1 : 1)
  if (diceCount > 100 || diceSides > 100_000 || Math.abs(modifier) > 1_000_000) return null
  if (!Array.isArray(payload.entries) || !payload.entries.length || payload.entries.length > STAGE_RANDOM_TABLE_MAX_ENTRIES) return null
  let totalTextLength = 0
  const entries: Array<{ min: number, max: number, text: string }> = []
  for (const value of payload.entries) {
    if (!value || typeof value !== 'object') return null
    const entry = value as { min?: unknown, max?: unknown, text?: unknown }
    const text = typeof entry.text === 'string' ? entry.text.trim() : ''
    if (typeof entry.min !== 'number' || typeof entry.max !== 'number' || !Number.isSafeInteger(entry.min) || !Number.isSafeInteger(entry.max) || entry.min > entry.max || !text) return null
    totalTextLength += Array.from(text).length
    if (totalTextLength > STAGE_RANDOM_TABLE_MAX_TEXT_LENGTH) return null
    entries.push({ min: Number(entry.min), max: Number(entry.max), text })
  }
  const ordered = [...entries].sort((left, right) => left.min - right.min || left.max - right.max)
  if (ordered.some((entry, index) => index > 0 && entry.min <= ordered[index - 1]!.max)) return null
  const minimumRoll = diceCount + modifier
  const maximumRoll = diceCount * diceSides + modifier
  let coveredThrough = minimumRoll - 1
  for (const entry of ordered) {
    if (entry.max < minimumRoll) continue
    if (entry.min > coveredThrough + 1) break
    coveredThrough = Math.max(coveredThrough, entry.max)
    if (coveredThrough >= maximumRoll) break
  }
  if (coveredThrough < maximumRoll) return null
  return { name, formula, entries }
}

export const rollStageRandomTable = (
  value: unknown,
  random: () => number = Math.random,
): { result: number, content: string } | null => {
  const payload = normalizeStageRandomTablePayload(value)
  if (!payload) return null
  const formulaMatch = stageSimpleDiceFormulaPattern.exec(payload.formula)
  if (!formulaMatch) return null
  const diceCount = Number(formulaMatch[1])
  const diceSides = Number(formulaMatch[2])
  const modifier = Number(formulaMatch[4] || 0) * (formulaMatch[3] === '-' ? -1 : 1)
  let result = modifier
  for (let index = 0; index < diceCount; index += 1) {
    const sample = Math.min(1 - Number.EPSILON, Math.max(0, random()))
    result += Math.floor(sample * diceSides) + 1
  }
  const matched = payload.entries.find((entry) => result >= entry.min && result <= entry.max)
  if (!matched) return null
  return {
    result,
    content: `${payload.name}\n${payload.formula} = ${result}\n${matched.text}`,
  }
}

const normalizeAtomicDescriptor = (value: unknown): StageAtomicActionDescriptor | null => {
  if (!value || typeof value !== 'object') return null
  const action = value as { type?: unknown, payload?: Record<string, unknown> }
  if (!action.payload || typeof action.payload !== 'object') return null
  if (action.type === 'chat.send') {
    const content = typeof action.payload.content === 'string' ? action.payload.content : ''
    if (!content || content.length > 10_000) return null
    return {
      type: action.type,
      payload: {
        content,
        ...(typeof action.payload.channelId === 'string' && action.payload.channelId.trim()
          ? { channelId: action.payload.channelId.trim() }
          : {}),
        ...(typeof action.payload.characterId === 'string' && action.payload.characterId.trim()
          ? { characterId: action.payload.characterId.trim() }
          : {}),
      },
    }
  }
  if (action.type === 'chat.random-table') {
    const payload = normalizeStageRandomTablePayload(action.payload)
    return payload ? { type: action.type, payload } : null
  }
  if (action.type === 'chat.insert') {
    const content = typeof action.payload.content === 'string' ? action.payload.content : ''
    return content && content.length <= 10_000 ? { type: action.type, payload: { content } } : null
  }
  if (action.type === 'scene.apply') {
    const sceneId = typeof action.payload.sceneId === 'string' ? action.payload.sceneId.trim() : ''
    return sceneId ? { type: action.type, payload: { sceneId } } : null
  }
  if (action.type === 'effect.play') {
    const effectId = typeof action.payload.effectId === 'string' ? action.payload.effectId.trim() : ''
    return effectId ? { type: action.type, payload: { effectId } } : null
  }
  if (action.type === 'object.toggle') {
    const objectId = typeof action.payload.objectId === 'string' ? action.payload.objectId.trim() : ''
    return objectId ? { type: action.type, payload: { objectId } } : null
  }
  return null
}

export const normalizeStageSequenceAction = (value: unknown): StageSequenceAction | null => {
  if (!value || typeof value !== 'object') return null
  const action = value as { id?: unknown, type?: unknown, schedule?: unknown, payload?: Record<string, unknown> }
  const actionId = typeof action.id === 'string' ? action.id.trim() : ''
  if (!actionId || action.type !== 'action.sequence' || !action.payload || action.payload.version !== 1) return null
  const rawSteps = Array.isArray(action.payload.steps) ? action.payload.steps : []
  const seen = new Set<string>()
  const steps = rawSteps.reduce<StageSequenceStep[]>((result, raw) => {
    if (result.length >= STAGE_SEQUENCE_MAX_STEPS || !raw || typeof raw !== 'object') return result
    const step = raw as { id?: unknown, sceneId?: unknown, timing?: unknown, action?: unknown }
    const stepId = typeof step.id === 'string' ? step.id.trim() : ''
    const descriptor = normalizeAtomicDescriptor(step.action)
    if (!stepId || seen.has(stepId) || !descriptor) return result
    seen.add(stepId)
    result.push({
      id: stepId,
      sceneId: typeof step.sceneId === 'string' && step.sceneId.trim() ? step.sceneId.trim() : null,
      timing: normalizeTiming(step.timing),
      action: descriptor,
    })
    return result
  }, [])
  return {
    id: actionId,
    type: 'action.sequence',
    schedule: normalizeStageActionSchedule(action.schedule),
    payload: {
      version: 1,
      name: typeof action.payload.name === 'string'
        ? Array.from(action.payload.name.trim() || '点击动作组合').slice(0, 128).join('')
        : '点击动作组合',
      steps,
    },
  }
}

export const sequenceStepAction = (step: StageSequenceStep): StageAtomicAction => ({
  id: step.id,
  schedule: createDefaultStageActionSchedule(),
  ...step.action,
} as StageAtomicAction)
