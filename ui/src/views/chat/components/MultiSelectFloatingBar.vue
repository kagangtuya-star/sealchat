<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { NDropdown, NIcon, NTooltip, type DropdownOption } from 'naive-ui';
import { Copy, Archive, Trash, Photo, BoxMultiple, X, ArrowsVertical, ArrowBarToDown, DotsVertical, Focus2, ArrowForwardUp, Bookmark } from '@vicons/tabler';
import { useChatStore } from '@/stores/chat';
import type { MessageFavoriteScope } from '@/features/message-favorites/api';
import { useMessageFavoriteStore } from '@/features/message-favorites/store';

const chat = useChatStore();

const emit = defineEmits<{
  (e: 'copy'): void;
  (e: 'forward'): void;
  (e: 'favorite', scope: MessageFavoriteScope): void;
  (e: 'archive'): void;
  (e: 'delete'): void;
  (e: 'copy-image'): void;
  (e: 'move-to-bottom'): void;
  (e: 'relocate'): void;
  (e: 'select-all'): void;
  (e: 'range-select'): void;
  (e: 'cancel'): void;
  (e: 'cancel-relocate'): void;
}>();

const selectedCount = computed(() => chat.multiSelect?.selectedIds.size ?? 0);
const hasSelection = computed(() => selectedCount.value > 0);
const isActive = computed(() => chat.multiSelect?.active ?? false);
const rangeModeEnabled = computed(() => chat.multiSelect?.rangeModeEnabled ?? false);
const relocateActive = computed(() => chat.multiSelect?.relocate?.active ?? false);
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

// 收藏只占一个按钮，点击后再选择作用域。
const favoriteStore = useMessageFavoriteStore();
const favoriteMenuOptions = computed<DropdownOption[]>(() => {
  const worldId = String(chat.currentWorldId || '').trim();
  const canManage = favoriteStore.worldManageHint(worldId, chat.worldDetailMap[worldId]?.memberRole);
  return [
    { key: 'personal', label: '收藏到个人' },
    {
      key: 'world',
      label: !worldId ? '收藏到世界（未进入世界）' : canManage ? '收藏到世界' : '收藏到世界（仅世界管理员）',
      disabled: !worldId || !canManage,
    },
  ];
});
const handleFavoriteMenuShow = (show: boolean) => {
  if (show) favoriteStore.probeWorldPermission(String(chat.currentWorldId || '').trim());
};
const handleFavoriteSelect = (key: string) => {
  emit('favorite', key === 'world' ? 'world' : 'personal');
};

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
      class="multi-select-bar sc-overlay"
      data-sc-ui="selection-toolbar"
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
        <span v-if="relocateActive" class="multi-select-bar__hint">请选择目标消息</span>
        <span v-else-if="rangeHint" class="multi-select-bar__hint">{{ rangeHint }}</span>
      </div>
      
      <div class="multi-select-bar__actions">
        <template v-if="relocateActive">
          <button
            class="multi-select-bar__button multi-select-bar__button--cancel"
            @click="emit('cancel-relocate')"
          >
            <n-icon :size="16"><X /></n-icon>
            <span>取消重定位</span>
          </button>
        </template>
        <template v-else>
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
              @click="emit('forward')"
            >
              <n-icon :size="16"><ArrowForwardUp /></n-icon>
              <span>转发</span>
            </button>
          </template>
          转发选中消息
        </n-tooltip>

        <n-dropdown
          trigger="click"
          placement="top"
          :z-index="tooltipZIndex"
          :options="favoriteMenuOptions"
          :disabled="!hasSelection"
          @update:show="handleFavoriteMenuShow"
          @select="handleFavoriteSelect"
        >
          <button
            class="multi-select-bar__button"
            :class="{ 'is-disabled': !hasSelection }"
            :disabled="!hasSelection"
            title="收藏选中消息到个人或世界"
          >
            <n-icon :size="16"><Bookmark /></n-icon>
            <span>收藏</span>
          </button>
        </n-dropdown>

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

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              :class="{ 'is-disabled': !hasSelection }"
              :disabled="!hasSelection"
              @click="emit('move-to-bottom')"
            >
              <n-icon :size="16"><ArrowBarToDown /></n-icon>
              <span>置底</span>
            </button>
          </template>
          将选中消息移动到当前列表底部，并保持原有顺序
        </n-tooltip>

        <n-tooltip trigger="hover" :z-index="tooltipZIndex" :placement="tooltipPlacement">
          <template #trigger>
            <button
              class="multi-select-bar__button"
              :class="{ 'is-disabled': !hasSelection }"
              :disabled="!hasSelection"
              @click="emit('relocate')"
            >
              <n-icon :size="16"><Focus2 /></n-icon>
              <span>重定位</span>
            </button>
          </template>
          选择目标消息后，将选中消息移动到该消息下方并保持顺序
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
        </template>
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
  background: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-mute);
  box-shadow: var(--chat-message-shadow);
  backdrop-filter: blur(12px);
  color: var(--sc-text-primary);
  font-family: var(--sc-font-family);
  box-sizing: border-box;
  max-width: calc(100vw - 24px);
  user-select: none;
  overflow: visible;
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
    background: var(--sc-bg-hover);
  }

  &.is-dragging {
    cursor: grabbing;
    background: var(--sc-bg-active);
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
  background: var(--sc-border-mute);
  margin: 0 8px;
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
    background: var(--sc-bg-hover);
  }

  &.is-disabled {
    opacity: 0.4;
    pointer-events: none;
  }

  &--danger {
    color: #ef4444;
  }

  &.is-active {
    background: var(--sc-bg-active);
    color: var(--sc-text-primary);
  }

  &--cancel {
    opacity: 0.7;
    &:hover {
      opacity: 1;
    }
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

  .multi-select-bar.is-floating {
    width: calc(100vw - 16px);
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
    border: 1px solid var(--sc-border-mute);
    border-radius: 999px;
    background: var(--sc-bg-elevated);
    transform: translateY(-42%);
    align-self: auto;
    box-shadow: var(--chat-message-shadow);
    z-index: 1;
  }

  .multi-select-bar__drag-handle :deep(svg) {
    width: 12px;
    height: 12px;
  }

  .multi-select-bar__button span {
    display: none;
  }

  .multi-select-bar__actions {
    display: grid;
    grid-template-columns: repeat(11, minmax(28px, 1fr));
    width: 100%;
    min-width: 0;
    gap: 4px;
  }

  .multi-select-bar__button {
    width: 100%;
    min-width: 0;
    min-height: 32px;
    justify-content: center;
    padding: 6px 4px;
  }

  .multi-select-bar__button:only-child {
    grid-column: 1 / -1;
  }

  .multi-select-bar__divider {
    display: none;
  }

  .slide-up-enter-from,
  .slide-up-leave-to {
    transform: translateY(20px);
  }
}

@media (max-width: 480px) {
  .multi-select-bar__actions {
    grid-template-columns: repeat(6, minmax(0, 1fr));
  }
}
</style>
