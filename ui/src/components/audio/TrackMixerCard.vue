<template>
  <div class="track-card" :class="[`track-card--${track.type}`, { 'track-card--muted': track.muted }]">
    <header class="track-card__header">
      <div class="track-card__header-main">
        <div class="track-card__info">
          <p class="track-card__type">{{ trackLabels[track.type] }}</p>
          <p class="track-card__title">{{ track.asset?.name || '未选择' }}</p>
        </div>
        <div class="track-card__actions">
          <n-button
            text
            size="tiny"
            @click="toggleSolo"
            :type="track.solo ? 'info' : 'primary'"
            :disabled="isReadOnly"
          >
            {{ track.solo ? '取消独奏' : '独奏' }}
          </n-button>
          <n-button text size="tiny" @click="toggleMute" :disabled="isReadOnly">
            {{ track.muted ? '取消静音' : '静音' }}
          </n-button>
        </div>
      </div>
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
    </header>

    <section class="track-card__body">
      <div class="track-card__transport">
        <n-button
          class="track-card__primary-action"
          size="tiny"
          circle
          :class="{ 'track-card__primary-action--active': isTrackPlaying }"
          :disabled="!track.assetId || isReadOnly"
          :aria-label="isTrackPlaying ? '暂停' : '播放'"
          @click="togglePlay"
        >
          <template #icon><n-icon :component="isTrackPlaying ? PlayerPause : PlayerPlay" /></template>
        </n-button>
        <n-button
          class="track-card__mode-action"
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
        <span class="track-card__section-label">播放进度</span>
        <div class="track-card__progress-row">
          <span class="track-card__progress-time">{{ formatTime(currentSeconds) }}</span>
          <n-slider
            class="track-card__progress-slider track-card__progress-slider--primary"
            :value="progressPercent"
            :step="0.5"
            :disabled="!track.assetId || isReadOnly"
            :format-tooltip="formatProgressTooltip"
            @update:value="handleSeek"
          />
          <span class="track-card__progress-time">{{ formatTime(track.duration) }}</span>
        </div>
      </div>

      <div class="track-card__control track-card__control--volume">
        <span class="track-card__control-icon" aria-hidden="true">🔉</span>
        <span class="track-card__section-label">音量</span>
        <n-slider
          class="track-card__control-slider track-card__volume-slider"
          :value="track.volume"
          :step="0.01"
          @update:value="setVolume"
          :min="0"
          :max="1"
          :disabled="isReadOnly"
        />
        <span class="track-card__control-icon track-card__control-icon--right" aria-hidden="true">🔊</span>
        <span class="track-card__control-value">{{ Math.round(track.volume * 100) }}%</span>
      </div>

      <div class="track-card__fade">
        <div class="track-card__control track-card__control--fade">
          <span class="track-card__control-icon" aria-hidden="true">↘</span>
          <span class="track-card__section-label">淡入</span>
          <n-slider
            class="track-card__control-slider track-card__fade-slider"
            :value="track.fadeIn"
            :step="100"
            :min="0"
            :max="10000"
            :format-tooltip="formatFadeTooltip"
            :disabled="isReadOnly"
            @update:value="setFadeIn"
          />
          <span class="track-card__control-value">{{ (track.fadeIn / 1000).toFixed(1) }}s</span>
        </div>
        <div class="track-card__control track-card__control--fade">
          <span class="track-card__control-icon" aria-hidden="true">↗</span>
          <span class="track-card__section-label">淡出</span>
          <n-slider
            class="track-card__control-slider track-card__fade-slider"
            :value="track.fadeOut"
            :step="100"
            :min="0"
            :max="10000"
            :format-tooltip="formatFadeTooltip"
            :disabled="isReadOnly"
            @update:value="setFadeOut"
          />
          <span class="track-card__control-value">{{ (track.fadeOut / 1000).toFixed(1) }}s</span>
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
import { PlayerPause, PlayerPlay } from '@vicons/tabler';
import type { TrackRuntime } from '@/stores/audioStudio';
import type { AudioAsset, PlaylistMode } from '@/types/audio';
import { useAudioStudioStore } from '@/stores/audioStudio';
import { isTrackPlaybackActive } from '@/stores/audioPlaybackState';

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
const isTrackPlaying = computed(() => isTrackPlaybackActive(props.track));
const progressPercent = computed(() => Math.round(props.track.progress * 100));
const currentSeconds = computed(() => {
  const duration = props.track.duration || 0;
  return duration * props.track.progress;
});

const selectableAssets = computed(() => (
  props.track.playlistFolderId ? props.track.playlistAssets : audio.trackSelectableAssets
));
const assetsAvailable = computed(() => selectableAssets.value.length > 0);
const assetOptions = computed(() =>
  selectableAssets.value.map((asset) => ({
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
  const asset =
    selectableAssets.value.find((item) => item.id === value)
    || audio.assets.find((item) => item.id === value)
    || audio.filteredAssets.find((item) => item.id === value);
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
  --audio-control-rail: rgba(148, 163, 184, 0.2);
  --audio-control-rail-hover: rgba(148, 163, 184, 0.28);
  --audio-control-fill: rgba(148, 163, 184, 0.72);
  --audio-control-fill-hover: rgba(203, 213, 225, 0.9);
  --audio-control-handle: rgba(255, 255, 255, 0.96);
  --audio-control-handle-border: rgba(148, 163, 184, 0.45);
  --audio-progress-rail: rgba(51, 65, 85, 0.92);
  --audio-progress-fill: linear-gradient(90deg, #34d399, #14b8a6);
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
  flex-direction: column;
  align-items: stretch;
  gap: 0.625rem;
}

.track-card__header-main {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.5rem;
}

.track-card__selector {
  width: 100%;
  min-width: 0;
}

.track-card__info {
  flex: 1;
  min-width: 0;
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
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-card__actions {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
  justify-content: flex-end;
  flex-shrink: 0;
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

.track-card__primary-action {
  flex-shrink: 0;
  --n-color: rgba(148, 163, 184, 0.12);
  --n-color-hover: rgba(148, 163, 184, 0.18);
  --n-color-pressed: rgba(148, 163, 184, 0.22);
  --n-border: 1px solid rgba(148, 163, 184, 0.18);
  --n-border-hover: 1px solid rgba(148, 163, 184, 0.26);
  --n-border-pressed: 1px solid rgba(148, 163, 184, 0.32);
  --n-text-color: rgba(226, 232, 240, 0.86);
  --n-text-color-hover: rgba(248, 250, 252, 0.96);
  --n-text-color-pressed: rgba(248, 250, 252, 0.96);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  transition: transform 0.16s ease, box-shadow 0.16s ease, border-color 0.16s ease, background 0.16s ease;
}

.track-card__primary-action:hover {
  transform: translateY(-1px);
}

.track-card__primary-action--active {
  --n-color: rgba(20, 184, 166, 0.12);
  --n-color-hover: rgba(20, 184, 166, 0.18);
  --n-color-pressed: rgba(20, 184, 166, 0.22);
  --n-border: 1px solid rgba(45, 212, 191, 0.3);
  --n-border-hover: 1px solid rgba(94, 234, 212, 0.4);
  --n-border-pressed: 1px solid rgba(94, 234, 212, 0.44);
  --n-text-color: #ccfbf1;
  --n-text-color-hover: #f0fdfa;
  --n-text-color-pressed: #f0fdfa;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 0 0 1px rgba(45, 212, 191, 0.08), 0 0 16px rgba(20, 184, 166, 0.1);
}

.track-card__mode-action {
  opacity: 0.92;
}

.track-card__speed {
  width: 96px;
  flex-shrink: 0;
}

.track-card__progress {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.track-card__progress-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 0.625rem;
}

.track-card__progress-time {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
  font-variant-numeric: tabular-nums;
}

.track-card__progress-slider {
  width: 100%;
}

.track-card__section-label {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #a0aec0);
  white-space: nowrap;
}

.track-card__control {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  color: var(--sc-text-secondary, #a0aec0);
}

.track-card__control-icon {
  width: 1rem;
  font-size: 0.88rem;
  line-height: 1;
  text-align: center;
  color: rgba(148, 163, 184, 0.88);
  flex-shrink: 0;
}

.track-card__control-icon--right {
  color: rgba(203, 213, 225, 0.92);
}

.track-card__control-slider {
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

.track-card__control-value {
  width: 3rem;
  flex-shrink: 0;
  text-align: right;
  font-size: 0.75rem;
  color: rgba(203, 213, 225, 0.92);
  font-variant-numeric: tabular-nums;
}

.track-card__fade {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.track-card__progress-slider--primary {
  --n-rail-height: 12px;
  --n-rail-color: var(--audio-progress-rail);
  --n-rail-color-hover: var(--audio-progress-rail);
  --n-fill-color: var(--audio-progress-fill);
  --n-fill-color-hover: var(--audio-progress-fill);
  --n-handle-size: 12px;
}

:deep(.track-card__progress-slider--primary .n-slider-rail) {
  height: 12px;
  border-radius: 999px;
}

:deep(.track-card__progress-slider--primary .n-slider-rail__fill) {
  border-radius: 999px;
}

:deep(.track-card__progress-slider--primary .n-slider-handles) {
  pointer-events: none;
}

:deep(.track-card__progress-slider--primary .n-slider-handle-wrapper) {
  opacity: 0;
}

:deep(.track-card__progress-slider--primary .n-slider-handle) {
  display: none;
}

:deep(.track-card__control-slider .n-slider-rail) {
  height: 4px;
}

:deep(.track-card__control-slider .n-slider-handle) {
  background-color: var(--audio-control-handle);
  border: 1px solid var(--audio-control-handle-border);
  box-sizing: border-box;
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
