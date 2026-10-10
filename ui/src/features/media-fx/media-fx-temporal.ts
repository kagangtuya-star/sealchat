import { createDefaultMediaFxTemporal, mediaFxTemporalHasContent, normalizeMediaFxTemporal, type MediaFxTemporal } from './media-fx'
import {
  createMediaFxTemporalGpuSession,
  mediaFxGpuSupported,
  type MediaFxTemporalBaseline,
  type MediaFxTemporalGpuFrame,
  type MediaFxTemporalGpuSession,
} from './media-fx-gpu'

// Temporal Media FX runtime: one page-wide clock and a small per-visual player.
//
// The clock is the only requestAnimationFrame loop of Media FX. It runs while at least
// one player is subscribed and the page is visible, delivers at most ~30 ticks per
// second, and never replays missed frames: a tick only carries the elapsed monotonic
// time since the previous tick (capped, and 0 after the page was hidden).
//
// A player renders live frames of one visual over its static baseline through a shared
// GPU session (media-fx-gpu) and hands every frame to the consumer's `present`. It only
// subscribes to the clock while it can actually draw something: active, a baseline is
// set, at least one temporal effect is on, the GPU path is usable and no frame failed.
// Changing temporal values never touches the baseline. When live output stops for any
// reason the consumer is told to show its static baseline again.

export const MEDIA_FX_TEMPORAL_MAX_FPS = 30
// A stalled tab or a long task must not jump the effects forward.
const MAX_TICK_DELTA_SECONDS = 0.25
// A 60 Hz display delivers frames every ~16.7 ms; a small tolerance keeps 30 fps there.
const FRAME_INTERVAL_TOLERANCE_MS = 2

export interface MediaFxTemporalTick {
  // Monotonic clock time in seconds.
  time: number
  // Seconds since the previous tick of this clock (0 for the first tick after a start
  // or a resume, capped otherwise).
  delta: number
}

export type MediaFxTemporalListener = (tick: MediaFxTemporalTick) => void

export interface MediaFxTemporalClockEnv {
  requestFrame: (callback: (timestamp: number) => void) => unknown
  cancelFrame: (handle: unknown) => void
  isHidden: () => boolean
  // Registers a visibility change listener and returns its cleanup.
  onVisibilityChange: (listener: () => void) => () => void
}

export interface MediaFxTemporalClock {
  subscribe(listener: MediaFxTemporalListener): () => void
  // True while a frame request is pending.
  readonly running: boolean
  readonly subscribers: number
}

export const createMediaFxTemporalClock = (
  env: MediaFxTemporalClockEnv,
  maxFps = MEDIA_FX_TEMPORAL_MAX_FPS,
): MediaFxTemporalClock => {
  const listeners = new Set<MediaFxTemporalListener>()
  const minIntervalMs = 1000 / maxFps - FRAME_INTERVAL_TOLERANCE_MS
  let handle: unknown = null
  let lastTickAt: number | null = null
  let removeVisibilityListener: (() => void) | null = null

  const cancel = () => {
    if (handle === null) return
    env.cancelFrame(handle)
    handle = null
  }

  const schedule = () => {
    if (handle !== null || !listeners.size || env.isHidden()) return
    handle = env.requestFrame(onFrame)
  }

  function onFrame(timestamp: number) {
    handle = null
    if (!listeners.size || env.isHidden()) return
    if (lastTickAt === null || timestamp - lastTickAt >= minIntervalMs) {
      const delta = lastTickAt === null ? 0 : Math.min(MAX_TICK_DELTA_SECONDS, Math.max(0, (timestamp - lastTickAt) / 1000))
      lastTickAt = timestamp
      const tick = { time: timestamp / 1000, delta }
      for (const listener of [...listeners]) {
        try {
          listener(tick)
        } catch {
          // One failing visual must not stop the others.
        }
      }
    }
    schedule()
  }

  const handleVisibilityChange = () => {
    if (env.isHidden()) {
      cancel()
      // Resume from "now": hidden time is neither replayed nor added to the effects.
      lastTickAt = null
      return
    }
    schedule()
  }

  return {
    subscribe(listener) {
      listeners.add(listener)
      removeVisibilityListener ??= env.onVisibilityChange(handleVisibilityChange)
      schedule()
      return () => {
        if (!listeners.delete(listener) || listeners.size) return
        cancel()
        lastTickAt = null
        removeVisibilityListener?.()
        removeVisibilityListener = null
      }
    },
    get running() {
      return handle !== null
    },
    get subscribers() {
      return listeners.size
    },
  }
}

const browserClockEnv = (): MediaFxTemporalClockEnv => ({
  requestFrame: (callback) => window.requestAnimationFrame(callback),
  cancelFrame: (handle) => window.cancelAnimationFrame(handle as number),
  isHidden: () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  onVisibilityChange: (listener) => {
    if (typeof document === 'undefined') return () => {}
    document.addEventListener('visibilitychange', listener)
    return () => document.removeEventListener('visibilitychange', listener)
  },
})

let pageClock: MediaFxTemporalClock | null = null

// The page-wide clock shared by every Media FX consumer (Stage, Surface,
// TheaterPresentation, editor previews). Created lazily; it owns no RAF until a player
// subscribes.
export const mediaFxTemporalClock = (): MediaFxTemporalClock => {
  pageClock ??= createMediaFxTemporalClock(browserClockEnv())
  return pageClock
}

// ---------------------------------------------------------------------------
// Player
// ---------------------------------------------------------------------------

export interface MediaFxTemporalPlayerOptions {
  // Copies a finished frame into the consumer's output right away; false when it cannot.
  present: (frame: MediaFxTemporalGpuFrame) => boolean
  // true once a live frame is shown; false when live output stops (inactive, cleared
  // baseline or a failure) and the consumer must show its static baseline again.
  onLiveChange?: (live: boolean) => void
  // Cheap per-tick visibility gate (e.g. a Konva node hidden by an ancestor). A tick it
  // rejects does no GPU work.
  canRender?: () => boolean
  clock?: MediaFxTemporalClock
  session?: MediaFxTemporalGpuSession
  gpuSupported?: () => boolean
}

export interface MediaFxTemporalPlayerInput {
  temporal: MediaFxTemporal | null | undefined
  // Consumer-side gate: capability, reduced motion, pause, visibility, ...
  active: boolean
}

export interface MediaFxTemporalPlayer {
  // A new baseline object means new static pixels (one upload on the next frame); the
  // same object keeps the uploaded texture.
  setBaseline(baseline: MediaFxTemporalBaseline | null): void
  update(input: MediaFxTemporalPlayerInput): void
  readonly live: boolean
  readonly running: boolean
  dispose(): void
}

export const createMediaFxTemporalPlayer = (options: MediaFxTemporalPlayerOptions): MediaFxTemporalPlayer => {
  const clock = options.clock ?? mediaFxTemporalClock()
  const session = options.session ?? createMediaFxTemporalGpuSession()
  const gpuSupported = options.gpuSupported ?? mediaFxGpuSupported
  let temporal = createDefaultMediaFxTemporal()
  let active = false
  let baseline: MediaFxTemporalBaseline | null = null
  let failed = false
  let live = false
  let unsubscribe: (() => void) | null = null
  // Speed-scaled temporal time of this visual; it pauses while the player does.
  let elapsed = 0
  let disposed = false

  const setLive = (next: boolean) => {
    if (live === next) return
    live = next
    options.onLiveChange?.(next)
  }

  const tick = (frameTick: MediaFxTemporalTick) => {
    if (disposed || !baseline) return
    if (options.canRender && !options.canRender()) return
    elapsed += frameTick.delta * temporal.speed
    let presented = false
    try {
      const frame = session.render(temporal, elapsed)
      presented = Boolean(frame && options.present(frame))
    } catch {
      presented = false
    }
    if (!presented) {
      // Keep the static baseline; a new baseline (or a new player) may try again.
      failed = true
      sync()
      return
    }
    setLive(true)
  }

  function sync() {
    const shouldRun = !disposed
      && active
      && Boolean(baseline)
      && !failed
      && mediaFxTemporalHasContent(temporal)
      && gpuSupported()
    if (shouldRun) {
      unsubscribe ??= clock.subscribe(tick)
      return
    }
    unsubscribe?.()
    unsubscribe = null
    if (!disposed) setLive(false)
  }

  return {
    setBaseline(next) {
      if (disposed || next === baseline) return
      baseline = next
      session.setBaseline(next)
      failed = false
      sync()
    },
    update(input) {
      if (disposed) return
      temporal = normalizeMediaFxTemporal(input.temporal)
      active = input.active
      sync()
    },
    get live() {
      return live
    },
    get running() {
      return Boolean(unsubscribe)
    },
    dispose() {
      if (disposed) return
      disposed = true
      unsubscribe?.()
      unsubscribe = null
      live = false
      baseline = null
      session.dispose()
    },
  }
}

// Live frames run every tick, so their raster follows the displayed size x DPR with a
// pixel budget instead of the (possibly 4K) source resolution.
export const MEDIA_FX_TEMPORAL_MAX_RASTER_PIXELS = 2048 * 2048
const MEDIA_FX_TEMPORAL_MAX_RASTER_EDGE = 4096

export interface MediaFxTemporalRaster {
  width: number
  height: number
  // Raster pixels per layout pixel.
  pixelRatio: number
}

export const resolveMediaFxTemporalRaster = (
  layoutWidth: number,
  layoutHeight: number,
  devicePixelRatio: number,
): MediaFxTemporalRaster | null => {
  if (!(layoutWidth > 0 && layoutHeight > 0) || !Number.isFinite(layoutWidth) || !Number.isFinite(layoutHeight)) return null
  const dpr = Number.isFinite(devicePixelRatio) && devicePixelRatio > 0 ? devicePixelRatio : 1
  const ratio = Math.min(
    dpr,
    MEDIA_FX_TEMPORAL_MAX_RASTER_EDGE / layoutWidth,
    MEDIA_FX_TEMPORAL_MAX_RASTER_EDGE / layoutHeight,
    Math.sqrt(MEDIA_FX_TEMPORAL_MAX_RASTER_PIXELS / (layoutWidth * layoutHeight)),
  )
  const pixelRatio = Math.max(0.001, Math.round(ratio * 1000) / 1000)
  return {
    width: Math.max(1, Math.round(layoutWidth * pixelRatio)),
    height: Math.max(1, Math.round(layoutHeight * pixelRatio)),
    pixelRatio,
  }
}

// Draws a live frame over a 2D canvas that has the frame's size. The copy reads the
// shared WebGL canvas (GPU to GPU in browsers that accelerate canvas); nothing is read
// back into JS memory.
export const presentMediaFxTemporalFrame = (target: HTMLCanvasElement | null | undefined, frame: MediaFxTemporalGpuFrame) => {
  const context = target?.getContext('2d')
  if (!target || !context) return false
  if (target.width !== frame.width || target.height !== frame.height) {
    target.width = frame.width
    target.height = frame.height
  } else {
    context.clearRect(0, 0, target.width, target.height)
  }
  context.drawImage(frame.canvas, frame.x, frame.y, frame.width, frame.height, 0, 0, frame.width, frame.height)
  return true
}
