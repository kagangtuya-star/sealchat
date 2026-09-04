<script setup lang="tsx">
import { computed, defineAsyncComponent, h, ref, shallowRef, watch, type Component } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { NDropdown, NIcon, NPopover, NTooltip, useDialog, type DropdownOption } from 'naive-ui';
import { LayoutSidebarLeftCollapse, LayoutSidebarLeftExpand, Users } from '@vicons/tabler';
import { SearchOutline, AppsOutline, MusicalNotesOutline, BrowsersOutline } from '@vicons/ionicons5';
import Notif from '@/views/notif.vue';
import UserProfile from '@/views/components/user-profile.vue';
import UserPresencePopover from '@/views/chat/components/UserPresencePopover.vue';
import { setLocale, setLocaleByNavigator } from '@/lang';
import { useUserStore } from '@/stores/user';
import { useChatStore } from '@/stores/chat';
import Avatar from '@/components/avatar.vue';

const AdminSettings = defineAsyncComponent(() => import('@/views/admin/admin-settings.vue'));

const chat = useChatStore();

type ConnectState = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';
type PresenceData = {
  lastPing: number;
  latencyMs: number;
  isFocused: boolean;
};
type PresenceMember = {
  id: string;
  nick?: string;
  name?: string;
  avatar?: string;
  identity?: {
    displayName?: string;
    color?: string;
  };
};

const props = withDefaults(defineProps<{
  sidebarCollapsed?: boolean;
  channelTitle?: string;
  connectState?: ConnectState;
  onlineMembersCount?: number;
  presenceMembers?: PresenceMember[];
  presenceMap?: Record<string, PresenceData>;
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
  presenceMembers: () => [],
  presenceMap: () => ({}),
  audioStudioActive: false,
  searchActive: false,
  embedPanelActive: false,
  embedPanelHasAttention: false,
  embedPanelDisabled: false,
  actionRibbonActive: false,
});

const emit = defineEmits<{
  (e: 'toggle-sidebar'): void;
  (e: 'request-presence-refresh'): void;
  (e: 'open-audio-studio'): void;
  (e: 'toggle-search'): void;
  (e: 'open-embed-panel'): void;
  (e: 'toggle-action-ribbon'): void;
  (e: 'open-display-settings'): void;
}>();

const router = useRouter();
const { t } = useI18n();
const dialog = useDialog();
const user = useUserStore();

const notifShow = ref(false);
const userProfileShow = ref(false);
const adminShow = ref(false);
const inputStatsShow = ref(false);
const inputStatsLoading = ref(false);
const inputStatsComponent = shallowRef<any>(null);
const presencePopoverVisible = ref(false);
const connectionRecoveryPulseKey = ref(0);
const onlineBadgeAnimationKey = ref(0);

const userDisplayName = computed(() => user.info.nick || user.info.username || '个人中心');

const sidebarToggleIcon = computed(() => props.sidebarCollapsed ? LayoutSidebarLeftExpand : LayoutSidebarLeftCollapse);

const connectionStatus = computed(() => {
  switch (props.connectState) {
    case 'connected':
      return { state: 'connected' as const, label: '已连接', spinning: false };
    case 'reconnecting':
      return { state: 'reconnecting' as const, label: '重连中', spinning: true };
    case 'disconnected':
      return { state: 'disconnected' as const, label: '已断开', spinning: false };
    case 'connecting':
    default:
      return { state: 'connecting' as const, label: '连接中', spinning: true };
  }
});

const connectionLatencyMs = computed(() => {
  const latency = Number(chat.lastLatencyMs);
  return Number.isFinite(latency) && latency > 0 ? Math.round(latency) : undefined;
});

const presenceTooltipLabel = computed(() => {
  if (connectionStatus.value.state === 'disconnected') {
    return connectionStatus.value.label;
  }
  return `${connectionStatus.value.label} · ${props.onlineMembersCount} 人在线`;
});

watch(
  () => props.connectState,
  (state, previousState) => {
    if (state === 'connected' && previousState && previousState !== 'connected') {
      connectionRecoveryPulseKey.value += 1;
    }
  },
);

watch(
  () => props.onlineMembersCount,
  (count, previousCount) => {
    if (typeof previousCount === 'number' && count !== previousCount) {
      onlineBadgeAnimationKey.value += 1;
    }
  },
);

const options = computed(() => {
  const items: DropdownOption[] = [
    { label: t('headerMenu.profile'), key: 'profile' },
    { label: t('headerMenu.inputStats'), key: 'inputStats' },
    { label: t('headerMenu.display'), key: 'display' },
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
  ];
  if (user.checkPerm('mod_admin')) {
    items.splice(2, 0, { label: t('headerMenu.admin'), key: 'admin' });
  }
  return items;
});

const ensureInputStatsLoaded = async () => {
  if (inputStatsComponent.value || inputStatsLoading.value) {
    return;
  }
  inputStatsLoading.value = true;
  try {
    inputStatsComponent.value = (await import('@/views/components/InputStats.vue')).default;
  } catch (err) {
    console.error('load input stats component failed', err);
    inputStatsShow.value = false;
  } finally {
    inputStatsLoading.value = false;
  }
};

const toggleInputStats = async () => {
  notifShow.value = false;
  adminShow.value = false;
  userProfileShow.value = false;

  if (inputStatsShow.value) {
    inputStatsShow.value = false;
    return;
  }

  inputStatsShow.value = true;
  await ensureInputStatsLoaded();
};

const handleSelect = async (key: string | number) => {
  switch (key) {
    case 'profile':
      notifShow.value = false;
      adminShow.value = false;
      inputStatsShow.value = false;
      userProfileShow.value = !userProfileShow.value;
      break;
    case 'inputStats':
      await toggleInputStats();
      break;
    case 'display':
      notifShow.value = false;
      userProfileShow.value = false;
      adminShow.value = false;
      inputStatsShow.value = false;
      emit('open-display-settings');
      break;
    case 'admin':
      notifShow.value = false;
      userProfileShow.value = false;
      inputStatsShow.value = false;
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
      <n-popover trigger="click" placement="bottom-end" :show="presencePopoverVisible"
        @update:show="presencePopoverVisible = $event">
        <template #trigger>
          <n-tooltip placement="bottom" trigger="hover">
            <template #trigger>
              <button
                type="button"
                class="sc-icon-button sc-online-button"
                :class="{ 'sc-online-button--busy': connectionStatus.spinning }"
                :aria-label="presenceTooltipLabel"
              >
                <n-icon :component="Users" size="16" class="sc-online-button__members-icon" />
                <span
                  :key="`online-${onlineBadgeAnimationKey}`"
                  class="online-badge"
                  :class="{ 'online-badge--changed': onlineBadgeAnimationKey > 0 }"
                >
                  {{ onlineMembersCount }}
                </span>
                <span
                  :key="`status-${connectionRecoveryPulseKey}`"
                  class="sc-online-button__status-dot"
                  :class="{
                    'is-connected': connectionStatus.state === 'connected',
                    'is-connecting': connectionStatus.state === 'connecting',
                    'is-reconnecting': connectionStatus.state === 'reconnecting',
                    'is-disconnected': connectionStatus.state === 'disconnected',
                    'is-busy': connectionStatus.spinning,
                    'is-recovering': connectionStatus.state === 'connected' && connectionRecoveryPulseKey > 0,
                  }"
                  aria-hidden="true"
                >
                  <span
                    v-if="connectionStatus.spinning"
                    class="sc-online-button__status-ring"
                    aria-hidden="true"
                  ></span>
                </span>
              </button>
            </template>
            <span>{{ presenceTooltipLabel }}</span>
          </n-tooltip>
        </template>
        <UserPresencePopover
          :members="presenceMembers"
          :presence-map="presenceMap"
          :connect-state="connectionStatus.state"
          :connection-label="connectionStatus.label"
          :latency-ms="connectionLatencyMs"
          @request-refresh="emit('request-presence-refresh')"
        />
      </n-popover>

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
              <Avatar
                class="sc-user-avatar"
                :src="user.info.avatar"
                :size="22"
                :border="false"
              />
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
  <div
    v-if="inputStatsShow"
    style="background-color: var(--n-color); margin-left: -1.5rem; padding-top: 2rem;"
    class="absolute flex justify-center items-start w-full h-full sc-overlay-layer"
  >
    <component
      :is="inputStatsComponent"
      v-if="inputStatsComponent"
      :current-world-id="chat.currentWorldId"
      @close="inputStatsShow = false"
    />
    <div v-else class="input-stats-loading">输入统计加载中...</div>
  </div>
  <Notif v-show="notifShow" />
</template>

<style scoped lang="scss">
.sc-header {
  background-color: var(--sc-bg-header);
  color: var(--sc-text-primary);
  transition: background-color 0.25s ease, color 0.25s ease;
}

.input-stats-loading {
  width: min(1100px, calc(100vw - 3rem));
  min-height: 16rem;
  margin-top: 1rem;
  border-radius: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sc-text-secondary);
  background-color: var(--sc-bg-elevated, var(--n-color));
}

.sc-actions {
  gap: 0.45rem;
}

.sc-user-button {
  overflow: hidden;
}

.sc-user-button :deep(.avatar-shell) {
  border-radius: 9999px;
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

@media (max-width: 640px) {
  .sc-actions {
    gap: 0.32rem;
  }

  .sc-user-button {
    width: 1.58rem;
    height: 1.58rem;
  }

  .sc-user-button :deep(.avatar-shell) {
    width: 16px !important;
    height: 16px !important;
    min-width: 16px !important;
    min-height: 16px !important;
  }
}

.sc-online-button--busy .sc-online-button__members-icon {
  opacity: 0.68;
}

.sc-online-button__members-icon {
  transition: opacity 0.2s ease;
}

.sc-online-button__status-dot {
  position: absolute;
  right: 0.08rem;
  bottom: 0.08rem;
  width: 0.48rem;
  height: 0.48rem;
  display: block;
  z-index: 2;
  border: 1px solid var(--sc-bg-header, #fff);
  border-radius: 9999px;
  box-shadow: 0 0 0 1px rgba(15, 23, 42, 0.16);
  color: #22c55e;
  background-color: currentColor;
  transition: background-color 0.2s ease, color 0.2s ease;
}

.sc-online-button__status-dot.is-connecting {
  color: #0ea5e9;
}

.sc-online-button__status-dot.is-reconnecting {
  color: #f97316;
}

.sc-online-button__status-dot.is-disconnected {
  color: #ef4444;
}

.sc-online-button__status-ring {
  position: absolute;
  inset: -0.22rem;
  display: block;
  pointer-events: none;
  transform-origin: center;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 9999px;
  opacity: 0.9;
  animation: sc-online-status-spin 0.9s linear infinite;
}

.sc-online-button__status-dot.is-busy {
  animation: sc-online-status-breathe 1.2s ease-in-out infinite;
}

.sc-online-button__status-dot.is-recovering {
  animation: sc-online-status-pulse 0.5s ease-out;
}

@keyframes sc-online-status-spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

@keyframes sc-online-status-breathe {
  0%,
  100% {
    opacity: 0.72;
  }

  50% {
    opacity: 1;
  }
}

@keyframes sc-online-status-pulse {
  0% {
    box-shadow: 0 0 0 1px rgba(15, 23, 42, 0.16), 0 0 0 0 currentColor;
  }

  55% {
    box-shadow: 0 0 0 1px rgba(15, 23, 42, 0.16), 0 0 0 0.28rem transparent;
  }

  100% {
    box-shadow: 0 0 0 1px rgba(15, 23, 42, 0.16), 0 0 0 0 currentColor;
  }
}

.online-badge--changed {
  animation: sc-online-badge-pop 0.2s ease-out;
}

@keyframes sc-online-badge-pop {
  0% {
    transform: scale(0.9);
  }

  65% {
    transform: scale(1.06);
  }

  100% {
    transform: scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .sc-online-button__status-dot.is-recovering,
  .sc-online-button__status-dot.is-busy,
  .online-badge--changed {
    animation: none;
  }
}

.sc-overlay-layer {
  pointer-events: auto;
  z-index: 1500;
}
</style>
