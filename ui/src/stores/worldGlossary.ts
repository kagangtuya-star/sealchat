import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { chatEvent, useChatStore } from './chat'
import type { WorldKeywordItem, WorldKeywordPayload } from '@/models/worldGlossary'
import {
  fetchWorldKeywords,
  createWorldKeyword,
  updateWorldKeyword,
  deleteWorldKeyword,
  bulkDeleteWorldKeywords,
  importWorldKeywords,
  exportWorldKeywords,
} from '@/models/worldGlossary'
import { escapeRegExp } from '@/utils/tools'

const KEYWORD_MAX_LENGTH = 500

interface KeywordPageState {
  items: WorldKeywordItem[]
  total: number
  page: number
  pageSize: number
  fetchedAt: number
}

export interface CompiledKeywordSpan {
  id: string
  keyword: string
  source: string
  regex: RegExp
  matchMode: 'plain' | 'regex'
  display: 'standard' | 'minimal'
  description: string
}

interface ImportStats {
  created: number
  updated: number
  skipped: number
}

interface KeywordEditorState {
  visible: boolean
  worldId: string | null
  keyword?: WorldKeywordItem | null
  prefill?: string | null
}

interface KeywordImportState {
  visible: boolean
  processing: boolean
  worldId: string | null
  lastStats: ImportStats | null
}

let gatewayBound = false

const clampText = (value?: string | null) => (value ? value.slice(0, KEYWORD_MAX_LENGTH) : value || '')

const normalizeKeywordItem = (item: WorldKeywordItem): WorldKeywordItem => ({
  ...item,
  keyword: clampText(item.keyword),
  aliases: (item.aliases || []).map((alias) => clampText(alias)),
  description: item.description ? clampText(item.description) : '',
})

export const useWorldGlossaryStore = defineStore('worldGlossary', () => {
  const pages = ref<Record<string, KeywordPageState>>({})
  const loadingMap = ref<Record<string, boolean>>({})
  const compiledMap = ref<Record<string, CompiledKeywordSpan[]>>({})
  const keywordById = ref<Record<string, WorldKeywordItem>>({})
  const versionMap = ref<Record<string, number>>({})
  const managerVisible = ref(false)
  const editorState = ref<KeywordEditorState>({ visible: false, worldId: null, keyword: null, prefill: null })
  const importState = ref<KeywordImportState>({ visible: false, processing: false, worldId: null, lastStats: null })
  const searchQuery = ref('')

  const currentWorldId = computed(() => useChatStore().currentWorldId)

  const currentKeywords = computed(() => {
    const worldId = currentWorldId.value
    if (!worldId) return []
    return pages.value[worldId]?.items || []
  })

  const currentCompiled = computed(() => {
    const worldId = currentWorldId.value
    if (!worldId) return []
    return compiledMap.value[worldId] || []
  })

  function setManagerVisible(visible: boolean) {
    managerVisible.value = visible
  }

  function openEditor(worldId: string, keyword?: WorldKeywordItem | null, prefill?: string | null) {
    editorState.value = { visible: true, worldId, keyword: keyword || null, prefill: prefill || null }
  }

  function closeEditor() {
    editorState.value = { visible: false, worldId: null, keyword: null, prefill: null }
  }

  function openImport(worldId: string) {
    if (!worldId) return
    importState.value.visible = true
    importState.value.worldId = worldId
    importState.value.lastStats = null
  }

  function closeImport() {
    importState.value.visible = false
    importState.value.worldId = null
    importState.value.lastStats = null
    importState.value.processing = false
  }

  function setSearchQuery(value: string) {
    searchQuery.value = value
  }

  function rebuildCompiled(worldId: string) {
    const page = pages.value[worldId]
    if (!page) {
      compiledMap.value[worldId] = []
      return
    }
    const entries: CompiledKeywordSpan[] = []
    page.items
      .filter((item) => item && item.isEnabled)
      .forEach((item) => {
        const baseSources = [item.keyword, ...(item.aliases || [])]
        baseSources
          .map((text) => text?.trim())
          .filter((text): text is string => Boolean(text))
          .forEach((text) => {
            try {
              const pattern =
                item.matchMode === 'regex'
                  ? new RegExp(text, 'g')
                  : new RegExp(escapeRegExp(text), 'gi')
              entries.push({
                id: item.id,
                keyword: item.keyword,
                source: text,
                regex: pattern,
                matchMode: item.matchMode,
                display: item.display,
                description: item.description,
              })
            } catch (error) {
              console.warn('invalid keyword pattern', item.keyword, error)
            }
          })
      })
    compiledMap.value[worldId] = entries
  }

  function updateKeywordCache(worldId: string, list: WorldKeywordItem[], meta?: { total?: number; page?: number; pageSize?: number }) {
    const normalizedList = list.map(normalizeKeywordItem)
    const total = meta?.total ?? list.length
    const page = meta?.page ?? 1
    const pageSize = meta?.pageSize ?? list.length
    pages.value = {
      ...pages.value,
      [worldId]: {
        items: normalizedList,
        total,
        page,
        pageSize,
        fetchedAt: Date.now(),
      },
    }
    const nextMap = { ...keywordById.value }
    normalizedList.forEach((item) => {
      nextMap[item.id] = item
    })
    const keepIds = new Set(normalizedList.map((item) => item.id))
    Object.entries(nextMap).forEach(([id, item]) => {
      if (item.worldId === worldId && !keepIds.has(id)) {
        delete nextMap[id]
      }
    })
    keywordById.value = nextMap
    rebuildCompiled(worldId)
  }

  async function ensureKeywords(worldId: string, opts?: { force?: boolean; query?: string }) {
    if (!worldId) return
    const page = pages.value[worldId]
    if (!opts?.force && page && Date.now() - page.fetchedAt < 30 * 1000) {
      return
    }
    loadingMap.value = { ...loadingMap.value, [worldId]: true }
    try {
      const data = await fetchWorldKeywords(worldId, {
        page: 1,
        pageSize: 500,
        includeDisabled: true,
      })
      updateKeywordCache(worldId, data.items, data)
      versionMap.value = { ...versionMap.value, [worldId]: Date.now() }
    } finally {
      loadingMap.value = { ...loadingMap.value, [worldId]: false }
    }
  }

  async function createKeyword(worldId: string, payload: WorldKeywordPayload) {
    const item = await createWorldKeyword(worldId, payload)
      const list = [...(pages.value[worldId]?.items || [])]
      list.unshift(normalizeKeywordItem(item))
      updateKeywordCache(worldId, list)
      return item
    }

  async function editKeyword(worldId: string, keywordId: string, payload: WorldKeywordPayload) {
    const item = await updateWorldKeyword(worldId, keywordId, payload)
    const list = (pages.value[worldId]?.items || []).map((existing) => (existing.id === keywordId ? normalizeKeywordItem(item) : existing))
    updateKeywordCache(worldId, list)
    return item
  }

  async function removeKeyword(worldId: string, keywordId: string) {
    await deleteWorldKeyword(worldId, keywordId)
    const list = (pages.value[worldId]?.items || []).filter((item) => item.id !== keywordId)
    updateKeywordCache(worldId, list)
  }

  async function removeKeywordBulk(worldId: string, ids: string[]) {
    const removed = await bulkDeleteWorldKeywords(worldId, ids)
    if (removed > 0) {
      const list = (pages.value[worldId]?.items || []).filter((item) => !ids.includes(item.id))
      updateKeywordCache(worldId, list)
    }
  }

  async function setKeywordEnabledBulk(worldId: string, ids: string[], enabled: boolean) {
    if (!worldId || !ids?.length) return
    const pageItems = pages.value[worldId]?.items || []
    const targetMap = new Map(pageItems.map((item) => [item.id, item]))
    const tasks = ids
      .map((id) => {
        const current = targetMap.get(id)
        if (!current || current.isEnabled === enabled) return null
        const payload: WorldKeywordPayload = {
          keyword: current.keyword,
          aliases: current.aliases,
          matchMode: current.matchMode,
          description: current.description,
          display: current.display,
          isEnabled: enabled,
        }
        return updateWorldKeyword(worldId, id, payload)
      })
      .filter((task): task is Promise<WorldKeywordItem> => Boolean(task))
    if (!tasks.length) {
      return
    }
    const updatedItems = await Promise.all(tasks)
    const normalizedUpdates = updatedItems.map(normalizeKeywordItem)
    const updatedMap = new Map(normalizedUpdates.map((item) => [item.id, item]))
    const nextList = pageItems.map((item) => updatedMap.get(item.id) || item)
    updateKeywordCache(worldId, nextList)
  }

  async function importKeywords(worldId: string, items: WorldKeywordPayload[], replace = false) {
    importState.value.processing = true
    const stats = await importWorldKeywords(worldId, { items, replace })
    importState.value.lastStats = stats
    importState.value.processing = false
    await ensureKeywords(worldId, { force: true })
    return stats
  }

  async function exportKeywords(worldId: string) {
    return exportWorldKeywords(worldId)
  }

  function handleGatewayEvent(event?: any) {
    if (!event || event.type !== 'world-keywords-updated') {
      return
    }
    const options = event?.argv?.options || {}
    const worldId = options.worldId as string | undefined
    if (!worldId) {
      return
    }
    const currentVersion = versionMap.value[worldId] || 0
    const nextVersion = typeof options.version === 'number' ? options.version : Date.now()
    if (nextVersion <= currentVersion) {
      return
    }
    versionMap.value = { ...versionMap.value, [worldId]: nextVersion }
    void ensureKeywords(worldId, { force: true })
  }

  function ensureGateway() {
    if (gatewayBound) return
    chatEvent.on('world-keywords-updated' as any, handleGatewayEvent)
    gatewayBound = true
  }

  ensureGateway()

  return {
    pages,
    compiledMap,
    keywordById,
    versionMap,
    managerVisible,
    editorState,
    importState,
    searchQuery,
    currentKeywords,
    currentCompiled,
    loadingMap,
    ensureKeywords,
    createKeyword,
    editKeyword,
    removeKeyword,
    removeKeywordBulk,
    importKeywords,
    exportKeywords,
    setKeywordEnabledBulk,
    setManagerVisible,
    openEditor,
    closeEditor,
    openImport,
    closeImport,
    setSearchQuery,
    rebuildCompiled,
  }
})
