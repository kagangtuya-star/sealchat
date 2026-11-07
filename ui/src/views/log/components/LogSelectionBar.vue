<script setup lang="ts">
// 浮动上下文工具条：按每条消息的 MessageCapabilities 展示/禁用并说明原因；不依赖 hover，触屏可用。
// 高频操作直接可见，作用于聊天原记录的破坏性操作收进“更多”菜单。
import { computed } from 'vue';
import type { DropdownOption } from 'naive-ui';
import {
  ArrowDown,
  ArrowUp,
  Ban,
  Dots,
  Focus2,
  ListCheck,
  Palette,
  Pencil,
  RowInsertBottom,
  RowInsertTop,
  Trash,
  UserCircle,
  X,
} from '@vicons/tabler';
import type { LogDesignMessage, LogElement } from '@/types/log';
import { LOG_ELEMENT_LABELS } from '@/services/log/logRenderer';
import LogMessageTemplateSelect from './LogMessageTemplateSelect.vue';

const props = defineProps<{
  messages: LogDesignMessage[];
  element: LogElement | null;
  canEditDraft: boolean;
  canManageSources: boolean;
  busy: boolean;
  isFirst: boolean;
  isLast: boolean;
  relocatingCount?: number;
  templateIds: string[];
  templateOverrides: Record<string, string>;
}>();

const emit = defineEmits<{
  edit: [tab: 'content' | 'identity' | 'appearance'];
  move: [dir: -1 | 1];
  relocate: [];
  'cancel-relocate': [];
  insert: [side: 'before' | 'after'];
  remove: [];
  exclude: [];
  'edit-element': [];
  'remove-element': [];
  /** 进入多选模式，当前已选消息成为初始选择。 */
  multi: [];
  clear: [];
  template: [templateId: string];
}>();

const single = computed(() => (props.messages.length === 1 ? props.messages[0] : null));
const caps = computed(() => single.value?.capabilities);
const reason = (allowed: boolean | undefined, text: string) => (allowed ? '' : text);
/** 可用时显示操作名，不可用时显示原因。 */
const tip = (label: string, allowed: boolean | undefined, why: string) => (allowed ? label : `${label}：${why}`);

const editTip = computed(() => tip('编辑正文', caps.value?.edit, '无权编辑：仅作者或已开启代编辑的上级管理员'));
const reorderWhy = '无权调整顺序';
const excludeWhy = '仅世界管理者可以调整公开范围';

const moreOptions = computed<DropdownOption[]>(() => [
  {
    key: 'remove',
    label: '删除原消息',
    disabled: props.busy || !caps.value?.remove,
    props: { title: reason(caps.value?.remove, '无权删除此消息') },
  },
]);
const onMore = (key: string) => {
  if (key === 'remove') emit('remove');
};
</script>

<template>
  <div class="log-ctx" :class="{ 'log-ctx--relocating': relocatingCount }" role="toolbar" :aria-label="relocatingCount ? '重定位消息' : '选中内容操作'">
    <template v-if="relocatingCount">
      <span class="log-ctx__label" role="status">
        <n-icon :component="Focus2" size="16" />
        <span>正在重定位 {{ relocatingCount }} 条消息 · 点击目标消息，将所选消息移动到其后</span>
      </span>
    </template>
    <template v-else-if="element">
      <span class="log-ctx__label">
        <span class="log-ctx__kind is-element">元素</span>
        <span class="log-ctx__name">{{ LOG_ELEMENT_LABELS[element.type] || element.type }}</span>
      </span>
      <span class="log-ctx__sep" />
      <button type="button" class="log-ctx__btn has-text" :disabled="!canEditDraft" title="在设计面板中编辑此 LOG 元素（不写入聊天）" @click="emit('edit-element')">
        <n-icon :component="Pencil" size="16" /><span>编辑元素</span>
      </button>
      <button type="button" class="log-ctx__btn is-danger" :disabled="!canEditDraft" title="删除元素" aria-label="删除元素" @click="emit('remove-element')">
        <n-icon :component="Trash" size="16" />
      </button>
    </template>

    <template v-else-if="single">
      <span class="log-ctx__label">
        <span class="log-ctx__kind">原消息</span>
      </span>
      <span class="log-ctx__sep" />
      <LogMessageTemplateSelect :template-ids="templateIds" :values="[templateOverrides[single.message.id]]" :disabled="busy || !canEditDraft" @change="(id) => emit('template', id)" />
      <button type="button" class="log-ctx__btn has-text" :disabled="busy || !caps?.edit" :title="editTip" @click="emit('edit', 'content')">
        <n-icon :component="Pencil" size="16" /><span>编辑</span>
      </button>
      <button type="button" class="log-ctx__btn" :disabled="busy || !caps?.changeIdentity" :title="tip('角色/差分', caps?.changeIdentity, '无权切换此消息的角色')" aria-label="角色/差分" @click="emit('edit', 'identity')">
        <n-icon :component="UserCircle" size="16" />
      </button>
      <button type="button" class="log-ctx__btn" :disabled="busy || !caps?.editAppearance" :title="tip('本条外观', caps?.editAppearance, '无权修改此消息的历史外观')" aria-label="本条外观" @click="emit('edit', 'appearance')">
        <n-icon :component="Palette" size="16" />
      </button>
      <span class="log-ctx__sep" />
      <button type="button" class="log-ctx__btn" :disabled="busy || !caps?.reorder || isFirst" :title="tip('上移', caps?.reorder, reorderWhy)" aria-label="上移" @click="emit('move', -1)">
        <n-icon :component="ArrowUp" size="16" />
      </button>
      <button type="button" class="log-ctx__btn" :disabled="busy || !caps?.reorder || isLast" :title="tip('下移', caps?.reorder, reorderWhy)" aria-label="下移" @click="emit('move', 1)">
        <n-icon :component="ArrowDown" size="16" />
      </button>
      <button type="button" class="log-ctx__btn has-text" :disabled="busy || !caps?.reorder" :title="tip('重定位', caps?.reorder, reorderWhy)" @click="emit('relocate')">
        <n-icon :component="Focus2" size="16" /><span>重定位</span>
      </button>
      <span class="log-ctx__sep" />
      <button type="button" class="log-ctx__btn" :disabled="!canEditDraft" title="在此消息前插入元素" aria-label="前插元素" @click="emit('insert', 'before')">
        <n-icon :component="RowInsertTop" size="16" />
      </button>
      <button type="button" class="log-ctx__btn" :disabled="!canEditDraft" title="在此消息后插入元素" aria-label="后插元素" @click="emit('insert', 'after')">
        <n-icon :component="RowInsertBottom" size="16" />
      </button>
      <span class="log-ctx__sep" />
      <button type="button" class="log-ctx__btn has-text" :disabled="busy || !canManageSources" :title="tip('仅从本 LOG 排除（聊天原消息不受影响）', canManageSources, excludeWhy)" @click="emit('exclude')">
        <n-icon :component="Ban" size="16" /><span>排除</span>
      </button>
      <n-dropdown trigger="click" placement="bottom-end" :options="moreOptions" @select="onMore">
        <button type="button" class="log-ctx__btn" title="更多：删除原消息" aria-label="更多操作">
          <n-icon :component="Dots" size="16" />
        </button>
      </n-dropdown>
      <span class="log-ctx__sep" />
      <button type="button" class="log-ctx__btn" :disabled="busy" title="进入多选模式（从此消息开始）" aria-label="多选" @click="emit('multi')">
        <n-icon :component="ListCheck" size="16" />
      </button>
    </template>

    <template v-else-if="messages.length > 1">
      <span class="log-ctx__label">
        <span class="log-ctx__kind">已选 {{ messages.length }} 条消息</span>
      </span>
      <span class="log-ctx__sep" />
      <button
        type="button"
        class="log-ctx__btn has-text"
        :disabled="busy || !canManageSources || messages.length > 200"
        :title="!canManageSources ? excludeWhy : messages.length > 200 ? '一次最多排除 200 条' : '从本 LOG 排除（聊天原消息不受影响）'"
        @click="emit('exclude')"
      >
        <n-icon :component="Ban" size="16" /><span>从本 LOG 排除</span>
      </button>
      <button type="button" class="log-ctx__btn has-text" :disabled="busy" title="进入多选模式，使用批量移动、重定位与删除" @click="emit('multi')">
        <n-icon :component="ListCheck" size="16" /><span>多选模式</span>
      </button>
    </template>

    <span class="log-ctx__sep" />
    <button v-if="relocatingCount" type="button" class="log-ctx__btn has-text" title="取消重定位 (Esc)" @click="emit('cancel-relocate')">
      <n-icon :component="X" size="16" /><span>取消</span>
    </button>
    <button v-else type="button" class="log-ctx__btn" title="取消选择 (Esc)" aria-label="取消选择" @click="emit('clear')">
      <n-icon :component="X" size="16" />
    </button>
  </div>
</template>

<style scoped>
.log-ctx {
  display: flex;
  align-items: center;
  gap: 2px;
  max-width: 100%;
  padding: 4px;
  border-radius: 10px;
  border: 1px solid var(--log-wb-line, var(--sc-border-strong, rgba(15, 23, 42, 0.12)));
  background: var(--sc-bg-elevated, #fff);
  color: var(--sc-text-primary, #0f172a);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.14), 0 1px 3px rgba(15, 23, 42, 0.08);
  overflow-x: auto;
  scrollbar-width: none;
}

.log-ctx::-webkit-scrollbar {
  display: none;
}

.log-ctx__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px 0 4px;
  white-space: nowrap;
  font-size: 12px;
}

.log-ctx--relocating .log-ctx__label {
  min-width: 0;
  white-space: normal;
}

.log-ctx__kind {
  padding: 2px 7px;
  border-radius: 6px;
  font-weight: 600;
  color: var(--log-wb-accent, #2f6fde);
  background: color-mix(in srgb, var(--log-wb-accent, #2f6fde) 12%, transparent);
}

.log-ctx__kind.is-element {
  color: #9a5b00;
  background: rgba(217, 119, 6, 0.14);
}

.log-ctx__name {
  font-weight: 600;
}

.log-ctx__sep {
  flex: none;
  width: 1px;
  height: 18px;
  margin: 0 3px;
  background: var(--sc-border-strong, rgba(15, 23, 42, 0.12));
}

.log-ctx__btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 30px;
  height: 30px;
  padding: 0 7px;
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

.log-ctx__btn.has-text {
  padding: 0 9px;
}

.log-ctx__btn:hover:not(:disabled) {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
}

.log-ctx__btn:focus-visible {
  outline: 2px solid var(--log-wb-accent, #2f6fde);
  outline-offset: -2px;
}

.log-ctx__btn:disabled {
  opacity: 0.38;
  cursor: not-allowed;
}

.log-ctx__btn.is-danger:hover:not(:disabled) {
  color: #d03050;
  background: rgba(208, 48, 80, 0.1);
}

:root[data-display-palette='night'] .log-ctx__kind.is-element {
  color: #f0b45a;
}
</style>
