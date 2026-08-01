<script setup lang="ts">
import { computed, onBeforeUnmount, ref, type CSSProperties } from 'vue'
import { resolveTheaterBackdropColor, resolveTheaterTextTransformStyle, resolveTheaterTransformLayoutStyle, resolveTheaterTransformStyle, type TheaterPresentation, type TheaterTransform, type TheaterVisualLayer } from '@/types/theaterPresentation'
import type { TheaterEditorCommand, TheaterSection, TheaterSelection } from './theaterPresentationEditorState'
import TheaterPresentationMedia from './TheaterPresentationMedia.vue'
import './theaterComposition.css'

const props = defineProps<{
  draft: TheaterPresentation
  selection: TheaterSelection
  activeSection: TheaterSection
  previewEnabled?: boolean
  previewName?: string
  previewText?: string
}>()
const emit = defineEmits<{
  dispatch: [command: TheaterEditorCommand, options?: { transient?: boolean }]
  gestureStart: []
  gestureEnd: []
}>()

type GestureKind = 'drag' | 'resize' | 'rotate'
type ResizeCorner = 'nw' | 'ne' | 'sw' | 'se'
interface Gesture {
  kind: GestureKind
  target: TheaterSelection
  transform: TheaterTransform
  startX: number
  startY: number
  rect: DOMRect
  aspect: number
  centerX: number
  centerY: number
  startAngle: number
  corner: ResizeCorner
}

const viewportRef = ref<HTMLElement | null>(null)
const compositionRef = ref<HTMLElement | null>(null)
let gesture: Gesture | null = null
let pendingEvent: PointerEvent | null = null
let frame = 0
const resizeCorners: ResizeCorner[] = ['nw', 'ne', 'sw', 'se']

const sameSelection = (left: TheaterSelection, right: TheaterSelection) => (
  left.kind === right.kind && (left.kind !== 'decoration' || right.kind !== 'decoration' || left.id === right.id)
)
const sectionForTarget = (target: TheaterSelection): TheaterSection => {
  if (target.kind === 'portrait') return 'portrait'
  if (target.kind === 'speaker') return 'speaker'
  if (target.kind === 'content') return 'content'
  if (target.kind === 'decoration') return 'decorations'
  return 'dialogue'
}
const canInteract = (target: TheaterSelection) => sectionForTarget(target) === props.activeSection

const targetContainer = (target: TheaterSelection): HTMLElement | null => {
  if (target.kind === 'decoration') return viewportRef.value?.querySelector<HTMLElement>('[data-portrait-root="1"]') || compositionRef.value
  if (target.kind === 'dialogue-frame' || target.kind === 'speaker' || target.kind === 'content') return viewportRef.value?.querySelector<HTMLElement>('[data-dialogue-root="1"]') || viewportRef.value
  return compositionRef.value
}

const beginGesture = (event: PointerEvent, kind: GestureKind, target: TheaterSelection, transform: TheaterTransform, corner: ResizeCorner = 'se') => {
  if (!canInteract(target)) return
  event.preventDefault()
  event.stopPropagation()
  const container = targetContainer(target)
  if (!container) return
  emit('dispatch', { type: 'select', target })
  emit('gestureStart')
  const rect = container.getBoundingClientRect()
  const objectRect = (event.currentTarget as HTMLElement).closest<HTMLElement>('[data-transform-target]')?.getBoundingClientRect()
  const centerX = objectRect ? objectRect.left + objectRect.width / 2 : rect.left + rect.width * (transform.x + transform.width / 2)
  const centerY = objectRect ? objectRect.top + objectRect.height / 2 : rect.top + rect.height * (transform.y + transform.height / 2)
  gesture = {
    kind,
    target,
    transform: { ...transform },
    startX: event.clientX,
    startY: event.clientY,
    rect,
    aspect: Math.max(0.01, transform.width / transform.height),
    centerX,
    centerY,
    startAngle: Math.atan2(event.clientY - centerY, event.clientX - centerX),
    corner,
  }
  window.addEventListener('pointermove', handlePointerMove)
  window.addEventListener('pointerup', endGesture, { once: true })
}

const applyPointerMove = (event: PointerEvent) => {
  if (!gesture) return
  const dx = (event.clientX - gesture.startX) / Math.max(1, gesture.rect.width)
  const dy = (event.clientY - gesture.startY) / Math.max(1, gesture.rect.height)
  let transform: Partial<TheaterTransform>
  if (gesture.kind === 'drag') {
    transform = { x: gesture.transform.x + dx, y: gesture.transform.y + dy }
  } else if (gesture.kind === 'resize') {
    const west = gesture.corner === 'nw' || gesture.corner === 'sw'
    const north = gesture.corner === 'nw' || gesture.corner === 'ne'
    let width = Math.max(0.01, gesture.transform.width + (west ? -dx : dx))
    let height = Math.max(0.01, gesture.transform.height + (north ? -dy : dy))
    if (event.shiftKey) {
      const mediaAspect = selectedMediaAspect(gesture.target) || gesture.aspect
      height = width / mediaAspect * (gesture.rect.width / Math.max(1, gesture.rect.height))
    }
    transform = {
      width,
      height,
      x: west ? gesture.transform.x + gesture.transform.width - width : gesture.transform.x,
      y: north ? gesture.transform.y + gesture.transform.height - height : gesture.transform.y,
    }
  } else {
    const angle = Math.atan2(event.clientY - gesture.centerY, event.clientX - gesture.centerX)
    transform = { rotation: gesture.transform.rotation + (angle - gesture.startAngle) * 180 / Math.PI }
  }
  emit('dispatch', { type: 'set-transform', target: gesture.target, transform }, { transient: true })
}

const handlePointerMove = (event: PointerEvent) => {
  pendingEvent = event
  if (frame) return
  frame = requestAnimationFrame(() => {
    frame = 0
    if (pendingEvent) applyPointerMove(pendingEvent)
    pendingEvent = null
  })
}

const endGesture = () => {
  if (frame) cancelAnimationFrame(frame)
  frame = 0
  if (pendingEvent) applyPointerMove(pendingEvent)
  pendingEvent = null
  gesture = null
  window.removeEventListener('pointermove', handlePointerMove)
  emit('gestureEnd')
}

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', handlePointerMove)
  if (frame) cancelAnimationFrame(frame)
})

const selectedMediaAspect = (target: TheaterSelection) => {
  let layer: TheaterVisualLayer | null = null
  if (target.kind === 'portrait') layer = props.draft.portrait
  if (target.kind === 'decoration') layer = props.draft.portraitDecorations.find((item) => item.id === target.id) || null
  if (target.kind === 'dialogue-frame') layer = props.draft.dialogue.frame
  return layer?.media?.width && layer.media.height ? layer.media.width / layer.media.height : 0
}

const layerStyle = (layer: TheaterVisualLayer) => ({
  ...resolveTheaterTransformStyle(layer.transform),
  mixBlendMode: layer.blendMode,
  display: layer.enabled ? 'block' : 'none',
}) as CSSProperties

const dialogueStyle = computed(() => resolveTheaterTransformLayoutStyle(props.draft.dialogue.transform) as CSSProperties)
const dialogueSurfaceStyle = computed<CSSProperties>(() => ({
  opacity: String(props.draft.dialogue.transform.opacity),
}))
const dialogueFrameStyle = (frame: TheaterVisualLayer): CSSProperties => ({
  ...layerStyle(frame),
  opacity: String(frame.transform.opacity * props.draft.dialogue.transform.opacity),
})
const portraitRootStyle = computed<CSSProperties>(() => props.draft.portrait
  ? layerStyle(props.draft.portrait)
  : ({ position: 'absolute', inset: '0' } as CSSProperties))
const textLayerStyle = (kind: 'speaker' | 'content') => ({
  ...resolveTheaterTextTransformStyle(props.draft.dialogue[kind].transform),
  display: props.draft.dialogue[kind].enabled ? (kind === 'speaker' ? 'grid' : 'block') : 'none',
  '--theater-font-scale': String(props.draft.dialogue[kind].fontScale),
}) as CSSProperties
const narrationStyle = computed<CSSProperties>(() => ({
  backgroundColor: resolveTheaterBackdropColor(
    props.draft.narration.backdropColor,
    props.draft.narration.backdropOpacity,
  ),
}))
</script>

<template>
  <div class="theater-preview-wrap">
    <div ref="viewportRef" class="theater-preview theater-composition-host" :class="{ 'is-disabled': previewEnabled === false, 'is-narration': draft.narration.enabled }" data-testid="theater-presentation-preview">
      <div v-if="previewEnabled === false" class="theater-preview__disabled">编辑预览已关闭</div>
      <div v-if="draft.narration.enabled" class="theater-preview__narration" :style="narrationStyle" />

      <div ref="compositionRef" class="theater-composition">
        <div
          data-transform-target
          data-portrait-root="1"
          v-show="!draft.narration.enabled"
          class="theater-preview__layer"
          :class="{ 'is-selected': selection.kind === 'portrait' && draft.portrait, 'is-locked': activeSection !== 'portrait' }"
          :style="portraitRootStyle"
          @pointerdown="draft.portrait && beginGesture($event, 'drag', { kind: 'portrait' }, draft.portrait.transform)"
        >
          <TheaterPresentationMedia v-if="draft.portrait" :media="draft.portrait.media" :playback-rate="draft.portrait.playbackRate" />
          <template v-if="draft.portrait && sameSelection(selection, { kind: 'portrait' })">
            <button v-for="corner in resizeCorners" :key="corner" class="theater-preview__handle" :class="`theater-preview__handle--${corner}`" :aria-label="`从 ${corner} 调整大小`" @pointerdown="beginGesture($event, 'resize', { kind: 'portrait' }, draft.portrait!.transform, corner)" />
            <button class="theater-preview__handle theater-preview__handle--rotate" aria-label="旋转" @pointerdown="beginGesture($event, 'rotate', { kind: 'portrait' }, draft.portrait!.transform)" />
          </template>
          <div
            v-for="layer in draft.portraitDecorations"
            :key="layer.id"
            data-transform-target
            class="theater-preview__layer theater-preview__decoration"
            :class="{ 'is-selected': selection.kind === 'decoration' && selection.id === layer.id, 'is-locked': activeSection !== 'decorations' }"
            :style="layerStyle(layer)"
            @pointerdown="beginGesture($event, 'drag', { kind: 'decoration', id: layer.id }, layer.transform)"
          >
            <TheaterPresentationMedia :media="layer.media" :playback-rate="layer.playbackRate" />
            <template v-if="sameSelection(selection, { kind: 'decoration', id: layer.id })">
              <button v-for="corner in resizeCorners" :key="corner" class="theater-preview__handle" :class="`theater-preview__handle--${corner}`" :aria-label="`从 ${corner} 调整大小`" @pointerdown="beginGesture($event, 'resize', { kind: 'decoration', id: layer.id }, layer.transform, corner)" />
              <button class="theater-preview__handle theater-preview__handle--rotate" aria-label="旋转" @pointerdown="beginGesture($event, 'rotate', { kind: 'decoration', id: layer.id }, layer.transform)" />
            </template>
          </div>
        </div>

        <div
          data-transform-target
          data-dialogue-root="1"
          class="theater-preview__dialogue"
          :class="{ 'is-selected': selection.kind === 'dialogue', 'is-locked': activeSection !== 'dialogue' }"
          :style="dialogueStyle"
          @pointerdown="beginGesture($event, 'drag', { kind: 'dialogue' }, draft.dialogue.transform)"
        >
        <div v-if="!draft.narration.enabled && !draft.dialogue.frame?.enabled" class="theater-preview__default-frame" :style="dialogueSurfaceStyle" />
        <div
          v-if="!draft.narration.enabled && draft.dialogue.frame?.enabled"
          class="theater-preview__frame"
          :style="dialogueFrameStyle(draft.dialogue.frame)"
        >
          <TheaterPresentationMedia :media="draft.dialogue.frame.media" :playback-rate="draft.dialogue.frame.playbackRate" />
        </div>
        <div
          data-transform-target
          class="theater-preview__dialogue-content theater-preview__name"
          :class="{ 'is-selected': selection.kind === 'speaker', 'is-locked': activeSection !== 'speaker' }"
          :style="{ ...textLayerStyle('speaker'), display: draft.narration.enabled ? 'none' : textLayerStyle('speaker').display }"
          @pointerdown="beginGesture($event, 'drag', { kind: 'speaker' }, draft.dialogue.speaker.transform)"
        >
          <span class="theater-preview__name-value">{{ previewName || '角色名' }}</span>
          <template v-if="selection.kind === 'speaker'">
            <button v-for="corner in resizeCorners" :key="corner" class="theater-preview__handle" :class="`theater-preview__handle--${corner}`" :aria-label="`从 ${corner} 调整昵称大小`" @pointerdown="beginGesture($event, 'resize', { kind: 'speaker' }, draft.dialogue.speaker.transform, corner)" />
            <button class="theater-preview__handle theater-preview__handle--rotate" aria-label="旋转昵称" @pointerdown="beginGesture($event, 'rotate', { kind: 'speaker' }, draft.dialogue.speaker.transform)" />
          </template>
        </div>
        <div
          data-transform-target
          class="theater-preview__dialogue-content theater-preview__text"
          :class="{ 'is-selected': selection.kind === 'content', 'is-locked': activeSection !== 'content' }"
          :style="{ ...textLayerStyle('content'), textAlign: draft.dialogue.textAlign, color: draft.dialogue.contentColor }"
          @pointerdown="beginGesture($event, 'drag', { kind: 'content' }, draft.dialogue.content.transform)"
        >
          {{ previewText || '夜色正好，我们该出发了。' }}
          <template v-if="selection.kind === 'content'">
            <button v-for="corner in resizeCorners" :key="corner" class="theater-preview__handle" :class="`theater-preview__handle--${corner}`" :aria-label="`从 ${corner} 调整聊天内容大小`" @pointerdown="beginGesture($event, 'resize', { kind: 'content' }, draft.dialogue.content.transform, corner)" />
            <button class="theater-preview__handle theater-preview__handle--rotate" aria-label="旋转聊天内容" @pointerdown="beginGesture($event, 'rotate', { kind: 'content' }, draft.dialogue.content.transform)" />
          </template>
        </div>
        <template v-if="sameSelection(selection, { kind: 'dialogue' })">
          <button v-for="corner in resizeCorners" :key="corner" class="theater-preview__handle" :class="`theater-preview__handle--${corner}`" :aria-label="`从 ${corner} 调整大小`" @pointerdown="beginGesture($event, 'resize', { kind: 'dialogue' }, draft.dialogue.transform, corner)" />
          <button class="theater-preview__handle theater-preview__handle--rotate" aria-label="旋转" @pointerdown="beginGesture($event, 'rotate', { kind: 'dialogue' }, draft.dialogue.transform)" />
        </template>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.theater-preview-wrap { width: 100%; height: 100%; min-width: 0; min-height: 0; }
.theater-preview { width: 100%; height: 100%; min-height: 420px; background: #202329; touch-action: none; user-select: none; }
.theater-preview.is-narration {
  background-color: #202329;
  background-image:
    linear-gradient(45deg, rgba(255,255,255,.08) 25%, transparent 25%),
    linear-gradient(-45deg, rgba(255,255,255,.08) 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, rgba(255,255,255,.08) 75%),
    linear-gradient(-45deg, transparent 75%, rgba(255,255,255,.08) 75%);
  background-position: 0 0, 0 8px, 8px -8px, -8px 0;
  background-size: 16px 16px;
}
.theater-preview.is-disabled > :not(.theater-preview__disabled) { visibility: hidden; }
.theater-preview__disabled { position: absolute; z-index: 1000; inset: 0; display: grid; place-items: center; color: rgba(255,255,255,.58); }
.theater-preview__narration { position: absolute; z-index: 0; inset: 0; pointer-events: none; }
.theater-preview .theater-composition { z-index: 1; }
.theater-preview__layer, .theater-preview__dialogue { cursor: move; box-sizing: border-box; }
.theater-preview .is-locked { cursor: default; }
.theater-preview__layer.is-selected, .theater-preview__dialogue.is-selected, .theater-preview__frame.is-selected { outline: 2px solid #60a5fa; outline-offset: -2px; }
.theater-preview__decoration { pointer-events: auto; }
.theater-preview__dialogue { color: white; }
.theater-preview__default-frame { position: absolute; inset: 0; background: rgba(12,12,14,.94); border: 1px solid rgba(255,255,255,.25); border-radius: 4px; }
.theater-preview__frame { position: absolute; inset: 0; z-index: 1; cursor: move; }
.theater-preview__dialogue-content { box-sizing: border-box; overflow: hidden; cursor: move; }
.theater-preview__name { container-type: size; place-items: center start; color: #f59e0b; font-weight: 600; }
.theater-preview__name-value { max-width: 100%; overflow: hidden; font-size: calc(100cqh * var(--theater-font-scale, 1)); line-height: 1; white-space: nowrap; }
.theater-preview__text { white-space: pre-wrap; font-size: calc(1em * var(--theater-font-scale, 1)); line-height: 1.5; }
.theater-preview__handle { position: absolute; z-index: 999; width: 18px; height: 18px; padding: 0; border: 2px solid white; background: #2563eb; box-shadow: 0 1px 4px rgba(0,0,0,.4); }
.theater-preview__handle--nw { left: -9px; top: -9px; cursor: nwse-resize; }
.theater-preview__handle--ne { right: -9px; top: -9px; cursor: nesw-resize; }
.theater-preview__handle--sw { left: -9px; bottom: -9px; cursor: nesw-resize; }
.theater-preview__handle--se { right: -9px; bottom: -9px; cursor: nwse-resize; }
.theater-preview__handle--rotate { left: calc(50% - 9px); top: -30px; border-radius: 50%; cursor: grab; }
@media (max-width: 720px) { .theater-preview__handle { width: 24px; height: 24px; } }
</style>
