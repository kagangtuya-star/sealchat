import { computed, onScopeDispose, reactive } from 'vue'
import {
  MESSAGE_FAVORITE_NOTE_MAX,
  describeMessageFavoriteError,
  isMessageFavoriteMissing,
  lockMessageFavoriteNote,
  messageFavoriteNoteLength,
  normalizeMessageFavoriteNote,
  parseMessageFavoriteNoteConflict,
  releaseMessageFavoriteNote,
  releaseMessageFavoriteNoteOnUnload,
  saveMessageFavoriteNote,
  type MessageFavoriteNote,
  type MessageFavoriteNoteLock,
} from './api'

// 单条备注帖的编辑会话。每个帖子的凭据、心跳和草稿独立。
// 编辑权是后端短租约：取得后才进入编辑，
// 期间定时续租，保存或取消时释放；异常退出依赖租约自动过期。
// 锁凭据只保存在本闭包内，不进入 store、列表或任何广播。
//
// 状态：
//   idle      查看备注，未持有编辑权
//   acquiring 正在获取编辑权
//   editing   持有编辑权，可修改草稿
//   saving    保存请求进行中（服务端保存成功即释放编辑权）
//   locked    其他窗口或成员正在编辑，备注保持只读
//   lost      编辑权已失效（被接管或过期后被他人取得），草稿保留供复制或重试

export type FavoriteNoteEditorStatus = 'idle' | 'acquiring' | 'editing' | 'saving' | 'locked' | 'lost'

export interface FavoriteNoteEditorState {
  noteId: string
  status: FavoriteNoteEditorStatus
  draft: string
  // 取得编辑权时服务端的备注，用于判断草稿是否有改动。
  base: string
  holder: MessageFavoriteNoteLock | null
  error: string
  // 重新取得编辑权时发现备注已被他人修改；草稿仍是本地版本。
  remoteChanged: boolean
  remoteNote: string
}

const HEARTBEAT_MIN_MS = 5000

// 编辑相关状态：这些状态下窗口需要保护草稿或编辑权，切换前确认。
export const isFavoriteNoteSessionBusy = (status: FavoriteNoteEditorStatus) => (
  status === 'acquiring' || status === 'editing' || status === 'saving' || status === 'lost'
)

export const useFavoriteNoteEditor = (options: {
  heartbeatMs?: number
  onSaved?: (id: string, result: { content: string, updatedAt: string, changed: boolean }) => void
  onLockChange?: (id: string, lock: MessageFavoriteNoteLock | null) => void
} = {}) => {
  const state = reactive<FavoriteNoteEditorState>({
    noteId: '',
    status: 'idle',
    draft: '',
    base: '',
    holder: null,
    error: '',
    remoteChanged: false,
    remoteNote: '',
  })
  let token = ''
  let session = 0
  let heartbeat: ReturnType<typeof setTimeout> | undefined

  const length = computed(() => messageFavoriteNoteLength(state.draft))
  const overLimit = computed(() => length.value > MESSAGE_FAVORITE_NOTE_MAX)
  const dirty = computed(() => (
    (state.status === 'acquiring' || state.status === 'editing' || state.status === 'saving' || state.status === 'lost')
    && normalizeMessageFavoriteNote(state.draft) !== normalizeMessageFavoriteNote(state.base)
  ))
  const busy = computed(() => isFavoriteNoteSessionBusy(state.status))

  const stopHeartbeat = () => {
    if (heartbeat !== undefined) clearTimeout(heartbeat)
    heartbeat = undefined
  }

  const loseLease = (message: string) => {
    stopHeartbeat()
    token = ''
    state.status = 'lost'
    state.error = message
  }

  const scheduleHeartbeat = (ttlSeconds: number) => {
    stopHeartbeat()
    const delay = options.heartbeatMs ?? Math.max(HEARTBEAT_MIN_MS, Math.floor((ttlSeconds * 1000) / 3))
    const current = session
    heartbeat = setTimeout(async () => {
      heartbeat = undefined
      if (current !== session || state.status !== 'editing' || !token) return
      const id = state.noteId
      try {
        const lease = await lockMessageFavoriteNote(id, token)
        if (current !== session || state.status !== 'editing') return
        if (state.error) state.error = ''
        scheduleHeartbeat(lease.ttlSeconds)
      } catch (error) {
        if (current !== session || state.status !== 'editing') return
        if (parseMessageFavoriteNoteConflict(error) || isMessageFavoriteMissing(error)) {
          loseLease(isMessageFavoriteMissing(error) ? '该备注或收藏已被移除' : '编辑权已失效，备注可能正被其他窗口编辑')
          return
        }
        // 网络抖动时继续尝试；若租约已经过期，下一次成功请求会进入 lost。
        state.error = '网络异常，正在重试保持编辑权'
        scheduleHeartbeat(ttlSeconds)
      }
    }, delay)
  }

  const releaseCurrent = () => {
    stopHeartbeat()
    const id = state.noteId
    const held = token
    token = ''
    // 保存请求会在服务端释放编辑权；此时再释放只会和保存竞争。
    if (held && id && state.status !== 'saving') {
      options.onLockChange?.(id, null)
      void releaseMessageFavoriteNote(id, held).catch(() => undefined)
    }
  }

  const resetState = (noteId = '') => {
    Object.assign(state, {
      noteId,
      status: 'idle',
      draft: '',
      base: '',
      holder: null,
      error: '',
      remoteChanged: false,
      remoteNote: '',
    } satisfies FavoriteNoteEditorState)
  }

  // 获取编辑权。lost 状态下重试时保留草稿；若备注已被他人更新则标记 remoteChanged。
  async function begin(item: MessageFavoriteNote) {
    if (state.noteId === item.id && (state.status === 'acquiring' || state.status === 'editing' || state.status === 'saving')) return
    const retry = state.noteId === item.id && state.status === 'lost'
    const keptDraft = state.draft
    const keptBase = state.base
    if (state.noteId !== item.id) releaseCurrent()
    const current = ++session
    resetState(item.id)
    state.status = 'acquiring'
    state.draft = retry ? keptDraft : item.content
    state.base = retry ? keptBase : item.content
    let lease
    try {
      lease = await lockMessageFavoriteNote(item.id)
    } catch (error) {
      if (current !== session) return
      const conflict = parseMessageFavoriteNoteConflict(error)
      if (conflict?.code === 'note_locked') {
        state.status = retry ? 'lost' : 'locked'
        state.holder = conflict.lock
        state.error = retry ? '备注正在其他窗口编辑，稍后可重试' : ''
        options.onLockChange?.(item.id, conflict.lock)
        return
      }
      state.status = retry ? 'lost' : 'idle'
      state.error = isMessageFavoriteMissing(error) ? '该备注或收藏已被移除' : describeMessageFavoriteError(error, '无法获取编辑权')
      return
    }
    if (current !== session) {
      // 请求期间已切换收藏或取消：立即归还刚取得的编辑权。
      if (lease.token) void releaseMessageFavoriteNote(item.id, lease.token).catch(() => undefined)
      return
    }
    token = lease.token
    options.onLockChange?.(item.id, null)
    if (retry) {
      state.remoteChanged = normalizeMessageFavoriteNote(lease.content) !== normalizeMessageFavoriteNote(keptBase)
      state.remoteNote = lease.content
      state.base = lease.content
    } else {
      state.base = lease.content
      state.draft = lease.content
    }
    state.status = 'editing'
    scheduleHeartbeat(lease.ttlSeconds)
  }

  async function save() {
    if (state.status !== 'editing' || !token || overLimit.value || !length.value) return false
    const current = session
    const id = state.noteId
    state.status = 'saving'
    state.error = ''
    stopHeartbeat()
    try {
      const result = await saveMessageFavoriteNote(id, token, state.draft)
      if (current === session) {
        token = ''
        options.onSaved?.(id, result)
        resetState(id)
      }
      return true
    } catch (error) {
      if (current !== session) return false
      if (parseMessageFavoriteNoteConflict(error)) {
        loseLease('编辑权已失效，未保存。可复制草稿后重新编辑')
      } else if (isMessageFavoriteMissing(error)) {
        loseLease('该备注或收藏已被移除，备注未保存')
      } else {
        state.status = 'editing'
        state.error = describeMessageFavoriteError(error, '保存备注失败')
        scheduleHeartbeat(60)
      }
      return false
    }
  }

  // 放弃编辑或结束查看；释放失败不阻止界面关闭。
  function cancel() {
    session += 1
    releaseCurrent()
    resetState()
  }

  // 采用他人更新后的最新备注，丢弃本地草稿。
  function adoptRemote() {
    if (!state.remoteChanged) return
    state.draft = state.remoteNote
    state.remoteChanged = false
  }

  const handlePageHide = () => {
    if (token && state.noteId && state.status !== 'saving') releaseMessageFavoriteNoteOnUnload(state.noteId, token)
  }
  if (typeof window !== 'undefined') window.addEventListener('pagehide', handlePageHide)

  onScopeDispose(() => {
    if (typeof window !== 'undefined') window.removeEventListener('pagehide', handlePageHide)
    cancel()
  })

  return { state, length, overLimit, dirty, busy, begin, save, cancel, adoptRemote }
}
