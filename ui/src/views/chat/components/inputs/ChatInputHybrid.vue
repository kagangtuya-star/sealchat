<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue';
import type { MentionOption } from 'naive-ui';
import { nanoid } from 'nanoid';
import { matchText } from '@/utils/pinyinMatch';

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
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'mention-search', value: string, prefix: string): void
  (event: 'mention-select', option: MentionOption): void
  (event: 'keydown', e: KeyboardEvent): void
  (event: 'focus'): void
  (event: 'blur'): void
  (event: 'composition-start'): void
  (event: 'composition-end'): void
  (event: 'remove-image', markerId: string): void
  (event: 'paste-image', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-files', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
}>();

const editorRef = ref<HTMLDivElement | null>(null);
const wrapperRef = ref<HTMLDivElement | null>(null);
const isFocused = ref(false);
const isInternalUpdate = ref(false); // 标记是否是内部输入导致的更新
const isComposing = ref(false);

// Mention 面板状态
const mentionVisible = ref(false);
const mentionActiveIndex = ref(0);
const mentionTriggerInfo = ref<{ prefix: string; startIndex: number; cursorPosition: number } | null>(null);
const mentionSearchValue = ref('');
const mentionDropdownRef = ref<HTMLDivElement | null>(null);

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

const PLACEHOLDER_PREFIX = '[[图片:';
const PLACEHOLDER_SUFFIX = ']]';
const BLOCK_TAGS = new Set([
  'DIV', 'P', 'PRE', 'BLOCKQUOTE', 'UL', 'OL', 'LI',
  'TABLE', 'THEAD', 'TBODY', 'TFOOT', 'TR', 'TD', 'TH',
  'SECTION', 'ARTICLE', 'ASIDE', 'HEADER', 'FOOTER', 'NAV',
  'H1', 'H2', 'H3', 'H4', 'H5', 'H6'
]);
const IMAGE_TOKEN_REGEX = /\[\[图片:([^\]]+)\]\]/g;

const buildMarkerToken = (markerId: string) => `${PLACEHOLDER_PREFIX}${markerId}${PLACEHOLDER_SUFFIX}`;
const getMarkerLength = (markerId: string) => buildMarkerToken(markerId).length;

const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), max);

const isImageElement = (node: Node): node is HTMLElement =>
  node.nodeType === Node.ELEMENT_NODE && (node as HTMLElement).classList.contains('hybrid-input__image');

const isMentionElement = (node: Node): node is HTMLElement =>
  node.nodeType === Node.ELEMENT_NODE && (node as HTMLElement).classList.contains('hybrid-input__mention');

// 从 mention 元素构建原始 Satori 标签
const buildMentionToken = (element: HTMLElement): string => {
  const atId = element.dataset.atId || '';
  const atName = element.dataset.atName || '';
  if (!atId) return '';
  const nameAttr = atName ? ` name="${atName.replace(/"/g, '&quot;')}"` : '';
  return `<at id="${atId}"${nameAttr}/>`;
};

const getMentionTokenLength = (element: HTMLElement): number => {
  return buildMentionToken(element).length;
};

const getNodeModelLength = (node: Node): number => {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent?.length ?? 0;
  }
  if (node.nodeName === 'BR') {
    return 1;
  }
  if (isImageElement(node)) {
    const markerId = (node as HTMLElement).dataset.markerId || '';
    return markerId ? getMarkerLength(markerId) : 0;
  }
  if (isMentionElement(node)) {
    return getMentionTokenLength(node as HTMLElement);
  }
  let total = 0;
  node.childNodes.forEach((child) => {
    total += getNodeModelLength(child);
  });
  return total;
};

const getOffsetWithinNode = (node: Node, offset: number): number => {
  if (node.nodeType === Node.TEXT_NODE) {
    const length = node.textContent?.length ?? 0;
    return clamp(offset, 0, length);
  }
  if (node.nodeName === 'BR') {
    return offset > 0 ? 1 : 0;
  }
  if (isImageElement(node)) {
    const markerId = (node as HTMLElement).dataset.markerId || '';
    const tokenLength = markerId ? getMarkerLength(markerId) : 0;
    return offset > 0 ? tokenLength : 0;
  }
  if (isMentionElement(node)) {
    const tokenLength = getMentionTokenLength(node as HTMLElement);
    return offset > 0 ? tokenLength : 0;
  }
  const children = Array.from(node.childNodes);
  const safeOffset = clamp(offset, 0, children.length);
  let total = 0;
  for (let i = 0; i < safeOffset; i++) {
    total += getNodeModelLength(children[i]);
  }
  return total;
};

const reduceNode = (node: Node, target: Node, offset: number): { found: boolean; length: number } => {
  if (node === target) {
    return { found: true, length: getOffsetWithinNode(node, offset) };
  }

  if (node.nodeType === Node.TEXT_NODE) {
    return { found: false, length: node.textContent?.length ?? 0 };
  }

  if (node.nodeName === 'BR') {
    return { found: false, length: 1 };
  }

  if (isImageElement(node)) {
    const markerId = (node as HTMLElement).dataset.markerId || '';
    return { found: false, length: markerId ? getMarkerLength(markerId) : 0 };
  }

  if (isMentionElement(node)) {
    return { found: false, length: getMentionTokenLength(node as HTMLElement) };
  }

  let total = 0;
  const children = Array.from(node.childNodes);
  for (let i = 0; i < children.length; i++) {
    const child = children[i];
    const { found, length } = reduceNode(child, target, offset);
    total += length;
    if (found) {
      return { found: true, length: total };
    }
  }

  return { found: false, length: total };
};

const calculateModelIndexForPosition = (container: Node, offset: number): number => {
  if (!editorRef.value) return 0;
  const { length } = reduceNode(editorRef.value, container, offset);
  return length;
};

const resolvePositionByIndex = (node: Node, position: number): { node: Node; offset: number } => {
  if (node.nodeType === Node.TEXT_NODE) {
    const length = node.textContent?.length ?? 0;
    return { node, offset: clamp(position, 0, length) };
  }

  if (node.nodeName === 'BR') {
    const parent = node.parentNode ?? node;
    const index = Array.prototype.indexOf.call(parent.childNodes, node);
    if (position <= 0) {
      return { node: parent, offset: index };
    }
    return { node: parent, offset: index + 1 };
  }

  if (isImageElement(node)) {
    const parent = node.parentNode ?? node;
    const index = Array.prototype.indexOf.call(parent.childNodes, node);
    if (position <= 0) {
      return { node: parent, offset: index };
    }
    return { node: parent, offset: index + 1 };
  }

  if (isMentionElement(node)) {
    const parent = node.parentNode ?? node;
    const index = Array.prototype.indexOf.call(parent.childNodes, node);
    if (position <= 0) {
      return { node: parent, offset: index };
    }
    return { node: parent, offset: index + 1 };
  }

  let remaining = position;
  const children = Array.from(node.childNodes);
  for (let i = 0; i < children.length; i++) {
    const child = children[i];
    const childLength = getNodeModelLength(child);
    if (remaining <= childLength) {
      return resolvePositionByIndex(child, remaining);
    }
    remaining -= childLength;
  }

  return { node, offset: children.length };
};

const getSelectionRange = () => {
  if (!editorRef.value) {
    const length = props.modelValue.length;
    return { start: length, end: length };
  }
  const selection = window.getSelection();
  if (!selection || !selection.rangeCount) {
    const length = props.modelValue.length;
    return { start: length, end: length };
  }
  const range = selection.getRangeAt(0);
  const start = calculateModelIndexForPosition(range.startContainer, range.startOffset);
  const end = calculateModelIndexForPosition(range.endContainer, range.endOffset);
  return { start, end };
};

const setSelectionRange = (start: number, end: number) => {
  if (!editorRef.value) return;
  const selection = window.getSelection();
  if (!selection) return;
  const totalLength = getNodeModelLength(editorRef.value);
  const safeStart = clamp(start, 0, totalLength);
  const safeEnd = clamp(end, 0, totalLength);
  const range = document.createRange();
  const minPos = Math.min(safeStart, safeEnd);
  const maxPos = Math.max(safeStart, safeEnd);
  const startPosition = resolvePositionByIndex(editorRef.value, minPos);
  const endPosition = resolvePositionByIndex(editorRef.value, maxPos);
  range.setStart(startPosition.node, startPosition.offset);
  range.setEnd(endPosition.node, endPosition.offset);
  selection.removeAllRanges();
  selection.addRange(range);
};

const moveCursorToEnd = () => {
  if (!editorRef.value) return;
  const totalLength = getNodeModelLength(editorRef.value);
  setSelectionRange(totalLength, totalLength);
  editorRef.value.focus();
};

// 撤销/重做历史记录
interface HistoryState {
  content: string;
  cursorPosition: number;
}
const history = ref<HistoryState[]>([]);
const historyIndex = ref(-1);
let historyTimer: number | null = null;

const classList = computed(() => {
  const base: string[] = ['hybrid-input'];
  if (props.whisperMode) {
    base.push('whisper-mode');
  }
  if (isFocused.value) {
    base.push('is-focused');
  }
  if (props.disabled) {
    base.push('is-disabled');
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

// 渲染内容（解析文本中的图片标记和 @提及）
const renderContent = (preserveCursor = false) => {
  if (!editorRef.value) return;

  // 保存光标位置
  let savedPosition = 0;
  if (preserveCursor && isFocused.value) {
    savedPosition = getCursorPosition();
  }

  const text = props.modelValue;
  // 匹配图片标记和 Satori <at> 标签
  const combinedRegex = /\[\[图片:([^\]]+)\]\]|<at\s+id="([^"]+)"(?:\s+name="([^"]*)")?\s*\/>/g;

  let lastIndex = 0;
  const fragments: Array<{ type: 'text' | 'image' | 'at'; content: string; markerId?: string; atId?: string; atName?: string }> = [];

  let match;
  while ((match = combinedRegex.exec(text)) !== null) {
    // 添加标记前的文本
    if (match.index > lastIndex) {
      fragments.push({
        type: 'text',
        content: text.substring(lastIndex, match.index),
      });
    }

    if (match[1]) {
      // 图片标记 [[图片:markerId]]
      fragments.push({
        type: 'image',
        content: match[0],
        markerId: match[1],
      });
    } else if (match[2]) {
      // Satori <at> 标签
      fragments.push({
        type: 'at',
        content: match[0],
        atId: match[2],
        atName: match[3] || '',
      });
    }

    lastIndex = match.index + match[0].length;
  }

  // 添加剩余文本
  if (lastIndex < text.length) {
    fragments.push({
      type: 'text',
      content: text.substring(lastIndex),
    });
  }

  // 渲染内容
  let html = '';
  fragments.forEach((fragment, fragmentIndex) => {
    if (fragment.type === 'text') {
      // 文本节点 - 保留换行
      const lines = fragment.content.split('\n');
      const nextFragment = fragments[fragmentIndex + 1];
      lines.forEach((line, index) => {
        if (index > 0) html += '<br>';
        const isLastLine = index === lines.length - 1;
        const skipTrailingEmptyLine = line === '' && isLastLine && nextFragment;
        if (skipTrailingEmptyLine) {
          return;
        }
        html += escapeHtml(line) || '<span class="empty-line">\u200B</span>';
      });
    } else if (fragment.type === 'image' && fragment.markerId) {
      // 图片节点
      const imageInfo = props.inlineImages[fragment.markerId];
      if (imageInfo) {
        const statusClass = `status-${imageInfo.status}`;
        html += `<span class="hybrid-input__image ${statusClass}" data-marker-id="${fragment.markerId}" contenteditable="false">`;

        if (imageInfo.previewUrl) {
          html += `<img src="${imageInfo.previewUrl}" alt="图片" />`;
        } else {
          html += `<span class="image-placeholder">📷</span>`;
        }

        if (imageInfo.status === 'uploading') {
          html += `<span class="image-status">上传中...</span>`;
        } else if (imageInfo.status === 'failed') {
          html += `<span class="image-status error">${imageInfo.error || '上传失败'}</span>`;
        }

        html += `<button class="image-remove" data-marker-id="${fragment.markerId}">×</button>`;
        html += `</span>`;
      }
    } else if (fragment.type === 'at' && fragment.atId) {
      // @提及节点 - 渲染为简单的 @名字 格式（不使用胶囊）
      const displayName = fragment.atName || fragment.atId;
      const sanitizedName = escapeHtml(displayName);
      // data-at-* 用于在提取文本时还原原始标签
      html += `<span class="hybrid-input__mention" data-at-id="${escapeHtml(fragment.atId)}" data-at-name="${sanitizedName}" contenteditable="false">@${sanitizedName}</span>`;
    }
  });

  editorRef.value.innerHTML = html || '<span class="empty-line">\u200B</span>';

  // 恢复光标位置
  if (preserveCursor && isFocused.value) {
    nextTick(() => {
      setCursorPosition(savedPosition);
    });
  }
};

// HTML 转义
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

// 监听内容变化
watch(() => props.modelValue, () => {
  // 如果是内部输入导致的更新，不重新渲染（避免光标丢失）
  if (isInternalUpdate.value) {
    return;
  }
  // 外部更新时保留光标位置（比如图片插入）
  renderContent(true);
});

// 监听图片变化（图片状态更新时保留光标）
watch(() => props.inlineImages, () => {
  renderContent(true);
}, { deep: true });

// 添加历史记录（带去抖动）
const addToHistory = (content: string, cursorPosition: number) => {
  // 清除计时器
  if (historyTimer !== null) {
    clearTimeout(historyTimer);
  }

  // 延迟添加到历史（500ms 内的连续输入只记录一次）
  historyTimer = window.setTimeout(() => {
    // 如果当前不在历史末尾，删除后面的记录
    if (historyIndex.value < history.value.length - 1) {
      history.value = history.value.slice(0, historyIndex.value + 1);
    }

    // 添加新记录
    history.value.push({ content, cursorPosition });
    historyIndex.value = history.value.length - 1;

    // 限制历史记录数量（最多 50 条）
    if (history.value.length > 50) {
      history.value.shift();
      historyIndex.value--;
    }

    historyTimer = null;
  }, 500);
};

// 撤销
const undo = () => {
  if (historyIndex.value > 0) {
    historyIndex.value--;
    const state = history.value[historyIndex.value];

    // 标记为内部更新，避免触发 watch
    isInternalUpdate.value = true;
    emit('update:modelValue', state.content);

    nextTick(() => {
      isInternalUpdate.value = false;
      renderContent(false);
      setCursorPosition(state.cursorPosition);
    });
  }
};

// 重做
const redo = () => {
  if (historyIndex.value < history.value.length - 1) {
    historyIndex.value++;
    const state = history.value[historyIndex.value];

    // 标记为内部更新，避免触发 watch
    isInternalUpdate.value = true;
    emit('update:modelValue', state.content);

    nextTick(() => {
      isInternalUpdate.value = false;
      renderContent(false);
      setCursorPosition(state.cursorPosition);
    });
  }
};

// 获取纯文本内容（不包括图片标记）
const getTextContent = (): string => {
  if (!editorRef.value) return '';
  return editorRef.value.innerText || '';
};

// 获取光标位置（在原始文本中的位置）
const getCursorPosition = (): number => {
  const { end } = getSelectionRange();
  return end;
};

// 设置光标位置
const setCursorPosition = (position: number) => {
  setSelectionRange(position, position);
};

interface MarkerInfo {
  markerId: string;
  start: number;
  end: number;
}

const findMarkerInfoAt = (position: number): MarkerInfo | null => {
  if (!props.modelValue || position < 0) {
    return null;
  }
  const text = props.modelValue;
  IMAGE_TOKEN_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = IMAGE_TOKEN_REGEX.exec(text)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (position >= start && position <= end) {
      return {
        markerId: match[1],
        start,
        end,
      };
    }
  }
  return null;
};

const removeImageMarker = (marker: MarkerInfo) => {
  const nextValue = `${props.modelValue.slice(0, marker.start)}${props.modelValue.slice(marker.end)}`;
  isInternalUpdate.value = true;
  emit('update:modelValue', nextValue);
  addToHistory(nextValue, marker.start);
  emit('remove-image', marker.markerId);
  nextTick(() => {
    isInternalUpdate.value = false;
    renderContent(false);
    setCursorPosition(marker.start);
  });
};

const insertPlainTextAtCursor = (text: string) => {
  if (!editorRef.value) return;
  const normalized = text.replace(/\r\n?/g, '\n');
  if (!normalized) {
    return;
  }
  if (!isFocused.value) {
    editorRef.value.focus();
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();

  const fragment = document.createDocumentFragment();
  const lines = normalized.split('\n');
  lines.forEach((line, index) => {
    if (index > 0) {
      fragment.appendChild(document.createElement('br'));
    }
    if (line.length) {
      fragment.appendChild(document.createTextNode(line));
    }
  });

  const lastNode = fragment.lastChild;
  range.insertNode(fragment);

  if (lastNode) {
    const cursorRange = document.createRange();
    if (lastNode.nodeType === Node.TEXT_NODE) {
      const textNode = lastNode as Text;
      cursorRange.setStart(textNode, textNode.textContent?.length ?? 0);
    } else {
      cursorRange.setStartAfter(lastNode);
    }
    cursorRange.collapse(true);
    selection.removeAllRanges();
    selection.addRange(cursorRange);
  }
};

// 处理输入事件
const handleInput = () => {
  if (!editorRef.value) return;

  const text = extractContentWithLineBreaks();

  // 添加到历史记录
  const cursorPosition = getCursorPosition();
  addToHistory(text, cursorPosition);

  // 标记为内部更新，避免触发重新渲染
  isInternalUpdate.value = true;
  emit('update:modelValue', text);

  // 检测 @ 提及触发
  checkMentionTrigger(text, cursorPosition);

  // 在下一个 tick 后重置标志
  nextTick(() => {
    isInternalUpdate.value = false;
  });
};

// @ 提及检测
const checkMentionTrigger = (text: string, cursorPosition: number) => {
  // 从光标位置向前查找最近的触发字符
  const textBeforeCursor = text.substring(0, cursorPosition);

  for (const prefix of props.mentionPrefix) {
    const prefixStr = String(prefix);
    const lastPrefixIndex = textBeforeCursor.lastIndexOf(prefixStr);

    if (lastPrefixIndex === -1) continue;

    // 检查触发字符前是否为空格、换行或字符串开头
    const charBefore = lastPrefixIndex > 0 ? textBeforeCursor[lastPrefixIndex - 1] : '';
    const isValidStart = lastPrefixIndex === 0 || /[\s\n]/.test(charBefore);

    if (!isValidStart) continue;

    // 获取触发字符后到光标之间的搜索模式
    const pattern = textBeforeCursor.substring(lastPrefixIndex + prefixStr.length);

    // 确保模式中没有空格（表示 @ 已结束）
    if (/\s/.test(pattern)) continue;

    // 显示 mention 面板并触发搜索
    mentionVisible.value = true;
    mentionActiveIndex.value = 0;
    mentionSearchValue.value = pattern;
    mentionTriggerInfo.value = { prefix: prefixStr, startIndex: lastPrefixIndex, cursorPosition };
    emit('mention-search', pattern, prefixStr);
    return;
  }

  // 没有找到有效的触发，隐藏面板
  closeMentionPanel();
};

// 关闭 mention 面板
const closeMentionPanel = () => {
  mentionVisible.value = false;
  mentionTriggerInfo.value = null;
  mentionActiveIndex.value = 0;
  mentionSearchValue.value = '';
};

// 处理 mention 选择
const handleMentionSelect = (option: MentionOption) => {
  if (!mentionTriggerInfo.value) return;

  const { startIndex } = mentionTriggerInfo.value;
  const cursorPosition = mentionTriggerInfo.value.cursorPosition;
  const text = props.modelValue;

  // 替换 @ 触发词和搜索模式为选中的值
  const before = text.substring(0, startIndex);
  const after = text.substring(cursorPosition);
  const newValue = before + option.value + ' ' + after;

  isInternalUpdate.value = true;
  emit('update:modelValue', newValue);
  emit('mention-select', option);

  closeMentionPanel();

  // 设置光标位置到插入内容之后
  nextTick(() => {
    isInternalUpdate.value = false;
    renderContent(false);
    const newCursorPos = startIndex + String(option.value).length + 1;
    setCursorPosition(newCursorPos);
    editorRef.value?.focus();
  });
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

const handleMentionSearchKeydown = (event: KeyboardEvent) => {
  if (handleMentionKeydown(event)) {
    return;
  }
};

// Mention 面板键盘导航
const handleMentionKeydown = (event: KeyboardEvent): boolean => {
  if (!mentionVisible.value) {
    return false;
  }

  const optionsCount = mentionFilteredOptions.value.length;
  if (!optionsCount) {
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

watch([mentionVisible, mentionFilteredOptions], () => {
  if (!mentionVisible.value) {
    return;
  }
  const optionCount = mentionFilteredOptions.value.length;
  if (!optionCount) {
    mentionActiveIndex.value = 0;
    return;
  }
  if (mentionActiveIndex.value >= optionCount) {
    mentionActiveIndex.value = 0;
  }
  scrollActiveMentionIntoView();
});

const extractContentWithLineBreaks = () => {
  const root = editorRef.value;
  if (!root) return '';

  const pieces: string[] = [];
  const childNodes = Array.from(root.childNodes);
  childNodes.forEach((child, index) => {
    collectNodeText(child, pieces, index === childNodes.length - 1);
  });

  let result = pieces.join('');
  result = result.replace(/\u200B/g, '');
  return result;
};

const collectNodeText = (node: Node, sink: string[], isLastSibling: boolean) => {
  if (node.nodeType === Node.TEXT_NODE) {
    const text = node.textContent?.replace(/\r\n/g, '\n') ?? '';
    if (text) {
      sink.push(text);
    }
    return;
  }

  if (node.nodeName === 'BR') {
    sink.push('\n');
    return;
  }

  if (isImageElement(node)) {
    const markerId = (node as HTMLElement).dataset.markerId;
    if (markerId) {
      sink.push(buildMarkerToken(markerId));
    }
    return;
  }

  if (isMentionElement(node)) {
    const token = buildMentionToken(node as HTMLElement);
    if (token) {
      sink.push(token);
    }
    return;
  }

  if (node.nodeType !== Node.ELEMENT_NODE) {
    return;
  }

  const element = node as HTMLElement;
  const isBlock = BLOCK_TAGS.has(element.tagName);
  const children = Array.from(element.childNodes);

  if (isBlock && sink.length && !endsWithLineBreak(sink)) {
    sink.push('\n');
  }

  if (!children.length) {
    if (isBlock && !isLastSibling && !endsWithLineBreak(sink)) {
      sink.push('\n');
    }
    return;
  }

  children.forEach((child, index) => {
    collectNodeText(child, sink, index === children.length - 1);
  });

  if (isBlock && !isLastSibling && !endsWithLineBreak(sink)) {
    sink.push('\n');
  }
};

const endsWithLineBreak = (chunks: string[]) => {
  if (!chunks.length) {
    return false;
  }
  return /\n$/.test(chunks[chunks.length - 1]);
};

// 处理粘贴事件
const handlePaste = (event: ClipboardEvent) => {
  const clipboard = event.clipboardData;
  if (!clipboard) return;

  const files: File[] = [];
  const items = clipboard.items;
  if (items) {
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile();
        if (file) {
          files.push(file);
        }
      }
    }
  }

  if (files.length > 0) {
    event.preventDefault();
    const position = getCursorPosition();
    emit('paste-image', { files, selectionStart: position, selectionEnd: position });
    return;
  }

  const plainText = clipboard.getData('text/plain') || clipboard.getData('text') || '';
  if (plainText) {
    event.preventDefault();
    insertPlainTextAtCursor(plainText);
    handleInput();
  }
};

// 处理拖拽事件
const handleDrop = (event: DragEvent) => {
  event.preventDefault();
  event.stopPropagation();

  const files = Array.from(event.dataTransfer?.files || []).filter((file) =>
    file.type.startsWith('image/')
  );

  if (files.length > 0) {
    const position = getCursorPosition();
    emit('drop-files', { files, selectionStart: position, selectionEnd: position });
  }
};

const handleDragOver = (event: DragEvent) => {
  event.preventDefault();
  event.stopPropagation();
};

// 处理按键事件
const handleKeydown = (event: KeyboardEvent) => {
  // 优先处理 mention 面板键盘导航
  if (handleMentionKeydown(event)) {
    return;
  }

  // 处理撤销/重做快捷键
  if ((event.ctrlKey || event.metaKey) && !event.shiftKey && event.key === 'z') {
    event.preventDefault();
    undo();
    return;
  }

  if ((event.ctrlKey || event.metaKey) && (event.key === 'y' || (event.shiftKey && event.key === 'z'))) {
    event.preventDefault();
    redo();
    return;
  }

  const composing = event.isComposing || isComposing.value;
  if (!composing && (event.key === 'Backspace' || event.key === 'Delete')) {
    const selection = getSelectionRange();
    if (selection.start === selection.end) {
      const position = event.key === 'Backspace' ? selection.start - 1 : selection.start;
      const marker = findMarkerInfoAt(position);
      if (marker) {
        event.preventDefault();
        removeImageMarker(marker);
        return;
      }
    }
  }

  emit('keydown', event);
};

// 处理图片删除点击
const handleClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement;
  if (target.classList.contains('image-remove')) {
    const markerId = target.dataset.markerId;
    if (markerId) {
      event.preventDefault();
      emit('remove-image', markerId);
    }
  }
};

// 焦点事件
const handleFocus = () => {
  isFocused.value = true;
  emit('focus');
};

const handleBlur = (event: FocusEvent) => {
  isFocused.value = false;
  // 延迟关闭面板，以允许点击选项
  const relatedTarget = event.relatedTarget as HTMLElement;
  if (relatedTarget?.closest('.mention-dropdown')) {
    return;
  }
  setTimeout(() => {
    closeMentionPanel();
  }, 150);
  emit('blur');
};

const handleCompositionStart = () => {
  isComposing.value = true;
  emit('composition-start');
};

const handleCompositionEnd = () => {
  isComposing.value = false;
  emit('composition-end');
};

// 暴露方法
const focus = () => {
  nextTick(() => {
    editorRef.value?.focus();
  });
};

const blur = () => {
  editorRef.value?.blur();
};

const getTextarea = (): HTMLTextAreaElement | undefined => {
  return undefined;
};

onMounted(() => {
  renderContent();
  // 初始化历史记录
  if (props.modelValue) {
    history.value.push({ content: props.modelValue, cursorPosition: 0 });
    historyIndex.value = 0;
  }
});

onBeforeUnmount(() => {
  // 清理计时器
  if (historyTimer !== null) {
    clearTimeout(historyTimer);
    historyTimer = null;
  }
});

defineExpose({
  focus,
  blur,
  getTextarea,
  getSelectionRange,
  setSelectionRange,
  moveCursorToEnd,
  getInstance: () => editorRef.value,
});
</script>

<template>
  <div ref="wrapperRef" class="hybrid-input-wrapper">
    <div
      ref="editorRef"
      :class="classList"
      :data-placeholder="placeholder"
      contenteditable
      :disabled="disabled"
      @input="handleInput"
      @paste="handlePaste"
      @drop="handleDrop"
      @dragover="handleDragOver"
      @keydown="handleKeydown"
      @click="handleClick"
      @focus="handleFocus"
      @blur="handleBlur"
      @compositionstart="handleCompositionStart"
      @compositionend="handleCompositionEnd"
    ></div>
    <!-- Mention 下拉面板 -->
    <Transition name="mention-fade">
      <div
        v-if="mentionVisible"
        class="mention-dropdown"
        tabindex="-1"
        ref="mentionDropdownRef"
        @pointerdown.stop
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
  </div>
</template>

<style lang="scss" scoped>
.hybrid-input-wrapper {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.hybrid-input {
  min-height: 2.5rem;
  max-height: 12rem;
  overflow-y: auto;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  border-radius: 0.5rem;
  background-color: var(--sc-bg-input, #ffffff);
  font-size: var(--chat-font-size, 0.9375rem);
  line-height: var(--chat-line-height, 1.6);
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;
  box-sizing: border-box;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
  position: relative;
  color: var(--sc-text-primary, #0f172a);
  scrollbar-gutter: stable;
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

  // 使用 CSS 实现占位符
  &:empty::before {
    content: attr(data-placeholder);
    color: var(--sc-text-secondary, #9ca3af);
    pointer-events: none;
    position: absolute;
    left: 0.75rem;
    top: 0.5rem;
  }

  &.is-focused {
    border-color: rgba(59, 130, 246, 0.7);
    box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.25);
  }

  &.whisper-mode {
    border-color: rgba(124, 58, 237, 0.8);
    box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.35);
    background-color: rgba(124, 58, 237, 0.08);
  }

  &.is-disabled {
    background-color: var(--sc-bg-surface, #f3f4f6);
    cursor: not-allowed;
    opacity: 0.6;
  }
}

.hybrid-input.chat-input--expanded {
  min-height: calc(100vh / 3);
  max-height: calc(100vh / 3);
}

.hybrid-input.chat-input--fullscreen {
  flex: 1 1 auto;
  min-height: 100%;
  max-height: 100%;
  height: 100%;
  overflow-y: auto;
  touch-action: pan-y;
  min-height: 0;
}

.hybrid-input.chat-input--custom-height {
  min-height: var(--custom-input-height, 2.5rem);
  max-height: var(--custom-input-height, 12rem);
}

.hybrid-input__placeholder {
  color: var(--sc-text-secondary, #9ca3af);
  pointer-events: none;
  position: absolute;
}

.empty-line {
  display: inline;
}

:deep(.hybrid-input__image) {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  position: relative;
  margin: 0 0.125rem;
  padding: 0.125rem 0.375rem;
  background-color: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  border-radius: 0.375rem;
  font-size: 0.75rem;
  vertical-align: middle;
  user-select: none;

  img {
    max-height: 4rem;
    max-width: 8rem;
    border-radius: 0.25rem;
    object-fit: contain;
  }

  .image-placeholder {
    font-size: 2rem;
  }

  .image-status {
    color: var(--sc-text-secondary, #6b7280);
    font-size: 0.75rem;

    &.error {
      color: #ef4444;
    }
  }

  .image-remove {
    position: absolute;
    top: -0.25rem;
    right: -0.25rem;
    width: 1.25rem;
    height: 1.25rem;
    display: flex;
    align-items: center;
    justify-content: center;
    background-color: rgba(239, 68, 68, 0.9);
    border: none;
    border-radius: 50%;
    color: #ffffff;
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.2s ease;

    &:hover {
      background-color: rgba(220, 38, 38, 1);
    }
  }

  &:hover .image-remove {
    opacity: 1;
  }

  &.status-uploading {
    border-color: #3b82f6;
    background-color: rgba(59, 130, 246, 0.05);
  }

  &.status-failed {
    border-color: #ef4444;
    background-color: rgba(239, 68, 68, 0.05);
  }
}

/* @提及样式 - 输入框中显示为简单的蓝色文本 */
:deep(.hybrid-input__mention) {
  color: #3b82f6;
  user-select: none;
  cursor: default;
}

/* 夜间模式滚动条样式 */
:root[data-display-palette='night'] .hybrid-input {
  &::-webkit-scrollbar-thumb {
    background: rgba(161, 161, 170, 0.35);
  }

  &::-webkit-scrollbar-thumb:hover {
    background: rgba(161, 161, 170, 0.55);
  }

  scrollbar-color: rgba(161, 161, 170, 0.35) transparent;
}

/* Mention 下拉面板样式 */
.mention-dropdown {
  position: absolute;
  bottom: 100%;
  left: 0;
  right: 0;
  max-height: 200px;
  overflow-y: auto;
  margin-bottom: 4px;
  background: var(--sc-bg-surface, #ffffff);
  border: 1px solid var(--sc-border-mute, #e5e7eb);
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  z-index: 1000;

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

/* Mention 面板过渡动画 */
.mention-fade-enter-active,
.mention-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.mention-fade-enter-from,
.mention-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

/* 夜间模式下拉面板 */
:root[data-display-palette='night'] .mention-dropdown {
  background: var(--sc-bg-surface, #1e1e2e);
  border-color: var(--sc-border-mute, #3f3f5a);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
</style>
