<script setup lang="ts">
import { computed } from 'vue'
import { NDropdown, NIcon, NTag, type DropdownOption } from 'naive-ui'
import { ArrowLeft, ArrowUpRight, DotsVertical, LayoutSidebarRight, Lock, MessageOff, X } from '@vicons/tabler'
import type { MessageFavoriteItem, MessageFavoriteTag } from './api'
import { buildFavoriteCardView, canJumpToFavorite } from './presentation'
import type { useFavoriteNotes } from './useFavoriteNotes'
import MessageFavoriteTagPicker from './MessageFavoriteTagPicker.vue'
import MessageFavoriteNoteSection from './MessageFavoriteNoteSection.vue'

// 收藏详情栏：在同一悬浮窗内展示原消息全文、标签与备注。
// split：宽窗口中与列表并排；overlay：窄窗口/移动端覆盖列表区域，标题栏仍可拖动和关闭。

const props = defineProps<{
  item: MessageFavoriteItem
  editor: ReturnType<typeof useFavoriteNotes>
  tags: MessageFavoriteTag[]
  currentUserId: string
  sourceLabel: string
  canMaintain: boolean
  canEditNote: boolean
  layout: 'split' | 'overlay'
  // 可把窗口加宽到分栏布局（桌面窄窗口）。
  canWiden: boolean
  jumping: boolean
  activeTagIds: string[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'jump'): void
  (e: 'remove'): void
  (e: 'widen'): void
  (e: 'filter-tag', tagId: string): void
}>()

const view = computed(() => (props.item.message ? buildFavoriteCardView(props.item.message, props.currentUserId) : null))
const jumpable = computed(() => canJumpToFavorite(props.item))
const tombstoneText = computed(() => (props.item.status === 'deleted' ? '原消息已删除，收藏与备注仍保留' : '当前无权查看原消息，收藏与备注仍保留'))
const menuOptions = computed<DropdownOption[]>(() => (props.canMaintain ? [{ key: 'remove', label: '取消收藏' }] : []))

const handleContentClick = (event: MouseEvent) => {
  const anchor = (event.target as HTMLElement | null)?.closest('a[href]') as HTMLAnchorElement | null
  if (!anchor) return
  event.preventDefault()
  if (/^https?:/i.test(anchor.href)) window.open(anchor.href, '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <aside
    class="mf-inspector"
    :class="`is-${layout}`"
    data-sc-ui="message-favorite-inspector"
    :data-sc-layout="layout"
    aria-label="收藏详情"
  >
    <header class="mf-inspector__bar">
      <button
        type="button"
        class="mf-inspector__icon-btn"
        :aria-label="layout === 'overlay' ? '返回收藏列表' : '关闭详情'"
        :title="layout === 'overlay' ? '返回列表' : '关闭详情'"
        @click="emit('close')"
      >
        <NIcon :size="16"><ArrowLeft v-if="layout === 'overlay'" /><X v-else /></NIcon>
      </button>
      <span class="mf-inspector__title">收藏详情</span>
      <span class="mf-inspector__spacer" />
      <button
        v-if="canWiden"
        type="button"
        class="mf-inspector__icon-btn"
        aria-label="加宽窗口，并排显示列表与详情"
        title="并排显示"
        @click="emit('widen')"
      >
        <NIcon :size="16"><LayoutSidebarRight /></NIcon>
      </button>
      <button
        v-if="jumpable"
        type="button"
        class="mf-inspector__icon-btn"
        :disabled="jumping"
        aria-label="跳转到原消息"
        title="跳转到原消息"
        @click="emit('jump')"
      >
        <NIcon :size="16"><ArrowUpRight /></NIcon>
      </button>
      <NDropdown v-if="menuOptions.length" trigger="click" placement="bottom-end" :options="menuOptions" @select="emit('remove')">
        <button type="button" class="mf-inspector__icon-btn" aria-label="更多操作" title="更多操作">
          <NIcon :size="16"><DotsVertical /></NIcon>
        </button>
      </NDropdown>
    </header>

    <div class="mf-inspector__scroll">
      <section class="mf-inspector__message" :class="{ 'is-tombstone': !view }" data-sc-favorite-part="message">
        <template v-if="view">
          <div class="mf-inspector__meta">
            <img class="mf-inspector__avatar" :src="view.avatarUrl" alt="" loading="lazy" draggable="false">
            <span class="mf-inspector__name" :style="view.senderColor ? { color: view.senderColor } : undefined">{{ view.senderName }}</span>
            <span class="mf-inspector__time" :title="view.timeTitle">{{ view.timeLabel }}</span>
            <NTag v-if="view.isWhisper" size="tiny" :bordered="false" round>
              <template #icon><NIcon><Lock /></NIcon></template>悄悄话
            </NTag>
            <NTag v-else-if="view.isOoc" size="tiny" :bordered="false" round>场外</NTag>
          </div>
          <div class="mf-inspector__source" :title="sourceLabel">{{ sourceLabel }}</div>
          <div class="mf-inspector__content" @click="handleContentClick" v-html="view.contentHtml" />
        </template>
        <template v-else>
          <div class="mf-inspector__meta">
            <NIcon :size="16"><MessageOff /></NIcon>
            <span>{{ tombstoneText }}</span>
          </div>
          <div v-if="sourceLabel" class="mf-inspector__source">{{ sourceLabel }}</div>
        </template>
      </section>

      <section v-if="item.tags.length || canMaintain" class="mf-inspector__tags" data-sc-favorite-part="tags" aria-label="标签">
        <NTag
          v-for="tag in item.tags"
          :key="tag.id"
          size="small"
          round
          checkable
          :checked="activeTagIds.includes(tag.id)"
          :title="activeTagIds.includes(tag.id) ? `取消筛选「${tag.name}」` : `按「${tag.name}」筛选`"
          @update:checked="emit('filter-tag', tag.id)"
        >
          {{ tag.name }}
        </NTag>
        <MessageFavoriteTagPicker v-if="canMaintain" :item="item" :tags="tags" :can-maintain="canMaintain" />
      </section>

      <MessageFavoriteNoteSection
        :item="item"
        :editor="editor"
        :current-user-id="currentUserId"
        :can-edit="canEditNote"
      />
    </div>
  </aside>
</template>

<style scoped>
.mf-inspector {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--sc-overlay-background, var(--sc-bg-elevated));
}

.mf-inspector.is-split {
  flex: 0 0 clamp(280px, 46%, 420px);
  border-inline-start: 1px solid var(--sc-border-mute);
}

.mf-inspector.is-overlay {
  position: absolute;
  z-index: 3;
  inset: 0;
}

.mf-inspector__bar {
  display: flex;
  flex: none;
  align-items: center;
  gap: 4px;
  min-height: 40px;
  padding: 0 6px;
  border-bottom: 1px solid var(--sc-border-mute);
}

.mf-inspector.is-overlay .mf-inspector__bar {
  border-top: 1px solid var(--sc-border-mute);
}

.mf-inspector__title {
  font-size: 13px;
  font-weight: 600;
}

.mf-inspector__spacer {
  flex: 1;
}

.mf-inspector__icon-btn {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--sc-text-secondary);
  cursor: pointer;
}

.mf-inspector__icon-btn:hover:not(:disabled) {
  background: var(--sc-bg-hover);
  color: var(--sc-text-primary);
}

.mf-inspector__icon-btn:disabled {
  cursor: default;
  opacity: 0.5;
}

.mf-inspector__scroll {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 10px 12px 12px;
  scrollbar-width: thin;
  scrollbar-color: var(--sc-scrollbar-thumb) transparent;
}

.mf-inspector__message {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.mf-inspector__message.is-tombstone {
  color: var(--sc-text-secondary);
  font-size: 13px;
}

.mf-inspector__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.mf-inspector__avatar {
  flex: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  object-fit: cover;
}

.mf-inspector__name {
  overflow: hidden;
  min-width: 2.5em;
  color: var(--sc-text-primary);
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mf-inspector__time,
.mf-inspector__source {
  overflow: hidden;
  min-width: 0;
  color: var(--sc-text-secondary);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mf-inspector__content {
  color: var(--sc-text-primary);
  font-size: 13px;
  line-height: 1.65;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.mf-inspector__content :deep(p) {
  margin: 0;
}

.mf-inspector__content :deep(img) {
  display: block;
  max-width: 100%;
  max-height: 320px;
  margin: 4px 0;
  border-radius: 6px;
  object-fit: contain;
}

.mf-inspector__content :deep(a) {
  color: var(--mf-accent);
}

.mf-inspector__tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}
</style>
