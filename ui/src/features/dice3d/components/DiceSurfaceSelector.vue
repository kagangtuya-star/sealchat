<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Dice3DCustomSurface } from '@/types'

const props = defineProps<{ modelValue: Dice3DCustomSurface }>()
const emit = defineEmits<{ 'update:modelValue': [value: Dice3DCustomSurface] }>()
const stageRef = ref<HTMLElement | null>(null)
const selecting = ref(false)
let start = { x: 0, y: 0 }
const rectStyle = computed(() => ({ left: `${props.modelValue.x * 100}%`, top: `${props.modelValue.y * 100}%`, width: `${props.modelValue.width * 100}%`, height: `${props.modelValue.height * 100}%` }))

const point = (event: PointerEvent) => {
  const rect = stageRef.value!.getBoundingClientRect()
  return { x: Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width)), y: Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height)) }
}
const onPointerDown = (event: PointerEvent) => {
  if (!stageRef.value) return
  selecting.value = true
  start = point(event)
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  emit('update:modelValue', { x: start.x, y: start.y, width: 0.1, height: 0.1 })
}
const onPointerMove = (event: PointerEvent) => {
  if (!selecting.value) return
  const current = point(event)
  let x = Math.min(start.x, current.x)
  let y = Math.min(start.y, current.y)
  const width = Math.min(1, Math.max(0.1, Math.abs(current.x - start.x)))
  const height = Math.min(1, Math.max(0.1, Math.abs(current.y - start.y)))
  x = Math.min(x, 1 - width)
  y = Math.min(y, 1 - height)
  emit('update:modelValue', { x, y, width, height })
}
const onPointerUp = () => { selecting.value = false }
</script>

<template>
  <div ref="stageRef" class="surface-selector" @pointerdown="onPointerDown" @pointermove="onPointerMove" @pointerup="onPointerUp" @pointercancel="onPointerUp">
    <div class="surface-selector__sidebar" />
    <div class="surface-selector__header" />
    <div class="surface-selector__messages"><i v-for="line in 5" :key="line" /></div>
    <div class="surface-selector__rect" :style="rectStyle"><span>3D 骰子显示区域</span></div>
    <div class="surface-selector__hint">按住拖动框选区域</div>
  </div>
</template>

<style scoped>
.surface-selector { position: relative; width: 100%; aspect-ratio: 16 / 8; min-height: 170px; overflow: hidden; border: 1px solid var(--sc-border-muted, rgba(148,163,184,.26)); border-radius: 10px; background: color-mix(in srgb, var(--sc-bg-input, #111827) 88%, #000); cursor: crosshair; touch-action: none; user-select: none; }
.surface-selector__sidebar { position: absolute; inset: 0 auto 0 0; width: 20%; background: rgba(148,163,184,.08); border-right: 1px solid rgba(148,163,184,.16); }.surface-selector__header { position: absolute; left: 20%; right: 0; top: 0; height: 16%; border-bottom: 1px solid rgba(148,163,184,.16); }
.surface-selector__messages { position: absolute; left: 25%; top: 24%; width: 46%; display: grid; gap: 10px; }.surface-selector__messages i { height: 8px; border-radius: 999px; background: rgba(148,163,184,.12); }.surface-selector__messages i:nth-child(even) { width: 72%; }
.surface-selector__rect { position: absolute; display: grid; place-items: center; min-width: 10%; min-height: 10%; border: 2px solid #36ad92; background: rgba(54,173,146,.16); box-shadow: 0 0 0 1px rgba(255,255,255,.12) inset; pointer-events: none; }.surface-selector__rect span { padding: 4px 7px; border-radius: 5px; color: #d7fff6; background: rgba(6,78,59,.72); font-size: 11px; }
.surface-selector__hint { position: absolute; right: 8px; bottom: 7px; color: var(--sc-text-secondary); font-size: 11px; pointer-events: none; }
</style>
