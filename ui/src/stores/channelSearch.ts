import { defineStore } from 'pinia'
import { api } from './_config'

export type ChannelSearchMatchMode = 'fuzzy' | 'exact'

export interface ChannelSearchFilters {
  speakerIds: string[]
  archived: 'all' | 'only' | 'exclude'
  icMode: 'all' | 'ic' | 'ooc'
  includeOutside: boolean
  timeRange: [number | null, number | null] | null
}

export interface ChannelSearchResult {
  id: string
  contentSnippet: string
  senderName: string
  senderAvatar?: string
  senderId?: string
  icMode: 'ic' | 'ooc'
  isArchived: boolean
  archivedAt?: number
  createdAt: number
  displayOrder?: number
  highlightRanges?: Array<[number, number]>
  keywordFragments?: { text: string; highlighted: boolean }[]
}

interface ChannelSearchState {
  panelVisible: boolean
  keyword: string
  lastKeyword: string
  matchMode: ChannelSearchMatchMode
  filters: ChannelSearchFilters
  page: number
  pageSize: number
  total: number
  loading: boolean
  error: string
  results: ChannelSearchResult[]
  currentChannelId: string | null
  requestSeq: number
  panelPosition: { x: number; y: number }
}

const defaultFilters = (): ChannelSearchFilters => ({
  speakerIds: [],
  archived: 'all',
  icMode: 'all',
  includeOutside: true,
  timeRange: null,
})

export const useChannelSearchStore = defineStore('channelSearch', {
  state: (): ChannelSearchState => ({
    panelVisible: false,
    keyword: '',
    lastKeyword: '',
    matchMode: 'fuzzy',
    filters: defaultFilters(),
    page: 1,
    pageSize: 10,
    total: 0,
    loading: false,
    error: '',
    results: [],
    currentChannelId: null,
    requestSeq: 0,
    panelPosition: {
      x: 48,
      y: 140,
    },
  }),

  getters: {
    totalPages: (state) => {
      if (state.pageSize <= 0) {
        return 1
      }
      return Math.max(1, Math.ceil(state.total / state.pageSize))
    },
    hasKeyword: (state) => state.keyword.trim().length > 0,
    isFilterActive: (state) => {
      const filters = state.filters
      return (
        filters.speakerIds.length > 0 ||
        filters.archived !== 'all' ||
        filters.icMode !== 'all' ||
        filters.includeOutside === false ||
        !!filters.timeRange
      )
    },
  },

  actions: {
    openPanel() {
      this.panelVisible = true
    },
    closePanel() {
      this.panelVisible = false
    },
    togglePanel() {
      this.panelVisible = !this.panelVisible
    },
    setKeyword(value: string) {
      this.keyword = value
    },
    setMatchMode(mode: ChannelSearchMatchMode) {
      this.matchMode = mode
    },
    updateFilters(payload: Partial<ChannelSearchFilters>) {
      this.filters = {
        ...this.filters,
        ...payload,
      }
    },
    resetFilters() {
      this.filters = defaultFilters()
    },
    setPage(page: number) {
      this.page = Math.max(1, page)
    },
    setPanelPosition(position: { x: number; y: number }) {
      this.panelPosition = { ...position }
    },
    bindChannel(channelId: string | null | undefined) {
      if (!channelId) {
        this.currentChannelId = null
        this.results = []
        this.total = 0
        return
      }
      if (this.currentChannelId !== channelId) {
        this.currentChannelId = channelId
        this.results = []
        this.total = 0
        this.page = 1
        this.lastKeyword = ''
        this.error = ''
      }
    },
    async search(channelId?: string) {
      const activeChannel = channelId ?? this.currentChannelId
      if (!activeChannel) {
        this.error = '请选择频道后再搜索'
        return
      }
      const keyword = this.keyword.trim()
      if (!keyword) {
        this.results = []
        this.total = 0
        this.error = ''
        this.lastKeyword = ''
        return
      }

      const seq = ++this.requestSeq
      this.loading = true
      this.error = ''

      const params: Record<string, any> = {
        keyword,
        match_mode: this.matchMode,
        page: this.page,
        page_size: this.pageSize,
      }

      if (this.filters.speakerIds.length) {
        params.speaker_ids = this.filters.speakerIds
      }
      if (this.filters.archived !== 'all') {
        params.archived = this.filters.archived
      }
      if (this.filters.icMode !== 'all') {
        params.ic_mode = this.filters.icMode
      }
      if (this.filters.includeOutside === false) {
        params.include_outside = false
      }
      if (this.filters.timeRange) {
        params.time_start = this.filters.timeRange[0]
        params.time_end = this.filters.timeRange[1]
      }

      try {
        const resp = await api.get(`api/v1/channels/${activeChannel}/messages/search`, {
          params,
        })
        if (seq !== this.requestSeq) {
          return
        }
        const payload = resp?.data ?? {}
        const items: ChannelSearchResult[] = Array.isArray(payload.items)
          ? payload.items.map((item: any) => ({
              // 统一 key，避免搜索结果缺少 id 时后续跳转、合并出现重复
              id: String(item.id || item.message_id || item.messageId || item._id || ''),
              contentSnippet: item.snippet || item.content_snippet || item.content || '',
              senderName: item.sender_name || item.user?.nick || item.user?.name || '未知成员',
              senderAvatar: item.user?.avatar,
              senderId: item.user_id || item.sender_id,
              icMode: item.ic_mode || item.icMode || 'ic',
              isArchived: !!(item.is_archived ?? item.archived),
              archivedAt: item.archived_at ?? item.archivedAt,
              createdAt: Number(item.created_at ?? item.createdAt ?? Date.now()),
              displayOrder: item.display_order ?? item.displayOrder,
              highlightRanges: item.highlight_ranges ?? item.highlightRanges,
            }))
          : []

        this.results = items
        this.total = Number(payload.total ?? items.length)
        this.lastKeyword = keyword
      } catch (error: any) {
        if (seq !== this.requestSeq) {
          return
        }
        const message = error?.response?.data?.error || error?.message || '搜索失败，请稍后重试'
        this.error = message
      } finally {
        if (seq === this.requestSeq) {
          this.loading = false
        }
      }
    },
  },
})
