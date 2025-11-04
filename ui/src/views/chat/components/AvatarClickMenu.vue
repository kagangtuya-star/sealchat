<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import { useChatStore, chatEvent } from '@/stores/chat';
import { computed, nextTick } from 'vue';
import { useMessage } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';

const chat = useChatStore()
const message = useMessage()
const { t } = useI18n();
const user = useUserStore()

const clickTalkTo = async () => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    if (data.user.id === user.info.id) return;
    const ch = await chat.channelPrivateCreate(data.user.id);
    if (ch?.channel?.id) {
      chat.sidebarTab = 'privateChats';
      await chat.ChannelPrivateList()
      nextTick(async () => {
        await chat.channelSwitchTo(ch.channel.id);
      })
    }
  }
}

const clickWhisper = () => {
  const data = chat.avatarMenu.item as any;
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
  chat.avatarMenu.show = false;
};

const clickFriendAdd = async () => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    if (data.user.id === user.info.id) {
      message.warning('不能添加自己为好友');
      return;
    }
    try {
      const ret = await chat.friendRequestCreate(user.info.id, data.user.id, '');
      if (ret.status === 0) {
        message.success('好友请求已发送');
      } else {
        message.error('已经是好友，或者正在申请列表中');
      }
    } catch (error) {
      console.error('添加好友失败:', error);
      message.error('添加好友失败，可能正在请求或者已经是好友');
    }
  }
}


const showFriendAdd = computed(() => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    // 不显示加好友选项的情况:
    // 1. 点击的是自己的头像
    // 2. 点击的用户已经是好友
    if (!data.user?.id) return false;

    if (data.user.id === user.info.id) {
      return false;
    }

    let ret = true;
    // 如果已经是好友，返回false
    chat.channelTreePrivate.map(channel => {
      if (channel.friendInfo?.userInfo?.id === data.user?.id) {
        if (channel.friendInfo?.isFriend) ret = false;
      }
    })

    return ret;
  }
  return false;
});

const showWhisper = computed(() => {
  const data = chat.avatarMenu.item;
  if (!data?.user?.id) {
    return false;
  }
  return data.user.id !== user.info.id;
});


const nick = computed(() => {
  const item = chat.avatarMenu.item;

  let realName = item?.user?.nick ?? '';

  let displayName = item?.sender_member_name || item?.member?.nick || item?.user?.name || '未知';
  if (displayName == realName) {
    return displayName;
  }
  return `${displayName}(${realName})`;
});

const showIdentitySettings = computed(() => {
  const data = chat.avatarMenu.item;
  if (!data?.user?.id) {
    return false;
  }
  return data.user.id === user.info.id;
});

const openIdentitySettings = () => {
  chat.avatarMenu.show = false;
  chatEvent.emit('channel-identity-open');
};
</script>

<template>
  <context-menu v-model:show="chat.avatarMenu.show" :options="chat.messageMenu.optionsComponent">
    <div class="px-4 pb-1 flex space-x-2">
      <Avatar :size="48" :src="chat.avatarMenu.item?.member?.avatar"></Avatar>
      <div>
        <div class="text-more" style="width: 9rem;" :title="nick">{{ nick }}</div>
        <div>
          {{ chat.avatarMenu.item?.user?.username }}
        </div>
      </div>
    </div>

    <context-menu-sperator />
    <context-menu-item v-if="showWhisper" :label="t('whisper.menu')" @click="clickWhisper" />
    <context-menu-item label="私聊" @click="clickTalkTo" />
    <context-menu-item v-if="showFriendAdd" label="加好友" @click="clickFriendAdd" />
    <context-menu-item v-if="showIdentitySettings" label="更改频道内资料" @click="openIdentitySettings" />
  </context-menu>
</template>
