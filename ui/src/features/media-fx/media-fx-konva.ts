import Konva from 'konva'
import type { Filter as KonvaFilter } from 'konva/lib/Node'

import {
  mediaFxFilterToCss,
  normalizeMediaFxSpec,
  prefersReducedMotion,
  resolveMediaFxMotionTrack,
  type MediaFxFilter,
  type MediaFxMotionFrame,
  type MediaFxMotionTrack,
} from './media-fx'
import { canvasFilterSupported } from './media-fx-canvas'

// Konva adapter.
//
// Layering contract:
//   object root group  -> business layout / drag / transformer / entrance tweens
//   motionNode (inner) -> Media FX motion only (x/y/scale around the content center)
//   imageNode          -> Media FX filters (cached, static images only)
//
// The controller never touches the root group, so continuous motion cannot fight
// with drag, rotation, entrance tweens or persisted object.transform values.

export interface KonvaMediaFxNodes {
  motionNode: Konva.Group
  imageNode?: Konva.Image | null
  // Called only when a non-looping motion reaches its natural end.
  onMotionComplete?: () => void
}

export interface KonvaMediaFxContext {
  // Content box of motionNode in its parent coordinates (0,0 .. width,height).
  width: number
  height: number
  motion?: boolean
  // Must be false for animated images / video: cache() would freeze one frame.
  filters?: boolean
  paused?: boolean
  reducedMotion?: boolean
}

export interface KonvaMediaFxController {
  update(spec: unknown, context: KonvaMediaFxContext): void
  // Re-evaluates the filter cache after the image source or its fit changed.
  refreshFilter(): void
  isMotionActive(): boolean
  clear(): void
  dispose(): void
}

const MAX_CACHE_PIXEL_RATIO = 3
const MAX_CACHE_EDGE_PX = 4096
const MIN_SEGMENT_SECONDS = 1 / 60

const sourceIds = new WeakMap<object, number>()
let sourceSequence = 0
const sourceIdentity = (source: unknown) => {
  if (!source || typeof source !== 'object') return 0
  let id = sourceIds.get(source)
  if (!id) {
    id = ++sourceSequence
    sourceIds.set(source, id)
  }
  return id
}

const sourceWidth = (source: CanvasImageSource | undefined) => {
  if (!source) return 0
  if (typeof HTMLImageElement !== 'undefined' && source instanceof HTMLImageElement) return source.naturalWidth || source.width
  if (typeof HTMLVideoElement !== 'undefined' && source instanceof HTMLVideoElement) return source.videoWidth
  const candidate = source as { width?: unknown }
  return typeof candidate.width === 'number' ? candidate.width : 0
}

// Fallback for browsers without CanvasRenderingContext2D.filter: only Konva built-in
// filters with a stable mapping are used. Partial grayscale / sepia have no
// one-to-one built-in equivalent and are skipped in this mode.
const fallbackKonvaFilters = (imageNode: Konva.Image, filter: MediaFxFilter) => {
  const filters: KonvaFilter[] = []
  if (filter.brightness !== 1) {
    imageNode.brightness(filter.brightness)
    filters.push(Konva.Filters.Brightness)
  }
  if (filter.contrast !== 1) {
    imageNode.contrast(100 * (Math.sqrt(filter.contrast) - 1))
    filters.push(Konva.Filters.Contrast)
  }
  if (filter.saturation !== 1 || filter.hueRotate !== 0) {
    imageNode.saturation(Math.max(-8, Math.log2(Math.max(0.004, filter.saturation))))
    imageNode.hue(filter.hueRotate)
    imageNode.luminance(0)
    filters.push(Konva.Filters.HSL)
  }
  if (filter.grayscale >= 1) filters.push(Konva.Filters.Grayscale)
  if (filter.sepia >= 1) filters.push(Konva.Filters.Sepia)
  if (filter.blurPx > 0) {
    imageNode.blurRadius(Math.round(filter.blurPx))
    filters.push(Konva.Filters.Blur)
  }
  return filters
}

export const createKonvaMediaFxController = (nodes: KonvaMediaFxNodes): KonvaMediaFxController => {
  const { motionNode, onMotionComplete } = nodes
  const imageNode = nodes.imageNode || null
  let filter: MediaFxFilter | null = null
  let filtersEnabled = true
  let filterSignature = ''
  let motionSignature = ''
  let track: MediaFxMotionTrack | null = null
  let tween: Konva.Tween | null = null
  let segmentToken = 0
  let layout = { width: 0, height: 0 }
  let disposed = false

  const requestDraw = () => motionNode.getLayer()?.batchDraw()

  const applyLayout = (width: number, height: number) => {
    if (layout.width === width && layout.height === height) return
    layout = { width, height }
    motionNode.setAttrs({ offsetX: width / 2, offsetY: height / 2 })
    if (!tween) applyFrame(null)
  }

  const applyFrame = (frame: Pick<MediaFxMotionFrame, 'x' | 'y' | 'scale'> | null) => {
    motionNode.setAttrs({
      x: layout.width / 2 + (frame ? frame.x * layout.width : 0),
      y: layout.height / 2 + (frame ? frame.y * layout.height : 0),
      scaleX: frame?.scale ?? 1,
      scaleY: frame?.scale ?? 1,
    })
  }

  const stopMotion = () => {
    segmentToken += 1
    tween?.destroy()
    tween = null
    track = null
    motionSignature = ''
    applyFrame(null)
  }

  const playSegment = (index: number, token: number) => {
    if (disposed || token !== segmentToken || !track) return
    const from = track.frames[index]
    const to = track.frames[index + 1]
    if (!from) return
    if (!to) {
      if (track.loop) playSegment(0, token)
      else {
        tween?.destroy()
        tween = null
        track = null
        applyFrame(null)
        requestDraw()
        onMotionComplete?.()
      }
      return
    }
    tween?.destroy()
    applyFrame(from)
    const current = new Konva.Tween({
      node: motionNode,
      duration: Math.max(MIN_SEGMENT_SECONDS, ((to.offset - from.offset) * track.durationMs) / 1_000),
      easing: track.easing === 'linear' ? Konva.Easings.Linear : Konva.Easings.EaseInOut,
      x: layout.width / 2 + to.x * layout.width,
      y: layout.height / 2 + to.y * layout.height,
      scaleX: to.scale,
      scaleY: to.scale,
      onUpdate: requestDraw,
      onFinish: () => {
        if (tween !== current) return
        playSegment(index + 1, token)
      },
    })
    tween = current
    current.play()
  }

  const applyMotion = (spec: ReturnType<typeof normalizeMediaFxSpec>, context: KonvaMediaFxContext) => {
    const reduced = context.reducedMotion ?? prefersReducedMotion()
    const nextTrack = context.motion === false || context.paused || reduced ? null : resolveMediaFxMotionTrack(spec.motion)
    const signature = nextTrack
      ? `${spec.motion.preset}|${spec.motion.intensity}|${spec.motion.durationMs}|${spec.motion.loop}|${layout.width}x${layout.height}`
      : ''
    // Same spec: keep the running loop and never replay a finished one-shot motion.
    if (signature === motionSignature) return
    stopMotion()
    if (!nextTrack) {
      requestDraw()
      return
    }
    motionSignature = signature
    track = nextTrack
    playSegment(0, segmentToken)
  }

  const applyFilter = () => {
    if (!imageNode) return
    const source = imageNode.image() as CanvasImageSource | undefined
    const css = filter && filtersEnabled && source ? mediaFxFilterToCss(filter) : ''
    const signature = css
      ? [
          css,
          sourceIdentity(source),
          imageNode.width(),
          imageNode.height(),
          imageNode.cropX(),
          imageNode.cropY(),
          imageNode.cropWidth(),
          imageNode.cropHeight(),
        ].join('|')
      : ''
    // cache() only reruns when the filter, the source or the fitted geometry changed.
    if (signature === filterSignature) return
    const hadFilter = Boolean(filterSignature)
    filterSignature = signature
    if (!css) {
      if (hadFilter) {
        imageNode.clearCache()
        imageNode.filters([])
        requestDraw()
      }
      return
    }
    imageNode.clearCache()
    imageNode.filters(canvasFilterSupported() ? [css] : fallbackKonvaFilters(imageNode, filter!))
    const nodeWidth = Math.max(1, imageNode.width())
    const nodeHeight = Math.max(1, imageNode.height())
    const sourceEdge = imageNode.cropWidth() || sourceWidth(source)
    const pixelRatio = Math.min(
      MAX_CACHE_PIXEL_RATIO,
      MAX_CACHE_EDGE_PX / Math.max(nodeWidth, nodeHeight),
      Math.max(Konva.pixelRatio || 1, sourceEdge / nodeWidth, 1),
    )
    imageNode.cache({ pixelRatio: Math.max(0.25, pixelRatio) })
    requestDraw()
  }

  const clear = () => {
    stopMotion()
    filter = null
    applyFilter()
  }

  return {
    update(input, context) {
      if (disposed) return
      const spec = normalizeMediaFxSpec(input)
      applyLayout(Math.max(0, context.width), Math.max(0, context.height))
      filter = spec.filter
      filtersEnabled = context.filters !== false
      applyFilter()
      applyMotion(spec, context)
    },
    refreshFilter() {
      if (!disposed) applyFilter()
    },
    isMotionActive() {
      return Boolean(track)
    },
    clear,
    dispose() {
      if (disposed) return
      clear()
      disposed = true
    },
  }
}
