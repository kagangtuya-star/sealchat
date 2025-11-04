<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import { useChatStore } from '@/stores/chat';
import { computed } from 'vue';
import Element from '@satorijs/element'
import { useMessage } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render';

const chat = useChatStore()
const message = useMessage()
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
  if (/<(p|img|br|span|at|strong|em|blockquote|ul|ol|li|code|pre|a)\b/i.test(trimmed)) {
    return 'rich';
  }
  return 'plain';
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
}

const clickEdit = () => {
  if (!chat.curChannel?.id || !menuMessage.value.raw?.id) {
    return;
  }
  const target = menuMessage.value.raw;
  const mode = detectContentMode(target.content || target.originalContent || '');
  chat.startEditingMessage({
    messageId: target.id,
    channelId: chat.curChannel.id,
    originalContent: target.content || '',
    draft: target.content || '',
    mode,
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
  <context-menu v-model:show="chat.messageMenu.show" :options="chat.messageMenu.optionsComponent">
    <context-menu-item v-if="chat.messageMenu.hasImage" label="添加到表情收藏" @click="addToMyEmoji" />
    <context-menu-item v-if="!chat.messageMenu.hasImage" label="复制内容" @click="clickCopy" />
    <context-menu-item v-if="canWhisper" :label="t('whisper.menu')" @click="clickWhisper" />
    <context-menu-item label="回复" @click="clickReplyTo" />
    <context-menu-item label="编辑消息" @click="clickEdit" v-if="isSelfMessage" />
    <context-menu-item label="撤回" @click="clickDelete" v-if="isSelfMessage" />
  </context-menu>
</template>
