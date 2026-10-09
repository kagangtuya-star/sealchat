import type { StageSequenceTiming } from '../shared/stage-types'

const wait = (delayMs: number) => new Promise<void>((resolve) => setTimeout(resolve, delayMs))

export const STAGE_ACTION_CANCELLED = Symbol('stage-action-cancelled')
type StageActionSequenceResult = void | typeof STAGE_ACTION_CANCELLED

export const runStageActionSequence = async <T extends { timing: StageSequenceTiming }>(
  steps: readonly T[],
  execute: (step: T) => Promise<StageActionSequenceResult>,
  signal?: AbortSignal,
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
    if (signal?.aborted) return STAGE_ACTION_CANCELLED
    if (step.timing.mode === 'sync' && batch.length) {
      batch.push(execute(step))
      continue
    }
    if (await finishBatch()) return STAGE_ACTION_CANCELLED
    if (step.timing.mode === 'delay' && step.timing.delayMs > 0) {
      if (signal) await waitAbortable(step.timing.delayMs, signal)
      else await wait(step.timing.delayMs)
      if (signal?.aborted) return STAGE_ACTION_CANCELLED
    }
    batch.push(execute(step))
  }
  if (await finishBatch()) return STAGE_ACTION_CANCELLED
}

export const waitAbortable = (ms: number, signal: AbortSignal) => new Promise<void>((resolve) => {
  const finish = () => { clearTimeout(timer); signal.removeEventListener('abort', finish); resolve() }
  const timer = setTimeout(finish, Math.max(0, ms))
  signal.addEventListener('abort', finish, { once: true })
  if (signal.aborted) finish()
})
