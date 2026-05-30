<script setup lang="ts">
import { ref } from 'vue';
import RichTextEditor from '@/components/rich-text/RichTextEditor.vue';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  maxlength?: number
  variant?: 'default' | 'keyword' | 'announcement' | 'sticky-note'
  minHeight?: string
}>(), {
  modelValue: '',
  placeholder: '用于聊天中的提示和解释（支持富文本格式）',
  maxlength: 2000,
  variant: 'keyword',
  minHeight: '220px',
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>();

const editorRef = ref<InstanceType<typeof RichTextEditor> | null>(null);

const updateValue = (...args: unknown[]) => {
  const value = typeof args[0] === 'string' ? args[0] : '';
  emit('update:modelValue', value);
};

defineExpose({
  focus: () => editorRef.value?.focus(),
  getEditor: () => editorRef.value?.getEditor?.(),
  getJson: () => editorRef.value?.getJson?.(),
  triggerFileSelect: () => editorRef.value?.triggerFileSelect?.(),
  hasOpenOverlay: () => editorRef.value?.hasOpenOverlay?.() ?? false,
  hasRecentOverlayInteraction: (thresholdMs?: number) =>
    editorRef.value?.hasRecentOverlayInteraction?.(thresholdMs) ?? false,
});
</script>

<template>
  <RichTextEditor
    ref="editorRef"
    :model-value="props.modelValue"
    :placeholder="props.placeholder"
    :maxlength="props.maxlength"
    :variant="props.variant"
    :min-height="props.minHeight"
    image-upload-mode="self"
    @update:model-value="updateValue"
  />
</template>
