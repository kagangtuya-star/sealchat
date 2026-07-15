<script setup lang="ts">
import Konva from 'konva'
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NButtonGroup, NCheckbox, NIcon, NInput, NInputNumber, NSelect, NTooltip } from 'naive-ui'
import {
  ArrowBackUp,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  Bolt,
  Clipboard,
  Components,
  Copy,
  Cut,
  Edit,
  FolderPlus,
  Focus,
  GripVertical,
  LayoutSidebarLeftExpand,
  LetterT,
  Message,
  Photo,
  Pin,
  Plus,
  Rectangle,
  Refresh,
  Stack2,
  Trash,
  X,
} from '@vicons/tabler'
import { api, urlBase } from '@/stores/_config'
import { compressImage } from '@/composables/useImageCompressor'
import {
  WORLD_UNIT_PX,
  resolveStageImageUrl,
  type StageAction,
  type StageActionTriggeredPayload,
  type StageImageRef,
  type StageObject,
  type StageObjectFit,
} from '../shared/stage-types'
import { stageActionSchema, type ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { syncStageObjectHierarchy } from './stage-layering'
import type { TheaterStageStore } from './StageStore'

const props = defineProps<{
  store: TheaterStageStore
  worldId: string
  channelId: string
  characterSnapshot: ChatCharactersSnapshotPayload
  chatBridgeOnline: boolean
  chatVisible: boolean
  syncReady: boolean
  syncing: boolean
  permissions: string[]
}>()
const emit = defineEmits<{
  actionTriggered: [payload: StageActionTriggeredPayload]
  selectCharacter: [identityId: string]
  selectCharacterVariant: [payload: { identityId: string, variantId: string | null }]
  toggleChat: []
  resetLayout: []
  exitTheater: []
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const viewportRef = ref<HTMLDivElement | null>(null)
const imageInputRef = ref<HTMLInputElement | null>(null)
const resourceError = ref('')
const resourceUploading = ref(false)
const scenePanelOpen = ref(false)
const inspectorPanelOpen = ref(false)
const layerPanelOpen = ref(false)
const MessageImageEditor = defineAsyncComponent(() => import('@/components/chat/MessageImageEditor.vue'))

type ImageTarget =
  | { kind: 'scene', target: 'background' | 'foreground' }
  | { kind: 'object', objectId: string }

interface TheaterResourceResponse {
  resource?: {
    id?: string
    status?: string
    processing?: { errorCode?: string }
  }
}

const pendingImageTarget = ref<ImageTarget | null>(null)
const imageEditorTarget = ref<ImageTarget | null>(null)
const imageEditorFile = ref<File | null>(null)
const imageEditorVisible = ref(false)
const draggedLayerId = ref<string | null>(null)
const workspaceRef = ref<HTMLDivElement | null>(null)
const hasPermission = (permission: string) => props.syncReady && props.permissions.includes(permission)
const canEditAllObjects = computed(() => hasPermission('stage.object.edit'))
const canEditDelegatedObjects = computed(() => hasPermission('stage.object.edit.delegated'))
const canSwitchScene = computed(() => hasPermission('stage.scene.switch'))
const canTriggerActions = computed(() => hasPermission('stage.action.trigger'))
const canUploadResources = computed(() => hasPermission('stage.resource.upload'))
const canEditObject = (object: StageObject | null | undefined) => Boolean(object) && (
  canEditAllObjects.value
  || (canEditDelegatedObjects.value && object!.editable && !object!.locked)
)

const isEditableShortcutTarget = (target: EventTarget | null) => {
  const element = target instanceof HTMLElement ? target : null
  return Boolean(element?.closest('input, textarea, select, [contenteditable="true"]'))
}

const handleStageShortcut = (event: KeyboardEvent) => {
  if (
    event.isComposing
    || event.altKey
    || !(event.ctrlKey || event.metaKey)
    || isEditableShortcutTarget(event.target)
    || imageEditorVisible.value
  ) return
  const key = event.key.toLowerCase()
  let handled = false
  if (key === 'c') handled = props.store.copySelectedObject()
  else if (key === 'x' && canEditAllObjects.value) handled = props.store.cutSelectedObject()
  else if (key === 'v' && canEditAllObjects.value) handled = Boolean(props.store.pasteObject())
  else if (key === 'z' && !event.shiftKey && canEditAllObjects.value) handled = props.store.undo()
  if (handled) event.preventDefault()
}

type PanelId = 'scene' | 'inspector' | 'layer'
interface PanelLayout {
  x: number
  y: number
  width: number
  height: number
}

const panelLayoutStorageKey = 'sealchat:theater-panel-layout:v1'
const panelMinimums: Record<PanelId, { width: number, height: number }> = {
  scene: { width: 140, height: 180 },
  inspector: { width: 240, height: 240 },
  layer: { width: 180, height: 220 },
}
const readPanelLayouts = (): Partial<Record<PanelId, PanelLayout>> => {
  try {
    const value = JSON.parse(localStorage.getItem(panelLayoutStorageKey) || '{}')
    return value && typeof value === 'object' ? value : {}
  } catch {
    return {}
  }
}
const panelLayouts = ref<Partial<Record<PanelId, PanelLayout>>>(readPanelLayouts())
let panelResizeObserver: ResizeObserver | null = null
let draggingPanel: { id: PanelId, pointerX: number, pointerY: number, x: number, y: number } | null = null

const panelDefaultLayout = (id: PanelId): PanelLayout => {
  const workspace = workspaceRef.value
  const workspaceWidth = workspace?.clientWidth || 960
  const workspaceHeight = workspace?.clientHeight || 640
  const width = id === 'scene' ? 168 : id === 'inspector' ? 280 : 220
  const height = Math.max(panelMinimums[id].height, workspaceHeight - 24)
  return {
    x: id === 'scene' ? 12 : Math.max(12, workspaceWidth - width - 12),
    y: 12,
    width,
    height,
  }
}

const clampPanelLayout = (id: PanelId, layout: PanelLayout): PanelLayout => {
  const workspaceWidth = Math.max(1, workspaceRef.value?.clientWidth || 960)
  const workspaceHeight = Math.max(1, workspaceRef.value?.clientHeight || 640)
  const minimum = panelMinimums[id]
  const width = Math.min(workspaceWidth, Math.max(minimum.width, Number(layout.width) || minimum.width))
  const height = Math.min(workspaceHeight, Math.max(minimum.height, Number(layout.height) || minimum.height))
  return {
    x: Math.min(Math.max(0, Number(layout.x) || 0), Math.max(0, workspaceWidth - width)),
    y: Math.min(Math.max(0, Number(layout.y) || 0), Math.max(0, workspaceHeight - height)),
    width,
    height,
  }
}

const ensurePanelLayout = (id: PanelId) => {
  const next = clampPanelLayout(id, panelLayouts.value[id] || panelDefaultLayout(id))
  panelLayouts.value = { ...panelLayouts.value, [id]: next }
  return next
}

const persistPanelLayouts = () => {
  try {
    localStorage.setItem(panelLayoutStorageKey, JSON.stringify(panelLayouts.value))
  } catch {
    // Private browsing or storage policy may disable local persistence.
  }
}

const panelStyle = (id: PanelId) => {
  const layout = panelLayouts.value[id]
  if (!layout) return undefined
  return {
    left: `${layout.x}px`,
    top: `${layout.y}px`,
    width: `${layout.width}px`,
    height: `${layout.height}px`,
  }
}

const togglePanel = (id: PanelId) => {
  if (id === 'scene') scenePanelOpen.value = !scenePanelOpen.value
  else if (id === 'inspector') inspectorPanelOpen.value = !inspectorPanelOpen.value
  else layerPanelOpen.value = !layerPanelOpen.value
}

const resetWorkspaceLayout = async () => {
  panelLayouts.value = {}
  persistPanelLayouts()
  emit('resetLayout')
  await nextTick()
  const openPanels: [PanelId, boolean][] = [
    ['scene', scenePanelOpen.value],
    ['inspector', inspectorPanelOpen.value],
    ['layer', layerPanelOpen.value],
  ]
  openPanels.forEach(([id, open]) => {
    if (open) ensurePanelLayout(id)
  })
  observeOpenPanels()
}

const startPanelDrag = (id: PanelId, event: PointerEvent) => {
  if (event.button !== 0 || (event.target as HTMLElement).closest('button, input, textarea, select')) return
  const layout = ensurePanelLayout(id)
  const heading = event.currentTarget as HTMLElement
  draggingPanel = { id, pointerX: event.clientX, pointerY: event.clientY, x: layout.x, y: layout.y }
  heading.setPointerCapture(event.pointerId)
  event.preventDefault()
}

const movePanel = (event: PointerEvent) => {
  if (!draggingPanel) return
  const current = panelLayouts.value[draggingPanel.id] || panelDefaultLayout(draggingPanel.id)
  const next = clampPanelLayout(draggingPanel.id, {
    ...current,
    x: draggingPanel.x + event.clientX - draggingPanel.pointerX,
    y: draggingPanel.y + event.clientY - draggingPanel.pointerY,
  })
  panelLayouts.value = { ...panelLayouts.value, [draggingPanel.id]: next }
}

const stopPanelDrag = () => {
  if (!draggingPanel) return
  draggingPanel = null
  persistPanelLayouts()
}

const observeOpenPanels = () => {
  panelResizeObserver?.disconnect()
  workspaceRef.value?.querySelectorAll<HTMLElement>('.theater-floating-panel').forEach((element) => panelResizeObserver?.observe(element))
}

const clampOpenPanels = () => {
  const ids: PanelId[] = ['scene', 'inspector', 'layer']
  let changed = false
  const next = { ...panelLayouts.value }
  ids.forEach((id) => {
    if (!next[id]) return
    const clamped = clampPanelLayout(id, next[id]!)
    if (JSON.stringify(clamped) !== JSON.stringify(next[id])) {
      next[id] = clamped
      changed = true
    }
  })
  if (changed) {
    panelLayouts.value = next
    persistPanelLayouts()
  }
}

const activeChatCharacter = computed(() => props.characterSnapshot.characters.find((character) => (
  character.identityId === props.characterSnapshot.activeIdentityId
  || character.isActive
)) || null)
const chatCharacterOptions = computed(() => props.characterSnapshot.characters.map((character) => ({
  value: character.identityId,
  label: character.resolvedAppearance.displayName || character.displayName || character.identityId,
})))
const chatCharacterVariantOptions = computed(() => {
  const character = activeChatCharacter.value
  if (!character) return []
  return [
    { value: '', label: '基础外观' },
    ...character.variants
      .filter((variant) => variant.enabled)
      .map((variant) => ({
        value: variant.variantId,
        label: variant.keyword || variant.appearancePatch.displayName || variant.variantId,
      })),
  ]
})

const handleChatCharacterSelect = (identityId: string) => {
  if (identityId && identityId !== props.characterSnapshot.activeIdentityId) {
    emit('selectCharacter', identityId)
  }
}

const handleChatCharacterVariantSelect = (variantId: string | null) => {
  const character = activeChatCharacter.value
  if (!character) return
  emit('selectCharacterVariant', {
    identityId: character.identityId,
    variantId: variantId || null,
  })
}

const actionId = () => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `action-${value}`
}

let stage: Konva.Stage | null = null
let backgroundLayer: Konva.Layer | null = null
let worldLayer: Konva.Layer | null = null
let foregroundLayer: Konva.Layer | null = null
let interactionLayer: Konva.Layer | null = null
let backgroundCameraGroup: Konva.Group | null = null
let worldCameraGroup: Konva.Group | null = null
let foregroundCameraGroup: Konva.Group | null = null
let gridGroup: Konva.Group | null = null
let objectRoot: Konva.Group | null = null
let transformer: Konva.Transformer | null = null
let resizeObserver: ResizeObserver | null = null
let panning = false
let panPointer = { x: 0, y: 0 }
let panOrigin = { x: 0, y: 0 }
let gridSignature = ''

const objectNodes = new Map<string, Konva.Group>()
const imageLoadVersions = new Map<string, number>()
const transformCenters = new Map<string, { x: number, y: number }>()

interface SurfaceSlot {
  group: Konva.Group
  base: Konva.Rect | null
  image: Konva.Image
  placeholder: Konva.Rect
  label: Konva.Text
  url: string
  version: number
  source: HTMLImageElement | null
}

let backgroundSlot: SurfaceSlot | null = null
let foregroundSlot: SurfaceSlot | null = null

const selectedObject = computed(() => {
  const id = props.store.state.selectedObjectId
  return id ? props.store.activeObjects.value[id] || null : null
})

const parentOptions = computed(() => Object.values(props.store.activeObjects.value)
  .filter((object) => object.type === 'group'
    && object.id !== selectedObject.value?.id
    && (!selectedObject.value
      || props.store.isPersistentObject(object.id) === props.store.isPersistentObject(selectedObject.value.id)))
  .map((object) => ({ label: object.name, value: object.id })))

interface LayerRow {
  object: StageObject
  depth: number
}

const layerRows = computed<LayerRow[]>(() => {
  const objects = Object.values(props.store.activeObjects.value)
  const rows: LayerRow[] = []
  const append = (parentId: string | null, depth: number) => {
    objects
      .filter((object) => object.parentId === parentId)
      .sort((a, b) => b.transform.z - a.transform.z || b.transform.order - a.transform.order)
      .forEach((object) => {
        rows.push({ object, depth })
        append(object.id, depth + 1)
      })
  }
  append(null, 0)
  return rows
})

const getObject = (objectId: string) => props.store.activeObjects.value[objectId]

const addAction = (type: StageAction['type']) => {
  const object = selectedObject.value
  if (!object || !canEditAllObjects.value) return
  const action: StageAction = type === 'chat.send'
    ? { id: actionId(), type, payload: { content: '舞台消息' } }
    : type === 'chat.insert'
      ? { id: actionId(), type, payload: { content: '舞台台词' } }
      : type === 'scene.apply'
        ? { id: actionId(), type, payload: { sceneId: props.store.state.activeSceneId } }
        : { id: actionId(), type, payload: { objectId: object.id } }
  props.store.addObjectAction(object.id, action)
}

const triggerObjectActions = (object: StageObject) => {
  if (!canTriggerActions.value || !['image', 'button'].includes(object.type) || !object.interactive || !object.visible) return
  const pointer = worldCameraGroup?.getRelativePointerPosition()
  object.actions.forEach((action) => {
    const parsed = stageActionSchema.safeParse(action)
    if (!parsed.success) return
    emit('actionTriggered', {
      objectId: object.id,
      actionId: parsed.data.id,
      action: parsed.data,
      ...(pointer ? {
        pointer: {
          x: Number((pointer.x / WORLD_UNIT_PX).toFixed(6)),
          y: Number((pointer.y / WORLD_UNIT_PX).toFixed(6)),
        },
      } : {}),
    })
  })
}

const applyCamera = () => {
  if (!stage) return
  const position = {
    x: stage.width() / 2 + props.store.state.camera.x,
    y: stage.height() / 2 + props.store.state.camera.y,
  }
  const scale = { x: props.store.state.camera.zoom, y: props.store.state.camera.zoom }
  for (const group of [backgroundCameraGroup, worldCameraGroup, foregroundCameraGroup]) {
    group?.position(position)
    group?.scale(scale)
  }
  backgroundLayer?.batchDraw()
  worldLayer?.batchDraw()
  foregroundLayer?.batchDraw()
  interactionLayer?.batchDraw()
}

const updateTransformer = () => {
  if (!transformer) return
  const object = selectedObject.value
  const node = object && object.type !== 'group' && !object.locked && canEditObject(object) ? objectNodes.get(object.id) : null
  transformer.nodes(node ? [node] : [])
  transformer.keepRatio(object?.type === 'image')
  transformer.enabledAnchors(object?.sizeLocked ? [] : [
    'top-left', 'top-center', 'top-right',
    'middle-left', 'middle-right',
    'bottom-left', 'bottom-center', 'bottom-right',
  ])
  transformer.rotateEnabled(true)
  interactionLayer?.batchDraw()
}

const selectObject = (objectId: string | null) => {
  if (objectId && !canEditObject(getObject(objectId))) return
  props.store.state.selectedObjectId = objectId
  nextTick(updateTransformer)
}

const setImageFit = (
  node: Konva.Image,
  source: HTMLImageElement,
  width: number,
  height: number,
  fit: StageObjectFit,
) => {
  const sourceWidth = Math.max(1, source.naturalWidth || source.width)
  const sourceHeight = Math.max(1, source.naturalHeight || source.height)
  node.image(source)
  if (fit === 'fill') {
    node.position({ x: 0, y: 0 })
    node.size({ width, height })
    node.crop({ x: 0, y: 0, width: sourceWidth, height: sourceHeight })
    return
  }
  const sourceRatio = sourceWidth / sourceHeight
  const targetRatio = width / height
  const useWidth = fit === 'cover' ? sourceRatio < targetRatio : sourceRatio > targetRatio
  const renderedWidth = useWidth ? width : height * sourceRatio
  const renderedHeight = useWidth ? width / sourceRatio : height
  node.position({ x: (width - renderedWidth) / 2, y: (height - renderedHeight) / 2 })
  node.size({ width: renderedWidth, height: renderedHeight })
  node.crop({ x: 0, y: 0, width: sourceWidth, height: sourceHeight })
}

const createSurfaceSlot = (cameraGroup: Konva.Group, withBase: boolean): SurfaceSlot => {
  const group = new Konva.Group()
  const base = withBase ? new Konva.Rect({ listening: false }) : null
  const image = new Konva.Image({ image: undefined, visible: false, listening: false })
  const placeholder = new Konva.Rect({
    visible: false,
    fill: 'rgba(15, 23, 42, 0.78)',
    stroke: 'rgba(148, 163, 184, 0.52)',
    dash: [10, 7],
    listening: false,
  })
  const label = new Konva.Text({
    visible: false,
    align: 'center',
    verticalAlign: 'middle',
    fill: '#cbd5e1',
    fontSize: 18,
    listening: false,
  })
  cameraGroup.add(group)
  if (base) group.add(base)
  group.add(image, placeholder, label)
  return { group, base, image, placeholder, label, url: '', version: 0, source: null }
}

const updateSurfaceSlot = (
  slot: SurfaceSlot,
  imageRef: StageImageRef | null,
  box: { x: number, y: number, width: number, height: number },
  fit: StageObjectFit,
  loadingLabel: string,
) => {
  slot.group.position({ x: box.x, y: box.y })
  slot.group.clip({ x: 0, y: 0, width: box.width, height: box.height })
  slot.base?.setAttrs({ width: box.width, height: box.height, fill: props.store.state.liveState.backgroundColor })
  slot.placeholder.setAttrs({ width: box.width, height: box.height })
  slot.label.setAttrs({ width: box.width, height: box.height })
  if (slot.source) setImageFit(slot.image, slot.source, box.width, box.height, fit)

  const resolved = imageRef ? resolveStageImageUrl(imageRef.url) : null
  if (!imageRef) {
    slot.url = ''
    slot.source = null
    slot.image.visible(false)
    slot.placeholder.visible(false)
    slot.label.visible(false)
    return
  }
  if (!resolved) {
    slot.url = imageRef.url
    slot.source = null
    slot.image.visible(false)
    slot.placeholder.visible(true)
    slot.label.text('图片地址被安全策略拒绝').visible(true)
    return
  }
  if (slot.url === resolved) return
  slot.url = resolved
  slot.source = null
  slot.version += 1
  const version = slot.version
  slot.image.visible(false)
  slot.placeholder.visible(true)
  slot.label.text(`${loadingLabel}加载中…`).visible(true)
  const source = new Image()
  source.onload = () => {
    if (slot.version !== version || slot.url !== resolved) return
    slot.source = source
    slot.image.image(source)
    setImageFit(
      slot.image,
      source,
      slot.placeholder.width(),
      slot.placeholder.height(),
      props.store.state.liveState.fieldObjectFit,
    )
    slot.image.visible(true)
    slot.placeholder.visible(false)
    slot.label.visible(false)
    slot.group.getLayer()?.batchDraw()
  }
  source.onerror = () => {
    if (slot.version !== version || slot.url !== resolved) return
    slot.image.visible(false)
    slot.placeholder.visible(true)
    slot.label.text(`${loadingLabel}加载失败`).visible(true)
    slot.group.getLayer()?.batchDraw()
  }
  source.src = resolved
}

const rebuildGrid = (fieldX: number, fieldY: number, fieldWidth: number, fieldHeight: number) => {
  if (!gridGroup) return
  const liveState = props.store.state.liveState
  const signature = [fieldWidth, fieldHeight, liveState.displayGrid, liveState.gridSize].join(':')
  if (signature === gridSignature) return
  gridSignature = signature
  gridGroup.destroyChildren()
  if (!liveState.displayGrid) return
  const step = Math.max(0.25, liveState.gridSize) * WORLD_UNIT_PX
  for (let x = fieldX; x <= fieldX + fieldWidth; x += step) {
    gridGroup.add(new Konva.Line({
      points: [x, fieldY, x, fieldY + fieldHeight],
      stroke: 'rgba(148, 163, 184, 0.12)',
      strokeWidth: 1,
      listening: false,
    }))
  }
  for (let y = fieldY; y <= fieldY + fieldHeight; y += step) {
    gridGroup.add(new Konva.Line({
      points: [fieldX, y, fieldX + fieldWidth, y],
      stroke: 'rgba(148, 163, 184, 0.12)',
      strokeWidth: 1,
      listening: false,
    }))
  }
}

const syncField = () => {
  if (!backgroundSlot || !foregroundSlot) return
  const liveState = props.store.state.liveState
  const width = liveState.fieldWidth * WORLD_UNIT_PX
  const height = liveState.fieldHeight * WORLD_UNIT_PX
  const box = { x: -width / 2, y: -height / 2, width, height }
  updateSurfaceSlot(backgroundSlot, liveState.background, box, liveState.fieldObjectFit, '背景')
  updateSurfaceSlot(foregroundSlot, liveState.foreground, box, liveState.fieldObjectFit, '前景')
  rebuildGrid(box.x, box.y, width, height)
  backgroundLayer?.batchDraw()
  worldLayer?.batchDraw()
  foregroundLayer?.batchDraw()
}

const rebuildObjectContent = (wrapper: Konva.Group, object: StageObject) => {
  if (wrapper.getAttr('stageObjectType') && wrapper.getAttr('stageObjectType') !== object.type) {
    imageLoadVersions.set(object.id, (imageLoadVersions.get(object.id) || 0) + 1)
    wrapper.setAttr('stageImageUrl', '')
  }
  wrapper.destroyChildren()
  wrapper.setAttr('stageObjectType', object.type)
  const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
  const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
  if (object.type === 'text') {
    wrapper.add(new Konva.Text({
      name: 'theater-object-content',
      text: object.text || object.name,
      width,
      height,
      fontSize: 28,
      fontStyle: 'bold',
      fill: object.fill,
      padding: 10,
      verticalAlign: 'middle',
    }))
    return
  }
  if (object.type === 'image') {
    wrapper.add(
      new Konva.Rect({
        name: 'theater-object-image-frame',
        width,
        height,
        listening: false,
      }),
      new Konva.Image({ image: undefined, name: 'theater-object-image', visible: false }),
      new Konva.Rect({
        name: 'theater-object-image-placeholder',
        width,
        height,
        fill: 'rgba(15, 23, 42, 0.82)',
        stroke: 'rgba(148, 163, 184, 0.62)',
        dash: [8, 6],
      }),
      new Konva.Text({
        name: 'theater-object-image-label',
        width,
        height,
        text: '未设置图片',
        align: 'center',
        verticalAlign: 'middle',
        fill: '#cbd5e1',
        fontSize: 14,
        padding: 10,
      }),
    )
    return
  }
  if (object.type === 'button') {
    wrapper.add(
      new Konva.Rect({
        name: 'theater-object-content',
        width,
        height,
        fill: object.fill,
        stroke: 'rgba(255, 255, 255, 0.7)',
        strokeWidth: 1,
        cornerRadius: 12,
        shadowColor: '#000000',
        shadowBlur: 18,
        shadowOpacity: 0.28,
      }),
      new Konva.Text({
        name: 'theater-object-button-label',
        text: object.text || object.name,
        width,
        height,
        align: 'center',
        verticalAlign: 'middle',
        fill: '#ffffff',
        fontSize: 20,
        fontStyle: 'bold',
        padding: 8,
      }),
    )
    return
  }
  wrapper.add(new Konva.Rect({
    name: 'theater-object-content',
    width,
    height,
    fill: object.fill,
    stroke: object.type === 'group' ? 'rgba(148, 163, 184, 0.8)' : 'rgba(255, 255, 255, 0.58)',
    dash: object.type === 'group' ? [10, 7] : undefined,
    strokeWidth: object.type === 'group' ? 2 : 1,
    cornerRadius: object.type === 'group' ? 6 : 14,
    shadowColor: object.type === 'group' ? undefined : '#000000',
    shadowBlur: object.type === 'group' ? 0 : 18,
    shadowOpacity: object.type === 'group' ? 0 : 0.28,
  }))
  if (object.type === 'group') {
    wrapper.add(new Konva.Text({
      name: 'theater-object-group-label',
      text: object.name,
      x: 8,
      y: 7,
      fill: '#cbd5e1',
      fontSize: 13,
      listening: false,
    }))
  }
}

const createObjectNode = (object: StageObject) => {
  const wrapper = new Konva.Group({ id: `theater-object-${object.id}` })
  wrapper.setAttr('stageObjectId', object.id)
  rebuildObjectContent(wrapper, object)
  wrapper.on('pointerdown', (event) => {
    const current = getObject(object.id)
    if (!canEditObject(current)) return
    event.cancelBubble = true
    selectObject(object.id)
  })
  wrapper.on('click tap', () => {
    const current = getObject(object.id)
    if (current) triggerObjectActions(current)
  })
  wrapper.on('contextmenu', (event) => {
    if (!canEditObject(getObject(object.id))) return
    event.evt.preventDefault()
    event.cancelBubble = true
    selectObject(object.id)
  })
  wrapper.on('dragstart', () => {
    if (canEditObject(getObject(object.id))) props.store.beginObjectEdit('移动对象')
  })
  wrapper.on('dragend', () => {
    const current = getObject(object.id)
    if (!canEditObject(current)) {
      props.store.cancelObjectEdit()
      return
    }
    current.transform.x = Number((wrapper.x() / WORLD_UNIT_PX).toFixed(6))
    current.transform.y = Number((wrapper.y() / WORLD_UNIT_PX).toFixed(6))
    props.store.commitObjectEdit()
  })
  wrapper.on('transformstart', () => {
    if (!canEditObject(getObject(object.id))) return
    props.store.beginObjectEdit('变换对象')
    transformCenters.set(object.id, { x: wrapper.x(), y: wrapper.y() })
  })
  wrapper.on('transformend', () => {
    const current = getObject(object.id)
    if (!canEditObject(current)) {
      props.store.cancelObjectEdit()
      return
    }
    const center = transformCenters.get(object.id) || { x: wrapper.x(), y: wrapper.y() }
    transformCenters.delete(object.id)
    current.transform.width = Number((Math.max(12, current.transform.width * WORLD_UNIT_PX * wrapper.scaleX()) / WORLD_UNIT_PX).toFixed(6))
    current.transform.height = Number((Math.max(12, current.transform.height * WORLD_UNIT_PX * wrapper.scaleY()) / WORLD_UNIT_PX).toFixed(6))
    current.transform.rotation = Number(wrapper.rotation().toFixed(6))
    current.transform.x = Number((center.x / WORLD_UNIT_PX).toFixed(6))
    current.transform.y = Number((center.y / WORLD_UNIT_PX).toFixed(6))
    wrapper.position(center)
    wrapper.scale({ x: 1, y: 1 })
    props.store.commitObjectEdit()
  })
  objectNodes.set(object.id, wrapper)
  return wrapper
}

const syncObjectImage = (wrapper: Konva.Group, object: StageObject, width: number, height: number) => {
  const frame = wrapper.findOne<Konva.Rect>('.theater-object-image-frame')
  const image = wrapper.findOne<Konva.Image>('.theater-object-image')
  const placeholder = wrapper.findOne<Konva.Rect>('.theater-object-image-placeholder')
  const label = wrapper.findOne<Konva.Text>('.theater-object-image-label')
  frame?.size({ width, height })
  placeholder?.size({ width, height })
  label?.size({ width, height })
  if (!image || !placeholder || !label) return
  const resolved = object.image ? resolveStageImageUrl(object.image.url) : null
  if (!object.image) {
    wrapper.setAttr('stageImageUrl', '')
    image.visible(false)
    placeholder.visible(true)
    label.text('未设置图片').visible(true)
    return
  }
  if (!resolved) {
    wrapper.setAttr('stageImageUrl', object.image.url)
    image.visible(false)
    placeholder.visible(true)
    label.text('图片地址被安全策略拒绝').visible(true)
    return
  }
  const currentSource = image.image() as HTMLImageElement | undefined
  if (wrapper.getAttr('stageImageUrl') === resolved && currentSource) {
    setImageFit(image, currentSource, width, height, 'contain')
    return
  }
  if (wrapper.getAttr('stageImageUrl') === resolved) return
  wrapper.setAttr('stageImageUrl', resolved)
  const version = (imageLoadVersions.get(object.id) || 0) + 1
  imageLoadVersions.set(object.id, version)
  image.visible(false)
  placeholder.visible(true)
  label.text('图片加载中…').visible(true)
  const source = new Image()
  source.onload = () => {
    if (imageLoadVersions.get(object.id) !== version || wrapper.getAttr('stageImageUrl') !== resolved) return
    const sourceWidth = Math.max(1, source.naturalWidth || source.width)
    const sourceHeight = Math.max(1, source.naturalHeight || source.height)
    object.transform.width = Number((Math.max(0.5, object.transform.height * sourceWidth / sourceHeight)).toFixed(6))
    image.image(source)
    setImageFit(
      image,
      source,
      frame?.width() || width,
      frame?.height() || height,
      'contain',
    )
    image.visible(true)
    placeholder.visible(false)
    label.visible(false)
    wrapper.getLayer()?.batchDraw()
  }
  source.onerror = () => {
    if (imageLoadVersions.get(object.id) !== version || wrapper.getAttr('stageImageUrl') !== resolved) return
    image.visible(false)
    placeholder.visible(true)
    label.text('图片加载失败').visible(true)
    wrapper.getLayer()?.batchDraw()
  }
  source.src = resolved
}

const updateObjectNode = (wrapper: Konva.Group, object: StageObject) => {
  if (wrapper.getAttr('stageObjectType') !== object.type) rebuildObjectContent(wrapper, object)
  const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
  const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
  wrapper.setAttrs({
    x: object.transform.x * WORLD_UNIT_PX,
    y: object.transform.y * WORLD_UNIT_PX,
    offsetX: width / 2,
    offsetY: height / 2,
    rotation: object.transform.rotation,
    visible: object.visible,
    draggable: !object.locked && canEditObject(object),
    listening: canEditObject(object) || (canTriggerActions.value && object.interactive && ['image', 'button'].includes(object.type)),
  })
  if (object.type === 'text') {
    wrapper.findOne<Konva.Text>('.theater-object-content')?.setAttrs({
      text: object.text || object.name,
      width,
      height,
      fill: object.fill,
    })
  } else if (object.type === 'image') {
    syncObjectImage(wrapper, object, width, height)
  } else if (object.type === 'button') {
    wrapper.findOne<Konva.Rect>('.theater-object-content')?.setAttrs({ width, height, fill: object.fill })
    wrapper.findOne<Konva.Text>('.theater-object-button-label')?.setAttrs({
      text: object.text || object.name,
      width,
      height,
    })
  } else {
    wrapper.findOne<Konva.Rect>('.theater-object-content')?.setAttrs({ width, height, fill: object.fill })
    wrapper.findOne<Konva.Text>('.theater-object-group-label')?.text(object.name)
  }
}

const syncObjects = () => {
  if (!objectRoot) return
  const objects = props.store.activeObjects.value
  for (const [objectId, node] of objectNodes) {
    if (objects[objectId]) continue
    imageLoadVersions.delete(objectId)
    node.destroy()
    objectNodes.delete(objectId)
  }
  for (const object of Object.values(objects)) {
    const node = objectNodes.get(object.id) || createObjectNode(object)
    updateObjectNode(node, object)
  }
  syncStageObjectHierarchy(objects, objectNodes, objectRoot)
  worldLayer?.batchDraw()
  nextTick(updateTransformer)
}

const resizeStage = () => {
  const element = viewportRef.value
  if (!stage || !element) return
  const rect = element.getBoundingClientRect()
  stage.size({ width: Math.max(1, rect.width), height: Math.max(1, rect.height) })
  clampOpenPanels()
  applyCamera()
}

const handleWheel = (event: Konva.KonvaEventObject<WheelEvent>) => {
  if (!stage || !worldCameraGroup) return
  event.evt.preventDefault()
  const pointer = stage.getPointerPosition()
  if (!pointer) return
  const oldZoom = props.store.state.camera.zoom
  const worldPoint = {
    x: (pointer.x - worldCameraGroup.x()) / oldZoom,
    y: (pointer.y - worldCameraGroup.y()) / oldZoom,
  }
  const direction = event.evt.deltaY > 0 ? -1 : 1
  const zoom = Math.min(3, Math.max(0.2, direction > 0 ? oldZoom * 1.08 : oldZoom / 1.08))
  props.store.state.camera.zoom = zoom
  props.store.state.camera.x = pointer.x - stage.width() / 2 - worldPoint.x * zoom
  props.store.state.camera.y = pointer.y - stage.height() / 2 - worldPoint.y * zoom
}

const startPan = (event: Konva.KonvaEventObject<PointerEvent>) => {
  if (!stage) return
  if (event.evt.button !== 0 || event.target !== stage) return
  event.evt.preventDefault()
  selectObject(null)
  panning = true
  panPointer = { x: event.evt.clientX, y: event.evt.clientY }
  panOrigin = { x: props.store.state.camera.x, y: props.store.state.camera.y }
}

const movePan = (event: Konva.KonvaEventObject<PointerEvent>) => {
  if (!panning) return
  props.store.state.camera.x = panOrigin.x + event.evt.clientX - panPointer.x
  props.store.state.camera.y = panOrigin.y + event.evt.clientY - panPointer.y
}

const stopPan = () => { panning = false }

const targetImageUrl = (target: ImageTarget) => target.kind === 'scene'
  ? props.store.state.liveState[target.target]?.url || ''
  : props.store.activeObjects.value[target.objectId]?.image?.url || ''

const applyImageUrl = (target: ImageTarget, url: string, resourceId?: string) => {
  if (target.kind === 'scene') return props.store.setSceneImage(target.target, url, resourceId)
  return props.store.setObjectImage(target.objectId, url, resourceId)
}

const theaterResourcePath = (resourceId = '') => {
  const base = `api/v1/worlds/${encodeURIComponent(props.worldId)}/channels/${encodeURIComponent(props.channelId)}/theater/resources`
  return resourceId ? `${base}/${encodeURIComponent(resourceId)}` : base
}

const waitForResource = async (resourceId: string) => {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    const response = await api.get<TheaterResourceResponse>(theaterResourcePath(resourceId))
    const status = response.data?.resource?.status
    if (status === 'ready') return
    if (status === 'failed') {
      throw new Error(response.data?.resource?.processing?.errorCode || '图片处理失败')
    }
    await new Promise((resolve) => window.setTimeout(resolve, 200))
  }
  throw new Error('图片处理超时')
}

const uploadImage = async (file: File, target: ImageTarget) => {
  if (!canEditAllObjects.value || !canUploadResources.value) throw new Error('缺少小剧场资源编辑权限')
  if (!props.worldId || !props.channelId) throw new Error('缺少小剧场频道信息')
  resourceUploading.value = true
  resourceError.value = ''
  try {
    const compressed = await compressImage(file, { mimeType: 'image/webp' })
    if (compressed.type !== 'image/webp') throw new Error('图片无法转换为 WebP')
    const formData = new FormData()
    formData.append('file', compressed)
    formData.append('mediaKind', 'image')
    formData.append('clientResourceId', crypto.randomUUID?.() || `image-${Date.now()}-${Math.random().toString(16).slice(2)}`)
    const response = await api.post<TheaterResourceResponse>(theaterResourcePath(), formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    const resourceId = response.data?.resource?.id
    if (!resourceId) throw new Error('上传响应缺少资源 ID')
    if (response.data?.resource?.status !== 'ready') await waitForResource(resourceId)
    const url = `${urlBase}/${theaterResourcePath(resourceId)}/variants/original/content`
    if (!applyImageUrl(target, url, resourceId)) throw new Error('图片目标已失效')
  } catch (error) {
    resourceError.value = error instanceof Error ? error.message : '图片上传失败'
    throw error
  } finally {
    resourceUploading.value = false
  }
}

const requestImageUpload = (target: ImageTarget) => {
  pendingImageTarget.value = target
  imageInputRef.value?.click()
}

const handleImageInput = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const target = pendingImageTarget.value
  input.value = ''
  pendingImageTarget.value = null
  if (!file || !target) return
  try {
    await uploadImage(file, target)
  } catch {
    // Error shown in inspector.
  }
}

const clearImage = (target: ImageTarget) => {
  if (!canEditAllObjects.value) return
  applyImageUrl(target, '')
  resourceError.value = ''
}

const openImageEditor = async (target: ImageTarget) => {
  const url = targetImageUrl(target)
  if (!url) return
  resourceUploading.value = true
  resourceError.value = ''
  try {
    const response = await api.get<Blob>(url, { responseType: 'blob' })
    const blob = response.data
    imageEditorFile.value = new File([blob], 'theater-image.webp', { type: blob.type || 'image/webp' })
    imageEditorTarget.value = target
    imageEditorVisible.value = true
  } catch (error) {
    resourceError.value = error instanceof Error ? error.message : '图片读取失败'
  } finally {
    resourceUploading.value = false
  }
}

const closeImageEditor = () => {
  imageEditorVisible.value = false
  imageEditorFile.value = null
  imageEditorTarget.value = null
}

const saveEditedImage = async (file: File) => {
  const target = imageEditorTarget.value
  if (!target) return
  imageEditorVisible.value = false
  try {
    await uploadImage(file, target)
    closeImageEditor()
  } catch {
    imageEditorVisible.value = true
  }
}

const handleCanvasDrop = async (event: DragEvent) => {
  if (!canEditAllObjects.value || !canUploadResources.value) return
  const file = Array.from(event.dataTransfer?.files || []).find((item) => item.type.startsWith('image/'))
  const rect = viewportRef.value?.getBoundingClientRect()
  if (!file || !rect) return
  const object = props.store.addObject('image')
  object.transform.x = (event.clientX - rect.left - rect.width / 2 - props.store.state.camera.x) / props.store.state.camera.zoom / WORLD_UNIT_PX
  object.transform.y = (event.clientY - rect.top - rect.height / 2 - props.store.state.camera.y) / props.store.state.camera.zoom / WORLD_UNIT_PX
  try {
    await uploadImage(file, { kind: 'object', objectId: object.id })
  } catch {
    props.store.removeSelectedObject(false)
  }
}

const handleLayerDrop = (event: DragEvent, targetId: string) => {
  if (!canEditAllObjects.value) return
  const objectId = draggedLayerId.value
  draggedLayerId.value = null
  if (!objectId || objectId === targetId) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  props.store.reorderObject(objectId, targetId, event.clientY < rect.top + rect.height / 2 ? 'before' : 'after')
}

onMounted(() => {
  if (!containerRef.value) return
  panelResizeObserver = new ResizeObserver((entries) => {
    entries.forEach((entry) => {
      const element = entry.target as HTMLElement
      const id = element.dataset.panelId as PanelId | undefined
      if (!id) return
      const current = panelLayouts.value[id] || panelDefaultLayout(id)
      const next = clampPanelLayout(id, { ...current, width: element.offsetWidth, height: element.offsetHeight })
      if (JSON.stringify(next) !== JSON.stringify(current)) {
        panelLayouts.value = { ...panelLayouts.value, [id]: next }
        persistPanelLayouts()
      }
    })
  })
  stage = new Konva.Stage({ container: containerRef.value, width: 1, height: 1 })
  backgroundLayer = new Konva.Layer({ listening: false })
  worldLayer = new Konva.Layer()
  foregroundLayer = new Konva.Layer({ listening: false })
  interactionLayer = new Konva.Layer()
  backgroundCameraGroup = new Konva.Group()
  worldCameraGroup = new Konva.Group()
  foregroundCameraGroup = new Konva.Group()
  gridGroup = new Konva.Group({ listening: false })
  objectRoot = new Konva.Group()
  transformer = new Konva.Transformer({
    rotateEnabled: true,
    keepRatio: false,
    centeredScaling: true,
    flipEnabled: false,
    borderStroke: '#38bdf8',
    anchorStroke: '#38bdf8',
    anchorFill: '#0f172a',
    anchorSize: 9,
  })
  backgroundSlot = createSurfaceSlot(backgroundCameraGroup, true)
  foregroundSlot = createSurfaceSlot(foregroundCameraGroup, false)
  worldCameraGroup.add(gridGroup, objectRoot)
  backgroundLayer.add(backgroundCameraGroup)
  worldLayer.add(worldCameraGroup)
  foregroundLayer.add(foregroundCameraGroup)
  interactionLayer.add(transformer)
  stage.add(backgroundLayer, worldLayer, foregroundLayer, interactionLayer)
  stage.on('wheel', handleWheel)
  stage.on('pointerdown', startPan)
  stage.on('pointermove', movePan)
  stage.on('pointerup pointercancel', stopPan)
  stage.on('contextmenu', (event) => event.evt.preventDefault())
  resizeObserver = new ResizeObserver(resizeStage)
  resizeObserver.observe(viewportRef.value!)
  resizeStage()
  syncField()
  syncObjects()
  window.addEventListener('pointermove', movePanel)
  window.addEventListener('pointerup', stopPanelDrag)
  window.addEventListener('pointercancel', stopPanelDrag)
  window.addEventListener('keydown', handleStageShortcut)
})

watch(() => props.store.state.liveState, () => {
  syncField()
  syncObjects()
}, { deep: true })
watch(() => props.store.state.persistentObjects, syncObjects, { deep: true })
watch(() => props.store.state.camera, applyCamera, { deep: true })
watch(() => [props.syncReady, ...props.permissions], () => {
  const object = selectedObject.value
  if (object && !canEditObject(object)) props.store.state.selectedObjectId = null
  syncObjects()
})
watch(() => props.store.state.selectedObjectId, () => {
  resourceError.value = ''
  if (props.store.state.selectedObjectId) inspectorPanelOpen.value = true
  updateTransformer()
})
watch([scenePanelOpen, inspectorPanelOpen, layerPanelOpen], async (open) => {
  await nextTick()
  const ids: PanelId[] = ['scene', 'inspector', 'layer']
  open.forEach((isOpen, index) => {
    if (isOpen) ensurePanelLayout(ids[index])
  })
  observeOpenPanels()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  panelResizeObserver?.disconnect()
  window.removeEventListener('pointermove', movePanel)
  window.removeEventListener('pointerup', stopPanelDrag)
  window.removeEventListener('pointercancel', stopPanelDrag)
  window.removeEventListener('keydown', handleStageShortcut)
  props.store.commitObjectEdit()
  objectNodes.clear()
  imageLoadVersions.clear()
  stage?.destroy()
  stage = null
})
</script>

<template>
  <section class="theater-stage-app">
    <input ref="imageInputRef" class="theater-image-input" type="file" accept="image/*" @change="handleImageInput">
    <header class="theater-stage-toolbar">
      <n-tooltip trigger="hover">
        <template #trigger>
          <n-button class="theater-toolbar-exit" quaternary size="small" aria-label="退出小剧场" @click="emit('exitTheater')">
            <template #icon><n-icon><ArrowLeft /></n-icon></template>
          </n-button>
        </template>
        退出小剧场
      </n-tooltip>
      <div class="theater-stage-title" :title="store.activeScene.value.name">
        {{ store.activeScene.value.name }}
      </div>
      <n-button-group class="theater-panel-switches" size="small">
        <n-tooltip v-if="canEditAllObjects || canSwitchScene" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': scenePanelOpen }" aria-label="切换场景面板" @click="togglePanel('scene')">
              <template #icon><n-icon><LayoutSidebarLeftExpand /></n-icon></template>
            </n-button>
          </template>
          场景
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects || canEditDelegatedObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': inspectorPanelOpen }" aria-label="切换组件编辑面板" @click="togglePanel('inspector')">
              <template #icon><n-icon><Components /></n-icon></template>
            </n-button>
          </template>
          组件编辑
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects || canEditDelegatedObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': layerPanelOpen }" aria-label="切换图层与属性面板" @click="togglePanel('layer')">
              <template #icon><n-icon><Stack2 /></n-icon></template>
            </n-button>
          </template>
          图层与属性
        </n-tooltip>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': chatVisible }" aria-label="切换聊天区" @click="emit('toggleChat')">
              <template #icon><n-icon><Message /></n-icon></template>
            </n-button>
          </template>
          {{ chatVisible ? '隐藏聊天' : '显示聊天' }}
        </n-tooltip>
      </n-button-group>
      <span
        class="theater-sync-status"
        :class="{ 'is-online': syncReady && !syncing, 'is-syncing': syncing }"
        :title="syncing ? '正在同步' : syncReady ? '后端同步已连接' : '后端同步未连接'"
      />
      <span v-if="canEditAllObjects" class="theater-toolbar-divider" />
      <n-button-group v-if="canEditAllObjects" class="theater-stage-object-actions" size="small">
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('shape')"><template #icon><n-icon><Rectangle /></n-icon></template></n-button></template>添加面板</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('text')"><template #icon><n-icon><LetterT /></n-icon></template></n-button></template>添加文字</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('image')"><template #icon><n-icon><Photo /></n-icon></template></n-button></template>添加图片面板</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('image', true)"><template #icon><n-icon><Pin /></n-icon></template></n-button></template>添加持久图片</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('button')"><template #icon><n-icon><Bolt /></n-icon></template></n-button></template>添加动作按钮</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('group')"><template #icon><n-icon><FolderPlus /></n-icon></template></n-button></template>添加组</n-tooltip>
      </n-button-group>
      <span v-if="canEditAllObjects" class="theater-toolbar-divider" />
      <n-button-group v-if="canEditAllObjects" class="theater-stage-object-actions" size="small">
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canCopy.value" aria-label="复制组件" @click="store.copySelectedObject"><template #icon><n-icon><Copy /></n-icon></template></n-button></template>复制组件 Ctrl+C</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canCut.value" aria-label="剪切组件" @click="store.cutSelectedObject"><template #icon><n-icon><Cut /></n-icon></template></n-button></template>剪切组件 Ctrl+X</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canPaste.value" aria-label="粘贴组件" @click="store.pasteObject"><template #icon><n-icon><Clipboard /></n-icon></template></n-button></template>粘贴组件 Ctrl+V</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canUndo.value" aria-label="撤回组件编辑" @click="store.undo"><template #icon><n-icon><ArrowBackUp /></n-icon></template></n-button></template>撤回 Ctrl+Z</n-tooltip>
      </n-button-group>
      <div class="theater-stage-character-bridge" :class="{ 'is-offline': !chatBridgeOnline }">
        <img
          v-if="activeChatCharacter?.resolvedAppearance.avatar"
          :src="activeChatCharacter.resolvedAppearance.avatar.url"
          :alt="activeChatCharacter.resolvedAppearance.displayName"
        >
        <span v-else class="theater-stage-character-bridge__placeholder">角</span>
        <div class="theater-stage-character-bridge__selects">
          <n-select
            :value="characterSnapshot.activeIdentityId"
            :options="chatCharacterOptions"
            :disabled="!chatBridgeOnline || !chatCharacterOptions.length"
            size="tiny"
            placeholder="聊天角色"
            @update:value="handleChatCharacterSelect"
          />
          <n-select
            :value="activeChatCharacter?.activeVariantId || ''"
            :options="chatCharacterVariantOptions"
            :disabled="!chatBridgeOnline || !activeChatCharacter"
            size="tiny"
            placeholder="头像差分"
            @update:value="handleChatCharacterVariantSelect"
          />
        </div>
        <small v-if="activeChatCharacter?.resolvedAppearance.decorations.length">
          佩饰 {{ activeChatCharacter.resolvedAppearance.decorations.length }} 层
        </small>
      </div>
      <n-button class="theater-stage-reset-camera" size="small" quaternary @click="store.resetCamera">
        <template #icon><n-icon><Focus /></n-icon></template>
        复位视角
      </n-button>
      <n-tooltip trigger="hover">
        <template #trigger>
          <n-button quaternary size="small" aria-label="恢复默认布局" @click="resetWorkspaceLayout">
            <template #icon><n-icon><Refresh /></n-icon></template>
          </n-button>
        </template>
        恢复默认布局
      </n-tooltip>
      <span class="theater-stage-zoom">{{ Math.round(store.state.camera.zoom * 100) }}%</span>
    </header>

    <div ref="workspaceRef" class="theater-stage-workspace">
      <div ref="viewportRef" class="theater-stage-viewport" @dragover.prevent @drop.prevent="handleCanvasDrop">
        <div ref="containerRef" class="theater-stage-canvas" />
      </div>

      <aside v-if="scenePanelOpen" class="theater-floating-panel theater-scene-rail" data-panel-id="scene" :style="panelStyle('scene')">
        <div class="theater-panel-heading" @pointerdown="startPanelDrag('scene', $event)">
          <span>场景</span>
          <div class="theater-panel-heading__actions">
            <n-button v-if="canEditAllObjects && canSwitchScene" text size="tiny" aria-label="新建场景" @click="store.addScene"><n-icon><Plus /></n-icon></n-button>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭场景面板" @click="scenePanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <button
          v-for="scene in store.scenes.value"
          :key="scene.id"
          class="theater-scene-card"
          :class="{ 'is-active': scene.id === store.state.activeSceneId }"
          :disabled="!canSwitchScene"
          @click="canSwitchScene && store.selectScene(scene.id)"
        >
          <span class="theater-scene-card__title">{{ scene.name }}</span>
        </button>
        <div v-if="canEditAllObjects && canSwitchScene" class="theater-scene-actions">
          <n-button size="tiny" quaternary @click="store.duplicateScene"><template #icon><n-icon><Copy /></n-icon></template>复制</n-button>
          <n-button size="tiny" quaternary :disabled="store.scenes.value.length <= 1" @click="store.removeScene"><template #icon><n-icon><Trash /></n-icon></template>删除</n-button>
        </div>
      </aside>

      <aside v-if="inspectorPanelOpen" class="theater-floating-panel theater-object-inspector" data-panel-id="inspector" :style="panelStyle('inspector')">
        <template v-if="selectedObject">
          <div class="theater-panel-heading" @pointerdown="startPanelDrag('inspector', $event)">
            <span>组件编辑</span>
            <div class="theater-panel-heading__actions">
              <small>{{ selectedObject.type }}</small>
              <n-button class="theater-panel-close" text size="tiny" aria-label="关闭组件编辑面板" @click="inspectorPanelOpen = false"><n-icon><X /></n-icon></n-button>
            </div>
          </div>
          <div
            class="theater-inspector"
            @focusin="store.beginObjectEdit('修改对象')"
            @focusout="store.commitObjectEdit"
          >
            <label>名称</label>
            <n-input v-model:value="selectedObject.name" size="small" />
            <template v-if="selectedObject.type === 'text' || selectedObject.type === 'button'">
              <label>内容</label>
              <n-input v-model:value="selectedObject.text" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" />
            </template>
            <template v-if="selectedObject.type === 'image' && canEditAllObjects">
              <label>图片</label>
              <div class="theater-image-actions">
                <n-button size="small" :disabled="!canUploadResources" :loading="resourceUploading" @click="requestImageUpload({ kind: 'object', objectId: selectedObject.id })">
                  <template #icon><n-icon><Photo /></n-icon></template>上传替换
                </n-button>
                <n-tooltip trigger="hover">
                  <template #trigger>
                    <n-button size="small" :disabled="!canUploadResources || !selectedObject.image || resourceUploading" aria-label="编辑图片" @click="openImageEditor({ kind: 'object', objectId: selectedObject.id })">
                      <template #icon><n-icon><Edit /></n-icon></template>
                    </n-button>
                  </template>
                  编辑图片
                </n-tooltip>
                <n-button size="small" quaternary type="error" :disabled="!selectedObject.image" @click="clearImage({ kind: 'object', objectId: selectedObject.id })">清除</n-button>
              </div>
            </template>
            <template v-if="selectedObject.type !== 'image'">
              <label>颜色</label>
              <n-input v-model:value="selectedObject.fill" />
            </template>
            <div class="theater-object-editor__transform">
              <label>X</label><n-input-number v-model:value="selectedObject.transform.x" :precision="2" />
              <label>Y</label><n-input-number v-model:value="selectedObject.transform.y" :precision="2" />
              <label>宽</label><n-input-number v-model:value="selectedObject.transform.width" :disabled="!canEditAllObjects && selectedObject.sizeLocked" :min="0.5" :precision="2" />
              <label>高</label><n-input-number v-model:value="selectedObject.transform.height" :disabled="!canEditAllObjects && selectedObject.sizeLocked" :min="0.5" :precision="2" />
            </div>
            <div v-if="canEditAllObjects" class="theater-object-editor__checks">
              <n-checkbox v-model:checked="selectedObject.visible">显示</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.interactive">可交互</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.editable">可编辑</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.locked">锁定位置</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.sizeLocked">锁定尺寸</n-checkbox>
            </div>
            <template v-if="canEditAllObjects && (selectedObject.type === 'image' || selectedObject.type === 'button')">
              <label>点击动作</label>
              <div class="theater-action-add">
                <n-button size="tiny" @click="addAction('chat.send')">发送</n-button>
                <n-button size="tiny" @click="addAction('chat.insert')">插入</n-button>
                <n-button size="tiny" @click="addAction('scene.apply')">场景</n-button>
                <n-button size="tiny" @click="addAction('object.toggle')">显隐</n-button>
              </div>
              <div v-for="action in selectedObject.actions" :key="action.id" class="theater-action-row">
                <small>{{ action.type }}</small>
                <n-input v-if="action.type === 'chat.send' || action.type === 'chat.insert'" v-model:value="action.payload.content" size="tiny" maxlength="10000" />
                <n-select v-else-if="action.type === 'scene.apply'" v-model:value="action.payload.sceneId" :options="store.scenes.value.map((scene) => ({ label: scene.name, value: scene.id }))" size="tiny" />
                <n-select v-else v-model:value="action.payload.objectId" :options="Object.values(store.activeObjects.value).map((item) => ({ label: item.name, value: item.id }))" size="tiny" />
                <n-button text type="error" size="tiny" @click="store.removeObjectAction(selectedObject.id, action.id)">删除</n-button>
              </div>
            </template>
            <template v-if="canEditAllObjects">
              <label>父级</label>
              <n-select
                :value="selectedObject.parentId"
                :options="parentOptions"
                size="small"
                clearable
                placeholder="根层级"
                @update:value="store.setParent(selectedObject.id, $event || null)"
              />
              <div class="theater-inspector-actions">
                <n-button size="tiny" @click="store.moveOrder(selectedObject.id, 1)"><template #icon><n-icon><ArrowUp /></n-icon></template>上移</n-button>
                <n-button size="tiny" @click="store.moveOrder(selectedObject.id, -1)"><template #icon><n-icon><ArrowDown /></n-icon></template>下移</n-button>
                <n-button size="tiny" :disabled="!selectedObject.parentId" @click="store.setParent(selectedObject.id, null)"><template #icon><n-icon><ArrowBackUp /></n-icon></template>移出组</n-button>
              </div>
              <small v-if="resourceError" class="theater-resource-error">{{ resourceError }}</small>
              <n-button size="small" secondary type="error" @click="store.removeSelectedObject"><template #icon><n-icon><Trash /></n-icon></template>删除对象</n-button>
            </template>
          </div>
        </template>
        <template v-else>
          <div class="theater-panel-heading" @pointerdown="startPanelDrag('inspector', $event)">
            <span>组件编辑</span>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭组件编辑面板" @click="inspectorPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
          <div class="theater-panel-empty">选择幕布或图层中的组件</div>
        </template>
      </aside>

      <aside v-if="layerPanelOpen" class="theater-floating-panel theater-layer-panel" data-panel-id="layer" :style="panelStyle('layer')">
        <div class="theater-panel-heading theater-layer-panel__top-heading" @pointerdown="startPanelDrag('layer', $event)">
          <span>图层与属性</span>
          <div class="theater-panel-heading__actions">
            <small>{{ layerRows.length }}</small>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭图层与属性面板" @click="layerPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <div v-if="canEditAllObjects" class="theater-media-settings">
          <label>背景图片</label>
          <div class="theater-image-actions">
            <n-button size="tiny" :disabled="!canUploadResources" :loading="resourceUploading" @click="requestImageUpload({ kind: 'scene', target: 'background' })"><template #icon><n-icon><Photo /></n-icon></template>上传</n-button>
            <n-button size="tiny" quaternary :disabled="!canUploadResources || !store.state.liveState.background" @click="openImageEditor({ kind: 'scene', target: 'background' })"><template #icon><n-icon><Edit /></n-icon></template></n-button>
            <n-button size="tiny" quaternary type="error" :disabled="!store.state.liveState.background" @click="clearImage({ kind: 'scene', target: 'background' })">清除</n-button>
          </div>
          <label>前景图片</label>
          <div class="theater-image-actions">
            <n-button size="tiny" :disabled="!canUploadResources" :loading="resourceUploading" @click="requestImageUpload({ kind: 'scene', target: 'foreground' })"><template #icon><n-icon><Photo /></n-icon></template>上传</n-button>
            <n-button size="tiny" quaternary :disabled="!canUploadResources || !store.state.liveState.foreground" @click="openImageEditor({ kind: 'scene', target: 'foreground' })"><template #icon><n-icon><Edit /></n-icon></template></n-button>
            <n-button size="tiny" quaternary type="error" :disabled="!store.state.liveState.foreground" @click="clearImage({ kind: 'scene', target: 'foreground' })">清除</n-button>
          </div>
          <small v-if="resourceError" class="theater-resource-error">{{ resourceError }}</small>
        </div>
        <div class="theater-panel-heading"><span>层级</span></div>
        <div class="theater-layer-list">
          <button
            v-for="row in layerRows"
            :key="row.object.id"
            class="theater-layer-row"
            :class="{ 'is-active': row.object.id === store.state.selectedObjectId }"
            :style="{ paddingLeft: `${10 + row.depth * 15}px` }"
            :disabled="!canEditObject(row.object)"
            :draggable="canEditAllObjects"
            @click="selectObject(row.object.id)"
            @dragstart="canEditAllObjects && (draggedLayerId = row.object.id, store.beginObjectEdit('调整对象顺序'))"
            @dragend="draggedLayerId = null; store.commitObjectEdit()"
            @dragover.prevent
            @drop.prevent="handleLayerDrop($event, row.object.id)"
          >
            <n-icon class="theater-layer-row__grip"><GripVertical /></n-icon>
            <span class="theater-layer-row__type">{{ row.object.type === 'group' ? '组' : row.object.type === 'text' ? '字' : row.object.type === 'image' ? '图' : row.object.type === 'button' ? '钮' : '面' }}</span>
            <span class="theater-layer-row__name">{{ row.object.name }}</span>
            <small v-if="store.isPersistentObject(row.object.id)">持久</small>
          </button>
        </div>
      </aside>
    </div>

    <MessageImageEditor
      v-if="imageEditorVisible"
      :show="imageEditorVisible"
      :file="imageEditorFile"
      @update:show="value => { imageEditorVisible = value }"
      @cancel="closeImageEditor"
      @confirm="saveEditedImage"
    />
  </section>
</template>

<style scoped>
.theater-stage-app {
  --theater-accent: #3b82f6;
  --theater-panel: color-mix(in srgb, var(--sc-bg-surface, #262626) 96%, transparent);
  --theater-panel-muted: color-mix(in srgb, var(--sc-bg-layer, #3f3f46) 76%, transparent);
  --theater-border: var(--sc-border-strong, rgba(255, 255, 255, .16));
  height: 100%; min-width: 0; display: flex; flex-direction: column;
  color: var(--sc-text-primary, #f4f4f5); background: var(--sc-bg-page, #141418);
}
.theater-image-input { display: none; }
.theater-stage-toolbar {
  height: 46px; flex: 0 0 46px; display: flex; align-items: center; gap: 7px; padding: 0 8px;
  overflow-x: auto; overflow-y: hidden; border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08));
  background: var(--sc-bg-header, #262626); scrollbar-width: none;
}
.theater-stage-toolbar::-webkit-scrollbar { display: none; }
.theater-toolbar-exit, .theater-panel-switches, .theater-stage-object-actions { flex: 0 0 auto; }
.theater-stage-title {
  width: 8em; flex: 0 0 8em; overflow: hidden; color: var(--sc-text-primary, #f4f4f5);
  font-size: 15px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap;
}
.theater-panel-switches :deep(.n-button), .theater-stage-object-actions :deep(.n-button) { width: 34px; padding: 0; }
.theater-panel-switches :deep(.n-button.is-active) {
  color: #fff; background: var(--theater-accent); border-color: var(--theater-accent);
}
.theater-toolbar-divider { width: 1px; height: 22px; flex: 0 0 1px; margin: 0 2px; background: var(--theater-border); }
.theater-sync-status { width: 7px; height: 7px; flex: 0 0 7px; border-radius: 50%; background: var(--sc-fg-muted, #71717a); }
.theater-sync-status.is-online { background: #22c55e; box-shadow: 0 0 0 3px rgba(34, 197, 94, .12); }
.theater-sync-status.is-syncing { background: #f59e0b; box-shadow: 0 0 0 3px rgba(245, 158, 11, .12); }
.theater-stage-character-bridge {
  width: 218px; flex: 0 0 218px; display: grid; grid-template-columns: 28px minmax(0, 1fr); align-items: center; gap: 6px;
  padding: 3px 6px; border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); border-radius: 6px;
  background: var(--theater-panel-muted);
}
.theater-stage-character-bridge.is-offline { opacity: .52; }
.theater-stage-character-bridge img, .theater-stage-character-bridge__placeholder { width: 28px; height: 28px; border-radius: 5px; object-fit: cover; }
.theater-stage-character-bridge__placeholder { display: grid; place-items: center; color: var(--sc-text-secondary, #b5b5c5); background: var(--sc-bg-input, #3f3f46); font-size: 11px; }
.theater-stage-character-bridge__selects { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 4px; }
.theater-stage-character-bridge small { display: none; }
.theater-stage-reset-camera { flex: 0 0 auto; }
.theater-stage-zoom { width: 38px; flex: 0 0 38px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; text-align: right; }
.theater-stage-workspace { position: relative; min-height: 0; flex: 1; overflow: hidden; }
.theater-stage-viewport { position: absolute; inset: 0; min-width: 0; min-height: 0; overflow: hidden; background: var(--sc-bg-page, #141418); }
.theater-stage-canvas { position: absolute; inset: 0; }
.theater-floating-panel {
  position: absolute; z-index: 10; box-sizing: border-box; display: flex; flex-direction: column; min-height: 0; overflow: hidden;
  border: 1px solid var(--theater-border); border-radius: 7px; background: var(--theater-panel);
  box-shadow: 0 14px 34px rgba(0, 0, 0, .24); backdrop-filter: blur(12px);
  resize: both; max-width: 100%; max-height: 100%; animation: theater-panel-in .16s ease-out;
}
@keyframes theater-panel-in { from { opacity: 0; transform: translateY(-4px); } }
.theater-scene-rail { min-width: min(124px, 100%); min-height: min(160px, 100%); gap: 6px; padding: 6px; overflow-y: auto; }
.theater-object-inspector { min-width: min(240px, 100%); min-height: min(240px, 100%); overflow-y: auto; }
.theater-layer-panel { min-width: min(180px, 100%); min-height: min(220px, 100%); }
.theater-panel-heading {
  height: 32px; flex: 0 0 32px; display: flex; align-items: center; justify-content: space-between; padding: 0 8px;
  color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; font-weight: 700; cursor: move; user-select: none; touch-action: none;
}
.theater-panel-heading__actions { display: flex; align-items: center; gap: 3px; }
.theater-panel-heading small { font-weight: 400; }
.theater-panel-close { color: var(--sc-text-secondary, #b5b5c5); }
.theater-panel-empty { padding: 28px 16px; color: var(--sc-text-secondary, #b5b5c5); font-size: 12px; text-align: center; }
.theater-scene-card {
  width: 100%; display: flex; align-items: center; min-height: 34px; padding: 7px 8px; border: 1px solid transparent; border-radius: 6px;
  color: var(--sc-text-secondary, #b5b5c5); background: transparent; font-size: 12px; line-height: 1.2; text-align: left; cursor: pointer;
  transition: color .14s ease, border-color .14s ease, background .14s ease;
}
.theater-scene-card:hover { color: var(--sc-text-primary, #f4f4f5); background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-scene-card.is-active { color: var(--sc-text-primary, #f4f4f5); border-color: color-mix(in srgb, var(--theater-accent) 70%, transparent); background: color-mix(in srgb, var(--theater-accent) 16%, transparent); }
.theater-scene-card__title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-scene-actions { display: flex; margin-top: auto; }
.theater-object-editor__transform { display: grid; grid-template-columns: auto minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 6px 8px; }
.theater-object-editor__transform label { color: var(--sc-text-secondary, #b5b5c5); font-size: 12px; }
.theater-object-editor__checks { display: flex; flex-wrap: wrap; gap: 10px 14px; padding-top: 2px; }
.theater-media-settings { display: grid; gap: 5px; padding: 9px; border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-media-settings label, .theater-inspector label { color: var(--sc-fg-muted, #71717a); font-size: 10px; }
.theater-image-actions { display: flex; align-items: center; gap: 4px; }
.theater-resource-error { color: #f87171; font-size: 10px; line-height: 1.3; }
.theater-layer-list { min-height: 100px; flex: 1; overflow: auto; padding: 4px 0; }
.theater-layer-row {
  width: 100%; height: 31px; display: flex; align-items: center; gap: 7px; border: 0;
  color: var(--sc-text-primary, #f4f4f5); background: transparent; font-size: 12px; text-align: left; cursor: pointer;
  transition: color .14s ease, background .14s ease;
}
.theater-layer-row:hover { background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-layer-row:active { cursor: grabbing; }
.theater-layer-row.is-active { color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--theater-accent) 18%, transparent); }
.theater-layer-row__grip { flex: 0 0 auto; color: var(--sc-fg-muted, #71717a); font-size: 14px; cursor: grab; }
.theater-layer-row__type { width: 22px; color: var(--sc-fg-muted, #71717a); font-size: 10px; }
.theater-layer-row__name { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-layer-row small { color: #eab308; font-size: 9px; }
.theater-inspector { display: grid; gap: 8px; padding: 10px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-inspector-actions, .theater-action-add { display: flex; flex-wrap: wrap; gap: 4px; }
.theater-action-row { display: grid; gap: 4px; padding: 6px; border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); border-radius: 6px; }
.theater-action-row small { color: var(--sc-text-secondary, #b5b5c5); font-size: 9px; }
@media (max-width: 1100px) {
  .theater-stage-toolbar { gap: 5px; padding: 0 6px; }
  .theater-stage-character-bridge { width: 176px; flex-basis: 176px; }
}
@media (max-width: 720px) {
  .theater-stage-title { width: 6em; flex-basis: 6em; }
  .theater-stage-reset-camera { width: 34px; padding: 0; font-size: 0; }
}
</style>
