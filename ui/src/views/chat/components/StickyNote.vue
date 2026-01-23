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
      @pointerdown="handlePointerDown"
    >
      <!-- 头部 -->
      <div
        ref="headerEl"
        class="sticky-note__header"
        @pointerdown="startDrag"
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
            @pointerdown.stop
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
                @pointerdown.stop
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M4 12v7a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-7h-2v6H6v-6H4zm8-9l5 5h-3v6h-4V8H7l5-5z"/>
                </svg>
              </button>
            </template>
            <div class="sticky-note__push-panel" @pointerdown.stop>
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
            @pointerdown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
            </svg>
          </button>
          <button
            class="sticky-note__action-btn"
            title="最小化"
            @click="minimize"
            @pointerdown.stop
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
            @pointerdown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M6 7h12v2H6V7zm2 3h8l-1 10H9L8 10zm3-5h2l1 1h5v2H4V6h5l1-1z"/>
            </svg>
          </button>
          <button
            class="sticky-note__action-btn sticky-note__action-btn--close"
            title="关闭"
            @click="close"
            @pointerdown.stop
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- 内容区域 -->
      <div class="sticky-note__body">
        <!-- 文本类型保持原有编辑逻辑 -->
        <template v-if="isTextType">
          <div
            v-if="isEditing"
            class="sticky-note__editor"
          >
            <!-- 富文本模式 -->
            <StickyNoteEditor
              v-if="richMode"
              ref="editorRef"
              v-model="localContent"
              :channel-id="note?.channelId"
              @update:model-value="debouncedSaveContent"
            />
            <!-- 简单模式 -->
            <div v-else class="sticky-note__simple-editor">
              <div class="sticky-note__simple-toolbar">
                <button
                  class="sticky-note__toolbar-btn"
                  :class="{ 'is-active': richMode }"
                  @click="richMode = true"
                  title="切换到富文本模式"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M5 4v3h5.5v12h3V7H19V4H5z"/>
                  </svg>
                </button>
              </div>
              <textarea
                v-model="localContent"
                class="sticky-note__textarea"
                placeholder="在此输入内容..."
                @input="debouncedSaveContent"
              ></textarea>
            </div>
          </div>
          <div
            v-else
            class="sticky-note__content"
            v-html="sanitizedContent"
            @click="handleContentClick"
          ></div>
        </template>
        <!-- 其他类型使用动态组件 -->
        <component
          v-else
          :is="currentTypeComponent"
          :note="note"
          :is-editing="isEditing"
        />
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
        @pointerdown.stop="startResize"
      ></div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, defineAsyncComponent } from 'vue'
import { useMessage } from 'naive-ui'
import { useStickyNoteStore, type StickyNote, type StickyNotePushLayout, type StickyNoteUserState, type StickyNoteType } from '@/stores/stickyNote'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import StickyNoteEditor from './StickyNoteEditor.vue'
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render'

// 动态导入类型组件
const StickyNoteText = defineAsyncComponent(() => import('./sticky-notes/StickyNoteText.vue'))
const StickyNoteCounter = defineAsyncComponent(() => import('./sticky-notes/StickyNoteCounter.vue'))
const StickyNoteList = defineAsyncComponent(() => import('./sticky-notes/StickyNoteList.vue'))
const StickyNoteSlider = defineAsyncComponent(() => import('./sticky-notes/StickyNoteSlider.vue'))
const StickyNoteTimer = defineAsyncComponent(() => import('./sticky-notes/StickyNoteTimer.vue'))
const StickyNoteClock = defineAsyncComponent(() => import('./sticky-notes/StickyNoteClock.vue'))
const StickyNoteRoundCounter = defineAsyncComponent(() => import('./sticky-notes/StickyNoteRoundCounter.vue'))

// 类型组件映射
const typeComponentMap: Record<StickyNoteType, ReturnType<typeof defineAsyncComponent>> = {
  text: StickyNoteText,
  counter: StickyNoteCounter,
  list: StickyNoteList,
  slider: StickyNoteSlider,
  chat: StickyNoteText, // chat 暂时使用 text
  timer: StickyNoteTimer,
  clock: StickyNoteClock,
  roundCounter: StickyNoteRoundCounter
}

const props = defineProps<{
  noteId: string
}>()

const stickyNoteStore = useStickyNoteStore()
const chatStore = useChatStore()
const userStore = useUserStore()
const message = useMessage()

const noteEl = ref<HTMLElement | null>(null)
const headerEl = ref<HTMLElement | null>(null)
const editorRef = ref<InstanceType<typeof StickyNoteEditor> | null>(null)

// 本地编辑状态
const localTitle = ref('')
const localContent = ref('')
const pushPopoverVisible = ref(false)
const pushTargets = ref<string[]>([])
const MIN_NOTE_WIDTH = 200
const MIN_NOTE_HEIGHT = 150
const VIEWPORT_PADDING = 8
const richMode = ref(false) // 富文本模式，默认关闭

// 拖拽状态
const isDragging = ref(false)
const dragOffset = ref({ x: 0, y: 0 })
const dragPointerId = ref<number | null>(null)

// 调整大小状态
const isResizing = ref(false)
const resizeStart = ref({ x: 0, y: 0, w: 0, h: 0 })
const resizePointerId = ref<number | null>(null)

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

// 进入编辑模式时，如果内容是富文本（TipTap JSON），自动切换到富文本模式
watch(isEditing, (editing) => {
  if (editing && note.value?.content) {
    richMode.value = isTipTapJson(note.value.content)
  }
})

const isOwner = computed(() => {
  const userId = userStore.info?.id
  if (!userId) return false
  return note.value?.creatorId === userId || note.value?.creator?.id === userId
})

// 当前便签类型对应的组件
const currentTypeComponent = computed(() => {
  const type = (note.value?.noteType || 'text') as StickyNoteType
  return typeComponentMap[type] || StickyNoteText
})

// 是否是文本类型（需要特殊处理编辑模式）
const isTextType = computed(() => {
  const type = note.value?.noteType || 'text'
  return type === 'text' || type === 'chat'
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
  // 检测是否为 TipTap JSON 格式
  if (isTipTapJson(content)) {
    try {
      return tiptapJsonToHtml(content, { imageClass: 'sticky-note__image' })
    } catch {
      // 渲染失败时回退到纯文本
    }
  }
  // 兼容旧内容 - 保留 img 标签，转义其他 HTML
  // 先提取所有 img 标签保护起来
  const imgPlaceholders: string[] = []
  let processed = content.replace(/<img\s+[^>]*>/gi, (match) => {
    imgPlaceholders.push(match)
    return `__IMG_PLACEHOLDER_${imgPlaceholders.length - 1}__`
  })
  
  // 转义其他 HTML
  processed = processed
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
  
  // 恢复 img 标签
  imgPlaceholders.forEach((img, i) => {
    processed = processed.replace(`__IMG_PLACEHOLDER_${i}__`, img)
  })
  
  return processed
})

const handleContentClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null
  if (!target) return
  if (target.closest('a')) return
  const spoiler = target.closest('.tiptap-spoiler') as HTMLElement | null
  if (!spoiler) return
  spoiler.classList.toggle('is-revealed')
}

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

function clampNumber(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max)
}

function getViewportSize() {
  return {
    width: Math.max(window.innerWidth, 1),
    height: Math.max(window.innerHeight, 1)
  }
}

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
function handlePointerDown(e: PointerEvent) {
  if (e.pointerType === 'mouse' && e.button !== 0) return
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

function roundRatio(value: number) {
  return Math.round(value * 10000) / 10000
}

function buildPushLayout(): StickyNotePushLayout | null {
  if (typeof window === 'undefined' || !noteEl.value) return null
  const viewportW = window.innerWidth
  const viewportH = window.innerHeight
  if (!viewportW || !viewportH) return null
  const rect = noteEl.value.getBoundingClientRect()
  return {
    xPct: roundRatio(rect.left / viewportW),
    yPct: roundRatio(rect.top / viewportH),
    wPct: roundRatio(rect.width / viewportW),
    hPct: roundRatio(rect.height / viewportH)
  }
}

async function pushToTargets() {
  if (!note.value || pushTargets.value.length === 0) {
    message.warning('请选择推送对象')
    return
  }
  const layout = buildPushLayout()
  const ok = await stickyNoteStore.pushNote(props.noteId, pushTargets.value, layout || undefined)
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
function startDrag(e: PointerEvent) {
  if (!noteEl.value) return
  if (e.pointerType === 'mouse' && e.button !== 0) return

  isDragging.value = true
  dragPointerId.value = e.pointerId
  const rect = noteEl.value.getBoundingClientRect()
  dragOffset.value = {
    x: e.clientX - rect.left,
    y: e.clientY - rect.top
  }

  document.addEventListener('pointermove', onDrag)
  document.addEventListener('pointerup', stopDrag)
  document.addEventListener('pointercancel', stopDrag)
}

function onDrag(e: PointerEvent) {
  if (!isDragging.value || !noteEl.value) return
  if (dragPointerId.value !== e.pointerId) return

  const viewport = getViewportSize()
  const rect = noteEl.value.getBoundingClientRect()
  const maxX = Math.max(0, viewport.width - rect.width)
  const maxY = Math.max(0, viewport.height - rect.height)
  const x = clampNumber(e.clientX - dragOffset.value.x, 0, maxX)
  const y = clampNumber(e.clientY - dragOffset.value.y, 0, maxY)

  noteEl.value.style.left = `${x}px`
  noteEl.value.style.top = `${y}px`
}

function stopDrag(e: PointerEvent) {
  if (!isDragging.value || !noteEl.value) return
  if (dragPointerId.value !== e.pointerId) return

  isDragging.value = false
  dragPointerId.value = null
  document.removeEventListener('pointermove', onDrag)
  document.removeEventListener('pointerup', stopDrag)
  document.removeEventListener('pointercancel', stopDrag)

  const rect = noteEl.value.getBoundingClientRect()
  stickyNoteStore.updateUserState(props.noteId, {
    positionX: Math.round(rect.left),
    positionY: Math.round(rect.top)
  }, { persistRemote: false })
}

// 调整大小逻辑
function startResize(e: PointerEvent) {
  if (!noteEl.value) return
  if (e.pointerType === 'mouse' && e.button !== 0) return

  isResizing.value = true
  resizePointerId.value = e.pointerId
  const rect = noteEl.value.getBoundingClientRect()
  resizeStart.value = {
    x: e.clientX,
    y: e.clientY,
    w: rect.width,
    h: rect.height
  }

  document.addEventListener('pointermove', onResize)
  document.addEventListener('pointerup', stopResize)
  document.addEventListener('pointercancel', stopResize)
}

function onResize(e: PointerEvent) {
  if (!isResizing.value || !noteEl.value) return
  if (resizePointerId.value !== e.pointerId) return

  const dx = e.clientX - resizeStart.value.x
  const dy = e.clientY - resizeStart.value.y

  const viewport = getViewportSize()
  const rect = noteEl.value.getBoundingClientRect()
  const maxW = Math.max(MIN_NOTE_WIDTH, viewport.width - rect.left - VIEWPORT_PADDING)
  const maxH = Math.max(MIN_NOTE_HEIGHT, viewport.height - rect.top - VIEWPORT_PADDING)
  const newW = clampNumber(resizeStart.value.w + dx, MIN_NOTE_WIDTH, maxW)
  const newH = clampNumber(resizeStart.value.h + dy, MIN_NOTE_HEIGHT, maxH)

  noteEl.value.style.width = `${newW}px`
  noteEl.value.style.height = `${newH}px`
}

function stopResize(e: PointerEvent) {
  if (!isResizing.value || !noteEl.value) return
  if (resizePointerId.value !== e.pointerId) return

  isResizing.value = false
  resizePointerId.value = null
  document.removeEventListener('pointermove', onResize)
  document.removeEventListener('pointerup', stopResize)
  document.removeEventListener('pointercancel', stopResize)

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
  document.removeEventListener('pointermove', onDrag)
  document.removeEventListener('pointerup', stopDrag)
  document.removeEventListener('pointercancel', stopDrag)
  document.removeEventListener('pointermove', onResize)
  document.removeEventListener('pointerup', stopResize)
  document.removeEventListener('pointercancel', stopResize)
})
</script>

<style scoped>
.sticky-note {
  position: fixed;
  display: flex;
  flex-direction: column;
  border-radius: 8px;
  max-width: calc(100vw - 16px);
  max-height: calc(100vh - 16px);
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
  touch-action: none;
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
  touch-action: none;
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

/* 富文本内容样式 */
.sticky-note__content :deep(img),
.sticky-note__image {
  max-width: 100%;
  height: auto;
  border-radius: 4px;
  margin: 4px 0;
  display: block;
}

.sticky-note__content :deep(p) {
  margin: 0 0 0.5em;
}

.sticky-note__content :deep(p:last-child) {
  margin-bottom: 0;
}

.sticky-note__content :deep(ul),
.sticky-note__content :deep(ol) {
  margin: 0.5em 0;
  padding-left: 1.5em;
}

.sticky-note__content :deep(ul) {
  list-style-type: disc;
}

.sticky-note__content :deep(ol) {
  list-style-type: decimal;
}

.sticky-note__content :deep(li) {
  margin: 0.25em 0;
}

.sticky-note__content :deep(a) {
  color: #2563eb;
  text-decoration: underline;
}

.sticky-note__content :deep(strong) {
  font-weight: 600;
}

.sticky-note__content :deep(mark) {
  padding: 0 2px;
  border-radius: 2px;
}

.sticky-note__content :deep(code) {
  background: rgba(0, 0, 0, 0.08);
  padding: 1px 4px;
  border-radius: 3px;
  font-family: monospace;
  font-size: 0.9em;
}

/* 简单编辑器样式 */
.sticky-note__simple-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.sticky-note__simple-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 6px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.1);
}

.sticky-note__toolbar-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  border-radius: 3px;
  cursor: pointer;
  color: rgba(0, 0, 0, 0.6);
  transition: all 0.15s;
}

.sticky-note__toolbar-btn:hover {
  background: rgba(0, 0, 0, 0.1);
  color: rgba(0, 0, 0, 0.8);
}

.sticky-note__toolbar-btn.is-active {
  background: rgba(0, 0, 0, 0.15);
  color: rgba(0, 0, 0, 0.9);
}

.sticky-note__toolbar-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.sticky-note__textarea {
  flex: 1;
  width: 100%;
  border: none;
  background: transparent;
  padding: 8px;
  font-size: 13px;
  line-height: 1.5;
  resize: none;
  outline: none;
  font-family: inherit;
}

/* 最小化滚动条 */
.sticky-note__body {
  scrollbar-width: thin;
  scrollbar-color: rgba(0, 0, 0, 0.2) transparent;
}

.sticky-note__body::-webkit-scrollbar {
  width: 4px;
}

.sticky-note__body::-webkit-scrollbar-track {
  background: transparent;
}

.sticky-note__body::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.2);
  border-radius: 2px;
}

.sticky-note__body::-webkit-scrollbar-thumb:hover {
  background: rgba(0, 0, 0, 0.3);
}

.sticky-note__textarea {
  scrollbar-width: thin;
  scrollbar-color: rgba(0, 0, 0, 0.2) transparent;
}

.sticky-note__textarea::-webkit-scrollbar {
  width: 4px;
}

.sticky-note__textarea::-webkit-scrollbar-track {
  background: transparent;
}

.sticky-note__textarea::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.2);
  border-radius: 2px;
}
</style>

<style>
/* ===== 夜间模式和自定义主题适配 ===== */
/* 便签背景始终是浅色的，所以所有文字都需要保持深色 */
/* 使用非 scoped 样式因为便签使用 Teleport 渲染 */

:root[data-display-palette='night'] .sticky-note__title-text {
  color: rgba(0, 0, 0, 0.75) !important;
}

:root[data-display-palette='night'] .sticky-note__title-input {
  color: rgba(0, 0, 0, 0.85) !important;
  background: rgba(255, 255, 255, 0.5);
}

:root[data-display-palette='night'] .sticky-note__content {
  color: rgba(0, 0, 0, 0.75) !important;
}

:root[data-display-palette='night'] .sticky-note__textarea {
  color: rgba(0, 0, 0, 0.85) !important;
}

:root[data-display-palette='night'] .sticky-note__action-btn {
  color: rgba(0, 0, 0, 0.6);
}

:root[data-display-palette='night'] .sticky-note__action-btn:hover {
  color: rgba(0, 0, 0, 0.8);
}

:root[data-display-palette='night'] .sticky-note__meta {
  color: rgba(0, 0, 0, 0.55);
}

:root[data-display-palette='night'] .sticky-note__meta-label {
  color: rgba(0, 0, 0, 0.45);
}

:root[data-display-palette='night'] .sticky-note__meta-value {
  color: rgba(0, 0, 0, 0.7);
}

:root[data-display-palette='night'] .sticky-note__meta-time {
  color: rgba(0, 0, 0, 0.5);
}

:root[data-display-palette='night'] .sticky-note__toolbar-btn {
  color: rgba(0, 0, 0, 0.6);
}

:root[data-display-palette='night'] .sticky-note__toolbar-btn:hover {
  color: rgba(0, 0, 0, 0.8);
}

/* 自定义主题模式 - 同样需要保持便签文字深色 */
:root[data-custom-theme='true'] .sticky-note__title-text {
  color: rgba(0, 0, 0, 0.75) !important;
}

:root[data-custom-theme='true'] .sticky-note__title-input {
  color: rgba(0, 0, 0, 0.85) !important;
  background: rgba(255, 255, 255, 0.5);
}

:root[data-custom-theme='true'] .sticky-note__content {
  color: rgba(0, 0, 0, 0.75) !important;
}

:root[data-custom-theme='true'] .sticky-note__textarea {
  color: rgba(0, 0, 0, 0.85) !important;
}

:root[data-custom-theme='true'] .sticky-note__action-btn {
  color: rgba(0, 0, 0, 0.6);
}

:root[data-custom-theme='true'] .sticky-note__meta {
  color: rgba(0, 0, 0, 0.55);
}

:root[data-custom-theme='true'] .sticky-note__toolbar-btn {
  color: rgba(0, 0, 0, 0.6);
}
</style>
