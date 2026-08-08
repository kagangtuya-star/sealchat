<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue';
import { NDrawer, NDrawerContent, NButton, NIcon, NEmpty, NCard, NInput, NForm, NFormItem, NModal, NPopconfirm, NTag, NSwitch, NSelect, NCheckbox, NRadioGroup, NRadioButton, NCollapseTransition, NColorPicker, useMessage } from 'naive-ui';
import { Plus, Trash, Edit, Link, Eye, Upload, Download, X, Refresh, ChevronDown, ChevronRight, Settings, GripVertical } from '@vicons/tabler';
import { characterApiUnsupportedText, useCharacterCardStore, type CharacterCard, type OnlineCharacterCardItem } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useCharacterCardTemplateStore, type CharacterCardTemplate } from '@/stores/characterCardTemplate';
import { useCharacterCardAvatarStore } from '@/stores/characterCardAvatar';
import { useChatStore } from '@/stores/chat';
import { useDisplayStore } from '@/stores/display';
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';
import {
  useChannelCharacterSnapshotStore,
  type CharacterSnapshotNumericSource,
  type TheaterCharacterOverlayTemplate,
} from '@/stores/channelCharacterSnapshot';
import {
  CHARACTER_SNAPSHOT_BADGE_TEMPLATE_PRESETS,
  CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS,
  getCharacterSnapshotTemplatePreset,
} from '@/utils/characterSnapshotTemplatePresets';
import {
  filterAliveNarratorIdentityIds,
  resolveCharacterCardNarratorCountBadge,
} from '@/utils/characterCardNarratorSettings';
import { DEFAULT_CARD_TEMPLATE, getWorldCardTemplate, resolveTemplateValue, setWorldCardTemplate } from '@/utils/characterCardTemplate';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import AvatarVue from '@/components/avatar.vue';
import AvatarEditor from '@/components/AvatarEditor.vue';
import type { ChannelIdentity } from '@/types';
import type { MessageVisibilityScope } from '@/stores/displayAvatarVisibility';

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
const userStore = useUserStore();
const utilsStore = useUtilsStore();
const snapshotStore = useChannelCharacterSnapshotStore();

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

const badgeSettingsExpanded = computed({
  get: () => displayStore.settings.characterCardBadgeSettingsExpanded,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardBadgeSettingsExpanded: value });
  },
});

const badgeSettingsToggleIcon = computed(() => (
  badgeSettingsExpanded.value ? ChevronDown : ChevronRight
));

const toggleBadgeSettingsExpanded = () => {
  badgeSettingsExpanded.value = !badgeSettingsExpanded.value;
};

const onlineCharacterCardsExpanded = ref(true);

const onlineCharacterCardsToggleIcon = computed(() => (
  onlineCharacterCardsExpanded.value ? ChevronDown : ChevronRight
));

const toggleOnlineCharacterCardsExpanded = () => {
  onlineCharacterCardsExpanded.value = !onlineCharacterCardsExpanded.value;
};

const badgeAutoContrastEnabled = computed({
  get: () => displayStore.settings.characterCardBadgeAutoContrastEnabled,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardBadgeAutoContrastEnabled: value });
  },
});

const badgeVisibilityScopeOptions: Array<{ label: string; value: MessageVisibilityScope }> = [
  { label: '全部显示', value: 'all' },
  { label: '场内显示', value: 'ic' },
  { label: '场外显示', value: 'ooc' },
];

const badgeVisibilityScope = computed({
  get: () => displayStore.settings.characterCardBadgeVisibilityScope,
  set: (value: MessageVisibilityScope) => {
    displayStore.updateSettings({ characterCardBadgeVisibilityScope: value });
  },
});

const onlineCharacterCardsEnabled = computed({
  get: () => displayStore.settings.onlineCharacterCardsEnabled,
  set: (value: boolean) => {
    displayStore.updateSettings({ onlineCharacterCardsEnabled: value });
  },
});

const characterCardSnapshotUploadEnabled = computed({
  get: () => displayStore.settings.characterCardSnapshotUploadEnabled,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardSnapshotUploadEnabled: value });
  },
});

const onlineCharacterCards = computed(() => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return [];
  return snapshotStore.getChannelItems(channelId)
    .filter(item => item.userId !== userStore.info?.id && item.data.card?.name)
    .map<OnlineCharacterCardItem>(item => ({
      userId: item.userId,
      userNick: item.data.identity.displayName,
      identityId: item.identityId,
      identityName: item.data.identity.displayName,
      identityColor: item.data.identity.color,
      identityAvatar: item.data.identity.avatarAttachmentId,
      card: item.data.card!,
      updatedAt: Math.floor((item.sourceUpdatedAt || item.lastSeenAt || 0) > 1_000_000_000_000
        ? (item.sourceUpdatedAt || item.lastSeenAt || 0) / 1000
        : (item.sourceUpdatedAt || item.lastSeenAt || 0)),
    }))
    .sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0));
});

const onlineCharacterCardsLoading = computed(() => (
  !!snapshotStore.loadingByChannel[resolvedChannelId.value]
));

const lastOnlineCharacterCardsRefreshAt = ref(0);

const refreshOnlineCharacterCards = async (manual = false) => {
  const channelId = resolvedChannelId.value;
  if (!channelId || !onlineCharacterCardsEnabled.value) return;
  const now = Date.now();
  if (manual && now - lastOnlineCharacterCardsRefreshAt.value < 3000) return;
  lastOnlineCharacterCardsRefreshAt.value = now;
  await cardStore.requestOnlineCardSnapshot(channelId, { requestPeers: true });
};

const syncOnlineCharacterCardsRefresh = () => {
  if (!props.visible || !resolvedChannelId.value || !onlineCharacterCardsEnabled.value) return;
  void refreshOnlineCharacterCards(false);
};

const formatOnlineCardUpdatedAt = (updatedAt?: number) => {
  if (!updatedAt) return '最后更新未知';
  const time = new Date(updatedAt * 1000);
  if (Number.isNaN(time.getTime())) return '最后更新未知';
  return `最后更新 ${time.toLocaleTimeString()}`;
};

const onlinePreviewCard = (item: OnlineCharacterCardItem): CharacterCard => ({
  id: `snapshot:${resolvedChannelId.value}:${item.identityId}`,
  name: item.card.name,
  sheetType: item.card.sheetType,
  attrs: item.card.attrs,
});

const openOnlineCharacterCardPreview = (item: OnlineCharacterCardItem) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  const windowId = sheetStore.openSheet(onlinePreviewCard(item), channelId, {
    name: item.card.name,
    type: item.card.sheetType,
    attrs: item.card.attrs || {},
    avatarUrl: item.identityAvatar || undefined,
  }, {
    templateText: item.card.templateText || undefined,
    readOnly: true,
    worldId: currentWorldId.value || undefined,
    placement: 'right',
  });
  if (isMobile.value) {
    handleClose();
  }
  sheetStore.setMode(windowId, 'view');
};

const narratorSettingsVisible = ref(false);
const narratorIdentityIdsDraft = ref<string[]>([]);

const aliveIdentityIds = computed(() => identities.value.map((item) => item.id));

const resolveActiveNarratorIdentityIds = (identityIds?: string[]) => {
  const channelId = resolvedChannelId.value;
  const stored = identityIds ?? cardStore.getNarratorIdentityIds(channelId);
  const loadedIdentities = channelId ? chatStore.channelIdentities[channelId] : undefined;
  // Identities not loaded yet: keep stored list to avoid false empty / accidental prune.
  if (loadedIdentities === undefined) {
    return Array.isArray(stored) ? stored : [];
  }
  return filterAliveNarratorIdentityIds(stored, loadedIdentities.map((item) => item.id));
};

const pruneStaleNarratorIdentities = () => {
  const channelId = resolvedChannelId.value;
  if (!channelId || chatStore.channelIdentities[channelId] === undefined) {
    return;
  }
  const stored = cardStore.getNarratorIdentityIds(channelId);
  const active = resolveActiveNarratorIdentityIds(stored);
  if (active.length === stored.length && active.every((id, index) => id === stored[index])) {
    return;
  }
  cardStore.setNarratorIdentityIds(channelId, active);
};

const syncNarratorIdentityIdsDraft = () => {
  pruneStaleNarratorIdentities();
  narratorIdentityIdsDraft.value = resolveActiveNarratorIdentityIds();
};

const narratorCountBadge = computed(() => resolveCharacterCardNarratorCountBadge(
  resolveActiveNarratorIdentityIds(),
));

const openNarratorSettings = () => {
  syncNarratorIdentityIdsDraft();
  narratorSettingsVisible.value = true;
};

const handleNarratorIdentityChecked = (identityId: string, checked: boolean) => {
  if (!identityId) {
    return;
  }
  narratorIdentityIdsDraft.value = checked
    ? Array.from(new Set([...narratorIdentityIdsDraft.value, identityId]))
    : narratorIdentityIdsDraft.value.filter(id => id !== identityId);
};

const handleNarratorSettingsSave = async () => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return false;
  }
  const nextIds = filterAliveNarratorIdentityIds(
    narratorIdentityIdsDraft.value,
    aliveIdentityIds.value,
  );
  narratorIdentityIdsDraft.value = nextIds;
  cardStore.setNarratorIdentityIds(channelId, nextIds);
  for (const identityId of nextIds) {
    await cardStore.broadcastActiveBadge(channelId, identityId, 'clear');
  }
  narratorSettingsVisible.value = false;
  return true;
};

const autoSyncBotNicknameEnabled = computed({
  get: () => displayStore.settings.characterCardAutoSyncBotNickname,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardAutoSyncBotNickname: value });
  },
});

const badgeTemplate = ref('');
const channelSnapshotTemplateManageAllowed = ref(false);
const theaterOverlayTemplateJson = ref(JSON.stringify({ version: 1, preferredColumns: 2, items: [] }, null, 2));
const theaterOverlaySettingsExpanded = ref(false);
const personalBadgeTemplateMode = ref<'inherit' | 'custom' | 'off'>('inherit');
const personalBadgeTemplate = ref('');
const personalOverlayTemplateMode = ref<'inherit' | 'custom' | 'off'>('inherit');
const personalOverlayTemplateJson = ref(JSON.stringify({ version: 1, preferredColumns: 2, items: [] }, null, 2));
const snapshotTemplateSaving = ref(false);
const overlayTemplateEditorVisible = ref(false);
const overlayTemplateEditorTarget = ref<'channel' | 'personal'>('channel');
const draggingOverlayItemId = ref('');
const overlayTemplateImportInput = ref<HTMLInputElement | null>(null);
const overlayTemplateEditorPreferredColumns = ref(2);
let nextOverlayItemSerial = 1;

interface OverlayEditorSource {
  value: string;
  kind: 'path' | 'literal';
}

interface OverlayEditorItem {
  id: string;
  name: string;
  current: OverlayEditorSource;
  min: OverlayEditorSource;
  max: OverlayEditorSource;
  barColor: string;
  textColor: string;
}

const overlayTemplatePresets = CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS;
const badgeTemplatePresets = CHARACTER_SNAPSHOT_BADGE_TEMPLATE_PRESETS;

const overlayTemplateEditorItems = ref<OverlayEditorItem[]>([]);
const templateModeOptions = [
  { label: '继承频道', value: 'inherit' },
  { label: '个人模板', value: 'custom' },
  { label: '关闭', value: 'off' },
];
const currentWorldId = computed(() => chatStore.currentWorldId || '');
const theaterOverlaySettingsToggleIcon = computed(() => (
  theaterOverlaySettingsExpanded.value ? ChevronDown : ChevronRight
));
const activeCharacterCardAttrs = computed<Record<string, any>>(() => {
  const channelId = resolvedChannelId.value;
  return channelId ? cardStore.activeCards[channelId]?.attrs || {} : {};
});
const canSyncBadgeTemplate = computed(() => {
  const worldId = currentWorldId.value;
  if (!worldId) return false;
  const detail = chatStore.worldDetailMap[worldId];
  const role = detail?.memberRole;
  return role === 'owner' || role === 'admin' || channelSnapshotTemplateManageAllowed.value;
});

const toggleTheaterOverlaySettingsExpanded = () => {
  theaterOverlaySettingsExpanded.value = !theaterOverlaySettingsExpanded.value;
};

const sourceToEditorSource = (source?: CharacterSnapshotNumericSource): OverlayEditorSource => {
  if (!source || typeof source !== 'object') return { value: '', kind: 'path' };
  return 'value' in source
    ? { value: String(source.value), kind: 'literal' }
    : { value: source.path || '', kind: 'path' };
};

const editorSourceToTemplateSource = (source: OverlayEditorSource): CharacterSnapshotNumericSource | undefined => {
  const value = source.value.trim();
  if (!value) return undefined;
  if (source.kind === 'literal') {
    const numericValue = Number(value);
    return Number.isFinite(numericValue) ? { value: numericValue } : undefined;
  }
  return { path: value };
};

const createOverlayEditorItem = (): OverlayEditorItem => ({
  id: `stat_${Date.now()}_${nextOverlayItemSerial++}`,
  name: '',
  current: { value: '', kind: 'path' },
  min: { value: '', kind: 'path' },
  max: { value: '', kind: 'path' },
  barColor: '#5b8ff9',
  textColor: '#f8fafc',
});

const parseOverlayTemplateForEditor = (raw: string): { items: OverlayEditorItem[]; preferredColumns: number } | null => {
  try {
    const value = JSON.parse(raw) as TheaterCharacterOverlayTemplate;
    if (value?.version !== 1 || !Array.isArray(value.items)) return null;
    const preferredColumns = Number(value.preferredColumns || 2);
    return {
      preferredColumns: Number.isFinite(preferredColumns)
        ? Math.min(4, Math.max(1, preferredColumns))
        : 2,
      items: value.items
        .filter(item => item && typeof item === 'object')
        .map((item, index) => ({
          id: String(item.id || `stat_${Date.now()}_${index + 1}`),
          name: String(item.name || ''),
          current: sourceToEditorSource(item.current),
          min: sourceToEditorSource(item.min),
          max: sourceToEditorSource(item.max),
          barColor: item.barColor || '#5b8ff9',
          textColor: item.textColor || '#f8fafc',
        })),
    };
  } catch {
    return null;
  }
};

const openOverlayTemplateEditor = (target: 'channel' | 'personal') => {
  overlayTemplateEditorTarget.value = target;
  const raw = target === 'channel' ? theaterOverlayTemplateJson.value : personalOverlayTemplateJson.value;
  const draft = parseOverlayTemplateForEditor(raw);
  overlayTemplateEditorItems.value = draft?.items || [];
  overlayTemplateEditorPreferredColumns.value = draft?.preferredColumns || 2;
  overlayTemplateEditorVisible.value = true;
};

const exportOverlayTemplate = () => {
  const content = serializeOverlayTemplateEditor();
  const blob = new Blob([content], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `sealchat-theater-overlay-${overlayTemplateEditorTarget.value}.json`;
  link.click();
  URL.revokeObjectURL(url);
};

const triggerOverlayTemplateImport = () => {
  if (!overlayTemplateImportInput.value) return;
  overlayTemplateImportInput.value.value = '';
  overlayTemplateImportInput.value.click();
};

const importOverlayTemplate = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  if (file.size > 1024 * 1024) {
    message.error('模板文件不能超过 1 MB');
    return;
  }
  try {
    const draft = parseOverlayTemplateForEditor(await file.text());
    if (!draft) throw new Error('模板格式无效');
    overlayTemplateEditorItems.value = draft.items;
    overlayTemplateEditorPreferredColumns.value = draft.preferredColumns;
    message.success('模板已导入，保存后生效');
  } catch (error) {
    message.error(error instanceof Error ? error.message : '模板导入失败');
  }
};

const addOverlayTemplateEditorItem = () => {
  overlayTemplateEditorItems.value.push(createOverlayEditorItem());
};

const removeOverlayTemplateEditorItem = (id: string) => {
  overlayTemplateEditorItems.value = overlayTemplateEditorItems.value.filter(item => item.id !== id);
};

const beginOverlayTemplateItemDrag = (id: string, event: DragEvent) => {
  draggingOverlayItemId.value = id;
  event.dataTransfer?.setData('text/plain', id);
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
};

const reorderOverlayTemplateEditorItem = (targetId: string) => {
  const sourceId = draggingOverlayItemId.value;
  draggingOverlayItemId.value = '';
  if (!sourceId || sourceId === targetId) return;
  const items = [...overlayTemplateEditorItems.value];
  const sourceIndex = items.findIndex(item => item.id === sourceId);
  const targetIndex = items.findIndex(item => item.id === targetId);
  if (sourceIndex < 0 || targetIndex < 0) return;
  const [source] = items.splice(sourceIndex, 1);
  items.splice(targetIndex, 0, source);
  overlayTemplateEditorItems.value = items;
};

const resolveOverlayEditorSource = (source: OverlayEditorSource): number | null => {
  const raw = source.kind === 'literal'
    ? source.value
    : resolveTemplateValue(activeCharacterCardAttrs.value, source.value);
  if (raw === null || raw === undefined || raw === '') return null;
  const numericValue = typeof raw === 'number' ? raw : Number(String(raw).trim());
  return Number.isFinite(numericValue) ? numericValue : null;
};

const formatOverlayEditorCurrentValue = (source: OverlayEditorSource) => {
  const value = resolveOverlayEditorSource(source);
  return value === null ? '未找到' : String(value);
};

const overlayTemplatePreviewItems = computed(() => overlayTemplateEditorItems.value
  .map(item => {
    const current = resolveOverlayEditorSource(item.current);
    const hasMax = item.max.value.trim() !== '';
    const max = hasMax ? resolveOverlayEditorSource(item.max) : null;
    const min = resolveOverlayEditorSource(item.min);
    if (current === null) return null;
    if (max === null) return { ...item, current, max: null, percent: 100 };
    const rangeMin = min ?? 0;
    if (max <= rangeMin) return { ...item, current, max: null, percent: 100 };
    const percent = Math.min(100, Math.max(0, ((current - rangeMin) / (max - rangeMin)) * 100));
    return { ...item, current, max, percent };
  })
  .filter((item): item is OverlayEditorItem & { current: number; max: number | null; percent: number } => !!item));

const serializeOverlayTemplateEditor = () => JSON.stringify({
  version: 1,
  preferredColumns: overlayTemplateEditorPreferredColumns.value,
  items: overlayTemplateEditorItems.value
    .map(item => ({
      id: item.id,
      name: item.name.trim(),
      current: editorSourceToTemplateSource(item.current),
      min: editorSourceToTemplateSource(item.min),
      max: editorSourceToTemplateSource(item.max),
      barColor: item.barColor,
      textColor: item.textColor,
    }))
    .filter(item => item.name && item.current),
}, null, 2);

const syncBadgeTemplate = () => {
  const worldId = currentWorldId.value;
  if (!worldId) {
    badgeTemplate.value = DEFAULT_CARD_TEMPLATE;
    return;
  }
  const stored = displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId];
  badgeTemplate.value = stored ?? getWorldCardTemplate(worldId);
};

const formatTemplateJson = (value?: string) => {
  try {
    return JSON.stringify(JSON.parse(String(value || '')), null, 2);
  } catch {
    return String(value || '');
  }
};

const syncSnapshotTemplateDrafts = async () => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return;
  try {
    channelSnapshotTemplateManageAllowed.value = await chatStore.hasChannelPermission(
      channelId,
      'func_channel_manage_info',
      userStore.info.id,
    );
  } catch {
    channelSnapshotTemplateManageAllowed.value = false;
  }
  await snapshotStore.initializeChannel(channelId);
  const settings = snapshotStore.settingsByChannel[channelId];
  const preference = snapshotStore.preferenceByChannel[channelId];
  if (settings) {
    badgeTemplate.value = settings.badgeTemplate || badgeTemplate.value || DEFAULT_CARD_TEMPLATE;
    theaterOverlayTemplateJson.value = formatTemplateJson(settings.theaterOverlayTemplateJson);
  }
  if (preference) {
    personalBadgeTemplateMode.value = preference.badgeTemplateMode || 'inherit';
    personalBadgeTemplate.value = preference.badgeTemplate || '';
    personalOverlayTemplateMode.value = preference.theaterOverlayTemplateMode || 'inherit';
    personalOverlayTemplateJson.value = formatTemplateJson(preference.theaterOverlayTemplateJson);
  }
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
  const channelId = resolvedChannelId.value;
  if (!channelId) return;
  const normalized = badgeTemplate.value.trim() || DEFAULT_CARD_TEMPLATE;
  badgeTemplate.value = normalized;
  persistBadgeTemplate();
  snapshotTemplateSaving.value = true;
  try {
    await snapshotStore.updateSettings(channelId, {
      badgeTemplate: normalized,
      theaterOverlayTemplateJson: theaterOverlayTemplateJson.value,
    });
    await snapshotStore.refreshChannel(channelId);
    message.success('频道人物卡模板已同步');
  } catch (e: any) {
    message.error(e?.response?.err || e?.message || '模板同步失败');
  } finally {
    snapshotTemplateSaving.value = false;
  }
};

const savePersonalSnapshotTemplates = async () => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return;
  snapshotTemplateSaving.value = true;
  try {
    await snapshotStore.updatePreference(channelId, {
      badgeTemplateMode: personalBadgeTemplateMode.value,
      badgeTemplate: personalBadgeTemplate.value,
      theaterOverlayTemplateMode: personalOverlayTemplateMode.value,
      theaterOverlayTemplateJson: personalOverlayTemplateJson.value,
    });
    await snapshotStore.syncLocalSnapshot(channelId, true);
    message.success('个人人物卡模板已保存');
  } catch (e: any) {
    message.error(e?.response?.err || e?.message || '个人模板保存失败');
  } finally {
    snapshotTemplateSaving.value = false;
  }
};

const saveOverlayTemplateEditor = async () => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return false;
  const templateJson = serializeOverlayTemplateEditor();
  snapshotTemplateSaving.value = true;
  try {
    if (overlayTemplateEditorTarget.value === 'channel') {
      const normalizedBadgeTemplate = badgeTemplate.value.trim() || DEFAULT_CARD_TEMPLATE;
      badgeTemplate.value = normalizedBadgeTemplate;
      persistBadgeTemplate();
      await snapshotStore.updateSettings(channelId, {
        badgeTemplate: normalizedBadgeTemplate,
        theaterOverlayTemplateJson: templateJson,
      });
      theaterOverlayTemplateJson.value = templateJson;
      await snapshotStore.refreshChannel(channelId);
      message.success('小剧场数据浮层模板已保存');
    } else {
      personalOverlayTemplateJson.value = templateJson;
      await snapshotStore.updatePreference(channelId, {
        badgeTemplateMode: personalBadgeTemplateMode.value,
        badgeTemplate: personalBadgeTemplate.value,
        theaterOverlayTemplateMode: personalOverlayTemplateMode.value,
        theaterOverlayTemplateJson: templateJson,
      });
      await snapshotStore.syncLocalSnapshot(channelId, true);
      message.success('个人小剧场浮层模板已保存');
    }
    overlayTemplateEditorVisible.value = false;
    return true;
  } catch (e: any) {
    message.error(e?.response?.err || e?.message || '模板保存失败');
    return false;
  } finally {
    snapshotTemplateSaving.value = false;
  }
};

const applySnapshotTemplatePreset = async (target: 'channel' | 'personal', preset: keyof typeof overlayTemplatePresets) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return;
  const templateJson = JSON.stringify(overlayTemplatePresets[preset], null, 2);
  const badgeTemplatePreset = badgeTemplatePresets[preset];
  snapshotTemplateSaving.value = true;
  try {
    if (target === 'channel') {
      badgeTemplate.value = badgeTemplatePreset;
      persistBadgeTemplate();
      await snapshotStore.updateSettings(channelId, {
        badgeTemplate: badgeTemplatePreset,
        theaterOverlayTemplateJson: templateJson,
      });
      theaterOverlayTemplateJson.value = templateJson;
      await snapshotStore.refreshChannel(channelId);
    } else {
      personalBadgeTemplateMode.value = 'custom';
      personalBadgeTemplate.value = badgeTemplatePreset;
      personalOverlayTemplateMode.value = 'custom';
      personalOverlayTemplateJson.value = templateJson;
      await snapshotStore.updatePreference(channelId, {
        badgeTemplateMode: 'custom',
        badgeTemplate: badgeTemplatePreset,
        theaterOverlayTemplateMode: 'custom',
        theaterOverlayTemplateJson: templateJson,
      });
      await snapshotStore.syncLocalSnapshot(channelId, true);
    }
  } finally {
    snapshotTemplateSaving.value = false;
  }
};

const applyDefaultSnapshotTemplatePreset = async (preset: keyof typeof overlayTemplatePresets) => {
  try {
    await applySnapshotTemplatePreset(canSyncBadgeTemplate.value ? 'channel' : 'personal', preset);
    message.success(`已启用 ${preset === 'coc' ? 'COC' : '忍神'} 默认人物卡模板`);
  } catch (e: any) {
    message.error(e?.response?.err || e?.message || '默认模板启用失败');
  }
};

const loadPanelData = async (channelId: string) => {
  await snapshotStore.initializeChannel(channelId);
  await syncSnapshotTemplateDrafts();
  await cardStore.loadCards(channelId);
  await templateStore.ensureTemplatesLoaded({ worldId: currentWorldId.value || undefined });
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

const ensureCharacterApiForPreview = async (channelId: string) => {
  if (!characterApiDisabled.value) {
    return true;
  }
  try {
    const result = await cardStore.revalidateCharacterApi(channelId);
    if (result.ok) {
      return true;
    }
    message.error(`${result.error || '人物卡 API 自动连接失败'}，请在人物卡面板手动重试`);
  } catch (e: any) {
    const error = e?.response?.data?.error || e?.message || '人物卡 API 自动连接失败';
    message.error(`${error}，请在人物卡面板手动重试`);
  }
  return false;
};

watch(() => props.visible, async (val) => {
  if (!val) {
    return;
  }
  pruneStaleNarratorIdentities();
  if (resolvedChannelId.value && !characterApiDisabled.value) {
    await loadPanelData(resolvedChannelId.value);
  }
}, { immediate: true });

watch(resolvedChannelId, async (newId) => {
  syncNarratorIdentityIdsDraft();
  if (props.visible && newId && !characterApiDisabled.value) {
    await loadPanelData(newId);
  }
});

watch(aliveIdentityIds, () => {
  pruneStaleNarratorIdentities();
  if (narratorSettingsVisible.value) {
    narratorIdentityIdsDraft.value = resolveActiveNarratorIdentityIds();
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
      void syncSnapshotTemplateDrafts();
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

watch(onlineCharacterCardsEnabled, (enabled, prevEnabled) => {
  const channelId = resolvedChannelId.value;
  if (!channelId || prevEnabled === undefined || enabled === prevEnabled) {
    return;
  }
  if (!enabled) {
    void cardStore.clearOnlineActiveCard(channelId);
    return;
  }
  void cardStore.broadcastOnlineActiveCard(channelId);
  void refreshOnlineCharacterCards(false);
});

watch(characterCardSnapshotUploadEnabled, (enabled, prevEnabled) => {
  const channelId = resolvedChannelId.value;
  if (!channelId || prevEnabled === undefined || enabled === prevEnabled) {
    return;
  }
  void snapshotStore.syncLocalSnapshot(channelId, true).catch((error) => {
    console.warn('[CharacterCard] Failed to update snapshot upload setting', error);
  });
});

watch(
  [() => props.visible, resolvedChannelId, onlineCharacterCardsEnabled],
  () => {
    syncOnlineCharacterCardsRefresh();
  },
  { immediate: true },
);

watch(resolvedChannelId, (newId, oldId) => {
  if (oldId && oldId !== newId) {
    void cardStore.clearOnlineActiveCard(oldId);
  }
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
const templateDefaultBadgeTemplate = ref('');
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

const canManageWorldSharedTemplates = computed(() => canSyncBadgeTemplate.value && !!currentWorldId.value);

const canEditTemplateItem = (item: CharacterCardTemplate) => !item.readonly;

const canToggleWorldSharedTemplate = (item: CharacterCardTemplate) => {
  return canManageWorldSharedTemplates.value && !item.readonly;
};

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
  if (lower === 'shinobigami' || normalized === '忍神') {
    templateSheetTypePreset.value = 'shinobigami';
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
  await templateStore.ensureTemplatesLoaded({ worldId: currentWorldId.value || undefined });
  templateManagerVisible.value = true;
};

const openTemplateCreateModal = () => {
  if (!ensureCharacterApiEnabled()) return;
  templateEditingId.value = '';
  templateName.value = '';
  setTemplateSheetType('coc7');
  templateContent.value = sheetStore.getDefaultTemplate('coc7');
  templateDefaultBadgeTemplate.value = '';
  templateGlobalDefault.value = false;
  templateSheetDefault.value = false;
  templateModalVisible.value = true;
};

const openTemplateEditModal = (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  if (item.readonly) return;
  templateEditingId.value = item.id;
  templateName.value = item.name;
  setTemplateSheetType(item.sheetType || '');
  templateContent.value = item.content;
  templateDefaultBadgeTemplate.value = item.defaultBadgeTemplate || '';
  templateGlobalDefault.value = !!item.isGlobalDefault;
  templateSheetDefault.value = !!item.isSheetDefault;
  templateModalVisible.value = true;
};

const syncOpenWindowsForTemplate = async (templateId: string) => {
  const windowIds = sheetStore.activeWindowIds.filter((windowId) => {
    const win = sheetStore.windows[windowId];
    if (!win || win.readOnly || win.templateMode !== 'managed') return false;
    const binding = templateStore.getBinding(win.channelId, win.cardId);
    return win.templateId === templateId || binding?.templateId === templateId;
  });
  await Promise.all(windowIds.map(windowId => sheetStore.syncWindowTemplateFromCloud(windowId)));
};

const handleSaveTemplate = async () => {
  if (!ensureCharacterApiEnabled()) return;
  const name = templateName.value.trim();
  const sheetType = resolveTemplateSheetType();
  const content = templateContent.value.trim();
  const defaultBadgeTemplate = templateDefaultBadgeTemplate.value;
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
        defaultBadgeTemplate,
        isGlobalDefault: templateGlobalDefault.value,
        isSheetDefault: templateSheetDefault.value,
      });
      await syncOpenWindowsForTemplate(templateEditingId.value);
      message.success('模板已更新');
    } else {
      await templateStore.createTemplate({
        name,
        sheetType,
        content,
        defaultBadgeTemplate,
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
  if (item.readonly) return;
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
      defaultBadgeTemplate: item.defaultBadgeTemplate,
    });
    message.success('模板已复制');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '模板复制失败');
  }
};

const toggleTemplateWorldShared = async (item: CharacterCardTemplate) => {
  if (!ensureCharacterApiEnabled()) return;
  const worldId = currentWorldId.value;
  if (!worldId || item.readonly) return;
  try {
    if (item.isSharedToCurrentWorld) {
      await templateStore.unshareTemplateFromWorld(worldId, item.id);
      message.success('已取消世界共享');
    } else {
      await templateStore.shareTemplateToWorld(worldId, item.id);
      message.success('已设为世界共享');
    }
  } catch (e: any) {
    message.error(e?.response?.data?.message || e?.response?.data?.error || e?.message || '设置失败');
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
    message.warning('该模板缺少规则类型，无法设为规则默认');
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
const newCardTemplateId = ref('');
const newCardAvatarFile = ref<File | null>(null);
const newCardAttrs = ref<Record<string, any>>({});
const creating = ref(false);

const DETACHED_TEMPLATE_VALUE = '__detached__';

const sheetTypeOptions = [
  { label: 'COC7', value: 'coc7' },
  { label: 'DND5', value: 'dnd5e' },
  { label: '忍神', value: 'shinobigami' },
  { label: '自定义', value: 'custom' },
];

const resolveSheetType = (preset: string, custom: string) => {
  if (preset === 'custom') {
    return custom.trim();
  }
  return preset;
};

const setNewCardSheetType = (value: string) => {
  const normalized = (value || '').trim();
  const lower = normalized.toLowerCase();
  if (lower === 'coc7' || lower === 'coc') {
    newCardSheetTypePreset.value = 'coc7';
    newCardSheetTypeCustom.value = '';
    return;
  }
  if (lower === 'dnd5e' || lower === 'dnd5' || lower === 'dnd') {
    newCardSheetTypePreset.value = 'dnd5e';
    newCardSheetTypeCustom.value = '';
    return;
  }
  if (lower === 'shinobigami' || normalized === '忍神') {
    newCardSheetTypePreset.value = 'shinobigami';
    newCardSheetTypeCustom.value = '';
    return;
  }
  newCardSheetTypePreset.value = 'custom';
  newCardSheetTypeCustom.value = normalized;
};

const newCardSheetType = computed(() => resolveSheetType(
  newCardSheetTypePreset.value,
  newCardSheetTypeCustom.value,
));

const newCardTemplateOptions = computed(() => [
  { label: '自定义（默认样式）', value: DETACHED_TEMPLATE_VALUE },
  ...templateStore.getTemplatesBySheetType(newCardSheetType.value).map(item => ({
    label: item.access === 'world_shared'
      ? `${item.name}${item.sharedByNickname ? ` [世界共享:${item.sharedByNickname}]` : ' [世界共享]'}`
      : `${item.name}${item.sheetType ? ` [${item.sheetType}]` : ''}`,
    value: item.id,
  })),
]);

const selectDefaultNewCardTemplate = () => {
  const preferredTemplate = templateStore.getPreferredTemplateBySheetType(newCardSheetType.value);
  if (preferredTemplate?.id) {
    newCardTemplateId.value = preferredTemplate.id;
    if (preferredTemplate.sheetType?.trim()) {
      setNewCardSheetType(preferredTemplate.sheetType);
    }
    return;
  }
  if (newCardSheetTypePreset.value === 'custom') {
    newCardTemplateId.value = DETACHED_TEMPLATE_VALUE;
    return;
  }
  const globalDefault = templateStore.getGlobalDefaultTemplate();
  const availableIds = new Set(newCardTemplateOptions.value.map(item => item.value));
  const globalDefaultId = globalDefault?.id || '';
  const canUseGlobalDefault = !!globalDefaultId
    && availableIds.has(globalDefaultId)
    && (!globalDefault?.sheetType?.trim()
      || templateStore.isSameSheetType(globalDefault?.sheetType, newCardSheetType.value));
  newCardTemplateId.value = canUseGlobalDefault
    ? globalDefaultId
    : DETACHED_TEMPLATE_VALUE;
};

const syncNewCardTemplateForSheetType = () => {
  const selectedTemplate = templateStore.getTemplateById(newCardTemplateId.value);
  if (selectedTemplate && templateStore.isSameSheetType(selectedTemplate.sheetType, newCardSheetType.value)) {
    return;
  }
  selectDefaultNewCardTemplate();
};

const handleNewCardTemplateChange = (templateId: string | number | null) => {
  const selectedTemplate = templateStore.getTemplateById(String(templateId || '').trim());
  const sheetType = selectedTemplate?.sheetType?.trim();
  if (!sheetType) return;
  setNewCardSheetType(sheetType);
};

const openCreateModal = async () => {
  if (!ensureCharacterApiEnabled()) return;
  await templateStore.ensureTemplatesLoaded({ worldId: currentWorldId.value || undefined });
  newCardName.value = '';
  newCardSheetTypePreset.value = 'coc7';
  newCardSheetTypeCustom.value = '';
  selectDefaultNewCardTemplate();
  newCardAvatarFile.value = null;
  newCardAttrs.value = {};
  createModalVisible.value = true;
};

watch(newCardSheetType, () => {
  if (!createModalVisible.value) return;
  syncNewCardTemplateForSheetType();
});

const handleCreateCard = async () => {
  if (!ensureCharacterApiEnabled()) return;
  if (!newCardName.value.trim()) {
    message.warning('请输入角色名称');
    return;
  }
  const selectedTemplate = newCardTemplateId.value && newCardTemplateId.value !== DETACHED_TEMPLATE_VALUE
    ? templateStore.getTemplateById(newCardTemplateId.value)
    : null;
  if (selectedTemplate?.sheetType?.trim()) {
    setNewCardSheetType(selectedTemplate.sheetType);
  }
  const sheetType = newCardSheetType.value;
  if (!sheetType) {
    message.warning('请输入自定义规则类型');
    return;
  }
  creating.value = true;
  try {
    const created = await cardStore.createCard(
      resolvedChannelId.value,
      newCardName.value.trim(),
      sheetType,
      newCardAttrs.value,
    );
    if (!created?.id) {
      throw new Error('人物卡创建失败');
    }
    const setupTasks: Array<{ label: string; task: Promise<unknown> }> = [];
    const snapshotTemplatePreset = getCharacterSnapshotTemplatePreset(sheetType);
    if (snapshotTemplatePreset) {
      setupTasks.push({
        label: '默认快照模板启用',
        task: applySnapshotTemplatePreset(
          canSyncBadgeTemplate.value ? 'channel' : 'personal',
          snapshotTemplatePreset,
        ),
      });
    }
    if (newCardTemplateId.value && newCardTemplateId.value !== DETACHED_TEMPLATE_VALUE) {
      setupTasks.push({
        label: '模板应用',
        task: templateStore.bindCardToTemplate({
          channelId: resolvedChannelId.value,
          externalCardId: created.id,
          cardName: created.name || newCardName.value.trim(),
          sheetType: created.sheetType || sheetType,
          templateId: newCardTemplateId.value,
        }),
      });
    } else {
      setupTasks.push({
        label: '模板应用',
        task: templateStore.bindCardToDetachedTemplate({
          channelId: resolvedChannelId.value,
          externalCardId: created.id,
          cardName: created.name || newCardName.value.trim(),
          sheetType: created.sheetType || sheetType,
          templateSnapshot: sheetStore.getDefaultTemplate(sheetType),
        }),
      });
    }
    if (newCardAvatarFile.value) {
      const avatarFile = newCardAvatarFile.value;
      setupTasks.push({
        label: '头像上传',
        task: (async () => {
          const uploadResult = await uploadImageAttachment(avatarFile, {
            channelId: resolvedChannelId.value,
            skipCompression: true,
          });
          return avatarStore.upsertBinding({
            channelId: resolvedChannelId.value,
            externalCardId: created.id,
            cardName: created.name || newCardName.value.trim(),
            sheetType: created.sheetType || sheetType,
            avatarAttachmentId: uploadResult.attachmentId,
          });
        })(),
      });
    }
    const setupResults = await Promise.allSettled(setupTasks.map(item => item.task));
    const failedLabels = setupResults
      .map((result, index) => result.status === 'rejected' ? setupTasks[index].label : '')
      .filter(Boolean);
    createModalVisible.value = false;
    newCardAvatarFile.value = null;
    if (failedLabels.length > 0) {
      message.warning(`人物卡已创建，但${failedLabels.join('、')}失败`);
    } else {
      message.success('创建成功');
    }
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '创建失败');
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

const syncActiveCardAvatarSnapshot = (channelId: string, cardId: string) => {
  if (cardStore.getActiveCardId(channelId) !== cardId) return;
  void snapshotStore.syncLocalSnapshot(channelId, true).catch((error) => {
    console.warn('Failed to sync character card avatar snapshot', error);
  });
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
      const boundCardName = String(cardStore.getCardById(boundCardId)?.name || '').trim();
      if (boundCardName) {
        await cardStore.syncBotNicknameForCard(channelId, boundCardName, {
          reason: 'character-sheet-restore-bound-card',
        });
      }
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
const avatarEditorTarget = ref<'existing-card' | 'new-card' | ''>('');
const avatarUploading = ref(false);
const avatarRemoveVisibleCardId = ref('');

const revealAvatarRemoveOnMobile = (card: CharacterCard) => {
  if (!isMobile.value || !getCardAvatarBinding(card)) return;
  avatarRemoveVisibleCardId.value = avatarRemoveVisibleCardId.value === card.id ? '' : card.id;
};

const handleAvatarUploadTrigger = (card: CharacterCard) => {
  avatarEditingCard.value = card;
  avatarEditorTarget.value = 'existing-card';
  avatarUploadInputRef.value?.click();
};

const handleNewCardAvatarTrigger = () => {
  avatarEditingCard.value = null;
  avatarEditorTarget.value = 'new-card';
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
  avatarEditorTarget.value = '';
};

const handleAvatarEditorSave = async (file: File) => {
  if (avatarEditorTarget.value === 'new-card') {
    newCardAvatarFile.value = file;
    message.success('头像已选择');
    handleAvatarEditorCancel();
    return;
  }
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
    syncActiveCardAvatarSnapshot(channelId, card.id);
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
    syncActiveCardAvatarSnapshot(channelId, card.id);
    avatarRemoveVisibleCardId.value = '';
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
    }, {
      worldId: currentWorldId.value || undefined,
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
    await templateStore.ensureTemplatesLoaded({ worldId: currentWorldId.value || undefined });
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
    const managedTemplateContent = binding?.mode === 'managed' && binding.templateId
      ? templateStore.getTemplateById(binding.templateId)?.content
      : undefined;
    const avatarUrl = resolveCardAvatarToken(card, effectiveCardData?.avatarUrl || '');
    const windowId = sheetStore.openSheet(card, channelId, {
      name: effectiveCardData?.name || card.name,
      type: effectiveCardData?.type || card.sheetType,
      attrs: effectiveCardData?.attrs || card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    }, {
      templateMode: binding?.mode,
      templateId: binding?.templateId || undefined,
      templateText: binding?.mode === 'detached'
        ? binding.templateSnapshot
        : managedTemplateContent,
      worldId: currentWorldId.value || undefined,
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
    }, {
      worldId: currentWorldId.value || undefined,
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
    return false;
  }
  if (mode === 'view' && !await ensureCharacterApiForPreview(channelId)) {
    return false;
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
      await cardStore.syncBotNicknameForCard(channelId, card.name, {
        reason: 'character-sheet-open-preview',
      });
    }
    await openCharacterSheetWindow(card, mode, { restoreToCurrentBinding });
    return true;
  } catch (e: any) {
    clearChannelSheetSwitchState(channelId);
    message.error(e?.response?.data?.error || e?.message || '切换人物卡失败');
    return false;
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

const openCardById = async (cardId: string, mode: 'view' | 'edit' = 'view') => {
  const channelId = resolvedChannelId.value;
  const normalizedCardId = String(cardId || '').trim();
  if (!channelId || !normalizedCardId) return false;
  if (mode === 'view' && !await ensureCharacterApiForPreview(channelId)) return false;
  if (characterApiDisabled.value) return false;

  await loadPanelData(channelId);
  const card = allChannelCards.value.find(item => item.id === normalizedCardId);
  if (!card) return false;

  return openCharacterSheet(card, mode);
};

defineExpose({ openCardById });
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
        <button
          type="button"
          class="settings-group-toggle"
          :aria-expanded="badgeSettingsExpanded"
          @click="toggleBadgeSettingsExpanded"
        >
          <span class="settings-group-toggle__title-wrap">
            <n-icon size="18" class="settings-group-toggle__icon">
              <component :is="badgeSettingsToggleIcon" />
            </n-icon>
            <span class="settings-group-toggle__title">角色徽标设置</span>
          </span>
          <span class="settings-group-toggle__state">{{ badgeSettingsExpanded ? '收起' : '展开' }}</span>
        </button>
        <n-collapse-transition :show="badgeSettingsExpanded">
          <div class="character-card-settings__body">
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
                <p class="settings-title">共享频道人物卡快照</p>
                <p class="settings-desc">为人物卡预览、角色徽章和小剧场数据浮层提供统一数据</p>
              </div>
              <n-switch v-model:value="onlineCharacterCardsEnabled">
                <template #checked>已启用</template>
                <template #unchecked>已关闭</template>
              </n-switch>
            </div>
            <div class="settings-row">
              <div>
                <p class="settings-title">上传自己的人物卡快照</p>
                <p class="settings-desc">关闭后立即清除自己的快照，但仍可读取其他成员快照</p>
              </div>
              <n-switch v-model:value="characterCardSnapshotUploadEnabled">
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
            <div class="settings-row settings-row--stacked">
              <div>
                <p class="settings-title">徽章显示范围</p>
                <p class="settings-desc">控制角色徽章在场内或场外消息中的可见性</p>
              </div>
              <n-radio-group
                v-model:value="badgeVisibilityScope"
                size="small"
                :disabled="characterApiDisabled || !badgeEnabled"
              >
                <n-radio-button
                  v-for="option in badgeVisibilityScopeOptions"
                  :key="option.value"
                  :value="option.value"
                >
                  {{ option.label }}
                </n-radio-button>
              </n-radio-group>
            </div>
            <div class="settings-row">
              <div>
                <p class="settings-title">
                  旁白角色
                  <span v-if="narratorCountBadge" class="settings-count-badge">{{ narratorCountBadge }}</span>
                </p>
                <p class="settings-desc">为当前频道指定旁白身份；这些身份不会显示或广播角色徽章。</p>
              </div>
              <n-button quaternary circle size="small" title="旁白角色设置" aria-label="旁白角色设置" @click="openNarratorSettings">
                <template #icon><n-icon :component="Settings" /></template>
              </n-button>
            </div>
            <div class="settings-row">
              <div>
                <p class="settings-title">自动同步昵称</p>
                <p class="settings-desc">切换频道角色或人物卡后，后台向所选 BOT 静默发送 nn 同步昵称</p>
              </div>
              <n-switch v-model:value="autoSyncBotNicknameEnabled">
                <template #checked>已启用</template>
                <template #unchecked>已关闭</template>
              </n-switch>
            </div>
            <div class="settings-row settings-row--template">
              <div>
                <p class="settings-title">徽章模板</p>
                <p class="settings-desc">支持 {属性名} 和 {对象.子项.属性}；单段属性会自动查找嵌套 JSON 中首个同名键。</p>
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
                >同步频道模板</n-button>
              </div>
            </div>
            <div class="settings-row settings-row--template">
              <p class="settings-title">启用默认模板</p>
              <div class="overlay-template-presets">
                <n-button size="tiny" :disabled="snapshotTemplateSaving" @click="applyDefaultSnapshotTemplatePreset('shinobigami')">忍神</n-button>
                <n-button size="tiny" :disabled="snapshotTemplateSaving" @click="applyDefaultSnapshotTemplatePreset('coc')">COC</n-button>
              </div>
            </div>
            <div class="settings-row settings-row--template">
              <div>
                <p class="settings-title">个人徽章模板</p>
                <p class="settings-desc">只影响自己快照向其他成员展示的徽章。</p>
              </div>
              <div class="settings-template-input">
                <n-select v-model:value="personalBadgeTemplateMode" size="small" :options="templateModeOptions" />
                <n-input
                  v-if="personalBadgeTemplateMode === 'custom'"
                  v-model:value="personalBadgeTemplate"
                  size="small"
                  placeholder="HP{生命值} SAN{理智}"
                />
                <n-button size="small" type="primary" :loading="snapshotTemplateSaving" @click="savePersonalSnapshotTemplates">保存个人模板</n-button>
              </div>
            </div>
          </div>
        </n-collapse-transition>
      </div>

      <div class="character-card-settings">
        <button
          type="button"
          class="settings-group-toggle"
          :aria-expanded="theaterOverlaySettingsExpanded"
          @click="toggleTheaterOverlaySettingsExpanded"
        >
          <span class="settings-group-toggle__title-wrap">
            <n-icon size="18" class="settings-group-toggle__icon">
              <component :is="theaterOverlaySettingsToggleIcon" />
            </n-icon>
            <span class="settings-group-toggle__title">小剧场数据浮窗设置</span>
          </span>
          <span class="settings-group-toggle__state">{{ theaterOverlaySettingsExpanded ? '收起' : '展开' }}</span>
        </button>
        <n-collapse-transition :show="theaterOverlaySettingsExpanded">
          <div class="character-card-settings__body">
            <div class="settings-row">
              <p class="settings-title">小剧场数据浮层模板</p>
              <n-button
                size="small"
                :disabled="!canSyncBadgeTemplate || snapshotTemplateSaving"
                @click="openOverlayTemplateEditor('channel')"
              >
                <template #icon><n-icon :component="Edit" /></template>
                编辑
              </n-button>
            </div>
            <div class="settings-row settings-row--template">
              <p class="settings-title">启用默认模板</p>
              <div class="overlay-template-presets">
                <n-button size="tiny" :disabled="snapshotTemplateSaving" @click="applyDefaultSnapshotTemplatePreset('shinobigami')">忍神</n-button>
                <n-button size="tiny" :disabled="snapshotTemplateSaving" @click="applyDefaultSnapshotTemplatePreset('coc')">COC</n-button>
              </div>
            </div>
            <div class="settings-row settings-row--template">
              <p class="settings-title">个人小剧场浮层模板</p>
              <div class="settings-template-input settings-template-input--inline">
                <n-select v-model:value="personalOverlayTemplateMode" size="small" :options="templateModeOptions" />
                <n-button
                  v-if="personalOverlayTemplateMode === 'custom'"
                  size="small"
                  @click="openOverlayTemplateEditor('personal')"
                >
                  <template #icon><n-icon :component="Edit" /></template>
                  编辑
                </n-button>
                <n-button size="small" type="primary" :loading="snapshotTemplateSaving" @click="savePersonalSnapshotTemplates">保存</n-button>
              </div>
            </div>
          </div>
        </n-collapse-transition>
      </div>

      <div v-if="onlineCharacterCardsEnabled" class="character-card-settings">
        <button
          type="button"
          class="settings-group-toggle"
          :aria-expanded="onlineCharacterCardsExpanded"
          @click="toggleOnlineCharacterCardsExpanded"
        >
          <span class="settings-group-toggle__title-wrap">
            <n-icon size="18" class="settings-group-toggle__icon">
              <component :is="onlineCharacterCardsToggleIcon" />
            </n-icon>
            <span class="settings-group-toggle__title">频道人物卡快照</span>
          </span>
          <span class="settings-group-toggle__state">{{ onlineCharacterCardsExpanded ? '收起' : '展开' }}</span>
        </button>
        <n-collapse-transition :show="onlineCharacterCardsExpanded">
          <div class="character-card-settings__body">
            <div class="online-character-cards__header">
              <div>
                <p class="settings-desc">显示频道成员最近同步的当前人物卡；数据库快照不会反写拥有者前端。</p>
              </div>
              <n-button
                tertiary
                circle
                size="small"
                title="刷新在线成员人物卡"
                aria-label="刷新在线成员人物卡"
                :loading="onlineCharacterCardsLoading"
                @click="refreshOnlineCharacterCards(true)"
              >
                <template #icon><n-icon :component="Refresh" /></template>
              </n-button>
            </div>
            <div class="character-card-list">
              <n-empty v-if="!onlineCharacterCardsLoading && onlineCharacterCards.length === 0" description="暂无在线成员人物卡" />
              <n-card
                v-for="item in onlineCharacterCards"
                :key="`${item.userId}:${item.identityId}`"
                size="small"
                class="character-card-item character-card-item--online"
              >
                <template #header>
                  <div class="card-header-main">
                    <AvatarVue :size="34" :src="item.identityAvatar" :fallback-text="item.identityName || item.card.name" use-text-fallback />
                    <div class="online-card-title-group">
                      <span class="online-card-user" :style="{ color: item.userColor || undefined }">{{ item.username || item.userNick || '未命名用户' }}</span>
                      <span class="card-name">{{ item.card.name }}</span>
                    </div>
                    <n-tag size="small" :bordered="false">{{ item.card.sheetType || 'custom' }}</n-tag>
                  </div>
                </template>
                <template #header-extra>
                  <n-button
                    secondary
                    size="tiny"
                    title="预览"
                    aria-label="查看人物卡"
                    @click="openOnlineCharacterCardPreview(item)"
                  >
                    <template #icon><n-icon :component="Eye" /></template>
                    查看
                  </n-button>
                </template>
                <div class="card-main-content">
                  <div class="online-card-meta">
                    <span class="online-card-meta__identity">{{ item.identityName || '未命名角色' }}</span>
                    <span class="online-card-meta__updated-at">{{ formatOnlineCardUpdatedAt(item.updatedAt) }}</span>
                  </div>
                </div>
              </n-card>
            </div>
          </div>
        </n-collapse-transition>
      </div>

      <n-divider style="margin: 8px 0 12px;" />

      <div class="card-search-row">
        <n-input
          v-model:value="cardSearchKeyword"
          size="small"
          clearable
          :disabled="characterApiDisabled"
          placeholder="搜索人物卡 (名称/规则/属性) 单击查看即可切换人物卡"
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
              <div
                class="card-avatar-control"
                :class="{ 'card-avatar-control--remove-visible': avatarRemoveVisibleCardId === card.id }"
                @click="revealAvatarRemoveOnMobile(card)"
              >
                <AvatarVue :size="34" :src="resolveCardAvatarToken(card)" />
                <n-button
                  v-if="getCardAvatarBinding(card)"
                  class="card-avatar-remove"
                  quaternary
                  circle
                  size="tiny"
                  type="error"
                  title="移除头像"
                  aria-label="移除头像"
                  :disabled="characterApiDisabled || avatarUploading || cardSwitchingId.length > 0"
                  @click.stop="handleAvatarRemove(card)"
                >
                  <template #icon><n-icon :component="X" /></template>
                </n-button>
              </div>
              <span class="card-name">{{ card.name }}</span>
              <span v-if="isCurrentActiveCard(card)" class="card-state-badge card-state-badge--active">使用中</span>
              <span v-else-if="isCurrentBoundCard(card)" class="card-state-badge">当前角色</span>
              <n-tag size="small" :bordered="false">{{ card.sheetType || 'custom' }}</n-tag>
            </div>
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

            <div class="card-actions" aria-label="人物卡操作">
              <n-button
                secondary
                size="tiny"
                title="上传头像"
                :disabled="characterApiDisabled || avatarUploading || cardSwitchingId.length > 0"
                @click="handleAvatarUploadTrigger(card)"
              >
                <template #icon><n-icon :component="Upload" /></template>
                头像
              </n-button>
              <n-button
                secondary
                size="tiny"
                title="预览"
                :loading="cardSwitchingId === card.id"
                :disabled="cardSwitchingId.length > 0 && cardSwitchingId !== card.id"
                @click="openPreview(card)"
              >
                <template #icon><n-icon :component="Eye" /></template>
                查看
              </n-button>
              <n-button
                secondary
                size="tiny"
                title="编辑"
                :disabled="characterApiDisabled || (cardSwitchingId.length > 0 && cardSwitchingId !== card.id)"
                :loading="cardSwitchingId === card.id"
                @click="openEditPanel(card)"
              >
                <template #icon><n-icon :component="Edit" /></template>
                编辑
              </n-button>
              <n-button
                secondary
                size="tiny"
                title="绑定身份"
                :disabled="characterApiDisabled || cardSwitchingId.length > 0"
                @click="openBindModal(card)"
              >
                <template #icon><n-icon :component="Link" /></template>
                绑定
              </n-button>
              <n-popconfirm @positive-click="handleDeleteCard(card)">
                <template #trigger>
                  <n-button
                    secondary
                    size="tiny"
                    type="error"
                    title="删除"
                    :disabled="characterApiDisabled || cardSwitchingId.length > 0"
                  >
                    <template #icon><n-icon :component="Trash" /></template>
                    删除
                  </n-button>
                </template>
                删除前将从所有群解绑此人物卡，确定删除？
              </n-popconfirm>
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
          placeholder="输入自定义规则类型"
          class="sheet-type-custom-input"
          :disabled="characterApiDisabled"
        />
      </n-form-item>
      <n-form-item label="人物卡模板">
        <n-select
          v-model:value="newCardTemplateId"
          :options="newCardTemplateOptions"
          placeholder="选择人物卡模板"
          :disabled="characterApiDisabled"
          @update:value="handleNewCardTemplateChange"
        />
      </n-form-item>
      <n-form-item label="人物卡头像">
        <div class="new-card-avatar-picker">
          <n-button secondary :disabled="characterApiDisabled || creating" @click="handleNewCardAvatarTrigger">
            <template #icon><n-icon :component="Upload" /></template>
            {{ newCardAvatarFile ? '重新选择' : '选择头像' }}
          </n-button>
          <span v-if="newCardAvatarFile" class="new-card-avatar-picker__status">已裁剪头像</span>
          <n-button
            v-if="newCardAvatarFile"
            text
            type="error"
            :disabled="creating"
            @click="newCardAvatarFile = null"
          >
            移除
          </n-button>
        </div>
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="overlayTemplateEditorVisible"
    preset="card"
    title="编辑小剧场数据浮层"
    style="width: min(980px, 94vw);"
    :bordered="false"
  >
    <div class="overlay-template-editor">
      <div class="overlay-template-editor__toolbar">
        <input
          ref="overlayTemplateImportInput"
          class="overlay-template-editor__file-input"
          type="file"
          accept="application/json,.json"
          @change="importOverlayTemplate"
        />
        <n-button size="small" @click="triggerOverlayTemplateImport">
          <template #icon><n-icon :component="Upload" /></template>
          导入
        </n-button>
        <n-button size="small" @click="exportOverlayTemplate">
          <template #icon><n-icon :component="Download" /></template>
          导出
        </n-button>
        <n-button type="primary" size="small" @click="addOverlayTemplateEditorItem">
          <template #icon><n-icon :component="Plus" /></template>
          新增
        </n-button>
      </div>
      <div class="overlay-template-editor__header" aria-hidden="true">
        <span />
        <span>数据名字</span>
        <span>当前值</span>
        <span>最小值</span>
        <span>最大值</span>
        <span>文本颜色</span>
        <span>数据条颜色</span>
        <span />
      </div>
      <div
        v-for="item in overlayTemplateEditorItems"
        :key="item.id"
        class="overlay-template-editor__row"
        :class="{ 'overlay-template-editor__row--dragging': draggingOverlayItemId === item.id }"
        @dragover.prevent
        @drop="reorderOverlayTemplateEditorItem(item.id)"
      >
        <button
          type="button"
          class="overlay-template-editor__drag-handle"
          title="拖动排序"
          aria-label="拖动排序"
          draggable="true"
          @dragstart="beginOverlayTemplateItemDrag(item.id, $event)"
          @dragend="draggingOverlayItemId = ''"
        >
          <n-icon :component="GripVertical" />
        </button>
        <n-input v-model:value="item.name" size="small" placeholder="生命值" />
        <n-input v-model:value="item.current.value" size="small" placeholder="生命值">
          <template #suffix><span class="overlay-template-editor__resolved-value">{{ formatOverlayEditorCurrentValue(item.current) }}</span></template>
        </n-input>
        <n-input v-model:value="item.min.value" size="small" placeholder="0" />
        <n-input v-model:value="item.max.value" size="small" placeholder="生命值上限" />
        <n-color-picker v-model:value="item.textColor" :show-alpha="false" size="small" />
        <n-color-picker v-model:value="item.barColor" :show-alpha="false" size="small" />
        <n-button
          quaternary
          circle
          size="small"
          type="error"
          title="删除"
          aria-label="删除"
          @click="removeOverlayTemplateEditorItem(item.id)"
        >
          <template #icon><n-icon :component="Trash" /></template>
        </n-button>
      </div>
      <n-empty v-if="!overlayTemplateEditorItems.length" size="small" description="暂无数据项" class="overlay-template-editor__empty" />

      <div class="overlay-template-preview">
        <span class="overlay-template-preview__label">预览</span>
        <div v-if="overlayTemplatePreviewItems.length" class="overlay-template-preview__stats">
          <div v-for="item in overlayTemplatePreviewItems" :key="item.id" class="overlay-template-preview__stat">
            <div class="overlay-template-preview__stat-line" :style="{ color: item.textColor }">
              <span>{{ item.name }}</span>
              <span>{{ item.max === null ? item.current : `${item.current}/${item.max}` }}</span>
            </div>
            <div class="overlay-template-preview__bar">
              <span :style="{ width: `${item.percent}%`, backgroundColor: item.barColor }" />
            </div>
          </div>
        </div>
        <span v-else class="overlay-template-preview__empty">暂无可预览数据</span>
      </div>
      <div class="overlay-template-editor__actions">
        <n-button @click="overlayTemplateEditorVisible = false">取消</n-button>
        <n-button type="primary" :loading="snapshotTemplateSaving" @click="saveOverlayTemplateEditor">保存</n-button>
      </div>
    </div>
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
          placeholder="全部规则"
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
              <n-tag v-if="tpl.access === 'world_shared'" size="small" type="warning" :bordered="false">世界共享</n-tag>
              <n-tag v-else size="small" type="default" :bordered="false">我的模板</n-tag>
              <n-tag v-if="tpl.isSharedToCurrentWorld && tpl.access !== 'world_shared'" size="small" type="primary" :bordered="false">已共享</n-tag>
              <n-tag v-if="tpl.isGlobalDefault && !tpl.readonly" size="small" type="info" :bordered="false">全局默认</n-tag>
              <n-tag v-if="tpl.isSheetDefault && !tpl.readonly" size="small" type="success" :bordered="false">规则默认</n-tag>
            </div>
          </div>
        </template>
        <div class="template-manager__preview">{{ formatTemplatePreview(tpl.content) || '空模板' }}</div>
        <div v-if="tpl.access === 'world_shared' && tpl.sharedByNickname" class="template-manager__meta">共享者：{{ tpl.sharedByNickname }}</div>
        <div class="template-manager__actions">
          <n-button
            v-if="canToggleWorldSharedTemplate(tpl)"
            text
            size="small"
            :disabled="characterApiDisabled"
            @click="toggleTemplateWorldShared(tpl)"
          >
            {{ tpl.isSharedToCurrentWorld ? '取消世界共享' : '设为世界共享' }}
          </n-button>
          <n-button v-if="canEditTemplateItem(tpl)" text size="small" :disabled="characterApiDisabled" @click="openTemplateEditModal(tpl)">编辑</n-button>
          <n-button text size="small" :disabled="characterApiDisabled" @click="handleCopyTemplate(tpl)">复制</n-button>
          <n-button v-if="canEditTemplateItem(tpl)" text size="small" :disabled="characterApiDisabled" @click="setAsGlobalDefault(tpl)">设为全局默认</n-button>
          <n-button v-if="canEditTemplateItem(tpl)" text size="small" :disabled="characterApiDisabled" @click="setAsSheetDefault(tpl)">设为规则默认</n-button>
          <n-popconfirm v-if="canEditTemplateItem(tpl)" @positive-click="handleDeleteTemplate(tpl)">
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
      <n-form-item label="规则类型">
        <n-select v-model:value="templateSheetTypePreset" :options="sheetTypeOptions" :disabled="characterApiDisabled" />
        <n-input
          v-if="templateSheetTypePreset === 'custom'"
          v-model:value="templateSheetTypeCustom"
          placeholder="输入自定义规则类型"
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
      <n-form-item label="默认角色徽章模板">
        <n-input
          v-model:value="templateDefaultBadgeTemplate"
          placeholder="留空则不修改当前世界本地徽章模板"
          :disabled="characterApiDisabled"
        />
      </n-form-item>
      <n-form-item label="默认设置">
        <div class="template-manager__defaults">
          <n-checkbox v-model:checked="templateGlobalDefault" :disabled="characterApiDisabled">设为全局默认</n-checkbox>
          <n-checkbox v-model:checked="templateSheetDefault" :disabled="characterApiDisabled">设为规则默认</n-checkbox>
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
    v-model:show="narratorSettingsVisible"
    preset="dialog"
    :show-icon="false"
    title="旁白角色设置"
    positive-text="保存"
    negative-text="取消"
    @positive-click="handleNarratorSettingsSave"
  >
    <div class="narrator-settings">
      <p class="settings-desc">可为当前频道选择多个旁白身份。保存后会立刻停止这些身份的角色徽章广播，并清掉本地缓存。</p>
      <div v-if="identities.length > 0" class="narrator-settings__list">
        <n-checkbox
          v-for="identity in identities"
          :key="identity.id"
          :checked="narratorIdentityIdsDraft.includes(identity.id)"
          @update:checked="(checked: boolean) => handleNarratorIdentityChecked(identity.id, checked)"
        >
          {{ identity.displayName || identity.id }}
        </n-checkbox>
      </div>
      <n-empty v-else description="当前频道暂无可选身份" />
    </div>
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

.new-card-avatar-picker {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.new-card-avatar-picker__status {
  color: var(--n-text-color-3);
  font-size: 13px;
  white-space: nowrap;
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
  gap: 0.5rem;
  padding: 0.25rem 0 1rem;
  border-bottom: 1px solid var(--sc-border-color);
  margin-bottom: 1rem;
}

.character-card-settings__body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding-top: 0.25rem;
}

.online-character-cards {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.online-character-cards__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
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

.settings-row--stacked {
  align-items: flex-start;
  flex-direction: column;
}

.settings-group-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0.35rem 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.settings-group-toggle__title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
}

.settings-group-toggle__icon {
  color: var(--sc-text-secondary);
  flex-shrink: 0;
}

.settings-group-toggle__title {
  font-size: 0.95rem;
  font-weight: 600;
}

.settings-group-toggle__state {
  color: var(--sc-text-secondary);
  font-size: 0.78rem;
  flex-shrink: 0;
}

.settings-title {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-weight: 500;
  margin-bottom: 0.1rem;
}

.settings-desc {
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.settings-count-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.1rem;
  height: 1.1rem;
  padding: 0 0.35rem;
  border-radius: 999px;
  background: rgba(59, 130, 246, 0.14);
  color: rgb(37, 99, 235);
  font-size: 0.72rem;
  line-height: 1;
}

.settings-template-input {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 210px;
}

.settings-template-input--inline {
  min-width: 250px;
}

.overlay-template-presets {
  display: flex;
  gap: 0.4rem;
}

.overlay-template-editor {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
}

.overlay-template-editor__toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.overlay-template-editor__file-input {
  display: none;
}

.overlay-template-editor__header,
.overlay-template-editor__row {
  display: grid;
  grid-template-columns: 28px minmax(90px, 1.15fr) minmax(110px, 1.25fr) minmax(72px, 0.8fr) minmax(90px, 1fr) 70px 70px 30px;
  align-items: center;
  gap: 0.45rem;
}

.overlay-template-editor__header {
  padding: 0 0.25rem;
  color: var(--sc-text-secondary);
  font-size: 0.74rem;
}

.overlay-template-editor__row {
  padding: 0.45rem 0.25rem;
  border-top: 1px solid var(--sc-border-color);
  transition: background-color 0.16s ease;
}

.overlay-template-editor__row--dragging {
  background: rgba(59, 130, 246, 0.08);
}

.overlay-template-editor__drag-handle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--sc-text-secondary);
  cursor: grab;
}

.overlay-template-editor__drag-handle:active {
  cursor: grabbing;
}

.overlay-template-editor__resolved-value {
  max-width: 58px;
  overflow: hidden;
  color: var(--sc-text-secondary);
  font-size: 0.72rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.overlay-template-editor__empty {
  padding: 1.5rem 0;
}

.overlay-template-preview {
  padding: 0.7rem 0.8rem;
  border: 1px solid var(--sc-border-color);
  border-radius: 6px;
  background: color-mix(in srgb, var(--sc-bg-secondary, #1e1e24) 86%, black);
}

.overlay-template-preview__label {
  display: block;
  margin-bottom: 0.5rem;
  color: var(--sc-text-secondary);
  font-size: 0.74rem;
}

.overlay-template-preview__stats {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.55rem 0.8rem;
}

.overlay-template-preview__stat-line {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 0.24rem;
  font-size: 0.76rem;
}

.overlay-template-preview__bar {
  height: 5px;
  overflow: hidden;
  border-radius: 3px;
  background: rgba(148, 163, 184, 0.25);
}

.overlay-template-preview__bar > span {
  display: block;
  height: 100%;
  border-radius: inherit;
}

.overlay-template-preview__empty {
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.overlay-template-editor__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.narrator-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.narrator-settings__list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: min(320px, 50vh);
  overflow: auto;
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

.template-manager__meta {
  margin-top: 0.35rem;
  font-size: 0.72rem;
  color: var(--sc-text-secondary);
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
    align-items: flex-start;
    gap: 0.4rem;
    min-width: 0;
  }

  .card-avatar-control {
    position: relative;
    flex: 0 0 34px;
    width: 34px;
    height: 34px;
  }

  .card-avatar-remove {
    position: absolute;
    top: -5px;
    right: -5px;
    z-index: 1;
    width: 16px;
    height: 16px;
    min-width: 16px;
    min-height: 16px;
    padding: 0;
    color: #fff !important;
    background: var(--n-error-color, #d03050) !important;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.28);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.15s ease;
  }

  .card-avatar-remove :deep(.n-button__icon) {
    font-size: 11px;
  }

  .card-avatar-control:hover .card-avatar-remove,
  .card-avatar-control--remove-visible .card-avatar-remove {
    opacity: 1;
    pointer-events: auto;
  }

  .card-name {
    flex: 1 1 auto;
    font-weight: 600;
    font-size: 0.92rem;
    min-width: 0;
    line-height: 1.2;
    overflow-wrap: anywhere;
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

  .card-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.3rem;
    padding-top: 0.4rem;
    border-top: 1px solid var(--sc-border-color);
  }

  .card-actions :deep(.n-button) {
    height: 26px;
    min-height: 26px;
    padding: 0 0.42rem;
  }

  &.character-card-item--online {
    position: relative;

    :deep(.n-card-header__extra) {
      position: absolute;
      top: 50%;
      right: 0.75rem;
      transform: translateY(-50%);
    }

    .card-header-main {
      padding-right: 4.25rem;
    }
  }

  .online-card-title-group {
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 0.12rem;
  }

  .online-card-user {
    font-size: 0.76rem;
    line-height: 1.1;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .online-card-meta {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    min-width: 0;
  }

  .online-card-meta__identity {
    color: var(--sc-text-secondary);
    font-size: 0.76rem;
    line-height: 1.2;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .online-card-meta__updated-at {
    color: var(--sc-text-tertiary);
    font-size: 0.72rem;
    line-height: 1.2;
    white-space: nowrap;
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

  .card-avatar-control--remove-visible .card-avatar-remove {
    opacity: 1;
    pointer-events: auto;
  }

  .settings-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .settings-template-input {
    min-width: 0;
    width: 100%;
  }

  .overlay-template-editor__header {
    display: none;
  }

  .overlay-template-editor__row {
    grid-template-columns: 28px minmax(0, 1fr) minmax(0, 1fr) 30px;
  }

  .overlay-template-editor__row > :nth-child(1) {
    grid-column: 1;
    grid-row: 1;
  }

  .overlay-template-editor__row > :nth-child(2) {
    grid-column: 2;
    grid-row: 1;
  }

  .overlay-template-editor__row > :nth-child(3) {
    grid-column: 3;
    grid-row: 1;
  }

  .overlay-template-editor__row > :nth-child(4) {
    grid-column: 2;
    grid-row: 2;
  }

  .overlay-template-editor__row > :nth-child(5) {
    grid-column: 3;
    grid-row: 2;
  }

  .overlay-template-editor__row > :nth-child(6) {
    grid-column: 2;
    grid-row: 3;
  }

  .overlay-template-editor__row > :nth-child(7) {
    grid-column: 3;
    grid-row: 3;
  }

  .overlay-template-editor__row > :nth-child(8) {
    grid-column: 4;
    grid-row: 1 / span 3;
  }

  .overlay-template-preview__stats {
    grid-template-columns: 1fr;
  }

  .online-character-cards__header {
    align-items: center;
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
