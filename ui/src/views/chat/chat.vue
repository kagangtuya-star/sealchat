<script setup lang="tsx">
import ChatItem from './components/chat-item.vue';
import { computed, ref, watch, h, onMounted, onBeforeMount, onBeforeUnmount, nextTick, reactive } from 'vue'
import { VirtualList } from 'vue-tiny-virtual-list';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { Event, Message, User } from '@satorijs/protocol'
import { useUserStore } from '@/stores/user';
import { ArrowBarToDown, Plus, Upload, Send, RotateClockwise } from '@vicons/tabler'
import { NIcon, c, useDialog, useMessage, type MentionOption } from 'naive-ui';
import VueScrollTo from 'vue-scrollto'
import ChatInputSwitcher from './components/ChatInputSwitcher.vue'
import { uploadImageAttachment } from './composables/useAttachmentUploader';
import { urlBase } from '@/stores/_config';
import { liveQuery } from "dexie";
import { useObservable } from "@vueuse/rxjs";
import { db, getSrc, type Thumb } from '@/models';
import { throttle } from 'lodash-es';
import AvatarVue from '@/components/avatar.vue';
import { Howl, Howler } from 'howler';
import SoundMessageCreated from '@/assets/message.mp3';
import RightClickMenu from './components/ChatRightClickMenu.vue'
import AvatarClickMenu from './components/AvatarClickMenu.vue'
import { nanoid } from 'nanoid';
import { useUtilsStore } from '@/stores/utils';
import { contentEscape, contentUnescape } from '@/utils/tools'
import IconNumber from '@/components/icons/IconNumber.vue'
import { computedAsync } from '@vueuse/core';
import type { UserEmojiModel } from '@/types';
import { Settings } from '@vicons/ionicons5';
import { dialogAskConfirm } from '@/utils/dialog';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render';
import DOMPurify from 'dompurify';

// const uploadImages = useObservable<Thumb[]>(
//   liveQuery(() => db.thumbs.toArray()) as any
// )

const chat = useChatStore();
const user = useUserStore();
const isEditing = computed(() => !!chat.editing);

const emojiLoading = ref(false)
const uploadImages = computedAsync(async () => {
  if (user.emojiCount) {
    const resp = await user.emojiList();
    return resp.data.items;
  }
  return [];
}, [], emojiLoading);

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n();

// const virtualListRef = ref<InstanceType<typeof VirtualList> | null>(null);
const messagesListRef = ref<HTMLElement | null>(null);
const textInputRef = ref<any>(null);
const inputMode = ref<'plain' | 'rich'>('plain');
const inlineImageInputRef = ref<HTMLInputElement | null>(null);

type SelectionRange = { start: number; end: number };

interface InlineImageDraft {
  id: string;
  token: string;
  status: 'uploading' | 'uploaded' | 'failed';
  objectUrl?: string;
  file?: File | null;
  attachmentId?: string;
  error?: string;
}

const inlineImages = reactive(new Map<string, InlineImageDraft>());
const inlineImageMarkerRegexp = /\[\[图片:([a-zA-Z0-9_-]+)\]\]/g;
let suspendInlineSync = false;

const hasUploadingInlineImages = computed(() => {
  for (const draft of inlineImages.values()) {
    if (draft.status === 'uploading') {
      return true;
    }
  }
  return false;
});

const hasFailedInlineImages = computed(() => {
  for (const draft of inlineImages.values()) {
    if (draft.status === 'failed') {
      return true;
    }
  }
  return false;
});

let pendingInlineSelection: SelectionRange | null = null;
const inlineImagePreviewMap = computed<Record<string, { status: 'uploading' | 'uploaded' | 'failed'; previewUrl?: string; error?: string }>>(() => {
  const result: Record<string, { status: 'uploading' | 'uploaded' | 'failed'; previewUrl?: string; error?: string }> = {};
  inlineImages.forEach((draft, key) => {
    let previewUrl = draft.objectUrl;
    if (!previewUrl && draft.attachmentId) {
      if (/^https?:/i.test(draft.attachmentId)) {
        previewUrl = draft.attachmentId;
      } else {
        const attachmentId = draft.attachmentId.startsWith('id:') ? draft.attachmentId.slice(3) : draft.attachmentId;
        previewUrl = `${urlBase}/api/v1/attachments/${attachmentId}`;
      }
    }
    result[key] = {
      status: draft.status,
      previewUrl,
      error: draft.error,
    };
  });
  return result;
});

const SCROLL_STICKY_THRESHOLD = 200;

const rows = ref<Message[]>([]);

const normalizeMessageShape = (msg: any): Message => {
  if (!msg) {
    return msg as Message;
  }
  if (msg.isEdited === undefined && msg.is_edited !== undefined) {
    msg.isEdited = msg.is_edited;
  }
  if (msg.editCount === undefined && msg.edit_count !== undefined) {
    msg.editCount = msg.edit_count;
  }
  if (msg.createdAt === undefined && msg.created_at !== undefined) {
    msg.createdAt = msg.created_at;
  }
  if (msg.updatedAt === undefined && msg.updated_at !== undefined) {
    msg.updatedAt = msg.updated_at;
  }
  if (msg.whisperTo === undefined && msg.whisper_to !== undefined) {
    msg.whisperTo = msg.whisper_to;
  }
  if ((msg as any).displayOrder === undefined && (msg as any).display_order !== undefined) {
    (msg as any).displayOrder = Number((msg as any).display_order);
  } else if ((msg as any).displayOrder !== undefined) {
    (msg as any).displayOrder = Number((msg as any).displayOrder);
  }
  if (msg.quote) {
    msg.quote = normalizeMessageShape(msg.quote);
  }
  return msg as Message;
};

const compareByDisplayOrder = (a: Message, b: Message) => {
  const orderA = Number((a as any).displayOrder ?? a.createdAt ?? 0);
  const orderB = Number((b as any).displayOrder ?? b.createdAt ?? 0);
  if (orderA === orderB) {
    return (Number(a.createdAt) || 0) - (Number(b.createdAt) || 0);
  }
  return orderA - orderB;
};

const sortRowsByDisplayOrder = () => {
  rows.value = rows.value
    .slice()
    .sort(compareByDisplayOrder);
};

const localReorderOps = new Set<string>();

const messageRowRefs = new Map<string, HTMLElement>();
const registerMessageRow = (el: HTMLElement | null, id: string) => {
  if (!id) {
    return;
  }
  if (el) {
    messageRowRefs.set(id, el);
  } else {
    messageRowRefs.delete(id);
  }
};

const dragState = reactive({
  snapshot: [] as Message[],
  clientOpId: null as string | null,
  overId: null as string | null,
  position: null as 'before' | 'after' | null,
  activeId: null as string | null,
  pointerId: null as number | null,
  startY: 0,
  ghostEl: null as HTMLElement | null,
  originEl: null as HTMLElement | null,
  autoScrollDirection: 0 as -1 | 0 | 1,
  autoScrollSpeed: 0,
  autoScrollRafId: null as number | null,
  lastClientY: null as number | null,
});

const AUTO_SCROLL_EDGE_THRESHOLD = 60;
const AUTO_SCROLL_MIN_SPEED = 2;
const AUTO_SCROLL_MAX_SPEED = 18;

const stopAutoScroll = () => {
  if (dragState.autoScrollRafId !== null) {
    cancelAnimationFrame(dragState.autoScrollRafId);
    dragState.autoScrollRafId = null;
  }
  dragState.autoScrollDirection = 0;
  dragState.autoScrollSpeed = 0;
};

const stepAutoScroll = () => {
  const container = messagesListRef.value;
  if (!container || dragState.autoScrollDirection === 0 || dragState.autoScrollSpeed <= 0) {
    stopAutoScroll();
    return;
  }
  const prev = container.scrollTop;
  container.scrollTop += dragState.autoScrollDirection * dragState.autoScrollSpeed;
  if (container.scrollTop === prev) {
    stopAutoScroll();
    return;
  }
  dragState.autoScrollRafId = requestAnimationFrame(stepAutoScroll);
  if (dragState.lastClientY !== null) {
    updateOverTarget(dragState.lastClientY);
  }
};

const startAutoScroll = () => {
  if (dragState.autoScrollRafId !== null) {
    return;
  }
  dragState.autoScrollRafId = requestAnimationFrame(stepAutoScroll);
};

const updateAutoScroll = (clientY: number) => {
  dragState.lastClientY = clientY;
  const container = messagesListRef.value;
  if (!container) {
    stopAutoScroll();
    return;
  }
  const rect = container.getBoundingClientRect();
  let direction: -1 | 0 | 1 = 0;
  let distance = 0;
  if (clientY < rect.top + AUTO_SCROLL_EDGE_THRESHOLD) {
    direction = -1;
    distance = rect.top + AUTO_SCROLL_EDGE_THRESHOLD - clientY;
  } else if (clientY > rect.bottom - AUTO_SCROLL_EDGE_THRESHOLD) {
    direction = 1;
    distance = clientY - (rect.bottom - AUTO_SCROLL_EDGE_THRESHOLD);
  }
  if (direction === 0) {
    stopAutoScroll();
    return;
  }
  const normalized = Math.min(distance, AUTO_SCROLL_EDGE_THRESHOLD) / AUTO_SCROLL_EDGE_THRESHOLD;
  const speed =
    AUTO_SCROLL_MIN_SPEED + normalized * (AUTO_SCROLL_MAX_SPEED - AUTO_SCROLL_MIN_SPEED);
  dragState.autoScrollDirection = direction;
  dragState.autoScrollSpeed = speed;
  startAutoScroll();
};

const clearGhost = () => {
  if (dragState.ghostEl && dragState.ghostEl.parentElement) {
    dragState.ghostEl.parentElement.removeChild(dragState.ghostEl);
  }
  dragState.ghostEl = null;
};

const resetDragState = () => {
  clearGhost();
  stopAutoScroll();
  dragState.snapshot = [];
  dragState.clientOpId = null;
  dragState.overId = null;
  dragState.position = null;
  dragState.activeId = null;
  dragState.pointerId = null;
  dragState.startY = 0;
  dragState.lastClientY = null;
  if (dragState.originEl) {
    dragState.originEl.classList.remove('message-row--drag-source');
  }
  dragState.originEl = null;
  document.body.style.userSelect = '';
};

const canReorderAll = computed(() => chat.canReorderAllMessages);
const isSelfMessage = (item?: Message) => item?.user?.id === user.info.id;
const canDragMessage = (item: Message) => {
  if (!item?.id) return false;
  if (chat.connectState !== 'connected') {
    return false;
  }
  if (chat.editing && chat.editing.messageId === item.id) {
    return false;
  }
  if ((item as any).is_revoked) {
    return false;
  }
  if (isSelfMessage(item)) {
    return true;
  }
  return canReorderAll.value;
};

const shouldShowHandle = (item: Message) => canDragMessage(item);

const rowClass = (item: Message) => ({
  'message-row': true,
  'message-row--self': isSelfMessage(item),
  'draggable-item': canDragMessage(item),
  'message-row--drop-before': dragState.overId === item.id && dragState.position === 'before',
  'message-row--drop-after': dragState.overId === item.id && dragState.position === 'after',
});

const createGhostElement = (rowEl: HTMLElement) => {
  const rect = rowEl.getBoundingClientRect();
  const ghost = rowEl.cloneNode(true) as HTMLElement;
  ghost.classList.add('message-row__ghost');
  ghost.style.position = 'fixed';
  ghost.style.left = `${rect.left}px`;
  ghost.style.top = `${rect.top}px`;
  ghost.style.width = `${rect.width}px`;
  ghost.style.pointerEvents = 'none';
  ghost.style.opacity = '0.85';
  ghost.style.zIndex = '999';
  document.body.appendChild(ghost);
  dragState.ghostEl = ghost;
};

const updateOverTarget = (clientY: number) => {
  let matched = false;
  if (dragState.activeId) {
    const activeEl = messageRowRefs.get(dragState.activeId);
    if (activeEl) {
      const rectActive = activeEl.getBoundingClientRect();
      if (clientY >= rectActive.top && clientY <= rectActive.bottom) {
        const mid = rectActive.top + rectActive.height / 2;
        dragState.overId = dragState.activeId;
        dragState.position = clientY <= mid ? 'before' : 'after';
        matched = true;
      }
    }
  }
  const currentRows = rows.value;
  for (const item of currentRows) {
    if (!item?.id || item.id === dragState.activeId) {
      continue;
    }
    const el = messageRowRefs.get(item.id);
    if (!el) {
      continue;
    }
    const rect = el.getBoundingClientRect();
    const mid = rect.top + rect.height / 2;
    if (clientY <= mid) {
      dragState.overId = item.id;
      dragState.position = 'before';
      matched = true;
      break;
    }
    if (clientY < rect.bottom) {
      dragState.overId = item.id;
      dragState.position = 'after';
      matched = true;
      break;
    }
  }
  if (!matched && currentRows.length > 0) {
    const last = currentRows[currentRows.length - 1];
    if (last?.id) {
      dragState.overId = last.id;
      dragState.position = 'after';
      matched = true;
    }
  }
  if (!matched) {
    dragState.overId = null;
    dragState.position = null;
  }
};

const cancelDrag = () => {
  window.removeEventListener('pointermove', onDragPointerMove);
  window.removeEventListener('pointerup', onDragPointerUp);
  window.removeEventListener('pointercancel', onDragPointerCancel);
  window.removeEventListener('keydown', onDragKeyDown);
  stopAutoScroll();
  if (dragState.snapshot.length > 0) {
    rows.value = dragState.snapshot.slice();
  }
  resetDragState();
};

const finalizeDrag = async () => {
  const channelId = chat.curChannel?.id;
  const activeId = dragState.activeId;
  const overId = dragState.overId;
  const position = dragState.position;
  const snapshot = dragState.snapshot.slice();

  window.removeEventListener('pointermove', onDragPointerMove);
  window.removeEventListener('pointerup', onDragPointerUp);
  window.removeEventListener('pointercancel', onDragPointerCancel);
  window.removeEventListener('keydown', onDragKeyDown);

  stopAutoScroll();
  clearGhost();
  document.body.style.userSelect = '';

  if (!channelId || !activeId || !overId || activeId === overId) {
    resetDragState();
    return;
  }

  const fromIndex = snapshot.findIndex((item) => item.id === activeId);
  const toReference = snapshot.findIndex((item) => item.id === overId);
  if (fromIndex < 0 || toReference < 0) {
    resetDragState();
    return;
  }

  const [moving] = snapshot.splice(fromIndex, 1);
  let targetIndex = toReference;
  if (position === 'after') {
    if (fromIndex < toReference) {
      targetIndex = toReference;
    } else {
      targetIndex = toReference + 1;
    }
  }
  if (targetIndex < 0) {
    targetIndex = 0;
  }
  if (targetIndex > snapshot.length) {
    targetIndex = snapshot.length;
  }
  snapshot.splice(targetIndex, 0, moving);
  rows.value = snapshot;

  const beforeId = rows.value[targetIndex + 1]?.id || '';
  const afterId = rows.value[targetIndex - 1]?.id || '';
  const clientOpId = dragState.clientOpId || nanoid();
  localReorderOps.add(clientOpId);
  try {
    const resp = await chat.messageReorder(channelId, {
      messageId: activeId,
      beforeId,
      afterId,
      clientOpId,
    });
    if (resp?.display_order !== undefined) {
      (moving as any).displayOrder = Number(resp.display_order);
      sortRowsByDisplayOrder();
    }
  } catch (error) {
    rows.value = dragState.snapshot.slice();
    message.error('消息排序失败，请稍后重试');
  } finally {
    localReorderOps.delete(clientOpId);
    resetDragState();
  }
};

const onDragPointerMove = (event: PointerEvent) => {
  if (event.pointerId !== dragState.pointerId) {
    return;
  }
  event.preventDefault();
  if (dragState.ghostEl) {
    dragState.ghostEl.style.transform = `translateY(${event.clientY - dragState.startY}px)`;
  }
  updateOverTarget(event.clientY);
  updateAutoScroll(event.clientY);
};

const onDragPointerUp = (event: PointerEvent) => {
  if (event.pointerId !== dragState.pointerId) {
    return;
  }
  event.preventDefault();
  finalizeDrag();
};

const onDragPointerCancel = (event: PointerEvent) => {
  if (event.pointerId !== dragState.pointerId) {
    return;
  }
  event.preventDefault();
  cancelDrag();
};

const onDragKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    event.preventDefault();
    cancelDrag();
  }
};

const onDragHandlePointerDown = (event: PointerEvent, item: Message) => {
  if (!canDragMessage(item) || !item.id) {
    return;
  }
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return;
  }
  const rowEl = messageRowRefs.get(item.id);
  if (!rowEl) {
    return;
  }
  rowEl.classList.add('message-row--drag-source');
  dragState.snapshot = rows.value.slice();
  dragState.clientOpId = nanoid();
  dragState.activeId = item.id;
  dragState.pointerId = event.pointerId;
  dragState.startY = event.clientY;
  dragState.overId = item.id;
  dragState.position = 'after';
  dragState.originEl = rowEl;
  document.body.style.userSelect = 'none';
  createGhostElement(rowEl);
  updateOverTarget(event.clientY);
  updateAutoScroll(event.clientY);

  window.addEventListener('pointermove', onDragPointerMove);
  window.addEventListener('pointerup', onDragPointerUp);
  window.addEventListener('pointercancel', onDragPointerCancel);
  window.addEventListener('keydown', onDragKeyDown);

  event.preventDefault();
};

const applyReorderPayload = (payload: any) => {
  if (!payload?.messageId) {
    return;
  }
  const target = rows.value.find((item) => item.id === payload.messageId);
  if (!target) {
    return;
  }
  if (payload.displayOrder !== undefined) {
    const parsed = Number(payload.displayOrder);
    if (!Number.isNaN(parsed)) {
      (target as any).displayOrder = parsed;
    }
  }
  sortRowsByDisplayOrder();
};

const normalizeMessageList = (items: any[] = []): Message[] => items.map((item) => normalizeMessageShape(item));

const upsertMessage = (incoming?: Message) => {
  if (!incoming || !incoming.id) {
    return;
  }
  const index = rows.value.findIndex((msg) => msg.id === incoming.id);
  if (index >= 0) {
    const merged = {
      ...rows.value[index],
      ...incoming,
    };
    rows.value.splice(index, 1, merged);
  } else {
    rows.value.push(incoming);
  }
  sortRowsByDisplayOrder();
};

async function replaceUsernames(text: string) {
  const resp = await chat.guildMemberList('');
  const infoMap = (resp.data as any[]).reduce((obj, item) => {
    obj[item.nick] = item;
    return obj;
  }, {})

  // 匹配 @ 后跟着字母数字下划线的用户名
  const regex = /@(\S+)/g;

  // 使用 replace 方法来替换匹配到的用户名
  const replacedText = text.replace(regex, (match, username) => {
    if (username in infoMap) {
      const info = infoMap[username];
      return `<at id="${info.id}" name="${info.nick}" />`
    }
    return match;
  });

  return replacedText;
}

const instantMessages = reactive(new Set<Message>());

interface TypingPreviewItem {
  userId: string;
  displayName: string;
  avatar?: string;
  content: string;
  indicatorOnly: boolean;
  mode: 'typing' | 'editing';
  messageId?: string;
}

interface EditingPreviewInfo {
  userId: string;
  displayName: string;
  avatar?: string;
  content: string;
  indicatorOnly: boolean;
  isSelf: boolean;
  summary: string;
  previewHtml: string;
}

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
  return 'indicator';
};
const typingPreviewMode = ref<TypingBroadcastState>(resolveTypingPreviewMode());
if (localStorage.getItem(legacyTypingPreviewKey) !== null) {
  localStorage.removeItem(legacyTypingPreviewKey);
}
const typingPreviewActive = ref(false);
const typingPreviewList = ref<TypingPreviewItem[]>([]);
const typingPreviewItems = computed(() => typingPreviewList.value.filter((item) => item.mode === 'typing'));
let lastTypingChannelId = '';

const upsertTypingPreview = (item: TypingPreviewItem) => {
  const shouldStick = isNearBottom();
  typingPreviewList.value = typingPreviewList.value.filter((i) => !(i.userId === item.userId && i.mode === item.mode));
  typingPreviewList.value.push(item);
  if (shouldStick) {
    toBottom();
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
};

const sendTypingUpdate = throttle((state: TypingBroadcastState, content: string, channelId: string) => {
  chat.messageTyping(state, content, channelId);
}, 400, { leading: true, trailing: true });

const stopTypingPreviewNow = () => {
  sendTypingUpdate.cancel();
  if (typingPreviewActive.value && lastTypingChannelId) {
    chat.messageTyping('silent', '', lastTypingChannelId);
  }
  typingPreviewActive.value = false;
  lastTypingChannelId = '';
};

const editingPreviewActive = ref(false);
let lastEditingChannelId = '';
let lastEditingMessageId = '';

const sendEditingPreview = throttle((channelId: string, messageId: string, content: string) => {
  if (typingPreviewMode.value !== 'content') {
    return;
  }
  chat.messageTyping('content', content, channelId, { mode: 'editing', messageId });
  editingPreviewActive.value = true;
  lastEditingChannelId = channelId;
  lastEditingMessageId = messageId;
}, 400, { leading: true, trailing: true });

const stopEditingPreviewNow = () => {
  sendEditingPreview.cancel();
  if (editingPreviewActive.value && lastEditingChannelId && lastEditingMessageId) {
    chat.messageTyping('silent', '', lastEditingChannelId, { mode: 'editing', messageId: lastEditingMessageId });
  }
  editingPreviewActive.value = false;
  lastEditingChannelId = '';
  lastEditingMessageId = '';
};

const convertMessageContentToDraft = (content?: string) => {
  resetInlineImages();
  if (!content) {
    return '';
  }
  if (isTipTapJson(content)) {
    return content;
  }
  let text = contentUnescape(content);
  const imageRecords: Array<{ id: string; token: string; attachmentId: string }> = [];
  text = text.replace(/<img\s+[^>]*src="([^"]+)"[^>]*\/?>/gi, (_, src) => {
    const markerId = nanoid();
    const token = `[[图片:${markerId}]]`;
    const attachmentId = src.startsWith('id:') ? src : src;
    imageRecords.push({ id: markerId, token, attachmentId });
    return token;
  });
  imageRecords.forEach(({ id, token, attachmentId }) => {
    const record: InlineImageDraft = reactive({
      id,
      token,
      status: 'uploaded',
      attachmentId,
      file: null,
    });
    inlineImages.set(id, record);
  });
  text = text.replace(/<at\s+[^>]*name="([^"]+)"[^>]*\/>/gi, (_, name) => `@${name}`);
  text = text.replace(/<at\s+[^>]*id="([^"]+)"[^>]*\/>/gi, (_, id) => `@${id}`);
  text = text.replace(/<br\s*\/?>/gi, '\n');
  return text;
};

const emitTypingPreview = () => {
  if (chat.connectState !== 'connected') return;
  const channelId = chat.curChannel?.id;
  if (!channelId) return;

  if (typingPreviewMode.value === 'silent') {
    stopTypingPreviewNow();
    return;
  }

  const raw = textToSend.value;
  if (raw.trim().length === 0) {
    stopTypingPreviewNow();
    return;
  }

  typingPreviewActive.value = true;
  lastTypingChannelId = channelId;
  const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
  const content = typingPreviewMode.value === 'content' ? truncated : '';
  sendTypingUpdate(typingPreviewMode.value, content, channelId);
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
  const raw = textToSend.value;
  const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
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

const textToSend = ref('');

// 草稿保存和恢复功能
const DRAFT_STORAGE_KEY = 'sealchat_message_draft';
const hasSavedDraft = ref(false);

// 保存草稿到 localStorage
const saveDraft = () => {
  if (!textToSend.value.trim() && Object.keys(inlineImages).length === 0) {
    // 空内容不保存
    return;
  }

  try {
    const draftData = {
      content: textToSend.value,
      mode: inputMode.value,
      timestamp: Date.now(),
      channelId: chat.curChannel?.id,
      inlineImages: Object.fromEntries(
        Object.entries(inlineImages).map(([key, value]) => [
          key,
          {
            status: value.status,
            previewUrl: value.previewUrl,
            attachmentId: value.attachmentId,
          }
        ])
      ),
    };
    localStorage.setItem(DRAFT_STORAGE_KEY, JSON.stringify(draftData));
    console.log('草稿已保存');
  } catch (e) {
    console.error('保存草稿失败', e);
  }
};

// 读取草稿
const loadDraft = () => {
  try {
    const stored = localStorage.getItem(DRAFT_STORAGE_KEY);
    if (!stored) {
      hasSavedDraft.value = false;
      return null;
    }

    const draftData = JSON.parse(stored);

    // 检查草稿是否过期（7天）
    if (Date.now() - draftData.timestamp > 7 * 24 * 60 * 60 * 1000) {
      clearDraft();
      return null;
    }

    hasSavedDraft.value = true;
    return draftData;
  } catch (e) {
    console.error('读取草稿失败', e);
    hasSavedDraft.value = false;
    return null;
  }
};

// 清除草稿
const clearDraft = () => {
  try {
    localStorage.removeItem(DRAFT_STORAGE_KEY);
    hasSavedDraft.value = false;
    console.log('草稿已清除');
  } catch (e) {
    console.error('清除草稿失败', e);
  }
};

// 恢复草稿
const restoreDraft = () => {
  const draft = loadDraft();
  if (!draft) {
    message.warning('没有找到可恢复的草稿');
    return;
  }

  // 确认恢复（如果当前有内容）
  if (textToSend.value.trim()) {
    dialog.warning({
      title: '恢复草稿',
      content: '当前输入框有内容，恢复草稿将覆盖现有内容，是否继续？',
      positiveText: '恢复',
      negativeText: '取消',
      onPositiveClick: () => {
        applyDraft(draft);
      },
    });
  } else {
    applyDraft(draft);
  }
};

// 应用草稿
const applyDraft = (draft: any) => {
  try {
    // 恢复输入模式
    if (draft.mode) {
      inputMode.value = draft.mode;
    }

    // 恢复文本内容
    textToSend.value = draft.content || '';

    // 恢复图片（仅恢复已上传成功的）
    if (draft.inlineImages) {
      Object.entries(draft.inlineImages).forEach(([markerId, imageData]: [string, any]) => {
        if (imageData.status === 'uploaded' && imageData.attachmentId && imageData.previewUrl) {
          inlineImages[markerId] = {
            status: 'uploaded',
            previewUrl: imageData.previewUrl,
            attachmentId: imageData.attachmentId,
          };
        }
      });
    }

    clearDraft();
    message.success('草稿已恢复');

    // 聚焦到输入框
    nextTick(() => {
      textInputRef.value?.focus();
    });
  } catch (e) {
    console.error('应用草稿失败', e);
    message.error('恢复草稿失败');
  }
};

const editingPreviewMap = computed<Record<string, EditingPreviewInfo>>(() => {
  const map: Record<string, EditingPreviewInfo> = {};
  typingPreviewList.value.forEach((item) => {
    if (item.mode === 'editing' && item.messageId) {
      const contentValue = item.content || '';
      const indicatorOnly = item.indicatorOnly || contentValue.trim().length === 0;
      const { summary, previewHtml } = indicatorOnly ? { summary: '', previewHtml: '' } : buildPreviewMeta(contentValue);
      map[item.messageId] = {
        userId: item.userId,
        displayName: item.displayName,
        avatar: item.avatar,
        content: contentValue,
        indicatorOnly,
        isSelf: item.userId === user.info.id,
        summary,
        previewHtml,
      };
    }
  });
  if (isEditing.value && chat.editing) {
    const draft = textToSend.value;
    const indicatorOnly = draft.trim().length === 0;
    const { summary, previewHtml } = indicatorOnly ? { summary: '', previewHtml: '' } : buildPreviewMeta(draft);
    map[chat.editing.messageId] = {
      userId: user.info.id,
      displayName: chat.curMember?.nick || user.info.nick || user.info.name || '我',
      avatar: chat.curMember?.avatar || user.info.avatar || '',
      content: draft,
      indicatorOnly,
      isSelf: true,
      summary,
      previewHtml,
    };
  }
  return map;
});
const whisperPanelVisible = ref(false);
const whisperPickerSource = ref<'slash' | 'manual' | null>(null);
const whisperQuery = ref('');
const whisperSelectionIndex = ref(0);
const whisperSearchInputRef = ref<any>(null);

interface WhisperCandidate {
  raw: any;
  id: string;
  avatar: string;
  displayName: string;
  secondaryName: string;
}

const whisperCandidates = computed<WhisperCandidate[]>(() => chat.curChannelUsers
  .filter((i: any) => i?.id && i.id !== user.info.id)
  .map((candidate: any) => ({
    raw: candidate,
    id: candidate.id,
    avatar: candidate.avatar || '',
    displayName: candidateDisplayName(candidate),
    secondaryName: candidateSecondaryName(candidate),
  }))
);

const candidateDisplayName = (candidate: any) => candidate?.nick || candidate?.name || candidate?.username || '未知成员';
const candidateSecondaryName = (candidate: any) => {
  const primary = candidateDisplayName(candidate);
  const backup = candidate?.username || candidate?.name || '';
  if (backup && backup !== primary) {
    return backup;
  }
  return '';
};

const filteredWhisperCandidates = computed(() => {
  const keyword = whisperQuery.value.trim().toLowerCase();
  if (!keyword) {
    return whisperCandidates.value;
  }
  return whisperCandidates.value.filter((candidate) => {
    const candidates = [
      candidate.displayName,
      candidate.secondaryName,
      candidate.id,
    ].filter(Boolean).map((str) => String(str).toLowerCase());
    return candidates.some((name) => name.includes(keyword));
  });
});

const canOpenWhisperPanel = computed(() => whisperCandidates.value.length > 0);
const whisperMode = computed(() => !!chat.whisperTarget);
const whisperTargetDisplay = computed(() => chat.whisperTarget?.nick || chat.whisperTarget?.name || '未知成员');
const whisperPlaceholderText = computed(() => t('inputBox.whisperPlaceholder', { target: `@${whisperTargetDisplay.value}` }));

const ensureInputFocus = () => {
  nextTick(() => {
    textInputRef.value?.focus?.();
  });
};

const getInputSelection = (): SelectionRange => {
  const selection = textInputRef.value?.getSelectionRange?.();
  if (selection) {
    return { start: selection.start, end: selection.end };
  }
  const length = textToSend.value.length;
  return { start: length, end: length };
};

const setInputSelection = (start: number, end: number) => {
  textInputRef.value?.setSelectionRange?.(start, end);
};

const moveInputCursorToEnd = () => {
  if (textInputRef.value?.moveCursorToEnd) {
    textInputRef.value.moveCursorToEnd();
    return;
  }
  const length = textToSend.value.length;
  setInputSelection(length, length);
  textInputRef.value?.focus?.();
};

const detectMessageContentMode = (content?: string): 'plain' | 'rich' => {
  if (!content) {
    return 'plain';
  }
  if (isTipTapJson(content)) {
    return 'rich';
  }
  return 'plain';
};

const beginEdit = (target?: Message) => {
  if (!target?.id || !chat.curChannel?.id) {
    return;
  }
  if (target.user?.id !== user.info.id) {
    message.error('只能编辑自己发送的消息');
    return;
  }
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  chat.curReplyTo = null;
  chat.clearWhisperTarget();
  const detectedMode = detectMessageContentMode(target.content);
  chat.startEditingMessage({
    messageId: target.id,
    channelId: chat.curChannel.id,
    originalContent: target.content || '',
    draft: target.content || '',
    mode: detectedMode,
  });
  inputMode.value = detectedMode;
};

const cancelEditing = () => {
  if (!chat.editing) {
    return;
  }
  stopEditingPreviewNow();
  chat.cancelEditing();
  textToSend.value = '';
  stopTypingPreviewNow();
  resetInlineImages();
  ensureInputFocus();
};

const saveEdit = async () => {
  if (!chat.editing) {
    return;
  }
  if (chat.connectState !== 'connected') {
    message.error('尚未连接，请稍等');
    return;
  }
  const draft = textToSend.value;
  const hasImages = containsInlineImageMarker(draft);
  if (draft.trim() === '' && !hasImages) {
    message.error('消息内容不能为空');
    return;
  }
  if (draft.length > 10000) {
    message.error('消息过长，请分段编辑');
    return;
  }
  if (hasUploadingInlineImages.value) {
    message.warning('仍有图片正在上传，请稍候再试');
    return;
  }
  if (hasFailedInlineImages.value) {
    message.error('存在上传失败的图片，请删除后重试');
    return;
  }
  try {
    stopTypingPreviewNow();
    let finalContent: string;
    if (inputMode.value === 'rich') {
      const editorInstance = textInputRef.value?.getEditor?.();
      if (editorInstance) {
        finalContent = JSON.stringify(editorInstance.getJSON());
      } else {
        finalContent = draft;
      }
    } else {
      finalContent = await buildMessageHtml(draft);
    }
    if (finalContent.trim() === '') {
      message.error('消息内容不能为空');
      return;
    }
    const updated = await chat.messageUpdate(chat.editing.channelId, chat.editing.messageId, finalContent);
    if (updated) {
      upsertMessage(updated as unknown as Message);
    }
    message.success('消息已更新');
    stopEditingPreviewNow();
    chat.cancelEditing();
    textToSend.value = '';
    resetInlineImages();
    ensureInputFocus();
  } catch (error: any) {
    console.error('更新消息失败', error);
    message.error((error?.message ?? '编辑失败，请稍后重试'));
  }
};

function openWhisperPanel(source: 'slash' | 'manual') {
  whisperPickerSource.value = source;
  whisperPanelVisible.value = true;
  whisperSelectionIndex.value = 0;
  if (source === 'manual') {
    whisperQuery.value = '';
    nextTick(() => {
      whisperSearchInputRef.value?.focus?.();
    });
  }
}

function closeWhisperPanel() {
  whisperPanelVisible.value = false;
  whisperSelectionIndex.value = 0;
  whisperQuery.value = '';
  whisperPickerSource.value = null;
}

const applyWhisperTarget = (candidate: WhisperCandidate) => {
  if (!candidate?.id) {
    return;
  }
  const raw = candidate.raw || {};
  const targetUser: User = {
    id: candidate.id,
    name: raw.name || raw.username || raw.nick || candidate.displayName,
    nick: candidate.displayName,
    avatar: candidate.avatar,
    discriminator: raw.discriminator || '',
    is_bot: !!raw.is_bot,
  };
  chat.setWhisperTarget(targetUser);
  const source = whisperPickerSource.value;
  closeWhisperPanel();
  if (source === 'slash') {
    textToSend.value = '';
  }
  ensureInputFocus();
};

const handleWhisperCommand = (value: string) => {
  const match = value.match(/^\/(w|whisper)\s*(.*)$/i);
  if (match) {
    const query = match[2]?.trim() || '';
    if (!whisperPanelVisible.value || whisperPickerSource.value !== 'slash') {
      openWhisperPanel('slash');
    }
    whisperQuery.value = query;
    return;
  }
  if (whisperPickerSource.value === 'slash') {
    closeWhisperPanel();
  }
};

const handleWhisperKeydown = (event: KeyboardEvent) => {
  if (!whisperPanelVisible.value) {
    return false;
  }
  const list = filteredWhisperCandidates.value;
  if (event.key === 'ArrowDown') {
    if (list.length) {
      whisperSelectionIndex.value = (whisperSelectionIndex.value + 1) % list.length;
    }
    event.preventDefault();
    return true;
  }
  if (event.key === 'ArrowUp') {
    if (list.length) {
      whisperSelectionIndex.value = (whisperSelectionIndex.value - 1 + list.length) % list.length;
    }
    event.preventDefault();
    return true;
  }
  if (event.key === 'Enter' || event.key === 'Tab') {
    const selected = list[whisperSelectionIndex.value];
    if (selected) {
      applyWhisperTarget(selected);
    }
    event.preventDefault();
    return true;
  }
  if (event.key === 'Escape') {
    const source = whisperPickerSource.value;
    closeWhisperPanel();
    if (source === 'slash') {
      textToSend.value = '';
    }
    event.preventDefault();
    return true;
  }
  return false;
};

const startWhisperSelection = () => {
  if (!canOpenWhisperPanel.value) {
    message.warning(t('inputBox.whisperNoOnline'));
    return;
  }
  openWhisperPanel('manual');
};

const clearWhisperTarget = () => {
  chat.clearWhisperTarget();
  ensureInputFocus();
};

const containsInlineImageMarker = (text: string) => /\[\[图片:[^\]]+\]\]/.test(text);

const collectInlineMarkerIds = (text: string) => {
  const markers = new Set<string>();
  inlineImageMarkerRegexp.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = inlineImageMarkerRegexp.exec(text)) !== null) {
    markers.add(match[1]);
  }
  inlineImageMarkerRegexp.lastIndex = 0;
  return markers;
};

const revokeInlineImage = (draft?: InlineImageDraft) => {
  if (draft?.objectUrl) {
    URL.revokeObjectURL(draft.objectUrl);
    draft.objectUrl = undefined;
  }
};

const removeInlineImage = (markerId: string) => {
  const draft = inlineImages.get(markerId);
  if (draft) {
    revokeInlineImage(draft);
    inlineImages.delete(markerId);

    // 从文本中移除对应的标记
    const marker = `[[图片:${markerId}]]`;
    textToSend.value = textToSend.value.replace(marker, '');
  }
};

const resetInlineImages = () => {
  inlineImages.forEach((draft) => revokeInlineImage(draft));
  inlineImages.clear();
};

const syncInlineMarkersWithText = (value: string) => {
  const markers = collectInlineMarkerIds(value);
  inlineImages.forEach((draft, key) => {
    if (!markers.has(key)) {
      revokeInlineImage(draft);
      inlineImages.delete(key);
    }
  });
};

// 格式化预览文本 - 支持图片和富文本
const formatInlinePreviewText = (value: string) => {
  // 检测是否为 TipTap JSON
  if (value.trim().startsWith('{') && value.includes('"type":"doc"')) {
    try {
      const json = JSON.parse(value);
      // 提取纯文本内容
      return extractTipTapText(json).slice(0, 100);
    } catch {
      // 如果解析失败，继续处理为普通文本
    }
  }

  // 替换图片标记为 [图片]
  return value.replace(/\[\[图片:[^\]]+\]\]/g, '[图片]');
};

// 从 TipTap JSON 提取纯文本
const extractTipTapText = (node: any): string => {
  if (!node) return '';

  if (node.text !== undefined) {
    return node.text;
  }

  if (node.type === 'image') {
    return '[图片]';
  }

  if (node.content && Array.isArray(node.content)) {
    return node.content.map(extractTipTapText).join('');
  }

  return '';
};

// 渲染预览内容（支持图片和富文本）
const renderPreviewContent = (value: string) => {
  // 检测是否为 TipTap JSON
  if (value.trim().startsWith('{') && value.includes('"type":"doc"')) {
    try {
      const json = JSON.parse(value);
      const html = tiptapJsonToHtml(json, {
        baseUrl: urlBase,
        imageClass: 'preview-inline-image',
        linkClass: 'text-blue-500',
      });
      return DOMPurify.sanitize(html);
    } catch {
      // 如果解析失败，继续处理为普通文本
    }
  }

  // 处理普通文本和图片标记
  const imageMarkerRegex = /\[\[图片:([^\]]+)\]\]/g;
  let result = '';
  let lastIndex = 0;

  let match;
  while ((match = imageMarkerRegex.exec(value)) !== null) {
    // 添加标记前的文本
    if (match.index > lastIndex) {
      result += escapeHtml(value.substring(lastIndex, match.index));
    }

    // 添加图片
    const markerId = match[1];
    const imageInfo = inlineImages.get(markerId);
    if (imageInfo && imageInfo.previewUrl) {
      result += `<img src="${imageInfo.previewUrl}" class="preview-inline-image" alt="图片" />`;
    } else {
      result += '<span class="preview-image-placeholder">[图片]</span>';
    }

    lastIndex = match.index + match[0].length;
  }

  // 添加剩余文本
  if (lastIndex < value.length) {
    result += escapeHtml(value.substring(lastIndex));
  }

  return DOMPurify.sanitize(result || value);
};

const buildPreviewMeta = (value: string) => {
  const summary = value ? formatInlinePreviewText(value) : '';
  const previewHtml = value ? renderPreviewContent(value) : '';
  return { summary, previewHtml };
};

const escapeHtml = (text: string): string => {
  const map: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;',
  };
  return text.replace(/[&<>"']/g, (char) => map[char] || char);
};

const buildMessageHtml = async (draft: string) => {
  const placeholderMap = new Map<string, string>();
  let index = 0;
  inlineImageMarkerRegexp.lastIndex = 0;
  const sanitizedDraft = draft.replace(inlineImageMarkerRegexp, (_, markerId) => {
    const record = inlineImages.get(markerId);
    if (record && record.status === 'uploaded' && record.attachmentId) {
      const placeholder = `__INLINE_IMG_${index++}__`;
      const src = record.attachmentId.startsWith('id:') ? record.attachmentId : `id:${record.attachmentId}`;
      placeholderMap.set(placeholder, `<img src="${src}" />`);
      return placeholder;
    }
    return '';
  });
  inlineImageMarkerRegexp.lastIndex = 0;
  let escaped = contentEscape(sanitizedDraft);
  escaped = escaped.replace(/\r\n/g, '\n').replace(/\n/g, '<br />');
  escaped = await replaceUsernames(escaped);
  let html = escaped;
  placeholderMap.forEach((value, key) => {
    html = html.split(key).join(value);
  });
  return html;
};

const captureSelectionRange = (): SelectionRange => {
  const selection = getInputSelection();
  return { start: selection.start, end: selection.end };
};

const startInlineImageUpload = async (markerId: string, draft: InlineImageDraft) => {
  try {
    if (!draft.file) {
      draft.status = 'failed';
      draft.error = '无效的图片文件';
      return;
    }
    const result = await uploadImageAttachment(draft.file as File, { channelId: chat.curChannel?.id });
    draft.attachmentId = result.attachmentId;
    draft.status = 'uploaded';
    draft.error = '';
  } catch (error: any) {
    draft.status = 'failed';
    draft.error = error?.message || '上传失败';
    message.error('图片上传失败，请删除占位符后重试');
  }
};

const insertInlineImages = (files: File[], selection?: SelectionRange) => {
  if (!files.length) {
    return;
  }
  const imageFiles = files.filter((file) => file.type.startsWith('image/'));
  if (!imageFiles.length) {
    message.warning('当前仅支持插入图片文件');
    return;
  }
  const draftText = textToSend.value;
  const range = selection ?? captureSelectionRange();
  const draftLength = draftText.length;
  const start = Math.max(0, Math.min(range.start, draftLength));
  const end = Math.max(start, Math.min(range.end, draftLength));
  let cursor = start;
  let updatedText = draftText.slice(0, start) + draftText.slice(end);
  imageFiles.forEach((file, index) => {
    const markerId = nanoid();
    const token = `[[图片:${markerId}]]`;
    const objectUrl = URL.createObjectURL(file);
    const draftRecord: InlineImageDraft = reactive({
      id: markerId,
      token,
      status: 'uploading',
      objectUrl,
      file,
  });
  inlineImages.set(markerId, draftRecord);
  updatedText = updatedText.slice(0, cursor) + token + updatedText.slice(cursor);
  cursor += token.length;
  startInlineImageUpload(markerId, draftRecord);
});
textToSend.value = updatedText;
nextTick(() => {
  requestAnimationFrame(() => {
    textInputRef.value?.focus?.();
    requestAnimationFrame(() => {
      setInputSelection(cursor, cursor);
    });
  });
});
};

const handlePlainPasteImage = (payload: { files: File[]; selectionStart: number; selectionEnd: number }) => {
  if (inputMode.value === 'rich') {
    // 富文本模式下的图片粘贴
    handleRichImageInsert(payload.files);
  } else {
    // 纯文本模式下的图片粘贴
    insertInlineImages(payload.files, { start: payload.selectionStart, end: payload.selectionEnd });
  }
};

const handlePlainDropFiles = (payload: { files: File[]; selectionStart: number; selectionEnd: number }) => {
  if (inputMode.value === 'rich') {
    // 富文本模式下的图片拖拽
    handleRichImageInsert(payload.files);
  } else {
    // 纯文本模式下的图片拖拽
    insertInlineImages(payload.files, { start: payload.selectionStart, end: payload.selectionEnd });
  }
};

const handleRichImageInsert = async (files: File[]) => {
  if (!files.length) return;

  const imageFiles = files.filter((file) => file.type.startsWith('image/'));
  if (!imageFiles.length) {
    message.warning('当前仅支持插入图片文件');
    return;
  }

  const editor = textInputRef.value?.getEditor?.();
  if (!editor) return;

  for (const file of imageFiles) {
    const markerId = nanoid();
    const objectUrl = URL.createObjectURL(file);

    // 在编辑器中插入临时图片（使用 object URL）
    editor.chain().focus().setImage({ src: objectUrl, alt: `图片-${markerId}` }).run();

    // 创建上传记录
    const draftRecord: InlineImageDraft = reactive({
      id: markerId,
      token: `[[图片:${markerId}]]`,
      status: 'uploading',
      objectUrl,
      file,
    });
    inlineImages.set(markerId, draftRecord);

    // 开始上传
    try {
      const result = await uploadImageAttachment(file, { channelId: chat.curChannel?.id });
      draftRecord.attachmentId = result.attachmentId;
      draftRecord.status = 'uploaded';
      draftRecord.error = '';

      // 更新编辑器中的图片 URL（使用 id: 协议）
      const finalUrl = `id:${result.attachmentId}`;
      const { state } = editor;
      const { doc } = state;

      doc.descendants((node, pos) => {
        if (node.type.name === 'image' && node.attrs.src === objectUrl) {
          const tr = state.tr.setNodeMarkup(pos, undefined, {
            ...node.attrs,
            src: finalUrl,
          });
          editor.view.dispatch(tr);
          return false;
        }
      });

      // 释放临时 URL
      URL.revokeObjectURL(objectUrl);
    } catch (error: any) {
      draftRecord.status = 'failed';
      draftRecord.error = error?.message || '上传失败';
      message.error(`图片上传失败: ${draftRecord.error}`);
    }
  }
};

const handleInlineFileChange = (event: Event) => {
  const input = event.target as HTMLInputElement | null;
  if (!input?.files?.length) {
    pendingInlineSelection = null;
    return;
  }

  const files = Array.from(input.files);

  if (inputMode.value === 'rich') {
    // 富文本模式：调用富文本图片插入
    handleRichImageInsert(files);
  } else {
    // 纯文本模式：调用纯文本图片插入
    insertInlineImages(files, pendingInlineSelection || undefined);
  }

  pendingInlineSelection = null;
  input.value = '';
};

watch(() => chat.editing?.messageId, (messageId, previousId) => {
  if (!messageId && previousId) {
    stopEditingPreviewNow();
    textToSend.value = '';
    return;
  }
  if (messageId && chat.editing) {
    if (previousId && previousId !== messageId) {
      stopEditingPreviewNow();
    }
    const editingMode = chat.editing.mode ?? detectMessageContentMode(chat.editing.originalContent || chat.editing.draft);
    inputMode.value = editingMode;
    let draft = '';
    if (editingMode === 'rich') {
      const source = chat.editing.draft ?? '';
      const original = chat.editing.originalContent ?? '';
      resetInlineImages();
      if (isTipTapJson(source)) {
        draft = source;
      } else if (isTipTapJson(original)) {
        draft = original;
      } else {
        draft = source;
      }
    } else {
      draft = convertMessageContentToDraft(chat.editing.draft);
    }
    chat.curReplyTo = null;
    chat.clearWhisperTarget();
    textToSend.value = draft;
    chat.updateEditingDraft(draft);
    chat.messageMenu.show = false;
   stopTypingPreviewNow();
    ensureInputFocus();
    nextTick(() => {
      if (inputMode.value === 'plain') {
        moveInputCursorToEnd();
      } else {
        const editor = textInputRef.value?.getEditor?.();
        editor?.chain().focus('end').run();
      }
      document.getElementById(messageId)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      emitEditingPreview();
    });
  }
});

const send = throttle(async () => {
  if (isEditing.value) {
    await saveEdit();
    return;
  }
  if (chat.connectState !== 'connected') {
    message.error('尚未连接，请稍等');
    return;
  }
  const draft = textToSend.value;

  // 检查是否为富文本模式
  const isRichMode = inputMode.value === 'rich';
  const hasImages = isRichMode ? false : containsInlineImageMarker(draft);

  if (draft.trim() === '' && !hasImages) {
    message.error('不能发送空消息');
    return;
  }
  if (draft.length > 10000) {
    message.error('消息过长，请分段发送');
    return;
  }

  // 仅在 Plain 模式检查图片上传状态
  if (!isRichMode) {
    if (hasUploadingInlineImages.value) {
      message.warning('仍有图片正在上传，请稍后再试');
      return;
    }
    if (hasFailedInlineImages.value) {
      message.error('存在上传失败的图片，请删除后重试');
      return;
    }
  }

  // 保存草稿（以防发送失败）
  saveDraft();

  const replyTo = chat.curReplyTo || undefined;
  stopTypingPreviewNow();
  suspendInlineSync = true;
  textToSend.value = '';
  suspendInlineSync = false;
  chat.curReplyTo = null;

  const now = Date.now();
  const clientId = nanoid();
  const wasAtBottom = isNearBottom();
  const tmpMsg: Message = {
    id: clientId,
    createdAt: now,
    updatedAt: now,
    content: draft,
    user: user.info,
    member: chat.curMember || undefined,
    quote: replyTo,
  };
  (tmpMsg as any).clientId = clientId;
  if (chat.curChannel) {
    (tmpMsg as any).channel = chat.curChannel;
  }

  const whisperTargetForSend = chat.whisperTarget;
  if (whisperTargetForSend) {
    (tmpMsg as any).isWhisper = true;
    (tmpMsg as any).whisperTo = whisperTargetForSend;
  }

  (tmpMsg as any).failed = false;
  rows.value.push(tmpMsg);
  instantMessages.add(tmpMsg);

  try {
    let finalContent: string;

    if (isRichMode) {
      // 富文本模式：直接发送 JSON
      finalContent = draft;
    } else {
      // 纯文本模式：转换为 HTML
      finalContent = await buildMessageHtml(draft);
    }

    tmpMsg.content = finalContent;
    const newMsg = await chat.messageCreate(finalContent, replyTo?.id, whisperTargetForSend?.id, clientId);
    for (const [k, v] of Object.entries(newMsg)) {
      (tmpMsg as any)[k] = v;
    }
    instantMessages.delete(tmpMsg);
    upsertMessage(tmpMsg);
    resetInlineImages();
    pendingInlineSelection = null;

    // 发送成功，清除草稿
    clearDraft();
    textToSend.value = '';
    ensureInputFocus();
  } catch (e) {
    message.error('发送失败,您可能没有权限在此频道发送消息');
    console.error('消息发送失败', e);
    suspendInlineSync = true;
    textToSend.value = draft;
    suspendInlineSync = false;
    syncInlineMarkersWithText(draft);
    const index = rows.value.findIndex(msg => msg.id === tmpMsg.id);
    if (index !== -1) {
      (rows.value[index] as any).failed = true;
    }
  }

  if (wasAtBottom) {
    toBottom();
  }
}, 500);

watch(textToSend, (value) => {
  handleWhisperCommand(value);
  if (isEditing.value) {
    chat.updateEditingDraft(value);
    emitEditingPreview();
  } else {
    emitTypingPreview();
  }
});

watch(filteredWhisperCandidates, (list) => {
  if (!list.length) {
    whisperSelectionIndex.value = 0;
  } else if (whisperSelectionIndex.value > list.length - 1) {
    whisperSelectionIndex.value = 0;
  }
});

watch(textToSend, (value) => {
  if (suspendInlineSync) {
    return;
  }
  syncInlineMarkersWithText(value);
});

watch(canOpenWhisperPanel, (canOpen) => {
  if (!canOpen && whisperPanelVisible.value && whisperPickerSource.value === 'manual') {
    closeWhisperPanel();
  }
});

watch(() => chat.whisperTarget, (target) => {
  if (target) {
    closeWhisperPanel();
    ensureInputFocus();
  }
});

watch(typingPreviewMode, (mode) => {
  localStorage.setItem(typingPreviewStorageKey, mode);
  if (mode === 'silent') {
    stopTypingPreviewNow();
    stopEditingPreviewNow();
    return;
  }
  if (typingPreviewActive.value && lastTypingChannelId) {
    const raw = textToSend.value;
    if (raw.trim().length > 0) {
      const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
      sendTypingUpdate.cancel();
      const content = mode === 'content' ? truncated : '';
      chat.messageTyping(mode, content, lastTypingChannelId);
    } else {
      stopTypingPreviewNow();
    }
  }
  if (mode === 'content' && isEditing.value) {
    emitEditingPreview();
  }
  if (mode !== 'content' && editingPreviewActive.value) {
    stopEditingPreviewNow();
  }
});

const isNearBottom = () => {
  const elLst = messagesListRef.value;
  if (!elLst) {
    return true;
  }
  const offset = elLst.scrollHeight - (elLst.clientHeight + elLst.scrollTop);
  return offset <= SCROLL_STICKY_THRESHOLD;
};

const toBottom = () => {
  scrollToBottom();
  showButton.value = false;
}

const doUpload = () => {
  pendingInlineSelection = captureSelectionRange();
  inlineImageInputRef.value?.click?.();
}

const handleRichUploadButtonClick = () => {
  // 富文本编辑器内的上传按钮点击事件
  doUpload();
}

const toggleInputMode = () => {
  if (inputMode.value === 'plain') {
    inputMode.value = 'rich';
    message.info('已切换至富文本模式');
  } else {
    inputMode.value = 'plain';
    message.info('已切换至纯文本模式');
  }
  ensureInputFocus();
}

const isMe = (item: Message) => {
  return user.info.id === item.user?.id;
}

const scrollToBottom = () => {
  // virtualListRef.value?.scrollToBottom();
  nextTick(() => {
    const elLst = messagesListRef.value;
    if (elLst) {
      elLst.scrollTop = elLst.scrollHeight;
    }
  });
}

const utils = useUtilsStore();

const emit = defineEmits(['drawer-show'])

let firstLoad = false;
onMounted(async () => {
  await chat.tryInit();
  await utils.configGet();
  await utils.commandsRefresh();

  chat.channelRefreshSetup()

  // 检查是否有保存的草稿
  loadDraft();

  const sound = new Howl({
    src: [SoundMessageCreated],
    html5: true
  });

  chatEvent.off('message-deleted', '*');
  chatEvent.on('message-deleted', (e?: Event) => {
    console.log('delete', e?.message?.id)
    for (let i of rows.value) {
      if (i.id === e?.message?.id) {
        i.content = '';
        (i as any).is_revoked = true;
      }
      if (i.quote) {
        if (i.quote?.id === e?.message?.id) {
          i.quote.content = '';
          (i as any).quote.is_revoked = true;
        }
      }
    }
  });

chatEvent.off('message-created', '*');
chatEvent.on('message-created', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const shouldStick = isNearBottom();
  const incoming = normalizeMessageShape(e.message);
  const isSelf = incoming.user?.id === user.info.id;
  if (isSelf) {
    let matchedPending: Message | undefined;
    const clientId = (incoming as any).clientId;
    if (clientId) {
      for (const pending of instantMessages) {
        if ((pending as any).clientId === clientId) {
          matchedPending = pending;
          break;
        }
      }
    } else {
      for (const pending of instantMessages) {
        if ((pending as any).content === incoming.content) {
          matchedPending = pending;
          break;
        }
      }
    }
    if (matchedPending) {
      instantMessages.delete(matchedPending);
      Object.assign(matchedPending, incoming);
      upsertMessage(matchedPending);
      removeTypingPreview(incoming.user?.id);
      removeTypingPreview(incoming.user?.id, 'editing');
      if (shouldStick) {
        toBottom();
      }
      return;
    }
  } else {
    sound.play();
  }
  upsertMessage(incoming);
  removeTypingPreview(incoming.user?.id);
  removeTypingPreview(incoming.user?.id, 'editing');
  if (shouldStick) {
    toBottom();
  }
});

chatEvent.off('message-updated', '*');
chatEvent.on('message-updated', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  upsertMessage(e.message);
  removeTypingPreview(e.user?.id, 'editing');
  if (chat.editing && chat.editing.messageId === e.message.id) {
    stopEditingPreviewNow();
    chat.cancelEditing();
    textToSend.value = '';
    ensureInputFocus();
  }
});

chatEvent.off('message-reordered', '*');
chatEvent.on('message-reordered', (e?: Event) => {
  if (!e || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const reorderPayload = (e as any)?.reorder;
  if (e.message) {
    upsertMessage(normalizeMessageShape(e.message));
  } else if (reorderPayload) {
    applyReorderPayload(reorderPayload);
  }
  const clientOpId = reorderPayload?.clientOpId;
  if (clientOpId && localReorderOps.has(clientOpId)) {
    localReorderOps.delete(clientOpId);
  }
});

chatEvent.off('typing-preview', '*');
chatEvent.on('typing-preview', (e?: Event) => {
  if (!e?.channel || e.channel.id !== chat.curChannel?.id) {
    return;
  }
  const typingUserId = e.user?.id;
  if (!typingUserId || typingUserId === user.info.id) {
    return;
  }
  const mode = e.typing?.mode === 'editing' ? 'editing' : 'typing';
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
  upsertTypingPreview({
    userId: typingUserId,
    displayName: e.member?.nick || e.user?.nick || '未知成员',
    avatar: e.member?.avatar || e.user?.avatar || '',
    content: typingState === 'content' ? (e.typing?.content || '') : '',
    indicatorOnly: typingState !== 'content' || !e.typing?.content,
    mode,
    messageId: e.typing?.messageId,
  });
});

  chatEvent.off('channel-deleted', '*');
  chatEvent.on('channel-deleted', (e) => {
    if (e) {
      // 当前频道没了，直接进行重载
      chat.channelSwitchTo(chat.channelTree[0].id);
    }
  })

  chatEvent.on('channel-member-updated', (e) => {
    if (e) {
      // 此事件只有member
      for (let i of rows.value) {
        if (i.user?.id === e.member?.user?.id) {
          (i as any).member.nick = e?.member?.nick
        }
      }
      if ((chat.curMember as any).id === (e as any).member?.id) {
        chat.curMember = e.member as any;
      }
    }
  })

  chatEvent.on('connected', async (e) => {
    // 重连了之后，重新加载这之间的数据
    console.log('尝试获取重连数据')
    stopTypingPreviewNow();
    resetTypingPreview();
    if (rows.value.length > 0) {
      let now = Date.now();
      const lastCreatedAt = rows.value[rows.value.length - 1].createdAt || now;

      // 获取断线期间消息
      const messages = await chat.messageListDuring(chat.curChannel?.id || '', lastCreatedAt, now)
      console.log('时间起始', lastCreatedAt, now)
      console.log('相关数据', messages)
      if (messages.next) {
        //  如果大于30个，那么基本上清除历史
        messagesNextFlag.value = messages.next || "";
        rows.value = rows.value.filter((i) => i.createdAt || now > lastCreatedAt);
      }
      // 插入新数据
      rows.value.push(...normalizeMessageList(messages.data));
      sortRowsByDisplayOrder();

      // 滚动到最下方
      nextTick(() => {
        scrollToBottom();
        showButton.value = false;
      })
    } else {
      await loadMessages();
    }
  })

  chatEvent.on('channel-switch-to', (e) => {
    if (!firstLoad) return;
    stopTypingPreviewNow();
    resetTypingPreview();
    stopEditingPreviewNow();
  chat.cancelEditing();
  textToSend.value = '';
  rows.value = []
  resetDragState();
  localReorderOps.clear();
  showButton.value = false;
    // 具体不知道原因，但是必须在这个位置reset才行
    // virtualListRef.value?.reset();
    loadMessages();
  })

  await loadMessages();
  firstLoad = true;
})

onBeforeUnmount(() => {
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  resetTypingPreview();
  cancelDrag();
});

const messagesNextFlag = ref("");

const loadMessages = async () => {
  resetTypingPreview();
  const messages = await chat.messageList(chat.curChannel?.id || '');
  messagesNextFlag.value = messages.next || "";
  rows.value.push(...normalizeMessageList(messages.data));
  sortRowsByDisplayOrder();

  nextTick(() => {
    scrollToBottom();
    showButton.value = false;
  })
}

const showButton = ref(false)
const onScroll = (evt: any) => {
  // 会打断输入，不要blur
  // if (textInputRef.value?.blur) {
  //   (textInputRef.value as any).blur()
  // }
  // console.log(222, messagesListRef.value?.scrollTop, messagesListRef.value?.scrollHeight)
  if (messagesListRef.value) {
    const elLst = messagesListRef.value;
    const offset = elLst.scrollHeight - (elLst.clientHeight + elLst.scrollTop);
    showButton.value = offset > SCROLL_STICKY_THRESHOLD;

    if (elLst.scrollTop === 0) {
      //  首次加载前不触发
      if (!firstLoad) return;
      reachTop(evt);
    }
  }
  // const vl = virtualListRef.value;
  // showButton.value = vl.clientRef.itemRefEl.clientHeight - vl.getOffset() > vl.clientRef.itemRefEl.clientHeight / 2
}

const pauseKeydown = ref(false);

const handleMentionSelect = () => {
  pauseKeydown.value = false;
};

const keyDown = function (e: KeyboardEvent) {
  if (pauseKeydown.value) return;

  if (!isEditing.value && handleWhisperKeydown(e)) {
    return;
  }

  // 检查是否为移动端
  if (/Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)) {
    // 如果是移动端,直接返回,不执行后续代码
    return;
  }

  if (e.key === 'Backspace' && chat.whisperTarget) {
    const selection = getInputSelection();
    if (selection.start === 0 && selection.end === 0 && textToSend.value.length === 0) {
      clearWhisperTarget();
      e.preventDefault();
      return;
    }
  }

  if (e.key === 'Escape' && isEditing.value) {
    cancelEditing();
    e.preventDefault();
    return;
  }

  if (e.key === 'Enter' && (!e.ctrlKey) && (!e.shiftKey)) {
    if (isEditing.value) {
      saveEdit();
    } else {
      send();
    }
    e.preventDefault();
  }
}

const atOptions = ref<MentionOption[]>([])
const atLoading = ref(true)
const atRenderLabel = (option: MentionOption) => {
  switch (option.type) {
    case 'cmd':
      return <div class="flex items-center space-x-1">
        <span>{(option as any).data.info}</span>
      </div>
    case 'at':
      return <div class="flex items-center space-x-1">
        <AvatarVue size={24} border={false} src={(option as any).data?.avatar} />
        <span>{option.label}</span>
      </div>
  }
}

const atPrefix = computed(() => chat.atOptionsOn ? ['@', '/', '.'] : ['@']);

const atHandleSearch = async (pattern: string, prefix: string) => {
  pauseKeydown.value = true;
  atLoading.value = true;

  const atElementCheck = () => {
    const els = document.getElementsByClassName("v-binder-follower-content");
    if (els.length) {
      return els[0].children.length > 0;
    }
    return false;
  }

  // 如果at框非正常消失，那么也一样要恢复回车键功能
  let x = setInterval(() => {
    if (!atElementCheck()) {
      pauseKeydown.value = false;
      clearInterval(x);
    }
  }, 100)

  const cmdCheck = () => {
    const text = textToSend.value.trim();
    if (text.startsWith(prefix)) {
      return true;
    }
  }

  switch (prefix) {
    case '@': {
      const lst = (await chat.guildMemberList('')).data.map((i: any) => {
        return {
          type: 'at',
          value: i.nick,
          label: i.nick,
          data: i,
        }
      })
      atOptions.value = lst;
      break;
    }
    case '.': case '/':
      // 好像暂时没法组织他弹出
      // if (!cmdCheck()) {
      //   atLoading.value = false;
      //   pauseKeydown.value = false;
      //   return;
      // }

      if (chat.atOptionsOn) {
        atOptions.value = [[`x`, 'x d100'],].map((i) => {
          return {
            type: 'cmd',
            value: i[0],
            label: i[0],
            data: {
              "info": '/x 简易骰点指令，如：/x d100 (100面骰)'
            }
          }
        });

        for (let [id, data] of Object.entries(utils.botCommands)) {
          for (let [k, v] of Object.entries(data)) {
            atOptions.value.push({
              type: 'cmd',
              value: k,
              label: k,
              data: {
                "info": `/${k} ` + (v as any).split('\n', 1)[0].replace(/^\.\S+/, '')
              }
            })
          }
        }
      }
      break;
  }

  atLoading.value = false;
}

let recentReachTopNext = '';

const reachTop = throttle(async (evt: any) => {
  console.log('reachTop', messagesNextFlag.value)
  if (recentReachTopNext === messagesNextFlag.value) return;
  recentReachTopNext = messagesNextFlag.value;

  if (messagesNextFlag.value) {
    const messages = await chat.messageList(chat.curChannel?.id || '', messagesNextFlag.value);
    messagesNextFlag.value = messages.next || "";

    let oldId = '';
    if (rows.value.length) {
      oldId = rows.value[0].id || '';
    }

    rows.value.unshift(...normalizeMessageList(messages.data));
    sortRowsByDisplayOrder();

    nextTick(() => {
      // 注意: el会变，如果不在下一帧取的话
      const el = document.getElementById(oldId)
      VueScrollTo.scrollTo(el, {
        container: messagesListRef.value,
        duration: 0,
        offset: 0,
      })
    })
    // virtualListRef.value?.scrollToIndex(messages.data.length);
  }
}, 1000)

const sendEmoji = throttle(async (i: UserEmojiModel) => {
  const resp = await chat.messageCreate(`<img src="id:${i.attachmentId}" />`);
  emojiPopoverShow.value = false;
  if (!resp) {
    message.error('发送失败,您可能没有权限在此频道发送消息');
    return;
  }
  toBottom();
}, 1000);

const avatarLongpress = (data: any) => {
  if (data.user) {
    textToSend.value += `@${data.user.nick} `;
    textInputRef.value?.focus();
  }
}

const selectedEmojiIds = ref<string[]>([]);

const emojiSelectedDelete = async () => {
  if (!await dialogAskConfirm(dialog)) return;

  if (selectedEmojiIds.value.length > 0) {
    await user.emojiDelete(selectedEmojiIds.value);
    // 例如：调用API删除表情，然后更新本地状态
    console.log('删除选中的表情：', selectedEmojiIds.value);
    // 删除后清空选中状态
    selectedEmojiIds.value = [];
    user.emojiCount++;
  } else {
    console.log('没有选中的表情可删除');
  }
}

const emojiPopoverShow = ref(false);
const isManagingEmoji = ref(false);
</script>

<template>
  <div class="flex flex-col h-full justify-between">
    <div class="chat overflow-y-auto h-full px-4 pt-6" v-show="rows.length > 0" @scroll="onScroll"
      ref="messagesListRef">
      <!-- <VirtualList itemKey="id" :list="rows" :minSize="50" ref="virtualListRef" @scroll="onScroll"
              @toBottom="reachBottom" @toTop="reachTop"> -->
      <template v-for="itemData in rows" :key="itemData.id">
        <div
          :class="rowClass(itemData)"
          :data-message-id="itemData.id"
          :ref="el => registerMessageRow(el as HTMLElement | null, itemData.id || '')"
        >
          <div
            v-if="shouldShowHandle(itemData)"
            class="message-row__handle"
            tabindex="-1"
            @pointerdown="onDragHandlePointerDown($event, itemData)"
          >
            <span class="message-row__dot" v-for="n in 6" :key="n"></span>
          </div>
          <chat-item
            :avatar="itemData.member?.avatar || itemData.user?.avatar"
            :username="itemData.member?.nick ?? '未知'"
            :content="itemData.content"
            :is-rtl="isMe(itemData)"
            :item="itemData"
            :editing-preview="editingPreviewMap[itemData.id]"
            @avatar-longpress="avatarLongpress(itemData)"
            @edit="beginEdit(itemData)"
            @edit-save="saveEdit"
            @edit-cancel="cancelEditing"
          />
        </div>
      </template>

      <template v-for="preview in typingPreviewItems" :key="`${preview.userId}-typing`">
        <div class="typing-preview-item">
          <AvatarVue :border="false" :size="40" :src="preview.avatar" />
          <div :class="['typing-preview-bubble', preview.indicatorOnly ? '' : 'typing-preview-bubble--content']">
            <div class="typing-preview-bubble__header">
              <div class="typing-preview-bubble__meta">
                <span class="typing-preview-bubble__name">{{ preview.displayName }}</span>
                <span class="typing-preview-bubble__tag">
                  {{ preview.indicatorOnly ? '正在输入' : '实时内容' }}
                </span>
              </div>
              <span class="typing-dots">
                <span></span>
                <span></span>
                <span></span>
              </span>
            </div>
            <div class="typing-preview-bubble__body"
              :class="{ 'typing-preview-bubble__placeholder': preview.indicatorOnly }">
              <template v-if="preview.indicatorOnly">
                正在输入
              </template>
              <template v-else>
                <div v-html="renderPreviewContent(preview.content)" class="preview-content"></div>
              </template>
            </div>
          </div>
        </div>
      </template>

      <!-- <VirtualList itemKey="id" :list="rows" :minSize="50" ref="virtualListRef" @scroll="onScroll"
              @toBottom="reachBottom" @toTop="reachTop">
              <template #default="{ itemData }">
                <chat-item :avatar="imgAvatar" :username="itemData.member?.nick" :content="itemData.content"
                  :is-rtl="isMe(itemData)" :createdAt="itemData.createdAt" />
              </template>
            </VirtualList> -->
    </div>
    <div v-if="rows.length === 0" class="flex h-full items-center text-2xl justify-center text-gray-400">说点什么吧</div>

    <div style="right: 20px ;bottom: 70px;" class=" fixed" v-if="showButton">
      <n-button size="large" circle color="#e5e7eb" @click="toBottom">
        <template #icon>
          <n-icon class="text-black">
            <ArrowBarToDown />
          </n-icon>
        </template>
      </n-button>
    </div>

    <!-- flex-grow -->
    <div class="edit-area flex justify-between space-x-2 my-2 px-2 relative">

      <!-- 左下，快捷指令栏 -->
      <div class="absolute  px-4 py-2" style="top: -2.7rem; left: 0rem" v-if="true">
        <div class="bg-white">
          <n-button @click="emit('drawer-show')" size="small" v-if="utils.isSmallPage">
            <template #icon>
              <n-icon :component="IconNumber"></n-icon>
            </template>
          </n-button>
        </div>
      </div>

      <div class="absolute bg-sky-300 rounded px-4 py-2" style="top: -4rem; right: 1rem" v-if="chat.curReplyTo">
        正在回复: {{ chat.curReplyTo.member?.nick }}
        <n-button @click="chat.curReplyTo = null">取消</n-button>
      </div>

      <div class="chat-input-container flex flex-col w-full relative">
        <transition name="fade">
          <div v-if="whisperPanelVisible" class="whisper-panel" @mousedown.stop>
            <div class="whisper-panel__title">{{ t('inputBox.whisperPanelTitle') }}</div>
            <n-input v-if="whisperPickerSource === 'manual'" ref="whisperSearchInputRef"
              v-model:value="whisperQuery" size="small" :placeholder="t('inputBox.whisperSearchPlaceholder')" clearable
              @keydown="handleWhisperKeydown" />
            <div class="whisper-panel__list" @keydown="handleWhisperKeydown">
              <div v-for="(candidate, idx) in filteredWhisperCandidates" :key="candidate.id"
                class="whisper-panel__item" :class="{ 'is-active': idx === whisperSelectionIndex }"
                @mousedown.prevent @mouseenter="whisperSelectionIndex = idx"
                @click="applyWhisperTarget(candidate)">
                <AvatarVue :border="false" :size="32" :src="candidate.avatar" />
                <div class="whisper-panel__meta">
                  <div class="whisper-panel__name">{{ candidate.displayName }}</div>
                  <div v-if="candidate.secondaryName" class="whisper-panel__sub">@{{ candidate.secondaryName }}</div>
                </div>
              </div>
              <div v-if="!filteredWhisperCandidates.length" class="whisper-panel__empty">{{ t('inputBox.whisperEmpty') }}</div>
            </div>
          </div>
        </transition>

        <div class="chat-input-area relative flex-1">
            <div v-if="whisperMode" class="whisper-pill" @mousedown.prevent>
              <span class="whisper-pill__label">{{ t('inputBox.whisperPillPrefix') }} @{{ whisperTargetDisplay }}</span>
              <button type="button" class="whisper-pill__close" @click="clearWhisperTarget">×</button>
            </div>

            <ChatInputSwitcher
              ref="textInputRef"
              v-model="textToSend"
              v-model:mode="inputMode"
              :placeholder="whisperMode ? whisperPlaceholderText : $t('inputBox.placeholder')"
              :whisper-mode="whisperMode"
              :mention-options="atOptions"
              :mention-loading="atLoading"
              :mention-prefix="atPrefix"
              :mention-render-label="atRenderLabel"
              :rows="1"
          :inline-images="inlineImagePreviewMap"
          @mention-search="atHandleSearch"
          @mention-select="handleMentionSelect"
          @keydown="keyDown"
          @paste-image="handlePlainPasteImage"
          @drop-files="handlePlainDropFiles"
          @upload-button-click="handleRichUploadButtonClick"
          @remove-image="removeInlineImage"
        />
            <input
              ref="inlineImageInputRef"
              class="hidden"
              type="file"
              accept="image/*"
              multiple
              @change="handleInlineFileChange"
            />
        </div>
        <div class="chat-input-actions flex items-center justify-between gap-2 mt-2">
          <div class="chat-input-actions__group chat-input-actions__group--addons">
            <div class="chat-input-actions__cell">
              <n-popover v-model:show="emojiPopoverShow" trigger="click">
                <template #trigger>
                  <n-button quaternary circle :disabled="isEditing">
                    <template #icon>
                      <n-icon :component="Plus" size="18" />
                    </template>
                  </n-button>
                </template>

                <div class="flex justify-between items-center">
                  <div class="text-base mb-1">{{ $t('inputBox.emojiTitle') }}</div>
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-button text size="small" @click="isManagingEmoji = !isManagingEmoji">
                        <template #icon>
                          <n-icon :component="Settings" />
                        </template>
                      </n-button>
                    </template>
                    表情管理
                  </n-tooltip>
                </div>

                <div v-if="!uploadImages?.length" class="flex justify-center w-full py-4 px-4">
                  <div class="w-56">当前没有收藏的表情，可以在聊天窗口的图片上<b class="px-1">长按</b>或<b class="px-1">右键</b>添加</div>
                </div>

                <template v-else>
                  <template v-if="isManagingEmoji">
                    <n-checkbox-group v-model:value="selectedEmojiIds">
                      <div class="grid grid-cols-4 gap-4 pt-2 pb-4">
                        <div class="cursor-pointer" v-for="i in uploadImages" :key="i.id">
                          <n-checkbox :value="i.id" class="mt-2">
                            <img :src="getSrc(i)"
                              style="width: 4.8rem; height: 4.8rem; object-fit: contain; cursor: pointer;" />
                          </n-checkbox>
                        </div>
                      </div>
                    </n-checkbox-group>

                    <div class="flex justify-end space-x-2 mb-4">
                      <n-button type="info" size="small" @click="emojiSelectedDelete" :disabled="selectedEmojiIds.length === 0">
                        删除选中
                      </n-button>
                      <n-button type="default" size="small" @click="() => { isManagingEmoji = false; selectedEmojiIds = []; }">
                        退出管理
                      </n-button>
                    </div>
                  </template>

                  <template v-else>
                    <div class="grid grid-cols-4 gap-4 pt-2 pb-4">
                      <div class="cursor-pointer" v-for="i in uploadImages" :key="i.id">
                        <img @click="sendEmoji(i)" :src="getSrc(i)"
                          style="width: 4.8rem; height: 4.8rem; object-fit: contain;" />
                      </div>
                    </div>
                  </template>
                </template>
              </n-popover>
            </div>

           <div class="chat-input-actions__cell">
             <n-tooltip trigger="hover">
               <template #trigger>
                 <n-button quaternary circle class="whisper-toggle-button" :class="{ 'whisper-toggle-button--active': whisperMode }"
                   @click="startWhisperSelection" :disabled="!canOpenWhisperPanel || isEditing">
                    <span class="chat-input-actions__icon">W</span>
                  </n-button>
                </template>
                {{ t('inputBox.whisperTooltip') }}
              </n-tooltip>
            </div>

            <div class="chat-input-actions__cell">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button quaternary circle class="typing-toggle" :class="typingToggleClass"
                    @click="toggleTypingPreview" :disabled="isEditing">
                    <span class="chat-input-actions__icon">👁</span>
                  </n-button>
                </template>
                {{ typingPreviewTooltip }}
              </n-tooltip>
            </div>
            <div class="chat-input-actions__cell">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button quaternary circle @click="doUpload" :disabled="isEditing">
                    <template #icon>
                      <n-icon :component="Upload" size="18" />
                    </template>
                  </n-button>
                </template>
                上传图片
              </n-tooltip>
            </div>

            <div class="chat-input-actions__cell">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    quaternary
                    circle
                    :type="inputMode === 'rich' ? 'primary' : 'default'"
                    @click="toggleInputMode"
                    :disabled="isEditing"
                  >
                    <span class="font-semibold">{{ inputMode === 'rich' ? 'P' : 'R' }}</span>
                  </n-button>
                </template>
                {{ inputMode === 'rich' ? '切换到纯文本模式' : '切换到富文本模式' }}
              </n-tooltip>
            </div>

            <div v-if="hasSavedDraft" class="chat-input-actions__cell">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button quaternary circle @click="restoreDraft" :disabled="isEditing">
                    <template #icon>
                      <n-icon :component="RotateClockwise" size="18" />
                    </template>
                  </n-button>
                </template>
                恢复上次未发送的内容
              </n-tooltip>
            </div>
          </div>

          <div class="chat-input-actions__cell chat-input-actions__send">
            <n-button type="primary" circle size="large" @click="send"
              :disabled="chat.connectState !== 'connected' || isEditing">
              <template #icon>
                <n-icon :component="Send" size="20" />
              </template>
            </n-button>
          </div>
        </div>
      </div>
    </div>
  </div>

  <RightClickMenu />
  <AvatarClickMenu />
</template>

<style lang="scss" scoped>
.message-row {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  position: relative;
  padding-left: 0.25rem;
}

.message-row--self {
  flex-direction: row-reverse;
}

.message-row__handle {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  min-height: 100%;
  cursor: grab;
  opacity: 0;
  transition: opacity 0.2s ease;
  margin-top: 0.5rem;
}

.message-row.draggable-item:hover .message-row__handle,
.message-row.draggable-item:focus-within .message-row__handle {
  opacity: 1;
}

.message-row__handle:active {
  cursor: grabbing;
}

.message-row__dot {
  width: 0.2rem;
  height: 0.2rem;
  margin: 0.12rem 0;
  background-color: #9ca3af;
  border-radius: 50%;
}

.message-row--drag-source {
  opacity: 0.4;
}

.message-row__ghost {
  box-shadow: 0 12px 24px rgba(30, 64, 175, 0.25);
  border-radius: 0.75rem;
}

.message-row--drop-before::after,
.message-row--drop-after::after {
  content: "";
  position: absolute;
  left: 0.5rem;
  right: 0.5rem;
  border-top: 2px solid rgba(59, 130, 246, 0.8);
  box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.15);
  pointer-events: none;
}

.message-row--drop-before::after {
  top: -0.3rem;
}

.message-row--drop-after::after {
  bottom: -0.3rem;
}

@media (hover: none) {
  .message-row__handle {
    opacity: 1;
  }
}

.chat>.virtual-list__client {
  @apply px-4 pt-4;

  &>div {
    margin-bottom: -1rem;
  }
}

.chat-item {
  @apply pb-8; // margin会抖动，pb不会
}

.typing-preview-item {
  display: flex;
  align-items: flex-end;
  gap: 0.75rem;
  margin-top: 0.75rem;
  font-size: 0.9375rem;
  color: #4b5563;
}

.typing-preview-bubble {
  max-width: 28rem;
  padding: 0.6rem 0.9rem;
  border-radius: 1rem;
  border: 1px dashed rgba(107, 114, 128, 0.65);
  background-color: rgba(243, 244, 246, 0.85);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(2px);
}

.typing-preview-bubble--content {
  border-color: rgba(59, 130, 246, 0.55);
  background-color: rgba(219, 234, 254, 0.95);
  color: #1d4ed8;
}

.typing-preview-bubble__footer {
  margin-top: 0.5rem;
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.typing-preview-bubble__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.35rem;
}

.typing-preview-bubble__meta {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.typing-preview-bubble__name {
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: #4b5563;
}

.typing-preview-bubble--content .typing-preview-bubble__name {
  color: #1e3a8a;
}

.typing-preview-bubble__tag {
  font-size: 0.625rem;
  padding: 0.1rem 0.4rem;
  border-radius: 9999px;
  background-color: rgba(156, 163, 175, 0.18);
  color: #4b5563;
  font-weight: 500;
}

.typing-preview-bubble--content .typing-preview-bubble__tag {
  background-color: rgba(59, 130, 246, 0.18);
  color: #1d4ed8;
}

.typing-preview-bubble__body {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  font-size: 0.9375rem;
}

.typing-preview-bubble__placeholder {
  color: #6b7280;
}

.preview-content {
  max-width: 100%;

  p {
    margin: 0;
    line-height: 1.5;
  }

  p + p {
    margin-top: 0.5rem;
  }

  strong {
    font-weight: 600;
  }

  em {
    font-style: italic;
  }

  u {
    text-decoration: underline;
  }

  s {
    text-decoration: line-through;
  }

  code {
    background-color: rgba(0, 0, 0, 0.05);
    border-radius: 0.25rem;
    padding: 0.125rem 0.375rem;
    font-family: 'Courier New', monospace;
    font-size: 0.9em;
  }
}

.preview-inline-image {
  max-height: 3rem;
  max-width: 6rem;
  border-radius: 0.375rem;
  vertical-align: middle;
  margin: 0 0.25rem;
  object-fit: contain;
}

.preview-image-placeholder {
  display: inline-block;
  padding: 0.125rem 0.375rem;
  background-color: rgba(0, 0, 0, 0.05);
  border-radius: 0.25rem;
  font-size: 0.75rem;
}

.typing-dots {
  display: inline-flex;
  align-items: center;
}

.typing-dots span {
  width: 0.35rem;
  height: 0.35rem;
  margin-left: 0.18rem;
  border-radius: 9999px;
  background-color: rgba(107, 114, 128, 0.9);
  animation: typing-dots 1.2s infinite ease-in-out;
}

.typing-preview-bubble--content .typing-dots span {
  background-color: rgba(37, 99, 235, 0.85);
}

.typing-dots span:first-child {
  margin-left: 0;
}

.typing-dots span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-dots span:nth-child(3) {
  animation-delay: 0.4s;
}

.typing-toggle {
  transition: color 0.2s ease;
}

.typing-toggle--indicator {
  color: #9ca3af;
}

.typing-toggle--indicator:hover {
  color: #6b7280;
}

.typing-toggle--content {
  color: #2563eb;
}

.typing-toggle--content:hover {
  color: #1d4ed8;
}

.typing-toggle--silent {
  color: #f59e0b;
}

.typing-toggle--silent:hover {
  color: #d97706;
}

.chat-input-container {
  width: 100%;
}

.chat-input-area {
  position: relative;
  display: flex;
  flex-direction: column;
}

.chat-input-area :deep(.n-input) {
  width: 100%;
}

.chat-input-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-top: 0.75rem;
}

.chat-input-actions__group {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}

.chat-input-actions__cell .n-button {
  width: 42px;
  height: 42px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.chat-input-actions__cell .n-button:disabled {
  opacity: 0.55;
}

.chat-input-actions__icon {
  display: inline-flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: center;
  font-weight: 600;
}

.chat-input-actions__send .n-button {
  width: 44px;
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.chat-text :deep(textarea) {
  padding: 0.75rem 1.25rem;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease, padding-top 0.2s ease;
}

.chat-text.whisper-mode :deep(textarea) {
  border-color: #7c3aed;
  box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.35);
  background-color: rgba(250, 245, 255, 0.92);
  padding-top: 1.35rem;
}

.whisper-pill {
  position: absolute;
  top: 0.35rem;
  left: 1.1rem;
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  background-color: rgba(124, 58, 237, 0.14);
  color: #5b21b6;
  font-size: 0.85rem;
  font-weight: 500;
  z-index: 2;
}

.whisper-pill__close {
  border: none;
  background: transparent;
  color: inherit;
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
  padding: 0;
}

.whisper-pill__close:hover {
  color: #4c1d95;
}

.whisper-panel {
  position: absolute;
  bottom: calc(100% + 0.75rem);
  left: 0;
  right: 0;
  margin: 0 auto;
  max-width: 340px;
  background: #ffffff;
  border-radius: 0.75rem;
  border: 1px solid rgba(124, 58, 237, 0.22);
  box-shadow: 0 18px 40px rgba(99, 102, 241, 0.18);
  padding: 0.75rem;
  z-index: 6;
}

.whisper-panel__title {
  font-size: 0.85rem;
  font-weight: 600;
  color: #5b21b6;
  margin-bottom: 0.4rem;
}

.whisper-panel__list {
  max-height: 220px;
  overflow-y: auto;
  margin-top: 0.4rem;
  padding-right: 0.2rem;
}

.whisper-panel__item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.45rem 0.55rem;
  border-radius: 0.65rem;
  cursor: pointer;
  transition: background-color 0.16s ease;
}

.whisper-panel__item:hover,
.whisper-panel__item.is-active {
  background: rgba(124, 58, 237, 0.14);
}

.whisper-panel__meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.whisper-panel__name {
  font-size: 0.9rem;
  font-weight: 600;
  color: #4338ca;
}

.whisper-panel__sub {
  font-size: 0.75rem;
  color: #6b7280;
}

.whisper-panel__empty {
  padding: 0.75rem 0.5rem;
  text-align: center;
  font-size: 0.85rem;
  color: #9ca3af;
}

.whisper-toggle-button {
  color: #6b7280;
}

.whisper-toggle-button--active {
  color: #7c3aed;
}

.whisper-toggle-button:disabled {
  color: #c5c5c5;
  cursor: not-allowed;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

@keyframes typing-dots {
  0%, 80%, 100% {
    transform: scale(0.4);
    opacity: 0.35;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}
</style>

<style lang="scss">
.chat>.virtual-list__client {
  &>div {
    margin-bottom: -1rem;
  }
}

.chat-text>.n-input>.n-input-wrapper {
  @apply bg-gray-100;
  padding: 0.75rem 1.25rem;
  border-radius: 0.85rem;
}
</style>
