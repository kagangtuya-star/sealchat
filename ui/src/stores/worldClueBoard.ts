import { computed, reactive, ref, toRaw } from 'vue'
import { defineStore } from 'pinia'
import { api } from './_config'
import { useUserStore } from './user'
import type { Diff, Snapshot } from '@quickdrawjs/core'

export const WORLD_CLUE_BOARD_KEY = 'main'
export const WORLD_CLUE_BOARD_SCOPE = 'personal'
export type WorldClueBoardScope = 'personal' | 'shared'
export interface WorldClueBoardEventPayload {
  operation?: WorldClueBoardOperation
  worldId: string
  boardKey: string
  scope: WorldClueBoardScope
  revision: number
  updatedBy?: string
  clientId?: string
}
export type WorldClueBoardOperation = { id: string } & (
  | { type: 'placements.put'; placements: Record<string, WorldClueBoardPlacement> }
  | { type: 'relation.put'; relation: WorldClueBoardRelation }
  | { type: 'relation.remove'; relationId: string }
  | { type: 'quickdraw.diff'; quickdrawDiff: Diff }
)

function boardRuntimeID(): string {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID()
  const values = new Uint32Array(4)
  if (globalThis.crypto?.getRandomValues) globalThis.crypto.getRandomValues(values)
  else for (let i = 0; i < values.length; i++) values[i] = Math.floor(Math.random() * 0x100000000)
  return `${Date.now().toString(36)}-${Array.from(values, value => value.toString(16)).join('-')}`
}

function applyBoardOperation(document: WorldClueBoardDocument, operation: WorldClueBoardOperation): boolean {
  switch (operation.type) {
    case 'placements.put':
      if (!operation.placements || typeof operation.placements !== 'object' || Array.isArray(operation.placements)) return false
      for (const placement of Object.values(operation.placements)) {
        if (!placement || !Number.isFinite(placement.x) || !Number.isFinite(placement.y)) return false
      }
      Object.assign(document.placements, operation.placements)
      return true
    case 'relation.put': {
      if (!operation.relation?.id || !operation.relation.sourceRef || !operation.relation.targetRef) return false
      const index = document.relations.findIndex(item => item.id === operation.relation.id)
      if (index < 0) document.relations.push(operation.relation)
      else document.relations[index] = operation.relation
      return true
    }
    case 'relation.remove': document.relations = document.relations.filter(item => item.id !== operation.relationId); return true
    case 'quickdraw.diff': {
      document.quickdraw ||= { engineVersion: '@quickdrawjs/core@0.2.0', snapshot: { document: { store: {} } } }
      const snapshot = document.quickdraw.snapshot as Snapshot
      if (!snapshot?.document?.store) return false
      const records = snapshot.document.store
      const diff = operation.quickdrawDiff
      if (!diff?.added || !diff.removed || !diff.updated) return false
      for (const id of Object.keys(diff.removed)) delete records[id]
      for (const [id, pair] of Object.entries(diff.updated)) records[id] = pair[1]
      Object.assign(records, diff.added)
      return true
    }
    default: return false
  }
}
export const WORLD_CLUE_BOARD_VERSION = 1 as const
export const WORLD_CLUE_BOARD_MAX_BYTES = 8 * 1024 * 1024

export type WorldClueBoardRelationKind = 'related' | 'references' | 'supports' | 'contradicts' | 'causes'

export type WorldClueBoardRelationEndpointKind = 'clue' | 'quickdraw'

export interface WorldClueBoardRelationEndpointRef {
  kind: WorldClueBoardRelationEndpointKind
  id: string
}

export interface WorldClueBoardPlacement {
  x: number
  y: number
  width?: number
  pinned?: boolean
}

export interface WorldClueBoardRelation {
  id: string
  sourceRef: WorldClueBoardRelationEndpointRef
  targetRef: WorldClueBoardRelationEndpointRef
  kind: WorldClueBoardRelationKind
  label?: string
}

export const WORLD_CLUE_BOARD_RELATION_KIND_LABELS: Record<WorldClueBoardRelationKind, string> = {
  related: '相关',
  references: '引用',
  supports: '支持',
  contradicts: '矛盾',
  causes: '因果',
}

export interface WorldClueBoardQuickdraw {
  engineVersion: string
  snapshot: unknown
}

export interface WorldClueBoardDocument {
  version: typeof WORLD_CLUE_BOARD_VERSION
  placements: Record<string, WorldClueBoardPlacement>
  relations: WorldClueBoardRelation[]
  quickdraw: WorldClueBoardQuickdraw | null
}

export interface WorldClueBoardResponse {
  canWrite: boolean
  worldId: string
  boardKey: string
  scope: string
  ownerUserId: string
  revision: number
  document: WorldClueBoardDocument
  updatedAt: number
}

export interface WorldClueBoardWriteResult {
  worldId: string
  boardKey: string
  scope: string
  revision: number
  updatedAt: number
}

type BoardStatus = 'idle' | 'loading' | 'ready' | 'saving' | 'conflict' | 'error'
type BoardErrorKind = 'network' | 'too-large' | 'invalid' | 'conflict' | 'unknown' | null

export interface WorldClueBoardSession {
  clientId: string
  sharedQueue: WorldClueBoardOperation[]
  sharedSending: Promise<boolean> | null
  localDrawingPending: boolean
  snapshotVersion: number
  scope: WorldClueBoardScope
  canWrite: boolean
  needsResync: boolean
  remoteRevision: number
  resyncTimer: ReturnType<typeof setTimeout> | null
  pendingRemoteEvents: Map<number, WorldClueBoardEventPayload>
  remoteGapTimer: ReturnType<typeof setTimeout> | null
  key: string
  userId: string
  worldId: string
  boardKey: string
  document: WorldClueBoardDocument
  revision: number
  updatedAt: number
  status: BoardStatus
  error: string
  errorKind: BoardErrorKind
  loaded: boolean
  loadFailed: boolean
  dirty: boolean
  editVersion: number
  pendingSave: boolean
  canRetry: boolean
  epoch: number
  saveTimer: ReturnType<typeof setTimeout> | null
  inFlight: Promise<boolean> | null
}

const emptyDocument = (): WorldClueBoardDocument => ({
  version: WORLD_CLUE_BOARD_VERSION,
  placements: {},
  relations: [],
  quickdraw: null,
})

function cloneDocument(document: WorldClueBoardDocument): WorldClueBoardDocument {
  // structuredClone is not available in every supported embedded browser.
  return JSON.parse(JSON.stringify(toRaw(document))) as WorldClueBoardDocument
}

function utf8Bytes(document: WorldClueBoardDocument): number {
  return new TextEncoder().encode(JSON.stringify(document)).byteLength
}

// The API limit applies to the complete JSON PUT body, not only to the
// document field. Keep the document in memory when it is too large, but stop
// before issuing a request that the server must reject with 413.
function utf8RequestBytes(document: WorldClueBoardDocument, expectedRevision: number): number {
  return new TextEncoder().encode(JSON.stringify({
    scope: WORLD_CLUE_BOARD_SCOPE,
    expectedRevision,
    document,
  })).byteLength
}

function normalizeDocument(value: unknown): WorldClueBoardDocument {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('画板文档格式错误')
  const source = value as Record<string, unknown>
  if (source.version !== WORLD_CLUE_BOARD_VERSION) throw new Error('画板文档版本不受支持')
  if (!source.placements || typeof source.placements !== 'object' || Array.isArray(source.placements)) throw new Error('画板位置格式错误')
  if (!Array.isArray(source.relations)) throw new Error('画板关系格式错误')
  if (source.quickdraw !== null && (typeof source.quickdraw !== 'object' || Array.isArray(source.quickdraw))) throw new Error('Quickdraw 快照格式错误')
  const relations = (source.relations as unknown[]).map((value) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('画板关系格式错误')
    const relation = value as Record<string, unknown>
    const sourceRef = relation.sourceRef && typeof relation.sourceRef === 'object' && !Array.isArray(relation.sourceRef)
      ? relation.sourceRef as Record<string, unknown>
      : { kind: 'clue', id: relation.sourceClueId }
    const targetRef = relation.targetRef && typeof relation.targetRef === 'object' && !Array.isArray(relation.targetRef)
      ? relation.targetRef as Record<string, unknown>
      : { kind: 'clue', id: relation.targetClueId }
    const sourceId = typeof sourceRef.id === 'string' ? sourceRef.id : ''
    const targetId = typeof targetRef.id === 'string' ? targetRef.id : ''
    const sourceKind = sourceRef.kind === 'quickdraw' ? 'quickdraw' : 'clue'
    const targetKind = targetRef.kind === 'quickdraw' ? 'quickdraw' : 'clue'
    const id = typeof relation.id === 'string' ? relation.id : ''
    const kind = relation.kind as WorldClueBoardRelationKind
    const label = typeof relation.label === 'string' ? relation.label : ''
    return {
      id,
      sourceRef: { kind: sourceKind, id: sourceId },
      targetRef: { kind: targetKind, id: targetId },
      kind,
      ...(label ? { label } : {}),
    } as WorldClueBoardRelation
  })
  return cloneDocument({
    version: WORLD_CLUE_BOARD_VERSION,
    placements: source.placements as Record<string, WorldClueBoardPlacement>,
    relations,
    quickdraw: (source.quickdraw || null) as WorldClueBoardQuickdraw | null,
  })
}

function errorDetails(error: any): { message: string; kind: BoardErrorKind; retry: boolean } {
  const status = Number(error?.response?.status || 0)
  if (status === 409) return { message: '画板版本冲突：保留本地修改，请先重新载入或处理冲突', kind: 'conflict', retry: false }
  if (status === 413) return { message: '画板文档超过 8MiB，请删除部分内容后再保存', kind: 'too-large', retry: true }
  if (status >= 400 && status < 500) return { message: error?.response?.data?.message || '画板文档无法保存', kind: 'invalid', retry: false }
  if (error?.isAxiosError === true && !error?.response) return { message: '画板网络请求失败，请检查连接后重试', kind: 'network', retry: true }
  return { message: error?.message || '画板请求失败', kind: 'unknown', retry: true }
}

function documentErrorDetails(error: unknown): { message: string; kind: BoardErrorKind; retry: boolean } {
  const message = error instanceof Error && error.message.trim() ? error.message : '画板文档解析失败'
  return { message, kind: error instanceof Error ? 'invalid' : 'unknown', retry: false }
}

export const useWorldClueBoardStore = defineStore('worldClueBoard', () => {
  const user = useUserStore()
  const sessions = reactive<Record<string, WorldClueBoardSession>>({})
  const activeKey = ref('')
  let realtimeStopped = false
  const current = computed(() => (activeKey.value ? sessions[activeKey.value] || null : null))

  const makeKey = (userId: string, worldId: string, boardKey = WORLD_CLUE_BOARD_KEY, scope: WorldClueBoardScope = 'personal') => `${userId}:${worldId}:${boardKey}:${scope}`
  const currentUserId = () => String(user.info?.id || '').trim()

  function ensureSession(worldId: string, boardKey = WORLD_CLUE_BOARD_KEY, userId = currentUserId(), scope: WorldClueBoardScope = 'personal'): WorldClueBoardSession {
    const normalizedWorldId = String(worldId || '').trim()
    const normalizedBoardKey = String(boardKey || '').trim()
    const key = makeKey(userId, normalizedWorldId, normalizedBoardKey, scope)
    let session = sessions[key]
    if (!session) {
      session = reactive({
        key,
        scope,
        clientId: boardRuntimeID(),
        sharedQueue: [],
        sharedSending: null,
        localDrawingPending: false,
        snapshotVersion: 0,
        canWrite: false,
        needsResync: false,
        remoteRevision: 0,
        resyncTimer: null,
        pendingRemoteEvents: new Map<number, WorldClueBoardEventPayload>(),
        remoteGapTimer: null,
        userId,
        worldId: normalizedWorldId,
        boardKey: normalizedBoardKey,
        document: emptyDocument(),
        revision: 0,
        updatedAt: 0,
        status: 'idle' as BoardStatus,
        error: '',
        errorKind: null as BoardErrorKind,
        loaded: false,
        loadFailed: false,
        dirty: false,
        editVersion: 0,
        pendingSave: false,
        canRetry: false,
        epoch: 0,
        saveTimer: null,
        inFlight: null,
      }) as WorldClueBoardSession
      sessions[key] = session
    }
    activeKey.value = key
    return session
  }

  function isCurrent(session: WorldClueBoardSession, key: string, epoch: number) {
    return sessions[key] === session && session.key === activeKey.value && session.epoch === epoch
  }

  async function load(worldId: string, options?: { force?: boolean; surfaceInit?: boolean; userId?: string; scope?: WorldClueBoardScope }): Promise<WorldClueBoardSession> {
    const session = ensureSession(worldId, WORLD_CLUE_BOARD_KEY, options?.userId, options?.scope)
    if (!session.worldId || !session.userId) {
      session.status = 'error'
      session.error = '未登录或缺少世界上下文'
      session.loadFailed = true
      return session
    }
    if (options?.surfaceInit && session.scope === 'personal') {
      // A newly mounted surface must start from the server baseline, not from
      // transient state left by a previous personal surface instance.
      if (session.inFlight) await session.inFlight.catch(() => false)
      if (session.saveTimer) clearTimeout(session.saveTimer)
      session.saveTimer = null
      session.canWrite = false
      session.status = 'idle'
      session.error = ''
      session.errorKind = null
      session.loaded = false
      session.loadFailed = false
      session.dirty = false
      session.pendingSave = false
      session.canRetry = false
    }
    if (session.loaded && !options?.force && !options?.surfaceInit) return session
    // A focus refresh is intentionally skipped while the user has local work.
    if (session.dirty && options?.force) return session
    if (session.scope === 'shared' && session.localDrawingPending) { requestSharedRefresh(); return session }
    const epoch = ++session.epoch
    const requestedEditVersion = session.editVersion
    session.status = 'loading'
    session.error = ''
    session.errorKind = null
    let response: { data: WorldClueBoardResponse }
    try {
      response = await api.get(`api/v1/worlds/${encodeURIComponent(session.worldId)}/clue-boards/${encodeURIComponent(session.boardKey)}`, { params: { scope: session.scope } })
    } catch (error) {
      if (isCurrent(session, session.key, epoch)) {
        if (session.dirty || session.localDrawingPending || session.editVersion !== requestedEditVersion) {
          if (session.scope === 'shared') requestSharedRefresh()
          return session
        }
        const details = errorDetails(error)
        session.status = 'error'
        session.error = details.message
        session.errorKind = details.kind
        session.canRetry = details.retry
        session.loadFailed = !session.loaded
      }
      return session
    }
    if (!isCurrent(session, session.key, epoch)) return session
    const data = response?.data as WorldClueBoardResponse
    let document: WorldClueBoardDocument
    try {
      document = normalizeDocument(data?.document)
    } catch (error) {
      if (session.dirty || session.localDrawingPending || session.editVersion !== requestedEditVersion) {
        if (session.scope === 'shared') requestSharedRefresh()
        return session
      }
      const details = documentErrorDetails(error)
      session.status = 'error'
      session.error = details.message
      session.errorKind = details.kind
      session.canRetry = details.retry
      session.loadFailed = !session.loaded
      return session
    }
    // A focus/connected refresh must never replace an edit that started
    // while its GET was in flight. The local document remains dirty and the
    // normal save path keeps its original expected revision.
    if (session.dirty || session.localDrawingPending || session.editVersion !== requestedEditVersion) {
      if (session.scope === 'shared') requestSharedRefresh()
      return session
    }
    session.document = document
    session.snapshotVersion++
    session.canWrite = data.canWrite === true
    session.revision = Number(data.revision) || 0
    session.needsResync = session.remoteRevision > session.revision
    session.updatedAt = Number(data.updatedAt) || 0
    session.loaded = true
    session.loadFailed = false
    session.dirty = false
    session.pendingSave = false
    session.status = 'ready'
    session.canRetry = false
    if (session.needsResync) requestSharedRefresh()
    return session
  }

  function markChanged(session: WorldClueBoardSession, document: WorldClueBoardDocument) {
    if (!session.loaded || session.loadFailed || !session.canWrite || session.scope === 'shared') return false
    if (utf8Bytes(document) > WORLD_CLUE_BOARD_MAX_BYTES) {
      session.error = '画板文档超过 8MiB，请删除部分内容后再保存'
      session.errorKind = 'too-large'
      session.canRetry = false
      session.status = 'error'
      return false
    }
    const wasConflict = session.status === 'conflict'
    session.document = cloneDocument(document)
    session.editVersion += 1
    session.dirty = true
    session.pendingSave = true
    session.status = wasConflict ? 'conflict' : 'ready'
    if (!wasConflict) {
      session.error = ''
      session.errorKind = null
    }
    if (!wasConflict) scheduleSave(session)
    return true
  }

  function updateDocument(documentOrMutator: WorldClueBoardDocument | ((draft: WorldClueBoardDocument) => void)) {
    const session = current.value
    if (!session) return false
    const next = typeof documentOrMutator === 'function' ? cloneDocument(session.document) : documentOrMutator
    if (typeof documentOrMutator === 'function') documentOrMutator(next)
    return markChanged(session, next)
  }

  function scheduleSave(session = current.value) {
    if (session?.scope === 'shared') return
    if (!session || session.key !== activeKey.value || !session.loaded || session.loadFailed || !session.dirty || session.status === 'conflict') return
    if (session.saveTimer) clearTimeout(session.saveTimer)
    session.saveTimer = setTimeout(() => {
      session.saveTimer = null
      void save(session)
    }, 650)
  }

  async function save(session = current.value): Promise<boolean> {
    if (session?.scope === 'shared') return drainSharedQueue(session)
    if (!session || session.key !== activeKey.value || !session.loaded || session.loadFailed || !session.dirty || session.status === 'conflict') return false
    if (session.inFlight) return session.inFlight
    const capturedKey = session.key
    const capturedEpoch = session.epoch
    const submittedVersion = session.editVersion
    const expectedRevision = session.revision
    const document = cloneDocument(session.document)
    if (utf8RequestBytes(document, expectedRevision) > WORLD_CLUE_BOARD_MAX_BYTES) {
      session.error = '画板文档超过 8MiB，请删除部分内容后再保存'
      session.errorKind = 'too-large'
      session.canRetry = false
      session.status = 'error'
      return false
    }
    session.status = 'saving'
    session.pendingSave = true
    const request = api.put<WorldClueBoardWriteResult>(
      `api/v1/worlds/${encodeURIComponent(session.worldId)}/clue-boards/${encodeURIComponent(session.boardKey)}`,
      { scope: session.scope, expectedRevision, document },
    ).then((response) => {
      if (!isCurrent(session, capturedKey, capturedEpoch)) return true
      const result = response.data
      session.revision = Number(result.revision) || session.revision
      session.updatedAt = Number(result.updatedAt) || Date.now()
      if (session.editVersion === submittedVersion) {
        session.dirty = false
        session.pendingSave = false
        session.status = 'ready'
      } else {
        // Edits made while PUT was in flight remain the source of truth.
        session.pendingSave = true
        session.status = 'ready'
        scheduleSave(session)
      }
      session.error = ''
      session.errorKind = null
      session.canRetry = false
      return true
    }).catch((error) => {
      if (!isCurrent(session, capturedKey, capturedEpoch)) return false
      const details = errorDetails(error)
      session.status = Number(error?.response?.status) === 409 ? 'conflict' : 'error'
      session.error = details.message
      session.errorKind = details.kind
      session.canRetry = details.retry
      session.pendingSave = true
      // Dirty state is intentionally retained for every failure.
      return false
    }).finally(() => {
      if (session.inFlight === request) session.inFlight = null
    })
    session.inFlight = request
    return request
  }

  async function flush(): Promise<boolean> {
    const session = current.value
    if (!session) return true
    if (session.scope === 'shared') {
      if (session.localDrawingPending) return false
      if (session.sharedSending) await session.sharedSending
      if (current.value !== session) return false
      if (session.sharedQueue.length && session.status === 'error') return false
      if (session.sharedQueue.length && !await drainSharedQueue(session)) return false
      if (session.needsResync) {
        await load(session.worldId, { force: true, userId: session.userId, scope: session.scope })
        return current.value === session && !session.loadFailed && !session.needsResync
      }
      return !session.dirty && !session.loadFailed
    }
    if (session.saveTimer) {
      clearTimeout(session.saveTimer)
      session.saveTimer = null
    }
    if (session.inFlight) await session.inFlight
    for (;;) {
      if (!session.dirty) return true
      if (session.status === 'conflict' || session.status === 'error') return false
      const version = session.editVersion
      const ok = await save(session)
      if (!ok) return false
      if (!session.dirty || session.editVersion === version) return !session.dirty
      // A change landed while the previous request was in flight. Submit it
      // immediately during an explicit flush instead of waiting for debounce.
    }
  }

  async function retry() {
    const session = current.value
    if (!session || !session.canRetry || session.status === 'conflict') return false
    if (session.loadFailed || (session.loaded && !session.dirty && session.status === 'error')) {
      // A refresh can fail after an older document was already loaded.
      // Retrying must issue the GET again; treating it as a save retry would
      // incorrectly mark the stale document ready.
      session.status = 'idle'
      session.error = ''
      session.errorKind = null
      session.canRetry = false
      await load(session.worldId, { force: true, userId: session.userId, scope: session.scope })
      return !session.loadFailed && session.status !== 'error'
    }
    session.error = ''
    session.status = 'ready'
    return flush()
  }

  async function discardLocalAndReload() {
    const session = current.value
    if (!session) return session
    if (session.sharedSending) await session.sharedSending
    if (current.value !== session) return session
    session.sharedQueue = []
    session.localDrawingPending = false
    if (session.saveTimer) clearTimeout(session.saveTimer)
    session.saveTimer = null
    session.dirty = false
    session.pendingSave = false
    session.error = ''
    session.errorKind = null
    session.status = 'idle'
    session.loadFailed = false
    return load(session.worldId, { force: true, userId: session.userId, scope: session.scope })
  }

  async function refreshOnFocus() {
    const session = current.value
    // An error can represent a Quickdraw snapshot kept only in editor memory
    // (for example a document-size rejection). Do not replace it with a focus
    // refresh until the user explicitly retries or discards it.
    if (!session || session.dirty || session.status === 'saving' || session.status === 'error' || session.status === 'conflict') return session
    return load(session.worldId, { force: true, userId: session.userId, scope: session.scope })
  }

  function clearRemoteEventBuffer(session: WorldClueBoardSession) {
    if (session.remoteGapTimer) clearTimeout(session.remoteGapTimer)
    session.remoteGapTimer = null
    session.pendingRemoteEvents.clear()
  }

  function requestSharedRefresh() {
    const session = current.value
    if (!session || session.scope !== 'shared') return
    clearRemoteEventBuffer(session)
    session.needsResync = true
    if (realtimeStopped) return
    if (session.resyncTimer) return
    session.resyncTimer = setTimeout(() => {
      session.resyncTimer = null
      if (current.value !== session) return
      if (!session.needsResync) return
      if (session.dirty || session.inFlight || session.sharedSending || session.localDrawingPending || session.status === 'loading') return
      void load(session.worldId, { force: true, userId: session.userId, scope: session.scope })
    }, 80)
  }

  function applyRemoteEvent(session: WorldClueBoardSession, payload: WorldClueBoardEventPayload): WorldClueBoardOperation | null {
    if (!payload.operation) return null
    try {
      const document = cloneDocument(session.document)
      if (!applyBoardOperation(document, payload.operation)) return null
      session.document = document
      session.revision = payload.revision
      return payload.operation
    } catch {
      return null
    }
  }

  function drainRemoteEvents(session: WorldClueBoardSession, first: WorldClueBoardEventPayload): WorldClueBoardOperation[] {
    const applied: WorldClueBoardOperation[] = []
    let payload: WorldClueBoardEventPayload | undefined = first
    while (payload) {
      const operation = applyRemoteEvent(session, payload)
      if (!operation) {
        clearRemoteEventBuffer(session)
        requestSharedRefresh()
        return []
      }
      applied.push(operation)
      const nextRevision = session.revision + 1
      payload = session.pendingRemoteEvents.get(nextRevision)
      if (payload) session.pendingRemoteEvents.delete(nextRevision)
    }
    if (session.pendingRemoteEvents.size === 0) clearRemoteEventBuffer(session)
    return applied
  }

  function startRemoteGapTimer(session: WorldClueBoardSession) {
    if (session.remoteGapTimer) return
    session.remoteGapTimer = setTimeout(() => {
      session.remoteGapTimer = null
      if (current.value !== session) {
        session.pendingRemoteEvents.clear()
        return
      }
      if (session.pendingRemoteEvents.size === 0) return
      session.pendingRemoteEvents.clear()
      requestSharedRefresh()
    }, 120)
  }

  function handleBoardChanged(payload: WorldClueBoardEventPayload): WorldClueBoardOperation[] {
    const session = current.value
    if (!session || session.scope !== 'shared' || payload.scope !== 'shared' || payload.worldId !== session.worldId || payload.boardKey !== session.boardKey || payload.revision <= session.revision) return []
    session.remoteRevision = Math.max(session.remoteRevision, payload.revision)
    if (payload.updatedBy === session.userId && payload.clientId === session.clientId) return []
    if (!payload.operation) {
      requestSharedRefresh()
      return []
    }
    if (!session.loaded || session.loadFailed || session.sharedQueue.length || session.sharedSending || session.localDrawingPending || session.needsResync || session.status === 'loading') {
      requestSharedRefresh()
      return []
    }
    if (payload.revision === session.revision + 1) return drainRemoteEvents(session, payload)
    session.pendingRemoteEvents.set(payload.revision, payload)
    startRemoteGapTimer(session)
    return []
  }

  function enqueueShared(operation: WorldClueBoardOperation) {
    const session = current.value
    if (!session || session.scope !== 'shared' || !session.canWrite || !session.loaded || session.loadFailed) return false
    const copy = JSON.parse(JSON.stringify(operation)) as WorldClueBoardOperation
    const document = cloneDocument(session.document)
    if (!applyBoardOperation(document, copy)) return false
    session.document = document
    session.editVersion++
    session.sharedQueue.push(copy)
    session.dirty = true
    session.pendingSave = true
    if (session.status !== 'error') void drainSharedQueue(session)
    return true
  }

  async function drainSharedQueue(session: WorldClueBoardSession): Promise<boolean> {
    if (session.sharedSending) return session.sharedSending
    const epoch = session.epoch
    const request = (async () => {
      while (session.sharedQueue.length) {
        if (!isCurrent(session, session.key, epoch)) return false
        const operation = session.sharedQueue[0]
        session.status = 'saving'
        try {
          const response = await api.post<WorldClueBoardWriteResult & { changed: boolean }>(
            `api/v1/worlds/${encodeURIComponent(session.worldId)}/clue-boards/${encodeURIComponent(session.boardKey)}/ops`,
            { scope: 'shared', clientId: session.clientId, operation },
          )
          if (!isCurrent(session, session.key, epoch)) return false
          if (response.data.revision > session.revision + (response.data.changed ? 1 : 0)) session.needsResync = true
          session.revision = Math.max(session.revision, response.data.revision)
          session.updatedAt = response.data.updatedAt
          session.sharedQueue.shift()
          session.error = ''
          session.errorKind = null
          session.canRetry = false
        } catch (error) {
          if (!isCurrent(session, session.key, epoch)) return false
          const details = errorDetails(error)
          session.status = 'error'
          session.error = details.kind === 'conflict' ? '协作提交暂未完成，请重试' : details.message
          session.errorKind = details.kind === 'conflict' ? 'network' : details.kind
          session.canRetry = details.retry || details.kind === 'conflict'
          return false
        }
      }
      session.dirty = false
      session.pendingSave = false
      session.status = 'ready'
      if (session.remoteRevision > session.revision) session.needsResync = true
      return true
    })().finally(() => {
      if (session.sharedSending === request) session.sharedSending = null
      if (current.value === session && session.needsResync && !session.dirty && !session.localDrawingPending) requestSharedRefresh()
    })
    session.sharedSending = request
    return request
  }

  const putSharedPlacements = (placements: Record<string, WorldClueBoardPlacement>) => enqueueShared({ id: boardRuntimeID(), type: 'placements.put', placements })
  const putSharedRelation = (relation: WorldClueBoardRelation) => enqueueShared({ id: boardRuntimeID(), type: 'relation.put', relation })
  const removeSharedRelation = (relationId: string) => enqueueShared({ id: boardRuntimeID(), type: 'relation.remove', relationId })
  const applySharedQuickdrawDiff = (quickdrawDiff: Diff) => enqueueShared({ id: boardRuntimeID(), type: 'quickdraw.diff', quickdrawDiff })

  function setDrawingPending(pending: boolean) {
    const session = current.value
    if (!session || session.scope !== 'shared') return
    session.localDrawingPending = pending
    if (!pending && session.needsResync) requestSharedRefresh()
  }

  function syncSharedSnapshot(snapshot: Snapshot) {
    const session = current.value
    if (!session || session.scope !== 'shared' || !session.canWrite || !session.loaded) return false
    session.document.quickdraw = { engineVersion: '@quickdrawjs/core@0.2.0', snapshot: JSON.parse(JSON.stringify(snapshot)) }
    return true
  }

  function stopRealtime() {
    realtimeStopped = true
    for (const session of Object.values(sessions)) {
      if (session.resyncTimer) clearTimeout(session.resyncTimer)
      session.resyncTimer = null
      clearRemoteEventBuffer(session)
    }
  }

  function startRealtime() { realtimeStopped = false }

  function clearSession(worldId?: string) {
    const keys = Object.keys(sessions).filter(key => !worldId || sessions[key].worldId === worldId)
    for (const key of keys) {
      const session = sessions[key]
      if (session.saveTimer) clearTimeout(session.saveTimer)
      if (session.resyncTimer) clearTimeout(session.resyncTimer)
      clearRemoteEventBuffer(session)
      delete sessions[key]
    }
    if (activeKey.value && !sessions[activeKey.value]) activeKey.value = ''
  }

  return {
    putSharedPlacements,
    putSharedRelation,
    removeSharedRelation,
    applySharedQuickdrawDiff,
    setDrawingPending,
    syncSharedSnapshot,
    handleBoardChanged,
    requestSharedRefresh,
    stopRealtime,
    startRealtime,
    sessions,
    activeKey,
    current,
    currentUserId,
    makeKey,
    ensureSession,
    load,
    updateDocument,
    markChanged,
    scheduleSave,
    save,
    flush,
    retry,
    discardLocalAndReload,
    refreshOnFocus,
    clearSession,
    utf8Bytes,
    utf8RequestBytes,
  }
})
