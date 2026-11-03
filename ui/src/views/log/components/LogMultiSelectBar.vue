<script setup lang="ts">
// 多选模式工具条：批量能力按整个选中集合计算（任一条不可操作即整体禁用并说明原因），不做部分执行。
// 不可用的按钮保持可聚焦（aria-disabled），悬停或聚焦即可看到原因。
import { computed } from 'vue';
import { ArrowDown, ArrowUp, ArrowsVertical, Ban, Checks, Focus2, ListCheck, Trash, X } from '@vicons/tabler';
import type { LogDesignMessage } from '@/types/log';
import LogMessageTemplateSelect from './LogMessageTemplateSelect.vue';

const BATCH_LIMIT = 200;

const props = defineProps<{
  /** 已选消息（按显示顺序）。 */
  messages: LogDesignMessage[];
  /** 当前来源片段已加载的消息数。 */
  total: number;
  complete: boolean;
  atTop: boolean;
  atBottom: boolean;
  canManageSources: boolean;
  busy: boolean;
  rangeMode: boolean;
  rangeAnchored: boolean;
  relocating: boolean;
  canEditDraft: boolean;
  templateIds: string[];
  templateOverrides: Record<string, string>;
}>();

const emit = defineEmits<{
  'select-all': [];
  'toggle-range': [];
  move: [dir: -1 | 1];
  relocate: [];
  'cancel-relocate': [];
  exclude: [];
  remove: [];
  exit: [];
  template: [templateId: string];
}>();

const count = computed(() => props.messages.length);
const denied = (key: 'reorder' | 'remove') => props.messages.filter((m) => !m.capabilities[key]).length;

/** 返回不可用原因；空字符串表示可用。顺序：忙碌 → 未选择 → 数量上限 → 权限 → 位置。 */
const common = () => {
  if (props.busy) return '正在处理上一个操作';
  if (!count.value) return '请先选择消息';
  if (count.value > BATCH_LIMIT) return `一次最多操作 ${BATCH_LIMIT} 条`;
  return '';
};
const reorderWhy = () => {
  const n = denied('reorder');
  return n ? `所选消息中有 ${n} 条无权调整顺序` : '';
};
const reasons = computed(() => ({
  up: common() || reorderWhy() || (props.atTop ? '已到达本片段已加载范围的顶部' : ''),
  down: common() || reorderWhy() || (props.atBottom ? (props.complete ? '已到达本片段底部' : '已到达已加载范围的底部，请先加载更多') : ''),
  relocate: common() || reorderWhy() || (count.value >= props.total ? '没有可作为目标的其他消息' : ''),
  exclude: common() || (props.canManageSources ? '' : '仅世界管理者可以调整公开范围'),
  remove: common() || (denied('remove') ? `所选消息中有 ${denied('remove')} 条无权删除` : ''),
}));
type ActionKey = keyof typeof reasons.value;
const tip = (key: ActionKey, label: string) => (reasons.value[key] ? `${label}：${reasons.value[key]}` : label);
const run = (key: ActionKey, fn: () => void) => {
  if (!reasons.value[key]) fn();
};

const allSelected = computed(() => props.total > 0 && count.value >= props.total);
const hint = computed(() => {
  if (props.relocating) return '点击目标消息，所选消息将按原顺序移到它之后';
  if (props.rangeMode) return props.rangeAnchored ? '点击终点，选中两者之间的全部消息' : '点击起点消息开始范围选择';
  if (!count.value) return '点击消息选择，Shift+点击连选';
  if (allSelected.value && !props.complete) return '仅包含已加载的消息';
  return '';
});
</script>

<template>
  <div class="log-multi" :class="{ 'is-relocating': relocating }" role="toolbar" :aria-label="relocating ? '批量重定位' : '多选消息'">
    <div class="log-multi__info">
      <span class="log-multi__mode">
        <n-icon :component="relocating ? Focus2 : ListCheck" size="15" />
        <span>{{ relocating ? '重定位' : '多选' }}</span>
      </span>
      <span class="log-multi__count" aria-live="polite">已选 <strong>{{ count }}</strong> 条</span>
      <span v-if="hint" class="log-multi__hint">{{ hint }}</span>
    </div>

    <div class="log-multi__actions">
      <template v-if="relocating">
        <button type="button" class="log-multi__btn has-text" title="取消重定位，保留当前选择 (Esc)" @click="emit('cancel-relocate')">
          <n-icon :component="X" size="16" /><span>取消重定位</span>
        </button>
      </template>
      <template v-else>
        <LogMessageTemplateSelect class="log-multi__format" :template-ids="templateIds" :values="messages.map((m) => templateOverrides[m.message.id])" :disabled="!!common() || !canEditDraft" @change="(id) => emit('template', id)" />
        <button
          type="button"
          class="log-multi__btn"
          :aria-disabled="busy || !total || allSelected"
          :title="allSelected ? '已全选当前片段已加载的消息' : '全选当前片段已加载的消息（不含 LOG 元素）'"
          @click="!busy && total && !allSelected && emit('select-all')"
        >
          <n-icon :component="Checks" size="16" /><span>全选</span>
        </button>
        <button
          type="button"
          class="log-multi__btn"
          :class="{ 'is-on': rangeMode }"
          :aria-pressed="rangeMode"
          :aria-disabled="busy"
          :title="rangeMode ? '关闭范围选择' : '范围选择：依次点击起点与终点（也可 Shift+点击）'"
          @click="!busy && emit('toggle-range')"
        >
          <n-icon :component="ArrowsVertical" size="16" /><span>范围</span>
        </button>
        <span class="log-multi__sep" />
        <button type="button" class="log-multi__btn is-icon" :aria-disabled="!!reasons.up" :title="tip('up', '整体上移一位')" aria-label="整体上移" @click="run('up', () => emit('move', -1))">
          <n-icon :component="ArrowUp" size="16" />
        </button>
        <button type="button" class="log-multi__btn is-icon" :aria-disabled="!!reasons.down" :title="tip('down', '整体下移一位')" aria-label="整体下移" @click="run('down', () => emit('move', 1))">
          <n-icon :component="ArrowDown" size="16" />
        </button>
        <button type="button" class="log-multi__btn" :aria-disabled="!!reasons.relocate" :title="tip('relocate', '重定位：选择目标消息，按原顺序移到其后')" @click="run('relocate', () => emit('relocate'))">
          <n-icon :component="Focus2" size="16" /><span>重定位</span>
        </button>
        <span class="log-multi__sep" />
        <button type="button" class="log-multi__btn" :aria-disabled="!!reasons.exclude" :title="tip('exclude', '仅从本 LOG 排除（聊天原消息不受影响）')" @click="run('exclude', () => emit('exclude'))">
          <n-icon :component="Ban" size="16" /><span>排除</span>
        </button>
        <button type="button" class="log-multi__btn is-danger" :aria-disabled="!!reasons.remove" :title="tip('remove', '删除聊天中的原消息')" @click="run('remove', () => emit('remove'))">
          <n-icon :component="Trash" size="16" /><span>删除</span>
        </button>
        <span class="log-multi__sep" />
        <button type="button" class="log-multi__btn is-exit" title="退出多选 (Esc)" @click="emit('exit')">
          <n-icon :component="X" size="16" /><span>退出</span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.log-multi {
  --log-multi-accent: var(--log-wb-accent, #2f6fde);
  display: flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  min-width: 0;
  padding: 4px 4px 4px 6px;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--log-multi-accent) 34%, var(--log-wb-line, rgba(15, 23, 42, 0.12)));
  background: var(--sc-bg-elevated, #fff);
  color: var(--sc-text-primary, #0f172a);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.14), 0 1px 3px rgba(15, 23, 42, 0.08);
}

.log-multi__info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding-right: 2px;
  font-size: 12px;
}

.log-multi__mode {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  padding: 0 8px;
  border-radius: 7px;
  font-weight: 600;
  color: var(--log-multi-accent);
  background: color-mix(in srgb, var(--log-multi-accent) 13%, transparent);
}

.log-multi__count {
  flex: none;
}

.log-multi__count strong {
  font-variant-numeric: tabular-nums;
}

.log-multi__hint {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--log-wb-muted, var(--sc-text-secondary, #475569));
}

.log-multi__actions {
  flex: none;
  display: flex;
  align-items: center;
  gap: 2px;
}

.log-multi__sep {
  flex: none;
  width: 1px;
  height: 18px;
  margin: 0 3px;
  background: var(--sc-border-strong, rgba(15, 23, 42, 0.12));
}

.log-multi__btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 30px;
  height: 30px;
  padding: 0 9px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease;
}

.log-multi__btn.is-icon {
  padding: 0 7px;
}

.log-multi__btn:hover:not([aria-disabled='true']) {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
}

.log-multi__btn:focus-visible {
  outline: 2px solid var(--log-multi-accent);
  outline-offset: -2px;
}

.log-multi__btn[aria-disabled='true'] {
  opacity: 0.38;
  cursor: not-allowed;
}

.log-multi__btn.is-on {
  color: var(--log-multi-accent);
  background: color-mix(in srgb, var(--log-multi-accent) 13%, transparent);
}

.log-multi__btn.is-danger {
  color: #d03050;
}

.log-multi__btn.is-danger:hover:not([aria-disabled='true']) {
  background: rgba(208, 48, 80, 0.1);
}

.log-multi__btn.is-exit {
  color: var(--log-wb-muted, var(--sc-text-secondary, #475569));
}

:root[data-display-palette='night'] .log-multi__btn.is-danger {
  color: #f2788e;
}

/* 画布较窄时只保留图标；再窄时说明独占一行，按钮整行排布，均可点击、不横向溢出。 */
@container (max-width: 820px) {
  .log-multi__btn > span {
    display: none;
  }

  .log-multi.is-relocating .log-multi__btn > span {
    display: inline;
  }
}

@container (max-width: 560px) {
  .log-multi {
    flex-wrap: wrap;
    width: 100%;
    padding: 6px;
    row-gap: 4px;
  }

  .log-multi__info {
    flex: 1 1 100%;
    padding: 0 2px;
  }

  .log-multi__actions {
    flex: 1 1 100%;
    flex-wrap: wrap;
    justify-content: space-between;
  }

  .log-multi__actions .log-multi__format {
    flex: 1 1 100%;
    width: 100%;
    margin-bottom: 4px;
  }

  .log-multi__sep {
    display: none;
  }

  .log-multi__btn {
    flex: 1 1 0;
    min-width: 0;
    padding: 0 4px;
  }
}
</style>
