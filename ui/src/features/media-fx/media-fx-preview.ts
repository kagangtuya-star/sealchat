import {
  createDefaultMediaFxAdvanced,
  createDefaultMediaFxFilter,
  createDefaultMediaFxMotion,
  createDefaultMediaFxTemporal,
  mediaFxAdvancedKeys,
  mediaFxFilterKeys,
  normalizeMediaFxSpec,
  type MediaFxSpec,
} from './media-fx'

// Static preview helpers for editors that show a rasterized "final look" next to the
// editing surface. Only filter / advanced change static pixels; motion is played on the
// preview element by the DOM adapter and temporal runs live over the static raster, so
// motion / temporal edits never trigger a re-raster.

export interface MediaFxStaticPreviewOptions {
  filters?: boolean
  advanced?: boolean
}

// Static part of a spec as the target renderer will draw it. Disabled sections fall
// back to defaults (the stored data itself is untouched); motion and temporal are
// always reset.
export const mediaFxStaticPreviewSpec = (input: unknown, options: MediaFxStaticPreviewOptions = {}): MediaFxSpec => {
  const spec = normalizeMediaFxSpec(input)
  return {
    ...spec,
    motion: createDefaultMediaFxMotion(),
    temporal: createDefaultMediaFxTemporal(),
    filter: options.filters === false ? createDefaultMediaFxFilter() : spec.filter,
    advanced: options.advanced === true ? spec.advanced : createDefaultMediaFxAdvanced(),
  }
}

export const mediaFxStaticSignature = (input: unknown) => {
  const spec = normalizeMediaFxSpec(input)
  return [
    ...mediaFxFilterKeys.map((key) => spec.filter[key]),
    ...mediaFxAdvancedKeys.map((key) => spec.advanced[key]),
  ].join('|')
}

export interface MediaFxPreviewSchedulerOptions<T> {
  delayMs: number
  render: () => Promise<T | null>
  commit: (result: T) => void
  // Releases a result that was produced for an outdated request.
  discard?: (result: T) => void
  onError?: (error: unknown) => void
  setTimer?: (callback: () => void, delayMs: number) => unknown
  clearTimer?: (handle: unknown) => void
}

export interface MediaFxPreviewScheduler {
  // Coalesces requests; a key equal to the last requested one is ignored.
  request(key: string): void
  // Drops pending / in-flight work and forgets the last key (e.g. preview hidden).
  invalidate(): void
  dispose(): void
}

// Debounced, generation-guarded single-flight renderer: at most one raster job runs
// at a time. While it is running, newer requests collapse to the latest key; stale
// results are discarded and the latest debounced request starts as soon as possible.
export const createMediaFxPreviewScheduler = <T>(options: MediaFxPreviewSchedulerOptions<T>): MediaFxPreviewScheduler => {
  const setTimer = options.setTimer ?? ((callback: () => void, delayMs: number) => setTimeout(callback, delayMs))
  const clearTimer = options.clearTimer ?? ((handle: unknown) => clearTimeout(handle as ReturnType<typeof setTimeout>))
  let generation = 0
  let lastKey: string | null = null
  let timer: unknown = null
  let running = false
  let queued = false
  let disposed = false

  const cancelTimer = () => {
    if (timer === null) return
    clearTimer(timer)
    timer = null
  }

  const run = async () => {
    timer = null
    if (disposed || running) {
      if (!disposed) queued = true
      return
    }
    running = true
    queued = false
    const job = generation
    let result: T | null = null
    try {
      result = await options.render()
    } catch (error) {
      if (!disposed && job === generation) {
        lastKey = null
        options.onError?.(error)
      }
    } finally {
      running = false
    }

    if (result !== null) {
      if (disposed || job !== generation) options.discard?.(result)
      else options.commit(result)
    } else if (!disposed && job === generation) {
      // A transient empty render should not permanently suppress retries for this key.
      lastKey = null
    }

    if (!disposed && queued) {
      queued = false
      void run()
    }
  }

  const schedule = () => {
    cancelTimer()
    queued = false
    timer = setTimer(() => {
      timer = null
      if (running) {
        queued = true
        return
      }
      void run()
    }, options.delayMs)
  }

  return {
    request(key) {
      if (disposed || key === lastKey) return
      lastKey = key
      // Invalidate an in-flight job immediately; its result is stale from now on.
      generation += 1
      schedule()
    },
    invalidate() {
      generation += 1
      lastKey = null
      queued = false
      cancelTimer()
    },
    dispose() {
      if (disposed) return
      disposed = true
      generation += 1
      queued = false
      cancelTimer()
    },
  }
}
