import { defineStore } from 'pinia';
import { ref, computed, watch } from 'vue';
import { chatEvent, useChatStore } from './chat';
import { useUserStore } from './user';
import { useDisplayStore } from './display';
import { useCharacterCardTemplateStore } from './characterCardTemplate';
import { useCharacterSheetStore } from './characterSheet';
import {
  getWorldCardTemplate,
  hasRenderableBadgeData,
} from '@/utils/characterCardTemplate';
import {
  clearNarratorBadgeCacheEntries,
  isCharacterCardNarratorIdentity,
  normalizeCharacterCardNarratorSettings,
  type CharacterCardNarratorSettings,
} from '@/utils/characterCardNarratorSettings';
import { cleanupDeletedCharacterCardState } from './characterCardDeleteCleanup';
import { useCharacterCardAvatarStore } from './characterCardAvatar';
import { useChannelCharacterSnapshotStore } from './channelCharacterSnapshot';
import {
  buildBotNicknameSyncCommand,
  resolveBotNicknameSyncName,
  shouldEnableBotNicknameSyncForChannel,
} from '@/utils/botNicknameSync';
import {
  CHARACTER_SNAPSHOT_BADGE_TEMPLATE_PRESETS,
  CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS,
  getCharacterSnapshotTemplatePreset,
} from '@/utils/characterSnapshotTemplatePresets';

// Character card type for UI (matching old API format)
export interface CharacterCard {
  id: string;
  name: string;
  sheetType: string;
  attrs?: Record<string, any>;
  templateMode?: 'managed' | 'detached';
  templateId?: string;
  templateSnapshot?: string;
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
  templateText?: string;
}

export interface CharacterCardBadgeEntry {
  identityId: string;
  channelId: string;
  template: string;
  attrs: Record<string, any>;
  updatedAt: number;
}

export interface OnlineCharacterCardItem {
  userId: string;
  username?: string;
  userNick?: string;
  userColor?: string;
  identityId: string;
  identityName?: string;
  identityColor?: string;
  identityAvatar?: string;
  card: {
    name: string;
    sheetType: string;
    attrs: Record<string, any>;
    templateText?: string;
  };
  updatedAt?: number;
}

type CharacterApiRevalidateResult =
  | { ok: true }
  | { ok: false; error: string };

const normalizeCharacterApiDisabledReason = (reason?: string, channel?: any) => {
  const msg = String(reason || '').trim();
  if (!msg) {
    return characterApiUnsupportedText;
  }
  if (msg === '请求超时' || msg.includes('请求超时')) {
    const primaryBotId = String(channel?.primaryBotId || '').trim();
    if (primaryBotId) {
      return '当前主控 BOT 请求超时，请切换主控 BOT 或重新验证。';
    }
    return '当前主控 BOT 请求超时，请重新验证。';
  }
  return msg;
};

interface SyncCardForIdentityOptions {
  preserveWhenUnbound?: boolean;
  reloadAfterSwitch?: boolean;
}

interface SyncCardForIdentityResult {
  ok: boolean;
  switched: boolean;
  preserved: boolean;
  boundCardId?: string;
}

// Convert API response to UI format
const toUICard = (card: CharacterCardFromAPI): CharacterCard => ({
  id: card.id,
  name: card.name,
  sheetType: card.sheet_type,
  updatedAt: card.updated_at,
});

const isDebugEnabled = () => typeof window !== 'undefined' && (window as any).__SC_DEBUG__ === true;
export const characterApiUnsupportedText = '当前BOT不支持人物卡API、未开启或未启用。';

export const useCharacterCardStore = defineStore('characterCard', () => {
  // List of user's character cards
  const cardList = ref<CharacterCard[]>([]);
  // Active card data per channel (from character.get)
  const activeCards = ref<Record<string, CharacterCardData>>({});
  // Badge data broadcasted by identities in channel
  const badgeByIdentity = ref<Record<string, CharacterCardBadgeEntry>>({});
  // Local identity bindings. Shared identities use one binding key across copies.
  const identityBindings = ref<Record<string, string>>({});
  const lastBotNicknameSyncByChannel = ref<Record<string, string>>({});
  const badgeCacheByChannel = ref<Record<string, Record<string, CharacterCardBadgeEntry>>>({});
  const onlineCardsByChannel = ref<Record<string, Record<string, OnlineCharacterCardItem>>>({});
  const onlineCardsLoadingByChannel = ref<Record<string, boolean>>({});
  const narratorIdentityIdsByChannel = ref<CharacterCardNarratorSettings>({});
  const botCharacterDisabledByChannel = ref<Record<string, boolean>>({});
  const characterApiHealthySessionByChannel = ref<Record<string, boolean>>({});

  const panelVisible = ref(false);
  const loading = ref(false);

  const chatStore = useChatStore();
  const userStore = useUserStore();
  const displayStore = useDisplayStore();
  const templateStore = useCharacterCardTemplateStore();
  const sheetStore = useCharacterSheetStore();
  const avatarStore = useCharacterCardAvatarStore();
  const snapshotStore = useChannelCharacterSnapshotStore();
  let loadedBindingsKey = '';
  let loadedBadgeCacheKey = '';
  let loadedNarratorSettingsKey = '';
  let badgeGatewayBound = false;
  const revalidateCharacterApiInFlight = new Map<string, Promise<CharacterApiRevalidateResult>>();

  const isBotCharacterDisabled = (channelId?: string) => {
    if (!channelId) {
      return true;
    }
    const channel = chatStore.findChannelById(channelId) as any;
    if (channel && channel.characterApiEnabled !== true) {
      return true;
    }
    return botCharacterDisabledByChannel.value[channelId] === true;
  };

  const markCharacterApiHealthy = (channelId?: string) => {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId || characterApiHealthySessionByChannel.value[normalizedChannelId] === true) {
      return;
    }
    characterApiHealthySessionByChannel.value = {
      ...characterApiHealthySessionByChannel.value,
      [normalizedChannelId]: true,
    };
  };

  const markBotCharacterDisabled = (channelId: string) => {
    if (!channelId || botCharacterDisabledByChannel.value[channelId]) {
      return;
    }
    botCharacterDisabledByChannel.value = {
      ...botCharacterDisabledByChannel.value,
      [channelId]: true,
    };
    if (isDebugEnabled()) {
      console.warn('[CharacterCard] character api disabled by bot capability', { channelId });
    }
  };

  const clearBotCharacterDisabled = (channelId: string) => {
    if (!channelId || botCharacterDisabledByChannel.value[channelId] !== true) {
      return;
    }
    const next = { ...botCharacterDisabledByChannel.value };
    delete next[channelId];
    botCharacterDisabledByChannel.value = next;
    if (isDebugEnabled()) {
      console.warn('[CharacterCard] character api capability restored', { channelId });
    }
  };

  const maybeDisableFromResponse = (channelId: string, resp: any) => {
    if (resp?.data?.ok === true) {
      markCharacterApiHealthy(channelId);
      return;
    }
    const err = resp?.data?.error;
    if (resp?.data?.ok === false && err === characterApiUnsupportedText) {
      markBotCharacterDisabled(channelId);
    }
  };

  const shouldSkipCharacterApi = (channelId: string, label: string) => {
    if (!channelId) {
      return false;
    }
    if (isBotCharacterDisabled(channelId)) {
      if (isDebugEnabled()) {
        console.warn(`[CharacterCard] ${label} skipped: bot character api disabled`, { channelId });
      }
      return true;
    }
    return false;
  };

  const assertCharacterApiEnabled = (channelId: string, label: string) => {
    if (shouldSkipCharacterApi(channelId, label)) {
      const error = new Error(characterApiUnsupportedText);
      (error as any).response = { data: { error: characterApiUnsupportedText } };
      throw error;
    }
  };

  const getCharacterApiDisabledReason = (channelId?: string) => {
    if (!channelId) {
      return characterApiUnsupportedText;
    }
    const channel = chatStore.findChannelById(channelId) as any;
    if (typeof channel?.characterApiReason === 'string' && channel.characterApiReason.trim()) {
      return normalizeCharacterApiDisabledReason(channel.characterApiReason, channel);
    }
    return characterApiUnsupportedText;
  };

  const hasSuccessfulCharacterApiSession = (channelId?: string) => {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId) {
      return false;
    }
    if (characterApiHealthySessionByChannel.value[normalizedChannelId] === true) {
      return true;
    }
    const channel = chatStore.findChannelById(normalizedChannelId) as any;
    if (channel?.characterApiEnabled === true) {
      markCharacterApiHealthy(normalizedChannelId);
      return true;
    }
    return false;
  };

  const isCharacterApiReady = (channelId?: string) => {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId) {
      return false;
    }
    return !isBotCharacterDisabled(normalizedChannelId);
  };

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

  const sharedIdentityBindingKey = (sharedIdentityId?: string) => {
    const normalized = String(sharedIdentityId || '').trim();
    return normalized ? `shared:${normalized}` : '';
  };

  const getIdentityBindingKey = (channelId: string, identityId: string, sharedIdentityId?: string) => {
    const sharedKey = sharedIdentityBindingKey(sharedIdentityId);
    if (sharedKey) return sharedKey;
    const identity = (chatStore.channelIdentities[channelId] || []).find(item => item.id === identityId);
    return sharedIdentityBindingKey(identity?.sharedIdentityId) || identityId;
  };

  const getBadgeCacheStorageKey = () => {
    const userId = getUserId();
    if (!userId || typeof window === 'undefined') {
      return '';
    }
    return `characterCardBadgeCache:${userId}`;
  };

  const getNarratorSettingsStorageKey = () => {
    const userId = getUserId();
    if (!userId || typeof window === 'undefined') {
      return '';
    }
    return `characterCardNarratorSettings:${userId}`;
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

  const ensureNarratorSettingsLoaded = () => {
    const key = getNarratorSettingsStorageKey();
    if (!key || key === loadedNarratorSettingsKey) {
      return key;
    }
    loadedNarratorSettingsKey = key;
    try {
      narratorIdentityIdsByChannel.value = normalizeCharacterCardNarratorSettings(localStorage.getItem(key)
        ? JSON.parse(localStorage.getItem(key) || '{}')
        : {});
    } catch (e) {
      console.warn('Failed to load character card narrator settings from localStorage', e);
      narratorIdentityIdsByChannel.value = {};
    }
    return key;
  };

  const persistNarratorSettings = () => {
    const key = ensureNarratorSettingsLoaded();
    if (!key) {
      return;
    }
    try {
      localStorage.setItem(key, JSON.stringify(narratorIdentityIdsByChannel.value));
    } catch (e) {
      console.warn('Failed to persist character card narrator settings to localStorage', e);
    }
  };

  const isNarratorIdentity = (channelId: string, identityId: string) => {
    ensureNarratorSettingsLoaded();
    return isCharacterCardNarratorIdentity(narratorIdentityIdsByChannel.value, channelId, identityId);
  };

  const getNarratorIdentityIds = (channelId: string) => {
    ensureNarratorSettingsLoaded();
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId) {
      return [];
    }
    return narratorIdentityIdsByChannel.value[normalizedChannelId] || [];
  };

  const applyNarratorBadgeCleanup = (channelId: string, identityIds: string[]) => {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId || identityIds.length === 0) {
      return;
    }
    const removeSet = new Set(identityIds.map(item => String(item || '').trim()).filter(Boolean));
    if (removeSet.size === 0) {
      return;
    }
    const nextBadgeByIdentity = { ...badgeByIdentity.value };
    for (const identityId of removeSet) {
      if (nextBadgeByIdentity[identityId]?.channelId === normalizedChannelId) {
        delete nextBadgeByIdentity[identityId];
      }
    }
    badgeByIdentity.value = nextBadgeByIdentity;
    badgeCacheByChannel.value = clearNarratorBadgeCacheEntries(
      badgeCacheByChannel.value,
      normalizedChannelId,
      Array.from(removeSet),
    );
    persistBadgeCache();
  };

  const setNarratorIdentityIds = (channelId: string, identityIds: string[]) => {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId) {
      return;
    }
    ensureNarratorSettingsLoaded();
    const normalized = Array.from(new Set(identityIds.map(item => String(item || '').trim()).filter(Boolean)));
    const next = { ...narratorIdentityIdsByChannel.value };
    if (normalized.length === 0) {
      delete next[normalizedChannelId];
    } else {
      next[normalizedChannelId] = normalized;
    }
    narratorIdentityIdsByChannel.value = next;
    persistNarratorSettings();
    applyNarratorBadgeCleanup(normalizedChannelId, normalized);
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
      if (isNarratorIdentity(channelId, identityId)) return;
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

  const normalizeOnlineCardItem = (raw: any): OnlineCharacterCardItem | null => {
    const userId = String(raw?.userId || '').trim();
    const identityId = String(raw?.identityId || '').trim();
    const cardName = String(raw?.card?.name || '').trim();
    if (!userId || !identityId || !cardName) return null;
    return {
      userId,
      username: String(raw?.username || '').trim(),
      userNick: String(raw?.userNick || '').trim(),
      userColor: String(raw?.userColor || '').trim(),
      identityId,
      identityName: String(raw?.identityName || '').trim(),
      identityColor: String(raw?.identityColor || '').trim(),
      identityAvatar: String(raw?.identityAvatar || '').trim(),
      card: {
        name: cardName,
        sheetType: String(raw?.card?.sheetType || raw?.card?.sheet_type || '').trim(),
        attrs: raw?.card?.attrs && typeof raw.card.attrs === 'object' ? raw.card.attrs : {},
        templateText: String(raw?.card?.templateText || raw?.card?.template_text || '').trim(),
      },
      updatedAt: Number(raw?.updatedAt || 0),
    };
  };

  const setOnlineCardsLoading = (channelId: string, loading: boolean) => {
    if (!channelId) return;
    onlineCardsLoadingByChannel.value = {
      ...onlineCardsLoadingByChannel.value,
      [channelId]: loading,
    };
  };

  const syncOnlinePreviewWindows = (channelId: string, entry: OnlineCharacterCardItem) => {
    if (!channelId || !entry?.userId || !entry.identityId) return;
    const previewCardId = `online:${entry.userId}:${entry.identityId}`;
    sheetStore.activeWindowIds.forEach((windowId) => {
      const win = sheetStore.windows[windowId];
      if (!win || !win.readOnly || win.cardId !== previewCardId || win.channelId !== channelId) return;
      sheetStore.updateReadOnlyWindowData(windowId, {
        cardName: entry.card.name,
        sheetType: entry.card.sheetType,
        attrs: entry.card.attrs || {},
        avatarUrl: entry.identityAvatar || undefined,
        templateText: entry.card.templateText || '',
      });
    });
  };

  const upsertOnlineCardEntry = (channelId: string, entry: OnlineCharacterCardItem) => {
    if (!channelId || !entry.userId) return;
    onlineCardsByChannel.value = {
      ...onlineCardsByChannel.value,
      [channelId]: {
        ...(onlineCardsByChannel.value[channelId] || {}),
        [entry.userId]: entry,
      },
    };
  };

  const removeOnlineCardEntry = (channelId: string, userId: string) => {
    if (!channelId || !userId) return;
    const current = onlineCardsByChannel.value[channelId] || {};
    if (!current[userId]) return;
    const nextChannel = { ...current };
    delete nextChannel[userId];
    onlineCardsByChannel.value = {
      ...onlineCardsByChannel.value,
      [channelId]: nextChannel,
    };
  };

  const applyOnlineCardEvent = (event?: any) => {
    const channelId = String(event?.channel?.id || '').trim();
    const payload = event?.onlineCharacterCard;
    if (!channelId || !payload) return;
    const item = normalizeOnlineCardItem(payload.item);
    if (payload.action === 'clear') {
      removeOnlineCardEntry(channelId, item?.userId || String(payload?.item?.userId || '').trim());
      return;
    }
    if (item) {
      upsertOnlineCardEntry(channelId, item);
      syncOnlinePreviewWindows(channelId, item);
    }
  };

  const applyOnlineCardSnapshot = (event?: any) => {
    const channelId = String(event?.channel?.id || '').trim();
    if (!channelId) return;
    const items = Array.isArray(event?.onlineCharacterCardSnapshot?.items)
      ? event.onlineCharacterCardSnapshot.items
      : [];
    const next: Record<string, OnlineCharacterCardItem> = {};
    for (const raw of items) {
      const item = normalizeOnlineCardItem(raw);
      if (item) {
        next[item.userId] = item;
        syncOnlinePreviewWindows(channelId, item);
      }
    }
    onlineCardsByChannel.value = {
      ...onlineCardsByChannel.value,
      [channelId]: next,
    };
    setOnlineCardsLoading(channelId, false);
  };

  const broadcastOnlineActiveCard = async (channelId: string) => {
    if (!channelId) return;
    await snapshotStore.syncLocalSnapshot(channelId);
  };

  const clearOnlineActiveCard = async (channelId: string) => {
    if (!channelId) return;
    try {
      await snapshotStore.syncLocalSnapshot(channelId);
    } catch (e) {
      console.warn('[CharacterCard] Failed to clear online active card', e);
    }
  };

  const handleOnlineCardRequest = (event?: any) => {
    const channelId = String(event?.channel?.id || '').trim();
    const requesterId = String(event?.onlineCharacterCardRequest?.requesterId || '').trim();
    if (!channelId || requesterId === getUserId()) return;
    void broadcastOnlineActiveCard(channelId).catch((e) => {
      console.warn('[CharacterCard] Failed to broadcast online active card', e);
    });
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
    const updatedAt = Number(
      payload?.updatedAt
      || event?.characterCardBadge?.updatedAt
      || event?.timestamp
      || Math.floor(Date.now() / 1000),
    );
    const action = typeof payload?.action === 'string' ? payload.action : 'update';
    const channelId = typeof event?.channel?.id === 'string'
      ? event.channel.id
      : badgeByIdentity.value[identityId]?.channelId || '';
    if (channelId && isNarratorIdentity(channelId, identityId)) {
      removeBadgeEntry(identityId);
      removeBadgeCacheEntry(channelId, identityId);
      return;
    }
    if (action === 'clear') {
      const existing = channelId ? badgeCacheByChannel.value[channelId]?.[identityId] : null;
      if (existing && updatedAt < existing.updatedAt) {
        return;
      }
      removeBadgeEntry(identityId);
      if (channelId) {
        removeBadgeCacheEntry(channelId, identityId);
      }
      return;
    }
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
      if (isNarratorIdentity(channelId, identityId)) continue;
      if (item?.action === 'clear') continue;
      const updatedAt = Number(
        item?.updatedAt
        || event?.timestamp
        || Math.floor(Date.now() / 1000),
      );
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
    chatEvent.on('character-online-card-requested' as any, handleOnlineCardRequest);
    chatEvent.on('character-online-card-updated' as any, applyOnlineCardEvent);
    chatEvent.on('character-online-card-snapshot' as any, applyOnlineCardSnapshot);
    chatEvent.on('channel-identity-updated' as any, (payload?: { channelId?: string; removedId?: string; replacedId?: string }) => {
      const channelId = String(payload?.channelId || '').trim();
      const removedId = String(payload?.removedId || payload?.replacedId || '').trim();
      if (channelId) {
        snapshotStore.scheduleLocalSync(channelId);
      }
      if (!channelId || !removedId) {
        return;
      }
      removeBadgeEntry(removedId);
      removeBadgeCacheEntry(channelId, removedId);
    });
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
      if (isDebugEnabled()) {
        console.warn('[CharacterCard] loadCardList skipped: no userId');
      }
      return;
    }

    const resolvedChannelId = channelId || chatStore.curChannel?.id || '';
    if (shouldSkipCharacterApi(resolvedChannelId, 'loadCardList')) {
      return;
    }

    // Ensure WebSocket is connected before sending API request
    await chatStore.ensureConnectionReady();

    loading.value = true;
    try {
      if (isDebugEnabled()) {
        console.log('[CharacterCard] Sending character.list request for user:', userId);
      }
      const payload: Record<string, string> = { user_id: userId };
      if (resolvedChannelId) {
        payload.group_id = resolvedChannelId;
      }
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; list?: CharacterCardFromAPI[]; error?: string } }>('character.list', payload);
      maybeDisableFromResponse(resolvedChannelId, resp);
      if (isDebugEnabled()) {
        console.log('[CharacterCard] character.list response:', resp);
      }
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
    if (shouldSkipCharacterApi(channelId, 'getActiveCard')) {
      return null;
    }

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; data?: Record<string, any>; name?: string; type?: string; error?: string } }>('character.get', {
        group_id: channelId,
        user_id: userId,
      });
      maybeDisableFromResponse(channelId, resp);
      if (resp?.data?.ok) {
        await templateStore.ensureTemplatesLoaded({ worldId: chatStore.currentWorldId || undefined });
        await templateStore.ensureBindingsLoaded(channelId);
        const activeCardId = getActiveCardId(channelId);
        const resolvedTemplate = activeCardId
          ? templateStore.resolveCardTemplate(channelId, activeCardId, resp.data.type || '', '')
          : '';
        const rawAvatar = [
          (resp.data as any)?.avatarUrl,
          (resp.data as any)?.avatar_url,
          (resp.data as any)?.avatarAttachmentId,
          (resp.data as any)?.avatar_attachment_id,
          (resp.data as any)?.avatar,
        ].find(value => typeof value === 'string' && value.trim());
        const cardData: CharacterCardData = {
          name: resp.data.name || '',
          type: resp.data.type || '',
          attrs: resp.data.data || {},
          avatarUrl: typeof rawAvatar === 'string' ? rawAvatar.trim() : undefined,
          templateText: resolvedTemplate || undefined,
        };
        activeCards.value[channelId] = cardData;
        void broadcastActiveBadge(channelId);
        void broadcastOnlineActiveCard(channelId).catch((error) => {
          console.warn('[CharacterCard] Failed to update online active card', error);
        });
        return cardData;
      }
    } catch (e) {
      console.warn('Failed to get active card', e);
    }
    return null;
  };

  const revalidateCharacterApi = async (channelId: string): Promise<CharacterApiRevalidateResult> => {
    const normalizedChannelId = (channelId || '').trim();
    if (!normalizedChannelId) {
      return { ok: false as const, error: '缺少频道ID' };
    }
    const pending = revalidateCharacterApiInFlight.get(normalizedChannelId);
    if (pending) {
      return pending;
    }

    const task = (async (): Promise<CharacterApiRevalidateResult> => {
      await chatStore.ensureConnectionReady();
      const payload: Record<string, string> = {
        group_id: normalizedChannelId,
      };
      const userId = getUserId();
      if (userId) {
        payload.user_id = userId;
      }

      try {
        const resp = await chatStore.sendAPI<{ data?: { ok?: boolean; error?: string } }>('character.capability.test', payload);
        const ok = resp?.data?.ok === true;
        if (ok) {
          markCharacterApiHealthy(normalizedChannelId);
          clearBotCharacterDisabled(normalizedChannelId);
          chatStore.patchChannelAttributes(normalizedChannelId, {
            characterApiEnabled: true,
            characterApiReason: '',
          } as any);
          return { ok: true as const };
        }
        const err = String(resp?.data?.error || characterApiUnsupportedText).trim() || characterApiUnsupportedText;
        markBotCharacterDisabled(normalizedChannelId);
        chatStore.patchChannelAttributes(normalizedChannelId, {
          characterApiEnabled: false,
          characterApiReason: err,
        } as any);
        return { ok: false as const, error: err };
      } catch (e: any) {
        const err = String(
          e?.response?.data?.error
          || e?.response?.err
          || e?.message
          || '人物卡 API 验证失败',
        ).trim() || '人物卡 API 验证失败';
        markBotCharacterDisabled(normalizedChannelId);
        chatStore.patchChannelAttributes(normalizedChannelId, {
          characterApiEnabled: false,
          characterApiReason: err,
        } as any);
        return { ok: false as const, error: err };
      }
    })();
    revalidateCharacterApiInFlight.set(normalizedChannelId, task);
    try {
      return await task;
    } finally {
      revalidateCharacterApiInFlight.delete(normalizedChannelId);
    }
  };

  const ensureCharacterApiReadyForBotCommand = async (channelId: string) => {
    const normalizedChannelId = String(channelId || '').trim();
    if (!normalizedChannelId) {
      return { attempted: false as const };
    }
    if (!hasSuccessfulCharacterApiSession(normalizedChannelId)) {
      return { attempted: false as const };
    }
    if (isCharacterApiReady(normalizedChannelId)) {
      return { attempted: false as const };
    }
    const result = await revalidateCharacterApi(normalizedChannelId);
    return {
      attempted: true as const,
      ...result,
    };
  };

  // Create a new character card
  const createCard = async (channelId: string, name: string, sheetType: string = 'coc7', _attrs: Record<string, any> = {}) => {
    const userId = getUserId();
    if (!userId || !channelId) return null;
    assertCharacterApiEnabled(channelId, 'createCard');

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; id?: string; name?: string; sheet_type?: string; error?: string } }>('character.new', {
        user_id: userId,
        group_id: channelId,
        name,
        sheet_type: sheetType,
      });
      maybeDisableFromResponse(channelId, resp);
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
    assertCharacterApiEnabled(channelId, 'saveCard');

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; id?: string; name?: string; action?: string; error?: string } }>('character.save', {
        user_id: userId,
        group_id: channelId,
        name,
        sheet_type: sheetType,
      });
      maybeDisableFromResponse(channelId, resp);
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
    assertCharacterApiEnabled(channelId, 'updateCard');

    await chatStore.ensureConnectionReady();

    try {
      const resp = await chatStore.sendAPI<{ data: { ok: boolean; error?: string } }>('character.set', {
        group_id: channelId,
        user_id: userId,
        name,
        attrs,
      });
      maybeDisableFromResponse(channelId, resp);
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
    if (shouldSkipCharacterApi(channelId, 'tagCard')) {
      return null;
    }

    await chatStore.ensureConnectionReady();

    try {
      const payload: Record<string, string> = {
        user_id: userId,
        group_id: channelId,
      };
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; action?: string; id?: string; name?: string; error?: string } }>('character.tag', payload);
      maybeDisableFromResponse(channelId, resp);
      if (resp?.data?.ok) {
        await getActiveCard(channelId);
        await applySnapshotTemplatePresetForCard(channelId, getActiveCardId(channelId));
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
      if (shouldSkipCharacterApi(resolvedChannelId, 'untagAllCard')) {
        return null;
      }
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;
      if (resolvedChannelId) payload.group_id = resolvedChannelId;

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; unbound_count?: number; error?: string } }>('character.untagAll', payload);
      maybeDisableFromResponse(resolvedChannelId, resp);
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
    if (shouldSkipCharacterApi(channelId, 'loadCard')) {
      return null;
    }

    await chatStore.ensureConnectionReady();

    try {
      const payload: Record<string, string> = {
        user_id: userId,
        group_id: channelId,
      };
      if (cardName) payload.name = cardName;
      if (cardId) payload.id = cardId;

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; id?: string; name?: string; sheet_type?: string; error?: string } }>('character.load', payload);
      maybeDisableFromResponse(channelId, resp);
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
        assertCharacterApiEnabled(resolvedChannelId, 'deleteCard');
      }
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
      maybeDisableFromResponse(resolvedChannelId, untagResp);
      if (!untagResp?.data?.ok) {
        throw new Error(untagResp?.data?.error || '解绑失败');
      }

      const resp = await chatStore.sendAPI<{ data: { ok: boolean; error?: string; binding_groups?: string[] } }>('character.delete', payload);
      maybeDisableFromResponse(resolvedChannelId, resp);
      if (resp?.data?.ok) {
        const deletedCardId = String(payload.id || '').trim();
        const cleanup = cleanupDeletedCharacterCardState({
          channelId: resolvedChannelId,
          deletedCardId,
          activeCardId: resolvedChannelId ? getActiveCardId(resolvedChannelId) : '',
          identityBindings: identityBindings.value,
          badgeByIdentity: badgeByIdentity.value,
          badgeCacheByChannel: badgeCacheByChannel.value,
          activeCards: activeCards.value,
        });
        identityBindings.value = cleanup.identityBindings;
        badgeByIdentity.value = cleanup.badgeByIdentity;
        badgeCacheByChannel.value = cleanup.badgeCacheByChannel;
        activeCards.value = cleanup.activeCards;
        persistIdentityBindings();
        persistBadgeCache();
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

  const applySnapshotTemplatePresetForCard = async (channelId: string, cardId: string) => {
    const card = getCardById(cardId);
    const preset = getCharacterSnapshotTemplatePreset(
      card?.sheetType || activeCards.value[channelId]?.type || '',
    );
    if (!channelId || !preset) return;
    try {
      const badgeTemplate = CHARACTER_SNAPSHOT_BADGE_TEMPLATE_PRESETS[preset];
      const theaterOverlayTemplateJson = JSON.stringify(
        CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS[preset],
        null,
        2,
      );
      const current = snapshotStore.preferenceByChannel[channelId];
      if (
        current?.badgeTemplateMode === 'custom'
        && current.badgeTemplate === badgeTemplate
        && current.theaterOverlayTemplateMode === 'custom'
        && current.theaterOverlayTemplateJson === theaterOverlayTemplateJson
      ) {
        return;
      }
      await snapshotStore.updatePreference(channelId, {
        badgeTemplateMode: 'custom',
        badgeTemplate,
        theaterOverlayTemplateMode: 'custom',
        theaterOverlayTemplateJson,
      });
      await snapshotStore.syncLocalSnapshot(channelId, true);
    } catch (error) {
      console.warn('[CharacterCard] Failed to apply snapshot template preset', { channelId, cardId, error });
    }
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
  const getBoundCardId = (identityId: string, sharedIdentityId?: string) => {
    loadIdentityBindings();
    const sharedKey = sharedIdentityBindingKey(sharedIdentityId);
    return (sharedKey && identityBindings.value[sharedKey]) || identityBindings.value[identityId];
  };

  const getIdentityDisplayName = (channelId: string, identityId: string) => {
    if (!channelId || !identityId) {
      return '';
    }
    const identity = (chatStore.channelIdentities[channelId] || []).find(item => item.id === identityId);
    return String(identity?.displayName || '').trim();
  };

  const canSyncBotNickname = (channelId: string) => {
    if (!displayStore.settings.characterCardAutoSyncBotNickname) {
      return false;
    }
    if (!channelId || chatStore.isObserver || chatStore.observerMode || !!chatStore.observerWorldId) {
      return false;
    }
    const channel = chatStore.findChannelById(channelId) as any;
    return shouldEnableBotNicknameSyncForChannel(channel);
  };

  const dispatchBotNicknameSync = async (channelId: string, targetName: string, reason: string, force = false) => {
    if (!canSyncBotNickname(channelId)) {
      return false;
    }
    const channel = chatStore.findChannelById(channelId) as any;
    const command = buildBotNicknameSyncCommand(targetName, channel?.botCommandPrefixes);
    if (!command) {
      return false;
    }
    if (!force && lastBotNicknameSyncByChannel.value[channelId] === command) {
      return false;
    }
    try {
      await chatStore.botCommandDispatch(channelId, command, {
        silent: true,
        reason,
      });
      lastBotNicknameSyncByChannel.value = {
        ...lastBotNicknameSyncByChannel.value,
        [channelId]: command,
      };
      return true;
    } catch (e) {
      console.warn('[CharacterCard] Failed to sync bot nickname', { channelId, reason, command, error: e });
      return false;
    }
  };

  const syncBotNicknameForIdentity = async (
    channelId: string,
    identityId: string,
    options?: { reason?: string; explicitCardName?: string; force?: boolean },
  ) => {
    if (!channelId || !identityId) {
      return false;
    }
    loadIdentityBindings();
    let boundCardName = '';
    const boundCardId = identityBindings.value[getIdentityBindingKey(channelId, identityId)];
    if (boundCardId) {
      boundCardName = String(getCardById(boundCardId)?.name || '').trim();
      if (!boundCardName) {
        await loadCardList(channelId);
        boundCardName = String(getCardById(boundCardId)?.name || '').trim();
      }
    }
    const targetName = resolveBotNicknameSyncName({
      identityName: getIdentityDisplayName(channelId, identityId),
      boundCardName,
      explicitCardName: options?.explicitCardName,
    });
    return dispatchBotNicknameSync(channelId, targetName, options?.reason || 'identity-sync', !!options?.force);
  };

  const syncBotNicknameForCard = async (
    channelId: string,
    cardName: string,
    options?: { reason?: string; force?: boolean },
  ) => dispatchBotNicknameSync(channelId, cardName, options?.reason || 'card-switch', !!options?.force);

  const syncCardForIdentity = async (
    channelId: string,
    identityId: string,
    options: SyncCardForIdentityOptions = {},
  ): Promise<SyncCardForIdentityResult> => {
    if (!channelId || !identityId) {
      return {
        ok: false,
        switched: false,
        preserved: false,
      };
    }
    loadIdentityBindings();
    const boundCardId = identityBindings.value[getIdentityBindingKey(channelId, identityId)];
    const preserveWhenUnbound = options.preserveWhenUnbound !== false;
    const reloadAfterSwitch = options.reloadAfterSwitch !== false;
    const nicknameSyncReason = boundCardId ? 'identity-switch-bound' : 'identity-switch-unbound';

    void syncBotNicknameForIdentity(channelId, identityId, {
      reason: nicknameSyncReason,
    });

    if (!boundCardId) {
      if (preserveWhenUnbound) {
        return {
          ok: true,
          switched: false,
          preserved: true,
        };
      }
      const cleared = await tagCard(channelId);
      if (!cleared) {
        return {
          ok: false,
          switched: false,
          preserved: false,
        };
      }
      if (reloadAfterSwitch) {
        await loadCards(channelId);
      }
      return {
        ok: true,
        switched: true,
        preserved: false,
      };
    }

    const tagged = await tagCard(channelId, undefined, boundCardId);
    if (!tagged) {
      return {
        ok: false,
        switched: false,
        preserved: false,
      };
    }
    if (reloadAfterSwitch) {
      await loadCards(channelId);
    }
    return {
      ok: true,
      switched: true,
      preserved: false,
      boundCardId,
    };
  };

  // Backwards compatibility: bindIdentity persists mapping locally then syncs SealDice
  const bindIdentity = async (channelId: string, identityId: string, cardId: string, sharedIdentityId?: string) => {
    if (!channelId || !identityId || !cardId) return null;
    loadIdentityBindings();
    const bindingKey = getIdentityBindingKey(channelId, identityId, sharedIdentityId);
    identityBindings.value[bindingKey] = cardId;
    if (bindingKey !== identityId) {
      delete identityBindings.value[identityId];
    }
    persistIdentityBindings();
    if (chatStore.getActiveIdentityId(channelId) === identityId) {
      await syncCardForIdentity(channelId, identityId, { preserveWhenUnbound: false });
    }
    return { ok: true };
  };

  // Backwards compatibility: unbindIdentity persists mapping locally then syncs SealDice
  const unbindIdentity = async (channelId: string, identityId: string, sharedIdentityId?: string) => {
    if (!channelId || !identityId) return null;
    loadIdentityBindings();
    const bindingKey = getIdentityBindingKey(channelId, identityId, sharedIdentityId);
    delete identityBindings.value[bindingKey];
    if (bindingKey !== identityId) {
      delete identityBindings.value[identityId];
    }
    persistIdentityBindings();
    if (chatStore.getActiveIdentityId(channelId) === identityId) {
      await syncCardForIdentity(channelId, identityId, { preserveWhenUnbound: false });
    }
    return { ok: true };
  };

  const requestBadgeSnapshot = async (channelId: string) => {
    if (!channelId) return;
    if (shouldSkipCharacterApi(channelId, 'requestBadgeSnapshot')) {
      return;
    }
    try {
      await snapshotStore.initializeChannel(channelId);
      await snapshotStore.refreshChannel(channelId);
    } catch (e) {
      console.warn('Failed to request badge snapshot', e);
    }
  };

  const requestOnlineCardSnapshot = async (channelId: string, _options?: { requestPeers?: boolean }) => {
    if (!channelId) return;
    setOnlineCardsLoading(channelId, true);
    try {
      await snapshotStore.initializeChannel(channelId);
      await snapshotStore.refreshChannel(channelId);
    } catch (e) {
      setOnlineCardsLoading(channelId, false);
      console.warn('[CharacterCard] Failed to request online character cards', e);
    }
  };

  const broadcastActiveBadge = async (channelId: string, _identityId?: string, _action: 'update' | 'clear' = 'update') => {
    if (!channelId) return;
    if (shouldSkipCharacterApi(channelId, 'broadcastActiveBadge')) {
      return;
    }
    try {
      await snapshotStore.syncLocalSnapshot(channelId);
    } catch (e) {
      console.warn('Failed to broadcast badge', e);
    }
  };

  const getBadgeByIdentity = (channelId: string, identityId: string) => {
    if (!channelId || !identityId) return null;
    if (isNarratorIdentity(channelId, identityId)) return null;
    const snapshot = snapshotStore.getSnapshot(channelId, identityId);
    if (snapshot) {
      if (!snapshot.data.badgeEnabled) return null;
      const template = String(snapshot.badgeTemplate || '').trim();
      const attrs = snapshot.data.badgeAttrs || {};
      if (template && hasRenderableBadgeData(template, attrs)) {
        return {
          identityId,
          channelId,
          template,
          attrs,
          updatedAt: Math.floor((snapshot.sourceUpdatedAt || snapshot.lastSeenAt || Date.now()) / 1000),
        };
      }
    }
    return badgeCacheByChannel.value[channelId]?.[identityId] || null;
  };

  watch(
    () => userStore.info?.id,
    () => {
      loadedBindingsKey = '';
      loadedNarratorSettingsKey = '';
      loadIdentityBindings();
      ensureNarratorSettingsLoaded();
    },
    { immediate: true },
  );

  snapshotStore.setLocalSnapshotProvider(async (channelId) => {
    if (!displayStore.settings.characterCardSnapshotUploadEnabled) return null;
    try {
      await avatarStore.ensureBindingsLoaded(channelId);
    } catch (error) {
      console.warn('[CharacterCard] Failed to load card avatar bindings for snapshot', error);
    }
    const identity = chatStore.getActiveIdentity(channelId);
    if (!identity?.id) return null;
    const isOocIdentity = chatStore.getIdentityIcOocMode(channelId, identity.id) === 'ooc';
    const ownUserId = String(identity.userId || getUserId());
    const retainedIdentity = isOocIdentity
      ? snapshotStore.getChannelItems(channelId).find((item) => (
        item.userId === ownUserId && chatStore.getIdentityIcOocMode(channelId, item.identityId) !== 'ooc'
      ))
      : null;
    if (isOocIdentity && !retainedIdentity) return undefined;
    const snapshotIdentityId = retainedIdentity?.identityId || identity.id;
    const snapshotIdentity = retainedIdentity?.data.identity;
    const active = activeCards.value[channelId];
    const variant = chatStore.getActiveIdentityVariant(channelId, identity.id);
    const cardId = getActiveCardId(channelId);
    const cardMeta = cardId ? cardList.value.find(card => card.id === cardId) : undefined;
    const cardAvatarAttachmentId = cardId
      ? avatarStore.resolveCardAvatar(cardId, channelId, active?.avatarUrl)
      : '';
    const rawSourceUpdatedAt = Number(cardMeta?.updatedAt || 0);
    const sourceUpdatedAt = rawSourceUpdatedAt > 0 && rawSourceUpdatedAt < 1_000_000_000_000
      ? rawSourceUpdatedAt * 1000
      : rawSourceUpdatedAt;
    const displayName = String(snapshotIdentity?.displayName || variant?.displayName || identity.displayName || '').trim();
    const color = String(snapshotIdentity?.color || variant?.color || identity.color || '').trim();
    const avatarAttachmentId = String(snapshotIdentity?.avatarAttachmentId || variant?.avatarAttachmentId || identity.avatarAttachmentId || '').trim();
    const avatarDecorations = Array.isArray(snapshotIdentity?.avatarDecorations)
      ? snapshotIdentity.avatarDecorations
      : Array.isArray((variant as any)?.appearance?.avatarDecorations)
        ? (variant as any).appearance.avatarDecorations
        : Array.isArray(identity.avatarDecorations)
          ? identity.avatarDecorations
          : [];
    const includeCard = displayStore.settings.onlineCharacterCardsEnabled && !!active;
    const includeBadge = displayStore.settings.characterCardBadgeEnabled && !!active && !isNarratorIdentity(channelId, snapshotIdentityId);
    return {
      identityId: snapshotIdentityId,
      sourceType: 'client',
      sourceCardId: cardId,
      sourceUpdatedAt,
      data: {
        identity: {
          id: snapshotIdentityId,
          userId: ownUserId,
          displayName,
          color,
          avatarAttachmentId,
          avatarDecorations,
        },
        ...(includeCard && active ? {
          card: {
            name: active.name,
            sheetType: active.type,
            avatarAttachmentId: cardAvatarAttachmentId,
            attrs: active.attrs || {},
            ...(active.templateText ? { templateText: active.templateText } : {}),
          },
        } : {}),
        badgeEnabled: includeBadge,
        badgeAttrs: includeBadge ? active?.attrs || {} : {},
      },
    };
  });
  ensureBadgeGateway();

  return {
    cardList,
    cards,
    activeCards,
    badgeByIdentity,
    onlineCardsByChannel,
    onlineCardsLoadingByChannel,
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
    getNarratorIdentityIds,
    setNarratorIdentityIds,
    isNarratorIdentity,
    getBoundCardId,
    syncCardForIdentity,
    syncBotNicknameForIdentity,
    syncBotNicknameForCard,
    bindIdentity,
    unbindIdentity,
    requestBadgeSnapshot,
    broadcastActiveBadge,
    requestOnlineCardSnapshot,
    broadcastOnlineActiveCard,
    clearOnlineActiveCard,
    revalidateCharacterApi,
    markCharacterApiHealthy,
    hasSuccessfulCharacterApiSession,
    isCharacterApiReady,
    ensureCharacterApiReadyForBotCommand,
    isBotCharacterDisabled,
    getCharacterApiDisabledReason,
  };
});
