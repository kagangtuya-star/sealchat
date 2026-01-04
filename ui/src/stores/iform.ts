import { defineStore } from 'pinia';
import { watch } from 'vue';
import { api } from './_config';
import { chatEvent, useChatStore } from './chat';
import { useUserStore } from './user';
import type { ChannelIForm, ChannelIFormEventPayload, ChannelIFormStatePayload } from '@/types/iform';

interface PanelState {
  formId: string;
  height: number;
  collapsed: boolean;
  forcing: boolean;
  fromPush: boolean;
  autoPlayHint: boolean;
  autoUnmuteHint: boolean;
}

interface FloatingState extends PanelState {
  width: number;
  x: number;
  y: number;
  minimized: boolean;
  zIndex: number;
  floating: true;
}

type IFormSurface = 'panel' | 'floating' | 'drawer';

interface CapabilitySnapshot {
  manage: boolean;
  broadcast: boolean;
}

interface EmbedHostCollection {
  panel?: HTMLElement | null;
  floating?: HTMLElement | null;
  drawer?: HTMLElement | null;
}

interface IFormStoreState {
  currentChannelId: string | null;
  drawerVisible: boolean;
  loading: boolean;
  saving: boolean;
  migrating: boolean;
  bootstrapped: boolean;
  zCounter: number;
  selectedFormIds: string[];
  formsByChannel: Record<string, ChannelIForm[]>;
  panelsByChannel: Record<string, Record<string, PanelState>>;
  floatingByChannel: Record<string, Record<string, FloatingState>>;
  attentionChannels: Record<string, boolean>;
  capabilities: Record<string, CapabilitySnapshot>;
  embedHostsByChannel: Record<string, Record<string, EmbedHostCollection>>;
}

let gatewayBound = false;
let gatewayHandler: ((event: any) => void) | null = null;

export const useIFormStore = defineStore('iform', {
  state: (): IFormStoreState => ({
    currentChannelId: null,
    drawerVisible: false,
    loading: false,
    saving: false,
    migrating: false,
    bootstrapped: false,
    zCounter: 32,
    selectedFormIds: [],
    formsByChannel: Object.create(null),
    panelsByChannel: Object.create(null),
    floatingByChannel: Object.create(null),
    attentionChannels: Object.create(null),
    capabilities: Object.create(null),
    embedHostsByChannel: Object.create(null),
  }),
  getters: {
    currentForms(state): ChannelIForm[] {
      if (!state.currentChannelId) {
        return [];
      }
      return state.formsByChannel[state.currentChannelId] || [];
    },
    currentPanels(state): PanelState[] {
      if (!state.currentChannelId) {
        return [];
      }
      const map = state.panelsByChannel[state.currentChannelId];
      return map ? Object.values(map) : [];
    },
    currentFloatingWindows(state): FloatingState[] {
      if (!state.currentChannelId) {
        return [];
      }
      const map = state.floatingByChannel[state.currentChannelId];
      return map ? Object.values(map).sort((a, b) => a.zIndex - b.zIndex) : [];
    },
    hasInlinePanels(): boolean {
      return this.currentPanels.length > 0;
    },
    hasFloatingWindows(): boolean {
      return this.currentFloatingWindows.length > 0;
    },
    canManage(state): boolean {
      const channelId = state.currentChannelId;
      if (!channelId) return false;
      return !!state.capabilities[channelId]?.manage;
    },
    canBroadcast(state): boolean {
      const channelId = state.currentChannelId;
      if (!channelId) return false;
      return !!state.capabilities[channelId]?.broadcast;
    },
    hasAttention(state): boolean {
      const channelId = state.currentChannelId;
      if (!channelId) {
        return false;
      }
      return !!state.attentionChannels[channelId];
    },
    activeEmbedFormIds(state): string[] {
      const channelId = state.currentChannelId;
      if (!channelId) {
        return [];
      }
      const hosts = state.embedHostsByChannel[channelId];
      if (!hosts) {
        return [];
      }
      return Object.keys(hosts).filter((formId) => {
        const registry = hosts[formId];
        return !!(registry?.floating || registry?.panel || registry?.drawer);
      });
    },
  },
  actions: {
    bootstrap() {
      if (this.bootstrapped) {
        return;
      }
      this.bootstrapped = true;
      const chat = useChatStore();
      const user = useUserStore();
      watch(
        () => chat.curChannel?.id,
        (channelId) => {
          this.setActiveChannel(channelId || null);
          if (channelId) {
            this.ensureForms(channelId);
            this.refreshCapabilities(channelId);
          }
        },
        { immediate: true },
      );
      watch(
        () => user.info.id,
        () => {
          if (this.currentChannelId) {
            this.refreshCapabilities(this.currentChannelId, true);
          }
        },
      );
      if (!gatewayBound) {
        const store = this;
        gatewayHandler = (event: any) => store.handleGatewayEvent(event);
        chatEvent.on('channel-iform-updated' as any, gatewayHandler);
        chatEvent.on('channel-iform-pushed' as any, gatewayHandler);
        gatewayBound = true;
      }
    },
    async ensureForms(channelId: string, force = false) {
      if (!channelId) {
        return;
      }
      if (!force && this.formsByChannel[channelId]) {
        return;
      }
      this.loading = true;
      try {
        const { data } = await api.get<{ items: ChannelIForm[] }>(`api/v1/channels/${channelId}/iforms`);
        this.formsByChannel = {
          ...this.formsByChannel,
          [channelId]: data?.items || [],
        };
        this.cleanRuntimeState(channelId);
      } finally {
        this.loading = false;
      }
    },
    async refreshCapabilities(channelId: string, force = false) {
      if (!channelId) {
        return;
      }
      if (!force && this.capabilities[channelId]) {
        return;
      }
      const chat = useChatStore();
      const user = useUserStore();
      const userId = user.info.id;
      if (!userId) {
        this.capabilities[channelId] = { manage: false, broadcast: false };
        return;
      }
      const [manageBase, broadcastBase] = await Promise.all([
        chat.hasChannelPermission(channelId, 'func_channel_iform_manage'),
        chat.hasChannelPermission(channelId, 'func_channel_iform_broadcast'),
      ]);
      const ownerId = chat.getChannelOwnerId(channelId);
      const isOwner = !!ownerId && ownerId === userId;
      const isAdmin = chat.isChannelAdmin(channelId, userId);
      const manage = manageBase || isOwner || isAdmin;
      const broadcast = broadcastBase || isOwner || isAdmin;
      this.capabilities = {
        ...this.capabilities,
        [channelId]: { manage, broadcast },
      };
    },
    setActiveChannel(channelId: string | null) {
      if (this.currentChannelId === channelId) {
        return;
      }
      this.currentChannelId = channelId;
      this.drawerVisible = false;
      this.selectedFormIds = [];
      if (channelId) {
        this.markAttention(channelId, false);
      }
    },
    openDrawer() {
      this.drawerVisible = true;
      if (this.currentChannelId) {
        this.markAttention(this.currentChannelId, false);
      }
    },
    closeDrawer() {
      this.drawerVisible = false;
    },
    toggleDrawer(force?: boolean) {
      if (typeof force === 'boolean') {
        this.drawerVisible = force;
      } else {
        this.drawerVisible = !this.drawerVisible;
      }
      if (this.drawerVisible && this.currentChannelId) {
        this.markAttention(this.currentChannelId, false);
      }
    },
    markAttention(channelId: string, flag: boolean) {
      if (!channelId) {
        return;
      }
      if (!flag && !this.attentionChannels[channelId]) {
        return;
      }
      this.attentionChannels = {
        ...this.attentionChannels,
        [channelId]: flag,
      };
    },
    setSelected(formIds: string[]) {
      this.selectedFormIds = [...new Set(formIds)];
    },
    toggleSelection(formId: string) {
      if (!formId) {
        return;
      }
      if (this.selectedFormIds.includes(formId)) {
        this.selectedFormIds = this.selectedFormIds.filter((id) => id !== formId);
      } else {
        this.selectedFormIds = [...this.selectedFormIds, formId];
      }
    },
    getForm(channelId: string | null, formId: string) {
      if (!channelId) {
        return undefined;
      }
      return (this.formsByChannel[channelId] || []).find((item) => item.id === formId);
    },
    ensurePanelMap(channelId: string) {
      if (!this.panelsByChannel[channelId]) {
        this.panelsByChannel = {
          ...this.panelsByChannel,
          [channelId]: {},
        };
      }
      if (!this.floatingByChannel[channelId]) {
        this.floatingByChannel = {
          ...this.floatingByChannel,
          [channelId]: {},
        };
      }
    },
    openPanel(formId: string, options?: Partial<PanelState>) {
      const channelId = this.currentChannelId;
      if (!channelId || !formId) {
        return;
      }
      this.ensurePanelMap(channelId);
      const form = this.getForm(channelId, formId);
      const baseHeight = options?.height ?? form?.defaultHeight ?? 360;
      const collapsed = options?.collapsed ?? form?.defaultCollapsed ?? false;
      const panel: PanelState = {
        formId,
        height: Math.max(1, Math.round(baseHeight)),
        collapsed,
        forcing: !!options?.forcing,
        fromPush: !!options?.fromPush,
        autoPlayHint: !!options?.autoPlayHint,
        autoUnmuteHint: !!options?.autoUnmuteHint,
      };
      this.panelsByChannel[channelId] = {
        ...this.panelsByChannel[channelId],
        [formId]: panel,
      };
    },
    closePanel(formId: string) {
      const channelId = this.currentChannelId;
      if (!channelId || !this.panelsByChannel[channelId]) {
        return;
      }
      const next = { ...this.panelsByChannel[channelId] };
      delete next[formId];
      this.panelsByChannel = {
        ...this.panelsByChannel,
        [channelId]: next,
      };
    },
    togglePanelCollapse(formId: string) {
      const channelId = this.currentChannelId;
      if (!channelId) {
        return;
      }
      const current = this.panelsByChannel[channelId]?.[formId];
      if (!current) {
        return;
      }
      current.collapsed = !current.collapsed;
    },
    resizePanel(formId: string, height: number) {
      const channelId = this.currentChannelId;
      if (!channelId) {
        return;
      }
      const current = this.panelsByChannel[channelId]?.[formId];
      if (!current) {
        return;
      }
      current.height = Math.max(1, Math.round(height));
    },
    openFloating(formId: string, options?: Partial<FloatingState>) {
      const channelId = this.currentChannelId;
      if (!channelId || !formId) {
        return;
      }
      this.ensurePanelMap(channelId);
      const form = this.getForm(channelId, formId);
      const desiredWidth = Math.max(1, Math.round(options?.width ?? form?.defaultWidth ?? 640));
      const desiredHeight = Math.max(1, Math.round(options?.height ?? form?.defaultHeight ?? 360));
      const maxSize = resolveMaxFloatingSize();
      const viewport = resolveViewport();
      const isMobileViewport = viewport.width < MOBILE_VIEWPORT_WIDTH;
      const baseWidth = isMobileViewport ? maxSize.width : desiredWidth;
      const baseHeight = isMobileViewport ? maxSize.height : desiredHeight;
      const size = clampSize(baseWidth, baseHeight);
      const position = resolveDefaultPosition(size.width, size.height, options);
      const clamped = clampPosition(position.x, position.y, size.width, size.height);
      const state: FloatingState = {
        formId,
        width: size.width,
        height: size.height,
        x: clamped.x,
        y: clamped.y,
        minimized: !!options?.minimized,
        zIndex: ++this.zCounter,
        collapsed: !!options?.collapsed,
        forcing: !!options?.forcing,
        fromPush: !!options?.fromPush,
        autoPlayHint: !!options?.autoPlayHint,
        autoUnmuteHint: !!options?.autoUnmuteHint,
        floating: true,
      };
      this.floatingByChannel[channelId] = {
        ...this.floatingByChannel[channelId],
        [formId]: state,
      };
    },
    closeFloating(formId: string) {
      const channelId = this.currentChannelId;
      if (!channelId || !this.floatingByChannel[channelId]) {
        return;
      }
      const next = { ...this.floatingByChannel[channelId] };
      delete next[formId];
      this.floatingByChannel = {
        ...this.floatingByChannel,
        [channelId]: next,
      };
    },
    toggleFloatingMinimize(formId: string) {
      const state = this.getFloatingState(formId);
      if (!state) {
        return;
      }
      state.minimized = !state.minimized;
      this.bringFloatingToFront(formId);
    },
    updateFloatingPosition(formId: string, x: number, y: number) {
      const state = this.getFloatingState(formId);
      if (!state) {
        return;
      }
      const clamped = clampPosition(x, y, state.width, state.height);
      state.x = clamped.x;
      state.y = clamped.y;
    },
    updateFloatingSize(formId: string, width: number, height: number) {
      const state = this.getFloatingState(formId);
      if (!state) {
        return;
      }
      const size = clampSize(width, height);
      state.width = size.width;
      state.height = size.height;
      const clamped = clampPosition(state.x, state.y, state.width, state.height);
      state.x = clamped.x;
      state.y = clamped.y;
    },
    updateFloatingRect(formId: string, rect: { x: number; y: number; width: number; height: number }) {
      const state = this.getFloatingState(formId);
      if (!state) {
        return;
      }
      const size = clampSize(rect.width, rect.height);
      const clamped = clampPosition(rect.x, rect.y, size.width, size.height);
      state.width = size.width;
      state.height = size.height;
      state.x = clamped.x;
      state.y = clamped.y;
    },
    fitFloatingToViewport(formId: string) {
      const state = this.getFloatingState(formId);
      if (!state) {
        return;
      }
      const maxSize = resolveMaxFloatingSize();
      const clamped = clampPosition(FLOATING_PADDING_X, FLOATING_MIN_Y, maxSize.width, maxSize.height);
      state.width = maxSize.width;
      state.height = maxSize.height;
      state.x = clamped.x;
      state.y = clamped.y;
      state.minimized = false;
      this.bringFloatingToFront(formId);
    },
    bringFloatingToFront(formId: string) {
      const state = this.getFloatingState(formId);
      if (!state) {
        return;
      }
      state.zIndex = ++this.zCounter;
    },
    getFloatingState(formId: string): FloatingState | undefined {
      const channelId = this.currentChannelId;
      if (!channelId) {
        return undefined;
      }
      return this.floatingByChannel[channelId]?.[formId];
    },
    async createForm(payload: Partial<ChannelIForm> & { name: string }) {
      const channelId = this.currentChannelId;
      if (!channelId) {
        throw new Error('未选择频道');
      }
      this.saving = true;
      try {
        await api.post(`api/v1/channels/${channelId}/iforms`, payload);
        await this.ensureForms(channelId, true);
      } finally {
        this.saving = false;
      }
    },
    async updateForm(formId: string, payload: Record<string, unknown>) {
      const channelId = this.currentChannelId;
      if (!channelId) {
        throw new Error('未选择频道');
      }
      this.saving = true;
      try {
        await api.patch(`api/v1/channels/${channelId}/iforms/${formId}`, payload);
        await this.ensureForms(channelId, true);
      } finally {
        this.saving = false;
      }
    },
    async deleteForm(formId: string) {
      const channelId = this.currentChannelId;
      if (!channelId) {
        throw new Error('未选择频道');
      }
      await api.delete(`api/v1/channels/${channelId}/iforms/${formId}`);
      await this.ensureForms(channelId, true);
      this.closePanel(formId);
      this.closeFloating(formId);
    },
    async pushStates(states: ChannelIFormStatePayload[], options?: { targetUserIds?: string[]; force?: boolean }) {
      const channelId = this.currentChannelId;
      if (!channelId) {
        throw new Error('未选择频道');
      }
      if (!states.length) {
        throw new Error('至少选择一个控件');
      }
      await api.post(`api/v1/channels/${channelId}/iforms/push`, {
        states,
        force: options?.force,
        targetUserIds: options?.targetUserIds,
      });
    },
    async migrateForms(targetIds: string[], formIds: string[], mode: 'copy' | 'move') {
      const channelId = this.currentChannelId;
      if (!channelId) {
        throw new Error('未选择频道');
      }
      if (!targetIds.length) {
        throw new Error('请选择目标频道');
      }
      this.migrating = true;
      try {
        await api.post(`api/v1/channels/${channelId}/iforms/migrate`, {
          targetChannelIds: targetIds,
          formIds,
          mode,
        });
        await this.ensureForms(channelId, true);
      } finally {
        this.migrating = false;
      }
    },
    applySnapshot(channelId: string, payload?: ChannelIFormEventPayload) {
      const forms = payload?.forms || [];
      this.formsByChannel = {
        ...this.formsByChannel,
        [channelId]: forms,
      };
      if (channelId === this.currentChannelId) {
        this.markAttention(channelId, false);
      }
      this.cleanRuntimeState(channelId);
      if (this.currentChannelId === channelId) {
        this.selectedFormIds = this.selectedFormIds.filter((id) => forms.some((form) => form.id === id));
      }
    },
    applyPush(channelId: string, payload?: ChannelIFormEventPayload) {
      const states = payload?.states || (payload?.state ? [payload.state] : []);
      if (!states.length) {
        return;
      }
      this.mergeForms(channelId, payload?.forms);
      if (this.currentChannelId !== channelId) {
        this.markAttention(channelId, true);
      }
      const prevChannel = this.currentChannelId;
      this.currentChannelId = channelId;
      states.forEach((state) => {
        if (state.floating) {
          this.openFloating(state.formId, {
            width: state.width,
            height: state.height,
            x: state.x,
            y: state.y,
            minimized: state.minimized,
            forcing: !!state.force,
            fromPush: true,
            autoPlayHint: !!state.autoPlay,
            autoUnmuteHint: !!state.autoUnmute,
          });
        } else {
          this.openPanel(state.formId, {
            height: state.height,
            collapsed: !!state.collapsed,
            forcing: !!state.force,
            fromPush: true,
            autoPlayHint: !!state.autoPlay,
            autoUnmuteHint: !!state.autoUnmute,
          });
        }
      });
      this.currentChannelId = prevChannel;
    },
    mergeForms(channelId: string, forms?: ChannelIForm[]) {
      if (!forms?.length) {
        return;
      }
      const existing = this.formsByChannel[channelId] || [];
      const map = new Map(existing.map((item) => [item.id, item]));
      forms.forEach((item) => {
        if (!item) {
          return;
        }
        const prev = map.get(item.id) || {};
        map.set(item.id, { ...prev, ...item });
      });
      this.formsByChannel = {
        ...this.formsByChannel,
        [channelId]: Array.from(map.values()),
      };
    },
    handleGatewayEvent(event: { type?: string; channel?: { id?: string }; iform?: ChannelIFormEventPayload }) {
      const channelId = event?.channel?.id;
      if (!channelId) {
        return;
      }
      if (event.type === 'channel-iform-updated') {
        this.applySnapshot(channelId, event.iform);
        return;
      }
      if (event.type === 'channel-iform-pushed') {
        this.applyPush(channelId, event.iform);
      }
    },
    cleanRuntimeState(channelId: string) {
      const forms = this.formsByChannel[channelId] || [];
      const validIds = new Set(forms.map((item) => item.id));
      const panels = this.panelsByChannel[channelId];
      if (panels) {
        const nextPanels: Record<string, PanelState> = {};
        Object.entries(panels).forEach(([formId, state]) => {
          if (validIds.has(formId)) {
            nextPanels[formId] = state;
          }
        });
        this.panelsByChannel = {
          ...this.panelsByChannel,
          [channelId]: nextPanels,
        };
      }
      const floating = this.floatingByChannel[channelId];
      if (floating) {
        const nextFloating: Record<string, FloatingState> = {};
        Object.entries(floating).forEach(([formId, state]) => {
          if (validIds.has(formId)) {
            nextFloating[formId] = state;
          }
        });
        this.floatingByChannel = {
          ...this.floatingByChannel,
          [channelId]: nextFloating,
        };
      }
      const hostRegistry = this.embedHostsByChannel[channelId];
      if (hostRegistry) {
        const nextHosts: Record<string, EmbedHostCollection> = {};
        Object.entries(hostRegistry).forEach(([formId, registry]) => {
          if (validIds.has(formId)) {
            nextHosts[formId] = registry;
          }
        });
        this.embedHostsByChannel = {
          ...this.embedHostsByChannel,
          [channelId]: nextHosts,
        };
      }
    },
    registerEmbedHost(formId: string, el: HTMLElement, surface: IFormSurface, channelId?: string | null) {
      const targetChannel = channelId ?? this.currentChannelId;
      if (!targetChannel || !formId || !el) {
        return;
      }
      this.ensureHostRegistry(targetChannel, formId);
      const registry = this.embedHostsByChannel[targetChannel];
      const nextSurface: EmbedHostCollection = {
        ...(registry[formId] || {}),
        [surface]: el,
      };
      this.embedHostsByChannel = {
        ...this.embedHostsByChannel,
        [targetChannel]: {
          ...registry,
          [formId]: nextSurface,
        },
      };
    },
    unregisterEmbedHost(formId: string, surface: IFormSurface, el?: HTMLElement | null, channelId?: string | null) {
      const targetChannel = channelId ?? this.currentChannelId;
      if (!targetChannel) {
        return;
      }
      const registry = this.embedHostsByChannel[targetChannel];
      if (!registry?.[formId]) {
        return;
      }
      const current = registry[formId];
      if (el && current[surface] && current[surface] !== el) {
        return;
      }
      const nextSurface: EmbedHostCollection = {
        ...current,
        [surface]: null,
      };
      const hasAny = nextSurface.floating || nextSurface.panel || nextSurface.drawer;
      const nextRegistry = { ...registry };
      if (hasAny) {
        nextRegistry[formId] = nextSurface;
      } else {
        delete nextRegistry[formId];
      }
      this.embedHostsByChannel = {
        ...this.embedHostsByChannel,
        [targetChannel]: nextRegistry,
      };
    },
    resolveEmbedHost(formId: string, channelId?: string | null): HTMLElement | null {
      const targetChannel = channelId ?? this.currentChannelId;
      if (!targetChannel) {
        return null;
      }
      const registry = this.embedHostsByChannel[targetChannel]?.[formId];
      if (!registry) {
        return null;
      }
      return registry.floating || registry.panel || registry.drawer || null;
    },
    ensureHostRegistry(channelId: string, formId: string) {
      if (!this.embedHostsByChannel[channelId]) {
        this.embedHostsByChannel = {
          ...this.embedHostsByChannel,
          [channelId]: {},
        };
      }
      if (!this.embedHostsByChannel[channelId][formId]) {
        this.embedHostsByChannel = {
          ...this.embedHostsByChannel,
          [channelId]: {
            ...this.embedHostsByChannel[channelId],
            [formId]: {},
          },
        };
      }
    },
  },
});

const FLOATING_MIN_WIDTH = 240;
const FLOATING_MIN_HEIGHT = 200;
const FLOATING_PADDING_X = 16;
const FLOATING_PADDING_Y = 16;
const FLOATING_MIN_Y = 48;
const MOBILE_VIEWPORT_WIDTH = 768;

function resolveViewport() {
  if (typeof window === 'undefined') {
    return { width: 1280, height: 720 };
  }
  return { width: window.innerWidth || 1280, height: window.innerHeight || 720 };
}

function resolveMaxFloatingSize() {
  const viewport = resolveViewport();
  return {
    width: Math.max(FLOATING_MIN_WIDTH, viewport.width - FLOATING_PADDING_X * 2),
    height: Math.max(FLOATING_MIN_HEIGHT, viewport.height - FLOATING_MIN_Y - FLOATING_PADDING_Y),
  };
}

function clampSize(width: number, height: number) {
  const maxSize = resolveMaxFloatingSize();
  return {
    width: Math.min(Math.max(FLOATING_MIN_WIDTH, Math.round(width)), maxSize.width),
    height: Math.min(Math.max(FLOATING_MIN_HEIGHT, Math.round(height)), maxSize.height),
  };
}

function resolveDefaultPosition(width: number, height: number, options?: Partial<FloatingState>) {
  if (typeof window === 'undefined') {
    return { x: options?.x ?? 120, y: options?.y ?? 120 };
  }
  const viewportWidth = window.innerWidth || 1280;
  const viewportHeight = window.innerHeight || 720;
  const defaultX = Math.max(FLOATING_PADDING_X, (viewportWidth - width) / 2);
  const defaultY = Math.max(FLOATING_MIN_Y, (viewportHeight - height) / 3);
  return {
    x: options?.x ?? defaultX,
    y: options?.y ?? defaultY,
  };
}

function clampPosition(x: number, y: number, width: number, height: number) {
  if (typeof window === 'undefined') {
    return { x, y };
  }
  const maxX = Math.max(FLOATING_PADDING_X, window.innerWidth - width - FLOATING_PADDING_X);
  const maxY = Math.max(FLOATING_MIN_Y, window.innerHeight - height - FLOATING_PADDING_Y);
  return {
    x: Math.min(Math.max(x, FLOATING_PADDING_X), maxX),
    y: Math.min(Math.max(y, FLOATING_MIN_Y), maxY),
  };
}
