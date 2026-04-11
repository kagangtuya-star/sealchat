<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import dayjs from 'dayjs'
import { Edit, Eye, Trash } from '@vicons/tabler'
import { useWindowSize } from '@vueuse/core'
import { NIcon, useDialog, useMessage } from 'naive-ui'
import type { AnnouncementItem, AnnouncementScopeType } from '@/models/announcement'
import { useAnnouncementStore } from '@/stores/announcement'
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render'
import AnnouncementEditorModal from './AnnouncementEditorModal.vue'
import AnnouncementPopupModal from './AnnouncementPopupModal.vue'

const props = defineProps<{
  visible: boolean
  scopeType: AnnouncementScopeType
  scopeId?: string
  title?: string
  canManage?: boolean
}>()

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
}>()

const announcementStore = useAnnouncementStore()
const dialog = useDialog()
const message = useMessage()
const { width: viewportWidth } = useWindowSize()

const loading = ref(false)
const items = ref<AnnouncementItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const editorVisible = ref(false)
const editingItem = ref<AnnouncementItem | null>(null)
const previewVisible = ref(false)
const previewItem = ref<AnnouncementItem | null>(null)
const handleVisibleUpdate = (value: boolean) => emit('update:visible', value)
const pageSizes = [20, 50, 100]

const modalStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'width: calc(100vw - 12px); max-width: calc(100vw - 12px);'
    : 'width: min(60vw, 1280px); max-width: calc(100vw - 32px);'
))

const headerStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 12px 14px 8px;'
    : 'padding: 14px 18px 10px;'
))

const contentStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 10px 12px 12px;'
    : 'padding: 10px 18px 16px;'
))

const renderBodyHtml = (item: AnnouncementItem) => {
  if (item.contentFormat === 'rich' && isTipTapJson(item.content)) {
    return tiptapJsonToHtml(item.content, { imageClass: 'announcement-content__image' })
  }
  const div = document.createElement('div')
  div.textContent = item.content || ''
  return div.innerHTML.replace(/\n/g, '<br />')
}

const load = async (options: { page?: number; pageSize?: number } = {}) => {
  loading.value = true
  try {
    const nextPage = options.page ?? page.value
    const nextPageSize = options.pageSize ?? pageSize.value
    const data = props.scopeType === 'world'
      ? await announcementStore.fetchWorldList(String(props.scopeId || '').trim(), { page: nextPage, pageSize: nextPageSize, includeAll: !!props.canManage })
      : await announcementStore.fetchLobbyList({ page: nextPage, pageSize: nextPageSize, includeAll: !!props.canManage })
    items.value = data.items || []
    total.value = data.total || 0
    page.value = data.page || nextPage
    pageSize.value = data.pageSize || nextPageSize

    const maxPage = Math.max(1, Math.ceil(total.value / pageSize.value))
    if (total.value > 0 && page.value > maxPage) {
      await load({ page: maxPage, pageSize: pageSize.value })
    }
  } catch (err: any) {
    message.error(err?.response?.data?.message || '加载公告失败')
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (value) => {
    if (value) {
      page.value = 1
      void load({ page: 1 })
    }
  },
)

const handlePageChange = (value: number) => {
  page.value = value
  void load({ page: value })
}

const handlePageSizeChange = (value: number) => {
  pageSize.value = value
  page.value = 1
  void load({ page: 1, pageSize: value })
}

const openCreate = () => {
  editingItem.value = null
  editorVisible.value = true
}

const openEdit = (item: AnnouncementItem) => {
  editingItem.value = item
  editorVisible.value = true
}

const openPreview = (item: AnnouncementItem) => {
  previewItem.value = item
  previewVisible.value = true
}

const handleDelete = (item: AnnouncementItem) => {
  dialog.warning({
    title: '删除公告',
    content: `确定要删除「${item.title}」吗？`,
    positiveText: '删除',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: async () => {
      try {
        if (props.scopeType === 'world') {
          await announcementStore.removeWorld(String(props.scopeId || '').trim(), item.id)
        } else {
          await announcementStore.removeLobby(item.id)
        }
        message.success('公告已删除')
        await load()
      } catch (err: any) {
        message.error(err?.response?.data?.message || '删除失败')
      }
    },
  })
}

const handleAck = async (item: AnnouncementItem) => {
  if (props.scopeType !== 'world') return
  try {
    await announcementStore.ackWorld(String(props.scopeId || '').trim(), item.id)
    message.success('已确认')
    await load()
  } catch (err: any) {
    message.error(err?.response?.data?.message || '确认失败')
  }
}

const handlePreviewAck = async () => {
  const item = previewItem.value
  if (props.scopeType !== 'world' || !item?.id) return
  try {
    await announcementStore.ackWorld(String(props.scopeId || '').trim(), item.id)
    message.success('已确认')
    previewVisible.value = false
    previewItem.value = null
    await load()
  } catch (err: any) {
    message.error(err?.response?.data?.message || '确认失败')
  }
}

const publishedText = (item: AnnouncementItem) => {
  const value = item.publishedAt || item.updatedAt || item.createdAt
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '未发布'
}

const plainTextOf = (item: AnnouncementItem) => {
  if (!item.content) return ''
  if (item.contentFormat === 'rich' && isTipTapJson(item.content)) {
    return tiptapJsonToPlainText(item.content).trim()
  }
  return item.content.trim()
}

const shouldCollapseBody = (item: AnnouncementItem) => {
  const text = plainTextOf(item)
  if (!text) return false
  const lineCount = text.split(/\r?\n/).filter(Boolean).length
  return text.length > 180 || lineCount > 4
}
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    closable
    :title="title || '公告'"
    :style="modalStyle"
    :header-style="headerStyle"
    :content-style="contentStyle"
    @update:show="handleVisibleUpdate"
  >
    <div class="announcement-manager">
      <div class="announcement-manager__toolbar">
        <div class="announcement-manager__summary">共 {{ total }} 条公告</div>
        <n-space size="small">
          <n-button size="small" @click="load" :loading="loading">刷新</n-button>
          <n-button v-if="canManage" size="small" type="primary" @click="openCreate">新建公告</n-button>
        </n-space>
      </div>

      <n-spin :show="loading">
        <n-empty v-if="!items.length" description="暂无公告" />
        <div v-else class="announcement-list">
          <n-card v-for="item in items" :key="item.id" size="small" class="announcement-card">
            <template #header>
              <div class="announcement-card__header">
                <div class="announcement-card__header-main">
                  <div class="announcement-card__title-row">
                    <div class="announcement-card__title">{{ item.title }}</div>
                    <div v-if="canManage" class="announcement-card__icon-actions">
                      <n-button circle quaternary size="tiny" title="编辑" @click="openEdit(item)">
                        <template #icon>
                          <n-icon><Edit /></n-icon>
                        </template>
                      </n-button>
                      <n-button circle quaternary size="tiny" type="error" title="删除" @click="handleDelete(item)">
                        <template #icon>
                          <n-icon><Trash /></n-icon>
                        </template>
                      </n-button>
                    </div>
                  </div>
                  <n-space size="small" class="announcement-card__tags">
                    <n-tag v-if="item.status === 'draft'" size="small">草稿</n-tag>
                    <n-tag v-if="item.isPinned" size="small" type="warning">置顶</n-tag>
                    <n-tag
                      v-if="props.scopeType === 'lobby' && item.showInTicker"
                      size="small"
                      type="primary"
                    >
                      广播区
                    </n-tag>
                    <n-tag v-if="item.popupMode === 'every_entry'" size="small" type="info">每次弹出</n-tag>
                    <n-tag v-else-if="item.popupMode === 'once_per_version'" size="small" type="info">每版本弹一次</n-tag>
                    <n-tag
                      v-if="props.scopeType === 'lobby' && item.reminderScope === 'site_wide'"
                      size="small"
                      type="success"
                    >
                      全站在线提醒
                    </n-tag>
                    <n-tag v-if="item.requireAck" size="small" type="error">需确认</n-tag>
                    <n-tag v-if="item.needsAck" size="small" type="warning">待确认</n-tag>
                  </n-space>
                </div>
              </div>
            </template>
            <div class="announcement-card__meta">
              <span v-if="item.creatorName">发布人：{{ item.creatorName }}</span>
              <span>时间：{{ publishedText(item) }}</span>
              <span v-if="item.requireAck">{{ item.ackCount || 0 }} 人已确认</span>
            </div>
            <div
              :class="[
                'announcement-card__body',
                'announcement-rich-html',
                { 'announcement-card__body--collapsed': shouldCollapseBody(item) },
              ]"
              v-html="renderBodyHtml(item)"
            />
            <div class="announcement-card__actions">
              <n-space justify="space-between" align="center">
                <n-button size="tiny" text @click="openPreview(item)">
                  <template #icon>
                    <n-icon><Eye /></n-icon>
                  </template>
                  查看
                </n-button>
                <n-space justify="end">
                <n-button
                  v-if="scopeType === 'world' && item.requireAck && item.needsAck"
                  size="tiny"
                  type="primary"
                  tertiary
                  @click="handleAck(item)"
                >
                  确认已读
                </n-button>
                </n-space>
              </n-space>
            </div>
          </n-card>
        </div>
      </n-spin>
      <div v-if="total > pageSize" class="announcement-manager__pagination">
        <n-pagination
          size="small"
          :page="page"
          :page-size="pageSize"
          :item-count="total"
          show-size-picker
          :page-sizes="pageSizes"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </div>
    </div>
    <AnnouncementEditorModal
      v-model:visible="editorVisible"
      :scope-type="scopeType"
      :scope-id="scopeId"
      :item="editingItem"
      @saved="load"
    />
    <AnnouncementPopupModal
      v-model:visible="previewVisible"
      :item="previewItem"
      @ack="handlePreviewAck"
    />
  </n-modal>
</template>

<style scoped>
.announcement-manager {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 80vh;
  overflow: auto;
}

.announcement-manager__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  padding-bottom: 2px;
}

.announcement-manager__summary {
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.announcement-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}

.announcement-manager__pagination {
  display: flex;
  justify-content: center;
  padding-top: 4px;
}

.announcement-card {
  width: 100%;
  box-sizing: border-box;
}

.announcement-card__header {
  min-width: 0;
}

.announcement-card__header-main {
  min-width: 0;
}

.announcement-card__title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.announcement-card__title {
  flex: 1 1 auto;
  min-width: 0;
  font-weight: 700;
  font-size: 16px;
  line-height: 1.35;
  word-break: break-word;
}

.announcement-card__icon-actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
}

.announcement-card__tags {
  margin-top: 8px;
}

.announcement-card__body {
  line-height: 1.65;
  word-break: break-word;
  overflow-wrap: anywhere;
  position: relative;
}

.announcement-card__body--collapsed {
  max-height: 9.5rem;
  overflow: hidden;
}

.announcement-card__body--collapsed::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 48px;
  background: linear-gradient(to bottom, rgba(31, 31, 35, 0), rgba(31, 31, 35, 0.96));
  pointer-events: none;
}

.announcement-card__body :deep(p:first-child) {
  margin-top: 0;
}

.announcement-card__header :deep(.n-space),
.announcement-card__actions :deep(.n-space) {
  flex-wrap: wrap;
}

.announcement-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.announcement-card__actions {
  margin-top: 8px;
}

.announcement-rich-html :deep(h1),
.announcement-rich-html :deep(h2),
.announcement-rich-html :deep(h3) {
  margin: 0.8rem 0 0.45rem;
  font-weight: 700;
  line-height: 1.3;
}

.announcement-rich-html :deep(h1) {
  font-size: 1.45rem;
}

.announcement-rich-html :deep(h2) {
  font-size: 1.22rem;
}

.announcement-rich-html :deep(h3) {
  font-size: 1.05rem;
}

.announcement-rich-html :deep(p) {
  margin: 0 0 0.7rem;
}

.announcement-rich-html :deep(ul),
.announcement-rich-html :deep(ol) {
  margin: 0 0 0.8rem;
  padding-left: 1.35rem;
}

.announcement-rich-html :deep(blockquote) {
  margin: 0 0 0.8rem;
  padding-left: 0.9rem;
  border-left: 3px solid rgba(88, 166, 255, 0.45);
  color: var(--sc-text-secondary);
}

.announcement-rich-html :deep(pre) {
  margin: 0 0 0.8rem;
  padding: 0.75rem 0.9rem;
  border-radius: 10px;
  overflow: auto;
  background: rgba(0, 0, 0, 0.18);
}

.announcement-rich-html :deep(code) {
  font-family: ui-monospace, SFMono-Regular, SFMono-Regular, Consolas, monospace;
}

.announcement-rich-html :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0.4rem 0 0.8rem;
  border-radius: 10px;
}

.announcement-rich-html :deep(a) {
  color: #5aa9ff;
  text-decoration: underline;
}

.announcement-rich-html :deep(hr) {
  margin: 0.9rem 0;
  border: 0;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

@media (max-width: 640px) {
  .announcement-manager {
    max-height: 84vh;
    gap: 8px;
  }

  .announcement-manager__toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }

  .announcement-card__title-row {
    gap: 6px;
  }

  .announcement-card__title {
    flex-basis: auto;
    font-size: 15px;
  }

  .announcement-card__tags {
    margin-top: 6px;
  }

  .announcement-card__actions :deep(.n-space) {
    width: 100%;
  }

  .announcement-card__body--collapsed {
    max-height: 8.25rem;
  }
}
</style>
