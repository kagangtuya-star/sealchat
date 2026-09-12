<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, toRaw, watch } from 'vue'
import { NAlert, NButton, NButtonGroup, NCheckbox, NDropdown, NIcon, NInput, NSpace, NTag, NTooltip, useMessage } from 'naive-ui'
import { AlertCircle, CircleCheck, CloudLock, CloudUpload, Folder, LayoutBoard, Refresh, Rotate2, Search, Star } from '@vicons/tabler'
import { api } from '@/stores/_config'
import { chatEvent, useChatStore } from '@/stores/chat'
import { useDisplayStore } from '@/stores/display'
import { useUserStore } from '@/stores/user'
import { useWorldClueStore, type WorldClueDetail, type WorldClueSummary } from '@/stores/worldClue'
import { useWorldClueBoardStore, type WorldClueBoardEventPayload, type WorldClueBoardScope, type WorldClueBoardPlacement, type WorldClueBoardRelation, type WorldClueBoardRelationEndpointRef, type WorldClueBoardRelationKind } from '@/stores/worldClueBoard'
import WorldCluePresentationHost from '@/components/world-clue/WorldCluePresentationHost.vue'
import WorldClueEditorModal from '@/views/chat/components/clue-box/WorldClueEditorModal.vue'
import ClueBoardCanvas from './ClueBoardCanvas.vue'
import ClueBoardDrawingSurface from './ClueBoardDrawingSurface.vue'
import ClueBoardRelationInspector from './ClueBoardRelationInspector.vue'
import ClueBoardRelationQuickBar from './ClueBoardRelationQuickBar.vue'
import { arrangeLayouts, buildNodeLayouts } from './boardLayout'
import type { BoardCamera, BoardDrawingEndpoint } from './boardTypes'
import { QUICKDRAW_ENGINE_VERSION } from './quickdraw-adapter'
import type { Diff, Snapshot } from '@quickdrawjs/core'
import { useClueBoardInteraction } from './useClueBoardInteraction'

const props = defineProps<{ resourceId: string; worldId: string; channelId: string }>()
const emit = defineEmits<{ ready: []; unavailable: [message: string]; error: [message: string] }>()

const worldClue = useWorldClueStore()
const board = useWorldClueBoardStore()
const chat = useChatStore()
const display = useDisplayStore()
const user = useUserStore()
const message = useMessage()

const searchText = ref('')
const onlySearchResults = ref(false)
const matchingIds = ref<Set<string> | null>(null)
const searchResults = ref<WorldClueSummary[]>([])
const searchFocused = ref(false)
const searchLoading = ref(false)
const searchError = ref('')
const selectedFolderId = ref('')
const favoritesOnly = ref(false)
const editingClue = ref<WorldClueDetail | null>(null)
const editorVisible = ref(false)
const initialLoading = ref(true)
const sourceEpoch = ref(0)
const searchEpoch = ref(0)
const detailEpoch = ref(0)
let initialEpoch = 0
const temporaryPlacementMap = new Map<string, WorldClueBoardPlacement>()
const camera = ref<BoardCamera>({ offsetX: 0, offsetY: 0, zoom: 1 })
const drawingRef = ref<InstanceType<typeof ClueBoardDrawingSurface> | null>(null)
const drawingEndpoints = ref<BoardDrawingEndpoint[]>([])
const selectedDrawing = computed(() => drawingEndpoints.value.find(item => item.selected))
const hoveredDrawingId = ref('')
const pendingDrawingSnapshot = shallowRef<Snapshot | null>(null)
const boardBody = ref<HTMLElement | null>(null)
const panGesture = ref<{ pointerId: number; x: number; y: number } | null>(null)
const drawingUnsaved = ref(false)
const drawingError = ref('')
const interactionLocked = ref(false)
const WHEEL_ZOOM_FACTOR = 1.05
const WHEEL_DELTA_PER_STEP = 100
let clearingDrawingSelection = false
let searchTimer: ReturnType<typeof setTimeout> | null = null
let focusListener: (() => void) | null = null
let sourceRequest: Promise<void> | null = null
let sourceRequestWorldId = ''
let layoutBackup: Record<string, { x: number; y: number; width?: number; pinned?: boolean }> | null = null
let lifecycleRequestId = ''
const editorRef = ref<InstanceType<typeof WorldClueEditorModal> | null>(null)
const interaction = useClueBoardInteraction()
const { selectedIds, selectedRelationId, inspectorVisible, selectedId, connection, selectNode: selectInteractionNode, selectRelation: selectInteractionRelation, clearSelection, beginConnection, setConnectionCursor, cancelConnection, closeInspector } = interaction

function handleBeforeUnload(event: BeforeUnloadEvent) {
  if (!session.value?.dirty && !drawingUnsaved.value) return
  event.preventDefault()
  event.returnValue = '线索板仍有未保存修改'
}

const session = computed(() => board.current)
const scope = ref<WorldClueBoardScope>('personal')
const switchingScope = ref(false)
const boardReadonly = computed(() => !session.value?.canWrite)
const scopePreferenceKey = () => `sealchat_clue_board_scope_v1:${String(user.info.id || '')}:${props.worldId}`

async function prepareSharedRealtime() {
  // InternalSurfaceView deliberately skips channel entry for personal boards.
  // The existing gateway sets its world context through channel.enter.
  const worldId = props.worldId
  const channelId = props.channelId
  const userId = String(user.info.id || '')
  try {
    await chat.ensureConnectionReady()
    if (props.worldId !== worldId || props.channelId !== channelId || String(user.info.id || '') !== userId) return false
    if (chat.curChannel?.id === channelId) return true
    return await chat.channelSwitchTo(channelId)
  } catch { return false }
}

async function switchScope(next: WorldClueBoardScope) {
  if (next === scope.value || switchingScope.value) return
  switchingScope.value = true
  const worldId = props.worldId
  const userId = String(user.info.id || '')
  try {
    if (!await flushBoard()) { message.warning('当前画板尚未同步，无法切换'); return }
    if (props.worldId !== worldId || String(user.info.id || '') !== userId) return
    if (next === 'shared' && !await prepareSharedRealtime()) { message.warning('无法建立协作连接'); return }
    if (props.worldId !== worldId || String(user.info.id || '') !== userId) return
    scope.value = next
    clearSelection()
    cancelConnection()
    temporaryPlacementMap.clear()
    layoutBackup = null
    await board.load(worldId, { scope: next, force: next === 'shared' })
    if (props.worldId !== worldId || String(user.info.id || '') !== userId || scope.value !== next) return
    await nextTick()
    drawingRef.value?.reloadSnapshot()
    try { localStorage.setItem(scopePreferenceKey(), next) } catch { /* Storage may be unavailable. */ }
  } finally { switchingScope.value = false }
}
const summaries = computed(() => worldClue.currentWorldId === props.worldId ? worldClue.summaries : [])
const allSummaryIds = computed(() => new Set(summaries.value.map(item => item.id)))
const folders = computed(() => worldClue.currentWorldId === props.worldId ? worldClue.folders : [])
const canManageWorld = computed(() => {
  const detail = chat.worldDetailMap[props.worldId]
  const role = detail?.memberRole
  return role === 'owner' || role === 'admin' || detail?.world?.ownerId === user.info.id || chat.worldMap[props.worldId]?.ownerId === user.info.id
})

const sourceSummaries = computed(() => summaries.value.filter(item => {
  if (selectedFolderId.value && item.sharedFolderId !== selectedFolderId.value && item.userState?.personalFolderId !== selectedFolderId.value) return false
  if (favoritesOnly.value && item.userState?.favorite !== true) return false
  return true
}))
const nodeLayouts = computed(() => buildNodeLayouts(
  sourceSummaries.value,
  session.value?.document.placements || {},
  temporaryPlacementMap,
  allSummaryIds.value,
))
const drawingSnapshot = computed<Snapshot | null>(() => {
  const current = session.value
  if (!current) return null
  // Shared canvas changes travel as diffs; only GET replaces its snapshot.
  const snapshot = current.scope === 'personal' && pendingDrawingSnapshot.value
    ? pendingDrawingSnapshot.value
    : current.scope === 'shared'
      ? (current.snapshotVersion, toRaw(current).document.quickdraw?.snapshot)
      : current.document.quickdraw?.snapshot
  return snapshot && typeof snapshot === 'object' ? snapshot as Snapshot : null
})
const drawingTheme = computed<'light' | 'dark'>(() => display.palette === 'night' ? 'dark' : 'light')
const drawingReadonly = computed(() => !session.value?.loaded || !!session.value?.loadFailed || props.resourceId !== 'main' || boardReadonly.value || switchingScope.value)
const drawingPreferenceKey = computed(() => `sealchat_clue_board_ui_v1:${String(user.info.id || '')}:${props.worldId}`)
const hiddenIds = computed(() => {
  if (!onlySearchResults.value || !matchingIds.value) return new Set<string>()
  return new Set(sourceSummaries.value.filter(item => !matchingIds.value?.has(item.id)).map(item => item.id))
})
const dimmedIds = computed(() => {
  if (onlySearchResults.value || !matchingIds.value) return new Set<string>()
  return new Set(sourceSummaries.value.filter(item => !matchingIds.value?.has(item.id)).map(item => item.id))
})
const relations = computed(() => session.value?.document.relations || [])
const selectedRelation = computed(() => {
  return relations.value.find(item => item.id === selectedRelationId.value) || null
})
const loadedCount = computed(() => summaries.value.length)
const hasDirty = computed(() => !!session.value?.dirty || drawingUnsaved.value)
const boardError = computed(() => session.value?.error || '')
const boardConflict = computed(() => session.value?.status === 'conflict')
const sharedSyncStatus = computed(() => {
  if (boardReadonly.value) return { label: '只读', icon: CloudLock, className: 'is-readonly' }
  if (boardError.value) return { label: '待重试', icon: AlertCircle, className: 'is-error' }
  const current = session.value
  if (current?.needsResync || current?.status === 'loading' || current?.status === 'saving' || current?.sharedSending || current?.sharedQueue.length || hasDirty.value) return { label: '同步中', icon: CloudUpload, className: 'is-syncing' }
  return { label: '已同步', icon: CircleCheck, className: 'is-synced' }
})
const clueOptions = computed(() => folders.value.map(folder => ({ label: `${folder.scope === 'personal' ? '我的' : '世界'}：${folder.name}`, value: folder.id })))
const folderDropdownOptions = computed(() => [
  { label: '全部文件夹', key: '' },
  ...clueOptions.value.map(option => ({ label: option.label, key: option.value })),
])
const selectedFolderLabel = computed(() => clueOptions.value.find(option => option.value === selectedFolderId.value)?.label || '全部文件夹')

function relationId(source: string, target: string, kind: string, label = '') {
  let hash = 2166136261
  for (const char of label) { hash ^= char.charCodeAt(0); hash = Math.imul(hash, 16777619) }
  const base = `manual:${kind}:${source}:${target}:${(hash >>> 0).toString(16)}`.slice(0, 150)
  const existing = new Set(relations.value.map(item => item.id))
  if (!existing.has(base)) return base
  let index = 2
  while (existing.has(`${base}:${index}`)) index += 1
  return `${base}:${index}`
}

async function refreshSource() {
  const worldId = props.worldId
  if (!worldId) return
  if (sourceRequest && sourceRequestWorldId === worldId) return sourceRequest
  const epoch = ++sourceEpoch.value
  let request: Promise<void>
  request = (async () => {
    try {
      await worldClue.loadWorld(worldId)
      if (epoch !== sourceEpoch.value || worldClue.currentWorldId !== worldId) return
    } catch (error: any) {
      if (epoch === sourceEpoch.value) searchError.value = error?.response?.data?.message || '线索列表加载失败'
    }
  })()
  sourceRequest = request
  sourceRequestWorldId = worldId
  try {
    await request
  } finally {
    if (sourceRequest === request) {
      sourceRequest = null
      sourceRequestWorldId = ''
    }
  }
}

async function refreshSourceForced() {
  // An editor save can complete after a previous focus/connected refresh has
  // already started; force a fresh list request so its summary is reflected
  // immediately instead of waiting for a websocket echo.
  const pending = sourceRequest
  if (pending) await pending.catch(() => undefined)
  sourceEpoch.value += 1
  sourceRequest = null
  sourceRequestWorldId = ''
  await refreshSource()
}

async function runSearch(value: string) {
  const keyword = value.trim()
  const epoch = ++searchEpoch.value
  searchError.value = ''
  if (!keyword) {
    matchingIds.value = null
    searchResults.value = []
    searchLoading.value = false
    return
  }
  searchLoading.value = true
  try {
    const response = await api.get(`api/v1/worlds/${encodeURIComponent(props.worldId)}/clues`, { params: { keyword } })
    if (epoch !== searchEpoch.value) return
    const items = (response.data?.items || []) as WorldClueSummary[]
    matchingIds.value = new Set(items.map(item => item.id))
    searchResults.value = items.slice(0, 10)
  } catch (error: any) {
    if (epoch === searchEpoch.value) {
      searchError.value = error?.response?.data?.message || '搜索失败'
      matchingIds.value = new Set()
      searchResults.value = []
    }
  } finally {
    if (epoch === searchEpoch.value) searchLoading.value = false
  }
}

function scheduleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => void runSearch(searchText.value), 240)
}

function selectFolder(key: string | number) {
  selectedFolderId.value = String(key)
}

function searchResultExcerpt(item: WorldClueSummary) {
  const content = item.contentText?.trim()
  if (content) return content
  if (item.kind === 'iframe') return item.embedDomain || '网页线索'
  if (item.kind === 'image') return '图片线索'
  return '文字线索'
}

async function focusSearchResult(item: WorldClueSummary) {
  searchFocused.value = false
  if (!nodeLayouts.value.some(node => node.id === item.id) && (selectedFolderId.value || favoritesOnly.value)) {
    selectedFolderId.value = ''
    favoritesOnly.value = false
    await nextTick()
  }
  selectNode({ id: item.id, additive: false })
  await nextTick()
  const node = nodeLayouts.value.find(candidate => candidate.id === item.id)
  if (node) drawingRef.value?.focusElement(node)
}

function updateBoard(mutator: (draft: NonNullable<typeof session.value>['document']) => void) {
  if (!session.value?.loaded || session.value.loadFailed || boardReadonly.value) return false
  return board.updateDocument((draft) => mutator(draft))
}

function applyPlacement(value: { id: string; placement: { x: number; y: number; width?: number; pinned?: boolean } }) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if (scope.value === 'shared') { board.putSharedPlacements({ [value.id]: { ...value.placement } }); return }
  updateBoard(document => { document.placements[value.id] = { ...value.placement } })
}

function togglePin(id: string) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const current = nodeLayouts.value.find(node => node.id === id)
  if (!current) return
  if (scope.value === 'shared') { board.putSharedPlacements({ [id]: { x: current.x, y: current.y, width: current.width, pinned: !current.pinned } }); return }
  updateBoard(document => { document.placements[id] = { x: current.x, y: current.y, width: current.width, pinned: !current.pinned } })
}

function clearDrawingSelection() {
  const drawing = drawingRef.value
  if (!drawing) return
  clearingDrawingSelection = true
  drawing.clearSelection()
  queueMicrotask(() => { clearingDrawingSelection = false })
}

function clearNodeAndRelationSelection() {
  cancelConnection()
  clearSelection()
}

function selectNode(value: { id: string; additive: boolean }) {
  clearDrawingSelection()
  cancelConnection()
  selectInteractionNode(value.id, value.additive)
}

function selectRelation(id: string) {
  clearDrawingSelection()
  selectInteractionRelation(id)
}

function onDrawingFocus() {
  if (clearingDrawingSelection) return
  clearNodeAndRelationSelection()
}

function onDrawingSelection() {
  if (clearingDrawingSelection) return
  clearNodeAndRelationSelection()
}

function onDrawingToolChange() {
  cancelConnection()
}

function onDrawingSnapshot(snapshot: Snapshot) {
  if (scope.value === 'shared') {
    if (board.syncSharedSnapshot(snapshot)) { drawingUnsaved.value = false; drawingError.value = '' }
    return
  }
  drawingUnsaved.value = true
  const accepted = updateBoard(document => {
    document.quickdraw = { engineVersion: QUICKDRAW_ENGINE_VERSION, snapshot }
  })
  // The Store rejects an over-limit document before replacing its Board
  // document. Quickdraw keeps the drawing in memory, while the visible Board
  // error and dirty flag make it explicit that it is not persisted yet.
  if (accepted) {
    pendingDrawingSnapshot.value = null
    drawingUnsaved.value = false
    drawingError.value = ''
  } else {
    // Keep a drawing-specific error because an older PUT may still complete
    // while this snapshot is intentionally kept only in Quickdraw memory; that
    // response must not make the current drawing look saved.
    drawingError.value = session.value?.errorKind === 'too-large'
      ? '绘图未保存：画板文档超过 8MiB，请删除部分内容后再保存'
      : '绘图未保存：画板文档尚未就绪或修改被拒绝'
    if (session.value?.errorKind === 'too-large') pendingDrawingSnapshot.value = snapshot
  }
}

function onDrawingLoaded() {
  pendingDrawingSnapshot.value = null
  drawingUnsaved.value = false
  drawingError.value = ''
}

function onDrawingPending(pending: boolean) {
  drawingUnsaved.value = pending
  board.setDrawingPending(pending)
}

function onDrawingDiff(diff: Diff) {
  if (!board.applySharedQuickdrawDiff(diff)) {
    drawingUnsaved.value = true
    drawingError.value = '绘图操作尚未提交，请重试或重新载入'
  }
}

function onBoardDrop(event: DragEvent) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const files = [...(event.dataTransfer?.files || [])].filter(file => file.type.startsWith('image/'))
  if (!files.length) return
  // A drop on a Vue clue node does not bubble through the sibling Quickdraw
  // surface. Forward only those image files; drops on the Quickdraw canvas are
  // stopped by its own public handler and are not duplicated.
  event.preventDefault()
  onDrawingFocus()
  drawingRef.value?.importImageFiles(files)
}

function onBoardDragOver(event: DragEvent) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if ([...(event.dataTransfer?.types || [])].includes('Files')) event.preventDefault()
}

function onBoardPointerMove(event: PointerEvent) {
  const pan = panGesture.value
  if (pan && pan.pointerId === event.pointerId) {
    event.preventDefault()
    event.stopPropagation()
    drawingRef.value?.pan(event.clientX - pan.x, event.clientY - pan.y)
    pan.x = event.clientX
    pan.y = event.clientY
    return
  }
  if (!connection.value) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  setConnectionCursor({ x: event.clientX - rect.left, y: event.clientY - rect.top })
  const id = drawingRef.value?.hitEndpoint(event.clientX, event.clientY) || ''
  hoveredDrawingId.value = connection.value.source.kind === 'quickdraw' && connection.value.source.id === id ? '' : id
}

function onBoardWheel(event: WheelEvent) {
  if (isBoardControl(event.target)) return
  if (!Number.isFinite(event.deltaY) || event.deltaY === 0) return
  const drawing = drawingRef.value
  if (!drawing) return
  const delta = event.deltaMode === 1
    ? event.deltaY * 16
    : event.deltaMode === 2
      ? event.deltaY * Math.max(window.innerHeight, 1)
      : event.deltaY
  const multiplier = Math.pow(WHEEL_ZOOM_FACTOR, -delta / WHEEL_DELTA_PER_STEP)
  if (!Number.isFinite(multiplier) || multiplier <= 0) return
  event.preventDefault()
  event.stopPropagation()
  drawing.zoomAt(event.clientX, event.clientY, multiplier)
}

function isBoardControl(target: EventTarget | null) {
  return target instanceof Element && !!target.closest('input, textarea, select, button, [contenteditable], [role="menu"], [role="listbox"], [role="combobox"], .clue-board-inspector, .clue-board-drawing-toolbar, .clue-board-drawing-inspector, .clue-board-relation-quickbar, .qd-text-edit')
}

function onBoardPointerDown(event: PointerEvent) {
  if (isBoardControl(event.target)) return
  if (event.button === 2) {
    cancelConnection()
    hoveredDrawingId.value = ''
    event.preventDefault()
    event.stopPropagation()
    panGesture.value = { pointerId: event.pointerId, x: event.clientX, y: event.clientY }
    boardBody.value?.setPointerCapture(event.pointerId)
    return
  }
  if (event.button !== 0 || !connection.value) return
  // Clue nodes own their click handler; capture drawing targets before the engine clears selection.
  if (event.target instanceof Element && event.target.closest('[data-clue-id]')) return
  event.preventDefault()
  event.stopPropagation()
  const id = drawingRef.value?.hitEndpoint(event.clientX, event.clientY)
  if (id) finishEndpoint({ kind: 'quickdraw', id })
  else cancelConnection()
}

function onBoardPointerUp(event: PointerEvent) {
  if (panGesture.value?.pointerId !== event.pointerId) return
  event.preventDefault()
  event.stopPropagation()
  panGesture.value = null
  if (boardBody.value?.hasPointerCapture(event.pointerId)) boardBody.value.releasePointerCapture(event.pointerId)
}

function onBoardContextMenu(event: MouseEvent) {
  if (!isBoardControl(event.target)) event.preventDefault()
}

function drawingOverlayStyle(item: BoardDrawingEndpoint) {
  return { left: `${item.x * camera.value.zoom + camera.value.offsetX}px`, top: `${item.y * camera.value.zoom + camera.value.offsetY}px`, width: `${item.width * camera.value.zoom}px`, height: `${item.height * camera.value.zoom}px` }
}

function openNode(id: string) {
  if (!summaries.value.some(item => item.id === id)) return
  chatEvent.emit('world-clue-open' as any, { worldId: props.worldId, clueId: id })
}

async function editNode(id: string) {
  const worldId = props.worldId
  const userId = String(user.info.id || '')
  const epoch = ++detailEpoch.value
  try {
    const clue = await worldClue.fetchDetailReadonly(worldId, id)
    if (epoch !== detailEpoch.value || props.worldId !== worldId || String(user.info.id || '') !== userId) return
    if (clue.effectiveAccess !== 'edit') {
      message.warning('没有编辑此线索的权限')
      return
    }
    editingClue.value = clue
    editorVisible.value = true
  } catch {
    message.warning('线索不可用或没有编辑权限')
  }
}

function addRelation(value: { sourceRef: WorldClueBoardRelationEndpointRef; targetRef: WorldClueBoardRelationEndpointRef; kind: WorldClueBoardRelationKind; label?: string }) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if (value.sourceRef.kind === value.targetRef.kind && value.sourceRef.id === value.targetRef.id) return
  const relation: WorldClueBoardRelation = {
    id: relationId(value.sourceRef.id, value.targetRef.id, value.kind, value.label || ''),
    sourceRef: value.sourceRef,
    targetRef: value.targetRef,
    kind: value.kind,
    ...(value.label ? { label: value.label } : {}),
  }
  if (scope.value === 'shared') board.putSharedRelation(relation)
  else updateBoard(document => { document.relations = [...document.relations, relation] })
  selectInteractionRelation(relation.id)
}

function startRelation(value: { id: string; kind: WorldClueBoardRelationKind }) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  clearDrawingSelection()
  drawingRef.value?.setTool('select')
  beginConnection({ kind: 'clue', id: value.id }, value.kind)
  const node = nodeLayouts.value.find(item => item.id === value.id)
  if (node) setConnectionCursor({ x: (node.x + node.width / 2) * camera.value.zoom + camera.value.offsetX, y: (node.y + node.height + 10) * camera.value.zoom + camera.value.offsetY })
}

function startDrawingRelation(kind: WorldClueBoardRelationKind) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const node = selectedDrawing.value
  if (!node) return
  drawingRef.value?.setTool('select')
  clearSelection()
  beginConnection({ kind: 'quickdraw', id: node.id }, kind)
  setConnectionCursor({ x: (node.x + node.width / 2) * camera.value.zoom + camera.value.offsetX, y: (node.y + node.height) * camera.value.zoom + camera.value.offsetY })
}

function finishRelation(targetId: string) {
  finishEndpoint({ kind: 'clue', id: targetId })
}

function finishEndpoint(target: WorldClueBoardRelationEndpointRef) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const active = connection.value
  if (!active || (active.source.kind === target.kind && active.source.id === target.id)) return
  addRelation({ sourceRef: active.source, targetRef: target, kind: active.kind })
  cancelConnection()
  hoveredDrawingId.value = ''
  clearDrawingSelection()
}

function reverseRelation(id: string) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if (scope.value === 'shared') {
    const relation = relations.value.find(item => item.id === id)
    if (relation && relation.kind !== 'related' && relation.kind !== 'contradicts') board.putSharedRelation({ ...relation, sourceRef: relation.targetRef, targetRef: relation.sourceRef })
    return
  }
  updateBoard(document => {
    const relation = document.relations.find(item => item.id === id)
    if (!relation || relation.kind === 'related' || relation.kind === 'contradicts') return
    ;[relation.sourceRef, relation.targetRef] = [relation.targetRef, relation.sourceRef]
  })
}

function updateRelationLabel(id: string, label: string) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const relation = relations.value.find(item => item.id === id)
  if (!relation) return
  const nextLabel = label.trim()
  const currentLabel = relation.label?.trim() ?? ''
  if (currentLabel === nextLabel) return
  if (scope.value === 'shared') {
    const next = { ...relation }
    if (nextLabel) next.label = nextLabel
    else delete next.label
    board.putSharedRelation(next)
    return
  }
  updateBoard(document => {
    const nextRelation = document.relations.find(item => item.id === id)
    if (!nextRelation) return
    if (nextLabel) nextRelation.label = nextLabel
    else delete nextRelation.label
  })
}

function updateRelationKind(id: string, kind: WorldClueBoardRelationKind) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const relation = relations.value.find(item => item.id === id)
  if (!relation || relation.kind === kind) return
  if (scope.value === 'shared') { board.putSharedRelation({ ...relation, kind }); return }
  updateBoard(document => {
    const nextRelation = document.relations.find(item => item.id === id)
    if (nextRelation) nextRelation.kind = kind
  })
}

function updateRelation(value: { id: string; kind?: WorldClueBoardRelationKind; label?: string }) {
  if (value.kind !== undefined) updateRelationKind(value.id, value.kind)
  if (value.label !== undefined) updateRelationLabel(value.id, value.label)
}

function removeRelation(id: string) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if (scope.value === 'shared') board.removeSharedRelation(id)
  else updateBoard(document => { document.relations = document.relations.filter(item => item.id !== id) })
  closeInspector()
}

function arrangeCurrent() {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  const targets = new Set(nodeLayouts.value.filter(node => !hiddenIds.value.has(node.id)).map(node => node.id))
  arrangeFor(targets)
}

function arrangeSelected() {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  arrangeFor(new Set([...selectedIds.value].filter(id => nodeLayouts.value.some(node => node.id === id) && !hiddenIds.value.has(id))))
}

function arrangeFor(targetIds: Set<string>) {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if (!targetIds.size) return
  if (!layoutBackup) layoutBackup = scope.value === 'shared'
    ? Object.fromEntries(nodeLayouts.value.map(node => [node.id, { x: node.x, y: node.y, width: node.width, pinned: node.pinned }]))
    : { ...(session.value?.document.placements || {}) }
  const visibleNodes = nodeLayouts.value.filter(node => !hiddenIds.value.has(node.id))
  const positions = arrangeLayouts(visibleNodes, targetIds, relations.value)
  if (scope.value === 'shared') { if (positions.size) board.putSharedPlacements(Object.fromEntries(positions)); return }
  updateBoard(document => {
    for (const [id, placement] of positions) document.placements[id] = placement
  })
}

function restoreLayout() {
  if (interactionLocked.value || boardReadonly.value || switchingScope.value) return
  if (!layoutBackup) return
  const backup = layoutBackup
  layoutBackup = null
  if (scope.value === 'shared') { board.putSharedPlacements(backup); return }
  updateBoard(document => { document.placements = { ...backup } })
}

async function retryLoad() {
  if (session.value?.loaded) await board.retry()
  else await board.load(props.worldId, { force: true, scope: scope.value })
}

async function discardAndReload() {
  const key = session.value?.key
  if (scope.value === 'shared' && drawingRef.value && !await drawingRef.value.flush()) return
  if (session.value?.key !== key) return
  await board.discardLocalAndReload()
  if (session.value?.key !== key) return
  drawingRef.value?.reloadSnapshot()
}

async function flushBoard() {
  const key = session.value?.key
  const drawing = drawingRef.value
  if (drawing) {
    const finalized = await drawing.flush()
    if (!finalized || drawingUnsaved.value) return false
  }
  if (session.value?.key !== key) return false
  return board.flush()
}

function postLifecycle(payload: Record<string, unknown>) {
  if (typeof window === 'undefined' || window.parent === window) return
  window.parent.postMessage({ type: 'sealchat.world-clue-board.lifecycle', ...payload }, window.location.origin)
}

async function handleLifecycle(event: MessageEvent) {
  if (typeof window === 'undefined' || window.parent === window) return
  if (event.origin !== window.location.origin || event.source !== window.parent) return
  if (event.data?.type !== 'sealchat.world-clue-board.lifecycle') return
  if (event.data.state === 'fit-content') {
    drawingRef.value?.fitBoard(nodeLayouts.value.filter(node => !hiddenIds.value.has(node.id)))
    return
  }
  const requestId = typeof event.data.requestId === 'string' ? event.data.requestId : ''
  if (!requestId) return
  if (event.data.state === 'request-close') {
    lifecycleRequestId = requestId
    try {
      const drawing = drawingRef.value
      if (drawing) {
        const finalized = await drawing.flush()
        if (!finalized || drawingUnsaved.value) {
          window.parent.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'close-result', requestId, accepted: false }, window.location.origin)
          return
        }
      }
      if (editorVisible.value) {
        const editor = editorRef.value
        if (!editor) {
          window.parent.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'close-result', requestId, accepted: false }, window.location.origin)
          return
        }
        await editor.requestClose()
        // A failed validation or a pending media upload
        // leaves the modal visible. Never destroy the iframe around it.
        if (editorVisible.value) {
          window.parent.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'close-result', requestId, accepted: false }, window.location.origin)
          return
        }
      }
      const accepted = await flushBoard()
      if (lifecycleRequestId !== requestId) return
      window.parent.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'close-result', requestId, accepted }, window.location.origin)
    } catch {
      if (lifecycleRequestId !== requestId) return
      window.parent.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'close-result', requestId, accepted: false }, window.location.origin)
    }
  }
}

function onEditorSaved(saved?: WorldClueDetail) {
  if (saved?.worldId && saved.worldId !== props.worldId) return
  void refreshSourceForced()
}

function toggleInteractionLock() {
  interactionLocked.value = !interactionLocked.value
  if (!interactionLocked.value) return
  cancelConnection()
  hoveredDrawingId.value = ''
  closeInspector()
}

async function initialLoad() {
  const epoch = ++initialEpoch
  if (props.resourceId !== 'main') {
    emit('unavailable', '画板仅支持 resourceId=main')
    return
  }
  if (!props.worldId || !props.channelId) {
    emit('unavailable', '画板缺少 worldId 或 channelId')
    return
  }
  initialLoading.value = true
  try { scope.value = localStorage.getItem(scopePreferenceKey()) === 'shared' ? 'shared' : 'personal' } catch { scope.value = 'personal' }
  if (scope.value === 'shared' && !await prepareSharedRealtime()) {
    if (epoch !== initialEpoch) return
    scope.value = 'personal'
    message.warning('无法建立协作连接，已打开个人画板')
  }
  if (epoch !== initialEpoch) return
  const [, loaded] = await Promise.all([refreshSource(), board.load(props.worldId, { scope: scope.value, force: scope.value === 'shared', surfaceInit: scope.value === 'personal' })])
  if (epoch !== initialEpoch) return
  if (loaded.status === 'error' && !loaded.loaded) {
    // Keep the surface visible so the user can explicitly retry; importantly,
    // no empty document is marked dirty or sent back to the server.
    initialLoading.value = false
    emit('ready')
    postLifecycle({ state: 'ready', requestId: '' })
    return
  }
  initialLoading.value = false
  emit('ready')
  // The embed may use this handshake to allow a controlled close. Publish it
  // only after the first source/document load has settled, so a close cannot
  // destroy an iframe while its Board GET is still in flight.
  postLifecycle({ state: 'ready', requestId: '' })
}

watch(searchText, value => {
  if (!value.trim()) {
    searchEpoch.value += 1
    matchingIds.value = null
    searchResults.value = []
    searchLoading.value = false
    searchError.value = ''
  }
  scheduleSearch()
})
watch(selectedRelation, relation => {
  if (selectedRelationId.value && !relation) {
    closeInspector()
  }
})
watch(() => [props.worldId, user.info.id] as const, () => {
  sourceEpoch.value += 1
  searchEpoch.value += 1
  detailEpoch.value += 1
  clearSelection()
  cancelConnection()
  drawingEndpoints.value = []
  matchingIds.value = null
  searchResults.value = []
  searchLoading.value = false
  searchError.value = ''
  selectedFolderId.value = ''
  favoritesOnly.value = false
  interactionLocked.value = false
  temporaryPlacementMap.clear()
  layoutBackup = null
  void initialLoad()
})

onMounted(() => {
  board.startRealtime()
  void initialLoad()
  const handleConnected = () => { void refreshSource(); if (!drawingUnsaved.value) void board.refreshOnFocus() }
  const handleFocus = () => { void refreshSource(); if (!drawingUnsaved.value) void board.refreshOnFocus() }
  chatEvent.on('connected', handleConnected)
  const handleBoardChanged = (event: { worldClueBoard?: WorldClueBoardEventPayload }) => {
    if (!event.worldClueBoard) return
    const operations = board.handleBoardChanged(event.worldClueBoard)
    for (const operation of operations) {
      if (operation.type === 'quickdraw.diff' && !drawingRef.value?.applyRemoteDiff(operation.quickdrawDiff)) {
        board.requestSharedRefresh()
        break
      }
    }
  }
  const handleClueChanged = async (event: { worldClue?: { worldId: string } }) => {
    if (event.worldClue?.worldId !== props.worldId) return
    const key = session.value?.key
    await refreshSourceForced()
    if (session.value?.key === key) board.requestSharedRefresh()
  }
  chatEvent.on('world-clue-board-changed' as any, handleBoardChanged)
  chatEvent.on('world-clue-changed' as any, handleClueChanged)
  const handleChannelEntered = () => {
    if (chat.currentWorldId === props.worldId) board.requestSharedRefresh()
  }
  chatEvent.on('channel-switch-to', handleChannelEntered)
  window.addEventListener('focus', handleFocus)
  window.addEventListener('message', handleLifecycle)
  window.addEventListener('beforeunload', handleBeforeUnload)
  focusListener = () => {
    chatEvent.off('connected', handleConnected)
    chatEvent.off('world-clue-board-changed' as any, handleBoardChanged)
    chatEvent.off('world-clue-changed' as any, handleClueChanged)
    chatEvent.off('channel-switch-to', handleChannelEntered)
    board.stopRealtime()
    window.removeEventListener('focus', handleFocus)
  }
})

onBeforeUnmount(() => {
  initialEpoch += 1
  if (searchTimer) clearTimeout(searchTimer)
  sourceEpoch.value += 1
  searchEpoch.value += 1
  detailEpoch.value += 1
  focusListener?.()
  window.removeEventListener('message', handleLifecycle)
  window.removeEventListener('beforeunload', handleBeforeUnload)
  void flushBoard()
})
</script>

<template>
  <section class="world-clue-board-app">
    <header class="world-clue-board-app__toolbar">
      <div class="world-clue-board-app__title">
        <strong>线索板</strong><span>已载入 {{ loadedCount }} 条</span>
        <NButtonGroup><NButton size="small" :type="scope === 'personal' ? 'primary' : 'default'" :disabled="switchingScope" @click="switchScope('personal')">个人</NButton><NButton size="small" :type="scope === 'shared' ? 'primary' : 'default'" :disabled="switchingScope" @click="switchScope('shared')">协作</NButton></NButtonGroup>
        <NTooltip v-if="scope === 'shared'">
          <template #trigger>
            <span class="world-clue-board-app__sync-status" :class="sharedSyncStatus.className" :aria-label="sharedSyncStatus.label" role="status">
              <NIcon :size="17"><component :is="sharedSyncStatus.icon" /></NIcon>
            </span>
          </template>
          {{ sharedSyncStatus.label }}
        </NTooltip>
        <NTag v-if="scope === 'personal' && hasDirty" type="warning" size="small">
          未保存
        </NTag>
        <NDropdown trigger="click" :options="folderDropdownOptions" :value="selectedFolderId" @select="selectFolder">
          <NTooltip><template #trigger><button type="button" class="world-clue-board-app__filter-button" :class="{ 'is-active': selectedFolderId }" :aria-label="selectedFolderLabel"><NIcon :size="18"><Folder /></NIcon></button></template>{{ selectedFolderLabel }}</NTooltip>
        </NDropdown>
        <NTooltip><template #trigger><button type="button" class="world-clue-board-app__filter-button" :class="{ 'is-active': favoritesOnly }" :aria-pressed="favoritesOnly" aria-label="只看收藏" @click="favoritesOnly = !favoritesOnly"><NIcon :size="18"><Star /></NIcon></button></template>{{ favoritesOnly ? '只看收藏：已开启' : '只看收藏' }}</NTooltip>
      </div>
      <div class="world-clue-board-app__toolbar-actions">
        <div class="world-clue-board-app__search">
          <NInput v-model:value="searchText" size="small" clearable placeholder="搜索线索" :loading="searchLoading" @focus="searchFocused = true" @blur="searchFocused = false"><template #prefix><Search /></template></NInput>
          <div v-if="searchFocused && searchText.trim() && searchResults.length" class="world-clue-board-app__search-results" role="listbox" aria-label="搜索结果">
            <button v-for="item in searchResults" :key="item.id" type="button" role="option" @pointerdown.prevent @click="focusSearchResult(item)">
              <strong>{{ item.title || '未命名线索' }}</strong>
              <span>{{ searchResultExcerpt(item) }}</span>
            </button>
          </div>
        </div>
        <NCheckbox v-model:checked="onlySearchResults">仅显示结果</NCheckbox>
        <NButtonGroup>
          <NButton size="small" :disabled="interactionLocked || boardReadonly || switchingScope" @click="arrangeCurrent"><template #icon><LayoutBoard /></template>整理当前视图</NButton>
          <NButton size="small" :disabled="interactionLocked || boardReadonly || switchingScope || !selectedIds.size" @click="arrangeSelected">整理所选</NButton>
          <NButton size="small" :disabled="interactionLocked || boardReadonly || switchingScope || !layoutBackup" @click="restoreLayout"><template #icon><Rotate2 /></template>恢复</NButton>
        </NButtonGroup>
      </div>
    </header>
    <NAlert v-if="searchError" type="warning" closable @close="searchError = ''">{{ searchError }}</NAlert>
    <NAlert v-if="drawingError" type="error" :show-icon="false">{{ drawingError }}</NAlert>
    <NAlert v-if="boardError" type="error" :show-icon="false">
      {{ boardError }}
	      <template #action><NSpace size="small"><NButton v-if="boardConflict || scope === 'shared'" size="small" :disabled="interactionLocked || switchingScope" @click="discardAndReload">放弃本地修改并重载</NButton><NButton v-if="session?.canRetry" size="small" @click="retryLoad"><Refresh /> 重试</NButton></NSpace></template>
    </NAlert>
    <main ref="boardBody" class="world-clue-board-app__body" :class="{ 'is-panning': panGesture }" @dragover="onBoardDragOver" @drop="onBoardDrop" @wheel.capture="onBoardWheel" @pointerdown.capture="onBoardPointerDown" @pointermove.capture="onBoardPointerMove" @pointerup.capture="onBoardPointerUp" @pointercancel.capture="onBoardPointerUp" @lostpointercapture="panGesture = null" @contextmenu.capture="onBoardContextMenu">
      <div v-if="initialLoading" class="world-clue-board-app__loading">正在载入线索板…</div>
      <template v-else>
        <ClueBoardDrawingSurface
          ref="drawingRef"
          :snapshot="drawingSnapshot"
          :theme="drawingTheme"
          :preference-key="drawingPreferenceKey"
          :readonly="drawingReadonly"
          :realtime-diff-enabled="scope === 'shared' && !boardReadonly"
          @user-diff="onDrawingDiff"
          :interaction-locked="interactionLocked || boardReadonly || switchingScope"
          @camera-change="camera = $event"
          @snapshot-change="onDrawingSnapshot"
          @snapshot-loaded="onDrawingLoaded"
          @drawing-pending="onDrawingPending"
          @focus="onDrawingFocus"
          @selection-change="onDrawingSelection"
          @endpoints-change="drawingEndpoints = $event"
          @tool-change="onDrawingToolChange"
          @toggle-lock="toggleInteractionLock"
        />
        <ClueBoardCanvas
          :nodes="nodeLayouts"
          :relations="relations"
          :drawing-endpoints="drawingEndpoints"
          :camera="camera"
          :selected-id="selectedId"
          :selected-ids="selectedIds"
          :hidden-ids="hiddenIds"
          :dimmed-ids="dimmedIds"
          :connection="connection"
          :interaction-locked="interactionLocked || boardReadonly || switchingScope"
          @select="selectNode"
          @select-relation="selectRelation"
          @start-connection="startRelation"
          @connect-target="finishRelation"
          @connection-cursor="setConnectionCursor"
          @update-relation-label="updateRelationLabel($event.id, $event.label)"
          @open="openNode"
          @edit="editNode"
          @toggle-pin="togglePin"
          @placement-change="applyPlacement"
        />
        <template v-for="item in drawingEndpoints" :key="item.id">
          <div v-if="connection && hoveredDrawingId === item.id" class="world-clue-board-app__drawing-target" :style="drawingOverlayStyle(item)" />
        </template>
        <ClueBoardRelationQuickBar v-if="!interactionLocked && !boardReadonly && selectedDrawing && !connection && !selectedIds.size && !inspectorVisible" class="world-clue-board-app__drawing-launcher" :style="{ left: `${(selectedDrawing.x + selectedDrawing.width / 2) * camera.zoom + camera.offsetX}px`, top: `${(selectedDrawing.y + selectedDrawing.height) * camera.zoom + camera.offsetY + 10}px` }" @start="startDrawingRelation" />
      </template>
      <ClueBoardRelationInspector
        v-if="!interactionLocked && inspectorVisible && selectedRelation"
        :relation="selectedRelation"
        :readonly="boardReadonly || switchingScope"
        :summaries="summaries"
        :drawing-endpoints="drawingEndpoints"
        @update="updateRelation"
        @remove="removeRelation"
        @reverse="reverseRelation"
        @close="closeInspector"
      />
    </main>
    <WorldCluePresentationHost :world-id="worldId" />
    <WorldClueEditorModal ref="editorRef" v-model:show="editorVisible" :world-id="worldId" :channel-id="channelId" :clue="editingClue" :can-manage="canManageWorld" @saved="onEditorSaved" />
  </section>
</template>

<style scoped>
.world-clue-board-app { position: relative; display: flex; width: 100%; height: 100%; min-height: 0; flex-direction: column; color: var(--sc-text-primary); background: var(--sc-bg-surface); }
.world-clue-board-app__toolbar { position: relative; z-index: 70; display: flex; min-height: 54px; align-items: center; gap: 12px; padding: 8px 14px; border-bottom: 1px solid var(--sc-border-mute); background: color-mix(in srgb, var(--sc-bg-elevated) 94%, transparent); }
.world-clue-board-app__title { display: flex; min-width: max-content; align-items: center; gap: 8px; }
.world-clue-board-app__title span { color: var(--sc-text-secondary); font-size: 12px; }
.world-clue-board-app__sync-status { display: inline-grid; width: 24px; height: 28px; flex: 0 0 24px; place-items: center; color: var(--sc-text-secondary); }
.world-clue-board-app__sync-status.is-readonly { color: var(--sc-text-secondary); }
.world-clue-board-app__sync-status.is-error { color: var(--error-color, #d03050); }
.world-clue-board-app__sync-status.is-syncing { color: var(--warning-color, #f0a020); }
.world-clue-board-app__sync-status.is-synced { color: var(--success-color, #18a058); }
.world-clue-board-app__toolbar-actions { display: flex; min-width: 0; flex: 1; align-items: center; justify-content: flex-end; gap: 7px; flex-wrap: wrap; }
.world-clue-board-app__filter-button { display: grid; width: 30px; height: 30px; padding: 0; place-items: center; color: var(--sc-text-secondary); border: 1px solid transparent; border-radius: 7px; background: transparent; cursor: pointer; }
.world-clue-board-app__filter-button:hover, .world-clue-board-app__filter-button.is-active { color: var(--primary-color, #3388de); border-color: color-mix(in srgb, var(--primary-color, #3388de) 45%, transparent); background: color-mix(in srgb, var(--primary-color, #3388de) 14%, transparent); }
.world-clue-board-app__search { position: relative; width: 210px; }
.world-clue-board-app__search-results { position: absolute; top: calc(100% + 5px); right: 0; width: min(360px, calc(100vw - 28px)); max-height: 360px; padding: 5px; overflow-y: auto; border: 1px solid var(--sc-border-mute); border-radius: 9px; background: var(--sc-bg-elevated); box-shadow: 0 10px 28px #0004; }
.world-clue-board-app__search-results button { display: flex; width: 100%; flex-direction: column; gap: 2px; padding: 7px 9px; text-align: left; color: var(--sc-text-primary); border: 0; border-radius: 6px; background: transparent; cursor: pointer; }
.world-clue-board-app__search-results button:hover, .world-clue-board-app__search-results button:focus-visible { background: color-mix(in srgb, var(--primary-color, #3388de) 12%, transparent); outline: none; }
.world-clue-board-app__search-results strong { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.world-clue-board-app__search-results span { display: -webkit-box; overflow: hidden; color: var(--sc-text-secondary); font-size: 11px; line-height: 1.35; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.world-clue-board-app__hint { color: var(--sc-text-secondary); font-size: 11px; }
.world-clue-board-app :deep(.n-alert) { margin: 8px 14px 0; }
.world-clue-board-app__body { position: relative; min-height: 0; flex: 1; }
.world-clue-board-app__body.is-panning { cursor: grabbing; }
.world-clue-board-app__drawing-target { position: absolute; z-index: 3; outline: 3px solid var(--primary-color, #3388de); pointer-events: none; }
.world-clue-board-app__drawing-launcher { position: absolute; z-index: 4; transform: translateX(-50%); }
.world-clue-board-app__body > .clue-board-canvas { position: absolute; inset: 0; }
.world-clue-board-app__body > .clue-board-inspector { position: absolute; z-index: 5; top: 12px; right: 12px; }
.world-clue-board-app__loading { display: grid; width: 100%; height: 100%; place-items: center; color: var(--sc-text-secondary); }
@media (max-width: 760px) { .world-clue-board-app__toolbar { align-items: flex-start; flex-direction: column; } .world-clue-board-app__title { min-width: 0; flex-wrap: wrap; } .world-clue-board-app__toolbar-actions { width: 100%; justify-content: flex-start; } .world-clue-board-app__hint { flex-basis: 100%; } }
</style>
