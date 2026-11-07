<template>
  <transition name="keyword-suggest-fade">
    <div v-if="visible" class="keyword-suggest" @mousedown.prevent>
      <div class="keyword-suggest__content sc-overlay" data-sc-ui="candidate-popup">
        <div v-if="title" class="keyword-suggest__title">{{ title }}</div>
        <div v-if="options.length" ref="listRef" class="keyword-suggest__list" role="listbox">
          <div
            v-for="(option, index) in options"
            :key="option.id"
            class="keyword-suggest__item"
            :class="{ 'is-active': index === activeIndex }"
            role="option"
            :aria-selected="index === activeIndex"
            @mousedown.prevent="$emit('select', option)"
            @mouseenter="$emit('hover', index)"
          >
            <span class="keyword-suggest__name">{{ option.label }}</span>
            <span v-if="option.category" class="keyword-suggest__category">{{ option.category }}</span>
            <span v-if="option.description" class="keyword-suggest__desc">{{ truncateDesc(option.description) }}</span>
          </div>
        </div>
        <div v-else class="keyword-suggest__empty">
          {{ loading ? '搜索中...' : (emptyText || '无匹配术语') }}
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { ComposerSuggestOption } from '@/views/chat/composables/useComposerSuggest'

// 聊天输入框候选浮层：世界术语与快速备选共用，候选来源与键盘导航由 useComposerSuggest 管理。

interface Props {
  visible: boolean
  options: ComposerSuggestOption[]
  activeIndex: number
  loading?: boolean
  /** 没有候选时的提示 */
  emptyText?: string
  /** 顶部来源说明（例如快速备选的前缀与来源） */
  title?: string
}

const props = defineProps<Props>()

defineEmits<{
  (e: 'select', option: ComposerSuggestOption): void
  (e: 'hover', index: number): void
}>()

const listRef = ref<HTMLElement | null>(null)

// 候选较多时列表可滚动，键盘切换后让当前项保持可见
watch(() => [props.visible, props.activeIndex, props.options] as const, () => {
  if (!props.visible) return
  void nextTick(() => {
    const active = listRef.value?.children[props.activeIndex] as HTMLElement | undefined
    active?.scrollIntoView?.({ block: 'nearest' })
  })
})

const MAX_DESC_LENGTH = 40

function truncateDesc(desc: string): string {
  if (!desc) return ''
  // 取第一行
  const firstLine = desc.split(/[\r\n]/)[0] || ''
  if (firstLine.length <= MAX_DESC_LENGTH) {
    return firstLine
  }
  return firstLine.slice(0, MAX_DESC_LENGTH) + '...'
}
</script>

<style scoped>
.keyword-suggest {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 0.5rem;
  right: 0.5rem;
  max-width: 320px;
  z-index: 90;
  pointer-events: auto;
}

.keyword-suggest__content {
  background: var(--sc-bg-elevated);
  color: var(--sc-text-primary);
  font-family: var(--sc-font-family);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-radius: 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  border: 1px solid var(--sc-border-mute);
  padding: 0.35rem;
}

.keyword-suggest__title {
  padding: 0.15rem 0.6rem 0.3rem;
  font-size: 11px;
  color: var(--sc-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.keyword-suggest__list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  max-height: min(16rem, 40vh);
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}

.keyword-suggest__item {
  padding: 0.3rem 0.6rem;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.15s ease;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.keyword-suggest__item:hover,
.keyword-suggest__item.is-active {
  background: var(--sc-bg-hover);
}

.keyword-suggest__name {
  font-weight: 500;
  font-size: 13px;
  color: var(--sc-text-primary);
  flex-shrink: 0;
}

.keyword-suggest__category {
  display: inline-block;
  font-size: 10px;
  color: var(--sc-text-secondary);
  background: var(--sc-bg-layer);
  padding: 0 5px;
  border-radius: 3px;
  line-height: 1.4;
  flex-shrink: 0;
}

.keyword-suggest__desc {
  font-size: 11px;
  color: var(--sc-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.keyword-suggest__empty {
  padding: 0.5rem 0.75rem;
  text-align: center;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

/* 过渡动画 */
.keyword-suggest-fade-enter-active,
.keyword-suggest-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.keyword-suggest-fade-enter-from,
.keyword-suggest-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

/* 移动端适配 */
@media (max-width: 480px) {
  .keyword-suggest {
    left: 0.25rem;
    right: 0.25rem;
    max-width: none;
    bottom: calc(100% + 4px);
  }

  .keyword-suggest__content {
    padding: 0.3rem;
    border-radius: 8px;
  }

  .keyword-suggest__item {
    padding: 0.4rem 0.5rem;
    gap: 0.4rem;
  }

  .keyword-suggest__name {
    font-size: 14px;
  }

  .keyword-suggest__category {
    font-size: 10px;
  }

  .keyword-suggest__desc {
    display: none;
  }

  .keyword-suggest__empty {
    padding: 0.6rem;
    font-size: 13px;
  }
}
</style>
