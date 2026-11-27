<script setup lang="ts">
import { ref, watch } from 'vue';
import type { LogBookmarkVisibility } from '@/types/log';

const props = defineProps<{
  show: boolean;
  label: string;
  location: string;
  busy: boolean;
}>();
const emit = defineEmits<{
  (e: 'update:show', show: boolean): void;
  (e: 'create', label: string, visibility: LogBookmarkVisibility): void;
}>();
const name = ref('');
const visibility = ref<LogBookmarkVisibility>('private');
watch(() => props.show, (show) => {
  if (!show) return;
  name.value = props.label;
  visibility.value = 'private';
});
const submit = () => {
  if (!props.busy && name.value.trim()) emit('create', name.value.trim(), visibility.value);
};
</script>

<template>
  <n-modal :show="show" preset="card" title="添加书签" style="width: min(380px, 94vw)" :closable="!busy" :mask-closable="!busy" :close-on-esc="!busy" @update:show="(value: boolean) => emit('update:show', value)">
    <form class="log-bookmark-form" @submit.prevent="submit">
      <n-input v-model:value="name" placeholder="书签名称" aria-label="书签名称" maxlength="60" :disabled="busy" />
      <span class="log-bookmark-form__location" :title="location">{{ location }}</span>
      <n-radio-group v-model:value="visibility" name="bookmark-visibility" aria-label="可见范围" size="small" :disabled="busy">
        <n-space :size="16">
          <n-radio value="private">仅自己可见</n-radio>
          <n-radio value="shared">共享</n-radio>
        </n-space>
      </n-radio-group>
      <div class="log-bookmark-form__actions">
        <n-button size="small" :disabled="busy" @click="emit('update:show', false)">取消</n-button>
        <n-button size="small" type="primary" attr-type="submit" :loading="busy" :disabled="!name.trim()">添加</n-button>
      </div>
    </form>
  </n-modal>
</template>

<style scoped>
.log-bookmark-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.log-bookmark-form__location {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--sc-text-secondary, #666);
  font-size: 12px;
}
.log-bookmark-form__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 4px;
}
</style>
