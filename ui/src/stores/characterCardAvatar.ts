import { defineStore } from 'pinia';
import { ref } from 'vue';
import { api } from './_config';
import { useUserStore } from './user';
import type { CharacterCard } from './characterCard';
import type { ChannelIdentity } from '@/types';
import { normalizeAttachmentId } from '@/composables/useAttachmentResolver';

export interface CharacterCardAvatarBinding {
  id: string;
  userId: string;
  channelId: string;
  externalCardId: string;
  cardName: string;
  sheetType: string;
  avatarAttachmentId: string;
  createdAt?: string;
  updatedAt?: string;
}

interface CharacterCardAvatarBindingPayload {
  channelId: string;
  externalCardId: string;
  cardName?: string;
  sheetType?: string;
  avatarAttachmentId: string;
}

const LEGACY_MIGRATION_FLAG_PREFIX = 'sealchat_character_card_avatar_migration_v1_done';

const buildLegacyMigrationFlagKey = (userId?: string, channelId?: string) => {
  if (!userId || !channelId) return '';
  return `${LEGACY_MIGRATION_FLAG_PREFIX}:${userId}:${channelId}`;
};

export const useCharacterCardAvatarStore = defineStore('characterCardAvatar', () => {
  const userStore = useUserStore();

  const bindingsByChannel = ref<Record<string, Record<string, CharacterCardAvatarBinding>>>({});
  const loadedChannels = ref<Record<string, boolean>>({});
  const migratingChannels = ref<Record<string, boolean>>({});

  const getBinding = (channelId: string, externalCardId: string) => {
    return bindingsByChannel.value[channelId]?.[externalCardId] || null;
  };

  const applyBinding = (item?: CharacterCardAvatarBinding | null) => {
    if (!item?.channelId || !item.externalCardId) {
      return null;
    }
    const channelMap = {
      ...(bindingsByChannel.value[item.channelId] || {}),
      [item.externalCardId]: item,
    };
    bindingsByChannel.value = {
      ...bindingsByChannel.value,
      [item.channelId]: channelMap,
    };
    loadedChannels.value = {
      ...loadedChannels.value,
      [item.channelId]: true,
    };
    return item;
  };

  const loadBindings = async (channelId: string) => {
    if (!channelId) return [];
    const resp = await api.get('/api/v1/character-card-avatar-bindings', { params: { channelId } });
    const items = Array.isArray(resp.data?.items) ? resp.data.items as CharacterCardAvatarBinding[] : [];
    const channelMap: Record<string, CharacterCardAvatarBinding> = {};
    items.forEach((item) => {
      if (item?.externalCardId) {
        channelMap[item.externalCardId] = item;
      }
    });
    bindingsByChannel.value = {
      ...bindingsByChannel.value,
      [channelId]: channelMap,
    };
    loadedChannels.value = {
      ...loadedChannels.value,
      [channelId]: true,
    };
    return items;
  };

  const ensureBindingsLoaded = async (channelId: string) => {
    if (!channelId || loadedChannels.value[channelId]) return;
    await loadBindings(channelId);
  };

  const upsertBinding = async (payload: CharacterCardAvatarBindingPayload) => {
    const resp = await api.post('/api/v1/character-card-avatar-bindings/upsert', {
      channelId: payload.channelId,
      externalCardId: payload.externalCardId,
      cardName: payload.cardName || '',
      sheetType: payload.sheetType || '',
      avatarAttachmentId: normalizeAttachmentId(payload.avatarAttachmentId || ''),
    });
    return applyBinding(resp.data?.item as CharacterCardAvatarBinding | undefined);
  };

  const removeBinding = async (channelId: string, externalCardId: string) => {
    if (!channelId || !externalCardId) return;
    await api.delete('/api/v1/character-card-avatar-bindings', {
      params: { channelId, externalCardId },
    });
    const channelMap = { ...(bindingsByChannel.value[channelId] || {}) };
    delete channelMap[externalCardId];
    bindingsByChannel.value = {
      ...bindingsByChannel.value,
      [channelId]: channelMap,
    };
    loadedChannels.value = {
      ...loadedChannels.value,
      [channelId]: true,
    };
  };

  const resolveCardAvatar = (externalCardId: string, channelId: string, fallbackAvatarUrl?: string) => {
    const binding = getBinding(channelId, externalCardId);
    if (binding?.avatarAttachmentId) {
      return binding.avatarAttachmentId;
    }
    return (fallbackAvatarUrl || '').trim();
  };

  const migrateLegacyBindings = async (
    channelId: string,
    cards: CharacterCard[],
    identities: ChannelIdentity[],
    localIdentityBindings: Record<string, string>,
  ) => {
    const userId = userStore.info?.id || '';
    const migrationKey = buildLegacyMigrationFlagKey(userId, channelId);
    if (!channelId || !userId || !migrationKey || migratingChannels.value[channelId]) {
      return [];
    }
    if (typeof window === 'undefined') {
      return [];
    }
    if (localStorage.getItem(migrationKey) === '1') {
      return [];
    }
    migratingChannels.value = {
      ...migratingChannels.value,
      [channelId]: true,
    };
    try {
      await ensureBindingsLoaded(channelId);
      const identityMap = new Map(identities.map(identity => [identity.id, identity] as const));
      const cardMap = new Map(cards.map(card => [card.id, card] as const));
      const deduped = new Map<string, CharacterCardAvatarBindingPayload>();
      for (const [identityId, cardId] of Object.entries(localIdentityBindings || {})) {
        const identity = identityMap.get(identityId);
        const card = cardMap.get(cardId);
        const attachmentId = normalizeAttachmentId(identity?.avatarAttachmentId || '');
        if (!card || !attachmentId || deduped.has(card.id) || getBinding(channelId, card.id)) {
          continue;
        }
        deduped.set(card.id, {
          channelId,
          externalCardId: card.id,
          cardName: card.name,
          sheetType: card.sheetType || '',
          avatarAttachmentId: attachmentId,
        });
      }
      const items = Array.from(deduped.values());
      if (items.length === 0) {
        localStorage.setItem(migrationKey, '1');
        return [];
      }
      const resp = await api.post('/api/v1/character-card-avatar-bindings/migrate-legacy', {
        channelId,
        items,
      });
      const createdItems = Array.isArray(resp.data?.items) ? resp.data.items as CharacterCardAvatarBinding[] : [];
      createdItems.forEach(item => {
        applyBinding(item);
      });
      localStorage.setItem(migrationKey, '1');
      return createdItems;
    } finally {
      const next = { ...migratingChannels.value };
      delete next[channelId];
      migratingChannels.value = next;
    }
  };

  return {
    bindingsByChannel,
    loadedChannels,
    getBinding,
    loadBindings,
    ensureBindingsLoaded,
    upsertBinding,
    removeBinding,
    resolveCardAvatar,
    migrateLegacyBindings,
  };
});
