import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { api } from './_config'

export type WorldClueKind = 'text' | 'image' | 'iframe'
export type WorldClueAccess = 'none' | 'view' | 'edit'
export type WorldClueStatus = 'draft' | 'published' | 'archived'
export type WorldClueContentFormat = 'plain' | 'tiptap'

export interface WorldCluePresentation {
  version: 1
  mediaPlacement: 'left' | 'right' | 'top' | 'bottom'
  mediaRatio: number
  objectFit: 'contain' | 'cover'
  mediaType: 'image' | 'video'
  enterAnimation: 'none' | 'fade' | 'fade-up' | 'scale' | 'fade-scale' | 'slide-left' | 'slide-right'
  exitAnimation: 'none' | 'fade' | 'fade-up' | 'scale' | 'fade-scale' | 'slide-left' | 'slide-right'
  animationDurationMs: number
  titleColor: string
  backgroundColorEnabled: boolean
  backgroundColor: string
  backgroundColorOpacity: number
  backgroundMediaEnabled: boolean
  backgroundMediaAttachmentId: string
  backgroundMediaUrl: string
  backgroundMediaType: 'image' | 'video'
  backgroundMediaMode: 'cover' | 'contain' | 'tile' | 'center'
  backgroundMediaOpacity: number
  backgroundMediaBlur: number
  backgroundMediaBrightness: number
}

export interface WorldClueUserState {
  personalFolderId?: string
  personalOrder: number
  favorite: boolean
  seenRevision: number
  seenPrivateRevision: number
  assignedPresentationSeq: number
  presentedPresentationSeq: number
  lastOpenedAt?: number
}

export interface WorldClueSummary {
  id: string
  worldId: string
  sharedFolderId?: string
  title: string
  kind: WorldClueKind
  contentText?: string
  imageAttachmentId?: string
  imageUrl?: string
  embedDomain?: string
  defaultAccess?: 'none' | 'view'
  effectiveAccess: WorldClueAccess
  status: WorldClueStatus
  revision: number
  publishSeq: number
  orderIndex: number
  privateRevision?: number
  hasPrivateContent: boolean
  unread: boolean
  userState?: WorldClueUserState
  creatorId: string
  updatedAt: number
  publishedAt?: number
}

export interface WorldClueDetail extends WorldClueSummary {
  contentFormat: WorldClueContentFormat
  content: string
  embedUrl?: string
  presentation: WorldCluePresentation
  privateContentFormat?: WorldClueContentFormat
  privateContent?: string
  managerNoteFormat?: WorldClueContentFormat
  managerNote?: string
  managerNoteText?: string
  updatedBy?: string
  publishedBy?: string
}

export interface WorldClueFolder {
  id: string
  worldId: string
  scope: 'shared' | 'personal'
  ownerUserId?: string
  parentId?: string
  name: string
  color?: string
  orderIndex: number
  createdAt: number
  updatedAt: number
}

export interface WorldClueResolveItem {
  accessible: boolean
  effectiveAccess?: WorldClueAccess
  title?: string
  kind?: WorldClueKind
  mediaType?: 'image' | 'video'
  thumbnailAttachmentId?: string
  thumbnailUrl?: string
  excerpt?: string
  embedDomain?: string
  revision?: number
  updatedAt?: number
  hasPrivateContent?: boolean
}

export interface WorldClueRosterMember {
  userId: string
  role: 'owner' | 'admin' | 'member' | 'spectator'
  username: string
  nickname: string
  avatar: string
  orderIndex: number
}

export interface WorldClueEditingLock {
  field: string
  userId: string
  sessionId?: string
  expireAt: number
  user?: {
    id: string
    name?: string
    nick?: string
    avatar?: string
  }
}

export interface WorldCluePresentationRequest {
  worldId: string
  clueId: string
  publishSeq: number
  manual?: boolean
}

type ResolveWaiter = { resolve: (value: WorldClueResolveItem) => void; reject: (reason?: unknown) => void }

export const useWorldClueStore = defineStore('worldClue', () => {
  const currentWorldId = ref('')
  const summariesByWorld = ref<Record<string, Record<string, WorldClueSummary>>>({})
  const foldersByWorld = ref<Record<string, WorldClueFolder[]>>({})
  const rosterByWorld = ref<Record<string, WorldClueRosterMember[]>>({})
  const editLocksByClue = ref<Record<string, WorldClueEditingLock[]>>({})
  const keywordsByWorld = ref<Record<string, string>>({})
  const detail = ref<WorldClueDetail | null>(null)
  const resolveCache = ref<Record<string, Record<string, WorldClueResolveItem>>>({})
  const presentationQueue = ref<WorldCluePresentationRequest[]>([])
  const uiVisible = ref(false)
  const loading = ref(false)
  const resolvePending = new Map<string, Set<string>>()
  const resolveWaiters = new Map<string, ResolveWaiter[]>()
  const resolveTimers = new Map<string, ReturnType<typeof setTimeout>>()
  const presentationKeys = new Set<string>()

  const summaries = computed(() => Object.values(summariesByWorld.value[currentWorldId.value] || {}))
  const folders = computed(() => foldersByWorld.value[currentWorldId.value] || [])
  const roster = computed(() => rosterByWorld.value[currentWorldId.value] || [])
  const unreadCount = computed(() => summaries.value.filter(item => item.unread).length)

  function summaryFromDetail(item: WorldClueDetail): WorldClueSummary {
    return {
      id: item.id,
      worldId: item.worldId,
      sharedFolderId: item.sharedFolderId,
      title: item.title,
      kind: item.kind,
      contentText: item.contentText,
      imageAttachmentId: item.imageAttachmentId,
      imageUrl: item.imageUrl,
      embedDomain: item.embedDomain,
      defaultAccess: item.defaultAccess,
      effectiveAccess: item.effectiveAccess,
      status: item.status,
      revision: item.revision,
      publishSeq: item.publishSeq,
      orderIndex: item.orderIndex,
      privateRevision: item.privateRevision,
      hasPrivateContent: item.hasPrivateContent,
      unread: item.unread,
      userState: item.userState,
      creatorId: item.creatorId,
      updatedAt: item.updatedAt,
      publishedAt: item.publishedAt,
    }
  }

  function setWorld(worldId: string) {
    if (worldId === currentWorldId.value) return
    currentWorldId.value = worldId
    detail.value = null
    uiVisible.value = false
    resolveCache.value[worldId] = {}
    presentationQueue.value = presentationQueue.value.filter(item => item.worldId === worldId)
    for (const key of presentationKeys) {
      if (!key.startsWith(`${worldId}:`)) presentationKeys.delete(key)
    }
  }

  function setVisible(value: boolean) {
    uiVisible.value = value
  }

  function toggleVisible() {
    uiVisible.value = !uiVisible.value
  }

  async function loadWorld(worldId: string, keyword = '') {
    if (!worldId) return
    setWorld(worldId)
    keywordsByWorld.value[worldId] = keyword
    loading.value = true
    try {
      const [cluesResponse, foldersResponse] = await Promise.all([
        api.get(`api/v1/worlds/${worldId}/clues`, { params: keyword ? { keyword } : undefined }),
        api.get(`api/v1/worlds/${worldId}/clue-folders`),
      ])
      if (currentWorldId.value !== worldId || keywordsByWorld.value[worldId] !== keyword) return
      summariesByWorld.value[worldId] = Object.fromEntries(
        ((cluesResponse.data?.items || []) as WorldClueSummary[]).map(item => [item.id, item]),
      )
      foldersByWorld.value[worldId] = (foldersResponse.data?.items || []) as WorldClueFolder[]
    } finally {
      if (currentWorldId.value === worldId && keywordsByWorld.value[worldId] === keyword) loading.value = false
    }
  }

  async function loadRoster(worldId: string) {
    if (!worldId) return []
    const response = await api.get(`api/v1/worlds/${worldId}/clue-roster`)
    const items = (response.data?.items || []) as WorldClueRosterMember[]
    if (currentWorldId.value === worldId) rosterByWorld.value[worldId] = items
    return items
  }

  async function addRosterMember(worldId: string, userId: string) {
    const response = await api.put(`api/v1/worlds/${worldId}/clue-roster/${encodeURIComponent(userId)}`)
    const item = response.data?.item as WorldClueRosterMember
    if (currentWorldId.value === worldId) {
      const current = rosterByWorld.value[worldId] || []
      rosterByWorld.value[worldId] = current.some(member => member.userId === item.userId)
        ? current.map(member => member.userId === item.userId ? item : member)
        : [...current, item].sort((a, b) => a.orderIndex - b.orderIndex)
    }
    return item
  }

  async function removeRosterMember(worldId: string, userId: string) {
    await api.delete(`api/v1/worlds/${worldId}/clue-roster/${encodeURIComponent(userId)}`)
    if (currentWorldId.value === worldId) {
      rosterByWorld.value[worldId] = (rosterByWorld.value[worldId] || []).filter(member => member.userId !== userId)
    }
  }

  async function fetchDetail(worldId: string, clueId: string, shouldMarkSeen = true) {
    const response = await api.get(`api/v1/worlds/${worldId}/clues/${clueId}`)
    const item = response.data?.item as WorldClueDetail
    const stillCurrent = currentWorldId.value === worldId
    if (stillCurrent) {
      detail.value = item
      summariesByWorld.value[worldId] ||= {}
      summariesByWorld.value[worldId][clueId] = summaryFromDetail(item)
    }
    if (shouldMarkSeen && stillCurrent) {
      void markSeen(worldId, clueId).catch(() => undefined)
    }
    return item
  }

  async function markSeen(worldId: string, clueId: string) {
    await api.post(`api/v1/worlds/${worldId}/clues/${clueId}/seen`)
    const summary = summariesByWorld.value[worldId]?.[clueId]
    if (summary) summary.unread = false
  }

  async function saveClue(worldId: string, clueId: string | null, payload: Record<string, unknown>) {
    const response = clueId
      ? await api.patch(`api/v1/worlds/${worldId}/clues/${clueId}`, payload)
      : await api.post(`api/v1/worlds/${worldId}/clues`, payload)
    const item = response.data?.item as WorldClueDetail
    summariesByWorld.value[worldId] ||= {}
    summariesByWorld.value[worldId][item.id] = summaryFromDetail(item)
    if (currentWorldId.value === worldId) detail.value = item
    return item
  }

  async function removeClue(worldId: string, clueId: string) {
    await api.delete(`api/v1/worlds/${worldId}/clues/${clueId}`)
    delete summariesByWorld.value[worldId]?.[clueId]
    delete resolveCache.value[worldId]?.[clueId]
    delete editLocksByClue.value[`${worldId}:${clueId}`]
    if (detail.value?.worldId === worldId && detail.value.id === clueId) detail.value = null
  }

  const editLockKey = (worldId: string, clueId: string) => `${worldId}:${clueId}`

  async function loadEditLocks(worldId: string, clueId: string) {
    if (!worldId || !clueId) return []
    const response = await api.get(`api/v1/worlds/${worldId}/clues/${clueId}/edit-locks`)
    const items = (response.data?.items || []) as WorldClueEditingLock[]
    editLocksByClue.value[editLockKey(worldId, clueId)] = items
    return items
  }

  async function acquireEditLock(worldId: string, clueId: string, field: string, sessionId: string): Promise<{ ok: boolean; conflict?: boolean; lock?: WorldClueEditingLock }> {
    try {
      const response = await api.post(`api/v1/worlds/${worldId}/clues/${clueId}/edit-lock/acquire`, { field, sessionId })
      const lock = response.data?.lock as WorldClueEditingLock
      if (lock) {
        const key = editLockKey(worldId, clueId)
        const current = editLocksByClue.value[key] || []
        editLocksByClue.value[key] = [...current.filter(item => item.field !== lock.field), lock]
      }
      return { ok: true, lock }
    } catch (error: any) {
      if (error?.response?.status === 409) {
        const lock = error.response.data?.lock as WorldClueEditingLock | undefined
        if (lock) {
          const key = editLockKey(worldId, clueId)
          const current = editLocksByClue.value[key] || []
          editLocksByClue.value[key] = [...current.filter(item => item.field !== lock.field), lock]
        }
        return { ok: false, conflict: true, lock }
      }
      throw error
    }
  }

  async function releaseEditLock(worldId: string, clueId: string, field: string, sessionId: string) {
    const response = await api.post(`api/v1/worlds/${worldId}/clues/${clueId}/edit-lock/release`, { field, sessionId })
    // A release is idempotent. Even when the lease already expired or was
    // replaced, discard our stale local snapshot so it cannot render as a
    // different user's lock until the next snapshot arrives.
    const key = editLockKey(worldId, clueId)
    editLocksByClue.value[key] = (editLocksByClue.value[key] || []).filter(item => !(item.field === field && item.sessionId === sessionId))
    return Boolean(response.data?.released)
  }

  async function publish(worldId: string, clueId: string, expectedPublishSeq: number) {
    const response = await api.post(`api/v1/worlds/${worldId}/clues/${clueId}/publish`, { expectedPublishSeq })
    const item = response.data?.item as WorldClueDetail
    summariesByWorld.value[worldId] ||= {}
    summariesByWorld.value[worldId][clueId] = summaryFromDetail(item)
    if (currentWorldId.value === worldId) detail.value = item
    return item
  }

  async function unpublish(worldId: string, clueId: string, expectedPublishSeq: number) {
    const response = await api.post(`api/v1/worlds/${worldId}/clues/${clueId}/unpublish`, { expectedPublishSeq })
    const item = response.data?.item as WorldClueDetail
    summariesByWorld.value[worldId] ||= {}
    summariesByWorld.value[worldId][clueId] = summaryFromDetail(item)
    if (currentWorldId.value === worldId) detail.value = item
    return item
  }

  function presentationKey(request: WorldCluePresentationRequest) {
    if (!request.worldId || !request.clueId || request.publishSeq <= 0) return ''
    return `${request.worldId}:${request.clueId}:${request.publishSeq}`
  }

  function rememberPresentation(request: WorldCluePresentationRequest) {
    const key = presentationKey(request)
    if (key) presentationKeys.add(key)
  }

  function enqueuePresentation(request: WorldCluePresentationRequest) {
    const key = presentationKey(request)
    if (!key) return
    if (presentationKeys.has(key)) return
    presentationKeys.add(key)
    presentationQueue.value.push(request)
  }

  async function loadPendingPresentations(worldId: string, enteredAt?: number) {
    if (!worldId) return
    const response = await api.get(`api/v1/worlds/${worldId}/clues/pending-presentations`)
    if (currentWorldId.value !== worldId) return
    for (const item of (response.data?.items || []) as WorldClueDetail[]) {
      const request = { worldId, clueId: item.id, publishSeq: item.publishSeq }
      if (enteredAt !== undefined && Number(item.publishedAt || 0) <= enteredAt) rememberPresentation(request)
      else enqueuePresentation(request)
    }
  }

  async function acknowledgePresented(request: WorldCluePresentationRequest) {
    await api.post(`api/v1/worlds/${request.worldId}/clues/${request.clueId}/presented`, { publishSeq: request.publishSeq })
  }

  function invalidate(worldId: string, clueId?: string) {
    if (clueId) {
      delete resolveCache.value[worldId]?.[clueId]
      if (detail.value?.worldId === worldId && detail.value.id === clueId) detail.value = null
    } else {
      resolveCache.value[worldId] = {}
    }
  }

  async function handleChanged(payload: { worldId?: string; clueId?: string; action?: string }) {
    const worldId = payload.worldId || ''
    if (!worldId) return
    if (payload.action === 'edit-lock') {
      return
    }
    invalidate(worldId, payload.clueId)
    if (worldId !== currentWorldId.value) return
    if (payload.action === 'remove' && payload.clueId) {
      delete summariesByWorld.value[worldId]?.[payload.clueId]
      return
    }
    if (payload.action === 'folders') {
      const keyword = keywordsByWorld.value[worldId] || ''
      const [foldersResponse, cluesResponse] = await Promise.all([
        api.get(`api/v1/worlds/${worldId}/clue-folders`),
        api.get(`api/v1/worlds/${worldId}/clues`, { params: keyword ? { keyword } : undefined }),
      ])
      if (currentWorldId.value === worldId && keywordsByWorld.value[worldId] === keyword) {
        foldersByWorld.value[worldId] = foldersResponse.data?.items || []
        summariesByWorld.value[worldId] = Object.fromEntries(
          ((cluesResponse.data?.items || []) as WorldClueSummary[]).map(item => [item.id, item]),
        )
      }
      return
    }
    if (payload.action === 'reorder') {
      const keyword = keywordsByWorld.value[worldId] || ''
      const response = await api.get(`api/v1/worlds/${worldId}/clues`, { params: keyword ? { keyword } : undefined })
      if (currentWorldId.value === worldId && keywordsByWorld.value[worldId] === keyword) {
        summariesByWorld.value[worldId] = Object.fromEntries(
          ((response.data?.items || []) as WorldClueSummary[]).map(item => [item.id, item]),
        )
      }
      return
    }
    if (payload.clueId) {
      try {
        const keyword = keywordsByWorld.value[worldId] || ''
        const response = await api.get(`api/v1/worlds/${worldId}/clues`, { params: keyword ? { keyword } : undefined })
        if (currentWorldId.value !== worldId || keywordsByWorld.value[worldId] !== keyword) return
        const item = ((response.data?.items || []) as WorldClueSummary[]).find(candidate => candidate.id === payload.clueId)
        if (!item) {
          delete summariesByWorld.value[worldId]?.[payload.clueId]
          return
        }
        summariesByWorld.value[worldId] ||= {}
        summariesByWorld.value[worldId][item.id] = item
      } catch {
        delete summariesByWorld.value[worldId]?.[payload.clueId]
      }
    }
  }

  function requestResolve(worldId: string, clueId: string): Promise<WorldClueResolveItem> {
    const cached = resolveCache.value[worldId]?.[clueId]
    if (cached) return Promise.resolve(cached)
    const key = `${worldId}:${clueId}`
    const promise = new Promise<WorldClueResolveItem>((resolve, reject) => {
      const waiters = resolveWaiters.get(key) || []
      waiters.push({ resolve, reject })
      resolveWaiters.set(key, waiters)
    })
    const ids = resolvePending.get(worldId) || new Set<string>()
    ids.add(clueId)
    resolvePending.set(worldId, ids)
    if (!resolveTimers.has(worldId)) {
      resolveTimers.set(worldId, setTimeout(() => void flushResolve(worldId), 0))
    }
    return promise
  }

  async function flushResolve(worldId: string) {
    resolveTimers.delete(worldId)
    const ids = Array.from(resolvePending.get(worldId) || [])
    resolvePending.delete(worldId)
    if (!ids.length) return
    try {
      const response = await api.post(`api/v1/worlds/${worldId}/clues/resolve`, { clueIds: ids })
      resolveCache.value[worldId] ||= {}
      for (const clueId of ids) {
        const item = (response.data?.items?.[clueId] || { accessible: false }) as WorldClueResolveItem
        resolveCache.value[worldId][clueId] = item
        for (const waiter of resolveWaiters.get(`${worldId}:${clueId}`) || []) waiter.resolve(item)
        resolveWaiters.delete(`${worldId}:${clueId}`)
      }
    } catch (error) {
      for (const clueId of ids) {
        for (const waiter of resolveWaiters.get(`${worldId}:${clueId}`) || []) waiter.reject(error)
        resolveWaiters.delete(`${worldId}:${clueId}`)
      }
    }
  }

  return {
    currentWorldId, summariesByWorld, foldersByWorld, rosterByWorld, editLocksByClue, detail, resolveCache, presentationQueue, uiVisible, loading,
    summaries, folders, roster, unreadCount, setWorld, setVisible, toggleVisible, loadWorld, loadRoster, addRosterMember, removeRosterMember, fetchDetail, saveClue,
    removeClue, publish, unpublish, enqueuePresentation, loadPendingPresentations, acknowledgePresented, markSeen,
    invalidate, handleChanged, requestResolve, loadEditLocks, acquireEditLock, releaseEditLock,
  }
})
