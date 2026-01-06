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
          ref="railPanelRef"
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
              <button
                class="sticky-note-rail__action"
                title="新建文件夹"
                @click="showFolderInput = true"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
                </svg>
              </button>
            </div>

            <!-- 新建文件夹输入 -->
            <div v-if="showFolderInput" class="sticky-note-rail__folder-input">
              <input
                v-model="newFolderName"
                type="text"
                placeholder="文件夹名称"
                @keyup.enter="createFolder"
                @keyup.escape="cancelFolderInput"
              />
              <button @click="createFolder">✓</button>
              <button @click="cancelFolderInput">✕</button>
            </div>

            <div class="sticky-note-rail__list">
              <!-- 未分类便签 -->
              <template v-if="uncategorizedNotes.length > 0">
                <div
                  v-for="note in uncategorizedNotes"
                  :key="note.id"
                  class="sticky-note-rail__item"
                  :class="`sticky-note-rail__item--${note.color}`"
                  draggable="true"
                  @click="openNote(note.id)"
                  @dragstart="onNoteDragStart($event, note.id)"
                  @dragover.prevent
                  @drop="onNoteDrop($event, '')"
                >
                  <div class="sticky-note-rail__item-title">
                    {{ note.title || '无标题便签' }}
                  </div>
                  <div class="sticky-note-rail__item-meta">
                    {{ formatCreator(note) }} · {{ formatDate(note.updatedAt) }}
                  </div>
                </div>
              </template>

              <!-- 文件夹 -->
              <div
                v-for="folder in stickyNoteStore.folderList"
                :key="folder.id"
                class="sticky-note-rail__folder"
                @dragover.prevent
                @drop="onNoteDrop($event, folder.id)"
              >
                <div
                  class="sticky-note-rail__folder-header"
                  @click="toggleFolder(folder.id)"
                  @mouseenter="hoveredFolderId = folder.id"
                  @mouseleave="hoveredFolderId = null"
                >
                  <svg
                    class="sticky-note-rail__folder-icon"
                    :class="{ 'sticky-note-rail__folder-icon--open': expandedFolders.has(folder.id) }"
                    width="12" height="12" viewBox="0 0 24 24" fill="currentColor"
                  >
                    <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z"/>
                  </svg>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" :style="{ color: folder.color || '#ffc107' }">
                    <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
                  </svg>
                  <!-- 编辑状态 -->
                  <input
                    v-if="editingFolderId === folder.id"
                    v-model="editingFolderName"
                    type="text"
                    class="sticky-note-rail__folder-name-input"
                    @click.stop
                    @keyup.enter="saveFolderName(folder.id)"
                    @keyup.escape="cancelFolderEdit"
                    @blur="saveFolderName(folder.id)"
                  />
                  <span
                    v-else
                    class="sticky-note-rail__folder-name"
                  >{{ folder.name }}</span>
                  <span class="sticky-note-rail__folder-count">{{ getFolderNoteCount(folder.id) }}</span>
                  <!-- 悬浮操作按钮 -->
                  <div v-if="hoveredFolderId === folder.id" class="sticky-note-rail__folder-actions">
                    <button
                      class="sticky-note-rail__folder-action"
                      title="重命名"
                      @click.stop="startFolderEdit(folder)"
                    >
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04c.39-.39.39-1.02 0-1.41l-2.34-2.34c-.39-.39-1.02-.39-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/>
                      </svg>
                    </button>
                    <button
                      class="sticky-note-rail__folder-action"
                      title="设置颜色"
                      @click.stop="toggleFolderColorPicker(folder.id, $event)"
                    >
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M12 3c-4.97 0-9 4.03-9 9s4.03 9 9 9c.83 0 1.5-.67 1.5-1.5 0-.39-.15-.74-.39-1.01-.23-.26-.38-.61-.38-.99 0-.83.67-1.5 1.5-1.5H16c2.76 0 5-2.24 5-5 0-4.42-4.03-8-9-8zm-5.5 9c-.83 0-1.5-.67-1.5-1.5S5.67 9 6.5 9 8 9.67 8 10.5 7.33 12 6.5 12zm3-4C8.67 8 8 7.33 8 6.5S8.67 5 9.5 5s1.5.67 1.5 1.5S10.33 8 9.5 8zm5 0c-.83 0-1.5-.67-1.5-1.5S13.67 5 14.5 5s1.5.67 1.5 1.5S15.33 8 14.5 8zm3 4c-.83 0-1.5-.67-1.5-1.5S16.67 9 17.5 9s1.5.67 1.5 1.5-.67 1.5-1.5 1.5z"/>
                      </svg>
                    </button>
                    <button
                      class="sticky-note-rail__folder-action"
                      title="推送文件夹"
                      @click.stop="openFolderPushPopup(folder.id, $event)"
                    >
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/>
                      </svg>
                    </button>
                    <button
                      class="sticky-note-rail__folder-action sticky-note-rail__folder-action--delete"
                      title="删除文件夹"
                      @click.stop="deleteFolder(folder.id)"
                    >×</button>
                  </div>
                </div>
                <Teleport to="body">
                  <!-- 颜色选择器 -->
                  <div
                    v-if="colorPickerFolderId === folder.id"
                    :ref="setColorPickerRef"
                    class="sticky-note-rail__color-picker"
                    :style="colorPickerStyle"
                    @click.stop
                  >
                    <button
                      v-for="color in folderColors"
                      :key="color.value"
                      class="sticky-note-rail__color-option"
                      :style="{ background: color.value }"
                      :title="color.label"
                      @click="setFolderColor(folder.id, color.value)"
                    ></button>
                  </div>
                  <!-- 推送用户选择弹窗 -->
                  <div
                    v-if="pushFolderId === folder.id"
                    :ref="setPushPopupRef"
                    class="sticky-note-rail__push-popup"
                    :style="pushPopupStyle"
                    @click.stop
                  >
                    <div class="sticky-note-rail__push-header">
                      <span>推送到 ({{ folderPushTargets.length }}/{{ pushTargetOptions.length }})</span>
                      <label class="sticky-note-rail__push-check-all">
                        <input type="checkbox" :checked="isAllSelected" @change="toggleSelectAll" />
                        全选
                      </label>
                    </div>
                    <div class="sticky-note-rail__push-list">
                      <label
                        v-for="user in pushTargetOptions"
                        :key="user.id"
                        class="sticky-note-rail__push-item"
                      >
                        <input
                          type="checkbox"
                          :value="user.id"
                          v-model="folderPushTargets"
                        />
                        {{ user.nick || user.name || user.id }}
                      </label>
                      <div v-if="pushTargetOptions.length === 0" class="sticky-note-rail__push-empty">
                        暂无可推送的用户
                      </div>
                    </div>
                    <div class="sticky-note-rail__push-actions">
                      <button class="sticky-note-rail__push-cancel" @click="closeFolderPushPopup">取消</button>
                      <button class="sticky-note-rail__push-confirm" @click="confirmPushFolder">推送</button>
                    </div>
                  </div>
                </Teleport>
                <div
                  v-if="expandedFolders.has(folder.id)"
                  class="sticky-note-rail__folder-content"
                >
                  <div
                    v-for="note in getNotesByFolder(folder.id)"
                    :key="note.id"
                    class="sticky-note-rail__item sticky-note-rail__item--nested"
                    :class="`sticky-note-rail__item--${note.color}`"
                    draggable="true"
                    @click="openNote(note.id)"
                    @dragstart="onNoteDragStart($event, note.id)"
                  >
                    <div class="sticky-note-rail__item-title">
                      {{ note.title || '无标题便签' }}
                    </div>
                    <div class="sticky-note-rail__item-meta">
                      {{ formatCreator(note) }} · {{ formatDate(note.updatedAt) }}
                    </div>
                  </div>
                  <div v-if="getNotesByFolder(folder.id).length === 0" class="sticky-note-rail__folder-empty">
                    拖拽便签到此处
                  </div>
                </div>
              </div>

              <div v-if="stickyNoteStore.noteList.length === 0 && stickyNoteStore.folderList.length === 0" class="sticky-note-rail__empty">
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
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useStickyNoteStore, type StickyNoteType } from '@/stores/stickyNote'
import { chatEvent, useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import StickyNote from './StickyNote.vue'
import StickyNoteTypeSelector from './sticky-notes/StickyNoteTypeSelector.vue'

const props = defineProps<{
  channelId: string
}>()

const stickyNoteStore = useStickyNoteStore()
const chatStore = useChatStore()
const userStore = useUserStore()

const railOpen = ref(false)
const railPinned = ref(false)
const showTypeSelector = ref(false)
const showFolderInput = ref(false)
const newFolderName = ref('')
const expandedFolders = ref<Set<string>>(new Set())
const draggingNoteId = ref<string | null>(null)
const hoveredFolderId = ref<string | null>(null)
const editingFolderId = ref<string | null>(null)
const editingFolderName = ref('')
const colorPickerFolderId = ref<string | null>(null)
const colorPickerStyle = ref<Record<string, string>>({})
const colorPickerAnchor = ref<DOMRect | null>(null)
const colorPickerRef = ref<HTMLElement | null>(null)
const pushFolderId = ref<string | null>(null)
const folderPushTargets = ref<string[]>([])
const pushPopupStyle = ref<Record<string, string>>({})
const pushPopupAnchor = ref<DOMRect | null>(null)
const pushPopupRef = ref<HTMLElement | null>(null)
const railPanelRef = ref<HTMLElement | null>(null)

function setColorPickerRef(el: HTMLElement | null) {
  colorPickerRef.value = el
}

function setPushPopupRef(el: HTMLElement | null) {
  pushPopupRef.value = el
}

// 文件夹颜色选项
const folderColors = [
  { value: '#ffc107', label: '黄色' },
  { value: '#4caf50', label: '绿色' },
  { value: '#2196f3', label: '蓝色' },
  { value: '#e91e63', label: '粉色' },
  { value: '#9c27b0', label: '紫色' },
  { value: '#ff9800', label: '橙色' },
  { value: '#607d8b', label: '灰色' },
  { value: '#f44336', label: '红色' }
]

// 计算最小化的便签
const minimizedNotes = computed(() => {
  return stickyNoteStore.activeNoteIds
    .map(id => stickyNoteStore.notes[id])
    .filter(note => note && stickyNoteStore.userStates[note.id]?.minimized)
})

// 未分类便签
const uncategorizedNotes = computed(() => {
  return stickyNoteStore.noteList.filter(note => !note.folderId)
})

// 获取文件夹内的便签
function getNotesByFolder(folderId: string) {
  return stickyNoteStore.noteList.filter(note => note.folderId === folderId)
}

// 获取文件夹内便签数量
function getFolderNoteCount(folderId: string) {
  return getNotesByFolder(folderId).length
}

// 切换文件夹展开状态
function toggleFolder(folderId: string) {
  if (expandedFolders.value.has(folderId)) {
    expandedFolders.value.delete(folderId)
  } else {
    expandedFolders.value.add(folderId)
  }
}

// 创建文件夹
async function createFolder() {
  if (!newFolderName.value.trim()) return
  await stickyNoteStore.createFolder({ name: newFolderName.value.trim() })
  cancelFolderInput()
}

// 取消文件夹输入
function cancelFolderInput() {
  showFolderInput.value = false
  newFolderName.value = ''
}

// 删除文件夹
async function deleteFolder(folderId: string) {
  if (confirm('确定删除此文件夹？文件夹内的便签将移出。')) {
    await stickyNoteStore.deleteFolder(folderId)
    expandedFolders.value.delete(folderId)
  }
}

// 开始编辑文件夹名称
function startFolderEdit(folder: { id: string; name: string }) {
  editingFolderId.value = folder.id
  editingFolderName.value = folder.name
  colorPickerFolderId.value = null
}

// 保存文件夹名称
async function saveFolderName(folderId: string) {
  if (editingFolderId.value !== folderId) return
  const newName = editingFolderName.value.trim()
  if (newName) {
    await stickyNoteStore.updateFolder(folderId, { name: newName })
  }
  cancelFolderEdit()
}

// 取消文件夹编辑
function cancelFolderEdit() {
  editingFolderId.value = null
  editingFolderName.value = ''
}

// 切换颜色选择器
function toggleFolderColorPicker(folderId: string, event?: MouseEvent) {
  if (colorPickerFolderId.value === folderId) {
    colorPickerFolderId.value = null
    colorPickerStyle.value = {}
    colorPickerAnchor.value = null
  } else {
    colorPickerFolderId.value = folderId
    pushFolderId.value = null
    pushPopupStyle.value = {}
    pushPopupAnchor.value = null
    const trigger = event?.currentTarget as HTMLElement | null
    colorPickerAnchor.value = trigger ? trigger.getBoundingClientRect() : null
    nextTick(() => {
      const anchor = colorPickerAnchor.value
      const picker = colorPickerRef.value
      if (!anchor || !picker) return
      const panelRect = railPanelRef.value?.getBoundingClientRect()
      const gap = 8
      const width = picker.offsetWidth
      const height = picker.offsetHeight
      const leftBase = panelRect ? panelRect.left - width - gap : anchor.left
      const left = Math.max(8, leftBase)
      let top = anchor.top - height - gap
      if (top < 8) {
        top = anchor.bottom + gap
      }
      colorPickerStyle.value = {
        top: `${top}px`,
        left: `${left}px`
      }
    })
  }
}

// 设置文件夹颜色
async function setFolderColor(folderId: string, color: string) {
  await stickyNoteStore.updateFolder(folderId, { color })
  colorPickerFolderId.value = null
}

// 推送目标用户列表
const pushTargetOptions = computed(() => {
  const currentUserId = userStore.info?.id
  return (chatStore.curChannelUsers || [])
    .filter(user => user?.id && user.id !== currentUserId)
})

// 是否全选
const isAllSelected = computed(() => {
  return pushTargetOptions.value.length > 0 &&
    pushTargetOptions.value.every(user => folderPushTargets.value.includes(user.id))
})

// 全选/取消全选
function toggleSelectAll() {
  if (isAllSelected.value) {
    folderPushTargets.value = []
  } else {
    folderPushTargets.value = pushTargetOptions.value.map(user => user.id)
  }
}

// 打开推送弹窗
function openFolderPushPopup(folderId: string, event?: MouseEvent) {
  const notes = getNotesByFolder(folderId)
  if (notes.length === 0) {
    alert('文件夹内没有便签')
    return
  }
  pushFolderId.value = folderId
  folderPushTargets.value = []
  colorPickerFolderId.value = null
  colorPickerStyle.value = {}
  colorPickerAnchor.value = null
  const trigger = event?.currentTarget as HTMLElement | null
  pushPopupAnchor.value = trigger ? trigger.getBoundingClientRect() : null
  nextTick(() => {
    const popup = pushPopupRef.value
    if (!popup) return
    const panelRect = railPanelRef.value?.getBoundingClientRect()
    const gap = 8
    const width = popup.offsetWidth
    const height = popup.offsetHeight
    const anchorRect = pushPopupAnchor.value
    const leftBase = panelRect ? panelRect.left - width - gap : (anchorRect ? anchorRect.left - width - gap : 8)
    const left = Math.max(8, leftBase)
    const maxTop = Math.max(8, window.innerHeight - height - 8)
    const topBase = panelRect ? panelRect.top : (anchorRect ? anchorRect.top : 8)
    const top = Math.min(Math.max(8, topBase), maxTop)
    pushPopupStyle.value = {
      top: `${top}px`,
      left: `${left}px`
    }
  })
}

// 关闭推送弹窗
function closeFolderPushPopup() {
  pushFolderId.value = null
  folderPushTargets.value = []
  pushPopupStyle.value = {}
  pushPopupAnchor.value = null
}

// 确认推送
async function confirmPushFolder() {
  if (!pushFolderId.value || folderPushTargets.value.length === 0) {
    alert('请选择推送对象')
    return
  }
  const notes = getNotesByFolder(pushFolderId.value)
  for (const note of notes) {
    await stickyNoteStore.pushNote(note.id, folderPushTargets.value)
  }
  closeFolderPushPopup()
}

// 拖拽开始
function onNoteDragStart(event: DragEvent, noteId: string) {
  draggingNoteId.value = noteId
  event.dataTransfer?.setData('text/plain', noteId)
}

// 拖拽放置
async function onNoteDrop(event: DragEvent, folderId: string) {
  const noteId = draggingNoteId.value || event.dataTransfer?.getData('text/plain')
  if (noteId) {
    await stickyNoteStore.moveNoteToFolder(noteId, folderId || null)
  }
  draggingNoteId.value = null
}

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
  min-height: 240px;
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

/* 文件夹输入 */
.sticky-note-rail__folder-input {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
}

.sticky-note-rail__folder-input input {
  flex: 1;
  padding: 4px 8px;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.2));
  border-radius: 4px;
  font-size: 12px;
  background: var(--sc-bg-elevated, #fff);
  color: var(--sc-text-primary, #0f172a);
}

.sticky-note-rail__folder-input button {
  padding: 4px 8px;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.06));
  color: var(--sc-text-primary, #0f172a);
}

/* 文件夹样式 */
.sticky-note-rail__folder {
  position: relative;
  border-bottom: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.06));
}

.sticky-note-rail__folder-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.sticky-note-rail__folder-header:hover {
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.04));
}

.sticky-note-rail__folder-icon {
  transition: transform 0.2s ease;
  color: var(--sc-text-secondary, #64748b);
}

.sticky-note-rail__folder-icon--open {
  transform: rotate(90deg);
}

.sticky-note-rail__folder-name {
  flex: 1;
  font-size: 12px;
  font-weight: 500;
  color: var(--sc-text-primary, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sticky-note-rail__folder-count {
  font-size: 10px;
  color: var(--sc-text-secondary, #94a3b8);
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.06));
  padding: 1px 5px;
  border-radius: 8px;
}

.sticky-note-rail__folder-delete {
  opacity: 0;
  width: 16px;
  height: 16px;
  border: none;
  background: transparent;
  color: var(--sc-text-secondary, #94a3b8);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  transition: opacity 0.15s, color 0.15s;
}

.sticky-note-rail__folder-header:hover .sticky-note-rail__folder-delete {
  opacity: 1;
}

.sticky-note-rail__folder-delete:hover {
  color: #ef4444;
}

.sticky-note-rail__folder-content {
  background: var(--sc-bg-surface, rgba(15, 23, 42, 0.02));
}

.sticky-note-rail__folder-empty {
  padding: 12px 20px;
  text-align: center;
  color: var(--sc-text-secondary, #94a3b8);
  font-size: 11px;
  font-style: italic;
}

/* 文件夹名称编辑输入框 */
.sticky-note-rail__folder-name-input {
  flex: 1;
  padding: 2px 6px;
  font-size: 12px;
  font-weight: 500;
  border: 1px solid var(--sc-border-focus, #3b82f6);
  border-radius: 4px;
  background: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  outline: none;
}

.sticky-note-rail__folder-name-input:focus {
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
}

/* 文件夹操作按钮容器 */
.sticky-note-rail__folder-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.sticky-note-rail__folder-header:hover .sticky-note-rail__folder-actions {
  opacity: 1;
}

/* 单个操作按钮 */
.sticky-note-rail__folder-action {
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--sc-text-secondary, #94a3b8);
  cursor: pointer;
  font-size: 12px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}

.sticky-note-rail__folder-action:hover {
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.08));
  color: var(--sc-text-primary, #0f172a);
}

.sticky-note-rail__folder-action--delete:hover {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
}

/* 颜色选择器下拉框 */
.sticky-note-rail__color-picker {
  position: fixed;
  z-index: 1100;
  display: grid;
  grid-template-columns: repeat(4, 16px);
  gap: 6px;
  padding: 6px;
  background: var(--sc-bg-elevated, #ffffff);
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

/* 颜色选项按钮 */
.sticky-note-rail__color-option {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 0.15s, border-color 0.15s;
}

.sticky-note-rail__color-option:hover {
  transform: scale(1.15);
  border-color: rgba(255, 255, 255, 0.5);
}

/* 推送用户选择弹窗 */
.sticky-note-rail__push-popup {
  position: fixed;
  z-index: 1101;
  width: 180px;
  background: var(--sc-bg-elevated, #ffffff);
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
  border-radius: 8px;
  box-shadow: -4px 4px 16px rgba(0, 0, 0, 0.15);
  padding: 8px;
  margin-right: 4px;
  max-height: 220px;
  display: flex;
  flex-direction: column;
}

.sticky-note-rail__push-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--sc-text-primary, #0f172a);
  flex-shrink: 0;
}

.sticky-note-rail__push-check-all {
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  font-size: 11px;
  color: var(--sc-text-secondary, #64748b);
}

.sticky-note-rail__push-check-all input {
  cursor: pointer;
}

.sticky-note-rail__push-list {
  flex: 1;
  min-height: 0;
  max-height: 120px;
  overflow-y: auto;
  margin-bottom: 8px;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.06));
  border-radius: 4px;
  padding: 4px;
}

.sticky-note-rail__push-list::-webkit-scrollbar {
  width: 4px;
}

.sticky-note-rail__push-list::-webkit-scrollbar-thumb {
  background: var(--sc-border-mute, rgba(15, 23, 42, 0.2));
  border-radius: 2px;
}

.sticky-note-rail__push-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 0;
  font-size: 12px;
  color: var(--sc-text-primary, #0f172a);
  cursor: pointer;
}

.sticky-note-rail__push-item:hover {
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.04));
}

.sticky-note-rail__push-item input {
  cursor: pointer;
}

.sticky-note-rail__push-empty {
  text-align: center;
  padding: 12px;
  color: var(--sc-text-secondary, #94a3b8);
  font-size: 11px;
}

.sticky-note-rail__push-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-shrink: 0;
}

.sticky-note-rail__push-cancel,
.sticky-note-rail__push-confirm {
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
  border: none;
}

.sticky-note-rail__push-cancel {
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.06));
  color: var(--sc-text-secondary, #64748b);
}

.sticky-note-rail__push-cancel:hover {
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.1));
}

.sticky-note-rail__push-confirm {
  background: #3b82f6;
  color: #ffffff;
}

.sticky-note-rail__push-confirm:hover {
  background: #2563eb;
}

.sticky-note-rail__item--nested {
  padding-left: 28px;
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
