<script setup lang="tsx">
import ChatItem from './components/chat-item.vue';
import { computed, ref, watch, h, onMounted, onBeforeMount, onBeforeUnmount, nextTick, reactive } from 'vue'
import { VirtualList } from 'vue-tiny-virtual-list';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { Event, Message, User } from '@satorijs/protocol'
import { useUserStore } from '@/stores/user';
import { ArrowBarToDown, Plus, Upload, Eye, EyeOff, Lock } from '@vicons/tabler'
import { NIcon, c, useDialog, useMessage, type MentionOption } from 'naive-ui';
import VueScrollTo from 'vue-scrollto'
import UploadSupport from './components/upload.vue'
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
const uploadSupportRef = ref<any>(null);
const messagesListRef = ref<HTMLElement | null>(null);
const textInputRef = ref<any>(null);

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
  if (msg.quote) {
    msg.quote = normalizeMessageShape(msg.quote);
  }
  return msg as Message;
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
  // 重新赋值触发渲染更新，避免在部分浏览器中出现静默不刷新的情况
  rows.value = rows.value.slice();
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
  if (!content) {
    return '';
  }
  let text = contentUnescape(content);
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

const typingToggleIcon = computed(() => (typingPreviewMode.value === 'indicator' ? EyeOff : Eye));

const textToSend = ref('');
const editingPreviewMap = computed<Record<string, EditingPreviewInfo>>(() => {
  const map: Record<string, EditingPreviewInfo> = {};
  typingPreviewList.value.forEach((item) => {
    if (item.mode === 'editing' && item.messageId) {
      map[item.messageId] = {
        userId: item.userId,
        displayName: item.displayName,
        avatar: item.avatar,
        content: item.content || '',
        indicatorOnly: item.indicatorOnly,
        isSelf: item.userId === user.info.id,
      };
    }
  });
  if (isEditing.value && chat.editing) {
    const draft = textToSend.value;
    map[chat.editing.messageId] = {
      userId: user.info.id,
      displayName: chat.curMember?.nick || user.info.nick || user.info.name || '我',
      avatar: chat.curMember?.avatar || user.info.avatar || '',
      content: draft,
      indicatorOnly: draft.trim().length === 0,
      isSelf: true,
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
  chat.startEditingMessage({
    messageId: target.id,
    channelId: chat.curChannel.id,
    originalContent: target.content || '',
    draft: target.content || '',
  });
};

const cancelEditing = () => {
  if (!chat.editing) {
    return;
  }
  stopEditingPreviewNow();
  chat.cancelEditing();
  textToSend.value = '';
  stopTypingPreviewNow();
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
  if (draft.trim() === '') {
    message.error('消息内容不能为空');
    return;
  }
  if (draft.length > 10000) {
    message.error('消息过长，请分段编辑');
    return;
  }
  try {
    stopTypingPreviewNow();
    const escaped = contentEscape(draft);
    const replaced = await replaceUsernames(escaped);
    if (replaced.trim() === '') {
      message.error('消息内容不能为空');
      return;
    }
    const updated = await chat.messageUpdate(chat.editing.channelId, chat.editing.messageId, replaced);
    if (updated) {
      upsertMessage(updated as unknown as Message);
    }
    message.success('消息已更新');
    stopEditingPreviewNow();
    chat.cancelEditing();
    textToSend.value = '';
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

const getTextarea = () => {
  const el = textInputRef.value?.$el?.getElementsByTagName('textarea')[0];
  return el as HTMLTextAreaElement | undefined;
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
    const draft = convertMessageContentToDraft(chat.editing.draft);
    chat.curReplyTo = null;
    chat.clearWhisperTarget();
    textToSend.value = draft;
    chat.updateEditingDraft(draft);
    chat.messageMenu.show = false;
    stopTypingPreviewNow();
    ensureInputFocus();
    nextTick(() => {
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
  let t = textToSend.value;
  if (t.trim() === '') {
    message.error('不能发送空消息');
    return;
  }
  if (t.length > 10000) {
    message.error('消息过长，请分段发送');
    return;
  }
  const replyTo = chat.curReplyTo || undefined;
  stopTypingPreviewNow();
  textToSend.value = '';
  chat.curReplyTo = null;

  const now = Date.now();
  const clientId = nanoid();
  const wasAtBottom = isNearBottom();
  const tmpMsg: Message = {
    id: clientId,
    createdAt: now,
    updatedAt: now,
    content: t,
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
    t = contentEscape(t);
    t = await replaceUsernames(t);

    tmpMsg.content = t;
    const newMsg = await chat.messageCreate(t, replyTo?.id, whisperTargetForSend?.id, clientId);
    for (const [k, v] of Object.entries(newMsg)) {
      (tmpMsg as any)[k] = v;
    }
    instantMessages.delete(tmpMsg);
    upsertMessage(tmpMsg);
  } catch (e) {
    message.error('发送失败,您可能没有权限在此频道发送消息');
    console.error('消息发送失败', e);

    const index = rows.value.findIndex(msg => msg.id === tmpMsg.id);
    if (index !== -1) {
      (rows.value[index] as any).failed = true;
      // rows.value.splice(index, 1);
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
  uploadSupportRef.value.openUpload();
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

  const elInput = textInputRef.value;
  if (elInput) {
    // 注: n-mention 不支持这个事件监听，所以这里手动监听
    elInput.$el.getElementsByTagName('textarea')[0].onkeydown = keyDown;
  }

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
      // 为防止混乱，重新排序
      rows.value.sort((a, b) => (a.createdAt || now) - (b.createdAt || now));

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
});

const messagesNextFlag = ref("");

const loadMessages = async () => {
  resetTypingPreview();
  const messages = await chat.messageList(chat.curChannel?.id || '');
  messagesNextFlag.value = messages.next || "";
  rows.value.push(...normalizeMessageList(messages.data));
  rows.value = rows.value.slice();

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
    const textarea = getTextarea();
    if (textarea && textarea.selectionStart === 0 && textarea.selectionEnd === 0 && textToSend.value.length === 0) {
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
    rows.value = rows.value.slice();

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
      <template v-for="itemData in rows">
        <!-- {{itemData}} -->
        <chat-item :avatar="itemData.member?.avatar || itemData.user?.avatar" :username="itemData.member?.nick ?? '未知'"
          :content="itemData.content" :is-rtl="isMe(itemData)" :item="itemData"
          :editing-preview="editingPreviewMap[itemData.id]"
          @avatar-longpress="avatarLongpress(itemData)" @edit="beginEdit(itemData)"
          @edit-save="saveEdit" @edit-cancel="cancelEditing" />
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
                {{ preview.content }}
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

      <div class="flex justify-between relative w-full">
        <!-- 输入框左侧按钮，因为n-mention不支持#prefix和#suffix，所以单独拿出来了 -->
        <div class="absolute" style="z-index: 1; left: 0.5rem; top: .55rem;">
          <n-popover v-model:show="emojiPopoverShow" trigger="click">
            <template #trigger>
              <n-button text :disabled="isEditing">
                <template #icon>
                  <n-icon :component="Plus" size="20" />
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
                  <n-button type="default" size="small" @click="() => { isManagingEmoji = false; selectedEmojiIds = []; }" class="mr-2">
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

        <div class="absolute flex items-center space-x-2" style="z-index: 1; right: 0.6rem; top: .55rem;">
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button text class="whisper-toggle-button" :class="{ 'whisper-toggle-button--active': whisperMode }"
                @click="startWhisperSelection" :disabled="!canOpenWhisperPanel || isEditing">
                <template #icon>
                  <n-icon :component="Lock" size="20" />
                </template>
              </n-button>
            </template>
            {{ t('inputBox.whisperTooltip') }}
          </n-tooltip>

          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button text class="typing-toggle" :class="typingToggleClass"
                @click="toggleTypingPreview" :disabled="isEditing">
                <template #icon>
                  <n-icon :component="typingToggleIcon" size="20" />
                </template>
              </n-button>
            </template>
            {{ typingPreviewTooltip }}
          </n-tooltip>

          <n-popover trigger="hover">
            <template #trigger>
              <n-button text @click="doUpload" :disabled="isEditing">
                <template #icon>
                  <n-icon :component="Upload" size="20" />
                </template>
              </n-button>
            </template>
            <span>上传图片</span>
          </n-popover>
        </div>

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

        <div v-if="whisperMode" class="whisper-pill" @mousedown.prevent>
          <span class="whisper-pill__label">{{ t('inputBox.whisperPillPrefix') }} @{{ whisperTargetDisplay }}</span>
          <button type="button" class="whisper-pill__close" @click="clearWhisperTarget">×</button>
        </div>

        <n-mention type="textarea" :rows="1" autosize v-model:value="textToSend" :on-keydown="keyDown"
          ref="textInputRef" :class="['chat-text', { 'whisper-mode': whisperMode }]"
          :placeholder="whisperMode ? whisperPlaceholderText : $t('inputBox.placeholder')" :options="atOptions"
          :loading="atLoading" @search="atHandleSearch" @select="pauseKeydown = false" placement="top-start"
          :prefix="atPrefix" :render-label="atRenderLabel">
        </n-mention>
      </div>
      <div class="flex" style="align-items: end; padding-bottom: 1px;">
        <n-button class="" type="primary" @click="send" :disabled="chat.connectState !== 'connected' || isEditing">{{ 
          $t('inputBox.send') }}</n-button>
      </div>
    </div>
  </div>

  <RightClickMenu />
  <AvatarClickMenu />
  <upload-support ref="uploadSupportRef" />
</template>

<style lang="scss" scoped>
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

.chat-text :deep(textarea) {
  padding-left: 2.4rem;
  padding-right: 3rem;
  padding-top: 1.6rem;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease, padding-top 0.2s ease;
}

.chat-text.whisper-mode :deep(textarea) {
  border-color: #7c3aed;
  box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.35);
  background-color: rgba(250, 245, 255, 0.92);
  padding-top: 2.8rem;
}

.whisper-pill {
  position: absolute;
  top: 0.4rem;
  left: 2.5rem;
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
  left: 2.5rem;
  right: 2.5rem;
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
  @apply bg-gray-200;
  padding-top: 1.6rem;
}

.chat-text>.n-input>.n-input-wrapper {
  padding-left: 2.4rem;
  padding-right: 3rem;
}
</style>
