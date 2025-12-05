<template>
  <div class="audio-library">
    <section class="audio-library__toolbar">
      <div class="audio-library__filters">
        <n-input
          v-model:value="keyword"
          size="small"
          clearable
          placeholder="搜索名称 / 标签 / 描述"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <n-icon size="16">
              <SearchOutline />
            </n-icon>
          </template>
        </n-input>

        <n-select
          v-model:value="selectedTags"
          multiple
          filterable
          tag
          size="small"
          placeholder="标签筛选"
          :options="tagOptions"
          class="audio-library__filter-item"
        />

        <n-select
          v-model:value="selectedCreators"
          multiple
          filterable
          size="small"
          placeholder="上传者"
          :options="creatorOptions"
          class="audio-library__filter-item"
        />

        <div class="audio-library__duration">
          <label>时长 (秒)</label>
          <n-slider v-model:value="durationRange" :max="durationMax" :min="0" range size="small" />
        </div>

        <div class="audio-library__filter-actions">
          <n-button size="small" @click="handleResetFilters">重置</n-button>
          <n-button type="primary" size="small" @click="handleSearch">应用</n-button>
        </div>
      </div>

      <div class="audio-library__toolbar-actions">
        <n-button size="small" quaternary @click="handleRefresh" :loading="audio.assetsLoading">
          <template #icon>
            <n-icon size="16">
              <ReloadOutline />
            </n-icon>
          </template>
          刷新列表
        </n-button>
        <n-button size="small" secondary @click="scrollToUpload" v-if="audio.canManage">
          <template #icon>
            <n-icon size="16">
              <CloudUploadOutline />
            </n-icon>
          </template>
          上传素材
        </n-button>
        <n-button size="small" type="primary" @click="openCreateFolder" v-if="audio.canManage">
          <template #icon>
            <n-icon size="16">
              <FolderOpenOutline />
            </n-icon>
          </template>
          新建文件夹
        </n-button>
      </div>

      <n-alert v-if="audio.networkMode !== 'normal'" class="audio-library__alert" type="warning" closable>
        当前处于弱网模式，素材加载将优先使用本地缓存，建议手动刷新确认最新数据。
      </n-alert>
    </section>

    <section v-if="hasSelection" class="audio-library__selection">
      <div>
        已选 {{ selectionCount }} 项
        <n-button text size="tiny" @click="clearSelection">清空</n-button>
      </div>
      <n-space size="small">
        <n-button size="small" @click="openBatchMoveModal" :loading="audio.assetBulkLoading" secondary>
          批量移动
        </n-button>
        <n-button size="small" @click="openBatchVisibilityModal" :loading="audio.assetBulkLoading">
          批量修改可见性
        </n-button>
        <n-button
          size="small"
          type="error"
          @click="confirmBatchDelete"
          :loading="audio.assetBulkLoading"
        >
          批量删除
        </n-button>
      </n-space>
    </section>

    <section class="audio-library__content">
      <aside class="audio-library__folders">
        <div class="audio-library__folder-header">
          <span>文件夹</span>
          <div class="audio-library__folder-actions" v-if="audio.canManage">
            <n-button quaternary size="tiny" @click="openCreateFolder">新建</n-button>
            <n-button quaternary size="tiny" :disabled="!currentFolder" @click="openRenameFolder">重命名</n-button>
            <n-button
              quaternary
              size="tiny"
              :disabled="!currentFolder"
              type="error"
              @click="confirmDeleteFolder"
            >
              删除
            </n-button>
          </div>
        </div>
        <n-tree
          block-line
          :data="folderTreeData"
          selectable
          :selected-keys="folderKeys"
          :node-props="treeNodeProps"
          @update:selected-keys="handleFolderSelect"
        />
      </aside>

      <section class="audio-library__table">
        <n-data-table
          size="small"
          :columns="columns"
          :data="tableData"
          :loading="audio.assetsLoading"
          :row-key="rowKey"
          :row-class-name="rowClassName"
          :row-props="rowProps"
          :checked-row-keys="checkedRowKeys"
          @update:checked-row-keys="handleCheckedRowKeysChange"
          bordered
        />
        <div class="audio-library__pagination">
          <n-pagination
            size="small"
            :page="audio.assetPagination.page"
            :page-size="audio.assetPagination.pageSize"
            :item-count="audio.assetPagination.total"
            :page-sizes="[10, 20, 30, 50]"
            show-size-picker
            @update:page="audio.setAssetPage"
            @update:page-size="audio.setAssetPageSize"
          />
        </div>
      </section>

      <section class="audio-library__detail">
        <template v-if="selectedAsset">
          <header class="audio-library__detail-header">
            <div>
              <h3>{{ selectedAsset.name }}</h3>
              <p class="audio-library__detail-subtitle">{{ folderLabel(selectedAsset.folderId) }}</p>
            </div>
            <n-tag size="small" :type="selectedAsset.visibility === 'public' ? 'success' : 'warning'">
              {{ selectedAsset.visibility === 'public' ? '公开' : '受限' }}
            </n-tag>
          </header>
          <ul class="audio-library__detail-list">
            <li>时长：{{ formatDuration(selectedAsset.duration) }}</li>
            <li>上传者：{{ selectedAsset.createdBy }}</li>
            <li>更新时间：{{ formatDate(selectedAsset.updatedAt) }}</li>
            <li>存储：{{ selectedAsset.storageType === 's3' ? '对象存储 (支持跳转)' : '本地文件' }}</li>
            <li>比特率：{{ selectedAsset.bitrate }} kbps · 大小：{{ formatFileSize(selectedAsset.size) }}</li>
          </ul>
          <div class="audio-library__tags">
            <strong>标签：</strong>
            <template v-if="selectedAsset.tags.length">
              <n-tag v-for="tag in selectedAsset.tags" :key="tag" size="small" class="audio-library__tag" bordered>
                {{ tag }}
              </n-tag>
            </template>
            <span v-else>未设置</span>
          </div>
          <div class="audio-library__description">
            <strong>备注：</strong>
            <p>{{ selectedAsset.description || '暂无备注' }}</p>
          </div>
          <div class="audio-library__detail-actions">
            <n-button quaternary size="small" @click="copyStream(selectedAsset.id)">复制播放链接</n-button>
            <n-button v-if="audio.canManage" secondary size="small" @click="openAssetEditor(selectedAsset)">
              编辑元数据
            </n-button>
          </div>
        </template>
        <n-empty description="选择一条素材以查看详情" v-else />
      </section>
    </section>

    <div ref="uploadAnchor" class="audio-library__upload">
      <UploadPanel />
    </div>

    <n-drawer
      :show="assetDrawerVisible"
      placement="right"
      :width="assetDrawerWidth"
      :mask-closable="false"
      @update:show="assetDrawerVisible = $event"
    >
      <n-drawer-content>
        <template #header>
          <div class="audio-asset-drawer__header">
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="assetDrawerVisible = false">
              返回
            </n-button>
            <span>编辑素材</span>
          </div>
        </template>
        <n-form ref="assetFormRef" :model="assetForm" :rules="assetFormRules" label-placement="top">
          <n-form-item label="名称" path="name">
            <n-input v-model:value="assetForm.name" maxlength="60" show-count />
          </n-form-item>
          <n-form-item label="备注" path="description">
            <n-input v-model:value="assetForm.description" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" />
          </n-form-item>
          <n-form-item label="标签" path="tags">
            <n-select
              v-model:value="assetForm.tags"
              multiple
              filterable
              tag
              placeholder="输入或选择标签"
              :options="tagOptions"
            />
          </n-form-item>
          <n-form-item label="所属文件夹" path="folderId">
            <n-tree-select
              v-model:value="assetForm.folderId"
              :options="folderSelectOptions"
              clearable
              placeholder="未分类"
            />
          </n-form-item>
          <n-form-item label="可见性" path="visibility">
            <n-radio-group v-model:value="assetForm.visibility">
              <n-radio value="public">公开</n-radio>
              <n-radio value="restricted">受限</n-radio>
            </n-radio-group>
          </n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="assetDrawerVisible = false">取消</n-button>
            <n-button type="primary" :loading="audio.assetMutationLoading" @click="handleSaveAsset">
              保存
            </n-button>
          </n-space>
        </template>
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="folderModalVisible" preset="dialog" :title="folderModalTitle" :mask-closable="false">
      <n-form ref="folderFormRef" :model="folderForm" :rules="folderFormRules" label-placement="top">
        <n-form-item label="名称" path="name">
          <n-input v-model:value="folderForm.name" maxlength="40" show-count />
        </n-form-item>
        <n-form-item label="上级" path="parentId">
          <n-tree-select
            v-model:value="folderForm.parentId"
            :options="folderSelectOptions"
            default-expand-all
            clearable
            placeholder="根目录"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="folderModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.folderActionLoading" @click="handleSaveFolder">
            保存
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="batchMoveModalVisible" preset="dialog" title="批量移动素材" :mask-closable="false">
      <p class="audio-library__modal-tip">选择目标文件夹，未选择则移动到未分类</p>
      <n-tree-select
        v-model:value="batchMoveTarget"
        :options="folderSelectOptions"
        clearable
        placeholder="未分类"
      />
      <template #action>
        <n-space justify="end">
          <n-button @click="batchMoveModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.assetBulkLoading" @click="handleBatchMoveSave">
            确认
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="batchVisibilityModalVisible" preset="dialog" title="批量修改可见性" :mask-closable="false">
      <p class="audio-library__modal-tip">将已选素材设置为统一的可见性状态</p>
      <n-radio-group v-model:value="batchVisibilityValue">
        <n-radio value="public">公开</n-radio>
        <n-radio value="restricted">受限</n-radio>
      </n-radio-group>
      <template #action>
        <n-space justify="end">
          <n-button @click="batchVisibilityModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.assetBulkLoading" @click="handleBatchVisibilitySave">
            确认
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { SearchOutline, CloudUploadOutline, FolderOpenOutline, ReloadOutline } from '@vicons/ionicons5';
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import {
  NButton,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormRules,
  type TreeOption,
} from 'naive-ui';
import { useWindowSize } from '@vueuse/core';
import type { AudioAsset, AudioFolder } from '@/types/audio';
import { useAudioStudioStore } from '@/stores/audioStudio';
import UploadPanel from './UploadPanel.vue';

const audio = useAudioStudioStore();
const message = useMessage();
const dialog = useDialog();

const durationMax = 600;
const keyword = ref(audio.filters.query ?? '');
const selectedTags = ref<string[]>([...audio.filters.tags]);
const selectedCreators = ref<string[]>([...audio.filters.creatorIds]);
const durationRange = ref<[number, number]>(audio.filters.durationRange ?? [0, durationMax]);
const folderKeys = ref<string[]>(audio.filters.folderId ? [audio.filters.folderId] : ['all']);
const uploadAnchor = ref<HTMLElement | null>(null);
const folderModalVisible = ref(false);
const folderModalMode = ref<'create' | 'rename'>('create');
const folderModalTitle = computed(() => (folderModalMode.value === 'create' ? '新建文件夹' : '重命名文件夹'));
const checkedRowKeys = ref<string[]>([]);
const batchMoveModalVisible = ref(false);
const batchMoveTarget = ref<string | null>(null);
const batchVisibilityModalVisible = ref(false);
const batchVisibilityValue = ref<'public' | 'restricted'>('public');
const folderFormRef = ref<FormInst | null>(null);
const folderForm = reactive({
  id: '',
  name: '',
  parentId: null as string | null,
});
const folderFormRules: FormRules = {
  name: [
    {
      required: true,
      message: '请输入文件夹名称',
      trigger: 'blur',
    },
  ],
};

const assetDrawerVisible = ref(false);
const assetFormRef = ref<FormInst | null>(null);
const assetForm = reactive({
  id: '',
  name: '',
  description: '',
  tags: [] as string[],
  folderId: null as string | null,
  visibility: 'public' as 'public' | 'restricted',
});
const assetFormRules: FormRules = {
  name: [{ required: true, message: '名称不能为空', trigger: 'blur' }],
};

const { width: viewportWidth } = useWindowSize();
const isMobileLayout = computed(() => viewportWidth.value > 0 && viewportWidth.value < 640);
const assetDrawerWidth = computed(() => (isMobileLayout.value ? '100%' : 360));

const tableData = computed(() => audio.filteredAssets);
const selectedAsset = computed(() => audio.selectedAsset);
const selectionCount = computed(() => checkedRowKeys.value.length);
const hasSelection = computed(() => selectionCount.value > 0);
const checkedAssets = computed(() => {
  const map = new Map(tableData.value.map((item) => [item.id, item] as const));
  return checkedRowKeys.value
    .map((id) => map.get(id))
    .filter((item): item is AudioAsset => Boolean(item));
});

const folderMap = computed(() => {
  const map = new Map<string, AudioFolder>();
  const traverse = (items: AudioFolder[]) => {
    items.forEach((folder) => {
      map.set(folder.id, folder);
      if (folder.children?.length) {
        traverse(folder.children);
      }
    });
  };
  traverse(audio.folders || []);
  return map;
});

const currentFolder = computed(() => {
  const key = folderKeys.value[0];
  if (!key || key === 'all') return null;
  return folderMap.value.get(key) ?? null;
});

const folderTreeData = computed<TreeOption[]>(() => {
  const build = (folders: AudioFolder[]): TreeOption[] =>
    folders.map((folder) => ({
      key: folder.id,
      label: folder.name,
      children: folder.children ? build(folder.children) : undefined,
    }));
  return [
    {
      key: 'all',
      label: '全部素材',
      children: build(audio.folders || []),
    },
  ];
});

const folderSelectOptions = computed(() => {
  const build = (folders: AudioFolder[]): TreeOption[] =>
    folders.map((folder) => ({
      key: folder.id,
      label: folder.name,
      value: folder.id,
      children: folder.children ? build(folder.children) : undefined,
    }));
  return build(audio.folders || []);
});

const tagOptions = computed(() => {
  const tags = new Set<string>();
  audio.assets.forEach((asset) => {
    asset.tags.forEach((tag) => tags.add(tag));
  });
  return Array.from(tags).map((tag) => ({ label: tag, value: tag }));
});

const creatorOptions = computed(() => {
  const creators = Array.from(new Set(audio.assets.map((asset) => asset.createdBy)));
  return creators.map((creator) => ({ label: creator, value: creator }));
});

const columns = computed<DataTableColumns<AudioAsset>>(() => [
  {
    type: 'selection',
    multiple: true,
    disabled: () => !audio.canManage,
    fixed: 'left',
  },
  {
    title: '名称',
    key: 'name',
    minWidth: 200,
    render: (row) =>
      h('div', { class: 'audio-table__name' }, [
        h('span', row.name),
        row.description ? h('p', { class: 'audio-table__desc' }, row.description) : null,
      ]),
  },
  {
    title: '文件夹',
    key: 'folder',
    minWidth: 120,
    render: (row) => folderLabel(row.folderId) || '未分类',
  },
  {
    title: '时长',
    key: 'duration',
    width: 90,
    render: (row) => formatDuration(row.duration),
  },
  {
    title: '标签',
    key: 'tags',
    minWidth: 160,
    render: (row) =>
      row.tags.length
        ? h(
            NSpace,
            { size: 4, wrap: true },
            {
              default: () =>
                row.tags.map((tag) =>
                  h(
                    NTag,
                    { size: 'tiny', bordered: false, key: tag },
                    { default: () => tag }
                  )
                ),
            }
          )
        : '-',
  },
  {
    title: '上传者',
    key: 'createdBy',
    width: 120,
    render: (row) => row.createdBy,
  },
  {
    title: '更新时间',
    key: 'updatedAt',
    width: 150,
    render: (row) => formatDate(row.updatedAt),
  },
  {
    title: '可见性',
    key: 'visibility',
    width: 90,
    render: (row) => (row.visibility === 'public' ? '公开' : '受限'),
  },
  {
    title: '操作',
    key: 'actions',
    width: 180,
    render: (row) =>
      h(
        NSpace,
        { size: 4 },
        {
          default: () => [
            h(
              NButton,
              {
                size: 'tiny',
                quaternary: true,
                disabled: !audio.canManage,
                onClick: () => openAssetEditor(row),
              },
              { default: () => '编辑' }
            ),
            h(
              NButton,
              {
                size: 'tiny',
                quaternary: true,
                type: 'error',
                disabled: !audio.canManage,
                onClick: () => confirmDeleteAsset(row),
              },
              { default: () => '删除' }
            ),
          ],
        }
      ),
  },
]);

const rowKey = (row: AudioAsset) => row.id;
const rowClassName = (row: AudioAsset) => (row.id === audio.selectedAssetId ? 'is-selected-row' : '');
const rowProps = (row: AudioAsset) => ({
  onClick: () => audio.setSelectedAsset(row.id),
});

function handleCheckedRowKeysChange(keys: Array<string | number>) {
  if (!Array.isArray(keys)) {
    checkedRowKeys.value = [];
    return;
  }
  checkedRowKeys.value = keys.map((key) => String(key));
}

function clearSelection() {
  checkedRowKeys.value = [];
}

function folderLabel(folderId: string | null) {
  if (!folderId) return '';
  return audio.folderPathLookup[folderId] || folderMap.value.get(folderId)?.name || '';
}

function formatDuration(value: number) {
  if (!value && value !== 0) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function formatDate(value?: string) {
  if (!value) return '未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString();
}

function formatFileSize(value: number) {
  if (!value) return '0 B';
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function handleSearch() {
  const durationFilter = getDurationFilter();
  audio.applyFilters({
    query: keyword.value,
    tags: selectedTags.value,
    creatorIds: selectedCreators.value,
    durationRange: durationFilter,
  });
  clearSelection();
}

function handleResetFilters() {
  keyword.value = '';
  selectedTags.value = [];
  selectedCreators.value = [];
  durationRange.value = [0, durationMax];
  audio.applyFilters({
    query: '',
    tags: [],
    creatorIds: [],
    durationRange: null,
  });
  clearSelection();
}

function getDurationFilter(): [number, number] | null {
  const [min, max] = durationRange.value;
  if (min <= 0 && max >= durationMax) {
    return null;
  }
  return [min, max];
}

async function handleRefresh() {
  await audio.fetchAssets();
  message.success('素材列表已刷新');
}

function scrollToUpload() {
  uploadAnchor.value?.scrollIntoView({ behavior: 'smooth' });
}

function treeNodeProps() {
  return {
    class: 'audio-library__tree-node',
  };
}

async function handleFolderSelect(keys: Array<string | number>) {
  const target = keys.length ? String(keys[0]) : 'all';
  folderKeys.value = target ? [target] : [];
  clearSelection();
  if (target === 'all') {
    await audio.applyFilters({ folderId: null });
    return;
  }
  await audio.applyFilters({ folderId: target });
}

function openCreateFolder() {
  folderModalMode.value = 'create';
  folderForm.id = '';
  folderForm.name = '';
  folderForm.parentId = currentFolder.value?.id ?? null;
  folderModalVisible.value = true;
}

function openRenameFolder() {
  if (!currentFolder.value) return;
  folderModalMode.value = 'rename';
  folderForm.id = currentFolder.value.id;
  folderForm.name = currentFolder.value.name;
  folderForm.parentId = currentFolder.value.parentId;
  folderModalVisible.value = true;
}

async function handleSaveFolder() {
  if (!audio.canManage) {
    message.error('没有权限管理文件夹');
    return;
  }
  await folderFormRef.value?.validate();
  try {
    if (folderModalMode.value === 'create') {
      await audio.createFolder({ name: folderForm.name.trim(), parentId: folderForm.parentId });
      message.success('文件夹已创建');
    } else {
      await audio.updateFolder(folderForm.id, { name: folderForm.name.trim(), parentId: folderForm.parentId });
      message.success('文件夹已更新');
    }
    folderModalVisible.value = false;
  } catch (err) {
    message.error('保存文件夹失败，请稍后重试');
    console.warn(err);
  }
}

function confirmDeleteFolder() {
  if (!currentFolder.value) return;
  dialog.warning({
    title: '删除确认',
    content: `确定删除“${currentFolder.value.name}”及其子文件夹吗？素材将移动到未分类。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await audio.deleteFolder(currentFolder.value!.id);
        message.success('文件夹已删除');
      } catch (err) {
        message.error('删除文件夹失败');
        console.warn(err);
      }
    },
  });
}

function openAssetEditor(asset: AudioAsset) {
  if (!audio.canManage) return;
  assetForm.id = asset.id;
  assetForm.name = asset.name;
  assetForm.description = asset.description || '';
  assetForm.tags = [...asset.tags];
  assetForm.folderId = asset.folderId;
  assetForm.visibility = asset.visibility;
  assetDrawerVisible.value = true;
}

async function handleSaveAsset() {
  await assetFormRef.value?.validate();
  try {
    await audio.updateAssetMeta(assetForm.id, {
      name: assetForm.name.trim(),
      description: assetForm.description,
      tags: [...assetForm.tags],
      folderId: assetForm.folderId,
      visibility: assetForm.visibility,
    });
    message.success('素材信息已保存');
    assetDrawerVisible.value = false;
  } catch (err) {
    console.warn(err);
    message.error('保存失败，请稍后重试');
  }
}

function confirmDeleteAsset(asset: AudioAsset) {
  if (!audio.canManage) return;
  dialog.warning({
    title: '删除素材',
    content: `确定删除“${asset.name}”吗？删除后播放列表将无法引用该素材。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await audio.deleteAsset(asset.id);
        message.success('素材已删除');
      } catch (err) {
        console.warn(err);
        message.error('删除失败，请稍后重试');
      }
    },
  });
}

function copyStream(id: string) {
  const url = audio.buildStreamUrl(id);
  navigator.clipboard.writeText(url).then(() => {
    message.success('播放链接已复制');
  });
}

function openBatchMoveModal() {
  if (!audio.canManage || !selectionCount.value) return;
  batchMoveTarget.value = currentFolder.value?.id ?? null;
  batchMoveModalVisible.value = true;
}

async function handleBatchMoveSave() {
  if (!audio.canManage) return;
  try {
    const summary = await audio.batchUpdateAssets(checkedRowKeys.value, {
      folderId: batchMoveTarget.value ?? null,
    });
    if (summary.success) {
      message.success(`已移动 ${summary.success} 条素材`);
    }
    if (summary.failed) {
      message.warning(`${summary.failed} 条素材移动失败`);
    }
    batchMoveModalVisible.value = false;
    clearSelection();
  } catch (err) {
    console.warn(err);
    message.error('批量移动失败');
  }
}

function openBatchVisibilityModal() {
  if (!audio.canManage || !selectionCount.value) return;
  batchVisibilityValue.value = 'public';
  batchVisibilityModalVisible.value = true;
}

async function handleBatchVisibilitySave() {
  if (!audio.canManage) return;
  try {
    const summary = await audio.batchUpdateAssets(checkedRowKeys.value, {
      visibility: batchVisibilityValue.value,
    });
    if (summary.success) {
      message.success(`已更新 ${summary.success} 条素材的可见性`);
    }
    if (summary.failed) {
      message.warning(`${summary.failed} 条素材更新失败`);
    }
    batchVisibilityModalVisible.value = false;
    clearSelection();
  } catch (err) {
    console.warn(err);
    message.error('批量修改可见性失败');
  }
}

function confirmBatchDelete() {
  if (!audio.canManage || !selectionCount.value) return;
  dialog.warning({
    title: '批量删除素材',
    content: `确定删除已选的 ${selectionCount.value} 条素材吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const summary = await audio.batchDeleteAssets(checkedRowKeys.value);
        if (summary.success) {
          message.success(`已删除 ${summary.success} 条素材`);
        }
        if (summary.failed) {
          message.warning(`${summary.failed} 条素材未能删除`);
        }
        clearSelection();
      } catch (err) {
        console.warn(err);
        message.error('批量删除失败');
      }
    },
  });
}

watch(
  () => audio.filters,
  (filters) => {
    keyword.value = filters.query ?? '';
    selectedTags.value = [...filters.tags];
    selectedCreators.value = [...filters.creatorIds];
    durationRange.value = filters.durationRange ? [...filters.durationRange] as [number, number] : [0, durationMax];
    folderKeys.value = filters.folderId ? [filters.folderId] : ['all'];
  },
  { deep: true }
);

watch(
  () => audio.filteredAssets,
  (list) => {
    if (!list.length) {
      audio.setSelectedAsset(null);
      return;
    }
    if (!audio.selectedAssetId || !list.some((item) => item.id === audio.selectedAssetId)) {
      audio.setSelectedAsset(list[0].id);
    }
  },
  { immediate: true }
);

watch(
  () => tableData.value,
  (list) => {
    const safeList = Array.isArray(list) ? list : [];
    const available = new Set(safeList.map((item) => item.id));
    checkedRowKeys.value = checkedRowKeys.value.filter((key) => available.has(key));
  },
  { deep: true }
);

watch(
  () => audio.canManage,
  (canManage) => {
    if (!canManage) {
      clearSelection();
    }
  }
);

onMounted(() => {
  if (!audio.initialized) {
    audio.ensureInitialized();
  }
  if (!audio.selectedAsset && audio.filteredAssets.length) {
    audio.setSelectedAsset(audio.filteredAssets[0].id);
  }
});
</script>

<style scoped lang="scss">
.audio-library {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.audio-library__toolbar {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
}

.audio-library__filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0.5rem;
  align-items: center;
}

.audio-library__filter-item {
  width: 100%;
}

.audio-library__duration {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.audio-library__filter-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.audio-library__toolbar-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
}

.audio-library__alert {
  margin-top: 0.25rem;
}

.audio-library__content {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr) 260px;
  gap: 0.75rem;
  min-height: 420px;
}

.audio-library__selection {
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.5rem 0.75rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(99, 179, 237, 0.08);
}

.audio-library__folders,
.audio-library__table,
.audio-library__detail {
  border: 1px solid var(--audio-card-border, var(--sc-border-mute));
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
}

.audio-library__folder-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.audio-library__folder-actions {
  display: flex;
  gap: 0.25rem;
}

.audio-library__table {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.audio-library__pagination {
  display: flex;
  justify-content: flex-end;
}

.audio-library__detail {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.audio-library__detail-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.audio-library__detail-subtitle {
  margin: 0;
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.audio-library__detail-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.85rem;
}

.audio-library__tags,
.audio-library__description {
  font-size: 0.85rem;
}

.audio-library__tag {
  margin-right: 0.25rem;
  margin-bottom: 0.25rem;
}

.audio-library__detail-actions {
  display: flex;
  gap: 0.5rem;
}

.audio-library__upload {
  margin-top: 0.5rem;
}

.audio-library__modal-tip {
  margin: 0 0 0.5rem;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.audio-asset-drawer__header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.audio-table__name {
  display: flex;
  flex-direction: column;
}

.audio-table__desc {
  margin: 0;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

:deep(.is-selected-row td) {
  background-color: rgba(99, 179, 237, 0.08);
}
</style>
