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

let pingLoopOn = false;

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
          } else if (apiMap.get(msg.echo)) {
            apiMap.get(msg.echo).resolve(msg);
            apiMap.delete(msg.echo);
          }
        },
        error: err => {
          console.log('ws error', err);
          this.subject = null;
          this.connectState = 'disconnected';
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
        complete: () => console.log('complete') // Called when connection is closed (for whatever reason).
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
      if (!pingLoopOn) {
        pingLoopOn = true;
        const user = useUserStore();
        setInterval(async () => {
          if (this.subject) {
            this.subject.next({
              op: 1, body: {
                token: user.token,
              }
            });
          }
        }, 10000)
      }

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

    async messageList(channelId: string, next?: string) {
      const resp = await this.sendAPI('message.list', { channel_id: channelId, next });
      this.canReorderAllMessages = !!resp.data?.can_reorder_all;
      return resp.data;
    },

    async messageListDuring(channelId: string, fromTime: any, toTime: any) {
      const resp = await this.sendAPI('message.list', {
        channel_id: channelId,
        type: 'time',
        from_time: fromTime,
        to_time: toTime,
      });
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
      return resp?.data;
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
    async channelMemberList(id: string) {
      const resp = await api.get<PaginationListResponse<UserRoleModel>>('api/v1/channel-member-list', { params: { id } });
      return resp;
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
    }
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
