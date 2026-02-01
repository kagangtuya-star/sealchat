import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useChatStore } from './chat'
import { useUserStore } from './user'
import { api } from './_config'
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render'

// 便签类型
export type StickyNoteType = 'text' | 'counter' | 'list' | 'slider' | 'chat' | 'timer' | 'clock' | 'roundCounter'

// 便签可见性
export type StickyNoteVisibility = 'all' | 'owner' | 'editors' | 'viewers'

// 类型特定数据结构
export interface CounterTypeData {
    value: number
    max?: number
}

export interface ListItem {
    id: string
    content: string
    checked: boolean
    indent: number
}

export interface ListTypeData {
    items: ListItem[]
}

export interface SliderTypeData {
    value: number
    min: number
    max: number
    step: number
}

export interface TimerTypeData {
    startTime: number
    baseValue: number
    direction: 'up' | 'down'
    running: boolean
    resetValue: number
}

export interface ClockTypeData {
    segments: number
    filled: number
}

export interface RoundCounterTypeData {
    round: number
    direction: 'up' | 'down'
    limit?: number
}

export type StickyNoteTypeData = CounterTypeData | ListTypeData | SliderTypeData | TimerTypeData | ClockTypeData | RoundCounterTypeData | null

// 便签类型定义
export interface StickyNote {
    id: string
    channelId: string
    worldId: string
    folderId?: string
    title: string
    content: string
    contentText: string
    color: string
    creatorId: string
    isPublic: boolean
    isPinned: boolean
    orderIndex: number
    noteType: StickyNoteType
    typeData: string
    visibility: StickyNoteVisibility
    viewerIds: string
    editorIds: string
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

// 便签文件夹类型
export interface StickyNoteFolder {
    id: string
    channelId: string
    worldId: string
    parentId?: string
    name: string
    color?: string
    orderIndex: number
    creatorId: string
    createdAt: number
    updatedAt: number
    children?: StickyNoteFolder[]
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

export interface StickyNotePushLayout {
    xPct: number
    yPct: number
    wPct: number
    hPct: number
}

export interface StickyNoteWithState {
    note: StickyNote
    userState?: StickyNoteUserState
}

interface StickyNoteLocalCache {
    version: number
    uiVisible: boolean
    privateCreateEnabled?: boolean
    notes: StickyNote[]
    userStates: StickyNoteUserState[]
    activeNoteIds: string[]
}

const LOCAL_CACHE_VERSION = 1
const STORAGE_KEY_PREFIX = 'sealchat_sticky_notes'
const MIN_NOTE_WIDTH = 200
const MIN_NOTE_HEIGHT = 150
const VIEWPORT_PADDING = 8
let viewportListenerBound = false

export const useStickyNoteStore = defineStore('stickyNote', () => {
    const userStore = useUserStore()
    const chatStore = useChatStore()

    // 当前频道的便签
    const notes = ref<Record<string, StickyNote>>({})
    // 文件夹
    const folders = ref<Record<string, StickyNoteFolder>>({})
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
    // 新建便签仅自己可见开关
    const privateCreateEnabled = ref(false)
    // 每频道远端持久化开关缓存
    const persistRemoteStateByChannel = ref<Record<string, boolean>>({})

    // 计算属性
    const noteList = computed(() => Object.values(notes.value))
    const folderList = computed(() => Object.values(folders.value))

    const activeNotes = computed(() =>
        activeNoteIds.value
            .map(id => notes.value[id])
            .filter(Boolean)
    )

    const pinnedNotes = computed(() =>
        noteList.value.filter(note => note.isPinned)
    )

    // 按文件夹分组便签
    const notesByFolder = computed(() => {
        const result: Record<string, StickyNote[]> = { '': [] }
        for (const folder of folderList.value) {
            result[folder.id] = []
        }
        for (const note of noteList.value) {
            const folderId = note.folderId || ''
            if (!result[folderId]) result[folderId] = []
            result[folderId].push(note)
        }
        return result
    })

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
        privateCreateEnabled.value = false

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
        if (typeof cache.privateCreateEnabled === 'boolean') {
            privateCreateEnabled.value = cache.privateCreateEnabled
        }
    }

    function persistLocalCache() {
        if (typeof window === 'undefined') return
        const key = buildLocalCacheKey(currentChannelId.value)
        if (!key) return
        const payload: StickyNoteLocalCache = {
            version: LOCAL_CACHE_VERSION,
            uiVisible: uiVisible.value,
            privateCreateEnabled: privateCreateEnabled.value,
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
            folders.value = {}
            userStates.value = {}
            activeNoteIds.value = []
            maxZIndex.value = 1000
            privateCreateEnabled.value = false
            const storedVisible = readUiVisible(channelId)
            if (storedVisible === null) {
                uiVisible.value = false
            } else {
                uiVisible.value = storedVisible
            }
        }

        try {
            // 并行加载便签和文件夹
            const [notesResponse, foldersResponse] = await Promise.all([
                api.get(`api/v1/channels/${channelId}/sticky-notes`),
                api.get(`api/v1/channels/${channelId}/sticky-note-folders`).catch(() => ({ data: { folders: [] } }))
            ])
            const items: StickyNoteWithState[] = notesResponse.data.items || []
            const folderItems: StickyNoteFolder[] = foldersResponse.data.folders || []

            // 加载文件夹
            const mergedFolders: Record<string, StickyNoteFolder> = {}
            for (const folder of folderItems) {
                if (folder?.id) {
                    mergedFolders[folder.id] = folder
                }
            }
            folders.value = mergedFolders

            const existingStates: Record<string, StickyNoteUserState> = { ...userStates.value }
            const existingActive = new Set(activeNoteIds.value)
            const mergedNotes: Record<string, StickyNote> = {}
            const mergedStates: Record<string, StickyNoteUserState> = {}
            const mergedActive = new Set<string>()
            let mergedMaxZIndex = 1000

            // 填充数据
            for (const item of items) {
                if (!item?.note?.id) continue
                const noteId = item.note.id
                mergedNotes[noteId] = item.note
                const state = item.userState || existingStates[noteId]
                if (state) {
                    mergedStates[noteId] = state
                    if (typeof state.zIndex === 'number' && state.zIndex > mergedMaxZIndex) {
                        mergedMaxZIndex = state.zIndex
                    }
                }
                if (existingActive.has(noteId)) {
                    mergedActive.add(noteId)
                }
                if (!hasLocalCache && item.userState?.isOpen) {
                    mergedActive.add(noteId)
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
        noteType?: StickyNoteType
        typeData?: string
        visibility?: StickyNoteVisibility
        viewerIds?: string
        editorIds?: string
        folderId?: string
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
        // 如果 content 是 TipTap JSON，自动生成纯文本版本用于搜索
        if (updates.content && isTipTapJson(updates.content)) {
            updates.contentText = tiptapJsonToPlainText(updates.content)
        }

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
        const shouldClamp =
            'positionX' in updates ||
            'positionY' in updates ||
            'width' in updates ||
            'height' in updates
        const clampedUpdates = shouldClamp ? { ...updates, ...clampNoteState(noteId, updates) } : updates

        Object.assign(userStates.value[noteId], clampedUpdates)
        persistLocalCache()

        if (options?.persistRemote === false) {
            return
        }

        if (!(await shouldPersistUserStateRemote())) {
            return
        }

        // 后台保存
        try {
            await api.patch(`api/v1/sticky-notes/${noteId}/state`, clampedUpdates)
        } catch (err) {
            console.error('保存便签状态失败:', err)
        }
    }

    // 推送便签
    async function pushNote(noteId: string, targetUserIds: string[], layout?: StickyNotePushLayout) {
        try {
            const payload: { targetUserIds: string[]; layout?: StickyNotePushLayout } = { targetUserIds }
            if (layout) {
                payload.layout = layout
            }
            await api.post(`api/v1/sticky-notes/${noteId}/push`, payload)
            return true
        } catch (err) {
            console.error('推送便签失败:', err)
            return false
        }
    }

    // 迁移/复制便签
    async function migrateNotes(targetIds: string[], noteIds: string[], mode: 'copy' | 'move') {
        const channelId = currentChannelId.value
        if (!channelId) {
            throw new Error('未选择频道')
        }
        if (!targetIds.length) {
            throw new Error('请选择目标频道')
        }
        try {
            await api.post(`api/v1/channels/${channelId}/sticky-notes/migrate`, {
                targetChannelIds: targetIds,
                noteIds,
                mode
            })
            return true
        } catch (err) {
            console.error('迁移/复制便签失败:', err)
            return false
        }
    }

    // 打开便签
    function openNote(noteId: string, options?: { persistRemote?: boolean; state?: Partial<StickyNoteUserState> }) {
        if (!activeNoteIds.value.includes(noteId)) {
            activeNoteIds.value.push(noteId)
        }
        bringToFront(noteId, options)
        const clamped = clampNoteState(noteId, options?.state)
        updateUserState(noteId, {
            isOpen: true,
            minimized: false,
            ...options?.state,
            ...clamped
        }, options)
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
    function bringToFront(noteId: string, options?: { persistRemote?: boolean }) {
        maxZIndex.value += 1
        updateUserState(noteId, { zIndex: maxZIndex.value }, options)
    }

    // 最小化便签
    function minimizeNote(noteId: string) {
        updateUserState(noteId, { minimized: true })
    }

    // 恢复便签
    function restoreNote(noteId: string) {
        const clamped = clampNoteState(noteId)
        updateUserState(noteId, { minimized: false, ...clamped })
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

    function clampNumber(value: number, min: number, max: number) {
        return Math.min(Math.max(value, min), max)
    }

    function getViewportSize() {
        if (typeof window === 'undefined') return null
        return {
            width: Math.max(window.innerWidth, 1),
            height: Math.max(window.innerHeight, 1)
        }
    }

    function clampNoteState(noteId: string, base?: Partial<StickyNoteUserState>) {
        const viewport = getViewportSize()
        if (!viewport) return {}

        const note = notes.value[noteId]
        const current = userStates.value[noteId]
        const rawW = base?.width ?? current?.width ?? note?.defaultW ?? 300
        const rawH = base?.height ?? current?.height ?? note?.defaultH ?? 250
        const width = clampNumber(rawW, MIN_NOTE_WIDTH, Math.max(MIN_NOTE_WIDTH, viewport.width - VIEWPORT_PADDING))
        const height = clampNumber(rawH, MIN_NOTE_HEIGHT, Math.max(MIN_NOTE_HEIGHT, viewport.height - VIEWPORT_PADDING))
        const maxX = Math.max(0, viewport.width - width)
        const maxY = Math.max(0, viewport.height - height)
        const rawX = base?.positionX ?? current?.positionX ?? note?.defaultX ?? 100
        const rawY = base?.positionY ?? current?.positionY ?? note?.defaultY ?? 100

        return {
            positionX: clampNumber(rawX, 0, maxX),
            positionY: clampNumber(rawY, 0, maxY),
            width,
            height
        }
    }

    function resetNotePosition(noteId: string, options?: { persistRemote?: boolean }) {
        const viewport = getViewportSize()
        if (!viewport) return

        const note = notes.value[noteId]
        const current = userStates.value[noteId]
        const rawW = current?.width ?? note?.defaultW ?? 300
        const rawH = current?.height ?? note?.defaultH ?? 250
        const width = clampNumber(rawW, MIN_NOTE_WIDTH, Math.max(MIN_NOTE_WIDTH, viewport.width - VIEWPORT_PADDING))
        const height = clampNumber(rawH, MIN_NOTE_HEIGHT, Math.max(MIN_NOTE_HEIGHT, viewport.height - VIEWPORT_PADDING))
        const positionX = Math.max(0, Math.round((viewport.width - width) / 2))
        const positionY = Math.max(0, Math.round((viewport.height - height) / 2))

        updateUserState(noteId, { positionX, positionY, width, height }, options)
    }

    function resetAllOpenNotes(options?: { persistRemote?: boolean }) {
        activeNoteIds.value.forEach(noteId => resetNotePosition(noteId, options))
    }

    function resolvePushLayout(layout?: StickyNotePushLayout): Partial<StickyNoteUserState> | null {
        if (!layout || typeof window === 'undefined') return null
        const { xPct, yPct, wPct, hPct } = layout
        if (![xPct, yPct, wPct, hPct].every(value => Number.isFinite(value))) {
            return null
        }
        const viewportW = Math.max(window.innerWidth, 1)
        const viewportH = Math.max(window.innerHeight, 1)
        const rawW = Math.round(wPct * viewportW)
        const rawH = Math.round(hPct * viewportH)
        const width = clampNumber(rawW, MIN_NOTE_WIDTH, Math.max(MIN_NOTE_WIDTH, viewportW))
        const height = clampNumber(rawH, MIN_NOTE_HEIGHT, Math.max(MIN_NOTE_HEIGHT, viewportH))
        const maxX = Math.max(0, viewportW - width)
        const maxY = Math.max(0, viewportH - height)
        const rawX = Math.round(xPct * viewportW)
        const rawY = Math.round(yPct * viewportH)
        return {
            positionX: clampNumber(rawX, 0, maxX),
            positionY: clampNumber(rawY, 0, maxY),
            width,
            height
        }
    }

    // 处理WebSocket事件
    function handleStickyNoteEvent(event: any) {
        const payload = event.stickyNote
        if (!payload) return

        const { note, action, targetUserIds, layout } = payload

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
                    const layoutState = resolvePushLayout(layout)
                    openNote(note.id, { persistRemote: false, state: layoutState || undefined })
                }
                break
        }
    }

    let resizeTimer: number | null = null

    function scheduleViewportClamp() {
        if (typeof window === 'undefined') return
        if (resizeTimer !== null) {
            window.clearTimeout(resizeTimer)
        }
        resizeTimer = window.setTimeout(() => {
            activeNoteIds.value.forEach(noteId => {
                const clamped = clampNoteState(noteId)
                updateUserState(noteId, clamped, { persistRemote: false })
            })
            resizeTimer = null
        }, 120)
    }

    if (typeof window !== 'undefined' && !viewportListenerBound) {
        viewportListenerBound = true
        window.addEventListener('resize', scheduleViewportClamp)
        window.addEventListener('orientationchange', scheduleViewportClamp)
    }

    function setVisible(value: boolean) {
        uiVisible.value = value
        writeUiVisible(currentChannelId.value, value)
        persistLocalCache()
    }

    function toggleVisible() {
        setVisible(!uiVisible.value)
    }

    function setPrivateCreateEnabled(value: boolean) {
        privateCreateEnabled.value = value
        persistLocalCache()
    }

    // 清理状态
    function reset() {
        notes.value = {}
        folders.value = {}
        userStates.value = {}
        activeNoteIds.value = []
        editingNoteId.value = null
        currentChannelId.value = ''
        maxZIndex.value = 1000
        loading.value = false
        uiVisible.value = false
        privateCreateEnabled.value = false
    }

    // 解析 typeData
    function parseTypeData<T extends StickyNoteTypeData>(note: StickyNote): T | null {
        if (!note.typeData) return null
        try {
            return JSON.parse(note.typeData) as T
        } catch {
            return null
        }
    }

    // 更新 typeData
    async function updateTypeData(noteId: string, typeData: StickyNoteTypeData) {
        const typeDataStr = JSON.stringify(typeData)
        await updateNote(noteId, { typeData: typeDataStr })
    }

    // 获取默认 typeData
    function getDefaultTypeData(noteType: StickyNoteType): StickyNoteTypeData {
        switch (noteType) {
            case 'counter':
                return { value: 0 } as CounterTypeData
            case 'list':
                return { items: [] } as ListTypeData
            case 'slider':
                return { value: 50, min: 0, max: 100, step: 1 } as SliderTypeData
            case 'timer':
                return { startTime: 0, baseValue: 0, direction: 'up', running: false, resetValue: 0 } as TimerTypeData
            case 'clock':
                return { segments: 4, filled: 0 } as ClockTypeData
            case 'roundCounter':
                return { round: 0, direction: 'up' } as RoundCounterTypeData
            default:
                return null
        }
    }

    // ===== 文件夹操作 =====

    // 创建文件夹
    async function createFolder(params: { name: string; parentId?: string; color?: string }) {
        if (!currentChannelId.value) return null
        try {
            const response = await api.post(`api/v1/channels/${currentChannelId.value}/sticky-note-folders`, params)
            const folder: StickyNoteFolder = response.data.folder
            folders.value[folder.id] = folder
            return folder
        } catch (err) {
            console.error('创建文件夹失败:', err)
            return null
        }
    }

    // 更新文件夹
    async function updateFolder(folderId: string, updates: Partial<StickyNoteFolder>) {
        try {
            const response = await api.patch(`api/v1/sticky-note-folders/${folderId}`, updates)
            folders.value[folderId] = response.data.folder
        } catch (err) {
            console.error('更新文件夹失败:', err)
        }
    }

    // 删除文件夹
    async function deleteFolder(folderId: string) {
        try {
            await api.delete(`api/v1/sticky-note-folders/${folderId}`)
            delete folders.value[folderId]
            // 清除便签的 folderId
            for (const note of Object.values(notes.value)) {
                if (note.folderId === folderId) {
                    note.folderId = ''
                }
            }
        } catch (err) {
            console.error('删除文件夹失败:', err)
        }
    }

    // 移动便签到文件夹
    async function moveNoteToFolder(noteId: string, folderId: string | null) {
        await updateNote(noteId, { folderId: folderId || '' })
    }

    return {
        // State
        notes,
        folders,
        userStates,
        activeNoteIds,
        editingNoteId,
        currentChannelId,
        loading,
        maxZIndex,
        uiVisible,
        privateCreateEnabled,

        // Computed
        noteList,
        folderList,
        notesByFolder,
        activeNotes,
        pinnedNotes,

        // Actions
        loadChannelNotes,
        createNote,
        updateNote,
        deleteNote,
        updateUserState,
        pushNote,
        migrateNotes,
        openNote,
        closeNote,
        bringToFront,
        minimizeNote,
        restoreNote,
        startEditing,
        stopEditing,
        resetNotePosition,
        resetAllOpenNotes,
        handleStickyNoteEvent,
        setVisible,
        toggleVisible,
        setPrivateCreateEnabled,
        reset,
        parseTypeData,
        updateTypeData,
        getDefaultTypeData,
        // 文件夹操作
        createFolder,
        updateFolder,
        deleteFolder,
        moveNoteToFolder
    }
})
