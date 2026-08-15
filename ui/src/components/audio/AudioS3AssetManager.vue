<template>
  <div class="audio-s3-library">
    <section class="audio-s3-library__toolbar">
      <n-input
        v-model:value="keyword"
        clearable
        size="small"
        placeholder="按对象名称或 S3 Key 搜索"
        @keyup.enter="applySearch"
        @clear="applySearch"
      >
        <template #prefix>
          <n-icon size="16"><SearchOutline /></n-icon>
        </template>
      </n-input>
      <div class="audio-s3-library__toolbar-actions">
        <n-button size="small" quaternary :loading="s3.assetsLoading" @click="refresh">
          <template #icon><n-icon size="16"><ReloadOutline /></n-icon></template>
          刷新列表
        </n-button>
        <n-button size="small" secondary @click="openUpload">
          <template #icon><n-icon size="16"><CloudUploadOutline /></n-icon></template>
          上传素材
        </n-button>
        <n-button size="small" type="primary" @click="openCreateFolder">
          <template #icon><n-icon size="16"><FolderOpenOutline /></n-icon></template>
          新建文件夹
        </n-button>
        <n-button v-if="s3.settings.canConfigure" size="small" secondary @click="s3ModeVisible = true">S3模式</n-button>
      </div>
    </section>

    <section v-if="checkedRowKeys.length" class="audio-s3-library__selection">
      <span>已选 {{ checkedRowKeys.length }} 项</span>
      <n-space size="small">
        <n-button size="small" @click="openBatchMove">批量移动</n-button>
        <n-button size="small" type="error" @click="confirmBatchDelete">批量删除</n-button>
        <n-button size="small" quaternary @click="checkedRowKeys = []">清空</n-button>
      </n-space>
    </section>

    <section class="audio-s3-library__content">
      <aside class="audio-s3-library__folders">
        <header class="audio-s3-library__panel-header">
          <strong>文件夹</strong>
          <n-space size="small">
            <n-button size="tiny" quaternary :disabled="!currentFolder" @click="openRenameFolder">重命名</n-button>
            <n-button size="tiny" quaternary type="error" :disabled="!currentFolder" @click="confirmDeleteFolder">删除</n-button>
          </n-space>
        </header>
        <n-tree
          block-line
          selectable
          default-expand-all
          :data="folderTreeData"
          :selected-keys="folderKeys"
          @update:selected-keys="handleFolderSelect"
        />
      </aside>

      <section class="audio-s3-library__table">
        <div class="audio-s3-library__table-top">
          <span><strong>{{ s3.total }}</strong> 条 S3 音频对象</span>
          <span class="audio-s3-library__path">{{ currentPathLabel }}</span>
        </div>
        <n-data-table
          size="small"
          :columns="columns"
          :data="s3.assets"
          :loading="s3.assetsLoading"
          :row-key="rowKey"
          :checked-row-keys="checkedRowKeys"
          :row-class-name="rowClassName"
          :row-props="rowProps"
          :max-height="'calc(100dvh - 350px)'"
          @update:checked-row-keys="handleCheckedRows"
          bordered
        />
        <div class="audio-s3-library__pagination">
          <n-pagination
            size="small"
            :page="s3.page"
            :page-size="s3.pageSize"
            :item-count="s3.total"
            :page-sizes="[10, 20, 50, 100]"
            show-size-picker
            @update:page="s3.setPage"
            @update:page-size="s3.setPageSize"
          />
        </div>
      </section>

      <aside class="audio-s3-library__detail">
        <template v-if="selectedAsset">
          <header class="audio-s3-library__detail-header">
            <div>
              <h3>{{ selectedAsset.name }}</h3>
              <p>{{ selectedAsset.objectKey }}</p>
            </div>
            <n-tag size="small" type="info">S3</n-tag>
          </header>
          <n-descriptions :column="1" size="small" bordered>
            <n-descriptions-item label="大小">{{ formatFileSize(selectedAsset.size) }}</n-descriptions-item>
            <n-descriptions-item label="更新时间">{{ formatDate(selectedAsset.updatedAt) }}</n-descriptions-item>
            <n-descriptions-item label="Content-Type">{{ selectedAsset.contentType || '未知' }}</n-descriptions-item>
            <n-descriptions-item label="ETag">{{ selectedAsset.etag || '未知' }}</n-descriptions-item>
          </n-descriptions>
          <div class="audio-s3-library__detail-actions">
            <n-button size="small" @click="copyStream(selectedAsset.id)">复制播放链接</n-button>
            <n-button size="small" secondary @click="openEditAsset(selectedAsset)">重命名 / 移动</n-button>
            <n-button size="small" type="error" ghost @click="confirmDeleteAsset(selectedAsset)">删除</n-button>
          </div>
        </template>
        <n-empty v-else description="请选择一条 S3 音频对象" />
      </aside>
    </section>

    <input
      ref="fileInput"
      class="audio-s3-library__file-input"
      type="file"
      multiple
      accept="audio/*,.mp3,.ogg,.opus,.wav,.flac,.m4a,.aac,.webm,.mp4"
      @change="handleFilesSelected"
    />

    <n-modal v-model:show="folderModalVisible" preset="dialog" :title="folderModalTitle" :mask-closable="false">
      <n-form label-placement="top">
        <n-form-item label="名称">
          <n-input v-model:value="folderForm.name" maxlength="120" show-count />
        </n-form-item>
        <n-form-item label="上级目录">
          <n-tree-select
            v-model:value="folderForm.parentId"
            :options="folderSelectOptions"
            clearable
            default-expand-all
            placeholder="S3 模式根路径"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="folderModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveFolder">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="assetModalVisible" preset="dialog" title="重命名 / 移动 S3 素材" :mask-closable="false">
      <n-form label-placement="top">
        <n-form-item label="对象名称">
          <n-input v-model:value="assetForm.name" maxlength="180" show-count />
        </n-form-item>
        <n-form-item label="目标目录">
          <n-tree-select
            v-model:value="assetForm.folderId"
            :options="folderSelectOptions"
            clearable
            default-expand-all
            placeholder="S3 模式根路径"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="assetModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveAsset">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="batchMoveVisible" preset="dialog" title="批量移动 S3 素材" :mask-closable="false">
      <n-tree-select
        v-model:value="batchMoveFolderId"
        :options="folderSelectOptions"
        clearable
        default-expand-all
        placeholder="S3 模式根路径"
      />
      <template #action>
        <n-space justify="end">
          <n-button @click="batchMoveVisible = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveBatchMove">确认移动</n-button>
        </n-space>
      </template>
    </n-modal>

    <AudioS3ModeDialog v-model:show="s3ModeVisible" @saved="handleModeSaved" />
  </div>
</template>

<script setup lang="ts">
import {
  CloudUploadOutline,
  CopyOutline,
  CreateOutline,
  FolderOpenOutline,
  ReloadOutline,
  SearchOutline,
  TrashOutline,
} from '@vicons/ionicons5';
import { computed, h, onMounted, reactive, ref } from 'vue';
import {
  NButton,
  useDialog,
  useMessage,
  type DataTableColumns,
  type TreeOption,
} from 'naive-ui';
import { copyTextWithResult } from '@/utils/clipboard';
import { useAudioStudioStore } from '@/stores/audioStudio';
import {
  useAudioS3LibraryStore,
  type AudioS3Asset,
  type AudioS3Folder,
} from '@/stores/audioS3Library';
import AudioS3ModeDialog from './AudioS3ModeDialog.vue';

const s3 = useAudioS3LibraryStore();
const audio = useAudioStudioStore();
const message = useMessage();
const dialog = useDialog();
const keyword = ref('');
const checkedRowKeys = ref<string[]>([]);
const folderKeys = ref<string[]>(['all']);
const fileInput = ref<HTMLInputElement | null>(null);
const saving = ref(false);
const s3ModeVisible = ref(false);
const folderModalVisible = ref(false);
const folderModalMode = ref<'create' | 'rename'>('create');
const assetModalVisible = ref(false);
const batchMoveVisible = ref(false);
const batchMoveFolderId = ref<string | null>(null);

const folderForm = reactive({
  id: '',
  name: '',
  parentId: null as string | null,
});
const assetForm = reactive({
  id: '',
  name: '',
  folderId: null as string | null,
});

const selectedAsset = computed(() => s3.selectedAsset as AudioS3Asset | null);
const folderMap = computed(() => {
  const result = new Map<string, AudioS3Folder>();
  const walk = (items: AudioS3Folder[]) => {
    for (const folder of items) {
      result.set(folder.id, folder);
      if (folder.children?.length) walk(folder.children as AudioS3Folder[]);
    }
  };
  walk(s3.folders);
  return result;
});
const currentFolder = computed(() => {
  const key = folderKeys.value[0];
  if (!key || key === 'all') return null;
  return folderMap.value.get(key) || null;
});
const currentPathLabel = computed(() => currentFolder.value?.path || '/');
const folderModalTitle = computed(() => folderModalMode.value === 'create' ? '新建 S3 文件夹' : '重命名 S3 文件夹');

const folderTreeData = computed<TreeOption[]>(() => {
  const build = (items: AudioS3Folder[]): TreeOption[] => items.map((folder) => ({
    key: folder.id,
    label: folder.name,
    children: folder.children?.length ? build(folder.children as AudioS3Folder[]) : undefined,
  }));
  return [{ key: 'all', label: '全部素材', children: build(s3.folders) }];
});

const folderSelectOptions = computed<TreeOption[]>(() => {
  const build = (items: AudioS3Folder[]): TreeOption[] => items.map((folder) => ({
    key: folder.id,
    value: folder.id,
    label: folder.name,
    children: folder.children?.length ? build(folder.children as AudioS3Folder[]) : undefined,
  }));
  return build(s3.folders);
});

function sortHeader(label: string, field: 'name' | 'updatedAt' | 'size') {
  const active = s3.sortBy === field;
  const glyph = active ? (s3.sortOrder === 'desc' ? '↓' : '↑') : '↕';
  return h('button', {
    type: 'button',
    class: ['audio-s3-library__sort', active && 'is-active'],
    onClick: (event: MouseEvent) => {
      event.stopPropagation();
      void s3.setSort(field);
    },
  }, [h('span', label), h('span', glyph)]);
}

const columns = computed<DataTableColumns<AudioS3Asset>>(() => [
  { type: 'selection', multiple: true, fixed: 'left' },
  {
    title: () => sortHeader('名称', 'name'),
    key: 'name',
    minWidth: 300,
    render: (row) => h('div', { class: 'audio-s3-library__name-cell' }, [
      h('strong', row.name),
      h('div', { class: 'audio-s3-library__inline-actions' }, [
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          onClick: (event: MouseEvent) => {
            event.stopPropagation();
            copyStream(row.id);
          },
        }, { icon: () => h(CopyOutline) }),
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          onClick: (event: MouseEvent) => {
            event.stopPropagation();
            openEditAsset(row);
          },
        }, { icon: () => h(CreateOutline) }),
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          type: 'error',
          onClick: (event: MouseEvent) => {
            event.stopPropagation();
            confirmDeleteAsset(row);
          },
        }, { icon: () => h(TrashOutline) }),
      ]),
    ]),
  },
  {
    title: () => sortHeader('大小', 'size'),
    key: 'size',
    width: 110,
    render: (row) => formatFileSize(row.size),
  },
  {
    title: () => sortHeader('更新时间', 'updatedAt'),
    key: 'updatedAt',
    width: 170,
    render: (row) => formatDate(row.updatedAt),
  },
]);

const rowKey = (row: AudioS3Asset) => row.id;
const rowClassName = (row: AudioS3Asset) => row.id === s3.selectedAssetId ? 'is-selected-row' : '';
const rowProps = (row: AudioS3Asset) => ({
  onClick: () => s3.setSelectedAsset(row.id),
});

onMounted(async () => {
  await s3.ensureSettings();
  if (s3.enabled) await refresh();
});

function handleCheckedRows(keys: Array<string | number>) {
  checkedRowKeys.value = keys.map((key) => String(key));
}

async function handleFolderSelect(keys: Array<string | number>) {
  const key = keys.length ? String(keys[0]) : 'all';
  folderKeys.value = [key];
  checkedRowKeys.value = [];
  await s3.setFolder(key === 'all' ? null : key);
}

async function refresh() {
  try {
    await s3.refresh();
    message.success('S3 素材列表已刷新');
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '刷新 S3 素材失败');
  }
}

async function applySearch() {
  try {
    await s3.setQuery(keyword.value);
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '搜索失败');
  }
}

function openUpload() {
  fileInput.value?.click();
}

async function handleFilesSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  if (!input.files?.length) return;
  saving.value = true;
  try {
    const result = await s3.uploadFiles(input.files, currentFolder.value?.id || null);
    message.success(`已上传 ${result.length} 个 S3 音频对象`);
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '上传失败');
  } finally {
    saving.value = false;
    input.value = '';
  }
}

function openCreateFolder() {
  folderModalMode.value = 'create';
  folderForm.id = '';
  folderForm.name = '';
  folderForm.parentId = currentFolder.value?.id || null;
  folderModalVisible.value = true;
}

function openRenameFolder() {
  if (!currentFolder.value) return;
  folderModalMode.value = 'rename';
  folderForm.id = currentFolder.value.id;
  folderForm.name = currentFolder.value.name;
  folderForm.parentId = currentFolder.value.parentId || null;
  folderModalVisible.value = true;
}

async function saveFolder() {
  if (!folderForm.name.trim()) {
    message.warning('请输入文件夹名称');
    return;
  }
  saving.value = true;
  try {
    if (folderModalMode.value === 'create') {
      await s3.createFolder({ name: folderForm.name.trim(), parentId: folderForm.parentId });
      message.success('S3 文件夹已创建');
    } else {
      const updated = await s3.updateFolder(folderForm.id, {
        name: folderForm.name.trim(),
        parentId: folderForm.parentId,
      });
      if (updated?.id) {
        folderKeys.value = [updated.id];
        await s3.setFolder(updated.id);
      }
      message.success('S3 文件夹已更新');
    }
    folderModalVisible.value = false;
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存 S3 文件夹失败');
  } finally {
    saving.value = false;
  }
}

function confirmDeleteFolder() {
  if (!currentFolder.value) return;
  const folder = currentFolder.value;
  dialog.warning({
    title: '删除 S3 文件夹',
    content: `确定递归删除“${folder.name}”及其中的对象吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => deleteFolder(folder, false),
  });
}

async function deleteFolder(folder: AudioS3Folder, forceDetach: boolean) {
  try {
    await s3.deleteFolder(folder.id, forceDetach);
    folderKeys.value = ['all'];
    message.success('S3 文件夹已删除');
  } catch (error: any) {
    if (error?.response?.status === 409 && !forceDetach) {
      dialog.warning({
        title: '文件夹内素材仍被引用',
        content: '继续删除将解除场景和播放状态中的相关引用。',
        positiveText: '解除引用并删除',
        negativeText: '取消',
        onPositiveClick: () => deleteFolder(folder, true),
      });
      return;
    }
    message.error(error?.response?.data?.message || error?.message || '删除 S3 文件夹失败');
  }
}

function openEditAsset(asset: AudioS3Asset) {
  assetForm.id = asset.id;
  assetForm.name = asset.name;
  assetForm.folderId = asset.folderId || null;
  assetModalVisible.value = true;
}

async function saveAsset() {
  if (!assetForm.name.trim()) {
    message.warning('请输入对象名称');
    return;
  }
  saving.value = true;
  try {
    await s3.updateAsset(assetForm.id, {
      name: assetForm.name.trim(),
      folderId: assetForm.folderId,
    });
    message.success('S3 音频对象已更新');
    assetModalVisible.value = false;
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '更新 S3 音频对象失败');
  } finally {
    saving.value = false;
  }
}

function confirmDeleteAsset(asset: AudioS3Asset) {
  dialog.warning({
    title: '删除 S3 音频对象',
    content: `确定删除“${asset.name}”吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => deleteAsset(asset, false),
  });
}

async function deleteAsset(asset: AudioS3Asset, forceDetach: boolean) {
  try {
    await s3.deleteAsset(asset.id, forceDetach);
    message.success('S3 音频对象已删除');
  } catch (error: any) {
    if (error?.response?.status === 409 && !forceDetach) {
      dialog.warning({
        title: '素材仍被引用',
        content: '继续删除将解除场景和播放状态中的相关引用。',
        positiveText: '解除引用并删除',
        negativeText: '取消',
        onPositiveClick: () => deleteAsset(asset, true),
      });
      return;
    }
    message.error(error?.response?.data?.message || error?.message || '删除 S3 音频对象失败');
  }
}

function openBatchMove() {
  batchMoveFolderId.value = currentFolder.value?.id || null;
  batchMoveVisible.value = true;
}

async function saveBatchMove() {
  saving.value = true;
  let success = 0;
  try {
    for (const id of checkedRowKeys.value) {
      const asset = s3.assets.find((item) => item.id === id);
      if (!asset) continue;
      await s3.updateAsset(id, { name: asset.name, folderId: batchMoveFolderId.value });
      success += 1;
    }
    checkedRowKeys.value = [];
    batchMoveVisible.value = false;
    message.success(`已移动 ${success} 个 S3 音频对象`);
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '批量移动失败');
  } finally {
    saving.value = false;
  }
}

function confirmBatchDelete() {
  const ids = [...checkedRowKeys.value];
  dialog.warning({
    title: '批量删除 S3 音频对象',
    content: `确定删除已选的 ${ids.length} 个对象吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      saving.value = true;
      let success = 0;
      let failed = 0;
      try {
        for (const id of ids) {
          try {
            await s3.deleteAsset(id, false);
            success += 1;
          } catch {
            failed += 1;
          }
        }
        checkedRowKeys.value = [];
        if (success) message.success(`已删除 ${success} 个对象`);
        if (failed) message.warning(`${failed} 个对象因仍被引用或远端错误未删除`);
      } finally {
        saving.value = false;
      }
    },
  });
}

function copyStream(assetId: string) {
  void copyTextWithResult(s3.buildRawStreamUrl(assetId), {
    onSuccess: () => message.success('播放链接已复制'),
    onFailure: () => message.error('复制失败'),
  });
}

async function handleModeSaved() {
  if (s3.enabled) {
    await refresh();
    return;
  }
  await Promise.all([
    audio.fetchFolders(),
    audio.fetchAssets({ pagination: { page: 1 } }),
    audio.fetchTrackSelectableAssets(),
  ]);
}

function formatFileSize(value: number) {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function formatDate(value?: string) {
  if (!value) return '未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString();
}
</script>

<style scoped lang="scss">
.audio-s3-library {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.audio-s3-library__toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.75rem;
  align-items: center;
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
}

.audio-s3-library__toolbar > :deep(.n-input) {
  min-width: 0;
}

.audio-s3-library__toolbar > :deep(.n-alert) {
  grid-column: 1 / -1;
}

.audio-s3-library__toolbar-actions,
.audio-s3-library__detail-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.audio-s3-library__toolbar-actions {
  min-width: max-content;
  flex-wrap: nowrap;
  justify-content: flex-end;
}

.audio-s3-library__selection {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  padding: 0.5rem 0.75rem;
  background: rgba(99, 179, 237, 0.08);
}

.audio-s3-library__content {
  display: grid;
  grid-template-columns: 230px minmax(0, 1fr) 300px;
  gap: 0.75rem;
  height: clamp(420px, calc(100dvh - 250px), 720px);
  min-height: 0;
}

.audio-s3-library__folders,
.audio-s3-library__table,
.audio-s3-library__detail {
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.audio-s3-library__panel-header,
.audio-s3-library__table-top,
.audio-s3-library__detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 0.65rem;
}

.audio-s3-library__table {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.audio-s3-library__folders {
  display: flex;
  flex-direction: column;
}

.audio-s3-library__folders :deep(.n-tree) {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.audio-s3-library__path {
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.audio-s3-library__pagination {
  display: flex;
  justify-content: flex-end;
}

.audio-s3-library__detail {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  overflow-y: auto;
}

.audio-s3-library__folders :deep(.n-tree),
.audio-s3-library__detail,
.audio-s3-library__table :deep(.n-data-table-wrapper),
.audio-s3-library__table :deep(.n-data-table-base-table-body) {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.3) transparent;
}

.audio-s3-library__folders :deep(.n-tree)::-webkit-scrollbar,
.audio-s3-library__detail::-webkit-scrollbar,
.audio-s3-library__table :deep(.n-data-table-wrapper)::-webkit-scrollbar,
.audio-s3-library__table :deep(.n-data-table-base-table-body)::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.audio-s3-library__folders :deep(.n-tree)::-webkit-scrollbar-track,
.audio-s3-library__detail::-webkit-scrollbar-track,
.audio-s3-library__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-track,
.audio-s3-library__table :deep(.n-data-table-base-table-body)::-webkit-scrollbar-track {
  background: transparent;
}

.audio-s3-library__folders :deep(.n-tree)::-webkit-scrollbar-thumb,
.audio-s3-library__detail::-webkit-scrollbar-thumb,
.audio-s3-library__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb,
.audio-s3-library__table :deep(.n-data-table-base-table-body)::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.3);
  border-radius: 3px;
}

.audio-s3-library__folders :deep(.n-tree)::-webkit-scrollbar-thumb:hover,
.audio-s3-library__detail::-webkit-scrollbar-thumb:hover,
.audio-s3-library__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb:hover,
.audio-s3-library__table :deep(.n-data-table-base-table-body)::-webkit-scrollbar-thumb:hover {
  background: rgba(128, 128, 128, 0.5);
}

.audio-s3-library__detail-header h3 {
  margin: 0;
  overflow-wrap: anywhere;
}

.audio-s3-library__detail-header p {
  margin: 0.2rem 0 0;
  color: var(--sc-text-secondary);
  font-size: 0.72rem;
  overflow-wrap: anywhere;
}

.audio-s3-library__name-cell {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
}

.audio-s3-library__inline-actions {
  display: inline-flex;
  gap: 0.2rem;
  opacity: 0;
  transition: opacity 0.16s ease;
}

:deep(.n-data-table-tr:hover .audio-s3-library__inline-actions),
:deep(.is-selected-row .audio-s3-library__inline-actions) {
  opacity: 1;
}

:deep(.is-selected-row td) {
  background: rgba(99, 179, 237, 0.08);
}

.audio-s3-library__sort {
  display: inline-flex;
  gap: 0.25rem;
  border: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}

.audio-s3-library__sort.is-active {
  color: var(--sc-primary, #2563eb);
}

.audio-s3-library__file-input {
  display: none;
}

@media (max-width: 1020px) {
  .audio-s3-library__content {
    grid-template-columns: 220px minmax(0, 1fr);
  }

  .audio-s3-library__detail {
    grid-column: 1 / -1;
  }
}

@media (max-width: 700px) {
  .audio-s3-library__toolbar,
  .audio-s3-library__content {
    grid-template-columns: 1fr;
  }

  .audio-s3-library__toolbar-actions {
    min-width: 0;
    flex-wrap: wrap;
    justify-content: flex-start;
  }

  .audio-s3-library__content {
    height: auto;
  }

  .audio-s3-library__folders,
  .audio-s3-library__table,
  .audio-s3-library__detail {
    min-height: 360px;
  }

  .audio-s3-library__selection {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
