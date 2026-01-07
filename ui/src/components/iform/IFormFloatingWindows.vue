<template>
  <teleport to="body">
    <div
      v-for="window in floatingWindows"
      :key="window.windowId"
      class="iform-floating"
      :class="{ 'is-minimized': window.minimized }"
      :style="floatingStyle(window)"
    >
      <header
        v-if="!window.minimized"
        class="iform-floating__header"
        @pointerdown.prevent="startDragging(window, $event)"
      >
        <div class="iform-floating__title" @dblclick="toggleMinimize(window.windowId)">
          <strong>{{ resolveForm(window.formId)?.name || '嵌入窗口' }}</strong>
          <n-tag v-if="window.fromPush" size="small" type="success">同步</n-tag>
        </div>
        <div class="iform-floating__actions" @pointerdown.stop>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button quaternary size="tiny" @click.stop="dockToPanel(window.windowId, window.formId)">
                <template #icon>
                  <n-icon :component="ReturnUpBackOutline" />
                </template>
              </n-button>
            </template>
            <span>固定到面板</span>
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button quaternary size="tiny" @click.stop="fitToViewport(window.windowId)">
                <template #icon>
                  <n-icon :component="ExpandOutline" />
                </template>
              </n-button>
            </template>
            <span>适配屏幕</span>
          </n-tooltip>
          <n-button quaternary size="tiny" @click.stop="toggleMinimize(window.windowId)">
            <template #icon>
              <n-icon :component="ContractOutline" />
            </template>
          </n-button>
          <n-button quaternary size="tiny" @click.stop="closeFloating(window.windowId)">
            <template #icon>
              <n-icon :component="CloseOutline" />
            </template>
          </n-button>
        </div>
      </header>
      <div class="iform-floating__body" :class="{ 'is-hidden': window.minimized }">
        <div v-if="window.autoPlayHint || window.autoUnmuteHint" class="iform-floating__banner">
          <n-icon size="14" :component="VolumeHighOutline" />
          <span>需要手动激活音/视频。</span>
        </div>
        <IFormEmbedPortal :window-id="window.windowId" :form-id="window.formId" surface="floating" />
        <div class="iform-floating__resize" @pointerdown.stop.prevent="startResizing(window, 'se', $event)">
          <n-icon size="16" :component="ResizeOutline" />
        </div>
      </div>
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-top"
        @pointerdown.stop.prevent="startResizing(window, 'n', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-right"
        @pointerdown.stop.prevent="startResizing(window, 'e', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-bottom"
        @pointerdown.stop.prevent="startResizing(window, 's', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-left"
        @pointerdown.stop.prevent="startResizing(window, 'w', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-top-left"
        @pointerdown.stop.prevent="startResizing(window, 'nw', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-top-right"
        @pointerdown.stop.prevent="startResizing(window, 'ne', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-bottom-left"
        @pointerdown.stop.prevent="startResizing(window, 'sw', $event)"
      />
      <div
        v-if="!window.minimized"
        class="iform-floating__resize-handle is-bottom-right"
        @pointerdown.stop.prevent="startResizing(window, 'se', $event)"
      />
      <button
        v-if="window.minimized"
        type="button"
        class="iform-floating__badge"
        @click.stop="toggleMinimize(window.windowId)"
        @pointerdown.prevent="startDragging(window, $event)"
      >
        <span>{{ formInitial(window.formId) }}</span>
      </button>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { computed, ref, nextTick, watch, onBeforeUnmount } from 'vue';
import { useEventListener } from '@vueuse/core';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedPortal from './IFormEmbedPortal.vue';
import { CloseOutline, ContractOutline, ExpandOutline, ResizeOutline, ReturnUpBackOutline, VolumeHighOutline } from '@vicons/ionicons5';
import type { ChannelIForm } from '@/types/iform';

const iform = useIFormStore();
iform.bootstrap();

const floatingWindows = computed(() => iform.currentFloatingWindows);
const formMap = computed<Map<string, ChannelIForm>>(() => {
  const map = new Map<string, ChannelIForm>();
  iform.currentForms.forEach((form) => {
    if (form) {
      map.set(form.id, form);
    }
  });
  return map;
});

const resolveForm = (formId: string) => formMap.value.get(formId);

const formInitial = (formId: string) => {
  const name = resolveForm(formId)?.name?.trim();
  if (!name) {
    return 'I';
  }
  return name.charAt(0).toUpperCase();
};

const floatingStyle = (windowState: (typeof floatingWindows.value)[number]) => ({
  left: `${windowState.x}px`,
  top: `${windowState.y}px`,
  width: windowState.minimized ? 'auto' : `${windowState.width}px`,
  height: windowState.minimized ? 'auto' : `${windowState.height}px`,
  zIndex: windowState.zIndex,
});

type ResizeDirection = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw';

const dragging = ref<{ windowId: string; offsetX: number; offsetY: number; pointerId: number; captureTarget?: HTMLElement | null } | null>(
  null,
);
const resizing = ref<{
  windowId: string;
  startWidth: number;
  startHeight: number;
  startX: number;
  startY: number;
  startLeft: number;
  startTop: number;
  pointerId: number;
  direction: ResizeDirection;
  captureTarget?: HTMLElement | null;
} | null>(null);

const startDragging = (windowState: (typeof floatingWindows.value)[number], event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return;
  }
  iform.bringFloatingToFront(windowState.windowId);
  const target = event.currentTarget as HTMLElement | null;
  target?.setPointerCapture?.(event.pointerId);
  dragging.value = {
    windowId: windowState.windowId,
    offsetX: event.clientX - windowState.x,
    offsetY: event.clientY - windowState.y,
    pointerId: event.pointerId,
    captureTarget: target,
  };
};

const startResizing = (
  windowState: (typeof floatingWindows.value)[number],
  direction: ResizeDirection,
  event: PointerEvent,
) => {
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return;
  }
  iform.bringFloatingToFront(windowState.windowId);
  const target = event.currentTarget as HTMLElement | null;
  target?.setPointerCapture?.(event.pointerId);
  resizing.value = {
    windowId: windowState.windowId,
    startWidth: windowState.width,
    startHeight: windowState.height,
    startX: event.clientX,
    startY: event.clientY,
    startLeft: windowState.x,
    startTop: windowState.y,
    pointerId: event.pointerId,
    direction,
    captureTarget: target,
  };
};

useEventListener(window, 'pointermove', (event: PointerEvent) => {
  if (dragging.value) {
    if (event.pointerId !== dragging.value.pointerId) {
      return;
    }
    if (event.buttons === 0) {
      resetPointerStateFor(dragging.value.windowId);
      return;
    }
    event.preventDefault();
    const x = event.clientX - dragging.value.offsetX;
    const y = event.clientY - dragging.value.offsetY;
    iform.updateFloatingPosition(dragging.value.windowId, x, y);
  } else if (resizing.value) {
    if (event.pointerId !== resizing.value.pointerId) {
      return;
    }
    if (event.buttons === 0) {
      resetPointerStateFor(resizing.value.windowId);
      return;
    }
    event.preventDefault();
    const deltaX = event.clientX - resizing.value.startX;
    const deltaY = event.clientY - resizing.value.startY;
    let width = resizing.value.startWidth;
    let height = resizing.value.startHeight;
    let x = resizing.value.startLeft;
    let y = resizing.value.startTop;
    const direction = resizing.value.direction;
    if (direction.includes('e')) {
      width = resizing.value.startWidth + deltaX;
    }
    if (direction.includes('s')) {
      height = resizing.value.startHeight + deltaY;
    }
    if (direction.includes('w')) {
      width = resizing.value.startWidth - deltaX;
      x = resizing.value.startLeft + deltaX;
    }
    if (direction.includes('n')) {
      height = resizing.value.startHeight - deltaY;
      y = resizing.value.startTop + deltaY;
    }
    iform.updateFloatingRect(resizing.value.windowId, { x, y, width, height });
  }
});

const clearPointerState = (event: PointerEvent) => {
  if (dragging.value?.pointerId === event.pointerId) {
    dragging.value?.captureTarget?.releasePointerCapture?.(event.pointerId);
    dragging.value = null;
  }
  if (resizing.value?.pointerId === event.pointerId) {
    resizing.value?.captureTarget?.releasePointerCapture?.(event.pointerId);
    resizing.value = null;
  }
};

const resetPointerStateFor = (windowId?: string) => {
  if (dragging.value && (!windowId || dragging.value.windowId === windowId)) {
    dragging.value.captureTarget?.releasePointerCapture?.(dragging.value.pointerId);
    dragging.value = null;
  }
  if (resizing.value && (!windowId || resizing.value.windowId === windowId)) {
    resizing.value.captureTarget?.releasePointerCapture?.(resizing.value.pointerId);
    resizing.value = null;
  }
};

useEventListener(window, 'pointerup', clearPointerState);
useEventListener(window, 'pointercancel', clearPointerState);
useEventListener(window, 'blur', () => resetPointerStateFor());
if (typeof document !== 'undefined') {
  useEventListener(document, 'lostpointercapture', (event: PointerEvent) => {
    if (dragging.value?.pointerId === event.pointerId) {
      resetPointerStateFor(dragging.value.windowId);
    }
    if (resizing.value?.pointerId === event.pointerId) {
      resetPointerStateFor(resizing.value.windowId);
    }
  }, { capture: true });
}

watch(floatingWindows, (windows) => {
  const activeIds = new Set(windows.map((item) => item.windowId));
  if (dragging.value && !activeIds.has(dragging.value.windowId)) {
    resetPointerStateFor(dragging.value.windowId);
  }
  if (resizing.value && !activeIds.has(resizing.value.windowId)) {
    resetPointerStateFor(resizing.value.windowId);
  }
});

onBeforeUnmount(() => {
  resetPointerStateFor();
});

const toggleMinimize = (windowId: string) => {
  iform.toggleFloatingMinimize(windowId);
};

const closeFloating = (windowId: string) => {
  resetPointerStateFor(windowId);
  iform.closeFloating(windowId);
};

const fitToViewport = (windowId: string) => {
  iform.fitFloatingToViewport(windowId);
};

const dockToPanel = async (windowId: string, formId: string) => {
  const form = resolveForm(formId);
  resetPointerStateFor(windowId);
  iform.openPanel(formId, {
    windowId,
    height: form?.defaultHeight,
    collapsed: form?.defaultCollapsed,
  });
  await nextTick();
  iform.closeFloating(windowId);
};
</script>

<style scoped>
.iform-floating {
  position: fixed;
  border-radius: 14px;
  border: none;
  box-shadow: none;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  background: transparent;
  backdrop-filter: none;
}

.iform-floating__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem;
  padding: 0.25rem 0.45rem;
  cursor: move;
  touch-action: none;
  background: rgba(15, 23, 42, 0.55);
  color: #e2e8f0;
  border-radius: 12px 12px 0 0;
  min-width: 160px;
}

.iform-floating__title {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.85rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.iform-floating__title strong {
  font-weight: 600;
  max-width: 10rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.iform-floating__actions {
  display: flex;
  gap: 0.2rem;
}

.iform-floating__body {
  position: relative;
  flex: 1;
  padding: 0;
  background: transparent;
  transition: opacity 0.2s ease, height 0.2s ease, padding 0.2s ease;
}

.iform-floating__body.is-hidden {
  position: absolute;
  left: -9999px;
  top: -9999px;
  width: 1px;
  height: 1px;
  padding: 0;
  opacity: 0;
  pointer-events: none;
  overflow: hidden;
}

.iform-floating__banner {
  position: absolute;
  top: 0.6rem;
  left: 0.75rem;
  display: inline-flex;
  gap: 0.35rem;
  align-items: center;
  font-size: 0.78rem;
  padding: 0.2rem 0.6rem;
  border-radius: 9999px;
  background: rgba(248, 189, 71, 0.25);
  color: #fef3c7;
  z-index: 2;
}

.iform-floating__resize {
  position: absolute;
  right: 0.3rem;
  bottom: 0.3rem;
  cursor: nwse-resize;
  color: rgba(255, 255, 255, 0.8);
  touch-action: none;
}

.iform-floating__resize-handle {
  position: absolute;
  z-index: 3;
  touch-action: none;
}

.iform-floating__resize-handle.is-top {
  top: 0;
  left: 12px;
  right: 12px;
  height: 12px;
  cursor: ns-resize;
}

.iform-floating__resize-handle.is-right {
  top: 28px;
  right: 0;
  width: 12px;
  height: calc(100% - 40px);
  cursor: ew-resize;
}

.iform-floating__resize-handle.is-bottom {
  left: 12px;
  right: 12px;
  bottom: 0;
  height: 12px;
  cursor: ns-resize;
}

.iform-floating__resize-handle.is-left {
  top: 28px;
  left: 0;
  width: 12px;
  height: calc(100% - 40px);
  cursor: ew-resize;
}

.iform-floating__resize-handle.is-top-left {
  top: 0;
  left: 0;
  width: 18px;
  height: 18px;
  cursor: nwse-resize;
}

.iform-floating__resize-handle.is-top-right {
  top: 0;
  right: 0;
  width: 18px;
  height: 18px;
  cursor: nesw-resize;
}

.iform-floating__resize-handle.is-bottom-left {
  bottom: 0;
  left: 0;
  width: 18px;
  height: 18px;
  cursor: nesw-resize;
}

.iform-floating__resize-handle.is-bottom-right {
  bottom: 0;
  right: 0;
  width: 18px;
  height: 18px;
  cursor: nwse-resize;
}

.iform-floating.is-minimized {
  padding: 0;
  border: none;
  box-shadow: none;
  background: transparent;
  width: auto !important;
  height: auto !important;
  overflow: visible;
}

.iform-floating :deep(.iform-frame) {
  border: none;
  border-radius: 14px;
  background: transparent;
  box-shadow: none;
}

.iform-floating :deep(.iform-frame__iframe),
.iform-floating :deep(.iform-frame__html) {
  border-radius: 14px;
}

.iform-floating__badge {
  width: 48px;
  height: 48px;
  border-radius: 9999px;
  border: none;
  background: rgba(14, 165, 233, 0.92);
  color: #f8fafc;
  font-weight: 600;
  font-size: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 10px 25px rgba(14, 165, 233, 0.45);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.iform-floating__badge:hover {
  transform: translateY(-1px);
  box-shadow: 0 14px 28px rgba(14, 165, 233, 0.55);
}
</style>
