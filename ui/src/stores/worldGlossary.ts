import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { chatEvent, useChatStore } from './chat'
import { useUserStore } from './user'
import type {
  EffectiveWorldKeywordItem,
  KeywordCategoryInfo,
  WorldKeywordItem,
  WorldKeywordPayload,
  WorldKeywordReorderItem,
} from '@/models/worldGlossary'
import {
  fetchEffectiveWorldKeywords,
  fetchEffectiveWorldKeywordsPublic,
  fetchWorldKeywords,
  fetchWorldKeywordsPublic,
  createWorldKeyword,
  bulkUpdateWorldKeywordCategoryPriority,
  fetchWorldKeywordCategoryInfos,
  updateWorldKeyword,
  deleteWorldKeyword,
  bulkDeleteWorldKeywords,
  reorderWorldKeywords,
  importWorldKeywords,
  exportWorldKeywords,
  fetchWorldKeywordCategories,
  fetchWorldKeywordCategoriesPublic,
  updateWorldKeywordCategoryPriority,
} from '@/models/worldGlossary'
import { escapeRegExp } from '@/utils/tools'
import { clampTextWithImageTokens } from '@/utils/attachmentMarkdown'
import {
  dedupeEffectiveKeywordsByKeyword,
  filterExactEffectiveKeywordCandidates,
} from '@/utils/worldKeywordConflictCandidates'
import { useUtilsStore } from './utils'

const DEFAULT_KEYWORD_MAX_LENGTH = 2000

interface KeywordPageState {
  items: WorldKeywordItem[]
  total: number
  page: number
  pageSize: number
  fetchedAt: number
}

interface EffectiveKeywordPageState {
  items: EffectiveWorldKeywordItem[]
  total: number
  fetchedAt: number
}

const DEFAULT_CATEGORY_PRIORITY = 0

export interface CompiledKeywordSpan {
  id: string
  keyword: string
  category: string
  source: string
  regex: RegExp
  matchMode: 'plain' | 'regex'
  display: 'standard' | 'minimal' | 'inherit'
  description: string
  descriptionFormat?: 'plain' | 'rich'
  categoryPriority?: number
  sourceType?: 'world' | 'external_library'
  sourceName?: string
  sourceSortOrder?: number
  sortOrder?: number
  updatedAt?: string
  canQuickEdit?: boolean
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
  const descriptionFormat = item.descriptionFormat === 'rich' ? 'rich' : 'plain'
  return {
    ...item,
    keyword: clampText(item.keyword, maxLen),
    aliases: (item.aliases || []).map((alias) => clampText(alias, maxLen)),
    description: item.description
      ? descriptionFormat === 'rich'
        ? item.description
        : clampDescription(item.description, maxLen)
      : '',
    descriptionFormat,
  }
}

const normalizeEffectiveKeywordItem = (item: EffectiveWorldKeywordItem): EffectiveWorldKeywordItem => {
  const normalized = normalizeKeywordItem(item)
  return {
    ...normalized,
    sourceType: item.sourceType,
    sourceId: item.sourceId,
    sourceName: item.sourceName,
    categoryPriority: item.categoryPriority ?? DEFAULT_CATEGORY_PRIORITY,
    sourceSortOrder: item.sourceSortOrder ?? 0,
    canQuickEdit: item.canQuickEdit,
  }
}

const compareEffectiveKeywordPriority = (left: EffectiveWorldKeywordItem, right: EffectiveWorldKeywordItem) => {
  const leftPriority = left.categoryPriority ?? DEFAULT_CATEGORY_PRIORITY
  const rightPriority = right.categoryPriority ?? DEFAULT_CATEGORY_PRIORITY
  if (leftPriority !== rightPriority) return rightPriority - leftPriority
  const leftTier = left.sourceType === 'world' ? 0 : 1
  const rightTier = right.sourceType === 'world' ? 0 : 1
  if (leftTier !== rightTier) return leftTier - rightTier
  const leftSourceSort = left.sourceSortOrder ?? 0
  const rightSourceSort = right.sourceSortOrder ?? 0
  if (leftSourceSort !== rightSourceSort) return rightSourceSort - leftSourceSort
  if ((left.sortOrder || 0) !== (right.sortOrder || 0)) return (right.sortOrder || 0) - (left.sortOrder || 0)
  if ((left.updatedAt || '') !== (right.updatedAt || '')) return (right.updatedAt || '').localeCompare(left.updatedAt || '')
  return (left.id || '').localeCompare(right.id || '')
}

export const useWorldGlossaryStore = defineStore('worldGlossary', () => {
  const pages = ref<Record<string, KeywordPageState>>({})
  const loadingMap = ref<Record<string, boolean>>({})
  const compiledMap = ref<Record<string, CompiledKeywordSpan[]>>({})
  const keywordById = ref<Record<string, WorldKeywordItem>>({})
  const categoryInfoMap = ref<Record<string, KeywordCategoryInfo[]>>({})
  const versionMap = ref<Record<string, number>>({})
  const effectivePages = ref<Record<string, EffectiveKeywordPageState>>({})
  const effectiveCompiledMap = ref<Record<string, CompiledKeywordSpan[]>>({})
  const effectiveKeywordById = ref<Record<string, EffectiveWorldKeywordItem>>({})
  const effectiveLoadingMap = ref<Record<string, boolean>>({})
  const effectiveVersionMap = ref<Record<string, number>>({})
  const managerVisible = ref(false)
  const editorState = ref<KeywordEditorState>({ visible: false, worldId: null, keyword: null, prefill: null })
  const quickPrefill = ref<string | null>(null)
  const importState = ref<KeywordImportState>({ visible: false, processing: false, worldId: null, lastStats: null })
  const searchQuery = ref('')
  const effectiveConflictCandidateCache = new Map<string, EffectiveWorldKeywordItem[]>()
  const effectiveConflictCandidatePending = new Map<string, Promise<EffectiveWorldKeywordItem[]>>()

  const currentWorldId = computed(() => useChatStore().currentWorldId)

  const currentKeywords = computed(() => {
    const worldId = currentWorldId.value
    if (!worldId) return []
    return dedupeEffectiveKeywordsByKeyword(effectivePages.value[worldId]?.items || [])
  })

  const currentCompiled = computed(() => {
    const worldId = currentWorldId.value
    if (!worldId) return []
    return effectiveCompiledMap.value[worldId] || []
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
                descriptionFormat: item.descriptionFormat,
                categoryPriority: DEFAULT_CATEGORY_PRIORITY,
                sourceType: 'world',
                sourceSortOrder: 0,
                sortOrder: item.sortOrder,
                updatedAt: item.updatedAt,
              })
            } catch (error) {
              console.warn('invalid keyword pattern', item.keyword, error)
            }
          })
      })
    compiledMap.value[worldId] = entries
  }

  function rebuildEffectiveCompiled(worldId: string) {
    const page = effectivePages.value[worldId]
    if (!page) {
      effectiveCompiledMap.value[worldId] = []
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
                descriptionFormat: item.descriptionFormat,
                categoryPriority: item.categoryPriority ?? DEFAULT_CATEGORY_PRIORITY,
                sourceType: item.sourceType,
                sourceName: item.sourceName,
                sourceSortOrder: item.sourceSortOrder ?? 0,
                sortOrder: item.sortOrder,
                updatedAt: item.updatedAt,
                canQuickEdit: item.canQuickEdit,
              })
            } catch (error) {
              console.warn('invalid effective keyword pattern', item.keyword, error)
            }
          })
      })
    effectiveCompiledMap.value[worldId] = entries
  }

  function updateKeywordCache(worldId: string, list: WorldKeywordItem[], meta?: { total?: number; page?: number; pageSize?: number }) {
    const normalizedList = list.map(normalizeKeywordItem)
    // Sort by sortOrder descending to ensure priority order
    normalizedList.sort((a, b) => (b.sortOrder || 0) - (a.sortOrder || 0))
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

  function updateEffectiveKeywordCache(worldId: string, list: EffectiveWorldKeywordItem[], meta?: { total?: number }) {
    const normalizedList = list.map(normalizeEffectiveKeywordItem)
    normalizedList.sort(compareEffectiveKeywordPriority)
    effectivePages.value = {
      ...effectivePages.value,
      [worldId]: {
        items: normalizedList,
        total: meta?.total ?? normalizedList.length,
        fetchedAt: Date.now(),
      },
    }
    const nextMap = { ...effectiveKeywordById.value }
    normalizedList.forEach((item) => {
      nextMap[item.id] = item
    })
    const keepIds = new Set(normalizedList.map((item) => item.id))
    Object.entries(nextMap).forEach(([id, item]) => {
      if (item.worldId === worldId && !keepIds.has(id)) {
        delete nextMap[id]
      }
    })
    effectiveKeywordById.value = nextMap
    rebuildEffectiveCompiled(worldId)
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

  async function ensureEffectiveKeywords(worldId: string, opts?: { force?: boolean; query?: string }) {
    if (!worldId) return
    const chat = useChatStore()
    const user = useUserStore()
    if (!chat.isObserver && !user.token) return
    const page = effectivePages.value[worldId]
    if (!opts?.force && page && Date.now() - page.fetchedAt < 60 * 1000) {
      return
    }
    effectiveLoadingMap.value = { ...effectiveLoadingMap.value, [worldId]: true }
    try {
      const data = chat.isObserver
        ? await fetchEffectiveWorldKeywordsPublic(worldId, { q: opts?.query })
        : await fetchEffectiveWorldKeywords(worldId, { q: opts?.query })
      updateEffectiveKeywordCache(worldId, data.items, data)
      Array.from(effectiveConflictCandidateCache.keys()).forEach((key) => {
        if (key.startsWith(`${worldId}::`)) {
          effectiveConflictCandidateCache.delete(key)
        }
      })
      Array.from(effectiveConflictCandidatePending.keys()).forEach((key) => {
        if (key.startsWith(`${worldId}::`)) {
          effectiveConflictCandidatePending.delete(key)
        }
      })
      effectiveVersionMap.value = { ...effectiveVersionMap.value, [worldId]: Date.now() }
    } finally {
      effectiveLoadingMap.value = { ...effectiveLoadingMap.value, [worldId]: false }
    }
  }

  async function ensureEffectiveKeywordConflictCandidates(worldId: string, matchedText: string) {
    const normalizedMatchedText = String(matchedText || '').trim().toLowerCase()
    if (!worldId || !normalizedMatchedText) {
      return []
    }
    const cacheKey = `${worldId}::${normalizedMatchedText}`
    const cached = effectiveConflictCandidateCache.get(cacheKey)
    if (cached) {
      return cached
    }
    const pending = effectiveConflictCandidatePending.get(cacheKey)
    if (pending) {
      return pending
    }

    const chat = useChatStore()
    const user = useUserStore()
    if (!chat.isObserver && !user.token) {
      return []
    }

    const request = (async () => {
      const data = chat.isObserver
        ? await fetchEffectiveWorldKeywordsPublic(worldId, { q: matchedText, includeAllMatches: true })
        : await fetchEffectiveWorldKeywords(worldId, { q: matchedText, includeAllMatches: true })
      const normalizedItems = data.items.map(normalizeEffectiveKeywordItem)
      const exactMatches = filterExactEffectiveKeywordCandidates(normalizedItems, matchedText)
      const nextMap = { ...effectiveKeywordById.value }
      exactMatches.forEach((item) => {
        nextMap[item.id] = item
      })
      effectiveKeywordById.value = nextMap
      effectiveConflictCandidateCache.set(cacheKey, exactMatches)
      return exactMatches
    })()

    effectiveConflictCandidatePending.set(cacheKey, request)
    try {
      return await request
    } finally {
      effectiveConflictCandidatePending.delete(cacheKey)
    }
  }

  async function createKeyword(worldId: string, payload: WorldKeywordPayload) {
    const item = await createWorldKeyword(worldId, payload)
    const list = [...(pages.value[worldId]?.items || [])]
    list.unshift(normalizeKeywordItem(item))
    updateKeywordCache(worldId, list)
    await ensureEffectiveKeywords(worldId, { force: true })
    return item
  }

  async function editKeyword(worldId: string, keywordId: string, payload: WorldKeywordPayload) {
    const item = await updateWorldKeyword(worldId, keywordId, payload)
    const list = (pages.value[worldId]?.items || []).map((existing) => (existing.id === keywordId ? normalizeKeywordItem(item) : existing))
    updateKeywordCache(worldId, list)
    await ensureEffectiveKeywords(worldId, { force: true })
    return item
  }

  async function removeKeyword(worldId: string, keywordId: string) {
    await deleteWorldKeyword(worldId, keywordId)
    const list = (pages.value[worldId]?.items || []).filter((item) => item.id !== keywordId)
    updateKeywordCache(worldId, list)
    await ensureEffectiveKeywords(worldId, { force: true })
  }

  async function removeKeywordBulk(worldId: string, ids: string[]) {
    const removed = await bulkDeleteWorldKeywords(worldId, ids)
    if (removed > 0) {
      const list = (pages.value[worldId]?.items || []).filter((item) => !ids.includes(item.id))
      updateKeywordCache(worldId, list)
      await ensureEffectiveKeywords(worldId, { force: true })
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
          descriptionFormat: current.descriptionFormat,
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
    await ensureEffectiveKeywords(worldId, { force: true })
  }

  async function setKeywordDisplayBulk(worldId: string, ids: string[], display: 'standard' | 'minimal' | 'inherit') {
    if (!worldId || !ids?.length) return
    const pageItems = pages.value[worldId]?.items || []
    const targetMap = new Map(pageItems.map((item) => [item.id, item]))
    const tasks = ids
      .map((id) => {
        const current = targetMap.get(id)
        const currentDisplay = current?.display || 'inherit'
        if (!current || currentDisplay === display) return null
        const payload: WorldKeywordPayload = {
          keyword: current.keyword,
          category: current.category,
          aliases: current.aliases,
          matchMode: current.matchMode,
          description: current.description,
          descriptionFormat: current.descriptionFormat,
          display,
          isEnabled: current.isEnabled,
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
    await ensureEffectiveKeywords(worldId, { force: true })
  }

  async function importKeywords(worldId: string, items: WorldKeywordPayload[], replace = false) {
    importState.value.processing = true
    const stats = await importWorldKeywords(worldId, { items, replace })
    importState.value.lastStats = stats
    importState.value.processing = false
    await ensureKeywords(worldId, { force: true })
    await ensureEffectiveKeywords(worldId, { force: true })
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

  async function ensureCategoryInfos(worldId: string, opts?: { force?: boolean }) {
    if (!worldId) return []
    if (!opts?.force && categoryInfoMap.value[worldId]?.length) {
      return categoryInfoMap.value[worldId]
    }
    const items = await fetchWorldKeywordCategoryInfos(worldId)
    categoryInfoMap.value = { ...categoryInfoMap.value, [worldId]: items }
    return items
  }

  async function setCategoryPriority(worldId: string, name: string, priority: number) {
    const item = await updateWorldKeywordCategoryPriority(worldId, name, priority)
    await ensureCategoryInfos(worldId, { force: true })
    await ensureEffectiveKeywords(worldId, { force: true })
    return item
  }

  async function setCategoryPriorities(worldId: string, items: Array<{ name: string; priority: number }>) {
    const updated = await bulkUpdateWorldKeywordCategoryPriority(worldId, items)
    await ensureCategoryInfos(worldId, { force: true })
    await ensureEffectiveKeywords(worldId, { force: true })
    return updated
  }

  async function reorderKeywords(worldId: string, items: WorldKeywordReorderItem[]) {
    const updated = await reorderWorldKeywords(worldId, items)
    if (updated > 0) {
      const pageItems = pages.value[worldId]?.items || []
      const orderMap = new Map(items.map((item) => [item.id, item.sortOrder]))
      const nextList = pageItems.map((item) => {
        const newOrder = orderMap.get(item.id)
        if (newOrder !== undefined) {
          return { ...item, sortOrder: newOrder }
        }
        return item
      })
      nextList.sort((a, b) => (b.sortOrder || 0) - (a.sortOrder || 0))
      updateKeywordCache(worldId, nextList)
      await ensureEffectiveKeywords(worldId, { force: true })
    }
    return updated
  }

  function handleGatewayEvent(event?: any) {
    if (!event || (event.type !== 'world-keywords-updated' && event.type !== 'world-external-glossaries-updated')) {
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
    if (event.type === 'world-keywords-updated') {
      void ensureKeywords(worldId, { force: true })
    }
    if (categoryInfoMap.value[worldId]) {
      void ensureCategoryInfos(worldId, { force: true })
    }
    effectiveVersionMap.value = { ...effectiveVersionMap.value, [worldId]: revision }
    void ensureEffectiveKeywords(worldId, { force: true })
  }

  function ensureGateway() {
    if (gatewayBound) return
    chatEvent.on('world-keywords-updated' as any, handleGatewayEvent)
    chatEvent.on('world-external-glossaries-updated' as any, handleGatewayEvent)
    gatewayBound = true
  }

  ensureGateway()

  return {
    pages,
    compiledMap,
    keywordById,
    categoryInfoMap,
    versionMap,
    effectivePages,
    effectiveCompiledMap,
    effectiveKeywordById,
    effectiveLoadingMap,
    effectiveVersionMap,
    managerVisible,
    editorState,
    quickPrefill,
    importState,
    searchQuery,
    currentKeywords,
    currentCompiled,
    loadingMap,
    ensureKeywords,
    ensureEffectiveKeywords,
    ensureEffectiveKeywordConflictCandidates,
    createKeyword,
    editKeyword,
    removeKeyword,
    removeKeywordBulk,
    importKeywords,
    exportKeywords,
    fetchCategories,
    ensureCategoryInfos,
    setCategoryPriority,
    setCategoryPriorities,
    reorderKeywords,
    setKeywordEnabledBulk,
    setKeywordDisplayBulk,
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
