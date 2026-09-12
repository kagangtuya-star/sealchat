<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NIcon, NTooltip } from 'naive-ui'
import {
  Adjustments, ArrowUpRight, Eraser, GripVertical, HandStop, Highlight, Line,
  Flare, Lock, LockOpen, Note, Photo, Pencil, Select, Shape, Typography,
} from '@vicons/tabler'
import type { BoardDrawingEndpoint, BoardElementGeometry } from './boardTypes'
import ClueBoardDrawingInspector from './ClueBoardDrawingInspector.vue'
import '@quickdrawjs/core/quickdraw.css'
import {
  COLOR_IDS,
  composeDiff,
  isDiffEmpty,
  type Diff,
  pageBounds,
  DASH_IDS,
  FILL_IDS,
  GEO_IDS,
  SIZE_IDS,
  type ColorId,
  type DashId,
  type FillId,
  type FontId,
  type GeoId,
  type GridId,
  type ShapeRecord,
  type ShapeType,
  type Snapshot,
  type SizeId,
  type ToolId,
} from '@quickdrawjs/core'
import { QUICKDRAW_ENGINE_VERSION, QuickdrawAdapter } from './quickdraw-adapter'
import type { SealChatCamera } from './quickdraw-camera-adapter'

const props = withDefaults(defineProps<{
  snapshot?: Snapshot | null
  theme: 'light' | 'dark'
  grid?: GridId
  preferenceKey: string
  readonly?: boolean
  realtimeDiffEnabled?: boolean
  interactionLocked?: boolean
}>(), {
  snapshot: null,
  grid: 'dots',
  readonly: false,
  realtimeDiffEnabled: false,
  interactionLocked: false,
})

const emit = defineEmits<{
  'user-diff': [diff: Diff]
  'snapshot-change': [snapshot: Snapshot]
  'snapshot-loaded': []
  'drawing-pending': [pending: boolean]
  'camera-change': [camera: SealChatCamera]
  'selection-change': [selected: boolean]
  'endpoints-change': [endpoints: BoardDrawingEndpoint[]]
  focus: []
  'tool-change': [tool: ToolId]
  'toggle-lock': []
}>()

interface DrawingPreferences {
  tool?: ToolId
  color?: ColorId
  size?: SizeId
  dash?: DashId
  fill?: FillId
  font?: FontId
  geoKind?: GeoId
  grid?: GridId
  penMode?: boolean
  dockPosition?: { x: number; y: number }
  inspectorPosition?: { x: number; y: number }
  inspectorOpen?: boolean
  inspectorCollapsed?: boolean
  // Compatibility with the short-lived parameter panel preference shape.
  parametersPosition?: { x: number; y: number }
  parametersCollapsed?: boolean
}

const TOOL_OPTIONS: Array<{ id: ToolId; label: string; title: string }> = [
  { id: 'hand', label: '手型', title: '平移画布（H）' },
  { id: 'select', label: '选择', title: '选择绘图对象（V）' },
  { id: 'draw', label: '画笔', title: '自由绘制（D）' },
  { id: 'highlight', label: '荧光笔', title: '荧光笔（I）' },
  { id: 'eraser', label: '橡皮', title: '擦除绘图对象（E）' },
  { id: 'line', label: '直线', title: '绘制直线（L）' },
  { id: 'arrow', label: '普通箭头', title: '普通箭头（A）' },
  { id: 'geo', label: '图形', title: '绘制图形（G）' },
  { id: 'text', label: '文本', title: '添加文本（T）' },
  { id: 'note', label: '便签', title: '添加便签（N）' },
  { id: 'laser', label: '激光笔', title: '临时激光笔' },
]
const KEYBOARD_ZOOM_FACTOR = 1.05
const toolIcons = { select: Select, hand: HandStop, draw: Pencil, highlight: Highlight, eraser: Eraser, line: Line, geo: Shape, arrow: ArrowUpRight, text: Typography, note: Note, laser: Flare }
const inspectorOpen = ref(true)
const inspectorCollapsed = ref(false)
const inspectorPosition = ref({ x: 20, y: 20 })
const inspectorPanel = ref<HTMLElement | null>(null)
const dockPosition = ref({ x: 20, y: 20 })
const dock = ref<HTMLElement | null>(null)
const inspectorPanelStyle = computed(() => ({ left: `${inspectorPosition.value.x}px`, top: `${inspectorPosition.value.y}px` }))
const dockStyle = computed(() => ({ left: `${dockPosition.value.x}px`, top: `${dockPosition.value.y}px` }))
const containerHeight = ref(560)
const inspectorMaxHeight = computed(() => Math.max(120, Math.min(520, containerHeight.value - 16)))

// The installed declaration exposes these three GridId values. Runtime-only
// palette experiments such as `iso` are deliberately not surfaced here.
const SUPPORTED_GRID_IDS: GridId[] = ['none', 'lines', 'dots']

const container = ref<HTMLElement | null>(null)
const activeTool = ref<ToolId>('select')
const color = ref<ColorId>('black')
const size = ref<SizeId>('m')
const dash = ref<DashId>('draw')
const fill = ref<FillId>('none')
const font = ref<FontId>('draw')
const geoKind = ref<GeoId>('rectangle')
const grid = ref<GridId>(props.grid)
const canUndo = ref(false)
const canRedo = ref(false)
const hasSelection = ref(false)
const selectedKinds = ref<ShapeType[]>([])
const selectedAlign = ref<'start' | 'middle' | 'end'>('start')
const selectedBend = ref(0)
const penMode = ref(false)
const textEditing = ref(false)

const adapter = ref<QuickdrawAdapter | null>(null)
const preferenceKey = computed(() => props.preferenceKey.trim())
const activePointers = new Set<number>()
const activePointerTypes = new Map<number, string>()
// Quickdraw's image helpers decode files asynchronously. Keep the public
// import promises visible to the host so a close request cannot race the
// Store transaction that finally adds the asset and image shape.
const pendingImports = new Set<Promise<void>>()
const pendingOperations = new Set<Promise<void>>()
const pickerInputs = new Map<HTMLInputElement, () => void>()
let unsubs: Array<() => void> = []
let finalizeQueued = false
let pendingUserChange = false
let pendingRealtimeDiff: Diff | null = null
let realtimeDiffTimer: ReturnType<typeof setTimeout> | null = null

function flushRealtimeDiff() {
  if (realtimeDiffTimer) clearTimeout(realtimeDiffTimer)
  realtimeDiffTimer = null
  const diff = pendingRealtimeDiff
  pendingRealtimeDiff = null
  if (diff && !isDiffEmpty(diff)) emit('user-diff', diff)
}

function applyRemoteDiff(diff: Diff) {
  if (!adapter.value || pendingUserChange || pendingRealtimeDiff || pendingImports.size || pendingOperations.size) return false
  adapter.value.applyDiff(diff)
  lastCommittedSnapshot = snapshotJSON(adapter.value.getSnapshot())
  lastAppliedSnapshot = lastCommittedSnapshot
  publishEndpoints()
  syncSelectionDetails()
  syncHistory()
  adapter.value.editor.requestRender()
  return true
}
let destroyed = false
let lastAppliedSnapshot = ''
let lastEmittedSnapshot = ''
let lastCommittedSnapshot = ''
let floatingDrag: { target: 'dock' | 'inspector'; pointerId: number; startX: number; startY: number; x: number; y: number } | null = null
let resizeObserver: ResizeObserver | null = null
let hasSavedDockPosition = false
let hasSavedInspectorPosition = false

function emptySnapshot(): Snapshot {
  return { document: { store: {} } }
}

function snapshotJSON(snapshot: Snapshot): string {
  try {
    return JSON.stringify(snapshot)
  } catch {
    return ''
  }
}

function readPreferences(): DrawingPreferences {
  if (typeof window === 'undefined' || !preferenceKey.value) return {}
  try {
    const raw = window.localStorage.getItem(preferenceKey.value)
    if (!raw) return {}
    const value = JSON.parse(raw)
    return value && typeof value === 'object' && !Array.isArray(value) ? value as DrawingPreferences : {}
  } catch {
    return {}
  }
}

function validValue<T extends string>(value: unknown, values: readonly T[], fallback: T): T {
  return typeof value === 'string' && values.includes(value as T) ? value as T : fallback
}

function applyPreferences(value: DrawingPreferences) {
  color.value = validValue(value.color, COLOR_IDS, 'black')
  size.value = validValue(value.size, SIZE_IDS, 'm')
  dash.value = validValue(value.dash, DASH_IDS, 'draw')
  fill.value = validValue(value.fill, FILL_IDS, 'none')
  geoKind.value = validValue(value.geoKind, GEO_IDS, 'rectangle')
  grid.value = validValue(value.grid, SUPPORTED_GRID_IDS, props.grid)
  font.value = validValue(value.font, ['draw', 'sans', 'serif', 'mono'] as const, 'draw')
  penMode.value = value.penMode === true
  const savedInspectorPosition = value.inspectorPosition || value.parametersPosition
  if (savedInspectorPosition && Number.isFinite(savedInspectorPosition.x) && Number.isFinite(savedInspectorPosition.y)) {
    inspectorPosition.value = { x: savedInspectorPosition.x, y: savedInspectorPosition.y }
    hasSavedInspectorPosition = true
  }
  if (value.dockPosition && Number.isFinite(value.dockPosition.x) && Number.isFinite(value.dockPosition.y)) {
    dockPosition.value = { x: value.dockPosition.x, y: value.dockPosition.y }
    hasSavedDockPosition = true
  }
  inspectorOpen.value = value.inspectorOpen !== false
  inspectorCollapsed.value = value.inspectorCollapsed === true || value.parametersCollapsed === true
}

function persistPreferences() {
  if (typeof window === 'undefined' || !preferenceKey.value) return
  const value: DrawingPreferences = {
    // Keep the last tool as a preference for other controls and future
    // sessions, but opening a board always starts in select for safety.
    tool: activeTool.value,
    color: color.value,
    size: size.value,
    dash: dash.value,
    fill: fill.value,
    font: font.value,
    geoKind: geoKind.value,
    grid: grid.value,
    penMode: penMode.value,
    dockPosition: dockPosition.value,
    inspectorPosition: inspectorPosition.value,
    inspectorOpen: inspectorOpen.value,
    inspectorCollapsed: inspectorCollapsed.value,
  }
  try {
    window.localStorage.setItem(preferenceKey.value, JSON.stringify(value))
  } catch {
    // A private/embedded browsing context may deny storage. The editor still
    // works with the in-memory preference.
  }
}

function isToolbarTarget(target: EventTarget | null): boolean {
  return typeof Element !== 'undefined' && target instanceof Element && !!target.closest('.clue-board-drawing-toolbar, .clue-board-drawing-inspector, .qd-text-edit')
}

function clampFloatingPosition(target: 'dock' | 'inspector', position: { x: number; y: number }) {
  const host = container.value
  const panel = target === 'dock' ? dock.value : inspectorPanel.value
  if (!host || !panel) return position
  const inset = 8
  const maxX = Math.max(inset, host.clientWidth - panel.offsetWidth - inset)
  const maxY = Math.max(inset, host.clientHeight - panel.offsetHeight - inset)
  return {
    x: Math.min(maxX, Math.max(inset, position.x)),
    y: Math.min(maxY, Math.max(inset, position.y)),
  }
}

function clampFloatingPanels() {
  dockPosition.value = clampFloatingPosition('dock', dockPosition.value)
  if (inspectorOpen.value) inspectorPosition.value = clampFloatingPosition('inspector', inspectorPosition.value)
}

function placeFloatingDefaults() {
  const host = container.value
  if (!host) return
  if (!hasSavedDockPosition && dock.value) {
    dockPosition.value = {
      x: Math.max(8, (host.clientWidth - dock.value.offsetWidth) / 2),
      y: Math.max(8, host.clientHeight - dock.value.offsetHeight - 12),
    }
  }
  if (!hasSavedInspectorPosition && inspectorPanel.value) {
    inspectorPosition.value = {
      x: Math.max(8, host.clientWidth - inspectorPanel.value.offsetWidth - 16),
      y: 16,
    }
  }
  clampFloatingPanels()
}

function stopFloatingDrag() {
  if (!floatingDrag) return
  floatingDrag = null
  window.removeEventListener('pointermove', onFloatingPointerMove)
  window.removeEventListener('pointerup', onFloatingPointerUp)
  window.removeEventListener('pointercancel', onFloatingPointerUp)
}

function floatingLocalPoint(event: PointerEvent) {
  const host = container.value
  if (!host) return { x: event.clientX, y: event.clientY }
  const rect = host.getBoundingClientRect()
  return {
    x: (event.clientX - rect.left) * (host.clientWidth / (rect.width || host.clientWidth || 1)),
    y: (event.clientY - rect.top) * (host.clientHeight / (rect.height || host.clientHeight || 1)),
  }
}

function onFloatingPointerMove(event: PointerEvent) {
  const drag = floatingDrag
  if (!drag || drag.pointerId !== event.pointerId) return
  event.preventDefault()
  const point = floatingLocalPoint(event)
  const position = clampFloatingPosition(drag.target, { x: drag.x + point.x - drag.startX, y: drag.y + point.y - drag.startY })
  if (drag.target === 'dock') dockPosition.value = position
  else inspectorPosition.value = position
}

function onFloatingPointerUp(event: PointerEvent) {
  if (floatingDrag?.pointerId !== event.pointerId) return
  stopFloatingDrag()
  persistPreferences()
}

function startFloatingDrag(target: 'dock' | 'inspector', event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const position = target === 'dock' ? dockPosition.value : inspectorPosition.value
  const point = floatingLocalPoint(event)
  floatingDrag = {
    target,
    pointerId: event.pointerId,
    startX: point.x,
    startY: point.y,
    x: position.x,
    y: position.y,
  }
  window.addEventListener('pointermove', onFloatingPointerMove)
  window.addEventListener('pointerup', onFloatingPointerUp)
  window.addEventListener('pointercancel', onFloatingPointerUp)
}

function toggleInspectorCollapsed() {
  inspectorCollapsed.value = !inspectorCollapsed.value
  persistPreferences()
  void nextTick(clampFloatingPanels)
}

function toggleInspector() {
  inspectorOpen.value = !inspectorOpen.value
  persistPreferences()
  if (inspectorOpen.value) void nextTick(clampFloatingPanels)
}

function syncHistory() {
  const editor = adapter.value?.editor
  canUndo.value = !!editor?.store.canUndo
  canRedo.value = !!editor?.store.canRedo
}

function syncStyles() {
  const editor = adapter.value?.editor
  if (!editor) return
  const current = editor.currentStyles()
  if (hasValue(current.color, COLOR_IDS)) color.value = current.color
  if (hasValue(current.size, SIZE_IDS)) size.value = current.size
  if (hasValue(current.dash, DASH_IDS)) dash.value = current.dash
  if (hasValue(current.fill, FILL_IDS)) fill.value = current.fill
  if (hasValue(current.font, ['draw', 'sans', 'serif', 'mono'] as const)) font.value = current.font
}

function syncSelectionDetails() {
  const editor = adapter.value?.editor
  if (!editor) return
  const shapes = [...editor.selection]
    .map(id => editor.store.get(id))
    .filter((shape): shape is ShapeRecord => shape?.typeName === 'shape')
  selectedKinds.value = [...new Set(shapes.map(shape => shape.type))]
  if (shapes.length === 1) {
    const shape = shapes[0]
    if (shape.type === 'geo' && hasValue(shape.props.geo, GEO_IDS)) geoKind.value = shape.props.geo
    if (shape.type === 'text' && (shape.props.align === 'start' || shape.props.align === 'middle' || shape.props.align === 'end')) {
      selectedAlign.value = shape.props.align
    } else if (shape.type === 'text') selectedAlign.value = 'start'
    selectedBend.value = (shape.type === 'line' || shape.type === 'arrow') && Number.isFinite(shape.props.bend) ? Number(shape.props.bend) : 0
  } else {
    selectedAlign.value = 'start'
    selectedBend.value = 0
  }
}

function hasValue<T extends string>(value: unknown, values: readonly T[]): value is T {
  return typeof value === 'string' && values.includes(value as T)
}

function emitFinalSnapshot() {
  flushRealtimeDiff()
  const current = adapter.value?.getSnapshot()
  if (!current) {
    pendingUserChange = false
    emit('drawing-pending', false)
    return
  }
  const json = snapshotJSON(current)
  if (!json || json === lastCommittedSnapshot) {
    pendingUserChange = false
    emit('drawing-pending', false)
    return
  }
  pendingUserChange = false
  lastCommittedSnapshot = json
  lastEmittedSnapshot = json
  // Let the host clear its transient flag before it evaluates whether this
  // final snapshot was accepted by the Board size/validation guard. An
  // over-limit rejection sets the flag back to true in the host callback.
  emit('drawing-pending', false)
  emit('snapshot-change', current)
}

function finalizePending() {
  if (destroyed || !pendingUserChange || activePointers.size || textEditing.value || pendingImports.size || pendingOperations.size) return
  emitFinalSnapshot()
}

function queueFinalize() {
  if (finalizeQueued || destroyed) return
  finalizeQueued = true
  Promise.resolve().then(() => {
    finalizeQueued = false
    finalizePending()
  })
}

function trackImport(task: Promise<void>): Promise<void> {
  let tracked!: Promise<void>
  tracked = Promise.resolve(task)
    .catch(() => undefined)
    .finally(() => {
      pendingImports.delete(tracked)
      if (!pendingUserChange && !pendingImports.size && !pendingOperations.size) emit('drawing-pending', false)
      queueFinalize()
    })
  pendingImports.add(tracked)
  return tracked
}

function trackOperation(task: Promise<void>): Promise<void> {
  let tracked!: Promise<void>
  tracked = Promise.resolve(task)
    .catch(() => undefined)
    .finally(() => {
      pendingOperations.delete(tracked)
      queueFinalize()
    })
  pendingOperations.add(tracked)
  return tracked
}

async function waitForPendingWork() {
  while (pendingImports.size || pendingOperations.size) {
    const pending = [...pendingImports, ...pendingOperations]
    await Promise.all(pending)
  }
}

function onStoreChange(diff: Diff, source: string) {
  publishEndpoints()
  syncSelectionDetails()
  if (source !== 'user') return
  // Any persisted Quickdraw mutation makes the drawing the active surface,
  // including keyboard actions that do not have a preceding pointer event.
  emit('focus')
  emit('drawing-pending', true)
  pendingUserChange = true
  if (props.realtimeDiffEnabled && !props.readonly) {
    pendingRealtimeDiff = pendingRealtimeDiff ? composeDiff(pendingRealtimeDiff, diff) : diff
    if (!realtimeDiffTimer) realtimeDiffTimer = setTimeout(flushRealtimeDiff, 50)
  }
  queueFinalize()
}

function applyIncomingSnapshot(value: Snapshot | null | undefined, force = false) {
  const qd = adapter.value
  if (!qd) return
  const next = value || emptySnapshot()
  const json = snapshotJSON(next)
  if (!json || (!force && (json === lastAppliedSnapshot || json === lastEmittedSnapshot))) return
  qd.loadSnapshot(next)
  const safeTool: ToolId = props.interactionLocked ? 'hand' : 'select'
  if (qd.editor.tool !== safeTool) qd.editor.setTool(safeTool)
  qd.editor.setSelection([])
  lastAppliedSnapshot = json
  lastEmittedSnapshot = ''
  lastCommittedSnapshot = snapshotJSON(qd.getSnapshot())
  pendingUserChange = false
  hasSelection.value = false
  selectedKinds.value = []
  syncHistory()
  emit('snapshot-loaded')
  publishEndpoints()
}

function setTool(tool: ToolId) {
  const editor = adapter.value?.editor
  if (!editor || props.readonly || (props.interactionLocked && tool !== 'hand')) return
  editor.setTool(tool)
  activeTool.value = editor.tool
  persistPreferences()
  emit('tool-change', activeTool.value)
}

function setStyle<K extends 'color' | 'size' | 'dash' | 'fill' | 'font'>(key: K, value: K extends 'color' ? ColorId : K extends 'size' ? SizeId : K extends 'dash' ? DashId : K extends 'fill' ? FillId : FontId) {
  const editor = adapter.value?.editor
  if (!editor || props.readonly || props.interactionLocked) return
  editor.setStyle(key, value as never)
  if (key === 'color') color.value = value as ColorId
  if (key === 'size') size.value = value as SizeId
  if (key === 'dash') dash.value = value as DashId
  if (key === 'fill') fill.value = value as FillId
  if (key === 'font') font.value = value as FontId
  persistPreferences()
}

function setGeo(value: GeoId) {
  const editor = adapter.value?.editor
  if (!editor || props.readonly || props.interactionLocked) return
  geoKind.value = value
  editor.setGeoKind(value)
  if (selectedKinds.value.includes('geo')) {
    updateSelectedShapeProps(shape => shape.type === 'geo', shape => ({ ...shape.props, geo: value }))
    persistPreferences()
    return
  }
  setTool('geo')
}

function setGrid(value: GridId) {
  const editor = adapter.value?.editor
  if (!editor || props.readonly) return
  grid.value = value
  editor.setGrid(value)
  persistPreferences()
}

function setPenMode(value: boolean) {
  const editor = adapter.value?.editor
  if (!editor || props.readonly || props.interactionLocked) return
  editor.setPenMode(value)
  penMode.value = editor.penMode
  persistPreferences()
}

function updateSelectedShapeProps(
  accepts: (shape: ShapeRecord) => boolean,
  patch: (shape: ShapeRecord) => Record<string, unknown>,
) {
  const editor = adapter.value?.editor
  if (!editor || props.readonly || props.interactionLocked) return
  editor.store.transact(() => {
    for (const id of editor.selection) {
      const shape = editor.store.get(id)
      if (shape?.typeName !== 'shape' || !accepts(shape)) continue
      editor.store.update(id, { props: patch(shape) })
    }
  })
  syncSelectionDetails()
}

function setTextAlign(value: 'start' | 'middle' | 'end') {
  updateSelectedShapeProps(shape => shape.type === 'text', shape => ({ ...shape.props, align: value }))
}

function setLineBend(value: number) {
  updateSelectedShapeProps(shape => shape.type === 'line' || shape.type === 'arrow', shape => ({ ...shape.props, bend: value }))
}

function deleteDrawingSelection() {
  if (props.readonly || props.interactionLocked) return
  adapter.value?.editor.deleteSelection()
}

function bringDrawingToFront() {
  if (props.readonly || props.interactionLocked) return
  adapter.value?.editor.bringToFront()
}

function sendDrawingToBack() {
  if (props.readonly || props.interactionLocked) return
  adapter.value?.editor.sendToBack()
}

function undo() {
  if (props.readonly || props.interactionLocked) return
  commitText()
  adapter.value?.editor.store.undo()
}

function redo() {
  if (props.readonly || props.interactionLocked) return
  commitText()
  adapter.value?.editor.store.redo()
}

function clearDrawing() {
  if (props.readonly || props.interactionLocked) return
  commitText()
  adapter.value?.editor.clearBoard()
}

function pickImage() {
  if (props.readonly || props.interactionLocked) return
  const editor = adapter.value?.editor
  if (!editor) return
  // Core's public pickImage() intentionally returns void, so this equivalent
  // in-surface picker feeds the same public importImageBlobs() API and keeps
  // its completion promise available to the close lifecycle.
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.multiple = true
  input.tabIndex = -1
  input.style.display = 'none'
  container.value?.appendChild(input)
  const cleanup = () => {
    pickerInputs.delete(input)
    input.removeEventListener('change', onChange)
    input.removeEventListener('cancel', onCancel)
    input.remove()
  }
  const onChange = () => {
    const files = [...(input.files || [])]
    cleanup()
    if (files.length) void importImageFiles(files)
  }
  const onCancel = () => cleanup()
  pickerInputs.set(input, cleanup)
  input.addEventListener('change', onChange, { once: true })
  // Chromium and WebKit expose a cancel event for an opened file picker;
  // Firefox simply leaves the hidden input untouched, which is cleaned up on
  // unmount below.
  input.addEventListener('cancel', onCancel, { once: true })
  input.click()
}

async function copyDrawing() {
  if (props.readonly || props.interactionLocked) return
  await adapter.value?.editor.copySelection()
}

async function cutDrawing() {
  if (props.readonly || props.interactionLocked) return
  const editor = adapter.value?.editor
  if (!editor) return
  await editor.copySelection()
  if (!props.readonly && !props.interactionLocked && !destroyed) editor.deleteSelection()
}

async function pasteDrawing() {
  if (props.readonly || props.interactionLocked) return
  commitText()
  const editor = adapter.value?.editor
  if (!editor) return
  // Prefer the image MIME path so the returned promise is the same public
  // import promise that the close lifecycle waits for. Shape payloads still
  // use Quickdraw's public clipboard parser below.
  let sawImageType = false
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.read) {
      const items = await navigator.clipboard.read()
      for (const item of items) {
        const type = item.types.find(value => value.startsWith('image/'))
        if (!type) continue
        sawImageType = true
        const blob = await item.getType(type)
        await importImageFiles([blob])
        return
      }
    }
  } catch {
    if (sawImageType) return
    // Permission-denied clipboard reads fall through to Quickdraw's text
    // payload path, which preserves its existing browser behavior.
  }
  if (props.interactionLocked || destroyed) return
  await editor.pasteFromClipboard()
}

function startPasteDrawing() {
  if (props.readonly || props.interactionLocked) return
  emit('drawing-pending', true)
  const task = pasteDrawing().finally(() => {
    if (!pendingUserChange) emit('drawing-pending', false)
  })
  void trackOperation(task)
}

function startCutDrawing() {
  if (props.readonly || props.interactionLocked) return
  emit('drawing-pending', true)
  const task = cutDrawing().finally(() => {
    if (!pendingUserChange) emit('drawing-pending', false)
  })
  void trackOperation(task)
}

function fitContent() {
  const qd = adapter.value
  if (!qd) return false
  return qd.fitContent({ margin: 48, maxZoom: 2.5, animate: 180 })
}

function fitBoard(nodes: BoardElementGeometry[]) {
  const qd = adapter.value
  if (!qd) return false
  const drawing = qd.camera.contentBounds()
  const rects = [...nodes.map(node => ({ x: node.x, y: node.y, w: node.width, h: node.height })), ...(drawing ? [drawing] : [])]
  if (!rects.length) return false
  const x = Math.min(...rects.map(rect => rect.x))
  const y = Math.min(...rects.map(rect => rect.y))
  return qd.camera.fitBounds({ x, y, w: Math.max(...rects.map(rect => rect.x + rect.w)) - x, h: Math.max(...rects.map(rect => rect.y + rect.h)) - y }, { margin: 70, maxZoom: 2.5 })
}

function focusElement(element: BoardElementGeometry) {
  const qd = adapter.value
  if (!qd) return false
  return qd.camera.fitBounds(
    { x: element.x, y: element.y, w: element.width, h: element.height },
    { margin: 96, maxZoom: 1.5, animate: 220 },
  )
}

function pan(dx: number, dy: number) { adapter.value?.editor.pan(dx, dy) }

function publishEndpoints() {
  const editor = adapter.value?.editor
  if (!editor) return
  // Only public shape records and selection IDs are used; drawings remain Board objects.
  const endpoints = editor.shapesSorted().flatMap((shape): BoardDrawingEndpoint[] => {
    if (shape.type !== 'note' && shape.type !== 'text') return []
    const bounds = pageBounds(shape)
    const text = typeof shape.props.text === 'string' ? shape.props.text : ''
    return [{ id: shape.id, x: bounds.x, y: bounds.y, width: bounds.w, height: bounds.h,
      name: text.split(/\r?\n/)[0].trim() || (shape.type === 'note' ? '便签' : '文本'),
      quickdrawKind: shape.type === 'note' ? 'sticky' : 'text', selected: editor.selection.size === 1 && editor.selection.has(shape.id) }]
  })
  emit('endpoints-change', endpoints)
}

function hitEndpoint(clientX: number, clientY: number): string | null {
  const qd = adapter.value
  if (!qd) return null
  const point = qd.camera.clientToWorld(clientX, clientY)
  const shape = qd.editor.hitTest(point.x, point.y)
  return shape && (shape.type === 'note' || shape.type === 'text') ? shape.id : null
}

function commitText() {
  const editor = adapter.value?.editor
  if (!editor) return
  // setTool is public and commits an active text editor before selecting the
  // requested tool. Re-setting the current tool is intentional.
  editor.setTool(editor.tool)
  textEditing.value = !!container.value?.querySelector('.qd-text-edit')
  if (!textEditing.value) queueFinalize()
}

async function flush(): Promise<boolean> {
  commitText()
  await waitForPendingWork()
  await Promise.resolve()
  flushRealtimeDiff()
  if (activePointers.size || textEditing.value) return false
  finalizePending()
  return !pendingUserChange
}

function clearSelection() {
  adapter.value?.editor.setSelection([])
}

function getSnapshot(): Snapshot | null {
  return adapter.value?.getSnapshot() || null
}

function importImageFiles(files: Blob[] | File[], at?: { x: number; y: number }): Promise<void> {
  if (props.readonly || props.interactionLocked || !files.length || destroyed) return Promise.resolve()
  const editor = adapter.value?.editor
  if (!editor) return Promise.resolve()
  emit('drawing-pending', true)
  return trackImport(editor.importImageBlobs(files, at))
}

function pagePointFor(event: DragEvent): { x: number; y: number } | undefined {
  const qd = adapter.value
  if (!qd) return undefined
  const local = qd.camera.clientToLocal(event.clientX, event.clientY)
  const page = qd.camera.screenToWorld(local)
  return Number.isFinite(page.x) && Number.isFinite(page.y) ? page : undefined
}

function clipboardImageFiles(data: DataTransfer | null): Blob[] {
  if (!data) return []
  const files = [...(data.files || [])].filter(file => file.type.startsWith('image/'))
  if (files.length) return files
  return [...(data.items || [])]
    .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
    .map(item => item.getAsFile())
    .filter((file): file is File => !!file)
}

function onPasteCapture(event: ClipboardEvent) {
  if (props.readonly || props.interactionLocked || isToolbarTarget(event.target) || textEditing.value) return
  const files = clipboardImageFiles(event.clipboardData)
  if (!files.length) return
  event.preventDefault()
  event.stopImmediatePropagation()
  emit('focus')
  void importImageFiles(files)
}

function onDropCapture(event: DragEvent) {
  if (props.readonly || props.interactionLocked || isToolbarTarget(event.target)) return
  const files = [...(event.dataTransfer?.files || [])].filter(file => file.type.startsWith('image/'))
  if (!files.length) return
  event.preventDefault()
  event.stopImmediatePropagation()
  emit('focus')
  void importImageFiles(files, pagePointFor(event))
}

function zoomAt(clientX: number, clientY: number, multiplier: number) {
  const qd = adapter.value
  if (!qd || !Number.isFinite(multiplier) || multiplier <= 0) return
  qd.camera.zoomAt(qd.camera.clientToLocal(clientX, clientY), multiplier)
}

function onKeyDownCapture(event: KeyboardEvent) {
  if (isToolbarTarget(event.target) || event.isComposing) return
  if (props.interactionLocked) {
    if (!['Control', 'Meta', 'Alt', 'Shift'].includes(event.key)) {
      event.preventDefault()
      event.stopImmediatePropagation()
    }
    return
  }
  const key = event.key.toLowerCase()
  if ((event.metaKey || event.ctrlKey) && (key === '=' || key === '+' || key === '-')) {
    event.preventDefault()
    event.stopImmediatePropagation()
    const qd = adapter.value
    if (!qd) return
    const { w, h } = qd.editor.viewSize()
    qd.camera.zoomAt({ x: w / 2, y: h / 2 }, key === '-' ? 1 / KEYBOARD_ZOOM_FACTOR : KEYBOARD_ZOOM_FACTOR)
    emit('focus')
    return
  }
  if (props.readonly) return
  if (!(event.metaKey || event.ctrlKey)) return
  if (key === 'z') {
    // Quickdraw's own key listener is intentionally bypassed here so the
    // board's capture handler also works when the canvas is embedded in a
    // host that handles keyboard shortcuts on the same element.
    event.preventDefault()
    event.stopImmediatePropagation()
    emit('focus')
    event.shiftKey ? redo() : undo()
    return
  }
  if (key !== 'v' && key !== 'x') return
  // On older browsers the native paste event is the only image path; leave
  // Ctrl/Cmd+V to Quickdraw so its paste listener can receive the files.
  if (key === 'v' && (typeof navigator === 'undefined' || !navigator.clipboard?.read)) return
  event.preventDefault()
  event.stopImmediatePropagation()
  emit('focus')
  key === 'v' ? startPasteDrawing() : startCutDrawing()
}

function reloadSnapshot() {
  applyIncomingSnapshot(props.snapshot, true)
}

function onPointerDown(event: PointerEvent) {
  if (props.readonly || isToolbarTarget(event.target)) return
  const penIsDown = Array.from(activePointerTypes.values()).some(type => type === 'pen')
  // Quickdraw ignores a resting palm while a pen stroke is active. Mirror
  // that public pen-mode boundary so the palm cannot keep a save batch open.
  if (adapter.value?.editor.penMode && event.pointerType === 'touch' && penIsDown) return
  activePointers.add(event.pointerId)
  activePointerTypes.set(event.pointerId, event.pointerType)
  emit('focus')
}

function onPointerUp(event: PointerEvent) {
  flushRealtimeDiff()
  activePointers.delete(event.pointerId)
  activePointerTypes.delete(event.pointerId)
  if (!activePointers.size) queueFinalize()
}

function onEditorSelection() {
  const selected = !!adapter.value?.editor.selection.size
  hasSelection.value = selected
  syncStyles()
  syncSelectionDetails()
  emit('selection-change', selected)
  publishEndpoints()
}

function onEditorTool() {
  const editor = adapter.value?.editor
  if (!editor) return
  if (props.interactionLocked && editor.tool !== 'hand') {
    editor.setTool('hand')
    return
  }
  emit('focus')
  activeTool.value = editor.tool
  syncStyles()
  persistPreferences()
  emit('tool-change', activeTool.value)
}

function onEditorEdit() {
  textEditing.value = !!container.value?.querySelector('.qd-text-edit')
  if (!textEditing.value) queueFinalize()
}

function onEditorCamera() {
  const qd = adapter.value
  if (qd) emit('camera-change', qd.camera.readCamera())
}

watch(() => props.snapshot, value => applyIncomingSnapshot(value), { deep: false })
watch(() => props.theme, value => {
  if (adapter.value) adapter.value.editor.setTheme(value)
})
watch(() => props.grid, value => {
  if (value !== grid.value) setGrid(value)
})
watch(() => props.readonly, value => {
  adapter.value?.editor.setReadonly(value)
})
watch(() => props.interactionLocked, value => {
  if (!value) return
  commitText()
  adapter.value?.editor.setTool('hand')
  activeTool.value = 'hand'
  emit('tool-change', 'hand')
})

onMounted(() => {
  applyPreferences(readPreferences())
  const host = container.value
  if (!host) return
  const qd = new QuickdrawAdapter(host, { theme: props.theme, grid: grid.value })
  adapter.value = qd
  qd.editor.setGeoKind(geoKind.value)
  qd.editor.setStyle('color', color.value)
  qd.editor.setStyle('size', size.value)
  qd.editor.setStyle('dash', dash.value)
  qd.editor.setStyle('fill', fill.value)
  qd.editor.setStyle('font', font.value)
  qd.editor.setPenMode(penMode.value)
  qd.editor.setReadonly(props.readonly)
  // Quickdraw's engine defaults to draw. A board must never open with a
  // pointer-down capable of leaving an accidental mark.
  qd.editor.setTool(props.interactionLocked ? 'hand' : 'select')
  activeTool.value = qd.editor.tool

  unsubs = [
    qd.listenChanges(onStoreChange, 'user'),
    qd.listenHistory(syncHistory),
    qd.editor.on('selection', onEditorSelection),
    qd.editor.on('tool', onEditorTool),
    qd.editor.on('styles', syncStyles),
    qd.editor.on('penmode', () => { penMode.value = qd.editor.penMode }),
    qd.editor.on('edit', onEditorEdit),
    qd.editor.on('camera', onEditorCamera),
  ]
  host.addEventListener('pointerdown', onPointerDown, true)
  host.addEventListener('pointerup', onPointerUp)
  host.addEventListener('pointercancel', onPointerUp)
  host.addEventListener('paste', onPasteCapture, true)
  host.addEventListener('drop', onDropCapture, true)
  host.addEventListener('keydown', onKeyDownCapture, true)
  resizeObserver = new ResizeObserver(() => {
    containerHeight.value = host.clientHeight
    void nextTick(clampFloatingPanels)
  })
  resizeObserver.observe(host)
  containerHeight.value = host.clientHeight
  applyIncomingSnapshot(props.snapshot)
  syncHistory()
  emit('camera-change', qd.camera.readCamera())
  void nextTick(placeFloatingDefaults)
})

onBeforeUnmount(() => {
  // Commit text while the editor and its textarea still exist. The parent
  // close path normally flushes first; this is the final lifecycle safeguard.
  commitText()
  finalizePending()
  destroyed = true
  flushRealtimeDiff()
  stopFloatingDrag()
  resizeObserver?.disconnect()
  resizeObserver = null
  const host = container.value
  if (host) {
    host.removeEventListener('pointerdown', onPointerDown, true)
    host.removeEventListener('pointerup', onPointerUp)
    host.removeEventListener('pointercancel', onPointerUp)
    host.removeEventListener('paste', onPasteCapture, true)
    host.removeEventListener('drop', onDropCapture, true)
    host.removeEventListener('keydown', onKeyDownCapture, true)
  }
  for (const cleanup of [...pickerInputs.values()]) cleanup()
  pickerInputs.clear()
  for (const unsubscribe of unsubs) unsubscribe()
  unsubs = []
  adapter.value?.destroy()
  adapter.value = null
  activePointers.clear()
  activePointerTypes.clear()
  pendingImports.clear()
  pendingOperations.clear()
})

defineExpose({
  applyRemoteDiff,
  clearSelection,
  commitText,
  flush,
  fitContent,
  fitBoard,
  focusElement,
  pan,
  zoomAt,
  hitEndpoint,
  getSnapshot,
  importImageFiles,
  reloadSnapshot,
  hasSelection,
  setTool,
})
</script>

<template>
  <div ref="container" class="clue-board-drawing-surface">
    <div ref="dock" class="clue-board-drawing-toolbar" :style="dockStyle" data-qd-seal-toolbar @keydown.stop="emit('focus')" @pointerdown.stop="emit('focus')">
      <div class="clue-board-drawing-toolbar__tools" role="toolbar" aria-label="绘图工具">
        <NTooltip>
          <template #trigger><button type="button" class="clue-board-drawing-toolbar__button" :class="{ 'is-active': interactionLocked }" :aria-pressed="interactionLocked" :aria-label="interactionLocked ? '解除防误编辑锁' : '锁定编辑交互'" @click.stop="emit('toggle-lock')"><NIcon :size="19"><component :is="interactionLocked ? Lock : LockOpen" /></NIcon></button></template>
          {{ interactionLocked ? '解除防误编辑锁' : '防误编辑锁' }}
        </NTooltip>
        <NTooltip>
          <template #trigger><button type="button" class="clue-board-drawing-toolbar__button clue-board-drawing-toolbar__grip" aria-label="拖动工具栏" @pointerdown.stop="startFloatingDrag('dock', $event)"><NIcon :size="18"><GripVertical /></NIcon></button></template>
          拖动工具栏
        </NTooltip>
        <span class="clue-board-drawing-toolbar__divider" />
        <NTooltip v-for="option in TOOL_OPTIONS" :key="option.id" trigger="hover">
          <template #trigger>
            <button
              type="button"
              class="clue-board-drawing-toolbar__button"
              :class="{ 'is-active': activeTool === option.id }"
              :aria-label="option.label"
              :aria-pressed="activeTool === option.id"
              :disabled="readonly || (interactionLocked && option.id !== 'hand')"
              @click.stop="setTool(option.id)"
            ><NIcon :size="20"><component :is="toolIcons[option.id as keyof typeof toolIcons]" /></NIcon></button>
          </template>
          {{ option.title }}
        </NTooltip>
        <NTooltip><template #trigger><button type="button" class="clue-board-drawing-toolbar__button" aria-label="图片" :disabled="readonly || interactionLocked" @click.stop="pickImage"><NIcon :size="20"><Photo /></NIcon></button></template>插入图片</NTooltip>
        <span class="clue-board-drawing-toolbar__divider" />
        <NTooltip><template #trigger><button type="button" class="clue-board-drawing-toolbar__button" :class="{ 'is-active': inspectorOpen }" aria-label="属性面板" :aria-expanded="inspectorOpen" @click.stop="toggleInspector"><NIcon :size="19"><Adjustments /></NIcon></button></template>{{ inspectorOpen ? '关闭属性面板' : '打开属性面板' }}</NTooltip>
      </div>
    </div>
    <div
      v-show="inspectorOpen"
      ref="inspectorPanel"
      class="clue-board-drawing-inspector-host"
      :style="inspectorPanelStyle"
      @keydown.stop="emit('focus')"
      @pointerdown.stop="emit('focus')"
    >
      <ClueBoardDrawingInspector
        :theme="theme"
        :active-tool="activeTool"
        :color="color"
        :size="size"
        :dash="dash"
        :fill="fill"
        :font="font"
        :geo-kind="geoKind"
        :grid="grid"
        :pen-mode="penMode"
        :has-selection="hasSelection"
        :selected-kinds="selectedKinds"
        :selected-align="selectedAlign"
        :selected-bend="selectedBend"
        :can-undo="canUndo"
        :can-redo="canRedo"
        :collapsed="inspectorCollapsed"
        :max-height="inspectorMaxHeight"
        :disabled="readonly || interactionLocked"
        @drag-start="startFloatingDrag('inspector', $event)"
        @toggle-collapse="toggleInspectorCollapsed"
        @close="toggleInspector"
        @color="setStyle('color', $event)"
        @size="setStyle('size', $event)"
        @dash="setStyle('dash', $event)"
        @fill="setStyle('fill', $event)"
        @font="setStyle('font', $event)"
        @geo="setGeo"
        @grid="setGrid"
        @pen-mode="setPenMode"
        @align="setTextAlign"
        @bend="setLineBend"
        @copy="copyDrawing"
        @cut="startCutDrawing"
        @delete="deleteDrawingSelection"
        @front="bringDrawingToFront"
        @back="sendDrawingToBack"
        @undo="undo"
        @redo="redo"
        @paste="startPasteDrawing"
        @fit="fitContent"
        @clear="clearDrawing"
      />
    </div>
  </div>
</template>

<style scoped>
.clue-board-drawing-surface { position: absolute; inset: 0; min-width: 0; min-height: 0; overflow: hidden; }
.clue-board-drawing-surface :deep(.qd-root) { border-radius: 0; }
.clue-board-drawing-toolbar { position: absolute; z-index: 50; width: max-content; max-width: calc(100% - 16px); color: var(--sc-text-primary); }
.clue-board-drawing-toolbar__tools { display: flex; max-width: 100%; align-items: center; gap: 3px; padding: 6px; overflow-x: auto; border: 1px solid var(--sc-border-mute); border-radius: 14px; background: color-mix(in srgb, var(--sc-bg-elevated) 96%, transparent); box-shadow: 0 9px 26px #0003; backdrop-filter: blur(12px); scrollbar-width: none; }
.clue-board-drawing-toolbar__tools::-webkit-scrollbar { display: none; }
.clue-board-drawing-toolbar__button { display: grid; width: 34px; height: 34px; flex: 0 0 34px; padding: 0; place-items: center; color: var(--sc-text-primary); border: 1px solid transparent; border-radius: 8px; background: transparent; cursor: pointer; white-space: nowrap; }
.clue-board-drawing-toolbar__button:hover:not(:disabled) { border-color: var(--sc-border-mute); background: color-mix(in srgb, var(--primary-color, #3388de) 12%, transparent); }
.clue-board-drawing-toolbar__button.is-active { color: var(--primary-color, #3388de); border-color: color-mix(in srgb, var(--primary-color, #3388de) 55%, transparent); background: color-mix(in srgb, var(--primary-color, #3388de) 16%, transparent); }
.clue-board-drawing-toolbar__button:disabled { cursor: not-allowed; opacity: .45; }
.clue-board-drawing-toolbar__grip { color: var(--sc-text-secondary); cursor: grab; touch-action: none; }
.clue-board-drawing-toolbar__grip:active { cursor: grabbing; }
.clue-board-drawing-toolbar__divider { width: 1px; height: 22px; flex: 0 0 1px; margin: 0 2px; background: var(--sc-border-mute); }
.clue-board-drawing-inspector-host { position: absolute; z-index: 51; width: max-content; max-width: calc(100% - 16px); }
@media (max-width: 760px) { .clue-board-drawing-toolbar__tools { gap: 1px; padding: 5px; } .clue-board-drawing-toolbar__button { width: 31px; height: 32px; flex-basis: 31px; } }
</style>
