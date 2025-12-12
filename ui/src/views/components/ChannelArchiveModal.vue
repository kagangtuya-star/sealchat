<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage, useDialog } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import { useBreakpoints } from '@vueuse/core';
import { SettingsSharp, Menu } from '@vicons/ionicons5';
import type { SChannel } from '@/types';

interface ArchivedChannel {
  id: string;
  name: string;
  note?: string;
  parentId?: string;
  children?: ArchivedChannel[];
}

interface ArchivedListResponse {
  items: ArchivedChannel[];
  total: number;
  canManage: boolean;
  canDelete: boolean;
}

const props = defineProps<{ show: boolean }>();
const emit = defineEmits<{ (e: 'update:show', value: boolean): void }>();

const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();

const breakpoints = useBreakpoints({ tablet: 768 });
const isMobile = breakpoints.smaller('tablet');

const visible = computed({
  get: () => props.show,
  set: (value: boolean) => emit('update:show', value),
});

const loading = ref(false);
const keyword = ref('');
const page = ref(1);
const pageSize = ref(8);
const total = ref(0);
const canManage = ref(false);
const canDelete = ref(false);
const archivedChannels = ref<ArchivedChannel[]>([]);
const selectedIds = ref<Set<string>>(new Set());
const operating = ref(false);

// 树形数据构建
const treeData = computed(() => {
  const items = archivedChannels.value;
  const parentMap = new Map<string, ArchivedChannel[]>();
  const roots: ArchivedChannel[] = [];

  items.forEach((item) => {
    if (!item.parentId) {
      roots.push({ ...item, children: [] });
    } else {
      if (!parentMap.has(item.parentId)) {
        parentMap.set(item.parentId, []);
      }
      parentMap.get(item.parentId)!.push(item);
    }
  });

  // 将子频道挂载到父频道
  roots.forEach((root) => {
    root.children = parentMap.get(root.id) || [];
  });

  // 处理孤儿子频道（父频道不在列表中）
  parentMap.forEach((children, parentId) => {
    if (!roots.find((r) => r.id === parentId)) {
      roots.push(...children);
    }
  });

  return roots;
});

const allIds = computed(() => {
  const ids: string[] = [];
  archivedChannels.value.forEach((ch) => {
    ids.push(ch.id);
  });
  return ids;
});

const isAllSelected = computed(() => {
  if (allIds.value.length === 0) return false;
  return allIds.value.every((id) => selectedIds.value.has(id));
});

const toggleSelectAll = () => {
  if (isAllSelected.value) {
    selectedIds.value.clear();
  } else {
    selectedIds.value = new Set(allIds.value);
  }
};

const toggleSelect = (id: string) => {
  if (selectedIds.value.has(id)) {
    selectedIds.value.delete(id);
  } else {
    selectedIds.value.add(id);
  }
  selectedIds.value = new Set(selectedIds.value);
};

const loadData = async () => {
  const worldId = chat.currentWorldId;
  if (!worldId) {
    message.warning('请先选择一个世界');
    return;
  }

  loading.value = true;
  try {
    const result = await chat.getArchivedChannels(worldId, {
      keyword: keyword.value.trim(),
      page: page.value,
      pageSize: pageSize.value,
    });
    archivedChannels.value = result.items || [];
    total.value = result.total || 0;
    canManage.value = result.canManage || false;
    canDelete.value = result.canDelete || false;
    selectedIds.value.clear();
  } catch (error: any) {
    message.error(error?.response?.data?.error || '加载归档频道失败');
  } finally {
    loading.value = false;
  }
};

watch(
  () => props.show,
  (val) => {
    if (val) {
      page.value = 1;
      keyword.value = '';
      loadData();
    }
  },
  { immediate: true },
);

const handleSearch = () => {
  page.value = 1;
  loadData();
};

const handlePageChange = (newPage: number) => {
  page.value = newPage;
  loadData();
};

const handleUnarchive = async (channelIds: string[], includeChildren = true) => {
  if (channelIds.length === 0) return;

  dialog.warning({
    title: '确认恢复频道',
    content: `确定要恢复选中的 ${channelIds.length} 个频道吗？${includeChildren ? '子频道也将一同恢复。' : ''}`,
    positiveText: '确认恢复',
    negativeText: '取消',
    onPositiveClick: async () => {
      operating.value = true;
      try {
        await chat.unarchiveChannels(channelIds, includeChildren);
        message.success('频道已恢复');
        await loadData();
        // 刷新频道列表
        if (chat.currentWorldId) {
          await chat.channelList(chat.currentWorldId, true);
        }
      } catch (error: any) {
        message.error(error?.response?.data?.error || '恢复失败');
      } finally {
        operating.value = false;
      }
    },
  });
};

const handleBatchUnarchive = () => {
  const ids = Array.from(selectedIds.value);
  if (ids.length === 0) {
    message.warning('请先选择要恢复的频道');
    return;
  }
  handleUnarchive(ids, true);
};

const handlePermanentDelete = async (channelIds: string[]) => {
  if (channelIds.length === 0) return;

  // 第一次确认
  dialog.error({
    title: '⚠️ 永久删除警告',
    content: `您确定要永久删除选中的 ${channelIds.length} 个频道吗？此操作不可恢复！`,
    positiveText: '我确定要删除',
    negativeText: '取消',
    onPositiveClick: () => {
      // 第二次确认
      dialog.error({
        title: '🚨 最终确认',
        content: '这是最后一次确认。删除后所有频道数据、消息记录将永久丢失。请输入 "CONFIRM_DELETE" 继续。',
        positiveText: '永久删除',
        negativeText: '取消',
        onPositiveClick: async () => {
          operating.value = true;
          try {
            await chat.deleteArchivedChannels(channelIds, 'CONFIRM_DELETE');
            message.success('频道已永久删除');
            await loadData();
          } catch (error: any) {
            message.error(error?.response?.data?.error || '删除失败');
          } finally {
            operating.value = false;
          }
        },
      });
    },
  });
};

const handleBatchDelete = () => {
  const ids = Array.from(selectedIds.value);
  if (ids.length === 0) {
    message.warning('请先选择要删除的频道');
    return;
  }
  handlePermanentDelete(ids);
};

const handleViewChannel = async (channelId: string) => {
  // 进入频道查看历史消息
  visible.value = false;
  await chat.channelSwitchTo(channelId);
};

const handleManageChannel = (channelId: string) => {
  // 打开频道管理设置（先切换到该频道，再打开设置）
  visible.value = false;
  // 通过事件触发侧边栏打开设置
  chat.channelSwitchTo(channelId).then(() => {
    // 不需要额外操作，用户可以在侧边栏右键管理
  });
};

const handleMenuSelect = async (key: string, channelId: string, includeChildren = true) => {
  switch (key) {
    case 'view':
      await handleViewChannel(channelId);
      break;
    case 'unarchive':
      await handleUnarchive([channelId], includeChildren);
      break;
    case 'delete':
      await handlePermanentDelete([channelId]);
      break;
    default:
      break;
  }
};

const closeModal = () => {
  visible.value = false;
};

const modalWidth = computed(() => (isMobile.value ? '100%' : '600px'));
</script>

<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="归档管理"
    :style="{ width: modalWidth, maxWidth: '100vw', maxHeight: isMobile ? '100vh' : '80vh' }"
    :mask-closable="true"
    :closable="true"
    :bordered="false"
    class="channel-archive-modal"
    @close="closeModal"
  >
    <div class="archive-container">
      <!-- 搜索栏 -->
      <div class="archive-search">
        <n-input
          v-model:value="keyword"
          placeholder="搜索频道..."
          clearable
          size="small"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <span>🔍</span>
          </template>
        </n-input>
        <n-button size="small" @click="handleSearch" :loading="loading">
          搜索
        </n-button>
      </div>

      <!-- 操作栏 -->
      <div v-if="canManage" class="archive-actions">
        <n-checkbox
          :checked="isAllSelected"
          :indeterminate="selectedIds.size > 0 && !isAllSelected"
          @update:checked="toggleSelectAll"
        >
          全选
        </n-checkbox>
        <div class="action-buttons">
          <n-button
            size="tiny"
            type="primary"
            :disabled="selectedIds.size === 0 || operating"
            @click="handleBatchUnarchive"
          >
            批量恢复
          </n-button>
          <n-button
            v-if="canDelete"
            size="tiny"
            type="error"
            :disabled="selectedIds.size === 0 || operating"
            @click="handleBatchDelete"
          >
            永久删除
          </n-button>
        </div>
      </div>

      <!-- 频道列表 -->
      <div class="archive-list scrollbar-thin">
        <n-spin :show="loading">
          <div v-if="archivedChannels.length === 0 && !loading" class="empty-state">
            <n-empty description="暂无归档频道" />
          </div>

          <div v-else class="channel-tree">
            <template v-for="channel in treeData" :key="channel.id">
              <div class="channel-item channel-item--root">
                <div class="channel-item__main" @click="handleViewChannel(channel.id)">
                  <n-checkbox
                    v-if="canManage"
                    :checked="selectedIds.has(channel.id)"
                    @update:checked="() => toggleSelect(channel.id)"
                    @click.stop
                  />
                  <span class="channel-icon">#</span>
                  <n-tooltip trigger="hover" placement="top">
                    <template #trigger>
                      <span class="channel-name clickable">{{ channel.name }}</span>
                    </template>
                    点击查看历史消息
                  </n-tooltip>
                </div>
                <div class="channel-item__actions">
                  <n-dropdown
                    trigger="click"
                    :options="[
                      { label: '查看历史', key: 'view' },
                      { label: '恢复频道', key: 'unarchive', show: canManage },
                      { label: '永久删除', key: 'delete', show: canDelete }
                    ].filter(o => o.show !== false)"
                    @select="(key: string) => handleMenuSelect(key, channel.id, true)"
                  >
                    <n-button quaternary circle size="tiny" :disabled="operating">
                      <template #icon>
                        <n-icon><Menu /></n-icon>
                      </template>
                    </n-button>
                  </n-dropdown>
                  <n-button quaternary circle size="tiny" @click="handleViewChannel(channel.id)">
                    <template #icon>
                      <n-icon><SettingsSharp /></n-icon>
                    </template>
                  </n-button>
                </div>
              </div>

              <!-- 子频道 -->
              <template v-if="channel.children?.length">
                <div
                  v-for="child in channel.children"
                  :key="child.id"
                  class="channel-item channel-item--child"
                >
                  <div class="channel-item__main" @click="handleViewChannel(child.id)">
                    <n-checkbox
                      v-if="canManage"
                      :checked="selectedIds.has(child.id)"
                      @update:checked="() => toggleSelect(child.id)"
                      @click.stop
                    />
                    <span class="channel-indent">└</span>
                    <span class="channel-icon">#</span>
                    <n-tooltip trigger="hover" placement="top">
                      <template #trigger>
                        <span class="channel-name clickable">{{ child.name }}</span>
                      </template>
                      点击查看历史消息
                    </n-tooltip>
                  </div>
                  <div class="channel-item__actions">
                    <n-dropdown
                      trigger="click"
                      :options="[
                        { label: '查看历史', key: 'view' },
                        { label: '恢复频道', key: 'unarchive', show: canManage },
                        { label: '永久删除', key: 'delete', show: canDelete }
                      ].filter(o => o.show !== false)"
                      @select="(key: string) => handleMenuSelect(key, child.id, false)"
                    >
                      <n-button quaternary circle size="tiny" :disabled="operating">
                        <template #icon>
                          <n-icon><Menu /></n-icon>
                        </template>
                      </n-button>
                    </n-dropdown>
                    <n-button quaternary circle size="tiny" @click="handleViewChannel(child.id)">
                      <template #icon>
                        <n-icon><SettingsSharp /></n-icon>
                      </template>
                    </n-button>
                  </div>
                </div>
              </template>
            </template>
          </div>
        </n-spin>
      </div>

      <!-- 分页 -->
      <div v-if="total > pageSize" class="archive-pagination">
        <n-pagination
          v-model:page="page"
          :page-count="Math.ceil(total / pageSize)"
          :page-size="pageSize"
          size="small"
          @update:page="handlePageChange"
        />
      </div>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
.channel-archive-modal {
  :deep(.n-card__content) {
    padding: 0;
  }
}

.archive-container {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1rem;
  max-height: 70vh;
}

.archive-search {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.archive-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--sc-border-mute, #e5e7eb);
}

.action-buttons {
  display: flex;
  gap: 0.5rem;
}

.archive-list {
  flex: 1;
  overflow-y: auto;
  min-height: 200px;
  max-height: 400px;
}

.empty-state {
  padding: 2rem;
  text-align: center;
}

.channel-tree {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.channel-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.75rem;
  border-radius: 0.375rem;
  background-color: var(--sc-bg-elevated, #fff);
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  transition: background-color 0.2s ease;

  &:hover {
    background-color: var(--sc-sidebar-hover, #f5f5f5);
  }

  &--child {
    margin-left: 1.5rem;
    border-color: var(--sc-border-mute, #e0e0e0);
  }
}

.channel-item__main {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex: 1;
  min-width: 0;
  cursor: pointer;
  padding: 0.25rem;
  border-radius: 0.25rem;
  transition: background-color 0.15s ease;

  &:hover {
    background-color: var(--sc-sidebar-hover, rgba(0, 0, 0, 0.05));
  }
}

.channel-item__actions {
  display: flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.channel-name.clickable {
  cursor: pointer;
  &:hover {
    text-decoration: underline;
  }
}

.channel-icon {
  color: var(--sc-text-secondary, #888);
  font-weight: bold;
}

.channel-indent {
  color: var(--sc-text-secondary, #888);
  margin-left: 0.25rem;
}

.channel-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--sc-text-primary, #333);
}

.archive-pagination {
  display: flex;
  justify-content: center;
  padding-top: 0.5rem;
  border-top: 1px solid var(--sc-border-mute, #e5e7eb);
}

/* 简化滚动条 */
.scrollbar-thin {
  scrollbar-width: thin;
  scrollbar-color: var(--sc-border-mute, #ccc) transparent;

  &::-webkit-scrollbar {
    width: 6px;
  }

  &::-webkit-scrollbar-track {
    background: transparent;
  }

  &::-webkit-scrollbar-thumb {
    background-color: var(--sc-border-mute, #ccc);
    border-radius: 3px;
  }
}

/* 移动端适配 */
@media (max-width: 768px) {
  .archive-container {
    max-height: calc(100vh - 80px);
  }

  .archive-list {
    max-height: calc(100vh - 250px);
  }

  .channel-item {
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .channel-item__actions {
    width: 100%;
    justify-content: flex-end;
  }

  .channel-item--child {
    margin-left: 1rem;
  }
}
</style>
