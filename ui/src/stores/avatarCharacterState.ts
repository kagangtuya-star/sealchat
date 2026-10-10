import { ref } from 'vue';
import { defineStore } from 'pinia';
import { chatEvent, useChatStore } from './chat';
import { useCharacterCardStore } from './characterCard';
import { useChannelCharacterSnapshotStore, type TheaterCharacterOverlayTemplate } from './channelCharacterSnapshot';
import { useUserStore } from './user';
import { buildBotStatSetCommand, resolveCharacterStatInverseValue, resolveCharacterStatMutationTarget } from '@/utils/characterStatMutation';
import { resolveCharacterNumericSource } from '@/utils/characterStatDisplay';
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
  userId: string;
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

export interface AvatarStatMutationOperation {
  channelId: string;
  identityId: string;
  source: AvatarCardSource;
  sourceCardId: string;
  statId: string;
  slot: 'current' | 'max';
  sourcePath: string;
  op: 'set' | 'add';
  value: number;
}

interface BotMutationStatus { editable: boolean; checking?: boolean; sourceCardId?: string; reason?: string }
interface PendingMutation {
  context: AvatarStatMutationOperation;
  pending: { op: 'set' | 'add'; value: number } | null;
  inFlight: { op: 'set' | 'add'; value: number } | null;
  inFlightDisplayValue: number | null;
  timer: ReturnType<typeof setTimeout> | null;
  running: boolean;
  retries: number;
}

const mutationKey = (item: Pick<AvatarStatMutationOperation, 'channelId' | 'source' | 'identityId' | 'sourceCardId' | 'statId' | 'slot'>) =>
  JSON.stringify([item.channelId, item.source, item.identityId, item.sourceCardId, item.statId, item.slot]);
const statusKey = (channelId: string, identityId: string, sourceCardId: string) =>
  JSON.stringify([channelId, identityId, sourceCardId]);
const BOT_MUTATION_STATUS_RETRY_INTERVAL_MS = 1000;
const BOT_MUTATION_STATUS_MAX_RETRIES = 18;
const apiErrorCode = (error: unknown) => {
  if (!error || typeof error !== 'object') return '';
  const response = (error as { response?: { err?: unknown } }).response;
  return String(response?.err || (error as Error).message || '').trim();
};

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
  const userStore = useUserStore();
  const settingsByChannel = ref<Record<string, ChannelAvatarCardSettings>>({});
  const worldStatesByChannel = ref<Record<string, Record<string, WorldCharacterState>>>({});
  const worldIdByChannel = ref<Record<string, string>>({});
  const worldReadyByChannel = ref<Record<string, boolean>>({});
  const latestWorldStateBySubject = ref<Record<string, WorldCharacterState>>({});
  const loading = new Map<string, Promise<void>>();
  const requestVersion = new Map<string, number>();
  const botMutationStatuses = ref<Record<string, BotMutationStatus>>({});
  const statusRequestVersion = new Map<string, number>();
  const botMutationRetryTimers = new Map<string, { timer: ReturnType<typeof setTimeout>; attempts: number }>();
  const optimisticVersion = ref(0);
  const mutationError = ref('');
  const pendingMutations = new Map<string, PendingMutation>();
  const botChannelTails = new Map<string, Promise<void>>();
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
    if (!identityId) return { source, template, attrs: null as Record<string, any> | null, ready, sourceCardId: '', ownerUserId: '' };
    if (source === 'bot') {
      const snapshot = snapshotStore.getSnapshot(channelId, identityId);
      return { source, template, attrs: snapshot?.data.card?.attrs ?? null, ready, sourceCardId: snapshot?.sourceCardId || '', ownerUserId: '' };
    }
    const state = getWorldState(channelId, identityId, sharedIdentityId);
    if (!ready) return { source, template, attrs: null as Record<string, any> | null, ready, sourceCardId: '', ownerUserId: state?.userId || '' };
    return { source, template, attrs: state?.attrs ?? {}, ready, sourceCardId: '', ownerUserId: state?.userId || '' };
  };

  const applySettings = (value: ChannelAvatarCardSettings | undefined) => {
    const channelId = String(value?.channelId || '').trim();
    if (!channelId || !value) return;
    const previous = settingsByChannel.value[channelId];
    if (previous && previous.serverRevision >= value.serverRevision) return;
    settingsByChannel.value = { ...settingsByChannel.value, [channelId]: value };
    invalidateBotMutationStatus(channelId);
    botMutationStatuses.value = {};
    for (const [key, version] of statusRequestVersion) statusRequestVersion.set(key, version + 1);
    for (const [key, entry] of pendingMutations) {
      if (entry.context.channelId !== channelId) continue;
      if (entry.timer) clearTimeout(entry.timer);
      pendingMutations.delete(key);
    }
    optimisticVersion.value++;
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
        next[identityId] = { ...current, userId: value.userId || current.userId, attrs: value.attrs, revision: value.revision };
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
        ? { ...item, userId: latest.userId || item.userId, attrs: latest.attrs, revision: latest.revision }
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

  const applyStatOperation = async (channelId: string, operation: CharacterStatOperation) => {
    if (getEffectiveSource(channelId) !== 'world') throw new Error('NOT_EDITABLE');
    const response = await chatStore.sendAPI<{ data?: WorldCharacterState }>('world.character_state.patch', {
      channelId, ...operation,
    } as APIMessage);
    applyWorldEvent(response?.data);
    return response?.data;
  };

  const getBotMutationStatus = (channelId: string, identityId: string, sourceCardId: string) =>
    botMutationStatuses.value[statusKey(channelId, identityId, sourceCardId)] || null;

  const cancelBotMutationRetry = (key: string) => {
    const entry = botMutationRetryTimers.get(key);
    if (entry) clearTimeout(entry.timer);
    botMutationRetryTimers.delete(key);
  };

  const isCurrentBotMutationStatusTarget = (channelId: string, identityId: string, sourceCardId: string) =>
    String(chatStore.curChannel?.id || '') === channelId
    && getEffectiveSource(channelId) === 'bot'
    && snapshotStore.getSnapshot(channelId, identityId)?.sourceCardId === sourceCardId;

  const requestBotMutationStatus = async (channelId: string, identityId: string, sourceCardId: string,
    key: string, version: number, retryAttempt: number) => {
    try {
      const response = await chatStore.sendAPI<{ data?: BotMutationStatus }>('avatar.card.bot_mutation.status', {
        channelId, identityId,
      } as APIMessage);
      if (statusRequestVersion.get(key) !== version || !isCurrentBotMutationStatusTarget(channelId, identityId, sourceCardId)) return;
      const reason = response?.data?.reason;
      const pending = reason === 'BOT_CHARACTER_CAPABILITY_PENDING';
      const previous = botMutationStatuses.value[key];
      const hasConfirmedEditable = previous?.editable === true && previous.sourceCardId === sourceCardId;
      botMutationStatuses.value = { ...botMutationStatuses.value,
        [key]: pending
          ? hasConfirmedEditable
            ? { ...previous, editable: true, checking: true, sourceCardId, reason }
            : { editable: false, checking: true, reason }
          : response?.data?.editable && response.data.sourceCardId === sourceCardId
            ? { editable: true, checking: false, sourceCardId }
            : { editable: false, checking: false, reason } };
      if (pending && retryAttempt < BOT_MUTATION_STATUS_MAX_RETRIES) {
        const timer = setTimeout(() => {
          const entry = botMutationRetryTimers.get(key);
          if (!entry || entry.timer !== timer) return;
          botMutationRetryTimers.delete(key);
          if (!isCurrentBotMutationStatusTarget(channelId, identityId, sourceCardId)
            || statusRequestVersion.get(key) !== version) return;
          const nextVersion = (statusRequestVersion.get(key) || 0) + 1;
          statusRequestVersion.set(key, nextVersion);
          void requestBotMutationStatus(channelId, identityId, sourceCardId, key, nextVersion, entry.attempts);
        }, BOT_MUTATION_STATUS_RETRY_INTERVAL_MS);
        botMutationRetryTimers.set(key, { timer, attempts: retryAttempt + 1 });
      }
    } catch {
      if (statusRequestVersion.get(key) === version && isCurrentBotMutationStatusTarget(channelId, identityId, sourceCardId)) {
        const previous = botMutationStatuses.value[key];
        botMutationStatuses.value = { ...botMutationStatuses.value,
          [key]: previous?.editable === true
            ? { ...previous, checking: false }
            : { editable: false, checking: false } };
      }
    }
  };

  const refreshBotMutationStatus = async (channelId: string, identityId: string, sourceCardId: string) => {
    const key = statusKey(channelId, identityId, sourceCardId);
    cancelBotMutationRetry(key);
    const version = (statusRequestVersion.get(key) || 0) + 1;
    statusRequestVersion.set(key, version);
    const previous = botMutationStatuses.value[key];
    botMutationStatuses.value = { ...botMutationStatuses.value,
      [key]: previous?.editable === true && previous.sourceCardId === sourceCardId
        ? { ...previous, checking: true }
        : { editable: false, checking: true } };
    if (!sourceCardId || !identityId || getEffectiveSource(channelId) !== 'bot') return;
    await requestBotMutationStatus(channelId, identityId, sourceCardId, key, version, 0);
  };

  const stopBotMutationStatusChecks = (channelId?: string) => {
    for (const [key, entry] of botMutationRetryTimers) {
      let matches = !channelId;
      if (channelId) {
        try { matches = JSON.parse(key)[0] === channelId; } catch { matches = false; }
      }
      if (matches) {
        clearTimeout(entry.timer);
        botMutationRetryTimers.delete(key);
      }
    }
    for (const [key, version] of statusRequestVersion) {
      let matches = !channelId;
      if (channelId) {
        try { matches = JSON.parse(key)[0] === channelId; } catch { matches = false; }
      }
      if (matches) statusRequestVersion.set(key, version + 1);
    }
  };

  const invalidateBotMutationStatus = (channelId?: string) => {
    stopBotMutationStatusChecks(channelId);
    if (!channelId) { botMutationStatuses.value = {}; return; }
    botMutationStatuses.value = Object.fromEntries(Object.entries(botMutationStatuses.value)
      .filter(([key]) => { try { return JSON.parse(key)[0] !== channelId; } catch { return false; } }));
  };

  const resolveMutationDisplayValue = (context: AvatarStatMutationOperation): number | null => {
    if (context.source === 'bot') {
      const attrs = snapshotStore.getSnapshot(context.channelId, context.identityId)?.data.card?.attrs;
      return attrs ? resolveCharacterNumericSource({ path: context.sourcePath }, attrs) : null;
    }
    const attrs = worldStatesByChannel.value[context.channelId]?.[context.identityId]?.attrs;
    return attrs ? resolveCharacterNumericSource({ path: context.sourcePath }, attrs) : null;
  };

  const getOptimisticStatValue = (context: AvatarStatMutationOperation, base: number | null) => {
    void optimisticVersion.value;
    const entry = pendingMutations.get(mutationKey(context));
    if (!entry) return base;
    if (entry.pending?.op === 'set') return entry.pending.value;
    if (entry.inFlightDisplayValue !== null) {
      return entry.pending?.op === 'add'
        ? entry.inFlightDisplayValue + entry.pending.value
        : entry.inFlightDisplayValue;
    }
    if (entry.pending?.op === 'add') {
      return base === null ? (entry.pending.value ? entry.pending.value : null) : base + entry.pending.value;
    }
    return base;
  };

  const isCurrentTarget = (context: AvatarStatMutationOperation, ignoreMenu = false) => {
    if (String(chatStore.curChannel?.id || '') !== context.channelId || getEffectiveSource(context.channelId) !== context.source) return false;
    const menuItem = chatStore.avatarMenu.item as any;
    const menuIdentityId = String(menuItem?.senderIdentityId || menuItem?.sender_identity_id
      || menuItem?.senderRoleId || menuItem?.sender_role_id || menuItem?.identity?.id || '').trim();
    if (!ignoreMenu && chatStore.avatarMenu.show && menuIdentityId && menuIdentityId !== context.identityId) return false;
    if (context.source === 'world') return isWorldStateReady(context.channelId);
    const snapshot = snapshotStore.getSnapshot(context.channelId, context.identityId);
    return !!context.sourceCardId && snapshot?.sourceCardId === context.sourceCardId;
  };

  const executeBotMutation = async (context: AvatarStatMutationOperation, op: 'set' | 'add', value: number) => {
    if (!isCurrentTarget(context) || !getBotMutationStatus(context.channelId, context.identityId, context.sourceCardId)?.editable) throw new Error('CARD_CHANGED');
    const snapshot = snapshotStore.getSnapshot(context.channelId, context.identityId);
    const own = snapshot?.userId === userStore.info.id;
    if (!own) {
      await chatStore.sendAPI('avatar.card.bot_mutation.delegate', {
        channelId: context.channelId, identityId: context.identityId, expectedSourceCardId: context.sourceCardId,
        statId: context.statId, slot: context.slot, op, value,
      } as APIMessage, { timeoutMs: 30_000 });
      return;
    }
    await executeOwnBotMutation(context, op, value);
  };

  const executeOwnBotMutation = async (context: AvatarStatMutationOperation, op: 'set' | 'add', value: number, delegated = false) => {
    if (!isCurrentTarget(context, delegated) || context.slot === 'max' && op !== 'set') throw new Error('CARD_CHANGED');
    const owner = (chatStore.channelIdentities[context.channelId] || []).find(identity => identity.id === context.identityId);
    if (owner?.userId !== userStore.info.id) throw new Error('PERMISSION_DENIED');
    const { card } = await cardStore.getActiveCardStrict(context.channelId, context.sourceCardId);
    if (!isCurrentTarget(context, delegated)) throw new Error('CARD_CHANGED');
    const target = resolveCharacterStatMutationTarget(context.sourcePath, card.attrs, {
      allowRootInit: true, directOnly: context.slot === 'max',
    });
    if (!target) throw new Error('NOT_EDITABLE');
    const latest = resolveCharacterNumericSource({ path: context.sourcePath }, card.attrs);
    if (op === 'add' && latest === null) throw new Error('NOT_EDITABLE');
    const displayValue = op === 'add' ? latest! + value : value;
    const raw = resolveCharacterStatInverseValue(target, card.attrs, displayValue);
    if (raw === null) throw new Error('NOT_EDITABLE');
    const channel = chatStore.findChannelById(context.channelId) as { botCommandPrefixes?: string[] } | undefined;
    const command = target.path.length === 1
      ? buildBotStatSetCommand(target.path[0], raw, channel?.botCommandPrefixes) : null;
    if (command) {
      let commandError: unknown;
      try {
        await chatStore.botInteract(context.channelId, command, { timeoutMs: 5000 });
      } catch (error) {
        if (apiErrorCode(error) === 'BOT_INTERACTION_BUSY') throw error;
        commandError = error;
      }
      const fresh = await cardStore.getActiveCardStrict(context.channelId, context.sourceCardId);
      const actual = resolveCharacterNumericSource({ path: context.sourcePath }, fresh.card.attrs);
      if (actual === null || Math.abs(actual - displayValue) > 1e-8 * Math.max(1, Math.abs(displayValue))) {
        throw commandError || new Error('AVATAR_CARD_BOT_MUTATION_FAILED');
      }
    } else {
      await cardStore.patchActiveCardValuePath(context.channelId, context.sourceCardId,
        context.sourcePath, target.path, op, value, context.slot === 'max');
    }
    await snapshotStore.syncLocalSnapshot(context.channelId, true);
  };

  const runInBotChannel = (channelId: string, work: () => Promise<void>) => {
    const previous = botChannelTails.get(channelId) || Promise.resolve();
    const next = previous.catch(() => {}).then(work);
    botChannelTails.set(channelId, next);
    void next.finally(() => { if (botChannelTails.get(channelId) === next) botChannelTails.delete(channelId); }).catch(() => {});
    return next;
  };

  const flushMutation = async (key: string) => {
    const entry = pendingMutations.get(key);
    if (!entry || entry.running || !entry.pending) return;
    if (entry.timer) clearTimeout(entry.timer);
    entry.timer = null;
    if (!isCurrentTarget(entry.context)) { pendingMutations.delete(key); optimisticVersion.value++; return; }
    const action = entry.pending;
    const actionOp = action.op;
    const actionValue = action.value;
    entry.pending = null;
    entry.inFlight = action;
    const currentDisplay = resolveMutationDisplayValue(entry.context);
    entry.inFlightDisplayValue = action.op === 'set'
      ? action.value
      : currentDisplay === null ? null : currentDisplay + action.value;
    entry.running = true;
    optimisticVersion.value++;
    try {
      if (entry.context.source === 'world') {
        await applyStatOperation(entry.context.channelId, { identityId: entry.context.identityId,
          path: entry.context.sourcePath, op: action.op, value: action.value });
      } else {
        await runInBotChannel(entry.context.channelId, () => executeBotMutation(entry.context, action.op, action.value));
      }
      entry.retries = 0;
    } catch (error) {
      const code = apiErrorCode(error);
      if ((code === 'BOT_INTERACTION_BUSY' || code === 'AVATAR_CARD_BOT_MUTATION_BUSY')
        && pendingMutations.get(key) === entry && entry.retries < 2 && isCurrentTarget(entry.context)) {
        entry.retries += 1;
        const newer = pendingMutations.get(key)?.pending;
        if (!newer) entry.pending = action;
        else if (newer.op === 'add') {
          const mergedValue = actionValue + newer.value;
          entry.pending = actionOp === 'set' ? { op: 'set', value: mergedValue } : { op: 'add', value: mergedValue };
        }
        entry.timer = setTimeout(() => { entry.timer = null; void flushMutation(key); }, 300);
      } else if (pendingMutations.get(key) === entry) {
        if ((code === 'BOT_INTERACTION_BUSY' || code === 'AVATAR_CARD_BOT_MUTATION_BUSY') && entry.retries >= 2) {
          entry.pending = null;
        }
        if (['CARD_CHANGED', 'TARGET_OFFLINE', 'NOT_EDITABLE'].includes(code)) invalidateBotMutationStatus(entry.context.channelId);
        mutationError.value = code || '角色数据修改失败';
      }
    } finally {
      entry.inFlight = null;
      entry.inFlightDisplayValue = null;
      entry.running = false;
      if (pendingMutations.get(key) === entry) {
        if (!entry.pending) pendingMutations.delete(key);
        else if (!entry.timer) void flushMutation(key);
      }
      optimisticVersion.value++;
    }
  };

  const queueStatMutation = (context: AvatarStatMutationOperation) => {
    if (!Number.isFinite(context.value) || !isCurrentTarget(context)) return;
    const key = mutationKey(context);
    let entry = pendingMutations.get(key);
    if (!entry) {
      entry = { context, pending: null, inFlight: null, inFlightDisplayValue: null, timer: null, running: false, retries: 0 };
      pendingMutations.set(key, entry);
    }
    if (context.op === 'set') entry.pending = { op: 'set', value: context.value };
    else if (entry.pending?.op === 'set') entry.pending.value += context.value;
    else entry.pending = { op: 'add', value: (entry.pending?.value || 0) + context.value };
    if (entry.timer) clearTimeout(entry.timer);
    entry.timer = setTimeout(() => { entry!.timer = null; void flushMutation(key); }, context.op === 'add' ? 450 : 0);
    optimisticVersion.value++;
  };

  const clearPendingMutations = () => {
    for (const entry of pendingMutations.values()) if (entry.timer) clearTimeout(entry.timer);
    pendingMutations.clear();
    optimisticVersion.value++;
  };

  const handleDelegatedMutation = async (payload: any) => {
    const context: AvatarStatMutationOperation = {
      channelId: String(payload?.channelId || ''), identityId: String(payload?.identityId || ''),
      source: 'bot', sourceCardId: String(payload?.expectedSourceCardId || ''),
      statId: String(payload?.statId || ''), slot: payload?.slot, sourcePath: String(payload?.sourcePath || ''),
      op: payload?.op, value: Number(payload?.value),
    };
    const requestId = String(payload?.requestId || '');
    if (!requestId) return;
    let errorCode = '';
    try {
      if (!context.channelId || !context.identityId || !context.sourceCardId || !context.sourcePath
        || !['current', 'max'].includes(context.slot) || !['add', 'set'].includes(context.op)
        || (context.slot === 'max' && context.op !== 'set') || !Number.isFinite(context.value)) throw new Error('NOT_EDITABLE');
      await runInBotChannel(context.channelId, () => executeOwnBotMutation(context, context.op, context.value, true));
    } catch (error) {
      errorCode = apiErrorCode(error) || 'AVATAR_CARD_BOT_MUTATION_FAILED';
    }
    try {
      await chatStore.sendAPI('avatar.card.bot_mutation.respond', {
        requestId, ok: !errorCode, ...(errorCode ? { error: errorCode } : {}),
      } as APIMessage);
    } catch (error) {
      console.warn('[AvatarCharacterState] delegated mutation response failed', error);
    }
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
    chatEvent.on('avatar-card-bot-mutation-request' as any, (event: any) => {
      void handleDelegatedMutation(event?.avatarCardBotMutation);
    });
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
      clearPendingMutations();
      invalidateBotMutationStatus();
      const channelId = String(event?.channelId || event?.channel?.id || chatStore.curChannel?.id || '').trim();
      void initializeChannel(channelId);
    });
    chatEvent.on('connected', () => {
      clearPendingMutations();
      invalidateBotMutationStatus();
      const channelId = String(chatStore.curChannel?.id || '').trim();
      if (channelId) void refreshChannel(channelId).catch(error => console.warn('[AvatarCharacterState] reconnect failed', error));
    });
    gatewayBound = true;
  };

  bindGateway();
  const currentChannelId = String(chatStore.curChannel?.id || '').trim();
  if (currentChannelId) void initializeChannel(currentChannelId);

  return { settingsByChannel, worldStatesByChannel, getSettings, getEffectiveSource, getWorldState,
    resolveAvatarCardData, isWorldStateReady, initializeChannel, refreshChannel, updateSettings, applyStatOperation,
    getBotMutationStatus, refreshBotMutationStatus, stopBotMutationStatusChecks, invalidateBotMutationStatus, getOptimisticStatValue,
    queueStatMutation, mutationError };
});
