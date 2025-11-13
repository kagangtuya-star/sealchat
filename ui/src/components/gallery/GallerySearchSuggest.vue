<template>
  <transition name="fade">
    <div v-if="visible" class="gallery-search-suggest">
      <div v-if="!items.length" class="gallery-search-suggest__empty">没有匹配的图片</div>
      <div v-else>
        <div
          v-for="(item, index) in items"
          :key="item.id"
          class="gallery-search-suggest__item"
          :class="{ 'is-active': index === activeIndex }"
          @mousedown.prevent="select(item)"
        >
          <img :src="item.thumbUrl || buildAttachmentUrl(item.attachmentId)" alt="" />
          <div class="gallery-search-suggest__meta">
            <div class="gallery-search-suggest__remark">/{{ item.remark }}</div>
            <div class="gallery-search-suggest__collection">{{ resolveCollectionName(item.collectionId) }}</div>
          </div>
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import type { GalleryCollection, GalleryItem } from '@/types';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

const props = defineProps<{
  items: GalleryItem[];
  collections: Record<string, GalleryCollection>;
  visible: boolean;
  activeIndex: number;
}>();
const emit = defineEmits<{ (e: 'select', item: GalleryItem): void }>();

function buildAttachmentUrl(attachmentId: string) {
  return resolveAttachmentUrl(attachmentId);
}

const resolveCollectionName = (collectionId: string) => props.collections[collectionId]?.name ?? '';

function select(item: GalleryItem) {
  emit('select', item);
}
</script>

<style scoped>
.gallery-search-suggest {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 100%;
  margin-bottom: 8px;
  background: var(--card-color);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
  overflow: hidden;
  max-height: 280px;
  display: flex;
  flex-direction: column;
}

.gallery-search-suggest__item {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 8px 12px;
  cursor: pointer;
}

.gallery-search-suggest__item.is-active {
  background: rgba(255, 255, 255, 0.08);
}

.gallery-search-suggest__item img {
  width: 40px;
  height: 40px;
  object-fit: contain;
  border-radius: 4px;
}

.gallery-search-suggest__remark {
  font-weight: 500;
}

.gallery-search-suggest__collection {
  font-size: 12px;
  color: var(--text-color-3);
}

.gallery-search-suggest__empty {
  padding: 16px;
  text-align: center;
  color: var(--text-color-3);
}
</style>
