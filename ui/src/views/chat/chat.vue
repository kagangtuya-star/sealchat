<script setup lang="tsx">
import ChatItem from './components/chat-item.vue';
import MultiSelectFloatingBar from './components/MultiSelectFloatingBar.vue';
import { computed, ref, watch, onMounted, onBeforeMount, onBeforeUnmount, nextTick, reactive } from 'vue'
import { VirtualList } from 'vue-tiny-virtual-list';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { Event, Message, User, WhisperMeta } from '@satorijs/protocol'
import type { ChannelIdentity, ChannelIdentityFolder, GalleryItem, UserInfo, SChannel } from '@/types'
import { useUserStore } from '@/stores/user';
import { ArrowBarToDown, Plus, Upload, Send, ArrowBackUp, Palette, Download, ArrowsVertical, Star, StarOff, FolderPlus, DotsVertical, Folders, Copy as CopyIcon, Search as SearchIcon, Check, X } from '@vicons/tabler'
import { NIcon, c, useDialog, useMessage, type MentionOption } from 'naive-ui';
import VueScrollTo from 'vue-scrollto'
import ChatInputSwitcher from './components/ChatInputSwitcher.vue'
import ChannelIdentitySwitcher from './components/ChannelIdentitySwitcher.vue'
import GalleryButton from '@/components/gallery/GalleryButton.vue'
import GalleryPanel from '@/components/gallery/GalleryPanel.vue'
import ChatIcOocToggle from './components/ChatIcOocToggle.vue'
import ChatActionRibbon from './components/ChatActionRibbon.vue'
import ChannelFavoriteBar from './components/ChannelFavoriteBar.vue'
import ChannelFavoriteManager from './components/ChannelFavoriteManager.vue'
import DisplaySettingsModal from './components/DisplaySettingsModal.vue'
import IcOocRoleConfigPanel from './components/IcOocRoleConfigPanel.vue'
import ChatSearchPanel from './components/ChatSearchPanel.vue'
import ArchiveDrawer from './components/archive/ArchiveDrawer.vue'
import ExportDialog from './components/export/ExportDialog.vue'
import ExportManagerModal from './components/export/ExportManagerModal.vue'
import ChatImportDialog from './components/ChatImportDialog.vue'
import ChatImportProgress from './components/ChatImportProgress.vue'
import ChannelImageViewerDrawer from './components/ChannelImageViewerDrawer.vue'
import DiceTray from './components/DiceTray.vue'
import IFormPanelHost from '@/components/iform/IFormPanelHost.vue';
import IFormFloatingWindows from '@/components/iform/IFormFloatingWindows.vue';
import IFormDrawer from '@/components/iform/IFormDrawer.vue';
import IFormEmbedInstances from '@/components/iform/IFormEmbedInstances.vue';
import StickyNoteManager from './components/StickyNoteManager.vue';
import CharacterSheetManager from './components/character-sheet/CharacterSheetManager.vue';
import { useStickyNoteStore } from '@/stores/stickyNote';
import { uploadImageAttachment } from './composables/useAttachmentUploader';
import { api, urlBase } from '@/stores/_config';
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
import { DEFAULT_PAGE_TITLE, useUtilsStore } from '@/stores/utils';
import { useDisplayStore } from '@/stores/display';
import { contentEscape, contentUnescape, arrayBufferToBase64, base64ToUint8Array } from '@/utils/tools'
import { triggerBlobDownload } from '@/utils/download';
import { copyTextWithFallback } from '@/utils/clipboard';
import dayjs from 'dayjs';
import IconNumber from '@/components/icons/IconNumber.vue'
import IconBuildingBroadcastTower from '@/components/icons/IconBuildingBroadcastTower.vue'
import { computedAsync, useDebounceFn, useEventListener, useWindowSize, useIntersectionObserver } from '@vueuse/core';
import { useGalleryStore } from '@/stores/gallery';
import { Settings, Close as CloseIcon, EyeOutline, EyeOffOutline } from '@vicons/ionicons5';
import { dialogAskConfirm } from '@/utils/dialog';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { resolveAttachmentUrl, fetchAttachmentMetaById, normalizeAttachmentId, type AttachmentMeta } from '@/composables/useAttachmentResolver';
import { ensureDefaultDiceExpr, matchDiceExpressions, type DiceMatch } from '@/utils/dice';
import { recordDiceHistory } from '@/views/chat/composables/useDiceHistory';
import DOMPurify from 'dompurify';
import type { DisplaySettings, ToolbarHotkeyKey } from '@/stores/display';
import { INPUT_AREA_HEIGHT_LIMITS } from '@/stores/display';
import { useIFormStore } from '@/stores/iform';
import { useWorldGlossaryStore } from '@/stores/worldGlossary';
import { useChannelSearchStore } from '@/stores/channelSearch';
import { useChannelImagesStore } from '@/stores/channelImages';
import { useOnboardingStore } from '@/stores/onboarding';
import WorldKeywordManager from '@/views/world/WorldKeywordManager.vue'
import OnboardingRoot from '@/components/onboarding/OnboardingRoot.vue'
import AvatarSetupPrompt from '@/components/AvatarSetupPrompt.vue'
import AvatarEditor from '@/components/AvatarEditor.vue'
import { isHotkeyMatchingEvent } from '@/utils/hotkey';
import { useRoute, useRouter } from 'vue-router';
import WebhookIntegrationManager from '@/views/split/components/WebhookIntegrationManager.vue';
import EmailNotificationManager from '@/views/split/components/EmailNotificationManager.vue';
import CharacterCardPanel from './components/CharacterCardPanel.vue';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import KeywordSuggestPanel from '@/components/chat/KeywordSuggestPanel.vue';
import { ensurePinyinLoaded, matchKeywords, matchText, type KeywordMatchResult } from '@/utils/pinyinMatch';

// const uploadImages = useObservable<Thumb[]>(
//   liveQuery(() => db.thumbs.toArray()) as any
// )

const chat = useChatStore();
const user = useUserStore();
const gallery = useGalleryStore();
const utils = useUtilsStore();
const display = useDisplayStore();
const worldGlossary = useWorldGlossaryStore();
const channelSearch = useChannelSearchStore();
const channelImages = useChannelImagesStore();
const onboarding = useOnboardingStore();
const iFormStore = useIFormStore();
const stickyNoteStore = useStickyNoteStore();
const characterCardStore = useCharacterCardStore();
const characterSheetStore = useCharacterSheetStore();
iFormStore.bootstrap();
const router = useRouter();
const route = useRoute();
const isEditing = computed(() => !!chat.editing);

const isEmbedMode = computed(() => route.path === '/embed');
const splitEntryEnabled = computed(() => route.path !== '/embed');

let stRefreshTimer: ReturnType<typeof setTimeout> | null = null;
const ST_REFRESH_DELAY = 1000;
const CARD_REFRESH_COMMAND_RE = /^([./。,，！!#\\/])?(st|sc|en|buff|ss|ds|cast|ri)(?=\\s|$|[^a-zA-Z])/i;

const hasCardRefreshCommand = (content: string) => {
  const plain = (content || '').replace(/<[^>]*>/g, '').trim();
  if (!plain) return false;
  const lines = plain.split(/\\r?\\n/);
  return lines.some(line => CARD_REFRESH_COMMAND_RE.test(line.trim()));
};

const scheduleCharacterSheetRefresh = () => {
  if (stRefreshTimer) clearTimeout(stRefreshTimer);
  stRefreshTimer = setTimeout(() => {
    const channelId = chat.curChannel?.id;
    if (channelId) {
      void characterCardStore.getActiveCard(channelId);
    }
    if (characterSheetStore.activeWindowIds.length > 0) {
      void characterSheetStore.refreshAllWindows();
    }
  }, ST_REFRESH_DELAY);
};

const openSplitView = () => {
  const currentChannelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const worldId = chat.currentWorldId ? String(chat.currentWorldId) : '';
  router.push({
    name: 'split',
    query: {
      layout: 'left-column',
      worldId,
      a: currentChannelId,
      b: '',
      notify: '',
    },
  });
};

const toggleStickyNotes = () => {
  stickyNoteStore.toggleVisible();
};

type ExternalPanelKey =
  | 'search'
  | 'archive'
  | 'export'
  | 'import'
  | 'identity'
  | 'gallery'
  | 'display'
  | 'favorites'
  | 'channel-images';

const openPanelForShell = (panel: ExternalPanelKey) => {
  switch (panel) {
    case 'search':
      channelSearch.togglePanel();
      return;
    case 'archive':
      archiveDrawerVisible.value = true;
      return;
    case 'export':
      exportManagerVisible.value = true;
      return;
    case 'import':
      importDialogVisible.value = true;
      return;
    case 'identity':
      void openIdentityManager();
      return;
    case 'gallery':
      void openGalleryPanel();
      return;
    case 'display':
      displaySettingsVisible.value = true;
      return;
    case 'favorites':
      channelFavoritesVisible.value = true;
      return;
    case 'channel-images':
      openChannelImagesPanel();
      return;
    default:
      return;
  }
};

const setFiltersForShell = (filters: any) => {
  chat.setFilterState(filters);
};

defineExpose({
  openPanelForShell,
  setFiltersForShell,
});
// 编辑模式下也允许使用上方功能区，只在个别操作需要限制时单独判断
const inputIcMode = computed<'ic' | 'ooc'>({
  get: () => {
    if (chat.editing?.icMode) {
      return chat.editing.icMode;
    }
    return chat.icMode;
  },
  set: (mode) => {
    if (chat.editing) {
      chat.updateEditingIcMode(mode);
    } else {
      chat.icMode = mode;
      // 触发自动角色切换
      chat.autoSwitchRoleOnIcOocChange(mode);
    }
  },
});

watch(
  () => chat.currentWorldId,
  (worldId) => {
    if (!worldId) {
      return
    }
    worldGlossary.ensureKeywords(worldId)
    chat.worldDetail(worldId)
    hideSelectionBar()
  },
  { immediate: true },
)

watch(
  () => chat.curChannel?.id,
  () => hideSelectionBar(),
)

const canManageWorldKeywords = computed(() => {
  const worldId = chat.currentWorldId
  if (!worldId) {
    return false
  }
  const detail = chat.worldDetailMap[worldId]
  const role = detail?.memberRole
  const allowMemberEdit = detail?.world?.allowMemberEditKeywords ?? detail?.allowMemberEditKeywords ?? false
  return role === 'owner' || role === 'admin' || (allowMemberEdit && role === 'member')
})
const displaySettingsVisible = ref(false);
const compactInlineLayout = computed(() => display.layout === 'compact' && !display.showAvatar);
const scrollButtonColor = computed(() => (display.palette === 'night' ? 'rgba(148, 163, 184, 0.25)' : '#e5e7eb'));
const scrollButtonTextColor = computed(() => (display.palette === 'night' ? 'rgba(248, 250, 252, 0.95)' : '#111827'));

const channelBackgroundStyle = computed(() => {
  const channel = chat.curChannel as SChannel | null;
  if (!channel?.backgroundAttachmentId) return null;
  let settings: { mode?: string; opacity?: number; blur?: number; brightness?: number } = {
    mode: 'cover', opacity: 30, blur: 0, brightness: 100
  };
  if (channel.backgroundSettings) {
    try {
      const parsed = typeof channel.backgroundSettings === 'string'
        ? JSON.parse(channel.backgroundSettings)
        : channel.backgroundSettings;
      settings = { ...settings, ...parsed };
    } catch { /* ignore */ }
  }
  const attachmentId = channel.backgroundAttachmentId;
  const bgUrl = resolveAttachmentUrl(attachmentId.startsWith('id:') ? attachmentId : `id:${attachmentId}`);
  let bgSize = 'cover';
  let bgRepeat = 'no-repeat';
  const bgPosition = 'center';
  switch (settings.mode) {
    case 'contain': bgSize = 'contain'; break;
    case 'tile': bgSize = 'auto'; bgRepeat = 'repeat'; break;
    case 'center': bgSize = 'auto'; break;
  }
  return {
    backgroundImage: `url(${bgUrl})`,
    backgroundSize: bgSize,
    backgroundRepeat: bgRepeat,
    backgroundPosition: bgPosition,
    opacity: (settings.opacity ?? 30) / 100,
    filter: `blur(${settings.blur ?? 0}px) brightness(${settings.brightness ?? 100}%)`,
  };
});

const channelBackgroundOverlayStyle = computed(() => {
  const channel = chat.curChannel as SChannel | null;
  if (!channel?.backgroundAttachmentId || !channel.backgroundSettings) return null;
  let settings: { overlayColor?: string; overlayOpacity?: number } = {};
  try {
    const parsed = typeof channel.backgroundSettings === 'string'
      ? JSON.parse(channel.backgroundSettings)
      : channel.backgroundSettings;
    settings = parsed;
  } catch { /* ignore */ }
  if (!settings.overlayColor || !(settings.overlayOpacity ?? 0)) return null;
  return {
    backgroundColor: settings.overlayColor,
    opacity: (settings.overlayOpacity ?? 0) / 100,
  };
});

const diceTrayVisible = ref(false);
const diceSettingsVisible = ref(false);
const diceFeatureUpdating = ref(false);
const botOptions = ref<UserInfo[]>([]);
const botOptionsLoading = ref(false);
const botOptionsFetched = ref(false);
const isMobileUa = typeof navigator !== 'undefined'
  ? /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  : false;
const diceTrayFollowerClass = 'dice-tray-mobile-wrapper';
const channelBotSelection = ref('');
const channelBotsLoading = ref(false);
const syncingChannelBot = ref(false);
const channelFeatures = reactive({
  builtInDiceEnabled: true,
  botFeatureEnabled: false,
});
const canUseBuiltInDice = computed(() => channelFeatures.builtInDiceEnabled);
const defaultDiceExpr = computed(() => ensureDefaultDiceExpr(chat.curChannel?.defaultDiceExpr));
const botRoleId = computed(() => {
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    return '';
  }
  return `ch-${channelId}-bot`;
});
const canEditDefaultDice = computed(() => {
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    return false;
  }
  return chat.isChannelAdmin(channelId, user.info.id);
});
const canManageChannelFeatures = computed(() => canEditDefaultDice.value);
const botSelectOptions = computed(() => botOptions.value.map((bot) => ({
  label: bot.nick || bot.username || 'Bot',
  value: bot.id,
})));
const hasBotOptions = computed(() => botOptions.value.length > 0);
const diceModeLabel = computed(() => channelFeatures.botFeatureEnabled ? 'BOT掷骰' : '内置掷骰');
const diceModeTooltip = computed(() => {
  if (channelFeatures.botFeatureEnabled) {
    return '当前使用机器人处理掷骰指令，点击齿轮可切换内置掷骰模式';
  }
  return '当前使用内置掷骰功能，点击齿轮可切换机器人掷骰模式';
});
const channelSendAllowed = ref(true);
let sendPermissionSeq = 0;
const isPrivateChatChannel = (channel?: SChannel | null) => {
  if (!channel) {
    return false;
  }
  if (channel.isPrivate) {
    return true;
  }
  if (channel.friendInfo) {
    return true;
  }
  const permType = typeof channel.permType === 'string' ? channel.permType.toLowerCase() : '';
  if (permType === 'private') {
    return true;
  }
  const typeValue = (channel as any)?.type;
  if (typeof typeValue === 'number' && typeValue === 3) {
    return true;
  }
  return false;
};
watch(
  () => chat.curChannel?.id,
  async (channelId) => {
    const seq = ++sendPermissionSeq;
    const currentChannel = chat.curChannel as SChannel | undefined;
    if (!channelId || !currentChannel) {
      channelSendAllowed.value = false;
      return;
    }
    if (isPrivateChatChannel(currentChannel)) {
      channelSendAllowed.value = true;
      return;
    }
    try {
      const allowed = await chat.hasChannelPermission(channelId, 'func_channel_text_send', user.info.id);
      if (seq === sendPermissionSeq) {
        channelSendAllowed.value = !!allowed;
      }
    } catch (error) {
      if (seq === sendPermissionSeq) {
        channelSendAllowed.value = false;
      }
    }
  },
  { immediate: true },
);
const spectatorInputDisabled = computed(() => !channelSendAllowed.value);
const webhookDrawerVisible = ref(false);
const webhookManageAllowed = ref(false);
const emailNotificationDrawerVisible = ref(false);
const characterCardPanelVisible = ref(false);
let webhookPermissionSeq = 0;

watch(
  () => chat.curChannel?.id,
  async (channelId) => {
    const seq = ++webhookPermissionSeq;
    const currentChannel = chat.curChannel as SChannel | undefined;
    if (!channelId || !currentChannel) {
      webhookManageAllowed.value = false;
      return;
    }
    if (isPrivateChatChannel(currentChannel)) {
      webhookManageAllowed.value = false;
      return;
    }
    try {
      const allowed = await chat.hasChannelPermission(channelId, 'func_channel_manage_info', user.info.id);
      if (seq === webhookPermissionSeq) {
        webhookManageAllowed.value = !!allowed;
      }
    } catch (error) {
      if (seq === webhookPermissionSeq) {
        webhookManageAllowed.value = false;
      }
    }
  },
  { immediate: true },
);
const toggleDiceTray = () => {
  if (!channelFeatures.builtInDiceEnabled && !channelFeatures.botFeatureEnabled) {
    message.warning('内置骰点已关闭，请在设置中启用或切换机器人。');
    diceTrayVisible.value = false;
    return;
  }
  diceTrayVisible.value = !diceTrayVisible.value;
};
watch(() => chat.curChannel, (channel) => {
  channelFeatures.builtInDiceEnabled = channel?.builtInDiceEnabled !== false;
  channelFeatures.botFeatureEnabled = channel?.botFeatureEnabled === true;
  if (!channelFeatures.builtInDiceEnabled && !channelFeatures.botFeatureEnabled) {
    diceTrayVisible.value = false;
  }
}, { immediate: true });
watch(() => chat.curChannel?.id, () => {
	diceSettingsVisible.value = false;
	channelBotSelection.value = '';
	botOptions.value = [];
});
watch(canManageChannelFeatures, (canManage) => {
  if (!canManage) {
    diceSettingsVisible.value = false;
  }
});
watch(() => channelFeatures.builtInDiceEnabled, (enabled) => {
	if (!enabled && !channelFeatures.botFeatureEnabled && !diceSettingsVisible.value) {
		diceTrayVisible.value = false;
	}
});
watch(() => channelFeatures.botFeatureEnabled, (enabled) => {
	if (!enabled && !channelFeatures.builtInDiceEnabled && !diceSettingsVisible.value) {
		diceTrayVisible.value = false;
	}
});

const markDiceTrayMobileWrapper = (enabled: boolean) => {
  if (!isMobileUa || typeof document === 'undefined') return;
  const followers = Array.from(document.querySelectorAll('.v-binder-follower-content')) as HTMLElement[];
  followers.forEach((el) => {
    if (!el) return;
    if (el.querySelector('.dice-tray')) {
      if (enabled) {
        el.classList.add(diceTrayFollowerClass);
      } else {
        el.classList.remove(diceTrayFollowerClass);
      }
    } else if (!enabled) {
      el.classList.remove(diceTrayFollowerClass);
    }
  });
};

watch(
  () => diceTrayVisible.value,
  (visible) => {
    if (!isMobileUa) return;
    if (visible) {
      nextTick(() => markDiceTrayMobileWrapper(true));
    } else {
      markDiceTrayMobileWrapper(false);
    }
  },
);
watch(diceTrayVisible, (visible) => {
  if (!visible) {
    diceSettingsVisible.value = false;
  }
});
watch(diceSettingsVisible, (visible) => {
  if (visible) {
    ensureBotOptionsLoaded();
    refreshChannelBotSelection();
  } else if (!channelFeatures.builtInDiceEnabled && !channelFeatures.botFeatureEnabled) {
    diceTrayVisible.value = false;
  }
});

const handleGlobalOverlayToggle = (payload?: { source?: string; open?: boolean }) => {
  if (!payload?.open) {
    return;
  }
  diceTrayVisible.value = false;
  diceSettingsVisible.value = false;
  if (payload.source !== 'emoji-panel') {
    emojiPopoverShow.value = false;
  }
};

const ensureBotOptionsLoaded = async (force = false) => {
	if (botOptionsLoading.value) {
		return;
	}
	if (!force && botOptionsFetched.value && botOptions.value.length) {
		return;
	}
	botOptionsLoading.value = true;
	try {
		const resp = await chat.botList(force);
		botOptions.value = resp?.items || [];
		botOptionsFetched.value = true;
	} catch (error: any) {
		message.error(error?.response?.data?.message || '获取机器人列表失败');
	} finally {
		botOptionsLoading.value = false;
	}
};

const handleBotListUpdated = async () => {
  botOptionsFetched.value = false;
  await ensureBotOptionsLoaded(true);
  if (diceSettingsVisible.value) {
    await refreshChannelBotSelection();
  }
};
chatEvent.on('bot-list-updated', handleBotListUpdated as any);
chatEvent.on('global-overlay-toggle', handleGlobalOverlayToggle as any);
onBeforeUnmount(() => {
  chatEvent.off('bot-list-updated', handleBotListUpdated as any);
  chatEvent.off('global-overlay-toggle', handleGlobalOverlayToggle as any);
});

const refreshChannelBotSelection = async () => {
  const channelId = chat.curChannel?.id;
  const roleId = botRoleId.value;
  if (!channelId || !roleId) {
    channelBotSelection.value = '';
    return;
  }
  channelBotsLoading.value = true;
  try {
    const resp = await chat.channelMemberList(channelId, { page: 1, pageSize: 200 });
    const items = resp?.data?.items || [];
    const current = items.find((item: any) => item.roleId === roleId && item.user?.id);
    channelBotSelection.value = current?.user?.id || '';
  } catch (error: any) {
    message.error(error?.response?.data?.error || '加载频道机器人失败');
  } finally {
    channelBotsLoading.value = false;
  }
};

const syncChannelBotSelection = async (nextBotId: string) => {
  const channelId = chat.curChannel?.id;
  const roleId = botRoleId.value;
  if (!channelId || !roleId) {
    return;
  }
  syncingChannelBot.value = true;
  try {
    const resp = await chat.channelMemberList(channelId, { page: 1, pageSize: 200 });
    const items = resp?.data?.items || [];
    const existingIds = items
      .filter((item: any) => item.roleId === roleId && item.user?.id)
      .map((item: any) => item.user.id as string);
    if (nextBotId && !existingIds.includes(nextBotId)) {
      await chat.userRoleLink(roleId, [nextBotId]);
    }
    const toRemove = nextBotId ? existingIds.filter(id => id !== nextBotId) : existingIds;
    if (toRemove.length) {
      await chat.userRoleUnlink(roleId, toRemove);
    }
    channelBotSelection.value = nextBotId;
  } catch (error: any) {
    message.error(error?.response?.data?.error || '配置机器人失败');
    throw error;
  } finally {
    syncingChannelBot.value = false;
  }
};

const handleBotSelectionChange = async (value: string | null) => {
	const normalized = value || '';
	channelBotSelection.value = normalized;
	try {
		await syncChannelBotSelection(normalized);
	} catch {
		// 已提示
	}
};

const clearChannelBots = async () => {
  try {
    await syncChannelBotSelection('');
  } catch {
    // ignore
  }
};

const updateChannelFeatureFlags = async (updates: { builtInDiceEnabled?: boolean; botFeatureEnabled?: boolean }) => {
  if (!chat.curChannel?.id) {
    return;
  }
  diceFeatureUpdating.value = true;
  try {
    await chat.updateChannelFeatures(chat.curChannel.id, updates);
  } catch (error: any) {
    message.error(error?.response?.data?.error || '更新频道特性失败');
    throw error;
  } finally {
    diceFeatureUpdating.value = false;
  }
};

const handleDiceFeatureToggle = async (value: boolean) => {
  if (!canManageChannelFeatures.value) {
    return;
  }
  try {
    const updates: { builtInDiceEnabled?: boolean; botFeatureEnabled?: boolean } = { builtInDiceEnabled: value };
    if (value && channelFeatures.botFeatureEnabled) {
      updates.botFeatureEnabled = false;
    }
    await updateChannelFeatureFlags(updates);
  } catch {
    // no-op
  }
};

const handleBotFeatureToggle = async (value: boolean) => {
  if (!canManageChannelFeatures.value || !botRoleId.value) {
    return;
  }
  try {
    if (value) {
      await ensureBotOptionsLoaded();
      if (!hasBotOptions.value) {
        message.error('暂无可用机器人令牌，请先在后台创建。');
        return;
      }
      if (!channelBotSelection.value) {
        channelBotSelection.value = botOptions.value[0]?.id || '';
      }
      if (!channelBotSelection.value) {
        return;
      }
      await syncChannelBotSelection(channelBotSelection.value);
      await updateChannelFeatureFlags({ botFeatureEnabled: true, builtInDiceEnabled: false });
    } else {
      await clearChannelBots();
      await updateChannelFeatureFlags({ botFeatureEnabled: false });
    }
  } catch {
    // 已提示
  }
};

const openChannelMemberSettings = () => {
  diceSettingsVisible.value = false;
  chatEvent.emit('channel-member-settings-open');
};
watch(() => chat.curChannel?.id, (id) => {
  if (id) {
    chat.ensureChannelPermissionCache(id);
  }
}, { immediate: true });
const INLINE_STACK_BREAKPOINT = 640;
const { width: windowWidth } = useWindowSize();
const compactInlineStackLayout = computed(() => {
  if (!compactInlineLayout.value) return false;
  const width = windowWidth.value;
  if (!width) return false;
  return width <= INLINE_STACK_BREAKPOINT;
});
const compactInlineGridLayout = computed(
  () => compactInlineLayout.value && !compactInlineStackLayout.value,
);

const defaultPageTitle = computed(() => {
  const title = utils.config?.pageTitle?.trim();
  if (title && title.length > 0) {
    return title;
  }
  return DEFAULT_PAGE_TITLE;
});
const syncPageTitle = (channelName?: string | null) => {
  if (typeof document === 'undefined') return;
  const fallback = defaultPageTitle.value;
  document.title = channelName && channelName.trim().length > 0 ? channelName : fallback;
};

watch(
  () => [chat.curChannel?.id, chat.curChannel?.name] as const,
  ([, name]) => {
    syncPageTitle(name);
  },
  { immediate: true },
);

watch(defaultPageTitle, () => {
  syncPageTitle(chat.curChannel?.name);
});

onBeforeUnmount(() => {
  syncPageTitle();
  removeSelfTypingPreview();
});

watch(
  () => display.settings,
  (value) => {
    display.applyTheme(value);
  },
  { deep: true, immediate: true },
);

// 新增状态
const showActionRibbon = ref(false);
const archiveDrawerVisible = ref(false);
const exportManagerVisible = ref(false);
const exportDialogVisible = ref(false);
const channelFavoritesVisible = ref(false);
const importDialogVisible = ref(false);
const importProgressVisible = ref(false);
const importJobId = ref('');
const avatarPromptVisible = ref(false);
const avatarPromptDismissedThisSession = ref(false);
const ribbonRoleOptions = ref<Array<{ id: string; label: string }>>([]);
let ribbonRoleOptionsSeq = 0;

const fetchRibbonRoleOptions = async (channelId?: string | null) => {
  const normalizedId = typeof channelId === 'string' ? channelId.trim() : '';
  if (!normalizedId) {
    ribbonRoleOptions.value = [];
    return;
  }
  const currentSeq = ++ribbonRoleOptionsSeq;
  try {
    const payload = await chat.channelSpeakerOptions(normalizedId);
    if (currentSeq !== ribbonRoleOptionsSeq) {
      return;
    }
    const items = Array.isArray(payload?.items) ? payload.items : [];
    const mapped = items
      .map((item) => ({
        id: String(item.id || ''),
        label: item.label || '未命名角色',
      }))
      .filter((item) => item.id);
    if (!mapped.some((item) => item.id === ROLELESS_FILTER_ID)) {
      mapped.push({ id: ROLELESS_FILTER_ID, label: '其他' });
    }
    ribbonRoleOptions.value = mapped;
  } catch (error) {
    if (currentSeq === ribbonRoleOptionsSeq) {
      ribbonRoleOptions.value = [];
    }
  }
};

watch(
  () => chat.curChannel?.id,
  (channelId) => {
    fetchRibbonRoleOptions(channelId);
  },
  { immediate: true },
);

const initCharacterCardBadge = (
  channelId?: string,
  enabled = display.settings.characterCardBadgeEnabled,
  options?: { skipActiveCard?: boolean },
) => {
  if (!channelId) return;
  if (!enabled) return;
  void characterCardStore.requestBadgeSnapshot(channelId);
  if (!options?.skipActiveCard) {
    void characterCardStore.getActiveCard(channelId);
  }
};

let identitySelectionEpoch = 0;
const simulateCurrentIdentitySelection = async (channelId?: string) => {
  if (!channelId || chat.isObserver) {
    return false;
  }
  const currentEpoch = ++identitySelectionEpoch;
  try {
    await chat.loadChannelIdentities(channelId, false);
    if (currentEpoch !== identitySelectionEpoch || channelId !== chat.curChannel?.id) {
      return false;
    }
    const identityId = chat.getActiveIdentityId(channelId);
    if (!identityId) {
      return false;
    }
    const boundCardId = characterCardStore.getBoundCardId(identityId);
    if (boundCardId) {
      await characterCardStore.tagCard(channelId, undefined, boundCardId);
    } else {
      await characterCardStore.tagCard(channelId);
    }
    await characterCardStore.loadCards(channelId);
    emitTypingPreview();
    return true;
  } catch (e) {
    console.warn('Failed to simulate identity selection', e);
    return false;
  }
};

let presenceBadgeChannelId = '';
let presenceBadgeInitialized = false;
const presenceBadgeUsers = new Set<string>();

watch(
  () => chat.curChannel?.id,
  (channelId) => {
    if (!channelId) return;
    void (async () => {
      const didSync = await simulateCurrentIdentitySelection(channelId);
      initCharacterCardBadge(channelId, undefined, { skipActiveCard: didSync });
    })();
  },
  { immediate: true },
);

watch(
  () => display.settings.characterCardBadgeEnabled,
  (enabled) => {
    if (!enabled) return;
    initCharacterCardBadge(chat.curChannel?.id, enabled);
  },
);

const syncActionRibbonState = () => {
  chatEvent.emit('action-ribbon-state', showActionRibbon.value);
};

const handleActionRibbonToggleRequest = () => {
  showActionRibbon.value = !showActionRibbon.value;
};

const handleActionRibbonStateRequest = () => {
  syncActionRibbonState();
};

const handleOpenDisplaySettings = () => {
  displaySettingsVisible.value = true;
};

const handleDisplaySettingsSave = (settings: DisplaySettings) => {
  display.updateSettings(settings);
  displaySettingsVisible.value = false;
};

// Avatar prompt handlers
const handleOpenAvatarPrompt = () => {
  avatarPromptVisible.value = true;
};

const handleAvatarPromptSetup = () => {
  avatarPromptVisible.value = false;
  // Emit event to open user profile panel
  chatEvent.emit('open-user-profile');
};

const handleAvatarPromptSkip = () => {
  avatarPromptVisible.value = false;
  avatarPromptDismissedThisSession.value = true;
};

// Check if avatar prompt should be shown on mount (session-based)
const checkAvatarPromptOnMount = () => {
  if (chat.isObserver) return;
  if (avatarPromptDismissedThisSession.value) return;
  if (!user.hasDefaultAvatar) return;
  // Show prompt after a brief delay for better UX
  setTimeout(() => {
    if (!avatarPromptDismissedThisSession.value && user.hasDefaultAvatar) {
      avatarPromptVisible.value = true;
    }
  }, 2000);
};

watch(
  showActionRibbon,
  () => {
    syncActionRibbonState();
  },
  { immediate: true },
);

chatEvent.on('action-ribbon-toggle', handleActionRibbonToggleRequest);
chatEvent.on('action-ribbon-state-request', handleActionRibbonStateRequest);
chatEvent.on('open-display-settings', handleOpenDisplaySettings);

const emojiLoading = ref(false)
// 统一使用 Gallery Store 的表情收藏数据
const emojiItems = computed<GalleryItem[]>(() => gallery.emojiItems);

const EMOJI_THUMB_SIZE = 80;
const emojiAttachmentMetaCache = reactive<Record<string, AttachmentMeta | null>>({});
const pendingEmojiMetaFetch = new Set<string>();

const ensureEmojiAttachmentMeta = async (attachmentId: string) => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized || pendingEmojiMetaFetch.has(normalized) || emojiAttachmentMetaCache[normalized] !== undefined) {
    return;
  }
  pendingEmojiMetaFetch.add(normalized);
  try {
    const meta = await fetchAttachmentMetaById(normalized);
    emojiAttachmentMetaCache[normalized] = meta;
  } finally {
    pendingEmojiMetaFetch.delete(normalized);
  }
};

const resolveEmojiAttachmentUrl = (attachmentId: string) => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized) {
    return '';
  }
  const meta = emojiAttachmentMetaCache[normalized];
  if (meta === undefined && !pendingEmojiMetaFetch.has(normalized)) {
    void ensureEmojiAttachmentMeta(normalized);
  }
  // Animated images should use original to preserve animation
  if (meta?.isAnimated) {
    return resolveAttachmentUrl(normalized);
  }
  // Use server-side thumbnail API for faster loading
  return `${urlBase}/api/v1/attachment/${normalized}/thumb?size=${EMOJI_THUMB_SIZE}`;
};

const getEmojiItemSrc = (item: GalleryItem) => {
  const id = item.attachmentId;
  return resolveEmojiAttachmentUrl(id);
};

const hasEmojiItems = computed(() => emojiItems.value.length > 0);

const emojiPopoverShow = ref(false);
const emojiTriggerButtonRef = ref<HTMLElement | null>(null);
const emojiAnchorElement = ref<HTMLElement | null>(null);
const emojiPopoverX = ref<number | null>(null);
const emojiPopoverY = ref<number | null>(null);
const emojiPopoverXCoord = computed(() => emojiPopoverX.value ?? undefined);
const emojiPopoverYCoord = computed(() => emojiPopoverY.value ?? undefined);
const emojiSearchQuery = ref('');
const isManagingEmoji = ref(false);
const emojiRemarkVisible = computed(() => gallery.emojiRemarkVisible);

// 表情分类选项卡（使用 store 持久化）
const activeEmojiTab = computed({
  get: () => gallery.activeEmojiTabId,
  set: (val) => {
    const userId = user.info?.id;
    if (userId) {
      gallery.setActiveEmojiTab(val, userId);
    }
  }
});
const emojiTabOptions = computed(() => {
  const ids = gallery.allEmojiCollectionIds;
  const ownerId = user.info?.id;
  if (!ownerId) return [];
  const collections = gallery.getCollections(ownerId);
  return ids.map(id => {
    const col = collections.find(c => c.id === id);
    return {
      id,
      name: col?.name || '未知分类',
      isFavorites: id === gallery.favoritesCollectionId
    };
  });
});
const hasMultipleTabs = computed(() => emojiTabOptions.value.length > 1);

const toggleEmojiRemarkVisible = () => {
  const userId = user.info?.id;
  if (!userId) {
    message.warning('请先登录');
    return;
  }
  gallery.setEmojiRemarkVisible(!gallery.emojiRemarkVisible, userId);
};

const resolveEmojiAnchorElement = () => {
  if (typeof window === 'undefined') {
    return null;
  }
  const current = emojiAnchorElement.value;
  if (current && document.body.contains(current)) {
    return current;
  }
  emojiAnchorElement.value = document.querySelector<HTMLElement>('.identity-switcher__avatar');
  return emojiAnchorElement.value;
};

const EMOJI_POPOVER_VERTICAL_OFFSET = 10; // 让弹层靠近头像顶部，避免遮挡

const syncEmojiPopoverPosition = () => {
  const anchor = resolveEmojiAnchorElement() || emojiTriggerButtonRef.value;
  if (!anchor) {
    return false;
  }
  const rect = anchor.getBoundingClientRect();
  emojiPopoverX.value = rect.left;
  emojiPopoverY.value = rect.top + EMOJI_POPOVER_VERTICAL_OFFSET;
  return true;
};

if (typeof window !== 'undefined') {
  useEventListener(window, 'resize', () => {
    if (emojiPopoverShow.value) {
      syncEmojiPopoverPosition();
    }
  });
  useEventListener(
    window,
    'scroll',
    () => {
      if (emojiPopoverShow.value) {
        syncEmojiPopoverPosition();
      }
    },
    { passive: true, capture: true },
  );
}

const allGalleryItems = computed(() =>
  Object.values(gallery.items).flatMap((entry) => entry?.items ?? [])
);

const emojiUsageKey = 'sealchat_emoji_usage';
const emojiUsageMap = ref<Record<string, number>>({});

const ensureEmojiCollectionLoaded = async () => {
  const ownerId = user.info?.id;
  if (!ownerId) {
    return;
  }
  try {
    await gallery.ensureEmojiCollection(ownerId);
  } catch {
    // ignore load errors for emoji collections
  }
};

onMounted(() => {
  try {
    const stored = localStorage.getItem(emojiUsageKey);
    if (stored) emojiUsageMap.value = JSON.parse(stored);
  } catch (e) {
    console.warn('Failed to load emoji usage', e);
  }
  // Check if we should show avatar prompt
  checkAvatarPromptOnMount();
});

const recordEmojiUsage = (id: string) => {
  emojiUsageMap.value[id] = Date.now();
  try {
    localStorage.setItem(emojiUsageKey, JSON.stringify(emojiUsageMap.value));
  } catch (e) {
    console.warn('Failed to save emoji usage', e);
  }
};

const sortByUsage = <T extends { id: string }>(items: T[]): T[] => {
  return [...items].sort((a, b) => {
    const timeA = emojiUsageMap.value[a.id] || 0;
    const timeB = emojiUsageMap.value[b.id] || 0;
    return timeB - timeA;
  });
};

const filteredEmojiItems = computed(() => {
  const query = emojiSearchQuery.value.trim();
  const tabId = activeEmojiTab.value;

  // 根据选项卡筛选
  let items: GalleryItem[];
  if (tabId) {
    items = gallery.getItemsByCollection(tabId);
  } else {
    items = emojiItems.value;
  }

  // 搜索过滤
  const filtered = !query ? items : items.filter((item, idx) => {
    const remark = (item.remark && item.remark.trim()) || `收藏${idx + 1}`;
    return matchText(query, remark);
  });
  return sortByUsage(filtered);
});

const galleryPanelVisible = computed(() => gallery.isPanelVisible);
const channelImagesPanelVisible = computed(() => channelImages.panelVisible);

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n();

// const virtualListRef = ref<InstanceType<typeof VirtualList> | null>(null);
const messagesListRef = ref<HTMLElement | null>(null);
const typingPreviewViewportRef = ref<HTMLElement | null>(null);
const selectionBar = reactive({
  visible: false,
  text: '',
  position: { x: 0, y: 0 },
})
const selectionBarRef = ref<HTMLElement | null>(null)
const selectionMaxLength = 120

const hideSelectionBar = () => {
  selectionBar.visible = false
  selectionBar.text = ''
}

const updateSelectionPosition = (rect: DOMRect) => {
  const width = 220
  const padding = 12
  const gap = 12
  const barHeight = selectionBarRef.value?.offsetHeight ?? 46
  const scrollTop = window.scrollY || document.documentElement.scrollTop || 0
  const x = Math.min(window.innerWidth - width - padding, Math.max(padding, rect.left + rect.width / 2 - width / 2))
  const aboveY = rect.top + scrollTop - barHeight - gap
  const belowY = rect.bottom + scrollTop + gap
  const viewportBottom = scrollTop + window.innerHeight
  const maxY = viewportBottom - barHeight - padding
  const clamped = (value: number) => Math.min(maxY, Math.max(padding, value))
  let targetY = aboveY
  const preferBelow = isMobileUa || window.innerWidth <= 768
  if (preferBelow) {
    targetY = belowY
    if (targetY + barHeight > viewportBottom - padding && aboveY >= padding) {
      targetY = aboveY
    }
  } else if (aboveY < padding) {
    targetY = belowY
  }
  selectionBar.position.x = x
  selectionBar.position.y = clamped(targetY)
}

const handleSelectionChange = () => {
  const container = messagesListRef.value
  if (!container || typeof window === 'undefined') {
    hideSelectionBar()
    return
  }
  const selection = window.getSelection()
  if (!selection || selection.isCollapsed) {
    hideSelectionBar()
    return
  }
  const text = selection.toString().trim()
  if (!text || text.length === 0 || text.length > selectionMaxLength) {
    hideSelectionBar()
    return
  }
  const range = selection.rangeCount ? selection.getRangeAt(0) : null
  if (!range) {
    hideSelectionBar()
    return
  }
  const node = range.commonAncestorContainer instanceof Element ? range.commonAncestorContainer : range.commonAncestorContainer?.parentElement
  if (!node || !container.contains(node)) {
    hideSelectionBar()
    return
  }
  const rect = range.getBoundingClientRect()
  if (rect.width === 0 && rect.height === 0) {
    hideSelectionBar()
    return
  }
  updateSelectionPosition(rect)
  selectionBar.text = text
  selectionBar.visible = true
}

const handlePointerDown = (event: PointerEvent) => {
  if (!selectionBar.visible) {
    return
  }
  const target = event.target as HTMLElement | null
  if (target && selectionBarRef.value?.contains(target)) {
    return
  }
  hideSelectionBar()
}

const handleSelectionCopy = async () => {
  if (!selectionBar.text) return
  const copied = await copyTextWithFallback(selectionBar.text)
  if (copied) {
    message.success('已复制选中文本')
  } else {
    message.error('复制失败')
  }
  hideSelectionBar()
}

const handleSelectionAddKeyword = () => {
  const worldId = chat.currentWorldId
  if (!worldId || !selectionBar.text) return
  const keywordText = selectionBar.text.trim()
  if (!keywordText) {
    hideSelectionBar()
    return
  }
  worldGlossary.openEditor(worldId, null, keywordText)
  nextTick(() => {
    worldGlossary.setQuickPrefill(keywordText)
  })
  hideSelectionBar()
}

const handleSelectionSearch = () => {
  const keyword = selectionBar.text.trim()
  if (!keyword) return
  channelSearch.openPanel()
  channelSearch.setKeyword(keyword)
  channelSearch.bindChannel(chat.curChannel?.id || null)
  void channelSearch.search(chat.curChannel?.id || undefined)
  hideSelectionBar()
}

const canAddKeywordFromSelection = computed(() => selectionBar.visible && canManageWorldKeywords.value && Boolean(chat.currentWorldId))

if (typeof window !== 'undefined') {
  useEventListener(document, 'selectionchange', handleSelectionChange)
  useEventListener(document, 'pointerdown', handlePointerDown, { capture: true })
  useEventListener(window, 'resize', hideSelectionBar)
}

const topSentinelRef = ref<HTMLElement | null>(null);
const bottomSentinelRef = ref<HTMLElement | null>(null);
const textInputRef = ref<any>(null);
const inputMode = ref<'plain' | 'rich'>('plain');
const richContentCache = ref<string | null>(null);
const plainTextFromRichCache = ref<string>('');
const wideInputMode = ref(false);
const isMobileWideInput = computed(() => wideInputMode.value && isMobileUa);
const inputAreaHeightPreview = ref<number | null>(null);
const customInputHeight = computed(() => (
  inputAreaHeightPreview.value !== null
    ? inputAreaHeightPreview.value
    : display.settings.inputAreaHeight
));
const chatInputClassList = computed(() => {
  const classes: string[] = [];
  if (wideInputMode.value) classes.push('chat-input--expanded');
  if (isMobileWideInput.value) classes.push('chat-input--fullscreen');
  if (customInputHeight.value > 0 && !isMobileWideInput.value) classes.push('chat-input--custom-height');
  return classes;
});
const chatInputStyle = computed(() => {
  if (!isMobileWideInput.value && customInputHeight.value > 0) {
    return { '--custom-input-height': `${customInputHeight.value}px` };
  }
  return {};
});
const wideInputTooltip = computed(() => (wideInputMode.value ? '退出广域输入模式' : '进入广域输入模式'));
const toggleWideInputMode = () => {
  wideInputMode.value = !wideInputMode.value;
  // 切换广域模式时清除自定义高度，回到默认的两种高度
  if (customInputHeight.value > 0) {
    display.updateSettings({ inputAreaHeight: 0 });
    inputAreaHeightPreview.value = null;
  }
  nextTick(() => {
    textInputRef.value?.focus?.();
    updateWideInputViewportHeight();
    requestAnimationFrame(updateWideInputViewportHeight);
    window.setTimeout(updateWideInputViewportHeight, 160);
  });
};

const updateWideInputViewportHeight = () => {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  if (!isMobileWideInput.value) {
    document.documentElement.style.removeProperty('--wide-input-height');
    return;
  }
  const viewport = window.visualViewport;
  const height = viewport?.height ?? window.innerHeight;
  document.documentElement.style.setProperty('--wide-input-height', `${Math.round(height)}px`);
};

if (typeof window !== 'undefined') {
  useEventListener(window, 'resize', updateWideInputViewportHeight);
  useEventListener(window, 'orientationchange', updateWideInputViewportHeight);
  if (window.visualViewport) {
    useEventListener(window.visualViewport, 'resize', updateWideInputViewportHeight);
  }
}

watch(isMobileWideInput, () => {
  updateWideInputViewportHeight();
}, { immediate: true });

onBeforeUnmount(() => {
  if (typeof document !== 'undefined') {
    document.documentElement.style.removeProperty('--wide-input-height');
  }
});

// 输入区域高度拖拽调整（通过上边框触发）
const inputContainerRef = ref<HTMLElement | null>(null);
const isResizingInput = ref(false);
const resizeStartY = ref(0);
const resizeStartHeight = ref(0);
const resizePointerId = ref<number | null>(null);
const shouldExitWideInput = ref(false);
const RESIZE_BORDER_THRESHOLD_DESKTOP = 8;
const RESIZE_BORDER_THRESHOLD_MOBILE = 20;

const handleInputBorderPointerDown = (e: PointerEvent) => {
  if (isMobileWideInput.value) return;
  const container = e.currentTarget as HTMLElement;
  if (!container) return;
  const rect = container.getBoundingClientRect();
  const offsetY = e.clientY - rect.top;
  const threshold = e.pointerType === 'touch' ? RESIZE_BORDER_THRESHOLD_MOBILE : RESIZE_BORDER_THRESHOLD_DESKTOP;
  if (offsetY > threshold) return;

  e.preventDefault();
  e.stopPropagation();

  // 在容器上捕获指针
  resizePointerId.value = e.pointerId;
  container.setPointerCapture(e.pointerId);

  isResizingInput.value = true;
  resizeStartY.value = e.clientY;
  const inputEditor = document.querySelector('.chat-input-editor-main') as HTMLElement;
  resizeStartHeight.value = customInputHeight.value > 0
    ? customInputHeight.value
    : (inputEditor?.offsetHeight || INPUT_AREA_HEIGHT_LIMITS.MIN);
  inputAreaHeightPreview.value = resizeStartHeight.value;

  container.addEventListener('pointermove', handleInputResizeMove as EventListener);
  container.addEventListener('pointerup', handleInputResizeEnd as EventListener);
  container.addEventListener('pointercancel', handleInputResizeEnd as EventListener);
  container.addEventListener('lostpointercapture', handleInputResizeEnd as EventListener);
  document.body.style.cursor = 'row-resize';
  document.body.style.userSelect = 'none';
};

const handleInputResizeMove = (e: PointerEvent) => {
  if (!isResizingInput.value) return;
  e.preventDefault();
  const deltaY = resizeStartY.value - e.clientY;
  const rawHeight = resizeStartHeight.value + deltaY;
  if (rawHeight <= INPUT_AREA_HEIGHT_LIMITS.MIN) {
    if (wideInputMode.value) {
      shouldExitWideInput.value = true;
      inputAreaHeightPreview.value = INPUT_AREA_HEIGHT_LIMITS.MIN;
    } else {
      shouldExitWideInput.value = false;
      inputAreaHeightPreview.value = 0;
    }
    return;
  }
  shouldExitWideInput.value = false;
  const newHeight = Math.min(INPUT_AREA_HEIGHT_LIMITS.MAX, rawHeight);
  inputAreaHeightPreview.value = Math.round(newHeight);
};

const handleInputResizeEnd = (e?: PointerEvent) => {
  if (!isResizingInput.value) return;
  isResizingInput.value = false;

  const container = inputContainerRef.value;
  const exitWideInput = shouldExitWideInput.value && wideInputMode.value;
  shouldExitWideInput.value = false;
  const finalHeight = inputAreaHeightPreview.value ?? display.settings.inputAreaHeight;
  inputAreaHeightPreview.value = null;
  if (container) {
    if (resizePointerId.value !== null) {
      try {
        container.releasePointerCapture(resizePointerId.value);
      } catch (_) { /* ignore */ }
    }
    container.removeEventListener('pointermove', handleInputResizeMove as EventListener);
    container.removeEventListener('pointerup', handleInputResizeEnd as EventListener);
    container.removeEventListener('pointercancel', handleInputResizeEnd as EventListener);
    container.removeEventListener('lostpointercapture', handleInputResizeEnd as EventListener);
  }

  resizePointerId.value = null;
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  if (exitWideInput) {
    if (finalHeight !== display.settings.inputAreaHeight) {
      display.updateSettings({ inputAreaHeight: finalHeight });
    }
    wideInputMode.value = false;
    nextTick(() => {
      textInputRef.value?.focus?.();
      updateWideInputViewportHeight();
      requestAnimationFrame(updateWideInputViewportHeight);
      window.setTimeout(updateWideInputViewportHeight, 160);
    });
    return;
  }
  if (finalHeight !== display.settings.inputAreaHeight) {
    display.updateSettings({ inputAreaHeight: finalHeight });
  }
};

const handleInputResizeReset = () => {
  inputAreaHeightPreview.value = null;
  display.updateSettings({ inputAreaHeight: 0 });
};
const inlineImageInputRef = ref<HTMLInputElement | null>(null);
const icHotkeyEnabled = computed(() => {
  const config = display.settings.toolbarHotkeys?.icToggle;
  if (config) {
    return config.enabled !== false;
  }
  return display.settings.enableIcToggleHotkey !== false;
});

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
const inlineImageAltMarkerRegexp = /^图片-([a-zA-Z0-9_-]+)$/;

const buildInlineImageToken = (markerId: string) => `[[图片:${markerId}]]`;

const resolveInlineImageMarkerId = (src?: string, alt?: string) => {
  const altMatch = alt ? alt.match(inlineImageAltMarkerRegexp) : null;
  if (altMatch) {
    return altMatch[1];
  }
  if (src) {
    for (const draft of inlineImages.values()) {
      if (draft.objectUrl && draft.objectUrl === src) {
        return draft.id;
      }
      if (draft.attachmentId) {
        const normalizedSrc = normalizeAttachmentId(src);
        const normalizedDraft = normalizeAttachmentId(draft.attachmentId);
        if (normalizedSrc && normalizedDraft && normalizedSrc === normalizedDraft) {
          return draft.id;
        }
      }
    }
  }
  return nanoid();
};

const buildInlineImageDraftFromRich = (markerId: string, src?: string) => {
  const record: InlineImageDraft = reactive({
    id: markerId,
    token: buildInlineImageToken(markerId),
    status: 'uploaded',
  });
  const raw = (src || '').trim();
  if (!raw) {
    return record;
  }
  if (/^(blob:|data:)/i.test(raw)) {
    record.objectUrl = raw;
    return record;
  }
  if (raw.startsWith('id:') || /^[0-9A-Za-z_-]+$/.test(raw)) {
    record.attachmentId = normalizeAttachmentId(raw);
  }
  return record;
};

const resolveInlineImageSource = (draft?: InlineImageDraft) => {
  if (!draft) {
    return '';
  }
  if (draft.status === 'uploading' && draft.objectUrl) {
    return draft.objectUrl;
  }
  if (draft.attachmentId) {
    return draft.attachmentId.startsWith('id:') ? draft.attachmentId : `id:${draft.attachmentId}`;
  }
  if (draft.objectUrl) {
    return draft.objectUrl;
  }
  return '';
};

const extractRichTextWithImages = (node: any, drafts: Map<string, InlineImageDraft>): string => {
  if (!node) {
    return '';
  }
  if (node.text !== undefined) {
    return node.text;
  }
  if (node.type === 'hardBreak') {
    return '\n';
  }
  if (node.type === 'image') {
    const src = node.attrs?.src || '';
    const alt = node.attrs?.alt || '';
    const markerId = resolveInlineImageMarkerId(src, alt);
    const token = buildInlineImageToken(markerId);
    if (!drafts.has(markerId)) {
      const existing = inlineImages.get(markerId);
      drafts.set(markerId, existing ?? buildInlineImageDraftFromRich(markerId, src));
    }
    return token;
  }
  if (node.content && node.content.length > 0) {
    const childTexts = node.content.map((child: any) => extractRichTextWithImages(child, drafts));
    const joined = childTexts.join('');
    if (node.type === 'paragraph' || node.type === 'heading' || node.type === 'listItem') {
      return joined + '\n';
    }
    return joined;
  }
  return '';
};

const convertRichContentToPlain = (content: string) => {
  const drafts = new Map<string, InlineImageDraft>();
  try {
    const json = JSON.parse(content);
    const text = extractRichTextWithImages(json, drafts).replace(/\n+$/, '');
    return { text, drafts };
  } catch {
    return { text: '', drafts };
  }
};

const applyInlineImageDrafts = (drafts: Map<string, InlineImageDraft>) => {
  inlineImages.forEach((draft, key) => {
    if (!drafts.has(key)) {
      revokeInlineImage(draft);
      inlineImages.delete(key);
    }
  });
  drafts.forEach((draft, key) => {
    if (!inlineImages.has(key)) {
      inlineImages.set(key, draft);
    }
  });
};

const buildRichContentFromPlain = (text: string) => {
  if (!text || (!text.trim() && !containsInlineImageMarker(text))) {
    return {
      type: 'doc',
      content: [{ type: 'paragraph' }],
    };
  }
  const lines = text.split('\n');
  const content = lines.map((line) => {
    inlineImageMarkerRegexp.lastIndex = 0;
    let lastIndex = 0;
    const nodes: Array<{ type: string; text?: string; attrs?: Record<string, string> }> = [];
    let match: RegExpExecArray | null;
    while ((match = inlineImageMarkerRegexp.exec(line)) !== null) {
      if (match.index > lastIndex) {
        nodes.push({ type: 'text', text: line.slice(lastIndex, match.index) });
      }
      const markerId = match[1];
      const draft = inlineImages.get(markerId);
      const src = resolveInlineImageSource(draft);
      if (src) {
        nodes.push({ type: 'image', attrs: { src, alt: `图片-${markerId}` } });
      } else {
        nodes.push({ type: 'text', text: match[0] });
      }
      lastIndex = match.index + match[0].length;
    }
    if (lastIndex < line.length) {
      nodes.push({ type: 'text', text: line.slice(lastIndex) });
    }
    return nodes.length ? { type: 'paragraph', content: nodes } : { type: 'paragraph' };
  });
  return { type: 'doc', content };
};

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
      previewUrl = resolveAttachmentUrl(draft.attachmentId);
    }
    result[key] = {
      status: draft.status,
      previewUrl,
      error: draft.error,
    };
  });
  return result;
});

const identityDialogVisible = ref(false);

watch(
  () => user.info.id,
  async (id) => {
    if (!id) return;
    gallery.loadEmojiPreference(id);
    await ensureEmojiCollectionLoaded();
  },
  { immediate: true }
);

watch(
  () => gallery.emojiCollectionIds,
  (ids) => {
    for (const id of ids) {
      void gallery.loadItems(id);
    }
  },
  { deep: true }
);

watch(emojiPopoverShow, (show, prevShow) => {
  if (!show) {
    isManagingEmoji.value = false;
    emojiSearchQuery.value = '';
  } else {
    nextTick(() => {
      syncEmojiPopoverPosition();
    });
    void ensureEmojiCollectionLoaded();
  }
  if (show) {
    chatEvent.emit('global-overlay-toggle', { source: 'emoji-panel', open: true } as any);
  } else if (prevShow) {
    chatEvent.emit('global-overlay-toggle', { source: 'emoji-panel', open: false } as any);
  }
});

watch(isManagingEmoji, (val) => {
  if (val) {
    void ensureEmojiCollectionLoaded();
  }
});

const openGalleryPanel = async () => {
  const userId = user.info?.id;
  if (!userId) {
    message.warning('请先登录后再打开画廊');
    return;
  }
  try {
    gallery.loadEmojiPreference(userId);
    await gallery.openPanel(userId);
  } catch (error) {
    console.warn('打开画廊失败', error);
    message.error('打开画廊失败，请稍后重试');
  }
};

const openChannelImagesPanel = () => {
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    message.warning('请先选择一个频道');
    return;
  }
  channelImages.openPanel(channelId);
};

const handleChannelImagesLocate = async (payload: { messageId: string; displayOrder?: number; createdAt?: number }) => {
  // 复用搜索跳转逻辑
  await handleSearchJump(payload);
  // 可选：关闭图片查看器
  // channelImages.closePanel();
};

const handleEmojiManageClick = async () => {
  isManagingEmoji.value = !isManagingEmoji.value;
  if (isManagingEmoji.value) {
    emojiPopoverShow.value = false;
    await openGalleryPanel();
  }
};

const handleEmojiTriggerClick = () => {
  if (emojiPopoverShow.value) {
    emojiPopoverShow.value = false;
    return;
  }
  syncEmojiPopoverPosition();
  emojiPopoverShow.value = true;
};


const buildEmojiRemarkMap = () => {
  // 优先使用表情收藏的备注映射，采用"先到先得"策略避免覆盖
  const remarkMap = new Map<string, string>();

  // 先添加表情收藏（优先级最高）
  for (const item of emojiItems.value) {
    const remark = item.remark?.trim();
    if (remark && item.attachmentId && !remarkMap.has(remark)) {
      remarkMap.set(remark, item.attachmentId);
    }
  }

  // 再添加其他画廊条目（不覆盖已存在的）
  for (const item of allGalleryItems.value) {
    const remark = item.remark?.trim();
    if (remark && item.attachmentId && !remarkMap.has(remark)) {
      remarkMap.set(remark, item.attachmentId);
    }
  }

  return remarkMap;
};

const replaceEmojiRemarksForPreview = (text: string): string => {
  const remarkMap = buildEmojiRemarkMap();
  return text.replace(/[\[【\/]([^\]】\/]+)[\]】\/]/g, (match, remark) => {
    const attachmentId = remarkMap.get(remark.trim());
    if (!attachmentId) return match;
    const normalized = attachmentId.startsWith('id:') ? attachmentId.slice(3) : attachmentId;
    return `[[img:id:${normalized}]]`;
  });
};

const replaceEmojiRemarks = (text: string): string => {
  const remarkMap = buildEmojiRemarkMap();
  return text.replace(/[\[【\/]([^\]】\/]+)[\]】\/]/g, (match, remark) => {
    const attachmentId = remarkMap.get(remark.trim());
    if (!attachmentId) return match;

    const normalized = attachmentId.startsWith('id:') ? attachmentId.slice(3) : attachmentId;
    const markerId = nanoid();
    const token = `[[图片:${markerId}]]`;
    const record: InlineImageDraft = reactive({
      id: markerId,
      token,
      status: 'uploaded',
      attachmentId: normalized,
    });
    inlineImages.set(markerId, record);
    return token;
  });
};

const handleSlashInput = (e: InputEvent) => {
  if (inputMode.value === 'rich' || e.inputType !== 'insertText' || e.data !== ' ') return;

  const text = textToSend.value;
  const { start } = captureSelectionRange();
  const before = text.slice(0, start);

  if (before.endsWith('/e ') && (start === 3 || !/[\u4e00-\u9fa5\w]/.test(text[start - 4]))) {
    textToSend.value = text.slice(0, start - 3) + text.slice(start);
    nextTick(() => {
      setInputSelection(start - 3, start - 3);
      emojiPopoverShow.value = true;
    });
  } else if (before.endsWith('/w ') && (start === 3 || !/[\u4e00-\u9fa5\w]/.test(text[start - 4]))) {
    textToSend.value = text.slice(0, start - 3) + text.slice(start);
    nextTick(() => {
      setInputSelection(start - 3, start - 3);
      openWhisperPanel('slash');
    });
  }
};
const identityDialogMode = ref<'create' | 'edit'>('create');
const identityManageVisible = ref(false);
const icOocRoleConfigPanelVisible = ref(false);
const identitySubmitting = ref(false);
const identityForm = reactive({
  displayName: '',
  color: '',
  avatarAttachmentId: '',
  isDefault: false,
  folderIds: [] as string[],
  characterCardId: '' as string,
});
const identityOriginalCardId = ref('');
const identityAvatarPreview = ref('');
const identityAvatarInputRef = ref<HTMLInputElement | null>(null);
const identityAvatarEditorVisible = ref(false);
const identityAvatarEditorFile = ref<File | null>(null);
const editingIdentity = ref<ChannelIdentity | null>(null);
const currentChannelIdentities = computed(() => chat.channelIdentities[chat.curChannel?.id || ''] || []);
const identityFolders = computed(() => chat.channelIdentityFolders[chat.curChannel?.id || ''] || []);
const identityFavoriteFolderIds = computed(() => chat.channelIdentityFavorites[chat.curChannel?.id || ''] || []);
const identityFolderMembership = computed<Record<string, string[]>>(() => chat.channelIdentityMembership[chat.curChannel?.id || ''] || {});
const activeIdentityFolderId = ref<'all' | 'favorites' | 'ungrouped' | string>('all');
const identitySelection = ref<string[]>([]);
const folderActionTarget = ref<string[]>([]);
const folderDialogVisible = ref(false);
const folderDialogMode = ref<'create' | 'rename'>('create');
const folderFormName = ref('');
const folderSubmitting = ref(false);
const editingFolder = ref<ChannelIdentityFolder | null>(null);
const folderActionOptions = [
  { label: '重命名', key: 'rename' },
  { label: '删除', key: 'delete', type: 'error' as const },
];
const folderAssigning = ref(false);
const isNightPalette = computed(() => display.palette === 'night');
const identityDrawerWidth = computed(() => (windowWidth.value <= 640 ? '100%' : Math.min(windowWidth.value * 0.95, 800)));
const isIdentityDrawerMobile = computed(() => windowWidth.value > 0 && windowWidth.value <= 640);

const folderMap = computed<Record<string, ChannelIdentityFolder>>(() => {
  const map: Record<string, ChannelIdentityFolder> = {};
  identityFolders.value.forEach(folder => {
    map[folder.id] = folder;
  });
  return map;
});

const folderSelectOptions = computed(() => identityFolders.value.map(folder => ({ label: folder.name, value: folder.id })));

const characterCardSelectOptions = computed(() => {
  const channelId = chat.curChannel?.id || '';
  const cards = characterCardStore.getCardsByChannel(channelId);
  return [
    { label: '不绑定', value: '' },
    ...cards.map(card => ({ label: card.name, value: card.id })),
  ];
});

const favoriteFolderSet = computed(() => new Set(identityFavoriteFolderIds.value));

const identityCountsByFolder = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {
    __all: currentChannelIdentities.value.length,
    __ungrouped: 0,
    __favorites: 0,
  };
  currentChannelIdentities.value.forEach(identity => {
    const folders = identityFolderMembership.value[identity.id] || [];
    if (!folders.length) {
      counts.__ungrouped += 1;
    }
    let inFavorites = false;
    folders.forEach(folderId => {
      counts[folderId] = (counts[folderId] || 0) + 1;
      if (!inFavorites && favoriteFolderSet.value.has(folderId)) {
        inFavorites = true;
      }
    });
    if (inFavorites) {
      counts.__favorites += 1;
    }
  });
  return counts;
});

const composedIdentityFolders = computed(() => {
  const entries: Array<{ id: string; label: string; count: number; folder?: ChannelIdentityFolder; isFavorite?: boolean; disabled?: boolean }> = [
    { id: 'all', label: '全部角色', count: identityCountsByFolder.value.__all || 0 },
    { id: 'favorites', label: '收藏文件夹', count: identityCountsByFolder.value.__favorites || 0, disabled: !identityFavoriteFolderIds.value.length },
    { id: 'ungrouped', label: '未分组', count: identityCountsByFolder.value.__ungrouped || 0 },
  ];
  identityFolders.value.forEach(folder => {
    entries.push({
      id: folder.id,
      label: folder.name,
      count: identityCountsByFolder.value[folder.id] || 0,
      folder,
      isFavorite: favoriteFolderSet.value.has(folder.id),
    });
  });
  return entries;
});

const filteredIdentities = computed(() => {
  const folderId = activeIdentityFolderId.value;
  if (folderId === 'all') {
    return currentChannelIdentities.value;
  }
  if (folderId === 'ungrouped') {
    return currentChannelIdentities.value.filter(identity => (identityFolderMembership.value[identity.id] || []).length === 0);
  }
  if (folderId === 'favorites') {
    if (!identityFavoriteFolderIds.value.length) {
      return [];
    }
    return currentChannelIdentities.value.filter(identity => (identityFolderMembership.value[identity.id] || []).some(id => favoriteFolderSet.value.has(id)));
  }
  return currentChannelIdentities.value.filter(identity => (identityFolderMembership.value[identity.id] || []).includes(folderId));
});

const isAllIdentitySelected = computed(() => {
  const ids = filteredIdentities.value.map(identity => identity.id);
  if (!ids.length) {
    return false;
  }
  return ids.every(id => identitySelection.value.includes(id));
});

const handleFolderItemClick = (item: { id: string; disabled?: boolean }) => {
  if (item.disabled) {
    return;
  }
  activeIdentityFolderId.value = item.id;
};

const toggleFolderFavorite = async (folder: ChannelIdentityFolder, next: boolean) => {
  if (!chat.curChannel?.id) {
    return;
  }
  try {
    await chat.toggleChannelIdentityFolderFavorite(folder.id, chat.curChannel.id, next);
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '操作失败，请稍后重试';
    message.error(errMsg);
  }
};

const openFolderDialog = (mode: 'create' | 'rename', folder?: ChannelIdentityFolder) => {
  folderDialogMode.value = mode;
  editingFolder.value = folder || null;
  folderFormName.value = folder?.name || '';
  folderDialogVisible.value = true;
};

const submitFolderDialog = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  const name = folderFormName.value.trim();
  if (!name) {
    message.warning('请输入文件夹名称');
    return;
  }
  folderSubmitting.value = true;
  try {
    if (folderDialogMode.value === 'create') {
      await chat.createChannelIdentityFolder(chat.curChannel.id, name);
      message.success('文件夹已创建');
    } else if (editingFolder.value) {
      await chat.updateChannelIdentityFolder(editingFolder.value.id, chat.curChannel.id, { name });
      message.success('文件夹已更新');
    }
    folderDialogVisible.value = false;
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '操作失败，请稍后重试';
    message.error(errMsg);
  } finally {
    folderSubmitting.value = false;
  }
};

const handleFolderAction = async (folder: ChannelIdentityFolder, key: string | number) => {
  if (key === 'rename') {
    openFolderDialog('rename', folder);
    return;
  }
  if (key === 'delete') {
    const confirmed = await dialogAskConfirm(dialog, {
      title: '删除文件夹',
      content: `确定删除「${folder.name}」文件夹吗？其中的角色不会被删除。`,
    });
    if (!confirmed || !chat.curChannel?.id) {
      return;
    }
    try {
      await chat.deleteChannelIdentityFolder(folder.id, chat.curChannel.id);
      message.success('文件夹已删除');
    } catch (error: any) {
      const errMsg = error?.response?.data?.error || '删除失败，请稍后重试';
      message.error(errMsg);
    }
  }
};

const handleIdentitySelection = (identityId: string, checked: boolean) => {
  if (checked) {
    if (!identitySelection.value.includes(identityId)) {
      identitySelection.value = [...identitySelection.value, identityId];
    }
  } else {
    identitySelection.value = identitySelection.value.filter(id => id !== identityId);
  }
};

const toggleSelectAll = (checked: boolean) => {
  if (checked) {
    identitySelection.value = filteredIdentities.value.map(identity => identity.id);
  } else {
    identitySelection.value = [];
  }
};

const ensureSelection = () => {
  if (!identitySelection.value.length) {
    message.warning('请先选择角色');
    return false;
  }
  return true;
};

const ensureFolderTargets = () => {
  if (!folderActionTarget.value.length) {
    message.warning('请选择目标文件夹');
    return false;
  }
  return true;
};

const handleIdentityFolderAssign = async (mode: 'append' | 'replace' | 'remove') => {
  if (!chat.curChannel?.id || !ensureSelection()) {
    return;
  }
  if (!folderActionTarget.value.length) {
    if (mode === 'remove') {
      message.warning('请选择需要移除的文件夹');
    } else if (!ensureFolderTargets()) {
      return;
    }
    return;
  }
  try {
    folderAssigning.value = true;
    await chat.assignIdentitiesToFolders(chat.curChannel.id, identitySelection.value, folderActionTarget.value, mode);
    message.success('角色分组已更新');
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '操作失败，请稍后重试';
    message.error(errMsg);
  } finally {
    folderAssigning.value = false;
  }
};

const handleIdentityFolderClear = async () => {
  if (!chat.curChannel?.id || !ensureSelection()) {
    return;
  }
  try {
    folderAssigning.value = true;
    await chat.assignIdentitiesToFolders(chat.curChannel.id, identitySelection.value, [], 'replace');
    message.success('已移除所选角色的所有文件夹');
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '操作失败，请稍后重试';
    message.error(errMsg);
  } finally {
    folderAssigning.value = false;
  }
};

const resolveFolderName = (folderId: string) => folderMap.value[folderId]?.name || '未命名文件夹';

watch(activeIdentityFolderId, () => {
  const visibleSet = new Set(filteredIdentities.value.map(identity => identity.id));
  identitySelection.value = identitySelection.value.filter(id => visibleSet.has(id));
});

watch(identityFolders, (folders) => {
  const valid = new Set(folders.map(folder => folder.id));
  folderActionTarget.value = folderActionTarget.value.filter(id => valid.has(id));
});

watch(() => chat.curChannel?.id, () => {
  activeIdentityFolderId.value = 'all';
  identitySelection.value = [];
  folderActionTarget.value = [];
});

watch(identityManageVisible, (visible) => {
  if (!visible) {
    identitySelection.value = [];
    folderActionTarget.value = [];
  }
});
let identityAvatarObjectURL: string | null = null;
let identityAvatarFile: File | null = null;
const identityAvatarDisplay = computed(() => identityAvatarPreview.value || resolveAttachmentUrl(identityForm.avatarAttachmentId));

const identityImportInputRef = ref<HTMLInputElement | null>(null);
const identityExporting = ref(false);
const identityImporting = ref(false);
const identitySyncDialogVisible = ref(false);
const identitySyncSourceChannelId = ref<string | null>(null);
const identitySyncing = ref(false);

const flattenSyncChannels = (channels?: SChannel[]): SChannel[] => {
  if (!channels || channels.length === 0) return [];
  const stack = [...channels];
  const result: SChannel[] = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    result.push(node);
    if (node.children && node.children.length > 0) {
      stack.unshift(...node.children);
    }
  }
  return result;
};

const getSyncChannelLabel = (channel: SChannel) => {
  if (!channel) return '未命名频道';
  const base = channel.name || '未命名频道';
  return channel.isPrivate ? `${base}（私密）` : base;
};

const identitySyncChannelOptions = computed(() => {
  const worldId = chat.currentWorldId;
  const worldTree =
    (worldId && chat.channelTreeByWorld?.[worldId]) ||
    chat.channelTree ||
    [];
  return flattenSyncChannels(worldTree as SChannel[])
    .filter(channel => Boolean(channel?.id) && !channel.isPrivate && channel.id !== chat.curChannel?.id)
    .map(channel => ({
      label: getSyncChannelLabel(channel),
      value: channel.id!,
    }));
});

const ensureIdentitySyncOptions = async () => {
  const worldId = chat.currentWorldId;
  if (!worldId) return;
  if (identitySyncChannelOptions.value.length > 0) return;
  try {
    await chat.channelList(worldId, true);
  } catch (error) {
    console.warn('加载频道列表失败', error);
  }
};

const identitySyncPromptPending = ref(false);
const identitySyncDismissedForSession = ref(false);

const isInObserverMode = () => {
  return chat.isObserver || chat.observerMode || !!chat.observerWorldId;
};

const canManageIdentities = () => {
  // 观察者模式不能管理
  if (isInObserverMode()) return false;
  // 检查世界成员角色
  const worldId = chat.currentWorldId;
  if (!worldId) return false;
  const detail = chat.worldDetailMap[worldId];
  const role = detail?.memberRole;
  // 只有 owner、admin、member 可以触发同步弹窗
  return role === 'owner' || role === 'admin' || role === 'member';
};

const maybePromptIdentitySync = async () => {
  // 等待一个微任务周期，确保路由守卫的状态更新已完成
  await Promise.resolve();
  await nextTick();

  if (!canManageIdentities()) {
    return;
  }
  const channelId = chat.curChannel?.id;
  const currentChannel = chat.curChannel as SChannel | undefined;
  if (!channelId || !currentChannel) {
    return;
  }
  if (identitySyncDialogVisible.value || identitySyncPromptPending.value) {
    return;
  }
  if (identitySyncDismissedForSession.value) {
    return;
  }
  if (isPrivateChatChannel(currentChannel) || !chat.currentWorldId) {
    return;
  }
  try {
    await chat.loadChannelIdentities(channelId, true);
  } catch (error) {
    console.warn('加载频道角色失败', error);
    return;
  }
  // 异步操作后再次检查权限
  if (!canManageIdentities()) {
    return;
  }
  const identities = chat.channelIdentities[channelId] || [];
  if (identities.length > 1) {
    return;
  }

  identitySyncPromptPending.value = true;
  const confirmed = await new Promise<boolean>((resolve) => {
    dialog.warning({
      title: '同步其他频道角色？',
      content: '当前频道角色较少且场内/场外未完整配置，是否从本世界其他频道同步？',
      positiveText: '同步',
      negativeText: '暂不',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
    });
  });
  identitySyncPromptPending.value = false;
  if (!confirmed) {
    identitySyncDismissedForSession.value = true;
    return;
  }
  await openIdentityManager();
  await nextTick();
  await openIdentitySyncDialog();
};

const IDENTITY_EXPORT_VERSION = 'sealchat.channel-identity/v2';

interface IdentityAvatarPayload {
  attachmentId?: string;
  hash: string;
  size: number;
  filename?: string;
  mimeType?: string;
  data: string;
}

interface IdentityExportItem {
  sourceId: string;
  displayName: string;
  color: string;
  isDefault: boolean;
  sortOrder: number;
  folderIds?: string[];
  avatar?: IdentityAvatarPayload;
}

interface IdentityExportFolder {
  sourceId: string;
  name: string;
  sortOrder: number;
  isFavorite?: boolean;
}

interface IdentityExportFile {
  version: string;
  generatedAt: string;
  source?: {
    channelId?: string;
    channelName?: string;
    guildId?: string;
  };
  items: IdentityExportItem[];
  folders?: IdentityExportFolder[];
}

const safeFilename = (value: string) => (value || 'channel').replace(/[\\/:*?"<>|]/g, '_');

const handleIdentityExport = async () => {
  if (identityExporting.value) {
    return;
  }
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  const identities = currentChannelIdentities.value;
  if (!identities.length) {
    message.warning('当前频道暂无可导出的角色');
    return;
  }
  const membershipMap = identityFolderMembership.value;
  const folderList = identityFolders.value;
  const favoriteSet = new Set(identityFavoriteFolderIds.value);
  identityExporting.value = true;
  try {
    const items: IdentityExportItem[] = [];
    for (const identity of identities) {
      const item: IdentityExportItem = {
        sourceId: identity.id,
        displayName: identity.displayName,
        color: identity.color,
        isDefault: identity.isDefault,
        sortOrder: identity.sortOrder,
      };
      const folderIds = identity.folderIds?.length ? identity.folderIds : (membershipMap[identity.id] || []);
      if (folderIds.length) {
        item.folderIds = [...folderIds];
      }
      if (identity.avatarAttachmentId) {
        const normalizedId = normalizeAttachmentId(identity.avatarAttachmentId);
        if (normalizedId) {
          const meta = await fetchAttachmentMetaById(identity.avatarAttachmentId);
          if (meta) {
            const resp = await fetch(`${urlBase}/api/v1/attachment/${normalizedId}`, {
              headers: { Authorization: user.token || '' },
            });
            if (!resp.ok) {
              throw new Error(`下载身份头像失败：${resp.status} ${resp.statusText}`);
            }
            const buffer = await resp.arrayBuffer();
            item.avatar = {
              attachmentId: normalizedId,
              hash: meta.hash,
              size: meta.size ?? buffer.byteLength,
              filename: meta.filename || `${safeFilename(identity.displayName || 'identity')}.png`,
              mimeType: resp.headers.get('content-type') || 'application/octet-stream',
              data: arrayBufferToBase64(buffer),
            };
          }
        }
      }
      items.push(item);
    }

    const payload: IdentityExportFile = {
      version: IDENTITY_EXPORT_VERSION,
      generatedAt: new Date().toISOString(),
      source: {
        channelId: chat.curChannel.id,
        channelName: chat.curChannel?.name || '',
        guildId: (chat.curChannel as any)?.guildId || '',
      },
      items,
      folders: folderList.map(folder => ({
        sourceId: folder.id,
        name: folder.name,
        sortOrder: folder.sortOrder,
        isFavorite: favoriteSet.has(folder.id),
      })),
    };

    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' });
    const timestamp = payload.generatedAt.replace(/[:.]/g, '-');
    const filename = `channel-identities-${safeFilename(chat.curChannel?.name || chat.curChannel?.id || 'channel')}-${timestamp}.json`;
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    message.success('频道角色导出完成');
  } catch (error: any) {
    console.error('导出频道角色失败', error);
    message.error(error?.message || '导出失败，请稍后重试');
  } finally {
    identityExporting.value = false;
  }
};

const triggerIdentityImport = () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  if (identityImporting.value) {
    return;
  }
  identityImportInputRef.value?.click();
};

const ensureImportAttachment = async (avatar?: IdentityAvatarPayload | null): Promise<string> => {
  if (!avatar) {
    return '';
  }
  if (!avatar.hash || !avatar.data || !avatar.size) {
    return normalizeAttachmentId(avatar.attachmentId || '');
  }
  try {
    const quickResp = await api.post('api/v1/attachment-upload-quick', {
      hash: avatar.hash,
      size: avatar.size,
      extra: 'channel-identity-avatar',
    });
    const quickId = quickResp.data?.file?.id;
    if (quickId) {
      return quickId;
    }
  } catch (error: any) {
    const msg = error?.response?.data?.message;
    if (!msg || msg !== '此项数据无法进行快速上传') {
      throw error;
    }
  }

  try {
    const bytes = base64ToUint8Array(avatar.data);
    const blob = new Blob([bytes], { type: avatar.mimeType || 'application/octet-stream' });
    const fileName = avatar.filename || `identity-avatar-${avatar.hash.slice(0, 8)}`;
    const file = new File([blob], fileName, { type: avatar.mimeType || 'application/octet-stream' });
    const uploadResult = await uploadImageAttachment(file, { channelId: chat.curChannel?.id });
    return normalizeAttachmentId(uploadResult.attachmentId);
  } catch (error) {
    console.error('上传身份头像失败', error);
    throw error;
  }
};

const handleIdentityImportChange = async (event: Event) => {
  const input = event.target as HTMLInputElement | null;
  const file = input?.files?.[0];
  if (input) {
    input.value = '';
  }
  if (!file) {
    return;
  }
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }

  try {
    const text = await file.text();
    const payload = JSON.parse(text) as IdentityExportFile;
    const compatibleVersions = [IDENTITY_EXPORT_VERSION, 'sealchat.channel-identity/v1'];
    if (!compatibleVersions.includes(payload.version)) {
      throw new Error('无法识别的导入文件版本');
    }
    const items = payload.items || [];
    if (!items.length) {
      message.warning('导入文件中没有可用的频道角色');
      return;
    }
    const confirmed = await dialogAskConfirm(dialog, {
      title: '导入频道角色',
      content: `检测到 ${items.length} 个角色配置，确定导入到当前频道吗？`,
    });
    if (!confirmed) {
      return;
    }

    identityImporting.value = true;
    const folderIdMap = new Map<string, string>();
    if (Array.isArray(payload.folders) && payload.folders.length && chat.curChannel?.id) {
      const sortedFolders = payload.folders.slice().sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0));
      for (const folder of sortedFolders) {
        if (!folder?.name) continue;
        try {
          const created = await chat.createChannelIdentityFolder(chat.curChannel.id, folder.name, folder.sortOrder);
          if (folder.sourceId) {
            folderIdMap.set(folder.sourceId, created.id);
          }
          if (folder.isFavorite) {
            await chat.toggleChannelIdentityFolderFavorite(created.id, chat.curChannel.id, true);
          }
        } catch (error) {
          console.warn('导入文件夹失败', error);
        }
      }
    }

    let successCount = 0;
    for (const item of items) {
      try {
        const avatarId = await ensureImportAttachment(item.avatar);
        const mappedFolderIds = (item.folderIds || [])
          .map(id => folderIdMap.get(id) || '')
          .filter((id): id is string => !!id);
        await chat.channelIdentityCreate({
          channelId: chat.curChannel.id,
          displayName: item.displayName || '',
          color: item.color || '',
          avatarAttachmentId: avatarId,
          isDefault: !!item.isDefault,
          folderIds: mappedFolderIds,
        });
        successCount += 1;
      } catch (error) {
        console.warn('单个角色导入失败', error);
      }
    }

    await chat.loadChannelIdentities(chat.curChannel.id, true);
    if (successCount > 0) {
      message.success(`成功导入 ${successCount} 个频道角色`);
    } else {
      message.warning('未导入任何角色，请检查文件内容');
    }
  } catch (error: any) {
    console.error('导入频道角色失败', error);
    message.error(error?.message || '导入失败，请检查文件内容');
  } finally {
    identityImporting.value = false;
  }
};

const openIdentitySyncDialog = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  identitySyncSourceChannelId.value = null;
  identitySyncDialogVisible.value = true;
  await ensureIdentitySyncOptions();
};

const handleIdentitySync = async (mode: 'overwrite' | 'append') => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  const sourceChannelId = identitySyncSourceChannelId.value;
  const targetChannelId = chat.curChannel.id;
  if (!sourceChannelId) {
    message.warning('请选择要同步的频道');
    return;
  }
  if (sourceChannelId === targetChannelId) {
    message.warning('不能选择当前频道');
    return;
  }
  if (mode === 'overwrite') {
    const confirmed = await dialogAskConfirm(dialog, {
      title: '确认覆盖场内/场外映射？',
      content: '将以导入方式新建角色，并用新角色覆盖场内/场外映射配置',
    });
    if (!confirmed) return;
  }

  identitySyncing.value = true;
  try {
    const sourceIdentities = await chat.loadChannelIdentities(sourceChannelId, true);
    const sourceList = Array.isArray(sourceIdentities) ? sourceIdentities : [];

    if (!sourceList.length) {
      message.warning('所选频道暂无可同步的角色');
      return;
    }

    const targetIdentities = await chat.loadChannelIdentities(targetChannelId, true);
    const targetList = Array.isArray(targetIdentities) ? targetIdentities : [];
    const normalizeIdentityName = (name: string) => name.trim().toLowerCase();
    const targetIdentityByName = new Map<string, (typeof targetList)[number]>();
    const duplicateTargetNames = new Set<string>();
    for (const identity of targetList) {
      const key = normalizeIdentityName(identity.displayName || '');
      if (!key) continue;
      if (targetIdentityByName.has(key)) {
        duplicateTargetNames.add(key);
        continue;
      }
      targetIdentityByName.set(key, identity);
    }

    const sourceFolders = chat.channelIdentityFolders[sourceChannelId] || [];
    const sourceFavorites = new Set(chat.channelIdentityFavorites[sourceChannelId] || []);
    const sourceMembership = chat.channelIdentityMembership[sourceChannelId] || {};
    const folderIdMap = new Map<string, string>();

    if (sourceFolders.length > 0) {
      const sortedFolders = sourceFolders
        .slice()
        .sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0));
      for (const folder of sortedFolders) {
        if (!folder?.name) continue;
        try {
          const created = await chat.createChannelIdentityFolder(targetChannelId, folder.name, folder.sortOrder);
          if (folder.id) {
            folderIdMap.set(folder.id, created.id);
          }
          if (folder.id && sourceFavorites.has(folder.id)) {
            await chat.toggleChannelIdentityFolderFavorite(created.id, targetChannelId, true);
          }
        } catch (error) {
          console.warn('同步文件夹失败', error);
        }
      }
    }

    let createdCount = 0;
    let updatedCount = 0;
    let skippedCount = 0;
    let failedCount = 0;
    let emptyNameCount = 0;
    const processedIdentityByName = new Map<string, string>();

    for (const identity of sourceList) {
      const displayName = (identity.displayName || '').trim();
      if (!displayName) {
        emptyNameCount += 1;
        continue;
      }
      const nameKey = normalizeIdentityName(displayName);
      const matchedIdentity = nameKey ? targetIdentityByName.get(nameKey) : undefined;
      const avatarPayload = identity.avatarAttachmentId
        ? {
            attachmentId: normalizeAttachmentId(identity.avatarAttachmentId),
            hash: '',
            size: 0,
            data: '',
          }
        : null;
      const folderIds = identity.folderIds?.length
        ? identity.folderIds
        : (sourceMembership[identity.id] || []);
      const mappedFolderIds = folderIds
        .map(id => folderIdMap.get(id) || '')
        .filter((id): id is string => !!id);
      try {
        const avatarId = await ensureImportAttachment(avatarPayload);
        if (matchedIdentity) {
          if (mode === 'append') {
            skippedCount += 1;
            continue;
          }
          const updated = await chat.channelIdentityUpdate(matchedIdentity.id, {
            channelId: targetChannelId,
            displayName,
            color: identity.color || '',
            avatarAttachmentId: avatarId,
            isDefault: !!identity.isDefault,
            folderIds: mappedFolderIds,
          });
          if (nameKey) {
            processedIdentityByName.set(nameKey, updated.id);
          }
          updatedCount += 1;
          continue;
        }
        const created = await chat.channelIdentityCreate({
          channelId: targetChannelId,
          displayName,
          color: identity.color || '',
          avatarAttachmentId: avatarId,
          isDefault: !!identity.isDefault,
          folderIds: mappedFolderIds,
        });
        if (nameKey && !targetIdentityByName.has(nameKey)) {
          targetIdentityByName.set(nameKey, created);
        }
        if (nameKey) {
          processedIdentityByName.set(nameKey, created.id);
        }
        createdCount += 1;
      } catch (error) {
        failedCount += 1;
        console.warn('同步单个角色失败', error);
      }
    }

    const resolveMappedIdentityId = (sourceId?: string | null) => {
      if (!sourceId) return null;
      const sourceIdentity = sourceList.find(item => item.id === sourceId);
      if (!sourceIdentity) return null;
      const nameKey = normalizeIdentityName(sourceIdentity.displayName || '');
      if (!nameKey) return null;
      return processedIdentityByName.get(nameKey) || targetIdentityByName.get(nameKey)?.id || null;
    };

    const sourceConfig = chat.getChannelIcOocRoleConfig(sourceChannelId);
    const targetConfig = chat.getChannelIcOocRoleConfig(targetChannelId);
    const mappedIcRoleId = resolveMappedIdentityId(sourceConfig.icRoleId);
    const mappedOocRoleId = resolveMappedIdentityId(sourceConfig.oocRoleId);
    let nextIcRoleId = targetConfig.icRoleId;
    let nextOocRoleId = targetConfig.oocRoleId;
    if (mode === 'overwrite') {
      nextIcRoleId = mappedIcRoleId;
      nextOocRoleId = mappedOocRoleId;
    } else {
      if (!nextIcRoleId && mappedIcRoleId) {
        nextIcRoleId = mappedIcRoleId;
      }
      if (!nextOocRoleId && mappedOocRoleId) {
        nextOocRoleId = mappedOocRoleId;
      }
    }
    const mappingChanged =
      nextIcRoleId !== targetConfig.icRoleId ||
      nextOocRoleId !== targetConfig.oocRoleId;
    if (mappingChanged) {
      chat.setChannelIcOocRoleConfig(targetChannelId, {
        icRoleId: nextIcRoleId,
        oocRoleId: nextOocRoleId,
      });
    }

    await chat.loadChannelIdentities(targetChannelId, true);
    identitySyncDialogVisible.value = false;

    const syncedCount = createdCount + updatedCount;
    const hasAnyWork = syncedCount > 0 || skippedCount > 0 || mappingChanged;
    if (!hasAnyWork) {
      message.warning('没有可同步的角色或映射');
      return;
    }
    const details: string[] = [];
    if (createdCount) details.push(`新增 ${createdCount}`);
    if (updatedCount) details.push(`覆盖 ${updatedCount}`);
    if (skippedCount) details.push(`跳过 ${skippedCount}`);
    if (failedCount) details.push(`失败 ${failedCount}`);
    if (emptyNameCount) details.push(`忽略 ${emptyNameCount} 个无名角色`);
    const mappingNote = mappingChanged ? '，已同步场内/场外映射' : '';
    const detailNote = details.length ? `（${details.join('，')}）` : '';
    const summaryText = syncedCount > 0 ? `已同步 ${syncedCount} 个角色` : '没有新增角色';
    message.success(`${summaryText}${detailNote}${mappingNote}`);
    if (duplicateTargetNames.size > 0) {
      message.warning(`目标频道存在 ${duplicateTargetNames.size} 个重名角色，同步时按第一个匹配处理`);
    }
  } catch (error) {
    console.error('同步频道角色失败', error);
    message.error('同步失败，请稍后重试');
  } finally {
    identitySyncing.value = false;
  }
};

const normalizeHexColor = (value: string) => {
  let color = value.trim().toLowerCase();
  if (!color) return '';
  if (!color.startsWith('#')) {
    color = `#${color}`;
  }
  if (/^#[0-9a-f]{3}$/.test(color)) {
    const [, r, g, b] = color.split('');
    color = `#${r}${r}${g}${g}${b}${b}`;
  }
  if (!/^#[0-9a-f]{6}$/.test(color)) {
    return '';
  }
  return color;
};

const applyIdentityAppearanceToMessages = (identity: ChannelIdentity) => {
  if (!identity || identity.channelId !== chat.curChannel?.id) {
    return;
  }
  const normalizedColor = normalizeHexColor(identity.color || '');
  const avatarAttachment = identity.avatarAttachmentId || '';
  const displayName = identity.displayName || '';
  let updated = false;
  for (const msg of rows.value) {
    const senderIdentityId = (msg as any).sender_identity_id;
    if (senderIdentityId === identity.id) {
      if (displayName) {
        msg.sender_member_name = displayName;
        (msg as any).sender_identity_name = displayName;
      }
      (msg as any).sender_identity_color = normalizedColor;
      (msg as any).sender_identity_avatar_id = avatarAttachment;
      if (!msg.identity) {
        msg.identity = {
          id: identity.id,
          displayName,
          color: normalizedColor,
          avatarAttachment,
        } as any;
      }
      updated = true;
    }
    if (msg.identity?.id === identity.id) {
      msg.identity.displayName = displayName;
      msg.identity.color = normalizedColor;
      msg.identity.avatarAttachment = avatarAttachment;
      updated = true;
    }
    if (msg.quote?.identity?.id === identity.id) {
      msg.quote.identity.displayName = displayName;
      msg.quote.identity.color = normalizedColor;
      msg.quote.identity.avatarAttachment = avatarAttachment;
      updated = true;
    }
    if ((msg.quote as any)?.sender_identity_id === identity.id) {
      (msg.quote as any).sender_identity_color = normalizedColor;
      (msg.quote as any).sender_identity_avatar_id = avatarAttachment;
      if (displayName) {
        msg.quote.sender_member_name = displayName;
      }
      updated = true;
    }
  }
  typingPreviewList.value = typingPreviewList.value.map((item) => {
    if (item.userId === user.info.id) {
      return {
        ...item,
        displayName: displayName || item.displayName,
      };
    }
    return item;
  });
  if (updated) {
    rows.value = [...rows.value];
  }
};

const clearRemovedIdentityFromMessages = (identityId: string) => {
  let updated = false;
  for (const msg of rows.value) {
    if ((msg as any).sender_identity_id === identityId) {
      const fallbackName = msg.member?.nick || msg.user?.nick || msg.user?.name || msg.sender_member_name;
      msg.sender_member_name = fallbackName;
      delete (msg as any).sender_identity_id;
      delete (msg as any).sender_identity_name;
      delete (msg as any).sender_identity_color;
      delete (msg as any).sender_identity_avatar_id;
      if (msg.identity?.id === identityId) {
        msg.identity = undefined;
      }
      updated = true;
    } else if (msg.identity?.id === identityId) {
      msg.identity = undefined;
      updated = true;
    }
    if (msg.quote?.identity?.id === identityId) {
      msg.quote.identity = undefined;
      updated = true;
    }
    if ((msg.quote as any)?.sender_identity_id === identityId) {
      const fallbackQuoteName = msg.quote?.member?.nick || msg.quote?.user?.nick || msg.quote?.user?.name || msg.quote?.sender_member_name;
      if (msg.quote) {
        msg.quote.sender_member_name = fallbackQuoteName;
      }
      delete (msg.quote as any)?.sender_identity_id;
      delete (msg.quote as any)?.sender_identity_name;
      delete (msg.quote as any)?.sender_identity_color;
      delete (msg.quote as any)?.sender_identity_avatar_id;
      updated = true;
    }
  }
  typingPreviewList.value = typingPreviewList.value.map((item) => {
    if (item.userId === user.info.id) {
      return {
        ...item,
        displayName: chat.curMember?.nick || user.info.nick || item.displayName,
      };
    }
    return item;
  });
  if (updated) {
    rows.value = [...rows.value];
  }
};

const handleIdentityColorBlur = () => {
  if (!identityForm.color) {
    return;
  }
  const normalized = normalizeHexColor(identityForm.color);
  if (!normalized) {
    message.warning('颜色格式应为 #RGB 或 #RRGGBB');
    identityForm.color = '';
    return;
  }
  identityForm.color = normalized;
};

const handleIdentityUpdated = (payload?: any) => {
  const identity = payload?.identity as ChannelIdentity | undefined;
  if (identity) {
    if (identity.channelId !== chat.curChannel?.id) {
      return;
    }
    applyIdentityAppearanceToMessages(identity);
  }
  if (payload?.removedId && payload?.channelId === chat.curChannel?.id) {
    clearRemovedIdentityFromMessages(payload.removedId);
  }
};

const revokeIdentityObjectURL = () => {
  if (identityAvatarObjectURL) {
    URL.revokeObjectURL(identityAvatarObjectURL);
    identityAvatarObjectURL = null;
  }
};

const resetIdentityForm = (identity?: ChannelIdentity | null) => {
  revokeIdentityObjectURL();
  identityAvatarFile = null;
  identityForm.displayName = identity?.displayName || '';
  identityForm.color = normalizeHexColor(identity?.color || '') || '';
  identityForm.avatarAttachmentId = identity?.avatarAttachmentId || '';
  identityForm.isDefault = identity?.isDefault ?? (currentChannelIdentities.value.length === 0);
  identityForm.folderIds = identity?.folderIds ? [...identity.folderIds] : [];
  identityForm.characterCardId = identity?.id ? characterCardStore.getBoundCardId(identity.id) || '' : '';
  identityOriginalCardId.value = identityForm.characterCardId;
  identityAvatarPreview.value = resolveAttachmentUrl(identity?.avatarAttachmentId);
};

const openIdentityCreate = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  editingIdentity.value = null;
  identityDialogMode.value = 'create';
  resetIdentityForm(null);
  if (!identityForm.displayName) {
    identityForm.displayName = chat.curMember?.nick || user.info.nick || user.info.username || '';
  }
  // Load character cards for the channel
  await characterCardStore.loadCards(chat.curChannel.id);
  identityForm.characterCardId = '';
  identityOriginalCardId.value = '';
  identityDialogVisible.value = true;
};

const openIdentityEdit = async (identity: ChannelIdentity) => {
  editingIdentity.value = identity;
  identityDialogMode.value = 'edit';
  resetIdentityForm(identity);
  // Load character cards for the channel
  if (chat.curChannel?.id) {
    await characterCardStore.loadCards(chat.curChannel.id);
    identityForm.characterCardId = identity?.id ? characterCardStore.getBoundCardId(identity.id) || '' : '';
    identityOriginalCardId.value = identityForm.characterCardId;
  }
  identityDialogVisible.value = true;
};

const openIdentityManager = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  await chat.loadChannelIdentities(chat.curChannel.id, true);
  identityManageVisible.value = true;
};

const closeIdentityDialog = () => {
  identityDialogVisible.value = false;
};

const handleIdentityAvatarTrigger = () => {
  identityAvatarInputRef.value?.click();
};

const handleIdentityAvatarChange = async (event: Event) => {
  const input = event.target as HTMLInputElement | null;
  if (!input || !input.files?.length) {
    return;
  }
  const file = input.files[0];
  // Check file size before processing
  const sizeLimit = utils.config?.imageSizeLimit ? utils.config.imageSizeLimit * 1024 : utils.fileSizeLimit;
  if (file.size > sizeLimit) {
    const limitMB = (sizeLimit / 1024 / 1024).toFixed(1);
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`);
    input.value = '';
    return;
  }
  // Open avatar editor modal
  identityAvatarEditorFile.value = file;
  identityAvatarEditorVisible.value = true;
  input.value = '';
};

const handleIdentityAvatarEditorSave = async (file: File) => {
  identityForm.avatarAttachmentId = '';
  identityAvatarFile = file;
  revokeIdentityObjectURL();
  identityAvatarObjectURL = URL.createObjectURL(file);
  identityAvatarPreview.value = identityAvatarObjectURL;
  identityAvatarEditorVisible.value = false;
  identityAvatarEditorFile.value = null;
};

const handleIdentityAvatarEditorCancel = () => {
  identityAvatarEditorVisible.value = false;
  identityAvatarEditorFile.value = null;
};

const removeIdentityAvatar = () => {
  identityForm.avatarAttachmentId = '';
  identityAvatarFile = null;
  revokeIdentityObjectURL();
  identityAvatarPreview.value = '';
};

const submitIdentityForm = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  if (!identityForm.displayName.trim()) {
    message.warning('频道昵称不能为空');
    return;
  }
  const rawColor = identityForm.color || '';
  const trimmedColor = rawColor.trim();
  const normalizedColor = trimmedColor ? normalizeHexColor(trimmedColor) : '';
  if (trimmedColor && !normalizedColor) {
    message.warning('颜色格式应为 #RGB 或 #RRGGBB');
    return;
  }
  identityForm.color = normalizedColor;
  identitySubmitting.value = true;
  const payload = {
    channelId: chat.curChannel.id,
    displayName: identityForm.displayName.trim(),
    color: normalizedColor,
    avatarAttachmentId: identityForm.avatarAttachmentId,
    isDefault: identityForm.isDefault,
    folderIds: identityForm.folderIds,
  };
  const wasCreating = identityDialogMode.value === 'create';
  try {
    if (identityAvatarFile) {
      const uploadResult = await uploadImageAttachment(identityAvatarFile, { channelId: chat.curChannel.id });
      const fileToken = uploadResult.attachmentId;
      if (!fileToken) {
        throw new Error('上传失败：未返回附件ID');
      }
      const normalizedToken = normalizeAttachmentId(fileToken);
      identityForm.avatarAttachmentId = normalizedToken;
      payload.avatarAttachmentId = identityForm.avatarAttachmentId;
      identityAvatarPreview.value = resolveAttachmentUrl(fileToken);
      identityAvatarFile = null;
    }
    if (identityDialogMode.value === 'create') {
      const createdIdentity = await chat.channelIdentityCreate(payload);
      // Handle character card binding for new identity
      if (createdIdentity?.id && chat.curChannel?.id && identityForm.characterCardId !== identityOriginalCardId.value) {
        try {
          if (identityForm.characterCardId) {
            await characterCardStore.bindIdentity(chat.curChannel.id, createdIdentity.id, identityForm.characterCardId);
          } else {
            await characterCardStore.unbindIdentity(chat.curChannel.id, createdIdentity.id);
          }
        } catch (e) {
          console.warn('Failed to bind character card', e);
        }
      }
      message.success('频道角色已创建');
    } else if (editingIdentity.value) {
      await chat.channelIdentityUpdate(editingIdentity.value.id, payload);
      // Handle character card binding changes for existing identity
      if (chat.curChannel?.id && identityForm.characterCardId !== identityOriginalCardId.value) {
        try {
          if (identityForm.characterCardId) {
            await characterCardStore.bindIdentity(chat.curChannel.id, editingIdentity.value.id, identityForm.characterCardId);
          } else {
            await characterCardStore.unbindIdentity(chat.curChannel.id, editingIdentity.value.id);
          }
        } catch (e) {
          console.warn('Failed to update character card binding', e);
        }
      }
      message.success('频道角色已更新');
    }
    await chat.loadChannelIdentities(chat.curChannel.id, true);
    identityDialogVisible.value = false;

    // After creating second role, auto-open IC/OOC config panel if auto-switch is enabled
    if (wasCreating && display.settings.autoSwitchRoleOnIcOocToggle) {
      const identities = chat.channelIdentities[chat.curChannel.id] || [];
      if (identities.length === 2) {
        // Brief delay for better UX before opening config panel
        setTimeout(() => {
          icOocRoleConfigPanelVisible.value = true;
        }, 300);
      }
    }
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '保存失败，请稍后重试';
    message.error(errMsg);
  } finally {
    identitySubmitting.value = false;
  }
};

const deleteIdentity = async (identity: ChannelIdentity) => {
  if (!chat.curChannel?.id) {
    return;
  }
  const confirmed = await dialogAskConfirm(dialog, {
    title: '删除频道角色',
    content: `确定要删除「${identity.displayName}」吗？此操作无法撤销。`,
  });
  if (!confirmed) {
    return;
  }
  try {
    await chat.channelIdentityDelete(chat.curChannel.id, identity.id);
    await chat.loadChannelIdentities(chat.curChannel.id, true);
    message.success('已删除频道角色');
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '删除失败，请稍后重试';
    message.error(errMsg);
  }
};

const getMessageDisplayName = (message: any) => {
  // 编辑状态下优先使用编辑预览中的角色名称
  const editingPreview = editingPreviewMap.value[message?.id];
  if (editingPreview?.isSelf && editingPreview.displayName) {
    return editingPreview.displayName;
  }
  return message?.identity?.displayName
    || message?.sender_member_name
    || message?.member?.nick
    || message?.user?.nick
    || message?.user?.name
    || '未知';
};

const getMessageAvatar = (message: any) => {
  // 编辑状态下优先使用编辑预览中的角色头像
  const editingPreview = editingPreviewMap.value[message?.id];
  if (editingPreview?.isSelf && editingPreview.avatar) {
    return editingPreview.avatar;
  }
  const candidates = [
    message?.identity?.avatarAttachment,
    (message as any)?.sender_identity_avatar_id,
    (message as any)?.sender_identity_avatar,
    (message as any)?.senderIdentityAvatarID,
    (message as any)?.senderIdentityAvatarId,
  ];
  for (const id of candidates) {
    if (id) {
      return resolveAttachmentUrl(id);
    }
  }
  return message?.member?.avatar || message?.user?.avatar || '';
};

const getMessageIdentityColor = (message: any) => {
  return normalizeHexColor(message?.identity?.color || message?.sender_identity_color || '') || '';
};

const getMessageTone = (message: any): 'ic' | 'ooc' | 'archived' => {
  if (message?.isArchived || message?.is_archived) {
    return 'archived';
  }
  // 如果正在编辑此消息（自己），使用编辑状态的 icMode
  if (chat.editing && chat.editing.messageId === message?.id) {
    return chat.editing.icMode === 'ooc' ? 'ooc' : 'ic';
  }
  // 如果他人正在编辑此消息，使用编辑预览中的 tone
  const editingPreview = editingPreviewMap.value[message?.id];
  if (editingPreview && !editingPreview.isSelf) {
    return editingPreview.tone === 'ooc' ? 'ooc' : 'ic';
  }
  if (message?.icMode === 'ooc' || message?.ic_mode === 'ooc') {
    return 'ooc';
  }
  return 'ic';
};

const getMessageAuthorId = (message: any): string => {
  return (
    message?.user?.id ||
    message?.member?.user?.id ||
    (message?.member && (message.member as any).user_id) ||
    (message?.member && (message.member as any).userId) ||
    (message as any)?.sender_user_id ||
    (message as any)?.senderUserId ||
    (message as any)?.sender?.id ||
    message?.user_id ||
    ''
  );
};

interface ArchivedPanelMessage {
  id: string;
  content: string;
  createdAt: string;
  archivedAt: string;
  archivedBy: string;
  sender: {
    name: string;
    avatar?: string;
  };
}

const ARCHIVE_PAGE_SIZE = 10;
const archivedMessagesRaw = ref<ArchivedPanelMessage[]>([]);
const archivedMessages = ref<ArchivedPanelMessage[]>([]);
const archivedLoading = ref(false);
const archivedSearchQuery = ref('');
const archivedCurrentPage = ref(1);
const archivedTotalCount = ref(0);

const resolveUserNameById = (userId: string): string => {
  if (!userId) {
    return '未知成员';
  }
  if (userId === user.info.id) {
    return user.info.nick || user.info.name || user.info.username || '我';
  }
  const candidate = chat.curChannelUsers.find((member: any) => member?.id === userId);
  return candidate?.nick || candidate?.name || userId;
};

const toIsoStringOrEmpty = (value: any): string => {
  const timestamp = normalizeTimestamp(value);
  if (timestamp === null) {
    return '';
  }
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? '' : date.toISOString();
};

const toArchivedPanelEntry = (message: Message): ArchivedPanelMessage => {
  return {
    id: message.id || '',
    content: message.content || '',
    createdAt: toIsoStringOrEmpty((message as any).createdAt ?? message.createdAt),
    archivedAt: toIsoStringOrEmpty((message as any).archivedAt ?? message.archivedAt),
    archivedBy: resolveUserNameById((message as any).archivedBy || ''),
    sender: {
      name: getMessageDisplayName(message),
      avatar: getMessageAvatar(message),
    },
  };
};

const filteredArchivedMessages = computed(() => {
  const keyword = archivedSearchQuery.value.trim();
  if (!keyword) {
    return [...archivedMessagesRaw.value];
  }
  return archivedMessagesRaw.value.filter((item) => {
    const fields = [item.content, item.sender?.name, item.archivedBy];
    return fields.some((field) => (field ? matchText(keyword, field) : false));
  });
});

const archivedPageCount = computed(() => {
  const total = filteredArchivedMessages.value.length;
  if (total === 0) {
    return 1;
  }
  return Math.max(1, Math.ceil(total / ARCHIVE_PAGE_SIZE));
});

const updateArchivedDisplay = () => {
  const totalPages = archivedPageCount.value;
  if (archivedCurrentPage.value > totalPages) {
    archivedCurrentPage.value = totalPages;
    return;
  }
  if (archivedCurrentPage.value < 1) {
    archivedCurrentPage.value = 1;
    return;
  }
  const start = (archivedCurrentPage.value - 1) * ARCHIVE_PAGE_SIZE;
  const end = start + ARCHIVE_PAGE_SIZE;
  archivedMessages.value = filteredArchivedMessages.value.slice(start, end);
  archivedTotalCount.value = filteredArchivedMessages.value.length;
};

watch(
  [filteredArchivedMessages, archivedCurrentPage],
  () => {
    updateArchivedDisplay();
  },
  { immediate: true },
);

const handleIdentityMenuOpen = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  await chat.loadChannelIdentities(chat.curChannel.id, false);
  const current = chat.getActiveIdentity(chat.curChannel.id);
  if (current) {
    openIdentityEdit(current);
  } else {
    openIdentityCreate();
  }
};

const handleArchiveMessages = async (messageIds: string[]) => {
  try {
    await chat.archiveMessages(messageIds);
    message.success('消息已归档');
    if (archiveDrawerVisible.value) {
      await fetchArchivedMessages();
    }
    await fetchLatestMessages();
  } catch (error) {
    const errMsg = (error as Error)?.message || '归档失败';
    message.error(errMsg);
  }
};

const handleUnarchiveMessages = async (messageIds: string[]) => {
  try {
    await chat.unarchiveMessages(messageIds);
    message.success('消息已恢复');
    if (archiveDrawerVisible.value) {
      await fetchArchivedMessages();
    }
    await fetchLatestMessages();
  } catch (error) {
    const errMsg = (error as Error)?.message || '恢复失败';
    message.error(errMsg);
  }
};

const delay = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

const logUploadConfig = computed(() => utils.config?.logUpload);
const canUseCloudUpload = computed(() => !!logUploadConfig.value?.endpoint && logUploadConfig.value?.enabled !== false);

type CloudUploadResult = {
  url?: string;
  name?: string;
  file_name?: string;
  uploaded_at?: number;
};

const showCloudUploadDialog = (payload: CloudUploadResult) => {
  if (!payload?.url) {
    return;
  }
  const fileLabel = payload.name || payload.file_name || 'log-zlib-compressed';
  const uploadedLabel = payload.uploaded_at ? new Date(payload.uploaded_at).toLocaleString() : '';
  dialog.success({
    title: '云端日志已上传',
    positiveText: '知道了',
    content: () => (
      <div class="cloud-upload-result">
        <p>文件：{fileLabel}</p>
        <p>
          链接：
          <a href={payload.url} target="_blank" rel="noopener">
            {payload.url}
          </a>
        </p>
        {uploadedLabel ? <p>上传时间：{uploadedLabel}</p> : null}
      </div>
    ),
  });
};

const pollExportTask = async (taskId: string, opts?: { autoUpload?: boolean; format?: string }) => {
  const maxAttempts = 30;
  const interval = 2000;
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    try {
      const status = await chat.getExportTaskStatus(taskId);
      if (status.status === 'done') {
        message.success('导出完成，正在下载文件');
        const { blob, fileName } = await chat.downloadExportResult(taskId, status.file_name);
        triggerBlobDownload(blob, fileName);
        if (opts?.autoUpload) {
          try {
            const uploadResp = await chat.uploadExportTask(taskId);
            if (uploadResp?.url) {
              showCloudUploadDialog(uploadResp);
            } else {
              message.warning('云端染色返回结果异常，未提供链接');
            }
          } catch (error: any) {
            const errMsg = error?.response?.data?.error || (error as Error)?.message || '未知错误';
            message.warning(`云端染色上传失败：${errMsg}`);
          }
        }
        return;
      }
      if (status.status === 'failed') {
        message.error(status.message || '导出任务失败');
        return;
      }
    } catch (error) {
      console.error('查询导出状态失败', error);
    }
    await delay(interval);
  }
  message.warning('导出仍在处理，请稍后再试或重新发起下载请求');
};

const EXPORT_SLICE_LIMIT_MIN = 1000;
const EXPORT_SLICE_LIMIT_MAX = 20000;
const EXPORT_CONCURRENCY_MIN = 1;
const EXPORT_CONCURRENCY_MAX = 8;
const EXPORT_SLICE_LIMIT_DEFAULT = 5000;
const EXPORT_CONCURRENCY_DEFAULT = 2;

const clampExportValue = (value: number | undefined, min: number, max: number, fallback: number) => {
  const parsed = Number(value ?? fallback);
  if (!Number.isFinite(parsed)) {
    return fallback;
  }
  const rounded = Math.round(parsed);
  if (rounded < min) return min;
  if (rounded > max) return max;
  return rounded;
};

const handleExportMessages = async (params: {
  format: string;
  displayName?: string;
  timeRange: [number, number] | null;
  includeOoc: boolean;
  includeArchived: boolean;
  withoutTimestamp: boolean;
  mergeMessages: boolean;
  textColorizeBBCode: boolean;
  autoUpload: boolean;
  maxExportMessages: number;
  maxExportConcurrency: number;
}) => {
  if (!chat.curChannel?.id) {
    message.error('请选择需要导出的频道');
    return;
  }
  try {
    const sliceLimit = clampExportValue(
      params.maxExportMessages,
      EXPORT_SLICE_LIMIT_MIN,
      EXPORT_SLICE_LIMIT_MAX,
      display.settings.maxExportMessages ?? EXPORT_SLICE_LIMIT_DEFAULT,
    );
    const maxConcurrency = clampExportValue(
      params.maxExportConcurrency,
      EXPORT_CONCURRENCY_MIN,
      EXPORT_CONCURRENCY_MAX,
      display.settings.maxExportConcurrency ?? EXPORT_CONCURRENCY_DEFAULT,
    );
    const displayOptions = { ...display.settings };

    const payload = {
      channelId: chat.curChannel.id,
      format: params.format,
      displayName: params.displayName?.trim() || undefined,
      timeRange: params.timeRange ?? undefined,
      includeOoc: params.includeOoc,
      includeArchived: params.includeArchived,
      withoutTimestamp: params.withoutTimestamp,
      mergeMessages: params.mergeMessages,
      textColorizeBBCode: params.textColorizeBBCode && params.format === 'txt',
      sliceLimit,
      maxConcurrency,
      displaySettings: displayOptions,
    };
    const result = await chat.createExportTask(payload);
    message.info(`导出任务已创建（#${result.task_id}），正在生成文件…`);
    exportDialogVisible.value = false;
    const shouldAutoUpload = Boolean(params.autoUpload && params.format === 'json' && canUseCloudUpload.value);
    void pollExportTask(result.task_id, { autoUpload: shouldAutoUpload, format: params.format });
  } catch (error: any) {
    console.error('导出失败', error);
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '导出失败';
    message.error(errMsg);
  }
};

const handleArchivePageChange = (page: number) => {
  archivedCurrentPage.value = page;
};

const handleArchiveSearchChange = (keyword: string) => {
  archivedSearchQuery.value = keyword;
  archivedCurrentPage.value = 1;
};

const fetchArchivedMessages = async () => {
  if (!chat.curChannel?.id) {
    archivedMessagesRaw.value = [];
    archivedMessages.value = [];
    archivedTotalCount.value = 0;
    return;
  }
  archivedLoading.value = true;
  try {
    const resp = await chat.messageList(chat.curChannel.id, undefined, {
      includeArchived: true,
      archivedOnly: true,
      includeOoc: true,
    });
    const items = resp?.data ?? [];
    const mapped = items
      .map((item: any) => normalizeMessageShape(item))
      .map((item: Message) => toArchivedPanelEntry(item))
      .sort((a, b) => (normalizeTimestamp(b.archivedAt) ?? 0) - (normalizeTimestamp(a.archivedAt) ?? 0));
    archivedMessagesRaw.value = mapped;
    archivedCurrentPage.value = 1;
  } catch (error) {
    console.error('加载归档消息失败', error);
    if (archiveDrawerVisible.value) {
      message.error('加载归档消息失败');
    }
  } finally {
    archivedLoading.value = false;
  }
};

watch(archiveDrawerVisible, (visible) => {
  if (visible) {
    archivedSearchQuery.value = '';
    archivedCurrentPage.value = 1;
    void fetchArchivedMessages();
  }
});

watch(() => chat.curChannel?.id, () => {
  archivedMessagesRaw.value = [];
  archivedMessages.value = [];
  archivedSearchQuery.value = '';
  archivedCurrentPage.value = 1;
  archivedTotalCount.value = 0;
});

const SCROLL_STICKY_THRESHOLD = 200;
const INITIAL_MESSAGE_LOAD_LIMIT = 30;
const PAGINATED_MESSAGE_LOAD_LIMIT = 20;
const SEARCH_ANCHOR_WINDOW_LIMIT = 10;
const SEARCH_JUMP_LIMIT_PRIMARY = 30;
const SEARCH_JUMP_LIMIT_RETRY = 50;
const HISTORY_PAGINATION_WINDOW_MS = 5 * 60 * 1000;
const HISTORY_WINDOW_EXPANSION_LIMIT = 5;

type ViewMode = 'live' | 'history';

const rows = ref<Message[]>([]);
const listRevision = ref(0);
const messageWindow = reactive({
  viewMode: 'live' as ViewMode,
  anchorMessageId: null as string | null,
  beforeCursor: '',
  afterCursor: '',
  loadingLatest: false,
  loadingBefore: false,
  loadingAfter: false,
  autoFillPending: false,
  earliestTimestamp: null as number | null,
  latestTimestamp: null as number | null,
  hasReachedStart: false,
  hasReachedLatest: false,
  lockedHistory: false,
  beforeCursorExhausted: false,
});
const viewMode = computed(() => messageWindow.viewMode);
const inHistoryMode = computed(() => viewMode.value === 'history');
const historyLocked = computed(() => messageWindow.lockedHistory);
const anchorMessageId = computed(() => messageWindow.anchorMessageId);

interface ResetWindowOptions {
  preserveRows?: boolean;
  preserveHistoryLock?: boolean;
}

const resetWindowState = (mode: ViewMode = 'live', options: ResetWindowOptions = {}) => {
  if (!options.preserveRows) {
    rows.value = [];
  }
  messageWindow.viewMode = mode;
  if (!options.preserveHistoryLock) {
    messageWindow.lockedHistory = false;
  }
  messageWindow.anchorMessageId = null;
  messageWindow.beforeCursor = '';
  messageWindow.beforeCursorExhausted = false;
  messageWindow.afterCursor = '';
  messageWindow.autoFillPending = false;
  messageWindow.earliestTimestamp = null;
  messageWindow.latestTimestamp = null;
  messageWindow.hasReachedStart = false;
  messageWindow.hasReachedLatest = false;
};

const updateViewMode = (mode: ViewMode, { force } = { force: false }) => {
  if (mode === 'live' && messageWindow.lockedHistory && !force) {
    return;
  }
  if (messageWindow.viewMode !== mode) {
    messageWindow.viewMode = mode;
  }
  if (mode === 'live') {
    messageWindow.lockedHistory = false;
  }
};

const lockHistoryView = () => {
  messageWindow.lockedHistory = true;
  updateViewMode('history', { force: true });
};

const unlockHistoryView = () => {
  messageWindow.lockedHistory = false;
  updateViewMode('live', { force: true });
  updateAnchorMessage(null);
};

const updateAnchorMessage = (id: string | null) => {
  messageWindow.anchorMessageId = id || null;
};

const applyCursorUpdate = (cursor?: { before?: string | null; after?: string | null }) => {
  if (!cursor) return;
  if (cursor.before !== undefined) {
    messageWindow.beforeCursor = cursor.before || '';
    messageWindow.beforeCursorExhausted = !messageWindow.beforeCursor;
    if (messageWindow.beforeCursor) {
      messageWindow.hasReachedStart = false;
    }
  }
  if (cursor.after !== undefined) {
    messageWindow.afterCursor = cursor.after || '';
    if (messageWindow.afterCursor) {
      messageWindow.hasReachedLatest = false;
    }
  }
};

watch(viewMode, (mode) => {
  if (mode === 'live') {
    updateAnchorMessage(null);
  }
});

const updateWindowAnchorsFromRows = () => {
  if (!rows.value.length) {
    messageWindow.earliestTimestamp = null;
    messageWindow.latestTimestamp = null;
    messageWindow.afterCursor = '';
    return;
  }
  const firstTs = normalizeTimestamp(rows.value[0]?.createdAt);
  const lastTs = normalizeTimestamp(rows.value[rows.value.length - 1]?.createdAt);
  if (firstTs !== null) {
    messageWindow.earliestTimestamp = firstTs;
  }
  if (lastTs !== null) {
    if (messageWindow.latestTimestamp === null || lastTs > messageWindow.latestTimestamp) {
      messageWindow.hasReachedLatest = false;
    }
    messageWindow.latestTimestamp = lastTs;
    messageWindow.afterCursor = String(lastTs);
  } else {
    messageWindow.afterCursor = '';
  }
};
interface VisibleRowEntry {
  message: Message;
  mergedWithPrev: boolean;
  entryKey: string;
}

const isMergeCandidate = (message?: Message | null) => {
  if (!message) return false;
  if ((message as any).is_revoked || (message as any).is_deleted) {
    return false;
  }
  if (message.isWhisper || (message as any).is_whisper) {
    return false;
  }
  return true;
};

const ROLELESS_FILTER_ID = '__roleless__';

const normalizeRoleFilterState = (roleIds?: string[]) => {
  const raw = Array.isArray(roleIds) ? roleIds : [];
  const normalized = raw
    .map((id) => String(id ?? '').trim())
    .filter((id) => id.length > 0);
  const includeRoleless = normalized.includes(ROLELESS_FILTER_ID);
  const filteredRoleIds = normalized.filter((id) => id !== ROLELESS_FILTER_ID);
  return { roleIds: filteredRoleIds, includeRoleless };
};

const roleFilterState = computed(() => normalizeRoleFilterState(chat.filterState.roleIds));
const roleFilterActive = computed(() => {
  const { roleIds, includeRoleless } = roleFilterState.value;
  return roleIds.length > 0 || includeRoleless;
});
const roleFilterSignature = computed(() => {
  const { roleIds, includeRoleless } = roleFilterState.value;
  return `${includeRoleless ? '1' : '0'}:${roleIds.join(',')}`;
});
const buildRoleFilterOptions = () => {
  const { roleIds, includeRoleless } = roleFilterState.value;
  if (!roleIds.length && !includeRoleless) {
    return {};
  }
  return { roleIds, includeRoleless };
};

const visibleRowEntries = computed<VisibleRowEntry[]>(() => {
  const { icFilter, showArchived } = chat.filterState;
  const { roleIds: filterRoleIds, includeRoleless } = roleFilterState.value;
  const allowMergeNeighbors = display.settings.mergeNeighbors && !roleFilterActive.value;

  const filtered = rows.value.filter((message) => {
    if ((message as any).is_deleted) {
      return false;
    }
    const isArchived = Boolean(message?.isArchived || message?.is_archived);
    if (!showArchived && isArchived) {
      return false;
    }

    const icValue = String(message?.icMode ?? message?.ic_mode ?? 'ic').toLowerCase();
    if (icFilter === 'ic' && icValue !== 'ic') {
      return false;
    }
    if (icFilter === 'ooc' && icValue !== 'ooc') {
      return false;
    }

    if (filterRoleIds.length > 0 || includeRoleless) {
      const roleKey = getMessageRoleIdentityKey(message);
      if (roleKey) {
        if (!filterRoleIds.includes(roleKey)) {
          return false;
        }
      } else if (!includeRoleless) {
        return false;
      }
    }

    return true;
  });

  let lastMergeCandidate: { message: Message; index: number } | null = null;
  return filtered.map((message, index) => {
    let merged = false;
    if (
      allowMergeNeighbors &&
      lastMergeCandidate &&
      isMergeCandidate(message) &&
      index - lastMergeCandidate.index === 1 &&
      shouldMergeMessages(lastMergeCandidate.message, message)
    ) {
      merged = true;
    }
    if (isMergeCandidate(message)) {
      lastMergeCandidate = { message, index };
    } else {
      lastMergeCandidate = null;
    }
    const idPart = message.id || `temp-${index}`;
    return {
      message,
      mergedWithPrev: merged,
      entryKey: `${idPart}-${index}-${merged ? 1 : 0}`,
    };
  });
});
const visibleRows = computed(() => visibleRowEntries.value.map((entry) => entry.message));

const getMessageRoleIdentityKey = (message: any): string => {
  return (
    message?.senderRoleId ||
    message?.sender_role_id ||
    (message as any)?.sender_identity_id ||
    message?.identity?.id ||
    ''
  );
};

const getMessageRoleKey = (message: any): string => {
  return (
    message?.senderRoleId ||
    message?.sender_role_id ||
    (message as any)?.sender_identity_id ||
    message?.identity?.id ||
    message?.member?.id ||
    message?.member?.member_id ||
    message?.sender_member_id ||
    getMessageAuthorId(message)
  );
};

const getMessageSceneKey = (message: any): string => {
  return String(message?.icMode ?? message?.ic_mode ?? 'ic').toLowerCase();
};

const shouldMergeMessages = (prev?: Message, current?: Message) => {
  if (!prev || !current) return false;
  if (prev.isWhisper !== current.isWhisper) return false;
  const roleSame = getMessageRoleKey(prev) && getMessageRoleKey(prev) === getMessageRoleKey(current);
  if (!roleSame) return false;
  return getMessageSceneKey(prev) === getMessageSceneKey(current);
};


const normalizeTimestamp = (value: any): number | null => {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : null;
  }
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) {
      return null;
    }
    const numeric = Number(trimmed);
    if (!Number.isNaN(numeric)) {
      return numeric;
    }
    const parsed = Date.parse(trimmed);
    return Number.isNaN(parsed) ? null : parsed;
  }
  if (value instanceof Date) {
    const ms = value.getTime();
    return Number.isNaN(ms) ? null : ms;
  }
  return null;
};

const normalizeMessageShape = (msg: any): Message => {
  if (!msg) {
    return msg as Message;
  }
  // 统一主键，避免不同接口返回 message_id/_id 导致重复插入
  if (!msg.id) {
    msg.id = msg.message_id || msg.messageId || msg._id || '';
  }
  if (msg.id && typeof msg.id !== 'string') {
    msg.id = String(msg.id);
  }
  if (msg.isEdited === undefined && msg.is_edited !== undefined) {
    msg.isEdited = msg.is_edited;
  }
  if (msg.editCount === undefined && msg.edit_count !== undefined) {
    msg.editCount = msg.edit_count;
  }
  if (msg.editedByUserId === undefined && msg.edited_by_user_id !== undefined) {
    msg.editedByUserId = msg.edited_by_user_id;
  }
  if (msg.editedByUserName === undefined && msg.edited_by_user_name !== undefined) {
    msg.editedByUserName = msg.edited_by_user_name;
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
  if (msg.whisperToIds === undefined && msg.whisper_to_ids !== undefined) {
    msg.whisperToIds = msg.whisper_to_ids;
  }
  if (msg.whisperToIds === undefined && msg.whisper_targets !== undefined) {
    msg.whisperToIds = msg.whisper_targets;
  }
  if (msg.whisperMeta === undefined && msg.whisper_meta !== undefined) {
    msg.whisperMeta = msg.whisper_meta;
  }
  if (msg.isDeleted === undefined && msg.is_deleted !== undefined) {
    msg.isDeleted = msg.is_deleted;
  }

  if (msg.senderRoleId === undefined && msg.sender_role_id !== undefined) {
    msg.senderRoleId = msg.sender_role_id;
  }
  if (!msg.senderRoleId) {
    const fallbackRoleId = msg.sender_role_id || (msg as any)?.sender_identity_id || msg.identity?.id || '';
    if (fallbackRoleId) {
      msg.senderRoleId = fallbackRoleId;
    }
  }
  if (!msg.sender_role_id && msg.senderRoleId) {
    msg.sender_role_id = msg.senderRoleId;
  }
  const mergeLegacyWhisperMeta = () => {
    const legacyPairs: Array<[keyof WhisperMeta, any]> = [
      ['senderMemberId', msg.whisper_sender_member_id],
      ['senderMemberName', msg.whisper_sender_member_name],
      ['senderUserNick', msg.whisper_sender_user_nick],
      ['senderUserName', msg.whisper_sender_user_name],
      ['targetMemberId', msg.whisper_target_member_id],
      ['targetMemberName', msg.whisper_target_member_name],
      ['targetUserNick', msg.whisper_target_user_nick],
      ['targetUserName', msg.whisper_target_user_name],
    ];
    const extracted: Partial<WhisperMeta> = {};
    let hasValue = false;
    legacyPairs.forEach(([key, value]) => {
      if (value === null || value === undefined) {
        return;
      }
      const text = typeof value === 'string' ? value.trim() : value;
      if (text === '' || text === false) {
        return;
      }
      (extracted as any)[key] = value;
      hasValue = true;
    });
    if (!hasValue) {
      return;
    }
    const meta = { ...(msg.whisperMeta || {}) };
    Object.entries(extracted).forEach(([key, value]) => {
      if (value === undefined || value === null || value === '') {
        return;
      }
      if (!meta[key]) {
        meta[key] = value;
      }
    });
    if (!meta.targetUserId && msg.whisper_to) {
      meta.targetUserId = msg.whisper_to;
    }
    if (!meta.targetUserIds) {
      const candidateList = msg.whisperToIds || msg.whisper_to_ids || msg.whisper_targets;
      if (Array.isArray(candidateList)) {
        const ids = candidateList.map((entry: any) => {
          if (!entry) return '';
          if (typeof entry === 'string') return entry;
          return entry.id || '';
        }).filter((id: string) => id);
        if (ids.length > 0) {
          meta.targetUserIds = ids;
        }
      }
    }
    if (!meta.senderUserId && msg.user?.id) {
      meta.senderUserId = msg.user.id;
    }
    if (Object.keys(meta).length > 0) {
      msg.whisperMeta = meta;
    }
  };
  mergeLegacyWhisperMeta();
  if (msg.isWhisper === undefined && msg.is_whisper !== undefined) {
    msg.isWhisper = Boolean(msg.is_whisper);
  } else if (msg.isWhisper !== undefined) {
    msg.isWhisper = Boolean(msg.isWhisper);
  }
  if (msg.isArchived === undefined && msg.is_archived !== undefined) {
    msg.isArchived = msg.is_archived;
  }
  if (msg.archivedAt === undefined && msg.archived_at !== undefined) {
    msg.archivedAt = msg.archived_at;
  }
  if (msg.archivedBy === undefined && msg.archived_by !== undefined) {
    msg.archivedBy = msg.archived_by;
  }
  if ((msg as any).displayOrder === undefined && (msg as any).display_order !== undefined) {
    (msg as any).displayOrder = Number((msg as any).display_order);
  } else if ((msg as any).displayOrder !== undefined) {
    (msg as any).displayOrder = Number((msg as any).displayOrder);
  }

  const normalizedCreatedAt = normalizeTimestamp(msg.createdAt);
  msg.createdAt = normalizedCreatedAt ?? undefined;
  const normalizedUpdatedAt = normalizeTimestamp(msg.updatedAt);
  msg.updatedAt = normalizedUpdatedAt ?? undefined;
  const normalizedArchivedAt = normalizeTimestamp(msg.archivedAt);
  msg.archivedAt = normalizedArchivedAt ?? undefined;

  if (msg.quote) {
    msg.quote = normalizeMessageShape(msg.quote);
  }
  if (Array.isArray((msg as any).reactions) && msg.id) {
    chat.setMessageReactions(msg.id, (msg as any).reactions);
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

const getMessageDisplayOrderValue = (message?: Message): number | null => {
  if (!message) {
    return null;
  }
  const raw = (message as any)?.displayOrder ?? message?.createdAt ?? null;
  if (raw === null || raw === undefined) {
    return null;
  }
  const value = Number(raw);
  return Number.isFinite(value) ? value : null;
};

const deriveLocalDisplayOrder = (list: Message[], index: number, fallback: number) => {
  const prevOrder = getMessageDisplayOrderValue(list[index - 1]);
  const nextOrder = getMessageDisplayOrderValue(list[index + 1]);
  if (prevOrder !== null && nextOrder !== null) {
    return (prevOrder + nextOrder) / 2;
  }
  if (prevOrder !== null) {
    return prevOrder + 1;
  }
  if (nextOrder !== null) {
    return nextOrder - 1;
  }
  return fallback;
};

const localReorderOps = new Set<string>();

const messageRowRefs = new Map<string, HTMLElement>();
const SEARCH_JUMP_WINDOWS_MS = [30, 120, 360, 1440, 10080].map((minutes) => minutes * 60 * 1000);
const searchJumping = ref(false);

interface SearchJumpWindow {
  messages: Message[];
  cursorBefore?: string | null;
  fromTime?: number;
}

const searchHighlightIds = ref(new Set<string>());
const searchHighlightTimers = new Map<string, number>();

const setMessageHighlight = (messageId: string, duration = 4000) => {
  if (!messageId) return;
  if (searchHighlightTimers.has(messageId)) {
    window.clearTimeout(searchHighlightTimers.get(messageId));
  }
  const next = new Set(searchHighlightIds.value);
  next.add(messageId);
  searchHighlightIds.value = next;
  const timer = window.setTimeout(() => {
    const updated = new Set(searchHighlightIds.value);
    updated.delete(messageId);
    searchHighlightIds.value = updated;
    searchHighlightTimers.delete(messageId);
  }, duration);
  searchHighlightTimers.set(messageId, timer);
};
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

const messageExistsLocally = (id: string) => rows.value.some((msg) => msg.id === id);

const mergeIncomingMessages = (items: Message[], cursor?: { before?: string | null; after?: string | null }) => {
  if (!Array.isArray(items) || items.length === 0) {
    return;
  }
  const nextRows = rows.value.slice();
  const prevFirst = nextRows[0];
  let mutated = false;
  items.forEach((incoming) => {
    if (!incoming || !incoming.id) {
      return;
    }
    const index = nextRows.findIndex((msg) => msg.id === incoming.id);
    if (index >= 0) {
      nextRows[index] = {
        ...nextRows[index],
        ...incoming,
      };
    } else {
      nextRows.push(incoming);
    }
    mutated = true;
  });
  if (!mutated) {
    return;
  }
  const sorted = nextRows.sort(compareByDisplayOrder);
  rows.value = sorted;
  computeAfterCursorFromRows();
  if (cursor) {
    if (cursor.before !== undefined) {
      const newFirst = sorted[0];
      const prevFirstOrder = prevFirst ? compareByDisplayOrder(newFirst, prevFirst) : -1;
      if (!prevFirst || prevFirstOrder < 0) {
        messageWindow.beforeCursor = cursor.before || '';
      }
    }
    if (cursor.after !== undefined) {
      messageWindow.afterCursor = cursor.after || '';
    }
  }
};

const loadSearchJumpWindow = async (from: number, to: number, limit: number) => {
  const resp = await chat.messageListDuring(chat.curChannel!.id, from, to, {
    includeArchived: true,
    includeOoc: true,
    limit,
    ...buildRoleFilterOptions(),
  });
  return {
    resp,
    normalized: normalizeMessageList(resp?.data || []),
  };
};

const buildAnchorWindowMessages = (messages: Message[], targetId: string) => {
  if (!Array.isArray(messages) || messages.length === 0) {
    return null;
  }
  const sorted = messages.slice().sort(compareByDisplayOrder);
  const targetIndex = sorted.findIndex((msg) => msg.id === targetId);
  if (targetIndex < 0) {
    return null;
  }
  const start = Math.max(0, targetIndex - SEARCH_ANCHOR_WINDOW_LIMIT);
  const end = Math.min(sorted.length, targetIndex + SEARCH_ANCHOR_WINDOW_LIMIT + 1);
  return sorted.slice(start, end);
};

const applyHistoricalWindowFromMessages = (
  messages: Message[],
  payload: { messageId: string },
  options: { cursorBefore?: string | null; fromTime?: number } = {},
) => {
  const windowMessages = buildAnchorWindowMessages(messages, payload.messageId);
  if (!windowMessages) {
    return false;
  }
  resetWindowState('history');
  rows.value = windowMessages;
  sortRowsByDisplayOrder();
  if (options.cursorBefore !== undefined) {
    applyCursorUpdate({ before: options.cursorBefore ?? '' });
  }
  computeAfterCursorFromRows();
  messageWindow.hasReachedStart = false;
  if (options.fromTime !== undefined) {
    messageWindow.beforeCursorExhausted = !messageWindow.beforeCursor && options.fromTime === 0;
  }
  messageWindow.hasReachedLatest = false;
  updateAnchorMessage(payload.messageId);
  showButton.value = true;
  lockHistoryView();
  return true;
};

const mountHistoricalWindowWithSpan = async (
  payload: { messageId: string; createdAt?: number },
  spanMs: number,
) => {
  if (!chat.curChannel?.id || !payload.createdAt || spanMs <= 0) {
    return false;
  }
  const center = Number(payload.createdAt);
  if (!Number.isFinite(center)) {
    return false;
  }
  const from = Math.max(0, Math.floor(center - spanMs));
  const to = Math.max(from + 1, Math.floor(center + spanMs));
  try {
    let { resp, normalized } = await loadSearchJumpWindow(from, to, SEARCH_JUMP_LIMIT_PRIMARY);
    if (!normalized.length) {
      return false;
    }
    let containsTarget = normalized.some((msg) => msg.id === payload.messageId);
    if (!containsTarget && normalized.length >= SEARCH_JUMP_LIMIT_PRIMARY) {
      const retry = await loadSearchJumpWindow(from, to, SEARCH_JUMP_LIMIT_RETRY);
      resp = retry.resp;
      normalized = retry.normalized;
      if (!normalized.length) {
        return false;
      }
      containsTarget = normalized.some((msg) => msg.id === payload.messageId);
    }
    if (!containsTarget) {
      return false;
    }
    return applyHistoricalWindowFromMessages(normalized, payload, {
      cursorBefore: resp?.next ?? '',
      fromTime: from,
    });
  } catch (error) {
    console.warn('加载历史视图失败', error);
    return false;
  }
};

const mountHistoricalWindow = async (payload: { messageId: string; createdAt?: number }) => {
  for (const span of SEARCH_JUMP_WINDOWS_MS) {
    const mounted = await mountHistoricalWindowWithSpan(payload, span);
    if (mounted) {
      return true;
    }
  }
  return false;
};

const loadMessagesWithinWindow = async (
  payload: { messageId: string; displayOrder?: number; createdAt?: number },
  spanMs: number,
) => {
  if (!chat.curChannel?.id || !payload.createdAt || spanMs <= 0) {
    return null;
  }
  const center = Number(payload.createdAt);
  if (!Number.isFinite(center)) {
    return null;
  }
  const from = Math.max(0, Math.floor(center - spanMs));
  const to = Math.max(from + 1, Math.floor(center + spanMs));
  try {
    let { resp, normalized } = await loadSearchJumpWindow(from, to, SEARCH_JUMP_LIMIT_PRIMARY);
    if (!normalized.length) {
      return null;
    }
    let containsTarget = normalized.some((msg) => msg.id === payload.messageId);
    if (!containsTarget && normalized.length >= SEARCH_JUMP_LIMIT_PRIMARY) {
      const retry = await loadSearchJumpWindow(from, to, SEARCH_JUMP_LIMIT_RETRY);
      resp = retry.resp;
      normalized = retry.normalized;
      if (!normalized.length) {
        return null;
      }
      containsTarget = normalized.some((msg) => msg.id === payload.messageId);
    }
    if (!containsTarget) {
      return null;
    }
    return {
      messages: normalized,
      cursorBefore: resp?.next ?? '',
      fromTime: from,
    };
  } catch (error) {
    console.warn('定位消息失败（时间窗口）', error);
    return null;
  }
};

const loadMessagesByCursor = async (payload: { messageId: string; displayOrder?: number; createdAt?: number }) => {
  if (!chat.curChannel?.id || payload.displayOrder === undefined) {
    return null;
  }
  const order = Number(payload.displayOrder);
  if (!Number.isFinite(order)) {
    return null;
  }
  const cursorOrder = order + 1e-6;
  const cursorTime = Math.max(0, Math.floor(Number(payload.createdAt ?? Date.now())));
  const cursor = `${cursorOrder.toFixed(8)}|${cursorTime}|${payload.messageId}`;
  try {
    const firstResp = await chat.messageList(chat.curChannel.id, cursor, {
      includeArchived: true,
      includeOoc: true,
      limit: SEARCH_JUMP_LIMIT_PRIMARY,
      ...buildRoleFilterOptions(),
    });
    let incoming = normalizeMessageList(firstResp?.data || []);
    if (!incoming.length) {
      return null;
    }
    let containsTarget = incoming.some((msg) => msg.id === payload.messageId);
    let cursorBefore = firstResp?.next ?? '';
    if (!containsTarget && incoming.length >= SEARCH_JUMP_LIMIT_PRIMARY) {
      const retryResp = await chat.messageList(chat.curChannel.id, cursor, {
        includeArchived: true,
        includeOoc: true,
        limit: SEARCH_JUMP_LIMIT_RETRY,
        ...buildRoleFilterOptions(),
      });
      incoming = normalizeMessageList(retryResp?.data || []);
      if (!incoming.length) {
        return null;
      }
      containsTarget = incoming.some((msg) => msg.id === payload.messageId);
      cursorBefore = retryResp?.next ?? '';
    }
    if (!containsTarget) {
      return null;
    }
    return {
      messages: incoming,
      cursorBefore,
    };
  } catch (error) {
    console.warn('定位消息失败（游标）', error);
    return null;
  }
};

const locateMessageForJump = async (payload: { messageId: string; displayOrder?: number; createdAt?: number }) => {
  for (const span of SEARCH_JUMP_WINDOWS_MS) {
    const window = await loadMessagesWithinWindow(payload, span);
    if (window) {
      return window;
    }
  }
  return loadMessagesByCursor(payload);
};

const ensureSearchTargetVisible = async (payload: { messageId: string; displayOrder?: number; createdAt?: number }) => {
  if (messageExistsLocally(payload.messageId)) {
    return true;
  }
  if (searchJumping.value) {
    message.info('正在定位消息，请稍候');
    return false;
  }
  searchJumping.value = true;
  const loadingMsg = message.loading('正在定位消息…', { duration: 0 });
  try {
    const mounted = await mountHistoricalWindow(payload);
    if (mounted) {
      return true;
    }
    const located = await locateMessageForJump(payload);
    if (!located) {
      message.warning('未能定位到该消息，可能已被删除或当前账号无权访问');
      return false;
    }
    const applied = applyHistoricalWindowFromMessages(located.messages, payload, {
      cursorBefore: located.cursorBefore,
      fromTime: located.fromTime,
    });
    if (!applied) {
      message.warning('仍未定位到该消息，稍后再试');
      return false;
    }
    return true;
  } finally {
    loadingMsg?.destroy?.();
    searchJumping.value = false;
  }
};

const handleSearchJump = async (payload: { messageId: string; displayOrder?: number; createdAt?: number; channelId?: string }) => {
  const targetId = payload?.messageId;
  if (!targetId) {
    message.warning('未找到要跳转的消息');
    return;
  }
  const targetChannelId = payload?.channelId;
  if (targetChannelId && targetChannelId !== chat.curChannel?.id) {
    const switched = await chat.channelSwitchTo(targetChannelId);
    if (!switched) {
      message.error('无法切换到目标频道，跳转已取消');
      return;
    }
  }

  // 如果没有 createdAt，先通过 API 获取消息详情
  let enrichedPayload = { ...payload };
  if (enrichedPayload.createdAt === undefined && chat.curChannel?.id) {
    try {
      const msgInfo = await chat.messageGetById(chat.curChannel.id, targetId);
      if (msgInfo) {
        enrichedPayload.createdAt = msgInfo.created_at;
        enrichedPayload.displayOrder = msgInfo.display_order;
      } else {
        message.warning('未能定位到该消息，可能已被删除或当前账号无权访问');
        return;
      }
    } catch (error) {
      console.warn('获取消息详情失败', error);
    }
  }

  await nextTick();
  let target = messageRowRefs.get(targetId);
  if (!target) {
    const loaded = await ensureSearchTargetVisible(enrichedPayload);
    if (!loaded) {
      return;
    }
    await nextTick();
    // 等待 DOM 渲染完成，最多重试几次
    for (let i = 0; i < 5; i++) {
      target = messageRowRefs.get(targetId);
      if (target) break;
      await new Promise(r => setTimeout(r, 50));
    }
    if (!target) {
      if (messageExistsLocally(targetId)) {
        message.warning('消息已加载，但当前筛选条件可能将其隐藏，请调整筛选后重试');
      } else {
        message.warning('仍未定位到该消息，稍后再试');
      }
      return;
    }
  }
  if (messagesListRef.value) {
    lockHistoryView();
    updateAnchorMessage(targetId);
    computeAfterCursorFromRows();
    VueScrollTo.scrollTo(target, {
      container: messagesListRef.value,
      duration: 350,
      offset: -60,
      easing: 'ease-in-out',
    });
    setMessageHighlight(targetId);
    showButton.value = true;
    void autoFillIfNeeded();
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
  handleEl: null as HTMLElement | null,
  autoScrollDirection: 0 as -1 | 0 | 1,
  autoScrollSpeed: 0,
  autoScrollRafId: null as number | null,
  lastClientY: null as number | null,
  // Optimization: RAF throttle for drag updates
  dragRafId: null as number | null,
  pendingClientY: null as number | null,
  // Track previous state to avoid redundant reorders
  prevOverId: null as string | null,
  prevPosition: null as 'before' | 'after' | null,
  // Ghost element offset
  ghostOffsetY: 0,
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

const releaseHandlePointerCapture = () => {
  if (dragState.handleEl && dragState.pointerId !== null) {
    try {
      dragState.handleEl.releasePointerCapture?.(dragState.pointerId);
    } catch {
      // ignore capture release errors
    }
  }
  dragState.handleEl = null;
};

const resetDragState = () => {
  clearGhost();
  stopAutoScroll();
  releaseHandlePointerCapture();
  // Cancel any pending RAF
  if (dragState.dragRafId !== null) {
    cancelAnimationFrame(dragState.dragRafId);
    dragState.dragRafId = null;
  }
  dragState.snapshot = [];
  dragState.clientOpId = null;
  dragState.overId = null;
  dragState.position = null;
  dragState.activeId = null;
  dragState.pointerId = null;
  dragState.startY = 0;
  dragState.lastClientY = null;
  dragState.pendingClientY = null;
  dragState.prevOverId = null;
  dragState.prevPosition = null;
  dragState.ghostOffsetY = 0;
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
  if ((item as any).is_revoked || (item as any).is_deleted) {
    return false;
  }
  if (isSelfMessage(item)) {
    return true;
  }
  return canReorderAll.value;
};

const shouldShowHandle = (item: Message) => canDragMessage(item);
const shouldShowInlineHeader = (entry: VisibleRowEntry) => !entry.mergedWithPrev;

const rowClass = (item: Message) => ({
  'message-row': true,
  'message-row--self': isSelfMessage(item),
  'draggable-item': canDragMessage(item),
  'message-row--drag-source': dragState.activeId === item.id,
  'message-row--drop-before': dragState.overId === item.id && dragState.position === 'before',
  'message-row--drop-after': dragState.overId === item.id && dragState.position === 'after',
  'message-row--search-hit': searchHighlightIds.value.has(item.id || ''),
  [`message-row--tone-${getMessageTone(item)}`]: true,
});

const rowSurfaceClass = (item: Message) => {
  const classes = [
    'message-row__surface',
    `message-row__surface--tone-${getMessageTone(item)}`,
  ];
  // 自己正在编辑该消息，或者他人正在编辑该消息（通过实时广播）
  if (chat.isEditingMessage(item.id || '') || editingPreviewMap.value[item.id || '']) {
    classes.push('message-row__surface--editing');
  }
  return classes;
};

const inheritChatContextClasses = (ghostEl: HTMLElement) => {
  const container = messagesListRef.value;
  if (!container) return;
  container.classList.forEach((className) => {
    if (className === 'chat' || className.startsWith('chat--')) {
      ghostEl.classList.add(className);
    }
  });
};

const createGhostElement = (rowEl: HTMLElement) => {
  const rect = rowEl.getBoundingClientRect();
  const ghost = document.createElement('div');
  ghost.className = 'message-row__ghost-float';
  
  const isDark = document.documentElement.classList.contains('dark') || 
                 document.body.classList.contains('dark');
  
  ghost.style.cssText = `
    position: fixed;
    left: ${rect.left}px;
    top: ${rect.top}px;
    width: ${rect.width}px;
    height: ${Math.min(rect.height, 160)}px;
    z-index: 9999;
    pointer-events: none;
    cursor: grabbing;
    box-shadow: 0 4px 12px rgba(0, 0, 0, ${isDark ? '0.25' : '0.15'});
    border-radius: 0.5rem;
    background: ${isDark ? 'var(--sc-bg-elevated, #1e1e1e)' : 'var(--sc-bg-surface, #fff)'};
    overflow: hidden;
    opacity: 0;
    transform: scale(1);
    transition: opacity 0.15s ease, transform 0.15s ease, box-shadow 0.15s ease;
  `;
  
  // Animate in after appending
  requestAnimationFrame(() => {
    ghost.style.opacity = '1';
    ghost.style.transform = 'scale(1.02)';
    ghost.style.boxShadow = `0 8px 24px rgba(0, 0, 0, ${isDark ? '0.4' : '0.2'})`;
  });
  // Clone the surface content - capture dimensions first
  const surface = rowEl.querySelector('.message-row__surface');
  if (surface) {
    const surfaceRect = surface.getBoundingClientRect();
    const clone = surface.cloneNode(true) as HTMLElement;
    // Reset all styles that might be inherited from drag-source
    clone.style.cssText = `
      pointer-events: none;
      opacity: 1 !important;
      max-height: none !important;
      height: ${Math.min(surfaceRect.height, 150)}px;
      overflow: hidden;
      transform: none !important;
      transition: none !important;
      margin: 0 !important;
      padding: inherit;
    `;
    ghost.appendChild(clone);
  }
  inheritChatContextClasses(ghost);
  document.body.appendChild(ghost);
  dragState.ghostEl = ghost;
  dragState.ghostOffsetY = rect.top - dragState.startY;
};

// Update ghost position to follow cursor
const updateGhostPosition = (clientY: number) => {
  if (!dragState.ghostEl) return;
  const newTop = clientY + (dragState.ghostOffsetY ?? 0);
  dragState.ghostEl.style.top = `${newTop}px`;
};

// Live reorder: move the dragged item within rows in real-time
const applyLiveReorder = () => {
  const activeId = dragState.activeId;
  const overId = dragState.overId;
  const position = dragState.position;
  if (!activeId || !overId || activeId === overId) {
    return;
  }
  // Skip if target hasn't changed (avoid redundant Vue updates)
  if (overId === dragState.prevOverId && position === dragState.prevPosition) {
    return;
  }
  dragState.prevOverId = overId;
  dragState.prevPosition = position;
  
  const currentRows = rows.value;
  const fromIndex = currentRows.findIndex((item) => item.id === activeId);
  const toReference = currentRows.findIndex((item) => item.id === overId);
  if (fromIndex < 0 || toReference < 0) {
    return;
  }
  let targetIndex = position === 'after' 
    ? (fromIndex < toReference ? toReference : toReference + 1)
    : (fromIndex < toReference ? toReference - 1 : toReference);
  if (targetIndex < 0) targetIndex = 0;
  if (targetIndex >= currentRows.length) targetIndex = currentRows.length - 1;
  if (fromIndex === targetIndex) {
    return;
  }
  const working = currentRows.slice();
  const [moving] = working.splice(fromIndex, 1);
  working.splice(targetIndex, 0, moving);
  rows.value = working;
};

const updateOverTarget = (clientY: number) => {
  // Hysteresis thresholds to prevent jitter at midpoint
  // Position only changes when crossing 35% or 65% of element height
  const THRESHOLD_BEFORE = 0.35; // Switch to 'before' when above 35%
  const THRESHOLD_AFTER = 0.65;  // Switch to 'after' when below 65%
  
  // Helper to calculate position with hysteresis
  const calcPosition = (rect: DOMRect, currentPos: 'before' | 'after' | null): 'before' | 'after' => {
    const relativeY = (clientY - rect.top) / rect.height;
    if (relativeY <= THRESHOLD_BEFORE) {
      return 'before';
    }
    if (relativeY >= THRESHOLD_AFTER) {
      return 'after';
    }
    // In the dead zone (35%-65%), keep current position to prevent flicker
    return currentPos || 'after';
  };

  // Fast path: check if still within current target before iterating all rows
  if (dragState.overId && dragState.overId !== dragState.activeId) {
    const currentEl = messageRowRefs.get(dragState.overId);
    if (currentEl) {
      const rect = currentEl.getBoundingClientRect();
      if (clientY >= rect.top && clientY < rect.bottom) {
        // Still within same element, just update position with hysteresis
        dragState.position = calcPosition(rect, dragState.position);
        return;
      }
    }
  }

  let matched = false;
  if (dragState.activeId) {
    const activeEl = messageRowRefs.get(dragState.activeId);
    if (activeEl) {
      const rectActive = activeEl.getBoundingClientRect();
      if (clientY >= rectActive.top && clientY <= rectActive.bottom) {
        dragState.overId = dragState.activeId;
        dragState.position = calcPosition(rectActive, dragState.position);
        matched = true;
      }
    }
  }
  if (!matched) {
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
      const relativeY = (clientY - rect.top) / rect.height;
      
      // Use thresholds for better stability
      if (relativeY <= THRESHOLD_BEFORE) {
        dragState.overId = item.id;
        dragState.position = 'before';
        matched = true;
        break;
      }
      if (clientY < rect.bottom) {
        dragState.overId = item.id;
        // When entering new element, use threshold logic
        dragState.position = relativeY >= THRESHOLD_AFTER ? 'after' : 
                             (dragState.overId === item.id ? dragState.position : 'after') || 'after';
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
  const originalRows = dragState.snapshot.slice();

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

  const working = originalRows.slice();
  const fromIndex = working.findIndex((item) => item.id === activeId);
  const toReference = working.findIndex((item) => item.id === overId);
  if (fromIndex < 0 || toReference < 0) {
    resetDragState();
    return;
  }

  const [moving] = working.splice(fromIndex, 1);
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
  if (targetIndex > working.length) {
    targetIndex = working.length;
  }
  working.splice(targetIndex, 0, moving);
  const estimateOrder = deriveLocalDisplayOrder(
    working,
    targetIndex,
    getMessageDisplayOrderValue(moving) ?? Date.now(),
  );
  (moving as any).displayOrder = estimateOrder;
  rows.value = working;
  listRevision.value += 1;

  const beforeId = working[targetIndex + 1]?.id || '';
  const afterId = working[targetIndex - 1]?.id || '';
  const clientOpId = dragState.clientOpId || nanoid();
  resetDragState();
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
    rows.value = originalRows;
    message.error('消息排序失败，请稍后重试');
  } finally {
    localReorderOps.delete(clientOpId);
    listRevision.value += 1;
  }
};

// Process drag update in animation frame for smooth 60fps updates
const processDragFrame = () => {
  dragState.dragRafId = null;
  const clientY = dragState.pendingClientY;
  if (clientY === null) return;
  dragState.pendingClientY = null;
  // Only move the ghost and track target - NO live reordering
  updateGhostPosition(clientY);
  updateOverTarget(clientY);
  updateAutoScroll(clientY);
};

const onDragPointerMove = (event: PointerEvent) => {
  if (event.pointerId !== dragState.pointerId) {
    return;
  }
  event.preventDefault();
  // Store pending position and schedule RAF if not already scheduled
  dragState.pendingClientY = event.clientY;
  if (dragState.dragRafId === null) {
    dragState.dragRafId = requestAnimationFrame(processDragFrame);
  }
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
  const handleEl = event.currentTarget as HTMLElement | null;
  const rowEl = messageRowRefs.get(item.id);
  if (!rowEl) {
    return;
  }
  if (handleEl) {
    dragState.handleEl = handleEl;
    try {
      handleEl.setPointerCapture?.(event.pointerId);
    } catch {
      // ignore capture failure
    }
  }
  dragState.snapshot = rows.value.slice();
  dragState.clientOpId = nanoid();
  dragState.activeId = item.id;
  dragState.pointerId = event.pointerId;
  dragState.startY = event.clientY;
  dragState.overId = item.id;
  dragState.position = 'after';
  dragState.originEl = rowEl;
  document.body.style.userSelect = 'none';
  
  // IMPORTANT: Create ghost BEFORE adding drag-source class (which collapses the row)
  createGhostElement(rowEl);
  
  // Now add the collapse class
  rowEl.classList.add('message-row--drag-source');
  
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

const normalizeMessageList = (items: any[] = []): Message[] =>
  items
    .map((item) => normalizeMessageShape(item))
    .filter((item) => !(item as any)?.is_deleted);

const upsertMessage = (incoming?: Message) => {
  if (!incoming || !incoming.id) {
    return;
  }
  if ((incoming as any).is_deleted || (incoming as any).isDeleted) {
    rows.value = rows.value.filter((msg) => msg.id !== incoming.id);
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
  color?: string;
  content: string;
  indicatorOnly: boolean;
  mode: 'typing' | 'editing';
  messageId?: string;
  tone: 'ic' | 'ooc';
  orderKey: number;
}

const resolveTypingTone = (typing?: { icMode?: string; ic_mode?: string; tone?: string }): 'ic' | 'ooc' => {
  const raw = typing?.icMode ?? typing?.ic_mode ?? typing?.tone;
  if (typeof raw === 'string' && raw.toLowerCase() === 'ooc') {
    return 'ooc';
  }
  return 'ic';
};

interface EditingPreviewInfo {
  userId: string;
  displayName: string;
  avatar?: string;
  content: string;
  indicatorOnly: boolean;
  isSelf: boolean;
  summary: string;
  previewHtml: string;
  tone: 'ic' | 'ooc';
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
let typingPreviewOrderSeq = Date.now();
const previewOrderMin = 1e-6;
const selfPreviewOrderKey = ref<number>(Number.MAX_SAFE_INTEGER);
const selfPreviewOrderModified = ref(false);
const resetSelfPreviewOrder = () => {
	selfPreviewOrderKey.value = Number.MAX_SAFE_INTEGER;
	selfPreviewOrderModified.value = false;
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
const shouldShowTypingHandle = (preview: TypingPreviewItem) => {
  if (!preview?.userId) {
    return false;
  }
  if (preview.userId === user.info.id) {
    return true;
  }
  return canReorderAll.value;
};
const inputPreviewEnabled = computed(() => display.settings.showInputPreview !== false);
const autoScrollTypingPreviewAlways = computed(() => display.settings.autoScrollTypingPreview === true);
const shouldObserveTypingPreview = computed(() => (
  inputPreviewEnabled.value
  && (autoScrollTypingPreviewAlways.value || (!inHistoryMode.value && !historyLocked.value))
));
const activeIdentityForPreview = computed(() => chat.getActiveIdentity(chat.curChannel?.id || ''));
const selfPreviewUserId = computed(() => user.info?.id || '__self__');
const typingPreviewItems = computed(() =>
  typingPreviewList.value
    .filter((item) => item.mode === 'typing')
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
  const identity = activeIdentityForPreview.value;
  if (identity?.displayName) {
    return identity.displayName;
  }
  return user.info?.nick || user.info?.name || '我';
};
const resolveSelfPreviewAvatar = () => {
  const identity = activeIdentityForPreview.value;
  if (identity?.avatarAttachmentId) {
    return resolveAttachmentUrl(identity.avatarAttachmentId);
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
  const draft = textToSend.value;
  if (!isContentMeaningful(inputMode.value, draft)) {
    removeSelfTypingPreview();
    return;
  }
  const identity = activeIdentityForPreview.value;
  const displayName = resolveSelfPreviewDisplayName();
  const avatar = resolveSelfPreviewAvatar();
  const normalizedColor = identity?.color ? normalizeHexColor(identity.color || '') || undefined : undefined;
  const tone = inputIcMode.value || 'ic';
  let previewContent = draft;
  if (inputMode.value !== 'rich') {
    const normalized = replaceEmojiRemarksForPreview(draft);
    previewContent = normalized.length > 500 ? normalized.slice(0, 500) : normalized;
  }
  const payload: TypingPreviewItem = {
    userId: selfPreviewUserId.value,
    displayName,
    avatar,
    color: normalizedColor,
    content: previewContent,
    indicatorOnly: false,
    mode: 'typing',
    tone,
    messageId: undefined,
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
		const extra: { whisperTo?: string; icMode: 'ic' | 'ooc'; orderKey?: number } = { icMode };
		if (targetId) {
			extra.whisperTo = targetId;
		}
		if (typeof options?.orderKey === 'number' && Number.isFinite(options.orderKey) && options.orderKey > 0) {
			extra.orderKey = options.orderKey;
		}
		lastTypingWhisperTargetId = targetId ?? null;
		chat.messageTyping(state, content, channelId, extra);
	},
	400,
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
    const extra = lastTypingWhisperTargetId ? { whisperTo: lastTypingWhisperTargetId, icMode } : { icMode };
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
  if (typingPreviewMode.value !== 'content') {
    return;
  }
  const whisperTargetId = chat.editing?.whisperTargetId || resolveCurrentWhisperTargetId();
  const icMode = chat.editing?.icMode === 'ooc' ? 'ooc' : 'ic';
  const extra: { mode: 'editing'; messageId: string; whisperTo?: string; icMode: 'ic' | 'ooc' } = {
    mode: 'editing',
    messageId,
    icMode,
  };
  if (whisperTargetId) {
    extra.whisperTo = whisperTargetId;
  }
  chat.messageTyping('content', content, channelId, extra);
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

const stripDiceChipMarkup = (html: string) => {
  if (!html || !html.includes('dice-chip')) {
    return html;
  }
  try {
    const parser = new DOMParser();
    const doc = parser.parseFromString(`<div>${html}</div>`, 'text/html');
    doc.querySelectorAll('span.dice-chip').forEach((element) => {
      const source = element.getAttribute('data-dice-source') || element.textContent || '';
      if (!element.parentNode) return;
      const replacement = doc.createTextNode(source);
      element.parentNode.replaceChild(replacement, element);
    });
    const first = doc.body.firstElementChild;
    if (first && first.tagName === 'DIV') {
      return first.innerHTML;
    }
    return doc.body.innerHTML;
  } catch (error) {
    console.warn('stripDiceChipMarkup failed', error);
    return html;
  }
};

const convertMessageContentToDraft = (content?: string) => {
  resetInlineImages();
  if (!content) {
    return '';
  }
  content = stripDiceChipMarkup(content);
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

  if (isEditing.value) {
    emitEditingPreview();
    return;
  }

  if (typingPreviewMode.value === 'silent') {
    stopTypingPreviewNow();
    return;
  }

  let raw = textToSend.value;

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
      stopTypingPreviewNow();
      return;
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
  const raw = textToSend.value;
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

const textToSend = ref('');

// 术语快捷输入状态
const keywordSuggestVisible = ref(false);
const keywordSuggestQuery = ref('');
const keywordSuggestIndex = ref(0);
const keywordSuggestSlashPos = ref(-1);
const keywordSuggestLoading = ref(false);
const keywordSuggestOptions = ref<KeywordMatchResult[]>([]);

// 输入历史（localStorage 版本，按频道保留 5 条）
const HISTORY_STORAGE_KEY = 'sealchat_input_history_v1';
const HISTORY_CHANNEL_FALLBACK = '__global__';
const MAX_HISTORY_PER_CHANNEL = 5;
const HISTORY_PREVIEW_MAX = 120;
const HISTORY_AUTO_RESTORE_WINDOW = 10 * 60 * 1000;
const pendingHistoryRestoreChannelKey = ref<string | null>(null);
const HISTORY_AUTORESTORE_STORAGE_KEY = 'sealchat_input_history_autorestore_v1';

interface HistoryAutoRestoreEntry {
  entryId: string;
  updatedAt: number;
}

type HistoryAutoRestoreStore = Record<string, HistoryAutoRestoreEntry>;

const scheduleHistoryAutoRestore = () => {
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    pendingHistoryRestoreChannelKey.value = null;
    return;
  }
  pendingHistoryRestoreChannelKey.value = String(channelId);
};

interface HistoryImageInfo {
  markerId: string;
  attachmentId: string;
}

interface InputHistoryEntry {
  id: string;
  channelKey: string;
  mode: 'plain' | 'rich';
  content: string;
  createdAt: number;
  images?: HistoryImageInfo[];
}

type HistoryStore = Record<string, InputHistoryEntry[]>;

interface HistoryEntryView extends InputHistoryEntry {
  preview: string;
  fullPreview: string;
  timeLabel: string;
}

const historyEntries = ref<InputHistoryEntry[]>([]);
const historyPopoverVisible = ref(false);
const hasHistoryEntries = computed(() => historyEntries.value.length > 0);
const currentChannelKey = computed(() => chat.curChannel?.id ? String(chat.curChannel.id) : HISTORY_CHANNEL_FALLBACK);
const lastHistorySignature = ref<string | null>(null);

const buildHistorySignature = (mode: 'plain' | 'rich', content: string) => `${mode}:${content}`;

const readHistoryStore = (): HistoryStore => {
  try {
    const raw = localStorage.getItem(HISTORY_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') {
      return parsed as HistoryStore;
    }
  } catch (e) {
    console.error('读取输入历史失败', e);
  }
  return {};
};

const writeHistoryStore = (store: HistoryStore) => {
  try {
    localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(store));
  } catch (e) {
    console.error('写入输入历史失败', e);
  }
};

const readHistoryAutoRestoreStore = (): HistoryAutoRestoreStore => {
  try {
    const raw = localStorage.getItem(HISTORY_AUTORESTORE_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') {
      return parsed as HistoryAutoRestoreStore;
    }
  } catch (e) {
    console.error('读取自动恢复状态失败', e);
  }
  return {};
};

const writeHistoryAutoRestoreStore = (store: HistoryAutoRestoreStore) => {
  try {
    localStorage.setItem(HISTORY_AUTORESTORE_STORAGE_KEY, JSON.stringify(store));
  } catch (e) {
    console.error('写入自动恢复状态失败', e);
  }
};

const getAutoRestoreEntryForChannel = (channelKey: string): HistoryAutoRestoreEntry | null => {
  if (!channelKey) {
    return null;
  }
  const store = readHistoryAutoRestoreStore();
  return store[channelKey] || null;
};

const markAutoRestoreEntry = (channelKey: string, entryId: string) => {
  if (!channelKey) {
    return;
  }
  const store = readHistoryAutoRestoreStore();
  store[channelKey] = {
    entryId,
    updatedAt: Date.now(),
  };
  writeHistoryAutoRestoreStore(store);
};

const clearAutoRestoreEntry = (channelKey: string) => {
  if (!channelKey) {
    return;
  }
  const store = readHistoryAutoRestoreStore();
  if (store[channelKey]) {
    delete store[channelKey];
    writeHistoryAutoRestoreStore(store);
  }
};

const normalizeHistoryEntries = (entries: any[]): InputHistoryEntry[] => {
  if (!Array.isArray(entries)) {
    return [];
  }
  return entries
    .map((entry) => {
      if (!entry || typeof entry !== 'object') {
        return null;
      }
      const mode = entry.mode === 'rich' ? 'rich' : 'plain';
      const content = typeof entry.content === 'string' ? entry.content : '';
      if (!content) {
        return null;
      }
      const createdAt = typeof entry.createdAt === 'number' ? entry.createdAt : Date.now();
      const id = typeof entry.id === 'string' ? entry.id : nanoid();
      const channelKey = typeof entry.channelKey === 'string' ? entry.channelKey : currentChannelKey.value;
      // 解析图片信息
      let images: HistoryImageInfo[] | undefined;
      if (Array.isArray(entry.images)) {
        images = entry.images
          .filter((img: any) => img && typeof img.markerId === 'string' && typeof img.attachmentId === 'string')
          .map((img: any) => ({ markerId: img.markerId, attachmentId: img.attachmentId }));
        if (images.length === 0) {
          images = undefined;
        }
      }
      return { id, channelKey, mode, content, createdAt, images } as InputHistoryEntry;
    })
    .filter((entry): entry is InputHistoryEntry => !!entry);
};

const refreshHistoryEntries = () => {
  const store = readHistoryStore();
  const rawEntries = store[currentChannelKey.value] || [];
  const entries = normalizeHistoryEntries(rawEntries)
    .sort((a, b) => b.createdAt - a.createdAt)
    .slice(0, MAX_HISTORY_PER_CHANNEL);
  historyEntries.value = entries;
  lastHistorySignature.value = entries.length
    ? buildHistorySignature(entries[0].mode, entries[0].content)
    : null;
};

const pruneAndPersist = (channelKey: string, entries: InputHistoryEntry[]) => {
  const store = readHistoryStore();
  store[channelKey] = entries.slice(0, MAX_HISTORY_PER_CHANNEL);
  writeHistoryStore(store);
  if (channelKey === currentChannelKey.value) {
    historyEntries.value = store[channelKey].slice();
    lastHistorySignature.value = historyEntries.value.length
      ? buildHistorySignature(historyEntries.value[0].mode, historyEntries.value[0].content)
      : null;
  }
};

const isRichContentEmpty = (content: string) => {
  if (!isTipTapJson(content)) {
    return content.trim().length === 0;
  }
  try {
    const plain = tiptapJsonToPlainText(content);
    return plain.trim().length === 0;
  } catch (e) {
    console.warn('富文本解析失败，按非空处理', e);
    return false;
  }
};

const isContentMeaningful = (mode: 'plain' | 'rich', content: string) => {
  if (!content) {
    return false;
  }
  if (mode === 'plain') {
    return content.trim().length > 0 || containsInlineImageMarker(content);
  }
  return !isRichContentEmpty(content);
};

// 从当前 inlineImages Map 中提取图片信息用于历史保存
const collectCurrentImageInfo = (): HistoryImageInfo[] => {
  const images: HistoryImageInfo[] = [];
  inlineImages.forEach((draft, markerId) => {
    if (draft.status === 'uploaded' && draft.attachmentId) {
      const attachmentId = draft.attachmentId.startsWith('id:')
        ? draft.attachmentId.slice(3)
        : draft.attachmentId;
      images.push({ markerId, attachmentId });
    }
  });
  return images;
};

const appendHistoryEntry = (mode: 'plain' | 'rich', content: string, options: { force?: boolean } = {}): boolean => {
  if (!isContentMeaningful(mode, content)) {
    return false;
  }
  const signature = buildHistorySignature(mode, content);
  if (!options.force && signature === lastHistorySignature.value) {
    const existingEntry = historyEntries.value.find(
      (entry) => buildHistorySignature(entry.mode, entry.content) === signature,
    );
    if (existingEntry) {
      markAutoRestoreEntry(currentChannelKey.value, existingEntry.id);
    }
    return false;
  }
  const channelKey = currentChannelKey.value;
  const store = readHistoryStore();
  const existing = normalizeHistoryEntries(store[channelKey] || []);
  const filtered = existing.filter((entry) => buildHistorySignature(entry.mode, entry.content) !== signature);
  
  // 提取当前图片信息
  const images = mode === 'plain' ? collectCurrentImageInfo() : undefined;
  
  const newEntry: InputHistoryEntry = {
    id: nanoid(),
    channelKey,
    mode,
    content,
    createdAt: Date.now(),
    images: images?.length ? images : undefined,
  };
  filtered.unshift(newEntry);
  pruneAndPersist(channelKey, filtered);
  lastHistorySignature.value = signature;
  if (!options.force) {
    markAutoRestoreEntry(channelKey, newEntry.id);
  }
  return true;
};

const formatHistoryTimestamp = (timestamp: number) => {
  const date = new Date(timestamp);
  return date.toLocaleString();
};

const getHistoryPreview = (entry: InputHistoryEntry) => {
  try {
    if (entry.mode === 'rich' && isTipTapJson(entry.content)) {
      const plain = tiptapJsonToPlainText(entry.content).replace(/\s+/g, ' ').trim();
      return plain || '[富文本内容]';
    }
    // 将图片标记替换为友好的显示文本
    let preview = contentUnescape(entry.content)
      .replace(/\[\[图片:[^\]]+\]\]/g, '[图片]')
      .replace(/\s+/g, ' ')
      .trim();
    return preview || (entry.images?.length ? '[图片]' : '[空内容]');
  } catch (e) {
    console.warn('生成历史预览失败', e);
    return entry.mode === 'rich' ? '[富文本内容]' : entry.content;
  }
};

const historyEntryViews = computed<HistoryEntryView[]>(() => {
  return historyEntries.value.map((entry) => {
    const fullPreview = getHistoryPreview(entry);
    const truncated = fullPreview.length > HISTORY_PREVIEW_MAX
      ? `${fullPreview.slice(0, HISTORY_PREVIEW_MAX)}…`
      : fullPreview;
    return {
      ...entry,
      fullPreview: fullPreview || (entry.mode === 'rich' ? '[富文本格式]' : '[文本内容]'),
      preview: truncated || (entry.mode === 'rich' ? '[富文本格式]' : '[文本内容]'),
      timeLabel: formatHistoryTimestamp(entry.createdAt),
    };
  });
});

const canManuallySaveHistory = computed(() => isContentMeaningful(inputMode.value, textToSend.value));

const restoreHistoryEntry = (entryId: string) => {
  const target = historyEntries.value.find((entry) => entry.id === entryId);
  if (!target) {
    message.warning('未找到可恢复的内容');
    return;
  }
  const willOverride = textToSend.value.trim().length > 0 && textToSend.value !== target.content;
  const proceed = () => {
    applyHistoryEntry(target);
    historyPopoverVisible.value = false;
  };
  if (willOverride) {
    dialog.warning({
      title: '恢复历史内容',
      content: '当前输入框已有内容，恢复历史将覆盖现有内容，是否继续？',
      positiveText: '恢复',
      negativeText: '取消',
      onPositiveClick: () => {
        proceed();
      },
    });
    return;
  }
  proceed();
};

// 从历史记录中恢复图片信息到 inlineImages Map
const restoreImagesFromHistory = (entry: InputHistoryEntry) => {
  if (entry.mode !== 'plain' || !entry.images?.length) {
    return;
  }
  // 检查内容中包含哪些图片标记
  const contentMarkers = collectInlineMarkerIds(entry.content);
  
  // 只恢复内容中存在的图片标记
  entry.images.forEach((imageInfo) => {
    if (contentMarkers.has(imageInfo.markerId) && !inlineImages.has(imageInfo.markerId)) {
      const attachmentId = imageInfo.attachmentId.startsWith('id:')
        ? imageInfo.attachmentId
        : `id:${imageInfo.attachmentId}`;
      const record: InlineImageDraft = reactive({
        id: imageInfo.markerId,
        token: `[[图片:${imageInfo.markerId}]]`,
        status: 'uploaded',
        attachmentId: attachmentId.slice(3), // 存储时不带 id: 前缀
      });
      inlineImages.set(imageInfo.markerId, record);
    }
  });
};

const applyHistoryEntry = (entry: InputHistoryEntry, options?: { silent?: boolean }) => {
  try {
    clearInputModeCache();
    inputMode.value = entry.mode;
    suspendInlineSync = true;
    textToSend.value = entry.content;
    suspendInlineSync = false;
    
    // 恢复图片信息
    restoreImagesFromHistory(entry);
    syncInlineMarkersWithText(entry.content);
    
    markAutoRestoreEntry(currentChannelKey.value, entry.id);
    if (!options?.silent) {
      message.success('已恢复历史输入');
    }
    nextTick(() => {
      textInputRef.value?.focus();
    });
  } catch (e) {
    console.error('恢复历史输入失败', e);
    message.error('恢复失败');
  }
};

const handleManualHistoryRecord = () => {
  if (!canManuallySaveHistory.value) {
    message.warning('当前内容为空，无法保存到历史');
    return;
  }
  const success = appendHistoryEntry(inputMode.value, textToSend.value, { force: true });
  if (success) {
    message.success('已保存当前输入');
    refreshHistoryEntries();
  }
};

const tryAutoRestoreHistory = () => {
  const channelKey = currentChannelKey.value;
  if (
    !channelKey ||
    channelKey === HISTORY_CHANNEL_FALLBACK ||
    pendingHistoryRestoreChannelKey.value !== channelKey
  ) {
    return;
  }
  pendingHistoryRestoreChannelKey.value = null;
  if (!chat.curChannel?.id) {
    return;
  }
  if (textToSend.value.trim().length > 0) {
    return;
  }
  const autoRestoreEntry = getAutoRestoreEntryForChannel(channelKey);
  if (!autoRestoreEntry) {
    return;
  }
  const target = historyEntries.value.find((entry) => entry.id === autoRestoreEntry.entryId);
  if (!target) {
    clearAutoRestoreEntry(channelKey);
    return;
  }
  const withinWindow = Date.now() - autoRestoreEntry.updatedAt <= HISTORY_AUTO_RESTORE_WINDOW;
  if (!withinWindow) {
    clearAutoRestoreEntry(channelKey);
    return;
  }
  applyHistoryEntry(target, { silent: true });
  message.info('已自动恢复上次输入');
};

const scheduleHistorySnapshot = throttle(
  () => {
    if (isEditing.value) {
      return;
    }
    appendHistoryEntry(inputMode.value, textToSend.value);
  },
  2000,
  { leading: false, trailing: true },
);

watch(currentChannelKey, () => {
  historyPopoverVisible.value = false;
  refreshHistoryEntries();
  scheduleHistoryAutoRestore();
});

const handleHistoryPopoverShow = (show: boolean) => {
  historyPopoverVisible.value = show;
  if (show) {
    refreshHistoryEntries();
  }
};

watch(hasHistoryEntries, (has) => {
  if (!has) {
    historyPopoverVisible.value = false;
  }
});

onMounted(() => {
  refreshHistoryEntries();
  scheduleHistoryAutoRestore();
});

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
        tone: item.tone ?? 'ic',
      };
    }
  });
  if (isEditing.value && chat.editing) {
    const draft = textToSend.value;
    const indicatorOnly = draft.trim().length === 0;
    const { summary, previewHtml } = indicatorOnly ? { summary: '', previewHtml: '' } : buildPreviewMeta(draft);
    let previewDisplayName = chat.curMember?.nick || user.info.nick || user.info.name || '我';
    let previewAvatar = chat.curMember?.avatar || user.info.avatar || '';
    const identityPreview = resolveIdentityPreviewInfo(chat.editing.channelId, chat.editing.identityId);
    if (identityPreview) {
      if (identityPreview.displayName) {
        previewDisplayName = identityPreview.displayName;
      }
      if (identityPreview.avatar) {
        previewAvatar = identityPreview.avatar;
      }
    }
    map[chat.editing.messageId] = {
      userId: user.info.id,
      displayName: previewDisplayName,
      avatar: previewAvatar,
      content: draft,
      indicatorOnly,
      isSelf: true,
      summary,
      previewHtml,
      tone: chat.editing.icMode === 'ooc' ? 'ooc' : 'ic',
    };
  }
  return map;
});

watch(
  () => chat.icMode,
  (mode, previous) => {
    if (mode === previous) {
      return;
    }
    if (isEditing.value) {
      emitEditingPreview();
    } else {
      emitTypingPreview();
    }
  },
);

watch(
  () => chat.editing?.icMode,
  (mode, previous) => {
    if (!chat.editing || mode === previous) {
      return;
    }
    emitEditingPreview();
    // 增加 listRevision 强制触发消息行重新渲染，确保外边框 CSS 实时更新
    listRevision.value += 1;
  },
);

// 监听编辑状态下角色 ID 的变化，确保头像和角色名实时更新
watch(
  () => chat.editing?.identityId,
  (identityId, previous) => {
    if (!chat.editing || identityId === previous) {
      return;
    }
    emitEditingPreview();
  },
);
const whisperPanelVisible = ref(false);
const whisperPickerSource = ref<'slash' | 'manual' | null>(null);
const whisperQuery = ref('');
const whisperSelectionIndex = ref(0);
const whisperSearchInputRef = ref<any>(null);
const whisperCandidateColorMap = ref<Map<string, string>>(new Map());
const whisperMentionableCandidates = ref<WhisperCandidate[]>([]);

type WhisperIdentityType = 'ic' | 'ooc' | 'user';

interface WhisperCandidate {
  raw: any;
  id: string;
  avatar: string;
  displayName: string;
  secondaryName: string;
  color: string;
  identityTypes: WhisperIdentityType[];
}

const whisperIdentityTypeOrder: Record<WhisperIdentityType, number> = {
  ic: 0,
  ooc: 1,
  user: 2,
};

const normalizeWhisperIdentityType = (value?: string): WhisperIdentityType => {
  if (value === 'ic' || value === 'ooc') {
    return value;
  }
  return 'user';
};

const whisperIdentityTypeLabel = (type: WhisperIdentityType): string => {
  switch (type) {
    case 'ic':
      return '场内';
    case 'ooc':
      return '场外';
    default:
      return '用户';
  }
};

const buildWhisperCandidates = (items: Array<{ userId?: string; displayName?: string; avatar?: string; color?: string; identityType?: string }>) => {
  const deduped = new Map<string, { candidate: WhisperCandidate; primaryWeight: number; types: Set<WhisperIdentityType> }>();
  for (const item of items) {
    const userId = String(item?.userId || '').trim();
    if (!userId || userId === user.info.id) {
      continue;
    }
    const identityType = normalizeWhisperIdentityType(item?.identityType);
    const displayName = item?.displayName || '未知成员';
    const avatar = item?.avatar || '';
    const color = normalizeHexColor(item?.color || '') || '';
    const weight = whisperIdentityTypeOrder[identityType];

    const existing = deduped.get(userId);
    if (!existing) {
      deduped.set(userId, {
        primaryWeight: weight,
        types: new Set<WhisperIdentityType>([identityType]),
        candidate: {
          raw: {
            id: userId,
            name: displayName,
            nick: displayName,
            avatar,
            color,
          },
          id: userId,
          avatar,
          displayName,
          secondaryName: '',
          color,
          identityTypes: [identityType],
        },
      });
      continue;
    }

    existing.types.add(identityType);
    if (weight < existing.primaryWeight) {
      existing.primaryWeight = weight;
      existing.candidate.avatar = avatar;
      existing.candidate.displayName = displayName;
      existing.candidate.color = color;
      existing.candidate.raw = {
        id: userId,
        name: displayName,
        nick: displayName,
        avatar,
        color,
      };
    }
  }

  const candidates = Array.from(deduped.values()).map((entry) => {
    const types = Array.from(entry.types).sort((a, b) => whisperIdentityTypeOrder[a] - whisperIdentityTypeOrder[b]);
    entry.candidate.identityTypes = types;
    return entry.candidate;
  });

  candidates.sort((a, b) => {
    const aHasIc = a.identityTypes.includes('ic');
    const bHasIc = b.identityTypes.includes('ic');
    if (aHasIc !== bHasIc) {
      return aHasIc ? -1 : 1;
    }
    return a.displayName.localeCompare(b.displayName);
  });

  return candidates;
};

const resolveWhisperTargetColor = (target: { id?: string; color?: string; nick_color?: string; nickColor?: string } | null | undefined) => {
  const id = target?.id;
  if (id) {
    const mapped = whisperCandidateColorMap.value.get(String(id));
    if (mapped) {
      return mapped;
    }
  }
  const fallback = target?.color || target?.nick_color || target?.nickColor || '';
  return normalizeHexColor(fallback) || '';
};

const getWhisperTargetStyle = (target: { id?: string; color?: string; nick_color?: string; nickColor?: string } | null | undefined) => {
  const color = resolveWhisperTargetColor(target);
  return color ? { color } : undefined;
};

const whisperCandidates = computed<WhisperCandidate[]>(() => whisperMentionableCandidates.value);

const filteredWhisperCandidates = computed(() => {
  const keyword = whisperQuery.value.trim();
  if (!keyword) {
    return whisperCandidates.value;
  }
  return whisperCandidates.value.filter((candidate) => {
    const candidates = [
      candidate.displayName,
      candidate.secondaryName,
      candidate.id,
    ].filter(Boolean).map((str) => String(str));
    return candidates.some((name) => matchText(keyword, name));
  });
});

const canOpenWhisperPanel = computed(() => {
  const channelId = chat.curChannel?.id || '';
  return Boolean(channelId) && channelId.length < 30;
});
const whisperTargets = computed(() => chat.whisperTargets);
const isWhisperTarget = (u: { id?: string } | null | undefined) => (
  Boolean(u?.id) && whisperTargets.value.some((item) => item.id === u?.id)
);
const whisperMode = computed(() => whisperTargets.value.length > 0);
const whisperToggleActive = computed(() => whisperPanelVisible.value || whisperTargets.value.length > 0);
const whisperPlaceholderText = computed(() => {
  if (!whisperMode.value) {
    return '';
  }
  if (whisperTargets.value.length === 1) {
    const target = whisperTargets.value[0];
    const name = target?.nick || target?.name || '未知成员';
    return t('inputBox.whisperPlaceholder', { target: `@${name}` });
  }
  return t('inputBox.whisperPlaceholderMultiple', { count: whisperTargets.value.length });
});

const ensureInputFocus = () => {
  nextTick(() => {
    if (textInputRef.value?.focus) {
      textInputRef.value.focus();
      return;
    }
    textInputRef.value?.getTextarea?.()?.focus();
  });
};

const getInputSelection = (): SelectionRange => {
  const selection = textInputRef.value?.getSelectionRange?.();
  if (selection) {
    return { start: selection.start, end: selection.end };
  }
  const textarea = textInputRef.value?.getTextarea?.();
  if (textarea) {
    return { start: textarea.selectionStart, end: textarea.selectionEnd };
  }
  const length = textToSend.value.length;
  return { start: length, end: length };
};

const isInputEffectivelyEmpty = () => {
  if (inlineImages.size > 0) {
    return false;
  }
  const raw = textToSend.value;
  if (!raw) {
    return true;
  }
  if (inputMode.value === 'rich') {
    const editorInstance = textInputRef.value?.getEditor?.();
    if (editorInstance) {
      return editorInstance.isEmpty;
    }
    return isRichContentEmpty(raw);
  }
  return raw.trim().length === 0;
};

const setInputSelection = (start: number, end: number) => {
  if (textInputRef.value?.setSelectionRange) {
    textInputRef.value.setSelectionRange(start, end);
    return;
  }
  textInputRef.value?.getTextarea?.()?.setSelectionRange(start, end);
};

const insertDiceExpression = (expr: string) => {
  if (!expr) {
    return;
  }
  if (inputMode.value === 'rich') {
    const editorInstance = textInputRef.value?.getEditor?.();
    if (editorInstance) {
      editorInstance.chain().focus().insertContent(`${expr} `).run();
      return;
    }
  }
  const selection = getInputSelection();
  const text = textToSend.value;
  const next = text.slice(0, selection.start) + expr + text.slice(selection.end);
  textToSend.value = next;
  const cursor = selection.start + expr.length;
  nextTick(() => {
    setInputSelection(cursor, cursor);
  });
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

const resolveMessageWhisperTargetId = (msg?: any): string | null => {
  if (!msg) {
    return null;
  }
  const metaIds = msg?.whisperMeta?.targetUserIds;
  if (Array.isArray(metaIds) && metaIds.length > 0) {
    return String(metaIds[0]);
  }
  const metaId = msg?.whisperMeta?.targetUserId;
  if (metaId) {
    return metaId;
  }
  const list = msg?.whisperToIds || msg?.whisper_to_ids || msg?.whisperTargets || msg?.whisper_targets;
  if (Array.isArray(list) && list.length > 0) {
    const first = list[0];
    if (typeof first === 'string') {
      return first;
    }
    if (first && typeof first === 'object' && first.id) {
      return first.id;
    }
  }
  const camel = msg?.whisperTo;
  if (typeof camel === 'string') {
    return camel;
  }
  if (camel && typeof camel === 'object' && camel.id) {
    return camel.id;
  }
  const snake = msg?.whisper_to;
  if (typeof snake === 'string') {
    return snake;
  }
  if (snake && typeof snake === 'object' && snake.id) {
    return snake.id;
  }
  const target = msg?.whisper_target;
  if (target && typeof target === 'object' && target.id) {
    return target.id;
  }
  return null;
};

const resolveMessageIdentityId = (msg?: any): string | null => {
  if (!msg) {
    return null;
  }
  const directIdentity = msg.identity || msg.identity_info || msg.identityData;
  if (directIdentity && typeof directIdentity === 'object' && directIdentity.id) {
    return directIdentity.id;
  }
  const camelRole = msg?.senderRoleId || msg?.senderRoleID;
  if (typeof camelRole === 'string' && camelRole.trim().length > 0) {
    return camelRole;
  }
  const snakeRole = msg?.sender_role_id;
  if (typeof snakeRole === 'string' && snakeRole.trim().length > 0) {
    return snakeRole;
  }
  const memberIdentity = msg?.member?.identity;
  if (memberIdentity && typeof memberIdentity === 'object' && memberIdentity.id) {
    return memberIdentity.id;
  }
  return null;
};

const findIdentityMeta = (channelId?: string, identityId?: string | null) => {
  if (!channelId || !identityId) {
    return null;
  }
  const list = chat.channelIdentities[channelId] || [];
  return list.find((item) => item.id === identityId) || null;
};

const resolveIdentityPreviewInfo = (channelId?: string, identityId?: string | null) => {
  const identity = findIdentityMeta(channelId, identityId);
  if (!identity) {
    return null;
  }
  return {
    displayName: identity.displayName,
    avatar: identity.avatarAttachmentId ? resolveAttachmentUrl(identity.avatarAttachmentId) : '',
    color: identity.color,
  };
};

const resolveMessageUserId = (msg?: Message) => (
  msg?.user?.id
  || (msg as any)?.user_id
  || (msg as any)?.member?.user?.id
  || (msg as any)?.member?.userId
  || (msg as any)?.member?.user_id
  || ''
);

const canEditMessage = (target?: Message) => {
  if (!target?.id || !chat.curChannel?.id) {
    return false;
  }
  const targetUserId = resolveMessageUserId(target);
  if (!targetUserId) {
    return false;
  }
  if (targetUserId === user.info.id) {
    return true;
  }
  const worldId = chat.currentWorldId;
  if (!worldId) {
    return false;
  }
  const detail = chat.worldDetailMap[worldId];
  const allowAdminEdit = detail?.allowAdminEditMessages
    ?? detail?.world?.allowAdminEditMessages
    ?? chat.worldMap[worldId]?.allowAdminEditMessages;
  if (!allowAdminEdit) {
    return false;
  }
  const isWorldAdmin = detail?.memberRole === 'owner'
    || detail?.memberRole === 'admin'
    || detail?.world?.ownerId === user.info.id
    || chat.worldMap[worldId]?.ownerId === user.info.id;
  if (!isWorldAdmin) {
    return false;
  }
  if (chat.isChannelAdmin(chat.curChannel.id, targetUserId)) {
    return false;
  }
  return true;
};

const beginEdit = (target?: Message) => {
  if (!target?.id || !chat.curChannel?.id) {
    return;
  }
  if (!canEditMessage(target)) {
    message.error('无权编辑该消息');
    return;
  }
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  chat.curReplyTo = null;
  chat.clearWhisperTargets();
  const detectedMode = detectMessageContentMode(target.content);
  const whisperTargetId = resolveMessageWhisperTargetId(target);
  const identityId = resolveMessageIdentityId(target);
  const icMode = String(target.icMode ?? target.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic';
  chat.startEditingMessage({
    messageId: target.id,
    channelId: chat.curChannel.id,
    originalContent: target.content || '',
    draft: target.content || '',
    mode: detectedMode,
    isWhisper: Boolean(target.isWhisper),
    whisperTargetId,
    icMode,
    identityId: identityId || null,
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
  const rawDraft = textToSend.value;
  const processedDraft = inputMode.value === 'rich' ? rawDraft : replaceEmojiRemarks(rawDraft);
  const hasImages = containsInlineImageMarker(processedDraft);
  if (processedDraft.trim() === '' && !hasImages) {
    message.error('消息内容不能为空');
    return;
  }
  if (processedDraft.length > 10000) {
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
        finalContent = processedDraft;
      }
    } else {
      finalContent = await buildMessageHtml(processedDraft);
    }
    if (finalContent.trim() === '') {
      message.error('消息内容不能为空');
      return;
    }
    const updateIcMode = chat.editing.icMode;
    const updateOptions: { icMode?: 'ic' | 'ooc'; identityId?: string | null } = {};
    if (updateIcMode) {
      updateOptions.icMode = updateIcMode;
    }
    if (chat.editing.identityId !== chat.editing.initialIdentityId) {
      updateOptions.identityId = chat.editing.identityId ?? null;
    }
    const hasOptions = Object.keys(updateOptions).length > 0;
    const updated = await chat.messageUpdate(
      chat.editing.channelId,
      chat.editing.messageId,
      finalContent,
      hasOptions ? updateOptions : undefined,
    );
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
  void loadWhisperCandidateColors();
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

const loadWhisperCandidateColors = async () => {
  const channelId = chat.curChannel?.id || '';
  if (!channelId || channelId.length >= 30) {
    whisperCandidateColorMap.value = new Map();
    whisperMentionableCandidates.value = [];
    return;
  }
  try {
    const resp = await chat.fetchMentionableMembers(channelId);
    const items = resp?.items || [];
    const candidates = buildWhisperCandidates(items);
    const nextMap = new Map<string, string>();
    for (const candidate of candidates) {
      if (candidate.color) {
        nextMap.set(candidate.id, candidate.color);
      }
    }
    whisperCandidateColorMap.value = nextMap;
    whisperMentionableCandidates.value = candidates;
  } catch (error) {
    console.warn('获取悄悄话候选成员颜色失败', error);
    whisperMentionableCandidates.value = [];
  }
};

const onWhisperTargetToggle = (candidate: WhisperCandidate) => {
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
  (targetUser as any).color = candidate.color || '';
  chat.toggleWhisperTarget(targetUser);
};

const confirmWhisperSelection = () => {
  chat.confirmWhisperTargets();
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
      onWhisperTargetToggle(selected);
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
  if (whisperPanelVisible.value || chat.whisperTargets.length > 0) {
    closeWhisperPanel();
    clearWhisperTargets();
    return;
  }
  openWhisperPanel('manual');
};

const clearWhisperTargets = () => {
  chat.clearWhisperTargets();
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

const normalizePlaceholderWhitespace = (value: string) => {
  const lines = value.split('\n');
  const result: string[] = [];
  const blankBuffer: string[] = [];

  const flushPendingBlanks = () => {
    if (!blankBuffer.length) {
      return;
    }
    result.push(...blankBuffer);
    blankBuffer.length = 0;
  };

  lines.forEach((line) => {
    const trimmed = line.trim();
    if (!trimmed) {
      if (result[result.length - 1]?.trim() === '[图片]') {
        blankBuffer.length = 0;
        return;
      }
      blankBuffer.push('');
      return;
    }

    if (trimmed === '[图片]') {
      blankBuffer.length = 0;
      result.push('[图片]');
      return;
    }

    flushPendingBlanks();
    result.push(line);
  });

  flushPendingBlanks();
  return result.join('\n');
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

  // 将 <at> 标签转换为 @名字 格式
  let replaced = value.replace(/<at\s+id="[^"]*"(?:\s+name="([^"]*)")?\s*\/>/g, (_, name) => {
    return `@${name || '用户'}`;
  });
  // 替换图片标记为 [图片]
  replaced = replaced.replace(/\[\[图片:[^\]]+\]\]/g, '[图片]');
  return normalizePlaceholderWhitespace(replaced);
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
const diceChipIconSvg = '<span class="dice-chip__icon" aria-hidden="true">🎲</span>';
const resolveDiceToneClass = () => (chat.icMode === 'ooc' ? 'ooc' : 'ic');
const buildPreviewDiceChip = (match: DiceMatch, index: number) => {
  const source = escapeHtml(match.source);
  const formula = escapeHtml(match.normalized);
  const tone = resolveDiceToneClass();
  return `<span class="dice-chip dice-chip--preview dice-chip--tone-${tone}" data-dice-tone="${tone}" data-index="${index}" title="${source}">${diceChipIconSvg}<span class="dice-chip__formula">${formula}</span><span class="dice-chip__equals">=</span><span class="dice-chip__result">?</span></span>`;
};

const renderDicePreviewSegment = (text: string) => {
  if (!text) return '';
  const matches = matchDiceExpressions(text, defaultDiceExpr.value);
  if (!matches.length) {
    return escapeHtml(text);
  }
  let html = '';
  let cursor = 0;
  matches.forEach((match, index) => {
    if (match.start > cursor) {
      html += escapeHtml(text.slice(cursor, match.start));
    }
    html += buildPreviewDiceChip(match, index);
    cursor = match.end;
  });
  if (cursor < text.length) {
    html += escapeHtml(text.slice(cursor));
  }
  return html;
};

const renderPreviewContent = (value: string) => {
  // 检测是否为 TipTap JSON
  if (isTipTapJson(value)) {
    try {
      const json = JSON.parse(value);
      const html = tiptapJsonToHtml(json, {
        baseUrl: urlBase,
        imageClass: 'preview-inline-image',
        linkClass: 'text-blue-500',
        attachmentResolver: resolveAttachmentUrl,
      });
      return DOMPurify.sanitize(html);
    } catch {
      // 如果解析失败，继续处理为普通文本
    }
  }

  // 预览模式：将 <at> 标签转换为简单的 @名字 格式
  let processedValue = value.replace(/<at\s+id="[^"]*"(?:\s+name="([^"]*)")?\s*\/>/g, (_, name) => {
    return `@${name || '用户'}`;
  });

  // 处理普通文本和图片标记
  const imageMarkerRegex = /\[\[(?:图片:([^\]]+)|img:id:([^\]]+))\]\]/g;
  let result = '';
  let lastIndex = 0;

  let match;
  while ((match = imageMarkerRegex.exec(processedValue)) !== null) {
    // 添加标记前的文本
    if (match.index > lastIndex) {
      result += renderDicePreviewSegment(processedValue.substring(lastIndex, match.index));
    }

    // 添加图片
    if (match[1]) {
      // [[图片:markerId]] 格式
      const markerId = match[1];
      const imageInfo = inlineImages.get(markerId);
      if (imageInfo && imageInfo.previewUrl) {
        result += `<img src="${imageInfo.previewUrl}" class="preview-inline-image" alt="图片" />`;
      } else {
        result += '<span class="preview-image-placeholder">[图片]</span>';
      }
    } else if (match[2]) {
      // [[img:id:attachmentId]] 格式
      const attachmentId = match[2];
      const resolved = resolveAttachmentUrl(`id:${attachmentId}`);
      result += `<img src="${resolved}" class="preview-inline-image" alt="图片" />`;
    }

    lastIndex = match.index + match[0].length;
  }

  // 添加剩余文本
  if (lastIndex < processedValue.length) {
    result += renderDicePreviewSegment(processedValue.substring(lastIndex));
  }

  return DOMPurify.sanitize(result || processedValue);
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
  let sanitizedDraft = draft.replace(inlineImageMarkerRegexp, (_, markerId) => {
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

  // 保护 Satori <at> 标签，避免被 contentEscape 转义
  const atTagRegexp = /<at\s+id="([^"]+)"(?:\s+name="([^"]*)")?\s*\/>/g;
  let atIndex = 0;
  sanitizedDraft = sanitizedDraft.replace(atTagRegexp, (match) => {
    const placeholder = `__SATORI_AT_${atIndex++}__`;
    placeholderMap.set(placeholder, match);
    return placeholder;
  });

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

  // 将多余空行折叠为单个换行，让图片占据当前空行
  while (cursor >= 2 && updatedText[cursor - 1] === '\n' && updatedText[cursor - 2] === '\n') {
    updatedText = updatedText.slice(0, cursor - 1) + updatedText.slice(cursor);
    cursor -= 1;
  }

  while (cursor < updatedText.length && updatedText[cursor] === '\n' && (cursor === 0 || updatedText[cursor - 1] === '\n')) {
    updatedText = updatedText.slice(0, cursor) + updatedText.slice(cursor + 1);
  }

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
    clearInputModeCache();
    textToSend.value = '';
    return;
  }
  if (messageId && chat.editing) {
    if (previousId && previousId !== messageId) {
      stopEditingPreviewNow();
    }
    clearInputModeCache();
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
    chat.clearWhisperTargets();
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
  if (spectatorInputDisabled.value) {
    message.warning('旁观者仅可查看频道内容，无法发送消息');
    return;
  }
  if (isEditing.value) {
    await saveEdit();
    return;
  }
  if (chat.connectState !== 'connected') {
    message.error('尚未连接，请稍等');
    return;
  }
  const sendMode = inputMode.value;
  const channelKey = currentChannelKey.value;
  let draft = textToSend.value;
  let identityIdOverride: string | undefined;

  // 仅纯文本模式支持 `/角色名 内容` 快捷切换
  if (inputMode.value === 'plain' && chat.curChannel?.id && draft.startsWith('/')) {
    const shortcutMatch = /^\/(\S+)\s+([\s\S]*)$/.exec(draft);
    if (shortcutMatch) {
      const targetName = shortcutMatch[1];
      const restContent = shortcutMatch[2] || '';
      const identities = chat.channelIdentities[chat.curChannel.id] || [];
      const matched = identities.find(item => item.displayName === targetName);
      if (matched) {
        chat.setActiveIdentity(chat.curChannel.id, matched.id);
        draft = restContent;
        textToSend.value = restContent;
        emitTypingPreview();
        identityIdOverride = matched.id;
      }
    }
  }

  // 检查是否为富文本模式
  const isRichMode = sendMode === 'rich';
  const diceMatchesInDraft = !isRichMode ? matchDiceExpressions(draft, defaultDiceExpr.value) : [];

  // 替换表情备注为图片标记
  if (!isRichMode) {
    draft = replaceEmojiRemarks(draft);
  }

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

  // 记录发送前的输入历史，便于失败后回溯
  appendHistoryEntry(sendMode, draft);

	const replyTo = chat.curReplyTo || undefined;
	stopTypingPreviewNow();
  suspendInlineSync = true;
  textToSend.value = '';
  clearInputModeCache();
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
  const activeIdentity = chat.getActiveIdentity(chat.curChannel?.id);
  if (activeIdentity) {
    const normalizedIdentityColor = normalizeHexColor(activeIdentity.color || '') || undefined;
    (tmpMsg as any).senderRoleId = activeIdentity.id;
    (tmpMsg as any).sender_role_id = activeIdentity.id;
    if (!tmpMsg.identity) {
      tmpMsg.identity = {
        id: activeIdentity.id,
        displayName: activeIdentity.displayName,
        color: normalizedIdentityColor,
        avatarAttachment: activeIdentity.avatarAttachmentId,
      } as any;
    }
    if (activeIdentity.displayName) {
      (tmpMsg as any).sender_member_name = activeIdentity.displayName;
    }
  }
  (tmpMsg as any).clientId = clientId;
  if (chat.curChannel) {
    (tmpMsg as any).channel = chat.curChannel;
  }

  const whisperTargetsForSend = chat.whisperTargets.slice();
  if (whisperTargetsForSend.length > 0) {
    (tmpMsg as any).isWhisper = true;
    (tmpMsg as any).whisperTo = whisperTargetsForSend[0];
    (tmpMsg as any).whisperToIds = whisperTargetsForSend;
  }

	(tmpMsg as any).failed = false;
	rows.value.push(tmpMsg);
	sortRowsByDisplayOrder();
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
		const newMsg = await chat.messageCreate(
			finalContent,
			replyTo?.id,
			whisperTargetsForSend[0]?.id,
			clientId,
			identityIdOverride,
		);
    if (!newMsg) {
      throw new Error('message.create returned empty result');
    }
    for (const [k, v] of Object.entries(newMsg as Record<string, any>)) {
      (tmpMsg as any)[k] = v;
    }
    if (diceMatchesInDraft.length) {
      diceMatchesInDraft.forEach((entry) => recordDiceHistory(entry.source.trim()));
    }
    instantMessages.delete(tmpMsg);
    upsertMessage(tmpMsg);
    resetInlineImages();
    pendingInlineSelection = null;

    if (channelKey) {
      clearAutoRestoreEntry(channelKey);
    }
    textToSend.value = '';
    clearInputModeCache();
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

const handleDiceInsert = (expr: string) => {
  insertDiceExpression(expr.trim() ? `${expr.trim()} ` : expr);
  ensureInputFocus();
};

const handleDiceRollNow = (expr: string) => {
  // 骰子"立即掷骰"功能：直接发送表达式，不插入到输入框
  // 支持快速连续点击，每次点击都独立发送一条消息
  const trimmedExpr = expr.trim();
  if (!trimmedExpr) return;
  
  // 临时设置要发送的内容
  textToSend.value = trimmedExpr;
  // 先调用 send() 创建待处理的调用，再 flush() 立即执行
  send();
  send.flush();
  // 发送后立即清空，为下次点击做准备
  nextTick(() => {
    textToSend.value = '';
  });
};

const handleDiceDefaultUpdate = async (expr: string) => {
  try {
    await chat.updateChannelDefaultDice(expr);
    message.success('默认骰已更新');
  } catch (error: any) {
    message.error(error?.message || '更新失败');
  }
};

watch(textToSend, (value) => {
  handleWhisperCommand(value);
  scheduleHistorySnapshot();
  checkKeywordSuggest();
  if (isEditing.value) {
    chat.updateEditingDraft(value);
    emitEditingPreview();
  } else {
    emitTypingPreview();
  }
  syncSelfTypingPreview();
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

watch(
  () => chat.curChannel?.id,
  (channelId, previous) => {
    if (channelId === previous) {
      return;
    }
    whisperCandidateColorMap.value = new Map();
    whisperMentionableCandidates.value = [];
    if (whisperPanelVisible.value) {
      void loadWhisperCandidateColors();
    }
  },
);

watch([
  inputPreviewEnabled,
  inputMode,
  inputIcMode,
  () => chat.curChannel?.id,
  () => activeIdentityForPreview.value?.id,
], () => {
  syncSelfTypingPreview();
});

watch(isEditing, (editing) => {
  if (editing) {
    removeSelfTypingPreview();
    return;
  }
  syncSelfTypingPreview();
});

watch(
  () => activeIdentityForPreview.value?.id,
  (identityId, previous) => {
    if (!chat.editing || chat.editing.channelId !== chat.curChannel?.id || identityId === previous) {
      return;
    }
    chat.updateEditingIdentity(identityId || null);
    emitEditingPreview();
  },
);

watch(() => chat.whisperTargets.map((target) => target.id).join(','), (targetIds, prevIds) => {
  if (targetIds === prevIds) {
    return;
  }
  if (targetIds && whisperCandidateColorMap.value.size === 0) {
    void loadWhisperCandidateColors();
  }
  stopTypingPreviewNow();
  emitTypingPreview();
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
      // 富文本模式不截断 JSON，否则会破坏 JSON 结构导致无法渲染
      const isRich = inputMode.value === 'rich' || isTipTapJson(raw);
      const truncated = isRich ? raw : (raw.length > 3000 ? raw.slice(0, 3000) : raw);
      sendTypingUpdate.cancel();
      const content = mode === 'content' ? truncated : '';
      const whisperId = resolveCurrentWhisperTargetId();
      const extra = whisperId ? { whisperTo: whisperId } : undefined;
      lastTypingWhisperTargetId = whisperId ?? null;
      chat.messageTyping(mode, content, lastTypingChannelId, extra);
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

watch(() => identityForm.color, (value) => {
  if (!value) {
    return;
  }
  const trimmed = value.trim();
  if (trimmed !== value) {
    identityForm.color = trimmed;
    return;
  }
  const lower = trimmed.toLowerCase();
  if (lower !== trimmed) {
    identityForm.color = lower;
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
  updateViewMode('live');
  updateAnchorMessage(null);
};

const doUpload = () => {
  pendingInlineSelection = captureSelectionRange();
  inlineImageInputRef.value?.click?.();
}

const handleRichUploadButtonClick = () => {
  // 富文本编辑器内的上传按钮点击事件
  doUpload();
}

const clearInputModeCache = () => {
  richContentCache.value = null;
  plainTextFromRichCache.value = '';
};

const toggleInputMode = () => {
  if (inputMode.value === 'plain') {
    // Plain → Rich
    const currentPlain = textToSend.value;
    if (richContentCache.value && currentPlain === plainTextFromRichCache.value) {
      // 未修改，恢复缓存的富文本
      textToSend.value = richContentCache.value;
    } else {
      // 已修改或无缓存，将纯文本转为 TipTap JSON
      richContentCache.value = null;
      plainTextFromRichCache.value = '';
      if (currentPlain.trim() || containsInlineImageMarker(currentPlain)) {
        textToSend.value = JSON.stringify(buildRichContentFromPlain(currentPlain));
      } else {
        textToSend.value = '';
      }
    }
    inputMode.value = 'rich';
    message.info('已切换至富文本模式');
  } else {
    // Rich → Plain
    const currentRich = textToSend.value;
    if (isTipTapJson(currentRich)) {
      richContentCache.value = currentRich;
      const { text, drafts } = convertRichContentToPlain(currentRich);
      plainTextFromRichCache.value = text;
      suspendInlineSync = true;
      applyInlineImageDrafts(drafts);
      textToSend.value = text;
      suspendInlineSync = false;
      syncInlineMarkersWithText(text);
    } else {
      // 非 TipTap JSON（可能是空内容或纯文本），直接清空缓存
      richContentCache.value = null;
      plainTextFromRichCache.value = '';
      // textToSend 保持原样
    }
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
    requestAnimationFrame(() => {
      const elLst = messagesListRef.value;
      if (!elLst) {
        return;
      }
      elLst.scrollTop = elLst.scrollHeight;
      requestAnimationFrame(() => {
        const retry = messagesListRef.value;
        if (!retry) {
          return;
        }
        const offset = retry.scrollHeight - (retry.clientHeight + retry.scrollTop);
        if (offset > 1) {
          retry.scrollTop = retry.scrollHeight;
        }
      });
    });
  });
}

const emit = defineEmits(['drawer-show'])

let firstLoad = false;
onMounted(async () => {
  await chat.tryInit();
  await utils.configGet();
  if (!chat.isObserver) {
    await utils.commandsRefresh();
  }

  chat.channelRefreshSetup()

  refreshHistoryEntries();
  scheduleHistoryAutoRestore();

  // 检查并启动新用户引导
  if (!chat.isObserver) {
    onboarding.checkAndStartOnboarding();
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

  chatEvent.off('message-removed', '*');
  chatEvent.on('message-removed', (e?: Event) => {
    const targetId = e?.message?.id;
    if (!targetId) {
      return;
    }
    for (let i of rows.value) {
      if (i.id === targetId) {
        i.content = '';
        (i as any).is_deleted = true;
      }
      if (i.quote && i.quote.id === targetId) {
        i.quote.content = '';
        (i.quote as any).is_deleted = true;
      }
    }
    rows.value = rows.value.filter((msg) => !(msg as any).is_deleted);
    if (archiveDrawerVisible.value) {
      const index = archivedMessagesRaw.value.findIndex((item) => item.id === targetId);
      if (index >= 0) {
        archivedMessagesRaw.value.splice(index, 1);
      }
    }
  });

chatEvent.off('message-created', '*');
chatEvent.on('message-created', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  if (hasCardRefreshCommand(incoming.content || '')) {
    scheduleCharacterSheetRefresh();
  }
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
      toBottom();
      return;
    }
  } else {
    sound.play();

    // 检测是否被 @ 了（包括 @all）
    const content = incoming.content || '';
    const currentUserId = user.info.id;
    const isMentioned = content.includes(`id="${currentUserId}"`) || content.includes('id="all"');

    if (isMentioned) {
      // 被 @ 时播放额外提示音或特殊处理
      import('naive-ui').then(({ useMessage }) => {
        const message = useMessage();
        const senderName = incoming.identity?.displayName
          || (incoming as any).sender_member_name
          || incoming.member?.nick
          || incoming.user?.nick
          || '有人';
        message.info(`${senderName} @ 了你`);
      });
    }

    // 如果窗口没有焦点，更新网页标题提示新消息
    if (!chat.isAppFocused && chat.curChannel?.name) {
      import('@/stores/utils').then(({ updateUnreadTitleNotification }) => {
        // 累加标题中的未读计数
        const currentTitle = document.title;
        const match = currentTitle.match(/^有(\d+)条新消息/);
        const currentCount = match ? parseInt(match[1], 10) : 0;
        updateUnreadTitleNotification(currentCount + 1, chat.curChannel?.name || '新消息');
      });
    }
    
    // 前台推送通知（页面打开但切换了标签页）
    if (!document.hasFocus()) {
      import('@/stores/pushNotification').then(({ usePushNotificationStore }) => {
        const pushStore = usePushNotificationStore();
        if (pushStore.enabled) {
          // 提取发送者名字
          const senderName = incoming.identity?.displayName
            || (incoming as any).sender_member_name
            || incoming.member?.nick
            || incoming.user?.nick
            || '新消息';
          
          // 提取消息内容预览（移除 HTML 标签）
          const rawContent = incoming.content || '';
          const plainText = rawContent.replace(/<[^>]*>/g, '').trim();
          const preview = plainText.length > 50 ? plainText.slice(0, 50) + '...' : plainText;
          
          // 获取发送者头像（优先角色头像，其次用户/成员头像）
          const avatarUrl = resolveAttachmentUrl((incoming.identity as any)?.avatarAttachmentId)
            || incoming.member?.avatar
            || incoming.user?.avatar
            || undefined;
          
          pushStore.showNotification(
            chat.curChannel?.name || 'SealChat',
            `${senderName}: ${preview || '发送了一条消息'}`,
            chat.curChannel?.id || '',
            avatarUrl
          );
        }
      });
    }
  }
  upsertMessage(incoming);
  removeTypingPreview(incoming.user?.id);
  removeTypingPreview(incoming.user?.id, 'editing');
  if (isSelf) {
    toBottom();
  } else if (!inHistoryMode.value && !historyLocked.value) {
    nextTick(() => {
      scrollToBottom();
    });
  }
});

chatEvent.off('message-updated', '*');
chatEvent.on('message-updated', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  if (e.user?.id && incoming?.user?.id) {
    const editorName = (e.user as any).nick
      || (e.user as any).name
      || (e.user as any).username
      || '';
    (incoming as any).editedByUserId = e.user.id;
    (incoming as any).editedByUserName = editorName || (incoming as any).editedByUserName || '';
  }
  upsertMessage(incoming);
  removeTypingPreview(e.user?.id, 'editing');
  if (chat.editing && chat.editing.messageId === e.message.id) {
    stopEditingPreviewNow();
    chat.cancelEditing();
    clearInputModeCache();
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

chatEvent.off('message-archived', '*');
chatEvent.on('message-archived', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  incoming.isArchived = true;
  upsertMessage(incoming as Message);
  if (!chat.filterState.showArchived) {
    const index = rows.value.findIndex(item => item.id === incoming.id);
    if (index >= 0) {
      rows.value.splice(index, 1);
    }
  }
  if (archiveDrawerVisible.value) {
    const entry = toArchivedPanelEntry(incoming as Message);
    const index = archivedMessagesRaw.value.findIndex(item => item.id === entry.id);
    if (index >= 0) {
      archivedMessagesRaw.value.splice(index, 1, entry);
    } else {
      archivedMessagesRaw.value.unshift(entry);
    }
  }
});

chatEvent.off('message-unarchived', '*');
chatEvent.on('message-unarchived', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  incoming.isArchived = false;
  upsertMessage(incoming as Message);
  const exists = rows.value.some(item => item.id === incoming.id);
  if (!exists) {
    rows.value.push(incoming as Message);
    sortRowsByDisplayOrder();
  }
  if (archiveDrawerVisible.value) {
    const index = archivedMessagesRaw.value.findIndex(item => item.id === incoming.id);
    if (index >= 0) {
      archivedMessagesRaw.value.splice(index, 1);
    }
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
  const avatar =
    identityAvatar ||
    e.member?.avatar ||
    e.user?.avatar ||
    '';
	upsertTypingPreview({
		userId: typingUserId,
		displayName,
		avatar,
		color: identityColor,
		content: typingState === 'content' ? (e.typing?.content || '') : '',
		indicatorOnly: typingState !== 'content' || !e.typing?.content,
		mode,
		messageId: e.typing?.messageId,
		tone: resolveTypingTone(e.typing),
		orderKey: typeof e.typing?.orderKey === 'number' ? e.typing.orderKey : Number.NaN,
	});
});

chatEvent.off('channel-presence-updated', '*');
chatEvent.on('channel-presence-updated', (e?: Event) => {
  const channelId = e?.channel?.id || '';
  if (!e?.presence || channelId !== chat.curChannel?.id) {
    return;
  }
  if (channelId !== presenceBadgeChannelId) {
    presenceBadgeChannelId = channelId;
    presenceBadgeInitialized = false;
    presenceBadgeUsers.clear();
  }
  let hasNewPresence = false;
  if (typeof (e as any)?.timestamp === 'number') {
    chat.syncServerTime((e as any).timestamp);
  }
  e.presence.forEach((item) => {
    const userId = item?.user?.id;
    if (!userId) {
      return;
    }
    if (!presenceBadgeUsers.has(userId)) {
      presenceBadgeUsers.add(userId);
      if (presenceBadgeInitialized) {
        hasNewPresence = true;
      }
    }
    chat.updatePresence(userId, {
      lastPing: typeof item?.lastSeen === 'number' ? chat.serverTsToLocal(item.lastSeen) : Date.now(),
      latencyMs: typeof item?.latency === 'number' ? item.latency : Number(item?.latency) || 0,
      isFocused: !!item?.focused,
    });
  });
  if (!presenceBadgeInitialized) {
    presenceBadgeInitialized = true;
    return;
  }
  if (hasNewPresence) {
    initCharacterCardBadge(channelId);
  }
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

  chatEvent.on('channel-identity-open', handleIdentityMenuOpen);
  chatEvent.on('channel-identity-updated', handleIdentityUpdated);

  chatEvent.on('connected', async (e) => {
    // 重连了之后，重新加载这之间的数据
    console.log('尝试获取重连数据')
    stopTypingPreviewNow();
    resetTypingPreview();
    if (rows.value.length > 0) {
      let now = Date.now();
      const lastCreatedAt = rows.value[rows.value.length - 1].createdAt || now;

      // 获取断线期间消息
      const messages = await chat.messageListDuring(chat.curChannel?.id || '', lastCreatedAt, now, {
        ...buildRoleFilterOptions(),
      })
      console.log('时间起始', lastCreatedAt, now)
      console.log('相关数据', messages)
      if (messages.next) {
        //  如果大于30个，那么基本上清除历史
        messageWindow.beforeCursor = messages.next || '';
        rows.value = rows.value.filter((i) => (i.createdAt || now) > lastCreatedAt);
      }
      // 插入新数据
      rows.value.push(...normalizeMessageList(messages.data));
      sortRowsByDisplayOrder();
      computeAfterCursorFromRows();

      // 滚动到最下方
      nextTick(() => {
        scrollToBottom();
        showButton.value = false;
        unlockHistoryView();
      })
    } else {
      await fetchLatestMessages();
    }
  })

  chatEvent.on('search-jump', async (e: any) => {
    if (!e?.messageId) return;
    await handleSearchJump({
      messageId: e.messageId,
      channelId: e.channelId,
      displayOrder: e.displayOrder,
      createdAt: e.createdAt,
    });
  });

  chatEvent.on('channel-switch-to', (e) => {
    if (!firstLoad) return;
  stopTypingPreviewNow();
  resetTypingPreview();
  stopEditingPreviewNow();
  chat.cancelEditing();
  textToSend.value = '';
  clearInputModeCache();
  resetWindowState('live');
  resetDragState();
  localReorderOps.clear();
  showButton.value = false;
    // 具体不知道原因，但是必须在这个位置reset才行
    // virtualListRef.value?.reset();
    refreshHistoryEntries();
    scheduleHistoryAutoRestore();
    const fetchTask = fetchLatestMessages();
    fetchTask.finally(() => {
      void maybePromptIdentitySync();
    });
  })

  await fetchLatestMessages();
  firstLoad = true;
  await maybePromptIdentitySync();
})

onBeforeUnmount(() => {
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  resetTypingPreview();
  disposeSelfPreviewObserver();
  disposeTypingViewportObserver();
  cancelDrag();
  stopTopObserver();
  stopBottomObserver();
  if (stRefreshTimer) {
    clearTimeout(stRefreshTimer);
    stRefreshTimer = null;
  }
});

const showButton = ref(false);
const historyHintVisible = computed(() => inHistoryMode.value || historyLocked.value);
const historyHintLabel = computed(() => (isMobileUa ? '历史' : '当前浏览历史消息'));

// 跳转到第一条未读消息相关
const hasFirstUnread = computed(() => {
  const info = chat.firstUnreadInfo;
  return !!(info && info.channelId === chat.curChannel?.id && info.messageId);
});

const jumpToFirstUnread = async () => {
  const info = chat.firstUnreadInfo;
  if (!info || info.channelId !== chat.curChannel?.id || !info.messageId) {
    return;
  }
  await handleSearchJump({
    messageId: info.messageId,
    createdAt: info.messageTime || undefined,
  });
  // 跳转后清除未读信息，避免重复跳转
  chat.firstUnreadInfo = null;
};

const dismissFirstUnread = () => {
  chat.firstUnreadInfo = null;
};

const computeAfterCursorFromRows = () => {
  updateWindowAnchorsFromRows();
};

const fetchOlderThanTimestamp = async (anchorTimestamp: number) => {
  let span = HISTORY_PAGINATION_WINDOW_MS;
  let attempts = 0;
  while (attempts < HISTORY_WINDOW_EXPANSION_LIMIT) {
    const from = Math.max(0, anchorTimestamp - span);
    const to = Math.max(from + 1, anchorTimestamp - 1);
    if (to <= from) {
      break;
    }
    try {
      const resp = await chat.messageListDuring(chat.curChannel!.id, from, to, {
        includeArchived: true,
        includeOoc: true,
        ...buildRoleFilterOptions(),
      });
      const normalized = normalizeMessageList(resp?.data || []).filter((msg) => {
        const created = normalizeTimestamp(msg.createdAt) ?? 0;
        return created < anchorTimestamp;
      });
      if (normalized.length) {
        const reachedStart = from === 0 && !resp?.next;
        return { messages: normalized, cursor: resp?.next ?? '', reachedStart };
      }
      if (from === 0) {
        return { messages: [], cursor: '', reachedStart: true };
      }
    } catch (error) {
      console.warn('按时间窗口加载旧消息失败', error);
      return { messages: [], cursor: '', reachedStart: false };
    }
    span *= 2;
    attempts += 1;
  }
  return { messages: [] as Message[], cursor: '', reachedStart: false };
};

const fetchNewerThanTimestamp = async (anchorTimestamp: number) => {
  let span = HISTORY_PAGINATION_WINDOW_MS;
  let attempts = 0;
  while (attempts < HISTORY_WINDOW_EXPANSION_LIMIT) {
    const from = Math.max(0, anchorTimestamp + 1);
    const to = anchorTimestamp + span;
    try {
      const resp = await chat.messageListDuring(chat.curChannel!.id, from, to, {
        includeArchived: true,
        includeOoc: true,
        ...buildRoleFilterOptions(),
      });
      const normalized = normalizeMessageList(resp?.data || []).filter((msg) => {
        const created = normalizeTimestamp(msg.createdAt) ?? 0;
        return created > anchorTimestamp;
      });
      if (normalized.length) {
        return {
          messages: normalized,
          reachedLatest: false,
        };
      }
      if (to >= Date.now()) {
        return { messages: [], reachedLatest: true };
      }
    } catch (error) {
      console.warn('按时间窗口加载新消息失败', error);
      return { messages: [], reachedLatest: false };
    }
    span *= 2;
    attempts += 1;
  }
  return { messages: [], reachedLatest: false };
};

const autoFillIfNeeded = async () => {
  await nextTick();
  const container = messagesListRef.value;
  if (!container) {
    return;
  }
  const shouldFill = container.scrollHeight <= container.clientHeight + 40;
  if (
    shouldFill &&
    !messageWindow.hasReachedStart &&
    !messageWindow.loadingBefore &&
    !messageWindow.autoFillPending
  ) {
    messageWindow.autoFillPending = true;
    const loaded = await loadOlderMessages();
    messageWindow.autoFillPending = false;
    if (loaded) {
      await autoFillIfNeeded();
    }
  }
};

const fetchLatestMessages = async () => {
  if (!chat.curChannel?.id || messageWindow.loadingLatest) {
    return;
  }
  const previousRows = rows.value.slice();
  resetWindowState('live', { preserveRows: true });
  resetTypingPreview();
  messageWindow.loadingLatest = true;
  try {
    const resp = await chat.messageList(chat.curChannel.id, undefined, {
      includeArchived: chat.filterState.showArchived,
      limit: INITIAL_MESSAGE_LOAD_LIMIT,
      ...buildRoleFilterOptions(),
    });
    rows.value = normalizeMessageList(resp.data);
    sortRowsByDisplayOrder();
    applyCursorUpdate({ before: resp?.next ?? '' });
    computeAfterCursorFromRows();
    await nextTick();
    scrollToBottom();
    showButton.value = false;
    await autoFillIfNeeded();
    tryAutoRestoreHistory();
  } catch (error) {
    rows.value = previousRows;
    resetWindowState('live', { preserveRows: true, preserveHistoryLock: false });
    throw error;
  } finally {
    messageWindow.loadingLatest = false;
  }
};

// Watch for showArchived filter changes to reload messages with archived content
watch(
  () => chat.filterState.showArchived,
  async (showArchived, prevShowArchived) => {
    // Only reload when switching to "show archived" mode
    // When hiding, the client-side filter in visibleRowEntries handles it
    if (showArchived && !prevShowArchived && chat.curChannel?.id) {
      await fetchLatestMessages();
    }
  },
);

watch(
  () => roleFilterSignature.value,
  async (next, prev) => {
    if (!chat.curChannel?.id || next === prev) {
      return;
    }
    await fetchLatestMessages();
  },
);

const loadOlderMessagesByWindow = async () => {
  const first = rows.value[0];
  const boundary = normalizeTimestamp(first?.createdAt);
  if (boundary === null || boundary === undefined) {
    return { messages: [] as Message[], cursor: '', reachedStart: false };
  }
  const result = await fetchOlderThanTimestamp(boundary);
  return result;
};

const loadOlderMessages = async () => {
  if (!chat.curChannel?.id || messageWindow.loadingBefore || messageWindow.hasReachedStart) {
    return false;
  }
  messageWindow.loadingBefore = true;
  try {
    const container = messagesListRef.value;
    const prevScrollHeight = container?.scrollHeight ?? 0;
    const prevScrollTop = container?.scrollTop ?? 0;
    let normalized: Message[] = [];
    let nextCursor: string | undefined;
    let reachedStart = false;
    const useCursor = Boolean(messageWindow.beforeCursor);

    if (useCursor) {
      const resp = await chat.messageList(chat.curChannel.id, messageWindow.beforeCursor, {
        includeArchived: chat.filterState.showArchived,
        limit: PAGINATED_MESSAGE_LOAD_LIMIT,
        ...buildRoleFilterOptions(),
      });
      normalized = normalizeMessageList(resp.data);
      nextCursor = resp?.next ?? '';
      if (!normalized.length && !nextCursor) {
        // Cursor已耗尽但仍有可能存在历史数据，改用时间窗口重试
        const fallback = await loadOlderMessagesByWindow();
        normalized = fallback.messages;
        nextCursor = fallback.cursor;
        reachedStart = fallback.reachedStart;
      }
    } else {
      const fallback = await loadOlderMessagesByWindow();
      normalized = fallback.messages;
      nextCursor = fallback.cursor;
      reachedStart = fallback.reachedStart;
    }

    if (nextCursor !== undefined) {
      applyCursorUpdate({ before: nextCursor ?? '' });
    }

    if (normalized.length) {
      const cursorPayload = nextCursor !== undefined ? { before: nextCursor ?? '' } : undefined;
      mergeIncomingMessages(normalized, cursorPayload);
      updateWindowAnchorsFromRows();
      messageWindow.hasReachedStart = false;
    }
    if (reachedStart) {
      messageWindow.hasReachedStart = true;
      messageWindow.beforeCursor = '';
      messageWindow.beforeCursorExhausted = true;
    }
    await nextTick();
    if (container) {
      const nextHeight = container.scrollHeight;
      const diff = nextHeight - prevScrollHeight;
      container.scrollTop = prevScrollTop + diff;
    }
    return normalized.length > 0;
  } finally {
    messageWindow.loadingBefore = false;
  }
};

const loadNewerMessages = async () => {
  if (
    !chat.curChannel?.id ||
    messageWindow.loadingAfter ||
    messageWindow.hasReachedLatest
  ) {
    return false;
  }
  const anchor =
    messageWindow.latestTimestamp ??
    normalizeTimestamp(rows.value[rows.value.length - 1]?.createdAt);
  if (anchor === null || anchor === undefined) {
    return false;
  }
  messageWindow.loadingAfter = true;
  try {
    const result = await fetchNewerThanTimestamp(anchor);
    if (result.messages.length) {
      mergeIncomingMessages(result.messages);
      updateWindowAnchorsFromRows();
      messageWindow.hasReachedLatest = false;
      return true;
    }
    if (result.reachedLatest) {
      messageWindow.hasReachedLatest = true;
      messageWindow.afterCursor = '';
      if (isNearBottom()) {
        updateViewMode('live');
      }
    }
    return false;
  } catch (error) {
    console.warn('加载较新消息失败', error);
    return false;
  } finally {
    messageWindow.loadingAfter = false;
  }
};

const handleBackToLatest = async () => {
  await fetchLatestMessages();
  unlockHistoryView();
};

const onScroll = () => {
  const container = messagesListRef.value;
  if (!container) {
    return;
  }
  hideSelectionBar()
  const offset = container.scrollHeight - (container.clientHeight + container.scrollTop);
  const stuckToBottom = offset <= SCROLL_STICKY_THRESHOLD;
  showButton.value = !stuckToBottom || historyLocked.value;
  if (!stuckToBottom) {
    updateViewMode('history');
    computeAfterCursorFromRows();
  } else if (!historyLocked.value) {
    updateViewMode('live');
  }
  // Removed duplicate trigger - IntersectionObserver on topSentinelRef handles loading older messages
};

const pauseKeydown = ref(false);

const handleMentionSelect = () => {
  pauseKeydown.value = false;
};

// 术语快捷输入相关函数
const performKeywordMatch = async (query: string) => {
  keywordSuggestLoading.value = true;
  try {
    await ensurePinyinLoaded();
    const keywords = worldGlossary.currentKeywords || [];
    const results = matchKeywords(query, keywords, 5);
    // 按分数升序排列（低分在上，高分在下靠近输入框）
    keywordSuggestOptions.value = results.sort((a, b) => a.score - b.score);
    keywordSuggestIndex.value = results.length > 0 ? results.length - 1 : 0;
    keywordSuggestVisible.value = results.length > 0;
  } finally {
    keywordSuggestLoading.value = false;
  }
};

const checkKeywordSuggest = () => {
  if (!display.settings.worldKeywordQuickInputEnabled) {
    keywordSuggestVisible.value = false;
    return;
  }

  const trigger = display.settings.worldKeywordQuickInputTrigger || '/';

  let text: string;
  let cursorPos: number;

  if (inputMode.value === 'rich') {
    // 富文本模式：从编辑器获取纯文本和光标位置
    const editorInstance = textInputRef.value?.getEditor?.();
    if (!editorInstance) {
      keywordSuggestVisible.value = false;
      return;
    }
    text = editorInstance.getText();
    cursorPos = editorInstance.state.selection.from - 1; // TipTap 的 from 是基于 1 的
  } else {
    // 纯文本模式
    text = textToSend.value;
    cursorPos = getInputSelection().start;
  }

  const beforeCursor = text.slice(0, cursorPos);

  // 查找最近的触发字符
  const slashIndex = beforeCursor.lastIndexOf(trigger);
  if (slashIndex === -1) {
    keywordSuggestVisible.value = false;
    return;
  }

  // 提取查询内容
  const query = beforeCursor.slice(slashIndex + 1);

  // 检测是否是快捷命令模式 (/e 空格 或 /w 空格) - 仅当触发字符为 / 时检查
  if (trigger === '/' && /^[ew]\s/.test(query)) {
    keywordSuggestVisible.value = false;
    return;
  }

  // 检测两个连续空格
  if (query.includes('  ')) {
    keywordSuggestVisible.value = false;
    return;
  }

  // 空查询时不显示
  if (!query.trim()) {
    keywordSuggestVisible.value = false;
    return;
  }

  // 执行匹配
  keywordSuggestSlashPos.value = slashIndex;
  keywordSuggestQuery.value = query;
  performKeywordMatch(query);
};

const handleKeywordSuggestKeydown = (e: KeyboardEvent): boolean => {
  if (!keywordSuggestVisible.value) return false;

  const options = keywordSuggestOptions.value;
  if (!options.length) return false;

  if (e.key === 'ArrowDown') {
    keywordSuggestIndex.value = Math.min(keywordSuggestIndex.value + 1, options.length - 1);
    e.preventDefault();
    return true;
  }

  if (e.key === 'ArrowUp') {
    keywordSuggestIndex.value = Math.max(keywordSuggestIndex.value - 1, 0);
    e.preventDefault();
    return true;
  }

  if (e.key === 'Enter' && !e.isComposing) {
    const selected = options[keywordSuggestIndex.value];
    if (selected) {
      applyKeywordSuggestion(selected);
      e.preventDefault();
      return true;
    }
  }

  if (e.key === 'Escape') {
    keywordSuggestVisible.value = false;
    e.preventDefault();
    return true;
  }

  return false;
};

const applyKeywordSuggestion = (result: KeywordMatchResult) => {
  const keyword = result.keyword.keyword;

  if (inputMode.value === 'rich') {
    // 富文本模式：使用 TipTap 编辑器 API
    const editorInstance = textInputRef.value?.getEditor?.();
    if (editorInstance) {
      // 删除触发字符和查询内容，然后插入术语
      const deleteCount = keywordSuggestQuery.value.length + 1; // +1 for trigger char
      editorInstance.chain()
        .focus()
        .deleteRange({
          from: editorInstance.state.selection.from - deleteCount,
          to: editorInstance.state.selection.from
        })
        .insertContent(keyword)
        .run();
    }
  } else {
    // 纯文本模式
    const slashPos = keywordSuggestSlashPos.value;
    const queryLen = keywordSuggestQuery.value.length;
    const cursorPos = slashPos + queryLen + 1; // +1 for trigger char

    const before = textToSend.value.slice(0, slashPos);
    const after = textToSend.value.slice(cursorPos);

    const newText = before + keyword + after;
    const newCursor = slashPos + keyword.length;

    textToSend.value = newText;

    // 使用双重 nextTick 确保 DOM 完全更新
    nextTick(() => {
      nextTick(() => {
        setInputSelection(newCursor, newCursor);
        textInputRef.value?.focus?.();
      });
    });
  }

  keywordSuggestVisible.value = false;
};

const handleKeywordSuggestSelect = (result: KeywordMatchResult) => {
  applyKeywordSuggestion(result);
};

const handleKeywordSuggestHover = (index: number) => {
  keywordSuggestIndex.value = index;
};

const handleKeywordSuggestBlur = () => {
  keywordSuggestVisible.value = false;
};

const toolbarHotkeyOrder: ToolbarHotkeyKey[] = [
  'icToggle',
  'whisper',
  'upload',
  'richMode',
  'broadcast',
  'emoji',
  'wideInput',
  'history',
  'diceTray',
];

const toolbarHotkeyHandlers: Record<ToolbarHotkeyKey, () => boolean | void> = {
  icToggle: () => {
    if (
      !icHotkeyEnabled.value ||
      isEditing.value ||
      dragState.activeId ||
      whisperPanelVisible.value
    ) {
      return false;
    }
    const nextMode: 'ic' | 'ooc' = inputIcMode.value === 'ic' ? 'ooc' : 'ic';
    inputIcMode.value = nextMode;
    emitTypingPreview();
    message.success(nextMode === 'ic' ? '已切换至场内模式' : '已切换至场外模式');
    return true;
  },
  whisper: () => {
    startWhisperSelection();
    return true;
  },
  upload: () => {
    doUpload();
    return true;
  },
  richMode: () => {
    toggleInputMode();
    return true;
  },
  broadcast: () => {
    toggleTypingPreview();
    return true;
  },
  emoji: () => {
    handleEmojiTriggerClick();
    return true;
  },
  wideInput: () => {
    toggleWideInputMode();
    return true;
  },
  history: () => {
    handleHistoryPopoverShow(!historyPopoverVisible.value);
    return true;
  },
  diceTray: () => {
    toggleDiceTray();
    return true;
  },
};

const handleToolbarHotkeyEvent = (event: KeyboardEvent) => {
  const configs = display.settings.toolbarHotkeys;
  for (const key of toolbarHotkeyOrder) {
    const config = configs[key];
    if (!config?.enabled || !config.hotkey) {
      continue;
    }
    if (!isHotkeyMatchingEvent(event, config.hotkey)) {
      continue;
    }
    const handler = toolbarHotkeyHandlers[key];
    if (!handler) {
      continue;
    }
    const result = handler();
    if (result !== false) {
      event.preventDefault();
      event.stopPropagation();
    }
    return result !== false;
  }
  return false;
};

const keyDown = function (e: KeyboardEvent) {
  if (pauseKeydown.value) return;

  // 优先处理术语快捷输入
  if (handleKeywordSuggestKeydown(e)) {
    return;
  }

  if (!isEditing.value && handleWhisperKeydown(e)) {
    return;
  }

  // 移动端不触发桌面快捷键
  if (isMobileUa) {
    return;
  }

  if (e.key === 'Backspace' && chat.whisperTargets.length > 0) {
    const selection = getInputSelection();
    if (selection.start === 0 && selection.end === 0 && textToSend.value.length === 0) {
      clearWhisperTargets();
      e.preventDefault();
      return;
    }
  }

  if (!e.isComposing && e.key === 'Backspace' && chat.curReplyTo) {
    const selection = getInputSelection();
    const atStart = selection.start <= 1 && selection.end <= 1;
    if (atStart && isInputEffectivelyEmpty()) {
      chat.curReplyTo = null;
      e.preventDefault();
      return;
    }
  }

  if (e.key === 'Escape' && isEditing.value) {
    cancelEditing();
    e.preventDefault();
    return;
  }

  if (handleToolbarHotkeyEvent(e)) {
    return;
  }

  if (e.key === 'Enter') {
    if (e.isComposing) {
      return;
    }
    const shortcut = display.settings.sendShortcut || 'enter';
    const ctrlLike = e.ctrlKey || e.metaKey;
    let shouldSend = false;
    if (shortcut === 'enter') {
      shouldSend = !ctrlLike && !e.shiftKey && !e.altKey;
    } else {
      shouldSend = ctrlLike && !e.shiftKey && !e.altKey;
    }
    if (shouldSend) {
      if (isEditing.value) {
        saveEdit();
      } else {
        send();
      }
      e.preventDefault();
    }
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
    case 'at': {
      const data = (option as any).data || {};
      const identityType = data.identityType;
      const color = data.color || 'inherit';
      const isAll = data.userId === 'all';
      return <div class="flex items-center space-x-2">
        {isAll ? (
          <span class="at-option-avatar at-option-avatar--all">@</span>
        ) : (
          <AvatarVue size={24} border={false} src={data.avatar} />
        )}
        <span style={{ color: isAll ? '#ef4444' : color }}>{option.label}</span>
        {identityType && identityType !== 'all' && (
          <span class={`at-option-tag at-option-tag--${identityType}`}>
            {identityType === 'ic' ? '场内' : identityType === 'ooc' ? '场外' : '用户'}
          </span>
        )}
      </div>
    }
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
      await ensurePinyinLoaded();
      const channelId = chat.curChannel?.id;
      if (!channelId) {
        atOptions.value = [];
        break;
      }
      const icMode = chat.icMode as 'ic' | 'ooc' | undefined;
      const result = await chat.fetchMentionableMembers(channelId, icMode);
      let lst: MentionOption[] = [];
      // @all option
      if (result.canAtAll) {
        const allMatches = !pattern || matchText(pattern, '全体成员') || pattern.toLowerCase() === 'all';
        if (allMatches) {
          lst.push({
            type: 'at',
            value: '<at id="all" name="全体成员"/>',
            label: '全体成员',
            data: { userId: 'all', displayName: '全体成员', identityType: 'all' },
          });
        }
      }
      // Filter and map members
      for (const item of result.items) {
        if (pattern && !matchText(pattern, item.displayName)) {
          continue;
        }
        const escapedName = item.displayName.replace(/"/g, '&quot;');
        lst.push({
          type: 'at',
          value: `<at id="${item.userId}" name="${escapedName}"/>`,
          label: item.displayName,
          data: item,
        });
      }
      atOptions.value = lst.slice(0, 10);
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

const { stop: stopTopObserver } = useIntersectionObserver(
  topSentinelRef,
  ([entry]) => {
    if (
      !entry?.isIntersecting ||
      !firstLoad ||
      messageWindow.loadingBefore ||
      messageWindow.hasReachedStart
    ) {
      return;
    }
    void loadOlderMessages();
  },
  {
    root: messagesListRef,
    threshold: 0.2,
  },
);

const { stop: stopBottomObserver } = useIntersectionObserver(
  bottomSentinelRef,
  ([entry]) => {
    if (
      !entry?.isIntersecting ||
      messageWindow.loadingAfter ||
      messageWindow.hasReachedLatest
    ) {
      return;
    }
    if (!inHistoryMode.value) {
      return;
    }
    void loadNewerMessages();
  },
  {
    root: messagesListRef,
    threshold: 0.2,
  },
);

const sendImageMessage = async (attachmentId: string) => {
  if (spectatorInputDisabled.value) {
    message.warning('旁观者仅可查看频道内容，无法发送消息');
    return false;
  }
  const normalized = attachmentId.startsWith('id:') ? attachmentId : `id:${attachmentId}`;
  const rawId = normalized.startsWith('id:') ? normalized.slice(3) : normalized;
  const resp = await chat.messageCreate(`<img src="id:${rawId}" />`);
  if (!resp) {
    message.error('发送失败,您可能没有权限在此频道发送消息');
    return false;
  }
  toBottom();
  return true;
};

const sendEmoji = throttle(async (item: GalleryItem) => {
  if (spectatorInputDisabled.value) {
    message.warning('旁观者仅可查看频道内容，无法发送消息');
    return;
  }
  if (await sendImageMessage(item.attachmentId)) {
    recordEmojiUsage(item.id);
    emojiPopoverShow.value = false;
  }
}, 1000);

const avatarLongpress = (data: any) => {
  if (isMobileUa) {
    return;
  }
  if (data.user) {
    textToSend.value += `@${data.user.nick} `;
    textInputRef.value?.focus();
  }
}

// Multi-select handlers
const allMessageIds = computed(() => rows.value.map(row => row.id));

const getMultiSelectedMessages = () => {
  if (!chat.multiSelect?.selectedIds.size) return [];
  const selected = Array.from(chat.multiSelect.selectedIds);
  return rows.value.filter(row => selected.includes(row.id));
};

const handleMultiSelectCopy = async () => {
  const messages = getMultiSelectedMessages();
  if (!messages.length) {
    message.warning('请先选择消息');
    return;
  }
  const text = messages.map(msg => {
    const time = msg.createdAt ? dayjs(msg.createdAt).format('YYYY-MM-DD HH:mm:ss') : '';
    const name = (msg as any).sender_member_name || (msg as any).identity?.displayName || msg.member?.nick || msg.user?.name || '未知';
    const content = typeof msg.content === 'string' ? msg.content.replace(/<[^>]*>/g, '') : '';
    return `[${time}] ${name}: ${content}`;
  }).join('\n');
  const copied = await copyTextWithFallback(text);
  if (copied) {
    message.success(`已复制 ${messages.length} 条消息`);
    chat.exitMultiSelectMode();
  } else {
    message.error('复制失败');
  }
};

const handleMultiSelectArchive = async () => {
  const ids = Array.from(chat.multiSelect?.selectedIds || []);
  if (!ids.length) {
    message.warning('请先选择消息');
    return;
  }
  try {
    await chat.archiveMessages(ids);
    message.success(`已归档 ${ids.length} 条消息`);
    chat.exitMultiSelectMode();
  } catch (e) {
    message.error('归档失败');
  }
};

const handleMultiSelectDelete = async () => {
  const ids = Array.from(chat.multiSelect?.selectedIds || []);
  if (!ids.length) {
    message.warning('请先选择消息');
    return;
  }
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    message.error('当前频道不可用');
    return;
  }
  dialog.warning({
    title: '批量删除',
    content: `确定要删除选中的 ${ids.length} 条消息吗？此操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        for (const id of ids) {
          await chat.messageRemove(channelId, id);
        }
        message.success(`已删除 ${ids.length} 条消息`);
        chat.exitMultiSelectMode();
      } catch (e) {
        message.error('删除失败');
      }
    },
  });
};

const handleMultiSelectCopyImage = async () => {
  const messages = getMultiSelectedMessages();
  if (!messages.length) {
    message.warning('请先选择消息');
    return;
  }
  try {
    const html2canvas = (await import('html2canvas')).default;
    
    // Find message elements in DOM
    const messageEls: HTMLElement[] = [];
    for (const msg of messages) {
      const el = document.getElementById(msg.id);
      if (el) messageEls.push(el);
    }
    if (!messageEls.length) {
      message.error('未找到消息元素');
      return;
    }

    // Get background color
    const rootStyles = getComputedStyle(document.documentElement);
    const bgColor = rootStyles.getPropertyValue('--sc-bg-base')?.trim() 
      || rootStyles.getPropertyValue('--chat-bg')?.trim()
      || getComputedStyle(document.body).backgroundColor
      || '#ffffff';

    // Render each message element in-place with onclone callback
    const canvases: HTMLCanvasElement[] = [];
    for (const el of messageEls) {
      const canvas = await html2canvas(el, {
        backgroundColor: bgColor,
        scale: 2,
        useCORS: true,
        allowTaint: true,
        logging: false,
        onclone: (clonedDoc, clonedEl) => {
          // Remove selection classes
          clonedEl.classList.remove('chat-item--multiselect', 'chat-item--selected');
          // Remove checkbox element
          const checkbox = clonedEl.querySelector('.chat-item__select-checkbox');
          if (checkbox) checkbox.remove();
        },
      });
      canvases.push(canvas);
    }

    // Calculate combined canvas size
    const totalHeight = canvases.reduce((sum, c) => sum + c.height, 0);
    const maxWidth = Math.max(...canvases.map(c => c.width));
    const padding = 16 * 2; // scale factor

    // Create combined canvas
    const combinedCanvas = document.createElement('canvas');
    combinedCanvas.width = maxWidth + padding * 2;
    combinedCanvas.height = totalHeight + padding * 2;
    const ctx = combinedCanvas.getContext('2d')!;
    
    // Fill background
    ctx.fillStyle = bgColor;
    ctx.fillRect(0, 0, combinedCanvas.width, combinedCanvas.height);

    // Draw each message canvas
    let y = padding;
    for (const canvas of canvases) {
      ctx.drawImage(canvas, padding, y);
      y += canvas.height;
    }

    // Copy to clipboard
    combinedCanvas.toBlob(async (blob) => {
      if (blob) {
        try {
          await navigator.clipboard.write([
            new ClipboardItem({ 'image/png': blob })
          ]);
          message.success('已复制为图片');
          chat.exitMultiSelectMode();
        } catch (e) {
          message.error('复制图片失败');
        }
      }
    }, 'image/png');
  } catch (e) {
    console.error(e);
    message.error('生成图片失败');
  }
};

const handleMultiSelectAll = () => {
  const allIds = rows.value.map(row => row.id);
  chat.selectMessagesByIds(allIds);
  message.info(`已选中 ${allIds.length} 条消息`);
};

const selectedEmojiIds = ref<string[]>([]);
const emojiRemarkModalVisible = ref(false);
const emojiRemarkInput = ref('');
const emojiRemarkSaving = ref(false);
const editingEmojiItem = ref<GalleryItem | null>(null);
const emojiRemarkPattern = /^[\p{L}\p{N}_]{1,64}$/u;

const resolveEmojiRemark = (item: GalleryItem, idx: number) => (item.remark?.trim() || `收藏${idx + 1}`);

const openEmojiRemarkEditor = (item: GalleryItem) => {
  editingEmojiItem.value = item;
  emojiRemarkInput.value = item.remark?.trim() || '';
  emojiRemarkModalVisible.value = true;
};

const submitEmojiRemark = async () => {
  if (!editingEmojiItem.value) {
    return false;
  }
  const remark = emojiRemarkInput.value.trim();
  if (!remark) {
    message.warning('备注不能为空');
    return false;
  }
  if (!emojiRemarkPattern.test(remark)) {
    message.warning('备注仅支持字母、数字和下划线，长度不超过64');
    return false;
  }
  emojiRemarkSaving.value = true;
  try {
    const collectionId = editingEmojiItem.value.collectionId;
    await gallery.updateItem(collectionId, editingEmojiItem.value.id, { remark });
    message.success('备注已更新');
    emojiRemarkModalVisible.value = false;
    return true;
  } catch (error: any) {
    console.error('更新表情备注失败', error);
    message.error(error?.message || '更新失败，请稍后再试');
    return false;
  } finally {
    emojiRemarkSaving.value = false;
  }
};

const cancelEmojiRemark = () => {
  if (emojiRemarkSaving.value) {
    return false;
  }
  emojiRemarkModalVisible.value = false;
  return true;
};

const exitEmojiManage = () => {
  isManagingEmoji.value = false;
  selectedEmojiIds.value = [];
};

const emojiSelectedDelete = async () => {
  if (!(await dialogAskConfirm(dialog))) return;

  if (!selectedEmojiIds.value.length) {
    message.info('没有选中的表情');
    return;
  }
  const collectionId = gallery.favoritesCollectionId;
  if (!collectionId) {
    message.error('未找到表情收藏分类');
    return;
  }
  try {
    await gallery.deleteItems(collectionId, selectedEmojiIds.value);
    message.success('已删除所选表情');
    selectedEmojiIds.value = [];
  } catch (error: any) {
    console.error('删除表情失败', error);
    message.error(error?.message || '删除失败，请稍后再试');
  }
};

const insertGalleryInline = (attachmentId: string) => {
  const normalized = attachmentId.startsWith('id:') ? attachmentId.slice(3) : attachmentId;
  if (inputMode.value === 'rich') {
    const editor = textInputRef.value?.getEditor?.();
    editor?.chain().focus().setImage({ src: `id:${normalized}` }).run();
    return;
  }

  const markerId = nanoid();
  const token = `[[图片:${markerId}]]`;
  const record: InlineImageDraft = reactive({
    id: markerId,
    token,
    status: 'uploaded',
    attachmentId: normalized,
  });
  inlineImages.set(markerId, record);

  const draft = textToSend.value;
  const selection = captureSelectionRange();
  const start = Math.max(0, Math.min(selection.start, selection.end));
  const end = Math.max(start, Math.max(selection.start, selection.end));
  textToSend.value = draft.slice(0, start) + token + draft.slice(end);
  const cursor = start + token.length;
  nextTick(() => setInputSelection(cursor, cursor));
  ensureInputFocus();
};

const getGalleryItemThumb = (item: GalleryItem) => {
  // Prefer gallery-saved thumbUrl if available (needs urlBase for dev environment)
  if (item.thumbUrl) {
    return `${urlBase}${item.thumbUrl}`;
  }
  return resolveEmojiAttachmentUrl(item.attachmentId);
};

const handleGalleryEmojiClick = (item: GalleryItem) => {
  recordEmojiUsage(item.id);
  insertGalleryInline(item.attachmentId);
};

const handleGalleryEmojiDragStart = (item: GalleryItem, evt: DragEvent) => {
  const dt = evt.dataTransfer;
  if (!dt) return;
  dt.effectAllowed = 'copy';
  try {
    dt.setData('application/x-sealchat-gallery-item', JSON.stringify({ attachmentId: item.attachmentId }));
  } catch (error) {
    console.warn('设置画廊拖拽数据失败', error);
  }
  dt.setData('text/plain', item.attachmentId);
};

const handleGalleryInsert = (src: string) => {
  const normalized = src.startsWith('id:') ? src.slice(3) : src;
  insertGalleryInline(normalized);
};

const handleGalleryDragOver = (event: DragEvent) => {
  const dt = event.dataTransfer;
  if (!dt) return;
  if (Array.from(dt.types || []).includes('application/x-sealchat-gallery-item')) {
    event.preventDefault();
    dt.dropEffect = 'copy';
  }
};

const handleGalleryDrop = async (event: DragEvent) => {
  const dt = event.dataTransfer;
  if (!dt) return;
  const data = dt.getData('application/x-sealchat-gallery-item');
  if (!data) {
    return;
  }
  event.preventDefault();
  try {
    const payload = JSON.parse(data) as { attachmentId?: string };
    if (payload?.attachmentId) {
      await sendImageMessage(payload.attachmentId);
    }
  } catch (error) {
    console.warn('解析画廊拖拽数据失败', error);
  }
};


onBeforeUnmount(() => {
  handleInputResizeEnd();
  chatEvent.off('channel-identity-open', handleIdentityMenuOpen);
  chatEvent.off('channel-identity-updated', handleIdentityUpdated);
  chatEvent.off('action-ribbon-toggle', handleActionRibbonToggleRequest);
  chatEvent.off('action-ribbon-state-request', handleActionRibbonStateRequest);
  chatEvent.off('open-display-settings', handleOpenDisplaySettings);
  revokeIdentityObjectURL();
  searchHighlightTimers.forEach((timer) => window.clearTimeout(timer));
  searchHighlightTimers.clear();
  if (isMobileUa) {
    markDiceTrayMobileWrapper(false);
  }
});
</script>

<template>
  <div class="flex flex-col h-full justify-between chat-root-container">
    <!-- 频道背景层 -->
    <div v-if="channelBackgroundStyle" class="channel-background-layer" :style="channelBackgroundStyle"></div>
    <div v-if="channelBackgroundOverlayStyle" class="channel-background-overlay" :style="channelBackgroundOverlayStyle"></div>
    <!-- 功能面板 -->
    <transition name="slide-down">
      <ChatActionRibbon
        v-if="showActionRibbon && !isEmbedMode"
        :filters="chat.filterState"
        :roles="ribbonRoleOptions"
        :archive-active="archiveDrawerVisible"
        :export-active="exportManagerVisible"
        :identity-active="identityDialogVisible"
        :gallery-active="galleryPanelVisible"
        :display-active="displaySettingsVisible"
        :favorite-active="display.favoriteBarEnabled"
        :channel-images-active="channelImagesPanelVisible"
        :can-import="canManageWorldKeywords"
        :import-active="importDialogVisible"
        :split-enabled="splitEntryEnabled"
        :split-active="false"
        :sticky-note-enabled="true"
        :sticky-note-active="stickyNoteStore.uiVisible"
        :webhook-enabled="webhookManageAllowed"
        :webhook-active="webhookDrawerVisible"
        :email-notification-enabled="true"
        :email-notification-active="emailNotificationDrawerVisible"
        :character-card-enabled="true"
        :character-card-active="characterCardPanelVisible"
        @update:filters="chat.setFilterState($event)"
        @open-archive="archiveDrawerVisible = true"
        @open-export="exportManagerVisible = true"
        @open-import="importDialogVisible = true"
        @open-identity-manager="openIdentityManager"
        @open-gallery="openGalleryPanel"
        @open-display-settings="displaySettingsVisible = true"
        @open-favorites="channelFavoritesVisible = true"
        @open-channel-images="openChannelImagesPanel"
        @open-split="openSplitView"
        @toggle-sticky-note="toggleStickyNotes"
        @open-webhook="webhookDrawerVisible = true"
        @open-email-notification="emailNotificationDrawerVisible = true"
        @open-character-card="characterCardPanelVisible = true"
        @clear-filters="chat.setFilterState({ icFilter: 'all', showArchived: false, roleIds: [] })"
      />
    </transition>

    <n-drawer v-model:show="webhookDrawerVisible" placement="right" :width="520">
      <n-drawer-content closable>
        <template #header>Webhook 授权</template>
        <WebhookIntegrationManager :channel-id="chat.curChannel?.id || ''" />
      </n-drawer-content>
    </n-drawer>

    <n-drawer v-model:show="emailNotificationDrawerVisible" placement="right" :width="480">
      <n-drawer-content closable>
        <template #header>邮件提醒</template>
        <EmailNotificationManager :channel-id="chat.curChannel?.id || ''" />
      </n-drawer-content>
    </n-drawer>

    <div
      v-if="selectionBar.visible"
      ref="selectionBarRef"
      class="selection-floating-bar"
      :style="{ top: `${selectionBar.position.y}px`, left: `${selectionBar.position.x}px` }"
    >
      <button class="selection-floating-bar__button" @click="handleSelectionCopy">
        <n-icon :component="CopyIcon" size="14" />
        复制
      </button>
      <button
        class="selection-floating-bar__button"
        :class="{ 'is-disabled': !canAddKeywordFromSelection }"
        :disabled="!canAddKeywordFromSelection"
        @click="handleSelectionAddKeyword"
      >
        <n-icon :component="Plus" size="14" />
        添加
      </button>
      <button class="selection-floating-bar__button" @click="handleSelectionSearch">
        <n-icon :component="SearchIcon" size="14" />
        搜索
      </button>
    </div>

    <div v-if="display.favoriteBarEnabled" class="favorite-bar-wrapper px-4">
      <ChannelFavoriteBar @manage="channelFavoritesVisible = true" />
    </div>

    <IFormEmbedInstances />
    <IFormPanelHost />

    <div
      class="chat overflow-y-auto h-full px-4 pt-6"
      :class="[`chat--layout-${display.layout}`, `chat--palette-${display.palette}`, { 'chat--no-avatar': !display.showAvatar, 'chat--show-drag-indicator': display.settings.showDragIndicator, 'chat--has-background': !!channelBackgroundStyle }]"
      v-show="rows.length > 0 || messageWindow.loadingLatest"
      @scroll="onScroll"
      @dragover="handleGalleryDragOver" @drop="handleGalleryDrop"
      ref="messagesListRef">
      <!-- <VirtualList itemKey="id" :list="rows" :minSize="50" ref="virtualListRef" @scroll="onScroll"
              @toBottom="reachBottom" @toTop="reachTop"> -->
      <div ref="topSentinelRef" class="message-sentinel message-sentinel--top"></div>
      <template v-for="(entry, index) in visibleRowEntries" :key="`${listRevision}-${entry.entryKey}`">
        <div
          :class="rowClass(entry.message)"
          :data-message-id="entry.message.id"
          :ref="el => registerMessageRow(el as HTMLElement | null, entry.message.id || '')"
        >
          <div :class="rowSurfaceClass(entry.message)">
            <template v-if="compactInlineGridLayout">
              <div class="message-row__grid">
                <div class="message-row__grid-handle">
                  <div
                    class="message-row__handle"
                    tabindex="-1"
                    :aria-hidden="!shouldShowHandle(entry.message)"
                    @pointerdown="onDragHandlePointerDown($event, entry.message)"
                  >
                    <span class="message-row__dot" v-for="n in 3" :key="n"></span>
                  </div>
                </div>
                <div class="message-row__grid-name">
                  <span
                    v-if="shouldShowInlineHeader(entry)"
                    class="message-row__name"
                    :style="getMessageIdentityColor(entry.message) ? { color: getMessageIdentityColor(entry.message) } : undefined"
                  >{{ getMessageDisplayName(entry.message) }}</span>
                  <span v-else class="message-row__name message-row__name--placeholder">占位</span>
                </div>
                <div class="message-row__grid-colon">
                  <span :class="['message-row__colon', { 'message-row__colon--placeholder': !shouldShowInlineHeader(entry) }]">：</span>
                </div>
                <div class="message-row__grid-content">
                  <chat-item
                    :avatar="getMessageAvatar(entry.message)"
                    :username="getMessageDisplayName(entry.message)"
                    :identity-color="getMessageIdentityColor(entry.message)"
                    :content="entry.message.content"
                    :item="entry.message"
                    :all-message-ids="allMessageIds"
                    :editing-preview="editingPreviewMap[entry.message.id]"
                    :tone="getMessageTone(entry.message)"
                    :show-avatar="false"
                    :hide-avatar="false"
                    :show-header="false"
                    :layout="display.layout"
                    :is-self="isSelfMessage(entry.message)"
                    :is-merged="entry.mergedWithPrev"
                    :world-keyword-editable="canManageWorldKeywords"
                    :body-only="true"
                    @avatar-longpress="avatarLongpress(entry.message)"
                    @edit="beginEdit(entry.message)"
                    @edit-save="saveEdit"
                    @edit-cancel="cancelEditing"
                  />
                </div>
              </div>
            </template>
            <template v-else-if="compactInlineLayout">
              <div
                class="message-row__handle"
                tabindex="-1"
                :aria-hidden="!shouldShowHandle(entry.message)"
                @pointerdown="onDragHandlePointerDown($event, entry.message)"
              >
                <span class="message-row__dot" v-for="n in 3" :key="n"></span>
              </div>
              <chat-item
                :avatar="getMessageAvatar(entry.message)"
                :username="getMessageDisplayName(entry.message)"
                :identity-color="getMessageIdentityColor(entry.message)"
                :content="entry.message.content"
                :item="entry.message"
                :all-message-ids="allMessageIds"
                :editing-preview="editingPreviewMap[entry.message.id]"
                :tone="getMessageTone(entry.message)"
                :show-avatar="false"
                :hide-avatar="false"
                :show-header="shouldShowInlineHeader(entry)"
                :layout="display.layout"
                :is-self="isSelfMessage(entry.message)"
                :is-merged="entry.mergedWithPrev"
                :world-keyword-editable="canManageWorldKeywords"
                @avatar-longpress="avatarLongpress(entry.message)"
                @edit="beginEdit(entry.message)"
                @edit-save="saveEdit"
                @edit-cancel="cancelEditing"
              />
            </template>
            <template v-else>
              <div
                class="message-row__handle"
                tabindex="-1"
                :aria-hidden="!shouldShowHandle(entry.message)"
                @pointerdown="onDragHandlePointerDown($event, entry.message)"
              >
                <span class="message-row__dot" v-for="n in 3" :key="n"></span>
              </div>
              <chat-item
                :avatar="getMessageAvatar(entry.message)"
                :username="getMessageDisplayName(entry.message)"
                :identity-color="getMessageIdentityColor(entry.message)"
                :content="entry.message.content"
                :item="entry.message"
                :all-message-ids="allMessageIds"
                :editing-preview="editingPreviewMap[entry.message.id]"
                :tone="getMessageTone(entry.message)"
                :show-avatar="display.showAvatar"
                :hide-avatar="display.showAvatar && entry.mergedWithPrev"
                :show-header="shouldShowInlineHeader(entry)"
                :layout="display.layout"
                :is-self="isSelfMessage(entry.message)"
                :is-merged="entry.mergedWithPrev"
                :world-keyword-editable="canManageWorldKeywords"
                @avatar-longpress="avatarLongpress(entry.message)"
                @edit="beginEdit(entry.message)"
                @edit-save="saveEdit"
                @edit-cancel="cancelEditing"
              />
            </template>
          </div>
        </div>
      </template>

      <div class="typing-preview-viewport" v-if="typingPreviewItems.length" ref="typingPreviewViewportRef">
        <div
          v-for="preview in typingPreviewItems"
          :key="`${preview.userId}-typing`"
          :class="typingPreviewItemClass(preview)"
          :ref="el => registerTypingPreviewRow(el as HTMLElement | null, preview)"
        >
          <div :class="typingPreviewSurfaceClass(preview)" :data-tone="preview.tone">
            <div
              v-if="shouldShowTypingHandle(preview)"
              :class="typingPreviewHandleClass(preview)"
              :aria-hidden="!canDragTypingPreview(preview)"
              tabindex="-1"
              @pointerdown="onPreviewDragHandlePointerDown($event, preview)"
            >
              <span class="message-row__dot" v-for="n in 3" :key="n"></span>
            </div>
            <template v-if="!display.showAvatar && compactInlineGridLayout">
              <div class="typing-preview-content typing-preview-content--grid">
                <div class="message-row__grid typing-preview-grid">
                  <div class="message-row__grid-handle typing-preview-grid__handle"></div>
                  <div class="message-row__grid-name">
                    <span
                      class="message-row__name"
                      :style="preview.color ? { color: preview.color } : undefined"
                    >{{ preview.displayName }}</span>
                  </div>
                  <div class="message-row__grid-colon">
                    <span class="message-row__colon">：</span>
                  </div>
                  <div class="message-row__grid-content">
                    <div
                      class="typing-preview-inline-body"
                      :class="{ 'typing-preview-inline-body--placeholder': preview.indicatorOnly }"
                      :data-tone="preview.tone"
                    >
                      <template v-if="preview.indicatorOnly">
                        <span>正在输入</span>
                      </template>
                      <template v-else>
                        <div v-html="renderPreviewContent(preview.content)" class="preview-content"></div>
                      </template>
                      <span class="typing-dots typing-dots--inline">
                        <span></span>
                        <span></span>
                        <span></span>
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </template>
            <template v-else>
              <div class="typing-preview-content">
                <div v-if="display.showAvatar" class="typing-preview-avatar">
                  <AvatarVue :border="false" :src="preview.avatar" />
                </div>
                <div class="typing-preview-main">
                  <div class="typing-preview-bubble-header">
                    <span
                      class="typing-preview-bubble-name"
                      :style="preview.color ? { color: preview.color } : undefined"
                    >{{ preview.displayName }}</span>
                    <span class="typing-dots typing-dots--header">
                      <span></span>
                      <span></span>
                      <span></span>
                    </span>
                  </div>
                  <div
                    :class="[
                      'typing-preview-bubble',
                      preview.indicatorOnly ? '' : 'typing-preview-bubble--content',
                    ]"
                    :data-tone="preview.tone || 'ic'"
                  >
                    <div
                      class="typing-preview-bubble__body"
                      :class="{ 'typing-preview-bubble__placeholder': preview.indicatorOnly }"
                      :data-tone="preview.tone || 'ic'"
                    >
                      <template v-if="preview.indicatorOnly">
                        正在输入
                      </template>
                      <template v-else>
                        <div v-html="renderPreviewContent(preview.content)" class="preview-content"></div>
                      </template>
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>
      <div
        ref="bottomSentinelRef"
        class="message-sentinel message-sentinel--bottom"
        v-show="inHistoryMode"
      ></div>

      <!-- <VirtualList itemKey="id" :list="rows" :minSize="50" ref="virtualListRef" @scroll="onScroll"
              @toBottom="reachBottom" @toTop="reachTop">
              <template #default="{ itemData }">
                <chat-item :avatar="imgAvatar" :username="itemData.member?.nick" :content="itemData.content"
                  :is-rtl="isMe(itemData)" :createdAt="itemData.createdAt" />
              </template>
            </VirtualList> -->
    </div>
    <div
      v-if="rows.length === 0 && !messageWindow.loadingLatest"
      class="flex h-full items-center text-2xl justify-center text-gray-400"
    >说点什么吧</div>

    <!-- flex-grow -->
    <div class="edit-area flex justify-between relative" :class="{ 'edit-area--wide-input': isMobileWideInput }">
      <div class="history-floating space-y-3 flex flex-col items-end">
        <!-- 跳转到第一条未读消息按钮 -->
        <n-button
          v-if="hasFirstUnread"
          class="jump-to-unread-button history-floating__button"
          size="small"
          type="info"
          @click="jumpToFirstUnread"
        >
          跳转到未读
          <span
            class="jump-to-unread-close"
            role="button"
            aria-label="关闭未读跳转"
            @click.stop="dismissFirstUnread"
          >X</span>
        </n-button>
        <div
          v-if="historyHintVisible"
          class="history-mode-hint"
          :class="{ 'history-mode-hint--mobile': isMobileUa }"
        >
          <template v-if="isMobileUa">
            <span class="history-mode-hint__label">历史</span>
          </template>
          <template v-else>
            <span class="history-mode-hint__label">{{ historyHintLabel }}</span>
          </template>
        </div>
        <n-button
          v-if="showButton"
          class="scroll-bottom-button history-floating__button"
          size="large"
          :circle="isMobileUa"
          :color="scrollButtonColor"
          :text-color="scrollButtonTextColor"
          @click="inHistoryMode ? handleBackToLatest() : toBottom"
        >
          <template #icon>
            <n-icon>
              <ArrowBarToDown />
            </n-icon>
          </template>
        </n-button>
      </div>

      <!-- 左下，快捷指令栏 -->
      <div class="channel-switch-trigger px-4 py-2" v-if="utils.isSmallPage && !isMobileWideInput">
        <n-button
          circle
          quaternary
          size="small"
          aria-label="切换频道列表"
          @click="emit('drawer-show')"
        >
          <template #icon>
            <n-icon :component="IconNumber"></n-icon>
          </template>
        </n-button>
      </div>

      <div class="reply-banner absolute rounded px-4 py-2" style="top: -4rem; left: 1rem" v-if="chat.curReplyTo">
        <div class="reply-banner__main">
          <span class="reply-banner__badge">回复中</span>
          <span class="reply-banner__target">{{ chat.curReplyTo.member?.nick }}</span>
        </div>
        <div class="reply-banner__actions">
          <span class="reply-banner__hint">Backspace</span>
          <n-button size="small" quaternary @click="chat.curReplyTo = null">取消</n-button>
        </div>
      </div>

      <div class="chat-input-wrapper flex flex-col w-full relative">
        <transition name="fade">
          <div v-if="whisperPanelVisible" class="whisper-panel" @mousedown.stop @pointerdown.stop>
            <div class="whisper-panel__title">{{ t('inputBox.whisperPanelTitle') }}</div>
            <n-input v-if="whisperPickerSource === 'manual'" ref="whisperSearchInputRef"
              v-model:value="whisperQuery" size="small" :placeholder="t('inputBox.whisperSearchPlaceholder')" clearable
              @keydown="handleWhisperKeydown" />
            <div class="whisper-panel__list" @keydown="handleWhisperKeydown">
              <div v-for="(candidate, idx) in filteredWhisperCandidates" :key="candidate.id"
                class="whisper-panel__item"
                :class="{ 'is-active': idx === whisperSelectionIndex || isWhisperTarget(candidate.raw) }"
                @mousedown.prevent @mouseenter="whisperSelectionIndex = idx"
                @click="onWhisperTargetToggle(candidate)">
                <AvatarVue :border="false" :size="32" :src="candidate.avatar" />
                <div class="whisper-panel__meta">
                  <div class="whisper-panel__name-row">
                    <div class="whisper-panel__name" :style="candidate.color ? { color: candidate.color } : undefined">{{ candidate.displayName }}</div>
                    <div v-if="candidate.identityTypes.length" class="whisper-panel__tags">
                      <span
                        v-for="type in candidate.identityTypes"
                        :key="type"
                        class="whisper-panel__tag"
                        :class="`whisper-panel__tag--${type}`"
                      >
                        {{ whisperIdentityTypeLabel(type) }}
                      </span>
                    </div>
                  </div>
                  <div v-if="candidate.secondaryName" class="whisper-panel__sub">@{{ candidate.secondaryName }}</div>
                </div>
                <n-checkbox
                  class="whisper-panel__checkbox"
                  :checked="isWhisperTarget(candidate.raw)"
                  @update:checked="() => onWhisperTargetToggle(candidate)"
                  @click.stop
                />
              </div>
              <div v-if="!filteredWhisperCandidates.length" class="whisper-panel__empty">{{ t('inputBox.whisperEmpty') }}</div>
            </div>
            <div class="whisper-panel__footer">
              <n-button size="small" @click="closeWhisperPanel">{{ t('inputBox.whisperCancel') }}</n-button>
              <n-button
                type="primary"
                size="small"
                :disabled="whisperTargets.length === 0"
                @click="confirmWhisperSelection"
              >
                {{ t('inputBox.whisperConfirm') }} ({{ whisperTargets.length }})
              </n-button>
            </div>
          </div>
        </transition>
        <div
          ref="inputContainerRef"
          class="chat-input-container flex flex-col w-full relative"
          :class="{ 'chat-input-container--spectator-hidden': spectatorInputDisabled, 'chat-input-container--resizing': isResizingInput }"
          @pointerdown="handleInputBorderPointerDown"
        >
          <div v-if="whisperTargets.length" class="whisper-pills">
            <span class="whisper-pill-prefix">{{ t('inputBox.whisperPillPrefix') }}</span>
            <n-tag
              v-for="target in whisperTargets"
              :key="target.id"
              class="whisper-pill-tag"
              type="info"
              size="small"
              closable
              :style="getWhisperTargetStyle(target)"
              @close.stop="chat.removeWhisperTarget(target)"
            >
              {{ target.nick || target.name }}
            </n-tag>
          </div>
          <div class="chat-input-area relative flex-1">
            <div
              :class="[
                'chat-input-actions',
                'input-floating-toolbar',
                'flex',
                'items-center',
                'justify-between',
                'gap-2',
                { 'flex-1': !isMobileWideInput },
              ]"
            >
              <div class="chat-input-actions__group chat-input-actions__group--leading">
                <div class="chat-input-actions__cell identity-switcher-cell">
                  <ChannelIdentitySwitcher
                    v-if="chat.curChannel"
                    @create="openIdentityCreate"
                    @manage="openIdentityManager"
                    @identity-changed="emitTypingPreview"
                    @avatar-setup="handleOpenAvatarPrompt"
                  />
                </div>
                <div class="chat-input-actions__cell">
                  <div class="emoji-trigger">
                    <n-tooltip trigger="hover">
                      <template #trigger>
                        <n-button
                          quaternary
                          circle
                          ref="emojiTriggerButtonRef"
                          @click="handleEmojiTriggerClick"
                        >
                          <template #icon>
                            <n-icon :component="Plus" size="18" />
                          </template>
                        </n-button>
                      </template>
                      添加表情
                    </n-tooltip>

                    <n-popover
                      v-model:show="emojiPopoverShow"
                      trigger="click"
                      placement="bottom-start"
                      :x="emojiPopoverXCoord"
                      :y="emojiPopoverYCoord"
                    >
                      <div class="emoji-panel" :class="{ 'emoji-panel--hide-remark': !emojiRemarkVisible }">
                        <div class="emoji-panel__header">
                          <div class="emoji-panel__header-left">
                            <div class="emoji-panel__title">{{ $t('inputBox.emojiTitle') }}</div>
                            <n-tooltip trigger="hover">
                              <template #trigger>
                                <n-button text size="small" @click="handleEmojiManageClick">
                                  <template #icon>
                                    <n-icon :component="Settings" />
                                  </template>
                                </n-button>
                              </template>
                              表情管理
                            </n-tooltip>
                          </div>
                          <div class="emoji-panel__header-right">
                            <n-tooltip trigger="hover">
                              <template #trigger>
                                <n-button
                                  text
                                  size="small"
                                  class="emoji-panel__toggle-remark"
                                  @click="toggleEmojiRemarkVisible"
                                >
                                  <span>{{ emojiRemarkVisible ? '隐藏备注' : '显示备注' }}</span>
                                  <n-icon :component="emojiRemarkVisible ? EyeOffOutline : EyeOutline" />
                                </n-button>
                              </template>
                              {{ emojiRemarkVisible ? '隐藏备注' : '显示备注' }}
                            </n-tooltip>
                            <n-tooltip trigger="hover">
                              <template #trigger>
                                <n-button text size="small" @click="emojiPopoverShow = false">
                                  <template #icon>
                                    <n-icon :component="CloseIcon" />
                                  </template>
                                </n-button>
                              </template>
                              关闭
                            </n-tooltip>
                          </div>
                        </div>

                        <div v-if="hasEmojiItems && hasMultipleTabs" class="emoji-panel__tabs">
                          <button
                            class="emoji-panel__tab"
                            :class="{ 'emoji-panel__tab--active': activeEmojiTab === null }"
                            @click="activeEmojiTab = null"
                          >
                            全部
                          </button>
                          <button
                            v-for="tab in emojiTabOptions"
                            :key="tab.id"
                            class="emoji-panel__tab"
                            :class="{ 'emoji-panel__tab--active': activeEmojiTab === tab.id }"
                            :title="tab.name"
                            @click="activeEmojiTab = tab.id"
                          >
                            <span class="emoji-panel__tab-text">{{ tab.name }}</span>
                          </button>
                        </div>

                        <div v-if="hasEmojiItems" class="emoji-panel__search">
                          <n-input
                            v-model:value="emojiSearchQuery"
                            size="small"
                            placeholder="搜索表情..."
                            clearable
                          />
                        </div>

                        <div v-if="!hasEmojiItems" class="emoji-panel__empty">
                          当前没有收藏的表情，可以在聊天窗口的图片上<b class="px-1">长按</b>或<b class="px-1">右键</b>添加
                        </div>

                        <div v-else class="emoji-panel__content">
                          <template v-if="isManagingEmoji">
                            <div v-if="filteredEmojiItems.length === 0" class="emoji-panel__empty">
                              没有匹配的表情
                            </div>
                            <template v-else>
                              <n-checkbox-group v-model:value="selectedEmojiIds">
                                <div class="emoji-grid">
                                  <div class="emoji-manage-item" v-for="(item, idx) in filteredEmojiItems" :key="item.id">
                                    <div class="emoji-manage-item__content">
                                      <n-checkbox :value="item.id">
                                        <div class="emoji-item">
                                          <img :src="getEmojiItemSrc(item)" :alt="item.remark || '表情'" />
                                          <div class="emoji-caption" :title="item.remark || `收藏${idx + 1}`">
                                            {{ item.remark || `收藏${idx + 1}` }}
                                          </div>
                                        </div>
                                      </n-checkbox>
                                      <n-button text size="tiny" @click.stop="openEmojiRemarkEditor(item)">编辑备注</n-button>
                                    </div>
                                  </div>
                                </div>
                              </n-checkbox-group>
                            </template>

                            <div class="emoji-panel__actions">
                              <n-button type="error" size="small" @click="emojiSelectedDelete" :disabled="selectedEmojiIds.length === 0">
                                删除选中
                              </n-button>
                              <n-button type="default" size="small" @click="exitEmojiManage">
                                退出管理
                              </n-button>
                            </div>
                          </template>
                          <template v-else>
                            <div v-if="filteredEmojiItems.length === 0" class="emoji-panel__empty">
                              没有匹配的表情
                            </div>
                            <div v-else class="emoji-grid">
                              <div
                                class="emoji-item"
                                v-for="(item, idx) in filteredEmojiItems"
                                :key="item.id"
                                draggable="true"
                                @dragstart="handleGalleryEmojiDragStart(item, $event)"
                                @click="sendEmoji(item)"
                              >
                                <img :src="getEmojiItemSrc(item)" :alt="item.remark || '表情'" />
                                <div class="emoji-caption" :title="item.remark || `收藏${idx + 1}`">{{ item.remark || `收藏${idx + 1}` }}</div>
                                <div class="emoji-item__actions">
                                  <n-button text size="tiny" @click.stop="openEmojiRemarkEditor(item)">备注</n-button>
                                </div>
                              </div>
                            </div>
                          </template>
                        </div>
                      </div>
                    </n-popover>
                  </div>
                </div>
                <div class="chat-input-actions__cell">
                  <GalleryButton />
                </div>
              </div>
              <div class="chat-input-actions__group chat-input-actions__group--addons">
                <div class="chat-input-actions__cell">
                  <ChatIcOocToggle
                    v-model="inputIcMode"
                  />
                </div>

               <div class="chat-input-actions__cell">
                 <n-tooltip trigger="hover">
                   <template #trigger>
                     <n-button quaternary circle class="whisper-toggle-button" :class="{ 'whisper-toggle-button--active': whisperToggleActive }"
                       @click="startWhisperSelection" :disabled="!canOpenWhisperPanel">
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
                        @click="toggleTypingPreview">
                        <n-icon
                          class="chat-input-actions__icon"
                          :component="IconBuildingBroadcastTower"
                          size="18"
                        />
                      </n-button>
                    </template>
                    {{ typingPreviewTooltip }}
                  </n-tooltip>
                </div>
                <div class="chat-input-actions__cell">
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-button quaternary circle @click="doUpload">
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
                      >
                        <span class="font-semibold">{{ inputMode === 'rich' ? 'P' : 'R' }}</span>
                      </n-button>
                    </template>
                    {{ inputMode === 'rich' ? '切换到纯文本模式' : '切换到富文本模式' }}
                  </n-tooltip>
                </div>

                <div class="chat-input-actions__cell">
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-button
                        quaternary
                        circle
                        :type="wideInputMode ? 'primary' : 'default'"
                        @click="toggleWideInputMode"
                      >
                        <template #icon>
                          <n-icon :component="ArrowsVertical" size="18" />
                        </template>
                      </n-button>
                    </template>
                    {{ wideInputTooltip }}
                  </n-tooltip>
                </div>

                <div class="chat-input-actions__cell">
                  <n-popover
                    trigger="click"
                    placement="top"
                    :show="historyPopoverVisible"
                    :show-arrow="false"
                    class="history-popover"
                    @update:show="handleHistoryPopoverShow"
                  >
                    <template #trigger>
                      <n-tooltip trigger="hover">
                        <template #trigger>
                          <n-button quaternary circle>
                            <template #icon>
                              <n-icon :component="ArrowBackUp" size="18" />
                            </template>
                          </n-button>
                        </template>
                        输入历史 / 保存当前
                      </n-tooltip>
                    </template>
                    <div class="history-panel" @click.stop>
                      <div class="history-panel__header">
                        <span class="history-panel__title">输入回溯</span>
                        <n-button
                          size="tiny"
                          tertiary
                          round
                          :disabled="!canManuallySaveHistory"
                          @click.stop="handleManualHistoryRecord"
                        >保存当前</n-button>
                      </div>
                      <div v-if="historyEntryViews.length" class="history-panel__body">
                        <button
                          v-for="entry in historyEntryViews"
                          :key="entry.id"
                          type="button"
                          class="history-entry"
                          @click="restoreHistoryEntry(entry.id)"
                        >
                          <div class="history-entry__meta">
                            <span class="history-entry__tag" :class="{ 'history-entry__tag--rich': entry.mode === 'rich' }">
                              {{ entry.mode === 'rich' ? '富文本' : '纯文本' }}
                            </span>
                            <span class="history-entry__time">{{ entry.timeLabel }}</span>
                          </div>
                          <div class="history-entry__preview" :title="entry.fullPreview">{{ entry.preview }}</div>
                        </button>
                      </div>
                      <div v-else class="history-panel__empty">
                        <p>暂无历史记录</p>
                        <p class="history-panel__hint">输入内容并点击「保存当前」即可添加</p>
                      </div>
                    </div>
                  </n-popover>
                </div>
                <div class="chat-input-actions__cell">
                  <n-popover trigger="manual" placement="top" :show="diceTrayVisible">
                    <template #trigger>
                      <n-tooltip trigger="hover">
                        <template #trigger>
                          <n-button class="chat-dice-button" quaternary circle :disabled="(!canUseBuiltInDice && !channelFeatures.botFeatureEnabled) || diceFeatureUpdating" @click="toggleDiceTray">
                            <template #icon>
                              <svg class="chat-input-actions__icon" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" focusable="false">
                                <rect width="12" height="12" x="2" y="10" rx="2" ry="2"></rect>
                                <path d="m17.92 14 3.5-3.5a2.24 2.24 0 0 0 0-3l-5-4.92a2.24 2.24 0 0 0-3 0L10 6M6 18h.01M10 14h.01M15 6h.01M18 9h.01"></path>
                              </svg>
                            </template>
                          </n-button>
                        </template>
                        掷骰
                      </n-tooltip>
                    </template>
                    <DiceTray
                      :default-dice="defaultDiceExpr"
                      :can-edit-default="canEditDefaultDice"
                      :built-in-dice-enabled="channelFeatures.builtInDiceEnabled"
                      :bot-feature-enabled="channelFeatures.botFeatureEnabled"
                      @insert="handleDiceInsert"
                      @roll="handleDiceRollNow"
                      @update-default="handleDiceDefaultUpdate"
                      @close="diceTrayVisible = false"
                    >
                      <template v-if="canManageChannelFeatures" #header-actions>
                        <template v-if="isMobileUa">
                          <n-tooltip trigger="hover">
                            <template #trigger>
                              <div class="dice-mode-status">
                                <span class="dice-mode-status__label">{{ diceModeLabel }}</span>
                                <n-button
                                  quaternary
                                  size="tiny"
                                  circle
                                  class="dice-tray-settings-trigger"
                                  :class="{ 'dice-tray-settings-trigger--active': diceSettingsVisible }"
                                  @click.stop="diceSettingsVisible = true"
                                >
                                  <n-icon :component="Settings" size="14" />
                                </n-button>
                              </div>
                            </template>
                            {{ diceModeTooltip }}
                          </n-tooltip>
                          <n-modal
                            v-model:show="diceSettingsVisible"
                            preset="card"
                            class="dice-settings-modal-mobile"
                            :bordered="false"
                            title="掷骰设置"
                          >
                            <div class="dice-settings-panel dice-settings-panel--modal">
                              <div class="dice-settings-panel__section">
                                <div class="dice-settings-panel__row">
                                  <div>
                                    <p class="dice-settings-panel__title">内置骰点</p>
                                    <p class="dice-settings-panel__desc">自动解析输入并生成骰点结果。</p>
                                  </div>
                                  <n-switch size="small" :value="channelFeatures.builtInDiceEnabled" :disabled="diceFeatureUpdating" @update:value="handleDiceFeatureToggle" />
                                </div>
                              </div>
                              <div class="dice-settings-panel__section">
                                <div class="dice-settings-panel__row">
                                  <div>
                                    <p class="dice-settings-panel__title">机器人骰点</p>
                                    <p class="dice-settings-panel__desc">交由机器人处理掷骰，避免与内置功能冲突。</p>
                                  </div>
                                  <n-switch size="small" :value="channelFeatures.botFeatureEnabled" :disabled="diceFeatureUpdating" @update:value="handleBotFeatureToggle" />
                                </div>
                                <div class="dice-settings-panel__body" v-if="channelFeatures.botFeatureEnabled">
                                  <n-select
                                    :value="channelBotSelection"
                                    class="dice-settings-panel__select"
                                    :options="botSelectOptions"
                                    :loading="botOptionsLoading || channelBotsLoading || syncingChannelBot"
                                    :disabled="syncingChannelBot || !hasBotOptions"
                                    placeholder="选择要启用的机器人"
                                    clearable
                                    @update:value="handleBotSelectionChange"
                                  />
                                  <div class="dice-settings-panel__hint" v-if="!botOptionsLoading && !hasBotOptions">
                                    暂无可用机器人，请先在后台创建令牌。
                                  </div>
                                </div>
                                <div class="dice-settings-panel__footer">
                                  <n-button text size="tiny" @click="openChannelMemberSettings">前往成员管理</n-button>
                                </div>
                              </div>
                            </div>
                          </n-modal>
                        </template>
                        <template v-else>
                          <n-popover trigger="manual" placement="bottom-end" :show="diceSettingsVisible" @clickoutside="diceSettingsVisible = false">
                            <template #trigger>
                              <n-tooltip trigger="hover">
                                <template #trigger>
                                  <div class="dice-mode-status">
                                    <span class="dice-mode-status__label">{{ diceModeLabel }}</span>
                                    <n-button
                                      quaternary
                                      size="tiny"
                                      circle
                                      class="dice-tray-settings-trigger"
                                      :class="{ 'dice-tray-settings-trigger--active': diceSettingsVisible }"
                                      @click.stop="diceSettingsVisible = !diceSettingsVisible"
                                    >
                                      <n-icon :component="Settings" size="14" />
                                    </n-button>
                                  </div>
                                </template>
                                {{ diceModeTooltip }}
                              </n-tooltip>
                            </template>
                            <div class="dice-settings-panel">
                              <div class="dice-settings-panel__section">
                                <div class="dice-settings-panel__row">
                                  <div>
                                    <p class="dice-settings-panel__title">内置骰点</p>
                                    <p class="dice-settings-panel__desc">自动解析输入并生成骰点结果。</p>
                                  </div>
                                  <n-switch size="small" :value="channelFeatures.builtInDiceEnabled" :disabled="diceFeatureUpdating" @update:value="handleDiceFeatureToggle" />
                                </div>
                              </div>
                              <div class="dice-settings-panel__section">
                                <div class="dice-settings-panel__row">
                                  <div>
                                    <p class="dice-settings-panel__title">机器人骰点</p>
                                    <p class="dice-settings-panel__desc">交由机器人处理掷骰，避免与内置功能冲突。</p>
                                  </div>
                                  <n-switch size="small" :value="channelFeatures.botFeatureEnabled" :disabled="diceFeatureUpdating" @update:value="handleBotFeatureToggle" />
                                </div>
                                <div class="dice-settings-panel__body" v-if="channelFeatures.botFeatureEnabled">
                                  <n-select
                                    :value="channelBotSelection"
                                    class="dice-settings-panel__select"
                                    :options="botSelectOptions"
                                    :loading="botOptionsLoading || channelBotsLoading || syncingChannelBot"
                                    :disabled="syncingChannelBot || !hasBotOptions"
                                    placeholder="选择要启用的机器人"
                                    clearable
                                    @update:value="handleBotSelectionChange"
                                  />
                                  <div class="dice-settings-panel__hint" v-if="!botOptionsLoading && !hasBotOptions">
                                    暂无可用机器人，请先在后台创建令牌。
                                  </div>
                                </div>
                                <div class="dice-settings-panel__footer">
                                  <n-button text size="tiny" @click="openChannelMemberSettings">前往成员管理</n-button>
                                </div>
                              </div>
                            </div>
                          </n-popover>
                        </template>
                      </template>
                    </DiceTray>
                  </n-popover>
                </div>
              </div>
            </div>
            <div class="chat-input-editor-row" :style="chatInputStyle">
              <div class="chat-input-editor-main">
                <KeywordSuggestPanel
                  :visible="keywordSuggestVisible"
                  :options="keywordSuggestOptions"
                  :active-index="keywordSuggestIndex"
                  :loading="keywordSuggestLoading"
                  @select="handleKeywordSuggestSelect"
                  @hover="handleKeywordSuggestHover"
                />
                <ChatInputSwitcher
                  ref="textInputRef"
                  v-model="textToSend"
                  v-model:mode="inputMode"
                  :placeholder="whisperMode ? whisperPlaceholderText : $t('inputBox.placeholder')"
                  :whisper-mode="whisperMode"
                  :disabled="spectatorInputDisabled"
                  :mention-options="atOptions"
                  :mention-loading="atLoading"
                  :mention-prefix="atPrefix"
                  :mention-render-label="atRenderLabel"
                  :rows="1"
                  :input-class="chatInputClassList"
                  :inline-images="inlineImagePreviewMap"
                  @mention-search="atHandleSearch"
                  @mention-select="handleMentionSelect"
                  @keydown="keyDown"
                  @blur="handleKeywordSuggestBlur"
                  @input="handleSlashInput"
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
              <div class="chat-input-actions__cell chat-input-actions__send chat-input-send-inline">
                <template v-if="isEditing">
                  <div class="edit-actions-group">
                    <n-button size="medium" @click="saveEdit"
                      :disabled="spectatorInputDisabled || chat.connectState !== 'connected'"
                      class="edit-action-btn edit-action-btn--save">
                      <template #icon>
                        <n-icon :component="Check" size="16" />
                      </template>
                    </n-button>
                    <n-button size="medium" @click="cancelEditing"
                      class="edit-action-btn edit-action-btn--cancel">
                      <template #icon>
                        <n-icon :component="X" size="16" />
                      </template>
                    </n-button>
                  </div>
                </template>
                <template v-else>
                  <n-button size="medium" @click="send"
                    :disabled="spectatorInputDisabled || chat.connectState !== 'connected'"
                    class="send-action-btn">
                    <template #icon>
                      <n-icon :component="Send" size="18" />
                    </template>
                  </n-button>
                </template>
              </div>
            </div>
        </div>
      </div>
    </div>
  </div>
  </div>

  <RightClickMenu />
  <AvatarClickMenu />
  <MultiSelectFloatingBar
    @copy="handleMultiSelectCopy"
    @archive="handleMultiSelectArchive"
    @delete="handleMultiSelectDelete"
    @copy-image="handleMultiSelectCopyImage"
    @select-all="handleMultiSelectAll"
  />
  <GalleryPanel @insert="handleGalleryInsert" />
  <CharacterCardPanel v-model:visible="characterCardPanelVisible" :channel-id="chat.curChannel?.id" />
  <ChannelImageViewerDrawer @locate-message="handleChannelImagesLocate" />
  <n-modal
    v-model:show="emojiRemarkModalVisible"
    preset="dialog"
    :show-icon="false"
    title="编辑表情备注"
    :positive-text="emojiRemarkSaving ? '保存中…' : '保存'"
    :positive-button-props="{ loading: emojiRemarkSaving }"
    negative-text="取消"
    @positive-click="submitEmojiRemark"
    @negative-click="cancelEmojiRemark"
  >
    <n-form label-width="72">
      <n-form-item label="备注">
        <n-input v-model:value="emojiRemarkInput" maxlength="64" placeholder="请输入备注" />
      </n-form-item>
    </n-form>
  </n-modal>
  <n-modal
    v-model:show="identityDialogVisible"
    preset="card"
    :title="identityDialogMode === 'create' ? '创建频道角色' : '编辑频道角色'"
    :auto-focus="false"
    class="identity-dialog"
  >
    <n-form label-width="90px" label-placement="left">
      <n-form-item label="频道昵称">
        <n-input v-model:value="identityForm.displayName" maxlength="32" show-count placeholder="请输入频道内显示的昵称" />
      </n-form-item>
      <n-form-item label="昵称颜色">
        <div class="identity-color-field">
          <n-color-picker
            v-model:value="identityForm.color"
            :modes="['hex']"
            :show-alpha="false"
            size="small"
            class="identity-color-picker"
          />
          <n-input
            v-model:value="identityForm.color"
            size="small"
            placeholder="#RRGGBB"
            class="identity-color-input"
            @blur="handleIdentityColorBlur"
            @keyup.enter="handleIdentityColorBlur"
          />
          <n-button tertiary size="small" @click="identityForm.color = ''">清除</n-button>
        </div>
      </n-form-item>
      <n-form-item label="频道头像">
        <div class="identity-avatar-field">
          <AvatarVue :size="48" :border="false" :src="identityAvatarDisplay || user.info.avatar" />
          <n-space>
            <n-button size="small" type="primary" @click="handleIdentityAvatarTrigger">上传头像</n-button>
            <n-button v-if="identityForm.avatarAttachmentId" size="small" tertiary @click="removeIdentityAvatar">移除</n-button>
          </n-space>
        </div>
      </n-form-item>
      <n-form-item label="绑定人物卡">
        <n-select
          v-model:value="identityForm.characterCardId"
          :options="characterCardSelectOptions"
          placeholder="选择要绑定的人物卡"
          clearable
        />
      </n-form-item>
      <n-form-item>
        <n-checkbox v-model:checked="identityForm.isDefault">
          设为频道默认身份
        </n-checkbox>
      </n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="closeIdentityDialog">取消</n-button>
        <n-button type="primary" :loading="identitySubmitting" @click="submitIdentityForm">保存</n-button>
      </n-space>
    </template>
  </n-modal>
  <input ref="identityAvatarInputRef" class="hidden" type="file" accept="image/*" @change="handleIdentityAvatarChange">
  <n-modal
    v-model:show="identityAvatarEditorVisible"
    preset="card"
    title="编辑头像"
    style="max-width: 450px;"
    :mask-closable="false"
  >
    <AvatarEditor
      :file="identityAvatarEditorFile"
      @save="handleIdentityAvatarEditorSave"
      @cancel="handleIdentityAvatarEditorCancel"
    />
  </n-modal>
  <n-drawer
    class="identity-manage-shell"
    v-model:show="identityManageVisible"
    placement="right"
    :width="identityDrawerWidth"
  >
    <n-drawer-content :class="['identity-manage-drawer', { 'identity-manage-drawer--night': isNightPalette }]">
      <template #header>
        <div class="identity-drawer__header">
          <div class="identity-drawer__header-main">
            <n-button v-if="isIdentityDrawerMobile" size="tiny" quaternary @click="identityManageVisible = false">
              返回
            </n-button>
            <div>
              <div class="identity-drawer__title">频道角色管理</div>
              <div class="identity-drawer__subtitle">支持导入/导出，便于跨频道迁移</div>
            </div>
          </div>
          <n-space>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  quaternary
                  circle
                  size="small"
                  @click="handleIdentityExport"
                  :disabled="identityExporting || !currentChannelIdentities.length"
                  :loading="identityExporting"
                >
                  <n-icon :component="Download" size="16" />
                </n-button>
              </template>
              导出当前频道角色
            </n-tooltip>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  quaternary
                  circle
                  size="small"
                  @click="triggerIdentityImport"
                  :disabled="identityImporting"
                  :loading="identityImporting"
                >
                  <n-icon :component="Upload" size="16" />
                </n-button>
              </template>
              导入角色配置
            </n-tooltip>
            <n-button
              text
              size="small"
              @click="icOocRoleConfigPanelVisible = true"
            >
              <template #icon>
                <n-icon :component="Settings" size="14" />
              </template>
              场内场外映射
            </n-button>
            <n-button
              text
              size="small"
              :disabled="identitySyncing"
              @click="openIdentitySyncDialog"
            >
              <template #icon>
                <n-icon :component="ArrowsVertical" size="14" />
              </template>
              同步其他频道
            </n-button>
          </n-space>
        </div>
      </template>
      <div v-if="currentChannelIdentities.length || identityFolders.length" class="identity-manager">
        <div class="identity-manager__sidebar">
          <div class="identity-folder-header">
            <div class="identity-folder-header__title">
              <n-icon :component="Folders" size="16" />
              <span>角色文件夹</span>
            </div>
            <n-button text size="tiny" @click="openFolderDialog('create')">
              <template #icon>
                <n-icon :component="FolderPlus" size="14" />
              </template>
              新建
            </n-button>
          </div>
          <n-scrollbar class="identity-folder-list">
            <div
              v-for="item in composedIdentityFolders"
              :key="item.id"
              class="identity-folder-item"
              :class="{ 'is-active': activeIdentityFolderId === item.id, 'is-disabled': item.disabled }"
              @click="handleFolderItemClick(item)"
            >
              <div class="identity-folder-item__label">
                <span>{{ item.label }}</span>
                <n-icon
                  v-if="item.folder"
                  class="identity-folder-item__favorite"
                  :component="item.isFavorite ? Star : StarOff"
                  size="14"
                  :class="{ 'is-active': item.isFavorite }"
                  @click.stop="toggleFolderFavorite(item.folder, !item.isFavorite)"
                />
              </div>
              <div class="identity-folder-item__meta" v-if="item.folder">
                <span class="identity-folder-item__count">{{ item.count }}</span>
                <n-dropdown trigger="click" :options="folderActionOptions" @select="key => handleFolderAction(item.folder!, key)">
                  <n-button quaternary text size="tiny">
                    <n-icon :component="DotsVertical" size="14" />
                  </n-button>
                </n-dropdown>
              </div>
              <div class="identity-folder-item__count" v-else>{{ item.count }}</div>
            </div>
          </n-scrollbar>
        </div>
        <div class="identity-manager__content">
          <div class="identity-manager__toolbar">
            <n-checkbox :checked="isAllIdentitySelected" :indeterminate="!!identitySelection.length && !isAllIdentitySelected" @update:checked="toggleSelectAll">
              全选
            </n-checkbox>
            <div class="identity-manager__selection">已选 {{ identitySelection.length }} 个角色</div>
            <n-select
              v-model:value="folderActionTarget"
              class="identity-manager__folder-select"
              size="small"
              multiple
              clearable
              placeholder="选择目标文件夹"
              :options="folderSelectOptions"
            />
            <n-space size="small">
              <n-button size="small" :disabled="!identitySelection.length || !folderActionTarget.length" :loading="folderAssigning" @click="handleIdentityFolderAssign('append')">添加</n-button>
              <n-button size="small" :disabled="!identitySelection.length || !folderActionTarget.length" :loading="folderAssigning" @click="handleIdentityFolderAssign('replace')">移动</n-button>
              <n-button size="small" tertiary :disabled="!identitySelection.length || !folderActionTarget.length" :loading="folderAssigning" @click="handleIdentityFolderAssign('remove')">移出</n-button>
              <n-button size="small" tertiary :disabled="!identitySelection.length" :loading="folderAssigning" @click="handleIdentityFolderClear">清除全部</n-button>
            </n-space>
          </div>
          <div v-if="filteredIdentities.length" class="identity-list identity-list--grid">
            <div
              v-for="identity in filteredIdentities"
              :key="identity.id"
              class="identity-list__item identity-list__item--selectable"
              :class="{ 'is-selected': identitySelection.includes(identity.id) }"
            >
              <n-checkbox
                class="identity-list__item-check"
                :checked="identitySelection.includes(identity.id)"
                @update:checked="val => handleIdentitySelection(identity.id, val)"
              />
              <AvatarVue
                :size="40"
                :border="false"
                :src="resolveAttachmentUrl(identity.avatarAttachmentId) || user.info.avatar"
              />
              <div class="identity-list__meta">
                <div class="identity-list__name">
                  <span v-if="identity.color" class="identity-list__color" :style="{ backgroundColor: identity.color }"></span>
                  <span :style="identity.color ? { color: identity.color } : undefined">{{ identity.displayName }}</span>
                  <n-tag size="small" type="info" v-if="identity.isDefault">默认</n-tag>
                </div>
                <div class="identity-list__hint">ID：{{ identity.id }}</div>
                <div class="identity-list__folders">
                  <n-tag size="small" v-if="!(identity.folderIds?.length)">未分组</n-tag>
                  <n-tag v-for="folderId in identity.folderIds" :key="folderId" size="small" type="info">{{ resolveFolderName(folderId) }}</n-tag>
                </div>
              </div>
              <div class="identity-list__actions">
                <n-button text size="small" @click="openIdentityEdit(identity)">编辑</n-button>
                <n-button text size="small" type="error" :disabled="currentChannelIdentities.length === 1" @click="deleteIdentity(identity)">删除</n-button>
              </div>
            </div>
          </div>
          <n-empty v-else description="该分组暂无角色">
            <template #extra>
              <n-button size="small" type="primary" @click="openIdentityCreate">创建新角色</n-button>
            </template>
          </n-empty>
        </div>
      </div>
      <n-empty v-else description="暂无频道角色">
        <template #extra>
          <n-button size="small" type="primary" @click="openIdentityCreate">创建新角色</n-button>
        </template>
      </n-empty>
      <template #footer>
        <n-button type="primary" block @click="openIdentityCreate">创建新角色</n-button>
      </template>
    </n-drawer-content>
  </n-drawer>
  <n-modal
    v-model:show="folderDialogVisible"
    preset="dialog"
    :title="folderDialogMode === 'create' ? '新建文件夹' : '重命名文件夹'"
    :mask-closable="false"
  >
    <n-form label-placement="left" label-width="0">
      <n-form-item>
        <n-input v-model:value="folderFormName" maxlength="32" show-count placeholder="请输入文件夹名称" />
      </n-form-item>
    </n-form>
    <template #action>
      <n-space justify="end">
        <n-button @click="folderDialogVisible = false">取消</n-button>
        <n-button type="primary" :loading="folderSubmitting" @click="submitFolderDialog">保存</n-button>
      </n-space>
    </template>
  </n-modal>
  <input ref="identityImportInputRef" class="hidden" type="file" accept="application/json" @change="handleIdentityImportChange">
  <n-modal
    :show="identitySyncDialogVisible"
    preset="card"
    title="同步其他频道角色"
    :style="{ width: 'min(520px, 92vw)' }"
    @update:show="identitySyncDialogVisible = $event"
  >
    <div class="space-y-3">
      <div>
        <div class="text-sm mb-2">选择要同步的频道</div>
        <n-select
          v-model:value="identitySyncSourceChannelId"
          :options="identitySyncChannelOptions"
          filterable
          clearable
          placeholder="选择频道"
        />
        <div class="text-xs text-gray-500 mt-2">
          同步会以导入方式新建角色，并同步场内/场外映射配置。
        </div>
      </div>
      <n-space justify="end">
        <n-button @click="identitySyncDialogVisible = false">取消</n-button>
        <n-button
          type="warning"
          :disabled="!identitySyncSourceChannelId || identitySyncing"
          :loading="identitySyncing"
          @click="handleIdentitySync('append')"
        >
          追加
        </n-button>
        <n-button
          type="primary"
          :disabled="!identitySyncSourceChannelId || identitySyncing"
          :loading="identitySyncing"
          @click="handleIdentitySync('overwrite')"
        >
          覆盖
        </n-button>
      </n-space>
    </div>
  </n-modal>
  <IcOocRoleConfigPanel v-model:show="icOocRoleConfigPanelVisible" />

  <!-- 新增组件 -->
  <ArchiveDrawer
    v-model:visible="archiveDrawerVisible"
    :messages="archivedMessages"
    :loading="archivedLoading"
    :page="archivedCurrentPage"
    :page-count="archivedPageCount"
    :total="archivedTotalCount"
    :search-query="archivedSearchQuery"
    @update:page="handleArchivePageChange"
    @update:search="handleArchiveSearchChange"
    @unarchive="handleUnarchiveMessages"
    @delete="handleArchiveMessages"
    @refresh="fetchArchivedMessages"
  />

  <ChatSearchPanel @jump-to-message="handleSearchJump" />

  <ExportManagerModal
    v-model:visible="exportManagerVisible"
    :channel-id="chat.curChannel?.id"
    @request-export="exportDialogVisible = true"
  />
  <ExportDialog
    v-model:visible="exportDialogVisible"
    :channel-id="chat.curChannel?.id"
    @export="handleExportMessages"
  />
  <ChatImportDialog
    v-model:visible="importDialogVisible"
    :channel-id="chat.curChannel?.id"
    :world-id="chat.currentWorldId"
    @import-started="(jobId: string) => { importJobId = jobId; importProgressVisible = true; }"
  />
  <ChatImportProgress
    v-model:visible="importProgressVisible"
    :channel-id="chat.curChannel?.id || ''"
    :job-id="importJobId"
    @complete="() => { chat.fetchMessages(chat.curChannel?.id); }"
  />
  <IFormFloatingWindows />
  <IFormDrawer />

  <DisplaySettingsModal
    v-model:visible="displaySettingsVisible"
    :settings="display.settings"
    @save="handleDisplaySettingsSave"
  />

  <ChannelFavoriteManager v-model:show="channelFavoritesVisible" />
  <WorldKeywordManager />

  <!-- 新用户引导系统 -->
  <OnboardingRoot v-if="!chat.isObserver" />

  <!-- 头像设置引导 -->
  <AvatarSetupPrompt
    v-if="!chat.isObserver"
    v-model:show="avatarPromptVisible"
    @setup="handleAvatarPromptSetup"
    @skip="handleAvatarPromptSkip"
  />

  <!-- 便签功能 -->
  <StickyNoteManager
    v-if="chat.curChannel?.id"
    :channel-id="chat.curChannel.id"
  />

  <!-- 人物卡预览窗口 -->
  <CharacterSheetManager />
</template>

<style lang="scss" scoped>
/* 频道背景层样式 */
.chat-root-container {
  position: relative;
}

.channel-background-layer {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
}

.channel-background-overlay {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}

.message-row {
  position: relative;
  padding-top: calc(var(--chat-bubble-gap, 0.85rem) / 2);
  padding-bottom: calc(var(--chat-bubble-gap, 0.85rem) / 2);
}

.chat--layout-bubble .message-row {
  padding-top: calc(var(--chat-bubble-gap, 0.85rem) * 0.8 / 2);
  padding-bottom: calc(var(--chat-bubble-gap, 0.85rem) * 0.8 / 2);
  background-color: var(--chat-stage-bg, var(--sc-bg-surface));
}

:root[data-custom-theme='true'] .chat--layout-bubble .message-row {
  background-color: transparent;
}

.chat--layout-compact .message-row {
  padding-top: calc(var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35)) / 2);
  padding-bottom: calc(var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35)) / 2);
}

.chat--layout-bubble .message-row:first-child {
  padding-top: 0;
}

.chat--layout-bubble .message-row:last-child {
  padding-bottom: 0;
}

.message-row--tone-ic,
.message-row--tone-ooc {
  margin: 0;
  border: none;
}

.selection-floating-bar {
  position: fixed;
  z-index: 2100;
  display: flex;
  gap: 4px;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid rgba(15, 23, 42, 0.12);
  box-shadow: 0 12px 34px rgba(15, 23, 42, 0.15);
  backdrop-filter: blur(8px);
  color: #111827;
}

:root[data-display-palette='night'] .selection-floating-bar {
  background: rgba(20, 24, 36, 0.95);
  border-color: rgba(255, 255, 255, 0.08);
  color: rgba(248, 250, 252, 0.95);
  box-shadow: 0 12px 34px rgba(0, 0, 0, 0.45);
}

.selection-floating-bar__button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  border-radius: 999px;
  background: transparent;
  color: inherit;
  padding: 4px 10px;
  font-size: 13px;
  cursor: pointer;
}

.selection-floating-bar__button:hover {
  background: rgba(15, 23, 42, 0.08);
}

:root[data-display-palette='night'] .selection-floating-bar__button:hover {
  background: rgba(255, 255, 255, 0.08);
}

.selection-floating-bar__button.is-disabled {
  opacity: 0.45;
  pointer-events: none;
}

.message-row__surface {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  width: 100%;
  padding-left: 0.25rem;
  position: relative;
  z-index: 0;
}

.message-row--tone-ic .message-row__surface,
.message-row--tone-ooc .message-row__surface {
  padding: 0;
  margin: 0;
  gap: 0;
  border: none;
  background: transparent;
}

.message-row__surface > * {
  position: relative;
  z-index: 1;
}

.message-row__surface--editing::before {
  content: '';
  position: absolute;
  inset: -0.15rem 0;
  border-radius: 1rem;
  background-color: var(--chat-preview-bg);
  background-image: radial-gradient(var(--chat-preview-dot) 1px, transparent 1px);
  background-size: 10px 10px;
  opacity: 0.9;
  z-index: 0;
}

.message-row__surface--tone-ic.message-row__surface--editing::before {
  background-color: var(--chat-ic-bg);
  background-image: radial-gradient(var(--chat-preview-dot-ic) 1px, transparent 1px);
}

.message-row__surface--tone-ooc.message-row__surface--editing::before {
  background-color: var(--chat-ooc-bg);
  background-image: radial-gradient(var(--chat-preview-dot-ooc) 1px, transparent 1px);
}

.chat--layout-compact .message-row__surface--editing::before {
  /* 紧凑模式：编辑态需要铺满整行（含两列网格/句柄），并沿用编辑蒙版色 */
  inset: 0;
  border-radius: 0.95rem;
  background-color: var(--chat-preview-bg);
  background-image: radial-gradient(var(--chat-preview-dot) 1px, transparent 1px);
  background-size: 10px 10px;
}

/* 气泡模式下移除编辑蒙版的网点纹理，仅保留纯色背景 */
.chat--layout-bubble .message-row__surface--editing::before {
  background-image: none;
  background-color: transparent;
}

.chat--layout-bubble .message-row__surface--tone-ic.message-row__surface--editing::before {
  background-color: transparent;
  background-image: none;
}

.chat--layout-bubble .message-row__surface--tone-ooc.message-row__surface--editing::before {
  background-color: transparent;
  background-image: none;
}

/* 紧凑模式下按 tone 细分颜色/网点，保持与本人编辑一致 */
.chat--layout-compact .message-row__surface--tone-ic.message-row__surface--editing::before {
  background-color: var(--chat-ic-bg);
  background-image: radial-gradient(var(--chat-preview-dot-ic) 1px, transparent 1px);
  background-size: 10px 10px;
}

.chat--layout-compact .message-row__surface--tone-ooc.message-row__surface--editing::before {
  background-color: var(--chat-ooc-bg);
  background-image: radial-gradient(var(--chat-preview-dot-ooc) 1px, transparent 1px);
  background-size: 10px 10px;
}

/* 夜间紧凑模式编辑场外消息需保持纯黑底，避免灰色噪点 */
.chat--layout-compact.chat--palette-night .message-row__surface--tone-ooc.message-row__surface--editing::before {
  background-color: #2D2D31;
  background-image: radial-gradient(var(--chat-preview-dot-ooc) 1px, transparent 1px);
  background-size: 10px 10px;
}

.cloud-upload-result {
  line-height: 1.6;
}

.cloud-upload-result a {
  color: var(--primary-color);
  word-break: break-all;
}

.chat {
  background-color: var(--chat-stage-bg, var(--sc-bg-surface));
  border: 1px solid var(--sc-border-strong);
  border-radius: 1rem;
  box-shadow: 0 18px 36px rgba(15, 23, 42, 0.08);
  transition: background-color 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
  scrollbar-color: var(--sc-scrollbar-thumb, var(--sc-border-mute)) transparent;
  font-size: var(--chat-font-size, 0.95rem);
  line-height: var(--chat-line-height, 1.6);
  letter-spacing: var(--chat-letter-spacing, 0px);
}

.favorite-bar-wrapper {
  margin-top: 0.75rem;
  margin-bottom: 0.5rem;
}

.chat.chat--palette-night {
  border: none;
  border-radius: 0;
  box-shadow: 0 22px 42px rgba(0, 0, 0, 0.6);
}

.chat::-webkit-scrollbar {
  width: var(--sc-scrollbar-size, 6px);
}

.chat::-webkit-scrollbar-track {
  background: transparent;
}

.chat::-webkit-scrollbar-thumb {
  background-color: var(--sc-scrollbar-thumb, var(--sc-border-mute));
  border-radius: 999px;
}

:global(.chat::-webkit-scrollbar-thumb:hover) {
  background-color: var(--sc-scrollbar-thumb-hover, var(--sc-border-strong));
}

:global(.chat.chat--palette-night) {
  scrollbar-color: var(--sc-scrollbar-thumb, rgba(159, 159, 159, 0.35)) transparent;
}

:global(.chat.chat--palette-night::-webkit-scrollbar-thumb) {
  background-color: var(--sc-scrollbar-thumb, rgba(159, 159, 159, 0.35));
}

.chat--palette-day {
  --chat-ic-bg: #FBFDF7;
  --chat-ooc-bg: #FFFFFF;
  --chat-preview-dot-ic: rgba(120, 130, 120, 0.35);
  --chat-preview-dot-ooc: rgba(148, 163, 184, 0.35);
}

.chat--palette-night {
  --chat-ic-bg: #3F3F46;
  --chat-ooc-bg: #2D2D31;
  --chat-preview-dot-ic: rgba(255, 255, 255, 0.25);
  --chat-preview-dot-ooc: rgba(255, 255, 255, 0.35);
}

/* Custom theme override - when custom theme is active, use CSS variables from :root */
:root[data-custom-theme='true'] .chat--palette-day,
:root[data-custom-theme='true'] .chat--palette-night {
  --chat-ic-bg: var(--custom-chat-ic-bg, var(--chat-ic-bg));
  --chat-ooc-bg: var(--custom-chat-ooc-bg, var(--chat-ooc-bg));
  --chat-stage-bg: var(--custom-chat-stage-bg, var(--chat-stage-bg));
  --chat-preview-bg: var(--custom-chat-preview-bg, var(--chat-preview-bg));
  --chat-preview-dot: var(--custom-chat-preview-dot, var(--chat-preview-dot));
}

.chat--layout-compact {
  --chat-compact-ic-bg: var(--chat-ic-bg);
  --chat-compact-ooc-bg: var(--chat-ooc-bg);
  --chat-compact-archived-bg: rgba(148, 163, 184, 0.2);
  background-color: var(--chat-compact-ic-bg);
  transition: background-color 0.25s ease;
}

.chat--layout-compact.chat--has-background {
  --chat-compact-ic-bg: transparent;
  --chat-compact-ooc-bg: transparent;
  --chat-compact-archived-bg: transparent;
  background-color: transparent;
}

.chat.chat--layout-compact.chat--no-avatar .message-row__surface {
  padding: 0.1rem 0.35rem;
}

.chat.chat--layout-compact {
  overflow-x: hidden;
}

.chat--layout-compact .message-row {
  width: 100%;
  padding-left: 0;
  padding-right: 0;
}

.chat--layout-compact .message-row--tone-ic {
  background-color: var(--chat-compact-ic-bg);
}

.chat--layout-compact .message-row--tone-ooc {
  background-color: var(--chat-compact-ooc-bg);
}

.chat--layout-compact .message-row--tone-archived {
  background-color: var(--chat-compact-archived-bg);
}

.chat--layout-compact .message-row__surface {
  padding: 0.1rem 0.35rem;
  border-radius: 0;
  background: transparent;
}

.chat--layout-compact .message-row--tone-ic .message-row__surface,
.chat--layout-compact .message-row--tone-ooc .message-row__surface {
  padding: 0;
  gap: 0;
  border: none;
}

.chat--layout-compact .message-row__surface--tone-ic {
  background-color: var(--chat-compact-ic-bg);
}

.chat--layout-compact .message-row__surface--tone-ooc {
  background-color: var(--chat-compact-ooc-bg);
}

.chat--layout-compact .message-row__surface--tone-archived {
  background-color: var(--chat-compact-archived-bg);
}


.chat--layout-compact .message-row__handle {
  margin-top: 0.1rem;
  width: 1rem;
}

.chat--layout-compact .typing-preview-viewport {
  padding: 0;
  gap: 0;
  background-color: transparent;
}

.chat--layout-compact .typing-preview-item {
  margin-top: 0;
}

.chat--layout-compact .typing-preview-surface {
  width: 100%;
  padding: 0;
  border-radius: 0;
  border: none;
  --typing-preview-bg: var(--chat-compact-ic-bg);
  background-color: var(--typing-preview-bg);
  background-image: none;
}

.chat--layout-compact .typing-preview-surface[data-tone='ooc'],
.chat--layout-compact .typing-preview-item--ooc .typing-preview-surface {
  --typing-preview-bg: var(--chat-compact-ooc-bg);
}

.chat--layout-compact .typing-preview-surface[data-tone='ic'],
.chat--layout-compact .typing-preview-item--ic .typing-preview-surface {
  --typing-preview-bg: var(--chat-compact-ic-bg);
}

:root[data-custom-theme='true'] .chat--layout-compact .typing-preview-surface::before {
  content: none;
  display: none;
}

:root[data-custom-theme='true'] .chat--layout-compact .typing-preview-surface {
  --typing-preview-bg: var(--custom-chat-preview-bg, var(--chat-ic-bg, #f6f7fb));
  --typing-preview-dot: var(--custom-chat-preview-dot, var(--chat-preview-dot, rgba(148, 163, 184, 0.35)));
  background-color: var(--typing-preview-bg);
  background-image: radial-gradient(var(--typing-preview-dot) 1px, transparent 1px);
  background-size: 10px 10px;
}

:root[data-custom-theme='true'] .chat--layout-compact .typing-preview-surface[data-tone='ooc'],
:root[data-custom-theme='true'] .chat--layout-compact .typing-preview-item--ooc .typing-preview-surface {
  --typing-preview-bg: var(--custom-chat-preview-bg, var(--chat-ooc-bg, #ffffff));
  --typing-preview-dot: var(--custom-chat-preview-dot, var(--chat-preview-dot-ooc));
}

:root[data-custom-theme='true'] .chat--layout-compact .typing-preview-surface[data-tone='ic'],
:root[data-custom-theme='true'] .chat--layout-compact .typing-preview-item--ic .typing-preview-surface {
  --typing-preview-bg: var(--custom-chat-preview-bg, var(--chat-ic-bg, #fbfdf7));
  --typing-preview-dot: var(--custom-chat-preview-dot, var(--chat-preview-dot-ic));
}

.identity-drawer__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding-right: 0.25rem;
}

.identity-drawer__header-main {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.identity-drawer__title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--sc-text-primary, #111827);
}

.identity-drawer__subtitle {
  margin-top: 0.15rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #6b7280);
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
  margin-top: 0;
  align-self: center;
  height: 100%;
  pointer-events: none;
  touch-action: none;
}

.typing-preview-handle {
  opacity: 1 !important;
  pointer-events: auto;
  touch-action: none;
}

.typing-preview-handle--dragging,
.typing-preview-handle:active {
  cursor: grabbing;
}

.message-row.draggable-item .message-row__handle {
  pointer-events: auto;
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

.chat--layout-compact .message-row__dot {
  margin: 0.08rem 0;
}

.chat--layout-compact.chat--no-avatar {
  --inline-handle-width: 1.5rem;
  --inline-grid-gap: 0.2rem;
  --inline-colon-anchor: 25%;
  --inline-colon-width: 1.2ch;
  --inline-name-max: 40ch;
}

.chat--layout-compact.chat--no-avatar .message-row__grid {
  display: grid;
  grid-template-columns:
    var(--inline-handle-width)
    minmax(
      0,
      clamp(
        0px,
        calc(
          var(--inline-colon-anchor) - var(--inline-handle-width) - (var(--inline-grid-gap) * 2)
        ),
        var(--inline-name-max)
      )
    )
    var(--inline-colon-width)
    minmax(0, 1fr);
  align-items: flex-start;
  column-gap: var(--inline-grid-gap);
  width: 100%;
}

.chat--layout-compact.chat--no-avatar .message-row__grid-handle {
  display: flex;
  justify-content: center;
  width: var(--inline-handle-width);
  min-width: var(--inline-handle-width);
}

.chat--layout-compact.chat--no-avatar .message-row__grid-name {
  font-weight: 600;
  color: var(--chat-text-primary, #1f2937);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  text-align: right;
  display: flex;
  justify-content: flex-end;
}

.chat--layout-compact.chat--no-avatar .message-row__name {
  font-weight: 600;
  color: var(--chat-text-primary, #1f2937);
  white-space: nowrap;
}

.chat--layout-compact.chat--no-avatar .message-row__name--placeholder {
  visibility: hidden;
  pointer-events: none;
  display: inline-block;
  min-width: 2ch;
}

.chat--layout-compact.chat--no-avatar .message-row__grid-colon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--chat-text-primary, #1f2937);
}

.chat--layout-compact.chat--no-avatar .message-row__colon--placeholder {
  visibility: hidden;
}

.chat--layout-compact.chat--no-avatar .message-row__grid-content {
  min-width: 0;
}

.chat--layout-compact.chat--no-avatar .message-row__grid-content :deep(.chat-item) {
  padding: 0;
  padding-bottom: var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35));
}

.chat--layout-compact.chat--no-avatar .message-row__grid-content :deep(.chat-item.chat-item--merged.chat-item--ic),
.chat--layout-compact.chat--no-avatar .message-row__grid-content :deep(.chat-item.chat-item--merged.chat-item--ooc) {
  padding-bottom: calc(
    var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35)) * 0.43
  );
}

.message-row__ghost {
  /* Ghost element disabled - using floating ghost instead */
  display: none;
}

/* Drag source collapses completely - siblings fill the gap */
.message-row--drag-source {
  opacity: 0 !important;
  pointer-events: none;
  max-height: 0 !important;
  overflow: hidden;
  margin-top: 0 !important;
  margin-bottom: 0 !important;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
  transition: max-height 0.2s cubic-bezier(0.33, 1, 0.68, 1),
              margin 0.2s cubic-bezier(0.33, 1, 0.68, 1),
              padding 0.2s cubic-bezier(0.33, 1, 0.68, 1),
              opacity 0.15s ease-out;
}

/* Slot-opening animation - messages slide to create space */
.message-row {
  position: relative;
  contain: layout style;
  transition: transform 0.18s cubic-bezier(0.33, 1, 0.68, 1);
}

/* When hovering over a drop target, shift it and all following rows down */
.message-row--drop-before:not(.message-row--drag-source),
.message-row--drop-before:not(.message-row--drag-source) ~ .message-row:not(.message-row--drag-source) {
  transform: translateY(3rem);
}

/* When dropping after, only shift rows AFTER the target */
.message-row--drop-after:not(.message-row--drag-source) ~ .message-row:not(.message-row--drag-source) {
  transform: translateY(3rem);
}

/* Fill the slot gap with tone-matched background color */
.message-row--tone-ic {
  --message-drop-gap-bg: var(--chat-ic-bg);
}

.message-row--tone-ooc {
  --message-drop-gap-bg: var(--chat-ooc-bg);
}

.message-row--tone-archived {
  --message-drop-gap-bg: rgba(148, 163, 184, 0.2);
}

.message-row--drop-before:not(.message-row--drag-source)::before,
.message-row--drop-after:not(.message-row--drag-source)::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  height: 3rem;
  background-color: var(--message-drop-gap-bg, var(--chat-ic-bg));
  z-index: -1;
}

.message-row--drop-before:not(.message-row--drag-source)::before {
  bottom: 100%;
}

.message-row--drop-after:not(.message-row--drag-source)::after {
  top: 100%;
}

/* Indicator line at the edge of slot - only shown when setting enabled */
.chat--show-drag-indicator .message-row--drop-before:not(.message-row--drag-source)::before {
  border-top: 3px solid var(--sc-primary, #3b82f6);
}

.chat--show-drag-indicator .message-row--drop-after:not(.message-row--drag-source)::after {
  border-bottom: 3px solid var(--sc-primary, #3b82f6);
}

/* Drag handle should prevent scroll interference */
.message-row__handle {
  touch-action: none;
  cursor: grab;
}

.message-row__handle:active {
  cursor: grabbing;
}

/* Subtle hover highlight for message positioning - compact mode only */
.message-row .message-row__surface {
  position: relative;
  transition: background-color 0.15s ease;
}

.chat--layout-compact .message-row:not(.message-row--search-hit):not(.message-row--drag-source):hover .message-row__surface::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: rgba(128, 128, 128, 0.07);
  pointer-events: none;
  z-index: 0;
  transition: opacity 0.15s ease;
}

/* Dragged message highlight during live reorder */
/* Combined with transition rules above for instant response */

.message-row--drag-source .message-row__surface {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  border-radius: 0.9rem;
}

/* Transparent overlay for dragged message */
.message-row--drag-source .message-row__surface::before {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--sc-bg-base, rgba(255, 255, 255, 0.15));
  border-radius: inherit;
  z-index: 1;
  pointer-events: none;
}

.message-row--search-hit .message-row__surface::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 0.9rem;
  z-index: 0;
  background: rgba(14, 165, 233, 0.18);
  box-shadow: 0 0 0 1px rgba(14, 165, 233, 0.25);
  animation: search-hit-pulse 2s ease forwards;
}

@keyframes search-hit-pulse {
  0% {
    opacity: 0.9;
  }

  50% {
    opacity: 0.4;
  }

  100% {
    opacity: 0;
  }
}

@media (hover: none) {
  .message-row.draggable-item .message-row__handle {
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

.chat--layout-compact.chat {
  padding-left: 0;
  padding-right: 0;
  padding-bottom: 0;
}

.chat--layout-compact.chat>.virtual-list__client {
  @apply px-0 pt-2;
}

.chat--layout-compact .chat-item {
  padding-bottom: var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35));
}

.chat--layout-compact .chat-item--merged.chat-item--ic,
.chat--layout-compact .chat-item--merged.chat-item--ooc {
  padding-bottom: calc(
    var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35)) * 0.43
  );
}

.channel-switch-trigger {
  position: fixed;
  top: 5.5rem;
  left: 0.5rem;
  z-index: 40;
  pointer-events: auto;
  background-color: var(--sc-chip-bg);
  border: 1px solid var(--sc-border-mute);
  border-radius: 999px;
}

.channel-switch-trigger .n-button {
  color: var(--sc-text-primary);
}

@media (min-width: 1024px) {
  .channel-switch-trigger {
    display: none;
  }
}


.typing-preview-item {
  margin-top: 0.75rem;
  font-size: 0.9375rem;
  color: var(--chat-text-secondary);
}

.typing-preview-item--dragging {
  position: relative;
  z-index: 10;
}

.typing-preview-item--dragging .typing-preview-surface {
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.15);
  border-radius: var(--chat-message-radius, 0.85rem);
}

.typing-preview-surface {
  display: flex;
  align-items: flex-start;
  gap: 0;
  width: 100%;
  padding: 0;
  border: none;
}

.chat--layout-bubble .typing-preview-surface {
  gap: 0.5rem;
  padding: 0.3rem 0;
}

.typing-preview-content {
  flex: 1;
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  min-width: 0;
}

.typing-preview-content--grid {
  gap: 0;
}

.typing-preview-main {
  flex: 1;
  min-width: 0;
}

.typing-preview-avatar {
  flex-shrink: 0;
  width: var(--chat-avatar-size, 3rem);
  height: var(--chat-avatar-size, 3rem);
  min-width: var(--chat-avatar-size, 3rem);
}

.message-row__handle--placeholder {
  opacity: 0 !important;
  pointer-events: none;
  cursor: default;
}

.typing-preview-viewport {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 0;
  width: 100%;
  align-self: stretch;
  max-height: none;
  overflow: visible;
}

.typing-preview-bubble {
  flex: 1;
  width: 100%;
  max-width: none;
  align-self: stretch;
  padding: 0 0.6rem;
  border-radius: 0;
  border: 1px solid transparent;
  display: flex;
  flex-direction: column;
  gap: 0;
  background-color: transparent;
  color: var(--chat-text-primary, #1f2937);
  box-shadow: none;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

.typing-preview-surface {
  position: relative;
  z-index: 0;
}

.typing-preview-surface > * {
  position: relative;
  z-index: 1;
}

.chat--layout-compact .typing-preview-surface::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 0.95rem;
  background-color: var(--chat-preview-bg, var(--chat-ic-bg, #f6f7fb));
  background-image: radial-gradient(
    var(--typing-preview-dot, var(--chat-preview-dot, rgba(148, 163, 184, 0.35))) 1px,
    transparent 1px
  );
  background-size: 10px 10px;
  opacity: 0.9;
  z-index: 0;
}

.chat--layout-compact .typing-preview-surface[data-tone='ic']::before {
  background-color: var(--chat-ic-bg, #fbfdf7);
  --typing-preview-dot: var(--chat-preview-dot-ic, var(--chat-preview-dot, rgba(148, 163, 184, 0.35)));
}

.chat--layout-compact .typing-preview-surface[data-tone='ooc']::before {
  background-color: var(--chat-ooc-bg, #ffffff);
  --typing-preview-dot: var(--chat-preview-dot-ooc, var(--chat-preview-dot, rgba(148, 163, 184, 0.25)));
}

.chat--layout-compact.chat--palette-day:not(.chat--no-avatar) .typing-preview-surface,
.chat--layout-compact.chat--palette-day:not(.chat--no-avatar) .typing-preview-bubble,
.chat--layout-compact.chat--palette-day:not(.chat--no-avatar) .typing-preview-bubble__body {
  border-color: transparent !important;
  box-shadow: none;
}

.chat--layout-bubble .typing-preview-bubble {
  padding: 0.5rem 0.75rem;
  border-radius: var(--chat-message-radius, 0.85rem);
  background-color: var(--chat-ic-bg, #f5f5f5);
  border-color: transparent;
  background-image: none;
}

.typing-preview-bubble[data-tone='ic'] {
  background-color: var(--chat-ic-bg, #f5f5f5);
  border-color: rgba(15, 23, 42, 0.14);
  --typing-preview-dot: var(--chat-preview-dot-ic, var(--chat-preview-dot, rgba(148, 163, 184, 0.35)));
}

.typing-preview-bubble[data-tone='ooc'] {
  background-color: var(--chat-ooc-bg, #ffffff);
  border-color: rgba(15, 23, 42, 0.12);
  --typing-preview-dot: var(--chat-preview-dot-ooc, var(--chat-preview-dot, rgba(148, 163, 184, 0.25)));
}

:root[data-display-palette='night'] .typing-preview-bubble[data-tone='ic'] {
  background-color: var(--chat-ic-bg, #3f3f45);
  border-color: rgba(255, 255, 255, 0.16);
  color: var(--chat-text-primary, #f4f4f5);
}

:root[data-display-palette='night'] .typing-preview-bubble[data-tone='ooc'] {
  background-color: var(--chat-ooc-bg, #2D2D31);
  border-color: rgba(255, 255, 255, 0.24);
  color: var(--chat-text-primary, #f5f3ff);
}

.chat--layout-compact .typing-preview-bubble {
  background-color: transparent !important;
  border-color: transparent !important;
  box-shadow: none;
}

.chat--layout-compact .typing-preview-bubble.typing-preview-bubble--content {
  padding: 0;
  margin: 0;
}

.chat--layout-compact
  .typing-preview-bubble.typing-preview-bubble--content
  .typing-preview-bubble__body {
  padding: 0;
  margin: 0;
}

.typing-preview-bubble--content {
  color: inherit;
}

.typing-preview-grid__handle {
  min-height: 0;
  display: flex;
  align-items: center;
}

.typing-preview-inline-body {
  display: inline-flex;
  align-items: center;
  align-self: start;
  gap: 0.4rem;
  line-height: 1.5;
  font-size: 0.9375rem;
  color: var(--chat-text-primary);
  min-width: 0;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.typing-preview-inline-body .preview-content {
  flex: 1 1 auto;
  min-width: 0;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.typing-preview-inline-body--placeholder {
  color: #6b7280;
}

.typing-preview-bubble-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.1rem;
}

.typing-preview-bubble-name {
  font-weight: 600;
  font-size: 0.9rem;
  color: var(--chat-text-primary, #1f2937);
}

.typing-preview-bubble__body {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: var(--chat-line-height, 1.6);
  font-size: var(--chat-font-size, 0.95rem);
  letter-spacing: var(--chat-letter-spacing, 0px);

  /* 段落样式 */
  p {
    margin: 0;
    line-height: 1.5;
  }

  p + p {
    margin-top: 0.5rem;
  }

  /* 标题样式 */
  h1, h2, h3 {
    margin: 0.5rem 0 0.25rem;
    font-weight: 600;
    line-height: 1.3;
  }

  h1 {
    font-size: 1.25rem;
  }

  h2 {
    font-size: 1.1rem;
  }

  h3 {
    font-size: 1rem;
  }

  /* 列表样式 */
  ul, ol {
    padding-left: 1.5rem;
    margin: 0.25rem 0;
    list-style-position: inside;
  }

  ul {
    list-style-type: disc !important;
  }

  ol {
    list-style-type: decimal !important;
  }

  li {
    margin: 0.125rem 0;
    display: list-item !important;
  }

  /* 引用样式 */
  blockquote {
    border-left: 3px solid #3b82f6;
    padding-left: 0.75rem;
    margin: 0.25rem 0;
    color: #6b7280;
  }

  /* 代码样式 */
  code {
    background-color: rgba(0, 0, 0, 0.05);
    border-radius: 0.25rem;
    padding: 0.125rem 0.375rem;
    font-family: 'Courier New', monospace;
    font-size: 0.9em;
  }

  pre {
    background-color: #1f2937;
    color: #f9fafb;
    border-radius: 0.375rem;
    padding: 0.5rem 0.75rem;
    margin: 0.25rem 0;
    overflow-x: auto;
    font-size: 0.85em;

    code {
      background-color: transparent;
      color: inherit;
      padding: 0;
    }
  }

  /* 高亮样式 */
  mark {
    background-color: #fef08a;
    padding: 0.1rem 0.2rem;
    border-radius: 0.125rem;
  }

  /* 分割线 */
  hr {
    border: none;
    border-top: 1px solid #e5e7eb;
    margin: 0.5rem 0;
  }

  /* 链接样式 */
  a {
    color: #3b82f6;
    text-decoration: underline;
  }

  /* 文本样式 */
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

  /* 图片样式 */
  img {
    max-width: min(36vw, 200px);
    max-height: 12rem;
    height: auto;
    border-radius: 0.5rem;
    display: inline-block;
    object-fit: contain;
  }
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

  /* 标题样式 */
  h1, h2, h3 {
    margin: 0.5rem 0 0.25rem;
    font-weight: 600;
    line-height: 1.3;
  }

  h1 {
    font-size: 1.25rem;
  }

  h2 {
    font-size: 1.1rem;
  }

  h3 {
    font-size: 1rem;
  }

  /* 列表样式 */
  ul, ol {
    padding-left: 1.5rem;
    margin: 0.25rem 0;
    list-style-position: inside;
  }

  ul {
    list-style-type: disc !important;
  }

  ol {
    list-style-type: decimal !important;
  }

  li {
    margin: 0.125rem 0;
    display: list-item !important;
  }

  /* 引用样式 */
  blockquote {
    border-left: 3px solid #3b82f6;
    padding-left: 0.75rem;
    margin: 0.25rem 0;
    color: #6b7280;
  }

  /* 代码块样式 */
  pre {
    background-color: #1f2937;
    color: #f9fafb;
    border-radius: 0.375rem;
    padding: 0.5rem 0.75rem;
    margin: 0.25rem 0;
    overflow-x: auto;
    font-size: 0.85em;

    code {
      background-color: transparent;
      color: inherit;
      padding: 0;
    }
  }

  /* 高亮样式 */
  mark {
    background-color: #fef08a;
    padding: 0.1rem 0.2rem;
    border-radius: 0.125rem;
  }

  /* 分割线 */
  hr {
    border: none;
    border-top: 1px solid #e5e7eb;
    margin: 0.5rem 0;
  }

  /* 链接样式 */
  a {
    color: #3b82f6;
    text-decoration: underline;
  }

  :deep(img) {
    max-width: min(36vw, 200px);
    height: auto;
    border-radius: 0.5rem;
    display: inline-block;
  }

  :deep(.preview-inline-image) {
    max-width: min(36vw, 200px);
    max-height: 12rem;
    width: auto;
    height: auto;
    border-radius: 0.5rem;
    display: inline-block;
    object-fit: contain;
  }

  :deep(.inline-image) {
    max-height: 6rem;
    width: auto;
    border-radius: 0.375rem;
    vertical-align: middle;
    margin: 0.25rem;
    object-fit: contain;
  }

  :deep(.rich-inline-image) {
    max-width: 100%;
    max-height: 12rem;
    height: auto;
    border-radius: 0.5rem;
    margin: 0.5rem 0.25rem;
    display: inline-block;
    object-fit: contain;
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

.typing-dots--inline {
  margin-left: 0.25rem;
}

.typing-dots--bubble {
  align-self: flex-end;
  margin-top: 0.15rem;
}

.typing-dots--header {
  margin-left: auto;
  gap: 0.2rem;
}

.typing-dots--header span {
  width: 0.25rem;
  height: 0.25rem;
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
  transition: color 0.2s ease, background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
  border: 1px solid transparent;
}

/* Indicator mode (default gray state) */
.typing-toggle--indicator {
  color: var(--sc-text-secondary, #9ca3af);
  background-color: transparent;
}

.typing-toggle--indicator:hover {
  color: var(--sc-text-primary, #6b7280);
  background-color: rgba(156, 163, 175, 0.12);
}

:root[data-display-palette='night'] .typing-toggle--indicator {
  color: rgba(156, 163, 175, 0.75);
}

:root[data-display-palette='night'] .typing-toggle--indicator:hover {
  color: rgba(209, 213, 219, 0.95);
  background-color: rgba(156, 163, 175, 0.18);
}

/* Content mode (active blue state) */
.typing-toggle--content {
  color: #2563eb;
  background-color: rgba(37, 99, 235, 0.12);
  border-color: rgba(37, 99, 235, 0.35);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.1);
}

.typing-toggle--content:hover {
  color: #1d4ed8;
  background-color: rgba(37, 99, 235, 0.18);
  border-color: rgba(37, 99, 235, 0.5);
}

:root[data-display-palette='night'] .typing-toggle--content {
  color: rgba(147, 197, 253, 0.95);
  background-color: rgba(59, 130, 246, 0.22);
  border-color: rgba(147, 197, 253, 0.4);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
}

:root[data-display-palette='night'] .typing-toggle--content:hover {
  color: #93c5fd;
  background-color: rgba(59, 130, 246, 0.3);
  border-color: rgba(147, 197, 253, 0.55);
}

/* Silent mode (amber/warning state) */
.typing-toggle--silent {
  color: #d97706;
  background-color: rgba(245, 158, 11, 0.12);
  border-color: rgba(245, 158, 11, 0.35);
  box-shadow: 0 0 0 2px rgba(245, 158, 11, 0.1);
}

.typing-toggle--silent:hover {
  color: #b45309;
  background-color: rgba(245, 158, 11, 0.18);
  border-color: rgba(245, 158, 11, 0.5);
}

:root[data-display-palette='night'] .typing-toggle--silent {
  color: rgba(252, 211, 77, 0.95);
  background-color: rgba(245, 158, 11, 0.22);
  border-color: rgba(252, 211, 77, 0.4);
  box-shadow: 0 0 0 2px rgba(245, 158, 11, 0.15);
}

:root[data-display-palette='night'] .typing-toggle--silent:hover {
  color: #fcd34d;
  background-color: rgba(245, 158, 11, 0.3);
  border-color: rgba(252, 211, 77, 0.55);
}

/* Custom theme overrides */
:root[data-custom-theme='true'] .typing-toggle--indicator {
  color: var(--sc-text-secondary) !important;
}

:root[data-custom-theme='true'] .typing-toggle--indicator:hover {
  color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-hover, rgba(156, 163, 175, 0.12)) !important;
}

:root[data-custom-theme='true'] .typing-toggle--content {
  color: var(--sc-primary-color, #2563eb) !important;
  background-color: rgba(var(--sc-primary-rgb, 37, 99, 235), 0.15) !important;
  border-color: rgba(var(--sc-primary-rgb, 37, 99, 235), 0.4) !important;
}

:root[data-custom-theme='true'] .typing-toggle--content:hover {
  background-color: rgba(var(--sc-primary-rgb, 37, 99, 235), 0.22) !important;
  border-color: rgba(var(--sc-primary-rgb, 37, 99, 235), 0.55) !important;
}

:root[data-custom-theme='true'] .typing-toggle--silent {
  color: var(--sc-warning-color, #d97706) !important;
  background-color: rgba(245, 158, 11, 0.15) !important;
  border-color: rgba(245, 158, 11, 0.4) !important;
}

:root[data-custom-theme='true'] .typing-toggle--silent:hover {
  background-color: rgba(245, 158, 11, 0.22) !important;
  border-color: rgba(245, 158, 11, 0.55) !important;
}

.edit-area {
  width: 100%;
  background-color: var(--sc-bg-surface);
  border-top: 1px solid var(--sc-border-mute);
  border-bottom: 1px solid var(--sc-border-mute);
  border-radius: 0;
  padding: 0;
  gap: 0;
  transition: background-color 0.25s ease, border-color 0.25s ease;
}

@media (max-width: 768px), (pointer: coarse) {
  .edit-area.edit-area--wide-input {
    position: fixed;
    inset: 0;
    width: 100%;
    height: var(--wide-input-height, 100vh);
    z-index: 60;
    flex-direction: column;
    justify-content: flex-start;
    align-items: stretch;
    overflow: hidden;
    padding-bottom: env(safe-area-inset-bottom);
    touch-action: none;
  }

  .edit-area.edit-area--wide-input .chat-input-container {
    flex: 1 1 auto;
    justify-content: flex-start;
    overflow: hidden;
    min-height: 0;
  }

  .edit-area.edit-area--wide-input .chat-input-area {
    flex: 1 1 auto;
    margin: 0;
    overflow: hidden;
    min-height: 0;
  }

  .edit-area.edit-area--wide-input .chat-input-actions {
    flex: 0 0 auto;
  }

  .edit-area.edit-area--wide-input .chat-input-editor-row {
    flex: 1 1 auto;
    margin-top: 0;
    align-items: stretch;
    min-height: 0;
    height: 100%;
  }

  .edit-area.edit-area--wide-input .chat-input-editor-main {
    flex: 1 1 auto;
    align-self: stretch;
    min-height: 0;
  }
}

.reply-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  background-color: var(--sc-bg-layer-strong, rgba(248, 250, 252, 0.85));
  color: var(--sc-text-primary);
  border: 1px solid var(--sc-border-mute);
  border-left: 3px solid var(--sc-primary, #3b82f6);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.12);
}

.reply-banner__main {
  display: flex;
  align-items: center;
  gap: 8px;
}

.reply-banner__badge {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  color: var(--sc-primary-color, #2563eb);
  background-color: rgba(var(--sc-primary-rgb, 37, 99, 235), 0.15);
  border: 1px solid rgba(var(--sc-primary-rgb, 37, 99, 235), 0.35);
}

.reply-banner__target {
  font-weight: 600;
}

.reply-banner__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.reply-banner__hint {
  font-size: 12px;
  color: var(--sc-text-secondary, #6b7280);
}

.scroll-bottom-button {
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.18);
}

:root[data-display-palette='night'] .scroll-bottom-button {
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.65);
}

/* 跳转到未读按钮样式 */
.jump-to-unread-button {
  position: relative;
  padding-right: 28px;
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.15);
  background-color: var(--sc-chip-bg) !important;
  border-color: var(--sc-border-mute) !important;
  color: var(--sc-text-primary) !important;
}

.jump-to-unread-button:hover {
  background-color: var(--sc-bg-hover, rgba(156, 163, 175, 0.12)) !important;
  border-color: var(--sc-border-strong) !important;
}

.jump-to-unread-close {
  position: absolute;
  top: -6px;
  right: -6px;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  border: 1px solid var(--sc-border-mute);
  background-color: var(--sc-bg-surface, #ffffff);
  color: var(--sc-text-secondary, #6b7280);
  font-size: 11px;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.12);
  transition: background-color 0.2s ease, color 0.2s ease, border-color 0.2s ease;
}

.jump-to-unread-close:hover {
  background-color: var(--sc-bg-hover, rgba(156, 163, 175, 0.12));
  color: var(--sc-text-primary, #1f2937);
  border-color: var(--sc-border-strong);
}

.message-sentinel {
  width: 100%;
  height: 1px;
}

.history-floating {
  position: absolute;
  right: 20px;
  bottom: calc(100% + 16px);
  z-index: 50;
}

@media (max-width: 768px) {
  .history-floating {
    right: 12px;
    bottom: calc(100% + 12px);
  }
}

.history-floating__button {
  align-self: flex-end;
}

.history-mode-hint {
  padding: 0.35rem 0.75rem;
  border-radius: 999px;
  font-size: 0.875rem;
  background-color: rgba(15, 23, 42, 0.75);
  color: #fff;
  box-shadow: 0 8px 16px rgba(0, 0, 0, 0.2);
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

:root[data-display-palette='day'] .history-mode-hint {
  background-color: rgba(255, 255, 255, 0.9);
  color: #111827;
  border: 1px solid rgba(148, 163, 184, 0.5);
}

.history-mode-hint--mobile {
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
}

.history-mode-hint__label {
  font-weight: 600;
}

.chat-input-wrapper {
  position: relative;
  display: flex;
  flex-direction: column;
  width: 100%;
  flex: 1;
  min-width: 0;
}

.chat-input-container {
  width: 100%;
  background-color: transparent;
  border: none;
  border-radius: 0;
  padding: 0;
  margin: 0;
  box-shadow: none;
  transition: background-color 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
  position: relative;

  // 上边框拖拽热区
  &::before {
    content: '';
    position: absolute;
    top: -4px;
    left: 0;
    right: 0;
    height: 12px;
    cursor: row-resize;
    z-index: 1;
    touch-action: none;
  }

  // 可见的分隔线
  &::after {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 1px;
    background: var(--sc-border-mute, rgba(148, 163, 184, 0.3));
    transition: background-color 0.15s ease;
    pointer-events: none;
  }

  &:hover::after {
    background: var(--sc-border-mute, rgba(148, 163, 184, 0.5));
  }

  &.chat-input-container--resizing::after {
    background: var(--primary-color, #3b82f6);
  }

  &.chat-input-container--resizing {
    overscroll-behavior: contain;
  }
}

// 移动端增大热区
@media (max-width: 768px), (pointer: coarse) {
  .chat-input-container::before {
    top: -8px;
    height: 20px;
  }

  .chat-input-container {
    touch-action: none;
  }
}

.chat-input-container--spectator-hidden {
  display: none;
}

:root[data-display-palette='night'] .chat-input-container {
  box-shadow: none;

  &::after {
    background: rgba(161, 161, 170, 0.25);
  }

  &:hover::after {
    background: rgba(161, 161, 170, 0.4);
  }

  &.chat-input-container--resizing::after {
    background: var(--primary-color, #60a5fa);
  }
}

.chat-input-area {
  position: relative;
  display: flex;
  flex-direction: column;
  background-color: transparent;
  border: none;
  border-radius: 0;
  padding: 0;
  margin: 0.25rem 0;
  gap: 0;
  transition: background-color 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
}

.chat-input-area :deep(.n-input) {
  width: 100%;
}

.chat-input-actions {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: clamp(0.3rem, 0.9vw, 0.5rem);
  margin-top: 0;
  flex: 1 1 auto;
  min-width: 0;
  flex-wrap: nowrap;
  overflow: visible;
}

.chat-input-actions__group {
  display: inline-flex;
  align-items: center;
  gap: clamp(0.2rem, 0.7vw, 0.35rem);
  flex-wrap: nowrap;
}

.chat-input-editor-row {
  display: flex;
  align-items: flex-end;
  gap: 0.75rem;
  margin-top: 0.75rem;
}

.chat-input-editor-main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  position: relative;
}

.chat-input-editor-main :deep(.hybrid-input) {
  width: 100%;
}

.chat-input-send-inline {
  flex: 0 0 auto;
  display: flex;
  align-items: flex-end;
  align-self: stretch;
}

.chat-input-send-inline .n-button {
  width: 44px;
  height: 44px;
  flex-shrink: 0;
}

.edit-actions-group {
  display: flex;
  flex-direction: column;
  gap: 0;
  height: 100%;
  max-height: 88px;
  min-height: 44px;
}

.send-action-btn {
  width: 44px !important;
  height: 44px !important;
  border-radius: 10px !important;
  padding: 0 !important;
  background-color: var(--sc-chip-bg) !important;
  border-color: var(--sc-border-mute) !important;
  color: var(--sc-text-primary) !important;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

.send-action-btn:hover:not(:disabled) {
  background-color: rgba(59, 130, 246, 0.15) !important;
  border-color: rgba(59, 130, 246, 0.4) !important;
  color: #2563eb !important;
}

.send-action-btn:disabled {
  opacity: 0.5;
}

:root[data-display-palette='night'] .send-action-btn:hover:not(:disabled) {
  background-color: rgba(96, 165, 250, 0.2) !important;
  border-color: rgba(96, 165, 250, 0.45) !important;
  color: #60a5fa !important;
}

.edit-action-btn {
  width: 44px !important;
  height: auto !important;
  flex: 1 1 0;
  min-height: 20px;
  max-height: 44px;
  border-radius: 0 !important;
  padding: 0 !important;
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.edit-action-btn--save {
  border-top-left-radius: 6px !important;
  border-top-right-radius: 6px !important;
  background-color: rgba(34, 197, 94, 0.15) !important;
  border-color: rgba(34, 197, 94, 0.3) !important;
  color: #16a34a !important;
}

.edit-action-btn--save:hover {
  background-color: rgba(34, 197, 94, 0.25) !important;
  border-color: rgba(34, 197, 94, 0.5) !important;
}

.edit-action-btn--cancel {
  border-bottom-left-radius: 6px !important;
  border-bottom-right-radius: 6px !important;
  background-color: var(--sc-chip-bg) !important;
  border-color: var(--sc-border-mute) !important;
  color: var(--sc-text-secondary) !important;
}

.edit-action-btn--cancel:hover {
  background-color: rgba(239, 68, 68, 0.12) !important;
  border-color: rgba(239, 68, 68, 0.3) !important;
  color: #dc2626 !important;
}

:root[data-display-palette='night'] .edit-action-btn--save {
  background-color: rgba(34, 197, 94, 0.2) !important;
  border-color: rgba(34, 197, 94, 0.35) !important;
  color: #4ade80 !important;
}

:root[data-display-palette='night'] .edit-action-btn--save:hover {
  background-color: rgba(34, 197, 94, 0.3) !important;
  border-color: rgba(34, 197, 94, 0.5) !important;
}

:root[data-display-palette='night'] .edit-action-btn--cancel:hover {
  background-color: rgba(239, 68, 68, 0.2) !important;
  border-color: rgba(239, 68, 68, 0.4) !important;
  color: #f87171 !important;
}

.chat-input-actions__cell {
  flex: 0 1 auto;
}

.chat-input-actions__cell .n-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.chat-input-actions__cell .n-button {
  width: clamp(24px, 2.8vw, 32px);
  height: clamp(24px, 2.8vw, 32px);
}

@media (max-width: 520px) {
  .chat-input-actions {
    gap: 0.25rem;
  }

  .chat-input-actions__group {
    gap: 0.2rem;
  }

  .chat-input-actions__cell .n-button {
    width: 24px;
    height: 24px;
  }

  .chat-input-actions__icon {
    font-size: 0.75rem;
  }

  .chat-input-editor-row {
    gap: 0.5rem;
  }

  .chat-input-send-inline .n-button {
    width: 40px;
    height: 40px;
  }

  .send-action-btn {
    width: 40px !important;
    height: 40px !important;
    border-radius: 8px !important;
  }

  .edit-actions-group {
    max-height: 80px;
    min-height: 40px;
  }

  .edit-action-btn {
    width: 40px !important;
    max-height: 40px;
  }
}

@media (max-width: 420px) {
  .chat-input-actions {
    gap: 0.2rem;
  }

  .chat-input-actions__cell .n-button {
    width: 22px;
    height: 22px;
  }

  .chat-input-actions__icon {
    font-size: 0.65rem;
  }
}

.chat-input-actions__cell .n-button:disabled {
  opacity: 0.55;
}

.chat-dice-button {
  color: var(--sc-text-primary);
}

:root[data-display-palette='night'] .chat-dice-button {
  color: rgba(226, 232, 240, 0.95);
}

.dice-mode-status {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  cursor: pointer;
}

.dice-mode-status__label {
  font-size: 11px;
  color: var(--sc-text-tertiary, #94a3b8);
  white-space: nowrap;
}

:root[data-display-palette='night'] .dice-mode-status__label {
  color: rgba(148, 163, 184, 0.85);
}

.dice-tray-settings-trigger {
  width: 1.5rem;
  height: 1.5rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  color: var(--sc-text-secondary);
  border: 1px solid transparent;
  transition: color 0.15s ease, border-color 0.15s ease, background-color 0.15s ease;
}

:root[data-display-palette='night'] .dice-tray-settings-trigger {
  color: rgba(226, 232, 240, 0.8);
}

.dice-tray-settings-trigger--active {
  color: var(--sc-primary-color, #2563eb);
  border-color: rgba(37, 99, 235, 0.4);
  background-color: rgba(37, 99, 235, 0.08);
}

:root[data-display-palette='night'] .dice-tray-settings-trigger--active {
  color: rgba(147, 197, 253, 0.95);
  border-color: rgba(147, 197, 253, 0.35);
  background-color: rgba(59, 130, 246, 0.18);
}

.dice-settings-panel {
  min-width: 260px;
  max-width: 320px;
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.dice-settings-panel--modal {
  min-width: 0;
  width: 100%;
  max-width: 100%;
  padding-right: 0;
}

.dice-settings-panel--modal .dice-settings-panel__section {
  padding-right: 0;
}

.dice-settings-panel--modal .dice-settings-panel__footer {
  padding-right: 0;
}

.dice-settings-modal-mobile :deep(.n-card) {
  width: min(360px, 92vw);
}

.dice-settings-modal-mobile :deep(.n-card__content) {
  padding-top: 0;
  max-height: min(70vh, 520px);
  overflow-y: auto;
}

.dice-settings-panel__section {
  border: 1px solid var(--sc-border-strong);
  border-radius: 0.75rem;
  padding: 0.65rem 0.75rem;
  background-color: var(--sc-bg-elevated);
}

.dice-settings-panel__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.dice-settings-panel__title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--sc-text-primary);
  margin: 0;
}

.dice-settings-panel__desc {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  margin: 0.1rem 0 0;
}

.dice-settings-panel__body {
  margin-top: 0.65rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dice-settings-panel__select {
  width: 100%;
}

.dice-settings-panel__hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.dice-settings-panel__footer {
  margin-top: 0.35rem;
  display: flex;
  justify-content: flex-end;
}


:deep(.history-popover .n-popover__content) {
  padding: 0;
  border-radius: 0.75rem;
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.18);
  min-width: 18rem;
  max-width: 22rem;
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
}

.history-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.9rem 1rem 1rem;
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
}

.history-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.history-panel__title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--sc-text-primary, #1f2937);
}

.history-panel__body {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: 14rem;
  overflow-y: auto;
  padding-right: 0.2rem;
  color: var(--sc-text-primary, #0f172a);

  /* 极简滚动条 */
  &::-webkit-scrollbar {
    width: 3px;
  }
  &::-webkit-scrollbar-track {
    background: transparent;
  }
  &::-webkit-scrollbar-thumb {
    background-color: rgba(148, 163, 184, 0.4);
    border-radius: 3px;
  }
  &::-webkit-scrollbar-thumb:hover {
    background-color: rgba(148, 163, 184, 0.7);
  }
}

.history-entry {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  width: 100%;
  text-align: left;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
  border-radius: 0.75rem;
  padding: 0.65rem 0.75rem;
  background: var(--sc-bg-subtle, rgba(248, 250, 252, 0.9));
  transition: border-color 0.2s ease, background-color 0.2s ease, box-shadow 0.2s ease;
}

:root[data-display-palette='night'] .history-entry {
  background: rgba(30, 41, 59, 0.5);
  border-color: rgba(71, 85, 105, 0.4);
}

.history-entry:hover {
  border-color: var(--sc-primary-color-hover, rgba(59, 130, 246, 0.35));
  background: var(--sc-bg-base, rgba(239, 246, 255, 0.92));
  box-shadow: 0 6px 16px rgba(59, 130, 246, 0.18);
}

:root[data-display-palette='night'] .history-entry:hover {
  background: rgba(51, 65, 85, 0.6);
  border-color: rgba(59, 130, 246, 0.5);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.history-entry__meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #6b7280);
}

.history-entry__tag {
  padding: 0.05rem 0.45rem;
  border-radius: 999px;
  background: rgba(99, 102, 241, 0.16);
  color: #4c51bf;
  font-weight: 500;
}

.history-entry__tag--rich {
  background: rgba(16, 185, 129, 0.16);
  color: #047857;
}

.history-entry__time {
  flex: 1;
  text-align: right;
}

.history-entry__preview {
  font-size: 0.85rem;
  color: var(--sc-text-primary, #1f2937);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-word;
}

.history-panel__empty {
  text-align: center;
  color: #6b7280;
  font-size: 0.85rem;
  padding: 1.2rem 0.5rem;
  border-radius: 0.65rem;
  background: rgba(248, 250, 252, 0.9);
}

.history-panel__hint {
  margin-top: 0.35rem;
  font-size: 0.78rem;
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
  width: 36px;
  height: 36px;
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

.whisper-pill-wrapper {
  padding: 0.35rem 1rem 0.25rem;
}

.whisper-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  background-color: rgba(124, 58, 237, 0.14);
  color: #5b21b6;
  font-size: 0.85rem;
  font-weight: 500;
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
  background: var(--sc-bg-elevated);
  border-radius: 0.75rem;
  border: 1px solid var(--sc-border-strong);
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

.whisper-panel__name-row {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
}

.whisper-panel__name {
  flex: 1;
  min-width: 0;
  font-size: 0.9rem;
  font-weight: 600;
  color: #4338ca;
}

.whisper-panel__tags {
  display: flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.whisper-panel__tag {
  display: inline-flex;
  align-items: center;
  padding: 0 0.35rem;
  border-radius: 0.35rem;
  font-size: 0.65rem;
  line-height: 1.4;
  background: rgba(67, 56, 202, 0.12);
  color: #4338ca;
}

.whisper-panel__tag--ic {
  background: rgba(16, 185, 129, 0.16);
  color: #047857;
}

.whisper-panel__tag--ooc {
  background: rgba(14, 165, 233, 0.16);
  color: #0369a1;
}

.whisper-panel__tag--user {
  background: rgba(107, 114, 128, 0.16);
  color: #4b5563;
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

.whisper-panel__checkbox {
  margin-left: auto;
}

.whisper-panel__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 8px 12px 0;
  border-top: 1px solid var(--sc-border-mute);
  margin-top: 0.6rem;
}

.whisper-pills {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  background: var(--sc-bg-elevated);
  border-bottom: 1px solid var(--sc-border-mute);
}

.whisper-pill-prefix {
  font-size: 12px;
  color: var(--sc-text-secondary);
  margin-right: 4px;
}

.whisper-pill-tag {
  max-width: 100px;
}

.identity-switcher-cell {
  display: flex;
  align-items: center;
}

.input-floating-toolbar {
  position: static;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: clamp(0.2rem, 0.7vw, 0.4rem);
  flex-wrap: nowrap;
  min-width: 0;
}

.input-floating-toolbar :deep(.n-button.n-button--primary-type.n-button--medium-type.n-button--circle) {
  width: clamp(24px, 2.8vw, 32px);
  height: clamp(24px, 2.8vw, 32px);
  padding: 0;
}

:root[data-display-palette='night'] .input-floating-toolbar :deep(.n-button:not([disabled]) .n-icon),
:root[data-display-palette='night'] .input-floating-toolbar :deep(.n-button:not([disabled]) .n-button__icon > svg),
:root[data-display-palette='night'] .input-floating-toolbar :deep(.n-button:not([disabled]) .n-button__icon) {
  color: rgba(255, 255, 255, 0.88);
}

:root[data-display-palette='night'] :deep(.n-dropdown-menu.n-popover-shared.n-dropdown) {
  color: rgba(248, 250, 252, 0.95);
}

:root[data-display-palette='night'] :deep(.n-dropdown-menu.n-popover-shared.n-dropdown .n-dropdown-option__label),
:root[data-display-palette='night'] :deep(.n-dropdown-menu.n-popover-shared.n-dropdown .n-dropdown-option__extra),
:root[data-display-palette='night'] :deep(.n-dropdown-menu.n-popover-shared.n-dropdown .n-dropdown-option__content) {
  color: rgba(248, 250, 252, 0.95);
}

@media (max-width: 600px) {
  .input-floating-toolbar {
    flex-wrap: wrap;
  }
}

.emoji-panel {
  width: 380px;
  max-height: 400px;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.emoji-panel__content {
  overflow-y: auto;
  max-height: 320px;
  padding-right: 4px;
}

@media (max-width: 768px) {
  .emoji-panel {
    width: calc(100vw - 32px);
    max-width: 320px;
  }
}

.emoji-panel__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.emoji-panel__header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.emoji-panel__header-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.emoji-panel__toggle-remark :deep(.n-icon) {
  margin-left: 4px;
}

.emoji-panel__title {
  font-weight: 600;
}

.emoji-panel__tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
  padding-bottom: 4px;
}

.emoji-panel__tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 10px;
  font-size: 12px;
  line-height: 1.4;
  border: 1px solid var(--border-color, rgba(0, 0, 0, 0.1));
  border-radius: 12px;
  background: var(--sc-bg-elevated, #f8fafc);
  color: var(--sc-text-secondary, #64748b);
  cursor: pointer;
  transition: all 0.15s ease;
  max-width: 100px;
  white-space: nowrap;
  overflow: hidden;
}

.emoji-panel__tab:hover {
  background: var(--sc-bg-hover, #e2e8f0);
  border-color: var(--border-color-hover, rgba(0, 0, 0, 0.15));
}

.emoji-panel__tab--active {
  background: var(--primary-color, #18a058);
  border-color: var(--primary-color, #18a058);
  color: #fff;
}

.emoji-panel__tab--active:hover {
  background: var(--primary-color-hover, #16924e);
  border-color: var(--primary-color-hover, #16924e);
}

.emoji-panel__tab-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.emoji-panel__search {
  margin-top: 8px;
  margin-bottom: 8px;
}

.emoji-panel__empty {
  text-align: center;
  font-size: 13px;
  color: var(--text-color-3);
  padding: 12px 0;
}

.emoji-panel__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.emoji-section__title {
  font-size: 12px;
  color: var(--text-color-3);
}

.emoji-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(64px, 1fr));
  gap: 0.5rem;
}

@media (max-width: 768px) {
  .emoji-grid {
    grid-template-columns: repeat(3, minmax(60px, 1fr));
    gap: 0.4rem;
  }
}

.emoji-item {
  display: flex;
  flex-direction: column;
  touch-action: manipulation;
  align-items: center;
  gap: 0.25rem;
  cursor: pointer;
  border-radius: 8px;
  padding: 0.15rem;
  transition: background-color 0.15s ease;
}

.emoji-item img {
  width: 4.2rem;
  height: 4.2rem;
  object-fit: contain;
}

.emoji-item:hover {
  background-color: rgba(255, 255, 255, 0.06);
}

.emoji-caption {
  font-size: 12px;
  color: var(--text-color-3);
  text-align: center;
  width: 100%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.emoji-item.is-active {
  background-color: rgba(255, 255, 255, 0.12);
}

.emoji-item__actions {
  display: flex;
  gap: 0.25rem;
  opacity: 0;
  transition: opacity 0.2s ease;
}

.emoji-item:hover .emoji-item__actions {
  opacity: 1;
}

.emoji-manage-item__content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}

.emoji-panel--hide-remark .emoji-caption,
.emoji-panel--hide-remark .emoji-item__actions {
  display: none;
}

.emoji-panel--hide-remark .emoji-manage-item__content :deep(.n-button) {
  display: none;
}

@media (max-width: 768px) {
  .emoji-item img {
    width: 4.8rem;
    height: 4.8rem;
  }
}

.emoji-manage-item :deep(.n-checkbox) {
  width: 100%;
  display: flex;
  justify-content: center;
}

.emoji-manage-item :deep(.n-checkbox__label) {
  padding: 0;
}


.identity-color-field {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.identity-color-picker {
  width: 36px;
  height: 32px;
  :deep(.n-color-picker-trigger) {
    padding: 0;
    border-radius: 8px;
    justify-content: center;
  }
  :deep(.n-color-picker-trigger__icon) {
    margin-right: 0;
  }
  :deep(.n-color-picker-trigger__value) {
    display: none;
  }
}

.identity-color-input {
  width: 110px;
}

.identity-avatar-field {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.identity-manager {
  display: grid;
  grid-template-columns: minmax(140px, 160px) minmax(0, 1fr);
  gap: 1rem;
  min-height: 420px;
  overflow: hidden;
}

.identity-manager__sidebar {
  border-right: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.2));
  padding-right: 0.75rem;
}

.identity-folder-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}

.identity-folder-header__title {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-weight: 600;
}

.identity-folder-list {
  max-height: 360px;
}

.identity-folder-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.35rem 0.4rem;
  border-radius: 8px;
  cursor: pointer;
  transition: background-color 0.2s ease;
}

.identity-folder-item + .identity-folder-item {
  margin-top: 0.25rem;
}

.identity-folder-item.is-active {
  background-color: rgba(59, 130, 246, 0.12);
  color: #2563eb;
}

.identity-folder-item.is-disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.identity-folder-item__label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-weight: 500;
}

.identity-folder-item__favorite {
  color: var(--sc-text-secondary, #94a3b8);
}

.identity-folder-item__favorite.is-active {
  color: #fbbf24;
}

.identity-folder-item__count {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #94a3b8);
}

.identity-folder-item__meta {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.identity-manager__content {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding-left: 0.25rem;
}

.identity-manager__toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding-bottom: 0.65rem;
  border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
}

.identity-manager__selection {
  font-size: 0.85rem;
  color: var(--sc-text-secondary, #6b7280);
}

.identity-manager__folder-select {
  flex: 1 1 160px;
  min-width: 140px;
  max-width: 220px;
}

.identity-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.identity-list--grid {
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  display: grid;
  gap: 0.75rem;
}

.identity-list__item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 0;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
  border-radius: 12px;
  padding: 0.7rem;
  width: 100%;
  flex-wrap: wrap;
  box-sizing: border-box;
}

.identity-list__item--selectable {
  position: relative;
  padding-left: 2.1rem;
}

.identity-list__item-check {
  position: absolute;
  top: 0.9rem;
  left: 0.65rem;
}

.identity-list__item--selectable .identity-list__meta {
  margin-left: 0;
}

.identity-list__item.is-selected {
  border-color: rgba(59, 130, 246, 0.45);
  background-color: rgba(59, 130, 246, 0.08);
}

.identity-list__meta {
  flex: 1;
  min-width: 0;
}

.identity-list__name {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-weight: 600;
}

.identity-list__color {
  width: 12px;
  height: 12px;
  border-radius: 9999px;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.4));
}

.identity-list__actions {
  display: flex;
  gap: 0.4rem;
  margin-left: auto;
  flex-wrap: wrap;
}

.identity-list__hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #6b7280);
  margin-top: 0.25rem;
}

.identity-list__folders {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.35rem;
}

.identity-manage-drawer--night .identity-folder-item__count,
.identity-manage-drawer--night .identity-manager__selection,
.identity-manage-drawer--night .identity-list__hint {
  color: rgba(226, 232, 240, 0.7);
}

.identity-manage-drawer--night .identity-folder-item {
  color: rgba(248, 250, 252, 0.9);
}

.identity-manage-drawer--night .identity-folder-item.is-active {
  background-color: rgba(59, 130, 246, 0.25);
  color: #bfdbfe;
}

.identity-manage-drawer--night .identity-list__item {
  border-color: rgba(59, 130, 246, 0.25);
  background-color: rgba(15, 23, 42, 0.4);
}

.identity-manage-drawer--night .identity-list__actions :deep(.n-button) {
  color: rgba(248, 250, 252, 0.85);
}

@media (max-width: 960px) {
  .identity-manager {
    grid-template-columns: minmax(130px, 150px) minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .identity-manage-shell :deep(.n-drawer) {
    width: 100% !important;
  }

  .identity-manager {
    grid-template-columns: 1fr;
  }

  .identity-manager__sidebar {
    border-right: none;
    border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.2));
    padding-right: 0;
    padding-bottom: 0.75rem;
    margin-bottom: 0.75rem;
  }

  .identity-manager__toolbar {
    flex-direction: column;
    align-items: flex-start;
    width: 100%;
  }

  .identity-manager__folder-select {
    width: 100%;
    max-width: none;
  }

  .identity-manager__selection {
    margin-left: 0;
  }

  .identity-list--grid {
    grid-template-columns: 1fr;
  }

  .identity-list__item {
    flex-direction: column;
    align-items: flex-start;
    width: 100%;
  }

  .identity-list__item-check {
    position: static;
    margin-bottom: 0.35rem;
    align-self: flex-start;
  }

  .identity-list__item--selectable .identity-list__meta {
    margin-left: 0;
  }
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

/* 过渡动画 */
.slide-down-enter-active,
.slide-down-leave-active {
  transition: all 0.3s ease;
}

.slide-down-enter-from,
.slide-down-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

</style>

<style lang="scss">
.chat>.virtual-list__client {
  &>div {
    margin-bottom: -1rem;
  }
}

.chat-text>.n-input>.n-input-wrapper {
  background-color: var(--sc-bg-input);
  border: 1px solid var(--sc-border-mute);
  padding: 0.75rem 1.25rem;
  border-radius: 0.85rem;
  transition: background-color 0.25s ease, border-color 0.25s ease;
}

:global(.dice-tray-mobile-wrapper) {
  width: min(92vw, 420px) !important;
  max-width: 100vw;
  left: 4vw !important;
  right: 4vw !important;
  position: fixed !important;
}

:global(.dice-tray-mobile-wrapper .dice-tray) {
  width: 100%;
  min-width: 0;
}

:global(.dice-tray-mobile-wrapper .dice-tray__body) {
  flex-direction: column;
  gap: 0.75rem;
}

:global(.dice-tray-mobile-wrapper .dice-tray__column--quick) {
  flex: 1;
}

:global(.dice-tray-mobile-wrapper .dice-tray__history) {
  max-height: 45vh;
  overflow-y: auto;
}
.identity-dialog :deep(.n-card) {
  background: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  border: 1px solid var(--sc-border-strong, rgba(15, 23, 42, 0.12));
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.15);
}

.identity-dialog :deep(.n-card__header),
.identity-dialog :deep(.n-card__content),
.identity-dialog :deep(.n-card__footer) {
  color: var(--sc-text-primary, #0f172a);
}

.identity-dialog :deep(.n-form-item-label__text) {
  color: var(--sc-text-secondary, #475569);
}

.identity-manage-shell :deep(.n-drawer),
.identity-manage-shell :deep(.n-drawer-body) {
  background-color: transparent;
}

.identity-manage-shell :deep(.n-drawer-body) {
  transition: background-color 0.25s ease, color 0.25s ease;
  padding: 0;
  overflow-x: hidden;
}

.identity-manage-drawer {
  background: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  min-height: 100%;
}

.identity-manage-drawer--night {
  background: #0f172a;
  color: rgba(248, 250, 252, 0.95);
}

.dice-chip {
  display: inline-flex !important;
  align-items: center;
  gap: 0.25rem;
  padding: 0.15rem 0.45rem;
  border-radius: 0.45rem;
  border: 1px solid rgba(15, 23, 42, 0.16);
  background: rgba(248, 250, 252, 0.95);
  color: #1f2937;
  font-size: 0.82rem;
  line-height: 1.15;
  vertical-align: middle;
  white-space: nowrap;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

.dice-chip__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  opacity: 0.9;
  margin-right: 0.2rem;
  font-size: 1em;
  line-height: 1;
}

.dice-chip__formula {
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  margin-right: 0.15rem;
}

.dice-chip__equals {
  font-size: 0.78em;
  opacity: 0.65;
  margin-right: 0.1rem;
}

.dice-chip__result {
  font-weight: 600;
  display: inline-flex;
  align-items: center;
}

.dice-chip--preview {
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: inherit;
  font-size: inherit;
  line-height: inherit;
  gap: 0;
  white-space: inherit;
}

.dice-chip--preview .dice-chip__icon {
  display: inline-flex;
  margin-right: 0.2rem;
}

.dice-chip--preview .dice-chip__formula,
.dice-chip--preview .dice-chip__equals,
.dice-chip--preview .dice-chip__result {
  font-weight: inherit;
  font-size: inherit;
  opacity: 1;
  margin-right: 0;
}

.dice-chip--error {
  border-color: rgba(220, 38, 38, 0.55);
  background: rgba(254, 226, 226, 0.95);
  color: #991b1b;
}

.dice-chip--error .dice-chip__result {
  color: inherit;
}

.dice-chip--tone-ic:not(.dice-chip--preview),
[data-dice-tone='ic']:not(.dice-chip--preview) {
  background: #fafbf8;
  border-color: rgba(15, 23, 42, 0.16);
  color: #1f2937;
}

.dice-chip--tone-ooc:not(.dice-chip--preview),
[data-dice-tone='ooc']:not(.dice-chip--preview) {
  background: color-mix(in srgb, var(--chat-ooc-bg) 85%, var(--sc-text-primary) 15%);
  border-color: color-mix(in srgb, var(--chat-ooc-border) 70%, var(--sc-text-primary) 30%);
  color: var(--sc-text-primary);
}

.dice-chip--tone-archived:not(.dice-chip--preview),
[data-dice-tone='archived']:not(.dice-chip--preview) {
  background: rgba(148, 163, 184, 0.2);
  border-color: rgba(148, 163, 184, 0.4);
  color: #334155;
}

:root[data-display-palette='night'] .dice-chip {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(148, 163, 184, 0.35);
  color: #f3f4f6;
}

:root[data-display-palette='night'] .dice-chip--preview {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.35);
  color: #f8fafc;
}

:root[data-display-palette='night'] .dice-chip--error {
  background: rgba(127, 29, 29, 0.7);
  border-color: rgba(248, 113, 113, 0.75);
  color: #fecaca;
}

:root[data-display-palette='night'] .dice-chip--tone-ic:not(.dice-chip--preview),
:root[data-display-palette='night'] [data-dice-tone='ic']:not(.dice-chip--preview) {
  background: #333135;
  border-color: rgba(255, 255, 255, 0.18);
  color: #f4f4f5;
}

:root[data-display-palette='night'] .dice-chip--tone-ooc:not(.dice-chip--preview),
:root[data-display-palette='night'] [data-dice-tone='ooc']:not(.dice-chip--preview) {
  background: color-mix(in srgb, var(--chat-ooc-bg) 85%, var(--sc-text-primary) 15%);
  border-color: color-mix(in srgb, var(--chat-ooc-border) 70%, var(--sc-text-primary) 30%);
  color: var(--sc-text-primary);
}

:root[data-display-palette='night'] .dice-chip--tone-archived:not(.dice-chip--preview),
:root[data-display-palette='night'] [data-dice-tone='archived']:not(.dice-chip--preview) {
  background: rgba(51, 65, 85, 0.65);
  border-color: rgba(148, 163, 184, 0.4);
  color: #e2e8f0;
}

.dice-chip:not(.dice-chip--preview) {
  box-shadow: var(--chat-dice-result-shadow);
  color: inherit;
  font-size: inherit;
  line-height: inherit;
  gap: 0;
  white-space: inherit;
}

.dice-chip:not(.dice-chip--preview) .dice-chip__icon {
  display: inline-flex;
  margin-right: 0.2rem;
}

.dice-chip:not(.dice-chip--preview) .dice-chip__formula,
.dice-chip:not(.dice-chip--preview) .dice-chip__equals,
.dice-chip:not(.dice-chip--preview) .dice-chip__result {
  font-weight: inherit;
  font-size: inherit;
  opacity: 1;
  margin-right: 0;
}

/* Keyword Tooltip Styles */
:global(.keyword-tooltip) {
  position: fixed;
  z-index: 9999;
  max-width: 360px;
  min-width: 180px;
  padding: 12px 16px;
  border-radius: 10px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
  background: #ffffff;
  border: 1px solid rgba(15, 23, 42, 0.12);
  color: #0f172a;
  font-size: 14px;
  line-height: 1.55;
  pointer-events: auto;
  animation: keyword-tooltip-fade-in 0.15s ease-out;
  transition: max-width 0.2s ease;
  overflow-wrap: break-word;
}

/* Tooltip scrollbar styling - minimal/invisible design */
/* Standard properties for Firefox */
:global(.keyword-tooltip) {
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}

:global(.keyword-tooltip:hover) {
  scrollbar-color: rgba(128, 128, 128, 0.2) transparent;
}

/* WebKit properties for Chrome/Safari/Edge */
:global(.keyword-tooltip::-webkit-scrollbar) {
  width: 4px;
  height: 4px;
}

:global(.keyword-tooltip::-webkit-scrollbar-track) {
  background: transparent;
}

:global(.keyword-tooltip::-webkit-scrollbar-thumb) {
  background: transparent;
  border-radius: 2px;
}

:global(.keyword-tooltip:hover::-webkit-scrollbar-thumb) {
  background: rgba(128, 128, 128, 0.2);
}

@keyframes keyword-tooltip-fade-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

:global(.keyword-tooltip--pinned) {
  box-shadow: 0 12px 32px rgba(15, 23, 42, 0.22);
  transition: opacity 0.15s ease, filter 0.15s ease;
}

/* Parent tooltip dims when child tooltip exists */
:global(.keyword-tooltip--pinned.keyword-tooltip--has-child) {
  opacity: 0.7;
  filter: brightness(0.92);
}

/* Enhanced shadow for nested tooltips by level */
:global(.keyword-tooltip--pinned[data-level="1"]) {
  box-shadow: 0 16px 40px rgba(15, 23, 42, 0.28);
  border-width: 1.5px;
}

:global(.keyword-tooltip--pinned[data-level="2"]) {
  box-shadow: 0 20px 48px rgba(15, 23, 42, 0.32);
  border-width: 2px;
}

:global(.keyword-tooltip--pinned[data-level="3"]) {
  box-shadow: 0 24px 56px rgba(15, 23, 42, 0.36);
  border-width: 2px;
}

:global(.keyword-tooltip__header) {
  font-weight: 600;
  margin-bottom: 6px;
  color: #1e293b;
  font-size: 15px;
}

:global(.keyword-tooltip__body) {
  color: #475569;
  white-space: pre-wrap;
  word-break: break-word;
  pointer-events: auto;
}

/* 多段首行缩进样式 */
:global(.keyword-tooltip__body--indented .keyword-tooltip__paragraph) {
  text-indent: var(--keyword-tooltip-text-indent, 0);
  margin: 0;
  padding: 0;
}

:global(.keyword-tooltip__body--indented .keyword-tooltip__paragraph + .keyword-tooltip__paragraph) {
  margin-top: 0.5em;
}

:global(.keyword-tooltip__body .keyword-highlight) {
  cursor: pointer;
  pointer-events: auto;
}

/* Night mode tooltip */
:global([data-display-palette='night'] .keyword-tooltip),
:global(:root[data-display-palette='night'] .keyword-tooltip) {
  background: #1e1e22;
  border-color: rgba(255, 255, 255, 0.12);
  color: #f4f4f5;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
}

:global([data-display-palette='night'] .keyword-tooltip--pinned),
:global(:root[data-display-palette='night'] .keyword-tooltip--pinned) {
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.55);
}

:global([data-display-palette='night'] .keyword-tooltip__header),
:global(:root[data-display-palette='night'] .keyword-tooltip__header) {
  color: #fafafa;
}

:global([data-display-palette='night'] .keyword-tooltip__body),
:global(:root[data-display-palette='night'] .keyword-tooltip__body) {
  color: rgba(248, 250, 252, 0.8);
}

/* Night mode tooltip scrollbar - minimal/invisible design */
/* Firefox */
:global([data-display-palette='night'] .keyword-tooltip:hover),
:global(:root[data-display-palette='night'] .keyword-tooltip:hover) {
  scrollbar-color: rgba(255, 255, 255, 0.2) transparent;
}

/* WebKit */
:global([data-display-palette='night'] .keyword-tooltip:hover::-webkit-scrollbar-thumb),
:global(:root[data-display-palette='night'] .keyword-tooltip:hover::-webkit-scrollbar-thumb) {
  background: rgba(255, 255, 255, 0.2);
}

/* Keyword Highlight Styles */
:global(.keyword-highlight) {
  display: inline;
  padding: 0 3px;
  margin: 0 1px;
  border-bottom: 1px dashed rgba(168, 108, 0, 0.85);
  background: rgba(255, 230, 150, 0.85);
  border-radius: 2px;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

:global(.keyword-highlight:not(.keyword-highlight--underline):hover) {
  background: rgba(255, 220, 120, 0.95);
}

:global(.keyword-highlight--underline) {
  background: transparent;
  border-bottom-style: dotted;
}

:global(.keyword-highlight--underline:hover) {
  background: transparent;
}

/* Night mode highlight */
:global([data-display-palette='night'] .keyword-highlight),
:global(:root[data-display-palette='night'] .keyword-highlight) {
  background: rgba(180, 140, 60, 0.35);
  border-bottom-color: rgba(220, 180, 80, 0.7);
  color: #fef3c7;
}

:global([data-display-palette='night'] .keyword-highlight:not(.keyword-highlight--underline):hover),
:global(:root[data-display-palette='night'] .keyword-highlight:not(.keyword-highlight--underline):hover) {
  background: rgba(180, 140, 60, 0.5);
}

:global([data-display-palette='night'] .keyword-highlight--underline),
:global(:root[data-display-palette='night'] .keyword-highlight--underline) {
  background: transparent;
  color: inherit;
}

:global([data-display-palette='night'] .keyword-highlight--underline:hover),
:global(:root[data-display-palette='night'] .keyword-highlight--underline:hover) {
  background: transparent;
}

/* Spoiler styles */
:root {
  --spoiler-bg: #cbd5e1;
  --spoiler-stripe: rgba(100, 116, 139, 0.55);
  --spoiler-border: rgba(15, 23, 42, 0.18);
  --spoiler-reveal-bg: rgba(226, 232, 240, 0.85);
}

:root[data-display-palette='night'] {
  --spoiler-bg: #3f3f46;
  --spoiler-stripe: rgba(148, 163, 184, 0.35);
  --spoiler-border: rgba(255, 255, 255, 0.18);
  --spoiler-reveal-bg: rgba(71, 85, 105, 0.35);
}

:root[data-custom-theme='true'] {
  --spoiler-bg: color-mix(in srgb, var(--sc-text-primary) 16%, transparent);
  --spoiler-stripe: color-mix(in srgb, var(--sc-text-primary) 35%, transparent);
  --spoiler-border: color-mix(in srgb, var(--sc-text-primary) 25%, transparent);
  --spoiler-reveal-bg: color-mix(in srgb, var(--sc-text-primary) 12%, transparent);
}

.tiptap-spoiler {
  display: inline-block;
  padding: 0 0.2em;
  border-radius: 0.2em;
  border: 1px solid var(--spoiler-border);
  color: transparent;
  background-color: var(--spoiler-bg);
  background-image: repeating-linear-gradient(
    -45deg,
    var(--spoiler-stripe) 0,
    var(--spoiler-stripe) 6px,
    transparent 6px,
    transparent 12px
  );
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease;
}

.tiptap-spoiler.is-revealed {
  color: inherit;
  background-color: var(--spoiler-reveal-bg);
  background-image: none;
}

.tiptap-editor .tiptap-spoiler,
.keyword-rich-content .tiptap-spoiler,
.sticky-note-editor__content .tiptap-spoiler {
  color: inherit;
  background-color: var(--spoiler-reveal-bg);
  background-image: none;
}

/* @ mention option styles */
.at-option-avatar {
  flex-shrink: 0;
}

.at-option-avatar--all {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #ef4444, #f97316);
  color: white;
  font-weight: 600;
  font-size: 12px;
  border-radius: 6px;
}

.at-option-tag {
  display: inline-block;
  font-size: 10px;
  padding: 0 5px;
  border-radius: 3px;
  line-height: 1.5;
  flex-shrink: 0;
}

.at-option-tag--ic {
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}

.at-option-tag--ooc {
  background: rgba(168, 85, 247, 0.15);
  color: #a855f7;
}

.at-option-tag--user {
  background: rgba(148, 163, 184, 0.15);
  color: #64748b;
}

:global([data-display-palette='night']) .at-option-tag--ic,
:global(:root[data-display-palette='night']) .at-option-tag--ic {
  background: rgba(59, 130, 246, 0.25);
  color: #60a5fa;
}

:global([data-display-palette='night']) .at-option-tag--ooc,
:global(:root[data-display-palette='night']) .at-option-tag--ooc {
  background: rgba(168, 85, 247, 0.25);
  color: #c084fc;
}
</style>
