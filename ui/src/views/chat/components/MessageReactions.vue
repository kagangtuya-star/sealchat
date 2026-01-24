<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue';
import type { MessageReaction } from '@/types';
import { buildEmojiRenderInfo } from '@/utils/emojiRender';
import { noteEmojiLoadFailure } from '@/utils/twemoji';
import { api } from '@/stores/_config';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';

interface ReactionUserItem {
  userId: string;
  identityId?: string;
  displayName: string;
  identityColor?: string;
}

const props = defineProps<{
  reactions: MessageReaction[];
  messageId: string;
}>();

const emit = defineEmits<{
  (e: 'toggle', emoji: string): void;
}>();

const chat = useChatStore();
const user = useUserStore();

const textFallback = reactive<Record<string, boolean>>({});

const reactionItems = computed(() =>
  props.reactions.map((item) => {
    const render = buildEmojiRenderInfo(item.emoji);
    return {
      ...item,
      url: render.src,
      fallbackUrl: render.fallback,
      isCustom: render.isCustom,
      useText: render.asText || !!textFallback[item.emoji],
    };
  })
);

const isMobileUa = typeof navigator !== 'undefined'
  ? /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  : false;

const DEFAULT_FETCH_LIMIT = 10;
const PAGE_FETCH_LIMIT = 50;
const SUMMARY_NAME_LIMIT = 3;
const CACHE_IMMEDIATE_MS = 800;
const REFRESH_DELAY_MS = 700;
const LONG_PRESS_DELAY = isMobileUa ? 420 : 360;
const longPressTimer = ref<number | null>(null);
const suppressNextClick = ref(false);
const suppressResetTimer = ref<number | null>(null);
const hoverCloseTimer = ref<number | null>(null);
const refreshTimer = ref<number | null>(null);
const requestSeq = ref(0);
const reactionUserCache = reactive<Record<string, { items: ReactionUserItem[]; total: number; ts: number; complete: boolean }>>({});

const popoverState = reactive({
  show: false,
  emoji: '',
  users: [] as ReactionUserItem[],
  total: 0,
  loading: false,
  allLoading: false,
  expanded: false,
  error: '',
});

const reactionFollowerClass = 'reaction-users-follower';

const markReactionFollower = (enabled: boolean) => {
  if (typeof document === 'undefined') return;
  const followers = Array.from(document.querySelectorAll('.v-binder-follower-content')) as HTMLElement[];
  followers.forEach((el) => {
    if (!el) return;
    if (!el.querySelector('.reaction-users-popover')) return;
    if (enabled) {
      el.classList.add(reactionFollowerClass);
    } else {
      el.classList.remove(reactionFollowerClass);
    }
  });
};

const handleImgError = (event: Event) => {
  const img = event.target as HTMLImageElement;
  const emoji = img.dataset.emoji || img.alt || '';
  noteEmojiLoadFailure(img.src, emoji);
  if (emoji) {
    textFallback[emoji] = true;
  }
};

const fetchReactionUsersPage = async (emoji: string, limit: number, offset = 0) => {
  const resp = await api.get(`api/v1/messages/${props.messageId}/reactions/users`, {
    params: { emoji, limit, offset },
  });
  const items = Array.isArray(resp?.data?.items) ? resp.data.items : [];
  const total = typeof resp?.data?.total === 'number' ? resp.data.total : items.length;
  return { items, total };
};

const mergeReactionUsers = (existing: ReactionUserItem[], incoming: ReactionUserItem[]) => {
  if (existing.length === 0) {
    return incoming;
  }
  const incomingMap = new Map<string, ReactionUserItem>();
  incoming.forEach((item) => {
    incomingMap.set(item.userId, item);
  });
  const merged: ReactionUserItem[] = [];
  existing.forEach((item) => {
    const next = incomingMap.get(item.userId);
    if (next) {
      merged.push({ ...item, ...next });
      incomingMap.delete(item.userId);
    }
  });
  incoming.forEach((item) => {
    if (incomingMap.has(item.userId)) {
      merged.push(item);
      incomingMap.delete(item.userId);
    }
  });
  return merged;
};

const applyCachedUsers = (cached: { items: ReactionUserItem[]; total: number; complete: boolean }, limit: number) => {
  popoverState.users = cached.items.slice(0, limit);
  popoverState.total = cached.total;
  popoverState.loading = false;
  popoverState.error = '';
  popoverState.expanded = cached.complete && cached.items.length >= cached.total;
};

const fetchReactionUsers = async (emoji: string, limit = DEFAULT_FETCH_LIMIT, options?: { force?: boolean }) => {
  if (!props.messageId || !emoji) return;
  const force = options?.force === true;
  const cached = reactionUserCache[emoji];
  const now = Date.now();
  if (!force && cached && now - cached.ts < CACHE_IMMEDIATE_MS) {
    applyCachedUsers(cached, limit);
    return;
  }
  if (!force && cached && now - cached.ts < 30000 && cached.items.length >= limit) {
    applyCachedUsers(cached, limit);
    return;
  }
  const seq = ++requestSeq.value;
  popoverState.loading = true;
  popoverState.error = '';
  try {
    const { items, total } = await fetchReactionUsersPage(emoji, limit, 0);
    if (seq !== requestSeq.value) return;
    const base = popoverState.show && popoverState.emoji === emoji ? popoverState.users : (reactionUserCache[emoji]?.items || []);
    const merged = mergeReactionUsers(base, items);
    popoverState.users = merged;
    popoverState.total = total;
    reactionUserCache[emoji] = { items: merged, total, ts: Date.now(), complete: items.length >= total };
  } catch (error) {
    if (seq !== requestSeq.value) return;
    popoverState.users = [];
    popoverState.total = 0;
    popoverState.error = '加载失败';
  } finally {
    if (seq === requestSeq.value) {
      popoverState.loading = false;
    }
  }
};

const getSelfReactionUser = (): ReactionUserItem | null => {
  const userId = user.info.id;
  if (!userId) return null;
  const identity = chat.getActiveIdentity(chat.curChannel?.id);
  const displayName = identity?.displayName
    || chat.curMember?.nick
    || user.info.nick
    || user.info.username
    || '我';
  const item: ReactionUserItem = {
    userId,
    displayName,
  };
  if (identity?.id) {
    item.identityId = identity.id;
  }
  if (identity?.color) {
    item.identityColor = identity.color;
  }
  return item;
};

const syncUserListForSelf = (list: ReactionUserItem[], target: ReactionUserItem, shouldHave: boolean) => {
  const next = list.filter((item) => item.userId !== target.userId);
  if (shouldHave) {
    next.push(target);
  }
  return next;
};

const applyOptimisticReactionChange = (emoji: string, nextCount: number, shouldHave: boolean) => {
  const selfItem = getSelfReactionUser();
  if (popoverState.show && popoverState.emoji === emoji) {
    popoverState.total = nextCount;
    if (selfItem) {
      popoverState.users = syncUserListForSelf(popoverState.users, selfItem, shouldHave);
      if (popoverState.expanded && popoverState.users.length > nextCount) {
        popoverState.users = popoverState.users.slice(0, nextCount);
      }
    }
  }

  if (!selfItem) {
    return;
  }
  const cached = reactionUserCache[emoji];
  const cachedItems = cached ? cached.items : [];
  const nextItems = syncUserListForSelf(cachedItems, selfItem, shouldHave);
  let total = cached?.total ?? nextItems.length;
  if (Number.isFinite(nextCount)) {
    total = nextCount;
  }
  if (nextItems.length > total) {
    nextItems.length = total;
  }
  reactionUserCache[emoji] = {
    items: nextItems,
    total,
    ts: Date.now(),
    complete: cached?.complete ?? false,
  };
};

const openPopover = (emoji: string, count: number, limit = DEFAULT_FETCH_LIMIT) => {
  popoverState.show = true;
  if (popoverState.emoji !== emoji) {
    popoverState.users = [];
    popoverState.error = '';
    popoverState.expanded = false;
  }
  popoverState.emoji = emoji;
  popoverState.total = count;
  void fetchReactionUsers(emoji, limit);
};

const closePopover = () => {
  popoverState.show = false;
  popoverState.emoji = '';
  suppressNextClick.value = false;
};

const startLongPress = (emoji: string, count: number, event: PointerEvent) => {
  if (event.pointerType !== 'touch') {
    return;
  }
  if (longPressTimer.value !== null) {
    clearTimeout(longPressTimer.value);
  }
  longPressTimer.value = window.setTimeout(() => {
    longPressTimer.value = null;
    suppressNextClick.value = true;
    if (suppressResetTimer.value !== null) {
      clearTimeout(suppressResetTimer.value);
    }
    suppressResetTimer.value = window.setTimeout(() => {
      suppressNextClick.value = false;
      suppressResetTimer.value = null;
    }, 700);
    openPopover(emoji, count);
  }, LONG_PRESS_DELAY);
};

const cancelLongPress = () => {
  if (longPressTimer.value !== null) {
    clearTimeout(longPressTimer.value);
    longPressTimer.value = null;
  }
};

const handleReactionClick = (emoji: string, count: number, event: MouseEvent) => {
  if (suppressNextClick.value) {
    suppressNextClick.value = false;
    event.preventDefault();
    event.stopPropagation();
    return;
  }
  const reaction = props.reactions.find((item) => item.emoji === emoji);
  const meReacted = reaction?.meReacted ?? false;
  const nextCount = meReacted ? Math.max(0, count - 1) : count + 1;
  applyOptimisticReactionChange(emoji, nextCount, !meReacted);
  if (!isMobileUa) {
    openPopover(emoji, nextCount, DEFAULT_FETCH_LIMIT);
  }
  emit('toggle', emoji);
  if (!isMobileUa) {
    if (refreshTimer.value !== null) {
      clearTimeout(refreshTimer.value);
    }
    refreshTimer.value = window.setTimeout(() => {
      if (popoverState.show && popoverState.emoji === emoji) {
        void fetchReactionUsers(emoji, DEFAULT_FETCH_LIMIT, { force: true });
      }
      refreshTimer.value = null;
    }, REFRESH_DELAY_MS);
  }
};

const handleHoverOpen = (emoji: string, count: number) => {
  if (isMobileUa) return;
  if (hoverCloseTimer.value !== null) {
    clearTimeout(hoverCloseTimer.value);
    hoverCloseTimer.value = null;
  }
  openPopover(emoji, count, DEFAULT_FETCH_LIMIT);
};

const scheduleHoverClose = () => {
  if (isMobileUa) return;
  if (hoverCloseTimer.value !== null) {
    clearTimeout(hoverCloseTimer.value);
  }
  hoverCloseTimer.value = window.setTimeout(() => {
    closePopover();
    hoverCloseTimer.value = null;
  }, 120);
};

const cancelHoverClose = () => {
  if (hoverCloseTimer.value !== null) {
    clearTimeout(hoverCloseTimer.value);
    hoverCloseTimer.value = null;
  }
};

const summaryNames = computed(() => {
  const total = popoverState.total;
  const limit = total > DEFAULT_FETCH_LIMIT ? SUMMARY_NAME_LIMIT : total || DEFAULT_FETCH_LIMIT;
  return popoverState.users.slice(0, limit).map((item) => item.displayName).filter(Boolean);
});

const summaryText = computed(() => {
  const total = popoverState.total;
  if (popoverState.loading && total === 0) {
    return '加载中…';
  }
  if (!total) {
    return '暂无回应';
  }
  const names = summaryNames.value;
  if (names.length === 0) {
    return `被${total}位用户反应`;
  }
  if (total > names.length) {
    return `被${names.join(', ')}和${total - names.length}位其他用户反应`;
  }
  return `被${names.join(', ')}反应`;
});

const showAllLink = computed(() => popoverState.total > DEFAULT_FETCH_LIMIT && !popoverState.expanded);

const loadAllUsers = async () => {
  if (popoverState.allLoading || !popoverState.emoji) return;
  const emoji = popoverState.emoji;
  const cached = reactionUserCache[emoji];
  if (cached && cached.complete && cached.items.length >= cached.total) {
    popoverState.users = cached.items;
    popoverState.total = cached.total;
    popoverState.expanded = true;
    return;
  }
  const seq = ++requestSeq.value;
  popoverState.allLoading = true;
  popoverState.error = '';
  try {
    let offset = 0;
    let total = popoverState.total;
    const allItems: ReactionUserItem[] = [];
    while (offset < total && allItems.length < 1000) {
      const { items, total: respTotal } = await fetchReactionUsersPage(emoji, PAGE_FETCH_LIMIT, offset);
      if (seq !== requestSeq.value) return;
      if (typeof respTotal === 'number' && respTotal > 0) {
        total = respTotal;
      }
      if (items.length === 0) {
        break;
      }
      allItems.push(...items);
      offset += items.length;
      if (items.length < PAGE_FETCH_LIMIT) {
        break;
      }
    }
    popoverState.users = allItems;
    popoverState.total = total;
    popoverState.expanded = true;
    const base = reactionUserCache[emoji]?.items || popoverState.users;
    const merged = mergeReactionUsers(base, allItems);
    popoverState.users = merged;
    reactionUserCache[emoji] = { items: merged, total, ts: Date.now(), complete: merged.length >= total };
  } catch (error) {
    if (seq !== requestSeq.value) return;
    popoverState.error = '加载失败';
  } finally {
    if (seq === requestSeq.value) {
      popoverState.allLoading = false;
    }
  }
};

onBeforeUnmount(() => {
  cancelLongPress();
  cancelHoverClose();
  if (suppressResetTimer.value !== null) {
    clearTimeout(suppressResetTimer.value);
    suppressResetTimer.value = null;
  }
  if (refreshTimer.value !== null) {
    clearTimeout(refreshTimer.value);
    refreshTimer.value = null;
  }
});

watch(reactionItems, (items) => {
  if (!popoverState.show || !popoverState.emoji) return;
  const current = items.find((item) => item.emoji === popoverState.emoji);
  if (!current) {
    closePopover();
    return;
  }
  popoverState.total = current.count;
  if (popoverState.expanded && popoverState.users.length > current.count) {
    popoverState.users = popoverState.users.slice(0, current.count);
  }
  const cached = reactionUserCache[popoverState.emoji];
  if (cached) {
    cached.total = current.count;
    if (cached.items.length > current.count) {
      cached.items = cached.items.slice(0, current.count);
    }
  }
}, { deep: true });

watch([() => popoverState.show, () => popoverState.emoji], async ([show]) => {
  if (!show) {
    markReactionFollower(false);
    return;
  }
  await nextTick();
  markReactionFollower(true);
});

onBeforeUnmount(() => {
  markReactionFollower(false);
});
</script>

<template>
  <div v-if="reactionItems.length" class="message-reactions">
    <n-popover
      v-for="reaction in reactionItems"
      :key="reaction.emoji"
      trigger="manual"
      :placement="isMobileUa ? 'top-start' : 'top'"
      content-class="reaction-users-popover"
      :show="popoverState.show && popoverState.emoji === reaction.emoji"
      @clickoutside="closePopover"
    >
      <template #trigger>
        <button
          class="message-reactions__item"
          :class="{ 'message-reactions__item--active': reaction.meReacted }"
          :title="reaction.emoji"
          @click="handleReactionClick(reaction.emoji, reaction.count, $event)"
          @mouseenter="handleHoverOpen(reaction.emoji, reaction.count)"
          @mouseleave="scheduleHoverClose"
          @pointerdown="startLongPress(reaction.emoji, reaction.count, $event)"
          @pointerup="cancelLongPress"
          @pointercancel="cancelLongPress"
          @pointerleave="cancelLongPress"
          @contextmenu.prevent
        >
          <span v-if="reaction.useText" class="message-reactions__emoji-text">{{ reaction.emoji }}</span>
          <img
            v-else
            :src="reaction.url"
            :alt="reaction.emoji"
            :data-emoji="reaction.emoji"
            :data-fallback="reaction.fallbackUrl"
            class="message-reactions__emoji"
            loading="lazy"
            @error="handleImgError"
          />
          <span class="message-reactions__count">{{ reaction.count }}</span>
        </button>
      </template>
      <div
        class="reaction-users-popover__panel"
        @mouseenter="cancelHoverClose"
        @mouseleave="scheduleHoverClose"
      >
        <div class="reaction-users-popover__summary">
          <span class="reaction-users-popover__emoji">
            <span v-if="reaction.useText" class="reaction-users-popover__emoji-text">{{ reaction.emoji }}</span>
            <img
              v-else
              :src="reaction.url"
              :alt="reaction.emoji"
              :data-emoji="reaction.emoji"
              :data-fallback="reaction.fallbackUrl"
              class="reaction-users-popover__emoji-img"
              loading="lazy"
              @error="handleImgError"
            />
          </span>
          <span class="reaction-users-popover__text">{{ summaryText }}</span>
        </div>
        <div v-if="popoverState.error" class="reaction-users-popover__status">{{ popoverState.error }}</div>
        <div v-else-if="popoverState.loading && popoverState.users.length === 0" class="reaction-users-popover__status">加载中…</div>
        <button
          v-if="showAllLink && !popoverState.allLoading"
          class="reaction-users-popover__show-all"
          @click.stop="loadAllUsers"
        >
          点击显示全部人
        </button>
        <div v-else-if="popoverState.allLoading" class="reaction-users-popover__status">加载全部中…</div>
      </div>
    </n-popover>
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
  border-radius: 20px;
  background: var(--sc-bg-elevated, rgba(0, 0, 0, 0.03));
  cursor: pointer;
  font-size: 14px;
  transition: all 0.15s;
  user-select: none;
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

.message-reactions__emoji-text {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  font-size: 16px;
  line-height: 1;
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

:global(.reaction-users-popover),
:global(.v-binder-follower-content.reaction-users-popover) {
  width: min(320px, 86vw);
  padding: 10px 12px;
  border-radius: 20px;
  border: 1px solid var(--chat-border-mute, rgba(15, 23, 42, 0.08));
  background: var(--sc-bg-elevated, #fff);
  color: var(--chat-text-primary, #1f2937);
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.18);
  box-sizing: border-box;
}

:global(.reaction-users-follower) {
  border-radius: 20px;
  --n-border-radius: 20px;
  background: transparent !important;
}

:global(.reaction-users-follower .n-popover) {
  border-radius: 20px;
  overflow: hidden;
}

:global(.reaction-users-follower .n-popover__content) {
  border-radius: 20px;
}

:global([data-display-palette='night']) .reaction-users-popover {
  background: rgba(15, 23, 42, 0.94);
  border-color: rgba(148, 163, 184, 0.35);
  color: #e2e8f0;
  box-shadow: 0 14px 30px rgba(0, 0, 0, 0.5);
}

@media (max-width: 640px) {
  :global(.reaction-users-popover),
  :global(.v-binder-follower-content.reaction-users-popover) {
    width: min(260px, calc(100vw - 24px));
    padding: 8px 10px;
    border-radius: 18px;
  }

  :global(.reaction-users-follower) {
    border-radius: 18px;
    --n-border-radius: 18px;
    background: transparent !important;
  }

  :global(.reaction-users-follower .n-popover) {
    border-radius: 18px;
  }

  :global(.reaction-users-follower .n-popover__content) {
    border-radius: 18px;
  }

  .reaction-users-popover__summary {
    gap: 6px;
  }

  .reaction-users-popover__emoji {
    width: 30px;
    height: 30px;
  }

  .reaction-users-popover__emoji-text {
    font-size: 30px;
  }

  .reaction-users-popover__emoji-img {
    width: 30px;
    height: 30px;
  }

  .reaction-users-popover__text {
    font-size: 12px;
  }
}

.reaction-users-popover__panel {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.reaction-users-popover__summary {
  display: flex;
  align-items: center;
  gap: 8px;
}

.reaction-users-popover__emoji {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 6px;
  background: var(--sc-bg-hover, rgba(15, 23, 42, 0.06));
}

.reaction-users-popover__emoji-text {
  font-size: 36px;
  line-height: 1;
}

.reaction-users-popover__emoji-img {
  width: 36px;
  height: 36px;
}

.reaction-users-popover__text {
  font-size: 13px;
  line-height: 1.4;
  color: inherit;
  word-break: break-word;
}

.reaction-users-popover__status {
  font-size: 12px;
  color: var(--chat-text-secondary, #6b7280);
  padding: 4px 0;
}

.reaction-users-popover__show-all {
  align-self: flex-start;
  border: none;
  padding: 0;
  background: none;
  font-size: 12px;
  color: var(--primary-color, #3b82f6);
  cursor: pointer;
}

.reaction-users-popover__show-all:hover {
  text-decoration: underline;
}

</style>
