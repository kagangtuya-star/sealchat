<template>
  <div class="transport-bar">
    <div class="transport-bar__controls">
      <n-button type="primary" size="small" @click="togglePlay" :disabled="isReadOnly">
        {{ audio.isPlaying ? '全部暂停' : '全部播放' }}
      </n-button>
    </div>

    <div class="transport-bar__volume">
      <span class="transport-bar__volume-label">总音量</span>
      <n-slider
        :value="audio.masterVolume"
        :step="0.01"
        :min="0"
        :max="1"
        @update:value="handleVolumeChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useAudioStudioStore } from '@/stores/audioStudio';

const audio = useAudioStudioStore();
const isReadOnly = computed(() => !audio.canManage);

function togglePlay() {
  audio.togglePlay();
}

function handleVolumeChange(volume: number) {
  audio.setMasterVolume(volume);
}
</script>

<style scoped lang="scss">
.transport-bar {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1rem;
  border-radius: 12px;
  background: var(--audio-panel-surface, var(--sc-bg-elevated, #f8fafc));
  border: 1px solid var(--audio-panel-border, var(--sc-border-mute, #e2e8f0));
  box-shadow: var(--audio-panel-shadow, 0 20px 40px rgba(15, 23, 42, 0.08));
  backdrop-filter: blur(12px);
}

.transport-bar__controls {
  flex-shrink: 0;
}

.transport-bar__volume {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex: 1;
  min-width: 120px;
}

.transport-bar__volume-label {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
  white-space: nowrap;
}
</style>

<!-- 非 scoped 样式用于自定义主题覆盖 -->
<style lang="scss">
:root[data-custom-theme='true'] .transport-bar.transport-bar {
  background: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-mute) !important;
  box-shadow: none !important;
}
</style>
