<script setup lang="ts">
import { computed, nextTick, ref, type ComponentPublicInstance } from 'vue'
import type { WorldClueBoardRelation } from '@/stores/worldClueBoard'
import { WORLD_CLUE_BOARD_RELATION_KIND_LABELS as relationKindLabels } from '@/stores/worldClueBoard'
import type { BoardCamera, BoardNodeLayout, BoardRelationEndpoint, BoardElementGeometry, BoardDrawingEndpoint } from './boardTypes'
import type { ClueBoardConnectionState } from './useClueBoardInteraction'

const props = defineProps<{
  relations: WorldClueBoardRelation[]
  nodes: BoardNodeLayout[]
  drawingEndpoints?: BoardDrawingEndpoint[]
  camera: BoardCamera
  hiddenIds?: Set<string>
  connection?: ClueBoardConnectionState | null
  interactionLocked?: boolean
}>()
const emit = defineEmits<{
  select: [id: string]
  'update-label': [value: { id: string; label: string }]
}>()

const editingId = ref('')
const editingLabel = ref('')
const editInput = ref<HTMLInputElement | null>(null)

function setEditInput(element: Element | ComponentPublicInstance | null) {
  editInput.value = element instanceof HTMLInputElement ? element : null
}

const pointOnRect = (node: BoardElementGeometry, dx: number, dy: number) => {
  if (!dx && !dy) return { x: node.x + node.width / 2, y: node.y }
  const scale = Math.min(
    Math.abs(dx) > 0 ? (node.width / 2) / Math.abs(dx) : Number.POSITIVE_INFINITY,
    Math.abs(dy) > 0 ? (node.height / 2) / Math.abs(dy) : Number.POSITIVE_INFINITY,
  )
  return { x: node.x + node.width / 2 + dx * scale, y: node.y + node.height / 2 + dy * scale }
}

const endpoints = computed<Array<BoardRelationEndpoint & { parallelIndex: number }>>(() => {
  const nodes = new Map(props.nodes.map(node => [node.id, node]))
  const drawings = new Map(props.drawingEndpoints?.map(node => [node.id, node]))
  const hidden = props.hiddenIds || new Set<string>()
  const visible = props.relations.flatMap((relation) => {
    const source = relation.sourceRef.kind === 'clue' ? nodes.get(relation.sourceRef.id) : drawings.get(relation.sourceRef.id)
    const target = relation.targetRef.kind === 'clue' ? nodes.get(relation.targetRef.id) : drawings.get(relation.targetRef.id)
    if (!source || !target || (relation.sourceRef.kind === 'clue' && hidden.has(source.id)) || (relation.targetRef.kind === 'clue' && hidden.has(target.id))) return []
    return [{ relation, source, target, parallelIndex: 0 }]
  })
  const groups = new Map<string, typeof visible>()
  for (const endpoint of visible) {
    const ids = [endpoint.source.id, endpoint.target.id].sort().join('|')
    const group = groups.get(ids) || []
    group.push(endpoint)
    groups.set(ids, group)
  }
  for (const group of groups.values()) group.forEach((endpoint, index) => { endpoint.parallelIndex = index - (group.length - 1) / 2 })
  return visible
})

function screenPoint(node: BoardElementGeometry, x: number, y: number) {
  return { x: x * props.camera.zoom + props.camera.offsetX, y: y * props.camera.zoom + props.camera.offsetY }
}

function lineFor(endpoint: BoardRelationEndpoint & { parallelIndex: number }) {
  const dx = endpoint.target.x + endpoint.target.width / 2 - endpoint.source.x - endpoint.source.width / 2
  const dy = endpoint.target.y + endpoint.target.height / 2 - endpoint.source.y - endpoint.source.height / 2
  const startPoint = pointOnRect(endpoint.source, dx, dy)
  const endPoint = pointOnRect(endpoint.target, -dx, -dy)
  const start = screenPoint(endpoint.source, startPoint.x, startPoint.y)
  const end = screenPoint(endpoint.target, endPoint.x, endPoint.y)
  const middle = { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 }
  const length = Math.hypot(end.x - start.x, end.y - start.y) || 1
  const offset = endpoint.parallelIndex * 18
  const control = { x: middle.x - ((end.y - start.y) / length) * offset, y: middle.y + ((end.x - start.x) / length) * offset }
  const label = { x: (start.x + 2 * control.x + end.x) / 4, y: (start.y + 2 * control.y + end.y) / 4 }
  return { start, end, control, label }
}

function pathFor(endpoint: BoardRelationEndpoint & { parallelIndex: number }) {
  const line = lineFor(endpoint)
  return { d: `M ${line.start.x} ${line.start.y} Q ${line.control.x} ${line.control.y} ${line.end.x} ${line.end.y}`, label: line.label }
}

const preview = computed(() => {
  const connection = props.connection
  if (!connection?.cursor) return null
  const source = connection.source.kind === 'clue'
    ? props.nodes.find(node => node.id === connection.source.id)
    : props.drawingEndpoints?.find(node => node.id === connection.source.id)
  if (!source) return null
  const start = screenPoint(source, source.x + source.width / 2, source.y + source.height / 2)
  const cursor = connection.cursor
  return { d: `M ${start.x} ${start.y} Q ${(start.x + cursor.x) / 2} ${(start.y + cursor.y) / 2 - 22} ${cursor.x} ${cursor.y}` }
})

function beginEdit(endpoint: BoardRelationEndpoint & { parallelIndex: number }) {
  if (props.interactionLocked) return
  editingId.value = endpoint.relation.id
  editingLabel.value = endpoint.relation.label || ''
  void nextTick(() => editInput.value?.focus())
}

function finishEdit() {
  if (!editingId.value) return
  emit('update-label', { id: editingId.value, label: editingLabel.value.trim() })
  editingId.value = ''
  editInput.value = null
}

function cancelEdit() {
  editingId.value = ''
  editInput.value = null
}

function relationClass(kind: string) { return `relation-${kind}` }
</script>

<template>
  <svg class="clue-board-relations" :class="{ 'is-locked': interactionLocked }" aria-label="线索关系层">
    <defs>
      <marker id="clue-board-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="strokeWidth">
        <path d="M0,0 L8,4 L0,8 z" fill="currentColor" />
      </marker>
    </defs>
    <g v-for="endpoint in endpoints" :key="endpoint.relation.id" :class="relationClass(endpoint.relation.kind)" @pointerdown.stop @click.stop="emit('select', endpoint.relation.id)">
      <path
        :d="pathFor(endpoint).d"
        :marker-end="endpoint.relation.kind === 'related' || endpoint.relation.kind === 'contradicts' ? undefined : 'url(#clue-board-arrow)'"
      />
      <text :x="pathFor(endpoint).label.x" :y="pathFor(endpoint).label.y - 6" @dblclick.stop="beginEdit(endpoint)">{{ endpoint.relation.label?.trim() || relationKindLabels[endpoint.relation.kind] }}</text>
      <foreignObject v-if="editingId === endpoint.relation.id" :x="pathFor(endpoint).label.x - 84" :y="pathFor(endpoint).label.y - 34" width="168" height="30" @pointerdown.stop>
        <input :ref="setEditInput" v-model="editingLabel" class="clue-board-relation-edit" maxlength="500" @keydown.enter.prevent="finishEdit" @keydown.esc.prevent="cancelEdit" @blur="finishEdit" />
      </foreignObject>
    </g>
    <path v-if="preview" class="relation-preview" :d="preview.d" />
  </svg>
</template>

<style scoped>
.clue-board-relations { position: absolute; inset: 0; z-index: 0; width: 100%; height: 100%; overflow: visible; color: var(--primary-color, #3388de); pointer-events: none; }
.clue-board-relations g { pointer-events: visiblePainted; cursor: pointer; }
.clue-board-relations path { fill: none; stroke: currentColor; stroke-width: 1.8; opacity: .72; pointer-events: stroke; }
.clue-board-relations text { fill: var(--sc-text-secondary); stroke: var(--sc-bg-surface); stroke-width: 3px; paint-order: stroke; font-size: 11px; text-anchor: middle; cursor: text; user-select: none; }
.clue-board-relations.is-locked text { cursor: default; }
.clue-board-relation-edit { width: 100%; box-sizing: border-box; padding: 3px 6px; border: 1px solid var(--primary-color, #3388de); border-radius: 5px; color: var(--sc-text-primary); background: var(--sc-bg-elevated); font: inherit; }
.relation-preview { stroke-dasharray: 5 4; opacity: .48; pointer-events: none; }
.relation-contradicts { color: #d96c6c; }
.relation-causes { color: #9c79dd; }
.relation-supports { color: #56a97a; }
.relation-references { color: #d5a34b; }
</style>
