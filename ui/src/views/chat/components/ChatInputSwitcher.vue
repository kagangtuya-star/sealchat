<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue';
import type { MentionOption } from 'naive-ui';
import ChatInputHybrid from './inputs/ChatInputHybrid.vue';
import ChatInputRich from './inputs/ChatInputRich.vue';

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
  inlineImages?: Record<string, { status: 'uploading' | 'uploaded' | 'failed'; previewUrl?: string; error?: string }>
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
  inlineImages: () => ({}),
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
  (event: 'paste-image', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-files', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'upload-button-click'): void
  (event: 'remove-image', markerId: string): void
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

const handlePasteImage = (payload: { files: File[]; selectionStart: number; selectionEnd: number }) => {
  emit('paste-image', payload);
};

const handleDropFiles = (payload: { files: File[]; selectionStart: number; selectionEnd: number }) => {
  emit('drop-files', payload);
};

const handleUploadButtonClick = () => {
  emit('upload-button-click');
};

const handleRemoveImage = (markerId: string) => {
  emit('remove-image', markerId);
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

const getEditor = () => {
  if (modeRef.value === 'rich') {
    return richRef.value?.getEditor?.();
  }
  return undefined;
};

defineExpose({
  focus,
  blur,
  getTextarea,
  getEditor,
  getMode: () => modeRef.value,
  switchMode,
});
</script>

<template>
  <ChatInputHybrid
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
    :inline-images="inlineImages"
    @mention-search="handleSearch"
    @mention-select="handleSelect"
    @keydown="handleKeydown"
    @focus="handleFocus"
    @blur="handleBlur"
    @remove-image="handleRemoveImage"
    @paste-image="handlePasteImage"
    @drop-files="handleDropFiles"
  />
  <ChatInputRich
    v-else
    ref="richRef"
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
    :inline-images="inlineImages"
    @mention-search="handleSearch"
    @mention-select="handleSelect"
    @keydown="handleKeydown"
    @focus="handleFocus"
    @blur="handleBlur"
    @paste-image="handlePasteImage"
    @drop-files="handleDropFiles"
    @upload-button-click="handleUploadButtonClick"
  />
</template>
