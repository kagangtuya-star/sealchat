<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { NIcon, NTooltip } from 'naive-ui';
import { Copy, Archive, Trash, Photo, BoxMultiple, X, ArrowsVertical, DotsVertical } from '@vicons/tabler';
import { useChatStore } from '@/stores/chat';

const chat = useChatStore();

const emit = defineEmits<{
  (e: 'copy'): void;
  (e: 'archive'): void;
  (e: 'delete'): void;
  (e: 'copy-image'): void;
  (e: 'select-all'): void;
  (e: 'range-select'): void;
  (e: 'cancel'): void;
}>();

const selectedCount = computed(() => chat.multiSelect?.selectedIds.size ?? 0);
const hasSelection = computed(() => selectedCount.value > 0);
const isActive = computed(() => chat.multiSelect?.active ?? false);
const rangeModeEnabled = computed(() => chat.multiSelect?.rangeModeEnabled ?? false);
const tooltipZIndex = 2200;
const tooltipPlacement = 'top';
const FLOATING_BAR_MARGIN = 12;
const barRef = ref<HTMLElement | null>(null);
const draggedPosition = ref<{ left: number; top: number } | null>(null);
const dragging = ref(false);
const dragState = ref<{
  pointerId: number;
  offsetX: number;
  offsetY: number;
  captureTarget: HTMLElement | null;
} | null>(null);
const rangeHint = computed(() => {
  if (!rangeModeEnabled.value) return '';
  if (!chat.multiSelect?.rangeAnchorId) return '点击消息选择起点';
  return '点击另一条消息完成范围选择';
});
const barStyle = computed(() => {
  if (!draggedPosition.value) {
    return undefined;
  }
  return {
    left: `${draggedPosition.value.left}px`,
    top: `${draggedPosition.value.top}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
  };
});

const resolveViewportSize = () => {
  if (typeof window === 'undefined') {
    return { width: 1280, height: 720 };
  }
  return { width: window.innerWidth, height: window.innerHeight };
};

const clampPosition = (left: number, top: number) => {
  const element = barRef.value;
  if (!element) {
    return { left, top };
  }
  const { width, height } = element.getBoundingClientRect();
  const viewport = resolveViewportSize();
  const maxLeft = Math.max(FLOATING_BAR_MARGIN, viewport.width - width - FLOATING_BAR_MARGIN);
  const maxTop = Math.max(FLOATING_BAR_MARGIN, viewport.height - height - FLOATING_BAR_MARGIN);

  return {
    left: Math.min(Math.max(FLOATING_BAR_MARGIN, left), maxLeft),
    top: Math.min(Math.max(FLOATING_BAR_MARGIN, top), maxTop),
  };
};

const syncDraggedPositionToViewport = () => {
  if (!draggedPosition.value) {
    return;
  }
  draggedPosition.value = clampPosition(draggedPosition.value.left, draggedPosition.value.top);
};

const resetFloatingPosition = () => {
  draggedPosition.value = null;
};

const stopDragging = () => {
  dragging.value = false;
  dragState.value?.captureTarget?.releasePointerCapture?.(dragState.value.pointerId);
  dragState.value = null;
};

const handleDragMove = (event: PointerEvent) => {
  if (!dragState.value || event.pointerId !== dragState.value.pointerId) {
    return;
  }
  event.preventDefault();
  draggedPosition.value = clampPosition(
    event.clientX - dragState.value.offsetX,
    event.clientY - dragState.value.offsetY,
  );
};

const handleDragEnd = (event: PointerEvent) => {
  if (!dragState.value || event.pointerId !== dragState.value.pointerId) {
    return;
  }
  stopDragging();
};

const startDragging = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return;
  }
  const element = barRef.value;
  const target = event.currentTarget as HTMLElement | null;
  if (!element || !target) {
    return;
  }

  const rect = element.getBoundingClientRect();
  draggedPosition.value = clampPosition(rect.left, rect.top);
  dragState.value = {
    pointerId: event.pointerId,
    offsetX: event.clientX - rect.left,
    offsetY: event.clientY - rect.top,
    captureTarget: target,
  };
  dragging.value = true;
  target.setPointerCapture?.(event.pointerId);
  event.preventDefault();
};

watch(
  () => isActive.value,
  async (active) => {
    if (active) {
      await nextTick();
      syncDraggedPositionToViewport();
      return;
    }
    stopDragging();
  },
  { immediate: true },
);

watch(rangeModeEnabled, () => {
  nextTick(() => syncDraggedPositionToViewport());
});

watch(selectedCount, () => {
  nextTick(() => syncDraggedPositionToViewport());
});

const handleWindowResize = () => {
  syncDraggedPositionToViewport();
};

if (typeof window !== 'undefined') {
  window.addEventListener('pointermove', handleDragMove, { passive: false });
  window.addEventListener('pointerup', handleDragEnd);
  window.addEventListener('pointercancel', handleDragEnd);
  window.addEventListener('resize', handleWindowResize);
}

onBeforeUnmount(() => {
  stopDragging();
  if (typeof window !== 'undefined') {
    window.removeEventListener('pointermove', handleDragMove);
    window.removeEventListener('pointerup', handleDragEnd);
    window.removeEventListener('pointercancel', handleDragEnd);
    window.removeEventListener('resize', handleWindowResize);
  }
});

const handleCancel = () => {
  chat.exitMultiSelectMode();
  emit('cancel');
};

const handleToggleRangeMode = () => {
  chat.toggleRangeMode();
};
</script>

<template>
  <Transition name="slide-up" @after-leave="resetFloatingPosition">
    <div
      v-if="isActive"
      ref="barRef"
      class="multi-select-bar"
      :class="{ 'is-dragging': dragging, 'is-floating': !!draggedPosition }"
      :style="barStyle"
    >
      <button
        type="button"
        class="multi-select-bar__drag-handle"
        :class="{ 'is-dragging': dragging }"
        aria-label="拖动多选工具栏"
        title="拖动工具栏"
        @pointerdown.stop.prevent="startDragging"
      >
        <n-icon :size="18"><DotsVertical /></n-icon>
      </button>

      <div class="multi-select-bar__info">
        <span class="multi-select-bar__count">已选 {{ selectedCount }} 条</span>
        <span v-if="rangeHint" class="multi-select-bar__hint">{{ rangeHint }}</span>
      </div>
      
      <div class="multi-select-bar__actions">
        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              :class="{ 'is-disabled': !hasSelection }"
              :disabled="!hasSelection"
              @click="emit('copy')"
            >
              <n-icon :size="16"><Copy /></n-icon>
              <span>复制</span>
            </button>
          </template>
          复制选中消息（带时间戳）
        </n-tooltip>

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              :class="{ 'is-disabled': !hasSelection }"
              :disabled="!hasSelection"
              @click="emit('archive')"
            >
              <n-icon :size="16"><Archive /></n-icon>
              <span>归档</span>
            </button>
          </template>
          批量归档选中消息
        </n-tooltip>

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button multi-select-bar__button--danger"
              :class="{ 'is-disabled': !hasSelection }"
              :disabled="!hasSelection"
              @click="emit('delete')"
            >
              <n-icon :size="16"><Trash /></n-icon>
              <span>删除</span>
            </button>
          </template>
          批量删除选中消息
        </n-tooltip>

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              :class="{ 'is-disabled': !hasSelection }"
              :disabled="!hasSelection"
              @click="emit('copy-image')"
            >
              <n-icon :size="16"><Photo /></n-icon>
              <span>复制为图片</span>
            </button>
          </template>
          将选中消息渲染为图片并复制
        </n-tooltip>

        <div class="multi-select-bar__divider"></div>

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              @click="emit('select-all')"
            >
              <n-icon :size="16"><BoxMultiple /></n-icon>
              <span>全选</span>
            </button>
          </template>
          选中当前所有可见消息
        </n-tooltip>

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              :class="{ 'is-active': rangeModeEnabled }"
              @click="handleToggleRangeMode"
            >
              <n-icon :size="16"><ArrowsVertical /></n-icon>
              <span>范围</span>
            </button>
          </template>
          {{ rangeModeEnabled ? '关闭范围选择模式' : '开启范围选择：点击起点再点击终点' }}
        </n-tooltip>

        <button
          class="multi-select-bar__button multi-select-bar__button--cancel"
          @click="handleCancel"
        >
          <n-icon :size="16"><X /></n-icon>
          <span>取消</span>
        </button>
      </div>
    </div>
  </Transition>
</template>

<style lang="scss" scoped>
.multi-select-bar {
  position: fixed;
  bottom: 80px;
  left: 50%;
  width: fit-content;
  transform: translateX(-50%);
  z-index: 2100;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid rgba(15, 23, 42, 0.12);
  box-shadow: 0 12px 40px rgba(15, 23, 42, 0.18);
  backdrop-filter: blur(12px);
  color: #111827;
  box-sizing: border-box;
  max-width: calc(100vw - 24px);
  user-select: none;
  overflow: visible;
}

:root[data-display-palette='night'] .multi-select-bar {
  background: rgba(20, 24, 36, 0.95);
  border-color: rgba(255, 255, 255, 0.1);
  color: rgba(248, 250, 252, 0.95);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
}

.multi-select-bar.is-dragging {
  transition: none;
}

.multi-select-bar__drag-handle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  align-self: stretch;
  width: 28px;
  min-height: 36px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: grab;
  touch-action: none;
  flex-shrink: 0;

  &:hover {
    background: rgba(15, 23, 42, 0.08);
  }

  &.is-dragging {
    cursor: grabbing;
    background: rgba(15, 23, 42, 0.12);
  }
}

:root[data-display-palette='night'] .multi-select-bar__drag-handle {
  &:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  &.is-dragging {
    background: rgba(255, 255, 255, 0.16);
  }
}

.multi-select-bar__info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 80px;
}

.multi-select-bar__count {
  font-weight: 600;
  font-size: 14px;
}

.multi-select-bar__hint {
  font-size: 11px;
  opacity: 0.6;
}

.multi-select-bar__actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.multi-select-bar__divider {
  width: 1px;
  height: 24px;
  background: rgba(15, 23, 42, 0.1);
  margin: 0 8px;
}

:root[data-display-palette='night'] .multi-select-bar__divider {
  background: rgba(255, 255, 255, 0.1);
}

.multi-select-bar__button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  padding: 6px 12px;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover:not(.is-disabled) {
    background: rgba(15, 23, 42, 0.08);
  }

  &.is-disabled {
    opacity: 0.4;
    pointer-events: none;
  }

  &--danger {
    color: #ef4444;
  }

  &.is-active {
    background: rgba(59, 130, 246, 0.15);
    color: #3b82f6;
  }

  &--cancel {
    opacity: 0.7;
    &:hover {
      opacity: 1;
    }
  }
}

:root[data-display-palette='night'] .multi-select-bar__button {
  &:hover:not(.is-disabled) {
    background: rgba(255, 255, 255, 0.1);
  }
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.25s ease;
}

.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateX(-50%) translateY(20px);
  opacity: 0;
}

@media (max-width: 768px) {
  .multi-select-bar {
    bottom: 70px;
    width: auto;
    left: 8px;
    right: 8px;
    transform: none;
    flex-wrap: wrap;
    justify-content: center;
    gap: 8px;
  }

  .multi-select-bar__info {
    width: 100%;
    flex-direction: row;
    justify-content: center;
    gap: 8px;
  }

  .multi-select-bar__drag-handle {
    position: absolute;
    top: 0;
    left: 12px;
    width: 22px;
    height: 22px;
    min-height: 22px;
    padding: 0;
    border: 1px solid rgba(15, 23, 42, 0.14);
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.96);
    transform: translateY(-42%);
    align-self: auto;
    box-shadow: 0 6px 16px rgba(15, 23, 42, 0.18);
    z-index: 1;
  }

  :root[data-display-palette='night'] .multi-select-bar__drag-handle {
    background: rgba(20, 24, 36, 0.98);
    border-color: rgba(255, 255, 255, 0.12);
    box-shadow: 0 6px 16px rgba(0, 0, 0, 0.38);
  }

  .multi-select-bar__drag-handle :deep(svg) {
    width: 12px;
    height: 12px;
  }

  .multi-select-bar__button span {
    display: none;
  }

  .slide-up-enter-from,
  .slide-up-leave-to {
    transform: translateY(20px);
  }
}
</style>
