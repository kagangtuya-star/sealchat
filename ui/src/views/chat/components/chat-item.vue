<script setup lang="tsx">
import dayjs from 'dayjs';
import Element from '@satorijs/element'
import { onMounted, ref, h, computed, watch, PropType, onBeforeUnmount, nextTick } from 'vue';
import { urlBase } from '@/stores/_config';
import DOMPurify from 'dompurify';
import { useUserStore } from '@/stores/user';
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import { Howl, Howler } from 'howler';
import { useMessage } from 'naive-ui';
import Avatar from '@/components/avatar.vue'
import { Lock, Edit, Check, X } from '@vicons/tabler';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { onLongPress } from '@vueuse/core';
import Viewer from 'viewerjs';
import 'viewerjs/dist/viewer.css';
import { useWorldGlossaryStore } from '@/stores/worldGlossary'
import { useDisplayStore } from '@/stores/display'
import { refreshWorldKeywordHighlights } from '@/utils/worldKeywordHighlighter'
import { createKeywordTooltip } from '@/utils/keywordTooltip'

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
const utils = useUtilsStore();
const { t } = useI18n();
const worldGlossary = useWorldGlossaryStore();
const displayStore = useDisplayStore();

const isMobileUa = typeof navigator !== 'undefined'
  ? /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  : false;

function timeFormat(time?: string) {
  if (!time) return '未知';
  // console.log('???', time, typeof time)
  // return dayjs(time).format('MM-DD HH:mm:ss');
  return dayjs(time).fromNow();
}

function timeFormat2(time?: string) {
  if (!time) return '未知';
  // console.log('???', time, typeof time)
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss');
}

let hasImage = ref(false);
const messageContentRef = ref<HTMLElement | null>(null);
let stopMessageLongPress: (() => void) | null = null;
let inlineImageViewer: Viewer | null = null;

const diceChipHtmlPattern = /<span[^>]*class="[^"]*dice-chip[^"]*"/i;

const parseContent = (payload: any, overrideContent?: string) => {
  const content = overrideContent ?? payload?.content ?? '';

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
          item.attrs.src = resolveAttachmentUrl(item.attrs.src);
        }
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
      case "at":
        if (item.attrs.id == user.info.id) {
          textItems.push(`<span class="text-blue-500 bg-gray-400 px-1" style="white-space: pre-wrap">@${item.attrs.name}</span>`);
        } else {
          textItems.push(`<span class="text-blue-500" style="white-space: pre-wrap">@${item.attrs.name}</span>`);
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

const destroyImageViewer = () => {
  if (inlineImageViewer) {
    inlineImageViewer.destroy();
    inlineImageViewer = null;
  }
};

const setupImageViewer = async () => {
  await nextTick();
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

  if (inlineImageViewer) {
    inlineImageViewer.update();
    return;
  }

  inlineImageViewer = new Viewer(host, {
    className: 'chat-inline-image-viewer',
    navbar: false,
    title: false,
    toolbar: true,
    tooltip: false,
    scalable: false,
    rotatable: false,
    transition: false,
    fullscreen: false,
    zIndex: 2500,
  });
};

const ensureImageViewer = () => {
  void setupImageViewer();
};

const handleContentDblclick = async (event: MouseEvent) => {
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
})

const timeText = ref(timeFormat(props.item?.createdAt));
const timeText2 = ref(timeFormat2(props.item?.createdAt));
const editedTimeText = ref(props.item?.isEdited ? timeFormat(props.item?.updatedAt) : '');
const editedTimeText2 = ref(props.item?.isEdited ? timeFormat2(props.item?.updatedAt) : '');

const getMemberDisplayName = (item: any) => item?.whisperMeta?.senderMemberName
  || item?.identity?.displayName
  || item?.sender_member_name
  || item?.member?.nick
  || item?.user?.nick
  || item?.user?.name
  || item?.whisperMeta?.senderUserNick
  || item?.whisperMeta?.senderUserName
  || '未知成员';
const getTargetDisplayName = (item: any) => item?.whisperMeta?.targetMemberName
  || item?.whisperTo?.nick
  || item?.whisperTo?.name
  || item?.whisperMeta?.targetUserNick
  || item?.whisperMeta?.targetUserName
  || '未知成员';

const buildWhisperLabel = (item?: any) => {
  if (!item?.isWhisper) return '';
  const senderName = getMemberDisplayName(item);
  const targetName = getTargetDisplayName(item);
  const senderLabel = `@${senderName}`;
  const targetLabel = `@${targetName}`;
  const senderUserId = item?.user?.id || item?.whisperMeta?.senderUserId;
  const targetUserId = item?.whisperTo?.id || item?.whisperMeta?.targetUserId;
  if (senderUserId === user.info.id) {
    return t('whisper.sendTo', { target: targetLabel });
  }
  if (targetUserId === user.info.id) {
    return t('whisper.from', { sender: senderLabel });
  }
  if (targetName && targetName !== '未知成员') {
    return t('whisper.sendTo', { target: targetLabel });
  }
  return t('whisper.generic');
};

const whisperLabel = computed(() => buildWhisperLabel(props.item));
const quoteWhisperLabel = computed(() => buildWhisperLabel((props.item as any)?.quote));

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
const canEdit = computed(() => props.item?.user?.id === user.info.id);

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

const keywordTooltip = createKeywordTooltip((keywordId) => {
  const keyword = worldGlossary.keywordById[keywordId]
  if (!keyword) {
    return null
  }
  return {
    title: keyword.keyword,
    description: keyword.description,
  }
})

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

const applyKeywordHighlights = async () => {
  await nextTick()
  const host = messageContentRef.value
  if (!host) {
    return
  }
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
      onKeywordDoubleInvoke: props.worldKeywordEditable ? handleKeywordQuickEdit : undefined,
    },
    keywordTooltipEnabled.value ? keywordTooltip : undefined,
  )
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

const openContextMenu = (point: { x: number, y: number }, item: any) => {
  chat.avatarMenu.show = false;
  chat.messageMenu.optionsComponent.x = point.x;
  chat.messageMenu.optionsComponent.y = point.y;
  chat.messageMenu.item = item;
  chat.messageMenu.hasImage = hasImage.value;
  chat.messageMenu.show = true;
};

const onContextMenu = (e: MouseEvent, item: any) => {
  e.preventDefault();
  openContextMenu({ x: e.clientX, y: e.clientY }, item);
};

const onMessageLongPress = (event: PointerEvent | MouseEvent | TouchEvent, item: any) => {
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
const doAvatarClick = (e: MouseEvent) => {
  if (isMobileUa) {
    return;
  }
  if (!props.item?.member?.nick) {
    message.warning('此用户无法查看')
    return;
  }
  chat.avatarMenu.show = true;

  chat.messageMenu.optionsComponent.x = e.x;
  chat.messageMenu.optionsComponent.y = e.y;
  chat.avatarMenu.item = props.item as any;
  emit('avatar-click')
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

const emit = defineEmits(['avatar-longpress', 'avatar-click', 'edit', 'edit-save', 'edit-cancel']);

const handleAvatarLongpress = () => {
  if (isMobileUa) {
    return;
  }
  emit('avatar-longpress');
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

  setInterval(() => {
    timeText.value = timeFormat(props.item?.createdAt);
    timeText2.value = timeFormat2(props.item?.createdAt);
    if (props.item?.isEdited) {
      editedTimeText.value = timeFormat(props.item?.updatedAt);
      editedTimeText2.value = timeFormat2(props.item?.updatedAt);
    }
  }, 10000);

  void applyKeywordHighlights()
})

watch([displayContent, () => props.tone], () => {
  applyDiceTone();
  ensureImageViewer();
}, { immediate: true });

watch(() => otherEditingPreview.value?.previewHtml, () => {
  applyDiceTone();
  ensureImageViewer();
});

watch(
  [
    () => compiledKeywords.value,
    () => displayStore.settings.worldKeywordHighlightEnabled,
    () => displayStore.settings.worldKeywordUnderlineOnly,
    () => displayStore.settings.worldKeywordTooltipEnabled,
    () => displayContent.value,
  ],
  () => {
    void applyKeywordHighlights()
  },
  { flush: 'post' },
)

onBeforeUnmount(() => {
  if (stopMessageLongPress) {
    stopMessageLongPress();
    stopMessageLongPress = null;
  }
  destroyImageViewer();
  keywordTooltip.hide()
});

const nick = computed(() => {
  if (props.item?.identity?.displayName) {
    return props.item.identity.displayName;
  }
  if (props.item?.sender_member_name) {
    return props.item.sender_member_name;
  }
  return props.item?.member?.nick || props.item?.user?.name || '未知';
});

const nameColor = computed(() => props.item?.identity?.color || props.identityColor || '');

watch(() => props.item?.updatedAt, () => {
  if (props.item?.isEdited) {
    editedTimeText.value = timeFormat(props.item?.updatedAt);
    editedTimeText2.value = timeFormat2(props.item?.updatedAt);
  }
});

</script>

<template>
  <div v-if="item?.is_deleted" class="py-4 text-center text-gray-400">一条消息已被删除</div>
  <div v-else-if="item?.is_revoked" class="py-4 text-center">一条消息已被撤回</div>
  <div v-else :id="item?.id" class="chat-item"
    :class="[
      { 'is-rtl': props.isRtl },
      { 'is-editing': isEditing },
      `chat-item--${props.tone}`,
      `chat-item--layout-${props.layout}`,
      { 'chat-item--self': props.isSelf },
      { 'chat-item--merged': props.isMerged },
      { 'chat-item--body-only': props.bodyOnly }
    ]">
    <div
      v-if="props.showAvatar"
      class="chat-item__avatar"
      :class="{ 'chat-item__avatar--hidden': props.hideAvatar }"
      @contextmenu="preventAvatarNativeMenu"
    >
      <Avatar :src="props.avatar" :border="false" @longpress="handleAvatarLongpress" @click="doAvatarClick" />
    </div>
    <!-- <img class="rounded-md w-12 h-12 border-gray-500 border" :src="props.avatar" /> -->
    <!-- <n-avatar :src="imgAvatar" size="large" bordered>海豹</n-avatar> -->
    <div class="right" :class="{ 'right--hidden-header': !props.showHeader || props.bodyOnly }">
      <span class="title" v-if="props.showHeader && !props.bodyOnly">
        <!-- 右侧 -->
        <n-popover trigger="hover" placement="bottom" v-if="props.isRtl">
          <template #trigger>
            <span class="time">{{ timeText }}</span>
          </template>
          <span>{{ timeText2 }}</span>
        </n-popover>
        <span v-if="props.isRtl" class="name" :style="nameColor ? { color: nameColor } : undefined">{{ nick }}</span>

        <span v-if="!props.isRtl" class="name" :style="nameColor ? { color: nameColor } : undefined">{{ nick }}</span>
        <n-popover trigger="hover" placement="bottom" v-if="!props.isRtl">
          <template #trigger>
            <span class="time">{{ timeText }}</span>
          </template>
          <span>{{ timeText2 }}</span>
        </n-popover>

        <!-- <span v-if="props.isRtl" class="time">{{ timeText }}</span> -->
        <n-popover trigger="hover" placement="bottom" v-if="props.item?.isEdited">
          <template #trigger>
            <span class="edited-label">(已编辑)</span>
          </template>
          <span v-if="editedTimeText2">编辑于 {{ editedTimeText2 }}</span>
          <span v-else>编辑时间未知</span>
        </n-popover>
        <span v-if="props.item?.user?.is_bot || props.item?.user_id?.startsWith('BOT:')"
          class=" bg-blue-500 rounded-md px-2 text-white">bot</span>
      </span>
      <div class="content break-all relative" ref="messageContentRef" @contextmenu="onContextMenu($event, item)" @dblclick="handleContentDblclick"
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
            <div v-if="props.item?.quote?.id" class="border-l-4 pl-2 border-blue-500 mb-2">
              <template v-if="(props.item as any)?.quote?.is_deleted">
                <span class="text-gray-400">此消息已删除</span>
              </template>
              <template v-else-if="props.item?.quote?.is_revoked">
                <span class="text-gray-400">此消息已撤回</span>
              </template>
              <template v-else>
                <div v-if="quoteWhisperLabel" class="whisper-label whisper-label--quote">
                  <n-icon :component="Lock" size="14" />
                  <span>{{ quoteWhisperLabel }}</span>
                </div>
                <span class="text-gray-500">
                  <component :is="parseContent(props.item?.quote)" />
                </span>
              </template>
            </div>
            <component :is="parseContent(props, displayContent)" />
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
  width: 3rem;
  height: 3rem;
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
  width: 2.75rem;
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
  background-color: #f3f4f6;
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
  --editing-preview-bg: #000000;
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
  font-size: 0.9rem;
  line-height: 1.5;
}

.editing-preview__body {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.9rem;
  line-height: 1.5;
  color: inherit;
}

.editing-preview__rich {
  word-break: break-word;
  white-space: normal;
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
</style>
