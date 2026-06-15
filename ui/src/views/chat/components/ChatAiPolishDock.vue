<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { NIcon } from 'naive-ui'
import {
  ArrowUndoOutline,
  BrushOutline,
  CloseCircleOutline,
  CloseOutline,
  ContractOutline,
  RefreshOutline,
} from '@vicons/ionicons5'
import { buildAIPolishDiffTokens } from '@/services/ai/ai-polish-diff'
import type {
  AIPolishDockState,
  AIPolishResultViewMode,
  AIPolishSlotState,
} from '@/services/ai/ai-polish-dock'

const props = defineProps<{
  visible: boolean
  faviconHref: string
  dockState: AIPolishDockState
}>()

const emit = defineEmits<{
  (event: 'restore'): void
  (event: 'toggle-minimize'): void
  (event: 'select-slot', slotIndex: number): void
  (event: 'read-current-input'): void
  (event: 'retry'): void
  (event: 'apply'): void
  (event: 'clear-slot'): void
  (event: 'close'): void
  (event: 'update:source-text', value: string): void
  (event: 'update:result-text', value: string): void
  (event: 'update:view-mode', value: AIPolishResultViewMode): void
}>()

const panelRef = ref<HTMLElement | null>(null)
const badgeRef = ref<HTMLElement | null>(null)
const panelPosition = ref({ x: 0, y: 0 })
const badgePosition = ref({ x: 0, y: 0 })
const panelSize = ref({ width: 860, height: 520 })
const badgeClickBlocked = ref(false)

const PANEL_POS_KEY = 'sealchat_ai_polish_dock_panel_pos_v1'
const PANEL_SIZE_KEY = 'sealchat_ai_polish_dock_panel_size_v1'
const BADGE_POS_KEY = 'sealchat_ai_polish_dock_badge_pos_v1'
const MIN_PANEL_WIDTH = 420
const MIN_PANEL_HEIGHT = 320
const EDGE_GAP = 8
const BADGE_FALLBACK_SIZE = 54

const emptySlot: AIPolishSlotState = {
  sourceText: '',
  resultText: '',
  status: 'idle',
  error: '',
  requestId: '',
  updatedAt: 0,
  viewMode: 'edit',
}

const activeSlot = computed(() => props.dockState.slots[props.dockState.activeSlotIndex] || emptySlot)
const activeViewMode = computed<AIPolishResultViewMode>(() => activeSlot.value.viewMode || 'edit')
const activeDiffTokens = computed(() => (
  buildAIPolishDiffTokens(activeSlot.value.sourceText, activeSlot.value.resultText)
))

const handleResultTextUpdate = (value: string) => {
  emit('update:result-text', value)
  if (activeViewMode.value !== 'edit') {
    emit('update:view-mode', 'edit')
  }
}

const switchViewMode = (viewMode: AIPolishResultViewMode) => {
  if (activeViewMode.value === viewMode) return
  emit('update:view-mode', viewMode)
}

const dragState = ref<{
  mode: 'panel' | 'badge'
  pointerId: number
  offsetX: number
  offsetY: number
  startX: number
  startY: number
  moved: boolean
  captureTarget: HTMLElement | null
} | null>(null)

const resizeState = ref<{
  pointerId: number
  startX: number
  startY: number
  startWidth: number
  startHeight: number
  captureTarget: HTMLElement | null
} | null>(null)

const slotStatusClass = (slotIndex: number) => {
  const slot = props.dockState.slots[slotIndex]
  return [
    `is-${slot?.status || 'idle'}`,
    { 'is-active': props.dockState.activeSlotIndex === slotIndex },
  ]
}

const readPosition = (key: string, fallback: { x: number; y: number }) => {
  if (typeof window === 'undefined') return fallback
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return fallback
    const parsed = JSON.parse(raw)
    if (typeof parsed?.x === 'number' && typeof parsed?.y === 'number') {
      return { x: parsed.x, y: parsed.y }
    }
  } catch {
    // ignore bad local cache
  }
  return fallback
}

const persistPosition = (key: string, value: { x: number; y: number }) => {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // ignore localStorage failure
  }
}

const readSize = (key: string, fallback: { width: number; height: number }) => {
  if (typeof window === 'undefined') return fallback
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return fallback
    const parsed = JSON.parse(raw)
    if (typeof parsed?.width === 'number' && typeof parsed?.height === 'number') {
      return { width: parsed.width, height: parsed.height }
    }
  } catch {
    // ignore bad local cache
  }
  return fallback
}

const persistSize = (key: string, value: { width: number; height: number }) => {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // ignore localStorage failure
  }
}

const clampSize = (value: { width: number; height: number }) => {
  if (typeof window === 'undefined') return value
  const maxWidth = Math.max(MIN_PANEL_WIDTH, window.innerWidth - EDGE_GAP * 2)
  const maxHeight = Math.max(MIN_PANEL_HEIGHT, window.innerHeight - EDGE_GAP * 2)
  return {
    width: Math.min(Math.max(MIN_PANEL_WIDTH, value.width), maxWidth),
    height: Math.min(Math.max(MIN_PANEL_HEIGHT, value.height), maxHeight),
  }
}

const clampPosition = (value: { x: number; y: number }, width: number, height: number) => {
  if (typeof window === 'undefined') return value
  const maxX = Math.max(EDGE_GAP, window.innerWidth - width - EDGE_GAP)
  const maxY = Math.max(EDGE_GAP, window.innerHeight - height - EDGE_GAP)
  return {
    x: Math.min(Math.max(EDGE_GAP, value.x), maxX),
    y: Math.min(Math.max(EDGE_GAP, value.y), maxY),
  }
}

const clampPanelPosition = (value: { x: number; y: number }) => (
  clampPosition(value, panelSize.value.width, panelSize.value.height)
)

const clampBadgePosition = (value: { x: number; y: number }) => {
  const width = badgeRef.value?.offsetWidth || BADGE_FALLBACK_SIZE
  const height = badgeRef.value?.offsetHeight || BADGE_FALLBACK_SIZE
  return clampPosition(value, width, height)
}

const panelStyle = computed(() => ({
  left: `${panelPosition.value.x}px`,
  top: `${panelPosition.value.y}px`,
  width: `${panelSize.value.width}px`,
  height: `${panelSize.value.height}px`,
}))

const badgeStyle = computed(() => ({
  left: `${badgePosition.value.x}px`,
  top: `${badgePosition.value.y}px`,
}))

const startPanelDrag = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  const target = event.currentTarget as HTMLElement | null
  const rect = panelRef.value?.getBoundingClientRect()
  if (!target || !rect) return
  target.setPointerCapture?.(event.pointerId)
  dragState.value = {
    mode: 'panel',
    pointerId: event.pointerId,
    offsetX: event.clientX - rect.left,
    offsetY: event.clientY - rect.top,
    startX: event.clientX,
    startY: event.clientY,
    moved: false,
    captureTarget: target,
  }
}

const startBadgeDrag = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  const target = event.currentTarget as HTMLElement | null
  const rect = badgeRef.value?.getBoundingClientRect()
  if (!target || !rect) return
  target.setPointerCapture?.(event.pointerId)
  dragState.value = {
    mode: 'badge',
    pointerId: event.pointerId,
    offsetX: event.clientX - rect.left,
    offsetY: event.clientY - rect.top,
    startX: event.clientX,
    startY: event.clientY,
    moved: false,
    captureTarget: target,
  }
}

const startResizing = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  const target = event.currentTarget as HTMLElement | null
  target?.setPointerCapture?.(event.pointerId)
  resizeState.value = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    startWidth: panelSize.value.width,
    startHeight: panelSize.value.height,
    captureTarget: target,
  }
}

const stopDragging = (event?: PointerEvent) => {
  const current = dragState.value
  if (!current) return
  if (event && event.pointerId !== current.pointerId) return
  current.captureTarget?.releasePointerCapture?.(current.pointerId)

  if (current.mode === 'panel') {
    panelPosition.value = clampPanelPosition(panelPosition.value)
    persistPosition(PANEL_POS_KEY, panelPosition.value)
  } else {
    badgePosition.value = clampBadgePosition(badgePosition.value)
    persistPosition(BADGE_POS_KEY, badgePosition.value)
    if (current.moved) {
      badgeClickBlocked.value = true
      window.setTimeout(() => {
        badgeClickBlocked.value = false
      }, 0)
    }
  }

  dragState.value = null
}

const stopResizing = (event?: PointerEvent) => {
  const current = resizeState.value
  if (!current) return
  if (event && event.pointerId !== current.pointerId) return
  current.captureTarget?.releasePointerCapture?.(current.pointerId)
  panelSize.value = clampSize(panelSize.value)
  panelPosition.value = clampPanelPosition(panelPosition.value)
  persistSize(PANEL_SIZE_KEY, panelSize.value)
  persistPosition(PANEL_POS_KEY, panelPosition.value)
  resizeState.value = null
}

const resetInteractions = () => {
  stopDragging()
  stopResizing()
}

const onPointerMove = (event: PointerEvent) => {
  if (resizeState.value) {
    if (event.pointerId !== resizeState.value.pointerId) return
    event.preventDefault()
    panelSize.value = clampSize({
      width: resizeState.value.startWidth + (event.clientX - resizeState.value.startX),
      height: resizeState.value.startHeight + (event.clientY - resizeState.value.startY),
    })
    panelPosition.value = clampPanelPosition(panelPosition.value)
    return
  }

  if (!dragState.value) return
  if (event.pointerId !== dragState.value.pointerId) return
  event.preventDefault()

  const movedX = Math.abs(event.clientX - dragState.value.startX)
  const movedY = Math.abs(event.clientY - dragState.value.startY)
  if (!dragState.value.moved && (movedX >= 4 || movedY >= 4)) {
    dragState.value.moved = true
  }

  const nextPosition = {
    x: event.clientX - dragState.value.offsetX,
    y: event.clientY - dragState.value.offsetY,
  }

  if (dragState.value.mode === 'panel') {
    panelPosition.value = clampPanelPosition(nextPosition)
  } else {
    badgePosition.value = clampBadgePosition(nextPosition)
  }
}

const handleWindowResize = () => {
  panelSize.value = clampSize(panelSize.value)
  panelPosition.value = clampPanelPosition(panelPosition.value)
  badgePosition.value = clampBadgePosition(badgePosition.value)
  persistSize(PANEL_SIZE_KEY, panelSize.value)
  persistPosition(PANEL_POS_KEY, panelPosition.value)
  persistPosition(BADGE_POS_KEY, badgePosition.value)
}

onMounted(() => {
  panelSize.value = clampSize(readSize(PANEL_SIZE_KEY, { width: 860, height: 520 }))
  panelPosition.value = clampPanelPosition(readPosition(PANEL_POS_KEY, {
    x: Math.max((window.innerWidth || 1200) - panelSize.value.width - 24, 24),
    y: 96,
  }))
  badgePosition.value = clampBadgePosition(readPosition(BADGE_POS_KEY, {
    x: Math.max((window.innerWidth || 1200) - BADGE_FALLBACK_SIZE - 24, 24),
    y: Math.max((window.innerHeight || 900) - 170, 96),
  }))

  window.addEventListener('pointermove', onPointerMove, { passive: false })
  window.addEventListener('pointerup', stopDragging)
  window.addEventListener('pointercancel', stopDragging)
  window.addEventListener('pointerup', stopResizing)
  window.addEventListener('pointercancel', stopResizing)
  window.addEventListener('resize', handleWindowResize)
  window.addEventListener('blur', resetInteractions)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', stopDragging)
  window.removeEventListener('pointercancel', stopDragging)
  window.removeEventListener('pointerup', stopResizing)
  window.removeEventListener('pointercancel', stopResizing)
  window.removeEventListener('resize', handleWindowResize)
  window.removeEventListener('blur', resetInteractions)
  resetInteractions()
})
</script>

<template>
  <teleport to="body">
    <div v-if="props.visible" class="chat-ai-polish-dock">
      <section
        v-if="!props.dockState.minimized"
        ref="panelRef"
        class="chat-ai-polish-dock__panel"
        :style="panelStyle"
      >
        <header class="chat-ai-polish-dock__header" @pointerdown.prevent="startPanelDrag">
          <div class="chat-ai-polish-dock__title">
            <n-icon size="16" :component="BrushOutline" />
            <span>AI 润色</span>
          </div>
          <div class="chat-ai-polish-dock__header-actions" @pointerdown.stop>
            <n-button quaternary circle size="small" @click="emit('toggle-minimize')">
              <template #icon>
                <n-icon :component="ContractOutline" />
              </template>
            </n-button>
            <n-button quaternary circle size="small" @click="emit('close')">
              <template #icon>
                <n-icon :component="CloseOutline" />
              </template>
            </n-button>
          </div>
        </header>

        <div class="chat-ai-polish-dock__slot-strip">
          <button
            v-for="slotIndex in 5"
            :key="slotIndex"
            type="button"
            class="chat-ai-polish-dock__slot-button"
            :class="slotStatusClass(slotIndex - 1)"
            @click="emit('select-slot', slotIndex - 1)"
          >
            {{ slotIndex }}
          </button>
        </div>

        <div class="chat-ai-polish-dock__actions">
          <n-button size="small" secondary @click="emit('read-current-input')">读取当前输入框</n-button>
          <n-button
            size="small"
            tertiary
            :disabled="activeSlot.status === 'loading' || !activeSlot.sourceText.trim()"
            @click="emit('retry')"
          >
            <template #icon>
              <n-icon :component="RefreshOutline" />
            </template>
            重新生成
          </n-button>
          <n-button
            size="small"
            type="primary"
            :disabled="!activeSlot.resultText.trim()"
            @click="emit('apply')"
          >
            <template #icon>
              <n-icon :component="ArrowUndoOutline" />
            </template>
            覆盖输入
          </n-button>
          <n-button
            size="small"
            quaternary
            :disabled="activeSlot.status === 'loading'"
            @click="emit('clear-slot')"
          >
            <template #icon>
              <n-icon :component="CloseCircleOutline" />
            </template>
            清空当前槽
          </n-button>
        </div>

        <div class="chat-ai-polish-dock__content">
          <div class="chat-ai-polish-dock__field">
            <div class="chat-ai-polish-dock__label">原文</div>
            <n-input
              class="chat-ai-polish-dock__textarea chat-ai-polish-dock__textarea--source"
              :value="activeSlot.sourceText"
              type="textarea"
              :rows="4"
              placeholder="可直接编辑原文，或读取当前输入框"
              @update:value="emit('update:source-text', $event)"
            />
          </div>

          <div class="chat-ai-polish-dock__field chat-ai-polish-dock__field--result">
            <div class="chat-ai-polish-dock__label">
              <div class="chat-ai-polish-dock__label-main">
                <span>润色结果</span>
                <div class="chat-ai-polish-dock__view-switch" @pointerdown.stop>
                  <button
                    type="button"
                    class="chat-ai-polish-dock__view-switch-button"
                    :class="{ 'is-active': activeViewMode === 'edit' }"
                    @click="switchViewMode('edit')"
                  >
                    纯结果
                  </button>
                  <button
                    type="button"
                    class="chat-ai-polish-dock__view-switch-button"
                    :class="{ 'is-active': activeViewMode === 'diff' }"
                    @click="switchViewMode('diff')"
                  >
                    显示改动
                  </button>
                </div>
                <span
                  v-if="activeViewMode === 'diff'"
                  class="chat-ai-polish-dock__view-hint"
                >
                  需修改结果可切回纯结果
                </span>
              </div>
              <span v-if="activeSlot.status === 'loading'" class="chat-ai-polish-dock__status">生成中</span>
              <span
                v-else-if="activeSlot.status === 'error'"
                class="chat-ai-polish-dock__status chat-ai-polish-dock__status--error"
              >
                {{ activeSlot.error || '生成失败' }}
              </span>
            </div>
            <n-input
              v-if="activeViewMode === 'edit'"
              class="chat-ai-polish-dock__textarea chat-ai-polish-dock__textarea--result"
              :value="activeSlot.resultText"
              type="textarea"
              :rows="8"
              :readonly="activeSlot.status === 'loading'"
              :placeholder="activeSlot.status === 'loading' ? 'AI 正在润色，请稍候…' : '润色结果将显示在这里'"
              @update:value="handleResultTextUpdate"
            />
            <div
              v-else
              class="chat-ai-polish-dock__diff-preview"
              role="textbox"
              aria-readonly="true"
            >
              <template v-if="activeDiffTokens.length > 0">
                <span
                  v-for="(token, index) in activeDiffTokens"
                  :key="`${index}-${token.type}-${token.text}`"
                  class="chat-ai-polish-dock__diff-token"
                  :class="[
                    `is-${token.type}`,
                    { 'is-subtle': token.subtle },
                  ]"
                >
                  {{ token.text }}
                </span>
              </template>
              <span v-else class="chat-ai-polish-dock__diff-placeholder">
                {{ activeSlot.status === 'loading' ? 'AI 正在润色，请稍候…' : '润色结果将显示在这里' }}
              </span>
            </div>
          </div>
        </div>

        <div class="chat-ai-polish-dock__resize" @pointerdown.stop.prevent="startResizing" />
      </section>

      <button
        v-else
        ref="badgeRef"
        type="button"
        class="chat-ai-polish-dock__badge"
        :style="badgeStyle"
        @click="badgeClickBlocked ? null : emit('restore')"
        @pointerdown.prevent="startBadgeDrag"
      >
        <img
          v-if="props.faviconHref"
          :src="props.faviconHref"
          alt="AI polish dock"
          class="chat-ai-polish-dock__badge-icon"
        />
        <span v-else>A</span>
      </button>
    </div>
  </teleport>
</template>

<style scoped>
.chat-ai-polish-dock {
  z-index: 2200;
}

.chat-ai-polish-dock__panel {
  position: fixed;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  box-sizing: border-box;
  border-radius: 18px;
  background: var(--sc-bg-elevated, #25262b);
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.28);
  border: 1px solid var(--sc-border-strong, rgba(255, 255, 255, 0.08));
  overflow: hidden;
  touch-action: none;
  z-index: 2201;
}

.chat-ai-polish-dock__header,
.chat-ai-polish-dock__title,
.chat-ai-polish-dock__slot-strip,
.chat-ai-polish-dock__actions {
  display: flex;
  align-items: center;
}

.chat-ai-polish-dock__header {
  justify-content: space-between;
  cursor: grab;
  user-select: none;
  flex: 0 0 auto;
}

.chat-ai-polish-dock__title {
  gap: 8px;
  font-weight: 700;
}

.chat-ai-polish-dock__header-actions,
.chat-ai-polish-dock__slot-strip,
.chat-ai-polish-dock__actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  flex: 0 0 auto;
}

.chat-ai-polish-dock__slot-button {
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 999px;
  background: var(--sc-bg-surface, rgba(255, 255, 255, 0.08));
  color: var(--sc-text-secondary, #d1d5db);
  cursor: pointer;
}

.chat-ai-polish-dock__slot-button.is-active {
  outline: 2px solid rgba(255, 255, 255, 0.3);
}

.chat-ai-polish-dock__slot-button.is-loading {
  background: #2563eb;
  color: #fff;
}

.chat-ai-polish-dock__slot-button.is-success {
  background: #16a34a;
  color: #fff;
}

.chat-ai-polish-dock__slot-button.is-error {
  background: #dc2626;
  color: #fff;
}

.chat-ai-polish-dock__content {
  display: grid;
  grid-template-columns: 1fr;
  grid-template-rows: minmax(0, 0.95fr) minmax(0, 1.05fr);
  gap: 12px;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.chat-ai-polish-dock__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 0;
  overflow: hidden;
}

.chat-ai-polish-dock__field--result {
  --sc-polish-diff-insert-bg: color-mix(in srgb, #22c55e 16%, var(--sc-bg-elevated, #25262b));
  --sc-polish-diff-insert-line: color-mix(in srgb, #22c55e 74%, var(--sc-text-primary, #f8fafc));
  --sc-polish-diff-delete-bg: color-mix(in srgb, #ef4444 6%, var(--sc-bg-elevated, #25262b));
  --sc-polish-diff-delete-line: color-mix(in srgb, #ef4444 48%, var(--sc-text-secondary, #b5b5c5));
  --sc-polish-diff-delete-opacity: 0.68;
  --sc-polish-diff-subtle-opacity: 0.52;
  min-height: 0;
}

.chat-ai-polish-dock__label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
}

.chat-ai-polish-dock__label-main {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.chat-ai-polish-dock__view-switch {
  display: inline-flex;
  align-items: center;
  padding: 2px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--sc-bg-surface, rgba(255, 255, 255, 0.08)) 92%, transparent);
  border: 1px solid color-mix(in srgb, var(--sc-border-strong, rgba(255, 255, 255, 0.12)) 78%, transparent);
  gap: 2px;
}

.chat-ai-polish-dock__view-switch-button {
  border: none;
  background: transparent;
  color: var(--sc-text-secondary, #b5b5c5);
  cursor: pointer;
  border-radius: 999px;
  padding: 3px 9px;
  font-size: 12px;
  line-height: 1.2;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.chat-ai-polish-dock__view-switch-button.is-active {
  color: var(--sc-text-primary, #f8fafc);
  background: color-mix(in srgb, var(--sc-bg-elevated, #25262b) 84%, var(--primary-color, #3b82f6) 16%);
}

.chat-ai-polish-dock__view-hint {
  font-size: 12px;
  font-weight: 400;
  color: var(--sc-text-secondary, #b5b5c5);
  white-space: nowrap;
}

.chat-ai-polish-dock__status {
  font-size: 12px;
  color: #93c5fd;
}

.chat-ai-polish-dock__status--error {
  color: #fca5a5;
}

.chat-ai-polish-dock__badge {
  position: fixed;
  width: 54px;
  height: 54px;
  border: none;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--sc-bg-elevated, #25262b);
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.28);
  cursor: pointer;
  overflow: hidden;
  touch-action: none;
  z-index: 2201;
}

.chat-ai-polish-dock__badge-icon {
  width: 22px;
  height: 22px;
  object-fit: contain;
}

.chat-ai-polish-dock__resize {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 20px;
  height: 20px;
  cursor: nwse-resize;
}

.chat-ai-polish-dock__resize::before {
  content: '';
  position: absolute;
  inset: 5px;
  border-right: 2px solid rgba(255, 255, 255, 0.45);
  border-bottom: 2px solid rgba(255, 255, 255, 0.45);
  border-bottom-right-radius: 6px;
}

.chat-ai-polish-dock__textarea {
  flex: 1;
  min-height: 0;
}

.chat-ai-polish-dock__textarea :deep(.n-input-wrapper) {
  height: 100%;
}

.chat-ai-polish-dock__textarea :deep(.n-input__textarea),
.chat-ai-polish-dock__textarea :deep(textarea) {
  height: 100%;
  min-height: 0;
}

.chat-ai-polish-dock__textarea :deep(textarea) {
  resize: none;
  overflow: auto;
}

.chat-ai-polish-dock__diff-preview {
  flex: 1;
  min-height: 0;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid var(--sc-border-strong, rgba(255, 255, 255, 0.12));
  background: var(--sc-bg-elevated, #25262b);
  color: var(--sc-text-primary, #f8fafc);
}

.chat-ai-polish-dock__diff-token {
  border-radius: 4px;
  padding: 0 1px;
}

.chat-ai-polish-dock__diff-token.is-insert {
  background: var(--sc-polish-diff-insert-bg);
  text-decoration: underline;
  text-decoration-color: var(--sc-polish-diff-insert-line);
  text-decoration-thickness: 1px;
  text-underline-offset: 3px;
  margin-left: -1px;
}

.chat-ai-polish-dock__diff-token.is-delete {
  background: var(--sc-polish-diff-delete-bg);
  text-decoration: line-through;
  text-decoration-color: var(--sc-polish-diff-delete-line);
  text-decoration-thickness: 1px;
  opacity: var(--sc-polish-diff-delete-opacity);
  font-size: 0.92em;
  padding: 0;
  margin-right: -1px;
}

.chat-ai-polish-dock__diff-token.is-subtle {
  opacity: var(--sc-polish-diff-subtle-opacity);
}

.chat-ai-polish-dock__diff-placeholder {
  color: var(--sc-text-secondary, #b5b5c5);
}

@media (max-width: 768px) {
  .chat-ai-polish-dock__panel {
    max-width: calc(100vw - 16px);
    max-height: calc(100vh - 16px);
  }
}
</style>
