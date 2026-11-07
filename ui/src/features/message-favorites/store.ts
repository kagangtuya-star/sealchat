import { onScopeDispose, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { useUserStore } from '@/stores/user'
import {
  MESSAGE_FAVORITE_BATCH_MAX,
  batchCreateMessageFavorites,
  batchDeleteMessageFavorites,
  createMessageFavoriteTag,
  deleteMessageFavorite,
  deleteMessageFavoriteTag,
  describeMessageFavoriteError,
  fetchMessageFavorites,
  fetchMessageFavoriteTags,
  moveMessageFavorite,
  renameMessageFavoriteTag,
  replaceMessageFavoriteTags,
  type MessageFavoriteBatchResult,
  type MessageFavoriteItem,
  type MessageFavoriteListQuery,
  type MessageFavoriteScope,
  type MessageFavoriteTag,
} from './api'

// 共享领域状态：收藏列表、标签、已知收藏 ID、权限和请求状态。
// 窗口几何、最小化和筛选输入属于悬浮窗自身，不放在这里。

export interface MessageFavoriteListState {
  query: MessageFavoriteListQuery
  items: MessageFavoriteItem[]
  nextCursor: string
  loaded: boolean
  loading: boolean
  loadingMore: boolean
  moving: boolean
  error: string
  seq: number
}

interface MessageFavoriteTagState {
  items: MessageFavoriteTag[]
  loaded: boolean
  loading: boolean
  seq: number
}

const PAGE_SIZE = 30
const PAGE_SIZE_MAX = 100
const REFRESH_DELAY = 250

export interface MessageFavoriteBatchRemoveResult {
  deleted: string[]
  // 服务端确认已不存在的 ID（例如已被其他客户端取消），同样视为已移除。
  missing: string[]
  // 因请求失败或用户上下文变化而未处理的 ID；非空时 error 说明原因。
  failed: string[]
  error: unknown
}

// personal 是跨世界的单一 bucket；world 以 worldId 为边界。
export const messageFavoriteBucketKey = (scope: MessageFavoriteScope, worldId: string) => (
  scope === 'world' ? `world:${worldId}` : 'personal'
)

const queryBucketKey = (query: MessageFavoriteListQuery) => messageFavoriteBucketKey(query.scope, query.worldId)

const emptyList = (query: MessageFavoriteListQuery): MessageFavoriteListState => ({
  query,
  items: [],
  nextCursor: '',
  loaded: false,
  loading: false,
  loadingMore: false,
  moving: false,
  error: '',
  seq: 0,
})

export const useMessageFavoriteStore = defineStore('messageFavorites', () => {
  const lists = ref<Record<string, MessageFavoriteListState>>({})
  const tags = ref<Record<string, MessageFavoriteTagState>>({})
  const worldManage = ref<Record<string, boolean>>({})
  // bucketKey -> messageId -> favoriteId。只记录本会话确知的收藏，未知不代表未收藏。
  const knownFavorites = ref<Record<string, Record<string, string>>>({})
  const refreshTimers = new Map<string, ReturnType<typeof setTimeout>>()
  const user = useUserStore()
  let activeUserId = ''
  let userRevision = 0

  const resetForUser = (userId: string) => {
    const normalized = String(userId || '').trim()
    if (normalized === activeUserId) return
    userRevision += 1
    Object.values(lists.value).forEach((state) => { state.seq += 1 })
    Object.values(tags.value).forEach((state) => { state.seq += 1 })
    refreshTimers.forEach((timer) => clearTimeout(timer))
    refreshTimers.clear()
    lists.value = {}
    tags.value = {}
    worldManage.value = {}
    knownFavorites.value = {}
    activeUserId = normalized
  }

  watch(() => String(user.info?.id || ''), resetForUser, { immediate: true })

  onScopeDispose(() => {
    refreshTimers.forEach((timer) => clearTimeout(timer))
    refreshTimers.clear()
  })

  const rememberFavorite = (key: string, messageId: string, favoriteId: string) => {
    if (!messageId || !favoriteId) return
    if (!knownFavorites.value[key]) knownFavorites.value[key] = {}
    const bucket = knownFavorites.value[key]
    bucket[messageId] = favoriteId
  }

  const forgetFavorites = (key: string, favoriteIds: Set<string>) => {
    const bucket = knownFavorites.value[key]
    if (!bucket) return
    for (const [messageId, id] of Object.entries(bucket)) {
      if (favoriteIds.has(id)) delete bucket[messageId]
    }
  }

  const knownFavoriteId = (scope: MessageFavoriteScope, worldId: string, messageId: string) => (
    knownFavorites.value[messageFavoriteBucketKey(scope, worldId)]?.[messageId] || ''
  )

  // 未从后端得到 canManageWorld 时返回 null，由调用方决定保守展示。
  const canManageWorld = (worldId: string): boolean | null => {
    if (!worldId) return false
    return worldId in worldManage.value ? worldManage.value[worldId] : null
  }

  // 入口展示用：后端已返回 canManageWorld 时以它为准，否则按成员角色做保守估计。
  // 这只影响按钮状态，真正的授权始终由后端判断。
  const worldManageHint = (worldId: string, memberRole?: string) => {
    const known = canManageWorld(worldId)
    if (known !== null) return known
    return !!worldId && (memberRole === 'owner' || memberRole === 'admin')
  }

  // 首次需要时用标签接口取回权限（顺带预热世界标签）。
  const probeWorldPermission = (worldId: string) => {
    if (!worldId || canManageWorld(worldId) !== null) return
    void ensureTags('world', worldId).catch(() => undefined)
  }

  const listState = (key: string) => lists.value[key] || null

  async function loadList(query: MessageFavoriteListQuery, options: { keepItems?: boolean } = {}) {
    const key = queryBucketKey(query)
    // 先写入再读回，确保后续修改经过响应式代理。
    if (!lists.value[key]) lists.value[key] = emptyList(query)
    const state = lists.value[key]
    state.query = { ...query, tagIds: [...query.tagIds] }
    const seq = ++state.seq
    state.loading = true
    state.loadingMore = false
    state.error = ''
    const limit = options.keepItems
      ? Math.min(PAGE_SIZE_MAX, Math.max(PAGE_SIZE, state.items.length))
      : PAGE_SIZE
    try {
      const result = await fetchMessageFavorites(query, { limit })
      if (state.seq !== seq) return
      state.items = result.items
      state.nextCursor = result.nextCursor
      state.loaded = true
      if (query.scope === 'world') worldManage.value[query.worldId] = result.canManageWorld
      result.items.forEach((item) => rememberFavorite(key, item.messageId, item.id))
    } catch (error) {
      if (state.seq !== seq) return
      state.error = describeMessageFavoriteError(error, '加载收藏失败')
    } finally {
      if (state.seq === seq) state.loading = false
    }
  }

  async function loadMore(key: string) {
    const state = lists.value[key]
    if (!state || !state.nextCursor || state.loading || state.loadingMore) return
    const seq = state.seq
    state.loadingMore = true
    try {
      const result = await fetchMessageFavorites(state.query, { cursor: state.nextCursor, limit: PAGE_SIZE })
      if (state.seq !== seq) return
      const seen = new Set(state.items.map((item) => item.id))
      state.items.push(...result.items.filter((item) => !seen.has(item.id)))
      state.nextCursor = result.nextCursor
      result.items.forEach((item) => rememberFavorite(key, item.messageId, item.id))
    } catch (error) {
      if (state.seq === seq) state.error = describeMessageFavoriteError(error, '加载更多收藏失败')
    } finally {
      if (state.seq === seq) state.loadingMore = false
    }
  }

  async function ensureTags(scope: MessageFavoriteScope, worldId: string, force = false) {
    const key = messageFavoriteBucketKey(scope, worldId)
    if (!tags.value[key]) tags.value[key] = { items: [], loaded: false, loading: false, seq: 0 }
    const state = tags.value[key]
    if (state.loading && !force) return
    if (state.loaded && !force) return
    const seq = ++state.seq
    state.loading = true
    try {
      const result = await fetchMessageFavoriteTags(scope, worldId)
      if (state.seq !== seq) return
      state.items = result.items
      state.loaded = true
      if (scope === 'world') worldManage.value[worldId] = result.canManageWorld
    } finally {
      if (state.seq === seq) state.loading = false
    }
  }

  const tagItems = (scope: MessageFavoriteScope, worldId: string) => (
    tags.value[messageFavoriteBucketKey(scope, worldId)]?.items || []
  )

  const tagsLoaded = (scope: MessageFavoriteScope, worldId: string) => (
    tags.value[messageFavoriteBucketKey(scope, worldId)]?.loaded === true
  )

  // 合并短时间内的多次失效信号；只刷新已加载过的 bucket。
  function scheduleRefresh(key: string, options: { tags?: boolean } = {}) {
    const previous = refreshTimers.get(key)
    if (previous !== undefined) clearTimeout(previous)
    refreshTimers.set(key, setTimeout(() => {
      refreshTimers.delete(key)
      const state = lists.value[key]
      if (state?.loaded) void loadList(state.query, { keepItems: true })
      const tagState = tags.value[key]
      if (options.tags && tagState?.loaded) {
        const worldId = key.startsWith('world:') ? key.slice('world:'.length) : ''
        void ensureTags(worldId ? 'world' : 'personal', worldId, true).catch(() => undefined)
      }
    }, REFRESH_DELAY))
  }

  // 后端事件只表示“世界收藏已变化”，不做增量合并。
  function handleWorldChanged(worldId: string) {
    if (!worldId) return
    scheduleRefresh(messageFavoriteBucketKey('world', worldId), { tags: true })
  }

  async function addMessages(scope: MessageFavoriteScope, worldId: string, messageIds: string[]): Promise<MessageFavoriteBatchResult> {
    const ids = Array.from(new Set(messageIds.map((id) => String(id || '').trim()).filter(Boolean)))
    const revision = userRevision
    const result = await batchCreateMessageFavorites(scope, worldId, ids)
    if (revision !== userRevision) return result
    const key = messageFavoriteBucketKey(scope, worldId)
    for (const ref of [...result.created, ...result.existing]) rememberFavorite(key, ref.messageId, ref.id)
    if (result.created.length) scheduleRefresh(key)
    return result
  }

  // onlyKey 为空时清理所有 bucket；收藏 ID 全局唯一。
  const removeFromLists = (favoriteIds: Set<string>, onlyKey = '') => {
    if (!favoriteIds.size) return
    for (const [key, state] of Object.entries(lists.value)) {
      if (onlyKey && key !== onlyKey) continue
      if (state.items.some((item) => favoriteIds.has(item.id))) {
        state.items = state.items.filter((item) => !favoriteIds.has(item.id))
      }
    }
    // 右键收藏可能在对应列表尚未加载时就写入 knownFavorites，删除时也要独立清理缓存。
    for (const key of Object.keys(knownFavorites.value)) {
      if (!onlyKey || key === onlyKey) forgetFavorites(key, favoriteIds)
    }
  }

  async function removeFavorite(favoriteId: string) {
    const revision = userRevision
    await deleteMessageFavorite(favoriteId)
    if (revision === userRevision) removeFromLists(new Set([favoriteId]))
  }

  // 批量取消同一收藏空间内的收藏。超过单次上限时按顺序分块，每块一次请求；
  // 某块失败或用户上下文变化即停止，已成功的块照常同步到缓存。
  async function removeFavorites(scope: MessageFavoriteScope, worldId: string, favoriteIds: string[]): Promise<MessageFavoriteBatchRemoveResult> {
    const ids = Array.from(new Set(favoriteIds.map((id) => String(id || '').trim()).filter(Boolean)))
    const revision = userRevision
    const key = messageFavoriteBucketKey(scope, worldId)
    const result: MessageFavoriteBatchRemoveResult = { deleted: [], missing: [], failed: [], error: null }
    for (let start = 0; start < ids.length; start += MESSAGE_FAVORITE_BATCH_MAX) {
      if (revision !== userRevision) {
        result.failed.push(...ids.slice(start))
        break
      }
      try {
        const response = await batchDeleteMessageFavorites(scope, worldId, ids.slice(start, start + MESSAGE_FAVORITE_BATCH_MAX))
        result.deleted.push(...response.deleted)
        result.missing.push(...response.missing)
        if (revision !== userRevision) continue
        removeFromLists(new Set(response.deleted))
        // missing 表示服务端确认 ID 已不存在；请求来自当前 bucket，只清理对应缓存即可。
        removeFromLists(new Set(response.missing), key)
      } catch (error) {
        result.error = error
        result.failed.push(...ids.slice(start))
        break
      }
    }
    // 删除期间仍在途的列表请求可能带回已删除项，结束后按当前查询重读一次。
    const state = lists.value[key]
    if (revision === userRevision && (state?.loading || state?.loadingMore)) scheduleRefresh(key)
    return result
  }

  // 只提交一个邻居：world 中可能有当前管理员不可见的项夹在可见邻居之间，
  // 同时提交前驱和后继会被后端判为冲突。
  async function moveFavorite(key: string, favoriteId: string, toIndex: number) {
    const revision = userRevision
    const state = lists.value[key]
    if (!state || state.moving) return false
    const from = state.items.findIndex((item) => item.id === favoriteId)
    const target = Math.max(0, Math.min(state.items.length - 1, toIndex))
    if (from < 0 || from === target) return false
    const next = state.items.slice()
    const [moving] = next.splice(from, 1)
    next.splice(target, 0, moving)
    const neighbor = target > 0 ? { afterId: next[target - 1].id } : { beforeId: next[1]?.id }
    if (!neighbor.afterId && !neighbor.beforeId) return false
    state.items = next
    state.moving = true
    try {
      await moveMessageFavorite(favoriteId, neighbor)
      return revision === userRevision
    } catch (error) {
      // 不让乐观顺序长期偏离服务端：立即按服务端顺序重载。
      if (revision === userRevision) await loadList(state.query, { keepItems: true })
      throw error
    } finally {
      state.moving = false
    }
  }

  async function setFavoriteTags(favoriteId: string, tagIds: string[]) {
    const revision = userRevision
    const nextTags = await replaceMessageFavoriteTags(favoriteId, tagIds)
    if (revision !== userRevision) return nextTags
    for (const state of Object.values(lists.value)) {
      const item = state.items.find((entry) => entry.id === favoriteId)
      if (item) item.tags = nextTags
    }
    return nextTags
  }

  const sortTags = (items: MessageFavoriteTag[]) => items.sort((a, b) => (
    a.name.toLowerCase().localeCompare(b.name.toLowerCase()) || a.id.localeCompare(b.id)
  ))

  async function createTag(scope: MessageFavoriteScope, worldId: string, name: string) {
    const revision = userRevision
    const tag = await createMessageFavoriteTag(scope, worldId, name)
    if (revision !== userRevision) return tag
    const key = messageFavoriteBucketKey(scope, worldId)
    const state = tags.value[key]
    if (state) state.items = sortTags([...state.items.filter((item) => item.id !== tag.id), tag])
    return tag
  }

  const patchTagEverywhere = (tagId: string, patch: (tag: MessageFavoriteTag) => MessageFavoriteTag | null) => {
    for (const state of Object.values(tags.value)) {
      state.items = sortTags(state.items.map((tag) => (tag.id === tagId ? patch(tag) : tag)).filter((tag): tag is MessageFavoriteTag => !!tag))
    }
    for (const state of Object.values(lists.value)) {
      for (const item of state.items) {
        if (item.tags.some((tag) => tag.id === tagId)) {
          item.tags = sortTags(item.tags.map((tag) => (tag.id === tagId ? patch(tag) : tag)).filter((tag): tag is MessageFavoriteTag => !!tag))
        }
      }
    }
  }

  async function renameTag(tagId: string, name: string) {
    const revision = userRevision
    const tag = await renameMessageFavoriteTag(tagId, name)
    if (revision !== userRevision) return tag
    patchTagEverywhere(tagId, (current) => ({ ...current, name: tag.name || name }))
    return tag
  }

  async function removeTag(tagId: string) {
    const revision = userRevision
    await deleteMessageFavoriteTag(tagId)
    if (revision === userRevision) patchTagEverywhere(tagId, () => null)
  }

  return {
    lists,
    tags,
    listState,
    knownFavoriteId,
    canManageWorld,
    worldManageHint,
    probeWorldPermission,
    tagItems,
    tagsLoaded,
    loadList,
    loadMore,
    ensureTags,
    scheduleRefresh,
    handleWorldChanged,
    addMessages,
    removeFavorite,
    removeFavorites,
    moveFavorite,
    setFavoriteTags,
    createTag,
    renameTag,
    removeTag,
  }
})
