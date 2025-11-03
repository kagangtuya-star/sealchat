<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue';
import type { MentionOption } from 'naive-ui';
import ChatInputPlain from './inputs/ChatInputPlain.vue';

type EditorMode = 'plain' | 'rich';

const props = withDefaults(defineProps<{
  modelValue: string
  mode?: EditorMode
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
  mode: 'plain',
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
  (event: 'update:mode', value: EditorMode): void
  (event: 'mention-search', value: string, prefix: string): void
  (event: 'mention-select', option: MentionOption): void
  (event: 'keydown', e: KeyboardEvent): void
  (event: 'focus'): void
  (event: 'blur'): void
  (event: 'rich-needed'): void
}>();

const modeRef = ref<EditorMode>(props.mode);
watch(() => props.mode, (value) => {
  if (value && value !== modeRef.value) {
    modeRef.value = value;
  }
});

const switchMode = (value: EditorMode) => {
  if (modeRef.value === value) {
    return;
  }
  modeRef.value = value;
  emit('update:mode', value);
};

const plainRef = ref<InstanceType<typeof ChatInputPlain> | null>(null);
const richRef = ref<any>(null);

const currentComponent = computed(() => modeRef.value);

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

const handleFocus = () => {
  emit('focus');
};

const handleBlur = () => {
  emit('blur');
};

const focus = () => {
  nextTick(() => {
    if (modeRef.value === 'plain') {
      plainRef.value?.focus?.();
    } else {
      richRef.value?.focus?.();
    }
  });
};

const blur = () => {
  if (modeRef.value === 'plain') {
    plainRef.value?.blur?.();
  } else {
    richRef.value?.blur?.();
  }
};

const getTextarea = () => {
  if (modeRef.value === 'plain') {
    return plainRef.value?.getTextarea?.();
  }
  return undefined;
};

defineExpose({
  focus,
  blur,
  getTextarea,
  getMode: () => modeRef.value,
  switchMode,
});
</script>

<template>
  <ChatInputPlain
    v-if="currentComponent === 'plain'"
    ref="plainRef"
    v-model="valueBinder"
    :placeholder="placeholder"
    :disabled="disabled"
    :whisper-mode="whisperMode"
    :mention-options="mentionOptions"
    :mention-loading="mentionLoading"
    :mention-prefix="mentionPrefix"
    :mention-render-label="mentionRenderLabel"
    :autosize="autosize"
    :rows="rows"
    :input-class="inputClass"
    @mention-search="handleSearch"
    @mention-select="handleSelect"
    @keydown="handleKeydown"
    @focus="handleFocus"
    @blur="handleBlur"
  />
  <div
    v-else
    class="rounded border border-gray-300 py-2 px-3 text-sm text-gray-500 bg-gray-50"
  >
    富文本模式即将上线，请稍后重试。
  </div>
</template>
