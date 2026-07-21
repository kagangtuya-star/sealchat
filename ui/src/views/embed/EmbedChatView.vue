<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, toRaw } from 'vue';
import { useRoute } from 'vue-router';
import { throttle } from 'lodash-es';
import Chat from '@/views/chat/chat.vue';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useChannelSearchStore } from '@/stores/channelSearch';
import { usePushNotificationStore } from '@/stores/pushNotification';
import { useIFormStore } from '@/stores/iform';
import { useAudioStudioStore } from '@/stores/audioStudio';
import AudioDrawer from '@/components/audio/AudioDrawer.vue';
import ChatHeader from '@/views/components/header.vue';
import ChatSidebar from '@/views/components/sidebar.vue';
import { formatSplitChannelDisplayName, type SplitChannelDisplayLike } from '@/views/split/splitChannelDisplay';
import type { SplitSessionPaneSnapshot } from '@/utils/splitSessionStorage';
import { TheaterBridgeClient } from '@/views/theater/bridge/TheaterBridgeClient';
import {
  THEATER_BRIDGE_VERSION,
  THEATER_CHAT_CAPABILITIES,
  type ChatCharacterReadResult,
  type ChatCharactersSnapshotPayload,
  type ChatComposerInsertPayload,
  type ChatComposerInsertResult,
  type ChatMessageSendPayload,
  type ChatMessageSendResult,
  type InitializePayload,
  type SelectCharacterPayload,
  type SelectCharacterResult,
  type SelectCharacterVariantPayload,
  type SceneAppliedPayload,
  type StageSceneReadResult,
} from '@/views/theater/bridge/theater-bridge-protocol';
import { PostMessageTransport } from '@/views/theater/bridge/theater-bridge-transport';
import { getCharacterSnapshotContentSignature } from '@/views/theater/bridge/theater-character-snapshot';
import { subscribeTheaterChatMessageEvents } from '@/views/theater/bridge/theater-chat-message-events';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import {
  installTheaterBridgeDebugConsoleCommand,
  isTheaterBridgeDebugEnabled,
} from '@/views/theater/bridge/theater-bridge-debug';

type PaneId = 'A' | 'B' | 'theater-chat';

installTheaterBridgeDebugConsoleCommand();
type ConnectState = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';
type PresenceData = {
  lastPing: number;
  latencyMs: number;
  isFocused: boolean;
};
type PresenceMember = {
  id: string;
  nick?: string;
  name?: string;
  avatar?: string;
  identity?: {
    displayName?: string;
    color?: string;
  };
};

type FilterState = {
  icFilter: 'all' | 'ic' | 'ooc';
  showArchived: boolean;
  roleIds: string[];
};

type RoleOption = { id: string; label?: string; name?: string };

type SplitChannelNode = {
  id: string;
  name: string;
  permType?: string;
  unread: number;
  children?: SplitChannelNode[];
};

const route = useRoute();
const chat = useChatStore();
const channelSearch = useChannelSearchStore();
const pushStore = usePushNotificationStore();
const iFormStore = useIFormStore();
iFormStore.bootstrap();
const audioStudio = useAudioStudioStore();

const paneId = computed(() => (typeof route.query.paneId === 'string' ? route.query.paneId : '') as PaneId | '');
const initialWorldId = computed(() => (typeof route.query.worldId === 'string' ? route.query.worldId : ''));
const initialChannelId = computed(() => (typeof route.query.channelId === 'string' ? route.query.channelId : ''));
const splitScopeWorldId = computed(() => (typeof route.query.scopeWorldId === 'string' ? route.query.scopeWorldId : ''));
const initialNotifyOwner = computed(() => (route.query.notifyOwner === '1' || route.query.notifyOwner === 'true'));
const initialAudioOwner = computed(() => {
  if (route.query.audioOwner === undefined) return true;
  return route.query.audioOwner === '1' || route.query.audioOwner === 'true';
});
const theaterMode = computed(() => route.query.mode === 'theater');
const theaterSessionId = computed(() => (typeof route.query.sessionId === 'string' ? route.query.sessionId.trim() : ''));
const chatViewRef = ref<any>(null);

const theaterSidebarVisible = ref(false);
const handleTheaterHeaderSidebarToggle = () => {
  theaterSidebarVisible.value = !theaterSidebarVisible.value;
};
const initializing = ref(false);
const restoringSession = ref(false);
const roleOptions = ref<RoleOption[]>([]);
const audioOwner = ref(initialAudioOwner.value);
let theaterBridgeClient: TheaterBridgeClient | null = null;
let disposeTheaterMessageEvents: (() => void) | null = null;
let theaterBridgeInitialized = false;
let theaterCharacterRevision = 0;
let theaterCharacterUpdatedAt = 0;
let theaterCharacterPublishChain = Promise.resolve();
let theaterLastCharacterSnapshot: ChatCharactersSnapshotPayload | null = null;
let theaterLastCharacterSnapshotSignature = '';
let theaterGrantedPermissions = new Set<string>();
let theaterPublishedContext = '';
let theaterBridgeGeneration = 0;
let forwardTheaterAppearanceInvalidation: ((event: Event) => void) | null = null;

const isOwnerOrAdmin = computed(() => {
  const worldId = chat.currentWorldId;
  if (!worldId) return false;
  const detail = chat.worldDetailMap[worldId];
  const role = detail?.memberRole;
  return role === 'owner' || role === 'admin';
});

const iFormButtonActive = computed(() => iFormStore.drawerVisible || iFormStore.hasInlinePanels || iFormStore.hasFloatingWindows);
const iFormHasAttention = computed(() => iFormStore.hasAttention);

const buildChannelTree = (): SplitChannelNode[] => {
  const unreadMap = chat.unreadCountMap || {};
  const walk = (items: any[]): SplitChannelNode[] => {
    if (!Array.isArray(items)) return [];
    return items
      .filter(Boolean)
      .map((item) => {
        const children = walk(item.children || []);
        const selfUnread = typeof unreadMap[item.id] === 'number' ? unreadMap[item.id] : 0;
        const childrenUnread = children.reduce((sum, child) => sum + (child.unread || 0), 0);
        return {
          id: String(item.id || ''),
          name: String(item.name || ''),
          permType: typeof item?.permType === 'string' ? item.permType : undefined,
          unread: selfUnread + childrenUnread,
          children,
        };
      })
      .filter((node) => node.id);
  };
  return walk(chat.channelTree as any[]);
};

const fetchRoleOptions = async (channelId: string) => {
  const normalizedId = typeof channelId === 'string' ? channelId.trim() : '';
  if (!normalizedId) {
    roleOptions.value = [];
    return;
  }
  try {
    const payload = await chat.channelSpeakerOptions(normalizedId);
    const items = Array.isArray(payload?.items) ? payload.items : [];
    roleOptions.value = items
      .map((item) => ({ id: String(item.id || ''), label: item.label || '未命名角色' }))
      .filter((item) => item.id);
  } catch {
    roleOptions.value = [];
  }
};

const postToParent = (payload: any) => {
  if (typeof window === 'undefined') return;
  if (!paneId.value) return;
  if (window.parent === window) return;
  try {
    window.parent.postMessage(payload, window.location.origin);
  } catch (e) {
    console.warn('[embed] postMessage failed', e);
  }
};

const stopTheaterBridge = () => {
  theaterBridgeGeneration += 1;
  disposeTheaterMessageEvents?.();
  disposeTheaterMessageEvents = null;
  theaterBridgeInitialized = false;
  theaterGrantedPermissions = new Set();
  theaterLastCharacterSnapshot = null;
  theaterLastCharacterSnapshotSignature = '';
  theaterPublishedContext = '';
  theaterBridgeClient?.disconnect();
  theaterBridgeClient = null;
};

const publishTheaterContext = () => {
  if (!theaterMode.value || !theaterSessionId.value) return;
  const worldId = String(chat.currentWorldId || '').trim();
  const channelId = String(chat.curChannel?.id || '').trim();
  if (!worldId || !channelId) return;
  const signature = `${worldId}:${channelId}`;
  if (signature === theaterPublishedContext) return;
  theaterPublishedContext = signature;
  postToParent({
    type: 'sealchat.theater.context',
    sessionId: theaterSessionId.value,
    worldId,
    channelId,
  });
};

const handleTheaterChannelSwitch = () => {
  disposeTheaterMessageEvents?.();
  disposeTheaterMessageEvents = null;
  publishTheaterContext();
};

const buildTheaterCharacterSnapshot = async (): Promise<ChatCharactersSnapshotPayload> => {
  const handler = chatViewRef.value?.getCharactersForTheater;
  if (typeof handler !== 'function') {
    throw new Error('聊天角色快照流程尚未就绪');
  }
  const revision = theaterCharacterRevision + 1;
  const updatedAt = Math.max(Date.now(), theaterCharacterUpdatedAt + 1);
  theaterCharacterRevision = revision;
  theaterCharacterUpdatedAt = updatedAt;
  const characters = await handler({ revision, updatedAt });
  const activeIdentityId = characters.find((character: { isActive?: boolean }) => character.isActive)?.identityId || null;
  return { revision, updatedAt, activeIdentityId, characters };
};

const queueTheaterCharacterPublish = (
  event: 'updated' | 'selected' | 'appearance' | 'variant',
  identityId?: string | null,
  variantId?: string | null,
) => {
  const task = theaterCharacterPublishChain.then(async () => {
    const client = theaterBridgeClient;
    if (!client || !theaterBridgeInitialized) return null;
    const snapshot = await buildTheaterCharacterSnapshot();
    const signature = getCharacterSnapshotContentSignature(snapshot);
    if (signature === theaterLastCharacterSnapshotSignature && theaterLastCharacterSnapshot) {
      return theaterLastCharacterSnapshot;
    }
    theaterLastCharacterSnapshot = snapshot;
    theaterLastCharacterSnapshotSignature = signature;
    if (event === 'selected' && identityId) {
      client.emit('stage', 'chat.character.selected', { ...snapshot, identityId });
    } else if (event === 'variant' && identityId) {
      client.emit('stage', 'chat.character.variant.selected', {
        ...snapshot,
        identityId,
        variantId: variantId || null,
      });
    } else if (event === 'appearance') {
      client.emit('stage', 'chat.character.appearance.updated', {
        ...snapshot,
        identityId: identityId || null,
      });
    } else {
      client.emit('stage', 'chat.character.updated', snapshot);
    }
    return snapshot;
  }).catch((error) => {
    if (import.meta.env.DEV || route.query.bridgeDebug === '1' || isTheaterBridgeDebugEnabled()) {
      console.warn('[theater-bridge:chat] character publish failed', error);
    }
    return null;
  });
  theaterCharacterPublishChain = task.then(() => undefined);
  return task;
};

const startTheaterBridge = async () => {
  if (!theaterMode.value || theaterBridgeClient) return;
  const worldId = initialWorldId.value.trim();
  const channelId = initialChannelId.value.trim();
  const sessionId = theaterSessionId.value;
  if (!worldId || !channelId || !sessionId || window.parent === window) return;
  if (chat.currentWorldId !== worldId || String(chat.curChannel?.id || '') !== channelId) {
    console.warn('[theater-bridge] chat context does not match iframe context');
    return;
  }
  const generation = ++theaterBridgeGeneration;

  const debug = () => import.meta.env.DEV || route.query.bridgeDebug === '1' || isTheaterBridgeDebugEnabled();
  const transport = new PostMessageTransport({
    receiveWindow: window,
    targetWindow: () => window.parent,
    expectedSource: () => window.parent,
    targetOrigin: window.location.origin,
    expectedOrigin: window.location.origin,
    onRejected: (reason, error) => {
      if (debug()) console.warn('[theater-bridge:chat] rejected', reason, error || '');
    },
  });
  const client = new TheaterBridgeClient({
    endpoint: 'chat',
    context: { worldId, channelId, sessionId },
    transport,
    capabilities: THEATER_CHAT_CAPABILITIES,
    debug,
  });
  client.onSystem<InitializePayload>('system.initialize', (payload, message) => {
    if (
      message.source !== 'host'
      || message.target !== 'chat'
      || payload.selectedVersion !== THEATER_BRIDGE_VERSION
      || payload.worldId !== worldId
      || payload.channelId !== channelId
    ) return;
    theaterGrantedPermissions = new Set(payload.permissions);
    client.setRemoteCapabilities('stage', payload.capabilities);
    client.sendSystem('host', 'system.initialized', {
      endpoint: 'chat',
      selectedVersion: THEATER_BRIDGE_VERSION,
      capabilities: [...THEATER_CHAT_CAPABILITIES],
    });
    theaterBridgeInitialized = true;
    disposeTheaterMessageEvents?.();
    disposeTheaterMessageEvents = subscribeTheaterChatMessageEvents({
      eventSource: chatEvent as any,
      client,
      bridgeContext: { worldId, channelId, sessionId },
      getCurrentContext: () => ({
        worldId: String(chat.currentWorldId || '').trim(),
        channelId: String(chat.curChannel?.id || '').trim(),
        sessionId: theaterSessionId.value,
      }),
      isInitialized: () => theaterBridgeInitialized && theaterBridgeClient === client,
      resolveAttachmentUrl,
    });
    queueTheaterCharacterPublish('updated');
    window.setTimeout(() => {
      void client.request<Record<string, never>, StageSceneReadResult>('stage', 'stage.scene.read', {})
        .then((result) => {
          if (debug() && result.ok) console.debug('[theater-bridge:chat] stage scene ready', result.state.activeSceneId);
        })
        .catch((error) => {
          if (debug()) console.warn('[theater-bridge:chat] stage.scene.read failed', error);
        });
    }, 0);
  });
  client.onEvent<SceneAppliedPayload>('stage.scene.applied', (payload) => {
    if (debug()) console.debug('[theater-bridge:chat] stage.scene.applied', payload);
  });
  client.onCommand<ChatMessageSendPayload, ChatMessageSendResult>('chat.message.send', async (payload, bridgeMessage) => {
    if (bridgeMessage.source !== 'stage' || bridgeMessage.target !== 'chat') {
      return { ok: false, error: { code: 'INVALID_SOURCE', message: 'chat.message.send 仅接受舞台端命令' } };
    }
    if (payload.channelId && payload.channelId !== channelId) {
      return { ok: false, error: { code: 'CHANNEL_MISMATCH', message: '消息频道与小剧场上下文不一致' } };
    }
    if (String(chat.curChannel?.id || '') !== channelId || chat.currentWorldId !== worldId) {
      return { ok: false, error: { code: 'CONTEXT_CHANGED', message: '聊天已离开小剧场绑定频道' } };
    }
    const handler = chatViewRef.value?.sendMessageForTheater;
    if (typeof handler !== 'function') {
      return { ok: false, error: { code: 'CHAT_UNAVAILABLE', message: '聊天发送流程尚未就绪' } };
    }
    return handler({ ...payload, channelId });
  });
  client.onCommand<ChatComposerInsertPayload, ChatComposerInsertResult>('chat.composer.insert', (payload, bridgeMessage) => {
    if (bridgeMessage.source !== 'stage' || bridgeMessage.target !== 'chat') {
      return { ok: false, error: { code: 'INVALID_SOURCE', message: 'chat.composer.insert 仅接受舞台端命令' } };
    }
    if (String(chat.curChannel?.id || '') !== channelId || chat.currentWorldId !== worldId) {
      return { ok: false, error: { code: 'CONTEXT_CHANGED', message: '聊天已离开小剧场绑定频道' } };
    }
    const handler = chatViewRef.value?.insertComposerForTheater;
    if (typeof handler !== 'function') {
      return { ok: false, error: { code: 'CHAT_UNAVAILABLE', message: '聊天输入流程尚未就绪' } };
    }
    return handler(payload);
  });
  client.onCommand<Record<string, never>, ChatCharacterReadResult>('chat.character.read', async (_payload, bridgeMessage) => {
    if (bridgeMessage.source !== 'stage' || bridgeMessage.target !== 'chat') {
      return { ok: false, error: { code: 'INVALID_SOURCE', message: 'chat.character.read 仅接受舞台端命令' } };
    }
    if (!theaterGrantedPermissions.has('chat.character.read')) {
      return { ok: false, error: { code: 'PERMISSION_DENIED', message: '缺少权限: chat.character.read' } };
    }
    if (String(chat.curChannel?.id || '') !== channelId || chat.currentWorldId !== worldId) {
      return { ok: false, error: { code: 'CONTEXT_CHANGED', message: '聊天已离开小剧场绑定频道' } };
    }
    return { ok: true, snapshot: await buildTheaterCharacterSnapshot() };
  });
  client.onCommand<SelectCharacterPayload, SelectCharacterResult>('chat.character.select', async (payload, bridgeMessage) => {
    if (bridgeMessage.source !== 'stage' || bridgeMessage.target !== 'chat') {
      return { ok: false, error: { code: 'INVALID_SOURCE', message: 'chat.character.select 仅接受舞台端命令' } };
    }
    if (!theaterGrantedPermissions.has('chat.character.select')) {
      return { ok: false, error: { code: 'PERMISSION_DENIED', message: '缺少权限: chat.character.select' } };
    }
    if (String(chat.curChannel?.id || '') !== channelId || chat.currentWorldId !== worldId) {
      return { ok: false, error: { code: 'CONTEXT_CHANGED', message: '聊天已离开小剧场绑定频道' } };
    }
    const handler = chatViewRef.value?.selectCharacterForTheater;
    if (typeof handler !== 'function') {
      return { ok: false, error: { code: 'CHAT_UNAVAILABLE', message: '聊天角色选择流程尚未就绪' } };
    }
    const result = await handler(payload);
    if (!result.ok) return result;
    const snapshot = await queueTheaterCharacterPublish('selected', payload.identityId);
    if (!snapshot) {
      return { ok: false, error: { code: 'SNAPSHOT_UNAVAILABLE', message: '角色已切换，但角色快照生成失败' } };
    }
    return { ok: true, snapshot };
  });
  client.onCommand<SelectCharacterVariantPayload, SelectCharacterResult>('chat.character.variant.select', async (payload, bridgeMessage) => {
    if (bridgeMessage.source !== 'stage' || bridgeMessage.target !== 'chat') {
      return { ok: false, error: { code: 'INVALID_SOURCE', message: 'chat.character.variant.select 仅接受舞台端命令' } };
    }
    if (!theaterGrantedPermissions.has('chat.character.variant.select')) {
      return { ok: false, error: { code: 'PERMISSION_DENIED', message: '缺少权限: chat.character.variant.select' } };
    }
    if (String(chat.curChannel?.id || '') !== channelId || chat.currentWorldId !== worldId) {
      return { ok: false, error: { code: 'CONTEXT_CHANGED', message: '聊天已离开小剧场绑定频道' } };
    }
    const handler = chatViewRef.value?.selectCharacterVariantForTheater;
    if (typeof handler !== 'function') {
      return { ok: false, error: { code: 'CHAT_UNAVAILABLE', message: '聊天差分选择流程尚未就绪' } };
    }
    const result = await handler(payload);
    if (!result.ok) return result;
    const snapshot = await queueTheaterCharacterPublish('variant', payload.identityId, payload.variantId);
    if (!snapshot) {
      return { ok: false, error: { code: 'SNAPSHOT_UNAVAILABLE', message: '差分已切换，但角色快照生成失败' } };
    }
    return { ok: true, snapshot };
  });
  await client.connect();
  if (generation !== theaterBridgeGeneration || initialWorldId.value.trim() !== worldId || initialChannelId.value.trim() !== channelId) {
    client.disconnect();
    return;
  }
  theaterBridgeClient = client;
  client.sendSystem('host', 'system.ready', {
    endpoint: 'chat',
    supportedVersions: [THEATER_BRIDGE_VERSION],
    capabilities: [...THEATER_CHAT_CAPABILITIES],
  });
};

const normalizeWorldOptions = (options: any): Array<{ value: string; label: string }> => {
  const raw = Array.isArray(options) ? options : [];
  return raw
    .map((item) => {
      const value = typeof item?.value === 'string' ? item.value : String(item?.value || '');
      const label = typeof item?.label === 'string' ? item.label : String(item?.label || '');
      return { value, label };
    })
    .filter((item) => item.value);
};

const normalizeFilterState = (state: any): FilterState => {
  const roleIdsRaw = Array.isArray(state?.roleIds) ? state.roleIds : [];
  const icFilter = ['all', 'ic', 'ooc'].includes(state?.icFilter) ? state.icFilter : 'all';
  return {
    icFilter,
    showArchived: !!state?.showArchived,
    roleIds: roleIdsRaw.map((id: any) => String(id || '')).filter(Boolean),
  };
};

const normalizeRoleOptions = (items: any): RoleOption[] => {
  const raw = Array.isArray(items) ? items : [];
  return raw
    .map((item) => {
      const id = typeof item?.id === 'string' ? item.id : String(item?.id || '');
      const label = typeof item?.label === 'string' ? item.label : typeof item?.name === 'string' ? item.name : undefined;
      return { id, label };
    })
    .filter((item) => item.id);
};

const normalizeChannelTree = (nodes: any): SplitChannelNode[] => {
  const raw = Array.isArray(nodes) ? nodes : [];
  const walk = (items: any[]): SplitChannelNode[] => {
    if (!Array.isArray(items)) return [];
    return items
      .map((item) => {
        const id = typeof item?.id === 'string' ? item.id : String(item?.id || '');
        const name = typeof item?.name === 'string' ? item.name : String(item?.name || '');
        const permType = typeof item?.permType === 'string' ? item.permType : undefined;
        const unread = typeof item?.unread === 'number' ? item.unread : Number(item?.unread || 0);
        const children = walk(item?.children || []);
        if (!id) return null;
        return { id, name, permType, unread: Number.isFinite(unread) ? unread : 0, children };
      })
      .filter(Boolean) as SplitChannelNode[];
  };
  return walk(raw);
};

const normalizePresenceMembers = (members: any): PresenceMember[] => {
  const raw = Array.isArray(members) ? members : [];
  return raw
    .map((item) => {
      const id = typeof item?.id === 'string' ? item.id : String(item?.id || '');
      if (!id) return null;
      const identityDisplayName = typeof item?.identity?.displayName === 'string'
        ? item.identity.displayName
        : typeof item?.identity?.display_name === 'string'
          ? item.identity.display_name
          : undefined;
      const identityColor = typeof item?.identity?.color === 'string'
        ? item.identity.color
        : undefined;
      return {
        id,
        nick: typeof item?.nick === 'string' ? item.nick : undefined,
        name: typeof item?.name === 'string' ? item.name : undefined,
        avatar: typeof item?.avatar === 'string' ? item.avatar : undefined,
        identity: identityDisplayName || identityColor
          ? {
            displayName: identityDisplayName,
            color: identityColor,
          }
          : undefined,
      };
    })
    .filter(Boolean) as PresenceMember[];
};

const normalizePresenceMap = (map: any): Record<string, PresenceData> => {
  const raw = map && typeof map === 'object' ? map : {};
  return Object.entries(raw).reduce<Record<string, PresenceData>>((acc, [userId, value]) => {
    if (!userId) return acc;
    const entry = value && typeof value === 'object' ? value as Record<string, any> : {};
    const lastPing = Number(entry.lastPing);
    const latencyMs = Number(entry.latencyMs);
    acc[String(userId)] = {
      lastPing: Number.isFinite(lastPing) ? lastPing : 0,
      latencyMs: Number.isFinite(latencyMs) ? latencyMs : 0,
      isFocused: !!entry.isFocused,
    };
    return acc;
  }, {});
};

const currentIdentityId = computed(() => {
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  return channelId ? chat.getActiveIdentityId(channelId) : '';
});

const currentIdentityVariantId = computed(() => {
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const identityId = currentIdentityId.value;
  return channelId && identityId ? chat.getActiveIdentityVariantId(channelId, identityId) : '';
});

const postState = (type: 'sealchat.embed.ready' | 'sealchat.embed.state') => {
  if (!paneId.value) return;
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const currentChannelDisplay = chat.curChannel as SplitChannelDisplayLike | null | undefined;
  const channelName = formatSplitChannelDisplayName(currentChannelDisplay);
  const channelPermType = typeof currentChannelDisplay?.permType === 'string' ? currentChannelDisplay.permType : '';
  const worldId = chat.currentWorldId || '';
  const worldName = chat.currentWorld?.name || '';
  const connectState = (chat.connectState || 'connecting') as ConnectState;
  const onlineMembersCount = Array.isArray(chat.curChannelUsers) ? chat.curChannelUsers.length : 0;
  const currentChannelUnread = channelId ? (chat.unreadCountMap?.[channelId] || 0) : 0;

  const payload = {
    type,
    paneId: paneId.value,
    worldId,
    worldName,
    // 注意：postMessage 需要结构化克隆，避免直接传递 Vue reactive/proxy（会触发 DataCloneError）
    worldOptions: normalizeWorldOptions(toRaw(chat.joinedWorldOptions)),
    channelId,
    channelName,
    channelPermType,
    connectState,
    onlineMembersCount,
    members: normalizePresenceMembers(toRaw(chat.curChannelUsers)),
    presenceMap: normalizePresenceMap(toRaw(chat.presenceMap)),
    currentChannelUnread,
    audioStudioDrawerVisible: !!audioStudio.drawerVisible,
    filterState: normalizeFilterState(toRaw(chat.filterState)),
    identityId: currentIdentityId.value,
    identityVariantId: currentIdentityVariantId.value,
    roleOptions: normalizeRoleOptions(toRaw(roleOptions.value)),
    canImport: isOwnerOrAdmin.value,
    channelTree: normalizeChannelTree(buildChannelTree()),
    searchPanelVisible: !!channelSearch.panelVisible,
    stickyNoteVisible: !!chatViewRef.value?.getStickyNoteVisible?.(),
    characterCardVisible: !!chatViewRef.value?.getCharacterCardVisible?.(),
    characterCardEnabled: !!channelId && chat.curChannel?.characterApiEnabled !== false,
    characterCardReason: typeof chat.curChannel?.characterApiReason === 'string' ? chat.curChannel.characterApiReason : '',
    iFormButtonActive: !!iFormButtonActive.value,
    iFormHasAttention: !!iFormHasAttention.value,
  };
  postToParent(payload);
};

const postStateThrottled = throttle((type: 'sealchat.embed.ready' | 'sealchat.embed.state') => postState(type), 200, {
  leading: true,
  trailing: true,
});

const syncAudioStudioContext = () => {
  if (!audioOwner.value) return;
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  audioStudio.setCurrentWorld(chat.currentWorldId || null);
  audioStudio.setActiveChannel(channelId || null);
};

const syncChannelSessionRestoreOverride = (filterState: FilterState | null | undefined) => {
  chat.setChannelSessionRestoreFilterOverride(normalizeFilterState(filterState).icFilter);
};

const restoreSplitPaneIdentitySnapshot = async (snapshot: SplitSessionPaneSnapshot) => {
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const identityId = typeof snapshot.identityId === 'string' ? snapshot.identityId.trim() : '';
  const identityVariantId = typeof snapshot.identityVariantId === 'string' ? snapshot.identityVariantId.trim() : '';
  if (!channelId || !identityId) {
    return;
  }
  const identities = await chat.loadChannelIdentities(channelId, false);
  const targetIdentity = Array.isArray(identities)
    ? identities.find((item) => String(item?.id || '') === identityId)
    : null;
  if (!targetIdentity) {
    return;
  }
  chat.setActiveIdentity(channelId, identityId, undefined, {
    persist: false,
    syncIcOocFromRole: false,
  });
  const variantsByIdentity = await chat.loadChannelIdentityVariants(channelId, false);
  const variants = variantsByIdentity?.[identityId] || [];
  if (!identityVariantId) {
    chat.setActiveIdentityVariant(channelId, identityId, '', undefined, { persist: false });
    return;
  }
  if (variants.some((item) => String(item?.id || '') === identityVariantId)) {
    chat.setActiveIdentityVariant(channelId, identityId, identityVariantId, undefined, { persist: false });
  }
};

const postFocus = () => {
  if (!paneId.value) return;
  postToParent({ type: 'sealchat.embed.focus', paneId: paneId.value });
};

const handleInteraction = () => postFocus();

const handleDrawerShow = () => {
  if (!paneId.value) return;
  postToParent({ type: 'sealchat.embed.requestToggleSidebar', paneId: paneId.value });
};

const handleMessage = async (event: MessageEvent) => {
  if (event.origin !== window.location.origin) return;
  const data = event.data as any;
  if (!data || typeof data !== 'object') return;
  if (data.paneId && paneId.value && data.paneId !== paneId.value) return;

  if (data.type === 'sealchat.embed.setNotifyOwner') {
    pushStore.setEmbedNotifyOwner(!!data.enabled);
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.setAudioOwner') {
    const enabled = !!data.enabled;
    audioOwner.value = enabled;
    audioStudio.setPlaybackAuthority(enabled);
    if (enabled) {
      syncAudioStudioContext();
    }
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.setFilterState') {
    if (data.filterState) {
      chat.setFilterState(data.filterState);
      syncChannelSessionRestoreOverride(data.filterState);
      postStateThrottled('sealchat.embed.state');
    }
    return;
  }

  if (data.type === 'sealchat.embed.openPanel') {
    const panel = typeof data.panel === 'string' ? data.panel : '';
    if (panel && chatViewRef.value?.openPanelForShell) {
      chatViewRef.value.openPanelForShell(panel);
    }
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.refreshPresence') {
    if (chatViewRef.value?.refreshPresenceForShell) {
      await chatViewRef.value.refreshPresenceForShell(!!data.silent);
    }
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.setStickyNoteVisible') {
    if (typeof data.visible === 'boolean' && chatViewRef.value?.setStickyNoteVisible) {
      chatViewRef.value.setStickyNoteVisible(data.visible);
    }
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.setCharacterCardVisible') {
    if (typeof data.visible === 'boolean' && chatViewRef.value?.setCharacterCardVisible) {
      chatViewRef.value.setCharacterCardVisible(data.visible);
    }
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.openAudioStudio') {
    const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
    audioStudio.setActiveChannel(channelId || null);
    audioStudio.toggleDrawer(true);
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.openIFormDrawer') {
    const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
    if (!channelId) return;
    try {
      await iFormStore.ensureForms(channelId);
      iFormStore.openDrawer();
      postStateThrottled('sealchat.embed.state');
    } catch (e) {
      console.warn('[embed] openIFormDrawer failed', e);
    }
    return;
  }

  if (data.type === 'sealchat.embed.setChannel') {
    const channelId = typeof data.channelId === 'string' ? data.channelId : '';
    if (!channelId) return;
    try {
      await chat.channelSwitchTo(channelId);
      postStateThrottled('sealchat.embed.state');
    } catch (e) {
      console.warn('[embed] channelSwitchTo failed', e);
    }
    return;
  }

  if (data.type === 'sealchat.embed.setWorld') {
    const worldId = typeof data.worldId === 'string' ? data.worldId : '';
    const channelId = typeof data.channelId === 'string' ? data.channelId : '';
    if (!worldId) return;
    try {
      await chat.switchWorld(worldId, { force: true });
      if (channelId) {
        await chat.channelSwitchTo(channelId);
      }
      postStateThrottled('sealchat.embed.state');
    } catch (e) {
      console.warn('[embed] switchWorld failed', e);
    }
    return;
  }

  if (data.type === 'sealchat.embed.restoreSession') {
    const snapshot = data.snapshot as SplitSessionPaneSnapshot | undefined;
    if (!snapshot || snapshot.mode !== 'chat') return;
    restoringSession.value = true;
    try {
      syncChannelSessionRestoreOverride(snapshot.filterState);
      if (snapshot.worldId) {
        await chat.switchWorld(snapshot.worldId, { force: true });
      }
      if (snapshot.channelId) {
        await chat.channelSwitchTo(snapshot.channelId);
      }
      chat.setFilterState(normalizeFilterState(snapshot.filterState));
      await restoreSplitPaneIdentitySnapshot(snapshot);
      if (chatViewRef.value?.setSearchPanelVisibleForShell) {
        chatViewRef.value.setSearchPanelVisibleForShell(!!snapshot.searchPanelVisible);
      } else if (snapshot.searchPanelVisible && chatViewRef.value?.openPanelForShell) {
        chatViewRef.value.openPanelForShell('search');
      }
      if (chatViewRef.value?.setStickyNoteVisible) {
        chatViewRef.value.setStickyNoteVisible(!!snapshot.stickyNoteVisible);
      }
      if (chatViewRef.value?.setCharacterCardVisible) {
        chatViewRef.value.setCharacterCardVisible(!!snapshot.characterCardVisible);
      }
      if (snapshot.audioStudioDrawerVisible) {
        const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
        audioStudio.setActiveChannel(channelId || null);
        audioStudio.toggleDrawer(true);
      } else {
        audioStudio.toggleDrawer(false);
      }
      if (snapshot.embedPanelActive) {
        const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
        if (channelId) {
          await iFormStore.ensureForms(channelId);
          iFormStore.openDrawer();
        }
      } else {
        iFormStore.closeDrawer();
      }
      postState('sealchat.embed.state');
    } catch (e) {
      console.warn('[embed] restoreSession failed', e);
    } finally {
      restoringSession.value = false;
    }
    return;
  }
};

const initialize = async () => {
  if (initializing.value) return;
  initializing.value = true;
  try {
    pushStore.setEmbedNotifyOwner(initialNotifyOwner.value);
    await chat.ensureWorldReady();
    if (initialWorldId.value) {
      chat.setCurrentWorld(initialWorldId.value);
    }
    // 先把世界列表/当前世界同步给壳页面，避免 WS 尚未 ready 时侧边栏一直空白
    postStateThrottled('sealchat.embed.state');
    await chat.channelList(chat.currentWorldId, true);
    if (initialChannelId.value) {
      await chat.channelSwitchTo(initialChannelId.value);
    }
    await fetchRoleOptions(chat.curChannel?.id ? String(chat.curChannel.id) : '');
    postStateThrottled('sealchat.embed.ready');
  } finally {
    initializing.value = false;
  }
};

watch(
  () => chat.curChannel?.id,
  (channelId) => {
    fetchRoleOptions(channelId ? String(channelId) : '');
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => [chat.curChannel?.id, chat.curChannel?.characterApiEnabled, chat.curChannel?.characterApiReason] as const,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => [chat.currentWorldId, chat.connectState, chat.curChannelUsers.length] as const,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => chat.presenceMap,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
  { deep: true },
);

watch(
  () => chat.unreadCountMap,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
  { deep: true },
);

watch(
  () => chat.filterState,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
  { deep: true },
);

watch(
  () => [currentIdentityId.value, currentIdentityVariantId.value] as const,
  ([identityId, variantId], previous) => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
    if (!theaterMode.value || !theaterBridgeInitialized) return;
    const [previousIdentityId, previousVariantId] = previous || ['', ''];
    if (identityId !== previousIdentityId && identityId) {
      queueTheaterCharacterPublish('selected', identityId, variantId);
    } else if (identityId && variantId !== previousVariantId) {
      queueTheaterCharacterPublish('variant', identityId, variantId);
    }
  },
);

watch(
  () => {
    const channelId = String(chat.curChannel?.id || '').trim();
    if (!channelId) return '';
    return JSON.stringify({
      identities: toRaw(chat.channelIdentities[channelId] || []),
      variants: toRaw(chat.channelIdentityVariants[channelId] || {}),
    });
  },
  () => {
    if (!theaterMode.value || !theaterBridgeInitialized) return;
    queueTheaterCharacterPublish('appearance', currentIdentityId.value || null);
  },
);

watch(
  () => chat.filterState.icFilter,
  (filter) => {
    chat.setChannelSessionRestoreFilterOverride(filter);
  },
  { immediate: true },
);

watch(
  () => chat.channelTree,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
  { deep: true },
);

watch(
  () => chat.worldDetailMap[chat.currentWorldId]?.memberRole,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => channelSearch.panelVisible,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => [iFormButtonActive.value, iFormHasAttention.value] as const,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => audioStudio.drawerVisible,
  () => {
    if (restoringSession.value) return;
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  audioOwner,
  (enabled) => {
    audioStudio.setPlaybackAuthority(enabled);
    if (enabled) {
      syncAudioStudioContext();
    }
  },
  { immediate: true },
);

watch(
  () => chat.curChannel?.id,
  (channelId) => {
    if (!audioOwner.value) return;
    audioStudio.setActiveChannel(channelId ? String(channelId) : null);
  },
  { immediate: true },
);

watch(
  () => chat.currentWorldId,
  (worldId) => {
    if (!audioOwner.value) return;
    audioStudio.setCurrentWorld(worldId || null);
  },
  { immediate: true },
);

watch(theaterSessionId, (sessionId, previousSessionId) => {
  if (previousSessionId && sessionId !== previousSessionId) stopTheaterBridge();
});

watch(
  () => [initialWorldId.value, initialChannelId.value] as const,
  ([worldId, channelId], previous) => {
    if (!previous || (worldId === previous[0] && channelId === previous[1])) return;
    stopTheaterBridge();
    void startTheaterBridge().catch((error) => {
      console.warn('[theater-bridge] chat context restart failed', error);
      stopTheaterBridge();
    });
  },
);

onMounted(async () => {
  chatEvent.on('channel-switch-to' as any, handleTheaterChannelSwitch as any);
  forwardTheaterAppearanceInvalidation = (event: Event) => {
    if (!theaterMode.value || window.parent === window) return;
    const detail = (event as CustomEvent<{ channelId?: string; targetUserId?: string }>).detail || {};
    window.parent.postMessage({
      type: 'sealchat.theater.appearance.invalidated',
      sessionId: theaterSessionId.value,
      channelId: detail.channelId || '',
      targetUserId: detail.targetUserId || '',
    }, window.location.origin);
  };
  window.addEventListener('sealchat:theater-appearance-invalidated', forwardTheaterAppearanceInvalidation);
  window.addEventListener('message', handleMessage);
  document.addEventListener('pointerdown', handleInteraction, { capture: true });
  document.addEventListener('keydown', handleInteraction, { capture: true });
  await initialize();
  try {
    await startTheaterBridge();
  } catch (error) {
    console.warn('[theater-bridge] chat startup failed', error);
    stopTheaterBridge();
  }
});

onBeforeUnmount(() => {
  chatEvent.off('channel-switch-to' as any, handleTheaterChannelSwitch as any);
  stopTheaterBridge();
  chat.setChannelSessionRestoreFilterOverride('all');
  window.removeEventListener('message', handleMessage);
  if (forwardTheaterAppearanceInvalidation) {
    window.removeEventListener('sealchat:theater-appearance-invalidated', forwardTheaterAppearanceInvalidation);
  }
  document.removeEventListener('pointerdown', handleInteraction, { capture: true } as any);
  document.removeEventListener('keydown', handleInteraction, { capture: true } as any);
});
</script>

<template>
  <div class="sc-embed-root">
    <ChatHeader
      v-if="theaterMode"
      :sidebar-collapsed="true"
      @toggle-sidebar="handleTheaterHeaderSidebarToggle"
    />
    <Chat ref="chatViewRef" class="sc-embed-chat" @drawer-show="handleDrawerShow" />
    <AudioDrawer />
    <n-drawer v-model:show="theaterSidebarVisible" placement="left" :width="'min(360px, 88vw)'">
      <n-drawer-content closable body-content-style="padding: 0">
        <template #header>频道选择</template>
        <ChatSidebar />
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.sc-embed-root {
  height: 100vh;
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  background: var(--sc-bg-page);
}

:global(html),
:global(body),
:global(#app) {
  width: 100%;
  margin: 0;
  padding: 0;
  overflow: hidden;
  background: var(--sc-bg-page);
}

.sc-embed-chat {
  min-height: 0;
  flex: 1;
  height: auto !important;
}
</style>
