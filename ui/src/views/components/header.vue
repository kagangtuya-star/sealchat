<script setup lang="tsx">
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { LayoutSidebarLeftCollapse, LayoutSidebarLeftExpand, Plus, Users, Link, Refresh, UserCircle, Palette } from '@vicons/tabler';
import { AppsOutline, MusicalNotesOutline, SearchOutline, UnlinkOutline, BrowsersOutline, NotificationsOutline } from '@vicons/ionicons5';
import { NIcon, useDialog, useMessage } from 'naive-ui';
import { computed, ref, type Component, h, defineAsyncComponent, onBeforeUnmount, onMounted, watch, withDefaults } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Notif from '../notif.vue'
import UserProfile from './user-profile.vue'
// import AdminSettings from './admin-settings.vue'
import { useI18n } from 'vue-i18n'
import { setLocale, setLocaleByNavigator } from '@/lang';
import UserPresencePopover from '../chat/components/UserPresencePopover.vue';
import { useChannelSearchStore } from '@/stores/channelSearch';
import AudioDrawer from '@/components/audio/AudioDrawer.vue';
import { useAudioStudioStore } from '@/stores/audioStudio';
import { useIFormStore } from '@/stores/iform';

const AdminSettings = defineAsyncComponent(() => import('../admin/admin-settings.vue'));

const { t } = useI18n()

const props = withDefaults(defineProps<{ sidebarCollapsed?: boolean }>(), {
  sidebarCollapsed: false,
})

const sidebarCollapsed = computed(() => props.sidebarCollapsed)

const emit = defineEmits<{
  (e: 'toggle-sidebar'): void
}>()

const notifShow = ref(false)
const userProfileShow = ref(false)
const adminShow = ref(false)
const chat = useChatStore();
const user = useUserStore();
const router = useRouter();
const route = useRoute();
const channelSearch = useChannelSearchStore();
const audioStudio = useAudioStudioStore();
const iFormStore = useIFormStore();
iFormStore.bootstrap();

const timelineItems = ref<any[]>([]);
const timelineLoading = ref(false);
const notifTimer = ref<number | null>(null);
const isAdmin = computed(() => !!user.checkPerm('mod_admin'));
const hasUnreadUpdate = computed(() => timelineItems.value.some((item) => item?.type === 'system.update' && !item?.isRead));
const showNotifBell = computed(() => isAdmin.value && (hasUnreadUpdate.value || notifShow.value));

const channelTitle = computed(() => {
  const raw = chat.curChannel?.name;
  const name = typeof raw === 'string' ? raw.trim() : '';
  return name ? `# ${name}` : t('headText');
});

const currentWorldName = computed(() => chat.currentWorld?.name || '未选择世界');
const isObserver = computed(() => chat.isObserver);

const openWorldLobby = () => {
  router.push({ name: 'world-lobby' });
};

const openWorldDetail = () => {
  if (!chat.currentWorldId) {
    openWorldLobby();
    return;
  }
  router.push({ name: 'world-detail', params: { worldId: chat.currentWorldId } });
};

const goLogin = () => {
  router.push({ name: 'user-signin', query: { redirect: route.fullPath } });
};

const openDisplaySettings = () => {
  chatEvent.emit('open-display-settings');
};

const refreshTimeline = async () => {
  if (!isAdmin.value) {
    timelineItems.value = [];
    return;
  }
  timelineLoading.value = true;
  try {
    const resp = await user.timelineList();
    timelineItems.value = resp.data.items || [];
  } catch (err) {
    console.error(err);
  } finally {
    timelineLoading.value = false;
  }
};

const markUpdateRead = async () => {
  const unreadIds = timelineItems.value
    .filter((item) => item?.type === 'system.update' && !item?.isRead)
    .map((item) => item.id)
    .filter((id) => !!id);
  if (!unreadIds.length) return;
  try {
    await user.timelineMarkRead(unreadIds);
    timelineItems.value = timelineItems.value.map((item) => {
      if (unreadIds.includes(item.id)) {
        return { ...item, isRead: true };
      }
      return item;
    });
  } catch (err) {
    console.error(err);
  }
};

const toggleNotifPanel = () => {
  userProfileShow.value = false;
  adminShow.value = false;
  notifShow.value = !notifShow.value;
};

const iFormButtonActive = computed(() => iFormStore.drawerVisible || iFormStore.hasInlinePanels || iFormStore.hasFloatingWindows);
const iFormHasAttention = computed(() => iFormStore.hasAttention);

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
          router.replace({ name: 'user-signin' });
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

const userDisplayName = computed(() => user.info.nick || user.info.username || '个人中心')

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
        icon: UnlinkOutline,
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

const handlePresenceRefresh = async (options?: { silent?: boolean }) => {
  const silent = !!options?.silent;
  const selfId = user.info?.id || '';
  try {
    const data = await chat.getChannelPresence();
    const updatedAt = typeof data?.updated_at === 'number' ? data.updated_at : undefined;
    if (typeof updatedAt === 'number') {
      chat.syncServerTime(updatedAt);
    }
    if (Array.isArray(data?.data)) {
      data.data.forEach((item: any) => {
        const userId = item?.user?.id || item?.user_id;
        if (!userId) {
          return;
        }
        const isSelf = selfId && userId === selfId;
        const lastSeenServer = item?.lastSeen ?? item?.last_seen;
        chat.updatePresence(userId, {
          lastPing: isSelf
            ? Date.now()
            : (typeof lastSeenServer === 'number' ? chat.serverTsToLocal(lastSeenServer) : Date.now()),
          latencyMs: isSelf ? chat.lastLatencyMs : (item?.latency ?? item?.latency_ms ?? 0),
          isFocused: isSelf ? chat.isAppFocused : (item?.focused ?? item?.is_focused ?? false),
        });
      });
    }
    // 立即触发一次新的延迟探测以刷新本端值，避免旧值累加
    chat.measureLatency();
    if (!silent) {
      message.success('状态已刷新');
    }
  } catch (error) {
    if (!silent) {
      message.error('刷新失败');
    } else {
      console.error('自动刷新在线状态失败', error);
    }
  }
};

const searchPanelActive = computed(() => channelSearch.panelVisible);
const toggleChannelSearch = () => {
  channelSearch.togglePanel();
};

const openAudioStudio = () => {
  audioStudio.toggleDrawer(true);
};

const handleIFormButtonClick = () => {
  if (!chat.curChannel?.id) {
    return;
  }
  iFormStore.ensureForms(chat.curChannel.id);
  iFormStore.openDrawer();
};

watch(
  () => chat.curChannel?.id,
  (channelId) => {
    audioStudio.setActiveChannel(channelId || null);
  },
  { immediate: true },
);

watch(
  () => chat.currentWorldId,
  (worldId) => {
    audioStudio.setCurrentWorld(worldId || null);
  },
  { immediate: true },
);

watch(presencePopoverVisible, (visible, oldVisible) => {
  if (visible && !oldVisible) {
    handlePresenceRefresh({ silent: true });
  }
});

watch(
  () => chat.curChannel?.id,
  (channelId, prevChannelId) => {
    if (!channelId || channelId === prevChannelId) {
      return;
    }
    chat.clearPresenceMap();
    handlePresenceRefresh({ silent: true });
  }
);

const emitOverlayState = (source: string, visible: boolean, prevVisible?: boolean) => {
  if (visible) {
    chatEvent.emit('global-overlay-toggle', { source, open: true } as any);
  } else if (prevVisible) {
    chatEvent.emit('global-overlay-toggle', { source, open: false } as any);
  }
};

watch(adminShow, (visible, prevVisible) => emitOverlayState('admin-settings', visible, prevVisible));
watch(userProfileShow, (visible, prevVisible) => emitOverlayState('user-profile', visible, prevVisible));
watch(notifShow, (visible, prevVisible) => emitOverlayState('notif-panel', visible, prevVisible));
watch(notifShow, async (visible) => {
  if (!visible || !isAdmin.value) {
    return;
  }
  await refreshTimeline();
  await markUpdateRead();
});

const toggleActionRibbon = () => {
  chatEvent.emit('action-ribbon-toggle');
};

const handleRibbonStateUpdate = (state: boolean) => {
  actionRibbonActive.value = !!state;
};

const handleOpenUserProfile = () => {
  notifShow.value = false;
  adminShow.value = false;
  userProfileShow.value = true;
};

onMounted(() => {
  chatEvent.on('action-ribbon-state', handleRibbonStateUpdate);
  chatEvent.on('open-user-profile', handleOpenUserProfile);
  chatEvent.emit('action-ribbon-state-request');
});

onBeforeUnmount(() => {
  chatEvent.off('action-ribbon-state', handleRibbonStateUpdate);
  chatEvent.off('open-user-profile', handleOpenUserProfile);
  if (notifTimer.value) {
    window.clearInterval(notifTimer.value);
    notifTimer.value = null;
  }
});

watch(isAdmin, (value) => {
  if (notifTimer.value) {
    window.clearInterval(notifTimer.value);
    notifTimer.value = null;
  }
  if (!value) {
    timelineItems.value = [];
    return;
  }
  refreshTimeline();
  notifTimer.value = window.setInterval(refreshTimeline, 60_000);
}, { immediate: true });

const sidebarToggleIcon = computed(() => sidebarCollapsed.value ? LayoutSidebarLeftExpand : LayoutSidebarLeftCollapse)
</script>

<template>
  <div class="sc-header border-b flex justify-between items-center w-full px-2" style="height: 3.5rem;">
    <div>
      <div class="flex items-center">
        <button
          type="button"
          class="sc-icon-button sc-sidebar-toggle-button mr-2"
          :class="{ 'is-collapsed': sidebarCollapsed }"
          aria-label="切换频道栏"
          @click="emit('toggle-sidebar')"
        >
          <n-icon :component="sidebarToggleIcon" size="20" />
        </button>
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

    <div v-if="!isObserver" class="sc-actions flex items-center">
      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button type="button" class="sc-icon-button sc-connection-icon" :class="connectionStatus.classes"
            :aria-label="connectionStatus.label" tabindex="-1">
          <n-icon :component="connectionStatus.icon" size="16"
            :class="{ 'sc-connection-icon--spin': connectionStatus.spinning }" />
          </button>
        </template>
        <span>{{ connectionStatus.label }}</span>
      </n-tooltip>

      <n-popover trigger="click" placement="bottom-end" :show="presencePopoverVisible"
        @update:show="presencePopoverVisible = $event">
        <template #trigger>
          <button type="button" class="sc-icon-button sc-online-button" aria-label="查看在线成员">
            <n-icon :component="Users" size="16" />
            <span class="online-badge">{{ onlineMembersCount }}</span>
          </button>
        </template>
        <UserPresencePopover :members="chat.curChannelUsers" :presence-map="chat.presenceMap"
          @request-refresh="handlePresenceRefresh" />
      </n-popover>

      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button sc-search-button"
            :class="{ 'is-active': audioStudio.drawerVisible }"
            aria-label="音频工作台"
            @click="openAudioStudio"
          >
            <n-icon :component="MusicalNotesOutline" size="16" />
          </button>
        </template>
        <span>音频工作台</span>
      </n-tooltip>

      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button sc-search-button sc-search-button--channel"
            :class="{ 'is-active': searchPanelActive }"
            aria-label="搜索频道消息"
            @click="toggleChannelSearch"
          >
            <n-icon :component="SearchOutline" size="16" />
          </button>
        </template>
        <span>搜索频道消息</span>
      </n-tooltip>

      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button sc-search-button"
            :class="{ 'is-active': iFormButtonActive }"
            aria-label="频道嵌入窗"
            :disabled="!chat.curChannel?.id"
            @click="handleIFormButtonClick"
          >
            <span v-if="iFormHasAttention" class="sc-icon-button__badge"></span>
            <n-icon :component="BrowsersOutline" size="16" />
          </button>
        </template>
        <span>频道嵌入窗</span>
      </n-tooltip>

      <button type="button" class="sc-icon-button action-toggle-button" :class="{ 'is-active': actionRibbonActive }"
        @click="toggleActionRibbon" :aria-pressed="actionRibbonActive" aria-label="切换功能面板">
        <n-icon :component="AppsOutline" size="18" />
      </button>

      <n-tooltip v-if="showNotifBell" placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button"
            :class="{ 'is-active': notifShow }"
            aria-label="查看更新通知"
            @click="toggleNotifPanel"
          >
            <span v-if="hasUnreadUpdate" class="sc-icon-button__badge"></span>
            <n-icon :component="NotificationsOutline" size="16" />
          </button>
        </template>
        <span>更新通知</span>
      </n-tooltip>

      <n-dropdown :overlap="usernameOverlap" placement="bottom-end" trigger="click" :options="options"
        @select="handleSelect">
        <n-tooltip trigger="hover">
          <template #trigger>
            <button
              type="button"
              class="sc-icon-button sc-user-button"
              :aria-label="`打开 ${userDisplayName} 的菜单`"
            >
              <n-icon :component="UserCircle" size="18" />
            </button>
          </template>
          <span>{{ userDisplayName }}</span>
        </n-tooltip>
      </n-dropdown>
    </div>
    <div v-else class="sc-actions flex items-center">
      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button"
            aria-label="显示设置"
            @click="openDisplaySettings"
          >
            <n-icon :component="Palette" size="16" />
          </button>
        </template>
        <span>显示设置</span>
      </n-tooltip>
      <n-button size="small" type="primary" @click="goLogin">登录</n-button>
    </div>
  </div>

  <div v-if="userProfileShow" style="background-color: var(--n-color); margin-left: -1.5rem;"
    class="absolute flex justify-center items-center w-full h-full sc-overlay-layer">
    <user-profile @close="userProfileShow = false" />
  </div>
  <div
    v-if="adminShow"
    style="background-color: var(--n-color); margin-left: -1.5rem;"
    class="absolute flex justify-center items-center w-full h-full sc-overlay-layer"
  >
    <AdminSettings @close="adminShow = false" />
  </div>
  <Notif v-show="notifShow" :items="timelineItems" :visible="notifShow" @close="notifShow = false" />
  <AudioDrawer />
</template>

<style scoped lang="scss">
.sc-header {
  background-color: var(--sc-bg-header);
  color: var(--sc-text-primary);
  transition: background-color 0.25s ease, color 0.25s ease;
}

.sc-actions {
  gap: 0.45rem;
}

.sc-icon-button {
  width: 1.95rem;
  height: 1.95rem;
  border-radius: 9999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background-color: transparent;
  padding: 0;
  cursor: pointer;
  position: relative;
  color: var(--sc-text-secondary);
  transition: color 0.2s ease, transform 0.2s ease, background-color 0.2s ease;
}

.sc-search-button--channel {
  border: 1px solid transparent;
}

.sc-icon-button:hover,
.sc-icon-button:focus-visible {
  color: #0ea5e9;
  transform: translateY(-0.5px);
}

.sc-overlay-layer {
  pointer-events: auto;
  z-index: 1500; /* keep below Naive UI overlay base (>=2000) so nested popups/modal remain visible */
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
  color: var(--sc-text-primary);
}

.action-toggle-button.is-active {
  color: #0369a1;
  background-color: rgba(14, 165, 233, 0.28);
  box-shadow: 0 10px 30px rgba(14, 165, 233, 0.35);
}

.sc-search-button.is-active {
  color: #0369a1;
  background-color: rgba(14, 165, 233, 0.2);
  box-shadow: inset 0 0 0 1px rgba(14, 165, 233, 0.35);
}

.sc-icon-button__badge {
  position: absolute;
  top: 0.15rem;
  right: 0.15rem;
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 9999px;
  background-color: #f97316;
  box-shadow: 0 0 0 2px rgba(15, 23, 42, 0.9);
}


.online-badge {
  position: absolute;
  top: -0.1rem;
  right: -0.05rem;
  min-width: 1.1rem;
  height: 1.1rem;
  border-radius: 9999px;
  background-color: var(--sc-badge-bg);
  color: var(--sc-badge-text);
  font-size: 0.65rem;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--sc-border-strong);
  line-height: 1;
}
</style>
