<template>
  <div class="gallery-grid">
    <div class="gallery-grid__toolbar">
      <slot name="toolbar"></slot>
    </div>
    <div v-if="loading" class="gallery-grid__placeholder">加载中...</div>
    <div v-else-if="!items.length" class="gallery-grid__placeholder">暂无图片资源</div>
    <div v-else class="gallery-grid__content">
      <div
        v-for="(item, index) in items"
        :key="item.id"
        :class="[
          'gallery-grid__item',
          { 'gallery-grid__item--selected': isSelected(item.id) },
          { 'gallery-grid__item--dragover': dragOverIndex === index }
        ]"
        draggable="true"
        @click="handleClick(item, index, $event)"
        @dblclick="handleDoubleClick(item)"
        @dragstart="handleDragStart(item, index, $event)"
        @dragover="handleDragOver(index, $event)"
        @dragleave="handleDragLeave"
        @drop="handleDrop(index, $event)"
      >
        <div v-if="selectable" class="gallery-grid__checkbox" @click.stop>
          <n-checkbox
            :checked="isSelected(item.id)"
            @update:checked="(checked) => handleCheckboxChange(item, checked)"
          />
        </div>
        <n-image
          :src="item.thumbUrl || buildAttachmentUrl(item.attachmentId)"
          :preview-src="buildAttachmentUrl(item.attachmentId)"
          object-fit="contain"
          preview-disabled
        />
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
import { computed, ref } from 'vue';
import { NButton, NImage, NCheckbox } from 'naive-ui';
import type { GalleryItem } from '@/types';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

const props = defineProps<{
  items: GalleryItem[];
  loading?: boolean;
  editable?: boolean;
  selectable?: boolean;
  selectedIds?: string[];
}>();

const emit = defineEmits<{
  (e: 'select', item: GalleryItem): void;
  (e: 'toggle-select', item: GalleryItem, selected: boolean): void;
  (e: 'range-select', startIndex: number, endIndex: number): void;
  (e: 'insert', item: GalleryItem): void;
  (e: 'drag-start', item: GalleryItem, evt: DragEvent): void;
  (e: 'reorder', fromIndex: number, toIndex: number): void;
  (e: 'edit', item: GalleryItem): void;
  (e: 'delete', item: GalleryItem): void;
}>();

const selectedSet = computed(() => new Set(props.selectedIds || []));
const dragOverIndex = ref<number | null>(null);
let lastClickIndex = -1;
let draggingIndex = -1;

function isSelected(id: string): boolean {
  return selectedSet.value.has(id);
}

function buildAttachmentUrl(attachmentId: string) {
  return resolveAttachmentUrl(attachmentId);
}

function handleClick(item: GalleryItem, index: number, evt: MouseEvent) {
  if (!props.selectable) {
    emit('select', item);
    return;
  }

  if (evt.shiftKey && lastClickIndex >= 0) {
    // Shift+click: range select
    emit('range-select', lastClickIndex, index);
  } else if (evt.ctrlKey || evt.metaKey) {
    // Ctrl/Cmd+click: toggle selection
    emit('toggle-select', item, !isSelected(item.id));
  } else {
    // Regular click: toggle selection
    emit('toggle-select', item, !isSelected(item.id));
  }
  lastClickIndex = index;
}

function handleDoubleClick(item: GalleryItem) {
  emit('insert', item);
}

function handleCheckboxChange(item: GalleryItem, checked: boolean) {
  emit('toggle-select', item, checked);
}

function handleDragStart(item: GalleryItem, index: number, evt: DragEvent) {
  draggingIndex = index;
  const dt = evt.dataTransfer;
  if (dt) {
    dt.effectAllowed = 'copyMove';
    try {
      const dragData = {
        itemId: item.id,
        attachmentId: item.attachmentId,
        fromIndex: index,
        selectedIds: props.selectable && isSelected(item.id)
          ? Array.from(selectedSet.value)
          : [item.id]
      };
      dt.setData('application/x-sealchat-gallery-item', JSON.stringify(dragData));
    } catch (error) {
      console.warn('设置画廊拖拽数据失败', error);
    }
    dt.setData('text/plain', item.attachmentId);
  }
  emit('drag-start', item, evt);
}

function handleDragOver(index: number, evt: DragEvent) {
  evt.preventDefault();
  if (index !== draggingIndex) {
    dragOverIndex.value = index;
  }
}

function handleDragLeave() {
  dragOverIndex.value = null;
}

function handleDrop(toIndex: number, evt: DragEvent) {
  evt.preventDefault();
  dragOverIndex.value = null;
  
  if (draggingIndex >= 0 && draggingIndex !== toIndex) {
    emit('reorder', draggingIndex, toIndex);
  }
  draggingIndex = -1;
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

/* Custom minimal scrollbar */
.gallery-grid__content::-webkit-scrollbar {
  width: 4px;
}

.gallery-grid__content::-webkit-scrollbar-track {
  background: transparent;
}

.gallery-grid__content::-webkit-scrollbar-thumb {
  background: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.4));
  border-radius: 2px;
}

.gallery-grid__content::-webkit-scrollbar-thumb:hover {
  background: var(--sc-scrollbar-thumb-hover, rgba(148, 163, 184, 0.6));
}

.gallery-grid__content {
  scrollbar-width: thin;
  scrollbar-color: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.4)) transparent;
}

.gallery-grid__item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: pointer;
  position: relative;
  border-radius: 8px;
  padding: 8px;
  border: 2px solid transparent;
  transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.gallery-grid__item:hover {
  background-color: var(--sc-hover-bg, var(--hover-color));
}

.gallery-grid__item--selected {
  border-color: var(--sc-primary, var(--primary-color));
  background-color: var(--sc-selected-bg, rgba(99, 102, 241, 0.1));
  box-shadow: 0 0 0 1px var(--sc-primary, var(--primary-color)) inset;
}

.gallery-grid__item--dragover {
  border-color: var(--sc-success, #10b981);
  background-color: rgba(16, 185, 129, 0.1);
}

.gallery-grid__checkbox {
  position: absolute;
  top: 4px;
  left: 4px;
  z-index: 2;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.gallery-grid__item:hover .gallery-grid__checkbox,
.gallery-grid__item--selected .gallery-grid__checkbox {
  opacity: 1;
}

.gallery-grid__caption {
  font-size: 12px;
  text-align: center;
  color: var(--sc-text-secondary, var(--text-color-2));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.gallery-grid__placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sc-text-tertiary, var(--text-color-3));
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

  .gallery-grid__checkbox {
    opacity: 1;
  }

  .gallery-grid__caption {
    font-size: 11px;
  }
}
</style>

