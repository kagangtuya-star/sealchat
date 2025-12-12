import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { WebSocketSubject, webSocket } from 'rxjs/webSocket';
import type { User, Opcode, GatewayPayloadStructure, Channel, EventName, Event, GuildMember } from '@satorijs/protocol'
import type { APIChannelCreateResp, APIChannelListResp, APIMessage, ChannelIdentity, ChannelIdentityFolder, ChannelRoleModel, ExportTaskListResponse, FriendInfo, FriendRequestModel, PaginationListResponse, SatoriMessage, SChannel, UserInfo, UserRoleModel } from '@/types';
import type { AudioPlaybackStatePayload } from '@/types/audio';
import { nanoid } from 'nanoid'
import { groupBy } from 'lodash-es';
import { Emitter } from '@/utils/event';
import { useUserStore } from './user';
import { api, urlBase } from './_config';
import { useAudioStudioStore } from '@/stores/audioStudio';
import { useMessage } from 'naive-ui';
import { memoizeWithTimeout } from '@/utils/tools';
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { PermTreeNode } from '@/types-perm';
import type { DisplaySettings } from './display';
import { useDisplayStore } from './display';

interface ChatState {
  subject: WebSocketSubject<any> | null;
  // user: User,
  channelTree: SChannel[],
  channelTreeByWorld: Record<string, SChannel[]>,
  channelTreeReady: Record<string, boolean>,
  channelTreePrivate: SChannel[],
  channelTreePrivateReady: boolean,
  curChannel: Channel | null,
  currentWorldId: string,
  joinedWorldIds: string[],
  worldListCache: { items: any[]; total: number; page: number; pageSize: number } | null,
  worldLobbyMode: 'mine' | 'explore',
  myWorldCache: { owned: any[]; joined: any[] },
  exploreWorldCache: { items: any[]; total: number; page: number; pageSize: number } | null,
  worldMap: Record<string, any>,
  worldDetailMap: Record<string, any>,
  worldSectionCache: Record<string, any>,
  curMember: GuildMember | null,
  channelCollapseState: Record<string, boolean>,
  connectState: 'connecting' | 'connected' | 'disconnected' | 'reconnecting',
  iReconnectAfterTime: number,
  curReplyTo: SatoriMessage | null; // Message 会报错
  curChannelUsers: User[],
  sidebarTab: 'channels' | 'privateChats',
  atOptionsOn: boolean,

  // 频道未读: id - 数量
  unreadCountMap: { [key: string]: number },

  whisperTarget: User | null,

  messageMenu: {
    show: boolean
    optionsComponent: MenuOptions
    item: SatoriMessage | null
    hasImage: boolean
  },

  avatarMenu: {
    show: boolean,
    optionsComponent: MenuOptions,
    item: SatoriMessage | null
  },

  editing: {
    messageId: string;
    channelId: string;
    originalContent: string;
    draft: string;
    mode?: 'plain' | 'rich';
    isWhisper?: boolean;
    whisperTargetId?: string | null;
    icMode?: 'ic' | 'ooc';
    identityId?: string | null;
    initialIdentityId?: string | null;
    activeIdentityBackup?: string | null;
  } | null

  canReorderAllMessages: boolean;
  channelIdentities: Record<string, ChannelIdentity[]>;
  activeChannelIdentity: Record<string, string>;
  channelIdentityFolders: Record<string, ChannelIdentityFolder[]>;
  channelIdentityFavorites: Record<string, string[]>;
  channelIdentityMembership: Record<string, Record<string, string[]>>;

  // 新增状态
  icMode: 'ic' | 'ooc';
  presenceMap: Record<string, { lastPing: number; latencyMs: number; isFocused: boolean }>;
  isAppFocused: boolean;
  lastPingSentAt: number | null;
  lastLatencyMs: number;
  filterState: {
    icOnly: boolean;
    showArchived: boolean;
    roleIds: string[];
  };
  channelRoleCache: Record<string, string[]>;
  channelMemberRoleMap: Record<string, Record<string, string[]>>;
  channelAdminMap: Record<string, Record<string, boolean>>;
  channelMemberPermMap: Record<string, Record<string, string[]>>;
  botListCache: PaginationListResponse<UserInfo> | null;
  botListCacheUpdatedAt: number;
  favoriteWorldIds: string[];
  channelIcOocRoleConfig: Record<string, { icRoleId: string | null; oocRoleId: string | null }>;
  // 临时显示的归档频道（查看归档频道时使用，切换后清除）
  temporaryArchivedChannel: SChannel | null;
}

const apiMap = new Map<string, any>();
let _connectResolve: any = null;

type myEventName =
  | EventName
  | 'message-created'
  | 'channel-switch-to'
  | 'connected'
  | 'channel-member-updated'
  | 'message-created-notice'
  | 'channel-identity-open'
  | 'channel-identity-updated'
  | 'channel-member-settings-open'
  | 'bot-list-updated'
  | 'global-overlay-toggle';
export const chatEvent = new Emitter<{
  [key in myEventName]: (msg?: Event) => void;
  // 'message-created': (msg: Event) => void;
}>();

let pingTimer: ReturnType<typeof setInterval> | null = null;
let latencyTimer: ReturnType<typeof setInterval> | null = null;
let focusListenersBound = false;
const pendingLatencyProbes: Record<string, number> = {};
const LATENCY_PROBE_TIMEOUT = 8000;

const clearPendingLatencyProbes = () => {
  Object.keys(pendingLatencyProbes).forEach((key) => {
    delete pendingLatencyProbes[key];
  });
};

const cleanupPendingLatencyProbes = () => {
  const now = Date.now();
  Object.entries(pendingLatencyProbes).forEach(([key, sentAt]) => {
    if (now - sentAt > LATENCY_PROBE_TIMEOUT) {
      delete pendingLatencyProbes[key];
    }
  });
};

export const useChatStore = defineStore({
  id: 'chat',
  state: (): ChatState => ({
    // user: { id: '1', },
    subject: null,
    channelTree: [] as any,
    channelTreeByWorld: {},
    channelTreeReady: {},
    channelTreePrivate: [] as any,
    channelTreePrivateReady: false,
    curChannel: null,
    currentWorldId: localStorage.getItem('currentWorldId') || '',
    joinedWorldIds: [],
    worldListCache: null,
    worldLobbyMode: 'mine',
    myWorldCache: { owned: [], joined: [] },
    exploreWorldCache: null,
    worldMap: {},
    worldDetailMap: {},
    worldSectionCache: {},
    curMember: null,
    channelCollapseState: {},
    connectState: 'connecting',
    iReconnectAfterTime: 0,
    curReplyTo: null,
    curChannelUsers: [],

    sidebarTab: 'channels',
    unreadCountMap: {},

    // 太遮挡视线，先关闭了
    atOptionsOn: false,

    whisperTarget: null,

    messageMenu: {
      show: false,
      optionsComponent: {
        iconFontClass: 'iconfont',
        customClass: "class-a",
        zIndex: 3,
        minWidth: 230,
        x: 500,
        y: 200,
      } as MenuOptions,
      item: null,
      hasImage: false
    },
    avatarMenu: {
      show: false,
      optionsComponent: {
        iconFontClass: 'iconfont',
        customClass: "class-a",
        zIndex: 3,
        minWidth: 230,
        x: 500,
        y: 200,
      } as MenuOptions,
      item: null,
    },

    editing: null,
    canReorderAllMessages: false,
    channelIdentities: {},
    activeChannelIdentity: {},
    channelIdentityFolders: {},
    channelIdentityFavorites: {},
    channelIdentityMembership: {},

    // 新增状态初始值
    icMode: 'ic',
    presenceMap: {},
    isAppFocused: true,
    lastPingSentAt: null,
    lastLatencyMs: 0,
    filterState: {
      icOnly: false,
      showArchived: false,
      roleIds: [],
    },
    channelRoleCache: {},
    channelMemberRoleMap: {},
    channelAdminMap: {},
    channelMemberPermMap: {},
    botListCache: null,
    botListCacheUpdatedAt: 0,
    favoriteWorldIds: (() => {
      if (typeof window === 'undefined') return [];
      try {
        const stored = localStorage.getItem('favoriteWorldIds');
        return stored ? JSON.parse(stored) : [];
      } catch (err) {
        console.warn('parse favoriteWorldIds failed', err);
        return [];
      }
    })(),
    channelIcOocRoleConfig: {},
    temporaryArchivedChannel: null,
  }),

  getters: {
    _lastChannel: (state) => {
      return localStorage.getItem('lastChannel') || '';
    },
    unreadCountPrivate: (state) => {
      return Object.entries(state.unreadCountMap).reduce((sum, [key, count]) => {
        return key.includes(':') ? sum + count : sum;
      }, 0);
    },
    unreadCountPublic: (state) => {
      const collectIds = (nodes?: SChannel[]) => {
        const ids: Set<string> = new Set();
        const traverse = (list?: SChannel[]) => {
          if (!Array.isArray(list)) return;
          list.forEach((item: SChannel) => {
            ids.add(item.id);
            if (Array.isArray(item.children)) {
              traverse(item.children as SChannel[]);
            }
          });
        };
        traverse(nodes);
        return ids;
      };
      const currentTree =
        state.channelTreeByWorld[state.currentWorldId] && state.channelTreeByWorld[state.currentWorldId].length
          ? state.channelTreeByWorld[state.currentWorldId]
          : state.channelTree;
      const validIds = collectIds(currentTree);
      if (validIds.size === 0) {
        return Object.entries(state.unreadCountMap).reduce((sum, [key, count]) => {
          return key.includes(':') ? sum : sum + count;
        }, 0);
      }
      return Object.entries(state.unreadCountMap).reduce((sum, [key, count]) => {
        if (key.includes(':')) return sum;
        return validIds.has(key) ? sum + count : sum;
      }, 0);
    },
    currentWorld(state) {
      if (!state.currentWorldId) return null;
      return state.worldMap[state.currentWorldId] || null;
    },
    currentWorldChannels(state) {
      if (!state.currentWorldId) return [] as SChannel[];
      return state.channelTreeByWorld[state.currentWorldId] || [];
    },
    joinedWorldOptions(state) {
      return state.joinedWorldIds.map((id) => {
        const world = state.worldMap[id];
        return {
          value: id,
          label: world?.name || `世界 ${id.slice(0, 6)}`,
        };
      });
    },
    favoriteWorldSet(state) {
      return new Set(state.favoriteWorldIds);
    },
    ownedWorlds(state) {
      return state.myWorldCache.owned || [];
    },
    joinedWorldsOnly(state) {
      return state.myWorldCache.joined || [];
    },
  },

  actions: {
    async connect() {
      this.stopPingLoop();
      if (!focusListenersBound && typeof window !== 'undefined' && typeof document !== 'undefined') {
        focusListenersBound = true;
        const updateFocusState = () => {
          const hasFocus = typeof document.hasFocus === 'function' ? document.hasFocus() : true;
          const isVisible = document.visibilityState !== 'hidden';
          this.setFocusState(hasFocus && isVisible);
        };
        window.addEventListener('focus', updateFocusState);
        window.addEventListener('blur', updateFocusState);
        document.addEventListener('visibilitychange', updateFocusState);
        updateFocusState();
      }
      const u: User = {
        id: '',
      }
      this.connectState = 'connecting';

      // 'ws://localhost:3212/ws/seal'
      // const subject = webSocket(`ws:${urlBase}/ws/seal`);
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const subject = webSocket(`${protocol}${urlBase}/ws/seal`);

      let isReady = false;

      // 发送协议握手
      // Opcode.IDENTIFY: 3
      const user = useUserStore();
      subject.next({
        op: 3, body: {
          token: user.token,
        }
      });

      subject.subscribe({
        next: (msg: any) => {
          // Opcode.READY
          if (msg.op === 4) {
            console.log('svr ready', msg);
            isReady = true
            this.connectReady();
          } else if (msg.op === 0) {
            // Opcode.EVENT
            const e = msg as Event;
            this.eventDispatch(e);
          } else if (msg.op === 2) {
            this.handlePong();
          } else if (msg.op === 6) {
            this.handleLatencyResult(msg?.body);
          } else if (apiMap.get(msg.echo)) {
            apiMap.get(msg.echo).resolve(msg);
            apiMap.delete(msg.echo);
          }
        },
        error: err => {
          console.log('ws error', err);
          this.subject = null;
          this.connectState = 'disconnected';
          this.stopPingLoop();
          this.reconnectAfter(5, () => {
            try {
              err.target?.close();
              this.subject?.unsubscribe();
              console.log('try close');
            } catch (e) {
              console.log('unsubscribe error', e)
            }
          })
        }, // Called if at any point WebSocket API signals some kind of error.
        complete: () => {
          console.log('complete');
          this.stopPingLoop();
        } // Called when connection is closed (for whatever reason).
      });

      this.subject = subject;
    },

    async reconnectAfter(secs: number, beforeConnect?: Function) {
      setTimeout(async () => {
        this.connectState = 'reconnecting';
        // alert(`连接已断开，${secs} 秒后自动重连`);
        for (let i = secs; i > 0; i--) {
          this.iReconnectAfterTime = i;
          await new Promise(resolve => setTimeout(resolve, 1000));
        }
        if (beforeConnect) beforeConnect();
        this.connect();
      }, 500);
    },

    async connectReady() {
      this.connectState = 'connected';

      chatEvent.emit('connected', undefined);
      this.startPingLoop();
      this.sendPresencePing(true);

      await this.ensureWorldReady();
      if (this.curChannel?.id) {
        await this.channelSwitchTo(this.curChannel?.id);
        const resp2 = await this.sendAPI('channel.member.list.online', { 'channel_id': this.curChannel?.id });
        this.curChannelUsers = resp2.data.data;
      }
      await this.channelList(this.currentWorldId, true);
      await this.ChannelPrivateList();
      await this.channelMembersCountRefresh();

      if (_connectResolve) {
        _connectResolve();
        _connectResolve = null;
      }
    },

    /** try to initialize */
    async tryInit() {
      if (!this.subject) {
        return new Promise((resolve) => {
          _connectResolve = resolve;
          this.connect();
        });
      }
    },

    async ensureConnectionReady() {
      if (this.connectState === 'connected') {
        return;
      }
      if (!this.subject) {
        await this.tryInit();
      }
      if (this.connectState === 'connected') {
        return;
      }
      await new Promise<void>((resolve) => {
        const handler = () => {
          chatEvent.off('connected', handler as any);
          resolve();
        };
        chatEvent.on('connected', handler as any);
      });
    },

    async setReplayTo(item: any) {
      this.curReplyTo = item;
    },

    async sendAPI<T = any>(api: string, data: APIMessage): Promise<T> {
      const echo = nanoid();
      return new Promise((resolve, reject) => {
        apiMap.set(echo, { resolve, reject });
        this.subject?.next({ api, data, echo });
      }).then((resp: any) => {
        if (resp?.err) {
          const error = new Error(resp.err);
          (error as any).response = resp;
          throw error;
        }
        return resp;
      });
    },

    async send(channelId: string, content: string) {
      let msg: APIMessage = {
        // api: 'message.create',
        channel_id: channelId,
        content: content
      }
      this.subject?.next(msg);
    },

    setCurrentWorld(worldId: string) {
      if (!worldId || this.currentWorldId === worldId) return;
      this.currentWorldId = worldId;
      localStorage.setItem('currentWorldId', worldId);
    },

    async initWorlds() {
      if (this.joinedWorldIds.length) {
        const stored = localStorage.getItem('currentWorldId');
        if (stored) {
          this.currentWorldId = stored;
        }
        return;
      }
      try {
        const resp = await api.get('/api/v1/worlds', { params: { joined: true } });
        const items = resp.data.items || [];
        this.joinedWorldIds = items.map((item: any) => item.world.id);
        items.forEach((item: any) => {
          if (item?.world?.id) {
            this.worldMap[item.world.id] = item.world;
          }
        });
        const stored = localStorage.getItem('currentWorldId');
        if (stored && this.joinedWorldIds.includes(stored)) {
          this.currentWorldId = stored;
        } else if (this.joinedWorldIds.length) {
          this.currentWorldId = this.joinedWorldIds[0];
          localStorage.setItem('currentWorldId', this.currentWorldId);
        }
      } catch (err) {
        console.warn('initWorlds failed', err);
      }
    },

    async ensureWorldReady() {
      await this.initWorlds();
      if (!this.currentWorldId && this.joinedWorldIds.length) {
        this.setCurrentWorld(this.joinedWorldIds[0]);
      }
      return !!this.currentWorldId;
    },

    async worldList(params?: { page?: number; pageSize?: number; joined?: boolean; keyword?: string }) {
      const resp = await api.get('/api/v1/worlds', { params });
      const data = resp.data;
      if (Array.isArray(data?.items)) {
        data.items.forEach((item: any) => {
          if (item?.world?.id) {
            this.worldMap[item.world.id] = item.world;
          }
        });
      }
      if (Array.isArray(data?.favoriteWorldIds)) {
        this.favoriteWorldIds = data.favoriteWorldIds;
        this.persistFavoriteWorlds();
      }
      if (params?.joined) {
        const items = data.items || [];
        this.joinedWorldIds = items.map((item: any) => item.world.id);
        const owned = items.filter((item: any) => item?.world?.ownerId === useUserStore().info.id);
        const joined = items.filter((item: any) => item?.world?.ownerId !== useUserStore().info.id);
        this.myWorldCache = { owned, joined };
      }
      this.worldListCache = data;
      return data;
    },

    async worldListExplore(params?: { page?: number; pageSize?: number; keyword?: string; visibility?: string; joined?: boolean }) {
      const resp = await api.get('/api/v1/worlds', {
        params: {
          page: params?.page || 1,
          pageSize: params?.pageSize || 50,
          visibility: params?.visibility || 'public',
          joined: params?.joined,
          keyword: params?.keyword,
        },
      });
      const data = resp.data;
      if (Array.isArray(data?.items)) {
        data.items.forEach((item: any) => {
          if (item?.world?.id) {
            this.worldMap[item.world.id] = item.world;
          }
        });
      }
      this.exploreWorldCache = data;
      return data;
    },

    async createWorld(payload: { name: string; description?: string; visibility?: string; avatar?: string }) {
      const resp = await api.post('/api/v1/worlds', payload);
      const worldId = resp.data.world?.id;
      if (worldId) {
        await this.initWorlds();
        this.worldMap[worldId] = resp.data.world;
        if (!this.joinedWorldIds.includes(worldId)) {
          this.joinedWorldIds.push(worldId);
        }
        this.setCurrentWorld(worldId);
        await this.channelList(worldId, true);
      }
      return resp.data;
    },

    persistFavoriteWorlds() {
      if (typeof window === 'undefined') return;
      localStorage.setItem('favoriteWorldIds', JSON.stringify(this.favoriteWorldIds));
    },

    async fetchFavoriteWorlds() {
      const resp = await api.get('/api/v1/worlds/favorites');
      const ids: string[] = resp.data?.worldIds || [];
      this.favoriteWorldIds = ids;
      this.persistFavoriteWorlds();
      return ids;
    },

    async toggleWorldFavorite(worldId: string) {
      if (!worldId) return;
      const willFavorite = !this.favoriteWorldIds.includes(worldId);
      const resp = await api.post('/api/v1/worlds/' + worldId + '/favorite', { favorite: willFavorite });
      const ids: string[] = resp.data?.worldIds || [];
      this.favoriteWorldIds = ids;
      this.persistFavoriteWorlds();
      return ids;
    },

    isWorldFavorited(worldId: string) {
      return this.favoriteWorldIds.includes(worldId);
    },

    async joinWorld(worldId: string) {
      await api.post(`/api/v1/worlds/${worldId}/join`, {});
      if (!this.joinedWorldIds.includes(worldId)) {
        this.joinedWorldIds.push(worldId);
      }
      this.setCurrentWorld(worldId);
      await this.channelList(worldId, true);
    },

    async leaveWorld(worldId: string) {
      await api.post(`/api/v1/worlds/${worldId}/leave`, {});
      this.joinedWorldIds = this.joinedWorldIds.filter(id => id !== worldId);
      if (this.currentWorldId === worldId) {
        this.currentWorldId = this.joinedWorldIds[0] || '';
        localStorage.setItem('currentWorldId', this.currentWorldId);
      }
      delete this.channelTreeByWorld[worldId];
    },

    async worldDetail(worldId: string) {
      if (!worldId) return null;
      if (this.worldDetailMap[worldId]) {
        return this.worldDetailMap[worldId];
      }
      const resp = await api.get(`/api/v1/worlds/${worldId}`);
      this.worldDetailMap[worldId] = resp.data;
      if (resp.data.world) {
        this.worldMap[worldId] = resp.data.world;
      }
      return resp.data;
    },

    async worldUpdate(worldId: string, payload: { name?: string; description?: string; visibility?: string; avatar?: string; enforceMembership?: boolean }) {
      const resp = await api.patch(`/api/v1/worlds/${worldId}`, payload);
      if (resp.data?.world) {
        this.worldMap[worldId] = resp.data.world;
        this.worldDetailMap[worldId] = resp.data;
      }
      return resp.data;
    },

    async worldDelete(worldId: string) {
      const resp = await api.delete(`/api/v1/worlds/${worldId}`);
      delete this.worldMap[worldId];
      delete this.worldDetailMap[worldId];
      delete this.worldSectionCache[worldId];
      this.joinedWorldIds = this.joinedWorldIds.filter(id => id !== worldId);
      if (this.currentWorldId === worldId) {
        this.currentWorldId = this.joinedWorldIds[0] || '';
      }
      return resp.data;
    },

    async createWorldInvite(worldId: string, payload: { ttlMinutes?: number; maxUse?: number; memo?: string }) {
      if (!worldId) throw new Error('world id required');
      const resp = await api.post(`/api/v1/worlds/${worldId}/invites`, payload);
      return resp.data;
    },

    async worldMemberList(worldId: string, params?: { page?: number; pageSize?: number; keyword?: string }) {
      const resp = await api.get(`/api/v1/worlds/${worldId}/members`, { params });
      return resp.data;
    },

    async worldMemberSetRole(worldId: string, userId: string, role: string) {
      const resp = await api.post(`/api/v1/worlds/${worldId}/members/${userId}/role`, { role });
      return resp.data;
    },

    async worldMemberRemove(worldId: string, userId: string) {
      const resp = await api.delete(`/api/v1/worlds/${worldId}/members/${userId}`);
      return resp.data;
    },

    async consumeWorldInvite(slug: string) {
      const resp = await api.post(`/api/v1/worlds/invites/${slug}/consume`, {});
      const worldId = resp.data?.world?.id;
      if (worldId) {
        this.worldMap[worldId] = resp.data.world;
        if (!this.joinedWorldIds.includes(worldId)) {
          this.joinedWorldIds.push(worldId);
        }
        this.setCurrentWorld(worldId);
      }
      return resp.data;
    },

    async loadWorldSections(worldId: string, sections: string[] = ['channels']) {
      const resp = await api.get(`/api/v1/worlds/${worldId}/sections`, { params: { sections: sections.join(',') } });
      const key = `${worldId}-${sections.sort().join(',')}`;
      this.worldSectionCache[key] = resp.data;
      if (resp.data.channels) {
        this.applyChannelTree(worldId, resp.data.channels);
      }
      return resp.data;
    },

    applyChannelTree(worldId: string, channels: Channel[]) {
      const groupedData = groupBy(channels, 'parentId');
      const buildTree = (parentId: string): any => {
        const children = groupedData[parentId] || [];
        return children.map((child: Channel) => ({
          ...child,
          children: buildTree(child.id),
        }));
      };
      const tree = buildTree('');
      this.channelTreeByWorld = {
        ...this.channelTreeByWorld,
        [worldId]: tree,
      };
      this.channelTreeReady = {
        ...this.channelTreeReady,
        [worldId]: true,
      };
      if (this.currentWorldId === worldId) {
        this.channelTree = tree;
      }
      this.ensureChannelCollapseState(tree as SChannel[]);
      return tree;
    },

    async switchWorld(worldId: string, options?: { force?: boolean }) {
      if (!worldId) {
        return;
      }
      if (!this.joinedWorldIds.includes(worldId)) {
        await this.joinWorld(worldId);
      } else {
        this.setCurrentWorld(worldId);
        await this.channelList(worldId, options?.force ?? true);
      }
      const firstChannel = this.channelTree[0];
      if (firstChannel) {
        await this.channelSwitchTo(firstChannel.id);
      }
    },

    async channelCreate(data: any) {
      const targetWorldId = data.worldId || this.currentWorldId;
      if (!targetWorldId) {
        throw new Error('worldId 缺失，无法创建频道');
      }
      const payload = {
        ...data,
        worldId: targetWorldId,
        world_id: targetWorldId, // 兼容旧字段
      };
      const resp = await this.sendAPI('channel.create', payload) as APIChannelCreateResp;
      if (resp?.err) {
        throw new Error(resp.err || '创建频道失败');
      }
      await this.channelList(targetWorldId, true);
      return resp;
    },

    async channelPrivateCreate(userId: string) {
      const resp = await this.sendAPI('channel.private.create', { 'user_id': userId });
      console.log('channel.private.create', resp);
      return resp.data;
    },

    setChannelUnreadCount(channelId: string, count: number) {
      if (!channelId) {
        return;
      }
      if (this.unreadCountMap[channelId] === count && channelId in this.unreadCountMap) {
        return;
      }
      this.unreadCountMap = {
        ...this.unreadCountMap,
        [channelId]: count,
      };
    },

    async channelSwitchTo(id: string) {
      let nextChannel = this.channelTree.find(c => c.id === id) ||
        this.channelTree.flatMap(c => c.children || []).find(c => c.id === id);

      let isFromArchive = false;

      if (!nextChannel) {
        nextChannel = this.channelTreePrivate.find(c => c.id === id);
      }

      // 如果本地找不到（可能是归档频道），尝试从 API 获取
      if (!nextChannel) {
        try {
          const channelResp = await this.channelInfoGet(id);
          // 确保返回的频道有有效的 id
          if (channelResp?.item && channelResp.item.id) {
            nextChannel = channelResp.item as SChannel;
            // 标记为从归档获取的频道
            if ((nextChannel as any).status === 'archived') {
              isFromArchive = true;
            }
          }
        } catch (error) {
          console.warn('获取频道信息失败', error);
        }
      }

      if (!nextChannel) {
        alert('频道不存在');
        return;
      }

      // 如果切换到的不是归档频道，清除之前的临时归档频道
      if (!isFromArchive) {
        this.temporaryArchivedChannel = null;
      } else {
        // 保存为临时归档频道，以便在侧边栏显示
        this.temporaryArchivedChannel = nextChannel as SChannel;
      }

      this.cancelEditing();

      let oldChannel = this.curChannel;
      this.curChannel = nextChannel;
      const resp = await this.sendAPI('channel.enter', { 'channel_id': id });
      // console.log('switch', resp, this.curChannel);

      if (!resp.data?.member) {
        this.curChannel = oldChannel;
        return false;
      }

      this.curMember = resp.data.member;
      await this.loadChannelIdentities(id);
      // 确保默认场外角色存在
      await this.ensureDefaultOocRole(id);
      localStorage.setItem('lastChannel', id);

      const resp2 = await this.sendAPI('channel.member.list.online', { 'channel_id': id });
      this.curChannelUsers = resp2.data.data;
      this.whisperTarget = null;

      try {
        await this.ensureChannelPermissionCache(id);
      } catch (error) {
        console.warn('ensureChannelPermissionCache failed', error);
      }

      this.setChannelUnreadCount(id, 0);

      chatEvent.emit('channel-switch-to', undefined);
      this.channelList(this.currentWorldId);
      return true;
    },


    getActiveIdentity(channelId?: string) {
      const targetId = channelId || this.curChannel?.id || '';
      if (!targetId) {
        return null;
      }
      const list = this.channelIdentities[targetId] || [];
      const activeId = this.activeChannelIdentity[targetId];
      const found = activeId ? list.find(item => item.id === activeId) : undefined;
      if (found) {
        return found;
      }
      return list.length > 0 ? list[0] : null;
    },

    getActiveIdentityId(channelId?: string) {
      return this.getActiveIdentity(channelId)?.id || '';
    },

    setActiveIdentity(channelId: string, identityId: string) {
      this.activeChannelIdentity = {
        ...this.activeChannelIdentity,
        [channelId]: identityId,
      };
      localStorage.setItem(`channelIdentity:${channelId}`, identityId || '');
    },

    upsertChannelIdentity(identity: ChannelIdentity) {
      const list = [...(this.channelIdentities[identity.channelId] || [])];
      const idx = list.findIndex(item => item.id === identity.id);
      if (idx >= 0) {
        list.splice(idx, 1, identity);
      } else {
        list.push(identity);
      }
      list.sort((a, b) => a.sortOrder - b.sortOrder);
      this.channelIdentities = {
        ...this.channelIdentities,
        [identity.channelId]: list,
      };
      if (identity.isDefault || !this.activeChannelIdentity[identity.channelId]) {
        this.setActiveIdentity(identity.channelId, identity.id);
      }
      if (identity.folderIds) {
        const membership = {
          ...(this.channelIdentityMembership[identity.channelId] || {}),
          [identity.id]: [...identity.folderIds],
        };
        this.channelIdentityMembership = {
          ...this.channelIdentityMembership,
          [identity.channelId]: membership,
        };
      }
      chatEvent.emit('channel-identity-updated', { identity, channelId: identity.channelId });
    },

    removeChannelIdentity(channelId: string, identityId: string) {
      const list = (this.channelIdentities[channelId] || []).filter(item => item.id !== identityId);
      this.channelIdentities = {
        ...this.channelIdentities,
        [channelId]: list,
      };
      if (this.activeChannelIdentity[channelId] === identityId) {
        const fallback = list.find(item => item.isDefault) || list[0];
        this.setActiveIdentity(channelId, fallback?.id || '');
      }
      if (this.channelIdentityMembership[channelId]) {
        const membership = { ...this.channelIdentityMembership[channelId] };
        delete membership[identityId];
        this.channelIdentityMembership = {
          ...this.channelIdentityMembership,
          [channelId]: membership,
        };
      }
    },

    async loadChannelIdentities(channelId: string, force = false) {
      if (!channelId) {
        return [];
      }
      if (!force && this.channelIdentities[channelId]) {
        const cached = localStorage.getItem(`channelIdentity:${channelId}`) || '';
        if (cached) {
          this.activeChannelIdentity = {
            ...this.activeChannelIdentity,
            [channelId]: cached,
          };
        }
        return this.channelIdentities[channelId];
      }
      const resp = await api.get<{ items: ChannelIdentity[]; folders: ChannelIdentityFolder[]; favorites: string[]; membership: Record<string, string[]> }>('api/v1/channel-identities', { params: { channelId } });
      const membership = resp.data.membership || {};
      const items = (resp.data.items || []).slice().sort((a, b) => a.sortOrder - b.sortOrder);
      items.forEach(item => {
        item.folderIds = membership[item.id] ? [...membership[item.id]] : [];
      });
      this.channelIdentities = {
        ...this.channelIdentities,
        [channelId]: items,
      };
      this.channelIdentityFolders = {
        ...this.channelIdentityFolders,
        [channelId]: resp.data.folders || [],
      };
      this.channelIdentityFavorites = {
        ...this.channelIdentityFavorites,
        [channelId]: resp.data.favorites || [],
      };
      this.channelIdentityMembership = {
        ...this.channelIdentityMembership,
        [channelId]: membership,
      };
      const savedActive = localStorage.getItem(`channelIdentity:${channelId}`) || '';
      const defaultItem = items.find(item => item.isDefault) || items[0];
      const activeId = savedActive && items.some(item => item.id === savedActive) ? savedActive : (defaultItem?.id || '');
      this.activeChannelIdentity = {
        ...this.activeChannelIdentity,
        [channelId]: activeId,
      };
      return items;
    },

    async channelIdentityCreate(payload: { channelId: string; displayName: string; color: string; avatarAttachmentId: string; isDefault: boolean; folderIds?: string[]; }) {
      const resp = await api.post<{ item: ChannelIdentity }>('api/v1/channel-identities', payload);
      const identity = resp.data.item;
      this.upsertChannelIdentity(identity);
      this.setActiveIdentity(payload.channelId, identity.id);
      return identity;
    },

    async channelIdentityUpdate(identityId: string, payload: { channelId: string; displayName: string; color: string; avatarAttachmentId: string; isDefault: boolean; folderIds?: string[]; }) {
      const resp = await api.put<{ item: ChannelIdentity }>(`api/v1/channel-identities/${identityId}`, payload);
      const identity = resp.data.item;
      this.upsertChannelIdentity(identity);
      return identity;
    },

    async channelIdentityDelete(channelId: string, identityId: string) {
      await api.delete('api/v1/channel-identities/' + identityId, { params: { channelId } });
      this.removeChannelIdentity(channelId, identityId);
      chatEvent.emit('channel-identity-updated', { channelId, removedId: identityId });
    },

    async createChannelIdentityFolder(channelId: string, name: string, sortOrder?: number) {
      const resp = await api.post<{ item: ChannelIdentityFolder }>('api/v1/channel-identity-folders', {
        channelId,
        name,
        sortOrder,
      });
      const list = [...(this.channelIdentityFolders[channelId] || []), resp.data.item].sort((a, b) => a.sortOrder - b.sortOrder);
      this.channelIdentityFolders = {
        ...this.channelIdentityFolders,
        [channelId]: list,
      };
      return resp.data.item;
    },

    async updateChannelIdentityFolder(folderId: string, channelId: string, payload: { name?: string; sortOrder?: number }) {
      const resp = await api.put<{ item: ChannelIdentityFolder }>(`api/v1/channel-identity-folders/${folderId}`, {
        channelId,
        name: payload.name,
        sortOrder: payload.sortOrder,
      });
      const list = (this.channelIdentityFolders[channelId] || []).map(folder => (folder.id === folderId ? resp.data.item : folder)).sort((a, b) => a.sortOrder - b.sortOrder);
      this.channelIdentityFolders = {
        ...this.channelIdentityFolders,
        [channelId]: list,
      };
      return resp.data.item;
    },

    async deleteChannelIdentityFolder(folderId: string, channelId: string) {
      await api.delete(`api/v1/channel-identity-folders/${folderId}`, { params: { channelId } });
      const list = (this.channelIdentityFolders[channelId] || []).filter(folder => folder.id !== folderId);
      this.channelIdentityFolders = {
        ...this.channelIdentityFolders,
        [channelId]: list,
      };
      const favorites = (this.channelIdentityFavorites[channelId] || []).filter(id => id !== folderId);
      this.channelIdentityFavorites = {
        ...this.channelIdentityFavorites,
        [channelId]: favorites,
      };
      this.removeFolderFromIdentityMembership(channelId, folderId);
    },

    async toggleChannelIdentityFolderFavorite(folderId: string, channelId: string, favorite: boolean) {
      const resp = await api.post<{ favorites: string[] }>(`api/v1/channel-identity-folders/${folderId}/favorite`, {
        channelId,
        favorite,
      });
      this.channelIdentityFavorites = {
        ...this.channelIdentityFavorites,
        [channelId]: resp.data.favorites || [],
      };
    },

    async assignIdentitiesToFolders(channelId: string, identityIds: string[], folderIds: string[], mode: 'replace' | 'append' | 'remove') {
      const resp = await api.post<{ membership: Record<string, string[]> }>('api/v1/channel-identity-folders/assign', {
        channelId,
        identityIds,
        folderIds,
        mode,
      });
      this.applyIdentityMembershipUpdate(channelId, resp.data.membership || {});
    },

    applyIdentityMembershipUpdate(channelId: string, updates: Record<string, string[]>) {
      if (!updates || Object.keys(updates).length === 0) {
        return;
      }
      const currentMembership = { ...(this.channelIdentityMembership[channelId] || {}) };
      const list = (this.channelIdentities[channelId] || []).map(identity => {
        if (updates[identity.id]) {
          const folders = updates[identity.id] || [];
          currentMembership[identity.id] = folders;
          return { ...identity, folderIds: folders } as ChannelIdentity;
        }
        return identity;
      });
      Object.entries(updates).forEach(([id, folders]) => {
        if (!currentMembership[id]) {
          currentMembership[id] = folders || [];
        }
      });
      if (list.length) {
        this.channelIdentities = {
          ...this.channelIdentities,
          [channelId]: list,
        };
      }
      this.channelIdentityMembership = {
        ...this.channelIdentityMembership,
        [channelId]: currentMembership,
      };
    },

    removeFolderFromIdentityMembership(channelId: string, folderId: string) {
      const currentMembership = { ...(this.channelIdentityMembership[channelId] || {}) };
      let changed = false;
      const list = (this.channelIdentities[channelId] || []).map(identity => {
        if (identity.folderIds && identity.folderIds.includes(folderId)) {
          const folders = identity.folderIds.filter(id => id !== folderId);
          currentMembership[identity.id] = folders;
          changed = true;
          return { ...identity, folderIds: folders } as ChannelIdentity;
        }
        return identity;
      });
      Object.keys(currentMembership).forEach(key => {
        const folders = currentMembership[key] || [];
        const filtered = folders.filter(id => id !== folderId);
        if (filtered.length !== folders.length) {
          currentMembership[key] = filtered;
          changed = true;
        }
      });
      if (changed) {
        this.channelIdentities = {
          ...this.channelIdentities,
          [channelId]: list,
        };
        this.channelIdentityMembership = {
          ...this.channelIdentityMembership,
          [channelId]: currentMembership,
        };
      }
    },

    findChannelById(channelId: string): SChannel | null {
      const traverse = (nodes: SChannel[] = []): SChannel | null => {
        for (const node of nodes) {
          if (node.id === channelId) {
            return node;
          }
          const found = traverse(((node as any).children || []) as SChannel[]);
          if (found) {
            return found;
          }
        }
        return null;
      };
      return traverse(this.channelTree) || this.channelTreePrivate.find(item => item.id === channelId) || null;
    },

    getChannelOwnerId(channelId?: string) {
      if (!channelId) {
        return '';
      }
      if (this.curChannel?.id === channelId) {
        return (this.curChannel as any)?.userId || '';
      }
      const target = this.findChannelById(channelId) as any;
      return target?.userId || '';
    },

    isChannelOwner(channelId?: string, userId?: string) {
      if (!channelId || !userId) {
        return false;
      }
      return this.getChannelOwnerId(channelId) === userId;
    },

    async ensureRolePermissions(roleId: string): Promise<string[]> {
      if (!roleId) {
        return [];
      }
      if (!this.channelRoleCache[roleId]) {
        try {
          const resp = await api.get<{ data: string[] }>('api/v1/channel-role-perms', { params: { roleId } });
          this.channelRoleCache = {
            ...this.channelRoleCache,
            [roleId]: resp.data.data || [],
          };
        } catch (error) {
          this.channelRoleCache = {
            ...this.channelRoleCache,
            [roleId]: [],
          };
        }
      }
      return this.channelRoleCache[roleId] || [];
    },

    async loadChannelMemberRoles(channelId: string, force = false) {
      if (!channelId) {
        return {} as Record<string, string[]>;
      }
      if (!force && this.channelMemberRoleMap[channelId]) {
        return this.channelMemberRoleMap[channelId];
      }
      const pageSize = 200;
      let page = 1;
      const aggregated: Record<string, string[]> = {};
      while (true) {
        const resp = await api.get<PaginationListResponse<UserRoleModel>>('api/v1/channel-member-list', {
          params: { id: channelId, page, pageSize },
        });
        const items = resp.data?.items || [];
        for (const item of items) {
          if (item.roleType !== 'channel') {
            continue;
          }
          if (!aggregated[item.userId]) {
            aggregated[item.userId] = [];
          }
          aggregated[item.userId].push(item.roleId);
        }
        const total = resp.data?.total ?? items.length;
        if (!total || page * pageSize >= total || items.length === 0) {
          break;
        }
        page += 1;
      }
      this.channelMemberRoleMap = {
        ...this.channelMemberRoleMap,
        [channelId]: aggregated,
      };
      this.channelMemberPermMap = {
        ...this.channelMemberPermMap,
        [channelId]: {},
      };
      return aggregated;
    },

    async updateChannelAdminMap(channelId: string, force = false) {
      if (!channelId) {
        return {} as Record<string, boolean>;
      }
      if (!force && this.channelAdminMap[channelId]) {
        return this.channelAdminMap[channelId];
      }
      const roleMap = await this.loadChannelMemberRoles(channelId, force);
      const uniqueRoleIds = new Set<string>();
      Object.values(roleMap).forEach((roleIds) => {
        roleIds.forEach((id) => {
          if (id) {
            uniqueRoleIds.add(id);
          }
        });
      });
      const rolePermMap: Record<string, string[]> = {};
      await Promise.all(Array.from(uniqueRoleIds).map(async (roleId) => {
        rolePermMap[roleId] = await this.ensureRolePermissions(roleId);
      }));
      const adminPerms = new Set([
        'func_channel_message_archive',
        'func_channel_message_delete',
        'func_channel_manage_info',
        'func_channel_manage_role',
        'func_channel_manage_role_root',
        'func_channel_role_link_root',
        'func_channel_role_unlink_root',
      ]);
      const adminMap: Record<string, boolean> = {};
      const ownerId = this.getChannelOwnerId(channelId);
      if (ownerId) {
        adminMap[ownerId] = true;
      }
      for (const [userId, roleIds] of Object.entries(roleMap)) {
        if (!userId) {
          continue;
        }
        const perms = new Set<string>();
        for (const roleId of roleIds) {
          (rolePermMap[roleId] || []).forEach((perm) => perms.add(perm));
        }
        const hasAdminPerm = Array.from(adminPerms).some((perm) => perms.has(perm));
        if (hasAdminPerm) {
          adminMap[userId] = true;
        }
      }
      this.channelAdminMap = {
        ...this.channelAdminMap,
        [channelId]: adminMap,
      };
      return adminMap;
    },

    async hasChannelPermission(channelId: string, permKey: string, userId?: string) {
      if (!channelId || !permKey) {
        return false;
      }
      const targetUser = userId || useUserStore().info.id;
      if (!targetUser) {
        return false;
      }
      await this.ensureChannelPermissionCache(channelId);
      if (!this.channelMemberPermMap[channelId]) {
        this.channelMemberPermMap[channelId] = {};
      }
      if (!this.channelMemberPermMap[channelId][targetUser]) {
        const roleIds = this.channelMemberRoleMap[channelId]?.[targetUser] || [];
        const permSet = new Set<string>();
        await Promise.all(roleIds.map(async (roleId) => {
          const perms = await this.ensureRolePermissions(roleId);
          perms.forEach((perm) => permSet.add(perm));
        }));
        this.channelMemberPermMap[channelId][targetUser] = Array.from(permSet);
      }
      return this.channelMemberPermMap[channelId][targetUser]?.includes(permKey) ?? false;
    },

    async ensureChannelPermissionCache(channelId: string) {
      if (!channelId) {
        return;
      }
      await this.loadChannelMemberRoles(channelId);
      await this.updateChannelAdminMap(channelId);
    },

    isChannelAdmin(channelId?: string, userId?: string) {
      if (!channelId || !userId) {
        return false;
      }
      return !!this.channelAdminMap[channelId]?.[userId];
    },

    toggleChannelCollapse(channelId: string) {
      const next = !this.channelCollapseState[channelId];
      this.setChannelCollapse(channelId, next);
    },

    setChannelCollapse(channelId: string, collapsed: boolean) {
      if (!channelId) return;
      if (this.channelCollapseState[channelId] === collapsed) {
        return;
      }
      this.channelCollapseState = {
        ...this.channelCollapseState,
        [channelId]: collapsed,
      };
    },

    collapseAllChannelGroups(collapsed = true) {
      const next = { ...this.channelCollapseState };
      this.channelTree.forEach((channel) => {
        if (channel.children?.length) {
          next[channel.id] = collapsed;
        }
      });
      this.channelCollapseState = next;
    },

    ensureChannelCollapseState(tree?: SChannel[]) {
      const next = { ...this.channelCollapseState };
      const traverse = (items?: SChannel[]) => {
        if (!items) return;
        items.forEach((item) => {
          if (item.children?.length) {
            if (next[item.id] === undefined) {
              next[item.id] = false;
            }
            traverse(item.children as SChannel[]);
          }
        });
      };
      traverse(tree || this.channelTree);
      this.channelCollapseState = next;
    },

    async channelList(worldId?: string, force = false) {
      const targetWorld = worldId || this.currentWorldId;
      if (!targetWorld) {
        await this.initWorlds();
      }
      const finalWorld = targetWorld || this.currentWorldId;
      if (!finalWorld) {
        return [];
      }
      await this.ensureConnectionReady();
      if (!force && this.channelTreeByWorld[finalWorld]) {
        this.channelTree = this.channelTreeByWorld[finalWorld];
        if (!this.channelTreeReady[finalWorld]) {
          this.channelTreeReady = {
            ...this.channelTreeReady,
            [finalWorld]: true,
          };
        }
        return this.channelTree;
      }
      const resp = await this.sendAPI('channel.list', { world_id: finalWorld, worldId: finalWorld }) as APIChannelListResp;
      const d = resp.data;
      const chns = d.data ?? [];

      const curItem = chns.find(c => c.id === this.curChannel?.id);
      this.curChannel = curItem || this.curChannel;

      const tree = this.applyChannelTree(finalWorld, chns);

      if (!this.curChannel) {
        // 这是为了正确标记人数，有点屎但实现了
        const lastChannel = this._lastChannel;
        const c = this.channelTree.find(c => c.id === lastChannel);
        if (c) {
          this.channelSwitchTo(c.id);
        } else {
          if (tree[0]) this.channelSwitchTo(tree[0].id);
        }
      }

      const countMap = await this.channelUnreadCount();
      this.unreadCountMap = countMap;
      // console.log('countMap', countMap);

      return tree;
    },

    patchChannelAttributes(channelId: string, attrs: Partial<SChannel>) {
      if (!channelId) {
        return;
      }
      const normalizedPatch: Partial<SChannel> = {};
      Object.entries(attrs).forEach(([key, value]) => {
        if (value !== undefined) {
          (normalizedPatch as any)[key] = value;
        }
      });
      if (Object.keys(normalizedPatch).length === 0) {
        return;
      }
      const apply = (items?: SChannel[]) => {
        if (!Array.isArray(items)) {
          return;
        }
        items.forEach((item) => {
          if (!item) return;
          if (item.id === channelId) {
            Object.assign(item, normalizedPatch);
          }
          if (item.children) {
            apply(item.children as SChannel[]);
          }
        });
      };
      apply(this.channelTree as any);
      apply(this.channelTreePrivate as any);
      if (this.curChannel?.id === channelId) {
        this.curChannel = {
          ...this.curChannel,
          ...normalizedPatch,
        } as Channel;
      }
    },

    patchChannelDefaultDice(channelId: string, expr: string) {
      if (!channelId || !expr) {
        return;
      }
      this.patchChannelAttributes(channelId, { defaultDiceExpr: expr });
    },

    async updateChannelDefaultDice(expr: string) {
      if (!this.curChannel?.id) {
        return;
      }
      const resp = await this.sendAPI('channel.dice.default.set', {
        channel_id: this.curChannel.id,
        default_dice_expr: expr,
      }) as { data?: { channel_id?: string; default_dice_expr?: string } };
      const payload = resp?.data;
      const channelId = payload?.channel_id || this.curChannel.id;
      const nextExpr = payload?.default_dice_expr || expr;
      this.patchChannelDefaultDice(channelId, nextExpr);
    },

    async updateChannelFeatures(channelId: string, updates: { builtInDiceEnabled?: boolean; botFeatureEnabled?: boolean }) {
      if (!channelId) {
        return null;
      }
      const body: Record<string, any> = { channel_id: channelId };
      if (typeof updates.builtInDiceEnabled === 'boolean') {
        body.built_in_dice_enabled = updates.builtInDiceEnabled;
      }
      if (typeof updates.botFeatureEnabled === 'boolean') {
        body.bot_feature_enabled = updates.botFeatureEnabled;
      }
      const resp = await this.sendAPI('channel.feature.update', body) as {
        data?: { channel_id?: string; built_in_dice_enabled?: boolean; bot_feature_enabled?: boolean };
      };
      const payload = resp?.data;
      const targetId = payload?.channel_id || channelId;
      const patch: Partial<SChannel> = {};
      if (typeof payload?.built_in_dice_enabled === 'boolean') {
        patch.builtInDiceEnabled = payload.built_in_dice_enabled;
      } else if (typeof updates.builtInDiceEnabled === 'boolean') {
        patch.builtInDiceEnabled = updates.builtInDiceEnabled;
      }
      if (typeof payload?.bot_feature_enabled === 'boolean') {
        patch.botFeatureEnabled = payload.bot_feature_enabled;
      } else if (typeof updates.botFeatureEnabled === 'boolean') {
        patch.botFeatureEnabled = updates.botFeatureEnabled;
      }
      this.patchChannelAttributes(targetId, patch);
      return payload;
    },

    async channelMembersCountRefresh() {
      if (this.channelTree) {
        const m: any = {}
        const lst = this.channelTree.map(i => {
          m[i.id] = i
          return i.id
        })
        const resp = await this.sendAPI('channel.members_count', {
          channel_ids: lst
        });
        for (let [k, v] of Object.entries(resp.data)) {
          m[k].membersCount = v
        }
      }
    },

    async channelRefreshSetup() {
      setInterval(async () => {
        await this.channelMembersCountRefresh();
        if (this.curChannel?.id) {
          const resp2 = await this.sendAPI('channel.member.list.online', { 'channel_id': this.curChannel?.id });
          this.curChannelUsers = resp2.data.data;
        }
      }, 10000);

      setInterval(async () => {
        await this.channelList();
      }, 20000);
    },

    async messageList(channelId: string, next?: string, options?: {
      includeArchived?: boolean;
      includeOoc?: boolean;
      archivedOnly?: boolean;
      icOnly?: boolean;
      userIds?: string[];
      limit?: number;
    }) {
      const payload: Record<string, any> = {
        channel_id: channelId,
      };
      if (next) {
        payload.next = next;
      }
      if (options) {
        if (typeof options.includeArchived === 'boolean') {
          payload.include_archived = options.includeArchived;
        }
        if (typeof options.includeOoc === 'boolean') {
          payload.include_ooc = options.includeOoc;
        }
        if (typeof options.archivedOnly === 'boolean') {
          payload.archived_only = options.archivedOnly;
        }
        if (typeof options.icOnly === 'boolean') {
          payload.ic_only = options.icOnly;
        }
        if (options.userIds && options.userIds.length > 0) {
          payload.user_ids = options.userIds;
        }
        if (typeof options.limit === 'number') {
          const normalizedLimit = Number(options.limit);
          if (Number.isFinite(normalizedLimit) && normalizedLimit > 0) {
            payload.limit = normalizedLimit;
          }
        }
      }
      const resp = await this.sendAPI('message.list', payload as APIMessage);
      this.canReorderAllMessages = !!resp.data?.can_reorder_all;
      return resp.data;
    },

    async messageListDuring(channelId: string, fromTime: any, toTime: any, options?: {
      includeArchived?: boolean;
      includeOoc?: boolean;
      icOnly?: boolean;
      userIds?: string[];
    }) {
      const payload: Record<string, any> = {
        channel_id: channelId,
        type: 'time',
        from_time: fromTime,
        to_time: toTime,
      };
      if (options) {
        if (typeof options.includeArchived === 'boolean') {
          payload.include_archived = options.includeArchived;
        }
        if (typeof options.includeOoc === 'boolean') {
          payload.include_ooc = options.includeOoc;
        }
        if (typeof options.icOnly === 'boolean') {
          payload.ic_only = options.icOnly;
        }
        if (options.userIds && options.userIds.length > 0) {
          payload.user_ids = options.userIds;
        }
      }
      const resp = await this.sendAPI('message.list', payload);
      this.canReorderAllMessages = !!resp.data?.can_reorder_all;
      return resp.data;
    },

    async guildMemberListRaw(guildId: string, next?: string) {
      const resp = await this.sendAPI('guild.member.list', { guild_id: guildId, next });
      // console.log(resp)
      return resp.data;
    },

    async guildMemberList(guildId: string, next?: string) {
      return memoizeWithTimeout(this.guildMemberListRaw, 30000)(guildId, next)
    },

    async messageDelete(channel_id: string, message_id: string) {
      const resp = await this.sendAPI('message.delete', { channel_id, message_id });
      return resp.data;
    },

    async messageRemove(channel_id: string, message_id: string) {
      const resp = await this.sendAPI('message.remove', { channel_id, message_id });
      return resp.data;
    },

    async messageUpdate(channel_id: string, message_id: string, content: string, options?: { icMode?: 'ic' | 'ooc'; identityId?: string | null }) {
      const payload: Record<string, any> = { channel_id, message_id, content };
      if (options?.icMode) {
        payload.ic_mode = options.icMode;
      }
      if (options && 'identityId' in options) {
        payload.identity_id = options.identityId ?? '';
      }
      const resp = await this.sendAPI<{ data: { message: SatoriMessage }, err?: string }>('message.update', payload);
      if ((resp as any)?.err) {
        throw new Error((resp as any).err);
      }
      return (resp as any).data?.message;
    },

    async messageReorder(channel_id: string, payload: { messageId: string; beforeId?: string; afterId?: string; clientOpId?: string }) {
      const resp = await this.sendAPI('message.reorder', {
        channel_id,
        message_id: payload.messageId,
        before_id: payload.beforeId || '',
        after_id: payload.afterId || '',
        client_op_id: payload.clientOpId || '',
      });
      return resp.data;
    },

    async messageCreate(
      content: string,
      quote_id?: string,
      whisper_to?: string,
      clientId?: string,
      identityId?: string,
      displayOrder?: number,
    ) {
      const payload: Record<string, any> = {
        channel_id: this.curChannel?.id,
        content,
        ic_mode: this.icMode,
      };
      if (quote_id) {
        payload.quote_id = quote_id;
      }
      const whisperId = whisper_to ?? this.whisperTarget?.id;
      if (whisperId) {
        payload.whisper_to = whisperId;
      }
      if (clientId) {
        payload.client_id = clientId;
      }
      const resolvedIdentityId = identityId || this.getActiveIdentityId(this.curChannel?.id);
      if (resolvedIdentityId) {
        payload.identity_id = resolvedIdentityId;
      }
      if (typeof displayOrder === 'number' && displayOrder > 0) {
        payload.display_order = displayOrder;
      }
      const resp = await this.sendAPI('message.create', payload);
      const message = resp?.data;
      if (!message || typeof message !== 'object') {
        return null;
      }
      return message;
    },

    async messageTyping(
      state: 'indicator' | 'content' | 'silent',
      content: string,
      channelId?: string,
      extra?: { mode?: string; messageId?: string; whisperTo?: string; icMode?: 'ic' | 'ooc'; orderKey?: number },
    ) {
      const targetChannelId = channelId || this.curChannel?.id;
      if (!targetChannelId) {
        return;
      }
      try {
        const payload: Record<string, any> = {
          channel_id: targetChannelId,
          state,
          enabled: state === 'content',
          content,
        };
        if (extra?.mode) {
          payload.mode = extra.mode;
        }
        if (extra?.messageId) {
          payload.message_id = extra.messageId;
        }
        if (extra?.icMode) {
          payload.ic_mode = extra.icMode;
        }
        let whisperTargetId: string | null | undefined = extra?.whisperTo;
        if (!whisperTargetId && this.whisperTarget?.id) {
          whisperTargetId = this.whisperTarget.id;
        }
        if (!whisperTargetId && extra?.messageId && this.editing?.messageId === extra.messageId && this.editing?.whisperTargetId) {
          whisperTargetId = this.editing.whisperTargetId;
        }
        if (!whisperTargetId && extra?.mode === 'editing' && this.editing?.whisperTargetId) {
          whisperTargetId = this.editing.whisperTargetId;
        }
        if (whisperTargetId) {
          payload.whisper_to = whisperTargetId;
        }
        const activeIdentity = this.getActiveIdentity(targetChannelId);
        if (activeIdentity) {
          payload.identity_id = activeIdentity.id;
        }
        if (typeof extra?.orderKey === 'number' && Number.isFinite(extra.orderKey) && extra.orderKey > 0) {
          payload.order_key = extra.orderKey;
        }
        const debugEnabled =
          typeof window !== 'undefined' &&
          (window as any).__SC_DEBUG_TYPING__ === true;
        if (debugEnabled) {
          console.debug(
            '[chat:messageTyping]',
            'state=', payload.state,
            'mode=', payload.mode,
            'channel=', payload.channel_id,
            'messageId=', payload.message_id,
            'identityId=', payload.identity_id || '(none)',
            'contentSample=',
            typeof payload.content === 'string' ? payload.content.slice(0, 20) : payload.content,
          );
        }
        await this.sendAPI('message.typing', payload as APIMessage);
      } catch (error) {
        console.warn('message.typing 调用失败', error);
      }
    },

    setWhisperTarget(target?: User | null) {
      this.whisperTarget = target ?? null;
    },

    clearWhisperTarget() {
      this.whisperTarget = null;
    },

    startEditingMessage(payload: { messageId: string; channelId: string; originalContent: string; draft: string; mode?: 'plain' | 'rich'; isWhisper?: boolean; whisperTargetId?: string | null; icMode?: 'ic' | 'ooc'; identityId?: string | null }) {
      const normalizedIdentityId = typeof payload.identityId === 'undefined' ? null : (payload.identityId || null);
      const previousActiveIdentity = payload.channelId ? this.getActiveIdentityId(payload.channelId) : '';
      this.editing = {
        ...payload,
        identityId: normalizedIdentityId,
        initialIdentityId: normalizedIdentityId,
        activeIdentityBackup: previousActiveIdentity || null,
      };
      if (payload.channelId && normalizedIdentityId) {
        this.setActiveIdentity(payload.channelId, normalizedIdentityId);
      }
    },

    updateEditingDraft(draft: string) {
      if (this.editing) {
        this.editing.draft = draft;
      }
    },

    updateEditingIcMode(mode: 'ic' | 'ooc') {
      if (this.editing) {
        this.editing.icMode = mode;
      }
    },

    updateEditingIdentity(identityId?: string | null) {
      if (this.editing) {
        this.editing.identityId = identityId || null;
      }
    },

    restoreEditingIdentity() {
      if (!this.editing?.channelId) {
        return;
      }
      const fallback = this.editing.activeIdentityBackup ?? '';
      this.setActiveIdentity(this.editing.channelId, fallback);
    },

    cancelEditing() {
      if (this.editing) {
        this.restoreEditingIdentity();
      }
      this.editing = null;
    },

    isEditingMessage(messageId?: string | null) {
      return !!(this.editing && messageId && this.editing.messageId === messageId);
    },

    // friend

    async ChannelPrivateList() {
      try {
        const resp = await this.sendAPI<{ data: { data: SChannel[] } }>('channel.private.list', {});
        this.channelTreePrivate = resp?.data.data || [];
        return this.channelTreePrivate;
      } finally {
        this.channelTreePrivateReady = true;
      }
    },

    // 好友相关的API
    // 获取试图加我好友的人
    async friendRequestList() {
      const resp = await this.sendAPI<{ data: { data: FriendRequestModel[] } }>('friend.request.list', {});
      return resp?.data.data;
    },

    // 删除好友
    async friendDelete(userId: string) {
      const resp = await this.sendAPI<{ data: any }>('friend.delete', { 'user_id': userId });
      return resp?.data;
    },

    // 获取我正在试图加好友的人
    async friendRequestingList() {
      const resp = await this.sendAPI<{ data: { data: FriendRequestModel[] } }>('friend.request.sender.list', {});
      return resp?.data.data;
    },

    // 通过好友审批
    async friendRequestApprove(requestId: string, accept = true) {
      const resp = await this.sendAPI<{ data: boolean }>('friend.approve', {
        "message_id": requestId,
        "approve": accept,
        // "comment"
      });
      return resp?.data;
    },

    // 获取未读信息
    async channelUnreadCount() {
      const resp = await this.sendAPI<{ data: { [key: string]: number } }>('unread.count', {});
      return resp?.data;
    },

    async friendRequestCreate(senderId: string, receiverId: string, note: string = '') {
      const resp = await this.sendAPI<{ data: { status: number } }>('friend.request.create', {
        senderId,
        receiverId,
        note,
      });
      return resp?.data;
    },

    // 频道管理
    async channelRoleList(id: string) {
      const resp = await api.get<PaginationListResponse<ChannelRoleModel>>('api/v1/channel-role-list', { params: { id } });
      return resp;
    },

    // 频道管理
    async channelMemberList(id: string, params?: { page?: number; pageSize?: number }) {
      const resp = await api.get<PaginationListResponse<UserRoleModel>>('api/v1/channel-member-list', {
        params: {
          id,
          page: params?.page,
          pageSize: params?.pageSize,
        },
      });
      return resp;
    },

    async channelMemberOptions(channelId: string) {
      if (!channelId) {
        return { items: [], total: 0 };
      }
      const resp = await api.get<{ items: Array<{ id: string; label: string }>; total: number }>(
        `api/v1/channels/${channelId}/member-options`,
      );
      return resp.data;
    },

    async channelSpeakerOptions(channelId: string) {
      if (!channelId) {
        return { items: [], total: 0 };
      }
      const resp = await api.get<{ items: Array<{ id: string; label: string }>; total: number }>(
        `api/v1/channels/${channelId}/speaker-options`,
      );
      return resp.data;
    },

    async channelSpeakerRoleOptions(channelId: string) {
      if (!channelId) {
        return { items: [], total: 0 };
      }
      const resp = await api.get<{ items: Array<{ id: string; label: string }>; total: number }>(
        `api/v1/channels/${channelId}/speaker-role-options`,
      );
      return resp.data;
    },

    // 添加用户角色
    async userRoleLink(roleId: string, userIds: string[]) {
      const resp = await api.post<{ data: boolean }>('api/v1/user-role-link', { roleId, userIds });
      return resp?.data;
    },

    // 移除用户角色
    async userRoleUnlink(roleId: string, userIds: string[]) {
      const resp = await api.post<{ data: boolean }>('api/v1/user-role-unlink', { roleId, userIds });
      return resp?.data;
    },

    async friendList() {
      const resp = await api.get<PaginationListResponse<FriendInfo>>('api/v1/friend-list', {});
      return resp?.data;
    },

    async botList(forceReload = false) {
      if (!forceReload && this.botListCache) {
        return this.botListCache;
      }
      const resp = await api.get<PaginationListResponse<UserInfo>>('api/v1/bot-list', {});
      if (resp?.data) {
        this.botListCache = resp.data;
        this.botListCacheUpdatedAt = Date.now();
      }
      return resp?.data;
    },

    invalidateBotListCache() {
      this.botListCache = null;
      this.botListCacheUpdatedAt = 0;
    },

    async channelInfoGet(id: string) {
      const resp = await api.get<{ item: SChannel }>(`api/v1/channel-info`, { params: { id } });
      return resp?.data;
    },

    // 编辑频道信息
    async channelInfoEdit(id: string, updates: {
      name?: string;
      note?: string;
      permType?: string;
      sortOrder?: number;
    }) {
      const resp = await api.post<{ message: string }>(`api/v1/channel-info-edit`, updates, { params: { id } });
      return resp?.data;
    },

    async channelDissolve(channelId: string) {
      if (!channelId) {
        throw new Error('缺少频道ID');
      }
      await api.delete(`api/v1/channels/${channelId}`);
      const wasCurrent = this.curChannel?.id === channelId;
      if (wasCurrent) {
        this.curChannel = null;
      }
      await this.channelList(this.currentWorldId, true);
      if (wasCurrent && this.channelTree.length) {
        await this.channelSwitchTo(this.channelTree[0].id);
      }
    },

    // 频道归档
    async archiveChannels(channelIds: string[], includeChildren = true) {
      if (!channelIds.length) {
        throw new Error('频道ID列表不能为空');
      }
      const resp = await api.post('api/v1/channels/archive', {
        channelIds,
        includeChildren,
      });
      // 刷新频道列表
      if (this.currentWorldId) {
        await this.channelList(this.currentWorldId, true);
      }
      return resp.data;
    },

    // 恢复归档频道
    async unarchiveChannels(channelIds: string[], includeChildren = true) {
      if (!channelIds.length) {
        throw new Error('频道ID列表不能为空');
      }
      const resp = await api.post('api/v1/channels/unarchive', {
        channelIds,
        includeChildren,
      });
      // 刷新频道列表
      if (this.currentWorldId) {
        await this.channelList(this.currentWorldId, true);
      }
      return resp.data;
    },

    // 永久删除归档频道
    async deleteArchivedChannels(channelIds: string[], confirmToken: string) {
      if (!channelIds.length) {
        throw new Error('频道ID列表不能为空');
      }
      const resp = await api.delete('api/v1/channels/archived', {
        data: {
          channelIds,
          confirmToken,
        },
      });
      return resp.data;
    },

    // 获取归档频道列表
    async getArchivedChannels(
      worldId: string,
      params?: { keyword?: string; page?: number; pageSize?: number },
    ): Promise<{ items: any[]; total: number; canManage: boolean; canDelete: boolean }> {
      if (!worldId) {
        throw new Error('世界ID不能为空');
      }
      const resp = await api.get(`api/v1/worlds/${worldId}/archived-channels`, {
        params: {
          keyword: params?.keyword,
          page: params?.page,
          pageSize: params?.pageSize,
        },
      });
      return resp.data;
    },

    // 获取频道权限树
    async channelPermTree() {
      const resp = await api.get<{ items: PermTreeNode[] }>('api/v1/channel-perm-tree');
      return resp?.data;
    },

    // 获取系统权限树
    async systemPermTree() {
      const resp = await api.get<{ items: any }>('api/v1/system-perm-tree');
      return resp?.data;
    },

    // 获取频道角色权限
    async channelRolePermsGet(channelId: string, roleId: string) {
      const resp = await api.get<{ data: any }>('api/v1/channel-role-perms', { params: { channelId, roleId } });
      return resp?.data;
    },

    // 更新频道角色权限
    async rolePermsSet(roleId: string, permissions: string[]) {
      const resp = await api.post<{ data: boolean }>('api/v1/role-perms-apply', {
        roleId,
        permissions
      });
      return resp?.data;
    },

    async eventDispatch(e: Event) {
      if (e.type === 'audio-state-updated') {
        const audioPayload = (e as any).audioState as AudioPlaybackStatePayload | undefined;
        if (audioPayload) {
          const audioStudio = useAudioStudioStore();
          await audioStudio.applyRemotePlayback(audioPayload);
        }
      }
      chatEvent.emit(e.type as any, e);
    },

    // 新增方法
    setIcMode(mode: 'ic' | 'ooc') {
      this.icMode = mode;
    },

    startPingLoop() {
      if (typeof window === 'undefined' || pingTimer) {
        return;
      }
      pingTimer = window.setInterval(() => {
        this.sendPresencePing();
      }, 5000);
      this.startLatencyProbeLoop();
    },

    stopPingLoop() {
      if (pingTimer) {
        clearInterval(pingTimer);
        pingTimer = null;
      }
      this.stopLatencyProbeLoop();
    },

    startLatencyProbeLoop() {
      if (typeof window === 'undefined' || latencyTimer) {
        return;
      }
      this.measureLatency();
      latencyTimer = window.setInterval(() => {
        this.measureLatency();
      }, 10000);
    },

    stopLatencyProbeLoop() {
      if (latencyTimer) {
        clearInterval(latencyTimer);
        latencyTimer = null;
      }
      clearPendingLatencyProbes();
    },

    measureLatency() {
      if (!this.subject) {
        return;
      }
      cleanupPendingLatencyProbes();
      const now = Date.now();
      const probeId = nanoid();
      const body = {
        id: probeId,
        clientSentAt: now,
      };
      pendingLatencyProbes[probeId] = now;
      this.subject.next({
        op: 5,
        body,
      });
    },

    handleLatencyResult(payload: any) {
      if (!payload) {
        return;
      }
      const probeId = typeof payload?.id === 'string' ? payload.id : undefined;
      const sentAtFromPending = probeId ? pendingLatencyProbes[probeId] : undefined;
      const fallbackSentAt = typeof payload?.clientSentAt === 'number' ? payload.clientSentAt : undefined;
      const sentAt = typeof sentAtFromPending === 'number' ? sentAtFromPending : fallbackSentAt;
      if (typeof sentAt !== 'number' || sentAt <= 0) {
        return;
      }
      if (probeId) {
        delete pendingLatencyProbes[probeId];
      }
      const now = Date.now();
      const rtt = now - sentAt;
      if (rtt <= 0) {
        return;
      }
      this.lastLatencyMs = Math.round(rtt);
      if (this.curChannel?.id) {
        this.updatePresence(useUserStore().info.id, {
          lastPing: Date.now(),
          latencyMs: this.lastLatencyMs,
          isFocused: this.isAppFocused,
        });
      }
    },

    setFocusState(focused: boolean) {
      const normalized = !!focused;
      if (this.isAppFocused === normalized) {
        return;
      }
      this.isAppFocused = normalized;
      this.sendPresencePing(true);
    },

    updatePresence(userId: string, data: { lastPing: number; latencyMs: number; isFocused: boolean }) {
      this.presenceMap = {
        ...this.presenceMap,
        [userId]: data,
      };
    },

    clearPresenceMap() {
      this.presenceMap = {};
    },

    async sendPresencePing(force = false) {
      if (!this.subject) {
        return;
      }
      const now = Date.now();
      if (!force && this.lastPingSentAt && now - this.lastPingSentAt < 1500) {
        return;
      }
      const user = useUserStore();
      if (!user.token) {
        return;
      }
      this.lastPingSentAt = now;
      this.subject.next({
        op: 1,
        body: {
          token: user.token,
          focused: this.isAppFocused,
          clientSentAt: now,
        },
      });
    },

    handlePong() {
      if (!this.lastPingSentAt) {
        return;
      }
      const latency = Date.now() - this.lastPingSentAt;
      if (latency >= 0) {
        this.lastLatencyMs = latency;
      }
      this.lastPingSentAt = null;
    },

    setFilterState(filters: Partial<{ icOnly: boolean; showArchived: boolean; roleIds: string[] }>) {
      this.filterState = {
        ...this.filterState,
        ...filters,
      };
    },

    async archiveMessages(messageIds: string[]) {
      if (!this.curChannel?.id || messageIds.length === 0) return;
      const resp = await this.sendAPI('message.archive', {
        channel_id: this.curChannel.id,
        message_ids: messageIds,
        reason: '整理消息',
      });
      const payload = resp?.data as { message_ids?: string[] } | undefined;
      if (!payload || !Array.isArray(payload.message_ids) || payload.message_ids.length === 0) {
        throw new Error('归档失败：未找到可归档的消息或无权限操作');
      }
      return payload;
    },

    async unarchiveMessages(messageIds: string[]) {
      if (!this.curChannel?.id || messageIds.length === 0) return;
      const resp = await this.sendAPI('message.unarchive', {
        channel_id: this.curChannel.id,
        message_ids: messageIds,
      });
      const payload = resp?.data as { message_ids?: string[] } | undefined;
      if (!payload || !Array.isArray(payload.message_ids) || payload.message_ids.length === 0) {
        throw new Error('取消归档失败：未找到目标消息或无权限操作');
      }
      return payload;
    },

    async getChannelPresence(channelId?: string) {
      const targetId = channelId || this.curChannel?.id;
      if (!targetId) return;
      const resp = await api.get('api/v1/channel-presence', {
        params: { channel_id: targetId },
      });
      return resp.data;
    },

    async createExportTask(params: {
      channelId: string;
      format: string;
      timeRange?: [number, number];
      includeOoc?: boolean;
      includeArchived?: boolean;
      withoutTimestamp?: boolean;
      mergeMessages?: boolean;
      textColorizeBBCode?: boolean;
      sliceLimit?: number;
      maxConcurrency?: number;
      displaySettings?: DisplaySettings;
      displayName?: string;
    }) {
      const payload: Record<string, any> = {
        channel_id: params.channelId,
        format: params.format,
        include_ooc: params.includeOoc ?? true,
        include_archived: params.includeArchived ?? false,
        without_timestamp: params.withoutTimestamp ?? false,
        merge_messages: params.mergeMessages ?? true,
      };
      if (params.displayName) {
        payload.display_name = params.displayName;
      }
      if (params.timeRange && params.timeRange.length === 2) {
        payload.time_range = params.timeRange;
      }
      if (params.sliceLimit) {
        payload.slice_limit = params.sliceLimit;
      }
      if (params.maxConcurrency) {
        payload.max_concurrency = params.maxConcurrency;
      }
      if (params.displaySettings) {
        payload.display_settings = params.displaySettings;
      }
      if (params.textColorizeBBCode) {
        payload.text_bbcode_colorize = true;
      }
      const resp = await api.post('api/v1/chat/export', payload);
      return resp.data as {
        task_id: string;
        status: string;
        message?: string;
        requested_at?: number;
      };
    },

    async getExportTaskStatus(taskId: string) {
      const resp = await api.get(`api/v1/chat/export/${taskId}`);
      return resp.data as {
        task_id: string;
        status: string;
        file_name?: string;
        message?: string;
        finished_at?: number;
      };
    },

    async listExportTasks(
      channelId: string,
      opts?: { page?: number; size?: number; status?: string; keyword?: string }
    ) {
      if (!channelId) {
        throw new Error('缺少频道 ID');
      }
      const params: Record<string, any> = { channel_id: channelId };
      if (opts?.page) {
        params.page = opts.page;
      }
      if (opts?.size) {
        params.size = opts.size;
      }
      if (opts?.status) {
        params.status = opts.status;
      }
      if (opts?.keyword) {
        params.keyword = opts.keyword;
      }
      const resp = await api.get('api/v1/chat/export', { params });
      return resp.data as ExportTaskListResponse;
    },

    async downloadExportResult(taskId: string, fileNameHint?: string) {
      const resp = await api.get<Blob>(`api/v1/chat/export/${taskId}`, {
        params: { download: 1 },
        responseType: 'blob',
        timeout: 60000,
      });
      const headers = resp.headers ?? {};
      const disposition = (headers['content-disposition'] || headers['Content-Disposition']) as string | undefined;
      let fileName = fileNameHint;
      if (!fileName && disposition) {
        const match = disposition.match(/filename\*?=(?:UTF-8'')?\"?([^\";]+)\"?/i);
        if (match && match[1]) {
          try {
            fileName = decodeURIComponent(match[1]);
          } catch {
            fileName = match[1];
          }
        }
      }
      if (!fileName) {
        fileName = `channel-export-${taskId}`;
      }
      return {
        blob: resp.data,
        fileName,
      };
    },

    async uploadExportTask(taskId: string, payload?: { name?: string }) {
      const resp = await api.post(`api/v1/chat/export/${taskId}/upload`, payload ?? {});
      return resp.data as {
        url: string;
        name?: string;
        file_name?: string;
        uploaded_at?: number;
      };
    },

    async retryExportTask(taskId: string) {
      const resp = await api.post(`api/v1/chat/export/${taskId}/retry`);
      return resp.data as {
        task_id: string;
        status: string;
        message?: string;
        requested_at?: number;
        display_name?: string;
      };
    },

    // IC/OOC 角色配置相关方法
    getChannelIcOocRoleConfig(channelId: string): { icRoleId: string | null; oocRoleId: string | null } {
      if (!channelId) {
        return { icRoleId: null, oocRoleId: null };
      }
      if (this.channelIcOocRoleConfig[channelId]) {
        return this.channelIcOocRoleConfig[channelId];
      }
      // 尝试从 localStorage 加载
      if (typeof window !== 'undefined') {
        try {
          const key = `channelIcOocRole:${channelId}`;
          const stored = localStorage.getItem(key);
          if (stored) {
            const config = JSON.parse(stored);
            this.channelIcOocRoleConfig[channelId] = config;
            return config;
          }
        } catch (err) {
          console.warn('Failed to load IC/OOC role config from localStorage', err);
        }
      }
      return { icRoleId: null, oocRoleId: null };
    },

    setChannelIcOocRoleConfig(
      channelId: string,
      config: { icRoleId?: string | null; oocRoleId?: string | null }
    ) {
      if (!channelId) {
        return;
      }
      const current = this.getChannelIcOocRoleConfig(channelId);
      const updated = {
        icRoleId: config.icRoleId !== undefined ? config.icRoleId : current.icRoleId,
        oocRoleId: config.oocRoleId !== undefined ? config.oocRoleId : current.oocRoleId,
      };
      this.channelIcOocRoleConfig = {
        ...this.channelIcOocRoleConfig,
        [channelId]: updated,
      };
      // 持久化到 localStorage
      if (typeof window !== 'undefined') {
        try {
          const key = `channelIcOocRole:${channelId}`;
          localStorage.setItem(key, JSON.stringify(updated));
        } catch (err) {
          console.warn('Failed to save IC/OOC role config to localStorage', err);
        }
      }
    },

    autoSwitchRoleOnIcOocChange(newMode: 'ic' | 'ooc') {
      const display = useDisplayStore();
      // 检查是否启用自动切换
      if (!display.settings.autoSwitchRoleOnIcOocToggle) {
        return;
      }
      const channelId = this.curChannel?.id;
      if (!channelId) {
        return;
      }
      const config = this.getChannelIcOocRoleConfig(channelId);
      const targetRoleId = newMode === 'ic' ? config.icRoleId : config.oocRoleId;

      // 如果没有配置对应的角色，不进行切换
      if (!targetRoleId) {
        return;
      }

      // 检查角色是否存在
      const identities = this.channelIdentities[channelId] || [];
      const targetRole = identities.find((identity) => identity.id === targetRoleId);
      if (!targetRole) {
        console.warn(`Target role ${targetRoleId} for ${newMode} mode not found`);
        return;
      }

      // 执行角色切换
      this.setActiveIdentity(channelId, targetRoleId);
    },

    async ensureDefaultOocRole(channelId: string) {
      const display = useDisplayStore();
      const user = useUserStore();

      // 检查是否启用自动切换
      if (!display.settings.autoSwitchRoleOnIcOocToggle) {
        return null;
      }

      if (!channelId) {
        return null;
      }

      // 检查是否已经配置了场外角色
      const config = this.getChannelIcOocRoleConfig(channelId);
      if (config.oocRoleId) {
        return config.oocRoleId;
      }

      // 自动创建默认场外角色
      const displayName = user.info.nick || user.info.username || '场外';
      const avatarAttachmentId = user.info.avatar || '';

      try {
        const identity = await this.channelIdentityCreate({
          channelId,
          displayName,
          color: '',
          avatarAttachmentId,
          isDefault: false,
        });

        // 设置为场外默认角色
        this.setChannelIcOocRoleConfig(channelId, { oocRoleId: identity.id });

        console.log(`Created default OOC role for channel ${channelId}`, identity);
        return identity.id;
      } catch (err) {
        console.warn('Failed to create default OOC role', err);
        return null;
      }
    },
  }
});

chatEvent.on('message-created-notice', (data: any) => {
  const chId = data.channelId;
  const chat = useChatStore();
  // console.log('xx', chId, chat.channelTree, chat.channelTreePrivate);

  if (chat.curChannel?.id === chId) {
    return;
  }

  if (chat.channelTree.find(c => c.id === chId) || chat.channelTreePrivate.find(c => c.id === chId)) {
    chat.unreadCountMap[chId] = (chat.unreadCountMap[chId] || 0) + 1;
  }
});

chatEvent.on('channel-updated', (event) => {
  const channelId = event?.channel?.id;
  if (!channelId) {
    return;
  }
  const chat = useChatStore();
  const patch: Partial<SChannel> = {};
  if (event.channel?.defaultDiceExpr) {
    patch.defaultDiceExpr = event.channel.defaultDiceExpr;
  }
  if (typeof event.channel?.builtInDiceEnabled === 'boolean') {
    patch.builtInDiceEnabled = event.channel.builtInDiceEnabled;
  }
  if (typeof event.channel?.botFeatureEnabled === 'boolean') {
    patch.botFeatureEnabled = event.channel.botFeatureEnabled;
  }
  chat.patchChannelAttributes(channelId, patch);
});
