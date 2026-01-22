<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="visible"
        class="dice-roll-popover-overlay"
        @click.self="handleCancel"
      >
        <div
          class="dice-roll-popover"
          :style="popoverStyle"
        >
          <div class="popover-header">
            <span class="popover-label">{{ label || '技能检定' }}</span>
            <button class="popover-close" @click="handleCancel">×</button>
          </div>

          <div class="popover-body">
            <div class="popover-template">
              <span class="template-preview">{{ previewExpression }}</span>
            </div>

            <div class="popover-mode">
              <button
                class="mode-btn"
                :class="{ active: mode === 'dis' }"
                @click="mode = 'dis'"
              >
                劣势
              </button>
              <button
                class="mode-btn"
                :class="{ active: mode === 'normal' }"
                @click="mode = 'normal'"
              >
                普通
              </button>
              <button
                class="mode-btn"
                :class="{ active: mode === 'adv' }"
                @click="mode = 'adv'"
              >
                优势
              </button>
            </div>

            <div class="popover-modifier">
              <label>额外加值</label>
              <div class="modifier-input">
                <button class="modifier-btn" @click="modifier--">-</button>
                <n-input-number
                  v-model:value="modifier"
                  :show-button="false"
                  size="small"
                  class="modifier-number"
                />
                <button class="modifier-btn" @click="modifier++">+</button>
              </div>
            </div>
          </div>

          <div class="popover-footer">
            <n-button size="small" @click="handleCancel">取消</n-button>
            <n-button type="primary" size="small" @click="handleConfirm">
              掷骰
            </n-button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue';
import { NButton, NInputNumber } from 'naive-ui';

const props = defineProps<{
  visible: boolean;
  label?: string;
  template: string;
  args?: Record<string, any>;
  targetRect?: { top: number; left: number; width: number; height: number };
  containerRect?: { top: number; left: number; width: number; height: number };
}>();

const emit = defineEmits<{
  confirm: [expression: string];
  cancel: [];
  'update:visible': [value: boolean];
}>();

const mode = ref<'normal' | 'adv' | 'dis'>('normal');
const modifier = ref(0);

const POPUP_WIDTH = 280;
const POPUP_HEIGHT = 220;
const POPUP_PADDING = 8;

const clamp = (value: number, min: number, max: number) => {
  const safeMax = Math.max(min, max);
  return Math.min(Math.max(value, min), safeMax);
};

const popoverStyle = computed(() => {
  const bounds = props.containerRect
    ? {
        left: props.containerRect.left + POPUP_PADDING,
        right: props.containerRect.left + props.containerRect.width - POPUP_PADDING,
        top: props.containerRect.top + POPUP_PADDING,
        bottom: props.containerRect.top + props.containerRect.height - POPUP_PADDING,
      }
    : {
        left: POPUP_PADDING,
        right: window.innerWidth - POPUP_PADDING,
        top: POPUP_PADDING,
        bottom: window.innerHeight - POPUP_PADDING,
      };

  if (!props.targetRect) {
    const centerLeft = (bounds.left + bounds.right) / 2;
    const centerTop = (bounds.top + bounds.bottom) / 2;
    const maxTop = bounds.bottom - POPUP_HEIGHT;
    const maxLeft = bounds.right - POPUP_WIDTH;
    return {
      top: `${clamp(centerTop - POPUP_HEIGHT / 2, bounds.top, maxTop)}px`,
      left: `${clamp(centerLeft - POPUP_WIDTH / 2, bounds.left, maxLeft)}px`,
    };
  }

  const preferredTop = props.targetRect.top + props.targetRect.height + 8;
  const preferredLeft = props.targetRect.left + props.targetRect.width / 2 - POPUP_WIDTH / 2;
  const maxTop = bounds.bottom - POPUP_HEIGHT;
  const maxLeft = bounds.right - POPUP_WIDTH;
  return {
    top: `${clamp(preferredTop, bounds.top, maxTop)}px`,
    left: `${clamp(preferredLeft, bounds.left, maxLeft)}px`,
  };
});

const previewExpression = computed(() => {
  let expr = props.template || '';
  const args = props.args || {};
  for (const [key, value] of Object.entries(args)) {
    expr = expr.replace(new RegExp(`\\{${key}\\}`, 'g'), String(value));
  }
  const trimmed = expr.trim();
  const match = trimmed.match(/^(\\.[^\\s]+)\\s*(.*)$/);
  const command = match ? match[1] : '.ra';
  let body = match ? match[2] : trimmed;
  const bodyTrimmed = body.trim();
  const commandPrefix = `${command} `;
  if (bodyTrimmed.startsWith(commandPrefix)) {
    body = bodyTrimmed.slice(commandPrefix.length);
  } else if (bodyTrimmed === command) {
    body = '';
  }
  const modText = modifier.value === 0 ? '' : `${modifier.value > 0 ? '+' : ''}${modifier.value}`;
  const commandWithMod = `${command}${modText}`;
  if (modifier.value !== 0) {
    body = body.trim();
  }
  body = body.trim();
  const modeToken = mode.value === 'adv' ? 'b' : mode.value === 'dis' ? 'p' : '';
  if (!body) {
    return modeToken ? `${commandWithMod} ${modeToken}` : commandWithMod;
  }
  return modeToken ? `${commandWithMod} ${modeToken} ${body}` : `${commandWithMod} ${body}`;
});

const handleConfirm = () => {
  emit('confirm', previewExpression.value);
  emit('update:visible', false);
};

const handleCancel = () => {
  emit('cancel');
  emit('update:visible', false);
};

const handleKeydown = (e: KeyboardEvent) => {
  if (!props.visible) return;
  if (e.key === 'Escape') {
    handleCancel();
  } else if (e.key === 'Enter' && !e.shiftKey) {
    handleConfirm();
  }
};

watch(
  () => props.visible,
  (v) => {
    if (v) {
      mode.value = 'normal';
      modifier.value = 0;
    }
  }
);

onMounted(() => {
  document.addEventListener('keydown', handleKeydown);
});

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeydown);
});
</script>

<style scoped>
.dice-roll-popover-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.3);
  z-index: 2200;
  display: flex;
  align-items: flex-start;
  justify-content: center;
}

.dice-roll-popover {
  position: absolute;
  min-width: 240px;
  max-width: 300px;
  background: var(--sc-bg-elevated, #fff);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
  overflow: hidden;
  pointer-events: auto;
}

.popover-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--sc-bg-panel, #f9fafb);
  border-bottom: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.06));
}

.popover-label {
  font-size: 14px;
  font-weight: 600;
  color: var(--sc-text-primary, #1f2937);
}

.popover-close {
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  border-radius: 6px;
  cursor: pointer;
  font-size: 18px;
  color: var(--sc-text-secondary, #6b7280);
  display: flex;
  align-items: center;
  justify-content: center;
}

.popover-close:hover {
  background: rgba(0, 0, 0, 0.08);
}

.popover-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.popover-template {
  padding: 8px 12px;
  background: var(--sc-bg-input, #f3f4f6);
  border-radius: 8px;
  font-family: 'SF Mono', monospace;
  font-size: 13px;
  color: var(--sc-text-primary, #1f2937);
  word-break: break-all;
}

.popover-mode {
  display: flex;
  gap: 4px;
}

.mode-btn {
  flex: 1;
  padding: 6px 8px;
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.1));
  background: var(--sc-bg-elevated, #fff);
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  color: var(--sc-text-secondary, #6b7280);
  transition: all 0.15s ease;
}

.mode-btn:hover {
  background: var(--sc-bg-panel, #f9fafb);
}

.mode-btn.active {
  background: var(--primary-color, #3388de);
  border-color: var(--primary-color, #3388de);
  color: #fff;
}

.popover-modifier {
  display: flex;
  align-items: center;
  gap: 8px;
}

.popover-modifier label {
  font-size: 13px;
  color: var(--sc-text-secondary, #6b7280);
  white-space: nowrap;
}

.modifier-input {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
}

.modifier-btn {
  width: 28px;
  height: 28px;
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.1));
  background: var(--sc-bg-elevated, #fff);
  border-radius: 6px;
  cursor: pointer;
  font-size: 16px;
  color: var(--sc-text-primary, #1f2937);
}

.modifier-btn:hover {
  background: var(--sc-bg-panel, #f9fafb);
}

.modifier-number {
  flex: 1;
  max-width: 60px;
}

.modifier-number :deep(.n-input__input-el) {
  text-align: center;
}

.popover-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 10px 12px;
  background: var(--sc-bg-panel, #f9fafb);
  border-top: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.06));
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

:root[data-display-palette='night'] .dice-roll-popover {
  background: var(--sc-bg-elevated, #1e293b);
}

:root[data-display-palette='night'] .popover-header,
:root[data-display-palette='night'] .popover-footer {
  background: var(--sc-bg-panel, rgba(30, 41, 59, 0.95));
}

:root[data-display-palette='night'] .mode-btn {
  background: var(--sc-bg-elevated, #1e293b);
  border-color: rgba(148, 163, 184, 0.2);
  color: var(--sc-text-secondary, #94a3b8);
}

:root[data-display-palette='night'] .modifier-btn {
  background: var(--sc-bg-elevated, #1e293b);
  border-color: rgba(148, 163, 184, 0.2);
  color: var(--sc-text-primary, #f1f5f9);
}
</style>
