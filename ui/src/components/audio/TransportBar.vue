<template>
  <div class="transport-bar">
    <div class="transport-bar__controls">
      <n-button
        class="transport-bar__primary-action"
        size="small"
        circle
        @click="togglePlay"
        :disabled="isReadOnly"
        :class="{ 'transport-bar__primary-action--active': isTransportPlaying }"
        :aria-label="isTransportPlaying ? '全部暂停' : '全部播放'"
      >
        <template #icon><n-icon :component="isTransportPlaying ? PlayerPause : PlayerPlay" /></template>
      </n-button>
    </div>

    <div class="transport-bar__volume">
      <span class="transport-bar__control-icon" aria-hidden="true">🔉</span>
      <span class="transport-bar__volume-label">总音量</span>
      <n-slider
        class="transport-bar__volume-slider"
        :value="audio.masterVolume"
        :step="0.01"
        :min="0"
        :max="1"
        @update:value="handleVolumeChange"
      />
      <span class="transport-bar__control-icon transport-bar__control-icon--right" aria-hidden="true">🔊</span>
      <span class="transport-bar__volume-value">{{ masterVolumePercent }}%</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { PlayerPause, PlayerPlay } from '@vicons/tabler';
import { useAudioStudioStore } from '@/stores/audioStudio';
import { hasAnyActivePlayback } from '@/stores/audioPlaybackState';

const audio = useAudioStudioStore();
const isReadOnly = computed(() => !audio.canManage);
const masterVolumePercent = computed(() => Math.round(audio.masterVolume * 100));
const isTransportPlaying = computed(() => hasAnyActivePlayback(Object.values(audio.tracks || {})));

function togglePlay() {
  audio.togglePlay();
}

function handleVolumeChange(volume: number) {
  audio.setMasterVolume(volume);
}
</script>

<style scoped lang="scss">
.transport-bar {
  --audio-control-rail: rgba(148, 163, 184, 0.2);
  --audio-control-rail-hover: rgba(148, 163, 184, 0.28);
  --audio-control-fill: rgba(203, 213, 225, 0.72);
  --audio-control-fill-hover: linear-gradient(90deg, #34d399, #14b8a6);
  --audio-control-handle: rgba(255, 255, 255, 0.96);
  --audio-control-handle-border: rgba(148, 163, 184, 0.45);
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

.transport-bar__primary-action {
  flex-shrink: 0;
  --n-color: rgba(148, 163, 184, 0.12);
  --n-color-hover: rgba(148, 163, 184, 0.18);
  --n-color-pressed: rgba(148, 163, 184, 0.22);
  --n-border: 1px solid rgba(148, 163, 184, 0.18);
  --n-border-hover: 1px solid rgba(148, 163, 184, 0.28);
  --n-border-pressed: 1px solid rgba(148, 163, 184, 0.32);
  --n-text-color: rgba(226, 232, 240, 0.88);
  --n-text-color-hover: rgba(248, 250, 252, 0.96);
  --n-text-color-pressed: rgba(248, 250, 252, 0.96);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  transition: transform 0.16s ease, box-shadow 0.16s ease, border-color 0.16s ease, background 0.16s ease;
}

.transport-bar__primary-action:hover {
  transform: translateY(-1px);
}

.transport-bar__primary-action--active {
  --n-color: rgba(20, 184, 166, 0.14);
  --n-color-hover: rgba(20, 184, 166, 0.2);
  --n-color-pressed: rgba(20, 184, 166, 0.24);
  --n-border: 1px solid rgba(45, 212, 191, 0.32);
  --n-border-hover: 1px solid rgba(94, 234, 212, 0.42);
  --n-border-pressed: 1px solid rgba(94, 234, 212, 0.46);
  --n-text-color: #ccfbf1;
  --n-text-color-hover: #f0fdfa;
  --n-text-color-pressed: #f0fdfa;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 0 0 1px rgba(45, 212, 191, 0.08), 0 0 18px rgba(20, 184, 166, 0.12);
}

.transport-bar__volume {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  flex: 1;
  min-width: 120px;
}

.transport-bar__control-icon {
  width: 1rem;
  text-align: center;
  font-size: 0.88rem;
  color: rgba(148, 163, 184, 0.88);
  flex-shrink: 0;
}

.transport-bar__control-icon--right {
  color: rgba(203, 213, 225, 0.92);
}

.transport-bar__volume-label {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
  white-space: nowrap;
}

.transport-bar__volume-slider {
  flex: 1;
  min-width: 0;
  --n-rail-height: 4px;
  --n-rail-color: var(--audio-control-rail);
  --n-rail-color-hover: var(--audio-control-rail-hover);
  --n-fill-color: var(--audio-control-fill);
  --n-fill-color-hover: var(--audio-control-fill-hover);
  --n-handle-color: var(--audio-control-handle);
  --n-handle-size: 12px;
}

.transport-bar__volume-value {
  width: 2.75rem;
  text-align: right;
  font-size: 0.75rem;
  color: rgba(203, 213, 225, 0.92);
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}

:deep(.transport-bar__volume-slider .n-slider-rail) {
  height: 4px;
}

:deep(.transport-bar__volume-slider .n-slider-handle) {
  background-color: var(--audio-control-handle);
  border: 1px solid var(--audio-control-handle-border);
  box-sizing: border-box;
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
