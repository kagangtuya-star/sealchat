import { computed, ref, watch } from 'vue';
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
  display?: {
    mode?: 'bar' | 'icon';
    icon?: {
      type: 'text' | 'image';
      value: string;
    };
    valuePerIcon?: number;
    subdivisions?: number;
  };
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
  platformTemplateRef?: string;
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
  badgeTemplateDisabled?: boolean;
  theaterOverlayTemplateJson?: string;
  contentHash: string;
  serverRevision: number;
  sourceUpdatedAt?: number;
  lastSeenAt: number;
}

export interface ChannelCharacterSnapshotSettings {
  channelId: string;
  badgeTemplate: string;
  theaterOverlayTemplateMode: CharacterSnapshotTemplateMode;
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

// `undefined` preserves the published snapshot without writing or clearing it.
type LocalSnapshotProvider = (channelId: string) => LocalCharacterSnapshotInput | null | undefined | Promise<LocalCharacterSnapshotInput | null | undefined>;

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
      ...(value.data.card.platformTemplateRef ? { platformTemplateRef: String(value.data.card.platformTemplateRef) } : {}),
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
    badgeTemplateDisabled: value.badgeTemplateDisabled === true,
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
  const clearedRevisions = new Map<string, Map<string, number>>();
  const channelMutations = new Map<string, number>();
  const userMutations = new Map<string, Map<string, number>>();
  const listRequests = new Map<string, number>();
  const syncRequests = new Map<string, { generation: number; force: boolean }>();
  const syncInFlight = new Map<string, Promise<ChannelCharacterSnapshotItem | null>>();
  let session = 0;
  let localSnapshotProvider: LocalSnapshotProvider | null = null;
  let gatewayBound = false;

  const setLocalSnapshotProvider = (provider: LocalSnapshotProvider) => {
    localSnapshotProvider = provider;
  };

  const markMutation = (channelId: string, userId?: string) => {
    const version = (channelMutations.get(channelId) || 0) + 1;
    channelMutations.set(channelId, version);
    if (userId) {
      const users = userMutations.get(channelId) || new Map<string, number>();
      users.set(userId, version);
      userMutations.set(channelId, users);
    }
  };
  const rememberClear = (channelId: string, identityId: string, revision: number) => {
    const revisions = clearedRevisions.get(channelId) || new Map<string, number>();
    revisions.set(identityId, Math.max(revisions.get(identityId) || 0, revision));
    clearedRevisions.set(channelId, revisions);
  };
  const resetAcknowledgement = (channelId: string) => {
    lastLocalSignature.delete(channelId);
    lastServerHash.delete(channelId);
    lastSyncedIdentityId.delete(channelId);
  };

  const replaceChannelItems = (channelId: string, values: any[], sinceMutation: number) => {
    const next: Record<string, ChannelCharacterSnapshotItem> = {};
    const current = snapshotsByChannel.value[channelId] || {};
    const protectedUsers = new Set<string>();
    userMutations.get(channelId)?.forEach((version, userId) => {
      if (version > sinceMutation) protectedUsers.add(userId);
    });
    values.forEach((value) => {
      const item = normalizeItem(value);
      if (!item || item.channelId !== channelId || protectedUsers.has(item.userId)) return;
      if ((clearedRevisions.get(channelId)?.get(item.identityId) ?? -1) >= item.serverRevision) return;
      const existing = current[item.identityId];
      next[item.identityId] = existing?.serverRevision > item.serverRevision ? existing : item;
    });
    Object.values(current).forEach(item => {
      if (protectedUsers.has(item.userId)) next[item.identityId] = item;
      if (!next[item.identityId]) rememberClear(channelId, item.identityId, item.serverRevision);
    });
    snapshotsByChannel.value = { ...snapshotsByChannel.value, [channelId]: next };
    const ownItem = Object.values(next).find(item => item.userId === String(userStore.info?.id || ''));
    if (ownItem) {
      lastSyncedIdentityId.set(channelId, ownItem.identityId);
    } else {
      resetAcknowledgement(channelId);
    }
    markMutation(channelId);
  };

  const applyItem = (item: ChannelCharacterSnapshotItem, clear = false) => {
    const { channelId, identityId } = item;
    const current = { ...(snapshotsByChannel.value[channelId] || {}) };
    const existing = current[identityId];
    const cleared = clearedRevisions.get(channelId)?.get(identityId) ?? -1;
    if ((existing && existing.serverRevision > item.serverRevision) || cleared >= item.serverRevision) return false;
    if (clear) {
      rememberClear(channelId, identityId, item.serverRevision);
      delete current[identityId];
      if (item.userId === String(userStore.info?.id || '') && lastSyncedIdentityId.get(channelId) === identityId) {
        resetAcknowledgement(channelId);
      }
    } else {
      current[identityId] = item;
      Object.keys(current).forEach((otherIdentityId) => {
        if (otherIdentityId !== identityId && current[otherIdentityId]?.userId === item.userId) {
          rememberClear(channelId, otherIdentityId, current[otherIdentityId].serverRevision);
          delete current[otherIdentityId];
        }
      });
      if (item.userId === String(userStore.info?.id || '')) lastSyncedIdentityId.set(channelId, identityId);
    }
    snapshotsByChannel.value = { ...snapshotsByChannel.value, [channelId]: current };
    markMutation(channelId, item.userId);
    return true;
  };

  const applySnapshotUpdate = (event: any) => {
    const item = normalizeItem(event?.characterSnapshot?.item);
    if (!item || (event?.channel?.id && event.channel.id !== item.channelId)) return;
    applyItem(item, event.characterSnapshot.action === 'clear');
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
    if (String(value?.userId || '') !== String(userStore.info?.id || '')) {
      void refreshChannel(channelId);
      return;
    }
    preferenceByChannel.value = { ...preferenceByChannel.value, [channelId]: value };
    void refreshChannel(channelId);
  };

  const bindGateway = () => {
    if (gatewayBound) return;
    chatEvent.on('character-snapshot-updated' as any, applySnapshotUpdate);
    // List APIs also emit an uncorrelated event. Only their request-scoped response is applied.
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
      void syncLocalSnapshot(channelId, false).catch(error => console.warn('[CharacterSnapshot] probe sync failed', error));
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
    const requestSession = session;
    const request = (listRequests.get(channelId) || 0) + 1;
    listRequests.set(channelId, request);
    const isCurrent = () => requestSession === session && listRequests.get(channelId) === request;
    loadingByChannel.value = { ...loadingByChannel.value, [channelId]: true };
    try {
      await chatStore.ensureConnectionReady();
      if (!isCurrent()) return;
      const mutation = channelMutations.get(channelId) || 0;
      const response = await chatStore.sendAPI<{ data?: { channelId?: string; items?: any[] } }>('character.snapshot.list', { channelId });
      if (!isCurrent()) return;
      if (response?.data?.channelId !== channelId || !Array.isArray(response.data.items)) return;
      // Preserve every user's update/clear since this request, including active-identity switches.
      replaceChannelItems(channelId, response.data.items, mutation);
    } finally {
      if (isCurrent()) loadingByChannel.value = { ...loadingByChannel.value, [channelId]: false };
    }
  };

  const loadSettings = async (channelId: string) => {
    const requestSession = session;
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotSettings }>('character.snapshot.settings.get', { channelId });
    if (requestSession === session && response?.data?.channelId === channelId) {
      settingsByChannel.value = { ...settingsByChannel.value, [channelId]: response.data };
    }
  };

  const loadPreference = async (channelId: string) => {
    const requestSession = session;
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotPreference }>('character.snapshot.preference.get', { channelId });
    if (requestSession === session && response?.data?.channelId === channelId) {
      preferenceByChannel.value = { ...preferenceByChannel.value, [channelId]: response.data };
    }
  };

  const initializeChannel = async (channelId: string) => {
    channelId = String(channelId || '').trim();
    if (!channelId) return;
    const current = initializationByChannel.get(channelId);
    if (current) return current;
    const requestSession = session;
    const initialization = (async () => {
      try {
        await chatStore.ensureConnectionReady();
        if (requestSession !== session) return;
        await Promise.all([refreshChannel(channelId), loadSettings(channelId), loadPreference(channelId)]);
      } catch (error) {
        if (requestSession === session) initializationByChannel.delete(channelId);
        console.warn('[CharacterSnapshot] initialize failed', error);
      }
    })();
    initializationByChannel.set(channelId, initialization);
    return initialization;
  };

  const writeLocalSnapshot = async (channelId: string, force: boolean, generation: number, requestSession: number) => {
    const isCurrent = () => requestSession === session && syncRequests.get(channelId)?.generation === generation;
    await chatStore.ensureConnectionReady();
    if (!isCurrent() || !localSnapshotProvider) return null;
    const input = await localSnapshotProvider(channelId);
    if (!isCurrent()) return null;
    if (input === undefined) return null;
    if (!input?.identityId) {
      const ownUserId = String(userStore.info?.id || '');
      const previousIdentityId = lastSyncedIdentityId.get(channelId)
        || getChannelItems(channelId).find(item => item.userId === ownUserId)?.identityId
        || '';
      if (!previousIdentityId) return null;
      const mutation = channelMutations.get(channelId) || 0;
      const response = await chatStore.sendAPI<{ data?: { item?: any } }>('character.snapshot.clear', { channelId, identityId: previousIdentityId });
      if (requestSession !== session) return null;
      const item = normalizeItem(response?.data?.item);
      if (item && (item.channelId !== channelId || item.identityId !== previousIdentityId)) return null;
      if (item) applyItem(item, true);
      else if (mutation === (channelMutations.get(channelId) || 0)) {
        const previous = snapshotsByChannel.value[channelId]?.[previousIdentityId];
        if (previous) applyItem(previous, true);
        resetAcknowledgement(channelId);
      }
      return null;
    }
    const signature = localSnapshotSignature(input);
    const ownItem = snapshotsByChannel.value[channelId]?.[input.identityId];
    if (!force && lastLocalSignature.get(channelId) === signature
      && ownItem?.contentHash === lastServerHash.get(channelId)) return null;
    const response = await chatStore.sendAPI<{ data?: { item?: any; changed?: boolean } }>('character.snapshot.upsert', {
      channelId,
      identityId: input.identityId,
      sourceType: input.sourceType || 'client',
      sourceCardId: input.sourceCardId || '',
      sourceUpdatedAt: input.sourceUpdatedAt || Date.now(),
      cardAction: input.data.card ? 'replace' : 'clear',
      data: input.data,
    });
    if (requestSession !== session) return null;
    const item = normalizeItem(response?.data?.item);
    if (item && item.channelId === channelId && item.identityId === input.identityId && applyItem(item)) {
      lastLocalSignature.set(channelId, signature);
      lastServerHash.set(channelId, item.contentHash);
      lastSyncedIdentityId.set(channelId, item.identityId);
    }
    return item;
  };

  const syncLocalSnapshot = (channelId: string, force = false): Promise<ChannelCharacterSnapshotItem | null> => {
    channelId = String(channelId || '').trim();
    if (!channelId || !localSnapshotProvider) return Promise.resolve(null);
    const previous = syncRequests.get(channelId);
    syncRequests.set(channelId, { generation: (previous?.generation || 0) + 1, force: force || previous?.force || false });
    const current = syncInFlight.get(channelId);
    if (current) return current;
    const requestSession = session;
    let task: Promise<ChannelCharacterSnapshotItem | null>;
    const finish = () => {
      if (syncInFlight.get(channelId) === task) {
        syncInFlight.delete(channelId);
        syncRequests.delete(channelId);
      }
    };
    task = (async () => {
      let item: ChannelCharacterSnapshotItem | null = null;
      while (requestSession === session) {
        const request = syncRequests.get(channelId)!;
        try {
          item = await writeLocalSnapshot(channelId, request.force, request.generation, requestSession);
        } catch (error) {
          // A failed older write must not swallow an already queued clear or newer publication.
          if (requestSession !== session || syncRequests.get(channelId)?.generation === request.generation) {
            finish();
            throw error;
          }
        }
        if (requestSession !== session || syncRequests.get(channelId)?.generation === request.generation) {
          // Release synchronously: a request queued before promise settlement must start a new drain.
          finish();
          break;
        }
      }
      return item;
    })().finally(finish);
    syncInFlight.set(channelId, task);
    return task;
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

  const updateSettings = async (channelId: string, update: Pick<ChannelCharacterSnapshotSettings, 'badgeTemplate' | 'theaterOverlayTemplateMode' | 'theaterOverlayTemplateJson'>) => {
    const requestSession = session;
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotSettings }>('character.snapshot.settings.update', { channelId, ...update });
    if (requestSession === session && response?.data?.channelId === channelId) settingsByChannel.value = { ...settingsByChannel.value, [channelId]: response.data };
    return response?.data;
  };

  const updatePreference = async (channelId: string, update: Pick<ChannelCharacterSnapshotPreference, 'badgeTemplateMode' | 'badgeTemplate' | 'theaterOverlayTemplateMode' | 'theaterOverlayTemplateJson'>) => {
    const requestSession = session;
    const response = await chatStore.sendAPI<{ data?: ChannelCharacterSnapshotPreference }>('character.snapshot.preference.update', { channelId, ...update });
    if (requestSession === session && response?.data?.channelId === channelId) preferenceByChannel.value = { ...preferenceByChannel.value, [channelId]: response.data };
    return response?.data;
  };

  const getChannelItems = (channelId: string) => Object.values(snapshotsByChannel.value[channelId] || {});
  const getSnapshot = (channelId: string, identityId: string) => snapshotsByChannel.value[channelId]?.[identityId] || null;

  const getOverlayTemplateForSnapshot = (item: ChannelCharacterSnapshotItem) => {
    if (!String(item.theaterOverlayTemplateJson || '').trim()) return null;
    return parseOverlayTemplate(item.theaterOverlayTemplateJson);
  };

  const currentChannelItems = computed(() => getChannelItems(String(chatStore.curChannel?.id || '')));

  watch(() => userStore.info?.id, () => {
    session += 1;
    syncTimers.forEach(timer => clearTimeout(timer));
    syncTimers.clear();
    initializationByChannel.clear();
    lastLocalSignature.clear();
    lastServerHash.clear();
    lastSyncedIdentityId.clear();
    clearedRevisions.clear();
    channelMutations.clear();
    userMutations.clear();
    listRequests.clear();
    syncRequests.clear();
    syncInFlight.clear();
    snapshotsByChannel.value = {};
    settingsByChannel.value = {};
    preferenceByChannel.value = {};
    loadingByChannel.value = {};
  }, { flush: 'sync' });

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
    getOverlayTemplateForSnapshot,
  };
});
