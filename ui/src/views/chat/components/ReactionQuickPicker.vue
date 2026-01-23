<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, reactive, ref } from 'vue';
import { Plus } from '@vicons/tabler';
import { buildEmojiRenderInfo } from '@/utils/emojiRender';
import { noteEmojiLoadFailure } from '@/utils/twemoji';
import { addRecentEmoji, getQuickEmojis, loadRecentEmojis, subscribeRecentEmojis } from '@/utils/recentEmojis';

const emit = defineEmits<{
  (e: 'select', emoji: string): void;
  (e: 'expand'): void;
}>();

const refreshTick = ref(0);
const textFallback = reactive<Record<string, boolean>>({});

const emojiItems = computed(() => {
  refreshTick.value;
  return getQuickEmojis().map((item) => {
    const render = buildEmojiRenderInfo(item.value);
    return {
      emoji: item.value,
      url: render.src,
      fallbackUrl: render.fallback,
      isCustom: render.isCustom,
      useText: render.asText || !!textFallback[item.value],
    };
  });
});

const handleSelect = (emoji: string) => {
  addRecentEmoji(emoji);
  refreshTick.value += 1;
  emit('select', emoji);
};

const handleImgError = (event: Event) => {
  const img = event.target as HTMLImageElement;
  const emoji = img.dataset.emoji || img.alt || '';
  noteEmojiLoadFailure(img.src, emoji);
  if (emoji) {
    textFallback[emoji] = true;
  }
};

let unsubscribe: (() => void) | null = null;

onMounted(() => {
  void loadRecentEmojis();
  unsubscribe = subscribeRecentEmojis(() => {
    refreshTick.value += 1;
  });
});

onBeforeUnmount(() => {
  if (unsubscribe) {
    unsubscribe();
    unsubscribe = null;
  }
});
</script>

<template>
  <div class="reaction-quick-picker">
    <button
      v-for="item in emojiItems"
      :key="item.emoji"
      class="reaction-quick-picker__item"
      :title="item.emoji"
      @click="handleSelect(item.emoji)"
    >
      <span v-if="item.useText" class="reaction-quick-picker__emoji-text">{{ item.emoji }}</span>
      <img
        v-else
        :src="item.url"
        :alt="item.emoji"
        :data-emoji="item.emoji"
        :data-fallback="item.fallbackUrl"
        class="twemoji-img"
        @error="handleImgError"
      />
    </button>
    <button class="reaction-quick-picker__expand" @click="emit('expand')">
      <n-icon :component="Plus" :size="18" />
    </button>
  </div>
</template>

<style scoped>
.reaction-quick-picker {
  display: flex;
  gap: 2px;
  padding: 6px 8px;
  background: var(--sc-bg-elevated, #fff);
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  border-bottom: 1px solid var(--chat-border-mute);
}

.reaction-quick-picker__item {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.15s;
}

.reaction-quick-picker__item:hover {
  background: var(--sc-bg-hover, rgba(0, 0, 0, 0.05));
}

.reaction-quick-picker__item .twemoji-img {
  width: 22px;
  height: 22px;
}

.reaction-quick-picker__emoji-text {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  font-size: 20px;
  line-height: 1;
}

.reaction-quick-picker__expand {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  cursor: pointer;
  border-radius: 6px;
  color: var(--chat-text-secondary);
  transition: background 0.15s, color 0.15s;
}

.reaction-quick-picker__expand:hover {
  background: var(--sc-bg-hover, rgba(0, 0, 0, 0.05));
  color: var(--primary-color, #3b82f6);
}

:root[data-display-palette='night'] .reaction-quick-picker {
  background: rgba(30, 30, 40, 0.95);
  border-color: rgba(255, 255, 255, 0.1);
}

:root[data-display-palette='night'] .reaction-quick-picker__item:hover,
:root[data-display-palette='night'] .reaction-quick-picker__expand:hover {
  background: rgba(255, 255, 255, 0.1);
}

:root[data-custom-theme='true'] .reaction-quick-picker {
  background: var(--sc-bg-elevated);
  border-color: var(--sc-border-strong);
}

:root[data-custom-theme='true'] .reaction-quick-picker__item:hover,
:root[data-custom-theme='true'] .reaction-quick-picker__expand:hover {
  background: var(--sc-bg-layer);
}

@media (max-width: 768px) {
  .reaction-quick-picker {
    flex-wrap: wrap;
    max-width: min(92vw, 320px);
  }

  .reaction-quick-picker__item,
  .reaction-quick-picker__expand {
    width: 28px;
    height: 28px;
  }

  .reaction-quick-picker__item .twemoji-img {
    width: 20px;
    height: 20px;
  }

  .reaction-quick-picker__emoji-text {
    width: 20px;
    height: 20px;
    font-size: 18px;
  }
}
</style>
