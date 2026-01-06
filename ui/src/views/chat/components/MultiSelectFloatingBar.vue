<script setup lang="ts">
import { computed } from 'vue';
import { NIcon, NCheckbox, NButton, NTooltip } from 'naive-ui';
import { Copy, Archive, Trash, Photo, BoxMultiple, X, ArrowsVertical } from '@vicons/tabler';
import { useChatStore } from '@/stores/chat';
import { useDisplayStore } from '@/stores/display';

const chat = useChatStore();
const display = useDisplayStore();

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
const rangeHint = computed(() => {
  if (!rangeModeEnabled.value) return '';
  if (!chat.multiSelect?.rangeAnchorId) return '点击消息选择起点';
  return '点击另一条消息完成范围选择';
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
  <Transition name="slide-up">
    <div v-if="isActive" class="multi-select-bar">
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
}

:root[data-display-palette='night'] .multi-select-bar {
  background: rgba(20, 24, 36, 0.95);
  border-color: rgba(255, 255, 255, 0.1);
  color: rgba(248, 250, 252, 0.95);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
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

  .multi-select-bar__button span {
    display: none;
  }

  .slide-up-enter-from,
  .slide-up-leave-to {
    transform: translateY(20px);
  }
}
</style>
