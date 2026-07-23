import { watch, type WatchStopHandle } from 'vue'

import { api } from '@/stores/_config'
import { chatEvent } from '@/stores/chat'
import type { StageActionTriggeredPayload, StageDrawing, StageImageRef, StageLiveState, StageObject, StageObjectType, StagePointerTrace, StagePointerTraceInput, StageScene, StageSurfaceFit, StageWorkspaceState } from '../shared/stage-types'
import { isSafeStageImageUrl, normalizeStageSurfaceStyle } from '../shared/stage-types'
import { createInitialTheaterStageState, type TheaterStageStore } from '../stage/StageStore'
import { stageActionSchema } from '../bridge/theater-bridge-protocol'

type JsonObject = Record<string, unknown>

interface TheaterObjectSnapshot {
  id: string
  sceneId?: string | null
  parentId?: string | null
  kind: string
  name: string
  x: number
  y: number
  width: number
  height: number
  rotation: number
  scale?: number
  scaleX?: number
  scaleY?: number
  z: number
  orderKey: string
  visible: boolean
  locked: boolean
  aspectRatioLocked?: boolean
  interactive: boolean
  editable: boolean
  ownerUserId?: string | null
  characterIdentityId?: string | null
  content: JsonObject
  actions: unknown[]
  metadata: JsonObject
}

interface TheaterSceneSnapshot {
  id: string
  name: string
  switchText: string
  order: number
  locked: boolean
  state: JsonObject
  objects: Record<string, TheaterObjectSnapshot>
}

interface TheaterDocument {
  activeSceneId: string | null
  liveState: JsonObject
  scenes: Record<string, TheaterSceneSnapshot>
  persistentObjects: Record<string, TheaterObjectSnapshot>
}

interface TheaterSnapshotResponse {
  revision: number
  schemaVersion: number
  permissions: string[]
  snapshot: {
    activeSceneId?: string | null
    liveState?: JsonObject
    scenes?: Record<string, TheaterSceneSnapshot>
    persistentObjects?: Record<string, TheaterObjectSnapshot>
  }
}

interface TheaterMutation {
  type: string
  payload: JsonObject
  permission: 'stage.object.edit' | 'stage.scene.switch'
}

interface TheaterSyncOptions {
  worldId: string
  channelId: string
  inputChannelId?: string
  scopeType?: 'channel' | 'world'
  store: TheaterStageStore
  sendGatewayAPI: (apiName: string, data: Record<string, unknown>) => Promise<any>
  onPermissionsChange?: (permissions: string[]) => void
  onSyncingChange?: (syncing: boolean) => void
  onPreloadRequested?: (sceneIds: string[], requestId: string) => void
  onPointerTrace?: (trace: StagePointerTrace) => void
  onEffectTriggered?: (effectId: string, triggerId: string) => void
  onError?: (message: string) => void
}

const clone = <T>(value: T): T => structuredClone(value)
const asObject = (value: unknown): JsonObject => value && typeof value === 'object' && !Array.isArray(value)
  ? value as JsonObject
  : {}
const finite = (value: unknown, fallback: number) => Number.isFinite(value) ? Number(value) : fallback
const same = (left: unknown, right: unknown) => JSON.stringify(left) === JSON.stringify(right)
const mutationId = (prefix: string) => `${prefix}-${typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`}`
const objectBatchUpdateLimit = 200
const normalizeSwitchText = (value: unknown) => typeof value === 'string'
  ? Array.from(value).slice(0, 10_000).join('')
  : ''

const isRecord = (value: unknown): value is JsonObject => Boolean(value) && typeof value === 'object' && !Array.isArray(value)

const mergeThreeWay = (base: unknown, local: unknown, remote: unknown): unknown => {
  if (same(local, base)) return clone(remote)
  if (same(remote, base) || local === undefined) return clone(local)
  if (!isRecord(local) || !isRecord(remote)) return clone(local)
  const baseRecord = isRecord(base) ? base : {}
  const result: JsonObject = {}
  const keys = new Set([...Object.keys(baseRecord), ...Object.keys(local), ...Object.keys(remote)])
  keys.forEach((key) => {
    const value = mergeThreeWay(baseRecord[key], local[key], remote[key])
    if (value !== undefined) result[key] = value
  })
  return result
}

const rebaseDocument = (base: TheaterDocument, local: TheaterDocument, remote: TheaterDocument): TheaterDocument => (
  mergeThreeWay(base, local, remote) as TheaterDocument
)

const imageRef = (value: unknown): StageImageRef | null => {
  const raw = asObject(value)
  const url = typeof raw.url === 'string' ? raw.url.trim() : ''
  if (!url || !isSafeStageImageUrl(url)) return null
  return {
    resourceId: typeof raw.resourceId === 'string' && raw.resourceId.trim() ? raw.resourceId.trim() : `resource-${url}`,
    url,
    ...(typeof raw.alt === 'string' ? { alt: raw.alt } : {}),
    ...(typeof raw.mimeType === 'string' && raw.mimeType.trim() ? { mimeType: raw.mimeType.trim().toLowerCase() } : {}),
    ...(raw.animated === true ? { animated: true } : {}),
    ...(Number.isInteger(raw.loopCount) && Number(raw.loopCount) > 0 && Number(raw.loopCount) <= 65_535 ? { loopCount: Number(raw.loopCount) } : {}),
  }
}

const drawingRef = (value: unknown): StageDrawing | undefined => {
  const raw = asObject(value)
  const style = asObject(raw.style)
  const tools: StageDrawing['tool'][] = ['pen', 'highlighter', 'line', 'arrow', 'rectangle', 'ellipse', 'triangle', 'polygon']
  const tool = tools.includes(raw.tool as StageDrawing['tool']) ? raw.tool as StageDrawing['tool'] : null
  if (!tool) return undefined
  const points = Array.isArray(raw.points)
    ? raw.points.filter((point): point is number => typeof point === 'number' && Number.isFinite(point)).slice(0, 2_000)
    : undefined
  if ((tool === 'pen' || tool === 'highlighter') && (!points || points.length < 2 || points.length % 2 !== 0)) return undefined
  return {
    tool,
    style: {
      stroke: typeof style.stroke === 'string' ? style.stroke : '#f8fafc',
      strokeWidth: Math.min(128, Math.max(1, finite(style.strokeWidth, 4))),
      opacity: Math.min(1, Math.max(0.05, finite(style.opacity, 1))),
      fill: typeof style.fill === 'string' ? style.fill : null,
      dash: style.dash === 'dashed' || style.dash === 'dotted' ? style.dash : 'solid',
    },
    ...(points ? { points } : {}),
    ...(tool === 'polygon' ? { sides: Math.min(12, Math.max(5, Math.round(finite(raw.sides, 6)))) } : {}),
    ...(tool === 'pen' || tool === 'highlighter'
      ? { smoothing: Math.min(1, Math.max(0, finite(raw.smoothing, 0.35))) }
      : {}),
  }
}

const stageStateFromServer = (value: unknown, objects: Record<string, StageObject>): StageLiveState => {
  const raw = asObject(value)
  const grid = asObject(raw.grid)
  const surfaceStyles = asObject(raw.surfaceStyles)
  const transition = asObject(raw.transition)
  const legacyFit: StageSurfaceFit = grid.objectFit === 'fill' || grid.objectFit === 'contain' ? grid.objectFit : 'cover'
  return {
    background: imageRef(raw.background),
    foreground: imageRef(raw.foreground),
    surfaceStyles: {
      background: normalizeStageSurfaceStyle(surfaceStyles.background, legacyFit, { opacity: 0.9, blurPx: 10 }),
      foreground: normalizeStageSurfaceStyle(surfaceStyles.foreground, legacyFit),
    },
    backgroundColor: typeof grid.backgroundColor === 'string' ? grid.backgroundColor : '#111827',
    fieldWidth: Math.max(1, finite(raw.fieldWidth, 40)),
    fieldHeight: Math.max(1, finite(raw.fieldHeight, 24)),
    fieldObjectFit: grid.objectFit === 'fill' || grid.objectFit === 'contain' ? grid.objectFit : 'cover',
    displayGrid: grid.display === true,
    gridSize: Math.max(0.01, finite(grid.size, 1)),
    alignWithGrid: grid.align === true,
    sceneObjects: objects,
    transition: {
      type: transition.type === 'crossfade' ? 'crossfade' : 'none',
      durationMs: Math.max(0, finite(transition.durationMs, 0)),
    },
    serverState: clone(raw),
  }
}

const serverStateFromStage = (state: StageLiveState): JsonObject => ({
  ...asObject(state.serverState),
  background: state.background,
  foreground: state.foreground,
  surfaceStyles: clone(state.surfaceStyles),
  fieldWidth: state.fieldWidth,
  fieldHeight: state.fieldHeight,
  grid: {
    backgroundColor: state.backgroundColor,
    objectFit: state.fieldObjectFit,
    display: state.displayGrid,
    size: state.gridSize,
    align: state.alignWithGrid,
  },
  transition: state.transition,
})

const objectFromServer = (value: TheaterObjectSnapshot): StageObject | null => {
  const content = asObject(value.content)
  const legacyScale = finite(value.scale, 1) > 0 ? Math.min(100, finite(value.scale, 1)) : 1
  const kind = ['group', 'drawing', 'text', 'image', 'button', 'character', 'video', 'effect'].includes(value.kind)
    ? value.kind as StageObjectType
    : null
  if (!kind) return null
  const drawing = kind === 'drawing' ? drawingRef(content.drawing) : undefined
  if (kind === 'drawing' && !drawing) return null
  const structuralGroup = kind === 'group'
  return {
    id: value.id,
    parentId: typeof value.parentId === 'string' && value.parentId ? value.parentId : null,
    type: kind,
    name: value.name || '未命名对象',
    transform: {
      x: finite(value.x, 0),
      y: finite(value.y, 0),
      width: finite(value.width, 1),
      height: finite(value.height, 1),
      rotation: finite(value.rotation, 0),
      scaleX: finite(value.scaleX, legacyScale) > 0 ? Math.min(100, finite(value.scaleX, legacyScale)) : legacyScale,
      scaleY: finite(value.scaleY, legacyScale) > 0 ? Math.min(100, finite(value.scaleY, legacyScale)) : legacyScale,
      z: finite(value.z, 0),
      order: finite(Number.parseFloat(value.orderKey), 0),
    },
    visible: value.visible !== false,
    locked: value.locked === true,
    aspectRatioLocked: value.aspectRatioLocked !== false,
    interactive: structuralGroup ? false : value.interactive !== false,
    editable: structuralGroup ? false : value.editable === true,
    fill: typeof content.fill === 'string' ? content.fill : '#60a5fa',
    drawing,
    ...(typeof content.text === 'string' ? { text: content.text } : {}),
    ...(imageRef(content.image) ? { image: imageRef(content.image)! } : {}),
    content,
    ownerUserId: typeof value.ownerUserId === 'string' ? value.ownerUserId : null,
    characterIdentityId: typeof value.characterIdentityId === 'string' ? value.characterIdentityId : null,
    actions: structuralGroup ? [] : Array.isArray(value.actions) ? value.actions as StageObject['actions'] : [],
    metadata: asObject(value.metadata),
  }
}

const objectForServer = (object: StageObject, sceneId: string | null): TheaterObjectSnapshot => ({
  id: object.id,
  sceneId,
  parentId: object.parentId,
  kind: object.type,
  name: object.name,
  x: object.transform.x,
  y: object.transform.y,
  width: object.transform.width,
  height: object.transform.height,
  rotation: object.transform.rotation,
  scaleX: object.transform.scaleX,
  scaleY: object.transform.scaleY,
  z: object.transform.z,
  orderKey: String(object.transform.order),
  visible: object.visible,
  locked: object.locked,
  aspectRatioLocked: object.aspectRatioLocked,
  interactive: object.type === 'group' ? false : object.interactive,
  editable: object.type === 'group' ? false : object.editable,
  ownerUserId: object.ownerUserId || null,
  characterIdentityId: object.characterIdentityId || null,
  content: {
    ...asObject(object.content),
    fill: object.fill,
    ...(object.text === undefined ? {} : { text: object.text }),
    ...(object.image === undefined ? {} : { image: object.image }),
    ...(object.drawing === undefined ? {} : { drawing: object.drawing }),
  },
  actions: object.type === 'group' ? [] : clone(object.actions),
  metadata: clone(object.metadata),
})

const normalizeDocument = (snapshot: TheaterSnapshotResponse['snapshot']): TheaterDocument => ({
  activeSceneId: typeof snapshot.activeSceneId === 'string' && snapshot.activeSceneId ? snapshot.activeSceneId : null,
  liveState: serverStateFromStage(stageStateFromServer(snapshot.liveState, {})),
  scenes: Object.fromEntries(Object.entries(snapshot.scenes || {}).map(([id, scene]) => [id, {
    ...scene,
    id,
    switchText: normalizeSwitchText(scene.switchText),
    state: serverStateFromStage(stageStateFromServer(scene.state, {})),
    objects: scene.objects || {},
  }])),
  persistentObjects: snapshot.persistentObjects || {},
})

const documentFromWorkspace = (workspace: StageWorkspaceState): TheaterDocument => ({
  activeSceneId: workspace.activeSceneId || null,
  liveState: serverStateFromStage(workspace.liveState),
  scenes: Object.fromEntries(Object.values(workspace.scenes).map((scene) => [scene.id, {
    id: scene.id,
    name: scene.name,
    switchText: scene.switchText,
    order: scene.order,
    locked: scene.locked,
    state: serverStateFromStage(scene.state),
    objects: Object.fromEntries(Object.values(scene.state.sceneObjects).map((object) => [
      object.id,
      objectForServer(object, scene.id),
    ])),
  }])),
  persistentObjects: Object.fromEntries(Object.values(workspace.persistentObjects).map((object) => [
    object.id,
    objectForServer(object, null),
  ])),
})

const workspaceFromDocument = (document: TheaterDocument): StageWorkspaceState => {
  if (!Object.keys(document.scenes).length) return createInitialTheaterStageState()
  const scenes = Object.fromEntries(Object.values(document.scenes).map((scene) => {
    const objects = Object.fromEntries(Object.values(scene.objects).flatMap((object) => {
      const parsed = objectFromServer(object)
      return parsed ? [[object.id, parsed]] : []
    }))
    const value: StageScene = {
      id: scene.id,
      name: scene.name,
      switchText: scene.switchText,
      order: scene.order,
      locked: scene.locked,
      state: stageStateFromServer(scene.state, objects),
    }
    return [scene.id, value]
  }))
  const activeSceneId = document.activeSceneId && scenes[document.activeSceneId]
    ? document.activeSceneId
    : Object.values(scenes).sort((a, b) => a.order - b.order)[0].id
  const persistentObjects = Object.fromEntries(Object.values(document.persistentObjects).flatMap((object) => {
    const parsed = objectFromServer(object)
    return parsed ? [[object.id, parsed]] : []
  }))
  return {
    activeSceneId,
    liveState: clone(scenes[activeSceneId].state),
    scenes,
    persistentObjects,
    camera: { x: 0, y: 0, zoom: 0.5 },
    selectedObjectId: null,
  }
}

const objectFields = (object: TheaterObjectSnapshot, previous: TheaterObjectSnapshot): JsonObject => {
  const values: JsonObject = {
    sceneId: object.sceneId || '',
    parentId: object.parentId || '',
    name: object.name,
    x: object.x,
    y: object.y,
    width: object.width,
    height: object.height,
    rotation: object.rotation,
    scaleX: object.scaleX ?? object.scale ?? 1,
    scaleY: object.scaleY ?? object.scale ?? 1,
    z: object.z,
    orderKey: object.orderKey,
    visible: object.visible,
    locked: object.locked,
    aspectRatioLocked: object.aspectRatioLocked,
    interactive: object.interactive,
    editable: object.editable,
    content: object.content,
    actions: object.actions,
    metadata: object.metadata,
  }
  const previousValues: JsonObject = {
    sceneId: previous.sceneId || '',
    parentId: previous.parentId || '',
    name: previous.name,
    x: previous.x,
    y: previous.y,
    width: previous.width,
    height: previous.height,
    rotation: previous.rotation,
    scaleX: previous.scaleX ?? previous.scale ?? 1,
    scaleY: previous.scaleY ?? previous.scale ?? 1,
    z: previous.z,
    orderKey: previous.orderKey,
    visible: previous.visible,
    locked: previous.locked,
    aspectRatioLocked: previous.aspectRatioLocked,
    interactive: previous.interactive,
    editable: previous.editable,
    content: previous.content,
    actions: previous.actions,
    metadata: previous.metadata,
  }
  return Object.fromEntries(Object.entries(values).filter(([key, value]) => !same(value, previousValues[key])))
}

const objectInput = (object: TheaterObjectSnapshot): JsonObject => ({
  id: object.id,
  parentId: object.parentId || null,
  kind: object.kind,
  name: object.name,
  x: object.x,
  y: object.y,
  width: object.width,
  height: object.height,
  rotation: object.rotation,
  scaleX: object.scaleX ?? object.scale ?? 1,
  scaleY: object.scaleY ?? object.scale ?? 1,
  z: object.z,
  orderKey: object.orderKey,
  visible: object.visible,
  locked: object.locked,
  aspectRatioLocked: object.aspectRatioLocked,
  interactive: object.interactive,
  editable: object.editable,
  ownerUserId: object.ownerUserId || null,
  characterIdentityId: object.characterIdentityId || null,
  content: object.content,
  actions: object.actions,
  metadata: object.metadata,
})

const allObjects = (document: TheaterDocument) => {
  const result: Record<string, TheaterObjectSnapshot> = { ...document.persistentObjects }
  Object.values(document.scenes).forEach((scene) => Object.assign(result, scene.objects))
  return result
}

const sortObjectsByParent = (objects: TheaterObjectSnapshot[]) => {
  const byId = new Map(objects.map((object) => [object.id, object]))
  const depth = (object: TheaterObjectSnapshot, seen = new Set<string>()): number => {
    if (!object.parentId || seen.has(object.id)) return 0
    const parent = byId.get(object.parentId)
    if (!parent) return 0
    seen.add(object.id)
    return depth(parent, seen) + 1
  }
  return [...objects].sort((left, right) => depth(left) - depth(right))
}

const diffDocuments = (before: TheaterDocument, after: TheaterDocument): TheaterMutation[] => {
  const mutations: TheaterMutation[] = []
  const beforeObjects = allObjects(before)
  const afterObjects = allObjects(after)

  Object.values(after.scenes)
    .filter((scene) => !before.scenes[scene.id])
    .sort((left, right) => left.order - right.order)
    .forEach((scene) => mutations.push({
      type: 'scene.create',
      permission: 'stage.object.edit',
      payload: { sceneId: scene.id, name: scene.name, switchText: scene.switchText, order: scene.order, state: scene.state },
    }))

  Object.values(after.scenes).forEach((scene) => {
    const previous = before.scenes[scene.id]
    if (!previous) return
    const fields: JsonObject = {}
    if (scene.name !== previous.name) fields.name = scene.name
    if (scene.switchText !== previous.switchText) fields.switchText = scene.switchText
    if (scene.order !== previous.order) fields.order = scene.order
    if (scene.locked !== previous.locked) fields.locked = scene.locked
    if (!same(scene.state, previous.state)) fields.state = scene.state
    if (Object.keys(fields).length) mutations.push({
      type: 'scene.update',
      permission: 'stage.object.edit',
      payload: { sceneId: scene.id, fields },
    })
  })

  sortObjectsByParent(Object.values(afterObjects).filter((object) => !beforeObjects[object.id])).forEach((object) => mutations.push({
    type: 'object.create',
    permission: 'stage.object.edit',
    payload: { sceneId: object.sceneId || null, object: objectInput(object) },
  }))

  const objectUpdates: { objectId: string, fields: JsonObject }[] = []
  sortObjectsByParent(Object.values(afterObjects)).forEach((object) => {
    const previous = beforeObjects[object.id]
    if (!previous) return
    const fields = objectFields(object, previous)
    if (!Object.keys(fields).length) return
    objectUpdates.push({ objectId: object.id, fields })
  })
  if (objectUpdates.length === 1) {
    mutations.push({
      type: 'object.update',
      permission: 'stage.object.edit',
      payload: objectUpdates[0],
    })
  } else if (objectUpdates.length > 1) {
    for (let index = 0; index < objectUpdates.length; index += objectBatchUpdateLimit) {
      const updates = objectUpdates.slice(index, index + objectBatchUpdateLimit)
      mutations.push(updates.length === 1 ? {
        type: 'object.update',
        permission: 'stage.object.edit',
        payload: updates[0],
      } : {
        type: 'object.batchUpdate',
        permission: 'stage.object.edit',
        payload: { updates },
      })
    }
  }

  const removedObjectIds = new Set(Object.keys(beforeObjects).filter((id) => !afterObjects[id]))
  Object.values(beforeObjects)
    .filter((object) => removedObjectIds.has(object.id) && (!object.parentId || !removedObjectIds.has(object.parentId)))
    .filter((object) => !object.sceneId || Boolean(after.scenes[object.sceneId]))
    .forEach((object) => mutations.push({
      type: 'object.delete',
      permission: 'stage.object.edit',
      payload: { objectId: object.id, cascade: true },
    }))

  Object.values(before.scenes)
    .filter((scene) => !after.scenes[scene.id])
    .forEach((scene) => mutations.push({
      type: 'scene.delete',
      permission: 'stage.object.edit',
      payload: { sceneId: scene.id, fallbackSceneId: after.activeSceneId || '' },
    }))

  if (after.activeSceneId && after.activeSceneId !== before.activeSceneId) mutations.push({
    type: 'scene.apply',
    permission: 'stage.scene.switch',
    payload: { sceneId: after.activeSceneId },
  })
  return mutations
}

const delegatedObjectFields = new Set([
  'name', 'x', 'y', 'width', 'height', 'rotation', 'scaleX', 'scaleY', 'z', 'orderKey', 'content',
])

const canApplyMutation = (mutation: TheaterMutation, permissions: string[], baseDocument: TheaterDocument) => {
  if (permissions.includes(mutation.permission)) return true
  if (mutation.type !== 'object.update' || !permissions.includes('stage.object.edit.delegated')) return false
  const objectId = typeof mutation.payload.objectId === 'string' ? mutation.payload.objectId : ''
  const object = allObjects(baseDocument)[objectId]
  const fields = asObject(mutation.payload.fields)
  return Boolean(object?.editable && !object.locked)
    && Object.keys(fields).every((field) => delegatedObjectFields.has(field))
}

const errorMessage = (error: unknown) => {
  const value = error as any
  return value?.response?.data?.error?.message || value?.message || '小剧场同步失败'
}

const isRevisionConflict = (error: unknown) => {
  const value = error as any
  return value?.response?.status === 409
    && value?.response?.data?.error?.code === 'STAGE_REVISION_CONFLICT'
}

const isPermissionDenied = (error: unknown) => {
  const value = error as any
  return value?.response?.status === 403
    || value?.response?.data?.error?.code === 'STAGE_PERMISSION_DENIED'
}

export class TheaterSyncClient {
  private revision = 0
  private schemaVersion = 1
  private permissions: string[] = []
  private baseDocument: TheaterDocument = normalizeDocument({})
  private stopWatch: WatchStopHandle | null = null
  private flushTimer: ReturnType<typeof setTimeout> | null = null
  private reconcileTimer: ReturnType<typeof setInterval> | null = null
  private started = false
  private applyingRemote = false
  private saving = false
  private flushAgain = false
  private pendingRemoteRevision = 0
  private hasLoaded = false
  // State mutations must retain revision order. Message and composer actions do
  // not mutate the theater document, so keeping them out of this queue lets a
  // single click start all of its effects together.
  private mutationActionQueue: Promise<void> = Promise.resolve()
  private consecutiveConflicts = 0

  private theaterBase() {
    if (this.options.scopeType === 'world' || !this.options.channelId) {
      return `api/v1/worlds/${encodeURIComponent(this.options.worldId)}/theater`
    }
    return `api/v1/worlds/${encodeURIComponent(this.options.worldId)}/channels/${encodeURIComponent(this.options.channelId)}/theater`
  }

  private readonly onGatewayEvent = (event: any) => {
    const theater = event?.theater
    if (!theater || theater.worldId !== this.options.worldId || (this.options.scopeType !== 'world' && theater.channelId !== this.options.channelId)) return
    const revision = finite(theater.revision, 0)
    if (revision <= this.revision) return
    if (this.saving) {
      this.pendingRemoteRevision = Math.max(this.pendingRemoteRevision, revision)
      return
    }
    if (this.flushTimer) {
      this.pendingRemoteRevision = Math.max(this.pendingRemoteRevision, revision)
      void this.flushNow()
      return
    }
    void this.reload()
  }

  private readonly onPreloadRequested = (event: any) => {
    const theater = event?.theater
    if (!theater || theater.worldId !== this.options.worldId || (this.options.scopeType !== 'world' && theater.channelId !== this.options.channelId)) return
    const payload = asObject(theater.payload)
    const sceneIds = Array.isArray(payload.sceneIds)
      ? [...new Set(payload.sceneIds.filter((sceneId): sceneId is string => typeof sceneId === 'string' && Boolean(sceneId.trim())).map((sceneId) => sceneId.trim()))]
      : []
    if (!sceneIds.length) return
    this.options.onPreloadRequested?.(sceneIds, typeof payload.requestId === 'string' ? payload.requestId : '')
  }

  private readonly onPointerTrace = (event: any) => {
    const theater = event?.theater
    if (!theater || theater.worldId !== this.options.worldId || (this.options.scopeType !== 'world' && theater.channelId !== this.options.channelId)) return
    const payload = asObject(theater.payload)
    const traceId = typeof payload.traceId === 'string' ? payload.traceId.trim() : ''
    const displayName = typeof payload.displayName === 'string' ? payload.displayName.trim() : ''
    const color = typeof payload.color === 'string' ? payload.color.trim() : ''
    const points = Array.isArray(payload.points)
      ? payload.points.filter((point): point is number => typeof point === 'number' && Number.isFinite(point)).slice(0, 128)
      : []
    if (!traceId || !displayName || !color || points.length < 2 || points.length % 2 !== 0) return
    this.options.onPointerTrace?.({ traceId, displayName, color, points, finished: payload.finished === true })
  }

  private readonly onEffectTriggered = (event: any) => {
    const theater = event?.theater
    if (!theater || theater.worldId !== this.options.worldId || (this.options.scopeType !== 'world' && theater.channelId !== this.options.channelId)) return
    const payload = asObject(theater.payload)
    const effectId = typeof payload.effectId === 'string' ? payload.effectId.trim() : ''
    const triggerId = typeof payload.triggerId === 'string' ? payload.triggerId.trim() : ''
    if (!effectId || !triggerId) return
    this.options.onEffectTriggered?.(effectId, triggerId)
  }

  private readonly onGatewayConnected = () => {
    void this.subscribe()
  }

  constructor(private readonly options: TheaterSyncOptions) {}

  async start() {
    if (this.started) return
    this.started = true
    chatEvent.on('theater.snapshot' as any, this.onGatewayEvent)
    chatEvent.on('theater.mutation.applied' as any, this.onGatewayEvent)
    chatEvent.on('theater.mutation.rejected' as any, this.onGatewayEvent)
    chatEvent.on('theater.preload.requested' as any, this.onPreloadRequested)
    chatEvent.on('theater.pointer.trace' as any, this.onPointerTrace)
    chatEvent.on('theater.effect.triggered' as any, this.onEffectTriggered)
    chatEvent.on('connected' as any, this.onGatewayConnected)
    await this.reload()
    if (!this.started) return
    this.stopWatch = watch(() => [
      this.options.store.state.activeSceneId,
      this.options.store.state.liveState,
      this.options.store.state.scenes,
      this.options.store.state.persistentObjects,
    ], () => this.scheduleFlush(), { deep: true, flush: 'sync' })
    await this.subscribe()
    if (!this.started) return
    if (!Object.keys(this.baseDocument.scenes).length && this.permissions.includes('stage.object.edit')) this.scheduleFlush(0)
    this.reconcileTimer = setInterval(() => { void this.reloadIfIdle() }, 30_000)
  }

  async stop() {
    if (!this.started) return
    this.started = false
    this.stopWatch?.()
    this.stopWatch = null
    if (this.flushTimer) clearTimeout(this.flushTimer)
    if (this.reconcileTimer) clearInterval(this.reconcileTimer)
    this.flushTimer = null
    this.reconcileTimer = null
    chatEvent.off('theater.snapshot' as any, this.onGatewayEvent)
    chatEvent.off('theater.mutation.applied' as any, this.onGatewayEvent)
    chatEvent.off('theater.mutation.rejected' as any, this.onGatewayEvent)
    chatEvent.off('theater.preload.requested' as any, this.onPreloadRequested)
    chatEvent.off('theater.pointer.trace' as any, this.onPointerTrace)
    chatEvent.off('theater.effect.triggered' as any, this.onEffectTriggered)
    chatEvent.off('connected' as any, this.onGatewayConnected)
    try {
      await this.options.sendGatewayAPI('theater.unsubscribe', {})
    } catch {
      // Connection may already be closed while leaving theater.
    }
  }

  async triggerAction(payload: StageActionTriggeredPayload) {
    if (payload.action.type !== 'scene.apply' && payload.action.type !== 'object.toggle') {
      return this.triggerActionNow(payload)
    }
    const previous = this.mutationActionQueue
    let release!: () => void
    this.mutationActionQueue = new Promise<void>((resolve) => { release = resolve })
    await previous
    try {
      return await this.triggerActionNow(payload)
    } finally {
      release()
    }
  }

  async triggerActionBatch(payloads: readonly StageActionTriggeredPayload[]) {
    if (!payloads.length || !payloads.every((payload) => payload.action.type === 'object.toggle')) return false
    const first = payloads[0]
    const previous = this.mutationActionQueue
    let release!: () => void
    this.mutationActionQueue = new Promise<void>((resolve) => { release = resolve })
    await previous
    try {
      await this.waitForSaving()
      await this.flushNow()
      await this.waitForSaving()
      let response
      try {
        response = await this.postActionBatch(first, payloads)
      } catch (error) {
        if (isPermissionDenied(error)) {
          await this.reload(true, undefined, true)
          return true
        }
        if (!isRevisionConflict(error)) throw error
        await this.reload(true)
        response = await this.postActionBatch(first, payloads)
      }
      if (response.data?.result?.mutation?.revision !== undefined) {
        this.revision = finite(response.data.result.mutation.revision, this.revision)
      }
      await this.reload(true)
      return true
    } finally {
      release()
    }
  }

  async requestPreload(sceneIds: string[]) {
    const normalized = [...new Set(sceneIds.map((sceneId) => sceneId.trim()).filter(Boolean))]
    if (!normalized.length) return
    await this.options.sendGatewayAPI('theater.preload', {
      worldId: this.options.worldId,
      channelId: this.options.scopeType === 'world' ? '' : this.options.channelId,
      requestId: mutationId('preload'),
      sceneIds: normalized,
    })
  }

  async publishPointerTrace(trace: StagePointerTraceInput) {
    await this.options.sendGatewayAPI('theater.pointer', {
      worldId: this.options.worldId,
      channelId: this.options.scopeType === 'world' ? '' : this.options.channelId,
      inputChannelId: this.options.inputChannelId || this.options.channelId,
      traceId: trace.traceId,
      identityId: trace.identityId,
      variantId: trace.variantId || '',
      points: trace.points,
      finished: trace.finished,
    })
  }

  private async triggerActionNow(payload: StageActionTriggeredPayload) {
    await this.waitForSaving()
    await this.flushNow()
    await this.waitForSaving()
    let response
    try {
      response = await this.postAction(payload)
    } catch (error) {
      if (isPermissionDenied(error)) {
        await this.reload(true, undefined, true)
        return true
      }
      if (!isRevisionConflict(error)) throw error
      await this.reload(true)
      try {
        response = await this.postAction(payload)
      } catch (retryError) {
        if (isPermissionDenied(retryError)) {
          await this.reload(true, undefined, true)
          return true
        }
        throw retryError
      }
    }
    const result = response.data?.result
    if (result?.mutation?.revision !== undefined) this.revision = finite(result.mutation.revision, this.revision)
    if (result?.kind === 'mutation') await this.reload(true)
    if (result?.kind === 'local') {
      const action = stageActionSchema.safeParse({
        ...payload.action,
        payload: result.descriptor,
      })
      return action.success ? action.data : true
    }
    return true
  }

  private postAction(payload: StageActionTriggeredPayload) {
    return api.post(`${this.theaterBase()}/actions/trigger`, {
      actionRequestId: mutationId('action'),
      objectId: payload.objectId,
      actionId: payload.actionId,
      ...(payload.stepId ? { stepId: payload.stepId } : {}),
      inputChannelId: this.options.inputChannelId || this.options.channelId,
      expectedRevision: this.revision,
    })
  }

  private postActionBatch(first: StageActionTriggeredPayload, payloads: readonly StageActionTriggeredPayload[]) {
    return api.post(`${this.theaterBase()}/actions/trigger-batch`, {
      actionRequestId: mutationId('action-batch'),
      objectId: first.objectId,
      actionIds: payloads.map((payload) => payload.actionId),
      expectedRevision: this.revision,
    })
  }

  private async reloadIfIdle() {
    if (!this.saving && !this.flushTimer) await this.reload()
  }

  private async waitForSaving() {
    while (this.saving) await new Promise((resolve) => setTimeout(resolve, 20))
  }

  private async reload(force = false, localChange?: { base: TheaterDocument, desired: TheaterDocument }, silent = false) {
    if (!this.started) return
    try {
      const response = await api.get<TheaterSnapshotResponse>(this.theaterBase())
      if (!this.started) return
      const data = response.data
      const nextRevision = finite(data.revision, 0)
      this.schemaVersion = finite(data.schemaVersion, 1)
      this.permissions = Array.isArray(data.permissions) ? data.permissions.filter((item): item is string => typeof item === 'string') : []
      this.options.onPermissionsChange?.([...this.permissions])
      if (!force && this.hasLoaded && nextRevision === this.revision) return
      const remoteDocument = normalizeDocument(data.snapshot || {})
      const nextDocument = localChange
        ? rebaseDocument(localChange.base, localChange.desired, remoteDocument)
        : remoteDocument
      this.revision = nextRevision
      this.baseDocument = remoteDocument
      this.applyingRemote = true
      try {
        const selectedIds = [...this.options.store.selection.selectedIds]
        const primaryId = this.options.store.state.selectedObjectId
        const workspace = workspaceFromDocument(nextDocument)
        if (this.hasLoaded) workspace.camera = this.options.store.getSnapshot().camera
        if (localChange) {
          const current = this.options.store.getSnapshot()
          workspace.camera = current.camera
          workspace.selectedObjectId = current.selectedObjectId && (
            workspace.persistentObjects[current.selectedObjectId]
            || workspace.liveState.sceneObjects[current.selectedObjectId]
          ) ? current.selectedObjectId : null
        }
        this.options.store.replaceState(workspace)
        if (this.options.store.selection.bulkMode) {
          const validIds = selectedIds.filter((id) => (
            workspace.persistentObjects[id] || workspace.liveState.sceneObjects[id]
          ))
          this.options.store.setSelectedObjectIds(validIds, primaryId)
        }
      } finally {
        this.applyingRemote = false
      }
      this.hasLoaded = true
    } catch (error) {
      if (!this.started) return
      if (!silent) {
        this.options.onError?.(errorMessage(error))
        throw error
      }
    }
  }

  private async subscribe() {
    if (!this.started) return
    try {
      await this.options.sendGatewayAPI('theater.subscribe', {
        worldId: this.options.worldId,
        channelId: this.options.scopeType === 'world' ? '' : this.options.channelId,
        knownRevision: this.revision,
      })
    } catch (error) {
      this.options.onError?.(errorMessage(error))
    }
  }

  private scheduleFlush(delay = 350) {
    if (!this.started || this.applyingRemote) return
    if (this.flushTimer) clearTimeout(this.flushTimer)
    this.flushTimer = setTimeout(() => {
      this.flushTimer = null
      void this.flushNow()
    }, delay)
  }

  private async flushNow() {
    if (!this.started || this.applyingRemote) return
    if (this.flushTimer) {
      clearTimeout(this.flushTimer)
      this.flushTimer = null
    }
    if (this.saving) {
      this.flushAgain = true
      return
    }
    const desired = documentFromWorkspace(this.options.store.getSnapshot())
    const baseAtFlush = clone(this.baseDocument)
    const mutations = diffDocuments(this.baseDocument, desired)
    if (!mutations.length) {
      const shouldReload = this.pendingRemoteRevision > this.revision
      this.pendingRemoteRevision = 0
      if (shouldReload) await this.reload()
      return
    }
    const denied = mutations.find((mutation) => !canApplyMutation(mutation, this.permissions, this.baseDocument))
    if (denied) {
      await this.reload(true, undefined, true)
      return
    }
    this.saving = true
    this.options.onSyncingChange?.(true)
    try {
      for (const mutation of mutations) {
        if (!this.started) return
        const response = await api.post(`${this.theaterBase()}/mutations`, {
          mutationId: mutationId('mutation'),
          worldId: this.options.worldId,
          channelId: this.options.scopeType === 'world' ? '' : this.options.channelId,
          expectedRevision: this.revision,
          type: mutation.type,
          payload: mutation.payload,
        })
        if (!this.started) return
        this.revision = finite(response.data?.revision, this.revision + 1)
      }
      this.baseDocument = desired
      this.consecutiveConflicts = 0
    } catch (error) {
      if (!this.started) return
      const conflict = isRevisionConflict(error)
      const permissionDenied = isPermissionDenied(error)
      if (!conflict && !permissionDenied) this.options.onError?.(errorMessage(error))
      if (conflict) {
        this.flushAgain = false
        if (this.flushTimer) clearTimeout(this.flushTimer)
        this.flushTimer = null
        this.consecutiveConflicts += 1
        await this.reload(true, { base: baseAtFlush, desired })
        if (this.consecutiveConflicts <= 2) {
          this.flushAgain = true
        } else {
          this.options.onError?.('舞台状态持续冲突，本地修改已保留，请稍后重试')
        }
      } else {
        await this.reload(true, undefined, permissionDenied)
      }
    } finally {
      this.saving = false
      this.options.onSyncingChange?.(false)
      const shouldReload = this.pendingRemoteRevision > this.revision
      this.pendingRemoteRevision = 0
      const hasLocalChanges = this.flushAgain
        || Boolean(this.flushTimer)
        || diffDocuments(this.baseDocument, documentFromWorkspace(this.options.store.getSnapshot())).length > 0
      if (shouldReload && !hasLocalChanges) await this.reload()
      if (this.flushAgain) {
        this.flushAgain = false
        this.scheduleFlush(0)
      }
    }
  }
}

export const theaterSyncTesting = {
  canApplyMutation,
  diffDocuments,
  documentFromWorkspace,
  normalizeDocument,
  rebaseDocument,
  workspaceFromDocument,
}
