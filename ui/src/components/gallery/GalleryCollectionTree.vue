<template>
  <div class="gallery-collection-tree">
    <div class="gallery-collection-tree__header">
      <slot name="header">分类</slot>
    </div>
    <div class="gallery-collection-tree__list">
      <div
        v-for="collection in collections"
        :key="collection.id"
        :class="[
          'gallery-collection-tree__item-wrapper',
          { 'gallery-collection-tree__item-wrapper--dragover': dragOverId === collection.id }
        ]"
        @dragover="handleDragOver($event, collection.id)"
        @dragleave="handleDragLeave"
        @drop="handleDrop($event, collection.id)"
      >
        <n-button
          text
          block
          class="gallery-collection-tree__item"
          :type="collection.id === activeId ? 'primary' : 'default'"
          @click="$emit('select', collection.id)"
        >
          <span class="gallery-collection-tree__name">{{ collection.name }}</span>
          <span class="gallery-collection-tree__meta" v-if="collection.quotaUsed">
            {{ formatSize(collection.quotaUsed) }}
          </span>
        </n-button>
        <n-dropdown
          v-if="collection.id === activeId"
          trigger="click"
          :options="contextMenuOptions"
          @select="(key) => $emit('context-action', key, collection)"
        >
          <n-button text size="small" class="gallery-collection-tree__menu">
            <template #icon>
              <n-icon :component="EllipsisVertical" />
            </template>
          </n-button>
        </n-dropdown>
      </div>
    </div>
    <div class="gallery-collection-tree__actions">
      <slot name="actions"></slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { NButton, NDropdown, NIcon } from 'naive-ui';
import { EllipsisVertical } from '@vicons/ionicons5';
import type { GalleryCollection } from '@/types';

const props = defineProps<{ collections: GalleryCollection[]; activeId: string | null }>();
const emit = defineEmits<{
  (e: 'select', id: string): void;
  (e: 'context-action', key: string, collection: GalleryCollection): void;
  (e: 'drop-items', targetCollectionId: string, itemIds: string[]): void;
}>();

const dragOverId = ref<string | null>(null);

const contextMenuOptions = [
  { label: '重命名', key: 'rename' },
  { label: '删除', key: 'delete' }
];

function formatSize(size: number) {
  if (!size) return '';
  if (size < 1024) return `${size}B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)}KB`;
  return `${(size / (1024 * 1024)).toFixed(1)}MB`;
}

function handleDragOver(evt: DragEvent, collectionId: string) {
  const dt = evt.dataTransfer;
  if (!dt) return;
  
  // Only accept gallery item drops
  if (Array.from(dt.types || []).includes('application/x-sealchat-gallery-item')) {
    evt.preventDefault();
    dt.dropEffect = 'move';
    dragOverId.value = collectionId;
  }
}

function handleDragLeave() {
  dragOverId.value = null;
}

function handleDrop(evt: DragEvent, targetCollectionId: string) {
  evt.preventDefault();
  dragOverId.value = null;
  
  const dt = evt.dataTransfer;
  if (!dt) return;
  
  const data = dt.getData('application/x-sealchat-gallery-item');
  if (!data) return;
  
  try {
    const parsed = JSON.parse(data);
    const itemIds = parsed.selectedIds || [parsed.itemId];
    if (itemIds.length > 0 && targetCollectionId !== props.activeId) {
      emit('drop-items', targetCollectionId, itemIds);
    }
  } catch (error) {
    console.warn('解析拖拽数据失败', error);
  }
}
</script>

<style scoped>
.gallery-collection-tree {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.gallery-collection-tree__header {
  font-weight: 500;
  color: var(--sc-text-primary, var(--text-color-1));
}

.gallery-collection-tree__item-wrapper {
  display: flex;
  align-items: center;
  gap: 4px;
  padding-right: 4px;
  border-radius: 6px;
  border: 2px solid transparent;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.gallery-collection-tree__item-wrapper--dragover {
  border-color: var(--sc-primary, var(--primary-color));
  background-color: var(--sc-selected-bg, rgba(99, 102, 241, 0.1));
}

.gallery-collection-tree__item-wrapper .gallery-collection-tree__item {
  flex: 1;
  min-width: 0;
}

.gallery-collection-tree__menu {
  flex-shrink: 0;
  opacity: 0.7;
  color: var(--sc-text-secondary, var(--text-color-2));
  transition: opacity 0.15s ease, color 0.15s ease;
}

.gallery-collection-tree__menu:hover {
  opacity: 1;
  color: var(--sc-primary, var(--primary-color));
}

.gallery-collection-tree__list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 320px;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;
  margin-right: -4px;
}

/* Custom minimal scrollbar */
.gallery-collection-tree__list::-webkit-scrollbar {
  width: 4px;
}

.gallery-collection-tree__list::-webkit-scrollbar-track {
  background: transparent;
}

.gallery-collection-tree__list::-webkit-scrollbar-thumb {
  background: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.4));
  border-radius: 2px;
}

.gallery-collection-tree__list::-webkit-scrollbar-thumb:hover {
  background: var(--sc-scrollbar-thumb-hover, rgba(148, 163, 184, 0.6));
}

/* Firefox scrollbar */
.gallery-collection-tree__list {
  scrollbar-width: thin;
  scrollbar-color: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.4)) transparent;
}

.gallery-collection-tree__item {
  justify-content: space-between;
  text-align: left;
  border-radius: 6px;
  transition: background-color 0.15s ease;
}

.gallery-collection-tree__name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gallery-collection-tree__meta {
  font-size: 12px;
  color: var(--sc-text-tertiary, var(--text-color-3));
  margin-left: 8px;
  flex-shrink: 0;
}

.gallery-collection-tree__actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* Mobile adaptation */
@media (max-width: 768px) {
  .gallery-collection-tree__list {
    max-height: 200px;
  }
}
</style>
