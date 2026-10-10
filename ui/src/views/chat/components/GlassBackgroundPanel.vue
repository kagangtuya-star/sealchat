<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useDraggable, useResizeObserver, useWindowSize } from '@vueuse/core';
import { NButton, NSwitch, NTabs, NTabPane } from 'naive-ui';
import { useGlassBackground } from '@/composables/useGlassBackground';
import GlassBackgroundEditor from './GlassBackgroundEditor.vue';
import WorldGlassPresetManager from './WorldGlassPresetManager.vue';
import type { GlassBackgroundChannelNode } from './glassBackgroundTypes';

const props = defineProps<{
  worldId?: string;
  channelId?: string;
  channelTree?: GlassBackgroundChannelNode[];
}>();
const emit = defineEmits<{ (event: 'close'): void }>();
const { settings, worldGlassState, update, reset } = useGlassBackground();
const panelRef = ref<HTMLElement>();
const handleRef = ref<HTMLElement>();
const minimized = ref(false);
const { width, height } = useWindowSize();
const key = 'sealchat.glass-background.panel.v1';
const { x, y } = useDraggable(panelRef, {
  handle: handleRef, initialValue: { x: 24, y: 80 }, preventDefault: true,
  onStart: (_, event) => {
    if (event.button !== 0 || (event.target as HTMLElement).closest('button')) return false;
  },
  onMove: () => clamp(),
  onEnd: () => { clamp(); persist(); },
});

function clamp() {
  const rect = panelRef.value?.getBoundingClientRect();
  x.value = Math.max(12, Math.min(x.value, width.value - (rect?.width ?? 380) - 12));
  y.value = Math.max(12, Math.min(y.value, height.value - (rect?.height ?? 48) - 12));
}
function persist() {
  try { localStorage.setItem(key, JSON.stringify({ version: 1, x: x.value, y: y.value, minimized: minimized.value })); }
  catch { /* Keep the panel usable without storage. */ }
}
onMounted(() => {
  try {
    const saved = JSON.parse(localStorage.getItem(key) || 'null');
    if (saved?.version === 1) {
      if (typeof saved.x === 'number' && Number.isFinite(saved.x)) x.value = saved.x;
      if (typeof saved.y === 'number' && Number.isFinite(saved.y)) y.value = saved.y;
      minimized.value = saved.minimized === true;
    }
  } catch { /* Use the default position. */ }
  nextTick(clamp);
});
watch([width, height, minimized], () => nextTick(() => { clamp(); persist(); }));
useResizeObserver(panelRef, clamp);
onBeforeUnmount(persist);
</script>

<template>
  <Teleport to="body">
    <section ref="panelRef" class="sc-glass-settings-panel" role="dialog" aria-label="玻璃背景" :style="{ left: `max(${x}px, env(safe-area-inset-left))`, top: `max(${y}px, env(safe-area-inset-top))` }">
      <header ref="handleRef" class="sc-glass-settings-panel__header">
        <strong>玻璃背景</strong>
        <NButton quaternary size="small" :aria-label="minimized ? '展开' : '最小化'" @click="minimized = !minimized">{{ minimized ? '＋' : '—' }}</NButton>
        <NButton quaternary size="small" aria-label="关闭" @click="emit('close')">×</NButton>
      </header>
      <div v-show="!minimized" class="sc-glass-settings-panel__body">
        <NTabs type="line" animated>
          <NTabPane name="personal" tab="个人背景">
            <p v-if="worldGlassState?.enabled && worldGlassState.preset">当前由世界预设统一设置，以下个人设置会保留。</p>
            <div class="sc-glass-settings-panel__row"><span>启用玻璃背景</span><NSwitch :value="settings.enabled" @update:value="update({ enabled: $event })" /></div>
            <GlassBackgroundEditor :settings="settings" @update="update" />
            <NButton size="small" @click="reset">恢复默认参数</NButton>
          </NTabPane>
          <NTabPane name="world" tab="世界预设">
            <WorldGlassPresetManager :world-id="props.worldId" :channel-id="props.channelId" :channel-tree="props.channelTree" />
          </NTabPane>
        </NTabs>
      </div>
    </section>
  </Teleport>
</template>

<style scoped>
.sc-glass-settings-panel { position: fixed; z-index: 3200; width: 380px; max-width: calc(100vw - 24px - env(safe-area-inset-left) - env(safe-area-inset-right)); max-height: calc(100dvh - 24px - env(safe-area-inset-top) - env(safe-area-inset-bottom)); display: flex; flex-direction: column; overflow: hidden; border: 1px solid var(--sc-border-strong); border-radius: 12px; background: var(--sc-bg-elevated); color: var(--sc-text-primary); box-shadow: 0 12px 36px #0003; }
.sc-glass-settings-panel__header { display: flex; align-items: center; gap: 4px; padding: 10px 12px; flex-shrink: 0; cursor: grab; touch-action: none; user-select: none; border-bottom: 1px solid var(--sc-border-mute); }
.sc-glass-settings-panel__header strong { flex: 1; }
.sc-glass-settings-panel__body { padding: 16px; overflow-y: auto; min-height: 0; display: flex; flex-direction: column; gap: 14px; }
.sc-glass-settings-panel__row { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.sc-glass-settings-panel__body :deep(.n-tab-pane) { display: flex; flex-direction: column; gap: 14px; }
</style>
