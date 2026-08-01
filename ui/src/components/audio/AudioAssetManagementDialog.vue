<script setup lang="ts">
import { api } from '@/stores/_config';
import type {
  AdminAudioAssetItem,
  AdminAudioFilterOption,
  AudioBulkDeleteResult,
  AudioDeleteImpact,
  AudioManageAssetListResult,
  AudioQuotaSummary,
} from '@/types/audio';
import { Refresh, Search, Trash } from '@vicons/tabler';
import { NButton, NTag, useDialog, useMessage, type DataTableColumns } from 'naive-ui';
import { computed, h, ref, watch } from 'vue';

type SearchField = 'all' | 'name' | 'worldName' | 'creatorName';
type SortField = 'updatedAt' | 'name' | 'scope' | 'worldName' | 'creatorName' | 'size' | 'lastAccessedAt';

const props = withDefaults(defineProps<{
  show: boolean;
  endpointBase: string;
  title?: string;
  showQuota?: boolean;
  showCleanup?: boolean;
  showQuotaAdmin?: boolean;
}>(), {
  title: '音频素材管理',
  showQuota: false,
  showCleanup: false,
  showQuotaAdmin: false,
});

const emit = defineEmits<{
  'update:show': [value: boolean];
  changed: [];
}>();

const message = useMessage();
const dialog = useDialog();

const visible = computed({
  get: () => props.show,
  set: (value) => emit('update:show', value),
});

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
const sortBy = ref<SortField>('updatedAt');
const sortOrder = ref<'asc' | 'desc'>('desc');
const activeSearchField = ref<SearchField>('all');
const worldOptions = ref<AdminAudioFilterOption[]>([]);
const creatorOptions = ref<AdminAudioFilterOption[]>([]);
const checkedRowKeys = ref<string[]>([]);
const selectedAssetId = ref<string | null>(null);
const detailModalVisible = ref(false);
const quotaSummary = ref<AudioQuotaSummary | null>(null);
let searchTimer: ReturnType<typeof setTimeout> | null = null;

const showCleanup = computed(() => props.showCleanup);
const showQuotaAdmin = computed(() => props.showQuotaAdmin);
const hasSelection = computed(() => checkedRowKeys.value.length > 0);
const selectedAsset = computed(() => rows.value.find((item) => item.id === selectedAssetId.value) || null);
const selectedSceneNames = computed(() => selectedAsset.value?.usageSummary?.sceneNames || []);
const selectedPlaybackLabels = computed(() => selectedAsset.value?.usageSummary?.playbackScopeLabels || []);

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

const columns = computed<DataTableColumns<AdminAudioAssetItem>>(() => [
  { type: 'selection' },
  {
    title: '名称',
    key: 'name',
    minWidth: 260,
    render: (row) => h('div', { class: 'audio-asset-management__name-cell' }, [
      h('button', {
        class: 'audio-asset-management__name-button',
        type: 'button',
        onClick: (event: MouseEvent) => {
          event.stopPropagation();
          openDetail(row);
        },
      }, row.name),
      row.description ? h('p', { class: 'audio-asset-management__desc' }, row.description) : null,
    ]),
  },
  {
    title: '级别',
    key: 'scope',
    width: 90,
    render: (row) => h(NTag, { size: 'small', type: row.scope === 'common' ? 'info' : 'warning' }, {
      default: () => (row.scope === 'common' ? '通用级' : '世界级'),
    }),
  },
  { title: '所属世界', key: 'worldName', width: 150, render: (row) => row.worldName || '全局' },
  { title: '上传者', key: 'creatorName', width: 140, render: (row) => row.creatorName || row.createdBy },
  { title: '大小', key: 'size', width: 110, render: (row) => formatFileSize(row.size) },
  { title: '最近访问', key: 'lastAccessedAt', width: 160, render: (row) => formatAccessTime(row.lastAccessedAt) },
  {
    title: '状态',
    key: 'usageSummary',
    width: 136,
    render: (row) => h(NTag, { size: 'small', type: row.safeToDelete ? 'success' : 'warning' }, {
      default: () => (row.safeToDelete ? '可直接删除' : '删除时将解除引用'),
    }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 140,
    render: (row) => h('div', { class: 'audio-asset-management__actions-cell' }, [
      h(NButton, { size: 'small', tertiary: true, onClick: () => openDetail(row) }, { default: () => '查看' }),
      h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => confirmDelete(row) }, { default: () => '删除' }),
    ]),
  },
]);

watch(() => props.show, (show) => {
  if (show) void refresh();
});

function endpoint(path = '') {
  return `${props.endpointBase}${path}`;
}

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
    referenced: selectedReferenced.value === 'all' ? undefined : selectedReferenced.value === 'yes',
    neverAccessed: selectedNeverAccessed.value === 'all' ? undefined : selectedNeverAccessed.value === 'yes',
  };
}

async function refresh() {
  loading.value = true;
  try {
    const resp = await api.get<AudioManageAssetListResult>(endpoint(), { params: buildListParams() });
    rows.value = resp.data.items || [];
    total.value = resp.data.total || 0;
    worldOptions.value = resp.data.worldOptions || [];
    creatorOptions.value = resp.data.creatorOptions || [];
    quotaSummary.value = resp.data.quota || null;
    const validSelection = new Set(rows.value.map((item) => item.id));
    checkedRowKeys.value = checkedRowKeys.value.filter((item) => validSelection.has(item));
    if (!selectedAssetId.value || !rows.value.some((item) => item.id === selectedAssetId.value)) {
      selectedAssetId.value = rows.value[0]?.id ?? null;
    }
  } catch (error) {
    message.error(extractErrorMessage(error, '读取音频素材失败'));
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

function resetFilters() {
  keyword.value = '';
  selectedScope.value = 'all';
  selectedWorldId.value = null;
  selectedCreatorId.value = null;
  selectedReferenced.value = 'all';
  selectedNeverAccessed.value = 'all';
  page.value = 1;
  void refresh();
}

function handleCheckedRowKeysChange(keys: Array<string | number>) {
  checkedRowKeys.value = keys.map((key) => String(key));
}

function openDetail(row: AdminAudioAssetItem) {
  selectedAssetId.value = row.id;
  detailModalVisible.value = true;
}

function rowProps(row: AdminAudioAssetItem) {
  return {
    style: 'cursor: var(--sc-cursor-pointer, pointer);',
    onClick: () => {
      selectedAssetId.value = row.id;
    },
    onDblclick: () => openDetail(row),
  };
}

function formatFileSize(value?: number | null) {
  const size = value ?? 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
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

function formatDuration(value?: number) {
  const seconds = Math.max(0, Math.floor(value ?? 0));
  const minutes = Math.floor(seconds / 60);
  const remain = seconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(remain).padStart(2, '0')}`;
}

function quotaProgress(summary: AudioQuotaSummary | null) {
  if (!summary?.limited || summary.usagePercent == null) return 0;
  return Math.max(0, Math.min(100, summary.usagePercent));
}

function formatQuotaSummary(summary: AudioQuotaSummary | null) {
  if (!summary) return '配额加载中';
  const used = formatFileSize(summary.usedBytes);
  if (!summary.limited) return `已用 ${used} / 无上限`;
  const quota = formatFileSize(summary.quotaBytes || 0);
  const remaining = formatFileSize(summary.remainingBytes || 0);
  return `已用 ${used} / ${quota}，剩余 ${remaining}`;
}

function resolveUsageSummaryText(row?: AdminAudioAssetItem | null) {
  const sceneCount = row?.usageSummary?.sceneRefCount || 0;
  const playbackCount = row?.usageSummary?.playbackStateRefCount || 0;
  if (!sceneCount && !playbackCount) return '未被场景或当前播放引用';
  return `场景 ${sceneCount} / 播放状态 ${playbackCount}`;
}

function extractErrorMessage(error: any, fallback: string) {
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || fallback;
}

function describeDeleteImpact(impact?: AudioDeleteImpact | null) {
  if (!impact) return '';
  const sceneCount = impact.detachedSceneCount || 0;
  const playbackCount = impact.detachedPlaybackStateCount || 0;
  if (!sceneCount && !playbackCount) return '';
  return `已解除 ${sceneCount} 个场景引用、${playbackCount} 个播放状态引用`;
}

function confirmDelete(row: AdminAudioAssetItem) {
  dialog.warning({
    title: '删除音频素材',
    content: `确定删除“${row.name}”吗？当前引用情况：${resolveUsageSummaryText(row)}。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const resp = await api.delete<{ message?: string; impact?: AudioDeleteImpact }>(endpoint(`/${row.id}`));
        const impactText = describeDeleteImpact(resp.data?.impact);
        message.success(impactText ? `素材已删除，${impactText}` : '素材已删除');
        emit('changed');
        await refresh();
      } catch (error) {
        message.error(extractErrorMessage(error, '删除失败'));
      }
    },
  });
}

function confirmBulkDelete() {
  if (!checkedRowKeys.value.length) return;
  dialog.warning({
    title: '批量删除音频素材',
    content: `确定删除选中的 ${checkedRowKeys.value.length} 条素材吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const resp = await api.post<AudioBulkDeleteResult>(endpoint('/bulk-delete'), { ids: checkedRowKeys.value });
        const successCount = resp.data?.successCount || 0;
        const failedCount = resp.data?.failedCount || 0;
        if (successCount) message.success(`已删除 ${successCount} 条素材`);
        if (failedCount) message.warning(`${failedCount} 条素材删除失败`);
        checkedRowKeys.value = [];
        emit('changed');
        await refresh();
      } catch (error) {
        message.error(extractErrorMessage(error, '批量删除失败'));
      }
    },
  });
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
</script>

<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :title="title"
    class="audio-asset-management sc-fluid-modal sc-fluid-modal--xwide"
    :style="{ width: 'min(1180px, 94vw)' }"
  >
    <div class="audio-asset-management__body">
      <section v-if="showQuota" class="audio-asset-management__quota">
        <div>
          <strong>当前音频配额</strong>
          <span>{{ formatQuotaSummary(quotaSummary) }}</span>
        </div>
        <n-progress
          type="line"
          :percentage="quotaProgress(quotaSummary)"
          :show-indicator="false"
          :status="quotaSummary?.limited ? 'default' : 'info'"
        />
      </section>

      <section class="audio-asset-management__toolbar">
        <n-input
          v-model:value="keyword"
          clearable
          placeholder="搜索名称 / 备注 / 标签"
          @input="handleSearchInput"
          @clear="handleSearchInput"
        >
          <template #prefix>
            <n-icon :component="Search" />
          </template>
        </n-input>
        <n-select v-model:value="selectedScope" :options="scopeOptions" style="width: 120px" @update:value="applyFilters" />
        <n-select
          v-model:value="selectedWorldId"
          clearable
          filterable
          :options="worldOptions"
          placeholder="所属世界"
          style="width: 170px"
          @update:value="applyFilters"
        />
        <n-select
          v-model:value="selectedCreatorId"
          clearable
          filterable
          :options="creatorOptions"
          placeholder="上传者"
          style="width: 150px"
          @update:value="applyFilters"
        />
        <n-select v-model:value="selectedReferenced" :options="referencedOptions" style="width: 140px" @update:value="applyFilters" />
        <n-select v-model:value="selectedNeverAccessed" :options="neverAccessedOptions" style="width: 140px" @update:value="applyFilters" />
      </section>

      <section class="audio-asset-management__actions">
        <n-button @click="refresh" :loading="loading">
          <template #icon><n-icon :component="Refresh" /></template>
          刷新
        </n-button>
        <n-button @click="resetFilters">重置筛选</n-button>
        <n-button type="error" :disabled="!hasSelection" @click="confirmBulkDelete">
          <template #icon><n-icon :component="Trash" /></template>
          批量删除
        </n-button>
        <n-button v-if="showCleanup" disabled>清理未使用</n-button>
        <n-button v-if="showQuotaAdmin" disabled>用户配额</n-button>
      </section>

      <div class="audio-asset-management__stats">
        共 <n-text type="primary">{{ total }}</n-text> 条素材
        <span v-if="checkedRowKeys.length">，已选 {{ checkedRowKeys.length }} 条</span>
      </div>

      <div class="audio-asset-management__table">
        <n-data-table
          :columns="columns"
          :data="rows"
          :loading="loading"
          :pagination="false"
          :checked-row-keys="checkedRowKeys"
          :row-key="(row: AdminAudioAssetItem) => row.id"
          :row-props="rowProps"
          size="small"
          :scroll-x="1260"
          :max-height="480"
          @update:checked-row-keys="handleCheckedRowKeysChange"
        />
        <div class="audio-asset-management__pagination">
          <n-pagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[10, 20, 50, 100]"
            show-size-picker
            show-quick-jumper
            :on-update:page="handlePageChange"
            :on-update:page-size="handlePageSizeChange"
          />
        </div>
      </div>
    </div>

    <n-modal v-model:show="detailModalVisible" preset="card" title="音频素材信息" :style="{ width: 'min(760px, 92vw)' }">
      <template v-if="selectedAsset">
        <div class="audio-asset-management__detail-head">
          <div>
            <h3>{{ selectedAsset.name }}</h3>
            <p>{{ selectedAsset.worldName || '全局素材' }}</p>
          </div>
          <n-tag :type="selectedAsset.safeToDelete ? 'success' : 'warning'">
            {{ selectedAsset.safeToDelete ? '可直接删除' : '删除时将解除引用' }}
          </n-tag>
        </div>
        <n-descriptions label-placement="top" :column="2" size="small" bordered>
          <n-descriptions-item label="上传者">{{ selectedAsset.creatorName || selectedAsset.createdBy }}</n-descriptions-item>
          <n-descriptions-item label="作用域">{{ selectedAsset.scope === 'common' ? '通用级' : '世界级' }}</n-descriptions-item>
          <n-descriptions-item label="文件大小">{{ formatFileSize(selectedAsset.size) }}</n-descriptions-item>
          <n-descriptions-item label="音频时长">{{ formatDuration(selectedAsset.duration) }}</n-descriptions-item>
          <n-descriptions-item label="最近访问">{{ formatAccessTime(selectedAsset.lastAccessedAt) }}</n-descriptions-item>
          <n-descriptions-item label="访问次数">{{ selectedAsset.accessCount ?? 0 }}</n-descriptions-item>
          <n-descriptions-item :span="2" label="引用状态">{{ resolveUsageSummaryText(selectedAsset) }}</n-descriptions-item>
          <n-descriptions-item :span="2" label="引用来源">
            <div class="audio-asset-management__reference-groups">
              <div>
                <strong>场景</strong>
                <div class="audio-asset-management__tags">
                  <n-tag v-for="name in selectedSceneNames" :key="name" size="small" type="info">{{ name }}</n-tag>
                  <span v-if="!selectedSceneNames.length">无</span>
                </div>
              </div>
              <div>
                <strong>当前播放状态</strong>
                <div class="audio-asset-management__tags">
                  <n-tag v-for="label in selectedPlaybackLabels" :key="label" size="small" type="warning">{{ label }}</n-tag>
                  <span v-if="!selectedPlaybackLabels.length">无</span>
                </div>
              </div>
            </div>
          </n-descriptions-item>
          <n-descriptions-item :span="2" label="备注">{{ selectedAsset.description || '暂无备注' }}</n-descriptions-item>
        </n-descriptions>
      </template>
      <n-empty v-else description="没有可查看的素材信息" />
    </n-modal>
  </n-modal>
</template>

<style scoped>
.audio-asset-management__body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.audio-asset-management__quota {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
}

.audio-asset-management__quota > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: var(--n-text-color-2);
  font-size: 13px;
}

.audio-asset-management__toolbar,
.audio-asset-management__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.audio-asset-management__stats {
  color: var(--n-text-color-3);
  font-size: 13px;
}

.audio-asset-management__table {
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  padding: 12px;
}

.audio-asset-management__pagination {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  margin-top: 12px;
  border-top: 1px solid var(--n-border-color);
}

.audio-asset-management__name-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audio-asset-management__name-button {
  appearance: none;
  border: 0;
  background: transparent;
  color: var(--n-text-color);
  padding: 0;
  text-align: left;
  font-weight: 600;
  cursor: pointer;
}

.audio-asset-management__name-button:hover {
  color: var(--n-primary-color);
}

.audio-asset-management__desc {
  margin: 0;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.audio-asset-management__actions-cell {
  display: flex;
  gap: 4px;
}

.audio-asset-management__detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}

.audio-asset-management__detail-head h3,
.audio-asset-management__detail-head p {
  margin: 0;
}

.audio-asset-management__detail-head p {
  color: var(--n-text-color-3);
}

.audio-asset-management__reference-groups {
  display: grid;
  gap: 8px;
}

.audio-asset-management__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}
</style>
