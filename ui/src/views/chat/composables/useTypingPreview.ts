import { computed, nextTick, onBeforeUnmount, reactive, ref, watch, type ComputedRef, type Ref } from 'vue';
import { throttle } from 'lodash-es';
import type { AvatarDecoration } from '@/types';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { useDisplayStore } from '@/stores/display';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { isTipTapJson } from '@/utils/tiptap-render';

export interface TypingPreviewOptions {
  chat: ReturnType<typeof useChatStore>;
  user: ReturnType<typeof useUserStore>;
  display: ReturnType<typeof useDisplayStore>;
  inputMode: Ref<'plain' | 'rich'>;
  inputIcMode: Ref<'ic' | 'ooc'>;
  textToSend: Ref<string>;
  isEditing: ComputedRef<boolean>;
  inHistoryMode: ComputedRef<boolean>;
  historyLocked: ComputedRef<boolean>;
  editingIdentityPreviewContext: ComputedRef<any>;
  resolveIdentityVariantShortcutMatch: (...args: any[]) => any;
  resolveIdentityAppearancePreview: (...args: any[]) => any;
  replaceEmojiRemarksForPreview: (text: string) => string;
  cloneAvatarDecorations: (...args: any[]) => AvatarDecoration[] | null | undefined;
  normalizeHexColor: (value: string) => string;
  isContentMeaningful: (mode: 'plain' | 'rich', content: string) => boolean;
  isNearBottom: () => boolean;
  scrollToBottom: () => void;
}

export interface TypingPreviewItem {
  userId: string;
  displayName: string;
  avatar?: string;
  avatarDecorations?: AvatarDecoration[] | null;
  color?: string;
  content: string;
  indicatorOnly: boolean;
  mode: 'typing' | 'editing';
  messageId?: string;
  isTemporary?: boolean;
  tone: 'ic' | 'ooc';
  orderKey: number;
}

export interface EditingPreviewInfo {
  userId: string;
  displayName: string;
  color?: string;
  avatar?: string;
  avatarDecorations?: AvatarDecoration[] | null;
  content: string;
  indicatorOnly: boolean;
  isSelf: boolean;
  isTemporary?: boolean;
  summary: string;
  previewHtml: string;
  tone: 'ic' | 'ooc';
}

export const useTypingPreview = (options: TypingPreviewOptions) => {
  const {
    chat,
    user,
    display,
    inputMode,
    inputIcMode,
    textToSend,
    isEditing,
    inHistoryMode,
    historyLocked,
    editingIdentityPreviewContext,
    resolveIdentityVariantShortcutMatch,
    resolveIdentityAppearancePreview,
    replaceEmojiRemarksForPreview,
    cloneAvatarDecorations,
    normalizeHexColor,
    isContentMeaningful,
    isNearBottom,
    scrollToBottom,
  } = options;
  const typingPreviewViewportRef = ref<HTMLElement | null>(null);

const resolveTypingTone = (typing?: { icMode?: string; ic_mode?: string; tone?: string }): 'ic' | 'ooc' => {
  const raw = typing?.icMode ?? typing?.ic_mode ?? typing?.tone;
  if (typeof raw === 'string' && raw.toLowerCase() === 'ooc') {
    return 'ooc';
  }
  return 'ic';
};

type TypingBroadcastState = 'indicator' | 'content' | 'silent';

const typingPreviewStorageKey = 'sealchat.typingPreviewMode';
const legacyTypingPreviewKey = 'sealchat.typingPreviewEnabled';
const resolveTypingPreviewMode = (): TypingBroadcastState => {
  const stored = localStorage.getItem(typingPreviewStorageKey);
  if (stored === 'indicator' || stored === 'content' || stored === 'silent') {
    return stored as TypingBroadcastState;
  }
  if (stored === 'on') {
    return 'content';
  }
  if (stored === 'off') {
    return 'indicator';
  }
  const legacy = localStorage.getItem(legacyTypingPreviewKey);
  if (legacy === 'true') {
    return 'content';
  }
  if (legacy === 'false') {
    return 'indicator';
  }
  return 'content';
};
const typingPreviewMode = ref<TypingBroadcastState>(resolveTypingPreviewMode());
if (localStorage.getItem(legacyTypingPreviewKey) !== null) {
  localStorage.removeItem(legacyTypingPreviewKey);
}
const typingPreviewActive = ref(false);
const typingPreviewList = ref<TypingPreviewItem[]>([]);
let typingPreviewOrderSeq = Date.now();
const previewOrderMin = 1e-6;
const selfPreviewOrderKey = ref<number>(Number.MAX_SAFE_INTEGER);
const selfPreviewOrderModified = ref(false);
const draftStartedAtMs = ref<number | null>(null);
const resetSelfPreviewOrder = () => {
	selfPreviewOrderKey.value = Number.MAX_SAFE_INTEGER;
	selfPreviewOrderModified.value = false;
};
const resetDraftOrderContext = () => {
  draftStartedAtMs.value = null;
  resetSelfPreviewOrder();
};
const typingPreviewRowRefs = new Map<string, HTMLElement>();
const typingPreviewItemKey = (preview: TypingPreviewItem | null | undefined) =>
  preview ? `${preview.userId || ''}-${preview.mode}` : '';
const registerTypingPreviewRow = (el: HTMLElement | null, preview: TypingPreviewItem) => {
  const key = typingPreviewItemKey(preview);
  if (!key) {
    return;
  }
  if (el) {
    typingPreviewRowRefs.set(key, el);
  } else {
    typingPreviewRowRefs.delete(key);
  }
};
const getPreviewOrderValue = (item?: TypingPreviewItem | null) => {
  if (!item) {
    return null;
  }
  const value = typeof item.orderKey === 'number' ? item.orderKey : Number.NaN;
  return Number.isFinite(value) && value > 0 ? value : null;
};
const derivePreviewOrderValue = (list: TypingPreviewItem[], index: number, fallback: number) => {
  const prevOrder = getPreviewOrderValue(list[index - 1]);
  const nextOrder = getPreviewOrderValue(list[index + 1]);
  if (prevOrder !== null && nextOrder !== null) {
    return (prevOrder + nextOrder) / 2;
  }
  if (prevOrder !== null) {
    return prevOrder + 1;
  }
  if (nextOrder !== null) {
    return nextOrder > 1 ? nextOrder - 1 : nextOrder / 2;
  }
  return fallback;
};
interface PreviewDragState {
  pointerId: number | null;
  activeKey: string | null;
  overKey: string | null;
  position: 'before' | 'after' | null;
  startY: number;
  initialOrderKey: number | null;
  handleEl: HTMLElement | null;
  initialModified: boolean;
}
const previewDragState = reactive<PreviewDragState>({
  pointerId: null,
  activeKey: null,
  overKey: null,
  position: null,
  startY: 0,
  initialOrderKey: null,
  handleEl: null,
  initialModified: false,
});
const resetPreviewDragState = () => {
  previewDragState.pointerId = null;
  previewDragState.activeKey = null;
  previewDragState.overKey = null;
  previewDragState.position = null;
  previewDragState.startY = 0;
  previewDragState.initialOrderKey = null;
  previewDragState.handleEl = null;
  previewDragState.initialModified = false;
};
const updateSelfPreviewOrderKey = (orderKey: number | null, markModified = false) => {
  if (orderKey === null || !Number.isFinite(orderKey)) {
    return;
  }
  const normalized = orderKey > 0 ? orderKey : previewOrderMin;
  selfPreviewOrderKey.value = normalized;
  if (markModified) {
    selfPreviewOrderModified.value = true;
  }
  typingPreviewList.value = typingPreviewList.value.map((item) => {
    if (item.userId === selfPreviewUserId.value && item.mode === 'typing') {
      return { ...item, orderKey: normalized };
    }
    return item;
  });
};
const getPreviewTargetIndex = (list: TypingPreviewItem[], overKey: string | null, position: 'before' | 'after' | null) => {
  if (!overKey || !position) {
    return null;
  }
  const overIndex = list.findIndex((item) => typingPreviewItemKey(item) === overKey);
  if (overIndex < 0) {
    return null;
  }
  if (position === 'before') {
    return overIndex;
  }
  return overIndex + 1;
};
const applyPreviewDragReorder = () => {
  const activeKey = previewDragState.activeKey;
  if (!activeKey) {
    return;
  }
  const previews = typingPreviewItems.value.slice();
  const fromIndex = previews.findIndex((item) => typingPreviewItemKey(item) === activeKey);
  if (fromIndex < 0) {
    return;
  }
  const [activeItem] = previews.splice(fromIndex, 1);
  const targetIndex = getPreviewTargetIndex(previews, previewDragState.overKey, previewDragState.position);
	if (targetIndex === null) {
		previews.splice(fromIndex, 0, activeItem);
		updateSelfPreviewOrderKey(previewDragState.initialOrderKey);
		selfPreviewOrderModified.value = previewDragState.initialModified;
		return;
	}
  const clampedTarget = Math.min(Math.max(targetIndex, 0), previews.length);
  previews.splice(clampedTarget, 0, activeItem);
  const fallback = getPreviewOrderValue(activeItem) ?? Date.now();
  const derived = derivePreviewOrderValue(previews, clampedTarget, fallback);
	updateSelfPreviewOrderKey(derived, true);
	broadcastTypingOrderChange();
};
const detachPreviewDragListeners = () => {
  window.removeEventListener('pointermove', onPreviewDragPointerMove);
  window.removeEventListener('pointerup', onPreviewDragPointerUp);
  window.removeEventListener('pointercancel', onPreviewDragPointerCancel);
};
const cancelPreviewDrag = () => {
	detachPreviewDragListeners();
	if (previewDragState.initialOrderKey !== null) {
		updateSelfPreviewOrderKey(previewDragState.initialOrderKey);
	}
	selfPreviewOrderModified.value = previewDragState.initialModified;
	if (previewDragState.handleEl && previewDragState.pointerId !== null) {
		try {
			previewDragState.handleEl.releasePointerCapture?.(previewDragState.pointerId);
		} catch {
			// ignore
		}
	}
	document.body.style.userSelect = '';
	resetPreviewDragState();
	broadcastTypingOrderChange.flush();
};
const finalizePreviewDrag = () => {
	detachPreviewDragListeners();
	if (previewDragState.handleEl && previewDragState.pointerId !== null) {
		try {
			previewDragState.handleEl.releasePointerCapture?.(previewDragState.pointerId);
		} catch {
			// ignore
		}
	}
	document.body.style.userSelect = '';
	resetPreviewDragState();
	broadcastTypingOrderChange.flush();
};
const updatePreviewDragTarget = (clientY: number) => {
  const activeKey = previewDragState.activeKey;
  if (!activeKey) {
    return;
  }
  const previews = typingPreviewItems.value;
  let matched = false;
  for (const preview of previews) {
    const key = typingPreviewItemKey(preview);
    if (!key || key === activeKey) {
      continue;
    }
    const el = typingPreviewRowRefs.get(key);
    if (!el) {
      continue;
    }
    const rect = el.getBoundingClientRect();
    const mid = rect.top + rect.height / 2;
    if (clientY <= mid) {
      previewDragState.overKey = key;
      previewDragState.position = 'before';
      matched = true;
      break;
    }
    if (clientY < rect.bottom) {
      previewDragState.overKey = key;
      previewDragState.position = 'after';
      matched = true;
      break;
    }
  }
  if (!matched && previews.length > 0) {
    const last = previews[previews.length - 1];
    const lastKey = typingPreviewItemKey(last);
    if (lastKey) {
      previewDragState.overKey = lastKey;
      previewDragState.position = 'after';
      matched = true;
    }
  }
  if (!matched) {
    previewDragState.overKey = null;
    previewDragState.position = null;
  }
};
const onPreviewDragPointerMove = (event: PointerEvent) => {
  if (event.pointerId !== previewDragState.pointerId) {
    return;
  }
  event.preventDefault();
  updatePreviewDragTarget(event.clientY);
  applyPreviewDragReorder();
};
const onPreviewDragPointerUp = (event: PointerEvent) => {
  if (event.pointerId !== previewDragState.pointerId) {
    return;
  }
  event.preventDefault();
  finalizePreviewDrag();
};
const onPreviewDragPointerCancel = (event: PointerEvent) => {
  if (event.pointerId !== previewDragState.pointerId) {
    return;
  }
  event.preventDefault();
  cancelPreviewDrag();
};
const getTypingOrderKey = (userId: string, mode: 'typing' | 'editing') => {
  const existing = typingPreviewList.value.find((item) => item.userId === userId && item.mode === mode);
  if (existing && Number.isFinite(existing.orderKey) && existing.orderKey > 0) {
    return existing.orderKey;
  }
  if (!Number.isFinite(typingPreviewOrderSeq) || typingPreviewOrderSeq <= 0) {
    typingPreviewOrderSeq = Date.now();
  }
  const next = Math.max(typingPreviewOrderSeq, previewOrderMin);
  typingPreviewOrderSeq += 1;
  return next;
};
const typingPreviewItemClass = (preview: TypingPreviewItem) => [
	'typing-preview-item',
	'message-row',
	`message-row--tone-${preview.tone}`,
	`typing-preview-item--${preview.tone}`,
	{
		'typing-preview-item--indicator': preview.indicatorOnly,
		'typing-preview-item--dragging': typingPreviewItemKey(preview) === previewDragState.activeKey,
	},
];
const typingPreviewSurfaceClass = (preview: TypingPreviewItem) => [
  'typing-preview-surface',
  'message-row__surface',
  `message-row__surface--tone-${preview.tone}`,
];
const typingPreviewHandleClass = (preview: TypingPreviewItem) => {
  const classes = ['message-row__handle'];
  const key = typingPreviewItemKey(preview);
  const isSelfPreview = preview.userId === selfPreviewUserId.value;
	if (isSelfPreview) {
    classes.push('typing-preview-handle');
    if (key && key === previewDragState.activeKey) {
      classes.push('typing-preview-handle--dragging');
    }
  } else {
    classes.push('message-row__handle--placeholder');
  }
  return classes;
};
const canDragTypingPreview = (preview: TypingPreviewItem) => preview.userId === selfPreviewUserId.value;
const onPreviewDragHandlePointerDown = (event: PointerEvent, preview: TypingPreviewItem) => {
  if (!canDragTypingPreview(preview)) {
    return;
  }
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return;
  }
  const key = typingPreviewItemKey(preview);
  if (!key) {
    return;
  }
  const handleEl = event.currentTarget as HTMLElement | null;
  if (handleEl) {
    previewDragState.handleEl = handleEl;
    try {
      handleEl.setPointerCapture?.(event.pointerId);
    } catch {
      // ignore capture errors
    }
  }
  previewDragState.pointerId = event.pointerId;
  previewDragState.activeKey = key;
  previewDragState.overKey = key;
  previewDragState.position = 'after';
  previewDragState.startY = event.clientY;
  previewDragState.initialOrderKey = getPreviewOrderValue(preview) ?? selfPreviewOrderKey.value;
  previewDragState.initialModified = selfPreviewOrderModified.value;
  document.body.style.userSelect = 'none';
  updatePreviewDragTarget(event.clientY);
  window.addEventListener('pointermove', onPreviewDragPointerMove);
  window.addEventListener('pointerup', onPreviewDragPointerUp);
  window.addEventListener('pointercancel', onPreviewDragPointerCancel);
  event.preventDefault();
};
const inputPreviewEnabled = computed(() => display.settings.showInputPreview !== false);
const autoScrollTypingPreviewAlways = computed(() => display.settings.autoScrollTypingPreview === true);
const shouldObserveTypingPreview = computed(() => (
  inputPreviewEnabled.value
  && (autoScrollTypingPreviewAlways.value || (!inHistoryMode.value && !historyLocked.value))
));
const activeIdentityForPreview = computed(() => {
  if (editingIdentityPreviewContext.value) {
    return editingIdentityPreviewContext.value.identity;
  }
  return chat.getActiveIdentity(chat.curChannel?.id || '');
});
const activeIdentityVariantShortcutContext = computed(() => {
  const rawDraft = textToSend.value;
  const channelId = chat.curChannel?.id || '';
  const identity = activeIdentityForPreview.value;
  const fallbackVariant = editingIdentityPreviewContext.value
    ? editingIdentityPreviewContext.value.variant
    : (identity ? chat.getActiveIdentityVariant(channelId, identity.id) : null);
  if (isEditing.value || inputMode.value !== 'plain' || !channelId || !identity) {
    return {
      draftContent: rawDraft,
      variant: fallbackVariant,
      matched: false,
    };
  }
  const trigger = display.settings.identityVariantQuickSwitchTrigger || '=';
  const shortcutResult = resolveIdentityVariantShortcutMatch(
    rawDraft,
    identity,
    chat.getIdentityVariants(channelId, identity.id),
    trigger,
  );
  if (shortcutResult?.matched) {
    return {
      draftContent: shortcutResult.restContent,
      variant: shortcutResult.matched,
      matched: true,
    };
  }
  if (shortcutResult?.resetToDefault) {
    return {
      draftContent: shortcutResult.restContent,
      variant: null,
      matched: true,
    };
  }
  return {
    draftContent: rawDraft,
    variant: fallbackVariant,
    matched: false,
  };
});
const activeIdentityVariantForPreview = computed(() => {
  if (editingIdentityPreviewContext.value) {
    return editingIdentityPreviewContext.value.variant;
  }
  return activeIdentityVariantShortcutContext.value.variant;
});
const activeIdentityAppearanceForPreview = computed(() => {
  if (editingIdentityPreviewContext.value) {
    return editingIdentityPreviewContext.value.appearance;
  }
  return resolveIdentityAppearancePreview(activeIdentityForPreview.value, activeIdentityVariantForPreview.value);
});
const activeIdentityAppearancePreviewSignature = computed(() => {
  const appearance = activeIdentityAppearanceForPreview.value;
  return [
    appearance?.identityId || '',
    appearance?.variantId || '',
    appearance?.displayName || '',
    appearance?.color || '',
    appearance?.avatarAttachmentId || '',
    JSON.stringify(appearance?.avatarDecorations || []),
    appearance?.isTemporary ? '1' : '0',
  ].join('__');
});
const effectiveIdentityVariantForEmojiPanel = computed(() => activeIdentityVariantForPreview.value);
const selfPreviewUserId = computed(() => user.info?.id || '__self__');
const isTypingPreviewVisibleForCurrentFilter = (tone: 'ic' | 'ooc') => {
  const filter = chat.filterState.icFilter;
  if (filter === 'ic') {
    return tone === 'ic';
  }
  if (filter === 'ooc') {
    return tone === 'ooc';
  }
  return true;
};
const typingPreviewItems = computed(() =>
  typingPreviewList.value
    .filter((item) => item.mode === 'typing' && isTypingPreviewVisibleForCurrentFilter(item.tone))
    .slice()
    .sort((a, b) => a.orderKey - b.orderKey),
);
const selfTypingPreview = computed(() =>
  typingPreviewItems.value.find((item) => item.userId === selfPreviewUserId.value && item.mode === 'typing') || null,
);
const selfTypingPreviewSignature = computed(() => {
  if (!selfTypingPreview.value) {
    return '';
  }
  return `${selfTypingPreview.value.content}__${selfTypingPreview.value.indicatorOnly ? '1' : '0'}`;
});
const hasSelfTypingPreview = computed(() =>
  typingPreviewItems.value.some((item) => item.userId === selfPreviewUserId.value && item.mode === 'typing'),
);

const selfTypingPreviewKey = computed(() =>
  selfPreviewUserId.value ? `${selfPreviewUserId.value}-typing` : '',
);
let selfPreviewResizeObserver: ResizeObserver | null = null;
let selfPreviewObservedEl: HTMLElement | null = null;
let lastSelfPreviewHeight = 0;
let pendingSelfPreviewScroll = false;

const disconnectSelfPreviewObserver = () => {
  if (selfPreviewResizeObserver && selfPreviewObservedEl) {
    selfPreviewResizeObserver.unobserve(selfPreviewObservedEl);
  }
  selfPreviewObservedEl = null;
  lastSelfPreviewHeight = 0;
};

const disposeSelfPreviewObserver = () => {
  disconnectSelfPreviewObserver();
  if (selfPreviewResizeObserver) {
    selfPreviewResizeObserver.disconnect();
    selfPreviewResizeObserver = null;
  }
};

const shouldAutoScrollTypingPreview = () => {
  if (!inputPreviewEnabled.value) {
    return false;
  }
  if (autoScrollTypingPreviewAlways.value) {
    return true;
  }
  if (inHistoryMode.value || historyLocked.value) {
    return false;
  }
  return isNearBottom();
};

const scheduleSelfPreviewAutoScroll = () => {
  if (pendingSelfPreviewScroll) {
    return;
  }
  if (!shouldAutoScrollTypingPreview()) {
    return;
  }
  pendingSelfPreviewScroll = true;
  nextTick(() => {
    requestAnimationFrame(() => {
      pendingSelfPreviewScroll = false;
      if (!shouldAutoScrollTypingPreview()) {
        return;
      }
      scrollToBottom();
    });
  });
};

const ensureSelfPreviewObserver = async () => {
  if (!shouldObserveTypingPreview.value) {
    disconnectSelfPreviewObserver();
    return;
  }
  const key = selfTypingPreviewKey.value;
  if (!key) {
    disconnectSelfPreviewObserver();
    return;
  }
  await nextTick();
  const el = typingPreviewRowRefs.get(key);
  if (!el) {
    disconnectSelfPreviewObserver();
    return;
  }
  if (selfPreviewObservedEl === el) {
    return;
  }
  disconnectSelfPreviewObserver();
  selfPreviewObservedEl = el;
  lastSelfPreviewHeight = el.getBoundingClientRect().height;
  if (!selfPreviewResizeObserver) {
    selfPreviewResizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry || entry.target !== selfPreviewObservedEl) {
        return;
      }
      const nextHeight = entry.contentRect.height;
      if (nextHeight > lastSelfPreviewHeight) {
        scheduleSelfPreviewAutoScroll();
      }
      lastSelfPreviewHeight = nextHeight;
    });
  }
  selfPreviewResizeObserver.observe(el);
};

watch(
  [typingPreviewItems, selfPreviewUserId, shouldObserveTypingPreview],
  () => {
    void ensureSelfPreviewObserver();
  },
  { flush: 'post' },
);

watch(
  hasSelfTypingPreview,
  (hasPreview, prevHasPreview) => {
    if (!hasPreview || prevHasPreview) {
      return;
    }
    scheduleSelfPreviewAutoScroll();
  },
  { flush: 'post' },
);

watch(
  selfTypingPreviewSignature,
  (next, prev) => {
    if (!next || next === prev) {
      return;
    }
    scheduleSelfPreviewAutoScroll();
  },
  { flush: 'post' },
);

// 监听整个 typing-preview-viewport 容器的高度变化（用于他人的实时广播）
let typingViewportResizeObserver: ResizeObserver | null = null;
let lastTypingViewportHeight = 0;

const shouldAutoScrollRemoteTyping = () => {
  if (inHistoryMode.value || historyLocked.value) {
    return false;
  }
  return true;
};

const scheduleRemotePreviewAutoScroll = () => {
  if (!shouldAutoScrollRemoteTyping()) {
    return;
  }
  nextTick(() => {
    requestAnimationFrame(() => {
      if (!shouldAutoScrollRemoteTyping()) {
        return;
      }
      scrollToBottom();
    });
  });
};

const setupTypingViewportObserver = () => {
  const el = typingPreviewViewportRef.value;
  if (!el) {
    return;
  }
  if (typingViewportResizeObserver) {
    typingViewportResizeObserver.disconnect();
  }
  lastTypingViewportHeight = el.getBoundingClientRect().height;
  typingViewportResizeObserver = new ResizeObserver((entries) => {
    const entry = entries[0];
    if (!entry) {
      return;
    }
    const nextHeight = entry.contentRect.height;
    if (nextHeight > lastTypingViewportHeight) {
      scheduleRemotePreviewAutoScroll();
    }
    lastTypingViewportHeight = nextHeight;
  });
  typingViewportResizeObserver.observe(el);
  scheduleRemotePreviewAutoScroll();
};

const disposeTypingViewportObserver = () => {
  if (typingViewportResizeObserver) {
    typingViewportResizeObserver.disconnect();
    typingViewportResizeObserver = null;
  }
  lastTypingViewportHeight = 0;
};

watch(
  typingPreviewViewportRef,
  (el) => {
    if (el) {
      setupTypingViewportObserver();
    } else {
      disposeTypingViewportObserver();
    }
  },
  { flush: 'post' },
);

const resolveSelfPreviewDisplayName = () => {
  const appearance = activeIdentityAppearanceForPreview.value;
  if (appearance?.displayName) {
    return appearance.displayName;
  }
  return user.info?.nick || (user.info as any)?.name || '我';
};
const resolveSelfPreviewAvatar = () => {
  const appearance = activeIdentityAppearanceForPreview.value;
  if (appearance?.avatarAttachmentId) {
    return resolveAttachmentUrl(appearance.avatarAttachmentId);
  }
  return chat.curMember?.avatar || user.info?.avatar || '';
};
const removeSelfTypingPreview = () => {
  const userId = selfPreviewUserId.value;
  if (userId) {
    removeTypingPreview(userId, 'typing');
  }
};
const syncSelfTypingPreview = () => {
  if (!inputPreviewEnabled.value || isEditing.value) {
    removeSelfTypingPreview();
    return;
  }
  const draft = activeIdentityVariantShortcutContext.value.draftContent;
  if (!isContentMeaningful(inputMode.value, draft)) {
    removeSelfTypingPreview();
    return;
  }
  const displayName = resolveSelfPreviewDisplayName();
  const avatar = resolveSelfPreviewAvatar();
  const normalizedColor = activeIdentityAppearanceForPreview.value?.color
    ? normalizeHexColor(activeIdentityAppearanceForPreview.value.color || '') || undefined
    : undefined;
  const tone = inputIcMode.value || 'ic';
  if (!isTypingPreviewVisibleForCurrentFilter(tone)) {
    removeSelfTypingPreview();
    return;
  }
  let previewContent = draft;
  if (inputMode.value !== 'rich') {
    const normalized = replaceEmojiRemarksForPreview(draft);
    previewContent = normalized.length > 500 ? normalized.slice(0, 500) : normalized;
  }
  const payload: TypingPreviewItem = {
    userId: selfPreviewUserId.value,
    displayName,
    avatar,
    avatarDecorations: cloneAvatarDecorations(activeIdentityAppearanceForPreview.value?.avatarDecorations),
    color: normalizedColor,
    content: previewContent,
    indicatorOnly: false,
    mode: 'typing',
    tone,
    messageId: undefined,
    isTemporary: Boolean(activeIdentityAppearanceForPreview.value?.isTemporary),
    orderKey: 0,
  };
  upsertTypingPreview(payload);
};
watch(selfPreviewUserId, (next, prev) => {
	if (prev && prev !== next) {
		removeTypingPreview(prev, 'typing');
		resetSelfPreviewOrder();
	}
	syncSelfTypingPreview();
});
let lastTypingChannelId = '';
let lastTypingWhisperTargetId: string | null = null;

const upsertTypingPreview = (item: TypingPreviewItem) => {
  const isSelfPreview = item.userId === selfPreviewUserId.value;
  let orderKey: number;
  if (isSelfPreview) {
    const existing = typingPreviewList.value.find((preview) => preview.userId === item.userId && preview.mode === item.mode);
    if (existing && Number.isFinite(existing.orderKey) && existing.orderKey > 0) {
      orderKey = existing.orderKey;
    } else if (Number.isFinite(selfPreviewOrderKey.value) && selfPreviewOrderKey.value > 0) {
      orderKey = selfPreviewOrderKey.value;
    } else {
      orderKey = Number.MAX_SAFE_INTEGER;
    }
		selfPreviewOrderKey.value = orderKey;
	} else {
		if (typeof item.orderKey === 'number' && Number.isFinite(item.orderKey) && item.orderKey > 0) {
			orderKey = item.orderKey;
		} else {
			orderKey = getTypingOrderKey(item.userId, item.mode);
		}
	}
  const existingIndex = typingPreviewList.value.findIndex((i) => i.userId === item.userId && i.mode === item.mode);
  if (existingIndex >= 0) {
    typingPreviewList.value.splice(existingIndex, 1, { ...item, orderKey });
  } else {
    typingPreviewList.value.push({ ...item, orderKey });
  }
};

const removeTypingPreview = (userId?: string, mode: 'typing' | 'editing' = 'typing') => {
	if (!userId) {
		return;
	}
	typingPreviewList.value = typingPreviewList.value.filter((item) => !(item.userId === userId && item.mode === mode));
};

const resetTypingPreview = () => {
	typingPreviewList.value = [];
	typingPreviewOrderSeq = Date.now();
	resetSelfPreviewOrder();
	typingPreviewRowRefs.clear();
};

const resolveCurrentWhisperTargetId = (): string | null => chat.whisperTargets[0]?.id || null;

const sendTypingUpdate = throttle(
	(state: TypingBroadcastState, content: string, channelId: string, options?: { whisperTo?: string | null; orderKey?: number }) => {
		const targetId = options?.whisperTo ?? resolveCurrentWhisperTargetId();
		const icMode = chat.icMode === 'ooc' ? 'ooc' : 'ic';
		const extra: {
			whisperTo?: string;
			icMode: 'ic' | 'ooc';
			orderKey?: number;
			identityId?: string;
			identityVariantId?: string;
		} = {
			icMode,
			identityId: activeIdentityForPreview.value?.id || undefined,
			identityVariantId: activeIdentityVariantForPreview.value?.id || undefined,
		};
		if (targetId) {
			extra.whisperTo = targetId;
		}
		if (typeof options?.orderKey === 'number' && Number.isFinite(options.orderKey) && options.orderKey > 0) {
			extra.orderKey = options.orderKey;
		}
		lastTypingWhisperTargetId = targetId ?? null;
		chat.messageTyping(state, content, channelId, extra);
	},
	800,
	{ leading: true, trailing: true },
);
const broadcastTypingOrderChange = throttle(
	() => {
		if (!typingPreviewActive.value || !chat.curChannel?.id) {
			return;
		}
		emitTypingPreview();
		sendTypingUpdate.flush();
	},
	250,
	{ leading: false, trailing: true },
);

const stopTypingPreviewNow = () => {
  sendTypingUpdate.cancel();
  if (typingPreviewActive.value && lastTypingChannelId) {
    const icMode = chat.icMode === 'ooc' ? 'ooc' : 'ic';
    const extra: { whisperTo?: string; icMode: 'ic' | 'ooc' } = lastTypingWhisperTargetId
      ? { whisperTo: lastTypingWhisperTargetId, icMode }
      : { icMode };
    chat.messageTyping('silent', '', lastTypingChannelId, extra);
  }
  typingPreviewActive.value = false;
  lastTypingChannelId = '';
  lastTypingWhisperTargetId = null;
  removeSelfTypingPreview();
};

const editingPreviewActive = ref(false);
let lastEditingChannelId = '';
let lastEditingMessageId = '';

let lastEditingWhisperTargetId: string | null = null;

const sendEditingPreview = throttle((channelId: string, messageId: string, content: string) => {
  const state = typingPreviewMode.value;
  if (state === 'silent') {
    return;
  }
  const whisperTargetId = chat.editing?.whisperTargetId || resolveCurrentWhisperTargetId();
  const icMode = chat.editing?.icMode === 'ooc' ? 'ooc' : 'ic';
  const extra: {
    mode: 'editing';
    messageId: string;
    whisperTo?: string;
    icMode: 'ic' | 'ooc';
    identityId?: string;
    identityVariantId?: string;
  } = {
    mode: 'editing',
    messageId,
    icMode,
    identityId: chat.editing?.identityId || undefined,
    identityVariantId: chat.editing?.identityVariantId || undefined,
  };
  if (whisperTargetId) {
    extra.whisperTo = whisperTargetId;
  }
  chat.messageTyping(state, state === 'content' ? content : '', channelId, extra);
  editingPreviewActive.value = true;
  lastEditingChannelId = channelId;
  lastEditingMessageId = messageId;
  lastEditingWhisperTargetId = whisperTargetId ?? null;
}, 400, { leading: true, trailing: true });

const stopEditingPreviewNow = () => {
  sendEditingPreview.cancel();
  if (editingPreviewActive.value && lastEditingChannelId && lastEditingMessageId) {
    const icMode = chat.editing?.icMode === 'ooc' ? 'ooc' : 'ic';
    const extra: Record<string, any> = { mode: 'editing', messageId: lastEditingMessageId, icMode };
    if (lastEditingWhisperTargetId) {
      extra.whisperTo = lastEditingWhisperTargetId;
    }
    chat.messageTyping('silent', '', lastEditingChannelId, extra);
  }
  editingPreviewActive.value = false;
  lastEditingChannelId = '';
  lastEditingMessageId = '';
  lastEditingWhisperTargetId = null;
};

const emitTypingPreview = () => {
  if (chat.connectState !== 'connected') return;
  const channelId = chat.curChannel?.id;
  if (!channelId) return;

  if (isEditing.value) {
    emitEditingPreview();
    return;
  }

  if (typingPreviewMode.value === 'silent') {
    stopTypingPreviewNow();
    return;
  }

  let raw = inputMode.value === 'plain'
    ? activeIdentityVariantShortcutContext.value.draftContent
    : textToSend.value;
  const canBroadcastIndicatorWithoutContent = inputMode.value === 'plain'
    && activeIdentityVariantShortcutContext.value.matched;

  if (inputMode.value === 'rich') {
    try {
      const json = JSON.parse(raw);
      if (!json.content || json.content.length === 0) {
        stopTypingPreviewNow();
        return;
      }
    } catch {
      stopTypingPreviewNow();
      return;
    }
  } else {
    if (raw.trim().length === 0) {
      if (!canBroadcastIndicatorWithoutContent) {
        stopTypingPreviewNow();
        return;
      }
      raw = '';
    }
    raw = replaceEmojiRemarksForPreview(raw);
  }

  typingPreviewActive.value = true;
  lastTypingChannelId = channelId;

  // 富文本模式不截断 JSON，否则会破坏 JSON 结构导致无法渲染
  const truncated = inputMode.value === 'rich' ? raw : (raw.length > 3000 ? raw.slice(0, 3000) : raw);
  const content = typingPreviewMode.value === 'content' ? truncated : '';
	const orderKeyForBroadcast = Number.isFinite(selfPreviewOrderKey.value)
		? selfPreviewOrderKey.value
		: undefined;
	sendTypingUpdate(typingPreviewMode.value, content, channelId, {
		whisperTo: resolveCurrentWhisperTargetId(),
		orderKey: orderKeyForBroadcast,
	});
};

const emitEditingPreview = () => {
  if (!chat.editing || chat.connectState !== 'connected') {
    return;
  }
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    return;
  }
  const messageId = chat.editing.messageId;
  const raw = chat.editing.draft || '';
  // 富文本模式不截断 JSON，否则会破坏 JSON 结构导致无法渲染
  const isRichMode = chat.editing.mode === 'rich' || isTipTapJson(raw);
  const truncated = isRichMode ? raw : (raw.length > 3000 ? raw.slice(0, 3000) : raw);
  sendEditingPreview(channelId, messageId, truncated);
};

const typingPreviewTooltip = computed(() => {
  switch (typingPreviewMode.value) {
    case 'indicator':
      return '当前：实时广播关闭（仅显示“正在输入”提示）。点击开启实时广播';
    case 'content':
      return '当前：实时广播开启。点击切换为沉默广播';
    case 'silent':
      return '当前：实时广播沉默。点击恢复指示模式';
    default:
      return '调整实时广播状态';
  }
});

const toggleTypingPreview = () => {
  if (typingPreviewMode.value === 'indicator') {
    typingPreviewMode.value = 'content';
    emitTypingPreview();
    return;
  }
  if (typingPreviewMode.value === 'content') {
    typingPreviewMode.value = 'silent';
    return;
  }
  typingPreviewMode.value = 'indicator';
  emitTypingPreview();
};

const typingToggleClass = computed(() => ({
  'typing-toggle--indicator': typingPreviewMode.value === 'indicator',
  'typing-toggle--content': typingPreviewMode.value === 'content',
  'typing-toggle--silent': typingPreviewMode.value === 'silent',
}));

  const handleTypingPreviewEvent = (e?: any) => {
    if (!e?.channel || e.channel.id !== chat.curChannel?.id) {
      return;
    }
    const typingUserId = e.user?.id;
    if (!typingUserId || typingUserId === user.info.id) {
      return;
    }
    const mode = e.typing?.mode === 'editing' ? 'editing' : 'typing';
    const identity = e.member?.identity;
    const identityColor = identity ? normalizeHexColor(identity.color || '') : '';
    const identityAvatar = identity?.avatarAttachmentId
      ? resolveAttachmentUrl(identity.avatarAttachmentId)
      : '';
    const debugEnabled =
      typeof window !== 'undefined' &&
      (window as any).__SC_DEBUG_TYPING__ === true;
    if (debugEnabled) {
      console.debug(
        '[typing-preview]',
        'user=', typingUserId,
        'mode=', mode,
        // @ts-expect-error preserve original debug evaluation order
        'state=', typingState,
        'messageId=', e.typing?.messageId,
        'identityId=', identity?.id || '(none)',
        'identityName=', identity?.displayName || '(none)',
      );
    }
    const typingState: TypingBroadcastState = (() => {
      const candidate = (e.typing?.state || '').toLowerCase();
      switch (candidate) {
        case 'content':
        case 'on':
          return 'content';
        case 'silent':
          return 'silent';
        case 'indicator':
        case 'off':
          return 'indicator';
        default:
          if (typeof e.typing?.enabled === 'boolean') {
            return e.typing.enabled ? 'content' : 'indicator';
          }
          return 'indicator';
      }
    })();
    if (typingState === 'silent') {
      removeTypingPreview(typingUserId, mode);
      return;
    }
    const displayName =
      (identity?.displayName && identity.displayName.trim()) ||
      e.member?.nick ||
      e.user?.nick ||
      '未知成员';
    const avatar = identityAvatar || e.member?.avatar || e.user?.avatar || '';
    upsertTypingPreview({
      userId: typingUserId,
      displayName,
      avatar,
      avatarDecorations: cloneAvatarDecorations(identity?.avatarDecorations, identity?.avatarDecoration),
      color: identityColor,
      content: typingState === 'content' ? (e.typing?.content || '') : '',
      indicatorOnly: typingState !== 'content' || !e.typing?.content,
      mode,
      messageId: e.typing?.messageId,
      isTemporary: Boolean(identity?.isTemporary),
      tone: resolveTypingTone(e.typing),
      orderKey: typeof e.typing?.orderKey === 'number' ? e.typing.orderKey : Number.NaN,
    });
  };
  chatEvent.off('typing-preview', '*');
  chatEvent.on('typing-preview', handleTypingPreviewEvent as any);

  watch(typingPreviewMode, (mode) => {
    localStorage.setItem(typingPreviewStorageKey, mode);
    if (mode === 'silent') {
      stopTypingPreviewNow();
      stopEditingPreviewNow();
      return;
    }
    if (typingPreviewActive.value && lastTypingChannelId) {
      const raw = inputMode.value === 'plain'
        ? activeIdentityVariantShortcutContext.value.draftContent
        : textToSend.value;
      const canBroadcastIndicatorWithoutContent = inputMode.value === 'plain'
        && activeIdentityVariantShortcutContext.value.matched;
      if (raw.trim().length > 0 || canBroadcastIndicatorWithoutContent) {
        const isRich = inputMode.value === 'rich' || isTipTapJson(raw);
        const truncated = isRich ? raw : (raw.length > 3000 ? raw.slice(0, 3000) : raw);
        sendTypingUpdate.cancel();
        const content = mode === 'content' ? truncated : '';
        const whisperId = resolveCurrentWhisperTargetId();
        const extra = {
          whisperTo: whisperId || undefined,
          identityId: activeIdentityForPreview.value?.id || undefined,
          identityVariantId: activeIdentityVariantForPreview.value?.id || undefined,
        };
        lastTypingWhisperTargetId = whisperId ?? null;
        chat.messageTyping(mode, content, lastTypingChannelId, extra);
      } else {
        stopTypingPreviewNow();
      }
    }
    if (isEditing.value) {
      emitEditingPreview();
      return;
    }
  });

  onBeforeUnmount(() => {
    stopTypingPreviewNow();
    stopEditingPreviewNow();
    resetTypingPreview();
    broadcastTypingOrderChange.cancel();
    sendTypingUpdate.cancel();
    sendEditingPreview.cancel();
    detachPreviewDragListeners();
    disposeSelfPreviewObserver();
    disposeTypingViewportObserver();
    chatEvent.off('typing-preview', handleTypingPreviewEvent as any);
    document.body.style.userSelect = '';
  });

  return {
    typingPreviewMode,
    typingPreviewActive,
    typingPreviewList,
    typingPreviewViewportRef,
    typingPreviewItems,
    typingPreviewTooltip,
    typingToggleClass,
    typingPreviewItemClass,
    typingPreviewSurfaceClass,
    typingPreviewHandleClass,
    canDragTypingPreview,
    onPreviewDragHandlePointerDown,
    registerTypingPreviewRow,
    emitTypingPreview,
    emitEditingPreview,
    stopTypingPreviewNow,
    stopEditingPreviewNow,
    sendTypingUpdate,
    sendEditingPreview,
    resetTypingPreview,
    resetDraftOrderContext,
    removeTypingPreview,
    removeSelfTypingPreview,
    upsertTypingPreview,
    syncSelfTypingPreview,
    editingPreviewActive,
    inputPreviewEnabled,
    autoScrollTypingPreviewAlways,
    shouldObserveTypingPreview,
    toggleTypingPreview,
    activeIdentityForPreview,
    activeIdentityVariantShortcutContext,
    activeIdentityVariantForPreview,
    activeIdentityAppearanceForPreview,
    activeIdentityAppearancePreviewSignature,
    effectiveIdentityVariantForEmojiPanel,
    selfPreviewUserId,
    draftStartedAtMs,
    selfPreviewOrderModified,
    resolveCurrentWhisperTargetId,
    disposeSelfPreviewObserver,
    disposeTypingViewportObserver,
  };
};
