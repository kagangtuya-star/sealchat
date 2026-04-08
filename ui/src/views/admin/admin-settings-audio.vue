<script setup lang="ts">
import { api } from '@/stores/_config';
import type {
  AdminAudioAssetItem,
  AdminAudioAssetListResult,
  AdminAudioCleanupPreview,
  AdminAudioFilterOption,
  AudioAssetUsageSummary,
  AudioBulkDeleteFailure,
  AudioBulkDeleteResult,
} from '@/types/audio';
import { Search, Refresh, Trash } from '@vicons/tabler';
import { NButton, NTag, useDialog, useMessage, type DataTableColumns } from 'naive-ui';
import { computed, h, onMounted, ref } from 'vue';

type AdminAudioSearchField = 'all' | 'name' | 'worldName' | 'creatorName';
type AdminAudioSortField = 'updatedAt' | 'name' | 'scope' | 'worldName' | 'creatorName' | 'size' | 'lastAccessedAt';

const message = useMessage();
const dialog = useDialog();

const loading = ref(false);
const rows = ref<AdminAudioAssetItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const keyword = ref('');
const selectedScope = ref<'all' | 'common' | 'world'>('all');
const selectedWorldId = ref<string | null>(null);
const selectedCreatorId = ref<string | null>(null);
const selectedReferenced = ref<'all' | 'yes' | 'no'>('all');
const selectedNeverAccessed = ref<'all' | 'yes' | 'no'>('all');
const inactiveDays = ref<number | null>(null);
const activeSearchField = ref<AdminAudioSearchField>('all');
const sortBy = ref<AdminAudioSortField>('updatedAt');
const sortOrder = ref<'asc' | 'desc'>('desc');
const worldOptions = ref<AdminAudioFilterOption[]>([]);
const creatorOptions = ref<AdminAudioFilterOption[]>([]);
const checkedRowKeys = ref<string[]>([]);
const selectedAssetId = ref<string | null>(null);
const detailModalVisible = ref(false);

const cleanupModalVisible = ref(false);
const cleanupLoading = ref(false);
const cleanupDays = ref<number>(30);
const cleanupPreview = ref<AdminAudioCleanupPreview | null>(null);

let searchTimer: ReturnType<typeof setTimeout> | null = null;

const selectedAsset = computed(() => rows.value.find((item) => item.id === selectedAssetId.value) || null);
const selectionCount = computed(() => checkedRowKeys.value.length);
const hasSelection = computed(() => selectionCount.value > 0);
const cleanupThresholdText = computed(() => (cleanupPreview.value ? formatDate(cleanupPreview.value.thresholdBefore) : ''));
const selectedSceneNames = computed(() => selectedAsset.value?.usageSummary?.sceneNames || []);
const selectedPlaybackLabels = computed(() => selectedAsset.value?.usageSummary?.playbackScopeLabels || []);
const canExecuteCleanup = computed(() => Boolean(cleanupPreview.value?.safeCandidates));
const searchFieldLabelMap: Record<AdminAudioSearchField, string> = {
  all: '全部字段',
  name: '名称',
  worldName: '所属世界',
  creatorName: '上传者',
};
const searchPlaceholder = computed(() => (
  activeSearchField.value === 'all'
    ? '搜索名称 / 备注 / 标签'
    : `仅搜索${searchFieldLabelMap[activeSearchField.value]}`
));
const activeSearchLabel = computed(() => searchFieldLabelMap[activeSearchField.value]);

const scopeOptions = [
  { label: '全部级别', value: 'all' },
  { label: '通用级', value: 'common' },
  { label: '世界级', value: 'world' },
];

const referencedOptions = [
  { label: '全部引用状态', value: 'all' },
  { label: '已被引用', value: 'yes' },
  { label: '未被引用', value: 'no' },
];

const neverAccessedOptions = [
  { label: '全部访问状态', value: 'all' },
  { label: '从未访问', value: 'yes' },
  { label: '已有访问记录', value: 'no' },
];

const cleanupDayOptions = [
  { label: '7 天', value: 7 },
  { label: '30 天', value: 30 },
  { label: '90 天', value: 90 },
  { label: '180 天', value: 180 },
];

function renderHeaderTrigger(label: string, field: AdminAudioSortField, searchField?: AdminAudioSearchField) {
  const sortActive = sortBy.value === field;
  const searchActive = Boolean(searchField && activeSearchField.value === searchField);
  const sortGlyph = sortActive ? (sortOrder.value === 'asc' ? '↑' : '↓') : '↕';
  return h(
    'button',
    {
      class: [
        'admin-audio__header-trigger',
        sortActive && 'admin-audio__header-trigger--sorted',
        searchActive && 'admin-audio__header-trigger--searching',
      ],
      type: 'button',
      onClick: () => handleHeaderTrigger(field, searchField),
    },
    [
      h('span', { class: 'admin-audio__header-label' }, label),
      searchField ? h('span', { class: 'admin-audio__header-search-indicator' }, '筛') : null,
      h('span', { class: 'admin-audio__header-sort-indicator' }, sortGlyph),
    ],
  );
}

const columns = computed<DataTableColumns<AdminAudioAssetItem>>(() => [
  {
    type: 'selection',
    disabled: (row) => !row.safeToDelete,
  },
  {
    title: () => renderHeaderTrigger('名称', 'name', 'name'),
    key: 'name',
    minWidth: 280,
    render: (row) => h('div', { class: 'admin-audio__name-cell' }, [
      h(
        'button',
        {
          class: 'admin-audio__name-button',
          type: 'button',
          onClick: (event: MouseEvent) => {
            event.stopPropagation();
            openDetail(row);
          },
        },
        row.name,
      ),
      row.description ? h('p', { class: 'admin-audio__desc' }, row.description) : null,
    ]),
  },
  {
    title: () => renderHeaderTrigger('级别', 'scope'),
    key: 'scope',
    width: 90,
    render: (row) => h(
      NTag,
      { size: 'small', type: row.scope === 'common' ? 'info' : 'warning' },
      { default: () => (row.scope === 'common' ? '通用级' : '世界级') },
    ),
  },
  {
    title: () => renderHeaderTrigger('所属世界', 'worldName', 'worldName'),
    key: 'worldName',
    width: 160,
    render: (row) => row.worldName || '全局',
  },
  {
    title: () => renderHeaderTrigger('上传者', 'creatorName', 'creatorName'),
    key: 'creatorName',
    width: 140,
    render: (row) => row.creatorName || row.createdBy,
  },
  {
    title: () => renderHeaderTrigger('大小', 'size'),
    key: 'size',
    width: 120,
    render: (row) => formatFileSize(row.size),
  },
  {
    title: () => renderHeaderTrigger('修改时间', 'updatedAt'),
    key: 'updatedAt',
    width: 160,
    render: (row) => formatDate(row.updatedAt),
  },
  {
    title: () => renderHeaderTrigger('最近访问', 'lastAccessedAt'),
    key: 'lastAccessedAt',
    width: 160,
    render: (row) => formatAccessTime(row.lastAccessedAt),
  },
  {
    title: '状态',
    key: 'usageSummary',
    width: 128,
    render: (row) => h('div', { class: 'admin-audio__usage-cell' }, [
      h(
        NTag,
        { size: 'small', type: row.safeToDelete ? 'success' : 'error' },
        { default: () => (row.safeToDelete ? '可安全删除' : '仍被引用') },
      ),
    ]),
  },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row) => h('div', { class: 'admin-audio__action-cell' }, [
      h(
        NButton,
        {
          size: 'small',
          tertiary: true,
          onClick: () => openDetail(row),
        },
        { default: () => '查看信息' },
      ),
      h(
        NButton,
        {
          size: 'small',
          tertiary: true,
          type: 'error',
          disabled: !row.safeToDelete,
          onClick: () => confirmDelete(row),
        },
        { default: () => '删除' },
      ),
    ]),
  },
]);

const cleanupColumns = computed<DataTableColumns<AdminAudioAssetItem>>(() =>
  columns.value.filter((column) => (column as any).type !== 'selection' && (column as any).key !== 'actions'),
);

onMounted(() => {
  void refresh();
});

function buildListParams() {
  return {
    page: page.value,
    pageSize: pageSize.value,
    query: keyword.value.trim() || undefined,
    queryField: activeSearchField.value === 'all' ? undefined : activeSearchField.value,
    sortBy: sortBy.value,
    sortOrder: sortOrder.value,
    scope: selectedScope.value === 'all' ? undefined : selectedScope.value,
    worldId: selectedWorldId.value || undefined,
    creatorId: selectedCreatorId.value || undefined,
    referenced:
      selectedReferenced.value === 'all'
        ? undefined
        : selectedReferenced.value === 'yes',
    neverAccessed:
      selectedNeverAccessed.value === 'all'
        ? undefined
        : selectedNeverAccessed.value === 'yes',
    inactiveDays: inactiveDays.value || undefined,
  };
}

async function refresh() {
  loading.value = true;
  try {
    const resp = await api.get<AdminAudioAssetListResult>('/api/v1/admin/audio-assets', {
      params: buildListParams(),
    });
    rows.value = resp.data.items || [];
    total.value = resp.data.total || 0;
    worldOptions.value = resp.data.worldOptions || [];
    creatorOptions.value = resp.data.creatorOptions || [];
    const validSelection = new Set(rows.value.filter((item) => item.safeToDelete).map((item) => item.id));
    checkedRowKeys.value = checkedRowKeys.value.filter((item) => validSelection.has(item));
    if (selectedAssetId.value && rows.value.some((item) => item.id === selectedAssetId.value)) {
      return;
    }
    selectedAssetId.value = rows.value[0]?.id ?? null;
  } finally {
    loading.value = false;
  }
}

function handleSearchInput() {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    page.value = 1;
    void refresh();
  }, 250);
}

function applyFilters() {
  page.value = 1;
  void refresh();
}

function handleHeaderTrigger(field: AdminAudioSortField, searchField?: AdminAudioSearchField) {
  if (searchField) {
    activeSearchField.value = searchField;
  }
  if (sortBy.value === field) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc';
  } else {
    sortBy.value = field;
    sortOrder.value = field === 'updatedAt' || field === 'lastAccessedAt' || field === 'size' ? 'desc' : 'asc';
  }
  page.value = 1;
  void refresh();
}

function clearColumnSearchField() {
  activeSearchField.value = 'all';
  page.value = 1;
  void refresh();
}

function resetFilters() {
  keyword.value = '';
  activeSearchField.value = 'all';
  sortBy.value = 'updatedAt';
  sortOrder.value = 'desc';
  selectedScope.value = 'all';
  selectedWorldId.value = null;
  selectedCreatorId.value = null;
  selectedReferenced.value = 'all';
  selectedNeverAccessed.value = 'all';
  inactiveDays.value = null;
  page.value = 1;
  void refresh();
}

function handlePageChange(nextPage: number) {
  page.value = nextPage;
  void refresh();
}

function handlePageSizeChange(nextPageSize: number) {
  pageSize.value = nextPageSize;
  page.value = 1;
  void refresh();
}

function handleCheckedRowKeysChange(keys: Array<string | number>) {
  checkedRowKeys.value = keys.map((key) => String(key));
}

function selectRow(row: AdminAudioAssetItem) {
  selectedAssetId.value = row.id;
}

function openDetail(row: AdminAudioAssetItem) {
  selectRow(row);
  detailModalVisible.value = true;
}

function rowProps(row: AdminAudioAssetItem) {
  return {
    style: 'cursor: pointer;',
    onClick: () => selectRow(row),
    onDblclick: () => openDetail(row),
  };
}

function formatFileSize(value?: number) {
  const size = value ?? 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function formatDuration(value?: number) {
  const seconds = Math.max(0, Math.floor(value ?? 0));
  const minutes = Math.floor(seconds / 60);
  const remain = seconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(remain).padStart(2, '0')}`;
}

function formatDate(value?: string | null) {
  if (!value) return '未知';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '未知';
  return date.toLocaleString();
}

function formatAccessTime(value?: string | null) {
  if (!value) return '从未记录访问';
  return formatDate(value);
}

function resolveUsageSummaryText(usage?: AudioAssetUsageSummary | null) {
  const sceneCount = usage?.sceneRefCount || 0;
  const playbackCount = usage?.playbackStateRefCount || 0;
  if (!sceneCount && !playbackCount) {
    return '未被场景或当前播放引用';
  }
  return `场景 ${sceneCount} / 播放状态 ${playbackCount}`;
}

function resolveUsageText(row: AdminAudioAssetItem) {
  return resolveUsageSummaryText(row.usageSummary);
}

function buildFailureSummary(failed: AudioBulkDeleteFailure[] = []) {
  if (!failed.length) return '';
  const lines = failed.slice(0, 5).map((item) => {
    const usageText = item.usageSummary ? resolveUsageSummaryText(item.usageSummary) : '';
    return `${item.assetId}：${item.reason}${usageText ? `（${usageText}）` : ''}`;
  });
  if (failed.length > 5) {
    lines.push(`其余 ${failed.length - 5} 条失败素材请结合筛选条件重新查看列表。`);
  }
  return lines.join('\n');
}

function openFailureDialog(title: string, failed: AudioBulkDeleteFailure[] = []) {
  if (!failed.length) return;
  dialog.warning({
    title,
    content: buildFailureSummary(failed),
    positiveText: '知道了',
  });
}

function openReferencedDialog(assetName: string, usage?: AudioAssetUsageSummary) {
  dialog.warning({
    title: '素材仍被引用',
    content: `“${assetName}” 当前无法删除。${usage ? `引用情况：${resolveUsageSummaryText(usage)}` : ''}`,
    positiveText: '知道了',
  });
}

function extractErrorMessage(error: any, fallback: string) {
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || fallback;
}

function confirmDelete(row: AdminAudioAssetItem) {
  dialog.warning({
    title: '安全删除音频素材',
    content: `确定删除“${row.name}”吗？该操作会执行引用检查。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.delete(`/api/v1/admin/audio-assets/${row.id}`);
        message.success('素材已删除');
        checkedRowKeys.value = checkedRowKeys.value.filter((item) => item !== row.id);
        await refresh();
      } catch (error) {
        const requestError = error as any;
        if (requestError?.response?.status === 409) {
          openReferencedDialog(row.name, requestError?.response?.data?.usage);
        }
        message.error(extractErrorMessage(requestError, '删除失败'));
      }
    },
  });
}

function confirmBulkDelete() {
  if (!checkedRowKeys.value.length) return;
  dialog.warning({
    title: '批量安全删除',
    content: `确定删除选中的 ${checkedRowKeys.value.length} 条素材吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const resp = await api.post<AudioBulkDeleteResult>('/api/v1/admin/audio-assets/bulk-delete', {
          ids: checkedRowKeys.value,
        });
        const successCount = resp.data?.successCount || 0;
        const failedCount = resp.data?.failedCount || 0;
        if (successCount) {
          message.success(`已删除 ${successCount} 条素材`);
        }
        if (failedCount) {
          message.warning(`${failedCount} 条素材删除失败`);
          openFailureDialog('批量删除失败详情', resp.data?.failed || []);
        }
        checkedRowKeys.value = [];
        await refresh();
      } catch (error) {
        message.error(extractErrorMessage(error, '批量删除失败'));
      }
    },
  });
}

async function loadCleanupPreview() {
  cleanupLoading.value = true;
  try {
    const resp = await api.get<AdminAudioCleanupPreview>('/api/v1/admin/audio-assets/cleanup-preview', {
      params: {
        days: cleanupDays.value,
        query: keyword.value.trim() || undefined,
        queryField: activeSearchField.value === 'all' ? undefined : activeSearchField.value,
        sortBy: sortBy.value,
        sortOrder: sortOrder.value,
        scope: selectedScope.value === 'all' ? undefined : selectedScope.value,
        worldId: selectedWorldId.value || undefined,
        creatorId: selectedCreatorId.value || undefined,
      },
    });
    cleanupPreview.value = resp.data;
  } catch (error) {
    message.error(extractErrorMessage(error, '读取清理预览失败'));
  } finally {
    cleanupLoading.value = false;
  }
}

async function executeCleanup() {
  cleanupLoading.value = true;
  try {
    const resp = await api.post<AudioBulkDeleteResult>('/api/v1/admin/audio-assets/cleanup', {
      days: cleanupDays.value,
      query: keyword.value.trim() || undefined,
      queryField: activeSearchField.value === 'all' ? undefined : activeSearchField.value,
      sortBy: sortBy.value,
      sortOrder: sortOrder.value,
      scope: selectedScope.value === 'all' ? undefined : selectedScope.value,
      worldId: selectedWorldId.value || undefined,
      creatorId: selectedCreatorId.value || undefined,
    });
    const successCount = resp.data?.successCount || 0;
    const failedCount = resp.data?.failedCount || 0;
    if (successCount) {
      message.success(`已清理 ${successCount} 条素材`);
    } else {
      message.info('没有可清理的素材');
    }
    if (failedCount) {
      message.warning(`${failedCount} 条素材因引用或其他原因未删除`);
      openFailureDialog('安全清理失败详情', resp.data?.failed || []);
    }
    cleanupModalVisible.value = false;
    cleanupPreview.value = null;
    checkedRowKeys.value = [];
    await refresh();
  } catch (error) {
    message.error(extractErrorMessage(error, '执行清理失败'));
  } finally {
    cleanupLoading.value = false;
  }
}
</script>

<template>
  <div class="admin-audio">
    <div class="admin-audio__toolbar">
      <div class="admin-audio__filters">
        <n-input
          v-model:value="keyword"
          clearable
          :placeholder="searchPlaceholder"
          @input="handleSearchInput"
          @clear="handleSearchInput"
        >
          <template #prefix>
            <n-icon :component="Search" />
          </template>
        </n-input>

        <n-tag size="small" :type="activeSearchField === 'all' ? 'default' : 'info'">
          搜索列：{{ activeSearchLabel }}
        </n-tag>

        <n-button v-if="activeSearchField !== 'all'" quaternary size="small" @click="clearColumnSearchField">
          清除列筛选
        </n-button>

        <n-select
          v-model:value="selectedScope"
          :options="scopeOptions"
          style="width: 120px"
          @update:value="applyFilters"
        />

        <n-select
          v-model:value="selectedWorldId"
          clearable
          filterable
          :options="worldOptions"
          placeholder="所属世界"
          style="width: 180px"
          @update:value="applyFilters"
        />

        <n-select
          v-model:value="selectedCreatorId"
          clearable
          filterable
          :options="creatorOptions"
          placeholder="上传者"
          style="width: 160px"
          @update:value="applyFilters"
        />

        <n-select
          v-model:value="selectedReferenced"
          :options="referencedOptions"
          style="width: 140px"
          @update:value="applyFilters"
        />

        <n-select
          v-model:value="selectedNeverAccessed"
          :options="neverAccessedOptions"
          style="width: 140px"
          @update:value="applyFilters"
        />

        <n-input-number
          v-model:value="inactiveDays"
          clearable
          :min="1"
          placeholder="超过 N 天未访问"
          style="width: 170px"
          @update:value="applyFilters"
        />
      </div>

      <div class="admin-audio__actions">
        <n-button @click="refresh" :loading="loading">
          <template #icon>
            <n-icon :component="Refresh" />
          </template>
          刷新
        </n-button>
        <n-button @click="resetFilters">重置筛选</n-button>
        <n-button :disabled="!hasSelection" type="error" @click="confirmBulkDelete">
          <template #icon>
            <n-icon :component="Trash" />
          </template>
          批量删除
        </n-button>
        <n-button type="warning" @click="cleanupModalVisible = true; cleanupPreview = null">
          <template #icon>
            <n-icon :component="Trash" />
          </template>
          清理未使用素材
        </n-button>
      </div>
    </div>

    <div class="admin-audio__stats">
      共 <n-text type="primary">{{ total }}</n-text> 条素材
      <span v-if="hasSelection">，已选 {{ selectionCount }} 条</span>
    </div>

    <div class="admin-audio__content">
      <div class="admin-audio__table-card">
        <div class="admin-audio__table-scroll">
          <n-data-table
            :columns="columns"
            :data="rows"
            :loading="loading"
            :pagination="false"
            :checked-row-keys="checkedRowKeys"
            :row-key="(row: AdminAudioAssetItem) => row.id"
            :row-props="rowProps"
            size="small"
            :scroll-x="1520"
            :max-height="520"
            @update:checked-row-keys="handleCheckedRowKeysChange"
          />
        </div>
        <div class="admin-audio__pagination">
          <n-pagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[10, 20, 50, 100]"
            show-size-picker
            show-quick-jumper
            :on-update:page="handlePageChange"
            :on-update:page-size="handlePageSizeChange"
          >
            <template #prefix="{ itemCount }">
              共 {{ itemCount }} 条
            </template>
          </n-pagination>
        </div>
      </div>
    </div>

    <n-modal v-model:show="detailModalVisible" preset="card" title="音频素材信息" class="admin-audio__detail-modal" :style="{ width: 'min(760px, 92vw)' }">
      <template v-if="selectedAsset">
        <div class="admin-audio__detail-header">
          <div>
            <h3>{{ selectedAsset.name }}</h3>
            <p>{{ selectedAsset.worldName || '全局素材' }}</p>
          </div>
          <n-tag :type="selectedAsset.safeToDelete ? 'success' : 'error'">
            {{ selectedAsset.safeToDelete ? '可安全删除' : '仍被引用' }}
          </n-tag>
        </div>

        <n-descriptions label-placement="top" :column="2" size="small" bordered>
          <n-descriptions-item label="上传者">
            {{ selectedAsset.creatorName || selectedAsset.createdBy }}
          </n-descriptions-item>
          <n-descriptions-item label="作用域">
            {{ selectedAsset.scope === 'common' ? '通用级' : '世界级' }}
          </n-descriptions-item>
          <n-descriptions-item label="文件大小">
            {{ formatFileSize(selectedAsset.size) }}
          </n-descriptions-item>
          <n-descriptions-item label="音频时长">
            {{ formatDuration(selectedAsset.duration) }}
          </n-descriptions-item>
          <n-descriptions-item label="最近访问">
            {{ formatAccessTime(selectedAsset.lastAccessedAt) }}
          </n-descriptions-item>
          <n-descriptions-item label="访问次数">
            {{ selectedAsset.accessCount ?? 0 }}
          </n-descriptions-item>
          <n-descriptions-item label="创建时间">
            {{ formatDate(selectedAsset.createdAt) }}
          </n-descriptions-item>
          <n-descriptions-item label="更新时间">
            {{ formatDate(selectedAsset.updatedAt) }}
          </n-descriptions-item>
          <n-descriptions-item :span="2" label="引用状态">
            {{ resolveUsageText(selectedAsset) }}
          </n-descriptions-item>
          <n-descriptions-item :span="2" label="引用来源">
            <div class="admin-audio__reference-groups">
              <div class="admin-audio__reference-group">
                <strong>场景</strong>
                <div class="admin-audio__tags">
                  <n-tag v-for="name in selectedSceneNames" :key="name" size="small" type="info">{{ name }}</n-tag>
                  <span v-if="!selectedSceneNames.length">无</span>
                </div>
              </div>
              <div class="admin-audio__reference-group">
                <strong>当前播放状态</strong>
                <div class="admin-audio__tags">
                  <n-tag v-for="label in selectedPlaybackLabels" :key="label" size="small" type="warning">{{ label }}</n-tag>
                  <span v-if="!selectedPlaybackLabels.length">无</span>
                </div>
              </div>
            </div>
          </n-descriptions-item>
          <n-descriptions-item :span="2" label="标签">
            <div class="admin-audio__tags">
              <n-tag v-for="tag in selectedAsset.tags" :key="tag" size="small">{{ tag }}</n-tag>
              <span v-if="!selectedAsset.tags?.length">未设置</span>
            </div>
          </n-descriptions-item>
          <n-descriptions-item :span="2" label="备注">
            {{ selectedAsset.description || '暂无备注' }}
          </n-descriptions-item>
        </n-descriptions>
      </template>
      <n-empty v-else description="没有可查看的素材信息" />
    </n-modal>

    <n-modal v-model:show="cleanupModalVisible" preset="dialog" title="清理指定周期未使用素材" :mask-closable="false">
      <div class="admin-audio__cleanup">
        <n-radio-group v-model:value="cleanupDays">
          <n-radio-button v-for="option in cleanupDayOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </n-radio-button>
        </n-radio-group>

        <div class="admin-audio__cleanup-actions">
          <n-button @click="loadCleanupPreview" :loading="cleanupLoading">生成预览</n-button>
        </div>

        <template v-if="cleanupPreview">
          <n-alert type="info" :show-icon="false">
            截止 {{ cleanupThresholdText }} 未访问的候选共 {{ cleanupPreview.totalCandidates }} 条，可安全清理 {{ cleanupPreview.safeCandidates }} 条，因引用跳过 {{ cleanupPreview.referencedSkipped }} 条。
          </n-alert>
          <n-alert v-if="cleanupPreview.referencedSkipped" type="warning" :show-icon="false">
            预览结果中仅展示可安全清理的素材；被场景或当前播放引用的素材已自动排除。
          </n-alert>
          <n-data-table
            :columns="cleanupColumns"
            :data="cleanupPreview.items"
            :pagination="false"
            size="small"
            :max-height="260"
          />
        </template>
      </div>

      <template #action>
        <n-space justify="end">
          <n-button @click="cleanupModalVisible = false">关闭</n-button>
          <n-button type="primary" :disabled="!canExecuteCleanup" :loading="cleanupLoading" @click="executeCleanup">
            执行安全清理
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.admin-audio {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  min-height: 0;
  overflow: auto;
  padding-right: 2px;
}

.admin-audio__toolbar {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.admin-audio__filters {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.admin-audio__actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.admin-audio__stats {
  color: var(--n-text-color-3);
  font-size: 13px;
}

.admin-audio__content {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.admin-audio__table-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  padding: 12px;
  background: var(--n-card-color);
}

.admin-audio__table-scroll {
  min-height: 0;
  overflow: hidden;
}

.admin-audio__table-card :deep(.n-data-table-wrapper) {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.35) transparent;
}

.admin-audio__table-card :deep(.n-data-table-wrapper)::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.admin-audio__table-card :deep(.n-data-table-wrapper)::-webkit-scrollbar-track {
  background: transparent;
}

.admin-audio__table-card :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.35);
  border-radius: 999px;
}

.admin-audio__table-card :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb:hover {
  background: rgba(128, 128, 128, 0.55);
}

.admin-audio__pagination {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--n-border-color);
  margin-top: 12px;
}

.admin-audio__detail-modal :deep(.n-card__content) {
  max-height: 70vh;
  overflow: auto;
}

.admin-audio__detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}

.admin-audio__detail-header h3 {
  margin: 0 0 4px;
  font-size: 18px;
}

.admin-audio__detail-header p {
  margin: 0;
  color: var(--n-text-color-3);
}

.admin-audio__name-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.admin-audio__header-trigger {
  appearance: none;
  border: 0;
  background: transparent;
  color: inherit;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  font-weight: 600;
  cursor: pointer;
}

.admin-audio__header-trigger--sorted,
.admin-audio__header-trigger--searching {
  color: var(--n-primary-color);
}

.admin-audio__header-search-indicator,
.admin-audio__header-sort-indicator {
  font-size: 11px;
  line-height: 1;
  opacity: 0.8;
}

.admin-audio__name-button {
  appearance: none;
  border: 0;
  background: transparent;
  padding: 0;
  text-align: left;
  font-size: 14px;
  font-weight: 600;
  color: var(--n-text-color);
  cursor: pointer;
}

.admin-audio__name-button:hover {
  color: var(--n-primary-color);
}

.admin-audio__usage-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.admin-audio__desc {
  margin: 0;
  color: var(--n-text-color-3);
  font-size: 12px;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.admin-audio__tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.admin-audio__reference-groups {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.admin-audio__reference-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.admin-audio__action-cell {
  display: flex;
  gap: 6px;
  align-items: center;
}

.admin-audio__cleanup {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.admin-audio__cleanup-actions {
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 960px) {
  .admin-audio__filters {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .admin-audio__actions {
    width: 100%;
  }

  .admin-audio__pagination {
    justify-content: flex-start;
    overflow-x: auto;
  }
}
</style>
