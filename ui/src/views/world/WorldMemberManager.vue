<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useDialog, useMessage, NButton, NDropdown, NTag, NSpace } from 'naive-ui';
import dayjs from 'dayjs';

const props = defineProps<{
  worldId: string;
  visible: boolean;
}>();

const visible = defineModel<boolean>('visible', { default: false });
const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();

interface MemberRow {
  id: string;
  worldId: string;
  userId: string;
  role: string;
  joinedAt: string;
  username: string;
  nickname: string;
}

const loading = ref(false);
const keyword = ref('');
const pagination = ref({ page: 1, pageSize: 20, total: 0 });
const rows = ref<MemberRow[]>([]);

const roleLabel = (role: string) => {
  switch (role) {
    case 'owner':
      return '拥有者';
    case 'admin':
      return '管理员';
    case 'spectator':
      return '旁观者';
    default:
      return '成员';
  }
};

const roleChangeOptions = (row: MemberRow) => [
  { label: '设为管理员', key: 'admin', disabled: row.role === 'admin' },
  { label: '设为成员', key: 'member', disabled: row.role === 'member' },
  { label: '设为旁观者', key: 'spectator', disabled: row.role === 'spectator' },
];

const columns = computed(() => [
  {
    title: '用户',
    key: 'user',
    minWidth: 180,
    render(row: MemberRow) {
      return h('div', { class: 'world-member-manager__user' }, [
        h('div', { class: 'world-member-manager__nick' }, row.nickname || row.username || '（未设置昵称）'),
        h('div', { class: 'world-member-manager__id' }, row.userId),
      ]);
    },
  },
  {
    title: '身份',
    key: 'role',
    width: 96,
    render(row: MemberRow) {
      const type = row.role === 'owner' ? 'warning' : row.role === 'admin' ? 'info' : 'default';
      return h(NTag, { type, size: 'small', bordered: false }, { default: () => roleLabel(row.role) });
    },
  },
  {
    title: '加入时间',
    key: 'joinedAt',
    width: 150,
    render(row: MemberRow) {
      return h('span', { class: 'world-member-manager__joined' }, dayjs(row.joinedAt).format('YYYY-MM-DD HH:mm'));
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 168,
    render(row: MemberRow) {
      const disabled = row.role === 'owner';
      return h(
        NSpace,
        { size: 'small', wrap: false },
        () => [
          h(
            NDropdown,
            {
              trigger: 'click',
              disabled,
              options: roleChangeOptions(row),
              onSelect: (key: string) => handleRoleChange(row, key),
            },
            {
              default: () => h(
                NButton,
                { size: 'tiny', secondary: true, disabled },
                { default: () => '更改身份' },
              ),
            },
          ),
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              type: 'error',
              disabled,
              onClick: () => handleRemove(row),
            },
            { default: () => '移除' },
          ),
        ],
      );
    },
  },
]);

const loadMembers = async () => {
  if (!props.worldId || !visible.value) return;
  loading.value = true;
  try {
    const resp = await chat.worldMemberList(props.worldId, {
      page: pagination.value.page,
      pageSize: pagination.value.pageSize,
      keyword: keyword.value.trim(),
    });
    rows.value = resp.items || [];
    pagination.value.total = resp.total || 0;
  } catch (err: any) {
    message.error(err?.response?.data?.message || '加载成员失败');
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  pagination.value.page = 1;
  loadMembers();
};

const handleRoleChange = async (row: MemberRow, role: string) => {
  if (!props.worldId || row.role === role) return;
  try {
    await chat.worldMemberSetRole(props.worldId, row.userId, role);
    message.success('角色已更新');
    await loadMembers();
  } catch (err: any) {
    message.error(err?.response?.data?.message || '角色更新失败');
  }
};

const handleRemove = (row: MemberRow) => {
  if (!props.worldId) return;
  dialog.warning({
    title: '移除成员',
    content: `确定要移除「${row.nickname || row.userId}」吗？`,
    positiveText: '确认移除',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: async () => {
      try {
        await chat.worldMemberRemove(props.worldId, row.userId);
        message.success('已移除成员');
        await loadMembers();
      } catch (err: any) {
        message.error(err?.response?.data?.message || '移除失败');
      }
    },
  });
};

const handlePageChange = (page: number) => {
  pagination.value.page = page;
  loadMembers();
};

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.pageSize = pageSize;
  pagination.value.page = 1;
  loadMembers();
};

watch(
  () => visible.value,
  (val) => {
    if (val) {
      pagination.value.page = 1;
      loadMembers();
    }
  },
);

watch(
  () => props.worldId,
  () => {
    if (visible.value) {
      pagination.value.page = 1;
      loadMembers();
    }
  },
);

onMounted(() => {
  if (visible.value) {
    loadMembers();
  }
});
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    title="成员管理"
    class="sc-fluid-modal sc-manage-modal"
    :bordered="false"
    @update:show="(val: boolean) => visible = val"
  >
    <div class="sc-manage-modal__body">
      <div class="sc-manage-toolbar">
        <n-input
          v-model:value="keyword"
          placeholder="搜索用户ID / 昵称"
          clearable
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        />
        <n-button secondary @click="handleSearch">搜索</n-button>
        <span class="sc-manage-toolbar__meta">共 {{ pagination.total }} 人</span>
      </div>
      <div class="sc-modal-table-scroll">
        <n-data-table
          :columns="columns"
          :data="rows"
          :loading="loading"
          :bordered="false"
          :pagination="false"
          :row-key="(row: MemberRow) => row.id"
          :scroll-x="620"
          size="small"
        />
      </div>
    </div>
    <template #footer>
      <div class="sc-manage-actions">
        <n-pagination
          :page="pagination.page"
          :page-size="pagination.pageSize"
          :item-count="pagination.total"
          show-size-picker
          :page-sizes="[10, 20, 50]"
          size="small"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </div>
    </template>
  </n-modal>
</template>

<!-- Unscoped: these cells are created with h() inside n-data-table, where scoped attrs do not apply. -->
<style>
.world-member-manager__user {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.world-member-manager__nick {
  font-weight: 600;
  color: var(--sc-text-primary);
}
.world-member-manager__id {
  font-size: 12px;
  font-family: var(--sc-code-font);
  color: var(--sc-text-secondary);
}
.world-member-manager__joined {
  white-space: nowrap;
  color: var(--sc-text-secondary);
}
</style>
