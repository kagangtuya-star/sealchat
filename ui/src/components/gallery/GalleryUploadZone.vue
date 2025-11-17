<template>
  <n-upload
    v-model:file-list="fileList"
    :show-file-list="false"
    multiple
    :max="12"
    accept="image/*"
    :disabled="disabled"
    @change="handleChange"
  >
    <n-upload-dragger>
      <div class="gallery-upload-zone">
        <slot>拖拽图片到此处或点击上传</slot>
      </div>
    </n-upload-dragger>
  </n-upload>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue';
import type { UploadFileInfo } from 'naive-ui';
import { NUpload, NUploadDragger } from 'naive-ui';

defineProps<{ disabled?: boolean }>();
const emit = defineEmits<{ (e: 'select', files: UploadFileInfo[]): void }>();

const fileList = ref<UploadFileInfo[]>([]);
const processedFileIds = new Set<string>();

function handleChange(options: { fileList: UploadFileInfo[] }) {
  const latestFiles = options.fileList.filter((item) => {
    if (!item.id) return false;
    return !processedFileIds.has(item.id);
  });
  if (!latestFiles.length) {
    return;
  }
  latestFiles.forEach((item) => {
    if (item.id) {
      processedFileIds.add(item.id);
    }
  });
  emit('select', latestFiles);
  // 上传处理完成后清空上传列表，避免后续事件重复触发
  nextTick(() => {
    fileList.value = [];
    processedFileIds.clear();
  });
}
</script>

<style scoped>
.gallery-upload-zone {
  padding: 24px 16px;
  text-align: center;
  color: var(--sc-text-secondary, var(--text-color-3));
  background-color: var(--sc-bg-input, #f9fafb);
  border: 1px dashed var(--sc-border-strong, rgba(148, 163, 184, 0.6));
  border-radius: 0.75rem;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

.gallery-upload-zone:hover {
  background-color: var(--sc-chip-bg, rgba(15, 23, 42, 0.08));
}
</style>
