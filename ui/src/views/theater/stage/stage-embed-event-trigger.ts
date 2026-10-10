import { stageActionSchema } from '../bridge/theater-bridge-protocol'
import {
  resolveStageEmbedEventActions,
  type StageActionTriggeredPayload,
  type StageEmbedEventPublished,
  type StageObject,
} from '../shared/stage-types'

export interface StageEmbedEventTriggerOptions {
  getObject: (objectId: string) => StageObject | null | undefined
  // Permission, visibility, interactivity and local stage gating.
  canRun: (object: StageObject) => boolean
  executionId: () => string
  emit: (payload: StageActionTriggeredPayload) => void
}

const HANDLED_EVENT_LIMIT = 256

// Events reach this trigger only from the publishing iframe object's own embed
// host after the server accepted events.publish; gateway broadcasts seen by
// other clients never do. Each eventId starts at most one execution, and the
// object's saved binding selects the actions — the event payload is never read.
export const createStageEmbedEventTrigger = (options: StageEmbedEventTriggerOptions) => {
  const handled = new Set<string>()
  return (event: StageEmbedEventPublished) => {
    if (!event.eventId || handled.has(event.eventId)) return 0
    handled.add(event.eventId)
    if (handled.size > HANDLED_EVENT_LIMIT) {
      const oldest = handled.values().next().value
      if (oldest) handled.delete(oldest)
    }
    const object = options.getObject(event.objectId)
    if (!object || object.type !== 'iframe' || !options.canRun(object)) return 0
    const actions = resolveStageEmbedEventActions(object, event.topic).flatMap((action) => {
      const parsed = stageActionSchema.safeParse(action)
      return parsed.success ? [parsed.data] : []
    })
    if (!actions.length) return 0
    const execution = {
      id: options.executionId(),
      mode: object.metadata.actionExecutionMode === 'sequential' ? 'sequential' as const : 'parallel' as const,
      total: actions.length,
    }
    actions.forEach((action, index) => {
      options.emit({
        objectId: object.id, actionId: action.id, action, execution: { ...execution, index },
        embedEvent: { eventId: event.eventId, formId: event.formId, topic: event.topic },
      })
    })
    return actions.length
  }
}
