import type { StageSequenceStep } from '../shared/stage-types'

const wait = (delayMs: number) => new Promise<void>((resolve) => setTimeout(resolve, delayMs))

export const runStageActionSequence = async (
  steps: readonly StageSequenceStep[],
  execute: (step: StageSequenceStep) => Promise<void>,
) => {
  let batch: Promise<void>[] = []
  const finishBatch = async () => {
    if (!batch.length) return
    const current = batch
    batch = []
    await Promise.all(current)
  }

  for (const step of steps) {
    if (step.timing.mode === 'sync' && batch.length) {
      batch.push(execute(step))
      continue
    }
    await finishBatch()
    if (step.timing.mode === 'delay' && step.timing.delayMs > 0) await wait(step.timing.delayMs)
    batch.push(execute(step))
  }
  await finishBatch()
}
