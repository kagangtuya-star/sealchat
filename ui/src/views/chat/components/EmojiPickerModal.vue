<script setup lang="ts">
import { computed, reactive, ref, onMounted, onUnmounted, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { addRecentEmoji, loadRecentEmojis } from '@/utils/recentEmojis';
import { normalizeAttachmentId, resolveAttachmentUrl, fetchAttachmentMetaById, type AttachmentMeta } from '@/composables/useAttachmentResolver';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useGalleryStore } from '@/stores/gallery';
import { useUserStore } from '@/stores/user';
import { useChatStore } from '@/stores/chat';
import { matchText } from '@/utils/pinyinMatch';
import 'emoji-picker-element';

const emit = defineEmits<{
  (e: 'select', emoji: string): void;
  (e: 'close'): void;
}>();

const pickerRef = ref<HTMLElement | null>(null);
const fileInputRef = ref<HTMLInputElement | null>(null);
const uploading = ref(false);

const gallery = useGalleryStore();
const user = useUserStore();
const chat = useChatStore();
const message = useMessage();

const customEmojiItems = computed(() => gallery.reactionEmojiItems);
const searchKeyword = ref('');
const displayLimit = ref(120);
const customGridRef = ref<HTMLElement | null>(null);
const PAGE_SIZE = 120;
const activeTab = ref<'emoji' | 'reaction'>('emoji');
const customEmojiMetaCache = reactive<Record<string, AttachmentMeta | null>>({});
const pendingCustomEmojiMeta = new Set<string>();

const filteredCustomEmojiItems = computed(() => {
  const list = customEmojiItems.value;
  const keyword = searchKeyword.value.trim();
  if (!keyword) return list;
  return list.filter((item) => {
    const haystack = `${item.remark || ''} ${item.attachmentId || ''} ${item.id || ''}`;
    return matchText(keyword, haystack);
  });
});

const visibleCustomEmojiItems = computed(() =>
  filteredCustomEmojiItems.value.slice(0, displayLimit.value)
);

const hasMoreCustomEmoji = computed(() => filteredCustomEmojiItems.value.length > displayLimit.value);

const handleEmojiClick = (event: CustomEvent) => {
  const emoji = event.detail?.unicode;
  if (emoji) {
    addRecentEmoji(emoji);
    emit('select', emoji);
    emit('close');
  }
};

const ensureCustomEmojis = async () => {
  const ownerId = user.info?.id;
  if (!ownerId) return;
  try {
    await gallery.ensureEmojiCollection(ownerId);
  } catch (error) {
    console.warn('加载自定义表情失败', error);
  }
};

const handleCustomSelect = (attachmentId: string) => {
  if (!attachmentId) return;
  const value = `id:${normalizeAttachmentId(attachmentId)}`;
  addRecentEmoji(value);
  emit('select', value);
  emit('close');
};

const triggerUpload = () => {
  fileInputRef.value?.click();
};

const handleFileInput = async (event: Event) => {
  const target = event.target as HTMLInputElement | null;
  const files = target?.files ? Array.from(target.files) : [];
  if (files.length === 0) return;
  uploading.value = true;
  try {
    for (const file of files) {
      const result = await uploadImageAttachment(file, { channelId: chat.curChannel?.id });
      if (!result?.attachmentId) {
        continue;
      }
      const rawId = normalizeAttachmentId(result.attachmentId);
      await gallery.addReactionEmoji(rawId, user.info.id);
      addRecentEmoji(`id:${rawId}`);
    }
  } catch (error: any) {
    const errMsg = error?.message || '上传表情失败';
    message.error(errMsg);
  } finally {
    uploading.value = false;
    if (target) {
      target.value = '';
    }
  }
};

const handleCustomGridScroll = (event: Event) => {
  if (!hasMoreCustomEmoji.value) return;
  const target = event.target as HTMLElement | null;
  if (!target) return;
  if (target.scrollTop + target.clientHeight >= target.scrollHeight - 40) {
    displayLimit.value += PAGE_SIZE;
  }
};

const ensureCustomEmojiMeta = async (attachmentId: string) => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized || pendingCustomEmojiMeta.has(normalized) || customEmojiMetaCache[normalized] !== undefined) {
    return;
  }
  pendingCustomEmojiMeta.add(normalized);
  try {
    const meta = await fetchAttachmentMetaById(normalized);
    customEmojiMetaCache[normalized] = meta;
  } finally {
    pendingCustomEmojiMeta.delete(normalized);
  }
};

const resolveCustomEmojiSrc = (attachmentId: string, thumbUrl?: string) => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized) return '';
  const meta = customEmojiMetaCache[normalized];
  if (meta === undefined && !pendingCustomEmojiMeta.has(normalized)) {
    void ensureCustomEmojiMeta(normalized);
  }
  if (meta?.isAnimated) {
    return resolveAttachmentUrl(`id:${normalized}`);
  }
  return resolveAttachmentUrl(thumbUrl || `id:${normalized}`);
};

const handleCustomSearchInput = (event: Event) => {
  const target = event.target as HTMLInputElement | null;
  searchKeyword.value = target?.value || '';
};

let searchInputListenerCleanup: (() => void) | null = null;
let searchBindingCleanup: (() => void) | null = null;
let searchObserver: MutationObserver | null = null;
let emojiFallbackCleanup: (() => void) | null = null;

const bindPickerSearch = () => {
  const picker = pickerRef.value?.querySelector('emoji-picker') as HTMLElement & { shadowRoot?: ShadowRoot } | null;
  if (!picker) return;

  const handleInput = (event: Event) => {
    const target = event.target as HTMLInputElement | null;
    searchKeyword.value = target?.value || '';
  };

  const attachSearchInput = () => {
    const root = picker.shadowRoot;
    if (!root) return false;
    const input = root.querySelector('input[type="search"], input') as HTMLInputElement | null;
    if (!input) return false;
    input.addEventListener('input', handleInput);
    searchInputListenerCleanup = () => {
      input.removeEventListener('input', handleInput);
    };
    return true;
  };

  const handleSearchEvent = (event: Event) => {
    const detail = (event as CustomEvent).detail as { search?: string } | undefined;
    if (detail?.search !== undefined) {
      searchKeyword.value = detail.search || '';
      return;
    }
    const input = picker.shadowRoot?.querySelector('input[type="search"], input') as HTMLInputElement | null;
    if (input) {
      searchKeyword.value = input.value || '';
    }
  };

  picker.addEventListener('search', handleSearchEvent as EventListener);
  if (!attachSearchInput()) {
    if (picker.shadowRoot) {
      searchObserver = new MutationObserver(() => {
        if (attachSearchInput()) {
          searchObserver?.disconnect();
          searchObserver = null;
        }
      });
      searchObserver.observe(picker.shadowRoot, { childList: true, subtree: true });
    }
  }

  const cleanup = () => {
    picker.removeEventListener('search', handleSearchEvent as EventListener);
    if (searchInputListenerCleanup) {
      searchInputListenerCleanup();
      searchInputListenerCleanup = null;
    }
    if (searchObserver) {
      searchObserver.disconnect();
      searchObserver = null;
    }
  };

  return cleanup;
};

const bindPickerEmojiFallback = () => {
  const picker = pickerRef.value?.querySelector('emoji-picker') as HTMLElement & { shadowRoot?: ShadowRoot } | null;
  const root = picker?.shadowRoot;
  if (!root) return;

  const handleImgError = (event: Event) => {
    const target = event.target as HTMLElement | null;
    if (!target || target.tagName !== 'IMG') return;
    const img = target as HTMLImageElement;
    const emoji = img.dataset.emoji || img.alt || img.getAttribute('aria-label') || '';
    if (!emoji) return;
    const fallback = document.createElement('span');
    fallback.textContent = emoji;
    fallback.style.display = 'inline-flex';
    fallback.style.alignItems = 'center';
    fallback.style.justifyContent = 'center';
    const width = img.getAttribute('width') || img.style.width;
    const height = img.getAttribute('height') || img.style.height;
    if (width) fallback.style.width = width;
    if (height) fallback.style.height = height;
    fallback.style.fontSize = '1em';
    fallback.style.lineHeight = '1';
    img.replaceWith(fallback);
  };

  root.addEventListener('error', handleImgError, true);

  return () => {
    root.removeEventListener('error', handleImgError, true);
  };
};

onMounted(() => {
  const picker = pickerRef.value?.querySelector('emoji-picker');
  if (picker) {
    picker.addEventListener('emoji-click', handleEmojiClick as EventListener);
  }
  void loadRecentEmojis();
  void ensureCustomEmojis();
  searchBindingCleanup = bindPickerSearch() ?? null;
  emojiFallbackCleanup = bindPickerEmojiFallback() ?? null;
});

onUnmounted(() => {
  const picker = pickerRef.value?.querySelector('emoji-picker');
  if (picker) {
    picker.removeEventListener('emoji-click', handleEmojiClick as EventListener);
  }
  if (searchBindingCleanup) {
    searchBindingCleanup();
    searchBindingCleanup = null;
  }
  if (emojiFallbackCleanup) {
    emojiFallbackCleanup();
    emojiFallbackCleanup = null;
  }
  if (searchObserver) {
    searchObserver.disconnect();
    searchObserver = null;
  }
});

watch(searchKeyword, () => {
  displayLimit.value = PAGE_SIZE;
  if (customGridRef.value) {
    customGridRef.value.scrollTop = 0;
  }
});
</script>

<template>
  <div class="emoji-picker-modal" @click.self="emit('close')">
    <div ref="pickerRef" class="emoji-picker-container">
      <div class="emoji-picker-tabs">
        <button class="emoji-picker-close" type="button" @click="emit('close')">
          返回
        </button>
        <button
          class="emoji-picker-tab"
          :class="{ 'emoji-picker-tab--active': activeTab === 'emoji' }"
          type="button"
          @click="activeTab = 'emoji'"
        >
          表情
        </button>
        <button
          class="emoji-picker-tab"
          :class="{ 'emoji-picker-tab--active': activeTab === 'reaction' }"
          type="button"
          @click="activeTab = 'reaction'"
        >
          表情反应
        </button>
      </div>
      <div class="emoji-picker-body">
        <div v-show="activeTab === 'reaction'" class="custom-emoji-section">
          <div class="custom-emoji-header">
            <span>自定义表情反应</span>
            <button class="custom-emoji-upload" type="button" :disabled="uploading" @click="triggerUpload">
              {{ uploading ? '上传中…' : '上传表情' }}
            </button>
            <input
              ref="fileInputRef"
              class="custom-emoji-input"
              type="file"
              accept="image/*"
              multiple
              @change="handleFileInput"
            />
          </div>
          <div class="custom-emoji-search">
            <input
              type="text"
              class="custom-emoji-search__input"
              placeholder="搜索表情"
              :value="searchKeyword"
              @input="handleCustomSearchInput"
            />
          </div>
          <div v-if="customEmojiItems.length" class="custom-emoji-meta">
            <span v-if="searchKeyword">匹配 {{ filteredCustomEmojiItems.length }} / {{ customEmojiItems.length }}</span>
            <span v-else>共 {{ customEmojiItems.length }} 个</span>
          </div>
          <div
            v-if="filteredCustomEmojiItems.length"
            ref="customGridRef"
            class="custom-emoji-grid"
            @scroll="handleCustomGridScroll"
          >
            <button
              v-for="item in visibleCustomEmojiItems"
              :key="item.id"
              class="custom-emoji-item"
              type="button"
              :title="item.remark || '自定义表情'"
              @click="handleCustomSelect(item.attachmentId)"
            >
              <img :src="resolveCustomEmojiSrc(item.attachmentId, item.thumbUrl)" :alt="item.remark || 'emoji'" />
            </button>
          </div>
          <div v-else class="custom-emoji-empty">
            {{ searchKeyword ? '没有匹配的表情' : '暂无自定义表情' }}
          </div>
          <div v-if="hasMoreCustomEmoji" class="custom-emoji-more">继续下拉加载更多</div>
        </div>
        <div v-show="activeTab === 'emoji'" class="emoji-picker-pane">
          <emoji-picker class="light"></emoji-picker>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.emoji-picker-modal {
  position: fixed;
  inset: 0;
  z-index: 2000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.3);
}

.emoji-picker-container {
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.25);
  background: var(--sc-bg-surface, #fff);
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.08));
  width: min(400px, 92vw);
  max-width: 400px;
  max-height: min(720px, 92vh);
  display: flex;
  flex-direction: column;
}

.emoji-picker-tabs {
  display: flex;
  gap: 6px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.08));
  background: var(--sc-bg-layer, rgba(15, 23, 42, 0.02));
}

.emoji-picker-close {
  display: none;
  border: none;
  background: transparent;
  color: var(--sc-text-secondary, #475569);
  font-size: 13px;
  padding: 6px 8px;
  border-radius: 999px;
  cursor: pointer;
}

.emoji-picker-tab {
  border: none;
  background: transparent;
  color: var(--sc-text-secondary, #475569);
  font-size: 13px;
  padding: 6px 10px;
  border-radius: 999px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.emoji-picker-tab--active {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.08));
  color: var(--sc-text-primary, #0f172a);
  font-weight: 600;
}

.emoji-picker-body {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}

.emoji-picker-pane {
  display: flex;
  flex: 1;
  min-height: 0;
}

.custom-emoji-section {
  padding: 12px 16px 8px;
}

.custom-emoji-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  color: var(--sc-text-secondary);
  margin-bottom: 8px;
}

.custom-emoji-upload {
  border: none;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
  color: var(--sc-text-primary);
  padding: 4px 10px;
  border-radius: 10px;
  cursor: pointer;
  font-size: 12px;
}

.custom-emoji-upload:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.custom-emoji-input {
  display: none;
}

.custom-emoji-search {
  margin-bottom: 8px;
}

.custom-emoji-search__input {
  width: 100%;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
  background: var(--sc-bg-layer, rgba(15, 23, 42, 0.03));
  color: var(--sc-text-primary, #0f172a);
  padding: 6px 10px;
  border-radius: 10px;
  font-size: 12px;
}

.custom-emoji-search__input::placeholder {
  color: var(--sc-text-tertiary, rgba(15, 23, 42, 0.5));
}

.custom-emoji-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  color: var(--sc-text-secondary);
  margin-bottom: 6px;
}

.custom-emoji-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
  gap: 6px;
  max-height: min(240px, 35vh);
  overflow: auto;
  padding-right: 4px;
}

.custom-emoji-item {
  border: 1px solid transparent;
  background: transparent;
  border-radius: 8px;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.custom-emoji-item img {
  width: 26px;
  height: 26px;
  object-fit: contain;
}

.custom-emoji-item:hover {
  border-color: var(--sc-border-strong, rgba(15, 23, 42, 0.12));
  background: var(--sc-bg-layer, rgba(15, 23, 42, 0.04));
}

.custom-emoji-empty {
  padding: 8px 0 6px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.custom-emoji-more {
  margin-top: 6px;
  font-size: 12px;
  color: var(--sc-text-tertiary, rgba(15, 23, 42, 0.5));
}

emoji-picker {
  --num-columns: 8;
  --emoji-padding: 0.32rem;
  --category-emoji-size: 1.25rem;
  flex: 1;
  width: 100%;
}

:root[data-display-palette='night'] emoji-picker {
  --background: #1e1e28;
  --border-color: rgba(255, 255, 255, 0.1);
}

:root[data-custom-theme='true'] emoji-picker {
  --background: var(--sc-bg-surface);
  --border-color: var(--sc-border-strong);
  --button-hover-background: var(--sc-bg-layer);
  --button-active-background: var(--sc-bg-layer);
  --input-border-color: var(--sc-border-mute);
  --input-bg: var(--sc-bg-layer);
  --input-text-color: var(--sc-text-primary);
}

@media (max-width: 768px) {
  .emoji-picker-modal {
    align-items: stretch;
  }

  .emoji-picker-container {
    width: 100%;
    height: 100%;
    max-height: none;
    border-radius: 0;
    max-width: none;
  }

  .emoji-picker-close {
    display: inline-flex;
    align-items: center;
  }

  .custom-emoji-grid {
    grid-template-columns: repeat(auto-fill, minmax(48px, 1fr));
    max-height: 32vh;
  }

  .custom-emoji-item {
    width: 48px;
    height: 48px;
  }

  .custom-emoji-item img {
    width: 32px;
    height: 32px;
  }

  emoji-picker {
    --num-columns: 8;
    --emoji-padding: 0.24rem;
  }
}

:root[data-custom-theme='true'] emoji-picker {
  --background: var(--sc-bg-surface);
  --border-color: var(--sc-border-strong);
  --text-color: var(--sc-text-primary);
}

:root[data-display-palette='night'] .emoji-picker-container {
  background: #1e1e28;
  border-color: rgba(255, 255, 255, 0.1);
}

:root[data-display-palette='night'] .custom-emoji-section {
  border-color: rgba(255, 255, 255, 0.08);
}

:root[data-display-palette='night'] .custom-emoji-upload {
  background: rgba(255, 255, 255, 0.08);
  color: #f4f4f5;
}

:root[data-display-palette='night'] .custom-emoji-item:hover {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(255, 255, 255, 0.08);
}
</style>
