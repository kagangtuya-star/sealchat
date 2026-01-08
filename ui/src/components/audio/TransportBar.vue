<template>
  <div class="transport-bar">
    <div class="transport-bar__controls">
      <n-button-group>
        <n-button quaternary size="small" @click="seekBackward" :disabled="isReadOnly">-5s</n-button>
        <n-button type="primary" size="small" @click="togglePlay" :disabled="isReadOnly">
          {{ audio.isPlaying ? '暂停' : '播放' }}
        </n-button>
        <n-button quaternary size="small" @click="seekForward" :disabled="isReadOnly">+5s</n-button>
      </n-button-group>
      <n-button
        quaternary
        size="small"
        @click="audio.toggleLoop()"
        :type="audio.loopEnabled ? 'info' : 'default'"
        :disabled="isReadOnly"
      >
        {{ audio.loopEnabled ? '循环中' : '循环关闭' }}
      </n-button>
    </div>

    <div class="transport-bar__progress">
      <n-slider
        :value="progressPercent"
        :step="0.5"
        :format-tooltip="formatProgressTooltip"
        @update:value="handleProgress"
        :disabled="isReadOnly"
      />
      <div class="transport-bar__progress-meta">
        <span>{{ formatTime(currentSeconds) }}</span>
        <span>{{ formatTime(maxDuration) }}</span>
      </div>
    </div>

    <div class="transport-bar__mix">
      <n-select
        class="transport-bar__speed"
        :value="audio.playbackRate"
        size="small"
        :options="speedOptions"
        @update:value="handleRateChange"
        :disabled="isReadOnly"
      />
      <span class="transport-bar__buffer">{{ audio.bufferMessage }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useAudioStudioStore } from '@/stores/audioStudio';

const audio = useAudioStudioStore();
const isReadOnly = computed(() => !audio.canManage);

const speedOptions = [0.75, 1, 1.25, 1.5].map((value) => ({ label: `${value}x`, value }));

const overallProgress = computed(() => {
  const all = Object.values(audio.tracks || {});
  if (!all.length) return 0;
  return Math.max(...all.map((track) => track.progress || 0));
});

const progressPercent = computed(() => Math.round(overallProgress.value * 100));

const maxDuration = computed(() => {
  const all = Object.values(audio.tracks || {});
  return Math.max(0, ...all.map((track) => track.duration || 0));
});

const currentSeconds = computed(() => maxDuration.value * overallProgress.value);

function togglePlay() {
  audio.togglePlay();
}

function seekBackward() {
  audio.seekAll(-5);
}

function seekForward() {
  audio.seekAll(5);
}

function handleProgress(value: number) {
  const duration = maxDuration.value;
  if (!duration) return;
  audio.seekToSeconds((value / 100) * duration);
}

function handleRateChange(rate: string | number | null) {
  if (rate == null) return;
  const parsed = typeof rate === 'number' ? rate : Number(rate);
  audio.setPlaybackRate(parsed || 1);
}

function formatProgressTooltip(val: number) {
  const duration = maxDuration.value;
  if (!duration) return '00:00';
  return formatTime((val / 100) * duration);
}

function formatTime(value: number) {
  if (!value || Number.isNaN(value)) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}
</script>

<style scoped lang="scss">
.transport-bar {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1rem;
  border-radius: 12px;
  background: var(--audio-panel-surface, var(--sc-bg-elevated, #f8fafc));
  border: 1px solid var(--audio-panel-border, var(--sc-border-mute, #e2e8f0));
  box-shadow: var(--audio-panel-shadow, 0 20px 40px rgba(15, 23, 42, 0.08));
  backdrop-filter: blur(12px);
}

.transport-bar__controls {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  align-items: center;
}

.transport-bar__progress {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.transport-bar__progress-meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
}

.transport-bar__mix {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
}

.transport-bar__speed {
  width: 110px;
}

.transport-bar__buffer {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
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
