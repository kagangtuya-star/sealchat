<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import dayjs from 'dayjs';
import { useDebounceFn } from '@vueuse/core';
import { NTag, useMessage, type DataTableColumns, type DataTableSortState } from 'naive-ui';
import { api } from '@/stores/_config';
import { useUserStore } from '@/stores/user';
import { formatStatusNumber } from '../status-history';

interface AdminWorldStatusItem {
  id: string;
  name: string;
  description: string;
  visibility: string;
  ownerId: string;
  ownerNickname: string;
  createdAt: string;
  memberCount: number;
  channelCount: number;
  onlineCount: number;
  messageCount: number;
  lastActiveAt: number;
  messageChars: number;
  messageCharsIc: number;
  messageCharsOoc: number;
}

interface AdminWorldStatusSummary {
  worldCount: number;
  memberCount: number;
  channelCount: number;
  messageCount: number;
  messageCountIc: number;
  messageCountOoc: number;
  messageChars: number;
  messageCharsIc: number;
  messageCharsOoc: number;
}

interface AdminWorldStatusResponse {
  items: AdminWorldStatusItem[];
  total: number;
  summary: AdminWorldStatusSummary;
  dataQuality: { messageChars: 'ready' | 'rebuilding' | 'incomplete' | 'failed' };
}

const visibilityTags: Record<string, { label: string; type: 'success' | 'warning' | 'info' }> = {
  public: { label: '公开', type: 'success' },
  private: { label: '私有', type: 'warning' },
  unlisted: { label: '不公开', type: 'info' },
};

const user = useUserStore();
const message = useMessage();
const emit = defineEmits<{
  summary: [summary: AdminWorldStatusSummary];
}>();

const items = ref<AdminWorldStatusItem[]>([]);
const total = ref(0);
const worldCount = ref(0);
const charQuality = ref<AdminWorldStatusResponse['dataQuality']['messageChars']>('ready');
const page = ref(1);
const pageSize = ref(20);
const loading = ref(false);
const keyword = ref('');
const sortBy = ref('createdAt');
const order = ref<'asc' | 'desc'>('asc');
let requestSeq = 0;
let disposed = false;

const formatTime = (value: number | string) => {
  if (!value) {
    return '暂无';
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm');
};

const baseColumns: DataTableColumns<AdminWorldStatusItem> = [
  {
    title: '世界',
    key: 'name',
    minWidth: 220,
    render: (row) => h('div', { class: 'status-world__name-cell' }, [
      h('div', { class: 'status-world__name' }, row.name || '未命名世界'),
      row.description ? h('div', { class: 'status-world__desc', title: row.description }, row.description) : null,
    ]),
  },
  {
    title: '可见性',
    key: 'visibility',
    width: 88,
    render: (row) => {
      const tag = visibilityTags[row.visibility] || { label: row.visibility || '未知', type: 'info' as const };
      return h(NTag, { size: 'small', type: tag.type, bordered: false }, { default: () => tag.label });
    },
  },
  {
    title: '创建者',
    key: 'ownerNickname',
    width: 120,
    ellipsis: { tooltip: true },
    render: (row) => row.ownerNickname || '—',
  },
  {
    title: '成员',
    key: 'memberCount',
    width: 80,
    sorter: true,
    render: (row) => formatStatusNumber(row.memberCount),
  },
  {
    title: '在线',
    key: 'onlineCount',
    width: 80,
    sorter: true,
    render: (row) => formatStatusNumber(row.onlineCount),
  },
  {
    title: '频道',
    key: 'channelCount',
    width: 80,
    sorter: true,
    render: (row) => formatStatusNumber(row.channelCount),
  },
  {
    title: '消息字数',
    key: 'messageChars',
    width: 150,
    sorter: true,
    render: (row) => h('div', { class: 'status-world__chars' }, [
      h('div', formatStatusNumber(row.messageChars)),
      h('div', { class: 'status-world__chars-split' }, `内 ${formatStatusNumber(row.messageCharsIc)} · 外 ${formatStatusNumber(row.messageCharsOoc)}`),
    ]),
  },
  {
    title: '最近活跃',
    key: 'lastActiveAt',
    width: 140,
    sorter: true,
    render: (row) => formatTime(row.lastActiveAt),
  },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 140,
    sorter: true,
    render: (row) => formatTime(row.createdAt),
  },
];

const columns = computed<DataTableColumns<AdminWorldStatusItem>>(() => baseColumns.map((column) => {
  if ('sorter' in column && column.sorter) {
    return { ...column, sortOrder: column.key === sortBy.value ? (order.value === 'asc' ? 'ascend' : 'descend') : false };
  }
  return column;
}));

const pagination = computed(() => ({
  page: page.value,
  pageSize: pageSize.value,
  itemCount: total.value,
  pageSizes: [20, 50, 100],
  showSizePicker: true,
}));

const refresh = async () => {
  if (disposed) {
    return;
  }
  const requestId = ++requestSeq;
  loading.value = true;
  try {
    const { data } = await api.get<AdminWorldStatusResponse>('api/v1/admin/status/worlds', {
      headers: { Authorization: user.token },
      params: {
        page: page.value,
        pageSize: pageSize.value,
        keyword: keyword.value.trim() || undefined,
        sortBy: sortBy.value,
        order: order.value,
      },
    });
    if (requestId !== requestSeq) {
      return;
    }
    items.value = data.items;
    total.value = data.total;
    worldCount.value = data.summary.worldCount;
    charQuality.value = data.dataQuality.messageChars;
    emit('summary', data.summary);
  } catch (err) {
    const status = (err as { response?: { status?: number } })?.response?.status;
    if (requestId === requestSeq && status !== 401 && status !== 403) {
      console.error(err);
      message.error('获取世界列表失败');
    }
  } finally {
    if (requestId === requestSeq) {
      loading.value = false;
    }
  }
};

const handlePageChange = (value: number) => {
  page.value = value;
  void refresh();
};

const handlePageSizeChange = (value: number) => {
  pageSize.value = value;
  page.value = 1;
  void refresh();
};

const handleSorterChange = (state: DataTableSortState | DataTableSortState[] | null) => {
  const sort = Array.isArray(state) ? state[0] : state;
  sortBy.value = sort?.order && typeof sort.columnKey === 'string' ? sort.columnKey : 'createdAt';
  order.value = sort?.order === 'descend' ? 'desc' : 'asc';
  page.value = 1;
  void refresh();
};

const searchByKeyword = useDebounceFn(() => {
  page.value = 1;
  void refresh();
}, 300);
watch(keyword, () => {
  void searchByKeyword();
});

onMounted(() => {
  void refresh();
});

onBeforeUnmount(() => {
  disposed = true;
  requestSeq += 1;
});

defineExpose({ refresh });
</script>

<template>
  <section class="status-world">
    <div class="status-world__header">
      <div>
        <h2>世界列表</h2>
        <p>状态正常的世界共 {{ worldCount }} 个<span v-if="total !== worldCount">，当前命中 {{ total }} 个</span>。在线人数按本次请求时停留在该世界的用户去重，不含访客和观察者。</p>
        <p>频道数不含归档频道；消息统计与最近活跃包含归档频道中的未删除消息，不含私聊和已删除频道。数据库统计最多缓存约 60 秒。</p>
        <p v-if="charQuality !== 'ready'">{{ charQuality === 'rebuilding' ? '消息字数正在回填，当前数字可能不完整。' : '消息字数尚未回填完整。' }}</p>
      </div>
      <n-input
        v-model:value="keyword"
        size="small"
        clearable
        placeholder="搜索世界名称、简介或创建者"
        class="status-world__search"
      />
    </div>
    <n-data-table
      :columns="columns"
      :data="items"
      :loading="loading"
      :pagination="pagination"
      remote
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
      @update:sorter="handleSorterChange"
      :row-key="(row: AdminWorldStatusItem) => row.id"
      :scroll-x="1100"
      size="small"
    />
  </section>
</template>

<style scoped lang="scss">
.status-world {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.status-world__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.status-world__header h2 {
  margin: 0;
  font-size: 1.02rem;
}

.status-world__header p {
  margin: 0.2rem 0 0;
  color: var(--sc-text-secondary);
  font-size: 0.82rem;
}

.status-world__search {
  width: 260px;
  max-width: 100%;
}

.status-world :deep(.status-world__name) {
  font-weight: 600;
}

.status-world :deep(.status-world__desc) {
  margin-top: 0.15rem;
  color: var(--sc-text-secondary);
  font-size: 0.75rem;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.status-world :deep(.status-world__chars-split) {
  color: var(--sc-text-secondary);
  font-size: 0.72rem;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 900px) {
  .status-world__header {
    flex-direction: column;
  }

  .status-world__search {
    width: 100%;
  }
}
</style>
