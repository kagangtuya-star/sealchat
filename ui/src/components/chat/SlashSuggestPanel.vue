<template>
  <transition name="fade">
    <div v-if="visible" class="slash-suggest" @mousedown.prevent>
      <div class="slash-suggest__content sc-overlay" data-sc-ui="image-popup">
        <div v-if="options.length" class="slash-suggest__grid">
          <div
            v-for="(option, index) in options"
            :key="option.id"
            class="slash-suggest__item"
            :class="{ 'is-active': index === activeIndex }"
            draggable="true"
            @mousedown.prevent="$emit('select', option)"
            @mouseenter="$emit('hover', index)"
            @dragstart="$emit('drag', option, $event)"
          >
            <img :src="option.thumbUrl" alt="预览" />
            <div class="slash-suggest__caption" :title="option.remark">{{ option.remark }}</div>
          </div>
        </div>
        <div v-else class="slash-suggest__empty">
          {{ loading ? '搜索中…' : '没有匹配的图片' }}
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
interface SlashOption {
  id: string;
  attachmentId: string;
  remark: string;
  thumbUrl: string;
  source: 'user' | 'gallery';
}

defineProps<{
  visible: boolean;
  options: SlashOption[];
  activeIndex: number;
  loading?: boolean;
}>();

defineEmits<{
  (e: 'select', option: SlashOption): void;
  (e: 'hover', index: number): void;
  (e: 'drag', option: SlashOption, evt: DragEvent): void;
}>();
</script>

<style scoped>
.slash-suggest {
  position: absolute;
  bottom: calc(100% + 12px);
  left: 0.5rem;
  width: 340px;
  max-height: 360px;
  z-index: 90;
  pointer-events: auto;
}

.slash-suggest__content {
  background: var(--sc-bg-elevated);
  color: var(--sc-text-primary);
  font-family: var(--sc-font-family);
  border-radius: 14px;
  box-shadow: 0 16px 32px rgba(15, 23, 42, 0.18);
  border: 1px solid var(--sc-border-mute);
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  overflow: hidden;
}

.slash-suggest__grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(72px, 1fr));
  gap: 0.75rem;
}

.slash-suggest__item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  padding: 0.35rem;
  border-radius: 10px;
  transition: background-color 0.15s ease;
  cursor: pointer;
}

.slash-suggest__item img {
  width: 4.6rem;
  height: 4.6rem;
  object-fit: contain;
  border-radius: 8px;
}

.slash-suggest__caption {
  font-size: 12px;
  text-align: center;
  color: var(--sc-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.slash-suggest__item:hover,
.slash-suggest__item.is-active {
  background: var(--sc-bg-hover);
}

.slash-suggest__empty {
  text-align: center;
  font-size: 13px;
  color: var(--sc-text-secondary);
  padding: 1rem 0;
}
</style>
