import { defineStore } from 'pinia'
import { api } from './_config'

export interface ChannelImageItem {
    id: string
    messageId: string
    attachmentId: string
    thumbUrl: string
    senderId: string
    senderName: string
    senderAvatar: string
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
    thumbnailMode: 'small' | 'large'  // 小图/大图模式
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
        thumbnailMode: 'large',  // 默认大图模式
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

        setThumbnailMode(mode: 'small' | 'large') {
            this.thumbnailMode = mode
        },

        toggleThumbnailMode() {
            this.thumbnailMode = this.thumbnailMode === 'large' ? 'small' : 'large'
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
                const resp = await api.get<ChannelImagesApiResponse>(
                    `api/v1/channels/${this.channelId}/images`,
                    {
                        params: {
                            page: this.page,
                            page_size: this.pageSize,
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
            this.thumbnailMode = 'large'
        },
    },
})
