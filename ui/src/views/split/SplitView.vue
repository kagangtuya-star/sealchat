<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, toRaw, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NDrawer, NDrawerContent } from 'naive-ui';
import { useWindowSize } from '@vueuse/core';
import ChatActionRibbon from '@/views/chat/components/ChatActionRibbon.vue';
import SplitHeader from '@/views/split/components/SplitHeader.vue';
import SplitChannelSidebar, { type PaneId, type SplitChannelNode } from '@/views/split/components/SplitChannelSidebar.vue';
import { setChannelTitle } from '@/stores/utils';
import { useChatStore } from '@/stores/chat';

type ConnectState = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';
type PaneMode = 'chat' | 'web';

type FilterState = {
  icFilter: 'all' | 'ic' | 'ooc';
  showArchived: boolean;
  roleIds: string[];
};

type RoleOption = { id: string; label?: string; name?: string };

type EmbedStateMessage = {
  type: 'sealchat.embed.state' | 'sealchat.embed.ready';
  paneId: PaneId;
  worldId?: string;
  worldName?: string;
  worldOptions?: Array<{ value: string; label: string }>;
  channelId?: string;
  channelName?: string;
  connectState?: ConnectState;
  onlineMembersCount?: number;
  currentChannelUnread?: number;
  audioStudioDrawerVisible?: boolean;
  filterState?: FilterState;
  roleOptions?: RoleOption[];
  canImport?: boolean;
  channelTree?: SplitChannelNode[];
  searchPanelVisible?: boolean;
  iFormButtonActive?: boolean;
  iFormHasAttention?: boolean;
};

type EmbedFocusMessage = {
  type: 'sealchat.embed.focus';
  paneId: PaneId;
};

type EmbedToggleSidebarMessage = {
  type: 'sealchat.embed.requestToggleSidebar';
  paneId: PaneId;
};

type EmbedMessage = EmbedStateMessage | EmbedFocusMessage | EmbedToggleSidebarMessage;

interface PaneState {
  id: PaneId;
  iframeKey: number;
  src: string;
  ready: boolean;
  mode: PaneMode;
  webUrl: string;
  worldId: string;
  worldName: string;
  worldOptions: Array<{ value: string; label: string }>;
  channelId: string;
  channelName: string;
  connectState: ConnectState;
  onlineMembersCount: number;
  currentChannelUnread: number;
  audioStudioDrawerVisible: boolean;
  filterState: FilterState;
  roleOptions: RoleOption[];
  canImport: boolean;
  channelTree: SplitChannelNode[];
  searchPanelVisible: boolean;
  embedPanelActive: boolean;
  embedPanelHasAttention: boolean;
  notifyOwner: boolean;
}

type OperationTarget = 'follow' | PaneId;

const router = useRouter();
const route = useRoute();
// 分屏壳页面自身不需要 WS：关闭主实例连接，避免占用连接数导致 iframe embed 连接失败
useChatStore().disconnect('split-shell');
const { width } = useWindowSize();

const isMobileViewport = computed(() => width.value < 700);

const activePaneId = ref<PaneId>('A');
const operationTarget = ref<OperationTarget>('follow');
const audioPlaybackTarget = ref<OperationTarget>('follow');
const lockSameWorld = ref(false);
const notifyOwnerPaneId = ref<PaneId | null>(null);
const webTargetPaneId = ref<PaneId>('A');

const actionRibbonVisible = ref(false);

const drawerVisible = ref(false);
const sidebarCollapsed = ref(false);
const computedCollapsed = computed(() => isMobileViewport.value || sidebarCollapsed.value);

const defaultFilterState: FilterState = { icFilter: 'all', showArchived: false, roleIds: [] };
const normalizeFilterState = (filters: FilterState): FilterState => {
  const rawRoleIds = Array.isArray(filters.roleIds) ? toRaw(filters.roleIds) : [];
  const roleIds = Array.isArray(rawRoleIds) ? rawRoleIds.map((id) => String(id ?? '')).filter(Boolean) : [];
  const icFilter = ['all', 'ic', 'ooc'].includes(filters.icFilter) ? filters.icFilter : 'all';
  return {
    icFilter,
    showArchived: !!filters.showArchived,
    roleIds,
  };
};
const storagePrefix = 'sealchat.split.pane';
const paneStorageKey = (paneId: PaneId, key: 'mode' | 'url') => `${storagePrefix}.${paneId}.${key}`;
const normalizeUrl = (value: string) => value.trim();
const isHttpUrl = (value: string) => {
  try {
    const url = new URL(value);
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
};
const getWebHost = (value: string) => {
  if (!isHttpUrl(value)) return '';
  try {
    return new URL(value).host;
  } catch {
    return '';
  }
};
const loadPaneStorage = (pane: PaneState) => {
  if (typeof window === 'undefined') return;
  const mode = window.localStorage.getItem(paneStorageKey(pane.id, 'mode'));
  if (mode === 'chat' || mode === 'web') {
    pane.mode = mode;
  }
  const url = window.localStorage.getItem(paneStorageKey(pane.id, 'url'));
  if (typeof url === 'string') {
    pane.webUrl = url;
  }
};
const persistPaneStorage = (pane: PaneState) => {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(paneStorageKey(pane.id, 'mode'), pane.mode);
  window.localStorage.setItem(paneStorageKey(pane.id, 'url'), pane.webUrl);
};

const splitContainerRef = ref<HTMLElement | null>(null);
const splitRatio = ref(0.5);
const splitDragging = ref(false);

const paneA = reactive<PaneState>({
  id: 'A',
  iframeKey: 0,
  src: '',
  ready: false,
  mode: 'chat',
  webUrl: '',
  worldId: typeof route.query.worldId === 'string' ? route.query.worldId : '',
  worldName: '',
  worldOptions: [],
  channelId: typeof route.query.a === 'string' ? route.query.a : '',
  channelName: '',
  connectState: 'connecting',
  onlineMembersCount: 0,
  currentChannelUnread: 0,
  audioStudioDrawerVisible: false,
  filterState: { ...defaultFilterState },
  roleOptions: [],
  canImport: false,
  channelTree: [],
  searchPanelVisible: false,
  embedPanelActive: false,
  embedPanelHasAttention: false,
  notifyOwner: false,
});

const paneB = reactive<PaneState>({
  id: 'B',
  iframeKey: 0,
  src: '',
  ready: false,
  mode: 'chat',
  webUrl: '',
  worldId: typeof route.query.worldId === 'string' ? route.query.worldId : '',
  worldName: '',
  worldOptions: [],
  channelId: typeof route.query.b === 'string' ? route.query.b : '',
  channelName: '',
  connectState: 'connecting',
  onlineMembersCount: 0,
  currentChannelUnread: 0,
  audioStudioDrawerVisible: false,
  filterState: { ...defaultFilterState },
  roleOptions: [],
  canImport: false,
  channelTree: [],
  searchPanelVisible: false,
  embedPanelActive: false,
  embedPanelHasAttention: false,
  notifyOwner: false,
});

const panes = computed(() => [paneA, paneB]);
const getPaneById = (paneId: PaneId) => (paneId === 'A' ? paneA : paneB);
const activePane = computed(() => (activePaneId.value === 'A' ? paneA : paneB));
const inactivePane = computed(() => (activePaneId.value === 'A' ? paneB : paneA));
const effectiveTargetPaneId = computed<PaneId>(() => (operationTarget.value === 'follow' ? activePaneId.value : operationTarget.value));
const operationPane = computed(() => (effectiveTargetPaneId.value === 'A' ? paneA : paneB));
const effectiveAudioPaneId = computed<PaneId>(() => (audioPlaybackTarget.value === 'follow' ? activePaneId.value : audioPlaybackTarget.value));
const activePaneHasChannel = computed(() => activePane.value.mode === 'chat' && !!activePane.value.channelId);

const activeChannelTitle = computed(() => {
  if (activePane.value.mode === 'web') {
    const host = getWebHost(normalizeUrl(activePane.value.webUrl));
    return host ? `网页分屏 · ${host}` : '网页分屏';
  }
  const raw = activePane.value.channelName || '';
  const name = raw.trim();
  return name ? `# ${name}` : '海豹尬聊 SealChat';
});

const activePaneConnectState = computed<ConnectState>(() => (activePane.value.mode === 'chat' ? activePane.value.connectState : 'disconnected'));
const activePaneOnlineMembersCount = computed(() => (activePane.value.mode === 'chat' ? activePane.value.onlineMembersCount : 0));
const activePaneAudioStudioActive = computed(() => activePane.value.mode === 'chat' && activePane.value.audioStudioDrawerVisible);
const activePaneSearchActive = computed(() => activePane.value.mode === 'chat' && activePane.value.searchPanelVisible);
const activePaneEmbedPanelActive = computed(() => activePane.value.mode === 'chat' && activePane.value.embedPanelActive);
const activePaneEmbedPanelHasAttention = computed(() => activePane.value.mode === 'chat' && activePane.value.embedPanelHasAttention);

watch(
  () => [activePaneId.value, activePane.value.channelName] as const,
  () => {
    setChannelTitle(activePane.value.channelName || '');
  },
  { immediate: true },
);


const buildEmbedSrc = (pane: PaneState) => {
  const params = new URLSearchParams();
  params.set('paneId', pane.id);
  if (pane.worldId) params.set('worldId', pane.worldId);
  if (pane.channelId) params.set('channelId', pane.channelId);
  if (notifyOwnerPaneId.value === pane.id) params.set('notifyOwner', '1');
  params.set('audioOwner', effectiveAudioPaneId.value === pane.id ? '1' : '0');
  // 不能直接用 import.meta.env.BASE_URL（vite base='./' 时会变成 ./#/embed，导致 iframe 请求落到 /app/ 而非 /app/index.html）
  // 这里用当前页面的 pathname+search，确保与 split 同一份 HTML（支持 /index.html#/split、/subdir/#/split 等部署形态）
  const base = typeof window === 'undefined' ? '/' : `${window.location.pathname}${window.location.search}`;
  return `${base}#/embed?${params.toString()}`;
};

const buildPaneSrc = (pane: PaneState) => {
  if (pane.mode === 'web') {
    const url = normalizeUrl(pane.webUrl);
    return isHttpUrl(url) ? url : 'about:blank';
  }
  return buildEmbedSrc(pane);
};

const refreshPaneSrc = (pane: PaneState) => {
  pane.ready = pane.mode !== 'chat';
  pane.src = buildPaneSrc(pane);
  pane.iframeKey += 1;
};

const persistRouteQuery = () => {
  router.replace({
    name: 'split',
    query: {
      worldId: activePane.value.worldId || '',
      a: paneA.channelId || '',
      b: paneB.channelId || '',
    },
  });
};

const getPaneIframe = (paneId: PaneId) => document.getElementById(`sc-split-iframe-${paneId}`) as HTMLIFrameElement | null;

const postToPane = (paneId: PaneId, payload: any) => {
  const pane = getPaneById(paneId);
  if (pane.mode !== 'chat') return false;
  const iframe = getPaneIframe(paneId);
  const targetWindow = iframe?.contentWindow;
  if (!targetWindow) return false;
  targetWindow.postMessage(payload, window.location.origin);
  return true;
};

const ensureWorldAlignment = async (sourcePaneId: PaneId) => {
  if (!lockSameWorld.value) return;
  const source = sourcePaneId === 'A' ? paneA : paneB;
  const target = sourcePaneId === 'A' ? paneB : paneA;
  const worldId = source.worldId;
  if (!worldId || target.worldId === worldId) return;
  postToPane(target.id, { type: 'sealchat.embed.setWorld', paneId: target.id, worldId });
};

const handleEmbedMessage = (event: MessageEvent) => {
  if (event.origin !== window.location.origin) return;
  const data = event.data as EmbedMessage | undefined;
  if (!data || typeof data !== 'object') return;
  const type = (data as any).type;
  if (typeof type !== 'string') return;

  if (type === 'sealchat.embed.focus') {
    const paneId = (data as EmbedFocusMessage).paneId;
    if (paneId === 'A' || paneId === 'B') {
      activePaneId.value = paneId;
    }
    return;
  }

  if (type === 'sealchat.embed.requestToggleSidebar') {
    if (isMobileViewport.value) {
      drawerVisible.value = !drawerVisible.value;
    } else {
      sidebarCollapsed.value = !sidebarCollapsed.value;
    }
    return;
  }

  if (type === 'sealchat.embed.state' || type === 'sealchat.embed.ready') {
    const msg = data as EmbedStateMessage;
    const target = msg.paneId === 'A' ? paneA : msg.paneId === 'B' ? paneB : null;
    if (!target) return;
    if (target.mode !== 'chat') return;
    target.ready = true;
    if (typeof msg.worldId === 'string') target.worldId = msg.worldId;
    if (typeof msg.worldName === 'string') target.worldName = msg.worldName;
    if (Array.isArray(msg.worldOptions)) target.worldOptions = msg.worldOptions;
    if (typeof msg.channelId === 'string') target.channelId = msg.channelId;
    if (typeof msg.channelName === 'string') target.channelName = msg.channelName;
    if (typeof msg.connectState === 'string') target.connectState = msg.connectState;
    if (typeof msg.onlineMembersCount === 'number') target.onlineMembersCount = msg.onlineMembersCount;
    if (typeof msg.currentChannelUnread === 'number') target.currentChannelUnread = msg.currentChannelUnread;
    if (typeof msg.audioStudioDrawerVisible === 'boolean') target.audioStudioDrawerVisible = msg.audioStudioDrawerVisible;
    if (msg.filterState) target.filterState = { ...msg.filterState };
    if (Array.isArray(msg.roleOptions)) target.roleOptions = msg.roleOptions;
    if (typeof msg.canImport === 'boolean') target.canImport = msg.canImport;
    if (Array.isArray(msg.channelTree)) target.channelTree = msg.channelTree;
    if (typeof msg.searchPanelVisible === 'boolean') target.searchPanelVisible = msg.searchPanelVisible;
    if (typeof msg.iFormButtonActive === 'boolean') target.embedPanelActive = msg.iFormButtonActive;
    if (typeof msg.iFormHasAttention === 'boolean') target.embedPanelHasAttention = msg.iFormHasAttention;

    persistRouteQuery();
    ensureWorldAlignment(target.id);
  }
};

const initialize = () => {
  loadPaneStorage(paneA);
  loadPaneStorage(paneB);
  refreshPaneSrc(paneA);
  refreshPaneSrc(paneB);
};

const setPaneMode = (paneId: PaneId, mode: PaneMode) => {
  const pane = getPaneById(paneId);
  if (pane.mode === mode) return;
  pane.mode = mode;
  persistPaneStorage(pane);
  refreshPaneSrc(pane);
};

const setPaneWebUrl = (paneId: PaneId, url: string) => {
  const pane = getPaneById(paneId);
  const normalized = normalizeUrl(url);
  if (pane.webUrl === normalized) return;
  pane.webUrl = normalized;
  persistPaneStorage(pane);
  if (pane.mode === 'web') {
    refreshPaneSrc(pane);
  }
};

const isPaneWebUrlInvalid = (pane: PaneState) => pane.mode === 'web' && !isHttpUrl(normalizeUrl(pane.webUrl));
const canOperateChatPane = (paneId: PaneId) => getPaneById(paneId).mode === 'chat';

const setActivePane = (paneId: PaneId) => {
  activePaneId.value = paneId;
};

const setWorldForTargetPane = (worldId: string) => {
  const normalized = typeof worldId === 'string' ? worldId.trim() : '';
  if (!normalized) return;
  const targetPaneId = effectiveTargetPaneId.value;
  if (!canOperateChatPane(targetPaneId)) return;
  postToPane(targetPaneId, { type: 'sealchat.embed.setWorld', paneId: targetPaneId, worldId: normalized });
  setActivePane(targetPaneId);
  if (lockSameWorld.value) {
    const other: PaneId = targetPaneId === 'A' ? 'B' : 'A';
    postToPane(other, { type: 'sealchat.embed.setWorld', paneId: other, worldId: normalized });
  }
};

const setNotifyOwner = (paneId: PaneId | null) => {
  notifyOwnerPaneId.value = paneId;
  postToPane('A', { type: 'sealchat.embed.setNotifyOwner', paneId: 'A', enabled: paneId === 'A' });
  postToPane('B', { type: 'sealchat.embed.setNotifyOwner', paneId: 'B', enabled: paneId === 'B' });
};

const syncAudioOwner = () => {
  const owner = effectiveAudioPaneId.value;
  postToPane('A', { type: 'sealchat.embed.setAudioOwner', paneId: 'A', enabled: owner === 'A' });
  postToPane('B', { type: 'sealchat.embed.setAudioOwner', paneId: 'B', enabled: owner === 'B' });
};

watch(
  () => effectiveAudioPaneId.value,
  () => {
    syncAudioOwner();
  },
  { immediate: true },
);

const toggleLockSameWorld = (enabled: boolean) => {
  lockSameWorld.value = enabled;
  if (enabled) {
    ensureWorldAlignment(activePaneId.value);
  }
};

const openChannelInTargetPane = (channelId: string) => {
  if (!channelId) return;
  const worldId = operationPane.value.worldId;
  if (!worldId) return;
  const targetPaneId = effectiveTargetPaneId.value;
  if (!canOperateChatPane(targetPaneId)) return;
  postToPane(targetPaneId, { type: 'sealchat.embed.setWorld', paneId: targetPaneId, worldId, channelId });
  setActivePane(targetPaneId);

  if (lockSameWorld.value) {
    const other: PaneId = targetPaneId === 'A' ? 'B' : 'A';
    postToPane(other, { type: 'sealchat.embed.setWorld', paneId: other, worldId });
  }
};

const swapPanes = () => {
  if (!canOperateChatPane('A') || !canOperateChatPane('B')) return;
  const a = paneA.channelId;
  const aw = paneA.worldId;
  const b = paneB.channelId;
  const bw = paneB.worldId;
  postToPane('A', { type: 'sealchat.embed.setWorld', paneId: 'A', worldId: bw, channelId: b });
  postToPane('B', { type: 'sealchat.embed.setWorld', paneId: 'B', worldId: aw, channelId: a });
};

const toggleSearch = () => {
  if (!canOperateChatPane(activePaneId.value)) return;
  postToPane(activePaneId.value, { type: 'sealchat.embed.openPanel', paneId: activePaneId.value, panel: 'search' });
};

const openAudioStudio = () => {
  const targetPaneId = effectiveAudioPaneId.value;
  if (!canOperateChatPane(targetPaneId)) return;
  postToPane(targetPaneId, { type: 'sealchat.embed.openAudioStudio', paneId: targetPaneId });
};

const openEmbedPanel = () => {
  if (!activePaneHasChannel.value) return;
  if (!canOperateChatPane(activePaneId.value)) return;
  postToPane(activePaneId.value, { type: 'sealchat.embed.openIFormDrawer', paneId: activePaneId.value });
};

const toggleActionRibbon = () => {
  actionRibbonVisible.value = !actionRibbonVisible.value;
};


const setFilters = (filters: FilterState) => {
  if (!canOperateChatPane(activePaneId.value)) return;
  const normalized = normalizeFilterState(filters);
  postToPane(activePaneId.value, { type: 'sealchat.embed.setFilterState', paneId: activePaneId.value, filterState: normalized });
};

const clearFilters = () => {
  setFilters({ ...defaultFilterState });
};

const openPanel = (panel: string) => {
  if (!canOperateChatPane(activePaneId.value)) return;
  postToPane(activePaneId.value, { type: 'sealchat.embed.openPanel', paneId: activePaneId.value, panel });
};

const exitSplit = async () => {
  await router.push({ name: 'home' });
};

const clampSplitRatio = (ratio: number) => Math.min(0.85, Math.max(0.15, ratio));

const updateSplitRatioFromClientX = (clientX: number) => {
  const el = splitContainerRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  if (!rect.width) return;
  const ratio = (clientX - rect.left) / rect.width;
  splitRatio.value = clampSplitRatio(ratio);
};

const handleSplitDividerPointerDown = (event: PointerEvent) => {
  if (event.button !== 0) return;
  splitDragging.value = true;
  try {
    (event.currentTarget as HTMLElement | null)?.setPointerCapture?.(event.pointerId);
  } catch {
    // ignore
  }
  updateSplitRatioFromClientX(event.clientX);
};

const handleSplitDividerPointerMove = (event: PointerEvent) => {
  if (!splitDragging.value) return;
  updateSplitRatioFromClientX(event.clientX);
};

const handleSplitDividerPointerUp = (event: PointerEvent) => {
  if (!splitDragging.value) return;
  splitDragging.value = false;
  try {
    (event.currentTarget as HTMLElement | null)?.releasePointerCapture?.(event.pointerId);
  } catch {
    // ignore
  }
};

const handleSplitDividerPointerCancel = (event: PointerEvent) => {
  if (!splitDragging.value) return;
  splitDragging.value = false;
  try {
    (event.currentTarget as HTMLElement | null)?.releasePointerCapture?.(event.pointerId);
  } catch {
    // ignore
  }
};

onMounted(() => {
  initialize();
  window.addEventListener('message', handleEmbedMessage);
});

onBeforeUnmount(() => {
  window.removeEventListener('message', handleEmbedMessage);
});

const collapsedWidth = computed(() => 0);
</script>

<template>
  <main class="h-screen sc-app-shell">
    <n-layout-header class="sc-layout-header">
      <SplitHeader
        :sidebar-collapsed="computedCollapsed"
        :channel-title="activeChannelTitle"
        :connect-state="activePaneConnectState"
        :online-members-count="activePaneOnlineMembersCount"
        :audio-studio-active="activePaneAudioStudioActive"
        :search-active="activePaneSearchActive"
        :embed-panel-active="activePaneEmbedPanelActive"
        :embed-panel-has-attention="activePaneEmbedPanelHasAttention"
        :embed-panel-disabled="!activePaneHasChannel"
        :action-ribbon-active="actionRibbonVisible"
        @toggle-sidebar="isMobileViewport ? (drawerVisible = !drawerVisible) : (sidebarCollapsed = !sidebarCollapsed)"
        @open-audio-studio="openAudioStudio"
        @toggle-search="toggleSearch"
        @open-embed-panel="openEmbedPanel"
        @toggle-action-ribbon="toggleActionRibbon"
      />
    </n-layout-header>

    <n-layout class="sc-layout-root" has-sider position="absolute" style="margin-top: 3.5rem;">
  <n-layout-sider
        class="sc-layout-sider"
        collapse-mode="width"
        :collapsed="computedCollapsed"
        :collapsed-width="collapsedWidth"
        :native-scrollbar="false"
      >
        <SplitChannelSidebar
          :active-pane-id="activePaneId"
          :panes="[
            { id: 'A', channelName: paneA.channelName, unread: paneA.currentChannelUnread, worldName: paneA.worldName },
            { id: 'B', channelName: paneB.channelName, unread: paneB.currentChannelUnread, worldName: paneB.worldName },
          ]"
          :world-id="operationPane.worldId"
          :world-options="operationPane.worldOptions"
          :lock-same-world="lockSameWorld"
          :notify-owner-pane-id="notifyOwnerPaneId"
          :operation-target="operationTarget"
          :audio-playback-target="audioPlaybackTarget"
          :world-name="operationPane.worldName"
          :web-target-pane-id="webTargetPaneId"
          :pane-modes="{ A: paneA.mode, B: paneB.mode }"
          :pane-web-urls="{ A: paneA.webUrl, B: paneB.webUrl }"
          :channel-tree="operationPane.channelTree"
          @set-active-pane="setActivePane"
          @set-operation-target="operationTarget = $event"
          @set-audio-playback-target="audioPlaybackTarget = $event"
          @set-world="setWorldForTargetPane"
          @toggle-lock-same-world="toggleLockSameWorld"
          @set-notify-owner="setNotifyOwner"
          @open-channel="openChannelInTargetPane"
          @set-web-target="webTargetPaneId = $event"
          @set-pane-mode="setPaneMode"
          @set-pane-url="setPaneWebUrl"
          @swap-panes="swapPanes"
          @exit-split="exitSplit"
        />
      </n-layout-sider>

      <n-layout class="sc-layout-content">
        <div class="sc-split-content">
          <div v-if="actionRibbonVisible" class="px-4 pt-4">
            <ChatActionRibbon
              :filters="activePane.filterState"
              :roles="activePane.roleOptions"
              :archive-active="false"
              :export-active="false"
              :identity-active="false"
              :gallery-active="false"
              :display-active="false"
              :favorite-active="false"
              :channel-images-active="false"
              :can-import="activePane.canImport"
              :import-active="false"
              :split-enabled="false"
              :split-active="false"
              :sticky-note-enabled="false"
              @update:filters="setFilters"
              @clear-filters="clearFilters"
              @open-archive="openPanel('archive')"
              @open-export="openPanel('export')"
              @open-import="openPanel('import')"
              @open-identity-manager="openPanel('identity')"
              @open-gallery="openPanel('gallery')"
              @open-display-settings="openPanel('display')"
              @open-favorites="openPanel('favorites')"
              @open-channel-images="openPanel('channel-images')"
            />
          </div>

          <div
            ref="splitContainerRef"
            class="sc-split-panes"
            :class="{ 'is-dragging': splitDragging }"
            :style="{
              '--sc-split-ratio': String(splitRatio),
            }"
          >
            <div class="sc-split-pane" :style="{ width: `${splitRatio * 100}%` }">
              <div class="sc-split-pane__frame">
                <iframe
                  :id="`sc-split-iframe-A`"
                  :key="paneA.iframeKey"
                  class="sc-split-iframe"
                  :src="paneA.src"
                  frameborder="0"
                />
                <div v-if="isPaneWebUrlInvalid(paneA)" class="sc-split-web-placeholder">
                  <div class="sc-split-web-placeholder__title">请输入 http/https 网址</div>
                  <div class="sc-split-web-placeholder__desc">部分站点禁止 iframe 嵌入，会显示空白。</div>
                </div>
              </div>
            </div>

            <div
              class="sc-split-divider"
              role="separator"
              aria-label="调整分屏大小"
              tabindex="0"
              @pointerdown="handleSplitDividerPointerDown"
              @pointermove="handleSplitDividerPointerMove"
              @pointerup="handleSplitDividerPointerUp"
              @pointercancel="handleSplitDividerPointerCancel"
            />

            <div class="sc-split-pane" :style="{ width: `${(1 - splitRatio) * 100}%` }">
              <div class="sc-split-pane__frame">
                <iframe
                  :id="`sc-split-iframe-B`"
                  :key="paneB.iframeKey"
                  class="sc-split-iframe"
                  :src="paneB.src"
                  frameborder="0"
                />
                <div v-if="isPaneWebUrlInvalid(paneB)" class="sc-split-web-placeholder">
                  <div class="sc-split-web-placeholder__title">请输入 http/https 网址</div>
                  <div class="sc-split-web-placeholder__desc">部分站点禁止 iframe 嵌入，会显示空白。</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <n-drawer v-model:show="drawerVisible" :width="'80%'" placement="left">
          <n-drawer-content closable body-content-style="padding: 0">
            <template #header>频道</template>
            <SplitChannelSidebar
              :active-pane-id="activePaneId"
              :panes="[
                { id: 'A', channelName: paneA.channelName, unread: paneA.currentChannelUnread, worldName: paneA.worldName },
                { id: 'B', channelName: paneB.channelName, unread: paneB.currentChannelUnread, worldName: paneB.worldName },
              ]"
              :world-id="operationPane.worldId"
              :world-options="operationPane.worldOptions"
              :lock-same-world="lockSameWorld"
              :notify-owner-pane-id="notifyOwnerPaneId"
              :operation-target="operationTarget"
              :audio-playback-target="audioPlaybackTarget"
              :world-name="operationPane.worldName"
              :web-target-pane-id="webTargetPaneId"
              :pane-modes="{ A: paneA.mode, B: paneB.mode }"
              :pane-web-urls="{ A: paneA.webUrl, B: paneB.webUrl }"
              :channel-tree="operationPane.channelTree"
              @set-active-pane="setActivePane"
              @set-operation-target="operationTarget = $event"
              @set-audio-playback-target="audioPlaybackTarget = $event"
              @set-world="setWorldForTargetPane"
              @toggle-lock-same-world="toggleLockSameWorld"
              @set-notify-owner="setNotifyOwner"
              @open-channel="(id) => { openChannelInTargetPane(id); drawerVisible = false; }"
              @set-web-target="webTargetPaneId = $event"
              @set-pane-mode="setPaneMode"
              @set-pane-url="setPaneWebUrl"
              @swap-panes="swapPanes"
              @exit-split="exitSplit"
            />
          </n-drawer-content>
        </n-drawer>

      </n-layout>
    </n-layout>

  </main>
</template>

<style scoped>
.sc-split-content {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sc-split-panes {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: stretch;
}

.sc-split-pane {
  min-width: 0;
  min-height: 0;
  flex: 0 0 auto;
  overflow: hidden;
}

.sc-split-pane__frame {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.sc-split-divider {
  width: 8px;
  flex: 0 0 8px;
  cursor: col-resize;
  background: transparent;
  position: relative;
  user-select: none;
  touch-action: none;
}

.sc-split-divider::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 50%;
  width: 1px;
  transform: translateX(-50%);
  background: var(--sc-border-strong);
}

.sc-split-divider:hover::before,
.sc-split-panes.is-dragging .sc-split-divider::before {
  width: 2px;
  background: rgba(14, 165, 233, 0.65);
}

.sc-split-iframe {
  border: 0;
  width: 100%;
  height: 100%;
  min-height: 0;
  display: block;
  background: var(--sc-bg-surface);
}

.sc-split-web-placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 16px;
  text-align: center;
  color: var(--sc-text-secondary);
  background: var(--sc-bg-surface);
}

.sc-split-web-placeholder__title {
  font-weight: 600;
  color: var(--sc-text-primary);
}

.sc-split-web-placeholder__desc {
  font-size: 12px;
}

.sc-split-panes.is-dragging .sc-split-iframe {
  pointer-events: none;
}
</style>
