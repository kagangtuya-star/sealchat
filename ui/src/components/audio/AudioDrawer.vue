<template>
  <n-drawer
    :show="audio.drawerVisible"
    placement="right"
    :width="drawerWidth"
    :mask-closable="true"
    :close-on-esc="true"
    @update:show="audio.toggleDrawer"
    class="audio-drawer"
  >
    <n-drawer-content title="音频工作台">
      <div class="audio-drawer__header">
        <div>
          <p class="audio-drawer__subtitle">多音轨播放 / 素材管理</p>
          <n-tag size="small">{{ audio.networkMode === 'normal' ? '标准模式' : '弱网模式' }}</n-tag>
        </div>
        <n-button quaternary size="small" @click="audio.ensureInitialized">刷新数据</n-button>
      </div>

      <n-tabs type="segment" :value="audio.activeTab" @update:value="handleTabChange">
        <n-tab-pane name="player" tab="播放控制">
          <div class="audio-drawer__player">
            <TransportBar />
            <div class="audio-drawer__tracks">
              <TrackMixerCard v-for="track in tracks" :key="track.id" :track="track" />
            </div>
          </div>
        </n-tab-pane>
        <n-tab-pane v-if="audio.canManage" name="playlist" tab="播放列表">
          <ScenePlaylist />
        </n-tab-pane>
        <n-tab-pane v-if="audio.canManage" name="library" tab="素材库">
          <AudioAssetManager />
        </n-tab-pane>
      </n-tabs>

      <n-alert v-if="audio.error" type="error" class="audio-drawer__alert">{{ audio.error }}</n-alert>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useAudioStudioStore } from '@/stores/audioStudio';
import TransportBar from './TransportBar.vue';
import TrackMixerCard from './TrackMixerCard.vue';
import ScenePlaylist from './ScenePlaylist.vue';
import AudioAssetManager from './AudioAssetManager.vue';

const audio = useAudioStudioStore();
type AudioTab = 'player' | 'playlist' | 'library';
const tracks = computed(() => Object.values(audio.tracks || {}));
const internalWidth = ref(420);
const updateWidth = () => {
  if (typeof window === 'undefined') return;
  internalWidth.value = window.innerWidth > 768 ? 420 : Math.min(window.innerWidth, 420);
};
const drawerWidth = computed(() => internalWidth.value);
const handleTabChange = (val: string | number) => {
  audio.selectTab((val as AudioTab) || 'player');
};

onMounted(() => {
  audio.ensureInitialized();
  updateWidth();
  window.addEventListener('resize', updateWidth);
});

onBeforeUnmount(() => {
  if (typeof window === 'undefined') return;
  window.removeEventListener('resize', updateWidth);
});
</script>

<style scoped lang="scss">
.audio-drawer :deep(.n-drawer-body) {
  background: var(--audio-panel-surface, var(--sc-bg-elevated));
  border-left: 1px solid var(--audio-panel-border, var(--sc-border-mute));
  box-shadow: var(--audio-panel-shadow, 0 20px 40px rgba(15, 23, 42, 0.08));
  color: var(--sc-text-primary);
}

.audio-drawer :deep(.n-drawer-body-content-wrapper) {
  background: transparent;
}

.audio-drawer__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.audio-drawer__subtitle {
  margin: 0;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.audio-drawer__player {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 0.5rem 0;
}

.audio-drawer__tracks {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.audio-drawer__alert {
  margin-top: 0.5rem;
}
</style>
