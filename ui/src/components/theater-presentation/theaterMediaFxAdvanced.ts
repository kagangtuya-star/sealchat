import { mediaFxAdvancedHasContent, normalizeMediaFxSpec, type MediaFxSpec } from '../../features/media-fx/media-fx'
import { bakeMediaFxToCanvas } from '../../features/media-fx/media-fx-canvas'
import { createMediaFxPreviewScheduler, mediaFxStaticPreviewSpec, mediaFxStaticSignature } from '../../features/media-fx/media-fx-preview'
import type { TheaterMediaRef } from '../../types/theaterPresentation'
import { resolveTheaterMediaFxCapabilities } from './theaterPresentationMedia'
import { fetchAttachmentBlobById } from '../../composables/useAttachmentResolver'

// Static advanced Media FX for TheaterPresentation DOM layers.
//
// The DOM path (TheaterPresentationMedia + v-media-fx: CSS basic filter, WAAPI motion)
// is always rendered and is the fallback. Only for a static image whose spec has
// advanced content while the GPU is available, the host rasterizes
//   source -> object-fit geometry -> basic filter -> advanced GPU   (bakeMediaFxToCanvas)
// at the rendered box size x device pixel ratio, shows that output in place of the <img>
// and drops its CSS basic filter (already baked). Motion stays on the host wrapper, so
// motion edits never re-raster. Any failure keeps the DOM path visible.
//
// This module owns the decisions (eligibility, raster size, geometry, signature,
// single-flight and stale guards). Element access, rasterizing and presenting are
// injected, so a later renderer can replace the static pixel step without touching the
// consumers or this lifecycle.

export type TheaterMediaFit = 'cover' | 'contain' | 'fill'

export interface TheaterMediaSize {
  width: number
  height: number
}

export interface TheaterMediaFxRaster extends TheaterMediaSize {
  // Raster pixels per layout pixel. Advanced strengths are defined in layout pixels, so
  // this is what keeps blur / blocks / offsets the same size at any DPR.
  pixelRatio: number
}

// One raster stays affordable on large layers with a high DPR: the pixel ratio is
// lowered instead, which keeps the effect scale in layout pixels.
export const THEATER_MEDIA_FX_MAX_RASTER_PIXELS = 2048 * 2048

export const theaterMediaFxAdvancedEligible = (
  spec: unknown,
  media: TheaterMediaRef | null | undefined,
  gpuAvailable: boolean,
) => Boolean(
  spec
  && resolveTheaterMediaFxCapabilities(media, gpuAvailable).advanced
  && mediaFxAdvancedHasContent(normalizeMediaFxSpec(spec).advanced),
)

export const resolveTheaterMediaFxRaster = (
  box: TheaterMediaSize,
  devicePixelRatio: number,
  maxTextureSize: number,
): TheaterMediaFxRaster | null => {
  // Whole layout pixels: sub-pixel layout jitter never triggers a new raster.
  const width = Math.round(box.width)
  const height = Math.round(box.height)
  const maxSide = Number.isFinite(maxTextureSize) ? Math.floor(maxTextureSize) : 0
  if (!(width >= 1 && height >= 1 && maxSide >= 1)) return null
  const dpr = Number.isFinite(devicePixelRatio) && devicePixelRatio > 0 ? devicePixelRatio : 1
  const ratio = Math.min(dpr, maxSide / width, maxSide / height, Math.sqrt(THEATER_MEDIA_FX_MAX_RASTER_PIXELS / (width * height)))
  const pixelRatio = Math.max(0.001, Math.round(ratio * 1000) / 1000)
  return {
    width: Math.min(maxSide, Math.max(1, Math.round(width * pixelRatio))),
    height: Math.min(maxSide, Math.max(1, Math.round(height * pixelRatio))),
    pixelRatio,
  }
}

// Same geometry as CSS object-fit with the default centered object-position.
export const resolveTheaterMediaFitRect = (source: TheaterMediaSize, target: TheaterMediaSize, fit: TheaterMediaFit) => {
  if (fit === 'fill') return { x: 0, y: 0, width: target.width, height: target.height }
  const scaleX = target.width / source.width
  const scaleY = target.height / source.height
  const scale = fit === 'cover' ? Math.max(scaleX, scaleY) : Math.min(scaleX, scaleY)
  const width = source.width * scale
  const height = source.height * scale
  return { x: (target.width - width) / 2, y: (target.height - height) / 2, width, height }
}

// The rendered object-fit of the visible <img> (consumers may override the default cover,
// e.g. resident portraits use contain). Anything this renderer cannot reproduce exactly
// returns null and keeps the DOM path.
export const readTheaterMediaFit = (element: Element): TheaterMediaFit | null => {
  if (typeof getComputedStyle !== 'function') return null
  const style = getComputedStyle(element)
  const position = style.objectPosition.trim()
  if (position && position !== '50% 50%') return null
  const fit = style.objectFit
  return fit === 'cover' || fit === 'contain' || fit === 'fill' ? fit : null
}

export interface TheaterMediaFxImage<TElement> {
  src: string
  attachmentId: string
  element: TElement
}

export interface TheaterMediaFxLoadedSource<TSource> extends TheaterMediaSize {
  source: TSource
  release?: () => void
}

export interface TheaterMediaFxRasterJob {
  key: string
  src: string
  attachmentId: string
  fit: TheaterMediaFit
  raster: TheaterMediaFxRaster
  // Static part only (basic filter + advanced); motion never reaches the rasterizer.
  spec: MediaFxSpec
}

export interface TheaterMediaFxAdvancedInput<TElement> {
  spec: unknown
  media: TheaterMediaRef | null | undefined
  gpuAvailable: boolean
  // The loaded <img> of the current candidate; null while a new source is loading.
  image: TheaterMediaFxImage<TElement> | null
  fit: TheaterMediaFit | null
  // Layout size of the visual box; null / empty while it is not laid out.
  box: TheaterMediaSize | null
  devicePixelRatio: number
}

export interface TheaterMediaFxAdvancedDeps<TElement, TSource, TOutput> {
  maxTextureSize: () => number | null
  loadSource: (image: TheaterMediaFxImage<TElement>) => Promise<TheaterMediaFxLoadedSource<TSource> | null>
  // Returns null (or throws) when the static bake or the GPU pass fails.
  rasterize: (source: TheaterMediaFxLoadedSource<TSource>, job: TheaterMediaFxRasterJob) => TOutput | null
  // Shows a finished output; false when the host can no longer show it.
  present: (output: TOutput) => boolean
  // Drops the shown output so the DOM fallback is visible again.
  clear: () => void
  onReadyChange: (ready: boolean) => void
  delayMs?: number
  setTimer?: (callback: () => void, delayMs: number) => unknown
  clearTimer?: (handle: unknown) => void
}

export interface TheaterMediaFxAdvancedController<TElement> {
  update(input: TheaterMediaFxAdvancedInput<TElement>): void
  readonly ready: boolean
  dispose(): void
}

// Pixel-read failures get a short page-wide cooldown. This avoids repeatedly probing the
// same CORS-blocked source when dialogue layers remount, while still allowing temporary
// network / proxy / decode failures to recover without changing the media or spec.
const UNREADABLE_SOURCE_LIMIT = 256
const UNREADABLE_SOURCE_COOLDOWN_MS = 30_000
const unreadableSources = new Map<string, number>()
const rememberUnreadableSource = (src: string) => {
  if (unreadableSources.size >= UNREADABLE_SOURCE_LIMIT) unreadableSources.clear()
  unreadableSources.set(src, Date.now() + UNREADABLE_SOURCE_COOLDOWN_MS)
}
const sourceReadCoolingDown = (src: string) => {
  const retryAt = unreadableSources.get(src)
  if (!retryAt) return false
  if (retryAt > Date.now()) return true
  unreadableSources.delete(src)
  return false
}

const sourceIdentity = (image: { src: string, attachmentId: string }) => JSON.stringify([image.src, image.attachmentId])

export const createTheaterMediaFxAdvancedController = <TElement, TSource, TOutput>(
  deps: TheaterMediaFxAdvancedDeps<TElement, TSource, TOutput>,
): TheaterMediaFxAdvancedController<TElement> => {
  let input: TheaterMediaFxAdvancedInput<TElement> | null = null
  let desired: TheaterMediaFxRasterJob | null = null
  let committed: { key: string, sourceId: string } | null = null
  let loaded: { sourceId: string, value: TheaterMediaFxLoadedSource<TSource> } | null = null
  let ready = false
  let disposed = false

  const setReady = (next: boolean) => {
    if (ready === next) return
    ready = next
    deps.onReadyChange(next)
  }

  const clearOutput = () => {
    if (!committed) return
    committed = null
    setReady(false)
    deps.clear()
  }

  const releaseSource = () => {
    loaded?.value.release?.()
    loaded = null
  }

  const isCurrentSource = (sourceId: string) => !disposed && input?.image && sourceIdentity(input.image) === sourceId

  // Decoded source cache for this host: size / spec changes re-raster without refetching.
  const acquireSource = async (image: TheaterMediaFxImage<TElement>) => {
    const sourceId = sourceIdentity(image)
    if (loaded?.sourceId === sourceId) return loaded.value
    releaseSource()
    let value: TheaterMediaFxLoadedSource<TSource> | null = null
    try {
      value = await deps.loadSource(image)
    } catch {
      value = null
    }
    if (!isCurrentSource(sourceId)) {
      value?.release?.()
      return null
    }
    if (!value || !(value.width > 0 && value.height > 0)) {
      value?.release?.()
      rememberUnreadableSource(sourceId)
      return null
    }
    loaded = { sourceId, value }
    return value
  }

  const scheduler = createMediaFxPreviewScheduler<{ job: TheaterMediaFxRasterJob, output: TOutput }>({
    delayMs: deps.delayMs ?? 48,
    setTimer: deps.setTimer,
    clearTimer: deps.clearTimer,
    render: async () => {
      const job = desired
      const image = input?.image
      if (!job || !image || sourceIdentity(image) !== sourceIdentity(job)) return null
      const source = await acquireSource(image)
      if (desired?.key !== job.key) return null
      if (!source) {
        clearOutput()
        return null
      }
      let output: TOutput | null = null
      try {
        output = deps.rasterize(source, job)
      } catch {
        output = null
      }
      if (!output) {
        clearOutput()
        return null
      }
      return { job, output }
    },
    commit: ({ job, output }) => {
      if (disposed || desired?.key !== job.key) return
      if (!deps.present(output)) {
        clearOutput()
        return
      }
      committed = { key: job.key, sourceId: sourceIdentity(job) }
      setReady(true)
    },
  })

  const stopRaster = () => {
    desired = null
    scheduler.invalidate()
  }

  return {
    update(next) {
      if (disposed) return
      input = next
      const image = next.image
      const sourceId = image ? sourceIdentity(image) : null
      if (loaded && loaded.sourceId !== sourceId) releaseSource()
      // An output of a previous source never stands in for the new one.
      if (committed && committed.sourceId !== sourceId) clearOutput()
      if (
        !image
        || !next.fit
        || sourceReadCoolingDown(sourceIdentity(image))
        || !theaterMediaFxAdvancedEligible(next.spec, next.media, next.gpuAvailable)
      ) {
        stopRaster()
        clearOutput()
        return
      }
      // Not laid out (e.g. hidden): keep whatever is shown and wait for a size.
      if (!next.box || !(next.box.width >= 1 && next.box.height >= 1)) {
        stopRaster()
        return
      }
      const maxTextureSize = deps.maxTextureSize()
      const raster = maxTextureSize ? resolveTheaterMediaFxRaster(next.box, next.devicePixelRatio, maxTextureSize) : null
      if (!raster) {
        stopRaster()
        clearOutput()
        return
      }
      const spec = mediaFxStaticPreviewSpec(next.spec, {
        filters: resolveTheaterMediaFxCapabilities(next.media, next.gpuAvailable).filters,
        advanced: true,
      })
      const key = [image.src, image.attachmentId, next.fit, raster.width, raster.height, raster.pixelRatio, mediaFxStaticSignature(spec)].join('|')
      desired = { key, src: image.src, attachmentId: image.attachmentId, fit: next.fit, raster, spec }
      if (committed?.key === key) {
        // Back to what is already shown: drop any pending job for another key.
        scheduler.invalidate()
        return
      }
      // Same source, new spec / size: the previous output stays until the new one is ready.
      scheduler.request(key)
    },
    get ready() {
      return ready
    },
    dispose() {
      if (disposed) return
      disposed = true
      desired = null
      committed = null
      scheduler.dispose()
      releaseSource()
    },
  }
}

// ---------------------------------------------------------------------------
// DOM implementations of the injected steps
// ---------------------------------------------------------------------------

const canReadImagePixels = (image: HTMLImageElement) => {
  try {
    const probe = document.createElement('canvas')
    probe.width = 1
    probe.height = 1
    const context = probe.getContext('2d')
    if (!context) return false
    context.drawImage(image, 0, 0, 1, 1)
    context.getImageData(0, 0, 1, 1)
    return true
  } catch {
    return false
  }
}

// Never adds crossorigin to the visible <img> (that would break images without CORS
// headers). A readable loaded image is used directly; otherwise the displayed
// candidate's stored bytes are proxied by SealChat and decoded off-DOM. Any failure
// returns null and the layer simply stays on the DOM path.
export const loadTheaterMediaFxSource = async (
  image: TheaterMediaFxImage<HTMLImageElement>,
): Promise<TheaterMediaFxLoadedSource<CanvasImageSource> | null> => {
  const element = image.element
  if (element.complete && element.naturalWidth > 0 && element.naturalHeight > 0 && canReadImagePixels(element)) {
    return { source: element, width: element.naturalWidth, height: element.naturalHeight }
  }
  if (!image.attachmentId || typeof createImageBitmap !== 'function') return null
  try {
    const blob = await fetchAttachmentBlobById(image.attachmentId, { proxy: true })
    if (!blob) return null
    const bitmap = await createImageBitmap(blob)
    if (!(bitmap.width > 0 && bitmap.height > 0)) {
      bitmap.close()
      return null
    }
    return { source: bitmap, width: bitmap.width, height: bitmap.height, release: () => bitmap.close() }
  } catch {
    return null
  }
}

// source -> object-fit geometry at raster size -> bakeMediaFxToCanvas (basic filter, then
// advanced GPU). requireAdvanced makes a failed GPU pass throw instead of returning a
// basic-only image, so the caller falls back to the DOM path.
export const rasterizeTheaterMediaFx = (
  loaded: TheaterMediaFxLoadedSource<CanvasImageSource>,
  job: TheaterMediaFxRasterJob,
): HTMLCanvasElement | null => {
  const { raster } = job
  const fitted = document.createElement('canvas')
  fitted.width = raster.width
  fitted.height = raster.height
  const context = fitted.getContext('2d')
  if (!context) return null
  const rect = resolveTheaterMediaFitRect(loaded, raster, job.fit)
  context.imageSmoothingQuality = 'high'
  context.drawImage(loaded.source, rect.x, rect.y, rect.width, rect.height)
  return bakeMediaFxToCanvas(fitted, raster, job.spec, { pixelRatio: raster.pixelRatio, requireAdvanced: true })
}
