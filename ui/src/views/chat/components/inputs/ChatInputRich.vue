<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, nextTick, shallowRef } from 'vue';
import type { MentionOption } from 'naive-ui';
import type { Editor } from '@tiptap/vue-3';

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
  (event: 'paste-image', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'drop-files', payload: { files: File[]; selectionStart: number; selectionEnd: number }): void
  (event: 'upload-button-click'): void
}>();

const editor = shallowRef<Editor | null>(null);
const editorElement = ref<HTMLElement | null>(null);
const isInitializing = ref(true);
const isFocused = ref(false);

const classList = computed(() => {
  const base: string[] = ['tiptap-editor'];
  if (props.whisperMode) {
    base.push('whisper-mode');
  }
  if (isFocused.value) {
    base.push('is-focused');
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

    const [
      { Editor: EditorClass },
      { EditorContent: EditorContentComp, BubbleMenu: BubbleMenuComp },
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
      import('@tiptap/extension-text-style'),
      import('@tiptap/extension-color'),
      import('@tiptap/extension-image'),
      import('@tiptap/extension-underline'),
      import('@tiptap/extension-highlight'),
      import('@tiptap/extension-text-align'),
    ]);

    EditorContent = EditorContentComp;
    BubbleMenu = BubbleMenuComp;

    // 创建编辑器实例
    editor.value = new EditorClass({
      content: props.modelValue || '<p></p>',
      extensions: [
        StarterKit.configure({
          heading: {
            levels: [1, 2, 3],
          },
          codeBlock: {
            HTMLAttributes: {
              class: 'code-block',
            },
          },
        }),
        TextStyle,
        Color,
        Underline,
        Highlight.configure({
          multicolor: true,
        }),
        TextAlign.configure({
          types: ['heading', 'paragraph'],
        }),
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
          HTMLAttributes: {
            class: 'rich-inline-image',
          },
        }),
      ],
      editorProps: {
        attributes: {
          class: 'tiptap-content',
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
        const jsonString = JSON.stringify(json);
        emit('update:modelValue', jsonString);
      },
      onFocus: () => {
        isFocused.value = true;
        emit('focus');
      },
      onBlur: () => {
        isFocused.value = false;
        emit('blur');
      },
      onCreate: ({ editor: ed }) => {
        // 初始化完成后，如果有内容则设置
        if (props.modelValue) {
          try {
            const json = JSON.parse(props.modelValue);
            ed.commands.setContent(json, false);
          } catch {
            // 如果不是 JSON，当作纯文本
            ed.commands.setContent(props.modelValue, false);
          }
        }
      },
    });

    isInitializing.value = false;
  } catch (error) {
    console.error('初始化富文本编辑器失败:', error);
    isInitializing.value = false;
  }
};

// 初始化
initEditor();

// 监听外部值变化
watch(() => props.modelValue, (newValue) => {
  if (!editor.value || editor.value.isDestroyed) return;

  try {
    const currentJson = JSON.stringify(editor.value.getJSON());
    if (currentJson !== newValue) {
      const json = JSON.parse(newValue);
      editor.value.commands.setContent(json, false);
    }
  } catch {
    // 非 JSON 格式，跳过
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

const insertImagePlaceholder = (markerId: string, previewUrl: string) => {
  if (!editor.value) return;

  // 在当前光标位置插入图片
  editor.value.chain().focus().setImage({ src: previewUrl, alt: `图片-${markerId}` }).run();
};

// Toolbar actions
const toggleBold = () => editor.value?.chain().focus().toggleBold().run();
const toggleItalic = () => editor.value?.chain().focus().toggleItalic().run();
const toggleUnderline = () => editor.value?.chain().focus().toggleUnderline().run();
const toggleStrike = () => editor.value?.chain().focus().toggleStrike().run();
const toggleCode = () => editor.value?.chain().focus().toggleCode().run();
const toggleCodeBlock = () => editor.value?.chain().focus().toggleCodeBlock().run();
const toggleBulletList = () => editor.value?.chain().focus().toggleBulletList().run();
const toggleOrderedList = () => editor.value?.chain().focus().toggleOrderedList().run();
const toggleBlockquote = () => editor.value?.chain().focus().toggleBlockquote().run();
const setHeading = (level: 1 | 2 | 3) => editor.value?.chain().focus().toggleHeading({ level }).run();
const setParagraph = () => editor.value?.chain().focus().setParagraph().run();
const setTextAlign = (align: 'left' | 'center' | 'right' | 'justify') => editor.value?.chain().focus().setTextAlign(align).run();
const toggleHighlight = () => editor.value?.chain().focus().toggleHighlight().run();
const insertHorizontalRule = () => editor.value?.chain().focus().setHorizontalRule().run();
const clearFormatting = () => editor.value?.chain().focus().clearNodes().unsetAllMarks().run();

const setLink = () => {
  const url = window.prompt('输入链接地址:');
  if (url) {
    editor.value?.chain().focus().setLink({ href: url }).run();
  }
};

const unsetLink = () => {
  editor.value?.chain().focus().unsetLink().run();
};

const isActive = (name: string, attrs?: Record<string, any>) => {
  return editor.value?.isActive(name, attrs) ?? false;
};

const handleKeydown = (event: KeyboardEvent) => {
  emit('keydown', event);
};

onBeforeUnmount(() => {
  editor.value?.destroy();
});

defineExpose({
  focus,
  blur,
  getTextarea,
  getInstance: () => editor.value,
  getEditor: () => editor.value,
  getJson: () => editor.value?.getJSON(),
  insertImagePlaceholder,
});
</script>

<template>
  <div :class="classList">
    <div v-if="isInitializing" class="tiptap-loading">
      <n-spin size="small" />
      <span class="ml-2 text-sm text-gray-500">加载编辑器...</span>
    </div>

    <div v-else class="tiptap-wrapper">
      <!-- 固定工具栏 -->
      <div class="tiptap-toolbar">
        <div class="tiptap-toolbar__group">
          <n-button
            size="small"
            text
            :type="isActive('paragraph') ? 'primary' : 'default'"
            @click="setParagraph"
            title="正文"
          >
            P
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('heading', { level: 1 }) ? 'primary' : 'default'"
            @click="setHeading(1)"
            title="标题 1"
          >
            H1
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('heading', { level: 2 }) ? 'primary' : 'default'"
            @click="setHeading(2)"
            title="标题 2"
          >
            H2
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('heading', { level: 3 }) ? 'primary' : 'default'"
            @click="setHeading(3)"
            title="标题 3"
          >
            H3
          </n-button>
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
          <n-button
            size="small"
            text
            :type="isActive('underline') ? 'primary' : 'default'"
            @click="toggleUnderline"
            title="下划线 (Ctrl+U)"
          >
            <span class="underline">U</span>
          </n-button>
          <n-button
            size="small"
            text
            :type="isActive('strike') ? 'primary' : 'default'"
            @click="toggleStrike"
            title="删除线"
          >
            <span class="line-through">S</span>
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
            :type="isActive('highlight') ? 'primary' : 'default'"
            @click="toggleHighlight"
            title="高亮"
          >
            H
          </n-button>
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
            @click="emit('upload-button-click')"
            title="插入图片"
          >
            🖼
          </n-button>
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
      <div class="tiptap-editor-wrapper" ref="editorElement" @keydown="handleKeydown">
        <component :is="EditorContent" v-if="editor" :editor="editor" />

        <!-- BubbleMenu 浮动工具栏 -->
        <component
          v-if="editor && BubbleMenu"
          :is="BubbleMenu"
          :editor="editor"
          :tippy-options="{ duration: 100, placement: 'top' }"
        >
          <div class="tiptap-bubble-menu">
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
              :type="isActive('underline') ? 'primary' : 'default'"
              @click="toggleUnderline"
              title="下划线"
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
          </div>
        </component>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.tiptap-editor {
  width: 100%;
  border: 1px solid #e5e7eb;
  border-radius: 0.85rem;
  background-color: #f9fafb;
  overflow: hidden;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;

  &.is-focused {
    border-color: #3b82f6;
    box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.25);
  }

  &.whisper-mode {
    border-color: #7c3aed;
    box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.35);
    background-color: rgba(250, 245, 255, 0.92);
  }
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
  border-bottom: 1px solid #e5e7eb;
  background-color: #ffffff;
  flex-wrap: wrap;
}

.tiptap-toolbar__group {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.tiptap-toolbar__divider {
  width: 1px;
  height: 1.25rem;
  background-color: #e5e7eb;
  margin: 0 0.25rem;
}

.tiptap-editor-wrapper {
  position: relative;
  min-height: 3rem;
  max-height: 12rem;
  overflow-y: auto;
}

.tiptap-bubble-menu {
  display: flex;
  gap: 0.25rem;
  padding: 0.375rem 0.5rem;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  align-items: center;
}

.tiptap-bubble-menu__divider {
  width: 1px;
  height: 1rem;
  background-color: #e5e7eb;
  margin: 0 0.25rem;
}
</style>

<style lang="scss">
.tiptap-content {
  padding: 0.75rem 1rem;
  outline: none;
  min-height: 3rem;
  max-height: 20rem;
  overflow-y: auto;

  /* 基础文本样式 */
  p {
    margin: 0;
    line-height: 1.6;
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
    max-height: 12rem;
    height: auto;
    border-radius: 0.5rem;
    vertical-align: middle;
    margin: 0.5rem 0.25rem;
    display: inline-block;
    object-fit: contain;
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
</style>
