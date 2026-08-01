<script setup lang="ts">
import { computed, ref } from 'vue';
import { useMessage } from 'naive-ui';
import ChatInputRich from '@/views/chat/components/inputs/ChatInputRich.vue';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import type { MentionOption } from 'naive-ui';

type RichTextEditorVariant = 'default' | 'keyword' | 'announcement' | 'sticky-note';
type ImageUploadMode = 'self' | 'delegate' | 'none';
type RichImagePayload = { files: File[]; selectionStart: number; selectionEnd: number };
type SmartLinkUploadSource = 'rich-editor' | 'smart-link-text-image' | 'smart-link-url-image';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  maxlength?: number
  variant?: RichTextEditorVariant
  imageUploadMode?: ImageUploadMode
  mentionOptions?: MentionOption[]
  mentionLoading?: boolean
  mentionPrefix?: (string | number)[]
  mentionRenderLabel?: (option: MentionOption) => any
  inputClass?: string | Record<string, boolean> | Array<string | Record<string, boolean>>
  editorStyle?: Record<string, string>
  minHeight?: string
  defaultIFormEmbedLink?: string
  performancePopoverPlacement?: 'bottom' | 'left-start' | 'right-start'
  performancePopoverContentClass?: string
}>(), {
  modelValue: '',
  placeholder: '',
  maxlength: 2000,
  variant: 'default',
  imageUploadMode: 'self',
  mentionOptions: () => [],
  mentionLoading: false,
  mentionPrefix: () => ['@'],
  inputClass: () => [],
  editorStyle: () => ({}),
  minHeight: '',
  defaultIFormEmbedLink: '',
  performancePopoverPlacement: 'bottom',
  performancePopoverContentClass: '',
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'focus'): void
  (event: 'blur'): void
  (event: 'paste-image', payload: RichImagePayload): void
  (event: 'drop-files', payload: RichImagePayload): void
  (event: 'upload-button-click', source?: 'rich-editor' | SmartLinkUploadSource): void
}>();

const message = useMessage();
const richEditorRef = ref<InstanceType<typeof ChatInputRich> | null>(null);
const fileInputRef = ref<HTMLInputElement | null>(null);
const isUploading = ref(false);

const classList = computed(() => [
  'rich-text-editor',
  `rich-text-editor--${props.variant}`,
]);

const chatInputClass = computed(() => [
  'rich-text-editor__input',
  `rich-text-editor__input--${props.variant}`,
  ...(Array.isArray(props.inputClass) ? props.inputClass : [props.inputClass]),
].filter(Boolean) as Array<string | Record<string, boolean>>);

const rootStyle = computed(() => ({
  ...props.editorStyle,
  ...(props.minHeight ? { '--rich-text-editor-min-height': props.minHeight } : {}),
}));

const updateValue = (...args: unknown[]) => {
  const value = typeof args[0] === 'string' ? args[0] : '';
  emit('update:modelValue', value);
};

const insertImageFiles = async (files: File[]) => {
  if (props.imageUploadMode !== 'self' || isUploading.value) return;
  const imageFiles = files.filter((file) => file.type.startsWith('image/'));
  if (!imageFiles.length) {
    message.warning('当前仅支持插入图片文件');
    return;
  }

  const editor = richEditorRef.value?.getEditor?.();
  if (!editor) return;

  isUploading.value = true;
  try {
    for (const file of imageFiles) {
      const result = await uploadImageAttachment(file);
      if (!result.attachmentId) continue;
      const attachmentId = result.attachmentId.replace(/^id:/, '');
      const imageUrl = `/api/v1/attachment/${attachmentId}`;
      editor.chain().focus().setImage({ src: imageUrl, alt: file.name || '' }).run();
    }
  } catch (error: any) {
    message.error(error?.message || '图片上传失败');
  } finally {
    isUploading.value = false;
  }
};

const handleImagePayload = (eventName: 'paste-image' | 'drop-files', payload: RichImagePayload) => {
  if (props.imageUploadMode === 'delegate') {
    if (eventName === 'paste-image') {
      emit('paste-image', payload);
    } else {
      emit('drop-files', payload);
    }
    return;
  }
  if (props.imageUploadMode === 'self') {
    void insertImageFiles(payload.files);
  }
};

const triggerFileSelect = (source: 'rich-editor' | SmartLinkUploadSource = 'rich-editor') => {
  if (props.imageUploadMode === 'delegate') {
    emit('upload-button-click', source);
    return;
  }
  if (props.imageUploadMode === 'self') {
    fileInputRef.value?.click();
  }
};

const handleUploadButtonClick = (...args: unknown[]) => {
  const source = typeof args[0] === 'string' ? args[0] as 'rich-editor' | SmartLinkUploadSource : 'rich-editor';
  triggerFileSelect(source);
};

const handleFileSelect = (event: Event) => {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  if (files.length) {
    void insertImageFiles(files);
  }
  input.value = '';
};

const focus = () => richEditorRef.value?.focus();
const blur = () => richEditorRef.value?.blur();

defineExpose({
  focus,
  blur,
  getTextarea: () => richEditorRef.value?.getTextarea?.(),
  getSelectionRange: () => richEditorRef.value?.getSelectionRange?.(),
  setSelectionRange: (start: number, end: number) => richEditorRef.value?.setSelectionRange?.(start, end),
  moveCursorToEnd: () => richEditorRef.value?.moveCursorToEnd?.(),
  getInstance: () => richEditorRef.value?.getInstance?.(),
  getEditor: () => richEditorRef.value?.getEditor?.(),
  getJson: () => richEditorRef.value?.getJson?.(),
  insertImagePlaceholder: (markerId: string, previewUrl: string) =>
    richEditorRef.value?.insertImagePlaceholder?.(markerId, previewUrl),
  applySmartLinkImage: (source: SmartLinkUploadSource, image: { url: string; label?: string }) =>
    (richEditorRef.value?.applySmartLinkImage as any)?.(source, image),
  triggerFileSelect,
  hasOpenOverlay: () => richEditorRef.value?.hasOpenOverlay?.() ?? false,
  hasRecentOverlayInteraction: (thresholdMs?: number) =>
    richEditorRef.value?.hasRecentOverlayInteraction?.(thresholdMs) ?? false,
});
</script>

<template>
  <div :class="classList" :style="rootStyle">
    <input
      ref="fileInputRef"
      type="file"
      accept="image/*"
      multiple
      class="rich-text-editor__file-input"
      @change="handleFileSelect"
    />
    <ChatInputRich
      ref="richEditorRef"
      :model-value="props.modelValue"
      :placeholder="props.placeholder"
      :surface-variant="props.variant === 'sticky-note' ? 'sticky-note' : 'default'"
      :mention-options="props.mentionOptions"
      :mention-loading="props.mentionLoading"
      :mention-prefix="props.mentionPrefix"
      :mention-render-label="props.mentionRenderLabel"
      :input-class="chatInputClass"
      :default-i-form-embed-link="props.defaultIFormEmbedLink"
      :performance-popover-placement="props.performancePopoverPlacement"
      :performance-popover-content-class="props.performancePopoverContentClass"
      @update:model-value="updateValue"
      @focus="emit('focus')"
      @blur="emit('blur')"
      @paste-image="handleImagePayload('paste-image', $event)"
      @drop-files="handleImagePayload('drop-files', $event)"
      @upload-button-click="handleUploadButtonClick"
    />
  </div>
</template>

<style scoped>
.rich-text-editor {
  width: 100%;
  --rich-text-editor-min-height: 220px;
}

.rich-text-editor__file-input {
  display: none;
}

:deep(.rich-text-editor__input--keyword),
:deep(.rich-text-editor__input--announcement) {
  min-height: var(--rich-text-editor-min-height);
}

:deep(.rich-text-editor__input--keyword .tiptap-wrapper),
:deep(.rich-text-editor__input--announcement .tiptap-wrapper) {
  min-height: var(--rich-text-editor-min-height);
}

.rich-text-editor--sticky-note {
  height: 100%;
  min-height: 0;
}
</style>
