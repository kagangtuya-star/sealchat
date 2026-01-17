<template>
  <transition name="keyword-suggest-fade">
    <div v-if="visible" class="keyword-suggest" @mousedown.prevent>
      <div class="keyword-suggest__content">
        <div v-if="options.length" class="keyword-suggest__list">
          <div
            v-for="(option, index) in options"
            :key="option.keyword.id"
            class="keyword-suggest__item"
            :class="{ 'is-active': index === activeIndex }"
            @mousedown.prevent="$emit('select', option)"
            @mouseenter="$emit('hover', index)"
          >
            <span class="keyword-suggest__name">{{ option.keyword.keyword }}</span>
            <span v-if="option.keyword.category" class="keyword-suggest__category">{{ option.keyword.category }}</span>
            <span v-if="option.keyword.description" class="keyword-suggest__desc">{{ truncateDesc(option.keyword.description) }}</span>
          </div>
        </div>
        <div v-else class="keyword-suggest__empty">
          {{ loading ? '搜索中...' : '无匹配术语' }}
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import type { KeywordMatchResult } from '@/utils/pinyinMatch'

interface Props {
  visible: boolean
  options: KeywordMatchResult[]
  activeIndex: number
  loading?: boolean
}

defineProps<Props>()

defineEmits<{
  (e: 'select', option: KeywordMatchResult): void
  (e: 'hover', index: number): void
}>()

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
  background: var(--card-color);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-radius: 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  border: 1px solid rgba(148, 163, 184, 0.2);
  padding: 0.35rem;
}

.keyword-suggest__list {
  display: flex;
  flex-direction: column;
  gap: 1px;
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
  background: rgba(148, 163, 184, 0.18);
}

.keyword-suggest__name {
  font-weight: 500;
  font-size: 13px;
  color: var(--text-color-1);
  flex-shrink: 0;
}

.keyword-suggest__category {
  display: inline-block;
  font-size: 10px;
  color: var(--text-color-3);
  background: rgba(148, 163, 184, 0.12);
  padding: 0 5px;
  border-radius: 3px;
  line-height: 1.4;
  flex-shrink: 0;
}

.keyword-suggest__desc {
  font-size: 11px;
  color: var(--text-color-3);
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
  color: var(--text-color-3);
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
