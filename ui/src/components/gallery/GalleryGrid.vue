<template>
  <div class="gallery-grid">
    <div class="gallery-grid__toolbar">
      <slot name="toolbar"></slot>
    </div>
    <div v-if="loading" class="gallery-grid__placeholder">加载中...</div>
    <div v-else-if="!items.length" class="gallery-grid__placeholder">暂无图片资源</div>
    <div v-else class="gallery-grid__content">
      <div
        v-for="item in items"
        :key="item.id"
        class="gallery-grid__item"
        @click="$emit('select', item)"
        draggable="true"
        @dragstart="handleDragStart(item, $event)"
      >
        <n-image :src="item.thumbUrl || buildAttachmentUrl(item.attachmentId)" :preview-src="buildAttachmentUrl(item.attachmentId)" object-fit="contain" preview-disabled />
        <div class="gallery-grid__caption">{{ item.remark }}</div>
        <div v-if="editable" class="gallery-grid__actions">
          <n-button quaternary size="tiny" @click.stop="emit('edit', item)">备注</n-button>
          <n-button quaternary size="tiny" type="error" @click.stop="emit('delete', item)">删除</n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { NButton, NImage } from 'naive-ui';
import type { GalleryItem } from '@/types';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

defineProps<{ items: GalleryItem[]; loading?: boolean; editable?: boolean }>();
const emit = defineEmits<{
  (e: 'select', item: GalleryItem): void;
  (e: 'drag-start', item: GalleryItem, evt: DragEvent): void;
  (e: 'edit', item: GalleryItem): void;
  (e: 'delete', item: GalleryItem): void;
}>();

function buildAttachmentUrl(attachmentId: string) {
  return resolveAttachmentUrl(attachmentId);
}

function handleDragStart(item: GalleryItem, evt: DragEvent) {
  const dt = evt.dataTransfer;
  if (dt) {
    dt.effectAllowed = 'copy';
    try {
      dt.setData('application/x-sealchat-gallery-item', JSON.stringify({ attachmentId: item.attachmentId }));
    } catch (error) {
      console.warn('设置画廊拖拽数据失败', error);
    }
    dt.setData('text/plain', item.attachmentId);
  }
  emit('drag-start', item, evt);
}
</script>

<style scoped>
.gallery-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
}

.gallery-grid__content {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(96px, 1fr));
  gap: 12px;
  overflow-y: auto;
  padding-right: 4px;
}

.gallery-grid__item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: pointer;
  position: relative;
  border-radius: 8px;
  padding: 8px;
  transition: background-color 0.2s ease;
}

.gallery-grid__item:hover {
  background-color: var(--hover-color);
}

.gallery-grid__caption {
  font-size: 12px;
  text-align: center;
  color: var(--text-color-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.gallery-grid__placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-color-3);
  min-height: 160px;
}

.gallery-grid__actions {
  position: absolute;
  top: 4px;
  right: 4px;
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.2s ease;
}

.gallery-grid__item:hover .gallery-grid__actions {
  opacity: 1;
}

@media (max-width: 768px) {
  .gallery-grid__content {
    grid-template-columns: repeat(auto-fill, minmax(80px, 1fr));
    gap: 8px;
  }

  .gallery-grid__item {
    padding: 6px;
  }

  .gallery-grid__actions {
    opacity: 1;
  }

  .gallery-grid__caption {
    font-size: 11px;
  }
}
</style>
