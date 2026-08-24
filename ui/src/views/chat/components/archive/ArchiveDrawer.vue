<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useWindowSize } from '@vueuse/core'
import type { Message } from '@satorijs/protocol'
import ChatItem from '../chat-item.vue'

interface ArchivedMessage {
  id: string
  message: Message
  createdAt: string
  archivedAt: string
  archivedBy: string
  sender: {
    name: string
    avatar?: string
  }
}

interface Props {
  visible: boolean
  messages: ArchivedMessage[]
  loading?: boolean
  page: number
  pageCount: number
  total: number
  searchQuery: string
  hasMore?: boolean
}

interface Emits {
  (e: 'update:visible', visible: boolean): void
  (e: 'update:page', page: number): void
  (e: 'update:search', keyword: string): void
  (e: 'unarchive', messageIds: string[]): void
  (e: 'delete', messageIds: string[]): void
  (e: 'load-more'): void
  (e: 'refresh'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const message = useMessage()
const { width: viewportWidth } = useWindowSize()
const isMobileLayout = computed(() => viewportWidth.value > 0 && viewportWidth.value < 768)
const selectedIds = ref<string[]>([])
const searchValue = ref(props.searchQuery)

watch(
  () => props.searchQuery,
  (value) => {
    if (value !== searchValue.value) {
      searchValue.value = value
    }
  },
)

const handleSearchInput = (value: string) => {
  searchValue.value = value
  emit('update:search', value)
}

const handlePageChange = (page: number) => {
  emit('update:page', page)
}

const allSelected = computed({
  get: () => selectedIds.value.length === props.messages.length && props.messages.length > 0,
  set: (value: boolean) => {
    selectedIds.value = value ? props.messages.map(m => m.id) : []
  }
})

const hasSelection = computed(() => selectedIds.value.length > 0)

const handleSelectionChange = (id: string, checked: boolean) => {
  if (checked) {
    if (!selectedIds.value.includes(id)) {
      selectedIds.value.push(id)
    }
    return
  }
  const index = selectedIds.value.indexOf(id)
  if (index > -1) {
    selectedIds.value.splice(index, 1)
  }
}

const formatDate = (dateStr: string) => {
  if (!dateStr) {
    return '未知时间'
  }
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) {
    return '未知时间'
  }
  return date.toLocaleString()
}

const handleUnarchive = async () => {
  if (!hasSelection.value) {
    message.warning('请选择要恢复的消息')
    return
  }

  try {
    emit('unarchive', selectedIds.value)
    selectedIds.value = []
  } catch (error) {
    message.error('恢复失败')
  }
}

const handleDelete = async () => {
  if (!hasSelection.value) {
    message.warning('请选择要删除的消息')
    return
  }

  // 这里应该有确认对话框
  try {
    emit('delete', selectedIds.value)
    selectedIds.value = []
  } catch (error) {
    message.error('删除失败')
  }
}

const handleClose = () => {
  emit('update:visible', false)
  selectedIds.value = []
}
</script>

<template>
  <n-drawer
    class="archive-drawer"
    :show="visible"
    @update:show="emit('update:visible', $event)"
    placement="right"
    :width="400"
  >
    <n-drawer-content>
      <template #header>
        <div class="archive-header">
          <div class="archive-header__title">
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="emit('update:visible', false)">
              返回
            </n-button>
            <span>归档消息管理</span>
          </div>
          <n-button text @click="emit('refresh')">
            <template #icon>
              <n-icon component="ReloadOutlined" />
            </template>
          </n-button>
        </div>
      </template>

      <div class="archive-content">
        <div class="archive-toolbar">
          <n-input
            v-model:value="searchValue"
            size="small"
            placeholder="搜索内容、发送者或归档人"
            clearable
            @update:value="handleSearchInput"
          />
          <n-tag size="small" type="info">
            共 {{ total }}<span v-if="props.hasMore && !props.searchQuery.trim()">+</span> 条
          </n-tag>
        </div>

        <div v-if="loading" class="archive-loading">
          <n-spin size="large" />
          <p>加载中...</p>
        </div>

        <div v-else-if="messages.length === 0" class="archive-empty">
          <n-empty description="暂无归档消息" />
        </div>

        <div v-else class="archive-list">
          <div class="archive-controls">
            <n-checkbox
              v-model:checked="allSelected"
              :indeterminate="hasSelection && !allSelected"
            >
              全选 ({{ messages.length }})
            </n-checkbox>

            <div class="control-actions">
              <n-button
                size="small"
                :disabled="!hasSelection"
                @click="handleUnarchive"
              >
                恢复选中
              </n-button>
              <n-button
                size="small"
                type="error"
                :disabled="!hasSelection"
                @click="handleDelete"
              >
                删除选中
              </n-button>
            </div>
          </div>

          <div class="message-list">
            <div
              v-for="msg in messages"
              :key="msg.id"
              class="message-item"
              :class="{ 'selected': selectedIds.includes(msg.id) }"
            >
              <n-checkbox
                :checked="selectedIds.includes(msg.id)"
                @update:checked="handleSelectionChange(msg.id, $event)"
              />

              <div class="message-content">
                <div class="message-header">
                  <span class="sender-name">{{ msg.sender.name }}</span>
                  <span class="message-date">{{ formatDate(msg.createdAt) }}</span>
                </div>
                <div class="message-renderer">
                  <ChatItem
                    :avatar="msg.sender.avatar"
                    :username="msg.sender.name"
                    :content="msg.message.content || ''"
                    :item="msg.message"
                    tone="archived"
                    layout="compact"
                    :show-avatar="false"
                    :body-only="true"
                    :readonly="true"
                  />
                </div>
                <div class="archive-info">
                  <span class="archive-date">归档于 {{ formatDate(msg.archivedAt) }}</span>
                  <span class="archive-by">by {{ msg.archivedBy }}</span>
                </div>
              </div>
            </div>
          </div>

          <div v-if="props.searchQuery.trim()" class="archive-pagination">
            <n-pagination
              size="small"
              :page="props.page"
              :page-count="Math.max(props.pageCount, 1)"
              :item-count="props.total"
              :page-size="30"
              :disabled="props.pageCount <= 1"
              @update:page="handlePageChange"
            />
          </div>
          <div v-else-if="props.hasMore" class="archive-load-more">
            <n-button size="small" :loading="props.loading" @click="emit('load-more')">
              加载更旧消息
            </n-button>
          </div>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style lang="scss" scoped>
.archive-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.archive-header__title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.archive-drawer :deep(.n-drawer) {
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
}

.archive-drawer :deep(.n-drawer-body) {
  background-color: var(--sc-bg-elevated, #ffffff);
}

.archive-content {
  height: 100%;
  display: flex;
  flex-direction: column;
  color: var(--sc-text-primary, #0f172a);
  background-color: var(--sc-bg-elevated, #ffffff);
  padding-bottom: 0.25rem;
}

.archive-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
  color: var(--sc-text-secondary, #475569);
}

.archive-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  gap: 1rem;
}

.archive-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 200px;
}

.archive-list {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.archive-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
  border-radius: 0.5rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.2));
  color: var(--sc-text-primary, #0f172a);
}

.control-actions {
  display: flex;
  gap: 0.5rem;
}

.message-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.message-item {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 0.75rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.2));
  border-radius: 0.5rem;
  background: var(--sc-bg-surface, #ffffff);
  transition: all 0.2s ease;
  color: var(--sc-text-primary, #0f172a);
}

.message-item:hover {
  border-color: var(--sc-border-strong, rgba(59, 130, 246, 0.3));
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.12);
  background: var(--sc-bg-elevated, #ffffff);
}

.message-item.selected {
  border-color: rgba(37, 99, 235, 0.45);
  background: rgba(59, 130, 246, 0.12);
}

.message-content {
  flex: 1;
  min-width: 0;
}

.message-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.sender-name {
  font-weight: 600;
  color: var(--sc-text-primary, #1f2937);
}

.message-date {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #6b7280);
}

.message-renderer {
  color: var(--sc-text-primary, #374151);
  line-height: 1.5;
  margin-bottom: 0.5rem;
  word-break: break-word;
}

.message-renderer :deep(.chat-item) {
  width: 100%;
}

.archive-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #9ca3af);
}

.archive-date {
  color: #f59e0b;
}

.archive-by {
  color: var(--sc-text-secondary, #6b7280);
}

.archive-pagination {
  display: flex;
  justify-content: flex-end;
  padding-top: 0.5rem;
}

.archive-load-more {
  display: flex;
  justify-content: center;
  padding-top: 0.5rem;
}
</style>
