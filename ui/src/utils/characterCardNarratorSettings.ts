export type CharacterCardNarratorSettings = Record<string, string[]>;

export const normalizeCharacterCardNarratorSettings = (value: unknown): CharacterCardNarratorSettings => {
  if (!value || typeof value !== 'object') {
    return {};
  }

  const next: CharacterCardNarratorSettings = {};
  for (const [channelId, rawList] of Object.entries(value as Record<string, unknown>)) {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId || !Array.isArray(rawList)) {
      continue;
    }
    const ids = Array.from(new Set(
      rawList
        .map((item) => String(item || '').trim())
        .filter(Boolean),
    ));
    if (ids.length > 0) {
      next[normalizedChannelId] = ids;
    }
  }
  return next;
};

export const isCharacterCardNarratorIdentity = (
  settings: CharacterCardNarratorSettings | undefined,
  channelId: string,
  identityId: string,
) => {
  const normalizedChannelId = String(channelId || '').trim();
  const normalizedIdentityId = String(identityId || '').trim();
  if (!normalizedChannelId || !normalizedIdentityId) {
    return false;
  }
  return (settings?.[normalizedChannelId] || []).includes(normalizedIdentityId);
};

export const clearNarratorBadgeCacheEntries = <
  T extends Record<string, Record<string, unknown>>,
>(
  cacheByChannel: T,
  channelId: string,
  identityIds: string[],
) => {
  const normalizedChannelId = String(channelId || '').trim();
  if (!normalizedChannelId || identityIds.length === 0) {
    return cacheByChannel;
  }

  const channelCache = cacheByChannel[normalizedChannelId];
  if (!channelCache || typeof channelCache !== 'object') {
    return cacheByChannel;
  }

  const removeSet = new Set(identityIds.map((item) => String(item || '').trim()).filter(Boolean));
  if (removeSet.size === 0) {
    return cacheByChannel;
  }

  const nextChannelCache = { ...channelCache };
  for (const identityId of removeSet) {
    delete nextChannelCache[identityId];
  }

  if (Object.keys(nextChannelCache).length === Object.keys(channelCache).length) {
    return cacheByChannel;
  }

  if (Object.keys(nextChannelCache).length === 0) {
    const { [normalizedChannelId]: _removed, ...rest } = cacheByChannel;
    return rest as T;
  }

  return {
    ...cacheByChannel,
    [normalizedChannelId]: nextChannelCache,
  };
};

export const resolveCharacterCardNarratorCountBadge = (identityIds: string[]) => {
  const count = Array.isArray(identityIds) ? identityIds.length : 0;
  return count > 0 ? String(count) : '';
};
