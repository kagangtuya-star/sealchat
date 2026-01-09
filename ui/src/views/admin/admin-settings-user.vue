<script setup lang="tsx">
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import type { ServerConfig, UserInfo } from '@/types';
import { Refresh, Search, UserPlus } from '@vicons/tabler';
import { cloneDeep } from 'lodash-es';
import { NIcon, useDialog, useMessage } from 'naive-ui';
import { computed, nextTick } from 'vue';
import { onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import UserCreateModal from './UserCreateModal.vue';

const emit = defineEmits(['close']);

const close = () => {
  emit('close');
}

const chat = useChatStore();
const utils = useUtilsStore();
const message = useMessage()

const { t } = useI18n()

// 分页和搜索状态
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const keyword = ref('');
const userType = ref(''); // '', 'user', 'bot'
const loading = ref(false);
const data = ref<UserInfo[]>([]);

// 用户类型选项
const typeOptions = [
  { label: '全部', value: '' },
  { label: '普通用户', value: 'user' },
  { label: 'BOT', value: 'bot' }
];

onMounted(async () => {
  refresh()
})

const refresh = async () => {
  loading.value = true;
  try {
    const resp = await utils.adminUserList({
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value,
      type: userType.value
    });
    data.value = resp.data.items || [];
    total.value = resp.data.total || 0;
  } finally {
    loading.value = false;
  }
}

// 搜索处理（防抖）
let searchTimer: any = null;
const handleSearch = () => {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    page.value = 1;
    refresh();
  }, 300);
}

// 类型筛选变化
const handleTypeChange = () => {
  page.value = 1;
  refresh();
}

// 分页变化
const handlePageChange = (newPage: number) => {
  page.value = newPage;
  refresh();
}

const handlePageSizeChange = (newPageSize: number) => {
  pageSize.value = newPageSize;
  page.value = 1;
  refresh();
}

const dialog = useDialog()

// User create modal
const showUserCreateModal = ref(false);

const handleUserCreated = () => {
  refresh();
};

const tryUserResetPassword = (i: UserInfo) => {
  dialog.warning({
    title: t('dialogLogOut.title'),
    content: '重置此用户密码为123456吗？',
    positiveText: t('dialogLogOut.positiveText'),
    negativeText: t('dialogLogOut.negativeText'),
    onPositiveClick: async () => {
      try {
        await utils.userResetPassword(i.id);
        message.success('重置成功');
      } catch (error) {
        message.error('重置失败: ' + (error as any).response?.data?.message || '未知错误');
      }
    },
    onNegativeClick: () => {
    }
  })
}

const tryUserDisable = (i: UserInfo) => {
  dialog.warning({
    title: t('dialogLogOut.title'),
    content: '确定要禁用此帐号吗？',
    positiveText: t('dialogLogOut.positiveText'),
    negativeText: t('dialogLogOut.negativeText'),
    onPositiveClick: async () => {
      try {
        await utils.userDisable(i.id);
        message.success('停用成功');
        refresh();
      } catch (error) {
        message.error('停用失败: ' + (error as any).response?.data?.message || '未知错误');
      }
    },
    onNegativeClick: () => {
    }
  })
}

const tryUserEnable = (i: UserInfo) => {
  dialog.warning({
    title: t('dialogLogOut.title'),
    content: '确定要启用此帐号吗？',
    positiveText: t('dialogLogOut.positiveText'),
    negativeText: t('dialogLogOut.negativeText'),
    onPositiveClick: async () => {
      try {
        await utils.userEnable(i.id);
        message.success('启用成功');
        refresh();
      } catch (error) {
        message.error('启用失败: ' + (error as any).response?.data?.message || '未知错误');
      }
    },
    onNegativeClick: () => {
    }
  })
}

const handleRoleChange = async (userId: string, roleLst: string[], oldRoleLst: string[]) => {
  // 计算需要移除和添加的成员
  const toRemove = oldRoleLst.filter(id => !roleLst.includes(id));
  const toAdd = roleLst.filter(id => !oldRoleLst.includes(id));

  try {
    if (toAdd.length) await utils.userRoleLinkByUserId(userId, toAdd);
    if (toRemove.length) await utils.userRoleUnlinkByUserId(userId, toRemove);
    refresh();
    message.success('角色已成功修改');
  } catch (error) {
    console.error('修改角色失败:', error);
    message.error('修改角色失败，请重试');
  }
};

// 格式化时间
const formatTime = (timeStr: string) => {
  if (!timeStr) return '-';
  const date = new Date(timeStr);
  return date.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  });
};

const columns = ref([
  {
    title: '用户名',
    key: 'username',
    width: 120
  },
  {
    title: '昵称',
    key: 'nick',
    width: 120,
    ellipsis: true
  },
  {
    title: '类型',
    key: 'is_bot',
    width: 80,
    render: (row: UserInfo) => {
      return row.is_bot ? (
        <n-tag type="info" size="small">BOT</n-tag>
      ) : (
        <n-tag type="default" size="small">用户</n-tag>
      );
    }
  },
  {
    title: '角色',
    key: 'role',
    width: 180,
    render: (row: UserInfo) => {
      return (
        <n-select
          v-model:value={row.roleIds}
          multiple
          options={[
            { label: '管理员', value: 'sys-admin' },
            { label: '普通用户', value: 'sys-user' },
          ]}
          size="small"
          on-update:value={(value: any) => handleRoleChange(row.id, value, row.roleIds ?? [])}
        />
      )
    },
  },
  {
    title: '状态',
    key: 'disabled',
    width: 80,
    render: (row: UserInfo) => {
      return row.disabled ? (
        <n-tag type="error" size="small">已禁用</n-tag>
      ) : (
        <n-tag type="success" size="small">正常</n-tag>
      );
    }
  },
  {
    title: '注册时间',
    key: 'createdAt',
    width: 100,
    render: (row: any) => formatTime(row.createdAt)
  },
  {
    title: '操作',
    width: 160,
    render: (row: UserInfo) => {
      const isDisabled = row.disabled;
      return <div class="flex space-x-2">
        <n-button type="warning" size="small" onClick={() => tryUserResetPassword(row)}>重置密码</n-button>
        {!isDisabled ? <n-button type="error" size="small" onClick={() => tryUserDisable(row)}>停用</n-button> :
          <n-button type="success" size="small" onClick={() => tryUserEnable(row)}>启用</n-button>}
      </div>
    }
  }
]);
</script>

<template>
  <div class="user-management">
    <!-- 搜索和筛选栏 -->
    <div class="user-management__toolbar">
      <div class="user-management__search">
        <n-input
          v-model:value="keyword"
          placeholder="搜索用户名/昵称"
          clearable
          @input="handleSearch"
          @clear="handleSearch"
          style="width: 200px"
        >
          <template #prefix>
            <n-icon :component="Search" />
          </template>
        </n-input>
        
        <n-select
          v-model:value="userType"
          :options="typeOptions"
          placeholder="用户类型"
          style="width: 120px"
          @update:value="handleTypeChange"
        />
        
        <n-button @click="refresh" :loading="loading">
          <template #icon>
            <n-icon :component="Refresh" />
          </template>
          刷新
        </n-button>

        <n-button type="primary" @click="showUserCreateModal = true">
          <template #icon>
            <n-icon :component="UserPlus" />
          </template>
          新增用户
        </n-button>
      </div>
      
      <div class="user-management__stats">
        共 <n-text type="primary">{{ total }}</n-text> 位用户
      </div>
    </div>
    
    <div class="user-management__table">
      <n-data-table 
        :columns="columns" 
        :data="data" 
        :loading="loading"
        :pagination="false" 
        :bordered="false"
        :max-height="400"
        :scroll-x="900"
        size="small"
      />
    </div>
    
    <!-- 分页控件 -->
    <div class="user-management__pagination">
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
    
    <!-- 关闭按钮 -->
    <div class="user-management__footer">
      <n-button @click="close">关闭</n-button>
    </div>

    <!-- 新增用户模态框 -->
    <UserCreateModal
      v-model:show="showUserCreateModal"
      @success="handleUserCreated"
    />
  </div>
</template>

<style scoped>
.user-management {
  display: flex;
  flex-direction: column;
  max-height: 65vh;
  overflow: hidden;
}

.user-management__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  flex-wrap: wrap;
  gap: 8px;
}

.user-management__search {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.user-management__stats {
  font-size: 14px;
  color: var(--n-text-color-3);
}

.user-management__table {
  flex: 1;
  min-height: 0;
  margin-bottom: 12px;
}

/* 表格滚动条容器 */
.user-management__table :deep(.n-data-table-wrapper) {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.3) transparent;
}

.user-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.user-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-track {
  background: transparent;
}

.user-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.3);
  border-radius: 3px;
}

.user-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb:hover {
  background: rgba(128, 128, 128, 0.5);
}

/* 表格基础滚动条 */
.user-management__table :deep(.n-scrollbar-rail) {
  --n-scrollbar-width: 6px;
  --n-scrollbar-height: 6px;
}

.user-management__pagination {
  display: flex;
  justify-content: flex-end;
  padding: 8px 0;
  flex-shrink: 0;
  background: var(--n-color);
  position: relative;
  z-index: 1;
}

/* 移动端分页适配 */
@media (max-width: 640px) {
  .user-management__pagination {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }
  
  .user-management__pagination :deep(.n-pagination) {
    flex-wrap: wrap;
    justify-content: center;
    gap: 4px;
  }
  
  .user-management__pagination :deep(.n-pagination-prefix) {
    width: 100%;
    text-align: center;
    margin-bottom: 4px;
  }
}

.user-management__footer {
  display: flex;
  justify-content: flex-end;
  padding-top: 8px;
  border-top: 1px solid var(--n-border-color);
  flex-shrink: 0;
  background: var(--n-color);
  position: relative;
  z-index: 1;
}

/* 夜间模式适配 */
:global(.dark) .user-management__stats {
  color: rgba(255, 255, 255, 0.6);
}

:global(.dark) .user-management__table :deep(.n-data-table-wrapper) {
  scrollbar-color: rgba(255, 255, 255, 0.2) transparent;
}

:global(.dark) .user-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
}

:global(.dark) .user-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.35);
}
</style>
