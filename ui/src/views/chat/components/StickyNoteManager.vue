<template>
  <div class="sticky-note-manager">
    <template v-if="stickyNoteStore.uiVisible">
      <!-- 渲染所有活跃的便签 -->
      <StickyNote
        v-for="noteId in stickyNoteStore.activeNoteIds"
        :key="noteId"
        :note-id="noteId"
      />

      <!-- 最小化的便签列表 -->
      <Transition name="slide-up">
        <div v-if="minimizedNotes.length > 0" class="sticky-note-minimized-bar">
          <div
            v-for="note in minimizedNotes"
            :key="note.id"
            class="sticky-note-minimized-item"
            :class="`sticky-note-minimized-item--${note.color}`"
            @click="restore(note.id)"
          >
            <span class="sticky-note-minimized-title">
              {{ note.title || '无标题' }}
            </span>
            <button
              class="sticky-note-minimized-close"
              @click.stop="close(note.id)"
            >
              ×
            </button>
          </div>
        </div>
      </Transition>

      <!-- 折叠栏 -->
      <div class="sticky-note-rail">
        <div
          class="sticky-note-rail__panel"
          :class="{ 'sticky-note-rail__panel--open': railOpen }"
          @mouseenter="openRail"
          @mouseleave="closeRail"
        >
          <!-- 便签文字角标 -->
          <div class="sticky-note-rail__badge">便签</div>
          <button
            class="sticky-note-rail__handle"
            title="展开便签"
            @click.stop="toggleRailPinned"
            @mousedown.stop
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M9 18l6-6-6-6v12z"/>
            </svg>
          </button>
          <div class="sticky-note-rail__body">
            <div class="sticky-note-rail__header">
              <span>便签</span>
              <span class="sticky-note-rail__count">{{ stickyNoteStore.noteList.length }}</span>
            </div>
            <div class="sticky-note-rail__actions">
              <div class="sticky-note-rail__action-wrapper">
                <button
                  class="sticky-note-rail__action sticky-note-rail__action--add"
                  title="新建便签"
                  @click="createNote"
                >
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
                  </svg>
                  <span>新建</span>
                </button>
                <!-- 类型选择器弹窗 -->
                <Transition name="fade">
                  <div v-if="showTypeSelector" class="sticky-note-type-popup">
                    <div class="sticky-note-type-popup__backdrop" @click="showTypeSelector = false"></div>
                    <div class="sticky-note-type-popup__content">
                      <StickyNoteTypeSelector @select="handleTypeSelect" />
                    </div>
                  </div>
                </Transition>
              </div>
            </div>
            <div class="sticky-note-rail__list">
              <div
                v-for="note in stickyNoteStore.noteList"
                :key="note.id"
                class="sticky-note-rail__item"
                :class="`sticky-note-rail__item--${note.color}`"
                @click="openNote(note.id)"
              >
                <div class="sticky-note-rail__item-title">
                  {{ note.title || '无标题便签' }}
                </div>
                <div class="sticky-note-rail__item-meta">
                  {{ formatCreator(note) }} · {{ formatDate(note.updatedAt) }}
                </div>
              </div>
              <div v-if="stickyNoteStore.noteList.length === 0" class="sticky-note-rail__empty">
                暂无便签
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useStickyNoteStore, type StickyNoteType } from '@/stores/stickyNote'
import { chatEvent } from '@/stores/chat'
import StickyNote from './StickyNote.vue'
import StickyNoteTypeSelector from './sticky-notes/StickyNoteTypeSelector.vue'

const props = defineProps<{
  channelId: string
}>()

const stickyNoteStore = useStickyNoteStore()

const railOpen = ref(false)
const railPinned = ref(false)
const showTypeSelector = ref(false)

// 计算最小化的便签
const minimizedNotes = computed(() => {
  return stickyNoteStore.activeNoteIds
    .map(id => stickyNoteStore.notes[id])
    .filter(note => note && stickyNoteStore.userStates[note.id]?.minimized)
})

// 格式化日期
function formatDate(timestamp: number): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  const now = new Date()
  const diff = now.getTime() - date.getTime()

  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`

  return date.toLocaleDateString('zh-CN', {
    month: 'short',
    day: 'numeric'
  })
}

function formatCreator(note: { creator?: { nickname?: string; nick?: string; name?: string } }): string {
  const creator = note.creator
  return creator?.nickname || creator?.nick || creator?.name || '未知用户'
}

// 创建新便签
async function createNote() {
  showTypeSelector.value = true
}

// 选择类型后创建便签
async function handleTypeSelect(type: StickyNoteType) {
  showTypeSelector.value = false
  const offset = stickyNoteStore.activeNoteIds.length * 30
  const typeData = stickyNoteStore.getDefaultTypeData(type)
  await stickyNoteStore.createNote({
    title: '',
    content: '',
    color: 'yellow',
    defaultX: 100 + offset,
    defaultY: 100 + offset,
    noteType: type,
    typeData: typeData ? JSON.stringify(typeData) : undefined
  })
}

// 打开便签
function openNote(noteId: string) {
  stickyNoteStore.openNote(noteId)
  if (!railPinned.value) {
    railOpen.value = false
  }
}

// 恢复最小化的便签
function restore(noteId: string) {
  stickyNoteStore.restoreNote(noteId)
}

// 关闭便签
function close(noteId: string) {
  stickyNoteStore.closeNote(noteId)
}

// 监听频道变化
watch(() => props.channelId, (newChannelId) => {
  if (newChannelId) {
    stickyNoteStore.loadChannelNotes(newChannelId)
  }
}, { immediate: true })

function openRail() {
  railOpen.value = true
}

function closeRail() {
  if (!railPinned.value) {
    railOpen.value = false
  }
}

function toggleRailPinned() {
  railPinned.value = !railPinned.value
  railOpen.value = railPinned.value
}

// 监听WebSocket事件
function handleEvent(event: any) {
  if (event.type?.startsWith('sticky-note-')) {
    stickyNoteStore.handleStickyNoteEvent(event)
  }
}

onMounted(() => {
  // 订阅WebSocket事件
  chatEvent.on('sticky-note-created', handleEvent)
  chatEvent.on('sticky-note-updated', handleEvent)
  chatEvent.on('sticky-note-deleted', handleEvent)
  chatEvent.on('sticky-note-pushed', handleEvent)
})

onUnmounted(() => {
  // 取消订阅
  chatEvent.off('sticky-note-created', handleEvent)
  chatEvent.off('sticky-note-updated', handleEvent)
  chatEvent.off('sticky-note-deleted', handleEvent)
  chatEvent.off('sticky-note-pushed', handleEvent)
})
</script>

<style scoped>
.sticky-note-manager {
  pointer-events: none;
}

.sticky-note-manager > * {
  pointer-events: auto;
}

/* 折叠栏 */
.sticky-note-rail {
  position: fixed;
  right: 0;
  top: 50%;
  z-index: 999;
  pointer-events: none;
}

.sticky-note-rail__panel {
  position: absolute;
  right: 0;
  top: 0;
  transform: translateY(-50%) translateX(calc(100% - 16px));
  transition: transform 0.2s ease;
  pointer-events: auto;
}

.sticky-note-rail__panel--open {
  transform: translateY(-50%) translateX(0);
}

.sticky-note-rail__handle {
  position: absolute;
  right: -8px;
  top: 18px;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 10px 0 0 10px;
  background: var(--sc-primary-color, #3b82f6);
  color: var(--sc-primary-contrast, #fff);
  cursor: pointer;
  box-shadow: 0 4px 10px rgba(var(--sc-primary-rgb, 59, 130, 246), 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 便签文字角标 - 显示在面板左侧外部，竖排文字 */
.sticky-note-rail__badge {
  position: absolute;
  left: 0;
  top: 10px;
  transform: translateX(-100%);
  writing-mode: vertical-rl;
  text-orientation: mixed;
  padding: 8px 5px;
  background: var(--sc-bg-surface, #f8fafc);
  color: var(--sc-text-primary, #0f172a);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 2px;
  border-radius: 6px 0 0 6px;
  box-shadow: -2px 0 8px rgba(0, 0, 0, 0.1);
  pointer-events: none;
  user-select: none;
}

.sticky-note-rail__body {
  width: 240px;
  background: var(--sc-bg-elevated, #ffffff);
  border: 1px solid var(--sc-border-strong, rgba(15, 23, 42, 0.12));
  border-right: none;
  border-radius: 12px 0 0 12px;
  box-shadow: 0 10px 26px rgba(0, 0, 0, 0.12);
  overflow: hidden;
}

.sticky-note-rail__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: var(--sc-bg-surface, #f8fafc);
  color: var(--sc-text-primary, #0f172a);
  font-size: 13px;
  font-weight: 600;
}

.sticky-note-rail__count {
  font-size: 11px;
  color: var(--sc-text-secondary, #64748b);
}

.sticky-note-rail__actions {
  display: flex;
  padding: 10px 12px;
  border-bottom: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
  gap: 8px;
}

.sticky-note-rail__action-wrapper {
  position: relative;
}

.sticky-note-rail__action {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 10px;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
  background: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.sticky-note-rail__action--add {
  border-color: rgba(var(--sc-primary-rgb, 59, 130, 246), 0.45);
  color: var(--sc-primary-color, #3b82f6);
  background: rgba(var(--sc-primary-rgb, 59, 130, 246), 0.08);
}

.sticky-note-rail__action--add:hover {
  background: rgba(var(--sc-primary-rgb, 59, 130, 246), 0.16);
}

.sticky-note-rail__list {
  max-height: 360px;
  overflow-y: auto;
}

.sticky-note-rail__item {
  padding: 10px 14px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: background 0.15s ease;
}

.sticky-note-rail__item:hover {
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.06));
}

.sticky-note-rail__item--yellow { border-left-color: #ffc107; }
.sticky-note-rail__item--pink { border-left-color: #e91e63; }
.sticky-note-rail__item--green { border-left-color: #4caf50; }
.sticky-note-rail__item--blue { border-left-color: #2196f3; }
.sticky-note-rail__item--purple { border-left-color: #9c27b0; }
.sticky-note-rail__item--orange { border-left-color: #ff9800; }

.sticky-note-rail__item-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--sc-text-primary, #0f172a);
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sticky-note-rail__item-meta {
  font-size: 11px;
  color: var(--sc-text-secondary, #64748b);
}

.sticky-note-rail__empty {
  padding: 28px 16px;
  text-align: center;
  color: var(--sc-text-secondary, #94a3b8);
  font-size: 12px;
}

/* 最小化便签栏 */
.sticky-note-minimized-bar {
  position: fixed;
  right: 24px;
  bottom: 140px;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
  padding: 8px;
  background: rgba(255, 255, 255, 0.95);
  border-radius: 12px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  z-index: 998;
  max-height: 45vh;
  overflow-y: auto;
}

.sticky-note-minimized-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 12px;
  cursor: pointer;
  font-size: 12px;
  transition: transform 0.15s;
  min-width: 140px;
  justify-content: space-between;
}

.sticky-note-minimized-item:hover {
  transform: translateX(-2px);
}

.sticky-note-minimized-item--yellow { background: #fff9c4; }
.sticky-note-minimized-item--pink { background: #f8bbd9; }
.sticky-note-minimized-item--green { background: #c8e6c9; }
.sticky-note-minimized-item--blue { background: #bbdefb; }
.sticky-note-minimized-item--purple { background: #e1bee7; }
.sticky-note-minimized-item--orange { background: #ffe0b2; }

.sticky-note-minimized-title {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-minimized-close {
  width: 18px;
  height: 18px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  color: rgba(0, 0, 0, 0.5);
}

.sticky-note-minimized-close:hover {
  background: rgba(0, 0, 0, 0.2);
}

/* 动画 */
.slide-enter-active,
.slide-leave-active {
  transition: all 0.3s ease;
}

.slide-enter-from,
.slide-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.95);
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s ease;
}

.slide-up-enter-from,
.slide-up-leave-to {
  opacity: 0;
  transform: translateX(20px) translateY(10px);
}

/* 类型选择器弹窗 */
.sticky-note-type-popup {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  z-index: 10;
  margin-top: 4px;
}

.sticky-note-type-popup__backdrop {
  position: fixed;
  inset: 0;
  z-index: -1;
}

.sticky-note-type-popup__content {
  background: var(--sc-bg-elevated, #ffffff);
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.1));
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.15);
}

/* 夜间模式弹窗适配 */
:root[data-display-palette='night'] .sticky-note-type-popup__content {
  background: var(--sc-bg-elevated, #2a2a2e);
  border-color: var(--sc-border-mute, rgba(255, 255, 255, 0.1));
}

:root[data-custom-theme='true'] .sticky-note-type-popup__content {
  background: var(--sc-bg-elevated, #ffffff);
  border-color: var(--sc-border-mute, rgba(0, 0, 0, 0.1));
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>

<style>
/* ===== 便签角标主题适配（非 scoped） ===== */
/* 夜间模式 */
:root[data-display-palette='night'] .sticky-note-rail__badge {
  background: #26262c;
  color: #e2e8f0;
}

/* 自定义主题 - 使用自定义背景色 */
:root[data-custom-theme='true'] .sticky-note-rail__badge {
  background: var(--sc-bg-surface, #26262c);
  color: var(--sc-text-primary, #e2e8f0);
}
</style>
