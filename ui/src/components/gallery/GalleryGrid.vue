<template>
  <div :class="['gallery-grid', sizeClass]">
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
          :src="resolveGalleryItemSrc(item)"
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
import { computed, reactive, ref, watch } from 'vue';
import { NButton, NImage, NCheckbox } from 'naive-ui';
import type { GalleryItem } from '@/types';
import { fetchAttachmentMetaById, normalizeAttachmentId, resolveAttachmentUrl, type AttachmentMeta } from '@/composables/useAttachmentResolver';

const props = defineProps<{
  items: GalleryItem[];
  loading?: boolean;
  editable?: boolean;
  selectable?: boolean;
  selectedIds?: string[];
  thumbnailSize?: 'small' | 'medium' | 'large' | 'xlarge';
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
const sizeClass = computed(() => `gallery-grid--${props.thumbnailSize ?? 'medium'}`);
const dragOverIndex = ref<number | null>(null);
let lastClickIndex = -1;
let draggingIndex = -1;

const attachmentMetaCache = reactive<Record<string, AttachmentMeta | null>>({});
const pendingMetaFetch = new Set<string>();

const ensureAttachmentMeta = async (attachmentId: string) => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized || pendingMetaFetch.has(normalized) || attachmentMetaCache[normalized] !== undefined) {
    return;
  }
  pendingMetaFetch.add(normalized);
  try {
    const meta = await fetchAttachmentMetaById(normalized);
    attachmentMetaCache[normalized] = meta;
  } finally {
    pendingMetaFetch.delete(normalized);
  }
};

function isSelected(id: string): boolean {
  return selectedSet.value.has(id);
}

function buildAttachmentUrl(attachmentId: string) {
  return resolveAttachmentUrl(attachmentId);
}

function resolveGalleryItemSrc(item: GalleryItem) {
  const normalized = normalizeAttachmentId(item.attachmentId);
  if (!normalized) {
    return '';
  }
  const meta = attachmentMetaCache[normalized];
  if (meta === undefined && !pendingMetaFetch.has(normalized)) {
    void ensureAttachmentMeta(normalized);
  }
  if (meta?.isAnimated) {
    return resolveAttachmentUrl(normalized);
  }
  return item.thumbUrl || resolveAttachmentUrl(normalized);
}

watch(
  () => props.items,
  (items) => {
    items.forEach((item) => {
      void ensureAttachmentMeta(item.attachmentId);
    });
  },
  { immediate: true },
);

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
  --grid-min-size: 96px;
  --grid-gap: 12px;
  --grid-item-padding: 8px;
  --grid-caption-size: 12px;
}

.gallery-grid--small {
  --grid-min-size: 72px;
  --grid-gap: 8px;
  --grid-item-padding: 6px;
  --grid-caption-size: 11px;
}

.gallery-grid--large {
  --grid-min-size: 128px;
  --grid-gap: 14px;
  --grid-item-padding: 10px;
  --grid-caption-size: 13px;
}

.gallery-grid--xlarge {
  --grid-min-size: 160px;
  --grid-gap: 16px;
  --grid-item-padding: 12px;
  --grid-caption-size: 14px;
}

.gallery-grid__content {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(var(--grid-min-size), 1fr));
  gap: var(--grid-gap);
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
  padding: var(--grid-item-padding);
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
  font-size: var(--grid-caption-size);
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
  .gallery-grid {
    --grid-min-size: 80px;
    --grid-gap: 8px;
    --grid-item-padding: 6px;
    --grid-caption-size: 11px;
  }

  .gallery-grid--small {
    --grid-min-size: 64px;
  }

  .gallery-grid--large {
    --grid-min-size: 96px;
    --grid-gap: 10px;
    --grid-item-padding: 8px;
    --grid-caption-size: 12px;
  }

  .gallery-grid--xlarge {
    --grid-min-size: 112px;
    --grid-gap: 12px;
    --grid-item-padding: 9px;
    --grid-caption-size: 12px;
  }

  .gallery-grid__actions {
    opacity: 1;
  }

  .gallery-grid__checkbox {
    opacity: 1;
  }

}
</style>
