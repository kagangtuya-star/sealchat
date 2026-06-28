<template>
  <div :class="['upload-panel', { 'upload-panel--compact': props.compact }]" v-if="audio.canManage">
    <header>
      <h4>上传音频</h4>
      <p>支持 OGG/MP3/WAV</p>
    </header>

    <div class="upload-panel__scope" v-if="audio.isSystemAdmin">
      <div class="upload-panel__scope-main">
        <n-radio-group v-model:value="uploadScope" size="small" class="upload-panel__scope-group">
          <n-radio-button value="common">通用级</n-radio-button>
          <n-radio-button value="world">世界级</n-radio-button>
        </n-radio-group>
        <span class="scope-hint" v-if="uploadScope === 'common'">所有世界可用</span>
        <span class="scope-hint" v-else-if="audio.currentWorldId">仅当前世界可用</span>
        <span class="scope-hint scope-hint--warn" v-else>请先进入一个世界</span>
      </div>
      <div v-if="quotaSummary" class="upload-panel__quota-inline">
        <span class="upload-panel__quota-text">{{ quotaInlineText }}</span>
        <div class="upload-panel__quota-bar" :class="{ 'is-unlimited': !quotaSummary.limited, 'is-overflow': quotaOverflow }">
          <div class="upload-panel__quota-fill" :style="{ width: `${quotaProgressPercent}%` }"></div>
        </div>
      </div>
    </div>
    <div class="upload-panel__scope upload-panel__scope--readonly" v-else-if="audio.canManageCurrentWorld">
      <div class="upload-panel__scope-main">
        <span class="scope-badge scope-badge--world">世界级</span>
        <span class="scope-hint">上传的音频仅当前世界可用</span>
      </div>
      <div v-if="quotaSummary" class="upload-panel__quota-inline">
        <span class="upload-panel__quota-text">{{ quotaInlineText }}</span>
        <div class="upload-panel__quota-bar" :class="{ 'is-unlimited': !quotaSummary.limited, 'is-overflow': quotaOverflow }">
          <div class="upload-panel__quota-fill" :style="{ width: `${quotaProgressPercent}%` }"></div>
        </div>
      </div>
    </div>

    <div class="upload-panel__target">
      <div class="upload-panel__target-header">
        <span>目标素材文件夹</span>
        <n-button text size="tiny" @click="toggleCreateFolderInput">
          {{ createFolderVisible ? '取消创建' : '新建文件夹' }}
        </n-button>
      </div>
      <n-tree-select
        v-model:value="targetFolderId"
        :options="targetFolderOptions"
        clearable
        filterable
        default-expand-all
        :loading="targetFoldersLoading"
        placeholder="未分类"
      />
      <div v-if="createFolderVisible" class="upload-panel__target-create">
        <n-input
          v-model:value="createFolderName"
          placeholder="输入新建文件夹名称"
          maxlength="40"
          @keydown.enter.prevent="handleCreateTargetFolder"
        />
        <n-button size="small" type="primary" :loading="audio.folderActionLoading" @click="handleCreateTargetFolder">
          创建文件夹
        </n-button>
      </div>
      <p class="upload-panel__target-hint">
        {{ targetFolderHint }}
      </p>
    </div>

    <label class="upload-panel__drop" @dragover.prevent @drop.prevent="handleDrop">
      <input type="file" multiple accept="audio/*" @change="handleChange" />
      <span>拖拽文件或点击选择</span>
    </label>

    <div class="upload-panel__import" v-if="audio.importEnabled">
      <n-button size="small" secondary @click="openImportDialog">读取数据目录</n-button>
      <span class="upload-panel__import-hint">按目录浏览服务器导入目录，只导入当前目录直接文件</span>
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
            <span v-if="task.retryCount" class="upload-task__retry">重试 {{ task.retryCount }}/2</span>
          </div>
        </div>
        <p v-if="task.error" class="upload-task__error">{{ task.error }}</p>
        <div class="upload-task__actions" v-if="task.status === 'success' || task.status === 'error'">
          <n-button text size="tiny" @click="removeTask(task.id)">移除</n-button>
        </div>
      </div>
    </div>

    <n-modal v-model:show="importDialogVisible" preset="card" title="读取数据目录" style="width: min(680px, 96vw)">
      <div class="import-browser__toolbar">
        <div class="import-browser__toolbar-main">
          <strong>{{ currentDirectoryLabel }}</strong>
          <span>总计 {{ importTotal }} · 可导入 {{ importValid }} · 不可导入 {{ importInvalid }}</span>
        </div>
        <n-button text size="tiny" :loading="audio.importBrowseLoading" @click="refreshImportBrowser">刷新</n-button>
      </div>

      <n-alert v-if="audio.importError" type="error" :show-icon="false" class="import-browser__alert">
        {{ audio.importError }}
      </n-alert>

      <div v-if="importJobStatus" class="import-progress">
        <div class="import-progress__header">
          <span>导入进度</span>
          <n-tag size="small" :type="importJobStatusType">{{ importStatusLabel }}</n-tag>
        </div>
        <n-progress type="line" :percentage="importJobStatus.percentage || 0" :show-indicator="true" processing />
        <p class="import-progress__summary">
          已扫描 {{ importJobStatus.totalFiles }} · 已提交 {{ importJobStatus.processedFiles }} · 已完成 {{ importCompletedCount }}
        </p>
        <p v-if="importJobStatus.errorMessage" class="import-progress__error">{{ importJobStatus.errorMessage }}</p>
        <p v-else-if="importJobFailureSummary" class="import-progress__failure">{{ importJobFailureSummary }}</p>
      </div>

      <n-tabs v-model:value="importContentTab" type="segment" animated class="import-browser__tabs">
        <n-tab-pane name="tree" tab="目录树">
          <div class="import-browser__panel">
            <n-tree
              block-line
              selectable
              default-expand-all
              :data="importTreeOptions"
              :selected-keys="[importDirectoryKey]"
              @update:selected-keys="handleImportDirectorySelect"
            />
          </div>
        </n-tab-pane>
        <n-tab-pane name="files" tab="当前目录文件">
          <div class="import-browser__panel">
            <div v-if="audio.importBrowseLoading" class="import-browser__loading">
              <n-spin size="small" />
              <span>正在读取导入目录...</span>
            </div>
            <div v-else-if="!importItems.length" class="import-browser__empty">当前目录没有可显示文件</div>
            <n-checkbox-group v-else v-model:value="importSelection">
              <n-space vertical size="small" class="import-browser__list">
                <n-checkbox v-for="item in importItems" :key="item.path" :value="item.path" :disabled="!item.valid">
                  <div class="import-item">
                    <div class="import-item__title">
                      <span class="import-item__name">{{ item.name }}</span>
                      <n-tag size="small" :type="item.valid ? 'success' : 'error'">
                        {{ item.valid ? '可导入' : '不可导入' }}
                      </n-tag>
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
          </div>
        </n-tab-pane>
      </n-tabs>

      <template #action>
        <n-space justify="space-between" wrap>
          <n-space>
            <n-button size="small" @click="selectAllImports" :disabled="importContentTab !== 'files' || !importItems.length">全选可导入</n-button>
            <n-button size="small" @click="clearImportSelection" :disabled="importContentTab !== 'files' || !importSelection.length">清空</n-button>
          </n-space>
          <n-space>
            <n-button
              size="small"
              secondary
              :loading="audio.importJobLoading"
              :disabled="importContentTab !== 'files' || !importSelection.length"
              @click="handleImportSelected"
            >
              导入选中
            </n-button>
            <n-button
              size="small"
              type="primary"
              :loading="audio.importJobLoading"
              :disabled="!importItems.length"
              @click="handleImportCurrentDirectory"
            >
              导入当前目录全部
            </n-button>
          </n-space>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import {
  NAlert,
  NButton,
  NCheckbox,
  NCheckboxGroup,
  NInput,
  NModal,
  NProgress,
  NRadioButton,
  NRadioGroup,
  NSpace,
  NSpin,
  NTabPane,
  NTag,
  NTabs,
  NTree,
  NTreeSelect,
  useMessage,
  type TreeOption,
} from 'naive-ui';
import { useAudioStudioStore } from '@/stores/audioStudio';
import type { AudioAssetScope, AudioFolder, UploadTaskState } from '@/types/audio';

const ROOT_DIRECTORY_KEY = '__root__';

const props = defineProps<{
  compact?: boolean;
}>();

const audio = useAudioStudioStore();
const message = useMessage();

const importDialogVisible = ref(false);
const importSelection = ref<string[]>([]);
const importContentTab = ref<'tree' | 'files'>('files');
const importDirectoryKey = ref(ROOT_DIRECTORY_KEY);
const uploadScope = ref<AudioAssetScope>(resolveDefaultUploadScope());
const targetFolders = ref<AudioFolder[]>([]);
const targetFolderId = ref<string | null>(null);
const targetFoldersLoading = ref(false);
const createFolderVisible = ref(false);
const createFolderName = ref('');
const finishedImportJobId = ref('');

const quotaSummary = computed(() => audio.quotaSummary);
const quotaOverflow = computed(() => {
  if (!quotaSummary.value?.limited) return false;
  const quotaBytes = quotaSummary.value.quotaBytes ?? 0;
  return quotaBytes > 0 && quotaSummary.value.usedBytes > quotaBytes;
});
const quotaProgressPercent = computed(() => {
  if (!quotaSummary.value) return 0;
  if (!quotaSummary.value.limited) return 100;
  return Math.max(0, Math.min(100, quotaSummary.value.usagePercent ?? 0));
});
const quotaInlineText = computed(() => {
  if (!quotaSummary.value) return '';
  if (!quotaSummary.value.limited) {
    return '管理员无上限';
  }
  if (quotaOverflow.value) {
    const overflowBytes = Math.max(0, quotaSummary.value.usedBytes - (quotaSummary.value.quotaBytes ?? 0));
    return `已超限 ${formatFileSize(overflowBytes)} · ${(quotaSummary.value.usagePercent ?? 0).toFixed(1)}%`;
  }
  return `剩余 ${formatFileSize(quotaSummary.value.remainingBytes ?? 0)} · ${(quotaSummary.value.usagePercent ?? 0).toFixed(1)}%`;
});
const canUpload = computed(() => uploadScope.value !== 'world' || Boolean(audio.currentWorldId));
const uploadOptions = computed(() => ({
  scope: uploadScope.value,
  worldId: uploadScope.value === 'world' ? audio.currentWorldId ?? undefined : undefined,
  folderId: targetFolderId.value ?? undefined,
}));
const targetFolderOptions = computed<TreeOption[]>(() => buildFolderOptions(targetFolders.value));
const targetFolderHint = computed(() => {
  if (uploadScope.value === 'world' && !audio.currentWorldId) {
    return '世界级素材需要先进入一个世界。';
  }
  if (targetFolderId.value) {
    return '新建文件夹时会默认创建到当前选中文件夹下。';
  }
  return '未选择文件夹时，素材会进入未分类。';
});
const hasCompletedTasks = computed(() => audio.uploadTasks.some((task) => task.status === 'success' || task.status === 'error'));
const importItems = computed(() => audio.importBrowse?.items || []);
const importTotal = computed(() => audio.importBrowse?.total || 0);
const importValid = computed(() => audio.importBrowse?.valid || 0);
const importInvalid = computed(() => audio.importBrowse?.invalid || 0);
const importTreeOptions = computed<TreeOption[]>(() => [
  {
    key: ROOT_DIRECTORY_KEY,
    label: '导入根目录',
    children: buildImportTreeOptions(audio.importDirectoryTree || []),
  },
]);
const currentDirectoryLabel = computed(() => (audio.importCurrentPath ? `当前目录：${audio.importCurrentPath}` : '当前目录：导入根目录'));
const importJobStatus = computed(() => audio.importJobStatus);
const importCompletedCount = computed(() => {
  const status = importJobStatus.value;
  if (!status) return 0;
  return status.importedCount + status.skippedCount + status.failedCount;
});
const importStatusLabel = computed(() => {
  switch (importJobStatus.value?.status) {
    case 'pending':
      return '等待中';
    case 'running':
      return '导入中';
    case 'done':
      return '已完成';
    case 'failed':
      return '失败';
    default:
      return '未开始';
  }
});
const importJobStatusType = computed<'default' | 'info' | 'success' | 'error'>(() => {
  switch (importJobStatus.value?.status) {
    case 'done':
      return 'success';
    case 'failed':
      return 'error';
    case 'pending':
    case 'running':
      return 'info';
    default:
      return 'default';
  }
});
const importJobFailureSummary = computed(() => {
  const failedItems = importJobStatus.value?.failed || [];
  if (!failedItems.length) return '';
  return failedItems
    .slice(0, 3)
    .map((item) => `${item.name || item.path}: ${item.error || item.reason || '导入失败'}`)
    .join('；');
});

watch(
  () => audio.currentWorldId,
  (worldId) => {
    if (!audio.isSystemAdmin) return;
    if (!worldId && uploadScope.value === 'world') {
      uploadScope.value = 'common';
      return;
    }
    if (worldId && uploadScope.value !== 'world') {
      uploadScope.value = 'world';
    }
  }
);

watch(
  [() => uploadScope.value, () => audio.currentWorldId],
  () => {
    void loadTargetFolders();
  },
  { immediate: true }
);

watch(
  () => audio.importCurrentPath,
  (path) => {
    importDirectoryKey.value = resolveDirectoryKey(path);
  },
  { immediate: true }
);

watch(
  () => `${audio.importJobStatus?.jobId || ''}:${audio.importJobStatus?.status || ''}`,
  () => {
    const status = audio.importJobStatus;
    if (!status || (status.status !== 'done' && status.status !== 'failed')) {
      return;
    }
    if (finishedImportJobId.value === status.jobId) {
      return;
    }
    finishedImportJobId.value = status.jobId;
    if (status.status === 'done') {
      message.success(`目录导入完成，已完成 ${importCompletedCount.value} 个文件`);
    } else {
      message.error(status.errorMessage || '目录导入失败');
    }
  }
);

onUnmounted(() => {
  audio.stopImportJobPolling();
});

function resolveDefaultUploadScope(): AudioAssetScope {
  if (!audio.isSystemAdmin) {
    return 'world';
  }
  return audio.currentWorldId ? 'world' : 'common';
}

function resolveDirectoryKey(path: string | null | undefined) {
  return path ? path : ROOT_DIRECTORY_KEY;
}

function buildFolderOptions(folders: AudioFolder[]): TreeOption[] {
  return folders.map((folder) => ({
    key: folder.id,
    value: folder.id,
    label: folder.name,
    children: folder.children?.length ? buildFolderOptions(folder.children) : undefined,
  }));
}

function buildImportTreeOptions(nodes: Array<{ path: string; name: string; children?: any[] }>): TreeOption[] {
  return nodes.map((node) => ({
    key: node.path,
    label: node.name,
    children: node.children?.length ? buildImportTreeOptions(node.children) : undefined,
  }));
}

function collectFolderIds(folders: AudioFolder[], bucket = new Set<string>()) {
  folders.forEach((folder) => {
    bucket.add(folder.id);
    if (folder.children?.length) {
      collectFolderIds(folder.children, bucket);
    }
  });
  return bucket;
}

async function loadTargetFolders() {
  if (!canUpload.value) {
    targetFolders.value = [];
    targetFolderId.value = null;
    return;
  }
  targetFoldersLoading.value = true;
  try {
    const folders =
      (await audio.fetchFolders({
        scope: uploadScope.value,
        worldId: uploadScope.value === 'world' ? audio.currentWorldId ?? null : null,
        includeCommon: false,
        applyState: false,
      })) || [];
    targetFolders.value = folders;
    const validIds = collectFolderIds(folders);
    if (targetFolderId.value && !validIds.has(targetFolderId.value)) {
      targetFolderId.value = null;
    }
  } finally {
    targetFoldersLoading.value = false;
  }
}

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
    uploadFiles(target.files);
    target.value = '';
  }
}

function handleDrop(event: DragEvent) {
  if (event.dataTransfer?.files?.length) {
    uploadFiles(event.dataTransfer.files);
  }
}

function uploadFiles(files: FileList) {
  if (!canUpload.value) {
    message.warning('请先进入一个世界后再上传世界级音频');
    return;
  }
  audio.handleUpload(files, uploadOptions.value);
}

function toggleCreateFolderInput() {
  createFolderVisible.value = !createFolderVisible.value;
  if (!createFolderVisible.value) {
    createFolderName.value = '';
  }
}

async function handleCreateTargetFolder() {
  const name = createFolderName.value.trim();
  if (!name) {
    message.warning('请输入文件夹名称');
    return;
  }
  if (uploadScope.value === 'world' && !audio.currentWorldId) {
    message.warning('请先进入一个世界后再创建世界级文件夹');
    return;
  }
  try {
    const folder = await audio.createFolder({
      name,
      parentId: targetFolderId.value || undefined,
      scope: uploadScope.value,
      worldId: uploadScope.value === 'world' ? audio.currentWorldId ?? undefined : undefined,
    });
    await loadTargetFolders();
    targetFolderId.value = folder?.id || targetFolderId.value;
    createFolderName.value = '';
    createFolderVisible.value = false;
    message.success('文件夹已创建');
  } catch (err) {
    console.warn(err);
    message.error('创建文件夹失败，请稍后重试');
  }
}

async function openImportDialog() {
  if (!canUpload.value) {
    message.warning('请先进入一个世界后再导入世界级音频');
    return;
  }
  importDialogVisible.value = true;
  importContentTab.value = 'files';
  await loadImportPath(audio.importCurrentPath || '');
}

async function loadImportPath(path: string) {
  importSelection.value = [];
  const browse = await audio.fetchImportBrowser(path);
  if (browse) {
    importDirectoryKey.value = resolveDirectoryKey(browse.currentPath);
  }
}

async function refreshImportBrowser() {
  await loadImportPath(audio.importCurrentPath || '');
}

async function handleImportDirectorySelect(keys: Array<string | number>) {
  const raw = keys.length ? String(keys[0]) : ROOT_DIRECTORY_KEY;
  importDirectoryKey.value = raw;
  await loadImportPath(raw === ROOT_DIRECTORY_KEY ? '' : raw);
  importContentTab.value = 'files';
}

function selectAllImports() {
  importSelection.value = importItems.value.filter((item) => item.valid).map((item) => item.path);
}

function clearImportSelection() {
  importSelection.value = [];
}

async function beginImport(all: boolean) {
  if (!canUpload.value) {
    message.warning('请先进入一个世界后再导入世界级音频');
    return;
  }
  if (!all && !importSelection.value.length) {
    message.warning('请先选择要导入的文件');
    return;
  }
  const job = await audio.startImportJob({
    directory: audio.importCurrentPath,
    all,
    paths: all ? [] : importSelection.value,
    ...uploadOptions.value,
  });
  if (!job?.jobId) {
    return;
  }
  finishedImportJobId.value = '';
  audio.startImportJobPolling(job.jobId);
  if (!all) {
    importSelection.value = [];
  }
  message.success('导入任务已提交');
}

async function handleImportSelected() {
  await beginImport(false);
}

async function handleImportCurrentDirectory() {
  await beginImport(true);
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

.upload-panel--compact {
  border-style: solid;
  padding: 0.25rem 0;
}

.upload-panel__scope {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0;
  justify-content: space-between;
  flex-wrap: wrap;
}

.upload-panel__scope-main {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 0;
}

.upload-panel__scope-group :deep(.n-radio-button),
.upload-panel__scope-group :deep(.n-radio-button__state-border) {
  box-shadow: none !important;
}

.upload-panel__scope-group :deep(.n-radio-button:hover),
.upload-panel__scope-group :deep(.n-radio-button:hover .n-radio-button__state-border),
.upload-panel__scope-group :deep(.n-radio-button:focus-within),
.upload-panel__scope-group :deep(.n-radio-button:focus-within .n-radio-button__state-border),
.upload-panel__scope-group :deep(.n-radio-button--checked),
.upload-panel__scope-group :deep(.n-radio-button--checked .n-radio-button__state-border) {
  box-shadow: none !important;
}

.upload-panel__scope--readonly {
  color: var(--sc-text-secondary);
}

.upload-panel__quota-inline {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-left: auto;
  min-width: min(100%, 240px);
}

.upload-panel__quota-text {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  white-space: nowrap;
}

.upload-panel__quota-bar {
  width: 150px;
  max-width: 36vw;
  height: 8px;
  border-radius: 999px;
  overflow: hidden;
  background: color-mix(in srgb, var(--sc-border-mute) 45%, transparent);
}

.upload-panel__quota-fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #0f766e 0%, #22c55e 100%);
}

.upload-panel__quota-bar.is-overflow .upload-panel__quota-fill {
  background: linear-gradient(90deg, #dc2626 0%, #f97316 100%);
}

.upload-panel__quota-bar.is-unlimited .upload-panel__quota-fill {
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.22) 0%, rgba(59, 130, 246, 0.38) 100%);
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

.upload-panel__target {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.upload-panel__target-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  font-size: 0.85rem;
}

.upload-panel__target-create {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.5rem;
}

.upload-panel__target-hint {
  margin: 0;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
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

.import-browser__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
}

.import-browser__toolbar-main {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
}

.import-browser__alert {
  margin-bottom: 0.75rem;
}

.import-progress {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.85rem;
  margin-bottom: 0.75rem;
  border-radius: 10px;
  background: rgba(148, 163, 184, 0.08);
}

.import-progress__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.import-progress__summary,
.import-progress__error,
.import-progress__failure {
  margin: 0;
  font-size: 0.8rem;
}

.import-progress__summary {
  color: var(--sc-text-secondary);
}

.import-progress__error {
  color: #fca5a5;
}

.import-progress__failure {
  color: #fbbf24;
}

.import-browser__tabs {
  margin-bottom: 0.25rem;
}

.import-browser__panel {
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 10px;
  padding: 0.75rem;
  min-height: 360px;
}

.import-browser__loading,
.import-browser__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  min-height: 280px;
  text-align: center;
  color: var(--sc-text-secondary);
}

.import-browser__list {
  max-height: 280px;
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

@media (max-width: 720px) {
  .upload-panel__quota-inline {
    width: 100%;
    margin-left: 0;
  }

  .upload-panel__quota-bar {
    flex: 1 1 auto;
    max-width: none;
  }

  .upload-panel__target-create {
    grid-template-columns: 1fr;
  }
}
</style>
