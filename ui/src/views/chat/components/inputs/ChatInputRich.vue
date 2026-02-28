<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, nextTick, shallowRef, reactive } from 'vue';
import { useMessage } from 'naive-ui';
import type { MentionOption } from 'naive-ui';
import type { Editor } from '@tiptap/vue-3';
import { Spoiler } from '@/utils/tiptap-spoiler';
import { useChatStore } from '@/stores/chat';
import { useIFormStore } from '@/stores/iform';
import { useUtilsStore } from '@/stores/utils';
import { generateIFormEmbedLink } from '@/utils/iformEmbedLink';
import { matchText } from '@/utils/pinyinMatch';
import { contentUnescape } from '@/utils/tools';

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
const isComposing = ref(false);

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

// 链接弹窗状态
const linkModalShow = ref(false);
const linkText = ref('');
const linkUrl = ref('');
const linkOpenInNewTab = ref(false);

const quickIFormModalShow = ref(false);
const creatingIForm = ref(false);
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

const applyCustomHighlightColor = () => {
  setHighlightColor(customHighlightColor.value);
};

const applyCustomTextColor = () => {
  setTextColor(customTextColor.value);
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
      { Editor: EditorClass, Node: TiptapNodeClass, mergeAttributes },
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
      import('@tiptap/extension-text-style').then(m => ({ default: m.TextStyle })),
      import('@tiptap/extension-color').then(m => ({ default: m.Color })),
      import('@tiptap/extension-image'),
      import('@tiptap/extension-underline'),
      import('@tiptap/extension-highlight'),
      import('@tiptap/extension-text-align'),
    ]);

    EditorContent = EditorContentComp;
    BubbleMenu = BubbleMenuComp;

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
    });

    // 创建编辑器实例
    editor.value = new EditorClass({
      content: props.modelValue || '<p></p>',
      extensions: [
        SatoriMention,
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
        Spoiler,
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
        isSyncingFromProps.value = true;
        emit('update:modelValue', jsonString);
        checkMentionTrigger(ed);
        nextTick(() => {
          isSyncingFromProps.value = false;
        });
      },
      onFocus: () => {
        isFocused.value = true;
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
          ed.commands.setContent(cloneEmptyDoc(), false);
          return;
        }
        try {
          const json = JSON.parse(props.modelValue);
          const normalized = normalizeMentionTokensInDoc(json);
          ed.commands.setContent(normalized, false);
        } catch {
          // 如果不是 JSON，当作纯文本
          ed.commands.setContent(props.modelValue, false);
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
  if (isSyncingFromProps.value) return;

  if (!newValue || newValue.trim() === '') {
    editor.value.commands.setContent(cloneEmptyDoc(), false);
    editor.value.commands.setTextSelection(0);
    return;
  }

  try {
    const incomingJson = JSON.parse(newValue);
    const normalizedIncoming = normalizeMentionTokensInDoc(incomingJson);
    const currentSerialized = JSON.stringify(serializeMentionNodesToTokens(editor.value.getJSON()));
    const incomingSerialized = JSON.stringify(serializeMentionNodesToTokens(normalizedIncoming));
    if (currentSerialized !== incomingSerialized) {
      editor.value.commands.setContent(normalizedIncoming, false);
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
const setTextAlign = (align: 'left' | 'center' | 'right' | 'justify') => editor.value?.chain().focus().setTextAlign(align).run();
const toggleHighlight = () => editor.value?.chain().focus().toggleHighlight().run();
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

// 高亮颜色操作
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

// 文字颜色操作
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

const setLink = () => {
  const { from, to } = editor.value?.state.selection || { from: 0, to: 0 };
  const hasSelection = from !== to;

  if (hasSelection) {
    // 有选中文本，获取选中内容作为默认链接文本
    const selectedText = editor.value?.state.doc.textBetween(from, to, ' ') || '';
    linkText.value = selectedText;
  } else {
    linkText.value = '';
  }
  linkUrl.value = '';
  linkOpenInNewTab.value = false;
  linkModalShow.value = true;
};

const confirmLink = () => {
  if (!linkUrl.value.trim()) {
    linkModalShow.value = false;
    return;
  }

  const url = linkUrl.value.trim();
  const target = linkOpenInNewTab.value ? '_blank' : '_self';
  const { from, to } = editor.value?.state.selection || { from: 0, to: 0 };
  const hasSelection = from !== to;

  if (hasSelection) {
    // 有选中文本，直接设置链接
    editor.value?.chain().focus().setLink({ href: url, target }).run();
  } else {
    // 没有选中文本，插入带链接的文本
    const text = linkText.value.trim() || url;
    editor.value?.chain().focus().insertContent({
      type: 'text',
      text: text,
      marks: [{ type: 'link', attrs: { href: url, target } }],
    }).run();
  }

  linkModalShow.value = false;
  linkText.value = '';
  linkUrl.value = '';
  linkOpenInNewTab.value = false;
};

const unsetLink = () => {
  editor.value?.chain().focus().unsetLink().run();
};

const isActive = (name: string, attrs?: Record<string, any>) => {
  return editor.value?.isActive(name, attrs) ?? false;
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
          <!-- 高亮颜色选择器 -->
          <n-popover
            trigger="click"
            placement="bottom"
            v-model:show="highlightColorPopoverShow"
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
            <div class="tiptap-color-picker">
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
            <div class="tiptap-color-picker">
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
              :style="mentionDropdownStyle"
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
        </Teleport>

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
          </div>
        </component>
      </div>
    </div>

    <!-- 链接插入弹窗 -->
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
            placeholder="显示的文字（可选，留空则显示链接地址）"
          />
        </n-form-item>
        <n-form-item label="链接地址">
          <n-input
            v-model:value="linkUrl"
            placeholder="https://example.com"
            @keydown.enter="confirmLink"
          />
        </n-form-item>
        <n-form-item label="打开方式">
          <n-checkbox v-model:checked="linkOpenInNewTab">在新标签页中打开</n-checkbox>
        </n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
          <n-button @click="linkModalShow = false">取消</n-button>
          <n-button type="primary" @click="confirmLink" :disabled="!linkUrl.trim()">确定</n-button>
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
            placeholder="可选：粘贴 iframe 代码"
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

:root[data-display-palette='night'] .mention-dropdown {
  background: var(--sc-bg-surface, #1e1e2e);
  border-color: var(--sc-border-mute, #3f3f5a);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
</style>

<style lang="scss">
.tiptap-content {
  padding: 0.75rem 1rem;
  outline: none;
  min-height: 3rem;
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

/* 工具栏夜间模式 */
:root[data-display-palette='night'] .tiptap-toolbar {
  background-color: var(--sc-bg-elevated, #27272a);
  border-bottom-color: var(--sc-border-strong, #52525b);
}

:root[data-display-palette='night'] .tiptap-toolbar__divider {
  background-color: var(--sc-border-strong, #3f3f46);
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
