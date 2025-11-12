<template>
  <div class="track-card" :class="[`track-card--${track.type}`, { 'track-card--muted': track.muted }]">
    <header class="track-card__header">
      <div class="track-card__info">
        <p class="track-card__type">{{ trackLabels[track.type] }}</p>
        <p class="track-card__title">{{ track.asset?.name || '未选择音频' }}</p>
      </div>
      <div class="track-card__actions">
        <n-select
          class="track-card__selector"
          size="small"
          placeholder="选择音频"
          :value="track.assetId"
          :options="assetOptions"
          filterable
          clearable
          :disabled="!assetsAvailable"
          @update:value="handleSelect"
        />
        <n-button text size="tiny" @click="toggleSolo" :type="track.solo ? 'info' : 'primary'">
          {{ track.solo ? '取消独奏' : '独奏' }}
        </n-button>
        <n-button text size="tiny" @click="toggleMute">
          {{ track.muted ? '取消静音' : '静音' }}
        </n-button>
      </div>
    </header>

    <section class="track-card__body">
      <div class="track-card__progress">
        <div class="progress-shell">
          <div class="progress-buffer" :style="{ width: `${Math.round(track.buffered * 100)}%` }"></div>
          <div class="progress-value" :style="{ width: `${progressPercent}%` }"></div>
        </div>
        <span>{{ formatTime(currentSeconds) }} / {{ formatTime(track.duration) }}</span>
      </div>

      <div class="track-card__volume">
        <span>音量</span>
        <n-slider :value="track.volume" :step="0.01" @update:value="setVolume" :min="0" :max="1"></n-slider>
      </div>
    </section>

    <footer class="track-card__footer">
      <n-tag v-if="track.status === 'loading'" type="info" size="small">加载中</n-tag>
      <n-tag v-else-if="track.status === 'error'" type="error" size="small">{{ track.error || '播放失败' }}</n-tag>
      <n-tag v-else-if="track.status === 'playing'" type="success" size="small">播放中</n-tag>
      <n-tag v-else-if="track.status === 'paused'" type="warning" size="small">已暂停</n-tag>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PropType } from 'vue';
import type { TrackRuntime } from '@/stores/audioStudio';
import type { AudioAsset } from '@/types/audio';
import { useAudioStudioStore } from '@/stores/audioStudio';

const props = defineProps({
  track: {
    type: Object as PropType<TrackRuntime>,
    required: true,
  },
});

const trackLabels: Record<string, string> = {
  music: '音乐轨',
  ambience: '环境轨',
  sfx: '音效轨',
};

const audio = useAudioStudioStore();
const progressPercent = computed(() => Math.round(props.track.progress * 100));
const currentSeconds = computed(() => {
  const duration = props.track.duration || 0;
  return duration * props.track.progress;
});

const assetsAvailable = computed(() => audio.filteredAssets?.length > 0);
const assetOptions = computed(() =>
  audio.filteredAssets.slice(0, 50).map((asset) => ({
    label: `${asset.name}${asset.tags?.length ? ` · ${asset.tags.join(',')}` : ''}`,
    value: asset.id,
  })),
);

function formatTime(value: number) {
  if (!value || Number.isNaN(value)) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function setVolume(value: number) {
  audio.setTrackVolume(props.track.type, value);
}

function toggleMute() {
  audio.toggleTrackMute(props.track.type);
}

function toggleSolo() {
  audio.toggleTrackSolo(props.track.type);
}

function handleSelect(value: string | null) {
  if (!value) return;
  const asset = audio.assets.find((item) => item.id === value) || audio.filteredAssets.find((item) => item.id === value);
  if (asset) {
    audio.assignAssetToTrack(props.track.type, asset as AudioAsset);
  }
}
</script>

<style scoped lang="scss">
.track-card {
  border: 1px solid var(--audio-card-border, var(--sc-border-mute));
  border-radius: 12px;
  padding: 1rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
  backdrop-filter: blur(10px);
  box-shadow: var(--audio-panel-shadow, 0 20px 40px rgba(15, 23, 42, 0.08));
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  transition: border-color 0.2s ease, background 0.2s ease;
}

.track-card--muted {
  opacity: 0.6;
}

.track-card__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.5rem;
}

.track-card__selector {
  min-width: 140px;
}

.track-card__type {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
  margin: 0;
}

.track-card__title {
  font-size: 1rem;
  margin: 0;
  font-weight: 600;
  color: var(--sc-text-primary, #e2e8f0);
}

.track-card__actions {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.track-card__body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.track-card__progress {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
}

.progress-shell {
  position: relative;
  width: 100%;
  height: 6px;
  border-radius: 999px;
  background: var(--audio-progress-track, rgba(255, 255, 255, 0.08));
}

.progress-buffer {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: var(--audio-progress-buffer, rgba(255, 255, 255, 0.2));
  border-radius: 999px;
}

.progress-value {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: linear-gradient(90deg, #63b3ed, #f687b3);
  border-radius: 999px;
}

.track-card__volume {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  color: var(--sc-text-secondary);
}

.track-card__footer {
  display: flex;
  gap: 0.5rem;
}
</style>
