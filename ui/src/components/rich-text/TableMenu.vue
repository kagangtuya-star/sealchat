<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Editor } from '@tiptap/vue-3';
import type {} from '@tiptap/extension-table';

const props = defineProps<{
  editor: Editor;
  stateVersion: number;
  disabled?: boolean;
  show: boolean;
  contentClass: string;
}>();
const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
  (event: 'interaction'): void;
}>();

const rows = ref(1);
const cols = ref(1);
const popoverTarget = computed(() => {
  // Resolve after opening, when EditorContent is mounted; stay inside modal/drawer focus scope.
  void props.show;
  return props.editor.view.dom.closest<HTMLElement>('[role="dialog"]') || undefined;
});
const commands = [
  ['addRowBefore', '上方插入行'],
  ['addRowAfter', '下方插入行'],
  ['addColumnBefore', '左侧插入列'],
  ['addColumnAfter', '右侧插入列'],
  ['deleteRow', '删除当前行'],
  ['deleteColumn', '删除当前列'],
  ['mergeCells', '合并单元格'],
  ['splitCell', '拆分单元格'],
  ['toggleHeaderRow', '切换表头行'],
  ['deleteTable', '删除整张表格'],
] as const;
type TableCommand = typeof commands[number][0];
const inTable = computed(() => {
  void props.stateVersion;
  return props.editor.isActive('table');
});
const actions = computed(() => {
  void props.stateVersion;
  return commands.map(([command, label]) => ({
    command, label, disabled: props.disabled || !props.editor.can()[command](),
  }));
});
const canInsert = computed(() => {
  void props.stateVersion;
  return !props.disabled && props.editor.can().insertTable({ rows: rows.value, cols: cols.value, withHeaderRow: true });
});
const selectSize = (index: number) => {
  rows.value = Math.floor(index / 6) + 1;
  cols.value = index % 6 + 1;
};
const insert = (index: number) => {
  selectSize(index);
  if (!canInsert.value) return;
  props.editor.chain().focus().insertTable({ rows: rows.value, cols: cols.value, withHeaderRow: true }).run();
  emit('update:show', false);
};
const run = (command: TableCommand) => {
  if (props.disabled || !props.editor.can()[command]()) return;
  // Keep the native selection, including CellSelection, when the popover takes focus.
  props.editor.chain().focus()[command]().run();
  emit('update:show', false);
};
const updateShow = (show: boolean) => {
  if (show) { rows.value = 1; cols.value = 1; }
  emit('interaction');
  emit('update:show', show);
};
</script>

<template>
  <n-popover
    trigger="click"
    placement="bottom-start"
    :show="props.show"
    :to="popoverTarget"
    :content-class="props.contentClass"
    @update:show="updateShow"
  >
    <template #trigger>
      <n-button size="small" text :disabled="props.disabled" :type="inTable ? 'primary' : 'default'" title="表格">
        表格
      </n-button>
    </template>
    <div class="table-menu" @pointerdown.stop="emit('interaction')">
      <div v-if="inTable" class="table-menu__actions">
        <n-button
          v-for="action in actions"
          :key="action.command"
          size="small"
          :disabled="action.disabled"
          :type="action.command === 'deleteTable' ? 'error' : 'default'"
          @click="run(action.command)"
        >{{ action.label }}</n-button>
      </div>
      <template v-else>
        <div class="table-menu__label" aria-live="polite">{{ cols }} 列 × {{ rows }} 行</div>
        <div class="table-menu__grid">
          <button
            v-for="index in 36"
            :key="index"
            type="button"
            :aria-label="`${(index - 1) % 6 + 1} 列 × ${Math.floor((index - 1) / 6) + 1} 行`"
            :class="{ 'is-selected': (index - 1) % 6 < cols && Math.floor((index - 1) / 6) < rows }"
            :disabled="!canInsert"
            @pointerenter="selectSize(index - 1)"
            @focus="selectSize(index - 1)"
            @click="insert(index - 1)"
          ></button>
        </div>
      </template>
    </div>
  </n-popover>
</template>

<style scoped>
.table-menu { max-height: min(70dvh, 400px); overflow-y: auto; }
.table-menu__label { margin-bottom: 8px; text-align: center; }
.table-menu__grid { display: grid; grid-template-columns: repeat(6, 28px); gap: 4px; }
.table-menu__grid button {
  height: 28px;
  border: 1px solid var(--sc-border-strong, currentColor);
  border-radius: 3px;
  background: transparent;
  cursor: pointer;
}
.table-menu__grid button.is-selected { background: var(--sc-bg-active); border-color: var(--sc-accent, #4098fc); }
.table-menu__grid button:focus-visible { outline: 2px solid var(--sc-accent, #4098fc); outline-offset: 1px; }
.table-menu__actions { display: grid; grid-template-columns: repeat(2, max-content); gap: 6px; }
</style>
