<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import type { SatoriMessage } from '@/types';
import { useChatStore, chatEvent } from '@/stores/chat';
import { computed, nextTick } from 'vue';
import { NIcon, useMessage } from 'naive-ui';
import { Edit, Id, Message2, MessageCircle2, UserPlus } from '@vicons/tabler';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';
import { useDisplayStore } from '@/stores/display';

const chat = useChatStore()
const message = useMessage()
const { t } = useI18n();
const user = useUserStore()
const display = useDisplayStore()
interface CharacterCardOpenPayload {
  item: SatoriMessage | null;
  clientX: number;
  clientY: number;
}

const emit = defineEmits<{
  (event: 'open-character-card', payload: CharacterCardOpenPayload): void;
}>();

const avatarMenuClass = computed(() => (display.palette === 'night' ? 'avatar-menu--night' : 'avatar-menu--day'))
const avatarMenuTheme = computed(() => (display.palette === 'night' ? 'default dark' : 'default'))
const avatarMenuOptions = computed<MenuOptions>(() => ({
  ...chat.avatarMenu.optionsComponent,
  theme: avatarMenuTheme.value,
  customClass: avatarMenuClass.value,
  adjustPosition: { xDirection: 'right', yDirection: 'top' },
}))

const isSelf = computed(() => {
  const data = chat.avatarMenu.item;
  return !!data?.user?.id && data.user.id === user.info.id;
});

const showUserActions = computed(() => !!chat.avatarMenu.item?.user?.id && !isSelf.value);

const clickTalkTo = async () => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    if (isSelf.value) return;
    chat.avatarMenu.show = false;
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
  if (isSelf.value) {
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
  chat.addWhisperTarget(targetUser);
  chat.confirmWhisperTargets();
  chat.avatarMenu.show = false;
};

const clickFriendAdd = async () => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    if (isSelf.value) {
      message.warning('不能添加自己为好友');
      return;
    }
    chat.avatarMenu.show = false;
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

    if (isSelf.value) {
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
  return isSelf.value;
});

const openIdentitySettings = () => {
  chat.avatarMenu.show = false;
  chatEvent.emit('channel-identity-open');
};

const clickCharacterCard = (event: MouseEvent) => {
  const item = chat.avatarMenu.item;
  chat.avatarMenu.show = false;
  emit('open-character-card', {
    item,
    clientX: event.clientX,
    clientY: event.clientY,
  });
};

const menuAvatar = computed(() => {
  const item = chat.avatarMenu.item;
  if (!item) return '';
  // bot: 优先使用 user.avatar
  if (item.user?.is_bot || item.user_id?.startsWith('BOT:')) {
    return item.user?.avatar || '';
  }
  // 普通用户: 优先使用频道角色头像(member.avatar)，其次用户头像(user.avatar)
  return item.member?.avatar || item.user?.avatar || '';
});
</script>

<template>
  <context-menu v-model:show="chat.avatarMenu.show" :options="avatarMenuOptions">
    <div class="px-4 pb-1 flex space-x-2">
      <Avatar :size="48" :src="menuAvatar"></Avatar>
      <div>
        <div class="text-more" style="width: 9rem;" :title="nick">{{ nick }}</div>
        <div>
          {{ chat.avatarMenu.item?.user?.username }}
        </div>
      </div>
    </div>

    <context-menu-sperator />
    <div class="avatar-menu-actions">
      <button v-if="showUserActions" type="button" class="avatar-menu-action" @click="clickWhisper">
        <NIcon :size="15" class="avatar-menu-action__icon"><MessageCircle2 /></NIcon>
        <span>{{ t('inputBox.whisperButton') }}</span>
      </button>
      <button v-if="showUserActions" type="button" class="avatar-menu-action" @click="clickTalkTo">
        <NIcon :size="15" class="avatar-menu-action__icon"><Message2 /></NIcon>
        <span>私聊</span>
      </button>
      <button v-if="showFriendAdd" type="button" class="avatar-menu-action" @click="clickFriendAdd">
        <NIcon :size="15" class="avatar-menu-action__icon"><UserPlus /></NIcon>
        <span>添加好友</span>
      </button>
      <button type="button" class="avatar-menu-action" @click="clickCharacterCard">
        <NIcon :size="15" class="avatar-menu-action__icon"><Id /></NIcon>
        <span>人物卡</span>
      </button>
      <button v-if="showIdentitySettings" type="button" class="avatar-menu-action" @click="openIdentitySettings">
        <NIcon :size="15" class="avatar-menu-action__icon"><Edit /></NIcon>
        <span>修改角色</span>
      </button>
    </div>
  </context-menu>
</template>

<style scoped>
:deep(.context-menu.avatar-menu--night),
:deep(.mx-context-menu.avatar-menu--night) {
  background: rgba(15, 23, 42, 0.95);
  border-color: rgba(148, 163, 184, 0.35);
  color: #e2e8f0;
}

:deep(.context-menu.avatar-menu--night .context-menu-item),
:deep(.mx-context-menu.avatar-menu--night .mx-context-menu-item) {
  color: inherit;
}

:deep(.context-menu.avatar-menu--night .context-menu-item:hover),
:deep(.mx-context-menu.avatar-menu--night .mx-context-menu-item:hover) {
  background: rgba(255, 255, 255, 0.08);
}

:deep(.context-menu.avatar-menu--day),
:deep(.mx-context-menu.avatar-menu--day) {
  background: rgba(248, 250, 252, 0.98);
  border-color: rgba(15, 23, 42, 0.08);
  color: #0f172a;
}

:deep(.context-menu.avatar-menu--day .context-menu-item),
:deep(.mx-context-menu.avatar-menu--day .mx-context-menu-item) {
  color: inherit;
}

:deep(.context-menu.avatar-menu--day .context-menu-item:hover),
:deep(.mx-context-menu.avatar-menu--day .mx-context-menu-item:hover) {
  background: rgba(15, 23, 42, 0.06);
}

.avatar-menu-actions {
  display: flex;
  gap: 0.2rem;
  min-width: 14rem;
  padding: 0.2rem 0.65rem 0.25rem;
}

.avatar-menu-action {
  display: inline-flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
  align-items: center;
  justify-content: center;
  gap: 0.08rem;
  padding: 0.22rem 0.3rem;
  border: 0;
  border-radius: 0.35rem;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
  line-height: 1.1;
  white-space: nowrap;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.avatar-menu-action__icon {
  flex-shrink: 0;
}

:deep(.context-menu.avatar-menu--night .avatar-menu-action:hover),
:deep(.mx-context-menu.avatar-menu--night .avatar-menu-action:hover) {
  background: rgba(255, 255, 255, 0.08);
}

:deep(.context-menu.avatar-menu--night .avatar-menu-action:active),
:deep(.mx-context-menu.avatar-menu--night .avatar-menu-action:active) {
  background: rgba(255, 255, 255, 0.14);
}

:deep(.context-menu.avatar-menu--day .avatar-menu-action:hover),
:deep(.mx-context-menu.avatar-menu--day .avatar-menu-action:hover) {
  background: rgba(15, 23, 42, 0.06);
}

:deep(.context-menu.avatar-menu--day .avatar-menu-action:active),
:deep(.mx-context-menu.avatar-menu--day .avatar-menu-action:active) {
  background: rgba(15, 23, 42, 0.1);
}
</style>
