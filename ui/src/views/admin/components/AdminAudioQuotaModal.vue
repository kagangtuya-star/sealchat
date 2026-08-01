<script setup lang="ts">
import { api } from '@/stores/_config';
import { useUtilsStore } from '@/stores/utils';
import type { UserInfo } from '@/types';
import type { AdminAudioQuotaItem, AdminAudioQuotaListResult } from '@/types/audio';
import { Refresh, Search } from '@vicons/tabler';
import { NTag, useMessage, type DataTableColumns } from 'naive-ui';
import { computed, h, ref, watch } from 'vue';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
}>();

const utils = useUtilsStore();
const message = useMessage();

const listLoading = ref(false);
const rows = ref<AdminAudioQuotaItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(10);
const keyword = ref('');
const selectedUserId = ref<string | null>(null);
const detailLoading = ref(false);
const detail = ref<AdminAudioQuotaItem | null>(null);
const saving = ref(false);
const deleting = ref(false);
const quotaInput = ref<number | null>(null);
const userSearchLoading = ref(false);
const userSearchValue = ref<string | null>(null);
const userOptions = ref<Array<{ label: string; value: string }>>([]);

let listSearchTimer: ReturnType<typeof setTimeout> | null = null;
let userSearchTimer: ReturnType<typeof setTimeout> | null = null;

const sourceLabelMap: Record<AdminAudioQuotaItem['source'], string> = {
  default: '默认配额',
  override: '单独配额',
  'admin-unlimited': '管理员无上限',
};

const selectedSourceLabel = computed(() => (detail.value ? sourceLabelMap[detail.value.source] : '-'));
const selectedPrimaryText = computed(() => {
  if (!detail.value) return '未选择用户';
  if (!detail.value.limited) {
    return `已用 ${formatFileSize(detail.value.usedBytes)} / 无上限`;
  }
  return `已用 ${formatFileSize(detail.value.usedBytes)} / ${formatFileSize(detail.value.quotaBytes ?? 0)}`;
});
const selectedSecondaryText = computed(() => {
  if (!detail.value) return '左侧选择已设覆盖用户，或右上搜索任意用户';
  if (!detail.value.limited) {
    return '平台管理员当前默认无上限；设置覆盖值后将按覆盖值生效';
  }
  if ((detail.value.quotaBytes ?? 0) < detail.value.usedBytes) {
    return `已超出 ${formatFileSize(detail.value.usedBytes - (detail.value.quotaBytes ?? 0))}，新增上传将被阻止`;
  }
  return `剩余 ${formatFileSize(detail.value.remainingBytes ?? 0)} · ${(detail.value.usagePercent ?? 0).toFixed(1)}%`;
});
const selectedQuotaPercent = computed(() => {
  if (!detail.value) return 0;
  if (!detail.value.limited) return 100;
  return Math.max(0, Math.min(100, detail.value.usagePercent ?? 0));
});
const selectedQuotaOverflow = computed(() => {
  if (!detail.value?.limited) return false;
  return detail.value.usedBytes > (detail.value.quotaBytes ?? 0);
});

const columns = computed<DataTableColumns<AdminAudioQuotaItem>>(() => [
  {
    title: '用户',
    key: 'username',
    minWidth: 180,
    render: (row) =>
      h('div', { class: 'audio-quota-modal__user-cell' }, [
        h('strong', row.nickname || row.username || row.userId),
        h('span', row.username || row.userId),
      ]),
  },
  {
    title: '覆盖值',
    key: 'quotaMB',
    width: 100,
    render: (row) => `${row.quotaMB} MB`,
  },
  {
    title: '已用',
    key: 'usedBytes',
    width: 110,
    render: (row) => formatFileSize(row.usedBytes),
  },
  {
    title: '状态',
    key: 'source',
    width: 118,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.limited ? 'info' : 'default' },
        { default: () => sourceLabelMap[row.source] },
      ),
  },
]);

watch(
  () => props.show,
  (show) => {
    if (!show) return;
    void refreshList();
    if (selectedUserId.value) {
      void loadDetail(selectedUserId.value);
    }
  },
);

function closeModal() {
  emit('update:show', false);
}

function extractErrorMessage(error: any, fallback: string) {
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || fallback;
}

function formatFileSize(value?: number | null) {
  const size = Math.max(0, value ?? 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function formatQuotaBytes(row: AdminAudioQuotaItem | null) {
  if (!row) return '-';
  if (!row.limited) return '无上限';
  return formatFileSize(row.quotaBytes ?? 0);
}

function buildUserOption(user: UserInfo) {
  const nick = user.nick?.trim() || '';
  const username = user.username?.trim() || user.id;
  const primary = nick || username || user.id;
  return {
    value: user.id,
    label: `${primary} · ${username} · ${user.id}`,
  };
}

async function refreshList() {
  listLoading.value = true;
  try {
    const resp = await api.get<AdminAudioQuotaListResult>('/api/v1/admin/audio-quotas', {
      params: {
        page: page.value,
        pageSize: pageSize.value,
        query: keyword.value.trim() || undefined,
      },
    });
    rows.value = resp.data.items || [];
    total.value = Number(resp.data.total || 0);
    if (selectedUserId.value && rows.value.some((item) => item.userId === selectedUserId.value)) {
      return;
    }
    if (!selectedUserId.value && rows.value.length) {
      selectedUserId.value = rows.value[0].userId;
      void loadDetail(rows.value[0].userId);
    }
  } catch (error) {
    message.error(extractErrorMessage(error, '读取覆盖列表失败'));
  } finally {
    listLoading.value = false;
  }
}

async function loadDetail(userId: string) {
  if (!userId) return;
  selectedUserId.value = userId;
  userSearchValue.value = userId;
  detailLoading.value = true;
  try {
    const resp = await api.get<AdminAudioQuotaItem>(`/api/v1/admin/audio-quotas/${encodeURIComponent(userId)}`);
    detail.value = resp.data;
    quotaInput.value = detail.value.hasOverride ? detail.value.quotaMB : null;
  } catch (error) {
    detail.value = null;
    quotaInput.value = null;
    message.error(extractErrorMessage(error, '读取用户配额失败'));
  } finally {
    detailLoading.value = false;
  }
}

function handleSearchInput() {
  if (listSearchTimer) clearTimeout(listSearchTimer);
  listSearchTimer = setTimeout(() => {
    page.value = 1;
    void refreshList();
  }, 250);
}

function handlePageChange(nextPage: number) {
  page.value = nextPage;
  void refreshList();
}

function handlePageSizeChange(nextPageSize: number) {
  pageSize.value = nextPageSize;
  page.value = 1;
  void refreshList();
}

function rowProps(row: AdminAudioQuotaItem) {
  return {
    style: 'cursor: var(--sc-cursor-pointer, pointer);',
    onClick: () => {
      void loadDetail(row.userId);
    },
  };
}

async function searchUsers(keywordText: string) {
  const query = keywordText.trim();
  if (!query) {
    userOptions.value = [];
    return;
  }
  userSearchLoading.value = true;
  try {
    const resp = await utils.adminUserList({
      page: 1,
      pageSize: 20,
      keyword: query,
    });
    userOptions.value = (resp.data.items || []).map(buildUserOption);
  } catch (error) {
    message.error(extractErrorMessage(error, '搜索用户失败'));
  } finally {
    userSearchLoading.value = false;
  }
}

function handleUserSearch(keywordText: string) {
  if (userSearchTimer) clearTimeout(userSearchTimer);
  userSearchTimer = setTimeout(() => {
    void searchUsers(keywordText);
  }, 250);
}

function handleUserSelect(userId: string | null) {
  if (!userId) return;
  void loadDetail(userId);
}

async function saveOverride() {
  if (!selectedUserId.value) {
    message.warning('请先选择用户');
    return;
  }
  const quotaMB = Math.trunc(quotaInput.value ?? 0);
  if (quotaMB <= 0) {
    message.warning('请输入大于 0 的配额值');
    return;
  }
  saving.value = true;
  try {
    const resp = await api.put<AdminAudioQuotaItem>(`/api/v1/admin/audio-quotas/${encodeURIComponent(selectedUserId.value)}`, {
      quotaMB,
    });
    detail.value = resp.data;
    quotaInput.value = resp.data.quotaMB;
    message.success('用户音频配额已保存');
    await refreshList();
  } catch (error) {
    message.error(extractErrorMessage(error, '保存用户配额失败'));
  } finally {
    saving.value = false;
  }
}

async function clearOverride() {
  if (!selectedUserId.value) {
    message.warning('请先选择用户');
    return;
  }
  deleting.value = true;
  try {
    await api.delete(`/api/v1/admin/audio-quotas/${encodeURIComponent(selectedUserId.value)}`);
    message.success('覆盖值已删除');
    await loadDetail(selectedUserId.value);
    await refreshList();
  } catch (error) {
    message.error(extractErrorMessage(error, '删除覆盖值失败'));
  } finally {
    deleting.value = false;
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="用户音频配额"
    class="audio-quota-modal sc-fluid-modal sc-fluid-modal--xwide"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <div class="audio-quota-modal__toolbar">
      <div class="audio-quota-modal__search">
        <n-input
          v-model:value="keyword"
          clearable
          placeholder="搜索已设置覆盖值的用户"
          @input="handleSearchInput"
          @clear="handleSearchInput"
        >
          <template #prefix>
            <n-icon :component="Search" />
          </template>
        </n-input>
        <n-button @click="refreshList" :loading="listLoading">
          <template #icon>
            <n-icon :component="Refresh" />
          </template>
          刷新
        </n-button>
      </div>

      <n-select
        v-model:value="userSearchValue"
        clearable
        filterable
        remote
        placeholder="搜索任意用户并载入详情"
        :options="userOptions"
        :loading="userSearchLoading"
        @search="handleUserSearch"
        @update:value="handleUserSelect"
      />
    </div>

    <div class="audio-quota-modal__layout">
      <section class="audio-quota-modal__list-card">
        <div class="audio-quota-modal__list-head">
          <div>
            <strong>覆盖列表</strong>
            <p>仅展示已设置单独配额的用户</p>
          </div>
          <span>共 {{ total }} 条</span>
        </div>
        <div class="audio-quota-modal__table-scroll sc-modal-table-scroll">
          <n-data-table
            :columns="columns"
            :data="rows"
            :loading="listLoading"
            :pagination="false"
            :row-key="(row: AdminAudioQuotaItem) => row.userId"
            :row-props="rowProps"
            :max-height="420"
            :scroll-x="560"
            size="small"
          />
        </div>
        <div class="audio-quota-modal__pagination">
          <n-pagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[10, 20, 50]"
            show-size-picker
            :on-update:page="handlePageChange"
            :on-update:page-size="handlePageSizeChange"
          />
        </div>
      </section>

      <section class="audio-quota-modal__detail-card">
        <div class="audio-quota-modal__detail-head">
          <div>
            <strong>{{ detail?.nickname || detail?.username || detail?.userId || '未选择用户' }}</strong>
            <p>{{ detail?.username || '请从左侧列表或顶部搜索中选择用户' }}</p>
          </div>
          <n-tag size="small" :type="detail?.limited ? 'info' : 'default'">
            {{ selectedSourceLabel }}
          </n-tag>
        </div>

        <div v-if="detail" class="audio-quota-modal__summary">
          <div class="audio-quota-modal__summary-copy">
            <strong>{{ selectedPrimaryText }}</strong>
            <span>{{ selectedSecondaryText }}</span>
          </div>
          <div class="audio-quota-modal__summary-progress" :class="{ 'is-unlimited': !detail.limited, 'is-overflow': selectedQuotaOverflow }">
            <div class="audio-quota-modal__summary-progress-fill" :style="{ width: `${selectedQuotaPercent}%` }"></div>
          </div>
        </div>

        <n-skeleton v-if="detailLoading" text :repeat="6" />
        <template v-else-if="detail">
          <n-descriptions label-placement="top" :column="2" size="small" bordered>
            <n-descriptions-item label="用户 ID">
              {{ detail.userId }}
            </n-descriptions-item>
            <n-descriptions-item label="当前策略">
              {{ selectedSourceLabel }}
            </n-descriptions-item>
            <n-descriptions-item label="已用容量">
              {{ formatFileSize(detail.usedBytes) }}
            </n-descriptions-item>
            <n-descriptions-item label="生效上限">
              {{ formatQuotaBytes(detail) }}
            </n-descriptions-item>
            <n-descriptions-item label="当前覆盖值">
              {{ detail.hasOverride ? `${detail.quotaMB} MB` : '未设置' }}
            </n-descriptions-item>
            <n-descriptions-item label="最后修改人">
              {{ detail.updatedBy || '-' }}
            </n-descriptions-item>
          </n-descriptions>

          <div class="audio-quota-modal__editor">
            <n-form-item label="设置单独配额 (MB)" feedback="删除覆盖值后，普通用户回退默认配额，平台管理员回退无上限">
              <n-input-number v-model:value="quotaInput" :min="1" :precision="0" placeholder="输入大于 0 的整数" />
            </n-form-item>

            <div class="audio-quota-modal__editor-actions">
              <n-button type="primary" :loading="saving" @click="saveOverride">保存覆盖值</n-button>
              <n-button :disabled="!detail.hasOverride" :loading="deleting" @click="clearOverride">删除覆盖值</n-button>
            </div>
          </div>
        </template>
        <n-empty v-else class="audio-quota-modal__empty" description="左侧选择已设覆盖用户，或搜索任意用户开始设置" />
      </section>
    </div>

    <template #footer>
      <div class="audio-quota-modal__footer">
        <n-button @click="closeModal">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.audio-quota-modal__toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 360px);
  gap: 12px;
  margin-bottom: 12px;
}

.audio-quota-modal__search {
  display: flex;
  gap: 8px;
}

.audio-quota-modal__layout {
  display: flex;
  gap: 12px;
  align-items: stretch;
  flex-wrap: wrap;
}

.audio-quota-modal__layout > * {
  flex: 1 1 420px;
  min-width: 0;
}

.audio-quota-modal__list-card {
  min-width: 360px;
}

.audio-quota-modal__detail-card {
  min-width: 420px;
}

.audio-quota-modal__list-card,
.audio-quota-modal__detail-card {
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  padding: 12px;
  background: var(--n-card-color);
  min-height: 0;
  overflow: hidden;
}

.audio-quota-modal__list-head,
.audio-quota-modal__detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}

.audio-quota-modal__list-head p,
.audio-quota-modal__detail-head p {
  margin: 4px 0 0;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.audio-quota-modal__user-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audio-quota-modal__user-cell span {
  color: var(--n-text-color-3);
  font-size: 12px;
}

.audio-quota-modal__pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.audio-quota-modal__table-scroll {
  min-width: 0;
}

.audio-quota-modal__table-scroll :deep(.n-data-table-wrapper) {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.35) transparent;
}

.audio-quota-modal__table-scroll :deep(.n-data-table-wrapper)::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.audio-quota-modal__table-scroll :deep(.n-data-table-wrapper)::-webkit-scrollbar-track {
  background: transparent;
}

.audio-quota-modal__table-scroll :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.35);
  border-radius: 999px;
}

.audio-quota-modal__table-scroll :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb:hover {
  background: rgba(128, 128, 128, 0.55);
}

.audio-quota-modal__summary {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  margin-bottom: 12px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--n-primary-color) 6%, transparent);
}

.audio-quota-modal__summary-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audio-quota-modal__summary-copy span {
  color: var(--n-text-color-3);
  font-size: 12px;
}

.audio-quota-modal__summary-progress {
  height: 10px;
  border-radius: 999px;
  overflow: hidden;
  background: color-mix(in srgb, var(--n-border-color) 50%, transparent);
}

.audio-quota-modal__summary-progress-fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #0f766e 0%, #22c55e 100%);
}

.audio-quota-modal__summary-progress.is-overflow .audio-quota-modal__summary-progress-fill {
  background: linear-gradient(90deg, #dc2626 0%, #f97316 100%);
}

.audio-quota-modal__summary-progress.is-unlimited .audio-quota-modal__summary-progress-fill {
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.22) 0%, rgba(59, 130, 246, 0.38) 100%);
}

.audio-quota-modal__editor {
  margin-top: 14px;
}

.audio-quota-modal__editor-actions,
.audio-quota-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.audio-quota-modal__empty {
  min-height: 320px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.audio-quota-modal__empty :deep(.n-empty) {
  width: 100%;
}

.audio-quota-modal__empty :deep(.n-empty-description) {
  white-space: normal;
  word-break: break-word;
  text-align: center;
}

@media (max-width: 900px) {
  .audio-quota-modal__toolbar {
    grid-template-columns: 1fr;
  }

  .audio-quota-modal__search {
    flex-direction: column;
  }

  .audio-quota-modal__layout > * {
    flex-basis: 100%;
    min-width: 0;
  }

  .audio-quota-modal__pagination {
    justify-content: flex-start;
    overflow-x: auto;
  }
}
</style>
