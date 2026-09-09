import type { StageSequenceStep } from '../shared/stage-types'

const wait = (delayMs: number) => new Promise<void>((resolve) => setTimeout(resolve, delayMs))

export const STAGE_ACTION_CANCELLED = Symbol('stage-action-cancelled')
type StageActionSequenceResult = void | typeof STAGE_ACTION_CANCELLED

export const runStageActionSequence = async (
  steps: readonly StageSequenceStep[],
  execute: (step: StageSequenceStep) => Promise<StageActionSequenceResult>,
): Promise<StageActionSequenceResult> => {
  let batch: Promise<StageActionSequenceResult>[] = []
  const finishBatch = async () => {
    if (!batch.length) return false
    const current = batch
    batch = []
    const results = await Promise.all(current)
    return results.some((result) => result === STAGE_ACTION_CANCELLED)
  }

  for (const step of steps) {
    if (step.timing.mode === 'sync' && batch.length) {
      batch.push(execute(step))
      continue
    }
    if (await finishBatch()) return STAGE_ACTION_CANCELLED
    if (step.timing.mode === 'delay' && step.timing.delayMs > 0) await wait(step.timing.delayMs)
    batch.push(execute(step))
  }
  if (await finishBatch()) return STAGE_ACTION_CANCELLED
}
