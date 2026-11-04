import { computed, effectScope, onScopeDispose, reactive, ref } from 'vue'
import {
  MESSAGE_FAVORITE_NOTE_MAX,
  createMessageFavoriteNote,
  deleteMessageFavoriteNote,
  describeMessageFavoriteError,
  fetchMessageFavoriteNotes,
  messageFavoriteNoteLength,
  type MessageFavoriteItem,
  type MessageFavoriteNote,
} from './api'
import { messageFavoriteBucketKey, useMessageFavoriteStore } from './store'
import { useFavoriteNoteEditor } from './useFavoriteNoteEditor'

// 窗口持有帖子及草稿，Inspector 的 split/overlay 切换不销毁编辑会话。
// 每帖复用原来的租约状态机；新帖草稿完全独立，不获取租约。
export const useFavoriteNotes = (options: { heartbeatMs?: number } = {}) => {
  const store = useMessageFavoriteStore()
  const notes = ref<MessageFavoriteNote[]>([])
  const loading = ref(false)
  const error = ref('')
  const composer = reactive({ draft: '', saving: false, error: '' })
  const entries = new Map<string, { editor: ReturnType<typeof useFavoriteNoteEditor>, scope: ReturnType<typeof effectScope> }>()
  const revision = ref(0)
  let target: MessageFavoriteItem | null = null
  let context = 0
  let request = 0
  let mutation = 0

  const changed = (item: MessageFavoriteItem) => store.scheduleRefresh(messageFavoriteBucketKey(item.scope, item.worldId))
  const existingEditorFor = (id: string) => {
    void revision.value
    return entries.get(id)?.editor || null
  }
  const editorFor = (note: MessageFavoriteNote) => {
    let entry = entries.get(note.id)
    if (!entry) {
      const owner = effectScope(true)
      const item = target!
      const editor = owner.run(() => useFavoriteNoteEditor({
        ...options,
        onLockChange: (id, lock) => {
          const stored = notes.value.find((post) => post.id === id)
          if (stored) stored.noteLock = lock
        },
        onSaved: (id, result) => {
          mutation += 1
          const stored = notes.value.find((post) => post.id === id)
          if (stored) Object.assign(stored, { content: result.content, updatedAt: result.updatedAt, noteLock: null })
          if (result.changed) changed(item)
        },
      }))!
      entry = { editor, scope: owner }
      entries.set(note.id, entry)
      revision.value += 1
    }
    return entry.editor
  }

  const dirty = computed(() => {
    void revision.value
    return !!composer.draft.trim() || Array.from(entries.values()).some(({ editor }) => editor.dirty.value)
  })
  const busy = computed(() => {
    void revision.value
    return dirty.value || composer.saving || Array.from(entries.values()).some(({ editor }) => editor.busy.value)
  })
  const pending = computed(() => {
    void revision.value
    return composer.saving || Array.from(entries.values()).some(({ editor }) => editor.state.status === 'saving')
  })
  const composerLength = computed(() => messageFavoriteNoteLength(composer.draft))
  const composerOverLimit = computed(() => composerLength.value > MESSAGE_FAVORITE_NOTE_MAX)

  const cancel = () => {
    context += 1
    request += 1
    for (const entry of entries.values()) entry.scope.stop()
    entries.clear()
    revision.value += 1
    target = null
    notes.value = []
    loading.value = false
    error.value = ''
    Object.assign(composer, { draft: '', saving: false, error: '' })
  }

  const refresh = async () => {
    if (!target) return
    const id = target.id
    const current = context
    const seq = ++request
    const beforeMutation = mutation
    loading.value = true
    error.value = ''
    try {
      const items = await fetchMessageFavoriteNotes(id)
      if (current !== context || seq !== request) return
      // A list started before a successful local mutation must not resurrect
      // deleted content or replace a freshly saved post.
      if (mutation !== beforeMutation) { void refresh(); return }
      const ids = new Set(items.map((note) => note.id))
      for (const old of notes.value) {
        if (!ids.has(old.id) && entries.get(old.id)?.editor.busy.value) items.push(old)
      }
      let entriesChanged = false
      for (const [id, entry] of entries) {
        if (!items.some((note) => note.id === id)) {
          entry.scope.stop()
          entries.delete(id)
          entriesChanged = true
        }
      }
      if (entriesChanged) revision.value += 1
      notes.value = items
    } catch (cause) {
      if (current === context && seq === request) error.value = describeMessageFavoriteError(cause, '加载备注失败')
    } finally {
      if (current === context && seq === request) loading.value = false
    }
  }

  const load = async (item: MessageFavoriteItem) => {
    if (target?.id !== item.id) { cancel(); target = item }
    await refresh()
  }

  const publish = async () => {
    if (!target || composer.saving || !composerLength.value || composerOverLimit.value) return false
    const current = context
    const item = target
    composer.saving = true
    composer.error = ''
    try {
      const note = await createMessageFavoriteNote(item.id, composer.draft)
      if (current !== context) return false
      mutation += 1
      notes.value.push(note)
      composer.draft = ''
      changed(item)
      return true
    } catch (cause) {
      if (current === context) composer.error = describeMessageFavoriteError(cause, '发布备注失败')
      return false
    } finally {
      if (current === context) composer.saving = false
    }
  }

  const remove = async (note: MessageFavoriteNote) => {
    if (!target || note.favoriteId !== target.id) return false
    const current = context
    const item = target
    try {
      await deleteMessageFavoriteNote(note.id)
      if (current !== context) return false
      mutation += 1
      entries.get(note.id)?.scope.stop()
      entries.delete(note.id)
      revision.value += 1
      notes.value = notes.value.filter((post) => post.id !== note.id)
      changed(item)
      return true
    } catch (cause) {
      if (current === context) error.value = describeMessageFavoriteError(cause, '删除备注失败')
      return false
    }
  }

  onScopeDispose(cancel)
  return { notes, loading, error, composer, composerLength, composerOverLimit, dirty, busy, pending, existingEditorFor, editorFor, load, refresh, publish, remove, cancel }
}
