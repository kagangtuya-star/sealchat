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
  lastActiveAt: number;
  messageChars: number;
  messageCharsIc: number;
  messageCharsOoc: number;
}

const visibilityTags: Record<string, { label: string; type: 'success' | 'warning' | 'info' }> = {
  public: { label: '公开', type: 'success' },
  private: { label: '私有', type: 'warning' },
  unlisted: { label: '不公开', type: 'info' },
};

const user = useUserStore();
const message = useMessage();

const items = ref<AdminWorldStatusItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const loading = ref(false);
const keyword = ref('');
const sortState = ref<DataTableSortState | null>(null);
let requestSeq = 0;
let disposed = false;

const formatTime = (value: number | string) => {
  if (!value) {
    return '暂无';
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm');
};

const columns: DataTableColumns<AdminWorldStatusItem> = [
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
    sorter: (a, b) => a.memberCount - b.memberCount,
    render: (row) => formatStatusNumber(row.memberCount),
  },
  {
    title: '在线',
    key: 'onlineCount',
    width: 80,
    sorter: (a, b) => a.onlineCount - b.onlineCount,
    render: (row) => formatStatusNumber(row.onlineCount),
  },
  {
    title: '频道',
    key: 'channelCount',
    width: 80,
    sorter: (a, b) => a.channelCount - b.channelCount,
    render: (row) => formatStatusNumber(row.channelCount),
  },
  {
    title: '消息字数',
    key: 'messageChars',
    width: 150,
    sorter: (a, b) => a.messageChars - b.messageChars,
    render: (row) => h('div', { class: 'status-world__chars' }, [
      h('div', formatStatusNumber(row.messageChars)),
      h('div', { class: 'status-world__chars-split' }, `内 ${formatStatusNumber(row.messageCharsIc)} · 外 ${formatStatusNumber(row.messageCharsOoc)}`),
    ]),
  },
  {
    title: '最近活跃',
    key: 'lastActiveAt',
    width: 140,
    sorter: (a, b) => a.lastActiveAt - b.lastActiveAt,
    render: (row) => formatTime(row.lastActiveAt),
  },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 140,
    sorter: (a, b) => dayjs(a.createdAt).valueOf() - dayjs(b.createdAt).valueOf(),
    render: (row) => formatTime(row.createdAt),
  },
];

const displayItems = computed(() => {
  const state = sortState.value;
  if (!state?.order) {
    return items.value;
  }
  const column = columns.find((col) => 'key' in col && col.key === state.columnKey);
  const sorter = column && 'sorter' in column && typeof column.sorter === 'function' ? column.sorter : null;
  if (!sorter) {
    return items.value;
  }
  const sorted = [...items.value].sort(sorter as (a: AdminWorldStatusItem, b: AdminWorldStatusItem) => number);
  return state.order === 'descend' ? sorted.reverse() : sorted;
});

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
    const resp = await api.get('api/v1/admin/worlds/status', {
      headers: { Authorization: user.token },
      params: {
        page: page.value,
        pageSize: pageSize.value,
        keyword: keyword.value.trim() || undefined,
      },
    });
    if (requestId !== requestSeq) {
      return;
    }
    const data = resp.data as { items?: AdminWorldStatusItem[]; total?: number };
    items.value = data.items || [];
    total.value = data.total ?? items.value.length;
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
  sortState.value = Array.isArray(state) ? state[0] ?? null : state;
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
        <p>全部状态正常的世界（含私有世界），共 {{ total }} 个；在线人数统计当前停留该世界的用户（不含访客和观察者），每分钟刷新。</p>
        <p>频道数不含归档频道；消息字数和最近活跃统计未删除消息（含归档频道），最近活跃取消息发送时间；列排序仅作用于当前页。</p>
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
      :data="displayItems"
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
