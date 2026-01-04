<script setup lang="ts">
import type { StickyNoteType } from '@/stores/stickyNote'

const emit = defineEmits<{
  select: [type: StickyNoteType]
}>()

const noteTypes: { type: StickyNoteType; label: string; icon: string }[] = [
  { type: 'text', label: '文本', icon: '📝' },
  { type: 'counter', label: '计数', icon: '🔢' },
  { type: 'list', label: '列表', icon: '☑️' },
  { type: 'slider', label: '滑块', icon: '📊' },
  { type: 'timer', label: '计时', icon: '⏱️' },
  { type: 'clock', label: '进度', icon: '🕐' },
  { type: 'roundCounter', label: '回合', icon: '🔄' }
]

function selectType(type: StickyNoteType) {
  emit('select', type)
}
</script>

<template>
  <div class="sticky-note-type-selector">
    <button
      v-for="item in noteTypes"
      :key="item.type"
      class="sticky-note-type-selector__item"
      :title="item.label"
      @click="selectType(item.type)"
    >
      <span class="sticky-note-type-selector__icon">{{ item.icon }}</span>
      <span class="sticky-note-type-selector__label">{{ item.label }}</span>
    </button>
  </div>
</template>

<style scoped>
.sticky-note-type-selector {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 4px;
  padding: 8px;
  width: 220px;
}

.sticky-note-type-selector__item {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 6px 4px;
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.1));
  border-radius: 6px;
  background: var(--sc-bg-elevated, rgba(255, 255, 255, 0.9));
  cursor: pointer;
  transition: all 0.15s;
}

.sticky-note-type-selector__item:hover {
  background: var(--sc-bg-hover, rgba(0, 0, 0, 0.05));
  border-color: var(--sc-border-strong, rgba(0, 0, 0, 0.2));
}

.sticky-note-type-selector__icon {
  font-size: 16px;
  line-height: 1;
}

.sticky-note-type-selector__label {
  font-size: 10px;
  color: var(--sc-text-secondary, rgba(0, 0, 0, 0.6));
  margin-top: 2px;
  white-space: nowrap;
}
</style>

<style>
/* 夜间模式适配 */
:root[data-display-palette='night'] .sticky-note-type-selector__item {
  background: var(--sc-bg-elevated, #2a2a2e);
  border-color: var(--sc-border-mute, rgba(255, 255, 255, 0.1));
}

:root[data-display-palette='night'] .sticky-note-type-selector__item:hover {
  background: var(--sc-bg-hover, rgba(255, 255, 255, 0.08));
  border-color: var(--sc-border-strong, rgba(255, 255, 255, 0.2));
}

:root[data-display-palette='night'] .sticky-note-type-selector__label {
  color: var(--sc-text-secondary, rgba(255, 255, 255, 0.6));
}

/* 自定义主题适配 */
:root[data-custom-theme='true'] .sticky-note-type-selector__item {
  background: var(--sc-bg-elevated, #ffffff);
  border-color: var(--sc-border-mute, rgba(0, 0, 0, 0.1));
}

:root[data-custom-theme='true'] .sticky-note-type-selector__item:hover {
  background: var(--sc-bg-hover, rgba(0, 0, 0, 0.05));
}

:root[data-custom-theme='true'] .sticky-note-type-selector__label {
  color: var(--sc-text-secondary, rgba(0, 0, 0, 0.6));
}
</style>
