<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import { resolveStageImageUrl, type StageObjectTransform } from '../shared/stage-types'
import { cloneStageData } from '../stage/stage-editing'
import type { TheaterEffectPlayback } from './theater-effect-runtime'
import {
  THEATER_EFFECT_DESIGN_HEIGHT,
  THEATER_EFFECT_DESIGN_WIDTH,
  theaterEffectConfigFromObject,
} from './theater-effect-types'

const props = defineProps<{
  playbacks: TheaterEffectPlayback[]
  selectedObject?: TheaterEffectPlayback['object'] | null
  editing: boolean
  editingTarget: 'frame' | 'media'
}>()

const emit = defineEmits<{
  transformStart: []
  transformUpdate: [transform: StageObjectTransform]
  transformEnd: []
  mediaTransformStart: []
  mediaTransformUpdate: [patch: { x: number, y: number }]
  mediaTransformEnd: []
}>()

const hostRef = ref<HTMLDivElement | null>(null)
const hostSize = ref({ width: 1, height: 1 })
let resizeObserver: ResizeObserver | null = null

const scale = computed(() => Math.min(
  hostSize.value.width / THEATER_EFFECT_DESIGN_WIDTH,
  hostSize.value.height / THEATER_EFFECT_DESIGN_HEIGHT,
))
const stageStyle = computed(() => ({
  width: `${THEATER_EFFECT_DESIGN_WIDTH}px`,
  height: `${THEATER_EFFECT_DESIGN_HEIGHT}px`,
  transform: `translate(-50%, -50%) scale(${scale.value})`,
}))

const objectStyle = (playback: TheaterEffectPlayback) => {
  const transform = playback.object.transform
  return {
    left: `${transform.x - transform.width / 2}px`,
    top: `${transform.y - transform.height / 2}px`,
    width: `${transform.width}px`,
    height: `${transform.height}px`,
    transform: `rotate(${transform.rotation}deg) scale(${transform.scaleX}, ${transform.scaleY})`,
    zIndex: String(100 + Math.round(transform.z * 10 + transform.order)),
  }
}

const mediaStyle = (playback: TheaterEffectPlayback) => {
  const transform = playback.config.builtin.mediaTransform
  return {
    '--effect-media-x': `${transform.x}px`,
    '--effect-media-y': `${transform.y}px`,
    '--effect-media-rotation': `${transform.rotation}deg`,
    '--effect-media-scale-x': transform.mirror ? -transform.scale : transform.scale,
    '--effect-media-scale-y': transform.scale,
  }
}

const mediaUrl = (playback: TheaterEffectPlayback) => {
  const image = playback.config.media || playback.object.image
  return image ? resolveStageImageUrl(image.url) : null
}

const mediaIsVideo = (playback: TheaterEffectPlayback) => (
  (playback.config.media || playback.object.image)?.mimeType?.startsWith('video/') === true
)

const selectedPlayback = computed<TheaterEffectPlayback | null>(() => {
  const object = props.selectedObject
  if (!object) return null
  return {
    instanceId: `editor:${object.id}`,
    effectId: object.id,
    expiresAt: Number.POSITIVE_INFINITY,
    object,
    config: theaterEffectConfigFromObject(object),
    preview: true,
  }
})

const visiblePlaybacks = computed(() => {
  const active = props.playbacks
  const selected = selectedPlayback.value
  if (!props.editing || !selected) return active
  if (active.some((playback) => playback.effectId === selected.effectId)) return active
  return [...active, selected]
})

let gesture: {
  kind: 'move' | 'resize' | 'media'
  pointerId: number
  startX: number
  startY: number
  transform: StageObjectTransform
  mediaX: number
  mediaY: number
} | null = null

const beginGesture = (event: PointerEvent, kind: 'move' | 'resize' | 'media') => {
  const object = props.selectedObject
  if (!props.editing || !object || event.button !== 0) return
  if (kind === 'move' && props.editingTarget !== 'frame') return
  if (kind === 'media' && props.editingTarget !== 'media') return
  const config = theaterEffectConfigFromObject(object)
  gesture = {
    kind,
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    transform: cloneStageData(object.transform),
    mediaX: config.builtin.mediaTransform.x,
    mediaY: config.builtin.mediaTransform.y,
  }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  if (kind === 'media') emit('mediaTransformStart')
  else emit('transformStart')
  event.preventDefault()
  event.stopPropagation()
}

const moveGesture = (event: PointerEvent) => {
  if (!gesture || event.pointerId !== gesture.pointerId) return
  const factor = Math.max(0.0001, scale.value)
  const deltaX = (event.clientX - gesture.startX) / factor
  const deltaY = (event.clientY - gesture.startY) / factor
  if (gesture.kind === 'media') {
    emit('mediaTransformUpdate', { x: gesture.mediaX + deltaX, y: gesture.mediaY + deltaY })
    return
  }
  const next = cloneStageData(gesture.transform)
  if (gesture.kind === 'move') {
    next.x = gesture.transform.x + deltaX
    next.y = gesture.transform.y + deltaY
  } else {
    next.width = Math.max(120, gesture.transform.width + deltaX * 2)
    next.height = Math.max(80, gesture.transform.height + deltaY * 2)
  }
  emit('transformUpdate', next)
}

const endGesture = (event: PointerEvent) => {
  if (!gesture || event.pointerId !== gesture.pointerId) return
  const kind = gesture.kind
  gesture = null
  if (kind === 'media') emit('mediaTransformEnd')
  else emit('transformEnd')
}

onMounted(() => {
  resizeObserver = new ResizeObserver(([entry]) => {
    hostSize.value = { width: entry.contentRect.width, height: entry.contentRect.height }
  })
  if (hostRef.value) resizeObserver.observe(hostRef.value)
})

onBeforeUnmount(() => resizeObserver?.disconnect())
</script>

<template>
  <div ref="hostRef" class="theater-effect-overlay" :class="{ 'is-editing': editing }">
    <div class="theater-effect-design-stage" :style="stageStyle">
      <article
        v-for="playback in visiblePlaybacks"
        :key="playback.instanceId"
        class="theater-effect-object"
        :class="{
          'is-editor': playback.instanceId.startsWith('editor:'),
          'is-selected': playback.effectId === selectedObject?.id,
          'is-media-edit': editingTarget === 'media',
          'has-media': Boolean(mediaUrl(playback)),
        }"
        :style="objectStyle(playback)"
        @pointerdown="beginGesture($event, 'move')"
        @pointermove="moveGesture"
        @pointerup="endGesture"
        @pointercancel="endGesture"
      >
        <div
          v-if="playback.config.kind === 'media'"
          class="theater-effect-media-only"
          @pointerdown.stop="beginGesture($event, editingTarget === 'media' ? 'media' : 'move')"
          @pointermove="moveGesture"
          @pointerup="endGesture"
          @pointercancel="endGesture"
        >
          <video v-if="mediaUrl(playback) && mediaIsVideo(playback)" class="theater-effect-media-only__asset" :style="mediaStyle(playback)" :src="mediaUrl(playback)!" autoplay loop muted playsinline />
          <img v-else-if="mediaUrl(playback)" class="theater-effect-media-only__asset" :style="mediaStyle(playback)" :src="mediaUrl(playback)!" alt="" draggable="false">
          <span v-else-if="editing && playback.effectId === selectedObject?.id">未设置媒体</span>
        </div>
        <div
          v-else
          class="theater-effect-cutin"
          :class="[
            `theme-${playback.config.builtin.theme}`,
            `format-${playback.config.builtin.format}`,
            { 'is-shaking': playback.config.builtin.shakeIntensity > 0 },
          ]"
          :style="{
            '--effect-duration': `${playback.config.durationMs}ms`,
            '--effect-preview-delay': `${playback.config.durationMs / -2}ms`,
            '--effect-accent': playback.config.builtin.accentColor,
            '--effect-main': playback.config.builtin.mainTextColor,
            '--effect-sub': playback.config.builtin.subTextColor,
            '--effect-dim': playback.config.builtin.dimIntensity / 100,
            '--effect-shake': `${playback.config.builtin.shakeIntensity * 2}px`,
          }"
        >
          <div class="theater-effect-cutin__dim" />
          <div class="theater-effect-cutin__paint" />
          <div class="theater-effect-cutin__deco"><i /><i /><i /></div>
          <div
            class="theater-effect-cutin__media"
            :style="mediaStyle(playback)"
            @pointerdown.stop="beginGesture($event, 'media')"
            @pointermove="moveGesture"
            @pointerup="endGesture"
            @pointercancel="endGesture"
          >
            <video v-if="mediaUrl(playback) && mediaIsVideo(playback)" :src="mediaUrl(playback)!" autoplay loop muted playsinline />
            <img v-else-if="mediaUrl(playback)" :src="mediaUrl(playback)!" alt="" draggable="false">
          </div>
          <div class="theater-effect-cutin__content">
            <strong>{{ playback.config.builtin.text }}</strong>
            <span>{{ playback.config.builtin.subText }}</span>
          </div>
        </div>
        <template v-if="editing && playback.effectId === selectedObject?.id">
          <div class="theater-effect-selection-label">{{ editingTarget === 'media' ? '拖动媒体' : '拖动特效框' }}</div>
          <button
            v-if="editingTarget === 'frame'"
            type="button"
            class="theater-effect-resize-handle"
            aria-label="调整特效大小"
            @pointerdown.stop="beginGesture($event, 'resize')"
            @pointermove="moveGesture"
            @pointerup="endGesture"
            @pointercancel="endGesture"
          />
        </template>
      </article>
    </div>
  </div>
</template>

<style scoped>
.theater-effect-overlay { position: absolute; z-index: 9; inset: 0; overflow: hidden; pointer-events: none; }
.theater-effect-design-stage { position: absolute; top: 50%; left: 50%; transform-origin: center; }
.theater-effect-object { position: absolute; transform-origin: center; contain: layout paint style; pointer-events: none; }
.theater-effect-object.is-editor { opacity: .78; }
.theater-effect-object.is-editor .theater-effect-cutin,
.theater-effect-object.is-editor .theater-effect-cutin * { animation-delay: var(--effect-preview-delay) !important; animation-play-state: paused !important; }
.theater-effect-overlay.is-editing .theater-effect-object.is-selected { outline: 2px solid #f59e0b; pointer-events: auto; cursor: move; }
.theater-effect-object.is-media-edit { cursor: default; }
.theater-effect-media-only { width: 100%; height: 100%; display: grid; place-items: center; overflow: hidden; color: #fbbf24; background: transparent; }
.theater-effect-overlay.is-editing .theater-effect-object.is-selected:not(.has-media) .theater-effect-media-only { background: rgba(15, 23, 42, .28); }
.theater-effect-media-only img, .theater-effect-media-only video { width: 100%; height: 100%; display: block; object-fit: contain; }
.theater-effect-media-only__asset { transform: translate(var(--effect-media-x, 0), var(--effect-media-y, 0)) rotate(var(--effect-media-rotation, 0deg)) scale(var(--effect-media-scale-x, 1), var(--effect-media-scale-y, 1)); }
.theater-effect-selection-label { position: absolute; top: -28px; left: 0; padding: 3px 7px; color: #111827; background: #f59e0b; font: 600 14px/1.2 sans-serif; }
.theater-effect-resize-handle { position: absolute; right: -9px; bottom: -9px; width: 18px; height: 18px; padding: 0; border: 2px solid #111827; border-radius: 50%; background: #f59e0b; cursor: nwse-resize; }
.theater-effect-cutin { --effect-accent: #e61c34; position: relative; width: 100%; height: 100%; overflow: hidden; color: white; isolation: isolate; }
.theater-effect-cutin.is-shaking { animation: effect-shake 420ms steps(2, end) 180ms both; }
.theater-effect-cutin__dim { position: absolute; inset: 0; background: rgba(0, 0, 0, var(--effect-dim)); animation: effect-dim var(--effect-duration) ease both; }
.theater-effect-cutin__paint { position: absolute; top: 35%; right: -8%; left: -8%; height: 30%; background: var(--effect-accent); transform: skewX(-12deg) scaleX(0); animation: effect-paint var(--effect-duration) cubic-bezier(.16, 1, .3, 1) both; }
.theater-effect-cutin__deco { position: absolute; inset: 0; }
.theater-effect-cutin__deco i { position: absolute; top: 49%; left: -10%; width: 120%; height: 3px; background: white; transform: scaleX(0); animation: effect-line var(--effect-duration) ease both; }
.theater-effect-cutin__deco i:nth-child(2) { top: 42%; height: 1px; animation-delay: 80ms; }
.theater-effect-cutin__deco i:nth-child(3) { top: 58%; height: 1px; animation-delay: 140ms; }
.theater-effect-cutin__media { position: absolute; z-index: 2; inset: 5% 4% 0 45%; transform-origin: center bottom; transform: translate(var(--effect-media-x, 0), var(--effect-media-y, 0)) rotate(var(--effect-media-rotation, 0deg)) scale(var(--effect-media-scale-x, 1), var(--effect-media-scale-y, 1)); animation: effect-media var(--effect-duration) ease-out both; }
.theater-effect-cutin__media img, .theater-effect-cutin__media video { width: 100%; height: 100%; display: block; object-fit: contain; object-position: center bottom; }
.theater-effect-cutin__content { position: absolute; z-index: 3; top: 50%; left: 8%; width: 58%; display: flex; flex-direction: column; transform: translateY(-50%); white-space: pre-line; }
.theater-effect-cutin__content strong { color: var(--effect-main); font: 900 clamp(48px, 7vw, 132px)/.86 Impact, sans-serif; letter-spacing: .04em; text-shadow: 5px 5px 0 rgba(0, 0, 0, .8); animation: effect-title var(--effect-duration) cubic-bezier(.2, .8, .2, 1) both; }
.theater-effect-cutin__content span { align-self: flex-start; margin-top: 18px; padding: 7px 22px; color: var(--effect-sub); background: white; font: 700 clamp(18px, 2vw, 36px)/1 sans-serif; animation: effect-sub var(--effect-duration) ease both; }
.theme-cyber .theater-effect-cutin__paint, .theme-neon .theater-effect-cutin__paint { background: linear-gradient(90deg, transparent, var(--effect-accent), #00e5ff, transparent); }
.theme-cyber .theater-effect-cutin__content strong, .theme-glitch .theater-effect-cutin__content strong { text-shadow: -5px 0 #00e5ff, 5px 0 #ff006e; animation-name: effect-glitch-title; }
.theme-cinematic .theater-effect-cutin__paint { top: 20%; height: 60%; background: linear-gradient(90deg, transparent, rgba(0, 0, 0, .92) 20% 80%, transparent); transform: none; }
.theme-impact .theater-effect-cutin__paint { top: 0; height: 100%; background: radial-gradient(circle, var(--effect-accent), #050505 55%); clip-path: polygon(0 25%, 100% 0, 90% 80%, 10% 100%); }
.theme-neon .theater-effect-cutin__content strong { color: white; text-shadow: 0 0 8px white, 0 0 30px var(--effect-accent); }
.theme-cleave .theater-effect-cutin__deco::after { position: absolute; top: 50%; left: -20%; width: 140%; height: 10px; background: white; box-shadow: 0 0 24px var(--effect-accent); transform: rotate(-12deg) scaleX(0); animation: effect-cleave var(--effect-duration) ease-out both; content: ''; }
.theme-eclipse .theater-effect-cutin__paint { top: 15%; left: 30%; width: 40%; height: 70%; border-radius: 50%; background: radial-gradient(circle, #050505 45%, var(--effect-accent) 48%, transparent 62%); transform: scale(0); animation-name: effect-eclipse; }
.format-boxed .theater-effect-cutin__media { inset: 24% 5% 22% 52%; overflow: hidden; border: 4px solid white; }
@keyframes effect-dim { 0%, 100% { opacity: 0; } 10%, 88% { opacity: 1; } }
@keyframes effect-paint { 0%, 100% { transform: skewX(-12deg) scaleX(0); } 12%, 88% { transform: skewX(-12deg) scaleX(1); } }
@keyframes effect-line { 0%, 100% { opacity: 0; transform: scaleX(0); } 14%, 84% { opacity: .8; transform: scaleX(1); } }
@keyframes effect-media { 0% { opacity: 0; transform: translate(calc(var(--effect-media-x, 0px) + 100px), var(--effect-media-y, 0px)) rotate(var(--effect-media-rotation, 0deg)) scale(var(--effect-media-scale-x, 1), var(--effect-media-scale-y, 1)); } 14%, 86% { opacity: 1; transform: translate(var(--effect-media-x, 0px), var(--effect-media-y, 0px)) rotate(var(--effect-media-rotation, 0deg)) scale(var(--effect-media-scale-x, 1), var(--effect-media-scale-y, 1)); } 100% { opacity: 0; transform: translate(calc(var(--effect-media-x, 0px) + 50px), var(--effect-media-y, 0px)) rotate(var(--effect-media-rotation, 0deg)) scale(var(--effect-media-scale-x, 1), var(--effect-media-scale-y, 1)); } }
@keyframes effect-title { 0% { opacity: 0; transform: translateX(-18%) skewX(-8deg); } 14%, 86% { opacity: 1; transform: skewX(-8deg); } 100% { opacity: 0; transform: translateX(10%) skewX(-8deg); } }
@keyframes effect-sub { 0%, 12%, 100% { opacity: 0; transform: scaleX(0); } 22%, 84% { opacity: 1; transform: scaleX(1); } }
@keyframes effect-glitch-title { 0%, 100% { opacity: 0; transform: translateX(-10%); } 12%, 18%, 24%, 86% { opacity: 1; transform: translateX(0); } 15%, 21% { transform: translateX(12px); } }
@keyframes effect-cleave { 0%, 18% { transform: rotate(-12deg) scaleX(0); } 28%, 80% { transform: rotate(-12deg) scaleX(1); } 100% { opacity: 0; transform: rotate(-12deg) scaleX(1); } }
@keyframes effect-eclipse { 0%, 100% { opacity: 0; transform: scale(0); } 18%, 82% { opacity: 1; transform: scale(1); } }
@keyframes effect-shake { 0%, 100% { transform: none; } 20% { transform: translate(var(--effect-shake), calc(0px - var(--effect-shake))); } 40% { transform: translate(calc(0px - var(--effect-shake)), var(--effect-shake)); } 60% { transform: translate(var(--effect-shake), var(--effect-shake)); } 80% { transform: translate(calc(0px - var(--effect-shake)), calc(0px - var(--effect-shake))); } }
@media (prefers-reduced-motion: reduce) {
  .theater-effect-cutin *, .theater-effect-cutin::before, .theater-effect-cutin::after { animation-duration: 180ms !important; animation-iteration-count: 1 !important; }
}
</style>
