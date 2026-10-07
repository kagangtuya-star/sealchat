<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch, type CSSProperties } from 'vue'
import { vMediaFx } from '@/features/media-fx/media-fx-dom'
import { mediaFxGpuMaxTextureSize, mediaFxGpuSupported, subscribeMediaFxGpuAvailability } from '@/features/media-fx/media-fx-gpu'
import { mediaFxStaticSignature } from '@/features/media-fx/media-fx-preview'
import type { TheaterMediaRef } from '@/types/theaterPresentation'
import TheaterPresentationMedia from './TheaterPresentationMedia.vue'
import { resolveTheaterMediaFxBinding } from './theaterPresentationMedia'
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
// and the wrapper then drops its CSS basic filter. Until then, or on any failure, the
// plain DOM path stays visible.

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
const loadedImage = shallowRef<TheaterMediaFxImage<HTMLImageElement> | null>(null)
const fit = ref<TheaterMediaFit | null>(null)
const box = ref<TheaterMediaSize | null>(null)
const pixelRatio = ref(1)

const unsubscribeGpuAvailability = subscribeMediaFxGpuAvailability((available) => {
  gpuAvailable.value = available
})

const advancedEligible = computed(() => theaterMediaFxAdvancedEligible(props.mediaFx, props.media, gpuAvailable.value))
// Track the actual static pixel values, not only the spec object identity. Editor
// transactions may update a nested spec without replacing every surrounding object;
// basic/motion still refresh through the directive, so Advanced needs its own stable key.
const staticFxSignature = computed(() => mediaFxStaticSignature(props.mediaFx))
const binding = computed(() => {
  const base = resolveTheaterMediaFxBinding(props.mediaFx, props.media, props.reducedMotion)
  return advancedReady.value ? { ...base, filters: false } : base
})
const mediaStyle = computed<CSSProperties | undefined>(() => (advancedReady.value ? { visibility: 'hidden' } : undefined))

const controller = createTheaterMediaFxAdvancedController<HTMLImageElement, CanvasImageSource, HTMLCanvasElement>({
  maxTextureSize: mediaFxGpuMaxTextureSize,
  loadSource: loadTheaterMediaFxSource,
  rasterize: rasterizeTheaterMediaFx,
  present: (output) => {
    const canvas = outputRef.value
    const context = canvas?.getContext('2d')
    if (!canvas || !context) return false
    canvas.width = output.width
    canvas.height = output.height
    context.drawImage(output, 0, 0)
    return true
  },
  clear: () => {
    const canvas = outputRef.value
    if (!canvas) return
    canvas.width = 0
    canvas.height = 0
  },
  onReadyChange: (ready) => {
    advancedReady.value = ready
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
// A new source starts on the DOM path; the previous output is dropped right away.
watch([() => props.media.resourceAttachmentId, () => props.media.fallbackAttachmentId], () => {
  loadedImage.value = null
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
})

onMounted(() => {
  setTracking(advancedEligible.value)
  watch(advancedEligible, (enabled) => {
    setTracking(enabled)
    readFit()
  })
  watch(
    () => [staticFxSignature.value, props.media, gpuAvailable.value, loadedImage.value, fit.value, box.value, pixelRatio.value],
    updateController,
    { flush: 'post' },
  )
  updateController()
})

onBeforeUnmount(() => {
  controller.dispose()
  unsubscribeGpuAvailability()
  setTracking(false)
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
