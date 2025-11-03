<script setup lang="ts">
import { nextTick, ref, computed, onMounted, onBeforeUnmount } from 'vue';
import type { MentionOption } from 'naive-ui';
import InlineImagePreview from './InlineImagePreview.vue';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  disabled?: boolean
  whisperMode?: boolean
  mentionOptions?: MentionOption[]
  mentionLoading?: boolean
  mentionPrefix?: (string | number)[]
  mentionRenderLabel?: (option: MentionOption) => any
  autosize?: boolean | { minRows?: number; maxRows?: number }
  rows?: number
  inputClass?: string | Record<string, boolean> | Array<string | Record<string, boolean>>
  inlineImages?: Record<string, { status: 'uploading' | 'uploaded' | 'failed'; previewUrl?: string; error?: string }>
}>(), {
  modelValue: '',
  placeholder: '',
  disabled: false,
  whisperMode: false,
  mentionOptions: () => [],
  mentionLoading: false,
  mentionPrefix: () => ['@'],
  autosize: true,
  rows: 1,
  inputClass: () => [],
  inlineImages: () => ({}),
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'mention-search', value: string, prefix: string): void
  (event: 'mention-select', option: MentionOption): void
  (event: 'keydown', e: KeyboardEvent): void
  (event: 'focus'): void
  (event: 'blur'): void
  (event: 'remove-image', markerId: string): void
  (event: 'paste-image', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-files', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
}>();

const mentionRef = ref<any>(null);
const wrapperRef = ref<HTMLElement | null>(null);

const classList = computed(() => {
  const base: string[] = ['chat-text'];
  if (props.whisperMode) {
    base.push('whisper-mode');
  }
  const append = (item: any) => {
    if (!item) return;
    if (typeof item === 'string') {
      base.push(item);
    } else if (Array.isArray(item)) {
      item.forEach(append);
    } else if (typeof item === 'object') {
      Object.entries(item).forEach(([key, value]) => {
        if (value) {
          base.push(key);
        }
      });
    }
  };
  append(props.inputClass);
  return base;
});

const valueBinder = computed({
  get: () => props.modelValue,
  set: (value: string) => emit('update:modelValue', value),
});

const handleSearch = (value: string, prefix: string) => {
  emit('mention-search', value, prefix);
};

const handleSelect = (option: MentionOption) => {
  emit('mention-select', option);
};

const handleKeydown = (event: KeyboardEvent) => {
  emit('keydown', event);
};

const handleRemoveImage = (markerId: string) => {
  emit('remove-image', markerId);
};

const getTextarea = (): HTMLTextAreaElement | undefined => {
  const textarea = mentionRef.value?.$el?.querySelector?.('textarea');
  return textarea || undefined;
};

// 处理粘贴事件
const handlePaste = (event: ClipboardEvent) => {
  const items = event.clipboardData?.items;
  if (!items) return;

  const files: File[] = [];
  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    if (item.kind === 'file' && item.type.startsWith('image/')) {
      const file = item.getAsFile();
      if (file) {
        files.push(file);
      }
    }
  }

  if (files.length > 0) {
    event.preventDefault();
    const textarea = getTextarea();
    const start = textarea?.selectionStart || 0;
    const end = textarea?.selectionEnd || 0;
    emit('paste-image', { files, selectionStart: start, selectionEnd: end });
  }
};

// 处理拖拽事件
const handleDrop = (event: DragEvent) => {
  event.preventDefault();
  event.stopPropagation();

  const files = Array.from(event.dataTransfer?.files || []).filter((file) =>
    file.type.startsWith('image/')
  );

  if (files.length > 0) {
    const textarea = getTextarea();
    const start = textarea?.selectionStart || 0;
    const end = textarea?.selectionEnd || 0;
    emit('drop-files', { files, selectionStart: start, selectionEnd: end });
  }
};

const handleDragOver = (event: DragEvent) => {
  event.preventDefault();
  event.stopPropagation();
};

// 挂载和卸载事件监听
onMounted(() => {
  nextTick(() => {
    const textarea = getTextarea();
    if (textarea) {
      textarea.addEventListener('paste', handlePaste as EventListener);
      textarea.addEventListener('drop', handleDrop as EventListener);
      textarea.addEventListener('dragover', handleDragOver as EventListener);
    }
  });
});

onBeforeUnmount(() => {
  const textarea = getTextarea();
  if (textarea) {
    textarea.removeEventListener('paste', handlePaste as EventListener);
    textarea.removeEventListener('drop', handleDrop as EventListener);
    textarea.removeEventListener('dragover', handleDragOver as EventListener);
  }
});

const focus = () => {
  nextTick(() => {
    mentionRef.value?.focus?.();
    if (!mentionRef.value?.focus && mentionRef.value?.$el) {
      const textarea = mentionRef.value.$el.querySelector('textarea') as HTMLTextAreaElement | null;
      textarea?.focus();
    }
  });
};

const blur = () => {
  const textarea = getTextarea();
  textarea?.blur();
};

defineExpose({
  focus,
  blur,
  getTextarea,
  getInstance: () => mentionRef.value,
});
</script>

<template>
  <div class="chat-input-plain-wrapper" ref="wrapperRef">
    <n-mention
      ref="mentionRef"
      type="textarea"
      :rows="rows"
      :autosize="autosize"
      :placeholder="placeholder"
      :disabled="disabled"
      v-model:value="valueBinder"
      :options="mentionOptions"
      :loading="mentionLoading"
      :prefix="mentionPrefix"
      :render-label="mentionRenderLabel"
      placement="top-start"
      :class="classList"
      @search="handleSearch"
      @select="handleSelect"
      @keydown="handleKeydown"
      @focus="emit('focus')"
      @blur="emit('blur')"
    />
    <InlineImagePreview
      :images="inlineImages"
      @remove="handleRemoveImage"
    />
  </div>
</template>

<style lang="scss" scoped>
.chat-input-plain-wrapper {
  width: 100%;
}
</style>
