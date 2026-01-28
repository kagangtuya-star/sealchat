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
          :disabled="!assetsAvailable || isReadOnly"
          @update:value="handleSelect"
        />
        <n-button text size="tiny" @click="toggleSolo" :type="track.solo ? 'info' : 'primary'" :disabled="isReadOnly">
          {{ track.solo ? '取消独奏' : '独奏' }}
        </n-button>
        <n-button text size="tiny" @click="toggleMute" :disabled="isReadOnly">
          {{ track.muted ? '取消静音' : '静音' }}
        </n-button>
      </div>
    </header>

    <section class="track-card__body">
      <div class="track-card__transport">
        <n-button
          size="tiny"
          :type="isTrackPlaying ? 'warning' : 'primary'"
          :disabled="!track.assetId || isReadOnly"
          @click="togglePlay"
        >
          {{ isTrackPlaying ? '暂停' : '播放' }}
        </n-button>
        <n-button
          size="tiny"
          :type="track.loopEnabled ? 'info' : 'default'"
          quaternary
          :disabled="!track.assetId || isReadOnly"
          @click="toggleLoop"
        >
          {{ track.loopEnabled ? '循环' : '单次' }}
        </n-button>
        <n-select
          class="track-card__speed"
          size="tiny"
          :value="track.playbackRate || 1"
          :options="speedOptions"
          :disabled="!track.assetId || isReadOnly"
          @update:value="setPlaybackRate"
        />
        <n-button
          size="tiny"
          type="error"
          quaternary
          :disabled="!track.assetId || isReadOnly"
          @click="clearTrack"
        >
          清空
        </n-button>
      </div>

      <div class="track-card__progress">
        <n-slider
          :value="progressPercent"
          :step="0.5"
          :disabled="!track.assetId || isReadOnly"
          :format-tooltip="formatProgressTooltip"
          @update:value="handleSeek"
        />
        <div class="track-card__progress-meta">
          <span>{{ formatTime(currentSeconds) }}</span>
          <span>{{ formatTime(track.duration) }}</span>
        </div>
      </div>

      <div class="track-card__volume">
        <span>音量</span>
        <n-slider
          :value="track.volume"
          :step="0.01"
          @update:value="setVolume"
          :min="0"
          :max="1"
          :disabled="isReadOnly"
        ></n-slider>
      </div>

      <div class="track-card__fade">
        <div class="track-card__fade-item">
          <span>淡入</span>
          <n-slider
            :value="track.fadeIn"
            :step="100"
            :min="0"
            :max="10000"
            :format-tooltip="formatFadeTooltip"
            :disabled="isReadOnly"
            @update:value="setFadeIn"
          />
          <span class="track-card__fade-value">{{ (track.fadeIn / 1000).toFixed(1) }}s</span>
        </div>
        <div class="track-card__fade-item">
          <span>淡出</span>
          <n-slider
            :value="track.fadeOut"
            :step="100"
            :min="0"
            :max="10000"
            :format-tooltip="formatFadeTooltip"
            :disabled="isReadOnly"
            @update:value="setFadeOut"
          />
          <span class="track-card__fade-value">{{ (track.fadeOut / 1000).toFixed(1) }}s</span>
        </div>
      </div>

      <div class="track-card__playlist" v-if="!isReadOnly">
        <div class="track-card__playlist-header">
          <span>播放列表</span>
          <n-tag v-if="track.playlistAssetIds?.length" size="small" type="info">
            {{ track.playlistIndex + 1 }}/{{ track.playlistAssetIds.length }}
          </n-tag>
        </div>
        <div class="track-card__playlist-controls">
          <n-select
            class="track-card__folder-select"
            size="small"
            placeholder="选择文件夹"
            :value="track.playlistFolderId"
            :options="folderOptions"
            clearable
            @update:value="handleFolderChange"
          />
          <n-select
            class="track-card__mode-select"
            size="small"
            placeholder="播放模式"
            :value="track.playlistMode"
            :options="playlistModeOptions"
            :disabled="!track.playlistFolderId"
            clearable
            @update:value="handleModeChange"
          />
        </div>
        <div class="track-card__playlist-nav" v-if="track.playlistAssetIds?.length">
          <n-button size="tiny" quaternary @click="handlePrev" :disabled="!track.playlistMode">上一曲</n-button>
          <n-button size="tiny" quaternary @click="handleNext" :disabled="!track.playlistMode">下一曲</n-button>
        </div>
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
import type { AudioAsset, PlaylistMode } from '@/types/audio';
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

const playlistModeOptions = [
  { label: '单曲循环', value: 'single' },
  { label: '顺序播放', value: 'sequential' },
  { label: '随机播放', value: 'shuffle' },
];

const speedOptions = [
  { label: '0.5x', value: 0.5 },
  { label: '0.75x', value: 0.75 },
  { label: '1x', value: 1 },
  { label: '1.25x', value: 1.25 },
  { label: '1.5x', value: 1.5 },
  { label: '2x', value: 2 },
];

const audio = useAudioStudioStore();
const isReadOnly = computed(() => !audio.canManage);
const isTrackPlaying = computed(() => props.track.status === 'playing');
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

const folderOptions = computed(() => {
  const flattenFolders = (folders: typeof audio.folders, prefix = ''): { label: string; value: string }[] => {
    const result: { label: string; value: string }[] = [];
    for (const folder of folders) {
      const label = prefix ? `${prefix}/${folder.name}` : folder.name;
      result.push({ label, value: folder.id });
      if (folder.children?.length) {
        result.push(...flattenFolders(folder.children, label));
      }
    }
    return result;
  };
  return flattenFolders(audio.folders);
});

function formatTime(value: number) {
  if (!value || Number.isNaN(value)) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function formatProgressTooltip(val: number) {
  const duration = props.track.duration || 0;
  if (!duration) return '00:00';
  return formatTime((val / 100) * duration);
}

function formatFadeTooltip(val: number) {
  return `${(val / 1000).toFixed(1)}s`;
}

function handleSeek(value: number) {
  const duration = props.track.duration || 0;
  if (!duration) return;
  audio.seekTrack(props.track.type, (value / 100) * duration);
}

function togglePlay() {
  audio.toggleTrackPlay(props.track.type);
}

function clearTrack() {
  audio.clearTrack(props.track.type);
}

function setVolume(value: number) {
  audio.setTrackVolume(props.track.type, value);
}

function setFadeIn(value: number) {
  audio.setTrackFadeIn(props.track.type, value);
}

function setFadeOut(value: number) {
  audio.setTrackFadeOut(props.track.type, value);
}

function toggleMute() {
  audio.toggleTrackMute(props.track.type);
}

function toggleSolo() {
  audio.toggleTrackSolo(props.track.type);
}

function toggleLoop() {
  audio.toggleTrackLoop(props.track.type);
}

function setPlaybackRate(value: number) {
  audio.setTrackPlaybackRate(props.track.type, value);
}

function handleSelect(value: string | null) {
  if (!value) return;
  const asset = audio.assets.find((item) => item.id === value) || audio.filteredAssets.find((item) => item.id === value);
  if (asset) {
    audio.assignAssetToTrack(props.track.type, asset as AudioAsset);
  }
}

function handleFolderChange(value: string | null) {
  audio.setTrackPlaylistFolder(props.track.type, value);
}

function handleModeChange(value: PlaylistMode | null) {
  audio.setTrackPlaylistMode(props.track.type, value);
}

function handlePrev() {
  audio.playPrevInPlaylist(props.track.type);
}

function handleNext() {
  audio.playNextInPlaylist(props.track.type);
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

.track-card__transport {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
}

.track-card__speed {
  width: 96px;
  flex-shrink: 0;
}

.track-card__progress {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.track-card__progress-meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
}

.track-card__volume {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  color: var(--sc-text-secondary);
}

.track-card__fade {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.track-card__fade-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.track-card__fade-item > span:first-child {
  width: 2rem;
  flex-shrink: 0;
}

.track-card__fade-item .n-slider {
  flex: 1;
}

.track-card__fade-value {
  width: 3rem;
  text-align: right;
  flex-shrink: 0;
}

.track-card__footer {
  display: flex;
  gap: 0.5rem;
}

.track-card__playlist {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.track-card__playlist-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.track-card__playlist-controls {
  display: flex;
  gap: 0.5rem;
}

.track-card__folder-select {
  flex: 1;
  min-width: 100px;
}

.track-card__mode-select {
  width: 100px;
  flex-shrink: 0;
}

.track-card__playlist-nav {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
}
</style>

<!-- 非 scoped 样式用于自定义主题覆盖 -->
<style lang="scss">
:root[data-custom-theme='true'] .track-card.track-card {
  background: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-mute) !important;
  box-shadow: none !important;
}

:root[data-custom-theme='true'] .track-card.track-card .progress-shell {
  background: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .track-card.track-card .progress-buffer {
  background: rgba(255, 255, 255, 0.15) !important;
}
</style>
