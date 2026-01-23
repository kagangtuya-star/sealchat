<script setup lang="ts">
import { computed } from 'vue';
import type { MessageReaction } from '@/types';
import { buildEmojiRenderInfo } from '@/utils/emojiRender';

const props = defineProps<{
  reactions: MessageReaction[];
  messageId: string;
}>();

const emit = defineEmits<{
  (e: 'toggle', emoji: string): void;
}>();

const reactionItems = computed(() =>
  props.reactions.map((item) => {
    const render = buildEmojiRenderInfo(item.emoji);
    return {
      ...item,
      url: render.src,
      fallbackUrl: render.fallback,
      isCustom: render.isCustom,
    };
  })
);

const handleImgError = (event: Event) => {
  const img = event.target as HTMLImageElement;
  const fallback = img.dataset.fallback;
  if (fallback && img.src !== fallback) {
    img.src = fallback;
  }
};
</script>

<template>
  <div v-if="reactionItems.length" class="message-reactions">
    <button
      v-for="reaction in reactionItems"
      :key="reaction.emoji"
      class="message-reactions__item"
      :class="{ 'message-reactions__item--active': reaction.meReacted }"
      :title="reaction.emoji"
      @click="emit('toggle', reaction.emoji)"
    >
      <img
        :src="reaction.url"
        :alt="reaction.emoji"
        :data-fallback="reaction.fallbackUrl"
        class="message-reactions__emoji"
        loading="lazy"
        @error="handleImgError"
      />
      <span class="message-reactions__count">{{ reaction.count }}</span>
    </button>
  </div>
</template>

<style scoped>
.message-reactions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 6px;
}

.message-reactions__item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border: 1px solid var(--chat-border-mute);
  border-radius: 12px;
  background: var(--sc-bg-elevated, rgba(0, 0, 0, 0.03));
  cursor: pointer;
  font-size: 14px;
  transition: all 0.15s;
}

.message-reactions__item:hover {
  border-color: var(--primary-color, #3b82f6);
}

.message-reactions__item--active {
  background: color-mix(in srgb, var(--primary-color, #3b82f6) 15%, transparent);
  border-color: var(--primary-color, #3b82f6);
}

.message-reactions__emoji {
  width: 16px;
  height: 16px;
}

.message-reactions__count {
  color: var(--chat-text-secondary);
  font-size: 12px;
  font-weight: 500;
}

.chat--layout-compact .message-reactions {
  margin-top: 4px;
}

.chat--layout-compact .message-reactions__item {
  padding: 1px 6px;
}

.chat--layout-compact .message-reactions__emoji {
  width: 14px;
  height: 14px;
}

.chat--layout-compact .message-reactions__count {
  font-size: 11px;
}

:root[data-display-palette='night'] .message-reactions__item {
  background: rgba(255, 255, 255, 0.05);
}

:root[data-display-palette='night'] .message-reactions__item:hover {
  background: rgba(255, 255, 255, 0.08);
}

:root[data-display-palette='night'] .message-reactions__item--active {
  background: color-mix(in srgb, var(--primary-color, #3b82f6) 20%, transparent);
}
</style>
