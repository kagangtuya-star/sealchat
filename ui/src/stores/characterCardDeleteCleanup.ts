export interface CharacterCardDeleteCleanupBadgeEntry {
  identityId: string
  channelId: string
  template: string
  attrs: Record<string, any>
  updatedAt: number
}

export interface CharacterCardDeleteCleanupActiveCard {
  name: string
  type: string
  attrs: Record<string, any>
  avatarUrl?: string
}

interface CleanupDeletedCharacterCardStateInput {
  channelId: string
  deletedCardId: string
  activeCardId?: string
  identityBindings: Record<string, string>
  badgeByIdentity: Record<string, CharacterCardDeleteCleanupBadgeEntry>
  badgeCacheByChannel: Record<string, Record<string, CharacterCardDeleteCleanupBadgeEntry>>
  activeCards: Record<string, CharacterCardDeleteCleanupActiveCard>
}

interface CleanupDeletedCharacterCardStateResult {
  affectedIdentityIds: string[]
  identityBindings: Record<string, string>
  badgeByIdentity: Record<string, CharacterCardDeleteCleanupBadgeEntry>
  badgeCacheByChannel: Record<string, Record<string, CharacterCardDeleteCleanupBadgeEntry>>
  activeCards: Record<string, CharacterCardDeleteCleanupActiveCard>
}

export function cleanupDeletedCharacterCardState(
  input: CleanupDeletedCharacterCardStateInput,
): CleanupDeletedCharacterCardStateResult {
  const channelId = String(input.channelId || '').trim()
  const deletedCardId = String(input.deletedCardId || '').trim()
  if (!channelId || !deletedCardId) {
    return {
      affectedIdentityIds: [],
      identityBindings: { ...input.identityBindings },
      badgeByIdentity: { ...input.badgeByIdentity },
      badgeCacheByChannel: { ...input.badgeCacheByChannel },
      activeCards: { ...input.activeCards },
    }
  }

  const affectedIdentityIds = Object.entries(input.identityBindings)
    .filter(([, cardId]) => cardId === deletedCardId)
    .map(([identityId]) => identityId)

  if (affectedIdentityIds.length === 0 && input.activeCardId !== deletedCardId) {
    return {
      affectedIdentityIds: [],
      identityBindings: { ...input.identityBindings },
      badgeByIdentity: { ...input.badgeByIdentity },
      badgeCacheByChannel: { ...input.badgeCacheByChannel },
      activeCards: { ...input.activeCards },
    }
  }

  const affectedSet = new Set(affectedIdentityIds)

  const identityBindings = { ...input.identityBindings }
  for (const identityId of affectedIdentityIds) {
    delete identityBindings[identityId]
  }

  const badgeByIdentity = { ...input.badgeByIdentity }
  for (const identityId of affectedIdentityIds) {
    delete badgeByIdentity[identityId]
  }

  const badgeCacheByChannel = { ...input.badgeCacheByChannel }
  const currentChannelCache = { ...(badgeCacheByChannel[channelId] || {}) }
  let cacheChanged = false
  for (const identityId of affectedIdentityIds) {
    if (!currentChannelCache[identityId]) continue
    delete currentChannelCache[identityId]
    cacheChanged = true
  }
  if (cacheChanged) {
    if (Object.keys(currentChannelCache).length === 0) {
      delete badgeCacheByChannel[channelId]
    } else {
      badgeCacheByChannel[channelId] = currentChannelCache
    }
  }

  const activeCards = { ...input.activeCards }
  if (input.activeCardId === deletedCardId) {
    delete activeCards[channelId]
  }

  return {
    affectedIdentityIds,
    identityBindings,
    badgeByIdentity,
    badgeCacheByChannel,
    activeCards,
  }
}
