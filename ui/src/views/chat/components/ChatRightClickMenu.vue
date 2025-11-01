<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import { useChatStore } from '@/stores/chat';
import { computed } from 'vue';
import Element from '@satorijs/element'
import { useMessage } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';

const chat = useChatStore()
const message = useMessage()
const { t } = useI18n();

const clickReplyTo = async () => {
  chat.setReplayTo(chat.messageMenu.item)
}

const user = useUserStore()

const clickDelete = async () => {
  if (chat.curChannel?.id && chat.messageMenu.item?.id) {
    await chat.messageDelete(chat.curChannel?.id, chat.messageMenu.item?.id)
    message.success('撤回成功')
  }
}

const clickEdit = () => {
  if (!chat.messageMenu.item?.id || !chat.curChannel?.id) {
    return;
  }
  chat.startEditingMessage({
    messageId: chat.messageMenu.item.id,
    channelId: chat.curChannel.id,
    originalContent: chat.messageMenu.item.content || '',
    draft: chat.messageMenu.item.content || ''
  });
  chat.messageMenu.show = false;
}

const clickCopy = async () => {
  let copyText = '';
  const items = Element.parse(chat.messageMenu.item?.content || '');
  for (let item of items) {
    if (item.type == 'text') {
      copyText += item.toString();
    }
  }

  try {
    // 执行复制操作
    await navigator.clipboard.writeText(copyText);
    message.success("已复制");
  } catch (err) {
    message.error('复制失败');
  }
}

const addToMyEmoji = async () => {
  const items = Element.parse(chat.messageMenu.item?.content || '');
  for (let item of items) {
    if (item.type == "img") {
      const id = item.attrs.src.replace('id:', '');
      try {
        const resp = await user.emojiAdd(id);
        console.log(222, resp);
        // await db.thumbs.add({
        //   id: id,
        //   recentUsed: Number(Date.now()),
        //   filename: 'image.png',
        //   mimeType: '',
        //   data: null, // 无数据，按id加载
        // });
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
  const data = chat.messageMenu.item as any;
  if (!data?.user?.id) {
    message.warning(t('whisper.userUnknown'));
    return;
  }
  if (data.user.id === user.info.id) {
    message.warning(t('whisper.selfNotAllowed'));
    return;
  }
  const targetUser: User = {
    id: data.user.id,
    name: data.user.name || data.user.username || '',
    nick: data.member?.nick || data.user.nick || data.user.name || '未知成员',
    avatar: data.member?.avatar || data.user.avatar || '',
    discriminator: data.user.discriminator || '',
    is_bot: !!data.user.is_bot,
  };
  chat.setWhisperTarget(targetUser);
  chat.messageMenu.show = false;
};

const showWhisper = computed(() => {
  const data = chat.messageMenu.item;
  if (!data?.user?.id) {
    return false;
  }
  return data.user.id !== user.info.id;
});
</script>

<template>
  <context-menu v-model:show="chat.messageMenu.show" :options="chat.messageMenu.optionsComponent">
    <context-menu-item v-if="chat.messageMenu.hasImage" label="添加到表情收藏" @click="addToMyEmoji" />
    <!-- <context-menu-sperator /> -->
    <!-- <context-menu-item label="Item with a icon" icon="icon-reload-1" @click="alertContextMenuItemClicked('Item2')" /> -->
    <!-- <context-menu-item label="Test Item" @click="alertContextMenuItemClicked('Item2')" /> -->
    <context-menu-item v-if="!chat.messageMenu.hasImage" label="复制内容" @click="clickCopy" />
    <context-menu-item v-if="showWhisper" :label="t('whisper.menu')" @click="clickWhisper" />
    <context-menu-item label="回复" @click="clickReplyTo" />
    <context-menu-item label="编辑消息" @click="clickEdit"
      v-if="chat.messageMenu.item?.user?.id && (chat.messageMenu.item?.user?.id === user.info.id)" />
    <context-menu-item label="撤回" @click="clickDelete"
      v-if="chat.messageMenu.item?.user?.id && (chat.messageMenu.item?.user?.id === user.info.id)" />
    <!-- <context-menu-group label="Menu with child">
      <context-menu-item label="Item1" @click="alertContextMenuItemClicked('Item2-1')" />
      <context-menu-item label="Item1" @click="alertContextMenuItemClicked('Item2-2')" />
      <context-menu-group label="Child with v-for 50">
        <context-menu-item v-for="index of 50" :key="index" :label="'Item3-' + index"
          @click="alertContextMenuItemClicked('Item3-' + index)" />
      </context-menu-group>
    </context-menu-group> -->
  </context-menu>
</template>
