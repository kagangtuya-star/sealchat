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
    senderRoleId?: string;
    isDeleted?: boolean;
    deletedAt?: number;
    deletedBy?: string;
  }
  interface Channel {
    defaultDiceExpr?: string;
    builtInDiceEnabled?: boolean;
    botFeatureEnabled?: boolean;
  }
}

export interface SatoriMessage {
  id?: string;
  channel?: Channel;
  guild?: Guild;
  user?: User;
  identity?: MessageIdentity;
  senderRoleId?: string;
  member?: GuildMember;
  content?: string;
  elements?: any[]; // Element[] 这个好像会让vscode提示一个错误
  timestamp?: number;
  quote?: SatoriMessage;
  createdAt?: number;
  updatedAt?: number;
  displayOrder?: number;

  sender_member_name?: string;
  sender_role_id?: string;
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

export interface TurnstileConfig {
  siteKey?: string;
  secretKey?: string;
}

export interface CaptchaTargetConfig {
  mode?: 'off' | 'local' | 'turnstile';
  turnstile?: TurnstileConfig;
}

export interface CaptchaConfig {
  signup?: CaptchaTargetConfig;
  signin?: CaptchaTargetConfig;
  mode?: 'off' | 'local' | 'turnstile';
  turnstile?: TurnstileConfig;
}

export interface ExportTaskItem {
  task_id: string;
  format: string;
  status: string;
  display_name?: string;
  file_name?: string;
  file_size: number;
  finished_at?: number;
  requested_at: number;
  message?: string;
  upload_url?: string;
  download_url: string;
}

export interface ExportTaskListResponse {
  total: number;
  total_size: number;
  page: number;
  size: number;
  items: ExportTaskItem[];
}

export interface ServerConfig {
  serveAt: string;
  domain: string;
  registerOpen: boolean;
  webUrl: string;
  pageTitle?: string;
  chatHistoryPersistentDays: number;
  imageSizeLimit: number;
  imageCompress: boolean;
  imageCompressQuality: number;
  keywordMaxLength?: number;
  builtInSealBotEnable: boolean;
  logUpload?: LogUploadConfig;
  captcha?: CaptchaConfig;
  emailNotification?: {
    enabled: boolean;
    minDelayMinutes?: number;
    maxDelayMinutes?: number;
  };
}

export interface UserInfo {
  id: string;
  createdAt: null | string;
  updatedAt: null | string;
  deletedAt: null | string;
  username: string;
  nick: string;
  avatar: string;
  nick_color?: string;
  brief: string;
  roleIds?: string[];
  disabled: boolean;
  is_bot?: boolean;
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
  defaultDiceExpr?: string;
  builtInDiceEnabled?: boolean;
  botFeatureEnabled?: boolean;
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
  worldId?: string
}

interface APIChannelList {
  // api: 'channel.list'
  world_id?: string
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
    world_id?: string,
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

export interface DiceMacro {
  id: string;
  channelId: string;
  digits: string;
  label: string;
  expr: string;
  note?: string;
  favorite?: boolean;
  createdAt?: string;
  updatedAt?: string;
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
  folderIds?: string[];
}

export interface ChannelIdentityFolder {
  id: string;
  channelId: string;
  userId: string;
  name: string;
  sortOrder: number;
}

export interface MessageIdentity {
  id?: string;
  displayName?: string;
  color?: string;
  avatarAttachment?: string;
}
