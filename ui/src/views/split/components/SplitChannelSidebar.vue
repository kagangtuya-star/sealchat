<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NBadge, NButton, NIcon, NInput, NRadioButton, NRadioGroup } from 'naive-ui';
import { ArrowsLeftRight } from '@vicons/tabler';
import { matchText } from '@/utils/pinyinMatch';

export type PaneId = 'A' | 'B';
type PaneMode = 'chat' | 'web';

export interface SplitChannelNode {
  id: string;
  name: string;
  unread: number;
  children?: SplitChannelNode[];
}

type WorldOption = { value: string; label: string };

interface PaneSummary {
  id: PaneId;
  channelName: string;
  unread: number;
  worldName: string;
}

type OperationTarget = 'follow' | PaneId;

const props = defineProps<{
  activePaneId: PaneId;
  panes: PaneSummary[];
  worldId: string;
  worldOptions: WorldOption[];
  lockSameWorld: boolean;
  notifyOwnerPaneId: PaneId | null;
  operationTarget: OperationTarget;
  audioPlaybackTarget: OperationTarget;
  worldName: string;
  channelTree: SplitChannelNode[];
  webTargetPaneId: PaneId;
  paneModes: { A: PaneMode; B: PaneMode };
  paneWebUrls: { A: string; B: string };
}>();

const emit = defineEmits<{
  (e: 'set-active-pane', paneId: PaneId): void;
  (e: 'set-operation-target', target: OperationTarget): void;
  (e: 'set-world', worldId: string): void;
  (e: 'toggle-lock-same-world', enabled: boolean): void;
  (e: 'set-notify-owner', paneId: PaneId | null): void;
  (e: 'set-audio-playback-target', target: OperationTarget): void;
  (e: 'open-channel', channelId: string): void;
  (e: 'set-web-target', paneId: PaneId): void;
  (e: 'set-pane-mode', paneId: PaneId, mode: PaneMode): void;
  (e: 'set-pane-url', paneId: PaneId, url: string): void;
  (e: 'swap-panes'): void;
  (e: 'exit-split'): void;
}>();

const channelFilter = ref('');
const webUrlDraft = ref('');

const webTargetMode = computed(() => props.paneModes[props.webTargetPaneId]);
const updateWebTarget = (value: string) => {
  if (value === 'A' || value === 'B') {
    emit('set-web-target', value);
  }
};
const applyWebUrl = () => {
  emit('set-pane-url', props.webTargetPaneId, webUrlDraft.value);
};
const clearWebUrl = () => {
  webUrlDraft.value = '';
  emit('set-pane-url', props.webTargetPaneId, '');
};

const filteredTree = computed(() => {
  const keyword = channelFilter.value.trim();
  if (!keyword) return props.channelTree;

  const filterNode = (node: SplitChannelNode): SplitChannelNode | null => {
    const selfMatch = matchText(keyword, node.name || '');
    const children = Array.isArray(node.children) ? node.children : [];
    const nextChildren = children.map(filterNode).filter(Boolean) as SplitChannelNode[];
    if (selfMatch || nextChildren.length > 0) {
      return { ...node, children: nextChildren };
    }
    return null;
  };

  return props.channelTree.map(filterNode).filter(Boolean) as SplitChannelNode[];
});

const notifyValue = computed(() => (props.notifyOwnerPaneId ? props.notifyOwnerPaneId : ''));
const setNotifyValue = (value: string) => {
  if (value === 'A' || value === 'B') {
    emit('set-notify-owner', value);
    return;
  }
  emit('set-notify-owner', null);
};

const setAudioPlaybackTarget = (value: string) => {
  if (value === 'follow' || value === 'A' || value === 'B') {
    emit('set-audio-playback-target', value);
  }
};

watch(
  () => [props.webTargetPaneId, props.paneWebUrls.A, props.paneWebUrls.B],
  () => {
    webUrlDraft.value = props.paneWebUrls[props.webTargetPaneId] || '';
  },
  { immediate: true },
);

const renderTree = (nodes: SplitChannelNode[], depth = 0): any[] => {
  if (!Array.isArray(nodes)) return [];
  const items: any[] = [];
  nodes.forEach((node) => {
    items.push({ node, depth });
    const children = Array.isArray(node.children) ? node.children : [];
    if (children.length > 0) {
      items.push(...renderTree(children, depth + 1));
    }
  });
  return items;
};

const flatTree = computed(() => renderTree(filteredTree.value, 0));
</script>

<template>
  <div class="sc-split-sidebar">
    <div class="sc-split-sidebar__section">
      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__title">分屏</div>
        <div class="sc-split-sidebar__row-actions">
          <n-button size="small" tertiary @click="emit('swap-panes')">
            <template #icon>
              <n-icon :component="ArrowsLeftRight" />
            </template>
            交换
          </n-button>
          <n-button size="small" tertiary @click="emit('exit-split')">退出</n-button>
        </div>
      </div>

      <div class="sc-split-sidebar__chips">
        <button
          v-for="pane in panes"
          :key="pane.id"
          type="button"
          class="sc-split-pane-chip"
          :class="{ 'is-active': pane.id === activePaneId }"
          @click="emit('set-active-pane', pane.id)"
        >
          <span class="sc-split-pane-chip__id">{{ pane.id }}</span>
          <span class="sc-split-pane-chip__content">
            <span class="sc-split-pane-chip__name">{{ pane.channelName || '未选择频道' }}</span>
            <span class="sc-split-pane-chip__world">{{ pane.worldName || '未选择世界' }}</span>
          </span>
          <n-badge v-if="pane.unread > 0" :value="pane.unread" :max="99" />
        </button>
      </div>

      <div class="sc-split-sidebar__meta">
        <div class="sc-split-sidebar__meta-label">当前世界</div>
        <div class="sc-split-sidebar__meta-value">{{ worldName || '未选择世界' }}</div>
      </div>
    </div>

    <div class="sc-split-sidebar__section">
      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">操作目标</div>
        <n-radio-group
          size="small"
          :value="operationTarget"
          @update:value="emit('set-operation-target', $event)"
        >
          <n-radio-button value="follow">跟随焦点</n-radio-button>
          <n-radio-button value="A">A</n-radio-button>
          <n-radio-button value="B">B</n-radio-button>
        </n-radio-group>
      </div>

      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">世界</div>
        <n-radio-group
          size="small"
          :value="lockSameWorld ? 'locked' : 'free'"
          @update:value="emit('toggle-lock-same-world', $event === 'locked')"
        >
          <n-radio-button value="free">允许不同世界</n-radio-button>
          <n-radio-button value="locked">锁定同世界</n-radio-button>
        </n-radio-group>
      </div>

      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">切换世界</div>
        <n-input v-if="worldOptions.length === 0" size="small" disabled placeholder="加载世界列表中…" />
        <n-select
          v-if="worldOptions.length > 0"
          size="small"
          :value="worldId"
          :options="worldOptions"
          placeholder="选择世界…"
          @update:value="emit('set-world', $event)"
        />
      </div>

      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">通知窗格</div>
        <n-radio-group size="small" :value="notifyValue" @update:value="setNotifyValue">
          <n-radio-button value="">无</n-radio-button>
          <n-radio-button value="A">A</n-radio-button>
          <n-radio-button value="B">B</n-radio-button>
        </n-radio-group>
      </div>

      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">音频播放</div>
        <n-radio-group size="small" :value="audioPlaybackTarget" @update:value="setAudioPlaybackTarget">
          <n-radio-button value="follow">跟随焦点</n-radio-button>
          <n-radio-button value="A">A</n-radio-button>
          <n-radio-button value="B">B</n-radio-button>
        </n-radio-group>
      </div>
    </div>

    <div class="sc-split-sidebar__section">
      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">网页分屏</div>
        <n-radio-group size="small" :value="webTargetPaneId" @update:value="updateWebTarget">
          <n-radio-button value="A">A</n-radio-button>
          <n-radio-button value="B">B</n-radio-button>
        </n-radio-group>
      </div>

      <div class="sc-split-sidebar__row">
        <div class="sc-split-sidebar__label">模式</div>
        <n-radio-group
          size="small"
          :value="webTargetMode"
          @update:value="emit('set-pane-mode', webTargetPaneId, $event)"
        >
          <n-radio-button value="chat">聊天</n-radio-button>
          <n-radio-button value="web">网页</n-radio-button>
        </n-radio-group>
      </div>

      <div class="sc-split-sidebar__row sc-split-sidebar__row--stack">
        <div class="sc-split-sidebar__label">网址</div>
        <div class="sc-split-sidebar__web-input">
          <n-input
            v-model:value="webUrlDraft"
            size="small"
            placeholder="https://example.com"
            clearable
          />
          <div class="sc-split-sidebar__web-actions">
            <n-button size="tiny" tertiary @click="applyWebUrl">应用</n-button>
            <n-button size="tiny" quaternary @click="clearWebUrl">清空</n-button>
          </div>
        </div>
      </div>
    </div>

    <div class="sc-split-sidebar__section sc-split-sidebar__section--channels">
      <n-input v-model:value="channelFilter" size="small" placeholder="筛选频道…" clearable />

      <div class="sc-split-channel-tree">
        <button
          v-for="item in flatTree"
          :key="`${item.node.id}-${item.depth}`"
          type="button"
          class="sc-split-channel-item"
          :style="{ paddingLeft: `${12 + item.depth * 14}px` }"
          @click="emit('open-channel', item.node.id)"
        >
          <span class="sc-split-channel-item__name">{{ item.node.name }}</span>
          <n-badge v-if="item.node.unread > 0" :value="item.node.unread" :max="99" />
        </button>

        <div v-if="flatTree.length === 0" class="sc-split-channel-empty">
          暂无频道
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sc-split-sidebar {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  background: var(--sc-bg-surface);
  color: var(--sc-text-primary);
  min-height: 0;
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.35)) transparent;
}

.sc-split-sidebar::-webkit-scrollbar {
  width: var(--sc-scrollbar-size, 6px);
}

.sc-split-sidebar::-webkit-scrollbar-track {
  background: transparent;
}

.sc-split-sidebar::-webkit-scrollbar-thumb {
  background-color: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.35));
  border-radius: 999px;
}

.sc-split-sidebar::-webkit-scrollbar-thumb:hover {
  background-color: var(--sc-scrollbar-thumb-hover, rgba(148, 163, 184, 0.55));
}

.sc-split-sidebar__section {
  border: 1px solid var(--sc-border-strong);
  background: var(--sc-bg-elevated);
  border-radius: 12px;
  padding: 10px;
}

.sc-split-sidebar__section--channels {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.sc-split-sidebar__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}

.sc-split-sidebar__row--stack {
  align-items: flex-start;
  flex-direction: column;
}

.sc-split-sidebar__row-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.sc-split-sidebar__row + .sc-split-sidebar__row {
  margin-top: 10px;
}

.sc-split-sidebar__row--lock {
  align-items: center;
}

.sc-split-sidebar__web-input {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sc-split-sidebar__web-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.sc-split-sidebar__title {
  font-weight: 700;
}

.sc-split-sidebar__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.sc-split-sidebar__chips {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
  margin-top: 10px;
}

.sc-split-pane-chip {
  width: 100%;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border-radius: 9999px;
  border: 1px solid transparent;
  padding: 6px 10px;
  background: transparent;
  color: var(--sc-text-primary);
  cursor: pointer;
  justify-content: space-between;
}

.sc-split-pane-chip:hover {
  background: var(--sc-chip-bg);
}

.sc-split-pane-chip.is-active {
  background-color: rgba(59, 130, 246, 0.18);
  border-color: rgba(37, 99, 235, 0.35);
}

.sc-split-pane-chip__id {
  font-weight: 700;
  width: 16px;
}

.sc-split-pane-chip__name {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: left;
}

.sc-split-pane-chip__content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.sc-split-pane-chip__world {
  font-size: 11px;
  color: var(--sc-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
  text-align: left;
}

.sc-split-sidebar__meta {
  margin-top: 10px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.sc-split-sidebar__meta-label {
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.sc-split-sidebar__meta-value {
  font-size: 12px;
  color: var(--sc-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 190px;
  text-align: right;
}

.sc-split-channel-tree {
  border-radius: 10px;
  border: 1px solid var(--sc-border-strong);
  background: var(--sc-bg-surface);
  padding: 6px 0;
}

.sc-split-channel-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 12px;
  background: transparent;
  border: 0;
  cursor: pointer;
  color: var(--sc-text-primary);
}

.sc-split-channel-item:hover {
  background: var(--sc-chip-bg);
}

.sc-split-channel-item__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  text-align: left;
}

.sc-split-channel-empty {
  padding: 12px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}
</style>
