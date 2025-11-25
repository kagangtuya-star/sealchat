<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import { useChatStore } from '@/stores/chat';
import { computed } from 'vue';
import Element from '@satorijs/element'
import { useDialog, useMessage, useThemeVars } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { useDisplayStore } from '@/stores/display';

const chat = useChatStore()
const message = useMessage()
const dialog = useDialog()
const themeVars = useThemeVars()
const display = useDisplayStore()
const { t } = useI18n();
const user = useUserStore()

const menuMessage = computed(() => {
  const raw = chat.messageMenu.item as any;
  if (!raw) {
    return {
      raw: null,
      author: null,
      member: null,
    };
  }

  const memberUser: User | undefined = raw.member?.user || raw.member?.userInfo;
  const author: User | null = raw.user || memberUser || raw.author || null;

  return {
    raw,
    author,
    member: raw.member,
  };
});

const detectContentMode = (content?: string): 'plain' | 'rich' => {
  if (!content) {
    return 'plain';
  }
  if (isTipTapJson(content)) {
    return 'rich';
  }
  const trimmed = content.trim();
  if (!trimmed) {
    return 'plain';
  }
  const containsRich = /<(p|span|at|strong|em|blockquote|ul|ol|li|code|pre|a)\b/i.test(trimmed);
  const onlyImagesOrText = /^(?:\s*(<img\b[^>]*>))*\s*$/.test(trimmed);
  return containsRich && !onlyImagesOrText ? 'rich' : 'plain';
};

const resolveWhisperTargetId = (msg?: any): string | null => {
  if (!msg) {
    return null;
  }
  const metaId = msg?.whisperMeta?.targetUserId;
  if (metaId) {
    return metaId;
  }
  const camel = msg?.whisperTo;
  if (typeof camel === 'string') {
    return camel;
  }
  if (camel && typeof camel === 'object' && camel.id) {
    return camel.id;
  }
  const snake = msg?.whisper_to;
  if (typeof snake === 'string') {
    return snake;
  }
  if (snake && typeof snake === 'object' && snake.id) {
    return snake.id;
  }
  const target = msg?.whisper_target;
  if (target && typeof target === 'object' && target.id) {
    return target.id;
  }
  return null;
};

const resolveIdentityId = (msg?: any): string | null => {
  if (!msg) {
    return null;
  }
  const direct = msg.identity || msg.identity_info || msg.identityData;
  if (direct && typeof direct === 'object' && direct.id) {
    return direct.id;
  }
  const camelRole = msg?.senderRoleId || msg?.senderRoleID;
  if (typeof camelRole === 'string' && camelRole.trim().length > 0) {
    return camelRole;
  }
  const snakeRole = msg?.sender_role_id;
  if (typeof snakeRole === 'string' && snakeRole.trim().length > 0) {
    return snakeRole;
  }
  const memberIdentity = msg?.member?.identity;
  if (memberIdentity && typeof memberIdentity === 'object' && memberIdentity.id) {
    return memberIdentity.id;
  }
  return null;
};

const isSelfMessage = computed(() => {
  const authorId = menuMessage.value.author?.id;
  if (!authorId) {
    return false;
  }
  return authorId === user.info.id;
});

const canWhisper = computed(() => {
  const authorId = menuMessage.value.author?.id;
  if (!authorId) {
    return false;
  }
  return authorId !== user.info.id;
});

const resolveUserId = (raw: any): string => {
  return (
    raw?.id ||
    raw?.user?.id ||
    raw?.member?.user?.id ||
    raw?.member?.user_id ||
    raw?.member?.userId ||
    raw?.user_id ||
    ''
  );
};

const channelId = computed(() => chat.curChannel?.id || '');
const currentUserId = computed(() => user.info.id);
const targetUserId = computed(() => {
  if (!menuMessage.value.raw) {
    return '';
  }
  return menuMessage.value.author?.id || resolveUserId(menuMessage.value.raw);
});

const isArchivedMessage = computed(() => {
  const raw: any = menuMessage.value.raw;
  if (!raw) {
    return false;
  }
  return Boolean(raw.isArchived ?? raw.is_archived ?? false);
});

const viewerIsAdmin = computed(() => {
  if (!channelId.value) {
    return false;
  }
  return chat.isChannelAdmin(channelId.value, currentUserId.value);
});

const targetIsAdmin = computed(() => {
  if (!channelId.value || !targetUserId.value) {
    return false;
  }
  return chat.isChannelAdmin(channelId.value, targetUserId.value);
});

const canArchiveByRule = computed(() => {
  if (!menuMessage.value.raw || !channelId.value || !targetUserId.value) {
    return false;
  }
  if (targetUserId.value === currentUserId.value) {
    return true;
  }
  if (!viewerIsAdmin.value) {
    return false;
  }
  if (targetIsAdmin.value) {
    return false;
  }
  return true;
});

const showArchiveAction = computed(() => !isArchivedMessage.value && canArchiveByRule.value);
const showUnarchiveAction = computed(() => isArchivedMessage.value && canArchiveByRule.value);
const canRemoveMessage = computed(() => {
  if (!menuMessage.value.raw || !channelId.value || !targetUserId.value) {
    return false;
  }
  if (isSelfMessage.value) {
    return true;
  }
  if (!viewerIsAdmin.value) {
    return false;
  }
  if (targetIsAdmin.value) {
    return false;
  }
  return true;
});

const clickArchive = async () => {
  if (!canArchiveByRule.value) {
    return;
  }
  const targetId = menuMessage.value.raw?.id;
  if (!channelId.value || !targetId) {
    return;
  }
  try {
    await chat.archiveMessages([targetId]);
    const raw: any = menuMessage.value.raw;
    if (raw) {
      raw.isArchived = true;
      raw.is_archived = true;
    }
    message.success('消息已归档');
  } catch (error) {
    const errMsg = (error as Error)?.message || '归档失败';
    message.error(errMsg);
  } finally {
    chat.messageMenu.show = false;
  }
};

const clickUnarchive = async () => {
  if (!canArchiveByRule.value) {
    return;
  }
  const targetId = menuMessage.value.raw?.id;
  if (!channelId.value || !targetId) {
    return;
  }
  try {
    await chat.unarchiveMessages([targetId]);
    const raw: any = menuMessage.value.raw;
    if (raw) {
      raw.isArchived = false;
      raw.is_archived = false;
    }
    message.success('消息已取消归档');
  } catch (error) {
    const errMsg = (error as Error)?.message || '取消归档失败';
    message.error(errMsg);
  } finally {
    chat.messageMenu.show = false;
  }
};

const clickReplyTo = () => {
  if (!menuMessage.value.raw) {
    return;
  }
  chat.setReplayTo(menuMessage.value.raw);
}

const clickDelete = async () => {
  if (!chat.curChannel?.id || !menuMessage.value.raw?.id) {
    return;
  }
  await chat.messageDelete(chat.curChannel.id, menuMessage.value.raw.id)
  message.success('撤回成功')
  chat.messageMenu.show = false;
}

const performRemove = async () => {
  if (!chat.curChannel?.id || !menuMessage.value.raw?.id) {
    return;
  }
  try {
    await chat.messageRemove(chat.curChannel.id, menuMessage.value.raw.id);
    message.success('删除成功');
  } catch (error) {
    const errMsg = (error as Error)?.message || '删除失败';
    message.error(errMsg);
  } finally {
    chat.messageMenu.show = false;
  }
};

const clickRemove = () => {
  if (!canRemoveMessage.value) {
    return;
  }
  dialog.warning({
    title: '删除消息',
    content: '删除后所有成员将无法再看到该消息，并且无法恢复，确定继续？',
    positiveText: '删除',
    negativeText: '取消',
    iconPlacement: 'top',
    contentStyle: {
      color: themeVars.value.textColor2,
    },
    maskClosable: false,
    onPositiveClick: async () => {
      await performRemove();
    },
  });
};

const clickEdit = () => {
  if (!chat.curChannel?.id || !menuMessage.value.raw?.id) {
    return;
  }
  const target = menuMessage.value.raw;
  const mode = detectContentMode(target.content || target.originalContent || '');
  const whisperTargetId = resolveWhisperTargetId(target);
  const identityId = resolveIdentityId(target);
  const icMode = String(target.icMode ?? target.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic';
  chat.startEditingMessage({
    messageId: target.id,
    channelId: chat.curChannel.id,
    originalContent: target.content || '',
    draft: target.content || '',
    mode,
    isWhisper: Boolean(target.isWhisper ?? target.is_whisper),
    whisperTargetId,
    icMode,
    identityId: identityId || null,
  });
  chat.messageMenu.show = false;
}

const clickCopy = async () => {
  const content = menuMessage.value.raw?.content || '';
  let copyText = '';
  if (detectContentMode(content) === 'rich') {
    try {
      const json = JSON.parse(content);
      copyText = tiptapJsonToPlainText(json);
    } catch (error) {
      console.warn('富文本解析失败，回退为纯文本复制', error);
      copyText = '';
    }
  } else {
    const items = Element.parse(content);
    for (const item of items) {
      if (item.type === 'text') {
        copyText += item.toString();
      }
    }
  }

  try {
    await navigator.clipboard.writeText(copyText);
    message.success("已复制");
  } catch (err) {
    message.error('复制失败');
  }
}

const addToMyEmoji = async () => {
  const items = Element.parse(menuMessage.value.raw?.content || '');
  for (let item of items) {
    if (item.type == "img") {
      const id = item.attrs.src.replace('id:', '');
      try {
        await user.emojiAdd(id);
        message.success('收藏成功');
      } catch (e: any) {
        if (e.name === "ConstraintError") {
          message.error('该表情已经存在于收藏了');
        }
      }
    }
  }
}

const clickWhisper = () => {
  const targetAuthor = menuMessage.value.author;
  if (!targetAuthor?.id) {
    message.warning(t('whisper.userUnknown'));
    return;
  }
  if (targetAuthor.id === user.info.id) {
    message.warning(t('whisper.selfNotAllowed'));
    return;
  }
  const memberInfo = menuMessage.value.member;
  const targetUser: User = {
    id: targetAuthor.id,
    name: targetAuthor.name || (targetAuthor as any).username || '',
    nick: memberInfo?.nick || targetAuthor.nick || targetAuthor.name || '未知成员',
    avatar: memberInfo?.avatar || targetAuthor.avatar || '',
    discriminator: targetAuthor.discriminator || '',
    is_bot: !!targetAuthor.is_bot,
  };
  chat.setWhisperTarget(targetUser);
  chat.messageMenu.show = false;
};

</script>

<template>
  <context-menu
    v-model:show="chat.messageMenu.show"
    :options="{
      ...chat.messageMenu.optionsComponent,
      theme: 'dark',
      // 结合夜间模式使用半透明背景与浅色边框，避免默认亮色影响
      customClass: display.palette === 'night' ? 'chat-menu--night' : 'chat-menu--day'
    } as MenuOptions">
    <context-menu-item v-if="chat.messageMenu.hasImage" label="添加到表情收藏" @click="addToMyEmoji" />
    <context-menu-item v-if="!chat.messageMenu.hasImage" label="复制内容" @click="clickCopy" />
    <context-menu-item v-if="canWhisper" :label="t('whisper.menu')" @click="clickWhisper" />
    <context-menu-item label="回复" @click="clickReplyTo" />
    <context-menu-item v-if="showArchiveAction" label="归档" @click="clickArchive" />
    <context-menu-item v-if="showUnarchiveAction" label="取消归档" @click="clickUnarchive" />
    <context-menu-item label="编辑消息" @click="clickEdit" v-if="isSelfMessage" />
    <context-menu-item label="撤回" @click="clickDelete" v-if="isSelfMessage" />
    <context-menu-item label="删除" @click="clickRemove" v-if="canRemoveMessage" />
  </context-menu>
</template>

<style scoped>
:deep(.context-menu.chat-menu--night) {
  background: rgba(15, 23, 42, 0.95);
  border-color: rgba(148, 163, 184, 0.35);
  color: #e2e8f0;
}

:deep(.context-menu.chat-menu--night .context-menu-item) {
  color: inherit;
}

:deep(.context-menu.chat-menu--night .context-menu-item:hover) {
  background: rgba(255, 255, 255, 0.08);
}
</style>
