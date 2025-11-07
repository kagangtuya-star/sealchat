import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { WebSocketSubject, webSocket } from 'rxjs/webSocket';
import type { User, Opcode, GatewayPayloadStructure, Channel, EventName, Event, GuildMember } from '@satorijs/protocol'
import type { APIChannelCreateResp, APIChannelListResp, APIMessage, ChannelIdentity, ChannelRoleModel, FriendInfo, FriendRequestModel, PaginationListResponse, SatoriMessage, SChannel, UserInfo, UserRoleModel } from '@/types';
import { nanoid } from 'nanoid'
import { groupBy } from 'lodash-es';
import { Emitter } from '@/utils/event';
import { useUserStore } from './user';
import { api, urlBase } from './_config';
import { useMessage } from 'naive-ui';
import { memoizeWithTimeout } from '@/utils/tools';
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { PermTreeNode } from '@/types-perm';

interface ChatState {
  subject: WebSocketSubject<any> | null;
  // user: User,
  channelTree: SChannel[],
  channelTreePrivate: SChannel[],
  curChannel: Channel | null,
  curMember: GuildMember | null,
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
  } | null

  canReorderAllMessages: boolean;
  channelIdentities: Record<string, ChannelIdentity[]>;
  activeChannelIdentity: Record<string, string>;

  // 新增状态
  icMode: 'ic' | 'ooc';
  presenceMap: Record<string, { lastPing: number; latencyMs: number; isFocused: boolean }>;
  isAppFocused: boolean;
  lastPingSentAt: number | null;
  lastLatencyMs: number;
  filterState: {
    icOnly: boolean;
    showArchived: boolean;
    userIds: string[];
  };
  channelRoleCache: Record<string, string[]>;
  channelMemberRoleMap: Record<string, Record<string, string[]>>;
  channelAdminMap: Record<string, Record<string, boolean>>;
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
  | 'channel-identity-updated';
export const chatEvent = new Emitter<{
  [key in myEventName]: (msg?: Event) => void;
  // 'message-created': (msg: Event) => void;
}>();

let pingTimer: ReturnType<typeof setInterval> | null = null;
let latencyTimer: ReturnType<typeof setInterval> | null = null;
let focusListenersBound = false;

export const useChatStore = defineStore({
  id: 'chat',
  state: (): ChatState => ({
    // user: { id: '1', },
    subject: null,
    channelTree: [] as any,
    channelTreePrivate: [] as any,
    curChannel: null,
    curMember: null,
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

    // 新增状态初始值
    icMode: 'ic',
    presenceMap: {},
    isAppFocused: true,
    lastPingSentAt: null,
    lastLatencyMs: 0,
    filterState: {
      icOnly: false,
      showArchived: false,
      userIds: [],
    },
    channelRoleCache: {},
    channelMemberRoleMap: {},
    channelAdminMap: {},
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
      return Object.entries(state.unreadCountMap).reduce((sum, [key, count]) => {
        return key.includes(':') ? sum : sum + count;
      }, 0);
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

      if (this.curChannel?.id) {
        await this.channelSwitchTo(this.curChannel?.id);
        const resp2 = await this.sendAPI('channel.member.list.online', { 'channel_id': this.curChannel?.id });
        this.curChannelUsers = resp2.data.data;
      }
      await this.channelList();
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

    async setReplayTo(item: any) {
      this.curReplyTo = item;
    },

    async sendAPI<T = any>(api: string, data: APIMessage): Promise<T> {
      const echo = nanoid();
      return new Promise((resolve, reject) => {
        apiMap.set(echo, { resolve, reject });
        this.subject?.next({ api, data, echo });
      })
    },

    async send(channelId: string, content: string) {
      let msg: APIMessage = {
        // api: 'message.create',
        channel_id: channelId,
        content: content
      }
      this.subject?.next(msg);
    },

    async channelCreate(data: any) {
      const resp = await this.sendAPI('channel.create', data) as APIChannelCreateResp;
    },

    async channelPrivateCreate(userId: string) {
      const resp = await this.sendAPI('channel.private.create', { 'user_id': userId });
      console.log('channel.private.create', resp);
      return resp.data;
    },

    async channelSwitchTo(id: string) {
      let nextChannel = this.channelTree.find(c => c.id === id) ||
        this.channelTree.flatMap(c => c.children || []).find(c => c.id === id);

      if (!nextChannel) {
        nextChannel = this.channelTreePrivate.find(c => c.id === id);
      }
      if (!nextChannel) {
        alert('频道不存在');
        return;
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
      localStorage.setItem('lastChannel', id);

      const resp2 = await this.sendAPI('channel.member.list.online', { 'channel_id': id });
      this.curChannelUsers = resp2.data.data;
      this.whisperTarget = null;

      try {
        await this.ensureChannelPermissionCache(id);
      } catch (error) {
        console.warn('ensureChannelPermissionCache failed', error);
      }

      chatEvent.emit('channel-switch-to', undefined);
      this.channelList();
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
      const resp = await api.get<{ items: ChannelIdentity[] }>('api/v1/channel-identities', { params: { channelId } });
      const items = (resp.data.items || []).slice().sort((a, b) => a.sortOrder - b.sortOrder);
      this.channelIdentities = {
        ...this.channelIdentities,
        [channelId]: items,
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

    async channelIdentityCreate(payload: { channelId: string; displayName: string; color: string; avatarAttachmentId: string; isDefault: boolean; }) {
      const resp = await api.post<{ item: ChannelIdentity }>('api/v1/channel-identities', payload);
      const identity = resp.data.item;
      this.upsertChannelIdentity(identity);
      this.setActiveIdentity(payload.channelId, identity.id);
      return identity;
    },

    async channelIdentityUpdate(identityId: string, payload: { channelId: string; displayName: string; color: string; avatarAttachmentId: string; isDefault: boolean; }) {
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

    async channelList() {
      const resp = await this.sendAPI('channel.list', {}) as APIChannelListResp;
      const d = resp.data;
      const chns = d.data ?? [];

      const curItem = chns.find(c => c.id === this.curChannel?.id);
      this.curChannel = curItem || this.curChannel;

      const groupedData = groupBy(chns, 'parentId');
      const buildTree = (parentId: string): any => {
        const children = groupedData[parentId] || [];
        return children.map((child: Channel) => ({
          ...child,
          children: buildTree(child.id),
        }));
      };

      const tree = buildTree('');
      this.channelTree = tree;

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

    async messageUpdate(channel_id: string, message_id: string, content: string) {
      const resp = await this.sendAPI<{ data: { message: SatoriMessage }, err?: string }>('message.update', { channel_id, message_id, content });
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

    async messageCreate(content: string, quote_id?: string, whisper_to?: string, clientId?: string) {
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
      const identityId = this.getActiveIdentityId(this.curChannel?.id);
      if (identityId) {
        payload.identity_id = identityId;
      }
      const resp = await this.sendAPI('message.create', payload);
      const message = resp?.data;
      if (!message || typeof message !== 'object') {
        return null;
      }
      return message;
    },

    async messageTyping(state: 'indicator' | 'content' | 'silent', content: string, channelId?: string, extra?: { mode?: string; messageId?: string }) {
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
        const activeIdentity = this.getActiveIdentity(targetChannelId);
        if (activeIdentity) {
          payload.identity_id = activeIdentity.id;
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

    startEditingMessage(payload: { messageId: string; channelId: string; originalContent: string; draft: string; mode?: 'plain' | 'rich' }) {
      this.editing = { ...payload };
    },

    updateEditingDraft(draft: string) {
      if (this.editing) {
        this.editing.draft = draft;
      }
    },

    cancelEditing() {
      this.editing = null;
    },

    isEditingMessage(messageId?: string | null) {
      return !!(this.editing && messageId && this.editing.messageId === messageId);
    },

    // friend

    async ChannelPrivateList() {
      const resp = await this.sendAPI<{ data: { data: SChannel[] } }>('channel.private.list', {});
      this.channelTreePrivate = resp?.data.data;
      return resp?.data.data;
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

    async botList() {
      const resp = await api.get<PaginationListResponse<UserInfo>>('api/v1/bot-list', {});
      return resp?.data;
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
    },

    measureLatency() {
      if (!this.subject) {
        return;
      }
      const now = Date.now();
      this.subject.next({
        op: 5,
        body: {
          probeSentAt: now,
        },
      });
    },

    handleLatencyResult(payload: any) {
      if (!payload) {
        return;
      }
      const sentAt = typeof payload?.probeSentAt === 'number' ? payload.probeSentAt : undefined;
      if (typeof sentAt !== 'number') {
        return;
      }
      const now = Date.now();
      const rtt = now - sentAt;
      if (rtt <= 0) {
        return;
      }
      const latency = rtt / 2;
      this.lastLatencyMs = Math.round(latency);
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

    setFilterState(filters: Partial<{ icOnly: boolean; showArchived: boolean; userIds: string[] }>) {
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
    }) {
      const payload: Record<string, any> = {
        channel_id: params.channelId,
        format: params.format,
        include_ooc: params.includeOoc ?? true,
        include_archived: params.includeArchived ?? false,
        without_timestamp: params.withoutTimestamp ?? false,
        merge_messages: params.mergeMessages ?? true,
      };
      if (params.timeRange && params.timeRange.length === 2) {
        payload.time_range = params.timeRange;
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
