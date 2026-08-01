<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import type { Dice3DDockStack } from '@/types'

const props = defineProps<{
  enabled: boolean
  x?: number
  y?: number
  corner?: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right' | 'free'
  stacks?: Dice3DDockStack[]
}>()

const emit = defineEmits<{
  (event: 'roll', expression: string): void
  (event: 'move', position: { x: number, y: number }): void
}>()

const dragging = ref(false)
const moved = ref(false)
const resolvePosition = () => {
  switch (props.corner) {
    case 'top-left': return { x: 0.1, y: 0.15 }
    case 'top-right': return { x: 0.9, y: 0.15 }
    case 'bottom-left': return { x: 0.1, y: 0.82 }
    case 'bottom-right': return { x: 0.9, y: 0.82 }
    default: return { x: props.x ?? 0.9, y: props.y ?? 0.82 }
  }
}
const position = ref(resolvePosition())
let start = { x: 0, y: 0, px: 0, py: 0 }
let lastMove = { x: 0, y: 0, time: 0 }
let flingSpeed = 0
let selectedExpression = '.r2d6'

const stacks = computed<Dice3DDockStack[]>(() => props.stacks?.length
  ? props.stacks.slice(0, 8)
  : [{ id: 'default-2d6', label: '2d6', expression: '.r2d6', color: '#f5f6fa' }])
const style = computed(() => ({ left: `${position.value.x * 100}%`, top: `${position.value.y * 100}%` }))

watch(() => [props.x, props.y, props.corner] as const, () => {
  if (dragging.value) return
  position.value = resolvePosition()
})

const onPointerDown = (event: PointerEvent) => {
  dragging.value = true
  moved.value = false
  start = { x: event.clientX, y: event.clientY, px: position.value.x, py: position.value.y }
  lastMove = { x: event.clientX, y: event.clientY, time: performance.now() }
  flingSpeed = 0
  selectedExpression = (event.target as HTMLElement | null)
    ?.closest<HTMLElement>('[data-expression]')?.dataset.expression || stacks.value[0].expression
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

const onPointerMove = (event: PointerEvent) => {
  if (!dragging.value) return
  const dx = event.clientX - start.x
  const dy = event.clientY - start.y
  if (Math.abs(dx) + Math.abs(dy) > 5) moved.value = true
  const now = performance.now()
  const elapsed = Math.max(1, now - lastMove.time)
  flingSpeed = Math.hypot(event.clientX - lastMove.x, event.clientY - lastMove.y) / elapsed
  lastMove = { x: event.clientX, y: event.clientY, time: now }
  position.value = {
    x: Math.max(0.04, Math.min(0.96, start.px + dx / window.innerWidth)),
    y: Math.max(0.08, Math.min(0.94, start.py + dy / window.innerHeight)),
  }
}

const onPointerUp = () => {
  dragging.value = false
  if (moved.value) emit('move', position.value)
  if (!moved.value || flingSpeed > 0.75) emit('roll', selectedExpression)
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="enabled"
      class="dice3d-dock"
      :class="{ 'is-dragging': dragging }"
      :style="style"
      role="group"
      title="拖动骰子堆；点击或快速甩动进行投掷"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="dragging = false"
    >
      <button
        v-for="(stack, index) in stacks"
        :key="stack.id"
        class="dice3d-dock__die"
        :data-expression="stack.expression"
        :style="{
          '--dice-stack-color': stack.color || '#f5f6fa',
          transform: `translate(${index * 8}px, ${index * -5}px) rotate(${(index - 1) * 7}deg)`,
          zIndex: index + 1,
        }"
        type="button"
        :title="`投掷 ${stack.expression}`"
      >{{ stack.label || stack.expression }}</button>
    </div>
  </Teleport>
</template>

<style scoped>
.dice3d-dock { position: fixed; z-index: 9600; min-width: 72px; height: 72px; padding: 0; border: 0; background: transparent; transform: translate(-50%, -50%); cursor: grab; touch-action: none; }
.dice3d-dock.is-dragging { cursor: grabbing; }
.dice3d-dock__die { position: absolute; left: 8px; bottom: 8px; display: grid; place-items: center; min-width: 46px; height: 42px; padding: 0 7px; border: 1px solid #d1d5db; border-radius: 10px; color: #111827; background: var(--dice-stack-color); box-shadow: 0 8px 18px rgba(0,0,0,.24); font: 700 13px/1 system-ui; cursor: pointer; }
</style>
