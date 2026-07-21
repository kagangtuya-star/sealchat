import { computed, reactive, watch, type ComputedRef } from 'vue'
import {
  createDefaultStageSurfaceStyle,
  isSafeStageImageUrl,
  normalizeStageSurfaceStyle,
  type StageAction,
  type StageDrawing,
  type StageImageRef,
  type StageLiveState,
  type StageObject,
  type StageObjectTransform,
  type StageObjectType,
  type StageScene,
  type StageSurfaceStylePatch,
  type StageSurfaceTarget,
  type StageWorkspaceState,
} from '../shared/stage-types'
import {
  applyObjectHistoryEntry,
  cloneStageData,
  collectObjectSubtree,
  createObjectHistoryEntry,
  instantiateClipboardBundle,
  type StageClipboardBundle,
  type StageObjectCollectionsSnapshot,
  type StageSelectionSnapshot,
} from './stage-editing'
import { createDefaultTheaterEffectConfig, normalizeTheaterEffectConfig } from '../effects/theater-effect-types'

const palette = ['#60a5fa', '#a78bfa', '#f472b6', '#34d399', '#fbbf24', '#fb7185']
const stageObjectTypes: StageObjectType[] = ['group', 'drawing', 'text', 'image', 'button', 'character', 'video', 'effect']
type StageInsertableObjectType = Exclude<StageObjectType, 'drawing'>

const uid = (prefix: string) => {
  const id = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${id}`
}

const clone = cloneStageData

const isRichTextValue = (value: unknown) => {
  if (typeof value !== 'string' || !value.trim()) return false
  try {
    return JSON.parse(value)?.type === 'doc'
  } catch {
    return false
  }
}

const createImageRef = (
  url: string,
  alt?: string,
  resourceId?: string,
  mimeType?: string,
  animated?: boolean,
  loopCount?: number,
): StageImageRef | null => {
  const normalized = url.trim()
  if (!normalized || !isSafeStageImageUrl(normalized)) return null
  return {
    resourceId: resourceId?.trim() || uid('resource'),
    url: normalized,
    ...(alt ? { alt } : {}),
    ...(mimeType?.trim() ? { mimeType: mimeType.trim().toLowerCase() } : {}),
    ...(animated === true ? { animated: true } : {}),
    ...(Number.isInteger(loopCount) && (loopCount || 0) > 0 && (loopCount || 0) <= 65_535 ? { loopCount } : {}),
  }
}

const makeObject = (
  name: string,
  type: StageObjectType,
  order: number,
  overrides: Partial<StageObject> = {},
): StageObject => ({
  id: uid('object'),
  parentId: null,
  type,
  name,
  transform: {
    x: type === 'effect' ? 960 : order * 1.5 - 3,
    y: type === 'effect' ? 540 : order - 2,
    width: type === 'effect' ? 1600 : type === 'group' ? 12 : type === 'image' ? 9 : 7,
    height: type === 'effect' ? 900 : type === 'group' ? 8 : type === 'image' ? 6 : 4.5,
    rotation: 0,
    scaleX: 1,
    scaleY: 1,
    z: 0,
    order,
  },
  visible: true,
  locked: false,
  aspectRatioLocked: type !== 'effect',
  interactive: type !== 'effect',
  editable: false,
  fill: type === 'text' ? '#ffffff' : palette[order % palette.length],
  text: type === 'text' ? name : undefined,
  content: type === 'effect' ? { effect: createDefaultTheaterEffectConfig() } : {},
  metadata: type === 'text' ? { textEditorMode: 'plain' } : {},
  actions: [],
  ...overrides,
})

const createLiveState = (color: string, sceneObjects: Record<string, StageObject> = {}): StageLiveState => ({
  background: null,
  foreground: null,
  surfaceStyles: {
    background: createDefaultStageSurfaceStyle('cover', { opacity: 0.9, blurPx: 10, brightness: 1, overlay: { enabled: false, color: '#000000', opacity: 0.4 } }),
    foreground: createDefaultStageSurfaceStyle(),
  },
  backgroundColor: color,
  fieldWidth: 40,
  fieldHeight: 24,
  fieldObjectFit: 'cover',
  displayGrid: false,
  gridSize: 1,
  alignWithGrid: false,
  sceneObjects,
  transition: { type: 'none', durationMs: 0 },
})

const createScene = (name: string, order: number, color: string): StageScene => {
  const title = makeObject('场景标题', 'text', 0, {
    text: name,
    transform: { x: 0, y: 0, width: 12, height: 4, rotation: 0, scaleX: 1, scaleY: 1, z: 0, order: 0 },
  })
  return {
    id: uid('scene'),
    name,
    order,
    locked: false,
    state: createLiveState(color, {
      [title.id]: title,
    }),
  }
}

export const createInitialTheaterStageState = (): StageWorkspaceState => {
  const opening = createScene('序幕', 0, '#111827')
  const tavern = createScene('酒馆', 1, '#2b1d18')
  return {
    activeSceneId: opening.id,
    liveState: clone(opening.state),
    scenes: { [opening.id]: opening, [tavern.id]: tavern },
    persistentObjects: {},
    camera: { x: 0, y: 0, zoom: 0.5 },
    selectedObjectId: null,
  }
}

const normalizeImageRef = (input: unknown): StageImageRef | null => {
  if (!input || typeof input !== 'object') return null
  const value = input as Partial<StageImageRef>
  if (typeof value.url !== 'string' || !isSafeStageImageUrl(value.url)) return null
  return {
    resourceId: typeof value.resourceId === 'string' && value.resourceId.trim()
      ? value.resourceId.trim()
      : uid('resource'),
    url: value.url.trim(),
    ...(typeof value.alt === 'string' ? { alt: value.alt } : {}),
    ...(typeof value.mimeType === 'string' && value.mimeType.trim() ? { mimeType: value.mimeType.trim().toLowerCase() } : {}),
    ...(value.animated === true ? { animated: true } : {}),
    ...(Number.isInteger(value.loopCount) && (value.loopCount || 0) > 0 && (value.loopCount || 0) <= 65_535 ? { loopCount: value.loopCount } : {}),
  }
}

const normalizeActions = (input: unknown): StageAction[] => {
  if (!Array.isArray(input)) return []
  return input.reduce<StageAction[]>((result, value) => {
    if (!value || typeof value !== 'object') return result
    const action = value as Partial<StageAction> & { payload?: Record<string, unknown> }
    const id = typeof action.id === 'string' ? action.id.trim() : ''
    if (!id || !action.payload || typeof action.payload !== 'object') return result
    if (action.type === 'chat.send') {
      const content = typeof action.payload.content === 'string' ? action.payload.content : ''
      if (!content || content.length > 10_000) return result
      result.push({
        id,
        type: action.type,
        payload: {
          content,
          ...(typeof action.payload.channelId === 'string' && action.payload.channelId.trim()
            ? { channelId: action.payload.channelId.trim() }
            : {}),
          ...(typeof action.payload.characterId === 'string' && action.payload.characterId.trim()
            ? { characterId: action.payload.characterId.trim() }
            : {}),
        },
      })
    } else if (action.type === 'chat.insert') {
      const content = typeof action.payload.content === 'string' ? action.payload.content : ''
      if (content && content.length <= 10_000) result.push({ id, type: action.type, payload: { content } })
    } else if (action.type === 'scene.apply') {
      const sceneId = typeof action.payload.sceneId === 'string' ? action.payload.sceneId.trim() : ''
      if (sceneId) result.push({ id, type: action.type, payload: { sceneId } })
    } else if (action.type === 'object.toggle') {
      const objectId = typeof action.payload.objectId === 'string' ? action.payload.objectId.trim() : ''
      if (objectId) result.push({ id, type: action.type, payload: { objectId } })
    }
    return result
  }, []).slice(0, 32)
}

const normalizeDrawing = (input: unknown): StageDrawing | undefined => {
  if (!input || typeof input !== 'object') return undefined
  const value = input as Partial<StageDrawing>
  const tools: StageDrawing['tool'][] = ['pen', 'highlighter', 'line', 'arrow', 'rectangle', 'ellipse', 'triangle', 'polygon']
  if (!value.tool || !tools.includes(value.tool) || !value.style || typeof value.style !== 'object') return undefined
  const style = value.style
  const dash = style.dash === 'dashed' || style.dash === 'dotted' ? style.dash : 'solid'
  const points = Array.isArray(value.points)
    ? value.points.filter((point): point is number => Number.isFinite(point)).slice(0, 2_000)
    : undefined
  if ((value.tool === 'pen' || value.tool === 'highlighter') && (!points || points.length < 2 || points.length % 2 !== 0)) return undefined
  return {
    tool: value.tool,
    style: {
      stroke: typeof style.stroke === 'string' ? style.stroke : '#f8fafc',
      strokeWidth: Number.isFinite(style.strokeWidth) ? Math.min(128, Math.max(1, style.strokeWidth)) : 4,
      opacity: Number.isFinite(style.opacity) ? Math.min(1, Math.max(0.05, style.opacity)) : 1,
      fill: typeof style.fill === 'string' ? style.fill : null,
      dash,
    },
    ...(points ? { points } : {}),
    ...(value.tool === 'polygon' ? { sides: Math.min(12, Math.max(5, Math.round(value.sides || 6))) } : {}),
    ...(value.tool === 'pen' || value.tool === 'highlighter'
      ? { smoothing: typeof value.smoothing === 'number' && Number.isFinite(value.smoothing) ? Math.min(1, Math.max(0, value.smoothing)) : 0.35 }
      : {}),
  }
}

const normalizeObject = (input: StageObject): StageObject | null => {
  if (!stageObjectTypes.includes(input.type)) return null
  const drawing = input.type === 'drawing' ? normalizeDrawing(input.drawing) : undefined
  if (input.type === 'drawing' && !drawing) return null
  const legacyTransform = input.transform as StageObjectTransform & { scale?: number }
  const legacyScale = Number.isFinite(legacyTransform?.scale) && (legacyTransform.scale || 0) > 0
    ? Math.min(100, Math.max(0.01, legacyTransform.scale!))
    : 1
  const transform = { ...legacyTransform }
  delete transform.scale
  const normalizeScale = (value: number | undefined) => Number.isFinite(value) && (value || 0) > 0
    ? Math.min(100, Math.max(0.01, value!))
    : legacyScale
  const metadata = input.metadata && typeof input.metadata === 'object' ? input.metadata : {}
  return {
    ...input,
    transform: {
      ...transform,
      scaleX: normalizeScale(input.transform?.scaleX),
      scaleY: normalizeScale(input.transform?.scaleY),
    },
    type: input.type,
    parentId: typeof input.parentId === 'string' ? input.parentId : null,
    visible: input.visible !== false,
    locked: input.locked === true,
    aspectRatioLocked: input.aspectRatioLocked !== false,
    interactive: input.interactive !== false,
    editable: input.editable === true,
    fill: input.type === 'text' ? '#ffffff' : typeof input.fill === 'string' ? input.fill : '#60a5fa',
    drawing,
    image: normalizeImageRef(input.image) || undefined,
    content: input.type === 'effect'
      ? {
          ...(input.content && typeof input.content === 'object' ? input.content : {}),
          effect: normalizeTheaterEffectConfig(input.content?.effect),
        }
      : input.content && typeof input.content === 'object' ? input.content : {},
    actions: normalizeActions(input.actions),
    metadata: input.type === 'text'
      ? {
          ...metadata,
          textEditorMode: metadata.textEditorMode === 'rich' || isRichTextValue(input.text) ? 'rich' : 'plain',
        }
      : metadata,
  }
}

const normalizeObjects = (input: unknown) => {
  if (!input || typeof input !== 'object') return {}
  return Object.entries(input as Record<string, StageObject>).reduce<Record<string, StageObject>>((result, [id, object]) => {
    if (!object || typeof object !== 'object' || typeof object.id !== 'string') return result
    const normalized = normalizeObject(object)
    if (normalized) result[id] = normalized
    return result
  }, {})
}

const normalizeLiveState = (input: Partial<StageLiveState> | undefined, fallbackColor = '#111827'): StageLiveState => ({
  background: normalizeImageRef(input?.background),
  foreground: normalizeImageRef(input?.foreground),
  surfaceStyles: {
    background: normalizeStageSurfaceStyle(input?.surfaceStyles?.background, input?.fieldObjectFit || 'cover', { opacity: 0.9, blurPx: 10 }),
    foreground: normalizeStageSurfaceStyle(input?.surfaceStyles?.foreground, input?.fieldObjectFit || 'cover'),
  },
  backgroundColor: typeof input?.backgroundColor === 'string' ? input.backgroundColor : fallbackColor,
  fieldWidth: typeof input?.fieldWidth === 'number' && input.fieldWidth > 0 ? input.fieldWidth : 40,
  fieldHeight: typeof input?.fieldHeight === 'number' && input.fieldHeight > 0 ? input.fieldHeight : 24,
  fieldObjectFit: input?.fieldObjectFit === 'fill' || input?.fieldObjectFit === 'contain'
    ? input.fieldObjectFit
    : 'cover',
  displayGrid: input?.displayGrid === true,
  gridSize: typeof input?.gridSize === 'number' && input.gridSize > 0 ? input.gridSize : 1,
  alignWithGrid: input?.alignWithGrid === true,
  sceneObjects: normalizeObjects(input?.sceneObjects),
  transition: {
    type: input?.transition?.type === 'crossfade' ? 'crossfade' : 'none',
    durationMs: typeof input?.transition?.durationMs === 'number' && input.transition.durationMs >= 0
      ? input.transition.durationMs
      : 0,
  },
  serverState: input?.serverState && typeof input.serverState === 'object' ? input.serverState : {},
})

export interface TheaterStageStore {
  state: StageWorkspaceState
  scenes: ComputedRef<StageScene[]>
  activeScene: ComputedRef<StageScene>
  activeObjects: ComputedRef<Record<string, StageObject>>
  selection: TheaterStageSelectionState
  selectedObjects: ComputedRef<StageObject[]>
  setBulkSelectionMode: (enabled: boolean) => void
  selectObject: (objectId: string | null, additive?: boolean) => void
  setSelectedObjectIds: (objectIds: string[], primaryId?: string | null) => void
  clearSelection: () => void
  patchSelectedObjects: (patch: StageObjectBatchPatch) => number
  setObjectFlag: (objectId: string, key: StageObjectQuickFlag, value: boolean) => boolean
  selectScene: (sceneId: string) => void
  addScene: () => void
  duplicateScene: () => void
  removeScene: () => void
  addObject: (type: StageInsertableObjectType, persistent?: boolean) => StageObject
  addDrawing: (
    drawing: StageDrawing,
    transform: Pick<StageObjectTransform, 'x' | 'y' | 'width' | 'height' | 'rotation'>,
  ) => StageObject
  removeObjects: (objectIds: string[], recordHistory?: boolean) => number
  removeSelectedObjects: (recordHistory?: boolean) => number
  removeSelectedObject: (recordHistory?: boolean) => void
  copySelectedObject: () => boolean
  cutSelectedObject: () => boolean
  pasteObject: () => StageObject | null
  undo: () => boolean
  canCopy: ComputedRef<boolean>
  canCut: ComputedRef<boolean>
  canPaste: ComputedRef<boolean>
  canUndo: ComputedRef<boolean>
  beginObjectEdit: (label?: string) => void
  commitObjectEdit: () => void
  cancelObjectEdit: () => void
  setParent: (objectId: string, parentId: string | null) => boolean
  reparentObject: (
    objectId: string,
    parentId: string | null,
    transform: Pick<StageObjectTransform, 'x' | 'y' | 'rotation' | 'scaleX' | 'scaleY'>,
  ) => boolean
  moveOrder: (objectId: string, direction: -1 | 1) => void
  reorderObject: (objectId: string, targetId: string, placement: 'before' | 'after') => void
  setSceneImage: (target: 'background' | 'foreground', url: string, resourceId?: string, mimeType?: string, animated?: boolean, loopCount?: number) => boolean
  patchSceneSurfaceStyle: (target: StageSurfaceTarget, patch: StageSurfaceStylePatch) => void
  resetSceneSurfaceStyle: (target: StageSurfaceTarget) => void
  setObjectImage: (
    objectId: string,
    url: string,
    resourceId?: string,
    mimeType?: string,
    animated?: boolean,
    loopCount?: number,
    dimensions?: { width: number, height: number },
  ) => boolean
  addObjectAction: (objectId: string, action: StageAction) => boolean
  removeObjectAction: (objectId: string, actionId: string) => boolean
  toggleObject: (objectId: string) => boolean
  isPersistentObject: (objectId: string) => boolean
  resetCamera: () => void
  getSnapshot: () => StageWorkspaceState
  applyScene: (sceneId: string) => boolean
  replaceState: (next: StageWorkspaceState) => void
}

export interface TheaterStageSelectionState {
  bulkMode: boolean
  selectedIds: string[]
}

export type StageObjectBatchPatch = Partial<Pick<StageObject,
  'visible' | 'interactive' | 'editable' | 'locked' | 'aspectRatioLocked' | 'fill'
>>

export type StageObjectQuickFlag = 'visible' | 'editable' | 'locked'

export const createTheaterStageStore = (_storageKey?: string): TheaterStageStore => {
  const state = reactive<StageWorkspaceState>(createInitialTheaterStageState())
  const scenes = computed(() => Object.values(state.scenes).sort((a, b) => a.order - b.order))
  const activeScene = computed(() => state.scenes[state.activeSceneId] || scenes.value[0])
  const activeObjects = computed(() => ({ ...state.liveState.sceneObjects, ...state.persistentObjects }))
  const selection = reactive<TheaterStageSelectionState>({ bulkMode: false, selectedIds: [] })
  const selectedObjects = computed(() => selection.selectedIds
    .map((id) => activeObjects.value[id])
    .filter((object): object is StageObject => Boolean(object)))
  const editingState = reactive({ historyDepth: 0, clipboardReady: false })
  const history: NonNullable<ReturnType<typeof createObjectHistoryEntry>>[] = []
  let clipboard: StageClipboardBundle | null = null
  let pasteCount = 0
  let transaction: {
    label: string
    before: StageObjectCollectionsSnapshot
    selectionBefore: StageSelectionSnapshot
  } | null = null

  const snapshotObjectCollections = (): StageObjectCollectionsSnapshot => ({
    sceneId: state.activeSceneId,
    sceneObjects: clone(state.liveState.sceneObjects),
    persistentObjects: clone(state.persistentObjects),
  })

  const snapshotSelection = (): StageSelectionSnapshot => ({
    selectedIds: [...selection.selectedIds],
    primaryId: state.selectedObjectId,
  })

  const beginObjectEdit = (label = '修改对象') => {
    if (transaction) return
    transaction = {
      label,
      before: snapshotObjectCollections(),
      selectionBefore: snapshotSelection(),
    }
  }

  const commitObjectEdit = () => {
    if (!transaction) return
    const current = transaction
    transaction = null
    const entry = createObjectHistoryEntry(
      current.label,
      current.before,
      snapshotObjectCollections(),
      current.selectionBefore,
      snapshotSelection(),
    )
    if (!entry) return
    history.push(entry)
    if (history.length > 100) history.shift()
    editingState.historyDepth = history.length
  }

  const cancelObjectEdit = () => {
    if (!transaction) return
    const current = transaction
    transaction = null
    if (current.before.sceneId !== state.activeSceneId) return
    state.liveState.sceneObjects = clone(current.before.sceneObjects)
    state.persistentObjects = clone(current.before.persistentObjects)
    setSelectedObjectIds(current.selectionBefore.selectedIds, current.selectionBefore.primaryId)
  }

  const runObjectEdit = <T>(label: string, mutate: () => T): T => {
    const ownsTransaction = !transaction
    if (ownsTransaction) beginObjectEdit(label)
    try {
      const result = mutate()
      if (ownsTransaction) commitObjectEdit()
      return result
    } catch (error) {
      if (ownsTransaction) cancelObjectEdit()
      throw error
    }
  }

  const canCopy = computed(() => selectedObjects.value.length === 1)
  const canCut = computed(() => canCopy.value)
  const canPaste = computed(() => editingState.clipboardReady)
  const canUndo = computed(() => editingState.historyDepth > 0)

  const setSelectedObjectIds = (objectIds: string[], primaryId?: string | null) => {
    const next = [...new Set(objectIds)].filter((id) => Boolean(activeObjects.value[id]))
    const primary = primaryId && next.includes(primaryId) ? primaryId : next[next.length - 1] || null
    selection.selectedIds = next
    state.selectedObjectId = primary
  }

  const clearSelection = () => setSelectedObjectIds([])

  const setBulkSelectionMode = (enabled: boolean) => {
    selection.bulkMode = enabled
    setSelectedObjectIds(state.selectedObjectId ? [state.selectedObjectId] : [])
  }

  const selectObject = (objectId: string | null, additive = false) => {
    if (!objectId) {
      clearSelection()
      return
    }
    if (!activeObjects.value[objectId]) return
    if (!selection.bulkMode || !additive) {
      setSelectedObjectIds([objectId], objectId)
      return
    }
    const selected = selection.selectedIds.includes(objectId)
    const next = selected
      ? selection.selectedIds.filter((id) => id !== objectId)
      : [...selection.selectedIds, objectId]
    setSelectedObjectIds(next, selected ? undefined : objectId)
  }

  const patchSelectedObjects = (patch: StageObjectBatchPatch) => runObjectEdit('批量修改对象', () => {
    const entries = Object.entries(patch) as [keyof StageObjectBatchPatch, StageObjectBatchPatch[keyof StageObjectBatchPatch]][]
    let changed = 0
    selectedObjects.value.forEach((object) => {
      let objectChanged = false
      entries.forEach(([key, value]) => {
        if (value === undefined || object[key] === value) return
        ;(object as unknown as Record<string, unknown>)[key] = value
        objectChanged = true
      })
      if (objectChanged) changed += 1
    })
    return changed
  })

  const setObjectFlag = (objectId: string, key: StageObjectQuickFlag, value: boolean) => runObjectEdit('快速修改对象', () => {
    const object = activeObjects.value[objectId]
    if (!object || object[key] === value) return false
    object[key] = value
    return true
  })

  const saveLiveState = () => {
    const scene = state.scenes[state.activeSceneId]
    if (scene) scene.state = clone(state.liveState)
  }

  const selectScene = (sceneId: string) => {
    if (!state.scenes[sceneId] || sceneId === state.activeSceneId) return
    saveLiveState()
    state.activeSceneId = sceneId
    state.liveState = clone(state.scenes[sceneId].state)
    clearSelection()
  }

  const renameScene = (sceneId: string, name: string) => {
    const scene = state.scenes[sceneId]
    const nextName = name.trim()
    if (!scene || !nextName || scene.name === nextName) return false
    scene.name = nextName
    return true
  }

  const addScene = () => {
    saveLiveState()
    const order = scenes.value.length
    const scene = createScene(`场景 ${order + 1}`, order, '#172033')
    state.scenes[scene.id] = scene
    state.activeSceneId = scene.id
    state.liveState = clone(scene.state)
    clearSelection()
  }

  const duplicateScene = () => {
    saveLiveState()
    const source = activeScene.value
    const idMap = new Map<string, string>()
    Object.keys(source.state.sceneObjects).forEach((id) => idMap.set(id, uid('object')))
    const objects = Object.values(source.state.sceneObjects).reduce<Record<string, StageObject>>((result, object) => {
      const id = idMap.get(object.id)!
      const transitionKey = typeof object.metadata.transitionKey === 'string' && object.metadata.transitionKey.trim()
        ? object.metadata.transitionKey.trim()
        : object.id
      object.metadata = { ...object.metadata, transitionKey }
      result[id] = {
        ...clone(object),
        id,
        parentId: object.parentId ? idMap.get(object.parentId) || null : null,
        metadata: { ...clone(object.metadata), transitionKey },
      }
      return result
    }, {})
    const scene: StageScene = {
      ...clone(source),
      id: uid('scene'),
      name: `${source.name} 副本`,
      order: scenes.value.length,
      state: { ...clone(source.state), sceneObjects: objects },
    }
    state.scenes[scene.id] = scene
    state.activeSceneId = scene.id
    state.liveState = clone(scene.state)
    clearSelection()
  }

  const removeScene = () => {
    if (scenes.value.length <= 1) return
    const currentIndex = scenes.value.findIndex((scene) => scene.id === state.activeSceneId)
    delete state.scenes[state.activeSceneId]
    const remaining = scenes.value
    const next = remaining[Math.max(0, currentIndex - 1)] || remaining[0]
    state.activeSceneId = next.id
    state.liveState = clone(next.state)
    clearSelection()
  }

  const placeObjectAbove = (
    object: StageObject,
    collection: Record<string, StageObject>,
    referenceId: string | null = state.selectedObjectId,
  ) => {
    const reference = referenceId ? activeObjects.value[referenceId] : undefined
    if (reference && collection[reference.id]) {
      object.parentId = reference.parentId
      reorderObject(object.id, reference.id, 'before')
      return
    }

    object.parentId = null
    const topObject = Object.values(activeObjects.value)
      .filter((item) => item.parentId === null && item.id !== object.id)
      .sort((a, b) => b.transform.z - a.transform.z || b.transform.order - a.transform.order)[0]
    if (topObject) reorderObject(object.id, topObject.id, 'before')
  }

  const addObject = (type: StageInsertableObjectType, persistent = false) => runObjectEdit('添加对象', () => {
    const objects = persistent ? state.persistentObjects : state.liveState.sceneObjects
    const object = makeObject(
      type === 'group'
        ? '新建组'
        : type === 'text'
          ? '新建文字'
          : type === 'image'
            ? '新建图片'
            : type === 'button'
              ? '新建按钮'
              : type === 'effect'
                ? '新建特效'
                : '新建对象',
      type,
      Object.keys(objects).length,
    )
    objects[object.id] = object
    placeObjectAbove(object, objects)
    setSelectedObjectIds([object.id], object.id)
    return object
  })

  const drawingNames: Record<StageDrawing['tool'], string> = {
    pen: '画笔',
    highlighter: '荧光笔',
    line: '直线',
    arrow: '箭头',
    rectangle: '矩形',
    ellipse: '椭圆',
    triangle: '三角形',
    polygon: '多边形',
  }

  const addDrawing = (
    drawing: StageDrawing,
    transform: Pick<StageObjectTransform, 'x' | 'y' | 'width' | 'height' | 'rotation'>,
  ) => runObjectEdit('添加绘制对象', () => {
    const objects = state.liveState.sceneObjects
    const order = Object.keys(objects).length
    const object = makeObject(`新建${drawingNames[drawing.tool]}`, 'drawing', order, {
      drawing: clone(drawing),
      fill: drawing.style.fill || drawing.style.stroke,
      interactive: false,
      aspectRatioLocked: false,
      transform: { ...transform, scaleX: 1, scaleY: 1, z: 0, order },
    })
    objects[object.id] = object
    placeObjectAbove(object, objects)
    setSelectedObjectIds([object.id], object.id)
    return object
  })

  const getObject = (objectId: string) => activeObjects.value[objectId]
  const getObjectCollection = (objectId: string) => state.persistentObjects[objectId]
    ? state.persistentObjects
    : state.liveState.sceneObjects

  const collectDescendants = (objectId: string): string[] => {
    const result: string[] = []
    const visit = (id: string) => {
      Object.values(activeObjects.value).forEach((object) => {
        if (object.parentId !== id) return
        result.push(object.id)
        visit(object.id)
      })
    }
    visit(objectId)
    return result
  }

  const selectedRootIds = (objectIds: string[]) => {
    const candidates = new Set(objectIds.filter((id) => Boolean(getObject(id))))
    return [...candidates].filter((id) => {
      let parentId = getObject(id)?.parentId || null
      while (parentId) {
        if (candidates.has(parentId)) return false
        parentId = getObject(parentId)?.parentId || null
      }
      return true
    })
  }

  const removeObjectsNow = (objectIds: string[]) => {
    const removedIds = new Set<string>()
    selectedRootIds(objectIds).forEach((id) => {
      removedIds.add(id)
      collectDescendants(id).forEach((childId) => removedIds.add(childId))
    })
    removedIds.forEach((id) => delete getObjectCollection(id)[id])
    if (removedIds.size) {
      setSelectedObjectIds(selection.selectedIds.filter((id) => !removedIds.has(id)))
    }
    return removedIds.size
  }

  const removeObjects = (objectIds: string[], recordHistory = true) => {
    if (!recordHistory) return removeObjectsNow(objectIds)
    const rootCount = selectedRootIds(objectIds).length
    if (!rootCount) return 0
    return runObjectEdit(rootCount > 1 ? '批量删除组件' : '删除组件', () => removeObjectsNow(objectIds))
  }

  const removeSelectedObjects = (recordHistory = true) => removeObjects([...selection.selectedIds], recordHistory)

  const removeSelectedObject = (recordHistory = true) => {
    const id = state.selectedObjectId
    if (id) removeObjects([id], recordHistory)
  }

  const copySelectedObject = () => {
    const rootId = state.selectedObjectId
    if (!rootId) return false
    const persistent = isPersistentObject(rootId)
    const collection = persistent ? state.persistentObjects : state.liveState.sceneObjects
    const objects = collectObjectSubtree(collection, rootId)
    if (!objects.length) return false
    clipboard = {
      version: 1,
      sourceSceneId: state.activeSceneId,
      persistent,
      rootId,
      objects,
    }
    pasteCount = 0
    editingState.clipboardReady = true
    return true
  }

  const cutSelectedObject = () => {
    if (!copySelectedObject()) return false
    runObjectEdit('剪切对象', () => removeObjectsNow(state.selectedObjectId ? [state.selectedObjectId] : []))
    return true
  }

  const pasteObject = () => {
    if (!clipboard) return null
    return runObjectEdit('粘贴对象', () => {
      const collection = clipboard!.persistent ? state.persistentObjects : state.liveState.sceneObjects
      const sourceRoot = clipboard!.objects.find((object) => object.id === clipboard!.rootId)
      const keepParent = sourceRoot?.parentId
        && (clipboard!.persistent || clipboard!.sourceSceneId === state.activeSceneId)
        && Boolean(collection[sourceRoot.parentId])
      pasteCount += 1
      const pasted = instantiateClipboardBundle(
        clipboard!,
        uid,
        pasteCount,
        keepParent && sourceRoot?.parentId ? sourceRoot.parentId : null,
      )
      pasted.objects.forEach((object) => { collection[object.id] = object })
      placeObjectAbove(collection[pasted.rootId], collection, sourceRoot?.id || null)
      setSelectedObjectIds([pasted.rootId], pasted.rootId)
      return collection[pasted.rootId]
    })
  }

  const undo = () => {
    commitObjectEdit()
    while (history.length) {
      const entry = history.pop()!
      editingState.historyDepth = history.length
      const scene = state.scenes[entry.sceneId]
      if (!scene) continue
      const sceneObjects = entry.sceneId === state.activeSceneId
        ? state.liveState.sceneObjects
        : scene.state.sceneObjects
      applyObjectHistoryEntry(entry, 'undo', sceneObjects, state.persistentObjects)
      const restoredSelection = entry.selectionBefore
      setSelectedObjectIds(restoredSelection.selectedIds, restoredSelection.primaryId)
      return true
    }
    return false
  }

  const canSetParent = (objectId: string, parentId: string | null) => {
    const object = getObject(objectId)
    if (!object || objectId === parentId) return false
    if (!parentId) return true
    const parent = getObject(parentId)
    if (!parent || parent.type !== 'group') return false
    if (collectDescendants(objectId).includes(parentId)) return false
    if (isPersistentObject(objectId) !== isPersistentObject(parentId)) return false
    return true
  }

  const setParent = (objectId: string, parentId: string | null) => runObjectEdit('调整对象分组', () => {
    if (!canSetParent(objectId, parentId)) return false
    const object = getObject(objectId)!
    object.parentId = parentId
    return true
  })

  const reparentObject = (
    objectId: string,
    parentId: string | null,
    transform: Pick<StageObjectTransform, 'x' | 'y' | 'rotation' | 'scaleX' | 'scaleY'>,
  ) => runObjectEdit('调整对象分组', () => {
    if (!canSetParent(objectId, parentId)) return false
    const object = getObject(objectId)!
    object.parentId = parentId
    object.transform.x = transform.x
    object.transform.y = transform.y
    object.transform.rotation = transform.rotation
    object.transform.scaleX = Math.min(100, Math.max(0.01, transform.scaleX))
    object.transform.scaleY = Math.min(100, Math.max(0.01, transform.scaleY))
    return true
  })

  const moveOrder = (objectId: string, direction: -1 | 1) => runObjectEdit('调整对象顺序', () => {
    const object = getObject(objectId)
    if (!object) return
    const siblings = Object.values(activeObjects.value)
      .filter((item) => item.parentId === object.parentId)
      .sort((a, b) => b.transform.z - a.transform.z || b.transform.order - a.transform.order)
    const index = siblings.findIndex((item) => item.id === objectId)
    const target = siblings[index - direction]
    if (!target) return
    reorderObject(objectId, target.id, direction > 0 ? 'before' : 'after')
  })

  const reorderObject = (objectId: string, targetId: string, placement: 'before' | 'after') => runObjectEdit('调整对象顺序', () => {
    const object = getObject(objectId)
    const target = getObject(targetId)
    if (!object || !target || object.id === target.id || object.parentId !== target.parentId) return
    const siblings = Object.values(activeObjects.value)
      .filter((item) => item.parentId === object.parentId)
      .sort((a, b) => b.transform.z - a.transform.z || b.transform.order - a.transform.order)
      .filter((item) => item.id !== objectId)
    const targetIndex = siblings.findIndex((item) => item.id === targetId)
    if (targetIndex < 0) return
    siblings.splice(targetIndex + (placement === 'after' ? 1 : 0), 0, object)
    siblings.forEach((item, index) => {
      const rank = siblings.length - index
      item.transform.z = rank
      item.transform.order = rank
    })
  })

  const setSceneImage = (target: 'background' | 'foreground', url: string, resourceId?: string, mimeType?: string, animated?: boolean, loopCount?: number) => {
    if (!url.trim()) {
      state.liveState[target] = null
      return true
    }
    const image = createImageRef(url, target === 'background' ? '场景背景' : '场景前景', resourceId, mimeType, animated, loopCount)
    if (!image) return false
    state.liveState[target] = image
    return true
  }

  const patchSceneSurfaceStyle = (target: StageSurfaceTarget, patch: StageSurfaceStylePatch) => {
    const current = state.liveState.surfaceStyles[target]
    state.liveState.surfaceStyles[target] = normalizeStageSurfaceStyle({
      ...current,
      ...patch,
      overlay: {
        ...current.overlay,
        ...patch.overlay,
      },
    }, current.fit)
  }

  const resetSceneSurfaceStyle = (target: StageSurfaceTarget) => {
    state.liveState.surfaceStyles[target] = target === 'background'
      ? createDefaultStageSurfaceStyle('cover', { opacity: 0.9, blurPx: 10 })
      : createDefaultStageSurfaceStyle()
  }

  const setObjectImage = (
    objectId: string,
    url: string,
    resourceId?: string,
    mimeType?: string,
    animated?: boolean,
    loopCount?: number,
    dimensions?: { width: number, height: number },
  ) => runObjectEdit('修改对象图片', () => {
    const object = getObject(objectId)
    if (!object || (object.type !== 'image' && object.type !== 'effect')) return false
    if (!url.trim()) {
      object.image = undefined
      if (object.type === 'effect') {
        const config = normalizeTheaterEffectConfig(object.content?.effect)
        config.media = null
        object.content = { ...object.content, effect: config }
      }
      return true
    }
    const image = createImageRef(url, object.name, resourceId, mimeType, animated, loopCount)
    if (!image) return false
    const effectConfig = object.type === 'effect'
      ? normalizeTheaterEffectConfig(object.content?.effect)
      : null
    const initializeMediaFrame = Boolean(
      effectConfig?.kind === 'media'
      && !object.image
      && !effectConfig.media
      && dimensions
      && Number.isFinite(dimensions.width)
      && Number.isFinite(dimensions.height)
      && dimensions.width > 0
      && dimensions.height > 0,
    )
    object.image = image
    if (effectConfig) {
      effectConfig.media = image
      if (initializeMediaFrame && dimensions) {
        object.transform = {
          ...object.transform,
          x: 960,
          y: 540,
          width: Math.round(dimensions.width),
          height: Math.round(dimensions.height),
          rotation: 0,
          scaleX: 1,
          scaleY: 1,
        }
        effectConfig.builtin.mediaTransform = {
          x: 0,
          y: 0,
          scale: 1,
          rotation: 0,
          mirror: false,
        }
      }
      object.content = { ...object.content, effect: effectConfig }
    }
    return true
  })

  const addObjectAction = (objectId: string, action: StageAction) => runObjectEdit('添加对象动作', () => {
    const object = getObject(objectId)
    if (!object || !['image', 'button'].includes(object.type)) return false
    object.actions.push(clone(action))
    return true
  })

  const removeObjectAction = (objectId: string, actionId: string) => runObjectEdit('删除对象动作', () => {
    const object = getObject(objectId)
    if (!object) return false
    const index = object.actions.findIndex((action) => action.id === actionId)
    if (index < 0) return false
    object.actions.splice(index, 1)
    return true
  })

  const toggleObject = (objectId: string) => {
    const object = getObject(objectId)
    if (!object) return false
    object.visible = !object.visible
    if (!object.visible && selection.selectedIds.includes(objectId)) {
      setSelectedObjectIds(selection.selectedIds.filter((id) => id !== objectId))
    }
    return true
  }

  const isPersistentObject = (objectId: string) => !!state.persistentObjects[objectId]
  const resetCamera = () => Object.assign(state.camera, { x: 0, y: 0, zoom: 0.5 })
  const getSnapshot = () => clone(state)
  const applyScene = (sceneId: string) => {
    if (!state.scenes[sceneId]) return false
    selectScene(sceneId)
    return true
  }
  const replaceState = (next: StageWorkspaceState) => {
    transaction = null
    const value = clone(next)
    state.activeSceneId = value.activeSceneId
    state.liveState = value.liveState
    state.scenes = value.scenes
    state.persistentObjects = value.persistentObjects
    state.camera = value.camera
    setSelectedObjectIds(value.selectedObjectId ? [value.selectedObjectId] : [], value.selectedObjectId)
  }

  watch(() => state.liveState, saveLiveState, { deep: true, flush: 'sync' })
  watch(activeObjects, () => {
    const valid = selection.selectedIds.filter((id) => Boolean(activeObjects.value[id]))
    if (valid.length !== selection.selectedIds.length) setSelectedObjectIds(valid)
  })

  return {
    state,
    scenes,
    activeScene,
    activeObjects,
    selection,
    selectedObjects,
    setBulkSelectionMode,
    selectObject,
    setSelectedObjectIds,
    clearSelection,
    patchSelectedObjects,
    setObjectFlag,
    selectScene,
    renameScene,
    addScene,
    duplicateScene,
    removeScene,
    addObject,
    addDrawing,
    removeObjects,
    removeSelectedObjects,
    removeSelectedObject,
    copySelectedObject,
    cutSelectedObject,
    pasteObject,
    undo,
    canCopy,
    canCut,
    canPaste,
    canUndo,
    beginObjectEdit,
    commitObjectEdit,
    cancelObjectEdit,
    setParent,
    reparentObject,
    moveOrder,
    reorderObject,
    setSceneImage,
    patchSceneSurfaceStyle,
    resetSceneSurfaceStyle,
    setObjectImage,
    addObjectAction,
    removeObjectAction,
    toggleObject,
    isPersistentObject,
    resetCamera,
    getSnapshot,
    applyScene,
    replaceState,
  }
}
