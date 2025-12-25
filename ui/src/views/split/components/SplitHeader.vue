<script setup lang="tsx">
import { computed, defineAsyncComponent, h, ref, type Component } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { NDropdown, NIcon, NTooltip, useDialog } from 'naive-ui';
import { LayoutSidebarLeftCollapse, LayoutSidebarLeftExpand, Link, Refresh, UserCircle, Users } from '@vicons/tabler';
import { SearchOutline, UnlinkOutline, AppsOutline, MusicalNotesOutline, BrowsersOutline } from '@vicons/ionicons5';
import Notif from '@/views/notif.vue';
import UserProfile from '@/views/components/user-profile.vue';
import { setLocale, setLocaleByNavigator } from '@/lang';
import { useUserStore } from '@/stores/user';

const AdminSettings = defineAsyncComponent(() => import('@/views/admin/admin-settings.vue'));

type ConnectState = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

const props = withDefaults(defineProps<{
  sidebarCollapsed?: boolean;
  channelTitle?: string;
  connectState?: ConnectState;
  onlineMembersCount?: number;
  audioStudioActive?: boolean;
  searchActive?: boolean;
  embedPanelActive?: boolean;
  embedPanelHasAttention?: boolean;
  embedPanelDisabled?: boolean;
  actionRibbonActive?: boolean;
}>(), {
  sidebarCollapsed: false,
  channelTitle: '',
  connectState: 'connecting',
  onlineMembersCount: 0,
  audioStudioActive: false,
  searchActive: false,
  embedPanelActive: false,
  embedPanelHasAttention: false,
  embedPanelDisabled: false,
  actionRibbonActive: false,
});

const emit = defineEmits<{
  (e: 'toggle-sidebar'): void;
  (e: 'open-audio-studio'): void;
  (e: 'toggle-search'): void;
  (e: 'open-embed-panel'): void;
  (e: 'toggle-action-ribbon'): void;
}>();

const router = useRouter();
const { t } = useI18n();
const dialog = useDialog();
const user = useUserStore();

const notifShow = ref(false);
const userProfileShow = ref(false);
const adminShow = ref(false);

const userDisplayName = computed(() => user.info.nick || user.info.username || '个人中心');

const sidebarToggleIcon = computed(() => props.sidebarCollapsed ? LayoutSidebarLeftExpand : LayoutSidebarLeftCollapse);

const connectionStatus = computed(() => {
  switch (props.connectState) {
    case 'connected':
      return { icon: Link, label: '已连接', classes: 'is-connected', spinning: false };
    case 'reconnecting':
      return { icon: Refresh, label: '重连中', classes: 'is-reconnecting', spinning: true };
    case 'disconnected':
      return { icon: UnlinkOutline, label: '已断开', classes: 'is-disconnected', spinning: false };
    case 'connecting':
    default:
      return { icon: Refresh, label: '连接中', classes: 'is-connecting', spinning: true };
  }
});

const options = computed(() => [
  { label: t('headerMenu.profile'), key: 'profile' },
  user.checkPerm('mod_admin') ? { label: t('headerMenu.admin'), key: 'admin' } : null,
  {
    label: t('headerMenu.lang'),
    key: 'lang',
    children: [
      { label: t('headerMenu.langAuto'), key: 'lang:auto' },
      { label: '简体中文', key: 'lang:zh-cn' },
      { label: 'English', key: 'lang:en' },
      { label: '日本語', key: 'lang:ja' },
    ],
  },
  { label: t('headerMenu.logout'), key: 'logout' },
].filter(Boolean));

const handleSelect = async (key: string | number) => {
  switch (key) {
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
          router.replace({ name: 'user-signin' });
        },
      });
      break;
    default:
      if (typeof key === 'string' && key.startsWith('lang:')) {
        if (key === 'lang:auto') {
          setLocaleByNavigator();
        } else {
          setLocale(key.replace('lang:', ''));
        }
      }
      break;
  }
};
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
        <span class="text-sm font-bold sm:text-xl">{{ channelTitle || t('headText') }}</span>
      </div>
    </div>

    <div class="sc-actions flex items-center">
      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button sc-connection-icon"
            :class="connectionStatus.classes"
            :aria-label="connectionStatus.label"
            tabindex="-1"
          >
            <n-icon
              :component="connectionStatus.icon"
              size="16"
              :class="{ 'sc-connection-icon--spin': connectionStatus.spinning }"
            />
          </button>
        </template>
        <span>{{ connectionStatus.label }}</span>
      </n-tooltip>

      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button type="button" class="sc-icon-button sc-online-button" aria-label="在线成员数">
            <n-icon :component="Users" size="16" />
            <span class="online-badge">{{ onlineMembersCount }}</span>
          </button>
        </template>
        <span>在线成员：{{ onlineMembersCount }}</span>
      </n-tooltip>

      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <button
            type="button"
            class="sc-icon-button sc-search-button"
            :class="{ 'is-active': audioStudioActive }"
            aria-label="音频工作台"
            @click="emit('open-audio-studio')"
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
            :class="{ 'is-active': searchActive }"
            aria-label="搜索频道消息"
            @click="emit('toggle-search')"
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
            :class="{ 'is-active': embedPanelActive }"
            aria-label="频道嵌入窗"
            :disabled="embedPanelDisabled"
            @click="emit('open-embed-panel')"
          >
            <span v-if="embedPanelHasAttention" class="sc-icon-button__badge"></span>
            <n-icon :component="BrowsersOutline" size="16" />
          </button>
        </template>
        <span>频道嵌入窗</span>
      </n-tooltip>

      <button
        type="button"
        class="sc-icon-button action-toggle-button"
        :class="{ 'is-active': actionRibbonActive }"
        @click="emit('toggle-action-ribbon')"
        :aria-pressed="actionRibbonActive"
        aria-label="切换功能面板"
      >
        <n-icon :component="AppsOutline" size="18" />
      </button>

      <n-dropdown placement="bottom-end" trigger="click" :options="options" @select="handleSelect">
        <n-tooltip trigger="hover">
          <template #trigger>
            <button type="button" class="sc-icon-button sc-user-button" :aria-label="`打开 ${userDisplayName} 的菜单`">
              <n-icon :component="UserCircle" size="18" />
            </button>
          </template>
          <span>{{ userDisplayName }}</span>
        </n-tooltip>
      </n-dropdown>
    </div>
  </div>

  <div
    v-if="userProfileShow"
    style="background-color: var(--n-color); margin-left: -1.5rem;"
    class="absolute flex justify-center items-center w-full h-full sc-overlay-layer"
  >
    <UserProfile @close="userProfileShow = false" />
  </div>
  <div
    v-if="adminShow"
    style="background-color: var(--n-color); margin-left: -1.5rem;"
    class="absolute flex justify-center items-center w-full h-full sc-overlay-layer"
  >
    <AdminSettings @close="adminShow = false" />
  </div>
  <Notif v-show="notifShow" />
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

.sc-icon-button:hover,
.sc-icon-button:focus-visible {
  color: #0ea5e9;
  transform: translateY(-0.5px);
}

.sc-search-button--channel {
  border: 1px solid transparent;
}

.sc-search-button.is-active {
  border-color: rgba(14, 165, 233, 0.45);
  background-color: rgba(14, 165, 233, 0.12);
  color: #0ea5e9;
}

.sc-icon-button__badge {
  position: absolute;
  top: 0.32rem;
  right: 0.32rem;
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 9999px;
  background-color: #ef4444;
  box-shadow: 0 0 0 2px var(--sc-bg-header);
}

.online-badge {
  position: absolute;
  right: -2px;
  top: -2px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 9999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(14, 165, 233, 0.18);
  color: var(--sc-text-primary);
  font-size: 11px;
  line-height: 1;
  border: 1px solid rgba(14, 165, 233, 0.35);
}

.sc-connection-icon--spin {
  animation: sc-connection-spin 1s linear infinite;
}

@keyframes sc-connection-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.sc-overlay-layer {
  pointer-events: auto;
  z-index: 1500;
}
</style>
