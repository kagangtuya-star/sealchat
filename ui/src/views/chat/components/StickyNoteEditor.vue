<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, nextTick, shallowRef } from 'vue';
import type { Editor } from '@tiptap/vue-3';
import { Spoiler } from '@/utils/tiptap-spoiler';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useMessage } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import { useIFormStore } from '@/stores/iform';
import { useUtilsStore } from '@/stores/utils';
import { generateIFormEmbedLink } from '@/utils/iformEmbedLink';
import { copyTextWithFallback } from '@/utils/clipboard';

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  disabled?: boolean
  channelId?: string
}>(), {
  modelValue: '',
  placeholder: '在此输入内容...',
  disabled: false,
  channelId: '',
});

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'focus'): void
  (event: 'blur'): void
}>();

const message = useMessage();
const chat = useChatStore();
const iFormStore = useIFormStore();
const utils = useUtilsStore();
iFormStore.bootstrap();

const editor = shallowRef<Editor | null>(null);
const editorElement = ref<HTMLElement | null>(null);
const isInitializing = ref(true);
const isFocused = ref(false);
const isSyncingFromProps = ref(false);
const isUploading = ref(false);

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
  const channelId = (props.channelId || '').trim();
  const worldId = (chat.currentWorldId || '').trim();
  if (!channelId || !worldId) {
    return '';
  }
  const firstForm = iFormStore.formsByChannel[channelId]?.[0];
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

const insertIFormEmbedLink = () => {
  const link = defaultIFormEmbedLink.value;
  if (!link || !editor.value) {
    message.warning('当前频道暂无可插入 iForm');
    return;
  }
  editor.value.chain().focus().insertContent(link).run();
};

const copyIFormEmbedLink = async () => {
  const link = defaultIFormEmbedLink.value;
  if (!link) {
    message.warning('当前频道暂无可复制 iForm');
    return;
  }
  const copied = await copyTextWithFallback(link);
  if (copied) {
    message.success('iForm 嵌入链接已复制');
  } else {
    message.error('复制失败');
  }
};

// 颜色选择器状态
const highlightColorPopoverShow = ref(false);

// 预设高亮颜色色板
const highlightColors = [
  '#fef08a', // 黄色
  '#bbf7d0', // 绿色
  '#bfdbfe', // 蓝色
  '#fecaca', // 红色
  '#e9d5ff', // 紫色
  '#fed7aa', // 橙色
];

const EMPTY_DOC = {
  type: 'doc',
  content: [{ type: 'paragraph' }],
};

const cloneEmptyDoc = () => JSON.parse(JSON.stringify(EMPTY_DOC));

let EditorContent: any = null;

// 动态导入 TipTap
const initEditor = async () => {
  try {
    isInitializing.value = true;

    const [
      { Editor: EditorClass },
      { EditorContent: EditorContentComp },
      { default: StarterKit },
      { default: TextStyle },
      { default: Color },
      { default: Image },
      { default: Highlight },
    ] = await Promise.all([
      import('@tiptap/core'),
      import('@tiptap/vue-3'),
      import('@tiptap/starter-kit'),
      import('@tiptap/extension-text-style').then(m => ({ default: m.TextStyle })),
      import('@tiptap/extension-color').then(m => ({ default: m.Color })),
      import('@tiptap/extension-image'),
      import('@tiptap/extension-highlight'),
    ]);

    EditorContent = EditorContentComp;

    // 创建编辑器实例
    editor.value = new EditorClass({
      content: props.modelValue || '<p></p>',
      extensions: [
        StarterKit.configure({
          heading: false,
          codeBlock: false,
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
        Color,
        Highlight.configure({
          multicolor: true,
        }),
        Spoiler,
        Image.configure({
          inline: true,
          allowBase64: true,
          HTMLAttributes: {
            class: 'sticky-note-editor__image',
          },
        }),
      ],
      editorProps: {
        attributes: {
          class: 'sticky-note-editor__content',
        },
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

          const files = Array.from(event.dataTransfer?.files || []).filter(file =>
            file.type.startsWith('image/')
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
      onFocus: () => {
        isFocused.value = true;
        emit('focus');
      },
      onBlur: () => {
        isFocused.value = false;
        emit('blur');
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
          // 纯文本兼容
          ed.commands.setContent(props.modelValue, false);
        }
      },
    });

    isInitializing.value = false;
  } catch (error) {
    console.error('初始化便签编辑器失败:', error);
    isInitializing.value = false;
  }
};

initEditor();

// 监听外部值变化
watch(() => props.modelValue, (newValue) => {
  if (!editor.value || editor.value.isDestroyed) return;
  if (isSyncingFromProps.value) return;

  if (!newValue || newValue.trim() === '') {
    editor.value.commands.setContent(cloneEmptyDoc(), false);
    return;
  }

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

// 图片上传处理
const handleImageUpload = async (files: File[]) => {
  if (!editor.value || isUploading.value) return;

  isUploading.value = true;

  try {
    for (const file of files) {
      const result = await uploadImageAttachment(file, {
        channelId: props.channelId,
      });

      if (result.attachmentId) {
        // 将 id:xxx 格式转换为实际 URL
        let imageUrl = result.attachmentId;
        if (imageUrl.startsWith('id:')) {
          const attachmentId = imageUrl.slice(3);
          // 动态获取 urlBase
          const { urlBase } = await import('@/stores/_config');
          imageUrl = `${urlBase}/api/v1/attachment/${attachmentId}`;
        }
        
        // 插入图片
        editor.value.chain().focus().setImage({
          src: imageUrl,
          alt: file.name,
        }).run();
      }
    }
  } catch (error: any) {
    message.error(error.message || '图片上传失败');
  } finally {
    isUploading.value = false;
  }
};

// 文件选择
const fileInputRef = ref<HTMLInputElement | null>(null);

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
const toggleBulletList = () => editor.value?.chain().focus().toggleBulletList().run();
const toggleOrderedList = () => editor.value?.chain().focus().toggleOrderedList().run();

const setHighlightColor = (color: string) => {
  editor.value?.chain().focus().setHighlight({ color }).run();
  highlightColorPopoverShow.value = false;
};

const removeHighlight = () => {
  editor.value?.chain().focus().unsetHighlight().run();
  highlightColorPopoverShow.value = false;
};

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

const focus = () => {
  nextTick(() => {
    editor.value?.commands.focus();
  });
};

const blur = () => {
  editor.value?.commands.blur();
};

const getJson = () => editor.value?.getJSON();

const setContent = (content: string) => {
  if (!editor.value) return;
  try {
    const json = JSON.parse(content);
    editor.value.commands.setContent(json, false);
  } catch {
    editor.value.commands.setContent(content, false);
  }
};

onBeforeUnmount(() => {
  editor.value?.destroy();
});

defineExpose({
  focus,
  blur,
  getJson,
  setContent,
  getEditor: () => editor.value,
});
</script>

<template>
  <div class="sticky-note-editor">
    <div v-if="isInitializing" class="sticky-note-editor__loading">
      <n-spin size="small" />
    </div>

    <template v-else>
      <!-- 工具栏 -->
      <div class="sticky-note-editor__toolbar">
        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('bold') }"
          @click="toggleBold"
          title="粗体"
        >
          <strong>B</strong>
        </button>
        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('italic') }"
          @click="toggleItalic"
          title="斜体"
        >
          <em>I</em>
        </button>
        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('underline') }"
          @click="toggleUnderline"
          title="下划线"
        >
          <u>U</u>
        </button>
        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('strike') }"
          @click="toggleStrike"
          title="删除线"
        >
          <s>S</s>
        </button>
        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('spoiler') }"
          @click="toggleSpoiler"
          title="隐藏/揭示"
        >
          SP
        </button>

        <span class="sticky-note-editor__divider"></span>

        <!-- 高亮颜色 -->
        <n-popover
          trigger="click"
          placement="bottom"
          v-model:show="highlightColorPopoverShow"
        >
          <template #trigger>
            <button
              class="sticky-note-editor__btn"
              :class="{ 'is-active': isActive('highlight') }"
              title="高亮"
            >
              <span class="highlight-icon">H</span>
            </button>
          </template>
          <div class="sticky-note-editor__color-picker">
            <div
              v-for="color in highlightColors"
              :key="color"
              class="sticky-note-editor__color-swatch"
              :style="{ backgroundColor: color }"
              @click="setHighlightColor(color)"
            ></div>
            <div class="sticky-note-editor__color-clear" @click="removeHighlight">
              清除
            </div>
          </div>
        </n-popover>

        <span class="sticky-note-editor__divider"></span>

        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('bulletList') }"
          @click="toggleBulletList"
          title="无序列表"
        >
          •
        </button>
        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('orderedList') }"
          @click="toggleOrderedList"
          title="有序列表"
        >
          1.
        </button>

        <span class="sticky-note-editor__divider"></span>

        <button
          class="sticky-note-editor__btn"
          :class="{ 'is-active': isActive('link') }"
          @click="isActive('link') ? unsetLink() : setLink()"
          :title="isActive('link') ? '移除链接' : '插入链接'"
        >
          🔗
        </button>
        <button
          class="sticky-note-editor__btn"
          :disabled="!defaultIFormEmbedLink"
          @click="copyIFormEmbedLink"
          :title="defaultIFormEmbedLink ? '复制首个 iForm 嵌入链接' : '当前频道暂无 iForm'"
        >
          ⧉
        </button>
        <button
          class="sticky-note-editor__btn"
          :disabled="!defaultIFormEmbedLink"
          @click="insertIFormEmbedLink"
          :title="defaultIFormEmbedLink ? '插入首个 iForm 嵌入链接' : '当前频道暂无 iForm'"
        >
          ↘
        </button>
        <button
          class="sticky-note-editor__btn"
          @click="triggerFileSelect"
          title="插入图片"
          :disabled="isUploading"
        >
          <template v-if="isUploading">
            <n-spin size="tiny" />
          </template>
          <template v-else>🖼</template>
        </button>
      </div>

      <!-- 隐藏的文件选择器 -->
      <input
        ref="fileInputRef"
        type="file"
        accept="image/*"
        multiple
        style="display: none"
        @change="handleFileSelect"
      />

      <!-- 编辑器内容 -->
      <div
        class="sticky-note-editor__wrapper"
        ref="editorElement"
      >
        <component :is="EditorContent" v-if="editor" :editor="editor" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.sticky-note-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: rgba(255, 255, 255, 0.4);
  border-radius: 4px;
}

.sticky-note-editor__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 60px;
}

.sticky-note-editor__toolbar {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px 6px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.1);
  flex-wrap: wrap;
}

.sticky-note-editor__btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  border-radius: 3px;
  cursor: pointer;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.7);
  transition: all 0.15s;
}

.sticky-note-editor__btn:hover {
  background: rgba(0, 0, 0, 0.1);
}

.sticky-note-editor__btn.is-active {
  background: rgba(0, 0, 0, 0.15);
  color: rgba(0, 0, 0, 0.9);
}

.sticky-note-editor__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.sticky-note-editor__divider {
  width: 1px;
  height: 16px;
  background: rgba(0, 0, 0, 0.15);
  margin: 0 4px;
}

.highlight-icon {
  background: linear-gradient(135deg, #fef08a, #fde047);
  padding: 2px 4px;
  border-radius: 2px;
  font-size: 10px;
  font-weight: bold;
}

.sticky-note-editor__color-picker {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  max-width: 160px;
}

.sticky-note-editor__color-swatch {
  width: 20px;
  height: 20px;
  border-radius: 3px;
  cursor: pointer;
  border: 1px solid rgba(0, 0, 0, 0.1);
  transition: transform 0.15s;
}

.sticky-note-editor__color-swatch:hover {
  transform: scale(1.15);
}

.sticky-note-editor__color-clear {
  width: 100%;
  text-align: center;
  font-size: 11px;
  color: rgba(0, 0, 0, 0.5);
  cursor: pointer;
  padding: 4px 0 0;
  margin-top: 4px;
  border-top: 1px solid rgba(0, 0, 0, 0.1);
}

.sticky-note-editor__color-clear:hover {
  color: rgba(0, 0, 0, 0.8);
}

.sticky-note-editor__wrapper {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content) {
  outline: none;
  min-height: 100%;
  font-size: 13px;
  line-height: 1.5;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content p) {
  margin: 0 0 0.5em;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content p:last-child) {
  margin-bottom: 0;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content ul),
.sticky-note-editor__wrapper :deep(.sticky-note-editor__content ol) {
  margin: 0.5em 0;
  padding-left: 1.5em;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content ul) {
  list-style-type: disc;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content ol) {
  list-style-type: decimal;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__content a) {
  color: #2563eb;
  text-decoration: underline;
}

.sticky-note-editor__wrapper :deep(.sticky-note-editor__image) {
  max-width: 100%;
  height: auto;
  border-radius: 4px;
  margin: 4px 0;
}

.sticky-note-editor__wrapper :deep(.ProseMirror-focused) {
  outline: none;
}

.sticky-note-editor__wrapper :deep(p.is-editor-empty:first-child::before) {
  content: attr(data-placeholder);
  float: left;
  color: rgba(0, 0, 0, 0.35);
  pointer-events: none;
  height: 0;
}
</style>

<style>
/* ===== 夜间模式和自定义主题适配 ===== */
/* 便签背景始终是浅色的，所以文字需要保持深色 */
/* 使用非 scoped 样式因为便签使用 Teleport 渲染 */

:root[data-display-palette='night'] .sticky-note-editor__wrapper .sticky-note-editor__content,
:root[data-display-palette='night'] .sticky-note-editor__wrapper .ProseMirror {
  color: rgba(0, 0, 0, 0.85) !important;
  caret-color: rgba(0, 0, 0, 0.85);
}

:root[data-display-palette='night'] .sticky-note-editor__wrapper .sticky-note-editor__content p,
:root[data-display-palette='night'] .sticky-note-editor__wrapper .ProseMirror p {
  color: inherit;
}

:root[data-display-palette='night'] .sticky-note-editor__btn {
  color: rgba(0, 0, 0, 0.7);
}

:root[data-display-palette='night'] .sticky-note-editor__btn:hover {
  color: rgba(0, 0, 0, 0.9);
}

:root[data-display-palette='night'] .sticky-note-editor__btn.is-active {
  color: rgba(0, 0, 0, 0.9);
}

/* 自定义主题模式 - 同样需要保持便签文字深色 */
:root[data-custom-theme='true'] .sticky-note-editor__wrapper .sticky-note-editor__content,
:root[data-custom-theme='true'] .sticky-note-editor__wrapper .ProseMirror {
  color: rgba(0, 0, 0, 0.85) !important;
  caret-color: rgba(0, 0, 0, 0.85);
}

:root[data-custom-theme='true'] .sticky-note-editor__wrapper .sticky-note-editor__content p,
:root[data-custom-theme='true'] .sticky-note-editor__wrapper .ProseMirror p {
  color: inherit;
}

:root[data-custom-theme='true'] .sticky-note-editor__btn {
  color: rgba(0, 0, 0, 0.7);
}
</style>
