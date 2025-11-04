<script setup lang="tsx">
import dayjs from 'dayjs';
import Element from '@satorijs/element'
import { onMounted, ref, h, computed, watch, PropType, onBeforeUnmount } from 'vue';
import { urlBase } from '@/stores/_config';
import DOMPurify from 'dompurify';
import { useUserStore } from '@/stores/user';
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import { Howl, Howler } from 'howler';
import { useMessage } from 'naive-ui';
import Avatar from '@/components/avatar.vue'
import { Lock, Edit } from '@vicons/tabler';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render';
import { onLongPress } from '@vueuse/core';

type EditingPreviewInfo = {
  userId: string;
  displayName: string;
  avatar?: string;
  content: string;
  indicatorOnly: boolean;
  isSelf: boolean;
  summary: string;
  previewHtml: string;
};

const user = useUserStore();
const chat = useChatStore();
const utils = useUtilsStore();
const { t } = useI18n();

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

const parseContent = (payload: any, overrideContent?: string) => {
  const content = overrideContent ?? payload?.content ?? '';

  // 检测是否为 TipTap JSON 格式
  if (isTipTapJson(content)) {
    try {
      const html = tiptapJsonToHtml(content, {
        baseUrl: urlBase,
        imageClass: 'inline-image',
        linkClass: 'text-blue-500',
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
        if (item.attrs.src && item.attrs.src.startsWith('id:')) {
          item.attrs.src = item.attrs.src.replace('id:', `${urlBase}/api/v1/attachments/`);
        }
        textItems.push(DOMPurify.sanitize(item.toString()));
        hasImage.value = true;
        break;
      case 'audio':
        let src = ''
        if (!item.attrs.src) break;

        src = item.attrs.src;
        if (item.attrs.src.startsWith('id:')) {
          src = item.attrs.src.replace('id:', `${urlBase}/api/v1/attachments/`);
        }

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
      default:
        textItems.push(`<span style="white-space: pre-wrap">${item.toString()}</span>`);
        break;
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

const props = defineProps({
  username: String,
  content: String,
  avatar: String,
  isRtl: Boolean,
  item: Object,
  identityColor: String,
  editingPreview: Object as PropType<EditingPreviewInfo | undefined>,
})

const timeText = ref(timeFormat(props.item?.createdAt));
const timeText2 = ref(timeFormat2(props.item?.createdAt));
const editedTimeText = ref(props.item?.isEdited ? timeFormat(props.item?.updatedAt) : '');
const editedTimeText2 = ref(props.item?.isEdited ? timeFormat2(props.item?.updatedAt) : '');

const getMemberDisplayName = (item: any) => item?.identity?.displayName || item?.sender_member_name || item?.member?.nick || item?.user?.nick || item?.user?.name || '未知成员';
const getTargetDisplayName = (item: any) => item?.whisperTo?.nick || item?.whisperTo?.name || '未知成员';

const buildWhisperLabel = (item?: any) => {
  if (!item?.isWhisper) return '';
  const senderName = getMemberDisplayName(item);
  const targetName = getTargetDisplayName(item);
  const senderLabel = `@${senderName}`;
  const targetLabel = `@${targetName}`;
  if (item?.user?.id === user.info.id) {
    return t('whisper.sendTo', { target: targetLabel });
  }
  if (item?.whisperTo?.id === user.info.id) {
    return t('whisper.from', { sender: senderLabel });
  }
  if (item?.whisperTo?.nick || item?.whisperTo?.name) {
    return t('whisper.sendTo', { target: targetLabel });
  }
  return t('whisper.generic');
};

const whisperLabel = computed(() => buildWhisperLabel(props.item));
const quoteWhisperLabel = computed(() => buildWhisperLabel((props.item as any)?.quote));

const isEditing = computed(() => chat.isEditingMessage(props.item?.id));
const canEdit = computed(() => props.item?.user?.id === user.info.id);

const displayContent = computed(() => {
  if (isEditing.value && chat.editing) {
    return chat.editing.draft;
  }
  return props.item?.content ?? props.content ?? '';
});

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

onMounted(() => {
  stopMessageLongPress = onLongPress(
    messageContentRef,
    (event) => {
      event.preventDefault?.();
      onMessageLongPress(event, props.item);
    },
    { modifiers: { prevent: true } }
  );

  setInterval(() => {
    timeText.value = timeFormat(props.item?.createdAt);
    timeText2.value = timeFormat2(props.item?.createdAt);
    if (props.item?.isEdited) {
      editedTimeText.value = timeFormat(props.item?.updatedAt);
      editedTimeText2.value = timeFormat2(props.item?.updatedAt);
    }
  }, 10000);
})

onBeforeUnmount(() => {
  if (stopMessageLongPress) {
    stopMessageLongPress();
    stopMessageLongPress = null;
  }
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
  <div v-if="item?.is_revoked" class="py-4 text-center">一条消息已被撤回</div>
  <div v-else :id="item?.id" class="chat-item" :style="props.isRtl ? { direction: 'rtl' } : {}"
    :class="[{ 'is-rtl': props.isRtl }, { 'is-editing': isEditing }]">
    <Avatar :src="props.avatar" @longpress="emit('avatar-longpress')" @click="doAvatarClick" />
    <!-- <img class="rounded-md w-12 h-12 border-gray-500 border" :src="props.avatar" /> -->
    <!-- <n-avatar :src="imgAvatar" size="large" bordered>海豹</n-avatar> -->
    <div class="right">
      <span class="title">
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
      <div class="content break-all relative" ref="messageContentRef" @contextmenu="onContextMenu($event, item)"
        :class="[{ 'whisper-content': props.item?.isWhisper }, { 'content--editing-preview': !!props.editingPreview }]">
        <div v-if="canEdit && !(props.editingPreview && props.editingPreview.isSelf)" class="message-action-bar"
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
        <template v-if="!props.editingPreview">
          <div>
            <div v-if="whisperLabel" class="whisper-label">
              <n-icon :component="Lock" size="16" />
              <span>{{ whisperLabel }}</span>
            </div>
            <div v-if="props.item?.quote?.id" class="border-l-4 pl-2 border-blue-500 mb-2">
              <template v-if="props.item?.quote?.is_revoked">
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
        </template>
        <template v-else>
          <div class="editing-preview__bubble editing-preview__bubble--inline">
            <div class="editing-preview__header">
              <span class="editing-preview__name">{{ props.editingPreview.displayName || '未知成员' }}</span>
              <span class="editing-preview__tag">正在编辑</span>
            </div>
          <div class="editing-preview__body" :class="{ 'is-placeholder': props.editingPreview.indicatorOnly }">
            <template v-if="props.editingPreview.indicatorOnly">
              正在更新内容...
            </template>
            <template v-else>
              <div
                v-if="props.editingPreview.previewHtml"
                class="editing-preview__rich"
                v-html="props.editingPreview.previewHtml"
              ></div>
              <span v-else>{{ props.editingPreview.summary || '[图片]' }}</span>
            </template>
          </div>
          <div v-if="props.editingPreview.isSelf" class="editing-preview__actions">
            <n-button size="tiny" type="primary" @click.stop="handleEditSave">保存</n-button>
            <n-button size="tiny" text @click.stop="handleEditCancel">取消</n-button>
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
  @apply flex;

  >.n-avatar {
    @apply rounded-md;
  }

  &.is-rtl {
    >.right {
      @apply mr-4;

      >.title {
        @apply justify-end;
      }

      >.content {
        &>.failed {
          left: -2rem;
          right: auto;
          top: 0;
        }

        &:before {
          display: none;
        }

        &::after {
          position: absolute;
          top: 0.5rem;
          height: 0.75rem;
          width: 0.75rem;
          background-color: inherit;
          content: "";
          right: -0.75rem;
          transform: scaleY(-1) scaleX(-1);
          mask-size: contain;
          mask-image: url("data:image/svg+xml,%3csvg width='3' height='3' xmlns='http://www.w3.org/2000/svg'%3e%3cpath fill='black' d='m 0 3 L 3 3 L 3 0 C 3 1 1 3 0 3'/%3e%3c/svg%3e");
        }
      }
    }
  }

  >.right {
    @apply ml-4;

    >.title {
      display: flex;
      gap: 0.5rem;
      direction: ltr;

      >.name {
        @apply font-semibold;
      }

      >.time {
        @apply text-gray-400;
      }
    }

    >.content {
      &>.failed {
        right: -2rem;
        top: 0;
      }

      &:before {
        position: absolute;
        top: 0.5rem;
        height: 0.75rem;
        width: 0.75rem;
        background-color: inherit;
        content: "";
        left: -0.75rem;
        transform: scaleY(-1);
        mask-size: contain;
        mask-image: url("data:image/svg+xml,%3csvg width='3' height='3' xmlns='http://www.w3.org/2000/svg'%3e%3cpath fill='black' d='m 0 3 L 3 3 L 3 0 C 3 1 1 3 0 3'/%3e%3c/svg%3e");
      }

  width: fit-content;
  direction: ltr;
  @apply text-base mt-1 px-4 py-2 relative;
  @apply rounded bg-gray-200 text-gray-900;
      min-width: 0;
        transition: box-shadow 0.2s ease, border-color 0.2s ease;
      }

      >.content.whisper-content {
    background: linear-gradient(135deg, #1f2937 0%, #312e81 100%);
    color: #f4f4ff;
      border: 1px solid rgba(129, 140, 248, 0.4);
      box-shadow: inset 0 0 0 1px rgba(30, 64, 175, 0.25);
    }

    >.content.whisper-content .text-gray-500 {
      color: #e0e7ff;
    }

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
  margin-top: 0.5rem;
}
}


.chat-item.is-editing > .right > .content {
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.35);
  border: 1px dashed rgba(37, 99, 235, 0.65);
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
}

.chat-item .content:hover .message-action-bar,
.chat-item.is-editing .message-action-bar,
.chat-item .message-action-bar--active {
  opacity: 1;
  pointer-events: auto;
}

.content--editing-preview {
  background: transparent;
  border: none;
  box-shadow: none;
  padding: 0;
}

.content--editing-preview.whisper-content {
  background: transparent;
}

.content--editing-preview .editing-preview__bubble--inline {
  border: 1px dashed rgba(59, 130, 246, 0.55);
  background-color: rgba(219, 234, 254, 0.92);
  color: #1d4ed8;
  border-radius: 0.75rem;
  padding: 0.55rem 0.75rem;
  max-width: 32rem;
  box-shadow: 0 4px 10px rgba(59, 130, 246, 0.12);
}

.editing-preview__bubble {
  border: 1px dashed rgba(59, 130, 246, 0.55);
  background-color: rgba(219, 234, 254, 0.9);
  color: #1d4ed8;
  border-radius: 0.75rem;
  padding: 0.55rem 0.75rem;
  max-width: 32rem;
  box-shadow: 0 4px 10px rgba(59, 130, 246, 0.12);
}

.editing-preview__header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.35rem;
}

.editing-preview__name {
  font-size: 0.75rem;
  font-weight: 600;
}

.editing-preview__tag {
  font-size: 0.625rem;
  padding: 0.1rem 0.4rem;
  border-radius: 9999px;
  background-color: rgba(59, 130, 246, 0.18);
}

.editing-preview__body {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.9rem;
  color: #1e3a8a;
}

.editing-preview__rich {
  word-break: break-word;
  white-space: normal;
}

.editing-preview__body.is-placeholder {
  color: #4b5563;
}

.editing-preview__actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 0.45rem;
}

.whisper-label {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.01em;
  color: #1f2937;
  background: rgba(67, 56, 202, 0.08);
  border-radius: 9999px;
  padding: 0.12rem 0.65rem;
  margin-bottom: 0.4rem;
}

.whisper-label svg {
  color: inherit;
}

.whisper-label--quote {
  font-size: 0.7rem;
  color: #4338ca;
  margin-bottom: 0.25rem;
}

.whisper-content .whisper-label,
.whisper-content .whisper-label--quote {
  color: #f4f4ff;
  background: rgba(129, 140, 248, 0.22);
}

.whisper-content .whisper-label--quote {
  color: #d6bcfa;
}

.whisper-content .whisper-label svg {
  color: #ede9fe;
}

.whisper-content .text-gray-400 {
  color: #e0e7ff;
}
</style>
