import { defineStore } from 'pinia'
import { api } from './_config'
import { useChatStore } from './chat'
import type { SChannel } from '@/types'

export type ChannelSearchMatchMode = 'fuzzy' | 'exact'

export interface ChannelSearchFilters {
  speakerIds: string[]
  archived: 'all' | 'only' | 'exclude'
  icMode: 'all' | 'ic' | 'ooc'
  includeOutside: boolean
  timeRange: [number | null, number | null] | null
  worldScope: boolean
}

export interface ChannelSearchResult {
  id: string
  content?: string
  contentSnippet: string
  senderName: string
  senderAvatar?: string
  senderId?: string
  icMode: 'ic' | 'ooc'
  isArchived: boolean
  archivedAt?: number
  archivedBy?: string
  createdAt: number
  displayOrder?: number
  highlightRanges?: Array<[number, number]>
  keywordFragments?: { text: string; highlighted: boolean }[]
  channelId?: string
  channelName?: string
}

interface ChannelSearchStep {
  keyword: string
  matchMode: ChannelSearchMatchMode
}

interface ChannelSearchBaseSnapshot {
  steps: ChannelSearchStep[]
  filters: ChannelSearchFilters
  worldScope: boolean
  channelId: string | null
}

interface ChannelSearchState {
  panelVisible: boolean
  keyword: string
  lastKeyword: string
  withinResultsEnabled: boolean
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
  panelSize: { width: number; height: number }
  lastBaseSnapshot: ChannelSearchBaseSnapshot | null
}

const defaultFilters = (): ChannelSearchFilters => ({
  speakerIds: [],
  archived: 'all',
  icMode: 'all',
  includeOutside: true,
  timeRange: null,
  worldScope: false,
})

const flattenChannels = (channels?: SChannel[]): SChannel[] => {
  if (!Array.isArray(channels) || channels.length === 0) {
    return []
  }
  const stack = [...channels]
  const result: SChannel[] = []
  while (stack.length) {
    const node = stack.shift()
    if (!node) continue
    result.push(node)
    if (Array.isArray(node.children) && node.children.length) {
      stack.unshift(...(node.children as SChannel[]))
    }
  }
  return result
}

const renameRefineBaseParams = (params: Record<string, any>) => {
  const mappings: Array<[string, string]> = [
    ['speaker_ids', 'base_speaker_ids'],
    ['archived', 'base_archived'],
    ['ic_mode', 'base_ic_mode'],
    ['include_outside', 'base_include_outside'],
    ['time_start', 'base_time_start'],
    ['time_end', 'base_time_end'],
  ]
  mappings.forEach(([from, to]) => {
    if (from in params) {
      params[to] = params[from]
      delete params[from]
    }
  })
}

export const useChannelSearchStore = defineStore('channelSearch', {
  state: (): ChannelSearchState => ({
    panelVisible: false,
    keyword: '',
    lastKeyword: '',
    withinResultsEnabled: false,
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
    panelSize: {
      width: 420,
      height: 700,
    },
    lastBaseSnapshot: null,
  }),

  getters: {
    totalPages: (state) => {
      if (state.pageSize <= 0) {
        return 1
      }
      return Math.max(1, Math.ceil(state.total / state.pageSize))
    },
    hasKeyword: (state) => state.keyword.trim().length > 0,
    canSearchWithinResults: (state) => !!state.lastBaseSnapshot && state.lastKeyword.trim().length > 0,
    currentResultWorldScope: (state) => state.lastBaseSnapshot?.worldScope === true,
    isFilterActive: (state) => {
      const filters = state.filters
      return (
        filters.speakerIds.length > 0 ||
        filters.archived !== 'all' ||
        filters.icMode !== 'all' ||
        filters.includeOutside === false ||
        !!filters.timeRange ||
        filters.worldScope === true
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
    setWithinResultsEnabled(value: boolean) {
      this.withinResultsEnabled = value
      this.page = 1
      if (!value) {
        this.lastBaseSnapshot = null
        this.error = ''
      }
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
    clearBaseSnapshot() {
      this.lastBaseSnapshot = null
      this.lastKeyword = ''
      this.withinResultsEnabled = false
    },
    setPage(page: number) {
      this.page = Math.max(1, page)
    },
    setPanelPosition(position: { x: number; y: number }) {
      this.panelPosition = { ...position }
    },
    setPanelSize(size: { width: number; height: number }) {
      this.panelSize = { ...size }
    },
    bindChannel(channelId: string | null | undefined) {
      if (!channelId) {
        this.currentChannelId = null
        this.results = []
        this.total = 0
        this.clearBaseSnapshot()
        return
      }
      if (this.currentChannelId !== channelId) {
        this.currentChannelId = channelId
        this.results = []
        this.total = 0
        this.page = 1
        this.clearBaseSnapshot()
        this.error = ''
      }
    },
    buildSearchParams(keyword: string, filters: ChannelSearchFilters, matchMode: ChannelSearchMatchMode, page?: number) {
      const pageSize = Math.max(1, this.pageSize)
      const params: Record<string, any> = {
        keyword,
        match_mode: matchMode,
        page_size: pageSize,
      }

      if (typeof page === 'number') {
        params.page = page
      }
      if (filters.speakerIds.length) {
        params.speaker_ids = filters.speakerIds
      }
      if (filters.archived !== 'all') {
        params.archived = filters.archived
      }
      if (filters.icMode !== 'all') {
        params.ic_mode = filters.icMode
      }
      if (filters.includeOutside === false) {
        params.include_outside = false
      }
      if (filters.timeRange) {
        params.time_start = filters.timeRange[0]
        params.time_end = filters.timeRange[1]
      }
      return params
    },
    async search(channelId?: string) {
      if (this.withinResultsEnabled && this.lastBaseSnapshot) {
        return this.searchWithinResults(channelId)
      }
      return this.searchPrimary(channelId)
    },
    async searchPrimary(channelId?: string) {
      const useWorldScope = this.filters.worldScope === true
      const activeChannel = useWorldScope ? null : channelId ?? this.currentChannelId
      if (!useWorldScope && !activeChannel) {
        this.error = '请选择频道后再搜索'
        return
      }
      const keyword = this.keyword.trim()
      if (!keyword) {
        this.results = []
        this.total = 0
        this.error = ''
        this.lastKeyword = ''
        this.lastBaseSnapshot = null
        return
      }

      const seq = ++this.requestSeq
      this.loading = true
      this.error = ''
      const baseParams = this.buildSearchParams(keyword, this.filters, this.matchMode)

      try {
        if (useWorldScope) {
          const worldResult = await this.searchWorldChannels(baseParams, seq)
          if (!worldResult || seq !== this.requestSeq) {
            return
          }
          this.results = worldResult.items
          this.total = worldResult.total
        } else if (activeChannel) {
          const params = {
            ...baseParams,
            page: this.page,
          }
          const { items, total } = await this.fetchChannelSearch(activeChannel, params)
          if (seq !== this.requestSeq) {
            return
          }
          this.results = items
          this.total = total
        }
        this.lastKeyword = keyword
        this.lastBaseSnapshot = {
          steps: [{ keyword, matchMode: this.matchMode }],
          filters: {
            ...this.filters,
            speakerIds: [...this.filters.speakerIds],
            timeRange: this.filters.timeRange ? [...this.filters.timeRange] as [number | null, number | null] : null,
          },
          worldScope: useWorldScope,
          channelId: activeChannel || this.currentChannelId,
        }
      } catch (error: any) {
        if (seq !== this.requestSeq) {
          return
        }
        const message = error?.response?.data?.error || error?.response?.data?.message || error?.message || '搜索失败，请稍后重试'
        this.error = message
      } finally {
        if (seq === this.requestSeq) {
          this.loading = false
        }
      }
    },
    async searchWithinResults(channelId?: string) {
      const snapshot = this.lastBaseSnapshot
      if (!snapshot || !this.lastKeyword.trim()) {
        this.error = '请先执行一次主搜索'
        return
      }
      const nextKeyword = this.keyword.trim()
      if (!nextKeyword) {
        this.results = []
        this.total = 0
        this.error = ''
        return
      }

      const useWorldScope = snapshot.worldScope === true
      const activeChannel = useWorldScope ? null : (snapshot.channelId || channelId || this.currentChannelId)
      if (!useWorldScope && !activeChannel) {
        this.error = '请选择频道后再搜索'
        return
      }

      const seq = ++this.requestSeq
      this.loading = true
      this.error = ''

      const baseFilters = {
        ...snapshot.filters,
        worldScope: snapshot.worldScope,
      }
      const baseParams = this.buildSearchParams('', baseFilters, this.matchMode)
      delete baseParams.keyword
      delete baseParams.match_mode
      const refineParams: Record<string, any> = {
        ...baseParams,
        page: this.page,
        base_keywords: snapshot.steps.map((step) => step.keyword),
        base_match_modes: snapshot.steps.map((step) => step.matchMode),
        keyword: nextKeyword,
        match_mode: this.matchMode,
      }
      renameRefineBaseParams(refineParams)

      try {
        if (useWorldScope) {
          const worldResult = await this.searchWorldChannels(refineParams, seq, true)
          if (!worldResult || seq !== this.requestSeq) {
            return
          }
          this.results = worldResult.items
          this.total = worldResult.total
        } else if (activeChannel) {
          const { items, total } = await this.fetchChannelSearchRefine(activeChannel, refineParams)
          if (seq !== this.requestSeq) {
            return
          }
          this.results = items
          this.total = total
        }
        this.lastKeyword = nextKeyword
        this.lastBaseSnapshot = {
          ...snapshot,
          steps: [...snapshot.steps, { keyword: nextKeyword, matchMode: this.matchMode }],
        }
      } catch (error: any) {
        if (seq !== this.requestSeq) {
          return
        }
        const message = error?.response?.data?.error || error?.response?.data?.message || error?.message || '搜索失败，请稍后重试'
        this.error = message
      } finally {
        if (seq === this.requestSeq) {
          this.loading = false
        }
      }
    },
    async fetchChannelSearch(channelId: string, params: Record<string, any>, channelNameHint?: string) {
      const chatStore = useChatStore()
      const requestParams: Record<string, any> = { ...params }
      let endpoint = `api/v1/channels/${channelId}/messages/search`
      if (chatStore.observerMode) {
        const observerSlug = (chatStore.observerSlug || '').trim()
        if (observerSlug) {
          requestParams.ob_slug = observerSlug
          endpoint = `api/v1/public/ob/channels/${channelId}/messages/search`
        }
      }
      const resp = await api.get(endpoint, {
        params: requestParams,
      })
      const payload = resp?.data ?? {}
      const resolvedChannelName =
        channelNameHint ||
        chatStore.findChannelById(channelId)?.name ||
        chatStore.curChannel?.name ||
        '未知频道'
      const items: ChannelSearchResult[] = Array.isArray(payload.items)
        ? payload.items.map((item: any) => ({
            id: String(item.id || item.message_id || item.messageId || item._id || ''),
            content: item.content || item.content_raw || '',
            contentSnippet: item.snippet || item.content_snippet || item.content || '',
            senderName: item.sender_name || item.user?.nick || item.user?.name || '未知成员',
            senderAvatar: item.user?.avatar,
            senderId: item.user_id || item.sender_id || item.user?.id,
            icMode: item.ic_mode || item.icMode || 'ic',
            isArchived: !!(item.is_archived ?? item.archived),
            archivedAt: item.archived_at ?? item.archivedAt,
            archivedBy: item.archived_by ?? item.archivedBy,
            createdAt: Number(item.created_at ?? item.createdAt ?? Date.now()),
            displayOrder: item.display_order ?? item.displayOrder,
            highlightRanges: item.highlight_ranges ?? item.highlightRanges,
            channelId,
            channelName: resolvedChannelName,
          }))
        : []
      return {
        items,
        total: Number(payload.total ?? items.length),
      }
    },
    async fetchChannelSearchRefine(channelId: string, params: Record<string, any>, channelNameHint?: string) {
      const chatStore = useChatStore()
      const requestParams: Record<string, any> = { ...params }
      let endpoint = `api/v1/channels/${channelId}/messages/search/refine`
      if (chatStore.observerMode) {
        const observerSlug = (chatStore.observerSlug || '').trim()
        if (observerSlug) {
          requestParams.ob_slug = observerSlug
          endpoint = `api/v1/public/ob/channels/${channelId}/messages/search/refine`
        }
      }
      const resp = await api.get(endpoint, {
        params: requestParams,
      })
      const payload = resp?.data ?? {}
      const resolvedChannelName =
        channelNameHint ||
        chatStore.findChannelById(channelId)?.name ||
        chatStore.curChannel?.name ||
        '未知频道'
      const items: ChannelSearchResult[] = Array.isArray(payload.items)
        ? payload.items.map((item: any) => ({
            id: String(item.id || item.message_id || item.messageId || item._id || ''),
            content: item.content || item.content_raw || '',
            contentSnippet: item.snippet || item.content_snippet || item.content || '',
            senderName: item.sender_name || item.user?.nick || item.user?.name || '未知成员',
            senderAvatar: item.user?.avatar,
            senderId: item.user_id || item.sender_id || item.user?.id,
            icMode: item.ic_mode || item.icMode || 'ic',
            isArchived: !!(item.is_archived ?? item.archived),
            archivedAt: item.archived_at ?? item.archivedAt,
            archivedBy: item.archived_by ?? item.archivedBy,
            createdAt: Number(item.created_at ?? item.createdAt ?? Date.now()),
            displayOrder: item.display_order ?? item.displayOrder,
            highlightRanges: item.highlight_ranges ?? item.highlightRanges,
            channelId,
            channelName: resolvedChannelName,
          }))
        : []
      return {
        items,
        total: Number(payload.total ?? items.length),
      }
    },
    async searchWorldChannels(baseParams: Record<string, any>, seq: number, refine = false) {
      const chatStore = useChatStore()
      const worldId = chatStore.currentWorldId
      const tree =
        (worldId && chatStore.channelTreeByWorld?.[worldId]) || chatStore.channelTree || []
      const targets = flattenChannels(tree)
        .filter((channel) => channel?.id)
        .map((channel) => ({
          id: channel.id,
          name: channel.name || '未命名频道',
        }))
      if (!targets.length) {
        this.error = worldId ? '当前世界暂无可搜索的频道' : '请先选择世界'
        this.results = []
        this.total = 0
        return { items: [], total: 0 }
      }
      const pageSize = Math.max(1, this.pageSize)
      const currentPage = Math.max(1, this.page)
      const globalStart = (currentPage - 1) * pageSize
      const globalEnd = globalStart + pageSize
      const aggregated: ChannelSearchResult[] = []
      let aggregatedTotal = 0
      let consumed = 0

      for (const target of targets) {
        if (seq !== this.requestSeq) {
          return null
        }
        try {
          const cache = new Map<number, ChannelSearchResult[]>()
          const firstParams = { ...baseParams, page: 1 }
          const fetcher = refine ? this.fetchChannelSearchRefine : this.fetchChannelSearch
          const { items: firstPageItems, total: channelTotal } = await fetcher(
            target.id,
            firstParams,
            target.name,
          )
          aggregatedTotal += channelTotal
          const channelStart = consumed
          const channelEnd = consumed + channelTotal
          consumed = channelEnd

          if (channelTotal > 0 && channelEnd > globalStart && channelStart < globalEnd) {
            const rangeStart = Math.max(0, globalStart - channelStart)
            const rangeEnd = Math.min(channelTotal, globalEnd - channelStart)
            if (rangeEnd > rangeStart) {
              cache.set(1, firstPageItems)
              const startPage = Math.floor(rangeStart / pageSize) + 1
              const endPage = Math.floor((rangeEnd - 1) / pageSize) + 1
              for (let pageNo = startPage; pageNo <= endPage; pageNo++) {
                if (seq !== this.requestSeq) {
                  return null
                }
                let pageItems = cache.get(pageNo)
                if (!pageItems) {
                  const { items } = await fetcher(
                    target.id,
                    { ...baseParams, page: pageNo },
                    target.name,
                  )
                  pageItems = items
                  cache.set(pageNo, pageItems)
                }
                const pageBaseIndex = (pageNo - 1) * pageSize
                const sliceStart = Math.max(0, rangeStart - pageBaseIndex)
                const sliceEnd = Math.min(pageItems.length, rangeEnd - pageBaseIndex)
                if (sliceStart < sliceEnd) {
                  aggregated.push(...pageItems.slice(sliceStart, sliceEnd))
                }
                if (aggregated.length >= pageSize) {
                  break
                }
              }
            }
          }
        } catch (error) {
          console.warn('世界搜索子频道失败', target.id, error)
        }
      }

      return { items: aggregated, total: aggregatedTotal }
    },
  },
})
