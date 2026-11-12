<script setup lang="ts">
// 单选/多选工具栏共用，只提交意图；草稿写入仍由工作台拥有。
import { computed } from 'vue';

const props = defineProps<{ templateIds: string[]; values: Array<string | undefined>; disabled: boolean }>();
const emit = defineEmits<{ change: [templateId: string] }>();
const value = computed(() => {
  const values = new Set(props.values.map((v) => v || ''));
  return values.size === 1 ? [...values][0] : null;
});
const options = computed(() => [
  { label: '自动匹配', key: '', value: '' },
  ...props.templateIds.map((id) => ({ label: `指定 ${id}`, key: id, value: id })),
  ...[...new Set(props.values.filter((id): id is string => !!id && !props.templateIds.includes(id)))]
    .map((id) => ({ label: `模板已不存在或不可用于消息：${id}`, key: id, value: id, disabled: true })),
]);
const currentDescription = computed(() => {
  if (value.value === null) return '当前：多种格式';
  if (!value.value) return '当前：自动匹配';
  return props.templateIds.includes(value.value) ? `当前：指定 ${value.value}` : `当前：模板已失效 ${value.value}`;
});
</script>

<template>
  <n-dropdown trigger="click" placement="bottom-start" :options="options" :disabled="disabled" @select="(id: string | number) => emit('change', String(id))">
    <button
      type="button"
      class="log-template-select"
      :disabled="disabled"
      aria-label="样式"
      :title="`对话框样式；${currentDescription}。只修改当前 LOG，自动匹配会清除精确覆盖`"
    >样式</button>
  </n-dropdown>
</template>

<style scoped>
.log-template-select {
  flex: none;
  height: 28px;
  padding: 0 10px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.log-template-select:hover:not(:disabled) { background: var(--sc-fill-secondary, rgba(127, 127, 127, 0.16)); }
.log-template-select:disabled { cursor: default; opacity: 0.45; }
</style>
