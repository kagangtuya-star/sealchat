import { computed, ref } from 'vue';
import { defineStore } from 'pinia';
import { chatEvent, useChatStore } from './chat';
import { useUserStore } from './user';

export type CharacterSnapshotTemplateMode = 'inherit' | 'custom' | 'off';

export type CharacterSnapshotNumericSource = { path: string } | { value: number };

export interface TheaterCharacterStatTemplate {
  id: string;
  name: string;
  current: CharacterSnapshotNumericSource;
  max?: CharacterSnapshotNumericSource;
  min?: CharacterSnapshotNumericSource;
  barColor?: string;
  textColor?: string;
}

export interface TheaterCharacterOverlayTemplate {
  version: 1;
  preferredColumns?: number;
  items: TheaterCharacterStatTemplate[];
}

export interface ChannelCharacterSnapshotIdentity {
  id: string;
  userId: string;
  displayName?: string;
  color?: string;
  avatarAttachmentId?: string;
  avatarDecorations?: unknown[];
}

export interface ChannelCharacterSnapshotCard {
  name: string;
  sheetType: string;
  avatarAttachmentId?: string;
  attrs: Record<string, any>;
  templateText?: string;
}

export interface ChannelCharacterSnapshotItem {
  channelId: string;
  identityId: string;
  userId: string;
  sourceType: string;
  sourceCardId?: string;
  data: {
    identity: ChannelCharacterSnapshotIdentity;
    card?: ChannelCharacterSnapshotCard;
    badgeEnabled?: boolean;
    badgeAttrs?: Record<string, any>;
  };
  badgeTemplate?: string;
  theaterOverlayTemplateJson?: string;
  contentHash: string;
  serverRevision: number;
  sourceUpdatedAt?: number;
  lastSeenAt: number;
}

export interface ChannelCharacterSnapshotSettings {
  channelId: string;
  badgeTemplate: string;
  theaterOverlayTemplateJson: string;
  schemaVersion: number;
  serverRevision: number;
  updatedBy?: string;
}

export interface ChannelCharacterSnapshotPreference {
  channelId: string;
  userId: string;
  badgeTemplateMode: CharacterSnapshotTemplateMode;
  badgeTemplate: string;
  theaterOverlayTemplateMode: CharacterSnapshotTemplateMode;
  theaterOverlayTemplateJson: string;
  schemaVersion: number;
  serverRevision: number;
}

export interface LocalCharacterSnapshotInput {
  identityId: string;
  sourceType?: 'client' | 'sealdice' | 'sealchat';
  sourceCardId?: string;
  sourceUpdatedAt?: number;
  data: ChannelCharacterSnapshotItem['data'];
}

type LocalSnapshotProvider = (channelId: string) => LocalCharacterSnapshotInput | null | Promise<LocalCharacterSnapshotInput | null>;

const defaultOverlayTemplate: TheaterCharacterOverlayTemplate = {
  version: 1,
  preferredColumns: 2,
  items: [],
};

const normalizeItem = (value: any): ChannelCharacterSnapshotItem | null => {
  const channelId = String(value?.channelId || '').trim();
  const identityId = String(value?.identityId || value?.data?.identity?.id || '').trim();
  const userId = String(value?.userId || value?.data?.identity?.userId || '').trim();
  if (!channelId || !identityId || !userId || !value?.data?.identity) return null;
  const card = value.data.card && typeof value.data.card === 'object'
    ? {
      name: String(value.data.card.name || ''),
      sheetType: String(value.data.card.sheetType || ''),
      avatarAttachmentId: String(value.data.card.avatarAttachmentId || ''),
      attrs: value.data.card.attrs && typeof value.data.card.attrs === 'object' ? value.data.card.attrs : {},
      ...(value.data.card.templateText ? { templateText: String(value.data.card.templateText) } : {}),
    }
    : undefined;
  return {
    channelId,
    identityId,
    userId,
    sourceType: String(value.sourceType || 'client'),
    sourceCardId: String(value.sourceCardId || ''),
    data: {
      identity: {
        id: identityId,
        userId,
        displayName: String(value.data.identity.displayName || ''),
        color: String(value.data.identity.color || ''),
        avatarAttachmentId: String(value.data.identity.avatarAttachmentId || ''),
        avatarDecorations: Array.isArray(value.data.identity.avatarDecorations) ? value.data.identity.avatarDecorations : [],
      },
      ...(card ? { card } : {}),
      badgeEnabled: value.data.badgeEnabled === true,
      badgeAttrs: value.data.badgeAttrs && typeof value.data.badgeAttrs === 'object' ? value.data.badgeAttrs : {},
    },
    badgeTemplate: String(value.badgeTemplate || ''),
    theaterOverlayTemplateJson: String(value.theaterOverlayTemplateJson || ''),
    contentHash: String(value.contentHash || ''),
    serverRevision: Number(value.serverRevision || 0),
    sourceUpdatedAt: Number(value.sourceUpdatedAt || 0),
    lastSeenAt: Number(value.lastSeenAt || 0),
  };
};

const parseOverlayTemplate = (raw?: string): TheaterCharacterOverlayTemplate => {
  try {
    const value = JSON.parse(String(raw || ''));
    if (value?.version !== 1 || !Array.isArray(value.items)) return defaultOverlayTemplate;
    return {
      version: 1,
      preferredColumns: Math.min(4, Math.max(1, Number(value.preferredColumns || 2))),
      items: value.items,
    };
  } catch {
    return defaultOverlayTemplate;
  }
};

const stableValue = (value: any): any => {
  if (Array.isArray(value)) return value.map(stableValue);
  if (!value || typeof value !== 'object') return value;
  return Object.keys(value).sort().reduce<Record<string, any>>((result, key) => {
    if (value[key] !== undefined) result[key] = stableValue(value[key]);
    return result;
  }, {});
};

const localSnapshotSignature = (input: LocalCharacterSnapshotInput) => JSON.stringify(stableValue(input));

export const useChannelCharacterSnapshotStore = defineStore('channelCharacterSnapshot', () => {
  const chatStore = useChatStore();
  const userStore = useUserStore();
  const snapshotsByChannel = ref<Record<string, Record<string, ChannelCharacterSnapshotItem>>>({});
  const settingsByChannel = ref<Record<string, ChannelCharacterSnapshotSettings>>({});
  const preferenceByChannel = ref<Record<string, ChannelCharacterSnapshotPreference>>({});
  const loadingByChannel = ref<Record<string, boolean>>({});
  const initializationByChannel = new Map<string, Promise<void>>();
  const syncTimers = new Map<string, ReturnType<typeof setTimeout>>();
  const lastLocalSignature = new Map<string, string>();
  const lastServerHash = new Map<string, string>();
  const lastSyncedIdentityId = new Map<string, string>();
  let localSnapshotProvider: LocalSnapshotProvider | null = null;
  let gatewayBound = false;

  const setLocalSnapshotProvider = (provider: LocalSnapshotProvider) => {
    localSnapshotProvider = provider;
  };

  const replaceChannelItems = (channelId: string, values: any[]) => {
    const next: Record<string, ChannelCharacterSnapshotItem> = {};
    values.forEach((value) => {
      const item = normalizeItem(value);
      if (item && item.channelId === channelId) next[item.identityId] = item;
    });
    snapshotsByChannel.value = { ...snapshotsByChannel.value, [channelId]: next };
    const ownItem = Object.values(next).find(item => item.userId === String(userStore.info?.id || ''));
    if (ownItem) {
      lastServerHash.set(channelId, ownItem.contentHash);
      lastSyncedIdentityId.set(channelId, ownItem.identityId);
    } else {
      lastLocalSignature.delete(channelId);
      lastServerHash.delete(channelId);
      lastSyncedIdentityId.delete(channelId);
    }
  };

  const applySnapshotUpdate = (event: any) => {
    const channelId = String(event?.channel?.id || event?.characterSnapshot?.item?.channelId || '').trim();
    const payload = event?.characterSnapshot;
    const identityId = String(payload?.item?.identityId || '').trim();
    if (!channelId || !identityId) return;
    const current = { ...(snapshotsByChannel.value[channelId] || {}) };
    if (payload.action === 'clear') {
      delete current[identityId];
      if (String(payload?.item?.userId || '') === String(userStore.info?.id || '')) {
        lastLocalSignature.delete(channelId);
        lastServerHash.delete(channelId);
        lastSyncedIdentityId.delete(channelId);
      }
    } else {
      const item = normalizeItem(payload.item);
      if (!item) return;
      const existing = current[identityId];
      if (existing && existing.serverRevision > item.serverRevision) return;
      current[identityId] = item;
      Object.keys(current).forEach((otherIdentityId) => {
        if (otherIdentityId !== identityId && current[otherIdentityId]?.userId === item.userId) {
          delete current[otherIdentityId];
        }
      });
    }
    snapshotsByChannel.value = { ...snapshotsByChannel.value, [channelId]: current };
  };

  const applySnapshotList = (event: any) => {
    const payload = event?.characterSnapshotList;
    const channelId = String(payload?.channelId || event?.channel?.id || '').trim();
    if (!channelId) return;
    replaceChannelItems(channelId, Array.isArray(payload?.items) ? payload.items : []);
    loadingByChannel.value = { ...loadingByChannel.value, [channelId]: false };
  };

  const applySettings = (event: any) => {
    const value = event?.characterSnapshotSettings;
    const channelId = String(value?.channelId || '').trim();
    if (!channelId) return;
    settingsByChannel.value = { ...settingsByChannel.value, [channelId]: value };
    void refreshChannel(channelId);
  };

  const applyPreference = (event: any) => {
    const value = event?.characterSnapshotPreference;
    const channelId = String(value?.channelId || '').trim();
    if (!channelId) return;
    void refreshChannel(channelId);
    if (String(value?.userId || '') !== String(userStore.info?.id || '')) return;
    preferenceByChannel.value = { ...preferenceByChannel.value, [channelId]: value };
  };

  const bindGateway = () => {
    if (gatewayBound) return;
    chatEvent.on('character-snapshot-updated' as any, applySnapshotUpdate);
    chatEvent.on('character-snapshot-list' as any, applySnapshotList);
    chatEvent.on('character-snapshot-settings-updated' as any, applySettings);
    chatEvent.on('character-snapshot-preference-updated' as any, applyPreference);
    chatEvent.on('character-snapshot-probe' as any, (event: any) => {
      const channelId = String(event?.characterSnapshotProbe?.channelId || event?.channel?.id || '').trim();
      if (!channelId || !localSnapshotProvider) return;
      const ownUserId = String(userStore.info?.id || '');
      const ownProbe = (Array.isArray(event?.characterSnapshotProbe?.items) ? event.characterSnapshotProbe.items : [])
        .find((item: any) => String(item?.userId || '') === ownUserId);
      if (!ownProbe || String(ownProbe.contentHash || '') !== String(lastServerHash.get(channelId) || '')) {
        lastLocalSignature.delete(channelId);
      }
      void syncLocalSnapshot(channelId, false);
    });
    chatEvent.on('channel-switch-to' as any, (event: any) => {
      const channelId = String(event?.channelId || event?.channel?.id || event?.argv?.channelId || chatStore.curChannel?.id || '').trim();
      if (!channelId) return;
      void initializeChannel(channelId).then(() => scheduleLocalSync(channelId, 0));
    });
    chatEvent.on('connected', () => {
      const channelId = String(chatStore.curChannel?.id || '').trim();
      if (!channelId) return;
      initializationByChannel.delete(channelId);
      void initializeChannel(channelId).then(() => scheduleLocalSync(channelId, 0));
    });
    gatewayBound = true;
  };

  const refreshChannel = async (channelId: string) => {
    channelId = String(channelId || '').trim();
    if (!channelId) return;
    loadingByChannel.value = { ...loadingByChannel.value, [channelId]: true };
    await chatStore.ensureConnectionReady();
    try {
      const response = await chatStore.sendAPI<{ data?: { channelId?: string; items?: any[] } }>('character.snapshot.list', { channelId });
      replaceChannelItems(channelId, Array.isArray(response?.data?.items) ? response.data.items : []);
    } finally {
      loadingByChannel.value = { ...loadingByChannel.value, [channelId]: false };
    }
  };

  const loadSettings = async (channelId: string) => {
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotSettings }>('character.snapshot.settings.get', { channelId });
    if (response?.data?.channelId) {
      settingsByChannel.value = { ...settingsByChannel.value, [channelId]: response.data };
    }
  };

  const loadPreference = async (channelId: string) => {
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotPreference }>('character.snapshot.preference.get', { channelId });
    if (response?.data?.channelId) {
      preferenceByChannel.value = { ...preferenceByChannel.value, [channelId]: response.data };
    }
  };

  const initializeChannel = async (channelId: string) => {
    channelId = String(channelId || '').trim();
    if (!channelId) return;
    const current = initializationByChannel.get(channelId);
    if (current) return current;
    const initialization = (async () => {
      try {
        await chatStore.ensureConnectionReady();
        await Promise.all([refreshChannel(channelId), loadSettings(channelId), loadPreference(channelId)]);
      } catch (error) {
        initializationByChannel.delete(channelId);
        console.warn('[CharacterSnapshot] initialize failed', error);
      }
    })();
    initializationByChannel.set(channelId, initialization);
    return initialization;
  };

  const syncLocalSnapshot = async (channelId: string, force = false) => {
    channelId = String(channelId || '').trim();
    if (!channelId || !localSnapshotProvider) return null;
    const input = await localSnapshotProvider(channelId);
    if (!input?.identityId) {
      const ownUserId = String(userStore.info?.id || '');
      const previousIdentityId = lastSyncedIdentityId.get(channelId)
        || getChannelItems(channelId).find(item => item.userId === ownUserId)?.identityId
        || '';
      if (!previousIdentityId) return null;
      await chatStore.ensureConnectionReady();
      await chatStore.sendAPI('character.snapshot.clear', { channelId, identityId: previousIdentityId });
      const current = { ...(snapshotsByChannel.value[channelId] || {}) };
      delete current[previousIdentityId];
      snapshotsByChannel.value = { ...snapshotsByChannel.value, [channelId]: current };
      lastLocalSignature.delete(channelId);
      lastServerHash.delete(channelId);
      lastSyncedIdentityId.delete(channelId);
      return null;
    }
    const signature = localSnapshotSignature(input);
    if (!force && lastLocalSignature.get(channelId) === signature) return null;
    await chatStore.ensureConnectionReady();
    const response = await chatStore.sendAPI<{ data?: { item?: any; changed?: boolean } }>('character.snapshot.upsert', {
      channelId,
      identityId: input.identityId,
      sourceType: input.sourceType || 'client',
      sourceCardId: input.sourceCardId || '',
      sourceUpdatedAt: input.sourceUpdatedAt || Date.now(),
      data: input.data,
    });
    const item = normalizeItem(response?.data?.item);
    if (item) {
      const current = { ...(snapshotsByChannel.value[channelId] || {}), [item.identityId]: item };
      Object.keys(current).forEach((identityId) => {
        if (current[identityId]?.userId === item.userId && identityId !== item.identityId) delete current[identityId];
      });
      snapshotsByChannel.value = { ...snapshotsByChannel.value, [channelId]: current };
      lastLocalSignature.set(channelId, signature);
      lastServerHash.set(channelId, item.contentHash);
      lastSyncedIdentityId.set(channelId, item.identityId);
    }
    return item;
  };

  const scheduleLocalSync = (channelId: string, delay = 350) => {
    channelId = String(channelId || '').trim();
    if (!channelId) return;
    const current = syncTimers.get(channelId);
    if (current) clearTimeout(current);
    syncTimers.set(channelId, setTimeout(() => {
      syncTimers.delete(channelId);
      void syncLocalSnapshot(channelId).catch((error) => console.warn('[CharacterSnapshot] sync failed', error));
    }, Math.max(0, delay)));
  };

  const updateSettings = async (channelId: string, update: Pick<ChannelCharacterSnapshotSettings, 'badgeTemplate' | 'theaterOverlayTemplateJson'>) => {
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotSettings }>('character.snapshot.settings.update', { channelId, ...update });
    if (response?.data?.channelId) settingsByChannel.value = { ...settingsByChannel.value, [channelId]: response.data };
    return response?.data;
  };

  const updatePreference = async (channelId: string, update: Pick<ChannelCharacterSnapshotPreference, 'badgeTemplateMode' | 'badgeTemplate' | 'theaterOverlayTemplateMode' | 'theaterOverlayTemplateJson'>) => {
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotPreference }>('character.snapshot.preference.update', { channelId, ...update });
    if (response?.data?.channelId) preferenceByChannel.value = { ...preferenceByChannel.value, [channelId]: response.data };
    return response?.data;
  };

  const getChannelItems = (channelId: string) => Object.values(snapshotsByChannel.value[channelId] || {});
  const getSnapshot = (channelId: string, identityId: string) => snapshotsByChannel.value[channelId]?.[identityId] || null;

  const getEffectiveBadgeTemplate = (channelId: string) => {
    const preference = preferenceByChannel.value[channelId];
    if (preference?.badgeTemplateMode === 'off') return '';
    if (preference?.badgeTemplateMode === 'custom') return String(preference.badgeTemplate || '').trim();
    return String(settingsByChannel.value[channelId]?.badgeTemplate || '').trim();
  };

  const getEffectiveOverlayTemplate = (channelId: string) => {
    const preference = preferenceByChannel.value[channelId];
    if (preference?.theaterOverlayTemplateMode === 'off') return null;
    const raw = preference?.theaterOverlayTemplateMode === 'custom'
      ? preference.theaterOverlayTemplateJson
      : settingsByChannel.value[channelId]?.theaterOverlayTemplateJson;
    return parseOverlayTemplate(raw);
  };

  const getOverlayTemplateForSnapshot = (item: ChannelCharacterSnapshotItem) => {
    if (!String(item.theaterOverlayTemplateJson || '').trim()) return null;
    return parseOverlayTemplate(item.theaterOverlayTemplateJson);
  };

  const currentChannelItems = computed(() => getChannelItems(String(chatStore.curChannel?.id || '')));

  bindGateway();

  return {
    snapshotsByChannel,
    settingsByChannel,
    preferenceByChannel,
    loadingByChannel,
    currentChannelItems,
    initializeChannel,
    refreshChannel,
    syncLocalSnapshot,
    scheduleLocalSync,
    setLocalSnapshotProvider,
    updateSettings,
    updatePreference,
    getChannelItems,
    getSnapshot,
    getEffectiveBadgeTemplate,
    getEffectiveOverlayTemplate,
    getOverlayTemplateForSnapshot,
  };
});
