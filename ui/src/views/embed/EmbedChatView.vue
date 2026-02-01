<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, toRaw } from 'vue';
import { useRoute } from 'vue-router';
import { throttle } from 'lodash-es';
import Chat from '@/views/chat/chat.vue';
import { useChatStore } from '@/stores/chat';
import { useChannelSearchStore } from '@/stores/channelSearch';
import { usePushNotificationStore } from '@/stores/pushNotification';
import { useIFormStore } from '@/stores/iform';
import { useAudioStudioStore } from '@/stores/audioStudio';
import AudioDrawer from '@/components/audio/AudioDrawer.vue';

type PaneId = 'A' | 'B';
type ConnectState = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

type FilterState = {
  icFilter: 'all' | 'ic' | 'ooc';
  showArchived: boolean;
  roleIds: string[];
};

type RoleOption = { id: string; label?: string; name?: string };

type SplitChannelNode = {
  id: string;
  name: string;
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
const initialNotifyOwner = computed(() => (route.query.notifyOwner === '1' || route.query.notifyOwner === 'true'));
const initialAudioOwner = computed(() => {
  if (route.query.audioOwner === undefined) return true;
  return route.query.audioOwner === '1' || route.query.audioOwner === 'true';
});

const chatViewRef = ref<any>(null);
const initializing = ref(false);
const roleOptions = ref<RoleOption[]>([]);
const audioOwner = ref(initialAudioOwner.value);

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
        const unread = typeof item?.unread === 'number' ? item.unread : Number(item?.unread || 0);
        const children = walk(item?.children || []);
        if (!id) return null;
        return { id, name, unread: Number.isFinite(unread) ? unread : 0, children };
      })
      .filter(Boolean) as SplitChannelNode[];
  };
  return walk(raw);
};

const postState = (type: 'sealchat.embed.ready' | 'sealchat.embed.state') => {
  if (!paneId.value) return;
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const channelName = typeof chat.curChannel?.name === 'string' ? chat.curChannel?.name : '';
  const worldId = chat.currentWorldId || '';
  const worldName = chat.currentWorld?.name || '';
  const connectState = (chat.connectState || 'connecting') as ConnectState;
  const onlineMembersCount = Array.isArray(chat.curChannelUsers) ? chat.curChannelUsers.length : 0;
  const currentChannelUnread = channelId ? (chat.unreadCountMap?.[channelId] || 0) : 0;

  postToParent({
    type,
    paneId: paneId.value,
    worldId,
    worldName,
    // 注意：postMessage 需要结构化克隆，避免直接传递 Vue reactive/proxy（会触发 DataCloneError）
    worldOptions: normalizeWorldOptions(toRaw(chat.joinedWorldOptions)),
    channelId,
    channelName,
    connectState,
    onlineMembersCount,
    currentChannelUnread,
    audioStudioDrawerVisible: !!audioStudio.drawerVisible,
    filterState: normalizeFilterState(toRaw(chat.filterState)),
    roleOptions: normalizeRoleOptions(toRaw(roleOptions.value)),
    canImport: isOwnerOrAdmin.value,
    channelTree: normalizeChannelTree(buildChannelTree()),
    searchPanelVisible: !!channelSearch.panelVisible,
    iFormButtonActive: !!iFormButtonActive.value,
    iFormHasAttention: !!iFormHasAttention.value,
  });
};

const postStateThrottled = throttle((type: 'sealchat.embed.ready' | 'sealchat.embed.state') => postState(type), 200, {
  leading: true,
  trailing: true,
});

const syncAudioStudioContext = () => {
  if (!audioOwner.value) return;
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  audioStudio.setActiveChannel(channelId || null);
  audioStudio.setCurrentWorld(chat.currentWorldId || null);
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
    if (enabled) {
      syncAudioStudioContext();
    } else {
      audioStudio.setActiveChannel(null);
    }
    postStateThrottled('sealchat.embed.state');
    return;
  }

  if (data.type === 'sealchat.embed.setFilterState') {
    if (data.filterState) {
      chat.setFilterState(data.filterState);
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
    postStateThrottled('sealchat.embed.state');
  },
);

watch(
  () => [chat.currentWorldId, chat.connectState, chat.curChannelUsers.length] as const,
  () => postStateThrottled('sealchat.embed.state'),
);

watch(
  () => chat.unreadCountMap,
  () => postStateThrottled('sealchat.embed.state'),
  { deep: true },
);

watch(
  () => chat.filterState,
  () => postStateThrottled('sealchat.embed.state'),
  { deep: true },
);

watch(
  () => chat.channelTree,
  () => postStateThrottled('sealchat.embed.state'),
  { deep: true },
);

watch(
  () => chat.worldDetailMap[chat.currentWorldId]?.memberRole,
  () => postStateThrottled('sealchat.embed.state'),
);

watch(
  () => channelSearch.panelVisible,
  () => postStateThrottled('sealchat.embed.state'),
);

watch(
  () => [iFormButtonActive.value, iFormHasAttention.value] as const,
  () => postStateThrottled('sealchat.embed.state'),
);

watch(
  () => audioStudio.drawerVisible,
  () => postStateThrottled('sealchat.embed.state'),
);

watch(
  audioOwner,
  (enabled) => {
    if (enabled) {
      syncAudioStudioContext();
    } else {
      audioStudio.setActiveChannel(null);
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

onMounted(() => {
  initialize();
  window.addEventListener('message', handleMessage);
  document.addEventListener('pointerdown', handleInteraction, { capture: true });
  document.addEventListener('keydown', handleInteraction, { capture: true });
});

onBeforeUnmount(() => {
  window.removeEventListener('message', handleMessage);
  document.removeEventListener('pointerdown', handleInteraction, { capture: true } as any);
  document.removeEventListener('keydown', handleInteraction, { capture: true } as any);
});
</script>

<template>
  <div class="sc-embed-root">
    <Chat ref="chatViewRef" @drawer-show="handleDrawerShow" />
    <AudioDrawer />
  </div>
</template>

<style scoped>
.sc-embed-root {
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}
</style>
