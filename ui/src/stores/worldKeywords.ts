import { defineStore } from 'pinia'
import { api } from './_config'
import { chatEvent } from './chat'
import type { WorldKeyword, WorldKeywordEventPayload } from '@/types/world'

interface KeywordEntry {
  keyword: string
  description: string
  normalized: string
}

export interface CompiledKeywordSet {
  entries: KeywordEntry[]
  pattern: RegExp | null
  map: Map<string, KeywordEntry>
}

interface State {
  keywordMap: Record<string, WorldKeyword[]>
  compiledMap: Record<string, CompiledKeywordSet>
  loadingMap: Record<string, boolean>
  lastFetchedAt: Record<string, number>
  inflightMap: Record<string, Promise<WorldKeyword[] | void> | null>
}

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

export const useWorldKeywordStore = defineStore('worldKeywords', {
  state: (): State => ({
    keywordMap: {},
    compiledMap: {},
    loadingMap: {},
    lastFetchedAt: {},
    inflightMap: {},
  }),
  getters: {
    keywords: (state) => (worldId: string) => state.keywordMap[worldId] || [],
    matcher: (state) => (worldId: string) => state.compiledMap[worldId],
    keywordCount: (state) => (worldId: string) => state.keywordMap[worldId]?.length ?? 0,
  },
  actions: {
    async ensure(worldId: string, force = false) {
      if (!worldId) return
      if (!force && this.keywordMap[worldId] && this.keywordMap[worldId].length && !this.isExpired(worldId)) {
        return
      }
      if (this.inflightMap[worldId]) {
        await this.inflightMap[worldId]
        return
      }
      const request = this.fetchKeywords(worldId, force)
      this.inflightMap[worldId] = request
      try {
        await request
      } finally {
        this.inflightMap[worldId] = null
      }
    },
    async fetchKeywords(worldId: string, force = false) {
      if (!worldId) return
      this.loadingMap[worldId] = true
      try {
        const resp = await api.get<{ items: WorldKeyword[] }>(`/api/v1/worlds/${worldId}/keywords`, { params: force ? { ts: Date.now() } : undefined })
        const items = resp.data?.items || []
        this.keywordMap[worldId] = items
        this.lastFetchedAt[worldId] = Date.now()
        this.compiledMap[worldId] = compileKeywordSet(items)
        return items
      } catch (error) {
        console.warn('fetch keywords failed', error)
        throw error
      } finally {
        this.loadingMap[worldId] = false
      }
    },
    async createKeyword(worldId: string, payload: { keyword: string; description: string }) {
      if (!worldId) return null
      const resp = await api.post<{ item: WorldKeyword }>(`/api/v1/worlds/${worldId}/keywords`, payload)
      const item = resp.data?.item
      if (item) {
        this.upsertKeyword(worldId, item)
      }
      return item
    },
    async updateKeyword(worldId: string, keywordId: string, payload: { keyword?: string; description?: string }) {
      if (!worldId || !keywordId) return null
      const resp = await api.patch<{ item: WorldKeyword }>(`/api/v1/worlds/${worldId}/keywords/${keywordId}`, payload)
      const item = resp.data?.item
      if (item) {
        this.upsertKeyword(worldId, item)
      }
      return item
    },
    async deleteKeyword(worldId: string, keywordId: string) {
      if (!worldId || !keywordId) return
      await api.delete(`/api/v1/worlds/${worldId}/keywords/${keywordId}`)
      this.removeKeyword(worldId, keywordId)
    },
    async exportKeywords(worldId: string) {
      if (!worldId) return
      const resp = await api.get(`/api/v1/worlds/${worldId}/keywords/export`, { responseType: 'blob' })
      const blob = resp.data as Blob
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `world-keywords-${worldId}.json`
      link.click()
      URL.revokeObjectURL(url)
    },
    async importKeywords(worldId: string, content: string) {
      if (!worldId) return null
      const resp = await api.post<{ created: number; updated: number; skipped: number; total: number }>(
        `/api/v1/worlds/${worldId}/keywords/import`,
        { content },
      )
      await this.fetchKeywords(worldId, true)
      return resp.data
    },
    upsertKeyword(worldId: string, keyword: WorldKeyword) {
      const list = this.keywordMap[worldId] || []
      const idx = list.findIndex((item) => item.id === keyword.id)
      if (idx >= 0) {
        list.splice(idx, 1, keyword)
      } else {
        list.push(keyword)
      }
      this.keywordMap[worldId] = [...list].sort((a, b) => a.keyword.localeCompare(b.keyword))
      this.compiledMap[worldId] = compileKeywordSet(this.keywordMap[worldId])
      this.lastFetchedAt[worldId] = Date.now()
    },
    removeKeyword(worldId: string, keywordId: string) {
      const list = this.keywordMap[worldId] || []
      this.keywordMap[worldId] = list.filter((item) => item.id !== keywordId)
      this.compiledMap[worldId] = compileKeywordSet(this.keywordMap[worldId])
      this.lastFetchedAt[worldId] = Date.now()
    },
    applyRealtimeSnapshot(payload?: WorldKeywordEventPayload) {
      if (!payload?.worldId) return
      const keywords = payload.keywords?.map<WorldKeyword>((entry) => ({
        id: entry.id,
        worldId: payload.worldId,
        keyword: entry.keyword,
        description: entry.description,
        createdAt: new Date(entry.updatedAt).toISOString(),
        updatedAt: new Date(entry.updatedAt).toISOString(),
        createdBy: '',
        updatedBy: '',
        createdByName: '',
        updatedByName: '',
      })) || []
      this.keywordMap[payload.worldId] = keywords
      this.compiledMap[payload.worldId] = compileKeywordSet(keywords)
      this.lastFetchedAt[payload.worldId] = Date.now()
    },
    isExpired(worldId: string) {
      const fetchedAt = this.lastFetchedAt[worldId]
      if (!fetchedAt) return true
      const TTL = 60 * 1000
      return Date.now() - fetchedAt > TTL
    },
    matchText(worldId: string, text: string) {
      const compiled = this.compiledMap[worldId]
      if (!compiled || !compiled.pattern || !text) {
        return { matches: [] as KeywordMatch[], count: 0 }
      }
      const matches: KeywordMatch[] = []
      const regex = new RegExp(compiled.pattern.source, compiled.pattern.flags)
      let exec: RegExpExecArray | null
      while ((exec = regex.exec(text)) !== null) {
        const value = exec[0]
        const entry = compiled.map.get(value.toLowerCase())
        if (!entry) continue
        matches.push({
          start: exec.index,
          end: exec.index + value.length,
          keyword: entry.keyword,
          description: entry.description,
          text: value,
        })
      }
      return { matches, count: matches.length }
    },
  },
})

export interface KeywordMatch {
  start: number
  end: number
  keyword: string
  description: string
  text: string
}

const compileKeywordSet = (items: WorldKeyword[]): CompiledKeywordSet => {
  if (!items?.length) {
    return { entries: [], pattern: null, map: new Map() }
  }
  const entries: KeywordEntry[] = items.map((item) => ({
    keyword: item.keyword,
    description: item.description,
    normalized: item.keyword.toLowerCase(),
  }))
  entries.sort((a, b) => b.keyword.length - a.keyword.length)
  const patternSource = entries.map((item) => escapeRegExp(item.keyword)).join('|')
  const pattern = patternSource ? new RegExp(patternSource, 'gi') : null
  const map = new Map<string, KeywordEntry>()
  entries.forEach((entry) => {
    if (!map.has(entry.normalized)) {
      map.set(entry.normalized, entry)
    }
  })
  return { entries, pattern, map }
}

chatEvent.on('world-keywords-updated', (event) => {
  const payload = (event as any)?.worldKeywords as WorldKeywordEventPayload | undefined
  if (!payload) return
  const store = useWorldKeywordStore()
  store.applyRealtimeSnapshot(payload)
})
