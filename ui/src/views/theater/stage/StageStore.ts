import { computed, reactive, toRaw, watch, type ComputedRef } from 'vue'
import {
  isSafeStageImageUrl,
  type StageAction,
  type StageImageRef,
  type StageLiveState,
  type StageObject,
  type StageObjectType,
  type StageScene,
  type StageWorkspaceState,
} from '../shared/stage-types'

const palette = ['#60a5fa', '#a78bfa', '#f472b6', '#34d399', '#fbbf24', '#fb7185']

const uid = (prefix: string) => {
  const id = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${id}`
}

const clone = <T>(value: T): T => structuredClone(toRaw(value))

const createImageRef = (url: string, alt?: string, resourceId?: string): StageImageRef | null => {
  const normalized = url.trim()
  if (!normalized || !isSafeStageImageUrl(normalized)) return null
  return { resourceId: resourceId?.trim() || uid('resource'), url: normalized, ...(alt ? { alt } : {}) }
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
    x: order * 1.5 - 3,
    y: order - 2,
    width: type === 'group' ? 12 : type === 'image' ? 9 : 7,
    height: type === 'group' ? 8 : type === 'image' ? 6 : 4.5,
    rotation: 0,
    z: 0,
    order,
  },
  visible: true,
  locked: false,
  sizeLocked: false,
  interactive: true,
  editable: false,
  fill: palette[order % palette.length],
  text: type === 'text' ? name : undefined,
  metadata: {},
  actions: [],
  ...overrides,
})

const createLiveState = (color: string, sceneObjects: Record<string, StageObject> = {}): StageLiveState => ({
  background: null,
  foreground: null,
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
  const group = makeObject('角色组', 'group', 0, { fill: 'rgba(96, 165, 250, 0.12)' })
  const actor = makeObject('主角', 'shape', 1, { parentId: group.id })
  const title = makeObject('场景标题', 'text', 2, { text: name, parentId: group.id })
  const panel = makeObject('信息面板', 'shape', 3, {
    transform: { x: -13, y: -7, width: 8, height: 4, rotation: 0, z: 1, order: 3 },
  })
  return {
    id: uid('scene'),
    name,
    order,
    locked: false,
    state: createLiveState(color, {
      [group.id]: group,
      [actor.id]: actor,
      [title.id]: title,
      [panel.id]: panel,
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
    camera: { x: 0, y: 0, zoom: 1 },
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

const normalizeObject = (input: StageObject): StageObject => ({
  ...input,
  type: ['group', 'shape', 'text', 'image', 'button', 'character', 'video'].includes(input.type) ? input.type : 'shape',
  parentId: typeof input.parentId === 'string' ? input.parentId : null,
  visible: input.visible !== false,
  locked: input.locked === true,
  sizeLocked: input.sizeLocked === true,
  interactive: input.interactive !== false,
  editable: input.editable === true,
  fill: typeof input.fill === 'string' ? input.fill : '#60a5fa',
  image: normalizeImageRef(input.image) || undefined,
  content: input.content && typeof input.content === 'object' ? input.content : {},
  actions: normalizeActions(input.actions),
  metadata: input.metadata && typeof input.metadata === 'object' ? input.metadata : {},
})

const normalizeObjects = (input: unknown) => {
  if (!input || typeof input !== 'object') return {}
  return Object.entries(input as Record<string, StageObject>).reduce<Record<string, StageObject>>((result, [id, object]) => {
    if (!object || typeof object !== 'object' || typeof object.id !== 'string') return result
    result[id] = normalizeObject(object)
    return result
  }, {})
}

const normalizeLiveState = (input: Partial<StageLiveState> | undefined, fallbackColor = '#111827'): StageLiveState => ({
  background: normalizeImageRef(input?.background),
  foreground: normalizeImageRef(input?.foreground),
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
  selectScene: (sceneId: string) => void
  addScene: () => void
  duplicateScene: () => void
  removeScene: () => void
  addObject: (type: StageObjectType, persistent?: boolean) => StageObject
  removeSelectedObject: () => void
  setParent: (objectId: string, parentId: string | null) => void
  moveOrder: (objectId: string, direction: -1 | 1) => void
  reorderObject: (objectId: string, targetId: string, placement: 'before' | 'after') => void
  setSceneImage: (target: 'background' | 'foreground', url: string, resourceId?: string) => boolean
  setObjectImage: (objectId: string, url: string, resourceId?: string) => boolean
  addObjectAction: (objectId: string, action: StageAction) => boolean
  removeObjectAction: (objectId: string, actionId: string) => boolean
  toggleObject: (objectId: string) => boolean
  isPersistentObject: (objectId: string) => boolean
  resetCamera: () => void
  getSnapshot: () => StageWorkspaceState
  applyScene: (sceneId: string) => boolean
  replaceState: (next: StageWorkspaceState) => void
}

export const createTheaterStageStore = (_storageKey?: string): TheaterStageStore => {
  const state = reactive<StageWorkspaceState>(createInitialTheaterStageState())
  const scenes = computed(() => Object.values(state.scenes).sort((a, b) => a.order - b.order))
  const activeScene = computed(() => state.scenes[state.activeSceneId] || scenes.value[0])
  const activeObjects = computed(() => ({ ...state.liveState.sceneObjects, ...state.persistentObjects }))

  const saveLiveState = () => {
    const scene = state.scenes[state.activeSceneId]
    if (scene) scene.state = clone(state.liveState)
  }

  const selectScene = (sceneId: string) => {
    if (!state.scenes[sceneId] || sceneId === state.activeSceneId) return
    saveLiveState()
    state.activeSceneId = sceneId
    state.liveState = clone(state.scenes[sceneId].state)
    state.selectedObjectId = null
  }

  const addScene = () => {
    saveLiveState()
    const order = scenes.value.length
    const scene = createScene(`场景 ${order + 1}`, order, '#172033')
    state.scenes[scene.id] = scene
    state.activeSceneId = scene.id
    state.liveState = clone(scene.state)
    state.selectedObjectId = null
  }

  const duplicateScene = () => {
    saveLiveState()
    const source = activeScene.value
    const idMap = new Map<string, string>()
    Object.keys(source.state.sceneObjects).forEach((id) => idMap.set(id, uid('object')))
    const objects = Object.values(source.state.sceneObjects).reduce<Record<string, StageObject>>((result, object) => {
      const id = idMap.get(object.id)!
      result[id] = {
        ...clone(object),
        id,
        parentId: object.parentId ? idMap.get(object.parentId) || null : null,
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
    state.selectedObjectId = null
  }

  const removeScene = () => {
    if (scenes.value.length <= 1) return
    const currentIndex = scenes.value.findIndex((scene) => scene.id === state.activeSceneId)
    delete state.scenes[state.activeSceneId]
    const remaining = scenes.value
    const next = remaining[Math.max(0, currentIndex - 1)] || remaining[0]
    state.activeSceneId = next.id
    state.liveState = clone(next.state)
    state.selectedObjectId = null
  }

  const addObject = (type: StageObjectType, persistent = false) => {
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
              : '新建面板',
      type,
      Object.keys(objects).length,
    )
    objects[object.id] = object
    state.selectedObjectId = object.id
    return object
  }

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

  const removeSelectedObject = () => {
    const id = state.selectedObjectId
    if (!id) return
    for (const childId of collectDescendants(id)) delete getObjectCollection(childId)[childId]
    delete getObjectCollection(id)[id]
    state.selectedObjectId = null
  }

  const setParent = (objectId: string, parentId: string | null) => {
    const object = getObject(objectId)
    if (!object || objectId === parentId) return
    if (parentId && collectDescendants(objectId).includes(parentId)) return
    if (parentId && !getObject(parentId)) return
    if (parentId && isPersistentObject(objectId) !== isPersistentObject(parentId)) return
    object.parentId = parentId
  }

  const moveOrder = (objectId: string, direction: -1 | 1) => {
    const object = getObject(objectId)
    if (!object) return
    const siblings = Object.values(activeObjects.value)
      .filter((item) => item.parentId === object.parentId)
      .sort((a, b) => b.transform.z - a.transform.z || b.transform.order - a.transform.order)
    const index = siblings.findIndex((item) => item.id === objectId)
    const target = siblings[index - direction]
    if (!target) return
    reorderObject(objectId, target.id, direction > 0 ? 'before' : 'after')
  }

  const reorderObject = (objectId: string, targetId: string, placement: 'before' | 'after') => {
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
  }

  const setSceneImage = (target: 'background' | 'foreground', url: string, resourceId?: string) => {
    if (!url.trim()) {
      state.liveState[target] = null
      return true
    }
    const image = createImageRef(url, target === 'background' ? '场景背景' : '场景前景', resourceId)
    if (!image) return false
    state.liveState[target] = image
    return true
  }

  const setObjectImage = (objectId: string, url: string, resourceId?: string) => {
    const object = getObject(objectId)
    if (!object || object.type !== 'image') return false
    if (!url.trim()) {
      object.image = undefined
      return true
    }
    const image = createImageRef(url, object.name, resourceId)
    if (!image) return false
    object.image = image
    return true
  }

  const addObjectAction = (objectId: string, action: StageAction) => {
    const object = getObject(objectId)
    if (!object || !['image', 'button'].includes(object.type)) return false
    object.actions.push(clone(action))
    return true
  }

  const removeObjectAction = (objectId: string, actionId: string) => {
    const object = getObject(objectId)
    if (!object) return false
    const index = object.actions.findIndex((action) => action.id === actionId)
    if (index < 0) return false
    object.actions.splice(index, 1)
    return true
  }

  const toggleObject = (objectId: string) => {
    const object = getObject(objectId)
    if (!object) return false
    object.visible = !object.visible
    if (!object.visible && state.selectedObjectId === objectId) state.selectedObjectId = null
    return true
  }

  const isPersistentObject = (objectId: string) => !!state.persistentObjects[objectId]
  const resetCamera = () => Object.assign(state.camera, { x: 0, y: 0, zoom: 1 })
  const getSnapshot = () => clone(state)
  const applyScene = (sceneId: string) => {
    if (!state.scenes[sceneId]) return false
    selectScene(sceneId)
    return true
  }
  const replaceState = (next: StageWorkspaceState) => {
    const value = clone(next)
    state.activeSceneId = value.activeSceneId
    state.liveState = value.liveState
    state.scenes = value.scenes
    state.persistentObjects = value.persistentObjects
    state.camera = value.camera
    state.selectedObjectId = value.selectedObjectId
  }

  watch(() => state.liveState, saveLiveState, { deep: true, flush: 'sync' })

  return {
    state,
    scenes,
    activeScene,
    activeObjects,
    selectScene,
    addScene,
    duplicateScene,
    removeScene,
    addObject,
    removeSelectedObject,
    setParent,
    moveOrder,
    reorderObject,
    setSceneImage,
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
