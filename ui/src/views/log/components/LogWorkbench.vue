<script setup lang="ts">
// 设计工作台：页面级编排。草稿控制器、章节消息窗口、样式包、预览与选择在此组合；
// 具体面板各自负责一块交互。认证与写入只发生在外层，iframe 只接收安全渲染模型。
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, toRaw, watch, type Component } from 'vue';
import { useDialog, useMessage, type DropdownOption } from 'naive-ui';
import { useMediaQuery } from '@vueuse/core';
import { useRouter } from 'vue-router';
import {
  AlertTriangle,
  ArrowLeft,
  Book2,
  Components,
  Download,
  Eye,
  LayoutSidebarRight,
  ListCheck,
  Minus,
  PageBreak,
  Palette,
  Pencil,
  Photo,
  Plus,
  Printer,
  Rocket,
  Search,
  Stack2,
  Users,
  X,
} from '@vicons/tabler';
import {
  describeLogError,
  fetchLogStylePackage,
  isLogError,
  updateLogExclusions,
} from '@/services/log/logApi';
import {
  listWorldChannels,
  relocateMessagesBatch,
  removeMessage,
  removeMessagesBatch,
  reorderMessage,
} from '@/services/log/logMessageCommands';
import {
  buildFrameDocument,
  composeDesignSection,
  designBackgroundToken,
  designStyleFromPackage,
  type DesignNodeCacheEntry,
} from '@/services/log/logRenderer';
import type { FrameDocument, FrameEvent, FrameLayoutResult, FramePrintRange } from '@/services/log/pagination/logFrameHost';
import { updateDraftImageWidth } from '@/services/log/logImageLayout';
import { buildTemplateLibrary, messageTemplateIds } from '@/services/log/logTemplateCompiler';
import type { LogAnchor, LogDesignMessage, LogElementType, LogProjectContext, LogProtocolMessage, LogSection, LogStylePackage } from '@/types/log';
import { LOG_ELEMENT_TYPES } from '@/types/log';
import { useLogDraft } from '../composables/useLogDraft';
import { useLogMessageWindow } from '../composables/useLogMessageWindow';
import { useLogAssets } from '../composables/useLogAssets';
import { useLogAssetLibrary } from '../composables/useLogAssetLibrary';
import { useLogSelection } from '../composables/useLogSelection';
import { createElement, downloadJson, elementAvailability, rebindStyle, setMessageTemplateOverrides } from '../logDraftOps';
import type { StyleStudioPreview, StyleStudioRequest } from '../logStyleWorkingCopy';
import { useLogStyleLibrary } from '../composables/useLogStyleLibrary';
import { planBatchRelocate, planBatchStep, type LogBatchMovePlan } from '../logBatchOps';
import type { LogImageRef } from '../logAssetRefs';
import { chapterTitle, chapterViews, sectionLabel } from '../logChapterOps';
import LogPreviewFrame from './LogPreviewFrame.vue';
import LogSelectionBar from './LogSelectionBar.vue';
import LogMultiSelectBar from './LogMultiSelectBar.vue';
import LogMessageEditor from './LogMessageEditor.vue';
import LogSectionsDialog from './LogSectionsDialog.vue';
import LogBulkPanel from './LogBulkPanel.vue';
import LogElementsPanel from './LogElementsPanel.vue';
import LogPagePanel from './LogPagePanel.vue';
import LogStylePanel from './LogStylePanel.vue';
import LogStyleStudio from './LogStyleStudio.vue';
import LogPublishPanel from './LogPublishPanel.vue';
import LogAssetsPanel from './LogAssetsPanel.vue';
import LogInsertElementDialog from './LogInsertElementDialog.vue';
import LogStandardExportDialog from './LogStandardExportDialog.vue';
import LogPdfExportDialog from './LogPdfExportDialog.vue';
import LogPrintDialog from './LogPrintDialog.vue';

const props = defineProps<{ context: LogProjectContext; slug: string }>();
const emit = defineEmits<{ (e: 'reload'): void }>();

const toast = useMessage();
const dialog = useDialog();
const router = useRouter();
const narrow = useMediaQuery('(max-width: 900px)');

const projectId = computed(() => props.context.projectId);
const caps = computed(() => props.context.capabilities);
const draftCtl = useLogDraft(projectId, { draftVersion: props.context.draftVersion, draft: props.context.draft });
const draft = draftCtl.draft;

// ---- 章节：查询用的章节只采用已保存版本（服务端按已保存草稿解析来源） ----
const sections = computed(() => draft.value.sections.slice().sort((a, b) => a.order - b.order));
const sectionId = ref(sections.value[0]?.id || '');
const currentSection = computed(() => sections.value.find((s) => s.id === sectionId.value) || null);
// 视觉章节：工作台仍按来源片段加载与编辑，预览与页眉使用片段所属章节的标题。
const chapters = computed(() => chapterViews(draft.value));
const currentChapterIndex = computed(() => chapters.value.findIndex((c) => c.sections.some((s) => s.id === sectionId.value)));
const currentChapter = computed(() => chapters.value[currentChapterIndex.value] || null);
const currentIsFragment = computed(() => (currentChapter.value?.sections.length || 0) > 1);
const visualTitle = computed(() => currentChapter.value?.title || '');
// 只有出现多来源章节时才需要频道名来区分片段；读取失败时退回频道 id。
const channelNames = shallowRef<Map<string, string>>(new Map());
let channelNamesRequested = false;
watch(() => chapters.value.some((c) => c.sections.length > 1), async (needed) => {
  if (!needed || channelNamesRequested) return;
  channelNamesRequested = true;
  try {
    const list = await listWorldChannels(props.context.worldId);
    channelNames.value = new Map(list.map((c) => [c.id, c.name]));
  } catch {
    channelNamesRequested = false;
  }
}, { immediate: true });
const sourceChannelName = (id: string) => channelNames.value.get(id) || id;
/** 多来源章节中当前片段的位置说明，例如“大厅 · 汇合（2/4）”。 */
const fragmentHint = computed(() => {
  const c = currentChapter.value;
  const s = currentSection.value;
  if (!c || !s || c.sections.length < 2) return '';
  return `${sectionLabel(s, sourceChannelName)}（${c.sections.indexOf(s) + 1}/${c.sections.length}）`;
});
const querySection = shallowRef<LogSection | null>(null);

const commitQuerySection = () => {
  const sec = currentSection.value;
  if (!sec) {
    querySection.value = null;
    return;
  }
  const prev = querySection.value;
  if (!prev || prev.id !== sec.id || JSON.stringify(prev.source) !== JSON.stringify(sec.source) || prev.title !== sec.title) {
    querySection.value = structuredClone(toRaw(sec));
  }
};

watch(sectionId, async () => {
  if (draftCtl.status.value !== 'saved') await draftCtl.flush();
  commitQuerySection();
}, { immediate: true });
watch(() => draftCtl.status.value, (s) => {
  if (s === 'saved') commitQuerySection();
});
watch(sections, (list) => {
  if (!list.some((s) => s.id === sectionId.value)) sectionId.value = list[0]?.id || '';
});

const win = useLogMessageWindow(projectId, querySection);
const onNearEnd = () => {
  if (!querySection.value || querySection.value.id !== currentSection.value?.id
    || !win.nextCursor.value || win.loading.value || win.error.value) return;
  void win.loadMore();
};

// ---- 样式包 ----
// stylePkg 是草稿正式绑定的版本；预览覆盖（样式面板的库预览、Style Studio 的工作副本）只替换 activePkg，
// 从不修改草稿的 stylePackageId / stylePackageVersion，关闭或取消即恢复。
const stylePkg = shallowRef<LogStylePackage | null>(null);
const libraryPreview = shallowRef<LogStylePackage | null>(null);
const studioPreview = shallowRef<StyleStudioPreview | null>(null);
const styleOverride = computed(() => studioPreview.value?.pkg || libraryPreview.value);
const styleError = ref('');
const activePkg = computed(() => styleOverride.value || stylePkg.value);
const messageTemplates = computed(() => activePkg.value
  ? messageTemplateIds(buildTemplateLibrary(activePkg.value.templates, activePkg.value.settingsSchema)) : []);
const assets = useLogAssets({
  projectId: () => projectId.value,
  stylePackage: () => (activePkg.value ? { id: activePkg.value.id, version: activePkg.value.version } : null),
});
const assetLibrary = useLogAssetLibrary({
  draft,
  update: draftCtl.update,
  stylePackage: () => activePkg.value,
  canEdit: () => caps.value.editDraft,
  assets,
});
let styleSeq = 0;
watch(() => `${draft.value.stylePackageId}@${draft.value.stylePackageVersion}`, async () => {
  const id = ++styleSeq;
  const { stylePackageId, stylePackageVersion } = draft.value;
  styleError.value = '';
  try {
    const pkg = await fetchLogStylePackage(stylePackageId, stylePackageVersion);
    if (id !== styleSeq) return;
    stylePkg.value = pkg;
  } catch (error) {
    if (id !== styleSeq) return;
    styleError.value = describeLogError(error, '样式包加载失败');
  }
}, { immediate: true });
/** Studio 工作副本中尚未保存的资源只有本地字节：预置到资源缓存，预览无需向服务端读取。 */
const seedStudioAssets = () => {
  for (const [id, a] of Object.entries(studioPreview.value?.assets || {})) assets.seed(`style:${id}`, a.data, a.mime);
};
const styleSourceKey = () => (studioPreview.value ? `studio${studioPreview.value.session}` : libraryPreview.value ? 'library' : 'bound');
watch(() => (activePkg.value ? `${activePkg.value.id}@${activePkg.value.version}:${styleSourceKey()}` : ''), () => {
  assets.invalidate('style:');
  seedStudioAssets();
});
watch(studioPreview, seedStudioAssets);

const styleLibrary = useLogStyleLibrary();
onMounted(() => void styleLibrary.reload());

/**
 * 让当前 LOG 使用某个样式版本：这是唯一写入草稿样式绑定的地方。
 * 先做兼容检查；需要调整草稿时请用户确认，存在无法自动修复的问题时拒绝切换。
 */
const bindStyle = (pkg: LogStylePackage): Promise<boolean> => new Promise((resolve) => {
  if (!caps.value.editDraft) {
    toast.warning('没有编辑当前 LOG 的权限');
    resolve(false);
    return;
  }
  const commit = () => {
    const { next, problems } = rebindStyle(draft.value, pkg);
    if (problems.some((p) => !p.fixable)) return false;
    const ok = draftCtl.update((d) => {
      Object.assign(d, next);
    });
    if (!ok) {
      toast.error('草稿存在冲突，未切换样式');
      return false;
    }
    if (libraryPreview.value) libraryPreview.value = null;
    toast.success(`当前 LOG 已切换到 ${pkg.name} ${pkg.version}（草稿保存后生效）`);
    return true;
  };
  const { problems } = rebindStyle(draft.value, pkg);
  if (!problems.length) {
    resolve(commit());
    return;
  }
  const blocked = problems.some((p) => !p.fixable);
  let settled = false;
  const done = (v: boolean) => {
    if (!settled) resolve(v);
    settled = true;
  };
  dialog[blocked ? 'error' : 'warning']({
    title: blocked ? '暂时无法切换到这个样式' : '切换样式需要调整草稿',
    content: () => h('ul', { style: 'margin: 0; padding-left: 18px' }, problems.map((p) => h('li', p.message))),
    positiveText: blocked ? '知道了' : '按上述调整并切换',
    negativeText: blocked ? undefined : '取消',
    onPositiveClick: () => done(blocked ? false : commit()),
    onNegativeClick: () => done(false),
    onClose: () => done(false),
    onMaskClick: () => done(false),
  });
});

// ---- Style Studio ----
// 宽屏时占据设计面板位置、与预览画布并排；窄屏时覆盖全屏，可临时切到预览查看效果。
const studioOpen = ref(false);
const studioPeek = ref(false);
const studioRequest = shallowRef<(StyleStudioRequest & { nonce: number }) | null>(null);
const studioRef = ref<InstanceType<typeof LogStyleStudio> | null>(null);
let studioNonce = 0;
const openStudio = (request: StyleStudioRequest) => {
  libraryPreview.value = null;
  studioRequest.value = { ...request, nonce: ++studioNonce };
  studioOpen.value = true;
  studioPeek.value = false;
};
const onStudioClosed = () => {
  studioOpen.value = false;
  studioPeek.value = false;
  studioPreview.value = null;
  studioRequest.value = null;
};
/** 从 Studio 切到其他设计面板：先经过 Studio 的未保存保护。 */
const leaveStudio = async () => {
  if (!studioOpen.value) return true;
  if (!studioRef.value) {
    onStudioClosed();
    return true;
  }
  return studioRef.value.requestClose();
};

// ---- 组装与预览文档 ----
const nodeCache = new Map<string, DesignNodeCacheEntry>();
const composition = computed(() => {
  const sec = querySection.value;
  if (!sec) return null;
  return composeDesignSection(sec.id, visualTitle.value, win.items.value, draft.value.elements, win.complete.value, nodeCache);
});
const orderedIds = computed(() => composition.value?.nodes.map((n) => n.id) || []);

const previewPublish = ref(false);
const viewMode = ref<'single' | 'spread' | 'flow'>('single');
const viewModes = [
  { value: 'single', label: '单页' },
  { value: 'spread', label: '双页' },
  { value: 'flow', label: '流式' },
] as const;
const zoom = ref(1);
const frame = shallowRef<{ doc: FrameDocument; assetTokens: string[]; issues: string[]; revision: number } | null>(null);
let frameRevision = 0;
let lastDocJson = '';
let buildTimer: ReturnType<typeof setTimeout> | null = null;

const rebuild = (force = false) => {
  const comp = composition.value;
  if (!comp) {
    frame.value = null;
    lastDocJson = '';
    return;
  }
  const pkg = activePkg.value;
  const d = draft.value;
  const result = buildFrameDocument({
    style: pkg ? designStyleFromPackage(pkg, d.themeTokens) : null,
    page: d.page,
    title: d.title,
    sectionTitle: visualTitle.value,
    nodes: comp.nodes,
    toc: comp.toc,
    backgroundToken: designBackgroundToken(d.page),
    imageWidths: d.imageWidths,
    templateSelection: d,
  });
  const json = JSON.stringify(result.doc);
  if (!force && json === lastDocJson && frame.value) return;
  lastDocJson = json;
  frame.value = { doc: result.doc, assetTokens: result.assetTokens, issues: result.issues, revision: ++frameRevision };
};
watch([composition, activePkg, () => draft.value.page, () => draft.value.themeTokens, () => draft.value.title, () => draft.value.imageWidths, () => draft.value.displayRules, () => draft.value.messageTemplateOverrides], () => {
  if (buildTimer) clearTimeout(buildTimer);
  buildTimer = setTimeout(() => {
    buildTimer = null;
    rebuild();
  }, 120);
}, { immediate: true });

const layoutInfo = shallowRef<FrameLayoutResult | null>(null);
const layoutError = ref('');
const layoutBusy = ref(false);
const previewRef = ref<InstanceType<typeof LogPreviewFrame> | null>(null);
let printLayoutWaiter: { targetRevision: number; resolve: (r: FrameLayoutResult) => void; reject: (e: Error) => void } | null = null;
const onLayout = (r: FrameLayoutResult, revision: number) => {
  if (revision < frameRevision) return;
  layoutInfo.value = r;
  layoutError.value = '';
  if (printLayoutWaiter && revision >= printLayoutWaiter.targetRevision) {
    const waiter = printLayoutWaiter;
    printLayoutWaiter = null;
    waiter.resolve(r);
  }
  if (relocateScrollId && r.map[relocateScrollId]) {
    previewRef.value?.scrollTo(relocateScrollId, true);
    relocateScrollId = null;
  }
};

// ---- 选择 ----
const selection = useLogSelection(orderedIds);
watch(orderedIds, (ids) => selection.prune(new Set(ids)));
/** 当前来源片段已加载消息的可见顺序（批量移动按此计算）。 */
const windowOrder = computed(() => win.items.value.map((it) => it.message.id));
const selectedMessages = computed(() => {
  const picked = new Set(selection.messageIds.value);
  return win.items.value.filter((it) => picked.has(it.message.id));
});
const selectedElement = computed(() => {
  const id = selection.elementId.value;
  return id ? draft.value.elements.find((e) => e.id === id) || null : null;
});
const selectedIndex = computed(() => (selectedMessages.value.length === 1 ? win.indexOf(selectedMessages.value[0].message.id) : -1));
const applyMessageTemplate = (templateId: string) => {
  const ids = selectedMessages.value.map((m) => m.message.id);
  if (!caps.value.editDraft || selectedElement.value || !ids.length || ids.length > 200 || (templateId && !messageTemplates.value.includes(templateId))) return;
  draftCtl.update((next) => setMessageTemplateOverrides(next, ids, templateId));
};

// ---- 面板 ----
type PanelTab = 'bulk' | 'elements' | 'page' | 'style' | 'assets' | 'publish';
const panelItems: Array<{ key: PanelTab; label: string; hint: string; icon: Component }> = [
  { key: 'elements', label: '元素', hint: '插入与编辑 LOG 元素', icon: Components },
  { key: 'page', label: '页面', hint: '纸张、边距、字体与页眉页脚', icon: PageBreak },
  { key: 'style', label: '样式', hint: '样式包、设计变量与模板', icon: Palette },
  { key: 'bulk', label: '批量', hint: '批量修改历史消息外观', icon: Users },
  { key: 'assets', label: '素材', hint: '当前 LOG 的图片素材与样式资源', icon: Photo },
  { key: 'publish', label: '发布', hint: '发布前检查与发布管理', icon: Rocket },
];
const panelTab = ref<PanelTab>('elements');
const panelMeta = computed(() => panelItems.find((p) => p.key === panelTab.value) || panelItems[0]);
/** 窄屏：设计面板以覆盖层打开。 */
const panelOpen = ref(false);
/** 宽屏：设计面板可收起为仅保留导航栏，状态按浏览器记忆。 */
const panelCollapsed = ref(false);
const panelWidth = ref(360);
try {
  const saved = Number(localStorage.getItem('sealchat.log.panelWidth'));
  if (saved >= 300 && saved <= 560) panelWidth.value = saved;
  panelCollapsed.value = localStorage.getItem('sealchat.log.panelCollapsed') === '1';
} catch {
  // 存储不可用时使用默认宽度
}
watch(panelCollapsed, (v) => {
  try {
    localStorage.setItem('sealchat.log.panelCollapsed', v ? '1' : '0');
  } catch {
    // 忽略
  }
});
const panelVisible = computed(() => (narrow.value ? panelOpen.value : !panelCollapsed.value));
const focusElementId = ref<string | null>(null);
const openPanel = async (tab: PanelTab) => {
  if (!(await leaveStudio())) return;
  panelTab.value = tab;
  panelOpen.value = true;
  panelCollapsed.value = false;
};
const openElementFromPublish = (id: string) => {
  const element = draft.value.elements.find((e) => e.id === id);
  if (element?.anchor.sectionId) sectionId.value = element.anchor.sectionId;
  focusElementId.value = id;
  openPanel('elements');
};
const hidePanel = () => {
  if (narrow.value) panelOpen.value = false;
  else panelCollapsed.value = true;
};
/** 导航栏：点击当前项收起面板，点击其他项切换并展开。 */
const onRailClick = (tab: PanelTab) => {
  if (!studioOpen.value && panelVisible.value && panelTab.value === tab) hidePanel();
  else void openPanel(tab);
};

let resizeStart: { x: number; w: number } | null = null;
const onResizeMove = (e: PointerEvent) => {
  if (!resizeStart) return;
  panelWidth.value = Math.min(560, Math.max(300, resizeStart.w + (resizeStart.x - e.clientX)));
};
const onResizeEnd = () => {
  resizeStart = null;
  window.removeEventListener('pointermove', onResizeMove);
  window.removeEventListener('pointerup', onResizeEnd);
  try {
    localStorage.setItem('sealchat.log.panelWidth', String(panelWidth.value));
  } catch {
    // 忽略
  }
};
const onResizeStart = (e: PointerEvent) => {
  resizeStart = { x: e.clientX, w: panelWidth.value };
  window.addEventListener('pointermove', onResizeMove);
  window.addEventListener('pointerup', onResizeEnd);
};
const onResizeKey = (e: KeyboardEvent) => {
  if (e.key === 'ArrowLeft') panelWidth.value = Math.min(560, panelWidth.value + 16);
  if (e.key === 'ArrowRight') panelWidth.value = Math.max(300, panelWidth.value - 16);
};

// ---- 消息编辑 ----
const editorShow = ref(false);
const editorTab = ref<'content' | 'identity' | 'appearance'>('content');
const editorMessageId = ref('');
const editorItem = computed(() => (editorMessageId.value ? win.byId(editorMessageId.value) : null));
const openEditor = (messageId: string, tab: 'content' | 'identity' | 'appearance') => {
  editorMessageId.value = messageId;
  editorTab.value = tab;
  editorShow.value = true;
};
const onEditorSaved = (message: LogProtocolMessage, version: number) => {
  win.applyServerMessage({ ...message, version });
};
watch(editorItem, (item) => {
  if (editorShow.value && !item && !win.loading.value) {
    editorShow.value = false;
    toast.warning('该消息已不在当前范围内');
  }
});

// ---- 原消息命令 ----
const busy = ref(false);
const sourceChannelId = computed(() => querySection.value?.source.channelId || '');
const target = (item: LogDesignMessage) => ({ channelId: sourceChannelId.value, messageId: item.message.id, expectedVersion: item.version });

const runCommand = async (label: string, fn: () => Promise<unknown>, after: () => void) => {
  busy.value = true;
  try {
    await fn();
    after();
  } catch (error) {
    if (isLogError(error, 'VERSION_CONFLICT')) {
      toast.warning('消息已在别处被修改，已重新加载最新内容');
      win.scheduleReload(0);
    } else {
      toast.error(`${label}失败：${describeLogError(error)}`);
    }
  } finally {
    busy.value = false;
  }
};

const placeAt = (item: LogDesignMessage, toIndex: number, onMoved?: () => void) => {
  // toIndex：移动后在当前窗口中的目标下标（不含自身）。
  const rest = win.items.value.filter((it) => it.message.id !== item.message.id);
  const afterId = rest[toIndex - 1]?.message.id;
  const beforeId = rest[toIndex]?.message.id;
  if (!afterId && !beforeId) return;
  return runCommand('移动', () => reorderMessage(target(item), { afterId, beforeId }), () => {
    win.markOwnWrite(item.message.id, item.version + 1);
    void win.load();
    onMoved?.();
  });
};

const moveSelected = (dir: -1 | 1) => {
  const item = selectedMessages.value[0];
  if (!item) return;
  const idx = win.indexOf(item.message.id);
  placeAt(item, idx + dir);
};

const onDrop = (nodeId: string, targetId: string, side: 'before' | 'after') => {
  const item = win.byId(nodeId);
  if (!item || !item.capabilities.reorder) return;
  let targetMessageId = targetId;
  if (targetId.startsWith('el-')) {
    // 拖到元素上：取元素所锚定的消息
    const el = draft.value.elements.find((e) => `el-${e.id}` === targetId);
    if (!el || el.anchor.kind !== 'message') return;
    targetMessageId = el.anchor.messageId;
  }
  if (targetMessageId === nodeId) return;
  const rest = win.items.value.filter((it) => it.message.id !== nodeId);
  const t = rest.findIndex((it) => it.message.id === targetMessageId);
  if (t < 0) return;
  placeAt(item, side === 'before' ? t : t + 1);
};

interface LogRelocateState {
  active: boolean;
  sourceMessageIds: string[];
  targetMessageId: string | null;
  placement: 'after';
}
const relocation = ref<LogRelocateState>({ active: false, sourceMessageIds: [], targetMessageId: null, placement: 'after' });
const frameRelocation = computed(() => relocation.value.active ? { sourceMessageIds: relocation.value.sourceMessageIds.slice() } : null);
let relocateDialog: ReturnType<typeof dialog.warning> | null = null;
let relocateScrollId: string | null = null;
const cancelRelocate = () => {
  relocation.value = { active: false, sourceMessageIds: [], targetMessageId: null, placement: 'after' };
  relocateDialog?.destroy();
  relocateDialog = null;
};
/** 单选模式只重定位一条；多选模式重定位整个选中集合（按显示顺序）。 */
const startRelocate = (ids: string[]) => {
  const multi = selection.multi.value;
  if (busy.value || previewPublish.value || !ids.length || (!multi && ids.length !== 1) || ids.length > 200) return;
  if (ids.some((id) => !win.byId(id)?.capabilities.reorder)) return;
  cancelRelocate();
  if (!multi) selection.select(ids[0]);
  const picked = new Set(ids);
  relocation.value = { active: true, sourceMessageIds: windowOrder.value.filter((id) => picked.has(id)), targetMessageId: null, placement: 'after' };
};
type RelocateDestination =
  | { items: [LogDesignMessage]; toIndex: number; plan?: undefined }
  | { items: LogDesignMessage[]; plan: LogBatchMovePlan; toIndex?: undefined };
const relocateDestination = (targetId: string): RelocateDestination | null => {
  const state = relocation.value;
  if (!state.active || !state.sourceMessageIds.length || state.sourceMessageIds.includes(targetId)) return null;
  const items = state.sourceMessageIds.map((id) => win.byId(id));
  if (items.some((it) => !it?.capabilities.reorder) || !win.byId(targetId)) return null;
  const sources = items as LogDesignMessage[];
  if (sources.length > 1) {
    // 批量：由 message.relocate.batch 按原有顺序放到目标之后；位置不变时不提交。
    const plan = planBatchRelocate(windowOrder.value, new Set(state.sourceMessageIds), targetId);
    if (!plan) {
      toast.info('所选消息已按原顺序位于此消息之后，请选择其他消息');
      return null;
    }
    return { items: sources, plan };
  }
  const item = sources[0];
  const rest = win.items.value.filter((it) => it.message.id !== item.message.id);
  const toIndex = rest.findIndex((it) => it.message.id === targetId) + 1;
  if (toIndex === win.indexOf(item.message.id)) {
    toast.info('消息已在此位置之后，请选择其他消息');
    return null;
  }
  return { items: [item], toIndex };
};
const confirmRelocate = () => {
  if (busy.value || !relocation.value.targetMessageId) return false;
  const destination = relocateDestination(relocation.value.targetMessageId);
  if (!destination) return;
  const state = relocation.value;
  const finish = () => {
    if (relocation.value !== state) return;
    const id = destination.items[0].message.id;
    // 单选模式保持源消息选中；多选模式保留整个选中集合，便于继续调整。
    if (!selection.multi.value) selection.select(id);
    relocateScrollId = id;
    cancelRelocate();
  };
  if (destination.plan) {
    const { items, plan } = destination;
    return runCommand('重定位', () => relocateMessagesBatch(batchTarget(items), plan), () => {
      afterBatchMove(items);
      finish();
    });
  }
  return placeAt(destination.items[0], destination.toIndex, finish);
};
const relocateSummary = (id: string) => {
  const node = nodeCache.get(id)?.node;
  if (node?.kind !== 'message') return '原消息';
  const text = node.plainText.replace(/\s+/g, ' ').trim();
  return `${node.speakerName.slice(0, 24) || '未命名角色'}：${text.slice(0, 40) || '（无文字内容）'}${text.length > 40 ? '…' : ''}`;
};
const pickRelocateTarget = (targetId: string | null) => {
  if (!targetId || busy.value || relocateDialog || !relocateDestination(targetId)) return;
  relocation.value.targetMessageId = targetId;
  const sources = relocation.value.sourceMessageIds;
  const confirmation = dialog.warning({
    title: sources.length > 1 ? '确认批量重定位' : '确认重定位',
    content: sources.length > 1
      ? `将选中的 ${sources.length} 条消息按原有顺序移动到「${relocateSummary(targetId)}」之后？`
      : `将「${relocateSummary(sources[0])}」移动到「${relocateSummary(targetId)}」之后？`,
    positiveText: '确认重定位',
    negativeText: '取消',
    onPositiveClick: confirmRelocate,
    onEsc: cancelRelocate,
    onAfterLeave: () => {
      if (relocateDialog !== confirmation) return;
      relocateDialog = null;
      relocation.value.targetMessageId = null;
    },
  });
  relocateDialog = confirmation;
};
watch([sectionId, projectId, () => JSON.stringify(currentSection.value?.source), previewPublish], () => {
  cancelRelocate();
  relocateScrollId = null;
  // 多选属于当前片段的瞬时状态：切换片段、来源、项目或进入发布预览时整体退出。
  closeBatchDialog();
  selection.exitMulti();
}, { flush: 'sync' });
watch(win.items, () => {
  if (!relocation.value.active) return;
  if (relocation.value.sourceMessageIds.some((id) => !win.byId(id)?.capabilities.reorder)) cancelRelocate();
  else if (relocation.value.targetMessageId && !win.byId(relocation.value.targetMessageId)) {
    relocateDialog?.destroy();
    relocateDialog = null;
    relocation.value.targetMessageId = null;
  }
});

// ---- 多选模式：工作台局部瞬时状态；批量命令按整个选中集合一次提交，不做部分执行 ----
const multiMode = computed(() => selection.multi.value);
const selectedSet = computed(() => new Set(selection.messageIds.value));
const multiAtTop = computed(() => !planBatchStep(windowOrder.value, selectedSet.value, -1));
const multiAtBottom = computed(() => !planBatchStep(windowOrder.value, selectedSet.value, 1));
let batchDialog: ReturnType<typeof dialog.warning> | null = null;
const closeBatchDialog = () => {
  batchDialog?.destroy();
  batchDialog = null;
};
const openBatchDialog = (kind: 'warning' | 'error', options: Parameters<typeof dialog.warning>[0]) => {
  closeBatchDialog();
  const instance = dialog[kind]({
    ...options,
    onAfterLeave: () => {
      if (batchDialog === instance) batchDialog = null;
    },
  });
  batchDialog = instance;
};
const enterMulti = (seedId?: string | null) => {
  if (previewPublish.value) return;
  cancelRelocate();
  selection.enterMulti(seedId);
};
const exitMulti = () => {
  cancelRelocate();
  closeBatchDialog();
  selection.exitMulti();
};

const batchTarget = (items: LogDesignMessage[]) => ({
  channelId: sourceChannelId.value,
  items: items.map((it) => ({ messageId: it.message.id, expectedVersion: it.version })),
});
const afterBatchMove = (items: LogDesignMessage[]) => {
  items.forEach((it) => win.markOwnWrite(it.message.id, it.version + 1));
  void win.load();
};
const canBatch = (items: LogDesignMessage[], key: 'reorder' | 'remove') => items.length > 0 && items.length <= 200 && items.every((it) => it.capabilities[key]);

/** 整体上移/下移一位：每个连续段与相邻的未选消息交换，直接执行。 */
const moveMulti = (dir: -1 | 1) => {
  const items = selectedMessages.value;
  if (busy.value || !canBatch(items, 'reorder')) return;
  const plan = planBatchStep(windowOrder.value, selectedSet.value, dir);
  if (!plan) return;
  return runCommand('移动', () => relocateMessagesBatch(batchTarget(items), plan), () => afterBatchMove(items));
};

const excludeMulti = () => {
  const items = selectedMessages.value.slice();
  if (busy.value || !items.length || items.length > 200 || !caps.value.manageSources) return;
  openBatchDialog('warning', {
    title: '从本 LOG 排除',
    content: `将选中的 ${items.length} 条消息从当前 LOG 中排除。只从当前 LOG 排除，不影响聊天原消息；本次会话内可在“发布”面板撤销。`,
    positiveText: `排除 ${items.length} 条`,
    negativeText: '取消',
    onPositiveClick: () => excludeSelected(items),
  });
};

const removeMulti = () => {
  const items = selectedMessages.value.slice();
  if (busy.value || !canBatch(items, 'remove')) return;
  openBatchDialog('error', {
    title: '删除聊天原消息',
    content: `将从聊天记录中删除选中的 ${items.length} 条原消息。这不只是从 LOG 移除：频道中的这些消息也会被删除，且无法在这里恢复。若只想不在 LOG 中显示，请改用“排除”。`,
    positiveText: `删除 ${items.length} 条原消息`,
    negativeText: '取消',
    onPositiveClick: () => runCommand('删除', () => removeMessagesBatch(batchTarget(items)), () => {
      items.forEach((it) => win.removeLocal(it.message.id));
      selection.clear();
      win.scheduleReload(300);
      toast.success(`已删除 ${items.length} 条原消息`);
    }),
  });
};

const selectAllMulti = () => {
  selection.selectAll();
  if (!win.complete.value) toast.info(`已选中已加载的 ${selection.messageIds.value.length} 条消息；本片段尚未完整加载`);
};

// ---- 右键菜单：原消息上的轻量入口 ----
const ctxMenu = ref({ show: false, x: 0, y: 0, nodeId: '' });
const ctxOptions = computed<DropdownOption[]>(() => {
  const item = win.byId(ctxMenu.value.nodeId);
  if (!item) return [];
  if (multiMode.value) {
    return [
      { key: 'toggle', label: selection.selected.value.includes(item.message.id) ? '取消选中此消息' : '选中此消息' },
      { type: 'divider', key: 'd1' },
      { key: 'exit', label: '退出多选' },
    ];
  }
  return [
    { key: 'multi', label: '多选（从此消息开始）' },
    { type: 'divider', key: 'd1' },
    { key: 'edit', label: '编辑正文', disabled: !item.capabilities.edit },
    { key: 'relocate', label: '重定位…', disabled: busy.value || !item.capabilities.reorder },
  ];
});
const openContextMenu = (nodeId: string, x: number, y: number) => {
  if (previewPublish.value || !win.byId(nodeId)) return;
  ctxMenu.value = { show: true, x, y, nodeId };
};
const onContextSelect = (key: string) => {
  const id = ctxMenu.value.nodeId;
  ctxMenu.value.show = false;
  if (!win.byId(id)) return;
  if (key === 'multi') enterMulti(id);
  else if (key === 'toggle') selection.select(id);
  else if (key === 'exit') exitMulti();
  else if (key === 'edit') {
    selection.select(id);
    openEditor(id, 'content');
  } else if (key === 'relocate') startRelocate([id]);
};

const removeSelected = () => {
  const item = selectedMessages.value[0];
  if (!item) return;
  dialog.error({
    title: '删除原消息',
    content: '这会从聊天记录中删除这条原消息（不仅是 LOG），删除后无法在这里恢复。若只想不在 LOG 中显示，请使用“仅从本 LOG 排除”。',
    positiveText: '确认删除原消息',
    negativeText: '取消',
    onPositiveClick: () => runCommand('删除', () => removeMessage(target(item)), () => {
      win.removeLocal(item.message.id);
      selection.clear();
      win.scheduleReload(300);
    }),
  });
};

/** 本会话内的排除记录（服务端不提供排除列表读取接口，仅能撤销本会话内的操作）。 */
const sessionExclusions = ref<Array<{ sectionId: string; ids: string[]; label: string }>>([]);
const setExcluded = async (sectionIdValue: string, ids: string[], excluded: boolean) => {
  busy.value = true;
  try {
    await draftCtl.runVersioned(async (expected) => {
      const r = await updateLogExclusions(projectId.value, sectionIdValue, expected, ids, excluded);
      return { value: r, draftVersion: r.draftVersion };
    });
    return true;
  } catch (error) {
    toast.error(describeLogError(error, excluded ? '排除失败' : '恢复失败'));
    return false;
  } finally {
    busy.value = false;
  }
};
const excludeSelected = async (items: LogDesignMessage[] = selectedMessages.value) => {
  const sec = querySection.value;
  if (!items.length || !sec) return;
  const ids = items.map((it) => it.message.id);
  if (await setExcluded(sec.id, ids, true)) {
    const first = nodeCache.get(ids[0])?.node;
    sessionExclusions.value = [...sessionExclusions.value, { sectionId: sec.id, ids, label: `${ids.length} 条（${first?.speakerName || ''}：${(first?.plainText || '').slice(0, 16)}…）` }];
    selection.clear();
    void win.load();
    toast.success('已从本 LOG 排除，聊天原消息不受影响');
  }
};
const undoExclusion = async (index: number) => {
  const entry = sessionExclusions.value[index];
  if (!entry) return;
  if (await setExcluded(entry.sectionId, entry.ids, false)) {
    sessionExclusions.value = sessionExclusions.value.filter((_, i) => i !== index);
    void win.load();
  }
};

// ---- 元素插入 ----
const insertShow = ref(false);
const insertAnchor = ref<LogAnchor | null>(null);
const insertOptions = computed(() => LOG_ELEMENT_TYPES.map((type) => {
  const a = elementAvailability(type, activePkg.value);
  return { type, disabled: !a.templateId, reason: a.reason };
}));
const requestInsert = (anchor: LogAnchor) => {
  insertAnchor.value = anchor;
  insertShow.value = true;
};
const insertAtSelection = (side: 'before' | 'after') => {
  const item = selectedMessages.value[0];
  const sec = querySection.value;
  if (!item || !sec) return;
  requestInsert({ kind: 'message', sectionId: sec.id, messageId: item.message.id, side });
};
const insertAtMessage = (messageId: string, side: 'before' | 'after') => {
  const sec = querySection.value;
  if (!sec || !win.byId(messageId)) return;
  selection.select(messageId);
  requestInsert({ kind: 'message', sectionId: sec.id, messageId, side });
};
const doInsert = (type: LogElementType, image: LogImageRef | null) => {
  const anchor = insertAnchor.value;
  if (!anchor) return;
  let createdId = '';
  draftCtl.update((d) => {
    const el = createElement(d, type, anchor, activePkg.value, image);
    if (el) {
      d.elements.push(el);
      createdId = el.id;
    }
  });
  insertShow.value = false;
  if (createdId) {
    focusElementId.value = createdId;
    selection.select(`el-${createdId}`);
    openPanel('elements');
  }
};
const removeElement = (id: string) => {
  draftCtl.update((d) => {
    d.elements = d.elements.filter((e) => e.id !== id);
  });
  selection.clear();
};

// ---- iframe 事件 ----
const onFrameEvent = (event: FrameEvent) => {
  // iframe 内的点击不会冒泡到外层，菜单在画布有任何交互时关闭。
  if (ctxMenu.value.show && event.type !== 'progress' && event.type !== 'context') ctxMenu.value.show = false;
  if ((relocation.value.active || selection.multi.value) && ['imageResize', 'open', 'drop', 'action'].includes(event.type)) return;
  switch (event.type) {
    case 'imageResize':
      if (caps.value.editDraft && orderedIds.value.includes(event.nodeId)) {
        draftCtl.update((d) => updateDraftImageWidth(d, event));
      }
      break;
    case 'select':
      if (relocation.value.active) {
        pickRelocateTarget(event.nodeId);
        break;
      }
      selection.select(event.nodeId, { shift: event.shift, additive: event.additive });
      if (!selection.multi.value && event.nodeId?.startsWith('el-')) focusElementId.value = event.nodeId.slice(3);
      break;
    case 'context':
      if (!relocation.value.active) openContextMenu(event.nodeId, event.x, event.y);
      break;
    case 'open':
      if (event.nodeId.startsWith('el-')) {
        focusElementId.value = event.nodeId.slice(3);
        openPanel('elements');
      } else {
        const item = win.byId(event.nodeId);
        if (item?.capabilities.edit) openEditor(event.nodeId, 'content');
      }
      break;
    case 'drop':
      onDrop(event.nodeId, event.targetId, event.side);
      break;
    case 'action': {
      const item = win.byId(event.nodeId);
      if (!item) break;
      if (event.action === 'edit') {
        selection.select(event.nodeId);
        if (item.capabilities.edit) openEditor(event.nodeId, 'content');
      } else if (event.action === 'insert-before') {
        if (caps.value.editDraft) insertAtMessage(event.nodeId, 'before');
      } else if (event.action === 'insert-after') {
        if (caps.value.editDraft) insertAtMessage(event.nodeId, 'after');
      }
      break;
    }
    case 'key':
      if (event.key === 'Escape') onEscape();
      break;
    case 'scrollMiss':
      if (win.nextCursor.value) toast.info('目标位置尚未加载，请先加载更多消息');
      break;
    default:
      break;
  }
};
/** Esc 分层：先关菜单；正在选择重定位目标时只取消重定位（保留多选）；多选时退出多选；否则清除选择。 */
const onEscape = () => {
  if (ctxMenu.value.show) ctxMenu.value.show = false;
  else if (relocation.value.active) cancelRelocate();
  else if (batchDialog || editorShow.value || insertShow.value) return;
  else if (selection.multi.value) exitMulti();
  else selection.clear();
};
const onGlobalKey = (e: KeyboardEvent) => {
  if (e.key === 'Escape') onEscape();
};
window.addEventListener('keydown', onGlobalKey);

const locate = (nodeId: string) => {
  // 多选模式下定位只滚动，不改变选中集合。
  if (!selection.multi.value) selection.select(nodeId);
  previewRef.value?.scrollTo(nodeId, true);
};

// ---- 定位搜索（只在已加载窗口内） ----
const searchText = ref('');
let searchCursor = -1;
const runSearch = () => {
  const q = searchText.value.trim().toLowerCase();
  if (!q) return;
  const nodes = composition.value?.nodes || [];
  for (let step = 1; step <= nodes.length; step += 1) {
    const i = (searchCursor + step) % nodes.length;
    const n = nodes[i];
    const text = n.kind === 'message' ? `${n.speakerName} ${n.plainText}` : Object.values(n.fields).join(' ');
    if (text.toLowerCase().includes(q)) {
      searchCursor = i;
      locate(n.id);
      return;
    }
  }
  toast.info(win.nextCursor.value ? '已加载部分未找到，可加载更多后再搜索' : '未找到');
};
watch(searchText, () => {
  searchCursor = -1;
});

// ---- 草稿字段 ----
const titleModel = computed({
  get: () => draft.value.title,
  set: (v: string) => draftCtl.update((d) => {
    d.title = v.slice(0, 200);
  }),
});
// 单来源章节直接以章节出现；多来源章节分组列出其片段，选择的仍是来源片段。
const sectionOptions = computed(() => chapters.value.map((c, i) => (c.sections.length === 1
  ? { label: chapterTitle(c, i), value: c.sections[0].id }
  : {
      type: 'group' as const,
      label: chapterTitle(c, i),
      key: c.key,
      children: c.sections.map((s) => ({ label: sectionLabel(s, sourceChannelName), value: s.id })),
    })));
const sectionsShow = ref(false);
const exportShow = ref(false);
const pdfShow = ref(false);

// ---- 浏览器打印（后台 PDF 不可用时的降级路径）：完整加载当前来源片段并切到纸张模式后复用预览打印 ----
const onLayoutError = (message: string, revision: number) => {
  if (revision < frameRevision) return;
  layoutError.value = message;
  if (!printLayoutWaiter) return;
  const waiter = printLayoutWaiter;
  printLayoutWaiter = null;
  waiter.reject(new Error(message || '打印排版失败'));
};
const waitForPrintLayout = (targetRevision: number, timeoutMs = 120000) => new Promise<FrameLayoutResult>((resolve, reject) => {
  const timer = setTimeout(() => {
    printLayoutWaiter = null;
    reject(new Error('等待排版超时，请稍后重试'));
  }, timeoutMs);
  printLayoutWaiter = {
    targetRevision,
    resolve: (r) => { clearTimeout(timer); resolve(r); },
    reject: (e) => { clearTimeout(timer); reject(e); },
  };
});
const printShow = ref(false);
const printBusy = ref(false);
const printError = ref('');
const printPageCount = computed(() => layoutInfo.value?.pageCount || 0);
const loadWholeSectionForPrint = async () => {
  const sectionAtStart = querySection.value?.id;
  while (!win.complete.value) {
    if (querySection.value?.id !== sectionAtStart) throw new Error('打印准备已取消：来源片段已切换');
    if (win.error.value) throw new Error(win.error.value);
    if (win.loading.value) {
      await new Promise<void>((resolve) => {
        const stopWatch = watch(win.loading, (busy) => {
          if (busy) return;
          stopWatch();
          resolve();
        });
      });
      continue;
    }
    if (win.nextCursor.value === null) break;
    await win.loadMore();
  }
  if (win.error.value) throw new Error(win.error.value);
};
const startBrowserPrint = async () => {
  if (printBusy.value || !previewRef.value) return;
  printBusy.value = true;
  printError.value = '';
  try {
    await loadWholeSectionForPrint();
    // 先排空 Vue 的数据监听，再同步刷新 debounce 中的文档。旧 Frame 的结果不能完成此 waiter。
    await nextTick();
    if (buildTimer) clearTimeout(buildTimer);
    buildTimer = null;
    if (viewMode.value === 'flow') viewMode.value = 'single';
    rebuild(true);
    if (!frame.value) throw new Error('当前来源片段没有可打印页面');
    const result = await waitForPrintLayout(frame.value.revision);
    if (!result?.pageCount) throw new Error('当前来源片段没有可打印页面');
  } catch (error) {
    printLayoutWaiter = null;
    printError.value = error instanceof Error ? error.message : String(error);
  } finally {
    printBusy.value = false;
  }
  printShow.value = true;
};
const runPrint = async (range?: FramePrintRange) => {
  if (printBusy.value || !previewRef.value) return;
  printBusy.value = true;
  printError.value = '';
  try {
    await previewRef.value.print(range);
  } catch (error) {
    printError.value = error instanceof Error ? error.message : String(error);
  } finally {
    printBusy.value = false;
  }
};

const saveStatusText = computed(() => ({
  saved: '已保存',
  dirty: '未保存',
  saving: '保存中…',
  error: '保存失败',
  conflict: '冲突',
}[draftCtl.status.value]));

const downloadLocalDraft = () => downloadJson(`log-draft-${projectId.value}-v${draftCtl.draftVersion.value}.json`, draftCtl.localSnapshotJson());

const zoomSteps = [0.5, 0.75, 1, 1.25, 1.5, 2];
const zoomOptions = zoomSteps.map((z) => ({ label: `${Math.round(z * 100)}%`, value: z }));
const stepZoom = (dir: -1 | 1) => {
  const next = dir > 0 ? zoomSteps.find((z) => z > zoom.value + 1e-6) : [...zoomSteps].reverse().find((z) => z < zoom.value - 1e-6);
  if (next) zoom.value = next;
};

const issueCount = computed(() => (layoutInfo.value?.warnings.length || 0) + (layoutInfo.value?.missingAssets || 0) + (frame.value?.issues.length || 0));

// ---- 工作区底色跟随外层主题（含暗色与自定义主题） ----
const canvasRef = ref<HTMLElement | null>(null);
const canvasTone = shallowRef<{ color: string; dark: boolean; accent: string } | null>(null);
/** 将任意 CSS 颜色（含 color-mix 结果）规范为 rgb()，供 iframe 校验后使用。 */
const readCanvasTone = () => {
  const el = canvasRef.value;
  if (!el) return;
  const css = getComputedStyle(el).backgroundColor;
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true });
  if (!ctx || !css) return;
  ctx.fillStyle = css;
  ctx.fillRect(0, 0, 1, 1);
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
  const color = `rgb(${r}, ${g}, ${b})`;
  ctx.fillStyle = getComputedStyle(el).getPropertyValue('--log-wb-accent').trim();
  ctx.fillRect(0, 0, 1, 1);
  const [ar, ag, ab] = ctx.getImageData(0, 0, 1, 1).data;
  const accent = `rgb(${ar}, ${ag}, ${ab})`;
  const dark = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255 < 0.45;
  if (canvasTone.value?.color !== color || canvasTone.value.dark !== dark || canvasTone.value.accent !== accent) canvasTone.value = { color, dark, accent };
};
let themeObserver: MutationObserver | null = null;
onMounted(() => {
  readCanvasTone();
  themeObserver = new MutationObserver(readCanvasTone);
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-display-palette', 'data-custom-theme', 'style', 'class'] });
});
const goWorld = () => router.push({ name: 'world-detail', params: { worldId: props.context.worldId } });

onBeforeUnmount(() => {
  cancelRelocate();
  closeBatchDialog();
  if (buildTimer) clearTimeout(buildTimer);
  themeObserver?.disconnect();
  window.removeEventListener('keydown', onGlobalKey);
  onResizeEnd();
});
</script>

<template>
  <div class="log-wb" :class="{ 'is-narrow': narrow }">
    <header class="log-wb__bar" role="toolbar" aria-label="LOG 排版工具栏">
      <div class="log-wb__group log-wb__group--doc">
        <button type="button" class="log-wb__icon-btn" title="返回世界" aria-label="返回世界" @click="goWorld">
          <n-icon :component="ArrowLeft" size="18" />
        </button>
        <span class="log-wb__badge" aria-hidden="true">LOG</span>
        <n-input
          v-model:value="titleModel"
          class="log-wb__title"
          size="small"
          :bordered="false"
          :disabled="!caps.editDraft"
          maxlength="200"
          placeholder="LOG 名称"
          aria-label="LOG 名称"
        />
        <span class="log-wb__crumb" aria-hidden="true">/</span>
        <n-select
          v-model:value="sectionId"
          class="log-wb__section"
          size="small"
          :bordered="false"
          :consistent-menu-width="false"
          :options="sectionOptions"
          placeholder="无章节"
          aria-label="章节"
        />
        <button type="button" class="log-wb__text-btn" @click="sectionsShow = true">
          <n-icon :component="Stack2" size="16" />
          <span>{{ caps.manageSources ? '章节与来源' : '章节结构' }}</span>
        </button>
      </div>

      <div class="log-wb__group log-wb__group--view">
        <div class="log-wb__seg" role="radiogroup" aria-label="预览方式">
          <button
            v-for="m in viewModes"
            :key="m.value"
            type="button"
            role="radio"
            :aria-checked="viewMode === m.value"
            :class="{ 'is-on': viewMode === m.value }"
            @click="viewMode = m.value"
          >
            {{ m.label }}
          </button>
        </div>
        <div class="log-wb__seg" role="radiogroup" aria-label="预览发布效果">
          <button type="button" role="radio" :aria-checked="!previewPublish" :class="{ 'is-on': !previewPublish }" title="设计视图：显示选择与快捷操作" @click="previewPublish = false">
            <n-icon :component="Pencil" size="14" /><span>设计</span>
          </button>
          <button
            type="button"
            role="radio"
            :aria-checked="previewPublish"
            :class="{ 'is-on': previewPublish }"
            :title="currentIsFragment ? '当前片段的发布版式；公开章节会按编排串联全部片段' : '发布效果：公开读者看到的版式'"
            @click="previewPublish = true"
          >
            <n-icon :component="Eye" size="14" /><span>{{ currentIsFragment ? '片段发布效果' : '发布效果' }}</span>
          </button>
        </div>
        <button
          type="button"
          class="log-wb__text-btn log-wb__multi-btn"
          :class="{ 'is-on': multiMode }"
          :aria-pressed="multiMode"
          :disabled="previewPublish || !sections.length"
          :title="previewPublish ? '在设计视图中使用多选' : multiMode ? '退出多选 (Esc)' : '多选消息：批量移动、重定位、排除与删除（也可右键消息进入）'"
          @click="multiMode ? exitMulti() : enterMulti()"
        >
          <n-icon :component="ListCheck" size="16" /><span>多选</span>
        </button>
      </div>

      <div class="log-wb__group log-wb__group--actions">
        <n-input
          v-model:value="searchText"
          class="log-wb__search"
          size="small"
          clearable
          placeholder="定位（已加载范围）"
          aria-label="定位搜索"
          @keydown.enter="runSearch"
        >
          <template #prefix><n-icon :component="Search" size="15" /></template>
        </n-input>
        <span class="log-wb__save" :class="`is-${draftCtl.status.value}`" :title="draftCtl.lastError.value || `草稿 v${draftCtl.draftVersion.value}`">
          <i class="log-wb__save-dot" aria-hidden="true" />{{ saveStatusText }}
        </span>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button size="small" quaternary :circle="narrow" aria-label="导出" @click="exportShow = true">
              <template #icon><n-icon :component="Download" /></template>
              <span v-if="!narrow">导出</span>
            </n-button>
          </template>
          按当前 LOG 编排顺序导出传统格式
        </n-tooltip>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button size="small" quaternary :circle="narrow" aria-label="导出 PDF" :disabled="!sections.length" @click="pdfShow = true">
              <template #icon><n-icon :component="Printer" /></template>
              <span v-if="!narrow">PDF</span>
            </n-button>
          </template>
          后台生成整份 LOG 的 PDF，或使用浏览器打印当前片段
        </n-tooltip>
        <n-button
          size="small"
          type="primary"
          class="log-wb__publish"
          :disabled="!caps.publish"
          :title="caps.publish ? '发布前检查与发布' : '仅世界管理者可以发布'"
          @click="openPanel('publish')"
        >
          <template #icon><n-icon :component="Rocket" /></template>
          发布
        </n-button>
        <button
          type="button"
          class="log-wb__icon-btn"
          :class="{ 'is-on': panelVisible }"
          :title="panelVisible ? '收起设计面板' : '展开设计面板'"
          aria-label="设计面板"
          :aria-expanded="panelVisible"
          @click="panelVisible ? hidePanel() : openPanel(panelTab)"
        >
          <n-icon :component="LayoutSidebarRight" size="18" />
        </button>
      </div>
    </header>

    <div class="log-wb__notices">
      <n-alert v-if="draftCtl.status.value === 'conflict'" type="error" title="草稿冲突">
        {{ draftCtl.lastError.value }}。不会自动覆盖服务器上的版本。
        <n-space size="small" style="margin-top: 6px">
          <n-button size="small" @click="downloadLocalDraft">下载本地草稿 JSON</n-button>
          <n-button size="small" type="primary" @click="emit('reload')">重新加载</n-button>
        </n-space>
      </n-alert>
      <n-alert v-else-if="draftCtl.status.value === 'error'" type="warning" :show-icon="false">
        {{ draftCtl.lastError.value }}
        <n-button size="tiny" style="margin-left: 8px" @click="draftCtl.save()">重试保存</n-button>
      </n-alert>
      <n-alert v-if="win.error.value" type="error" :show-icon="false">
        {{ win.error.value }}
        <n-button size="tiny" style="margin-left: 8px" @click="win.load()">重试</n-button>
      </n-alert>
      <n-alert v-if="styleError" type="error" :show-icon="false">{{ styleError }}</n-alert>
      <n-alert v-if="win.staleHint.value && !win.loading.value" type="info" :show-icon="false">来源消息已在别处变化，已重新加载。</n-alert>
    </div>

    <div class="log-wb__main">
      <section ref="canvasRef" class="log-wb__canvas" :class="{ 'is-multi': multiMode }" aria-label="纸张预览">
        <div v-if="!sections.length" class="log-wb__empty">
          <n-icon :component="Book2" size="40" class="log-wb__empty-icon" />
          <div class="log-wb__empty-title">还没有章节</div>
          <p class="log-wb__empty-text">{{ caps.manageSources ? '添加一个章节并选择来源频道后，消息会在这里排版成页面。' : '请联系世界管理者设置来源频道。' }}</p>
          <n-button v-if="caps.manageSources" size="small" type="primary" @click="sectionsShow = true">添加章节来源</n-button>
        </div>
        <LogPreviewFrame
          v-else
          ref="previewRef"
          :doc="frame?.doc || null"
          :asset-tokens="frame?.assetTokens || []"
          :prepare-assets="assets.prepare"
          :mode="viewMode === 'flow' ? 'flow' : 'paged'"
          :spread="viewMode === 'spread'"
          :design="!previewPublish"
          :can-resize-images="caps.editDraft && !previewPublish"
          :relocation="frameRelocation"
          :multi="multiMode"
          :selection="selection.selected.value"
          :zoom="zoom"
          :canvas="canvasTone"
          :title="`${draft.title} 排版预览`"
          @frame-event="onFrameEvent"
          @near-end="onNearEnd"
          :frame-revision="frame?.revision || 0"
          @layout="onLayout"
          @layout-error="onLayoutError"
          @busy="(b: boolean) => (layoutBusy = b)"
        />

        <div class="log-wb__float log-wb__float--top">
          <Transition name="log-wb-pop" mode="out-in">
            <LogMultiSelectBar
              v-if="multiMode"
              key="multi"
              :messages="selectedMessages"
              :total="win.items.value.length"
              :complete="win.complete.value"
              :at-top="multiAtTop"
              :at-bottom="multiAtBottom"
              :can-manage-sources="caps.manageSources"
              :can-edit-draft="caps.editDraft"
              :template-ids="messageTemplates"
              :template-overrides="draft.messageTemplateOverrides || {}"
              :busy="busy"
              :range-mode="selection.rangeMode.value"
              :range-anchored="!!selection.rangeAnchor.value"
              :relocating="relocation.active"
              @select-all="selectAllMulti"
              @toggle-range="selection.toggleRangeMode()"
              @move="moveMulti"
              @relocate="startRelocate(selection.messageIds.value)"
              @cancel-relocate="cancelRelocate"
              @exclude="excludeMulti"
              @remove="removeMulti"
              @exit="exitMulti"
              @template="applyMessageTemplate"
            />
            <LogSelectionBar
              v-else-if="relocation.active || selection.selected.value.length"
              key="single"
              :messages="selectedMessages"
              :element="selectedElement"
              :can-edit-draft="caps.editDraft"
              :can-manage-sources="caps.manageSources"
              :busy="busy"
              :is-first="selectedIndex <= 0"
              :is-last="selectedIndex < 0 || selectedIndex >= win.items.value.length - 1"
              :relocating-count="relocation.active ? relocation.sourceMessageIds.length : 0"
              :template-ids="messageTemplates"
              :template-overrides="draft.messageTemplateOverrides || {}"
              @template="applyMessageTemplate"
              @edit="(tab) => selectedMessages[0] && openEditor(selectedMessages[0].message.id, tab)"
              @move="moveSelected"
              @relocate="startRelocate(selection.messageIds.value)"
              @cancel-relocate="cancelRelocate"
              @insert="insertAtSelection"
              @remove="removeSelected"
              @exclude="excludeSelected"
              @edit-element="openPanel('elements')"
              @remove-element="selectedElement && removeElement(selectedElement.id)"
              @multi="enterMulti()"
              @clear="selection.clear()"
            />
          </Transition>
        </div>

        <n-dropdown
          trigger="manual"
          placement="bottom-start"
          size="small"
          :show="ctxMenu.show"
          :x="ctxMenu.x"
          :y="ctxMenu.y"
          :options="ctxOptions"
          @select="onContextSelect"
          @clickoutside="ctxMenu.show = false"
        />

        <div v-if="studioPreview || (studioOpen && narrow && studioPeek)" class="log-wb__float log-wb__float--studio">
          <span v-if="studioPreview" class="log-wb__studio-badge">预览：样式设计器中的工作副本（未保存，不改变当前 LOG）</span>
          <n-button v-if="studioOpen && narrow && studioPeek" size="small" type="primary" @click="studioPeek = false">返回样式设计器</n-button>
        </div>

        <div v-if="sections.length" class="log-wb__float log-wb__float--zoom" role="group" aria-label="缩放">
          <button type="button" class="log-wb__icon-btn is-sm" :disabled="zoom <= zoomSteps[0]" title="缩小" aria-label="缩小" @click="stepZoom(-1)">
            <n-icon :component="Minus" size="15" />
          </button>
          <n-select v-model:value="zoom" class="log-wb__zoom-select" size="tiny" :bordered="false" :consistent-menu-width="false" :options="zoomOptions" aria-label="缩放比例" />
          <button type="button" class="log-wb__icon-btn is-sm" :disabled="zoom >= zoomSteps[zoomSteps.length - 1]" title="放大" aria-label="放大" @click="stepZoom(1)">
            <n-icon :component="Plus" size="15" />
          </button>
        </div>
      </section>

      <LogStyleStudio
        v-if="studioOpen"
        v-show="!(narrow && studioPeek)"
        ref="studioRef"
        class="log-wb__studio"
        :library="styleLibrary"
        :draft="draft"
        :bound-package="stylePkg"
        :can-bind="caps.editDraft"
        :bind="bindStyle"
        :request="studioRequest"
        :narrow="narrow"
        @close="onStudioClosed"
        @preview="(p: StyleStudioPreview | null) => (studioPreview = p)"
        @peek="studioPeek = true"
      />

      <div v-if="narrow && panelOpen && !studioOpen" class="log-wb__scrim" aria-hidden="true" @click="panelOpen = false" />

      <aside class="log-wb__inspector" :class="{ 'is-collapsed': !panelVisible || (narrow && studioOpen) }" aria-label="设计面板">
        <div v-show="panelVisible && !studioOpen" class="log-wb__dock" :style="narrow ? undefined : { width: `${panelWidth}px` }">
          <div
            v-if="!narrow"
            class="log-wb__resizer"
            role="separator"
            aria-orientation="vertical"
            aria-label="调整面板宽度"
            tabindex="0"
            @pointerdown.prevent="onResizeStart"
            @keydown="onResizeKey"
          />
          <div class="log-wb__dock-head">
            <div class="log-wb__dock-title">
              <strong>{{ panelMeta.label }}</strong>
              <span>{{ panelMeta.hint }}</span>
            </div>
            <button type="button" class="log-wb__icon-btn is-sm" title="收起设计面板" aria-label="收起设计面板" @click="hidePanel">
              <n-icon :component="X" size="16" />
            </button>
          </div>
          <div class="log-wb__dock-body" role="tabpanel" :aria-label="panelMeta.label">
            <LogBulkPanel
              v-if="panelTab === 'bulk'"
              :project-id="projectId"
              :world-id="context.worldId"
              :section="querySection"
              :messages="win.items.value"
              :can-bulk="caps.bulkAppearance"
              :draft="draft"
              :update="draftCtl.update"
              :can-edit-draft="caps.editDraft"
              :template-ids="messageTemplates"
              @applied="win.load()"
            />
            <LogElementsPanel
              v-else-if="panelTab === 'elements'"
              :draft="draft"
              :update="draftCtl.update"
              :style-package="activePkg"
              :anchor-states="composition?.anchorStates || null"
              :section-id="querySection?.id || ''"
              :messages="win.items.value"
              :complete="win.complete.value"
              :can-edit="caps.editDraft"
              :focus-element-id="focusElementId"
              :scope-label="fragmentHint ? '片段' : '章节'"
              @locate="locate"
              :library="assetLibrary"
              @request-insert="requestInsert"
            />
            <LogPagePanel v-else-if="panelTab === 'page'" :draft="draft" :update="draftCtl.update" :style-package="activePkg" :can-edit="caps.editDraft" :library="assetLibrary" />
            <LogStylePanel
              v-else-if="panelTab === 'style'"
              :draft="draft"
              :update="draftCtl.update"
              :style-package="stylePkg"
              :can-edit="caps.editDraft"
              :library="styleLibrary"
              :bind="bindStyle"
              @preview="(p: LogStylePackage | null) => (libraryPreview = p)"
              @open-studio="openStudio"
            />
            <LogAssetsPanel
              v-else-if="panelTab === 'assets'"
              :library="assetLibrary"
              @locate-element="openElementFromPublish"
              @open-page="openPanel('page')"
            />
            <LogPublishPanel
              v-else-if="panelTab === 'publish'"
              :context="context"
              :slug="slug"
              :draft-ctl="draftCtl"
              :sections="sections"
              :loaded-count="win.items.value.length"
              :complete="win.complete.value"
              :current-is-fragment="currentIsFragment"
              :anchor-states="composition?.anchorStates || null"
              :layout="layoutInfo"
              :template-issues="frame?.issues || []"
              :session-exclusions="sessionExclusions"
              @undo-exclusion="undoExclusion"
              @locate-element="openElementFromPublish"
              @reload="emit('reload')"
            />
          </div>
        </div>

        <nav class="log-wb__rail" role="tablist" aria-label="设计面板导航" aria-orientation="vertical">
          <button
            v-for="item in panelItems"
            :key="item.key"
            type="button"
            role="tab"
            class="log-wb__rail-btn"
            :class="{ 'is-on': panelVisible && !studioOpen && panelTab === item.key, 'is-current': panelTab === item.key }"
            :aria-selected="panelVisible && !studioOpen && panelTab === item.key"
            :title="item.hint"
            @click="onRailClick(item.key)"
          >
            <n-icon :component="item.icon" size="20" />
            <span>{{ item.label }}</span>
            <i v-if="item.key === 'publish' && issueCount" class="log-wb__rail-dot" aria-hidden="true" />
          </button>
        </nav>
      </aside>
    </div>

    <footer class="log-wb__status" role="status" aria-live="polite">
      <span class="log-wb__stat is-strong">{{ currentChapter ? chapterTitle(currentChapter, currentChapterIndex) : '—' }}</span>
      <span v-if="fragmentHint" class="log-wb__stat">片段：{{ fragmentHint }}</span>
      <span class="log-wb__stat">已加载 {{ win.items.value.length }} 条{{ win.loading.value ? '（加载中…）' : win.complete.value ? (fragmentHint ? '（本片段完整）' : '（本章完整）') : '' }}</span>
      <button v-if="win.nextCursor.value && !win.loading.value" type="button" class="log-wb__link-btn" @click="win.loadMore()">加载更多</button>
      <span v-if="layoutInfo" class="log-wb__stat">{{ viewMode === 'flow' ? '流式' : `共 ${layoutInfo.pageCount} 页${win.complete.value ? '' : '（未完整，页数暂不确定）'}` }}</span>
      <span v-if="layoutBusy" class="log-wb__stat is-busy">排版中…</span>
      <button v-if="layoutInfo?.warnings.length" type="button" class="log-wb__chip is-warn" title="查看发布前检查" @click="openPanel('publish')">
        <n-icon :component="AlertTriangle" size="13" />{{ layoutInfo.warnings.length }} 个排版警告
      </button>
      <button v-if="layoutInfo?.missingAssets" type="button" class="log-wb__chip is-warn" title="查看发布前检查" @click="openPanel('publish')">
        <n-icon :component="AlertTriangle" size="13" />{{ layoutInfo.missingAssets }} 个资源缺失（已用占位）
      </button>
      <button v-if="frame?.issues.length" type="button" class="log-wb__chip is-warn" :title="frame.issues.join('\n')" @click="openPanel('publish')">
        <n-icon :component="AlertTriangle" size="13" />{{ frame.issues.length }} 个模板问题
      </button>
      <span v-if="layoutError" class="log-wb__stat is-error">{{ layoutError }}</span>
      <span class="log-wb__spacer" />
      <span class="log-wb__stat">{{ Math.round(zoom * 100) }}%</span>
      <span class="log-wb__stat">草稿 v{{ draftCtl.draftVersion.value }}</span>
    </footer>

    <LogMessageEditor
      v-model:show="editorShow"
      v-model:tab="editorTab"
      :item="editorItem"
      :project-id="projectId"
      :section-id="querySection?.id || ''"
      :channel-id="sourceChannelId"
      @saved="onEditorSaved"
    />

    <LogStandardExportDialog
      v-if="exportShow"
      v-model:show="exportShow"
      :project-id="projectId"
      :draft-ctl="draftCtl"
    />

    <LogPdfExportDialog
      v-if="pdfShow"
      v-model:show="pdfShow"
      :project-id="projectId"
      :draft-ctl="draftCtl"
      :title="draft.title"
      @browser-print="startBrowserPrint"
    />
    <LogPrintDialog
      v-model:show="printShow"
      :page-count="printPageCount"
      :busy="printBusy"
      :error-text="printError"
      @print="runPrint"
    />

    <LogSectionsDialog
      v-model:show="sectionsShow"
      :draft="draft"
      :update="draftCtl.update"
      :world-id="context.worldId"
      :can-manage="caps.manageSources"
      :current-section-id="sectionId || null"
      @select="(id: string) => (sectionId = id)"
    />

    <LogInsertElementDialog
      v-model:show="insertShow"
      :options="insertOptions"
      :can-edit="caps.editDraft"
      :library="assetLibrary"
      @insert="doInsert"
    />
  </div>
</template>

<style scoped>
.log-wb {
  --log-wb-accent: var(--primary-color, #2f6fde);
  --log-wb-chrome: var(--sc-bg-elevated, #fff);
  --log-wb-line: var(--sc-border-strong, rgba(15, 23, 42, 0.12));
  --log-wb-line-soft: var(--sc-border-mute, rgba(15, 23, 42, 0.06));
  --log-wb-hover: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
  --log-wb-muted: var(--sc-text-secondary, #475569);
  --log-wb-canvas: color-mix(in srgb, var(--sc-bg-page, #f5f5f7) 84%, #8a8a8f);
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--sc-bg-surface, #f5f5f4);
  color: var(--sc-text-primary, #222);
  font-size: 13px;
}

/* ---- 顶部工具栏 ---- */
.log-wb__bar {
  position: relative;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--log-wb-line);
  background: var(--log-wb-chrome);
}

.log-wb__group {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.log-wb__group--doc {
  flex: 1 1 0;
}

.log-wb__group--view {
  flex: none;
  gap: 8px;
}

.log-wb__group--actions {
  flex: 1 1 0;
  justify-content: flex-end;
}

.log-wb__badge {
  flex: none;
  padding: 2px 6px;
  border-radius: 5px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: var(--log-wb-accent);
  background: color-mix(in srgb, var(--log-wb-accent) 12%, transparent);
}

/* 文档名与章节以“可编辑文字”呈现，悬停/聚焦时才显示底色。 */
.log-wb__title,
.log-wb__section :deep(.n-base-selection),
.log-wb__zoom-select :deep(.n-base-selection) {
  --n-color: transparent !important;
  --n-color-active: transparent !important;
  --n-color-focus: transparent !important;
  --n-color-disabled: transparent !important;
  background-color: transparent !important;
}

.log-wb__title {
  flex: 0 1 220px;
  min-width: 96px;
  font-weight: 600;
  border-radius: 6px;
  transition: background-color 0.12s ease;
}

.log-wb__title:hover,
.log-wb__title:focus-within,
.log-wb__section:hover {
  background-color: var(--log-wb-hover) !important;
}

.log-wb__title :deep(.n-input__input-el) {
  font-weight: 600;
}

.log-wb__crumb {
  flex: none;
  color: var(--log-wb-muted);
  opacity: 0.5;
}

.log-wb__section {
  flex: 0 1 170px;
  min-width: 90px;
  border-radius: 6px;
}

.log-wb__section :deep(.n-base-selection) {
  --n-color: var(--log-wb-hover) !important;
  --n-color-active: var(--log-wb-hover) !important;
  --n-color-focus: var(--log-wb-hover) !important;
  background-color: var(--log-wb-hover) !important;
  box-shadow: inset 0 0 0 1px var(--log-wb-line-soft);
  border-radius: 7px;
}

.log-wb__section:hover :deep(.n-base-selection),
.log-wb__section:focus-within :deep(.n-base-selection) {
  box-shadow: inset 0 0 0 1px var(--log-wb-line);
}

.log-wb__search {
  flex: 0 1 200px;
  min-width: 110px;
}

/* 无边框按钮：工具栏、面板与浮动控件共用 */
.log-wb__icon-btn,
.log-wb__text-btn,
.log-wb__link-btn,
.log-wb__chip,
.log-wb__seg > button,
.log-wb__rail-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease, box-shadow 0.12s ease;
}

.log-wb__icon-btn {
  flex: none;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  color: var(--log-wb-muted);
}

.log-wb__icon-btn.is-sm {
  width: 26px;
  height: 26px;
  border-radius: 6px;
}

.log-wb__icon-btn:hover:not(:disabled),
.log-wb__text-btn:hover {
  background: var(--log-wb-hover);
  color: var(--sc-text-primary, #222);
}

.log-wb__icon-btn.is-on {
  color: var(--log-wb-accent);
  background: color-mix(in srgb, var(--log-wb-accent) 12%, transparent);
}

.log-wb__icon-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.log-wb__text-btn {
  flex: none;
  height: 30px;
  padding: 0 9px;
  border-radius: 7px;
  color: var(--sc-text-primary, #222);
  background: var(--log-wb-hover);
  box-shadow: inset 0 0 0 1px var(--log-wb-line-soft);
}

.log-wb__text-btn .n-icon {
  color: var(--log-wb-accent);
}

.log-wb__text-btn:hover,
.log-wb__text-btn:focus-visible {
  background: color-mix(in srgb, var(--log-wb-accent) 9%, var(--log-wb-chrome));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--log-wb-accent) 30%, var(--log-wb-line));
}

.log-wb__text-btn:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--log-wb-accent) 38%, transparent);
  outline-offset: 1px;
}

.log-wb__seg {
  display: inline-flex;
  padding: 2px;
  border-radius: 9px;
  background: var(--log-wb-hover);
  box-shadow: inset 0 0 0 1px var(--log-wb-line-soft);
}

.log-wb__seg > button {
  height: 26px;
  padding: 0 11px;
  border-radius: 7px;
  color: var(--log-wb-muted);
}

.log-wb__seg > button:hover {
  color: var(--sc-text-primary, #222);
}

.log-wb__seg > button.is-on {
  color: var(--sc-text-primary, #222);
  font-weight: 600;
  background: var(--log-wb-chrome);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12), 0 0 0 1px var(--log-wb-line-soft);
}

.log-wb__save {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px;
  font-size: 12px;
  color: var(--log-wb-muted);
}

.log-wb__save-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #18a058;
}

.log-wb__save.is-dirty .log-wb__save-dot {
  background: #d99a1e;
}

.log-wb__save.is-saving .log-wb__save-dot {
  background: var(--log-wb-accent);
  animation: log-wb-pulse 1s ease-in-out infinite;
}

.log-wb__save.is-error,
.log-wb__save.is-conflict {
  color: #d03050;
  font-weight: 600;
}

.log-wb__save.is-error .log-wb__save-dot,
.log-wb__save.is-conflict .log-wb__save-dot {
  background: #d03050;
}

@keyframes log-wb-pulse {
  50% {
    opacity: 0.35;
  }
}

.log-wb__publish {
  flex: none;
}

/* ---- 提示条 ---- */
.log-wb__notices {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--log-wb-line-soft);
  background: var(--log-wb-chrome);
}

.log-wb__notices:empty {
  display: none;
}

/* ---- 主区：画布 + Inspector ---- */
.log-wb__main {
  flex: 1;
  min-height: 0;
  display: flex;
  position: relative;
}

.log-wb__canvas {
  flex: 1;
  min-width: 0;
  min-height: 0;
  position: relative;
  background: var(--log-wb-canvas);
}

.log-wb__canvas :deep(.log-preview-frame__busy) {
  top: auto;
  right: auto;
  bottom: 14px;
  left: 14px;
  color: var(--sc-text-primary, #444);
  background: var(--log-wb-chrome);
  box-shadow: 0 2px 10px rgba(15, 23, 42, 0.16), 0 0 0 1px var(--log-wb-line-soft);
}

.log-wb__empty {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  text-align: center;
}

.log-wb__empty-icon {
  color: var(--log-wb-muted);
  opacity: 0.6;
}

.log-wb__empty-title {
  font-size: 15px;
  font-weight: 600;
}

.log-wb__empty-text {
  max-width: 320px;
  margin: 0 0 6px;
  color: var(--log-wb-muted);
}

.log-wb__float {
  position: absolute;
  z-index: 4;
}

.log-wb__float--top {
  top: 12px;
  left: 12px;
  right: 12px;
  display: flex;
  justify-content: center;
  pointer-events: none;
  container-type: inline-size;
}

/* 多选模式：画布内缘一圈强调色细框，与单条编辑区分且不遮挡内容。 */
.log-wb__canvas.is-multi::after {
  content: '';
  position: absolute;
  inset: 0;
  z-index: 2;
  pointer-events: none;
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--log-wb-accent) 55%, transparent);
}

.log-wb__multi-btn.is-on {
  color: var(--log-wb-accent);
  font-weight: 600;
  background: color-mix(in srgb, var(--log-wb-accent) 13%, var(--log-wb-chrome));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--log-wb-accent) 40%, var(--log-wb-line));
}

.log-wb__multi-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.log-wb__float--top > * {
  pointer-events: auto;
}

.log-wb__float--studio {
  left: 50%;
  bottom: 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  max-width: calc(100% - 200px);
  transform: translateX(-50%);
}

.log-wb__studio-badge {
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 12px;
  text-align: center;
  color: var(--log-wb-accent);
  background: var(--log-wb-chrome);
  box-shadow: 0 2px 10px rgba(15, 23, 42, 0.16), 0 0 0 1px color-mix(in srgb, var(--log-wb-accent) 40%, transparent);
}

/* ---- Style Studio ---- */
.log-wb__studio {
  flex: 0 0 auto;
  width: clamp(520px, 60%, 1120px);
  border-left: 1px solid var(--log-wb-line);
}

.is-narrow .log-wb__studio {
  position: absolute;
  inset: 0;
  z-index: 7;
  width: auto;
  border-left: 0;
}

.is-narrow .log-wb__float--studio {
  max-width: calc(100% - 28px);
  bottom: 60px;
}

.log-wb__float--zoom {
  right: 14px;
  bottom: 14px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--log-wb-chrome);
  box-shadow: 0 2px 10px rgba(15, 23, 42, 0.16), 0 0 0 1px var(--log-wb-line-soft);
}

.log-wb__zoom-select {
  width: 72px;
}

.log-wb-pop-enter-active,
.log-wb-pop-leave-active {
  transition: opacity 0.14s ease, transform 0.14s ease;
}

.log-wb-pop-enter-from,
.log-wb-pop-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

/* ---- Inspector ---- */
.log-wb__inspector {
  position: relative;
  flex: none;
  display: flex;
  min-height: 0;
  background: var(--log-wb-chrome);
  border-left: 1px solid var(--log-wb-line);
}

.log-wb__dock {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  border-right: 1px solid var(--log-wb-line-soft);
}

.log-wb__resizer {
  position: absolute;
  left: -4px;
  top: 0;
  bottom: 0;
  width: 8px;
  cursor: col-resize;
  z-index: 2;
}

.log-wb__resizer::after {
  content: '';
  position: absolute;
  left: 3px;
  top: 0;
  bottom: 0;
  width: 2px;
  background: transparent;
  transition: background-color 0.12s ease;
}

.log-wb__resizer:hover::after,
.log-wb__resizer:focus-visible::after {
  background: var(--log-wb-accent);
}

.log-wb__resizer:focus-visible {
  outline: none;
}

.log-wb__dock-head {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 10px 10px 16px;
  border-bottom: 1px solid var(--log-wb-line-soft);
}

.log-wb__dock-title {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.log-wb__dock-title strong {
  font-size: 14px;
}

.log-wb__dock-title span {
  font-size: 12px;
  color: var(--log-wb-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-wb__dock-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 4px 16px 8px;
  scrollbar-gutter: stable;
}

.log-wb__rail {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 60px;
  padding: 8px 6px;
}

.log-wb__rail-btn {
  position: relative;
  flex-direction: column;
  gap: 3px;
  height: 52px;
  border-radius: 9px;
  font-size: 11px;
  color: var(--log-wb-muted);
}

.log-wb__rail-btn:hover {
  background: var(--log-wb-hover);
  color: var(--sc-text-primary, #222);
}

.log-wb__rail-btn.is-on {
  color: var(--log-wb-accent);
  font-weight: 600;
  background: color-mix(in srgb, var(--log-wb-accent) 12%, transparent);
}

.log-wb__rail-btn:last-child {
  margin-top: auto;
}

.log-wb__rail-dot {
  position: absolute;
  top: 8px;
  right: 12px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #d99a1e;
  box-shadow: 0 0 0 2px var(--log-wb-chrome);
}

.log-wb__icon-btn:focus-visible,
.log-wb__text-btn:focus-visible,
.log-wb__link-btn:focus-visible,
.log-wb__chip:focus-visible,
.log-wb__seg > button:focus-visible,
.log-wb__rail-btn:focus-visible {
  outline: 2px solid var(--log-wb-accent);
  outline-offset: -2px;
}

.log-wb__scrim {
  position: absolute;
  inset: 0;
  z-index: 5;
  background: rgba(15, 23, 42, 0.28);
}

/* ---- 状态栏 ---- */
.log-wb__status {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 28px;
  padding: 0 10px;
  font-size: 12px;
  border-top: 1px solid var(--log-wb-line);
  background: var(--log-wb-chrome);
  color: var(--log-wb-muted);
  overflow-x: auto;
  white-space: nowrap;
  scrollbar-width: none;
}

.log-wb__stat {
  padding: 0 6px;
}

.log-wb__stat + .log-wb__stat {
  border-left: 1px solid var(--log-wb-line-soft);
}

.log-wb__stat.is-strong {
  color: var(--sc-text-primary, #222);
  font-weight: 600;
}

.log-wb__stat.is-busy {
  color: var(--log-wb-accent);
}

.log-wb__stat.is-error {
  color: #d03050;
}

.log-wb__link-btn {
  height: 20px;
  padding: 0 6px;
  border-radius: 5px;
  color: var(--log-wb-accent);
}

.log-wb__link-btn:hover {
  background: color-mix(in srgb, var(--log-wb-accent) 12%, transparent);
}

.log-wb__chip {
  height: 20px;
  padding: 0 7px;
  border-radius: 999px;
  gap: 4px;
}

.log-wb__chip.is-warn {
  color: #a86a00;
  background: rgba(217, 154, 30, 0.14);
}

.log-wb__chip.is-warn:hover {
  background: rgba(217, 154, 30, 0.24);
}

.log-wb__spacer {
  flex: 1;
}

/* ---- 窄屏 ---- */
.is-narrow .log-wb__bar {
  flex-wrap: wrap;
  row-gap: 6px;
  padding: 6px 8px;
}

.is-narrow .log-wb__group--doc {
  flex: 1 1 100%;
  order: 1;
}

.is-narrow .log-wb__group--actions {
  flex: 1 1 auto;
  order: 2;
}

.is-narrow .log-wb__group--view {
  order: 3;
  flex: 1 1 100%;
  overflow-x: auto;
  scrollbar-width: none;
}

.is-narrow .log-wb__text-btn > span,
.is-narrow .log-wb__badge,
.is-narrow .log-wb__crumb {
  display: none;
}

.is-narrow .log-wb__search {
  flex: 1 1 120px;
}

.is-narrow .log-wb__inspector {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 6;
  flex-direction: column;
  width: min(420px, 100%);
  border-left: 0;
  box-shadow: -8px 0 28px rgba(15, 23, 42, 0.22);
}

.is-narrow .log-wb__inspector.is-collapsed {
  display: none;
}

.is-narrow .log-wb__dock {
  flex: 1;
  border-right: 0;
}

.is-narrow .log-wb__rail {
  order: -1;
  flex-direction: row;
  width: auto;
  padding: 6px 8px;
  border-bottom: 1px solid var(--log-wb-line-soft);
  overflow-x: auto;
}

.is-narrow .log-wb__rail-btn {
  flex: 1 0 56px;
  height: 46px;
}

.is-narrow .log-wb__rail-btn:last-child {
  margin-top: 0;
}

.is-narrow .log-wb__dock-body {
  padding: 4px 14px 8px;
}

@media (prefers-reduced-motion: reduce) {
  .log-wb * {
    transition: none !important;
    animation: none !important;
  }
}
</style>
