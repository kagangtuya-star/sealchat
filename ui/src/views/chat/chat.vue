<script setup lang="tsx">
import ChatItem from './components/chat-item.vue';
import { computed, ref, watch, onMounted, onBeforeMount, onBeforeUnmount, nextTick, reactive } from 'vue'
import { VirtualList } from 'vue-tiny-virtual-list';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { Event, Message, User, WhisperMeta } from '@satorijs/protocol'
import type { ChannelIdentity, ChannelIdentityFolder, GalleryItem, UserInfo, SChannel } from '@/types'
import { useUserStore } from '@/stores/user';
import { ArrowBarToDown, Plus, Upload, Send, ArrowBackUp, Palette, Download, ArrowsVertical, Star, StarOff, FolderPlus, DotsVertical, Folders, Copy as CopyIcon, Search as SearchIcon } from '@vicons/tabler'
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
import ChatSearchPanel from './components/ChatSearchPanel.vue'
import ArchiveDrawer from './components/archive/ArchiveDrawer.vue'
import ExportDialog from './components/export/ExportDialog.vue'
import ExportManagerModal from './components/export/ExportManagerModal.vue'
import DiceTray from './components/DiceTray.vue'
import IFormPanelHost from '@/components/iform/IFormPanelHost.vue';
import IFormFloatingWindows from '@/components/iform/IFormFloatingWindows.vue';
import IFormDrawer from '@/components/iform/IFormDrawer.vue';
import IFormEmbedInstances from '@/components/iform/IFormEmbedInstances.vue';
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
import { useUtilsStore } from '@/stores/utils';
import { useDisplayStore } from '@/stores/display';
import { contentEscape, contentUnescape, arrayBufferToBase64, base64ToUint8Array } from '@/utils/tools'
import { triggerBlobDownload } from '@/utils/download';
import IconNumber from '@/components/icons/IconNumber.vue'
import IconBuildingBroadcastTower from '@/components/icons/IconBuildingBroadcastTower.vue'
import { computedAsync, useDebounceFn, useEventListener, useWindowSize, useIntersectionObserver } from '@vueuse/core';
import type { UserEmojiModel } from '@/types';
import { useGalleryStore } from '@/stores/gallery';
import { Settings } from '@vicons/ionicons5';
import { dialogAskConfirm } from '@/utils/dialog';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { resolveAttachmentUrl, fetchAttachmentMetaById, normalizeAttachmentId, type AttachmentMeta } from '@/composables/useAttachmentResolver';
import { ensureDefaultDiceExpr, matchDiceExpressions, type DiceMatch } from '@/utils/dice';
import DOMPurify from 'dompurify';
import type { DisplaySettings } from '@/stores/display';
import { useIFormStore } from '@/stores/iform';
import { useWorldGlossaryStore } from '@/stores/worldGlossary';
import { useChannelSearchStore } from '@/stores/channelSearch';
import WorldKeywordManager from '@/views/world/WorldKeywordManager.vue'

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
const iFormStore = useIFormStore();
iFormStore.bootstrap();
const isEditing = computed(() => !!chat.editing);
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
  return role === 'owner' || role === 'admin'
})
const displaySettingsVisible = ref(false);
const compactInlineLayout = computed(() => display.layout === 'compact' && !display.showAvatar);
const scrollButtonColor = computed(() => (display.palette === 'night' ? 'rgba(148, 163, 184, 0.25)' : '#e5e7eb'));
const scrollButtonTextColor = computed(() => (display.palette === 'night' ? 'rgba(248, 250, 252, 0.95)' : '#111827'));
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
onBeforeUnmount(() => {
  chatEvent.off('bot-list-updated', handleBotListUpdated as any);
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
  return '海豹尬聊 SealChat';
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
    ribbonRoleOptions.value = items
      .map((item) => ({
        id: String(item.id || ''),
        label: item.label || '未命名角色',
      }))
      .filter((item) => item.id);
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

const syncActionRibbonState = () => {
  chatEvent.emit('action-ribbon-state', showActionRibbon.value);
};

const handleActionRibbonToggleRequest = () => {
  showActionRibbon.value = !showActionRibbon.value;
};

const handleActionRibbonStateRequest = () => {
  syncActionRibbonState();
};

const handleDisplaySettingsSave = (settings: DisplaySettings) => {
  display.updateSettings(settings);
  displaySettingsVisible.value = false;
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

const emojiLoading = ref(false)
const uploadImages = computedAsync(async () => {
  if (user.emojiCount) {
    const resp = await user.emojiList();
    return resp.data.items;
  }
  return [];
}, [], emojiLoading);

const hasUserEmoji = computed(() => (uploadImages.value?.length ?? 0) > 0);
const galleryEmojiItems = computed<GalleryItem[]>(() => {
  if (!gallery.emojiCollectionId) return [];
  return gallery.getItemsByCollection(gallery.emojiCollectionId);
});
const galleryEmojiName = computed(() => gallery.emojiCollection?.name ?? '');
const hasGalleryEmoji = computed(() => galleryEmojiItems.value.length > 0);

const emojiPopoverShow = ref(false);
const emojiTriggerButtonRef = ref<HTMLElement | null>(null);
const emojiAnchorElement = ref<HTMLElement | null>(null);
const emojiPopoverX = ref<number | null>(null);
const emojiPopoverY = ref<number | null>(null);
const emojiPopoverXCoord = computed(() => emojiPopoverX.value ?? undefined);
const emojiPopoverYCoord = computed(() => emojiPopoverY.value ?? undefined);
const emojiSearchQuery = ref('');
const isManagingEmoji = ref(false);

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

onMounted(() => {
  try {
    const stored = localStorage.getItem(emojiUsageKey);
    if (stored) emojiUsageMap.value = JSON.parse(stored);
  } catch (e) {
    console.warn('Failed to load emoji usage', e);
  }
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

const filteredUserEmojis = computed(() => {
  const query = emojiSearchQuery.value.trim().toLowerCase();
  const items = uploadImages.value || [];
  const filtered = !query ? items : items.filter((item, idx) => {
    const remark = (item.remark && item.remark.trim()) || `收藏${idx + 1}`;
    return remark.toLowerCase().includes(query);
  });
  return sortByUsage(filtered);
});

const filteredGalleryEmojis = computed(() => {
  const query = emojiSearchQuery.value.trim().toLowerCase();
  const filtered = !query ? galleryEmojiItems.value : galleryEmojiItems.value.filter(item =>
    item.remark?.toLowerCase().includes(query)
  );
  return sortByUsage(filtered);
});

const galleryPanelVisible = computed(() => gallery.isPanelVisible);

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n();

// const virtualListRef = ref<InstanceType<typeof VirtualList> | null>(null);
const messagesListRef = ref<HTMLElement | null>(null);
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
  if (typeof navigator === 'undefined' || !navigator.clipboard) {
    message.warning('当前环境不支持复制')
    hideSelectionBar()
    return
  }
  try {
    await navigator.clipboard.writeText(selectionBar.text)
    message.success('已复制选中文本')
  } catch (error) {
    console.warn('复制失败', error)
    message.error('复制失败')
  }
  hideSelectionBar()
}

const handleSelectionAddKeyword = () => {
  const worldId = chat.currentWorldId
  if (!worldId || !selectionBar.text) return
  worldGlossary.setManagerVisible(true)
  worldGlossary.openEditor(worldId, null, selectionBar.text)
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
const wideInputMode = ref(false);
const chatInputClassList = computed(() => (wideInputMode.value ? ['chat-input--expanded'] : []));
const wideInputTooltip = computed(() => (wideInputMode.value ? '退出广域输入模式' : '进入广域输入模式'));
const toggleWideInputMode = () => {
  wideInputMode.value = !wideInputMode.value;
  nextTick(() => textInputRef.value?.focus?.());
};
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
    await gallery.loadCollections(id).catch(() => undefined);
    if (gallery.emojiCollectionId) {
      await gallery.loadItems(gallery.emojiCollectionId).catch(() => undefined);
    }
  },
  { immediate: true }
);

watch(
  () => gallery.emojiCollectionId,
  (collectionId) => {
    if (collectionId) {
      void gallery.loadItems(collectionId);
    }
  }
);

watch(emojiPopoverShow, (show) => {
  if (!show) {
    isManagingEmoji.value = false;
    emojiSearchQuery.value = '';
  } else {
    nextTick(() => {
      syncEmojiPopoverPosition();
    });
    gallery.loadEmojiCollection();
  }
});

watch(isManagingEmoji, (val) => {
  if (val) {
    gallery.loadEmojiCollection();
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
  const allEmojis = [
    ...(uploadImages.value || []).map(item => ({
      remark: item.remark?.trim(),
      attachmentId: item.attachmentId || item.id
    })),
    ...allGalleryItems.value.map(item => ({
      remark: item.remark?.trim(),
      attachmentId: item.attachmentId
    }))
  ].filter(e => e.remark && e.attachmentId);

  const remarkMap = new Map<string, string>();
  allEmojis.forEach(e => {
    if (e.remark) remarkMap.set(e.remark, e.attachmentId);
  });
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
const identitySubmitting = ref(false);
const identityForm = reactive({
  displayName: '',
  color: '',
  avatarAttachmentId: '',
  isDefault: false,
  folderIds: [] as string[],
});
const identityAvatarPreview = ref('');
const identityAvatarInputRef = ref<HTMLInputElement | null>(null);
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

const folderMap = computed<Record<string, ChannelIdentityFolder>>(() => {
  const map: Record<string, ChannelIdentityFolder> = {};
  identityFolders.value.forEach(folder => {
    map[folder.id] = folder;
  });
  return map;
});

const folderSelectOptions = computed(() => identityFolders.value.map(folder => ({ label: folder.name, value: folder.id })));

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
  identityDialogVisible.value = true;
};

const openIdentityEdit = (identity: ChannelIdentity) => {
  editingIdentity.value = identity;
  identityDialogMode.value = 'edit';
  resetIdentityForm(identity);
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
  identityForm.avatarAttachmentId = '';
  identityAvatarFile = file;
  revokeIdentityObjectURL();
  identityAvatarObjectURL = URL.createObjectURL(file);
  identityAvatarPreview.value = identityAvatarObjectURL;
  input.value = '';
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
      await chat.channelIdentityCreate(payload);
      message.success('频道角色已创建');
    } else if (editingIdentity.value) {
      await chat.channelIdentityUpdate(editingIdentity.value.id, payload);
      message.success('频道角色已更新');
    }
    await chat.loadChannelIdentities(chat.curChannel.id, true);
    identityDialogVisible.value = false;
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
  return message?.identity?.displayName
    || message?.sender_member_name
    || message?.member?.nick
    || message?.user?.nick
    || message?.user?.name
    || '未知';
};

const getMessageAvatar = (message: any) => {
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
  const keyword = archivedSearchQuery.value.trim().toLowerCase();
  if (!keyword) {
    return [...archivedMessagesRaw.value];
  }
  return archivedMessagesRaw.value.filter((item) => {
    const fields = [item.content, item.sender?.name, item.archivedBy];
    return fields.some((field) => field?.toLowerCase().includes(keyword));
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

const roleFilterActive = computed(() => Array.isArray(chat.filterState.roleIds) && chat.filterState.roleIds.length > 0);

const visibleRowEntries = computed<VisibleRowEntry[]>(() => {
  const { icOnly, showArchived, roleIds } = chat.filterState;
  const filterRoleIds = Array.isArray(roleIds) ? roleIds : [];
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
    if (icOnly && icValue !== 'ic') {
      return false;
    }

    if (filterRoleIds.length > 0) {
      const roleKey = getMessageRoleKey(message);
      if (!roleKey || !filterRoleIds.includes(roleKey)) {
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
  if (msg.createdAt === undefined && msg.created_at !== undefined) {
    msg.createdAt = msg.created_at;
  }
  if (msg.updatedAt === undefined && msg.updated_at !== undefined) {
    msg.updatedAt = msg.updated_at;
  }
  if (msg.whisperTo === undefined && msg.whisper_to !== undefined) {
    msg.whisperTo = msg.whisper_to;
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
    const resp = await chat.messageListDuring(chat.curChannel.id, from, to, {
      includeArchived: true,
      includeOoc: true,
    });
    const normalized = normalizeMessageList(resp?.data || []);
    if (!normalized.length) {
      return false;
    }
    const containsTarget = normalized.some((msg) => msg.id === payload.messageId);
    if (!containsTarget) {
      return false;
    }
    const targetIndex = normalized.findIndex((msg) => msg.id === payload.messageId);
    const start = Math.max(0, targetIndex - SEARCH_ANCHOR_WINDOW_LIMIT);
    const end = Math.min(normalized.length, targetIndex + SEARCH_ANCHOR_WINDOW_LIMIT + 1);
    const windowMessages = normalized.slice(start, end);
    resetWindowState('history');
    rows.value = windowMessages;
    sortRowsByDisplayOrder();
    applyCursorUpdate({ before: resp?.next ?? '' });
    computeAfterCursorFromRows();
    messageWindow.hasReachedStart = false;
    messageWindow.beforeCursorExhausted = !messageWindow.beforeCursor && from === 0;
    messageWindow.hasReachedLatest = false;
    updateAnchorMessage(payload.messageId);
    showButton.value = true;
    lockHistoryView();
    return true;
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
    return false;
  }
  const center = Number(payload.createdAt);
  if (!Number.isFinite(center)) {
    return false;
  }
  const from = Math.max(0, Math.floor(center - spanMs));
  const to = Math.max(from + 1, Math.floor(center + spanMs));
  try {
    const resp = await chat.messageListDuring(chat.curChannel.id, from, to, {
      includeArchived: true,
      includeOoc: true,
    });
    const incoming = normalizeMessageList(resp?.data || []);
    if (!incoming.length) {
      return false;
    }
    mergeIncomingMessages(incoming, resp ? { before: resp.next ?? '' } : undefined);
    return messageExistsLocally(payload.messageId);
  } catch (error) {
    console.warn('定位消息失败（时间窗口）', error);
    return false;
  }
};

const loadMessagesByCursor = async (payload: { messageId: string; displayOrder?: number; createdAt?: number }) => {
  if (!chat.curChannel?.id || payload.displayOrder === undefined) {
    return false;
  }
  const order = Number(payload.displayOrder);
  if (!Number.isFinite(order)) {
    return false;
  }
  const cursorOrder = order + 1e-6;
  const cursorTime = Math.max(0, Math.floor(Number(payload.createdAt ?? Date.now())));
  const cursor = `${cursorOrder.toFixed(8)}|${cursorTime}|${payload.messageId}`;
  try {
    const resp = await chat.messageList(chat.curChannel.id, cursor, {
      includeArchived: true,
      includeOoc: true,
    });
    const incoming = normalizeMessageList(resp?.data || []);
    if (!incoming.length) {
      return false;
    }
    mergeIncomingMessages(incoming, resp ? { before: resp.next ?? '' } : undefined);
    return messageExistsLocally(payload.messageId);
  } catch (error) {
    console.warn('定位消息失败（游标）', error);
    return false;
  }
};

const locateMessageForJump = async (payload: { messageId: string; displayOrder?: number; createdAt?: number }) => {
  for (const span of SEARCH_JUMP_WINDOWS_MS) {
    const found = await loadMessagesWithinWindow(payload, span);
    if (found) {
      return true;
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
    }
    return located;
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
  await nextTick();
  let target = messageRowRefs.get(targetId);
  if (!target) {
    const loaded = await ensureSearchTargetVisible(payload);
    if (!loaded) {
      return;
    }
    await nextTick();
    target = messageRowRefs.get(targetId);
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
  if (chat.isEditingMessage(item.id || '')) {
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
  const ghost = rowEl.cloneNode(true) as HTMLElement;
  ghost.classList.add('message-row__ghost');
  inheritChatContextClasses(ghost);
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
let typingPreviewOrderSeq = 0;
const getTypingOrderKey = (userId: string, mode: 'typing' | 'editing') => {
  const existing = typingPreviewList.value.find((item) => item.userId === userId && item.mode === mode);
  if (existing) {
    return existing.orderKey;
  }
  return typingPreviewOrderSeq++;
};
const typingPreviewItemClass = (preview: TypingPreviewItem) => [
  'typing-preview-item',
  'message-row',
  `message-row--tone-${preview.tone}`,
  `typing-preview-item--${preview.tone}`,
  { 'typing-preview-item--indicator': preview.indicatorOnly },
];
const typingPreviewSurfaceClass = (preview: TypingPreviewItem) => [
  'typing-preview-surface',
  'message-row__surface',
  `message-row__surface--tone-${preview.tone}`,
];
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
const activeIdentityForPreview = computed(() => chat.getActiveIdentity(chat.curChannel?.id || ''));
const selfPreviewUserId = computed(() => user.info?.id || '__self__');
const typingPreviewItems = computed(() =>
  typingPreviewList.value
    .filter((item) => item.mode === 'typing')
    .slice()
    .sort((a, b) => a.orderKey - b.orderKey),
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
  if (!inputPreviewEnabled.value) {
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
  }
  syncSelfTypingPreview();
});
let lastTypingChannelId = '';
let lastTypingWhisperTargetId: string | null = null;

const upsertTypingPreview = (item: TypingPreviewItem) => {
  const shouldStick = !inHistoryMode.value && visibleRows.value.length === rows.value.length && isNearBottom();
  const isSelfPreview = item.userId === selfPreviewUserId.value;
  const orderKey = isSelfPreview ? Number.MAX_SAFE_INTEGER : getTypingOrderKey(item.userId, item.mode);
  const existingIndex = typingPreviewList.value.findIndex((i) => i.userId === item.userId && i.mode === item.mode);
  if (existingIndex >= 0) {
    typingPreviewList.value.splice(existingIndex, 1, { ...item, orderKey });
  } else {
    typingPreviewList.value.push({ ...item, orderKey });
  }
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
  typingPreviewOrderSeq = 0;
};

const resolveCurrentWhisperTargetId = (): string | null => chat.whisperTarget?.id || null;

const sendTypingUpdate = throttle((state: TypingBroadcastState, content: string, channelId: string, whisperTo?: string | null) => {
  const targetId = whisperTo ?? resolveCurrentWhisperTargetId();
  const icMode = chat.icMode === 'ooc' ? 'ooc' : 'ic';
  const extra: { whisperTo?: string; icMode: 'ic' | 'ooc' } = { icMode };
  if (targetId) {
    extra.whisperTo = targetId;
  }
  lastTypingWhisperTargetId = targetId ?? null;
  chat.messageTyping(state, content, channelId, extra);
}, 400, { leading: true, trailing: true });

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

  const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
  const content = typingPreviewMode.value === 'content' ? truncated : '';
  sendTypingUpdate(typingPreviewMode.value, content, channelId, resolveCurrentWhisperTargetId());
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

interface InputHistoryEntry {
  id: string;
  channelKey: string;
  mode: 'plain' | 'rich';
  content: string;
  createdAt: number;
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
      return { id, channelKey, mode, content, createdAt } as InputHistoryEntry;
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
  const newEntry: InputHistoryEntry = {
    id: nanoid(),
    channelKey,
    mode,
    content,
    createdAt: Date.now(),
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
      return plain;
    }
    return contentUnescape(entry.content).replace(/\s+/g, ' ').trim();
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

const applyHistoryEntry = (entry: InputHistoryEntry, options?: { silent?: boolean }) => {
  try {
    inputMode.value = entry.mode;
    suspendInlineSync = true;
    textToSend.value = entry.content;
    suspendInlineSync = false;
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
  },
);
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
  const metaId = msg?.whisperMeta?.targetUserId;
  if (metaId) {
    return metaId;
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

  // 替换图片标记为 [图片]
  const replaced = value.replace(/\[\[图片:[^\]]+\]\]/g, '[图片]');
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
  if (value.trim().startsWith('{') && value.includes('"type":"doc"')) {
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

  // 处理普通文本和图片标记
  const imageMarkerRegex = /\[\[(?:图片:([^\]]+)|img:id:([^\]]+))\]\]/g;
  let result = '';
  let lastIndex = 0;

  let match;
  while ((match = imageMarkerRegex.exec(value)) !== null) {
    // 添加标记前的文本
    if (match.index > lastIndex) {
      result += renderDicePreviewSegment(value.substring(lastIndex, match.index));
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
  if (lastIndex < value.length) {
    result += renderDicePreviewSegment(value.substring(lastIndex));
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
    const newMsg = await chat.messageCreate(finalContent, replyTo?.id, whisperTargetForSend?.id, clientId, identityIdOverride);
    if (!newMsg) {
      throw new Error('message.create returned empty result');
    }
    for (const [k, v] of Object.entries(newMsg as Record<string, any>)) {
      (tmpMsg as any)[k] = v;
    }
    instantMessages.delete(tmpMsg);
    upsertMessage(tmpMsg);
    resetInlineImages();
    pendingInlineSelection = null;

    if (channelKey) {
      clearAutoRestoreEntry(channelKey);
    }
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

const handleDiceInsert = (expr: string) => {
  insertDiceExpression(expr.trim() ? `${expr.trim()} ` : expr);
  ensureInputFocus();
};

const handleDiceRollNow = (expr: string) => {
  insertDiceExpression(expr.trim());
  send();
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

watch([
  inputPreviewEnabled,
  inputMode,
  inputIcMode,
  () => chat.curChannel?.id,
  () => activeIdentityForPreview.value?.id,
], () => {
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

watch(() => chat.whisperTarget?.id, (targetId, prevId) => {
  if (chat.whisperTarget && targetId) {
    closeWhisperPanel();
    ensureInputFocus();
  }
  if (targetId === prevId) {
    return;
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
      const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
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

const emit = defineEmits(['drawer-show'])

let firstLoad = false;
onMounted(async () => {
  await chat.tryInit();
  await utils.configGet();
  await utils.commandsRefresh();

  chat.channelRefreshSetup()

  refreshHistoryEntries();
  scheduleHistoryAutoRestore();

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
    const shouldStick = !inHistoryMode.value && visibleRows.value.length === rows.value.length && isNearBottom();
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
  });
});

chatEvent.off('channel-presence-updated', '*');
chatEvent.on('channel-presence-updated', (e?: Event) => {
  if (!e?.presence || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  e.presence.forEach((item) => {
    const userId = item?.user?.id;
    if (!userId) {
      return;
    }
    chat.updatePresence(userId, {
      lastPing: item?.lastSeen ?? Date.now(),
      latencyMs: typeof item?.latency === 'number' ? item.latency : Number(item?.latency) || 0,
      isFocused: !!item?.focused,
    });
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
      const messages = await chat.messageListDuring(chat.curChannel?.id || '', lastCreatedAt, now)
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

  chatEvent.on('channel-switch-to', (e) => {
    if (!firstLoad) return;
  stopTypingPreviewNow();
  resetTypingPreview();
  stopEditingPreviewNow();
  chat.cancelEditing();
  textToSend.value = '';
  resetWindowState('live');
  resetDragState();
  localReorderOps.clear();
  showButton.value = false;
    // 具体不知道原因，但是必须在这个位置reset才行
    // virtualListRef.value?.reset();
    refreshHistoryEntries();
    scheduleHistoryAutoRestore();
    fetchLatestMessages();
  })

  await fetchLatestMessages();
  firstLoad = true;
})

onBeforeUnmount(() => {
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  resetTypingPreview();
  cancelDrag();
  stopTopObserver();
  stopBottomObserver();
});

const showButton = ref(false);
const historyHintVisible = computed(() => inHistoryMode.value || historyLocked.value);
const historyHintLabel = computed(() => (isMobileUa ? '历史' : '当前浏览历史消息'));

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
  if (container.scrollTop <= 80 && firstLoad && !messageWindow.loadingBefore) {
    void loadOlderMessages();
  }
};

const pauseKeydown = ref(false);

const handleMentionSelect = () => {
  pauseKeydown.value = false;
};

const keyDown = function (e: KeyboardEvent) {
  if (pauseKeydown.value) return;

  if (!isEditing.value && handleWhisperKeydown(e)) {
    return;
  }

  // 移动端不触发桌面快捷键
  if (isMobileUa) {
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

const reachTop = throttle(async () => {
  await loadOlderMessages();
}, 800);

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

const sendEmoji = throttle(async (i: UserEmojiModel) => {
  if (spectatorInputDisabled.value) {
    message.warning('旁观者仅可查看频道内容，无法发送消息');
    return;
  }
  if (await sendImageMessage(i.attachmentId)) {
    recordEmojiUsage(i.id);
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

const selectedEmojiIds = ref<string[]>([]);
const emojiRemarkModalVisible = ref(false);
const emojiRemarkInput = ref('');
const emojiRemarkSaving = ref(false);
const editingEmoji = ref<UserEmojiModel | null>(null);
const emojiRemarkPattern = /^[\p{L}\p{N}_]{1,64}$/u;

const resolveEmojiRemark = (item: UserEmojiModel, idx: number) => (item.remark?.trim() || `收藏${idx + 1}`);

const openEmojiRemarkEditor = (item: UserEmojiModel) => {
  editingEmoji.value = item;
  emojiRemarkInput.value = item.remark?.trim() || '';
  emojiRemarkModalVisible.value = true;
};

const submitEmojiRemark = async () => {
  if (!editingEmoji.value) {
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
    await user.emojiUpdate(editingEmoji.value.id, { remark });
    editingEmoji.value.remark = remark;
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
  try {
    await user.emojiDelete(selectedEmojiIds.value);
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

const getGalleryItemThumb = (item: GalleryItem) => item.thumbUrl || resolveAttachmentUrl(item.attachmentId);

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
  chatEvent.off('channel-identity-open', handleIdentityMenuOpen);
  chatEvent.off('channel-identity-updated', handleIdentityUpdated);
  chatEvent.off('action-ribbon-toggle', handleActionRibbonToggleRequest);
  chatEvent.off('action-ribbon-state-request', handleActionRibbonStateRequest);
  revokeIdentityObjectURL();
  searchHighlightTimers.forEach((timer) => window.clearTimeout(timer));
  searchHighlightTimers.clear();
  if (isMobileUa) {
    markDiceTrayMobileWrapper(false);
  }
});
</script>

<template>
  <div class="flex flex-col h-full justify-between">
    <!-- 功能面板 -->
    <transition name="slide-down">
      <ChatActionRibbon
        v-if="showActionRibbon"
        :filters="chat.filterState"
        :roles="ribbonRoleOptions"
        :archive-active="archiveDrawerVisible"
        :export-active="exportManagerVisible"
        :identity-active="identityDialogVisible"
        :gallery-active="galleryPanelVisible"
        :display-active="displaySettingsVisible"
        :favorite-active="display.favoriteBarEnabled"
        @update:filters="chat.setFilterState($event)"
        @open-archive="archiveDrawerVisible = true"
        @open-export="exportManagerVisible = true"
        @open-identity-manager="openIdentityManager"
        @open-gallery="openGalleryPanel"
        @open-display-settings="displaySettingsVisible = true"
        @open-favorites="channelFavoritesVisible = true"
        @clear-filters="chat.setFilterState({ icOnly: false, showArchived: false, roleIds: [] })"
      />
    </transition>

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
      :class="[`chat--layout-${display.layout}`, `chat--palette-${display.palette}`, { 'chat--no-avatar': !display.showAvatar }]"
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

      <div class="typing-preview-viewport" v-if="typingPreviewItems.length">
        <div
          v-for="preview in typingPreviewItems"
          :key="`${preview.userId}-typing`"
          :class="typingPreviewItemClass(preview)"
        >
          <div :class="typingPreviewSurfaceClass(preview)" :data-tone="preview.tone">
            <div
              v-if="shouldShowTypingHandle(preview)"
              class="message-row__handle message-row__handle--placeholder"
              aria-hidden="true"
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
                  <AvatarVue :border="false" :size="48" :src="preview.avatar" />
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
    <div class="edit-area flex justify-between relative">
      <div class="history-floating space-y-3 flex flex-col items-end">
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
      <div class="channel-switch-trigger px-4 py-2" v-if="utils.isSmallPage">
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

      <div class="reply-banner absolute rounded px-4 py-2" style="top: -4rem; right: 1rem" v-if="chat.curReplyTo">
        正在回复: {{ chat.curReplyTo.member?.nick }}
        <n-button @click="chat.curReplyTo = null">取消</n-button>
      </div>

      <div
        class="chat-input-container flex flex-col w-full relative"
        :class="{ 'chat-input-container--spectator-hidden': spectatorInputDisabled }"
      >
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
            <div class="chat-input-actions input-floating-toolbar flex flex-1 items-center justify-between gap-2">
              <div class="chat-input-actions__group chat-input-actions__group--leading">
                <div class="chat-input-actions__cell identity-switcher-cell">
                  <ChannelIdentitySwitcher
                    v-if="chat.curChannel"
                    @create="openIdentityCreate"
                    @manage="openIdentityManager"
                    @identity-changed="emitTypingPreview"
                  />
                </div>
                <div class="chat-input-actions__cell">
                  <div class="emoji-trigger">
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

                    <n-popover
                      v-model:show="emojiPopoverShow"
                      trigger="click"
                      placement="bottom-start"
                      :x="emojiPopoverXCoord"
                      :y="emojiPopoverYCoord"
                    >
                      <div class="emoji-panel">
                        <div class="emoji-panel__header">
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

                        <div v-if="hasGalleryEmoji && !isManagingEmoji" class="emoji-panel__search">
                          <n-input
                            v-model:value="emojiSearchQuery"
                            size="small"
                            placeholder="搜索表情..."
                            clearable
                          />
                        </div>

                        <div v-if="!hasUserEmoji && !hasGalleryEmoji" class="emoji-panel__empty">
                          当前没有收藏的表情，可以在聊天窗口的图片上<b class="px-1">长按</b>或<b class="px-1">右键</b>添加
                        </div>

                        <div v-else class="emoji-panel__content">
                        <template v-if="true">
                          <template v-if="hasUserEmoji && !emojiSearchQuery">
                            <template v-if="isManagingEmoji">
                              <n-checkbox-group v-model:value="selectedEmojiIds">
                                <div class="emoji-grid">
                                  <div class="emoji-manage-item" v-for="(item, idx) in uploadImages" :key="item.id">
                                    <div class="emoji-manage-item__content">
                                      <n-checkbox :value="item.id">
                                        <div class="emoji-item">
                                          <img :src="getSrc(item)" alt="表情" />
                                          <div class="emoji-caption" :title="resolveEmojiRemark(item, idx)">
                                            {{ resolveEmojiRemark(item, idx) }}
                                          </div>
                                        </div>
                                      </n-checkbox>
                                      <n-button text size="tiny" @click.stop="openEmojiRemarkEditor(item)">编辑备注</n-button>
                                    </div>
                                  </div>
                                </div>
                              </n-checkbox-group>

                              <div class="emoji-panel__actions">
                                <n-button type="info" size="small" @click="emojiSelectedDelete" :disabled="selectedEmojiIds.length === 0">
                                  删除选中
                                </n-button>
                                <n-button type="default" size="small" @click="exitEmojiManage">
                                  退出管理
                                </n-button>
                              </div>
                            </template>
                            <template v-else>
                              <div class="emoji-grid">
                                <div class="emoji-item" v-for="(item, idx) in filteredUserEmojis" :key="item.id" @click="sendEmoji(item)">
                                  <img :src="getSrc(item)" alt="表情" />
                                  <div class="emoji-caption" :title="resolveEmojiRemark(item, idx)">{{ resolveEmojiRemark(item, idx) }}</div>
                                  <div class="emoji-item__actions">
                                    <n-button text size="tiny" @click.stop="openEmojiRemarkEditor(item)">备注</n-button>
                                  </div>
                                </div>
                              </div>
                            </template>
                          </template>

                          <template v-if="!isManagingEmoji && (hasGalleryEmoji || emojiSearchQuery)">
                            <div class="emoji-section__title">联动分类：{{ galleryEmojiName || '未命名分类' }}</div>
                            <div v-if="filteredGalleryEmojis.length === 0" class="emoji-panel__empty">
                              没有匹配的表情
                            </div>
                            <div v-else class="emoji-grid">
                              <div
                                class="emoji-item"
                                v-for="item in filteredGalleryEmojis"
                                :key="item.id"
                                draggable="true"
                                @dragstart="handleGalleryEmojiDragStart(item, $event)"
                                @click="handleGalleryEmojiClick(item)"
                              >
                                <img :src="getGalleryItemThumb(item)" alt="表情" />
                                <div class="emoji-caption">{{ item.remark || '未命名表情' }}</div>
                              </div>
                            </div>
                          </template>
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
                     <n-button quaternary circle class="whisper-toggle-button" :class="{ 'whisper-toggle-button--active': whisperMode }"
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
                      @insert="handleDiceInsert"
                      @roll="handleDiceRollNow"
                      @update-default="handleDiceDefaultUpdate"
                    >
                      <template v-if="canManageChannelFeatures" #header-actions>
                        <n-popover trigger="manual" placement="bottom-end" :show="diceSettingsVisible" @clickoutside="diceSettingsVisible = false">
                          <template #trigger>
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
                    </DiceTray>
                  </n-popover>
                </div>
              </div>
            </div>
            <div class="chat-input-editor-row">
              <div class="chat-input-editor-main">
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
                <n-button type="primary" circle size="medium" @click="send"
                  :disabled="spectatorInputDisabled || chat.connectState !== 'connected' || isEditing">
                  <template #icon>
                    <n-icon :component="Send" size="18" />
                  </template>
                </n-button>
              </div>
            </div>
        </div>
      </div>
    </div>
  </div>

  <RightClickMenu />
  <AvatarClickMenu />
  <GalleryPanel @insert="handleGalleryInsert" />
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
  <n-drawer
    class="identity-manage-shell"
    v-model:show="identityManageVisible"
    placement="right"
    :width="identityDrawerWidth"
  >
    <n-drawer-content :class="['identity-manage-drawer', { 'identity-manage-drawer--night': isNightPalette }]">
      <template #header>
        <div class="identity-drawer__header">
          <div>
            <div class="identity-drawer__title">频道角色管理</div>
            <div class="identity-drawer__subtitle">支持导入/导出，便于跨频道迁移</div>
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
  <IFormFloatingWindows />
  <IFormDrawer />

  <DisplaySettingsModal
    v-model:visible="displaySettingsVisible"
    :settings="display.settings"
    @save="handleDisplaySettingsSave"
  />

  <ChannelFavoriteManager v-model:show="channelFavoritesVisible" />
  <WorldKeywordManager />
</template>

<style lang="scss" scoped>
.message-row {
  position: relative;
}

.message-row + .message-row {
  margin-top: var(--chat-bubble-gap, 0.85rem);
}

.chat--layout-bubble .message-row + .message-row {
  margin-top: calc(var(--chat-bubble-gap, 0.85rem) * 0.8);
}

.chat--layout-compact .message-row + .message-row {
  margin-top: var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.85rem) * 0.35));
}

.message-row--tone-ic,
.message-row--tone-ooc {
  margin: 0;
  padding: 0;
  border: none;
}

.message-row + .message-row--tone-ic,
.message-row + .message-row--tone-ooc {
  margin-top: 0;
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

.message-row--self.message-row--tone-ic:not(:first-child),
.message-row--self.message-row--tone-ooc:not(:first-child) {
  margin-top: 0;
}

.chat--layout-compact .message-row--self.message-row--tone-ic:not(:first-child),
.chat--layout-compact .message-row--self.message-row--tone-ooc:not(:first-child) {
  margin-top: 0;
}

.message-row--tone-ic:not(:first-child),
.message-row--tone-ooc:not(:first-child) {
  margin-top: 0;
}

.chat--layout-compact .message-row--tone-ic:not(:first-child),
.chat--layout-compact .message-row--tone-ooc:not(:first-child) {
  margin-top: 0;
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
  background-color: #000000;
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
  background-color: var(--sc-bg-surface);
  border: 1px solid var(--sc-border-strong);
  border-radius: 1rem;
  box-shadow: 0 18px 36px rgba(15, 23, 42, 0.08);
  transition: background-color 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
  scrollbar-color: var(--sc-border-mute) transparent;
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
  width: 8px;
}

.chat::-webkit-scrollbar-track {
  background: transparent;
}

.chat::-webkit-scrollbar-thumb {
  background-color: var(--sc-border-mute);
  border-radius: 999px;
}

:global(.chat.chat--palette-night) {
  scrollbar-color: #9f9f9f transparent;
}

:global(.chat.chat--palette-night::-webkit-scrollbar-thumb) {
  background-color: #9f9f9f;
}

.chat--palette-day {
  --chat-ic-bg: #FBFDF7;
  --chat-ooc-bg: #FFFFFF;
  --chat-preview-dot-ic: rgba(120, 130, 120, 0.35);
  --chat-preview-dot-ooc: rgba(148, 163, 184, 0.35);
}

.chat--palette-night {
  --chat-ic-bg: #3F3F46;
  --chat-ooc-bg: #000000;
  --chat-preview-dot-ic: rgba(255, 255, 255, 0.25);
  --chat-preview-dot-ooc: rgba(255, 255, 255, 0.35);
}

.chat--layout-compact {
  background-color: var(--chat-stage-bg);
  transition: background-color 0.25s ease;
}

.chat.chat--layout-compact.chat--no-avatar .message-row__surface {
  padding: 0.1rem 0.35rem;
}

.chat.chat--layout-compact {
  overflow-x: hidden;
}

.chat--layout-compact .message-row {
  width: 100%;
  padding: 0;
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
  background-color: var(--chat-ic-bg);
}

.chat--layout-compact .message-row__surface--tone-ooc {
  background-color: var(--chat-ooc-bg);
}

.chat--layout-compact .message-row__surface--tone-archived {
  background-color: rgba(148, 163, 184, 0.2);
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
  --typing-preview-bg: var(--chat-ic-bg);
  --typing-preview-dot: var(--chat-preview-dot-ic);
  background-color: var(--typing-preview-bg);
  background-image: radial-gradient(var(--typing-preview-dot) 1px, transparent 1px);
  background-size: 10px 10px;
}

.chat--layout-compact .typing-preview-surface[data-tone='ooc'],
.chat--layout-compact .typing-preview-item--ooc .typing-preview-surface {
  --typing-preview-bg: var(--chat-ooc-bg);
  --typing-preview-dot: var(--chat-preview-dot-ooc);
}

.chat--layout-compact .typing-preview-surface[data-tone='ic'],
.chat--layout-compact .typing-preview-item--ic .typing-preview-surface {
  --typing-preview-bg: var(--chat-ic-bg);
  --typing-preview-dot: var(--chat-preview-dot-ic);
}

.identity-drawer__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding-right: 0.25rem;
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
  width: 3rem;
  height: 3rem;
  min-width: 3rem;
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

.chat--layout-compact.chat--palette-day:not(.chat--no-avatar) .typing-preview-surface,
.chat--layout-compact.chat--palette-day:not(.chat--no-avatar) .typing-preview-bubble,
.chat--layout-compact.chat--palette-day:not(.chat--no-avatar) .typing-preview-bubble__body {
  border-color: transparent !important;
  box-shadow: none;
}

.chat--layout-bubble .typing-preview-bubble {
  padding: 0.5rem 0.75rem;
  border-radius: var(--chat-message-radius, 0.85rem);
  background-color: var(--chat-preview-bg, #f6f7fb);
}

.typing-preview-bubble[data-tone='ic'] {
  background-color: #fbfdf7;
  border-color: rgba(15, 23, 42, 0.14);
}

.typing-preview-bubble[data-tone='ooc'] {
  background-color: #ffffff;
  border-color: rgba(15, 23, 42, 0.12);
}

:root[data-display-palette='night'] .typing-preview-bubble[data-tone='ic'] {
  background-color: #3f3f45;
  border-color: rgba(255, 255, 255, 0.16);
  color: #f4f4f5;
}

:root[data-display-palette='night'] .typing-preview-bubble[data-tone='ooc'] {
  background-color: #000000;
  border-color: rgba(255, 255, 255, 0.24);
  color: #f5f3ff;
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

.reply-banner {
  background-color: var(--sc-chip-bg);
  color: var(--sc-text-primary);
  border: 1px solid var(--sc-border-mute);
}

.scroll-bottom-button {
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.18);
}

:root[data-display-palette='night'] .scroll-bottom-button {
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.65);
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

.chat-input-container {
  width: 100%;
  background-color: transparent;
  border: none;
  border-radius: 0;
  padding: 0;
  margin: 0;
  box-shadow: none;
  transition: background-color 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
}

.chat-input-container--spectator-hidden {
  display: none;
}

:root[data-display-palette='night'] .chat-input-container {
  box-shadow: none;
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
}

.chat-input-editor-main :deep(.hybrid-input) {
  width: 100%;
}

.chat-input-send-inline {
  flex: 0 0 auto;
  display: flex;
  align-items: flex-end;
}

.chat-input-send-inline .n-button {
  width: 44px;
  height: 44px;
  flex-shrink: 0;
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
}

.history-entry {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  width: 100%;
  text-align: left;
  border: 1px solid rgba(148, 163, 184, 0.25);
  border-radius: 0.75rem;
  padding: 0.65rem 0.75rem;
  background: rgba(248, 250, 252, 0.9);
  transition: border-color 0.2s ease, background-color 0.2s ease, box-shadow 0.2s ease;
}

.history-entry:hover {
  border-color: rgba(59, 130, 246, 0.35);
  background: rgba(239, 246, 255, 0.92);
  box-shadow: 0 6px 16px rgba(59, 130, 246, 0.18);
}

.history-entry__meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: #6b7280;
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
  color: #1f2937;
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
  width: 320px;
  max-height: 400px;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
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

.emoji-panel__title {
  font-weight: 600;
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
  grid-template-columns: repeat(4, minmax(70px, 1fr));
  gap: 0.75rem;
}

@media (max-width: 768px) {
  .emoji-grid {
    grid-template-columns: repeat(3, minmax(60px, 1fr));
    gap: 0.5rem;
  }
}

.emoji-item {
  display: flex;
  flex-direction: column;
  touch-action: manipulation;
  align-items: center;
  gap: 0.4rem;
  cursor: pointer;
  border-radius: 8px;
  padding: 0.25rem;
  transition: background-color 0.15s ease;
}

.emoji-item img {
  width: 4.8rem;
  height: 4.8rem;
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
  gap: 0.35rem;
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
</style>
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
  border-style: dashed;
  background: rgba(148, 163, 184, 0.25);
  color: #475569;
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
  background: #fcfcfc;
  border-color: rgba(15, 23, 42, 0.12);
  color: #1f2937;
}

.dice-chip--tone-archived:not(.dice-chip--preview),
[data-dice-tone='archived']:not(.dice-chip--preview) {
  background: rgba(148, 163, 184, 0.2);
  border-color: rgba(148, 163, 184, 0.4);
  color: #334155;
}

:global([data-display-palette='night']) .dice-chip {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(148, 163, 184, 0.35);
  color: #f3f4f6;
}

:global([data-display-palette='night']) .dice-chip--preview {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.35);
  color: #f8fafc;
}

:global([data-display-palette='night']) .dice-chip--error {
  background: rgba(127, 29, 29, 0.7);
  border-color: rgba(248, 113, 113, 0.75);
  color: #fecaca;
}

:global([data-display-palette='night']) .dice-chip--tone-ic:not(.dice-chip--preview),
:global([data-display-palette='night']) [data-dice-tone='ic']:not(.dice-chip--preview) {
  background: #333135;
  border-color: rgba(255, 255, 255, 0.18);
  color: #f4f4f5;
}

:global([data-display-palette='night']) .dice-chip--tone-ooc:not(.dice-chip--preview),
:global([data-display-palette='night']) [data-dice-tone='ooc']:not(.dice-chip--preview) {
  background: #2a282a;
  border-color: rgba(255, 255, 255, 0.15);
  color: #f5f3ff;
}

:global([data-display-palette='night']) .dice-chip--tone-archived:not(.dice-chip--preview),
:global([data-display-palette='night']) [data-dice-tone='archived']:not(.dice-chip--preview) {
  background: rgba(51, 65, 85, 0.65);
  border-color: rgba(148, 163, 184, 0.4);
  color: #e2e8f0;
}
