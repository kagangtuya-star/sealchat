import { computed, reactive, ref, toRaw } from 'vue'
import { defineStore } from 'pinia'
import { api } from './_config'
import { useUserStore } from './user'

export const WORLD_CLUE_BOARD_KEY = 'main'
export const WORLD_CLUE_BOARD_SCOPE = 'personal'
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
  return { message: '画板网络请求失败，请检查连接后重试', kind: 'network', retry: true }
}

export const useWorldClueBoardStore = defineStore('worldClueBoard', () => {
  const user = useUserStore()
  const sessions = reactive<Record<string, WorldClueBoardSession>>({})
  const activeKey = ref('')
  const current = computed(() => (activeKey.value ? sessions[activeKey.value] || null : null))

  const makeKey = (userId: string, worldId: string, boardKey = WORLD_CLUE_BOARD_KEY) => `${userId}:${worldId}:${boardKey}`
  const currentUserId = () => String(user.info?.id || '').trim()

  function ensureSession(worldId: string, boardKey = WORLD_CLUE_BOARD_KEY, userId = currentUserId()): WorldClueBoardSession {
    const normalizedWorldId = String(worldId || '').trim()
    const normalizedBoardKey = String(boardKey || '').trim()
    const key = makeKey(userId, normalizedWorldId, normalizedBoardKey)
    let session = sessions[key]
    if (!session) {
      session = reactive({
        key,
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

  async function load(worldId: string, options?: { force?: boolean; userId?: string }): Promise<WorldClueBoardSession> {
    const session = ensureSession(worldId, WORLD_CLUE_BOARD_KEY, options?.userId)
    if (!session.worldId || !session.userId) {
      session.status = 'error'
      session.error = '未登录或缺少世界上下文'
      session.loadFailed = true
      return session
    }
    if (session.loaded && !options?.force) return session
    // A focus refresh is intentionally skipped while the user has local work.
    if (session.dirty && options?.force) return session
    const epoch = ++session.epoch
    const requestedEditVersion = session.editVersion
    session.status = 'loading'
    session.error = ''
    session.errorKind = null
    try {
      const response = await api.get(`api/v1/worlds/${encodeURIComponent(session.worldId)}/clue-boards/${encodeURIComponent(session.boardKey)}`)
      if (!isCurrent(session, session.key, epoch)) return session
      const data = response.data as WorldClueBoardResponse
      const document = normalizeDocument(data.document)
      // A focus/connected refresh must never replace an edit that started
      // while its GET was in flight. The local document remains dirty and the
      // normal save path keeps its original expected revision.
      if (session.dirty || session.editVersion !== requestedEditVersion) {
        return session
      }
      session.document = document
      session.revision = Number(data.revision) || 0
      session.updatedAt = Number(data.updatedAt) || 0
      session.loaded = true
      session.loadFailed = false
      session.dirty = false
      session.pendingSave = false
      session.status = 'ready'
      session.canRetry = false
      return session
    } catch (error) {
      if (isCurrent(session, session.key, epoch)) {
        if (session.dirty || session.editVersion !== requestedEditVersion) return session
        const details = errorDetails(error)
        session.status = 'error'
        session.error = details.message
        session.errorKind = details.kind
        session.canRetry = true
        session.loadFailed = true
      }
      return session
    }
  }

  function markChanged(session: WorldClueBoardSession, document: WorldClueBoardDocument) {
    if (!session.loaded || session.loadFailed) return false
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
    if (!session || session.key !== activeKey.value || !session.loaded || session.loadFailed || !session.dirty || session.status === 'conflict') return
    if (session.saveTimer) clearTimeout(session.saveTimer)
    session.saveTimer = setTimeout(() => {
      session.saveTimer = null
      void save(session)
    }, 650)
  }

  async function save(session = current.value): Promise<boolean> {
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
      { scope: WORLD_CLUE_BOARD_SCOPE, expectedRevision, document },
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
    if (session.loadFailed) {
      // A focus/connected refresh can fail after an older document was
      // already loaded.  Retrying must issue the GET again; treating it as a
      // save retry would incorrectly mark the stale document ready.
      session.status = 'idle'
      session.error = ''
      session.errorKind = null
      session.canRetry = false
      await load(session.worldId, { force: true, userId: session.userId })
      return !session.loadFailed
    }
    session.error = ''
    session.status = 'ready'
    return flush()
  }

  async function discardLocalAndReload() {
    const session = current.value
    if (!session) return session
    if (session.saveTimer) clearTimeout(session.saveTimer)
    session.saveTimer = null
    session.dirty = false
    session.pendingSave = false
    session.error = ''
    session.errorKind = null
    session.status = 'idle'
    session.loadFailed = false
    return load(session.worldId, { force: true, userId: session.userId })
  }

  async function refreshOnFocus() {
    const session = current.value
    // An error can represent a Quickdraw snapshot kept only in editor memory
    // (for example a document-size rejection). Do not replace it with a focus
    // refresh until the user explicitly retries or discards it.
    if (!session || session.dirty || session.status === 'saving' || session.status === 'error' || session.status === 'conflict') return session
    return load(session.worldId, { force: true, userId: session.userId })
  }

  function clearSession(worldId?: string) {
    const keys = Object.keys(sessions).filter(key => !worldId || sessions[key].worldId === worldId)
    for (const key of keys) {
      const session = sessions[key]
      if (session.saveTimer) clearTimeout(session.saveTimer)
      delete sessions[key]
    }
    if (activeKey.value && !sessions[activeKey.value]) activeKey.value = ''
  }

  return {
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
