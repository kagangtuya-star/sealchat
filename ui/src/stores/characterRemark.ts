import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { chatEvent, useChatStore } from './chat'
import { useDisplayStore } from './display'
import { useUserStore } from './user'

export interface CharacterRemarkEntry {
  identityId: string
  channelId: string
  userId: string
  content: string
  revision: number
}

const CHARACTER_REMARK_MAX_LENGTH = 80

export const useCharacterRemarkStore = defineStore('characterRemark', () => {
  const remarkByIdentity = ref<Record<string, CharacterRemarkEntry>>({})
  const remarkCacheByChannel = ref<Record<string, Record<string, CharacterRemarkEntry>>>({})
  const savingIdentityId = ref('')

  const chatStore = useChatStore()
  const userStore = useUserStore()
  const displayStore = useDisplayStore()

  let loadedCacheKey = ''
  let gatewayBound = false

  const getUserId = () => userStore.info?.id || ''

  const getCacheStorageKey = () => {
    const userId = getUserId()
    if (!userId || typeof window === 'undefined') {
      return ''
    }
    return `characterRemarkCache:${userId}`
  }

  const ensureCacheLoaded = () => {
    const key = getCacheStorageKey()
    if (!key || key === loadedCacheKey) {
      return key
    }
    loadedCacheKey = key
    try {
      const raw = localStorage.getItem(key)
      if (!raw) {
        remarkCacheByChannel.value = {}
        return key
      }
      const parsed = JSON.parse(raw)
      if (parsed && typeof parsed === 'object') {
        remarkCacheByChannel.value = parsed
      } else {
        remarkCacheByChannel.value = {}
      }
    } catch (error) {
      console.warn('Failed to load character remarks from localStorage', error)
      remarkCacheByChannel.value = {}
    }
    return key
  }

  const persistCache = () => {
    const key = ensureCacheLoaded()
    if (!key) {
      return
    }
    try {
      localStorage.setItem(key, JSON.stringify(remarkCacheByChannel.value))
    } catch (error) {
      console.warn('Failed to persist character remarks to localStorage', error)
    }
  }

  const loadRemarkCache = (channelId: string) => {
    if (!channelId) return
    const key = ensureCacheLoaded()
    if (!key) return
    const cached = remarkCacheByChannel.value[channelId]
    if (!cached || typeof cached !== 'object') {
      return
    }
    const next = { ...remarkByIdentity.value }
    let changed = false
    Object.values(cached).forEach((entry) => {
      if (!entry || typeof entry !== 'object') return
      const rawEntry = entry as CharacterRemarkEntry & { updatedAt?: number }
      const identityId = typeof rawEntry.identityId === 'string' ? rawEntry.identityId : ''
      if (!identityId) return
      const normalized: CharacterRemarkEntry = {
        identityId,
        channelId: typeof rawEntry.channelId === 'string' && rawEntry.channelId ? rawEntry.channelId : channelId,
        userId: typeof rawEntry.userId === 'string' ? rawEntry.userId : '',
        content: typeof rawEntry.content === 'string' ? rawEntry.content : '',
        revision: typeof rawEntry.revision === 'number'
          ? rawEntry.revision
          : (typeof rawEntry.updatedAt === 'number' ? rawEntry.updatedAt : 0),
      }
      if (!normalized.content.trim()) {
        return
      }
      const existing = next[identityId]
      if (!existing || normalized.revision > existing.revision) {
        next[identityId] = normalized
        changed = true
      }
    })
    if (changed) {
      remarkByIdentity.value = next
    }
  }

  const upsertRemarkEntry = (entry: CharacterRemarkEntry) => {
    const existing = remarkByIdentity.value[entry.identityId]
    if (existing && entry.revision <= existing.revision) {
      return
    }
    remarkByIdentity.value = { ...remarkByIdentity.value, [entry.identityId]: entry }
  }

  const removeRemarkEntry = (identityId: string) => {
    if (!identityId) return
    const next = { ...remarkByIdentity.value }
    delete next[identityId]
    remarkByIdentity.value = next
  }

  const upsertRemarkCacheEntry = (entry: CharacterRemarkEntry) => {
    if (!entry.identityId || !entry.channelId) return
    const key = ensureCacheLoaded()
    if (!key) return
    const channelMap = { ...(remarkCacheByChannel.value[entry.channelId] || {}) }
    const existing = channelMap[entry.identityId]
    if (existing && entry.revision <= existing.revision) {
      return
    }
    channelMap[entry.identityId] = entry
    remarkCacheByChannel.value = {
      ...remarkCacheByChannel.value,
      [entry.channelId]: channelMap,
    }
    persistCache()
  }

  const removeRemarkCacheEntry = (channelId: string, identityId: string) => {
    if (!channelId || !identityId) return
    const key = ensureCacheLoaded()
    if (!key) return
    const channelMap = { ...(remarkCacheByChannel.value[channelId] || {}) }
    if (!channelMap[identityId]) {
      return
    }
    delete channelMap[identityId]
    if (Object.keys(channelMap).length === 0) {
      const { [channelId]: _removed, ...rest } = remarkCacheByChannel.value
      remarkCacheByChannel.value = rest
    } else {
      remarkCacheByChannel.value = {
        ...remarkCacheByChannel.value,
        [channelId]: channelMap,
      }
    }
    persistCache()
  }

  const removeRemarkEntriesByChannel = (channelId: string) => {
    if (!channelId) return
    const next = { ...remarkByIdentity.value }
    let changed = false
    Object.keys(next).forEach((identityId) => {
      if (next[identityId]?.channelId === channelId) {
        delete next[identityId]
        changed = true
      }
    })
    if (changed) {
      remarkByIdentity.value = next
    }
  }

  const clearRemarkCacheForChannel = (channelId: string) => {
    if (!channelId) return
    const key = ensureCacheLoaded()
    if (!key) return
    if (!Object.prototype.hasOwnProperty.call(remarkCacheByChannel.value, channelId)) {
      return
    }
    const { [channelId]: _removed, ...rest } = remarkCacheByChannel.value
    remarkCacheByChannel.value = rest
    persistCache()
  }

  const replaceRemarkCacheForChannel = (channelId: string, entries: Record<string, CharacterRemarkEntry>) => {
    if (!channelId) return
    const key = ensureCacheLoaded()
    if (!key) return
    remarkCacheByChannel.value = {
      ...remarkCacheByChannel.value,
      [channelId]: entries,
    }
    persistCache()
  }

  const applyRemarkEvent = (event?: any) => {
    const payload = event?.characterRemark
    const identityId = typeof payload?.identityId === 'string' ? payload.identityId : ''
    if (!identityId) {
      return
    }
    const revision = typeof payload?.revision === 'number'
      ? payload.revision
      : (typeof event?.timestamp === 'number' ? event.timestamp : Date.now())
    const action = typeof payload?.action === 'string' ? payload.action : 'update'
    if (action === 'clear') {
      const existing = remarkByIdentity.value[identityId]
      if (existing && revision < existing.revision) {
        return
      }
      const channelId = typeof event?.channel?.id === 'string'
        ? event.channel.id
        : remarkByIdentity.value[identityId]?.channelId || ''
      removeRemarkEntry(identityId)
      if (channelId) {
        removeRemarkCacheEntry(channelId, identityId)
      }
      return
    }
    const channelId = typeof event?.channel?.id === 'string' ? event.channel.id : ''
    const content = typeof payload?.content === 'string' ? payload.content.trim() : ''
    if (!channelId || !content) {
      return
    }
    const entry: CharacterRemarkEntry = {
      identityId,
      channelId,
      userId: typeof payload?.userId === 'string' ? payload.userId : '',
      content,
      revision,
    }
    upsertRemarkEntry(entry)
    upsertRemarkCacheEntry(entry)
  }

  const applyRemarkSnapshot = (event?: any) => {
    const channelId = typeof event?.channel?.id === 'string' ? event.channel.id : ''
    if (!channelId) {
      return
    }
    const items = Array.isArray(event?.characterRemarkSnapshot?.items)
      ? event.characterRemarkSnapshot.items
      : []
    if (!items.length) {
      removeRemarkEntriesByChannel(channelId)
      clearRemarkCacheForChannel(channelId)
      return
    }
    const next = { ...remarkByIdentity.value }
    const cacheNext: Record<string, CharacterRemarkEntry> = {}
    Object.keys(next).forEach((key) => {
      if (next[key]?.channelId === channelId) {
        delete next[key]
      }
    })
    items.forEach((item: any) => {
      const identityId = typeof item?.identityId === 'string' ? item.identityId : ''
      const content = typeof item?.content === 'string' ? item.content.trim() : ''
      if (!identityId || !content || item?.action === 'clear') {
        return
      }
      const entry: CharacterRemarkEntry = {
        identityId,
        channelId,
        userId: typeof item?.userId === 'string' ? item.userId : '',
        content,
        revision: typeof item?.revision === 'number'
          ? item.revision
          : (typeof event?.timestamp === 'number' ? event.timestamp : Date.now()),
      }
      next[identityId] = entry
      cacheNext[identityId] = entry
    })
    remarkByIdentity.value = next
    replaceRemarkCacheForChannel(channelId, cacheNext)
  }

  const ensureGateway = () => {
    if (gatewayBound) return
    chatEvent.on('character-remark-updated' as any, applyRemarkEvent)
    chatEvent.on('character-remark-snapshot' as any, applyRemarkSnapshot)
    gatewayBound = true
  }

  const getRemarkByIdentity = (identityId: string, channelId?: string) => {
    const entry = remarkByIdentity.value[identityId]
    if (!entry) return null
    if (channelId && entry.channelId && entry.channelId !== channelId) {
      return null
    }
    if (!entry.content.trim()) {
      return null
    }
    return entry
  }

  const isOwnedByCurrentUser = (channelId: string, identityId: string) => {
    const userId = getUserId()
    if (!userId || !channelId || !identityId) {
      return false
    }
    const identities = chatStore.channelIdentities[channelId] || []
    return identities.some((identity) => identity.id === identityId && identity.userId === userId)
  }

  const shouldShowRemark = (entry: CharacterRemarkEntry | null | undefined) => {
    if (!entry?.content.trim()) {
      return false
    }
    const currentUserId = getUserId()
    const isSelf = !!currentUserId && entry.userId === currentUserId
    if (isSelf) {
      return displayStore.settings.showOwnIdentityRemark
    }
    return displayStore.settings.showOthersIdentityRemark
  }

  const requestRemarkSnapshot = async (channelId: string) => {
    if (!channelId) return
    loadRemarkCache(channelId)
    await chatStore.ensureConnectionReady()
    try {
      await chatStore.sendAPI('character.remark.snapshot', { channel_id: channelId } as any)
    } catch (error) {
      console.warn('Failed to request character remark snapshot', error)
    }
  }

  const saveRemark = async (channelId: string, identityId: string, content: string) => {
    if (!channelId || !identityId) {
      return { ok: false as const, error: '缺少频道或身份信息' }
    }
    const normalized = content.trim()
    if (normalized.length > CHARACTER_REMARK_MAX_LENGTH) {
      return { ok: false as const, error: `角色备注长度需在${CHARACTER_REMARK_MAX_LENGTH}个字符以内` }
    }
    savingIdentityId.value = identityId
    try {
      await chatStore.ensureConnectionReady()
      if (!normalized) {
        await chatStore.sendAPI('character.remark.broadcast', {
          channel_id: channelId,
          identity_id: identityId,
          action: 'clear',
        } as any)
      } else {
        await chatStore.sendAPI('character.remark.broadcast', {
          channel_id: channelId,
          identity_id: identityId,
          content: normalized,
          action: 'update',
        } as any)
      }
      return { ok: true as const }
    } catch (error: any) {
      const message = String(
        error?.response?.data?.error
        || error?.response?.err
        || error?.message
        || '保存角色备注失败',
      ).trim() || '保存角色备注失败'
      return { ok: false as const, error: message }
    } finally {
      if (savingIdentityId.value === identityId) {
        savingIdentityId.value = ''
      }
    }
  }

  watch(
    () => userStore.info?.id,
    () => {
      loadedCacheKey = ''
      ensureCacheLoaded()
    },
    { immediate: true },
  )

  ensureGateway()

  return {
    remarkByIdentity,
    remarkCacheByChannel,
    savingIdentityId,
    requestRemarkSnapshot,
    saveRemark,
    getRemarkByIdentity,
    shouldShowRemark,
    isOwnedByCurrentUser,
    maxLength: CHARACTER_REMARK_MAX_LENGTH,
  }
})
