<script setup lang="ts">
import { nextTick, ref, computed } from 'vue';
import type { MentionOption } from 'naive-ui';

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
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'mention-search', value: string, prefix: string): void
  (event: 'mention-select', option: MentionOption): void
  (event: 'keydown', e: KeyboardEvent): void
  (event: 'focus'): void
  (event: 'blur'): void
}>();

const mentionRef = ref<any>(null);

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

const getTextarea = (): HTMLTextAreaElement | undefined => {
  const textarea = mentionRef.value?.$el?.querySelector?.('textarea');
  return textarea || undefined;
};

defineExpose({
  focus,
  blur,
  getTextarea,
  getInstance: () => mentionRef.value,
});
</script>

<template>
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
</template>
