<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import { computed, ref, defineAsyncComponent } from 'vue';
import Element from '@satorijs/element'
import { useDialog, useMessage, useThemeVars } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { useGalleryStore } from '@/stores/gallery';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { useDisplayStore } from '@/stores/display';
import { generateMessageLink } from '@/utils/messageLink';
import { copyTextWithFallback } from '@/utils/clipboard';
const ReactionQuickPicker = defineAsyncComponent(() => import('./ReactionQuickPicker.vue'));
const EmojiPickerModal = defineAsyncComponent(() => import('./EmojiPickerModal.vue'));

const chat = useChatStore()
const utils = useUtilsStore()
const message = useMessage()
const dialog = useDialog()
const themeVars = useThemeVars()
const display = useDisplayStore()
const { t } = useI18n();
const user = useUserStore()
const gallery = useGalleryStore()

const showEmojiPicker = ref(false);

const contextMenuClass = computed(() => (display.palette === 'night' ? 'chat-menu--night' : 'chat-menu--day'))
const contextMenuTheme = computed(() => (display.palette === 'night' ? 'default dark' : 'default'))
const contextMenuOptions = computed<MenuOptions>(() => ({
  ...chat.messageMenu.optionsComponent,
  theme: contextMenuTheme.value,
  customClass: contextMenuClass.value,
}))

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
  const metaIds = msg?.whisperMeta?.targetUserIds;
  if (Array.isArray(metaIds) && metaIds.length > 0) {
    return String(metaIds[0]);
  }
  const metaId = msg?.whisperMeta?.targetUserId;
  if (metaId) {
    return metaId;
  }
  const list = msg?.whisperToIds || msg?.whisper_to_ids || msg?.whisperTargets || msg?.whisper_targets;
  if (Array.isArray(list) && list.length > 0) {
    const first = list[0];
    if (typeof first === 'string') {
      return first;
    }
    if (first && typeof first === 'object' && first.id) {
      return first.id;
    }
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

const canEdit = computed(() => {
  if (isSelfMessage.value) return true;
  if (!targetUserId.value) return false;
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
      if (targetIsAdmin.value) {
        return false;
      }
      return true;
    }
  }
  return false;
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
const isPinnedMessage = computed(() => {
  const raw: any = menuMessage.value.raw;
  if (!raw) {
    return false;
  }
  return Boolean(raw.isPinned ?? raw.is_pinned ?? false);
});
const resolveRoleRank = (uid: string): number => {
  if (!uid || !channelId.value) {
    return 0;
  }
  if (chat.isChannelOwner(channelId.value, uid)) {
    return 4;
  }
  const roleIds = chat.channelMemberRoleMap[channelId.value]?.[uid] || [];
  if (!Array.isArray(roleIds) || roleIds.length === 0) {
    return 0;
  }
  let rank = 0;
  roleIds.forEach((roleId) => {
    if (typeof roleId !== 'string') {
      return;
    }
    if (roleId.endsWith('-owner')) {
      rank = Math.max(rank, 4);
    } else if (roleId.endsWith('-admin')) {
      rank = Math.max(rank, 3);
    } else if (roleId.endsWith('-member')) {
      rank = Math.max(rank, 2);
    } else if (roleId.endsWith('-spectator')) {
      rank = Math.max(rank, 1);
    }
  });
  return rank;
};
const viewerRoleRank = computed(() => resolveRoleRank(currentUserId.value));
const pinnerUserId = computed(() => {
  const raw: any = menuMessage.value.raw;
  return String(raw?.pinnedBy ?? raw?.pinned_by ?? '').trim();
});
const pinnerRoleRank = computed(() => resolveRoleRank(pinnerUserId.value));
const canPinByRule = computed(() => {
  if (!menuMessage.value.raw || !channelId.value) {
    return false;
  }
  if (isPinnedMessage.value) {
    return false;
  }
  return viewerRoleRank.value >= 2;
});
const canUnpinByRule = computed(() => {
  if (!menuMessage.value.raw || !channelId.value) {
    return false;
  }
  if (viewerRoleRank.value < 2) {
    return false;
  }
  if (!isPinnedMessage.value) {
    return false;
  }
  if (pinnerUserId.value && pinnerUserId.value === currentUserId.value) {
    return true;
  }
  return viewerRoleRank.value > pinnerRoleRank.value;
});
const canRemoveMessage = computed(() => {
  if (!menuMessage.value.raw || !channelId.value || !targetUserId.value) {
    return false;
  }
  if (isSelfMessage.value) {
    return true;
  }
  const worldId = chat.currentWorldId;
  const worldDetail = chat.worldDetailMap[worldId];
  const ownerId = worldDetail?.world?.ownerId || chat.worldMap[worldId]?.ownerId;
  const isWorldAdmin = worldDetail?.memberRole === 'owner' || worldDetail?.memberRole === 'admin' || ownerId === user.info.id;
  if (!viewerIsAdmin.value && !isWorldAdmin) {
    return false;
  }
  if (ownerId && targetUserId.value === ownerId) {
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

const clickPin = async () => {
  if (!canPinByRule.value) {
    return;
  }
  const targetId = menuMessage.value.raw?.id;
  if (!channelId.value || !targetId) {
    return;
  }
  try {
    await chat.pinMessages([targetId]);
    const raw: any = menuMessage.value.raw;
    if (raw) {
      raw.isPinned = true;
      raw.is_pinned = true;
      raw.pinnedBy = currentUserId.value;
      raw.pinned_by = currentUserId.value;
      raw.pinnedAt = Date.now();
      raw.pinned_at = raw.pinnedAt;
    }
    message.success('消息已置顶');
  } catch (error) {
    const errMsg = (error as Error)?.message || '置顶失败';
    message.error(errMsg);
  } finally {
    chat.messageMenu.show = false;
  }
};

const clickUnpin = async () => {
  if (!canUnpinByRule.value) {
    return;
  }
  const targetId = menuMessage.value.raw?.id;
  if (!channelId.value || !targetId) {
    return;
  }
  try {
    await chat.unpinMessages([targetId]);
    const raw: any = menuMessage.value.raw;
    if (raw) {
      raw.isPinned = false;
      raw.is_pinned = false;
      raw.pinnedBy = '';
      raw.pinned_by = '';
      raw.pinnedAt = undefined;
      raw.pinned_at = undefined;
    }
    message.success('消息已取消置顶');
  } catch (error) {
    const errMsg = (error as Error)?.message || '取消置顶失败';
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

const handleQuickReaction = async (emoji: string) => {
  const messageId = menuMessage.value.raw?.id;
  if (!messageId) {
    return;
  }
  chat.messageMenu.show = false;
  try {
    await chat.addReaction(messageId, emoji);
  } catch (error) {
    const errMsg = (error as Error)?.message || '添加反应失败';
    message.error(errMsg);
  }
};

const openFullEmojiPicker = () => {
  chat.messageMenu.show = false;
  showEmojiPicker.value = true;
};

const handleFullEmojiSelect = async (emoji: string) => {
  const messageId = menuMessage.value.raw?.id;
  if (!messageId) {
    return;
  }
  showEmojiPicker.value = false;
  try {
    await chat.addReaction(messageId, emoji);
  } catch (error) {
    const errMsg = (error as Error)?.message || '添加反应失败';
    message.error(errMsg);
  }
};

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

const clickResizeImage = () => {
  if (!chat.curChannel?.id || !menuMessage.value.raw?.id) {
    return;
  }
  const target = menuMessage.value.raw;
  chatEvent.emit('message-image-resize-enter' as any, {
    messageId: target.id,
    channelId: chat.curChannel.id,
  } as any);
  chat.messageMenu.show = false;
};

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

  const copied = await copyTextWithFallback(copyText);
  if (copied) {
    message.success("已复制");
  } else {
    message.error('复制失败');
  }
}

const addToMyEmoji = async () => {
  const items = Element.parse(menuMessage.value.raw?.content || '');
  for (let item of items) {
    if (item.type == "img") {
      const id = item.attrs.src.replace('id:', '');
      try {
        await gallery.addEmoji(id, user.info.id);
        message.success('收藏成功');
      } catch (e: any) {
        const errMsg = e?.response?.data?.message || e?.message || '收藏失败';
        if (errMsg.includes('已存在') || e.name === "ConstraintError") {
          message.error('该表情已经存在于收藏了');
        } else {
          message.error(errMsg);
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
  chat.clearWhisperTargets();
  chat.toggleWhisperTarget(targetUser);
  chat.confirmWhisperTargets();
  chat.messageMenu.show = false;
};

const clickMultiSelect = () => {
  const targetId = menuMessage.value.raw?.id;
  if (targetId) {
    chat.enterMultiSelectMode(targetId);
  }
  chat.messageMenu.show = false;
};

const clickCopyMessageLink = async () => {
  const msgId = menuMessage.value.raw?.id;
  const curChannelId = chat.curChannel?.id;
  const worldId = chat.currentWorldId;

  if (!msgId || !curChannelId || !worldId) {
    message.warning('无法生成消息链接');
    return;
  }

  const linkBase = (() => {
    const domain = utils.config?.domain?.trim() || '';
    if (!domain) return '';
    const webUrl = utils.config?.webUrl?.trim() || '';
    let base = domain;
    if (!/^(https?:)?\/\//i.test(base)) {
      base = `${window.location.protocol}//${base}`;
    }
    if (webUrl) {
      base = `${base}${webUrl.startsWith('/') ? '' : '/'}${webUrl}`;
    }
    return base;
  })();

  const link = generateMessageLink(
    {
      worldId,
      channelId: curChannelId,
      messageId: msgId,
    },
    linkBase ? { base: linkBase } : undefined,
  );

  const copied = await copyTextWithFallback(link);
  if (copied) {
    message.success('消息链接已复制');
  } else {
    message.error('复制失败');
  }

  chat.messageMenu.show = false;
};

</script>

<template>
  <context-menu
    v-model:show="chat.messageMenu.show"
    :options="contextMenuOptions">
    <div v-if="chat.messageMenu.show" class="reaction-picker-slot">
      <ReactionQuickPicker
        @select="handleQuickReaction"
        @expand="openFullEmojiPicker"
      />
    </div>
    <context-menu-item v-if="chat.messageMenu.hasImage" label="添加到表情收藏" @click="addToMyEmoji" />
    <context-menu-item v-if="!chat.messageMenu.hasImage" label="复制内容" @click="clickCopy" />
    <context-menu-item label="复制消息链接" @click="clickCopyMessageLink" />
    <context-menu-item v-if="canWhisper" :label="t('whisper.menu')" @click="clickWhisper" />
    <context-menu-item label="回复" @click="clickReplyTo" />
    <context-menu-item v-if="canPinByRule" label="置顶消息" @click="clickPin" />
    <context-menu-item v-if="canUnpinByRule" label="取消置顶" @click="clickUnpin" />
    <context-menu-item v-if="showArchiveAction" label="归档" @click="clickArchive" />
    <context-menu-item v-if="showUnarchiveAction" label="取消归档" @click="clickUnarchive" />
    <context-menu-item label="编辑消息" @click="clickEdit" v-if="canEdit" />
    <context-menu-item label="调整图片大小" @click="clickResizeImage" v-if="chat.messageMenu.hasImage && canEdit" />
    <context-menu-item label="多选" @click="clickMultiSelect" />
    <context-menu-item label="撤回" @click="clickDelete" v-if="isSelfMessage" />
    <context-menu-item label="删除" @click="clickRemove" v-if="canRemoveMessage" />
  </context-menu>

  <EmojiPickerModal
    v-if="showEmojiPicker"
    @select="handleFullEmojiSelect"
    @close="showEmojiPicker = false"
  />
</template>

<style scoped>
.reaction-picker-slot {
  padding: 6px 8px;
}

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

:deep(.context-menu.chat-menu--day) {
  background: rgba(248, 250, 252, 0.98);
  border-color: rgba(15, 23, 42, 0.08);
  color: #0f172a;
}

:deep(.context-menu.chat-menu--day .context-menu-item) {
  color: inherit;
}

:deep(.context-menu.chat-menu--day .context-menu-item:hover) {
  background: rgba(15, 23, 42, 0.06);
}

:root[data-custom-theme='true'] :deep(.context-menu.chat-menu--night),
:root[data-custom-theme='true'] :deep(.context-menu.chat-menu--day) {
  background: var(--sc-bg-surface);
  border-color: var(--sc-border-strong);
  color: var(--sc-text-primary);
}

:root[data-custom-theme='true'] :deep(.context-menu.chat-menu--night .context-menu-item:hover),
:root[data-custom-theme='true'] :deep(.context-menu.chat-menu--day .context-menu-item:hover) {
  background: var(--sc-bg-layer);
}
</style>
