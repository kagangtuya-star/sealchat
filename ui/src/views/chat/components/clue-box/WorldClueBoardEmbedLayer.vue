<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NTooltip, useMessage } from 'naive-ui'
import { Copy, ExternalLink, LayoutBoard, Maximize, Minus, X } from '@vicons/tabler'
import { useUserStore } from '@/stores/user'
import { copyTextWithFallback } from '@/utils/clipboard'
import { openInternalSurfaceLink } from '@/utils/internalSurfaceLink'
import { generateWorldClueBoardLink } from '@/utils/worldClueBoardLink'

const props = defineProps<{ show: boolean; worldId: string; channelId: string }>()
const emit = defineEmits<{ 'update:show': [value: boolean] }>()
const message = useMessage()
const iframe = ref<HTMLIFrameElement | null>(null)
const ready = ref(false)
const closing = ref(false)
const link = ref('')
const surfaceWorldId = ref('')
const surfaceChannelId = ref('')
const user = useUserStore()
const panel = ref<HTMLElement | null>(null)
const geometry = ref({ x: 0, y: 0, width: 900, height: 700 })
const gesture = ref<{ pointerId: number; edge: string; x: number; y: number; start: typeof geometry.value } | null>(null)
const mobile = ref(false)
const minimized = ref(false)
const miniPanel = ref<HTMLElement | null>(null)
const miniPosition = ref({ x: 0, y: 0 })
const miniGesture = ref<{ pointerId: number; x: number; y: number; originX: number; originY: number; moved: boolean } | null>(null)
const restoringFromStorage = ref(false)
const MINI_WIDTH = 120
const MINI_HEIGHT = 38
const storageKey = computed(() => `sealchat_clue_board_panel_v1:${String(user.info.id || '')}:${surfaceWorldId.value || props.worldId}`)
const miniStorageKey = computed(() => `sealchat_clue_board_mini_v1:${String(user.info.id || '')}:${surfaceWorldId.value || props.worldId}`)
const stateStorageKey = computed(() => `sealchat_clue_board_state_v1:${String(user.info.id || '')}:${surfaceWorldId.value || props.worldId}`)
const panelStyle = computed(() => mobile.value ? {} : { left: `${geometry.value.x}px`, top: `${geometry.value.y}px`, width: `${geometry.value.width}px`, height: `${geometry.value.height}px` })
const miniStyle = computed(() => ({ left: `${miniPosition.value.x}px`, top: `${miniPosition.value.y}px`, width: `${MINI_WIDTH}px`, height: `${MINI_HEIGHT}px` }))
const resizeEdges = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw']

function clampGeometry() {
  const width = Math.min(window.innerWidth, Math.max(420, geometry.value.width))
  const height = Math.min(window.innerHeight, Math.max(320, geometry.value.height))
  geometry.value = { width, height, x: Math.max(0, Math.min(window.innerWidth - width, geometry.value.x)), y: Math.max(0, Math.min(window.innerHeight - height, geometry.value.y)) }
}

function restoreGeometry() {
  mobile.value = window.innerWidth <= 680
  geometry.value = { x: window.innerWidth * .11, y: window.innerHeight * .09, width: window.innerWidth * .78, height: window.innerHeight * .82 }
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey.value) || 'null')
    if (saved && ['x', 'y', 'width', 'height'].every(key => typeof saved[key] === 'number' && Number.isFinite(saved[key]))) geometry.value = { x: saved.x, y: saved.y, width: saved.width, height: saved.height }
  } catch { /* Storage may be unavailable in private browsing. */ }
  clampGeometry()
}

function clampMiniPosition() {
  const maxX = Math.max(0, window.innerWidth - MINI_WIDTH)
  const maxY = Math.max(0, window.innerHeight - MINI_HEIGHT)
  miniPosition.value = {
    x: Math.max(0, Math.min(maxX, miniPosition.value.x)),
    y: Math.max(0, Math.min(maxY, miniPosition.value.y)),
  }
}

function restoreMiniPosition() {
  miniPosition.value = {
    x: Math.max(0, window.innerWidth - MINI_WIDTH - 24),
    y: 24,
  }
  try {
    const saved = JSON.parse(localStorage.getItem(miniStorageKey.value) || 'null')
    if (saved && typeof saved.x === 'number' && Number.isFinite(saved.x) && typeof saved.y === 'number' && Number.isFinite(saved.y)) {
      miniPosition.value = { x: saved.x, y: saved.y }
    }
  } catch { /* Storage may be unavailable in private browsing. */ }
  clampMiniPosition()
}

function saveMiniPosition() {
  try { localStorage.setItem(miniStorageKey.value, JSON.stringify({ x: miniPosition.value.x, y: miniPosition.value.y })) } catch { /* Keep in-memory position. */ }
}

function saveBoardState() {
  try {
    localStorage.setItem(stateStorageKey.value, JSON.stringify({ open: true, minimized: minimized.value }))
  } catch { /* Keep in-memory state. */ }
}

function clearBoardState() {
  try { localStorage.removeItem(stateStorageKey.value) } catch { /* Ignore unavailable storage. */ }
}

function restoreBoardState() {
  if (props.show) return
  try {
    const saved = JSON.parse(localStorage.getItem(stateStorageKey.value) || 'null')
    if (saved?.open !== true) return
    restoringFromStorage.value = true
    minimized.value = saved.minimized === true
    emit('update:show', true)
  } catch { /* Storage may be unavailable in private browsing. */ }
}

function startGesture(event: PointerEvent, edge = '') {
  if (mobile.value || event.button !== 0 || (event.target instanceof Element && event.target.closest('button'))) return
  event.preventDefault()
  gesture.value = { pointerId: event.pointerId, edge, x: event.clientX, y: event.clientY, start: { ...geometry.value } }
  panel.value?.setPointerCapture(event.pointerId)
}

function startMiniGesture(event: PointerEvent) {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  event.preventDefault()
  const target = event.currentTarget as HTMLElement | null
  miniGesture.value = {
    pointerId: event.pointerId,
    x: event.clientX,
    y: event.clientY,
    originX: miniPosition.value.x,
    originY: miniPosition.value.y,
    moved: false,
  }
  target?.setPointerCapture?.(event.pointerId)
}

function moveMiniGesture(event: PointerEvent) {
  const active = miniGesture.value
  if (!active || active.pointerId !== event.pointerId) return
  const dx = event.clientX - active.x
  const dy = event.clientY - active.y
  if (!active.moved && (Math.abs(dx) > 4 || Math.abs(dy) > 4)) active.moved = true
  if (!active.moved) return
  event.preventDefault()
  miniPosition.value = { x: active.originX + dx, y: active.originY + dy }
  clampMiniPosition()
}

function finishMiniGesture(cancelled = false) {
  const active = miniGesture.value
  if (!active) return
  miniGesture.value = null
  if (miniPanel.value?.hasPointerCapture(active.pointerId)) miniPanel.value.releasePointerCapture(active.pointerId)
  if (active.moved || !cancelled) saveMiniPosition()
  if (!cancelled && !active.moved) minimized.value = false
}

function handleMiniKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' && event.key !== ' ') return
  event.preventDefault()
  minimized.value = false
}

function restoreBoard() {
  minimized.value = false
}

function moveGesture(event: PointerEvent) {
  const active = gesture.value
  if (!active || active.pointerId !== event.pointerId) return
  const dx = event.clientX - active.x
  const dy = event.clientY - active.y
  const start = active.start
  if (!active.edge) geometry.value = { ...start, x: start.x + dx, y: start.y + dy }
  else {
    const minW = Math.min(420, window.innerWidth)
    const minH = Math.min(320, window.innerHeight)
    const left = active.edge.includes('w') ? Math.max(0, Math.min(start.x + start.width - minW, start.x + dx)) : start.x
    const top = active.edge.includes('n') ? Math.max(0, Math.min(start.y + start.height - minH, start.y + dy)) : start.y
    const right = active.edge.includes('e') ? Math.min(window.innerWidth, Math.max(start.x + minW, start.x + start.width + dx)) : start.x + start.width
    const bottom = active.edge.includes('s') ? Math.min(window.innerHeight, Math.max(start.y + minH, start.y + start.height + dy)) : start.y + start.height
    geometry.value = { x: left, y: top, width: right - left, height: bottom - top }
  }
  clampGeometry()
}

function finishGesture() {
  const active = gesture.value
  if (!active) return
  gesture.value = null
  if (panel.value?.hasPointerCapture(active.pointerId)) panel.value.releasePointerCapture(active.pointerId)
  try { localStorage.setItem(storageKey.value, JSON.stringify(geometry.value)) } catch { /* Keep in-memory geometry. */ }
}

function onResize() { mobile.value = window.innerWidth <= 680; clampGeometry(); clampMiniPosition() }
function fitContent() {
  iframe.value?.contentWindow?.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'fit-content' }, window.location.origin)
}
let requestId = ''
let closeTimer: ReturnType<typeof setTimeout> | null = null

function resource() {
  return generateWorldClueBoardLink({ worldId: surfaceWorldId.value, channelId: surfaceChannelId.value })
}

function loadLink() {
  if (!surfaceWorldId.value || !surfaceChannelId.value) {
    link.value = ''
    ready.value = false
    closing.value = false
    return
  }
  link.value = resource().url
  ready.value = false
  closing.value = false
}

function onMessage(event: MessageEvent) {
  if (event.origin !== window.location.origin || event.source !== iframe.value?.contentWindow) return
  if (event.data?.type !== 'sealchat.world-clue-board.lifecycle') return
  if (event.data.state === 'ready') {
    ready.value = true
    return
  }
  if (event.data.state === 'close-result' && event.data.requestId === requestId) {
    if (closeTimer) clearTimeout(closeTimer)
    closeTimer = null
    closing.value = false
    if (event.data.accepted === true) {
      clearBoardState()
      emit('update:show', false)
    }
    else message.warning('线索板仍有未完成的编辑，已保留窗口')
  }
}

function requestClose() {
  if (closing.value) return
  if (!link.value || !iframe.value?.contentWindow) {
    clearBoardState()
    emit('update:show', false)
    return
  }
  if (!ready.value) {
    // Board App 尚未进入可编辑 ready 状态，无需 flush。
    clearBoardState()
    emit('update:show', false)
    return
  }
  requestId = `${Date.now()}-${Math.random().toString(36).slice(2)}`
  closing.value = true
  iframe.value.contentWindow.postMessage({ type: 'sealchat.world-clue-board.lifecycle', state: 'request-close', requestId }, window.location.origin)
  closeTimer = setTimeout(() => {
    closeTimer = null
    closing.value = false
    message.warning('线索板关闭确认超时，已保留窗口')
  }, 5000)
}

async function copyLink() {
  if (!link.value) return
  await copyTextWithFallback(link.value)
  message.success('线索板链接已复制')
}

function openStandalone() {
  if (!link.value) return
  const opened = openInternalSurfaceLink(link.value, { width: 1100, height: 760 })
  if (!opened) message.warning('无法打开独立窗口，请允许浏览器弹窗')
}

watch(() => props.show, visible => {
  if (!visible) return
  const restoring = restoringFromStorage.value
  restoringFromStorage.value = false
  if (!restoring) minimized.value = false
  surfaceWorldId.value = props.worldId
  surfaceChannelId.value = props.channelId
  restoreGeometry()
  restoreMiniPosition()
  loadLink()
  saveBoardState()
}, { immediate: true })
watch(storageKey, () => { if (props.show) restoreGeometry() })
watch(miniStorageKey, () => { if (props.show) restoreMiniPosition() })
watch(minimized, () => { if (props.show) saveBoardState() })
watch(() => props.worldId, (worldId, previousWorldId) => {
  if (props.show && worldId !== previousWorldId) requestClose()
})
onBeforeUnmount(() => {
  window.removeEventListener('message', onMessage)
  window.removeEventListener('resize', onResize)
  if (closeTimer) clearTimeout(closeTimer)
})
onMounted(() => {
  window.addEventListener('message', onMessage)
  window.addEventListener('resize', onResize)
  restoreBoardState()
})

defineExpose({ restoreBoard })
</script>

<template>
  <Teleport to="body">
  <section v-if="show" v-show="!minimized" ref="panel" class="world-clue-board-embed" :class="{ 'is-moving': gesture }" :style="panelStyle" role="dialog" aria-label="线索板" @pointermove="moveGesture" @pointerup="finishGesture" @pointercancel="finishGesture" @lostpointercapture="finishGesture">
    <header class="world-clue-board-embed__header" @pointerdown="startGesture($event)">
      <strong>线索板</strong>
      <div class="world-clue-board-embed__actions">
        <NTooltip><template #trigger><NButton quaternary circle size="small" aria-label="复制链接" :disabled="!link" @click="copyLink"><template #icon><NIcon><Copy /></NIcon></template></NButton></template>复制链接</NTooltip>
        <NTooltip><template #trigger><NButton quaternary circle size="small" aria-label="独立打开" :disabled="!link" @click="openStandalone"><template #icon><NIcon><ExternalLink /></NIcon></template></NButton></template>独立打开</NTooltip>
        <NTooltip><template #trigger><NButton quaternary circle size="small" aria-label="适配内容" :disabled="!ready" @click="fitContent"><template #icon><NIcon><Maximize /></NIcon></template></NButton></template>适配内容</NTooltip>
        <NTooltip><template #trigger><NButton quaternary circle size="small" aria-label="最小化" @click="minimized = true"><template #icon><NIcon><Minus /></NIcon></template></NButton></template>最小化</NTooltip>
        <NTooltip><template #trigger><NButton quaternary circle size="small" aria-label="关闭" :disabled="closing" @click="requestClose"><template #icon><NIcon><X /></NIcon></template></NButton></template>关闭</NTooltip>
      </div>
    </header>
    <div v-if="!link" class="world-clue-board-embed__empty">没有可用的同世界频道上下文，已禁用线索板入口。</div>
    <iframe v-else ref="iframe" class="world-clue-board-embed__frame" :src="link" title="线索板" />
    <div v-for="edge in resizeEdges" :key="edge" class="world-clue-board-embed__resize" :class="`edge-${edge}`" @pointerdown.stop="startGesture($event, edge)" />
  </section>
  <div
    v-if="show && minimized"
    ref="miniPanel"
    class="world-clue-board-mini"
    :class="{ 'is-moving': miniGesture }"
    :style="miniStyle"
    role="button"
    tabindex="0"
    aria-label="线索板"
    @pointerdown="startMiniGesture"
    @pointermove="moveMiniGesture"
    @pointerup="finishMiniGesture()"
    @pointercancel="finishMiniGesture(true)"
    @lostpointercapture="finishMiniGesture(true)"
    @keydown="handleMiniKeydown"
  >
    <NIcon><LayoutBoard /></NIcon><span>线索板</span>
  </div>
  </Teleport>
</template>

<style scoped>
.world-clue-board-embed { position: fixed; z-index: 2100; display: flex; flex-direction: column; border: 0; border-radius: 14px; color: var(--sc-text-primary); background: var(--sc-bg-elevated); box-shadow: 0 20px 80px #0005; }
.world-clue-board-embed__header { display: flex; align-items: center; justify-content: space-between; flex: 0 0 44px; padding: 0 12px; cursor: grab; touch-action: none; user-select: none; }
.world-clue-board-embed__actions { display: flex; gap: 4px; }
.world-clue-board-embed__frame { display: block; flex: 1; width: 100%; min-height: 0; border: 0; border-radius: 0 0 14px 14px; background: var(--sc-bg-surface); }
.world-clue-board-embed.is-moving .world-clue-board-embed__frame { pointer-events: none; }
.world-clue-board-embed__resize { position: absolute; touch-action: none; }
.edge-n, .edge-s { height: 8px; left: 10px; right: 10px; cursor: ns-resize; }
.edge-n { top: -4px; } .edge-s { bottom: -4px; }
.edge-e, .edge-w { width: 8px; top: 10px; bottom: 10px; cursor: ew-resize; }
.edge-e { right: -4px; } .edge-w { left: -4px; }
.edge-ne, .edge-nw, .edge-se, .edge-sw { width: 14px; height: 14px; }
.edge-ne, .edge-sw { cursor: nesw-resize; } .edge-nw, .edge-se { cursor: nwse-resize; }
.edge-ne { top: -4px; right: -4px; } .edge-nw { top: -4px; left: -4px; }
.edge-se { bottom: -4px; right: -4px; } .edge-sw { bottom: -4px; left: -4px; }
.world-clue-board-embed__empty { display: grid; min-height: 360px; place-items: center; color: var(--sc-text-secondary); }
.world-clue-board-mini { position: fixed; z-index: 2101; display: inline-flex; box-sizing: border-box; align-items: center; justify-content: center; gap: 5px; border: 1px solid var(--sc-border-strong); border-radius: 8px; padding: 0 8px; color: var(--sc-text-primary); background: var(--sc-bg-elevated); box-shadow: 0 2px 6px rgba(15, 23, 42, 0.12); font-size: .78rem; font-weight: 500; cursor: grab; touch-action: none; user-select: none; }
.world-clue-board-mini.is-moving { cursor: grabbing; }
@media (max-width: 680px) { .world-clue-board-embed { inset: 0; width: 100vw; height: 100dvh; border-radius: 0; } .world-clue-board-embed__frame { border-radius: 0; } .world-clue-board-embed__resize { display: none; } }
</style>
