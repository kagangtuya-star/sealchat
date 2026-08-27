import { defineStore } from 'pinia'
import { api } from './_config'
import { useChatStore } from './chat'
import {
    nextChannelImagesIcModeFilter,
    nextChannelImagesSortOrder,
    readChannelImagesIcModeFilter,
    readChannelImagesSortOrder,
    readChannelImagesThumbnailMode,
    writeChannelImagesIcModeFilter,
    writeChannelImagesSortOrder,
    writeChannelImagesThumbnailMode,
    type ChannelImagesIcModeFilter,
    type ChannelImagesSortOrder,
    type ChannelImagesThumbnailMode,
} from './channelImagesFilter'

export interface ChannelImageItem {
    id: string
    messageId: string
    attachmentId: string
    thumbUrl: string
    senderId: string
    senderName: string
    senderAvatar: string
    senderIdentityIsTemporary: boolean
    createdAt: number
    displayOrder: number
}

interface ChannelImagesState {
    panelVisible: boolean
    channelId: string | null
    items: ChannelImageItem[]
    loading: boolean
    loadingMore: boolean
    page: number
    pageSize: number
    total: number
    hasMore: boolean
    previewIndex: number | null
    thumbnailMode: ChannelImagesThumbnailMode  // 小图/大图模式
    icModeFilter: ChannelImagesIcModeFilter
    sortOrder: ChannelImagesSortOrder
}

interface ChannelImagesApiResponse {
    items: Array<{
        id: string
        message_id: string
        attachment_id: string
        thumb_url: string
        sender_id: string
        sender_name: string
        sender_avatar: string
        sender_identity_is_temporary: boolean
        created_at: number
        display_order: number
    }>
    total: number
    page: number
    page_size: number
    has_more: boolean
}

export const useChannelImagesStore = defineStore('channelImages', {
    state: (): ChannelImagesState => ({
        panelVisible: false,
        channelId: null,
        items: [],
        loading: false,
        loadingMore: false,
        page: 1,
        pageSize: 50,
        total: 0,
        hasMore: false,
        previewIndex: null,
        thumbnailMode: readChannelImagesThumbnailMode(),  // 默认大图模式
        icModeFilter: readChannelImagesIcModeFilter(),
        sortOrder: readChannelImagesSortOrder(),
    }),

    getters: {
        isEmpty: (state) => state.items.length === 0 && !state.loading,
        previewItem: (state): ChannelImageItem | null => {
            if (state.previewIndex === null || state.previewIndex < 0) {
                return null
            }
            return state.items[state.previewIndex] ?? null
        },
    },

    actions: {
        openPanel(channelId: string) {
            if (!channelId) return
            this.panelVisible = true
            if (this.channelId !== channelId) {
                this.channelId = channelId
                this.items = []
                this.page = 1
                this.total = 0
                this.hasMore = false
                this.previewIndex = null
                void this.loadImages()
            }
        },

        closePanel() {
            this.panelVisible = false
            this.previewIndex = null
        },

        togglePanel(channelId?: string) {
            if (this.panelVisible) {
                this.closePanel()
            } else if (channelId) {
                this.openPanel(channelId)
            }
        },

        setPreviewIndex(index: number | null) {
            this.previewIndex = index
        },

        nextPreview() {
            if (this.previewIndex === null) return
            if (this.previewIndex < this.items.length - 1) {
                this.previewIndex++
            }
        },

        prevPreview() {
            if (this.previewIndex === null) return
            if (this.previewIndex > 0) {
                this.previewIndex--
            }
        },

        setThumbnailMode(mode: ChannelImagesThumbnailMode) {
            this.thumbnailMode = mode
            writeChannelImagesThumbnailMode(mode)
        },

        toggleThumbnailMode() {
            this.thumbnailMode = this.thumbnailMode === 'large' ? 'small' : 'large'
        },

        setIcModeFilter(mode: ChannelImagesIcModeFilter) {
            if (this.icModeFilter === mode) return
            this.icModeFilter = mode
            writeChannelImagesIcModeFilter(mode)
            this.page = 1
            this.items = []
            this.total = 0
            this.hasMore = false
            this.previewIndex = null
            if (this.panelVisible && this.channelId) {
                void this.loadImages(true)
            }
        },

        cycleIcModeFilter() {
            this.setIcModeFilter(nextChannelImagesIcModeFilter(this.icModeFilter))
        },

        setSortOrder(order: ChannelImagesSortOrder) {
            if (this.sortOrder === order) return
            this.sortOrder = order
            writeChannelImagesSortOrder(order)
            this.page = 1
            this.items = []
            this.total = 0
            this.hasMore = false
            this.previewIndex = null
            if (this.panelVisible && this.channelId) {
                void this.loadImages(true)
            }
        },

        toggleSortOrder() {
            this.setSortOrder(nextChannelImagesSortOrder(this.sortOrder))
        },

        // 刷新图片列表（用于实时更新）
        async refresh() {
            if (!this.channelId || this.loading) return
            await this.loadImages(true)
        },

        async loadImages(reset = false) {
            if (!this.channelId) return

            if (reset) {
                this.page = 1
                this.items = []
            }

            this.loading = true
            try {
                const chat = useChatStore()
                const observerMode = chat.observerMode
                const observerSlug = observerMode ? String(chat.observerSlug || '').trim() : ''
                const endpoint = observerMode
                    ? `api/v1/public/ob/channels/${this.channelId}/images`
                    : `api/v1/channels/${this.channelId}/images`
                const resp = await api.get<ChannelImagesApiResponse>(
                    endpoint,
                    {
                        params: {
                            page: this.page,
                            page_size: this.pageSize,
                            ic_mode: this.icModeFilter,
                            sort: this.sortOrder,
                            ...(observerMode ? { ob_slug: observerSlug } : {}),
                        },
                    }
                )
                const data = resp.data
                const normalized = (data.items || []).map((item) => ({
                    id: item.id,
                    messageId: item.message_id,
                    attachmentId: item.attachment_id,
                    thumbUrl: item.thumb_url,
                    senderId: item.sender_id,
                    senderName: item.sender_name,
                    senderAvatar: item.sender_avatar,
                    senderIdentityIsTemporary: Boolean(item.sender_identity_is_temporary),
                    createdAt: item.created_at,
                    displayOrder: item.display_order,
                }))

                if (reset || this.page === 1) {
                    this.items = normalized
                } else {
                    // Merge avoiding duplicates
                    const existing = new Set(this.items.map((i) => i.id))
                    const newItems = normalized.filter((i) => !existing.has(i.id))
                    this.items = [...this.items, ...newItems]
                }

                this.total = data.total
                this.hasMore = data.has_more
            } catch (error) {
                console.error('加载频道图片失败', error)
            } finally {
                this.loading = false
            }
        },

        async loadMore() {
            if (!this.channelId || this.loadingMore || !this.hasMore) return

            this.loadingMore = true
            this.page++
            try {
                await this.loadImages()
            } finally {
                this.loadingMore = false
            }
        },

        reset() {
            this.panelVisible = false
            this.channelId = null
            this.items = []
            this.loading = false
            this.loadingMore = false
            this.page = 1
            this.total = 0
            this.hasMore = false
            this.previewIndex = null
            this.thumbnailMode = readChannelImagesThumbnailMode()
            this.icModeFilter = readChannelImagesIcModeFilter()
            this.sortOrder = readChannelImagesSortOrder()
        },
    },
})
