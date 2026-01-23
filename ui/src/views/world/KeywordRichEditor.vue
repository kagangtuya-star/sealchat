<script setup lang="ts">
import { ref, watch, onBeforeUnmount, nextTick, shallowRef, computed } from 'vue';
import type { Editor } from '@tiptap/vue-3';
import { Spoiler } from '@/utils/tiptap-spoiler';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useMessage } from 'naive-ui';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  maxlength?: number
}>(), {
  modelValue: '',
  placeholder: '用于聊天中的提示和解释（支持富文本格式）',
  maxlength: 2000,
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>();

const message = useMessage();
const editor = shallowRef<Editor | null>(null);
const isInitializing = ref(true);
const isSyncingFromProps = ref(false);
const fileInputRef = ref<HTMLInputElement | null>(null);
const isUploading = ref(false);

// Color picker states
const highlightColorPopoverShow = ref(false);
const textColorPopoverShow = ref(false);

// Link modal state
const linkModalShow = ref(false);
const linkText = ref('');
const linkUrl = ref('');

// Color palettes
const highlightColors = [
  '#fef08a', '#bbf7d0', '#bfdbfe', '#fecaca',
  '#e9d5ff', '#fed7aa', '#99f6e4',
];
const textColors = [
  '#dc2626', '#ea580c', '#ca8a04', '#16a34a',
  '#0284c7', '#7c3aed', '#db2777',
];
const customHighlightColor = ref('#fce7f3');
const customTextColor = ref('#1f2937');

let EditorContent: any = null;

const EMPTY_DOC = {
  type: 'doc',
  content: [{ type: 'paragraph' }],
};

const cloneEmptyDoc = () => JSON.parse(JSON.stringify(EMPTY_DOC));

// Character count
const contentLength = computed(() => {
  if (!editor.value) return 0;
  return editor.value.state.doc.textContent.length;
});

const initEditor = async () => {
  try {
    isInitializing.value = true;

    const [
      { Editor: EditorClass },
      { EditorContent: EditorContentComp },
      { default: StarterKit },
      { default: Link },
      { default: TextStyle },
      { default: Color },
      { default: Image },
      { default: Underline },
      { default: Highlight },
      { default: TextAlign },
    ] = await Promise.all([
      import('@tiptap/core'),
      import('@tiptap/vue-3'),
      import('@tiptap/starter-kit'),
      import('@tiptap/extension-link'),
      import('@tiptap/extension-text-style').then(m => ({ default: m.TextStyle })),
      import('@tiptap/extension-color').then(m => ({ default: m.Color })),
      import('@tiptap/extension-image'),
      import('@tiptap/extension-underline'),
      import('@tiptap/extension-highlight'),
      import('@tiptap/extension-text-align'),
    ]);

    EditorContent = EditorContentComp;

    editor.value = new EditorClass({
      content: cloneEmptyDoc(),
      extensions: [
        StarterKit.configure({
          heading: { levels: [1, 2, 3] },
          codeBlock: {
            HTMLAttributes: { class: 'code-block' },
          },
        }),
        TextStyle,
        Color,
        Underline,
        Highlight.configure({ multicolor: true }),
        Spoiler,
        TextAlign.configure({ types: ['heading', 'paragraph'] }),
        Link.configure({
          openOnClick: false,
          HTMLAttributes: {
            class: 'text-blue-500 underline cursor-pointer',
            target: '_blank',
            rel: 'noopener noreferrer',
          },
        }),
        Image.configure({
          inline: true,
          allowBase64: true,
          HTMLAttributes: { class: 'rich-inline-image' },
        }),
      ],
      editorProps: {
        attributes: { class: 'keyword-rich-content' },
        handlePaste: (view, event) => {
          const items = event.clipboardData?.items;
          if (!items) return false;

          const files: File[] = [];
          for (let i = 0; i < items.length; i++) {
            const item = items[i];
            if (item.kind === 'file' && item.type.startsWith('image/')) {
              const file = item.getAsFile();
              if (file) files.push(file);
            }
          }

          if (files.length > 0) {
            event.preventDefault();
            handleImageUpload(files);
            return true;
          }
          return false;
        },
        handleDrop: (view, event, slice, moved) => {
          if (moved) return false;
          const files = Array.from(event.dataTransfer?.files || []).filter(
            file => file.type.startsWith('image/')
          );
          if (files.length > 0) {
            event.preventDefault();
            handleImageUpload(files);
            return true;
          }
          return false;
        },
      },
      onUpdate: ({ editor: ed }) => {
        const json = ed.getJSON();
        const jsonString = JSON.stringify(json);
        isSyncingFromProps.value = true;
        emit('update:modelValue', jsonString);
        nextTick(() => {
          isSyncingFromProps.value = false;
        });
      },
      onCreate: ({ editor: ed }) => {
        if (!props.modelValue) {
          ed.commands.setContent(cloneEmptyDoc(), false);
          return;
        }
        try {
          const json = JSON.parse(props.modelValue);
          ed.commands.setContent(json, false);
        } catch {
          ed.commands.setContent(props.modelValue, false);
        }
      },
    });

    isInitializing.value = false;
  } catch (error) {
    console.error('Failed to initialize rich text editor:', error);
    isInitializing.value = false;
  }
};

initEditor();

watch(() => props.modelValue, (newValue) => {
  if (!editor.value || editor.value.isDestroyed) return;
  if (isSyncingFromProps.value) return;

  if (!newValue || newValue.trim() === '') {
    editor.value.commands.setContent(cloneEmptyDoc(), false);
    editor.value.commands.setTextSelection(0);
    return;
  }

  try {
    const currentJson = JSON.stringify(editor.value.getJSON());
    if (currentJson !== newValue) {
      const json = JSON.parse(newValue);
      editor.value.commands.setContent(json, false);
    }
  } catch {
    // Non-JSON format, skip
  }
});

// Image upload
const handleImageUpload = async (files: File[]) => {
  if (isUploading.value || !editor.value) return;
  isUploading.value = true;

  try {
    for (const file of files) {
      if (!file.type.startsWith('image/')) continue;
      const result = await uploadImageAttachment(file);
      if (result.attachmentId) {
        const attachmentId = result.attachmentId.replace(/^id:/, '');
        const imageUrl = `/api/v1/attachment/${attachmentId}`;
        editor.value.chain().focus().setImage({ src: imageUrl, alt: '' }).run();
      }
    }
  } catch (error: any) {
    message.error(error.message || '图片上传失败');
  } finally {
    isUploading.value = false;
  }
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

// Toolbar actions
const toggleBold = () => editor.value?.chain().focus().toggleBold().run();
const toggleItalic = () => editor.value?.chain().focus().toggleItalic().run();
const toggleUnderline = () => editor.value?.chain().focus().toggleUnderline().run();
const toggleStrike = () => editor.value?.chain().focus().toggleStrike().run();
const toggleSpoiler = () => editor.value?.chain().focus().toggleSpoiler().run();
const toggleCode = () => editor.value?.chain().focus().toggleCode().run();
const toggleCodeBlock = () => editor.value?.chain().focus().toggleCodeBlock().run();
const toggleBulletList = () => editor.value?.chain().focus().toggleBulletList().run();
const toggleOrderedList = () => editor.value?.chain().focus().toggleOrderedList().run();
const toggleBlockquote = () => editor.value?.chain().focus().toggleBlockquote().run();
const setHeading = (level: 1 | 2 | 3) => editor.value?.chain().focus().toggleHeading({ level }).run();
const setParagraph = () => editor.value?.chain().focus().setParagraph().run();
const setTextAlign = (align: 'left' | 'center' | 'right') => editor.value?.chain().focus().setTextAlign(align).run();
const insertHorizontalRule = () => editor.value?.chain().focus().setHorizontalRule().run();
const clearFormatting = () => editor.value?.chain().focus().clearNodes().unsetAllMarks().run();

// Highlight color
const setHighlightColor = (color: string) => {
  editor.value?.chain().focus().setHighlight({ color }).run();
  highlightColorPopoverShow.value = false;
};
const removeHighlight = () => {
  editor.value?.chain().focus().unsetHighlight().run();
  highlightColorPopoverShow.value = false;
};
const getActiveHighlightColor = () => {
  if (!editor.value) return null;
  const attrs = editor.value.getAttributes('highlight');
  return attrs?.color || null;
};
const applyCustomHighlightColor = () => {
  setHighlightColor(customHighlightColor.value);
};

// Text color
const setTextColor = (color: string) => {
  editor.value?.chain().focus().setColor(color).run();
  textColorPopoverShow.value = false;
};
const removeTextColor = () => {
  editor.value?.chain().focus().unsetColor().run();
  textColorPopoverShow.value = false;
};
const getActiveTextColor = () => {
  if (!editor.value) return null;
  const attrs = editor.value.getAttributes('textStyle');
  return attrs?.color || null;
};
const applyCustomTextColor = () => {
  setTextColor(customTextColor.value);
};

// Link
const setLink = () => {
  const { from, to } = editor.value?.state.selection || { from: 0, to: 0 };
  const hasSelection = from !== to;
  if (hasSelection) {
    const selectedText = editor.value?.state.doc.textBetween(from, to, ' ') || '';
    linkText.value = selectedText;
  } else {
    linkText.value = '';
  }
  linkUrl.value = '';
  linkModalShow.value = true;
};

const confirmLink = () => {
  if (!linkUrl.value.trim()) {
    linkModalShow.value = false;
    return;
  }
  const url = linkUrl.value.trim();
  const { from, to } = editor.value?.state.selection || { from: 0, to: 0 };
  const hasSelection = from !== to;

  if (hasSelection) {
    editor.value?.chain().focus().setLink({ href: url, target: '_blank' }).run();
  } else {
    const text = linkText.value.trim() || url;
    editor.value?.chain().focus().insertContent({
      type: 'text',
      text: text,
      marks: [{ type: 'link', attrs: { href: url, target: '_blank' } }],
    }).run();
  }

  linkModalShow.value = false;
  linkText.value = '';
  linkUrl.value = '';
};

const unsetLink = () => {
  editor.value?.chain().focus().unsetLink().run();
};

const isActive = (name: string, attrs?: Record<string, any>) => editor.value?.isActive(name, attrs) ?? false;

const focus = () => {
  nextTick(() => {
    editor.value?.commands.focus();
  });
};

onBeforeUnmount(() => {
  editor.value?.destroy();
});

defineExpose({
  focus,
  getEditor: () => editor.value,
  getJson: () => editor.value?.getJSON(),
  triggerFileSelect,
});
</script>

<template>
  <div class="keyword-rich-editor">
    <!-- Hidden file input -->
    <input
      ref="fileInputRef"
      type="file"
      accept="image/*"
      multiple
      style="display: none"
      @change="handleFileSelect"
    />

    <div v-if="isInitializing" class="keyword-rich-loading">
      <n-spin size="small" />
      <span class="ml-2 text-sm text-gray-500">加载编辑器...</span>
    </div>

    <div v-else class="keyword-rich-wrapper">
      <!-- Toolbar -->
      <div class="keyword-rich-toolbar">
        <!-- Headings -->
        <div class="keyword-rich-toolbar__group">
          <n-button
            size="tiny"
            text
            :type="isActive('paragraph') ? 'primary' : 'default'"
            @click="setParagraph"
            title="正文"
          >
            P
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('heading', { level: 1 }) ? 'primary' : 'default'"
            @click="setHeading(1)"
            title="标题 1"
          >
            H1
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('heading', { level: 2 }) ? 'primary' : 'default'"
            @click="setHeading(2)"
            title="标题 2"
          >
            H2
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('heading', { level: 3 }) ? 'primary' : 'default'"
            @click="setHeading(3)"
            title="标题 3"
          >
            H3
          </n-button>
        </div>

        <div class="keyword-rich-toolbar__divider"></div>

        <!-- Text formatting -->
        <div class="keyword-rich-toolbar__group">
          <n-button
            size="tiny"
            text
            :type="isActive('bold') ? 'primary' : 'default'"
            @click="toggleBold"
            title="粗体 (Ctrl+B)"
          >
            <span class="font-bold">B</span>
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('italic') ? 'primary' : 'default'"
            @click="toggleItalic"
            title="斜体 (Ctrl+I)"
          >
            <span class="italic">I</span>
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('underline') ? 'primary' : 'default'"
            @click="toggleUnderline"
            title="下划线 (Ctrl+U)"
          >
            <span class="underline">U</span>
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('strike') ? 'primary' : 'default'"
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
          <n-button
            size="tiny"
            text
            :type="isActive('code') ? 'primary' : 'default'"
            @click="toggleCode"
            title="行内代码"
          >
            <span class="font-mono text-xs">&lt;/&gt;</span>
          </n-button>

          <!-- Highlight color picker -->
          <n-popover
            trigger="click"
            placement="bottom"
            v-model:show="highlightColorPopoverShow"
          >
            <template #trigger>
              <n-button
                size="tiny"
                text
                :type="isActive('highlight') ? 'primary' : 'default'"
                title="高亮颜色"
              >
                <span class="keyword-rich-highlight-icon">H</span>
              </n-button>
            </template>
            <div class="keyword-rich-color-picker">
              <div
                v-for="color in highlightColors"
                :key="color"
                class="keyword-rich-color-swatch"
                :class="{ 'is-active': getActiveHighlightColor() === color }"
                :style="{ backgroundColor: color }"
                @click="setHighlightColor(color)"
                :title="color"
              ></div>
              <label class="keyword-rich-color-swatch keyword-rich-color-custom" title="自定义颜色">
                <input
                  type="color"
                  v-model="customHighlightColor"
                  @change="applyCustomHighlightColor"
                  class="keyword-rich-color-input"
                />
                <span class="keyword-rich-color-custom__icon">+</span>
              </label>
              <div class="keyword-rich-color-picker__clear" @click="removeHighlight">
                清除高亮
              </div>
            </div>
          </n-popover>

          <!-- Text color picker -->
          <n-popover
            trigger="click"
            placement="bottom"
            v-model:show="textColorPopoverShow"
          >
            <template #trigger>
              <n-button
                size="tiny"
                text
                :type="getActiveTextColor() ? 'primary' : 'default'"
                title="文字颜色"
              >
                <span class="keyword-rich-textcolor-icon">A</span>
              </n-button>
            </template>
            <div class="keyword-rich-color-picker">
              <div
                v-for="color in textColors"
                :key="color"
                class="keyword-rich-color-swatch"
                :class="{ 'is-active': getActiveTextColor() === color }"
                :style="{ backgroundColor: color }"
                @click="setTextColor(color)"
                :title="color"
              ></div>
              <label class="keyword-rich-color-swatch keyword-rich-color-custom" title="自定义颜色">
                <input
                  type="color"
                  v-model="customTextColor"
                  @change="applyCustomTextColor"
                  class="keyword-rich-color-input"
                />
                <span class="keyword-rich-color-custom__icon">+</span>
              </label>
              <div class="keyword-rich-color-picker__clear" @click="removeTextColor">
                清除颜色
              </div>
            </div>
          </n-popover>
        </div>

        <div class="keyword-rich-toolbar__divider"></div>

        <!-- Alignment -->
        <div class="keyword-rich-toolbar__group">
          <n-button
            size="tiny"
            text
            :type="isActive({ textAlign: 'left' }) ? 'primary' : 'default'"
            @click="setTextAlign('left')"
            title="左对齐"
          >
            ≡
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive({ textAlign: 'center' }) ? 'primary' : 'default'"
            @click="setTextAlign('center')"
            title="居中"
          >
            ≣
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive({ textAlign: 'right' }) ? 'primary' : 'default'"
            @click="setTextAlign('right')"
            title="右对齐"
          >
            ≣
          </n-button>
        </div>

        <div class="keyword-rich-toolbar__divider"></div>

        <!-- Lists & blocks -->
        <div class="keyword-rich-toolbar__group">
          <n-button
            size="tiny"
            text
            :type="isActive('bulletList') ? 'primary' : 'default'"
            @click="toggleBulletList"
            title="无序列表"
          >
            •
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('orderedList') ? 'primary' : 'default'"
            @click="toggleOrderedList"
            title="有序列表"
          >
            1.
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('blockquote') ? 'primary' : 'default'"
            @click="toggleBlockquote"
            title="引用"
          >
            "
          </n-button>
          <n-button
            size="tiny"
            text
            :type="isActive('codeBlock') ? 'primary' : 'default'"
            @click="toggleCodeBlock"
            title="代码块"
          >
            { }
          </n-button>
        </div>

        <div class="keyword-rich-toolbar__divider"></div>

        <!-- Insert -->
        <div class="keyword-rich-toolbar__group">
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
            @click="triggerFileSelect"
            title="插入图片"
            :loading="isUploading"
          >
            🖼
          </n-button>
          <n-button
            size="tiny"
            text
            @click="insertHorizontalRule"
            title="分割线"
          >
            ―
          </n-button>
          <n-button
            size="tiny"
            text
            @click="clearFormatting"
            title="清除格式"
          >
            ⊗
          </n-button>
        </div>
      </div>

      <!-- Editor content -->
      <div class="keyword-rich-editor-area">
        <component :is="EditorContent" v-if="editor" :editor="editor" />
      </div>

      <!-- Footer with character count -->
      <div class="keyword-rich-footer">
        <span class="keyword-rich-count">{{ contentLength }}/{{ maxlength }}</span>
      </div>
    </div>

    <!-- Link modal -->
    <n-modal
      v-model:show="linkModalShow"
      preset="card"
      :bordered="false"
      title="插入链接"
      style="width: 360px; max-width: 90vw;"
      :mask-closable="true"
    >
      <n-form label-placement="top">
        <n-form-item label="链接文本">
          <n-input
            v-model:value="linkText"
            placeholder="显示的文字（可选）"
          />
        </n-form-item>
        <n-form-item label="链接地址">
          <n-input
            v-model:value="linkUrl"
            placeholder="https://example.com"
            @keydown.enter="confirmLink"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
          <n-button @click="linkModalShow = false">取消</n-button>
          <n-button type="primary" @click="confirmLink" :disabled="!linkUrl.trim()">确定</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.keyword-rich-editor {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  border-radius: 6px;
  background: var(--sc-bg-input, #fff);
  transition: border-color 0.15s ease;
  flex: 1;
  min-height: 0;
  height: var(--desc-editor-height, 100%);
  max-height: var(--desc-editor-height, 100%);
}

.keyword-rich-editor:focus-within {
  border-color: var(--primary-color, rgba(59, 130, 246, 0.6));
}

.keyword-rich-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}

.keyword-rich-wrapper {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.keyword-rich-toolbar {
  display: flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.2));
  background: rgba(248, 250, 252, 0.6);
  flex-wrap: wrap;
}

.keyword-rich-toolbar__group {
  display: flex;
  align-items: center;
  gap: 0.15rem;
}

.keyword-rich-toolbar__divider {
  width: 1px;
  height: 1rem;
  background-color: rgba(148, 163, 184, 0.3);
  margin: 0 0.2rem;
}

.keyword-rich-highlight-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1rem;
  height: 1rem;
  border-radius: 0.2rem;
  font-weight: 600;
  font-size: 0.65rem;
  background-color: rgba(254, 240, 138, 0.6);
  color: #4b5563;
}

.keyword-rich-textcolor-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1rem;
  height: 1rem;
  font-weight: 600;
  font-size: 0.75rem;
  color: #4b5563;
  border-bottom: 2px solid #3b82f6;
}

/* Color picker */
.keyword-rich-color-picker {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.375rem;
  padding: 0.5rem;
  min-width: 8rem;
}

.keyword-rich-color-swatch {
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(0, 0, 0, 0.1);
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
}

.keyword-rich-color-swatch:hover {
  transform: scale(1.15);
}

.keyword-rich-color-swatch.is-active {
  box-shadow: 0 0 0 2px #3b82f6;
}

.keyword-rich-color-picker__clear {
  grid-column: span 4;
  padding: 0.375rem 0.25rem;
  text-align: center;
  font-size: 0.75rem;
  color: #6b7280;
  cursor: pointer;
  border-top: 1px solid #e5e7eb;
  margin-top: 0.25rem;
}

.keyword-rich-color-picker__clear:hover {
  color: #dc2626;
}

.keyword-rich-color-custom {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f87171 0%, #fbbf24 25%, #34d399 50%, #60a5fa 75%, #a78bfa 100%);
  cursor: pointer;
}

.keyword-rich-color-input {
  position: absolute;
  width: 100%;
  height: 100%;
  opacity: 0;
  cursor: pointer;
}

.keyword-rich-color-custom__icon {
  font-size: 0.875rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
  pointer-events: none;
}

.keyword-rich-editor-area {
  flex: 1;
  min-height: 100px;
  max-height: 100%;
  overflow-y: auto;
  padding: 8px 10px;
}

.keyword-rich-editor-area::-webkit-scrollbar {
  width: 4px;
}
.keyword-rich-editor-area::-webkit-scrollbar-track {
  background: transparent;
}
.keyword-rich-editor-area::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.3);
  border-radius: 2px;
}
.keyword-rich-editor-area::-webkit-scrollbar-thumb:hover {
  background: rgba(148, 163, 184, 0.5);
}

.keyword-rich-footer {
  display: flex;
  justify-content: flex-end;
  padding: 2px 8px 4px;
  border-top: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.12));
}

.keyword-rich-count {
  font-size: 10px;
  color: var(--sc-text-secondary, #94a3b8);
}

/* Night mode */
:root[data-display-palette='night'] .keyword-rich-editor {
  border-color: rgba(255, 255, 255, 0.12);
  background: rgba(30, 30, 34, 0.9);
}

:root[data-display-palette='night'] .keyword-rich-toolbar {
  background: rgba(39, 39, 42, 0.8);
  border-bottom-color: rgba(255, 255, 255, 0.08);
}

:root[data-display-palette='night'] .keyword-rich-toolbar__divider {
  background-color: rgba(255, 255, 255, 0.15);
}

:root[data-display-palette='night'] .keyword-rich-highlight-icon {
  background-color: rgba(254, 240, 138, 0.3);
  color: #e5e7eb;
}

:root[data-display-palette='night'] .keyword-rich-textcolor-icon {
  color: #e5e7eb;
  border-bottom-color: #60a5fa;
}

:root[data-display-palette='night'] .keyword-rich-color-picker {
  background-color: #2D2D31;
  border-radius: 0.375rem;
}

:root[data-display-palette='night'] .keyword-rich-color-swatch {
  border-color: rgba(255, 255, 255, 0.15);
}

:root[data-display-palette='night'] .keyword-rich-color-picker__clear {
  border-top-color: #52525b;
  color: #a1a1aa;
}

:root[data-display-palette='night'] .keyword-rich-color-picker__clear:hover {
  color: #f87171;
}

:root[data-display-palette='night'] .keyword-rich-editor-area::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
}

:root[data-display-palette='night'] .keyword-rich-editor-area::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.35);
}

:root[data-display-palette='night'] .keyword-rich-footer {
  border-color: rgba(255, 255, 255, 0.08);
}

:root[data-display-palette='night'] .keyword-rich-count {
  color: rgba(248, 250, 252, 0.5);
}
</style>

<style>
/* TipTap content styles - must be unscoped */
.keyword-rich-content {
  outline: none;
  min-height: 80px;
  color: var(--sc-text-primary, #1e293b);
  font-size: 14px;
  line-height: 1.55;
}

.keyword-rich-content p {
  margin: 0;
  min-height: 1.4rem;
}

.keyword-rich-content p + p {
  margin-top: 0.4rem;
}

.keyword-rich-content p.is-editor-empty:first-child::before {
  color: #9ca3af;
  content: attr(data-placeholder);
  float: left;
  height: 0;
  pointer-events: none;
}

.keyword-rich-content h1,
.keyword-rich-content h2,
.keyword-rich-content h3 {
  margin: 0.75rem 0 0.5rem;
  font-weight: 600;
  line-height: 1.3;
}

.keyword-rich-content h1:first-child,
.keyword-rich-content h2:first-child,
.keyword-rich-content h3:first-child {
  margin-top: 0;
}

.keyword-rich-content h1 {
  font-size: 1.5rem;
}

.keyword-rich-content h2 {
  font-size: 1.25rem;
}

.keyword-rich-content h3 {
  font-size: 1.1rem;
}

.keyword-rich-content strong {
  font-weight: 700;
}

.keyword-rich-content em {
  font-style: italic;
}

.keyword-rich-content u {
  text-decoration: underline;
}

.keyword-rich-content s {
  text-decoration: line-through;
}

.keyword-rich-content code {
  background-color: #f3f4f6;
  border-radius: 0.25rem;
  padding: 0.1rem 0.3rem;
  font-family: 'Courier New', 'Consolas', monospace;
  font-size: 0.9em;
}

.keyword-rich-content pre {
  background-color: #1f2937;
  color: #f9fafb;
  border-radius: 0.5rem;
  padding: 0.75rem;
  margin: 0.5rem 0;
  overflow-x: auto;
  font-family: 'Courier New', 'Consolas', monospace;
  font-size: 0.9em;
  line-height: 1.5;
}

.keyword-rich-content pre code {
  background: transparent;
  color: inherit;
  padding: 0;
}

.keyword-rich-content mark {
  background-color: #fef08a;
  padding: 0 0.1em;
  border-radius: 0.125rem;
}

.keyword-rich-content ul,
.keyword-rich-content ol {
  padding-left: 1.5rem;
  margin: 0.4rem 0;
}

.keyword-rich-content ul {
  list-style-type: disc;
}

.keyword-rich-content ol {
  list-style-type: decimal;
}

.keyword-rich-content li {
  margin: 0.2rem 0;
}

.keyword-rich-content li p {
  margin: 0;
}

.keyword-rich-content blockquote {
  border-left: 3px solid #3b82f6;
  padding-left: 0.75rem;
  margin: 0.4rem 0;
  color: #6b7280;
  font-style: italic;
}

.keyword-rich-content a {
  color: #3b82f6;
  text-decoration: underline;
}

.keyword-rich-content a:hover {
  color: #2563eb;
}

.keyword-rich-content hr {
  border: none;
  border-top: 2px solid #e5e7eb;
  margin: 1rem 0;
}

.keyword-rich-content .rich-inline-image,
.keyword-rich-content img {
  max-width: 100%;
  max-height: 10rem;
  height: auto;
  border-radius: 0.35rem;
  vertical-align: middle;
  margin: 0.25rem;
  display: inline-block;
  object-fit: contain;
}

.keyword-rich-content [style*="text-align: center"] {
  text-align: center;
}

.keyword-rich-content [style*="text-align: right"] {
  text-align: right;
}

/* Night mode for content */
:root[data-display-palette='night'] .keyword-rich-content {
  color: rgba(248, 250, 252, 0.9);
}

:root[data-display-palette='night'] .keyword-rich-content p.is-editor-empty:first-child::before {
  color: rgba(248, 250, 252, 0.4);
}

:root[data-display-palette='night'] .keyword-rich-content code {
  background-color: #52525b;
  color: #fafafa;
}

:root[data-display-palette='night'] .keyword-rich-content pre {
  background-color: #18181b;
  color: #f4f4f5;
}

:root[data-display-palette='night'] .keyword-rich-content mark {
  background-color: #854d0e;
  color: #fef3c7;
}

:root[data-display-palette='night'] .keyword-rich-content blockquote {
  border-left-color: #60a5fa;
  color: #d4d4d8;
}

:root[data-display-palette='night'] .keyword-rich-content a {
  color: #93c5fd;
}

:root[data-display-palette='night'] .keyword-rich-content a:hover {
  color: #bfdbfe;
}

:root[data-display-palette='night'] .keyword-rich-content hr {
  border-top-color: #52525b;
}
</style>
