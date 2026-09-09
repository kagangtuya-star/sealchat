<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon } from 'naive-ui'
import { ArrowsMaximize, X as CloseIcon } from '@vicons/tabler'
import Viewer from 'viewerjs'
import 'viewerjs/dist/viewer.css'
import { urlBase } from '@/stores/_config'
import type { WorldClueDetail } from '@/stores/worldClue'
import WorldClueContentView from './WorldClueContentView.vue'

const props = withDefaults(defineProps<{ clue: WorldClueDetail; mode?: 'presentation' | 'surface' }>(), {
  mode: 'presentation',
})
const emit = defineEmits<{ (event: 'close'): void }>()
const leaving = ref(false)
const durationMS = computed(() => Math.max(0, Math.min(1000, props.clue.presentation?.animationDurationMs || 0)))
const duration = computed(() => `${durationMS.value}ms`)
const isSurface = computed(() => props.mode === 'surface')
const mediaPlacement = computed(() => props.clue.presentation?.mediaPlacement || 'left')
const customBackgroundUrl = computed(() => props.clue.presentation?.backgroundMediaEnabled
  ? (props.clue.presentation.backgroundMediaAttachmentId
      ? `${urlBase}/api/v1/worlds/${encodeURIComponent(props.clue.worldId)}/clues/${encodeURIComponent(props.clue.id)}/background-media?v=${encodeURIComponent(props.clue.presentation.backgroundMediaAttachmentId)}`
      : props.clue.presentation.backgroundMediaUrl.trim())
  : '')
const backgroundVideoFallbackUrl = ref('')
const customBackgroundIsVideo = computed(() => {
  if (!customBackgroundUrl.value) return false
  if (props.clue.presentation?.backgroundMediaType === 'video' || backgroundVideoFallbackUrl.value === customBackgroundUrl.value) return true
  try {
    return /\.(mp4|webm|ogg|ogv|mov|m4v)$/i.test(new URL(customBackgroundUrl.value).pathname)
  } catch {
    return false
  }
})
watch(customBackgroundUrl, () => { backgroundVideoFallbackUrl.value = '' })
function useVideoBackground() {
  backgroundVideoFallbackUrl.value = customBackgroundUrl.value
}
const customBackgroundIsTiled = computed(() => props.clue.presentation?.backgroundMediaMode === 'tile' && !customBackgroundIsVideo.value)
const customBackgroundFit = computed(() => {
  const mode = props.clue.presentation?.backgroundMediaMode || 'cover'
  if (mode === 'center') return customBackgroundIsVideo.value ? 'contain' : 'none'
  if (mode === 'tile') return 'cover'
  return mode
})
const customBackgroundStyle = computed(() => {
  const presentation = props.clue.presentation
  const blur = presentation?.backgroundMediaBlur ?? 0
  return {
    '--clue-background-inset': `${-blur * 2}px`,
    opacity: (presentation?.backgroundMediaOpacity ?? 100) / 100,
    filter: `blur(${blur}px) brightness(${presentation?.backgroundMediaBrightness ?? 100}%)`,
  }
})
const tiledBackgroundStyle = computed(() => ({ backgroundImage: `url(${JSON.stringify(customBackgroundUrl.value)})` }))
const colorBackgroundStyle = computed(() => ({
  backgroundColor: props.clue.presentation?.backgroundColor || '#000000',
  opacity: (props.clue.presentation?.backgroundColorOpacity ?? 45) / 100,
}))
const backdropUrl = computed(() => {
  if (props.clue.kind !== 'image') return ''
  if (props.clue.imageAttachmentId) {
    return `${urlBase}/api/v1/worlds/${encodeURIComponent(props.clue.worldId)}/clues/${encodeURIComponent(props.clue.id)}/image?v=${encodeURIComponent(props.clue.imageAttachmentId)}`
  }
  return props.clue.imageUrl || ''
})
const mainVideoFallbackUrl = ref('')
const mainMediaIsVideo = computed(() => {
  if (props.clue.presentation?.mediaType === 'video' || mainVideoFallbackUrl.value === backdropUrl.value) return true
  try {
    return /\.(webm)$/i.test(new URL(backdropUrl.value).pathname)
  } catch {
    return false
  }
})
watch(backdropUrl, () => { mainVideoFallbackUrl.value = '' })
function useMainVideo() {
  mainVideoFallbackUrl.value = backdropUrl.value
}
const contentClue = computed<WorldClueDetail>(() => props.clue.kind === 'image'
  ? { ...props.clue, kind: 'text' }
  : props.clue)
const animationClass = computed(() => leaving.value
  && !isSurface.value
  ? `clue-overlay--leaving clue-overlay--exit-${props.clue.presentation?.exitAnimation || 'fade'}`
  : isSurface.value ? 'clue-overlay--surface'
  : `clue-overlay--${props.clue.presentation?.enterAnimation || 'fade'}`)
let closeTimer: number | undefined
let imageViewer: Viewer | null = null
let viewerImage: HTMLImageElement | null = null
function destroyImageViewer() {
  const viewer = imageViewer
  imageViewer = null
  viewer?.destroy()
  viewerImage?.remove()
  viewerImage = null
}
function openImageViewer() {
  if (!backdropUrl.value || mainMediaIsVideo.value) return
  destroyImageViewer()
  const image = document.createElement('img')
  image.src = backdropUrl.value
  image.style.display = 'none'
  document.body.appendChild(image)
  viewerImage = image
  imageViewer = new Viewer(image, {
    className: 'world-clue-image-viewer',
    navbar: false,
    title: false,
    toolbar: {
      zoomIn: true, zoomOut: true, oneToOne: true, reset: true,
      prev: false, play: false, next: false,
      rotateLeft: true, rotateRight: true,
      flipHorizontal: false, flipVertical: false,
    },
    tooltip: true,
    movable: true,
    zoomable: true,
    scalable: true,
    rotatable: true,
    transition: true,
    fullscreen: true,
    keyboard: true,
    zIndex: 25000,
    hidden: () => {
      const viewer = imageViewer
      imageViewer = null
      viewerImage?.remove()
      viewerImage = null
      viewer?.destroy()
    },
  })
  imageViewer.show()
}
function requestClose() {
  if (leaving.value) return
  leaving.value = true
  const reduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  const wait = reduced ? 0 : durationMS.value
  if (!wait) emit('close')
  else closeTimer = window.setTimeout(() => emit('close'), wait)
}
const onKeydown = (event: KeyboardEvent) => { if (event.key === 'Escape' && !imageViewer) requestClose() }
onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  if (closeTimer !== undefined) window.clearTimeout(closeTimer)
  destroyImageViewer()
})
</script>

<template>
  <div
    class="clue-overlay"
    :class="animationClass"
    :style="{ '--clue-duration': duration, '--clue-media-ratio': String(clue.presentation?.mediaRatio || .42) }"
    role="dialog"
    aria-modal="true"
    :aria-label="clue.title"
  >
    <div v-if="customBackgroundUrl" class="clue-overlay__background-media" :style="customBackgroundStyle" aria-hidden="true">
      <div v-if="customBackgroundIsTiled" class="clue-overlay__background-tile" :style="tiledBackgroundStyle" />
      <video v-else-if="customBackgroundIsVideo" :src="customBackgroundUrl" :style="{ objectFit: customBackgroundFit }" autoplay muted loop playsinline />
      <img v-else :src="customBackgroundUrl" :style="{ objectFit: customBackgroundFit }" alt="" @error="useVideoBackground" />
    </div>
    <img v-else-if="backdropUrl && !mainMediaIsVideo" class="clue-overlay__backdrop" :src="backdropUrl" alt="" aria-hidden="true" />
    <div v-if="clue.presentation?.backgroundColorEnabled" class="clue-overlay__background-color" :style="colorBackgroundStyle" aria-hidden="true" />
    <NButton v-if="!isSurface" class="clue-overlay__close" circle quaternary size="large" aria-label="关闭" @click="requestClose"><template #icon><NIcon><CloseIcon /></NIcon></template></NButton>
    <div class="clue-overlay__surface" :class="[`clue-overlay__surface--${mediaPlacement}`, `clue-overlay__surface--${clue.kind}`]">
      <header><span class="clue-overlay__kind">线索</span><h2 :style="{ color: clue.presentation?.titleColor || '#ffffff' }">{{ clue.title }}</h2></header>
      <div v-if="clue.kind === 'image' && backdropUrl" class="clue-overlay__media" :class="{ 'is-cover': clue.presentation?.objectFit === 'cover' }">
        <video v-if="mainMediaIsVideo" :src="backdropUrl" :style="{ objectFit: clue.presentation?.objectFit || 'contain' }" controls autoplay muted loop playsinline />
        <img v-else :src="backdropUrl" :alt="clue.title" :style="{ objectFit: clue.presentation?.objectFit || 'contain' }" @error="useMainVideo" />
        <NButton v-if="!mainMediaIsVideo" class="clue-overlay__expand" circle quaternary size="large" title="放大图片" aria-label="放大图片" @click="openImageViewer">
          <template #icon><NIcon><ArrowsMaximize /></NIcon></template>
        </NButton>
      </div>
      <WorldClueContentView :clue="contentClue" />
    </div>
  </div>
</template>

<style scoped>
.clue-overlay { position: fixed; inset: 0; z-index: 24000; display: grid; box-sizing: border-box; height: 100dvh; place-items: center; overflow: hidden; padding: max(24px, env(safe-area-inset-top)) max(24px, env(safe-area-inset-right)) max(24px, env(safe-area-inset-bottom)) max(24px, env(safe-area-inset-left)); color: #f5f7fa; background: transparent; backdrop-filter: blur(8px); animation: clue-fade var(--clue-duration) ease both; }
.clue-overlay--surface { position: relative; inset: auto; z-index: auto; display: block; box-sizing: border-box; width: 100%; min-height: 100%; height: 100%; max-height: 100%; overflow: hidden; padding: 20px; background: transparent; backdrop-filter: none; animation: none; }
.clue-overlay__backdrop { position: absolute; inset: -48px; z-index: 0; width: calc(100% + 96px); height: calc(100% + 96px); pointer-events: none; object-fit: cover; opacity: .42; filter: blur(34px) brightness(.72) saturate(.8); transform: scale(1.06); }
.clue-overlay__background-media { position: absolute; inset: var(--clue-background-inset, 0); z-index: 0; overflow: hidden; pointer-events: none; }
.clue-overlay__background-media > img, .clue-overlay__background-media > video, .clue-overlay__background-tile { display: block; width: 100%; height: 100%; pointer-events: none; }
.clue-overlay__background-media > img, .clue-overlay__background-media > video { object-position: center; }
.clue-overlay__background-tile { background-position: center; background-repeat: repeat; }
.clue-overlay__background-color { position: absolute; inset: 0; z-index: 1; pointer-events: none; }
.clue-overlay__surface { --clue-gap: clamp(30px, 5vw, 68px); position: relative; z-index: 2; display: grid; width: min(1180px, 100%); max-height: calc(100dvh - 48px); align-content: center; column-gap: var(--clue-gap); overflow: auto; padding: clamp(10px, 2vw, 28px); background: transparent; scrollbar-width: thin; }
.clue-overlay--surface .clue-overlay__surface { width: 100%; min-height: 0; height: 100%; max-height: 100%; align-content: start; overflow: auto; padding: 0; }
.clue-overlay__surface--left { grid-template-columns: minmax(0, calc(var(--clue-media-ratio) * 100%)) minmax(0, 1fr); }
.clue-overlay__surface--right { grid-template-columns: minmax(0, 1fr) minmax(0, calc(var(--clue-media-ratio) * 100%)); }
.clue-overlay__surface--top, .clue-overlay__surface--bottom { grid-template-columns: minmax(0, 1fr); row-gap: 22px; }
.clue-overlay__surface :deep(.world-clue-content) { display: contents; }
.clue-overlay__surface--left header, .clue-overlay__surface--left :deep(.world-clue-content__body) { grid-column: 2; }
.clue-overlay__surface--left header { grid-row: 1; align-self: end; }
.clue-overlay__surface--left .clue-overlay__media, .clue-overlay__surface--left :deep(.world-clue-content__media) { grid-column: 1; grid-row: 1 / span 2; }
.clue-overlay__surface--left :deep(.world-clue-content__body) { grid-row: 2; }
.clue-overlay__surface--right header, .clue-overlay__surface--right :deep(.world-clue-content__body) { grid-column: 1; }
.clue-overlay__surface--right header { grid-row: 1; align-self: end; }
.clue-overlay__surface--right .clue-overlay__media, .clue-overlay__surface--right :deep(.world-clue-content__media) { grid-column: 2; grid-row: 1 / span 2; }
.clue-overlay__surface--right :deep(.world-clue-content__body) { grid-row: 2; }
.clue-overlay__surface--top .clue-overlay__media, .clue-overlay__surface--top :deep(.world-clue-content__media) { grid-row: 1; }
.clue-overlay__surface--top header { grid-row: 2; }
.clue-overlay__surface--top :deep(.world-clue-content__body) { grid-row: 3; }
.clue-overlay__surface--bottom header { grid-row: 1; }
.clue-overlay__surface--bottom :deep(.world-clue-content__body) { grid-row: 2; }
.clue-overlay__surface--bottom .clue-overlay__media, .clue-overlay__surface--bottom :deep(.world-clue-content__media) { grid-row: 3; }
.clue-overlay__surface--text { width: min(820px, 100%); grid-template-columns: minmax(0, 1fr); }
.clue-overlay__surface--text header, .clue-overlay__surface--text :deep(.world-clue-content__body) { grid-column: 1; }
.clue-overlay__surface--text header { grid-row: 1; }
.clue-overlay__surface--text :deep(.world-clue-content__body) { grid-row: 2; }
.clue-overlay__surface header { min-width: 0; margin-bottom: 16px; }
.clue-overlay__surface h2 { margin: 5px 0 0; color: #fff; font-family: Georgia, 'Times New Roman', serif; font-size: clamp(26px, 3vw, 46px); font-weight: 500; letter-spacing: 0; line-height: 1.2; overflow-wrap: anywhere; text-shadow: 0 2px 18px #000b; }
.clue-overlay__kind { color: rgb(255 255 255 / 72%); font-size: 11px; font-weight: 600; text-shadow: 0 1px 10px #000c; }
.clue-overlay__media, .clue-overlay__surface :deep(.world-clue-content__media) { position: relative; display: grid; width: 100%; min-width: 0; max-width: 620px; min-height: 0; place-self: center; overflow: hidden; border-radius: 8px; background: transparent; box-shadow: 0 16px 44px #0006; }
.clue-overlay__media img, .clue-overlay__media video { display: block; width: 100%; height: auto; max-height: min(74dvh, 720px); object-position: center; }
.clue-overlay__media.is-cover { height: min(64dvh, 620px); }
.clue-overlay__media.is-cover img, .clue-overlay__media.is-cover video { height: 100%; max-height: none; }
.clue-overlay__surface :deep(.world-clue-content__media img), .clue-overlay__surface :deep(.world-clue-content__media video) { width: 100%; height: auto; max-height: min(74dvh, 720px); }
.clue-overlay__surface :deep(.world-clue-content__body) { color: rgb(255 255 255 / 92%); font-size: clamp(15px, 1.35vw, 19px); text-shadow: 0 1px 12px #000c; }
.clue-overlay__expand { position: absolute; top: 9px; right: 9px; z-index: 1; color: #fff; background: rgb(0 0 0 / 38%); backdrop-filter: blur(8px); }
.clue-overlay__close { position: fixed; top: max(16px, env(safe-area-inset-top)); right: max(16px, env(safe-area-inset-right)); z-index: 3; color: #fff; background: rgb(0 0 0 / 28%); backdrop-filter: blur(8px); }
.clue-overlay--fade-up .clue-overlay__surface { animation: clue-up var(--clue-duration) ease both; }
.clue-overlay--scale .clue-overlay__surface, .clue-overlay--fade-scale .clue-overlay__surface { animation: clue-scale var(--clue-duration) ease both; }
.clue-overlay--slide-left .clue-overlay__surface { animation: clue-left var(--clue-duration) ease both; }
.clue-overlay--slide-right .clue-overlay__surface { animation: clue-right var(--clue-duration) ease both; }
.clue-overlay--leaving { animation: clue-fade-out var(--clue-duration) ease both; }
.clue-overlay--exit-fade-up .clue-overlay__surface { animation: clue-up-out var(--clue-duration) ease both; }
.clue-overlay--exit-scale .clue-overlay__surface, .clue-overlay--exit-fade-scale .clue-overlay__surface { animation: clue-scale-out var(--clue-duration) ease both; }
.clue-overlay--exit-slide-left .clue-overlay__surface { animation: clue-left-out var(--clue-duration) ease both; }
.clue-overlay--exit-slide-right .clue-overlay__surface { animation: clue-right-out var(--clue-duration) ease both; }
@keyframes clue-fade { from { opacity: 0; } }
@keyframes clue-fade-out { to { opacity: 0; } }
@keyframes clue-up { from { opacity: 0; transform: translateY(24px); } }
@keyframes clue-up-out { to { opacity: 0; transform: translateY(-24px); } }
@keyframes clue-scale { from { opacity: 0; transform: scale(.96); } }
@keyframes clue-scale-out { to { opacity: 0; transform: scale(.96); } }
@keyframes clue-left { from { opacity: 0; transform: translateX(-32px); } }
@keyframes clue-left-out { to { opacity: 0; transform: translateX(-32px); } }
@keyframes clue-right { from { opacity: 0; transform: translateX(32px); } }
@keyframes clue-right-out { to { opacity: 0; transform: translateX(32px); } }
@media (prefers-reduced-motion: reduce) { .clue-overlay, .clue-overlay__surface { animation: none !important; } }
@media (max-width: 720px) { .clue-overlay:not(.clue-overlay--surface) { padding: 0; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface, .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--left, .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--right { width: 100%; height: 100dvh; max-height: 100dvh; grid-template-columns: minmax(0, 1fr); align-content: start; row-gap: 18px; padding: max(56px, env(safe-area-inset-top)) 20px max(28px, env(safe-area-inset-bottom)); } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface .clue-overlay__media, .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface :deep(.world-clue-content__media) { grid-column: 1; grid-row: 1; width: min(86vw, 520px); max-height: 52dvh; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__media.is-cover { height: 46dvh; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface header, .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--left header, .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--right header { grid-column: 1; grid-row: 2; align-self: auto; margin: 0; text-align: center; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface :deep(.world-clue-content__body), .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--left :deep(.world-clue-content__body), .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--right :deep(.world-clue-content__body) { grid-column: 1; grid-row: 3; text-align: center; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--text header { grid-row: 1; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface--text :deep(.world-clue-content__body) { grid-row: 2; } .clue-overlay:not(.clue-overlay--surface) .clue-overlay__surface h2 { font-size: clamp(28px, 9vw, 42px); } }
@media (max-width: 460px) { .clue-overlay--surface { padding: 16px; } .clue-overlay--surface .clue-overlay__surface, .clue-overlay--surface .clue-overlay__surface--left, .clue-overlay--surface .clue-overlay__surface--right { width: 100%; grid-template-columns: minmax(0, 1fr); align-content: start; row-gap: 18px; padding: 0; } .clue-overlay--surface .clue-overlay__surface .clue-overlay__media, .clue-overlay--surface .clue-overlay__surface :deep(.world-clue-content__media) { grid-column: 1; grid-row: 1; width: 100%; } .clue-overlay--surface .clue-overlay__surface header, .clue-overlay--surface .clue-overlay__surface--left header, .clue-overlay--surface .clue-overlay__surface--right header { grid-column: 1; grid-row: 2; margin: 0; text-align: left; } .clue-overlay--surface .clue-overlay__surface :deep(.world-clue-content__body), .clue-overlay--surface .clue-overlay__surface--left :deep(.world-clue-content__body), .clue-overlay--surface .clue-overlay__surface--right :deep(.world-clue-content__body) { grid-column: 1; grid-row: 3; text-align: left; } }
</style>
