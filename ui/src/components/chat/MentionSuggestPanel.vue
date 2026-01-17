<template>
  <transition name="mention-suggest-fade">
    <div v-if="visible" class="mention-suggest" @mousedown.prevent>
      <div class="mention-suggest__content">
        <div v-if="items.length" class="mention-suggest__list">
          <!-- @all option -->
          <div
            v-if="canAtAll"
            class="mention-suggest__item mention-suggest__item--all"
            :class="{ 'is-active': activeIndex === 0 }"
            @mousedown.prevent="$emit('select', { userId: 'all', displayName: '全体成员', identityType: 'all' })"
            @mouseenter="$emit('hover', 0)"
          >
            <div class="mention-suggest__avatar mention-suggest__avatar--all">@</div>
            <span class="mention-suggest__name">全体成员</span>
            <span class="mention-suggest__tag mention-suggest__tag--all">@all</span>
          </div>
          <!-- Member items -->
          <div
            v-for="(item, index) in items"
            :key="item.identityId || item.userId"
            class="mention-suggest__item"
            :class="{ 'is-active': index + (canAtAll ? 1 : 0) === activeIndex }"
            @mousedown.prevent="$emit('select', item)"
            @mouseenter="$emit('hover', index + (canAtAll ? 1 : 0))"
          >
            <Avatar :src="item.avatar" :size="24" :border="false" class="mention-suggest__avatar" />
            <span class="mention-suggest__name" :style="{ color: item.color || 'inherit' }">
              {{ item.displayName }}
            </span>
            <span v-if="item.identityType" class="mention-suggest__tag" :class="`mention-suggest__tag--${item.identityType}`">
              {{ identityTypeLabel(item.identityType) }}
            </span>
          </div>
        </div>
        <div v-else class="mention-suggest__empty">
          {{ loading ? '搜索中...' : '无匹配成员' }}
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import Avatar from '@/components/avatar.vue'

export interface MentionableItem {
  userId: string
  displayName: string
  color?: string
  avatar?: string
  identityId?: string
  identityType: 'ic' | 'ooc' | 'user' | 'all'
}

interface Props {
  visible: boolean
  items: MentionableItem[]
  activeIndex: number
  loading?: boolean
  canAtAll?: boolean
}

defineProps<Props>()

defineEmits<{
  (e: 'select', item: MentionableItem): void
  (e: 'hover', index: number): void
}>()

function identityTypeLabel(type: string): string {
  switch (type) {
    case 'ic':
      return '场内'
    case 'ooc':
      return '场外'
    case 'user':
      return '用户'
    default:
      return ''
  }
}
</script>

<style scoped>
.mention-suggest {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 0.5rem;
  right: 0.5rem;
  max-width: 320px;
  z-index: 90;
  pointer-events: auto;
}

.mention-suggest__content {
  background: var(--card-color);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-radius: 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  border: 1px solid rgba(148, 163, 184, 0.2);
  padding: 0.35rem;
  max-height: 280px;
  overflow-y: auto;
}

.mention-suggest__list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.mention-suggest__item {
  padding: 0.35rem 0.6rem;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.15s ease;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.mention-suggest__item:hover,
.mention-suggest__item.is-active {
  background: rgba(148, 163, 184, 0.18);
}

.mention-suggest__avatar {
  flex-shrink: 0;
}

.mention-suggest__avatar--all {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #ef4444, #f97316);
  color: white;
  font-weight: 600;
  font-size: 12px;
  border-radius: 6px;
}

.mention-suggest__name {
  font-weight: 500;
  font-size: 13px;
  color: var(--text-color-1);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mention-suggest__tag {
  display: inline-block;
  font-size: 10px;
  padding: 0 5px;
  border-radius: 3px;
  line-height: 1.5;
  flex-shrink: 0;
}

.mention-suggest__tag--ic {
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}

.mention-suggest__tag--ooc {
  background: rgba(168, 85, 247, 0.15);
  color: #a855f7;
}

.mention-suggest__tag--user {
  background: rgba(148, 163, 184, 0.15);
  color: var(--text-color-3);
}

.mention-suggest__tag--all {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.mention-suggest__item--all .mention-suggest__name {
  color: #ef4444;
}

.mention-suggest__empty {
  padding: 0.5rem 0.75rem;
  text-align: center;
  font-size: 12px;
  color: var(--text-color-3);
}

/* 夜间模式 */
:root[data-display-palette='night'] .mention-suggest__tag--ic {
  background: rgba(59, 130, 246, 0.25);
  color: #60a5fa;
}

:root[data-display-palette='night'] .mention-suggest__tag--ooc {
  background: rgba(168, 85, 247, 0.25);
  color: #c084fc;
}

:root[data-display-palette='night'] .mention-suggest__tag--all {
  background: rgba(239, 68, 68, 0.25);
  color: #f87171;
}

:root[data-display-palette='night'] .mention-suggest__item--all .mention-suggest__name {
  color: #f87171;
}

/* 过渡动画 */
.mention-suggest-fade-enter-active,
.mention-suggest-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.mention-suggest-fade-enter-from,
.mention-suggest-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

/* 移动端适配 */
@media (max-width: 480px) {
  .mention-suggest {
    left: 0.25rem;
    right: 0.25rem;
    max-width: none;
    bottom: calc(100% + 4px);
  }

  .mention-suggest__content {
    padding: 0.3rem;
    border-radius: 8px;
    max-height: 240px;
  }

  .mention-suggest__item {
    padding: 0.4rem 0.5rem;
    gap: 0.4rem;
  }

  .mention-suggest__name {
    font-size: 14px;
  }

  .mention-suggest__empty {
    padding: 0.6rem;
    font-size: 13px;
  }
}
</style>
