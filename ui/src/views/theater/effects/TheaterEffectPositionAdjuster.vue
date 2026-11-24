<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NInputNumber, NModal, useThemeVars } from 'naive-ui'
import { ArrowBackUp, Crosshair, Frame, Maximize, Photo, PlayerPlay, PlayerStop, X } from '@vicons/tabler'

import type { StageImageRef, StageObject, StageObjectTransform } from '../shared/stage-types'
import { cloneStageData } from '../stage/stage-editing'
import type { TheaterEffectPlayback } from './theater-effect-runtime'
import TheaterEffectOverlay from './TheaterEffectOverlay.vue'
import {
  DEFAULT_THEATER_EFFECT_FRAME,
  matchTheaterEffectAnchor,
  theaterEffectAnchorLabels,
  theaterEffectAnchorPosition,
  theaterEffectAnchors,
  type TheaterEffectAnchor,
} from './theater-effect-position'
import { THEATER_EFFECT_DESIGN_HEIGHT, THEATER_EFFECT_DESIGN_WIDTH, theaterEffectConfigFromObject } from './theater-effect-types'

type FrameBox = Pick<StageObjectTransform, 'x' | 'y' | 'width' | 'height'>

// Host-agnostic position editor for one effect object. Every preview is the real
// TheaterEffectOverlay; the host owns persistence and receives whole transforms.
const props = withDefaults(defineProps<{
  object: StageObject
  resolveMedia: (image: StageImageRef) => { url: string, sourceKind: 'image' | 'video' } | null
  disabled?: boolean
  defaultFrame?: FrameBox
}>(), { disabled: false, defaultFrame: () => ({ ...DEFAULT_THEATER_EFFECT_FRAME }) })

const emit = defineEmits<{
  updateTransform: [transform: StageObjectTransform]
  updateMediaPosition: [position: { x: number, y: number }]
}>()

const SNAP_DISTANCE = 12
const MIN_WIDTH = 120
const MIN_HEIGHT = 80
const CENTER_X = THEATER_EFFECT_DESIGN_WIDTH / 2
const CENTER_Y = THEATER_EFFECT_DESIGN_HEIGHT / 2

const themeVars = useThemeVars()
const show = ref(false)
const target = ref<'frame' | 'media'>('frame')
const config = computed(() => theaterEffectConfigFromObject(props.object))
const transform = computed(() => props.object.transform)
const mediaPosition = computed(() => config.value.builtin.mediaTransform)
const supportsMedia = computed(() => config.value.kind !== 'web')
const hasMedia = computed(() => supportsMedia.value && Boolean(config.value.media || props.object.image))
const activeTarget = computed(() => (supportsMedia.value ? target.value : 'frame'))
const anchor = computed(() => matchTheaterEffectAnchor(transform.value))
const kindLabel = computed(() => ({ builtin: '内置特效', media: '媒体特效', web: '网页特效' })[config.value.kind])

// The "editor:" instance prefix makes the overlay render its paused editing frame.
const stillPlayback = computed<TheaterEffectPlayback>(() => ({
  instanceId: `editor:position:${props.object.id}`,
  effectId: props.object.id,
  expiresAt: Number.POSITIVE_INFINITY,
  object: props.object,
  config: config.value,
  preview: true,
}))

const playback = ref<TheaterEffectPlayback | null>(null)
let playSerial = 0
let playTimer: ReturnType<typeof setTimeout> | null = null
const stopPlay = () => {
  if (playTimer) clearTimeout(playTimer)
  playTimer = null
  playback.value = null
}
const play = () => {
  stopPlay()
  const object = cloneStageData(props.object)
  const effect = theaterEffectConfigFromObject(object)
  playback.value = {
    instanceId: `position-play:${object.id}:${++playSerial}`,
    effectId: object.id,
    expiresAt: Date.now() + effect.durationMs,
    object,
    config: effect,
    preview: true,
  }
  playTimer = setTimeout(stopPlay, effect.durationMs)
}

const editable = computed(() => !props.disabled && !playback.value)
const stagePlaybacks = computed(() => {
  if (playback.value) return [playback.value]
  return editable.value ? [] : [stillPlayback.value]
})

watch(() => props.object.id, () => {
  stopPlay()
  if (!supportsMedia.value) target.value = 'frame'
})
watch(show, (visible) => { if (!visible) stopPlay() })

// Overlay chrome is authored in design pixels; counter-scale it to stay legible.
const canvasRef = ref<HTMLDivElement | null>(null)
const canvasWidth = ref(THEATER_EFFECT_DESIGN_WIDTH)
let resizeObserver: ResizeObserver | null = null
onMounted(() => {
  resizeObserver = new ResizeObserver(([entry]) => { canvasWidth.value = Math.max(1, entry.contentRect.width) })
})
watch(canvasRef, (element, previous) => {
  if (previous) resizeObserver?.unobserve(previous)
  if (element) resizeObserver?.observe(element)
})
onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  stopPlay()
})

const canvasStyle = computed(() => ({
  '--tep-ui': String(THEATER_EFFECT_DESIGN_WIDTH / canvasWidth.value),
}))
const rootStyle = computed(() => ({ '--tep-accent': themeVars.value.primaryColor }))

// Dashed frame bounds, so even transparent or empty effects show where they sit.
const frameBoundsStyle = computed(() => {
  const value = transform.value
  const width = value.width * Math.abs(value.scaleX || 1)
  const height = value.height * Math.abs(value.scaleY || 1)
  return {
    left: `${((value.x - width / 2) / THEATER_EFFECT_DESIGN_WIDTH) * 100}%`,
    top: `${((value.y - height / 2) / THEATER_EFFECT_DESIGN_HEIGHT) * 100}%`,
    width: `${(width / THEATER_EFFECT_DESIGN_WIDTH) * 100}%`,
    height: `${(height / THEATER_EFFECT_DESIGN_HEIGHT) * 100}%`,
    transform: `rotate(${value.rotation}deg)`,
  }
})

const emitFrame = (patch: Partial<FrameBox>) => {
  if (props.disabled) return
  const next = { ...cloneStageData(props.object.transform), ...patch }
  next.x = Math.round(next.x)
  next.y = Math.round(next.y)
  next.width = Math.max(MIN_WIDTH, Math.round(next.width))
  next.height = Math.max(MIN_HEIGHT, Math.round(next.height))
  emit('updateTransform', next)
}
const emitMedia = (patch: Partial<{ x: number, y: number }>) => {
  if (props.disabled) return
  const next = { x: mediaPosition.value.x, y: mediaPosition.value.y, ...patch }
  emit('updateMediaPosition', { x: Math.round(next.x), y: Math.round(next.y) })
}

const snapped = ref({ x: false, y: false })
const dragging = ref(false)
const snap = (value: number, to: number, distance = SNAP_DISTANCE) => (Math.abs(value - to) <= distance ? to : value)
const handleGestureTransform = (next: StageObjectTransform) => {
  const x = snap(next.x, CENTER_X)
  const y = snap(next.y, CENTER_Y)
  snapped.value = { x: x === CENTER_X, y: y === CENTER_Y }
  emitFrame({ x, y, width: next.width, height: next.height })
}
const handleGestureMedia = (next: { x: number, y: number }) => {
  emitMedia({ x: snap(next.x, 0, SNAP_DISTANCE / 2), y: snap(next.y, 0, SNAP_DISTANCE / 2) })
}
const beginDrag = () => { dragging.value = true }
const endDrag = () => {
  dragging.value = false
  snapped.value = { x: false, y: false }
}

const placeAt = (value: TheaterEffectAnchor) => emitFrame(theaterEffectAnchorPosition(transform.value, value))
const centerFrame = () => emitFrame({ x: CENTER_X, y: CENTER_Y })
const fillStage = () => emitFrame({ x: CENTER_X, y: CENTER_Y, width: THEATER_EFFECT_DESIGN_WIDTH, height: THEATER_EFFECT_DESIGN_HEIGHT })
const resetFrame = () => emitFrame({ ...props.defaultFrame })
const resetMedia = () => emitMedia({ x: 0, y: 0 })

const frameIsDefault = computed(() => (['x', 'y', 'width', 'height'] as const)
  .every((key) => Math.round(transform.value[key]) === Math.round(props.defaultFrame[key])))
const mediaIsDefault = computed(() => mediaPosition.value.x === 0 && mediaPosition.value.y === 0)

const handleCanvasKeydown = (event: KeyboardEvent) => {
  if (!editable.value) return
  const step = event.shiftKey ? 10 : 1
  const delta = {
    ArrowLeft: [-step, 0],
    ArrowRight: [step, 0],
    ArrowUp: [0, -step],
    ArrowDown: [0, step],
  }[event.key]
  if (!delta) return
  event.preventDefault()
  if (activeTarget.value === 'media') emitMedia({ x: mediaPosition.value.x + delta[0], y: mediaPosition.value.y + delta[1] })
  else emitFrame({ x: transform.value.x + delta[0], y: transform.value.y + delta[1] })
}

const numberInput = (apply: (value: number) => void) => (value: number | null) => {
  if (value !== null && Number.isFinite(value)) apply(value)
}
const signed = (value: number) => `${value > 0 ? '+' : ''}${Math.round(value)}`
</script>

<template>
  <div class="tep" :style="rootStyle">
    <div
      class="tep-thumb"
      role="button"
      tabindex="0"
      title="在 16:9 画布中预览并调整位置"
      aria-label="打开位置与预览"
      @click="show = true"
      @keydown.enter.prevent="show = true"
      @keydown.space.prevent="show = true"
    >
      <TheaterEffectOverlay v-if="!show && config.kind !== 'web'" class="tep-overlay" :playbacks="[stillPlayback]" :resolve-media="resolveMedia" />
      <span class="tep-bounds" :style="frameBoundsStyle" />
    </div>
    <div class="tep-meta">
      <span class="tep-meta__line"><b>X</b>{{ Math.round(transform.x) }}<b>Y</b>{{ Math.round(transform.y) }}</span>
      <span class="tep-meta__line is-muted">{{ Math.round(transform.width) }} × {{ Math.round(transform.height) }}<template v-if="anchor"> · {{ theaterEffectAnchorLabels[anchor] }}</template></span>
      <span v-if="hasMedia && !mediaIsDefault" class="tep-meta__line is-muted">媒体偏移 {{ signed(mediaPosition.x) }}, {{ signed(mediaPosition.y) }}</span>
      <n-button size="tiny" secondary class="tep-meta__open" @click="show = true">
        <template #icon><n-icon><Crosshair /></n-icon></template>
        {{ disabled ? '预览位置' : '调整位置' }}
      </n-button>
    </div>

    <n-modal v-model:show="show" :auto-focus="false">
      <div class="tep-dialog" :style="rootStyle" role="dialog" aria-modal="true" aria-label="特效位置与预览">
        <header class="tep-dialog__header">
          <div class="tep-dialog__title">
            <strong>位置与预览</strong>
            <small>{{ object.name || '未命名特效' }} · {{ kindLabel }}</small>
          </div>
          <n-button quaternary circle size="small" aria-label="关闭" @click="show = false">
            <template #icon><n-icon><X /></n-icon></template>
          </n-button>
        </header>

        <div class="tep-dialog__body">
          <section class="tep-stage">
            <div class="tep-toolbar">
              <div v-if="supportsMedia" class="tep-segment" role="radiogroup" aria-label="拖动对象">
                <button type="button" role="radio" :aria-checked="activeTarget === 'frame'" :class="{ 'is-active': activeTarget === 'frame' }" @click="target = 'frame'">
                  <n-icon><Frame /></n-icon>特效框
                </button>
                <button
                  type="button"
                  role="radio"
                  :aria-checked="activeTarget === 'media'"
                  :class="{ 'is-active': activeTarget === 'media' }"
                  :disabled="!hasMedia"
                  :title="hasMedia ? '单独移动特效中的媒体' : '先为特效设置媒体'"
                  @click="target = 'media'"
                >
                  <n-icon><Photo /></n-icon>媒体
                </button>
              </div>
              <span class="tep-toolbar__hint">
                {{ playback ? '正在预览动画…' : disabled ? '只读预览' : activeTarget === 'media' ? '拖动媒体调整内部位置' : '拖动特效框移动，拖右下角圆点缩放' }}
              </span>
              <n-button size="small" :type="playback ? 'default' : 'primary'" secondary @click="playback ? stopPlay() : play()">
                <template #icon><n-icon><component :is="playback ? PlayerStop : PlayerPlay" /></n-icon></template>
                {{ playback ? '停止' : '预览动画' }}
              </n-button>
            </div>

            <div
              ref="canvasRef"
              class="tep-canvas"
              :class="{ 'is-media-target': activeTarget === 'media', 'is-dragging': dragging }"
              :style="canvasStyle"
              tabindex="0"
              aria-label="1920×1080 演出画布，方向键微调位置"
              @keydown="handleCanvasKeydown"
            >
              <TheaterEffectOverlay
                class="tep-overlay"
                :playbacks="stagePlaybacks"
                :selected-object="editable ? object : null"
                :editing="editable"
                :editing-target="activeTarget"
                :resolve-media="resolveMedia"
                @transform-start="beginDrag"
                @transform-update="handleGestureTransform"
                @transform-end="endDrag"
                @media-transform-start="beginDrag"
                @media-transform-update="handleGestureMedia"
                @media-transform-end="endDrag"
              />
              <div class="tep-guides" aria-hidden="true">
                <i class="tep-guides__safe" />
                <i class="tep-guides__v" :class="{ 'is-snapped': snapped.x }" />
                <i class="tep-guides__h" :class="{ 'is-snapped': snapped.y }" />
              </div>
            </div>
            <p class="tep-caption">1920 × 1080 演出画布 · 方向键微调，Shift 每次 10 像素 · 靠近中线自动吸附</p>
          </section>

          <aside class="tep-side">
            <template v-if="activeTarget === 'frame'">
              <div class="tep-group">
                <span class="tep-group__label">位置</span>
                <div class="tep-fields">
                  <n-input-number size="small" :value="Math.round(transform.x)" :show-button="false" :precision="0" :update-value-on-input="false" :disabled="disabled" @update:value="numberInput(x => emitFrame({ x }))">
                    <template #prefix>X</template>
                  </n-input-number>
                  <n-input-number size="small" :value="Math.round(transform.y)" :show-button="false" :precision="0" :update-value-on-input="false" :disabled="disabled" @update:value="numberInput(y => emitFrame({ y }))">
                    <template #prefix>Y</template>
                  </n-input-number>
                </div>
              </div>
              <div class="tep-group">
                <span class="tep-group__label">尺寸</span>
                <div class="tep-fields">
                  <n-input-number size="small" :value="Math.round(transform.width)" :min="MIN_WIDTH" :show-button="false" :precision="0" :update-value-on-input="false" :disabled="disabled" @update:value="numberInput(width => emitFrame({ width }))">
                    <template #prefix>W</template>
                  </n-input-number>
                  <n-input-number size="small" :value="Math.round(transform.height)" :min="MIN_HEIGHT" :show-button="false" :precision="0" :update-value-on-input="false" :disabled="disabled" @update:value="numberInput(height => emitFrame({ height }))">
                    <template #prefix>H</template>
                  </n-input-number>
                </div>
              </div>
              <div class="tep-group">
                <span class="tep-group__label">快速定位</span>
                <div class="tep-anchors" role="group" aria-label="九宫格定位">
                  <button
                    v-for="item in theaterEffectAnchors"
                    :key="item"
                    type="button"
                    :class="{ 'is-active': anchor === item }"
                    :title="theaterEffectAnchorLabels[item]"
                    :aria-label="`对齐到${theaterEffectAnchorLabels[item]}`"
                    :disabled="disabled"
                    @click="placeAt(item)"
                  ><i /></button>
                </div>
              </div>
              <div class="tep-actions">
                <n-button size="small" secondary :disabled="disabled || anchor === 'center'" @click="centerFrame">
                  <template #icon><n-icon><Crosshair /></n-icon></template>居中
                </n-button>
                <n-button size="small" secondary :disabled="disabled" @click="fillStage">
                  <template #icon><n-icon><Maximize /></n-icon></template>铺满
                </n-button>
                <n-button size="small" quaternary :disabled="disabled || frameIsDefault" @click="resetFrame">
                  <template #icon><n-icon><ArrowBackUp /></n-icon></template>恢复默认
                </n-button>
              </div>
            </template>
            <template v-else>
              <div class="tep-group">
                <span class="tep-group__label">媒体偏移</span>
                <div class="tep-fields">
                  <n-input-number size="small" :value="Math.round(mediaPosition.x)" :min="-1920" :max="1920" :show-button="false" :precision="0" :update-value-on-input="false" :disabled="disabled" @update:value="numberInput(x => emitMedia({ x }))">
                    <template #prefix>X</template>
                  </n-input-number>
                  <n-input-number size="small" :value="Math.round(mediaPosition.y)" :min="-1080" :max="1080" :show-button="false" :precision="0" :update-value-on-input="false" :disabled="disabled" @update:value="numberInput(y => emitMedia({ y }))">
                    <template #prefix>Y</template>
                  </n-input-number>
                </div>
              </div>
              <p class="tep-note">相对特效框内的默认媒体位置偏移；缩放、旋转与镜像在属性表单中调整。</p>
              <div class="tep-actions">
                <n-button size="small" secondary :disabled="disabled || mediaIsDefault" @click="resetMedia">
                  <template #icon><n-icon><ArrowBackUp /></n-icon></template>媒体复位
                </n-button>
              </div>
            </template>
          </aside>
        </div>

        <footer class="tep-dialog__footer">
          <span>修改即时写入当前编辑草稿，无需先保存即可预览。</span>
          <n-button size="small" type="primary" @click="show = false">完成</n-button>
        </footer>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.tep { min-width: 0; display: flex; align-items: center; gap: 10px; }
.tep-thumb { position: relative; width: 168px; flex: 0 0 auto; aspect-ratio: 16 / 9; overflow: hidden; border: 1px solid var(--theater-border, var(--sc-border-strong)); border-radius: 6px; background: #10141c; cursor: zoom-in; isolation: isolate; }
.tep-thumb:hover, .tep-thumb:focus-visible { border-color: var(--tep-accent); outline: none; box-shadow: 0 0 0 2px color-mix(in srgb, var(--tep-accent) 28%, transparent); }
.tep-bounds { position: absolute; z-index: 1; border: 1px dashed rgba(251, 191, 36, .85); pointer-events: none; }
.tep-meta { min-width: 0; display: grid; gap: 3px; justify-items: start; font-size: 12px; font-variant-numeric: tabular-nums; }
.tep-meta__line { color: var(--sc-text-primary); }
.tep-meta__line b { margin: 0 3px 0 6px; color: var(--sc-text-secondary); font-weight: 500; }
.tep-meta__line b:first-child { margin-left: 0; }
.tep-meta__line.is-muted { color: var(--sc-text-secondary); }
.tep-meta__open { margin-top: 3px; }

/* The real overlay is a fixed-stacking layer; keep it inside its host box. */
.tep-overlay { z-index: 0 !important; }
.tep :deep(.theater-effect-object.is-editor), .tep-dialog :deep(.theater-effect-object.is-editor) { opacity: 1; }

.tep-dialog { width: min(1080px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); display: flex; flex-direction: column; overflow: hidden; border: 1px solid var(--sc-border-strong); border-radius: 12px; color: var(--sc-text-primary); background: var(--sc-bg-surface); box-shadow: 0 24px 80px rgba(0, 0, 0, .32); }
.tep-dialog__header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--sc-border-strong); }
.tep-dialog__title { min-width: 0; display: grid; gap: 1px; }
.tep-dialog__title strong { font-size: 15px; }
.tep-dialog__title small { overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.tep-dialog__body { min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) 232px; gap: 16px; overflow: auto; padding: 14px 16px; }
.tep-dialog__footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 16px; border-top: 1px solid var(--sc-border-strong); color: var(--sc-text-secondary); font-size: 12px; }

.tep-stage { min-width: 0; display: grid; gap: 8px; align-content: start; }
.tep-toolbar { min-width: 0; display: flex; align-items: center; gap: 10px; }
.tep-toolbar__hint { min-width: 0; flex: 1; overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.tep-segment { display: inline-flex; flex: 0 0 auto; padding: 2px; border: 1px solid var(--sc-border-strong); border-radius: 7px; background: var(--sc-bg-elevated, transparent); }
.tep-segment button { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px; border: 0; border-radius: 5px; color: var(--sc-text-secondary); background: transparent; font: inherit; font-size: 12px; cursor: pointer; }
.tep-segment button.is-active { color: #fff; background: var(--tep-accent); }
.tep-segment button:disabled { opacity: .45; cursor: not-allowed; }
.tep-segment button:focus-visible { outline: 2px solid var(--tep-accent); outline-offset: 1px; }

.tep-canvas {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: 8px;
  background:
    conic-gradient(#171c26 25%, #12161e 0 50%, #171c26 0 75%, #12161e 0) 0 0 / 28px 28px;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, .08);
  isolation: isolate;
  touch-action: none;
}
.tep-canvas:focus-visible { outline: 2px solid var(--tep-accent); outline-offset: 2px; }
.tep-canvas.is-dragging { cursor: grabbing; }
/* Overlay chrome is authored in 1920-wide design pixels; --tep-ui scales it back to screen pixels. */
.tep-canvas :deep(.theater-effect-selection-label) { display: none; }
.tep-canvas :deep(.theater-effect-overlay.is-editing .theater-effect-object.is-selected) { outline-width: calc(2px * var(--tep-ui)); }
.tep-canvas :deep(.theater-effect-resize-handle) { right: calc(-8px * var(--tep-ui)); bottom: calc(-8px * var(--tep-ui)); width: calc(16px * var(--tep-ui)); height: calc(16px * var(--tep-ui)); border-width: calc(2px * var(--tep-ui)); }
/* In frame mode the built-in media layer must not swallow the frame drag. */
.tep-canvas:not(.is-media-target) :deep(.theater-effect-cutin__media) { pointer-events: none; }
.tep-canvas.is-media-target :deep(.theater-effect-cutin__media),
.tep-canvas.is-media-target :deep(.theater-effect-media-only__asset),
.tep-canvas.is-media-target :deep(.theater-effect-media-only video),
.tep-canvas.is-media-target :deep(.theater-effect-media-only canvas) { cursor: grab; }
.tep-canvas.is-media-target :deep(.theater-effect-cutin__media),
.tep-canvas.is-media-target :deep(.theater-effect-media-only__asset) { outline: calc(1.5px * var(--tep-ui)) dashed rgba(251, 191, 36, .9); outline-offset: calc(-1.5px * var(--tep-ui)); }

.tep-guides { position: absolute; z-index: 1; inset: 0; pointer-events: none; }
.tep-guides i { position: absolute; display: block; }
.tep-guides__safe { inset: 5%; border: 1px dashed rgba(255, 255, 255, .12); }
.tep-guides__v { top: 0; bottom: 0; left: 50%; width: 0; border-left: 1px dashed rgba(255, 255, 255, .14); }
.tep-guides__h { top: 50%; right: 0; left: 0; height: 0; border-top: 1px dashed rgba(255, 255, 255, .14); }
.tep-guides i.is-snapped { border-color: var(--tep-accent); border-style: solid; }
.tep-caption { margin: 0; color: var(--sc-text-secondary); font-size: 11px; }

.tep-side { min-width: 0; display: grid; gap: 14px; align-content: start; }
.tep-group { display: grid; gap: 6px; }
.tep-group__label { color: var(--sc-text-secondary); font-size: 12px; }
.tep-fields { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.tep-fields :deep(.n-input__prefix) { margin-right: 6px; color: var(--sc-text-secondary); font-size: 11px; font-weight: 600; }
.tep-anchors { width: 108px; display: grid; grid-template-columns: repeat(3, 1fr); gap: 3px; padding: 4px; border: 1px solid var(--sc-border-strong); border-radius: 8px; aspect-ratio: 16 / 9; }
.tep-anchors button { display: grid; place-items: center; padding: 0; border: 0; border-radius: 4px; background: transparent; cursor: pointer; }
.tep-anchors button i { width: 6px; height: 6px; border-radius: 50%; background: var(--sc-text-secondary); opacity: .45; transition: transform .12s ease, opacity .12s ease; }
.tep-anchors button:hover:not(:disabled) { background: color-mix(in srgb, var(--tep-accent) 14%, transparent); }
.tep-anchors button:hover:not(:disabled) i { opacity: .9; transform: scale(1.2); }
.tep-anchors button.is-active i { width: 9px; height: 9px; background: var(--tep-accent); opacity: 1; }
.tep-anchors button:disabled { cursor: not-allowed; }
.tep-anchors button:focus-visible { outline: 2px solid var(--tep-accent); outline-offset: -1px; }
.tep-actions { display: flex; flex-wrap: wrap; gap: 6px; }
.tep-note { margin: 0; color: var(--sc-text-secondary); font-size: 12px; line-height: 1.6; }

@media (max-width: 760px) {
  .tep-dialog { width: 100vw; max-height: 100dvh; border: 0; border-radius: 0; }
  .tep-dialog__body { grid-template-columns: minmax(0, 1fr); padding: 12px; }
  .tep-side { grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); }
  .tep-toolbar__hint { display: none; }
  .tep-toolbar > .n-button { margin-left: auto; }
  .tep-dialog__footer > span { display: none; }
  .tep-dialog__footer { justify-content: flex-end; }
}
@media (max-width: 420px) {
  .tep-thumb { width: 128px; }
}
</style>
