import { defineStore } from 'pinia'
import { ref } from 'vue'
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
  const latestRevisionByIdentity = ref<Record<string, number>>({})
  const savingIdentityId = ref('')

  const chatStore = useChatStore()
  const userStore = useUserStore()
  const displayStore = useDisplayStore()

  let gatewayBound = false

  const getUserId = () => userStore.info?.id || ''

  const upsertRemarkEntry = (entry: CharacterRemarkEntry) => {
    const latestRevision = latestRevisionByIdentity.value[entry.identityId] || 0
    if (entry.revision <= latestRevision) {
      return
    }
    latestRevisionByIdentity.value = {
      ...latestRevisionByIdentity.value,
      [entry.identityId]: entry.revision,
    }
    remarkByIdentity.value = { ...remarkByIdentity.value, [entry.identityId]: entry }
  }

  const removeRemarkEntry = (identityId: string) => {
    if (!identityId) return
    const next = { ...remarkByIdentity.value }
    delete next[identityId]
    remarkByIdentity.value = next
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
      const latestRevision = latestRevisionByIdentity.value[identityId] || 0
      if (revision < latestRevision) {
        return
      }
      latestRevisionByIdentity.value = {
        ...latestRevisionByIdentity.value,
        [identityId]: revision,
      }
      removeRemarkEntry(identityId)
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
      return
    }
    const next = { ...remarkByIdentity.value }
    const snapshotIdentityIds = new Set<string>()
    items.forEach((item: any) => {
      const identityId = typeof item?.identityId === 'string' ? item.identityId : ''
      const content = typeof item?.content === 'string' ? item.content.trim() : ''
      if (!identityId || !content || item?.action === 'clear') {
        return
      }
      snapshotIdentityIds.add(identityId)
      const revision = typeof item?.revision === 'number'
        ? item.revision
        : (typeof event?.timestamp === 'number' ? event.timestamp : Date.now())
      const latestRevision = latestRevisionByIdentity.value[identityId] || 0
      if (revision < latestRevision) {
        return
      }
      latestRevisionByIdentity.value = {
        ...latestRevisionByIdentity.value,
        [identityId]: revision,
      }
      const entry: CharacterRemarkEntry = {
        identityId,
        channelId,
        userId: typeof item?.userId === 'string' ? item.userId : '',
        content,
        revision,
      }
      next[identityId] = entry
    })
    Object.keys(next).forEach((key) => {
      if (next[key]?.channelId === channelId && !snapshotIdentityIds.has(key)) {
        delete next[key]
      }
    })
    remarkByIdentity.value = next
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

  ensureGateway()

  return {
    remarkByIdentity,
    savingIdentityId,
    requestRemarkSnapshot,
    saveRemark,
    getRemarkByIdentity,
    shouldShowRemark,
    isOwnedByCurrentUser,
    maxLength: CHARACTER_REMARK_MAX_LENGTH,
  }
})
