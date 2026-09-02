<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { X as CloseIcon } from '@vicons/tabler'
import { NIcon } from 'naive-ui'

export interface WorldMessageToastPayload {
  worldId: string
  channelId: string
  messageId: string
  channelName: string
  speakerName: string
  preview: string
  createdAt: number
}

interface ToastItem extends WorldMessageToastPayload {
  key: string
  remainingMs: number
  startedAt: number
  timer: ReturnType<typeof setTimeout> | null
  hovering: boolean
  focused: boolean
  offsetX: number
  swipeSuppressClick: boolean
  gesture: {
    pointerId: number
    startX: number
    moved: boolean
  } | null
}

const TOAST_DURATION_MS = 5000
const MAX_TOASTS = 4
const SWIPE_THRESHOLD_PX = 56
const MAX_TEXT_LENGTH = 120
const hoverCapable = typeof window === 'undefined' || typeof window.matchMedia !== 'function'
  ? true
  : window.matchMedia('(hover: hover)').matches

const emit = defineEmits<{
  (event: 'select', payload: WorldMessageToastPayload): void
}>()

const items = ref<ToastItem[]>([])

const normalizeText = (value: unknown, fallback: string, maxLength = MAX_TEXT_LENGTH) => {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return text.length > maxLength ? `${text.slice(0, maxLength - 1)}…` : text
}

const normalizePayload = (payload: WorldMessageToastPayload): WorldMessageToastPayload | null => {
  const worldId = String(payload?.worldId || '').trim()
  const channelId = String(payload?.channelId || '').trim()
  const messageId = String(payload?.messageId || '').trim()
  if (!worldId || !channelId || !messageId) return null
  const createdAt = Number(payload?.createdAt)
  return {
    worldId,
    channelId,
    messageId,
    channelName: normalizeText(payload?.channelName, '未知频道', 64),
    speakerName: normalizeText(payload?.speakerName, '未知角色', 64),
    preview: normalizeText(payload?.preview, '发送了一条消息'),
    createdAt: Number.isFinite(createdAt) && createdAt > 0 ? createdAt : Date.now(),
  }
}

const clearTimer = (item: ToastItem) => {
  if (!item.timer) return
  clearTimeout(item.timer)
  item.timer = null
}

const dismiss = (key: string) => {
  const item = items.value.find((entry) => entry.key === key)
  if (!item) return
  clearTimer(item)
  items.value = items.value.filter((entry) => entry.key !== key)
}

const scheduleDismiss = (item: ToastItem) => {
  clearTimer(item)
  if (item.remainingMs <= 0) {
    dismiss(item.key)
    return
  }
  item.startedAt = Date.now()
  item.timer = setTimeout(() => dismiss(item.key), item.remainingMs)
}

const pauseItem = (item: ToastItem) => {
  if (!item.timer) return
  item.remainingMs = Math.max(0, item.remainingMs - (Date.now() - item.startedAt))
  clearTimer(item)
}

const syncItemTimer = (item: ToastItem) => {
  if (item.hovering || item.focused) {
    pauseItem(item)
  } else if (!item.timer) {
    scheduleDismiss(item)
  }
}

const enqueue = (payload: WorldMessageToastPayload) => {
  const normalized = normalizePayload(payload)
  if (!normalized) return
  const key = `${normalized.worldId}:${normalized.channelId}:${normalized.messageId}`
  if (items.value.some((item) => item.key === key)) return

  const item: ToastItem = {
    ...normalized,
    key,
    remainingMs: TOAST_DURATION_MS,
    startedAt: Date.now(),
    timer: null,
    hovering: false,
    focused: false,
    offsetX: 0,
    swipeSuppressClick: false,
    gesture: null,
  }
  items.value = [...items.value, item]
  scheduleDismiss(item)
  if (items.value.length > MAX_TOASTS) {
    dismiss(items.value[0].key)
  }
}

const handleHover = (item: ToastItem, hovering: boolean) => {
  if (!hoverCapable) return
  item.hovering = hovering
  syncItemTimer(item)
}

const handleFocusIn = (item: ToastItem) => {
  item.focused = true
  syncItemTimer(item)
}

const handleFocusOut = (item: ToastItem, event: FocusEvent) => {
  const current = event.currentTarget as HTMLElement | null
  const next = event.relatedTarget as Node | null
  if (current && next && current.contains(next)) return
  item.focused = false
  syncItemTimer(item)
}

const handlePointerDown = (item: ToastItem, event: PointerEvent) => {
  if (event.pointerType === 'mouse') return
  item.gesture = {
    pointerId: event.pointerId,
    startX: event.clientX,
    moved: false,
  }
  item.offsetX = 0
  item.swipeSuppressClick = false
  const target = event.currentTarget as HTMLElement | null
  target?.setPointerCapture?.(event.pointerId)
}

const handlePointerMove = (item: ToastItem, event: PointerEvent) => {
  if (!item.gesture || item.gesture.pointerId !== event.pointerId) return
  const delta = event.clientX - item.gesture.startX
  if (Math.abs(delta) < 4) return
  item.gesture.moved = true
  item.offsetX = delta
  if (event.cancelable) event.preventDefault()
}

const handlePointerEnd = (item: ToastItem, event: PointerEvent) => {
  if (!item.gesture || item.gesture.pointerId !== event.pointerId) return
  const delta = event.clientX - item.gesture.startX
  const moved = item.gesture.moved
  item.gesture = null
  if (Math.abs(delta) >= SWIPE_THRESHOLD_PX) {
    item.swipeSuppressClick = true
    if (event.cancelable) event.preventDefault()
    dismiss(item.key)
    return
  }
  if (moved) {
    item.swipeSuppressClick = true
    item.offsetX = 0
    setTimeout(() => {
      item.swipeSuppressClick = false
    }, 0)
  }
}

const handleToastClick = (item: ToastItem, event: MouseEvent) => {
  if (item.swipeSuppressClick) {
    event.preventDefault()
    event.stopPropagation()
    item.swipeSuppressClick = false
    return
  }
  dismiss(item.key)
  emit('select', {
    worldId: item.worldId,
    channelId: item.channelId,
    messageId: item.messageId,
    channelName: item.channelName,
    speakerName: item.speakerName,
    preview: item.preview,
    createdAt: item.createdAt,
  })
}

const handleToastKeydown = (item: ToastItem, event: KeyboardEvent) => {
  if (event.key !== 'Enter' && event.key !== ' ') return
  if ((event.target as HTMLElement | null)?.closest('button')) return
  event.preventDefault()
  handleToastClick(item, event as unknown as MouseEvent)
}

const handleDismissClick = (item: ToastItem, event: MouseEvent) => {
  event.preventDefault()
  event.stopPropagation()
  dismiss(item.key)
}

const toastStyle = (item: ToastItem) => item.offsetX
  ? {
      transform: `translate3d(${item.offsetX}px, 0, 0)`,
      transition: item.gesture ? 'none' : 'transform 160ms ease-out',
    }
  : undefined

const dismissAll = () => {
  items.value.forEach(clearTimer)
  items.value = []
}

const dismissChannel = (worldId: string, channelId: string) => {
  const normalizedWorldId = String(worldId || '').trim()
  const normalizedChannelId = String(channelId || '').trim()
  if (!normalizedWorldId || !normalizedChannelId) return
  items.value
    .filter((item) => item.worldId === normalizedWorldId && item.channelId === normalizedChannelId)
    .forEach((item) => dismiss(item.key))
}

defineExpose({ enqueue, dismiss, dismissAll, dismissChannel })

onBeforeUnmount(() => {
  dismissAll()
})
</script>

<template>
  <div class="world-message-toast-stack" aria-live="polite" aria-label="世界内新消息">
    <TransitionGroup name="world-message-toast" tag="div" class="world-message-toast-stack__list">
      <article
        v-for="item in items"
        :key="item.key"
        class="world-message-toast"
        :class="{ 'is-dragging': item.gesture }"
        :style="toastStyle(item)"
        :data-world-id="item.worldId"
        :data-channel-id="item.channelId"
        :data-message-id="item.messageId"
        :data-created-at="String(item.createdAt)"
        tabindex="0"
        @click="handleToastClick(item, $event)"
        @keydown="handleToastKeydown(item, $event)"
        @mouseenter="handleHover(item, true)"
        @mouseleave="handleHover(item, false)"
        @focusin="handleFocusIn(item)"
        @focusout="handleFocusOut(item, $event)"
        @pointerdown="handlePointerDown(item, $event)"
        @pointermove="handlePointerMove(item, $event)"
        @pointerup="handlePointerEnd(item, $event)"
        @pointercancel="handlePointerEnd(item, $event)"
      >
        <button
          type="button"
          class="world-message-toast__close"
          aria-label="关闭消息提醒"
          title="关闭消息提醒"
          @click="handleDismissClick(item, $event)"
        >
          <NIcon :component="CloseIcon" :size="16" />
        </button>
        <div class="world-message-toast__channel">{{ item.channelName }}</div>
        <div class="world-message-toast__message">
          <span class="world-message-toast__speaker">{{ item.speakerName }}：</span>
          <span class="world-message-toast__preview">{{ item.preview }}</span>
        </div>
      </article>
    </TransitionGroup>
    <slot />
  </div>
</template>

<style scoped lang="scss">
.world-message-toast-stack {
  position: absolute;
  top: max(0.75rem, env(safe-area-inset-top));
  right: 0.75rem;
  z-index: 30;
  width: min(340px, calc(100vw - 24px));
  pointer-events: none;
}

.world-message-toast-stack__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.world-message-toast {
  position: relative;
  min-height: 58px;
  box-sizing: border-box;
  padding: 10px 36px 10px 12px;
  border: 1px solid color-mix(in srgb, var(--sc-border-strong) 72%, transparent);
  border-radius: 12px;
  background: color-mix(in srgb, var(--sc-bg-elevated) 94%, transparent);
  color: var(--sc-text-primary);
  box-shadow: 0 8px 24px color-mix(in srgb, #000 16%, transparent);
  backdrop-filter: blur(10px);
  cursor: pointer;
  pointer-events: auto;
  touch-action: pan-y;
  user-select: none;
  outline: none;
  transition: border-color 160ms ease, box-shadow 160ms ease;
}

.world-message-toast:hover,
.world-message-toast:focus-visible {
  border-color: color-mix(in srgb, var(--primary-color, #3b82f6) 48%, var(--sc-border-strong));
  box-shadow: 0 10px 28px color-mix(in srgb, #000 22%, transparent);
}

.world-message-toast.is-dragging {
  opacity: 0.72;
}

.world-message-toast__channel {
  min-width: 0;
  overflow: hidden;
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.world-message-toast__message {
  display: flex;
  min-width: 0;
  margin-top: 3px;
  font-size: 13px;
  line-height: 1.4;
}

.world-message-toast__speaker,
.world-message-toast__preview {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.world-message-toast__speaker {
  flex: 0 1 auto;
  max-width: 45%;
  color: var(--sc-text-primary);
  font-weight: 650;
}

.world-message-toast__preview {
  flex: 1 1 auto;
  min-width: 0;
  color: var(--sc-text-secondary);
}

.world-message-toast__close {
  position: absolute;
  top: 5px;
  right: 5px;
  display: inline-flex;
  width: 19px;
  height: 19px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: #dc2626;
  color: #fff;
  cursor: pointer;
  opacity: 0;
  transition: opacity 140ms ease, background-color 140ms ease;
}

.world-message-toast:hover .world-message-toast__close,
.world-message-toast:focus-within .world-message-toast__close,
.world-message-toast__close:focus-visible {
  opacity: 1;
}

.world-message-toast__close:hover {
  background: #b91c1c;
}

.world-message-toast-enter-active,
.world-message-toast-leave-active {
  transition: opacity 180ms ease, transform 180ms ease;
}

.world-message-toast-enter-from,
.world-message-toast-leave-to {
  opacity: 0;
  transform: translate3d(8px, 0, 0);
}

.world-message-toast-leave-active {
  position: absolute;
  width: 100%;
}

.world-message-toast-move {
  transition: transform 180ms ease;
}

@media (hover: none), (pointer: coarse) {
  .world-message-toast__close {
    opacity: 1;
  }
}

@media (max-width: 520px) {
  .world-message-toast-stack {
    right: 12px;
    width: min(340px, calc(100vw - 24px));
  }
}

:root[data-display-palette='night'] .world-message-toast {
  box-shadow: 0 10px 28px color-mix(in srgb, #000 38%, transparent);
}

@media (prefers-reduced-motion: reduce) {
  .world-message-toast,
  .world-message-toast-enter-active,
  .world-message-toast-leave-active,
  .world-message-toast-move {
    transition-duration: 1ms;
  }
}
</style>
