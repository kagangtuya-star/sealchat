<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue';
import { NDrawer, NDrawerContent, NButton, NIcon, NEmpty, NCard, NInput, NForm, NFormItem, NModal, NPopconfirm, NTag, NSwitch, NSelect, NDivider, NCheckbox, useMessage } from 'naive-ui';
import { Plus, Trash, Edit, Link, Eye } from '@vicons/tabler';
import { characterApiUnsupportedText, useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useCharacterCardTemplateStore, type CharacterCardTemplate } from '@/stores/characterCardTemplate';
import { useChatStore } from '@/stores/chat';
import { useDisplayStore } from '@/stores/display';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { DEFAULT_CARD_TEMPLATE, getWorldCardTemplate, setWorldCardTemplate } from '@/utils/characterCardTemplate';
import type { CharacterCard, ChannelIdentity } from '@/types';

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
const chatStore = useChatStore();
const displayStore = useDisplayStore();

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

watch(() => props.visible, async (val) => {
  if (val && resolvedChannelId.value && !characterApiDisabled.value) {
    await cardStore.loadCards(resolvedChannelId.value);
    await templateStore.ensureTemplatesLoaded();
    await templateStore.loadBindings(resolvedChannelId.value);
    await templateStore.migrateLocalTemplatesIfNeeded(
      resolvedChannelId.value,
      channelCards.value.map(item => ({ id: item.id, name: item.name, sheetType: item.sheetType || '' })),
    );
    await templateStore.loadBindings(resolvedChannelId.value);
  }
}, { immediate: true });

watch(resolvedChannelId, async (newId) => {
  if (props.visible && newId && !characterApiDisabled.value) {
    await cardStore.loadCards(newId);
    await templateStore.ensureTemplatesLoaded();
    await templateStore.loadBindings(newId);
    await templateStore.migrateLocalTemplatesIfNeeded(
      newId,
      channelCards.value.map(item => ({ id: item.id, name: item.name, sheetType: item.sheetType || '' })),
    );
    await templateStore.loadBindings(newId);
  }
});

watch(
  [() => props.visible, channelCards],
  async ([visible, cards]) => {
    const channelId = resolvedChannelId.value;
    if (!visible || !channelId || !cards.length || characterApiDisabled.value) return;
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

const templateFilterSheetType = ref('');
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
  const filter = templateFilterSheetType.value.trim().toLowerCase();
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

// Edit card modal
const editModalVisible = ref(false);
const editingCard = ref<CharacterCard | null>(null);
const editCardName = ref('');
const editCardSheetTypePreset = ref('coc7');
const editCardSheetTypeCustom = ref('');
const editCardOriginalName = ref('');
const editCardOriginalSheetType = ref('');
const editCardAttrsJson = ref('');
const saving = ref(false);
const pendingRestore = ref<{
  channelId: string;
  cardId?: string;
  cardName?: string;
  cardType?: string;
  attrs?: Record<string, any>;
} | null>(null);

const setEditSheetType = (value: string) => {
  const normalized = (value || '').trim();
  let lower = normalized.toLowerCase();
  if (lower === 'coc') {
    lower = 'coc7';
  } else if (lower === 'dnd' || lower === 'dnd5') {
    lower = 'dnd5e';
  }
  if (lower === 'coc7' || lower === 'dnd5e') {
    editCardSheetTypePreset.value = lower;
    editCardSheetTypeCustom.value = '';
  } else if (normalized) {
    editCardSheetTypePreset.value = 'custom';
    editCardSheetTypeCustom.value = normalized;
  } else {
    editCardSheetTypePreset.value = 'coc7';
    editCardSheetTypeCustom.value = '';
  }
};

const syncEditOriginals = () => {
  editCardOriginalName.value = editCardName.value;
  editCardOriginalSheetType.value = resolveSheetType(editCardSheetTypePreset.value, editCardSheetTypeCustom.value);
};

const rememberActiveCard = async (channelId: string) => {
  if (cardStore.isBotCharacterDisabled(channelId)) {
    pendingRestore.value = null;
    return;
  }
  await cardStore.getActiveCard(channelId);
  const active = cardStore.activeCards[channelId];
  const activeId = cardStore.getActiveCardId(channelId);
  if (activeId || active?.name) {
    pendingRestore.value = {
      channelId,
      cardId: activeId || undefined,
      cardName: active?.name || '',
      cardType: active?.type || '',
      attrs: active?.attrs || {},
    };
  } else {
    pendingRestore.value = null;
  }
};

const restoreActiveCard = async () => {
  const pending = pendingRestore.value;
  if (!pending) return;
  pendingRestore.value = null;
  if (cardStore.isBotCharacterDisabled(pending.channelId)) {
    return;
  }
  try {
    if (pending.cardId) {
      await cardStore.tagCard(pending.channelId, undefined, pending.cardId);
      return;
    }
    await cardStore.tagCard(pending.channelId);
    if (pending.cardName || pending.attrs) {
      await cardStore.updateCard(pending.channelId, pending.cardName || '', pending.attrs || {});
    }
  } catch (e) {
    console.warn('Failed to restore active character card', e);
  }
};

const openEditModal = async (card: CharacterCard) => {
  if (!ensureCharacterApiEnabled()) return;
  editingCard.value = card;
  editCardName.value = card.name;
  setEditSheetType(card.sheetType || 'coc7');
  editCardAttrsJson.value = JSON.stringify(card.attrs || {}, null, 2);
  editModalVisible.value = true;
  syncEditOriginals();
  if (!resolvedChannelId.value) {
    pendingRestore.value = null;
    return;
  }
  try {
    await rememberActiveCard(resolvedChannelId.value);
    if (pendingRestore.value?.cardId === card.id) {
      pendingRestore.value = null;
    } else {
      await cardStore.tagCard(resolvedChannelId.value, card.name, card.id);
    }
    await cardStore.getActiveCard(resolvedChannelId.value);
    const active = cardStore.activeCards[resolvedChannelId.value];
    if (active) {
      editCardName.value = active.name || editCardName.value;
      setEditSheetType(active.type || resolveSheetType(editCardSheetTypePreset.value, editCardSheetTypeCustom.value));
      editCardAttrsJson.value = JSON.stringify(active.attrs || {}, null, 2);
      syncEditOriginals();
    }
  } catch (e) {
    console.warn('Failed to load character card attrs', e);
  }
};

watch(editModalVisible, async (val, oldVal) => {
  if (!val && oldVal) {
    await restoreActiveCard();
  }
});

const handleSaveCard = async () => {
  if (!ensureCharacterApiEnabled()) return;
  if (!editingCard.value) return;
  if (!resolvedChannelId.value) {
    message.warning('请先选择频道');
    return;
  }
  if (!editCardName.value.trim()) {
    message.warning('请输入角色名称');
    return;
  }
  let attrs: Record<string, any> = {};
  try {
    attrs = JSON.parse(editCardAttrsJson.value || '{}');
  } catch {
    message.error('属性 JSON 格式错误');
    return;
  }
  saving.value = true;
  try {
    const nextName = editCardOriginalName.value || editCardName.value.trim();
    await cardStore.updateCard(resolvedChannelId.value, nextName, attrs);
    await cardStore.loadCards(resolvedChannelId.value);
    message.success('保存成功');
    editModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '保存失败');
  } finally {
    saving.value = false;
  }
};

const handleDeleteCard = async (card: CharacterCard) => {
  if (!ensureCharacterApiEnabled()) return;
  try {
    await cardStore.deleteCard(card.id);
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

const resolveCardAvatarUrl = (cardId: string) => {
  const bound = getBoundIdentities(cardId);
  const identity = bound.find(item => item.avatarAttachmentId) || bound[0];
  if (!identity?.avatarAttachmentId) return '';
  return resolveAttachmentUrl(identity.avatarAttachmentId) || identity.avatarAttachmentId;
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

const formatAttrs = (attrs: Record<string, any> | undefined) => {
  if (!attrs || Object.keys(attrs).length === 0) return '暂无属性';
  return Object.entries(attrs).map(([k, v]) => `${k}: ${v}`).join(', ');
};

const openPreview = async (card: CharacterCard) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  if (characterApiDisabled.value) {
    const avatarUrl = resolveCardAvatarUrl(card.id);
    sheetStore.openSheet(card, channelId, {
      name: card.name,
      type: card.sheetType,
      attrs: card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    });
    if (isMobile.value) {
      handleClose();
    }
    return;
  }
  try {
    let cardData = cardStore.activeCards[channelId];
    if (!cardData || cardData.name !== card.name) {
      await cardStore.getActiveCard(channelId);
      cardData = cardStore.activeCards[channelId];
    }
    await templateStore.ensureTemplatesLoaded();
    await templateStore.ensureBindingsLoaded(channelId);
    const resolvedSheetType = (cardData?.type || card.sheetType || '').trim();
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
    const avatarUrl = resolveCardAvatarUrl(card.id);
    sheetStore.openSheet(card, channelId, {
      name: cardData?.name || card.name,
      type: cardData?.type || card.sheetType,
      attrs: cardData?.attrs || card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    }, {
      templateMode: binding?.mode,
      templateId: binding?.templateId || undefined,
      templateText: binding?.mode === 'detached' ? binding.templateSnapshot : undefined,
    });
    if (isMobile.value) {
      handleClose();
    }
  } catch (e: any) {
    console.warn('Failed to open character preview', e);
    const avatarUrl = resolveCardAvatarUrl(card.id);
    sheetStore.openSheet(card, channelId, {
      name: card.name,
      type: card.sheetType,
      attrs: card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    });
    if (isMobile.value) {
      handleClose();
    }
  }
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
        {{ characterApiUnavailableText }}
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
          v-for="card in filteredChannelCards"
          :key="card.id"
          size="small"
          class="character-card-item"
        >
          <template #header>
            <span class="card-name">{{ card.name }}</span>
            <n-tag size="small" :bordered="false">{{ card.sheetType || 'custom' }}</n-tag>
          </template>
          <template #header-extra>
            <n-button text size="small" title="预览" @click="openPreview(card)">
              <template #icon><n-icon :component="Eye" /></template>
            </n-button>
            <n-button text size="small" :disabled="characterApiDisabled" @click="openEditModal(card)">
              <template #icon><n-icon :component="Edit" /></template>
            </n-button>
            <n-button text size="small" :disabled="characterApiDisabled" @click="openBindModal(card)">
              <template #icon><n-icon :component="Link" /></template>
            </n-button>
            <n-popconfirm @positive-click="handleDeleteCard(card)">
              <template #trigger>
                <n-button text size="small" type="error" :disabled="characterApiDisabled">
                  <template #icon><n-icon :component="Trash" /></template>
                </n-button>
              </template>
              删除前将从所有群解绑此人物卡，确定删除？
            </n-popconfirm>
          </template>
          <div class="card-attrs">{{ formatAttrs(card.attrs) }}</div>
          <div v-if="getBoundIdentities(card.id).length > 0" class="card-bindings">
            <span class="bindings-label">已绑定：</span>
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
        </n-card>
      </div>
    </n-drawer-content>
  </n-drawer>

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
          :options="[{ label: '全部规制', value: '' }, ...sheetTypeOptions.filter(opt => opt.value !== 'custom')]"
          placeholder="筛选规制"
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

  <!-- Edit Modal -->
  <n-modal
    v-model:show="editModalVisible"
    preset="dialog"
    :show-icon="false"
    title="编辑人物卡"
    :positive-text="saving ? '保存中…' : '保存'"
    :positive-button-props="{ loading: saving, disabled: characterApiDisabled }"
    negative-text="取消"
    @positive-click="handleSaveCard"
  >
    <n-form label-width="80">
      <n-form-item label="角色名称">
        <n-input v-model:value="editCardName" maxlength="32" disabled />
      </n-form-item>
      <n-form-item label="卡片类型">
        <n-select v-model:value="editCardSheetTypePreset" :options="sheetTypeOptions" disabled />
        <n-input
          v-if="editCardSheetTypePreset === 'custom'"
          v-model:value="editCardSheetTypeCustom"
          placeholder="输入自定义规制类型"
          class="sheet-type-custom-input"
          disabled
        />
      </n-form-item>
      <n-form-item label="属性(JSON)">
        <n-input
          v-model:value="editCardAttrsJson"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 10 }"
          placeholder='例如: {"hp": 10, "hpmax": 10, "san": 50}'
          :disabled="characterApiDisabled"
        />
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
  margin-bottom: 0.75rem;
  padding: 0.65rem 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(245, 158, 11, 0.35);
  background: rgba(245, 158, 11, 0.12);
  color: var(--sc-text-primary);
  font-size: 0.82rem;
  line-height: 1.4;
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
  .card-name {
    font-weight: 500;
    margin-right: 0.5rem;
  }
  .card-attrs {
    color: var(--sc-text-secondary);
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
  }
  .card-bindings {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.25rem;
    .bindings-label {
      font-size: 0.8rem;
      color: var(--sc-text-tertiary);
    }
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
