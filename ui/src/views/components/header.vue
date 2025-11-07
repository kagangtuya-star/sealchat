<script setup lang="tsx">
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { Plus, Users, Link, Refresh } from '@vicons/tabler';
import { AppsOutline } from '@vicons/ionicons5';
import { NIcon, useDialog, useMessage } from 'naive-ui';
import { computed, ref, type Component, h, defineAsyncComponent, onBeforeUnmount, onMounted } from 'vue';
import Notif from '../notif.vue'
import UserProfile from './user-profile.vue'
// import AdminSettings from './admin-settings.vue'
import { useI18n } from 'vue-i18n'
import { setLocale, setLocaleByNavigator } from '@/lang';
import UserPresencePopover from '../chat/components/UserPresencePopover.vue';

const AdminSettings = defineAsyncComponent(() => import('../admin/admin-settings.vue'));

const { t } = useI18n()

const notifShow = ref(false)
const userProfileShow = ref(false)
const adminShow = ref(false)
const chat = useChatStore();
const user = useUserStore();

const channelTitle = computed(() => {
  const raw = chat.curChannel?.name;
  const name = typeof raw === 'string' ? raw.trim() : '';
  return name ? `# ${name}` : t('headText');
});

const options = computed(() => [
  {
    label: t('headerMenu.profile'),
    key: 'profile',
    // icon: renderIcon(UserIcon)
  },
  user.checkPerm('mod_admin') ? {
    label: t('headerMenu.admin'),
    key: 'admin',
    // icon: renderIcon(UserIcon)
  } : null,
  {
    label: t('headerMenu.lang'),
    key: 'lang',
    children: [
      {
        label: t('headerMenu.langAuto'),
        key: 'lang:auto'
      },
      {
        label: '简体中文',
        key: 'lang:zh-cn'
      },
      {
        label: 'English',
        key: 'lang:en'
      },
      {
        label: '日本語',
        key: 'lang:ja'
      }
    ]
    // icon: renderIcon(UserIcon)
  },
  // {
  //   label: t('headerMenu.notice'),
  //   key: 'notice',
  //   // icon: renderIcon(UserIcon)
  // },
  {
    label: t('headerMenu.logout'),
    key: 'logout',
    // icon: renderIcon(LogoutIcon)
  }
].filter(i => i != null))


const handleSelect = async (key: string | number) => {
  switch (key) {
    case 'notice':
      userProfileShow.value = false;
      adminShow.value = false;
      notifShow.value = !notifShow.value;
      break;

    case 'profile':
      notifShow.value = false;
      adminShow.value = false;
      userProfileShow.value = !userProfileShow.value;
      break;

    case 'admin':
      notifShow.value = false;
      userProfileShow.value = false;
      adminShow.value = !adminShow.value;
      break;

    case 'logout':
      dialog.warning({
        title: t('dialogLogOut.title'),
        content: t('dialogLogOut.content'),
        positiveText: t('dialogLogOut.positiveText'),
        negativeText: t('dialogLogOut.negativeText'),
        onPositiveClick: () => {
          user.logout();
          chat.subject?.unsubscribe();
          window.location.reload();
          // router.push({ name: 'user-signin' });
        },
        onNegativeClick: () => {
        }
      })
      break;

    default:
      if (typeof key == "string" && key.startsWith('lang:')) {
        if (key == 'lang:auto') {
          setLocaleByNavigator();
        } else {
          setLocale(key.replace('lang:', ''));
        }
      }
      break;
  }
}

const renderIcon = (icon: Component) => {
  return () => {
    return h(NIcon, null, {
      default: () => h(icon)
    })
  }
}

const chOptions = computed(() => {
  const lst = chat.channelTree.map(i => {
    return {
      label: (i.type === 3 || (i as any).isPrivate) ? i.name : `${i.name} (${(i as any).membersCount})`,
      key: i.id,
      icon: undefined as any,
      props: undefined as any,
    }
  })
  lst.push({ label: t('channelListNew'), key: 'new', icon: renderIcon(Plus), props: { style: { 'font-weight': 'bold' } } })
  return lst;
})

const channelSelect = async (key: string) => {
  if (key === 'new') {
    showModal.value = true;
    // chat.channelCreate('测试频道');
    // message.info('暂不支持新建频道');
  } else {
    await chat.channelSwitchTo(key);
  }
}

const message = useMessage()
const usernameOverlap = ref(false);
const dialog = useDialog()

const showModal = ref(false);
const newChannelName = ref('');
const newChannel = async () => {
  if (!newChannelName.value.trim()) {
    message.error(t('dialoChannelgNew.channelNameHint'));
    return;
  }
  await chat.channelCreate(newChannelName.value);
  await chat.channelList();
}

const presencePopoverVisible = ref(false);
const actionRibbonActive = ref(false);
const onlineMembersCount = computed(() => chat.curChannelUsers.length);

const connectionStatus = computed(() => {
  switch (chat.connectState) {
    case 'connected':
      return {
        icon: Link,
        classes: 'text-green-600',
        label: t('connectState.connected'),
        spinning: false,
      };
    case 'connecting':
      return {
        icon: Refresh,
        classes: 'text-sky-600',
        label: t('connectState.connecting'),
        spinning: true,
      };
    case 'reconnecting':
      return {
        icon: Refresh,
        classes: 'text-orange-500',
        label: t('connectState.reconnecting', [chat.iReconnectAfterTime]),
        spinning: true,
      };
    case 'disconnected':
      return {
        icon: LinkOff,
        classes: 'text-red-600',
        label: t('connectState.disconnected'),
        spinning: false,
      };
    default:
      return {
        icon: Link,
        classes: 'text-gray-400',
        label: t('connectState.connecting'),
        spinning: false,
      };
  }
});

const handlePresenceRefresh = async () => {
  try {
    const data = await chat.getChannelPresence();
    if (data?.data) {
      data.data.forEach((item: any) => {
        chat.updatePresence(item.user_id, {
          lastPing: item.last_seen || Date.now(),
          latencyMs: item.latency_ms || 0,
          isFocused: item.is_focused || false,
        });
      });
    }
    message.success('状态已刷新');
  } catch (error) {
    message.error('刷新失败');
  }
};

const toggleActionRibbon = () => {
  chatEvent.emit('action-ribbon-toggle');
};

const handleRibbonStateUpdate = (state: boolean) => {
  actionRibbonActive.value = !!state;
};

onMounted(() => {
  chatEvent.on('action-ribbon-state', handleRibbonStateUpdate);
  chatEvent.emit('action-ribbon-state-request');
});

onBeforeUnmount(() => {
  chatEvent.off('action-ribbon-state', handleRibbonStateUpdate);
});
</script>

<template>
  <div class="sc-header border-b flex justify-between items-center w-full px-2" style="height: 3.5rem;">
    <div>
      <div class="flex items-center">
        <n-icon size="36" class="mr-2">
          <img src="@/assets/head3.png" />
        </n-icon>
        <span class="text-sm font-bold sm:text-xl">{{ channelTitle }}</span>
      </div>

      <!-- <n-button>登录</n-button>
      <n-button>切换房间</n-button> -->
      <span class="ml-4 hidden">
        <n-dropdown trigger="click" :options="chOptions" @select="channelSelect">
          <!-- <n-button>{{ chat.curChannel?.name || '加载中 ...' }}</n-button> -->
          <n-button text v-if="(chat.curChannel?.type === 3 || (chat.curChannel as any)?.isPrivate)">{{
            chat.curChannel?.name ? `${chat.curChannel?.name}` : '加载中 ...' }} ▼</n-button>
          <n-button text v-else>{{
            chat.curChannel?.name ? `${chat.curChannel?.name} (${(chat.curChannel as
              any).membersCount})`
              : '加载中 ...' }} ▼</n-button>
        </n-dropdown>
      </span>
    </div>

    <div class="sc-actions flex items-center">
      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button type="button" class="sc-icon-button sc-connection-icon" :class="connectionStatus.classes"
            :aria-label="connectionStatus.label" tabindex="-1">
            <n-icon :component="connectionStatus.icon" size="20"
              :class="{ 'sc-connection-icon--spin': connectionStatus.spinning }" />
          </button>
        </template>
        <span>{{ connectionStatus.label }}</span>
      </n-tooltip>

      <n-popover trigger="click" placement="bottom-end" :show="presencePopoverVisible"
        @update:show="presencePopoverVisible = $event">
        <template #trigger>
          <button type="button" class="sc-icon-button sc-online-button" aria-label="查看在线成员">
            <n-icon :component="Users" size="18" />
            <span class="online-badge">{{ onlineMembersCount }}</span>
          </button>
        </template>
        <UserPresencePopover :members="chat.curChannelUsers" :presence-map="chat.presenceMap"
          @request-refresh="handlePresenceRefresh" />
      </n-popover>

      <button type="button" class="sc-icon-button action-toggle-button" :class="{ 'is-active': actionRibbonActive }"
        @click="toggleActionRibbon" :aria-pressed="actionRibbonActive" aria-label="切换功能面板">
        <n-icon :component="AppsOutline" size="20" />
      </button>

      <n-dropdown :overlap="usernameOverlap" placement="bottom-end" trigger="click" :options="options"
        @select="handleSelect">
        <span class="flex justify-center cursor-pointer">
          <span>{{ user.info.nick }}</span>
          <svg style="width: 1rem" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
            viewBox="0 0 24 24">
            <path d="M7 10l5 5l5-5H7z" fill="currentColor"></path>
          </svg>
        </span>
      </n-dropdown>
    </div>
  </div>

  <div v-if="userProfileShow" style="background-color: var(--n-color); margin-left: -1.5rem;"
    class="absolute flex justify-center items-center w-full h-full pointer-events-none z-10">
    <user-profile @close="userProfileShow = false" />
  </div>
  <div v-if="adminShow" style="background-color: var(--n-color); margin-left: -1.5rem;"
    class="absolute flex justify-center items-center w-full h-full pointer-events-none z-10">
    <AdminSettings @close="adminShow = false" />
  </div>
  <notif v-show="notifShow" />
</template>

<style scoped lang="scss">
.sc-actions {
  gap: 0.75rem;
}

.sc-icon-button {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 9999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background-color: transparent;
  padding: 0;
  cursor: pointer;
  position: relative;
  color: #475569;
  transition: color 0.2s ease, transform 0.2s ease;
}

.sc-icon-button:hover,
.sc-icon-button:focus-visible {
  color: #0ea5e9;
  transform: translateY(-1px);
}

.sc-connection-icon {
  cursor: default;
}

.sc-connection-icon--spin {
  animation: sc-connection-spin 0.9s linear infinite;
}

@keyframes sc-connection-spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

.action-toggle-button {
  color: #0f172a;
}

.action-toggle-button.is-active {
  color: #0369a1;
  background-color: rgba(14, 165, 233, 0.28);
  box-shadow: 0 10px 30px rgba(14, 165, 233, 0.35);
}

.online-badge {
  position: absolute;
  top: -0.1rem;
  right: -0.05rem;
  min-width: 1.1rem;
  height: 1.1rem;
  border-radius: 9999px;
  background-color: #fff;
  color: #0f172a;
  font-size: 0.65rem;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(15, 23, 42, 0.15);
  line-height: 1;
}
</style>
