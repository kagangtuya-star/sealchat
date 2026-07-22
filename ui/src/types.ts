import type { User, Message, Guild, GuildMember, Opcode, GatewayPayloadStructure, Channel } from '@satorijs/protocol'
import type { TheaterPresentation, TheaterPresentationPatch } from '@/types/theaterPresentation'

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
  targetUserIds?: string[];
  targetDisplayNames?: string[];
}

declare module '@satorijs/protocol' {
  interface User {
    avatarDecoration?: AvatarDecoration;
  }
  interface Message {
    whisperMeta?: WhisperMeta;
    whisperToIds?: User[];
    senderRoleId?: string;
    isPinned?: boolean;
    pinnedAt?: number;
    pinnedBy?: string;
    isDeleted?: boolean;
    diceVisual?: DiceVisualPayload;
    deletedAt?: number;
    deletedBy?: string;
    reactions?: MessageReaction[];
  }
  interface Channel {
    defaultDiceExpr?: string;
    botCommandPrefixes?: string[];
    builtInDiceEnabled?: boolean;
    botFeatureEnabled?: boolean;
    primaryBotId?: string;
    eventBotIds?: string[];
    botWhisperForwardConfig?: string;
    characterApiEnabled?: boolean;
    characterApiReason?: string;
  }
}

export interface Dice3DSkin {
  faceBackground: string;
  faceForeground: string;
  edgeColor: string;
  outlineColor: string;
  roughness: number;
  metalness: number;
  scale: number;
  textures?: Record<string, string>;
}

export interface Dice3DMotionConfig {
  speed: number;
  throwForce: number;
  wallBounce: number;
  entryEdge: 'random' | 'top' | 'right' | 'bottom' | 'left';
  lingerMs: number;
  maxDice: number;
  interactive: boolean;
}

export interface Dice3DAudioConfig {
  enabled: boolean;
  volume: number;
  soundAssetId?: string;
}

export interface Dice3DCustomSurface {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Dice3DDockStack {
  id: string;
  label: string;
  expression: string;
  color?: string;
}

export interface Dice3DBotRule {
  id: string;
  name: string;
  enabled: boolean;
  channelIds?: string[];
  botUserIds?: string[];
  pattern: string;
  countGroup: string;
  sidesGroup: string;
  valuesGroup: string;
  valueSeparatorPattern: string;
  priority: number;
}

export interface Dice3DWorldConfig {
  version: number;
  platformStyleId?: string;
  enabled: boolean;
  surfaceMode: 'auto' | 'chat' | 'theater' | 'fullscreen' | 'custom';
  customSurface: Dice3DCustomSurface;
  defaultSkin: Dice3DSkin;
  motion: Dice3DMotionConfig;
  audio: Dice3DAudioConfig;
  botRules: Dice3DBotRule[];
}

export interface Dice3DMemberProfile {
  version: number;
  useOverride: boolean;
  skin: Dice3DSkin;
  audio?: Dice3DAudioConfig;
  dockEnabled: boolean;
  dockCorner: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right' | 'free';
  dockX: number;
  dockY: number;
  dockStacks: Dice3DDockStack[];
}

export interface Dice3DStylePreset {
  id: string;
  name: string;
  config: Dice3DWorldConfig;
  createdAt: number;
  updatedAt: number;
}

export interface DiceVisualPayload {
  version: number;
  rollId: string;
  messageId: string;
  channelId: string;
  actorUserId: string;
  seed: number;
  groups: Array<{ type: string; results: number[] }>;
  appearance: Dice3DSkin;
  motion: Dice3DMotionConfig;
  audio: Dice3DAudioConfig;
  surfaceMode: Dice3DWorldConfig['surfaceMode'];
  customSurface: Dice3DCustomSurface;
  createdAt: number;
}

export type BotWhisperForwardRuleType = 'legacy_hidden_dice' | 'keyword' | 'regex' | 'all';
export type BotWhisperForwardRuleLogic = 'any' | 'all';

export interface BotWhisperForwardRule {
  id: string;
  type: BotWhisperForwardRuleType;
  enabled: boolean;
  keyword?: string;
  pattern?: string;
  flags?: string;
}

export interface BotWhisperForwardConfig {
  enabled: boolean;
  asWhisper: boolean;
  appendAtTargetsWhenWhisper: boolean;
  ruleLogic: BotWhisperForwardRuleLogic;
  rules: BotWhisperForwardRule[];
}

export type BotOneBotTransportType = 'forward_ws' | 'reverse_ws' | 'http';

export interface BotOneBotConfig {
  enabled: boolean;
  transportType: BotOneBotTransportType;
  httpPathSuffix?: string;
  httpPostPathSuffix?: string;
  url?: string;
  apiUrl?: string;
  eventUrl?: string;
  useUniversalClient: boolean;
  reconnectIntervalMs: number;
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
  sender_identity_variant_id?: string;
  sender_identity_is_temporary?: boolean;
  isPinned?: boolean;
  pinnedAt?: number;
  pinnedBy?: string;
  isWhisper?: boolean;
  whisperTo?: User | null;
  whisperToIds?: User[];
}

export interface MessageReaction {
  emoji: string;
  count: number;
  meReacted: boolean;
}

export interface MessageReactionEvent {
  messageId: string;
  emoji: string;
  count: number;
  action: 'add' | 'remove';
  userId: string;
  timestamp: number;
}

export type BattleReportStatus = 'ready' | 'generating' | 'failed';

export interface BattleReport {
  id: string;
  channelId: string;
  worldId: string;
  title: string;
  content?: string;
  contentPreview: string;
  periodStart: number;
  periodEnd: number;
  contextReportCount: number;
  sortOrder: number;
  status: BattleReportStatus;
  errorMessage?: string;
  creatorId: string;
  updaterId: string;
  aiSource?: string;
  aiProviderId?: string;
  aiModel?: string;
  aiFeatureKey?: string;
  createdAt: number;
  updatedAt: number;
}

export interface BattleReportPayload {
  title?: string;
  content?: string;
  periodStart?: number;
  periodEnd?: number;
  contextReportCount?: number;
  source?: string;
  sourceChannelIds?: string[];
  aiProviderId?: string;
  aiModel?: string;
  aiFeatureKey?: string;
}

export interface BattleReportDisplayChannel {
  id: string;
  worldId: string;
  sourceChannelId: string;
  displayChannelId: string;
  displayName: string;
  enabled: boolean;
  createdAt: number;
  updatedAt: number;
}

import type { PlatformTheme } from '@/services/theme/themeTypes';

export interface LogUploadConfig {
  enabled?: boolean;
  endpoint?: string;
  endpoints?: string[];
  client?: string;
  uniformId?: string;
  version?: number;
  note?: string;
}

export interface TurnstileConfig {
  siteKey?: string;
  secretKey?: string;
}

export interface CaptchaCapConfig {
  challengeCount?: number;
  challengeSize?: number;
  challengeDifficulty?: number;
  challengeExpiresSeconds?: number;
  tokenTTLSeconds?: number;
}

export interface CaptchaTargetConfig {
  mode?: 'off' | 'local' | 'turnstile' | 'cap';
  turnstile?: TurnstileConfig;
  cap?: CaptchaCapConfig;
}

export interface CaptchaConfig {
  signup?: CaptchaTargetConfig;
  signin?: CaptchaTargetConfig;
  passwordReset?: CaptchaTargetConfig;
  mode?: 'off' | 'local' | 'turnstile' | 'cap';
  turnstile?: TurnstileConfig;
  cap?: CaptchaCapConfig;
}

export interface LoginBackgroundConfig {
  attachmentId?: string;
  mode?: 'cover' | 'contain' | 'tile' | 'center';
  opacity?: number;
  blur?: number;
  brightness?: number;
  overlayColor?: string;
  overlayOpacity?: number;
  panelAutoTint?: boolean;
  panelTintColor?: string;
  panelTintOpacity?: number;
  panelBlur?: number;
  panelSaturate?: number;
  panelContrast?: number;
  panelBorderOpacity?: number;
  panelShadowStrength?: number;
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
  file_missing?: boolean;
}

export interface ExportTaskListResponse {
  total: number;
  total_size: number;
  page: number;
  size: number;
  items: ExportTaskItem[];
}

export interface ServerAudioConfig {
  storageDir?: string;
  tempDir?: string;
  maxUploadSizeMB?: number;
  userQuotaMB?: number;
  allowedMimeTypes?: string[];
  enableTranscode?: boolean;
  defaultBitrateKbps?: number;
  alternateBitrates?: number[];
  ffmpegPath?: string;
  allowWorldAudioWorkbench?: boolean;
  allowNonAdminCreateWorld?: boolean;
}

export interface BackupConfig {
  enabled: boolean;
  intervalHours: number;
  retentionCount: number;
  path: string;
}

export interface SQLiteConfig {
  autoVacuumEnabled?: boolean;
  autoVacuumIntervalHours?: number;
}

export interface BackupInfo {
  filename: string;
  size: number;
  createdAt: number;
  protected: boolean;
}

export interface ThemeManagementConfig {
  platformThemes?: PlatformTheme[];
  defaultPlatformThemeId?: string;
  platformDice3DStyles?: Dice3DStylePreset[];
  defaultPlatformDice3DStyleId?: string;
}

export type CursorSlot = 'default' | 'pointer' | 'text' | 'grab' | 'grabbing' | 'not-allowed';
export type CursorMode = 'inherit' | 'browser' | 'custom';

export interface CursorAssetConfig {
  mode: CursorMode;
  attachmentId?: string;
  hotspotX?: number;
  hotspotY?: number;
  width?: number;
  height?: number;
  size?: number;
  animated?: boolean;
}

export interface CursorThemeConfig {
  version: 1;
  slots: Partial<Record<CursorSlot, CursorAssetConfig>>;
}

export interface UITextReplaceRule {
  id: string;
  searchText: string;
  replaceText: string;
  enabled: boolean;
}

export interface UITextReplaceConfig {
  enabled: boolean;
  rules: UITextReplaceRule[];
}

export type CertificateIssuer = 'letsencrypt_shortlived' | 'zerossl_90d';
export type CertificateChallenge = 'http-01' | 'tls-alpn-01';

export interface CertificateConfig {
  enabled: boolean;
  subjectIp: string;
  issuer: CertificateIssuer;
  challenge: CertificateChallenge;
  email: string;
  storageDir: string;
  httpsServeAt?: string;
  forceHTTPS: boolean;
  redirectHTTP: boolean;
  checkIntervalMinutes: number;
  renewBeforeDays: number;
  retryInitialMinutes: number;
  retryMaxMinutes: number;
  zeroSSLAPIKey?: string;
  zeroSSLEABKeyID?: string;
  zeroSSLEABMACKey?: string;
  staging?: boolean;
}

export interface CertificateStatus {
  enabled: boolean;
  runtimeActive: boolean;
  subjectIp: string;
  issuer: string;
  challenge: string;
  certificatePresent: boolean;
  notBefore?: string;
  notAfter?: string;
  remainingDays: number;
  lastError?: string;
  lastCheckAt?: string;
  lastSuccessAt?: string;
  nextCheckAt?: string;
  retryCount: number;
  retrying: boolean;
  renewBeforeDays: number;
  checkIntervalMinutes: number;
  retryInitialMinutes: number;
  retryMaxMinutes: number;
}

export interface CertificateLogEntry {
  time: string;
  level: string;
  event: string;
  message: string;
  subjectIp?: string;
  issuer?: string;
  challenge?: string;
}

export interface PerformanceProfilerConfig {
  enabled: boolean;
  outputDir: string;
  lightSampleIntervalSec: number;
  snapshotIntervalSec: number;
  cpuProfileDurationSec: number;
  retentionDays: number;
}

export type AIRoutingMode = 'round_robin';
export type AIFeatureAccessMode = 'all' | 'users' | 'worlds' | 'users_or_worlds';
export type AIRunSource = 'platform' | 'user';

export interface AIModelParams {
  temperature?: number;
  maxTokens?: number;
  maxInputChars?: number;
  topP?: number;
}

export interface AIFeatureAccessConfig {
  mode: AIFeatureAccessMode;
  userIds: string[];
  worldIds: string[];
}

export interface AIFeatureConfig {
  enabled: boolean;
  userCustomOnly: boolean;
  defaultPrompt: string;
  defaultModel: string;
  params: AIModelParams;
  access: AIFeatureAccessConfig;
}

export interface AIRetryConfig {
  maxAttempts: number;
  initialDelayMs: number;
  maxDelayMs: number;
}

export interface AIRoutingConfig {
  mode: AIRoutingMode;
}

export interface AIProviderConfig {
  id: string;
  name: string;
  enabled: boolean;
  baseUrl: string;
  apiKey?: string;
  models: string[];
  selectedModel?: string;
  weight: number;
}

export interface AIModelPricingConfig {
  providerId: string;
  model: string;
  promptPricePer1MTokens: number;
  completionPricePer1MTokens: number;
  cachePricePer1MTokens: number;
}

export interface AIQuotaPolicyConfig {
  dailyLimit?: number | null;
  monthlyLimit?: number | null;
  lifetimeLimit?: number | null;
}

export interface UserAIProviderProfile {
  id: string;
  name: string;
  enabled: boolean;
  baseUrl: string;
  apiKey?: string;
  models: string[];
  selectedModel?: string;
  hasApiKey?: boolean;
}

export interface UserAIFeatureBinding {
  providerId: string;
  model: string;
}

export interface UserAISettings {
  profiles: UserAIProviderProfile[];
  featureBindings: Record<string, UserAIFeatureBinding>;
}

export interface AIConfig {
  enabled: boolean;
  routing: AIRoutingConfig;
  retry: AIRetryConfig;
  providers: AIProviderConfig[];
  features: Record<string, AIFeatureConfig>;
  pricing: AIModelPricingConfig[];
  logRetentionDays: number;
  quotaDefault: AIQuotaPolicyConfig;
}

export interface AIFeatureCapability {
  key: string;
  enabled: boolean;
  userCustomOnly: boolean;
  defaultPrompt?: string;
  defaultModel?: string;
  params?: AIModelParams;
}

export interface AdminAIUsageLogItem {
  id: string;
  userId: string;
  usernameSnapshot: string;
  nicknameSnapshot?: string;
  featureKey: string;
  providerId: string;
  model: string;
  source: AIRunSource;
  status: string;
  promptTokens: number;
  completionTokens: number;
  cacheTokens: number;
  promptPricePer1M: number;
  completionPricePer1M: number;
  cachePricePer1M: number;
  promptCost: number;
  completionCost: number;
  cacheCost: number;
  totalCost: number;
  latencyMs: number;
  startedAt: string;
  finishedAt: string;
  errorMessage?: string;
}

export interface AdminAIUsageLogListResult {
  items: AdminAIUsageLogItem[];
  page: number;
  pageSize: number;
  total: number;
}

export type AdminAIQuotaPolicySource = 'default' | 'override';

export interface AdminAIQuotaUsageSummary {
  dailySettled: number;
  monthlySettled: number;
  lifetimeSettled: number;
  activeReserved: number;
}

export interface AdminAIQuotaDetail {
  userId: string;
  username: string;
  nickname: string;
  source: AdminAIQuotaPolicySource;
  defaultPolicy: AIQuotaPolicyConfig;
  override?: AIQuotaPolicyConfig | null;
  effectivePolicy: AIQuotaPolicyConfig;
  usage: AdminAIQuotaUsageSummary;
}

export interface AdminAIQuotaListResult {
  items: AdminAIQuotaDetail[];
  page: number;
  pageSize: number;
  total: number;
}

export interface ServerConfig {
  serveAt: string;
  domain: string;
  registerOpen: boolean;
  registerInviteCode?: string;
  registerInviteRequired?: boolean;
  webUrl: string;
  pageTitle?: string;
  pageDescription?: string;
  faviconAttachmentId?: string;
  chatHistoryPersistentDays: number;
  messageSortBasis?: 'typing_start' | 'send_time';
  imageSizeLimit: number;
  imageCompress: boolean;
  imageCompressQuality: number;
  keywordMaxLength?: number;
  builtInSealBotEnable: boolean;
  botIncomingParenAsOoc?: boolean;
  logUpload?: LogUploadConfig;
  captcha?: CaptchaConfig;
  emailNotification?: {
    enabled: boolean;
    minDelayMinutes?: number;
    maxDelayMinutes?: number;
  };
  emailAuth?: {
    enabled: boolean;
  };
  backup?: BackupConfig;
  sqlite?: SQLiteConfig;
  audio?: ServerAudioConfig;
  ffmpegAvailable?: boolean;
  audioImportEnabled?: boolean;
  loginBackground?: LoginBackgroundConfig;
  themeManagement?: ThemeManagementConfig;
  cursorTheme?: CursorThemeConfig;
  uiTextReplace?: UITextReplaceConfig;
  certificate?: CertificateConfig;
  ai?: AIConfig;
  performanceProfiler?: PerformanceProfilerConfig;
}

export interface UserInfo {
  id: string;
  createdAt: null | string;
  updatedAt: null | string;
  deletedAt: null | string;
  username: string;
  nick: string;
  avatar: string;
  avatarDecoration?: AvatarDecoration | null;
  nick_color?: string;
  brief: string;
  roleIds?: string[];
  disabled: boolean;
  is_bot?: boolean;
  email?: string;
  emailVerified?: boolean;
  emailVerifiedAt?: string;
}

export interface AvatarDecorationSettings {
  scale?: number;
  offsetX?: number;
  offsetY?: number;
  rotation?: number;
  zIndex?: number;
  opacity?: number;
  playbackRate?: number;
  blendMode?: string;
}

export interface AvatarDecoration {
  id?: string;
  enabled: boolean;
  decorationId?: string;
  resourceAttachmentId?: string;
  fallbackAttachmentId?: string;
  settings?: AvatarDecorationSettings;
}

export interface ChannelMemberCandidateItem {
  userId: string;
  username: string;
  nickname: string;
  avatar: string;
  worldRole: string;
  joinedAt?: string;
  alreadyInChannel: boolean;
}

export interface ChannelMemberCandidatesResponse {
  items: ChannelMemberCandidateItem[];
  page: number;
  pageSize: number;
  total: number;
}

export interface ChannelAddWorldMembersResponse {
  roleId: string;
  candidateCount: number;
  addedCount: number;
  skippedExistingCount: number;
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

export interface ChannelBackgroundSettings {
  mode: 'cover' | 'contain' | 'tile' | 'center';
  opacity: number;       // 0-100
  blur: number;          // 0-20 (px)
  brightness: number;    // 50-150 (%)
  overlayColor?: string; // rgba color
  overlayOpacity?: number; // 0-100
}

export interface BackgroundPreset {
  id: string;
  name: string;
  category?: string;
  attachmentId: string;
  thumbnailUrl?: string;
  settings: ChannelBackgroundSettings;
  createdAt: number;
}

export interface SChannel extends Channel {
  isPrivate?: boolean;
  worldId?: string;
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
  primaryBotId?: string;
  eventBotIds?: string[];
  botWhisperForwardConfig?: string;
  characterApiEnabled?: boolean;
  characterApiReason?: string;
  backgroundAttachmentId?: string;
  backgroundSettings?: ChannelBackgroundSettings | string;
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
  collectionType?: string;
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
  avatarDecoration?: AvatarDecoration | null;
  avatarDecorations?: AvatarDecoration[] | null;
  characterCardId?: string;
  isDefault: boolean;
  isTemporary: boolean;
  botAppearanceMode?: 'inherit' | 'custom' | '';
  icOocOnActivate?: '' | 'ic' | 'ooc';
  sortOrder: number;
  folderIds?: string[];
  theaterPresentation?: TheaterPresentation | null;
}

export interface ChannelIdentityVariant {
  id: string;
  identityId: string;
  channelId: string;
  userId: string;
  selectorEmoji: string;
  keyword: string;
  note: string;
  avatarAttachmentId: string;
  displayName?: string;
  color?: string;
  appearance?: Record<string, any>;
  theaterPresentation?: TheaterPresentationPatch | null;
  sortOrder: number;
  enabled: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface ChannelIcOocRoleConfig {
  icRoleId: string | null;
  oocRoleId: string | null;
}

export interface ChannelIdentityManageCandidate {
  userId: string;
  username: string;
  nickname: string;
  avatar: string;
  rank: number;
  roleLabel: string;
  isSelf: boolean;
}

export interface ChannelIdentityManageCandidatesResponse {
  items: ChannelIdentityManageCandidate[];
  total: number;
  page: number;
  pageSize: number;
}

export interface CharacterCard {
  id: string;
  userId: string;
  channelId: string;
  name: string;
  sheetType: string;
  attrs: Record<string, any>;
  templateMode?: 'managed' | 'detached';
  templateId?: string;
  templateSnapshot?: string;
  createdAt?: string;
  updatedAt?: string;
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
  variantId?: string;
  displayName?: string;
  color?: string;
  avatarAttachment?: string;
  avatarDecoration?: AvatarDecoration | null;
  avatarDecorations?: AvatarDecoration[] | null;
  theaterPresentation?: TheaterPresentation | null;
  isTemporary?: boolean;
}
