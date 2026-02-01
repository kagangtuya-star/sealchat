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

    <div class="upload-panel__import" v-if="audio.importEnabled">
      <n-button size="small" secondary @click="openImportDialog">读取数据目录</n-button>
      <span class="upload-panel__import-hint">读取服务器导入目录内的音频文件</span>
    </div>

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

    <n-modal v-model:show="importDialogVisible" preset="card" title="读取数据目录" style="width: min(720px, 92vw)">
      <div class="import-preview__summary">
        <span>总计 {{ importTotal }} · 可导入 {{ importValid }} · 不可导入 {{ importInvalid }}</span>
        <n-button text size="tiny" :loading="audio.importPreviewLoading" @click="refreshImportPreview">刷新</n-button>
      </div>
      <n-alert v-if="audio.importError" type="error" :show-icon="false" class="import-preview__alert">
        {{ audio.importError }}
      </n-alert>
      <div v-if="audio.importPreviewLoading" class="import-preview__loading">
        <n-spin size="small" />
        <span>正在读取导入目录...</span>
      </div>
      <div v-else-if="!importItems.length" class="import-preview__empty">未发现可识别的音频文件</div>
      <n-checkbox-group v-else v-model:value="importSelection">
        <n-space vertical size="small" class="import-preview__list">
          <n-checkbox v-for="item in importItems" :key="item.path" :value="item.path" :disabled="!item.valid">
            <div class="import-item">
              <div class="import-item__title">
                <span class="import-item__name">{{ item.name }}</span>
                <n-tag size="small" :type="item.valid ? 'success' : 'error'">{{ item.valid ? '可导入' : '不可导入' }}</n-tag>
              </div>
              <div class="import-item__meta">
                {{ formatFileSize(item.size) }} · {{ item.mimeType || '未知类型' }}
                <span v-if="item.modTime"> · {{ formatDate(item.modTime) }}</span>
              </div>
              <p v-if="!item.valid" class="import-item__reason">{{ item.reason }}</p>
            </div>
          </n-checkbox>
        </n-space>
      </n-checkbox-group>
      <div v-if="importResultSummary" class="import-preview__result">{{ importResultSummary }}</div>
      <template #action>
        <n-space justify="end" wrap>
          <n-button size="small" @click="selectAllImports" :disabled="!importItems.length">全选</n-button>
          <n-button size="small" @click="clearImportSelection" :disabled="!importItems.length">清空</n-button>
          <n-button size="small" secondary :loading="audio.importLoading" @click="handleImportSelected">导入选中</n-button>
          <n-button size="small" type="primary" :loading="audio.importLoading" @click="handleImportAll">导入全部</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { NAlert, NCheckbox, NCheckboxGroup, NModal, NRadioGroup, NRadioButton, NButton, NTag, NSpin, NSpace } from 'naive-ui';
import { useMessage } from 'naive-ui';
import { useAudioStudioStore } from '@/stores/audioStudio';
import type { AudioAssetScope, UploadTaskState } from '@/types/audio';

const audio = useAudioStudioStore();
const message = useMessage();

const uploadScope = ref<AudioAssetScope>(audio.isSystemAdmin ? 'common' : 'world');
const importDialogVisible = ref(false);
const importSelection = ref<string[]>([]);

const uploadOptions = computed(() => ({
  scope: uploadScope.value,
  worldId: uploadScope.value === 'world' ? audio.currentWorldId ?? undefined : undefined,
  folderId: audio.filters.folderId ?? undefined,
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

const importItems = computed(() => audio.importPreview?.items || []);
const importTotal = computed(() => audio.importPreview?.total || 0);
const importValid = computed(() => audio.importPreview?.valid || 0);
const importInvalid = computed(() => audio.importPreview?.invalid || 0);
const importResultSummary = computed(() => {
  const result = audio.importResult;
  if (!result) return '';
  const warnings = result.imported.filter((item) => item.warning).length;
  const parts = [
    `成功 ${result.imported.length}`,
    `失败 ${result.failed.length}`,
    `跳过 ${result.skipped.length}`,
  ];
  if (warnings) {
    parts.push(`清理警告 ${warnings}`);
  }
  return `导入结果：${parts.join(' · ')}`;
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

function formatFileSize(value: number) {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatDate(value: number) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString();
}

async function openImportDialog() {
  if (!canUpload.value) {
    message.warning('请先进入一个世界后再导入世界级音频');
    return;
  }
  importDialogVisible.value = true;
  importSelection.value = [];
  await audio.previewImport();
}

async function refreshImportPreview() {
  importSelection.value = [];
  await audio.previewImport();
}

function selectAllImports() {
  importSelection.value = importItems.value.filter((item) => item.valid).map((item) => item.path);
}

function clearImportSelection() {
  importSelection.value = [];
}

async function handleImportSelected() {
  if (!importSelection.value.length) {
    message.warning('请先选择要导入的文件');
    return;
  }
  if (!canUpload.value) {
    message.warning('请先进入一个世界后再导入世界级音频');
    return;
  }
  const result = await audio.importFromDir({
    all: false,
    paths: importSelection.value,
    ...uploadOptions.value,
  });
  if (result) {
    message.success(`导入完成：成功 ${result.imported.length}，失败 ${result.failed.length}，跳过 ${result.skipped.length}`);
  }
}

async function handleImportAll() {
  if (!canUpload.value) {
    message.warning('请先进入一个世界后再导入世界级音频');
    return;
  }
  const result = await audio.importFromDir({
    all: true,
    ...uploadOptions.value,
  });
  if (result) {
    message.success(`导入完成：成功 ${result.imported.length}，失败 ${result.failed.length}，跳过 ${result.skipped.length}`);
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

.upload-panel__import {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.upload-panel__import-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
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

.import-preview__summary {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  margin-bottom: 0.5rem;
}

.import-preview__alert {
  margin-bottom: 0.5rem;
}

.import-preview__loading {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
  padding: 0.75rem 0;
}

.import-preview__empty {
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
  padding: 0.75rem 0;
}

.import-preview__list {
  max-height: 360px;
  overflow: auto;
  padding-right: 0.5rem;
}

.import-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.import-item__title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
}

.import-item__name {
  font-size: 0.9rem;
}

.import-item__meta {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.import-item__reason {
  margin: 0;
  font-size: 0.75rem;
  color: #fca5a5;
}

.import-preview__result {
  margin-top: 0.5rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
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
