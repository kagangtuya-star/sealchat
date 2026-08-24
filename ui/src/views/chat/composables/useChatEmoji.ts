import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type ComputedRef } from 'vue';
import { useEventListener } from '@vueuse/core';
import type { ChannelIdentityVariant, GalleryItem } from '@/types';
import { useChatStore } from '@/stores/chat';
import { useGalleryStore, DEFAULT_GALLERY_PAGE_SIZE } from '@/stores/gallery';
import { useUserStore } from '@/stores/user';
import { fetchAttachmentMetaById, normalizeAttachmentId, resolveAttachmentUrl, type AttachmentMeta } from '@/composables/useAttachmentResolver';
import { matchText } from '@/utils/pinyinMatch';
import { dialogAskConfirm } from '@/utils/dialog';
import { useRobustInfiniteScroll } from '@/composables/useRobustInfiniteScroll';
import { chatEvent } from '@/stores/chat';
import { urlBase } from '@/stores/_config';

interface ChatEmojiOptions {
  chat: ReturnType<typeof useChatStore>;
  gallery: ReturnType<typeof useGalleryStore>;
  user: ReturnType<typeof useUserStore>;
  message: any;
  dialog: any;
  editingIdentityPreviewContext: ComputedRef<any>;
  resolveIdentityAppearancePreview: (...args: any[]) => any;
  cloneAvatarDecorations: (...args: any[]) => any;
  resolveVariantNote: (variant: ChannelIdentityVariant) => string;
  canManageIdentities: () => boolean;
}

export const useChatEmoji = ({
  chat,
  gallery,
  user,
  message,
  dialog,
  editingIdentityPreviewContext,
  resolveIdentityAppearancePreview,
  cloneAvatarDecorations,
  resolveVariantNote,
  canManageIdentities,
}: ChatEmojiOptions) => {
  const emojiLoading = ref(false);
  const emojiItems = computed<GalleryItem[]>(() => gallery.emojiItems);
  const EMOJI_THUMB_SIZE = 80;
  const emojiAttachmentMetaCache = reactive<Record<string, AttachmentMeta | null>>({});
  const pendingEmojiMetaFetch = new Set<string>();

  const ensureEmojiAttachmentMeta = async (attachmentId: string) => {
    const normalized = normalizeAttachmentId(attachmentId);
    if (!normalized || pendingEmojiMetaFetch.has(normalized) || emojiAttachmentMetaCache[normalized] !== undefined) return;
    pendingEmojiMetaFetch.add(normalized);
    try {
      emojiAttachmentMetaCache[normalized] = await fetchAttachmentMetaById(normalized);
    } finally {
      pendingEmojiMetaFetch.delete(normalized);
    }
  };

  const resolveEmojiAttachmentUrl = (attachmentId: string) => {
    const normalized = normalizeAttachmentId(attachmentId);
    if (!normalized) return '';
    const meta = emojiAttachmentMetaCache[normalized];
    if (meta === undefined && !pendingEmojiMetaFetch.has(normalized)) void ensureEmojiAttachmentMeta(normalized);
    if (meta?.isAnimated) return resolveAttachmentUrl(normalized);
    return `${urlBase}/api/v1/attachment/${normalized}/thumb?size=${EMOJI_THUMB_SIZE}`;
  };

  const getEmojiItemSrc = (item: GalleryItem) => resolveEmojiAttachmentUrl(item.attachmentId);
  const hasEmojiItems = computed(() => emojiItems.value.length > 0);
  const emojiPopoverShow = ref(false);
  const emojiTriggerButtonRef = ref<HTMLElement | null>(null);
  const emojiAnchorElement = ref<HTMLElement | null>(null);
  const emojiPopoverX = ref<number | null>(null);
  const emojiPopoverY = ref<number | null>(null);
  const emojiPopoverXCoord = computed(() => emojiPopoverX.value ?? undefined);
  const emojiPopoverYCoord = computed(() => emojiPopoverY.value ?? undefined);
  const emojiSearchQuery = ref('');
  const emojiPanelTab = ref<'gallery' | 'utf' | 'variant'>('gallery');
  const emojiPanelRenderKey = ref(0);
  const emojiPanelContentRef = ref<HTMLElement | null>(null);
  const emojiPanelLoadMoreSentinelRef = ref<HTMLElement | null>(null);
  const isManagingEmoji = ref(false);
  const emojiRemarkVisible = computed(() => gallery.emojiRemarkVisible);

  const activeIdentityForEmojiPanel = computed(() => editingIdentityPreviewContext.value?.identity || chat.getActiveIdentity(chat.curChannel?.id || ''));
  const activeIdentityVariantOptions = computed(() => {
    const channelId = chat.curChannel?.id || '';
    const identityId = activeIdentityForEmojiPanel.value?.id || '';
    if (!channelId || !identityId) return [] as ChannelIdentityVariant[];
    return chat.getIdentityVariants(channelId, identityId).filter((item) => item.enabled !== false);
  });
  const activeIdentityVariantForEmojiPanel = computed(() => {
    if (editingIdentityPreviewContext.value) return editingIdentityPreviewContext.value.variant;
    const channelId = chat.curChannel?.id || '';
    const identityId = activeIdentityForEmojiPanel.value?.id || '';
    return channelId && identityId ? chat.getActiveIdentityVariant(channelId, identityId) : null;
  });
  const filteredIdentityVariantOptions = computed(() => {
    const query = emojiSearchQuery.value.trim();
    if (!query) return activeIdentityVariantOptions.value;
    return activeIdentityVariantOptions.value.filter((item) => matchText(query, `${item.keyword || ''} ${item.note || ''} ${item.displayName || ''}`));
  });
  const hasIdentityVariantOptions = computed(() => activeIdentityVariantOptions.value.length > 0);
  const identityVariantTabTooltip = computed(() => {
    if (!activeIdentityForEmojiPanel.value) return '请先选择频道角色，再切换头像差分';
    if (!hasIdentityVariantOptions.value) return canManageIdentities() ? '当前频道角色尚未配置头像差分，点击前往设置' : '当前频道角色尚未配置头像差分';
    return '切换当前频道角色的头像差分，可用已配置的匹配规则快捷切换或恢复';
  });
  const describeIdentityVariantCard = (variant?: ChannelIdentityVariant | null) => {
    if (!variant) return '恢复为当前频道角色的默认头像';
    const summary = [resolveVariantNote(variant), variant.keyword ? `关键词：=${variant.keyword}` : ''].filter(Boolean).join('\n');
    const details = [variant.displayName ? `覆盖昵称：${variant.displayName}` : '仅覆盖头像', variant.color ? `覆盖颜色：${variant.color}` : '', variant.note && variant.note !== resolveVariantNote(variant) ? variant.note : ''].filter(Boolean).join('\n');
    return [summary, details].filter(Boolean).join('\n');
  };

  const activeEmojiTab = computed({
    get: () => gallery.activeEmojiTabId,
    set: (value) => { if (user.info?.id) gallery.setActiveEmojiTab(value, user.info.id); },
  });
  const emojiTabOptions = computed(() => {
    const ownerId = user.info?.id;
    if (!ownerId) return [];
    const collections = gallery.getCollections(ownerId);
    return gallery.allEmojiCollectionIds.map((id) => ({ id, name: collections.find((item) => item.id === id)?.name || '未知分类', isFavorites: id === gallery.favoritesCollectionId }));
  });
  const hasMultipleTabs = computed(() => emojiTabOptions.value.length > 1);
  const emojiPanelPagination = computed(() => {
    const tabId = activeEmojiTab.value;
    return tabId ? gallery.getItemPagination(tabId) : { page: 1, pageSize: DEFAULT_GALLERY_PAGE_SIZE, total: emojiItems.value.length };
  });
  const emojiPanelLoading = computed(() => activeEmojiTab.value ? gallery.isCollectionLoading(activeEmojiTab.value) : false);
  const emojiPanelLoadingMore = computed(() => activeEmojiTab.value ? gallery.isCollectionLoadingMore(activeEmojiTab.value) : false);
  const emojiPanelHasMore = computed(() => {
    const tabId = activeEmojiTab.value;
    return !!tabId && emojiPanelPagination.value.total > gallery.getItemsByCollection(tabId).length;
  });

  const toggleEmojiRemarkVisible = () => {
    const userId = user.info?.id;
    if (!userId) { message.warning('请先登录'); return; }
    gallery.setEmojiRemarkVisible(!gallery.emojiRemarkVisible, userId);
  };
  const resolveEmojiAnchorElement = () => {
    const current = emojiAnchorElement.value;
    return typeof window !== 'undefined' && current && document.body.contains(current) ? current : null;
  };
  const syncEmojiPopoverPosition = (trigger?: HTMLElement | null) => {
    const anchor = trigger || resolveEmojiAnchorElement() || emojiTriggerButtonRef.value;
    if (!anchor) return false;
    emojiAnchorElement.value = anchor;
    const rect = anchor.getBoundingClientRect();
    emojiPopoverX.value = rect.left;
    emojiPopoverY.value = rect.top + 10;
    return true;
  };
  const allGalleryItems = computed(() => Object.values(gallery.items).flatMap((entry) => entry?.items ?? []));
  const emojiUsageMap = ref<Record<string, number>>({});
  const emojiUsageKey = 'sealchat_emoji_usage';
  const ensureEmojiCollectionLoaded = async () => {
    const ownerId = user.info?.id;
    if (!ownerId) return;
    try { await gallery.ensureEmojiCollection(ownerId); } catch { /* ignore load errors */ }
  };
  const loadMoreEmojiPanelItems = async () => {
    const tabId = activeEmojiTab.value;
    if (!tabId || emojiPanelLoading.value || emojiPanelLoadingMore.value || !emojiPanelHasMore.value) return;
    const current = gallery.getItemPagination(tabId);
    await gallery.loadItems(tabId, { page: current.page + 1, pageSize: current.pageSize, append: true });
  };
  const handleEmojiPanelContentScroll = (event: Event) => {
    if (emojiPanelTab.value !== 'gallery') return;
    const target = event.target as HTMLElement | null;
    if (target && target.scrollTop + target.clientHeight >= target.scrollHeight - 40) void loadMoreEmojiPanelItems();
  };
  const refreshEmojiPanelRender = () => { emojiPanelRenderKey.value += 1; };

  if (typeof window !== 'undefined') {
    useEventListener(window, 'resize', () => { if (emojiPopoverShow.value) syncEmojiPopoverPosition(); });
    useEventListener(window, 'scroll', () => { if (emojiPopoverShow.value) syncEmojiPopoverPosition(); }, { passive: true, capture: true });
  }
  onMounted(() => {
    try { const stored = localStorage.getItem(emojiUsageKey); if (stored) emojiUsageMap.value = JSON.parse(stored); } catch (error) { console.warn('Failed to load emoji usage', error); }
  });
  const recordEmojiUsage = (id: string) => {
    emojiUsageMap.value[id] = Date.now();
    try { localStorage.setItem(emojiUsageKey, JSON.stringify(emojiUsageMap.value)); } catch (error) { console.warn('Failed to save emoji usage', error); }
  };
  const sortByUsage = <T extends { id: string }>(items: T[]) => [...items].sort((a, b) => (emojiUsageMap.value[b.id] || 0) - (emojiUsageMap.value[a.id] || 0));
  const filteredEmojiItems = computed(() => {
    const query = emojiSearchQuery.value.trim();
    const items = activeEmojiTab.value ? gallery.getItemsByCollection(activeEmojiTab.value) : emojiItems.value;
    const filtered = !query ? items : items.filter((item, index) => matchText(query, (item.remark && item.remark.trim()) || `收藏${index + 1}`));
    return sortByUsage(filtered);
  });
  useRobustInfiniteScroll({
    containerRef: emojiPanelContentRef,
    sentinelRef: emojiPanelLoadMoreSentinelRef,
    enabled: computed(() => emojiPanelTab.value === 'gallery'),
    canLoadMore: emojiPanelHasMore,
    loading: computed(() => emojiPanelLoading.value || emojiPanelLoadingMore.value),
    onLoadMore: loadMoreEmojiPanelItems,
    triggerDeps: () => [emojiPanelTab.value, activeEmojiTab.value, filteredEmojiItems.value.length, emojiPanelLoading.value, emojiPanelLoadingMore.value],
    rootMargin: '0px 0px 80px 0px', bottomOffset: 40, scrollFallback: true, observeResize: true, requestAnimationFrameCheck: true,
  });
  const buildEmojiRemarkMap = () => {
    const map = new Map<string, string>();
    for (const item of [...emojiItems.value, ...allGalleryItems.value]) {
      const remark = item.remark?.trim();
      if (remark && item.attachmentId && !map.has(remark)) map.set(remark, item.attachmentId);
    }
    return map;
  };
  const replaceEmojiRemarksForPreview = (text: string) => {
    const map = buildEmojiRemarkMap();
    return text.replace(/[\[【\/]([^\]】\/]+)[\]】\/]/g, (match, remark) => {
      const attachmentId = map.get(remark.trim());
      if (!attachmentId) return match;
      const normalized = attachmentId.startsWith('id:') ? attachmentId.slice(3) : attachmentId;
      return `[[img:id:${normalized}]]`;
    });
  };

  const selectedEmojiIds = ref<string[]>([]);
  const emojiRemarkModalVisible = ref(false);
  const emojiRemarkInput = ref('');
  const emojiRemarkSaving = ref(false);
  const editingEmojiItem = ref<GalleryItem | null>(null);
  const emojiRemarkPattern = /^[\p{L}\p{N}_]{1,64}$/u;
  const resolveEmojiRemark = (item: GalleryItem, idx: number) => item.remark?.trim() || `收藏${idx + 1}`;
  const openEmojiRemarkEditor = (item: GalleryItem) => { editingEmojiItem.value = item; emojiRemarkInput.value = item.remark?.trim() || ''; emojiRemarkModalVisible.value = true; };
  const submitEmojiRemark = async () => {
    if (!editingEmojiItem.value) return false;
    const remark = emojiRemarkInput.value.trim();
    if (!remark) { message.warning('备注不能为空'); return false; }
    if (!emojiRemarkPattern.test(remark)) { message.warning('备注仅支持字母、数字和下划线，长度不超过64'); return false; }
    emojiRemarkSaving.value = true;
    try { await gallery.updateItem(editingEmojiItem.value.collectionId, editingEmojiItem.value.id, { remark }); message.success('备注已更新'); emojiRemarkModalVisible.value = false; return true; }
    catch (error: any) { console.error('更新表情备注失败', error); message.error(error?.message || '更新失败，请稍后再试'); return false; }
    finally { emojiRemarkSaving.value = false; }
  };
  const cancelEmojiRemark = () => { if (emojiRemarkSaving.value) return false; emojiRemarkModalVisible.value = false; return true; };
  const exitEmojiManage = () => { isManagingEmoji.value = false; selectedEmojiIds.value = []; };
  const emojiSelectedDelete = async () => {
    if (!(await dialogAskConfirm(dialog))) return;
    if (!selectedEmojiIds.value.length) { message.info('没有选中的表情'); return; }
    const collectionId = gallery.favoritesCollectionId;
    if (!collectionId) { message.error('未找到表情收藏分类'); return; }
    try { await gallery.deleteItems(collectionId, selectedEmojiIds.value); message.success('已删除所选表情'); selectedEmojiIds.value = []; }
    catch (error: any) { console.error('删除表情失败', error); message.error(error?.message || '删除失败，请稍后再试'); }
  };

  watch(() => user.info.id, async (id) => { if (!id) return; gallery.loadEmojiPreference(id); await ensureEmojiCollectionLoaded(); }, { immediate: true });
  watch(() => gallery.emojiCollectionIds, (ids) => { for (const id of ids) void gallery.loadItems(id); }, { deep: true });
  watch(emojiPopoverShow, (show, previous) => {
    if (!show) { isManagingEmoji.value = false; emojiSearchQuery.value = ''; }
    else { refreshEmojiPanelRender(); nextTick(() => syncEmojiPopoverPosition()); void ensureEmojiCollectionLoaded(); }
    if (show) chatEvent.emit('global-overlay-toggle', { source: 'emoji-panel', open: true } as any);
    else if (previous) chatEvent.emit('global-overlay-toggle', { source: 'emoji-panel', open: false } as any);
  });
  watch(hasIdentityVariantOptions, (hasOptions) => { if (!hasOptions && emojiPanelTab.value === 'variant') emojiPanelTab.value = 'gallery'; });
  watch(isManagingEmoji, (value) => { if (value) void ensureEmojiCollectionLoaded(); });
  onBeforeUnmount(() => { emojiPopoverShow.value = false; pendingEmojiMetaFetch.clear(); });

  return {
    emojiLoading, emojiItems, ensureEmojiAttachmentMeta, resolveEmojiAttachmentUrl, getEmojiItemSrc, hasEmojiItems,
    emojiPopoverShow, emojiTriggerButtonRef, emojiAnchorElement, emojiPopoverXCoord, emojiPopoverYCoord, emojiSearchQuery,
    emojiPanelTab, emojiPanelRenderKey, emojiPanelContentRef, emojiPanelLoadMoreSentinelRef, isManagingEmoji, emojiRemarkVisible,
    activeIdentityForEmojiPanel, activeIdentityVariantOptions, activeIdentityVariantForEmojiPanel, filteredIdentityVariantOptions,
    hasIdentityVariantOptions, identityVariantTabTooltip, describeIdentityVariantCard, activeEmojiTab, emojiTabOptions, hasMultipleTabs,
    emojiPanelPagination, emojiPanelLoading, emojiPanelLoadingMore, emojiPanelHasMore, toggleEmojiRemarkVisible, syncEmojiPopoverPosition,
    allGalleryItems, emojiUsageMap, ensureEmojiCollectionLoaded, loadMoreEmojiPanelItems, handleEmojiPanelContentScroll, refreshEmojiPanelRender,
    recordEmojiUsage, filteredEmojiItems, buildEmojiRemarkMap, replaceEmojiRemarksForPreview,
    selectedEmojiIds, emojiRemarkModalVisible, emojiRemarkInput, emojiRemarkSaving, editingEmojiItem, resolveEmojiRemark,
    openEmojiRemarkEditor, submitEmojiRemark, cancelEmojiRemark, exitEmojiManage, emojiSelectedDelete,
  };
};
