<template>
  <div
    v-if="windowData && !windowData.isMinimized"
    ref="windowEl"
    class="character-sheet-window"
    :class="{ 'is-flipped': windowData.mode === 'edit', 'is-mobile': isMobile }"
    :style="windowStyle"
    @pointerdown="handlePointerDown"
  >
    <div
      ref="headerEl"
      class="sheet-window__header"
      @pointerdown="startDrag"
    >
      <div class="sheet-window__title">
        <button
          v-if="isMobile"
          class="sheet-window__mobile-back"
          title="关闭"
          @click="handleMobileBack"
          @pointerdown.stop
        >
          <n-icon :component="ChevronBack" :size="15" />
          <span>关闭</span>
        </button>
        <n-icon v-else :component="User" :size="16" />
        <span class="sheet-window__title-text">{{ windowData.cardName || '人物卡' }}</span>
      </div>
      <div class="sheet-window__controls">
        <button
          class="sheet-window__control-btn"
          :title="windowData.mode === 'view' ? '编辑' : '预览'"
          @click="sheetStore.toggleMode(windowId)"
          @pointerdown.stop
        >
          <n-icon :component="windowData.mode === 'view' ? Edit : Eye" :size="14" />
        </button>
        <button
          class="sheet-window__control-btn"
          :class="{ 'sheet-window__control-btn--mobile-text': isMobile }"
          title="最小化"
          @click="sheetStore.minimizeSheet(windowId)"
          @pointerdown.stop
        >
          <n-icon :component="Minus" :size="14" />
          <span v-if="isMobile" class="sheet-window__control-text">最小化</span>
        </button>
        <button
          v-if="!isMobile"
          class="sheet-window__control-btn sheet-window__control-btn--close"
          title="关闭"
          @click="sheetStore.closeSheet(windowId)"
          @pointerdown.stop
        >
          <n-icon :component="Close" :size="14" />
        </button>
      </div>
    </div>

    <div class="sheet-window__content">
      <div class="sheet-window__flipper">
        <div class="sheet-window__front">
          <IframeSandbox
            :html="windowData.template"
            :data="iframeData"
            :window-id="windowId"
            @iframe-event="handleIframeEvent"
          />
        </div>
        <div class="sheet-window__back">
          <n-tabs type="line" size="small" class="sheet-window__tabs">
            <n-tab-pane name="data" tab="数据">
              <div class="sheet-window__editor">
                <n-input
                  v-model:value="jsonText"
                  type="textarea"
                  placeholder="JSON 数据"
                  :autosize="{ minRows: 8 }"
                  class="sheet-window__json-input"
                  @blur="handleJsonSave"
                />
                <div v-if="jsonError" class="sheet-window__json-error">
                  {{ jsonError }}
                </div>
              </div>
            </n-tab-pane>
            <n-tab-pane name="template" tab="模板">
              <div class="sheet-window__editor">
                <div class="sheet-window__template-source">
                  <n-select
                    v-model:value="selectedTemplateMode"
                    :options="templateModeOptions"
                    size="small"
                  />
                  <n-select
                    v-if="selectedTemplateMode === 'managed'"
                    v-model:value="selectedTemplateId"
                    :options="managedTemplateOptions"
                    placeholder="选择模板库模板"
                    size="small"
                    filterable
                    clearable
                    @update:value="handleManagedTemplateChange"
                  />
                </div>
                <n-input
                  v-model:value="templateText"
                  type="textarea"
                  placeholder="HTML 模板"
                  :autosize="{ minRows: 8 }"
                  class="sheet-window__template-input"
                  @blur="handleTemplateSave"
                />
                <div class="sheet-window__template-actions">
                  <n-button size="tiny" @click="resetTemplate">
                    重置为默认模板
                  </n-button>
                  <n-button size="tiny" @click="resetTemplateToCoc">
                    重置为COC默认模板
                  </n-button>
                  <n-input
                    v-model:value="templateSaveName"
                    size="tiny"
                    placeholder="新模板名称"
                    class="sheet-window__template-name"
                  />
                  <n-button size="tiny" @click="handleSaveAsTemplate">
                    另存为新模板
                  </n-button>
                  <n-button
                    size="tiny"
                    :disabled="selectedTemplateMode !== 'managed' || !selectedTemplateId"
                    @click="handleSyncToTemplate"
                  >
                    覆盖同步到模板库
                  </n-button>
                </div>
              </div>
            </n-tab-pane>
          </n-tabs>
        </div>
      </div>
    </div>

    <div
      v-if="!isMobile"
      class="sheet-window__resize-handle"
      @pointerdown="startResize"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue';
import { NIcon, NTabs, NTabPane, NInput, NButton, NSelect, useMessage } from 'naive-ui';
import { Close, Remove as Minus, Create as Edit, Eye, ChevronBack } from '@vicons/ionicons5';
import { User } from '@vicons/tabler';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useCharacterCardTemplateStore, type CharacterCardTemplateMode } from '@/stores/characterCardTemplate';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import IframeSandbox, { type SealChatEvent } from './IframeSandbox.vue';

const props = defineProps<{
  windowId: string;
}>();

const sheetStore = useCharacterSheetStore();
const templateStore = useCharacterCardTemplateStore();
const message = useMessage();

const windowEl = ref<HTMLElement | null>(null);
const headerEl = ref<HTMLElement | null>(null);

const jsonText = ref('');
const jsonError = ref('');
const templateText = ref('');
const templateSaveName = ref('');
const selectedTemplateMode = ref<CharacterCardTemplateMode>('detached');
const selectedTemplateId = ref<string>('');

const templateModeOptions = [
  { label: '自定义（脱离模板库）', value: 'detached' },
  { label: '模板库模板', value: 'managed' },
];

const managedTemplateOptions = computed(() => {
  const sheetType = windowData.value?.sheetType || '';
  return templateStore.getTemplatesBySheetType(sheetType).map(item => ({
    label: `${item.name}${item.sheetType ? ` [${item.sheetType}]` : ''}`,
    value: item.id,
  }));
});

const isMobile = ref(false);
const isDragging = ref(false);
const isResizing = ref(false);
const dragStart = ref({ x: 0, y: 0, posX: 0, posY: 0 });
const resizeStart = ref({ x: 0, y: 0, w: 0, h: 0 });

const windowData = computed(() => sheetStore.windows[props.windowId]);

const windowStyle = computed(() => {
  const win = windowData.value;
  if (!win) return {};
  if (isMobile.value) {
    return { zIndex: win.zIndex };
  }
  return {
    transform: `translate(${win.positionX}px, ${win.positionY}px)`,
    width: `${win.width}px`,
    height: `${win.height}px`,
    zIndex: win.zIndex,
  };
});

const iframeData = computed(() => {
  const rawAvatar = windowData.value?.avatarUrl || '';
  return {
    name: windowData.value?.cardName || '',
    attrs: windowData.value?.attrs || {},
    avatarUrl: resolveAttachmentUrl(rawAvatar) || rawAvatar,
  };
});

const emit = defineEmits<{
  rollRequest: [payload: SealChatEvent['payload']['roll']];
}>();

const handleIframeEvent = (event: SealChatEvent) => {
  if (event.action === 'ROLL_DICE' && event.payload.roll) {
    emit('rollRequest', event.payload.roll);
  } else if (event.action === 'EDIT_START') {
    sheetStore.beginEditLock(props.windowId);
  } else if (event.action === 'EDIT_END') {
    sheetStore.endEditLock(props.windowId);
  } else if (event.action === 'UPDATE_ATTRS' && event.payload.attrs) {
    sheetStore.updateAttrs(props.windowId, {
      ...windowData.value?.attrs,
      ...event.payload.attrs,
    });
  }
};

const checkMobile = () => {
  isMobile.value = typeof window !== 'undefined' && window.innerWidth < 768;
};

const handlePointerDown = () => {
  sheetStore.bringToFront(props.windowId);
};

const handleMobileBack = () => {
  sheetStore.closeSheet(props.windowId);
};

const startDrag = (e: PointerEvent) => {
  if (isMobile.value) return;
  const win = windowData.value;
  if (!win) return;

  isDragging.value = true;
  dragStart.value = {
    x: e.clientX,
    y: e.clientY,
    posX: win.positionX,
    posY: win.positionY,
  };

  document.addEventListener('pointermove', onDrag);
  document.addEventListener('pointerup', stopDrag);
};

const onDrag = (e: PointerEvent) => {
  if (!isDragging.value) return;
  const dx = e.clientX - dragStart.value.x;
  const dy = e.clientY - dragStart.value.y;
  sheetStore.updatePosition(
    props.windowId,
    dragStart.value.posX + dx,
    dragStart.value.posY + dy
  );
};

const stopDrag = () => {
  isDragging.value = false;
  document.removeEventListener('pointermove', onDrag);
  document.removeEventListener('pointerup', stopDrag);
};

const startResize = (e: PointerEvent) => {
  e.stopPropagation();
  const win = windowData.value;
  if (!win) return;

  isResizing.value = true;
  resizeStart.value = {
    x: e.clientX,
    y: e.clientY,
    w: win.width,
    h: win.height,
  };

  document.addEventListener('pointermove', onResize);
  document.addEventListener('pointerup', stopResize);
};

const onResize = (e: PointerEvent) => {
  if (!isResizing.value) return;
  const dw = e.clientX - resizeStart.value.x;
  const dh = e.clientY - resizeStart.value.y;
  sheetStore.updateSize(
    props.windowId,
    resizeStart.value.w + dw,
    resizeStart.value.h + dh
  );
};

const stopResize = () => {
  isResizing.value = false;
  document.removeEventListener('pointermove', onResize);
  document.removeEventListener('pointerup', stopResize);
};

const syncJsonText = () => {
  const win = windowData.value;
  if (win) {
    jsonText.value = JSON.stringify(win.attrs, null, 2);
    jsonError.value = '';
  }
};

const syncTemplateText = () => {
  const win = windowData.value;
  if (win) {
    templateText.value = win.template;
    selectedTemplateMode.value = win.templateMode || 'detached';
    selectedTemplateId.value = win.templateId || '';
    if (!templateSaveName.value) {
      templateSaveName.value = `${win.cardName || '人物卡'}模板`;
    }
  }
};

const handleJsonSave = () => {
  try {
    const parsed = JSON.parse(jsonText.value);
    jsonError.value = '';
    sheetStore.updateAttrs(props.windowId, parsed);
  } catch (e: any) {
    jsonError.value = 'JSON 格式错误: ' + (e.message || '');
  }
};

const handleTemplateSave = () => {
  sheetStore.updateTemplate(props.windowId, templateText.value);
};

const handleManagedTemplateChange = async (templateId: string) => {
  if (!templateId) return;
  try {
    await sheetStore.applyManagedTemplate(props.windowId, templateId);
    const win = windowData.value;
    if (win) {
      templateText.value = win.template;
      selectedTemplateMode.value = 'managed';
      selectedTemplateId.value = templateId;
    }
    message.success('模板已切换');
  } catch (e: any) {
    message.error(e?.message || '模板切换失败');
  }
};

const handleSaveAsTemplate = async () => {
  const name = templateSaveName.value.trim();
  if (!name) {
    message.warning('请输入新模板名称');
    return;
  }
  try {
    const created = await sheetStore.saveCurrentTemplateAsNew(props.windowId, name);
    if (created?.id) {
      selectedTemplateMode.value = 'managed';
      selectedTemplateId.value = created.id;
    }
    message.success('已保存为新模板');
  } catch (e: any) {
    message.error(e?.message || '保存失败');
  }
};

const handleSyncToTemplate = async () => {
  if (!selectedTemplateId.value) {
    message.warning('请先选择模板');
    return;
  }
  try {
    await sheetStore.syncCurrentTemplateToTemplate(props.windowId, selectedTemplateId.value);
    message.success('已同步到模板库');
  } catch (e: any) {
    message.error(e?.message || '同步失败');
  }
};

const resetTemplate = () => {
  const defaultTpl = sheetStore.getDefaultTemplate(windowData.value?.sheetType);
  templateText.value = defaultTpl;
  sheetStore.updateTemplate(props.windowId, defaultTpl);
  selectedTemplateMode.value = 'detached';
  selectedTemplateId.value = '';
};

const resetTemplateToCoc = () => {
  const cocTpl = sheetStore.getDefaultTemplate('coc7');
  templateText.value = cocTpl;
  sheetStore.updateTemplate(props.windowId, cocTpl);
  selectedTemplateMode.value = 'detached';
  selectedTemplateId.value = '';
};

watch(selectedTemplateMode, async (mode) => {
  const win = windowData.value;
  if (!win) return;
  if (win.templateMode === mode && (mode !== 'managed' || (win.templateId || '') === selectedTemplateId.value)) {
    return;
  }
  if (mode === 'detached') {
    selectedTemplateId.value = '';
    try {
      await sheetStore.applyDetachedTemplate(props.windowId, templateText.value);
    } catch (e) {
      console.warn('Failed to switch template mode to detached', e);
    }
    return;
  }
  if (mode === 'managed' && selectedTemplateId.value) {
    await handleManagedTemplateChange(selectedTemplateId.value);
  }
});

watch(
  () => windowData.value?.attrs,
  () => {
    if (windowData.value?.mode === 'view') {
      syncJsonText();
    }
  },
  { deep: true }
);

watch(
  () => windowData.value?.mode,
  (mode) => {
    if (mode === 'edit') {
      syncJsonText();
      syncTemplateText();
    }
  }
);

onMounted(() => {
  checkMobile();
  window.addEventListener('resize', checkMobile);
  void templateStore.ensureTemplatesLoaded();
  syncJsonText();
  syncTemplateText();
  const win = windowData.value;
  if (win) {
    const normalized = sheetStore.normalizeTemplate(win.cardId, win.template, win.sheetType);
    if (normalized !== win.template) {
      sheetStore.updateTemplate(props.windowId, normalized);
      templateText.value = normalized;
    }
  }
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', checkMobile);
  stopDrag();
  stopResize();
});
</script>

<style scoped>
.character-sheet-window {
  position: fixed;
  top: 0;
  left: 0;
  display: flex;
  flex-direction: column;
  background: var(--sc-bg-card, rgba(255, 255, 255, 0.98));
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12), 0 2px 8px rgba(0, 0, 0, 0.08);
  overflow: hidden;
  pointer-events: auto;
  backdrop-filter: blur(8px);
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.06));
}

.character-sheet-window.is-mobile {
  inset: 0 !important;
  transform: none !important;
  width: 100% !important;
  height: 100% !important;
  border-radius: 0;
}

.sheet-window__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px;
  min-height: 32px;
  background: var(--sc-bg-panel, #f9fafb);
  border-bottom: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.06));
  cursor: move;
  user-select: none;
  touch-action: none;
}

.is-mobile .sheet-window__header {
  cursor: default;
}

.sheet-window__title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--sc-text-primary, #1f2937);
  min-width: 0;
}

.sheet-window__mobile-back {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  border: none;
  border-radius: 6px;
  padding: 4px 6px;
  background: rgba(0, 0, 0, 0.06);
  color: var(--sc-text-secondary, #6b7280);
  font-size: 12px;
  cursor: pointer;
}

.sheet-window__title-text {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sheet-window__controls {
  display: flex;
  gap: 4px;
}

.sheet-window__control-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  width: 22px;
  height: 22px;
  border: none;
  background: rgba(0, 0, 0, 0.06);
  border-radius: 5px;
  cursor: pointer;
  color: var(--sc-text-secondary, #6b7280);
  transition: all 0.15s ease;
}

.sheet-window__control-text {
  font-size: 11px;
  line-height: 1;
}

.sheet-window__control-btn:hover {
  background: rgba(0, 0, 0, 0.1);
  color: var(--sc-text-primary, #1f2937);
}

.sheet-window__control-btn--close:hover {
  background: #ef4444;
  color: white;
}

.sheet-window__content {
  flex: 1;
  overflow: hidden;
  perspective: 1000px;
}

.sheet-window__flipper {
  position: relative;
  width: 100%;
  height: 100%;
  transition: transform 0.5s ease;
  transform-style: preserve-3d;
}

.is-flipped .sheet-window__flipper {
  transform: rotateY(180deg);
}

.sheet-window__front,
.sheet-window__back {
  position: absolute;
  inset: 0;
  backface-visibility: hidden;
  overflow: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--sc-border-strong, rgba(100, 116, 139, 0.4)) transparent;
}

.sheet-window__back {
  transform: rotateY(180deg);
  background: var(--sc-bg-base, #fff);
  color: var(--sc-text-primary, #1f2937);
}

.sheet-window__tabs {
  height: 100%;
}

.sheet-window__tabs :deep(.n-tabs-nav) {
  background: var(--sc-bg-panel, #f9fafb);
}

.sheet-window__tabs :deep(.n-tabs-tab__label) {
  color: var(--sc-text-secondary, #6b7280);
}

.sheet-window__tabs :deep(.n-tabs-tab--active .n-tabs-tab__label) {
  color: var(--sc-text-primary, #1f2937);
}

.sheet-window__tabs :deep(.n-tabs-bar) {
  background: var(--sc-accent, var(--primary-color, #3388de));
}

.sheet-window__tabs :deep(.n-tabs-pane-wrapper) {
  height: calc(100% - 40px);
}

.sheet-window__tabs :deep(.n-tab-pane) {
  height: 100%;
  padding: 12px;
  overflow: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--sc-border-strong, rgba(100, 116, 139, 0.4)) transparent;
}

.sheet-window__editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
}

.sheet-window__front::-webkit-scrollbar,
.sheet-window__back::-webkit-scrollbar,
.sheet-window__tabs :deep(.n-tab-pane::-webkit-scrollbar) {
  width: 6px;
  height: 6px;
}

.sheet-window__front::-webkit-scrollbar-thumb,
.sheet-window__back::-webkit-scrollbar-thumb,
.sheet-window__tabs :deep(.n-tab-pane::-webkit-scrollbar-thumb) {
  background: var(--sc-border-strong, rgba(100, 116, 139, 0.4));
  border-radius: 999px;
}

.sheet-window__front::-webkit-scrollbar-track,
.sheet-window__back::-webkit-scrollbar-track,
.sheet-window__tabs :deep(.n-tab-pane::-webkit-scrollbar-track) {
  background: transparent;
}

.sheet-window__json-input,
.sheet-window__template-input {
  flex: 1;
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
  font-size: 12px;
}

.sheet-window__back :deep(.n-input) {
  --n-color: var(--sc-bg-input, #f3f4f6);
  --n-color-focus: var(--sc-bg-input, #f3f4f6);
  --n-border: var(--sc-border-mute, rgba(0, 0, 0, 0.1));
  --n-border-hover: var(--sc-border-mute, rgba(0, 0, 0, 0.2));
  --n-border-focus: var(--sc-accent, var(--primary-color, #3388de));
  --n-text-color: var(--sc-text-primary, #1f2937);
  --n-placeholder-color: var(--sc-text-secondary, #6b7280);
  --n-caret-color: var(--sc-text-primary, #1f2937);
}

.sheet-window__json-input :deep(textarea),
.sheet-window__template-input :deep(textarea) {
  min-height: 200px !important;
}

.sheet-window__json-error {
  padding: 8px 12px;
  background: var(--sc-danger-bg, rgba(239, 68, 68, 0.1));
  border-radius: 6px;
  color: var(--sc-danger, #dc2626);
  font-size: 12px;
}

.sheet-window__template-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.sheet-window__template-source {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.5rem;
}

.sheet-window__template-name {
  width: 180px;
}

.is-mobile .sheet-window__header {
  padding: 8px 10px;
  min-height: 40px;
  gap: 0.5rem;
}

.is-mobile .sheet-window__title {
  flex: 1;
  gap: 4px;
}

.is-mobile .sheet-window__title-text {
  max-width: 48vw;
}

.is-mobile .sheet-window__controls {
  gap: 6px;
}

.is-mobile .sheet-window__control-btn {
  width: 30px;
  height: 30px;
  border-radius: 8px;
}

.is-mobile .sheet-window__control-btn--mobile-text {
  width: auto;
  padding: 0 8px;
}

.is-mobile .sheet-window__template-source {
  grid-template-columns: 1fr;
}

.is-mobile .sheet-window__template-actions {
  justify-content: flex-start;
}

.is-mobile .sheet-window__template-name {
  width: 100%;
}

.is-mobile .sheet-window__tabs :deep(.n-tab-pane) {
  padding: 10px;
}

.sheet-window__resize-handle {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 16px;
  height: 16px;
  cursor: nwse-resize;
  touch-action: none;
  background: linear-gradient(
    135deg,
    transparent 50%,
    rgba(0, 0, 0, 0.08) 50%,
    rgba(0, 0, 0, 0.15) 100%
  );
  border-radius: 0 0 12px 0;
}

:root[data-display-palette='night'] .character-sheet-window {
  background: var(--sc-bg-card, rgba(30, 41, 59, 0.98));
  border-color: rgba(148, 163, 184, 0.2);
}

:root[data-display-palette='night'] .sheet-window__header {
  background: var(--sc-bg-panel, rgba(30, 41, 59, 0.95));
  border-color: rgba(148, 163, 184, 0.15);
}

:root[data-display-palette='night'] .sheet-window__mobile-back {
  background: rgba(255, 255, 255, 0.08);
  color: var(--sc-text-secondary, #94a3b8);
}

:root[data-display-palette='night'] .sheet-window__control-btn {
  background: rgba(255, 255, 255, 0.08);
  color: var(--sc-text-secondary, #94a3b8);
}

:root[data-display-palette='night'] .sheet-window__control-btn:hover {
  background: rgba(255, 255, 255, 0.15);
  color: var(--sc-text-primary, #f1f5f9);
}

:root[data-display-palette='night'] .sheet-window__back {
  background: var(--sc-bg-base, #0f172a);
  color: var(--sc-text-primary, #f1f5f9);
}

:root[data-display-palette='night'] .sheet-window__tabs :deep(.n-tabs-nav) {
  background: var(--sc-bg-panel, rgba(30, 41, 59, 0.95));
}

:root[data-display-palette='night'] .sheet-window__tabs :deep(.n-tabs-tab__label) {
  color: var(--sc-text-secondary, #94a3b8);
}

:root[data-display-palette='night'] .sheet-window__tabs :deep(.n-tabs-tab--active .n-tabs-tab__label) {
  color: var(--sc-text-primary, #f1f5f9);
}

:root[data-display-palette='night'] .sheet-window__back :deep(.n-input) {
  --n-color: var(--sc-bg-input, rgba(15, 23, 42, 0.9));
  --n-color-focus: var(--sc-bg-input, rgba(15, 23, 42, 0.9));
  --n-border: var(--sc-border-mute, rgba(148, 163, 184, 0.2));
  --n-border-hover: var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  --n-border-focus: var(--sc-accent, var(--primary-color, #3388de));
  --n-text-color: var(--sc-text-primary, #f1f5f9);
  --n-placeholder-color: var(--sc-text-secondary, #94a3b8);
  --n-caret-color: var(--sc-text-primary, #f1f5f9);
}

:root[data-display-palette='night'] .sheet-window__front,
:root[data-display-palette='night'] .sheet-window__back,
:root[data-display-palette='night'] .sheet-window__tabs :deep(.n-tab-pane) {
  scrollbar-color: rgba(148, 163, 184, 0.5) transparent;
}

:root[data-display-palette='night'] .sheet-window__front::-webkit-scrollbar-thumb,
:root[data-display-palette='night'] .sheet-window__back::-webkit-scrollbar-thumb,
:root[data-display-palette='night'] .sheet-window__tabs :deep(.n-tab-pane::-webkit-scrollbar-thumb) {
  background: rgba(148, 163, 184, 0.5);
}
</style>
