<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useDialog, useMessage } from 'naive-ui';
import { LayoutGrid, LayoutList, Search, Star, StarOff, UserCircle } from '@vicons/tabler';
import { useRouter } from 'vue-router';
import { setLocale, setLocaleByNavigator } from '@/lang';
import { useI18n } from 'vue-i18n';
import { useUserStore } from '@/stores/user';
import UserProfile from '@/views/components/user-profile.vue';

type LobbyMode = 'mine' | 'explore';
type WorldLobbyViewMode = 'list' | 'grid';

interface PaginationState {
  page: number;
  pageSize: number;
  total: number;
}

interface FetchOptions {
  keyword?: string;
  page?: number;
  pageSize?: number;
}

interface GridTetherState {
  worldId: string;
  mouseX: number;
  mouseY: number;
  buttonX: number;
  buttonY: number;
  proximity: number;
  beamWidth: number;
}

const DEFAULT_PAGE_SIZE = 20;
const PAGE_SIZES = [10, 20, 50];
const MAX_DESCRIPTION_LENGTH = 30;
const DESCRIPTION_LINE_LENGTH = 11;
const WORLD_VIEW_MODE_STORAGE_KEY = 'sc.world-lobby.view-mode';
const GRID_TETHER_MAX_DISTANCE = 260;
const GRID_ENTER_PULSE_DELAY_MS = 40;
const GRID_ENTER_COLLAPSE_DELAY_MS = 90;

const isWorldLobbyViewMode = (value: unknown): value is WorldLobbyViewMode => value === 'list' || value === 'grid';

const readStoredViewMode = (): WorldLobbyViewMode => {
  if (typeof window === 'undefined') {
    return 'grid';
  }
  try {
    const raw = window.localStorage.getItem(WORLD_VIEW_MODE_STORAGE_KEY);
    return isWorldLobbyViewMode(raw) ? raw : 'grid';
  } catch {
    return 'grid';
  }
};

const chat = useChatStore();
const user = useUserStore();
const message = useMessage();
const dialog = useDialog();
const router = useRouter();
const { t } = useI18n();
const AdminSettings = defineAsyncComponent(() => import('@/views/admin/admin-settings.vue'));

const loading = ref(false);
const inviteSlug = ref('');
const joining = ref(false);
const searchKeyword = ref('');
const createVisible = ref(false);
const creating = ref(false);
const userProfileShow = ref(false);
const adminShow = ref(false);
const viewMode = ref<WorldLobbyViewMode>(readStoredViewMode());
const requestSeq = ref(0);
const gridActionOpenWorldId = ref<string | null>(null);
const mobileGridActionMode = ref(false);
const gridTetherState = ref<GridTetherState | null>(null);
const gridEnterPulseWorldId = ref<string | null>(null);
const gridEnterCollapseWorldId = ref<string | null>(null);
const gridEnteringWorldId = ref<string | null>(null);
let mobileGridActionMediaQuery: MediaQueryList | null = null;

const minePagination = ref<PaginationState>({
  page: 1,
  pageSize: DEFAULT_PAGE_SIZE,
  total: 0,
});

const explorePagination = ref<PaginationState>({
  page: 1,
  pageSize: DEFAULT_PAGE_SIZE,
  total: 0,
});

const createForm = ref({
  name: '',
  description: '',
  visibility: 'public',
});

const normalizePositiveInt = (value: unknown, fallback: number) => {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) {
    return fallback;
  }
  return Math.floor(num);
};

const normalizeNonNegativeInt = (value: unknown, fallback: number) => {
  const num = Number(value);
  if (!Number.isFinite(num) || num < 0) {
    return fallback;
  }
  return Math.floor(num);
};

const beginRequest = () => {
  const seq = ++requestSeq.value;
  loading.value = true;
  return seq;
};

const isLatestRequest = (seq: number) => seq === requestSeq.value;

const endRequest = (seq: number) => {
  if (isLatestRequest(seq)) {
    loading.value = false;
  }
};

const formatWorldDescription = (description?: string) => {
  const value = (description || '暂无简介').trim() || '暂无简介';
  const limited = Array.from(value).slice(0, MAX_DESCRIPTION_LENGTH);
  const segments: string[] = [];
  for (let i = 0; i < limited.length; i += DESCRIPTION_LINE_LENGTH) {
    segments.push(limited.slice(i, i + DESCRIPTION_LINE_LENGTH).join(''));
  }
  return segments.join('\n');
};

const fetchList = async (options: FetchOptions = {}) => {
  const seq = beginRequest();
  try {
    const keyword = options.keyword ?? searchKeyword.value.trim();
    const page = options.page ?? minePagination.value.page;
    const pageSize = options.pageSize ?? minePagination.value.pageSize;
    const data = await chat.worldList({
      page,
      pageSize,
      joined: true,
      keyword: keyword || undefined,
    });
    if (!isLatestRequest(seq)) {
      return;
    }
    const nextPage = normalizePositiveInt(data?.page, page);
    const nextPageSize = normalizePositiveInt(data?.pageSize, pageSize);
    const nextTotal = normalizeNonNegativeInt(data?.total, 0);
    const maxPage = Math.max(1, Math.ceil(nextTotal / nextPageSize));

    if (nextTotal > 0 && nextPage > maxPage) {
      minePagination.value = {
        page: maxPage,
        pageSize: nextPageSize,
        total: nextTotal,
      };
      await fetchList({ keyword, page: maxPage, pageSize: nextPageSize });
      return;
    }

    minePagination.value = {
      page: nextPage,
      pageSize: nextPageSize,
      total: nextTotal,
    };
  } catch {
    if (isLatestRequest(seq)) {
      message.error('加载世界列表失败');
    }
  } finally {
    endRequest(seq);
  }
};

const fetchExploreList = async (options: FetchOptions = {}) => {
  const seq = beginRequest();
  try {
    const keyword = options.keyword ?? searchKeyword.value.trim();
    const page = options.page ?? explorePagination.value.page;
    const pageSize = options.pageSize ?? explorePagination.value.pageSize;
    const data = await chat.worldListExplore({
      page,
      pageSize,
      visibility: 'public',
      joined: false,
      keyword: keyword || undefined,
    });
    if (!isLatestRequest(seq)) {
      return;
    }
    const nextPage = normalizePositiveInt(data?.page, page);
    const nextPageSize = normalizePositiveInt(data?.pageSize, pageSize);
    const nextTotal = normalizeNonNegativeInt(data?.total, 0);
    const maxPage = Math.max(1, Math.ceil(nextTotal / nextPageSize));

    if (nextTotal > 0 && nextPage > maxPage) {
      explorePagination.value = {
        page: maxPage,
        pageSize: nextPageSize,
        total: nextTotal,
      };
      await fetchExploreList({ keyword, page: maxPage, pageSize: nextPageSize });
      return;
    }

    explorePagination.value = {
      page: nextPage,
      pageSize: nextPageSize,
      total: nextTotal,
    };
  } catch {
    if (isLatestRequest(seq)) {
      message.error('加载公开世界失败');
    }
  } finally {
    endRequest(seq);
  }
};

const lobbyMode = computed<LobbyMode>(() => (chat.worldLobbyMode === 'explore' ? 'explore' : 'mine'));

const mineWorlds = computed<any[]>(() => chat.worldListCache?.items || []);
const exploreWorlds = computed<any[]>(() => chat.exploreWorldCache?.items || []);
const activeWorlds = computed<any[]>(() => (lobbyMode.value === 'mine' ? mineWorlds.value : exploreWorlds.value));
const activeCardTitle = computed(() => (lobbyMode.value === 'mine' ? '世界列表' : '探索世界'));
const activeEmptyText = computed(() => (lobbyMode.value === 'mine' ? '暂无世界' : '暂无公开世界'));
const activePagination = computed(() => (lobbyMode.value === 'mine' ? minePagination.value : explorePagination.value));
const showPagination = computed(() => activePagination.value.total > activePagination.value.pageSize);

const viewToggleIcon = computed(() => (viewMode.value === 'list' ? LayoutGrid : LayoutList));
const viewToggleLabel = computed(() => (viewMode.value === 'list' ? '网格视图' : '列表视图'));

const refreshCurrentMode = async () => {
  if (lobbyMode.value === 'mine') {
    await fetchList();
  } else {
    await fetchExploreList();
  }
};

const resetAndFetchCurrentMode = async () => {
  if (lobbyMode.value === 'mine') {
    minePagination.value.page = 1;
    await fetchList({ page: 1 });
  } else {
    explorePagination.value.page = 1;
    await fetchExploreList({ page: 1 });
  }
};

const handleSearch = async () => {
  await resetAndFetchCurrentMode();
};

watch(searchKeyword, (val) => {
  if (val === '') {
    void resetAndFetchCurrentMode();
  }
});

watch(activeWorlds, (worlds) => {
  if (!gridActionOpenWorldId.value) {
    if (!gridTetherState.value) {
      return;
    }
  }
  const hasActive = worlds.some(item => item?.world?.id === gridActionOpenWorldId.value);
  if (!hasActive) {
    gridActionOpenWorldId.value = null;
  }
  if (gridTetherState.value) {
    const hasTetherTarget = worlds.some(item => item?.world?.id === gridTetherState.value?.worldId);
    if (!hasTetherTarget) {
      gridTetherState.value = null;
    }
  }
});

watch(viewMode, (mode) => {
  if (mode !== 'grid') {
    gridActionOpenWorldId.value = null;
    gridTetherState.value = null;
    gridEnterPulseWorldId.value = null;
    gridEnterCollapseWorldId.value = null;
    gridEnteringWorldId.value = null;
  }
  if (typeof window === 'undefined') {
    return;
  }
  try {
    window.localStorage.setItem(WORLD_VIEW_MODE_STORAGE_KEY, mode);
  } catch {
    // ignore localStorage failures in private mode or restricted environments
  }
});

const syncMobileGridActionMode = () => {
  mobileGridActionMode.value = Boolean(mobileGridActionMediaQuery?.matches);
  if (!mobileGridActionMode.value) {
    gridActionOpenWorldId.value = null;
  }
  gridTetherState.value = null;
  gridEnterPulseWorldId.value = null;
  gridEnterCollapseWorldId.value = null;
  gridEnteringWorldId.value = null;
};

onMounted(async () => {
  if (typeof window !== 'undefined') {
    mobileGridActionMediaQuery = window.matchMedia('(max-width: 640px), (hover: none), (pointer: coarse)');
    syncMobileGridActionMode();
    if (typeof mobileGridActionMediaQuery.addEventListener === 'function') {
      mobileGridActionMediaQuery.addEventListener('change', syncMobileGridActionMode);
    } else {
      mobileGridActionMediaQuery.addListener(syncMobileGridActionMode);
    }
  }
  await chat.fetchFavoriteWorlds().catch(() => {});
  await refreshCurrentMode();
});

onBeforeUnmount(() => {
  if (!mobileGridActionMediaQuery) {
    return;
  }
  if (typeof mobileGridActionMediaQuery.removeEventListener === 'function') {
    mobileGridActionMediaQuery.removeEventListener('change', syncMobileGridActionMode);
  } else {
    mobileGridActionMediaQuery.removeListener(syncMobileGridActionMode);
  }
});

const enterWorld = async (worldId: string) => {
  try {
    await chat.switchWorld(worldId, { force: true });
    await router.push({ name: 'home' });
  } catch (err: any) {
    message.error(err?.response?.data?.message || '进入世界失败');
  }
};

const consumeInvite = async () => {
  const slug = inviteSlug.value.trim();
  if (!slug) return;
  joining.value = true;
  try {
    const resp = await chat.consumeWorldInvite(slug);
    const worldId = resp.world?.id;
    const worldName = resp.world?.name || '目标世界';
    if (resp.already_joined && worldId) {
      message.info(`您已经加入了「${worldName}」`);
      await chat.switchWorld(worldId, { force: true });
      await router.push({ name: 'world-channel', params: { worldId } });
      return;
    }
    if (worldId) {
      await chat.switchWorld(worldId, { force: true });
      message.success('已加入世界');
      await router.push({ name: 'world-channel', params: { worldId } });
    }
  } catch (e: any) {
    const msg = e?.response?.data?.message || '加入失败';
    message.error(msg);
  } finally {
    joining.value = false;
  }
};

const isWorldFavorited = (worldId: string) => chat.favoriteWorldIds.includes(worldId);

const toggleFavorite = async (worldId: string) => {
  try {
    await chat.toggleWorldFavorite(worldId);
    await refreshCurrentMode();
  } catch (err: any) {
    message.error(err?.response?.data?.message || '更新收藏失败');
  }
};

const isGridCardActionsVisible = (worldId: string) =>
  mobileGridActionMode.value && gridActionOpenWorldId.value === worldId;

const toggleGridCardActions = (worldId: string) => {
  if (!mobileGridActionMode.value) {
    return;
  }
  gridActionOpenWorldId.value = gridActionOpenWorldId.value === worldId ? null : worldId;
};

const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), max);

const updateGridTetherState = (worldId: string, event: MouseEvent) => {
  if (mobileGridActionMode.value) {
    return;
  }
  const cardEl = event.currentTarget;
  if (!(cardEl instanceof HTMLElement)) {
    return;
  }
  const enterButton = cardEl.querySelector('.world-grid-action-btn--enter');
  if (!(enterButton instanceof HTMLElement)) {
    return;
  }

  const cardRect = cardEl.getBoundingClientRect();
  const buttonRect = enterButton.getBoundingClientRect();
  const localWidth = cardEl.clientWidth || cardEl.offsetWidth || cardRect.width;
  const localHeight = cardEl.clientHeight || cardEl.offsetHeight || cardRect.height;
  const scaleX = cardRect.width > 0 ? cardRect.width / Math.max(localWidth, 1) : 1;
  const scaleY = cardRect.height > 0 ? cardRect.height / Math.max(localHeight, 1) : 1;
  const mouseX = clamp((event.clientX - cardRect.left) / Math.max(scaleX, 0.0001), 0, localWidth);
  const mouseY = clamp((event.clientY - cardRect.top) / Math.max(scaleY, 0.0001), 0, localHeight);
  const buttonX = clamp(
    (buttonRect.left + buttonRect.width / 2 - cardRect.left) / Math.max(scaleX, 0.0001),
    0,
    localWidth,
  );
  const buttonY = clamp(
    (buttonRect.top + buttonRect.height / 2 - cardRect.top) / Math.max(scaleY, 0.0001),
    0,
    localHeight,
  );
  const distance = Math.hypot(buttonX - mouseX, buttonY - mouseY);
  const proximity = Math.max(0, 1 - Math.min(distance, GRID_TETHER_MAX_DISTANCE) / GRID_TETHER_MAX_DISTANCE);

  gridTetherState.value = {
    worldId,
    mouseX,
    mouseY,
    buttonX,
    buttonY,
    proximity,
    beamWidth: 1 + proximity * 1.8,
  };
};

const handleGridCardMouseEnter = (item: any, event: MouseEvent) => {
  const worldId = item?.world?.id;
  if (!worldId) {
    return;
  }
  updateGridTetherState(worldId, event);
};

const handleGridCardMouseMove = (item: any, event: MouseEvent) => {
  const worldId = item?.world?.id;
  if (!worldId) {
    return;
  }
  updateGridTetherState(worldId, event);
};

const handleGridCardMouseLeave = (item: any) => {
  const worldId = item?.world?.id;
  if (!worldId) {
    return;
  }
  if (gridTetherState.value?.worldId === worldId) {
    gridTetherState.value = null;
  }
};

const isGridTetherVisible = (worldId: string) =>
  !mobileGridActionMode.value && gridTetherState.value?.worldId === worldId;

const getGridCardFxStyle = (worldId: string): CSSProperties => {
  const state = gridTetherState.value?.worldId === worldId ? gridTetherState.value : null;
  if (!state) {
    return {
      '--world-enter-glow-size': '8px',
      '--world-enter-glow-alpha': '0.14',
    } as CSSProperties;
  }
  return {
    '--world-enter-glow-size': `${(8 + state.proximity * 18).toFixed(2)}px`,
    '--world-enter-glow-alpha': `${(0.18 + state.proximity * 0.58).toFixed(3)}`,
  } as CSSProperties;
};

const getGridTetherBeamStyle = (worldId: string): CSSProperties => {
  const state = gridTetherState.value;
  if (!state || state.worldId !== worldId) {
    return {};
  }
  const dx = state.buttonX - state.mouseX;
  const dy = state.buttonY - state.mouseY;
  const length = Math.hypot(dx, dy);
  const angle = (Math.atan2(dy, dx) * 180) / Math.PI;
  const tailAlpha = (0.2 + state.proximity * 0.2).toFixed(3);
  const headAlpha = (0.58 + state.proximity * 0.42).toFixed(3);
  return {
    left: `${state.mouseX.toFixed(2)}px`,
    top: `${state.mouseY.toFixed(2)}px`,
    width: `${length.toFixed(2)}px`,
    transform: `rotate(${angle.toFixed(2)}deg) scaleX(var(--world-tether-scale, 1))`,
    '--world-tether-width': `${state.beamWidth.toFixed(2)}px`,
    '--world-tether-tail-alpha': tailAlpha,
    '--world-tether-head-alpha': headAlpha,
  } as CSSProperties;
};

const getGridTetherCursorDotStyle = (worldId: string): CSSProperties => {
  const state = gridTetherState.value;
  if (!state || state.worldId !== worldId) {
    return {};
  }
  const size = 3 + state.proximity * 2.2;
  return {
    left: `${state.mouseX.toFixed(2)}px`,
    top: `${state.mouseY.toFixed(2)}px`,
    width: `${size.toFixed(2)}px`,
    height: `${size.toFixed(2)}px`,
    opacity: `${(0.48 + state.proximity * 0.32).toFixed(3)}`,
  };
};

const getGridTetherTargetDotStyle = (worldId: string): CSSProperties => {
  const state = gridTetherState.value;
  if (!state || state.worldId !== worldId) {
    return {};
  }
  const size = 4.8 + state.proximity * 3.4;
  return {
    left: `${state.buttonX.toFixed(2)}px`,
    top: `${state.buttonY.toFixed(2)}px`,
    width: `${size.toFixed(2)}px`,
    height: `${size.toFixed(2)}px`,
    opacity: `${(0.72 + state.proximity * 0.28).toFixed(3)}`,
  };
};

const sleep = (ms: number) => new Promise<void>(resolve => setTimeout(resolve, ms));

const runDesktopGridEnterAnimation = async (worldId: string) => {
  if (gridEnteringWorldId.value) {
    return;
  }
  gridEnteringWorldId.value = worldId;
  gridEnterPulseWorldId.value = worldId;
  await sleep(GRID_ENTER_PULSE_DELAY_MS);
  gridEnterCollapseWorldId.value = worldId;
  await sleep(GRID_ENTER_COLLAPSE_DELAY_MS);
  try {
    await handleGridEnterWorld(worldId);
  } finally {
    gridTetherState.value = null;
    gridEnterPulseWorldId.value = null;
    gridEnterCollapseWorldId.value = null;
    gridEnteringWorldId.value = null;
  }
};

const handleGridCardClick = (item: any) => {
  const worldId = item?.world?.id;
  if (!worldId) {
    return;
  }
  if (mobileGridActionMode.value) {
    toggleGridCardActions(worldId);
    return;
  }
  void runDesktopGridEnterAnimation(worldId);
};

const handleGridFavorite = async (worldId: string) => {
  await toggleFavorite(worldId);
  if (mobileGridActionMode.value) {
    gridActionOpenWorldId.value = null;
  }
};

const handleGridLeaveWorld = (item: any) => {
  confirmLeaveWorld(item);
  if (mobileGridActionMode.value) {
    gridActionOpenWorldId.value = null;
  }
};

const handleGridEnterWorld = async (worldId: string) => {
  await enterWorld(worldId);
  if (mobileGridActionMode.value) {
    gridActionOpenWorldId.value = null;
  }
};

const getWorldRoleTag = (role: string) => {
  switch (role) {
    case 'owner':
      return { label: '拥有者', type: 'warning' as const };
    case 'admin':
      return { label: '管理员', type: 'info' as const };
    case 'spectator':
      return { label: '旁观者', type: 'default' as const };
    case 'member':
      return { label: '成员', type: 'success' as const };
    default:
      return { label: '已加入', type: 'success' as const };
  }
};

const confirmLeaveWorld = (item: any) => {
  if (!item?.world?.id) return;
  if (item.memberRole === 'owner') {
    message.warning('世界创建者无法退出该世界');
    return;
  }
  dialog.warning({
    title: '确认退出世界',
    content: `确定要退出「${item.world.name}」吗？退出后需要重新邀请才能再次进入。`,
    positiveText: '确认退出',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: async () => {
      try {
        await chat.leaveWorld(item.world.id);
        message.success('已退出世界');
        await refreshCurrentMode();
      } catch (error: any) {
        message.error(error?.response?.data?.message || '退出失败');
      }
    },
  });
};

const resetCreateForm = () => {
  createForm.value = {
    name: '',
    description: '',
    visibility: 'public',
  };
};

const handleCreateWorld = async () => {
  if (!createForm.value.name.trim()) {
    message.error('请输入世界名称');
    return;
  }
  creating.value = true;
  try {
    await chat.createWorld({
      name: createForm.value.name,
      description: createForm.value.description,
      visibility: createForm.value.visibility,
    });
    message.success('创建世界成功');
    createVisible.value = false;
    resetCreateForm();
    chat.worldLobbyMode = 'mine';
    minePagination.value.page = 1;
    await fetchList({ page: 1 });
  } catch (err: any) {
    message.error(err?.response?.data?.message || err?.message || '创建世界失败');
  } finally {
    creating.value = false;
  }
};

const switchLobbyMode = async () => {
  if (lobbyMode.value === 'mine') {
    chat.worldLobbyMode = 'explore';
    await fetchExploreList();
  } else {
    chat.worldLobbyMode = 'mine';
    await fetchList();
  }
};

const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'list' ? 'grid' : 'list';
};

const userDisplayName = computed(() => user.info.nick || user.info.username || '个人中心');

const headerMenuOptions = computed(() => [
  {
    label: t('headerMenu.profile'),
    key: 'profile',
  },
  user.checkPerm('mod_admin') ? {
    label: t('headerMenu.admin'),
    key: 'admin',
  } : null,
  {
    label: t('headerMenu.lang'),
    key: 'lang',
    children: [
      {
        label: t('headerMenu.langAuto'),
        key: 'lang:auto',
      },
      {
        label: '简体中文',
        key: 'lang:zh-cn',
      },
      {
        label: 'English',
        key: 'lang:en',
      },
      {
        label: '日本語',
        key: 'lang:ja',
      },
    ],
  },
  {
    label: t('headerMenu.logout'),
    key: 'logout',
  },
].filter(Boolean));

const handleHeaderMenuSelect = (key: string | number) => {
  switch (key) {
    case 'profile':
      adminShow.value = false;
      userProfileShow.value = !userProfileShow.value;
      break;
    case 'admin':
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

const handleMinePageChange = (page: number) => {
  minePagination.value.page = page;
  void fetchList({ page });
};

const handleMinePageSizeChange = (pageSize: number) => {
  minePagination.value.pageSize = pageSize;
  minePagination.value.page = 1;
  void fetchList({ page: 1, pageSize });
};

const handleExplorePageChange = (page: number) => {
  explorePagination.value.page = page;
  void fetchExploreList({ page });
};

const handleExplorePageSizeChange = (pageSize: number) => {
  explorePagination.value.pageSize = pageSize;
  explorePagination.value.page = 1;
  void fetchExploreList({ page: 1, pageSize });
};
</script>

<template>
  <div class="world-lobby-root p-4">
    <div class="world-lobby-header">
      <h2 class="text-lg font-bold">世界大厅</h2>
      <n-space size="small">
        <n-button size="small" quaternary @click="toggleViewMode">
          <template #icon>
            <n-icon>
              <component :is="viewToggleIcon" />
            </n-icon>
          </template>
          {{ viewToggleLabel }}
        </n-button>
        <n-button size="small" @click="refreshCurrentMode" :loading="loading">
          刷新
        </n-button>
        <n-button size="small" type="primary" @click="createVisible = true" v-if="lobbyMode === 'mine'">
          创建世界
        </n-button>
        <n-button size="small" :type="lobbyMode === 'mine' ? 'tertiary' : 'primary'" @click="switchLobbyMode">
          {{ lobbyMode === 'mine' ? '探索世界' : '我的世界' }}
        </n-button>
        <n-dropdown placement="bottom-end" trigger="click" :options="headerMenuOptions" @select="handleHeaderMenuSelect">
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
      </n-space>
    </div>

    <div class="world-toolbar-row">
      <n-input
        v-model:value="searchKeyword"
        size="small"
        clearable
        placeholder="搜索世界或频道"
        @keyup.enter="handleSearch"
        @clear="resetAndFetchCurrentMode"
      >
        <template #prefix>
          <n-icon size="14">
            <Search />
          </n-icon>
        </template>
      </n-input>
      <n-button size="small" type="primary" @click="handleSearch" :loading="loading">搜索</n-button>
    </div>

    <div class="world-toolbar-row">
      <n-input v-model:value="inviteSlug" size="small" placeholder="输入邀请码" />
      <n-button size="small" type="primary" :loading="joining" @click="consumeInvite">通过邀请码加入</n-button>
    </div>

    <template v-if="viewMode === 'list'">
      <n-card :title="activeCardTitle" class="sc-card-scroll">
        <div class="card-body-scroll">
          <n-empty v-if="!activeWorlds.length" :description="activeEmptyText" />

          <div v-else class="world-list">
            <div v-for="item in activeWorlds" :key="item.world.id" class="world-row">
              <div class="flex items-start gap-2">
                <n-button quaternary circle size="tiny" @click="toggleFavorite(item.world.id)">
                  <n-icon
                    size="16"
                    :color="isWorldFavorited(item.world.id) ? 'var(--sc-accent, #f59e0b)' : 'var(--sc-text-secondary, #94a3b8)'"
                  >
                    <component :is="isWorldFavorited(item.world.id) ? Star : StarOff" />
                  </n-icon>
                </n-button>
                <div class="flex-1 min-w-0">
                  <div class="font-bold text-sm flex items-center gap-1">
                    {{ item.world.name }}
                    <n-tag v-if="isWorldFavorited(item.world.id)" size="tiny" type="warning">收藏</n-tag>
                  </div>
                  <div class="text-xs world-desc">{{ formatWorldDescription(item.world.description) }}</div>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <n-tag v-if="item.isMember" size="small" :type="getWorldRoleTag(item.memberRole).type">
                  {{ getWorldRoleTag(item.memberRole).label }}
                </n-tag>
                <n-button
                  v-if="item.isMember && item.memberRole !== 'owner'"
                  size="tiny"
                  quaternary
                  type="error"
                  @click="confirmLeaveWorld(item)"
                >
                  退出
                </n-button>
                <n-button size="tiny" type="primary" @click="enterWorld(item.world.id)">进入</n-button>
              </div>
            </div>
          </div>
        </div>
      </n-card>
    </template>

    <template v-else>
      <div class="world-grid-board">
        <n-empty v-if="!activeWorlds.length" :description="activeEmptyText" />
        <div v-else class="world-grid world-grid--full">
          <div
            v-for="item in activeWorlds"
            :key="item.world.id"
            class="world-grid-card"
            :class="{
              'world-grid-card--actions-open': isGridCardActionsVisible(item.world.id),
              'world-grid-card--enter-pulse': gridEnterPulseWorldId === item.world.id,
              'world-grid-card--enter-collapse': gridEnterCollapseWorldId === item.world.id,
            }"
            :style="getGridCardFxStyle(item.world.id)"
            @mouseenter="handleGridCardMouseEnter(item, $event)"
            @mousemove="handleGridCardMouseMove(item, $event)"
            @mouseleave="handleGridCardMouseLeave(item)"
            @click="handleGridCardClick(item)"
          >
            <div v-if="isGridTetherVisible(item.world.id)" class="world-grid-card__tether-layer">
              <span class="world-grid-card__tether-beam" :style="getGridTetherBeamStyle(item.world.id)" />
              <span
                class="world-grid-card__tether-dot world-grid-card__tether-dot--cursor"
                :style="getGridTetherCursorDotStyle(item.world.id)"
              />
              <span
                class="world-grid-card__tether-dot world-grid-card__tether-dot--target"
                :style="getGridTetherTargetDotStyle(item.world.id)"
              />
            </div>
            <div class="world-grid-card__header">
              <div class="world-grid-card__title-wrap">
                <div class="world-grid-card__title">{{ item.world.name }}</div>
                <div class="world-grid-card__meta">
                  <n-tag v-if="isWorldFavorited(item.world.id)" size="tiny" type="warning">收藏</n-tag>
                  <n-tag v-if="item.isMember" size="tiny" :type="getWorldRoleTag(item.memberRole).type">
                    {{ getWorldRoleTag(item.memberRole).label }}
                  </n-tag>
                  <n-tag size="tiny" :bordered="false">{{ item.memberCount || 0 }} 人</n-tag>
                </div>
              </div>
            </div>
            <div class="world-grid-card__desc">{{ formatWorldDescription(item.world.description) }}</div>
            <div class="world-grid-card__actions">
              <n-button
                quaternary
                circle
                size="small"
                class="world-grid-action-btn world-grid-action-btn--icon"
                @click.stop="handleGridFavorite(item.world.id)"
              >
                <n-icon
                  size="16"
                  :color="isWorldFavorited(item.world.id) ? 'var(--sc-accent, #f59e0b)' : 'var(--sc-text-secondary, #94a3b8)'"
                >
                  <component :is="isWorldFavorited(item.world.id) ? Star : StarOff" />
                </n-icon>
              </n-button>
              <n-button
                v-if="item.isMember && item.memberRole !== 'owner'"
                size="small"
                quaternary
                class="world-grid-action-btn world-grid-action-btn--danger"
                @click.stop="handleGridLeaveWorld(item)"
              >
                退出
              </n-button>
              <n-button
                size="small"
                quaternary
                class="world-grid-action-btn world-grid-action-btn--enter"
                @click.stop="handleGridEnterWorld(item.world.id)"
              >
                进入
              </n-button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <div v-if="showPagination" class="world-pagination">
      <n-pagination
        v-if="lobbyMode === 'mine'"
        size="small"
        :page="minePagination.page"
        :page-size="minePagination.pageSize"
        :item-count="minePagination.total"
        show-size-picker
        :page-sizes="PAGE_SIZES"
        @update:page="handleMinePageChange"
        @update:page-size="handleMinePageSizeChange"
      />
      <n-pagination
        v-else
        size="small"
        :page="explorePagination.page"
        :page-size="explorePagination.pageSize"
        :item-count="explorePagination.total"
        show-size-picker
        :page-sizes="PAGE_SIZES"
        @update:page="handleExplorePageChange"
        @update:page-size="handleExplorePageSizeChange"
      />
    </div>

    <n-modal v-model:show="createVisible" preset="dialog" title="创建世界" style="max-width: 420px">
      <n-form label-width="72">
        <n-form-item label="名称">
          <n-input v-model:value="createForm.name" placeholder="输入世界名称" />
        </n-form-item>
        <n-form-item label="简介">
          <n-input
            v-model:value="createForm.description"
            type="textarea"
            placeholder="简单介绍这个世界"
            maxlength="30"
            show-count
          />
        </n-form-item>
        <n-form-item label="可见性">
          <n-select
            v-model:value="createForm.visibility"
            :options="[
              { label: '公开', value: 'public' },
              { label: '私有', value: 'private' },
              { label: '隐藏链接', value: 'unlisted' },
            ]"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button
            quaternary
            @click="() => {
              createVisible = false;
              resetCreateForm();
            }"
          >
            取消
          </n-button>
          <n-button type="primary" :loading="creating" @click="handleCreateWorld">创建</n-button>
        </n-space>
      </template>
    </n-modal>

    <div
      v-if="userProfileShow"
      class="world-lobby-overlay world-lobby-overlay--profile sc-overlay-layer"
    >
      <UserProfile @close="userProfileShow = false" />
    </div>
    <div
      v-if="adminShow"
      class="world-lobby-overlay world-lobby-overlay--admin sc-overlay-layer"
    >
      <AdminSettings @close="adminShow = false" />
    </div>
  </div>
</template>

<style scoped>
.world-lobby-root {
  position: relative;
  width: 100%;
  min-height: 100vh;
  min-height: 100dvh;
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: hidden;
  overflow-x: clip;
  box-sizing: border-box;
  background-color: var(--sc-bg-page);
}

/* Fix subtle left-edge seam in dark palettes on some mobile browsers */
.world-lobby-root::before {
  content: '';
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  width: 1px;
  background-color: var(--sc-bg-page);
  pointer-events: none;
  z-index: 1;
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

.sc-overlay-layer {
  pointer-events: auto;
  z-index: 2600;
}

.world-lobby-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 18px;
  background-color: rgba(15, 23, 42, 0.48);
  backdrop-filter: blur(2px) saturate(108%);
}

.world-lobby-overlay--profile :deep(.sc-form-scroll) {
  width: min(680px, calc(100vw - 36px));
  max-width: 680px;
  max-height: min(88vh, calc(100vh - 36px));
  overflow: auto;
  background-color: var(--sc-bg-surface);
  border: 1px solid var(--sc-border-mute);
  box-shadow:
    0 20px 46px rgba(15, 23, 42, 0.2),
    0 2px 12px rgba(15, 23, 42, 0.12);
}

.world-lobby-overlay--admin :deep(.sc-admin-settings-shell) {
  width: min(1120px, calc(100vw - 36px));
  max-height: min(88vh, calc(100vh - 36px));
  margin-top: 0 !important;
  overflow: auto;
  background-color: var(--sc-bg-surface);
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  box-shadow:
    0 20px 46px rgba(15, 23, 42, 0.2),
    0 2px 12px rgba(15, 23, 42, 0.12);
}

:global(:root[data-display-palette='day']) .world-lobby-overlay {
  background-color: rgba(148, 163, 184, 0.28);
}

:global(:root[data-display-palette='night']) .world-lobby-overlay {
  background-color: rgba(2, 6, 23, 0.62);
}

@media (max-width: 640px) {
  .world-lobby-root::before {
    width: 2px;
  }

  .world-lobby-overlay {
    padding: 10px;
  }

  .world-lobby-overlay--profile :deep(.sc-form-scroll) {
    width: calc(100vw - 20px);
    max-height: calc(100vh - 20px);
    border-radius: 12px;
    padding: 12px;
  }

  .world-lobby-overlay--admin :deep(.sc-admin-settings-shell) {
    width: calc(100vw - 20px);
    max-height: calc(100vh - 20px);
    min-height: 0 !important;
    border-radius: 12px;
    padding: 12px;
  }
}

.world-lobby-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.world-toolbar-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.world-toolbar-row :deep(.n-input) {
  flex: 1;
  min-width: 220px;
}

.sc-card-scroll {
  max-height: 520px;
}

.card-body-scroll {
  max-height: 360px;
  overflow: auto;
  padding-right: 4px;
}

.world-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.world-desc {
  white-space: pre-line;
  color: var(--sc-text-secondary);
}

.world-row {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: start;
  gap: 8px;
  padding: 10px;
  border-radius: 10px;
  border: 1px solid var(--sc-border-mute);
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.world-row:hover {
  background-color: var(--sc-chip-bg);
  border-color: var(--sc-border-strong);
}

.world-grid-board {
  width: 100%;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;
  scrollbar-gutter: stable;
  scrollbar-width: thin;
  scrollbar-color: var(--sc-scrollbar-thumb) transparent;
}

.world-grid-board::-webkit-scrollbar {
  width: 5px;
}

.world-grid-board::-webkit-scrollbar-track {
  background: transparent;
}

.world-grid-board::-webkit-scrollbar-thumb {
  background: var(--sc-scrollbar-thumb);
  border-radius: 999px;
}

.world-grid-board::-webkit-scrollbar-thumb:hover {
  background: var(--sc-scrollbar-thumb-hover);
}

.world-grid {
  display: grid;
  gap: 12px;
  align-content: start;
  grid-auto-rows: minmax(186px, auto);
}

.world-grid--full {
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
}

.world-grid-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  position: relative;
  overflow: hidden;
  cursor: pointer;
  min-height: 186px;
  padding: 12px;
  border-radius: 12px;
  border: 1px solid var(--sc-border-mute);
  background: linear-gradient(160deg, var(--sc-bg-layer-strong), var(--sc-bg-surface));
  transition: transform 0.18s ease, border-color 0.22s ease, box-shadow 0.22s ease;
}

.world-grid-card:hover,
.world-grid-card:focus-within,
.world-grid-card--actions-open {
  transform: translateY(-2px) scale(1.012);
  border-color: var(--sc-border-strong);
  box-shadow:
    0 15px 28px color-mix(in srgb, var(--sc-fg-primary) 14%, transparent),
    0 2px 8px color-mix(in srgb, var(--sc-fg-primary) 10%, transparent);
}

.world-grid-card__header {
  position: relative;
  z-index: 2;
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.world-grid-card__title-wrap {
  flex: 1;
  min-width: 0;
}

.world-grid-card__title {
  color: var(--sc-text-primary);
  font-weight: 700;
  font-size: 14px;
  line-height: 1.4;
  word-break: break-word;
}

.world-grid-card__meta {
  margin-top: 6px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.world-grid-card__desc {
  position: relative;
  z-index: 2;
  flex: 1;
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-line;
}

.world-grid-card__tether-layer {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
  overflow: hidden;
}

.world-grid-card__tether-beam {
  position: absolute;
  height: var(--world-tether-width, 1px);
  border-radius: 999px;
  transform-origin: left center;
  background: linear-gradient(
    90deg,
    rgba(51, 136, 222, var(--world-tether-tail-alpha, 0.2)) 0%,
    rgba(51, 136, 222, var(--world-tether-head-alpha, 0.95)) 100%
  );
  filter: saturate(115%);
  opacity: 0.92;
  transition: height 0.12s ease, filter 0.12s ease, opacity 0.12s ease, transform 0.12s ease;
}

.world-grid-card__tether-dot {
  position: absolute;
  border-radius: 999px;
  transform: translate(-50%, -50%);
  background: rgba(51, 136, 222, 0.92);
  transition: transform 0.12s ease, opacity 0.12s ease, box-shadow 0.12s ease;
}

.world-grid-card__tether-dot--cursor {
  box-shadow: 0 0 8px rgba(51, 136, 222, 0.35);
}

.world-grid-card__tether-dot--target {
  box-shadow: 0 0 14px rgba(51, 136, 222, 0.7);
}

.world-grid-card--enter-pulse .world-grid-card__tether-beam {
  height: calc(var(--world-tether-width, 1px) + 1.6px);
  filter: brightness(1.65) saturate(145%);
  opacity: 1;
}

.world-grid-card--enter-pulse .world-grid-card__tether-dot--target {
  transform: translate(-50%, -50%) scale(1.2);
  box-shadow: 0 0 18px rgba(51, 136, 222, 0.98);
}

.world-grid-card--enter-collapse .world-grid-card__tether-beam {
  transform-origin: right center;
  --world-tether-scale: 0.08;
}

.world-grid-card--enter-collapse .world-grid-card__tether-dot--cursor {
  opacity: 0;
}

.world-grid-card__actions {
  position: absolute;
  right: 10px;
  bottom: 10px;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 8px;
  opacity: 0;
  pointer-events: none;
  transform: translateY(8px) scale(0.96);
  transform-origin: right bottom;
  transition: opacity 0.18s ease, transform 0.18s ease;
  padding: 6px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--sc-bg-surface) 92%, transparent);
  border: 1px solid color-mix(in srgb, var(--sc-border-strong) 42%, transparent);
  box-shadow: 0 8px 18px color-mix(in srgb, var(--sc-fg-primary) 10%, transparent);
  backdrop-filter: blur(6px);
}

.world-grid-card:hover .world-grid-card__actions,
.world-grid-card:focus-within .world-grid-card__actions,
.world-grid-card--actions-open .world-grid-card__actions {
  opacity: 1;
  pointer-events: auto;
  transform: translateY(0) scale(1);
}

.world-grid-card :deep(.world-grid-action-btn) {
  border-radius: 9px;
  border: 1px solid color-mix(in srgb, var(--sc-border-strong) 38%, transparent);
  background: color-mix(in srgb, var(--sc-bg-surface) 80%, var(--sc-chip-bg));
  color: var(--sc-text-secondary);
  transition: transform 0.15s ease, border-color 0.2s ease, background-color 0.2s ease, color 0.2s ease;
}

.world-grid-card :deep(.world-grid-action-btn:hover) {
  color: var(--sc-text-primary);
  border-color: color-mix(in srgb, var(--sc-border-strong) 68%, transparent);
  background: color-mix(in srgb, var(--sc-bg-elevated) 86%, var(--sc-chip-bg));
}

.world-grid-card :deep(.world-grid-action-btn--icon) {
  width: 30px;
  min-width: 30px;
  padding: 0;
}

.world-grid-card :deep(.world-grid-action-btn--enter) {
  color: color-mix(in srgb, #3388de 34%, var(--sc-text-primary));
  box-shadow: 0 0 var(--world-enter-glow-size, 8px) rgba(51, 136, 222, var(--world-enter-glow-alpha, 0.14));
}

.world-grid-card :deep(.world-grid-action-btn--danger) {
  color: color-mix(in srgb, #dc2626 60%, var(--sc-text-primary));
}

.world-pagination {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 960px) {
  .world-grid--full {
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  }
}

@media (max-width: 640px) {
  .world-lobby-header {
    align-items: stretch;
  }

  .world-lobby-header :deep(.n-space) {
    width: 100%;
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .world-lobby-header :deep(.n-space .n-button) {
    flex: 1 1 calc(50% - 8px);
  }

  .world-toolbar-row {
    flex-direction: column;
    align-items: stretch;
  }

  .world-row {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .world-grid-board {
    min-height: 0;
    padding-right: 2px;
  }

  .world-grid--full {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .world-grid-card {
    min-height: 148px;
    padding: 10px;
  }

  .world-grid {
    grid-auto-rows: minmax(148px, auto);
  }

  .world-grid-card__actions {
    left: 8px;
    right: 8px;
    bottom: 8px;
    justify-content: flex-end;
  }

  .world-grid-card__actions :deep(.world-grid-action-btn) {
    flex: 1;
    min-width: 0;
  }

  .world-pagination {
    justify-content: center;
  }
}
</style>
