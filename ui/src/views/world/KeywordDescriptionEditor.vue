<script setup lang="ts">
import { ref, watch, onMounted, nextTick, computed } from 'vue';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useMessage } from 'naive-ui';
import {
  createImageTokenRegex,
  isValidAttachmentToken,
  stripValidImageTokens,
} from '@/utils/attachmentMarkdown';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  maxlength?: number
}>(), {
  modelValue: '',
  placeholder: '用于聊天中的提示和解释',
  maxlength: 2000,
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>();

const message = useMessage();
const editorRef = ref<HTMLDivElement | null>(null);
const isUploading = ref(false);
const fileInputRef = ref<HTMLInputElement | null>(null);
const isSyncing = ref(false);

// Text content length for character count
const contentLength = computed(() => {
  return stripValidImageTokens(props.modelValue).length;
});

// Convert markdown with images to HTML for display
const markdownToHtml = (text: string): string => {
  if (!text) return '';
  
  const imageRegex = createImageTokenRegex();
  let result = '';
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  
  while ((match = imageRegex.exec(text)) !== null) {
    // Escape and add text before image
    if (match.index > lastIndex) {
      result += escapeHtml(text.slice(lastIndex, match.index));
    }
    
    // Add image with delete button wrapper (attachments only)
    const [full, alt, src] = match;
    if (!isValidAttachmentToken(src)) {
      result += escapeHtml(full);
    } else {
      const imageUrl = `/api/v1/attachment/${src}/thumb?size=100`;
      const fullUrl = `/api/v1/attachment/${src}`;
      result += `<span class="img-wrap" contenteditable="false" data-id="${src}"><img class="inline-img" src="${imageUrl}" data-id="${src}" data-original="${fullUrl}" alt="${escapeHtml(alt)}" /><button type="button" class="img-del" data-id="${src}">×</button></span>`;
    }
    
    lastIndex = match.index + full.length;
  }
  
  // Add remaining text
  if (lastIndex < text.length) {
    result += escapeHtml(text.slice(lastIndex));
  }
  
  // Convert newlines to <br>
  result = result.replace(/\n/g, '<br>');
  
  return result;
};

// Convert HTML back to markdown
const htmlToMarkdown = (html: string): string => {
  const temp = document.createElement('div');
  temp.innerHTML = html;
  
  let result = '';
  
  const processNode = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      result += node.textContent || '';
    } else if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node as HTMLElement;
      
      if (el.tagName === 'IMG') {
        const dataId = el.getAttribute('data-id') || '';
        const alt = el.getAttribute('alt') || '';
        if (dataId && isValidAttachmentToken(dataId)) {
          result += `![${alt}](id:${dataId})`;
        }
      } else if (el.tagName === 'SPAN' && el.classList.contains('img-wrap')) {
        // Handle image wrapper - extract the img data-id
        const img = el.querySelector('img');
        if (img) {
          const dataId = img.getAttribute('data-id') || '';
          const alt = img.getAttribute('alt') || '';
          if (dataId && isValidAttachmentToken(dataId)) {
            result += `![${alt}](id:${dataId})`;
          }
        }
      } else if (el.tagName === 'BR') {
        result += '\n';
      } else if (el.tagName === 'DIV' || el.tagName === 'P') {
        // Block elements add newlines
        if (result.length > 0 && !result.endsWith('\n')) {
          result += '\n';
        }
        el.childNodes.forEach(processNode);
        if (!result.endsWith('\n')) {
          result += '\n';
        }
      } else {
        el.childNodes.forEach(processNode);
      }
    }
  };
  
  temp.childNodes.forEach(processNode);
  
  // Trim trailing newlines
  return result.replace(/\n+$/, '');
};

const escapeHtml = (text: string): string => {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
};

// Sync content from props to editor
const syncFromProps = () => {
  if (!editorRef.value || isSyncing.value) return;
  
  const html = markdownToHtml(props.modelValue);
  if (editorRef.value.innerHTML !== html) {
    editorRef.value.innerHTML = html || '';
  }
};

// Handle input changes
const handleInput = () => {
  if (!editorRef.value || isSyncing.value) return;
  
  isSyncing.value = true;
  const markdown = htmlToMarkdown(editorRef.value.innerHTML);
  emit('update:modelValue', markdown);
  nextTick(() => {
    isSyncing.value = false;
  });
};

// Watch for external changes
watch(() => props.modelValue, () => {
  if (!isSyncing.value) {
    syncFromProps();
  }
});

onMounted(() => {
  syncFromProps();
});

// Image upload
const handleImageUpload = async (files: File[]) => {
  if (isUploading.value) return;

  isUploading.value = true;

  try {
    for (const file of files) {
      if (!file.type.startsWith('image/')) continue;
      
      const result = await uploadImageAttachment(file);

      if (result.attachmentId) {
        // Extract ID from 'id:xxx' format
        const attachmentId = result.attachmentId.replace(/^id:/, '');
        insertImageAtCursor(attachmentId);
      }
    }
  } catch (error: any) {
    message.error(error.message || '图片上传失败');
  } finally {
    isUploading.value = false;
  }
};

const insertImageAtCursor = (attachmentId: string) => {
  if (!editorRef.value) return;
  
  // Create wrapper span with image and delete button
  const wrapper = document.createElement('span');
  wrapper.className = 'img-wrap';
  wrapper.contentEditable = 'false';
  wrapper.setAttribute('data-id', attachmentId);
  
  const img = document.createElement('img');
  img.className = 'inline-img';
  img.src = `/api/v1/attachment/${attachmentId}/thumb?size=100`;
  img.setAttribute('data-id', attachmentId);
  img.setAttribute('data-original', `/api/v1/attachment/${attachmentId}`);
  img.alt = '';
  
  const delBtn = document.createElement('button');
  delBtn.type = 'button';
  delBtn.className = 'img-del';
  delBtn.setAttribute('data-id', attachmentId);
  delBtn.textContent = '×';
  
  wrapper.appendChild(img);
  wrapper.appendChild(delBtn);
  
  const selection = window.getSelection();
  if (selection && selection.rangeCount > 0) {
    const range = selection.getRangeAt(0);
    // Check if selection is within our editor
    if (editorRef.value.contains(range.commonAncestorContainer)) {
      range.deleteContents();
      range.insertNode(wrapper);
      range.setStartAfter(wrapper);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
    } else {
      editorRef.value.appendChild(wrapper);
    }
  } else {
    editorRef.value.appendChild(wrapper);
  }
  
  handleInput();
  editorRef.value.focus();
};

const insertPlainTextAtCursor = (text: string) => {
  if (!editorRef.value) return;

  const selection = window.getSelection();
  const fragment = document.createDocumentFragment();
  const lines = text.split(/\r?\n/);
  let lastNode: Node | null = null;

  lines.forEach((line, index) => {
    if (line) {
      const node = document.createTextNode(line);
      fragment.appendChild(node);
      lastNode = node;
    }
    if (index < lines.length - 1) {
      const br = document.createElement('br');
      fragment.appendChild(br);
      lastNode = br;
    }
  });

  if (selection && selection.rangeCount > 0) {
    const range = selection.getRangeAt(0);
    if (editorRef.value.contains(range.commonAncestorContainer)) {
      range.deleteContents();
      range.insertNode(fragment);
      if (lastNode) {
        range.setStartAfter(lastNode);
        range.collapse(true);
        selection.removeAllRanges();
        selection.addRange(range);
      }
      return;
    }
  }

  editorRef.value.appendChild(fragment);
};

// Handle clicks on delete buttons
const handleEditorClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement;
  if (target.classList.contains('img-del')) {
    event.preventDefault();
    event.stopPropagation();
    const wrapper = target.closest('.img-wrap');
    if (wrapper) {
      wrapper.remove();
      handleInput();
    }
  }
};

// Handle paste
const handlePaste = (event: ClipboardEvent) => {
  const clipboard = event.clipboardData;
  if (!clipboard) return;

  const items = clipboard.items;
  const files: File[] = [];
  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    if (item.kind === 'file' && item.type.startsWith('image/')) {
      const file = item.getAsFile();
      if (file) files.push(file);
    }
  }

  const text = clipboard.getData('text/plain');
  event.preventDefault();

  if (text) {
    insertPlainTextAtCursor(text);
    handleInput();
  }

  if (files.length > 0) {
    void handleImageUpload(files);
  }
};

// Handle drop
const handleDrop = (event: DragEvent) => {
  const dataTransfer = event.dataTransfer;
  if (!dataTransfer) return;

  const files = Array.from(dataTransfer.files || []);
  const imageFiles = files.filter(file => file.type.startsWith('image/'));
  const text = dataTransfer.getData('text/plain');

  if (files.length > 0) {
    event.preventDefault();
    if (imageFiles.length > 0) {
      void handleImageUpload(imageFiles);
    } else {
      message.warning('仅支持图片文件');
    }
    return;
  }

  if (text) {
    event.preventDefault();
    insertPlainTextAtCursor(text);
    handleInput();
  }
};

const handleDragOver = (event: DragEvent) => {
  event.preventDefault();
};

const triggerFileSelect = () => {
  fileInputRef.value?.click();
};

const handleFileSelect = (event: Event) => {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []).filter(file =>
    file.type.startsWith('image/')
  );
  if (files.length > 0) {
    handleImageUpload(files);
  }
  input.value = '';
};

const focus = () => {
  editorRef.value?.focus();
};

defineExpose({
  focus,
  triggerFileSelect,
});
</script>

<template>
  <div class="desc-editor">
    <!-- Hidden file input -->
    <input
      ref="fileInputRef"
      type="file"
      accept="image/*"
      multiple
      style="display: none"
      @change="handleFileSelect"
    />

    <!-- Editor input only -->
    <div
      ref="editorRef"
      class="desc-editor__input"
      contenteditable="true"
      :data-placeholder="placeholder"
      @input="handleInput"
      @paste="handlePaste"
      @drop="handleDrop"
      @dragover="handleDragOver"
      @click="handleEditorClick"
    ></div>

    <!-- Character count -->
    <div class="desc-editor__footer">
      <span class="desc-editor__count">{{ contentLength }}/{{ maxlength }}</span>
    </div>
  </div>
</template>

<style scoped>
.desc-editor {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  border-radius: 6px;
  background: var(--sc-bg-input, #fff);
  transition: border-color 0.15s ease;
}

.desc-editor:focus-within {
  border-color: var(--primary-color, rgba(59, 130, 246, 0.6));
}

.desc-editor__input {
  min-height: 100px;
  max-height: 280px;
  overflow-y: auto;
  padding: 8px 10px;
  font-size: 14px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
  outline: none;
  color: var(--sc-text-primary, #1e293b);
  background: transparent;
}

/* Minimal scrollbar */
.desc-editor__input::-webkit-scrollbar {
  width: 4px;
}
.desc-editor__input::-webkit-scrollbar-track {
  background: transparent;
}
.desc-editor__input::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.3);
  border-radius: 2px;
}
.desc-editor__input::-webkit-scrollbar-thumb:hover {
  background: rgba(148, 163, 184, 0.5);
}

.desc-editor__input:empty::before {
  content: attr(data-placeholder);
  color: var(--sc-text-secondary, #94a3b8);
  pointer-events: none;
}

.desc-editor__footer {
  display: flex;
  justify-content: flex-end;
  padding: 2px 8px 4px;
  border-top: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.12));
}

.desc-editor__count {
  font-size: 10px;
  color: var(--sc-text-secondary, #94a3b8);
}

/* Image wrapper with delete button */
.desc-editor__input :deep(.img-wrap) {
  position: relative;
  display: inline-block;
  margin: 2px;
  vertical-align: middle;
}

.desc-editor__input :deep(.inline-img) {
  max-width: 90px;
  max-height: 60px;
  object-fit: contain;
  border-radius: 3px;
  cursor: pointer;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
  vertical-align: middle;
}

.desc-editor__input :deep(.img-del) {
  position: absolute;
  top: -4px;
  right: -4px;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: rgba(239, 68, 68, 0.9);
  color: #fff;
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
  display: flex;
  align-items: center;
  justify-content: center;
}

.desc-editor__input :deep(.img-wrap:hover .img-del) {
  opacity: 1;
}

/* Night mode */
:root[data-display-palette='night'] .desc-editor {
  border-color: rgba(255, 255, 255, 0.12);
  background: rgba(30, 30, 34, 0.9);
}

:root[data-display-palette='night'] .desc-editor__input {
  color: rgba(248, 250, 252, 0.9);
}

:root[data-display-palette='night'] .desc-editor__input:empty::before {
  color: rgba(248, 250, 252, 0.4);
}

:root[data-display-palette='night'] .desc-editor__input::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
}

:root[data-display-palette='night'] .desc-editor__input::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.35);
}

:root[data-display-palette='night'] .desc-editor__footer {
  border-color: rgba(255, 255, 255, 0.08);
}

:root[data-display-palette='night'] .desc-editor__count {
  color: rgba(248, 250, 252, 0.5);
}

:root[data-display-palette='night'] .desc-editor__input :deep(.inline-img) {
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

/* Custom theme */
:root[data-custom-theme='true'] .desc-editor {
  border-color: var(--sc-border-mute);
  background: var(--sc-bg-input);
}

:root[data-custom-theme='true'] .desc-editor__input {
  color: var(--sc-text-primary);
}

:root[data-custom-theme='true'] .desc-editor__input:empty::before {
  color: var(--sc-text-secondary);
}

:root[data-custom-theme='true'] .desc-editor__footer {
  border-color: var(--sc-border-mute);
}

:root[data-custom-theme='true'] .desc-editor__count {
  color: var(--sc-text-secondary);
}
</style>
