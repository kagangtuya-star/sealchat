<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue';
import { NDrawer, NDrawerContent, NButton, NIcon, NEmpty, NCard, NInput, NForm, NFormItem, NModal, NPopconfirm, NTag, NSwitch, NSelect, NDivider, NCheckbox, useMessage } from 'naive-ui';
import { Plus, Trash, Edit, Link, Eye, Upload, X, Refresh } from '@vicons/tabler';
import { characterApiUnsupportedText, useCharacterCardStore, type CharacterCard } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useCharacterCardTemplateStore, type CharacterCardTemplate } from '@/stores/characterCardTemplate';
import { useCharacterCardAvatarStore } from '@/stores/characterCardAvatar';
import { useChatStore } from '@/stores/chat';
import { useDisplayStore } from '@/stores/display';
import { useUtilsStore } from '@/stores/utils';
import { DEFAULT_CARD_TEMPLATE, getWorldCardTemplate, setWorldCardTemplate } from '@/utils/characterCardTemplate';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import AvatarVue from '@/components/avatar.vue';
import AvatarEditor from '@/components/AvatarEditor.vue';
import type { ChannelIdentity } from '@/types';

const props = defineProps<{
  visible: boolean;
  channelId?: string;
}>();

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void;
}>();

const message = useMessage();
const cardStore = useCharacterCardStore();
const sheetStore = useCharacterSheetStore();
const templateStore = useCharacterCardTemplateStore();
const avatarStore = useCharacterCardAvatarStore();
const chatStore = useChatStore();
const displayStore = useDisplayStore();
const utilsStore = useUtilsStore();

const viewportWidth = ref(typeof window === 'undefined' ? 1024 : window.innerWidth);
const updateViewportWidth = () => {
  if (typeof window === 'undefined') return;
  viewportWidth.value = window.innerWidth;
};
const isMobile = computed(() => viewportWidth.value < 768);
const drawerWidth = computed(() => (isMobile.value ? `${Math.max(320, viewportWidth.value)}px` : 420));

const resolvedChannelId = computed(() => props.channelId || chatStore.curChannel?.id || '');

const characterApiDisabled = computed(() => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return true;
  return cardStore.isBotCharacterDisabled(channelId);
});

const characterApiUnavailableText = computed(() => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return '请先选择频道';
  return cardStore.getCharacterApiDisabledReason(channelId) || characterApiUnsupportedText;
});
const revalidatingCharacterApi = ref(false);

const ensureCharacterApiEnabled = (showMessage = true) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    if (showMessage) {
      message.warning('请先选择频道');
    }
    return false;
  }
  if (!characterApiDisabled.value) {
    return true;
  }
  if (showMessage) {
    message.warning(characterApiUnavailableText.value);
  }
  return false;
};

const channelCards = computed(() => cardStore.getCardsByChannel(resolvedChannelId.value));

const identities = computed<ChannelIdentity[]>(() => {
  const id = resolvedChannelId.value;
  if (!id) return [];
  return chatStore.channelIdentities[id] || [];
});

const badgeEnabled = computed({
  get: () => displayStore.settings.characterCardBadgeEnabled,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardBadgeEnabled: value });
  },
});

const badgeAutoContrastEnabled = computed({
  get: () => displayStore.settings.characterCardBadgeAutoContrastEnabled,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardBadgeAutoContrastEnabled: value });
  },
});

const badgeTemplate = ref('');
const currentWorldId = computed(() => chatStore.currentWorldId || '');
const canSyncBadgeTemplate = computed(() => {
  const worldId = currentWorldId.value;
  if (!worldId) return false;
  const detail = chatStore.worldDetailMap[worldId];
  const role = detail?.memberRole;
  return role === 'owner' || role === 'admin';
});

const syncBadgeTemplate = () => {
  const worldId = currentWorldId.value;
  if (!worldId) {
    badgeTemplate.value = DEFAULT_CARD_TEMPLATE;
    return;
  }
  const stored = displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId];
  badgeTemplate.value = stored ?? getWorldCardTemplate(worldId);
};

const persistBadgeTemplate = () => {
  const worldId = currentWorldId.value;
  if (!worldId) return;
  const normalized = badgeTemplate.value.trim() || DEFAULT_CARD_TEMPLATE;
  badgeTemplate.value = normalized;
  setWorldCardTemplate(worldId, normalized);
  displayStore.updateSettings({
    characterCardBadgeTemplateByWorld: {
      ...displayStore.settings.characterCardBadgeTemplateByWorld,
      [worldId]: normalized,
    },
  });
};

const resetBadgeTemplate = () => {
  badgeTemplate.value = DEFAULT_CARD_TEMPLATE;
  persistBadgeTemplate();
};

const syncBadgeTemplateToWorld = async () => {
  const worldId = currentWorldId.value;
  if (!worldId) return;
  const normalized = badgeTemplate.value.trim() || DEFAULT_CARD_TEMPLATE;
  badgeTemplate.value = normalized;
  persistBadgeTemplate();
  try {
    await chatStore.worldUpdate(worldId, { characterCardBadgeTemplate: normalized });
    message.success('模板已同步');
  } catch (e: any) {
    message.error(e?.response?.data?.message || '模板同步失败');
  }
};

const loadPanelData = async (channelId: string) => {
  await cardStore.loadCards(channelId);
  await templateStore.ensureTemplatesLoaded();
  await templateStore.loadBindings(channelId);
  await avatarStore.loadBindings(channelId);
  await avatarStore.migrateLegacyBindings(
    channelId,
    channelCards.value,
    identities.value,
    cardStore.identityBindings,
  );
  await avatarStore.loadBindings(channelId);
  await templateStore.migrateLocalTemplatesIfNeeded(
    channelId,
    channelCards.value.map(item => ({ id: item.id, name: item.name, sheetType: item.sheetType || '' })),
  );
  await templateStore.loadBindings(channelId);
};

const handleRevalidateCharacterApi = async () => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  if (revalidatingCharacterApi.value) {
    return;
  }
  revalidatingCharacterApi.value = true;
  try {
    const result = await cardStore.revalidateCharacterApi(channelId);
    if (result.ok) {
      message.success('人物卡 API 验证成功，已解除禁用');
      return;
    }
    message.error(result.error || '人物卡 API 验证失败');
  } finally {
    revalidatingCharacterApi.value = false;
  }
};

watch(() => props.visible, async (val) => {
  if (val && resolvedChannelId.value && !characterApiDisabled.value) {
    await loadPanelData(resolvedChannelId.value);
  }
}, { immediate: true });

watch(resolvedChannelId, async (newId) => {
  if (props.visible && newId && !characterApiDisabled.value) {
    await loadPanelData(newId);
  }
});

watch(characterApiDisabled, async (disabled, prevDisabled) => {
  const channelId = resolvedChannelId.value;
  if (!props.visible || !channelId) {
    return;
  }
  if (prevDisabled && !disabled) {
    await loadPanelData(channelId);
  }
});

watch(
  [() => props.visible, channelCards],
  async ([visible, cards]) => {
    const channelId = resolvedChannelId.value;
    if (!visible || !channelId || !cards.length || characterApiDisabled.value) return;
    await avatarStore.migrateLegacyBindings(
      channelId,
      cards,
      identities.value,
      cardStore.identityBindings,
    );
    await avatarStore.loadBindings(channelId);
    await templateStore.migrateLocalTemplatesIfNeeded(
      channelId,
      cards.map(item => ({ id: item.id, name: item.name, sheetType: item.sheetType || '' })),
    );
    await templateStore.loadBindings(channelId);
  },
  { deep: true },
);

watch(
  [() => props.visible, currentWorldId],
  ([visible]) => {
    if (visible) {
      syncBadgeTemplate();
    }
  },
  { immediate: true },
);

watch(channelCards, (cards) => {
  const channelId = resolvedChannelId.value;
  if (!channelId || !cards.length) return;
  const bindingMap = templateStore.bindingsByChannel[channelId] || {};
  cards.forEach(card => {
    const binding = bindingMap[card.id];
    if (!binding) return;
    card.templateMode = binding.mode;
    card.templateId = binding.templateId || undefined;
    card.templateSnapshot = binding.templateSnapshot || undefined;
  });
}, { immediate: true, deep: true });

watch(badgeEnabled, (enabled) => {
  const channelId = resolvedChannelId.value;
  if (!channelId || characterApiDisabled.value) return;
  if (enabled) {
    void cardStore.requestBadgeSnapshot(channelId);
    void cardStore.getActiveCard(channelId);
    return;
  }
  void cardStore.broadcastActiveBadge(channelId, undefined, 'clear');
});

watch(
  () => sheetStore.activeWindowIds.map(windowId => {
    const window = sheetStore.windows[windowId];
    return `${windowId}:${window?.channelId || ''}:${window?.cardId || ''}`;
  }).join('|'),
  () => {
    Object.entries(channelSheetSwitchStates.value).forEach(([channelId, state]) => {
      if (state.switching) return;
      if (state.windowId && sheetStore.windows[state.windowId]) return;
      if (state.restoreToCurrentBinding) {
        upsertChannelSheetSwitchState(channelId, { switching: true });
        void restoreBoundCardAfterSheetClose(channelId);
        return;
      }
      clearChannelSheetSwitchState(channelId);
    });
  },
);

onMounted(() => {
  updateViewportWidth();
  window.addEventListener('resize', updateViewportWidth);
});

onBeforeUnmount(() => {
  if (typeof window === 'undefined') return;
  window.removeEventListener('resize', updateViewportWidth);
});

const handleClose = () => {
  templateManagerVisible.value = false;
  emit('update:visible', false);
};

const templateFilterSheetType = ref<string | null>(null);
const templateSearchKeyword = ref('');
const cardSearchKeyword = ref('');
const templateManagerVisible = ref(false);
const templateModalVisible = ref(false);
const templateEditingId = ref('');
const templateName = ref('');
const templateSheetTypePreset = ref('coc7');
const templateSheetTypeCustom = ref('');
const templateContent = ref('');
const templateGlobalDefault = ref(false);
const templateSheetDefault = ref(false);
const templateSaving = ref(false);

const managedTemplates = computed(() => {
  const filter = (templateFilterSheetType.value ?? '').trim().toLowerCase();
  return templateStore.templates.filter(item => {
    if (!filter) return true;
    return (item.sheetType || '').trim().toLowerCase() === filter;
  });
});

const filteredManagedTemplates = computed(() => {
  const keyword = templateSearchKeyword.value.trim().toLowerCase();
  if (!keyword) return managedTemplates.value;
  return managedTemplates.value.filter(item => {
    const name = (item.name || '').toLowerCase();
    const sheetType = (item.sheetType || '').toLowerCase();
    const content = (item.content || '').toLowerCase();
    return name.includes(keyword) || sheetType.includes(keyword) || content.includes(keyword);
  });
});

const allChannelCards = computed(() => (Array.isArray(channelCards.value) ? channelCards.value : []));

const buildCardAttrsSearchText = (attrs: Record<string, any> | undefined) => {
  if (!attrs || typeof attrs !== 'object') return '';
  return Object.entries(attrs)
    .map(([key, value]) => `${key}:${String(value ?? '')}`)
    .join(' ')
    .toLowerCase();
};

const filteredChannelCards = computed(() => {
  const source = allChannelCards.value;
  const keyword = cardSearchKeyword.value.trim().toLowerCase();
  if (!keyword) return source;
  return source.filter(card => {
    const name = (card.name || '').toLowerCase();
    const sheetType = (card.sheetType || '').toLowerCase();
    const attrs = buildCardAttrsSearchText(card.attrs);
    return name.includes(keyword) || sheetType.includes(keyword) || attrs.includes(keyword);
  });
});

const currentIdentityId = computed(() => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return '';
  return chatStore.getActiveIdentityId(channelId);
});

const currentBoundCardId = computed(() => {
  const identityId = currentIdentityId.value;
  if (!identityId) return '';
  return cardStore.getBoundCardId(identityId) || '';
});

const currentActiveCardId = computed(() => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return '';
  return cardStore.getActiveCardId(channelId);
});

const sortedFilteredChannelCards = computed(() => {
  const activeCardId = currentActiveCardId.value;
  const boundCardId = currentBoundCardId.value;
  return filteredChannelCards.value
    .map((card, index) => ({
      card,
      index,
      score: card.id === activeCardId ? 2 : (card.id === boundCardId ? 1 : 0),
    }))
    .sort((a, b) => {
      if (b.score !== a.score) return b.score - a.score;
      return a.index - b.index;
    })
    .map(item => item.card);
});

interface ChannelSheetSwitchState {
  cardId: string;
  windowId: string;
  switching: boolean;
  restoreToCurrentBinding: boolean;
}

const channelSheetSwitchStates = ref<Record<string, ChannelSheetSwitchState>>({});
const cardSwitchingId = ref('');

const setTemplateSheetType = (value: string) => {
  const normalized = (value || '').trim();
  const lower = normalized.toLowerCase();
  if (lower === 'coc7' || lower === 'coc') {
    templateSheetTypePreset.value = 'coc7';
    templateSheetTypeCustom.value = '';
    return;
  }
  if (lower === 'dnd5e' || lower === 'dnd5' || lower === 'dnd') {
    templateSheetTypePreset.value = 'dnd5e';
    templateSheetTypeCustom.value = '';
    return;
  }
  if (normalized) {
    templateSheetTypePreset.value = 'custom';
    templateSheetTypeCustom.value = normalized;
    return;
  }
  templateSheetTypePreset.value = 'custom';
  templateSheetTypeCustom.value = '';
};

const resolveTemplateSheetType = () => resolveSheetType(templateSheetTypePreset.value, templateSheetTypeCustom.value);

const openTemplateManager = async () => {
  if (!ensureCharacterApiEnabled()) return;
  await templateStore.ensureTemplatesLoaded();
  templateManagerVisible.value = true;
};

const openTemplateCreateModal = () => {
  if (!ensureCharacterApiEnabled()) return;
  templateEditingId.value = '';
  templateName.value = '';
  setTemplateSheetType('coc7');
  templateContent.value = sheetStore.getDefaultTemplate('coc7');
  templateGlobalDefault.value = false;
  templateSheetDefault.value = false;
  templateModalVisible.value = true;
};

const openTemplateEditModal = (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  templateEditingId.value = item.id;
  templateName.value = item.name;
  setTemplateSheetType(item.sheetType || '');
  templateContent.value = item.content;
  templateGlobalDefault.value = !!item.isGlobalDefault;
  templateSheetDefault.value = !!item.isSheetDefault;
  templateModalVisible.value = true;
};

const handleSaveTemplate = async () => {
  if (!ensureCharacterApiEnabled()) return;
  const name = templateName.value.trim();
  const sheetType = resolveTemplateSheetType();
  const content = templateContent.value.trim();
  if (!name) {
    message.warning('请输入模板名称');
    return;
  }
  if (!content) {
    message.warning('模板内容不能为空');
    return;
  }
  templateSaving.value = true;
  try {
    if (templateEditingId.value) {
      await templateStore.updateTemplate(templateEditingId.value, {
        name,
        sheetType,
        content,
        isGlobalDefault: templateGlobalDefault.value,
        isSheetDefault: templateSheetDefault.value,
      });
      message.success('模板已更新');
    } else {
      await templateStore.createTemplate({
        name,
        sheetType,
        content,
        isGlobalDefault: templateGlobalDefault.value,
        isSheetDefault: templateSheetDefault.value,
      });
      message.success('模板已创建');
    }
    templateModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '模板保存失败');
  } finally {
    templateSaving.value = false;
  }
};

const handleDeleteTemplate = async (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  try {
    await templateStore.deleteTemplate(item.id);
    message.success('模板已删除');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '模板删除失败');
  }
};

const handleCopyTemplate = async (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  try {
    await templateStore.createTemplate({
      name: `${item.name}-副本`,
      sheetType: item.sheetType,
      content: item.content,
    });
    message.success('模板已复制');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '模板复制失败');
  }
};

const setAsGlobalDefault = async (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  try {
    await templateStore.setTemplateDefault(item.id, 'global');
    message.success('已设为全局默认模板');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '设置失败');
  }
};

const setAsSheetDefault = async (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  if (!(item.sheetType || '').trim()) {
    message.warning('该模板缺少规制类型，无法设为规制默认');
    return;
  }
  try {
    await templateStore.setTemplateDefault(item.id, 'sheet');
    message.success(`已设为 ${item.sheetType} 默认模板`);
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '设置失败');
  }
};

const formatTemplatePreview = (content: string) => {
  const plain = String(content || '').replace(/\s+/g, ' ').trim();
  if (plain.length <= 120) return plain;
  return `${plain.slice(0, 120)}...`;
};

// Create card modal
const createModalVisible = ref(false);
const newCardName = ref('');
const newCardSheetTypePreset = ref('coc7');
const newCardSheetTypeCustom = ref('');
const newCardAttrs = ref<Record<string, any>>({});
const creating = ref(false);

const sheetTypeOptions = [
  { label: 'COC7', value: 'coc7' },
  { label: 'DND5', value: 'dnd5e' },
  { label: '自定义', value: 'custom' },
];

const resolveSheetType = (preset: string, custom: string) => {
  if (preset === 'custom') {
    return custom.trim();
  }
  return preset;
};

const openCreateModal = () => {
  if (!ensureCharacterApiEnabled()) return;
  newCardName.value = '';
  newCardSheetTypePreset.value = 'coc7';
  newCardSheetTypeCustom.value = '';
  newCardAttrs.value = {};
  createModalVisible.value = true;
};

const handleCreateCard = async () => {
  if (!ensureCharacterApiEnabled()) return;
  if (!newCardName.value.trim()) {
    message.warning('请输入角色名称');
    return;
  }
  const sheetType = resolveSheetType(newCardSheetTypePreset.value, newCardSheetTypeCustom.value);
  if (!sheetType) {
    message.warning('请输入自定义规制类型');
    return;
  }
  creating.value = true;
  try {
    await cardStore.createCard(resolvedChannelId.value, newCardName.value.trim(), sheetType, newCardAttrs.value);
    message.success('创建成功');
    createModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '创建失败');
  } finally {
    creating.value = false;
  }
};

const handleDeleteCard = async (card: CharacterCard) => {
  if (!ensureCharacterApiEnabled()) return;
  try {
    await cardStore.deleteCard(card.id);
    if (resolvedChannelId.value) {
      try {
        await avatarStore.removeBinding(resolvedChannelId.value, card.id);
      } catch (avatarError) {
        console.warn('Failed to remove character card avatar binding after delete', avatarError);
      }
    }
    message.success('已删除');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '删除失败');
  }
};

// Bind modal
const bindModalVisible = ref(false);
const bindingCard = ref<CharacterCard | null>(null);
const selectedIdentityId = ref<string | null>(null);

const identityOptions = computed(() => {
  return identities.value.map(i => ({
    label: i.displayName || '未命名身份',
    value: i.id,
  }));
});

const openBindModal = (card: CharacterCard) => {
  if (!ensureCharacterApiEnabled()) return;
  bindingCard.value = card;
  selectedIdentityId.value = null;
  bindModalVisible.value = true;
};

const handleBind = async () => {
  if (!ensureCharacterApiEnabled()) return;
  if (!bindingCard.value || !selectedIdentityId.value || !resolvedChannelId.value) return;
  try {
    await cardStore.bindIdentity(resolvedChannelId.value, selectedIdentityId.value, bindingCard.value.id);
    message.success('绑定成功');
    bindModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '绑定失败');
  }
};

const getBoundIdentities = (cardId: string) => {
  const result: ChannelIdentity[] = [];
  for (const [identityId, boundCardId] of Object.entries(cardStore.identityBindings)) {
    if (boundCardId === cardId) {
      const identity = identities.value.find(i => i.id === identityId);
      if (identity) result.push(identity);
    }
  }
  return result;
};

const resolveCardAvatarToken = (card: CharacterCard, fallbackAvatarUrl = '') => {
  return avatarStore.resolveCardAvatar(card.id, resolvedChannelId.value, fallbackAvatarUrl);
};

const getCardAvatarBinding = (card: CharacterCard) => {
  return avatarStore.getBinding(resolvedChannelId.value, card.id);
};

const syncSheetAvatar = (card: CharacterCard, fallbackAvatarUrl = '') => {
  sheetStore.updateCardAvatar(card.id, resolveCardAvatarToken(card, fallbackAvatarUrl) || undefined);
};

const handleUnbind = async (identityId: string) => {
  if (!ensureCharacterApiEnabled()) return;
  if (!resolvedChannelId.value) return;
  try {
    await cardStore.unbindIdentity(resolvedChannelId.value, identityId);
    message.success('已解绑');
  } catch (e: any) {
    message.error(e?.response?.data?.error || '解绑失败');
  }
};

const getCardAttrEntries = (attrs: Record<string, any> | undefined) => {
  if (!attrs || typeof attrs !== 'object') return [];
  return Object.entries(attrs).filter(([, value]) => {
    if (value === undefined || value === null) return false;
    return String(value).trim() !== '';
  });
};

const isCurrentActiveCard = (card: CharacterCard) => card.id === currentActiveCardId.value;

const isCurrentBoundCard = (card: CharacterCard) => {
  const boundCardId = currentBoundCardId.value;
  return !!boundCardId && card.id === boundCardId;
};

const upsertChannelSheetSwitchState = (channelId: string, patch: Partial<ChannelSheetSwitchState>) => {
  if (!channelId) return;
  const prev = channelSheetSwitchStates.value[channelId] || {
    cardId: '',
    windowId: '',
    switching: false,
    restoreToCurrentBinding: false,
  };
  channelSheetSwitchStates.value = {
    ...channelSheetSwitchStates.value,
    [channelId]: {
      ...prev,
      ...patch,
    },
  };
};

const clearChannelSheetSwitchState = (channelId: string) => {
  if (!channelId || !channelSheetSwitchStates.value[channelId]) return;
  const next = { ...channelSheetSwitchStates.value };
  delete next[channelId];
  channelSheetSwitchStates.value = next;
};

const getChannelSheetWindows = (channelId: string) =>
  sheetStore.activeWindowIds
    .map(windowId => sheetStore.windows[windowId])
    .filter(window => window?.channelId === channelId);

const closeChannelSheetWindows = (channelId: string, exceptCardId = '') => {
  if (!channelId) return;
  const windows = getChannelSheetWindows(channelId);
  windows.forEach((window) => {
    if (!window?.id) return;
    if (exceptCardId && window.cardId === exceptCardId) return;
    sheetStore.closeSheet(window.id);
  });
};

const restoreBoundCardAfterSheetClose = async (channelId: string) => {
  if (!channelId) return;
  const identityId = chatStore.getActiveIdentityId(channelId);
  const boundCardId = identityId ? (cardStore.getBoundCardId(identityId) || '') : '';
  try {
    if (boundCardId && cardStore.getActiveCardId(channelId) !== boundCardId) {
      await cardStore.tagCard(channelId, undefined, boundCardId);
    }
  } catch (e) {
    console.warn('Failed to restore bound character card after sheet close', e);
  } finally {
    clearChannelSheetSwitchState(channelId);
  }
};

const avatarUploadInputRef = ref<HTMLInputElement | null>(null);
const avatarEditingCard = ref<CharacterCard | null>(null);
const avatarEditorVisible = ref(false);
const avatarEditorFile = ref<File | null>(null);
const avatarUploading = ref(false);

const handleAvatarUploadTrigger = (card: CharacterCard) => {
  avatarEditingCard.value = card;
  avatarUploadInputRef.value?.click();
};

const handleAvatarFileChange = (event: Event) => {
  const input = event.target as HTMLInputElement | null;
  if (!input || !input.files?.length) {
    return;
  }
  const file = input.files[0];
  const sizeLimit = utilsStore.config?.imageSizeLimit ? utilsStore.config.imageSizeLimit * 1024 : utilsStore.fileSizeLimit;
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

const handleAvatarEditorCancel = () => {
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  avatarEditingCard.value = null;
};

const handleAvatarEditorSave = async (file: File) => {
  const card = avatarEditingCard.value;
  const channelId = resolvedChannelId.value;
  if (!card || !channelId) {
    handleAvatarEditorCancel();
    return;
  }
  avatarUploading.value = true;
  try {
    const uploadResult = await uploadImageAttachment(file, { channelId, skipCompression: true });
    await avatarStore.upsertBinding({
      channelId,
      externalCardId: card.id,
      cardName: card.name,
      sheetType: card.sheetType || '',
      avatarAttachmentId: uploadResult.attachmentId,
    });
    syncSheetAvatar(card);
    message.success('头像已更新');
    handleAvatarEditorCancel();
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '头像上传失败');
  } finally {
    avatarUploading.value = false;
  }
};

const handleAvatarRemove = async (card: CharacterCard) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return;
  try {
    await avatarStore.removeBinding(channelId, card.id);
    const activeFallback = cardStore.getActiveCardId(channelId) === card.id
      ? (cardStore.activeCards[channelId]?.avatarUrl || '')
      : '';
    syncSheetAvatar(card, activeFallback);
    message.success('头像已移除');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '头像移除失败');
  }
};

const openCharacterSheetWindow = async (
  card: CharacterCard,
  mode: 'view' | 'edit' = 'view',
  options?: { restoreToCurrentBinding?: boolean },
) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }

  closeChannelSheetWindows(channelId, card.id);

  if (characterApiDisabled.value) {
    const avatarUrl = resolveCardAvatarToken(card);
    const windowId = sheetStore.openSheet(card, channelId, {
      name: card.name,
      type: card.sheetType,
      attrs: card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    });
    upsertChannelSheetSwitchState(channelId, {
      cardId: card.id,
      windowId,
      switching: false,
      restoreToCurrentBinding: false,
    });
    if (mode === 'edit') {
      sheetStore.setMode(windowId, 'edit');
    }
    if (isMobile.value) {
      handleClose();
    }
    return;
  }

  try {
    let cardData = cardStore.activeCards[channelId];
    const activeCardId = cardStore.getActiveCardId(channelId);
    const shouldUseActiveCardData = activeCardId === card.id;
    if (!cardData || shouldUseActiveCardData) {
      await cardStore.getActiveCard(channelId);
      cardData = cardStore.activeCards[channelId];
    }
    const effectiveCardData = cardStore.getActiveCardId(channelId) === card.id ? cardData : undefined;
    await templateStore.ensureTemplatesLoaded();
    await templateStore.ensureBindingsLoaded(channelId);
    const resolvedSheetType = (effectiveCardData?.type || card.sheetType || '').trim();
    const fallbackTemplate = sheetStore.getTemplate(card.id, resolvedSheetType);
    const ensured = await templateStore.ensureCardBinding({
      channelId,
      externalCardId: card.id,
      cardName: card.name,
      sheetType: resolvedSheetType,
      fallbackTemplate,
    });
    const binding = templateStore.getBinding(channelId, card.id) || ensured;
    if (binding?.mode) {
      card.templateMode = binding.mode;
      card.templateId = binding.templateId || undefined;
      card.templateSnapshot = binding.templateSnapshot || undefined;
    }
    const avatarUrl = resolveCardAvatarToken(card, effectiveCardData?.avatarUrl || '');
    const windowId = sheetStore.openSheet(card, channelId, {
      name: effectiveCardData?.name || card.name,
      type: effectiveCardData?.type || card.sheetType,
      attrs: effectiveCardData?.attrs || card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    }, {
      templateMode: binding?.mode,
      templateId: binding?.templateId || undefined,
      templateText: binding?.mode === 'detached' ? binding.templateSnapshot : undefined,
    });
    upsertChannelSheetSwitchState(channelId, {
      cardId: card.id,
      windowId,
      switching: false,
      restoreToCurrentBinding: !!options?.restoreToCurrentBinding,
    });
    if (mode === 'edit') {
      sheetStore.setMode(windowId, 'edit');
    }
    if (isMobile.value) {
      handleClose();
    }
  } catch (e: any) {
    console.warn('Failed to open character preview', e);
    const avatarUrl = resolveCardAvatarToken(card);
    const windowId = sheetStore.openSheet(card, channelId, {
      name: card.name,
      type: card.sheetType,
      attrs: card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    });
    upsertChannelSheetSwitchState(channelId, {
      cardId: card.id,
      windowId,
      switching: false,
      restoreToCurrentBinding: !!options?.restoreToCurrentBinding,
    });
    if (mode === 'edit') {
      sheetStore.setMode(windowId, 'edit');
    }
    if (isMobile.value) {
      handleClose();
    }
  }
};

const openCharacterSheet = async (card: CharacterCard, mode: 'view' | 'edit' = 'view') => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }

  const shouldSwitchCard = !characterApiDisabled.value && currentActiveCardId.value !== card.id;
  const restoreToCurrentBinding = !characterApiDisabled.value
    && !!currentBoundCardId.value
    && currentBoundCardId.value !== card.id;

  cardSwitchingId.value = card.id;
  upsertChannelSheetSwitchState(channelId, {
    cardId: card.id,
    windowId: '',
    switching: true,
    restoreToCurrentBinding,
  });

  try {
    closeChannelSheetWindows(channelId);
    if (shouldSwitchCard) {
      const switched = await cardStore.tagCard(channelId, undefined, card.id);
      if (!switched) {
        throw new Error('切换人物卡失败');
      }
    }
    await openCharacterSheetWindow(card, mode, { restoreToCurrentBinding });
  } catch (e: any) {
    clearChannelSheetSwitchState(channelId);
    message.error(e?.response?.data?.error || e?.message || '切换人物卡失败');
  } finally {
    if (cardSwitchingId.value === card.id) {
      cardSwitchingId.value = '';
    }
  }
};

const openPreview = async (card: CharacterCard) => {
  await openCharacterSheet(card, 'view');
};

const openEditPanel = async (card: CharacterCard) => {
  await openCharacterSheet(card, 'edit');
};
</script>

<template>
  <n-drawer
    :show="visible"
    placement="right"
    :width="drawerWidth"
    @update:show="handleClose"
  >
    <n-drawer-content closable>
      <template #header>
        <div class="character-card-header">
          <div class="character-card-header__left">
            <n-button v-if="isMobile" size="tiny" quaternary @click="handleClose">返回</n-button>
            <span>人物卡管理</span>
          </div>
          <div class="character-card-header__actions">
            <n-button size="small" type="primary" :disabled="characterApiDisabled" @click="openTemplateManager">模板管理器</n-button>
            <n-button size="small" type="primary" :disabled="characterApiDisabled" @click="openCreateModal">
              <template #icon><n-icon :component="Plus" /></template>
              新建
            </n-button>
          </div>
        </div>
      </template>

      <div v-if="characterApiDisabled" class="character-api-unavailable">
        <span class="character-api-unavailable__text">{{ characterApiUnavailableText }}</span>
        <n-button
          size="tiny"
          tertiary
          type="warning"
          :loading="revalidatingCharacterApi"
          :disabled="!resolvedChannelId"
          @click="handleRevalidateCharacterApi"
        >
          重新验证
        </n-button>
      </div>

      <div class="character-card-settings">
        <div class="settings-row">
          <div>
            <p class="settings-title">聊天角色徽章</p>
            <p class="settings-desc">开启后且可读到人物卡数据时，在昵称后显示简洁属性</p>
          </div>
          <n-switch v-model:value="badgeEnabled" :disabled="characterApiDisabled">
            <template #checked>已启用</template>
            <template #unchecked>已关闭</template>
          </n-switch>
        </div>
        <div class="settings-row">
          <div>
            <p class="settings-title">自动提高可读性</p>
            <p class="settings-desc">当徽标颜色与频道背景接近时，自动调整文字、底色与边框</p>
          </div>
          <n-switch v-model:value="badgeAutoContrastEnabled">
            <template #checked>已启用</template>
            <template #unchecked>已关闭</template>
          </n-switch>
        </div>
        <div class="settings-row settings-row--template">
          <div>
            <p class="settings-title">徽章模板</p>
            <p class="settings-desc">使用 {属性名} 占位，例如：HP{生命值} SAN{理智} 闪避{闪避}</p>
          </div>
          <div class="settings-template-input">
            <n-input
              v-model:value="badgeTemplate"
              size="small"
              :disabled="characterApiDisabled"
              placeholder="HP{生命值} SAN{理智} 闪避{闪避}"
              @blur="persistBadgeTemplate"
            />
            <n-button size="small" quaternary :disabled="characterApiDisabled" @click="resetBadgeTemplate">恢复默认</n-button>
            <n-button
              v-if="canSyncBadgeTemplate"
              size="small"
              tertiary
              :disabled="characterApiDisabled"
              @click="syncBadgeTemplateToWorld"
            >模板同步</n-button>
          </div>
        </div>
      </div>

      <n-divider style="margin: 8px 0 12px;" />

      <div class="card-search-row">
        <n-input
          v-model:value="cardSearchKeyword"
          size="small"
          clearable
          :disabled="characterApiDisabled"
          placeholder="搜索人物卡（名称/规制/属性）"
        />
      </div>

      <div class="character-card-list">
        <n-empty v-if="allChannelCards.length === 0" description="暂无人物卡" />
        <n-empty v-else-if="filteredChannelCards.length === 0" description="未找到匹配人物卡" />
        <n-card
          v-for="card in sortedFilteredChannelCards"
          :key="card.id"
          size="small"
          :class="[
            'character-card-item',
            {
              'character-card-item--active': isCurrentActiveCard(card),
              'character-card-item--bound': isCurrentBoundCard(card),
            },
          ]"
        >
          <template #header>
            <div class="card-header-main">
              <AvatarVue :size="34" :src="resolveCardAvatarToken(card)" />
              <span class="card-name">{{ card.name }}</span>
              <span v-if="isCurrentActiveCard(card)" class="card-state-badge card-state-badge--active">使用中</span>
              <span v-else-if="isCurrentBoundCard(card)" class="card-state-badge">当前角色</span>
              <n-tag size="small" :bordered="false">{{ card.sheetType || 'custom' }}</n-tag>
            </div>
          </template>
          <template #header-extra>
            <n-button
              v-if="!isCurrentActiveCard(card)"
              text
              size="small"
              title="切换并查看"
              :loading="cardSwitchingId === card.id"
              :disabled="characterApiDisabled || cardSwitchingId.length > 0"
              @click="openPreview(card)"
            >
              <template #icon><n-icon :component="Refresh" /></template>
            </n-button>
            <n-button
              quaternary
              circle
              size="small"
              title="上传头像"
              aria-label="上传头像"
              :disabled="characterApiDisabled || avatarUploading || cardSwitchingId.length > 0"
              @click="handleAvatarUploadTrigger(card)"
            >
              <template #icon><n-icon :component="Upload" /></template>
            </n-button>
            <n-button
              v-if="getCardAvatarBinding(card)"
              quaternary
              circle
              size="small"
              type="error"
              title="移除头像"
              aria-label="移除头像"
              :disabled="characterApiDisabled || avatarUploading || cardSwitchingId.length > 0"
              @click="handleAvatarRemove(card)"
            >
              <template #icon><n-icon :component="X" /></template>
            </n-button>
            <n-button
              text
              size="small"
              title="预览"
              :loading="cardSwitchingId === card.id"
              :disabled="cardSwitchingId.length > 0 && cardSwitchingId !== card.id"
              @click="openPreview(card)"
            >
              <template #icon><n-icon :component="Eye" /></template>
            </n-button>
            <n-button
              text
              size="small"
              :disabled="characterApiDisabled || (cardSwitchingId.length > 0 && cardSwitchingId !== card.id)"
              :loading="cardSwitchingId === card.id"
              @click="openEditPanel(card)"
            >
              <template #icon><n-icon :component="Edit" /></template>
            </n-button>
            <n-button
              text
              size="small"
              :disabled="characterApiDisabled || cardSwitchingId.length > 0"
              @click="openBindModal(card)"
            >
              <template #icon><n-icon :component="Link" /></template>
            </n-button>
            <n-popconfirm @positive-click="handleDeleteCard(card)">
              <template #trigger>
                <n-button text size="small" type="error" :disabled="characterApiDisabled || cardSwitchingId.length > 0">
                  <template #icon><n-icon :component="Trash" /></template>
                </n-button>
              </template>
              删除前将从所有群解绑此人物卡，确定删除？
            </n-popconfirm>
          </template>
          <div class="card-main-content">
            <div v-if="getCardAttrEntries(card.attrs).length > 0" class="card-attrs">
              <div class="card-attr-list">
                <span
                  v-for="[key, value] in getCardAttrEntries(card.attrs)"
                  :key="`${card.id}-${key}`"
                  class="card-attr-chip"
                >
                  <span class="card-attr-chip__key">{{ key }}</span>
                  <span class="card-attr-chip__value">{{ value }}</span>
                </span>
              </div>
            </div>

            <div v-if="getBoundIdentities(card.id).length > 0" class="card-bindings">
              <span class="bindings-label">绑定</span>
              <div class="card-bindings__tags">
                <n-tag
                  v-for="identity in getBoundIdentities(card.id)"
                  :key="identity.id"
                  size="small"
                  :closable="!characterApiDisabled"
                  @close="handleUnbind(identity.id)"
                >
                  {{ identity.displayName }}
                </n-tag>
              </div>
            </div>
          </div>
        </n-card>
      </div>
    </n-drawer-content>
  </n-drawer>

  <input
    ref="avatarUploadInputRef"
    type="file"
    accept="image/*"
    class="card-avatar-file-input"
    @change="handleAvatarFileChange"
  />

  <!-- Create Modal -->
  <n-modal
    v-model:show="createModalVisible"
    preset="dialog"
    :show-icon="false"
    title="新建人物卡"
    :positive-text="creating ? '创建中…' : '创建'"
    :positive-button-props="{ loading: creating, disabled: characterApiDisabled }"
    negative-text="取消"
    @positive-click="handleCreateCard"
  >
    <n-form label-width="80">
      <n-form-item label="角色名称">
        <n-input v-model:value="newCardName" maxlength="32" placeholder="请输入角色名称" />
      </n-form-item>
      <n-form-item label="卡片类型">
        <n-select v-model:value="newCardSheetTypePreset" :options="sheetTypeOptions" :disabled="characterApiDisabled" />
        <n-input
          v-if="newCardSheetTypePreset === 'custom'"
          v-model:value="newCardSheetTypeCustom"
          placeholder="输入自定义规制类型"
          class="sheet-type-custom-input"
          :disabled="characterApiDisabled"
        />
      </n-form-item>
    </n-form>
  </n-modal>

  <!-- Template Manager Modal -->
  <n-modal
    v-model:show="templateManagerVisible"
    preset="card"
    title="模板管理器"
    style="width: min(900px, 92vw);"
    :bordered="false"
  >
    <div class="template-manager template-manager--modal">
      <div class="template-manager__toolbar">
        <n-select
          v-model:value="templateFilterSheetType"
          :options="sheetTypeOptions.filter(opt => opt.value !== 'custom')"
          placeholder="全部规制"
          size="small"
          clearable
          :disabled="characterApiDisabled"
        />
        <n-input
          v-model:value="templateSearchKeyword"
          size="small"
          clearable
          placeholder="搜索模板（名称/内容）"
          :disabled="characterApiDisabled"
        />
        <n-button size="small" type="primary" :disabled="characterApiDisabled" @click="openTemplateCreateModal">新增模板</n-button>
      </div>

      <n-empty v-if="filteredManagedTemplates.length === 0" description="暂无模板" />
      <n-card v-for="tpl in filteredManagedTemplates" :key="tpl.id" size="small" class="template-manager__item">
        <template #header>
          <div class="template-manager__header">
            <span>{{ tpl.name }}</span>
            <div class="template-manager__tags">
              <n-tag size="small" :bordered="false">{{ tpl.sheetType || '通用' }}</n-tag>
              <n-tag v-if="tpl.isGlobalDefault" size="small" type="info" :bordered="false">全局默认</n-tag>
              <n-tag v-if="tpl.isSheetDefault" size="small" type="success" :bordered="false">规制默认</n-tag>
            </div>
          </div>
        </template>
        <div class="template-manager__preview">{{ formatTemplatePreview(tpl.content) || '空模板' }}</div>
        <div class="template-manager__actions">
          <n-button text size="small" :disabled="characterApiDisabled" @click="openTemplateEditModal(tpl)">编辑</n-button>
          <n-button text size="small" :disabled="characterApiDisabled" @click="handleCopyTemplate(tpl)">复制</n-button>
          <n-button text size="small" :disabled="characterApiDisabled" @click="setAsGlobalDefault(tpl)">设为全局默认</n-button>
          <n-button text size="small" :disabled="characterApiDisabled" @click="setAsSheetDefault(tpl)">设为规制默认</n-button>
          <n-popconfirm @positive-click="handleDeleteTemplate(tpl)">
            <template #trigger>
              <n-button text size="small" type="error" :disabled="characterApiDisabled">删除</n-button>
            </template>
            删除模板后，已引用卡片会转为脱离模板快照，确认删除？
          </n-popconfirm>
        </div>
      </n-card>
    </div>
  </n-modal>

  <!-- Template Create/Edit Modal -->
  <n-modal
    v-model:show="templateModalVisible"
    preset="dialog"
    :show-icon="false"
    :title="templateEditingId ? '编辑模板' : '新建模板'"
    :positive-text="templateSaving ? '保存中…' : '保存'"
    :positive-button-props="{ loading: templateSaving, disabled: characterApiDisabled }"
    negative-text="取消"
    @positive-click="handleSaveTemplate"
  >
    <n-form label-width="90">
      <n-form-item label="模板名称">
        <n-input v-model:value="templateName" maxlength="100" placeholder="输入模板名称" :disabled="characterApiDisabled" />
      </n-form-item>
      <n-form-item label="规制类型">
        <n-select v-model:value="templateSheetTypePreset" :options="sheetTypeOptions" :disabled="characterApiDisabled" />
        <n-input
          v-if="templateSheetTypePreset === 'custom'"
          v-model:value="templateSheetTypeCustom"
          placeholder="输入自定义规制类型"
          class="sheet-type-custom-input"
          :disabled="characterApiDisabled"
        />
      </n-form-item>
      <n-form-item label="模板内容">
        <n-input
          v-model:value="templateContent"
          type="textarea"
          :autosize="{ minRows: 8, maxRows: 16 }"
          placeholder="输入 HTML 模板"
          :disabled="characterApiDisabled"
        />
      </n-form-item>
      <n-form-item label="默认设置">
        <div class="template-manager__defaults">
          <n-checkbox v-model:checked="templateGlobalDefault" :disabled="characterApiDisabled">设为全局默认</n-checkbox>
          <n-checkbox v-model:checked="templateSheetDefault" :disabled="characterApiDisabled">设为规制默认</n-checkbox>
        </div>
      </n-form-item>
    </n-form>
  </n-modal>

  <!-- Bind Modal -->
  <n-modal
    v-model:show="bindModalVisible"
    preset="dialog"
    :show-icon="false"
    title="绑定身份"
    positive-text="绑定"
    negative-text="取消"
    :positive-button-props="{ disabled: characterApiDisabled }"
    @positive-click="handleBind"
  >
    <n-form label-width="80">
      <n-form-item label="选择身份">
        <n-select
          v-model:value="selectedIdentityId"
          :options="identityOptions"
          placeholder="选择要绑定的频道身份"
          :disabled="characterApiDisabled"
        />
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="avatarEditorVisible"
    preset="card"
    title="裁剪人物卡头像"
    style="max-width: 560px;"
    :mask-closable="false"
  >
    <AvatarEditor
      :file="avatarEditorFile"
      @save="handleAvatarEditorSave"
      @cancel="handleAvatarEditorCancel"
    />
  </n-modal>
</template>

<style lang="scss" scoped>
.character-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 0.75rem;
  padding-right: 1rem;
}

.character-card-header__left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.card-avatar-file-input {
  display: none;
}

.character-card-header__actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.character-card-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.character-card-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.25rem 0 1rem;
  border-bottom: 1px solid var(--sc-border-color);
  margin-bottom: 1rem;
}

.character-api-unavailable {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
  padding: 0.65rem 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(245, 158, 11, 0.35);
  background: rgba(245, 158, 11, 0.12);
  color: var(--sc-text-primary);
  font-size: 0.82rem;
  line-height: 1.4;
}

.character-api-unavailable__text {
  flex: 1;
  min-width: 0;
}

.settings-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.settings-row--template {
  align-items: flex-start;
}

.settings-title {
  font-weight: 500;
  margin-bottom: 0.1rem;
}

.settings-desc {
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.settings-template-input {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 210px;
}

.template-manager {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  margin-bottom: 1rem;
}

.template-manager--modal {
  max-height: 72vh;
  overflow: auto;
  padding-right: 0.25rem;
}

.card-search-row {
  margin-bottom: 0.75rem;
}

.template-manager__toolbar {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr) auto;
  gap: 0.5rem;
}

.template-manager__item {
  :deep(.n-card__content) {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
}

.template-manager__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  width: 100%;
}

.template-manager__tags {
  display: flex;
  gap: 0.3rem;
  flex-wrap: wrap;
}

.template-manager__preview {
  font-size: 0.78rem;
  color: var(--sc-text-secondary);
  line-height: 1.35;
}

.template-manager__actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.25rem;
}

.template-manager__defaults {
  display: flex;
  gap: 0.8rem;
  flex-wrap: wrap;
}

.character-card-item {
  :deep(.n-card) {
    border-radius: 10px;
  }

  &.character-card-item--active :deep(.n-card) {
    border-color: rgba(59, 130, 246, 0.42);
    box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.16);
    background: rgba(59, 130, 246, 0.04);
  }

  &.character-card-item--bound:not(.character-card-item--active) :deep(.n-card) {
    border-color: rgba(148, 163, 184, 0.3);
  }

  :deep(.n-card-header) {
    align-items: flex-start;
    gap: 0.5rem;
    padding-bottom: 0.45rem;
  }

  :deep(.n-card-header__main) {
    min-width: 0;
  }

  :deep(.n-card-header__extra) {
    display: flex;
    align-items: center;
    gap: 0.1rem;
    flex-wrap: nowrap;
  }

  :deep(.n-card__content) {
    padding-top: 0;
    padding-bottom: 0.1rem;
  }

  .card-header-main {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
    overflow: hidden;
  }

  .card-name {
    font-weight: 600;
    font-size: 0.92rem;
    min-width: 0;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-state-badge {
    display: inline-flex;
    align-items: center;
    flex: 0 0 auto;
    padding: 0.1rem 0.38rem;
    border-radius: 999px;
    background: rgba(148, 163, 184, 0.14);
    color: var(--sc-text-secondary);
    font-size: 0.68rem;
    line-height: 1.1;
    white-space: nowrap;
  }

  .card-state-badge--active {
    background: rgba(59, 130, 246, 0.16);
    color: #60a5fa;
  }

  .card-main-content {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    min-width: 0;
  }

  .card-attrs {
    min-width: 0;
  }

  .card-attr-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .card-attr-chip {
    display: inline-flex;
    align-items: baseline;
    gap: 0.22rem;
    min-width: 0;
    max-width: 100%;
    padding: 0.2rem 0.42rem;
    border-radius: 999px;
    background: rgba(148, 163, 184, 0.12);
    color: var(--sc-text-secondary);
    font-size: 0.75rem;
    line-height: 1.15;
  }

  .card-attr-chip__key {
    color: var(--sc-text-tertiary);
    white-space: nowrap;
  }

  .card-attr-chip__value {
    color: var(--sc-text-primary);
    overflow-wrap: anywhere;
  }

  .card-bindings {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    min-width: 0;
    overflow: hidden;

    .bindings-label {
      flex: 0 0 auto;
      font-size: 0.7rem;
      color: var(--sc-text-tertiary);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
  }

  .card-bindings__tags {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
  }

  .card-bindings__tags :deep(.n-tag) {
    max-width: 9rem;
    flex: 0 1 auto;
    overflow: hidden;
  }

  .card-bindings__tags :deep(.n-tag__content) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.sheet-type-custom-input {
  margin-top: 8px;
}

@media (max-width: 767px) {
  .character-card-header {
    padding-right: 0;
  }

  .character-card-header__actions {
    gap: 0.35rem;
  }

  .settings-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .settings-template-input {
    min-width: 0;
    width: 100%;
  }

  .template-manager__toolbar {
    grid-template-columns: 1fr;
  }

  .character-card-item :deep(.n-card-header) {
    align-items: flex-start;
    gap: 0.25rem;
  }

  .character-card-item :deep(.n-card-header__extra) {
    display: flex;
    align-items: center;
    gap: 0.15rem;
  }

  .character-card-item :deep(.n-card-header__extra .n-button) {
    min-width: 30px;
    min-height: 30px;
  }
}
</style>
