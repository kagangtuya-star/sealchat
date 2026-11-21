<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { NIcon } from 'naive-ui'
import { AlertTriangle, EyeOff, GripVertical } from '@vicons/tabler'
import { moveWorkbenchPanel, resizeWorkbenchPanel } from './layout'
import { workbenchPanelTitle, workbenchSourceTypeLabel } from './sources'
import type { WorkbenchLayout, WorkbenchPanel, WorkbenchRect } from './types'

// 设计画布：只渲染占位卡片（不加载 iframe），指针拖拽按网格吸附，落点冲突时整体交换或拒绝。
// 键盘：方向键移动，Shift+方向键调整尺寸，Delete 删除，Enter 打开属性。

const props = defineProps<{
  layout: WorkbenchLayout
  selectedId: string
  issues: Record<string, string>
}>()

const emit = defineEmits<{
  select: [id: string]
  change: [layout: WorkbenchLayout]
  rejected: [message: string]
  remove: [id: string]
}>()

const gridRef = ref<HTMLElement | null>(null)
const ghost = ref<{ id: string; rect: WorkbenchRect; valid: boolean } | null>(null)

type Interaction = {
  kind: 'move' | 'resize'
  id: string
  pointerId: number
  start: WorkbenchRect
  grabCol: number
  grabRow: number
  moved: boolean
}
let interaction: Interaction | null = null

const cellCount = computed(() => props.layout.grid.columns * props.layout.grid.rows)
const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${props.layout.grid.columns}, minmax(0, 1fr))`,
  gridTemplateRows: `repeat(${props.layout.grid.rows}, minmax(var(--sc-workbench-row-min, 32px), 1fr))`,
  gap: `${props.layout.grid.gap}px`,
}))
const placement = (rect: WorkbenchRect) => ({
  gridColumn: `${rect.x + 1} / span ${rect.w}`,
  gridRow: `${rect.y + 1} / span ${rect.h}`,
})

// 背景格子显式定位，避免自动排布挤出隐式行。
const cellPlacement = (index: number) => ({
  gridColumn: `${(index % props.layout.grid.columns) + 1}`,
  gridRow: `${Math.floor(index / props.layout.grid.columns) + 1}`,
})

const cellAt = (clientX: number, clientY: number) => {
  const grid = gridRef.value
  if (!grid) return null
  const box = grid.getBoundingClientRect()
  const { columns, rows, gap } = props.layout.grid
  const cellW = (box.width - gap * (columns - 1)) / columns
  const cellH = (box.height - gap * (rows - 1)) / rows
  return {
    col: Math.floor((clientX - box.left + gap / 2) / (cellW + gap)),
    row: Math.floor((clientY - box.top + gap / 2) / (cellH + gap)),
  }
}

const previewFor = (active: Interaction, clientX: number, clientY: number) => {
  const cell = cellAt(clientX, clientY)
  if (!cell) return null
  if (active.kind === 'move') {
    const next = moveWorkbenchPanel(props.layout, active.id, cell.col - active.grabCol, cell.row - active.grabRow)
    const rect = next?.panels.find(panel => panel.id === active.id)?.rect
    const { columns, rows } = props.layout.grid
    const fallback = {
      ...active.start,
      x: Math.min(Math.max(cell.col - active.grabCol, 0), columns - active.start.w),
      y: Math.min(Math.max(cell.row - active.grabRow, 0), rows - active.start.h),
    }
    return { layout: next, rect: rect || fallback }
  }
  const next = resizeWorkbenchPanel(props.layout, active.id, cell.col - active.start.x + 1, cell.row - active.start.y + 1)
  const rect = next?.panels.find(panel => panel.id === active.id)?.rect
  return {
    layout: next,
    rect: rect || {
      ...active.start,
      w: Math.min(Math.max(cell.col - active.start.x + 1, 1), props.layout.grid.columns - active.start.x),
      h: Math.min(Math.max(cell.row - active.start.y + 1, 1), props.layout.grid.rows - active.start.y),
    },
  }
}

const detachWindowListeners = () => {
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', handlePointerUp)
  window.removeEventListener('pointercancel', handlePointerCancel)
}

const begin = (kind: Interaction['kind'], panel: WorkbenchPanel, event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  event.preventDefault()
  emit('select', panel.id)
  const cell = cellAt(event.clientX, event.clientY)
  if (!cell) return
  interaction = {
    kind,
    id: panel.id,
    pointerId: event.pointerId,
    start: { ...panel.rect },
    grabCol: cell.col - panel.rect.x,
    grabRow: cell.row - panel.rect.y,
    moved: false,
  }
  window.addEventListener('pointermove', handlePointerMove)
  window.addEventListener('pointerup', handlePointerUp)
  window.addEventListener('pointercancel', handlePointerCancel)
}

function handlePointerMove(event: PointerEvent) {
  const active = interaction
  if (!active || event.pointerId !== active.pointerId) return
  const preview = previewFor(active, event.clientX, event.clientY)
  if (!preview) return
  const same = preview.rect.x === active.start.x && preview.rect.y === active.start.y && preview.rect.w === active.start.w && preview.rect.h === active.start.h
  if (!same) active.moved = true
  ghost.value = active.moved ? { id: active.id, rect: preview.rect, valid: !!preview.layout } : null
}

function handlePointerUp(event: PointerEvent) {
  const active = interaction
  if (!active || event.pointerId !== active.pointerId) return
  const preview = active.moved ? previewFor(active, event.clientX, event.clientY) : null
  interaction = null
  ghost.value = null
  detachWindowListeners()
  if (!preview) return
  if (preview.layout) {
    if (preview.layout !== props.layout) emit('change', preview.layout)
  } else {
    emit('rejected', active.kind === 'move' ? '目标位置放不下该面板' : '尺寸会与其他面板重叠')
  }
}

function handlePointerCancel(event: PointerEvent) {
  if (!interaction || event.pointerId !== interaction.pointerId) return
  interaction = null
  ghost.value = null
  detachWindowListeners()
}

const handleKeydown = (panel: WorkbenchPanel, event: KeyboardEvent) => {
  const deltas: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }
  if (event.key === 'Delete' || event.key === 'Backspace') {
    event.preventDefault()
    emit('remove', panel.id)
    return
  }
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    emit('select', panel.id)
    return
  }
  const delta = deltas[event.key]
  if (!delta) return
  event.preventDefault()
  emit('select', panel.id)
  const next = event.shiftKey
    ? resizeWorkbenchPanel(props.layout, panel.id, panel.rect.w + delta[0], panel.rect.h + delta[1])
    : moveWorkbenchPanel(props.layout, panel.id, panel.rect.x + delta[0], panel.rect.y + delta[1])
  if (!next) emit('rejected', event.shiftKey ? '尺寸会与其他面板重叠' : '目标位置放不下该面板')
  else if (next !== props.layout) emit('change', next)
}

onBeforeUnmount(() => {
  interaction = null
  detachWindowListeners()
})
</script>

<template>
  <div ref="gridRef" class="wb-design" :style="gridStyle" data-sc-ui="workbench-design-canvas" @pointerdown.self="emit('select', '')">
    <span v-for="index in cellCount" :key="`cell-${index}`" class="wb-design__cell" :style="cellPlacement(index - 1)" aria-hidden="true" />
    <div
      v-for="panel in layout.panels"
      :key="panel.id"
      class="wb-design__panel"
      :class="{ 'is-selected': panel.id === selectedId, 'is-dragging': ghost?.id === panel.id, 'has-issue': !!issues[panel.id] }"
      :style="placement(panel.rect)"
      role="button"
      tabindex="0"
      :aria-label="`${workbenchPanelTitle(panel)}，${workbenchSourceTypeLabel(panel.source)}，第 ${panel.rect.x + 1} 列第 ${panel.rect.y + 1} 行，宽 ${panel.rect.w} 高 ${panel.rect.h}`"
      :aria-pressed="panel.id === selectedId"
      @pointerdown="begin('move', panel, $event)"
      @keydown="handleKeydown(panel, $event)"
    >
      <span class="wb-design__handle" aria-hidden="true"><NIcon><GripVertical /></NIcon></span>
      <span class="wb-design__meta">
        <strong>{{ workbenchPanelTitle(panel) }}</strong>
        <small>{{ workbenchSourceTypeLabel(panel.source) }} · {{ panel.rect.w }}×{{ panel.rect.h }}</small>
        <small v-if="issues[panel.id]" class="wb-design__issue"><NIcon><AlertTriangle /></NIcon>{{ issues[panel.id] }}</small>
        <small v-if="panel.hiddenByDefault" class="wb-design__muted"><NIcon><EyeOff /></NIcon>默认对玩家隐藏</small>
      </span>
      <span
        class="wb-design__resize"
        aria-hidden="true"
        @pointerdown.stop="begin('resize', panel, $event)"
      />
    </div>
    <div
      v-if="ghost"
      class="wb-design__ghost"
      :class="{ 'is-invalid': !ghost.valid }"
      :style="placement(ghost.rect)"
      aria-hidden="true"
    />
  </div>
</template>

<style scoped>
.wb-design { position: relative; display: grid; min-height: 100%; touch-action: none; user-select: none; }
.wb-design__cell { border-radius: 4px; background: color-mix(in srgb, var(--sc-text-secondary, #888) 8%, transparent); pointer-events: none; }
.wb-design__panel { position: relative; z-index: 1; display: flex; min-width: 0; min-height: 0; gap: 6px; padding: 8px 10px; overflow: hidden; border: 1px solid var(--sc-border-strong, rgba(127, 127, 127, .35)); border-radius: var(--sc-workbench-panel-radius, 10px); color: var(--sc-text-primary, inherit); background: var(--sc-workbench-panel-bg, var(--sc-bg-elevated, #fff)); cursor: grab; outline: none; }
.wb-design__panel:focus-visible, .wb-design__panel.is-selected { border-color: var(--sc-workbench-accent, var(--primary-color, #3b82f6)); box-shadow: 0 0 0 2px color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 40%, transparent); }
.wb-design__panel.has-issue { border-style: dashed; border-color: var(--sc-color-warning, #f0a020); }
.wb-design__panel.is-dragging { opacity: .45; }
.wb-design__handle { flex: none; color: var(--sc-text-secondary, #888); }
.wb-design__meta { display: flex; min-width: 0; flex-direction: column; gap: 2px; }
.wb-design__meta strong { overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.wb-design__meta small { display: inline-flex; align-items: center; gap: 3px; color: var(--sc-text-secondary, #888); font-size: 12px; }
.wb-design__issue { color: var(--sc-color-warning, #f0a020) !important; }
.wb-design__resize { position: absolute; right: 0; bottom: 0; width: 16px; height: 16px; border-right: 3px solid var(--sc-border-strong, #999); border-bottom: 3px solid var(--sc-border-strong, #999); border-bottom-right-radius: 8px; cursor: nwse-resize; }
.wb-design__ghost { z-index: 2; border: 2px dashed var(--sc-workbench-accent, var(--primary-color, #3b82f6)); border-radius: var(--sc-workbench-panel-radius, 10px); background: color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 14%, transparent); pointer-events: none; }
.wb-design__ghost.is-invalid { border-color: var(--sc-color-error, #d03050); background: color-mix(in srgb, var(--sc-color-error, #d03050) 14%, transparent); }
</style>
