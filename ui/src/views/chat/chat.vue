<script setup lang="tsx">
import ChatItem from './components/chat-item.vue';
import MultiSelectFloatingBar from './components/MultiSelectFloatingBar.vue';
import MessageForwardDialog from './components/MessageForwardDialog.vue';
import { VirtualList } from 'vue-tiny-virtual-list';
import { chatEvent, useChatStore, type PendingMessageJump } from '@/stores/chat';
import type { Event, Message, User } from '@satorijs/protocol'
import type { AvatarDecoration, ChannelIdentity, ChannelIdentityFolder, ChannelIdentityManageCandidate, ChannelIdentityVariant, GalleryItem, UserInfo, SChannel, WhisperMeta } from '@/types'
import { useUserStore } from '@/stores/user';
import { ArrowBarToDown, Plus, Upload, Send, ArrowBackUp, MessagePlus, Palette, Download, ArrowsVertical, Star, StarOff, FolderPlus, DotsVertical, Folders, Copy as CopyIcon, Search as SearchIcon, Check, X, ChevronDown, ChevronRight, MoodSmile as EmojiTriggerIcon } from '@vicons/tabler'
import { NIcon, c } from 'naive-ui';
import VueScrollTo from 'vue-scrollto'
import ChatInputSwitcher from './components/ChatInputSwitcher.vue'
import ChannelIdentitySwitcher from './components/ChannelIdentitySwitcher.vue'
import GalleryButton from '@/components/gallery/GalleryButton.vue'
import GalleryPanel from '@/components/gallery/GalleryPanel.vue'
import ChatIcOocToggle from './components/ChatIcOocToggle.vue'
import ChatActionRibbon from './components/ChatActionRibbon.vue'
import ChatAiPolishDock from './components/ChatAiPolishDock.vue'
import ChannelFavoriteBar from './components/ChannelFavoriteBar.vue'
import ChannelFavoriteManager from './components/ChannelFavoriteManager.vue'
import ChannelRemarkManager from './components/ChannelRemarkManager.vue'
import DisplaySettingsModal from './components/DisplaySettingsModal.vue'
import IcOocRoleConfigPanel from './components/IcOocRoleConfigPanel.vue'
import ChatSearchPanel from './components/ChatSearchPanel.vue'
import ArchiveDrawer from './components/archive/ArchiveDrawer.vue'
import ExportDialog from './components/export/ExportDialog.vue'
import ExportManagerModal from './components/export/ExportManagerModal.vue'
import BattleReportDrawer from './components/BattleReportDrawer.vue'
import ChatImportDialog from './components/ChatImportDialog.vue'
import ChatImportProgress from './components/ChatImportProgress.vue'
import ChannelImageViewerDrawer from './components/ChannelImageViewerDrawer.vue'
import DiceTrayFloatingWindow from './components/DiceTrayFloatingWindow.vue'
import ChatDiceModeControl from './components/ChatDiceModeControl.vue'
import { getDiceModeLabel, shouldShowDiceTrayTrigger } from './diceMode'
import IFormPanelHost from '@/components/iform/IFormPanelHost.vue';
import IFormFloatingWindows from '@/components/iform/IFormFloatingWindows.vue';
import IFormDrawer from '@/components/iform/IFormDrawer.vue';
import IFormEmbedInstances from '@/components/iform/IFormEmbedInstances.vue';
import StickyNoteManager from './components/StickyNoteManager.vue';
import DiceOverlayLoader from '@/features/dice3d/components/DiceOverlayLoader.vue';
import DiceSettingsDrawer from '@/features/dice3d/components/DiceSettingsDrawer.vue';
import DiceDock from '@/features/dice3d/components/DiceDock.vue';
import { loadDice3DSettings, saveDice3DProfile } from '@/features/dice3d/api';
import { dice3dRuntime } from '@/features/dice3d/runtime';
import { resolveDice3DPlaybackPayload } from '@/features/dice3d/playbackProfile';
import type { Dice3DMemberProfile, Dice3DWorldConfig, DiceVisualPayload } from '@/types';
import CharacterSheetManager from './components/character-sheet/CharacterSheetManager.vue';
import { useStickyNoteStore } from '@/stores/stickyNote';
import { useAudioStudioStore } from '@/stores/audioStudio';
import { usePushNotificationStore } from '@/stores/pushNotification';
import {
  buildIcOocSplitScopeWorldId,
  readSplitSessionSnapshot,
  resolveIcOocSplitSessionSnapshot,
  writeSplitSessionSnapshot,
} from '@/utils/splitSessionStorage';
import { uploadImageAttachment } from './composables/useAttachmentUploader';
import { api, urlBase } from '@/stores/_config';
import { liveQuery } from "dexie";
import { useObservable } from "@vueuse/rxjs";
import { db, getSrc, type Thumb } from '@/models';
import { throttle } from 'lodash-es';
import AvatarVue from '@/components/avatar.vue';
import RightClickMenu from './components/ChatRightClickMenu.vue'
import AvatarClickMenu from './components/AvatarClickMenu.vue'
import { nanoid } from 'nanoid';
import { DEFAULT_PAGE_TITLE, useUtilsStore } from '@/stores/utils';
import { useDisplayStore } from '@/stores/display';
import { normalizeMessageIcMode, resolveAvatarRenderState } from '@/stores/displayAvatarVisibility';
import { useCharacterRemarkStore } from '@/stores/characterRemark';
import { contentEscape, contentUnescape, arrayBufferToBase64, base64ToUint8Array } from '@/utils/tools'
import { triggerBlobDownload } from '@/utils/download';
import { copyTextWithFallback } from '@/utils/clipboard';
import IconBuildingBroadcastTower from '@/components/icons/IconBuildingBroadcastTower.vue'
import { computedAsync, useDebounceFn, useEventListener, useWindowSize, useIntersectionObserver } from '@vueuse/core';
import { useGalleryStore } from '@/stores/gallery';
import { Settings, Close as CloseIcon, EyeOutline, EyeOffOutline } from '@vicons/ionicons5';
import { dialogAskConfirm } from '@/utils/dialog';
import {
  clearTheaterAppearanceEditIntent,
  consumeTheaterAppearanceEditIntent,
  writeTheaterAppearanceEditIntent,
} from '@/utils/theaterAppearanceEditIntent';
import { useI18n } from 'vue-i18n';
import { useAIStore } from '@/stores/ai';
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { resolveAttachmentUrl, fetchAttachmentMetaById, fetchAttachmentFileById, normalizeAttachmentId } from '@/composables/useAttachmentResolver';
import { ensureDefaultDiceExpr, matchDiceExpressions, parseMultiDiceExpression, type DiceMatch } from '@/utils/dice';
import { recordDiceHistory } from '@/views/chat/composables/useDiceHistory';
import DOMPurify from 'dompurify';
import type { DisplaySettings, ToolbarHotkeyKey } from '@/stores/display';
import { INPUT_AREA_HEIGHT_LIMITS } from '@/stores/display';
import { renderQuickFormatHtmlFromEscaped, restoreQuickFormatTextFromHtml, serializePlainTextFromDomNode } from '@/utils/plainQuickFormat';
import { isSmartLinkNode, smartLinkToPlainText } from '@/utils/tiptapSmartLink';
import { isBotCommandLikeContent, renderBotCommandTextAsHtml } from '@/utils/botCommand';
import { shouldAttemptCharacterApiReconnectBeforeBotCommand } from '@/utils/characterApiReconnectGuard';
import { buildOptimisticMessageIcModeFields } from '@/utils/optimisticMessageIcMode';
import { normalizePunctuationForMessageSend } from '@/utils/punctuationNormalizer';
import { buildGeneratedAvatarFile } from '@/utils/generatedAvatarImage';
import { extractPushNotificationPreviewText } from '@/utils/pushNotificationPreview';
import { useIFormStore } from '@/stores/iform';
import { useWorldGlossaryStore } from '@/stores/worldGlossary';
import { useChannelSearchStore, type ChannelSearchResult } from '@/stores/channelSearch';
import { useChannelImagesStore } from '@/stores/channelImages';
import { useChannelImageLayoutStore } from '@/stores/channelImageLayout';
import { useOnboardingStore } from '@/stores/onboarding';
import WorldKeywordManager from '@/views/world/WorldKeywordManager.vue'
import OnboardingRoot from '@/components/onboarding/OnboardingRoot.vue'
import AvatarSetupPrompt from '@/components/AvatarSetupPrompt.vue'
import AvatarEditor from '@/components/AvatarEditor.vue'
import AvatarDecorationEditor from '@/components/avatar-decoration/AvatarDecorationEditor.vue'
import TheaterPresentationEditorModal from '@/components/theater-presentation/TheaterPresentationEditorModal.vue'
import UserAvatarDecoration from '@/components/user-avatar-decoration.vue'
import {
  applyWorldTheaterPresentationTemplate,
  createDefaultTheaterPresentation,
  mergeWorldTheaterPresentationTemplate,
  resolveTheaterPresentation,
  type TheaterPresentation,
  type TheaterPresentationPatch,
  type WorldTheaterPresentationTemplate,
  type WorldTheaterPresentationTemplateSection,
} from '@/types/theaterPresentation'
import { normalizeAvatarDecorations, firstAvatarDecoration } from '@/utils/avatarDecorations'
import {
  cloneChannelIdentityTheaterPresentation,
  cloneChannelIdentityTheaterPresentationPatch,
  resolveChannelIdentityVariantTheaterPatch,
} from '@/utils/channelIdentityTheaterPresentation'
import {
  buildIdentityAssetKey,
  normalizeIdentityExportFileForImport,
  resolveIdentityExportVariantTheaterPresentation,
  remapDecorationsForImport,
  resolveIdentityMatchByName,
  resolveIdentityAssetFetchUrl,
  resolveIdentityAssetTransferUrl,
  shouldUseIdentityAssetRemoteImport,
  shouldIgnoreIdentityAssetFetchStatus,
  type IdentityAssetPayload,
  type IdentityAvatarPayload,
  type IdentityExportDecorationItem,
  type IdentityExportFile,
  type IdentityExportFolder,
  type IdentityExportItem,
  type IdentityExportVariantItem,
} from '@/utils/channelIdentityMigration'
import AnnouncementManagerModal from '@/components/announcement/AnnouncementManagerModal.vue';
import { isHotkeyMatchingEvent } from '@/utils/hotkey';
import { useRoute, useRouter } from 'vue-router';
import WebhookIntegrationManager from '@/views/split/components/WebhookIntegrationManager.vue';
import EmailNotificationManager from '@/views/split/components/EmailNotificationManager.vue';
import BridgeStatusPanel from './components/BridgeStatusPanel.vue';
import CharacterCardPanel from './components/CharacterCardPanel.vue';
import { characterApiUnsupportedText, useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useChannelCharacterSnapshotStore } from '@/stores/channelCharacterSnapshot';
import KeywordSuggestPanel from '@/components/chat/KeywordSuggestPanel.vue';
import MessageImageEditor from '@/components/chat/MessageImageEditor.vue';
import { ensurePinyinLoaded, matchKeywords, matchText, type KeywordMatchResult } from '@/utils/pinyinMatch';
import { generateIFormEmbedLink } from '@/utils/iformEmbedLink';
import { buildMessageCursor } from '@/utils/messageCursor';
import { buildRoleSnapshot } from '@/bridge/sealchatBridgeSerializer';
import type { BridgeRoleSnapshot } from '@/bridge/sealchatBridgeProtocol';
import { resolveDeletedChannelFallbackId } from '@/stores/chatChannelSelection';
import {
  buildInputHistorySignature,
  captureWhisperSnapshot,
  normalizeWhisperSnapshot,
  restoreWhisperSnapshot,
  type WhisperSnapshot,
} from './inputHistoryWhisperState';
import { buildEditMessageUpdateOptions } from './editMessageUpdate';
import { shouldMergeNeighborMessages } from './messageMerge';
import { resolveInterjectTargetMode, shouldAllowInterject } from './interjectFlow';
import { useMentionSuggestions } from './composables/useMentionSuggestions';
import { useChatAIPolish } from './composables/useChatAIPolish';
import { useMessageSelection } from './composables/useMessageSelection';
import { useTypingPreview, type EditingPreviewInfo, type TypingPreviewItem } from './composables/useTypingPreview';
import { useChatEmoji } from './composables/useChatEmoji';
import {
  hasTheaterComposerDraft,
  shouldResolveTheaterIdentityShortcut,
  validateTheaterCharacter,
  validateTheaterVariant,
} from './theater-chat-guards';

const EmojiPickerModal = defineAsyncComponent(() => import('./components/EmojiPickerModal.vue'));

// const uploadImages = useObservable<Thumb[]>(
//   liveQuery(() => db.thumbs.toArray()) as any
// )

const chat = useChatStore();
const user = useUserStore();
const gallery = useGalleryStore();
const utils = useUtilsStore();
const display = useDisplayStore();
const characterRemarkStore = useCharacterRemarkStore();
const worldGlossary = useWorldGlossaryStore();
const channelSearch = useChannelSearchStore();
const channelImages = useChannelImagesStore();
const aiStore = useAIStore();
const channelImageLayout = useChannelImageLayoutStore();
const onboarding = useOnboardingStore();
const iFormStore = useIFormStore();
const stickyNoteStore = useStickyNoteStore();
const dice3dSettingsVisible = ref(false);
const dice3dConfig = ref<Dice3DWorldConfig | null>(null);
const dice3dProfile = ref<Dice3DMemberProfile | null>(null);
const characterCardStore = useCharacterCardStore();
const characterSheetStore = useCharacterSheetStore();
const channelCharacterSnapshotStore = useChannelCharacterSnapshotStore();
iFormStore.bootstrap();
const router = useRouter();
const route = useRoute();
const pushStore = usePushNotificationStore();
const isEditing = computed(() => !!chat.editing);
const isEditingCurrentChannel = computed(() => {
  const channelId = String(chat.curChannel?.id || '').trim();
  return Boolean(chat.editing && channelId && chat.editing.channelId === channelId);
});

const isEmbedMode = computed(() => route.path === '/embed');
const isTheaterEmbedMode = computed(() => isEmbedMode.value && route.query.mode === 'theater');
const splitEntryEnabled = computed(() => route.path !== '/embed');
const routeWorldId = computed(() => typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '');
const theaterEntryEnabled = computed(() => {
  if (['/embed', '/split', '/theater'].includes(route.path)) return false;
  const worldId = routeWorldId.value || String(chat.currentWorldId || '').trim();
  const channelWorldId = String(chat.curChannel?.worldId || '').trim();
  return !!worldId
    && !!chat.curChannel?.id
    && String(chat.currentWorldId || '').trim() === worldId
    && (!channelWorldId || channelWorldId === worldId);
});

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
    if (channelId && !characterCardStore.isBotCharacterDisabled(channelId)) {
      void characterCardStore.getActiveCard(channelId);
    }
    if (characterSheetStore.activeWindowIds.length > 0) {
      void characterSheetStore.refreshAllWindows();
    }
  }, ST_REFRESH_DELAY);
};

const audioStudio = useAudioStudioStore();

const openSplitRoute = async (scopeWorldId: string, worldId: string, channelIdA: string, channelIdB: string) => {
  audioStudio.setPlaybackAuthority(false);
  try {
    await router.push({
      name: 'split',
      query: {
        layout: 'left-column',
        scopeWorldId,
        worldId,
        a: channelIdA,
        b: channelIdB,
        notify: '',
      },
    });
  } catch (error) {
    audioStudio.setPlaybackAuthority(true);
    throw error;
  }
};

const openSplitView = async () => {
  const currentChannelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const worldId = chat.currentWorldId ? String(chat.currentWorldId) : '';
  await openSplitRoute(worldId, worldId, currentChannelId, '');
};

const openTheaterView = async () => {
  const worldId = routeWorldId.value || String(chat.currentWorldId || '').trim();
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const channelWorldId = String(chat.curChannel?.worldId || '').trim();
  if (!worldId || !channelId || String(chat.currentWorldId || '').trim() !== worldId || (channelWorldId && channelWorldId !== worldId)) {
    message.warning('正在切换世界，请稍后再试');
    return;
  }
  const ffmpegUnavailable = !audioStudio.ffmpegAvailable;
  await router.push({
    name: 'theater',
    query: { worldId, channelId },
  });
  if (ffmpegUnavailable) {
    dialog.warning({
      title: '未安装ffmpeg',
      content: () => (
        <div>
          <p>当前未安装ffmpeg，动图与音频将无法处理完成上传，请参考用户交流群文档进行安装。</p>
          <p>
            <a href="https://github.com/GyanD/codexffmpeg/releases/" target="_blank" rel="noopener noreferrer">下载ffmpeg-essentials_build.zip</a>
            后将 <code>ffmpeg</code> 与 <code>ffprobe</code>（Windows 为 <code>.exe</code>）放入程序根目录（<code>sealchat-server.exe</code> 路径），重启服务即可启用。
          </p>
        </div>
      ),
      positiveText: '知道了',
    });
  }
};

const openIcOocSplitView = async (side: 'left' | 'right') => {
  const currentChannelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const worldId = chat.currentWorldId ? String(chat.currentWorldId) : '';
  if (!worldId || !currentChannelId) {
    message.warning('请先进入频道');
    return;
  }
  const scopeWorldId = buildIcOocSplitScopeWorldId(worldId);
  const existingSnapshot = readSplitSessionSnapshot(scopeWorldId);
  const activeIdentityId = chat.getActiveIdentityId(currentChannelId);
  const activeIdentityVariantId = activeIdentityId
    ? chat.getActiveIdentityVariantId(currentChannelId, activeIdentityId)
    : '';
  const snapshot = resolveIcOocSplitSessionSnapshot(
    scopeWorldId,
    worldId,
    currentChannelId,
    side === 'right' ? 'ooc-left' : 'ic-left',
    existingSnapshot,
    {
      mode: chat.icMode === 'ooc' ? 'ooc' : 'ic',
      identityId: activeIdentityId,
      identityVariantId: activeIdentityVariantId,
      filterState: {
        icFilter: chat.filterState.icFilter,
        showArchived: chat.filterState.showArchived,
        roleIds: [...chat.filterState.roleIds],
        whisperOnly: chat.filterState.whisperOnly,
        fromTime: chat.filterState.fromTime,
        toTime: chat.filterState.toTime,
      },
    },
  );
  const writeOk = writeSplitSessionSnapshot(scopeWorldId, snapshot);
  if (!writeOk) {
    message.error('初始化场内外分屏失败');
    return;
  }
  await openSplitRoute(
    scopeWorldId,
    worldId,
    snapshot.panes.A.channelId || currentChannelId,
    snapshot.panes.B.channelId || currentChannelId,
  );
};

const toggleStickyNotes = () => {
  stickyNoteStore.toggleVisible();
};

const openDice3DSettings = () => {
  dice3dSettingsVisible.value = true;
};

const canManageDice3DWorld = computed(() => {
  const worldId = String(chat.currentWorldId || '').trim();
  const role = worldId ? chat.worldDetailMap[worldId]?.memberRole : '';
  return role === 'owner' || role === 'admin' || Boolean(user.checkPerm?.('mod_admin'));
});

const refreshDice3DSettings = async () => {
  const worldId = String(chat.currentWorldId || '').trim();
  if (!worldId || chat.observerMode) {
    dice3dConfig.value = null;
    dice3dProfile.value = null;
    return;
  }
  try {
    const settings = await loadDice3DSettings(worldId);
    if (worldId !== String(chat.currentWorldId || '').trim()) return;
    dice3dConfig.value = settings.config;
    dice3dProfile.value = settings.profile;
  } catch {
    if (worldId !== String(chat.currentWorldId || '').trim()) return;
    dice3dConfig.value = null;
    dice3dProfile.value = null;
  }
};

watch(() => chat.currentWorldId, () => void refreshDice3DSettings(), { immediate: true });

const handleDice3DProfileSaved = (profile: Dice3DMemberProfile) => {
  dice3dProfile.value = profile;
};

const handleDice3DSettingsUpdated = (event?: Event) => {
  const rawArgv = (event as any)?.argv || {};
  const options = (rawArgv.options || rawArgv.Options || {}) as Record<string, unknown>;
  const worldId = String(options.worldId || '').trim();
  if (!worldId || worldId !== String(chat.currentWorldId || '').trim()) return;

  const eventUserId = String(options.userId || '').trim();
  if (event?.type === 'world-member-dice3d-updated' && eventUserId && eventUserId !== String(user.info.id || '').trim()) return;
  void refreshDice3DSettings();
};

chatEvent.on('world-dice3d-updated' as any, handleDice3DSettingsUpdated as any);
chatEvent.on('world-member-dice3d-updated' as any, handleDice3DSettingsUpdated as any);

const handleDice3DDockMove = async (position: { x: number, y: number }) => {
  const worldId = String(chat.currentWorldId || '').trim();
  if (!worldId || !dice3dProfile.value) return;
  dice3dProfile.value = { ...dice3dProfile.value, dockCorner: 'free', dockX: position.x, dockY: position.y };
  try {
    await saveDice3DProfile(worldId, dice3dProfile.value);
  } catch {
    // 位置保留在本次会话；下次成功保存配置时同步。
  }
};

const resolveIFormEmbedLinkBase = () => {
  const domain = utils.config?.domain?.trim() || '';
  if (!domain) {
    return undefined;
  }
  const webUrl = utils.config?.webUrl?.trim() || '';
  let base = domain;
  if (!/^(https?:)?\/\//i.test(base)) {
    base = `${window.location.protocol}//${base}`;
  }
  if (webUrl) {
    base = `${base}${webUrl.startsWith('/') ? '' : '/'}${webUrl}`;
  }
  return base;
};

const defaultIFormEmbedLink = computed(() => {
  const worldId = chat.currentWorldId;
  const channelId = chat.curChannel?.id;
  if (!worldId || !channelId) {
    return '';
  }
  const firstForm = iFormStore.currentForms[0];
  if (!firstForm?.id) {
    return '';
  }
  return generateIFormEmbedLink(
    {
      worldId,
      channelId: firstForm.sourceChannelId || firstForm.channelId,
      formId: firstForm.id,
      width: firstForm.defaultWidth,
      height: firstForm.defaultHeight,
    },
    { base: resolveIFormEmbedLinkBase() },
  );
});

type ExternalPanelKey =
  | 'search'
  | 'archive'
  | 'export'
  | 'import'
  | 'identity'
  | 'gallery'
  | 'display'
  | 'dice3d'
  | 'favorites'
  | 'character-remark'
  | 'channel-images'
  | 'world-glossary'
  | 'world-announcement'
  | 'character-card'
  | 'sticky-note';

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
    case 'dice3d':
      openDice3DSettings();
      return;
    case 'favorites':
      channelFavoritesVisible.value = true;
      return;
    case 'character-remark':
      characterRemarkManagerVisible.value = true;
      return;
    case 'channel-images':
      openChannelImagesPanel();
      return;
    case 'world-glossary':
      if (!chat.currentWorldId) return;
      worldGlossary.ensureKeywords(chat.currentWorldId, { force: true });
      worldGlossary.ensureEffectiveKeywords(chat.currentWorldId, { force: true });
      worldGlossary.setManagerVisible(true);
      return;
    case 'world-announcement':
      if (!chat.currentWorldId) return;
      showWorldAnnouncementModal.value = true;
      return;
    case 'character-card':
      openCharacterCardPanel();
      return;
    case 'sticky-note':
      setStickyNoteVisible(true);
      return;
    default:
      return;
  }
};

const setSearchPanelVisibleForShell = (visible: boolean) => {
  if (visible) {
    channelSearch.openPanel();
    return;
  }
  channelSearch.closePanel();
};

const setFiltersForShell = (filters: any) => {
  chat.setFilterState(filters);
};

const setStickyNoteVisible = (visible: boolean) => {
  stickyNoteStore.setVisible(visible);
};

const setCharacterCardVisible = (visible: boolean) => {
  if (!visible) {
    characterCardPanelVisible.value = false;
    return;
  }
  openCharacterCardPanel();
};

const getStickyNoteVisible = () => stickyNoteStore.uiVisible;

const getCharacterCardVisible = () => characterCardPanelVisible.value;

const refreshPresenceForShell = async (silent = true) => {
  const selfId = user.info?.id || '';
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  if (!channelId) {
    chat.curChannelUsers = [];
    chat.clearPresenceMap();
    if (!silent) {
      message.warning('当前无可用频道');
    }
    return;
  }

  try {
    const onlineResp = await chat.sendAPI<any>('channel.member.list.online', { channel_id: channelId } as any);
    const onlineItems = Array.isArray(onlineResp?.data?.data) ? onlineResp.data.data : [];
    chat.curChannelUsers = onlineItems;

    const data = await chat.getChannelPresence();
    const updatedAt = typeof data?.updated_at === 'number' ? data.updated_at : undefined;
    if (typeof updatedAt === 'number') {
      chat.syncServerTime(updatedAt);
    }
    if (Array.isArray(data?.data)) {
      data.data.forEach((item: any) => {
        const userId = item?.user?.id || item?.user_id;
        if (!userId) {
          return;
        }
        const isSelf = selfId && userId === selfId;
        const lastSeenServer = item?.lastSeen ?? item?.last_seen;
        chat.updatePresence(userId, {
          lastPing: isSelf
            ? Date.now()
            : (typeof lastSeenServer === 'number' ? chat.serverTsToLocal(lastSeenServer) : Date.now()),
          latencyMs: isSelf ? chat.lastLatencyMs : (item?.latency ?? item?.latency_ms ?? 0),
          isFocused: isSelf ? chat.isAppFocused : (item?.focused ?? item?.is_focused ?? false),
        });
      });
    }
    chat.measureLatency();
    if (!silent) {
      message.success('状态已刷新');
    }
  } catch (error) {
    if (!silent) {
      message.error('刷新失败');
    } else {
      console.error('shell refresh presence failed', error);
    }
  }
};

interface TheaterMessageSendRequest {
  content: string;
  channelId?: string;
  characterId?: string;
  preserveComposer?: boolean;
}

interface TheaterComposerInsertRequest {
  content: string;
}

interface TheaterCharacterSnapshotRequest {
  revision: number;
  updatedAt: number;
}

interface TheaterCharacterSelectRequest {
  identityId: string;
}

interface TheaterCharacterVariantSelectRequest {
  identityId: string;
  variantId: string | null;
}

const sendMessageForTheater = async (payload: TheaterMessageSendRequest) => {
  const currentChannelId = String(chat.curChannel?.id || '').trim();
  if (!currentChannelId || (payload.channelId && payload.channelId !== currentChannelId)) {
    return { ok: false as const, error: { code: 'CHANNEL_MISMATCH', message: '聊天频道与小剧场上下文不一致' } };
  }
  if (spectatorInputDisabled.value) {
    return { ok: false as const, error: { code: 'PERMISSION_DENIED', message: '当前频道不允许发送消息' } };
  }
  if (!payload.preserveComposer && isEditing.value) {
    return { ok: false as const, error: { code: 'COMPOSER_BUSY', message: '正在编辑消息，无法执行舞台发送' } };
  }
  if (chat.connectState !== 'connected') {
    return { ok: false as const, error: { code: 'CHAT_DISCONNECTED', message: '聊天尚未连接' } };
  }
  if (!payload.preserveComposer && hasTheaterComposerDraft({
    meaningfulText: isContentMeaningful(inputMode.value, textToSend.value),
    inlineImageCount: inlineImages.size,
  })) {
    return { ok: false as const, error: { code: 'COMPOSER_BUSY', message: '输入框已有草稿，未覆盖现有内容' } };
  }

  let identityIdOverride: string | undefined;
  if (payload.characterId) {
    const identities = await chat.loadChannelIdentities(currentChannelId, false);
    const characterError = validateTheaterCharacter(identities, payload.characterId);
    if (characterError) return characterError;
    identityIdOverride = payload.characterId;
  }

  if (payload.preserveComposer) {
    try {
      const sent = await chat.messageCreate(
        payload.content,
        undefined,
        undefined,
        undefined,
        identityIdOverride,
        undefined,
        [],
      );
      return sent
        ? { ok: true as const, messageId: String(sent.id || '') }
        : { ok: false as const, error: { code: 'MESSAGE_SEND_REJECTED', message: '聊天发送流程拒绝了消息' } };
    } catch (error) {
      return {
        ok: false as const,
        error: { code: 'MESSAGE_SEND_FAILED', message: error instanceof Error ? error.message : '消息发送失败' },
      };
    }
  }

  textToSend.value = payload.content;
  const outcome = await performSend({
    identityIdOverride,
    mode: isTipTapJson(payload.content) ? 'rich' : 'plain',
  });
  return outcome || {
    ok: false as const,
    error: { code: 'MESSAGE_SEND_REJECTED', message: '聊天发送流程拒绝了消息' },
  };
};

const insertComposerForTheater = (payload: TheaterComposerInsertRequest) => {
  if (spectatorInputDisabled.value) {
    return { ok: false as const, error: { code: 'PERMISSION_DENIED', message: '当前频道不允许编辑消息草稿' } };
  }
  insertComposerText(payload.content);
  return { ok: true as const };
};

const getCharactersForTheater = async (
  request: TheaterCharacterSnapshotRequest,
): Promise<BridgeRoleSnapshot[]> => {
  const channelId = String(chat.curChannel?.id || '').trim();
  if (!channelId) return [];
  const identities = await chat.loadChannelIdentities(channelId, false);
  await chat.loadChannelIdentityVariants(channelId, false);
  const activeIdentityId = chat.getActiveIdentityId(channelId);
  return identities.map((identity) => {
    const selectedVariant = chat.getActiveIdentityVariant(channelId, identity.id);
    const activeVariant = selectedVariant?.enabled === false ? null : selectedVariant;
    return buildRoleSnapshot({
      identity,
      variant: activeVariant,
      variants: chat.getIdentityVariants(channelId, identity.id),
      resolvedAppearance: resolveIdentityAppearancePreview(identity, activeVariant),
      isActive: identity.id === activeIdentityId,
      revision: request.revision,
      updatedAt: request.updatedAt,
      resolveAttachmentUrl,
    });
  });
};

const selectCharacterForTheater = async (payload: TheaterCharacterSelectRequest) => {
  const channelId = String(chat.curChannel?.id || '').trim();
  const identityId = String(payload.identityId || '').trim();
  if (!channelId) {
    return { ok: false as const, error: { code: 'INVALID_CHARACTER', message: '角色参数无效' } };
  }
  const identities = await chat.loadChannelIdentities(channelId, false);
  const characterError = validateTheaterCharacter(identities, identityId);
  if (characterError) return characterError;
  try {
    const syncResult = await characterCardStore.syncCardForIdentity(channelId, identityId, { preserveWhenUnbound: true });
    if (!syncResult.ok) {
      return { ok: false as const, error: { code: 'CHARACTER_CARD_SYNC_FAILED', message: '角色卡同步失败，未切换聊天角色' } };
    }
  } catch (error) {
    console.warn('[theater-bridge] character card sync failed', error);
    return { ok: false as const, error: { code: 'CHARACTER_CARD_SYNC_FAILED', message: '角色卡同步失败，未切换聊天角色' } };
  }
  chat.setActiveIdentity(channelId, identityId);
  emitTypingPreview();
  return { ok: true as const };
};

const selectCharacterVariantForTheater = async (payload: TheaterCharacterVariantSelectRequest) => {
  const channelId = String(chat.curChannel?.id || '').trim();
  const identityId = String(payload.identityId || '').trim();
  const variantId = String(payload.variantId || '').trim();
  if (!channelId) {
    return { ok: false as const, error: { code: 'INVALID_CHARACTER', message: '角色参数无效' } };
  }
  const identities = await chat.loadChannelIdentities(channelId, false);
  const characterError = validateTheaterCharacter(identities, identityId);
  if (characterError) return characterError;
  const variantsByIdentity = await chat.loadChannelIdentityVariants(channelId, false);
  const variants = variantsByIdentity[identityId] || [];
  const variantError = validateTheaterVariant({
    activeIdentityId: chat.getActiveIdentityId(channelId),
    identityId,
    variantId,
    variants,
  });
  if (variantError) return variantError;
  chat.setActiveIdentityVariant(channelId, identityId, variantId);
  emitTypingPreview();
  return { ok: true as const };
};

const openCharacterCardForTheater = async (payload: { identityId: string }) => {
  const channelId = String(chat.curChannel?.id || '').trim();
  const identityId = String(payload.identityId || '').trim();
  if (!channelId || !identityId) {
    return { ok: false as const, error: { code: 'INVALID_CHARACTER', message: '人物卡参数无效' } };
  }
  await channelCharacterSnapshotStore.initializeChannel(channelId);
  let snapshot = channelCharacterSnapshotStore.getSnapshot(channelId, identityId);
  if (!snapshot) {
    await channelCharacterSnapshotStore.refreshChannel(channelId);
    snapshot = channelCharacterSnapshotStore.getSnapshot(channelId, identityId);
  }
  const card = snapshot?.data.card;
  if (!snapshot || !card) {
    return { ok: false as const, error: { code: 'CARD_UNAVAILABLE', message: '该角色没有可查看的人物卡快照' } };
  }
  const isOwnCard = String(snapshot.userId || '') === String(user.info?.id || '');
  const sourceCardId = String(snapshot.sourceCardId || '').trim();
  if (isOwnCard) {
    try {
      const opened = sourceCardId
        ? await characterCardPanelRef.value?.openCardById(sourceCardId, 'view')
        : false;
      if (opened) return { ok: true as const };
    } catch (error) {
      console.warn('[theater-bridge] failed to open editable character card', error);
    }
    message.warning('未能定位当前人物卡，已打开只读快照');
  }
  const windowId = characterSheetStore.openSheet({
    id: `snapshot:${channelId}:${identityId}`,
    name: card.name,
    sheetType: card.sheetType,
    attrs: card.attrs,
    channelId,
    userId: snapshot.userId,
  }, channelId, {
    name: card.name,
    type: card.sheetType,
    attrs: card.attrs,
    avatarUrl: snapshot.data.identity.avatarAttachmentId || undefined,
    templateText: card.templateText,
  }, {
    templateText: card.templateText,
    readOnly: true,
    worldId: chat.currentWorldId || undefined,
    placement: 'right',
  });
  characterSheetStore.setMode(windowId, 'view');
  return { ok: true as const };
};

defineExpose({
  openPanelForShell,
  refreshPresenceForShell,
  setSearchPanelVisibleForShell,
  setFiltersForShell,
  setStickyNoteVisible,
  setCharacterCardVisible,
  getStickyNoteVisible,
  getCharacterCardVisible,
  sendMessageForTheater,
  insertComposerForTheater,
  getCharactersForTheater,
  selectCharacterForTheater,
  selectCharacterVariantForTheater,
  openCharacterCardForTheater,
});
// 编辑模式下也允许使用上方功能区，只在个别操作需要限制时单独判断
const inputIcMode = computed<'ic' | 'ooc'>({
  get: () => {
    if (chat.editing?.icMode) {
      return chat.editing.icMode;
    }
    return chat.icMode;
  },
  set: (mode: 'ic' | 'ooc') => {
    if (chat.editing) {
      chat.updateEditingIcMode(mode);
    } else {
      const channelId = chat.curChannel?.id || '';
      const previousIdentityId = channelId ? chat.getActiveIdentityId(channelId) : '';
      chat.setIcMode(mode, channelId);
      // 触发自动角色切换
      chat.autoSwitchRoleOnIcOocChange(mode);
      const nextIdentityId = channelId ? chat.getActiveIdentityId(channelId) : '';
      if (channelId && nextIdentityId && nextIdentityId !== previousIdentityId) {
        void (async () => {
          const syncResult = await characterCardStore.syncCardForIdentity(channelId, nextIdentityId, {
            preserveWhenUnbound: true,
          });
          if (syncResult.ok) {
            emitTypingPreview();
          }
        })();
      }
    }
  },
});

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
const characterRemarkManagerVisible = ref(false);
const showWorldAnnouncementModal = ref(false);
const compactInlineLayout = computed(() => display.layout === 'compact' && !display.showAvatar);
const scrollButtonColor = computed(() => (display.palette === 'night' ? 'rgba(148, 163, 184, 0.25)' : '#e5e7eb'));
const scrollButtonTextColor = computed(() => (display.palette === 'night' ? 'rgba(248, 250, 252, 0.95)' : '#111827'));
const historyNavigationOpacityStyle = computed(() => ({
  opacity: display.settings.historyNavigationOpacity / 100,
}));
const canManageWorldAnnouncements = computed(() => {
  const worldId = chat.currentWorldId;
  if (!worldId) return false;
  const role = chat.worldDetailMap[worldId]?.memberRole;
  return role === 'owner' || role === 'admin';
});

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

const diceTrayWindowRef = ref<{
  toggle: () => void;
  hide: () => void;
  minimize: () => void;
  restore: () => void;
} | null>(null);
const diceSettingsVisible = ref(false);
const diceFeatureUpdating = ref(false);
const botOptions = ref<UserInfo[]>([]);
const botOptionsLoading = ref(false);
const botOptionsFetched = ref(false);
const isActualMobileUa = typeof navigator !== 'undefined'
  ? /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  : false;
const isMobileUa = isActualMobileUa
  || (route.path === '/embed' && route.query.viewport === 'mobile');
const hasTouchPoints = typeof navigator !== 'undefined'
  ? (navigator.maxTouchPoints || 0) > 0
  : false;
const chatRootContainerRef = ref<HTMLElement | null>(null);
const isCoarsePointerDevice = typeof window !== 'undefined'
  ? window.matchMedia('(pointer: coarse)').matches
  : false;
const diceTrayStorageScope = computed(() => {
  if (!isEmbedMode.value) return 'main';
  if (isTheaterEmbedMode.value) return 'embed:theater';
  const paneId = typeof route.query.paneId === 'string' ? route.query.paneId.trim() : '';
  return `embed:${paneId || 'standalone'}`;
});
const channelBotSelection = ref('');
const channelBotsLoading = ref(false);
const syncingChannelBot = ref(false);
const channelFeatures = reactive({
  builtInDiceEnabled: true,
  botFeatureEnabled: false,
});
const defaultDiceExpr = computed(() => ensureDefaultDiceExpr(chat.curChannel?.defaultDiceExpr));
const botRoleId = computed(() => {
  const channelId = chat.curChannel?.id;
  if (!channelId) {
    return '';
  }
  return `ch-${channelId}-bot`;
});
const channelFeatureManageAllowed = ref(false);
const canEditDefaultDice = computed(() => channelFeatureManageAllowed.value);
const canManageChannelFeatures = computed(() => canEditDefaultDice.value);
const botSelectOptions = computed(() => botOptions.value.map((bot) => ({
  label: bot.nick || bot.username || 'Bot',
  value: bot.id,
})));
const hasBotOptions = computed(() => botOptions.value.length > 0);
const diceModeLabel = computed(() => getDiceModeLabel({
  builtInDiceEnabled: channelFeatures.builtInDiceEnabled,
  botFeatureEnabled: channelFeatures.botFeatureEnabled,
  isBotPrivateChatChannel: isCurrentBotPrivateChatChannel.value,
}));
const diceModeTooltip = computed(() => {
  if (isCurrentBotPrivateChatChannel.value) {
    return '当前为机器人私聊，已固定使用 BOT 掷骰模式';
  }
  if (effectiveBotFeatureEnabled.value) {
    return '当前使用机器人处理掷骰指令，点击齿轮可切换内置掷骰模式';
  }
  if (!effectiveBuiltInDiceEnabled.value) {
    return '当前频道已关闭掷骰，可在设置中重新启用。';
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
const isBotPrivateChatChannel = (channel?: SChannel | null) => {
  if (!isPrivateChatChannel(channel)) {
    return false;
  }
  return channel?.friendInfo?.userInfo?.is_bot === true;
};
const isCurrentBotPrivateChatChannel = computed(() => isBotPrivateChatChannel(chat.curChannel as SChannel | null));
const effectiveBuiltInDiceEnabled = computed(() => (
  isCurrentBotPrivateChatChannel.value ? false : channelFeatures.builtInDiceEnabled
));
const effectiveBotFeatureEnabled = computed(() => (
  isCurrentBotPrivateChatChannel.value ? true : channelFeatures.botFeatureEnabled
));
const canUseBuiltInDice = computed(() => effectiveBuiltInDiceEnabled.value);
const showDiceTrayTrigger = computed(() => shouldShowDiceTrayTrigger({
  builtInDiceEnabled: channelFeatures.builtInDiceEnabled,
  botFeatureEnabled: channelFeatures.botFeatureEnabled,
  isBotPrivateChatChannel: isCurrentBotPrivateChatChannel.value,
}));
const showDiceModeStatus = computed(() => canManageChannelFeatures.value || isCurrentBotPrivateChatChannel.value);
const showDiceModeSettings = computed(() => canManageChannelFeatures.value && !isCurrentBotPrivateChatChannel.value);
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
const bridgeStatusDrawerVisible = ref(false);
const avatarReissueLoading = ref(false);
const avatarReissueResultText = ref('');
const emailNotificationDrawerVisible = ref(false);
const characterCardPanelVisible = ref(false);
const characterCardPanelRef = ref<{
  openCardById: (cardId: string, mode?: 'view' | 'edit') => Promise<boolean>;
} | null>(null);
const characterCardAvailable = computed(() => {
  const channelId = chat.curChannel?.id || '';
  if (!channelId) return false;
  return !characterCardStore.isBotCharacterDisabled(channelId);
});

const openCharacterCardPanel = () => {
  const channelId = chat.curChannel?.id || '';
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  characterCardPanelVisible.value = true;
  if (!characterCardAvailable.value) {
    const tip = characterCardStore.getCharacterApiDisabledReason(channelId) || characterApiUnsupportedText;
    message.warning(tip);
  }
};

const handleBridgeAvatarReissue = async () => {
  const channelId = chat.curChannel?.id || '';
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  const confirmed = await dialogAskConfirm(dialog, {
    title: '刷新当前频道角色头像？',
    content: '会为当前频道内你可管理用户的频道角色头像与差分头像重新签发附件 ID 和存储文件名，文件内容不会变化。',
    positiveText: '开始刷新',
    negativeText: '取消',
  });
  if (!confirmed) {
    return;
  }
  avatarReissueLoading.value = true;
  avatarReissueResultText.value = '';
  try {
    const result = await chat.reissueChannelIdentityAvatars(channelId);
    const summaryParts = [
      `已刷新 ${result.refreshedIdentityCount} 个角色头像`,
      `${result.refreshedVariantCount} 个差分头像`,
      `生成 ${result.createdAttachmentCount} 个新附件`,
    ];
    let summary = summaryParts.join('，');
    if (result.failedCount > 0) {
      const failurePreview = result.failed
        .slice(0, 3)
        .map((item) => `${item.scope}:${item.referenceId} - ${item.reason}`)
        .join('；');
      summary = `部分成功：${summary}；失败 ${result.failedCount} 项${failurePreview ? `。${failurePreview}` : ''}`;
      avatarReissueResultText.value = summary;
      message.warning(summary);
      return;
    }
    avatarReissueResultText.value = summary;
    message.success(summary);
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || error?.response?.data?.message || error?.message || '刷新失败';
    avatarReissueResultText.value = `刷新失败：${errMsg}`;
    message.error(errMsg);
  } finally {
    avatarReissueLoading.value = false;
  }
};
let webhookPermissionSeq = 0;
let channelFeaturePermissionSeq = 0;

watch(
  () => chat.curChannel?.id,
  async (channelId) => {
    const seq = ++channelFeaturePermissionSeq;
    const currentChannel = chat.curChannel as SChannel | undefined;
    if (!channelId || !currentChannel) {
      channelFeatureManageAllowed.value = false;
      return;
    }
    if (isPrivateChatChannel(currentChannel)) {
      channelFeatureManageAllowed.value = false;
      return;
    }
    try {
      const [canManageInfo, canRoleLink] = await Promise.all([
        chat.hasChannelPermission(channelId, 'func_channel_manage_info', user.info.id),
        chat.hasChannelPermission(channelId, 'func_channel_role_link', user.info.id),
      ]);
      if (seq === channelFeaturePermissionSeq) {
        channelFeatureManageAllowed.value = !!(canManageInfo || canRoleLink);
      }
    } catch {
      if (seq === channelFeaturePermissionSeq) {
        channelFeatureManageAllowed.value = false;
      }
    }
  },
  { immediate: true },
);
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
  if (!effectiveBuiltInDiceEnabled.value && !effectiveBotFeatureEnabled.value) {
    message.warning('内置骰点已关闭，请在设置中启用或切换机器人。');
    return;
  }
  diceTrayWindowRef.value?.toggle();
};
const handleDiceTrayModeChange = (mode: 'hidden' | 'expanded' | 'minimized') => {
  if (mode !== 'expanded') {
    diceSettingsVisible.value = false;
  }
};
watch(
  () => [chat.curChannel?.id, chat.curChannel?.builtInDiceEnabled, chat.curChannel?.botFeatureEnabled, chat.curChannel?.friendInfo?.userInfo?.is_bot] as const,
  ([, builtInDiceEnabled, botFeatureEnabled]) => {
    if (isCurrentBotPrivateChatChannel.value) {
      channelFeatures.builtInDiceEnabled = false;
      channelFeatures.botFeatureEnabled = true;
    } else {
      channelFeatures.builtInDiceEnabled = builtInDiceEnabled !== false;
      channelFeatures.botFeatureEnabled = botFeatureEnabled === true;
    }
  },
  { immediate: true },
);
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
  if (!enabled && !effectiveBotFeatureEnabled.value) {
    diceSettingsVisible.value = false;
  }
});
watch(() => channelFeatures.botFeatureEnabled, (enabled) => {
  if (!enabled && !effectiveBuiltInDiceEnabled.value) {
    diceSettingsVisible.value = false;
  }
});
watch(diceSettingsVisible, (visible) => {
  if (visible) {
    ensureBotOptionsLoaded();
    refreshChannelBotSelection();
  }
});

const handleGlobalOverlayToggle = (payload?: { source?: string; open?: boolean }) => {
  if (!payload?.open) {
    return;
  }
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
	console.info('[dice-bot] bot-options-load-start', {
		channelId: chat.curChannel?.id || '',
		force,
		ts: Date.now(),
	});
	try {
		const resp = await chat.botList(force);
		botOptions.value = resp?.items || [];
		botOptionsFetched.value = true;
	} catch (error: any) {
		message.error(error?.response?.data?.message || '获取机器人列表失败');
	} finally {
		botOptionsLoading.value = false;
		console.info('[dice-bot] bot-options-load-finish', {
			channelId: chat.curChannel?.id || '',
			count: botOptions.value.length,
			ts: Date.now(),
		});
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
  console.info('[dice-bot] channel-bot-refresh-start', {
    channelId,
    ts: Date.now(),
  });
  try {
    const resp = await chat.channelMemberListAll(channelId, 200);
    const items = resp?.data?.items || [];
    const existingIds = items
      .filter((item: any) => item.roleId === roleId && item.user?.id)
      .map((item: any) => item.user?.id as string)
      .filter(Boolean);
    const primaryBotId = String(chat.curChannel?.primaryBotId || '').trim();
    channelBotSelection.value = primaryBotId && existingIds.includes(primaryBotId)
      ? primaryBotId
      : (existingIds[0] || '');
  } catch (error: any) {
    message.error(error?.response?.data?.error || '加载频道机器人失败');
  } finally {
    channelBotsLoading.value = false;
    console.info('[dice-bot] channel-bot-refresh-finish', {
      channelId,
      selectedBotId: channelBotSelection.value,
      ts: Date.now(),
    });
  }
};

const syncChannelBotSelection = async (nextBotId: string) => {
  const channelId = chat.curChannel?.id;
  const roleId = botRoleId.value;
  if (!channelId || !roleId) {
    return;
  }
  syncingChannelBot.value = true;
  console.info('[dice-bot] channel-bot-sync-start', {
    channelId,
    nextBotId,
    ts: Date.now(),
  });
  try {
    const resp = await chat.channelMemberListAll(channelId, 200);
    const items = resp?.data?.items || [];
    const existingIds = items
      .filter((item: any) => item.roleId === roleId && item.user?.id)
      .map((item: any) => item.user.id as string);
    if (nextBotId && !existingIds.includes(nextBotId)) {
      await chat.userRoleLink(roleId, [nextBotId]);
    }
    if (!nextBotId && existingIds.length) {
      await chat.userRoleUnlink(roleId, existingIds);
    }
    await chat.updateChannelFeatures(channelId, { primaryBotId: nextBotId || '' });
    if (nextBotId) {
      void characterCardStore.revalidateCharacterApi(channelId);
    }
    channelBotSelection.value = nextBotId;
  } catch (error: any) {
    message.error(error?.response?.data?.error || '配置机器人失败');
    throw error;
  } finally {
    syncingChannelBot.value = false;
    console.info('[dice-bot] channel-bot-sync-finish', {
      channelId,
      selectedBotId: channelBotSelection.value,
      ts: Date.now(),
    });
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
    const payload = await chat.updateChannelFeatures(chat.curChannel.id, updates);
    if (typeof payload?.built_in_dice_enabled === 'boolean') {
      channelFeatures.builtInDiceEnabled = payload.built_in_dice_enabled;
    } else if (typeof updates.builtInDiceEnabled === 'boolean') {
      channelFeatures.builtInDiceEnabled = updates.builtInDiceEnabled;
    }
    if (typeof payload?.bot_feature_enabled === 'boolean') {
      channelFeatures.botFeatureEnabled = payload.bot_feature_enabled;
    } else if (typeof updates.botFeatureEnabled === 'boolean') {
      channelFeatures.botFeatureEnabled = updates.botFeatureEnabled;
    }
    if (typeof payload?.primary_bot_id === 'string') {
      chat.patchChannelAttributes(chat.curChannel.id, { primaryBotId: payload.primary_bot_id });
    }
  } catch (error: any) {
    message.error(error?.response?.data?.error || '更新频道特性失败');
    throw error;
  } finally {
    diceFeatureUpdating.value = false;
  }
};

const handleDiceFeatureToggle = async (value: boolean) => {
  if (!canManageChannelFeatures.value || isCurrentBotPrivateChatChannel.value) {
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
  if (!canManageChannelFeatures.value || !botRoleId.value || isCurrentBotPrivateChatChannel.value) {
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
watch(() => chat.curChannel?.id, (id, prevId) => {
  if (id === prevId) {
    return;
  }
  if (chat.messageInsertTarget.enabled) {
    chat.clearMessageInsertTarget();
  }
});
const INLINE_STACK_BREAKPOINT = 640;
const { width: windowWidth } = useWindowSize();
const webhookDrawerWidth = computed(() => (windowWidth.value > 0 && windowWidth.value < 768 ? '100%' : 520));
const bridgeStatusDrawerWidth = computed(() => (windowWidth.value > 0 && windowWidth.value < 768 ? '100%' : 560));
const emailNotificationDrawerWidth = computed(() => (windowWidth.value > 0 && windowWidth.value < 768 ? '100%' : 480));
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
const showActionRibbon = ref(isTheaterEmbedMode.value);
const archiveDrawerVisible = ref(false);
const exportManagerVisible = ref(false);
const exportDialogVisible = ref(false);
const exportDialogBatchMode = ref(false);
const exportManagerRefreshVersion = ref(0);
const exportManagerRevealVersion = ref(0);
const battleReportDrawerVisible = ref(false);
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
        id: String(item.id || '').trim(),
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
  if (characterCardStore.isBotCharacterDisabled(channelId)) return;
  if (!enabled) return;
  void characterCardStore.requestBadgeSnapshot(channelId);
  if (!options?.skipActiveCard) {
    void characterCardStore.getActiveCard(channelId);
  }
};

const initCharacterRemark = (channelId?: string) => {
  if (!channelId) return;
  void characterRemarkStore.requestRemarkSnapshot(channelId);
};

const CHARACTER_RESUME_SYNC_MIN_INTERVAL_MS = 1500;
let lastCharacterResumeSyncAt = 0;
let characterResumeSyncEpoch = 0;
const syncCharacterCardAfterResume = async (
  reason: string,
  options?: { forceIdentityReload?: boolean; bypassCooldown?: boolean },
) => {
  const channelId = chat.curChannel?.id;
  if (!channelId || chat.isObserver) {
    return;
  }
  if (characterCardStore.isBotCharacterDisabled(channelId)) {
    return;
  }
  const now = Date.now();
  if (!options?.bypassCooldown && now - lastCharacterResumeSyncAt < CHARACTER_RESUME_SYNC_MIN_INTERVAL_MS) {
    return;
  }
  lastCharacterResumeSyncAt = now;
  const currentEpoch = ++characterResumeSyncEpoch;
  try {
    await chat.loadChannelIdentities(channelId, !!options?.forceIdentityReload);
  } catch (error) {
    console.warn('[CharacterCard] resume identity sync failed', reason, error);
  }
  if (currentEpoch !== characterResumeSyncEpoch || channelId !== chat.curChannel?.id) {
    return;
  }
  initCharacterCardBadge(channelId);
};

const handleForegroundResume = () => {
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
    return;
  }
  chat.recoverConnectionOnForeground('chat-view-resume');
  void syncCharacterCardAfterResume('chat-view-resume');
};

const handleVisibilityResume = () => {
  if (typeof document === 'undefined' || document.visibilityState !== 'visible') {
    return;
  }
  handleForegroundResume();
};

let identitySelectionEpoch = 0;
const simulateCurrentIdentitySelection = async (channelId?: string) => {
  if (!channelId || chat.isObserver) {
    return false;
  }
  if (characterCardStore.isBotCharacterDisabled(channelId)) {
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
    const syncResult = await characterCardStore.syncCardForIdentity(channelId, identityId, {
      preserveWhenUnbound: true,
    });
    if (!syncResult.ok) {
      return false;
    }
    emitTypingPreview();
    return syncResult.switched;
  } catch (e) {
    console.warn('Failed to simulate identity selection', e);
    return false;
  }
};

let presenceBadgeChannelId = '';
let presenceBadgeInitialized = false;
const presenceBadgeUsers = new Set<string>();

watch(
  () => [chat.curChannel?.id, chat.curChannel?.characterApiEnabled] as const,
  ([channelId, characterApiEnabled]) => {
    initCharacterRemark(channelId);
    if (channelId && characterApiEnabled === true) {
      characterCardStore.markCharacterApiHealthy(channelId);
    }
    if (!channelId || characterApiEnabled !== true) return;
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
  showActionRibbon.value = true;
  displaySettingsVisible.value = true;
};

const handleDisplaySettingsSave = (settings: Partial<DisplaySettings>) => {
  display.updateSettings(settings);
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
  (visible) => {
    syncActionRibbonState();
  },
  { immediate: true },
);

chatEvent.on('action-ribbon-toggle', handleActionRibbonToggleRequest);
chatEvent.on('action-ribbon-state-request', handleActionRibbonStateRequest);
chatEvent.on('open-display-settings', handleOpenDisplaySettings);

const editingIdentityPreviewContext = computed(() => {
  if (!isEditingCurrentChannel.value || !chat.editing) {
    return null;
  }
  const channelId = String(chat.editing.channelId || '').trim();
  const identityId = String(chat.editing.identityId || '').trim();
  const identity = identityId
    ? (chat.channelIdentities[channelId] || []).find((item) => item.id === identityId) || null
    : null;
  const variantId = identityId ? String(chat.editing.identityVariantId || '').trim() : '';
  const variant = identity && variantId
    ? chat.getIdentityVariants(channelId, identityId).find((item) => item.id === variantId) || null
    : null;
  let appearance = identity ? resolveIdentityAppearancePreview(identity, variant) : null;
  const snapshot = chat.editing.identitySnapshot as MessageIdentitySnapshot | null | undefined;
  if (!appearance && snapshot && snapshot.identityId === (identityId || null)) {
    appearance = {
      identityId: snapshot.identityId || '',
      variantId,
      displayName: snapshot.displayName || '',
      color: snapshot.color || '',
      avatarAttachmentId: snapshot.avatarAttachmentId || '',
      avatarDecorations: cloneAvatarDecorations(snapshot.avatarDecorations),
      isTemporary: Boolean(snapshot.isTemporary),
    };
  }
  return {
    channelId,
    identityId: identityId || null,
    identity,
    variantId: variantId || null,
    variant,
    appearance,
  };
});
const galleryPanelVisible = computed(() => gallery.isPanelVisible);
const channelImagesPanelVisible = computed(() => channelImages.panelVisible);

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n();
const {
  emojiLoading,
  emojiItems,
  ensureEmojiAttachmentMeta,
  resolveEmojiAttachmentUrl,
  getEmojiItemSrc,
  hasEmojiItems,
  emojiPopoverShow,
  emojiTriggerButtonRef,
  emojiAnchorElement,
  emojiPopoverXCoord,
  emojiPopoverYCoord,
  emojiSearchQuery,
  emojiPanelTab,
  emojiPanelRenderKey,
  emojiPanelContentRef,
  emojiPanelLoadMoreSentinelRef,
  isManagingEmoji,
  emojiRemarkVisible,
  activeIdentityForEmojiPanel,
  activeIdentityVariantOptions,
  activeIdentityVariantForEmojiPanel,
  filteredIdentityVariantOptions,
  hasIdentityVariantOptions,
  identityVariantTabTooltip,
  describeIdentityVariantCard,
  activeEmojiTab,
  emojiTabOptions,
  hasMultipleTabs,
  emojiPanelPagination,
  emojiPanelLoading,
  emojiPanelLoadingMore,
  emojiPanelHasMore,
  toggleEmojiRemarkVisible,
  syncEmojiPopoverPosition,
  allGalleryItems,
  emojiUsageMap,
  ensureEmojiCollectionLoaded,
  loadMoreEmojiPanelItems,
  handleEmojiPanelContentScroll,
  refreshEmojiPanelRender,
  recordEmojiUsage,
  filteredEmojiItems,
  buildEmojiRemarkMap,
  replaceEmojiRemarksForPreview,
  selectedEmojiIds,
  emojiRemarkModalVisible,
  emojiRemarkInput,
  emojiRemarkSaving,
  editingEmojiItem,
  resolveEmojiRemark,
  openEmojiRemarkEditor,
  submitEmojiRemark,
  cancelEmojiRemark,
  exitEmojiManage,
  emojiSelectedDelete,
} = useChatEmoji({
  chat,
  gallery,
  user,
  message,
  dialog,
  editingIdentityPreviewContext,
  resolveIdentityAppearancePreview: (...args) => resolveIdentityAppearancePreview(...args),
  cloneAvatarDecorations: (...args) => cloneAvatarDecorations(...args),
  resolveVariantNote: (variant) => resolveVariantNote(variant),
  canManageIdentities: () => canManageIdentities(),
});
onMounted(() => {
  checkAvatarPromptOnMount();
});

// const virtualListRef = ref<InstanceType<typeof VirtualList> | null>(null);
const messagesListRef = ref<HTMLElement | null>(null);
const selectionBar = reactive({
  visible: false,
  text: '',
  position: { x: 0, y: 0 },
})
const selectionBarRef = ref<HTMLElement | null>(null)
const selectionMaxLength = 120
let activeMessageSelectionRange: Range | null = null

const hideSelectionBar = () => {
  selectionBar.visible = false
  selectionBar.text = ''
  activeMessageSelectionRange = null
}

const serializeMessageSelectionRange = (range: Range | null): string => {
  if (!range) {
    return ''
  }
  const fragment = range.cloneContents()
  const text = Array.from(fragment.childNodes)
    .map((node) => serializePlainTextFromDomNode(node))
    .join('')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
  return text
}

watch(
  () => chat.currentWorldId,
  (worldId) => {
    if (!worldId) {
      return
    }
    worldGlossary.ensureKeywords(worldId)
    worldGlossary.ensureEffectiveKeywords(worldId)
    chat.worldDetail(worldId)
    void aiStore.loadCapabilities(String(worldId))
    hideSelectionBar()
  },
  { immediate: true },
)

watch(
  () => chat.curChannel?.id,
  () => hideSelectionBar(),
)

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
  const serializedText = serializeMessageSelectionRange(range)
  if (!serializedText || serializedText.length === 0 || serializedText.length > selectionMaxLength) {
    hideSelectionBar()
    return
  }
  updateSelectionPosition(rect)
  activeMessageSelectionRange = range.cloneRange()
  selectionBar.text = serializedText
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
  const text = serializeMessageSelectionRange(activeMessageSelectionRange) || selectionBar.text
  if (!text) return
  const copied = await copyTextWithFallback(text)
  if (copied) {
    message.success('已复制选中文本')
  } else {
    message.error('复制失败')
  }
  hideSelectionBar()
}

const handleNativeSelectionCopy = (event: ClipboardEvent) => {
  const container = messagesListRef.value
  if (!container || !event.clipboardData) {
    return
  }
  const selection = window.getSelection()
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
    return
  }
  const range = selection.getRangeAt(0)
  const node = range.commonAncestorContainer instanceof Element ? range.commonAncestorContainer : range.commonAncestorContainer?.parentElement
  if (!node || !container.contains(node)) {
    return
  }
  const serializedText = serializeMessageSelectionRange(range)
  if (!serializedText) {
    return
  }
  event.preventDefault()
  event.clipboardData.setData('text/plain', serializedText)
  selectionBar.text = serializedText
  activeMessageSelectionRange = range.cloneRange()
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
  channelSearch.setWithinResultsEnabled(false)
  channelSearch.bindChannel(chat.curChannel?.id || null)
  void channelSearch.searchPrimary(chat.curChannel?.id || undefined)
  hideSelectionBar()
}

const canAddKeywordFromSelection = computed(() => selectionBar.visible && canManageWorldKeywords.value && Boolean(chat.currentWorldId))

if (typeof window !== 'undefined') {
  useEventListener(document, 'selectionchange', handleSelectionChange)
  useEventListener(document, 'pointerdown', handlePointerDown, { capture: true })
  useEventListener(window, 'resize', hideSelectionBar)
  useEventListener(document, 'copy', handleNativeSelectionCopy)
}

const topSentinelRef = ref<HTMLElement | null>(null);
const bottomSentinelRef = ref<HTMLElement | null>(null);
const textInputRef = ref<any>(null);
const minimalInputActionsHostRef = ref<HTMLElement | null>(null);
const minimalInputMeasureRef = ref<HTMLElement | null>(null);
const minimalInputToolbarVisible = ref(false);
const minimalInputMeasuredHeight = ref(0);
let minimalInputMeasureObserver: ResizeObserver | null = null;
const inputMode = ref<'plain' | 'rich'>('plain');
const richContentCache = ref<string | null>(null);
const plainTextFromRichCache = ref<string>('');
const wideInputMode = ref(false);
const MINIMAL_INPUT_STACKED_THRESHOLD = 104;
const MINIMAL_INPUT_WIDE_BUTTON_THRESHOLD = 128;
const isMobileInteractionMode = computed(() => {
  if (isMobileUa) {
    return true;
  }
  const width = windowWidth.value;
  return width > 0 && width <= 768 && isCoarsePointerDevice && hasTouchPoints;
});
const isMobileWideInput = computed(() => wideInputMode.value && isMobileInteractionMode.value);
const isMinimalInputActive = computed(() => (
  display.settings.mobileMinimalInputEnabled
  && !isMobileWideInput.value
));
const inputExtraActionsTeleportTarget = computed<HTMLElement | null>(() => {
  if (!isMinimalInputActive.value || !minimalInputToolbarVisible.value) {
    return null;
  }
  return minimalInputActionsHostRef.value;
});
const showMinimalStackedSideControls = computed(() => (
  isMinimalInputActive.value
  && minimalInputMeasuredHeight.value >= MINIMAL_INPUT_STACKED_THRESHOLD
));
const showMinimalWideInputShortcut = computed(() => (
  showMinimalStackedSideControls.value
  && minimalInputMeasuredHeight.value >= MINIMAL_INPUT_WIDE_BUTTON_THRESHOLD
));
const inputAreaHeightPreview = ref<number | null>(null);
const inputAreaHeightBeforeWideMode = ref<number | null>(null);
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
  const nextWideInputMode = !wideInputMode.value;
  if (nextWideInputMode) {
    inputAreaHeightBeforeWideMode.value = customInputHeight.value > 0 ? customInputHeight.value : null;
    // 进入广域模式时隐藏用户自定义高度，回到广域预设高度
    if (customInputHeight.value > 0) {
      display.updateSettings({ inputAreaHeight: 0 });
      inputAreaHeightPreview.value = null;
    }
  } else {
    const savedHeight = inputAreaHeightBeforeWideMode.value;
    if (customInputHeight.value <= 0 && savedHeight && savedHeight > 0) {
      display.updateSettings({ inputAreaHeight: savedHeight });
    }
    inputAreaHeightBeforeWideMode.value = null;
  }
  wideInputMode.value = nextWideInputMode;
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

const measureMinimalInputHeight = () => {
  const el = minimalInputMeasureRef.value;
  minimalInputMeasuredHeight.value = el ? Math.round(el.getBoundingClientRect().height) : 0;
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

watch(minimalInputMeasureRef, (el, prevEl) => {
  if (minimalInputMeasureObserver && prevEl) {
    minimalInputMeasureObserver.unobserve(prevEl);
  }
  if (!el || typeof ResizeObserver === 'undefined') {
    if (!el) {
      minimalInputMeasuredHeight.value = 0;
    }
    return;
  }
  if (!minimalInputMeasureObserver) {
    minimalInputMeasureObserver = new ResizeObserver(() => {
      measureMinimalInputHeight();
    });
  }
  minimalInputMeasureObserver.observe(el);
  nextTick(() => {
    measureMinimalInputHeight();
  });
}, { flush: 'post' });

onBeforeUnmount(() => {
  if (typeof document !== 'undefined') {
    document.documentElement.style.removeProperty('--wide-input-height');
  }
  if (minimalInputMeasureObserver) {
    minimalInputMeasureObserver.disconnect();
    minimalInputMeasureObserver = null;
  }
});

// 输入区域高度拖拽调整（通过上边框触发）
const inputContainerRef = ref<HTMLElement | null>(null);
const isResizingInput = ref(false);
const resizeStartY = ref(0);
const resizeStartHeight = ref(0);
const resizePointerId = ref<number | null>(null);
const shouldExitWideInput = ref(false);
let resizeEventTarget: HTMLElement | null = null;

const handleInputBorderPointerDown = (e: PointerEvent) => {
  if (isMobileWideInput.value) return;
  if (e.pointerType === 'mouse' && e.button !== 0) return;
  const container = inputContainerRef.value;
  const target = e.currentTarget as HTMLElement | null;
  if (!container) return;
  if (!target) return;

  e.preventDefault();
  e.stopPropagation();

  // 在真实拖拽热区上捕获指针，避免移动端手势在容器本体上被浏览器接管。
  resizePointerId.value = e.pointerId;
  resizeEventTarget = target;
  target.setPointerCapture(e.pointerId);

  isResizingInput.value = true;
  resizeStartY.value = e.clientY;
  const inputEditor = document.querySelector('.chat-input-editor-main') as HTMLElement;
  resizeStartHeight.value = customInputHeight.value > 0
    ? customInputHeight.value
    : (inputEditor?.offsetHeight || INPUT_AREA_HEIGHT_LIMITS.MIN);
  inputAreaHeightPreview.value = resizeStartHeight.value;

  target.addEventListener('pointermove', handleInputResizeMove as EventListener);
  target.addEventListener('pointerup', handleInputResizeEnd as EventListener);
  target.addEventListener('pointercancel', handleInputResizeEnd as EventListener);
  target.addEventListener('lostpointercapture', handleInputResizeEnd as EventListener);
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

  const eventTarget = resizeEventTarget;
  const exitWideInput = shouldExitWideInput.value && wideInputMode.value;
  shouldExitWideInput.value = false;
  const finalHeight = inputAreaHeightPreview.value ?? display.settings.inputAreaHeight;
  inputAreaHeightPreview.value = null;
  if (eventTarget) {
    if (resizePointerId.value !== null) {
      try {
        eventTarget.releasePointerCapture(resizePointerId.value);
      } catch (_) { /* ignore */ }
    }
    eventTarget.removeEventListener('pointermove', handleInputResizeMove as EventListener);
    eventTarget.removeEventListener('pointerup', handleInputResizeEnd as EventListener);
    eventTarget.removeEventListener('pointercancel', handleInputResizeEnd as EventListener);
    eventTarget.removeEventListener('lostpointercapture', handleInputResizeEnd as EventListener);
  }

  resizeEventTarget = null;
  resizePointerId.value = null;
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  if (exitWideInput) {
    inputAreaHeightBeforeWideMode.value = null;
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
  if (wideInputMode.value && finalHeight !== display.settings.inputAreaHeight) {
    inputAreaHeightBeforeWideMode.value = null;
  }
  if (finalHeight !== display.settings.inputAreaHeight) {
    display.updateSettings({ inputAreaHeight: finalHeight });
  }
};

const handleInputResizeReset = () => {
  inputAreaHeightPreview.value = null;
  if (wideInputMode.value) {
    inputAreaHeightBeforeWideMode.value = null;
  }
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
type InlineUploadSource =
  | 'default'
  | 'rich-toolbar'
  | 'rich-editor'
  | 'smart-link-text-image'
  | 'smart-link-url-image';

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
  const normalized = normalizeAttachmentId(raw);
  if (normalized && (raw.startsWith('id:') || /^[0-9A-Za-z_-]+$/.test(raw) || /api\/v1\/attachment\//i.test(raw))) {
    record.attachmentId = normalized;
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
    const normalized = normalizeAttachmentId(draft.attachmentId);
    return normalized ? `/api/v1/attachment/${normalized}` : '';
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
  if (isSmartLinkNode(node)) {
    return smartLinkToPlainText(node.attrs);
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
  const normalizedText = text.replace(/\r\n/g, '\n');
  const lines = normalizedText.split('\n');
  const paragraphNodes: Array<{ type: string; text?: string; attrs?: Record<string, string> }> = [];

  lines.forEach((line, index) => {
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
    if (nodes.length) {
      paragraphNodes.push(...nodes);
    }
    if (index < lines.length - 1) {
      paragraphNodes.push({ type: 'hardBreak' });
    }
  });

  return {
    type: 'doc',
    content: [
      paragraphNodes.length
        ? { type: 'paragraph', content: paragraphNodes }
        : { type: 'paragraph' },
    ],
  };
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
let pendingInlineUploadSource: InlineUploadSource = 'default';
let activeInlineEditorSelection: SelectionRange | null = null;
let activeInlineEditorSource: InlineUploadSource = 'default';
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

const richInlineImageEditorVisible = ref(false);
const richInlineImageEditorFile = ref<File | null>(null);

const closeRichInlineImageEditor = () => {
  richInlineImageEditorVisible.value = false;
  richInlineImageEditorFile.value = null;
  activeInlineEditorSelection = null;
  activeInlineEditorSource = 'default';
};

const openRichInlineImageEditor = (
  files: File[],
  source: InlineUploadSource = pendingInlineUploadSource,
  selection: SelectionRange | null = pendingInlineSelection,
) => {
  const imageFiles = files.filter((file) => file.type.startsWith('image/'));
  if (!imageFiles.length) {
    message.warning('当前仅支持插入图片文件');
    return;
  }
  if (imageFiles.length > 1) {
    message.info('富文本插图编辑首版仅支持单张图片，已载入第一张');
  }
  activeInlineEditorSource = source;
  activeInlineEditorSelection = selection ? { ...selection } : null;
  richInlineImageEditorFile.value = imageFiles[0] || null;
  richInlineImageEditorVisible.value = !!richInlineImageEditorFile.value;
};

const handleMessageInlineImageEdit = async (payload: { attachmentId: string; messageId?: string; src?: string }) => {
  const attachmentId = normalizeAttachmentId(payload.attachmentId || payload.src || '');
  if (!attachmentId) {
    message.warning('无法识别消息图片');
    return;
  }

  const source: InlineUploadSource = inputMode.value === 'rich' ? 'rich-editor' : 'default';
  const selection = captureSelectionRange();

  try {
    const file = await fetchAttachmentFileById(attachmentId, `message-image-${attachmentId}.png`);
    if (!file) {
      message.error('图片载入失败');
      return;
    }
    openRichInlineImageEditor([file], source, selection);
  } catch (error: any) {
    console.error('载入聊天消息图片失败', error);
    message.error(error?.message || '图片载入失败');
  }
};

const handleRichInlineImageEditorConfirm = async (file: File) => {
  const activeSource = activeInlineEditorSource;
  const isSmartLinkUpload = activeSource === 'smart-link-text-image'
    || activeSource === 'smart-link-url-image';
  const shouldInsertIntoRichEditor = activeSource === 'rich-editor' || inputMode.value === 'rich';
  const targetSelection = activeInlineEditorSelection ? { ...activeInlineEditorSelection } : undefined;
  closeRichInlineImageEditor();
  if (isSmartLinkUpload) {
    try {
      const result = await uploadImageAttachment(file, {
        channelId: chat.curChannel?.id,
        skipCompression: true,
      });
      const normalizedId = normalizeAttachmentId(result.attachmentId);
      const finalUrl = normalizedId ? `/api/v1/attachment/${normalizedId}` : String(result.attachmentId || '');
      if (!finalUrl) {
        throw new Error('图片上传成功但未获取到可用地址');
      }
      textInputRef.value?.applySmartLinkImage?.(activeSource, {
        url: finalUrl,
        label: file.name || '已选图片',
      });
    } catch (error: any) {
      console.error('上传 smart link 图片失败', error);
      message.error(error?.message || '图片上传失败');
    }
    return;
  }
  if (shouldInsertIntoRichEditor) {
    await handleRichImageInsert([file], { skipCompression: true });
    return;
  }
  insertInlineImages([file], targetSelection, { skipCompression: true });
};

const identityDialogVisible = ref(false);

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

const handleEmojiTriggerClick = (event?: MouseEvent) => {
  if (emojiPopoverShow.value) {
    emojiPopoverShow.value = false;
    return;
  }
  emojiPanelTab.value = 'gallery';
  syncEmojiPopoverPosition(event?.currentTarget as HTMLElement | null);
  emojiPopoverShow.value = true;
};

const switchEmojiPanelTab = (tab: 'gallery' | 'utf' | 'variant') => {
  emojiPanelTab.value = tab;
  refreshEmojiPanelRender();
  if (tab !== 'gallery') {
    isManagingEmoji.value = false;
  }
};

const handleUtfEmojiSelect = (emoji: string) => {
  if (!emoji || emoji.startsWith('id:')) {
    return;
  }
  if (inputMode.value === 'rich') {
    const editorInstance = textInputRef.value?.getEditor?.();
    if (editorInstance) {
      editorInstance.chain().focus().insertContent(emoji).run();
      return;
    }
  }
  const selection = getInputSelection();
  const text = textToSend.value;
  textToSend.value = text.slice(0, selection.start) + emoji + text.slice(selection.end);
  const cursor = selection.start + emoji.length;
  nextTick(() => setInputSelection(cursor, cursor));
  ensureInputFocus();
};

const handleEmojiVariantSelect = (variantId: string) => {
  const channelId = chat.curChannel?.id || '';
  const identityId = activeIdentityForEmojiPanel.value?.id || '';
  if (!channelId || !identityId) {
    return;
  }
  if (isEditingCurrentChannel.value && chat.editing?.identityId === identityId) {
    chat.updateEditingIdentityVariant(variantId || null);
    emojiSearchQuery.value = '';
    emojiPopoverShow.value = false;
    emitEditingPreview();
    return;
  }
  chat.setActiveIdentityVariant(channelId, identityId, variantId);
  emojiSearchQuery.value = '';
  emojiPopoverShow.value = false;
  emitTypingPreview();
};

const openActiveIdentityVariantSetup = async () => {
  const activeIdentity = activeIdentityForEmojiPanel.value;
  if (!activeIdentity) {
    message.warning('请先选择频道角色');
    return;
  }
  if (!canManageIdentities()) {
    message.warning('当前无权限配置头像差分');
    return;
  }
  emojiPopoverShow.value = false;
  message.info('请先为当前频道角色配置头像差分');
  await openIdentityEdit(activeIdentity);
};

const handleEmojiVariantTabClick = async () => {
  if (!activeIdentityForEmojiPanel.value) {
    message.warning('请先选择频道角色');
    return;
  }
  if (!hasIdentityVariantOptions.value) {
    await openActiveIdentityVariantSetup();
    return;
  }
  switchEmojiPanelTab('variant');
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

type IdentityShortcutMatchResult = {
  matched: ChannelIdentity | null;
  restContent: string;
  ambiguous: boolean;
};

type IdentityVariantShortcutMatchResult = {
  matched: ChannelIdentityVariant | null;
  restContent: string;
  ambiguous: boolean;
  resetToDefault?: boolean;
};

type IdentityVariantMatchMode = 'prefix' | 'keyword' | 'regex';

type IdentityAppearancePreview = {
  identityId: string;
  variantId: string;
  displayName: string;
  color: string;
  avatarAttachmentId: string;
  avatarDecorations?: AvatarDecoration[] | null;
  theaterPresentation?: TheaterPresentation | null;
  isTemporary: boolean;
};

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

const cloneAvatarDecorations = (
  value?: AvatarDecoration[] | AvatarDecoration | null,
  legacyValue?: AvatarDecoration | null,
): AvatarDecoration[] => normalizeAvatarDecorations(value, legacyValue).map(item => ({
  ...item,
  settings: item.settings ? { ...item.settings } : undefined,
}))

const resolveIdentityShortcutMatch = (
  rawDraft: string,
  identities: ChannelIdentity[],
  trigger = '/',
): IdentityShortcutMatchResult | null => {
  const shortcutMatch = new RegExp(`^${escapeRegExp(trigger)}(\\S+)(?:\\s+([\\s\\S]*))?$`).exec(rawDraft);
  if (!shortcutMatch) {
    return null;
  }
  const targetNameRaw = (shortcutMatch[1] || '').trim();
  if (!targetNameRaw) {
    return null;
  }
  const restContent = shortcutMatch[2] ?? '';
  const targetName = targetNameRaw.toLowerCase();
  const normalizedCandidates = identities
    .map((item, index) => {
      const displayName = (item.displayName || '').trim();
      return {
        item,
        index,
        normalizedName: displayName.toLowerCase(),
        length: displayName.length,
      };
    })
    .filter(item => !!item.normalizedName);

  const exact = normalizedCandidates.find(item => item.normalizedName === targetName);
  if (exact) {
    return {
      matched: exact.item,
      restContent,
      ambiguous: false,
    };
  }

  const prefixCandidates = normalizedCandidates.filter(item => item.normalizedName.startsWith(targetName));
  if (!prefixCandidates.length) {
    return {
      matched: null,
      restContent,
      ambiguous: false,
    };
  }

  const sortedCandidates = prefixCandidates.slice().sort((a, b) => {
    if (a.length !== b.length) {
      return a.length - b.length;
    }
    return a.index - b.index;
  });
  const best = sortedCandidates[0];
  const hasAmbiguousShortest = sortedCandidates.some((item, index) => index > 0 && item.length === best.length && item.normalizedName !== best.normalizedName);
  if (hasAmbiguousShortest) {
    return {
      matched: null,
      restContent,
      ambiguous: true,
    };
  }

  return {
    matched: best.item,
    restContent,
    ambiguous: false,
  };
};

const shouldSuppressKeywordSuggestForIdentityShortcut = (rawDraft: string, trigger: string): boolean => {
  const channelId = chat.curChannel?.id;
  const identityTrigger = display.settings.identityQuickSwitchTrigger || '/';
  if (!channelId || trigger !== identityTrigger) {
    return false;
  }
  const identities = chat.channelIdentities[channelId] || [];
  const shortcutResult = resolveIdentityShortcutMatch(rawDraft, identities, identityTrigger);
  return !!shortcutResult?.matched || !!shortcutResult?.ambiguous;
};

const resolveIdentityVariantShortcutMatch = (
  rawDraft: string,
  identity: ChannelIdentity,
  variants: ChannelIdentityVariant[],
  legacyTrigger = '=',
): IdentityVariantShortcutMatchResult | null => {
  if (!rawDraft) {
    return null;
  }
  const normalizedDraft = rawDraft.toLowerCase();
  const resetMode = (identity.variantResetMatchMode || 'prefix') as IdentityVariantMatchMode;
  const resetContent = String(identity.variantResetMatchContent || '还原').trim() || '还原';
  const resetConfig = String(identity.variantResetMatchConfig || (resetMode === 'prefix' ? '=' : resetMode === 'keyword' ? 'any' : 'sensitive')).trim();
  if (resetMode === 'prefix') {
    const activation = `${resetConfig || legacyTrigger || '='}${resetContent}`;
    if (normalizedDraft.startsWith(activation.toLowerCase()) && (rawDraft.length === activation.length || /\s/.test(rawDraft.charAt(activation.length)))) {
      return {
        matched: null,
        restContent: rawDraft.slice(activation.length).replace(/^\s+/, ''),
        ambiguous: false,
        resetToDefault: true,
      };
    }
  } else if (resetMode === 'keyword') {
    const matchAll = resetConfig === 'all';
    const separator = matchAll ? '&' : '|';
    const keywords = resetContent.split(separator).map(keyword => keyword.trim().toLowerCase()).filter(Boolean);
    const matched = keywords.length > 0 && (matchAll
      ? keywords.every(keyword => normalizedDraft.includes(keyword))
      : keywords.some(keyword => normalizedDraft.includes(keyword)));
    if (matched) {
      return { matched: null, restContent: rawDraft, ambiguous: false, resetToDefault: true };
    }
  } else if (resetMode === 'regex') {
    try {
      if (new RegExp(resetContent, resetConfig === 'insensitive' ? 'i' : '').test(rawDraft)) {
        return { matched: null, restContent: rawDraft, ambiguous: false, resetToDefault: true };
      }
    } catch {
      // Invalid persisted reset regex does not block message sending.
    }
  }
  for (const targetMode of ['prefix', 'keyword', 'regex'] as IdentityVariantMatchMode[]) {
    for (const item of variants) {
      if (!item || item.enabled === false) {
        continue;
      }
      const content = String(item.keyword || '').trim();
      if (!content) {
        continue;
      }
      const mode = (item.matchMode || 'prefix') as IdentityVariantMatchMode;
      if (mode !== targetMode) {
        continue;
      }
      if (mode === 'prefix') {
        const symbol = String(item.matchConfig || legacyTrigger || '=').trim();
        const activation = `${symbol}${content}`;
        if (normalizedDraft.startsWith(activation.toLowerCase()) && (rawDraft.length === activation.length || /\s/.test(rawDraft.charAt(activation.length)))) {
          return { matched: item, restContent: rawDraft.slice(activation.length).replace(/^\s+/, ''), ambiguous: false };
        }
        continue;
      }
      if (mode === 'keyword') {
        const matchAll = item.matchConfig === 'all';
        const separator = matchAll ? '&' : '|';
        const keywords = content.split(separator).map(keyword => keyword.trim().toLowerCase()).filter(Boolean);
        const matched = keywords.length > 0 && (matchAll
          ? keywords.every(keyword => normalizedDraft.includes(keyword))
          : keywords.some(keyword => normalizedDraft.includes(keyword)));
        if (matched) {
          return { matched: item, restContent: rawDraft, ambiguous: false };
        }
        continue;
      }
      if (mode === 'regex') {
        try {
          if (new RegExp(content, item.matchConfig === 'insensitive' ? 'i' : '').test(rawDraft)) {
            return { matched: item, restContent: rawDraft, ambiguous: false };
          }
        } catch {
          continue;
        }
      }
    }
  }
  return { matched: null, restContent: rawDraft, ambiguous: false };
};

const resolveIdentityVariantIdFromMessage = (msg?: any): string | null => {
  if (!msg) {
    return null;
  }
  const directIdentity = msg.identity || msg.identity_info || msg.identityData;
  if (directIdentity && typeof directIdentity === 'object' && directIdentity.variantId) {
    return String(directIdentity.variantId).trim() || null;
  }
  const snake = msg?.sender_identity_variant_id;
  if (typeof snake === 'string' && snake.trim().length > 0) {
    return snake.trim();
  }
  return null;
};

const resolveIdentityAppearancePreview = (identity?: ChannelIdentity | null, variant?: ChannelIdentityVariant | null): IdentityAppearancePreview | null => {
  if (!identity) {
    return null;
  }
  const variantTheaterPresentation = variant?.theaterPresentation !== undefined
    ? variant.theaterPresentation
    : variant?.appearance?.theaterPresentation
  const hasTheaterPresentation = Boolean(identity.theaterPresentation)
    || variantTheaterPresentation !== undefined
  return {
    identityId: identity.id,
    variantId: variant?.id || '',
    displayName: variant?.displayName || identity.displayName || '',
    color: variant?.color || identity.color || '',
    avatarAttachmentId: variant?.avatarAttachmentId || identity.avatarAttachmentId || '',
    avatarDecorations: cloneAvatarDecorations(identity.avatarDecorations, identity.avatarDecoration),
    theaterPresentation: hasTheaterPresentation
      ? resolveTheaterPresentation(
          identity.theaterPresentation,
          variantTheaterPresentation && typeof variantTheaterPresentation === 'object'
            ? variantTheaterPresentation as TheaterPresentationPatch
            : null,
        )
      : null,
    isTemporary: Boolean(identity.isTemporary),
  };
};
const identityDialogMode = ref<'create' | 'edit'>('create');
const identityManageVisible = ref(false);
const icOocRoleConfigPanelVisible = ref(false);
const identitySubmitting = ref(false);
const identityDecorationEditorVisible = ref(false);
const theaterPresentationEditorVisible = ref(false);
const theaterPresentationEditorMode = ref<'base' | 'variant'>('base');
const theaterPresentationApplying = ref(false);
const worldTheaterTemplateSaving = ref(false);
const currentWorldTheaterTemplate = computed<WorldTheaterPresentationTemplate>(() => {
  const worldId = String(chat.currentWorldId || '').trim();
  if (!worldId) return {};
  return chat.worldDetailMap[worldId]?.world?.theaterPresentationTemplate
    || chat.worldMap[worldId]?.theaterPresentationTemplate
    || {};
});
const canSetWorldTheaterTemplate = computed(() => {
  const worldId = String(chat.currentWorldId || '').trim();
  if (!worldId) return false;
  const role = chat.worldDetailMap[worldId]?.memberRole;
  return role === 'owner' || role === 'admin' || Boolean(user.checkPerm?.('mod_admin'));
});
const identityForm = reactive({
  displayName: '',
  color: '',
  avatarAttachmentId: '',
  avatarDecorations: [] as AvatarDecoration[],
  theaterPresentation: null as TheaterPresentation | null,
  isDefault: false,
  isTemporary: false,
  botAppearanceMode: '' as '' | 'inherit' | 'custom',
  variantResetMatchMode: 'prefix' as IdentityVariantMatchMode,
  variantResetMatchConfig: '=',
  variantResetMatchContent: '还原',
  icOocOnActivate: '' as '' | 'ic' | 'ooc',
  folderIds: [] as string[],
  characterCardId: '' as string,
  promoteToShared: false,
});
const identityColorDraft = ref('');
const identityOriginalCardId = ref('');
const identityAvatarPreview = ref('');
const identityAvatarInputRef = ref<HTMLInputElement | null>(null);
const identityAvatarEditorVisible = ref(false);
const identityAvatarEditorFile = ref<File | null>(null);
const editingIdentity = ref<ChannelIdentity | null>(null);
const identityVariantDialogVisible = ref(false);
const identityVariantDialogMode = ref<'create' | 'edit'>('create');
const identityVariantSubmitting = ref(false);
const editingIdentityVariant = ref<ChannelIdentityVariant | null>(null);
const identityVariantEmojiPickerVisible = ref(false);
const identityVariantMatchModeOptions = [
  { label: '前缀匹配', value: 'prefix' },
  { label: '关键词匹配', value: 'keyword' },
  { label: '正则表达式匹配', value: 'regex' },
];
const identityVariantKeywordMatchOptions = [
  { label: '任一关键词', value: 'any' },
  { label: '全部关键词', value: 'all' },
];
const identityVariantRegexMatchOptions = [
  { label: '区分大小写', value: 'sensitive' },
  { label: '忽略大小写', value: 'insensitive' },
];
const identityVariantResetDialogVisible = ref(false);
const identityVariantResetForm = reactive({
  matchMode: 'prefix' as IdentityVariantMatchMode,
  matchDrafts: {
    prefix: { config: '=', content: '还原' },
    keyword: { config: 'any', content: '还原' },
    regex: { config: 'sensitive', content: '还原' },
  } as Record<IdentityVariantMatchMode, { config: string; content: string }>,
});
const activeIdentityVariantResetMatchDraft = computed(() => identityVariantResetForm.matchDrafts[identityVariantResetForm.matchMode]);
const identityVariantResetMatchContentPlaceholder = computed(() => {
  if (identityVariantResetForm.matchMode === 'prefix') return '例如：还原';
  if (identityVariantResetForm.matchMode === 'keyword') {
    return activeIdentityVariantResetMatchDraft.value.config === 'all' ? '例如：结束&恢复' : '例如：还原|恢复';
  }
  return '例如：^(还原|恢复)';
});
const identityVariantForm = reactive({
  selectorEmoji: '',
  matchMode: 'prefix' as IdentityVariantMatchMode,
  matchDrafts: {
    prefix: { config: '=', content: '' },
    keyword: { config: 'any', content: '' },
    regex: { config: 'sensitive', content: '' },
  } as Record<IdentityVariantMatchMode, { config: string; content: string }>,
  note: '',
  avatarAttachmentId: '',
  displayName: '',
  color: '',
  theaterPresentation: {} as TheaterPresentationPatch,
  enabled: true,
});
const activeIdentityVariantMatchDraft = computed(() => identityVariantForm.matchDrafts[identityVariantForm.matchMode]);
const identityVariantMatchContentPlaceholder = computed(() => {
  if (identityVariantForm.matchMode === 'prefix') return '例如：笑';
  if (identityVariantForm.matchMode === 'keyword') {
    return activeIdentityVariantMatchDraft.value.config === 'all' ? '例如：笑&挥手' : '例如：笑|开心';
  }
  return '例如：笑|开心|挥手';
});
const identityVariantColorDraft = ref('');
const identityVariantAvatarPreview = ref('');
const identityVariantAvatarInputRef = ref<HTMLInputElement | null>(null);
const identityVariantAvatarEditorVisible = ref(false);
const identityVariantAvatarEditorFile = ref<File | null>(null);
const identityManageTargetUserId = ref('');
const identityManageTargetLabel = ref('');
const identityManageTargetRoleLabel = ref('');
const identityManageTargetAvatar = ref('');
const identityManageTargetKind = ref<'self' | 'user' | 'bot'>('self');
const identityManageCandidateModalVisible = ref(false);
const identityManageCandidateKeyword = ref('');
const identityManageCandidates = ref<ChannelIdentityManageCandidate[]>([]);
const identityManageCandidatesLoading = ref(false);
const identityManageCandidateSelectedUserId = ref<string | null>(null);
const identityManageBotModalVisible = ref(false);
const identityManageBots = ref<UserInfo[]>([]);
const identityManageBotsLoading = ref(false);
const identityManageBotSelectedUserId = ref<string | null>(null);
const currentIdentityTargetUserId = computed(() => identityManageTargetUserId.value || user.info.id || '');
const isManagingBotIdentity = computed(() => identityManageTargetKind.value === 'bot');
const botBaseAppearanceInherited = computed(() => (
  isManagingBotIdentity.value && identityForm.botAppearanceMode !== 'custom'
));
const isManagingOtherUserIdentity = computed(() => (
  !!identityManageTargetUserId.value && identityManageTargetUserId.value !== (user.info.id || '')
));
const currentManagedIdentityLabel = computed(() => {
  if (!isManagingOtherUserIdentity.value) {
    return user.info.nick || user.info.username || '自己';
  }
  return identityManageTargetLabel.value || identityManageTargetUserId.value;
});
const currentChannelIdentities = computed(() => (
  isManagingBotIdentity.value
    ? chat.getScopedChannelIdentities(chat.curChannel?.id || '', currentIdentityTargetUserId.value).filter(item => item.isDefault).slice(0, 1)
    : chat.getScopedChannelIdentities(chat.curChannel?.id || '', currentIdentityTargetUserId.value)
));
const managedIdentityFallbackAvatar = computed(() => (
  resolveAttachmentUrl(identityManageTargetAvatar.value) || user.info.avatar || ''
));
const currentEditingIdentityVariants = computed(() => {
  const channelId = chat.curChannel?.id || '';
  const identityId = editingIdentity.value?.id || '';
  if (!channelId || !identityId) {
    return [] as ChannelIdentityVariant[];
  }
  return chat.getIdentityVariants(channelId, identityId, currentIdentityTargetUserId.value);
});
const identityFolders = computed(() => (
  chat.getScopedChannelIdentityFolders(chat.curChannel?.id || '', currentIdentityTargetUserId.value)
));
const identityFavoriteFolderIds = computed(() => (
  chat.getScopedChannelIdentityFavorites(chat.curChannel?.id || '', currentIdentityTargetUserId.value)
));
const identityFolderMembership = computed<Record<string, string[]>>(() => (
  chat.getScopedChannelIdentityMembership(chat.curChannel?.id || '', currentIdentityTargetUserId.value)
));
const isEditingTemporaryIdentity = computed(() => identityDialogMode.value === 'edit' && Boolean(editingIdentity.value?.isTemporary));
const isDelegatedSharedIdentity = computed(() => (
  isManagingOtherUserIdentity.value && Boolean(editingIdentity.value?.sharedIdentityId)
));
const sharedSynchronizedFieldsDisabled = computed(() => botBaseAppearanceInherited.value || isDelegatedSharedIdentity.value);
const canPromoteEditingIdentityToShared = computed(() => (
  identityDialogMode.value === 'edit'
  && Boolean(editingIdentity.value)
  && !editingIdentity.value?.sharedIdentityId
  && !isEditingTemporaryIdentity.value
  && !isManagingBotIdentity.value
  && !isManagingOtherUserIdentity.value
));
const identityDialogTitle = computed(() => {
  if (isManagingBotIdentity.value) {
    return '编辑 BOT 频道外观';
  }
  if (identityDialogMode.value === 'create') {
    return '创建频道角色';
  }
  return isEditingTemporaryIdentity.value ? '编辑临时频道角色' : '编辑频道角色';
});
const identityDialogSubmitText = computed(() => (
  isEditingTemporaryIdentity.value ? '保存并生成新身份' : '保存'
));
const identityTemporaryHint = computed(() => (
  isEditingTemporaryIdentity.value
    ? '改名会生成新的频道角色 ID，历史消息保留旧身份；头像差分与已绑人物卡不会自动迁移。'
    : ''
));
const temporaryIdentityActivateModeLabel = computed(() => (
  identityForm.icOocOnActivate === 'ooc' ? '场外' : '场内'
));
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
let identityVariantAvatarObjectURL: string | null = null;
let identityVariantAvatarFile: File | null = null;

const resolveVariantSelectorEmojiSrc = (selectorEmoji?: string) => {
  const raw = String(selectorEmoji || '').trim();
  if (!raw.startsWith('id:')) {
    return '';
  }
  return resolveAttachmentUrl(raw);
};

const isVariantSelectorEmojiAttachment = (selectorEmoji?: string) => !!resolveVariantSelectorEmojiSrc(selectorEmoji);

const resolveVariantNote = (variant?: ChannelIdentityVariant | null) => {
  const note = String(variant?.note || '').trim();
  if (note) {
    return note;
  }
  const keyword = String(variant?.keyword || '').trim();
  if (keyword) {
    return `=${keyword}`;
  }
  return '未备注';
};

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
    await chat.toggleChannelIdentityFolderFavorite(folder.id, chat.curChannel.id, next, currentIdentityTargetUserId.value);
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
      await chat.createChannelIdentityFolder(chat.curChannel.id, name, undefined, currentIdentityTargetUserId.value);
      message.success('文件夹已创建');
    } else if (editingFolder.value) {
      await chat.updateChannelIdentityFolder(editingFolder.value.id, chat.curChannel.id, { name, targetUserId: currentIdentityTargetUserId.value });
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
      await chat.deleteChannelIdentityFolder(folder.id, chat.curChannel.id, currentIdentityTargetUserId.value);
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
    await chat.assignIdentitiesToFolders(chat.curChannel.id, identitySelection.value, folderActionTarget.value, mode, currentIdentityTargetUserId.value);
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
    await chat.assignIdentitiesToFolders(chat.curChannel.id, identitySelection.value, [], 'replace', currentIdentityTargetUserId.value);
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
  resetIdentityManageTarget();
});

watch(() => chat.currentWorldId, () => {
  resetIdentityManageTarget();
});

watch(identityManageVisible, (visible) => {
  if (!visible) {
    identitySelection.value = [];
    folderActionTarget.value = [];
    resetIdentityManageTarget();
  }
});

watch(identityManageCandidateKeyword, useDebounceFn(async (value: string) => {
  if (!identityManageCandidateModalVisible.value) {
    return;
  }
  try {
    await loadIdentityManageCandidates(value);
  } catch (error) {
    console.warn('加载代管候选用户失败', error);
  }
}, 250));

watch(identityDialogVisible, (visible) => {
  if (!visible) {
    identityVariantDialogVisible.value = false;
    identityVariantEmojiPickerVisible.value = false;
    editingIdentityVariant.value = null;
    revokeIdentityVariantObjectURL();
  }
});

watch(identityVariantDialogVisible, (visible) => {
  if (!visible) {
    identityVariantEmojiPickerVisible.value = false;
    editingIdentityVariant.value = null;
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

const canManageOtherUserIdentities = computed(() => {
  if (isInObserverMode()) return false;
  const worldId = chat.currentWorldId;
  if (!worldId) return false;
  const detail = chat.worldDetailMap[worldId];
  const enabled = detail?.allowManageOtherUserChannelIdentities || detail?.world?.allowManageOtherUserChannelIdentities;
  if (!enabled) return false;
  return Boolean(detail?.memberRole);
});

const resetIdentityManageTarget = () => {
  identityManageTargetUserId.value = '';
  identityManageTargetLabel.value = '';
  identityManageTargetRoleLabel.value = '';
  identityManageTargetAvatar.value = '';
  identityManageTargetKind.value = 'self';
  identityManageCandidateSelectedUserId.value = null;
  identityManageCandidateKeyword.value = '';
  identityManageCandidates.value = [];
  identityManageBotSelectedUserId.value = null;
  identityManageBots.value = [];
};

const loadIdentityManageCandidates = async (keyword = '') => {
  if (!chat.curChannel?.id) {
    identityManageCandidates.value = [];
    return;
  }
  identityManageCandidatesLoading.value = true;
  try {
    const resp = await chat.channelIdentityManageCandidates(chat.curChannel.id, {
      keyword,
      page: 1,
      pageSize: 50,
    });
    identityManageCandidates.value = resp.items || [];
  } finally {
    identityManageCandidatesLoading.value = false;
  }
};

const openIdentityManageUserDialog = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  if (!canManageOtherUserIdentities.value) {
    message.warning('当前世界未开启该功能或你没有可管理的目标');
    return;
  }
  identityManageCandidateSelectedUserId.value = currentIdentityTargetUserId.value || user.info.id || null;
  identityManageCandidateKeyword.value = '';
  identityManageCandidateModalVisible.value = true;
  try {
    await loadIdentityManageCandidates('');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '加载候选用户失败');
  }
};

const openIdentityManageBotDialog = async () => {
  const channelId = chat.curChannel?.id || '';
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  if (!canManageChannelFeatures.value) {
    message.warning('你没有设置当前频道 BOT 外观的权限');
    return;
  }
  identityManageBotModalVisible.value = true;
  identityManageBotsLoading.value = true;
  identityManageBotSelectedUserId.value = isManagingBotIdentity.value ? currentIdentityTargetUserId.value : null;
  try {
    const resp = await chat.channelMemberListAll(channelId, 200);
    const roleId = `ch-${channelId}-bot`;
    const found = (resp?.data?.items || [])
      .filter(item => item.roleId === roleId && Boolean(item.user?.id))
      .map(item => item.user!);
    identityManageBots.value = Array.from(new Map(found.map(item => [item.id, item])).values());
    if (identityManageBots.value.length === 1) {
      identityManageBotSelectedUserId.value = identityManageBots.value[0].id;
    }
  } catch (error: any) {
    message.error(error?.response?.data?.error || '加载频道 BOT 失败');
  } finally {
    identityManageBotsLoading.value = false;
  }
};

const confirmIdentityManageBot = async () => {
  const channelId = chat.curChannel?.id || '';
  const targetUserId = identityManageBotSelectedUserId.value || '';
  if (!channelId || !targetUserId) {
    message.warning('请选择 BOT');
    return;
  }
  const selected = identityManageBots.value.find(item => item.id === targetUserId);
  identityManageTargetUserId.value = targetUserId;
  identityManageTargetLabel.value = selected?.nick || selected?.username || targetUserId;
  identityManageTargetRoleLabel.value = 'BOT';
  identityManageTargetAvatar.value = selected?.avatar || '';
  identityManageTargetKind.value = 'bot';
  identityManageBotModalVisible.value = false;
  await chat.loadChannelIdentities(channelId, true, targetUserId);
  identityManageVisible.value = true;
};

const confirmIdentityManageUser = async () => {
  if (!chat.curChannel?.id) {
    return;
  }
  const targetUserId = identityManageCandidateSelectedUserId.value || '';
  if (!targetUserId) {
    message.warning('请选择用户');
    return;
  }
  const selected = identityManageCandidates.value.find(item => item.userId === targetUserId);
  identityManageTargetUserId.value = targetUserId === (user.info.id || '') ? '' : targetUserId;
  identityManageTargetLabel.value = selected?.nickname || selected?.username || targetUserId;
  identityManageTargetRoleLabel.value = selected?.roleLabel || '';
  identityManageTargetAvatar.value = selected?.avatar || '';
  identityManageTargetKind.value = targetUserId === (user.info.id || '') ? 'self' : 'user';
  identityManageCandidateModalVisible.value = false;
  await chat.loadChannelIdentities(chat.curChannel.id, true, identityManageTargetUserId.value || undefined);
  identityManageVisible.value = true;
};

const exitIdentityManageUserMode = async () => {
  const channelId = chat.curChannel?.id;
  resetIdentityManageTarget();
  if (channelId) {
    await chat.loadChannelIdentities(channelId, false);
  }
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
    const lastLoadedAt = chat.channelIdentityLoadedAt?.[channelId] || 0;
    const shouldForce = !lastLoadedAt || Date.now() - lastLoadedAt > 15000;
    await chat.loadChannelIdentities(channelId, shouldForce);
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
    let settled = false;
    const settle = (value: boolean) => {
      if (settled) return;
      settled = true;
      resolve(value);
    };
    dialog.warning({
      title: '从其他频道同步角色？',
      content: '当前频道角色较少且场内/场外未完整配置，是否从本世界其他频道同步？',
      positiveText: '同步',
      negativeText: '暂不',
      maskClosable: false,
      closeOnEsc: false,
      closable: false,
      onPositiveClick: () => settle(true),
      onNegativeClick: () => settle(false),
      onClose: () => settle(false),
    });
  });
  identitySyncPromptPending.value = false;
  if (!confirmed) {
    identitySyncDismissedForSession.value = true;
    return;
  }
  try {
    await openIdentityManager();
  } catch (error) {
    console.warn('打开角色管理失败', error);
    message.error('打开角色管理失败，请稍后重试');
    return;
  }
  await nextTick();
  await openIdentitySyncDialog();
};

const IDENTITY_EXPORT_VERSION = 'sealchat.channel-identity/v6';
const IDENTITY_EXPORT_COMPATIBLE_VERSIONS = [
  'sealchat.channel-identity/v1',
  'sealchat.channel-identity/v2',
  'sealchat.channel-identity/v3',
  'sealchat.channel-identity/v4',
  'sealchat.channel-identity/v5',
  'sealchat.channel-identity/v6',
];

interface IdentityAssetExportIssueState {
  missingAssets: number;
}

const safeFilename = (value: string) => (value || 'channel').replace(/[\\/:*?"<>|]/g, '_');

// Theater settings come from Vue reactive state; structuredClone rejects Proxy values.
const cloneIdentityMigrationValue = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

const downloadAttachmentAsPayload = async (
  attachmentId: string,
  fallbackFilename: string,
  issueState?: IdentityAssetExportIssueState,
): Promise<IdentityAvatarPayload | null> => {
  const normalizedId = normalizeAttachmentId(attachmentId);
  if (!normalizedId) {
    return null;
  }
  const meta = await fetchAttachmentMetaById(normalizedId);
  if (!meta) {
    return null;
  }
  if (meta.storageType === 's3') {
    const sourceUrl = normalizedId
      ? `${String(urlBase || '').replace(/\/$/, '')}/api/v1/attachment/${normalizedId}`
      : '';
    if (!sourceUrl) {
      issueState && (issueState.missingAssets += 1);
      console.warn('导出角色素材时无法生成平台附件链接，已跳过', {
        attachmentId: normalizedId,
      });
      return null;
    }
    return {
      attachmentId: normalizedId,
      hash: meta.hash || '',
      size: meta.size ?? 0,
      filename: meta.filename || fallbackFilename,
      mimeType: meta.mimeType || 'application/octet-stream',
      sourceUrl,
    };
  }
  const downloadUrl = resolveIdentityAssetFetchUrl({
    normalizedId,
    externalUrl: meta.externalUrl,
    publicUrl: meta.publicUrl,
    urlBase,
  });
  const resp = await fetch(downloadUrl, {
    headers: { Authorization: user.token || '' },
  });
  if (!resp.ok) {
    if (shouldIgnoreIdentityAssetFetchStatus(resp.status)) {
      issueState && (issueState.missingAssets += 1);
      console.warn('导出角色素材时发现缺失附件，已跳过', {
        attachmentId: normalizedId,
        status: resp.status,
      });
      return null;
    }
    throw new Error(`下载附件失败：${resp.status} ${resp.statusText}`);
  }
  const buffer = await resp.arrayBuffer();
  return {
    attachmentId: normalizedId,
    hash: meta.hash || '',
    size: meta.size ?? buffer.byteLength,
    filename: meta.filename || fallbackFilename,
    mimeType: resp.headers.get('content-type') || meta.mimeType || 'application/octet-stream',
    data: arrayBufferToBase64(buffer),
  };
};

const registerIdentityAsset = async (
  assetMap: Map<string, IdentityAssetPayload>,
  attachmentId?: string | null,
  fallbackFilename = 'identity-asset',
  issueState?: IdentityAssetExportIssueState,
) => {
  const normalizedId = normalizeAttachmentId(String(attachmentId || ''));
  if (!normalizedId) {
    return '';
  }
  const meta = await fetchAttachmentMetaById(normalizedId);
  const payload = await downloadAttachmentAsPayload(normalizedId, fallbackFilename, issueState);
  if (!payload) {
    return '';
  }
  const assetKey = buildIdentityAssetKey({
    hash: payload.hash || meta?.hash || '',
    size: payload.size || meta?.size || 0,
  }, normalizedId);
  if (!assetMap.has(assetKey)) {
    assetMap.set(assetKey, {
      assetKey,
      ...payload,
    });
  }
  return assetKey;
};

const mapTheaterPresentationForExport = async (
  input: Record<string, any> | null | undefined,
  _assetMap: Map<string, IdentityAssetPayload>,
  _fallbackPrefix: string,
  _issueState?: IdentityAssetExportIssueState,
) => {
  if (!input) return null;
  return cloneIdentityMigrationValue(input);
};

const remapTheaterPresentationForImport = async (
  input: Record<string, any> | null | undefined,
  options: { channelId: string; identityId: string; variantId?: string | null; targetUserId?: string | null; assetIdMap: Map<string, string>; mediaCache?: Map<string, any> },
) => {
  void options;
  return input == null ? input ?? null : cloneIdentityMigrationValue(input);
};

const mapDecorationsForExport = async (
  decorations: AvatarDecoration[] | null | undefined,
  assetMap: Map<string, IdentityAssetPayload>,
  fallbackPrefix: string,
  issueState?: IdentityAssetExportIssueState,
): Promise<IdentityExportDecorationItem[]> => {
  const normalized = cloneAvatarDecorations(decorations);
  const result: IdentityExportDecorationItem[] = [];
  for (const item of normalized) {
    const resourceAssetKey = await registerIdentityAsset(
      assetMap,
      item.resourceAttachmentId,
      `${fallbackPrefix}-resource`,
      issueState,
    );
    const fallbackAssetKey = await registerIdentityAsset(
      assetMap,
      item.fallbackAttachmentId,
      `${fallbackPrefix}-fallback`,
      issueState,
    );
    if (item.resourceAttachmentId && !resourceAssetKey) {
      throw new Error(`头像装饰资源无法导出: ${fallbackPrefix}`);
    }
    if (item.fallbackAttachmentId && !fallbackAssetKey) {
      throw new Error(`头像装饰兜底资源无法导出: ${fallbackPrefix}`);
    }
    const exported: IdentityExportDecorationItem = {
      ...item,
      resourceAssetKey: resourceAssetKey || undefined,
      fallbackAssetKey: fallbackAssetKey || undefined,
    };
    delete exported.resourceAttachmentId;
    delete exported.fallbackAttachmentId;
    result.push(exported);
  }
  return result;
};

const normalizeExportItemDecorations = (item: IdentityExportItem) => {
  if (Array.isArray(item.avatarDecorations) && item.avatarDecorations.length > 0) {
    return item.avatarDecorations;
  }
  if (item.avatarDecoration) {
    return [item.avatarDecoration];
  }
  return [] as IdentityExportDecorationItem[];
};

const importAttachmentFromRemoteUrl = async (
  avatar: IdentityAvatarPayload,
  options?: { channelId?: string; targetUserId?: string | null },
): Promise<string> => {
  const transferUrl = resolveIdentityAssetTransferUrl(avatar);
  if (!transferUrl) {
    return '';
  }
  const resp = await api.post('api/v1/attachment-import-from-url', {
    url: transferUrl,
    filename: avatar.filename,
    contentType: avatar.mimeType,
    channelId: options?.channelId || chat.curChannel?.id,
    targetUserId: options?.targetUserId || undefined,
  });
  return normalizeAttachmentId(resp.data?.file?.id || '');
};

const ensureImportAttachment = async (
  avatar?: IdentityAvatarPayload | null,
  options?: { channelId?: string; targetUserId?: string | null },
): Promise<string> => {
  if (!avatar) {
    return '';
  }
  if (avatar.hash && avatar.size) {
    try {
      const quickResp = await api.post('api/v1/attachment-upload-quick', {
        hash: avatar.hash,
        size: avatar.size,
        extra: 'channel-identity-avatar',
        channelId: options?.channelId || chat.curChannel?.id,
        targetUserId: options?.targetUserId || undefined,
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
  }

  if (shouldUseIdentityAssetRemoteImport(avatar)) {
    try {
      const importedId = await importAttachmentFromRemoteUrl(avatar, options);
      if (importedId) {
        return importedId;
      }
    } catch (error) {
      console.warn('远端 URL 导入身份素材失败，准备回退', error);
      if (!avatar.data) {
        throw error;
      }
    }
  }

  if (!avatar.hash || !avatar.data || !avatar.size) {
    if (avatar.attachmentId) {
      throw new Error('角色素材仅含源附件 ID，无法安全导入到目标频道');
    }
    throw new Error('角色素材缺少可重建数据');
  }

  try {
    const bytes = base64ToUint8Array(avatar.data);
    const blob = new Blob([bytes], { type: avatar.mimeType || 'application/octet-stream' });
    const fileName = avatar.filename || `identity-avatar-${avatar.hash.slice(0, 8)}`;
    const file = new File([blob], fileName, { type: avatar.mimeType || 'application/octet-stream' });
    const uploadResult = await uploadImageAttachment(file, {
      channelId: options?.channelId || chat.curChannel?.id,
      targetUserId: options?.targetUserId,
      skipCompression: true,
    });
    return normalizeAttachmentId(uploadResult.attachmentId);
  } catch (error) {
    console.error('上传身份头像失败', error);
    throw error;
  }
};

const ensureImportAssets = async (
  assets?: IdentityAssetPayload[],
  options?: { channelId?: string; targetUserId?: string | null },
) => {
  const result = new Map<string, string>();
  for (const asset of assets || []) {
    if (!asset?.assetKey) {
      continue;
    }
    const attachmentId = await ensureImportAttachment({
      attachmentId: asset.attachmentId,
      hash: asset.hash,
      size: asset.size,
      filename: asset.filename,
      mimeType: asset.mimeType,
      data: asset.data,
      sourceUrl: asset.sourceUrl,
      externalUrl: asset.externalUrl,
      publicUrl: asset.publicUrl,
    }, options);
    if (attachmentId) {
      result.set(asset.assetKey, attachmentId);
    } else {
      throw new Error(`角色素材无法重建: ${asset.assetKey}`);
    }
  }
  return result;
};

const buildIdentityExportSnapshot = async (options: {
  channelId: string;
  channelName?: string;
  guildId?: string;
  identities: ChannelIdentity[];
  folders: ChannelIdentityFolder[];
  favoriteSet: Set<string>;
  membershipMap: Record<string, string[]>;
  variantsByIdentity: Record<string, ChannelIdentityVariant[]>;
  icOocConfig: {
    icRoleId?: string | null;
    oocRoleId?: string | null;
  };
}) => {
  const assetMap = new Map<string, IdentityAssetPayload>();
  const issueState: IdentityAssetExportIssueState = { missingAssets: 0 };
  const items: IdentityExportItem[] = [];
  const variants: IdentityExportVariantItem[] = [];

  for (const identity of options.identities) {
    const displayName = identity.displayName || '';
    const folderIds = identity.folderIds?.length ? identity.folderIds : (options.membershipMap[identity.id] || []);
    const avatarAssetKey = await registerIdentityAsset(
      assetMap,
      identity.avatarAttachmentId,
      `${safeFilename(displayName || 'identity')}.png`,
      issueState,
    );
    if (identity.avatarAttachmentId && !avatarAssetKey) {
      throw new Error(`角色头像无法导出: ${displayName || identity.id}`);
    }
    const avatarDecorations = await mapDecorationsForExport(
      cloneAvatarDecorations(identity.avatarDecorations, identity.avatarDecoration),
      assetMap,
      safeFilename(displayName || identity.id || 'identity-decoration'),
      issueState,
    );
    const theaterPresentation = await mapTheaterPresentationForExport(
      identity.theaterPresentation as Record<string, any> | null | undefined,
      assetMap,
      safeFilename(displayName || identity.id || 'identity-theater'),
      issueState,
    );
    items.push({
      sourceId: identity.id,
      displayName,
      color: identity.color,
      isDefault: identity.isDefault,
      sortOrder: identity.sortOrder,
      folderIds: folderIds.length ? [...folderIds] : undefined,
      avatarAssetKey: avatarAssetKey || undefined,
      avatarDecorations,
      theaterPresentation,
    });

    const identityVariants = (options.variantsByIdentity[identity.id] || [])
      .slice()
      .sort((a, b) => a.sortOrder - b.sortOrder);
    for (const variant of identityVariants) {
      const avatarVariantAssetKey = await registerIdentityAsset(
        assetMap,
        variant.avatarAttachmentId,
        `${safeFilename(variant.keyword || variant.displayName || displayName || 'variant')}.png`,
        issueState,
      );
      if (variant.avatarAttachmentId && !avatarVariantAssetKey) {
        throw new Error(`差分头像无法导出: ${variant.keyword || variant.id}`);
      }
      const theaterPresentation = await mapTheaterPresentationForExport(
        resolveIdentityExportVariantTheaterPresentation(variant),
        assetMap,
        safeFilename(variant.keyword || variant.id || 'variant-theater'),
        issueState,
      );
      variants.push({
        sourceId: variant.id,
        identitySourceId: identity.id,
        selectorEmoji: variant.selectorEmoji,
        keyword: variant.keyword,
        matchMode: variant.matchMode || 'prefix',
        matchConfig: variant.matchConfig || '=',
        note: variant.note,
        avatarAssetKey: avatarVariantAssetKey || undefined,
        displayName: variant.displayName,
        color: variant.color,
        appearance: variant.appearance || {},
        theaterPresentation,
        sortOrder: variant.sortOrder,
        enabled: variant.enabled !== false,
      });
    }
  }

  return {
    payload: {
      version: IDENTITY_EXPORT_VERSION,
      generatedAt: new Date().toISOString(),
      source: {
        channelId: options.channelId,
        channelName: options.channelName || '',
        guildId: options.guildId || '',
      },
      items,
      folders: options.folders.map(folder => ({
        sourceId: folder.id,
        name: folder.name,
        sortOrder: folder.sortOrder,
        isFavorite: options.favoriteSet.has(folder.id),
      })) as IdentityExportFolder[],
      variants,
      icOocConfig: options.icOocConfig.icRoleId || options.icOocConfig.oocRoleId
        ? {
            icRoleId: options.icOocConfig.icRoleId,
            oocRoleId: options.icOocConfig.oocRoleId,
          }
        : undefined,
      assets: Array.from(assetMap.values()),
    } as IdentityExportFile,
    missingAssetCount: issueState.missingAssets,
  };
};

const buildIdentityMigrationSnapshotFromChannel = async (
  sourceChannelId: string,
  targetUserId?: string | null,
) => {
  const identities = await chat.loadChannelIdentities(sourceChannelId, true, targetUserId);
  const variantsByIdentity = await chat.loadChannelIdentityVariants(sourceChannelId, true, targetUserId);
  const scopedIdentities = Array.isArray(identities) ? identities : [];
  const sourceFolders = chat.getScopedChannelIdentityFolders(sourceChannelId, targetUserId);
  const sourceFavorites = new Set(chat.getScopedChannelIdentityFavorites(sourceChannelId, targetUserId));
  const sourceMembership = chat.getScopedChannelIdentityMembership(sourceChannelId, targetUserId);
  const sourceChannel = chat.findChannelById(sourceChannelId);
  return await buildIdentityExportSnapshot({
    channelId: sourceChannelId,
    channelName: sourceChannel?.name || '',
    guildId: (sourceChannel as any)?.guildId || '',
    identities: scopedIdentities,
    folders: sourceFolders,
    favoriteSet: sourceFavorites,
    membershipMap: sourceMembership,
    variantsByIdentity,
    icOocConfig: chat.getChannelIcOocRoleConfig(sourceChannelId, targetUserId),
  });
};

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
  const favoriteSet = new Set<string>(identityFavoriteFolderIds.value);
  const scopedIcOocConfig = chat.getChannelIcOocRoleConfig(chat.curChannel.id, currentIdentityTargetUserId.value);
  identityExporting.value = true;
  try {
    const variantsByIdentity = await chat.loadChannelIdentityVariants(chat.curChannel.id, true, currentIdentityTargetUserId.value);
    const snapshot = await buildIdentityExportSnapshot({
      channelId: chat.curChannel.id,
      channelName: chat.curChannel?.name || '',
      guildId: (chat.curChannel as any)?.guildId || '',
      identities,
      folders: folderList,
      favoriteSet,
      membershipMap,
      variantsByIdentity,
      icOocConfig: scopedIcOocConfig,
    });
    const payload = snapshot.payload;

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
    if (snapshot.missingAssetCount > 0) {
      message.warning(`有 ${snapshot.missingAssetCount} 个缺失素材已跳过`);
    }
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

const importIdentityMigrationSnapshot = async (
  payload: IdentityExportFile,
  options: {
    targetChannelId: string;
    mode: 'append' | 'overwrite';
    targetUserId?: string | null;
  },
) => {
  const targetChannelId = options.targetChannelId;
  const targetUserId = options.targetUserId;
  const items = payload.items || [];
  const assetIdMap = await ensureImportAssets(payload.assets, { channelId: targetChannelId, targetUserId });
  const theaterMediaCache = new Map<string, any>();
  const folderIdMap = new Map<string, string>();

  for (const item of items) {
    if (item.avatarAssetKey && !assetIdMap.has(item.avatarAssetKey)) {
      throw new Error(`角色头像资源文件缺失: ${item.avatarAssetKey}`);
    }
    if (item.avatar && !item.avatar.data && !resolveIdentityAssetTransferUrl(item.avatar)) {
      throw new Error(`旧版角色头像缺少可重建数据: ${item.displayName || item.sourceId}`);
    }
    remapDecorationsForImport(normalizeExportItemDecorations(item), assetIdMap);
  }
  for (const variant of payload.variants || []) {
    if (variant.avatarAssetKey && !assetIdMap.has(variant.avatarAssetKey)) {
      throw new Error(`差分头像资源文件缺失: ${variant.avatarAssetKey}`);
    }
  }

  const targetIdentities = await chat.loadChannelIdentities(targetChannelId, true, targetUserId);
  await chat.loadChannelIdentityVariants(targetChannelId, true, targetUserId);
  const targetFolders = chat.getScopedChannelIdentityFolders(targetChannelId, targetUserId);

  if (Array.isArray(payload.folders) && payload.folders.length) {
    const sortedFolders = payload.folders.slice().sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0));
    for (const folder of sortedFolders) {
      if (!folder?.name) continue;
      try {
        const existing = targetFolders.find(item => String(item.name || '').trim().toLowerCase() === String(folder.name || '').trim().toLowerCase());
        const created = existing || await chat.createChannelIdentityFolder(targetChannelId, folder.name, folder.sortOrder, targetUserId);
        if (folder.sourceId) {
          folderIdMap.set(folder.sourceId, created.id);
        }
        if (folder.isFavorite) {
          await chat.toggleChannelIdentityFolderFavorite(created.id, targetChannelId, true, targetUserId);
        }
      } catch (error) {
        console.warn('导入文件夹失败', error);
      }
    }
  }

  const targetList = Array.isArray(targetIdentities) ? [...targetIdentities] : [];
  const duplicateTargetNames = new Set<string>();
  const seenTargetNames = new Set<string>();
  for (const identity of targetList) {
    const normalizedName = String(identity.displayName || '').trim().toLowerCase();
    if (!normalizedName) continue;
    if (seenTargetNames.has(normalizedName)) {
      duplicateTargetNames.add(normalizedName);
      continue;
    }
    seenTargetNames.add(normalizedName);
  }

  let createdCount = 0;
  let updatedCount = 0;
  let skippedCount = 0;
  let failedCount = 0;
  let emptyNameCount = 0;
  let mappingChanged = false;
  const identityIdMap = new Map<string, string>();
  const overwriteIdentityIds = new Set<string>();
  const overwriteIdentitySnapshots = new Map<string, ChannelIdentity>();

  for (const item of items) {
    const displayName = String(item.displayName || '').trim();
    if (!displayName) {
      emptyNameCount += 1;
      continue;
    }
    const matchedIdentity = resolveIdentityMatchByName(targetList, displayName);
    const mappedFolderIds = (item.folderIds || [])
      .map(id => folderIdMap.get(id) || '')
      .filter((id): id is string => !!id);
    const avatarId = item.avatarAssetKey
      ? (assetIdMap.get(item.avatarAssetKey) || '')
      : await ensureImportAttachment(item.avatar, { channelId: targetChannelId, targetUserId });
    const avatarDecorations = remapDecorationsForImport(normalizeExportItemDecorations(item), assetIdMap) as AvatarDecoration[];

    try {
      if (matchedIdentity && options.mode === 'append') {
        skippedCount += 1;
        continue;
      }

      if (matchedIdentity && options.mode === 'overwrite') {
        overwriteIdentitySnapshots.set(item.sourceId, cloneIdentityMigrationValue(matchedIdentity));
        const theaterPresentation = item.theaterPresentation === undefined
          ? undefined
          : await remapTheaterPresentationForImport(item.theaterPresentation, {
              channelId: targetChannelId,
              identityId: matchedIdentity.id,
              targetUserId,
              assetIdMap,
              mediaCache: theaterMediaCache,
            });
        const updated = await chat.channelIdentityUpdate(matchedIdentity.id, {
          channelId: targetChannelId,
          targetUserId: targetUserId || undefined,
          displayName,
          color: item.color || '',
          avatarAttachmentId: avatarId,
          avatarDecorations,
          theaterPresentation: theaterPresentation as any,
          skipTheaterAssetValidation: true,
          isDefault: !!item.isDefault,
          folderIds: mappedFolderIds,
        });
        identityIdMap.set(item.sourceId, updated.id);
        overwriteIdentityIds.add(updated.id);
        const targetIndex = targetList.findIndex(existing => existing.id === updated.id);
        if (targetIndex >= 0) {
          targetList.splice(targetIndex, 1, updated);
        }
        updatedCount += 1;
        continue;
      }

      const created = await chat.channelIdentityCreate({
        channelId: targetChannelId,
        targetUserId: targetUserId || undefined,
        displayName,
        color: item.color || '',
        avatarAttachmentId: avatarId,
        avatarDecorations,
        isDefault: !!item.isDefault,
        folderIds: mappedFolderIds,
      });
      if (item.theaterPresentation !== undefined) {
        const theaterPresentation = await remapTheaterPresentationForImport(item.theaterPresentation, {
          channelId: targetChannelId,
          identityId: created.id,
          targetUserId,
          assetIdMap,
          mediaCache: theaterMediaCache,
        });
        await chat.channelIdentityUpdate(created.id, {
          channelId: targetChannelId,
          targetUserId: targetUserId || undefined,
          displayName,
          color: item.color || '',
          avatarAttachmentId: avatarId,
          avatarDecorations,
          theaterPresentation: theaterPresentation as any,
          skipTheaterAssetValidation: true,
          isDefault: !!item.isDefault,
          folderIds: mappedFolderIds,
        });
        created.theaterPresentation = theaterPresentation as any;
      }
      identityIdMap.set(item.sourceId, created.id);
      targetList.push(created);
      createdCount += 1;
    } catch (error) {
      failedCount += 1;
      console.warn('导入单个角色失败', error);
    }
  }

  const variantsByIdentity = new Map<string, IdentityExportVariantItem[]>();
  for (const variant of payload.variants || []) {
    const sourceIdentityId = String(variant.identitySourceId || '').trim();
    if (!sourceIdentityId) {
      continue;
    }
    const list = variantsByIdentity.get(sourceIdentityId) || [];
    list.push(variant);
    variantsByIdentity.set(sourceIdentityId, list);
  }

  const sourceIdentityIds = new Set<string>(items.map(item => String(item.sourceId || '')).filter(Boolean));
  for (const sourceIdentityId of sourceIdentityIds) {
    const variants = variantsByIdentity.get(sourceIdentityId) || [];
    const targetIdentityId = identityIdMap.get(sourceIdentityId);
    if (!targetIdentityId) {
      continue;
    }

    const sortedVariants = variants.slice().sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0));
    const overwriteExisting = options.mode === 'overwrite' && overwriteIdentityIds.has(targetIdentityId);
    const oldVariants = overwriteExisting
      ? chat.getIdentityVariants(targetChannelId, targetIdentityId, targetUserId).slice()
      : [];
    const stagedVariants: Array<{ item: IdentityExportVariantItem; id: string; temporaryKeyword: string }> = [];
    try {
      for (const variant of sortedVariants) {
        const temporaryKeyword = overwriteExisting ? `import-${nanoid(16)}` : variant.keyword;
        const created = await chat.channelIdentityVariantCreate({
          channelId: targetChannelId,
          targetUserId: targetUserId || undefined,
          identityId: targetIdentityId,
          selectorEmoji: variant.selectorEmoji,
          keyword: temporaryKeyword,
          matchMode: variant.matchMode || 'prefix',
          matchConfig: variant.matchConfig || '=',
          note: variant.note || '',
          avatarAttachmentId: variant.avatarAssetKey ? (assetIdMap.get(variant.avatarAssetKey) || '') : '',
          displayName: variant.displayName || '',
          color: variant.color || '',
          appearance: variant.appearance || {},
          enabled: variant.enabled !== false,
        });
        stagedVariants.push({ item: variant, id: created.id, temporaryKeyword });
        if (variant.theaterPresentation !== undefined) {
          const theaterPresentation = await remapTheaterPresentationForImport(variant.theaterPresentation, {
            channelId: targetChannelId,
            identityId: targetIdentityId,
            variantId: created.id,
            targetUserId,
            assetIdMap,
            mediaCache: theaterMediaCache,
          });
          await chat.channelIdentityVariantUpdate(created.id, {
            channelId: targetChannelId,
            targetUserId: targetUserId || undefined,
            identityId: targetIdentityId,
            selectorEmoji: variant.selectorEmoji,
            keyword: temporaryKeyword,
            matchMode: variant.matchMode || 'prefix',
            matchConfig: variant.matchConfig || '=',
            note: variant.note || '',
            avatarAttachmentId: variant.avatarAssetKey ? (assetIdMap.get(variant.avatarAssetKey) || '') : '',
            displayName: variant.displayName || '',
            color: variant.color || '',
            appearance: variant.appearance || {},
            theaterPresentation: theaterPresentation as any,
            skipTheaterAssetValidation: true,
            enabled: variant.enabled !== false,
          });
        }
      }

      if (overwriteExisting) {
        for (const oldVariant of oldVariants) {
          await chat.channelIdentityVariantDelete(targetChannelId, oldVariant.id, targetUserId);
        }
        for (const staged of stagedVariants) {
          const variant = staged.item;
          await chat.channelIdentityVariantUpdate(staged.id, {
            channelId: targetChannelId,
            targetUserId: targetUserId || undefined,
            identityId: targetIdentityId,
            selectorEmoji: variant.selectorEmoji,
            keyword: variant.keyword,
            matchMode: variant.matchMode || 'prefix',
            matchConfig: variant.matchConfig || '=',
            note: variant.note || '',
            avatarAttachmentId: variant.avatarAssetKey ? (assetIdMap.get(variant.avatarAssetKey) || '') : '',
            displayName: variant.displayName || '',
            color: variant.color || '',
            appearance: variant.appearance || {},
            enabled: variant.enabled !== false,
          });
        }
      }
    } catch (error) {
      failedCount += Math.max(1, sortedVariants.length);
      console.warn('导入角色差分失败', error);
      for (const staged of stagedVariants) {
        try {
          await chat.channelIdentityVariantDelete(targetChannelId, staged.id, targetUserId);
        } catch (cleanupError) {
          console.warn('清理暂存差分失败', cleanupError);
        }
      }
      if (overwriteExisting && oldVariants.every(old => chat.getIdentityVariants(targetChannelId, targetIdentityId, targetUserId).some(item => item.id === old.id))) {
        const snapshot = overwriteIdentitySnapshots.get(sourceIdentityId);
        if (snapshot) {
          try {
            await chat.channelIdentityUpdate(snapshot.id, {
              channelId: targetChannelId,
              targetUserId: targetUserId || undefined,
              displayName: snapshot.displayName,
              color: snapshot.color || '',
              avatarAttachmentId: snapshot.avatarAttachmentId || '',
              avatarDecorations: cloneAvatarDecorations(snapshot.avatarDecorations, snapshot.avatarDecoration),
              theaterPresentation: snapshot.theaterPresentation,
              isDefault: !!snapshot.isDefault,
              isTemporary: !!snapshot.isTemporary,
              icOocOnActivate: snapshot.icOocOnActivate || '',
              folderIds: snapshot.folderIds || [],
            });
            updatedCount = Math.max(0, updatedCount - 1);
            identityIdMap.delete(sourceIdentityId);
          } catch (restoreError) {
            console.warn('恢复覆盖前角色失败', restoreError);
          }
        }
      }
    }
  }

  if (payload.icOocConfig) {
    const currentConfig = chat.getChannelIcOocRoleConfig(targetChannelId, targetUserId);
    const importedConfig = {
      icRoleId: payload.icOocConfig.icRoleId ? (identityIdMap.get(payload.icOocConfig.icRoleId) || null) : null,
      oocRoleId: payload.icOocConfig.oocRoleId ? (identityIdMap.get(payload.icOocConfig.oocRoleId) || null) : null,
    };
    const nextConfig = options.mode === 'overwrite'
      ? importedConfig
      : {
          icRoleId: currentConfig.icRoleId || importedConfig.icRoleId,
          oocRoleId: currentConfig.oocRoleId || importedConfig.oocRoleId,
        };
    mappingChanged = nextConfig.icRoleId !== currentConfig.icRoleId || nextConfig.oocRoleId !== currentConfig.oocRoleId;
    if (mappingChanged && (nextConfig.icRoleId || nextConfig.oocRoleId || currentConfig.icRoleId || currentConfig.oocRoleId)) {
      await chat.setChannelIcOocRoleConfig(targetChannelId, nextConfig, targetUserId);
    }
  }

  await Promise.all([
    chat.loadChannelIdentities(targetChannelId, true, targetUserId),
    chat.loadChannelIdentityVariants(targetChannelId, true, targetUserId),
  ]);

  return {
    createdCount,
    updatedCount,
    skippedCount,
    failedCount,
    emptyNameCount,
    mappingChanged,
    duplicateTargetNames,
  };
};

const handleIdentityImportChange = async (event: globalThis.Event) => {
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
    const payload = normalizeIdentityExportFileForImport(
      JSON.parse(text) as IdentityExportFile,
      IDENTITY_EXPORT_COMPATIBLE_VERSIONS,
    );
    const items = payload.items || [];
    if (!items.length) {
      message.warning('导入文件中没有可用的频道角色');
      return;
    }
    const importMode = await new Promise<'append' | 'overwrite' | null>((resolve) => {
      let settled = false;
      const settle = (value: 'append' | 'overwrite' | null) => {
        if (settled) return;
        settled = true;
        resolve(value);
      };
      dialog.warning({
        title: '导入频道角色',
        content: `检测到 ${items.length} 个角色配置。追加会完整跳过同名角色；覆盖会替换同名角色及其差分。`,
        positiveText: '覆盖同名角色',
        negativeText: '追加新角色',
        onPositiveClick: () => settle('overwrite'),
        onNegativeClick: () => settle('append'),
        onClose: () => settle(null),
      });
    });
    if (!importMode) {
      return;
    }

    identityImporting.value = true;
    const result = await importIdentityMigrationSnapshot(payload, {
      targetChannelId: chat.curChannel.id,
      mode: importMode,
      targetUserId: currentIdentityTargetUserId.value,
    });
    const importedCount = result.createdCount + result.updatedCount;
    const details: string[] = [];
    if (result.createdCount) details.push(`新增 ${result.createdCount}`);
    if (result.updatedCount) details.push(`覆盖 ${result.updatedCount}`);
    if (result.skippedCount) details.push(`跳过 ${result.skippedCount}`);
    if (result.failedCount) details.push(`失败 ${result.failedCount}`);
    if (result.emptyNameCount) details.push(`忽略 ${result.emptyNameCount} 个无名角色`);
    const detailNote = details.length ? `（${details.join('，')}）` : '';
    if (importedCount > 0) {
      message.success(`已导入 ${importedCount} 个频道角色${detailNote}`);
    } else if (result.skippedCount > 0) {
      message.warning(`未新增频道角色${detailNote}`);
    } else {
      message.warning('未导入任何角色，请检查文件内容');
    }
    if (result.duplicateTargetNames.size > 0) {
      message.warning(`目标频道存在 ${result.duplicateTargetNames.size} 个重名角色，导入时按第一个匹配处理`);
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
    const snapshot = await buildIdentityMigrationSnapshotFromChannel(sourceChannelId, currentIdentityTargetUserId.value);
    if (!(snapshot.payload.items || []).length) {
      message.warning('所选频道暂无可同步的角色');
      return;
    }
    const result = await importIdentityMigrationSnapshot(snapshot.payload, {
      targetChannelId,
      mode,
      targetUserId: currentIdentityTargetUserId.value,
    });
    identitySyncDialogVisible.value = false;

    const syncedCount = result.createdCount + result.updatedCount;
    const hasAnyWork = syncedCount > 0 || result.skippedCount > 0 || result.mappingChanged;
    if (!hasAnyWork) {
      message.warning('没有可同步的角色或映射');
      return;
    }
    const details: string[] = [];
    if (result.createdCount) details.push(`新增 ${result.createdCount}`);
    if (result.updatedCount) details.push(`覆盖 ${result.updatedCount}`);
    if (result.skippedCount) details.push(`跳过 ${result.skippedCount}`);
    if (result.failedCount) details.push(`失败 ${result.failedCount}`);
    if (result.emptyNameCount) details.push(`忽略 ${result.emptyNameCount} 个无名角色`);
    const mappingNote = result.mappingChanged ? '，已同步场内/场外映射' : '';
    const detailNote = details.length ? `（${details.join('，')}）` : '';
    const summaryText = syncedCount > 0 ? `已同步 ${syncedCount} 个角色` : '没有新增角色';
    message.success(`${summaryText}${detailNote}${mappingNote}`);
    if (snapshot.missingAssetCount > 0) {
      message.warning(`同步时有 ${snapshot.missingAssetCount} 个缺失素材已跳过`);
    }
    if (result.duplicateTargetNames.size > 0) {
      message.warning(`目标频道存在 ${result.duplicateTargetNames.size} 个重名角色，同步时按第一个匹配处理`);
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

const normalizeColorDraftText = (value: string) => String(value || '').trim().toLowerCase();

const commitIdentityColorDraft = (showWarning = true) => {
  const draft = normalizeColorDraftText(identityColorDraft.value);
  if (!draft) {
    identityForm.color = '';
    identityColorDraft.value = '';
    return true;
  }
  const normalized = normalizeHexColor(draft);
  if (!normalized) {
    if (showWarning) {
      message.warning('颜色格式应为 #RGB 或 #RRGGBB');
    }
    return false;
  }
  identityForm.color = normalized;
  identityColorDraft.value = normalized;
  return true;
};

const commitIdentityVariantColorDraft = (showWarning = true) => {
  const draft = normalizeColorDraftText(identityVariantColorDraft.value);
  if (!draft) {
    identityVariantForm.color = '';
    identityVariantColorDraft.value = '';
    return true;
  }
  const normalized = normalizeHexColor(draft);
  if (!normalized) {
    if (showWarning) {
      message.warning('颜色格式应为 #RGB 或 #RRGGBB');
    }
    return false;
  }
  identityVariantForm.color = normalized;
  identityVariantColorDraft.value = normalized;
  return true;
};

const applyIdentityAppearanceToMessages = (identity: ChannelIdentity) => {
  if (!identity || identity.channelId !== chat.curChannel?.id) {
    return;
  }
  if (chat.getActiveIdentityId(identity.channelId) !== identity.id) {
    return;
  }
  const appearance = resolveIdentityAppearancePreview(identity, null);
  typingPreviewList.value = typingPreviewList.value.map((item) => {
    if (item.userId === user.info.id) {
      return {
        ...item,
        displayName: appearance?.displayName || item.displayName,
        color: appearance?.color || item.color,
        avatar: appearance?.avatarAttachmentId
          ? resolveAttachmentUrl(appearance.avatarAttachmentId)
          : (appearance?.isTemporary ? '' : (chat.curMember?.avatar || user.info.avatar || item.avatar)),
        avatarDecorations: cloneAvatarDecorations(appearance?.avatarDecorations),
        isTemporary: Boolean(appearance?.isTemporary),
      };
    }
    return item;
  });
};

const clearRemovedIdentityFromMessages = (_identityId: string) => {
  // 历史消息以消息快照为准，删除 live identity 时不再擦除 sender_identity_* 数据。
};

const handleIdentityColorBlur = () => {
  commitIdentityColorDraft(true);
};

const handleIdentityColorPickerUpdate = (value: string | null) => {
  const normalized = normalizeHexColor(String(value || ''));
  identityForm.color = normalized;
  identityColorDraft.value = normalized;
};

const clearIdentityColor = () => {
  identityForm.color = '';
  identityColorDraft.value = '';
};

const handleIdentityVariantColorBlur = () => {
  commitIdentityVariantColorDraft(true);
};

const handleIdentityVariantColorPickerUpdate = (value: string | null) => {
  const normalized = normalizeHexColor(String(value || ''));
  identityVariantForm.color = normalized;
  identityVariantColorDraft.value = normalized;
};

const clearIdentityVariantColor = () => {
  identityVariantForm.color = '';
  identityVariantColorDraft.value = '';
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
  identityColorDraft.value = identityForm.color;
  identityForm.avatarAttachmentId = identity?.avatarAttachmentId || '';
  identityForm.avatarDecorations = cloneAvatarDecorations(identity?.avatarDecorations, identity?.avatarDecoration);
  identityForm.theaterPresentation = cloneChannelIdentityTheaterPresentation(identity?.theaterPresentation);
  identityForm.isDefault = identity?.isDefault ?? (currentChannelIdentities.value.length === 0);
  identityForm.isTemporary = Boolean(identity?.isTemporary);
  identityForm.botAppearanceMode = isManagingBotIdentity.value
    ? (identity?.botAppearanceMode === 'custom' ? 'custom' : 'inherit')
    : '';
  identityForm.variantResetMatchMode = (identity?.variantResetMatchMode || 'prefix') as IdentityVariantMatchMode;
  identityForm.variantResetMatchConfig = identity?.variantResetMatchConfig
    || (identityForm.variantResetMatchMode === 'prefix' ? '=' : identityForm.variantResetMatchMode === 'keyword' ? 'any' : 'sensitive');
  identityForm.variantResetMatchContent = identity?.variantResetMatchContent || '还原';
  identityForm.icOocOnActivate = identity?.isTemporary
    ? (identity.icOocOnActivate === 'ooc' ? 'ooc' : 'ic')
    : '';
  identityForm.folderIds = identity?.folderIds ? [...identity.folderIds] : [];
  identityForm.characterCardId = !isManagingBotIdentity.value && identity?.id
    ? characterCardStore.getBoundCardId(identity.id, identity.sharedIdentityId) || ''
    : '';
  identityOriginalCardId.value = identityForm.characterCardId;
  identityForm.promoteToShared = false;
  identityAvatarPreview.value = resolveAttachmentUrl(identity?.avatarAttachmentId);
};

const handlePromoteIdentityToSharedUpdate = async (checked: boolean) => {
  if (!checked) {
    identityForm.promoteToShared = false;
    return;
  }
  const confirmed = await dialogAskConfirm(
    dialog,
    '提升为跨频道角色（实验性）',
    '这是实验性功能。提升后，昵称、颜色、头像、头像装饰、差分配置角色卡会在全部频道间同步，小剧场演出与差分演出会在同一世界内同步。这可能出现bug且不支持降级（你可以在用户群提出反馈）。确定继续吗？',
  );
  identityForm.promoteToShared = confirmed;
};

const openIdentityDecorationEditor = () => {
  if (isDelegatedSharedIdentity.value) return;
  identityDecorationEditorVisible.value = true;
};

const askEnterTheaterForAppearanceEdit = () => new Promise<boolean>((resolve) => {
  let settled = false;
  const settle = (value: boolean) => {
    if (settled) return;
    settled = true;
    resolve(value);
  };
  dialog.warning({
    title: '提示',
    content: '编辑必须在小剧场模式进行，是否进入小剧场？',
    positiveText: '进入',
    negativeText: '取消',
    onPositiveClick: () => settle(true),
    onNegativeClick: () => settle(false),
    onClose: () => settle(false),
  });
});

const enterTheaterForAppearanceEdit = async (mode: 'base' | 'variant') => {
  const worldId = routeWorldId.value || String(chat.currentWorldId || '').trim();
  const channelId = chat.curChannel?.id ? String(chat.curChannel.id) : '';
  const channelWorldId = String(chat.curChannel?.worldId || '').trim();
  const identityId = String(editingIdentity.value?.id || '').trim();
  if (!worldId || !channelId || String(chat.currentWorldId || '').trim() !== worldId || (channelWorldId && channelWorldId !== worldId)) {
    message.warning('正在切换世界，请稍后再试');
    return false;
  }
  if (!identityId) {
    message.warning('请先保存频道角色后再编辑演出外观');
    return false;
  }
  writeTheaterAppearanceEditIntent({
    channelId,
    identityId,
    mode,
    variantId: mode === 'variant' ? String(editingIdentityVariant.value?.id || '').trim() || undefined : undefined,
    targetUserId: identityManageTargetUserId.value || undefined,
    targetKind: identityManageTargetKind.value,
    targetLabel: identityManageTargetLabel.value || undefined,
    targetAvatar: identityManageTargetAvatar.value || undefined,
  });
  identityDialogVisible.value = false;
  identityVariantDialogVisible.value = false;
  identityManageVisible.value = false;
  theaterPresentationEditorVisible.value = false;
  try {
    await router.push({
      name: 'theater',
      query: { worldId, channelId },
    });
    return true;
  } catch (error) {
    clearTheaterAppearanceEditIntent();
    message.error(error instanceof Error ? error.message : '进入小剧场失败');
    return false;
  }
};

const ensureTheaterModeForAppearanceEdit = async (mode: 'base' | 'variant') => {
  if (isTheaterEmbedMode.value) return true;
  const confirmed = await askEnterTheaterForAppearanceEdit();
  if (!confirmed) return false;
  await enterTheaterForAppearanceEdit(mode);
  return false;
};

const openIdentityTheaterPresentationEditor = async () => {
  if (isDelegatedSharedIdentity.value) return;
  if (!(await ensureTheaterModeForAppearanceEdit('base'))) return;
  if (editingIdentity.value?.sharedIdentityId && editingIdentity.value.id && chat.curChannel?.id) {
    try {
      await chat.loadChannelIdentities(chat.curChannel.id, true, currentIdentityTargetUserId.value);
      const refreshed = chat.getScopedChannelIdentities(chat.curChannel.id, currentIdentityTargetUserId.value)
        .find(item => item.id === editingIdentity.value?.id);
      if (refreshed) {
        editingIdentity.value = refreshed;
        identityForm.theaterPresentation = cloneChannelIdentityTheaterPresentation(refreshed.theaterPresentation);
      }
    } catch (error: any) {
      message.error(error?.response?.data?.error || '刷新共享演出外观失败');
      return;
    }
  }
  theaterPresentationEditorMode.value = 'base';
  theaterPresentationEditorVisible.value = true;
};

const openIdentityVariantTheaterPresentationEditor = async () => {
  if (!(await ensureTheaterModeForAppearanceEdit('variant'))) return;
  if (editingIdentityVariant.value?.sharedVariantId && editingIdentity.value?.id && chat.curChannel?.id) {
    try {
      await chat.loadChannelIdentityVariants(chat.curChannel.id, true, currentIdentityTargetUserId.value);
      const refreshed = chat.getIdentityVariants(chat.curChannel.id, editingIdentity.value.id, currentIdentityTargetUserId.value)
        .find(item => item.id === editingIdentityVariant.value?.id);
      if (refreshed) {
        editingIdentityVariant.value = refreshed;
        resetIdentityVariantForm(refreshed);
      }
    } catch (error: any) {
      message.error(error?.response?.data?.error || '刷新共享差分演出失败');
      return;
    }
  }
  theaterPresentationEditorMode.value = 'variant';
  theaterPresentationEditorVisible.value = true;
};

const handleTheaterPresentationApply = async (value: TheaterPresentation | TheaterPresentationPatch) => {
  if (theaterPresentationApplying.value) return;
  theaterPresentationApplying.value = true;
  let saved = false;
  try {
    if (theaterPresentationEditorMode.value === 'variant') {
      identityVariantForm.theaterPresentation = cloneChannelIdentityTheaterPresentationPatch(value as TheaterPresentationPatch);
      if (identityVariantDialogMode.value === 'edit' && editingIdentityVariant.value?.id) {
        saved = await submitIdentityVariantForm({ closeDialog: false, successMessage: false });
      } else {
        saved = true;
      }
    } else {
      const submittedPresentation = cloneChannelIdentityTheaterPresentation(value as TheaterPresentation);
      identityForm.theaterPresentation = submittedPresentation;
      if (editingIdentity.value?.sharedIdentityId && editingIdentity.value.id && chat.curChannel?.id) {
        try {
          const savedIdentity = await chat.sharedChannelIdentityTheaterPresentationSet(editingIdentity.value.id, {
            channelId: chat.curChannel.id,
            theaterPresentation: identityForm.theaterPresentation,
            expectedRevision: editingIdentity.value.sharedRevision,
          });
          const canonicalPresentation = cloneChannelIdentityTheaterPresentation(savedIdentity.theaterPresentation)
            || submittedPresentation;
          if (!canonicalPresentation) {
            throw new Error('服务端未返回已保存的共享演出外观');
          }
          savedIdentity.theaterPresentation = canonicalPresentation;
          chat.upsertChannelIdentity(savedIdentity);
          editingIdentity.value = savedIdentity;
          identityForm.theaterPresentation = canonicalPresentation;
          saved = true;
        } catch (error: any) {
          const conflictRevision = Number(error?.response?.data?.revision || 0);
          if (error?.response?.status === 409 && conflictRevision > 0 && editingIdentity.value) {
            await chat.loadChannelIdentities(chat.curChannel.id, true, currentIdentityTargetUserId.value).catch(() => undefined);
            const refreshed = chat.getScopedChannelIdentities(chat.curChannel.id, currentIdentityTargetUserId.value)
              .find(item => item.id === editingIdentity.value?.id);
            if (refreshed) {
              editingIdentity.value = refreshed;
              identityForm.theaterPresentation = cloneChannelIdentityTheaterPresentation(refreshed.theaterPresentation);
            }
          }
          message.error(error?.response?.data?.error || error?.message || '保存共享演出外观失败');
        }
      } else {
        saved = await submitIdentityForm({ closeDialog: false, successMessage: false });
      }
    }
    if (saved) {
      theaterPresentationEditorVisible.value = false;
    }
  } finally {
    theaterPresentationApplying.value = false;
  }
};

const handleSetWorldTheaterTemplate = async (
  sections: WorldTheaterPresentationTemplateSection[],
  presentation: TheaterPresentation,
) => {
  const worldId = String(chat.currentWorldId || '').trim();
  if (!worldId || !canSetWorldTheaterTemplate.value || worldTheaterTemplateSaving.value) return;
  worldTheaterTemplateSaving.value = true;
  try {
    const template = mergeWorldTheaterPresentationTemplate(currentWorldTheaterTemplate.value, presentation, sections);
    await chat.worldTheaterPresentationTemplateSet(worldId, template);
    // 后端会级联刷新仍使用旧默认的角色；强制重载当前频道角色列表以同步 UI。
    const channelId = String(chat.curChannel?.id || '').trim();
    if (channelId) {
      await chat.loadChannelIdentities(channelId, true, currentIdentityTargetUserId.value || undefined);
      if (editingIdentity.value?.id) {
        const refreshed = chat.getScopedChannelIdentities(channelId, currentIdentityTargetUserId.value)
          .find((item) => item.id === editingIdentity.value?.id);
        if (refreshed) {
          editingIdentity.value = refreshed;
          identityForm.theaterPresentation = cloneChannelIdentityTheaterPresentation(refreshed.theaterPresentation);
        }
      }
    }
    message.success('世界默认演出外观已更新');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '更新世界默认演出外观失败');
  } finally {
    worldTheaterTemplateSaving.value = false;
  }
};

const handleIdentityDecorationEditorShow = async (show: boolean) => {
  if (show) {
    identityDecorationEditorVisible.value = true;
    return;
  }
  if (!identityDecorationEditorVisible.value || identitySubmitting.value) {
    return;
  }
  if (await submitIdentityForm({ closeDialog: false, successMessage: false })) {
    identityDecorationEditorVisible.value = false;
  }
};

const setTemporaryIdentityActivateMode = (mode: 'ic' | 'ooc') => {
  identityForm.icOocOnActivate = mode;
};

watch(() => identityForm.isTemporary, (isTemporary) => {
  if (!isTemporary) {
    if (identityDialogMode.value === 'create') {
      identityForm.icOocOnActivate = '';
    }
    return;
  }
  if (!identityForm.icOocOnActivate) {
    identityForm.icOocOnActivate = chat.icMode === 'ooc' ? 'ooc' : 'ic';
  }
});

const openIdentityCreate = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  editingIdentity.value = null;
  identityDialogMode.value = 'create';
  resetIdentityForm(null);
  if (Object.keys(currentWorldTheaterTemplate.value).length) {
    identityForm.theaterPresentation = applyWorldTheaterPresentationTemplate(
      createDefaultTheaterPresentation(),
      currentWorldTheaterTemplate.value,
    );
  }
  if (!identityForm.displayName) {
    identityForm.displayName = chat.curMember?.nick || user.info.nick || user.info.username || '';
  }
  // Load character cards for the channel
  if (!isManagingBotIdentity.value && !characterCardStore.isBotCharacterDisabled(chat.curChannel.id)) {
    await characterCardStore.loadCards(chat.curChannel.id);
  }
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
    await chat.loadChannelIdentityVariants(chat.curChannel.id, true, currentIdentityTargetUserId.value);
    if (!isManagingBotIdentity.value && !characterCardStore.isBotCharacterDisabled(chat.curChannel.id)) {
      await characterCardStore.loadCards(chat.curChannel.id);
    }
    identityForm.characterCardId = !isManagingBotIdentity.value && identity?.id
      ? characterCardStore.getBoundCardId(identity.id, identity.sharedIdentityId) || ''
      : '';
    identityOriginalCardId.value = identityForm.characterCardId;
  }
  identityDialogVisible.value = true;
};

const openActiveTemporaryIdentityEdit = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  await chat.loadChannelIdentities(chat.curChannel.id, false, currentIdentityTargetUserId.value);
  const current = chat.getActiveIdentity(chat.curChannel.id);
  if (!current?.isTemporary) {
    message.warning('当前不是临时角色');
    return;
  }
  await openIdentityEdit(current);
};

const openIdentityManager = async () => {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  await chat.loadChannelIdentities(chat.curChannel.id, true, currentIdentityTargetUserId.value);
  identityManageVisible.value = true;
};

const closeIdentityDialog = () => {
  identityDialogVisible.value = false;
  identityDecorationEditorVisible.value = false;
  theaterPresentationEditorVisible.value = false;
  identityVariantDialogVisible.value = false;
  identityVariantEmojiPickerVisible.value = false;
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

const revokeIdentityVariantObjectURL = () => {
  if (identityVariantAvatarObjectURL) {
    URL.revokeObjectURL(identityVariantAvatarObjectURL);
    identityVariantAvatarObjectURL = null;
  }
};

const resetIdentityVariantForm = (variant?: ChannelIdentityVariant | null) => {
  revokeIdentityVariantObjectURL();
  identityVariantAvatarFile = null;
  identityVariantForm.selectorEmoji = variant?.selectorEmoji || '';
  identityVariantForm.matchMode = (variant?.matchMode || 'prefix') as IdentityVariantMatchMode;
  identityVariantForm.matchDrafts = {
    prefix: { config: '=', content: '' },
    keyword: { config: 'any', content: '' },
    regex: { config: 'sensitive', content: '' },
  };
  const activeDraft = identityVariantForm.matchDrafts[identityVariantForm.matchMode];
  activeDraft.config = variant?.matchConfig || activeDraft.config;
  activeDraft.content = variant?.keyword || '';
  identityVariantForm.note = variant?.note || '';
  identityVariantForm.avatarAttachmentId = variant?.avatarAttachmentId || '';
  identityVariantForm.displayName = variant?.displayName || '';
  identityVariantForm.color = normalizeHexColor(variant?.color || '') || '';
  identityVariantForm.theaterPresentation = resolveChannelIdentityVariantTheaterPatch(variant);
  identityVariantColorDraft.value = identityVariantForm.color;
  identityVariantForm.enabled = variant?.enabled !== false;
  identityVariantAvatarPreview.value = resolveAttachmentUrl(variant?.avatarAttachmentId);
};

const openIdentityVariantResetConfig = () => {
  identityVariantResetForm.matchMode = identityForm.variantResetMatchMode || 'prefix';
  identityVariantResetForm.matchDrafts = {
    prefix: { config: '=', content: '还原' },
    keyword: { config: 'any', content: '还原' },
    regex: { config: 'sensitive', content: '还原' },
  };
  const activeDraft = identityVariantResetForm.matchDrafts[identityVariantResetForm.matchMode];
  activeDraft.config = identityForm.variantResetMatchConfig || activeDraft.config;
  activeDraft.content = identityForm.variantResetMatchContent || activeDraft.content;
  identityVariantResetDialogVisible.value = true;
};

const applyIdentityVariantResetConfig = async () => {
  const matchMode = identityVariantResetForm.matchMode;
  const matchConfig = String(activeIdentityVariantResetMatchDraft.value.config || '').trim();
  const matchContent = String(activeIdentityVariantResetMatchDraft.value.content || '').trim();
  if (!matchContent) {
    message.warning('请输入匹配内容');
    return;
  }
  if (matchMode === 'prefix' && (!matchConfig || /\s/.test(matchConfig))) {
    message.warning('前缀符号不能为空或包含空白');
    return;
  }
  if (matchMode === 'keyword') {
    const forbidden = matchConfig === 'all' ? '|' : '&';
    const separator = matchConfig === 'all' ? '&' : '|';
    if (matchContent.includes(forbidden) || matchContent.split(separator).some(item => !item.trim())) {
      message.warning('关键词匹配内容不能混用 | 和 &，且不能包含空关键词');
      return;
    }
  }
  if (matchMode === 'regex') {
    try {
      new RegExp(matchContent, matchConfig === 'insensitive' ? 'i' : '');
    } catch {
      message.warning('请输入有效的正则表达式');
      return;
    }
  }
  identityForm.variantResetMatchMode = matchMode;
  identityForm.variantResetMatchConfig = matchConfig;
  identityForm.variantResetMatchContent = matchContent;
  if (await submitIdentityForm({ closeDialog: false, successMessage: false })) {
    identityVariantResetDialogVisible.value = false;
    message.success('恢复默认头像规则已更新');
  }
};

const closeIdentityVariantDialog = () => {
  if (identityVariantSubmitting.value) {
    return;
  }
  identityVariantDialogVisible.value = false;
  identityVariantEmojiPickerVisible.value = false;
};

const openIdentityVariantCreate = () => {
  if (!editingIdentity.value?.id) {
    message.warning('请先保存频道角色后再添加差分');
    return;
  }
  editingIdentityVariant.value = null;
  identityVariantDialogMode.value = 'create';
  resetIdentityVariantForm(null);
  identityVariantDialogVisible.value = true;
};

const openIdentityVariantEdit = (variant: ChannelIdentityVariant) => {
  editingIdentityVariant.value = variant;
  identityVariantDialogMode.value = 'edit';
  resetIdentityVariantForm(variant);
  identityVariantDialogVisible.value = true;
};

let theaterAppearanceEditResumeRunning = false;
const resumeTheaterAppearanceEditIntent = async () => {
  if (theaterAppearanceEditResumeRunning || !isTheaterEmbedMode.value) return;
  const channelId = String(chat.curChannel?.id || '').trim();
  if (!channelId) return;
  const intent = consumeTheaterAppearanceEditIntent(channelId);
  if (!intent) return;
  theaterAppearanceEditResumeRunning = true;
  try {
    if (intent.targetUserId && intent.targetKind && intent.targetKind !== 'self') {
      identityManageTargetUserId.value = intent.targetUserId;
      identityManageTargetKind.value = intent.targetKind;
      identityManageTargetLabel.value = intent.targetLabel || intent.targetUserId;
      identityManageTargetAvatar.value = intent.targetAvatar || '';
      identityManageTargetRoleLabel.value = intent.targetKind === 'bot' ? 'BOT' : '';
    } else {
      resetIdentityManageTarget();
    }
    await chat.loadChannelIdentities(channelId, true, currentIdentityTargetUserId.value);
    const identity = chat.getScopedChannelIdentities(channelId, currentIdentityTargetUserId.value)
      .find((item) => String(item.id || '') === intent.identityId);
    if (!identity) {
      message.warning('目标角色不存在，已取消自动进入编辑');
      return;
    }
    await openIdentityEdit(identity);
    if (intent.mode === 'variant') {
      await chat.loadChannelIdentityVariants(channelId, true, currentIdentityTargetUserId.value);
      const variants = chat.getIdentityVariants(channelId, intent.identityId, currentIdentityTargetUserId.value);
      const variant = variants.find((item) => String(item.id || '') === String(intent.variantId || ''));
      if (!variant) {
        message.warning('目标差分不存在，已打开角色编辑');
        return;
      }
      openIdentityVariantEdit(variant);
      theaterPresentationEditorMode.value = 'variant';
    } else {
      theaterPresentationEditorMode.value = 'base';
    }
    await nextTick();
    theaterPresentationEditorVisible.value = true;
  } catch (error) {
    console.warn('[theater-appearance-edit] resume failed', error);
    message.warning('自动进入演出外观编辑失败，请手动打开');
  } finally {
    theaterAppearanceEditResumeRunning = false;
  }
};

watch(
  () => [isTheaterEmbedMode.value, String(chat.curChannel?.id || '')] as const,
  ([theaterMode, channelId]) => {
    if (!theaterMode || !channelId) return;
    void resumeTheaterAppearanceEditIntent();
  },
  { immediate: true },
);

const handleIdentityVariantAvatarTrigger = () => {
  identityVariantAvatarInputRef.value?.click();
};

const handleIdentityVariantAvatarChange = (event: Event) => {
  const input = event.target as HTMLInputElement | null;
  if (!input || !input.files?.length) {
    return;
  }
  const file = input.files[0];
  const sizeLimit = utils.config?.imageSizeLimit ? utils.config.imageSizeLimit * 1024 : utils.fileSizeLimit;
  if (file.size > sizeLimit) {
    const limitMB = (sizeLimit / 1024 / 1024).toFixed(1);
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`);
    input.value = '';
    return;
  }
  identityVariantAvatarEditorFile.value = file;
  identityVariantAvatarEditorVisible.value = true;
  input.value = '';
};

const handleIdentityVariantAvatarEditorSave = (file: File) => {
  identityVariantForm.avatarAttachmentId = '';
  identityVariantAvatarFile = file;
  revokeIdentityVariantObjectURL();
  identityVariantAvatarObjectURL = URL.createObjectURL(file);
  identityVariantAvatarPreview.value = identityVariantAvatarObjectURL;
  identityVariantAvatarEditorVisible.value = false;
  identityVariantAvatarEditorFile.value = null;
};

const handleIdentityVariantAvatarEditorCancel = () => {
  identityVariantAvatarEditorVisible.value = false;
  identityVariantAvatarEditorFile.value = null;
};

const removeIdentityVariantAvatar = () => {
  identityVariantForm.avatarAttachmentId = '';
  identityVariantAvatarFile = null;
  revokeIdentityVariantObjectURL();
  identityVariantAvatarPreview.value = '';
};

const handleIdentityVariantSelectorEmoji = (emoji: string) => {
  if (!emoji) {
    return;
  }
  identityVariantForm.selectorEmoji = emoji;
  identityVariantEmojiPickerVisible.value = false;
};

const submitIdentityVariantForm = async (options: { closeDialog?: boolean; successMessage?: boolean } = {}) => {
  const { closeDialog = true, successMessage = true } = options;
  if (identityVariantSubmitting.value) {
    return false;
  }
  if (!chat.curChannel?.id || !editingIdentity.value?.id) {
    message.warning('请先选择频道角色');
    return false;
  }
  const selectorEmoji = String(identityVariantForm.selectorEmoji || '').trim();
  const matchMode = identityVariantForm.matchMode;
  const matchConfig = String(activeIdentityVariantMatchDraft.value.config || '').trim();
  const keyword = String(activeIdentityVariantMatchDraft.value.content || '').trim();
  const note = String(identityVariantForm.note || '').trim();
  const rawColor = normalizeColorDraftText(identityVariantColorDraft.value);
  const normalizedColor = rawColor ? normalizeHexColor(rawColor) : '';
  if (!selectorEmoji) {
    message.warning('请选择差分表情');
    return false;
  }
  if (!keyword) {
    message.warning('请输入匹配内容');
    return false;
  }
  if (matchMode === 'prefix' && (!matchConfig || /\s/.test(matchConfig))) {
    message.warning('前缀符号不能为空或包含空白');
    return false;
  }
  if (matchMode === 'keyword') {
    const forbidden = matchConfig === 'all' ? '|' : '&';
    const separator = matchConfig === 'all' ? '&' : '|';
    if (keyword.includes(forbidden) || keyword.split(separator).some(item => !item.trim())) {
      message.warning('关键词匹配内容不能混用 | 和 &，且不能包含空关键词');
      return false;
    }
  }
  if (matchMode === 'regex') {
    try {
      new RegExp(keyword, matchConfig === 'insensitive' ? 'i' : '');
    } catch {
      message.warning('请输入有效的正则表达式');
      return false;
    }
  }
  if (!commitIdentityVariantColorDraft(true)) {
    return false;
  }
  identityVariantSubmitting.value = true;
  try {
    let avatarAttachmentId = identityVariantForm.avatarAttachmentId;
    if (identityVariantAvatarFile) {
      const uploadResult = await uploadImageAttachment(identityVariantAvatarFile, { channelId: chat.curChannel.id });
      const fileToken = uploadResult.attachmentId;
      if (!fileToken) {
        throw new Error('上传失败：未返回附件ID');
      }
      avatarAttachmentId = normalizeAttachmentId(fileToken);
      identityVariantForm.avatarAttachmentId = avatarAttachmentId;
      identityVariantAvatarPreview.value = resolveAttachmentUrl(fileToken);
      identityVariantAvatarFile = null;
    }
    const payload = {
      channelId: chat.curChannel.id,
      targetUserId: currentIdentityTargetUserId.value,
      identityId: editingIdentity.value.id,
      selectorEmoji,
      keyword,
      matchMode,
      matchConfig,
      note,
      avatarAttachmentId,
      displayName: String(identityVariantForm.displayName || '').trim(),
      color: normalizedColor,
      appearance: {},
      theaterPresentation: cloneChannelIdentityTheaterPresentationPatch(identityVariantForm.theaterPresentation),
      expectedRevision: editingIdentityVariant.value?.sharedVariantId
        ? editingIdentityVariant.value.sharedRevision
        : undefined,
      enabled: identityVariantForm.enabled,
    };
    let savedVariant: ChannelIdentityVariant | null = null;
    if (identityVariantDialogMode.value === 'create') {
      savedVariant = await chat.channelIdentityVariantCreate(payload);
      if (successMessage) {
        message.success('头像差分已创建');
      }
    } else if (editingIdentityVariant.value?.id) {
      savedVariant = await chat.channelIdentityVariantUpdate(editingIdentityVariant.value.id, payload);
      if (successMessage) {
        message.success('头像差分已更新');
      }
    }
    if (!closeDialog && savedVariant) {
      editingIdentityVariant.value = savedVariant;
      identityVariantDialogMode.value = 'edit';
      resetIdentityVariantForm(savedVariant);
    }
    if (closeDialog) {
      identityVariantDialogVisible.value = false;
    }
    return true;
  } catch (error: any) {
    if (error?.response?.status === 409 && editingIdentityVariant.value?.id && chat.curChannel?.id && editingIdentity.value?.id) {
      await chat.loadChannelIdentityVariants(chat.curChannel.id, true, currentIdentityTargetUserId.value).catch(() => undefined);
      const refreshed = chat.getIdentityVariants(chat.curChannel.id, editingIdentity.value.id, currentIdentityTargetUserId.value)
        .find(item => item.id === editingIdentityVariant.value?.id);
      if (refreshed) {
        editingIdentityVariant.value = refreshed;
        resetIdentityVariantForm(refreshed);
      }
    }
    const errMsg = error?.response?.data?.error || error?.message || '保存差分失败，请稍后重试';
    message.error(errMsg);
    return false;
  } finally {
    identityVariantSubmitting.value = false;
  }
};

const deleteIdentityVariant = async (variant: ChannelIdentityVariant) => {
  if (!chat.curChannel?.id || !editingIdentity.value?.id) {
    return;
  }
  const confirmed = await dialogAskConfirm(dialog, {
    title: '删除头像差分',
    content: `确定删除差分「${resolveVariantNote(variant)}」吗？此操作无法撤销。`,
  });
  if (!confirmed) {
    return;
  }
  try {
      await chat.channelIdentityVariantDelete(chat.curChannel.id, variant.id, currentIdentityTargetUserId.value);
    if (editingIdentityVariant.value?.id === variant.id) {
      identityVariantDialogVisible.value = false;
      editingIdentityVariant.value = null;
    }
    message.success('头像差分已删除');
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '删除差分失败，请稍后重试';
    message.error(errMsg);
  }
};

const submitIdentityForm = async (options: { closeDialog?: boolean; successMessage?: boolean } = {}) => {
  const { closeDialog = true, successMessage = true } = options;
  if (identitySubmitting.value) {
    return false;
  }
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return false;
  }
  if (!identityForm.displayName.trim()) {
    message.warning('频道昵称不能为空');
    return false;
  }
  if (!commitIdentityColorDraft(true)) {
    return false;
  }
  const normalizedColor = identityForm.color;
  identitySubmitting.value = true;
  const payload = {
    channelId: chat.curChannel.id,
    targetUserId: currentIdentityTargetUserId.value,
    displayName: identityForm.displayName.trim(),
    color: normalizedColor,
    avatarAttachmentId: identityForm.avatarAttachmentId,
    avatarDecorations: cloneAvatarDecorations(identityForm.avatarDecorations)
      .filter(item => item.resourceAttachmentId),
    theaterPresentation: editingIdentity.value?.sharedIdentityId
      ? undefined
      : identityForm.theaterPresentation
        ? cloneChannelIdentityTheaterPresentation(identityForm.theaterPresentation)
        : null,
    isDefault: identityForm.isDefault,
    isTemporary: identityForm.isTemporary,
    botAppearanceMode: isManagingBotIdentity.value ? identityForm.botAppearanceMode : '',
    variantResetMatchMode: identityForm.variantResetMatchMode,
    variantResetMatchConfig: identityForm.variantResetMatchConfig,
    variantResetMatchContent: identityForm.variantResetMatchContent,
    icOocOnActivate: identityForm.isTemporary ? (identityForm.icOocOnActivate || (chat.icMode === 'ooc' ? 'ooc' : 'ic')) : '',
    folderIds: identityForm.folderIds,
    promoteToShared: identityDialogMode.value === 'edit' && identityForm.promoteToShared,
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
    } else if (payload.isTemporary) {
      const generatedAvatarFile = await buildGeneratedAvatarFile({
        displayName: payload.displayName,
        accentColor: payload.color,
        size: 256,
        themeSeed: {
          palette: display.palette,
          customThemeEnabled: display.settings.customThemeEnabled,
          activeCustomThemeId: display.settings.activeCustomThemeId,
        },
      }, `identity-${Date.now()}.png`);
      const uploadResult = await uploadImageAttachment(generatedAvatarFile, {
        channelId: chat.curChannel.id,
        skipCompression: true,
      });
      const fileToken = uploadResult.attachmentId;
      if (!fileToken) {
        throw new Error('上传失败：未返回附件ID');
      }
      const normalizedToken = normalizeAttachmentId(fileToken);
      identityForm.avatarAttachmentId = normalizedToken;
      payload.avatarAttachmentId = identityForm.avatarAttachmentId;
      identityAvatarPreview.value = resolveAttachmentUrl(fileToken);
    }
    let savedIdentity: ChannelIdentity | null = null;
    if (identityDialogMode.value === 'create') {
      const createdIdentity = await chat.channelIdentityCreate(payload);
      savedIdentity = createdIdentity;
      // Handle character card binding for new identity
      if (createdIdentity?.id && chat.curChannel?.id) {
        if (characterCardStore.isBotCharacterDisabled(chat.curChannel.id)) {
          message.warning(characterCardStore.getCharacterApiDisabledReason(chat.curChannel.id));
        } else {
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
      }
      if (successMessage) {
        message.success('频道角色已创建');
      }
    } else if (editingIdentity.value) {
      if (editingIdentity.value.isTemporary) {
        const replacedIdentity = await chat.channelIdentityReplaceTemporary(editingIdentity.value.id, payload);
        savedIdentity = replacedIdentity;
        if (replacedIdentity?.id && chat.curChannel?.id && effectiveBotFeatureEnabled.value) {
          if (characterCardStore.isBotCharacterDisabled(chat.curChannel.id)) {
            message.warning(characterCardStore.getCharacterApiDisabledReason(chat.curChannel.id));
          } else {
            try {
              if (identityForm.characterCardId) {
                await characterCardStore.bindIdentity(chat.curChannel.id, replacedIdentity.id, identityForm.characterCardId);
              } else {
                await characterCardStore.unbindIdentity(chat.curChannel.id, replacedIdentity.id);
              }
            } catch (e) {
              console.warn('Failed to bind character card for replaced identity', e);
            }
          }
        }
        if (successMessage) {
          message.success('临时频道角色已替换，新身份已生效');
        }
      } else {
        savedIdentity = await chat.channelIdentityUpdate(editingIdentity.value.id, payload);
        // Handle character card binding changes for existing identity
        const characterCardBindingChanged = identityForm.characterCardId !== identityOriginalCardId.value
          || Boolean(payload.promoteToShared && savedIdentity?.sharedIdentityId);
        if (!isManagingBotIdentity.value && chat.curChannel?.id && characterCardBindingChanged) {
          if (characterCardStore.isBotCharacterDisabled(chat.curChannel.id)) {
            message.warning(characterCardStore.getCharacterApiDisabledReason(chat.curChannel.id));
          } else {
            try {
              if (identityForm.characterCardId) {
                await characterCardStore.bindIdentity(
                  chat.curChannel.id,
                  savedIdentity?.id || editingIdentity.value.id,
                  identityForm.characterCardId,
                  savedIdentity?.sharedIdentityId,
                );
              } else {
                await characterCardStore.unbindIdentity(
                  chat.curChannel.id,
                  savedIdentity?.id || editingIdentity.value.id,
                  savedIdentity?.sharedIdentityId,
                );
              }
            } catch (e) {
              console.warn('Failed to update character card binding', e);
            }
          }
        }
        if (successMessage) {
          message.success(payload.promoteToShared ? '已提升为跨频道角色' : '频道角色已更新');
        }
      }
    }
    await chat.loadChannelIdentities(chat.curChannel.id, true, currentIdentityTargetUserId.value);
    if (!closeDialog && savedIdentity) {
      editingIdentity.value = savedIdentity;
      identityDialogMode.value = 'edit';
      resetIdentityForm(savedIdentity);
    }
    if (closeDialog) {
      identityDialogVisible.value = false;
    }

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
    return true;
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '保存失败，请稍后重试';
    message.error(errMsg);
    return false;
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
    await chat.channelIdentityDelete(chat.curChannel.id, identity.id, currentIdentityTargetUserId.value);
    await chat.loadChannelIdentities(chat.curChannel.id, true, currentIdentityTargetUserId.value);
    message.success('已删除频道角色');
  } catch (error: any) {
    const errMsg = error?.response?.data?.error || '删除失败，请稍后重试';
    message.error(errMsg);
  }
};

const getMessageDisplayName = (message: any) => {
  // 仅在“编辑自己的消息”时使用本地编辑预览覆盖名称
  const editingPreview = editingPreviewMap.value[message?.id];
  const messageUserId = (
    message?.user?.id
    || message?.member?.user?.id
    || message?.member?.userId
    || message?.member?.user_id
    || message?.user_id
    || ''
  );
  const canOverrideIdentity = editingPreview?.isSelf && !!messageUserId && messageUserId === user.info.id;
  if (canOverrideIdentity && editingPreview.displayName) {
    return editingPreview.displayName;
  }
  return message?.identity?.displayName
    || message?.sender_member_name
    || message?.member?.nick
    || message?.user?.nick
    || message?.user?.name
    || '未知';
};

const resolveMessageAvatarSource = (message: any) => {
  // 仅在“编辑自己的消息”时使用本地编辑预览覆盖头像
  const editingPreview = editingPreviewMap.value[message?.id];
  const messageUserId = (
    message?.user?.id
    || message?.member?.user?.id
    || message?.member?.userId
    || message?.member?.user_id
    || message?.user_id
    || ''
  );
  const canOverrideIdentity = editingPreview?.isSelf && !!messageUserId && messageUserId === user.info.id;
  if (canOverrideIdentity && editingPreview.avatar) {
    return editingPreview.avatar;
  }
  const identitySnapshot = resolveMessageIdentitySnapshot(message);
  const candidates = [
    message?.identity?.avatarAttachment,
    (message as any)?.sender_identity_avatar_id,
    (message as any)?.sender_identity_avatar,
    (message as any)?.senderIdentityAvatarID,
    (message as any)?.senderIdentityAvatarId,
  ];
  for (const id of candidates) {
    if (id) {
      return resolveAttachmentUrl(id) || String(id);
    }
  }
  if (identitySnapshot?.isTemporary) {
    return '';
  }
  return message?.member?.avatar || message?.user?.avatar || '';
};

const getMessageAvatar = (message: any) => resolveMessageAvatarSource(message);

const getMessageAvatarMergeKey = (message: any) => {
  const avatarSrc = resolveMessageAvatarSource(message) || '';
  let avatarDecorations = cloneAvatarDecorations(
    message?.identity?.avatarDecorations || (message as any)?.sender_identity_decoration,
    message?.identity?.avatarDecoration || null,
  );
  if (avatarDecorations.length === 0) {
    const channelId = String(message?.channel?.id || chat.curChannel?.id || '').trim();
    const identityId = String(
      message?.identity?.id
      || (message as any)?.sender_identity_id
      || (message as any)?.senderRoleId
      || (message as any)?.sender_role_id
      || '',
    ).trim();
    if (channelId && identityId) {
      const liveIdentity = (chat.channelIdentities[channelId] || []).find((item) => item.id === identityId);
      avatarDecorations = cloneAvatarDecorations(liveIdentity?.avatarDecorations, liveIdentity?.avatarDecoration || null);
    }
  }
  return `${avatarSrc}__${JSON.stringify(avatarDecorations)}`;
};

const getMessageIdentityColor = (message: any) => {
  return normalizeHexColor(message?.identity?.color || message?.sender_identity_color || '') || '';
};

const getMessageIcMode = (message: any): 'ic' | 'ooc' => {
  if (chat.editing && chat.editing.messageId === message?.id) {
    return normalizeMessageIcMode(chat.editing.icMode);
  }
  const editingPreview = editingPreviewMap.value[message?.id];
  if (editingPreview && !editingPreview.isSelf) {
    return normalizeMessageIcMode(editingPreview.tone);
  }
  return normalizeMessageIcMode(message?.icMode ?? message?.ic_mode);
};

const getMessageTone = (message: any): 'ic' | 'ooc' | 'archived' => {
  if (message?.isArchived || message?.is_archived) {
    return 'archived';
  }
  return getMessageIcMode(message);
};

const getMessageAvatarRenderState = (message: any, mergedWithPrev = false) => resolveAvatarRenderState({
  avatarsEnabled: display.showAvatar,
  avatarVisibilityScope: display.settings.avatarVisibilityScope,
  icMode: getMessageIcMode(message),
  mergedWithPrev,
});

const getTypingPreviewAvatarRenderState = (preview: TypingPreviewItem) => resolveAvatarRenderState({
  avatarsEnabled: display.showAvatar,
  avatarVisibilityScope: display.settings.avatarVisibilityScope,
  icMode: preview.tone,
  mergedWithPrev: false,
});

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
  message: Message;
  createdAt: string;
  archivedAt: string;
  archivedBy: string;
  sender: {
    name: string;
    avatar?: string;
  };
}

const ARCHIVE_PAGE_SIZE = 30;
const archivedMessagesRaw = ref<ArchivedPanelMessage[]>([]);
const archivedMessages = ref<ArchivedPanelMessage[]>([]);
const archivedLoading = ref(false);
const archivedSearchQuery = ref('');
const archivedCurrentPage = ref(1);
const archivedTotalCount = ref(0);
const archivedHasMore = ref(false);
const archivedNextCursor = ref('');
const archivedRequestSeq = ref(0);

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
    message,
    createdAt: toIsoStringOrEmpty((message as any).createdAt ?? message.createdAt),
    archivedAt: toIsoStringOrEmpty((message as any).archivedAt ?? message.archivedAt),
    archivedBy: resolveUserNameById((message as any).archivedBy || ''),
    sender: {
      name: getMessageDisplayName(message),
      avatar: getMessageAvatar(message),
    },
  };
};

const archivedPageCount = computed(() => {
  const total = archivedTotalCount.value;
  if (total === 0) {
    return 1;
  }
  return Math.max(1, Math.ceil(total / ARCHIVE_PAGE_SIZE));
});

const updateArchivedDisplay = () => {
  archivedMessages.value = [...archivedMessagesRaw.value];
  if (!archivedSearchQuery.value.trim()) {
    archivedTotalCount.value = archivedMessagesRaw.value.length;
  }
};

watch(
  archivedMessagesRaw,
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
      await fetchArchivedMessages(true);
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
      await fetchArchivedMessages(true);
    }
    await fetchLatestMessages();
  } catch (error) {
    const errMsg = (error as Error)?.message || '恢复失败';
    message.error(errMsg);
  }
};

const handleDeleteArchivedMessages = async (messageIds: string[]) => {
  try {
    await chat.removeMessages(messageIds);
    message.success('消息已删除');
    if (archiveDrawerVisible.value) {
      await fetchArchivedMessages(true);
    }
    await fetchLatestMessages();
  } catch (error) {
    const errMsg = (error as Error)?.message || '删除失败';
    message.error(errMsg);
  }
};

const delay = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

const logUploadConfig = computed(() => utils.config?.logUpload);
const hasLogUploadEndpoints = (config?: { endpoint?: string; endpoints?: string[]; enabled?: boolean } | null) => {
  if (!config || config.enabled === false) {
    return false;
  }
  const targets = [config.endpoint, ...(config.endpoints || [])]
    .map((item) => (item || '').trim())
    .filter((item, index, list) => !!item && list.indexOf(item) === index);
  return targets.length > 0;
};
const canUseCloudUpload = computed(() => hasLogUploadEndpoints(logUploadConfig.value));

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

const showBatchCloudUploadDialog = (items: CloudUploadResult[]) => {
  const links = items.filter(item => !!item?.url);
  if (!links.length) {
    message.warning('云端染色返回异常，未提供链接');
    return;
  }
  dialog.success({
    title: `云端日志已上传（${links.length} 个）`,
    positiveText: '知道了',
    content: () => (
      <div class="cloud-upload-result">
        {links.map((item, index) => (
          <p key={`${item.url}-${index}`}>
            {item.name || item.file_name || `频道 ${index + 1}`}：
            <a href={item.url} target="_blank" rel="noopener">{item.url}</a>
          </p>
        ))}
      </div>
    ),
  });
};

const refreshExportManager = (opts?: { revealLatestTask?: boolean }) => {
  exportManagerRefreshVersion.value += 1;
  if (opts?.revealLatestTask) {
    exportManagerRevealVersion.value += 1;
  }
};

const pollExportTask = async (taskId: string, opts?: { autoUpload?: boolean; batchUpload?: boolean; format?: string }) => {
  const maxAttempts = 30;
  const interval = 2000;
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    try {
      const status = await chat.getExportTaskStatus(taskId);
      if (status.status === 'done') {
        refreshExportManager();
        message.success('导出完成，正在下载文件');
        const { blob, fileName } = await chat.downloadExportResult(taskId, status.file_name);
        triggerBlobDownload(blob, fileName);
        if (opts?.autoUpload) {
          try {
            if (opts.batchUpload) {
              const uploadResp = await chat.uploadBatchExportTask(taskId);
              showBatchCloudUploadDialog(uploadResp?.items || []);
            } else {
              const uploadResp = await chat.uploadExportTask(taskId);
              if (uploadResp?.url) {
                showCloudUploadDialog(uploadResp);
              } else {
                message.warning('云端染色返回结果异常，未提供链接');
              }
            }
          } catch (error: any) {
            const errMsg = error?.response?.data?.error || (error as Error)?.message || '未知错误';
            message.warning(`云端染色上传失败：${errMsg}`);
          }
        }
        return;
      }
      if (status.status === 'failed') {
        refreshExportManager();
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
  includeImages: boolean;
  removeDiceCommands: boolean;
  withoutTimestamp: boolean;
  mergeMessages: boolean;
  autoCorrectPunctuation: boolean;
  textColorizeBBCode: boolean;
  textColorizeBBCodeMap?: Record<string, string>;
  textColorizeBBCodeNameMap?: Record<string, string>;
  autoUpload: boolean;
  maxExportMessages: number;
  maxExportConcurrency: number;
  channelIds?: string[];
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
      includeImages: params.includeImages,
      includeDiceCommands: !params.removeDiceCommands,
      withoutTimestamp: params.withoutTimestamp,
      mergeMessages: params.mergeMessages,
      autoCorrectPunctuation: params.autoCorrectPunctuation,
      textColorizeBBCode: params.textColorizeBBCode && params.format === 'txt',
      textColorizeBBCodeMap: params.textColorizeBBCode && params.format === 'txt'
        ? (params.textColorizeBBCodeMap || {})
        : undefined,
      textColorizeBBCodeNameMap: params.textColorizeBBCode && params.format === 'txt'
        ? (params.textColorizeBBCodeNameMap || {})
        : undefined,
      sliceLimit,
      maxConcurrency,
      displaySettings: displayOptions,
    };
    const channelIds = Array.from(new Set((params.channelIds || []).map(id => String(id || '').trim()).filter(Boolean)));
    const isBatchExport = channelIds.length > 0;
    const result = isBatchExport
      ? await chat.createBatchExportTask({ ...payload, channelIds })
      : await chat.createExportTask(payload);
    message.info(`${isBatchExport ? '批量导出' : '导出'}任务已创建（#${result.task_id}），正在生成文件…`);
    refreshExportManager({ revealLatestTask: true });
    exportDialogVisible.value = false;
    const shouldAutoUpload = Boolean(params.autoUpload && params.format === 'json' && canUseCloudUpload.value);
    void pollExportTask(result.task_id, { autoUpload: shouldAutoUpload, batchUpload: isBatchExport, format: params.format });
  } catch (error: any) {
    console.error('导出失败', error);
    const errMsg = error?.response?.data?.error || (error as Error)?.message || '导出失败';
    message.error(errMsg);
  }
};

const handleArchivePageChange = (page: number) => {
  if (!archivedSearchQuery.value.trim()) {
    return;
  }
  archivedCurrentPage.value = page;
  void fetchArchivedMessages(false);
};

const handleArchiveSearchChange = (keyword: string) => {
  archivedSearchQuery.value = keyword;
  archivedCurrentPage.value = 1;
  void fetchArchivedMessages(true);
};

const toArchivedPanelEntryFromSearch = (item: ChannelSearchResult): ArchivedPanelMessage => {
  const rawMessage = {
    id: item.id,
    content: item.content || item.contentSnippet || '',
    channel_id: item.channelId || chat.curChannel?.id || '',
    created_at: item.createdAt,
    archived_at: item.archivedAt,
    archived_by: item.archivedBy || '',
    is_archived: true,
    ic_mode: item.icMode,
    sender_member_name: item.senderName,
    user: item.senderId || item.senderAvatar
      ? { id: item.senderId || '', nickname: item.senderName, avatar: item.senderAvatar || '' }
      : undefined,
  };
  return toArchivedPanelEntry(normalizeMessageShape(rawMessage));
};

const sortArchivedPanelMessages = (items: ArchivedPanelMessage[]) => items.sort((a, b) => {
  const archivedDiff = (normalizeTimestamp(b.archivedAt) ?? 0) - (normalizeTimestamp(a.archivedAt) ?? 0);
  if (archivedDiff !== 0) {
    return archivedDiff;
  }
  const createdDiff = (normalizeTimestamp(b.createdAt) ?? 0) - (normalizeTimestamp(a.createdAt) ?? 0);
  if (createdDiff !== 0) {
    return createdDiff;
  }
  return b.id.localeCompare(a.id);
});

const fetchArchivedMessages = async (reset = true) => {
  if (!chat.curChannel?.id) {
    archivedMessagesRaw.value = [];
    archivedMessages.value = [];
    archivedTotalCount.value = 0;
    archivedHasMore.value = false;
    archivedNextCursor.value = '';
    return;
  }
  const channelId = chat.curChannel.id;
  const keyword = archivedSearchQuery.value.trim();
  const requestSeq = ++archivedRequestSeq.value;
  archivedLoading.value = true;
  try {
    if (keyword) {
      const result = await channelSearch.fetchChannelSearch(channelId, {
        keyword,
        match_mode: 'fuzzy',
        page: archivedCurrentPage.value,
        page_size: ARCHIVE_PAGE_SIZE,
        archived: 'only',
      });
      if (requestSeq !== archivedRequestSeq.value || chat.curChannel?.id !== channelId) {
        return;
      }
      archivedMessagesRaw.value = result.items.map(toArchivedPanelEntryFromSearch);
      archivedTotalCount.value = result.total;
      archivedHasMore.value = false;
      archivedNextCursor.value = '';
      return;
    }

    if (reset) {
      archivedMessagesRaw.value = [];
      archivedCurrentPage.value = 1;
      archivedNextCursor.value = '';
      archivedHasMore.value = false;
    }
    const resp = await chat.messageList(channelId, archivedNextCursor.value || undefined, {
      includeArchived: true,
      archivedOnly: true,
      includeOoc: true,
      limit: ARCHIVE_PAGE_SIZE,
    });
    if (requestSeq !== archivedRequestSeq.value || chat.curChannel?.id !== channelId) {
      return;
    }
    const mapped = (resp?.data ?? [])
      .map((item: any) => normalizeMessageShape(item))
      .map((item: Message) => toArchivedPanelEntry(item));
    archivedMessagesRaw.value = sortArchivedPanelMessages(
      reset ? mapped : [...archivedMessagesRaw.value, ...mapped],
    );
    archivedNextCursor.value = String(resp?.next || '');
    archivedHasMore.value = Boolean(archivedNextCursor.value);
    archivedTotalCount.value = archivedMessagesRaw.value.length;
  } catch (error) {
    if (requestSeq !== archivedRequestSeq.value) {
      return;
    }
    console.error('加载归档消息失败', error);
    if (archiveDrawerVisible.value) {
      message.error('加载归档消息失败');
    }
  } finally {
    if (requestSeq === archivedRequestSeq.value) {
      archivedLoading.value = false;
    }
  }
};

watch(archiveDrawerVisible, (visible) => {
  if (visible) {
    archivedSearchQuery.value = '';
    archivedCurrentPage.value = 1;
    void fetchArchivedMessages(true);
  }
});

watch(() => chat.curChannel?.id, () => {
  archivedMessagesRaw.value = [];
  archivedMessages.value = [];
  archivedSearchQuery.value = '';
  archivedCurrentPage.value = 1;
  archivedTotalCount.value = 0;
  archivedHasMore.value = false;
  archivedNextCursor.value = '';
});

const SCROLL_STICKY_THRESHOLD = 200;
const INITIAL_MESSAGE_LOAD_LIMIT = 30;
const PAGINATED_MESSAGE_LOAD_LIMIT = 20;
const SEARCH_ANCHOR_WINDOW_LIMIT = 12;
const SEARCH_ANCHOR_WINDOW_MAX = 24;
const SEARCH_CONTEXT_ROW_ESTIMATE = 72;
const SEARCH_JUMP_LIMIT_PRIMARY = 30;
const SEARCH_JUMP_LIMIT_RETRY = 50;
const HISTORY_PAGINATION_WINDOW_MS = 5 * 60 * 1000;
const HISTORY_WINDOW_EXPANSION_LIMIT = 5;

type ViewMode = 'live' | 'history';

const rows = ref<Message[]>([]);
const pinnedRows = ref<Message[]>([]);
const pinnedCollapseStorageKey = 'sealchat.pinnedCollapsed';
const resolvePinnedCollapsed = () => localStorage.getItem(pinnedCollapseStorageKey) === 'true';
const pinnedCollapsed = ref(resolvePinnedCollapsed());
const listRevision = ref(0);
const searchBrowseSession = reactive({
  active: false,
  channelId: '',
  anchorMessageId: '',
  beforeCursor: '',
  afterCursor: '',
  hasMoreBefore: false,
  hasMoreAfter: false,
});
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

watch(pinnedCollapsed, (collapsed) => {
  localStorage.setItem(pinnedCollapseStorageKey, String(collapsed));
});

interface ResetWindowOptions {
  preserveRows?: boolean;
  preserveHistoryLock?: boolean;
  preserveSearchSession?: boolean;
}

const resetSearchBrowseSession = () => {
  searchBrowseSession.active = false;
  searchBrowseSession.channelId = '';
  searchBrowseSession.anchorMessageId = '';
  searchBrowseSession.beforeCursor = '';
  searchBrowseSession.afterCursor = '';
  searchBrowseSession.hasMoreBefore = false;
  searchBrowseSession.hasMoreAfter = false;
};

const resetWindowState = (mode: ViewMode = 'live', options: ResetWindowOptions = {}) => {
  if (!options.preserveRows) {
    rows.value = [];
  }
  if (!options.preserveSearchSession) {
    resetSearchBrowseSession();
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

const isSearchBrowseActive = () => searchBrowseSession.active && searchBrowseSession.channelId === (chat.curChannel?.id || '');

const activateSearchBrowseSession = (
  payload: { messageId: string },
  options: {
    beforeCursor?: string | null;
    afterCursor?: string | null;
    hasMoreBefore?: boolean;
    hasMoreAfter?: boolean;
  } = {},
) => {
  searchBrowseSession.active = true;
  searchBrowseSession.channelId = chat.curChannel?.id || '';
  searchBrowseSession.anchorMessageId = payload.messageId;
  searchBrowseSession.beforeCursor = options.beforeCursor ?? messageWindow.beforeCursor;
  searchBrowseSession.afterCursor = options.afterCursor ?? messageWindow.afterCursor;
  searchBrowseSession.hasMoreBefore =
    typeof options.hasMoreBefore === 'boolean'
      ? options.hasMoreBefore
      : Boolean(searchBrowseSession.beforeCursor);
  searchBrowseSession.hasMoreAfter =
    typeof options.hasMoreAfter === 'boolean'
      ? options.hasMoreAfter
      : !messageWindow.hasReachedLatest;
};

const applyCursorUpdate = (cursor?: { before?: string | null; after?: string | null }) => {
  if (!cursor) return;
  if (cursor.before !== undefined) {
    messageWindow.beforeCursor = cursor.before || '';
    messageWindow.beforeCursorExhausted = !messageWindow.beforeCursor;
    if (isSearchBrowseActive()) {
      searchBrowseSession.beforeCursor = messageWindow.beforeCursor;
      searchBrowseSession.hasMoreBefore = Boolean(messageWindow.beforeCursor);
    }
    if (messageWindow.beforeCursor) {
      messageWindow.hasReachedStart = false;
    }
  }
  if (cursor.after !== undefined) {
    messageWindow.afterCursor = cursor.after || '';
    if (isSearchBrowseActive()) {
      searchBrowseSession.afterCursor = messageWindow.afterCursor;
    }
    if (messageWindow.afterCursor) {
      messageWindow.hasReachedLatest = false;
    }
  }
};

watch(viewMode, (mode) => {
  if (mode === 'live') {
    updateAnchorMessage(null);
    resetSearchBrowseSession();
  }
});

const updateWindowAnchorsFromRows = () => {
  if (!rows.value.length) {
    messageWindow.earliestTimestamp = null;
    messageWindow.latestTimestamp = null;
    messageWindow.afterCursor = '';
    if (isSearchBrowseActive()) {
      searchBrowseSession.afterCursor = '';
    }
    return;
  }
  const firstMessage = rows.value[0];
  const lastMessage = rows.value[rows.value.length - 1];
  const firstTs = normalizeTimestamp(firstMessage?.createdAt);
  const lastTs = normalizeTimestamp(lastMessage?.createdAt);
  if (firstTs !== null) {
    messageWindow.earliestTimestamp = firstTs;
  }
  if (lastTs !== null) {
    if (messageWindow.latestTimestamp === null || lastTs > messageWindow.latestTimestamp) {
      messageWindow.hasReachedLatest = false;
    }
    messageWindow.latestTimestamp = lastTs;
    messageWindow.afterCursor = buildMessageCursor(lastMessage as any);
  } else {
    messageWindow.afterCursor = '';
  }
  if (isSearchBrowseActive()) {
    searchBrowseSession.afterCursor = messageWindow.afterCursor;
  }
};

const resolveSearchAnchorWindowLimit = () => {
  const containerHeight =
    messagesListRef.value?.clientHeight ??
    (typeof window !== 'undefined' ? Math.round(window.innerHeight * 0.72) : 720);
  const estimatedVisibleRows = Math.max(8, Math.ceil(containerHeight / SEARCH_CONTEXT_ROW_ESTIMATE));
  return Math.max(
    SEARCH_ANCHOR_WINDOW_LIMIT,
    Math.min(SEARCH_ANCHOR_WINDOW_MAX, estimatedVisibleRows),
  );
};

const sortPinnedRows = () => {
  pinnedRows.value = pinnedRows.value
    .slice()
    .sort((a, b) => {
      const pinA = normalizeTimestamp((a as any).pinnedAt ?? (a as any).pinned_at) ?? 0;
      const pinB = normalizeTimestamp((b as any).pinnedAt ?? (b as any).pinned_at) ?? 0;
      if (pinA === pinB) {
        return compareByDisplayOrder(a, b);
      }
      return pinB - pinA;
    });
};

const removePinnedMessage = (messageId?: string) => {
  if (!messageId) {
    return;
  }
  pinnedRows.value = pinnedRows.value.filter((msg) => msg.id !== messageId);
};

const removeRevokedPlaceholderMessage = (messageId?: string) => {
  if (!messageId) {
    return;
  }
  rows.value = rows.value.filter((msg) => msg.id !== messageId);
  removePinnedMessage(messageId);
};

const upsertPinnedMessage = (incoming?: Message) => {
  if (!incoming || !incoming.id) {
    return;
  }
  const isPinned = Boolean((incoming as any).isPinned ?? (incoming as any).is_pinned ?? false);
  if (!isPinned || (incoming as any).isDeleted || (incoming as any).is_deleted) {
    removePinnedMessage(incoming.id);
    return;
  }
  const index = pinnedRows.value.findIndex((msg) => msg.id === incoming.id);
  if (index >= 0) {
    pinnedRows.value.splice(index, 1, {
      ...pinnedRows.value[index],
      ...incoming,
    });
  } else {
    pinnedRows.value.push(incoming);
  }
  sortPinnedRows();
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
const messageFilterSignature = computed(() => [
  chat.filterState.icFilter,
  chat.filterState.showArchived ? '1' : '0',
  chat.filterState.whisperOnly ? '1' : '0',
  chat.filterState.fromTime ?? '',
  chat.filterState.toTime ?? '',
  roleFilterSignature.value,
].join('|'));
const buildMessageFilterOptions = () => ({
  includeArchived: chat.filterState.showArchived,
  ...(chat.filterState.icFilter === 'ic' ? { icOnly: true } : {}),
  ...(chat.filterState.icFilter === 'ooc' ? { oocOnly: true } : {}),
  whisperOnly: chat.filterState.whisperOnly,
  ...(chat.filterState.fromTime !== null ? { fromTime: chat.filterState.fromTime } : {}),
  ...(chat.filterState.toTime !== null ? { toTime: chat.filterState.toTime } : {}),
  ...buildRoleFilterOptions(),
});
const intersectMessageFilterTimeWindow = (fromTime: number, toTime: number) => {
  const filterFromTime = chat.filterState.fromTime;
  const filterToTime = chat.filterState.toTime;
  const effectiveFromTime = filterFromTime === null ? fromTime : Math.max(fromTime, filterFromTime);
  const effectiveToTime = filterToTime === null ? toTime : Math.min(toTime, filterToTime);
  if (effectiveToTime < effectiveFromTime) {
    return null;
  }
  return { fromTime: effectiveFromTime, toTime: effectiveToTime };
};

const visibleRowEntries = computed<VisibleRowEntry[]>(() => {
  const {
    icFilter,
    showArchived,
    whisperOnly,
    fromTime,
    toTime,
  } = chat.filterState;
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

    if (whisperOnly && !(message as any)?.isWhisper) {
      return false;
    }

    const createdAt = normalizeTimestamp(message?.createdAt);
    if (fromTime !== null && (createdAt === null || createdAt < fromTime)) {
      return false;
    }
    if (toTime !== null && (createdAt === null || createdAt > toTime)) {
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
  return shouldMergeNeighborMessages(
    prev ? {
      ...prev,
      roleKey: getMessageRoleKey(prev),
      sceneKey: getMessageSceneKey(prev),
      avatarMergeKey: getMessageAvatarMergeKey(prev),
    } : null,
    current ? {
      ...current,
      roleKey: getMessageRoleKey(current),
      sceneKey: getMessageSceneKey(current),
      avatarMergeKey: getMessageAvatarMergeKey(current),
    } : null,
  );
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

const MAX_QUOTE_NORMALIZE_DEPTH = 8;

interface NormalizeMessageShapeContext {
  seen: WeakSet<object>;
  quoteDepth: number;
}

const createNormalizeMessageShapeContext = (): NormalizeMessageShapeContext => ({
  seen: new WeakSet<object>(),
  quoteDepth: 0,
});

const normalizeMessageShape = (msg: any, context: NormalizeMessageShapeContext = createNormalizeMessageShapeContext()): Message => {
  if (!msg) {
    return msg as Message;
  }
  if (typeof msg === 'object') {
    if (context.seen.has(msg)) {
      return {
        ...(msg as Record<string, unknown>),
        quote: undefined,
      } as Message;
    }
    context.seen.add(msg);
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
  if (msg.widgetData === undefined && msg.widget_data !== undefined) {
    msg.widgetData = msg.widget_data;
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
  if (msg.isPinned === undefined && msg.is_pinned !== undefined) {
    msg.isPinned = msg.is_pinned;
  }
  if (msg.archivedAt === undefined && msg.archived_at !== undefined) {
    msg.archivedAt = msg.archived_at;
  }
  if (msg.pinnedAt === undefined && msg.pinned_at !== undefined) {
    msg.pinnedAt = msg.pinned_at;
  }
  if (msg.archivedBy === undefined && msg.archived_by !== undefined) {
    msg.archivedBy = msg.archived_by;
  }
  if (msg.pinnedBy === undefined && msg.pinned_by !== undefined) {
    msg.pinnedBy = msg.pinned_by;
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
  const normalizedPinnedAt = normalizeTimestamp(msg.pinnedAt);
  msg.pinnedAt = normalizedPinnedAt ?? undefined;

  if (msg.quote) {
    if (context.quoteDepth >= MAX_QUOTE_NORMALIZE_DEPTH) {
      msg.quote = {
        ...(msg.quote as Record<string, unknown>),
        quote: undefined,
      };
    } else {
      msg.quote = normalizeMessageShape(msg.quote, {
        seen: context.seen,
        quoteDepth: context.quoteDepth + 1,
      });
    }
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

interface MessageInsertPlacement {
  anchorMessageId: string;
  beforeId: string;
  afterId?: string;
  localDisplayOrder: number;
  summary: string;
}

const activeMessageInsertTarget = computed(() => {
  const target = chat.messageInsertTarget;
  const channelId = String(chat.curChannel?.id || '').trim();
  if (!target.enabled || !channelId || target.channelId !== channelId || !target.messageId) {
    return null;
  }
  return target;
});

const messageInsertHintText = computed(() => {
  const target = activeMessageInsertTarget.value;
  if (!target) {
    return '';
  }
  return target.summary || '该消息';
});

const clearMessageInsertTarget = () => {
  const channelId = String(chat.curChannel?.id || '').trim();
  chat.clearMessageInsertTarget(channelId);
};

const resolveMessageInsertPlacement = (): MessageInsertPlacement | null => {
  const target = activeMessageInsertTarget.value;
  if (!target) {
    return null;
  }
  const anchorIndex = rows.value.findIndex((item) => item.id === target.messageId);
  if (anchorIndex < 0) {
    return null;
  }
  const anchor = rows.value[anchorIndex];
  if (!anchor?.id) {
    return null;
  }
  const beforeId = anchor.id;
  const afterId = rows.value[anchorIndex - 1]?.id || '';
  const beforeOrder = getMessageDisplayOrderValue(anchor);
  const afterOrder = getMessageDisplayOrderValue(rows.value[anchorIndex - 1]);
  let localDisplayOrder = beforeOrder ?? Date.now();
  if (afterOrder !== null && beforeOrder !== null) {
    localDisplayOrder = (afterOrder + beforeOrder) / 2;
  } else if (beforeOrder !== null) {
    localDisplayOrder = beforeOrder - 1;
  } else if (afterOrder !== null) {
    localDisplayOrder = afterOrder + 1;
  }
  return {
    anchorMessageId: target.messageId,
    beforeId,
    afterId: afterId || undefined,
    localDisplayOrder,
    summary: target.summary || '该消息',
  };
};

const validateMessageInsertTarget = (options?: { silent?: boolean }) => {
  const target = activeMessageInsertTarget.value;
  if (!target) {
    return true;
  }
  const placement = resolveMessageInsertPlacement();
  if (placement) {
    return true;
  }
  clearMessageInsertTarget();
  if (!options?.silent) {
    message.warning('目标消息已不可用，已恢复普通发送');
  }
  return false;
};

const shouldAutoScrollForSelfMessage = (messageData?: any) => !Boolean(messageData?.insertAboveTargetId);

const localReorderOps = new Set<string>();

const messageRowRefs = new Map<string, HTMLElement>();
const SEARCH_JUMP_WINDOWS_MS = [30, 120, 360, 1440, 10080].map((minutes) => minutes * 60 * 1000);
const searchJumping = ref(false);
const searchJumpRequestSeq = ref(0);

interface SearchJumpWindow {
  messages: Message[];
  cursorBefore?: string | null;
  cursorAfter?: string | null;
  hasMoreBefore?: boolean;
  hasMoreAfter?: boolean;
  fromTime?: number;
}

interface SearchJumpContextResult {
  messages: Message[];
  beforeCursor?: string;
  afterCursor?: string;
  hasMoreBefore?: boolean;
  hasMoreAfter?: boolean;
  notFoundReason?: string;
}

const createSearchJumpToken = () => {
  searchJumpRequestSeq.value += 1;
  return searchJumpRequestSeq.value;
};

const isSearchJumpTokenActive = (token: number) => token === searchJumpRequestSeq.value;

const searchHighlightIds = ref(new Set<string>());
const searchHighlightTimers = new Map<string, number>();
const sentConfirmHighlightIds = ref(new Set<string>());
const sentConfirmHighlightTimers = new Map<string, number>();
const sentConfirmDedupTimers = new Map<string, number>();

const SENT_CONFIRM_HIGHLIGHT_DURATION_MS = 1600;
const SENT_CONFIRM_DEDUP_DURATION_MS = 2500;

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

const setSentConfirmHighlight = (messageId: string, duration = SENT_CONFIRM_HIGHLIGHT_DURATION_MS) => {
  if (!messageId) return;
  if (sentConfirmHighlightTimers.has(messageId)) {
    window.clearTimeout(sentConfirmHighlightTimers.get(messageId));
  }
  const next = new Set(sentConfirmHighlightIds.value);
  next.add(messageId);
  sentConfirmHighlightIds.value = next;
  const timer = window.setTimeout(() => {
    const updated = new Set(sentConfirmHighlightIds.value);
    updated.delete(messageId);
    sentConfirmHighlightIds.value = updated;
    sentConfirmHighlightTimers.delete(messageId);
  }, duration);
  sentConfirmHighlightTimers.set(messageId, timer);
};

const resolveSentConfirmKey = (messageData?: any) => String(
  messageData?.clientId
  || messageData?.client_id
  || messageData?.id
  || ''
).trim();

const notifyNewMessageHighlight = (messageData?: any) => {
  if (!display.settings.highlightNewlySentMessage) {
    return;
  }
  const messageId = String(messageData?.id || '').trim();
  const dedupKey = resolveSentConfirmKey(messageData) || messageId;
  if (!messageId || !dedupKey) {
    return;
  }
  if (sentConfirmDedupTimers.has(dedupKey)) {
    return;
  }
  const timer = window.setTimeout(() => {
    sentConfirmDedupTimers.delete(dedupKey);
  }, SENT_CONFIRM_DEDUP_DURATION_MS);
  sentConfirmDedupTimers.set(dedupKey, timer);
  setSentConfirmHighlight(messageId);
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
  if (id === imageLayoutEditingMessageId) {
    if (el) {
      void ensureImageLayoutRowObserver();
    } else {
      disconnectImageLayoutRowObserver();
    }
  }
};

let imageLayoutEditingMessageId = "";
let imageLayoutRowResizeObserver: ResizeObserver | null = null;
let imageLayoutObservedRowEl: HTMLElement | null = null;
let imageLayoutObservedRowHeight = 0;

const disconnectImageLayoutRowObserver = () => {
  if (imageLayoutRowResizeObserver && imageLayoutObservedRowEl) {
    imageLayoutRowResizeObserver.unobserve(imageLayoutObservedRowEl);
  }
  imageLayoutObservedRowEl = null;
  imageLayoutObservedRowHeight = 0;
};

const disposeImageLayoutRowObserver = () => {
  disconnectImageLayoutRowObserver();
  if (imageLayoutRowResizeObserver) {
    imageLayoutRowResizeObserver.disconnect();
    imageLayoutRowResizeObserver = null;
  }
};

const ensureImageLayoutRowObserver = async () => {
  if (!imageLayoutEditingMessageId || typeof ResizeObserver === "undefined") {
    disconnectImageLayoutRowObserver();
    return;
  }
  await nextTick();
  const rowEl = messageRowRefs.get(imageLayoutEditingMessageId);
  if (!rowEl) {
    disconnectImageLayoutRowObserver();
    return;
  }
  if (!imageLayoutRowResizeObserver) {
    imageLayoutRowResizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) {
        return;
      }
      const nextHeight = entry.contentRect.height;
      if (nextHeight > imageLayoutObservedRowHeight && isNearBottom()) {
        scrollToBottom();
      }
      imageLayoutObservedRowHeight = nextHeight;
    });
  }
  if (imageLayoutObservedRowEl !== rowEl) {
    if (imageLayoutObservedRowEl) {
      imageLayoutRowResizeObserver.unobserve(imageLayoutObservedRowEl);
    }
    imageLayoutObservedRowEl = rowEl;
    imageLayoutObservedRowHeight = rowEl.getBoundingClientRect().height;
    imageLayoutRowResizeObserver.observe(rowEl);
  }
};

const handleImageLayoutEditStateChange = (payload?: { messageId?: string; active?: boolean }) => {
  const messageId = String(payload?.messageId || "").trim();
  const active = Boolean(payload?.active);
  if (!active) {
    if (!messageId || messageId === imageLayoutEditingMessageId) {
      imageLayoutEditingMessageId = "";
      disconnectImageLayoutRowObserver();
    }
    return;
  }
  if (!messageId) {
    return;
  }
  imageLayoutEditingMessageId = messageId;
  void ensureImageLayoutRowObserver();
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
  items.forEach((incoming) => {
    upsertPinnedMessage(incoming);
  });
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

const fetchPinnedMessages = async () => {
  if (!chat.curChannel?.id) {
    pinnedRows.value = [];
    return;
  }
  try {
    const list = await chat.pinnedMessageList(chat.curChannel.id, 20);
    pinnedRows.value = normalizeMessageList(list || []).filter((item) => {
      return Boolean((item as any).isPinned ?? (item as any).is_pinned ?? false);
    });
    sortPinnedRows();
  } catch (error) {
    pinnedRows.value = [];
    console.warn('加载置顶消息失败', error);
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
  const windowLimit = resolveSearchAnchorWindowLimit();
  if (sorted.length <= windowLimit * 2 + 1) {
    return sorted;
  }
  const start = Math.max(0, targetIndex - windowLimit);
  const end = Math.min(sorted.length, targetIndex + windowLimit + 1);
  return sorted.slice(start, end);
};

const applyHistoricalWindowFromMessages = (
  messages: Message[],
  payload: { messageId: string },
  options: {
    cursorBefore?: string | null;
    cursorAfter?: string | null;
    fromTime?: number;
    hasMoreBefore?: boolean;
    hasMoreAfter?: boolean;
    activateSearchBrowse?: boolean;
  } = {},
) => {
  const windowMessages = buildAnchorWindowMessages(messages, payload.messageId);
  if (!windowMessages) {
    return false;
  }
  resetWindowState('history');
  rows.value = windowMessages;
  sortRowsByDisplayOrder();
  applyCursorUpdate({
    before: options.hasMoreBefore === false ? '' : options.cursorBefore,
    after: options.cursorAfter,
  });
  computeAfterCursorFromRows();
  messageWindow.hasReachedStart = options.hasMoreBefore === false;
  if (options.fromTime !== undefined) {
    messageWindow.beforeCursorExhausted = !messageWindow.beforeCursor && options.fromTime === 0;
  }
  messageWindow.hasReachedLatest = false;
  if (options.activateSearchBrowse) {
    activateSearchBrowseSession(payload, {
      beforeCursor: options.hasMoreBefore === false ? '' : (options.cursorBefore ?? ''),
      afterCursor: options.cursorAfter,
      hasMoreBefore: options.hasMoreBefore,
      hasMoreAfter: options.hasMoreAfter,
    });
  }
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
      hasMoreBefore: Boolean(resp?.next),
      activateSearchBrowse: true,
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
      hasMoreBefore: Boolean(resp?.next),
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
      hasMoreBefore: Boolean(cursorBefore),
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

const loadJumpContextByMessageId = async (
  payload: { messageId: string },
): Promise<SearchJumpContextResult | null> => {
  if (!chat.curChannel?.id || !payload.messageId) {
    return null;
  }
  try {
    const contextWindow = resolveSearchAnchorWindowLimit();
    const resp = await chat.messageContext(chat.curChannel.id, payload.messageId, {
      before: contextWindow,
      after: contextWindow,
      includeArchived: true,
      includeOoc: true,
    });
    if (!resp) {
      return null;
    }
    const normalized = normalizeMessageList(Array.isArray(resp.data) ? resp.data : []);
    const containsTarget = normalized.some((msg) => msg.id === payload.messageId);
    if (containsTarget) {
      return {
        messages: normalized,
        beforeCursor: typeof resp.before_cursor === 'string' ? resp.before_cursor : '',
        afterCursor: typeof resp.after_cursor === 'string' ? resp.after_cursor : '',
        hasMoreBefore: resp.has_more_before === true,
        hasMoreAfter: resp.has_more_after === true,
      };
    }
    const notFoundReason = typeof resp.not_found_reason === 'string' ? resp.not_found_reason : '';
    if (notFoundReason) {
      return {
        messages: [],
        notFoundReason,
      };
    }
    return null;
  } catch (error) {
    console.warn('按消息上下文定位失败', error);
    return null;
  }
};

const ensureSearchTargetVisible = async (
  payload: { messageId: string; displayOrder?: number; createdAt?: number },
  requestToken: number,
) => {
  const isStale = () => !isSearchJumpTokenActive(requestToken);
  if (messageExistsLocally(payload.messageId)) {
    return true;
  }
  if (isStale()) {
    return false;
  }
  searchJumping.value = true;
  const loadingMsg = message.loading('正在定位消息…', { duration: 0 });
  try {
    const contextResult = await loadJumpContextByMessageId(payload);
    if (isStale()) {
      return false;
    }
    if (contextResult?.messages?.length) {
      const applied = applyHistoricalWindowFromMessages(contextResult.messages, payload, {
        cursorBefore: contextResult.hasMoreBefore ? contextResult.beforeCursor : '',
        cursorAfter: contextResult.afterCursor,
        hasMoreBefore: contextResult.hasMoreBefore,
        hasMoreAfter: contextResult.hasMoreAfter,
        activateSearchBrowse: true,
      });
      if (applied) {
        return true;
      }
    } else if (contextResult?.notFoundReason) {
      switch (contextResult.notFoundReason) {
        case 'deleted':
          message.warning('消息已被删除，无法跳转');
          break;
        case 'no_permission':
          message.warning('无法访问该消息，可能没有权限');
          break;
        case 'not_exists':
          message.warning('未找到该消息');
          break;
        default:
          message.warning('未能定位到该消息，可能已被删除或当前账号无权访问');
          break;
      }
      return false;
    }

    const mounted = await mountHistoricalWindow(payload);
    if (isStale()) {
      return false;
    }
    if (mounted) {
      return true;
    }
    const located = await locateMessageForJump(payload);
    if (isStale()) {
      return false;
    }
    if (!located) {
      message.warning('未能定位到该消息，可能已被删除或当前账号无权访问');
      return false;
    }
    const applied = applyHistoricalWindowFromMessages(located.messages, payload, {
      cursorBefore: located.cursorBefore,
      cursorAfter: located.cursorAfter,
      hasMoreBefore: located.hasMoreBefore,
      hasMoreAfter: located.hasMoreAfter,
      activateSearchBrowse: true,
      fromTime: located.fromTime,
    });
    if (!applied) {
      message.warning('仍未定位到该消息，稍后再试');
      return false;
    }
    return true;
  } finally {
    loadingMsg?.destroy?.();
    if (isSearchJumpTokenActive(requestToken)) {
      searchJumping.value = false;
    }
  }
};

const handleSearchJump = async (payload: { messageId: string; displayOrder?: number; createdAt?: number; channelId?: string }) => {
  const requestToken = createSearchJumpToken();
  const targetId = payload?.messageId;
  if (!targetId) {
    message.warning('未找到要跳转的消息');
    return false;
  }
  const targetChannelId = payload?.channelId;
  if (targetChannelId && targetChannelId !== chat.curChannel?.id) {
    const switched = await chat.channelSwitchTo(targetChannelId);
    if (!switched) {
      message.error('无法切换到目标频道，跳转已取消');
      return false;
    }
    if (!isSearchJumpTokenActive(requestToken)) {
      return false;
    }
  }

  // 如果没有 createdAt，尝试通过 API 获取消息详情，失败时继续走上下文定位
  let enrichedPayload = { ...payload };
  if (enrichedPayload.createdAt === undefined && chat.curChannel?.id) {
    try {
      const msgInfo = await chat.messageGetById(chat.curChannel.id, targetId);
      if (!isSearchJumpTokenActive(requestToken)) {
        return false;
      }
      if (msgInfo) {
        enrichedPayload.createdAt = msgInfo.created_at;
        enrichedPayload.displayOrder = msgInfo.display_order;
      }
    } catch (error) {
      console.warn('获取消息详情失败', error);
    }
  }

  await nextTick();
  if (!isSearchJumpTokenActive(requestToken)) {
    return false;
  }
  let target = messageRowRefs.get(targetId);
  if (!target) {
    const loaded = await ensureSearchTargetVisible(enrichedPayload, requestToken);
    if (!loaded) {
      return false;
    }
    await nextTick();
    if (!isSearchJumpTokenActive(requestToken)) {
      return false;
    }
    // 等待 DOM 渲染完成，最多重试几次
    for (let i = 0; i < 5; i++) {
      target = messageRowRefs.get(targetId);
      if (target) break;
      await new Promise(r => setTimeout(r, 50));
      if (!isSearchJumpTokenActive(requestToken)) {
        return false;
      }
    }
    if (!target) {
      if (messageExistsLocally(targetId)) {
        message.warning('消息已加载，但当前筛选条件可能将其隐藏，请调整筛选后重试');
      } else {
        message.warning('仍未定位到该消息，稍后再试');
      }
      return false;
    }
  }
  if (messagesListRef.value) {
    activateSearchBrowseSession(
      { messageId: targetId },
      {
        beforeCursor: searchBrowseSession.active ? searchBrowseSession.beforeCursor : messageWindow.beforeCursor,
        afterCursor: searchBrowseSession.active ? searchBrowseSession.afterCursor : messageWindow.afterCursor,
        hasMoreBefore: searchBrowseSession.active ? searchBrowseSession.hasMoreBefore : Boolean(messageWindow.beforeCursor),
        hasMoreAfter: searchBrowseSession.active ? searchBrowseSession.hasMoreAfter : !messageWindow.hasReachedLatest,
      },
    );
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
  if (isSearchJumpTokenActive(requestToken)) {
    searchJumping.value = false;
  }
  return true;
};

const initialMessageJumpReady = ref(false);
const consumingPendingMessageJump = ref(false);

const clearPendingMessageJumpQuery = async (pending: PendingMessageJump) => {
  if (route.name !== 'world-channel') {
    return;
  }
  const routeWorldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  const routeChannelId = typeof route.params.channelId === 'string' ? route.params.channelId.trim() : '';
  const routeMessageId = typeof route.query.msg === 'string' ? route.query.msg.trim() : '';
  if (
    routeWorldId !== pending.worldId
    || routeChannelId !== pending.channelId
    || routeMessageId !== pending.messageId
  ) {
    return;
  }
  const nextQuery = { ...route.query };
  delete nextQuery.msg;
  try {
    await router.replace({
      name: route.name,
      params: route.params,
      query: nextQuery,
    });
  } catch (error) {
    console.warn('[message-link] clear msg query failed', error);
  }
};

const consumePendingMessageJump = async () => {
  if (!initialMessageJumpReady.value || consumingPendingMessageJump.value) {
    return;
  }
  const pending = chat.pendingMessageJump;
  if (!pending) {
    return;
  }
  const currentWorldId = String(chat.currentWorldId || '').trim();
  const currentChannelId = String(chat.curChannel?.id || '').trim();
  if (pending.worldId !== currentWorldId || pending.channelId !== currentChannelId) {
    return;
  }
  consumingPendingMessageJump.value = true;
  try {
    const jumped = await handleSearchJump({
      messageId: pending.messageId,
      channelId: pending.channelId,
    });
    if (chat.pendingMessageJump?.requestKey !== pending.requestKey) {
      return;
    }
    chat.clearPendingMessageJump(pending.requestKey);
    if (jumped && pending.source === 'route') {
      await clearPendingMessageJumpQuery(pending);
    }
  } finally {
    consumingPendingMessageJump.value = false;
  }
};

watch(
  () => [
    initialMessageJumpReady.value,
    chat.pendingMessageJump?.requestKey,
    chat.currentWorldId,
    chat.curChannel?.id,
  ],
  () => {
    void consumePendingMessageJump();
  },
  { immediate: true },
);

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

const mobileMessageDragLongPressEnabled = computed(() => (
  isMobileUa && display.settings.mobileMessageDragLongPressEnabled === true
));

const pendingDragHoldState = reactive({
  item: null as Message | null,
  pointerId: null as number | null,
  startX: 0,
  startY: 0,
  lastClientY: 0,
  handleEl: null as HTMLElement | null,
  timerId: null as number | null,
});

const AUTO_SCROLL_EDGE_THRESHOLD = 60;
const AUTO_SCROLL_MIN_SPEED = 2;
const AUTO_SCROLL_MAX_SPEED = 18;
const MOBILE_MESSAGE_DRAG_LONG_PRESS_MS = 350;
const MOBILE_MESSAGE_DRAG_CANCEL_DISTANCE_PX = 10;

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

const clearPendingDragHoldTimer = () => {
  if (pendingDragHoldState.timerId === null || typeof window === 'undefined') {
    return;
  }
  window.clearTimeout(pendingDragHoldState.timerId);
  pendingDragHoldState.timerId = null;
};

const detachPendingDragHoldListeners = () => {
  window.removeEventListener('pointermove', onPendingDragHoldPointerMove);
  window.removeEventListener('pointerup', onPendingDragHoldPointerUp);
  window.removeEventListener('pointercancel', onPendingDragHoldPointerCancel);
  window.removeEventListener('blur', onPendingDragHoldWindowBlur);
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', onPendingDragHoldVisibilityChange);
  }
};

const resetPendingDragHoldState = () => {
  clearPendingDragHoldTimer();
  pendingDragHoldState.item = null;
  pendingDragHoldState.pointerId = null;
  pendingDragHoldState.startX = 0;
  pendingDragHoldState.startY = 0;
  pendingDragHoldState.lastClientY = 0;
  pendingDragHoldState.handleEl = null;
};

const cancelPendingDragHold = () => {
  detachPendingDragHoldListeners();
  resetPendingDragHoldState();
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
type LocalMessageSendStatus = 'sending' | 'sent' | 'failed';
const SEND_STATUS_SPINNER_DELAY_MS = 1000;
const sendStatusDelayTimers = new Map<string, ReturnType<typeof setTimeout>>();

const resolveMessageSendStatus = (messageData?: any): LocalMessageSendStatus => {
  const rawStatus = String(messageData?.sendStatus || '').toLowerCase();
  if (rawStatus === 'sending' || rawStatus === 'failed' || rawStatus === 'sent') {
    return rawStatus as LocalMessageSendStatus;
  }
  if (messageData?.failed === true) {
    return 'failed';
  }
  return 'sent';
};

const resolveSendStatusTimerKey = (messageData?: any) => {
  const key = String(
    messageData?.clientId
    || messageData?.client_id
    || messageData?.id
    || '',
  ).trim();
  return key;
};

const clearSendStatusDelayTimer = (messageData?: any) => {
  const key = resolveSendStatusTimerKey(messageData);
  if (!key) {
    return;
  }
  const timer = sendStatusDelayTimers.get(key);
  if (timer) {
    clearTimeout(timer);
    sendStatusDelayTimers.delete(key);
  }
};

const scheduleSendStatusDelayTimer = (messageData: any) => {
  const key = resolveSendStatusTimerKey(messageData);
  messageData.showSendIndicator = false;
  if (!key) {
    return;
  }
  clearSendStatusDelayTimer(messageData);
  const timer = setTimeout(() => {
    sendStatusDelayTimers.delete(key);
    if (resolveMessageSendStatus(messageData) !== 'sending') {
      return;
    }
    messageData.showSendIndicator = true;
  }, SEND_STATUS_SPINNER_DELAY_MS);
  sendStatusDelayTimers.set(key, timer);
};

const shouldRenderSendingIndicator = (messageData?: any) => (
  resolveMessageSendStatus(messageData) === 'sending' && messageData?.showSendIndicator === true
);

const setMessageSendStatus = (messageData: any, status: LocalMessageSendStatus, reason = '') => {
  if (!messageData) {
    return;
  }
  messageData.sendStatus = status;
  messageData.failed = status === 'failed';
  if (status === 'sending') {
    messageData.sendErrorReason = '';
    scheduleSendStatusDelayTimer(messageData);
    return;
  }
  clearSendStatusDelayTimer(messageData);
  messageData.showSendIndicator = false;
  messageData.sendErrorReason = status === 'failed'
    ? (reason || '发送失败，点击重试')
    : '';
};

const getMessageSendErrorReason = (messageData?: any) => {
  const reason = typeof messageData?.sendErrorReason === 'string' ? messageData.sendErrorReason.trim() : '';
  return reason || '发送失败，点击重试';
};

const shouldShowMessageSendStatus = (messageData?: Message) => {
  if (!messageData || !isSelfMessage(messageData)) {
    return false;
  }
  const status = resolveMessageSendStatus(messageData as any);
  if (status === 'failed') {
    return true;
  }
  if (status === 'sending') {
    return shouldRenderSendingIndicator(messageData as any);
  }
  return false;
};

const canRetrySendMessage = (messageData?: Message) => {
  if (!messageData || !isSelfMessage(messageData)) {
    return false;
  }
  return resolveMessageSendStatus(messageData as any) === 'failed';
};

const resolveMessageSendFailureReason = (error: any): string => {
  const respErr = String(error?.response?.err || error?.response?.error || '').trim();
  const raw = String(error?.message || respErr || '').trim();
  if (!raw) {
    return '服务器未返回成功确认';
  }
  const lower = raw.toLowerCase();
  if (lower.includes('timeout')) {
    return '发送超时，服务器未返回成功确认';
  }
  if (lower.includes('returned empty result')) {
    return '服务器未返回成功确认，可能没有发送权限';
  }
  if (lower.includes('ws not connected') || lower.includes('ws connection') || lower.includes('disconnected')) {
    return '连接已断开，请等待重连后重试';
  }
  if (raw.includes('没有权限') || lower.includes('forbidden') || lower.includes('permission')) {
    return '你可能没有权限在此频道发送消息';
  }
  return raw;
};

const resolveWhisperTargetIdsFromMessage = (messageData: any): string[] => {
  const resolved = new Set<string>();
  const collect = (raw: any) => {
    if (!Array.isArray(raw)) {
      return;
    }
    raw.forEach((item) => {
      const candidate = typeof item === 'string'
        ? item
        : (item?.id || item?.userId || item?.user_id || '');
      const id = String(candidate || '').trim();
      if (id) {
        resolved.add(id);
      }
    });
  };
  collect(messageData?.whisperToIds);
  collect(messageData?.whisper_to_ids);
  collect(messageData?.whisper_targets);
  collect(messageData?.whisperMeta?.targetUserIds);
  const direct = String(
    messageData?.whisperTo?.id
    || messageData?.whisper_to
    || messageData?.whisperMeta?.targetUserId
    || '',
  ).trim();
  if (direct) {
    resolved.add(direct);
  }
  return Array.from(resolved);
};

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
const shouldShowInlineHeader = (entry: VisibleRowEntry) => !entry.mergedWithPrev || shouldShowMessageSendStatus(entry.message);

const rowClass = (item: Message) => ({
  'message-row': true,
  'message-row--self': isSelfMessage(item),
  'draggable-item': canDragMessage(item),
  'message-row--drag-source': dragState.activeId === item.id,
  'message-row--drop-before': dragState.overId === item.id && dragState.position === 'before',
  'message-row--drop-after': dragState.overId === item.id && dragState.position === 'after',
  'message-row--search-hit': searchHighlightIds.value.has(item.id || ''),
  'message-row--sent-confirm-hit': sentConfirmHighlightIds.value.has(item.id || ''),
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
  ghost.setAttribute('data-sc-font-surface', 'true');
  
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

const onDragWindowBlur = () => {
  if (!dragState.activeId) {
    return;
  }
  cancelDrag();
};

const onDragVisibilityChange = () => {
  if (typeof document === 'undefined') {
    return;
  }
  if (document.visibilityState !== 'hidden' || !dragState.activeId) {
    return;
  }
  cancelDrag();
};

const onDragHandleLostPointerCapture = () => {
  if (!dragState.activeId) {
    return;
  }
  cancelDrag();
};

const detachDragListeners = () => {
  window.removeEventListener('pointermove', onDragPointerMove);
  window.removeEventListener('pointerup', onDragPointerUp);
  window.removeEventListener('pointercancel', onDragPointerCancel);
  window.removeEventListener('keydown', onDragKeyDown);
  window.removeEventListener('blur', onDragWindowBlur);
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', onDragVisibilityChange);
  }
  dragState.handleEl?.removeEventListener('lostpointercapture', onDragHandleLostPointerCapture);
};

const restoreMessageOrderFromSnapshot = (messageId: string, snapshot: Message[]) => {
  if (!messageId || snapshot.length === 0) {
    return;
  }
  const source = snapshot.find((item) => item.id === messageId);
  if (!source) {
    return;
  }
  const previousOrder = getMessageDisplayOrderValue(source);
  const index = rows.value.findIndex((item) => item.id === messageId);
  if (index < 0) {
    return;
  }
  const current = rows.value[index];
  if (previousOrder === null) {
    delete (current as any).displayOrder;
    delete (current as any).display_order;
  } else {
    (current as any).displayOrder = previousOrder;
    (current as any).display_order = previousOrder;
  }
  rows.value.splice(index, 1, current);
  sortRowsByDisplayOrder();
};

const cancelDrag = () => {
  cancelPendingDragHold();
  detachDragListeners();
  stopAutoScroll();
  resetDragState();
};

const finalizeDrag = async () => {
  const channelId = chat.curChannel?.id;
  const activeId = dragState.activeId;
  const overId = dragState.overId;
  const position = dragState.position;
  const originalRows = dragState.snapshot.slice();

  detachDragListeners();

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
    chatEvent.emit('battle-report-display-message-reordered' as any, { channelId });
  } catch (error) {
    restoreMessageOrderFromSnapshot(activeId, originalRows);
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

const startMessageDrag = ({
  item,
  pointerId,
  clientY,
  handleEl,
}: {
  item: Message;
  pointerId: number;
  clientY: number;
  handleEl: HTMLElement | null;
}) => {
  if (!canDragMessage(item) || !item.id) {
    return;
  }
  const rowEl = messageRowRefs.get(item.id);
  if (!rowEl) {
    return;
  }
  if (handleEl) {
    dragState.handleEl = handleEl;
    try {
      handleEl.setPointerCapture?.(pointerId);
    } catch {
      // ignore capture failure
    }
  }
  dragState.snapshot = rows.value.slice();
  dragState.clientOpId = nanoid();
  dragState.activeId = item.id;
  dragState.pointerId = pointerId;
  dragState.startY = clientY;
  dragState.overId = item.id;
  dragState.position = 'after';
  dragState.originEl = rowEl;
  document.body.style.userSelect = 'none';
  
  // IMPORTANT: Create ghost BEFORE adding drag-source class (which collapses the row)
  createGhostElement(rowEl);
  
  // Now add the collapse class
  rowEl.classList.add('message-row--drag-source');
  
  updateOverTarget(clientY);
  updateAutoScroll(clientY);

  window.addEventListener('pointermove', onDragPointerMove);
  window.addEventListener('pointerup', onDragPointerUp);
  window.addEventListener('pointercancel', onDragPointerCancel);
  window.addEventListener('keydown', onDragKeyDown);
  window.addEventListener('blur', onDragWindowBlur);
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onDragVisibilityChange);
  }
  handleEl?.addEventListener('lostpointercapture', onDragHandleLostPointerCapture);
};

const onPendingDragHoldPointerMove = (event: PointerEvent) => {
  if (event.pointerId !== pendingDragHoldState.pointerId) {
    return;
  }
  pendingDragHoldState.lastClientY = event.clientY;
  const deltaX = event.clientX - pendingDragHoldState.startX;
  const deltaY = event.clientY - pendingDragHoldState.startY;
  if (Math.hypot(deltaX, deltaY) > MOBILE_MESSAGE_DRAG_CANCEL_DISTANCE_PX) {
    cancelPendingDragHold();
  }
};

const onPendingDragHoldPointerUp = (event: PointerEvent) => {
  if (event.pointerId !== pendingDragHoldState.pointerId) {
    return;
  }
  cancelPendingDragHold();
};

const onPendingDragHoldPointerCancel = (event: PointerEvent) => {
  if (event.pointerId !== pendingDragHoldState.pointerId) {
    return;
  }
  cancelPendingDragHold();
};

const onPendingDragHoldWindowBlur = () => {
  if (pendingDragHoldState.pointerId === null) {
    return;
  }
  cancelPendingDragHold();
};

const onPendingDragHoldVisibilityChange = () => {
  if (typeof document === 'undefined' || document.visibilityState !== 'hidden') {
    return;
  }
  cancelPendingDragHold();
};

const schedulePendingDragHold = (event: PointerEvent, item: Message) => {
  cancelPendingDragHold();
  pendingDragHoldState.item = item;
  pendingDragHoldState.pointerId = event.pointerId;
  pendingDragHoldState.startX = event.clientX;
  pendingDragHoldState.startY = event.clientY;
  pendingDragHoldState.lastClientY = event.clientY;
  pendingDragHoldState.handleEl = event.currentTarget as HTMLElement | null;
  window.addEventListener('pointermove', onPendingDragHoldPointerMove);
  window.addEventListener('pointerup', onPendingDragHoldPointerUp);
  window.addEventListener('pointercancel', onPendingDragHoldPointerCancel);
  window.addEventListener('blur', onPendingDragHoldWindowBlur);
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onPendingDragHoldVisibilityChange);
  }
  if (typeof window !== 'undefined') {
    pendingDragHoldState.timerId = window.setTimeout(() => {
      const itemToDrag = pendingDragHoldState.item;
      const pointerId = pendingDragHoldState.pointerId;
      const handleEl = pendingDragHoldState.handleEl;
      const clientY = pendingDragHoldState.lastClientY || pendingDragHoldState.startY;
      detachPendingDragHoldListeners();
      resetPendingDragHoldState();
      if (!itemToDrag || pointerId === null) {
        return;
      }
      startMessageDrag({
        item: itemToDrag,
        pointerId,
        clientY,
        handleEl,
      });
    }, MOBILE_MESSAGE_DRAG_LONG_PRESS_MS);
  }
};

const onDragHandlePointerDown = (event: PointerEvent, item: Message) => {
  if (!canDragMessage(item) || !item.id) {
    return;
  }
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return;
  }
  if (mobileMessageDragLongPressEnabled.value && event.pointerType !== 'mouse') {
    schedulePendingDragHold(event, item);
    return;
  }
  startMessageDrag({
    item,
    pointerId: event.pointerId,
    clientY: event.clientY,
    handleEl: event.currentTarget as HTMLElement | null,
  });

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

function handleBattleReportDisplayRefresh(payload: any) {
  const channelId = String(payload?.channelId || '').trim();
  if (!channelId || channelId !== chat.curChannel?.id) {
    return;
  }
  scheduleLatestMessagesRefetch();
}

const recordIdentitySpokenFromMessage = (message?: Message) => {
  if (!message) {
    return;
  }
  const identityId = resolveMessageIdentityId(message);
  if (!identityId) {
    return;
  }
  const channelId = String((message as any)?.channel?.id || chat.curChannel?.id || '').trim();
  if (!channelId) {
    return;
  }
  const spokenAt = normalizeTimestamp((message as any)?.createdAt) ?? Date.now();
  chat.recordIdentitySpoken(channelId, identityId, spokenAt);
};

const normalizeMessageList = (items: any[] = []): Message[] =>
  items
    .map((item) => {
      const normalized = normalizeMessageShape(item);
      recordIdentitySpokenFromMessage(normalized);
      return normalized;
    })
    .filter((item) => !(item as any)?.is_deleted);

const upsertMessage = (incoming?: Message) => {
  if (!incoming || !incoming.id) {
    return;
  }
  if ((incoming as any).is_deleted || (incoming as any).isDeleted) {
    rows.value = rows.value.filter((msg) => msg.id !== incoming.id);
    removePinnedMessage(incoming.id);
    updateWindowAnchorsFromRows();
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
  updateWindowAnchorsFromRows();
  upsertPinnedMessage(incoming);
};

async function replaceUsernames(text: string) {
  if (!text || !text.includes('@')) {
    return text;
  }
  const escapeAtAttrValue = (value: unknown) => String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  const regex = /@(\S+)/g;
  if (!regex.test(text)) {
    return text;
  }
  regex.lastIndex = 0;

  let resp: any;
  try {
    resp = await chat.guildMemberList('');
  } catch (error) {
    console.warn('replaceUsernames 拉取成员列表失败，跳过替换', error);
    return text;
  }
  const memberList = Array.isArray(resp?.data) ? resp.data : [];
  if (!memberList.length) {
    return text;
  }
  const infoMap = memberList.reduce<Record<string, any>>((obj, item) => {
    const nick = String(item?.nick || '').trim();
    if (nick) {
      obj[nick] = item;
    }
    return obj;
  }, {});

  if (Object.keys(infoMap).length === 0) {
    return text;
  }

  // 使用 replace 方法来替换匹配到的用户名
  const replacedText = text.replace(regex, (match, username) => {
    if (username in infoMap) {
      const info = infoMap[username];
      const safeId = escapeAtAttrValue(info.id);
      const safeNick = escapeAtAttrValue(info.nick);
      return `<at id="${safeId}" name="${safeNick}" />`
    }
    return match;
  });

  return replacedText;
}

const instantMessages = reactive(new Set<Message>());

// Typing preview composable initialized after textToSend.

const textToSend = ref('');
const {
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
  resetTypingPreview,
  resetDraftOrderContext,
  removeTypingPreview,
  removeSelfTypingPreview,
  upsertTypingPreview,
  syncSelfTypingPreview,
  editingPreviewActive,
  inputPreviewEnabled,
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
} = useTypingPreview({
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
  resolveIdentityVariantShortcutMatch: (...args) => resolveIdentityVariantShortcutMatch(...args),
  resolveIdentityAppearancePreview: (...args) => resolveIdentityAppearancePreview(...args),
  replaceEmojiRemarksForPreview,
  cloneAvatarDecorations: (...args) => cloneAvatarDecorations(...args),
  normalizeHexColor: (value) => normalizeHexColor(value),
  isContentMeaningful: (mode, content) => isContentMeaningful(mode, content),
  isNearBottom: () => isNearBottom(),
  scrollToBottom: () => scrollToBottom(),
});

const stripDiceChipMarkup = (html: string) => {
  if (!html || !html.includes('dice-chip')) {
    return html;
  }
  try {
    const parser = new DOMParser();
    const doc = parser.parseFromString(`<div>${html}</div>`, 'text/html');
    let replaced = false;
    doc.querySelectorAll('span.dice-roll-group').forEach((element) => {
      const source = element.getAttribute('data-dice-source') || '';
      if (!element.parentNode) return;
      const replacement = doc.createTextNode(source);
      element.parentNode.replaceChild(replacement, element);
      replaced = true;
    });
    doc.querySelectorAll('span').forEach((element) => {
      const classAttr = element.getAttribute('class') || '';
      if (!classAttr.includes('dice-chip')) {
        return;
      }
      const source = element.getAttribute('data-dice-source') || element.textContent || '';
      if (!element.parentNode) return;
      const replacement = doc.createTextNode(source);
      element.parentNode.replaceChild(replacement, element);
      replaced = true;
    });
    if (!replaced) {
      return html;
    }
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
  let text = contentUnescape(content);
  text = stripDiceChipMarkup(text);
  if (isTipTapJson(text)) {
    return text;
  }
  const imageRecords: Array<{ id: string; token: string; attachmentId: string }> = [];
  text = text.replace(/<img\s+[^>]*src="([^"]+)"[^>]*\/?>/gi, (_, src: string) => {
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
  text = text.replace(/<at\s+[^>]*name="([^"]+)"[^>]*\/>/gi, (_, name: string) => `@${name}`);
  text = text.replace(/<at\s+[^>]*id="([^"]+)"[^>]*\/>/gi, (_, id: string) => `@${id}`);
  text = restoreQuickFormatTextFromHtml(text);
  text = text.replace(/<br\s*\/?>/gi, '\n');
  return text;
};

const showAIPolish = computed(() => aiStore.isFeatureEnabled('polish'))
const showBattleSummary = computed(() => aiStore.isFeatureEnabled('battle_summary'))
const reeditRevokedSource = ref<{ channelId: string; messageId: string } | null>(null);

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
const LEGACY_HISTORY_AUTORESTORE_STORAGE_KEY = 'sealchat_input_history_autorestore_v1';
const HISTORY_SESSION_DRAFT_PREFIX = 'sealchat_input_session_draft_v1';
const HISTORY_SESSION_DRAFT_WINDOW_PREFIX = 'sealchat_input_session_draft_window_v1:';
const HISTORY_SESSION_DRAFT_TTL = 24 * 60 * 60 * 1000;

interface HistoryImageInfo {
  markerId: string;
  attachmentId: string;
}

interface SessionDraftEntry {
  mode: 'plain' | 'rich';
  content: string;
  updatedAt: number;
  images?: HistoryImageInfo[];
  whisperSnapshot?: WhisperSnapshot;
}

interface InputHistoryEntry {
  id: string;
  channelKey: string;
  mode: 'plain' | 'rich';
  content: string;
  createdAt: number;
  images?: HistoryImageInfo[];
  whisperSnapshot?: WhisperSnapshot;
}

type HistoryStore = Record<string, InputHistoryEntry[]>;

type SessionDraftStore = Record<string, SessionDraftEntry>;

interface HistoryEntryView extends InputHistoryEntry {
  preview: string;
  fullPreview: string;
  timeLabel: string;
}

const historyEntries = ref<InputHistoryEntry[]>([]);
const historyPopoverVisible = ref(false);
const hasHistoryEntries = computed(() => historyEntries.value.length > 0);
const currentChannelKey = computed(() => chat.curChannel?.id ? String(chat.curChannel.id) : HISTORY_CHANNEL_FALLBACK);
const draftOwnerChannelKey = ref(HISTORY_CHANNEL_FALLBACK);
const lastHistorySignature = ref<string | null>(null);

const resolveSessionDraftStorageKey = () => {
  if (typeof window === 'undefined') {
    return HISTORY_SESSION_DRAFT_PREFIX;
  }
  try {
    const hash = window.location.hash || '';
    if (!hash.startsWith('#/embed')) {
      return HISTORY_SESSION_DRAFT_PREFIX;
    }
    const queryIndex = hash.indexOf('?');
    if (queryIndex === -1) {
      return HISTORY_SESSION_DRAFT_PREFIX;
    }
    const params = new URLSearchParams(hash.slice(queryIndex + 1));
    const paneId = params.get('paneId')?.trim();
    if (!paneId) {
      return HISTORY_SESSION_DRAFT_PREFIX;
    }
    return `${HISTORY_SESSION_DRAFT_WINDOW_PREFIX}${paneId}`;
  } catch {
    return HISTORY_SESSION_DRAFT_PREFIX;
  }
};

const readSessionDraftStore = (): SessionDraftStore => {
  try {
    const raw = sessionStorage.getItem(resolveSessionDraftStorageKey());
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') {
      return parsed as SessionDraftStore;
    }
  } catch (e) {
    console.warn('读取会话草稿失败', e);
  }
  return {};
};

const writeSessionDraftStore = (store: SessionDraftStore) => {
  try {
    sessionStorage.setItem(resolveSessionDraftStorageKey(), JSON.stringify(store));
  } catch (e) {
    console.warn('写入会话草稿失败', e);
  }
};

const sanitizeSessionDraftStore = (store: SessionDraftStore) => {
  const now = Date.now();
  let changed = false;
  Object.keys(store).forEach((channelKey) => {
    const entry = store[channelKey];
    if (!entry || typeof entry !== 'object') {
      delete store[channelKey];
      changed = true;
      return;
    }
    if (!entry.content || typeof entry.content !== 'string') {
      delete store[channelKey];
      changed = true;
      return;
    }
    const updatedAt = typeof entry.updatedAt === 'number' ? entry.updatedAt : 0;
    if (!updatedAt || now - updatedAt > HISTORY_SESSION_DRAFT_TTL) {
      delete store[channelKey];
      changed = true;
    }
  });
  return changed;
};

const writeSessionDraftForChannel = (channelKey: string, draft: SessionDraftEntry | null) => {
  if (!channelKey || channelKey === HISTORY_CHANNEL_FALLBACK) {
    return;
  }
  const store = readSessionDraftStore();
  const changed = sanitizeSessionDraftStore(store);
  if (draft) {
    store[channelKey] = draft;
    writeSessionDraftStore(store);
    return;
  }
  if (store[channelKey]) {
    delete store[channelKey];
    writeSessionDraftStore(store);
    return;
  }
  if (changed) {
    writeSessionDraftStore(store);
  }
};

const readSessionDraftForChannel = (channelKey: string): SessionDraftEntry | null => {
  if (!channelKey || channelKey === HISTORY_CHANNEL_FALLBACK) {
    return null;
  }
  const store = readSessionDraftStore();
  const changed = sanitizeSessionDraftStore(store);
  if (changed) {
    writeSessionDraftStore(store);
  }
  const entry = store[channelKey];
  if (!entry || typeof entry.content !== 'string') {
    return null;
  }
  return {
    mode: entry.mode === 'rich' ? 'rich' : 'plain',
    content: entry.content,
    updatedAt: typeof entry.updatedAt === 'number' ? entry.updatedAt : Date.now(),
    images: Array.isArray(entry.images) ? entry.images : undefined,
    whisperSnapshot: normalizeWhisperSnapshot((entry as any).whisperSnapshot),
  };
};

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

const clearLegacyHistoryAutoRestoreStore = () => {
  try {
    localStorage.removeItem(LEGACY_HISTORY_AUTORESTORE_STORAGE_KEY);
  } catch (e) {
    console.warn('清理旧版历史自动恢复状态失败', e);
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
      const whisperSnapshot = normalizeWhisperSnapshot(entry.whisperSnapshot);
      return {
        id,
        channelKey,
        mode,
        content,
        createdAt,
        images,
        whisperSnapshot,
      } as InputHistoryEntry;
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
    ? buildInputHistorySignature(entries[0].mode, entries[0].content, entries[0].whisperSnapshot)
    : null;
};

const pruneAndPersist = (channelKey: string, entries: InputHistoryEntry[]) => {
  const store = readHistoryStore();
  store[channelKey] = entries.slice(0, MAX_HISTORY_PER_CHANNEL);
  writeHistoryStore(store);
  if (channelKey === currentChannelKey.value) {
    historyEntries.value = store[channelKey].slice();
    lastHistorySignature.value = historyEntries.value.length
      ? buildInputHistorySignature(
        historyEntries.value[0].mode,
        historyEntries.value[0].content,
        historyEntries.value[0].whisperSnapshot,
      )
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

const resolveAIPolishInput = () => {
  if (!isContentMeaningful(inputMode.value, textToSend.value)) {
    return '';
  }
  if (inputMode.value === 'plain') {
    return activeIdentityVariantShortcutContext.value.draftContent;
  }
  const raw = String(textToSend.value || '');
  if (!raw) {
    return '';
  }
  if (isTipTapJson(raw)) {
    return tiptapJsonToPlainText(raw);
  }
  return raw.trim();
};

const hasAIPolishInput = computed(() => resolveAIPolishInput().trim().length > 0);
const canRunAIPolish = computed(() => hasAIPolishInput.value);
const {
  aiPolishDockVisible,
  aiPolishDockState,
  aiPolishActiveSlot,
  aiPolishAnyLoading,
  aiPolishFaviconHref,
  runAIPolish,
  applyAIPolishResult,
  retryCurrentAIPolishTask,
  readCurrentInputIntoAIPolishSlot,
  updateActiveAIPolishResultText,
  updateActiveAIPolishSourceText,
  updateActiveAIPolishViewMode,
  clearCurrentAIPolishSlot,
  closeAIPolishDock,
  setActiveAIPolishSlot,
  toggleAIPolishDockMinimized,
} = useChatAIPolish({
  aiStore,
  chat,
  utils,
  message,
  dialog,
  inputMode,
  textToSend,
  textInputRef,
  resolveInput: resolveAIPolishInput,
  buildRichContentFromPlain,
});

const hasMeaningfulDraft = computed(() => (
  isEditing.value || isContentMeaningful(inputMode.value, textToSend.value)
));

type InterjectPhase = 'awaiting-first-send' | 'awaiting-second-send';

interface InterjectComposerSnapshot {
  channelId: string;
  icMode: 'ic' | 'ooc';
  identityId: string | null;
  identityVariantId: string | null;
}

interface InterjectEditSnapshot {
  messageId: string;
  channelId: string;
  originalContent: string;
  draft: string;
  mode: 'plain' | 'rich';
  isWhisper: boolean;
  whisperTargetId: string | null;
  whisperTargets: User[];
  icMode: 'ic' | 'ooc';
  identityId: string | null;
  identityVariantId: string | null;
  identitySnapshot: MessageIdentitySnapshot | null;
}

interface InterjectSession {
  phase: InterjectPhase;
  channelId: string;
  secondMode: 'ic' | 'ooc';
  firstEditSnapshot: InterjectEditSnapshot | null;
  returnComposerSnapshot: InterjectComposerSnapshot | null;
}

const interjectSession = ref<InterjectSession | null>(null);
const clearInterjectSession = () => {
  interjectSession.value = null;
};
const canStartInterject = computed(() => {
  if (interjectSession.value) {
    return false;
  }
  return shouldAllowInterject({
    isEditing: isEditing.value,
    isConnected: chat.connectState === 'connected',
    spectatorInputDisabled: spectatorInputDisabled.value,
    draftText: textToSend.value,
    hasMeaningfulDraft: !isEditing.value && hasMeaningfulDraft.value,
    hasUploadingInlineImages: hasUploadingInlineImages.value,
    hasFailedInlineImages: hasFailedInlineImages.value,
  });
});
const interjectTooltip = computed(() => {
  const phase = interjectSession.value?.phase;
  if (phase === 'awaiting-first-send') {
    return '插话中：正在发送第一条消息';
  }
  if (phase === 'awaiting-second-send') {
    return '插话中：发送下一条消息后回到首条编辑';
  }
  return '插话';
});
const interjectButtonType = computed(() => (
  interjectSession.value ? 'primary' : 'default'
));

function startInterject() {
  if (!chat.curChannel?.id) {
    message.warning('请先选择频道');
    return;
  }
  if (interjectSession.value) {
    return;
  }
  if (!canStartInterject.value) {
    message.warning('当前输入内容不可插话');
    return;
  }
  interjectSession.value = {
    phase: 'awaiting-first-send',
    channelId: chat.curChannel.id,
    secondMode: resolveInterjectTargetMode(inputIcMode.value, display.settings.interjectSwitchRule),
    firstEditSnapshot: null,
    returnComposerSnapshot: createInterjectComposerSnapshot(),
  };
  send();
  send.flush();
}

const handleInterjectSendSuccess = (
  sentMessage: Message,
  firstEditSnapshot?: InterjectEditSnapshot | null,
) => {
  const session = interjectSession.value;
  if (!session) {
    return;
  }
  const activeChannelId = String(chat.curChannel?.id || '').trim();
  if (!activeChannelId || session.channelId !== activeChannelId) {
    clearInterjectSession();
    return;
  }
  if (session.phase === 'awaiting-first-send') {
    session.firstEditSnapshot = firstEditSnapshot || createInterjectEditSnapshot({
      messageId: String(sentMessage?.id || '').trim(),
      channelId: activeChannelId,
      originalContent: sentMessage?.content || '',
      draft: sentMessage?.content || '',
      mode: detectMessageContentMode(sentMessage?.content),
      isWhisper: Boolean((sentMessage as any)?.isWhisper),
      whisperTargetId: resolveMessageWhisperTargetId(sentMessage),
      whisperTargets: resolveMessageWhisperTargets(sentMessage),
      icMode: String((sentMessage as any)?.icMode ?? (sentMessage as any)?.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic',
      identityId: resolveMessageIdentityId(sentMessage),
      identityVariantId: resolveIdentityVariantIdFromMessage(sentMessage),
      identitySnapshot: resolveMessageIdentitySnapshot(sentMessage),
    });
    if (!session.firstEditSnapshot) {
      clearInterjectSession();
      message.warning('插话首条消息快照记录失败');
      return;
    }
    session.phase = 'awaiting-second-send';
    inputIcMode.value = session.secondMode;
    nextTick(() => {
      ensureInputFocus();
    });
    return;
  }
  if (session.phase === 'awaiting-second-send') {
    const firstEditSnapshot = session.firstEditSnapshot;
    const returnComposerSnapshot = session.returnComposerSnapshot;
    clearInterjectSession();
    if (!firstEditSnapshot) {
      message.warning('插话首条消息快照缺失，无法进入编辑态');
      return;
    }
    if (returnComposerSnapshot) {
      restoreInterjectComposerSnapshot(returnComposerSnapshot);
    }
    restoreInterjectEditingSnapshot(firstEditSnapshot);
  }
};

const handleInterjectSendFailure = () => {
  if (interjectSession.value?.phase === 'awaiting-first-send') {
    clearInterjectSession();
  }
};

const getServerAlignedNowMs = () => {
  const localNow = Date.now();
  const offset = Number(chat.serverTimeOffsetMs);
  if (!Number.isFinite(offset)) {
    return localNow;
  }
  const alignedNow = Math.floor(localNow - offset);
  return Number.isFinite(alignedNow) && alignedNow > 0 ? alignedNow : localNow;
};

const syncDraftStartedAt = (content: string) => {
  if (isEditing.value) {
    draftStartedAtMs.value = null;
    return;
  }
  if (resolveConfiguredMessageSortBasis() !== 'typing_start') {
    draftStartedAtMs.value = null;
    return;
  }
  if (!isContentMeaningful(inputMode.value, content)) {
    resetDraftOrderContext();
    return;
  }
  if (
    typeof draftStartedAtMs.value !== 'number'
    || !Number.isFinite(draftStartedAtMs.value)
    || draftStartedAtMs.value <= 0
  ) {
    draftStartedAtMs.value = Date.now();
  }
};

interface SendDisplayOrderResolution {
  localDisplayOrder: number;
  explicitDisplayOrder?: number;
  typingDurationMs?: number;
}

type MessageSortBasis = 'typing_start' | 'send_time';

const resolveConfiguredMessageSortBasis = (): MessageSortBasis => {
  const raw = String((utils.config as any)?.messageSortBasis || '').trim().toLowerCase();
  return raw === 'send_time' ? 'send_time' : 'typing_start';
};

const resolveManualPreviewDisplayOrder = (fallbackNowMs: number): number | null => {
  if (!selfPreviewOrderModified.value) {
    return null;
  }
  const typingItems = typingPreviewItems.value.slice().sort((a, b) => {
    if (a.orderKey !== b.orderKey) {
      return a.orderKey - b.orderKey;
    }
    return String(a.userId || '').localeCompare(String(b.userId || ''));
  });
  if (typingItems.length === 0) {
    return null;
  }
  let rank = typingItems.findIndex((item) => item.userId === selfPreviewUserId.value && item.mode === 'typing');
  let count = typingItems.length;
  if (rank < 0) {
    rank = count;
    count += 1;
  }
  const configuredWindowMs = Number((utils.config as any)?.typingOrderWindowMs);
  const windowMs = Number.isFinite(configuredWindowMs) && configuredWindowMs > 0
    ? Math.floor(configuredWindowMs)
    : 1000;
  if (!Number.isFinite(windowMs) || windowMs <= 0) {
    return null;
  }
  const base = Math.floor(fallbackNowMs / windowMs) * windowMs;
  const step = windowMs / (count + 1);
  const resolved = base + step * (rank + 1);
  return Number.isFinite(resolved) && resolved > 0 ? resolved : null;
};

const resolveSendDisplayOrder = (localNowMs: number, fallbackNowMs: number): SendDisplayOrderResolution => {
  const messageSortBasis = resolveConfiguredMessageSortBasis();
  const startedAt = Number(draftStartedAtMs.value);
  const manualOrder = resolveManualPreviewDisplayOrder(fallbackNowMs);
  if (manualOrder !== null) {
    return {
      localDisplayOrder: manualOrder,
      explicitDisplayOrder: manualOrder,
    };
  }
  if (messageSortBasis === 'send_time') {
    return {
      localDisplayOrder: fallbackNowMs > 0 ? fallbackNowMs : localNowMs,
    };
  }
  const timeBasedOrder = Number.isFinite(startedAt) && startedAt > 0
    ? startedAt
    : localNowMs;
  let typingDurationMs: number | undefined;
  if (Number.isFinite(startedAt) && startedAt > 0) {
    const duration = Math.floor(localNowMs - startedAt);
    if (Number.isFinite(duration) && duration > 0) {
      typingDurationMs = Math.min(duration, 24 * 60 * 60 * 1000);
    }
  }
  return {
    localDisplayOrder: timeBasedOrder,
    typingDurationMs,
  };
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
  const whisperSnapshot = captureWhisperSnapshot(chat.whisperTargets);
  const signature = buildInputHistorySignature(mode, content, whisperSnapshot);
  if (!options.force && signature === lastHistorySignature.value) {
    return false;
  }
  const channelKey = currentChannelKey.value;
  const store = readHistoryStore();
  const existing = normalizeHistoryEntries(store[channelKey] || []);
  const filtered = existing.filter((entry) => buildInputHistorySignature(
    entry.mode,
    entry.content,
    entry.whisperSnapshot,
  ) !== signature);
  
  // 提取当前图片信息
  const images = mode === 'plain' ? collectCurrentImageInfo() : undefined;
  
  const newEntry: InputHistoryEntry = {
    id: nanoid(),
    channelKey,
    mode,
    content,
    createdAt: Date.now(),
    images: images?.length ? images : undefined,
    whisperSnapshot,
  };
  filtered.unshift(newEntry);
  pruneAndPersist(channelKey, filtered);
  lastHistorySignature.value = signature;
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
    draftOwnerChannelKey.value = entry.channelKey || currentChannelKey.value;
    restoreWhisperSnapshot(chat, entry.whisperSnapshot);
    clearInputModeCache();
    inputMode.value = entry.mode;
    suspendInlineSync = true;
    textToSend.value = entry.content;
    suspendInlineSync = false;
    
    // 恢复图片信息
    restoreImagesFromHistory(entry);
    syncInlineMarkersWithText(entry.content);

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

const hasMeaningfulDraftInInput = () => isContentMeaningful(inputMode.value, textToSend.value);
let lastAutoRestoreNoticeAt = 0;
let lastAutoRestoreNoticeChannelKey = '';

const notifyAutoRestoreSuccess = (channelKey: string) => {
  const now = Date.now();
  if (lastAutoRestoreNoticeChannelKey === channelKey && now - lastAutoRestoreNoticeAt < 1500) {
    return;
  }
  lastAutoRestoreNoticeChannelKey = channelKey;
  lastAutoRestoreNoticeAt = now;
  message.info('已自动恢复上次输入');
};

const persistSessionDraftForChannel = (
  channelKey: string,
  options: { clearWhenEmpty?: boolean } = {},
) => {
  if (!channelKey || channelKey === HISTORY_CHANNEL_FALLBACK || isEditing.value) {
    return;
  }
  if (!isContentMeaningful(inputMode.value, textToSend.value)) {
    if (options.clearWhenEmpty) {
      writeSessionDraftForChannel(channelKey, null);
    }
    return;
  }
  const images = inputMode.value === 'plain' ? collectCurrentImageInfo() : undefined;
  writeSessionDraftForChannel(channelKey, {
    mode: inputMode.value,
    content: textToSend.value,
    updatedAt: Date.now(),
    images: images?.length ? images : undefined,
    whisperSnapshot: captureWhisperSnapshot(chat.whisperTargets),
  });
};

const resolveDraftOwnerChannelKey = () => {
  const owner = String(draftOwnerChannelKey.value || '').trim();
  if (owner && owner !== HISTORY_CHANNEL_FALLBACK) {
    return owner;
  }
  return currentChannelKey.value;
};

const persistOwnedSessionDraft = (options: { clearWhenEmpty?: boolean } = {}) => {
  persistSessionDraftForChannel(resolveDraftOwnerChannelKey(), options);
};

const syncSessionDraftSnapshot = () => {
  persistOwnedSessionDraft({ clearWhenEmpty: true });
};

const scheduleSessionDraftSnapshot = throttle(
  () => {
    syncSessionDraftSnapshot();
  },
  600,
  { leading: false, trailing: true },
);

const tryAutoRestoreSessionDraft = () => {
  const channelKey = currentChannelKey.value;
  if (!channelKey || channelKey === HISTORY_CHANNEL_FALLBACK) {
    return;
  }
  if (isEditing.value || hasMeaningfulDraftInInput()) {
    return;
  }
  const draft = readSessionDraftForChannel(channelKey);
  if (!draft || !isContentMeaningful(draft.mode, draft.content)) {
    draftOwnerChannelKey.value = channelKey;
    writeSessionDraftForChannel(channelKey, null);
    return;
  }
  const entry: InputHistoryEntry = {
    id: `session:${channelKey}`,
    channelKey,
    mode: draft.mode,
    content: draft.content,
    createdAt: draft.updatedAt,
    images: draft.images,
    whisperSnapshot: draft.whisperSnapshot,
  };
  applyHistoryEntry(entry, { silent: true });
  notifyAutoRestoreSuccess(channelKey);
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
});

const handleHistoryPopoverShow = (show: boolean) => {
  historyPopoverVisible.value = show;
  if (show) {
    refreshHistoryEntries();
  }
};

const closeInputExtraOverlays = () => {
  emojiPopoverShow.value = false;
  historyPopoverVisible.value = false;
  diceSettingsVisible.value = false;
};

watch(
  () => isMinimalInputActive.value,
  (active) => {
    if (!active) {
      minimalInputToolbarVisible.value = false;
      closeInputExtraOverlays();
    }
  },
);

watch(minimalInputToolbarVisible, (visible) => {
  if (!visible) {
    closeInputExtraOverlays();
  }
});

const toggleMinimalInputToolbar = () => {
  const next = !minimalInputToolbarVisible.value;
  minimalInputToolbarVisible.value = next;
  if (!next) {
    closeInputExtraOverlays();
  }
};

watch(hasHistoryEntries, (has) => {
  if (!has) {
    historyPopoverVisible.value = false;
  }
});

onMounted(() => {
  clearLegacyHistoryAutoRestoreStore();
  refreshHistoryEntries();
  nextTick(() => {
    tryAutoRestoreSessionDraft();
  });
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
        avatarDecorations: cloneAvatarDecorations(item.avatarDecorations),
        content: contentValue,
        indicatorOnly,
        isSelf: item.userId === user.info.id,
        isTemporary: item.isTemporary,
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
    let previewColor = '';
    let previewIsTemporary = false;
    let previewAvatarDecorations: AvatarDecoration[] = [];
    const identityPreview = resolveIdentityPreviewInfo(
      chat.editing.channelId,
      chat.editing.identityId,
      chat.editing.identityVariantId,
      chat.editing.identitySnapshot as MessageIdentitySnapshot | null | undefined,
    );
    if (identityPreview) {
      if (identityPreview.displayName) {
        previewDisplayName = identityPreview.displayName;
      }
      previewColor = identityPreview.color || '';
      previewAvatarDecorations = cloneAvatarDecorations(identityPreview.avatarDecorations);
      previewIsTemporary = Boolean(identityPreview.isTemporary);
      if (identityPreview.avatar || previewIsTemporary) {
        previewAvatar = identityPreview.avatar || '';
      }
    }
    map[chat.editing.messageId] = {
      userId: user.info.id,
      displayName: previewDisplayName,
      color: previewColor,
      avatar: previewAvatar,
      avatarDecorations: previewAvatarDecorations,
      content: draft,
      indicatorOnly,
      isSelf: true,
      isTemporary: previewIsTemporary,
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
const whisperCandidateUsers = ref<Array<{
  userId: string;
  userDisplayName: string;
  userColor?: string;
  avatar: string;
  icIdentityId?: string;
  icDisplayName?: string;
  icColor?: string;
  icAvatar?: string;
  oocIdentityId?: string;
  oocDisplayName?: string;
  oocColor?: string;
  oocAvatar?: string;
}>>([]);

type WhisperIdentityType = 'ic' | 'ooc' | 'user';

interface WhisperCandidate {
  raw: any;
  id: string;
  avatar: string;
  displayName: string;
  secondaryName: string;
  color: string;
  identityTypes: WhisperIdentityType[];
  userDisplayName: string;
  icDisplayName: string;
  oocDisplayName: string;
  userColor: string;
  icColor: string;
  oocColor: string;
}

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

const resolveWhisperCandidatePreferredName = (
  item: { userDisplayName?: string; icDisplayName?: string; oocDisplayName?: string },
  mode: 'ic' | 'ooc',
) => {
  if (mode === 'ooc') {
    return item.oocDisplayName || item.icDisplayName || item.userDisplayName || '未知成员';
  }
  return item.icDisplayName || item.oocDisplayName || item.userDisplayName || '未知成员';
};

const buildWhisperCandidateSummary = (item: { userDisplayName?: string; icDisplayName?: string; oocDisplayName?: string }) => {
  const ic = item.icDisplayName || '未配置';
  const ooc = item.oocDisplayName || '未配置';
  const userName = item.userDisplayName || '未知成员';
  return `场内：${ic} | 场外：${ooc} | 用户：${userName}`;
};

const resolveWhisperMetaNameStyle = (color?: string) => {
  const normalized = normalizeHexColor(color || '');
  return normalized ? { color: normalized } : undefined;
};

const buildWhisperCandidates = (items: Array<{
  userId?: string;
  userDisplayName?: string;
  userColor?: string;
  avatar?: string;
  icIdentityId?: string;
  icDisplayName?: string;
  icColor?: string;
  icAvatar?: string;
  oocIdentityId?: string;
  oocDisplayName?: string;
  oocColor?: string;
  oocAvatar?: string;
}>, mode: 'ic' | 'ooc') => {
  const candidates: WhisperCandidate[] = [];
  for (const item of items) {
    const userId = String(item?.userId || '').trim();
    if (!userId || userId === user.info.id) {
      continue;
    }
    const icDisplayName = String(item?.icDisplayName || '').trim();
    const oocDisplayName = String(item?.oocDisplayName || '').trim();
    const userDisplayName = String(item?.userDisplayName || '').trim() || '未知成员';
    const userColor = normalizeHexColor(item?.userColor || '') || '';
    const icColor = normalizeHexColor(item?.icColor || '') || '';
    const oocColor = normalizeHexColor(item?.oocColor || '') || '';
    const identityTypes: WhisperIdentityType[] = [];
    if (icDisplayName) identityTypes.push('ic');
    if (oocDisplayName) identityTypes.push('ooc');
    if (!identityTypes.length) identityTypes.push('user');

    const displayName = resolveWhisperCandidatePreferredName({ userDisplayName, icDisplayName, oocDisplayName }, mode);
    const avatar = mode === 'ooc'
      ? (item?.oocAvatar || item?.icAvatar || item?.avatar || '')
      : (item?.icAvatar || item?.oocAvatar || item?.avatar || '');
    const color = normalizeHexColor(
      mode === 'ooc'
        ? (item?.oocColor || item?.icColor || item?.userColor || '')
        : (item?.icColor || item?.oocColor || item?.userColor || ''),
    ) || '';

    candidates.push({
      raw: item,
      id: userId,
      avatar,
      displayName,
      secondaryName: buildWhisperCandidateSummary({ userDisplayName, icDisplayName, oocDisplayName }),
      color,
      identityTypes,
      userDisplayName,
      icDisplayName,
      oocDisplayName,
      userColor,
      icColor,
      oocColor,
    });
  }

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

const resolveWhisperTargetColor = (target: {
  id?: string;
  color?: string;
  nick_color?: string;
  nickColor?: string;
  whisperIcColor?: string;
  whisperOocColor?: string;
} | null | undefined) => {
  const fallback = chat.icMode === 'ooc'
    ? (target?.whisperOocColor || target?.whisperIcColor || target?.color || target?.nick_color || target?.nickColor || '')
    : (target?.whisperIcColor || target?.whisperOocColor || target?.color || target?.nick_color || target?.nickColor || '');
  return normalizeHexColor(fallback) || '';
};

const getWhisperTargetStyle = (target: { id?: string; color?: string; nick_color?: string; nickColor?: string } | null | undefined) => {
  const color = resolveWhisperTargetColor(target);
  return color ? { color } : undefined;
};

const whisperCandidates = computed<WhisperCandidate[]>(() => buildWhisperCandidates(whisperCandidateUsers.value, chat.icMode as 'ic' | 'ooc'));

const filteredWhisperCandidates = computed(() => {
  const keyword = whisperQuery.value.trim();
  if (!keyword) {
    return whisperCandidates.value;
  }
  return whisperCandidates.value.filter((candidate) => {
    const candidates = [
      candidate.displayName,
      candidate.secondaryName,
      candidate.icDisplayName,
      candidate.oocDisplayName,
      candidate.userDisplayName,
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
    const name = resolveSelectedWhisperTargetName(target);
    return t('inputBox.whisperPlaceholder', { target: `@${name}` });
  }
  return t('inputBox.whisperPlaceholderMultiple', { count: whisperTargets.value.length });
});

const resolveSelectedWhisperTargetName = (target: any) => {
  if (!target) {
    return '未知成员';
  }
  if (chat.icMode === 'ooc') {
    return target?.whisperOocDisplayName || target?.whisperIcDisplayName || target?.nick || target?.name || target?.whisperUserDisplayName || '未知成员';
  }
  return target?.whisperIcDisplayName || target?.whisperOocDisplayName || target?.nick || target?.name || target?.whisperUserDisplayName || '未知成员';
};

const ensureInputFocus = () => {
  nextTick(() => {
    if (textInputRef.value?.focus) {
      textInputRef.value.focus();
      return;
    }
    textInputRef.value?.getTextarea?.()?.focus();
  });
};

const MOBILE_SEND_LONG_PRESS_MS = 500;
let mobileSendLongPressTimer: ReturnType<typeof setTimeout> | null = null;
let mobileSendLongPressTriggered = false;

const clearMobileSendLongPressTimer = () => {
  if (mobileSendLongPressTimer !== null) {
    clearTimeout(mobileSendLongPressTimer);
    mobileSendLongPressTimer = null;
  }
};

const insertMobileSendLineBreak = () => {
  if (!isMobileInteractionMode.value) {
    return;
  }
  if (textInputRef.value?.insertLineBreak) {
    textInputRef.value.insertLineBreak();
    ensureInputFocus();
    return;
  }
  insertComposerText('\n');
};

const handleSendPointerDown = (event: PointerEvent) => {
  if (!isMobileInteractionMode.value) {
    return;
  }
  event.preventDefault();
  clearMobileSendLongPressTimer();
  mobileSendLongPressTriggered = false;
  mobileSendLongPressTimer = setTimeout(() => {
    mobileSendLongPressTimer = null;
    mobileSendLongPressTriggered = true;
    insertMobileSendLineBreak();
  }, MOBILE_SEND_LONG_PRESS_MS);
};

const handleSendPointerUp = () => {
  if (isMobileInteractionMode.value) {
    clearMobileSendLongPressTimer();
  }
};

const handleSendClick = () => {
  if (isMobileInteractionMode.value && mobileSendLongPressTriggered) {
    mobileSendLongPressTriggered = false;
    return;
  }
  send();
};

const handleSendMouseDown = (event: MouseEvent) => {
  if (isMobileInteractionMode.value) {
    event.preventDefault();
  }
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

const insertComposerText = (content: string) => {
  if (!content) return;
  if (inputMode.value === 'rich') {
    const editor = textInputRef.value?.getEditor?.();
    if (editor) {
      const { from, to } = editor.state.selection;
      editor.view.dispatch(editor.state.tr.insertText(content, from, to).scrollIntoView());
      editor.chain().focus().run();
      return;
    }
  }
  const selection = getInputSelection();
  const draft = textToSend.value;
  textToSend.value = draft.slice(0, selection.start) + content + draft.slice(selection.end);
  const cursor = selection.start + content.length;
  nextTick(() => {
    setInputSelection(cursor, cursor);
    ensureInputFocus();
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

const scheduleInterjectCursorRestoreToEnd = (mode: 'plain' | 'rich') => {
  nextTick(() => {
    nextTick(() => {
      if (mode === 'plain') {
        moveInputCursorToEnd();
        return;
      }
      const editor = textInputRef.value?.getEditor?.();
      editor?.chain().focus('end').run();
    });
  });
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

const buildWhisperTargetUserFromMessage = (id: string, entry?: any, displayName?: string): User | null => {
  const normalizedId = String(id || entry?.id || '').trim();
  if (!normalizedId) {
    return null;
  }
  const channelUser = chat.curChannelUsers.find((member: any) => member?.id === normalizedId);
  const nick = String(
    entry?.nick
    || channelUser?.nick
    || displayName
    || entry?.name
    || channelUser?.name
    || normalizedId,
  ).trim();
  return {
    id: normalizedId,
    name: String(entry?.name || channelUser?.name || nick || normalizedId).trim(),
    nick: nick || normalizedId,
    avatar: String(entry?.avatar || channelUser?.avatar || '').trim(),
    discriminator: String(entry?.discriminator || channelUser?.discriminator || '').trim(),
    is_bot: Boolean(entry?.is_bot || channelUser?.is_bot),
  };
};

const resolveMessageWhisperTargets = (msg?: any): User[] => {
  if (!msg) {
    return [];
  }
  const metaIds = Array.isArray(msg?.whisperMeta?.targetUserIds) ? msg.whisperMeta.targetUserIds : [];
  const metaNames = Array.isArray(msg?.whisperMeta?.targetDisplayNames) ? msg.whisperMeta.targetDisplayNames : [];
  const list = msg?.whisperToIds || msg?.whisper_to_ids || msg?.whisperTargets || msg?.whisper_targets;
  if (Array.isArray(list) && list.length > 0) {
    return list
      .map((entry: any, index: number) => {
        if (typeof entry === 'string') {
          return buildWhisperTargetUserFromMessage(entry, null, metaNames[index]);
        }
        return buildWhisperTargetUserFromMessage(entry?.id, entry, metaNames[index]);
      })
      .filter((target: User | null): target is User => Boolean(target));
  }
  if (metaIds.length > 0) {
    return metaIds
      .map((id: string, index: number) => buildWhisperTargetUserFromMessage(id, null, metaNames[index]))
      .filter((target: User | null): target is User => Boolean(target));
  }
  const singleId = resolveMessageWhisperTargetId(msg);
  if (!singleId) {
    return [];
  }
  const single = buildWhisperTargetUserFromMessage(singleId, msg?.whisperTo || msg?.whisper_to || msg?.whisper_target, msg?.whisperMeta?.targetMemberName);
  return single ? [single] : [];
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

type MessageIdentitySnapshot = {
  identityId: string | null;
  displayName: string;
  color: string;
  avatarAttachmentId: string;
  avatarDecorations?: AvatarDecoration[] | null;
  isTemporary: boolean;
};

const resolveMessageIdentitySnapshot = (msg?: any): MessageIdentitySnapshot | null => {
  if (!msg) {
    return null;
  }
  const directIdentity = msg.identity || msg.identity_info || msg.identityData;
  const identityId = resolveMessageIdentityId(msg);
  const displayName = String(
    directIdentity?.displayName
    || msg?.sender_identity_name
    || (identityId ? msg?.sender_member_name : '')
    || '',
  ).trim();
  const color = normalizeHexColor(
    String(
      directIdentity?.color
      || msg?.sender_identity_color
      || '',
    ),
  ) || '';
  const avatarAttachmentId = String(
    directIdentity?.avatarAttachment
    || msg?.sender_identity_avatar_id
    || msg?.sender_identity_avatar
    || msg?.senderIdentityAvatarID
    || msg?.senderIdentityAvatarId
    || '',
  ).trim();
  const avatarDecorations = cloneAvatarDecorations(
    directIdentity?.avatarDecorations || msg?.sender_identity_decoration,
    directIdentity?.avatarDecoration || null,
  );
  const isTemporary = Boolean(
    directIdentity?.isTemporary
    ?? msg?.sender_identity_is_temporary
    ?? msg?.senderIdentityIsTemporary,
  );
  if (!identityId && !displayName && !color && !avatarAttachmentId && avatarDecorations.length === 0 && !isTemporary) {
    return null;
  }
  return {
    identityId,
    displayName,
    color,
    avatarAttachmentId,
    avatarDecorations,
    isTemporary,
  };
};

const createInterjectIdentitySnapshot = (
  identity?: ChannelIdentity | null,
  appearance?: IdentityAppearancePreview | null,
): MessageIdentitySnapshot | null => {
  const identityId = String(identity?.id || appearance?.identityId || '').trim() || null;
  const displayName = String(appearance?.displayName || identity?.displayName || '').trim();
  const color = normalizeHexColor(appearance?.color || identity?.color || '') || '';
  const avatarAttachmentId = String(appearance?.avatarAttachmentId || identity?.avatarAttachmentId || '').trim();
  const avatarDecorations = cloneAvatarDecorations(appearance?.avatarDecorations);
  const isTemporary = Boolean(appearance?.isTemporary ?? identity?.isTemporary);
  if (!identityId && !displayName && !color && !avatarAttachmentId && avatarDecorations.length === 0 && !isTemporary) {
    return null;
  }
  return {
    identityId,
    displayName,
    color,
    avatarAttachmentId,
    avatarDecorations,
    isTemporary,
  };
};

const createInterjectEditSnapshot = (source: {
  messageId?: string | null;
  channelId?: string | null;
  originalContent?: string;
  draft?: string;
  mode?: 'plain' | 'rich';
  isWhisper?: boolean;
  whisperTargetId?: string | null;
  whisperTargets?: User[];
  icMode?: 'ic' | 'ooc';
  identityId?: string | null;
  identityVariantId?: string | null;
  identitySnapshot?: MessageIdentitySnapshot | null;
}): InterjectEditSnapshot | null => {
  const messageId = String(source.messageId || '').trim();
  const channelId = String(source.channelId || '').trim();
  if (!messageId || !channelId) {
    return null;
  }
  const whisperTargets = (source.whisperTargets || [])
    .filter((target) => target?.id)
    .map((target) => ({ ...target }));
  const whisperTargetId = String(source.whisperTargetId || whisperTargets[0]?.id || '').trim() || null;
  const identityId = String(source.identityId || '').trim() || null;
  const identityVariantId = String(source.identityVariantId || '').trim() || null;
  return {
    messageId,
    channelId,
    originalContent: source.originalContent || '',
    draft: source.draft || source.originalContent || '',
    mode: source.mode === 'rich' ? 'rich' : 'plain',
    isWhisper: Boolean(source.isWhisper) || whisperTargets.length > 0,
    whisperTargetId,
    whisperTargets,
    icMode: source.icMode === 'ooc' ? 'ooc' : 'ic',
    identityId,
    identityVariantId,
    identitySnapshot: source.identitySnapshot ? {
      ...source.identitySnapshot,
      avatarDecorations: cloneAvatarDecorations(source.identitySnapshot.avatarDecorations),
    } : null,
  };
};

const createInterjectComposerSnapshot = (): InterjectComposerSnapshot | null => {
  const channelId = String(chat.curChannel?.id || '').trim();
  if (!channelId) {
    return null;
  }
  const identityId = String(chat.getActiveIdentityId(channelId) || '').trim() || null;
  const identityVariantId = identityId
    ? (String(chat.getActiveIdentityVariantId(channelId, identityId) || '').trim() || null)
    : null;
  return {
    channelId,
    icMode: inputIcMode.value === 'ooc' ? 'ooc' : 'ic',
    identityId,
    identityVariantId,
  };
};

const restoreInterjectComposerSnapshot = (snapshot: InterjectComposerSnapshot) => {
  const channelId = String(snapshot.channelId || '').trim();
  if (!channelId) {
    return;
  }
  const identityId = String(snapshot.identityId || '').trim();
  if (identityId) {
    chat.setActiveIdentity(channelId, identityId, undefined, { syncIcOocFromRole: false });
    chat.setActiveIdentityVariant(channelId, identityId, snapshot.identityVariantId || '');
  } else {
    chat.setActiveIdentity(channelId, '', undefined, { syncIcOocFromRole: false });
  }
  chat.setIcMode(snapshot.icMode === 'ooc' ? 'ooc' : 'ic', channelId);
};

const restoreInterjectEditingSnapshot = (snapshot: InterjectEditSnapshot) => {
  const messageId = String(snapshot.messageId || '').trim();
  const channelId = String(snapshot.channelId || '').trim();
  if (!messageId || !channelId) {
    message.warning('插话首条消息缺失，无法进入编辑态');
    return;
  }
  reeditRevokedSource.value = null;
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  chat.curReplyTo = null;
  chat.clearWhisperTargets();
  invalidateEditSession();
  const identitySnapshot = snapshot.identitySnapshot
    ? {
      ...snapshot.identitySnapshot,
      avatarDecorations: cloneAvatarDecorations(snapshot.identitySnapshot.avatarDecorations),
    }
    : null;
  chat.startEditingMessage({
    messageId,
    channelId,
    originalContent: snapshot.originalContent || '',
    draft: snapshot.draft || '',
    mode: snapshot.mode,
    isWhisper: Boolean(snapshot.isWhisper),
    whisperTargetId: snapshot.whisperTargetId || null,
    whisperTargets: (snapshot.whisperTargets || []).map((target) => ({ ...target })),
    icMode: snapshot.icMode === 'ooc' ? 'ooc' : 'ic',
    identityId: snapshot.identityId || null,
    identityVariantId: snapshot.identityVariantId || null,
    identitySnapshot,
  });
  inputMode.value = snapshot.mode;
  scheduleInterjectCursorRestoreToEnd(snapshot.mode);
};

const findIdentityMeta = (channelId?: string, identityId?: string | null) => {
  if (!channelId || !identityId) {
    return null;
  }
  const list = chat.channelIdentities[channelId] || [];
  return list.find((item) => item.id === identityId) || null;
};

const resolveIdentityPreviewInfo = (
  channelId?: string,
  identityId?: string | null,
  identityVariantId?: string | null,
  snapshot?: MessageIdentitySnapshot | null,
) => {
  if (!identityId) {
    return null;
  }
  const identity = findIdentityMeta(channelId, identityId);
  if (identity) {
    const variant = identityVariantId ? chat.getIdentityVariants(channelId, identityId).find(item => item.id === identityVariantId) || null : null;
    const appearance = resolveIdentityAppearancePreview(identity, variant);
    return {
      displayName: appearance?.displayName || '',
      avatar: appearance?.avatarAttachmentId ? resolveAttachmentUrl(appearance.avatarAttachmentId) : '',
      color: appearance?.color || '',
      avatarDecorations: cloneAvatarDecorations(appearance?.avatarDecorations),
      isTemporary: Boolean(appearance?.isTemporary),
    };
  }
  if (!snapshot || snapshot.identityId !== identityId) {
    return null;
  }
  return {
    displayName: snapshot.displayName || '',
    avatar: snapshot.avatarAttachmentId ? resolveAttachmentUrl(snapshot.avatarAttachmentId) : '',
    color: snapshot.color || '',
    avatarDecorations: cloneAvatarDecorations(snapshot.avatarDecorations),
    isTemporary: Boolean(snapshot.isTemporary),
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
  if (!chat.canModerateTargetByRole(chat.curChannel.id, user.info.id, targetUserId)) {
    return false;
  }
  return true;
};

const cacheRevokedDraftFromMessage = (target?: Message | null, overrideChannelId?: string) => {
  if (!target?.id) {
    return;
  }
  const ownerId = resolveMessageUserId(target);
  if (!ownerId || ownerId !== user.info.id) {
    return;
  }
  const channelId = String(
    overrideChannelId
    || (target as any)?.channel?.id
    || (target as any)?.channel_id
    || chat.curChannel?.id
    || '',
  ).trim();
  if (!channelId) {
    return;
  }
  const rawContent = typeof target.content === 'string'
    ? target.content
    : (typeof (target as any)?.originalContent === 'string' ? (target as any).originalContent : '');
  if (!rawContent) {
    return;
  }
  const mode = detectMessageContentMode(rawContent);
  const whisperTargetId = resolveMessageWhisperTargetId(target);
  const identityId = resolveMessageIdentityId(target);
  const identityVariantId = resolveIdentityVariantIdFromMessage(target);
  const icMode = String(target.icMode ?? (target as any)?.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic';
  chat.cacheRevokedDraft({
    messageId: target.id,
    channelId,
    content: rawContent,
    mode,
    isWhisper: Boolean(target.isWhisper ?? (target as any)?.is_whisper),
    whisperTargetId,
    icMode,
    identityId: identityId || null,
    identityVariantId,
  });
};

interface EditSaveSnapshot {
  token: number;
  channelId: string;
  messageId: string;
  isWhisper: boolean;
  whisperTargetIds: string[];
  icMode: 'ic' | 'ooc';
  identityId: string | null;
  identityVariantId: string | null;
  initialIdentityId: string | null;
  initialIdentityVariantId: string | null;
}

const isSavingEdit = ref(false);
const editSessionToken = ref(0);
const invalidateEditSession = () => {
  editSessionToken.value += 1;
  isSavingEdit.value = false;
};
const cancelEditingSession = () => {
  if (chat.editing) {
    chat.cancelEditing();
  }
  invalidateEditSession();
};
const createEditSaveSnapshot = (): EditSaveSnapshot | null => {
  const editing = chat.editing;
  if (!editing) {
    return null;
  }
  return {
    token: editSessionToken.value,
    channelId: editing.channelId,
    messageId: editing.messageId,
    isWhisper: Boolean(editing.isWhisper),
    whisperTargetIds: chat.whisperTargets.map((target) => String(target?.id || '').trim()).filter(Boolean),
    icMode: editing.icMode === 'ooc' ? 'ooc' : 'ic',
    identityId: editing.identityId ?? null,
    identityVariantId: editing.identityVariantId ?? null,
    initialIdentityId: editing.initialIdentityId ?? null,
    initialIdentityVariantId: editing.initialIdentityVariantId ?? null,
  };
};
const isEditSaveSnapshotAlive = (snapshot: EditSaveSnapshot) => {
  const editing = chat.editing;
  return Boolean(
    editing
    && editSessionToken.value === snapshot.token
    && editing.channelId === snapshot.channelId
    && editing.messageId === snapshot.messageId,
  );
};

const beginEdit = (target?: Message) => {
  if (!target?.id || !chat.curChannel?.id) {
    return;
  }
  clearInterjectSession();
  reeditRevokedSource.value = null;
  if (!canEditMessage(target)) {
    message.error('无权编辑该消息');
    return;
  }
  stopTypingPreviewNow();
  stopEditingPreviewNow();
  chat.curReplyTo = null;
  chat.clearWhisperTargets();
  invalidateEditSession();
  const detectedMode = detectMessageContentMode(target.content);
  const whisperTargetId = resolveMessageWhisperTargetId(target);
  const whisperTargets = resolveMessageWhisperTargets(target);
  const identityId = resolveMessageIdentityId(target);
  const identityVariantId = resolveIdentityVariantIdFromMessage(target);
  const identitySnapshot = resolveMessageIdentitySnapshot(target);
  const icMode = String(target.icMode ?? target.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic';
  chat.startEditingMessage({
    messageId: target.id,
    channelId: chat.curChannel.id,
    originalContent: target.content || '',
    draft: target.content || '',
    mode: detectedMode,
    isWhisper: Boolean(target.isWhisper),
    whisperTargetId,
    whisperTargets,
    icMode,
    identityId: identityId || null,
    identityVariantId,
    identitySnapshot,
  });
  inputMode.value = detectedMode;
};

const handleReeditRevokedMessage = async (target?: Message) => {
  const messageId = String(target?.id || '').trim();
  const channelId = String(chat.curChannel?.id || '').trim();
  if (!messageId || !channelId) {
    return;
  }
  let cachedDraft = chat.getRevokedDraft(channelId, messageId);
  if (!cachedDraft) {
    try {
      cachedDraft = await chat.fetchRevokedDraft(channelId, messageId);
    } catch (error) {
      console.warn('拉取撤回草稿失败', error);
    }
  }
  if (!cachedDraft) {
    message.warning('撤回内容不可恢复');
    return;
  }
  if (chat.editing) {
    stopEditingPreviewNow();
    cancelEditingSession();
  }
  stopTypingPreviewNow();
  clearInputModeCache();
  chat.curReplyTo = null;
  chat.clearWhisperTargets();

  inputMode.value = detectMessageContentMode(cachedDraft.content);
  let draft = '';
  if (inputMode.value === 'rich') {
    resetInlineImages();
    draft = cachedDraft.content;
  } else {
    draft = convertMessageContentToDraft(cachedDraft.content);
  }
  textToSend.value = draft;
  chat.messageMenu.show = false;
  reeditRevokedSource.value = { channelId, messageId };
  syncSessionDraftSnapshot();
  ensureInputFocus();
  nextTick(() => {
    if (inputMode.value === 'plain') {
      moveInputCursorToEnd();
      return;
    }
    const editor = textInputRef.value?.getEditor?.();
    editor?.chain().focus('end').run();
  });
};

const cancelEditing = () => {
  if (!chat.editing && !isSavingEdit.value) {
    return;
  }
  stopEditingPreviewNow();
  cancelEditingSession();
  textToSend.value = '';
  syncSessionDraftSnapshot();
  stopTypingPreviewNow();
  resetInlineImages();
  ensureInputFocus();
};

const saveEdit = async () => {
  if (isSavingEdit.value) {
    return;
  }
  const snapshot = createEditSaveSnapshot();
  if (!snapshot) {
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
  isSavingEdit.value = true;
  try {
    if (!isEditSaveSnapshotAlive(snapshot)) {
      return;
    }
    stopTypingPreviewNow();
    let finalContent: string;
    if (inputMode.value === 'rich') {
      finalContent = processedDraft;
    } else {
      finalContent = await normalizePlainMessageContent(processedDraft);
    }
    if (!isEditSaveSnapshotAlive(snapshot)) {
      return;
    }
    if (finalContent.trim() === '') {
      message.error('消息内容不能为空');
      return;
    }
    if (snapshot.isWhisper && snapshot.whisperTargetIds.length === 0) {
      message.error('悄悄话至少需要一个可见对象');
      return;
    }
    const updateOptions = buildEditMessageUpdateOptions(snapshot);
    const updated = await chat.messageUpdate(
      snapshot.channelId,
      snapshot.messageId,
      finalContent,
      updateOptions,
    );
    if (!isEditSaveSnapshotAlive(snapshot)) {
      return;
    }
    if (updated) {
      upsertMessage(updated as unknown as Message);
    }
    message.success('消息已更新');
    stopEditingPreviewNow();
    cancelEditingSession();
    textToSend.value = '';
    syncSessionDraftSnapshot();
    resetInlineImages();
    ensureInputFocus();
  } catch (error: any) {
    if (!isEditSaveSnapshotAlive(snapshot)) {
      return;
    }
    console.error('更新消息失败', error);
    const fallbackMessage = '编辑失败，请稍后重试';
    const errorMessage = String(error?.message || '');
    message.error(errorMessage.includes('Cannot read properties of null') ? fallbackMessage : (errorMessage || fallbackMessage));
  } finally {
    isSavingEdit.value = false;
  }
};

function openWhisperPanel(source: 'slash' | 'manual') {
  whisperPickerSource.value = source;
  whisperPanelVisible.value = true;
  whisperSelectionIndex.value = 0;
  void loadWhisperCandidates();
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

const loadWhisperCandidates = async () => {
  const channelId = chat.curChannel?.id || '';
  if (!channelId || channelId.length >= 30) {
    whisperCandidateUsers.value = [];
    return;
  }
  try {
    const resp = await chat.fetchWhisperCandidates(channelId);
    whisperCandidateUsers.value = resp?.items || [];
  } catch (error) {
    console.warn('获取悄悄话候选成员失败', error);
    whisperCandidateUsers.value = [];
  }
};

const onWhisperTargetToggle = (candidate: WhisperCandidate) => {
  if (!candidate?.id) {
    return;
  }
  const raw = candidate.raw || {};
  const targetUser: User = {
    id: candidate.id,
    name: raw.userDisplayName || raw.name || raw.username || raw.nick || candidate.displayName,
    nick: candidate.displayName,
    avatar: candidate.avatar,
    discriminator: raw.discriminator || '',
    is_bot: !!raw.is_bot,
  };
  (targetUser as any).color = candidate.color || '';
  (targetUser as any).whisperIcDisplayName = candidate.icDisplayName || '';
  (targetUser as any).whisperOocDisplayName = candidate.oocDisplayName || '';
  (targetUser as any).whisperUserDisplayName = candidate.userDisplayName || '';
  (targetUser as any).whisperUserColor = candidate.userColor || '';
  (targetUser as any).whisperIcColor = candidate.icColor || '';
  (targetUser as any).whisperOocColor = candidate.oocColor || '';
  chat.toggleWhisperTarget(targetUser);
};

const handleWhisperCandidateChecked = (candidate: WhisperCandidate, checked: boolean) => {
  if (checked === isWhisperTarget(candidate)) {
    return;
  }
  onWhisperTargetToggle(candidate);
};

const selectAllFilteredWhisperCandidates = () => {
  filteredWhisperCandidates.value.forEach((candidate) => {
    if (!isWhisperTarget(candidate)) {
      onWhisperTargetToggle(candidate);
    }
  });
};

const invertFilteredWhisperCandidates = () => {
  filteredWhisperCandidates.value.forEach((candidate) => {
    onWhisperTargetToggle(candidate);
  });
};

const confirmWhisperSelection = () => {
  chat.confirmWhisperTargets();
  const source = whisperPickerSource.value;
  closeWhisperPanel();
  if (source === 'slash') {
    textToSend.value = '';
    syncSessionDraftSnapshot();
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
      syncSessionDraftSnapshot();
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
  if (whisperPanelVisible.value) {
    closeWhisperPanel();
    return;
  }
  openWhisperPanel('manual');
};

const clearWhisperTargets = () => {
  chat.clearWhisperTargets();
  ensureInputFocus();
};

const containsInlineImageMarker = (text: string) => /\[\[图片:[^\]]+\]\]/.test(text);

const AT_TOKEN_FLEX_REGEX = /<at\s+id=(?:\\?"|')([^"'>]+)(?:\\?"|')(?:\s+name=(?:\\?"|')([^"']*)(?:\\?"|'))?\s*\/?\s*>/g;

const decodeAtTokenText = (value: string) => {
  return contentUnescape(value);
};

const replaceAtTokensWithDisplayText = (value: string) => {
  AT_TOKEN_FLEX_REGEX.lastIndex = 0;
  return value.replace(AT_TOKEN_FLEX_REGEX, (_full, id: string, name: string) => {
    const display = decodeAtTokenText(name || id || '用户');
    return `@${display}`;
  });
};

const collectMentionIdsFromText = (value: string, output: Set<string>) => {
  AT_TOKEN_FLEX_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = AT_TOKEN_FLEX_REGEX.exec(value)) !== null) {
    const id = decodeAtTokenText(match[1] || '').trim();
    if (id) {
      output.add(id);
    }
  }
};

const collectMentionIdsFromTipTapNode = (node: any, output: Set<string>) => {
  if (!node) {
    return;
  }

  if (typeof node.text === 'string' && node.text) {
    collectMentionIdsFromText(node.text, output);
  }

  if (node.type === 'mention' || node.type === 'satoriMention') {
    const id = String(node.attrs?.id || '').trim();
    if (id) {
      output.add(id);
    }
  }

  if (Array.isArray(node.content)) {
    node.content.forEach((child: any) => collectMentionIdsFromTipTapNode(child, output));
  }
};

const collectMentionIdsFromContent = (content: string) => {
  const output = new Set<string>();
  if (!content) {
    return output;
  }

  collectMentionIdsFromText(content, output);

  if (isTipTapJson(content)) {
    try {
      const json = JSON.parse(content);
      collectMentionIdsFromTipTapNode(json, output);
    } catch {
      // ignore
    }
  }

  return output;
};

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
  let replaced = replaceAtTokensWithDisplayText(value);
  // 替换图片标记为 [图片]
  replaced = replaced.replace(/\[\[图片:[^\]]+\]\]/g, '[图片]');
  return normalizePlaceholderWhitespace(replaced);
};

// 从 TipTap JSON 提取纯文本
const extractTipTapText = (node: any): string => {
  if (!node) return '';

  if (node.text !== undefined) {
    return replaceAtTokensWithDisplayText(node.text);
  }

  if (node.type === 'mention' || node.type === 'satoriMention') {
    const mentionId = String(node.attrs?.id || '').trim();
    const mentionName = String(node.attrs?.name || '').trim();
    return `@${mentionName || mentionId || '用户'}`;
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
const buildSinglePreviewDiceChip = (formula: string, source: string, index: string | number) => {
  const tone = resolveDiceToneClass();
  return `<span class="dice-chip dice-chip--preview dice-chip--tone-${tone}" data-dice-tone="${tone}" data-index="${index}" title="${source}">${diceChipIconSvg}<span class="dice-chip__formula">${formula}</span><span class="dice-chip__equals">=</span><span class="dice-chip__result">?</span></span>`;
};

const buildPreviewDiceChip = (match: DiceMatch, index: number) => {
  const source = escapeHtml(match.source);
  const { repeatCount, formula } = parseMultiDiceExpression(match.normalized, defaultDiceExpr.value);
  const escapedFormula = escapeHtml(formula);
  if (repeatCount <= 1) {
    return buildSinglePreviewDiceChip(escapedFormula, source, index);
  }
  return `<span class="dice-roll-group" data-dice-source="${source}">${Array.from({ length: repeatCount }, (_, offset) => buildSinglePreviewDiceChip(escapedFormula, source, `${index}-${offset}`)).join('')}</span>`;
};

const renderDicePreviewSegment = (text: string) => {
  if (!text) return '';
  const disableAllFormatting = isBotCommandLikeContent(text, chat.curChannel?.botCommandPrefixes);
  const matches = matchDiceExpressions(text, defaultDiceExpr.value);
  if (!matches.length) {
    return renderQuickFormatHtmlFromEscaped(escapeHtml(text), { disableAllFormatting });
  }
  let html = '';
  let cursor = 0;
  matches.forEach((match, index) => {
    if (match.start > cursor) {
      html += renderQuickFormatHtmlFromEscaped(escapeHtml(text.slice(cursor, match.start)), { disableAllFormatting });
    }
    html += buildPreviewDiceChip(match, index);
    cursor = match.end;
  });
  if (cursor < text.length) {
    html += renderQuickFormatHtmlFromEscaped(escapeHtml(text.slice(cursor)), { disableAllFormatting });
  }
  return html;
};

const renderPreviewContent = (value: string) => {
  // 检测是否为 TipTap JSON
  if (isTipTapJson(value)) {
    try {
      if (isBotCommandLikeContent(value, chat.curChannel?.botCommandPrefixes)) {
        return DOMPurify.sanitize(renderBotCommandTextAsHtml(value));
      }
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
  let processedValue = replaceAtTokensWithDisplayText(value);

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

const normalizePlainMessageContent = async (draft: string) => {
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
  if (escaped.includes('@')) {
    escaped = await replaceUsernames(escaped);
  }
  placeholderMap.forEach((value, key) => {
    escaped = escaped.split(key).join(value);
  });
  return escaped;
};

const captureSelectionRange = (): SelectionRange => {
  const selection = getInputSelection();
  return { start: selection.start, end: selection.end };
};

const startInlineImageUpload = async (
  markerId: string,
  draft: InlineImageDraft,
  options?: { skipCompression?: boolean },
) => {
  try {
    if (!draft.file) {
      draft.status = 'failed';
      draft.error = '无效的图片文件';
      return;
    }
    const result = await uploadImageAttachment(draft.file as File, {
      channelId: chat.curChannel?.id,
      skipCompression: options?.skipCompression === true,
    });
    draft.attachmentId = result.attachmentId;
    draft.status = 'uploaded';
    draft.error = '';
    syncSessionDraftSnapshot();
  } catch (error: any) {
    draft.status = 'failed';
    draft.error = error?.message || '上传失败';
    message.error('图片上传失败，请删除占位符后重试');
    syncSessionDraftSnapshot();
  }
};

const insertInlineImages = (
  files: File[],
  selection?: SelectionRange,
  options?: { skipCompression?: boolean },
) => {
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

  imageFiles.forEach((file) => {
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
  startInlineImageUpload(markerId, draftRecord, options);
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

const handleDropGalleryItem = (payload: { attachmentId: string; selectionStart: number; selectionEnd: number }) => {
  if (!payload.attachmentId) {
    return;
  }
  insertGalleryInline(payload.attachmentId, { start: payload.selectionStart, end: payload.selectionEnd });
};

const handleRichImageInsert = async (files: File[], options?: { skipCompression?: boolean }) => {
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
      const result = await uploadImageAttachment(file, {
        channelId: chat.curChannel?.id,
        skipCompression: options?.skipCompression === true,
      });
      draftRecord.attachmentId = result.attachmentId;
      draftRecord.status = 'uploaded';
      draftRecord.error = '';

      // 更新编辑器中的图片 URL（使用可直接访问的相对路径）
      const normalizedId = normalizeAttachmentId(result.attachmentId);
      const finalUrl = normalizedId ? `/api/v1/attachment/${normalizedId}` : objectUrl;
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
    pendingInlineUploadSource = 'default';
    return;
  }

  const files = Array.from(input.files);

  if (pendingInlineUploadSource !== 'default' || inputMode.value === 'rich') {
    openRichInlineImageEditor(files, pendingInlineUploadSource, pendingInlineSelection);
  } else {
    // 纯文本模式：调用纯文本图片插入
    insertInlineImages(files, pendingInlineSelection || undefined);
  }

  pendingInlineSelection = null;
  pendingInlineUploadSource = 'default';
  input.value = '';
};

watch(() => chat.editing?.messageId, (messageId, previousId) => {
  if (messageId !== previousId) {
    invalidateEditSession();
  }
  if (messageId && interjectSession.value) {
    clearInterjectSession();
  }
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
    if (chat.editing.isWhisper) {
      (chat.editing.whisperTargets || []).forEach((target) => {
        if (target?.id) {
          chat.toggleWhisperTarget({ ...target });
        }
      });
    }
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

const retrySendMessage = async (target?: Message) => {
  if (!target?.id) {
    return;
  }
  const current = rows.value.find((msg) => msg.id === target.id) || target;
  if (!canRetrySendMessage(current)) {
    return;
  }
  const currentData = current as any;
  const content = String(current.content || '').trim();
  if (!content) {
    message.error('消息内容为空，无法重试');
    return;
  }
  const clientId = String(currentData.clientId || currentData.client_id || current.id || '').trim();
  if (!clientId) {
    message.error('缺少 client_id，无法重试');
    return;
  }
  const quoteId = current.quote?.id || currentData.quote_id || undefined;
  const whisperTargetIds = resolveWhisperTargetIdsFromMessage(currentData);
  const whisperTo = whisperTargetIds[0] || undefined;
  const identityId = String(
    currentData.senderRoleId
    || currentData.sender_role_id
    || currentData.sender_identity_id
    || current.identity?.id
    || '',
  ).trim() || undefined;
  const identityVariantId = String(
    currentData.sender_identity_variant_id
    || current.identity?.variantId
    || '',
  ).trim() || undefined;
  const displayOrder = Number(currentData.displayOrder);
  const validDisplayOrder = Number.isFinite(displayOrder) && displayOrder > 0
    ? displayOrder
    : undefined;

  setMessageSendStatus(currentData, 'sending');
  instantMessages.add(current);

  try {
    const newMsg = await chat.messageCreate(
      content,
      quoteId,
      whisperTo,
      clientId,
      identityId,
      validDisplayOrder,
      whisperTargetIds,
      undefined,
      undefined,
      identityVariantId,
    );
    if (!newMsg) {
      throw new Error('message.create returned empty result');
    }
    Object.entries(newMsg as Record<string, any>).forEach(([k, v]) => {
      (current as any)[k] = v;
    });
    setMessageSendStatus(current as any, 'sent');
    instantMessages.delete(current);
    upsertMessage(current);
    notifyNewMessageHighlight(current);
    toBottom();
  } catch (error) {
    const reason = resolveMessageSendFailureReason(error);
    setMessageSendStatus(current as any, 'failed', reason);
    message.error(`发送失败：${reason}`);
  }
};

const isChannelDefaultDiceCommandResponse = (messageData: unknown): messageData is Record<string, unknown> => (
  !!messageData
  && typeof messageData === 'object'
  && String((messageData as Record<string, unknown>).id || '').startsWith('channel-default-dice-command:')
);

const performSend = async (options?: {
  identityIdOverride?: string
  identityVariantIdOverride?: string
  mode?: 'plain' | 'rich'
}) => {
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
  const sendMode = options?.mode || inputMode.value;
  const sendIcMode: 'ic' | 'ooc' = inputIcMode.value === 'ooc' ? 'ooc' : 'ic';
  let draft = textToSend.value;
  let identityIdOverride = options?.identityIdOverride;
  let identityVariantIdOverride = options?.identityVariantIdOverride;
  const activeReeditSource = (() => {
    const source = reeditRevokedSource.value;
    if (!source) {
      return null;
    }
    if (source.channelId !== String(chat.curChannel?.id || '').trim()) {
      return null;
    }
    return { ...source };
  })();

  const identityQuickSwitchTrigger = display.settings.identityQuickSwitchTrigger || '/';
  const identityQuickSwitchChannelId = String(chat.curChannel?.id || '');

  // 仅纯文本模式支持 `触发字符 + 角色名` 或 `触发字符 + 角色名 内容` 快捷切换
  if (shouldResolveTheaterIdentityShortcut({
    identityIdOverride,
    inputMode: sendMode,
    channelId: identityQuickSwitchChannelId,
    draft,
    trigger: identityQuickSwitchTrigger,
  })) {
    const identities = chat.channelIdentities[identityQuickSwitchChannelId] || [];
    const shortcutResult = resolveIdentityShortcutMatch(draft, identities, identityQuickSwitchTrigger);
    if (shortcutResult?.ambiguous) {
      message.warning('匹配到多个同长度角色，请输入更长名称');
      return;
    }
    if (shortcutResult?.matched) {
      chat.setActiveIdentity(identityQuickSwitchChannelId, shortcutResult.matched.id);
      await characterCardStore.syncCardForIdentity(identityQuickSwitchChannelId, shortcutResult.matched.id, {
        preserveWhenUnbound: true,
        reloadAfterSwitch: false,
      });
      draft = shortcutResult.restContent;
      textToSend.value = shortcutResult.restContent;
      emitTypingPreview();
      identityIdOverride = shortcutResult.matched.id;
      if (!shortcutResult.restContent.trim()) {
        stopTypingPreviewNow();
        return;
      }
    }
  }

  const identityVariantQuickSwitchTrigger = display.settings.identityVariantQuickSwitchTrigger || '=';
  if (sendMode === 'plain' && chat.curChannel?.id) {
    const activeIdentityId = identityIdOverride || chat.getActiveIdentityId(chat.curChannel.id);
    const activeIdentity = chat.getScopedChannelIdentities(chat.curChannel.id).find(item => item.id === activeIdentityId);
    const variants = chat.getIdentityVariants(chat.curChannel.id, activeIdentityId);
    const shortcutResult = activeIdentity
      ? resolveIdentityVariantShortcutMatch(draft, activeIdentity, variants, identityVariantQuickSwitchTrigger)
      : null;
    if (shortcutResult?.ambiguous) {
      message.warning('匹配到多个同长度差分，请输入更长关键词');
      return;
    }
    if ((shortcutResult?.matched || shortcutResult?.resetToDefault) && activeIdentityId) {
      const nextVariantId = shortcutResult?.matched?.id || '';
      chat.setActiveIdentityVariant(chat.curChannel.id, activeIdentityId, nextVariantId);
      draft = shortcutResult.restContent;
      textToSend.value = shortcutResult.restContent;
      emitTypingPreview();
      identityVariantIdOverride = nextVariantId;
      if (!shortcutResult.restContent.trim()) {
        stopTypingPreviewNow();
        return;
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

  const outgoingDraft = normalizePunctuationForMessageSend(
    draft,
    display.settings.autoCorrectPunctuation,
    sendMode,
    isBotCommandLikeContent(draft, chat.curChannel?.botCommandPrefixes),
    Boolean(activeReeditSource),
  );

  let insertPlacement = resolveMessageInsertPlacement();
  if (activeMessageInsertTarget.value && !insertPlacement) {
    validateMessageInsertTarget();
    insertPlacement = null;
  }

  const now = Date.now();
  const orderNow = getServerAlignedNowMs();
  const sendOrder = resolveSendDisplayOrder(now, orderNow);
  const localDisplayOrder = insertPlacement?.localDisplayOrder ?? sendOrder.localDisplayOrder;
  const explicitDisplayOrder = insertPlacement ? undefined : sendOrder.explicitDisplayOrder;
  const typingDurationMs = sendOrder.typingDurationMs;
  resetDraftOrderContext();

	const replyTo = chat.curReplyTo || undefined;
	stopTypingPreviewNow();
  suspendInlineSync = true;
  textToSend.value = '';
  syncSessionDraftSnapshot();
  clearInputModeCache();
  suspendInlineSync = false;
  if (isMobileInteractionMode.value) {
    ensureInputFocus();
  }
  chat.curReplyTo = null;

  const clientId = nanoid();
  const wasAtBottom = isNearBottom();
  const tmpMsg: Message = {
    id: clientId,
    createdAt: now,
    updatedAt: now,
    content: outgoingDraft,
    user: user.info,
    member: chat.curMember || undefined,
    quote: replyTo,
  };
  Object.assign(tmpMsg as any, buildOptimisticMessageIcModeFields(chat.icMode === 'ooc' ? 'ooc' : 'ic'));
  const activeIdentity = identityIdOverride
    ? findIdentityMeta(chat.curChannel?.id, identityIdOverride)
    : chat.getActiveIdentity(chat.curChannel?.id);
  const activeChannelId = String(chat.curChannel?.id || '').trim();
  const activeIdentityVariant = activeIdentity
    ? (identityVariantIdOverride
      ? (chat.getIdentityVariants(activeChannelId, activeIdentity.id).find(item => item.id === identityVariantIdOverride) || null)
      : chat.getActiveIdentityVariant(activeChannelId, activeIdentity.id))
    : null;
  const activeAppearance = resolveIdentityAppearancePreview(activeIdentity, activeIdentityVariant);
  if (activeIdentity) {
    const normalizedIdentityColor = normalizeHexColor(activeAppearance?.color || '') || undefined;
    (tmpMsg as any).senderRoleId = activeIdentity.id;
    (tmpMsg as any).sender_role_id = activeIdentity.id;
    (tmpMsg as any).sender_identity_variant_id = activeAppearance?.variantId || '';
    if (activeChannelId) {
      chat.recordIdentitySpoken(activeChannelId, activeIdentity.id, now);
    }
    if (!tmpMsg.identity) {
      tmpMsg.identity = {
        id: activeIdentity.id,
        variantId: activeAppearance?.variantId || '',
        displayName: activeAppearance?.displayName || activeIdentity.displayName,
        color: normalizedIdentityColor,
        avatarAttachment: activeAppearance?.avatarAttachmentId || activeIdentity.avatarAttachmentId,
        avatarDecorations: cloneAvatarDecorations(activeAppearance?.avatarDecorations),
        avatarDecoration: firstAvatarDecoration(activeAppearance?.avatarDecorations),
        isTemporary: Boolean(activeAppearance?.isTemporary),
      } as any;
    }
    (tmpMsg as any).sender_identity_decoration = cloneAvatarDecorations(activeAppearance?.avatarDecorations);
    if (activeAppearance?.displayName) {
      (tmpMsg as any).sender_member_name = activeAppearance.displayName;
    }
  }
  (tmpMsg as any).clientId = clientId;
  if (chat.curChannel) {
    (tmpMsg as any).channel = chat.curChannel;
  }
  (tmpMsg as any).displayOrder = localDisplayOrder;
  (tmpMsg as any).display_order = localDisplayOrder;
  if (insertPlacement) {
    (tmpMsg as any).insertAboveTargetId = insertPlacement.anchorMessageId;
  }

  const whisperTargetsForSend = chat.whisperTargets.slice();
  if (whisperTargetsForSend.length > 0) {
    (tmpMsg as any).isWhisper = true;
    (tmpMsg as any).whisperTo = whisperTargetsForSend[0];
    (tmpMsg as any).whisperToIds = whisperTargetsForSend;
  }

  setMessageSendStatus(tmpMsg as any, 'sending');
  rows.value.push(tmpMsg);
  sortRowsByDisplayOrder();
  instantMessages.add(tmpMsg);
  let sendOutcome:
    | { ok: true; messageId: string }
    | { ok: false; error: { code: string; message: string } };

  try {
    let finalContent: string;

    if (isRichMode) {
      // 富文本模式：直接发送 JSON
      finalContent = outgoingDraft;
    } else {
      // 纯文本模式：仅做安全转义与 Satori 占位替换，轻量 Markdown 交给前端渲染
      finalContent = await normalizePlainMessageContent(outgoingDraft);
    }

    if (
      activeChannelId
      && shouldAttemptCharacterApiReconnectBeforeBotCommand({
        content: finalContent,
        botCommandPrefixes: chat.curChannel?.botCommandPrefixes,
        botFeatureEnabled: effectiveBotFeatureEnabled.value,
        isBotPrivateChat: isCurrentBotPrivateChatChannel.value,
        characterApiReady: characterCardStore.isCharacterApiReady(activeChannelId),
        hadSuccessfulCharacterApiSession: characterCardStore.hasSuccessfulCharacterApiSession(activeChannelId),
      })
    ) {
      try {
        await characterCardStore.ensureCharacterApiReadyForBotCommand(activeChannelId);
      } catch (error) {
        console.warn('[CharacterCard] Failed to revalidate before bot command send', {
          channelId: activeChannelId,
          error,
        });
      }
    }

    tmpMsg.content = finalContent;
    const whisperTargetIds = whisperTargetsForSend
      .map((target) => String(target?.id || '').trim())
      .filter(Boolean);
    const newMsg = await chat.messageCreate(
      finalContent,
      replyTo?.id,
      whisperTargetIds[0],
      clientId,
      identityIdOverride,
      explicitDisplayOrder,
      whisperTargetIds,
      typingDurationMs,
      insertPlacement ? { beforeId: insertPlacement.beforeId, afterId: insertPlacement.afterId } : undefined,
      identityVariantIdOverride,
    );
    if (!newMsg) {
      throw new Error('message.create returned empty result');
    }
    if (isChannelDefaultDiceCommandResponse(newMsg)) {
      setMessageSendStatus(tmpMsg as any, 'sent');
      instantMessages.delete(tmpMsg);
      const index = rows.value.findIndex(item => item.id === tmpMsg.id);
      if (index !== -1) {
        rows.value.splice(index, 1);
      }
      resetInlineImages();
      pendingInlineSelection = null;
      textToSend.value = '';
      syncSessionDraftSnapshot();
      clearInputModeCache();
      ensureInputFocus();
      message.success(`默认骰已设为 ${String(newMsg.content || '')}`);
      sendOutcome = { ok: true, messageId: String(newMsg.id) };
      return sendOutcome;
    }
    for (const [k, v] of Object.entries(newMsg as Record<string, any>)) {
      (tmpMsg as any)[k] = v;
    }
    const interjectFirstEditSnapshot = interjectSession.value?.phase === 'awaiting-first-send'
      ? createInterjectEditSnapshot({
        messageId: String(tmpMsg.id || '').trim(),
        channelId: activeChannelId,
        originalContent: finalContent,
        draft: finalContent,
        mode: sendMode,
        isWhisper: whisperTargetsForSend.length > 0,
        whisperTargetId: whisperTargetIds[0] || null,
        whisperTargets: whisperTargetsForSend.map((target) => ({ ...target })),
        icMode: sendIcMode,
        identityId: activeIdentity?.id || identityIdOverride || null,
        identityVariantId: activeAppearance?.variantId || identityVariantIdOverride || null,
        identitySnapshot: createInterjectIdentitySnapshot(activeIdentity, activeAppearance),
      })
      : null;
    if (diceMatchesInDraft.length) {
      diceMatchesInDraft.forEach((entry) => recordDiceHistory(entry.source.trim()));
    }
    setMessageSendStatus(tmpMsg as any, 'sent');
    instantMessages.delete(tmpMsg);
    upsertMessage(tmpMsg);
    notifyNewMessageHighlight(tmpMsg);
    if (activeReeditSource) {
      try {
        await chat.messageRemove(activeReeditSource.channelId, activeReeditSource.messageId);
        removeRevokedPlaceholderMessage(activeReeditSource.messageId);
        chat.clearRevokedDraft(activeReeditSource.channelId, activeReeditSource.messageId);
      } catch (removeError) {
        console.warn('撤回占位持久化隐藏失败', removeError);
        message.warning('消息已发送，但撤回提示未持久化隐藏，请重试');
      } finally {
        if (
          reeditRevokedSource.value
          && reeditRevokedSource.value.channelId === activeReeditSource.channelId
          && reeditRevokedSource.value.messageId === activeReeditSource.messageId
        ) {
          reeditRevokedSource.value = null;
        }
      }
    }
    resetInlineImages();
    pendingInlineSelection = null;

    textToSend.value = '';
    syncSessionDraftSnapshot();
    clearInputModeCache();
    const shouldDeferInputFocusToInterjectFlow = Boolean(interjectSession.value);
    if (!shouldDeferInputFocusToInterjectFlow) {
      ensureInputFocus();
    }
    handleInterjectSendSuccess(tmpMsg, interjectFirstEditSnapshot);
    sendOutcome = { ok: true, messageId: String(tmpMsg.id || clientId) };
  } catch (e) {
    const reason = resolveMessageSendFailureReason(e);
    message.error(`发送失败：${reason}`);
    console.error('消息发送失败', e);
    suspendInlineSync = true;
    textToSend.value = draft;
    suspendInlineSync = false;
    syncInlineMarkersWithText(draft);
    syncSessionDraftSnapshot();
    const index = rows.value.findIndex(msg => msg.id === tmpMsg.id);
    if (index !== -1) {
      setMessageSendStatus(rows.value[index] as any, 'failed', reason);
    } else {
      setMessageSendStatus(tmpMsg as any, 'failed', reason);
    }
    handleInterjectSendFailure();
    sendOutcome = { ok: false, error: { code: 'MESSAGE_SEND_FAILED', message: reason } };
  }

  if (wasAtBottom && !insertPlacement) {
    toBottom();
  }
  return sendOutcome;
};

const send = throttle(() => performSend(), 500);

const handleDiceInsert = (expr: string) => {
  insertDiceExpression(expr.trim() ? `${expr.trim()} ` : expr);
  ensureInputFocus();
};

const handleIdentitySwitcherChange = () => {
  if (isEditingCurrentChannel.value) {
    emitEditingPreview();
    return;
  }
  emitTypingPreview();
};

const handleEditingIdentitySelected = (identityId: string) => {
  if (!isEditingCurrentChannel.value) {
    return;
  }
  chat.updateEditingIdentity(identityId || null);
  emitEditingPreview();
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
    syncSessionDraftSnapshot();
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
  syncDraftStartedAt(value);
  handleWhisperCommand(value);
  scheduleHistorySnapshot();
  scheduleSessionDraftSnapshot();
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
    clearInterjectSession();
    resetDraftOrderContext();
    whisperCandidateUsers.value = [];
    if (whisperPanelVisible.value) {
      void loadWhisperCandidates();
    }
  },
);

watch(inputMode, () => {
  syncSessionDraftSnapshot();
});

watch([
  inputPreviewEnabled,
  inputMode,
  inputIcMode,
  () => chat.curChannel?.id,
  () => activeIdentityForPreview.value?.id,
  () => activeIdentityAppearancePreviewSignature.value,
], () => {
  syncSelfTypingPreview();
});

watch(isEditing, (editing) => {
  if (editing) {
    resetDraftOrderContext();
    removeSelfTypingPreview();
    return;
  }
  syncSelfTypingPreview();
});

watch(
  () => activeIdentityAppearancePreviewSignature.value,
  (signature, previous) => {
    if (signature === previous) {
      return;
    }
    syncSelfTypingPreview();
    if (isEditing.value) {
      emitEditingPreview();
      return;
    }
    emitTypingPreview();
  },
);

watch(() => chat.whisperTargets.map((target) => target.id).join(','), (targetIds, prevIds) => {
  if (targetIds === prevIds) {
    return;
  }
  syncSessionDraftSnapshot();
  if (targetIds && whisperCandidateUsers.value.length === 0) {
    void loadWhisperCandidates();
  }
  stopTypingPreviewNow();
  emitTypingPreview();
});

watch(() => identityForm.color, (value) => {
  const normalized = normalizeColorDraftText(value);
  if (identityColorDraft.value !== normalized) {
    identityColorDraft.value = normalized;
  }
});

watch(() => identityVariantForm.color, (value) => {
  const normalized = normalizeColorDraftText(value);
  if (identityVariantColorDraft.value !== normalized) {
    identityVariantColorDraft.value = normalized;
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

const doUpload = (source: InlineUploadSource = 'default') => {
  pendingInlineSelection = captureSelectionRange();
  pendingInlineUploadSource = source;
  inlineImageInputRef.value?.click?.();
}

const handleToolbarUploadClick = () => {
  doUpload('rich-toolbar');
}

const handleRichUploadButtonClick = (source: InlineUploadSource = 'rich-editor') => {
  // 富文本编辑器内的上传按钮点击事件
  doUpload(source);
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
let disposeChatMessageHandlers: (() => void) | null = null;
const handleChannelSwitchEvent = (e: any) => {
  if (!firstLoad) return;
  const payload = (e as any)?.argv || {};
  const isReenter = !!payload?.reenter;
  persistOwnedSessionDraft();
  stopTypingPreviewNow();
  resetTypingPreview();
  stopEditingPreviewNow();
  cancelEditingSession();
  if (!isReenter) {
    textToSend.value = '';
  }
  draftOwnerChannelKey.value = currentChannelKey.value;
  clearInputModeCache();
  resetWindowState('live');
  pinnedRows.value = [];
  chat.clearMessageInsertTarget();
  chat.cancelMultiSelectRelocate();
  resetDragState();
  localReorderOps.clear();
  showButton.value = false;
  // 具体不知道原因，但是必须在这个位置reset才行
  // virtualListRef.value?.reset();
  refreshHistoryEntries();
  nextTick(() => {
    tryAutoRestoreSessionDraft();
  });
  const fetchTask = fetchLatestMessages();
  fetchTask.finally(() => {
    void fetchPinnedMessages();
    void maybePromptIdentitySync();
  });
};

const handleChannelContextCleared = () => {
  persistOwnedSessionDraft();
  stopTypingPreviewNow();
  resetTypingPreview();
  stopEditingPreviewNow();
  cancelEditingSession();
  textToSend.value = '';
  draftOwnerChannelKey.value = currentChannelKey.value;
  clearInputModeCache();
  resetWindowState('live');
  pinnedRows.value = [];
  chat.clearMessageInsertTarget();
  chat.cancelMultiSelectRelocate();
  resetDragState();
  localReorderOps.clear();
  showButton.value = false;
};

function handleOpenBattleSummaryEvent() {
  openBattleSummary();
}

onMounted(async () => {
  chatEvent.on('open-battle-summary' as any, handleOpenBattleSummaryEvent as any);
  await chat.tryInit();
  draftOwnerChannelKey.value = currentChannelKey.value;
  clearLegacyHistoryAutoRestoreStore();
  refreshHistoryEntries();
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityResume);
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('pageshow', handleForegroundResume);
    window.addEventListener('online', handleForegroundResume);
  }

  chatEvent.off('message-deleted', '*');
  chatEvent.on('message-deleted', (e?: Event) => {
    const targetId = e?.message?.id;
    if (!targetId) {
      return;
    }
    console.log('delete', targetId)
    const currentChannelId = String(chat.curChannel?.id || '').trim();
    for (let i of rows.value) {
      if (i.id === targetId) {
        cacheRevokedDraftFromMessage(i, currentChannelId);
        i.content = '';
        (i as any).is_revoked = true;
      }
      if (i.quote) {
        if (i.quote?.id === targetId) {
          i.quote.content = '';
          (i as any).quote.is_revoked = true;
        }
      }
    }
    for (let i of pinnedRows.value) {
      if (i.id === targetId) {
        cacheRevokedDraftFromMessage(i, currentChannelId);
        i.content = '';
        (i as any).is_revoked = true;
      }
      if (i.quote?.id === targetId) {
        i.quote.content = '';
        (i.quote as any).is_revoked = true;
      }
    }
  });

const handleMessageRemoved = (e?: Event) => {
  const targetId = e?.message?.id;
    if (!targetId) {
      return;
    }
    const removedChannelId = String(e?.channel?.id || chat.curChannel?.id || '').trim();
    if (removedChannelId) {
      chat.clearRevokedDraft(removedChannelId, targetId);
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
  if (chat.multiSelect?.relocate.active && chat.isRelocateSourceMessage(targetId)) {
    chat.cancelMultiSelectRelocate();
  }
  if (chat.isMessageInsertTarget(removedChannelId, targetId)) {
    chat.clearMessageInsertTarget(removedChannelId);
  }
  removePinnedMessage(targetId);
  if (archiveDrawerVisible.value) {
      const index = archivedMessagesRaw.value.findIndex((item) => item.id === targetId);
      if (index >= 0) {
        archivedMessagesRaw.value.splice(index, 1);
        updateArchivedDisplay();
      }
    }
  };

const handleMessageCreated = (e?: Event) => {
  if (!e?.message) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  const incomingChannelId = String(e.channel?.id || (incoming as any)?.channel?.id || (incoming as any)?.channel_id || '').trim();
  const currentChannelId = String(chat.curChannel?.id || '').trim();
  const isCurrentChannelMessage = !!incomingChannelId && incomingChannelId === currentChannelId;
	if (isCurrentChannelMessage && (incoming as any).diceVisual) {
		const payload = resolveDice3DPlaybackPayload(
			(incoming as any).diceVisual as DiceVisualPayload,
			String(user.info.id || '').trim(),
			dice3dConfig.value,
			dice3dProfile.value,
		);
		if (!isTheaterEmbedMode.value || !dice3dRuntime.forwardToTheater(payload)) {
			dice3dRuntime.play(payload);
		}
	}
  const isSelf = incoming.user?.id === user.info.id;
  const content = incoming.content || '';
  const currentUserId = user.info.id;
  const mentionIds = !isSelf ? collectMentionIdsFromContent(content) : new Set<string>();
  const isMentioned = !isSelf && (mentionIds.has(currentUserId) || mentionIds.has('all'));
  if (!isCurrentChannelMessage) {
    if (incomingChannelId && isMentioned) {
      chat.setChannelMentionState(incomingChannelId, true);
    }
    return;
  }
  const incomingIdentityId = resolveMessageIdentityId(incoming);
  if (incomingChannelId && incomingIdentityId) {
    chat.recordIdentitySpoken(
      incomingChannelId,
      incomingIdentityId,
      normalizeTimestamp(incoming.createdAt) ?? Date.now(),
    );
  }
  if (hasCardRefreshCommand(incoming.content || '')) {
    scheduleCharacterSheetRefresh();
  }
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
      setMessageSendStatus(matchedPending as any, 'sent');
      upsertMessage(matchedPending);
      notifyNewMessageHighlight(matchedPending);
      removeTypingPreview(incoming.user?.id);
      removeTypingPreview(incoming.user?.id, 'editing');
      if (shouldAutoScrollForSelfMessage(matchedPending)) {
        toBottom();
      }
      return;
    }
  } else {
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
          
          // 提取消息内容预览，兼容富文本、旧 HTML 和实体编码
          const rawContent = incoming.content || '';
          const plainText = extractPushNotificationPreviewText(rawContent);
          const preview = plainText.length > 50 ? plainText.slice(0, 50) + '...' : plainText;
          
          // 获取发送者头像（优先角色头像，其次用户/成员头像）
          const avatarUrl = resolveAttachmentUrl((incoming.identity as any)?.avatarAttachmentId)
            || incoming.member?.avatar
            || incoming.user?.avatar
            || undefined;
          const notificationTitle = `${isMentioned ? '[有人@我]' : ''}${chat.curChannel?.name || 'SealChat'}`;
          
          pushStore.showNotification(
            notificationTitle,
            `${senderName}: ${preview || '发送了一条消息'}`,
            chat.curChannel?.id || '',
            avatarUrl,
            incoming.id,
          );
        }
      });
    }
  }
  upsertMessage(incoming);
  if (!isSelf) {
    notifyNewMessageHighlight(incoming);
  }
  removeTypingPreview(incoming.user?.id);
  removeTypingPreview(incoming.user?.id, 'editing');
  if (isSelf) {
    if (shouldAutoScrollForSelfMessage(incoming)) {
      toBottom();
    }
  } else if (!inHistoryMode.value && !historyLocked.value) {
    nextTick(() => {
      scrollToBottom();
    });
  }
};

chatEvent.off('channel-image-layout-updated' as any, '*');
chatEvent.on('channel-image-layout-updated' as any, (e?: Event) => {
  const payload = (e as any)?.channelImageLayout || (e as any)?.channel_image_layout;
  if (!payload) {
    return;
  }
  const channelId = String(payload.channelId || payload.channel_id || e?.channel?.id || '').trim();
  if (!channelId || channelId !== chat.curChannel?.id) {
    return;
  }
  channelImageLayout.applyRealtimeUpdate(payload);
});

const handleMessageUpdated = (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  if ((e as any).is_interactive_update) {
    const incoming = normalizeMessageShape(e.message);
    const rowIndex = rows.value.findIndex((m: any) => m.id === incoming.id);
    if (rowIndex >= 0) {
      rows.value[rowIndex] = {
        ...rows.value[rowIndex],
        ...incoming,
      };
    }
    const pinIndex = pinnedRows.value.findIndex((m: any) => m.id === incoming.id);
    if (pinIndex >= 0) {
      pinnedRows.value[pinIndex] = {
        ...pinnedRows.value[pinIndex],
        ...incoming,
      };
      sortPinnedRows();
    }
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
    cancelEditingSession();
    clearInputModeCache();
    textToSend.value = '';
    syncSessionDraftSnapshot();
    ensureInputFocus();
  }
};

const chatViewMessageHandlers = {
  created: handleMessageCreated,
  updated: handleMessageUpdated,
  removed: handleMessageRemoved,
};
// 只替换聊天视图自己的监听器，避免清空小剧场桥接监听，并兼容 HMR。
const chatEventWithMessageOwner = chatEvent as typeof chatEvent & {
  __chatViewMessageHandlers?: typeof chatViewMessageHandlers
};
const previousChatViewMessageHandlers = chatEventWithMessageOwner.__chatViewMessageHandlers;
if (previousChatViewMessageHandlers) {
  chatEvent.off('message-created', previousChatViewMessageHandlers.created);
  chatEvent.off('message-updated', previousChatViewMessageHandlers.updated);
  chatEvent.off('message-removed', previousChatViewMessageHandlers.removed);
}
chatEventWithMessageOwner.__chatViewMessageHandlers = chatViewMessageHandlers;
chatEvent.on('message-created', chatViewMessageHandlers.created);
chatEvent.on('message-updated', chatViewMessageHandlers.updated);
chatEvent.on('message-removed', chatViewMessageHandlers.removed);
disposeChatMessageHandlers = () => {
  if (chatEventWithMessageOwner.__chatViewMessageHandlers !== chatViewMessageHandlers) return;
  chatEvent.off('message-created', chatViewMessageHandlers.created);
  chatEvent.off('message-updated', chatViewMessageHandlers.updated);
  chatEvent.off('message-removed', chatViewMessageHandlers.removed);
  delete chatEventWithMessageOwner.__chatViewMessageHandlers;
};

chatEvent.off('message-reordered', '*');
chatEvent.on('message-reordered', (e?: Event) => {
  if (!e || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const reorderPayload = (e as any)?.reorder;
  if (reorderPayload) {
    applyReorderPayload(reorderPayload);
  } else if (e.message) {
    upsertMessage(normalizeMessageShape(e.message));
  }
  const clientOpId = reorderPayload?.clientOpId;
  if (clientOpId && localReorderOps.has(clientOpId)) {
    localReorderOps.delete(clientOpId);
  }
  chatEvent.emit('battle-report-display-message-reordered' as any, { channelId: e.channel?.id });
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
    if (chat.isMessageInsertTarget(chat.curChannel?.id, incoming.id)) {
      clearMessageInsertTarget();
    }
  }
  removePinnedMessage(incoming.id);
  if (archiveDrawerVisible.value) {
    const entry = toArchivedPanelEntry(incoming as Message);
    const index = archivedMessagesRaw.value.findIndex(item => item.id === entry.id);
    if (index >= 0) {
      archivedMessagesRaw.value.splice(index, 1, entry);
    } else if (!archivedSearchQuery.value.trim()) {
      archivedMessagesRaw.value.unshift(entry);
    }
    updateArchivedDisplay();
  }
});

chatEvent.off('message-pinned', '*');
chatEvent.on('message-pinned', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  (incoming as any).isPinned = true;
  upsertPinnedMessage(incoming);
  upsertMessage(incoming as Message);
});

chatEvent.off('message-unpinned', '*');
chatEvent.on('message-unpinned', (e?: Event) => {
  if (!e?.message || e.channel?.id !== chat.curChannel?.id) {
    return;
  }
  const incoming = normalizeMessageShape(e.message);
  (incoming as any).isPinned = false;
  removePinnedMessage(incoming.id);
  upsertMessage(incoming as Message);
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
      updateArchivedDisplay();
    }
  }
});

chatEvent.off('channel-presence-updated', '*');
chatEvent.on('channel-presence-updated', (e?: Event) => {
  const channelId = e?.channel?.id || '';
  if (!channelId) {
    return;
  }
  const presenceList = Array.isArray(e?.presence) ? e.presence : [];
  chat.patchChannelAttributes(channelId, {
    membersCount: presenceList.length,
  } as any);
  if (channelId !== chat.curChannel?.id) {
    return;
  }
  if (channelId !== presenceBadgeChannelId) {
    presenceBadgeChannelId = channelId;
    presenceBadgeInitialized = false;
    presenceBadgeUsers.clear();
  }
  let hasNewPresence = false;
  const nextChannelUsers: User[] = [];
  const seenUserIds = new Set<string>();
  if (typeof (e as any)?.timestamp === 'number') {
    chat.syncServerTime((e as any).timestamp);
  }
  presenceList.forEach((item) => {
    const userId = item?.user?.id;
    if (!userId || seenUserIds.has(userId)) {
      return;
    }
    seenUserIds.add(userId);
    if (!presenceBadgeUsers.has(userId)) {
      presenceBadgeUsers.add(userId);
      if (presenceBadgeInitialized) {
        hasNewPresence = true;
      }
    }
    if (item?.user) {
      nextChannelUsers.push(item.user as User);
    }
    chat.updatePresence(userId, {
      lastPing: typeof item?.lastSeen === 'number' ? chat.serverTsToLocal(item.lastSeen) : Date.now(),
      latencyMs: typeof item?.latency === 'number' ? item.latency : Number(item?.latency) || 0,
      isFocused: !!item?.focused,
    });
  });
  chat.curChannelUsers = nextChannelUsers;
  if (!presenceBadgeInitialized) {
    presenceBadgeInitialized = true;
    return;
  }
  if (hasNewPresence) {
    initCharacterCardBadge(channelId);
    initCharacterRemark(channelId);
  }
});

  chatEvent.off('channel-deleted', '*');
  chatEvent.on('channel-deleted', async (e) => {
    const deletedChannelId = String(e?.channel?.id || '').trim();
    const currentChannelId = String(chat.curChannel?.id || '').trim();
    if (!deletedChannelId || deletedChannelId !== currentChannelId) {
      return;
    }
    await chat.channelList(chat.currentWorldId, true, { autoSwitch: false });
    const fallbackChannelId = resolveDeletedChannelFallbackId({
      deletedChannelId,
      currentChannelId,
      channelTree: chat.channelTree,
    });
    if (fallbackChannelId) {
      await chat.channelSwitchTo(fallbackChannelId);
    } else {
      chat.clearCurrentChannelContext('channel-deleted:noFallbackChannel');
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
      const reconnectWindow = intersectMessageFilterTimeWindow(lastCreatedAt, now);

      // 获取断线期间消息
      const messages = reconnectWindow
        ? await chat.messageListDuring(
            chat.curChannel?.id || '',
            reconnectWindow.fromTime,
            reconnectWindow.toTime,
            { ...buildMessageFilterOptions() },
          )
        : { data: [], next: '' };
      console.log('时间起始', reconnectWindow?.fromTime, reconnectWindow?.toTime)
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
    await fetchPinnedMessages();
    initCharacterRemark(chat.curChannel?.id);
    void syncCharacterCardAfterResume('ws-connected', {
      forceIdentityReload: true,
      bypassCooldown: true,
    });
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

  chatEvent.off('channel-context-cleared', '*');
  chatEvent.on('channel-context-cleared', handleChannelContextCleared as any);
  chatEvent.on('channel-switch-to', handleChannelSwitchEvent as any)
  chatEvent.on('battle-report-display-refresh' as any, handleBattleReportDisplayRefresh as any);
  chatEvent.on('battle-report-open-editor' as any, handleBattleReportOpenEditorRequest as any);

  await fetchLatestMessages();
  await fetchPinnedMessages();
  initCharacterRemark(chat.curChannel?.id);
  initialMessageJumpReady.value = true;
  void consumePendingMessageJump();
  firstLoad = true;
  await maybePromptIdentitySync();

  await utils.configGet();
  if (!chat.isObserver) {
    await utils.commandsRefresh();
  }

  chat.channelRefreshSetup()

  // 检查并启动新用户引导
  if (!chat.isObserver) {
    onboarding.checkAndStartOnboarding();
  }
})

onBeforeUnmount(() => {
  disposeChatMessageHandlers?.();
  disposeChatMessageHandlers = null;
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityResume);
  }
  if (typeof window !== 'undefined') {
    window.removeEventListener('pageshow', handleForegroundResume);
    window.removeEventListener('online', handleForegroundResume);
  }
  scheduleHistorySnapshot.cancel();
  scheduleSessionDraftSnapshot.cancel();
  syncSessionDraftSnapshot();
  disposeImageLayoutRowObserver();
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
    const queryWindow = intersectMessageFilterTimeWindow(from, to);
    if (!queryWindow) {
      return { messages: [] as Message[], cursor: '', reachedStart: true };
    }
    const reachedFilterStart = chat.filterState.fromTime !== null
      && queryWindow.fromTime === chat.filterState.fromTime;
    try {
      const resp = await chat.messageListDuring(chat.curChannel!.id, queryWindow.fromTime, queryWindow.toTime, {
        ...buildMessageFilterOptions(),
      });
      const normalized = normalizeMessageList(resp?.data || []).filter((msg) => {
        const created = normalizeTimestamp(msg.createdAt) ?? 0;
        return created < anchorTimestamp;
      });
      if (normalized.length) {
        const reachedStart = (queryWindow.fromTime === 0 || reachedFilterStart) && !resp?.next;
        return { messages: normalized, cursor: resp?.next ?? '', reachedStart };
      }
      if (queryWindow.fromTime === 0 || reachedFilterStart) {
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

let latestMessagesFetchEpoch = 0;
let latestMessagesRefetchQueued = false;

const scheduleLatestMessagesRefetch = () => {
  if (latestMessagesRefetchQueued) {
    return;
  }
  latestMessagesRefetchQueued = true;
  Promise.resolve().then(() => {
    latestMessagesRefetchQueued = false;
    if (!chat.curChannel?.id || messageWindow.loadingLatest) {
      return;
    }
    void fetchLatestMessages().catch((error) => {
      console.warn('[channel-load] deferred messages fetch failed', error);
    });
  });
};

const fetchLatestMessages = async () => {
  if (!chat.curChannel?.id || messageWindow.loadingLatest) {
    return;
  }
  const channelIdAtStart = chat.curChannel.id;
  const fetchEpoch = ++latestMessagesFetchEpoch;
  const filterSignatureAtStart = messageFilterSignature.value;
  const isStale = () => (
    fetchEpoch !== latestMessagesFetchEpoch
    || chat.curChannel?.id !== channelIdAtStart
    || messageFilterSignature.value !== filterSignatureAtStart
  );
  console.info('[channel-load] messages-fetch-start', {
    channelId: channelIdAtStart,
    fetchEpoch,
    ts: Date.now(),
  });
  let fetchSucceeded = false;
  const previousRows = rows.value.slice();
  resetWindowState('live', { preserveRows: true });
  resetTypingPreview();
  messageWindow.loadingLatest = true;
  try {
    const resp = await chat.messageList(channelIdAtStart, undefined, {
      limit: INITIAL_MESSAGE_LOAD_LIMIT,
      ...buildMessageFilterOptions(),
    });
    if (isStale()) {
      return;
    }
    fetchSucceeded = true;
    console.info('[channel-load] messages-fetch-success', {
      channelId: channelIdAtStart,
      fetchEpoch,
      count: Array.isArray(resp.data) ? resp.data.length : 0,
      ts: Date.now(),
    });
    rows.value = normalizeMessageList(resp.data);
    sortRowsByDisplayOrder();
    validateMessageInsertTarget({ silent: true });
    applyCursorUpdate({ before: resp?.next ?? '' });
    computeAfterCursorFromRows();
    await nextTick();
    scrollToBottom();
    showButton.value = false;
    await autoFillIfNeeded();
    tryAutoRestoreSessionDraft();
    console.info('[channel-load] messages-rendered', {
      channelId: channelIdAtStart,
      fetchEpoch,
      rows: rows.value.length,
      ts: Date.now(),
    });
  } catch (error) {
    if (isStale()) {
      return;
    }
    rows.value = previousRows;
    resetWindowState('live', { preserveRows: true, preserveHistoryLock: false });
    throw error;
  } finally {
    const stale = isStale();
    messageWindow.loadingLatest = false;
    console.info('[channel-load] messages-fetch-finish', {
      channelId: channelIdAtStart,
      fetchEpoch,
      stale,
      ok: fetchSucceeded,
      ts: Date.now(),
    });
    if (stale) {
      scheduleLatestMessagesRefetch();
    }
  }
};

watch(
  () => messageFilterSignature.value,
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
  if (isSearchBrowseActive() && !searchBrowseSession.hasMoreBefore && !messageWindow.beforeCursor) {
    messageWindow.hasReachedStart = true;
    messageWindow.beforeCursorExhausted = true;
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
        limit: PAGINATED_MESSAGE_LOAD_LIMIT,
        ...buildMessageFilterOptions(),
      });
      normalized = normalizeMessageList(resp.data);
      nextCursor = resp?.next ?? '';
      if (!normalized.length && !nextCursor) {
        reachedStart = true;
      }
    } else {
      reachedStart = true;
      nextCursor = '';
    }

    if (nextCursor !== undefined) {
      applyCursorUpdate({ before: nextCursor ?? '' });
      if (isSearchBrowseActive()) {
        searchBrowseSession.hasMoreBefore = Boolean(nextCursor);
      }
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
      if (isSearchBrowseActive()) {
        searchBrowseSession.beforeCursor = '';
        searchBrowseSession.hasMoreBefore = false;
      }
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
  if (!messageWindow.afterCursor) {
    messageWindow.hasReachedLatest = true;
    if (isSearchBrowseActive()) {
      searchBrowseSession.hasMoreAfter = false;
    }
    return false;
  }
  messageWindow.loadingAfter = true;
  try {
    const resp = await chat.messageList(chat.curChannel.id, messageWindow.afterCursor, {
      limit: PAGINATED_MESSAGE_LOAD_LIMIT,
      direction: 'after',
      ...buildMessageFilterOptions(),
    });
    const normalized = normalizeMessageList(resp?.data || []);
    if (normalized.length) {
      mergeIncomingMessages(normalized);
      messageWindow.hasReachedLatest = false;
      if (isSearchBrowseActive()) {
        searchBrowseSession.hasMoreAfter = Boolean(resp?.next);
      }
      return true;
    }
    messageWindow.hasReachedLatest = true;
    if (isSearchBrowseActive()) {
      searchBrowseSession.hasMoreAfter = false;
    }
    if (isNearBottom()) {
      updateViewMode('live');
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
  const shortcutDraft = beforeCursor.slice(slashIndex);

  // 检测是否是快捷命令模式 (/e 空格 或 /w 空格) - 仅当触发字符为 / 时检查
  if (trigger === '/' && /^[ew]\s/.test(query)) {
    keywordSuggestVisible.value = false;
    return;
  }

  if (shouldSuppressKeywordSuggestForIdentityShortcut(shortcutDraft, trigger)) {
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

const handleChatInputBlur = () => {
  handleKeywordSuggestBlur();
  syncSessionDraftSnapshot();
};

const toolbarHotkeyOrder: ToolbarHotkeyKey[] = [
  'send',
  'icToggle',
  'interject',
  'whisper',
  'upload',
  'richMode',
  'broadcast',
  'emoji',
  'wideInput',
  'history',
  'diceTray',
];

const toolbarHotkeyHandlers: Record<ToolbarHotkeyKey, (event: KeyboardEvent) => boolean | void> = {
  send: (event) => {
    if (event.isComposing) {
      return false;
    }
    if (isEditing.value) {
      void saveEdit();
    } else {
      send();
    }
    return true;
  },
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
  interject: () => {
    if (!canStartInterject.value) {
      return false;
    }
    startInterject();
    return true;
  },
  whisper: () => {
    startWhisperSelection();
    return true;
  },
  upload: () => {
    handleToolbarUploadClick();
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
    const result = handler(event);
    if (result !== false) {
      event.preventDefault();
      event.stopPropagation();
    }
    return result !== false;
  }
  return false;
};

const customSendShortcutEnabled = computed(() => {
  const config = display.settings.toolbarHotkeys?.send;
  return Boolean(config?.enabled && config.hotkey);
});

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
  if (isActualMobileUa) {
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

  if (e.key === 'Enter' && !customSendShortcutEnabled.value) {
    if (e.isComposing) {
      return;
    }
    const shortcut = display.settings.sendShortcut || 'enter';
    const ctrlLike = e.ctrlKey || e.metaKey;
    const isBareEnter = !ctrlLike && !e.shiftKey && !e.altKey;
    let shouldSend = false;
    if (shortcut === 'enter') {
      shouldSend = isBareEnter;
    } else {
      shouldSend = ctrlLike && !e.shiftKey && !e.altKey;
    }
    if (shouldSend) {
      if (isEditing.value) {
        void saveEdit();
      } else {
        send();
      }
      e.preventDefault();
    }
  }
}

const {
  atOptions,
  atLoading,
  atPrefix,
  atRenderLabel,
  atHandleSearch,
} = useMentionSuggestions({
  chat,
  utils,
  inputIcMode,
  textToSend,
  pauseKeydown,
});

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

const {
  allMessageIds,
  forwardDialogVisible,
  forwardDialogSourceChannelId,
  forwardDialogSourceWorldId,
  forwardDialogMessageIds,
  forwardDialogMessages,
  handleMultiSelectForward,
  handleMessageForwardSuccess,
  handleMultiSelectCopy,
  handleMultiSelectArchive,
  handleMultiSelectDelete,
  handleMultiSelectCopyImage,
  handleMultiSelectMoveToBottom,
  handleMultiSelectRelocate,
  handleCancelMultiSelectRelocate,
  handleRelocateTargetPick,
  handleMultiSelectAll,
} = useMessageSelection({ chat, rows, pinnedRows, message, dialog });

const insertGalleryInline = (attachmentId: string, selection?: SelectionRange) => {
  const normalized = attachmentId.startsWith('id:') ? attachmentId.slice(3) : attachmentId;
  if (inputMode.value === 'rich') {
    const editor = textInputRef.value?.getEditor?.();
    editor?.chain().focus().setImage({ src: `/api/v1/attachment/${normalized}` }).run();
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
  const range = selection ?? captureSelectionRange();
  const start = Math.max(0, Math.min(range.start, range.end));
  const end = Math.max(start, Math.max(range.start, range.end));
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

const isFavoriteQuickGalleryEmoji = (item: GalleryItem) => {
  return !!gallery.favoritesCollectionId && item.collectionId === gallery.favoritesCollectionId;
};

const handleQuickGalleryEmojiClick = (item: GalleryItem) => {
  if (isFavoriteQuickGalleryEmoji(item) || display.settings.quickGalleryLinkedEmojiSendDirectly) {
    void sendEmoji(item);
    return;
  }
  handleGalleryEmojiClick(item);
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

const openBattleSummary = () => {
  if (!chat.curChannel?.id) {
    message.error('未选择频道')
    return
  }
  battleReportDrawerVisible.value = true
}

const handleBattleReportOpenEditorRequest = async (payload: any) => {
  if (payload?.deferToDrawer) return
  const reportId = String(payload?.reportId || '').trim()
  if (!reportId) return
  battleReportDrawerVisible.value = true
  await nextTick()
  chatEvent.emit('battle-report-open-editor' as any, {
    ...payload,
    deferToDrawer: true,
  })
}

onBeforeUnmount(() => {
  handleInputResizeEnd();
  clearMobileSendLongPressTimer();
  chatEvent.off('channel-identity-open', handleIdentityMenuOpen);
  chatEvent.off('channel-identity-updated', handleIdentityUpdated);
  chatEvent.off('action-ribbon-toggle', handleActionRibbonToggleRequest);
  chatEvent.off('action-ribbon-state-request', handleActionRibbonStateRequest);
  chatEvent.off('open-display-settings', handleOpenDisplaySettings);
  chatEvent.off('channel-context-cleared', handleChannelContextCleared as any);
  chatEvent.off('channel-switch-to', handleChannelSwitchEvent as any);
  chatEvent.off('battle-report-display-refresh' as any, handleBattleReportDisplayRefresh as any);
  chatEvent.off('battle-report-open-editor' as any, handleBattleReportOpenEditorRequest as any);
  chatEvent.off('open-battle-summary' as any, handleOpenBattleSummaryEvent as any);
  chatEvent.off('world-dice3d-updated' as any, handleDice3DSettingsUpdated as any);
  chatEvent.off('world-member-dice3d-updated' as any, handleDice3DSettingsUpdated as any);
  revokeIdentityObjectURL();
  revokeIdentityVariantObjectURL();
  searchHighlightTimers.forEach((timer) => window.clearTimeout(timer));
  searchHighlightTimers.clear();
  sendStatusDelayTimers.forEach((timer) => window.clearTimeout(timer));
  sendStatusDelayTimers.clear();
});
</script>

<template>
  <div
    ref="chatRootContainerRef"
    class="flex flex-col h-full justify-between chat-root-container"
    :class="{ 'chat-root-container--embed': isEmbedMode }"
  >
    <!-- 频道背景层 -->
    <div v-if="channelBackgroundStyle" class="channel-background-layer" :style="channelBackgroundStyle"></div>
    <div v-if="channelBackgroundOverlayStyle" class="channel-background-overlay" :style="channelBackgroundOverlayStyle"></div>
    <!-- 功能面板 -->
    <transition name="slide-down">
      <div v-if="showActionRibbon && (!isEmbedMode || isTheaterEmbedMode)" class="chat-top-toolbar-stack">
        <ChatActionRibbon
          :filters="chat.filterState"
          :roles="ribbonRoleOptions"
          :archive-active="archiveDrawerVisible"
          :export-active="exportManagerVisible"
          :identity-active="identityDialogVisible"
          :gallery-active="galleryPanelVisible"
          :display-active="displaySettingsVisible"
          :favorite-active="channelFavoritesVisible"
          :character-remark-active="characterRemarkManagerVisible"
          :channel-images-active="channelImagesPanelVisible"
          :battle-summary-enabled="showBattleSummary"
          :battle-summary-active="battleReportDrawerVisible"
          :can-import="canManageWorldKeywords"
          :import-active="importDialogVisible"
          :split-enabled="splitEntryEnabled"
          :split-active="false"
          :theater-enabled="theaterEntryEnabled"
          :theater-active="false"
          :ic-ooc-split-enabled="splitEntryEnabled"
          :ic-ooc-split-active="false"
          :sticky-note-enabled="true"
          :sticky-note-active="stickyNoteStore.uiVisible"
		  :dice3d-enabled="!chat.observerMode"
		  :dice3d-active="dice3dSettingsVisible"
          :webhook-enabled="webhookManageAllowed"
          :webhook-active="webhookDrawerVisible"
          :bridge-status-active="bridgeStatusDrawerVisible"
          :email-notification-enabled="webhookManageAllowed"
          :email-notification-active="emailNotificationDrawerVisible"
          :character-card-enabled="!!chat.curChannel?.id"
          :character-card-active="characterCardPanelVisible"
          @update:filters="chat.setFilterState($event)"
          @open-archive="archiveDrawerVisible = true"
          @open-export="exportManagerVisible = true"
          @open-import="importDialogVisible = true"
          @open-identity-manager="openIdentityManager"
          @open-gallery="openGalleryPanel"
          @open-display-settings="displaySettingsVisible = true"
          @open-favorites="channelFavoritesVisible = true"
          @open-character-remark="characterRemarkManagerVisible = true"
          @open-channel-images="openChannelImagesPanel"
          @open-battle-summary="openBattleSummary"
          @open-split="openSplitView"
          @open-theater="openTheaterView"
          @open-ic-ooc-split="openIcOocSplitView"
          @toggle-sticky-note="toggleStickyNotes"
		  @open-dice3d="openDice3DSettings"
          @open-webhook="webhookDrawerVisible = true"
          @open-bridge-status="bridgeStatusDrawerVisible = true"
          @open-email-notification="emailNotificationDrawerVisible = true"
          @open-character-card="openCharacterCardPanel"
          @clear-filters="chat.setFilterState({ icFilter: 'all', showArchived: false, roleIds: [], whisperOnly: false, fromTime: null, toTime: null })"
        />
      </div>
    </transition>

    <Teleport v-if="inputExtraActionsTeleportTarget" :to="inputExtraActionsTeleportTarget">
      <div
        class="chat-input-actions__teleport-content"
        :class="{ 'chat-input-actions__teleport-content--compact-toolbar': isMinimalInputActive }"
      >
        <div class="chat-input-actions__group chat-input-actions__group--leading chat-input-actions__group--leading-extras">
          <div v-if="showDiceTrayTrigger" class="chat-input-actions__cell">
            <div class="emoji-trigger">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    quaternary
                    circle
                    ref="emojiTriggerButtonRef"
                    @click="handleEmojiTriggerClick($event)"
                  >
                    <template #icon>
                      <n-icon :component="EmojiTriggerIcon" size="18" />
                    </template>
                  </n-button>
                </template>
                打开表情盘
              </n-tooltip>

              <n-popover
                v-model:show="emojiPopoverShow"
                trigger="manual"
                placement="bottom-start"
                :x="emojiPopoverXCoord"
                :y="emojiPopoverYCoord"
                @clickoutside="emojiPopoverShow = false"
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

                  <div class="emoji-panel__tabs emoji-panel__tabs--with-utf">
                    <template v-if="hasEmojiItems && hasMultipleTabs">
                      <button
                        class="emoji-panel__tab"
                        :class="{ 'emoji-panel__tab--active': emojiPanelTab === 'gallery' && activeEmojiTab === null }"
                        @click="switchEmojiPanelTab('gallery'); activeEmojiTab = null"
                      >
                        全部
                      </button>
                      <button
                        v-for="tab in emojiTabOptions"
                        :key="tab.id"
                        class="emoji-panel__tab"
                        :class="{ 'emoji-panel__tab--active': emojiPanelTab === 'gallery' && activeEmojiTab === tab.id }"
                        :title="tab.name"
                        @click="switchEmojiPanelTab('gallery'); activeEmojiTab = tab.id"
                      >
                        <span class="emoji-panel__tab-text">{{ tab.name }}</span>
                      </button>
                    </template>
                    <button
                      class="emoji-panel__tab emoji-panel__tab--utf"
                      :class="{ 'emoji-panel__tab--active': emojiPanelTab === 'utf' }"
                      title="UTF 表情"
                      @click="switchEmojiPanelTab('utf')"
                    >
                      <span class="emoji-panel__tab-icon" aria-hidden="true">😊</span>
                      <span class="emoji-panel__tab-text">UTF</span>
                    </button>
                    <n-tooltip trigger="hover">
                      <template #trigger>
                        <button
                          class="emoji-panel__tab emoji-panel__tab--variant"
                          :class="{
                            'emoji-panel__tab--active': emojiPanelTab === 'variant',
                            'emoji-panel__tab--muted': !activeIdentityForEmojiPanel || !hasIdentityVariantOptions,
                          }"
                          :aria-disabled="!activeIdentityForEmojiPanel || !hasIdentityVariantOptions"
                          @click="handleEmojiVariantTabClick"
                        >
                          <span class="emoji-panel__tab-icon" aria-hidden="true">🎭</span>
                          <span class="emoji-panel__tab-text">差分</span>
                        </button>
                      </template>
                      {{ identityVariantTabTooltip }}
                    </n-tooltip>
                  </div>

                  <div v-if="(emojiPanelTab === 'gallery' && hasEmojiItems) || emojiPanelTab === 'variant'" class="emoji-panel__search">
                    <n-input
                      v-model:value="emojiSearchQuery"
                      size="small"
                      :placeholder="emojiPanelTab === 'variant' ? '搜索差分关键词或备注...' : '搜索表情...'"
                      clearable
                    />
                  </div>

                  <div v-if="emojiPanelTab === 'gallery' && !hasEmojiItems" class="emoji-panel__empty">
                    当前没有收藏的表情，可以在聊天窗口的图片上<b class="px-1">长按</b>或<b class="px-1">右键</b>添加
                  </div>

                  <div
                    :key="`emoji-panel-${emojiPanelRenderKey}`"
                    ref="emojiPanelContentRef"
                    class="emoji-panel__content"
                    :class="{ 'emoji-panel__content--utf': emojiPanelTab === 'utf' }"
                    @scroll="handleEmojiPanelContentScroll"
                  >
                    <template v-if="emojiPanelTab === 'utf'">
                      <div class="emoji-panel__utf-host">
                        <EmojiPickerModal
                          embedded
                          mode="emoji-only"
                          initial-tab="emoji"
                          @select="handleUtfEmojiSelect"
                        />
                      </div>
                    </template>
                    <template v-else-if="emojiPanelTab === 'variant'">
                      <div v-if="!activeIdentityForEmojiPanel" class="emoji-panel__empty">
                        请先选择频道角色
                      </div>
                      <div v-else-if="!hasIdentityVariantOptions" class="emoji-panel__empty">
                        当前频道角色没有可用的头像差分
                      </div>
                      <div v-else class="identity-variant-picker">
                        <n-tooltip trigger="hover">
                          <template #trigger>
                            <button
                              type="button"
                              class="identity-variant-picker__item identity-variant-picker__item--default"
                              :class="{ 'is-active': !effectiveIdentityVariantForEmojiPanel }"
                              @click="handleEmojiVariantSelect('')"
                            >
                              <div class="identity-variant-picker__badge">↺</div>
                              <AvatarVue
                                :size="40"
                                :border="false"
                                :src="resolveAttachmentUrl(activeIdentityForEmojiPanel.avatarAttachmentId) || (activeIdentityForEmojiPanel.isTemporary ? '' : user.info.avatar)"
                                :use-text-fallback="activeIdentityForEmojiPanel.isTemporary"
                                :fallback-text="activeIdentityForEmojiPanel.displayName"
                              />
                              <div class="identity-variant-picker__title">默认头像</div>
                              <div class="identity-variant-picker__hint">恢复</div>
                            </button>
                          </template>
                          {{ describeIdentityVariantCard(null) }}
                        </n-tooltip>
                        <n-tooltip
                          v-for="variant in filteredIdentityVariantOptions"
                          :key="variant.id"
                          trigger="hover"
                        >
                          <template #trigger>
                            <button
                              type="button"
                              class="identity-variant-picker__item"
                              :class="{ 'is-active': effectiveIdentityVariantForEmojiPanel?.id === variant.id }"
                              @click="handleEmojiVariantSelect(variant.id)"
                            >
                              <div class="identity-variant-picker__badge">
                                <img
                                  v-if="isVariantSelectorEmojiAttachment(variant.selectorEmoji)"
                                  :src="resolveVariantSelectorEmojiSrc(variant.selectorEmoji)"
                                  :alt="resolveVariantNote(variant)"
                                />
                                <span v-else>{{ variant.selectorEmoji || '🙂' }}</span>
                              </div>
                              <AvatarVue
                                :size="40"
                                :border="false"
                                :src="resolveAttachmentUrl(variant.avatarAttachmentId || activeIdentityForEmojiPanel.avatarAttachmentId) || (activeIdentityForEmojiPanel.isTemporary ? '' : user.info.avatar)"
                                :use-text-fallback="activeIdentityForEmojiPanel.isTemporary"
                                :fallback-text="variant.displayName || activeIdentityForEmojiPanel.displayName"
                              />
                              <div class="identity-variant-picker__title">{{ resolveVariantNote(variant) }}</div>
                              <div class="identity-variant-picker__hint">={{ variant.keyword }}</div>
                            </button>
                          </template>
                          <span class="identity-variant-picker__tooltip">{{ describeIdentityVariantCard(variant) }}</span>
                        </n-tooltip>
                        <div v-if="emojiSearchQuery && !filteredIdentityVariantOptions.length" class="emoji-panel__empty">
                          没有匹配的头像差分
                        </div>
                      </div>
                    </template>
                    <template v-else>
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
                          @click="handleQuickGalleryEmojiClick(item)"
                        >
                          <img :src="getEmojiItemSrc(item)" :alt="item.remark || '表情'" />
                          <div class="emoji-caption" :title="item.remark || `收藏${idx + 1}`">{{ item.remark || `收藏${idx + 1}` }}</div>
                          <div class="emoji-item__actions">
                            <n-button text size="tiny" @click.stop="openEmojiRemarkEditor(item)">备注</n-button>
                          </div>
                        </div>
                      </div>
                    </template>
                    </template>
                    <div
                      v-if="emojiPanelTab === 'gallery' && filteredEmojiItems.length"
                      ref="emojiPanelLoadMoreSentinelRef"
                      class="emoji-grid__sentinel"
                      aria-hidden="true"
                    ></div>
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
              compact
            />
          </div>
          <div class="chat-input-actions__cell">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  quaternary
                  circle
                  :type="interjectButtonType"
                  :disabled="!canStartInterject"
                  @click="startInterject"
                >
                  <template #icon>
                    <n-icon :component="MessagePlus" size="18" />
                  </template>
                </n-button>
              </template>
              {{ interjectTooltip }}
            </n-tooltip>
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
                <n-button quaternary circle @click="handleToolbarUploadClick">
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
          <div v-if="showAIPolish" class="chat-input-actions__cell">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button quaternary circle :disabled="!canRunAIPolish" @click="runAIPolish">
                  <template #icon>
                    <n-icon :component="Palette" size="18" />
                  </template>
                </n-button>
              </template>
              润色
            </n-tooltip>
          </div>
          <div class="chat-input-actions__cell">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  class="chat-dice-button"
                  quaternary
                  circle
                  :disabled="(!canUseBuiltInDice && !effectiveBotFeatureEnabled) || diceFeatureUpdating"
                  @click="toggleDiceTray"
                >
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
          </div>
        </div>
      </div>
    </Teleport>

    <n-drawer v-model:show="webhookDrawerVisible" placement="right" :width="webhookDrawerWidth">
      <n-drawer-content closable>
        <template #header>Webhook 授权</template>
        <WebhookIntegrationManager :channel-id="chat.curChannel?.id || ''" />
      </n-drawer-content>
    </n-drawer>

    <n-drawer v-model:show="bridgeStatusDrawerVisible" placement="right" :width="bridgeStatusDrawerWidth">
      <n-drawer-content closable>
        <template #header>桥接状态</template>
        <BridgeStatusPanel
          :channel-id="chat.curChannel?.id || ''"
          :refreshing="avatarReissueLoading"
          :result-text="avatarReissueResultText"
          @refresh-avatars="handleBridgeAvatarReissue"
        />
      </n-drawer-content>
    </n-drawer>

    <n-drawer v-model:show="emailNotificationDrawerVisible" placement="right" :width="emailNotificationDrawerWidth">
      <n-drawer-content closable>
        <template #header>未读提醒</template>
        <EmailNotificationManager scope-type="channel" :scope-id="chat.curChannel?.id || ''" />
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
      v-if="display.settings.showPinnedMessages && pinnedRows.length > 0"
      class="chat-pinned-zone px-4"
      :class="[`chat--layout-${display.layout}`, `chat--palette-${display.palette}`, { 'chat--no-avatar': !display.showAvatar, 'chat--has-background': !!channelBackgroundStyle }]"
    >
      <div class="chat-pinned-zone__header" @click="pinnedCollapsed = !pinnedCollapsed">
        <span class="chat-pinned-zone__title">置顶消息</span>
        <n-icon class="chat-pinned-zone__toggle" size="14">
          <component :is="pinnedCollapsed ? ChevronRight : ChevronDown" />
        </n-icon>
      </div>
      <div v-show="!pinnedCollapsed" class="chat-pinned-zone__list">
        <template v-for="pinItem in pinnedRows" :key="`pinned-${pinItem.id}`">
          <div :class="['chat-pinned-zone__row', rowClass(pinItem)]" :data-message-id="pinItem.id">
            <div :class="rowSurfaceClass(pinItem)">
              <chat-item
                :avatar="getMessageAvatar(pinItem)"
                :username="getMessageDisplayName(pinItem)"
                :identity-color="getMessageIdentityColor(pinItem)"
                :content="pinItem.content"
                :item="pinItem"
                :all-message-ids="allMessageIds"
                :editing-preview="editingPreviewMap[pinItem.id]"
                :edit-saving="isSavingEdit"
                :tone="getMessageTone(pinItem)"
                :show-avatar="getMessageAvatarRenderState(pinItem).showAvatar"
                :hide-avatar="false"
                :show-header="true"
                :layout="display.layout"
                :is-self="isSelfMessage(pinItem)"
                :is-merged="false"
                :world-keyword-editable="canManageWorldKeywords"
                @avatar-longpress="avatarLongpress(pinItem)"
                @edit="beginEdit(pinItem)"
                @edit-save="saveEdit"
                @edit-cancel="cancelEditing"
                @reedit-revoked="handleReeditRevokedMessage"
                @retry-send="retrySendMessage"
                @image-layout-edit-state-change="handleImageLayoutEditStateChange"
                @edit-inline-image="handleMessageInlineImageEdit"
                @relocate-target-pick="handleRelocateTargetPick"
              />
            </div>
          </div>
        </template>
      </div>
    </div>

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
                  <div
                    v-if="shouldShowInlineHeader(entry)"
                    class="message-row__name-wrap"
                  >
                    <span
                      class="message-row__name"
                      :style="getMessageIdentityColor(entry.message) ? { color: getMessageIdentityColor(entry.message) } : undefined"
                    >{{ getMessageDisplayName(entry.message) }}</span>
                    <span
                      v-if="shouldRenderSendingIndicator(entry.message) && isSelfMessage(entry.message)"
                      class="message-row__send-status message-row__send-status--sending"
                      aria-label="发送中"
                      title="发送中"
                    >
                      <span class="message-row__send-spinner"></span>
                    </span>
                    <n-tooltip v-else-if="canRetrySendMessage(entry.message)" trigger="hover">
                      <template #trigger>
                        <button
                          type="button"
                          class="message-row__send-status message-row__send-status--failed"
                          aria-label="发送失败，点击重试"
                          @click.stop="retrySendMessage(entry.message)"
                        >!</button>
                      </template>
                      {{ getMessageSendErrorReason(entry.message) }}
                    </n-tooltip>
                  </div>
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
                    :edit-saving="isSavingEdit"
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
                    @reedit-revoked="handleReeditRevokedMessage"
                    @retry-send="retrySendMessage"
                    @image-layout-edit-state-change="handleImageLayoutEditStateChange"
                    @edit-inline-image="handleMessageInlineImageEdit"
                    @relocate-target-pick="handleRelocateTargetPick"
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
                :edit-saving="isSavingEdit"
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
                @reedit-revoked="handleReeditRevokedMessage"
                @retry-send="retrySendMessage"
                @image-layout-edit-state-change="handleImageLayoutEditStateChange"
                @edit-inline-image="handleMessageInlineImageEdit"
                @relocate-target-pick="handleRelocateTargetPick"
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
                :edit-saving="isSavingEdit"
                :tone="getMessageTone(entry.message)"
                :show-avatar="getMessageAvatarRenderState(entry.message, entry.mergedWithPrev).showAvatar"
                :hide-avatar="getMessageAvatarRenderState(entry.message, entry.mergedWithPrev).hideAvatar"
                :show-header="shouldShowInlineHeader(entry)"
                :layout="display.layout"
                :is-self="isSelfMessage(entry.message)"
                :is-merged="entry.mergedWithPrev"
                :world-keyword-editable="canManageWorldKeywords"
                @avatar-longpress="avatarLongpress(entry.message)"
                @edit="beginEdit(entry.message)"
                @edit-save="saveEdit"
                @edit-cancel="cancelEditing"
                @reedit-revoked="handleReeditRevokedMessage"
                @retry-send="retrySendMessage"
                @image-layout-edit-state-change="handleImageLayoutEditStateChange"
                @edit-inline-image="handleMessageInlineImageEdit"
                @relocate-target-pick="handleRelocateTargetPick"
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
            <template v-if="!display.showAvatar && compactInlineGridLayout">
              <div class="typing-preview-content typing-preview-content--grid">
                <div class="message-row__grid typing-preview-grid">
                  <div class="message-row__grid-handle typing-preview-grid__handle">
                    <div
                      :class="typingPreviewHandleClass(preview)"
                      :aria-hidden="!canDragTypingPreview(preview)"
                      tabindex="-1"
                      @pointerdown="onPreviewDragHandlePointerDown($event, preview)"
                    >
                      <span class="message-row__dot" v-for="n in 3" :key="n"></span>
                    </div>
                  </div>
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
              <div
                :class="typingPreviewHandleClass(preview)"
                :aria-hidden="!canDragTypingPreview(preview)"
                tabindex="-1"
                @pointerdown="onPreviewDragHandlePointerDown($event, preview)"
              >
                <span class="message-row__dot" v-for="n in 3" :key="n"></span>
              </div>
              <div class="typing-preview-content">
                <div
                  v-if="getTypingPreviewAvatarRenderState(preview).showAvatar"
                  class="typing-preview-avatar"
                  :class="{ 'typing-preview-avatar--hidden': getTypingPreviewAvatarRenderState(preview).hideAvatar }"
                >
                  <UserAvatarDecoration
                    :border="false"
                    :src="preview.avatar"
                    :decorations="preview.avatarDecorations"
                    :use-text-fallback="Boolean(preview.isTemporary)"
                    :fallback-text="preview.displayName"
                  />
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
      v-if="rows.length === 0 && (!display.settings.showPinnedMessages || pinnedRows.length === 0) && !messageWindow.loadingLatest"
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
          :style="historyNavigationOpacityStyle"
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
          :style="historyNavigationOpacityStyle"
          @click="inHistoryMode ? handleBackToLatest() : toBottom"
        >
          <template #icon>
            <n-icon>
              <ArrowBarToDown />
            </n-icon>
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
            <div class="whisper-panel__title">
              <span>{{ t('inputBox.whisperPanelTitle') }}</span>
              <div class="whisper-panel__toolbar">
                <n-button size="tiny" quaternary :disabled="!filteredWhisperCandidates.length" @click="selectAllFilteredWhisperCandidates">全选</n-button>
                <n-button size="tiny" quaternary :disabled="!filteredWhisperCandidates.length" @click="invertFilteredWhisperCandidates">反选</n-button>
              </div>
            </div>
            <n-input v-if="whisperPickerSource === 'manual'" ref="whisperSearchInputRef"
              v-model:value="whisperQuery" size="small" :placeholder="t('inputBox.whisperSearchPlaceholder')" clearable
              @keydown="handleWhisperKeydown" />
            <div class="whisper-panel__list" @keydown="handleWhisperKeydown">
              <div v-for="(candidate, idx) in filteredWhisperCandidates" :key="candidate.id"
                class="whisper-panel__item"
                :class="{ 'is-active': idx === whisperSelectionIndex || isWhisperTarget(candidate) }"
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
                  <div v-if="candidate.secondaryName" class="whisper-panel__sub">
                    <span class="whisper-panel__sub-label">场内：</span>
                    <span class="whisper-panel__sub-name" :style="resolveWhisperMetaNameStyle(candidate.icColor)">{{ candidate.icDisplayName || '未配置' }}</span>
                    <span class="whisper-panel__sub-sep"> | </span>
                    <span class="whisper-panel__sub-label">场外：</span>
                    <span class="whisper-panel__sub-name" :style="resolveWhisperMetaNameStyle(candidate.oocColor)">{{ candidate.oocDisplayName || '未配置' }}</span>
                    <span class="whisper-panel__sub-sep"> | </span>
                    <span class="whisper-panel__sub-label">用户：</span>
                    <span class="whisper-panel__sub-name" :style="resolveWhisperMetaNameStyle(candidate.userColor)">{{ candidate.userDisplayName }}</span>
                  </div>
                </div>
                <n-checkbox
                  class="whisper-panel__checkbox"
                  :checked="isWhisperTarget(candidate)"
                  @update:checked="(checked) => handleWhisperCandidateChecked(candidate, checked)"
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
        >
          <div
            v-if="!isMobileWideInput"
            class="chat-input-resize-handle"
            aria-hidden="true"
            @pointerdown="handleInputBorderPointerDown"
          />
          <div v-if="activeMessageInsertTarget" class="message-insert-banner">
            <div class="message-insert-banner__main">
              <span class="message-insert-banner__badge">插入到上方</span>
              <span class="message-insert-banner__text">后续新消息将插到：{{ messageInsertHintText }} 上方</span>
            </div>
            <div class="message-insert-banner__actions">
              <n-button size="small" quaternary @click="clearMessageInsertTarget">取消</n-button>
            </div>
          </div>
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
              {{ resolveSelectedWhisperTargetName(target) }}
            </n-tag>
          </div>
          <div class="chat-input-area relative flex-1">
            <div
              v-if="isMinimalInputActive && minimalInputToolbarVisible"
              ref="minimalInputActionsHostRef"
              class="chat-input-inline-toolbar-host"
            />
            <div
              v-if="!isMinimalInputActive"
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
                    :controlled-selection="isEditingCurrentChannel"
                    :selected-identity-id="isEditingCurrentChannel ? (chat.editing?.identityId || null) : null"
                    :selected-identity-variant-id="isEditingCurrentChannel ? (chat.editing?.identityVariantId || null) : null"
                    :preview-appearance="activeIdentityAppearanceForPreview"
                    @create="openIdentityCreate"
                    @edit-temporary="openActiveTemporaryIdentityEdit"
                    @manage="openIdentityManager"
                    @identity-changed="handleIdentitySwitcherChange"
                    @identity-selected="handleEditingIdentitySelected"
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
                          @click="handleEmojiTriggerClick($event)"
                        >
                          <template #icon>
                            <n-icon :component="EmojiTriggerIcon" size="18" />
                          </template>
                        </n-button>
                      </template>
                      打开表情盘
                    </n-tooltip>

                    <n-popover
                      v-model:show="emojiPopoverShow"
                      trigger="manual"
                      placement="bottom-start"
                      :x="emojiPopoverXCoord"
                      :y="emojiPopoverYCoord"
                      @clickoutside="emojiPopoverShow = false"
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

                        <div class="emoji-panel__tabs emoji-panel__tabs--with-utf">
                          <template v-if="hasEmojiItems && hasMultipleTabs">
                            <button
                              class="emoji-panel__tab"
                              :class="{ 'emoji-panel__tab--active': emojiPanelTab === 'gallery' && activeEmojiTab === null }"
                              @click="switchEmojiPanelTab('gallery'); activeEmojiTab = null"
                            >
                              全部
                            </button>
                            <button
                              v-for="tab in emojiTabOptions"
                              :key="tab.id"
                              class="emoji-panel__tab"
                              :class="{ 'emoji-panel__tab--active': emojiPanelTab === 'gallery' && activeEmojiTab === tab.id }"
                              :title="tab.name"
                              @click="switchEmojiPanelTab('gallery'); activeEmojiTab = tab.id"
                            >
                              <span class="emoji-panel__tab-text">{{ tab.name }}</span>
                            </button>
                          </template>
                          <button
                            class="emoji-panel__tab emoji-panel__tab--utf"
                            :class="{ 'emoji-panel__tab--active': emojiPanelTab === 'utf' }"
                            title="UTF 表情"
                            @click="switchEmojiPanelTab('utf')"
                          >
                            <span class="emoji-panel__tab-icon" aria-hidden="true">😊</span>
                            <span class="emoji-panel__tab-text">UTF</span>
                          </button>
                          <n-tooltip trigger="hover">
                            <template #trigger>
                              <button
                                class="emoji-panel__tab emoji-panel__tab--variant"
                                :class="{
                                  'emoji-panel__tab--active': emojiPanelTab === 'variant',
                                  'emoji-panel__tab--muted': !activeIdentityForEmojiPanel || !hasIdentityVariantOptions,
                                }"
                                :aria-disabled="!activeIdentityForEmojiPanel || !hasIdentityVariantOptions"
                                @click="handleEmojiVariantTabClick"
                              >
                                <span class="emoji-panel__tab-icon" aria-hidden="true">🎭</span>
                                <span class="emoji-panel__tab-text">差分</span>
                              </button>
                            </template>
                            {{ identityVariantTabTooltip }}
                          </n-tooltip>
                        </div>

                        <div v-if="(emojiPanelTab === 'gallery' && hasEmojiItems) || emojiPanelTab === 'variant'" class="emoji-panel__search">
                          <n-input
                            v-model:value="emojiSearchQuery"
                            size="small"
                            :placeholder="emojiPanelTab === 'variant' ? '搜索差分关键词或备注...' : '搜索表情...'"
                            clearable
                          />
                        </div>

                        <div v-if="emojiPanelTab === 'gallery' && !hasEmojiItems" class="emoji-panel__empty">
                          当前没有收藏的表情，可以在聊天窗口的图片上<b class="px-1">长按</b>或<b class="px-1">右键</b>添加
                        </div>

                        <div
                          :key="`emoji-panel-${emojiPanelRenderKey}`"
                          ref="emojiPanelContentRef"
                          class="emoji-panel__content"
                          :class="{ 'emoji-panel__content--utf': emojiPanelTab === 'utf' }"
                          @scroll="handleEmojiPanelContentScroll"
                        >
                          <template v-if="emojiPanelTab === 'utf'">
                            <div class="emoji-panel__utf-host">
                              <EmojiPickerModal
                                embedded
                                mode="emoji-only"
                                initial-tab="emoji"
                                @select="handleUtfEmojiSelect"
                              />
                            </div>
                          </template>
                          <template v-else-if="emojiPanelTab === 'variant'">
                            <div v-if="!activeIdentityForEmojiPanel" class="emoji-panel__empty">
                              请先选择频道角色
                            </div>
                            <div v-else-if="!hasIdentityVariantOptions" class="emoji-panel__empty">
                              当前频道角色没有可用的头像差分
                            </div>
                            <div v-else class="identity-variant-picker">
                              <n-tooltip trigger="hover">
                                <template #trigger>
                                  <button
                                    type="button"
                                    class="identity-variant-picker__item identity-variant-picker__item--default"
                                    :class="{ 'is-active': !effectiveIdentityVariantForEmojiPanel }"
                                    @click="handleEmojiVariantSelect('')"
                                  >
                                    <div class="identity-variant-picker__badge">↺</div>
                                    <AvatarVue
                                      :size="40"
                                      :border="false"
                                      :src="resolveAttachmentUrl(activeIdentityForEmojiPanel.avatarAttachmentId) || (activeIdentityForEmojiPanel.isTemporary ? '' : user.info.avatar)"
                                      :use-text-fallback="activeIdentityForEmojiPanel.isTemporary"
                                      :fallback-text="activeIdentityForEmojiPanel.displayName"
                                    />
                                    <div class="identity-variant-picker__title">默认头像</div>
                                    <div class="identity-variant-picker__hint">恢复</div>
                                  </button>
                                </template>
                                {{ describeIdentityVariantCard(null) }}
                              </n-tooltip>
                              <n-tooltip
                                v-for="variant in filteredIdentityVariantOptions"
                                :key="variant.id"
                                trigger="hover"
                              >
                                <template #trigger>
                                  <button
                                    type="button"
                                    class="identity-variant-picker__item"
                                    :class="{ 'is-active': effectiveIdentityVariantForEmojiPanel?.id === variant.id }"
                                    @click="handleEmojiVariantSelect(variant.id)"
                                  >
                                    <div class="identity-variant-picker__badge">
                                      <img
                                        v-if="isVariantSelectorEmojiAttachment(variant.selectorEmoji)"
                                        :src="resolveVariantSelectorEmojiSrc(variant.selectorEmoji)"
                                        :alt="resolveVariantNote(variant)"
                                      />
                                      <span v-else>{{ variant.selectorEmoji || '🙂' }}</span>
                                    </div>
                                    <AvatarVue
                                      :size="40"
                                      :border="false"
                                      :src="resolveAttachmentUrl(variant.avatarAttachmentId || activeIdentityForEmojiPanel.avatarAttachmentId) || (activeIdentityForEmojiPanel.isTemporary ? '' : user.info.avatar)"
                                      :use-text-fallback="activeIdentityForEmojiPanel.isTemporary"
                                      :fallback-text="variant.displayName || activeIdentityForEmojiPanel.displayName"
                                    />
                                    <div class="identity-variant-picker__title">{{ resolveVariantNote(variant) }}</div>
                                    <div class="identity-variant-picker__hint">={{ variant.keyword }}</div>
                                  </button>
                                </template>
                                <span class="identity-variant-picker__tooltip">{{ describeIdentityVariantCard(variant) }}</span>
                              </n-tooltip>
                              <div v-if="emojiSearchQuery && !filteredIdentityVariantOptions.length" class="emoji-panel__empty">
                                没有匹配的头像差分
                              </div>
                            </div>
                          </template>
                          <template v-else>
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
                                @click="handleQuickGalleryEmojiClick(item)"
                              >
                                <img :src="getEmojiItemSrc(item)" :alt="item.remark || '表情'" />
                                <div class="emoji-caption" :title="item.remark || `收藏${idx + 1}`">{{ item.remark || `收藏${idx + 1}` }}</div>
                                <div class="emoji-item__actions">
                                  <n-button text size="tiny" @click.stop="openEmojiRemarkEditor(item)">备注</n-button>
                                </div>
                              </div>
                            </div>
                          </template>
                          </template>
                          <div
                            v-if="emojiPanelTab === 'gallery' && filteredEmojiItems.length"
                            ref="emojiPanelLoadMoreSentinelRef"
                            class="emoji-grid__sentinel"
                            aria-hidden="true"
                          ></div>
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
                      <n-button
                        quaternary
                        circle
                        :type="interjectButtonType"
                        :disabled="!canStartInterject"
                        @click="startInterject"
                      >
                        <template #icon>
                          <n-icon :component="MessagePlus" size="18" />
                        </template>
                      </n-button>
                    </template>
                    {{ interjectTooltip }}
                  </n-tooltip>
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
                      <n-button quaternary circle @click="handleToolbarUploadClick">
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
                <div class="chat-input-actions__cell" v-if="showAIPolish">
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-button quaternary circle :disabled="!canRunAIPolish" @click="runAIPolish">
                        <template #icon>
                          <n-icon :component="Palette" size="18" />
                        </template>
                      </n-button>
                    </template>
                    润色
                  </n-tooltip>
                </div>
                <div class="chat-input-actions__cell" v-if="showDiceTrayTrigger">
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-button class="chat-dice-button" quaternary circle :disabled="(!canUseBuiltInDice && !effectiveBotFeatureEnabled) || diceFeatureUpdating" @click="toggleDiceTray">
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
                </div>
              </div>
            </div>
            <div
              v-if="isMinimalInputActive"
              class="chat-input-editor-row chat-input-editor-row--minimal"
              :class="{ 'chat-input-editor-row--side-stacked': showMinimalStackedSideControls }"
              :style="chatInputStyle"
            >
              <div
                class="chat-input-minimal-side"
                :class="{ 'chat-input-minimal-side--editing-floating': isEditing && !showMinimalStackedSideControls }"
              >
                <div class="chat-input-actions__cell identity-switcher-cell identity-switcher-cell--minimal">
                  <ChannelIdentitySwitcher
                    v-if="chat.curChannel"
                    :controlled-selection="isEditingCurrentChannel"
                    :selected-identity-id="isEditingCurrentChannel ? (chat.editing?.identityId || null) : null"
                    :selected-identity-variant-id="isEditingCurrentChannel ? (chat.editing?.identityVariantId || null) : null"
                    compact
                    icon-only
                    :preview-appearance="activeIdentityAppearanceForPreview"
                    @create="openIdentityCreate"
                    @edit-temporary="openActiveTemporaryIdentityEdit"
                    @manage="openIdentityManager"
                    @identity-changed="handleIdentitySwitcherChange"
                    @identity-selected="handleEditingIdentitySelected"
                    @avatar-setup="handleOpenAvatarPrompt"
                  />
                </div>
                <div
                  v-if="showMinimalStackedSideControls && isEditing"
                  class="chat-input-minimal-side__aux chat-input-minimal-side__aux--editing"
                >
                  <ChatIcOocToggle
                    v-model="inputIcMode"
                    compact
                  />
                </div>
                <div
                  v-else-if="showMinimalStackedSideControls"
                  class="chat-input-minimal-side__aux"
                >
                  <ChatIcOocToggle
                    v-model="inputIcMode"
                    compact
                  />
                </div>
                <div
                  v-else-if="isEditing"
                  class="chat-input-actions__cell chat-input-minimal-side__floating-toggle"
                >
                  <ChatIcOocToggle
                    v-model="inputIcMode"
                    compact
                  />
                </div>
              </div>
              <div class="chat-input-editor-main">
                <KeywordSuggestPanel
                  :visible="keywordSuggestVisible"
                  :options="keywordSuggestOptions"
                  :active-index="keywordSuggestIndex"
                  :loading="keywordSuggestLoading"
                  @select="handleKeywordSuggestSelect"
                  @hover="handleKeywordSuggestHover"
                />
                <div
                  ref="minimalInputMeasureRef"
                  class="chat-input-measure-shell"
                >
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
                    :send-shortcut="customSendShortcutEnabled ? 'ctrlEnter' : display.settings.sendShortcut"
                    :inline-images="inlineImagePreviewMap"
                    :default-i-form-embed-link="defaultIFormEmbedLink"
                    @mention-search="atHandleSearch"
                    @mention-select="handleMentionSelect"
                    @keydown="keyDown"
                    @blur="handleChatInputBlur"
                    @input="handleSlashInput"
                    @paste-image="handlePlainPasteImage"
                    @drop-files="handlePlainDropFiles"
                    @drop-gallery-item="handleDropGalleryItem"
                    @upload-button-click="handleRichUploadButtonClick"
                    @remove-image="removeInlineImage"
                  />
                </div>
              </div>
              <div
                v-if="!isEditing && !hasMeaningfulDraft && !showMinimalStackedSideControls"
                class="chat-input-actions__cell"
              >
                <ChatIcOocToggle
                  v-model="inputIcMode"
                  compact
                />
              </div>
              <div
                v-if="!isEditing"
                class="chat-input-minimal-actions"
                :class="{
                  'chat-input-minimal-actions--stacked': showMinimalStackedSideControls,
                  'chat-input-minimal-actions--draft': hasMeaningfulDraft,
                }"
              >
                <template v-if="showMinimalStackedSideControls">
                  <div
                    v-if="showMinimalWideInputShortcut"
                    class="chat-input-actions__cell chat-input-minimal-actions__slot chat-input-minimal-actions__slot--top"
                  >
                    <n-button
                      quaternary
                      circle
                      class="chat-input-minimal-tool-btn"
                      :class="{ 'chat-input-minimal-tool-btn--active': wideInputMode }"
                      :aria-label="wideInputTooltip"
                      @click="toggleWideInputMode"
                    >
                      <template #icon>
                        <n-icon :component="ArrowsVertical" size="16" />
                      </template>
                    </n-button>
                  </div>
                  <div
                    v-if="hasMeaningfulDraft"
                    class="chat-input-actions__cell chat-input-minimal-actions__slot chat-input-minimal-actions__slot--middle"
                  >
                    <n-button
                      size="medium"
                      @pointerdown="handleSendPointerDown"
                      @pointerup="handleSendPointerUp"
                      @pointercancel="handleSendPointerUp"
                      @mousedown="handleSendMouseDown"
                      @click="handleSendClick"
                      :disabled="spectatorInputDisabled || chat.connectState !== 'connected'"
                      class="send-action-btn send-action-btn--compact"
                    >
                      <template #icon>
                        <n-icon :component="Send" size="18" />
                      </template>
                    </n-button>
                  </div>
                  <div class="chat-input-actions__cell chat-input-minimal-actions__slot chat-input-minimal-actions__slot--bottom">
                    <n-button
                      quaternary
                      circle
                      class="chat-input-minimal-toggle"
                      :class="{ 'chat-input-minimal-toggle--active': minimalInputToolbarVisible }"
                      :aria-label="minimalInputToolbarVisible ? '收起完整工具栏' : '展开完整工具栏'"
                      @click="toggleMinimalInputToolbar"
                    >
                      <template #icon>
                        <n-icon :component="Plus" size="18" />
                      </template>
                    </n-button>
                  </div>
                </template>
                <div
                  v-else
                  class="chat-input-minimal-actions__primary"
                >
                  <div
                    v-if="hasMeaningfulDraft"
                    class="chat-input-actions__cell"
                  >
                    <n-button
                      size="medium"
                      @pointerdown="handleSendPointerDown"
                      @pointerup="handleSendPointerUp"
                      @pointercancel="handleSendPointerUp"
                      @mousedown="handleSendMouseDown"
                      @click="handleSendClick"
                      :disabled="spectatorInputDisabled || chat.connectState !== 'connected'"
                      class="send-action-btn send-action-btn--compact"
                    >
                      <template #icon>
                        <n-icon :component="Send" size="18" />
                      </template>
                    </n-button>
                  </div>
                  <div class="chat-input-actions__cell">
                    <n-button
                      quaternary
                      circle
                      class="chat-input-minimal-toggle"
                      :class="{ 'chat-input-minimal-toggle--active': minimalInputToolbarVisible }"
                      :aria-label="minimalInputToolbarVisible ? '收起完整工具栏' : '展开完整工具栏'"
                      @click="toggleMinimalInputToolbar"
                    >
                      <template #icon>
                        <n-icon :component="Plus" size="18" />
                      </template>
                    </n-button>
                  </div>
                </div>
              </div>
              <div
                v-if="isEditing"
                class="chat-input-actions__cell chat-input-actions__send chat-input-send-inline"
              >
                <div class="edit-actions-group">
                  <n-button size="medium" @click="saveEdit"
                    :disabled="isSavingEdit || spectatorInputDisabled || chat.connectState !== 'connected'"
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
              </div>
            </div>
            <div v-else class="chat-input-editor-row" :style="chatInputStyle">
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
                  :send-shortcut="customSendShortcutEnabled ? 'ctrlEnter' : display.settings.sendShortcut"
                  :inline-images="inlineImagePreviewMap"
                  :default-i-form-embed-link="defaultIFormEmbedLink"
                  @mention-search="atHandleSearch"
                  @mention-select="handleMentionSelect"
                  @keydown="keyDown"
                  @blur="handleChatInputBlur"
                  @input="handleSlashInput"
                  @paste-image="handlePlainPasteImage"
                  @drop-files="handlePlainDropFiles"
                  @drop-gallery-item="handleDropGalleryItem"
                  @upload-button-click="handleRichUploadButtonClick"
                  @remove-image="removeInlineImage"
                />
              </div>
              <div class="chat-input-actions__cell chat-input-actions__send chat-input-send-inline">
                <template v-if="isEditing">
                  <div class="edit-actions-group">
                    <n-button size="medium" @click="saveEdit"
                      :disabled="isSavingEdit || spectatorInputDisabled || chat.connectState !== 'connected'"
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
                  <n-button
                    size="medium"
                    @pointerdown="handleSendPointerDown"
                    @pointerup="handleSendPointerUp"
                    @pointercancel="handleSendPointerUp"
                    @mousedown="handleSendMouseDown"
                    @click="handleSendClick"
                    :disabled="spectatorInputDisabled || chat.connectState !== 'connected'"
                    class="send-action-btn">
                    <template #icon>
                      <n-icon :component="Send" size="18" />
                    </template>
                  </n-button>
                </template>
              </div>
            </div>
            <input
              ref="inlineImageInputRef"
              class="hidden"
              type="file"
              accept="image/*"
              multiple
              @change="handleInlineFileChange"
            />
        </div>
      </div>
    </div>
  </div>
  </div>

  <RightClickMenu />
  <AvatarClickMenu />
  <MessageImageEditor
    :show="richInlineImageEditorVisible"
    :file="richInlineImageEditorFile"
    @update:show="value => { if (!value) closeRichInlineImageEditor(); }"
    @cancel="closeRichInlineImageEditor"
    @confirm="handleRichInlineImageEditorConfirm"
  />
  <MultiSelectFloatingBar
    @copy="handleMultiSelectCopy"
    @forward="handleMultiSelectForward"
    @archive="handleMultiSelectArchive"
    @delete="handleMultiSelectDelete"
    @copy-image="handleMultiSelectCopyImage"
    @move-to-bottom="handleMultiSelectMoveToBottom"
    @relocate="handleMultiSelectRelocate"
    @select-all="handleMultiSelectAll"
    @cancel-relocate="handleCancelMultiSelectRelocate"
  />
  <MessageForwardDialog
    v-model:visible="forwardDialogVisible"
    :source-channel-id="forwardDialogSourceChannelId"
    :source-world-id="forwardDialogSourceWorldId"
    :message-ids="forwardDialogMessageIds"
    :messages="forwardDialogMessages"
    @success="handleMessageForwardSuccess"
  />
  <GalleryPanel @insert="handleGalleryInsert" />
  <CharacterCardPanel ref="characterCardPanelRef" v-model:visible="characterCardPanelVisible" :channel-id="chat.curChannel?.id" />
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
    :auto-focus="false"
    class="identity-dialog"
  >
    <template #header>
      <div class="identity-dialog__header">
        <span class="identity-dialog__header-title">{{ identityDialogTitle }}</span>
        <n-tooltip v-if="identityTemporaryHint" trigger="hover">
          <template #trigger>
            <button
              type="button"
              class="identity-dialog__header-help"
              aria-label="查看临时身份说明"
            >
              ？
            </button>
          </template>
          {{ identityTemporaryHint }}
        </n-tooltip>
      </div>
    </template>
    <n-form label-width="90px" label-placement="left" class="identity-dialog__form">
      <n-form-item v-if="isManagingBotIdentity" label="全局资料">
        <div class="flex flex-col gap-1">
          <n-switch
            :value="identityForm.botAppearanceMode !== 'custom'"
            @update:value="identityForm.botAppearanceMode = $event ? 'inherit' : 'custom'"
          >
            <template #checked>跟随 BOT 全局资料</template>
            <template #unchecked>使用频道自定义资料</template>
          </n-switch>
          <n-text depth="3">仅控制昵称、颜色、头像；头像装饰与小剧场演出始终按当前频道保存。</n-text>
        </div>
      </n-form-item>
      <n-form-item label="频道昵称">
        <n-input v-model:value="identityForm.displayName" :disabled="sharedSynchronizedFieldsDisabled" maxlength="32" show-count placeholder="请输入频道内显示的昵称" />
      </n-form-item>
      <n-form-item label="昵称颜色">
        <div class="identity-color-field">
          <n-color-picker
            :value="identityForm.color"
            :modes="['hex']"
            :show-alpha="false"
            size="small"
            class="identity-color-picker"
            :disabled="sharedSynchronizedFieldsDisabled"
            @update:value="handleIdentityColorPickerUpdate"
          />
          <n-input
            v-model:value="identityColorDraft"
            size="small"
            placeholder="#RRGGBB"
            class="identity-color-input"
            :disabled="sharedSynchronizedFieldsDisabled"
            @blur="handleIdentityColorBlur"
            @keyup.enter="handleIdentityColorBlur"
          />
          <n-button tertiary size="small" :disabled="sharedSynchronizedFieldsDisabled" @click="clearIdentityColor">清除</n-button>
        </div>
      </n-form-item>
      <n-form-item label="频道头像">
        <div class="identity-avatar-field">
          <AvatarVue
            :size="48"
            :border="false"
            :src="identityAvatarDisplay || (identityForm.isTemporary ? '' : managedIdentityFallbackAvatar)"
            :use-text-fallback="identityForm.isTemporary"
            :fallback-text="identityForm.displayName"
          />
          <n-space>
            <n-button size="small" type="primary" :disabled="sharedSynchronizedFieldsDisabled" @click="handleIdentityAvatarTrigger">上传头像</n-button>
            <n-button v-if="identityForm.avatarAttachmentId" size="small" tertiary :disabled="sharedSynchronizedFieldsDisabled" @click="removeIdentityAvatar">移除</n-button>
          </n-space>
        </div>
      </n-form-item>
      <n-form-item label="头像装饰">
        <div class="flex flex-col gap-2">
          <n-space align="center">
            <n-button size="small" type="primary" secondary :disabled="isDelegatedSharedIdentity" @click="openIdentityDecorationEditor">编辑装饰</n-button>
            <n-tag v-if="identityForm.avatarDecorations.some(item => item.enabled && item.resourceAttachmentId)" size="small" type="success">
              已配置
            </n-tag>
            <n-text v-else depth="3">未配置</n-text>
          </n-space>
          <n-text depth="3">
            {{ editingIdentity?.sharedIdentityId || identityForm.promoteToShared
              ? '会同步到此角色的其他频道副本，并且只显示在频道消息头像上。'
              : '仅对当前频道角色生效，并且只会显示在频道消息头像上。' }}
          </n-text>
        </div>
      </n-form-item>
      <n-form-item label="小剧场演出">
        <div class="flex flex-col gap-2">
          <n-space align="center">
            <n-button size="small" type="primary" secondary :disabled="isDelegatedSharedIdentity" @click="openIdentityTheaterPresentationEditor">编辑演出外观</n-button>
            <n-tag v-if="identityForm.theaterPresentation" size="small" type="success">已配置</n-tag>
            <n-text v-else depth="3">未配置</n-text>
          </n-space>
          <n-text depth="3">仅用于小剧场消息演出，不影响频道消息头像。</n-text>
        </div>
      </n-form-item>
      <n-alert v-if="isDelegatedSharedIdentity" type="info" :show-icon="false">
        此角色由本人跨频道共享。管理员可维护会同步到全部副本的头像差分；昵称、颜色、头像、头像装饰及基础小剧场演出只读。当前频道默认身份、文件夹、IC/OOC 映射和人物卡保持独立。
      </n-alert>
      <n-form-item v-if="!isEditingTemporaryIdentity && !isManagingBotIdentity" label="绑定人物卡">
        <n-select
          v-model:value="identityForm.characterCardId"
          :options="characterCardSelectOptions"
          placeholder="选择要绑定的人物卡"
          clearable
        />
      </n-form-item>
      <n-form-item v-if="!isEditingTemporaryIdentity && !isManagingBotIdentity" class="identity-dialog__check-item">
        <n-checkbox v-model:checked="identityForm.isDefault">
          设为频道默认身份
        </n-checkbox>
      </n-form-item>
      <n-form-item
        v-if="identityDialogMode === 'edit' && !isEditingTemporaryIdentity && !isManagingBotIdentity && !isManagingOtherUserIdentity"
        label="跨频道角色"
      >
        <div class="flex flex-col gap-1">
          <n-tag v-if="editingIdentity?.sharedIdentityId" type="success" size="small">已启用跨频道同步</n-tag>
          <n-checkbox
            v-else-if="canPromoteEditingIdentityToShared"
            :checked="identityForm.promoteToShared"
            @update:checked="handlePromoteIdentityToSharedUpdate"
          >
            提升为跨频道角色
            <n-tag size="small" type="warning">实验性</n-tag>
          </n-checkbox>
        </div>
      </n-form-item>
      <n-form-item v-if="!isManagingBotIdentity && (identityForm.isTemporary || isEditingTemporaryIdentity)" label="切换到此角色时">
        <div class="identity-mini-mode-switch">
          <n-button-group size="small">
            <n-button
              :type="identityForm.icOocOnActivate !== 'ooc' ? 'primary' : 'default'"
              @click="setTemporaryIdentityActivateMode('ic')"
            >
              场内
            </n-button>
            <n-button
              :type="identityForm.icOocOnActivate === 'ooc' ? 'primary' : 'default'"
              @click="setTemporaryIdentityActivateMode('ooc')"
            >
              场外
            </n-button>
          </n-button-group>
          <span class="identity-mini-mode-switch__hint">切换到这个临时角色时，自动切到{{ temporaryIdentityActivateModeLabel }}</span>
        </div>
      </n-form-item>
      <n-form-item v-if="identityDialogMode === 'create' && !isManagingBotIdentity" class="identity-dialog__check-item">
        <n-checkbox v-model:checked="identityForm.isTemporary">
          创建为临时 NPC 角色
        </n-checkbox>
      </n-form-item>
      <template v-if="!isEditingTemporaryIdentity">
        <n-divider title-placement="left" class="identity-dialog__variant-divider">头像差分</n-divider>
        <div v-if="identityDialogMode === 'edit' && editingIdentity" class="identity-variant-section">
          <div class="identity-variant-section__header">
            <div>
              <div class="identity-variant-section__title">{{ editingIdentity.sharedIdentityId ? '配置跨频道同步的头像差分' : '为当前频道角色配置头像差分' }}</div>
              <div class="identity-variant-section__hint">
                {{ isManagingBotIdentity ? 'BOT 发送消息时由后端按匹配规则自动选择差分或恢复默认头像' : '可通过已配置的匹配规则切换差分或恢复默认头像' }}
              </div>
            </div>
            <div class="identity-variant-section__actions">
              <n-button size="small" @click="openIdentityVariantResetConfig">恢复默认头像</n-button>
              <n-button size="small" type="primary" @click="openIdentityVariantCreate">新增差分</n-button>
            </div>
          </div>
          <div v-if="currentEditingIdentityVariants.length" class="identity-variant-list">
            <div
              v-for="variant in currentEditingIdentityVariants"
              :key="variant.id"
              class="identity-variant-list__item"
            >
              <button
                type="button"
                class="identity-variant-list__selector"
                @click="openIdentityVariantEdit(variant)"
              >
                <img
                  v-if="isVariantSelectorEmojiAttachment(variant.selectorEmoji)"
                  :src="resolveVariantSelectorEmojiSrc(variant.selectorEmoji)"
                  :alt="resolveVariantNote(variant)"
                />
                <span v-else>{{ variant.selectorEmoji || '🙂' }}</span>
              </button>
              <AvatarVue
                :size="40"
                :border="false"
                :src="resolveAttachmentUrl(variant.avatarAttachmentId || identityForm.avatarAttachmentId) || (identityForm.isTemporary ? '' : managedIdentityFallbackAvatar)"
                :use-text-fallback="identityForm.isTemporary"
                :fallback-text="variant.displayName || identityForm.displayName"
              />
              <div class="identity-variant-list__meta">
                <div class="identity-variant-list__name-row">
                  <span class="identity-variant-list__name">{{ resolveVariantNote(variant) }}</span>
                  <n-tag size="small" type="info">={{ variant.keyword }}</n-tag>
                  <n-tag v-if="variant.enabled === false" size="small" type="warning">停用</n-tag>
                </div>
                <div class="identity-variant-list__sub">
                  <span v-if="variant.displayName">覆盖昵称：{{ variant.displayName }}</span>
                  <span v-else>仅覆盖头像</span>
                  <span v-if="variant.color">颜色：{{ variant.color }}</span>
                </div>
              </div>
              <div class="identity-variant-list__actions">
                <n-button text size="small" @click="openIdentityVariantEdit(variant)">编辑</n-button>
                <n-button text size="small" type="error" @click="deleteIdentityVariant(variant)">删除</n-button>
              </div>
            </div>
          </div>
          <n-empty v-else description="当前角色还没有头像差分">
            <template #extra>
              <n-button size="small" type="primary" @click="openIdentityVariantCreate">创建首个差分</n-button>
            </template>
          </n-empty>
        </div>
        <n-alert v-else type="info" :show-icon="false">
          请先保存频道角色，随后即可继续配置头像差分。
        </n-alert>
      </template>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="closeIdentityDialog">取消</n-button>
        <n-button type="primary" :loading="identitySubmitting" @click="submitIdentityForm">{{ identityDialogSubmitText }}</n-button>
      </n-space>
    </template>
  </n-modal>
  <n-modal
    :show="identityDecorationEditorVisible"
    @update:show="handleIdentityDecorationEditorShow"
    preset="card"
    title="编辑频道角色头像装饰"
    style="max-width: 760px;"
    :auto-focus="false"
  >
    <AvatarDecorationEditor
      v-model="identityForm.avatarDecorations"
      :avatar-src="identityAvatarDisplay || (identityForm.isTemporary ? '' : managedIdentityFallbackAvatar)"
      :fallback-text="identityForm.displayName"
      :preview-name="identityForm.displayName || '频道角色预览'"
      :upload-channel-id="chat.curChannel?.id"
    />
    <template #footer>
      <n-space justify="end">
        <n-button :loading="identitySubmitting" @click="handleIdentityDecorationEditorShow(false)">完成</n-button>
      </n-space>
    </template>
  </n-modal>
  <n-modal
    v-model:show="identityVariantResetDialogVisible"
    preset="card"
    title="恢复默认规则"
    :auto-focus="false"
    class="identity-variant-dialog"
  >
    <n-form label-width="90px" label-placement="left">
      <n-form-item>
        <template #label>
          <span class="identity-variant-match-label">
            匹配式
            <n-popover trigger="hover" placement="right-start" :width="260">
              <template #trigger>
                <button type="button" class="identity-variant-match-help" aria-label="匹配式说明">?</button>
              </template>
              <div class="identity-variant-match-help-content">
                <div><strong>前缀匹配：</strong>消息以指定符号和匹配内容开头时恢复默认头像，并移除匹配头。</div>
                <div><strong>关键词匹配：</strong>消息包含关键词时恢复；| 表示“或”，&amp; 表示“与”。</div>
                <div><strong>正则表达式匹配：</strong>使用正则规则匹配消息内容；无效规则不会触发。</div>
              </div>
            </n-popover>
          </span>
        </template>
        <n-select v-model:value="identityVariantResetForm.matchMode" :options="identityVariantMatchModeOptions" />
      </n-form-item>
      <n-form-item>
        <div class="identity-variant-match-grid">
          <label class="identity-variant-match-field">
            <span>{{ identityVariantResetForm.matchMode === 'prefix' ? '前缀符号' : identityVariantResetForm.matchMode === 'keyword' ? '关键词匹配类型' : '正则表达式匹配式' }}</span>
            <n-input
              v-if="identityVariantResetForm.matchMode === 'prefix'"
              v-model:value="activeIdentityVariantResetMatchDraft.config"
              maxlength="8"
              placeholder="="
            />
            <n-select
              v-else-if="identityVariantResetForm.matchMode === 'keyword'"
              v-model:value="activeIdentityVariantResetMatchDraft.config"
              :options="identityVariantKeywordMatchOptions"
            />
            <n-select
              v-else
              v-model:value="activeIdentityVariantResetMatchDraft.config"
              :options="identityVariantRegexMatchOptions"
            />
          </label>
          <label class="identity-variant-match-field">
            <span>匹配内容</span>
            <n-input
              v-model:value="activeIdentityVariantResetMatchDraft.content"
              maxlength="255"
              :placeholder="identityVariantResetMatchContentPlaceholder"
            />
          </label>
        </div>
      </n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="identityVariantResetDialogVisible = false">取消</n-button>
        <n-button type="primary" :loading="identitySubmitting" @click="applyIdentityVariantResetConfig">确定</n-button>
      </n-space>
    </template>
  </n-modal>
  <n-modal
    v-model:show="identityVariantDialogVisible"
    preset="card"
    :title="identityVariantDialogMode === 'create' ? '新增头像差分' : '编辑头像差分'"
    :auto-focus="false"
    class="identity-variant-dialog"
  >
    <n-form label-width="90px" label-placement="left">
      <n-form-item label="切换表情">
        <div class="identity-variant-selector-field">
          <button
            type="button"
            class="identity-variant-selector-field__preview"
            @click="identityVariantEmojiPickerVisible = true"
          >
            <img
              v-if="isVariantSelectorEmojiAttachment(identityVariantForm.selectorEmoji)"
              :src="resolveVariantSelectorEmojiSrc(identityVariantForm.selectorEmoji)"
              alt="差分表情"
            />
            <span v-else>{{ identityVariantForm.selectorEmoji || '🙂' }}</span>
          </button>
          <n-space>
            <n-button size="small" type="primary" @click="identityVariantEmojiPickerVisible = true">选择表情</n-button>
            <n-button
              v-if="identityVariantForm.selectorEmoji"
              size="small"
              tertiary
              @click="identityVariantForm.selectorEmoji = ''"
            >
              清除
            </n-button>
          </n-space>
        </div>
      </n-form-item>
      <n-form-item>
        <template #label>
          <span class="identity-variant-match-label">
            匹配式
            <n-popover trigger="hover" placement="right-start" :width="260">
              <template #trigger>
                <button type="button" class="identity-variant-match-help" aria-label="匹配式说明">?</button>
              </template>
              <div class="identity-variant-match-help-content">
                <div><strong>前缀匹配：</strong>消息以指定符号和关键词开头时触发，例如 =笑。</div>
                <div><strong>关键词匹配：</strong>消息包含关键词时触发；| 表示“或”，&amp; 表示“与”。</div>
                <div><strong>正则表达式匹配：</strong>使用正则规则匹配消息内容；无效规则不会触发。</div>
              </div>
            </n-popover>
          </span>
        </template>
        <n-select v-model:value="identityVariantForm.matchMode" :options="identityVariantMatchModeOptions" />
      </n-form-item>
      <n-form-item>
        <div class="identity-variant-match-grid">
          <label class="identity-variant-match-field">
            <span>{{ identityVariantForm.matchMode === 'prefix' ? '前缀符号' : identityVariantForm.matchMode === 'keyword' ? '关键词匹配类型' : '正则表达式匹配式' }}</span>
            <n-input
              v-if="identityVariantForm.matchMode === 'prefix'"
              v-model:value="activeIdentityVariantMatchDraft.config"
              maxlength="8"
              placeholder="="
            />
            <n-select
              v-else-if="identityVariantForm.matchMode === 'keyword'"
              v-model:value="activeIdentityVariantMatchDraft.config"
              :options="identityVariantKeywordMatchOptions"
            />
            <n-select
              v-else
              v-model:value="activeIdentityVariantMatchDraft.config"
              :options="identityVariantRegexMatchOptions"
            />
          </label>
          <label class="identity-variant-match-field">
            <span>匹配内容</span>
            <n-input
              v-model:value="activeIdentityVariantMatchDraft.content"
              maxlength="255"
              :placeholder="identityVariantMatchContentPlaceholder"
            />
          </label>
        </div>
      </n-form-item>
      <n-form-item label="备注">
        <n-input
          v-model:value="identityVariantForm.note"
          maxlength="255"
          placeholder="例如 战斗中 / 严肃 / 开心"
        />
      </n-form-item>
      <n-form-item label="差分头像">
        <div class="identity-avatar-field">
          <AvatarVue
            :size="48"
            :border="false"
            :src="identityVariantAvatarPreview || resolveAttachmentUrl(identityVariantForm.avatarAttachmentId) || identityAvatarDisplay || (identityForm.isTemporary ? '' : managedIdentityFallbackAvatar)"
            :use-text-fallback="identityForm.isTemporary"
            :fallback-text="identityVariantForm.displayName || identityForm.displayName"
          />
          <n-space>
            <n-button size="small" type="primary" @click="handleIdentityVariantAvatarTrigger">上传头像</n-button>
            <n-button
              v-if="identityVariantForm.avatarAttachmentId || identityVariantAvatarPreview"
              size="small"
              tertiary
              @click="removeIdentityVariantAvatar"
            >
              移除
            </n-button>
          </n-space>
        </div>
      </n-form-item>
      <n-form-item label="覆盖昵称">
        <n-input
          v-model:value="identityVariantForm.displayName"
          maxlength="32"
          placeholder="留空则沿用频道角色昵称"
        />
      </n-form-item>
      <n-form-item label="覆盖颜色">
        <div class="identity-color-field">
          <n-color-picker
            :value="identityVariantForm.color"
            :modes="['hex']"
            :show-alpha="false"
            size="small"
            class="identity-color-picker"
            @update:value="handleIdentityVariantColorPickerUpdate"
          />
          <n-input
            v-model:value="identityVariantColorDraft"
            size="small"
            placeholder="#RRGGBB"
            class="identity-color-input"
            @blur="handleIdentityVariantColorBlur"
            @keyup.enter="handleIdentityVariantColorBlur"
          />
          <n-button tertiary size="small" @click="clearIdentityVariantColor">清除</n-button>
        </div>
      </n-form-item>
      <n-form-item label="小剧场演出">
        <div class="flex flex-col gap-2">
          <n-space align="center">
            <n-button size="small" type="primary" secondary @click="openIdentityVariantTheaterPresentationEditor">编辑差分演出</n-button>
            <n-tag v-if="Object.keys(identityVariantForm.theaterPresentation).length" size="small" type="success">已设置差分</n-tag>
            <n-text v-else depth="3">全部继承</n-text>
          </n-space>
          <n-text depth="3">各部分可独立选择继承、自定义或清除。</n-text>
        </div>
      </n-form-item>
      <n-form-item>
        <n-checkbox v-model:checked="identityVariantForm.enabled">
          启用该差分
        </n-checkbox>
      </n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="closeIdentityVariantDialog">取消</n-button>
        <n-button type="primary" :loading="identityVariantSubmitting" @click="submitIdentityVariantForm">保存</n-button>
      </n-space>
    </template>
  </n-modal>
  <TheaterPresentationEditorModal
    v-model:show="theaterPresentationEditorVisible"
    :mode="theaterPresentationEditorMode"
    :presentation="identityForm.theaterPresentation"
    :base="identityForm.theaterPresentation"
    :patch="identityVariantForm.theaterPresentation"
    :channel-id="chat.curChannel?.id || ''"
    :identity-id="editingIdentity?.id || ''"
    :variant-id="theaterPresentationEditorMode === 'variant' ? (editingIdentityVariant?.id || '') : ''"
    :target-user-id="currentIdentityTargetUserId"
    :preview-name="identityForm.displayName || '角色名'"
    :world-template="currentWorldTheaterTemplate"
    :can-set-world-template="canSetWorldTheaterTemplate && !isManagingBotIdentity"
    :world-template-saving="worldTheaterTemplateSaving"
    :applying="theaterPresentationApplying"
    @apply="handleTheaterPresentationApply"
    @set-world-template="handleSetWorldTheaterTemplate"
  />
  <EmojiPickerModal
    v-if="identityVariantEmojiPickerVisible"
    mode="all"
    initial-tab="emoji"
    @select="handleIdentityVariantSelectorEmoji"
    @close="identityVariantEmojiPickerVisible = false"
  />
  <input ref="identityAvatarInputRef" class="hidden" type="file" accept="image/*" @change="handleIdentityAvatarChange">
  <input ref="identityVariantAvatarInputRef" class="hidden" type="file" accept="image/*" @change="handleIdentityVariantAvatarChange">
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
  <n-modal
    v-model:show="identityVariantAvatarEditorVisible"
    preset="card"
    title="编辑差分头像"
    style="max-width: 450px;"
    :mask-closable="false"
  >
    <AvatarEditor
      :file="identityVariantAvatarEditorFile"
      @save="handleIdentityVariantAvatarEditorSave"
      @cancel="handleIdentityVariantAvatarEditorCancel"
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
              <div class="identity-drawer__subtitle">
                <template v-if="isManagingOtherUserIdentity">
                  当前管理：{{ currentManagedIdentityLabel }}
                  <span v-if="identityManageTargetRoleLabel">（{{ identityManageTargetRoleLabel }}）</span>
                </template>
                <template v-else>
                  支持导入/导出，便于跨频道迁移
                </template>
              </div>
            </div>
          </div>
          <n-space>
            <n-tooltip v-if="!isManagingBotIdentity" trigger="hover">
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
            <n-tooltip v-if="!isManagingBotIdentity" trigger="hover">
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
              v-if="canManageChannelFeatures && !isManagingBotIdentity"
              text
              size="small"
              @click="openIdentityManageBotDialog"
            >
              <template #icon>
                <n-icon :component="Settings" size="14" />
              </template>
              设置 BOT
            </n-button>
            <n-button
              v-if="!isManagingBotIdentity"
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
              v-if="!isManagingBotIdentity"
              text
              size="small"
              :disabled="identitySyncing"
              @click="openIdentitySyncDialog"
            >
              <template #icon>
                <n-icon :component="ArrowsVertical" size="14" />
              </template>
              从其他频道同步
            </n-button>
            <n-button
              v-if="canManageOtherUserIdentities && !isManagingBotIdentity"
              text
              size="small"
              @click="openIdentityManageUserDialog"
            >
              <template #icon>
                <n-icon :component="SearchIcon" size="14" />
              </template>
              管理其他用户
            </n-button>
            <n-button
              v-if="isManagingOtherUserIdentity"
              text
              size="small"
              type="warning"
              @click="exitIdentityManageUserMode"
            >
              {{ isManagingBotIdentity ? '退出 BOT 设置' : '退出代管' }}
            </n-button>
          </n-space>
        </div>
      </template>
      <div v-if="currentChannelIdentities.length || identityFolders.length" class="identity-manager" :class="{ 'identity-manager--bot': isManagingBotIdentity }">
        <div v-if="!isManagingBotIdentity" class="identity-manager__sidebar">
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
          <div v-if="!isManagingBotIdentity" class="identity-manager__toolbar">
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
                v-if="!isManagingBotIdentity"
                class="identity-list__item-check"
                :checked="identitySelection.includes(identity.id)"
                @update:checked="val => handleIdentitySelection(identity.id, val)"
              />
              <AvatarVue
                :size="40"
                :border="false"
                :src="resolveAttachmentUrl(identity.avatarAttachmentId) || (identity.isTemporary ? '' : managedIdentityFallbackAvatar)"
                :use-text-fallback="identity.isTemporary"
                :fallback-text="identity.displayName"
              />
              <div class="identity-list__meta">
                <div class="identity-list__name">
                  <span v-if="identity.color" class="identity-list__color" :style="{ backgroundColor: identity.color }"></span>
                  <span
                    class="identity-list__display-name"
                    :style="identity.color ? { color: identity.color } : undefined"
                  >
                    {{ identity.displayName }}
                  </span>
                  <n-tag size="small" type="info" v-if="identity.isDefault">默认</n-tag>
                  <n-tag size="small" type="warning" v-if="identity.isTemporary">临时</n-tag>
                </div>
                <div class="identity-list__hint">ID：{{ identity.id }}</div>
                <div class="identity-list__hint">差分：{{ chat.getIdentityVariants(chat.curChannel?.id || '', identity.id, currentIdentityTargetUserId).length }} 个</div>
                <div v-if="!isManagingBotIdentity" class="identity-list__folders">
                  <n-tag size="small" type="success" v-if="identity.sharedIdentityId">跨频道</n-tag>
                  <n-tag size="small" v-if="!(identity.folderIds?.length)">未分组</n-tag>
                  <n-tag v-for="folderId in identity.folderIds" :key="folderId" size="small" type="info">{{ resolveFolderName(folderId) }}</n-tag>
                </div>
              </div>
              <div class="identity-list__actions">
                <n-button text size="small" @click="openIdentityEdit(identity)">编辑</n-button>
                <n-button v-if="!isManagingBotIdentity" text size="small" type="error" :disabled="currentChannelIdentities.length === 1 || (isManagingOtherUserIdentity && Boolean(identity.sharedIdentityId))" @click="deleteIdentity(identity)">删除</n-button>
              </div>
            </div>
          </div>
          <n-empty v-else description="该分组暂无角色">
            <template v-if="!isManagingBotIdentity" #extra>
              <n-button size="small" type="primary" @click="openIdentityCreate">创建新角色</n-button>
            </template>
          </n-empty>
        </div>
      </div>
      <n-empty v-else description="暂无频道角色">
        <template v-if="!isManagingBotIdentity" #extra>
          <n-button size="small" type="primary" @click="openIdentityCreate">创建新角色</n-button>
        </template>
      </n-empty>
      <template v-if="!isManagingBotIdentity" #footer>
        <n-button type="primary" block @click="openIdentityCreate">创建新角色</n-button>
      </template>
    </n-drawer-content>
  </n-drawer>
  <n-modal
    v-model:show="identityManageCandidateModalVisible"
    preset="card"
    title="选择要管理的用户"
    :style="{ width: 'min(560px, 92vw)' }"
  >
    <div class="space-y-3">
      <n-input
        v-model:value="identityManageCandidateKeyword"
        clearable
        placeholder="搜索用户ID / 用户名 / 昵称"
      />
      <n-spin :show="identityManageCandidatesLoading">
        <div class="space-y-2" style="max-height: 360px; overflow: auto;">
          <div
            v-for="item in identityManageCandidates"
            :key="item.userId"
            class="identity-manage-candidate"
            :class="{ 'is-active': identityManageCandidateSelectedUserId === item.userId }"
            @click="identityManageCandidateSelectedUserId = item.userId"
          >
            <AvatarVue :size="36" :border="false" :src="resolveAttachmentUrl(item.avatar) || ''" :fallback-text="item.nickname || item.username || item.userId" />
            <div class="identity-manage-candidate__meta">
              <div class="identity-manage-candidate__name">
                {{ item.nickname || item.username || item.userId }}
                <n-tag size="small" type="info">{{ item.roleLabel }}</n-tag>
                <n-tag v-if="item.isSelf" size="small">自己</n-tag>
              </div>
              <div class="identity-manage-candidate__sub">
                {{ item.username || item.userId }}
              </div>
            </div>
            <n-radio :checked="identityManageCandidateSelectedUserId === item.userId" />
          </div>
          <n-empty v-if="!identityManageCandidatesLoading && !identityManageCandidates.length" description="没有可管理的用户" />
        </div>
      </n-spin>
      <n-space justify="end">
        <n-button @click="identityManageCandidateModalVisible = false">取消</n-button>
        <n-button type="primary" :disabled="!identityManageCandidateSelectedUserId" @click="confirmIdentityManageUser">确认</n-button>
      </n-space>
    </div>
  </n-modal>
  <n-modal
    v-model:show="identityManageBotModalVisible"
    preset="card"
    title="设置 BOT"
    :style="{ width: 'min(560px, 92vw)' }"
  >
    <n-spin :show="identityManageBotsLoading">
      <div class="space-y-2" style="max-height: 360px; overflow: auto;">
        <div
          v-for="item in identityManageBots"
          :key="item.id"
          class="identity-manage-candidate"
          :class="{ 'is-active': identityManageBotSelectedUserId === item.id }"
          @click="identityManageBotSelectedUserId = item.id"
        >
          <AvatarVue :size="40" :border="false" :src="resolveAttachmentUrl(item.avatar) || ''" :fallback-text="item.nick || item.username || item.id" />
          <div class="identity-manage-candidate__meta">
            <div class="identity-manage-candidate__name">
              {{ item.nick || item.username || item.id }}
              <n-tag size="small" type="info">BOT</n-tag>
            </div>
            <div class="identity-manage-candidate__sub">{{ item.username || item.id }}</div>
          </div>
          <n-radio :checked="identityManageBotSelectedUserId === item.id" />
        </div>
        <n-empty v-if="!identityManageBotsLoading && !identityManageBots.length" description="当前频道未绑定 BOT" />
      </div>
    </n-spin>
    <template #footer>
      <n-space justify="end">
        <n-button @click="identityManageBotModalVisible = false">取消</n-button>
        <n-button type="primary" :disabled="!identityManageBotSelectedUserId" @click="confirmIdentityManageBot">确认</n-button>
      </n-space>
    </template>
  </n-modal>
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
    title="从其他频道同步角色"
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
  <IcOocRoleConfigPanel
    v-model:show="icOocRoleConfigPanelVisible"
    :target-user-id="currentIdentityTargetUserId"
  />

  <!-- 新增组件 -->
  <ArchiveDrawer
    v-model:visible="archiveDrawerVisible"
    :messages="archivedMessages"
    :loading="archivedLoading"
    :page="archivedCurrentPage"
    :page-count="archivedPageCount"
    :total="archivedTotalCount"
    :search-query="archivedSearchQuery"
    :has-more="archivedHasMore"
    @update:page="handleArchivePageChange"
    @update:search="handleArchiveSearchChange"
    @unarchive="handleUnarchiveMessages"
    @delete="handleDeleteArchivedMessages"
    @load-more="() => fetchArchivedMessages(false)"
    @refresh="fetchArchivedMessages"
  />

  <ChatSearchPanel @jump-to-message="handleSearchJump" />

  <ExportManagerModal
    v-model:visible="exportManagerVisible"
    :channel-id="chat.curChannel?.id"
    :refresh-version="exportManagerRefreshVersion"
    :reveal-latest-task-version="exportManagerRevealVersion"
    @request-export="exportDialogBatchMode = false; exportDialogVisible = true"
    @request-batch-export="exportDialogBatchMode = true; exportDialogVisible = true"
  />
  <ExportDialog
    v-model:visible="exportDialogVisible"
    :channel-id="chat.curChannel?.id"
    :batch-mode="exportDialogBatchMode"
    :battle-summary-enabled="showBattleSummary"
    @export="handleExportMessages"
    @request-battle-summary="openBattleSummary"
  />
  <BattleReportDrawer
    v-model:visible="battleReportDrawerVisible"
    :channel-id="chat.curChannel?.id"
    :world-id="chat.currentWorldId"
    :readonly="chat.isObserver"
    :observer-mode="chat.observerMode"
  />
  <ChatAiPolishDock
    :visible="aiPolishDockVisible"
    :favicon-href="aiPolishFaviconHref"
    :dock-state="aiPolishDockState"
    @restore="toggleAIPolishDockMinimized(aiPolishDockState, false)"
    @toggle-minimize="toggleAIPolishDockMinimized(aiPolishDockState)"
    @select-slot="setActiveAIPolishSlot(aiPolishDockState, $event)"
    @read-current-input="readCurrentInputIntoAIPolishSlot"
    @retry="retryCurrentAIPolishTask"
    @apply="applyAIPolishResult"
    @clear-slot="clearCurrentAIPolishSlot"
    @close="closeAIPolishDock"
    @update:source-text="updateActiveAIPolishSourceText"
    @update:result-text="updateActiveAIPolishResultText"
    @update:view-mode="updateActiveAIPolishViewMode"
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
  <DiceTrayFloatingWindow
    ref="diceTrayWindowRef"
    :available="showDiceTrayTrigger"
    :storage-scope="diceTrayStorageScope"
    :default-dice="defaultDiceExpr"
    :can-edit-default="canEditDefaultDice"
    :built-in-dice-enabled="effectiveBuiltInDiceEnabled"
    :bot-feature-enabled="effectiveBotFeatureEnabled"
    @insert="handleDiceInsert"
    @roll="handleDiceRollNow"
    @update-default="handleDiceDefaultUpdate"
    @mode-change="handleDiceTrayModeChange"
  >
    <template #header-actions="{ isMobile: diceTrayIsMobile }">
      <ChatDiceModeControl
        :visible="diceSettingsVisible"
        :show-status="showDiceModeStatus"
        :show-settings="showDiceModeSettings"
        :is-mobile="diceTrayIsMobile"
        :mode-label="diceModeLabel"
        :mode-tooltip="diceModeTooltip"
        :built-in-dice-enabled="channelFeatures.builtInDiceEnabled"
        :bot-feature-enabled="channelFeatures.botFeatureEnabled"
        :dice-feature-updating="diceFeatureUpdating"
        :channel-bot-selection="channelBotSelection"
        :bot-select-options="botSelectOptions"
        :bot-options-loading="botOptionsLoading"
        :channel-bots-loading="channelBotsLoading"
        :syncing-channel-bot="syncingChannelBot"
        :has-bot-options="hasBotOptions"
        @update:visible="diceSettingsVisible = $event"
        @toggle-built-in="handleDiceFeatureToggle"
        @toggle-bot="handleBotFeatureToggle"
        @select-bot="handleBotSelectionChange"
        @open-channel-member-settings="openChannelMemberSettings"
      />
    </template>
  </DiceTrayFloatingWindow>
  <IFormFloatingWindows />
  <IFormDrawer />

  <DisplaySettingsModal
    v-model:visible="displaySettingsVisible"
    :settings="display.settings"
    @save="handleDisplaySettingsSave"
  />

  <ChannelFavoriteManager v-model:show="channelFavoritesVisible" />
  <ChannelRemarkManager v-model:show="characterRemarkManagerVisible" />
  <WorldKeywordManager />
  <AnnouncementManagerModal
    v-model:visible="showWorldAnnouncementModal"
    scope-type="world"
    :scope-id="chat.currentWorldId"
    title="世界公告"
    :can-manage="canManageWorldAnnouncements"
  />

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

	<DiceOverlayLoader v-if="!isTheaterEmbedMode && display.settings.dice3dEnabled" :surface-element="messagesListRef" />
	<DiceDock
		v-if="!isTheaterEmbedMode && display.settings.dice3dEnabled"
		:enabled="dice3dProfile?.dockEnabled === true"
			:x="dice3dProfile?.dockX"
			:y="dice3dProfile?.dockY"
			:corner="dice3dProfile?.dockCorner"
			:stacks="dice3dProfile?.dockStacks"
		@roll="handleDiceRollNow"
		@move="handleDice3DDockMove"
	/>
	<DiceSettingsDrawer
		v-model:show="dice3dSettingsVisible"
		:world-id="chat.currentWorldId"
		:can-manage-world="canManageDice3DWorld"
		@profile-saved="handleDice3DProfileSaved"
	/>

  <!-- 人物卡预览窗口 -->
  <CharacterSheetManager />
</template>

<style lang="scss" scoped src="./styles/chat.scoped.scss"></style>

<style lang="scss" src="./styles/chat.global.scss"></style>
