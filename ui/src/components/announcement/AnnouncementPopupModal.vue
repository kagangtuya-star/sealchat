<script setup lang="ts">
import { computed } from 'vue'
import dayjs from 'dayjs'
import { useWindowSize } from '@vueuse/core'
import type { AnnouncementItem } from '@/models/announcement'
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render'

const props = defineProps<{
  visible: boolean
  item: AnnouncementItem | null
}>()

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'ack'): void
}>()

const handleVisibleUpdate = (value: boolean) => emit('update:visible', value)
const { width: viewportWidth } = useWindowSize()

const modalStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'width: calc(100vw - 12px); max-width: calc(100vw - 12px);'
    : 'width: min(60vw, 1120px); max-width: calc(100vw - 32px);'
))

const headerStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 12px 14px 8px;'
    : 'padding: 14px 18px 10px;'
))

const contentStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 10px 12px 12px;'
    : 'padding: 10px 18px 14px;'
))

const footerStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 0 12px 12px;'
    : 'padding: 0 18px 14px;'
))

const bodyHtml = computed(() => {
  const item = props.item
  if (!item) return ''
  if (item.contentFormat === 'rich' && isTipTapJson(item.content)) {
    return tiptapJsonToHtml(item.content, { imageClass: 'announcement-content__image' })
  }
  return `<div class="announcement-content__plain">${escapeHtml(item.content || '')}</div>`
})

const publishedText = computed(() => {
  const value = props.item?.publishedAt || props.item?.updatedAt || props.item?.createdAt
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm') : ''
})

const handleAck = () => emit('ack')

function escapeHtml(value: string) {
  const div = document.createElement('div')
  div.textContent = value
  return div.innerHTML.replace(/\n/g, '<br />')
}
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    closable
    :title="item?.title || '公告'"
    :mask-closable="false"
    :style="modalStyle"
    :header-style="headerStyle"
    :content-style="contentStyle"
    :footer-style="footerStyle"
    @update:show="handleVisibleUpdate"
  >
    <div v-if="item" class="announcement-popup">
      <div class="announcement-popup__meta">
        <span v-if="item.creatorName">发布人：{{ item.creatorName }}</span>
        <span v-if="publishedText">发布时间：{{ publishedText }}</span>
        <span v-if="item.requireAck">{{ item.ackCount || 0 }} 人已确认</span>
      </div>
      <div class="announcement-popup__body announcement-rich-html" v-html="bodyHtml" />
    </div>
    <template v-if="item?.requireAck && item?.needsAck" #action>
      <n-space justify="end">
        <n-button type="primary" @click="handleAck">
          确认已读
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.announcement-popup {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.announcement-popup__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.announcement-popup__body {
  max-height: 74vh;
  overflow: auto;
  line-height: 1.7;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.announcement-popup__body :deep(p:first-child) {
  margin-top: 0;
}

.announcement-rich-html :deep(h1),
.announcement-rich-html :deep(h2),
.announcement-rich-html :deep(h3) {
  margin: 0.8rem 0 0.45rem;
  font-weight: 700;
  line-height: 1.3;
}

.announcement-rich-html :deep(h1) {
  font-size: 1.5rem;
}

.announcement-rich-html :deep(h2) {
  font-size: 1.25rem;
}

.announcement-rich-html :deep(h3) {
  font-size: 1.08rem;
}

.announcement-rich-html :deep(p) {
  margin: 0 0 0.75rem;
}

.announcement-rich-html :deep(ul),
.announcement-rich-html :deep(ol) {
  margin: 0 0 0.85rem;
  padding-left: 1.35rem;
}

.announcement-rich-html :deep(blockquote) {
  margin: 0 0 0.85rem;
  padding-left: 0.95rem;
  border-left: 3px solid rgba(88, 166, 255, 0.45);
  color: var(--sc-text-secondary);
}

.announcement-rich-html :deep(pre) {
  margin: 0 0 0.85rem;
  padding: 0.78rem 0.92rem;
  border-radius: 10px;
  overflow: auto;
  background: rgba(0, 0, 0, 0.18);
}

.announcement-rich-html :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0.45rem 0 0.9rem;
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
  .announcement-popup {
    gap: 8px;
  }

  .announcement-popup__body {
    max-height: 76vh;
  }
}
</style>
