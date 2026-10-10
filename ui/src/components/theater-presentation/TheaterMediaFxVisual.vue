<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch, type CSSProperties } from 'vue'
import { mediaFxTemporalHasContent, mediaFxTemporalKeys, normalizeMediaFxSpec } from '@/features/media-fx/media-fx'
import { vMediaFx } from '@/features/media-fx/media-fx-dom'
import {
  mediaFxGpuMaxTextureSize,
  mediaFxGpuSupported,
  subscribeMediaFxGpuAvailability,
  type MediaFxTemporalBaseline,
} from '@/features/media-fx/media-fx-gpu'
import { mediaFxStaticSignature } from '@/features/media-fx/media-fx-preview'
import { createMediaFxTemporalPlayer, presentMediaFxTemporalFrame } from '@/features/media-fx/media-fx-temporal'
import type { TheaterMediaRef } from '@/types/theaterPresentation'
import TheaterPresentationMedia from './TheaterPresentationMedia.vue'
import { resolveTheaterMediaFxBinding, resolveTheaterMediaFxCapabilities } from './theaterPresentationMedia'
import {
  createTheaterMediaFxAdvancedController,
  loadTheaterMediaFxSource,
  rasterizeTheaterMediaFx,
  readTheaterMediaFit,
  theaterMediaFxAdvancedEligible,
  type TheaterMediaFit,
  type TheaterMediaFxImage,
  type TheaterMediaSize,
} from './theaterMediaFxAdvanced'

// Media visual of one TheaterPresentation layer, shared by the editor preview and every
// runtime consumer. The root is the Media FX wrapper (v-media-fx: WAAPI motion + CSS
// basic filter) around TheaterPresentationMedia; consumers keep layout, opacity, blend
// mode and fades on their own layer element outside it. For eligible static images the
// static advanced output (basic filter + advanced baked) replaces the <img> once ready,
// and the wrapper then drops its CSS basic filter. The original stays hidden while the
// first advanced output is pending; a processing failure reveals the plain DOM fallback.
//
// Temporal effects reuse that static output as their baseline: a shared-clock player
// draws live frames into the same output canvas while the layer is active, on screen,
// not paused by reduced motion, and repaints the baseline whenever live output stops or
// fails. Temporal-only specs use the same raster path (basic filter baked) so the live
// frames have a baseline; motion stays on the wrapper (WAAPI), never on the frame loop.

const props = withDefaults(defineProps<{
  media: TheaterMediaRef
  mediaFx?: unknown
  reducedMotion?: boolean
  playbackRate?: number
  active?: boolean
}>(), { reducedMotion: false, playbackRate: 1, active: true })
const emit = defineEmits<{ dimensions: [width: number, height: number] }>()

const hostRef = ref<HTMLElement | null>(null)
const outputRef = ref<HTMLCanvasElement | null>(null)
const gpuAvailable = ref(mediaFxGpuSupported())
const advancedReady = ref(false)
const advancedFallback = ref(false)
const loadedImage = shallowRef<TheaterMediaFxImage<HTMLImageElement> | null>(null)
const fit = ref<TheaterMediaFit | null>(null)
const box = ref<TheaterMediaSize | null>(null)
const pixelRatio = ref(1)

const unsubscribeGpuAvailability = subscribeMediaFxGpuAvailability((available) => {
  gpuAvailable.value = available
})

// Temporal values only (not the static look): their changes never re-raster.
const temporalSpec = computed(() => normalizeMediaFxSpec(props.mediaFx).temporal)
const temporalSignature = computed(() => mediaFxTemporalKeys.map((key) => temporalSpec.value[key]).join('|'))
const temporalLive = computed(() => (
  !props.reducedMotion
  && resolveTheaterMediaFxCapabilities(props.media, gpuAvailable.value).temporal
  && mediaFxTemporalHasContent(temporalSpec.value)
))
const advancedEligible = computed(() => theaterMediaFxAdvancedEligible(props.mediaFx, props.media, gpuAvailable.value, temporalLive.value))
// Off-screen layers keep their static look and do no live work.
const onScreen = ref(true)
// Track the actual static pixel values, not only the spec object identity. Editor
// transactions may update a nested spec without replacing every surrounding object;
// basic/motion still refresh through the directive, so Advanced needs its own stable key.
const staticFxSignature = computed(() => mediaFxStaticSignature(props.mediaFx))
const binding = computed(() => {
  const base = resolveTheaterMediaFxBinding(props.mediaFx, props.media, props.reducedMotion)
  return advancedReady.value ? { ...base, filters: false } : base
})
const mediaStyle = computed<CSSProperties | undefined>(() => (
  advancedReady.value || (advancedEligible.value && !advancedFallback.value) ? { visibility: 'hidden' } : undefined
))

// Static output kept as the temporal baseline, only while temporal plays live; an
// advanced-only layer does not hold a second copy of its raster.
let baseline: MediaFxTemporalBaseline | null = null
let rasterPixelRatio = 1

const releaseCanvas = (canvas: HTMLCanvasElement | null | undefined) => {
  if (canvas) canvas.width = canvas.height = 0
}

const setBaseline = (next: MediaFxTemporalBaseline | null) => {
  const previous = baseline?.source as HTMLCanvasElement | undefined
  baseline = next
  temporalPlayer.setBaseline(next)
  if (previous !== next?.source) releaseCanvas(previous)
}

const paintOutput = (source: HTMLCanvasElement) => {
  const canvas = outputRef.value
  const context = canvas?.getContext('2d')
  if (!canvas || !context) return false
  canvas.width = source.width
  canvas.height = source.height
  context.drawImage(source, 0, 0)
  return true
}

// Temporal became live over an output that is already shown: while nothing live runs
// the output canvas holds exactly the static look, so a copy of it is the baseline.
const ensureBaseline = () => {
  const canvas = outputRef.value
  if (baseline || !temporalLive.value || !advancedReady.value || !canvas?.width || !canvas.height || temporalPlayer.live) return
  const copy = document.createElement('canvas')
  copy.width = canvas.width
  copy.height = canvas.height
  const context = copy.getContext('2d')
  if (!context) return
  context.drawImage(canvas, 0, 0)
  setBaseline({ source: copy, width: copy.width, height: copy.height, pixelRatio: rasterPixelRatio })
}

const temporalPlayer = createMediaFxTemporalPlayer({
  present: (frame) => presentMediaFxTemporalFrame(outputRef.value, frame),
  // Inactive, failed or cleared: the static output is shown again.
  onLiveChange: (live) => {
    if (!live && baseline) paintOutput(baseline.source as HTMLCanvasElement)
  },
})

const controller = createTheaterMediaFxAdvancedController<HTMLImageElement, CanvasImageSource, HTMLCanvasElement>({
  maxTextureSize: mediaFxGpuMaxTextureSize,
  loadSource: loadTheaterMediaFxSource,
  rasterize: rasterizeTheaterMediaFx,
  present: (output, job) => {
    if (!paintOutput(output)) return false
    rasterPixelRatio = job.raster.pixelRatio
    if (temporalLive.value) {
      setBaseline({ source: output, width: output.width, height: output.height, pixelRatio: rasterPixelRatio })
    } else {
      setBaseline(null)
      releaseCanvas(output)
    }
    return true
  },
  clear: () => {
    setBaseline(null)
    releaseCanvas(outputRef.value)
  },
  onReadyChange: (ready) => {
    advancedReady.value = ready
  },
  onFallbackChange: (fallback) => {
    advancedFallback.value = fallback
  },
})

const readFit = () => {
  const image = loadedImage.value?.element
  fit.value = image && advancedEligible.value ? readTheaterMediaFit(image) : null
}

const handleImageLoad = (element: HTMLImageElement, attachmentId: string) => {
  loadedImage.value = { src: element.currentSrc || element.src, attachmentId, element }
  readFit()
}

// Compare attachment values: editor/live presentation clones keep the same loaded image.
// A new source waits for its own output instead of briefly showing an unprocessed image.
watch([() => props.media.resourceAttachmentId, () => props.media.fallbackAttachmentId], () => {
  loadedImage.value = null
  advancedFallback.value = false
})

// Size / DPR tracking only exists while the layer actually uses the advanced path.
let resizeObserver: ResizeObserver | null = null
let pixelRatioQuery: MediaQueryList | null = null

const stopPixelRatioTracking = () => {
  pixelRatioQuery?.removeEventListener('change', trackPixelRatio)
  pixelRatioQuery = null
}

function trackPixelRatio() {
  stopPixelRatioTracking()
  pixelRatio.value = window.devicePixelRatio || 1
  if (typeof window.matchMedia !== 'function') return
  pixelRatioQuery = window.matchMedia(`(resolution: ${pixelRatio.value}dppx)`)
  pixelRatioQuery.addEventListener('change', trackPixelRatio)
}

const setTracking = (enabled: boolean) => {
  if (!enabled) {
    resizeObserver?.disconnect()
    resizeObserver = null
    stopPixelRatioTracking()
    box.value = null
    return
  }
  if (!pixelRatioQuery) trackPixelRatio()
  const host = hostRef.value
  if (!host) return
  // Prime the raster immediately when Advanced is enabled. Waiting exclusively for the
  // first ResizeObserver delivery makes the editor preview lag behind the slider update.
  const rect = host.getBoundingClientRect()
  if (rect.width >= 1 && rect.height >= 1) box.value = { width: rect.width, height: rect.height }
  if (resizeObserver || typeof ResizeObserver === 'undefined') return
  resizeObserver = new ResizeObserver((entries) => {
    const next = entries[entries.length - 1]?.contentRect
    if (next) box.value = { width: next.width, height: next.height }
  })
  resizeObserver.observe(host)
}

const updateController = () => controller.update({
  spec: props.mediaFx,
  media: props.media,
  gpuAvailable: gpuAvailable.value,
  image: loadedImage.value,
  fit: fit.value,
  box: box.value,
  devicePixelRatio: pixelRatio.value,
  temporalLive: temporalLive.value,
})

// Visibility gate for live frames only; the static output never depends on it.
let intersectionObserver: IntersectionObserver | null = null
const setVisibilityTracking = (enabled: boolean) => {
  if (!enabled) {
    intersectionObserver?.disconnect()
    intersectionObserver = null
    onScreen.value = true
    return
  }
  const host = hostRef.value
  if (intersectionObserver || !host || typeof IntersectionObserver === 'undefined') return
  intersectionObserver = new IntersectionObserver((entries) => {
    const entry = entries[entries.length - 1]
    if (entry) onScreen.value = entry.isIntersecting
  })
  intersectionObserver.observe(host)
}

const updateTemporal = () => {
  ensureBaseline()
  temporalPlayer.update({
    temporal: temporalSpec.value,
    active: temporalLive.value && props.active && onScreen.value && advancedReady.value,
  })
}

// Candidate changes include primary -> fallback and URL refreshes, even when the
// stored media IDs stay the same. Clear stale output before the new image loads.
const handleImageLoading = () => {
  loadedImage.value = null
  advancedFallback.value = false
  updateController()
}

onMounted(() => {
  setTracking(advancedEligible.value)
  watch(advancedEligible, (enabled) => {
    setTracking(enabled)
    readFit()
  })
  watch(
    () => [staticFxSignature.value, temporalLive.value, props.media, gpuAvailable.value, loadedImage.value, fit.value, box.value, pixelRatio.value],
    updateController,
    { flush: 'post' },
  )
  updateController()
  setVisibilityTracking(temporalLive.value)
  watch(temporalLive, setVisibilityTracking, { flush: 'post' })
  watch(
    () => [temporalSignature.value, temporalLive.value, props.active, onScreen.value, advancedReady.value],
    updateTemporal,
    { flush: 'post' },
  )
  updateTemporal()
})

onBeforeUnmount(() => {
  temporalPlayer.dispose()
  controller.dispose()
  setBaseline(null)
  unsubscribeGpuAvailability()
  setTracking(false)
  setVisibilityTracking(false)
})
</script>

<template>
  <div ref="hostRef" v-media-fx="binding" class="theater-media-fx" :class="{ 'theater-media-fx--advanced': advancedEligible }">
    <TheaterPresentationMedia
      :media="media"
      :playback-rate="playbackRate"
      :active="active"
      :style="mediaStyle"
      @dimensions="(width, height) => emit('dimensions', width, height)"
      @image-load="handleImageLoad"
      @image-loading="handleImageLoading"
    />
    <canvas v-if="advancedEligible" v-show="advancedReady" ref="outputRef" class="theater-media-fx__output" aria-hidden="true" />
  </div>
</template>

<style scoped>
.theater-media-fx--advanced { position: relative; }
.theater-media-fx__output {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
  user-select: none;
}
</style>
