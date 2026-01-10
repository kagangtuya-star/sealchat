import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { chatEvent, useChatStore } from './chat'
import { useUserStore } from './user'
import type { WorldKeywordItem, WorldKeywordPayload } from '@/models/worldGlossary'
import {
  fetchWorldKeywords,
  fetchWorldKeywordsPublic,
  createWorldKeyword,
  updateWorldKeyword,
  deleteWorldKeyword,
  bulkDeleteWorldKeywords,
  importWorldKeywords,
  exportWorldKeywords,
  fetchWorldKeywordCategories,
  fetchWorldKeywordCategoriesPublic,
} from '@/models/worldGlossary'
import { escapeRegExp } from '@/utils/tools'
import { clampTextWithImageTokens } from '@/utils/attachmentMarkdown'
import { useUtilsStore } from './utils'

const DEFAULT_KEYWORD_MAX_LENGTH = 2000

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
  category: string
  source: string
  regex: RegExp
  matchMode: 'plain' | 'regex'
  display: 'standard' | 'minimal' | 'inherit'
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

const getKeywordMaxLength = () => {
  try {
    const utils = useUtilsStore()
    return utils.config?.keywordMaxLength || DEFAULT_KEYWORD_MAX_LENGTH
  } catch {
    return DEFAULT_KEYWORD_MAX_LENGTH
  }
}

const clampText = (value?: string | null, maxLength?: number) => {
  const limit = maxLength ?? getKeywordMaxLength()
  return value ? value.slice(0, limit) : value || ''
}

const clampDescription = (value?: string | null, maxLength?: number) => {
  const limit = maxLength ?? getKeywordMaxLength()
  return value ? clampTextWithImageTokens(value, limit) : value || ''
}

const normalizeKeywordItem = (item: WorldKeywordItem): WorldKeywordItem => {
  const maxLen = getKeywordMaxLength()
  return {
    ...item,
    keyword: clampText(item.keyword, maxLen),
    aliases: (item.aliases || []).map((alias) => clampText(alias, maxLen)),
    description: item.description ? clampDescription(item.description, maxLen) : '',
  }
}

export const useWorldGlossaryStore = defineStore('worldGlossary', () => {
  const pages = ref<Record<string, KeywordPageState>>({})
  const loadingMap = ref<Record<string, boolean>>({})
  const compiledMap = ref<Record<string, CompiledKeywordSpan[]>>({})
  const keywordById = ref<Record<string, WorldKeywordItem>>({})
  const versionMap = ref<Record<string, number>>({})
  const managerVisible = ref(false)
  const editorState = ref<KeywordEditorState>({ visible: false, worldId: null, keyword: null, prefill: null })
  const quickPrefill = ref<string | null>(null)
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

  function setQuickPrefill(value: string | null) {
    quickPrefill.value = value
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
        const display = item.display || 'inherit'
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
                category: item.category || '',
                source: text,
                regex: pattern,
                matchMode: item.matchMode,
                display,
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
    const chat = useChatStore()
    const user = useUserStore()
    if (!chat.isObserver && !user.token) return
    const page = pages.value[worldId]
    if (!opts?.force && page && Date.now() - page.fetchedAt < 60 * 1000) {
      return
    }
    loadingMap.value = { ...loadingMap.value, [worldId]: true }
    try {
      const data = chat.isObserver
        ? await fetchWorldKeywordsPublic(worldId, { page: 1, pageSize: 5000 })
        : await fetchWorldKeywords(worldId, {
          page: 1,
          pageSize: 5000,
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
          category: current.category,
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
    const normalizedUpdates = updatedItems.map((item) => normalizeKeywordItem(item))
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

  async function exportKeywords(worldId: string, category?: string) {
    return exportWorldKeywords(worldId, category)
  }

  async function fetchCategories(worldId: string) {
    const chat = useChatStore()
    if (chat.isObserver) {
      return fetchWorldKeywordCategoriesPublic(worldId)
    }
    return fetchWorldKeywordCategories(worldId)
  }

  function handleGatewayEvent(event?: any) {
    if (!event || event.type !== 'world-keywords-updated') {
      return
    }
    const rawArgv = event?.argv || {}
    const options = (rawArgv.options || rawArgv.Options || {}) as Record<string, any>
    const worldId = options.worldId as string | undefined
    if (!worldId) {
      return
    }
    const revision = typeof options.revision === 'number' ? options.revision : typeof options.version === 'number' ? options.version : Date.now()
    const currentRevision = versionMap.value[worldId] || 0
    if (revision <= currentRevision) {
      return
    }
    versionMap.value = { ...versionMap.value, [worldId]: revision }
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
    quickPrefill,
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
    fetchCategories,
    setKeywordEnabledBulk,
    setManagerVisible,
    openEditor,
    setQuickPrefill,
    closeEditor,
    openImport,
    closeImport,
    setSearchQuery,
    rebuildCompiled,
  }
})
