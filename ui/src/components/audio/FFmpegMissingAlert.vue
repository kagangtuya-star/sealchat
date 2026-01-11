<template>
  <div class="ffmpeg-missing">
    <div class="ffmpeg-missing__icon">
      <n-icon :size="48" :component="MusicalNotesOutline" />
    </div>
    <h3 class="ffmpeg-missing__title">音频工作台暂不可用</h3>
    <p class="ffmpeg-missing__desc">
      服务器未检测到 FFmpeg，音频工作台功能已禁用。建议同时提供 FFprobe 用于时长探测。
    </p>
    <div class="ffmpeg-missing__action">
      <n-button type="primary" tag="a" :href="downloadUrl" target="_blank" rel="noopener noreferrer">
        <template #icon>
          <n-icon :component="DownloadOutline" />
        </template>
        下载 FFmpeg
      </n-button>
    </div>
    <p class="ffmpeg-missing__hint">
      下载后将 <code>ffmpeg</code> 与 <code>ffprobe</code>（Windows 为 <code>.exe</code>）放入程序根目录（<code>sealchat-server.exe</code> 路径），重启服务即可启用。
    </p>
  </div>
</template>

<script setup lang="ts">
import { NButton, NIcon } from 'naive-ui';
import { MusicalNotesOutline, DownloadOutline } from '@vicons/ionicons5';

const downloadUrl = 'https://github.com/BtbN/FFmpeg-Builds/releases';
</script>

<style scoped lang="scss">
.ffmpeg-missing {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 3rem 2rem;
  min-height: 320px;
  color: var(--sc-text-secondary);
}

.ffmpeg-missing__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: var(--sc-bg-muted, rgba(148, 163, 184, 0.12));
  margin-bottom: 1.25rem;
  color: var(--sc-text-tertiary, #94a3b8);
}

.ffmpeg-missing__title {
  margin: 0 0 0.5rem;
  font-size: 1.125rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.ffmpeg-missing__desc {
  margin: 0 0 1.25rem;
  font-size: 0.875rem;
  line-height: 1.5;
  max-width: 280px;
}

.ffmpeg-missing__action {
  margin-bottom: 1rem;
}

.ffmpeg-missing__hint {
  margin: 0;
  font-size: 0.75rem;
  color: var(--sc-text-tertiary, #94a3b8);
  max-width: 320px;
  line-height: 1.4;
}

.ffmpeg-missing__hint code {
  padding: 0.125rem 0.375rem;
  background: var(--sc-bg-muted, rgba(148, 163, 184, 0.15));
  border-radius: 4px;
  font-size: 0.7rem;
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Consolas, 'Liberation Mono', Menlo, monospace;
}

:root[data-display-palette='night'] .ffmpeg-missing__icon {
  background: rgba(71, 85, 105, 0.35);
  color: rgba(148, 163, 184, 0.85);
}

:root[data-display-palette='night'] .ffmpeg-missing__hint code {
  background: rgba(71, 85, 105, 0.45);
}
</style>
