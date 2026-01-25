import { defineStore } from 'pinia';
import { ref, computed, watch } from 'vue';
import { useChatStore } from './chat';
import { useUserStore } from './user';

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
  // Local identity bindings (cached for UI convenience)
  const identityBindings = ref<Record<string, string>>({});

  const panelVisible = ref(false);
  const loading = ref(false);

  const chatStore = useChatStore();
  const userStore = useUserStore();
  let loadedBindingsKey = '';

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

  // Get user ID for API calls
  const getUserId = () => {
    return userStore.info?.id || '';
  };

  // Load character card list from SealDice via WebSocket
  const loadCardList = async () => {
    const userId = getUserId();
    if (!userId) {
      console.warn('[CharacterCard] loadCardList skipped: no userId');
      return;
    }

    // Ensure WebSocket is connected before sending API request
    await chatStore.ensureConnectionReady();

    loading.value = true;
    try {
      console.log('[CharacterCard] Sending character.list request for user:', userId);
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; list?: CharacterCardFromAPI[] } }>('character.list', {
        user_id: userId,
      });
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
    await loadCardList();
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
        await loadCardList();
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
        await loadCardList();
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
        await loadCardList();
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
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;
      if (channelId) payload.group_id = channelId;

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
        await loadCardList();
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

  watch(
    () => userStore.info?.id,
    () => {
      loadedBindingsKey = '';
      loadIdentityBindings();
    },
    { immediate: true },
  );

  return {
    cardList,
    cards,
    activeCards,
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
    getBoundCardId,
    bindIdentity,
    unbindIdentity,
  };
});
