<template>
  <div class="upload-panel" v-if="audio.canManage">
    <header>
      <h4>上传音频</h4>
      <p>支持 OGG/MP3/WAV（建议 OGG/Opus 以降低带宽）</p>
    </header>

    <div class="upload-panel__scope" v-if="audio.isSystemAdmin">
      <n-radio-group v-model:value="uploadScope" size="small">
        <n-radio-button value="common">通用级</n-radio-button>
        <n-radio-button value="world">世界级</n-radio-button>
      </n-radio-group>
      <span class="scope-hint" v-if="uploadScope === 'common'">所有世界可用</span>
      <span class="scope-hint" v-else-if="audio.currentWorldId">仅当前世界可用</span>
      <span class="scope-hint scope-hint--warn" v-else>请先进入一个世界</span>
    </div>
    <div class="upload-panel__scope upload-panel__scope--readonly" v-else-if="audio.canManageCurrentWorld">
      <span class="scope-badge scope-badge--world">世界级</span>
      <span class="scope-hint">上传的音频仅当前世界可用</span>
    </div>

    <label class="upload-panel__drop" @dragover.prevent @drop.prevent="handleDrop">
      <input type="file" multiple accept="audio/*" @change="handleChange" />
      <span>拖拽文件或点击选择</span>
    </label>

    <div class="upload-panel__tasks" v-if="audio.uploadTasks.length">
      <div class="upload-panel__tasks-header">
        <span>上传队列 ({{ audio.uploadTasks.length }})</span>
        <div class="upload-panel__tasks-actions">
          <n-button text size="tiny" @click="clearCompleted" v-if="hasCompletedTasks">
            清除已完成
          </n-button>
          <n-button text size="tiny" type="error" @click="clearAll">
            全部清除
          </n-button>
        </div>
      </div>
      <div v-for="task in audio.uploadTasks" :key="task.id" class="upload-task">
        <div class="upload-task__info">
          <strong class="upload-task__filename">{{ task.filename }}</strong>
          <div class="upload-task__meta">
            <n-tag :type="getStatusType(task.status)" size="small">
              {{ getStatusLabel(task.status) }}
            </n-tag>
            <span v-if="task.retryCount" class="upload-task__retry">
              重试 {{ task.retryCount }}/2
            </span>
          </div>
        </div>
        <p v-if="task.error" class="upload-task__error">{{ task.error }}</p>
        <div class="upload-task__actions" v-if="task.status === 'success' || task.status === 'error'">
          <n-button text size="tiny" @click="removeTask(task.id)">移除</n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { NRadioGroup, NRadioButton, NButton, NTag } from 'naive-ui';
import { useMessage } from 'naive-ui';
import { useAudioStudioStore } from '@/stores/audioStudio';
import type { AudioAssetScope, UploadTaskState } from '@/types/audio';

const audio = useAudioStudioStore();
const message = useMessage();

const uploadScope = ref<AudioAssetScope>(audio.isSystemAdmin ? 'common' : 'world');

const uploadOptions = computed(() => ({
  scope: uploadScope.value,
  worldId: uploadScope.value === 'world' ? audio.currentWorldId ?? undefined : undefined,
}));

const canUpload = computed(() => {
  if (uploadScope.value === 'world' && !audio.currentWorldId) {
    return false;
  }
  return true;
});

const hasCompletedTasks = computed(() => {
  return audio.uploadTasks.some((t) => t.status === 'success' || t.status === 'error');
});

function getStatusLabel(status: UploadTaskState['status']): string {
  switch (status) {
    case 'pending':
      return '等待中';
    case 'uploading':
      return '上传中';
    case 'transcoding':
      return '转码中';
    case 'success':
      return '完成';
    case 'error':
      return '失败';
    default:
      return status;
  }
}

function getStatusType(status: UploadTaskState['status']): 'default' | 'info' | 'success' | 'warning' | 'error' {
  switch (status) {
    case 'pending':
      return 'default';
    case 'uploading':
      return 'info';
    case 'transcoding':
      return 'warning';
    case 'success':
      return 'success';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
}

function handleChange(event: Event) {
  const target = event.target as HTMLInputElement;
  if (target.files) {
    if (!canUpload.value) {
      message.warning('请先进入一个世界后再上传世界级音频');
      target.value = '';
      return;
    }
    audio.handleUpload(target.files, uploadOptions.value);
    target.value = '';
  }
}

function handleDrop(event: DragEvent) {
  if (event.dataTransfer?.files?.length) {
    if (!canUpload.value) {
      message.warning('请先进入一个世界后再上传世界级音频');
      return;
    }
    audio.handleUpload(event.dataTransfer.files, uploadOptions.value);
  }
}

function removeTask(taskId: string) {
  audio.removeUploadTask(taskId);
}

function clearCompleted() {
  audio.clearCompletedUploadTasks();
}

function clearAll() {
  audio.clearAllUploadTasks();
}
</script>

<style scoped lang="scss">
.upload-panel {
  border: 1px dashed rgba(226, 232, 240, 0.3);
  border-radius: 12px;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.upload-panel__scope {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0;
}

.upload-panel__scope--readonly {
  color: var(--sc-text-secondary);
}

.scope-badge {
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
}

.scope-badge--world {
  background: rgba(99, 179, 237, 0.2);
  color: #63b3ed;
}

.scope-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.scope-hint--warn {
  color: #f6ad55;
}

.upload-panel__drop {
  height: 120px;
  border: 1px dashed rgba(99, 179, 237, 0.6);
  border-radius: 12px;
  display: flex;
  justify-content: center;
  align-items: center;
  color: var(--sc-text-secondary);
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
}

.upload-panel__drop:hover {
  border-color: rgba(99, 179, 237, 0.9);
  background: rgba(99, 179, 237, 0.05);
}

.upload-panel__drop input {
  display: none;
}

.upload-panel__tasks {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.upload-panel__tasks-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  padding-bottom: 0.25rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.upload-panel__tasks-actions {
  display: flex;
  gap: 0.5rem;
}

.upload-task {
  padding: 0.5rem 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.upload-task:last-child {
  border-bottom: none;
}

.upload-task__info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.35rem;
}

.upload-task__filename {
  font-size: 0.85rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 200px;
}

.upload-task__meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.upload-task__retry {
  font-size: 0.7rem;
  color: var(--sc-text-secondary);
}

.upload-task__error {
  color: #feb2b2;
  font-size: 0.75rem;
  margin: 0.25rem 0 0;
}

.upload-task__actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 0.25rem;
}
</style>
