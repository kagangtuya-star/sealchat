import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { WorldClueBoardRelationEndpointRef, WorldClueBoardRelationKind } from '@/stores/worldClueBoard'

export interface ClueBoardConnectionState {
  source: WorldClueBoardRelationEndpointRef
  kind: WorldClueBoardRelationKind
  cursor: { x: number; y: number } | null
}

/** Shared, disposable interaction state for the board's node/edge surfaces. */
export function useClueBoardInteraction() {
  const selectedIds = ref<Set<string>>(new Set())
  const selectedRelationId = ref('')
  const inspectorVisible = ref(false)
  const connection = ref<ClueBoardConnectionState | null>(null)
  const selectedId = computed(() => selectedIds.value.values().next().value as string | undefined)

  function clearSelection() {
    selectedIds.value = new Set()
    selectedRelationId.value = ''
    inspectorVisible.value = false
  }

  function selectNode(id: string, additive = false) {
    selectedRelationId.value = ''
    inspectorVisible.value = false
    if (additive) {
      const next = new Set(selectedIds.value)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      selectedIds.value = next
    } else {
      selectedIds.value = new Set([id])
    }
  }

  function selectRelation(id: string) {
    selectedIds.value = new Set()
    selectedRelationId.value = id
    inspectorVisible.value = true
  }

  function beginConnection(source: WorldClueBoardRelationEndpointRef, kind: WorldClueBoardRelationKind) {
    selectedRelationId.value = ''
    inspectorVisible.value = false
    connection.value = { source, kind, cursor: null }
  }

  function setConnectionCursor(cursor: { x: number; y: number } | null) {
    if (connection.value) connection.value.cursor = cursor
  }

  function cancelConnection() {
    connection.value = null
  }

  function closeInspector() {
    inspectorVisible.value = false
    selectedRelationId.value = ''
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && connection.value) {
      event.preventDefault()
      cancelConnection()
    }
  }

  onMounted(() => window.addEventListener('keydown', onKeydown))
  onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

  return {
    selectedIds,
    selectedRelationId,
    inspectorVisible,
    selectedId,
    connection,
    clearSelection,
    selectNode,
    selectRelation,
    beginConnection,
    setConnectionCursor,
    cancelConnection,
    closeInspector,
  }
}
