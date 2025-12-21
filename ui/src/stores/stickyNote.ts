import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useChatStore } from './chat'
import { useUserStore } from './user'
import { api } from './_config'

// 便签类型定义
export interface StickyNote {
    id: string
    channelId: string
    worldId: string
    title: string
    content: string
    contentText: string
    color: string
    creatorId: string
    isPublic: boolean
    isPinned: boolean
    orderIndex: number
    defaultX: number
    defaultY: number
    defaultW: number
    defaultH: number
    createdAt: number
    updatedAt: number
    creator?: {
        id: string
        nickname?: string
        nick?: string
        name?: string
        avatar: string
    }
}

export interface StickyNoteUserState {
    noteId: string
    isOpen: boolean
    positionX: number
    positionY: number
    width: number
    height: number
    minimized: boolean
    zIndex: number
}

export interface StickyNoteWithState {
    note: StickyNote
    userState?: StickyNoteUserState
}

interface StickyNoteLocalCache {
    version: number
    uiVisible: boolean
    notes: StickyNote[]
    userStates: StickyNoteUserState[]
    activeNoteIds: string[]
}

const LOCAL_CACHE_VERSION = 1
const STORAGE_KEY_PREFIX = 'sealchat_sticky_notes'

export const useStickyNoteStore = defineStore('stickyNote', () => {
    const userStore = useUserStore()
    const chatStore = useChatStore()

    // 当前频道的便签
    const notes = ref<Record<string, StickyNote>>({})
    // 用户状态
    const userStates = ref<Record<string, StickyNoteUserState>>({})
    // 当前打开的便签ID列表
    const activeNoteIds = ref<string[]>([])
    // 当前正在编辑的便签ID
    const editingNoteId = ref<string | null>(null)
    // 当前频道ID
    const currentChannelId = ref<string>('')
    // 最大z-index
    const maxZIndex = ref(1000)
    // 加载状态
    const loading = ref(false)
    // 便签界面可见状态
    const uiVisible = ref(false)
    // 每频道远端持久化开关缓存
    const persistRemoteStateByChannel = ref<Record<string, boolean>>({})

    // 计算属性
    const noteList = computed(() => Object.values(notes.value))

    const activeNotes = computed(() =>
        activeNoteIds.value
            .map(id => notes.value[id])
            .filter(Boolean)
    )

    const pinnedNotes = computed(() =>
        noteList.value.filter(note => note.isPinned)
    )

    async function shouldPersistUserStateRemote() {
        const channelId = currentChannelId.value
        const userId = userStore.info?.id
        if (!channelId || !userId) {
            return false
        }
        if (persistRemoteStateByChannel.value[channelId] !== undefined) {
            return persistRemoteStateByChannel.value[channelId]
        }
        try {
            await chatStore.ensureChannelPermissionCache(channelId)
        } catch {
            return false
        }
        const allowed = chatStore.isChannelAdmin(channelId, userId) || chatStore.isChannelOwner(channelId, userId)
        persistRemoteStateByChannel.value = {
            ...persistRemoteStateByChannel.value,
            [channelId]: allowed
        }
        return allowed
    }

    function buildLocalCacheKey(channelId: string) {
        const userId = userStore.info?.id
        if (!channelId || !userId) return ''
        return `${STORAGE_KEY_PREFIX}:${userId}:${channelId}`
    }

    function readLocalCache(channelId: string): StickyNoteLocalCache | null {
        if (typeof window === 'undefined') return null
        const key = buildLocalCacheKey(channelId)
        if (!key) return null
        try {
            const raw = localStorage.getItem(key)
            if (!raw) return null
            const parsed = JSON.parse(raw) as StickyNoteLocalCache
            if (!parsed || typeof parsed !== 'object') return null
            return parsed
        } catch {
            return null
        }
    }

    function applyLocalCache(cache: StickyNoteLocalCache) {
        notes.value = {}
        userStates.value = {}
        activeNoteIds.value = []
        maxZIndex.value = 1000

        if (Array.isArray(cache.notes)) {
            for (const note of cache.notes) {
                if (note?.id) {
                    notes.value[note.id] = note
                }
            }
        }

        if (Array.isArray(cache.userStates)) {
            for (const state of cache.userStates) {
                if (state?.noteId) {
                    userStates.value[state.noteId] = state
                    if (typeof state.zIndex === 'number' && state.zIndex > maxZIndex.value) {
                        maxZIndex.value = state.zIndex
                    }
                }
            }
        }

        if (Array.isArray(cache.activeNoteIds)) {
            activeNoteIds.value = cache.activeNoteIds.filter(id => !!notes.value[id])
            for (const noteId of activeNoteIds.value) {
                if (userStates.value[noteId]) {
                    userStates.value[noteId].isOpen = true
                }
            }
        }

        if (typeof cache.uiVisible === 'boolean') {
            uiVisible.value = cache.uiVisible
        }
    }

    function persistLocalCache() {
        if (typeof window === 'undefined') return
        const key = buildLocalCacheKey(currentChannelId.value)
        if (!key) return
        const payload: StickyNoteLocalCache = {
            version: LOCAL_CACHE_VERSION,
            uiVisible: uiVisible.value,
            notes: Object.values(notes.value),
            userStates: Object.values(userStates.value),
            activeNoteIds: activeNoteIds.value.slice()
        }
        try {
            localStorage.setItem(key, JSON.stringify(payload))
        } catch (error) {
            console.warn('便签缓存写入失败', error)
        }
    }

    function buildUiVisibleKey(channelId: string) {
        const userId = userStore.info?.id
        if (!channelId || !userId) return ''
        return `sticky-note-ui-visible:${userId}:${channelId}`
    }

    function readUiVisible(channelId: string): boolean | null {
        if (typeof window === 'undefined') return null
        const key = buildUiVisibleKey(channelId)
        if (!key) return null
        try {
            const raw = localStorage.getItem(key)
            if (raw === null) return null
            return raw === 'true'
        } catch {
            return null
        }
    }

    function writeUiVisible(channelId: string, value: boolean) {
        if (typeof window === 'undefined') return
        const key = buildUiVisibleKey(channelId)
        if (!key) return
        try {
            localStorage.setItem(key, String(value))
        } catch {
            // ignore
        }
    }

    // Authorization header 由 _config.ts 拦截器自动注入

    // 加载频道便签
    async function loadChannelNotes(channelId: string) {
        if (!channelId) return
        currentChannelId.value = channelId
        loading.value = true

        const localCache = readLocalCache(channelId)
        const hasLocalCache = !!localCache
        if (localCache) {
            applyLocalCache(localCache)
            if (typeof localCache.uiVisible !== 'boolean') {
                const storedVisible = readUiVisible(channelId)
                if (storedVisible !== null) {
                    uiVisible.value = storedVisible
                }
            }
        } else {
            notes.value = {}
            userStates.value = {}
            activeNoteIds.value = []
            maxZIndex.value = 1000
            const storedVisible = readUiVisible(channelId)
            if (storedVisible === null) {
                uiVisible.value = false
            } else {
                uiVisible.value = storedVisible
            }
        }

        try {
            const response = await api.get(`api/v1/channels/${channelId}/sticky-notes`)
            const items: StickyNoteWithState[] = response.data.items || []

            const mergedNotes: Record<string, StickyNote> = { ...notes.value }
            const mergedStates: Record<string, StickyNoteUserState> = { ...userStates.value }
            const mergedActive = new Set(activeNoteIds.value)
            let mergedMaxZIndex = maxZIndex.value

            // 填充数据
            for (const item of items) {
                if (!item?.note?.id) continue
                const noteId = item.note.id
                const existing = mergedNotes[noteId]
                if (!existing || (typeof item.note.updatedAt === 'number' && typeof existing.updatedAt === 'number' && item.note.updatedAt > existing.updatedAt)) {
                    mergedNotes[noteId] = item.note
                } else if (!existing) {
                    mergedNotes[noteId] = item.note
                }
                if (!mergedStates[noteId] && item.userState) {
                    mergedStates[noteId] = item.userState
                }
                if (!hasLocalCache && item.userState?.isOpen) {
                    mergedActive.add(noteId)
                }
                if (typeof item.userState?.zIndex === 'number' && item.userState.zIndex > mergedMaxZIndex) {
                    mergedMaxZIndex = item.userState.zIndex
                }
            }

            notes.value = mergedNotes
            userStates.value = mergedStates
            activeNoteIds.value = Array.from(mergedActive).filter(id => !!mergedNotes[id])
            maxZIndex.value = mergedMaxZIndex

            if (!hasLocalCache) {
                const storedVisible = readUiVisible(channelId)
                if (storedVisible === null) {
                    uiVisible.value = activeNoteIds.value.length > 0
                } else {
                    uiVisible.value = storedVisible
                }
            }

            persistLocalCache()
        } catch (err) {
            const status = (err as any)?.response?.status
            if (status === 404) {
                return
            }
            console.error('加载便签失败:', err)
        } finally {
            loading.value = false
        }
    }

    // 创建便签
    async function createNote(params: {
        title?: string
        content?: string
        color?: string
        defaultX?: number
        defaultY?: number
        defaultW?: number
        defaultH?: number
    }) {
        if (!currentChannelId.value) return null

        try {
            const response = await api.post(`api/v1/channels/${currentChannelId.value}/sticky-notes`, params)
            const note: StickyNote = response.data.note

            // 添加到本地状态
            notes.value[note.id] = note

            // 自动打开新创建的便签
            openNote(note.id)

            return note
        } catch (err) {
            console.error('创建便签失败:', err)
            return null
        }
    }

    // 更新便签内容
    async function updateNote(noteId: string, updates: Partial<StickyNote>) {
        const existing = notes.value[noteId]
        if (existing) {
            notes.value[noteId] = {
                ...existing,
                ...updates
            }
            persistLocalCache()
        }
        try {
            const response = await api.patch(`api/v1/sticky-notes/${noteId}`, updates)
            notes.value[noteId] = response.data.note
            persistLocalCache()
        } catch (err) {
            console.error('更新便签失败:', err)
        }
    }

    // 删除便签
    async function deleteNote(noteId: string) {
        try {
            await api.delete(`api/v1/sticky-notes/${noteId}`)

            // 从本地状态移除
            delete notes.value[noteId]
            delete userStates.value[noteId]
            closeNoteLocal(noteId)
            persistLocalCache()
        } catch (err) {
            console.error('删除便签失败:', err)
        }
    }

    // 更新用户状态
    async function updateUserState(
        noteId: string,
        updates: Partial<StickyNoteUserState>,
        options?: { persistRemote?: boolean }
    ) {
        // 先更新本地状态
        if (!userStates.value[noteId]) {
            const note = notes.value[noteId]
            userStates.value[noteId] = {
                noteId,
                isOpen: false,
                positionX: note?.defaultX ?? 100,
                positionY: note?.defaultY ?? 100,
                width: note?.defaultW ?? 300,
                height: note?.defaultH ?? 250,
                minimized: false,
                zIndex: 1000
            }
        }
        Object.assign(userStates.value[noteId], updates)
        persistLocalCache()

        if (options?.persistRemote === false) {
            return
        }

        if (!(await shouldPersistUserStateRemote())) {
            return
        }

        // 后台保存
        try {
            await api.patch(`api/v1/sticky-notes/${noteId}/state`, updates)
        } catch (err) {
            console.error('保存便签状态失败:', err)
        }
    }

    // 推送便签
    async function pushNote(noteId: string, targetUserIds: string[]) {
        try {
            await api.post(`api/v1/sticky-notes/${noteId}/push`, { targetUserIds })
            return true
        } catch (err) {
            console.error('推送便签失败:', err)
            return false
        }
    }

    // 打开便签
    function openNote(noteId: string) {
        if (!activeNoteIds.value.includes(noteId)) {
            activeNoteIds.value.push(noteId)
        }
        bringToFront(noteId)
        updateUserState(noteId, { isOpen: true, minimized: false })
    }

    function closeNoteLocal(noteId: string) {
        const idx = activeNoteIds.value.indexOf(noteId)
        if (idx !== -1) {
            activeNoteIds.value.splice(idx, 1)
        }
        if (editingNoteId.value === noteId) {
            editingNoteId.value = null
        }
        persistLocalCache()
    }

    // 关闭便签
    function closeNote(noteId: string) {
        closeNoteLocal(noteId)
        updateUserState(noteId, { isOpen: false, minimized: false })
    }

    // 置顶便签
    function bringToFront(noteId: string) {
        maxZIndex.value += 1
        updateUserState(noteId, { zIndex: maxZIndex.value })
    }

    // 最小化便签
    function minimizeNote(noteId: string) {
        updateUserState(noteId, { minimized: true })
    }

    // 恢复便签
    function restoreNote(noteId: string) {
        updateUserState(noteId, { minimized: false })
        bringToFront(noteId)
    }

    // 开始编辑
    function startEditing(noteId: string) {
        editingNoteId.value = noteId
    }

    // 结束编辑
    function stopEditing() {
        editingNoteId.value = null
    }

    // 处理WebSocket事件
    function handleStickyNoteEvent(event: any) {
        const payload = event.stickyNote
        if (!payload) return

        const { note, action, targetUserIds } = payload

        switch (action) {
            case 'create':
                if (note && note.channelId === currentChannelId.value) {
                    notes.value[note.id] = note
                    persistLocalCache()
                }
                break
            case 'update':
                if (note && notes.value[note.id]) {
                    notes.value[note.id] = note
                    persistLocalCache()
                }
                break
            case 'delete':
                if (note) {
                    delete notes.value[note.id]
                    closeNoteLocal(note.id)
                }
                break
            case 'push':
                // 被推送的用户自动打开便签
                if (note) {
                    const userId = userStore.info?.id
                    const isTarget = !targetUserIds?.length || (!!userId && targetUserIds.includes(userId))
                    if (!isTarget) break
                    notes.value[note.id] = note
                    setVisible(true)
                    openNote(note.id)
                }
                break
        }
    }

    function setVisible(value: boolean) {
        uiVisible.value = value
        writeUiVisible(currentChannelId.value, value)
        persistLocalCache()
    }

    function toggleVisible() {
        setVisible(!uiVisible.value)
    }

    // 清理状态
    function reset() {
        notes.value = {}
        userStates.value = {}
        activeNoteIds.value = []
        editingNoteId.value = null
        currentChannelId.value = ''
        maxZIndex.value = 1000
        loading.value = false
        uiVisible.value = false
    }

    return {
        // State
        notes,
        userStates,
        activeNoteIds,
        editingNoteId,
        currentChannelId,
        loading,
        maxZIndex,
        uiVisible,

        // Computed
        noteList,
        activeNotes,
        pinnedNotes,

        // Actions
        loadChannelNotes,
        createNote,
        updateNote,
        deleteNote,
        updateUserState,
        pushNote,
        openNote,
        closeNote,
        bringToFront,
        minimizeNote,
        restoreNote,
        startEditing,
        stopEditing,
        handleStickyNoteEvent,
        setVisible,
        toggleVisible,
        reset
    }
})
