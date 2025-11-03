<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue';
import type { MentionOption } from 'naive-ui';
import { nanoid } from 'nanoid';

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
  (event: 'remove-image', markerId: string): void
  (event: 'paste-image', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-files', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
}>();

const editorRef = ref<HTMLDivElement | null>(null);
const isFocused = ref(false);
const isInternalUpdate = ref(false); // 标记是否是内部输入导致的更新

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

// 渲染内容（解析文本中的图片标记）
const renderContent = (preserveCursor = false) => {
  if (!editorRef.value) return;

  // 保存光标位置
  let savedPosition = 0;
  if (preserveCursor && isFocused.value) {
    savedPosition = getCursorPosition();
  }

  const text = props.modelValue;
  const imageMarkerRegex = /\[\[图片:([^\]]+)\]\]/g;

  let lastIndex = 0;
  const fragments: Array<{ type: 'text' | 'image'; content: string; markerId?: string }> = [];

  let match;
  while ((match = imageMarkerRegex.exec(text)) !== null) {
    // 添加标记前的文本
    if (match.index > lastIndex) {
      fragments.push({
        type: 'text',
        content: text.substring(lastIndex, match.index),
      });
    }

    // 添加图片
    fragments.push({
      type: 'image',
      content: match[0],
      markerId: match[1],
    });

    lastIndex = match.index + match[0].length;
  }

  // 添加剩余文本
  if (lastIndex < text.length) {
    fragments.push({
      type: 'text',
      content: text.substring(lastIndex),
    });
  }

  // 渲染内容（占位符通过 CSS 实现，不需要手动插入）
  let html = '';
  fragments.forEach((fragment) => {
    if (fragment.type === 'text') {
      // 文本节点 - 保留换行
      const lines = fragment.content.split('\n');
      lines.forEach((line, index) => {
        if (index > 0) html += '<br>';
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
    }
  });

  editorRef.value.innerHTML = html || '<span class="empty-line">\u200B</span>';

  // 恢复光标位置
  if (preserveCursor && isFocused.value && savedPosition > 0) {
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
  const selection = window.getSelection();
  if (!selection || !selection.rangeCount || !editorRef.value) return 0;

  const range = selection.getRangeAt(0);
  const preCaretRange = range.cloneRange();
  preCaretRange.selectNodeContents(editorRef.value);
  preCaretRange.setEnd(range.endContainer, range.endOffset);

  return preCaretRange.toString().length;
};

// 设置光标位置
const setCursorPosition = (position: number) => {
  if (!editorRef.value) return;

  const selection = window.getSelection();
  if (!selection) return;

  let currentPos = 0;
  const walker = document.createTreeWalker(
    editorRef.value,
    NodeFilter.SHOW_TEXT,
    null
  );

  let node: Node | null;
  while ((node = walker.nextNode())) {
    const nodeLength = node.textContent?.length || 0;
    if (currentPos + nodeLength >= position) {
      const range = document.createRange();
      range.setStart(node, position - currentPos);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
      return;
    }
    currentPos += nodeLength;
  }
};

// 处理输入事件
const handleInput = () => {
  if (!editorRef.value) return;

  // 获取当前文本内容（保留图片标记）
  let text = '';
  const nodes = editorRef.value.childNodes;

  for (let i = 0; i < nodes.length; i++) {
    const node = nodes[i];
    if (node.nodeType === Node.TEXT_NODE) {
      text += node.textContent || '';
    } else if (node.nodeName === 'BR') {
      text += '\n';
    } else if (node.nodeName === 'SPAN' && (node as HTMLElement).classList.contains('hybrid-input__image')) {
      // 图片节点 - 保留标记
      const markerId = (node as HTMLElement).dataset.markerId;
      if (markerId) {
        text += `[[图片:${markerId}]]`;
      }
    } else {
      text += node.textContent || '';
    }
  }

  // 添加到历史记录
  const cursorPosition = getCursorPosition();
  addToHistory(text, cursorPosition);

  // 标记为内部更新，避免触发重新渲染
  isInternalUpdate.value = true;
  emit('update:modelValue', text);

  // 在下一个 tick 后重置标志
  nextTick(() => {
    isInternalUpdate.value = false;
  });
};

// 处理粘贴事件
const handlePaste = (event: ClipboardEvent) => {
  const items = event.clipboardData?.items;
  if (!items) return;

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
    const position = getCursorPosition();
    emit('paste-image', { files, selectionStart: position, selectionEnd: position });
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

const handleBlur = () => {
  isFocused.value = false;
  emit('blur');
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
  getInstance: () => editorRef.value,
});
</script>

<template>
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
  ></div>
</template>

<style lang="scss" scoped>
.hybrid-input {
  min-height: 2.5rem;
  max-height: 12rem;
  overflow-y: auto;
  padding: 0.5rem 0.75rem;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  background-color: #ffffff;
  font-size: 0.875rem;
  line-height: 1.5;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
  word-wrap: break-word;
  word-break: break-word;
  position: relative;

  // 使用 CSS 实现占位符
  &:empty::before {
    content: attr(data-placeholder);
    color: #9ca3af;
    pointer-events: none;
    position: absolute;
    left: 0.75rem;
    top: 0.5rem;
  }

  &.is-focused {
    border-color: #3b82f6;
    box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.25);
  }

  &.whisper-mode {
    border-color: #7c3aed;
    box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.35);
    background-color: rgba(250, 245, 255, 0.92);
  }

  &.is-disabled {
    background-color: #f3f4f6;
    cursor: not-allowed;
    opacity: 0.6;
  }
}

.hybrid-input__placeholder {
  color: #9ca3af;
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
  background-color: #f3f4f6;
  border: 1px solid #e5e7eb;
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
    color: #6b7280;
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
</style>
