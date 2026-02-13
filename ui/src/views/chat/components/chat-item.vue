<script setup lang="tsx">
import dayjs from 'dayjs';
import Element from '@satorijs/element'
import { onMounted, ref, h, computed, watch, onBeforeUnmount, nextTick, defineAsyncComponent } from 'vue';
import type { PropType } from 'vue';
import { urlBase } from '@/stores/_config';
import DOMPurify from 'dompurify';
import { useUserStore } from '@/stores/user';
import { useChatStore } from '@/stores/chat';
import { useStickyNoteStore, type StickyNote, type StickyNoteType, type StickyNoteEmbedLayoutState } from '@/stores/stickyNote';
import { useIFormStore } from '@/stores/iform';
import { useUtilsStore } from '@/stores/utils';
import { Howl, Howler } from 'howler';
import { useMessage } from 'naive-ui';
import Avatar from '@/components/avatar.vue'
import { ArrowBackUp, Lock, Edit, Check, X } from '@vicons/tabler';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { normalizeAttachmentId, resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { onLongPress } from '@vueuse/core';
import Viewer from 'viewerjs';
import 'viewerjs/dist/viewer.css';
import { useWorldGlossaryStore } from '@/stores/worldGlossary'
import { useDisplayStore, type TimestampFormat } from '@/stores/display'
import { useChannelImageLayoutStore } from '@/stores/channelImageLayout';
import { refreshWorldKeywordHighlights } from '@/utils/worldKeywordHighlighter'
import { createKeywordTooltip } from '@/utils/keywordTooltip'
import { resolveMessageLinkInfo, renderMessageLinkHtml } from '@/utils/messageLinkRenderer'
import { MESSAGE_LINK_REGEX, TITLED_MESSAGE_LINK_REGEX, parseMessageLink } from '@/utils/messageLink'
import { parseSingleIFormEmbedLinkText, updateIFormEmbedLinkSize } from '@/utils/iformEmbedLink'
import { parseSingleStickyNoteEmbedLinkText, type StickyNoteEmbedLinkParams } from '@/utils/stickyNoteEmbedLink'
import { copyTextWithFallback } from '@/utils/clipboard'
import { chatEvent } from '@/stores/chat'
import CharacterCardBadge from './CharacterCardBadge.vue'
import MessageReactions from './MessageReactions.vue'
import IFormEmbedFrame from '@/components/iform/IFormEmbedFrame.vue'
import type { ChannelIForm } from '@/types/iform';

type EditingPreviewInfo = {
  userId: string;
  displayName: string;
  avatar?: string;
  content: string;
  indicatorOnly: boolean;
  isSelf: boolean;
  summary: string;
  previewHtml: string;
  tone: 'ic' | 'ooc';
};

const user = useUserStore();
const chat = useChatStore();
const stickyNoteStore = useStickyNoteStore();
const iFormStore = useIFormStore();
const utils = useUtilsStore();
const { t } = useI18n();
const worldGlossary = useWorldGlossaryStore();
const displayStore = useDisplayStore();
const channelImageLayout = useChannelImageLayoutStore();

const isMobileUa = typeof navigator !== 'undefined'
  ? /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  : false;

function timeFormat2(time?: string) {
  if (!time) return '未知';
  // console.log('???', time, typeof time)
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss');
}

const timestampFormatPatterns: Record<Exclude<TimestampFormat, 'relative'>, string> = {
  time: 'HH:mm',
  datetime: 'YYYY-MM-DD HH:mm',
  datetimeSeconds: 'YYYY-MM-DD HH:mm:ss',
};

const formatTimestampByPreference = (time?: string, format: TimestampFormat = 'datetimeSeconds') => {
  if (!time) return '未知';
  if (format === 'relative') {
    return dayjs(time).fromNow();
  }
  const pattern = timestampFormatPatterns[format] || timestampFormatPatterns.datetimeSeconds;
  return dayjs(time).format(pattern);
};

const TIMESTAMP_HOVER_DELAY = 2000;

let hasImage = ref(false);
const messageContentRef = ref<HTMLElement | null>(null);
let stopMessageLongPress: (() => void) | null = null;
let inlineImageViewer: Viewer | null = null;

const IMAGE_LAYOUT_MIN_SIZE = 48;
const IMAGE_LAYOUT_MAX_SIZE = 4096;

const imageResizeMode = ref(false);
const imageResizeSelectedAttachmentId = ref('');
const imageResizeDraftLayouts = ref<Record<string, { width: number; height: number }>>({});
const imageResizeFreeScaling = ref(false);
let imageResizePointerState: {
  pointerId: number;
  pointerType: string;
  moveThreshold: number;
  movementScale: number;
  attachmentId: string;
  startX: number;
  startY: number;
  startWidth: number;
  startHeight: number;
  aspectRatio: number;
  moved: boolean;
} | null = null;

const diceChipHtmlPattern = /<span[^>]*class="[^"]*dice-chip[^"]*"/i;

const MESSAGE_IFORM_MIN_WIDTH = 120;
const MESSAGE_IFORM_MIN_HEIGHT = 72;
const MESSAGE_IFORM_RESIZE_SYNC_DEBOUNCE = 480;
const StickyNoteCounterEmbed = defineAsyncComponent(() => import('./sticky-notes/StickyNoteCounter.vue'));
const StickyNoteListEmbed = defineAsyncComponent(() => import('./sticky-notes/StickyNoteList.vue'));
const StickyNoteSliderEmbed = defineAsyncComponent(() => import('./sticky-notes/StickyNoteSlider.vue'));
const StickyNoteTimerEmbed = defineAsyncComponent(() => import('./sticky-notes/StickyNoteTimer.vue'));
const StickyNoteClockEmbed = defineAsyncComponent(() => import('./sticky-notes/StickyNoteClock.vue'));
const StickyNoteRoundCounterEmbed = defineAsyncComponent(() => import('./sticky-notes/StickyNoteRoundCounter.vue'));

const resolveStickyNoteAccent = (color: string): string => {
  const colorMap: Record<string, string> = {
    yellow: '#ffc107',
    pink: '#e91e63',
    green: '#4caf50',
    blue: '#2196f3',
    purple: '#9c27b0',
    orange: '#ff9800',
  };
  return colorMap[color] || '#64748b';
};

const resolveStickyNoteContentText = (note: any): string => {
  const contentText = String(note?.contentText || '').trim();
  if (contentText) {
    return contentText;
  }
  const rawContent = String(note?.content || '').trim();
  if (!rawContent) {
    return '';
  }
  if (isTipTapJson(rawContent)) {
    try {
      return tiptapJsonToPlainText(rawContent).trim();
    } catch {
      return rawContent;
    }
  }
  return rawContent;
};

const resolveStickyNoteEmbedComponent = (type: StickyNoteType) => {
  switch (type) {
    case 'counter':
      return StickyNoteCounterEmbed;
    case 'list':
      return StickyNoteListEmbed;
    case 'slider':
      return StickyNoteSliderEmbed;
    case 'timer':
      return StickyNoteTimerEmbed;
    case 'clock':
      return StickyNoteClockEmbed;
    case 'roundCounter':
      return StickyNoteRoundCounterEmbed;
    default:
      return null;
  }
};

const STICKY_NOTE_EMBED_RESIZE_MIN_WIDTH = 150;
const STICKY_NOTE_EMBED_RESIZE_MIN_HEIGHT = 60;
const STICKY_NOTE_EMBED_RESIZE_MAX_WIDTH = 760;
const STICKY_NOTE_EMBED_RESIZE_MAX_HEIGHT = 680;

const clampStickyNoteEmbedSize = (value: number, min: number, max: number): number => {
  if (!Number.isFinite(value)) {
    return min;
  }
  return Math.max(min, Math.min(max, Math.round(value)));
};

const handleStickyNoteEmbedResizePersist = (noteId: string, event: Event) => {
  const target = event.currentTarget as HTMLElement | null;
  if (!target) {
    return;
  }
  const width = clampStickyNoteEmbedSize(target.clientWidth, STICKY_NOTE_EMBED_RESIZE_MIN_WIDTH, STICKY_NOTE_EMBED_RESIZE_MAX_WIDTH);
  const height = clampStickyNoteEmbedSize(target.clientHeight, STICKY_NOTE_EMBED_RESIZE_MIN_HEIGHT, STICKY_NOTE_EMBED_RESIZE_MAX_HEIGHT);
  const current = stickyNoteStore.getEmbedLayoutState(noteId);
  if (current.width === width && current.height === height) {
    return;
  }
  const patch: StickyNoteEmbedLayoutState = { width, height };
  void stickyNoteStore.updateEmbedLayoutState(noteId, patch);
};

const resolveSingleIFormLinkFromContent = (content: string) => {
  let singleIFormLink = parseSingleIFormEmbedLinkText(content);
  if (!singleIFormLink && isTipTapJson(content)) {
    const plainText = tiptapJsonToPlainText(content);
    singleIFormLink = parseSingleIFormEmbedLinkText(plainText);
  }
  return singleIFormLink;
};

const resolveSingleStickyNoteLinkFromContent = (content: string) => {
  let singleStickyNoteLink = parseSingleStickyNoteEmbedLinkText(content);
  if (!singleStickyNoteLink && isTipTapJson(content)) {
    const plainText = tiptapJsonToPlainText(content);
    singleStickyNoteLink = parseSingleStickyNoteEmbedLinkText(plainText);
  }
  return singleStickyNoteLink;
};

const parseContent = (payload: any, overrideContent?: string) => {
  const content = overrideContent ?? payload?.content ?? '';

  const singleStickyNoteLink = resolveSingleStickyNoteLinkFromContent(content);
  if (singleStickyNoteLink) {
    const isCurrentChannel = chat.curChannel?.id === singleStickyNoteLink.channelId;
    if (isCurrentChannel) {
      const liveNote = stickyNoteStore.notes[singleStickyNoteLink.noteId];
      const noteType = (liveNote?.noteType || 'text') as StickyNoteType;
      const embedComponent = resolveStickyNoteEmbedComponent(noteType);
      const isInteractiveType = Boolean(embedComponent && liveNote);
      const title = String(liveNote?.title || '').trim() || '未命名便签';
      const fullText = resolveStickyNoteContentText(liveNote);
      const accentColor = resolveStickyNoteAccent(liveNote?.color || 'blue');
      const previewTitle = fullText || '点击打开便签';
      const noteId = singleStickyNoteLink.noteId;
      const embedState = stickyNoteStore.getEmbedLayoutState(noteId);
      const isCollapsed = embedState.collapsed === true;
      const widgetWidth = Number.isFinite(embedState.width)
        ? clampStickyNoteEmbedSize(embedState.width as number, STICKY_NOTE_EMBED_RESIZE_MIN_WIDTH, STICKY_NOTE_EMBED_RESIZE_MAX_WIDTH)
        : undefined;
      const widgetHeight = Number.isFinite(embedState.height)
        ? clampStickyNoteEmbedSize(embedState.height as number, STICKY_NOTE_EMBED_RESIZE_MIN_HEIGHT, STICKY_NOTE_EMBED_RESIZE_MAX_HEIGHT)
        : undefined;
      const openStickyNote = (event?: MouseEvent | KeyboardEvent) => {
        event?.preventDefault();
        event?.stopPropagation();
        void handleStickyNoteEmbedClick(singleStickyNoteLink);
      };
      return h(
        'details',
        {
          class: ['message-sticky-note-embed', isInteractiveType ? 'message-sticky-note-embed--interactive' : ''],
          open: !isCollapsed,
          title: previewTitle,
          'data-sticky-note-link': singleStickyNoteLink.rawLink,
          style: {
            '--sticky-note-accent': accentColor,
          } as Record<string, string>,
          onToggle: (event: Event) => {
            const target = event.currentTarget as HTMLDetailsElement | null;
            const collapsed = !(target?.open ?? true);
            const patch: StickyNoteEmbedLayoutState = { collapsed };
            void stickyNoteStore.updateEmbedLayoutState(noteId, patch);
          },
          onClick: (event: MouseEvent) => {
            event.stopPropagation();
          },
        },
        [
          h('summary', { class: 'message-sticky-note-embed__summary-row' }, [
            h('span', { class: 'message-sticky-note-embed__fold-icon' }, '▾'),
            h('span', { class: 'message-sticky-note-embed__body' }, [
              isInteractiveType
                ? h(
                  'button',
                  {
                    type: 'button',
                    class: 'message-sticky-note-embed__title-btn',
                    onClick: openStickyNote,
                  },
                  title,
                )
                : h('span', { class: 'message-sticky-note-embed__title' }, title),
            ]),
            h('span', { class: 'message-sticky-note-embed__side' }, [
              h(
                'button',
                {
                  type: 'button',
                  class: 'message-sticky-note-embed__copy-btn',
                  title: '打开便签',
                  onClick: openStickyNote,
                },
                '↗',
              ),
              h(
                'button',
                {
                  type: 'button',
                  class: 'message-sticky-note-embed__copy-btn',
                  title: '复制便签链接',
                  onClick: (event: MouseEvent) => {
                    event.preventDefault();
                    event.stopPropagation();
                    void copyStickyNoteEmbedLinkFromCard(singleStickyNoteLink.rawLink);
                  },
                },
                '⧉',
              ),
            ]),
          ]),
          h('div', { class: 'message-sticky-note-embed__panel' }, [
            h(
              'div',
              {
                class: 'message-sticky-note-embed__widget',
                style: {
                  width: widgetWidth ? `${widgetWidth}px` : undefined,
                  height: widgetHeight ? `${widgetHeight}px` : undefined,
                } as Record<string, string | undefined>,
                onClick: (event: MouseEvent) => event.stopPropagation(),
                onPointerdown: (event: PointerEvent) => event.stopPropagation(),
                onMouseup: (event: MouseEvent) => handleStickyNoteEmbedResizePersist(noteId, event),
                onPointerup: (event: PointerEvent) => handleStickyNoteEmbedResizePersist(noteId, event),
              },
              [
                isInteractiveType && liveNote && embedComponent
                  ? h(embedComponent, { note: liveNote as StickyNote, isEditing: false })
                  : h('span', { class: 'message-sticky-note-embed__content' }, fullText || '（空便签）'),
              ],
            ),
          ]),
        ],
      );
    }
  }

  const singleIFormLink = resolveSingleIFormLinkFromContent(content);
  if (singleIFormLink) {
    const targetChannelForms = singleIFormLink.channelId
      ? (iFormStore.formsByChannel[singleIFormLink.channelId] || [])
      : [];
    const matchedForm = targetChannelForms.find((item) => item.id === singleIFormLink.formId);
    const width = Math.max(
      MESSAGE_IFORM_MIN_WIDTH,
      Math.round(singleIFormLink.width || matchedForm?.defaultWidth || 640),
    );
    const height = Math.max(
      MESSAGE_IFORM_MIN_HEIGHT,
      Math.round(singleIFormLink.height || matchedForm?.defaultHeight || 360),
    );
    const runtimeForm: ChannelIForm = {
      id: singleIFormLink.formId,
      channelId: singleIFormLink.channelId,
      name: matchedForm?.name || '消息嵌入窗',
      url: matchedForm?.url,
      embedCode: matchedForm?.embedCode,
      defaultWidth: width,
      defaultHeight: height,
      defaultCollapsed: false,
      defaultFloating: false,
      allowPopout: false,
      orderIndex: 0,
      mediaOptions: matchedForm?.mediaOptions,
    };
    return h(
      'div',
      {
        class: 'message-iform-embed',
        'data-iform-link': singleIFormLink.rawLink,
        'data-message-id': payload?.id || '',
        style: {
          width: `${width}px`,
          height: `${height}px`,
          minWidth: `${MESSAGE_IFORM_MIN_WIDTH}px`,
          minHeight: `${MESSAGE_IFORM_MIN_HEIGHT}px`,
        },
      },
      [h(IFormEmbedFrame, { form: runtimeForm })],
    );
  }

  // 检测是否为 TipTap JSON 格式
  if (isTipTapJson(content)) {
    try {
      const html = tiptapJsonToHtml(content, {
        baseUrl: urlBase,
        imageClass: 'inline-image',
        linkClass: 'text-blue-500',
        attachmentResolver: resolveAttachmentUrl,
      });
      const sanitizedHtml = DOMPurify.sanitize(html);
      hasImage.value = html.includes('<img');
      return <span v-html={sanitizedHtml}></span>;
    } catch (error) {
      console.error('TipTap JSON 渲染失败:', error);
      // 降级处理：显示错误消息
      return <span class="text-red-500">内容格式错误</span>;
    }
  }

  // 使用原有的 Element.parse 逻辑
  const items = Element.parse(content);
  let textItems = []
  hasImage.value = false;

  for (const item of items) {
    switch (item.type) {
      case 'img':
        if (item.attrs.src) {
          const attachmentId = normalizeAttachmentId(item.attrs.src || '');
          if (attachmentId) {
            item.attrs['data-attachment-id'] = attachmentId;
          }
          item.attrs.src = resolveAttachmentUrl(item.attrs.src);
        }
        // 添加 lazy loading 优化性能
        item.attrs.loading = 'lazy';
        textItems.push(DOMPurify.sanitize(item.toString()));
        hasImage.value = true;
        break;
      case 'audio':
        let src = ''
        if (!item.attrs.src) break;

        src = item.attrs.src;
        src = resolveAttachmentUrl(item.attrs.src);

        let info = utils.sounds.get(src);

        if (!info) {
          const sound = new Howl({
            src: [src],
            html5: true
          });

          info = {
            sound,
            time: 0,
            playing: false
          }
          utils.sounds.set(src, info);
          utils.soundsTryInit()
        }

        const doPlay = () => {
          if (!info) return;
          if (info.playing) {
            info.sound.pause();
            info.playing = false;
          } else {
            info.sound.play();
            info.playing = true;
          }
        }

        textItems.push(<n-button rounded onClick={doPlay} type="primary">
          {info.playing ? `暂停 ${Math.floor(info.time)}/${Math.floor(info.sound.duration()) || '-'}` : '播放'}
        </n-button>)
        // textItems.push(DOMPurify.sanitize(item.toString()));
        // hasImage.value = true;
        break;
      case "at": {
        const atId = item.attrs.id;
        const atName = item.attrs.name || '';
        const isAll = atId === 'all';
        const isSelf = atId === user.info.id;
        let className = 'mention-capsule';
        if (isAll) {
          className += ' mention-capsule--all';
        } else if (isSelf) {
          className += ' mention-capsule--self';
        }
        // XSS 防护：使用 DOMPurify 转义名称
        const sanitizedName = DOMPurify.sanitize(atName, { ALLOWED_TAGS: [] });
        textItems.push(`<span class="${className}">@${sanitizedName}</span>`);
        break;
      }
      default: {
        const raw = item.toString();
        if (diceChipHtmlPattern.test(raw)) {
          textItems.push(raw);
        } else {
          textItems.push(`<span style="white-space: pre-wrap">${raw}</span>`);
        }
        break;
      }
    }
  }

  return <span>
    {textItems.map((item) => {
      if (typeof item === 'string') {
        return <span v-html={item}></span>
      } else {
        // vnode
        return item;
      }
    })}
  </span>
}


let messageIFormResizeObserver: ResizeObserver | null = null;
let messageIFormResizePersistTimer: ReturnType<typeof setTimeout> | null = null;
let messageIFormPersistInFlight = false;
let messageIFormQueuedSize: { width: number; height: number } | null = null;
let messageIFormLastSyncKey = '';
let messageIFormResizePointerActive = false;

const cleanupMessageIFormResizeSync = () => {
  if (messageIFormResizeObserver) {
    messageIFormResizeObserver.disconnect();
    messageIFormResizeObserver = null;
  }
  if (messageIFormResizePersistTimer) {
    clearTimeout(messageIFormResizePersistTimer);
    messageIFormResizePersistTimer = null;
  }
  messageIFormQueuedSize = null;
  messageIFormResizePointerActive = false;
};

const resolveMessageUpdateOptions = () => {
  const tone = props.tone;
  if (tone === 'ic' || tone === 'ooc') {
    return { icMode: tone as 'ic' | 'ooc' };
  }
  return undefined;
};

const persistMessageIFormSize = async (width: number, height: number) => {
  const item = props.item as any;
  const messageId = item?.id;
  const channelId = item?.channel?.id || item?.channelId || chat.curChannel?.id;
  if (!messageId || !channelId) {
    return;
  }
  if (!canEdit.value) {
    return;
  }

  const singleIFormLink = resolveSingleIFormLinkFromContent(displayContent.value || '');
  if (!singleIFormLink) {
    return;
  }

  const nextWidth = Math.max(MESSAGE_IFORM_MIN_WIDTH, Math.round(width));
  const nextHeight = Math.max(MESSAGE_IFORM_MIN_HEIGHT, Math.round(height));
  const currentLinkWidth = singleIFormLink.width ? Math.max(MESSAGE_IFORM_MIN_WIDTH, Math.round(singleIFormLink.width)) : undefined;
  const currentLinkHeight = singleIFormLink.height ? Math.max(MESSAGE_IFORM_MIN_HEIGHT, Math.round(singleIFormLink.height)) : undefined;
  if (typeof currentLinkWidth === 'number' && typeof currentLinkHeight === 'number') {
    if (Math.abs(nextWidth - currentLinkWidth) < 2 && Math.abs(nextHeight - currentLinkHeight) < 2) {
      return;
    }
  }

  const nextLink = updateIFormEmbedLinkSize(singleIFormLink.rawLink, nextWidth, nextHeight);
  if (!nextLink || nextLink === singleIFormLink.rawLink) {
    return;
  }

  const syncKey = `${messageId}:${nextLink}`;
  if (syncKey === messageIFormLastSyncKey) {
    return;
  }

  if (messageIFormPersistInFlight) {
    messageIFormQueuedSize = { width: nextWidth, height: nextHeight };
    return;
  }

  messageIFormPersistInFlight = true;
  try {
    const options = resolveMessageUpdateOptions();
    await chat.messageUpdate(channelId, messageId, nextLink, options);
    messageIFormLastSyncKey = syncKey;
  } catch (error) {
    console.error('同步消息 iForm 尺寸失败', error);
  } finally {
    messageIFormPersistInFlight = false;
    if (messageIFormQueuedSize) {
      const queuedSize = messageIFormQueuedSize;
      messageIFormQueuedSize = null;
      if (queuedSize.width !== nextWidth || queuedSize.height !== nextHeight) {
        void persistMessageIFormSize(queuedSize.width, queuedSize.height);
      }
    }
  }
};

const schedulePersistMessageIFormSize = (width: number, height: number) => {
  if (messageIFormResizePersistTimer) {
    clearTimeout(messageIFormResizePersistTimer);
  }
  messageIFormResizePersistTimer = setTimeout(() => {
    void persistMessageIFormSize(width, height);
  }, MESSAGE_IFORM_RESIZE_SYNC_DEBOUNCE);
};

const resetMessageIFormSyncBaseline = () => {
  const item = props.item as any;
  const messageId = item?.id || '';
  const singleIFormLink = resolveSingleIFormLinkFromContent(displayContent.value || '');
  messageIFormLastSyncKey = messageId && singleIFormLink
    ? `${messageId}:${singleIFormLink.rawLink}`
    : '';
};

const setupMessageIFormResizeSync = () => {
  cleanupMessageIFormResizeSync();
  const host = messageContentRef.value;
  if (!host || typeof ResizeObserver === 'undefined') {
    return;
  }
  const embedEl = host.querySelector<HTMLElement>('.message-iform-embed');
  if (!embedEl) {
    return;
  }
  messageIFormResizeObserver = new ResizeObserver((entries) => {
    if (!messageIFormResizePointerActive) {
      return;
    }
    const entry = entries[0];
    if (!entry) {
      return;
    }
    const nextWidth = Math.max(MESSAGE_IFORM_MIN_WIDTH, Math.round(entry.contentRect.width));
    const nextHeight = Math.max(MESSAGE_IFORM_MIN_HEIGHT, Math.round(entry.contentRect.height));
    schedulePersistMessageIFormSize(nextWidth, nextHeight);
  });
  messageIFormResizeObserver.observe(embedEl);
};


const handleMessageIFormPointerDown = (event: MouseEvent | PointerEvent) => {
  const target = event.target as HTMLElement | null;
  if (imageResizeMode.value) {
    const host = messageContentRef.value;
    const image = target?.closest<HTMLImageElement>('img');
    if (host && image && host.contains(image)) {
      if ('button' in event && typeof event.button === 'number' && event.button !== 0) {
        return;
      }
      const attachmentId = normalizeAttachmentId(image.dataset.attachmentId || image.getAttribute('data-attachment-id') || image.getAttribute('src') || '');
      if (attachmentId) {
        imageResizeSelectedAttachmentId.value = attachmentId;
        if ('pointerId' in event) {
          startImageResizePointer(event as PointerEvent, image, attachmentId);
        }
        if (event.cancelable) {
          event.preventDefault();
        }
        event.stopPropagation();
        return;
      }
    }
  }
  if (!canEdit.value) {
    return;
  }
  if (!target?.closest('.message-iform-embed')) {
    return;
  }
  messageIFormResizePointerActive = true;
};

const flushMessageIFormSizeFromDom = () => {
  const host = messageContentRef.value;
  if (!host) {
    return;
  }
  const embedEl = host.querySelector<HTMLElement>('.message-iform-embed');
  if (!embedEl) {
    return;
  }
  const nextWidth = Math.max(MESSAGE_IFORM_MIN_WIDTH, Math.round(embedEl.getBoundingClientRect().width));
  const nextHeight = Math.max(MESSAGE_IFORM_MIN_HEIGHT, Math.round(embedEl.getBoundingClientRect().height));
  schedulePersistMessageIFormSize(nextWidth, nextHeight);
};

const handleMessageIFormPointerUp = () => {
  handleImageResizePointerUp();
  if (!messageIFormResizePointerActive) {
    return;
  }
  flushMessageIFormSizeFromDom();
  messageIFormResizePointerActive = false;
};

const resetMessageIFormPointerState = () => {
  messageIFormResizePointerActive = false;
  clearImageResizePointerState();
};

const destroyImageViewer = () => {
  if (inlineImageViewer) {
    inlineImageViewer.destroy();
    inlineImageViewer = null;
  }
};

const setupImageViewer = async () => {
  await nextTick();
  if (imageResizeMode.value) {
    destroyImageViewer();
    return;
  }
  const host = messageContentRef.value;
  if (!host) {
    destroyImageViewer();
    return;
  }

  const inlineImages = host.querySelectorAll<HTMLImageElement>('img');
  if (!inlineImages.length) {
    destroyImageViewer();
    return;
  }

  // 总是重新创建viewer以确保选项正确（因为图片数量可能变化）
  destroyImageViewer();

  const hasMultiple = inlineImages.length > 1;
  inlineImageViewer = new Viewer(host, {
    className: 'chat-inline-image-viewer',
    navbar: hasMultiple,  // 多图时显示缩略图导航
    title: false,
    toolbar: {
      zoomIn: true,
      zoomOut: true,
      oneToOne: true,
      reset: true,
      prev: hasMultiple,  // 多图时显示上一张
      play: false,
      next: hasMultiple,  // 多图时显示下一张
      rotateLeft: true,
      rotateRight: true,
      flipHorizontal: false,
      flipVertical: false,
    },
    tooltip: true,
    movable: true,
    zoomable: true,
    scalable: true,
    rotatable: true,
    transition: true,
    fullscreen: true,
    keyboard: true,  // 启用键盘导航 (←/→)
    zIndex: 2500,
  });
};

const ensureImageViewer = () => {
  void setupImageViewer();
};

const handleContentDblclick = async (event: MouseEvent) => {
  if (imageResizeMode.value) {
    event.preventDefault();
    return;
  }
  const host = messageContentRef.value;
  if (!host) return;
  const target = event.target as HTMLElement | null;
  if (!target) return;
  const image = target.closest<HTMLImageElement>('img');
  if (!image || !host.contains(image)) {
    return;
  }

  event.preventDefault();
  await setupImageViewer();
  if (!inlineImageViewer) {
    return;
  }
  const imageList = Array.from(host.querySelectorAll<HTMLImageElement>('img'));
  const imageIndex = imageList.indexOf(image);
  inlineImageViewer.view(imageIndex >= 0 ? imageIndex : 0);
};

const handleContentClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null;
  if (!target) return;
  const host = messageContentRef.value;
  if (imageResizeMode.value && host) {
    const image = target.closest<HTMLImageElement>('img');
    if (image && host.contains(image)) {
      const attachmentId = normalizeAttachmentId(image.dataset.attachmentId || image.getAttribute('data-attachment-id') || image.getAttribute('src') || '');
      if (attachmentId) {
        imageResizeSelectedAttachmentId.value = attachmentId;
        event.preventDefault();
        event.stopPropagation();
        void applyImageLayoutToDom();
      }
      return;
    }
  }
  if (target.closest('a')) return;
  const spoiler = target.closest('.tiptap-spoiler') as HTMLElement | null;
  if (!spoiler) return;
  spoiler.classList.toggle('is-revealed');
};

const props = defineProps({
  username: String,
  content: String,
  avatar: String,
  isRtl: Boolean,
  item: Object,
  identityColor: String,
  editingPreview: Object as PropType<EditingPreviewInfo | undefined>,
  tone: {
    type: String as PropType<'ic' | 'ooc' | 'archived'>,
    default: 'ic'
  },
  showAvatar: {
    type: Boolean,
    default: true,
  },
  hideAvatar: {
    type: Boolean,
    default: false,
  },
  showHeader: {
    type: Boolean,
    default: true,
  },
  layout: {
    type: String as PropType<'bubble' | 'compact'>,
    default: 'bubble',
  },
  isSelf: {
    type: Boolean,
    default: false,
  },
  isMerged: {
    type: Boolean,
    default: false,
  },
  bodyOnly: {
    type: Boolean,
    default: false,
  },
  worldKeywordEditable: {
    type: Boolean,
    default: false,
  },
  isMultiSelectMode: {
    type: Boolean,
    default: false,
  },
  isSelected: {
    type: Boolean,
    default: false,
  },
  allMessageIds: {
    type: Array as () => string[],
    default: () => [],
  },
})

const emit = defineEmits(['avatar-longpress', 'avatar-click', 'edit', 'edit-save', 'edit-cancel', 'toggle-select', 'range-click', 'image-layout-edit-state-change']);

const timestampTicker = ref(Date.now());
const inlineTimestampText = computed(() => {
  timestampTicker.value;
  return formatTimestampByPreference(props.item?.createdAt, displayStore.settings.timestampFormat);
});
const tooltipTimestampText = computed(() => {
  timestampTicker.value;
  return timeFormat2(props.item?.createdAt);
});
const editedTimeText2 = computed(() => (props.item?.isEdited ? timeFormat2(props.item?.updatedAt) : ''));

const getMemberDisplayName = (item: any) => item?.whisperMeta?.senderMemberName
  || item?.identity?.displayName
  || item?.sender_identity_name
  || item?.sender_member_name
  || resolveChannelIdentityDisplayName(item?.sender_identity_id || item?.senderIdentityId)
  || item?.member?.nick
  || item?.user?.nick
  || item?.user?.name
  || resolveChannelUserDisplayName(item?.user?.id || item?.user_id || item?.userId)
  || item?.whisperMeta?.senderUserNick
  || item?.whisperMeta?.senderUserName
  || '未知成员';
const getTargetDisplayName = (item: any) => item?.whisperMeta?.targetMemberName
  || item?.whisperTo?.nick
  || item?.whisperTo?.name
  || item?.whisperMeta?.targetUserNick
  || item?.whisperMeta?.targetUserName
  || '未知成员';

const channelUserNameMap = computed(() => {
  const map = new Map<string, string>();
  (chat.curChannelUsers || []).forEach((user: any) => {
    const name = user?.nick || user?.nickname || user?.name || user?.username || '';
    if (user?.id && name) {
      map.set(String(user.id), name);
    }
  });
  return map;
});

const resolveChannelUserDisplayName = (userId?: string) => {
  if (!userId) return '';
  return channelUserNameMap.value.get(String(userId)) || '';
};

const channelIdentityMap = computed(() => {
  const map = new Map<string, { name: string; color: string }>();
  const list = chat.channelIdentities[chat.curChannel?.id || ''] || [];
  list.forEach((identity) => {
    if (!identity?.id) return;
    map.set(identity.id, {
      name: identity.displayName || '',
      color: identity.color || '',
    });
  });
  return map;
});

const resolveChannelIdentityDisplayName = (identityId?: string) => {
  if (!identityId) return '';
  return channelIdentityMap.value.get(String(identityId))?.name || '';
};

const resolveChannelIdentityColor = (identityId?: string) => {
  if (!identityId) return '';
  return channelIdentityMap.value.get(String(identityId))?.color || '';
};

const resolveWhisperTargets = (item: any) => {
  const list = item?.whisperToIds || item?.whisper_to_ids || item?.whisperTargets || item?.whisper_targets;
  if (Array.isArray(list) && list.length > 0) {
    return list.map((entry: any) => {
      if (typeof entry === 'string') {
        const name = resolveChannelUserDisplayName(entry) || entry;
        return { id: entry, name };
      }
      const id = entry?.id || '';
      const name = entry?.nick || entry?.name || resolveChannelUserDisplayName(id) || entry?.username || id || '未知成员';
      return { id, name };
    });
  }
  const metaIds = item?.whisperMeta?.targetUserIds;
  if (Array.isArray(metaIds) && metaIds.length > 0) {
    return metaIds.map((id: string) => ({
      id,
      name: resolveChannelUserDisplayName(id) || id || '未知成员',
    }));
  }
  return [];
};

const quoteInlineImageTokenPattern = /\[\[(?:图片:[^\]]+|img:[^\]]+)\]\]/gi;

const buildQuoteSummary = (quote?: any) => {
  if (!quote) return '';
  const meta = quote as any;
  if (meta?.is_deleted || meta?.isDeleted) {
    return '此消息已删除';
  }
  if (meta?.is_revoked || meta?.isRevoked) {
    return '此消息已撤回';
  }
  const content = quote?.content ?? '';
  if (typeof content !== 'string' || content.trim() === '') {
    return '[图片]';
  }
  if (isTipTapJson(content)) {
    try {
      const json = JSON.parse(content);
      const text = tiptapJsonToPlainText(json).trim();
      return text || '[图片]';
    } catch (error) {
      console.warn('TipTap JSON 文本解析失败', error);
      return '[图片]';
    }
  }
  const items = Element.parse(content);
  let text = '';
  let fallback = '';
  items.forEach((item) => {
    if (item.type === 'text') {
      text += item.toString();
      return;
    }
    if (item.type === 'at') {
      const name = item.attrs?.name;
      text += name ? `@${name}` : item.toString();
      return;
    }
    if (!fallback) {
      if (item.type === 'img') fallback = '[图片]';
      if (item.type === 'audio') fallback = '[语音]';
      if (item.type === 'file') fallback = '[附件]';
    }
  });
  const normalized = text.replace(quoteInlineImageTokenPattern, '[图片]').trim();
  if (normalized) return normalized;
  const replaced = content.replace(quoteInlineImageTokenPattern, '[图片]').trim();
  if (replaced && replaced !== content) return replaced;
  return fallback || '[图片]';
};

const buildWhisperLabel = (item?: any) => {
  if (!item?.isWhisper) return '';
  const senderName = getMemberDisplayName(item);
  const senderUserId = item?.user?.id || item?.whisperMeta?.senderUserId;
  const senderLabel = `@${senderName}`;
  const targets = resolveWhisperTargets(item);
  const targetNames = targets.map((target: any) => target?.name).filter(Boolean);
  if (targetNames.length > 0) {
    if (senderUserId === user.info.id) {
      return t('whisper.sentTo', { targets: targetNames.join('、') });
    }
    const otherRecipients = targets.filter((target: any) => {
      const targetId = target?.id;
      if (!targetId) return false;
      return targetId !== user.info.id && targetId !== senderUserId;
    });
    if (otherRecipients.length > 0) {
      const otherNames = otherRecipients.map((target: any) => target?.name).filter(Boolean).join('、');
      return t('whisper.fromMultiple', { sender: senderLabel, otherUsers: otherNames });
    }
    return t('whisper.from', { sender: senderLabel });
  }

  const targetName = getTargetDisplayName(item);
  const targetLabel = `@${targetName}`;
  const targetUserId = item?.whisperTo?.id || item?.whisperMeta?.targetUserId;
  if (senderUserId === user.info.id) {
    return t('whisper.sentTo', { targets: targetLabel });
  }
  if (targetUserId === user.info.id) {
    return t('whisper.from', { sender: senderLabel });
  }
  if (targetName && targetName !== '未知成员') {
    return t('whisper.sentTo', { targets: targetLabel });
  }
  return t('whisper.generic');
};

const whisperLabel = computed(() => buildWhisperLabel(props.item));
const quoteItem = computed(() => props.item?.quote ?? null);
const quoteDisplayName = computed(() => (quoteItem.value ? getMemberDisplayName(quoteItem.value) : ''));
const quoteNameColor = computed(() => quoteItem.value?.identity?.color
  || (quoteItem.value as any)?.sender_identity_color
  || resolveChannelIdentityColor((quoteItem.value as any)?.sender_identity_id || (quoteItem.value as any)?.senderIdentityId)
  || '');
const quoteIsDeleted = computed(() => Boolean((quoteItem.value as any)?.is_deleted || (quoteItem.value as any)?.isDeleted));
const quoteIsRevoked = computed(() => Boolean((quoteItem.value as any)?.is_revoked || (quoteItem.value as any)?.isRevoked));
const quoteSummary = computed(() => buildQuoteSummary(quoteItem.value));
const quoteJumpEnabled = computed(() => Boolean(quoteItem.value?.id));

const selfEditingPreview = computed(() => (
  props.editingPreview && props.editingPreview.isSelf ? props.editingPreview : null
));
const otherEditingPreview = computed(() => (
  props.editingPreview && !props.editingPreview.isSelf ? props.editingPreview : null
));

const contentClassList = computed(() => {
  const classes: Record<string, boolean> = {
    'whisper-content': Boolean(props.item?.isWhisper),
    'content--editing-preview': Boolean(otherEditingPreview.value),
  };
  if (otherEditingPreview.value && props.layout === 'bubble') {
    classes['content--editing-preview--bubble'] = true;
  }
  return classes;
});

const isEditing = computed(() => chat.isEditingMessage(props.item?.id));
const resolveMessageUserId = (item: any) => (
  item?.user?.id
  || item?.user_id
  || item?.member?.user?.id
  || item?.member?.userId
  || item?.member?.user_id
  || ''
);
const targetUserId = computed(() => resolveMessageUserId(props.item));
const canEdit = computed(() => {
  // 自己的消息可编辑
  if (targetUserId.value && targetUserId.value === user.info.id) return true;
  if (!targetUserId.value) return false;
  // 检查世界管理员编辑权限
  const worldId = chat.currentWorldId;
  const worldDetail = chat.worldDetailMap[worldId];
  const allowAdminEdit = worldDetail?.allowAdminEditMessages
    || worldDetail?.world?.allowAdminEditMessages
    || chat.worldMap[worldId]?.allowAdminEditMessages;
  if (allowAdminEdit) {
    const memberRole = worldDetail?.memberRole;
    const ownerId = worldDetail?.world?.ownerId || chat.worldMap[worldId]?.ownerId;
    const isWorldAdmin = memberRole === 'owner' || memberRole === 'admin' || ownerId === user.info.id;
    if (isWorldAdmin) {
      const channelId = chat.curChannel?.id;
      if (channelId && targetUserId.value && chat.isChannelAdmin(channelId, targetUserId.value)) {
        return false;
      }
      return true; // 后端会进一步验证目标消息作者是否为非管理员
    }
  }
  return false;
});

// Multi-select computed properties (merged from props and store)
const effectiveMultiSelectMode = computed(() => props.isMultiSelectMode || chat.multiSelect?.active || false);
const effectiveIsSelected = computed(() => {
  if (props.isMultiSelectMode) return props.isSelected;
  if (chat.multiSelect?.active && props.item?.id) {
    return chat.multiSelect.selectedIds.has(props.item.id);
  }
  return false;
});

const hoverTimestampVisible = ref(false);
let hoverTimer: ReturnType<typeof setTimeout> | null = null;
let timestampInterval: ReturnType<typeof setInterval> | null = null;

const shouldForceTimestampVisible = computed(() => displayStore.settings.alwaysShowTimestamp);
const timestampShouldRender = computed(() => {
  if (!props.showHeader || props.bodyOnly) {
    return false;
  }
  if (!props.item?.createdAt) {
    return false;
  }
  return shouldForceTimestampVisible.value || hoverTimestampVisible.value;
});

const clearHoverTimer = () => {
  if (hoverTimer) {
    clearTimeout(hoverTimer);
    hoverTimer = null;
  }
};

const handleTimestampHoverStart = () => {
  if (shouldForceTimestampVisible.value || isMobileUa) {
    return;
  }
  clearHoverTimer();
  hoverTimer = setTimeout(() => {
    hoverTimestampVisible.value = true;
  }, TIMESTAMP_HOVER_DELAY);
};

const handleTimestampHoverEnd = () => {
  if (shouldForceTimestampVisible.value || isMobileUa) {
    return;
  }
  clearHoverTimer();
  hoverTimestampVisible.value = false;
};

const handleMobileTimestampTap = (e: MouseEvent) => {
  // In multi-select mode, clicking anywhere on the message toggles selection
  if (effectiveMultiSelectMode.value) {
    handleMessageClick(e);
    return;
  }
  
  if (!isMobileUa || shouldForceTimestampVisible.value) {
    return;
  }
  // Ignore if target is an interactive element
  const target = e.target as HTMLElement;
  if (target.closest('a, button, img, .message-action-bar')) {
    return;
  }
  e.stopPropagation(); // Prevent global click handler from immediately hiding
  hoverTimestampVisible.value = !hoverTimestampVisible.value;
};

const chatItemRef = ref<HTMLElement | null>(null);

const handleGlobalClickForTimestamp = (e: MouseEvent) => {
  if (!isMobileUa || shouldForceTimestampVisible.value || !hoverTimestampVisible.value) {
    return;
  }
  const target = e.target as HTMLElement;
  // If click is outside this chat item, hide timestamp
  if (chatItemRef.value && !chatItemRef.value.contains(target)) {
    hoverTimestampVisible.value = false;
  }
};

watch(shouldForceTimestampVisible, (value) => {
  if (value) {
    clearHoverTimer();
  }
  hoverTimestampVisible.value = false;
});

const inlineImageTokenPattern = /\[\[(?:图片:[^\]]+|img:[^\]]+)\]\]/gi;

const displayContent = computed(() => {
  if (isEditing.value && chat.editing) {
    const draft = chat.editing.draft || '';
    if (isTipTapJson(draft)) {
      return draft;
    }
    return draft.replace(inlineImageTokenPattern, '[图片]');
  }
  return props.item?.content ?? props.content ?? '';
});

const resolveMessageChannelId = () => {
  const raw = props.item as any;
  return String(raw?.channel?.id || raw?.channelId || chat.curChannel?.id || '').trim();
};

const currentMessageId = computed(() => String((props.item as any)?.id || '').trim());
const currentMessageChannelId = computed(() => resolveMessageChannelId());

const imageAttachmentIDPattern = /id:([a-zA-Z0-9_-]+)/g;
const imageTagIDPattern = /<(?:img|image)[^>]+src=["']id:([a-zA-Z0-9_-]+)["'][^>]*>/gi;

const extractImageAttachmentIds = (content: string): string[] => {
  if (!content) {
    return [];
  }
  imageTagIDPattern.lastIndex = 0;
  imageAttachmentIDPattern.lastIndex = 0;
  const ids: string[] = [];
  const seen = new Set<string>();
  let match: RegExpExecArray | null;
  while ((match = imageTagIDPattern.exec(content)) !== null) {
    const id = normalizeAttachmentId(match[1] || '');
    if (!id || seen.has(id)) {
      continue;
    }
    seen.add(id);
    ids.push(id);
  }
  if (ids.length > 0) {
    return ids;
  }
  while ((match = imageAttachmentIDPattern.exec(content)) !== null) {
    const id = normalizeAttachmentId(match[1] || '');
    if (!id || seen.has(id)) {
      continue;
    }
    seen.add(id);
    ids.push(id);
  }
  return ids;
};

const messageImageAttachmentIds = computed(() => extractImageAttachmentIds(displayContent.value || ''));
const imageResizeHasChanges = computed(() => Object.keys(imageResizeDraftLayouts.value).length > 0);

const clampImageLayoutSize = (value: number) => {
  if (!Number.isFinite(value)) {
    return IMAGE_LAYOUT_MIN_SIZE;
  }
  return Math.max(IMAGE_LAYOUT_MIN_SIZE, Math.min(IMAGE_LAYOUT_MAX_SIZE, Math.round(value)));
};

const resolveStoredImageLayout = (attachmentId: string) => {
  const channelId = currentMessageChannelId.value;
  if (!channelId || !attachmentId) {
    return null;
  }
  return channelImageLayout.getLayout(channelId, attachmentId);
};

const resolveImageLayoutByAttachmentId = (attachmentId: string) => {
  if (!attachmentId) {
    return null;
  }
  const draft = imageResizeDraftLayouts.value[attachmentId];
  if (draft) {
    return {
      width: clampImageLayoutSize(draft.width),
      height: clampImageLayoutSize(draft.height),
    };
  }
  const stored = resolveStoredImageLayout(attachmentId);
  if (stored) {
    return {
      width: clampImageLayoutSize(stored.width),
      height: clampImageLayoutSize(stored.height),
    };
  }
  return null;
};

const resolveImageAttachmentIdFromElement = (
  image: HTMLImageElement,
  fallbackIds: string[],
  index: number,
): string => {
  const datasetId = normalizeAttachmentId(image.dataset.attachmentId || '');
  if (datasetId) {
    return datasetId;
  }
  const attrId = normalizeAttachmentId(image.getAttribute('data-attachment-id') || '');
  if (attrId) {
    image.dataset.attachmentId = attrId;
    return attrId;
  }
  const srcId = normalizeAttachmentId(image.getAttribute('src') || '');
  if (srcId) {
    image.dataset.attachmentId = srcId;
    return srcId;
  }
  if (index < fallbackIds.length) {
    const fallback = normalizeAttachmentId(fallbackIds[index]);
    if (fallback) {
      image.dataset.attachmentId = fallback;
      return fallback;
    }
  }
  return '';
};

const applyImageLayoutToDom = async () => {
  await nextTick();
  const host = messageContentRef.value;
  if (!host) {
    return;
  }
  const images = Array.from(host.querySelectorAll<HTMLImageElement>('img'));
  const fallbackIds = messageImageAttachmentIds.value;
  images.forEach((image, index) => {
    const attachmentId = resolveImageAttachmentIdFromElement(image, fallbackIds, index);
    const layout = resolveImageLayoutByAttachmentId(attachmentId);
    const unlock = Boolean(layout);

    image.classList.toggle('message-image-adjustable', imageResizeMode.value && !!attachmentId);
    image.classList.toggle('message-image-selected', imageResizeMode.value && !!attachmentId && attachmentId === imageResizeSelectedAttachmentId.value);
    image.classList.toggle('message-image-unlocked', unlock && !!attachmentId);
    image.draggable = false;

    if (layout && attachmentId) {
      image.style.width = String(layout.width) + 'px';
      image.style.height = String(layout.height) + 'px';
      image.style.maxWidth = 'none';
      image.style.maxHeight = 'none';
      image.style.touchAction = imageResizeMode.value && attachmentId === imageResizeSelectedAttachmentId.value ? 'none' : '';
      return;
    }

    image.style.width = '';
    image.style.height = '';
    image.style.maxWidth = '';
    image.style.maxHeight = '';
    image.style.touchAction = '';
  });
};

const ensureMessageImageLayoutsLoaded = async () => {
  const channelId = currentMessageChannelId.value;
  if (!channelId) {
    return;
  }
  const attachmentIds = messageImageAttachmentIds.value;
  if (!attachmentIds.length) {
    return;
  }
  await channelImageLayout.ensureLayouts(channelId, attachmentIds);
};

const clearImageResizePointerState = () => {
  imageResizePointerState = null;
};

const emitImageResizeState = (active: boolean) => {
  const messageId = currentMessageId.value;
  if (!messageId) {
    return;
  }
  emit('image-layout-edit-state-change', { messageId, active });
};

const stopImageResizeMode = async (emitState = true, restoreViewer = true) => {
  imageResizeMode.value = false;
  imageResizeFreeScaling.value = false;
  imageResizeSelectedAttachmentId.value = '';
  imageResizeDraftLayouts.value = {};
  clearImageResizePointerState();
  if (emitState) {
    emitImageResizeState(false);
  }
  await applyImageLayoutToDom();
  if (restoreViewer) {
    ensureImageViewer();
  }
};

const enterImageResizeMode = async () => {
  if (!canEdit.value || !hasImage.value) {
    return;
  }
  const attachmentIds = messageImageAttachmentIds.value;
  if (!attachmentIds.length) {
    return;
  }
  if (!imageResizeSelectedAttachmentId.value || !attachmentIds.includes(imageResizeSelectedAttachmentId.value)) {
    imageResizeSelectedAttachmentId.value = attachmentIds[0] || '';
  }
  imageResizeMode.value = true;
  imageResizeFreeScaling.value = false;
  destroyImageViewer();
  emitImageResizeState(true);
  await ensureMessageImageLayoutsLoaded();
  await applyImageLayoutToDom();
};

const toggleImageResizeScaleMode = () => {
  imageResizeFreeScaling.value = !imageResizeFreeScaling.value;
};

const cancelImageResize = () => {
  void stopImageResizeMode(true, true);
};

const saveImageResizedLayout = async () => {
  if (!imageResizeMode.value) {
    return;
  }
  const channelId = currentMessageChannelId.value;
  const messageId = currentMessageId.value;
  if (!channelId || !messageId) {
    return;
  }
  const attachmentSet = new Set(messageImageAttachmentIds.value);
  const payload = Object.entries(imageResizeDraftLayouts.value)
    .filter(([attachmentId]) => attachmentSet.has(attachmentId))
    .map(([attachmentId, layout]) => ({
      attachmentId,
      width: clampImageLayoutSize(layout.width),
      height: clampImageLayoutSize(layout.height),
    }));

  if (payload.length === 0) {
    await stopImageResizeMode(true, true);
    return;
  }

  try {
    await channelImageLayout.saveMessageLayouts(channelId, messageId, payload);
    message.success('图片尺寸已保存');
    await stopImageResizeMode(true, true);
  } catch (error: any) {
    const errMsg = error?.response?.data?.message || error?.message || '保存图片尺寸失败';
    message.error(errMsg);
  }
};

const handleImageResizeEnterRequest = (payload?: any) => {
  const targetMessageId = String(payload?.messageId || payload?.message_id || '').trim();
  if (!targetMessageId) {
    return;
  }
  if (targetMessageId !== currentMessageId.value) {
    if (imageResizeMode.value) {
      void stopImageResizeMode(true, true);
    }
    return;
  }
  void enterImageResizeMode();
};

const startImageResizePointer = (event: PointerEvent, image: HTMLImageElement, attachmentId: string) => {
  const rect = image.getBoundingClientRect();
  const pointerType = event.pointerType || 'mouse';
  const isTouchPointer = pointerType === 'touch';
  imageResizePointerState = {
    pointerId: event.pointerId,
    pointerType,
    moveThreshold: isTouchPointer ? 4 : 1.5,
    movementScale: isTouchPointer ? 0.82 : 1,
    attachmentId,
    startX: event.clientX,
    startY: event.clientY,
    startWidth: rect.width,
    startHeight: rect.height,
    aspectRatio: rect.height > 0 ? rect.width / rect.height : 1,
    moved: false,
  };
};

const handleImageResizePointerMove = (event: PointerEvent) => {
  if (!imageResizeMode.value || !imageResizePointerState) {
    return;
  }
  if (event.pointerId !== imageResizePointerState.pointerId) {
    return;
  }
  const rawDx = event.clientX - imageResizePointerState.startX;
  const rawDy = event.clientY - imageResizePointerState.startY;
  const threshold = imageResizePointerState.moveThreshold;
  if (!imageResizePointerState.moved && Math.abs(rawDx) < threshold && Math.abs(rawDy) < threshold) {
    return;
  }
  imageResizePointerState.moved = true;

  const scale = imageResizePointerState.movementScale;
  const dx = rawDx * scale;
  const dy = rawDy * scale;

  const nextWidthByPointer = clampImageLayoutSize(imageResizePointerState.startWidth + dx);
  const nextHeightByPointer = clampImageLayoutSize(imageResizePointerState.startHeight + dy);
  let nextWidth = nextWidthByPointer;
  let nextHeight = nextHeightByPointer;

  if (!imageResizeFreeScaling.value) {
    const aspectRatio = imageResizePointerState.aspectRatio > 0 ? imageResizePointerState.aspectRatio : 1;
    if (Math.abs(dx) >= Math.abs(dy)) {
      nextWidth = nextWidthByPointer;
      nextHeight = clampImageLayoutSize(nextWidth / aspectRatio);
    } else {
      nextHeight = nextHeightByPointer;
      nextWidth = clampImageLayoutSize(nextHeight * aspectRatio);
    }
  }

  const attachmentId = imageResizePointerState.attachmentId;
  imageResizeDraftLayouts.value = {
    ...imageResizeDraftLayouts.value,
    [attachmentId]: {
      width: nextWidth,
      height: nextHeight,
    },
  };
  if (event.cancelable) {
    event.preventDefault();
  }
  void applyImageLayoutToDom();
};

const handleImageResizePointerUp = () => {
  clearImageResizePointerState();
};

const imageResizeLayoutSignature = computed(() => {
  const channelId = currentMessageChannelId.value;
  const parts = messageImageAttachmentIds.value.map((attachmentId) => {
    const draft = imageResizeDraftLayouts.value[attachmentId];
    if (draft) {
      return attachmentId + ':' + draft.width + 'x' + draft.height + ':d';
    }
    const stored = channelImageLayout.getLayout(channelId, attachmentId);
    if (!stored) {
      return attachmentId + ':none';
    }
    return attachmentId + ':' + stored.width + 'x' + stored.height + ':' + String(stored.updatedAt || 0);
  });
  return [
    imageResizeMode.value ? '1' : '0',
    imageResizeSelectedAttachmentId.value,
    parts.join('|'),
  ].join('||');
});

watch([currentMessageChannelId, messageImageAttachmentIds], () => {
  void ensureMessageImageLayoutsLoaded();
}, { immediate: true });

watch(imageResizeLayoutSignature, () => {
  void applyImageLayoutToDom();
}, { immediate: true });

watch(messageImageAttachmentIds, (ids) => {
  if (!ids.length && imageResizeMode.value) {
    void stopImageResizeMode(true, true);
    return;
  }
  if (imageResizeSelectedAttachmentId.value && !ids.includes(imageResizeSelectedAttachmentId.value)) {
    imageResizeSelectedAttachmentId.value = ids[0] || '';
  }
});

const compiledKeywords = computed(() => {
  const worldId = chat.currentWorldId
  if (!worldId) {
    return []
  }
  return worldGlossary.compiledMap[worldId] || []
})

const keywordHighlightEnabled = computed(() => displayStore.settings.worldKeywordHighlightEnabled !== false)
const keywordUnderlineOnly = computed(() => !!displayStore.settings.worldKeywordUnderlineOnly)
const keywordTooltipEnabled = computed(() => displayStore.settings.worldKeywordTooltipEnabled !== false)
const keywordDeduplicateEnabled = computed(() => !!displayStore.settings.worldKeywordDeduplicateEnabled)

const keywordTooltipResolver = (keywordId: string) => {
  const keyword = worldGlossary.keywordById[keywordId]
  if (!keyword) {
    return null
  }
  return {
    title: keyword.keyword,
    description: keyword.description,
    descriptionFormat: keyword.descriptionFormat,
  }
}

const handleKeywordQuickEdit = (keywordId: string) => {
  if (!props.worldKeywordEditable) {
    return
  }
  const worldId = chat.currentWorldId
  if (!worldId) {
    return
  }
  const keyword = worldGlossary.keywordById[keywordId]
  if (!keyword) {
    return
  }
  worldGlossary.openEditor(worldId, keyword)
}

let keywordTooltipInstance = createKeywordTooltip(keywordTooltipResolver, {
  level: 0,
  compiledKeywords: compiledKeywords.value,
  onKeywordDoubleInvoke: props.worldKeywordEditable ? handleKeywordQuickEdit : undefined,
  underlineOnly: keywordUnderlineOnly.value,
  textIndent: displayStore.settings.worldKeywordTooltipTextIndent,
})

// Lazy rendering state
let isVisible = false
let keywordObserver: IntersectionObserver | null = null
let pendingHighlights = false

const applyKeywordHighlights = async () => {
  await nextTick()
  const host = messageContentRef.value
  if (!host) {
    return
  }
  
  // If not visible yet, mark as pending and skip
  if (!isVisible) {
    pendingHighlights = true
    return
  }
  
  pendingHighlights = false
  const compiled = compiledKeywords.value
  if (!keywordHighlightEnabled.value || !compiled.length) {
    refreshWorldKeywordHighlights(host, [], { underlineOnly: false })
    return
  }
  refreshWorldKeywordHighlights(
    host,
    compiled,
    {
      underlineOnly: keywordUnderlineOnly.value,
      deduplicate: keywordDeduplicateEnabled.value,
      onKeywordDoubleInvoke: props.worldKeywordEditable ? handleKeywordQuickEdit : undefined,
    },
    keywordTooltipEnabled.value ? keywordTooltipInstance : undefined,
  )
}

// Setup IntersectionObserver for lazy rendering
const setupVisibilityObserver = () => {
  const host = messageContentRef.value
  if (!host || keywordObserver) return
  
  keywordObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      const wasVisible = isVisible
      isVisible = entry.isIntersecting
      
      // Apply highlights when becoming visible with pending updates
      if (isVisible && !wasVisible && pendingHighlights) {
        void applyKeywordHighlights()
      }
    })
  }, {
    rootMargin: '100px', // Pre-load 100px before visible
    threshold: 0
  })
  
  keywordObserver.observe(host)
}

const applyDiceTone = () => {
  nextTick(() => {
    const host = messageContentRef.value;
    if (!host) return;
    const tone = (props.tone || 'ic') as 'ic' | 'ooc' | 'archived';
    host.querySelectorAll<HTMLElement>('span.dice-chip').forEach((chip) => {
      chip.setAttribute('data-dice-tone', tone);
      chip.classList.remove('dice-chip--tone-ic', 'dice-chip--tone-ooc', 'dice-chip--tone-archived');
      chip.classList.add(`dice-chip--tone-${tone}`);
    });
  });
};

// 处理消息链接渲染
const processMessageLinks = () => {
  nextTick(() => {
    const host = messageContentRef.value;
    if (!host) return;

    // 1. 处理已标记的 pending 链接（来自 tiptap-render）
    const pendingLinks = host.querySelectorAll<HTMLAnchorElement>('.message-jump-link-pending');
    pendingLinks.forEach((link) => {
      let worldId = link.dataset.worldId || '';
      let channelId = link.dataset.channelId || '';
      let messageId = link.dataset.messageId || '';
      const url = link.href;

      if (!worldId || !channelId || !messageId) {
        const parsed = parseMessageLink(url);
        if (parsed) {
          worldId = parsed.worldId;
          channelId = parsed.channelId;
          messageId = parsed.messageId;
        }
      }

      if (!worldId || !channelId || !messageId) return;

      const info = resolveMessageLinkInfo(url, {
        currentWorldId: chat.currentWorldId,
        worldMap: chat.worldMap,
        findChannelById: (id) => chat.findChannelById(id),
      });

      if (!info) {
        link.classList.remove('message-jump-link-pending');
        return;
      }

      // 创建新的链接元素
      const wrapper = document.createElement('span');
      wrapper.innerHTML = renderMessageLinkHtml(info);
      const newLink = wrapper.firstElementChild as HTMLAnchorElement;
      if (!newLink) return;

      // 绑定点击事件（内联跳转，不开新标签页）
      newLink.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        handleMessageLinkClick(info);
      });

      link.replaceWith(newLink);
    });

    // 2. 处理纯文本中的消息链接 URL
    processPlainTextMessageLinks(host);
  });
};

// 处理纯文本中的消息链接
// 支持两种格式:
// 1. [自定义标题](http://.../#/worldId/channelId?msg=messageId)
// 2. http://.../#/worldId/channelId?msg=messageId
const processPlainTextMessageLinks = (host: HTMLElement) => {
  const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT, null);
  const nodesToProcess: { node: Text; segments: Array<{ type: 'text' | 'titled' | 'plain'; content: string; title?: string; url?: string; index: number; length: number }> }[] = [];

  // 收集需要处理的文本节点
  let textNode: Text | null;
  while ((textNode = walker.nextNode() as Text | null)) {
    // 跳过已处理的链接内部的文本
    const parent = textNode.parentElement;
    if (parent?.closest('.message-jump-link, a')) continue;

    const text = textNode.textContent || '';
    const segments: Array<{ type: 'text' | 'titled' | 'plain'; content: string; title?: string; url?: string; index: number; length: number }> = [];

    // 先匹配带标题的链接 [title](url)
    TITLED_MESSAGE_LINK_REGEX.lastIndex = 0;
    let titledMatch: RegExpExecArray | null;
    const titledMatches: { index: number; length: number; title: string; url: string }[] = [];
    while ((titledMatch = TITLED_MESSAGE_LINK_REGEX.exec(text)) !== null) {
      titledMatches.push({
        index: titledMatch.index,
        length: titledMatch[0].length,
        title: titledMatch[1],
        url: titledMatch[2],
      });
    }

    // 再匹配普通链接，但排除已被带标题链接覆盖的部分
    MESSAGE_LINK_REGEX.lastIndex = 0;
    let plainMatch: RegExpExecArray | null;
    const plainMatches: { index: number; length: number; url: string }[] = [];
    while ((plainMatch = MESSAGE_LINK_REGEX.exec(text)) !== null) {
      const matchStart = plainMatch.index;
      const matchEnd = matchStart + plainMatch[0].length;
      // 检查是否被带标题的链接覆盖
      const isCovered = titledMatches.some(t => matchStart >= t.index && matchEnd <= t.index + t.length);
      if (!isCovered) {
        plainMatches.push({
          index: plainMatch.index,
          length: plainMatch[0].length,
          url: plainMatch[0],
        });
      }
    }

    if (titledMatches.length === 0 && plainMatches.length === 0) continue;

    // 合并并排序所有匹配
    const allMatches = [
      ...titledMatches.map(m => ({ ...m, type: 'titled' as const })),
      ...plainMatches.map(m => ({ ...m, type: 'plain' as const, title: undefined })),
    ].sort((a, b) => a.index - b.index);

    nodesToProcess.push({ node: textNode, segments: allMatches.map(m => ({
      type: m.type,
      content: text.slice(m.index, m.index + m.length),
      title: m.title,
      url: m.url,
      index: m.index,
      length: m.length,
    })) });
  }

  // 处理收集到的节点（倒序处理避免索引变化）
  for (const { node, segments } of nodesToProcess.reverse()) {
    const text = node.textContent || '';
    const fragment = document.createDocumentFragment();
    let lastIndex = 0;

    for (const seg of segments) {
      // 添加链接前的文本
      if (seg.index > lastIndex) {
        fragment.appendChild(document.createTextNode(text.slice(lastIndex, seg.index)));
      }

      // 解析链接参数
      const url = seg.url!;
      const params = parseMessageLink(url);
      if (params) {
        const info = resolveMessageLinkInfo(url, {
          currentWorldId: chat.currentWorldId,
          worldMap: chat.worldMap,
          findChannelById: (id) => chat.findChannelById(id),
        }, seg.title);

        if (info) {
          const wrapper = document.createElement('span');
          wrapper.innerHTML = renderMessageLinkHtml(info);
          const linkEl = wrapper.firstElementChild as HTMLAnchorElement;
          if (linkEl) {
            linkEl.addEventListener('click', (e) => {
              e.preventDefault();
              e.stopPropagation();
              handleMessageLinkClick(info);
            });
            fragment.appendChild(linkEl);
          } else {
            fragment.appendChild(document.createTextNode(seg.content));
          }
        } else {
          fragment.appendChild(document.createTextNode(seg.content));
        }
      } else {
        fragment.appendChild(document.createTextNode(seg.content));
      }

      lastIndex = seg.index + seg.length;
    }

    // 添加剩余文本
    if (lastIndex < text.length) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
    }

    node.replaceWith(fragment);
  }
};

const STATE_WIDGET_REGEX = /\[([^\]\|]+(?:\|[^\]\|]+)+)\]/g;

type StateWidgetTextSegment = {
  node: Text;
  start: number;
  end: number;
  text: string;
};

type StateWidgetRange = {
  start: number;
  end: number;
  isMarkdownLink: boolean;
};

type StateWidgetRenderItem = {
  node: Text;
  from: number;
  to: number;
  keepText?: string;
  widgetIndex?: number;
};

const collectStateWidgetTextSegments = (host: HTMLElement): StateWidgetTextSegment[] => {
  const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT, null);
  const segments: StateWidgetTextSegment[] = [];
  let cursor = 0;
  let textNode: Text | null;

  while ((textNode = walker.nextNode() as Text | null)) {
    const parent = textNode.parentElement;
    if (parent?.closest('.state-text-widget, a')) {
      continue;
    }
    const text = textNode.textContent || '';
    if (!text) {
      continue;
    }
    const start = cursor;
    const end = start + text.length;
    segments.push({ node: textNode, start, end, text });
    cursor = end;
  }

  return segments;
};

const collectStateWidgetRanges = (fullText: string): StateWidgetRange[] => {
  const ranges: StateWidgetRange[] = [];
  STATE_WIDGET_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = STATE_WIDGET_REGEX.exec(fullText)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    const isMarkdownLink = end < fullText.length && fullText[end] === '(';
    ranges.push({ start, end, isMarkdownLink });
  }
  return ranges;
};

const buildStateWidgetRenderMap = (
  segments: StateWidgetTextSegment[],
  ranges: StateWidgetRange[],
  entries: Array<{ type: string; options: string[]; index: number }>,
): Map<Text, StateWidgetRenderItem[]> => {
  const renderMap = new Map<Text, StateWidgetRenderItem[]>();
  const pushItem = (node: Text, item: StateWidgetRenderItem) => {
    const list = renderMap.get(node) || [];
    list.push(item);
    renderMap.set(node, list);
  };

  let widgetCounter = 0;
  for (const range of ranges) {
    const targetWidgetIndex = !range.isMarkdownLink && widgetCounter < entries.length
      ? widgetCounter
      : undefined;
    let widgetInserted = false;

    for (const seg of segments) {
      if (seg.end <= range.start || seg.start >= range.end) {
        continue;
      }
      const from = Math.max(seg.start, range.start) - seg.start;
      const to = Math.min(seg.end, range.end) - seg.start;
      if (from >= to) {
        continue;
      }
      if (range.isMarkdownLink) {
        const keepText = seg.text.slice(from, to);
        pushItem(seg.node, { node: seg.node, from, to, keepText });
        continue;
      }

      if (!widgetInserted && targetWidgetIndex !== undefined) {
        pushItem(seg.node, { node: seg.node, from, to, widgetIndex: targetWidgetIndex });
        widgetInserted = true;
      } else {
        // 非首段：删除匹配区间，避免跨节点重复插入 widget
        pushItem(seg.node, { node: seg.node, from, to, keepText: '' });
      }
    }

    if (!range.isMarkdownLink && targetWidgetIndex !== undefined) {
      widgetCounter++;
    }
  }

  return renderMap;
};

const processStateTextWidgets = () => {
  nextTick(() => {
    const host = messageContentRef.value;
    if (!host) return;
    const item = props.item as any;
    if (!item?.widgetData) return;

    let entries: Array<{ type: string; options: string[]; index: number }>;
    try {
      entries = typeof item.widgetData === 'string' ? JSON.parse(item.widgetData) : item.widgetData;
    } catch { return; }
    if (!entries?.length) return;

    // Permission pre-check
    const userId = user.info.id;
    const isSender = item.user?.id === userId || item.userId === userId || item.user_id === userId;
    let isMentioned = false;
    if (!isSender) {
      const content = item.content || '';
      const atRegex = /<at\s[^>]*id="([^"]*)"[^>]*\/?>/g;
      let m: RegExpExecArray | null;
      while ((m = atRegex.exec(content)) !== null) {
        if (m[1] === userId) { isMentioned = true; break; }
      }
    }
    let isAdmin = false;
    if (!isSender && !isMentioned) {
      const worldId = chat.currentWorldId;
      if (worldId) {
        const detail = chat.worldDetailMap[worldId];
        const memberRole = detail?.memberRole;
        const ownerId = detail?.world?.ownerId || chat.worldMap[worldId]?.ownerId;
        isAdmin = memberRole === 'owner' || memberRole === 'admin' || ownerId === userId;
      }
    }
    const canInteract = isSender || isMentioned || isAdmin;

    const segments = collectStateWidgetTextSegments(host);
    if (!segments.length) {
      return;
    }

    const fullText = segments.map(seg => seg.text).join('');
    const ranges = collectStateWidgetRanges(fullText);
    if (!ranges.length) {
      return;
    }

    const renderMap = buildStateWidgetRenderMap(segments, ranges, entries);
    renderMap.forEach((items, node) => {
      const text = node.textContent || '';
      const fragment = document.createDocumentFragment();
      let lastIndex = 0;

      const sorted = items.slice().sort((a, b) => a.from - b.from || a.to - b.to);
      for (const renderItem of sorted) {
        if (renderItem.from > lastIndex) {
          fragment.appendChild(document.createTextNode(text.slice(lastIndex, renderItem.from)));
        }

        if (renderItem.keepText !== undefined) {
          if (renderItem.keepText) {
            fragment.appendChild(document.createTextNode(renderItem.keepText));
          }
          lastIndex = renderItem.to;
          continue;
        }

        const widgetIdx = renderItem.widgetIndex;
        if (widgetIdx === undefined || widgetIdx >= entries.length) {
          fragment.appendChild(document.createTextNode(text.slice(renderItem.from, renderItem.to)));
          lastIndex = renderItem.to;
          continue;
        }

        const entry = entries[widgetIdx];

        const span = document.createElement('span');
        span.className = 'state-text-widget' + (canInteract ? ' state-text-widget--active' : '');
        span.dataset.widgetIndex = String(widgetIdx);
        const currentIndex = entry.index ?? 0;
        span.textContent = entry.options[currentIndex] || entry.options[0] || '';

        if (canInteract) {
          const msgId = item.id;
          const wIdx = widgetIdx;
          span.addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            chat.interactWithWidget(msgId, wIdx);
          });
        }

        fragment.appendChild(span);
        lastIndex = renderItem.to;
      }

      if (lastIndex < text.length) {
        fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
      }
      node.replaceWith(fragment);
    });
  });
};

const handleMessageLinkClick = async (info: { worldId: string; channelId: string; messageId: string; isCurrentWorld: boolean }) => {
  // 内联跳转，不开新标签页
  if (!info.isCurrentWorld) {
    try {
      await chat.switchWorld(info.worldId, { force: true });
    } catch {
      message.error('无法访问该世界');
      return;
    }
  }

  if (chat.curChannel?.id !== info.channelId) {
    const switched = await chat.channelSwitchTo(info.channelId);
    if (!switched) {
      message.error('无法访问该频道');
      return;
    }
  }

  await nextTick();
  chatEvent.emit('search-jump', {
    messageId: info.messageId,
    channelId: info.channelId,
  });
};

const handleStickyNoteEmbedClick = async (info: StickyNoteEmbedLinkParams) => {
  if (!info.worldId || !info.channelId || !info.noteId) {
    return;
  }

  if (chat.currentWorldId !== info.worldId || chat.curChannel?.id !== info.channelId) {
    message.warning('仅支持当前频道便签链接');
    return;
  }

  await nextTick();
  if (stickyNoteStore.currentChannelId !== info.channelId || !stickyNoteStore.notes[info.noteId]) {
    await stickyNoteStore.loadChannelNotes(info.channelId);
  }

  const targetNote = stickyNoteStore.notes[info.noteId];
  if (!targetNote) {
    message.warning('便签不存在或无权限访问');
    return;
  }

  stickyNoteStore.setVisible(true);
  stickyNoteStore.openNote(info.noteId);
  chatEvent.emit('sticky-note-highlight' as any, { noteId: info.noteId, ttlMs: 3000 } as any);
};

const copyStickyNoteEmbedLinkFromCard = async (rawLink: string) => {
  if (!rawLink) {
    return;
  }
  const copied = await copyTextWithFallback(rawLink);
  if (copied) {
    message.success('便签链接已复制');
    return;
  }
  message.error('复制失败');
};

const openContextMenu = (point: { x: number, y: number }, item: any) => {
  if (imageResizeMode.value) {
    return;
  }
  chat.avatarMenu.show = false;
  chat.messageMenu.optionsComponent.x = point.x;
  chat.messageMenu.optionsComponent.y = point.y;
  chat.messageMenu.item = item;
  chat.messageMenu.hasImage = hasImage.value;
  chat.messageMenu.show = true;
};

const onContextMenu = (e: MouseEvent, item: any) => {
  e.preventDefault();
  if (imageResizeMode.value) {
    e.stopPropagation();
    return;
  }
  openContextMenu({ x: e.clientX, y: e.clientY }, item);
};

const onMessageLongPress = (event: PointerEvent | MouseEvent | TouchEvent, item: any) => {
  if (imageResizeMode.value) {
    event.preventDefault?.();
    return;
  }
  const resolvePoint = (): { x: number, y: number } => {
    if ('clientX' in event && typeof event.clientX === 'number') {
      return { x: event.clientX, y: event.clientY };
    }
    if ('touches' in event && event.touches?.length) {
      const touch = event.touches[0];
      return { x: touch.clientX, y: touch.clientY };
    }
    const rect = messageContentRef.value?.getBoundingClientRect();
    if (rect) {
      return {
        x: rect.left + rect.width / 2,
        y: rect.top + rect.height / 2,
      };
    }
    return { x: 0, y: 0 };
  };

  openContextMenu(resolvePoint(), item);
};

const message = useMessage()
let avatarClickTimer: ReturnType<typeof setTimeout> | null = null;

const handleQuoteClick = () => {
  const quote = quoteItem.value as any;
  if (!quote?.id) {
    message.warning('未找到要跳转的消息');
    return;
  }
  const createdAt = quote.createdAt ?? quote.created_at;
  const displayOrder = quote.displayOrder ?? quote.display_order;
  chatEvent.emit('search-jump', {
    messageId: quote.id,
    createdAt,
    displayOrder,
  });
};

const getAvatarMenuPoint = (event: MouseEvent) => {
  const target = event.currentTarget as HTMLElement | null;
  if (target) {
    const rect = target.getBoundingClientRect();
    return {
      x: rect.right + 4,
      y: rect.top,
    };
  }
  return { x: event.clientX, y: event.clientY };
};

const doAvatarClick = (e: MouseEvent) => {
  if (isMobileUa) {
    return;
  }
  if (avatarClickTimer) {
    clearTimeout(avatarClickTimer);
    avatarClickTimer = null;
  }
  const point = getAvatarMenuPoint(e);
  avatarClickTimer = setTimeout(() => {
    chat.avatarMenu.optionsComponent.x = point.x;
    chat.avatarMenu.optionsComponent.y = point.y;
    chat.avatarMenu.item = props.item as any;
    chat.avatarMenu.show = true;
    emit('avatar-click')
  }, 320);
}

const preventAvatarNativeMenu = (event: Event) => {
  if (!isMobileUa) {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
};

const handleEditClick = (e: MouseEvent) => {
  e.stopPropagation();
  if (!canEdit.value) {
    return;
  }
  emit('edit', props.item);
}

const handleEditSave = (e: MouseEvent) => {
  e.stopPropagation();
  emit('edit-save', props.item);
}

const handleEditCancel = (e: MouseEvent) => {
  e.stopPropagation();
  emit('edit-cancel', props.item);
}

const handleSelectToggle = (e: MouseEvent) => {
  e.stopPropagation();
  handleMessageClick(e);
};

// Handle click on message block in multi-select mode
const handleMessageClick = (e: MouseEvent) => {
  if (!effectiveMultiSelectMode.value || !props.item?.id) return;
  
  // If in range mode, use range selection
  if (chat.multiSelect?.rangeModeEnabled) {
    // Use allMessageIds prop if available, otherwise emit for parent handling
    if (props.allMessageIds.length > 0) {
      chat.handleRangeClick(props.item.id, props.allMessageIds);
    } else {
      emit('range-click', props.item.id);
    }
    return;
  }
  
  // Otherwise toggle selection
  chat.toggleMessageSelection(props.item.id);
  emit('toggle-select', props.item?.id);
};

const handleAvatarLongpress = () => {
  if (isMobileUa) {
    return;
  }
  emit('avatar-longpress');
};

let avatarViewer: Viewer | null = null;
const doAvatarDblClick = (e: MouseEvent) => {
  if (isMobileUa) return;
  if (avatarClickTimer) {
    clearTimeout(avatarClickTimer);
    avatarClickTimer = null;
  }
  e.preventDefault();
  e.stopPropagation();
  chat.avatarMenu.show = false;
  const avatarUrl = displayAvatar.value || props.item?.member?.avatar || props.item?.user?.avatar;
  if (!avatarUrl) return;

  const resolvedUrl = resolveAttachmentUrl(avatarUrl) || avatarUrl;

  const tempImg = document.createElement('img');
  tempImg.src = resolvedUrl;
  tempImg.style.display = 'none';
  document.body.appendChild(tempImg);

  if (avatarViewer) {
    avatarViewer.destroy();
    avatarViewer = null;
  }

  avatarViewer = new Viewer(tempImg, {
    navbar: false,
    title: false,
    toolbar: {
      zoomIn: true,
      zoomOut: true,
      oneToOne: true,
      reset: true,
      prev: false,
      play: false,
      next: false,
      rotateLeft: true,
      rotateRight: true,
      flipHorizontal: false,
      flipVertical: false,
    },
    tooltip: true,
    movable: true,
    zoomable: true,
    rotatable: true,
    transition: true,
    fullscreen: true,
    keyboard: true,
    zIndex: 3000,
    hidden: () => {
      tempImg.remove();
      if (avatarViewer) {
        avatarViewer.destroy();
        avatarViewer = null;
      }
    },
  });

  avatarViewer.show();
};

onMounted(() => {
  stopMessageLongPress = onLongPress(
    messageContentRef,
    (event) => {
      if (!isMobileUa) {
        return;
      }
      const isTouchEvent =
        ('touches' in event) ||
        ('pointerType' in event && event.pointerType === 'touch');
      if (isTouchEvent) {
        event.preventDefault?.();
      }
      onMessageLongPress(event, props.item);
    }
  );

  applyDiceTone();
  ensureImageViewer();
  void ensureMessageImageLayoutsLoaded();
  void applyImageLayoutToDom();
  processMessageLinks();
  processStateTextWidgets();

  timestampInterval = setInterval(() => {
    timestampTicker.value = Date.now();
  }, 10000);

  // Setup lazy rendering observer
  setupVisibilityObserver()
  void applyKeywordHighlights()

  // Mobile: listen for global clicks to hide timestamp
  if (isMobileUa) {
    document.addEventListener('click', handleGlobalClickForTimestamp, true);
  }
  chatEvent.on('message-image-resize-enter' as any, handleImageResizeEnterRequest as any);
  window.addEventListener('pointermove', handleImageResizePointerMove, true);
  window.addEventListener('pointerup', handleMessageIFormPointerUp, true);
  window.addEventListener('mouseup', handleMessageIFormPointerUp, true);
  window.addEventListener('pointercancel', resetMessageIFormPointerState, true);
  window.addEventListener('blur', resetMessageIFormPointerState);
})

watch([displayContent, () => props.tone], () => {
  applyDiceTone();
  ensureImageViewer();
  void ensureMessageImageLayoutsLoaded();
  void applyImageLayoutToDom();
  processMessageLinks();
  processStateTextWidgets();
  resetMessageIFormSyncBaseline();
  nextTick(() => {
    setupMessageIFormResizeSync();
  });
}, { immediate: true });

watch(() => (props.item as any)?.widgetData, (newData) => {
  nextTick(() => {
    const host = messageContentRef.value;
    if (!host || !newData) return;
    let entries: Array<{ type: string; options: string[]; index: number }>;
    try {
      entries = typeof newData === 'string' ? JSON.parse(newData) : newData;
    } catch { return; }
    if (!entries?.length) return;
    const spans = host.querySelectorAll<HTMLSpanElement>('.state-text-widget');
    spans.forEach((span) => {
      const idx = parseInt(span.dataset.widgetIndex || '', 10);
      if (isNaN(idx) || idx >= entries.length) return;
      const entry = entries[idx];
      const currentIndex = entry.index ?? 0;
      span.textContent = entry.options[currentIndex] || entry.options[0] || '';
    });
  });
});

watch(() => otherEditingPreview.value?.previewHtml, () => {
  applyDiceTone();
  ensureImageViewer();
  void applyImageLayoutToDom();
});

watch(
  [
    () => compiledKeywords.value,
    () => displayStore.settings.worldKeywordHighlightEnabled,
    () => displayStore.settings.worldKeywordUnderlineOnly,
    () => displayStore.settings.worldKeywordTooltipEnabled,
    () => displayStore.settings.worldKeywordDeduplicateEnabled,
    () => displayStore.settings.worldKeywordTooltipTextIndent,
    () => displayContent.value,
  ],
  () => {
    // Recreate tooltip instance when settings change
    keywordTooltipInstance.destroy()
    keywordTooltipInstance = createKeywordTooltip(keywordTooltipResolver, {
      level: 0,
      compiledKeywords: compiledKeywords.value,
      onKeywordDoubleInvoke: props.worldKeywordEditable ? handleKeywordQuickEdit : undefined,
      underlineOnly: keywordUnderlineOnly.value,
      textIndent: displayStore.settings.worldKeywordTooltipTextIndent,
    })
    void applyKeywordHighlights()
  },
  { flush: 'post' },
)

onBeforeUnmount(() => {
  if (stopMessageLongPress) {
    stopMessageLongPress();
    stopMessageLongPress = null;
  }
  clearHoverTimer();
  if (timestampInterval) {
    clearInterval(timestampInterval);
    timestampInterval = null;
  }
  // Cleanup visibility observer
  if (keywordObserver) {
    keywordObserver.disconnect();
    keywordObserver = null;
  }
  // Mobile: remove global click listener
  if (isMobileUa) {
    document.removeEventListener('click', handleGlobalClickForTimestamp, true);
  }
  chatEvent.off('message-image-resize-enter' as any, handleImageResizeEnterRequest as any);
  window.removeEventListener('pointermove', handleImageResizePointerMove, true);
  window.removeEventListener('pointerup', handleMessageIFormPointerUp, true);
  window.removeEventListener('mouseup', handleMessageIFormPointerUp, true);
  window.removeEventListener('pointercancel', resetMessageIFormPointerState, true);
  window.removeEventListener('blur', resetMessageIFormPointerState);
  cleanupMessageIFormResizeSync();
  clearImageResizePointerState();
  destroyImageViewer();
  keywordTooltipInstance.hideAll()
  keywordTooltipInstance.destroy()
});

const nick = computed(() => {
  // 编辑状态下优先使用编辑预览中的角色名称（自己或他人）
  if (selfEditingPreview.value?.displayName) {
    return selfEditingPreview.value.displayName;
  }
  if (otherEditingPreview.value?.displayName) {
    return otherEditingPreview.value.displayName;
  }
  if (props.item?.identity?.displayName) {
    return props.item.identity.displayName;
  }
  // 检查后端直接设置的 sender_identity_name（导入的消息）
  if (props.item?.sender_identity_name) {
    return props.item.sender_identity_name;
  }
  if (props.item?.sender_member_name) {
    return props.item.sender_member_name;
  }
  return props.item?.member?.nick || props.item?.user?.name || '未知';
});

// 编辑状态下优先使用编辑预览中的头像（自己或他人）
const displayAvatar = computed(() => {
  if (selfEditingPreview.value?.avatar) {
    return selfEditingPreview.value.avatar;
  }
  if (otherEditingPreview.value?.avatar) {
    return otherEditingPreview.value.avatar;
  }
  return props.avatar;
});

const messageReactions = computed(() => {
  if (!props.item?.id) {
    return [];
  }
  return chat.getMessageReactions(props.item.id);
});

const handleReactionToggle = async (emoji: string) => {
  if (!props.item?.id) return;
  const reaction = messageReactions.value.find((item) => item.emoji === emoji);
  if (reaction?.meReacted) {
    await chat.removeReaction(props.item.id, emoji);
  } else {
    await chat.addReaction(props.item.id, emoji);
  }
};

const nameColor = computed(() => props.item?.identity?.color || props.item?.sender_identity_color || props.identityColor || '');

const senderIdentityId = computed(() => props.item?.identity?.id || props.item?.sender_identity_id || props.item?.senderIdentityId || '');


</script>

<template>
  <div v-if="item?.is_deleted" class="py-4 text-center text-gray-400">一条消息已被删除</div>
  <div v-else-if="item?.is_revoked" class="py-4 text-center">一条消息已被撤回</div>
  <div
    v-else
    ref="chatItemRef"
    :id="item?.id"
    class="chat-item"
    :class="[
      { 'is-rtl': props.isRtl },
      { 'is-editing': isEditing },
      `chat-item--${props.tone}`,
      `chat-item--layout-${props.layout}`,
      { 'chat-item--self': props.isSelf },
      { 'chat-item--merged': props.isMerged },
      { 'chat-item--body-only': props.bodyOnly },
      { 'chat-item--multiselect': effectiveMultiSelectMode },
      { 'chat-item--selected': effectiveIsSelected }
    ]"
    @mouseenter="handleTimestampHoverStart"
    @mouseleave="handleTimestampHoverEnd"
    @click="handleMobileTimestampTap"
  >
    <!-- Multi-select checkbox -->
    <div
      v-if="effectiveMultiSelectMode"
      class="chat-item__select-checkbox"
      @click.stop="handleSelectToggle"
    >
      <n-checkbox :checked="effectiveIsSelected" />
    </div>
    <div
      v-if="props.showAvatar"
      class="chat-item__avatar"
      :class="{ 'chat-item__avatar--hidden': props.hideAvatar }"
      @contextmenu="preventAvatarNativeMenu"
    >
      <Avatar :src="displayAvatar" :border="false" @longpress="handleAvatarLongpress" @click="doAvatarClick" @dblclick="doAvatarDblClick" />
    </div>
    <!-- <img class="rounded-md w-12 h-12 border-gray-500 border" :src="props.avatar" /> -->
    <!-- <n-avatar :src="imgAvatar" size="large" bordered>海豹</n-avatar> -->
    <div class="right" :class="{ 'right--hidden-header': !props.showHeader || props.bodyOnly }">
      <span class="title" v-if="props.showHeader && !props.bodyOnly">
        <!-- 右侧 -->
        <n-popover trigger="hover" placement="bottom" v-if="props.isRtl && timestampShouldRender">
          <template #trigger>
            <span class="time">{{ inlineTimestampText }}</span>
          </template>
          <span>{{ tooltipTimestampText }}</span>
        </n-popover>
        <span v-if="props.isRtl" class="name" :style="nameColor ? { color: nameColor } : undefined">{{ nick }}</span>
        <CharacterCardBadge v-if="props.isRtl" :identity-id="senderIdentityId" :identity-color="nameColor" />

        <span v-if="!props.isRtl" class="name" :style="nameColor ? { color: nameColor } : undefined">{{ nick }}</span>
        <CharacterCardBadge v-if="!props.isRtl" :identity-id="senderIdentityId" :identity-color="nameColor" />
        <n-popover trigger="hover" placement="bottom" v-if="!props.isRtl && timestampShouldRender">
          <template #trigger>
            <span class="time">{{ inlineTimestampText }}</span>
          </template>
          <span>{{ tooltipTimestampText }}</span>
        </n-popover>

        <!-- <span v-if="props.isRtl" class="time">{{ inlineTimestampText }}</span> -->
        <n-popover trigger="hover" placement="bottom" v-if="props.item?.isEdited">
          <template #trigger>
            <span class="edited-label">(已编辑)</span>
          </template>
          <div>
            <span v-if="props.item?.editedByUserName">由 {{ props.item.editedByUserName }} 编辑</span>
            <span v-if="editedTimeText2">{{ props.item?.editedByUserName ? '于' : '编辑于' }} {{ editedTimeText2 }}</span>
            <span v-else-if="!props.item?.editedByUserName">编辑时间未知</span>
          </div>
        </n-popover>
        <span v-if="props.item?.user?.is_bot || props.item?.user_id?.startsWith('BOT:')"
          class=" bg-blue-500 rounded-md px-2 text-white">bot</span>
      </span>
      <div class="content break-all relative" ref="messageContentRef" @contextmenu="onContextMenu($event, item)" @dblclick="handleContentDblclick" @click="handleContentClick" @pointerdown="handleMessageIFormPointerDown" @mousedown="handleMessageIFormPointerDown"
        :class="contentClassList">
        <div v-if="canEdit && !selfEditingPreview" class="message-action-bar"
          :class="{ 'message-action-bar--active': isEditing }">
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button text size="small" class="message-action-bar__btn" @click="handleEditClick">
                <n-icon :component="Edit" size="18" />
              </n-button>
            </template>
            编辑消息
          </n-tooltip>
        </div>
        <template v-if="!otherEditingPreview">
          <div>
            <div v-if="whisperLabel" class="whisper-label">
              <n-icon :component="Lock" size="16" />
              <span>{{ whisperLabel }}</span>
            </div>
            <div
              v-if="quoteItem"
              class="message-quote"
              :class="{
                'message-quote--disabled': !quoteJumpEnabled,
                'message-quote--muted': quoteIsDeleted || quoteIsRevoked,
              }"
              @click.stop="handleQuoteClick"
            >
              <n-icon class="message-quote__icon" :component="ArrowBackUp" size="14" />
              <div class="message-quote__body">
                <span class="message-quote__name" :style="quoteNameColor ? { color: quoteNameColor } : undefined">
                  {{ quoteDisplayName }}
                </span>
                <span class="message-quote__summary">
                  {{ quoteSummary }}
                </span>
              </div>
            </div>
            <component :is="parseContent(props, displayContent)" />
            <div v-if="imageResizeMode" class="image-resize-actions">
              <n-button size="tiny" type="primary" :disabled="!imageResizeHasChanges" @click.stop="saveImageResizedLayout">
                保存调整后的大小
              </n-button>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    size="tiny"
                    quaternary
                    circle
                    class="image-resize-actions__ratio-toggle"
                    :class="{ 'is-free': imageResizeFreeScaling }"
                    @click.stop="toggleImageResizeScaleMode"
                  >
                    <n-icon :component="Lock" size="14" />
                  </n-button>
                </template>
                {{ imageResizeFreeScaling ? '自由缩放：开（再次点击切回锁定比例）' : '锁定比例：开（点击后可自由缩放）' }}
              </n-tooltip>
              <n-button size="tiny" tertiary @click.stop="cancelImageResize">
                取消
              </n-button>
              <span v-if="messageImageAttachmentIds.length > 1" class="image-resize-actions__tip">单击后拖动已选图片可调整</span>
            </div>
          </div>
          <div v-if="selfEditingPreview" class="editing-self-actions">
            <n-button quaternary size="tiny" class="editing-self-actions__btn editing-self-actions__btn--save" @click.stop="handleEditSave">
              <n-icon :component="Check" size="14" class="editing-self-actions__btn-icon" />
              保存
            </n-button>
            <n-button text size="tiny" class="editing-self-actions__btn editing-self-actions__btn--cancel" @click.stop="handleEditCancel">
              <n-icon :component="X" size="12" class="editing-self-actions__btn-icon" />
              取消
            </n-button>
          </div>
        </template>
        <template v-else>
          <div
            :class="[
              'editing-preview__bubble',
              'editing-preview__bubble--inline',
              otherEditingPreview?.tone ? `editing-preview__bubble--tone-${otherEditingPreview.tone}` : '',
            ]"
            :data-tone="otherEditingPreview?.tone || 'ic'"
          >
            <div class="editing-preview__body" :class="{ 'is-placeholder': otherEditingPreview?.indicatorOnly }">
            <template v-if="otherEditingPreview?.indicatorOnly">
              正在更新内容...
            </template>
            <template v-else>
              <div
                v-if="otherEditingPreview?.previewHtml"
                class="editing-preview__rich"
                v-html="otherEditingPreview?.previewHtml"
              ></div>
              <span v-else>{{ otherEditingPreview?.summary || '[图片]' }}</span>
            </template>
            </div>
          </div>
        </template>
        <div v-if="props.item?.failed" class="failed absolute bg-red-600 rounded-md px-2 text-white">!</div>
      </div>
      <MessageReactions
        v-if="props.item?.id"
        :reactions="messageReactions"
        :message-id="props.item.id"
        @toggle="handleReactionToggle"
      />
    </div>
  </div>
</template>

<style lang="scss">
.chat-item {
  display: flex;
  width: 100%;
  align-items: flex-start;
  gap: 0.4rem;
}

.chat-item__avatar {
  flex-shrink: 0;
  width: var(--chat-avatar-size, 3rem);
  height: var(--chat-avatar-size, 3rem);
}

@media (pointer: coarse) {
  .chat-item__avatar {
    -webkit-touch-callout: none;
    user-select: none;
  }
}

.chat-item__avatar--hidden {
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  height: 0.25rem;
  min-height: 0;
  margin-top: 0;
  overflow: hidden;
}

/* Multi-select styles */
.chat-item__select-checkbox {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  flex-shrink: 0;
  cursor: pointer;
  z-index: 1;
}

.chat-item--multiselect {
  cursor: pointer;
}

.chat-item--selected {
  background-color: rgba(59, 130, 246, 0.1);
  border-radius: 8px;
  transition: background-color 0.15s ease;
}

:root[data-display-palette='night'] .chat-item--selected {
  background-color: rgba(59, 130, 246, 0.15);
}

.chat-item > .right {
  margin-left: 0.4rem;
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.chat--layout-compact .chat-item {
  gap: 0;
}

.chat--layout-compact .chat-item > .right {
  gap: 0.05rem;
}

.right--hidden-header {
  gap: 0;
}

.chat-item > .right > .title {
  display: flex;
  gap: 0.4rem;
  direction: ltr;
}

.chat-item > .right > .title > .name {
  font-weight: 600;
}

.chat-item > .right > .title > .time {
  color: #94a3b8;
}

.chat-item > .right > .content {
  position: relative;
  width: fit-content;
  max-width: 100%;
  padding: var(--chat-message-padding-y, 0.85rem) var(--chat-message-padding-x, 1.1rem);
  border-radius: var(--chat-message-radius, 0.85rem);
  background: var(--chat-ic-bg, #f5f5f5);
  color: var(--chat-text-primary, #111827);
  text-align: left;
  border: none;
  box-shadow: var(--chat-message-shadow, none);
  transition: background-color 0.25s ease, border-color 0.25s ease, color 0.25s ease, box-shadow 0.25s ease;
  font-size: var(--chat-font-size, 0.95rem);
  line-height: var(--chat-line-height, 1.6);
  letter-spacing: var(--chat-letter-spacing, 0px);
}

.chat-item > .right > .content .failed {
  right: -2rem;
  top: 0;
}

.chat-item > .right > .content.whisper-content {
  background: var(--chat-whisper-bg, #eef2ff);
  border: 1px solid var(--chat-whisper-border, rgba(99, 102, 241, 0.35));
  color: var(--chat-text-primary, #1f2937);
}

.chat-item--layout-bubble > .right {
  margin-left: 0.5rem;
  max-width: calc(100% - 3.5rem);
}

.chat-item--layout-bubble .chat-item__avatar {
  width: var(--chat-avatar-size, 2.75rem);
  height: var(--chat-avatar-size, 2.75rem);
  margin-right: 0.5rem;
}

.chat-item--layout-bubble .right > .content {
  border-radius: 0.85rem;
  padding: calc(var(--chat-message-padding-y, 0.85rem) * 0.8)
    calc(var(--chat-message-padding-x, 1.1rem) * 0.95);
}

.chat-item--layout-bubble.chat-item--self {
  flex-direction: row-reverse;
  justify-content: flex-end;
}

.chat-item--layout-bubble.chat-item--self .chat-item__avatar {
  margin-left: 0.5rem;
  margin-right: 0;
}

.chat-item--layout-bubble.chat-item--self > .right {
  margin-left: 0;
  margin-right: 0.5rem;
  align-items: flex-end;
  text-align: right;
}

.chat-item--layout-bubble.chat-item--self > .right > .title {
  justify-content: flex-end;
}

.chat-item--layout-bubble.chat-item--self > .right > .content {
  margin-left: auto;
  text-align: left;
}

.chat-item--merged > .right {
  margin-left: 0.4rem;
}

.chat-item--merged > .right > .content {
  margin-left: 0;
}

.chat-item--body-only {
  display: block;
}

.chat-item--body-only > .right {
  margin-left: 0;
}

.chat-item--layout-compact {
  width: 100%;
}

.chat-item--layout-compact > .right {
  width: 100%;
  flex: 1;
}

.chat-item--layout-compact > .right > .content {
  display: block;
  width: 100%;
  max-width: none;
  padding: 0.18rem 0;
  background: transparent;
  box-shadow: none;
  border: none;
  border-radius: 0;
}

.chat--layout-compact .chat-item > .right > .content {
  width: 100%;
  max-width: none;
}

.chat--layout-compact .chat-item--merged > .right > .content {
  padding-top: 0.1rem;
}

.message-quote {
  --quote-accent: var(--primary-color, #3b82f6);
  --quote-bg: var(--sc-bg-elevated, rgba(59, 130, 246, 0.05));
  --quote-bg-hover: var(--sc-bg-input, rgba(59, 130, 246, 0.08));
  --quote-border: var(--sc-border-strong, rgba(59, 130, 246, 0.25));
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0.5rem;
  align-items: center;
  margin-bottom: 0.5rem;
  padding: 0.45rem 0.6rem;
  border: 1px solid var(--quote-border);
  border-left-width: 3px;
  border-left-color: var(--quote-accent);
  border-radius: 0.6rem;
  background: var(--quote-bg);
  cursor: pointer;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

:root[data-display-palette='night'] .message-quote,
[data-display-palette='night'] .message-quote {
  --quote-bg: var(--sc-bg-input, rgba(15, 23, 42, 0.35));
  --quote-bg-hover: var(--sc-bg-elevated, rgba(15, 23, 42, 0.5));
  --quote-border: var(--sc-border-strong, rgba(148, 163, 184, 0.4));
}

.message-quote__icon {
  color: var(--quote-accent);
}

.message-quote__body {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.message-quote__name {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--chat-text-primary, #1f2937);
  line-height: 1.2;
}

.message-quote__summary {
  font-size: 0.82rem;
  color: var(--chat-text-primary, #1f2937);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.message-quote--muted .message-quote__summary {
  color: var(--chat-text-secondary, #94a3b8);
}

.message-quote--muted .message-quote__name {
  color: var(--chat-text-secondary, #94a3b8);
}

.message-quote--disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

.message-quote:not(.message-quote--disabled):hover {
  background: var(--quote-bg-hover);
}

.chat-item--layout-compact .message-quote {
  background: transparent;
  border-radius: 0;
  padding: 0.1rem 0 0.35rem 0.6rem;
  margin-bottom: 0.35rem;
  border: none;
  border-left: 2px solid var(--quote-accent);
}

.chat-item--layout-compact .message-quote__icon {
  color: var(--quote-accent);
}

.chat-item--layout-compact .message-quote__name {
  font-size: 0.72rem;
}

.chat-item--layout-compact .message-quote__summary {
  font-size: 0.78rem;
}

.chat-item--layout-compact .message-quote:not(.message-quote--disabled):hover {
  background: transparent;
}

.content img {
  max-width: min(36vw, 200px);
}

.content .inline-image {
  max-height: 6rem;
  width: auto;
  border-radius: 0.375rem;
  vertical-align: middle;
  margin: 0 0.25rem;
}

.content .message-image-adjustable {
  user-select: none;
  outline: 2px dashed color-mix(in srgb, var(--chat-accent, #3b82f6) 68%, white 32%);
  outline-offset: 2px;
  transition: box-shadow 0.16s ease, transform 0.16s ease, outline-color 0.16s ease;
}

.content .message-image-adjustable:not(.message-image-selected) {
  cursor: pointer;
}

.content .message-image-selected {
  cursor: nwse-resize;
  outline-style: solid;
  outline-width: 2px;
  outline-color: var(--chat-accent, #3b82f6);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--chat-accent, #3b82f6) 82%, white 18%);
  touch-action: none;
}

.content .message-image-unlocked {
  max-width: none !important;
  max-height: none !important;
}

.image-resize-actions {
  margin-top: 0.45rem;
  display: flex;
  align-items: center;
  gap: 0.45rem;
  flex-wrap: wrap;
}

.image-resize-actions__tip {
  font-size: 0.72rem;
  color: var(--chat-text-secondary, #64748b);
}

.image-resize-actions__ratio-toggle {
  width: 1.55rem;
  min-width: 1.55rem;
  height: 1.55rem;
  padding: 0;
}

.image-resize-actions__ratio-toggle.is-free {
  color: var(--chat-warning, #f97316);
  border-color: color-mix(in srgb, var(--chat-warning, #f97316) 50%, transparent 50%);
}

.content .rich-inline-image {
  max-width: 100%;
  max-height: 12rem;
  height: auto;
  border-radius: 0.5rem;
  vertical-align: middle;
  margin: 0.5rem 0.25rem;
  display: inline-block;
  object-fit: contain;
}

/* 富文本内容样式 */
.content {
  font-size: var(--chat-font-size, 0.95rem);
  line-height: var(--chat-line-height, 1.6);
  letter-spacing: var(--chat-letter-spacing, 0px);
}

.content h1,
.content h2,
.content h3 {
  margin: 0.75rem 0 0.5rem;
  font-weight: 600;
  line-height: 1.3;
}

.content h1 {
  font-size: 1.5rem;
}

.content h2 {
  font-size: 1.25rem;
}

.content h3 {
  font-size: 1.1rem;
}

.content ul,
.content ol {
  padding-left: 1.5rem;
  margin: 0.5rem 0;
}

.content ul {
  list-style-type: disc;
}

.content ol {
  list-style-type: decimal;
}

.content li {
  margin: 0.25rem 0;
}

.content blockquote {
  border-left: 3px solid #3b82f6;
  padding-left: 1rem;
  margin: 0.5rem 0;
  color: #6b7280;
}

.content code {
  background-color: var(--chat-inline-code-bg, #f3f4f6);
  color: var(--chat-inline-code-fg, inherit);
  border: 1px solid var(--chat-inline-code-border, transparent);
  border-radius: 0.25rem;
  padding: 0.125rem 0.375rem;
  font-family: 'Courier New', monospace;
  font-size: 0.9em;
}

.content pre {
  background-color: #1f2937;
  color: #f9fafb;
  border-radius: 0.5rem;
  padding: 1rem;
  margin: 0.75rem 0;
  overflow-x: auto;
}

.content pre code {
  background-color: transparent;
  color: inherit;
  padding: 0;
}

.content strong {
  font-weight: 600;
}

.content em {
  font-style: italic;
}

.content u {
  text-decoration: underline;
}

.content s {
  text-decoration: line-through;
}

.content mark {
  background-color: #fef08a;
  padding: 0.1rem 0.2rem;
  border-radius: 0.125rem;
}

.content a {
  color: #3b82f6;
  text-decoration: underline;
}

.content hr {
  border: none;
  border-top: 2px solid #e5e7eb;
  margin: 1rem 0;
}

.content p {
  margin: 0;
  line-height: 1.5;
}

.content p + p {
  margin-top: var(--chat-paragraph-spacing, 0.5rem);
}
.edited-label {
  @apply text-xs text-blue-500 font-medium;
  margin-left: 0.2rem;
}

.message-action-bar {
  position: absolute;
  top: -1.6rem;
  right: -0.4rem;
  display: flex;
  gap: 0.25rem;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
}

.message-action-bar__btn {
  pointer-events: auto;
  color: rgba(15, 23, 42, 0.75);
}

:root[data-display-palette='night'] .message-action-bar__btn {
  color: #c5cfd9;
}

.chat-item .content:hover .message-action-bar,
.chat-item.is-editing .message-action-bar,
.chat-item .message-action-bar--active {
  opacity: 1;
  pointer-events: auto;
}

.chat-item--layout-compact .message-action-bar {
  top: 50%;
  right: 0.35rem;
  transform: translateY(-50%);
}

.chat-item > .right > .content.content--editing-preview {
  background: transparent;
  border: none;
  box-shadow: none;
  padding: 0;
}

.chat-item--ooc .right > .content.content--editing-preview,
.chat-item--layout-bubble .right > .content.content--editing-preview {
  background: transparent;
  border: none;
  box-shadow: none;
}

.content--editing-preview.whisper-content {
  background: transparent;
}


.editing-preview__bubble {
  width: 100%;
  border-radius: var(--chat-message-radius, 0.85rem);
  padding: 0.6rem 0.9rem;
  max-width: 32rem;
  --editing-preview-bg: var(--chat-preview-bg, #f6f7fb);
  --editing-preview-dot: var(--chat-preview-dot, rgba(148, 163, 184, 0.45));
  background-color: var(--editing-preview-bg);
  border: 1px solid transparent;
  box-shadow: none;
  color: var(--chat-text-primary, #1f2937);
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

.editing-preview__bubble[data-tone='ic'] {
  --editing-preview-bg: #fbfdf7;
  --editing-preview-dot: var(--chat-preview-dot-ic, rgba(148, 163, 184, 0.35));
  border-color: rgba(15, 23, 42, 0.14);
}

.editing-preview__bubble[data-tone='ooc'] {
  --editing-preview-bg: #ffffff;
  --editing-preview-dot: var(--chat-preview-dot-ooc, rgba(148, 163, 184, 0.25));
  border-color: rgba(15, 23, 42, 0.12);
}

:root[data-display-palette='night'] .editing-preview__bubble[data-tone='ic'] {
  --editing-preview-bg: #3f3f45;
  --editing-preview-dot: var(--chat-preview-dot-ic-night, rgba(148, 163, 184, 0.2));
  border-color: rgba(255, 255, 255, 0.16);
  color: #f4f4f5;
}

:root[data-display-palette='night'] .editing-preview__bubble[data-tone='ooc'] {
  --editing-preview-bg: #2D2D31;
  --editing-preview-dot: var(--chat-preview-dot-ooc-night, rgba(148, 163, 184, 0.2));
  border-color: rgba(255, 255, 255, 0.24);
  color: #f5f3ff;
}

.chat-item--layout-compact .content--editing-preview .editing-preview__bubble,
.chat-item--layout-compact .editing-preview__bubble--inline {
  background-image: radial-gradient(var(--editing-preview-dot) 1px, transparent 1px);
  background-size: 10px 10px;
  max-width: none;
  width: 100%;
  display: block;
  box-sizing: border-box;
  border-radius: 0.45rem;
}

.editing-preview__body {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: var(--chat-font-size, 0.95rem);
  line-height: var(--chat-line-height, 1.6);
  letter-spacing: var(--chat-letter-spacing, 0px);
  color: inherit;
}

.editing-preview__rich {
  word-break: break-word;
  white-space: pre-wrap;
}

.editing-preview__body.is-placeholder {
  color: #6b7280;
}

.editing-self-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  align-items: center;
  margin-top: 0.3rem;
}

.editing-self-actions__btn {
  color: #111827 !important;
  --n-text-color: currentColor;
  --n-text-color-hover: color-mix(in srgb, currentColor 80%, transparent);
  padding: 0 0.2rem;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}

:root[data-display-palette='day'] .editing-self-actions__btn {
  color: #111827 !important;
}

:root[data-display-palette='night'] .editing-self-actions__btn {
  color: #C5CFD9 !important;
}

.editing-self-actions__btn-icon {
  color: currentColor;
}

.whisper-label {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.78rem;
  font-weight: 600;
  letter-spacing: 0.01em;
  color: #4c1d95;
  background: rgba(99, 102, 241, 0.08);
  border-radius: 0.65rem;
  padding: 0.25rem 0.65rem;
  margin-bottom: 0.55rem;
  white-space: pre-line;
}

.whisper-label svg {
  color: inherit;
  margin-right: 0.35rem;
}

.whisper-label--quote {
  font-size: 0.72rem;
  color: #5b21b6;
  margin-bottom: 0.25rem;
}

.whisper-content .whisper-label,
.whisper-content .whisper-label--quote {
  background: rgba(99, 102, 241, 0.12);
  color: #4c1d95;
}

.whisper-content .whisper-label--quote {
  color: #6d28d9;
}

.whisper-content .whisper-label svg {
  color: #4c1d95;
}

.whisper-content .text-gray-400 {
  color: #5b21b6;
}

/* Tone 样式 */
.chat-item--ooc .right .content {
  background: var(--chat-ooc-bg, rgba(156, 163, 175, 0.1));
  border: none;
  color: var(--chat-ooc-text, var(--chat-text-secondary, #6b7280));
  font-size: calc(var(--chat-font-size, 0.95rem) - 2px);
}

.chat-item--archived {
  opacity: 0.6;
}

.chat-item--archived .right .content {
  background: var(--chat-archived-bg, rgba(248, 250, 252, 0.8));
  border: 1px solid var(--chat-archived-border, rgba(209, 213, 219, 0.5));
  color: var(--chat-text-secondary, #94a3b8);
}

.chat--layout-compact .chat-item--archived .right .content,
.chat--layout-compact .chat-item--ooc .right .content {
  background: transparent;
  border: none;
  border-radius: 0;
  padding: 0;
  box-shadow: none;
}

.chat--layout-compact .chat-item--ooc .right .content {
  color: var(--chat-ooc-text, var(--chat-text-secondary, #6b7280));
  font-size: calc(var(--chat-font-size, 0.95rem) - 2px);
}

.chat--layout-compact .chat-item > .right > .content.whisper-content {
  background: transparent;
  border: none;
  color: var(--chat-text-primary);
  padding-left: 0;
  padding-right: 0;
}

.chat--layout-compact .whisper-label,
.chat--layout-compact .whisper-label--quote {
  background: transparent;
  padding-left: 0;
  padding-right: 0;
  border-radius: 0;
  color: var(--chat-text-secondary);
}

.chat--layout-compact .chat-item--ooc {
  width: 100%;
  background: transparent;
  border-radius: 0;
  padding: 0;
}

.chat--layout-compact .chat-item--ooc > .right > .content {
  padding: 0;
  background: transparent;
  color: var(--chat-text-secondary);
}

/* @ mention capsule styles */
.mention-capsule {
  display: inline;
  background-color: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
  padding: 0 0.35em;
  border-radius: 4px;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.mention-capsule:hover {
  background-color: rgba(59, 130, 246, 0.2);
}

.mention-capsule--self {
  background-color: rgba(59, 130, 246, 0.2);
  font-weight: 600;
}

.mention-capsule--all {
  background-color: rgba(239, 68, 68, 0.1);
  color: #ef4444;
}

.mention-capsule--all:hover {
  background-color: rgba(239, 68, 68, 0.2);
}

/* Night mode */
:root[data-display-palette='night'] .mention-capsule {
  background-color: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
}

:root[data-display-palette='night'] .mention-capsule:hover {
  background-color: rgba(59, 130, 246, 0.3);
}

:root[data-display-palette='night'] .mention-capsule--self {
  background-color: rgba(59, 130, 246, 0.3);
}

:root[data-display-palette='night'] .mention-capsule--all {
  background-color: rgba(239, 68, 68, 0.2);
  color: #f87171;
}

:root[data-display-palette='night'] .mention-capsule--all:hover {
  background-color: rgba(239, 68, 68, 0.3);
}

.state-text-widget {
  display: inline;
  padding: 1px 6px;
  border-radius: 4px;
  border-bottom: 2px dashed var(--primary-color, #4098fc);
  background-color: rgba(64, 152, 252, 0.08);
  font-weight: 500;
  user-select: none;
  transition: background-color 0.15s, border-color 0.15s;
}

.state-text-widget--active {
  cursor: pointer;
}

.state-text-widget--active:hover {
  background-color: rgba(64, 152, 252, 0.18);
  border-bottom-style: solid;
}

.message-sticky-note-embed {
  --sticky-note-accent: #64748b;
  display: block;
  max-width: min(500px, 100%);
  width: auto;
  padding: 0.05rem 0;
  border: none;
  border-radius: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
  transition: opacity 0.15s ease;
}

.message-sticky-note-embed:hover {
  opacity: 0.96;
}

.message-sticky-note-embed--interactive {
  cursor: default;
}

.message-sticky-note-embed:focus-within {
  outline: 2px solid color-mix(in srgb, var(--primary-color, #4098fc) 50%, transparent);
  outline-offset: 2px;
}

.message-sticky-note-embed__summary-row {
  display: flex;
  align-items: flex-start;
  gap: 0.32rem;
  list-style: none;
  cursor: pointer;
}

.message-sticky-note-embed__summary-row::-webkit-details-marker {
  display: none;
}

.message-sticky-note-embed__fold-icon {
  margin-top: 0.1rem;
  font-size: 0.9rem;
  line-height: 1;
  color: var(--chat-text-secondary, #64748b);
  transition: transform 0.16s ease;
}

.message-sticky-note-embed:not([open]) .message-sticky-note-embed__fold-icon {
  transform: rotate(-90deg);
}

.message-sticky-note-embed__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}

.message-sticky-note-embed__title {
  display: block;
  white-space: normal;
  font-weight: 600;
  font-size: 0.82rem;
  line-height: 1.25;
  color: color-mix(in srgb, var(--sticky-note-accent) 78%, currentColor);
}

.message-sticky-note-embed__title-btn {
  display: block;
  padding: 0;
  border: none;
  background: transparent;
  text-align: left;
  white-space: normal;
  font-size: 0.82rem;
  font-weight: 600;
  line-height: 1.25;
  color: color-mix(in srgb, var(--sticky-note-accent) 78%, currentColor);
  cursor: pointer;
}

.message-sticky-note-embed__title-btn:hover {
  text-decoration: underline;
}

.message-sticky-note-embed__content {
  display: block;
  font-size: 0.71rem;
  color: var(--chat-text-secondary, #64748b);
  line-height: 1.35;
  white-space: pre-wrap;
  word-break: break-word;
}

.message-sticky-note-embed__panel {
  margin-left: 0.92rem;
  margin-top: 0.08rem;
}

.message-sticky-note-embed__widget {
  width: min(430px, 100%);
  max-width: 100%;
  min-width: 0;
  min-height: 0;
  max-height: 680px;
  resize: both;
  overflow: auto;
  scrollbar-width: thin;
}

.message-sticky-note-embed__widget :deep(.sticky-note-counter),
.message-sticky-note-embed__widget :deep(.sticky-note-slider),
.message-sticky-note-embed__widget :deep(.sticky-note-list),
.message-sticky-note-embed__widget :deep(.sticky-note-timer),
.message-sticky-note-embed__widget :deep(.sticky-note-clock),
.message-sticky-note-embed__widget :deep(.sticky-note-round) {
  padding: 4px 0;
}

.message-sticky-note-embed__widget :deep(.sticky-note-counter__btn) {
  width: 34px;
  height: 34px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-counter__value) {
  width: 80px;
  height: 34px;
  font-size: 18px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-counter__hint) {
  margin-top: 6px;
  font-size: 10px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-slider__value-input) {
  width: 58px;
  padding: 4px;
  font-size: 14px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-slider__settings-trigger) {
  height: 18px;
  margin-top: 4px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-slider__settings-hint) {
  font-size: 10px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-list) {
  max-height: 200px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-list__item) {
  padding: 4px 6px;
  gap: 6px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-list__content),
.message-sticky-note-embed__widget :deep(.sticky-note-list__edit-input) {
  font-size: 12px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-list__footer-btn),
.message-sticky-note-embed__widget :deep(.sticky-note-list__add-btn) {
  font-size: 11px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-timer__display) {
  font-size: 24px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-timer__btn),
.message-sticky-note-embed__widget :deep(.sticky-note-timer__dir-btn),
.message-sticky-note-embed__widget :deep(.sticky-note-timer__adj-btn),
.message-sticky-note-embed__widget :deep(.sticky-note-timer__set-reset) {
  font-size: 11px;
  padding: 4px 8px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-clock__circle) {
  width: 92px;
  height: 92px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-clock__count) {
  font-size: 9px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-clock__btn),
.message-sticky-note-embed__widget :deep(.sticky-note-clock__adj) {
  font-size: 11px;
  padding: 3px 7px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-round__value) {
  font-size: 26px;
  min-width: 62px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-round__nav) {
  width: 30px;
  height: 30px;
}

.message-sticky-note-embed__widget :deep(.sticky-note-round__btn),
.message-sticky-note-embed__widget :deep(.sticky-note-round__limit),
.message-sticky-note-embed__widget :deep(.sticky-note-round__limit-input) {
  font-size: 11px;
}

.message-sticky-note-embed__side {
  margin-top: 0.02rem;
  display: flex;
  flex-direction: row;
  align-items: flex-end;
  gap: 0.2rem;
  flex-shrink: 0;
}

.message-sticky-note-embed__copy-btn {
  width: 1rem;
  height: 1rem;
  border: none;
  border-radius: 3px;
  background: transparent;
  color: color-mix(in srgb, var(--chat-text-secondary, #64748b) 90%, transparent);
  font-size: 0.66rem;
  cursor: pointer;
  line-height: 1;
  opacity: 0.72;
  transition: opacity 0.15s ease, color 0.15s ease;
}

.message-sticky-note-embed__copy-btn:hover {
  opacity: 1;
  color: var(--chat-text-primary, #0f172a);
}

.message-iform-embed {
  position: relative;
  overflow: auto;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--chat-border-mute, rgba(15, 23, 42, 0.08)) 88%, transparent);
  background: color-mix(in srgb, var(--chat-ic-bg, #f5f5f5) 88%, transparent);
  resize: both;
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}

.message-iform-embed::-webkit-scrollbar {
  width: 4px;
  height: 4px;
}

.message-iform-embed::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: transparent;
}

.message-iform-embed:hover {
  scrollbar-color: color-mix(in srgb, var(--chat-border-mute, rgba(15, 23, 42, 0.24)) 85%, transparent) transparent;
}

.message-iform-embed:hover::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--chat-border-mute, rgba(15, 23, 42, 0.24)) 85%, transparent);
}

.message-iform-embed :deep(.iform-frame) {
  border: none;
  border-radius: 10px;
  background: transparent;
  box-shadow: none;
}

.message-iform-embed :deep(.iform-frame__iframe),
.message-iform-embed :deep(.iform-frame__html) {
  border-radius: 10px;
}

:root[data-display-palette='night'] .message-iform-embed {
  border-color: color-mix(in srgb, var(--chat-border-mute, rgba(148, 163, 184, 0.35)) 85%, transparent);
  background: color-mix(in srgb, var(--chat-ic-bg, rgba(15, 23, 42, 0.45)) 75%, transparent);
}

:root[data-display-palette='night'] .message-sticky-note-embed {
  background: transparent;
}

:root[data-display-palette='night'] .message-sticky-note-embed__content {
  color: color-mix(in srgb, var(--chat-text-secondary, #94a3b8) 95%, transparent);
}

:root[data-display-palette='night'] .message-sticky-note-embed__copy-btn {
  color: color-mix(in srgb, var(--chat-text-secondary, #94a3b8) 92%, transparent);
}

:root[data-display-palette='night'] .message-sticky-note-embed__copy-btn:hover {
  color: color-mix(in srgb, var(--chat-text-primary, #e2e8f0) 96%, transparent);
}

.state-text-widget--active:active {
  background-color: rgba(64, 152, 252, 0.28);
}

:root[data-display-palette='night'] .state-text-widget {
  background-color: rgba(64, 152, 252, 0.12);
  border-bottom-color: rgba(64, 152, 252, 0.6);
}

:root[data-display-palette='night'] .state-text-widget--active:hover {
  background-color: rgba(64, 152, 252, 0.25);
}
</style>
