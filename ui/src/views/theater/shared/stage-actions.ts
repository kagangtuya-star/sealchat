import type {
  StageAction,
  StageAtomicAction,
  StageAtomicActionDescriptor,
  StageSequenceAction,
  StageSequenceStep,
  StageSequenceTiming,
} from './stage-types'

export const STAGE_SEQUENCE_MAX_STEPS = 32
export const STAGE_SEQUENCE_MAX_DELAY_MS = 60_000

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
  objectId = '',
): StageAtomicActionDescriptor => {
  if (type === 'chat.send') return { type, payload: { content: '舞台消息' } }
  if (type === 'chat.insert') return { type, payload: { content: '舞台台词' } }
  if (type === 'scene.apply') return { type, payload: { sceneId } }
  return { type, payload: { objectId } }
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
  if (action.type === 'chat.insert') {
    const content = typeof action.payload.content === 'string' ? action.payload.content : ''
    return content && content.length <= 10_000 ? { type: action.type, payload: { content } } : null
  }
  if (action.type === 'scene.apply') {
    const sceneId = typeof action.payload.sceneId === 'string' ? action.payload.sceneId.trim() : ''
    return sceneId ? { type: action.type, payload: { sceneId } } : null
  }
  if (action.type === 'object.toggle') {
    const objectId = typeof action.payload.objectId === 'string' ? action.payload.objectId.trim() : ''
    return objectId ? { type: action.type, payload: { objectId } } : null
  }
  return null
}

export const normalizeStageSequenceAction = (value: unknown): StageSequenceAction | null => {
  if (!value || typeof value !== 'object') return null
  const action = value as { id?: unknown, type?: unknown, payload?: Record<string, unknown> }
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
  ...step.action,
} as StageAtomicAction)
