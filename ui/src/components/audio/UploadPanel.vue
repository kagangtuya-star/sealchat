<template>
  <div class="upload-panel" v-if="audio.canManage">
    <header>
      <h4>上传音频</h4>
      <p>支持 OGG/MP3/WAV（建议 OGG/Opus 以降低带宽）</p>
    </header>

    <label class="upload-panel__drop" @dragover.prevent @drop.prevent="handleDrop">
      <input type="file" multiple accept="audio/*" @change="handleChange" />
      <span>拖拽文件或点击选择</span>
    </label>

    <div class="upload-panel__tasks" v-if="audio.uploadTasks.length">
      <div v-for="task in audio.uploadTasks" :key="task.id" class="upload-task">
        <div class="upload-task__info">
          <strong>{{ task.filename }}</strong>
          <small>{{ task.status }}</small>
        </div>
        <n-progress :percentage="task.progress" :status="task.status === 'error' ? 'error' : 'success'" />
        <p v-if="task.error" class="upload-task__error">{{ task.error }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useAudioStudioStore } from '@/stores/audioStudio';

const audio = useAudioStudioStore();

function handleChange(event: Event) {
  const target = event.target as HTMLInputElement;
  if (target.files) {
    audio.handleUpload(target.files);
    target.value = '';
  }
}

function handleDrop(event: DragEvent) {
  if (event.dataTransfer?.files?.length) {
    audio.handleUpload(event.dataTransfer.files);
  }
}
</script>

<style scoped lang="scss">
.upload-panel {
  border: 1px dashed rgba(226, 232, 240, 0.3);
  border-radius: 12px;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.upload-panel__drop {
  height: 120px;
  border: 1px dashed rgba(99, 179, 237, 0.6);
  border-radius: 12px;
  display: flex;
  justify-content: center;
  align-items: center;
  color: var(--sc-text-secondary);
  cursor: pointer;
}

.upload-panel__drop input {
  display: none;
}

.upload-task {
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  padding-top: 0.5rem;
}

.upload-task__info {
  display: flex;
  justify-content: space-between;
  font-size: 0.85rem;
}

.upload-task__error {
  color: #feb2b2;
  font-size: 0.75rem;
}
</style>
