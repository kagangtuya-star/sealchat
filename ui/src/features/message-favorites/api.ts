import { api, buildAuthorizedJsonRequestInit, urlBase } from '@/stores/_config'
import type { Message } from '@satorijs/protocol'

// 消息收藏 REST 契约的唯一归一化边界。字段语义见 doc/message-favorites-backend-design.md。

export type MessageFavoriteScope = 'personal' | 'world'
export type MessageFavoriteStatus = 'available' | 'deleted' | 'unavailable'
export type MessageFavoriteRejectReason = 'not_visible' | 'wrong_world' | 'whisper_not_allowed' | 'not_found' | string

export interface MessageFavoriteTag {
  id: string
  scope: MessageFavoriteScope
  worldId: string
  name: string
}

// 其他客户端正在编辑备注的快照（不含锁凭据）；只在列表读取时刷新。
export interface MessageFavoriteNoteLock {
  userId: string
  expiresAt: number
}

export interface MessageFavoriteNote {
  id: string
  favoriteId: string
  authorUserId: string
  author: { id: string, name: string, nick: string, avatar: string } | null
  content: string
  createdAt: string
  updatedAt: string
  noteLock: MessageFavoriteNoteLock | null
}

export const favoriteNoteAuthorName = (note: MessageFavoriteNote) => note.author?.nick || note.author?.name || '用户'

export interface MessageFavoriteItem {
  id: string
  scope: MessageFavoriteScope
  worldId: string
  channelId: string
  messageId: string
  sortOrder: number
  createdAt: string
  tags: MessageFavoriteTag[]
  status: MessageFavoriteStatus
  // 只有 available 时存在；deleted / unavailable 不携带旧正文。
  message: Message | null
  noteCount: number
  latestNote: MessageFavoriteNote | null
}

export interface MessageFavoriteRef {
  id: string
  worldId: string
  messageId: string
}

export interface MessageFavoriteListQuery {
  scope: MessageFavoriteScope
  // world：收藏所属世界（必填）；personal：可选世界筛选。
  worldId: string
  q: string
  tagIds: string[]
}

export interface MessageFavoriteListResult {
  items: MessageFavoriteItem[]
  nextCursor: string
  canManageWorld: boolean
}

export interface MessageFavoriteBatchResult {
  created: MessageFavoriteRef[]
  existing: MessageFavoriteRef[]
  rejected: Array<{ messageId: string, reason: MessageFavoriteRejectReason }>
}

const normalizeScope = (raw: unknown): MessageFavoriteScope => (raw === 'world' ? 'world' : 'personal')

const normalizeTag = (raw: any): MessageFavoriteTag => ({
  id: String(raw?.id || ''),
  scope: normalizeScope(raw?.scope),
  worldId: String(raw?.worldId || ''),
  name: String(raw?.name || ''),
})

const normalizeStatus = (raw: unknown): MessageFavoriteStatus => (
  raw === 'available' || raw === 'deleted' ? raw : 'unavailable'
)

const normalizeNoteLock = (raw: any): MessageFavoriteNoteLock | null => {
  const expiresAt = Number(raw?.expiresAt) || 0
  return raw && typeof raw === 'object' && expiresAt > 0 ? { userId: String(raw.userId || ''), expiresAt } : null
}

const normalizeNote = (raw: any): MessageFavoriteNote => ({
  id: String(raw?.id || ''),
  favoriteId: String(raw?.favoriteId || ''),
  authorUserId: String(raw?.authorUserId || ''),
  author: raw?.author && typeof raw.author === 'object' ? {
    id: String(raw.author.id || ''), name: String(raw.author.name || ''),
    nick: String(raw.author.nick || ''), avatar: String(raw.author.avatar || ''),
  } : null,
  content: String(raw?.content || ''),
  createdAt: String(raw?.createdAt || ''),
  updatedAt: String(raw?.updatedAt || ''),
  noteLock: normalizeNoteLock(raw?.noteLock),
})

const normalizeItem = (raw: any): MessageFavoriteItem => {
  const status = normalizeStatus(raw?.status)
  return {
    id: String(raw?.id || ''),
    scope: normalizeScope(raw?.scope),
    worldId: String(raw?.worldId || ''),
    channelId: String(raw?.channelId || ''),
    messageId: String(raw?.messageId || ''),
    sortOrder: Number(raw?.sortOrder) || 0,
    createdAt: String(raw?.createdAt || ''),
    tags: Array.isArray(raw?.tags) ? raw.tags.map(normalizeTag) : [],
    status,
    message: status === 'available' && raw?.message && typeof raw.message === 'object' ? raw.message as Message : null,
    noteCount: Number(raw?.noteCount) || 0,
    latestNote: raw?.latestNote && typeof raw.latestNote === 'object' ? normalizeNote(raw.latestNote) : null,
  }
}

const normalizeRef = (raw: any): MessageFavoriteRef => ({
  id: String(raw?.id || ''),
  worldId: String(raw?.worldId || ''),
  messageId: String(raw?.messageId || ''),
})

const scopeParams = (scope: MessageFavoriteScope, worldId: string) => (
  scope === 'world' ? { scope, worldId } : { scope }
)

export const describeMessageFavoriteError = (error: unknown, fallback = '操作失败') => {
  const data = (error as any)?.response?.data
  const message = String(data?.message || data?.error || '').trim()
  return message || (error instanceof Error && error.message ? error.message : fallback)
}

export const fetchMessageFavorites = async (
  query: MessageFavoriteListQuery,
  options: { cursor?: string, limit?: number } = {},
): Promise<MessageFavoriteListResult> => {
  const params: Record<string, string | number> = { scope: query.scope }
  if (query.worldId) params.worldId = query.worldId
  if (query.q) params.q = query.q
  // 后端同时接受重复参数和逗号分隔；逗号形式避免 axios 数组序列化差异。
  if (query.tagIds.length) params.tagIds = query.tagIds.join(',')
  if (options.cursor) params.cursor = options.cursor
  if (options.limit) params.limit = options.limit
  const resp = await api.get('api/v1/message-favorites', { params })
  return {
    items: Array.isArray(resp.data?.items) ? resp.data.items.map(normalizeItem) : [],
    nextCursor: String(resp.data?.nextCursor || ''),
    canManageWorld: resp.data?.canManageWorld === true,
  }
}

export const batchCreateMessageFavorites = async (
  scope: MessageFavoriteScope,
  worldId: string,
  messageIds: string[],
): Promise<MessageFavoriteBatchResult> => {
  const resp = await api.post('api/v1/message-favorites/batch', { ...scopeParams(scope, worldId), messageIds })
  return {
    created: Array.isArray(resp.data?.created) ? resp.data.created.map(normalizeRef) : [],
    existing: Array.isArray(resp.data?.existing) ? resp.data.existing.map(normalizeRef) : [],
    rejected: Array.isArray(resp.data?.rejected)
      ? resp.data.rejected.map((item: any) => ({ messageId: String(item?.messageId || ''), reason: String(item?.reason || '') }))
      : [],
  }
}

export const deleteMessageFavorite = async (id: string) => {
  await api.delete(`api/v1/message-favorites/${encodeURIComponent(id)}`)
}

// 与后端 MessageFavoriteBatchMax 一致：单次批量请求的最大 ID 数。
export const MESSAGE_FAVORITE_BATCH_MAX = 100

export interface MessageFavoriteBatchDeleteResult {
  deleted: string[]
  // 服务端确认已不存在的 ID（通常已被其他客户端取消）；其他收藏空间中的现存 ID 会被拒绝。
  missing: string[]
}

const normalizeIds = (raw: unknown) => (Array.isArray(raw) ? raw.map((id) => String(id || '')).filter(Boolean) : [])

// 只删除 scope/worldId 所指收藏空间内的收藏；权限按该空间判断。
export const batchDeleteMessageFavorites = async (
  scope: MessageFavoriteScope,
  worldId: string,
  ids: string[],
): Promise<MessageFavoriteBatchDeleteResult> => {
  const resp = await api.post('api/v1/message-favorites/batch-delete', { ...scopeParams(scope, worldId), ids })
  return { deleted: normalizeIds(resp.data?.deleted), missing: normalizeIds(resp.data?.missing) }
}

// afterId 是目标前驱，beforeId 是目标后继；至少提供一个。
export const moveMessageFavorite = async (id: string, neighbor: { afterId?: string, beforeId?: string }) => {
  await api.post(`api/v1/message-favorites/${encodeURIComponent(id)}/move`, neighbor)
}

export const replaceMessageFavoriteTags = async (id: string, tagIds: string[]): Promise<MessageFavoriteTag[]> => {
  const resp = await api.put(`api/v1/message-favorites/${encodeURIComponent(id)}/tags`, { tagIds })
  return Array.isArray(resp.data?.tags) ? resp.data.tags.map(normalizeTag) : []
}

export const fetchMessageFavoriteTags = async (scope: MessageFavoriteScope, worldId: string) => {
  const resp = await api.get('api/v1/message-favorite-tags', { params: scopeParams(scope, worldId) })
  return {
    items: Array.isArray(resp.data?.items) ? resp.data.items.map(normalizeTag) as MessageFavoriteTag[] : [],
    canManageWorld: resp.data?.canManageWorld === true,
  }
}

export const createMessageFavoriteTag = async (scope: MessageFavoriteScope, worldId: string, name: string) => {
  const resp = await api.post('api/v1/message-favorite-tags', { ...scopeParams(scope, worldId), name })
  return normalizeTag(resp.data?.item)
}

export const renameMessageFavoriteTag = async (id: string, name: string) => {
  const resp = await api.patch(`api/v1/message-favorite-tags/${encodeURIComponent(id)}`, { name })
  return normalizeTag(resp.data?.item)
}

export const deleteMessageFavoriteTag = async (id: string) => {
  await api.delete(`api/v1/message-favorite-tags/${encodeURIComponent(id)}`)
}

// 与后端 MessageFavoriteNoteMax / NormalizeMessageFavoriteNote 一致：纯文本，
// 统一换行、去除首尾空白，按 Unicode 码点计数。
export const MESSAGE_FAVORITE_NOTE_MAX = 2000

export const normalizeMessageFavoriteNote = (raw: string) => String(raw || '').replace(/\r\n?/g, '\n').trim()

export const messageFavoriteNoteLength = (raw: string) => Array.from(normalizeMessageFavoriteNote(raw)).length

export interface MessageFavoriteNoteLease {
  // 只在取得编辑权时返回；续租响应不回传凭据。
  token: string
  expiresAt: number
  ttlSeconds: number
  content: string
}

export type MessageFavoriteNoteConflict =
  | { code: 'note_locked', lock: MessageFavoriteNoteLock | null }
  | { code: 'note_lock_lost' }

const normalizeNoteLease = (raw: any): MessageFavoriteNoteLease => ({
  token: String(raw?.token || ''),
  expiresAt: Number(raw?.expiresAt) || 0,
  ttlSeconds: Number(raw?.ttlSeconds) || 60,
  content: String(raw?.content || ''),
})

const noteBase = (id: string) => `api/v1/message-favorite-notes/${encodeURIComponent(id)}`
const notesBase = (favoriteId: string) => `api/v1/message-favorites/${encodeURIComponent(favoriteId)}/notes`

export const fetchMessageFavoriteNotes = async (favoriteId: string): Promise<MessageFavoriteNote[]> => {
  const resp = await api.get(notesBase(favoriteId))
  return Array.isArray(resp.data?.items) ? resp.data.items.map(normalizeNote) : []
}

export const createMessageFavoriteNote = async (favoriteId: string, content: string): Promise<MessageFavoriteNote> => {
  const resp = await api.post(notesBase(favoriteId), { content })
  return normalizeNote(resp.data?.item)
}

export const deleteMessageFavoriteNote = async (id: string) => {
  await api.delete(noteBase(id))
}

// 409 响应中的编辑锁冲突；其他错误返回 null。
export const parseMessageFavoriteNoteConflict = (error: unknown): MessageFavoriteNoteConflict | null => {
  const response = (error as any)?.response
  if (response?.status !== 409) return null
  const code = response?.data?.code
  if (code === 'note_locked') return { code, lock: normalizeNoteLock(response.data.lock) }
  if (code === 'note_lock_lost') return { code }
  return null
}

export const isMessageFavoriteMissing = (error: unknown) => (error as any)?.response?.status === 404

// 不带 token 时获取编辑权；带当前 token 时续租。
export const lockMessageFavoriteNote = async (id: string, token = ''): Promise<MessageFavoriteNoteLease> => {
  const resp = await api.post(`${noteBase(id)}/lock`, token ? { token } : {})
  return normalizeNoteLease(resp.data)
}

// 保存后服务端同时释放编辑权。
export const saveMessageFavoriteNote = async (id: string, token: string, content: string): Promise<{ content: string, updatedAt: string, changed: boolean }> => {
  const resp = await api.put(noteBase(id), { token, content })
  return {
    content: String(resp.data?.content || ''),
    updatedAt: String(resp.data?.updatedAt || ''),
    changed: resp.data?.changed === true,
  }
}

export const releaseMessageFavoriteNote = async (id: string, token: string) => {
  await api.post(`${noteBase(id)}/release`, { token })
}

// 页面卸载时尽力释放：keepalive 请求可在 pagehide 后继续发送。失败无妨，租约会自动过期。
export const releaseMessageFavoriteNoteOnUnload = (id: string, token: string) => {
  if (typeof fetch !== 'function') return
  try {
    void fetch(`${urlBase}/${noteBase(id)}/release`, buildAuthorizedJsonRequestInit({
      method: 'POST',
      keepalive: true,
      body: JSON.stringify({ token }),
    })).catch(() => undefined)
  } catch {
    // 浏览器拒绝 keepalive 请求时依赖租约过期。
  }
}
