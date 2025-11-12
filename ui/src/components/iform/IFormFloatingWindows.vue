<template>
  <teleport to="body">
    <div
      v-for="window in floatingWindows"
      :key="window.formId"
      class="iform-floating"
      :class="{ 'is-minimized': window.minimized }"
      :style="floatingStyle(window)"
    >
      <header
        v-if="!window.minimized"
        class="iform-floating__header"
        @mousedown.prevent="startDragging(window, $event)"
      >
        <div class="iform-floating__title" @dblclick="toggleMinimize(window.formId)">
          <strong>{{ resolveForm(window.formId)?.name || '嵌入窗口' }}</strong>
          <n-tag v-if="window.fromPush" size="small" type="success">同步</n-tag>
        </div>
        <div class="iform-floating__actions">
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button quaternary size="tiny" @click.stop="dockToPanel(window.formId)">
                <template #icon>
                  <n-icon :component="ReturnUpBackOutline" />
                </template>
              </n-button>
            </template>
            <span>固定到面板</span>
          </n-tooltip>
          <n-button quaternary size="tiny" @click.stop="toggleMinimize(window.formId)">
            <template #icon>
              <n-icon :component="ContractOutline" />
            </template>
          </n-button>
          <n-button quaternary size="tiny" @click.stop="closeFloating(window.formId)">
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
        <IFormEmbedPortal :form-id="window.formId" surface="floating" />
        <div class="iform-floating__resize" @mousedown.stop.prevent="startResizing(window, $event)">
          <n-icon size="16" :component="ResizeOutline" />
        </div>
      </div>
      <button
        v-if="window.minimized"
        type="button"
        class="iform-floating__badge"
        @click.stop="toggleMinimize(window.formId)"
        @mousedown.prevent="startDragging(window, $event)"
      >
        <span>{{ formInitial(window.formId) }}</span>
      </button>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { computed, ref, nextTick } from 'vue';
import { useEventListener } from '@vueuse/core';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedPortal from './IFormEmbedPortal.vue';
import { CloseOutline, ContractOutline, ResizeOutline, ReturnUpBackOutline, VolumeHighOutline } from '@vicons/ionicons5';
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

const dragging = ref<{ formId: string; offsetX: number; offsetY: number } | null>(null);
const resizing = ref<{ formId: string; startWidth: number; startHeight: number; startX: number; startY: number } | null>(null);

const startDragging = (windowState: (typeof floatingWindows.value)[number], event: MouseEvent) => {
  iform.bringFloatingToFront(windowState.formId);
  dragging.value = {
    formId: windowState.formId,
    offsetX: event.clientX - windowState.x,
    offsetY: event.clientY - windowState.y,
  };
};

const startResizing = (windowState: (typeof floatingWindows.value)[number], event: MouseEvent) => {
  resizing.value = {
    formId: windowState.formId,
    startWidth: windowState.width,
    startHeight: windowState.height,
    startX: event.clientX,
    startY: event.clientY,
  };
};

useEventListener(window, 'mousemove', (event: MouseEvent) => {
  if (dragging.value) {
    event.preventDefault();
    const x = event.clientX - dragging.value.offsetX;
    const y = event.clientY - dragging.value.offsetY;
    iform.updateFloatingPosition(dragging.value.formId, x, y);
  } else if (resizing.value) {
    event.preventDefault();
    const deltaX = event.clientX - resizing.value.startX;
    const deltaY = event.clientY - resizing.value.startY;
    const width = resizing.value.startWidth + deltaX;
    const height = resizing.value.startHeight + deltaY;
    iform.updateFloatingSize(resizing.value.formId, width, height);
  }
});

useEventListener(window, 'mouseup', () => {
  dragging.value = null;
  resizing.value = null;
});

const toggleMinimize = (formId: string) => {
  iform.toggleFloatingMinimize(formId);
};

const closeFloating = (formId: string) => {
  iform.closeFloating(formId);
};

const dockToPanel = async (formId: string) => {
  const form = resolveForm(formId);
  iform.openPanel(formId, {
    height: form?.defaultHeight,
    collapsed: form?.defaultCollapsed,
  });
  await nextTick();
  iform.closeFloating(formId);
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
