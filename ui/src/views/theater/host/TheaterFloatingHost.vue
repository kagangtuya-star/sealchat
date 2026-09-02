<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import {
  buildInternalSurfaceResourceKey,
  parseInternalSurfaceLink,
  type InternalSurfaceType,
} from '@/utils/internalSurfaceLink'
import {
  THEATER_FLOATING_TAKEOVER_ACK,
  isTheaterFloatingTakeoverRequest,
  requestChatFloatingTakeover,
  type TheaterFloatingTakeoverAck,
  type TheaterFloatingTakeoverRequest,
} from '@/utils/theaterFloatingBridge'

interface TheaterFloatingWindowState {
  id: string
  key: string
  url: string
  title: string
  x: number
  y: number
  width: number
  height: number
  zIndex: number
  minimized: boolean
  resourceType: InternalSurfaceType
  avatarUrl?: string
}

const props = defineProps<{
  chatFrame: HTMLIFrameElement | null
  worldId: string
  channelId: string
}>()

const hostRef = ref<HTMLElement | null>(null)
const windows = ref<TheaterFloatingWindowState[]>([])
const zCounter = ref(40)
const interaction = ref<{
  kind: 'drag' | 'resize'
  id: string
  pointerId: number
  startClientX: number
  startClientY: number
  startX: number
  startY: number
  startWidth: number
  startHeight: number
  target: HTMLElement
  moved: boolean
  offsetX: number
  offsetY: number
} | null>(null)
const suppressedRestoreClickId = ref<string | null>(null)

const MIN_WIDTH = 320
const MIN_HEIGHT = 220
const DEFAULT_WIDTH = 520
const DEFAULT_HEIGHT = 440
const EDGE_PADDING = 8
const MINIMIZED_HEIGHT = 38
const MINIMIZED_TITLE_WIDTH = 200
const MINIMIZED_CHARACTER_SIZE = 52
const STORAGE_PREFIX = 'sealchat:theater-floating-windows:v1:'
const MAX_PERSISTED_WINDOWS = 32
const PERSIST_DELAY = 150

let mounted = false
let restoring = false
let restoreEpoch = 0
let loadedWorldId = ''
let loadedChannelId = ''
let persistTimer: number | null = null

const findWindow = (id: string) => windows.value.find(item => item.id === id)
const hostRect = () => hostRef.value?.getBoundingClientRect()
const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max))
const minimizedWidth = (item: TheaterFloatingWindowState) => (
  item.resourceType === 'character' ? MINIMIZED_CHARACTER_SIZE : Math.min(MINIMIZED_TITLE_WIDTH, item.width)
)
const minimizedHeight = (item: TheaterFloatingWindowState) => (
  item.resourceType === 'character' ? MINIMIZED_CHARACTER_SIZE : MINIMIZED_HEIGHT
)

const storageKey = (worldId: string, channelId: string) => (
  `${STORAGE_PREFIX}${encodeURIComponent(worldId)}:${encodeURIComponent(channelId)}`
)

const normalizeStoredWindow = (value: unknown, worldId: string, channelId: string): TheaterFloatingWindowState | null => {
  if (!value || typeof value !== 'object') return null
  const stored = value as Record<string, unknown>
  const key = typeof stored.key === 'string' ? stored.key : ''
  const url = typeof stored.url === 'string' ? stored.url : ''
  const parsed = parseInternalSurfaceLink(url)
  if (
    !parsed
    || parsed.worldId !== worldId
    || parsed.channelId !== channelId
    || !key
    || buildInternalSurfaceResourceKey(parsed) !== key
  ) return null
  const finite = (input: unknown, fallback: number) => (
    typeof input === 'number' && Number.isFinite(input) ? input : fallback
  )
  return {
    id: key,
    key,
    url,
    title: typeof stored.title === 'string' && stored.title.trim() ? stored.title : '内部窗口',
    x: finite(stored.x, EDGE_PADDING),
    y: finite(stored.y, EDGE_PADDING),
    width: Math.max(MIN_WIDTH, finite(stored.width, DEFAULT_WIDTH)),
    height: Math.max(MIN_HEIGHT, finite(stored.height, DEFAULT_HEIGHT)),
    zIndex: Math.max(1, finite(stored.zIndex, 40)),
    minimized: stored.minimized === true,
    resourceType: parsed.type,
    avatarUrl: typeof stored.avatarUrl === 'string' && stored.avatarUrl ? stored.avatarUrl : undefined,
  }
}

const readStoredWindows = (worldId: string, channelId: string) => {
  if (!worldId || !channelId) return []
  try {
    const value = JSON.parse(window.localStorage.getItem(storageKey(worldId, channelId)) || '[]')
    if (!Array.isArray(value)) return []
    const keys = new Set<string>()
    return value.slice(0, MAX_PERSISTED_WINDOWS).flatMap((entry) => {
      const item = normalizeStoredWindow(entry, worldId, channelId)
      if (!item || keys.has(item.key)) return []
      keys.add(item.key)
      return [item]
    })
  } catch {
    return []
  }
}

const persistWindows = () => {
  if (restoring || !loadedWorldId || !loadedChannelId) return
  try {
    window.localStorage.setItem(storageKey(loadedWorldId, loadedChannelId), JSON.stringify(windows.value))
  } catch {
    // Private browsing or storage policy may disable local persistence.
  }
}

const flushPersist = () => {
  if (persistTimer !== null) {
    window.clearTimeout(persistTimer)
    persistTimer = null
  }
  persistWindows()
}

const schedulePersist = () => {
  if (restoring || !loadedWorldId) return
  if (persistTimer !== null) window.clearTimeout(persistTimer)
  persistTimer = window.setTimeout(() => {
    persistTimer = null
    persistWindows()
  }, PERSIST_DELAY)
}

const bringToFront = (id: string) => {
  const item = findWindow(id)
  if (!item) return
  item.zIndex = ++zCounter.value
}

const clampWindowPosition = (item: TheaterFloatingWindowState) => {
  const rect = hostRect()
  if (!rect) return
  const visibleWidth = item.minimized ? minimizedWidth(item) : item.width
  const visibleHeight = item.minimized ? minimizedHeight(item) : item.height
  item.x = clamp(item.x, EDGE_PADDING, rect.width - visibleWidth - EDGE_PADDING)
  item.y = clamp(item.y, EDGE_PADDING, rect.height - visibleHeight - EDGE_PADDING)
}

const fitWindowsToHost = () => {
  const rect = hostRect()
  if (!rect || rect.width <= 0 || rect.height <= 0) return
  windows.value.forEach((item) => {
    item.width = clamp(item.width, MIN_WIDTH, rect.width - EDGE_PADDING * 2)
    item.height = clamp(item.height, MIN_HEIGHT, rect.height - EDGE_PADDING * 2)
    clampWindowPosition(item)
  })
}

const restoreWindows = async (worldId: string, channelId: string) => {
  const epoch = ++restoreEpoch
  restoring = true
  loadedWorldId = worldId.trim()
  loadedChannelId = channelId.trim()
  windows.value = readStoredWindows(loadedWorldId, loadedChannelId)
  zCounter.value = Math.max(40, ...windows.value.map(item => item.zIndex))
  await nextTick()
  if (epoch !== restoreEpoch) return
  fitWindowsToHost()
  restoring = false
  schedulePersist()
}

const acceptTakeover = (request: TheaterFloatingTakeoverRequest) => {
  const rect = hostRect()
  if (!rect) return false
  if (
    request.clientX < rect.left
    || request.clientX > rect.right
    || request.clientY < rect.top
    || request.clientY > rect.bottom
  ) return false

  const parsed = parseInternalSurfaceLink(request.resource.url)
  if (
    !parsed
    || parsed.worldId !== props.worldId
    || parsed.channelId !== props.channelId
    || buildInternalSurfaceResourceKey(parsed) !== request.resource.key
  ) return false

  const existing = windows.value.find(item => item.key === request.resource.key)
  if (existing) {
    existing.minimized = false
    existing.title = request.resource.title || existing.title
    existing.avatarUrl = request.resource.presentation?.avatarUrl || existing.avatarUrl
    bringToFront(existing.id)
    clampWindowPosition(existing)
    return true
  }

  const requestedWidth = request.resource.presentation?.width
  const requestedHeight = request.resource.presentation?.height
  const width = clamp(requestedWidth ?? DEFAULT_WIDTH, MIN_WIDTH, rect.width - EDGE_PADDING * 2)
  const height = clamp(requestedHeight ?? DEFAULT_HEIGHT, MIN_HEIGHT, rect.height - EDGE_PADDING * 2)
  const initiallyMinimized = request.resource.presentation?.minimized === true
  const item: TheaterFloatingWindowState = {
    id: request.resource.key,
    key: request.resource.key,
    url: request.resource.url,
    title: request.resource.title || '内部窗口',
    x: request.clientX - rect.left - (initiallyMinimized && parsed.type === 'character' ? MINIMIZED_CHARACTER_SIZE / 2 : 80),
    y: request.clientY - rect.top - 18,
    width,
    height,
    zIndex: ++zCounter.value,
    minimized: initiallyMinimized,
    resourceType: parsed.type,
    avatarUrl: request.resource.presentation?.avatarUrl || undefined,
  }
  clampWindowPosition(item)
  windows.value.push(item)
  return true
}

const postAck = (event: MessageEvent, requestId: string, accepted: boolean) => {
  const target = props.chatFrame?.contentWindow
  if (!target || event.source !== target) return
  const ack: TheaterFloatingTakeoverAck = {
    type: THEATER_FLOATING_TAKEOVER_ACK,
    requestId,
    accepted,
  }
  target.postMessage(ack, event.origin)
}

const handleTakeoverMessage = async (event: MessageEvent<unknown>) => {
  if (event.origin !== window.location.origin) return
  if (event.source !== props.chatFrame?.contentWindow) return
  if (!isTheaterFloatingTakeoverRequest(event.data)) return
  const accepted = acceptTakeover(event.data)
  if (accepted) await nextTick()
  postAck(event, event.data.requestId, accepted)
}

const windowStyle = (item: TheaterFloatingWindowState) => {
  const width = item.minimized ? minimizedWidth(item) : item.width
  const height = item.minimized ? minimizedHeight(item) : item.height
  return {
    left: `${item.x}px`,
    top: `${item.y}px`,
    width: `${width}px`,
    height: `${height}px`,
    zIndex: item.zIndex,
  }
}

const titleInitial = (item: TheaterFloatingWindowState) => item.title.trim().charAt(0) || '?'

const startInteraction = (
  kind: 'drag' | 'resize',
  item: TheaterFloatingWindowState,
  event: PointerEvent,
) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  event.preventDefault()
  event.stopPropagation()
  bringToFront(item.id)
  const target = event.currentTarget as HTMLElement
  const rect = hostRect()
  if (!rect) return
  target.setPointerCapture?.(event.pointerId)
  interaction.value = {
    kind,
    id: item.id,
    pointerId: event.pointerId,
    startClientX: event.clientX,
    startClientY: event.clientY,
    startX: item.x,
    startY: item.y,
    startWidth: item.width,
    startHeight: item.height,
    target,
    moved: false,
    offsetX: event.clientX - rect.left - item.x,
    offsetY: event.clientY - rect.top - item.y,
  }
}

const handlePointerMove = (event: PointerEvent) => {
  const active = interaction.value
  if (!active || active.pointerId !== event.pointerId) return
  const item = findWindow(active.id)
  const rect = hostRect()
  if (!item || !rect) return
  event.preventDefault()
  const deltaX = event.clientX - active.startClientX
  const deltaY = event.clientY - active.startClientY
  if (Math.hypot(deltaX, deltaY) >= 4) active.moved = true
  if (active.kind === 'drag') {
    item.x = active.startX + deltaX
    item.y = active.startY + deltaY
    clampWindowPosition(item)
    return
  }
  item.width = clamp(active.startWidth + deltaX, MIN_WIDTH, rect.width - item.x - EDGE_PADDING)
  item.height = clamp(active.startHeight + deltaY, MIN_HEIGHT, rect.height - item.y - EDGE_PADDING)
}

const stopInteraction = (event: PointerEvent) => {
  const active = interaction.value
  if (!active || active.pointerId !== event.pointerId) return
  const item = findWindow(active.id)
  try {
    active.target.releasePointerCapture?.(event.pointerId)
  } catch {
    // Pointer capture can already be released by browser.
  }
  if (active.kind === 'drag' && active.moved) {
    suppressedRestoreClickId.value = active.id
    window.setTimeout(() => {
      if (suppressedRestoreClickId.value === active.id) suppressedRestoreClickId.value = null
    }, 0)
  }
  interaction.value = null
  const chatFrame = props.chatFrame
  if (active.kind === 'drag' && event.type === 'pointerup' && item && chatFrame) {
    void requestChatFloatingTakeover({
      key: item.key,
      url: item.url,
      title: item.title,
      presentation: {
        minimized: item.minimized,
        avatarUrl: item.avatarUrl,
        width: item.width,
        height: item.height,
      },
    }, event, chatFrame, {
      x: active.offsetX,
      y: active.offsetY,
    }).then((accepted) => {
      if (accepted) closeWindow(item.id)
    })
  }
}

const toggleMinimized = (item: TheaterFloatingWindowState) => {
  item.minimized = !item.minimized
  bringToFront(item.id)
  clampWindowPosition(item)
}

const restoreMinimized = (item: TheaterFloatingWindowState) => {
  if (!item.minimized) return
  if (suppressedRestoreClickId.value === item.id) {
    suppressedRestoreClickId.value = null
    return
  }
  toggleMinimized(item)
}

const closeWindow = (id: string) => {
  if (interaction.value?.id === id) interaction.value = null
  windows.value = windows.value.filter(item => item.id !== id)
}

watch(windows, schedulePersist, { deep: true })
watch(() => [props.worldId, props.channelId] as const, ([worldId, channelId]) => {
  if (!mounted) return
  flushPersist()
  void restoreWindows(worldId, channelId)
})

useResizeObserver(hostRef, fitWindowsToHost)

onMounted(() => {
  mounted = true
  window.addEventListener('message', handleTakeoverMessage)
  void restoreWindows(props.worldId, props.channelId)
})
onBeforeUnmount(() => {
  flushPersist()
  mounted = false
  window.removeEventListener('message', handleTakeoverMessage)
})
</script>

<template>
  <div ref="hostRef" class="theater-floating-host" aria-label="小剧场浮窗层">
    <section
      v-for="item in windows"
      :key="item.id"
      class="theater-floating-window"
      :class="{
        'is-minimized': item.minimized,
        'is-character': item.resourceType === 'character',
      }"
      :style="windowStyle(item)"
      @pointerdown="bringToFront(item.id)"
    >
      <button
        v-if="item.minimized && item.resourceType === 'character'"
        type="button"
        class="theater-floating-window__character-badge"
        :title="`${item.title}（点击恢复）`"
        @pointerdown="startInteraction('drag', item, $event)"
        @pointermove="handlePointerMove"
        @pointerup="stopInteraction"
        @pointercancel="stopInteraction"
        @click="restoreMinimized(item)"
      >
        <img v-if="item.avatarUrl" :src="item.avatarUrl" :alt="item.title">
        <span v-else>{{ titleInitial(item) }}</span>
      </button>
      <button
        v-if="item.minimized && item.resourceType !== 'character'"
        type="button"
        class="theater-floating-window__title-badge"
        :title="`${item.title}（点击恢复）`"
        @pointerdown="startInteraction('drag', item, $event)"
        @pointermove="handlePointerMove"
        @pointerup="stopInteraction"
        @pointercancel="stopInteraction"
        @click="restoreMinimized(item)"
      >
        {{ item.title }}
      </button>
      <header
        class="theater-floating-window__header"
        :class="{ 'is-hidden': item.minimized }"
        @pointerdown="startInteraction('drag', item, $event)"
        @pointermove="handlePointerMove"
        @pointerup="stopInteraction"
        @pointercancel="stopInteraction"
        @dblclick="toggleMinimized(item)"
      >
        <span class="theater-floating-window__title">{{ item.title }}</span>
        <span v-if="!item.minimized" class="theater-floating-window__actions" @pointerdown.stop>
          <button type="button" :title="item.minimized ? '恢复' : '最小化'" @click="toggleMinimized(item)">
            {{ item.minimized ? '□' : '—' }}
          </button>
          <button type="button" title="关闭" @click="closeWindow(item.id)">×</button>
        </span>
      </header>
      <div class="theater-floating-window__body" :class="{ 'is-hidden': item.minimized }">
        <iframe
          class="theater-floating-window__frame"
          :src="item.url"
          :title="item.title"
          frameborder="0"
          allow="autoplay; clipboard-read; clipboard-write"
        />
      </div>
      <div
        class="theater-floating-window__resize"
        :class="{ 'is-hidden': item.minimized }"
        @pointerdown="startInteraction('resize', item, $event)"
        @pointermove="handlePointerMove"
        @pointerup="stopInteraction"
        @pointercancel="stopInteraction"
      />
    </section>
  </div>
</template>

<style scoped>
.theater-floating-host { position: absolute; z-index: 30; inset: 0; overflow: hidden; pointer-events: none; }
.theater-floating-window { position: absolute; display: flex; flex-direction: column; overflow: hidden; pointer-events: auto; border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .12)); border-radius: 10px; background: var(--sc-bg-surface, #1b1b20); box-shadow: 0 14px 36px rgba(0, 0, 0, .32); }
.theater-floating-window__header { box-sizing: border-box; display: flex; flex: 0 0 38px; align-items: center; justify-content: space-between; min-width: 0; padding: 0 6px 0 12px; color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--sc-bg-elevated, #26262c) 94%, transparent); cursor: move; touch-action: none; user-select: none; }
.theater-floating-window__header.is-hidden { display: none; }
.theater-floating-window__title { flex: 1; min-width: 0; overflow: hidden; font-size: 13px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.theater-floating-window__actions { display: inline-flex; gap: 2px; }
.theater-floating-window__actions button { width: 28px; height: 28px; padding: 0; border: 0; border-radius: 5px; color: inherit; background: transparent; cursor: pointer; }
.theater-floating-window__actions button:hover { background: var(--sc-bg-hover, rgba(255, 255, 255, .08)); }
.theater-floating-window__body { position: relative; flex: 1; min-height: 0; }
.theater-floating-window__body.is-hidden { visibility: hidden; pointer-events: none; }
.theater-floating-window__frame { display: block; width: 100%; height: 100%; margin: 0; border: 0; outline: 0; background: var(--sc-bg-surface, #fff); }
.theater-floating-window__resize { position: absolute; z-index: 2; right: 0; bottom: 0; width: 18px; height: 18px; cursor: nwse-resize; touch-action: none; }
.theater-floating-window__resize.is-hidden { display: none; }
.theater-floating-window.is-minimized { min-width: 0; }
.theater-floating-window__title-badge,
.theater-floating-window__character-badge { position: absolute; z-index: 3; inset: 0; box-sizing: border-box; width: 100%; height: 100%; padding: 0; border: 0; color: var(--sc-text-primary, #f4f4f5); background: transparent; cursor: move; touch-action: none; user-select: none; }
.theater-floating-window__title-badge { overflow: hidden; padding: 0 14px; font-size: 13px; font-weight: 600; text-align: left; text-overflow: ellipsis; white-space: nowrap; background: color-mix(in srgb, var(--sc-bg-elevated, #26262c) 94%, transparent); }
.theater-floating-window.is-character.is-minimized { border-radius: 50%; background: var(--sc-bg-elevated, #26262c); box-shadow: 0 8px 24px rgba(0, 0, 0, .28); }
.theater-floating-window__character-badge img { display: block; width: 100%; height: 100%; object-fit: cover; }
.theater-floating-window__character-badge span { display: flex; width: 100%; height: 100%; align-items: center; justify-content: center; font-size: 20px; font-weight: 700; }
</style>
