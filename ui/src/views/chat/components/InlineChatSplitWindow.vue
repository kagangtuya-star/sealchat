<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { AppsOutline, CloseOutline, ContractOutline, ExpandOutline, ResizeOutline } from '@vicons/ionicons5'

interface Props {
  worldId: string
  channelId: string
  paneId: string
  title?: string
  persistLayout?: boolean
  forceOoc?: boolean
  zIndex: number
  cascadeIndex: number
}

interface LayoutState {
  x: number
  y: number
  width: number
  height: number
  minimized: boolean
}

const props = withDefaults(defineProps<Props>(), {
  persistLayout: false,
})
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'focus'): void
}>()

const STORAGE_KEY = 'sealchat.inline-chat-split.layout'
const MIN_WIDTH = 280
const MIN_HEIGHT = 220
const VIEWPORT_GAP = 12
const DESKTOP_TOP = 68
const DESKTOP_RIGHT = 22
const HEADER_HEIGHT = 36

const iframeRef = ref<HTMLIFrameElement | null>(null)

const viewportSize = () => ({
  width: Math.max(0, window.innerWidth),
  height: Math.max(0, window.innerHeight),
})

const createDefaultLayout = (): LayoutState => {
  const viewport = viewportSize()
  const mobile = viewport.width < 768
  const availableWidth = Math.max(MIN_WIDTH, viewport.width - VIEWPORT_GAP * 2)
  const width = mobile
    ? Math.min(availableWidth, Math.max(MIN_WIDTH, viewport.width * 0.9))
    : Math.max(MIN_WIDTH, viewport.width * 0.2)
  const availableHeight = Math.max(MIN_HEIGHT, viewport.height - DESKTOP_TOP - VIEWPORT_GAP)
  const height = mobile
    ? Math.min(availableHeight, Math.max(MIN_HEIGHT, viewport.height * 0.45))
    : Math.max(MIN_HEIGHT, viewport.height * 0.25)
  return {
    x: Math.max(VIEWPORT_GAP, viewport.width - width - DESKTOP_RIGHT),
    y: Math.min(DESKTOP_TOP, Math.max(VIEWPORT_GAP, viewport.height - height - VIEWPORT_GAP)),
    width,
    height,
    minimized: false,
  }
}

const loadLayout = (): LayoutState => {
  const fallback = createDefaultLayout()
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as Partial<LayoutState>
    return {
      x: typeof parsed.x === 'number' && Number.isFinite(parsed.x) ? parsed.x : fallback.x,
      y: typeof parsed.y === 'number' && Number.isFinite(parsed.y) ? parsed.y : fallback.y,
      width: typeof parsed.width === 'number' && Number.isFinite(parsed.width) ? parsed.width : fallback.width,
      height: typeof parsed.height === 'number' && Number.isFinite(parsed.height) ? parsed.height : fallback.height,
      minimized: typeof parsed.minimized === 'boolean' ? parsed.minimized : false,
    }
  } catch {
    return fallback
  }
}

const layout = reactive<LayoutState>(loadLayout())

const cascadeOffset = Math.max(0, props.cascadeIndex) * 22
layout.x += cascadeOffset
layout.y += cascadeOffset

const clampLayout = () => {
  const viewport = viewportSize()
  const maxWidth = Math.max(0, viewport.width - VIEWPORT_GAP * 2)
  const maxHeight = Math.max(0, viewport.height - VIEWPORT_GAP * 2)
  layout.width = Math.min(Math.max(Math.min(MIN_WIDTH, maxWidth), layout.width), maxWidth)
  layout.height = Math.min(Math.max(Math.min(MIN_HEIGHT, maxHeight), layout.height), maxHeight)
  const visibleWidth = layout.minimized ? Math.min(240, layout.width) : layout.width
  const visibleHeight = layout.minimized ? HEADER_HEIGHT : layout.height
  layout.x = Math.min(Math.max(VIEWPORT_GAP, layout.x), Math.max(VIEWPORT_GAP, viewport.width - visibleWidth - VIEWPORT_GAP))
  layout.y = Math.min(Math.max(VIEWPORT_GAP, layout.y), Math.max(VIEWPORT_GAP, viewport.height - visibleHeight - VIEWPORT_GAP))
}

clampLayout()

const persistLayout = () => {
  if (!props.persistLayout) return
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({
      x: layout.x,
      y: layout.y,
      width: layout.width,
      height: layout.height,
      minimized: layout.minimized,
    }))
  } catch {
    // localStorage 不可用时，仅保留当前页面内的布局。
  }
}

const windowStyle = computed(() => ({
  left: `${layout.x}px`,
  top: `${layout.y}px`,
  width: layout.minimized ? `${Math.min(240, layout.width)}px` : `${layout.width}px`,
  height: layout.minimized ? `${HEADER_HEIGHT}px` : `${layout.height}px`,
  zIndex: props.zIndex,
}))

const iframeSrc = computed(() => {
  const params = new URLSearchParams({
    worldId: props.worldId,
    channelId: props.channelId,
    paneId: props.paneId,
    inlineSplit: '1',
    audioOwner: '0',
  })
  if (props.forceOoc === true) params.set('forceOoc', '1')
  const base = `${window.location.pathname}${window.location.search}`
  return `${base}#/embed?${params.toString()}`
})

type PointerOperation =
  | { type: 'drag'; pointerId: number; offsetX: number; offsetY: number; target: HTMLElement }
  | { type: 'resize'; pointerId: number; startX: number; startY: number; width: number; height: number; target: HTMLElement }

let pointerOperation: PointerOperation | null = null

const startDragging = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  const target = event.currentTarget as HTMLElement
  target.setPointerCapture?.(event.pointerId)
  pointerOperation = {
    type: 'drag',
    pointerId: event.pointerId,
    offsetX: event.clientX - layout.x,
    offsetY: event.clientY - layout.y,
    target,
  }
}

const startResizing = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  emit('focus')
  const target = event.currentTarget as HTMLElement
  target.setPointerCapture?.(event.pointerId)
  pointerOperation = {
    type: 'resize',
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    width: layout.width,
    height: layout.height,
    target,
  }
}

const handlePointerMove = (event: PointerEvent) => {
  const operation = pointerOperation
  if (!operation || operation.pointerId !== event.pointerId) return
  event.preventDefault()
  if (operation.type === 'drag') {
    layout.x = event.clientX - operation.offsetX
    layout.y = event.clientY - operation.offsetY
  } else {
    layout.width = operation.width + event.clientX - operation.startX
    layout.height = operation.height + event.clientY - operation.startY
  }
  clampLayout()
}

const finishPointerOperation = (event: PointerEvent) => {
  const operation = pointerOperation
  if (!operation || operation.pointerId !== event.pointerId) return
  operation.target.releasePointerCapture?.(event.pointerId)
  pointerOperation = null
  persistLayout()
}

const toggleMinimized = () => {
  layout.minimized = !layout.minimized
  clampLayout()
  persistLayout()
}

const handleClose = () => {
  persistLayout()
  emit('close')
}

const toggleActionRibbon = () => {
  const targetWindow = iframeRef.value?.contentWindow
  if (!targetWindow) return
  targetWindow.postMessage({
    type: 'sealchat.embed.toggleActionRibbon',
    paneId: props.paneId,
  }, window.location.origin)
}

const handleEmbedMessage = (event: MessageEvent) => {
  const targetWindow = iframeRef.value?.contentWindow
  if (event.origin !== window.location.origin || !targetWindow || event.source !== targetWindow) return
  const data = event.data as { type?: unknown; paneId?: unknown } | null
  if (!data || data.type !== 'sealchat.embed.ready' || data.paneId !== props.paneId) return
  if (props.forceOoc !== true) return
  targetWindow.postMessage({
    type: 'sealchat.embed.setIcMode',
    paneId: props.paneId,
    icMode: 'ooc',
  }, window.location.origin)
  targetWindow.postMessage({
    type: 'sealchat.embed.setFilterState',
    paneId: props.paneId,
    filterState: {
      icFilter: 'ooc',
      showArchived: false,
      roleIds: [],
      whisperOnly: false,
      fromTime: null,
      toTime: null,
    },
  }, window.location.origin)
}

const handleViewportResize = () => {
  clampLayout()
  persistLayout()
}

onMounted(() => {
  window.addEventListener('message', handleEmbedMessage)
  window.addEventListener('pointermove', handlePointerMove)
  window.addEventListener('pointerup', finishPointerOperation)
  window.addEventListener('pointercancel', finishPointerOperation)
  window.addEventListener('resize', handleViewportResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('message', handleEmbedMessage)
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', finishPointerOperation)
  window.removeEventListener('pointercancel', finishPointerOperation)
  window.removeEventListener('resize', handleViewportResize)
  pointerOperation = null
})
</script>

<template>
  <Teleport to="body">
    <section
      class="inline-chat-split"
      :class="{ 'is-minimized': layout.minimized }"
      :style="windowStyle"
      @pointerdown="emit('focus')"
    >
      <header class="inline-chat-split__header" @pointerdown.prevent="startDragging">
        <strong class="inline-chat-split__title">{{ title?.trim() || '页内分屏' }}</strong>
        <div class="inline-chat-split__actions" @pointerdown.stop>
          <button type="button" class="inline-chat-split__action" title="更多跑团功能" aria-label="更多跑团功能" @click.stop="toggleActionRibbon">
            <n-icon :component="AppsOutline" />
          </button>
          <button type="button" class="inline-chat-split__action" :title="layout.minimized ? '还原' : '最小化'" @click.stop="toggleMinimized">
            <n-icon :component="layout.minimized ? ExpandOutline : ContractOutline" />
          </button>
          <button type="button" class="inline-chat-split__action" title="关闭" @click.stop="handleClose">
            <n-icon :component="CloseOutline" />
          </button>
        </div>
      </header>
      <div v-show="!layout.minimized" class="inline-chat-split__body">
        <iframe
          ref="iframeRef"
          :key="paneId"
          class="inline-chat-split__iframe"
          :src="iframeSrc"
          title="页内聊天浮窗"
          frameborder="0"
        />
        <button
          type="button"
          class="inline-chat-split__resize"
          title="调整大小"
          @pointerdown.stop.prevent="startResizing"
        >
          <n-icon :component="ResizeOutline" />
        </button>
      </div>
    </section>
  </Teleport>
</template>

<style scoped>
.inline-chat-split {
  position: fixed;
  display: flex;
  flex-direction: column;
  min-width: 280px;
  min-height: 220px;
  overflow: hidden;
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  background: var(--sc-bg-elevated);
  color: var(--sc-text-primary);
  box-shadow: 0 14px 38px rgba(15, 23, 42, 0.2);
}

.inline-chat-split.is-minimized {
  min-width: 0;
  min-height: 0;
}

.inline-chat-split__header {
  box-sizing: border-box;
  height: 36px;
  flex: 0 0 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 6px 0 10px;
  border-bottom: 1px solid var(--sc-border-mute);
  background: var(--sc-bg-elevated);
  cursor: move;
  touch-action: none;
  user-select: none;
}

.is-minimized .inline-chat-split__header {
  border-bottom: 0;
}

.inline-chat-split__title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  color: var(--sc-text-primary);
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inline-chat-split__actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.inline-chat-split__action,
.inline-chat-split__resize {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  color: var(--sc-text-secondary);
  cursor: pointer;
}

.inline-chat-split__action {
  width: 26px;
  height: 26px;
  border-radius: 6px;
}

.inline-chat-split__action:hover {
  background: var(--sc-chip-bg);
  color: var(--sc-text-primary);
}

.inline-chat-split__body {
  position: relative;
  min-width: 0;
  min-height: 0;
  flex: 1;
}

.inline-chat-split__iframe {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
  background: var(--sc-bg-elevated);
}

.inline-chat-split__resize {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 24px;
  height: 24px;
  color: var(--sc-text-secondary);
  cursor: nwse-resize;
  touch-action: none;
}

@media (max-width: 767px) {
  .inline-chat-split {
    max-width: calc(100vw - 24px);
    max-height: calc(100vh - 24px);
  }
}
</style>
