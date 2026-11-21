<script setup lang="ts">
// 单条原消息编辑：写回聊天原消息（message.update + expected_version/client_op_id）。
// 正文停止输入约 600ms 自动保存；输入法组字期间不提交；每条消息一个在途提交，新内容排队；
// 冲突保留本地输入并展示服务端最新内容，由用户选择，不自动覆盖。
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import AvatarVue from '@/components/avatar.vue';
import RichTextEditor from '@/components/rich-text/RichTextEditor.vue';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import {
  describeLogError,
  fetchChannelIdentitiesForLog,
  fetchIdentityManageCandidatesForLog,
  fetchLogMessageContext,
  isLogError,
  type LogIdentityOption,
  type LogIdentityVariantOption,
} from '@/services/log/logApi';
import {
  newClientOpId,
  updateMessageAppearance,
  updateMessageContent,
  updateMessageIdentity,
} from '@/services/log/logMessageCommands';
import { detectContentFormat, renderMessageBody } from '@/services/log/logContent';
import { resolveDesignSpeakerName } from '@/services/log/logRenderer';
import { convertTextContentFormat, type TextContentFormat } from '@/utils/textContentFormat';
import type { LogDesignMessage, LogProtocolMessage } from '@/types/log';
import type { ChannelIdentityManageCandidate } from '@/types';

const props = defineProps<{
  show: boolean;
  item: LogDesignMessage | null;
  tab: 'content' | 'identity' | 'appearance';
  projectId: string;
  sectionId: string;
  channelId: string;
  /** 当前 LOG 工程对该消息有正文覆盖（如批量校对结果）时为 true。 */
  contentOverridden?: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void;
  (e: 'update:tab', v: 'content' | 'identity' | 'appearance'): void;
  (e: 'saved', message: LogProtocolMessage, version: number): void;
  (e: 'clear-override'): void;
}>();

const toast = useMessage();
const SAVE_DELAY = 600;
const HEX = /^#[0-9a-fA-F]{6}$/;
const IDENTITY_SEARCH_THRESHOLD = 8;
const IDENTITY_OWNER_SEARCH_DELAY = 200;

// ---- 正文 ----
const messageId = ref('');
const text = ref('');
const format = ref<'tiptap' | 'plain'>('plain');
const baseVersion = ref(0);
const serverContent = ref('');
const contentStatus = ref<'idle' | 'dirty' | 'saving' | 'saved' | 'error' | 'conflict'>('idle');
const contentError = ref('');
const conflict = ref<{ content: string; version: number; plain: string } | null>(null);
const composing = ref(false);
let timer: ReturnType<typeof setTimeout> | null = null;
let inFlight: Promise<void> | null = null;
let lastSent = '';

const caps = computed(() => props.item?.capabilities);
const speaker = computed(() => (props.item ? resolveDesignSpeakerName(props.item.message) : ''));

const resetFrom = (item: LogDesignMessage) => {
  messageId.value = item.message.id;
  text.value = item.message.content || '';
  serverContent.value = item.message.content || '';
  lastSent = serverContent.value;
  format.value = detectContentFormat(serverContent.value);
  baseVersion.value = item.version;
  contentStatus.value = 'idle';
  contentError.value = '';
  conflict.value = null;
  resetAppearance(item);
  selectedIdentityOwnerUserId.value = item.message.user?.id || '';
  selectedIdentityOwner.value = null;
  identityOwners.value = [];
  identityOwnerTotal.value = 0;
  identityOwnerAvailable.value = false;
  identityOwnerLoading.value = false;
  identityOwnerSearch.value = '';
  identityOwnerPopover.value = false;
  if (ownerSearchTimer) clearTimeout(ownerSearchTimer);
  ownerSearchTimer = null;
  identities.value = [];
  variants.value = [];
  identityRequestSeq += 1;
  ownerRequestSeq += 1;
  identityId.value = item.message.identity?.id || '';
  variantId.value = item.message.identity?.variantId || '';
  identitySearch.value = '';
  variantSearch.value = '';
};


const scheduleSave = () => {
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => {
    timer = null;
    if (!composing.value) void saveContent();
  }, SAVE_DELAY);
};

const onTextChange = (value: string) => {
  text.value = value;
  if (contentStatus.value === 'conflict') return;
  if (value === lastSent && !inFlight) {
    contentStatus.value = 'saved';
    return;
  }
  contentStatus.value = 'dirty';
  if (!composing.value) scheduleSave();
};

const onCompositionStart = () => {
  composing.value = true;
};
const onCompositionEnd = () => {
  composing.value = false;
  if (contentStatus.value === 'dirty') scheduleSave();
};

const switchContentFormat = (next: TextContentFormat) => {
  if (next === format.value || contentStatus.value === 'conflict') return;
  const converted = convertTextContentFormat(text.value, format.value, next);
  format.value = next;
  onTextChange(converted);
};

const saveContent = async (): Promise<void> => {
  if (!props.item || !caps.value?.edit || contentStatus.value === 'conflict') return;
  if (inFlight) {
    await inFlight;
    return saveContent();
  }
  const snapshot = text.value;
  if (snapshot === lastSent) {
    contentStatus.value = 'saved';
    return;
  }
  if (format.value === 'plain' && !snapshot.trim()) {
    contentStatus.value = 'error';
    contentError.value = '正文不能为空；如需移除请使用撤回或删除';
    return;
  }
  contentStatus.value = 'saving';
  const target = { channelId: props.channelId, messageId: messageId.value, expectedVersion: baseVersion.value, clientOpId: newClientOpId('edit') };
  const run = async () => {
    try {
      const msg = await updateMessageContent(target, snapshot);
      lastSent = snapshot;
      const version = typeof msg?.version === 'number' ? msg.version : baseVersion.value + 1;
      baseVersion.value = version;
      serverContent.value = msg?.content ?? snapshot;
      contentError.value = '';
      contentStatus.value = text.value === lastSent ? 'saved' : 'dirty';
      if (msg) emit('saved', msg, version);
    } catch (error) {
      if (isLogError(error, 'VERSION_CONFLICT')) {
        contentStatus.value = 'conflict';
        await loadServerVersion();
      } else {
        contentStatus.value = 'error';
        contentError.value = describeLogError(error, '保存失败');
      }
    }
  };
  inFlight = run().finally(() => {
    inFlight = null;
  });
  await inFlight;
  // 重新读取：等待期间状态已被异步回调改变。
  const after: string = contentStatus.value;
  if (after === 'dirty' && !composing.value) scheduleSave();
};

const loadServerVersion = async () => {
  try {
    const ctx = await fetchLogMessageContext(props.projectId, props.sectionId, messageId.value);
    const latest = ctx.items.find((it) => it.message.id === messageId.value);
    if (!latest) {
      conflict.value = { content: '', version: baseVersion.value, plain: '（消息已不在当前范围内，可能被删除、撤回或移出筛选）' };
      return;
    }
    conflict.value = { content: latest.message.content, version: latest.version, plain: renderMessageBody(latest.message.content).plainText };
  } catch (error) {
    conflict.value = { content: '', version: baseVersion.value, plain: `无法读取最新内容：${describeLogError(error)}` };
  }
};

const acceptServer = () => {
  const c = conflict.value;
  if (!c || !c.content) return;
  text.value = c.content;
  lastSent = c.content;
  serverContent.value = c.content;
  format.value = detectContentFormat(c.content);
  baseVersion.value = c.version;
  conflict.value = null;
  contentStatus.value = 'saved';
};

/** 用户明确选择以本地内容覆盖：基于最新版本重新提交一次。 */
const overwriteWithLocal = () => {
  const c = conflict.value;
  if (!c || !c.content) return;
  baseVersion.value = c.version;
  lastSent = c.content;
  conflict.value = null;
  contentStatus.value = 'dirty';
  void saveContent();
};

const contentStatusText = computed(() => ({
  idle: '',
  dirty: composing.value ? '输入中…' : '未保存',
  saving: '保存中…',
  saved: '已保存',
  error: contentError.value || '保存失败',
  conflict: '冲突：消息已在别处被修改',
}[contentStatus.value]));

// ---- 角色/差分 ----
const identities = ref<LogIdentityOption[]>([]);
const variants = ref<LogIdentityVariantOption[]>([]);
const identityId = ref('');
const variantId = ref('');
const identityLoading = ref(false);
const identityError = ref('');
const identityBusy = ref(false);
const identitySearch = ref('');
const variantSearch = ref('');
const identityListRef = ref<HTMLElement | null>(null);
const variantListRef = ref<HTMLElement | null>(null);
const selectedIdentityOwnerUserId = ref('');
const selectedIdentityOwner = ref<ChannelIdentityManageCandidate | null>(null);
const identityOwners = ref<ChannelIdentityManageCandidate[]>([]);
const identityOwnerTotal = ref(0);
const identityOwnerAvailable = ref(false);
const identityOwnerLoading = ref(false);
const identityOwnerSearch = ref('');
const identityOwnerPopover = ref(false);
let identityRequestSeq = 0;
let ownerRequestSeq = 0;
let ownerSearchTimer: ReturnType<typeof setTimeout> | null = null;

const originalOwnerUserId = computed(() => props.item?.message.user?.id || '');
const ownerChanged = computed(() => selectedIdentityOwnerUserId.value !== originalOwnerUserId.value);
const canSwitchOwner = computed(() => !!originalOwnerUserId.value && !props.item?.message.isWhisper &&
  !props.item?.message.user?.isBot && !props.item?.message.user?.is_bot);
const showOwnerSwitcher = computed(() => !!originalOwnerUserId.value);
const ownerSwitcherEnabled = computed(() => canSwitchOwner.value && !!caps.value?.changeIdentity &&
  identityOwnerAvailable.value && !identityOwnerLoading.value);
const ownerSwitcherDisabledReason = computed(() => {
  if (identityOwnerLoading.value) return '正在检查可代管的角色所属用户';
  if (props.item?.message.isWhisper) return '悄悄话暂不支持切换角色所属用户';
  if (props.item?.message.user?.isBot || props.item?.message.user?.is_bot) return 'BOT 消息暂不支持切换角色所属用户';
  if (!caps.value?.changeIdentity) return '当前消息没有角色编辑权限';
  return '当前不可切换到其他用户。请确认已开启“允许管理其他用户频道角色”，且存在你有权代管的用户';
});
const selectedOwner = computed(() => identityOwners.value.find((owner) => owner.userId === selectedIdentityOwnerUserId.value) ||
  (selectedIdentityOwner.value?.userId === selectedIdentityOwnerUserId.value ? selectedIdentityOwner.value : undefined));
const selectedOwnerName = computed(() => selectedOwner.value?.nickname || selectedOwner.value?.username ||
  props.item?.message.user?.nick || props.item?.message.user?.name || props.item?.message.user?.username || originalOwnerUserId.value);
const filteredOwners = computed(() => {
  const query = normalizedSearch(identityOwnerSearch.value);
  return identityOwners.value.filter((owner) => !query ||
    `${owner.nickname} ${owner.username} ${owner.userId}`.toLocaleLowerCase().includes(query));
});

const loadIdentityOwners = async (keyword = '') => {
  const sequence = ++ownerRequestSeq;
  if (!canSwitchOwner.value || !caps.value?.changeIdentity) {
    identityOwners.value = [];
    identityOwnerTotal.value = 0;
    identityOwnerAvailable.value = false;
    identityOwnerLoading.value = false;
    return;
  }
  const channelId = props.channelId;
  const targetMessageId = messageId.value;
  const normalizedKeyword = keyword.trim();
  identityOwnerLoading.value = true;
  try {
    const result = await fetchIdentityManageCandidatesForLog(channelId, normalizedKeyword);
    if (sequence !== ownerRequestSeq || channelId !== props.channelId || targetMessageId !== props.item?.message.id) return;
    identityOwners.value = result.items;
    const selected = result.items.find((owner) => owner.userId === selectedIdentityOwnerUserId.value);
    if (selected) selectedIdentityOwner.value = selected;
    if (!normalizedKeyword) {
      identityOwnerTotal.value = result.total;
      identityOwnerAvailable.value = result.items.some((owner) => owner.userId !== originalOwnerUserId.value);
    }
  } catch {
    if (sequence !== ownerRequestSeq) return;
    if (!normalizedKeyword) {
      identityOwners.value = [];
      identityOwnerTotal.value = 0;
      identityOwnerAvailable.value = false;
    }
    // 代管入口是增强功能，读取失败不影响原作者角色切换。
  } finally {
    if (sequence === ownerRequestSeq) identityOwnerLoading.value = false;
  }
};

const normalizedSearch = (value: string) => value.trim().toLocaleLowerCase();
const scrollOptionIntoView = async (containerRef: typeof identityListRef, id: string) => {
  await nextTick();
  const container = containerRef.value;
  if (!container) return;
  const option = Array.from(container.querySelectorAll<HTMLElement>('[data-option-id]'))
    .find((el) => el.dataset.optionId === id);
  option?.scrollIntoView({ block: 'nearest' });
};

const scrollIdentityIntoView = () => scrollOptionIntoView(identityListRef, identityId.value);
const scrollVariantIntoView = () => scrollOptionIntoView(variantListRef, variantId.value || '__default__');

const loadIdentities = async () => {
  const sequence = ++identityRequestSeq;
  const item = props.item;
  const channelId = props.channelId;
  const ownerUserId = selectedIdentityOwnerUserId.value;
  const isCurrent = () => sequence === identityRequestSeq && channelId === props.channelId &&
    item?.message.id === props.item?.message.id && ownerUserId === selectedIdentityOwnerUserId.value;
  identities.value = [];
  variants.value = [];
  identityLoading.value = false;
  if (!item || !item.capabilities.changeIdentity || !ownerUserId) return;
  identityLoading.value = true;
  identityError.value = '';
  try {
    const res = await fetchChannelIdentitiesForLog(channelId, ownerUserId);
    if (!isCurrent()) return;
    identities.value = res.identities;
    variants.value = res.variants;
    void scrollIdentityIntoView();
    void scrollVariantIntoView();
  } catch (error) {
    if (!isCurrent()) return;
    identityError.value = describeLogError(error, '读取角色失败');
  } finally {
    if (isCurrent()) identityLoading.value = false;
  }
};

const selectIdentityOwner = (userId: string) => {
  if (identityBusy.value || !canSwitchOwner.value || !identityOwners.value.some((owner) => owner.userId === userId)) return;
  identityOwnerPopover.value = false;
  if (selectedIdentityOwnerUserId.value === userId) return;
  selectedIdentityOwnerUserId.value = userId;
  selectedIdentityOwner.value = identityOwners.value.find((owner) => owner.userId === userId) || null;
  identityId.value = userId === originalOwnerUserId.value ? props.item?.message.identity?.id || '' : '';
  variantId.value = userId === originalOwnerUserId.value ? props.item?.message.identity?.variantId || '' : '';
  identitySearch.value = '';
  variantSearch.value = '';
  identityOwnerSearch.value = '';
  void loadIdentities();
};

const selectedIdentity = computed(() => identities.value.find((item) => item.id === identityId.value) || null);
const currentVariants = computed(() => variants.value.filter((item) => item.identityId === identityId.value));
const filteredIdentities = computed(() => {
  const query = normalizedSearch(identitySearch.value);
  if (!query) return identities.value;
  return identities.value.filter((item) =>
    `${item.displayName || ''} ${item.id}`.toLocaleLowerCase().includes(query));
});
const filteredVariants = computed(() => {
  const query = normalizedSearch(variantSearch.value);
  if (!query) return currentVariants.value;
  return currentVariants.value.filter((item) =>
    `${item.label} ${item.displayName || ''} ${item.id}`.toLocaleLowerCase().includes(query));
});
const showIdentitySearch = computed(() => identities.value.length > IDENTITY_SEARCH_THRESHOLD);
const showVariantSearch = computed(() => currentVariants.value.length > IDENTITY_SEARCH_THRESHOLD);

const identityDirty = computed(() => {
  const cur = props.item?.message.identity;
  return ownerChanged.value || identityId.value !== (cur?.id || '') || variantId.value !== (cur?.variantId || '');
});

const selectIdentity = (id: string) => {
  if (identityId.value === id) {
    void scrollIdentityIntoView();
    return;
  }
  identityId.value = id;
  variantId.value = '';
  variantSearch.value = '';
  void scrollIdentityIntoView();
  void scrollVariantIntoView();
};

const selectVariant = (id: string) => {
  variantId.value = id;
  void scrollVariantIntoView();
};

watch([() => props.show, () => props.tab], ([show, tab]) => {
  if (!show || tab !== 'identity') return;
  void scrollIdentityIntoView();
  void scrollVariantIntoView();
});

watch(identitySearch, (query) => {
  if (!query) void scrollIdentityIntoView();
});
watch(variantSearch, (query) => {
  if (!query) void scrollVariantIntoView();
});
watch(identityOwnerSearch, (query) => {
  if (!identityOwnerPopover.value) return;
  if (ownerSearchTimer) clearTimeout(ownerSearchTimer);
  ownerSearchTimer = setTimeout(() => {
    ownerSearchTimer = null;
    void loadIdentityOwners(query);
  }, IDENTITY_OWNER_SEARCH_DELAY);
});

const applyIdentity = async () => {
  const item = props.item;
  if (!item || !selectedIdentity.value || !identityDirty.value || identityLoading.value || identityBusy.value) return;
  if (contentStatus.value === 'dirty' || contentStatus.value === 'saving' || contentStatus.value === 'conflict') {
    toast.warning('请先保存或处理正文修改，再切换角色');
    return;
  }
  identityBusy.value = true;
  const reassigning = ownerChanged.value;
  try {
    const msg = await updateMessageIdentity(
      { channelId: props.channelId, messageId: messageId.value, expectedVersion: baseVersion.value },
      serverContent.value,
      identityId.value,
      variantId.value || null,
      reassigning ? selectedIdentityOwnerUserId.value : undefined,
    );
    if (msg) {
      const version = typeof msg.version === 'number' ? msg.version : baseVersion.value + 1;
      baseVersion.value = version;
      emit('saved', msg, version);
    }
    toast.success(reassigning ? '已重新归属本条消息的发送用户与角色' : '已切换角色');
  } catch (error) {
    toast.error(describeLogError(error, '切换失败'));
  } finally {
    identityBusy.value = false;
  }
};

// ---- 本条外观快照 ----
const appearance = ref({ editName: false, name: '', editColor: false, color: '', editAvatar: false, avatar: '' });
const appearanceBusy = ref(false);
const avatarUploading = ref(false);
const avatarInput = ref<HTMLInputElement | null>(null);

function resetAppearance(item: LogDesignMessage) {
  const id = item.message.identity;
  appearance.value = {
    editName: false,
    name: id?.displayName || '',
    editColor: false,
    color: id?.color || '',
    editAvatar: false,
    avatar: id?.avatarAttachment || '',
  };
}

const colorValid = computed(() => !appearance.value.editColor || appearance.value.color === '' || HEX.test(appearance.value.color));
const appearanceDirty = computed(() => appearance.value.editName || appearance.value.editColor || appearance.value.editAvatar);

const pickAvatar = () => avatarInput.value?.click();
const onAvatarFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  if (!/^image\/(png|jpeg|webp|gif)$/.test(file.type)) {
    toast.error('只支持 PNG/JPEG/WebP/GIF 图片');
    return;
  }
  avatarUploading.value = true;
  try {
    const { attachmentId } = await uploadImageAttachment(file, { channelId: props.channelId });
    appearance.value.avatar = attachmentId.replace(/^id:/, '');
    appearance.value.editAvatar = true;
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '上传失败');
  } finally {
    avatarUploading.value = false;
  }
};

const applyAppearance = async () => {
  if (!props.item || !appearanceDirty.value || !colorValid.value) return;
  const a = appearance.value;
  const patch: { name?: string; color?: string; avatar_attachment_id?: string } = {};
  if (a.editName) patch.name = a.name.trim();
  if (a.editColor) patch.color = a.color;
  if (a.editAvatar) patch.avatar_attachment_id = a.avatar;
  appearanceBusy.value = true;
  try {
    const msg = await updateMessageAppearance({ channelId: props.channelId, messageId: messageId.value, expectedVersion: baseVersion.value }, patch);
    if (msg) {
      const version = typeof msg.version === 'number' ? msg.version : baseVersion.value + 1;
      baseVersion.value = version;
      emit('saved', msg, version);
    }
    a.editName = false;
    a.editColor = false;
    a.editAvatar = false;
    toast.success('已更新本条外观');
  } catch (error) {
    toast.error(describeLogError(error, '修改失败'));
  } finally {
    appearanceBusy.value = false;
  }
};

watch([() => props.item, () => props.channelId], ([item, channelId], [prev, prevChannelId]) => {
  if (!item) return;
  if (!prev || item.message.id !== messageId.value || channelId !== prevChannelId) {
    if (timer) clearTimeout(timer);
    timer = null;
    resetFrom(item);
    void loadIdentities();
    void loadIdentityOwners();
    return;
  }
  // 其他来源（聊天页、他人）推进了版本：本地无未保存输入时跟随服务端。
  if (item.version > baseVersion.value && !inFlight && (contentStatus.value === 'idle' || contentStatus.value === 'saved')) {
    resetFrom(item);
    void loadIdentities();
    void loadIdentityOwners();
  }
}, { immediate: true });

// ---- 关闭 ----
const close = async () => {
  if (timer) {
    clearTimeout(timer);
    timer = null;
  }
  if (contentStatus.value === 'dirty') await saveContent();
  if (contentStatus.value === 'conflict' || contentStatus.value === 'error') {
    toast.warning('正文尚未保存，本地输入仍保留在编辑器中');
    return;
  }
  emit('update:show', false);
};

const onKeydown = (e: KeyboardEvent) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault();
    if (timer) clearTimeout(timer);
    timer = null;
    void saveContent();
  }
};

onBeforeUnmount(() => {
  identityRequestSeq += 1;
  ownerRequestSeq += 1;
  if (ownerSearchTimer) clearTimeout(ownerSearchTimer);
  ownerSearchTimer = null;
  if (timer) {
    clearTimeout(timer);
    timer = null;
    if (contentStatus.value === 'dirty') void saveContent();
  }
});

const tabModel = computed({
  get: () => props.tab,
  set: (v) => emit('update:tab', v),
});
</script>

<template>
  <n-drawer :show="show" :width="440" placement="right" :mask-closable="true" :close-on-esc="false" @update:show="(v: boolean) => (v ? emit('update:show', true) : close())">
    <n-drawer-content closable :native-scrollbar="false" @keydown="onKeydown">
      <template #header>
        <div class="log-editor__header">
          <span>编辑原消息</span>
          <span class="log-editor__speaker">{{ speaker }}</span>
        </div>
      </template>
      <n-alert type="info" :show-icon="false" class="log-editor__note">
        修改会写回聊天原消息，聊天页与 LOG 同步显示。
      </n-alert>
      <n-tabs v-model:value="tabModel" type="line" animated>
        <n-tab-pane name="content" tab="正文" :disabled="!caps?.edit">
          <n-alert v-if="contentOverridden" type="warning" :show-icon="false" class="log-editor__note">
            本条在当前 LOG 工程中使用校对后的正文，排版与发布显示该正文；这里编辑的是聊天原消息，不会改变工程正文。
            <n-button size="tiny" quaternary type="primary" @click="emit('clear-override')">改回显示原消息</n-button>
          </n-alert>
          <div class="log-editor__format-switch">
            <span class="log-editor__format-label">编辑模式</span>
            <n-button-group size="tiny">
              <n-button :type="format === 'tiptap' ? 'primary' : 'default'" :disabled="contentStatus === 'conflict'" @click="switchContentFormat('tiptap')">富文本</n-button>
              <n-button :type="format === 'plain' ? 'primary' : 'default'" :disabled="contentStatus === 'conflict'" @click="switchContentFormat('plain')">普通文本</n-button>
            </n-button-group>
          </div>
          <div class="log-editor__content" @compositionstart="onCompositionStart" @compositionend="onCompositionEnd">
            <RichTextEditor
              v-if="format === 'tiptap'"
              :model-value="text"
              :maxlength="50000"
              image-upload-mode="none"
              min-height="160px"
              @update:model-value="onTextChange"
            />
            <n-input
              v-else
              type="textarea"
              :value="text"
              :autosize="{ minRows: 6, maxRows: 18 }"
              @update:value="onTextChange"
            />
          </div>
          <div class="log-editor__status" role="status" aria-live="polite">
            <span :class="`is-${contentStatus}`">{{ contentStatusText }}</span>
            <n-button size="small" :disabled="contentStatus !== 'dirty' && contentStatus !== 'error'" @click="saveContent">保存 (Ctrl+S)</n-button>
          </div>
          <div v-if="conflict" class="log-editor__conflict">
            <div class="log-editor__conflict-title">服务器上的最新内容（版本 {{ conflict.version }}）</div>
            <pre class="log-editor__conflict-body">{{ conflict.plain }}</pre>
            <n-space size="small">
              <n-button size="small" :disabled="!conflict.content" @click="acceptServer">采用服务器版本（放弃本地输入）</n-button>
              <n-button size="small" type="warning" :disabled="!conflict.content" @click="overwriteWithLocal">以我的内容覆盖</n-button>
            </n-space>
          </div>
        </n-tab-pane>
        <n-tab-pane name="identity" tab="角色/差分" :disabled="!caps?.changeIdentity">
          <div class="log-editor__field-header">
            <div class="log-editor__field-label">角色</div>
            <div v-if="showOwnerSwitcher" class="log-editor__owner-switcher">
              <n-popover v-if="ownerSwitcherEnabled" v-model:show="identityOwnerPopover" trigger="click" placement="bottom-end" :to="false" :width="260">
                <template #trigger>
                  <n-button size="tiny" round :disabled="identityBusy" class="log-editor__owner-chip" aria-label="角色所属用户">
                    <AvatarVue :src="selectedOwner ? selectedOwner.avatar : item?.message.user?.avatar || ''" :fallback-text="selectedOwnerName" :use-text-fallback="true" :size="18" :border="false" />
                    <span class="log-editor__owner-name">{{ selectedOwnerName }}</span><span aria-hidden="true">▾</span>
                  </n-button>
                </template>
                <div class="log-editor__owner-picker">
                  <n-input v-if="identityOwnerTotal > IDENTITY_SEARCH_THRESHOLD || identityOwnerSearch" v-model:value="identityOwnerSearch" size="small" clearable placeholder="搜索用户昵称 / 用户名" aria-label="搜索角色所属用户" />
                  <n-spin :show="identityOwnerLoading" size="small">
                    <div class="log-editor__owner-list" role="group" aria-label="角色所属用户">
                      <button v-for="owner in filteredOwners" :key="owner.userId" type="button" class="log-editor__owner-option" :class="{ 'is-active': owner.userId === selectedIdentityOwnerUserId }" :aria-pressed="owner.userId === selectedIdentityOwnerUserId" @click="selectIdentityOwner(owner.userId)">
                        <AvatarVue :src="owner.avatar" :fallback-text="owner.nickname || owner.username" :use-text-fallback="true" :size="26" :border="false" />
                        <span class="log-editor__owner-details"><span class="log-editor__owner-name">{{ owner.nickname || owner.username }}</span><small>{{ owner.username }} · {{ owner.roleLabel }}</small></span>
                      </button>
                      <n-empty v-if="!filteredOwners.length && !identityOwnerLoading" description="没有匹配的用户" size="small" />
                    </div>
                  </n-spin>
                </div>
              </n-popover>
              <n-tooltip v-else trigger="hover" placement="bottom-end">
                <template #trigger>
                  <span class="log-editor__owner-chip-disabled">
                    <n-button size="tiny" round disabled class="log-editor__owner-chip" aria-label="角色所属用户不可切换">
                      <AvatarVue :src="selectedOwner ? selectedOwner.avatar : item?.message.user?.avatar || ''" :fallback-text="selectedOwnerName" :use-text-fallback="true" :size="18" :border="false" />
                      <span class="log-editor__owner-name">{{ selectedOwnerName }}</span><span aria-hidden="true">▾</span>
                    </n-button>
                  </span>
                </template>
                {{ ownerSwitcherDisabledReason }}
              </n-tooltip>
            </div>
            <span v-if="showIdentitySearch" class="log-editor__option-count">{{ filteredIdentities.length }}/{{ identities.length }}</span>
          </div>
          <n-spin :show="identityLoading">
            <n-alert v-if="identityError" type="error" :show-icon="false">{{ identityError }}</n-alert>
            <div class="log-editor__identity-section">
              <n-input
                v-if="showIdentitySearch"
                v-model:value="identitySearch"
                size="small"
                clearable
                placeholder="搜索角色名称"
                class="log-editor__search"
              />
              <div v-if="identities.length && filteredIdentities.length" ref="identityListRef" class="log-editor__identity-grid log-editor__option-list">
                <button
                  v-for="identity in filteredIdentities"
                  :key="identity.id"
                  type="button"
                  class="log-editor__identity-card"
                  :class="{ 'is-active': identity.id === identityId }"
                  :aria-pressed="identity.id === identityId"
                  :data-option-id="identity.id"
                  @click="selectIdentity(identity.id)"
                >
                  <AvatarVue :src="identity.avatarAttachmentId" :fallback-text="identity.displayName || identity.id" :use-text-fallback="true" :size="42" :border="false" />
                  <span class="log-editor__identity-name" :title="identity.displayName || identity.id">{{ identity.displayName || identity.id }}</span>
                </button>
              </div>
              <n-empty v-else-if="identities.length" description="没有匹配的角色" size="small" />
              <n-empty v-else description="没有可用角色" size="small" />
            </div>
            <div class="log-editor__identity-section">
              <div class="log-editor__field-header">
                <div class="log-editor__field-label">差分</div>
                <span v-if="showVariantSearch" class="log-editor__option-count">{{ filteredVariants.length }}/{{ currentVariants.length }}</span>
              </div>
              <n-input
                v-if="identityId && showVariantSearch"
                v-model:value="variantSearch"
                size="small"
                clearable
                placeholder="搜索差分"
                class="log-editor__search"
              />
              <div v-if="identityId" ref="variantListRef" class="log-editor__variant-grid log-editor__option-list log-editor__option-list--variants">
                <button
                  type="button"
                  class="log-editor__variant-card"
                  :class="{ 'is-active': variantId === '' }"
                  :aria-pressed="variantId === ''"
                  data-option-id="__default__"
                  @click="selectVariant('')"
                >
                  <AvatarVue :src="selectedIdentity?.avatarAttachmentId || ''" :fallback-text="selectedIdentity?.displayName || '默认'" :use-text-fallback="true" :size="38" :border="false" />
                  <span class="log-editor__identity-name">默认外观</span>
                </button>
                <button
                  v-for="variant in filteredVariants"
                  :key="variant.id"
                  type="button"
                  class="log-editor__variant-card"
                  :class="{ 'is-active': variant.id === variantId }"
                  :aria-pressed="variant.id === variantId"
                  :data-option-id="variant.id"
                  @click="selectVariant(variant.id)"
                >
                  <AvatarVue :src="variant.avatarAttachmentId || selectedIdentity?.avatarAttachmentId || ''" :fallback-text="variant.displayName || variant.label" :use-text-fallback="true" :size="38" :border="false" />
                  <span class="log-editor__identity-name" :title="variant.label">{{ variant.label }}</span>
                </button>
                <div v-if="variantSearch && !filteredVariants.length" class="log-editor__no-match">没有匹配的差分</div>
              </div>
              <n-empty v-else description="请先选择角色" size="small" />
            </div>
            <p class="log-editor__hint">{{ ownerChanged ? '应用后将本条消息的发送用户与角色重新归属到所选用户。请先选择该用户的角色；正文与 IC/OOC 保持不变。' : '同一用户内切换只改变此消息引用的角色与差分，正文与 IC/OOC、悄悄话设置保持不变。' }}</p>
            <p v-if="item?.message.isWhisper" class="log-editor__hint">悄悄话仅支持原发送用户的角色，不支持跨用户重新归属。</p>
            <n-button type="primary" size="small" :loading="identityBusy" :disabled="!identityDirty || !selectedIdentity || identityLoading" @click="applyIdentity">应用</n-button>
          </n-spin>
        </n-tab-pane>
        <n-tab-pane name="appearance" tab="本条外观" :disabled="!caps?.editAppearance">
          <p class="log-editor__hint">只修改这条消息的历史外观快照，不修改角色资料；未勾选的项目保持不变，留空表示清除覆盖。</p>
          <n-form label-placement="top" size="small">
            <n-form-item>
              <template #label><n-checkbox v-model:checked="appearance.editName">显示名</n-checkbox></template>
              <n-input v-model:value="appearance.name" :disabled="!appearance.editName" maxlength="64" />
            </n-form-item>
            <n-form-item :validation-status="colorValid ? undefined : 'error'" :feedback="colorValid ? '' : '颜色须为 #RRGGBB'">
              <template #label><n-checkbox v-model:checked="appearance.editColor">名字颜色</n-checkbox></template>
              <n-space size="small" align="center">
                <n-color-picker v-model:value="appearance.color" :modes="['hex']" :show-alpha="false" :disabled="!appearance.editColor" style="width: 120px" />
                <n-button size="tiny" quaternary :disabled="!appearance.editColor" @click="appearance.color = ''">清除</n-button>
              </n-space>
            </n-form-item>
            <n-form-item>
              <template #label><n-checkbox v-model:checked="appearance.editAvatar">头像</n-checkbox></template>
              <n-space size="small" align="center">
                <n-button size="small" :loading="avatarUploading" @click="pickAvatar">上传图片</n-button>
                <n-button size="small" quaternary @click="appearance.avatar = ''; appearance.editAvatar = true">清除头像覆盖</n-button>
                <span class="log-editor__hint">{{ appearance.editAvatar ? (appearance.avatar ? '将使用新上传的附件' : '将清除头像覆盖') : '' }}</span>
              </n-space>
              <input ref="avatarInput" type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden @change="onAvatarFile">
            </n-form-item>
          </n-form>
          <n-button type="primary" size="small" :loading="appearanceBusy" :disabled="!appearanceDirty || !colorValid" @click="applyAppearance">应用到本条</n-button>
        </n-tab-pane>
      </n-tabs>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.log-editor__header {
  display: flex;
  gap: 8px;
  align-items: baseline;
}

.log-editor__speaker {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.log-editor__note {
  margin-bottom: 8px;
}

.log-editor__format-switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.log-editor__format-label,
.log-editor__field-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--sc-text-secondary, #666);
}

.log-editor__field-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.log-editor__option-count {
  font-size: 11px;
  color: var(--sc-text-secondary, #777);
  font-variant-numeric: tabular-nums;
}

.log-editor__search {
  margin-top: 6px;
}

.log-editor__owner-switcher {
  margin-left: auto;
  min-width: 0;
}

.log-editor__owner-chip-disabled {
  display: inline-flex;
  max-width: 100%;
  cursor: help;
}

.log-editor__owner-chip {
  max-width: 210px;
}

.log-editor__owner-chip :deep(.n-button__content) {
  gap: 5px;
  min-width: 0;
}

.log-editor__owner-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.log-editor__owner-picker {
  max-width: calc(100vw - 56px);
}

.log-editor__owner-list {
  max-height: 240px;
  overflow-y: auto;
  overscroll-behavior: contain;
  margin-top: 4px;
}

.log-editor__owner-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px;
  width: 100%;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.log-editor__owner-option:hover,
.log-editor__owner-option.is-active {
  background: rgba(32, 128, 240, 0.1);
}

.log-editor__owner-details {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.log-editor__owner-details small {
  color: var(--sc-text-secondary, #777);
}

.log-editor__identity-section + .log-editor__identity-section {
  margin-top: 14px;
}

.log-editor__identity-grid,
.log-editor__variant-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 6px;
}

.log-editor__option-list {
  max-height: 260px;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  padding-right: 3px;
}

.log-editor__option-list--variants {
  max-height: 220px;
}

.log-editor__no-match {
  grid-column: 1 / -1;
  padding: 8px 4px;
  font-size: 12px;
  color: var(--sc-text-secondary, #777);
  text-align: center;
}

.log-editor__identity-card,
.log-editor__variant-card {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px;
  border: 1px solid var(--sc-border-color, rgba(128, 128, 128, 0.26));
  border-radius: 8px;
  background: var(--sc-surface-color, rgba(128, 128, 128, 0.06));
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.log-editor__variant-card {
  padding: 7px;
}

.log-editor__identity-card:hover,
.log-editor__variant-card:hover {
  border-color: var(--n-color-target, #2080f0);
}

.log-editor__identity-card.is-active,
.log-editor__variant-card.is-active {
  border-color: #2080f0;
  background: rgba(32, 128, 240, 0.1);
}

.log-editor__identity-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.log-editor__status {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
  font-size: 12px;
}

.log-editor__status .is-error,
.log-editor__status .is-conflict {
  color: #d03050;
}

.log-editor__conflict {
  margin-top: 12px;
  padding: 10px;
  border: 1px solid #f0a020;
  border-radius: 6px;
}

.log-editor__conflict-title {
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 6px;
}

.log-editor__conflict-body {
  max-height: 200px;
  overflow: auto;
  white-space: pre-wrap;
  font-family: inherit;
  font-size: 13px;
  margin: 0 0 8px;
}

.log-editor__hint {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  margin: 4px 0 8px;
}

@media (max-width: 520px) {
  .log-editor__identity-grid,
  .log-editor__variant-grid {
    grid-template-columns: 1fr;
  }
}
</style>
