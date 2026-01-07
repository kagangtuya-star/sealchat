<template>
  <div v-if="panels.length" class="iform-panel-stack">
    <section
      v-for="panel in panels"
      :key="panel.windowId"
      class="iform-panel"
      :class="{ 'is-collapsed': panel.collapsed }"
      :data-pill="pillLabel(panel)"
      :data-sync="panel.fromPush ? '1' : null"
      @click="handleSectionClick(panel, $event)"
    >
      <div class="iform-panel__card">
        <header class="iform-panel__header">
          <div class="iform-panel__title">
            <strong>{{ resolveForm(panel.formId)?.name || '未命名嵌入' }}</strong>
            <n-tag v-if="panel.fromPush" size="small" type="info">同步</n-tag>
          </div>
          <div class="iform-panel__actions">
            <n-button quaternary size="tiny" @click.stop="toggleCollapse(panel.windowId)">
              <template #icon>
                <n-icon :component="panel.collapsed ? ChevronDown : ChevronUp" />
              </template>
            </n-button>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button quaternary size="tiny" @click="openFloating(panel.windowId, panel.formId)">
                  <template #icon>
                    <n-icon :component="OpenOutline" />
                  </template>
                </n-button>
              </template>
              <span>弹出窗口</span>
            </n-tooltip>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button quaternary size="tiny" :disabled="!iform.canBroadcast" @click="pushSingle(panel.formId)">
                  <template #icon>
                    <n-icon :component="ShareOutline" />
                  </template>
                </n-button>
              </template>
              <span>推送到频道</span>
            </n-tooltip>
            <n-button quaternary size="tiny" @click="closePanel(panel.windowId)">
              <template #icon>
                <n-icon :component="CloseOutline" />
              </template>
            </n-button>
          </div>
        </header>
        <div
          class="iform-panel__body"
          :class="{ 'is-hidden': panel.collapsed }"
          :style="panelBodyStyle(panel)"
        >
          <div v-if="panel.autoPlayHint || panel.autoUnmuteHint" class="iform-panel__banner">
            <n-icon size="14" :component="VolumeHighOutline" />
            <span>如果页面包含媒体，请点击解除静音/播放。</span>
          </div>
          <IFormEmbedPortal :window-id="panel.windowId" :form-id="panel.formId" surface="panel" />
          <div class="iform-panel__resize" @mousedown.prevent="startResizing(panel, $event)">
            <n-icon size="16" :component="ResizeOutline" />
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, nextTick } from 'vue';
import { useEventListener } from '@vueuse/core';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedPortal from './IFormEmbedPortal.vue';
import { ChevronDown, ChevronUp, CloseOutline, OpenOutline, ResizeOutline, ShareOutline, VolumeHighOutline } from '@vicons/ionicons5';
import type { ChannelIForm } from '@/types/iform';
import { useMessage } from 'naive-ui';

const iform = useIFormStore();
iform.bootstrap();

const panels = computed(() => iform.currentPanels);
const formMap = computed<Map<string, ChannelIForm>>(() => {
  const map = new Map<string, ChannelIForm>();
  for (const item of iform.currentForms) {
    if (item) {
      map.set(item.id, item);
    }
  }
  return map;
});

const resolveForm = (formId: string) => formMap.value.get(formId);
const message = useMessage();

const resizing = ref<{ windowId: string; startHeight: number; startY: number } | null>(null);

const panelBodyStyle = (panel: { height: number }) => ({
  height: `${Math.max(panel.height, 1)}px`,
});

const pillLabel = (panel: { formId: string; fromPush?: boolean }) => {
  const form = resolveForm(panel.formId);
  if (!form) {
    return panel.fromPush ? '同步嵌入' : '嵌入窗';
  }
  return panel.fromPush ? `${form.name || '嵌入窗'} · 同步` : (form.name || '嵌入窗');
};

const toggleCollapse = (windowId: string) => {
  iform.togglePanelCollapse(windowId);
};

const resetResizing = () => {
  if (resizing.value) {
    resizing.value = null;
  }
};

const closePanel = (windowId: string) => {
  resetResizing();
  iform.closePanel(windowId);
};

const openFloating = async (windowId: string, formId: string) => {
  const form = resolveForm(formId);
  resetResizing();
  iform.openFloating(formId, {
    windowId,
    width: form?.defaultWidth,
    height: form?.defaultHeight,
    collapsed: false,
    fromPush: false,
  });
  await nextTick();
  iform.closePanel(windowId);
};

const handleSectionClick = (panel: { windowId: string; collapsed: boolean }, event: MouseEvent) => {
  if (!panel.collapsed) {
    return;
  }
  event.stopPropagation();
  toggleCollapse(panel.windowId);
};

const pushSingle = async (formId: string) => {
  if (!iform.canBroadcast) {
    return;
  }
  const form = resolveForm(formId);
  if (!form) {
    return;
  }
  try {
    await iform.pushStates([
      {
        formId,
        width: form.defaultWidth,
        height: form.defaultHeight,
        collapsed: false,
        floating: false,
      },
    ], { force: true });
    message.success('已推送到频道');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '推送失败');
  }
};

const startResizing = (panel: { windowId: string; height: number }, event: MouseEvent) => {
  resizing.value = { windowId: panel.windowId, startHeight: panel.height, startY: event.clientY };
};

useEventListener(window, 'mousemove', (event: MouseEvent) => {
  if (!resizing.value) {
    return;
  }
  event.preventDefault();
  const delta = event.clientY - resizing.value.startY;
  const nextHeight = resizing.value.startHeight + delta;
  iform.resizePanel(resizing.value.windowId, nextHeight);
});

useEventListener(window, 'mouseup', () => {
  resetResizing();
});

useEventListener(window, 'blur', () => {
  resetResizing();
});
</script>

<style scoped>
.iform-panel-stack {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  margin: 1rem 0;
}

.iform-panel {
  position: relative;
  padding-bottom: 0.5rem;
}

.iform-panel__card {
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.08));
  border-radius: 16px;
  background: var(--sc-bg-elevated, rgba(255, 255, 255, 0.95));
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  transition: opacity 0.2s ease, border-color 0.2s ease, max-height 0.2s ease;
}

.iform-panel.is-collapsed .iform-panel__card {
  opacity: 0;
  max-height: 0;
  border-color: transparent;
  pointer-events: none;
}

.iform-panel.is-collapsed {
  min-height: 2.5rem;
  cursor: pointer;
}

.iform-panel.is-collapsed::after {
  content: attr(data-pill);
  position: absolute;
  right: 0.2rem;
  top: 0.2rem;
  font-size: 0.75rem;
  line-height: 1;
  padding: 0.2rem 0.65rem;
  border-radius: 9999px;
  background: rgba(14, 165, 233, 0.95);
  color: rgba(255, 255, 255, 0.95);
  box-shadow: 0 6px 18px rgba(14, 165, 233, 0.35);
  pointer-events: none;
}

.iform-panel.is-collapsed::before {
  content: '';
  position: absolute;
  right: 0.55rem;
  top: 0.2rem;
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 9999px;
  background: #06b6d4;
  box-shadow: 0 0 10px rgba(6, 182, 212, 0.7);
  pointer-events: none;
}

.iform-panel.is-collapsed[data-sync='1']::after {
  background: rgba(248, 113, 113, 0.95);
  box-shadow: 0 6px 18px rgba(248, 113, 113, 0.35);
}

.iform-panel.is-collapsed[data-sync='1']::before {
  background: #f97316;
  box-shadow: 0 0 10px rgba(249, 115, 22, 0.65);
}

.iform-panel__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.07));
}

.iform-panel__title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.95rem;
}

.iform-panel__actions {
  display: flex;
  gap: 0.35rem;
}

.iform-panel__body {
  position: relative;
  padding: 1rem;
  overflow: hidden;
  transition: opacity 0.2s ease, height 0.2s ease, padding 0.2s ease;
}

.iform-panel__body.is-hidden {
  opacity: 0;
  visibility: hidden;
  height: 1px !important;
  padding-top: 0;
  padding-bottom: 0;
}

.iform-panel__resize {
  position: absolute;
  right: 0.5rem;
  bottom: 0.35rem;
  cursor: ns-resize;
  opacity: 0.6;
  transition: opacity 0.2s ease;
}

.iform-panel__resize:hover {
  opacity: 1;
}

.iform-panel__banner {
  position: absolute;
  top: 0.75rem;
  left: 1rem;
  z-index: 2;
  display: inline-flex;
  gap: 0.35rem;
  align-items: center;
  font-size: 0.78rem;
  padding: 0.2rem 0.6rem;
  border-radius: 9999px;
  background: rgba(14, 165, 233, 0.16);
  color: #0369a1;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
