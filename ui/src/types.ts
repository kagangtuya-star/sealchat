import type { User, Message, Guild, GuildMember, Opcode, GatewayPayloadStructure, Channel } from '@satorijs/protocol'

export interface WhisperMeta {
  senderMemberId?: string;
  senderMemberName?: string;
  senderUserId?: string;
  senderUserNick?: string;
  senderUserName?: string;
  targetMemberId?: string;
  targetMemberName?: string;
  targetUserId?: string;
  targetUserNick?: string;
  targetUserName?: string;
}

declare module '@satorijs/protocol' {
  interface Message {
    whisperMeta?: WhisperMeta;
  }
}

export interface SatoriMessage {
  id?: string;
  channel?: Channel;
  guild?: Guild;
  user?: User;
  identity?: MessageIdentity;
  member?: GuildMember;
  content?: string;
  elements?: any[]; // Element[] 这个好像会让vscode提示一个错误
  timestamp?: number;
  quote?: SatoriMessage;
  createdAt?: number;
  updatedAt?: number;
  displayOrder?: number;

  sender_member_name?: string;
  isWhisper?: boolean;
  whisperTo?: User | null;
}

export interface LogUploadConfig {
  enabled?: boolean;
  endpoint?: string;
  client?: string;
  uniformId?: string;
  version?: number;
  note?: string;
}

export interface ServerConfig {
  serveAt: string;
  domain: string;
  registerOpen: boolean;
  webUrl: string;
  chatHistoryPersistentDays: number;
  imageSizeLimit: number;
  imageCompress: boolean;
  imageCompressQuality: number;
  builtInSealBotEnable: boolean;
  logUpload?: LogUploadConfig;
  oneBot?: OneBotConfig;
}

export interface OneBotForwardWSConfig {
  host?: string;
  port?: number;
  apiPath?: string;
  eventPath?: string;
  universalPath?: string;
}

export interface OneBotReverseWSConfig {
  enabled?: boolean;
  apiEndpoints?: string[];
  eventEndpoints?: string[];
  universalEndpoints?: string[];
  useUniversalEndpointFirst?: boolean;
  reconnectIntervalSeconds?: number;
}

export interface OneBotAuthConfig {
  accessToken?: string;
}

export interface OneBotConfig {
  enabled?: boolean;
  version?: string;
  defaultConnMode?: string;
  auth?: OneBotAuthConfig;
  ws?: OneBotForwardWSConfig;
  wsReverse?: OneBotReverseWSConfig;
}

export interface OneBotRuntimeState {
  botId?: string;
  status?: 'disabled' | 'disconnected' | 'connecting' | 'connected';
  lastError?: string;
  updatedAt?: string | number;
}

export interface BotProfileView {
  id: string;
  name: string;
  avatarUrl?: string;
  channelRoleName?: string;
  connMode: 'forward_ws' | 'reverse_ws';
  remoteSelfId?: string;
  forwardHost?: string;
  forwardPort?: number;
  forwardApiPath?: string;
  forwardEventPath?: string;
  forwardUniversal?: string;
  reverseApiEndpoints?: string[];
  reverseEventUrls?: string[];
  reverseUniversalUrls?: string[];
  reverseUseUniversal?: boolean;
  reverseReconnectSec?: number;
  accessToken?: string;
  defaultChannelId?: string;
  enabled: boolean;
  createdAt?: string;
  updatedAt?: string;
  runtime?: OneBotRuntimeState;
}

export interface BotProfileOption {
  id: string;
  name: string;
  avatarUrl?: string;
  connMode?: string;
  userId?: string;
  remoteSelfId?: string;
}

export interface ChannelBotSettings {
  channelId: string;
  bindingId?: string;
  botId?: string;
  remoteChannelId?: string;
  remoteGroupId?: string;
  remoteNumericId?: string;
  enabled: boolean;
  updatedAt?: number;
}

export interface UserInfo {
  id: string;
  createdAt: null | string;
  updatedAt: null | string;
  deletedAt: null | string;
  username: string;
  nick: string;
  avatar: string;
  brief: string;
  roleIds?: string[];
  disabled: boolean;
}

export interface TalkMessage {
  id: string;
  time: number;
  name: string;
  content: string;
  isMe?: boolean;
  raw?: any;
}

// https://satori.js.org/zh-CN/resources/message.html#%E5%8F%91%E9%80%81%E6%B6%88%E6%81%AF
interface APIMessageCreate {
  // api: 'message.create'
  channel_id: string
  content: string
}

export interface SChannel extends Channel {
  isPrivate?: boolean;
  createdAt?: string; // 频道创建时间
  updatedAt?: string; // 频道最后更新时间
  rootId?: string; // 根频道ID
  recentSentAt?: number; // 最近发送消息的时间戳
  permType?: string; // 权限类型
  friendInfo?: FriendInfo; // 好友信息(如果是私聊频道)
  membersCount?: number; // 频道成员数量

  children?: SChannel[];
  sortOrder?: number;
  typingIndicatorSetting?: boolean;
  desc?: string;
  note?: string;
}

export type APIMessageCreateResp = Message

interface APIMessageGet {
  api: 'message.get'
  channel_id: string
  message_id: string
}

// 扩展部分
interface APIChannelCreate {
  api: 'channel.create'
  name: string
}

interface APIChannelList {
  // api: 'channel.list'
}


export interface APIChannelCreateResp {
  id: string
  name: string
  parent_id: string
  // type
}

export interface APIChannelListResp {
  echo?: string,
  data: {
    data: Channel[],
    next?: string,  
  }
}

export type APIMessage = APIMessageCreate | APIMessageGet | APIChannelList;

interface ModelDataBase {
  id: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface UserEmojiModel {
  id: string
  attachmentId: string;
  remark?: string;
  order?: number;
}

export interface GalleryCollection extends ModelDataBase {
  ownerType: 'user' | 'channel';
  ownerId: string;
  name: string;
  order: number;
  quotaUsed: number;
  createdBy?: string;
  updatedBy?: string;
}

export interface GalleryItem extends ModelDataBase {
  collectionId: string;
  attachmentId: string;
  thumbUrl: string;
  remark: string;
  tags?: string;
  order: number;
  createdBy: string;
  size: number;
}

export interface GallerySearchResponse {
  items: GalleryItem[];
  collections: Record<string, GalleryCollection>;
}

export enum ChannelType {
  Public = 'public',
  NonPublic = 'non-public',
  Private = 'private'
}


export interface FriendInfo extends ModelDataBase {
  userId1: string;
  userId2: string;
  isFriend: boolean;
  userInfo: null | UserInfo; // 这里的 'any' 可以根据实际情况替换为更具体的类型
}

export interface FriendRequestModel extends ModelDataBase {
  senderId: string;   // 发送者
  receiverId: string; // 接收者
  note: string;       // 申请理由
  status: string;     // 可能的值：pending, accept, reject

  userInfoSender?: UserInfo;
  userInfoReceiver?: UserInfo;

  userInfoTemp?: UserInfo;
}

// 频道角色类
export interface ChannelRoleModel extends ModelDataBase {
  name: string;
  desc: string;
  channelId: string;
}

export interface UserRoleModel extends ModelDataBase {
  roleType: string; // 可以是 "channel" 或 "system"
  userId: string;
  roleId: string;

  user?: UserInfo;
}

export interface PaginationListResponse<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

export interface ChannelIdentity {
  id: string;
  channelId: string;
  userId: string;
  displayName: string;
  color: string;
  avatarAttachmentId: string;
  isDefault: boolean;
  sortOrder: number;
}

export interface MessageIdentity {
  id?: string;
  displayName?: string;
  color?: string;
  avatarAttachment?: string;
}
