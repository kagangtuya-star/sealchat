<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, ref } from 'vue';
import { useMessage } from 'naive-ui';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import type { MessageImageEditorResult } from '@/composables/useMessageImageEditor';

const props = defineProps<{ disabled?: boolean; worldId?: string }>();
const coverAttachmentId = defineModel<string>({ default: '' });
const uploading = defineModel<boolean>('uploading', { default: false });
const editing = defineModel<boolean>('editing', { default: false });
const MessageImageEditor = defineAsyncComponent(() => import('@/components/chat/MessageImageEditor.vue'));
const editorFile = ref<File | null>(null);
const message = useMessage();
const inputRef = ref<HTMLInputElement | null>(null);
const previewUrl = computed(() => resolveAttachmentUrl(coverAttachmentId.value));
const allowedMimeTypes = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/avif']);
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
  if (!file || props.disabled || uploading.value || editing.value) return;
  if (!allowedMimeTypes.has(file.type.toLowerCase())) {
    message.error('请选择 PNG / JPEG / WebP / GIF / AVIF 图片');
    return;
  }
  editorFile.value = file;
  editing.value = true;
};

const handleEditorConfirm = async ({ file }: MessageImageEditorResult) => {
  if (!active || !editorFile.value || props.disabled || uploading.value) return;
  closeEditor();
  uploading.value = true;
  try {
    const result = await uploadImageAttachment(file, {
      channelId: null,
      rootId: props.worldId,
      rootIdType: 'world_cover',
      skipCompression: true,
    });
    if (active) coverAttachmentId.value = result.attachmentId;
  } catch (error: any) {
    if (active) message.error(error?.message || '封面上传失败');
  } finally {
    if (active) uploading.value = false;
  }
};
</script>

<template>
  <div class="world-create-cover">
    <div v-if="previewUrl" class="world-create-cover__preview">
      <img :src="previewUrl" alt="世界封面预览" />
    </div>
    <div class="world-create-cover__actions">
      <n-button size="small" secondary :loading="uploading" :disabled="disabled || uploading || editing" @click="inputRef?.click()">
        {{ coverAttachmentId ? '更换封面' : '上传封面' }}
      </n-button>
      <n-button v-if="coverAttachmentId" size="small" quaternary :disabled="disabled || uploading || editing" @click="coverAttachmentId = ''">
        移除
      </n-button>
    </div>
    <div class="world-create-cover__hint"><slot name="hint">选填，建议使用横向图片（约 4:1）。</slot></div>
    <input ref="inputRef" type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/avif" hidden :disabled="disabled || uploading || editing" @change="handleSelected" />
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

.world-create-cover__preview img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
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
