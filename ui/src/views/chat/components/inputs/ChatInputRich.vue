<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick, shallowRef, reactive } from 'vue';
import { useMessage } from 'naive-ui';
import type { MentionOption } from 'naive-ui';
import type { Editor } from '@tiptap/vue-3';
import { Plugin } from 'prosemirror-state';
import { loadTipTapBundle } from '@/utils/tiptap-loader';
import { listPlatformFonts } from '@/services/font/platformFontApi';
import { ensurePlatformFontLoaded } from '@/services/font/platformFontRegistry';
import { createPlatformFontSelectPreviewController } from '@/services/font/platformFontSelectPreview';
import { useChatStore } from '@/stores/chat';
import { useIFormStore } from '@/stores/iform';
import { useUtilsStore } from '@/stores/utils';
import { generateIFormEmbedLink } from '@/utils/iformEmbedLink';
import { matchText } from '@/utils/pinyinMatch';
import { contentUnescape } from '@/utils/tools';
import { plainTextToTiptapJson } from '@/utils/tiptap-render';
import type { PlatformFontAsset } from '@/services/font/platformFontTypes';
import {
  SMART_LINK_DATA_ATTR,
  SMART_LINK_IMAGE_ROLE_ATTR,
  SMART_LINK_NODE_TYPE,
  SMART_LINK_TEXT_IMAGE_ROLE,
  normalizeSmartLinkAttrs,
  resolveSmartLinkDisplayText,
  type SmartLinkTextType,
  type SmartLinkUrlType,
} from '@/utils/tiptapSmartLink';
import { normalizePerformanceEffect, type PerformanceEffect, type PerformanceEnterMode } from '@/utils/tiptap-performance-mark';
import type { PerformanceCommandType } from '@/utils/tiptap-performance-node';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  disabled?: boolean
  whisperMode?: boolean
  mentionOptions?: MentionOption[]
  mentionLoading?: boolean
  mentionPrefix?: (string | number)[]
  mentionRenderLabel?: (option: MentionOption) => any
  autosize?: boolean | { minRows?: number; maxRows?: number }
  rows?: number
  inputClass?: string | Record<string, boolean> | Array<string | Record<string, boolean>>
  inlineImages?: Record<string, { status: 'uploading' | 'uploaded' | 'failed'; previewUrl?: string; error?: string }>
  defaultIFormEmbedLink?: string
  surfaceVariant?: 'default' | 'sticky-note'
}>(), {
  modelValue: '',
  placeholder: '',
  disabled: false,
  whisperMode: false,
  mentionOptions: () => [],
  mentionLoading: false,
  mentionPrefix: () => ['@'],
  autosize: true,
  rows: 1,
  inputClass: () => [],
  inlineImages: () => ({}),
  defaultIFormEmbedLink: '',
  surfaceVariant: 'default',
});

type SmartLinkUploadSource = 'smart-link-text-image' | 'smart-link-url-image';
type TextDecorationLine = 'underline' | 'line-through';
type TextDecorationThickness = 'thin' | 'regular' | 'bold';
type TextDecorationPattern = 'solid' | 'dotted' | 'dense-dotted';
type TextDecorationWave = 'none' | 'soft' | 'heavy';
type TextDecorationCount = 'single' | 'double' | 'triple';
type TextDecorationSelectionState = 'none' | 'partial' | 'full';

interface TextDecorationStyleState {
  thickness: TextDecorationThickness;
  pattern: TextDecorationPattern;
  wave: TextDecorationWave;
  count: TextDecorationCount;
}

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'mention-search', value: string, prefix: string): void
  (event: 'mention-select', option: MentionOption): void
  (event: 'keydown', e: KeyboardEvent): void
  (event: 'focus'): void
  (event: 'blur'): void
  (event: 'paste-image', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-files', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-gallery-item', payload: { attachmentId: string; selectionStart: number; selectionEnd: number }): void
  (event: 'upload-button-click', source?: 'rich-editor' | SmartLinkUploadSource): void
  (event: 'composition-start'): void
  (event: 'composition-end'): void
}>();

const message = useMessage();
const chat = useChatStore();
const iform = useIFormStore();
const utils = useUtilsStore();
iform.bootstrap();

const editor = shallowRef<Editor | null>(null);
const editorElement = ref<HTMLElement | null>(null);
const isInitializing = ref(true);
const isFocused = ref(false);
const isSyncingFromProps = ref(false);
const editorStateVersion = ref(0);
const isComposing = ref(false);
const isMobile = ref(false);
const fontSelectorExpanded = ref(false);
const desktopFontSelectorExpanded = ref(false);
const isStickyNoteSurface = computed(() => props.surfaceVariant === 'sticky-note');
const toolbarPopoverContentClass = computed(() => (
  isStickyNoteSurface.value
    ? 'tiptap-toolbar-popover tiptap-toolbar-popover--sticky-note'
    : 'tiptap-toolbar-popover'
));
const toolbarPickerClass = computed(() => [
  'tiptap-toolbar-picker',
  { 'tiptap-toolbar-picker--sticky-note': isStickyNoteSurface.value },
]);
const platformFontSelectMenuClass = computed(() => (
  isStickyNoteSurface.value
    ? 'tiptap-platform-font-select__menu tiptap-platform-font-select__menu--sticky-note'
    : 'tiptap-platform-font-select__menu'
));
const platformFontSelectMenuProps = computed(() => ({
  class: platformFontSelectMenuClass.value,
}));
const savedEditorSelectionRange = ref<{ start: number; end: number } | null>(null);
const performanceTriggerRef = ref<HTMLElement | null>(null);
const MOBILE_BREAKPOINT = 768;
const RICH_CONTENT_PARSE_OPTIONS = { preserveWhitespace: 'full' as const };
const SILENT_SET_CONTENT_OPTIONS = { emitUpdate: false, parseOptions: RICH_CONTENT_PARSE_OPTIONS };

// Mention 面板状态
const mentionVisible = ref(false);
const mentionActiveIndex = ref(0);
const mentionTriggerInfo = ref<{ prefix: string; startPos: number; cursorPos: number } | null>(null);
const mentionSearchValue = ref('');
const mentionDropdownRef = ref<HTMLDivElement | null>(null);
const mentionDropdownStyle = ref<Record<string, string>>({});
const rootRef = ref<HTMLElement | null>(null);
let mentionPositionRaf: number | null = null;

const MENTION_TOKEN_REGEX = /<at\s+id=(['"])([^'"]*)\1(?:\s+name=(['"])(.*?)\3)?\s*\/?\s*>/g;
const GALLERY_ITEM_MIME_TYPE = 'application/x-sealchat-gallery-item';

const decodeMentionText = (value: string) => {
  return contentUnescape(value);
};

const encodeMentionAttr = (value: string) => {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;');
};

const buildMentionToken = (id: string, name: string) => {
  const safeId = encodeMentionAttr(id.trim());
  if (!safeId) {
    return '';
  }
  const safeName = encodeMentionAttr(name.trim());
  const nameAttr = safeName ? ` name="${safeName}"` : '';
  return `<at id="${safeId}"${nameAttr}/>`;
};

const splitTextWithMentionTokens = (text: string, marks?: any[]) => {
  const result: any[] = [];
  if (!text) {
    return result;
  }

  MENTION_TOKEN_REGEX.lastIndex = 0;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  const pushText = (value: string) => {
    if (!value) return;
    const node: any = { type: 'text', text: value };
    if (marks?.length) {
      node.marks = marks;
    }
    result.push(node);
  };

  while ((match = MENTION_TOKEN_REGEX.exec(text)) !== null) {
    if (match.index > lastIndex) {
      pushText(text.slice(lastIndex, match.index));
    }

    const id = decodeMentionText(match[2] || '').trim();
    const name = decodeMentionText(match[4] || '').trim();
    if (id) {
      result.push({
        type: 'satoriMention',
        attrs: {
          id,
          name,
        },
      });
    } else {
      pushText(match[0]);
    }

    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    pushText(text.slice(lastIndex));
  }

  if (!result.length) {
    pushText(text);
  }

  return result;
};

const normalizeMentionTokensInNode = (node: any): any[] => {
  if (!node || typeof node !== 'object') {
    return [node];
  }

  if (node.type === 'text' && typeof node.text === 'string') {
    return splitTextWithMentionTokens(node.text, node.marks);
  }

  const nextNode: any = { ...node };
  if (Array.isArray(node.content)) {
    const normalizedChildren: any[] = [];
    node.content.forEach((child: any) => {
      normalizedChildren.push(...normalizeMentionTokensInNode(child));
    });
    nextNode.content = normalizedChildren;
  }

  return [nextNode];
};

const normalizeMentionTokensInDoc = (json: any) => {
  if (!json || typeof json !== 'object') {
    return json;
  }
  const normalized = normalizeMentionTokensInNode(json);
  return normalized[0] ?? json;
};

const serializeMentionNodesInNode = (node: any): any[] => {
  if (!node || typeof node !== 'object') {
    return [node];
  }

  if (node.type === 'satoriMention') {
    const id = String(node.attrs?.id || '').trim();
    const name = String(node.attrs?.name || '').trim();
    const token = buildMentionToken(id, name);
    if (!token) {
      return [];
    }
    return [{ type: 'text', text: token }];
  }

  const nextNode: any = { ...node };
  if (Array.isArray(node.content)) {
    const serializedChildren: any[] = [];
    node.content.forEach((child: any) => {
      serializedChildren.push(...serializeMentionNodesInNode(child));
    });
    nextNode.content = serializedChildren;
  }

  return [nextNode];
};

const serializeMentionNodesToTokens = (json: any) => {
  if (!json || typeof json !== 'object') {
    return json;
  }
  const serialized = serializeMentionNodesInNode(json);
  return serialized[0] ?? json;
};

const parseIncomingRichContent = (value: string) => {
  const trimmed = String(value || '').trim();
  if (!trimmed) {
    return cloneEmptyDoc();
  }
  try {
    const json = JSON.parse(trimmed);
    return normalizeMentionTokensInDoc(json);
  } catch {
    return normalizeMentionTokensInDoc(plainTextToTiptapJson(value));
  }
};

const parseMentionOption = (option: MentionOption) => {
  const data = (option as any)?.data || {};
  const idFromData = String(data.userId || data.id || '').trim();
  const nameFromData = String(data.displayName || option.label || '').trim();
  if (idFromData) {
    return { id: idFromData, name: nameFromData };
  }

  const value = String(option.value || '');
  MENTION_TOKEN_REGEX.lastIndex = 0;
  const match = MENTION_TOKEN_REGEX.exec(value);
  if (!match) {
    return { id: '', name: '' };
  }
  return {
    id: decodeMentionText(match[2] || '').trim(),
    name: decodeMentionText(match[4] || '').trim(),
  };
};

const updateMentionDropdownPosition = () => {
  if (typeof window === 'undefined') {
    return;
  }
  const host = rootRef.value;
  if (!host) {
    return;
  }

  const rect = host.getBoundingClientRect();
  const safeWidth = Math.min(rect.width, window.innerWidth - 12);
  const safeLeft = Math.min(
    Math.max(6, rect.left),
    Math.max(6, window.innerWidth - safeWidth - 6),
  );
  const bottom = Math.max(0, window.innerHeight - rect.top + 6);

  mentionDropdownStyle.value = {
    position: 'fixed',
    left: `${safeLeft}px`,
    width: `${safeWidth}px`,
    bottom: `${bottom}px`,
    zIndex: '4200',
  };
};

const extractGalleryAttachmentId = (event: DragEvent) => {
  const dt = event.dataTransfer;
  if (!dt || !Array.from(dt.types || []).includes(GALLERY_ITEM_MIME_TYPE)) {
    return '';
  }
  try {
    const raw = dt.getData(GALLERY_ITEM_MIME_TYPE);
    if (!raw) {
      return '';
    }
    const payload = JSON.parse(raw) as { attachmentId?: string };
    return typeof payload?.attachmentId === 'string' ? payload.attachmentId : '';
  } catch (error) {
    console.warn('解析表情拖拽数据失败', error);
    return '';
  }
};

const scheduleMentionDropdownPosition = () => {
  if (typeof window === 'undefined') {
    return;
  }
  if (mentionPositionRaf !== null) {
    cancelAnimationFrame(mentionPositionRaf);
  }
  mentionPositionRaf = window.requestAnimationFrame(() => {
    mentionPositionRaf = null;
    updateMentionDropdownPosition();
  });
};

const handleMentionViewportChange = () => {
  if (mentionVisible.value) {
    scheduleMentionDropdownPosition();
  }
};

const getMentionOptionText = (option: MentionOption) => {
  const data = (option as any)?.data || {};
  const candidates = [
    option.label,
    option.value,
    data.displayName,
    data.userId,
    data.identityId,
  ]
    .filter(Boolean)
    .map((value) => String(value).toLowerCase());
  return candidates.join(' ');
};

const mentionFilteredOptions = computed(() => {
  const options = props.mentionOptions || [];
  const keyword = mentionSearchValue.value.trim();
  if (!keyword) {
    return options;
  }
  return options.filter((option) => matchText(keyword, getMentionOptionText(option)));
});

const closeMentionPanel = () => {
  mentionVisible.value = false;
  mentionTriggerInfo.value = null;
  mentionActiveIndex.value = 0;
  mentionSearchValue.value = '';
};

const rememberEditorSelection = () => {
  const ed = editor.value;
  if (!ed) {
    return;
  }
  const { from, to } = ed.state.selection;
  savedEditorSelectionRange.value = { start: from, end: to };
};

const restoreEditorSelection = () => {
  const ed = editor.value;
  const range = savedEditorSelectionRange.value;
  if (!ed || !range) {
    return;
  }
  const docSize = ed.state.doc.content.size;
  const safeStart = clamp(range.start, 0, docSize);
  const safeEnd = clamp(range.end, 0, docSize);
  ed.chain().focus().setTextSelection({ from: safeStart, to: safeEnd }).run();
};

const syncToolbarStateFromEditor = () => {
  const ed = editor.value;
  if (!ed) {
    selectedPlatformFontId.value = null;
    selectedFontSize.value = null;
    selectedBlockType.value = 'paragraph';
    return;
  }
  const attrs = ed.getAttributes('textStyle') as Record<string, any>;
  selectedPlatformFontId.value = typeof attrs?.fontAssetId === 'string' ? attrs.fontAssetId : null;
  selectedFontSize.value = typeof attrs?.fontSize === 'string' ? attrs.fontSize : null;
  if (ed.isActive('heading', { level: 1 })) {
    selectedBlockType.value = 'heading-1';
  } else if (ed.isActive('heading', { level: 2 })) {
    selectedBlockType.value = 'heading-2';
  } else if (ed.isActive('heading', { level: 3 })) {
    selectedBlockType.value = 'heading-3';
  } else {
    selectedBlockType.value = 'paragraph';
  }
  primePlatformFontPreview(selectedPlatformFontId.value);
};

const bumpEditorStateVersion = () => {
  editorStateVersion.value += 1;
  syncToolbarStateFromEditor();
};

const runEditorCommandWithSelection = (command: (chain: any) => void) => {
  const ed = editor.value;
  if (!ed) {
    return false;
  }
  restoreEditorSelection();
  const chain = ed.chain().focus();
  command(chain);
  const result = chain.run();
  rememberEditorSelection();
  bumpEditorStateVersion();
  return result;
};

const markToolbarPickerTriggerInteraction = (event: PointerEvent | MouseEvent) => {
  event.preventDefault();
  rememberEditorSelection();
  markOverlayInteraction();
};

const closeToolbarPopovers = () => {
  blockTypePopoverShow.value = false;
  fontSizePopoverShow.value = false;
  performancePopoverShow.value = false;
  underlineStylePopoverShow.value = false;
  strikeStylePopoverShow.value = false;
};

const scrollActiveMentionIntoView = () => {
  nextTick(() => {
    const container = mentionDropdownRef.value;
    if (!container) {
      return;
    }
    const items = container.querySelectorAll('.mention-dropdown__item');
    const target = items[mentionActiveIndex.value] as HTMLElement | undefined;
    if (target?.scrollIntoView) {
      target.scrollIntoView({ block: 'nearest' });
    }
  });
};

const handleMentionHover = (index: number) => {
  mentionActiveIndex.value = index;
  scrollActiveMentionIntoView();
};

const handleMentionSelect = (option: MentionOption) => {
  const ed = editor.value;
  if (!ed || !mentionTriggerInfo.value) return;

  const mention = parseMentionOption(option);
  if (!mention.id) {
    return;
  }

  const from = Math.max(1, mentionTriggerInfo.value.startPos);
  const to = Math.max(from, mentionTriggerInfo.value.cursorPos);

  ed.chain().focus().insertContentAt({ from, to }, [
    {
      type: 'satoriMention',
      attrs: {
        id: mention.id,
        name: mention.name,
      },
    },
    {
      type: 'text',
      text: ' ',
    },
  ]).run();
  emit('mention-select', option);
  closeMentionPanel();
};

const handleMentionKeydown = (event: KeyboardEvent): boolean => {
  if (!mentionVisible.value) {
    return false;
  }

  const optionsCount = mentionFilteredOptions.value.length;
  if (!optionsCount) {
    if (event.key === 'Escape') {
      event.preventDefault();
      closeMentionPanel();
      return true;
    }
    return false;
  }

  switch (event.key) {
    case 'ArrowUp':
      event.preventDefault();
      mentionActiveIndex.value = Math.max(0, mentionActiveIndex.value - 1);
      scrollActiveMentionIntoView();
      return true;
    case 'ArrowDown':
      event.preventDefault();
      mentionActiveIndex.value = Math.min(optionsCount - 1, mentionActiveIndex.value + 1);
      scrollActiveMentionIntoView();
      return true;
    case 'Enter':
    case 'Tab':
      event.preventDefault();
      const selectedOption = mentionFilteredOptions.value[mentionActiveIndex.value];
      if (selectedOption) {
        handleMentionSelect(selectedOption);
      }
      return true;
    case 'Escape':
      event.preventDefault();
      closeMentionPanel();
      return true;
  }

  return false;
};

const handleMentionSearchKeydown = (event: KeyboardEvent) => {
  if (handleMentionKeydown(event)) {
    return;
  }
};

const checkMentionTrigger = (ed: any) => {
  if (isComposing.value) {
    return;
  }

  const { from, to } = ed.state.selection;
  if (from !== to) {
    closeMentionPanel();
    return;
  }

  const textBeforeCursor = ed.state.doc.textBetween(0, from, '\n', '\n');

  for (const prefix of props.mentionPrefix) {
    const prefixStr = String(prefix);
    const lastPrefixIndex = textBeforeCursor.lastIndexOf(prefixStr);

    if (lastPrefixIndex === -1) continue;

    const charBefore = lastPrefixIndex > 0 ? textBeforeCursor[lastPrefixIndex - 1] : '';
    const isValidStart = lastPrefixIndex === 0 || /[\s\n]/.test(charBefore);

    if (!isValidStart) continue;

    const pattern = textBeforeCursor.substring(lastPrefixIndex + prefixStr.length);

    if (/\s/.test(pattern)) continue;

    mentionVisible.value = true;
    mentionActiveIndex.value = 0;
    mentionSearchValue.value = pattern;
    mentionTriggerInfo.value = {
      prefix: prefixStr,
      startPos: Math.max(1, from - pattern.length - prefixStr.length),
      cursorPos: from,
    };
    emit('mention-search', pattern, prefixStr);
    return;
  }

  closeMentionPanel();
};

watch(mentionVisible, (visible) => {
  if (typeof window === 'undefined') {
    return;
  }

  if (visible) {
    nextTick(() => {
      scheduleMentionDropdownPosition();
    });
    window.addEventListener('resize', handleMentionViewportChange);
    window.addEventListener('scroll', handleMentionViewportChange, true);
    return;
  }

  window.removeEventListener('resize', handleMentionViewportChange);
  window.removeEventListener('scroll', handleMentionViewportChange, true);
});

watch([mentionVisible, mentionFilteredOptions], () => {
  if (!mentionVisible.value) {
    return;
  }
  const optionCount = mentionFilteredOptions.value.length;
  if (!optionCount) {
    mentionActiveIndex.value = 0;
    scheduleMentionDropdownPosition();
    return;
  }
  if (mentionActiveIndex.value >= optionCount) {
    mentionActiveIndex.value = 0;
  }
  scrollActiveMentionIntoView();
  scheduleMentionDropdownPosition();
});

// 颜色选择器状态
const highlightColorPopoverShow = ref(false);
const textColorPopoverShow = ref(false);
const blockTypePopoverShow = ref(false);
const fontSizePopoverShow = ref(false);
const performancePopoverShow = ref(false);
const underlineStylePopoverShow = ref(false);
const strikeStylePopoverShow = ref(false);

// 链接弹窗状态
const linkModalShow = ref(false);
const linkText = ref('');
const linkUrl = ref('');
const linkOpenInNewTab = ref(false);
const linkTextType = ref<SmartLinkTextType>('text');
const linkUrlType = ref<SmartLinkUrlType>('url');
const linkTextImage = ref('');
const linkUrlImage = ref('');
const linkTextImageLabel = ref('');
const linkUrlImageLabel = ref('');
const rubyModalShow = ref(false);
const rubyBaseText = ref('');
const rubyTextInput = ref('');
const rubySelectionMode = ref<'insert' | 'apply' | 'edit'>('insert');
const rubyFontPanelExpanded = ref(false);
const rubySizePanelExpanded = ref(false);
const rubyBaseFontId = ref<string | null>(null);
const rubyRtFontId = ref<string | null>(null);
const rubyBaseFontSizeInput = ref('');
const rubyRtFontSizeInput = ref('');
const performanceEffect = ref<PerformanceEffect>('wave');
const performanceEnterMode = ref<PerformanceEnterMode>('normal');
const performanceEnterSpeed = ref(5);
const performanceToneIntensity = ref(0);
const performanceCommandType = ref<PerformanceCommandType>('delay');
const performanceCommandValue = ref('500');

watch(linkModalShow, (visible) => {
  if (!visible) {
    resetLinkModalState();
  }
});

watch(rubyModalShow, (visible) => {
  if (!visible) {
    resetRubyModalState();
  }
});

const resetLinkModalState = () => {
  linkText.value = '';
  linkUrl.value = '';
  linkOpenInNewTab.value = false;
  linkTextType.value = 'text';
  linkUrlType.value = 'url';
  linkTextImage.value = '';
  linkUrlImage.value = '';
  linkTextImageLabel.value = '';
  linkUrlImageLabel.value = '';
};

const resetRubyModalState = () => {
  rubyBaseText.value = '';
  rubyTextInput.value = '';
  rubySelectionMode.value = 'insert';
  rubyFontPanelExpanded.value = false;
  rubySizePanelExpanded.value = false;
  rubyBaseFontId.value = null;
  rubyRtFontId.value = null;
  rubyBaseFontSizeInput.value = '';
  rubyRtFontSizeInput.value = '';
};

const clampPerformanceToneIntensity = (value: number) => Math.max(-4, Math.min(4, Math.round(value)));
const clampPerformanceEnterSpeed = (value: number) => Math.max(1, Math.min(9, Math.round(value)));
const performanceEnterModeOptions = [
  { label: '正常', value: 'normal' },
  { label: '朦胧显现', value: 'blur' },
  { label: '逐字', value: 'typewriter' },
] as const;
const performanceToneMarks = {
  [-4]: '低语',
  [-1]: '压低',
  [1]: '强调',
  [4]: '爆发',
};
const performanceToneLabel = computed(() => {
  const value = performanceToneIntensity.value;
  if (value <= -3) return '低语';
  if (value < 0) return '收束';
  if (value === 0) return '中性';
  if (value < 3) return '强调';
  return '爆发';
});
const performanceSpeedLabel = computed(() => (
  performanceEnterSpeed.value <= 3 ? '慢'
    : performanceEnterSpeed.value >= 7 ? '快'
      : '中'
));

const applySmartLinkImage = (
  source: SmartLinkUploadSource,
  payload: { url: string; label?: string },
) => {
  const url = String(payload.url || '').trim();
  if (!url) {
    return;
  }
  const label = String(payload.label || '').trim();
  if (source === 'smart-link-text-image') {
    linkTextType.value = 'image';
    linkText.value = '';
    linkTextImage.value = url;
    linkTextImageLabel.value = label;
    return;
  }
  linkUrlType.value = 'image';
  linkUrl.value = '';
  linkUrlImage.value = url;
  linkUrlImageLabel.value = label;
};

const clearSmartLinkImage = (side: 'text' | 'url') => {
  if (side === 'text') {
    linkTextType.value = 'text';
    linkTextImage.value = '';
    linkTextImageLabel.value = '';
    return;
  }
  linkUrlType.value = 'url';
  linkUrlImage.value = '';
  linkUrlImageLabel.value = '';
};

const requestSmartLinkImageUpload = (source: SmartLinkUploadSource) => {
  emit('upload-button-click', source);
};

const quickIFormModalShow = ref(false);
const creatingIForm = ref(false);
const overlayInteractionAt = ref(0);
const quickIFormForm = reactive({
  name: '',
  url: '',
  embedCode: '',
  defaultWidth: 640,
  defaultHeight: 360,
});

const canQuickCreateIForm = computed(() => {
  return !!chat.currentWorldId && !!chat.curChannel?.id && iform.canManage;
});

const resetQuickIFormForm = () => {
  Object.assign(quickIFormForm, {
    name: '',
    url: '',
    embedCode: '',
    defaultWidth: 640,
    defaultHeight: 360,
  });
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

const openQuickIFormCreateModal = () => {
  if (!chat.curChannel?.id || !chat.currentWorldId) {
    message.warning('当前未定位到有效频道');
    return;
  }
  if (!iform.canManage) {
    message.warning('你没有创建 iForm 的权限');
    return;
  }
  resetQuickIFormForm();
  quickIFormModalShow.value = true;
};

const confirmQuickIFormCreate = async () => {
  if (!chat.curChannel?.id || !chat.currentWorldId) {
    message.warning('当前未定位到有效频道');
    return;
  }
  if (!iform.canManage) {
    message.warning('你没有创建 iForm 的权限');
    return;
  }
  const name = quickIFormForm.name.trim() || `消息嵌入 ${new Date().toLocaleTimeString('zh-CN', { hour12: false })}`;
  const url = quickIFormForm.url.trim();
  const embedCode = quickIFormForm.embedCode.trim();
  if (!url && !embedCode) {
    message.warning('请至少填写 URL 或嵌入代码');
    return;
  }
  const width = Math.max(120, Math.round(quickIFormForm.defaultWidth || 640));
  const height = Math.max(72, Math.round(quickIFormForm.defaultHeight || 360));

  creatingIForm.value = true;
  try {
    const created = await iform.createForm({
      name,
      url,
      embedCode,
      defaultWidth: width,
      defaultHeight: height,
      defaultCollapsed: false,
      defaultFloating: true,
    });
    const createdForm = created?.id
      ? (iform.currentForms.find((item) => item.id === created.id) || created)
      : null;
    if (!createdForm?.id) {
      throw new Error('创建成功但未获取到控件信息');
    }
    const link = generateIFormEmbedLink(
      {
        worldId: String(chat.currentWorldId),
        channelId: String(chat.curChannel.id),
        formId: createdForm.id,
        width: createdForm.defaultWidth || width,
        height: createdForm.defaultHeight || height,
      },
      { base: resolveIFormEmbedLinkBase() },
    );
    editor.value?.chain().focus().insertContent(link).run();
    quickIFormModalShow.value = false;
    message.success('已创建 iForm 并插入嵌入链接');
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '创建 iForm 失败');
  } finally {
    creatingIForm.value = false;
  }
};

// 预设高亮颜色色板 (7个预设 + 1个自定义)
const highlightColors = [
  '#fef08a', // 黄色（默认）
  '#bbf7d0', // 绿色
  '#bfdbfe', // 蓝色
  '#fecaca', // 红色
  '#e9d5ff', // 紫色
  '#fed7aa', // 橙色
  '#99f6e4', // 青色
];

// 预设文字颜色色板 (7个预设 + 1个自定义)
const textColors = [
  '#dc2626', // 红色
  '#ea580c', // 橙色
  '#ca8a04', // 黄色
  '#16a34a', // 绿色
  '#0284c7', // 蓝色
  '#7c3aed', // 紫色
  '#db2777', // 粉色
];

// 自定义颜色输入
const customHighlightColor = ref('#fce7f3');
const customTextColor = ref('#1f2937');
const platformFonts = ref<PlatformFontAsset[]>([]);
const platformFontLoading = ref(false);
const selectedPlatformFontId = ref<string | null>(null);
const selectedFontSize = ref<string | null>(null);
const selectedBlockType = ref<'paragraph' | 'heading-1' | 'heading-2' | 'heading-3'>('paragraph');
const customFontSizeInput = ref('');
const blockTypeOptions = [
  { value: 'paragraph', shortLabel: 'P', label: '正文' },
  { value: 'heading-1', shortLabel: 'H1', label: '标题 1' },
  { value: 'heading-2', shortLabel: 'H2', label: '标题 2' },
  { value: 'heading-3', shortLabel: 'H3', label: '标题 3' },
] as const;
const fontSizeOptions = [
  { value: null, shortLabel: 'A', label: '默认字号' },
  { value: '8px', shortLabel: '8', label: '8 px' },
  { value: '12px', shortLabel: '12', label: '12 px' },
  { value: '16px', shortLabel: '16', label: '16 px' },
  { value: '24px', shortLabel: '24', label: '24 px' },
  { value: '32px', shortLabel: '32', label: '32 px' },
  { value: '48px', shortLabel: '48', label: '48 px' },
] as const;
const decorationThicknessOptions = [
  { value: 'thin', label: '细', meta: '1px' },
  { value: 'regular', label: '中', meta: '2px' },
  { value: 'bold', label: '粗', meta: '3px' },
] as const;
const decorationPatternOptions = [
  { value: 'solid', label: '实线', meta: '━' },
  { value: 'dotted', label: '细点', meta: '···' },
  { value: 'dense-dotted', label: '密点', meta: '•••' },
] as const;
const decorationWaveOptions = [
  { value: 'none', label: '无', meta: '一' },
  { value: 'soft', label: '轻', meta: '≈' },
  { value: 'heavy', label: '重', meta: '≋' },
] as const;
const decorationCountOptions = [
  { value: 'single', label: '一重', meta: '1' },
  { value: 'double', label: '二重', meta: '2' },
  { value: 'triple', label: '三重', meta: '3' },
] as const;
const selectedUnderlineDecorationStyle = reactive<TextDecorationStyleState>({
  thickness: 'regular',
  pattern: 'solid',
  wave: 'none',
  count: 'single',
});
const selectedStrikeDecorationStyle = reactive<TextDecorationStyleState>({
  thickness: 'regular',
  pattern: 'solid',
  wave: 'none',
  count: 'single',
});
const DECORATION_THICKNESS_CSS: Record<TextDecorationThickness, string> = {
  thin: '1px',
  regular: '0.12em',
  bold: '0.18em',
};

const isDecorationThickness = (value: unknown): value is TextDecorationThickness =>
  value === 'thin' || value === 'regular' || value === 'bold';
const isDecorationPattern = (value: unknown): value is TextDecorationPattern =>
  value === 'solid' || value === 'dotted' || value === 'dense-dotted';
const isDecorationWave = (value: unknown): value is TextDecorationWave =>
  value === 'none' || value === 'soft' || value === 'heavy';
const isDecorationCount = (value: unknown): value is TextDecorationCount =>
  value === 'single' || value === 'double' || value === 'triple';
const isTextDecorationLine = (value: unknown): value is TextDecorationLine =>
  value === 'underline' || value === 'line-through';

const normalizeDecorationStyle = (attrs: Record<string, any> = {}): TextDecorationStyleState => ({
  thickness: isDecorationThickness(attrs.textDecorationThickness) ? attrs.textDecorationThickness : 'regular',
  pattern: isDecorationPattern(attrs.textDecorationPattern) ? attrs.textDecorationPattern : 'solid',
  wave: isDecorationWave(attrs.textDecorationWave) ? attrs.textDecorationWave : 'none',
  count: isDecorationCount(attrs.textDecorationCount) ? attrs.textDecorationCount : 'single',
});

const textDecorationLineFromAttrs = (attrs: Record<string, any> = {}): TextDecorationLine | null => (
  isTextDecorationLine(attrs.textDecorationLine) ? attrs.textDecorationLine : null
);

const renderTextDecorationStyleAttrs = (attrs: Record<string, any> = {}) => {
  const line = textDecorationLineFromAttrs(attrs);
  if (!line) {
    return {};
  }
  const style = normalizeDecorationStyle(attrs);
  const classes = [
    'tiptap-text-decoration',
    `tiptap-text-decoration--${line === 'underline' ? 'underline' : 'strike'}`,
    `tiptap-text-decoration--${style.pattern}`,
    `tiptap-text-decoration--wave-${style.wave}`,
    `tiptap-text-decoration--${style.count}`,
  ].join(' ');
  const cssText = [
    `--tiptap-decoration-line-kind: ${line}`,
    `--tiptap-decoration-thickness: ${DECORATION_THICKNESS_CSS[style.thickness]}`,
    `--tiptap-decoration-pattern: ${style.pattern}`,
    `--tiptap-decoration-wave: ${style.wave}`,
    `--tiptap-decoration-count: ${style.count}`,
  ].join('; ');
  return {
    class: classes,
    'data-text-decoration-line': line,
    'data-text-decoration-thickness': style.thickness,
    'data-text-decoration-pattern': style.pattern,
    'data-text-decoration-wave': style.wave,
    'data-text-decoration-count': style.count,
    style: cssText,
  };
};
const {
  platformFontOptions,
  renderPlatformFontLabel,
  renderPlatformFontOption,
  handleDropdownVisible: handlePlatformFontDropdownVisible,
  primeSelectedPreview: primePlatformFontPreview,
} = createPlatformFontSelectPreviewController({
  fonts: platformFonts,
  selectedId: selectedPlatformFontId,
  menuClass: 'tiptap-platform-font-select__menu',
});
const {
  platformFontOptions: rubyBaseFontOptions,
  renderPlatformFontLabel: renderRubyBaseFontLabel,
  renderPlatformFontOption: renderRubyBaseFontOption,
  handleDropdownVisible: handleRubyBaseFontDropdownVisible,
  primeSelectedPreview: primeRubyBaseFontPreview,
} = createPlatformFontSelectPreviewController({
  fonts: platformFonts,
  selectedId: rubyBaseFontId,
  menuClass: 'tiptap-platform-font-select__menu',
});
const {
  platformFontOptions: rubyRtFontOptions,
  renderPlatformFontLabel: renderRubyRtFontLabel,
  renderPlatformFontOption: renderRubyRtFontOption,
  handleDropdownVisible: handleRubyRtFontDropdownVisible,
  primeSelectedPreview: primeRubyRtFontPreview,
} = createPlatformFontSelectPreviewController({
  fonts: platformFonts,
  selectedId: rubyRtFontId,
  menuClass: 'tiptap-platform-font-select__menu',
});

const updateIsMobile = () => {
  const isNarrowViewport = window.innerWidth <= MOBILE_BREAKPOINT;
  const isCoarsePointer = window.matchMedia?.('(hover: none) and (pointer: coarse)')?.matches ?? false;
  const hasTouchPoints = (navigator?.maxTouchPoints || 0) > 0;
  const isMobileUa = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator?.userAgent || '');
  isMobile.value = isNarrowViewport || isCoarsePointer || (hasTouchPoints && isMobileUa);
  if (!isMobile.value) {
    fontSelectorExpanded.value = false;
  }
};

const toggleFontSelectorExpanded = () => {
  fontSelectorExpanded.value = !fontSelectorExpanded.value;
};

const closeFontSelector = () => {
  fontSelectorExpanded.value = false;
  desktopFontSelectorExpanded.value = false;
};

const handleDesktopFontSelectorShowUpdate = (show: boolean) => {
  if (isMobile.value) {
    return;
  }
  desktopFontSelectorExpanded.value = show;
};

const handlePlatformFontSelectShowUpdate = (show: boolean) => {
  handlePlatformFontDropdownVisible(show);
  handleDesktopFontSelectorShowUpdate(show);
};

const closePerformancePopover = () => {
  performancePopoverShow.value = false;
};

const closePerformancePopoverAfterSubmit = () => {
  if (isMobile.value) {
    closePerformancePopover();
  }
};

const syncPerformanceControlsFromSelection = () => {
  const attrs = (editor.value?.getAttributes('performance') || {}) as Record<string, any>;
  const effect = normalizePerformanceEffect(attrs.effect);
  const enterMode = String(attrs.enterMode || '').trim();
  const enterSpeed = Number(attrs.enterSpeed);
  const toneIntensity = Number(attrs.toneIntensity);
  if (effect) {
    performanceEffect.value = effect;
  }
  if (enterMode === 'normal' || enterMode === 'blur' || enterMode === 'typewriter') {
    performanceEnterMode.value = enterMode;
  }
  if (Number.isFinite(enterSpeed)) {
    performanceEnterSpeed.value = clampPerformanceEnterSpeed(enterSpeed);
  }
  if (Number.isFinite(toneIntensity)) {
    performanceToneIntensity.value = clampPerformanceToneIntensity(toneIntensity);
  } else if (attrs.scale === 'shout') {
    performanceToneIntensity.value = 3;
  } else if (attrs.scale === 'whisper') {
    performanceToneIntensity.value = -3;
  }
};

const openPerformancePopover = () => {
  closeToolbarPopovers();
  rememberEditorSelection();
  syncPerformanceControlsFromSelection();
  markOverlayInteraction();
  performancePopoverShow.value = true;
};

const getSelectionSnapshot = () => {
  const ed = editor.value;
  if (!ed) {
    return null;
  }
  const docSize = ed.state.doc.content.size;
  const range = savedEditorSelectionRange.value || {
    start: ed.state.selection.from,
    end: ed.state.selection.to,
  };
  const start = clamp(range.start, 0, docSize);
  const end = clamp(range.end, 0, docSize);
  return {
    from: Math.min(start, end),
    to: Math.max(start, end),
  };
};

const getSelectedTextBlockRanges = (selection = getSelectionSnapshot()) => {
  const ed = editor.value;
  if (!ed || !selection) {
    return [] as Array<{ from: number; to: number }>;
  }
  const { from, to } = selection;
  const $from = ed.state.doc.resolve(from);
  const ranges: Array<{ from: number; to: number }> = [];
  ed.state.doc.nodesBetween(from, to, (node, pos) => {
    if (!node.isTextblock) {
      return;
    }
    const start = pos + 1;
    const end = pos + node.content.size;
    if (start < end) {
      ranges.push({ from: start, to: end });
    }
    return false;
  });
  if (ranges.length > 0) {
    return ranges;
  }
  for (let depth = $from.depth; depth >= 0; depth -= 1) {
    const node = $from.node(depth);
    if (!node.isTextblock) {
      continue;
    }
    const start = $from.start(depth);
    const end = $from.end(depth);
    if (start < end) {
      return [{ from: start, to: end }];
    }
  }
  return [];
};

const updatePerformanceMarksInRange = (
  from: number,
  to: number,
  updater: (attrs: Record<string, any>) => Record<string, any>,
) => {
  const ed = editor.value;
  if (!ed) {
    return false;
  }
  const markType = ed.state.schema.marks.performance;
  if (!markType) {
    return false;
  }
  let tr = ed.state.tr;
  let touched = false;
  ed.state.doc.nodesBetween(from, to, (node, pos) => {
    if (!node.isText) {
      return;
    }
    const start = Math.max(from, pos);
    const end = Math.min(to, pos + node.nodeSize);
    if (start >= end) {
      return;
    }
    const currentMark = node.marks.find((mark) => mark.type === markType);
    const nextAttrs = updater({ ...(currentMark?.attrs || {}) });
    tr = tr.removeMark(start, end, markType);
    tr = tr.addMark(start, end, markType.create(nextAttrs));
    touched = true;
  });
  if (!touched) {
    return false;
  }
  ed.view.dispatch(tr);
  rememberEditorSelection();
  bumpEditorStateVersion();
  return true;
};

const getPerformanceBlockAttrs = () => ({
  enterMode: performanceEnterMode.value,
  enterSpeed: clampPerformanceEnterSpeed(performanceEnterSpeed.value),
  toneIntensity: clampPerformanceToneIntensity(performanceToneIntensity.value),
  scale: null,
});

const applyPerformanceBlockSettings = ({ silent = true }: { silent?: boolean } = {}) => {
  const ed = editor.value;
  if (!ed) {
    return false;
  }
  const selection = getSelectionSnapshot();
  const blockRanges = getSelectedTextBlockRanges(selection);
  if (blockRanges.length === 0) {
    if (!silent) {
      message.warning('当前块不支持演出设置');
    }
    return false;
  }
  const baseAttrs = getPerformanceBlockAttrs();
  let applied = false;
  blockRanges.forEach((range) => {
    applied = updatePerformanceMarksInRange(range.from, range.to, (attrs) => ({
      ...attrs,
      ...baseAttrs,
    })) || applied;
  });
  return applied;
};

const applyPerformanceEffectToSelection = () => {
  const ed = editor.value;
  if (!ed) {
    return;
  }
  if (!applyPerformanceBlockSettings({ silent: false })) {
    return;
  }
  restoreEditorSelection();
  const selection = getSelectionSnapshot();
  const blockRanges = getSelectedTextBlockRanges(selection);
  if (blockRanges.length === 0) {
    return;
  }
  const baseAttrs = getPerformanceBlockAttrs();
  const { from, to } = selection || ed.state.selection;
  const targetFrom = from === to ? blockRanges[0].from : from;
  const targetTo = from === to ? blockRanges[blockRanges.length - 1].to : to;
  updatePerformanceMarksInRange(targetFrom, targetTo, (attrs) => ({
    ...attrs,
    ...baseAttrs,
    effect: performanceEffect.value,
  }));
  closePerformancePopoverAfterSubmit();
};

const setPerformanceEnterMode = (mode: PerformanceEnterMode) => {
  performanceEnterMode.value = mode;
  applyPerformanceBlockSettings();
};

const handlePerformanceEnterSpeedUpdate = (value: number) => {
  performanceEnterSpeed.value = clampPerformanceEnterSpeed(value);
  applyPerformanceBlockSettings();
};

const handlePerformanceToneIntensityUpdate = (value: number) => {
  performanceToneIntensity.value = clampPerformanceToneIntensity(value);
  applyPerformanceBlockSettings();
};

const insertPerformanceCommandNode = () => {
  const ed = editor.value;
  if (!ed) {
    return;
  }
  if (!applyPerformanceBlockSettings({ silent: false })) {
    return;
  }
  const command = performanceCommandType.value === 'pause' ? 'pause' : 'delay';
  const rawValue = performanceCommandValue.value.trim();
  const numericValue = rawValue === '' ? null : Number(rawValue);
  if (command === 'delay' && !Number.isFinite(numericValue)) {
    message.warning('停顿时长要是数字');
    return;
  }
  ed.chain().focus().insertContent({
    type: 'performanceCommand',
    attrs: {
      command,
      value: command === 'delay' ? numericValue : null,
    },
  }).setMark('performance', getPerformanceBlockAttrs()).run();
  closePerformancePopoverAfterSubmit();
};

const applyCustomHighlightColor = () => {
  setHighlightColor(customHighlightColor.value);
};

const applyCustomTextColor = () => {
  setTextColor(customTextColor.value);
};

const currentBlockTypeOption = computed(() => {
  void editorStateVersion.value;
  return blockTypeOptions.find((option) => option.value === selectedBlockType.value) || blockTypeOptions[0];
});

const currentFontSizeOption = computed(() => {
  void editorStateVersion.value;
  const matched = fontSizeOptions.find((option) => option.value === selectedFontSize.value);
  if (matched) {
    return matched;
  }
  if (selectedFontSize.value) {
    const shortLabel = selectedFontSize.value.replace(/px$/i, '');
    return {
      value: selectedFontSize.value,
      shortLabel,
      label: `${shortLabel} px`,
    };
  }
  return fontSizeOptions[0];
});

const normalizeFontSizeValue = (value: string): string | null => {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }
  const normalized = trimmed.toLowerCase().endsWith('px') ? trimmed.slice(0, -2).trim() : trimmed;
  if (!/^\d+$/.test(normalized)) {
    return null;
  }
  const size = Number.parseInt(normalized, 10);
  if (!Number.isFinite(size) || size < 1 || size > 200) {
    return null;
  }
  return `${size}px`;
};

const normalizeOptionalFontSizeValue = (value: string): string | null => {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }
  return normalizeFontSizeValue(trimmed);
};

const EMPTY_DOC = {
  type: 'doc',
  content: [
    {
      type: 'paragraph',
    },
  ],
};

const cloneEmptyDoc = () => JSON.parse(JSON.stringify(EMPTY_DOC));

const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), max);

const classList = computed(() => {
  const base: string[] = ['tiptap-editor'];
  if (props.whisperMode) {
    base.push('whisper-mode');
  }
  if (isFocused.value) {
    base.push('is-focused');
  }
  if (isStickyNoteSurface.value) {
    base.push('tiptap-editor--sticky-note-surface');
  }
  const append = (item: any) => {
    if (!item) return;
    if (typeof item === 'string') {
      base.push(item);
    } else if (Array.isArray(item)) {
      item.forEach(append);
    } else if (typeof item === 'object') {
      Object.entries(item).forEach(([key, value]) => {
        if (value) {
          base.push(key);
        }
      });
    }
  };
  append(props.inputClass);
  return base;
});

let EditorContent: any = null;
let BubbleMenu: any = null;

// 动态导入 TipTap
const initEditor = async () => {
  try {
    isInitializing.value = true;

    const {
      Editor: EditorClass,
      Node: TiptapNodeClass,
      Extension,
      mergeAttributes,
      EditorContent: EditorContentComp,
      BubbleMenu: BubbleMenuComp,
      StarterKit,
      TextStyle,
      Color,
      Image,
      Highlight,
      TextAlign,
      Spoiler,
      Ruby,
      Performance,
      PerformanceCommand,
    } = await loadTipTapBundle();

    EditorContent = EditorContentComp;
    BubbleMenu = BubbleMenuComp;

    const preserveRichTextInputWhitespace = () => Extension.create({
      name: 'preserveRichTextInputWhitespace',
      addProseMirrorPlugins() {
        return [
          new Plugin({
            props: {
              handleTextInput(view: any, from: number, to: number, text: string) {
                if (text !== ' ') {
                  return false;
                }
                view.dispatch(view.state.tr.insertText(text, from, to).scrollIntoView());
                return true;
              },
            },
          }),
        ];
      },
    });

    const SatoriMention = TiptapNodeClass.create({
      name: 'satoriMention',
      inline: true,
      group: 'inline',
      atom: true,
      selectable: false,
      draggable: false,
      addAttributes() {
        return {
          id: { default: '' },
          name: { default: '' },
        };
      },
      parseHTML() {
        return [{ tag: 'span[data-satori-mention-id]' }];
      },
      renderHTML({ node, HTMLAttributes }: any) {
        const id = String(node.attrs?.id || '').trim();
        const name = String(node.attrs?.name || '').trim();
        const display = name || id || '用户';
        const cls = id === 'all' ? 'tiptap-mention-chip tiptap-mention-chip--all' : 'tiptap-mention-chip';
        return [
          'span',
          mergeAttributes(HTMLAttributes, {
            class: cls,
            contenteditable: 'false',
            'data-satori-mention-id': id,
            'data-satori-mention-name': name,
          }),
          `@${display}`,
        ];
      },
      renderText({ node }: any) {
        const id = String(node.attrs?.id || '').trim();
        const name = String(node.attrs?.name || '').trim();
        return `@${name || id || '用户'}`;
      },
      addKeyboardShortcuts() {
        const deleteAdjacentMention = (direction: 'backward' | 'forward') => ({ editor }: any) => {
          const { from, empty } = editor.state.selection;
          if (!empty) {
            return false;
          }
          const $from = editor.state.selection.$from;
          const targetNode = direction === 'backward' ? $from.nodeBefore : $from.nodeAfter;
          if (!targetNode || targetNode.type?.name !== 'satoriMention') {
            return false;
          }
          const targetPos = direction === 'backward' ? from - targetNode.nodeSize : from;
          editor.chain().focus().deleteRange({ from: targetPos, to: targetPos + targetNode.nodeSize }).run();
          return true;
        };
        return {
          Backspace: deleteAdjacentMention('backward'),
          Delete: deleteAdjacentMention('forward'),
        };
      },
    });

    const PlatformFontTextStyle = Extension.create({
      name: 'platformFontTextStyle',
      addGlobalAttributes() {
        return [
          {
            types: ['textStyle'],
            attributes: {
              fontAssetId: {
                default: null,
                parseHTML: (element: HTMLElement) => element.getAttribute('data-platform-font-id'),
                renderHTML: (attributes: Record<string, any>) => {
                  if (!attributes.fontAssetId) return {};
                  return { 'data-platform-font-id': attributes.fontAssetId };
                },
              },
              platformFontFamily: {
                default: null,
                parseHTML: (element: HTMLElement) => element.getAttribute('data-platform-font-family'),
                renderHTML: (attributes: Record<string, any>) => {
                  if (!attributes.platformFontFamily) return {};
                  return { 'data-platform-font-family': attributes.platformFontFamily };
                },
              },
              fontFamily: {
                default: null,
                parseHTML: (element: HTMLElement) => element.style.fontFamily || element.getAttribute('data-platform-font-family'),
                renderHTML: (attributes: Record<string, any>) => {
                  if (!attributes.fontFamily) return {};
                  return { style: `font-family: ${attributes.fontFamily}` };
                },
              },
              fontSize: {
                default: null,
                parseHTML: (element: HTMLElement) => element.style.fontSize || element.getAttribute('data-font-size'),
                renderHTML: (attributes: Record<string, any>) => {
                  if (!attributes.fontSize) return {};
                  return {
                    'data-font-size': attributes.fontSize,
                    style: `font-size: ${attributes.fontSize}`,
                  };
                },
              },
              textDecorationLine: {
                default: null,
                parseHTML: (element: HTMLElement) => (
                  element.getAttribute('data-text-decoration-line')
                  || (element.style.textDecorationLine === 'underline' || element.tagName.toLowerCase() === 'u' ? 'underline' : null)
                  || (element.style.textDecorationLine === 'line-through' || element.tagName.toLowerCase() === 's' ? 'line-through' : null)
                ),
                renderHTML: (attributes: Record<string, any>) => renderTextDecorationStyleAttrs(attributes),
              },
              textDecorationThickness: {
                default: null,
                parseHTML: (element: HTMLElement) => element.getAttribute('data-text-decoration-thickness'),
                renderHTML: () => ({}),
              },
              textDecorationPattern: {
                default: null,
                parseHTML: (element: HTMLElement) => element.getAttribute('data-text-decoration-pattern'),
                renderHTML: () => ({}),
              },
              textDecorationWave: {
                default: null,
                parseHTML: (element: HTMLElement) => element.getAttribute('data-text-decoration-wave'),
                renderHTML: () => ({}),
              },
              textDecorationCount: {
                default: null,
                parseHTML: (element: HTMLElement) => element.getAttribute('data-text-decoration-count'),
                renderHTML: () => ({}),
              },
            },
          },
        ];
      },
    });

    const SmartLinkNode = TiptapNodeClass.create({
      name: SMART_LINK_NODE_TYPE,
      inline: true,
      group: 'inline',
      atom: true,
      selectable: true,
      draggable: false,
      addAttributes() {
        return {
          textType: { default: 'text' },
          textValue: { default: '' },
          urlType: { default: 'url' },
          urlValue: { default: '' },
          target: { default: '_self' },
        };
      },
      parseHTML() {
        return [{ tag: `span[${SMART_LINK_DATA_ATTR}="true"]` }, { tag: `a[${SMART_LINK_DATA_ATTR}="true"]` }];
      },
      renderHTML({ node, HTMLAttributes }: any) {
        const attrs = normalizeSmartLinkAttrs(node.attrs);
        if (!attrs) {
          return ['span', mergeAttributes(HTMLAttributes), ''];
        }

        const dataset = {
          [SMART_LINK_DATA_ATTR]: 'true',
          'data-text-type': attrs.textType,
          'data-text-value': attrs.textValue,
          'data-url-type': attrs.urlType,
          'data-url-value': attrs.urlValue,
          'data-target': attrs.target,
          class: 'smart-link-node',
          contenteditable: 'false',
        };

        const content = attrs.textType === 'image'
          ? ['img', {
            src: attrs.textValue,
            alt: attrs.textValue,
            class: 'rich-inline-image smart-link-node__image',
            [SMART_LINK_IMAGE_ROLE_ATTR]: SMART_LINK_TEXT_IMAGE_ROLE,
          }]
          : ['span', { class: 'smart-link-node__text' }, resolveSmartLinkDisplayText(attrs)];

        if (attrs.urlType === 'url') {
          return [
            'a',
            mergeAttributes(HTMLAttributes, dataset, {
              href: attrs.urlValue,
              target: attrs.target,
              rel: 'noopener noreferrer',
            }),
            content,
          ];
        }

        return [
          'span',
          mergeAttributes(HTMLAttributes, dataset, {
            role: 'button',
            tabindex: '0',
          }),
          content,
        ];
      },
      renderText({ node }: any) {
        return resolveSmartLinkDisplayText(node.attrs);
      },
    });

    // 创建编辑器实例
    editor.value = new EditorClass({
      content: props.modelValue || '<p></p>',
      parseOptions: RICH_CONTENT_PARSE_OPTIONS,
      extensions: [
        preserveRichTextInputWhitespace(),
        SatoriMention,
        SmartLinkNode,
        StarterKit.configure({
          heading: {
            levels: [1, 2, 3],
          },
          codeBlock: {
            HTMLAttributes: {
              class: 'code-block',
            },
          },
          underline: {},
          link: {
            openOnClick: false,
            HTMLAttributes: {
              class: 'text-blue-500 underline cursor-pointer',
              target: '_blank',
              rel: 'noopener noreferrer',
            },
          },
        }),
        TextStyle,
        PlatformFontTextStyle,
        Color,
        Highlight.configure({
          multicolor: true,
        }),
        Spoiler,
        Ruby,
        Performance,
        PerformanceCommand,
        TextAlign.configure({
          types: ['heading', 'paragraph'],
        }),
        Image.configure({
          inline: true,
          allowBase64: true,
          HTMLAttributes: {
            class: 'rich-inline-image',
          },
        }),
      ],
      editorProps: {
        attributes: {
          class: 'tiptap-content',
        },
        handleKeyDown: (_view, event) => {
          if (handleMentionKeydown(event)) {
            return true;
          }
          emit('keydown', event);
          return event.defaultPrevented;
        },
        handlePaste: (view, event) => {
          const items = event.clipboardData?.items;
          if (!items) return false;

          const files: File[] = [];
          for (let i = 0; i < items.length; i++) {
            const item = items[i];
            if (item.kind === 'file' && item.type.startsWith('image/')) {
              const file = item.getAsFile();
              if (file) {
                files.push(file);
              }
            }
          }

          if (files.length > 0) {
            event.preventDefault();
            const { from, to } = view.state.selection;
            emit('paste-image', { files, selectionStart: from, selectionEnd: to });
            return true;
          }

          return false;
        },
        handleDrop: (view, event, slice, moved) => {
          if (moved) return false;

          const attachmentId = extractGalleryAttachmentId(event);
          if (attachmentId) {
            event.preventDefault();
            const { from, to } = view.state.selection;
            emit('drop-gallery-item', { attachmentId, selectionStart: from, selectionEnd: to });
            return true;
          }

          const files = Array.from(event.dataTransfer?.files || []).filter((file) =>
            file.type.startsWith('image/')
          );

          if (files.length > 0) {
            event.preventDefault();
            const { from, to } = view.state.selection;
            emit('drop-files', { files, selectionStart: from, selectionEnd: to });
            return true;
          }

          return false;
        },
      },
      onUpdate: ({ editor: ed }) => {
        const json = ed.getJSON();
        const serializedJson = serializeMentionNodesToTokens(json);
        const jsonString = JSON.stringify(serializedJson);
        bumpEditorStateVersion();
        isSyncingFromProps.value = true;
        emit('update:modelValue', jsonString);
        checkMentionTrigger(ed);
        nextTick(() => {
          isSyncingFromProps.value = false;
        });
      },
      onSelectionUpdate: ({ editor: ed }) => {
        editor.value = ed as Editor;
        rememberEditorSelection();
        bumpEditorStateVersion();
      },
      onFocus: () => {
        isFocused.value = true;
        rememberEditorSelection();
        bumpEditorStateVersion();
        emit('focus');
      },
      onBlur: ({ event }) => {
        isFocused.value = false;
        const relatedTarget = (event as FocusEvent).relatedTarget as HTMLElement | null;
        if (!relatedTarget?.closest('.mention-dropdown')) {
          setTimeout(() => {
            closeMentionPanel();
          }, 150);
        }
        emit('blur');
      },
      onCreate: ({ editor: ed }) => {
        // 初始化完成后，如果有内容则设置
        if (!props.modelValue) {
          ed.commands.setContent(cloneEmptyDoc(), SILENT_SET_CONTENT_OPTIONS);
          bumpEditorStateVersion();
          return;
        }
        ed.commands.setContent(parseIncomingRichContent(props.modelValue), SILENT_SET_CONTENT_OPTIONS);
        bumpEditorStateVersion();
      },
    }) as unknown as Editor;

    isInitializing.value = false;
  } catch (error) {
    console.error('初始化富文本编辑器失败:', error);
    isInitializing.value = false;
  }
};

// 监听外部值变化
watch(() => props.modelValue, (newValue) => {
  if (!editor.value || editor.value.isDestroyed) return;
  if (isSyncingFromProps.value) return;

  if (!newValue || newValue.trim() === '') {
    editor.value.commands.setContent(cloneEmptyDoc(), SILENT_SET_CONTENT_OPTIONS);
    editor.value.commands.setTextSelection(0);
    bumpEditorStateVersion();
    return;
  }

  try {
    const normalizedIncoming = parseIncomingRichContent(newValue);
    const currentSerialized = JSON.stringify(serializeMentionNodesToTokens(editor.value.getJSON()));
    const incomingSerialized = JSON.stringify(serializeMentionNodesToTokens(normalizedIncoming));
    if (currentSerialized !== incomingSerialized) {
      editor.value.commands.setContent(normalizedIncoming, SILENT_SET_CONTENT_OPTIONS);
      bumpEditorStateVersion();
    }
  } catch {
    // ignore
  }
});

// 监听 inline images 变化，更新编辑器中的图片
watch(() => props.inlineImages, (images) => {
  if (!editor.value || !images) return;

  Object.entries(images).forEach(([markerId, imageInfo]) => {
    if (imageInfo.status === 'uploaded' && imageInfo.previewUrl) {
      // 查找编辑器中所有临时图片节点
      const { state } = editor.value!;
      const { doc } = state;
      let found = false;

      doc.descendants((node, pos) => {
        if (node.type.name === 'image' && node.attrs.src?.includes(markerId)) {
          // 更新图片节点
          const tr = state.tr.setNodeMarkup(pos, undefined, {
            ...node.attrs,
            src: imageInfo.previewUrl,
          });
          editor.value!.view.dispatch(tr);
          found = true;
          return false;
        }
      });
    }
  });
}, { deep: true });

const focus = () => {
  nextTick(() => {
    editor.value?.commands.focus();
  });
};

const blur = () => {
  editor.value?.commands.blur();
};

const getTextarea = (): HTMLTextAreaElement | undefined => {
  return undefined;
};

const getSelectionRange = () => {
  const ed = editor.value;
  if (!ed) {
    const length = props.modelValue.length;
    return { start: length, end: length };
  }
  const { from, to } = ed.state.selection;
  return { start: from, end: to };
};

const setSelectionRange = (start: number, end: number) => {
  const ed = editor.value;
  if (!ed) return;
  const docSize = ed.state.doc.content.size;
  const safeStart = clamp(start, 0, docSize);
  const safeEnd = clamp(end, 0, docSize);
  ed.chain().setTextSelection({ from: safeStart, to: safeEnd }).run();
};

const moveCursorToEnd = () => {
  editor.value?.chain().focus('end').run();
};

const insertImagePlaceholder = (markerId: string, previewUrl: string) => {
  if (!editor.value) return;

  // 在当前光标位置插入图片
  editor.value.chain().focus().setImage({ src: previewUrl, alt: `图片-${markerId}` }).run();
};

// Toolbar actions
const toggleBold = () => {
  const result = editor.value?.chain().focus().toggleBold().run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  return result;
};
const toggleItalic = () => {
  const result = editor.value?.chain().focus().toggleItalic().run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  return result;
};
const isDecorationActive = (line: TextDecorationLine) => {
  const attrs = (editor.value?.getAttributes('textStyle') || {}) as Record<string, any>;
  if (textDecorationLineFromAttrs(attrs) === line) {
    return true;
  }
  return line === 'underline' ? isActive('underline') : isActive('strike');
};

const markHasDecorationLine = (marks: readonly any[] | undefined, line: TextDecorationLine) => {
  const hasStyledDecoration = marks?.some((mark) => (
    mark?.type?.name === 'textStyle'
    && textDecorationLineFromAttrs(mark.attrs || {}) === line
  ));
  const hasLegacyDecoration = marks?.some((mark) => (
    line === 'underline' ? mark?.type?.name === 'underline' : mark?.type?.name === 'strike'
  ));
  return Boolean(hasStyledDecoration || hasLegacyDecoration);
};

const getDecorationSelectionState = (line: TextDecorationLine): TextDecorationSelectionState => {
  const ed = editor.value;
  if (!ed) {
    return 'none';
  }
  const { from, to, empty } = ed.state.selection;
  if (empty || from === to) {
    return isDecorationActive(line) ? 'full' : 'none';
  }

  let selectedTextSize = 0;
  let decoratedTextSize = 0;
  ed.state.doc.nodesBetween(from, to, (node: any, pos: number) => {
    if (!node?.isText || typeof node.text !== 'string') {
      return;
    }
    const textFrom = Math.max(from, pos);
    const textTo = Math.min(to, pos + node.nodeSize);
    const size = Math.max(0, textTo - textFrom);
    if (!size) {
      return;
    }
    selectedTextSize += size;
    if (markHasDecorationLine(node.marks, line)) {
      decoratedTextSize += size;
    }
  });

  if (selectedTextSize === 0) {
    return isDecorationActive(line) ? 'full' : 'none';
  }
  if (decoratedTextSize === 0) {
    return 'none';
  }
  return decoratedTextSize >= selectedTextSize ? 'full' : 'partial';
};

const rememberDecorationStyle = (line: TextDecorationLine, style: TextDecorationStyleState) => {
  const target = line === 'underline' ? selectedUnderlineDecorationStyle : selectedStrikeDecorationStyle;
  target.thickness = style.thickness;
  target.pattern = style.pattern;
  target.wave = style.wave;
  target.count = style.count;
};

const applyDecorationStyle = (line: TextDecorationLine, nextStyle?: Partial<TextDecorationStyleState>) => {
  const remembered = line === 'underline' ? selectedUnderlineDecorationStyle : selectedStrikeDecorationStyle;
  const style = { ...remembered, ...nextStyle };
  rememberDecorationStyle(line, style);
  const result = runEditorCommandWithSelection((chain) => {
    chain
      .unsetUnderline()
      .unsetStrike()
      .setMark('textStyle', {
        textDecorationLine: line,
        textDecorationThickness: style.thickness,
        textDecorationPattern: style.pattern,
        textDecorationWave: style.wave,
        textDecorationCount: style.count,
      });
  });
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  return result;
};

const applyUnderlineDecorationStyle = (nextStyle?: Partial<TextDecorationStyleState>) =>
  applyDecorationStyle('underline', nextStyle);
const applyStrikeDecorationStyle = (nextStyle?: Partial<TextDecorationStyleState>) =>
  applyDecorationStyle('line-through', nextStyle);

const removeDecorationStyle = (line: TextDecorationLine) => {
  const result = runEditorCommandWithSelection((chain) => {
    if (line === 'underline') {
      chain.unsetUnderline();
    } else {
      chain.unsetStrike();
    }
    chain.setMark('textStyle', {
      textDecorationLine: null,
      textDecorationThickness: null,
      textDecorationPattern: null,
      textDecorationWave: null,
      textDecorationCount: null,
    });
  });
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  return result;
};

const toggleDecorationStyle = (line: TextDecorationLine) => {
  if (isDecorationActive(line)) {
    return removeDecorationStyle(line);
  }
  return applyDecorationStyle(line);
};

const openDecorationPopover = (line: TextDecorationLine) => {
  closeToolbarPopovers();
  rememberEditorSelection();
  const attrs = (editor.value?.getAttributes('textStyle') || {}) as Record<string, any>;
  if (textDecorationLineFromAttrs(attrs) === line) {
    rememberDecorationStyle(line, normalizeDecorationStyle(attrs));
  }
  markOverlayInteraction();
  if (line === 'underline') {
    underlineStylePopoverShow.value = true;
  } else {
    strikeStylePopoverShow.value = true;
  }
};

const closeDecorationPopover = (line: TextDecorationLine) => {
  markOverlayInteraction();
  if (line === 'underline') {
    underlineStylePopoverShow.value = false;
  } else {
    strikeStylePopoverShow.value = false;
  }
};

const handleDecorationTriggerPointerDown = (event: PointerEvent | MouseEvent, line: TextDecorationLine) => {
  event.preventDefault();
  rememberEditorSelection();
  markOverlayInteraction();

  const selectionState = getDecorationSelectionState(line);
  if (selectionState === 'full') {
    removeDecorationStyle(line);
    if (line === 'underline') {
      underlineStylePopoverShow.value = false;
    } else {
      strikeStylePopoverShow.value = false;
    }
    return;
  }

  if (selectionState === 'partial') {
    openDecorationPopover(line);
    return;
  }

  if (selectionState === 'none') {
    applyDecorationStyle(line);
    openDecorationPopover(line);
  }
};

const triggerDecorationPopover = (line: TextDecorationLine, show: boolean) => {
  if (show) {
    openDecorationPopover(line);
    return;
  }
  if (line === 'underline') {
    underlineStylePopoverShow.value = false;
  } else {
    strikeStylePopoverShow.value = false;
  }
};

const applyDecorationOption = (
  event: MouseEvent,
  line: TextDecorationLine,
  key: keyof TextDecorationStyleState,
  value: TextDecorationStyleState[keyof TextDecorationStyleState],
) => {
  event.preventDefault();
  markOverlayInteraction();
  if (line === 'underline') {
    applyUnderlineDecorationStyle({ [key]: value });
  } else {
    applyStrikeDecorationStyle({ [key]: value });
  }
};

const toggleUnderline = () => toggleDecorationStyle('underline');
const toggleStrike = () => toggleDecorationStyle('line-through');
const toggleSpoiler = () => {
  const result = editor.value?.chain().focus().toggleSpoiler().run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  return result;
};
const toggleCode = () => editor.value?.chain().focus().toggleCode().run();
const toggleCodeBlock = () => editor.value?.chain().focus().toggleCodeBlock().run();
const toggleBulletList = () => editor.value?.chain().focus().toggleBulletList().run();
const toggleOrderedList = () => editor.value?.chain().focus().toggleOrderedList().run();
const toggleBlockquote = () => editor.value?.chain().focus().toggleBlockquote().run();
const applyBlockType = (value: 'paragraph' | 'heading-1' | 'heading-2' | 'heading-3') => {
  runEditorCommandWithSelection((chain) => {
    if (value === 'paragraph') {
      chain.setParagraph();
    } else {
      const level = Number(value.replace('heading-', '')) as 1 | 2 | 3;
      chain.setHeading({ level });
    }
  });
  selectedBlockType.value = value;
  blockTypePopoverShow.value = false;
};
const setHeading = (level: 1 | 2 | 3) => applyBlockType(`heading-${level}` as 'heading-1' | 'heading-2' | 'heading-3');
const setParagraph = () => applyBlockType('paragraph');
const setTextAlign = (align: 'left' | 'center' | 'right' | 'justify') => editor.value?.chain().focus().setTextAlign(align).run();
const toggleHighlight = () => {
  const result = editor.value?.chain().focus().toggleHighlight().run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  return result;
};
const insertHorizontalRule = () => editor.value?.chain().focus().setHorizontalRule().run();
const clearFormatting = () => editor.value?.chain().focus().clearNodes().unsetAllMarks().run();
const insertStateWidgetTemplate = () => {
  if (!editor.value) return;
  const { from, to } = editor.value.state.selection;
  const selectedText = from !== to
    ? editor.value.state.doc.textBetween(from, to, ' ').trim()
    : '';
  const firstOption = selectedText || '选项1';
  const template = `[${firstOption}|选项2|选项3]`;
  editor.value.chain().focus().insertContent(template).run();
};

const insertIFormEmbedLink = () => {
  openQuickIFormCreateModal();
};

const DEFAULT_RUBY_RT_SCALE = '0.92em';

const resolveRubyRtScale = (fontSize: string | null) => {
  const value = String(fontSize || '').trim().toLowerCase();
  if (!value.endsWith('px')) {
    return DEFAULT_RUBY_RT_SCALE;
  }
  const size = Number.parseFloat(value);
  if (!Number.isFinite(size) || size <= 0) {
    return DEFAULT_RUBY_RT_SCALE;
  }
  if (size <= 16) {
    return '1em';
  }
  if (size <= 24) {
    return '0.96em';
  }
  if (size <= 36) {
    return '0.94em';
  }
  return DEFAULT_RUBY_RT_SCALE;
};

const resolvePlatformFontStyleAttrs = (fontId: string | null) => {
  if (!fontId) {
    return {
      fontAssetId: null,
      platformFontFamily: null,
      fontFamily: null,
    };
  }
  const target = platformFonts.value.find((item) => item.id === fontId) || null;
  if (!target) {
    return {
      fontAssetId: null,
      platformFontFamily: null,
      fontFamily: null,
    };
  }
  return {
    fontAssetId: target.id,
    platformFontFamily: target.family,
    fontFamily: `"${target.family}"`,
  };
};

const getActiveRubyAttrs = () => {
  const ed = editor.value;
  return (ed?.getAttributes('ruby') || {}) as Record<string, any>;
};

const syncRubyModalStyleStateFromAttrs = (attrs?: Record<string, any>) => {
  const rubyAttrs = attrs || getActiveRubyAttrs();
  rubyBaseFontId.value = typeof rubyAttrs.rubyBaseFontAssetId === 'string'
    ? rubyAttrs.rubyBaseFontAssetId
    : typeof rubyAttrs.rubyFontAssetId === 'string'
      ? rubyAttrs.rubyFontAssetId
      : null;
  rubyRtFontId.value = typeof rubyAttrs.rubyRtFontAssetId === 'string'
    ? rubyAttrs.rubyRtFontAssetId
    : typeof rubyAttrs.rubyFontAssetId === 'string'
      ? rubyAttrs.rubyFontAssetId
      : null;
  rubyBaseFontSizeInput.value = String(
    rubyAttrs.rubyBaseFontSize || rubyAttrs.rubyFontSize || '',
  ).replace(/px$/i, '');
  rubyRtFontSizeInput.value = String(
    rubyAttrs.rubyRtFontSize || rubyAttrs.rubyFontSize || '',
  ).replace(/px$/i, '');
  rubyFontPanelExpanded.value = Boolean(rubyBaseFontId.value || rubyRtFontId.value);
  rubySizePanelExpanded.value = Boolean(rubyBaseFontSizeInput.value || rubyRtFontSizeInput.value);
  primeRubyBaseFontPreview(rubyBaseFontId.value);
  primeRubyRtFontPreview(rubyRtFontId.value);
};

const buildRubyMarkAttrs = (rubyText: string) => {
  const ed = editor.value;
  const textStyleAttrs = (ed?.getAttributes('textStyle') || {}) as Record<string, any>;
  const highlightAttrs = (ed?.getAttributes('highlight') || {}) as Record<string, any>;
  const decorationLine = textDecorationLineFromAttrs(textStyleAttrs);
  const rubyFontSize = typeof textStyleAttrs.fontSize === 'string' ? textStyleAttrs.fontSize : null;
  const rubyBaseFontSize = normalizeOptionalFontSizeValue(rubyBaseFontSizeInput.value) || rubyFontSize;
  const rubyRtFontSize = normalizeOptionalFontSizeValue(rubyRtFontSizeInput.value) || rubyFontSize;
  const rubyBaseFontStyle = resolvePlatformFontStyleAttrs(rubyBaseFontId.value);
  const rubyRtFontStyle = resolvePlatformFontStyleAttrs(rubyRtFontId.value);
  return {
    rubyText: rubyText.trim(),
    rubyFontAssetId: typeof textStyleAttrs.fontAssetId === 'string' ? textStyleAttrs.fontAssetId : null,
    rubyPlatformFontFamily: typeof textStyleAttrs.platformFontFamily === 'string' ? textStyleAttrs.platformFontFamily : null,
    rubyFontFamily: typeof textStyleAttrs.fontFamily === 'string' ? textStyleAttrs.fontFamily : null,
    rubyFontSize,
    rubyBaseFontAssetId: rubyBaseFontStyle.fontAssetId,
    rubyBasePlatformFontFamily: rubyBaseFontStyle.platformFontFamily,
    rubyBaseFontFamily: rubyBaseFontStyle.fontFamily,
    rubyBaseFontSize,
    rubyRtFontAssetId: rubyRtFontStyle.fontAssetId,
    rubyRtPlatformFontFamily: rubyRtFontStyle.platformFontFamily,
    rubyRtFontFamily: rubyRtFontStyle.fontFamily,
    rubyRtFontSize,
    rubyColor: typeof textStyleAttrs.color === 'string' ? textStyleAttrs.color : null,
    rubyFontWeight: ed?.isActive('bold') ? '700' : null,
    rubyFontStyle: ed?.isActive('italic') ? 'italic' : null,
    rubyRtScale: resolveRubyRtScale(rubyRtFontSize),
    rubyTextDecoration: decorationLine || (ed?.isActive('strike') ? 'line-through' : ed?.isActive('underline') ? 'underline' : null),
    rubyBackgroundColor: typeof highlightAttrs.color === 'string' ? highlightAttrs.color : null,
    rubySpoiler: ed?.isActive('spoiler') ? 'true' : null,
  };
};

const syncActiveRubyVisualAttrs = () => {
  const ed = editor.value;
  if (!ed) {
    return;
  }
  const rubyText = String(ed.getAttributes('ruby')?.rubyText || '').trim();
  if (!rubyText) {
    return;
  }
  ed.chain().focus().setMark('ruby', buildRubyMarkAttrs(rubyText)).run();
  rememberEditorSelection();
  bumpEditorStateVersion();
};

// 高亮颜色操作
const setHighlightColor = (color: string) => {
  const result = editor.value?.chain().focus().setHighlight({ color }).run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  highlightColorPopoverShow.value = false;
};

const removeHighlight = () => {
  const result = editor.value?.chain().focus().unsetHighlight().run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  highlightColorPopoverShow.value = false;
};

const getActiveHighlightColor = () => {
  if (!editor.value) return null;
  const attrs = editor.value.getAttributes('highlight');
  return attrs?.color || null;
};

// 文字颜色操作
const setTextColor = (color: string) => {
  const result = editor.value?.chain().focus().setColor(color).run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  textColorPopoverShow.value = false;
};

const refreshPlatformFonts = async () => {
  platformFontLoading.value = true;
  try {
    platformFonts.value = await listPlatformFonts();
    primePlatformFontPreview(selectedPlatformFontId.value);
  } catch (error) {
    console.warn('加载平台字体失败', error);
    platformFonts.value = [];
  } finally {
    platformFontLoading.value = false;
  }
};

const applyPlatformFont = async (fontId: string | null) => {
  markOverlayInteraction();
  if (!fontId) {
    runEditorCommandWithSelection((chain) => {
      chain.setMark('textStyle', {
        fontAssetId: null,
        platformFontFamily: null,
        fontFamily: null,
      });
    });
    selectedPlatformFontId.value = null;
    syncActiveRubyVisualAttrs();
    if (isMobile.value) {
      closeFontSelector();
    }
    return;
  }
  const target = platformFonts.value.find((item) => item.id === fontId) || null;
  if (!target) {
    message.warning('平台字体不存在或尚未加载');
    return;
  }
  const family = await ensurePlatformFontLoaded(target.id, target.family);
  runEditorCommandWithSelection((chain) => {
    chain.setMark('textStyle', {
      fontAssetId: target.id,
      platformFontFamily: family,
      fontFamily: `"${family}"`,
    });
  });
  selectedPlatformFontId.value = target.id;
  syncActiveRubyVisualAttrs();
  if (isMobile.value) {
    closeFontSelector();
  }
};

const applyFontSize = (size: string | null) => {
  runEditorCommandWithSelection((chain) => {
    chain.setMark('textStyle', {
      fontSize: size,
    });
  });
  selectedFontSize.value = size;
  syncActiveRubyVisualAttrs();
  fontSizePopoverShow.value = false;
};

const applyFontSizeOption = (event: MouseEvent, size: string | null) => {
  event.preventDefault();
  markOverlayInteraction();
  applyFontSize(size);
};

const applyCustomFontSize = () => {
  const normalized = normalizeFontSizeValue(customFontSizeInput.value);
  if (!normalized) {
    message.warning('请输入 1 到 200 的字号，可省略 px');
    return;
  }
  customFontSizeInput.value = normalized.replace(/px$/i, '');
  applyFontSize(normalized);
};

const applyBlockTypeOption = (event: MouseEvent, value: 'paragraph' | 'heading-1' | 'heading-2' | 'heading-3') => {
  event.preventDefault();
  markOverlayInteraction();
  applyBlockType(value);
};

const triggerBlockTypePopover = (show: boolean) => {
  if (show) {
    closeToolbarPopovers();
    rememberEditorSelection();
  }
  blockTypePopoverShow.value = show;
};

const triggerFontSizePopover = (show: boolean) => {
  if (show) {
    closeToolbarPopovers();
    rememberEditorSelection();
    customFontSizeInput.value = selectedFontSize.value?.replace(/px$/i, '') || '';
  }
  fontSizePopoverShow.value = show;
};

const removeTextColor = () => {
  const result = editor.value?.chain().focus().unsetColor().run();
  if (result) {
    syncActiveRubyVisualAttrs();
  }
  textColorPopoverShow.value = false;
};

const getActiveTextColor = () => {
  if (!editor.value) return null;
  const attrs = editor.value.getAttributes('textStyle');
  return attrs?.color || null;
};

const findSelectedSmartLink = () => {
  const ed = editor.value;
  if (!ed) {
    return null as { node: any; pos: number } | null;
  }
  const { from, to } = ed.state.selection;
  let result: { node: any; pos: number } | null = null;
  ed.state.doc.descendants((node: any, pos: number) => {
    if (node.type?.name !== SMART_LINK_NODE_TYPE) {
      return true;
    }
    const end = pos + node.nodeSize;
    const overlaps = (pos <= from && from < end)
      || (pos < to && to <= end)
      || (from <= pos && end <= to);
    if (overlaps) {
      result = { node, pos };
      return false;
    }
    return true;
  });
  return result;
};

const setLink = () => {
  const selectedSmartLink = findSelectedSmartLink();
  resetLinkModalState();

  if (selectedSmartLink) {
    const attrs = normalizeSmartLinkAttrs(selectedSmartLink.node.attrs);
    if (attrs) {
      linkTextType.value = attrs.textType;
      linkUrlType.value = attrs.urlType;
      linkText.value = attrs.textType === 'text' ? attrs.textValue : '';
      linkUrl.value = attrs.urlType === 'url' ? attrs.urlValue : '';
      linkTextImage.value = attrs.textType === 'image' ? attrs.textValue : '';
      linkUrlImage.value = attrs.urlType === 'image' ? attrs.urlValue : '';
      linkTextImageLabel.value = attrs.textType === 'image' ? '已选文本图片' : '';
      linkUrlImageLabel.value = attrs.urlType === 'image' ? '已选目标图片' : '';
      linkOpenInNewTab.value = attrs.target === '_blank';
    }
    linkModalShow.value = true;
    return;
  }

  const { from, to } = editor.value?.state.selection || { from: 0, to: 0 };
  const hasSelection = from !== to;
  if (hasSelection) {
    linkText.value = editor.value?.state.doc.textBetween(from, to, ' ') || '';
  }
  linkModalShow.value = true;
};

const closeLinkModal = () => {
  linkModalShow.value = false;
  resetLinkModalState();
};

const confirmLink = () => {
  const ed = editor.value;
  if (!ed) {
    closeLinkModal();
    return;
  }

  const selectedSmartLink = findSelectedSmartLink();
  const { from, to } = ed.state.selection;
  const hasSelection = from !== to;
  const selectedText = hasSelection ? ed.state.doc.textBetween(from, to, ' ').trim() : '';

  const textType = linkTextType.value;
  const urlType = linkUrlType.value;
  const target = linkOpenInNewTab.value ? '_blank' : '_self';
  const textValue = textType === 'image'
    ? linkTextImage.value.trim()
    : (linkText.value.trim() || selectedText);
  const urlValue = urlType === 'image'
    ? linkUrlImage.value.trim()
    : linkUrl.value.trim();

  if (!urlValue) {
    message.warning(urlType === 'image' ? '请先选择目标图片' : '请输入链接地址');
    return;
  }

  if ((textType === 'image' || urlType === 'image') && !textValue) {
    message.warning(textType === 'image' ? '请先选择链接文本图片' : '请输入链接文本');
    return;
  }

  if (textType === 'text' && urlType === 'url') {
    if (selectedSmartLink) {
      const tr = ed.state.tr.delete(selectedSmartLink.pos, selectedSmartLink.pos + selectedSmartLink.node.nodeSize);
      ed.view.dispatch(tr);
      ed.chain().focus().insertContent({
        type: 'text',
        text: textValue || urlValue,
        marks: [{ type: 'link', attrs: { href: urlValue, target } }],
      }).run();
      closeLinkModal();
      return;
    }

    if (hasSelection) {
      ed.chain().focus().setLink({ href: urlValue, target }).run();
    } else {
      ed.chain().focus().insertContent({
        type: 'text',
        text: textValue || urlValue,
        marks: [{ type: 'link', attrs: { href: urlValue, target } }],
      }).run();
    }
    closeLinkModal();
    return;
  }

  const attrs = normalizeSmartLinkAttrs({
    textType,
    textValue,
    urlType,
    urlValue,
    target,
  });
  if (!attrs) {
    message.warning('链接配置不完整');
    return;
  }

  if (selectedSmartLink) {
    const tr = ed.state.tr.setNodeMarkup(selectedSmartLink.pos, undefined, attrs);
    ed.view.dispatch(tr);
    closeLinkModal();
    return;
  }

  const chain = ed.chain().focus();
  if (hasSelection) {
    chain.deleteSelection();
  }
  chain.insertContent({
    type: SMART_LINK_NODE_TYPE,
    attrs,
  }).run();
  closeLinkModal();
};

const unsetLink = () => {
  editor.value?.chain().focus().unsetLink().run();
};

const getSelectedPlainText = () => {
  const ed = editor.value;
  if (!ed) {
    return '';
  }
  const { from, to } = ed.state.selection;
  if (from === to) {
    return '';
  }
  return ed.state.doc.textBetween(from, to, ' ').trim();
};

const getUniformRubyTextFromSelection = () => {
  const ed = editor.value;
  if (!ed) {
    return '';
  }
  const { from, to } = ed.state.selection;
  if (from === to) {
    return '';
  }

  const rubyTexts = new Set<string>();
  let hasTextNode = false;
  ed.state.doc.nodesBetween(from, to, (node) => {
    if (!node.isText) {
      return;
    }
    hasTextNode = true;
    const rubyMark = node.marks.find((mark) => mark.type.name === 'ruby');
    if (!rubyMark) {
      rubyTexts.add('');
      return;
    }
    rubyTexts.add(String(rubyMark.attrs?.rubyText || '').trim());
  });

  if (!hasTextNode || rubyTexts.size !== 1) {
    return '';
  }

  return Array.from(rubyTexts)[0] || '';
};

const getUniformRubyAttrsFromSelection = () => {
  const ed = editor.value;
  if (!ed) {
    return null;
  }
  const { from, to } = ed.state.selection;
  if (from === to) {
    return null;
  }

  let firstAttrs: Record<string, any> | null = null;
  let hasTextNode = false;
  let isUniform = true;

  ed.state.doc.nodesBetween(from, to, (node) => {
    if (!node.isText || !isUniform) {
      return;
    }
    hasTextNode = true;
    const rubyMark = node.marks.find((mark) => mark.type.name === 'ruby');
    if (!rubyMark) {
      isUniform = false;
      return;
    }
    const attrs = rubyMark.attrs || {};
    if (!firstAttrs) {
      firstAttrs = attrs;
      return;
    }
    if (JSON.stringify(firstAttrs) !== JSON.stringify(attrs)) {
      isUniform = false;
    }
  });

  if (!hasTextNode || !isUniform || !firstAttrs) {
    return null;
  }
  return firstAttrs;
};

const openRubyModal = () => {
  const ed = editor.value;
  if (!ed) {
    return;
  }

  rememberEditorSelection();
  markOverlayInteraction();
  const selectedText = getSelectedPlainText();
  const selectedRubyText = getUniformRubyTextFromSelection();
  const selectedRubyAttrs = getUniformRubyAttrsFromSelection();

  if (!selectedText) {
    rubySelectionMode.value = 'insert';
    rubyBaseText.value = '';
    rubyTextInput.value = '';
    syncRubyModalStyleStateFromAttrs();
  } else if (selectedRubyText) {
    rubySelectionMode.value = 'edit';
    rubyBaseText.value = selectedText;
    rubyTextInput.value = selectedRubyText;
    syncRubyModalStyleStateFromAttrs(selectedRubyAttrs || undefined);
  } else {
    rubySelectionMode.value = 'apply';
    rubyBaseText.value = selectedText;
    rubyTextInput.value = '';
    syncRubyModalStyleStateFromAttrs();
  }

  rubyModalShow.value = true;
};

const closeRubyModal = () => {
  rubyModalShow.value = false;
  resetRubyModalState();
};

const handleRubyBaseFontShowUpdate = (show: boolean) => {
  handleRubyBaseFontDropdownVisible(show);
};

const handleRubyRtFontShowUpdate = (show: boolean) => {
  handleRubyRtFontDropdownVisible(show);
};

const confirmRuby = () => {
  const ed = editor.value;
  if (!ed) {
    closeRubyModal();
    return;
  }

  const normalizedRubyText = rubyTextInput.value.trim();
  const normalizedBaseFontSize = normalizeOptionalFontSizeValue(rubyBaseFontSizeInput.value);
  const normalizedRtFontSize = normalizeOptionalFontSizeValue(rubyRtFontSizeInput.value);

  if (rubyBaseFontSizeInput.value.trim() && !normalizedBaseFontSize) {
    message.warning('正文字号请输入 1 到 200 的数字，可省略 px');
    return;
  }
  if (rubyRtFontSizeInput.value.trim() && !normalizedRtFontSize) {
    message.warning('注音字号请输入 1 到 200 的数字，可省略 px');
    return;
  }

  if (rubySelectionMode.value === 'insert') {
    const baseText = rubyBaseText.value.trim();
    if (!baseText || !normalizedRubyText) {
      message.warning('请输入正文与注音');
      return;
    }
    restoreEditorSelection();
    const insertFrom = ed.state.selection.from;
    ed.chain().focus().insertContent(baseText).run();
    ed.chain().focus().setTextSelection({
      from: insertFrom,
      to: insertFrom + baseText.length,
    }).setMark('ruby', buildRubyMarkAttrs(normalizedRubyText)).run();
    rememberEditorSelection();
    bumpEditorStateVersion();
    closeRubyModal();
    return;
  }

  restoreEditorSelection();
  const chain = ed.chain().focus();
  const applied = normalizedRubyText
    ? chain.setMark('ruby', buildRubyMarkAttrs(normalizedRubyText)).run()
    : chain.unsetRuby().run();

  if (!applied) {
    message.warning('当前选区无法应用注音');
    return;
  }

  rememberEditorSelection();
  bumpEditorStateVersion();
  closeRubyModal();
};

const clearRuby = () => {
  rubyTextInput.value = '';
  confirmRuby();
};

const isActive = (name: string | Record<string, any>, attrs?: Record<string, any>) => {
  return (editor.value as any)?.isActive(name, attrs) ?? false;
};

watch(editor, (instance) => {
  if (!instance) return;
  bumpEditorStateVersion();
});

onMounted(() => {
  updateIsMobile();
  window.addEventListener('resize', updateIsMobile);
});

// 初始化
initEditor();
void refreshPlatformFonts();

const markOverlayInteraction = () => {
  overlayInteractionAt.value = Date.now();
};

const hasOpenOverlay = () => {
  return mentionVisible.value
    || highlightColorPopoverShow.value
    || textColorPopoverShow.value
    || blockTypePopoverShow.value
    || fontSizePopoverShow.value
    || performancePopoverShow.value
    || underlineStylePopoverShow.value
    || strikeStylePopoverShow.value
    || fontSelectorExpanded.value
    || desktopFontSelectorExpanded.value
    || linkModalShow.value
    || rubyModalShow.value
    || quickIFormModalShow.value;
};

const hasRecentOverlayInteraction = (thresholdMs = 250) => {
  return Date.now() - overlayInteractionAt.value <= thresholdMs;
};

const handleCompositionStart = () => {
  isComposing.value = true;
  emit('composition-start');
};

const handleCompositionEnd = () => {
  isComposing.value = false;
  emit('composition-end');
  if (editor.value) {
    checkMentionTrigger(editor.value);
  }
};

onBeforeUnmount(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('resize', updateIsMobile);
    window.removeEventListener('resize', handleMentionViewportChange);
    window.removeEventListener('scroll', handleMentionViewportChange, true);
    if (mentionPositionRaf !== null) {
      cancelAnimationFrame(mentionPositionRaf);
      mentionPositionRaf = null;
    }
  }
  editor.value?.destroy();
});

defineExpose({
  focus,
  blur,
  getTextarea,
  getSelectionRange,
  setSelectionRange,
  moveCursorToEnd,
  getInstance: () => editor.value,
  getEditor: () => editor.value,
  getJson: () => editor.value?.getJSON(),
  insertImagePlaceholder,
  applySmartLinkImage,
  hasOpenOverlay,
  hasRecentOverlayInteraction,
});
</script>

<template>
  <div ref="rootRef" :class="classList">
    <div v-if="isInitializing" class="tiptap-loading">
      <n-spin size="small" />
      <span class="ml-2 text-sm text-gray-500">加载编辑器...</span>
    </div>

    <div v-else class="tiptap-wrapper">
      <!-- 固定工具栏 -->
      <div class="tiptap-toolbar">
        <div class="tiptap-toolbar__group">
          <n-popover
            trigger="click"
            placement="bottom-start"
            :show="blockTypePopoverShow"
            :content-class="toolbarPopoverContentClass"
            @update:show="triggerBlockTypePopover"
          >
            <template #trigger>
              <div @pointerdown="markToolbarPickerTriggerInteraction">
                <n-button
                  size="small"
                  text
                  class="tiptap-toolbar-picker-btn"
                  :type="selectedBlockType !== 'paragraph' ? 'primary' : 'default'"
                  title="标题层级"
                >
                  <span class="tiptap-toolbar-picker-btn__value">{{ currentBlockTypeOption.shortLabel }}</span>
                  <span class="tiptap-toolbar-picker-btn__caret">▾</span>
                </n-button>
              </div>
            </template>
            <div :class="toolbarPickerClass" @pointerdown.stop="markOverlayInteraction">
              <button
                v-for="option in blockTypeOptions"
                :key="option.value"
                type="button"
                :class="['tiptap-toolbar-picker__item', { 'is-active': option.value === selectedBlockType }]"
                @mousedown.prevent="applyBlockTypeOption($event, option.value)"
              >
                <span class="tiptap-toolbar-picker__item-label">{{ option.label }}</span>
                <span class="tiptap-toolbar-picker__item-meta">{{ option.shortLabel }}</span>
              </button>
            </div>
          </n-popover>
        </div>

        <div class="tiptap-toolbar__group">
          <n-popover
            trigger="click"
            placement="bottom-start"
            :show="fontSizePopoverShow"
            :content-class="toolbarPopoverContentClass"
            @update:show="triggerFontSizePopover"
          >
            <template #trigger>
              <div @pointerdown="markToolbarPickerTriggerInteraction">
                <n-button
                  size="small"
                  text
                  class="tiptap-toolbar-picker-btn"
                  :type="selectedFontSize ? 'primary' : 'default'"
                  title="字体大小"
                >
                  <span class="tiptap-toolbar-picker-btn__value">{{ currentFontSizeOption.shortLabel }}</span>
                  <span class="tiptap-toolbar-picker-btn__caret">▾</span>
                </n-button>
              </div>
            </template>
            <div :class="toolbarPickerClass" @pointerdown.stop="markOverlayInteraction">
              <button
                v-for="option in fontSizeOptions"
                :key="option.label"
                type="button"
                :class="['tiptap-toolbar-picker__item', { 'is-active': option.value === selectedFontSize }]"
                @mousedown.prevent="applyFontSizeOption($event, option.value)"
              >
                <span class="tiptap-toolbar-picker__item-label">{{ option.label }}</span>
                <span class="tiptap-toolbar-picker__item-meta">{{ option.shortLabel }}</span>
              </button>
              <div class="tiptap-toolbar-picker__custom" @mousedown.stop>
                <n-input
                  v-model:value="customFontSizeInput"
                  size="tiny"
                  placeholder="自定义 px"
                  @keydown.enter.prevent="applyCustomFontSize"
                >
                  <template #suffix>px</template>
                </n-input>
                <n-button size="tiny" secondary @mousedown.prevent="applyCustomFontSize">
                  应用
                </n-button>
              </div>
            </div>
          </n-popover>
        </div>

        <div class="tiptap-toolbar__divider"></div>

        <div class="tiptap-toolbar__group">
          <n-button
            size="small"
            text
            :type="isActive('bold') ? 'primary' : 'default'"
            @click="toggleBold"
            title="粗体 (Ctrl+B)"
          >
            <span class="font-bold">B</span>
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('italic') ? 'primary' : 'default'"
            @click="toggleItalic"
            title="斜体 (Ctrl+I)"
          >
            <span class="italic">I</span>
          </n-button>
          <n-popover
            trigger="manual"
            placement="bottom-start"
            :show="underlineStylePopoverShow"
            :content-class="toolbarPopoverContentClass"
            @update:show="triggerDecorationPopover('underline', $event)"
          >
            <template #trigger>
              <div
                @pointerdown="handleDecorationTriggerPointerDown($event, 'underline')"
                @click.stop.prevent
              >
                <n-button
                  size="small"
                  text
                  :type="isDecorationActive('underline') ? 'primary' : 'default'"
                  title="下划线样式 (Ctrl+U)"
                >
                  <span class="underline">U</span>
                </n-button>
              </div>
            </template>
            <div class="tiptap-decoration-panel" @pointerdown.stop="markOverlayInteraction">
              <div class="tiptap-decoration-panel__header">
                <span class="tiptap-decoration-panel__title">下划线</span>
                <button
                  type="button"
                  class="tiptap-decoration-panel__close"
                  title="关闭"
                  aria-label="关闭下划线样式面板"
                  @mousedown.prevent.stop="closeDecorationPopover('underline')"
                >
                  ×
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">粗细</span>
                <button
                  v-for="option in decorationThicknessOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedUnderlineDecorationStyle.thickness }]"
                  @mousedown.prevent="applyDecorationOption($event, 'underline', 'thickness', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">线型</span>
                <button
                  v-for="option in decorationPatternOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedUnderlineDecorationStyle.pattern }]"
                  @mousedown.prevent="applyDecorationOption($event, 'underline', 'pattern', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">波浪</span>
                <button
                  v-for="option in decorationWaveOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedUnderlineDecorationStyle.wave }]"
                  @mousedown.prevent="applyDecorationOption($event, 'underline', 'wave', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">数量</span>
                <button
                  v-for="option in decorationCountOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedUnderlineDecorationStyle.count }]"
                  @mousedown.prevent="applyDecorationOption($event, 'underline', 'count', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
            </div>
          </n-popover>
          <n-popover
            trigger="manual"
            placement="bottom-start"
            :show="strikeStylePopoverShow"
            :content-class="toolbarPopoverContentClass"
            @update:show="triggerDecorationPopover('line-through', $event)"
          >
            <template #trigger>
              <div
                @pointerdown="handleDecorationTriggerPointerDown($event, 'line-through')"
                @click.stop.prevent
              >
                <n-button
                  size="small"
                  text
                  :type="isDecorationActive('line-through') ? 'primary' : 'default'"
                  title="删除线样式"
                >
                  <span class="line-through">S</span>
                </n-button>
              </div>
            </template>
            <div class="tiptap-decoration-panel" @pointerdown.stop="markOverlayInteraction">
              <div class="tiptap-decoration-panel__header">
                <span class="tiptap-decoration-panel__title">删除线</span>
                <button
                  type="button"
                  class="tiptap-decoration-panel__close"
                  title="关闭"
                  aria-label="关闭删除线样式面板"
                  @mousedown.prevent.stop="closeDecorationPopover('line-through')"
                >
                  ×
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">粗细</span>
                <button
                  v-for="option in decorationThicknessOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedStrikeDecorationStyle.thickness }]"
                  @mousedown.prevent="applyDecorationOption($event, 'line-through', 'thickness', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">线型</span>
                <button
                  v-for="option in decorationPatternOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedStrikeDecorationStyle.pattern }]"
                  @mousedown.prevent="applyDecorationOption($event, 'line-through', 'pattern', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">波浪</span>
                <button
                  v-for="option in decorationWaveOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedStrikeDecorationStyle.wave }]"
                  @mousedown.prevent="applyDecorationOption($event, 'line-through', 'wave', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
              <div class="tiptap-decoration-panel__row">
                <span class="tiptap-decoration-panel__label">数量</span>
                <button
                  v-for="option in decorationCountOptions"
                  :key="option.value"
                  type="button"
                  :class="['tiptap-decoration-panel__chip', { 'is-active': option.value === selectedStrikeDecorationStyle.count }]"
                  @mousedown.prevent="applyDecorationOption($event, 'line-through', 'count', option.value)"
                >
                  <span>{{ option.label }}</span><small>{{ option.meta }}</small>
                </button>
              </div>
            </div>
          </n-popover>
          <n-button
            size="small"
            text
            :type="isActive('spoiler') ? 'primary' : 'default'"
            @click="toggleSpoiler"
            title="隐藏/揭示"
          >
            <span class="font-semibold">SP</span>
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('code') ? 'primary' : 'default'"
            @click="toggleCode"
            title="行内代码"
          >
            <span class="font-mono text-xs">&lt;/&gt;</span>
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('ruby') ? 'primary' : 'default'"
            @click="openRubyModal"
            title="注音 / Ruby"
          >
            Rb
          </n-button>
          <template v-if="isMobile">
            <span ref="performanceTriggerRef">
              <n-button
                size="small"
                text
                :type="isActive('performance') ? 'primary' : 'default'"
                title="文字演出"
                class="tiptap-toolbar-btn"
                @click="performancePopoverShow ? closePerformancePopover() : openPerformancePopover()"
              >
                Fx
              </n-button>
            </span>
          </template>
          <n-popover
            v-else
            trigger="manual"
            placement="bottom"
            :show="performancePopoverShow"
            :content-class="toolbarPopoverContentClass"
          >
            <template #trigger>
              <span ref="performanceTriggerRef">
                <n-button
                  size="small"
                  text
                  :type="isActive('performance') ? 'primary' : 'default'"
                  title="文字演出"
                  class="tiptap-toolbar-btn"
                  @click="performancePopoverShow ? closePerformancePopover() : openPerformancePopover()"
                >
                  Fx
                </n-button>
              </span>
            </template>
            <div class="tiptap-performance-panel" @pointerdown.stop="markOverlayInteraction">
              <div class="tiptap-performance-panel__topbar">
                <div class="tiptap-performance-panel__title">文字演出</div>
                <button type="button" class="tiptap-performance-panel__close" @click="closePerformancePopover">×</button>
              </div>
              <div class="tiptap-performance-panel__section">
                <div class="tiptap-performance-panel__header">
                  <div class="tiptap-performance-panel__label">当前文本块</div>
                  <div class="tiptap-performance-panel__hint">进入方式与语气实时应用到当前块</div>
                </div>
                <div class="tiptap-performance-panel__subsection">
                  <div class="tiptap-performance-panel__label">文本进入</div>
                  <div class="tiptap-performance-panel__chips">
                    <button
                      v-for="option in performanceEnterModeOptions"
                      :key="option.value"
                      type="button"
                      class="tiptap-performance-chip"
                      :class="{ 'is-active': performanceEnterMode === option.value }"
                      @click="setPerformanceEnterMode(option.value)"
                    >{{ option.label }}</button>
                  </div>
                </div>
                <div class="tiptap-performance-panel__slider-grid">
                  <div class="tiptap-performance-panel__subsection">
                    <div class="tiptap-performance-panel__slider-head">
                      <span class="tiptap-performance-panel__label">进入速度</span>
                      <span class="tiptap-performance-panel__value">{{ performanceSpeedLabel }}</span>
                    </div>
                    <n-slider
                      :value="performanceEnterSpeed"
                      :min="1"
                      :max="9"
                      :step="1"
                      @update:value="handlePerformanceEnterSpeedUpdate"
                    />
                    <div class="tiptap-performance-panel__scale">
                      <span>慢</span>
                      <span>中</span>
                      <span>快</span>
                    </div>
                  </div>
                  <div class="tiptap-performance-panel__subsection">
                    <div class="tiptap-performance-panel__slider-head">
                      <span class="tiptap-performance-panel__label">语气尺度</span>
                      <span class="tiptap-performance-panel__value">{{ performanceToneLabel }}</span>
                    </div>
                    <n-slider
                      :value="performanceToneIntensity"
                      :min="-4"
                      :max="4"
                      :step="1"
                      :marks="performanceToneMarks"
                      @update:value="handlePerformanceToneIntensityUpdate"
                    />
                  </div>
                </div>
              </div>
              <div class="tiptap-performance-panel__section">
                <div class="tiptap-performance-panel__header">
                  <div class="tiptap-performance-panel__label">选区文字效果</div>
                  <div class="tiptap-performance-panel__hint">仅作用于当前选区文字</div>
                </div>
                <div class="tiptap-performance-panel__label">文字效果</div>
                <div class="tiptap-performance-panel__chips">
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'wave' }" @click="performanceEffect = 'wave'">波浪</button>
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'shake' }" @click="performanceEffect = 'shake'">抖动</button>
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'rainbow' }" @click="performanceEffect = 'rainbow'">虹彩</button>
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'glitch' }" @click="performanceEffect = 'glitch'">故障</button>
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'blink' }" @click="performanceEffect = 'blink'">闪烁</button>
                </div>
                <n-button size="tiny" type="primary" @click="applyPerformanceEffectToSelection">应用文字效果到选区</n-button>
              </div>
              <div class="tiptap-performance-panel__section">
                <div class="tiptap-performance-panel__header">
                  <div class="tiptap-performance-panel__label">节奏命令</div>
                  <div class="tiptap-performance-panel__hint">仅在朦胧显现 / 逐字时生效</div>
                </div>
                <div class="tiptap-performance-panel__chips">
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceCommandType === 'delay' }" @click="performanceCommandType = 'delay'">停顿</button>
                  <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceCommandType === 'pause' }" @click="performanceCommandType = 'pause'">暂停并高亮</button>
                </div>
                <div class="tiptap-performance-panel__command-row">
                  <n-input
                    v-if="performanceCommandType === 'delay'"
                    v-model:value="performanceCommandValue"
                    size="small"
                    placeholder="停顿毫秒，例如 500"
                  />
                </div>
                <n-button size="tiny" secondary @click="insertPerformanceCommandNode">插入命令</n-button>
              </div>
            </div>
          </n-popover>
          <!-- 高亮颜色选择器 -->
          <n-popover
            trigger="click"
            placement="bottom"
            v-model:show="highlightColorPopoverShow"
            :content-class="toolbarPopoverContentClass"
          >
            <template #trigger>
              <n-button
                size="small"
                text
                :type="isActive('highlight') ? 'primary' : 'default'"
                title="高亮颜色"
                class="tiptap-toolbar-btn"
              >
                <span class="tiptap-highlight-icon">H</span>
              </n-button>
            </template>
            <div class="tiptap-color-picker" @pointerdown.stop="markOverlayInteraction">
              <div
                v-for="color in highlightColors"
                :key="color"
                class="tiptap-color-swatch"
                :class="{ 'is-active': getActiveHighlightColor() === color }"
                :style="{ backgroundColor: color }"
                @click="setHighlightColor(color)"
                :title="color"
              ></div>
              <!-- 自定义颜色选择器 -->
              <label class="tiptap-color-swatch tiptap-color-custom" title="自定义颜色">
                <input
                  type="color"
                  v-model="customHighlightColor"
                  @change="applyCustomHighlightColor"
                  class="tiptap-color-input"
                />
                <span class="tiptap-color-custom__icon">+</span>
              </label>
              <div class="tiptap-color-picker__clear" @click="removeHighlight">
                清除高亮
              </div>
            </div>
          </n-popover>
          <!-- 文字颜色选择器 -->
          <n-popover
            trigger="click"
            placement="bottom"
            v-model:show="textColorPopoverShow"
            :content-class="toolbarPopoverContentClass"
          >
            <template #trigger>
              <n-button
                size="small"
                text
                :type="getActiveTextColor() ? 'primary' : 'default'"
                title="文字颜色"
                class="tiptap-toolbar-btn"
              >
                <span class="tiptap-textcolor-icon">A</span>
              </n-button>
            </template>
            <div class="tiptap-color-picker" @pointerdown.stop="markOverlayInteraction">
              <div
                v-for="color in textColors"
                :key="color"
                class="tiptap-color-swatch"
                :class="{ 'is-active': getActiveTextColor() === color }"
                :style="{ backgroundColor: color }"
                @click="setTextColor(color)"
                :title="color"
              ></div>
              <!-- 自定义颜色选择器 -->
              <label class="tiptap-color-swatch tiptap-color-custom" title="自定义颜色">
                <input
                  type="color"
                  v-model="customTextColor"
                  @change="applyCustomTextColor"
                  class="tiptap-color-input"
                />
                <span class="tiptap-color-custom__icon">+</span>
              </label>
              <div class="tiptap-color-picker__clear" @click="removeTextColor">
                清除颜色
              </div>
            </div>
          </n-popover>
        </div>

        <div class="tiptap-toolbar__group tiptap-toolbar__group--font" :class="{ 'is-expanded': !isMobile && desktopFontSelectorExpanded }">
          <n-button
            v-if="isMobile && !fontSelectorExpanded"
            size="small"
            text
            class="tiptap-platform-font-toggle"
            title="选择字体"
            @click="toggleFontSelectorExpanded"
          >
            A
          </n-button>
          <n-select
            v-else
            size="small"
            class="tiptap-platform-font-select"
            clearable
            filterable
            :loading="platformFontLoading"
            :value="selectedPlatformFontId"
            :options="platformFontOptions"
            :placeholder="isMobile ? '字体' : '平台字体'"
            :render-label="renderPlatformFontLabel"
            :render-option="renderPlatformFontOption"
            :menu-props="platformFontSelectMenuProps"
            @update:value="applyPlatformFont"
            @update:show="handlePlatformFontSelectShowUpdate"
            @focus="desktopFontSelectorExpanded = true"
            @blur="closeFontSelector"
          />
        </div>

        <div class="tiptap-toolbar__divider"></div>

        <div class="tiptap-toolbar__group">
          <n-button
            size="small"
            text
            :type="isActive({ textAlign: 'left' }) ? 'primary' : 'default'"
            @click="setTextAlign('left')"
            title="左对齐"
          >
            ≡
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive({ textAlign: 'center' }) ? 'primary' : 'default'"
            @click="setTextAlign('center')"
            title="居中"
          >
            ≣
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive({ textAlign: 'right' }) ? 'primary' : 'default'"
            @click="setTextAlign('right')"
            title="右对齐"
          >
            ≣
          </n-button>
        </div>

        <div class="tiptap-toolbar__divider"></div>

        <div class="tiptap-toolbar__group">
          <n-button
            size="small"
            text
            :type="isActive('bulletList') ? 'primary' : 'default'"
            @click="toggleBulletList"
            title="无序列表"
          >
            •
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('orderedList') ? 'primary' : 'default'"
            @click="toggleOrderedList"
            title="有序列表"
          >
            1.
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('blockquote') ? 'primary' : 'default'"
            @click="toggleBlockquote"
            title="引用"
          >
            "
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('codeBlock') ? 'primary' : 'default'"
            @click="toggleCodeBlock"
            title="代码块"
          >
            { }
          </n-button>
        </div>

        <div class="tiptap-toolbar__divider"></div>

        <div class="tiptap-toolbar__group">
          <n-button
            size="small"
            text
            :type="isActive('link') ? 'primary' : 'default'"
            @click="isActive('link') ? unsetLink() : setLink()"
            :title="isActive('link') ? '移除链接' : '插入链接'"
          >
            🔗
          </n-button>
          <n-button
            size="small"
            text
            @click="emit('upload-button-click', 'rich-editor')"
            title="插入图片"
          >
            🖼
          </n-button>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button
                size="small"
                text
                @click="insertStateWidgetTemplate"
                title="插入三段状态文本"
              >
                ◫
              </n-button>
            </template>
            插入三段状态文本：`[选项1|选项2|选项3]`
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button
                size="small"
                text
                :disabled="!canQuickCreateIForm"
                @click="insertIFormEmbedLink"
                title="创建并插入 iForm 嵌入"
              >
                ⧉
              </n-button>
            </template>
            {{ canQuickCreateIForm ? '弹窗创建 iForm 并自动插入链接' : '当前频道无权限或不可创建 iForm' }}
          </n-tooltip>
          <n-button
            size="small"
            text
            @click="insertHorizontalRule"
            title="分割线"
          >
            ―
          </n-button>
          <n-button
            size="small"
            text
            @click="clearFormatting"
            title="清除格式"
          >
            ⊗
          </n-button>
        </div>
      </div>

      <!-- 编辑器内容区 -->
      <div
        class="tiptap-editor-wrapper"
        ref="editorElement"
        @compositionstart="handleCompositionStart"
        @compositionend="handleCompositionEnd"
      >
        <component :is="EditorContent" v-if="editor" :editor="editor" />

        <Teleport to="body">
          <Transition name="mention-fade">
            <div
              v-if="mentionVisible"
              class="mention-dropdown"
              data-sc-font-surface="true"
              :style="mentionDropdownStyle"
              tabindex="-1"
              ref="mentionDropdownRef"
              @pointerdown.stop="markOverlayInteraction"
            >
              <input
                v-model="mentionSearchValue"
                class="mention-dropdown__search"
                type="text"
                placeholder="搜索成员"
                @keydown="handleMentionSearchKeydown"
                @pointerdown.stop
              />
              <div
                v-for="(option, index) in mentionFilteredOptions"
                :key="option.value"
                :class="['mention-dropdown__item', { 'is-active': index === mentionActiveIndex }]"
                @pointerdown.stop
                @mousedown.prevent="handleMentionSelect(option)"
                @mouseenter="handleMentionHover(index)"
              >
                <component
                  :is="mentionRenderLabel ? mentionRenderLabel(option) : undefined"
                  v-if="mentionRenderLabel"
                />
                <span v-else>{{ option.label }}</span>
              </div>
              <div v-if="mentionLoading" class="mention-dropdown__loading">
                加载中...
              </div>
              <div v-else-if="mentionFilteredOptions.length === 0" class="mention-dropdown__empty">
                无匹配成员
              </div>
            </div>
          </Transition>
        </Teleport>

        <!-- BubbleMenu 浮动工具栏 -->
        <component
          v-if="editor && BubbleMenu"
          :is="BubbleMenu"
          :editor="editor"
          :tippy-options="{ duration: 100, placement: 'top' }"
        >
          <div class="tiptap-bubble-menu" @pointerdown.stop="markOverlayInteraction">
            <n-button
              size="tiny"
              text
              :type="isActive('bold') ? 'primary' : 'default'"
              @click="toggleBold"
              title="粗体"
            >
              <span class="font-bold">B</span>
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isActive('italic') ? 'primary' : 'default'"
              @click="toggleItalic"
              title="斜体"
            >
              <span class="italic">I</span>
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isDecorationActive('underline') ? 'primary' : 'default'"
              @click="toggleUnderline"
              title="下划线"
            >
              <span class="underline">U</span>
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isDecorationActive('line-through') ? 'primary' : 'default'"
              @click="toggleStrike"
              title="删除线"
            >
              <span class="line-through">S</span>
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isActive('spoiler') ? 'primary' : 'default'"
              @click="toggleSpoiler"
              title="隐藏/揭示"
            >
              <span class="font-semibold">SP</span>
            </n-button>
            <div class="tiptap-bubble-menu__divider"></div>
            <n-button
              size="tiny"
              text
              :type="isActive('link') ? 'primary' : 'default'"
              @click="isActive('link') ? unsetLink() : setLink()"
              :title="isActive('link') ? '移除链接' : '插入链接'"
            >
              🔗
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isActive('code') ? 'primary' : 'default'"
              @click="toggleCode"
              title="代码"
            >
              <span class="font-mono text-xs">&lt;/&gt;</span>
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isActive('ruby') ? 'primary' : 'default'"
              @click="openRubyModal"
              title="注音 / Ruby"
            >
              Rb
            </n-button>
            <n-button
              size="tiny"
              text
              :type="isActive('performance') ? 'primary' : 'default'"
              @click="openPerformancePopover"
              title="文字演出"
            >
              Fx
            </n-button>
          </div>
        </component>
      </div>
    </div>

    <n-modal
      v-model:show="rubyModalShow"
      preset="card"
      :bordered="false"
      title="插入注音"
      style="width: 360px; max-width: 90vw;"
      :mask-closable="true"
      @pointerdown.stop="markOverlayInteraction"
    >
      <n-form label-placement="top">
        <n-form-item label="正文">
          <n-input
            v-model:value="rubyBaseText"
            :readonly="rubySelectionMode !== 'insert'"
            :placeholder="rubySelectionMode === 'insert' ? '请输入正文' : '当前选区正文'"
          />
        </n-form-item>
        <n-form-item label="注音">
          <n-input
            v-model:value="rubyTextInput"
            placeholder="请输入注音文字"
            @keydown.enter.prevent="confirmRuby"
          />
        </n-form-item>
        <div class="ruby-modal__advanced">
          <button
            type="button"
            class="ruby-modal__toggle"
            @click="rubyFontPanelExpanded = !rubyFontPanelExpanded"
          >
            <span>不同字体</span>
            <span>{{ rubyFontPanelExpanded ? '▾' : '▸' }}</span>
          </button>
          <div v-if="rubyFontPanelExpanded" class="ruby-modal__panel">
            <n-form-item label="上方注音字体">
              <n-select
                clearable
                filterable
                :loading="platformFontLoading"
                :value="rubyRtFontId"
                :options="rubyRtFontOptions"
                placeholder="默认跟随正文"
                :render-label="renderRubyRtFontLabel"
                :render-option="renderRubyRtFontOption"
                :menu-props="platformFontSelectMenuProps"
                @update:value="rubyRtFontId = $event"
                @update:show="handleRubyRtFontShowUpdate"
              />
            </n-form-item>
            <n-form-item label="下方正文字体">
              <n-select
                clearable
                filterable
                :loading="platformFontLoading"
                :value="rubyBaseFontId"
                :options="rubyBaseFontOptions"
                placeholder="默认跟随当前文字"
                :render-label="renderRubyBaseFontLabel"
                :render-option="renderRubyBaseFontOption"
                :menu-props="platformFontSelectMenuProps"
                @update:value="rubyBaseFontId = $event"
                @update:show="handleRubyBaseFontShowUpdate"
              />
            </n-form-item>
          </div>
        </div>
        <div class="ruby-modal__advanced">
          <button
            type="button"
            class="ruby-modal__toggle"
            @click="rubySizePanelExpanded = !rubySizePanelExpanded"
          >
            <span>不同字号</span>
            <span>{{ rubySizePanelExpanded ? '▾' : '▸' }}</span>
          </button>
          <div v-if="rubySizePanelExpanded" class="ruby-modal__panel ruby-modal__panel--size">
            <n-form-item label="上方注音字号">
              <n-input
                v-model:value="rubyRtFontSizeInput"
                placeholder="默认跟随当前文字"
                @keydown.enter.prevent="confirmRuby"
              >
                <template #suffix>px</template>
              </n-input>
            </n-form-item>
            <n-form-item label="下方正文字号">
              <n-input
                v-model:value="rubyBaseFontSizeInput"
                placeholder="默认跟随当前文字"
                @keydown.enter.prevent="confirmRuby"
              >
                <template #suffix>px</template>
              </n-input>
            </n-form-item>
          </div>
        </div>
        <div class="ruby-modal__note">
          <div v-if="rubySelectionMode === 'insert'">插入模式：输入正文与注音后插入到当前光标位置。</div>
          <div v-else-if="rubySelectionMode === 'edit'">编辑模式：修改当前选区注音，或清除注音保留正文。</div>
          <div v-else>应用模式：当前选区正文保持不变，仅应用注音。</div>
        </div>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
          <n-button @click="closeRubyModal">取消</n-button>
          <n-button
            v-if="rubySelectionMode !== 'insert'"
            @click="clearRuby"
          >
            清除注音
          </n-button>
          <n-button type="primary" @click="confirmRuby">
            确定
          </n-button>
        </div>
      </template>
    </n-modal>

    <Teleport to="body">
      <Transition name="mention-fade">
        <div
          v-if="isMobile && performancePopoverShow"
          class="tiptap-performance-sheet"
          @click="closePerformancePopover"
        >
          <div
            class="tiptap-performance-sheet__panel"
            @click.stop
            @pointerdown.stop="markOverlayInteraction"
          >
            <div class="tiptap-performance-panel tiptap-performance-panel--mobile">
              <div class="tiptap-performance-panel__topbar">
                <div class="tiptap-performance-panel__title">文字演出</div>
                <button type="button" class="tiptap-performance-panel__close" @click="closePerformancePopover">×</button>
              </div>
              <div class="tiptap-performance-sheet__body">
                <div class="tiptap-performance-panel__section">
                  <div class="tiptap-performance-panel__header">
                    <div class="tiptap-performance-panel__label">当前文本块</div>
                    <div class="tiptap-performance-panel__hint">进入方式与语气实时应用到当前块</div>
                  </div>
                  <div class="tiptap-performance-panel__subsection">
                    <div class="tiptap-performance-panel__label">文本进入</div>
                    <div class="tiptap-performance-panel__chips">
                      <button
                        v-for="option in performanceEnterModeOptions"
                        :key="option.value"
                        type="button"
                        class="tiptap-performance-chip"
                        :class="{ 'is-active': performanceEnterMode === option.value }"
                        @click="setPerformanceEnterMode(option.value)"
                      >{{ option.label }}</button>
                    </div>
                  </div>
                  <div class="tiptap-performance-panel__slider-grid">
                    <div class="tiptap-performance-panel__subsection">
                      <div class="tiptap-performance-panel__slider-head">
                        <span class="tiptap-performance-panel__label">进入速度</span>
                        <span class="tiptap-performance-panel__value">{{ performanceSpeedLabel }}</span>
                      </div>
                      <n-slider
                        :value="performanceEnterSpeed"
                        :min="1"
                        :max="9"
                        :step="1"
                        @update:value="handlePerformanceEnterSpeedUpdate"
                      />
                      <div class="tiptap-performance-panel__scale">
                        <span>慢</span>
                        <span>中</span>
                        <span>快</span>
                      </div>
                    </div>
                    <div class="tiptap-performance-panel__subsection">
                      <div class="tiptap-performance-panel__slider-head">
                        <span class="tiptap-performance-panel__label">语气尺度</span>
                        <span class="tiptap-performance-panel__value">{{ performanceToneLabel }}</span>
                      </div>
                      <n-slider
                        :value="performanceToneIntensity"
                        :min="-4"
                        :max="4"
                        :step="1"
                        :marks="performanceToneMarks"
                        @update:value="handlePerformanceToneIntensityUpdate"
                      />
                    </div>
                  </div>
                </div>
                <div class="tiptap-performance-panel__section">
                  <div class="tiptap-performance-panel__header">
                    <div class="tiptap-performance-panel__label">选区文字效果</div>
                    <div class="tiptap-performance-panel__hint">仅作用于当前选区文字</div>
                  </div>
                  <div class="tiptap-performance-panel__label">文字效果</div>
                  <div class="tiptap-performance-panel__chips">
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'wave' }" @click="performanceEffect = 'wave'">波浪</button>
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'shake' }" @click="performanceEffect = 'shake'">抖动</button>
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'rainbow' }" @click="performanceEffect = 'rainbow'">虹彩</button>
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'glitch' }" @click="performanceEffect = 'glitch'">故障</button>
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceEffect === 'blink' }" @click="performanceEffect = 'blink'">闪烁</button>
                  </div>
                  <n-button size="tiny" type="primary" @click="applyPerformanceEffectToSelection">应用文字效果到选区</n-button>
                </div>
                <div class="tiptap-performance-panel__section">
                  <div class="tiptap-performance-panel__header">
                    <div class="tiptap-performance-panel__label">节奏命令</div>
                    <div class="tiptap-performance-panel__hint">仅在朦胧显现 / 逐字时生效</div>
                  </div>
                  <div class="tiptap-performance-panel__chips">
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceCommandType === 'delay' }" @click="performanceCommandType = 'delay'">停顿</button>
                    <button type="button" class="tiptap-performance-chip" :class="{ 'is-active': performanceCommandType === 'pause' }" @click="performanceCommandType = 'pause'">暂停并高亮</button>
                  </div>
                  <div class="tiptap-performance-panel__command-row">
                    <n-input
                      v-if="performanceCommandType === 'delay'"
                      v-model:value="performanceCommandValue"
                      size="small"
                      placeholder="停顿毫秒，例如 500"
                    />
                  </div>
                  <n-button size="tiny" secondary @click="insertPerformanceCommandNode">插入命令</n-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 链接插入弹窗 -->
    <n-modal
      v-model:show="linkModalShow"
      preset="card"
      :bordered="false"
      title="插入链接"
      style="width: 360px; max-width: 90vw;"
      :mask-closable="true"
      @pointerdown.stop="markOverlayInteraction"
    >
      <n-form label-placement="top">
        <n-form-item label="链接文本">
          <div class="smart-link-modal__field">
            <n-input
              v-model:value="linkText"
              :disabled="linkTextType === 'image'"
              placeholder="显示的文字（可选，留空则显示链接地址）"
            />
            <div class="smart-link-modal__actions">
              <n-button size="tiny" secondary @click="requestSmartLinkImageUpload('smart-link-text-image')">
                上传文本图片
              </n-button>
              <n-button
                v-if="linkTextType === 'image'"
                size="tiny"
                quaternary
                @click="clearSmartLinkImage('text')"
              >
                改用文字
              </n-button>
            </div>
            <div v-if="linkTextType === 'image' && linkTextImage" class="smart-link-modal__preview">
              <img :src="linkTextImage" alt="链接文本图片" class="smart-link-modal__preview-image">
            </div>
          </div>
        </n-form-item>
        <n-form-item label="链接地址">
          <div class="smart-link-modal__field">
            <n-input
              v-model:value="linkUrl"
              :disabled="linkUrlType === 'image'"
              placeholder="https://example.com"
              @keydown.enter="confirmLink"
            />
            <div class="smart-link-modal__actions">
              <n-button size="tiny" secondary @click="requestSmartLinkImageUpload('smart-link-url-image')">
                上传目标图片
              </n-button>
              <n-button
                v-if="linkUrlType === 'image'"
                size="tiny"
                quaternary
                @click="clearSmartLinkImage('url')"
              >
                改用网址
              </n-button>
            </div>
            <div v-if="linkUrlType === 'image' && linkUrlImage" class="smart-link-modal__preview">
              <img :src="linkUrlImage" alt="链接目标图片" class="smart-link-modal__preview-image">
            </div>
          </div>
        </n-form-item>
        <n-form-item v-if="linkUrlType === 'url'" label="打开方式">
          <n-checkbox v-model:checked="linkOpenInNewTab">在新标签页中打开</n-checkbox>
        </n-form-item>
        <div class="smart-link-modal__note">
          <div>图片文本 + 普通链接：点击图片打开链接</div>
          <div>文字文本 + 图片链接：点击文字查看大图</div>
          <div>图片文本 + 图片链接：点击图片查看目标图</div>
        </div>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
          <n-button @click="closeLinkModal">取消</n-button>
          <n-button
            type="primary"
            @click="confirmLink"
            :disabled="linkUrlType === 'url' ? !linkUrl.trim() : !linkUrlImage.trim()"
          >
            确定
          </n-button>
        </div>
      </template>
    </n-modal>


    <n-modal
      v-model:show="quickIFormModalShow"
      preset="card"
      :bordered="false"
      title="创建消息嵌入 iForm"
      style="width: 460px; max-width: 92vw;"
      :mask-closable="!creatingIForm"
      @pointerdown.stop="markOverlayInteraction"
    >
      <n-form label-placement="top">
        <n-form-item label="名称">
          <n-input
            v-model:value="quickIFormForm.name"
            placeholder="示例：战斗地图 / 音乐播放器"
          />
        </n-form-item>
        <n-form-item label="URL">
          <n-input
            v-model:value="quickIFormForm.url"
            placeholder="https://example.com"
          />
        </n-form-item>
        <n-form-item label="嵌入代码">
          <n-input
            type="textarea"
            v-model:value="quickIFormForm.embedCode"
            placeholder="可选：粘贴 HTML / iframe 代码（可含 script）"
            :rows="3"
          />
        </n-form-item>
        <n-form-item label="默认尺寸">
          <div style="display: flex; gap: 0.5rem; width: 100%;">
            <n-input-number v-model:value="quickIFormForm.defaultWidth" :min="120" :step="10" style="flex: 1;" placeholder="宽度" />
            <n-input-number v-model:value="quickIFormForm.defaultHeight" :min="72" :step="10" style="flex: 1;" placeholder="高度" />
          </div>
        </n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
          <n-button :disabled="creatingIForm" @click="quickIFormModalShow = false">取消</n-button>
          <n-button type="primary" :loading="creatingIForm" @click="confirmQuickIFormCreate">创建并插入</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style lang="scss" scoped>
.tiptap-editor {
  width: 100%;
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  border-radius: 0.85rem;
  background-color: var(--sc-bg-input, #f9fafb);
  overflow: hidden;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;

  &.is-focused {
    border-color: var(--primary-color, #3b82f6);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--primary-color, #3b82f6) 32%, transparent);
  }

  &.whisper-mode {
    border-color: var(--chat-whisper-border, rgba(124, 58, 237, 0.8));
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--chat-whisper-border, rgba(124, 58, 237, 0.8)) 45%, transparent);
    background-color: var(--chat-whisper-bg, rgba(250, 245, 255, 0.92));
  }
}

.tiptap-editor.chat-input--fullscreen {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.tiptap-editor.chat-input--fullscreen .tiptap-wrapper {
  flex: 1 1 auto;
  min-height: 0;
}

.tiptap-editor.chat-input--expanded .tiptap-editor-wrapper {
  min-height: calc(100vh / 3);
  max-height: calc(100vh / 3);
}

.tiptap-editor.chat-input--expanded .tiptap-content {
  min-height: max(6rem, calc(100vh / 3 - 2.5rem));
  max-height: max(6rem, calc(100vh / 3 - 2.5rem));
}

.tiptap-editor.chat-input--fullscreen .tiptap-editor-wrapper {
  flex: 1 1 auto;
  min-height: 100%;
  max-height: 100%;
  height: 100%;
  overflow-y: auto;
  touch-action: pan-y;
  min-height: 0;
}

.tiptap-editor.chat-input--fullscreen .tiptap-content {
  min-height: max(6rem, calc(100% - 2.5rem));
  max-height: max(6rem, calc(100% - 2.5rem));
}

.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection),
.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection-label) {
  --n-color: rgba(255, 255, 255, 0.82) !important;
  --n-color-active: #ffffff !important;
  --n-color-focus: #ffffff !important;
  --n-text-color: #0f172a !important;
  --n-placeholder-color: #64748b !important;
  background-color: rgba(255, 255, 255, 0.82) !important;
  color: #0f172a !important;
}

.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection:hover),
.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection--active) {
  background-color: #ffffff !important;
}

.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection-input),
.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection-input__content),
.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection-placeholder),
.tiptap-editor--sticky-note-surface :deep(.tiptap-platform-font-select .n-base-selection__state-border) {
  color: #0f172a !important;
}

.tiptap-editor.chat-input--custom-height .tiptap-editor-wrapper {
  min-height: var(--custom-input-height, 3rem);
  max-height: var(--custom-input-height, 12rem);
}

.tiptap-editor.chat-input--custom-height .tiptap-content {
  min-height: max(3rem, calc(var(--custom-input-height, 3rem) - 2.5rem));
  max-height: max(3rem, calc(var(--custom-input-height, 12rem) - 2.5rem));
}

.tiptap-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}

.tiptap-wrapper {
  display: flex;
  flex-direction: column;
}

.tiptap-toolbar {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid var(--sc-border-mute, #e5e7eb);
  background-color: var(--sc-bg-elevated, #ffffff);
  flex-wrap: wrap;
}

.tiptap-toolbar__group {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.tiptap-toolbar-picker-btn {
  gap: 0.2rem;
}

.tiptap-toolbar-picker-btn__value {
  min-width: 1.25rem;
  text-align: center;
  font-size: 0.8125rem;
  font-weight: 600;
  line-height: 1;
}

.tiptap-toolbar-picker-btn__caret {
  font-size: 0.625rem;
  opacity: 0.75;
  line-height: 1;
}

.tiptap-toolbar__group--font {
  flex: 0 0 7.5rem;
  min-width: 7.5rem;
  transition: flex-basis 0.18s ease, min-width 0.18s ease;
}

.tiptap-toolbar__group--font.is-expanded {
  flex-basis: 12.5rem;
  min-width: 12.5rem;
}

.tiptap-platform-font-select {
  width: 100%;
  min-width: 0;
}

.tiptap-platform-font-toggle {
  flex: 0 0 auto;
}

.tiptap-toolbar__divider {
  width: 1px;
  height: 1.25rem;
  background-color: var(--sc-border-mute, #e5e7eb);
  margin: 0 0.25rem;
}

.tiptap-editor-wrapper {
  position: relative;
  min-height: 3rem;
  max-height: 12rem;
  overflow-y: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;

  /* 极简滚动条样式 - Webkit (Chrome, Safari, Edge) */
  &::-webkit-scrollbar {
    width: 4px;
  }

  &::-webkit-scrollbar-track {
    background: transparent;
  }

  &::-webkit-scrollbar-thumb {
    background: rgba(148, 163, 184, 0.35);
    border-radius: 2px;
  }

  &::-webkit-scrollbar-thumb:hover {
    background: rgba(148, 163, 184, 0.55);
  }

  /* Firefox */
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.35) transparent;
}

.mention-dropdown {
  position: fixed;
  max-height: min(240px, 45vh);
  overflow-y: auto;
  background: var(--sc-bg-surface, #ffffff);
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  border-radius: 8px;
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.18);
  z-index: 4200;

  &__search {
    width: calc(100% - 16px);
    margin: 8px;
    padding: 6px 8px;
    border: 1px solid var(--sc-border-mute, #e5e7eb);
    border-radius: 6px;
    background: var(--sc-bg-input, #ffffff);
    color: var(--text-color-1);
    font-size: 0.75rem;
    outline: none;
  }

  &__search:focus {
    border-color: rgba(99, 102, 241, 0.6);
    box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.16);
  }

  &__item {
    display: flex;
    align-items: center;
    padding: 8px 12px;
    cursor: pointer;
    transition: background-color 0.15s ease;

    &:hover,
    &.is-active {
      background-color: var(--sc-bg-hover, rgba(59, 130, 246, 0.08));
    }

    &.is-active {
      background-color: var(--sc-bg-active, rgba(59, 130, 246, 0.12));
    }
  }

  &__loading {
    padding: 8px 12px;
    color: var(--sc-text-secondary, #6b7280);
    font-size: 0.875rem;
    text-align: center;
  }

  &__empty {
    padding: 8px 12px;
    color: var(--sc-text-secondary, #9ca3af);
    font-size: 0.875rem;
    text-align: center;
  }
}

.mention-fade-enter-active,
.mention-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.mention-fade-enter-from,
.mention-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

.tiptap-bubble-menu {
  display: flex;
  gap: 0.25rem;
  padding: 0.375rem 0.5rem;
  background: var(--sc-bg-elevated, #ffffff);
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  border-radius: 0.5rem;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  align-items: center;
  color: var(--sc-text-primary, #0f172a);
}

.tiptap-bubble-menu__divider {
  width: 1px;
  height: 1rem;
  background-color: var(--sc-border-mute, #e5e7eb);
  margin: 0 0.25rem;
}

.tiptap-toolbar-picker {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  min-width: 9rem;
  padding: 0.375rem;
}

.tiptap-toolbar-picker__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  width: 100%;
  border: 0;
  border-radius: 0.5rem;
  background: transparent;
  padding: 0.45rem 0.6rem;
  color: var(--sc-text-primary, #0f172a);
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.tiptap-toolbar-picker__item:hover,
.tiptap-toolbar-picker__item.is-active {
  background: rgba(59, 130, 246, 0.1);
}

.tiptap-toolbar-picker__item.is-active {
  color: #2563eb;
}

.tiptap-toolbar-picker__item-label {
  font-size: 0.8125rem;
  text-align: left;
}

.tiptap-toolbar-picker__item-meta {
  font-size: 0.75rem;
  opacity: 0.72;
}

.tiptap-toolbar-picker__custom {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.2rem 0.15rem;
  border-top: 1px solid var(--sc-border-mute, #e5e7eb);
  margin-top: 0.15rem;
}

.tiptap-toolbar-picker__custom :deep(.n-input) {
  flex: 1 1 auto;
}

.tiptap-decoration-panel {
  display: grid;
  gap: 0.45rem;
  min-width: 17rem;
  max-width: min(19rem, calc(100vw - 2rem));
  padding: 0.45rem;
}

.tiptap-decoration-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  min-height: 1.75rem;
  padding: 0 0.1rem 0.2rem;
}

.tiptap-decoration-panel__title {
  color: var(--sc-text-primary, #0f172a);
  font-size: 0.78rem;
  font-weight: 700;
}

.tiptap-decoration-panel__close {
  display: inline-grid;
  place-items: center;
  width: 1.45rem;
  height: 1.45rem;
  border: 0;
  border-radius: 0.35rem;
  background: transparent;
  color: var(--sc-text-secondary, #64748b);
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
}

.tiptap-decoration-panel__close:hover {
  background: color-mix(in srgb, var(--primary-color, #3b82f6) 10%, transparent);
  color: var(--sc-text-primary, #0f172a);
}

.tiptap-decoration-panel__row {
  display: grid;
  grid-template-columns: 2.5rem repeat(3, minmax(0, 1fr));
  align-items: center;
  gap: 0.3rem;
}

.tiptap-decoration-panel__label {
  color: var(--sc-text-secondary, #64748b);
  font-size: 0.72rem;
  font-weight: 700;
}

.tiptap-decoration-panel__chip {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.05rem;
  min-height: 2.15rem;
  border: 1px solid transparent;
  border-radius: 0.45rem;
  background: transparent;
  color: var(--sc-text-primary, #0f172a);
  font-size: 0.75rem;
  line-height: 1.05;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.tiptap-decoration-panel__chip small {
  font-size: 0.65rem;
  opacity: 0.68;
}

.tiptap-decoration-panel__chip:hover,
.tiptap-decoration-panel__chip.is-active {
  border-color: color-mix(in srgb, var(--primary-color, #3b82f6) 32%, transparent);
  background: color-mix(in srgb, var(--primary-color, #3b82f6) 12%, transparent);
}

.tiptap-decoration-panel__chip.is-active {
  color: var(--primary-color, #2563eb);
}

.tiptap-performance-panel {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
  min-width: 20rem;
  max-width: min(24rem, calc(100vw - 2rem));
  padding: 0.75rem;
}

.tiptap-performance-panel__topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.tiptap-performance-panel__title {
  font-size: 0.86rem;
  font-weight: 700;
  color: var(--sc-text-primary, #0f172a);
}

.tiptap-performance-panel__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.6rem;
  height: 1.6rem;
  border: 0;
  border-radius: 999px;
  background: color-mix(in srgb, var(--sc-bg-layer, #2f2f34) 85%, transparent);
  color: var(--sc-text-secondary, #94a3b8);
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
  transition: background-color 0.16s ease, color 0.16s ease;
}

.tiptap-performance-panel__close:hover {
  background: color-mix(in srgb, var(--primary-color, #60a5fa) 18%, transparent);
  color: var(--sc-text-primary, #0f172a);
}

.tiptap-performance-panel__section {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  padding: 0.1rem 0;
}

.tiptap-performance-panel__header {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.tiptap-performance-panel__hint {
  font-size: 0.72rem;
  color: var(--sc-text-secondary, #94a3b8);
}

.tiptap-performance-panel__subsection {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.tiptap-performance-panel__slider-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.7rem;
}

.tiptap-performance-panel__label {
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--sc-text-secondary, #64748b);
}

.tiptap-performance-panel__slider-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.tiptap-performance-panel__value {
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--sc-text-primary, #0f172a);
}

.tiptap-performance-panel__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.tiptap-performance-panel__scale {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0 0.1rem;
  font-size: 0.72rem;
  color: var(--sc-text-secondary, #94a3b8);
}

.tiptap-performance-chip {
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  background: var(--sc-bg-surface, #fff);
  color: var(--sc-text-primary, #0f172a);
  border-radius: 999px;
  padding: 0.35rem 0.65rem;
  font-size: 0.75rem;
  cursor: pointer;
}

.tiptap-performance-chip.is-active {
  background: rgba(37, 99, 235, 0.12);
  border-color: rgba(37, 99, 235, 0.35);
  color: #1d4ed8;
}

.tiptap-performance-panel__command-row {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.tiptap-performance-panel :deep(.n-slider) {
  margin: 0.15rem 0 0.15rem;
}

@media (min-width: 880px) {
  .tiptap-performance-panel__slider-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/* 颜色选择器样式 */
.tiptap-color-picker {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.375rem;
  padding: 0.5rem;
  min-width: 8rem;
}

.tiptap-color-swatch {
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(0, 0, 0, 0.1);
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease;

  &:hover {
    transform: scale(1.15);
  }

  &.is-active {
    box-shadow: 0 0 0 2px #3b82f6;
  }
}

.tiptap-color-picker__clear {
  grid-column: span 4;
  padding: 0.375rem 0.25rem;
  text-align: center;
  font-size: 0.75rem;
  color: #6b7280;
  cursor: pointer;
  border-top: 1px solid #e5e7eb;
  margin-top: 0.25rem;

  &:hover {
    color: #dc2626;
  }
}

/* 工具栏颜色图标样式 - 与其他图标一致 */
.tiptap-highlight-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  border-radius: 0.25rem;
  font-weight: 600;
  font-size: 0.75rem;
  background-color: rgba(254, 240, 138, 0.6);
  color: #4b5563;
}

.tiptap-textcolor-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  font-weight: 600;
  font-size: 0.85rem;
  color: #4b5563;
  border-bottom: 2px solid #3b82f6;
}

/* 自定义颜色选择器 */
.tiptap-color-custom {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f87171 0%, #fbbf24 25%, #34d399 50%, #60a5fa 75%, #a78bfa 100%);
  cursor: pointer;
}

.tiptap-color-input {
  position: absolute;
  width: 100%;
  height: 100%;
  opacity: 0;
  cursor: pointer;
}

.tiptap-color-custom__icon {
  font-size: 0.875rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
  pointer-events: none;
}

.tiptap-text-decoration {
  --tiptap-decoration-thickness: 0.12em;
  --tiptap-decoration-underline-offset: 0.18em;
  --tiptap-decoration-dot-size: max(1px, calc(var(--tiptap-decoration-thickness) * 0.55));
  --tiptap-decoration-dot-step: 0.36em;
  --tiptap-decoration-wave-height: 0.22em;
  --tiptap-decoration-line-layer-1: linear-gradient(currentColor, currentColor);
  --tiptap-decoration-line-layer-2: linear-gradient(transparent, transparent);
  --tiptap-decoration-line-layer-3: linear-gradient(transparent, transparent);
  --tiptap-decoration-line-size: 100% var(--tiptap-decoration-thickness);
  --tiptap-decoration-line-pos-1: 0 calc(100% + var(--tiptap-decoration-underline-offset));
  --tiptap-decoration-line-pos-2: 0 calc(100% + var(--tiptap-decoration-underline-offset) + 0.24em);
  --tiptap-decoration-line-pos-3: 0 calc(100% + var(--tiptap-decoration-underline-offset) + 0.48em);
  text-decoration: none !important;
  padding-bottom: calc(var(--tiptap-decoration-underline-offset) + var(--tiptap-decoration-wave-height));
  background-repeat: repeat-x;
  background-origin: content-box;
  background-clip: padding-box;
  background-image:
    var(--tiptap-decoration-line-layer-1),
    var(--tiptap-decoration-line-layer-2),
    var(--tiptap-decoration-line-layer-3);
  background-size:
    var(--tiptap-decoration-line-size),
    var(--tiptap-decoration-line-size),
    var(--tiptap-decoration-line-size);
  background-position:
    var(--tiptap-decoration-line-pos-1),
    var(--tiptap-decoration-line-pos-2),
    var(--tiptap-decoration-line-pos-3);
  box-decoration-break: clone;
  -webkit-box-decoration-break: clone;
}

.tiptap-text-decoration--strike {
  --tiptap-decoration-line-pos-1: 0 50%;
  --tiptap-decoration-line-pos-2: 0 calc(50% - 0.22em);
  --tiptap-decoration-line-pos-3: 0 calc(50% + 0.22em);
  padding-bottom: 0;
}

.tiptap-text-decoration--dotted {
  --tiptap-decoration-line-layer-1: radial-gradient(circle, currentColor var(--tiptap-decoration-dot-size), transparent calc(var(--tiptap-decoration-dot-size) + 0.5px));
  --tiptap-decoration-line-size: var(--tiptap-decoration-dot-step) calc(var(--tiptap-decoration-thickness) * 2 + 2px);
}

.tiptap-text-decoration--dense-dotted {
  --tiptap-decoration-dot-step: 0.24em;
  --tiptap-decoration-dot-size: max(1px, calc(var(--tiptap-decoration-thickness) * 0.7));
  --tiptap-decoration-line-layer-1: radial-gradient(circle, currentColor var(--tiptap-decoration-dot-size), transparent calc(var(--tiptap-decoration-dot-size) + 0.45px));
  --tiptap-decoration-line-size: var(--tiptap-decoration-dot-step) calc(var(--tiptap-decoration-thickness) * 2 + 2px);
}

.tiptap-text-decoration--wave-soft {
  --tiptap-decoration-line-layer-1: radial-gradient(ellipse at 50% 100%, transparent 42%, currentColor 45% 54%, transparent 58%);
  --tiptap-decoration-line-size: 0.48em var(--tiptap-decoration-wave-height);
}

.tiptap-text-decoration--wave-heavy {
  --tiptap-decoration-wave-height: 0.3em;
  --tiptap-decoration-line-layer-1: radial-gradient(ellipse at 50% 100%, transparent 36%, currentColor 40% 58%, transparent 62%);
  --tiptap-decoration-line-size: 0.46em var(--tiptap-decoration-wave-height);
}

.tiptap-text-decoration--double {
  --tiptap-decoration-line-layer-2: var(--tiptap-decoration-line-layer-1);
}

.tiptap-text-decoration--triple {
  --tiptap-decoration-line-layer-2: var(--tiptap-decoration-line-layer-1);
  --tiptap-decoration-line-layer-3: var(--tiptap-decoration-line-layer-1);
}

@media (max-width: 767px) {
  .tiptap-toolbar__group--font {
    flex: 0 0 auto;
    min-width: 0;
    transition: none;
  }

  .tiptap-toolbar__group--font.is-expanded {
    flex-basis: auto;
    min-width: 0;
  }

  .tiptap-platform-font-select {
    min-width: 7.5rem;
  }
}

/* 夜间模式颜色选择器 */
:root[data-display-palette='night'] .tiptap-color-picker {
  background-color: #2D2D31;
  border-radius: 0.375rem;
}

:root[data-display-palette='night'] .tiptap-color-swatch {
  border-color: rgba(255, 255, 255, 0.15);
}

:root[data-display-palette='night'] .tiptap-color-picker__clear {
  border-top-color: #52525b;
  color: #a1a1aa;

  &:hover {
    color: #f87171;
  }
}

:root[data-display-palette='night'] .tiptap-highlight-icon {
  background-color: rgba(254, 240, 138, 0.3);
  color: #e5e7eb;
}

:root[data-display-palette='night'] .tiptap-textcolor-icon {
  color: #e5e7eb;
  border-bottom-color: #60a5fa;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker__item {
  color: #e5e7eb;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker__item:hover,
:root[data-display-palette='night'] .tiptap-toolbar-picker__item.is-active {
  background: rgba(96, 165, 250, 0.18);
}

:root[data-display-palette='night'] .tiptap-toolbar-picker__item.is-active {
  color: #93c5fd;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker__custom {
  border-top-color: #3f3f5a;
}

:root[data-display-palette='night'] .mention-dropdown {
  background: var(--sc-bg-surface, #1e1e2e);
  border-color: var(--sc-border-mute, #3f3f5a);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
</style>

<style lang="scss">
.tiptap-platform-font-select__menu {
  border-radius: 14px;
  box-shadow: 0 18px 42px rgba(15, 23, 42, 0.16), 0 6px 16px rgba(15, 23, 42, 0.08);
}

.tiptap-toolbar-popover,
.tiptap-platform-font-select__menu {
  box-sizing: border-box;
  max-width: min(30rem, calc(100vw - 1rem));
}

.tiptap-platform-font-select__menu--sticky-note {
  --n-color: #ffffff !important;
  --n-option-text-color: #0f172a !important;
  --n-option-color-pending: rgba(37, 99, 235, 0.08) !important;
  --n-option-color-active: rgba(37, 99, 235, 0.12) !important;
  --n-option-color-active-pending: rgba(37, 99, 235, 0.16) !important;
  background-color: #ffffff !important;
  color: #0f172a !important;
  box-shadow: 0 18px 42px rgba(15, 23, 42, 0.16), 0 6px 16px rgba(15, 23, 42, 0.08);
}

.tiptap-platform-font-select__menu--sticky-note .n-base-select-option,
.tiptap-platform-font-select__menu--sticky-note .n-base-select-option__content {
  color: #0f172a !important;
}

:root[data-display-palette='night'] .n-popover-shared:has(.tiptap-toolbar-popover--sticky-note),
:root[data-display-palette='night'] .n-popover-shared:has(.tiptap-toolbar-popover--sticky-note) .n-popover-arrow,
:root[data-display-palette='night'] .n-popover-shared:has(.tiptap-toolbar-popover--sticky-note) .n-popover-arrow-wrapper,
:root[data-custom-theme='true'] .n-popover-shared:has(.tiptap-toolbar-popover--sticky-note),
:root[data-custom-theme='true'] .n-popover-shared:has(.tiptap-toolbar-popover--sticky-note) .n-popover-arrow,
:root[data-custom-theme='true'] .n-popover-shared:has(.tiptap-toolbar-popover--sticky-note) .n-popover-arrow-wrapper,
.n-popover-shared:has(.tiptap-toolbar-popover--sticky-note),
.n-popover-shared:has(.tiptap-toolbar-popover--sticky-note) .n-popover-arrow,
.n-popover-shared:has(.tiptap-toolbar-popover--sticky-note) .n-popover-arrow-wrapper,
:root[data-display-palette='night'] .n-popover.tiptap-toolbar-popover--sticky-note,
:root[data-display-palette='night'] .n-popover-shared.tiptap-toolbar-popover--sticky-note,
:root[data-display-palette='night'] .n-popover-shared.tiptap-toolbar-popover--sticky-note .n-popover-arrow,
:root[data-display-palette='night'] .n-popover-shared.tiptap-toolbar-popover--sticky-note .n-popover-arrow-wrapper,
:root[data-custom-theme='true'] .n-popover.tiptap-toolbar-popover--sticky-note,
:root[data-custom-theme='true'] .n-popover-shared.tiptap-toolbar-popover--sticky-note,
:root[data-custom-theme='true'] .n-popover-shared.tiptap-toolbar-popover--sticky-note .n-popover-arrow,
:root[data-custom-theme='true'] .n-popover-shared.tiptap-toolbar-popover--sticky-note .n-popover-arrow-wrapper,
.n-popover.tiptap-toolbar-popover--sticky-note,
.n-popover-shared.tiptap-toolbar-popover--sticky-note,
.n-popover-shared.tiptap-toolbar-popover--sticky-note .n-popover-arrow,
.n-popover-shared.tiptap-toolbar-popover--sticky-note .n-popover-arrow-wrapper {
  --n-color: #ffffff !important;
  --n-text-color: #0f172a !important;
  background-color: #ffffff !important;
  color: #0f172a !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-popover--sticky-note,
:root[data-custom-theme='true'] .tiptap-toolbar-popover--sticky-note,
.tiptap-toolbar-popover--sticky-note {
  --n-color: #ffffff !important;
  --n-text-color: #0f172a !important;
  background-color: #ffffff !important;
  color: #0f172a !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note,
.tiptap-toolbar-picker--sticky-note {
  background: #ffffff !important;
  color: #0f172a !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item,
.tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item {
  color: #0f172a !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item:hover,
:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item.is-active,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item:hover,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item.is-active,
.tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item:hover,
.tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item.is-active {
  background: rgba(37, 99, 235, 0.1) !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item.is-active,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item.is-active,
.tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__item.is-active {
  color: #2563eb !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__custom,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__custom,
.tiptap-toolbar-picker--sticky-note .tiptap-toolbar-picker__custom {
  border-top-color: #e5e7eb !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .n-input,
:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .n-input-wrapper,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .n-input,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .n-input-wrapper,
.tiptap-toolbar-picker--sticky-note .n-input,
.tiptap-toolbar-picker--sticky-note .n-input-wrapper {
  --n-color: #ffffff !important;
  --n-text-color: #0f172a !important;
  --n-placeholder-color: #94a3b8 !important;
  --n-border: 1px solid #d1d5db !important;
  background-color: #ffffff !important;
  color: #0f172a !important;
}

:root[data-display-palette='night'] .tiptap-toolbar-picker--sticky-note .n-button,
:root[data-custom-theme='true'] .tiptap-toolbar-picker--sticky-note .n-button,
.tiptap-toolbar-picker--sticky-note .n-button {
  --n-text-color: #0f172a !important;
  --n-color: #f8fafc !important;
  --n-color-hover: #eef2ff !important;
  --n-border: 1px solid #d1d5db !important;
}

:root[data-display-palette='night'] .tiptap-platform-font-select__menu--sticky-note,
:root[data-custom-theme='true'] .tiptap-platform-font-select__menu--sticky-note {
  --n-color: #ffffff !important;
  --n-option-text-color: #0f172a !important;
  background-color: #ffffff !important;
  color: #0f172a !important;
}

.tiptap-platform-font-option {
  width: 100%;
}

.tiptap-platform-font-option__label {
  display: block;
  width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 767px) {
  .tiptap-toolbar-popover,
  .tiptap-platform-font-select__menu,
  .n-popover-shared:has(.tiptap-toolbar-popover),
  .n-popover-shared:has(.tiptap-platform-font-select__menu) {
    box-sizing: border-box;
    max-width: calc(100vw - 1rem) !important;
    max-height: min(70vh, 32rem);
    overflow: auto;
  }

  .tiptap-toolbar-picker,
  .tiptap-decoration-panel,
  .tiptap-performance-panel,
  .tiptap-color-picker {
    min-width: 0;
    width: min(100%, calc(100vw - 1.5rem));
    max-width: 100%;
    box-sizing: border-box;
  }

  .tiptap-decoration-panel {
    gap: 0.35rem;
  }

  .tiptap-decoration-panel__row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .tiptap-decoration-panel__label {
    grid-column: 1 / -1;
  }

  .tiptap-decoration-panel__chip {
    min-width: 0;
  }

  .tiptap-performance-panel__chips {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .tiptap-performance-chip {
    min-width: 0;
    text-align: center;
  }

  .tiptap-color-picker {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

.tiptap-performance-sheet {
  position: fixed;
  inset: 0;
  z-index: 4200;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  background: rgba(15, 23, 42, 0.32);
  backdrop-filter: blur(2px);
}

.tiptap-performance-sheet__panel {
  width: min(100%, 32rem);
  max-height: min(78vh, 36rem);
  padding: 0 0.5rem max(0.5rem, env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
}

.tiptap-performance-panel--mobile {
  min-width: 0;
  width: 100%;
  max-width: none;
  max-height: min(78vh, 36rem);
  border-radius: 1rem 1rem 0 0;
  background: var(--sc-bg-elevated, #ffffff);
  box-shadow: 0 -10px 32px rgba(15, 23, 42, 0.28);
  padding-bottom: 0;
  overflow: hidden;
}

.tiptap-performance-sheet__body {
  overflow: auto;
  max-height: calc(min(78vh, 36rem) - 3.25rem);
  padding-bottom: calc(0.75rem + env(safe-area-inset-bottom, 0px));
}

@media (max-width: 767px) {
  .tiptap-performance-sheet__panel {
    width: 100%;
    padding: 0;
  }

  .tiptap-performance-panel--mobile {
    border-radius: 1rem 1rem 0 0;
  }
}

.tiptap-content {
  padding: 0.75rem 1rem;
  outline: none;
  min-height: 3rem;
  white-space: break-spaces;
  color: #1f2937; /* 日间模式默认文字颜色 */
  font-size: var(--chat-font-size, 0.9375rem);
  line-height: var(--chat-line-height, 1.6);

  /* 基础文本样式 */
  p {
    margin: 0;
    line-height: inherit;
    min-height: 1.5rem;
  }

  p.is-editor-empty:first-child::before {
    color: #9ca3af;
    content: attr(data-placeholder);
    float: left;
    height: 0;
    pointer-events: none;
  }

  p + p {
    margin-top: 0.5rem;
  }

  .tiptap-mention-chip {
    display: inline-flex;
    align-items: center;
    padding: 0 0.4em;
    margin: 0 0.05em;
    border-radius: 4px;
    background-color: rgba(59, 130, 246, 0.1);
    color: #3b82f6;
    font-weight: 500;
    line-height: 1.45;
    user-select: none;
    cursor: default;
  }

  .tiptap-mention-chip--all {
    background-color: rgba(239, 68, 68, 0.1);
    color: #ef4444;
  }

  /* 标题样式 */
  h1,
  h2,
  h3 {
    margin: 1rem 0 0.75rem;
    font-weight: 600;
    line-height: 1.3;

    &:first-child {
      margin-top: 0;
    }
  }

  h1 {
    font-size: 1.75rem;
  }

  h2 {
    font-size: 1.5rem;
  }

  h3 {
    font-size: 1.25rem;
  }

  /* 列表样式 */
  ul,
  ol {
    padding-left: 1.75rem;
    margin: 0.75rem 0;
  }

  ul {
    list-style-type: disc;
  }

  ol {
    list-style-type: decimal;
  }

  li {
    margin: 0.25rem 0;
    line-height: 1.6;

    p {
      margin: 0;
    }
  }

  /* 引用块样式 */
  blockquote {
    border-left: 4px solid #3b82f6;
    padding-left: 1rem;
    margin: 0.75rem 0;
    color: #6b7280;
    font-style: italic;
  }

  /* 代码样式 */
  code {
    background-color: #f3f4f6;
    border-radius: 0.25rem;
    padding: 0.15rem 0.4rem;
    font-family: 'Courier New', 'Consolas', monospace;
    font-size: 0.9em;
    color: #1f2937;
  }

  pre {
    background-color: #1f2937;
    color: #f9fafb;
    border-radius: 0.5rem;
    padding: 1rem;
    margin: 0.75rem 0;
    overflow-x: auto;
    font-family: 'Courier New', 'Consolas', monospace;
    font-size: 0.9em;
    line-height: 1.5;

    code {
      background: transparent;
      color: inherit;
      padding: 0;
      font-size: inherit;
    }
  }

  /* 文本标记 */
  strong {
    font-weight: 700;
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

  mark {
    background-color: #fef08a;
    padding: 0.1rem 0.2rem;
    border-radius: 0.125rem;
  }

  /* 链接样式 */
  a {
    color: #3b82f6;
    text-decoration: underline;
    cursor: pointer;

    &:hover {
      color: #2563eb;
    }
  }

  /* 分割线 */
  hr {
    border: none;
    border-top: 2px solid #e5e7eb;
    margin: 1.5rem 0;
  }

  /* 图片样式 - 修复显示问题 */
  .rich-inline-image,
  img {
    max-width: 100%;
    max-height: 6rem;
    height: auto;
    border-radius: 0.375rem;
    vertical-align: text-bottom;
    margin: 0 0.25rem;
    display: inline-block;
    object-fit: contain;
  }

  .smart-link-node {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
  }

  .smart-link-node__text {
    color: #2563eb;
    text-decoration: underline;
  }

  .tiptap-ruby {
    position: relative;
    display: inline-block;
    padding-top: 0.38em;
    ruby-align: center;
    ruby-position: over;
    font-family: var(--ruby-base-font-family, var(--ruby-font-family, inherit));
    font-size: var(--ruby-base-font-size, var(--ruby-font-size, inherit));
    color: var(--ruby-color, inherit);
    font-weight: var(--ruby-font-weight, inherit);
    font-style: var(--ruby-font-style, inherit);
    text-decoration: var(--ruby-text-decoration, inherit);
    background-color: var(--ruby-background-color, transparent);
  }

  .tiptap-ruby[data-ruby-spoiler='true'] {
    background-color: var(--spoiler-reveal-bg);
  }

  .tiptap-ruby::before {
    content: attr(data-ruby-text);
    position: absolute;
    left: 50%;
    bottom: calc(100% - 0.16em);
    transform: translateX(-50%);
    font-family: var(--ruby-rt-font-family, var(--ruby-font-family, inherit));
    font-size: var(--ruby-rt-font-size, calc(var(--ruby-base-font-size, var(--ruby-font-size, 1em)) * 0.58));
    font-weight: var(--ruby-font-weight, inherit);
    font-style: var(--ruby-font-style, inherit);
    text-decoration: var(--ruby-text-decoration, inherit);
    background-color: var(--ruby-background-color, transparent);
    line-height: 0.82;
    white-space: nowrap;
    color: var(--ruby-color, inherit);
    pointer-events: none;
  }

  .tiptap-ruby[data-ruby-spoiler='true']::before {
    background-color: var(--spoiler-reveal-bg);
  }

  /* 对齐样式 */
  [style*="text-align: center"] {
    text-align: center;
  }

  [style*="text-align: right"] {
    text-align: right;
  }

  [style*="text-align: justify"] {
    text-align: justify;
  }
}

.smart-link-modal__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.5rem;
}

.smart-link-modal__field {
  display: flex;
  flex-direction: column;
  width: 100%;
  min-width: 0;
}

.smart-link-modal__preview {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  min-width: 0;
  margin-top: 0.75rem;
  padding: 0.5rem 0.625rem;
  border: 1px solid var(--sc-border-secondary, rgba(148, 163, 184, 0.28));
  border-radius: 0.625rem;
  background: color-mix(in srgb, var(--sc-bg-elevated, #f8fafc) 90%, var(--primary-color, #3b82f6) 10%);
}

.smart-link-modal__preview-image {
  width: 3.25rem;
  height: 3.25rem;
  min-width: 3.25rem;
  min-height: 3.25rem;
  max-width: 3.25rem;
  max-height: 3.25rem;
  object-fit: contain;
  border-radius: 0.5rem;
  flex-shrink: 0;
  margin: 0;
}

.smart-link-modal__note {
  display: grid;
  gap: 0.35rem;
  padding: 0.75rem;
  border-radius: 0.75rem;
  background: color-mix(in srgb, var(--primary-color, #3b82f6) 8%, transparent);
  color: var(--sc-text-secondary, #475569);
  font-size: 0.8125rem;
  line-height: 1.45;
}

.ruby-modal__note {
  display: grid;
  gap: 0.35rem;
  padding: 0.75rem;
  border-radius: 0.75rem;
  background: color-mix(in srgb, var(--primary-color, #3b82f6) 8%, transparent);
  color: var(--sc-text-secondary, #475569);
  font-size: 0.8125rem;
  line-height: 1.45;
}

.ruby-modal__advanced {
  display: grid;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.ruby-modal__toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0.625rem 0.75rem;
  border: 1px solid var(--sc-border-secondary, rgba(148, 163, 184, 0.28));
  border-radius: 0.75rem;
  background: color-mix(in srgb, var(--sc-bg-elevated, #f8fafc) 90%, var(--primary-color, #3b82f6) 10%);
  color: inherit;
  cursor: pointer;
}

.ruby-modal__panel {
  display: grid;
  gap: 0.25rem;
}

/* ===== 夜间模式适配 ===== */

/* 编辑器容器夜间模式 */
:root[data-display-palette='night'] .tiptap-editor {
  background-color: var(--sc-bg-input, #3f3f46);
  border-color: var(--sc-border-strong, #52525b);
}

:root[data-display-palette='night'] .tiptap-editor.is-focused {
  border-color: var(--primary-color, #60a5fa);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--primary-color, #60a5fa) 35%, transparent);
}

:root[data-display-palette='night'] .tiptap-editor.whisper-mode {
  background-color: var(--chat-whisper-bg, rgba(76, 29, 149, 0.25));
  border-color: var(--chat-whisper-border, rgba(167, 139, 250, 0.85));
}

:root[data-display-palette='night'] .smart-link-modal__preview {
  border-color: var(--sc-border-strong, rgba(82, 82, 91, 0.9));
  background: color-mix(in srgb, var(--sc-bg-elevated, #27272a) 88%, var(--primary-color, #60a5fa) 12%);
}

:root[data-display-palette='night'] .smart-link-modal__note {
  color: var(--sc-text-secondary, #cbd5e1);
  background: color-mix(in srgb, var(--sc-bg-elevated, #27272a) 86%, var(--primary-color, #60a5fa) 14%);
}

:root[data-display-palette='night'] .ruby-modal__note {
  color: var(--sc-text-secondary, #cbd5e1);
  background: color-mix(in srgb, var(--sc-bg-elevated, #27272a) 86%, var(--primary-color, #60a5fa) 14%);
}

:root[data-display-palette='night'] .ruby-modal__toggle {
  border-color: var(--sc-border-strong, rgba(82, 82, 91, 0.9));
  background: color-mix(in srgb, var(--sc-bg-elevated, #27272a) 88%, var(--primary-color, #60a5fa) 12%);
}

/* 工具栏夜间模式 */
:root[data-display-palette='night'] .tiptap-toolbar {
  background-color: var(--sc-bg-elevated, #27272a);
  border-bottom-color: var(--sc-border-strong, #52525b);
}

:root[data-display-palette='night'] .tiptap-toolbar__divider {
  background-color: var(--sc-border-strong, #3f3f46);
}

:root[data-display-palette='night'] .tiptap-platform-font-select__menu {
  box-shadow: 0 20px 44px rgba(0, 0, 0, 0.48), 0 8px 20px rgba(0, 0, 0, 0.28);
}

/* 浮动菜单夜间模式 */
:root[data-display-palette='night'] .tiptap-bubble-menu {
  background: var(--sc-bg-elevated, #27272a);
  border-color: var(--sc-border-strong, #3f3f46);
  color: var(--sc-text-primary, #f4f4f5);
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
}

:root[data-display-palette='night'] .tiptap-bubble-menu__divider {
  background-color: var(--sc-border-strong, #3f3f46);
}

/* 编辑内容区夜间模式 */
:root[data-display-palette='night'] .tiptap-content {
  color: #f4f4f5;
}

:root[data-display-palette='night'] .tiptap-content p.is-editor-empty:first-child::before {
  color: #a1a1aa;
}

:root[data-display-palette='night'] .tiptap-content blockquote {
  border-left-color: #60a5fa;
  color: #d4d4d8;
}

:root[data-display-palette='night'] .tiptap-content code {
  background-color: #52525b;
  color: #fafafa;
}

:root[data-display-palette='night'] .tiptap-content pre {
  background-color: #18181b;
  color: #f4f4f5;
}

:root[data-display-palette='night'] .tiptap-content hr {
  border-top-color: #52525b;
}

/* 夜间模式滚动条样式 */
:root[data-display-palette='night'] .tiptap-editor-wrapper {
  &::-webkit-scrollbar-thumb {
    background: rgba(161, 161, 170, 0.35);
  }

  &::-webkit-scrollbar-thumb:hover {
    background: rgba(161, 161, 170, 0.55);
  }

  scrollbar-color: rgba(161, 161, 170, 0.35) transparent;
}

:root[data-display-palette='night'] .tiptap-content a {
  color: #93c5fd;

  &:hover {
    color: #bfdbfe;
  }
}

:root[data-display-palette='night'] .tiptap-content mark {
  background-color: #854d0e;
  color: #fef3c7;
}

:root[data-display-palette='night'] .tiptap-content .tiptap-mention-chip {
  background-color: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
}

:root[data-display-palette='night'] .tiptap-content .tiptap-mention-chip--all {
  background-color: rgba(239, 68, 68, 0.2);
  color: #f87171;
}
</style>
