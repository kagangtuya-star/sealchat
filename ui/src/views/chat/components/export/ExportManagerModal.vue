<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import { useDebounceFn, useWindowSize } from '@vueuse/core';
import type { ExportTaskItem } from '@/types';
import { useChatStore } from '@/stores/chat';
import { triggerBlobDownload } from '@/utils/download';
import { useDisplayStore } from '@/stores/display';

interface Props {
  visible: boolean;
  channelId?: string;
}

interface Emits {
  (e: 'update:visible', visible: boolean): void;
  (e: 'request-export'): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();

const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();
const { width } = useWindowSize();
const isMobile = computed(() => width.value <= 720);
const modalWidth = computed(() => (isMobile.value ? '92vw' : '760px'));
const display = useDisplayStore();
const isNightMode = computed(() => display.settings?.palette === 'night');
const exportManagerClasses = computed(() => ({
  'export-manager--mobile': isMobile.value,
  'export-manager--night': !!isNightMode.value,
  'export-manager--light': !isNightMode.value,
}));

const loading = ref(false);
const uploadingTaskId = ref<string | null>(null);
const retryingTaskId = ref<string | null>(null);
const deletingTaskId = ref<string | null>(null);
const page = ref(1);
const pageSize = ref(5);
const statusFilter = ref<'done' | 'failed' | 'all'>('done');
const keywordInput = ref('');
const searchKeyword = ref('');

const tasks = ref<ExportTaskItem[]>([]);
const total = ref(0);
const totalSize = ref(0);

const statusOptions = [
  { label: '仅成功', value: 'done' },
  { label: '仅失败', value: 'failed' },
  { label: '全部', value: 'all' },
];

const debouncedSearch = useDebounceFn(() => {
  searchKeyword.value = keywordInput.value.trim();
  page.value = 1;
  fetchTasks();
}, 300);

watch(keywordInput, () => {
  debouncedSearch();
});

const resolvedStatusParam = computed(() => {
  if (statusFilter.value === 'all') return undefined;
  return statusFilter.value;
});

const formatFileSize = (value: number) => {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const shown = size % 1 === 0 ? size : size.toFixed(1);
  return `${shown} ${units[unitIndex]}`;
};

const totalSizeLabel = computed(() => formatFileSize(total.value > 0 ? totalSize.value : 0));

const statusLabel = (status: string) => {
  switch (status) {
    case 'done':
      return '已完成';
    case 'failed':
      return '失败';
    case 'processing':
      return '进行中';
    default:
      return status || '未知';
  }
};

const statusType = (status: string) => {
  switch (status) {
    case 'done':
      return 'success';
    case 'failed':
      return 'error';
    default:
      return 'default';
  }
};

const isDeletableStatus = (status: string) => status === 'done' || status === 'failed';

const formatTime = (timestamp?: number) => {
  if (!timestamp) {
    return '生成中';
  }
  return new Date(timestamp).toLocaleString();
};

const taskDisplayName = (item: ExportTaskItem) => {
  return item.display_name || item.file_name || `#${item.task_id.slice(0, 8)}`;
};

const taskMeta = (item: ExportTaskItem) => {
  const pieces: string[] = [];
  if (item.format) {
    pieces.push(item.format.toUpperCase());
  }
  if (item.file_size) {
    pieces.push(formatFileSize(item.file_size));
  }
  pieces.push(`#${item.task_id.slice(0, 8)}`);
  return pieces.join(' · ');
};

const fetchTasks = async () => {
  if (!props.channelId) {
    tasks.value = [];
    total.value = 0;
    totalSize.value = 0;
    return;
  }
  loading.value = true;
  try {
    const resp = await chat.listExportTasks(props.channelId, {
      page: page.value,
      size: pageSize.value,
      status: resolvedStatusParam.value,
      keyword: searchKeyword.value,
    });
    tasks.value = resp?.items ?? [];
    total.value = resp?.total ?? 0;
    totalSize.value = resp?.total_size ?? 0;
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '获取导出任务失败';
    message.error(errMsg);
  } finally {
    loading.value = false;
  }
};

const STREAMING_DOWNLOAD_THRESHOLD = 300 * 1024; // 300KB
const DOWNLOAD_EXT_MAP: Record<string, string> = {
  txt: '.txt',
  json: '.json',
  html: '.zip',
};

const resolveDownloadFileName = (item: ExportTaskItem) => {
  const rawName = (item.file_name || item.display_name || `export-${item.task_id.slice(0, 8)}`).trim();
  const formatExt = DOWNLOAD_EXT_MAP[item.format?.toLowerCase() || ''];
  if (!formatExt) {
    return rawName || item.task_id;
  }
  const lower = rawName.toLowerCase();
  if (lower.endsWith(formatExt)) {
    return rawName;
  }
  return `${rawName || item.task_id}${formatExt}`;
};

const startBrowserDownload = (item: ExportTaskItem) => {
  const defaultUrl = new URL(`/api/v1/chat/export/${item.task_id}?download=1`, window.location.origin).toString();
  const url = item.download_url?.trim() || defaultUrl;
  const fileName = resolveDownloadFileName(item);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  link.rel = 'noopener';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
};

const handleDownload = async (item: ExportTaskItem) => {
  if (item.file_missing) {
    message.warning('文件已被清理，请先重新生成。');
    return;
  }
  const fileSize = Number(item.file_size || 0);
  if (fileSize > STREAMING_DOWNLOAD_THRESHOLD) {
    startBrowserDownload(item);
    return;
  }
  try {
    const { blob, fileName } = await chat.downloadExportResult(item.task_id, item.display_name || item.file_name);
    triggerBlobDownload(blob, fileName);
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '下载失败';
    message.error(errMsg);
  }
};

const handleUploadToCloud = async (item: ExportTaskItem) => {
  uploadingTaskId.value = item.task_id;
  try {
    const resp = await chat.uploadExportTask(item.task_id);
    if (resp?.url) {
      message.success('已上传到云端');
      await fetchTasks();
    } else {
      message.warning('云端上传返回异常，未获得链接');
    }
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '上传失败';
    message.error(errMsg);
  } finally {
    uploadingTaskId.value = null;
  }
};

const handleRetry = async (item: ExportTaskItem) => {
  retryingTaskId.value = item.task_id;
  try {
    const resp = await chat.retryExportTask(item.task_id);
    message.success(`已重新创建任务（#${resp.task_id.slice(0, 8)}），稍后可在列表中查看。`);
    fetchTasks();
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '重新生成失败';
    message.error(errMsg);
  } finally {
    retryingTaskId.value = null;
  }
};

const handleDelete = async (item: ExportTaskItem) => {
  if (!isDeletableStatus(item.status)) {
    message.warning('任务进行中，无法删除。');
    return;
  }
  deletingTaskId.value = item.task_id;
  try {
    await chat.deleteExportTask(item.task_id);
    message.success('导出记录已删除');
    fetchTasks();
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '删除失败';
    message.error(errMsg);
  } finally {
    deletingTaskId.value = null;
  }
};

const confirmDelete = (item: ExportTaskItem) => {
  if (!isDeletableStatus(item.status)) {
    message.warning('任务进行中，无法删除。');
    return;
  }
  dialog.warning({
    title: '删除导出记录',
    content: `确认删除「${taskDisplayName(item)}」？本地文件也会被移除。`,
    positiveText: '删除',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: () => handleDelete(item),
  });
};

const handleRefresh = () => {
  fetchTasks();
};

const handleStatusChange = (value: 'done' | 'failed' | 'all') => {
  statusFilter.value = value;
  page.value = 1;
  fetchTasks();
};

const handleManualSearch = () => {
  searchKeyword.value = keywordInput.value.trim();
  page.value = 1;
  fetchTasks();
};

const handlePageChange = (value: number) => {
  page.value = value;
  fetchTasks();
};

const handlePageSizeChange = (value: number) => {
  pageSize.value = value;
  page.value = 1;
  fetchTasks();
};

const handleClose = () => {
  emit('update:visible', false);
};

const handleCreateExport = () => {
  emit('request-export');
};

const resetState = () => {
  page.value = 1;
  pageSize.value = 5;
  statusFilter.value = 'done';
  keywordInput.value = '';
  searchKeyword.value = '';
};

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      resetState();
      fetchTasks();
    }
  },
);

watch(
  () => props.channelId,
  () => {
    if (props.visible) {
      page.value = 1;
      fetchTasks();
    } else {
      tasks.value = [];
      total.value = 0;
      totalSize.value = 0;
    }
  },
);
</script>

<template>
  <n-modal
    :show="props.visible"
    preset="card"
    title="导出管理"
    :mask-closable="false"
    :style="{ width: modalWidth }"
    class="export-manager-modal"
    display-directive="if"
    @update:show="emit('update:visible', $event)"
  >
    <div class="export-manager" :class="exportManagerClasses">
      <div class="export-manager__summary">
        <div class="summary-item">
          <div class="summary-label">任务数量</div>
          <div class="summary-value">{{ total }}</div>
        </div>
        <div class="summary-item">
          <div class="summary-label">累计体积</div>
          <div class="summary-value">{{ totalSizeLabel }}</div>
        </div>
      </div>

      <div class="export-manager__filters">
        <div class="filters-row filters-row--primary">
          <n-input
            v-model:value="keywordInput"
            size="small"
            clearable
            class="keyword-input"
            placeholder="搜索文件名或自定义名称"
            @keyup.enter="handleManualSearch"
          />
        </div>
        <div class="filters-row filters-row--secondary" :class="{ 'filters-row--stack': isMobile }">
          <n-select
            size="small"
            class="status-select"
            :value="statusFilter"
            :options="statusOptions"
            @update:value="handleStatusChange"
          />
          <n-space class="filter-actions" align="center" justify="end">
            <n-button size="small" tertiary @click="handleRefresh" :loading="loading">刷新</n-button>
            <n-button size="small" type="primary" :disabled="!props.channelId" @click="handleCreateExport">
              新建导出
            </n-button>
          </n-space>
        </div>
      </div>

      <n-spin :show="loading">
        <n-empty v-if="!tasks.length && !loading" description="暂无导出任务" />
        <div v-else class="export-manager__list">
          <div v-for="item in tasks" :key="item.task_id" class="export-entry">
            <div class="export-entry__header">
              <div class="export-entry__title">
                <div class="export-entry__name">{{ taskDisplayName(item) }}</div>
                <div class="export-entry__meta">{{ taskMeta(item) }}</div>
              </div>
              <n-tag size="small" :type="statusType(item.status)">
                {{ statusLabel(item.status) }}
              </n-tag>
            </div>
            <div class="export-entry__footer">
              <div class="export-entry__time">{{ formatTime(item.finished_at || item.requested_at) }}</div>
              <div class="export-entry__actions" :class="{ 'export-entry__actions--stack': isMobile }">
                <n-button
                  text
                  size="small"
                  :disabled="item.status !== 'done' || item.file_missing"
                  @click="handleDownload(item)"
                >
                  查看
                </n-button>
                <n-button
                  text
                  size="small"
                  type="error"
                  :disabled="!isDeletableStatus(item.status)"
                  :loading="deletingTaskId === item.task_id"
                  @click="confirmDelete(item)"
                >
                  删除
                </n-button>
                <n-button
                  v-if="item.format === 'json' && item.upload_url"
                  text
                  size="small"
                  tag="a"
                  :href="item.upload_url"
                  target="_blank"
                >
                  云端
                </n-button>
                <n-button
                  v-else-if="item.format === 'json'"
                  text
                  size="small"
                  :loading="uploadingTaskId === item.task_id"
                  @click="handleUploadToCloud(item)"
                  :disabled="item.status !== 'done' || item.file_missing"
                >
                  上传
                </n-button>
                <n-button
                  v-if="item.file_missing || item.status === 'failed'"
                  text
                  size="small"
                  type="warning"
                  :loading="retryingTaskId === item.task_id"
                  @click="handleRetry(item)"
                >
                  重新生成
                </n-button>
              </div>
            </div>
            <div v-if="item.file_missing" class="export-entry__warning">
              文件已被清理，可点击“重新生成”使用原参数再次导出。
            </div>
          </div>
        </div>
      </n-spin>

      <div v-if="total > pageSize" class="export-manager__pagination">
        <n-pagination
          :page="page"
          :page-size="pageSize"
          :item-count="total"
          show-size-picker
          :page-sizes="[5, 10, 20]"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </div>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
.export-manager-modal :deep(.n-card__content) {
  padding-top: 8px;
}

.export-manager {
  --export-entry-bg: var(--n-card-color, var(--n-color));
  --export-entry-border: var(--n-border-color);
  --export-entry-text: var(--n-text-color);
  --export-entry-muted: var(--n-text-color-3);
  --export-entry-warning-bg: var(--n-color-hover);

  display: flex;
  flex-direction: column;
  gap: 16px;

  &--light {
    --export-entry-bg: #ffffff;
    --export-entry-border: rgba(15, 23, 42, 0.12);
    --export-entry-text: #111111;
    --export-entry-muted: rgba(17, 17, 17, 0.65);
    --export-entry-warning-bg: #f3f3f3;
  }

  &--night {
    --export-entry-bg: #1f1f1f;
    --export-entry-border: rgba(255, 255, 255, 0.2);
    --export-entry-text: #ffffff;
    --export-entry-muted: rgba(255, 255, 255, 0.65);
    --export-entry-warning-bg: #262626;
  }
}

.export-manager__summary {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;

  .summary-item {
    flex: 1;
    min-width: 140px;
    border: 1px solid var(--n-border-color);
    border-radius: 10px;
    padding: 12px;
    background: var(--n-color);
  }

  .summary-label {
    font-size: 12px;
    color: var(--n-text-color-3);
    margin-bottom: 4px;
  }

  .summary-value {
    font-size: 20px;
    font-weight: 600;
  }
}

.export-manager__filters {
  display: flex;
  flex-direction: column;
  gap: 10px;

  .filters-row {
    display: flex;
    gap: 12px;
    align-items: center;
  }

  .keyword-input {
    width: 100%;
  }

  .filters-row--primary .keyword-input {
    flex: 1;
  }

  .filters-row--secondary {
    justify-content: space-between;
    flex-wrap: wrap;
  }

  .filters-row--stack {
    flex-direction: column;
    align-items: flex-start;

    .status-select,
    .filter-actions {
      width: 100%;
    }

    .filter-actions {
      justify-content: space-between;
    }
  }

  .status-select {
    width: 150px;
  }

  .filter-actions {
    min-width: 200px;
    justify-content: flex-end;
    flex-shrink: 0;
  }
}

.export-manager__list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.export-entry {
  border: 1px solid var(--export-entry-border);
  border-radius: 12px;
  padding: 12px 16px;
  background: var(--export-entry-bg);
  color: var(--export-entry-text);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.export-entry__header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.export-entry__title {
  min-width: 0;
}

.export-entry__name {
  font-weight: 600;
  font-size: 15px;
  color: var(--export-entry-text);
  word-break: break-word;
}

.export-entry__meta {
  font-size: 12px;
  color: var(--export-entry-muted);
  margin-top: 2px;
}

.export-entry__footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.export-entry__time {
  font-size: 12px;
  color: var(--export-entry-muted);
}

.export-entry__actions {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
}

.export-entry__actions--stack {
  width: 100%;
  justify-content: flex-start;
}

.export-entry__warning {
  font-size: 12px;
  color: var(--export-entry-muted);
  background: var(--export-entry-warning-bg);
  border-radius: 8px;
  padding: 6px 10px;
  margin-top: -4px;
}

.export-manager__pagination {
  display: flex;
  justify-content: flex-end;
}

.export-manager--mobile {
  .export-manager__summary {
    flex-direction: column;
  }

  .export-manager__filters {
    grid-template-columns: 1fr;

    .status-select,
    .filter-actions {
      width: 100%;
    }

    .filter-actions {
      justify-content: space-between;
    }
  }

  .export-entry__footer {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
