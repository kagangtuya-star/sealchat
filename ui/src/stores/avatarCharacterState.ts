import { ref } from 'vue';
import { defineStore } from 'pinia';
import { chatEvent, useChatStore } from './chat';
import { useCharacterCardStore } from './characterCard';
import { useChannelCharacterSnapshotStore, type TheaterCharacterOverlayTemplate } from './channelCharacterSnapshot';
import type { APIMessage } from '@/types';

export type AvatarCardSource = 'bot' | 'world';

export interface ChannelAvatarCardSettings {
  channelId: string;
  sourceMode: '' | AvatarCardSource;
  botTemplateJson: string;
  worldTemplateJson: string;
  schemaVersion: number;
  serverRevision: number;
  updatedBy?: string;
}

export interface WorldCharacterState {
  worldId: string;
  identityId: string;
  sharedIdentityId?: string;
  subjectKey: string;
  attrs: Record<string, any>;
  revision: number;
}

export interface CharacterStatOperation {
  identityId: string;
  path: string;
  op: 'set' | 'add';
  value: number;
}

const emptyTemplate: TheaterCharacterOverlayTemplate = { version: 1, preferredColumns: 2, items: [] };

const parseTemplate = (raw: string): TheaterCharacterOverlayTemplate => {
  try {
    const value = JSON.parse(raw);
    if (value?.version === 1 && Array.isArray(value.items)) return value as TheaterCharacterOverlayTemplate;
  } catch { /* An unset template has no items. */ }
  return emptyTemplate;
};

export const useAvatarCharacterStateStore = defineStore('avatarCharacterState', () => {
  const chatStore = useChatStore();
  const cardStore = useCharacterCardStore();
  const snapshotStore = useChannelCharacterSnapshotStore();
  const settingsByChannel = ref<Record<string, ChannelAvatarCardSettings>>({});
  const worldStatesByChannel = ref<Record<string, Record<string, WorldCharacterState>>>({});
  const worldIdByChannel = ref<Record<string, string>>({});
  const worldReadyByChannel = ref<Record<string, boolean>>({});
  const latestWorldStateBySubject = ref<Record<string, WorldCharacterState>>({});
  const loading = new Map<string, Promise<void>>();
  const requestVersion = new Map<string, number>();
  let gatewayBound = false;

  const getSettings = (channelId: string): ChannelAvatarCardSettings => settingsByChannel.value[channelId] || {
    channelId, sourceMode: '', botTemplateJson: '', worldTemplateJson: '', schemaVersion: 1, serverRevision: 0,
  };

  const getEffectiveSource = (channelId: string): AvatarCardSource => {
    const mode = getSettings(channelId).sourceMode;
    if (mode === 'bot' || mode === 'world') return mode;
    return cardStore.isBotCharacterDisabled(channelId) ? 'world' : 'bot';
  };

  const isWorldStateReady = (channelId: string) => worldReadyByChannel.value[channelId] === true;

  const getWorldState = (channelId: string, identityId: string, sharedIdentityId?: string) => {
    const state = worldStatesByChannel.value[channelId]?.[identityId];
    if (!state || (sharedIdentityId && state.sharedIdentityId !== sharedIdentityId)) return null;
    return state;
  };

  const resolveAvatarCardData = (channelId: string, identityId: string, sharedIdentityId?: string) => {
    const source = getEffectiveSource(channelId);
    const settings = getSettings(channelId);
    const template = parseTemplate(source === 'bot' ? settings.botTemplateJson : settings.worldTemplateJson);
    const ready = source === 'bot' ? true : isWorldStateReady(channelId);
    if (!identityId) return { source, template, attrs: null as Record<string, any> | null, ready };
    if (source === 'bot') {
      const snapshot = snapshotStore.getSnapshot(channelId, identityId);
      return { source, template, attrs: snapshot?.data.card?.attrs ?? null, ready };
    }
    if (!ready) return { source, template, attrs: null as Record<string, any> | null, ready };
    return { source, template, attrs: getWorldState(channelId, identityId, sharedIdentityId)?.attrs ?? {}, ready };
  };

  const applySettings = (value: ChannelAvatarCardSettings | undefined) => {
    const channelId = String(value?.channelId || '').trim();
    if (!channelId || !value) return;
    const previous = settingsByChannel.value[channelId];
    if (previous && previous.serverRevision > value.serverRevision) return;
    settingsByChannel.value = { ...settingsByChannel.value, [channelId]: value };
  };

  const applyWorldEvent = (value: WorldCharacterState | undefined) => {
    if (!value?.worldId || !value.subjectKey) return;
    if (!Object.keys(settingsByChannel.value).some(channelId => getEffectiveSource(channelId) === 'world')) return;
    const subjectKey = `${value.worldId}:${value.subjectKey}`;
    const latest = latestWorldStateBySubject.value[subjectKey];
    if (latest && latest.revision > value.revision) return;
    latestWorldStateBySubject.value = { ...latestWorldStateBySubject.value, [subjectKey]: value };
    const nextChannels = { ...worldStatesByChannel.value };
    for (const [channelId, states] of Object.entries(nextChannels)) {
      if (worldIdByChannel.value[channelId] !== value.worldId || getEffectiveSource(channelId) !== 'world') continue;
      let changed = false;
      const next = { ...states };
      for (const [identityId, current] of Object.entries(states)) {
        if (current.subjectKey !== value.subjectKey || current.revision > value.revision) continue;
        next[identityId] = { ...current, attrs: value.attrs, revision: value.revision };
        changed = true;
      }
      if (changed) nextChannels[channelId] = next;
    }
    worldStatesByChannel.value = nextChannels;
  };

  const refreshChannel = async (channelId: string) => {
    channelId = String(channelId || '').trim();
    if (!channelId) return;
    const version = (requestVersion.get(channelId) || 0) + 1;
    requestVersion.set(channelId, version);
    worldReadyByChannel.value = { ...worldReadyByChannel.value, [channelId]: false };
    await chatStore.ensureConnectionReady();
    const settingsResponse = await chatStore.sendAPI<{ data?: ChannelAvatarCardSettings }>('avatar.card.settings.get', { channelId } as APIMessage);
    if (requestVersion.get(channelId) !== version) return;
    applySettings(settingsResponse?.data);
    if (getEffectiveSource(channelId) === 'bot') {
      worldReadyByChannel.value = { ...worldReadyByChannel.value, [channelId]: false };
      await snapshotStore.refreshChannel(channelId);
      return;
    }
    worldReadyByChannel.value = { ...worldReadyByChannel.value, [channelId]: false };
    const statesResponse = await chatStore.sendAPI<{ data?: { channelId: string; worldId: string; items: WorldCharacterState[] } }>('world.character_state.list', { channelId } as APIMessage);
    if (requestVersion.get(channelId) !== version || getEffectiveSource(channelId) !== 'world') return;
    const payload = statesResponse?.data;
    if (payload?.channelId !== channelId) return;
    const next: Record<string, WorldCharacterState> = {};
    (payload.items || []).forEach(item => {
      if (!item.identityId) return;
      const latest = latestWorldStateBySubject.value[`${payload.worldId}:${item.subjectKey}`];
      next[item.identityId] = latest && latest.revision > item.revision
        ? { ...item, attrs: latest.attrs, revision: latest.revision }
        : item;
    });
    worldIdByChannel.value = { ...worldIdByChannel.value, [channelId]: payload.worldId };
    worldStatesByChannel.value = { ...worldStatesByChannel.value, [channelId]: next };
    worldReadyByChannel.value = { ...worldReadyByChannel.value, [channelId]: true };
  };

  const initializeChannel = (channelId: string) => {
    channelId = String(channelId || '').trim();
    if (!channelId) return Promise.resolve();
    const existing = loading.get(channelId);
    if (existing) return existing;
    const task = refreshChannel(channelId).catch(error => {
      console.warn('[AvatarCharacterState] initialize failed', error);
    }).finally(() => loading.delete(channelId));
    loading.set(channelId, task);
    return task;
  };

  const updateSettings = async (channelId: string, patch: {
    sourceMode?: AvatarCardSource;
    template?: { source: AvatarCardSource; json: string };
  }) => {
    const response = await chatStore.sendAPI<{ data?: ChannelAvatarCardSettings }>('avatar.card.settings.update', {
      channelId, ...(patch.sourceMode ? { sourceMode: patch.sourceMode } : {}),
      ...(patch.template ? { templateSource: patch.template.source, templateJson: patch.template.json } : {}),
    } as APIMessage);
    applySettings(response?.data);
    if (patch.sourceMode) await refreshChannel(channelId);
    return response?.data;
  };

  const applyBotStatOperation = async (_channelId: string, _operation: CharacterStatOperation): Promise<never> => {
    // A BOT command builder is required here; future writes must go through chatStore.botInteract.
    throw new Error('当前 BOT 不支持从头像卡片直接修改属性');
  };

  const applyStatOperation = async (channelId: string, operation: CharacterStatOperation) => {
    if (getEffectiveSource(channelId) === 'bot') return applyBotStatOperation(channelId, operation);
    const response = await chatStore.sendAPI<{ data?: WorldCharacterState }>('world.character_state.patch', {
      channelId, ...operation,
    } as APIMessage);
    applyWorldEvent(response?.data);
    return response?.data;
  };

  const bindGateway = () => {
    if (gatewayBound) return;
    chatEvent.on('avatar-card-settings-updated' as any, (event: any) => {
      applySettings(event?.avatarCardSettings);
      const channelId = String(event?.avatarCardSettings?.channelId || '').trim();
      if (channelId === chatStore.curChannel?.id) {
        void refreshChannel(channelId).catch(error => console.warn('[AvatarCharacterState] settings refresh failed', error));
      }
    });
    chatEvent.on('world-character-state-updated' as any, (event: any) => applyWorldEvent(event?.worldCharacterState));
    chatEvent.on('channel-identities-updated' as any, (event: any) => {
      const options = event?.argv?.options || event?.argv?.Options || {};
      const channelId = String(options.channelId || event?.channel?.id || '').trim();
      if (channelId && channelId === chatStore.curChannel?.id) {
        void refreshChannel(channelId).catch(error => console.warn('[AvatarCharacterState] identity refresh failed', error));
      }
    });
    chatEvent.on('channel-identity-updated', (event) => {
      const channelId = String(event?.channelId || '').trim();
      const removedId = String(event?.removedId || '').trim();
      if (!channelId || !removedId) return;
      const states = { ...(worldStatesByChannel.value[channelId] || {}) };
      delete states[removedId];
      worldStatesByChannel.value = { ...worldStatesByChannel.value, [channelId]: states };
    });
    chatEvent.on('channel-switch-to' as any, (event: any) => {
      const channelId = String(event?.channelId || event?.channel?.id || chatStore.curChannel?.id || '').trim();
      void initializeChannel(channelId);
    });
    chatEvent.on('connected', () => {
      const channelId = String(chatStore.curChannel?.id || '').trim();
      if (channelId) void refreshChannel(channelId).catch(error => console.warn('[AvatarCharacterState] reconnect failed', error));
    });
    gatewayBound = true;
  };

  bindGateway();
  const currentChannelId = String(chatStore.curChannel?.id || '').trim();
  if (currentChannelId) void initializeChannel(currentChannelId);

  return { settingsByChannel, worldStatesByChannel, getSettings, getEffectiveSource, getWorldState,
    resolveAvatarCardData, isWorldStateReady, initializeChannel, refreshChannel, updateSettings, applyStatOperation };
});
