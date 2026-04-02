import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { chatEvent } from './chat'
import type {
  ExternalGlossaryLibraryItem,
  ExternalGlossaryLibraryPayload,
  ExternalGlossaryTermItem,
} from '@/models/externalGlossary'
import {
  bulkDeleteExternalGlossaryLibraries,
  bulkDeleteExternalGlossaryTerms,
  createExternalGlossaryCategory,
  createExternalGlossaryLibrary,
  createExternalGlossaryTerm,
  deleteExternalGlossaryCategory,
  deleteExternalGlossaryLibrary,
  deleteExternalGlossaryTerm,
  exportExternalGlossaryLibrary,
  exportExternalGlossaryTerms,
  fetchExternalGlossaryCategories,
  fetchExternalGlossaryLibraries,
  fetchExternalGlossaryTerms,
  importExternalGlossaryLibrary,
  importExternalGlossaryTerms,
  renameExternalGlossaryCategory,
  reorderExternalGlossaryLibraries,
  reorderExternalGlossaryTerms,
  updateExternalGlossaryLibrary,
  updateExternalGlossaryTerm,
} from '@/models/externalGlossary'
import type { WorldKeywordPayload, WorldKeywordReorderItem } from '@/models/worldGlossary'

interface LibraryPageState {
  items: ExternalGlossaryLibraryItem[]
  total: number
  page: number
  pageSize: number
  fetchedAt: number
}

interface TermPageState {
  items: ExternalGlossaryTermItem[]
  total: number
  page: number
  pageSize: number
  fetchedAt: number
}

let gatewayBound = false

export const useExternalGlossaryStore = defineStore('externalGlossary', () => {
  const libraryPage = ref<LibraryPageState | null>(null)
  const termPages = ref<Record<string, TermPageState>>({})
  const categoriesMap = ref<Record<string, string[]>>({})
  const libraryVersion = ref(0)
  const libraryLoading = ref(false)
  const termLoadingMap = ref<Record<string, boolean>>({})
  const activeLibraryId = ref('')

  const currentTerms = computed(() => {
    const libraryId = activeLibraryId.value
    return libraryId ? termPages.value[libraryId]?.items || [] : []
  })

  function setActiveLibrary(libraryId: string) {
    activeLibraryId.value = libraryId || ''
  }

  function updateLibraryPage(items: ExternalGlossaryLibraryItem[], meta?: { total?: number; page?: number; pageSize?: number }) {
    const normalized = [...items].sort((a, b) => (b.sortOrder || 0) - (a.sortOrder || 0))
    libraryPage.value = {
      items: normalized,
      total: meta?.total ?? normalized.length,
      page: meta?.page ?? 1,
      pageSize: meta?.pageSize ?? Math.max(normalized.length, 1),
      fetchedAt: Date.now(),
    }
    if (!activeLibraryId.value && normalized.length) {
      activeLibraryId.value = normalized[0].id
    } else if (activeLibraryId.value && !normalized.some((item) => item.id === activeLibraryId.value)) {
      activeLibraryId.value = normalized[0]?.id || ''
    }
  }

  function updateTermPage(libraryId: string, items: ExternalGlossaryTermItem[], meta?: { total?: number; page?: number; pageSize?: number }) {
    const normalized = [...items].sort((a, b) => (b.sortOrder || 0) - (a.sortOrder || 0))
    termPages.value = {
      ...termPages.value,
      [libraryId]: {
        items: normalized,
        total: meta?.total ?? normalized.length,
        page: meta?.page ?? 1,
        pageSize: meta?.pageSize ?? Math.max(normalized.length, 1),
        fetchedAt: Date.now(),
      },
    }
  }

  async function ensureLibraries(opts?: { force?: boolean; query?: string }) {
    const page = libraryPage.value
    if (!opts?.force && page && Date.now() - page.fetchedAt < 60 * 1000 && !opts?.query) {
      return
    }
    libraryLoading.value = true
    try {
      const data = await fetchExternalGlossaryLibraries({
        page: 1,
        pageSize: 5000,
        includeDisabled: true,
        q: opts?.query,
      })
      updateLibraryPage(data.items, data)
      libraryVersion.value = Date.now()
    } finally {
      libraryLoading.value = false
    }
  }

  async function ensureTerms(libraryId: string, opts?: { force?: boolean; query?: string; category?: string }) {
    if (!libraryId) return
    const page = termPages.value[libraryId]
    if (!opts?.force && page && Date.now() - page.fetchedAt < 60 * 1000 && !opts?.query && !opts?.category) {
      return
    }
    termLoadingMap.value = { ...termLoadingMap.value, [libraryId]: true }
    try {
      const data = await fetchExternalGlossaryTerms(libraryId, {
        page: 1,
        pageSize: 5000,
        includeDisabled: true,
        q: opts?.query,
        category: opts?.category,
      })
      updateTermPage(libraryId, data.items, data)
    } finally {
      termLoadingMap.value = { ...termLoadingMap.value, [libraryId]: false }
    }
  }

  async function ensureCategories(libraryId: string, opts?: { force?: boolean }) {
    if (!libraryId) return []
    if (!opts?.force && categoriesMap.value[libraryId]?.length) {
      return categoriesMap.value[libraryId]
    }
    const categories = await fetchExternalGlossaryCategories(libraryId)
    categoriesMap.value = { ...categoriesMap.value, [libraryId]: categories }
    return categories
  }

  async function createLibrary(payload: ExternalGlossaryLibraryPayload) {
    const item = await createExternalGlossaryLibrary(payload)
    updateLibraryPage([item, ...(libraryPage.value?.items || [])])
    return item
  }

  async function editLibrary(libraryId: string, payload: ExternalGlossaryLibraryPayload) {
    const item = await updateExternalGlossaryLibrary(libraryId, payload)
    updateLibraryPage((libraryPage.value?.items || []).map((existing) => (existing.id === libraryId ? { ...existing, ...item } : existing)))
    return item
  }

  async function removeLibrary(libraryId: string) {
    await deleteExternalGlossaryLibrary(libraryId)
    updateLibraryPage((libraryPage.value?.items || []).filter((item) => item.id !== libraryId))
  }

  async function removeLibraries(ids: string[]) {
    const deleted = await bulkDeleteExternalGlossaryLibraries(ids)
    if (deleted > 0) {
      updateLibraryPage((libraryPage.value?.items || []).filter((item) => !ids.includes(item.id)))
    }
    return deleted
  }

  async function sortLibraries(items: WorldKeywordReorderItem[]) {
    const updated = await reorderExternalGlossaryLibraries(items)
    if (updated > 0) {
      const orderMap = new Map(items.map((item) => [item.id, item.sortOrder]))
      updateLibraryPage((libraryPage.value?.items || []).map((item) => {
        const sortOrder = orderMap.get(item.id)
        return sortOrder === undefined ? item : { ...item, sortOrder }
      }))
    }
    return updated
  }

  async function createTerm(libraryId: string, payload: WorldKeywordPayload) {
    const item = await createExternalGlossaryTerm(libraryId, payload)
    updateTermPage(libraryId, [item, ...(termPages.value[libraryId]?.items || [])])
    await ensureCategories(libraryId, { force: true })
    await ensureLibraries({ force: true })
    return item
  }

  async function editTerm(libraryId: string, termId: string, payload: WorldKeywordPayload) {
    const item = await updateExternalGlossaryTerm(libraryId, termId, payload)
    updateTermPage(libraryId, (termPages.value[libraryId]?.items || []).map((existing) => (existing.id === termId ? item : existing)))
    await ensureCategories(libraryId, { force: true })
    return item
  }

  async function removeTerm(libraryId: string, termId: string) {
    await deleteExternalGlossaryTerm(libraryId, termId)
    updateTermPage(libraryId, (termPages.value[libraryId]?.items || []).filter((item) => item.id !== termId))
    await ensureCategories(libraryId, { force: true })
    await ensureLibraries({ force: true })
  }

  async function removeTerms(libraryId: string, ids: string[]) {
    const deleted = await bulkDeleteExternalGlossaryTerms(libraryId, ids)
    if (deleted > 0) {
      updateTermPage(libraryId, (termPages.value[libraryId]?.items || []).filter((item) => !ids.includes(item.id)))
      await ensureCategories(libraryId, { force: true })
      await ensureLibraries({ force: true })
    }
    return deleted
  }

  async function sortTerms(libraryId: string, items: WorldKeywordReorderItem[]) {
    const updated = await reorderExternalGlossaryTerms(libraryId, items)
    if (updated > 0) {
      const orderMap = new Map(items.map((item) => [item.id, item.sortOrder]))
      updateTermPage(libraryId, (termPages.value[libraryId]?.items || []).map((item) => {
        const sortOrder = orderMap.get(item.id)
        return sortOrder === undefined ? item : { ...item, sortOrder }
      }))
    }
    return updated
  }

  async function setTermsEnabled(libraryId: string, ids: string[], enabled: boolean) {
    const pageItems = termPages.value[libraryId]?.items || []
    const targetMap = new Map(pageItems.map((item) => [item.id, item]))
    const tasks = ids.map((id) => {
      const current = targetMap.get(id)
      if (!current || current.isEnabled === enabled) return null
      return updateExternalGlossaryTerm(libraryId, id, {
        keyword: current.keyword,
        category: current.category,
        aliases: current.aliases,
        matchMode: current.matchMode,
        description: current.description,
        descriptionFormat: current.descriptionFormat,
        display: current.display,
        isEnabled: enabled,
      })
    }).filter((task): task is Promise<ExternalGlossaryTermItem> => Boolean(task))
    if (!tasks.length) return
    const updates = await Promise.all(tasks)
    const updateMap = new Map(updates.map((item) => [item.id, item]))
    updateTermPage(libraryId, pageItems.map((item) => updateMap.get(item.id) || item))
  }

  async function setTermsDisplay(libraryId: string, ids: string[], display: 'standard' | 'minimal' | 'inherit') {
    const pageItems = termPages.value[libraryId]?.items || []
    const targetMap = new Map(pageItems.map((item) => [item.id, item]))
    const tasks = ids.map((id) => {
      const current = targetMap.get(id)
      if (!current || current.display === display) return null
      return updateExternalGlossaryTerm(libraryId, id, {
        keyword: current.keyword,
        category: current.category,
        aliases: current.aliases,
        matchMode: current.matchMode,
        description: current.description,
        descriptionFormat: current.descriptionFormat,
        display,
        isEnabled: current.isEnabled,
      })
    }).filter((task): task is Promise<ExternalGlossaryTermItem> => Boolean(task))
    if (!tasks.length) return
    const updates = await Promise.all(tasks)
    const updateMap = new Map(updates.map((item) => [item.id, item]))
    updateTermPage(libraryId, pageItems.map((item) => updateMap.get(item.id) || item))
  }

  async function importLibrary(payload: { library: ExternalGlossaryLibraryPayload; items: WorldKeywordPayload[]; replace?: boolean }) {
    const result = await importExternalGlossaryLibrary(payload)
    await ensureLibraries({ force: true })
    if (result.item?.id) {
      await ensureTerms(result.item.id, { force: true })
      await ensureCategories(result.item.id, { force: true })
    }
    return result
  }

  async function exportLibrary(libraryId: string) {
    return exportExternalGlossaryLibrary(libraryId)
  }

  async function importTerms(libraryId: string, items: WorldKeywordPayload[], replace = false) {
    const stats = await importExternalGlossaryTerms(libraryId, { items, replace })
    await ensureTerms(libraryId, { force: true })
    await ensureCategories(libraryId, { force: true })
    await ensureLibraries({ force: true })
    return stats
  }

  async function exportTerms(libraryId: string, category?: string) {
    return exportExternalGlossaryTerms(libraryId, category)
  }

  async function addCategory(libraryId: string, name: string) {
    const nameCreated = await createExternalGlossaryCategory(libraryId, name)
    await ensureCategories(libraryId, { force: true })
    return nameCreated
  }

  async function renameCategory(libraryId: string, oldName: string, newName: string) {
    const result = await renameExternalGlossaryCategory(libraryId, oldName, newName)
    await ensureTerms(libraryId, { force: true })
    await ensureCategories(libraryId, { force: true })
    return result
  }

  async function removeCategory(libraryId: string, name: string) {
    const updated = await deleteExternalGlossaryCategory(libraryId, name)
    await ensureTerms(libraryId, { force: true })
    await ensureCategories(libraryId, { force: true })
    return updated
  }

  function handleGatewayEvent(event?: any) {
    if (!event || event.type !== 'external-glossaries-updated') {
      return
    }
    const rawArgv = event?.argv || {}
    const options = (rawArgv.options || rawArgv.Options || {}) as Record<string, any>
    const revision = typeof options.revision === 'number' ? options.revision : typeof options.version === 'number' ? options.version : Date.now()
    if (revision <= libraryVersion.value) {
      return
    }
    libraryVersion.value = revision
    void ensureLibraries({ force: true })
    const libraryIds = Array.isArray(options.libraryIds) ? options.libraryIds.map((item) => String(item)) : []
    libraryIds.forEach((libraryId) => {
      if (termPages.value[libraryId]) {
        void ensureTerms(libraryId, { force: true })
      }
      if (categoriesMap.value[libraryId]) {
        void ensureCategories(libraryId, { force: true })
      }
    })
  }

  function ensureGateway() {
    if (gatewayBound) return
    chatEvent.on('external-glossaries-updated' as any, handleGatewayEvent)
    gatewayBound = true
  }

  ensureGateway()

  return {
    libraryPage,
    termPages,
    categoriesMap,
    libraryLoading,
    termLoadingMap,
    activeLibraryId,
    currentTerms,
    setActiveLibrary,
    ensureLibraries,
    ensureTerms,
    ensureCategories,
    createLibrary,
    editLibrary,
    removeLibrary,
    removeLibraries,
    sortLibraries,
    createTerm,
    editTerm,
    removeTerm,
    removeTerms,
    sortTerms,
    setTermsEnabled,
    setTermsDisplay,
    importLibrary,
    exportLibrary,
    importTerms,
    exportTerms,
    addCategory,
    renameCategory,
    removeCategory,
  }
})
