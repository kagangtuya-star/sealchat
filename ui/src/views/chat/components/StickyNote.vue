<template>
  <Teleport to="body">
    <div
      v-if="note && !userState?.minimized"
      ref="noteEl"
      class="sticky-note"
      :class="[
        `sticky-note--${note.color || 'yellow'}`,
        { 'sticky-note--editing': isEditing }
      ]"
      :style="noteStyle"
      @mousedown="handleMouseDown"
    >
      <!-- 头部 -->
      <div
        ref="headerEl"
        class="sticky-note__header"
        @mousedown.left="startDrag"
      >
        <div class="sticky-note__title">
          <input
            v-if="isEditing"
            v-model="localTitle"
            class="sticky-note__title-input"
            placeholder="便签标题"
            @blur="saveTitle"
            @keyup.enter="saveTitle"
          />
          <span v-else class="sticky-note__title-text">
            {{ note.title || '无标题便签' }}
          </span>
        </div>
        <div class="sticky-note__actions">
          <button
            class="sticky-note__action-btn"
            title="编辑"
            @click="toggleEdit"
            @mousedown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04c.39-.39.39-1.02 0-1.41l-2.34-2.34c-.39-.39-1.02-.39-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/>
            </svg>
          </button>
          <n-popover
            v-model:show="pushPopoverVisible"
            trigger="click"
            placement="bottom-end"
            :show-arrow="false"
          >
            <template #trigger>
              <button
                class="sticky-note__action-btn"
                title="推送"
                @mousedown.stop
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M4 12v7a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-7h-2v6H6v-6H4zm8-9l5 5h-3v6h-4V8H7l5-5z"/>
                </svg>
              </button>
            </template>
            <div class="sticky-note__push-panel" @mousedown.stop>
              <div class="sticky-note__push-title">推送便签</div>
              <div class="sticky-note__push-toolbar">
                <n-checkbox v-model:checked="checkAll" :disabled="allTargetIds.length === 0">
                  全选
                </n-checkbox>
                <span class="sticky-note__push-count">
                  {{ pushTargets.length }}/{{ allTargetIds.length }}
                </span>
              </div>
              <n-select
                v-model:value="pushTargets"
                :options="pushOptions"
                multiple
                size="small"
                placeholder="选择成员"
              />
              <div class="sticky-note__push-actions">
                <n-button
                  size="tiny"
                  type="primary"
                  :disabled="pushTargets.length === 0"
                  @click="pushToTargets"
                >
                  推送
                </n-button>
              </div>
            </div>
          </n-popover>
          <button
            class="sticky-note__action-btn"
            title="复制内容"
            @click="copyContent"
            @mousedown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
            </svg>
          </button>
          <button
            class="sticky-note__action-btn"
            title="最小化"
            @click="minimize"
            @mousedown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M6 19h12v2H6z"/>
            </svg>
          </button>
          <button
            v-if="isOwner"
            class="sticky-note__action-btn"
            title="删除"
            @click="deleteNote"
            @mousedown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M6 7h12v2H6V7zm2 3h8l-1 10H9L8 10zm3-5h2l1 1h5v2H4V6h5l1-1z"/>
            </svg>
          </button>
          <button
            class="sticky-note__action-btn sticky-note__action-btn--close"
            title="关闭"
            @click="close"
            @mousedown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- 内容区域 -->
      <div class="sticky-note__body">
        <div
          v-if="isEditing"
          class="sticky-note__editor"
        >
          <textarea
            v-model="localContent"
            class="sticky-note__textarea"
            placeholder="在此输入内容..."
            @input="debouncedSaveContent"
          ></textarea>
        </div>
        <div
          v-else
          class="sticky-note__content"
          v-html="sanitizedContent"
        ></div>
      </div>

      <!-- 底部信息 -->
      <div class="sticky-note__footer">
        <div class="sticky-note__meta">
          <span class="sticky-note__meta-label">编辑者</span>
          <span class="sticky-note__meta-value">{{ creatorName }}</span>
          <div class="sticky-note__colors" v-if="isEditing">
            <button
              v-for="color in colors"
              :key="color"
              class="sticky-note__color-btn"
              :class="{ 'sticky-note__color-btn--active': note.color === color }"
              :style="{ backgroundColor: getColorValue(color) }"
              @click="changeColor(color)"
            ></button>
          </div>
        </div>
        <span class="sticky-note__meta-time">
          {{ formatTime(note.updatedAt) }}
        </span>
      </div>

      <!-- 调整大小手柄 -->
      <div
        class="sticky-note__resize-handle"
        @mousedown.left.stop="startResize"
      ></div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useStickyNoteStore, type StickyNote, type StickyNoteUserState } from '@/stores/stickyNote'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'

const props = defineProps<{
  noteId: string
}>()

const stickyNoteStore = useStickyNoteStore()
const chatStore = useChatStore()
const userStore = useUserStore()
const message = useMessage()

const noteEl = ref<HTMLElement | null>(null)
const headerEl = ref<HTMLElement | null>(null)

// 本地编辑状态
const localTitle = ref('')
const localContent = ref('')
const pushPopoverVisible = ref(false)
const pushTargets = ref<string[]>([])

// 拖拽状态
const isDragging = ref(false)
const dragOffset = ref({ x: 0, y: 0 })

// 调整大小状态
const isResizing = ref(false)
const resizeStart = ref({ x: 0, y: 0, w: 0, h: 0 })

// 颜色选项
const colors = ['yellow', 'pink', 'green', 'blue', 'purple', 'orange']

// 计算属性
const note = computed<StickyNote | undefined>(() =>
  stickyNoteStore.notes[props.noteId]
)

const userState = computed<StickyNoteUserState | undefined>(() =>
  stickyNoteStore.userStates[props.noteId]
)

const isEditing = computed(() =>
  stickyNoteStore.editingNoteId === props.noteId
)

const isOwner = computed(() => {
  const userId = userStore.info?.id
  if (!userId) return false
  return note.value?.creatorId === userId || note.value?.creator?.id === userId
})

const creatorName = computed(() => {
  const creator = note.value?.creator
  return creator?.nickname || creator?.nick || creator?.name || '未知用户'
})

function buildPushTargetsKey() {
  const userId = userStore.info?.id
  const channelId = chatStore.curChannel?.id
  if (!userId || !channelId) return ''
  return `sticky-note-push-targets:${userId}:${channelId}`
}

function readPushTargets(): string[] {
  if (typeof window === 'undefined') return []
  const key = buildPushTargetsKey()
  if (!key) return []
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((id): id is string => typeof id === 'string')
  } catch {
    return []
  }
}

function writePushTargets(value: string[]) {
  if (typeof window === 'undefined') return
  const key = buildPushTargetsKey()
  if (!key) return
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // ignore
  }
}

const pushOptions = computed(() => {
  const currentUserId = chatStore.curUser?.id
  return (chatStore.curChannelUsers || [])
    .filter(user => user?.id && user.id !== currentUserId)
    .map(user => ({
      label: user.nick || user.name || user.id,
      value: user.id
    }))
})

const allTargetIds = computed(() => pushOptions.value.map(option => option.value))

const checkAll = computed({
  get: () => allTargetIds.value.length > 0 && allTargetIds.value.every(id => pushTargets.value.includes(id)),
  set: (value: boolean) => {
    pushTargets.value = value ? allTargetIds.value.slice() : []
  }
})

function loadPushTargets() {
  const stored = readPushTargets()
  const validIds = allTargetIds.value
  if (stored.length > 0) {
    pushTargets.value = stored.filter(id => validIds.includes(id))
    return
  }
  pushTargets.value = pushTargets.value.filter(id => validIds.includes(id))
}

const sanitizedContent = computed(() => {
  const content = note.value?.content || ''
  // 简单的HTML转义，实际项目应使用DOMPurify
  return content
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
})

const noteStyle = computed(() => {
  const state = userState.value
  const n = note.value

  const x = state?.positionX || n?.defaultX || 100
  const y = state?.positionY || n?.defaultY || 100
  const w = state?.width || n?.defaultW || 300
  const h = state?.height || n?.defaultH || 250
  const z = state?.zIndex || 1000

  return {
    left: `${x}px`,
    top: `${y}px`,
    width: `${w}px`,
    height: `${h}px`,
    zIndex: z
  }
})

// 颜色映射
function getColorValue(color: string): string {
  const colorMap: Record<string, string> = {
    yellow: '#fff9c4',
    pink: '#f8bbd9',
    green: '#c8e6c9',
    blue: '#bbdefb',
    purple: '#e1bee7',
    orange: '#ffe0b2'
  }
  return colorMap[color] || colorMap.yellow
}

// 格式化时间
function formatTime(timestamp: number): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return date.toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

// 事件处理
function handleMouseDown() {
  stickyNoteStore.bringToFront(props.noteId)
}

function toggleEdit() {
  if (isEditing.value) {
    saveTitle()
    saveContentNow()
    stickyNoteStore.stopEditing()
  } else {
    localTitle.value = note.value?.title || ''
    localContent.value = note.value?.content || ''
    stickyNoteStore.startEditing(props.noteId)
  }
}

function saveTitle() {
  if (note.value && localTitle.value !== note.value.title) {
    stickyNoteStore.updateNote(props.noteId, { title: localTitle.value })
  }
}

let saveTimeout: ReturnType<typeof setTimeout> | null = null
function debouncedSaveContent() {
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(() => {
    saveContentNow()
  }, 500)
}

function saveContentNow() {
  if (saveTimeout) {
    clearTimeout(saveTimeout)
    saveTimeout = null
  }
  if (note.value && localContent.value !== note.value.content) {
    stickyNoteStore.updateNote(props.noteId, {
      content: localContent.value,
      contentText: localContent.value.replace(/<[^>]*>/g, '')
    })
  }
}

function copyContent() {
  const text = note.value?.contentText || note.value?.content || ''
  navigator.clipboard.writeText(text)
}

async function pushToTargets() {
  if (!note.value || pushTargets.value.length === 0) {
    message.warning('请选择推送对象')
    return
  }
  const ok = await stickyNoteStore.pushNote(props.noteId, pushTargets.value)
  if (ok) {
    writePushTargets(pushTargets.value)
    message.success('已推送便签')
    pushPopoverVisible.value = false
    pushTargets.value = []
  } else {
    message.error('推送便签失败')
  }
}

function changeColor(color: string) {
  stickyNoteStore.updateNote(props.noteId, { color })
}

function minimize() {
  if (isEditing.value) {
    saveTitle()
    saveContentNow()
    stickyNoteStore.stopEditing()
  }
  stickyNoteStore.minimizeNote(props.noteId)
}

function close() {
  if (isEditing.value) {
    saveTitle()
    saveContentNow()
    stickyNoteStore.stopEditing()
  }
  stickyNoteStore.closeNote(props.noteId)
}

function deleteNote() {
  if (!note.value) return
  const confirmed = window.confirm('确认删除该便签？')
  if (!confirmed) return
  stickyNoteStore.deleteNote(props.noteId)
}

// 拖拽逻辑
function startDrag(e: MouseEvent) {
  if (!noteEl.value) return

  isDragging.value = true
  const rect = noteEl.value.getBoundingClientRect()
  dragOffset.value = {
    x: e.clientX - rect.left,
    y: e.clientY - rect.top
  }

  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', stopDrag)
}

function onDrag(e: MouseEvent) {
  if (!isDragging.value || !noteEl.value) return

  const x = e.clientX - dragOffset.value.x
  const y = e.clientY - dragOffset.value.y

  noteEl.value.style.left = `${Math.max(0, x)}px`
  noteEl.value.style.top = `${Math.max(0, y)}px`
}

function stopDrag() {
  if (!isDragging.value || !noteEl.value) return

  isDragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)

  const rect = noteEl.value.getBoundingClientRect()
  stickyNoteStore.updateUserState(props.noteId, {
    positionX: Math.round(rect.left),
    positionY: Math.round(rect.top)
  }, { persistRemote: false })
}

// 调整大小逻辑
function startResize(e: MouseEvent) {
  if (!noteEl.value) return

  isResizing.value = true
  const rect = noteEl.value.getBoundingClientRect()
  resizeStart.value = {
    x: e.clientX,
    y: e.clientY,
    w: rect.width,
    h: rect.height
  }

  document.addEventListener('mousemove', onResize)
  document.addEventListener('mouseup', stopResize)
}

function onResize(e: MouseEvent) {
  if (!isResizing.value || !noteEl.value) return

  const dx = e.clientX - resizeStart.value.x
  const dy = e.clientY - resizeStart.value.y

  const newW = Math.max(200, resizeStart.value.w + dx)
  const newH = Math.max(150, resizeStart.value.h + dy)

  noteEl.value.style.width = `${newW}px`
  noteEl.value.style.height = `${newH}px`
}

function stopResize() {
  if (!isResizing.value || !noteEl.value) return

  isResizing.value = false
  document.removeEventListener('mousemove', onResize)
  document.removeEventListener('mouseup', stopResize)

  const rect = noteEl.value.getBoundingClientRect()
  stickyNoteStore.updateUserState(props.noteId, {
    width: Math.round(rect.width),
    height: Math.round(rect.height)
  }, { persistRemote: false })
}

// 监听便签变化同步本地状态
watch(() => note.value, (newNote) => {
  if (newNote && isEditing.value) {
    // 如果是外部更新，不覆盖本地编辑状态
  } else if (newNote) {
    localTitle.value = newNote.title || ''
    localContent.value = newNote.content || ''
  }
}, { immediate: true })

watch(() => pushPopoverVisible.value, (visible) => {
  if (visible) {
    loadPushTargets()
    return
  }
  pushTargets.value = []
})

watch(() => allTargetIds.value, () => {
  if (pushPopoverVisible.value) {
    loadPushTargets()
    return
  }
  pushTargets.value = pushTargets.value.filter(id => allTargetIds.value.includes(id))
})

onUnmounted(() => {
  if (saveTimeout) {
    clearTimeout(saveTimeout)
    saveTimeout = null
  }
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
  document.removeEventListener('mousemove', onResize)
  document.removeEventListener('mouseup', stopResize)
})
</script>

<style scoped>
.sticky-note {
  position: fixed;
  display: flex;
  flex-direction: column;
  border-radius: 8px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
  overflow: hidden;
  font-family: system-ui, -apple-system, sans-serif;
  user-select: none;
  transition: box-shadow 0.2s;
}

.sticky-note:hover {
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.2);
}

.sticky-note--editing {
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.25);
}

/* 颜色主题 */
.sticky-note--yellow { background: linear-gradient(135deg, #fff9c4 0%, #fff59d 100%); }
.sticky-note--pink { background: linear-gradient(135deg, #f8bbd9 0%, #f48fb1 100%); }
.sticky-note--green { background: linear-gradient(135deg, #c8e6c9 0%, #a5d6a7 100%); }
.sticky-note--blue { background: linear-gradient(135deg, #bbdefb 0%, #90caf9 100%); }
.sticky-note--purple { background: linear-gradient(135deg, #e1bee7 0%, #ce93d8 100%); }
.sticky-note--orange { background: linear-gradient(135deg, #ffe0b2 0%, #ffcc80 100%); }

.sticky-note__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  cursor: move;
  background: rgba(0, 0, 0, 0.05);
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
}

.sticky-note__title {
  flex: 1;
  min-width: 0;
}

.sticky-note__title-text {
  font-size: 13px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.75);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sticky-note__title-input {
  width: 100%;
  border: none;
  background: rgba(255, 255, 255, 0.5);
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 600;
  outline: none;
}

.sticky-note__actions {
  display: flex;
  gap: 4px;
  margin-left: 8px;
}

.sticky-note__action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 4px;
  cursor: pointer;
  color: rgba(0, 0, 0, 0.6);
  transition: all 0.15s;
}

.sticky-note__action-btn:hover {
  background: rgba(0, 0, 0, 0.15);
  color: rgba(0, 0, 0, 0.8);
}

.sticky-note__action-btn--close:hover {
  background: #ef5350;
  color: white;
}

.sticky-note__body {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.sticky-note__content {
  font-size: 13px;
  line-height: 1.5;
  color: rgba(0, 0, 0, 0.75);
  word-wrap: break-word;
  user-select: text;
}

.sticky-note__editor {
  height: 100%;
}

.sticky-note__textarea {
  width: 100%;
  height: 100%;
  border: none;
  background: rgba(255, 255, 255, 0.4);
  padding: 8px;
  border-radius: 4px;
  font-size: 13px;
  line-height: 1.5;
  resize: none;
  outline: none;
  font-family: inherit;
}

.sticky-note__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-top: 1px solid rgba(0, 0, 0, 0.08);
  background: rgba(0, 0, 0, 0.03);
}

.sticky-note__meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: rgba(0, 0, 0, 0.55);
}

.sticky-note__meta-label {
  color: rgba(0, 0, 0, 0.45);
}

.sticky-note__meta-value {
  font-weight: 600;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note__meta-time {
  font-size: 11px;
  color: rgba(0, 0, 0, 0.5);
}

.sticky-note__colors {
  display: flex;
  gap: 4px;
}

.sticky-note__color-btn {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 0.15s;
}

.sticky-note__color-btn:hover {
  transform: scale(1.2);
}

.sticky-note__color-btn--active {
  border-color: rgba(0, 0, 0, 0.4);
}

.sticky-note__resize-handle {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 16px;
  height: 16px;
  cursor: nwse-resize;
  background: linear-gradient(
    135deg,
    transparent 50%,
    rgba(0, 0, 0, 0.1) 50%,
    rgba(0, 0, 0, 0.2) 100%
  );
  border-radius: 0 0 8px 0;
}

.sticky-note__push-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 220px;
  color: var(--sc-text-primary, #1f2937);
}

.sticky-note__push-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--sc-text-secondary, #6b7280);
}

.sticky-note__push-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.sticky-note__push-count {
  font-size: 11px;
  color: var(--sc-text-secondary, #6b7280);
}

.sticky-note__push-actions {
  display: flex;
  justify-content: flex-end;
}
</style>
