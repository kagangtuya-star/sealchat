<script setup lang="tsx">
import Avatar from '@/components/avatar.vue';
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import type { AdminUserDetailStats, ServerConfig, UserInfo } from '@/types';
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
const userType = ref('user'); // '', 'user', 'bot'
const userStatus = ref('');
const loading = ref(false);
const data = ref<UserInfo[]>([]);

// 用户类型选项
const typeOptions = [
  { label: '全部', value: '' },
  { label: '普通用户', value: 'user' },
  { label: 'BOT', value: 'bot' }
];

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '正常', value: 'active' },
  { label: '已停用', value: 'disabled' }
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
      type: userType.value,
      status: userStatus.value,
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

const handleStatusChange = () => {
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

const tryUserDelete = (i: UserInfo) => {
  dialog.error({
    title: '删除用户',
    content: '删除后将隐藏该用户、移除其世界与频道关系、删除其上传的音频素材并自动解除相关引用。此操作不会物理清空历史消息等数据。确定继续吗？',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await utils.userDelete(i.id);
        message.success('删除成功');
        refresh();
      } catch (error) {
        const respError = (error as any)?.response?.data;
        message.error((respError?.error || respError?.message || '删除失败'));
      }
    },
  })
}

const showUserDetailModal = ref(false);
const userDetail = ref<UserInfo | null>(null);
const userDetailStats = ref<AdminUserDetailStats | null>(null);
const userDetailLoading = ref(false);
let userDetailRequestId = 0;

const formatDateTime = (timeStr?: string | null) => {
  if (!timeStr) return '-';
  const date = new Date(timeStr);
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN');
};

const formatRoleNames = (roleIds?: string[]) => {
  const roleNames: Record<string, string> = {
    'sys-admin': '管理员',
    'sys-user': '普通用户',
  };
  return roleIds?.map(roleId => roleNames[roleId] || roleId).join('、') || '-';
};

const openUserDetail = async (row: UserInfo) => {
  const requestId = ++userDetailRequestId;
  userDetail.value = row;
  userDetailStats.value = null;
  showUserDetailModal.value = true;
  userDetailLoading.value = true;
  try {
    const resp = await utils.adminUserDetail(row.id);
    if (requestId !== userDetailRequestId) return;
    userDetail.value = resp.data.user || row;
    userDetailStats.value = resp.data.stats;
  } catch {
    if (requestId === userDetailRequestId) {
      message.error('获取用户详情失败');
    }
  } finally {
    if (requestId === userDetailRequestId) {
      userDetailLoading.value = false;
    }
  }
};

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
    const respError = (error as any)?.response?.data;
    const errorMessage = respError?.error || respError?.message || '修改角色失败，请重试';
    message.error(errorMessage);
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
    title: '邮箱',
    key: 'email',
    width: 180,
    ellipsis: true,
    render: (row: UserInfo) => {
      if (!row.email) {
        return <span class="text-gray-400">-</span>;
      }
      return (
        <div class="flex items-center gap-1">
          <span class="truncate">{row.email}</span>
          {row.emailVerified ? (
            <n-tag type="success" size="small">已验证</n-tag>
          ) : (
            <n-tag type="warning" size="small">未验证</n-tag>
          )}
        </div>
      );
    }
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
    width: 220,
    render: (row: UserInfo) => {
      const isDisabled = row.disabled;
      return <div class="flex space-x-2">
        <n-button size="small" onClick={() => void openUserDetail(row)}>详情</n-button>
        <n-button type="warning" size="small" onClick={() => tryUserResetPassword(row)}>重置密码</n-button>
        {!isDisabled ? <n-button type="error" size="small" onClick={() => tryUserDisable(row)}>停用</n-button> :
          <>
            <n-button type="success" size="small" onClick={() => tryUserEnable(row)}>启用</n-button>
            <n-button type="error" size="small" onClick={() => tryUserDelete(row)}>删除</n-button>
          </>}
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
          placeholder="搜索用户名/昵称/邮箱"
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

        <n-select
          v-model:value="userStatus"
          :options="statusOptions"
          placeholder="用户状态"
          style="width: 140px"
          @update:value="handleStatusChange"
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
        :scroll-x="1100"
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

    <n-modal
      v-model:show="showUserDetailModal"
      preset="card"
      title="用户详情"
      :style="{ width: 'min(620px, 92vw)' }"
    >
      <n-spin :show="userDetailLoading">
        <div v-if="userDetail" class="user-management__detail">
          <div class="user-management__detail-header">
            <Avatar
              :src="userDetail.avatar"
              :size="80"
              :fallback-text="userDetail.nick || userDetail.username"
              use-text-fallback
            />
            <div class="user-management__detail-heading">
              <strong>{{ userDetail.nick || userDetail.username }}</strong>
              <span>@{{ userDetail.username }}</span>
            </div>
          </div>

          <n-descriptions label-placement="top" :column="2" size="small" bordered>
            <n-descriptions-item label="SealChat ID">
              {{ userDetail.id }}
            </n-descriptions-item>
            <n-descriptions-item label="用户类型">
              {{ userDetail.is_bot ? 'BOT' : '用户' }}
            </n-descriptions-item>
            <n-descriptions-item label="邮箱">
              {{ userDetail.email || '-' }}
            </n-descriptions-item>
            <n-descriptions-item label="邮箱状态">
              <n-tag :type="userDetail.emailVerified ? 'success' : 'warning'" size="small">
                {{ userDetail.email ? (userDetail.emailVerified ? '已验证' : '未验证') : '未绑定' }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="系统角色">
              {{ formatRoleNames(userDetail.roleIds) }}
            </n-descriptions-item>
            <n-descriptions-item label="状态">
              <n-tag :type="userDetail.disabled ? 'error' : 'success'" size="small">
                {{ userDetail.disabled ? '已禁用' : '正常' }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="加入世界数">
              {{ userDetailStats?.joinedWorldCount ?? '-' }}
            </n-descriptions-item>
            <n-descriptions-item label="创建世界数">
              {{ userDetailStats?.createdWorldCount ?? '-' }}
            </n-descriptions-item>
            <n-descriptions-item label="注册时间">
              {{ formatDateTime(userDetail.createdAt) }}
            </n-descriptions-item>
            <n-descriptions-item label="更新时间">
              {{ formatDateTime(userDetail.updatedAt) }}
            </n-descriptions-item>
            <n-descriptions-item :span="2" label="简介">
              {{ userDetail.brief || '-' }}
            </n-descriptions-item>
          </n-descriptions>
        </div>
        <n-empty v-else description="没有可查看的用户信息" />
      </n-spin>
    </n-modal>

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

.user-management__detail-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 16px;
}

.user-management__detail-heading {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.user-management__detail-heading strong {
  font-size: 18px;
}

.user-management__detail-heading span {
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
