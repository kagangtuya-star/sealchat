import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { chatEvent, useChatStore } from './chat'
import type { WorldExternalGlossaryLibraryItem } from '@/models/worldExternalGlossary'
import {
  bulkDisableWorldExternalGlossaries,
  bulkEnableWorldExternalGlossaries,
  disableWorldExternalGlossary,
  enableWorldExternalGlossary,
  fetchWorldExternalGlossaries,
} from '@/models/worldExternalGlossary'

interface WorldExternalGlossaryPageState {
  items: WorldExternalGlossaryLibraryItem[]
  total: number
  fetchedAt: number
}

let gatewayBound = false

export const useWorldExternalGlossaryStore = defineStore('worldExternalGlossary', () => {
  const pages = ref<Record<string, WorldExternalGlossaryPageState>>({})
  const loadingMap = ref<Record<string, boolean>>({})
  const versionMap = ref<Record<string, number>>({})
  const managerVisible = ref(false)

  const currentWorldId = computed(() => useChatStore().currentWorldId)
  const currentLibraries = computed(() => {
    const worldId = currentWorldId.value
    return worldId ? pages.value[worldId]?.items || [] : []
  })

  function setManagerVisible(visible: boolean) {
    managerVisible.value = visible
  }

  function updatePage(worldId: string, items: WorldExternalGlossaryLibraryItem[], total?: number) {
    pages.value = {
      ...pages.value,
      [worldId]: {
        items: [...items].sort((a, b) => (b.sortOrder || 0) - (a.sortOrder || 0)),
        total: total ?? items.length,
        fetchedAt: Date.now(),
      },
    }
  }

  async function ensureLibraries(worldId: string, opts?: { force?: boolean }) {
    if (!worldId) return
    const page = pages.value[worldId]
    if (!opts?.force && page && Date.now() - page.fetchedAt < 60 * 1000) {
      return
    }
    loadingMap.value = { ...loadingMap.value, [worldId]: true }
    try {
      const data = await fetchWorldExternalGlossaries(worldId)
      updatePage(worldId, data.items, data.total)
      versionMap.value = { ...versionMap.value, [worldId]: Date.now() }
    } finally {
      loadingMap.value = { ...loadingMap.value, [worldId]: false }
    }
  }

  async function enableLibrary(worldId: string, libraryId: string) {
    await enableWorldExternalGlossary(worldId, libraryId)
    updatePage(worldId, (pages.value[worldId]?.items || []).map((item) => (item.id === libraryId ? { ...item, isBound: true } : item)))
  }

  async function disableLibrary(worldId: string, libraryId: string) {
    await disableWorldExternalGlossary(worldId, libraryId)
    updatePage(worldId, (pages.value[worldId]?.items || []).map((item) => (item.id === libraryId ? { ...item, isBound: false } : item)))
  }

  async function bulkEnable(worldId: string, libraryIds: string[]) {
    const updated = await bulkEnableWorldExternalGlossaries(worldId, libraryIds)
    if (updated > 0) {
      const idSet = new Set(libraryIds)
      updatePage(worldId, (pages.value[worldId]?.items || []).map((item) => (idSet.has(item.id) ? { ...item, isBound: true } : item)))
    }
    return updated
  }

  async function bulkDisable(worldId: string, libraryIds: string[]) {
    const updated = await bulkDisableWorldExternalGlossaries(worldId, libraryIds)
    if (updated > 0) {
      const idSet = new Set(libraryIds)
      updatePage(worldId, (pages.value[worldId]?.items || []).map((item) => (idSet.has(item.id) ? { ...item, isBound: false } : item)))
    }
    return updated
  }

  function handleGatewayEvent(event?: any) {
    if (!event || event.type !== 'world-external-glossaries-updated') {
      return
    }
    const rawArgv = event?.argv || {}
    const options = (rawArgv.options || rawArgv.Options || {}) as Record<string, any>
    const worldId = options.worldId as string | undefined
    if (!worldId) return
    const revision = typeof options.revision === 'number' ? options.revision : typeof options.version === 'number' ? options.version : Date.now()
    const currentRevision = versionMap.value[worldId] || 0
    if (revision <= currentRevision) {
      return
    }
    versionMap.value = { ...versionMap.value, [worldId]: revision }
    void ensureLibraries(worldId, { force: true })
  }

  function ensureGateway() {
    if (gatewayBound) return
    chatEvent.on('world-external-glossaries-updated' as any, handleGatewayEvent)
    gatewayBound = true
  }

  ensureGateway()

  return {
    pages,
    loadingMap,
    managerVisible,
    currentLibraries,
    setManagerVisible,
    ensureLibraries,
    enableLibrary,
    disableLibrary,
    bulkEnable,
    bulkDisable,
  }
})
