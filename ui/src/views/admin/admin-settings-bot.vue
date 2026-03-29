<script setup lang="tsx">
import AvatarEditor from '@/components/AvatarEditor.vue';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { urlBase } from '@/stores/_config';
import { useChatStore, chatEvent } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import type { BotOneBotConfig } from '@/types';
import AdminBotActiveReferencePopover from './components/AdminBotActiveReferencePopover.vue';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { Refresh, Search, Trash } from '@vicons/tabler';
import type { DataTableColumns } from 'naive-ui';
import { NIcon, useDialog, useMessage } from 'naive-ui';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

interface BotListItem {
  id: string
  name: string
  token: string
  avatar?: string
  nickColor?: string
  expiresAt?: number
  updatedAt?: string | number
  createdAt?: string | number
  botKind?: string
  isSystemManaged?: boolean
  activeReferenceCount?: number
  activeReferences?: Array<{
    kind?: string
    integrationId?: string
    name?: string
    source?: string
    scopeType?: string
    worldId?: string
    worldName?: string
    channelId?: string
    channelName?: string
  }>
  userNickname?: string
  oneBotSelfId?: number | string
  onebotConfig?: BotOneBotConfig | null
}

const emit = defineEmits(['close']);
const { t } = useI18n();

const utils = useUtilsStore();
const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();

const cancel = () => emit('close');

const BOT_CONFIG_MODAL_Z_INDEX = 3200;
const BOT_AVATAR_MODAL_Z_INDEX = 3210;

const showModal = ref(false);
const editingToken = ref<BotListItem | null>(null);
const newTokenName = ref('bot');
const newTokenAvatar = ref('');
const newTokenColor = ref('#2563eb');
const normalizeOneBotTransportType = (value?: string): BotOneBotConfig['transportType'] => {
  switch (String(value || '').trim()) {
  case 'reverse_ws':
    return 'reverse_ws';
  case 'http':
    return 'http';
  default:
    return 'forward_ws';
  }
};
const normalizeOneBotPathSuffix = (value: string | undefined, defaultValue: string) => {
  let next = String(value || '').trim();
  if (!next) {
    next = defaultValue;
  }
  if (!next.startsWith('/')) {
    next = `/${next}`;
  }
  next = next.replace(/\/+$/, '');
  return next || defaultValue;
};
const normalizeOneBotHTTPPostAddress = (value?: string) => {
  let next = String(value || '').trim();
  if (!next) {
    return '';
  }
  next = next.replace(/\/+$/, '');
  return next;
};
const isAbsoluteHTTPURL = (value?: string) => /^https?:\/\//i.test(String(value || '').trim());
const defaultOneBotConfig = (): BotOneBotConfig => ({
  enabled: false,
  transportType: 'forward_ws',
  httpPathSuffix: '/onebot/v11/http',
  httpPostPathSuffix: '',
  url: '',
  apiUrl: '',
  eventUrl: '',
  useUniversalClient: true,
  reconnectIntervalMs: 3000,
});
const onebotConfig = ref<BotOneBotConfig>(defaultOneBotConfig());
const avatarFileInputRef = ref<HTMLInputElement | null>(null);
const avatarEditorVisible = ref(false);
const avatarEditorFile = ref<File | null>(null);
const avatarPreview = ref('');
let avatarPreviewObjectUrl: string | null = null;
const uploadingAvatar = ref(false);
const avatarVersion = ref(0);

const activeTab = ref<'manual' | 'system'>('manual');
const showSystemBots = ref(false);
const keyword = ref('');
const loading = ref(false);
const rows = ref<BotListItem[]>([]);
const total = ref(0);
const checkedRowKeys = ref<string[]>([]);
const page = ref(1);
const pageSize = ref(20);
let searchTimer: ReturnType<typeof setTimeout> | null = null;

const appendAvatarVersion = (url: string, version?: number | string) => {
  if (!url || !version) {
    return url;
  }
  const mark = url.includes('?') ? '&' : '?';
  return `${url}${mark}v=${encodeURIComponent(String(version))}`;
};

const resolveBotAvatarValue = (token?: Partial<BotListItem> | null) => {
  if (!token) return '';
  return token.avatar || '';
};

const botAvatarDisplay = computed(() => {
  const base = avatarPreview.value || resolveAttachmentUrl(newTokenAvatar.value);
  return appendAvatarVersion(base, avatarPreview.value ? undefined : avatarVersion.value);
});

const currentScope = computed<'manual' | 'system'>(() => activeTab.value === 'system' ? 'system' : 'manual');
const onebotConfigVisible = computed(() => !editingToken.value?.isSystemManaged);
const onebotSectionVisible = computed(() => onebotConfigVisible.value && Boolean(onebotConfig.value.enabled));
const onebotTransportType = computed(() => normalizeOneBotTransportType(onebotConfig.value.transportType));
const onebotForwardWSUniversalURL = computed(() => {
  const protocol = typeof window !== 'undefined' && window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}${urlBase}/onebot/v11/ws`;
});
const onebotForwardWSAPIURL = computed(() => `${onebotForwardWSUniversalURL.value}/api`);
const onebotForwardWSEventURL = computed(() => `${onebotForwardWSUniversalURL.value}/event`);
const onebotHTTPBaseURL = computed(() => {
  const protocol = typeof window !== 'undefined' ? window.location.protocol : 'http:';
  return `${protocol}${urlBase}${normalizeOneBotPathSuffix(onebotConfig.value.httpPathSuffix, '/onebot/v11/http')}/:action`;
});
const onebotHTTPSendGroupExampleURL = computed(() => {
  const protocol = typeof window !== 'undefined' ? window.location.protocol : 'http:';
  return `${protocol}${urlBase}${normalizeOneBotPathSuffix(onebotConfig.value.httpPathSuffix, '/onebot/v11/http')}/send_group_msg`;
});
const onebotHTTPPostExampleURL = computed(() => {
  const value = normalizeOneBotHTTPPostAddress(onebotConfig.value.httpPostPathSuffix);
  if (!value) {
    return '未填写';
  }
  if (isAbsoluteHTTPURL(value)) {
    return value;
  }
  if (value.startsWith('/')) {
    const protocol = typeof window !== 'undefined' ? window.location.protocol : 'http:';
    return `${protocol}${urlBase}${value}`;
  }
  return value;
});
const hasRows = computed(() => rows.value.length > 0);
const systemTabVisible = computed(() => showSystemBots.value);
const pageCount = computed(() => Math.max(1, Math.ceil(Math.max(total.value, 1) / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return rows.value.slice(start, start + pageSize.value);
});
const selectedRows = computed(() => {
  const keySet = new Set(checkedRowKeys.value);
  return rows.value.filter((item) => keySet.has(item.id));
});
const selectedProtectedCount = computed(() => selectedRows.value.filter((item) => isDeleteBlocked(item)).length);
const batchDeleteDisabled = computed(() => selectedRows.value.length === 0 || selectedProtectedCount.value > 0);
const currentStatsText = computed(() => {
  if (currentScope.value === 'system') {
    return `系统 BOT ${total.value} 个`;
  }
  return `标准 BOT ${total.value} 个`;
});

const kindLabel = (row: BotListItem) => {
  switch ((row.botKind || '').trim()) {
  case 'channel_webhook':
    return '频道 Webhook';
  case 'digest_pull':
    return '摘要拉取';
  case 'manual':
    return '标准 BOT';
  default:
    return row.isSystemManaged ? '系统 BOT' : '标准 BOT';
  }
};

const clearAvatarPreview = () => {
  if (avatarPreviewObjectUrl) {
    URL.revokeObjectURL(avatarPreviewObjectUrl);
    avatarPreviewObjectUrl = null;
  }
  avatarPreview.value = '';
};

const setAvatarPreview = (file: File) => {
  clearAvatarPreview();
  avatarPreviewObjectUrl = URL.createObjectURL(file);
  avatarPreview.value = avatarPreviewObjectUrl;
};

const resetForm = () => {
  newTokenName.value = 'bot';
  newTokenAvatar.value = '';
  newTokenColor.value = '#2563eb';
  onebotConfig.value = defaultOneBotConfig();
  clearAvatarPreview();
};

const resetSelection = () => {
  checkedRowKeys.value = [];
};

const normalizeRows = (items: any[]) => {
  return (items || []).map((item) => ({
    ...item,
    avatar: item.avatar || item.avatarAttachmentId || item.avatar_id || item.avatarId || item.avatar_attachment_id || '',
    isSystemManaged: Boolean(item.isSystemManaged),
    activeReferenceCount: Number(item.activeReferenceCount || 0) || 0,
    activeReferences: Array.isArray(item.activeReferences) ? item.activeReferences : [],
    onebotConfig: item.onebotConfig ? {
      ...defaultOneBotConfig(),
      ...item.onebotConfig,
      transportType: normalizeOneBotTransportType(item.onebotConfig?.transportType),
      httpPathSuffix: normalizeOneBotPathSuffix(item.onebotConfig?.httpPathSuffix, '/onebot/v11/http'),
      httpPostPathSuffix: normalizeOneBotHTTPPostAddress(item.onebotConfig?.httpPostPathSuffix),
      reconnectIntervalMs: Number(item.onebotConfig?.reconnectIntervalMs || 3000) || 3000,
    } : null,
  })) as BotListItem[];
};

const refresh = async () => {
  loading.value = true;
  try {
    const resp = await utils.botTokenList({
      keyword: keyword.value.trim(),
      scope: currentScope.value,
    });
    rows.value = normalizeRows(resp.data?.items || []);
    total.value = Number(resp.data?.total || 0) || rows.value.length;
    if (page.value > pageCount.value) {
      page.value = pageCount.value;
    }
    resetSelection();
  } catch (error: any) {
    message.error(`BOT 列表加载失败: ${error?.response?.data?.message || error?.message || '未知错误'}`);
  } finally {
    loading.value = false;
  }
};

const queueRefresh = () => {
  if (searchTimer) {
    clearTimeout(searchTimer);
  }
  searchTimer = setTimeout(() => {
    page.value = 1;
    void refresh();
  }, 250);
};

const isDeleteBlocked = (row: BotListItem) => Boolean(row.isSystemManaged && (row.activeReferenceCount || 0) > 0);

const formatExpireAt = (value?: number) => {
  if (!value || value <= 0) {
    return '已失效';
  }
  try {
    return new Date(value).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  } catch {
    return String(value);
  }
};

const formatToken = (token?: string) => {
  const value = String(token || '').trim();
  if (!value) return '-';
  if (value.length <= 12) return value;
  return `${value.slice(0, 6)}...${value.slice(-4)}`;
};

const syncBotListSideEffects = () => {
  chat.invalidateBotListCache();
  chatEvent.emit('bot-list-updated');
};

const openCreateModal = () => {
  editingToken.value = null;
  resetForm();
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  showModal.value = true;
};

const openEditModal = (token: BotListItem) => {
  editingToken.value = token;
  newTokenName.value = token.name || 'bot';
  newTokenAvatar.value = resolveBotAvatarValue(token);
  newTokenColor.value = token.nickColor || '#2563eb';
  onebotConfig.value = token.onebotConfig ? {
    ...defaultOneBotConfig(),
    ...token.onebotConfig,
    transportType: normalizeOneBotTransportType(token.onebotConfig?.transportType),
    httpPathSuffix: normalizeOneBotPathSuffix(token.onebotConfig?.httpPathSuffix, '/onebot/v11/http'),
    httpPostPathSuffix: normalizeOneBotHTTPPostAddress(token.onebotConfig?.httpPostPathSuffix),
  } : defaultOneBotConfig();
  clearAvatarPreview();
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  showModal.value = true;
};

const emitUpdatedChannelIdentities = (items?: any[]) => {
  if (!Array.isArray(items) || items.length === 0) {
    return;
  }
  items.forEach((item) => {
    const identity = {
      id: String(item?.id || '').trim(),
      channelId: String(item?.channelId || item?.channel_id || '').trim(),
      userId: String(item?.userId || item?.user_id || '').trim(),
      displayName: String(item?.displayName || item?.display_name || '').trim(),
      color: String(item?.color || '').trim(),
      avatarAttachmentId: String(item?.avatarAttachmentId || item?.avatar_attachment_id || '').trim(),
      isDefault: Boolean(item?.isDefault ?? item?.is_default),
      sortOrder: Number(item?.sortOrder ?? item?.sort_order ?? 0) || 0,
    };
    if (!identity.id || !identity.channelId) {
      return;
    }
    chatEvent.emit('channel-identity-updated' as any, { identity, channelId: identity.channelId } as any);
  });
};

const submitToken = async () => {
  const payloadOneBotConfig = onebotConfigVisible.value ? {
    enabled: Boolean(onebotConfig.value.enabled),
    transportType: onebotTransportType.value,
    httpPathSuffix: normalizeOneBotPathSuffix(onebotConfig.value.httpPathSuffix, '/onebot/v11/http'),
    httpPostPathSuffix: normalizeOneBotHTTPPostAddress(onebotConfig.value.httpPostPathSuffix),
    url: onebotConfig.value.url?.trim() || '',
    apiUrl: onebotConfig.value.apiUrl?.trim() || '',
    eventUrl: onebotConfig.value.eventUrl?.trim() || '',
    useUniversalClient: Boolean(onebotConfig.value.useUniversalClient),
    reconnectIntervalMs: Number(onebotConfig.value.reconnectIntervalMs || 3000) || 3000,
  } : undefined;
  const payload = {
    name: newTokenName.value.trim() || 'bot',
    avatar: newTokenAvatar.value.trim(),
    nickColor: newTokenColor.value,
    onebotConfig: payloadOneBotConfig,
  };
  try {
    let resp: any;
    if (editingToken.value) {
      resp = await utils.botTokenUpdate({
        id: editingToken.value.id,
        ...payload,
      });
      emitUpdatedChannelIdentities(resp?.data?.updatedIdentities);
      message.success('更新成功');
    } else {
      resp = await utils.botTokenAdd(payload);
      emitUpdatedChannelIdentities(resp?.data?.updatedIdentities);
      message.success('添加成功');
    }
    await refresh();
    syncBotListSideEffects();
    showModal.value = false;
    if (!editingToken.value) {
      resetForm();
    }
  } catch (error: any) {
    message.error((editingToken.value ? '更新失败: ' : '添加失败: ') + (error?.response?.data?.message || '未知错误'));
  }
};

const deleteItem = async (item: BotListItem) => {
  if (isDeleteBlocked(item)) {
    message.warning('该系统 BOT 仍被 active integration 引用，无法直接删除。');
    return;
  }
  dialog.warning({
    title: '删除机器人',
    content: `确定删除 ${item.name || item.id} 吗？此操作不可撤销。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await utils.botTokenDelete(item.id);
        message.success('删除成功');
        await refresh();
        syncBotListSideEffects();
      } catch (error: any) {
        message.error(`删除失败: ${error?.response?.data?.message || '未知错误'}`);
      }
    },
  });
};

const batchDelete = async () => {
  if (batchDeleteDisabled.value) {
    return;
  }
  const count = selectedRows.value.length;
  dialog.warning({
    title: '批量删除机器人',
    content: `确定批量删除已选中的 ${count} 个机器人吗？此操作不可撤销。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const resp = await utils.botTokenBatchDelete(checkedRowKeys.value);
        const deletedCount = Number(resp?.data?.deletedCount || 0) || 0;
        const failedCount = Number(resp?.data?.failedCount || 0) || 0;
        if (deletedCount > 0) {
          message.success(`已删除 ${deletedCount} 个机器人`);
        }
        if (failedCount > 0) {
          const firstFailed = resp?.data?.failedItems?.[0];
          message.warning(`有 ${failedCount} 个机器人删除失败${firstFailed?.message ? `：${firstFailed.message}` : ''}`);
        }
        await refresh();
        syncBotListSideEffects();
      } catch (error: any) {
        message.error(`批量删除失败: ${error?.response?.data?.message || '未知错误'}`);
      }
    },
  });
};

const copyToken = async (value?: string) => {
  const token = String(value || '').trim();
  if (!token) return;
  try {
    await navigator.clipboard.writeText(token);
    message.success('Token 已复制');
  } catch {
    message.warning('复制失败，请手动复制');
  }
};

const resolveAvatar = (value?: string, version?: number | string) => {
  if (!value) {
    return '';
  }
  const resolved = resolveAttachmentUrl(value);
  return appendAvatarVersion(resolved, version);
};

const triggerAvatarUpload = () => {
  avatarFileInputRef.value?.click();
};

const handleAvatarFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input?.files?.[0];
  if (!file) {
    return;
  }
  const sizeLimit = utils.fileSizeLimit;
  if (file.size > sizeLimit) {
    const limitMB = (sizeLimit / 1024 / 1024).toFixed(1);
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`);
    input.value = '';
    return;
  }
  avatarEditorFile.value = file;
  avatarEditorVisible.value = true;
  input.value = '';
};

const handleAvatarEditorSave = async (file: File) => {
  uploadingAvatar.value = true;
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  setAvatarPreview(file);
  try {
    const result = await uploadImageAttachment(file, { channelId: 'bot-avatar', skipCompression: true });
    if (!result.attachmentId) {
      throw new Error('上传失败');
    }
    newTokenAvatar.value = result.attachmentId;
    avatarVersion.value = Date.now();
    message.success('头像上传成功');
  } catch (error: any) {
    message.error(error?.message || '头像上传失败');
  } finally {
    uploadingAvatar.value = false;
  }
};

const handleAvatarEditorCancel = () => {
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
};

const clearBotAvatar = () => {
  newTokenAvatar.value = '';
  clearAvatarPreview();
};

const rowKey = (row: BotListItem) => row.id;

const handleCheckedRowKeysChange = (keys: Array<string | number>) => {
  checkedRowKeys.value = keys.map((item) => String(item));
};

const columns = computed<DataTableColumns<BotListItem>>(() => [
  {
    type: 'selection',
    disabled: (row: BotListItem) => isDeleteBlocked(row),
  },
  {
    title: '机器人',
    key: 'name',
    minWidth: 240,
    render: (row: BotListItem) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', minWidth: 0 }}>
        {resolveBotAvatarValue(row) ? (
          <img
            src={resolveAvatar(resolveBotAvatarValue(row), row.updatedAt)}
            style={{
              width: '36px',
              height: '36px',
              minWidth: '36px',
              minHeight: '36px',
              borderRadius: '8px',
              objectFit: 'cover',
              display: 'block',
              backgroundColor: 'rgba(148, 163, 184, 0.12)',
            }}
          />
        ) : (
          <n-avatar size="medium">{row.name?.slice(0, 1) || 'B'}</n-avatar>
        )}
        <div style={{ minWidth: 0, display: 'flex', flexDirection: 'column', gap: '2px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
            <span style={{ fontWeight: 600, color: 'var(--n-text-color-1)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {row.name || 'Bot'}
            </span>
            <span
              style={{
                width: '0.85rem',
                height: '0.85rem',
                minWidth: '0.85rem',
                borderRadius: '999px',
                border: '1px solid rgba(148, 163, 184, 0.4)',
                display: 'inline-block',
                backgroundColor: row.nickColor || 'transparent',
              }}
            ></span>
          </div>
          <div style={{ fontSize: '12px', color: 'var(--n-text-color-3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {row.userNickname || row.id}
          </div>
          {!row.isSystemManaged && row.oneBotSelfId ? (
            <div style={{ fontSize: '12px', color: 'var(--n-text-color-3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              OneBot 号 {String(row.oneBotSelfId)}
            </div>
          ) : null}
        </div>
      </div>
    ),
  },
  {
    title: '分类',
    key: 'botKind',
    width: 160,
    render: (row: BotListItem) => (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: '6px' }}>
        <n-tag size="small" type={row.isSystemManaged ? 'warning' : 'success'}>
          {kindLabel(row)}
        </n-tag>
        {row.isSystemManaged ? (
          <AdminBotActiveReferencePopover
            count={row.activeReferenceCount || 0}
            references={row.activeReferences || []}
          />
        ) : null}
      </div>
    ),
  },
  {
    title: 'Token',
    key: 'token',
    minWidth: 220,
    render: (row: BotListItem) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
        <code style={{
          fontSize: '12px',
          padding: '2px 6px',
          borderRadius: '6px',
          background: 'rgba(15, 23, 42, 0.06)',
          whiteSpace: 'nowrap',
        }}
        >
          {formatToken(row.token)}
        </code>
        <n-button text size="small" onClick={() => copyToken(row.token)}>复制</n-button>
      </div>
    ),
  },
  {
    title: '到期时间',
    key: 'expiresAt',
    width: 120,
    render: (row: BotListItem) => (
      <span>{formatExpireAt(row.expiresAt)}</span>
    ),
  },
  {
    title: '操作',
    key: 'actions',
    width: 170,
    render: (row: BotListItem) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
        <n-button size="small" onClick={() => openEditModal(row)}>编辑</n-button>
        <n-button size="small" type="error" disabled={isDeleteBlocked(row)} onClick={() => deleteItem(row)}>删除</n-button>
      </div>
    ),
  },
]);

watch(showModal, (visible) => {
  if (visible) {
    return;
  }
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  clearAvatarPreview();
});

watch(newTokenAvatar, (value, oldValue) => {
  if (!value || value === oldValue) {
    return;
  }
  avatarVersion.value = Date.now();
});

watch(showSystemBots, (visible) => {
  if (!visible && activeTab.value === 'system') {
    activeTab.value = 'manual';
  }
});

watch(currentScope, () => {
  page.value = 1;
  void refresh();
});

watch(pageSize, () => {
  page.value = 1;
});

onMounted(async () => {
  await refresh();
});

onUnmounted(() => {
  clearAvatarPreview();
  if (searchTimer) {
    clearTimeout(searchTimer);
  }
});
</script>

<template>
  <div class="bot-management">
    <div class="bot-management__body">
      <div class="bot-management__toolbar">
        <div class="bot-management__controls">
          <n-input
            v-model:value="keyword"
            placeholder="搜索名称、ID、昵称或类型"
            clearable
            style="width: 240px"
            @input="queueRefresh"
            @clear="queueRefresh"
          >
            <template #prefix>
              <n-icon :component="Search" />
            </template>
          </n-input>
          <label class="bot-management__switch">
            <span>显示系统 BOT</span>
            <n-switch v-model:value="showSystemBots" />
          </label>
          <n-button :loading="loading" @click="refresh">
            <template #icon>
              <n-icon :component="Refresh" />
            </template>
            刷新
          </n-button>
          <n-button type="primary" @click="openCreateModal">新增 BOT</n-button>
        </div>
        <div class="bot-management__stats">
          <span class="bot-management__stats-main">{{ currentStatsText }}</span>
          <span class="bot-management__stats-sub">当前选中 {{ checkedRowKeys.length }} 项</span>
        </div>
      </div>

      <div class="bot-management__hint">
        <span>标准 BOT 用于频道骰点与角色绑定。系统 BOT 仅供 webhook / digest 集成使用，默认隐藏。</span>
      </div>

      <div v-if="showSystemBots && activeTab === 'system'" class="bot-management__system-note">
        系统 BOT 存在 active 引用时不可删除，请先撤销对应的 webhook 或 digest integration。
      </div>

      <n-tabs v-model:value="activeTab" type="segment" animated class="bot-management__tabs">
        <n-tab-pane name="manual" tab="标准 BOT" />
        <n-tab-pane v-if="systemTabVisible" name="system" tab="系统 BOT" />
      </n-tabs>

      <div v-if="checkedRowKeys.length > 0" class="bot-management__batch-bar">
        <span>已选中 {{ checkedRowKeys.length }} 项</span>
        <div class="bot-management__batch-actions">
          <n-button type="error" :disabled="batchDeleteDisabled" @click="batchDelete">
            <template #icon>
              <n-icon :component="Trash" />
            </template>
            批量删除
          </n-button>
          <span v-if="selectedProtectedCount > 0" class="bot-management__batch-warning">
            有 {{ selectedProtectedCount }} 项仍被 active integration 引用，无法删除
          </span>
        </div>
      </div>

      <div class="bot-management__table">
        <n-data-table
          :columns="columns"
          :data="pagedRows"
          :loading="loading"
          :pagination="false"
          :bordered="false"
          :max-height="360"
          :row-key="rowKey"
          :checked-row-keys="checkedRowKeys"
          :scroll-x="960"
          size="small"
          @update:checked-row-keys="handleCheckedRowKeysChange"
        />
        <n-empty v-if="!loading && !hasRows" description="当前筛选条件下没有 BOT" class="bot-management__empty" />
      </div>
    </div>

    <div class="bot-management__footer">
      <n-pagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :item-count="total"
        :page-sizes="[10, 20, 50, 100]"
        show-size-picker
        :disabled="loading || total === 0"
      >
        <template #prefix="{ itemCount }">
          共 {{ itemCount }} 条
        </template>
      </n-pagination>
      <n-button @click="cancel">关闭</n-button>
    </div>
  </div>

  <n-modal
    v-model:show="showModal"
    :z-index="BOT_CONFIG_MODAL_Z_INDEX"
    preset="dialog"
    :title="editingToken ? '编辑机器人' : '配置机器人外观'"
    :positive-text="editingToken ? '保存' : t('dialoChannelgNew.positiveText')"
    :negative-text="t('dialoChannelgNew.negativeText')"
    @positive-click="submitToken"
  >
    <n-form label-placement="top">
      <n-form-item label="机器人名称">
        <n-input v-model:value="newTokenName" placeholder="机器人名称" />
      </n-form-item>
      <n-form-item label="机器人头像">
        <input ref="avatarFileInputRef" type="file" accept="image/*" class="hidden" @change="handleAvatarFileChange">
        <div class="bot-avatar-uploader">
          <img
            v-if="botAvatarDisplay"
            :src="botAvatarDisplay"
            class="bot-avatar-uploader__preview"
          />
          <n-avatar v-else size="large">
            {{ newTokenName.slice(0, 1) || 'B' }}
          </n-avatar>
          <div class="bot-avatar-uploader__actions">
            <n-space>
              <n-button size="tiny" :loading="uploadingAvatar" @click="triggerAvatarUpload">上传头像</n-button>
              <n-button size="tiny" quaternary :disabled="!newTokenAvatar" @click="clearBotAvatar">清除</n-button>
            </n-space>
            <n-input v-model:value="newTokenAvatar" size="small" placeholder="也可粘贴图片地址或附件ID" @update:value="clearAvatarPreview" />
            <p class="bot-avatar-uploader__hint">支持本地上传，系统会返回附件ID，以 <code>id:xxxxx</code> 开头。</p>
          </div>
        </div>
      </n-form-item>
      <n-form-item label="昵称色彩">
        <div class="flex items-center space-x-3 w-full">
          <n-color-picker v-model:value="newTokenColor" :modes="['hex']" :show-alpha="false" size="small" />
          <span class="text-xs text-gray-500">用于频道中展示机器人昵称颜色</span>
        </div>
      </n-form-item>
      <template v-if="onebotConfigVisible">
        <div class="bot-management__onebot-title">OneBot v11</div>
        <div class="bot-management__onebot-hint">
          启用后可选择正向 WS、反向 WS 或 HTTP API 的配置，但只支持基础消息收发功能。
        </div>
        <n-form-item v-if="editingToken?.oneBotSelfId" label="BOT 伪号码">
          <n-input :value="String(editingToken.oneBotSelfId)" readonly />
        </n-form-item>
        <n-form-item label="启用 OneBot v11">
          <n-switch v-model:value="onebotConfig.enabled" />
        </n-form-item>
        <template v-if="onebotSectionVisible">
          <n-form-item label="连接方式">
            <n-radio-group v-model:value="onebotConfig.transportType">
              <n-radio-button value="forward_ws">正向 WS</n-radio-button>
              <n-radio-button value="reverse_ws">反向 WS</n-radio-button>
              <n-radio-button value="http">HTTP API</n-radio-button>
            </n-radio-group>
          </n-form-item>

          <template v-if="onebotTransportType === 'forward_ws'">
            <div class="bot-management__onebot-panel">
              <div class="bot-management__onebot-row">
                <div class="bot-management__onebot-label">Universal 接入地址</div>
                <code class="bot-management__onebot-code">{{ onebotForwardWSUniversalURL }}</code>
              </div>
              <div class="bot-management__onebot-row">
                <div class="bot-management__onebot-label">API 接入地址</div>
                <code class="bot-management__onebot-code">{{ onebotForwardWSAPIURL }}</code>
              </div>
              <div class="bot-management__onebot-row">
                <div class="bot-management__onebot-label">Event 接入地址</div>
                <code class="bot-management__onebot-code">{{ onebotForwardWSEventURL }}</code>
              </div>
              <div class="bot-management__onebot-label">鉴权方式：使用当前 BOT Token</div>
            </div>
          </template>

          <template v-else-if="onebotTransportType === 'reverse_ws'">
            <div class="bot-management__onebot-hint">
              SealChat 将主动连接外部 OneBot 服务。未填写 URL 时不会建立反向连接。
            </div>
            <n-form-item label="Universal URL">
              <n-input v-model:value="onebotConfig.url" placeholder="ws://127.0.0.1:8080/onebot/ws" />
            </n-form-item>
            <n-form-item label="使用 Universal 单连接">
              <n-switch v-model:value="onebotConfig.useUniversalClient" />
            </n-form-item>
            <n-form-item v-if="!onebotConfig.useUniversalClient" label="API URL">
              <n-input v-model:value="onebotConfig.apiUrl" placeholder="ws://127.0.0.1:8080/onebot/ws/api" />
            </n-form-item>
            <n-form-item v-if="!onebotConfig.useUniversalClient" label="Event URL">
              <n-input v-model:value="onebotConfig.eventUrl" placeholder="ws://127.0.0.1:8080/onebot/ws/event" />
            </n-form-item>
            <n-form-item label="重连间隔（毫秒）">
              <n-input-number v-model:value="onebotConfig.reconnectIntervalMs" :min="500" :step="500" style="width: 180px" />
            </n-form-item>
          </template>

          <template v-else>
            <div class="bot-management__onebot-panel">
              <n-form-item label="HTTP API 后缀">
                <n-input v-model:value="onebotConfig.httpPathSuffix" placeholder="/onebot/v11/http" />
              </n-form-item>
              <div class="bot-management__onebot-row">
                <div class="bot-management__onebot-label">HTTP API 地址前缀</div>
                <code class="bot-management__onebot-code">{{ onebotHTTPBaseURL }}</code>
              </div>
              <div class="bot-management__onebot-row">
                <div class="bot-management__onebot-label">调用示例</div>
                <code class="bot-management__onebot-code">{{ onebotHTTPSendGroupExampleURL }}</code>
              </div>
              <n-form-item label="HTTP POST 地址（预留）">
                <n-input v-model:value="onebotConfig.httpPostPathSuffix" placeholder="http://127.0.0.1:55001/OlivOSMsgApi/qq/onebot/default" />
              </n-form-item>
              <div class="bot-management__onebot-row">
                <div class="bot-management__onebot-label">HTTP POST 地址示例</div>
                <code class="bot-management__onebot-code">{{ onebotHTTPPostExampleURL }}</code>
              </div>
              <div class="bot-management__onebot-label">说明：HTTP API 后缀已生效，可用于兼容非标准路径的 BOT；HTTP POST 请填写完整 URL，当前仅支持已实现的消息事件上报，不包含 quick operation、notice、request。</div>
              <div class="bot-management__onebot-label">鉴权方式：使用当前 BOT Token</div>
            </div>
          </template>
        </template>
      </template>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="avatarEditorVisible"
    :z-index="BOT_AVATAR_MODAL_Z_INDEX"
    preset="card"
    title="编辑头像"
    style="max-width: 450px;"
    :mask-closable="false"
  >
    <AvatarEditor
      :file="avatarEditorFile"
      @save="handleAvatarEditorSave"
      @cancel="handleAvatarEditorCancel"
    />
  </n-modal>
</template>

<style scoped>
.bot-management {
  display: flex;
  flex-direction: column;
  height: 61vh;
  min-height: 0;
  overflow: hidden;
}

.bot-management__body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 4px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.bot-management__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.bot-management__controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.bot-management__switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--n-text-color-2);
  padding: 0 4px;
}

.bot-management__stats {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.bot-management__stats-main {
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color-1);
}

.bot-management__stats-sub {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.bot-management__hint {
  font-size: 12px;
  color: var(--n-text-color-3);
  padding: 10px 12px;
  background: rgba(148, 163, 184, 0.08);
  border: 1px solid rgba(148, 163, 184, 0.12);
  border-radius: 10px;
}

.bot-management__system-note {
  font-size: 12px;
  color: #92400e;
  background: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.22);
  border-radius: 10px;
  padding: 10px 12px;
}

.bot-management__tabs {
  margin-top: -2px;
}

.bot-management__batch-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  background: rgba(37, 99, 235, 0.06);
  border: 1px solid rgba(37, 99, 235, 0.12);
  font-size: 13px;
}

.bot-management__batch-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.bot-management__batch-warning {
  color: #b45309;
  font-size: 12px;
}

.bot-management__table {
  position: relative;
  flex: 1 1 auto;
  min-height: 260px;
  margin-bottom: 4px;
}

.bot-management__table :deep(.n-data-table-wrapper) {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.3) transparent;
}

.bot-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.bot-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-track {
  background: transparent;
}

.bot-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.3);
  border-radius: 3px;
}

.bot-management__table :deep(.n-data-table-wrapper)::-webkit-scrollbar-thumb:hover {
  background: rgba(128, 128, 128, 0.5);
}

.bot-management__empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}

.bot-management__footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding-top: 10px;
  border-top: 1px solid var(--n-border-color);
  flex-shrink: 0;
  background: var(--n-color);
}

.bot-avatar-uploader {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.bot-avatar-uploader__preview {
  width: 40px;
  height: 40px;
  min-width: 40px;
  min-height: 40px;
  border-radius: 3px;
  object-fit: cover;
}

.bot-avatar-uploader__actions {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bot-avatar-uploader__hint {
  font-size: 12px;
  color: #94a3b8;
  margin: 0;
}

.bot-management__onebot-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color-1);
  margin-top: 4px;
}

.bot-management__onebot-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-top: -4px;
}

.bot-management__onebot-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border-radius: 10px;
  background: rgba(148, 163, 184, 0.08);
  border: 1px solid rgba(148, 163, 184, 0.12);
}

.bot-management__onebot-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.bot-management__onebot-label {
  font-size: 12px;
  color: var(--n-text-color-2);
}

.bot-management__onebot-code {
  display: block;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.08);
  color: var(--n-text-color-1);
  font-size: 12px;
  line-height: 1.5;
  word-break: break-all;
}

@media (max-width: 960px) {
  .bot-management {
    height: 68vh;
  }

  .bot-management__toolbar,
  .bot-management__batch-bar,
  .bot-management__footer {
    flex-direction: column;
    align-items: stretch;
  }

  .bot-management__stats {
    align-items: flex-start;
  }
}
</style>
