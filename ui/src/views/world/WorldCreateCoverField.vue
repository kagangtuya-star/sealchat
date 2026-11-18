<script setup lang="ts">
import { defineAsyncComponent, onBeforeUnmount, ref } from 'vue';
import { useMessage } from 'naive-ui';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import type { MessageImageEditorResult } from '@/composables/useMessageImageEditor';
import WorldCoverMedia from './WorldCoverMedia.vue';

const props = defineProps<{ disabled?: boolean; worldId?: string }>();
const coverAttachmentId = defineModel<string>({ default: '' });
const uploading = defineModel<boolean>('uploading', { default: false });
const editing = defineModel<boolean>('editing', { default: false });
const MessageImageEditor = defineAsyncComponent(() => import('@/components/chat/MessageImageEditor.vue'));
const editorFile = ref<File | null>(null);
const message = useMessage();
const inputRef = ref<HTMLInputElement | null>(null);
const editableMimeTypes = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/avif']);
const originalMimeTypes = new Set(['image/gif', 'video/webm']);
const accept = [...editableMimeTypes, ...originalMimeTypes].join(',');
let active = true;

const closeEditor = () => {
  editing.value = false;
  editorFile.value = null;
};

onBeforeUnmount(() => {
  active = false;
  uploading.value = false;
  closeEditor();
});

const handleSelected = (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file || !active || props.disabled || uploading.value || editing.value) return;
  const mimeType = file.type.toLowerCase();
  if (originalMimeTypes.has(mimeType)) {
    void uploadCover(file);
    return;
  }
  if (!editableMimeTypes.has(mimeType)) {
    message.error('请选择 PNG / JPEG / WebP / AVIF / GIF 图片或 WebM 视频');
    return;
  }
  editorFile.value = file;
  editing.value = true;
};

const uploadCover = async (file: File) => {
  uploading.value = true;
  try {
    const result = await uploadImageAttachment(file, {
      channelId: null,
      rootId: props.worldId,
      rootIdType: 'world_cover',
      skipCompression: true,
    });
    if (active) coverAttachmentId.value = result.attachmentId;
  } catch (error) {
    if (active) message.error((error instanceof Error && error.message) || '封面上传失败');
  } finally {
    if (active) uploading.value = false;
  }
};

const handleEditorConfirm = ({ file }: MessageImageEditorResult) => {
  if (!active || !editorFile.value || props.disabled || uploading.value) return;
  closeEditor();
  void uploadCover(file);
};
</script>

<template>
  <div class="world-create-cover">
    <WorldCoverMedia class="world-create-cover__preview" :attachment="coverAttachmentId" alt="世界封面预览" />
    <div class="world-create-cover__actions">
      <n-button size="small" secondary :loading="uploading" :disabled="disabled || uploading || editing" @click="inputRef?.click()">
        {{ coverAttachmentId ? '更换封面' : '上传封面' }}
      </n-button>
      <n-button v-if="coverAttachmentId" size="small" quaternary :disabled="disabled || uploading || editing" @click="coverAttachmentId = ''">
        移除
      </n-button>
    </div>
    <div class="world-create-cover__hint"><slot name="hint">选填，建议使用横向图片（约 4:1）。</slot></div>
    <input ref="inputRef" type="file" :accept="accept" hidden :disabled="disabled || uploading || editing" @change="handleSelected" />
    <MessageImageEditor
      v-if="editing"
      :show="editing"
      :file="editorFile"
      @update:show="value => { if (!value) closeEditor(); }"
      @cancel="closeEditor"
      @confirm="handleEditorConfirm"
    />
  </div>
</template>

<style scoped>
.world-create-cover {
  width: 100%;
}

.world-create-cover__preview {
  aspect-ratio: 4 / 1;
  overflow: hidden;
  border: 1px solid var(--sc-border-mute);
  border-radius: 8px;
  margin-bottom: 8px;
}

.world-create-cover__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.world-create-cover__hint {
  margin-top: 8px;
  color: var(--sc-text-secondary);
  font-size: 12px;
}
</style>
