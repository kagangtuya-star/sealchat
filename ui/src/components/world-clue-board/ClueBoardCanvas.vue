<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import ClueBoardClueNode from './ClueBoardClueNode.vue'
import ClueBoardRelationLayer from './ClueBoardRelationLayer.vue'
import ClueBoardRelationQuickBar from './ClueBoardRelationQuickBar.vue'
import type { WorldClueBoardPlacement, WorldClueBoardRelation } from '@/stores/worldClueBoard'
import type { BoardCamera, BoardNodeLayout, BoardDrawingEndpoint } from './boardTypes'
import type { ClueBoardConnectionState } from './useClueBoardInteraction'

const props = defineProps<{
  nodes: BoardNodeLayout[]
  relations: WorldClueBoardRelation[]
  drawingEndpoints?: BoardDrawingEndpoint[]
  camera?: BoardCamera
  selectedId?: string
  selectedIds?: Set<string>
  hiddenIds?: Set<string>
  dimmedIds?: Set<string>
  connection?: ClueBoardConnectionState | null
  interactionLocked?: boolean
}>()

const emit = defineEmits<{
  select: [value: { id: string; additive: boolean }]
  open: [id: string]
  edit: [id: string]
  'select-relation': [id: string]
  'toggle-pin': [id: string]
  'placement-change': [value: { id: string; placement: WorldClueBoardPlacement }]
  'start-connection': [value: { id: string; kind: ClueBoardConnectionState['kind'] }]
  'connect-target': [id: string]
  'connection-cursor': [value: { x: number; y: number } | null]
  'update-relation-label': [value: { id: string; label: string }]
}>()

const drag = ref<{ id: string; pointerId: number; startX: number; startY: number } | null>(null)
const dragOffset = ref<{ x: number; y: number } | null>(null)

const activeCamera = computed(() => props.camera || { offsetX: 0, offsetY: 0, zoom: 1 })
const worldStyle = computed(() => ({ transform: `translate(${activeCamera.value.offsetX}px, ${activeCamera.value.offsetY}px) scale(${activeCamera.value.zoom})` }))
const displayNodes = computed(() => props.nodes.map(node => {
  if (!drag.value || drag.value.id !== node.id || !dragOffset.value) return node
  return { ...node, x: node.x + dragOffset.value.x / activeCamera.value.zoom, y: node.y + dragOffset.value.y / activeCamera.value.zoom }
}))
const selectedNode = computed(() => {
  if (props.selectedIds?.size !== 1) return null
  const id = props.selectedIds.values().next().value as string | undefined
  if (!id || props.hiddenIds?.has(id)) return null
  return displayNodes.value.find(node => node.id === id) || null
})
const quickBarStyle = computed(() => {
  const node = selectedNode.value
  if (!node) return {}
  return {
    left: `${(node.x + node.width / 2) * activeCamera.value.zoom + activeCamera.value.offsetX}px`,
    top: `${(node.y + node.height) * activeCamera.value.zoom + activeCamera.value.offsetY + 10}px`,
  }
})

function onPointermove(event: PointerEvent) {
  if (drag.value && drag.value.pointerId === event.pointerId) {
    dragOffset.value = { x: event.clientX - drag.value.startX, y: event.clientY - drag.value.startY }
    event.preventDefault()
    return
  }
  if (props.connection) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    emit('connection-cursor', { x: event.clientX - rect.left, y: event.clientY - rect.top })
  }
}

function finishPointer(event: PointerEvent, cancelled = false) {
  if (drag.value && drag.value.pointerId === event.pointerId) {
    const current = drag.value
    const node = props.nodes.find(item => item.id === current.id)
    if (!cancelled && node && dragOffset.value && (Math.abs(dragOffset.value.x) > 1 || Math.abs(dragOffset.value.y) > 1)) {
      emit('placement-change', { id: current.id, placement: { x: node.x + dragOffset.value.x / activeCamera.value.zoom, y: node.y + dragOffset.value.y / activeCamera.value.zoom, width: node.width, pinned: node.pinned } })
    }
    drag.value = null
    dragOffset.value = null
  }
}

function beginNodeDrag(node: BoardNodeLayout, event: PointerEvent) {
  if (event.button !== 0 || props.interactionLocked) return
  if (props.connection) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  drag.value = { id: node.id, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY }
  dragOffset.value = { x: 0, y: 0 }
  ;(event.currentTarget as HTMLElement | null)?.setPointerCapture?.(event.pointerId)
  event.stopPropagation()
}

function onNodeSelect(node: BoardNodeLayout, value: { additive: boolean }) {
  if (props.connection) {
    if (props.interactionLocked) return
    if (props.connection.source.kind !== 'clue' || props.connection.source.id !== node.id) emit('connect-target', node.id)
    return
  }
  emit('select', { id: node.id, additive: value.additive })
}

onBeforeUnmount(() => {
  drag.value = null
})
</script>

<template>
  <section class="clue-board-canvas" @pointermove="onPointermove" @pointerup="finishPointer" @pointercancel="event => finishPointer(event, true)">
    <ClueBoardRelationLayer :relations="relations" :nodes="displayNodes" :drawing-endpoints="drawingEndpoints" :camera="activeCamera" :hidden-ids="hiddenIds" :connection="connection" :interaction-locked="interactionLocked" @select="emit('select-relation', $event)" @update-label="!interactionLocked && emit('update-relation-label', $event)" />
    <div class="clue-board-canvas__world" :style="worldStyle">
      <template v-for="node in displayNodes" :key="node.id">
      <ClueBoardClueNode
        :node="node"
        :world-id="node.summary.worldId"
        :selected="selectedIds ? selectedIds.has(node.id) : selectedId === node.id"
        :dimmed="dimmedIds?.has(node.id)"
        :filtered-out="hiddenIds?.has(node.id)"
        :interaction-locked="interactionLocked"
        class="clue-board-canvas__node"
        :class="{ 'is-link-target': connection && !(connection.source.kind === 'clue' && connection.source.id === node.id) }"
        :style="{ left: `${node.x}px`, top: `${node.y}px` }"
        @pointerdown="beginNodeDrag(node, $event)"
        @select="onNodeSelect(node, $event)"
        @open="emit('open', node.id)"
        @edit="emit('edit', node.id)"
        @toggle-pin="!interactionLocked && emit('toggle-pin', node.id)"
      />
      </template>
    </div>
    <ClueBoardRelationQuickBar
      v-if="!interactionLocked && selectedNode && !connection"
      class="clue-board-canvas__quickbar"
      :style="quickBarStyle"
      @start="emit('start-connection', { id: selectedNode.id, kind: $event })"
    />
    <div v-if="!nodes.length" class="clue-board-canvas__empty">当前世界暂无可载入的线索</div>
  </section>
</template>

<style scoped>
.clue-board-canvas { position: relative; z-index: 1; width: 100%; height: 100%; overflow: hidden; outline: none; background: transparent; pointer-events: none; touch-action: none; }
.clue-board-canvas:active { cursor: grabbing; }
.clue-board-canvas__world { position: absolute; inset: 0; z-index: 1; transform-origin: 0 0; pointer-events: none; }
.clue-board-canvas__node { position: absolute; pointer-events: auto; }
.clue-board-canvas__node.is-link-target:hover { outline: 3px solid var(--primary-color, #3388de); cursor: crosshair; }
.clue-board-canvas__quickbar { position: absolute; z-index: 4; transform: translateX(-50%); }
.clue-board-canvas__empty { position: absolute; inset: 0; z-index: 2; display: grid; place-items: center; color: var(--sc-text-secondary); pointer-events: none; }
</style>
