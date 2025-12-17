<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NRadioButton, NRadioGroup, NSelect } from 'naive-ui';

type LayoutMode = 'left-column' | 'per-pane';

type ChannelOption = { label: string; value: string };

type EmbedReadyMessage = {
  type: 'sealchat.embed.ready' | 'sealchat.embed.state';
  paneId: string;
  worldId?: string;
  currentChannelId?: string;
  channelOptions?: ChannelOption[];
};

type EmbedFocusMessage = {
  type: 'sealchat.embed.focus';
  paneId: string;
  focused: boolean;
};

type EmbedMessage = EmbedReadyMessage | EmbedFocusMessage;

type PaneId = 'A' | 'B';

interface PaneState {
  id: PaneId;
  iframeKey: number;
  src: string;
  ready: boolean;
  focused: boolean;
  worldId?: string;
  currentChannelId?: string;
  channelOptions: ChannelOption[];
  notifyOwner: boolean;
}

const router = useRouter();
const route = useRoute();

const layoutMode = ref<LayoutMode>((route.query.layout as LayoutMode) || 'left-column');
const initialWorldId = typeof route.query.worldId === 'string' && route.query.worldId.trim()
  ? route.query.worldId.trim()
  : undefined;

const paneA = reactive<PaneState>({
  id: 'A',
  iframeKey: 0,
  src: '',
  ready: false,
  focused: false,
  worldId: initialWorldId,
  currentChannelId: typeof route.query.a === 'string' ? route.query.a : undefined,
  channelOptions: [],
  notifyOwner: route.query.notify === 'A',
});

const paneB = reactive<PaneState>({
  id: 'B',
  iframeKey: 0,
  src: '',
  ready: false,
  focused: false,
  worldId: initialWorldId,
  currentChannelId: typeof route.query.b === 'string' ? route.query.b : undefined,
  channelOptions: [],
  notifyOwner: route.query.notify === 'B',
});

const panes = computed(() => [paneA, paneB]);

const buildEmbedSrc = (pane: PaneState) => {
  const params = new URLSearchParams();
  params.set('paneId', pane.id);
  if (pane.currentChannelId) params.set('channelId', pane.currentChannelId);
  if (pane.worldId) params.set('worldId', pane.worldId);
  if (pane.notifyOwner) params.set('notifyOwner', '1');
  const base = import.meta.env.BASE_URL || '/';
  return `${base}#/embed?${params.toString()}`;
};

const refreshPaneSrc = (pane: PaneState) => {
  pane.ready = false;
  pane.src = buildEmbedSrc(pane);
  pane.iframeKey += 1;
};

const persistRouteQuery = () => {
  router.replace({
    name: 'split',
    query: {
      layout: layoutMode.value,
      worldId: paneA.worldId || paneB.worldId || '',
      a: paneA.currentChannelId || '',
      b: paneB.currentChannelId || '',
      notify: paneA.notifyOwner ? 'A' : paneB.notifyOwner ? 'B' : '',
    },
  });
};

const postToPane = (pane: PaneState, payload: any) => {
  const iframe = document.getElementById(`sc-split-iframe-${pane.id}`) as HTMLIFrameElement | null;
  const targetWindow = iframe?.contentWindow;
  if (!targetWindow) return false;
  targetWindow.postMessage(payload, window.location.origin);
  return true;
};

const setPaneChannel = async (pane: PaneState, channelId: string | null | undefined) => {
  const nextChannelId = typeof channelId === 'string' ? channelId : '';
  if (!nextChannelId) return;
  pane.currentChannelId = nextChannelId;
  persistRouteQuery();
  if (pane.ready) {
    const ok = postToPane(pane, { type: 'sealchat.embed.setChannel', paneId: pane.id, channelId: nextChannelId });
    if (!ok) {
      refreshPaneSrc(pane);
    }
    return;
  }
  refreshPaneSrc(pane);
};

const setNotifyOwner = (owner: PaneId | null) => {
  const nextA = owner === 'A';
  const nextB = owner === 'B';
  const changedA = paneA.notifyOwner !== nextA;
  const changedB = paneB.notifyOwner !== nextB;
  paneA.notifyOwner = nextA;
  paneB.notifyOwner = nextB;
  persistRouteQuery();
  if (changedA) refreshPaneSrc(paneA);
  if (changedB) refreshPaneSrc(paneB);
};

const handleNotifyOwnerChange = (value: string) => {
  if (value === 'A' || value === 'B') {
    setNotifyOwner(value);
    return;
  }
  setNotifyOwner(null);
};

const handleEmbedMessage = (event: MessageEvent) => {
  if (event.origin !== window.location.origin) return;
  const data = event.data as EmbedMessage | undefined;
  if (!data || typeof data !== 'object') return;
  if (!('type' in data) || typeof (data as any).type !== 'string') return;

  if (data.type === 'sealchat.embed.focus') {
    const target = data.paneId === 'A' ? paneA : data.paneId === 'B' ? paneB : null;
    if (!target) return;
    target.focused = !!data.focused;
    return;
  }

  if (data.type === 'sealchat.embed.ready' || data.type === 'sealchat.embed.state') {
    const target = data.paneId === 'A' ? paneA : data.paneId === 'B' ? paneB : null;
    if (!target) return;
    target.ready = true;
    if (typeof data.worldId === 'string') target.worldId = data.worldId;
    if (typeof data.currentChannelId === 'string') target.currentChannelId = data.currentChannelId;
    if (Array.isArray(data.channelOptions)) target.channelOptions = data.channelOptions;
    persistRouteQuery();
  }
};

const exitSplit = async () => {
  await router.push({ name: 'home' });
};

const initialize = () => {
  paneA.src = buildEmbedSrc(paneA);
  paneB.src = buildEmbedSrc(paneB);
};

watch(layoutMode, () => {
  persistRouteQuery();
});

onMounted(() => {
  initialize();
  window.addEventListener('message', handleEmbedMessage);
});

onBeforeUnmount(() => {
  window.removeEventListener('message', handleEmbedMessage);
});

const layoutContainerClass = computed(() => {
  return layoutMode.value === 'left-column' ? 'split-grid split-grid--left' : 'split-grid split-grid--per-pane';
});
</script>

<template>
  <main class="h-screen w-screen sc-split-root">
    <div class="sc-split-topbar">
      <div class="sc-split-topbar__left">
        <n-button type="primary" secondary @click="exitSplit">返回聊天</n-button>
        <div class="sc-split-topbar__title">分屏</div>
      </div>

      <div class="sc-split-topbar__right">
        <n-radio-group v-model:value="layoutMode" size="small">
          <n-radio-button value="left-column">频道栏 | A | B</n-radio-button>
          <n-radio-button value="per-pane">A(频道栏-聊天) | B(频道栏-聊天)</n-radio-button>
        </n-radio-group>

        <div class="sc-split-notify">
          <span class="sc-split-notify__label">通知窗格</span>
          <n-radio-group
            :value="paneA.notifyOwner ? 'A' : paneB.notifyOwner ? 'B' : ''"
            size="small"
            @update:value="handleNotifyOwnerChange"
          >
            <n-radio-button value="">无</n-radio-button>
            <n-radio-button value="A">A</n-radio-button>
            <n-radio-button value="B">B</n-radio-button>
          </n-radio-group>
        </div>
      </div>
    </div>

    <div class="sc-split-body" :class="layoutContainerClass">
      <div v-if="layoutMode === 'left-column'" class="sc-split-channelbar">
        <div class="sc-split-channelbar__section">
          <div class="sc-split-channelbar__label">窗格 A 频道</div>
          <n-select
            size="small"
            :value="paneA.currentChannelId"
            :options="paneA.channelOptions"
            placeholder="加载频道列表中…"
            clearable
            @update:value="setPaneChannel(paneA, $event)"
          />
        </div>
        <div class="sc-split-channelbar__section">
          <div class="sc-split-channelbar__label">窗格 B 频道</div>
          <n-select
            size="small"
            :value="paneB.currentChannelId"
            :options="paneB.channelOptions"
            placeholder="加载频道列表中…"
            clearable
            @update:value="setPaneChannel(paneB, $event)"
          />
        </div>
        <div class="sc-split-channelbar__hint">
          频道列表来自对应窗格（iframe）加载完成后的上报。
        </div>
      </div>

      <div class="sc-split-pane">
        <div v-if="layoutMode === 'per-pane'" class="sc-split-pane__bar">
          <div class="sc-split-pane__label">窗格 A</div>
          <n-select
            size="small"
            :value="paneA.currentChannelId"
            :options="paneA.channelOptions"
            placeholder="选择频道…"
            clearable
            @update:value="setPaneChannel(paneA, $event)"
          />
        </div>
        <iframe
          :id="`sc-split-iframe-${paneA.id}`"
          :key="paneA.iframeKey"
          class="sc-split-iframe"
          :src="paneA.src"
        />
      </div>

      <div class="sc-split-pane">
        <div v-if="layoutMode === 'per-pane'" class="sc-split-pane__bar">
          <div class="sc-split-pane__label">窗格 B</div>
          <n-select
            size="small"
            :value="paneB.currentChannelId"
            :options="paneB.channelOptions"
            placeholder="选择频道…"
            clearable
            @update:value="setPaneChannel(paneB, $event)"
          />
        </div>
        <iframe
          :id="`sc-split-iframe-${paneB.id}`"
          :key="paneB.iframeKey"
          class="sc-split-iframe"
          :src="paneB.src"
        />
      </div>
    </div>
  </main>
</template>

<style scoped>
.sc-split-root {
  display: flex;
  flex-direction: column;
  background: var(--sc-bg-surface);
}

.sc-split-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: var(--sc-bg-elevated);
  border-bottom: 1px solid var(--sc-border-strong);
}

.sc-split-topbar__left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.sc-split-topbar__title {
  font-weight: 600;
  color: var(--sc-text-primary);
}

.sc-split-topbar__right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.sc-split-notify {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sc-split-notify__label {
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.sc-split-body {
  flex: 1;
  min-height: 0;
}

.split-grid {
  height: 100%;
  display: grid;
  gap: 0;
}

.split-grid--left {
  grid-template-columns: 280px 1fr 1fr;
}

.split-grid--per-pane {
  grid-template-columns: 1fr 1fr;
}

.sc-split-channelbar {
  padding: 12px;
  border-right: 1px solid var(--sc-border-strong);
  background: var(--sc-bg-surface);
  overflow: auto;
}

.sc-split-channelbar__section + .sc-split-channelbar__section {
  margin-top: 12px;
}

.sc-split-channelbar__label {
  font-size: 12px;
  color: var(--sc-text-secondary);
  margin-bottom: 6px;
}

.sc-split-channelbar__hint {
  margin-top: 14px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.sc-split-pane {
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--sc-border-strong);
}

.split-grid--left .sc-split-pane:last-child {
  border-right: 0;
}

.split-grid--per-pane .sc-split-pane:last-child {
  border-right: 0;
}

.sc-split-pane__bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--sc-border-strong);
  background: var(--sc-bg-elevated);
}

.sc-split-pane__label {
  font-size: 12px;
  color: var(--sc-text-secondary);
  white-space: nowrap;
}

.sc-split-iframe {
  border: 0;
  width: 100%;
  height: 100%;
  min-height: 0;
  background: var(--sc-bg-surface);
}
</style>
