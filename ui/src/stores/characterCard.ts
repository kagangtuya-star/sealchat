import { defineStore } from 'pinia';
import { ref, computed, watch } from 'vue';
import { chatEvent, useChatStore } from './chat';
import { useUserStore } from './user';
import { useDisplayStore } from './display';
import { extractTemplateKeys, getWorldCardTemplate } from '@/utils/characterCardTemplate';

// Character card type for UI (matching old API format)
export interface CharacterCard {
  id: string;
  name: string;
  sheetType: string;
  attrs?: Record<string, any>;
  channelId?: string;
  userId?: string;
  updatedAt?: number;
}

// Character card type from SealDice protocol
interface CharacterCardFromAPI {
  id: string;
  name: string;
  sheet_type: string;
  updated_at?: number;
}

// Active card data (from character.get)
export interface CharacterCardData {
  name: string;
  type: string;
  attrs: Record<string, any>;
  avatarUrl?: string;
}

export interface CharacterCardBadgeEntry {
  identityId: string;
  channelId: string;
  template: string;
  attrs: Record<string, any>;
  updatedAt: number;
}

// Convert API response to UI format
const toUICard = (card: CharacterCardFromAPI): CharacterCard => ({
  id: card.id,
  name: card.name,
  sheetType: card.sheet_type,
  updatedAt: card.updated_at,
});

export const useCharacterCardStore = defineStore('characterCard', () => {
  // List of user's character cards
  const cardList = ref<CharacterCard[]>([]);
  // Active card data per channel (from character.get)
  const activeCards = ref<Record<string, CharacterCardData>>({});
  // Badge data broadcasted by identities in channel
  const badgeByIdentity = ref<Record<string, CharacterCardBadgeEntry>>({});
  // Local identity bindings (cached for UI convenience)
  const identityBindings = ref<Record<string, string>>({});
  const badgeCacheByChannel = ref<Record<string, Record<string, CharacterCardBadgeEntry>>>({});

  const panelVisible = ref(false);
  const loading = ref(false);

  const chatStore = useChatStore();
  const userStore = useUserStore();
  const displayStore = useDisplayStore();
  let loadedBindingsKey = '';
  let loadedBadgeCacheKey = '';
  let badgeGatewayBound = false;

  const getBindingsStorageKey = () => {
    const userId = getUserId();
    if (!userId || typeof window === 'undefined') {
      return '';
    }
    return `characterCardIdentityBindings:${userId}`;
  };

  const loadIdentityBindings = () => {
    const key = getBindingsStorageKey();
    if (!key || key === loadedBindingsKey) {
      return;
    }
    loadedBindingsKey = key;
    try {
      const raw = localStorage.getItem(key);
      if (!raw) {
        identityBindings.value = {};
        return;
      }
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object') {
        identityBindings.value = parsed;
      } else {
        identityBindings.value = {};
      }
    } catch (e) {
      console.warn('Failed to load character card bindings from localStorage', e);
      identityBindings.value = {};
    }
  };

  const persistIdentityBindings = () => {
    const key = getBindingsStorageKey();
    if (!key) {
      return;
    }
    try {
      localStorage.setItem(key, JSON.stringify(identityBindings.value));
    } catch (e) {
      console.warn('Failed to persist character card bindings to localStorage', e);
    }
  };

  const getBadgeCacheStorageKey = () => {
    const userId = getUserId();
    if (!userId || typeof window === 'undefined') {
      return '';
    }
    return `characterCardBadgeCache:${userId}`;
  };

  const ensureBadgeCacheLoaded = () => {
    const key = getBadgeCacheStorageKey();
    if (!key || key === loadedBadgeCacheKey) {
      return key;
    }
    loadedBadgeCacheKey = key;
    try {
      const raw = localStorage.getItem(key);
      if (!raw) {
        badgeCacheByChannel.value = {};
        return key;
      }
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object') {
        badgeCacheByChannel.value = parsed;
      } else {
        badgeCacheByChannel.value = {};
      }
    } catch (e) {
      console.warn('Failed to load character card badges from localStorage', e);
      badgeCacheByChannel.value = {};
    }
    return key;
  };

  const persistBadgeCache = () => {
    const key = ensureBadgeCacheLoaded();
    if (!key) {
      return;
    }
    try {
      localStorage.setItem(key, JSON.stringify(badgeCacheByChannel.value));
    } catch (e) {
      console.warn('Failed to persist character card badges to localStorage', e);
    }
  };

  const loadBadgeCache = (channelId: string) => {
    if (!channelId) return;
    const key = ensureBadgeCacheLoaded();
    if (!key) return;
    const cached = badgeCacheByChannel.value[channelId];
    if (!cached || typeof cached !== 'object') {
      return;
    }
    const next = { ...badgeByIdentity.value };
    let changed = false;
    Object.values(cached).forEach((entry) => {
      if (!entry || typeof entry !== 'object') return;
      const identityId = typeof entry.identityId === 'string' ? entry.identityId : '';
      if (!identityId) return;
      const updatedAt = typeof entry.updatedAt === 'number' ? entry.updatedAt : 0;
      const normalized: CharacterCardBadgeEntry = {
        identityId,
        channelId: typeof entry.channelId === 'string' && entry.channelId ? entry.channelId : channelId,
        template: typeof entry.template === 'string' ? entry.template : '',
        attrs: entry?.attrs && typeof entry.attrs === 'object' ? entry.attrs : {},
        updatedAt,
      };
      const existing = next[identityId];
      if (!existing || normalized.updatedAt > existing.updatedAt) {
        next[identityId] = normalized;
        changed = true;
      }
    });
    if (changed) {
      badgeByIdentity.value = next;
    }
  };

  const upsertBadgeCacheEntry = (entry: CharacterCardBadgeEntry) => {
    if (!entry?.identityId || !entry.channelId) return;
    const key = ensureBadgeCacheLoaded();
    if (!key) return;
    const channelId = entry.channelId;
    const channelMap = { ...(badgeCacheByChannel.value[channelId] || {}) };
    const existing = channelMap[entry.identityId];
    if (existing && entry.updatedAt <= existing.updatedAt) {
      return;
    }
    channelMap[entry.identityId] = entry;
    badgeCacheByChannel.value = { ...badgeCacheByChannel.value, [channelId]: channelMap };
    persistBadgeCache();
  };

  const removeBadgeCacheEntry = (channelId: string, identityId: string) => {
    if (!channelId || !identityId) return;
    const key = ensureBadgeCacheLoaded();
    if (!key) return;
    const channelMap = { ...(badgeCacheByChannel.value[channelId] || {}) };
    if (!channelMap[identityId]) {
      return;
    }
    delete channelMap[identityId];
    if (Object.keys(channelMap).length === 0) {
      const { [channelId]: _removed, ...rest } = badgeCacheByChannel.value;
      badgeCacheByChannel.value = rest;
    } else {
      badgeCacheByChannel.value = { ...badgeCacheByChannel.value, [channelId]: channelMap };
    }
    persistBadgeCache();
  };

  const replaceBadgeCacheForChannel = (channelId: string, entries: Record<string, CharacterCardBadgeEntry>) => {
    if (!channelId) return;
    const key = ensureBadgeCacheLoaded();
    if (!key) return;
    badgeCacheByChannel.value = {
      ...badgeCacheByChannel.value,
      [channelId]: entries,
    };
    persistBadgeCache();
  };

  const resolveWorldBadgeTemplate = (worldId: string) => {
    if (!worldId) return '';
    const world = (chatStore as any).worldMap?.[worldId];
    const fromMap = typeof world?.characterCardBadgeTemplate === 'string' ? world.characterCardBadgeTemplate.trim() : '';
    if (fromMap) return fromMap;
    const fromDetail = (chatStore as any).worldDetailMap?.[worldId]?.world?.characterCardBadgeTemplate;
    if (typeof fromDetail === 'string' && fromDetail.trim()) {
      return fromDetail.trim();
    }
    return '';
  };

  const resolveBadgeTemplate = (worldId: string) => {
    const worldTemplate = resolveWorldBadgeTemplate(worldId);
    if (worldTemplate) return worldTemplate;
    const localTemplate = displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId];
    if (localTemplate && localTemplate.trim()) {
      return localTemplate.trim();
    }
    return getWorldCardTemplate(worldId);
  };

  const upsertBadgeEntry = (entry: CharacterCardBadgeEntry) => {
    const existing = badgeByIdentity.value[entry.identityId];
    if (existing && entry.updatedAt <= existing.updatedAt) {
      return;
    }
    badgeByIdentity.value = { ...badgeByIdentity.value, [entry.identityId]: entry };
  };

  const removeBadgeEntry = (identityId: string) => {
    if (!identityId) return;
    const next = { ...badgeByIdentity.value };
    delete next[identityId];
    badgeByIdentity.value = next;
  };

  const applyBadgeEvent = (event?: any) => {
    const payload = event?.characterCardBadge;
    const identityId = typeof payload?.identityId === 'string' ? payload.identityId : '';
    if (!identityId) {
      return;
    }
    const action = typeof payload?.action === 'string' ? payload.action : 'update';
    if (action === 'clear') {
      const channelId = typeof event?.channel?.id === 'string'
        ? event.channel.id
        : badgeByIdentity.value[identityId]?.channelId || '';
      removeBadgeEntry(identityId);
      if (channelId) {
        removeBadgeCacheEntry(channelId, identityId);
      }
      return;
    }
    const channelId = typeof event?.channel?.id === 'string' ? event.channel.id : '';
    const updatedAt = typeof event?.timestamp === 'number' ? event.timestamp : Math.floor(Date.now() / 1000);
    const template = typeof payload?.template === 'string' ? payload.template : '';
    const attrs = payload?.attrs && typeof payload.attrs === 'object' ? payload.attrs : {};
    const entry: CharacterCardBadgeEntry = {
      identityId,
      channelId,
      template,
      attrs,
      updatedAt,
    };
    upsertBadgeEntry(entry);
    upsertBadgeCacheEntry(entry);
  };

  const applyBadgeSnapshot = (event?: any) => {
    const channelId = typeof event?.channel?.id === 'string' ? event.channel.id : '';
    if (!channelId) {
      return;
    }
    const items = Array.isArray(event?.characterCardBadgeSnapshot?.items)
      ? event.characterCardBadgeSnapshot.items
      : [];
    if (!items.length) {
      loadBadgeCache(channelId);
      return;
    }
    const updatedAt = typeof event?.timestamp === 'number' ? event.timestamp : Math.floor(Date.now() / 1000);
    const next = { ...badgeByIdentity.value };
    const cacheNext: Record<string, CharacterCardBadgeEntry> = {};
    Object.keys(next).forEach((key) => {
      if (next[key]?.channelId === channelId) {
        delete next[key];
      }
    });
    for (const item of items) {
      const identityId = typeof item?.identityId === 'string' ? item.identityId : '';
      if (!identityId) continue;
      if (item?.action === 'clear') continue;
      const template = typeof item?.template === 'string' ? item.template : '';
      const attrs = item?.attrs && typeof item.attrs === 'object' ? item.attrs : {};
      const entry: CharacterCardBadgeEntry = {
        identityId,
        channelId,
        template,
        attrs,
        updatedAt,
      };
      next[identityId] = entry;
      cacheNext[identityId] = entry;
    }
    badgeByIdentity.value = next;
    replaceBadgeCacheForChannel(channelId, cacheNext);
  };

  const ensureBadgeGateway = () => {
    if (badgeGatewayBound) return;
    chatEvent.on('character-card-badge-updated' as any, applyBadgeEvent);
    chatEvent.on('character-card-badge-snapshot' as any, applyBadgeSnapshot);
    badgeGatewayBound = true;
  };

  // Get user ID for API calls
  const getUserId = () => {
    return userStore.info?.id || '';
  };

  // Load character card list from SealDice via WebSocket
  const loadCardList = async (channelId?: string) => {
    const userId = getUserId();
    if (!userId) {
      console.warn('[CharacterCard] loadCardList skipped: no userId');
      return;
    }

    const resolvedChannelId = channelId || chatStore.curChannel?.id || '';

    // Ensure WebSocket is connected before sending API request
    await chatStore.ensureConnectionReady();

    loading.value = true;
    try {
      console.log('[CharacterCard] Sending character.list request for user:', userId);
      const payload: Record<string, string> = { user_id: userId };
      if (resolvedChannelId) {
        payload.group_id = resolvedChannelId;
      }
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; list?: CharacterCardFromAPI[] } }>('character.list', payload);
      console.log('[CharacterCard] character.list response:', resp);
      if (resp?.data?.ok && Array.isArray(resp.data.list)) {
        cardList.value = resp.data.list.map(toUICard);
      }
    } catch (e) {
      console.warn('Failed to load character card list', e);
    } finally {
      loading.value = false;
    }
  };

  // Backwards compatible loadCards (accepts optional channelId)
  const loadCards = async (channelId?: string) => {
    await loadCardList(channelId);
    loadIdentityBindings();
    if (channelId) {
      await getActiveCard(channelId);
    }
  };

  // Get active card for a channel
  const getActiveCard = async (channelId: string) => {
    const userId = getUserId();
    if (!userId || !channelId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; data?: Record<string, any>; name?: string; type?: string } }>('character.get', {
        group_id: channelId,
        user_id: userId,
      });
      if (resp?.data?.ok) {
        const cardData: CharacterCardData = {
          name: resp.data.name || '',
          type: resp.data.type || '',
          attrs: resp.data.data || {},
        };
        activeCards.value[channelId] = cardData;
        void broadcastActiveBadge(channelId);
        return cardData;
      }
    } catch (e) {
      console.warn('Failed to get active card', e);
    }
    return null;
  };

  // Create a new character card
  const createCard = async (channelId: string, name: string, sheetType: string = 'coc7', _attrs: Record<string, any> = {}) => {
    const userId = getUserId();
    if (!userId || !channelId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; id?: string; name?: string; sheet_type?: string } }>('character.new', {
        user_id: userId,
        group_id: channelId,
        name,
        sheet_type: sheetType,
      });
      if (resp?.data?.ok) {
        await loadCardList(channelId);
        return {
          id: resp.data.id,
          name: resp.data.name,
          sheetType: resp.data.sheet_type,
        };
      }
    } catch (e) {
      console.warn('Failed to create character card', e);
    }
    return null;
  };

  // Save current group's card data as a character card
  const saveCard = async (channelId: string, name: string, sheetType: string = 'coc7') => {
    const userId = getUserId();
    if (!userId || !channelId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; id?: string; name?: string; action?: string } }>('character.save', {
        user_id: userId,
        group_id: channelId,
        name,
        sheet_type: sheetType,
      });
      if (resp?.data?.ok) {
        await loadCardList(channelId);
        return resp.data;
      }
    } catch (e) {
      console.warn('Failed to save character card', e);
    }
    return null;
  };

  // Update card attributes - backwards compatible signature
  // Old: updateCard(cardId, name, sheetType, attrs)
  // New: Uses character.set with channelId
  const updateCard = async (cardIdOrChannelId: string, name: string, sheetTypeOrAttrs: string | Record<string, any>, attrsOrUndefined?: Record<string, any>) => {
    const userId = getUserId();
    if (!userId) return null;

    // Determine if this is old style (cardId, name, sheetType, attrs) or new style (channelId, name, attrs)
    let channelId: string;
    let attrs: Record<string, any>;

    if (typeof sheetTypeOrAttrs === 'object') {
      // New style: (channelId, name, attrs)
      channelId = cardIdOrChannelId;
      attrs = sheetTypeOrAttrs;
    } else {
      // Old style: (cardId, name, sheetType, attrs)
      // For now, use current channel as fallback
      channelId = chatStore.curChannel?.id || '';
      attrs = attrsOrUndefined || {};
    }

    if (!channelId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean } }>('character.set', {
        group_id: channelId,
        user_id: userId,
        name,
        attrs,
      });
      if (resp?.data?.ok) {
        await getActiveCard(channelId);
        await loadCardList(channelId);
        return true;
      }
    } catch (e) {
      console.warn('Failed to update character card', e);
    }
    return false;
  };

  // Bind/unbind card to channel (character.tag)
  const tagCard = async (channelId: string, cardName?: string, cardId?: string) => {
    const userId = getUserId();
    if (!userId || !channelId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const payload: Record<string, string> = {
        user_id: userId,
        group_id: channelId,
      };
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; action?: string; id?: string; name?: string } }>('character.tag', payload);
      if (resp?.data?.ok) {
        await getActiveCard(channelId);
        return resp.data;
      }
    } catch (e) {
      console.warn('Failed to tag character card', e);
    }
    return null;
  };

  // Unbind card from all channels
  const untagAllCard = async (cardName?: string, cardId?: string, channelId?: string) => {
    const userId = getUserId();
    if (!userId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const payload: Record<string, string> = { user_id: userId };
      const resolvedChannelId = channelId || chatStore.curChannel?.id || '';
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;
      if (resolvedChannelId) payload.group_id = resolvedChannelId;

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; unbound_count?: number } }>('character.untagAll', payload);
      if (resp?.data?.ok) {
        return resp.data;
      }
    } catch (e) {
      console.warn('Failed to untag all character card', e);
    }
    return null;
  };

  // Load card data to channel's independent card
  const loadCard = async (channelId: string, cardName?: string, cardId?: string) => {
    const userId = getUserId();
    if (!userId || !channelId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const payload: Record<string, string> = {
        user_id: userId,
        group_id: channelId,
      };
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; id?: string; name?: string; sheet_type?: string } }>('character.load', payload);
      if (resp?.data?.ok) {
        await getActiveCard(channelId);
        return resp.data;
      }
    } catch (e) {
      console.warn('Failed to load character card', e);
    }
    return null;
  };

  // Delete a character card - backwards compatible (accepts cardId as first param)
  const deleteCard = async (cardIdOrName?: string, cardId?: string) => {
    const userId = getUserId();
    if (!userId) return null;

    await chatStore.ensureConnectionReady();

    try {
      const payload: Record<string, string> = { user_id: userId };
      const resolvedChannelId = chatStore.curChannel?.id || '';
      if (resolvedChannelId) {
        payload.group_id = resolvedChannelId;
      }

      // If second param is provided, first is name, second is id
      // If only first param, treat it as cardId
      if (cardId) {
        payload.name = cardIdOrName || '';
        payload.id = cardId;
      } else if (cardIdOrName) {
        payload.id = cardIdOrName;
      }

      if (!payload.id && !payload.name) {
        throw new Error('角色卡ID或名称不能为空');
      }

      const untagResp = await chatStore.sendAPI<{ data: { ok: boolean; error?: string } }>('character.untagAll', payload);
      if (!untagResp?.data?.ok) {
        throw new Error(untagResp?.data?.error || '解绑失败');
      }

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; error?: string; binding_groups?: string[] } }>('character.delete', payload);
      if (resp?.data?.ok) {
        await loadCardList(resolvedChannelId);
        return resp.data;
      } else if (resp?.data?.error) {
        throw new Error(resp.data.error);
      }
    } catch (e) {
      console.warn('Failed to delete character card', e);
      throw e;
    }
    return null;
  };

  // Get cards list (computed)
  const cards = computed(() => cardList.value);

  // Get card by ID from list
  const getCardById = (cardId: string) => {
    return cardList.value.find(c => c.id === cardId);
  };

  // Get card by name from list
  const getCardByName = (name: string) => {
    return cardList.value.find(c => c.name === name);
  };

  // Resolve active card ID for a channel by matching name/type with list
  const getActiveCardId = (channelId: string) => {
    const active = activeCards.value[channelId];
    if (!active) return '';
    const byNameAndType = cardList.value.find(card =>
      card.name === active.name && (!active.type || card.sheetType === active.type),
    );
    if (byNameAndType) return byNameAndType.id;
    const byName = cardList.value.find(card => card.name === active.name);
    return byName?.id || '';
  };

  // Backwards compatibility: getCardsByChannel returns all cards (SealDice doesn't filter by channel)
  const getCardsByChannel = (_channelId: string) => cardList.value;

  // Backwards compatibility: getBoundCardId
  const getBoundCardId = (identityId: string) => identityBindings.value[identityId];

  // Backwards compatibility: bindIdentity persists mapping locally then syncs SealDice
  const bindIdentity = async (channelId: string, identityId: string, cardId: string) => {
    if (!channelId || !identityId || !cardId) return null;
    loadIdentityBindings();
    identityBindings.value[identityId] = cardId;
    persistIdentityBindings();
    if (chatStore.getActiveIdentityId(channelId) === identityId) {
      await tagCard(channelId, undefined, cardId);
      await loadCards(channelId);
    }
    return { ok: true };
  };

  // Backwards compatibility: unbindIdentity persists mapping locally then syncs SealDice
  const unbindIdentity = async (channelId: string, identityId: string) => {
    if (!channelId || !identityId) return null;
    loadIdentityBindings();
    delete identityBindings.value[identityId];
    persistIdentityBindings();
    if (chatStore.getActiveIdentityId(channelId) === identityId) {
      await tagCard(channelId);
      await loadCards(channelId);
    }
    return { ok: true };
  };

  const requestBadgeSnapshot = async (channelId: string) => {
    if (!channelId) return;
    loadBadgeCache(channelId);
    await chatStore.ensureConnectionReady();
    try {
      await chatStore.sendAPI('character.badge.snapshot', { channel_id: channelId });
    } catch (e) {
      console.warn('Failed to request badge snapshot', e);
    }
  };

  const broadcastActiveBadge = async (channelId: string, identityId?: string, action: 'update' | 'clear' = 'update') => {
    if (!channelId) return;
    const resolvedIdentityId = identityId || chatStore.getActiveIdentityId(channelId);
    if (!resolvedIdentityId) return;
    await chatStore.ensureConnectionReady();
    if (!displayStore.settings.characterCardBadgeEnabled) {
      action = 'clear';
    }
    if (action === 'clear') {
      try {
        await chatStore.sendAPI('character.badge.broadcast', {
          channel_id: channelId,
          identity_id: resolvedIdentityId,
          action: 'clear',
        });
      } catch (e) {
        console.warn('Failed to clear badge', e);
      }
      return;
    }
    const attrsSource = activeCards.value[channelId]?.attrs;
    if (!attrsSource) {
      await broadcastActiveBadge(channelId, resolvedIdentityId, 'clear');
      return;
    }
    const worldId = chatStore.currentWorldId || '';
    const template = resolveBadgeTemplate(worldId);
    if (!template) {
      await broadcastActiveBadge(channelId, resolvedIdentityId, 'clear');
      return;
    }
    const keys = extractTemplateKeys(template);
    const filteredAttrs: Record<string, any> = {};
    if (keys.length > 0) {
      for (const key of keys) {
        const value = attrsSource[key];
        if (value !== undefined && value !== null && value !== '') {
          filteredAttrs[key] = value;
        }
      }
      if (Object.keys(filteredAttrs).length === 0) {
        await broadcastActiveBadge(channelId, resolvedIdentityId, 'clear');
        return;
      }
    }
    try {
      await chatStore.sendAPI('character.badge.broadcast', {
        channel_id: channelId,
        identity_id: resolvedIdentityId,
        template,
        attrs: keys.length > 0 ? filteredAttrs : {},
        action: 'update',
      });
    } catch (e) {
      console.warn('Failed to broadcast badge', e);
    }
  };

  const getBadgeByIdentity = (identityId: string) => badgeByIdentity.value[identityId];

  watch(
    () => userStore.info?.id,
    () => {
      loadedBindingsKey = '';
      loadIdentityBindings();
    },
    { immediate: true },
  );

  ensureBadgeGateway();

  return {
    cardList,
    cards,
    activeCards,
    badgeByIdentity,
    identityBindings,
    panelVisible,
    loading,
    loadCardList,
    loadCards,
    getActiveCard,
    createCard,
    saveCard,
    updateCard,
    tagCard,
    untagAllCard,
    loadCard,
    deleteCard,
    getCardById,
    getCardByName,
    getActiveCardId,
    getCardsByChannel,
    getBadgeByIdentity,
    getBoundCardId,
    bindIdentity,
    unbindIdentity,
    requestBadgeSnapshot,
    broadcastActiveBadge,
  };
});
