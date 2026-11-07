// LOG 排版器 v1 wire DTO（doc/log-layout/01-contract.md）。
// 新 LOG REST 为 camelCase；消息命令仍沿用既有 snake_case 协议。

export interface LogCapabilities {
  editDraft: boolean;
  manageSources: boolean;
  manageLinks: boolean;
  publish: boolean;
  editOwnMessages: boolean;
  editOthersMessages: boolean;
  bulkAppearance: boolean;
}

export interface LogMessageCapabilities {
  edit: boolean;
  changeIdentity: boolean;
  reorder: boolean;
  revoke: boolean;
  remove: boolean;
  editAppearance: boolean;
}

export interface LogSourceSpec {
  channelId: string;
  fromTime: string | null;
  /** 接续片段可使用排他起点，避免相邻时间范围重复或留出毫秒级空隙。 */
  fromExclusive?: boolean;
  toTime: string;
  includeOoc: boolean;
  includeArchived: boolean;
  /** 是否在 LOG 中展示回复消息附带的引用块；旧草稿缺失时等价于 false。 */
  showQuoteContent?: boolean;
  identityIds: string[];
}

/**
 * 来源片段：单频道的一段线性消息范围，是查询、游标、排除集合与元素锚点的边界。
 * 章节模式下 title 只是编排用的片段名称；视觉章节标题属于 LogChapter。
 */
export interface LogSection {
  id: string;
  title: string;
  source: LogSourceSpec;
  order: number;
  /** 所属视觉章节；草稿没有 chapters 时（旧草稿）不存在，每个片段自成一章。 */
  chapterId?: string;
}

/** 视觉/叙事章节：只做分组与命名，不是查询单元。 */
export interface LogChapter {
  id: string;
  title: string;
  order: number;
}

export type LogAnchor =
  | { kind: 'sectionStart' | 'sectionEnd'; sectionId: string }
  | { kind: 'message'; sectionId: string; messageId: string; side: 'before' | 'after' };

export const LOG_ELEMENT_TYPES = [
  'cover', 'imagePlate', 'pageBreak', 'transition', 'quote',
  'prologue', 'epilogue', 'heading', 'toc', 'custom',
] as const;
export type LogElementType = typeof LOG_ELEMENT_TYPES[number];

export interface LogElement {
  id: string;
  type: LogElementType;
  anchor: LogAnchor;
  order: number;
  templateId: string;
  content: Record<string, unknown> & { imageWidth?: number };
  style: Record<string, unknown>;
}

export interface LogPageSettings {
  preset: 'a4' | 'a5' | 'custom';
  widthMm: number;
  heightMm: number;
  marginsMm: { top: number; right: number; bottom: number; left: number };
  gutterMm: number;
  columns: 1 | 2;
  backgroundColor: string;
  /** 样式包内图片资源 id（样式资源）。 */
  backgroundAssetId: string | null;
  /** 当前 LOG 工程素材 id（LogDraft.assets）；与 backgroundAssetId 互斥，旧草稿不存在。 */
  backgroundProjectAssetId?: string | null;
  fontFamily: string;
  fontSizePt: number;
  lineHeight: number;
  headerTemplateId: string | null;
  footerTemplateId: string | null;
  showPageNumbers: boolean;
  mergeNeighbors: boolean;
}

export type LogTokenValue = string | number | boolean;

/**
 * LOG 工程素材登记：文件本体是既有附件，登记只是本工程内的引用授权。
 * 与样式包资源分开：元素 content.projectAssetId / 页面 backgroundProjectAssetId 引用它，
 * content.assetId / backgroundAssetId 仍只指样式包资源。
 */
export interface LogProjectAsset {
  id: string;
  attachmentId: string;
  name: string;
}

/** 当前 LOG 工程的显示覆盖；所有已填写条件同时满足，按数组顺序 first-match。 */
export interface LogDisplayRule {
  id: string;
  match: { messageType?: string; identityId?: string; userId?: string; variantId?: string };
  templateId: string;
}

export interface LogDraft {
  schemaVersion: 1;
  title: string;
  sections: LogSection[];
  /** 可选的章节层；存在时每个 section 都必须引用其中一个章节，且同章片段在 order 上连续。 */
  chapters?: LogChapter[];
  elements: LogElement[];
  page: LogPageSettings;
  stylePackageId: string;
  stylePackageVersion: string;
  themeTokens: Record<string, LogTokenValue>;
  /** 可选的工程图片素材登记；不随样式包切换。 */
  assets?: LogProjectAsset[];
  /** 正文图片宽度 / 当前内容栏宽度，key 为 message:<id>:image:<index>。 */
  imageWidths?: Record<string, number>;
  displayRules?: LogDisplayRule[];
  messageTemplateOverrides?: Record<string, string>;
}

export interface LogProjectContext {
  projectId: string;
  worldId: string;
  enabled: boolean;
  draftVersion: number;
  draft: LogDraft;
  capabilities: LogCapabilities;
  publishedReleaseId: string | null;
  management: null | { layoutSlug: string; viewSlug: string };
}

/** 设计接口返回的既有协议消息；只声明 LOG 使用到的字段。 */
export interface LogProtocolMessage {
  id: string;
  version?: number;
  channel?: { id: string; name?: string } | null;
  user?: { id: string; nick?: string; name?: string; username?: string; avatar?: string; is_bot?: boolean; isBot?: boolean } | null;
  member?: { nick?: string; name?: string } | null;
  identity?: {
    id: string;
    variantId?: string;
    sharedIdentityId?: string;
    displayName: string;
    color: string;
    avatarAttachment: string;
  } | null;
  content: string;
  createdAt: number;
  updatedAt?: number;
  displayOrder: number;
  icMode?: string;
  isWhisper?: boolean;
  isArchived?: boolean;
  isDeleted?: boolean;
  isEdited?: boolean;
  quote?: LogProtocolMessage | null;
  diceVisual?: unknown;
  sender_identity_id?: string;
  sender_identity_variant_id?: string;
  sender_member_name?: string;
}

/** 悄悄话收件人：key 对每个收件人稳定（设计态为用户 ID，发布版为公开哈希）。 */
export interface LogWhisperTarget {
  key: string;
  name: string;
}

export interface LogQuotePresentation {
  speakerName: string;
  text: string;
}

/**
 * 服务端展示投影（与发布版同源，只读）：message.content 仍是唯一可编辑原文。
 * embed 仅当整条消息都是站内嵌入链接时出现；whisper 与消息类型正交。
 */
/** 一条展开线索的公开展示语义；不含专属信息、管理备注、权限或线索箱 presentation。 */
export interface LogWorldClue {
  title: string;
  /** text / image / iframe（网页线索）。 */
  kind: string;
  kindLabel: string;
  /** 网页线索的 http(s) 来源，仅作展示文字。 */
  source?: string;
}

export interface LogMessagePresentation {
  embed?: { kind: 'stickyNote' | 'worldClue' | string; bodyHtml: string; plainText: string; clues?: LogWorldClue[] };
  /** 空字符串表示服务端无法安全解析历史头像。 */
  avatarToken?: string;
  whisper?: { targets: LogWhisperTarget[] };
  quote?: LogQuotePresentation;
}

export interface LogDesignMessage {
  message: LogProtocolMessage;
  version: number;
  capabilities: LogMessageCapabilities;
  presentation?: LogMessagePresentation;
}

export interface LogMessageWindow {
  items: LogDesignMessage[];
  nextCursor: string | null;
  sourceVersion: number;
}

export type LogJobState = 'queued' | 'running' | 'done' | 'failed';

export interface LogReleaseJob {
  jobId: string;
  status: LogJobState;
  progress: { done: number; total: number };
  releaseId?: string;
  error?: { code: string; message: string };
}

export interface LogStyleSummary {
  id: string;
  version: string;
  name: string;
  builtin: boolean;
  previewAssetId: string | null;
}

export interface LogStyleRule {
  messageType?: string;
  identityKey?: string;
  variantKey?: string;
  templateId: string;
}

export interface LogStyleAsset {
  id: string;
  path: string;
  mime: string;
  sha256: string;
}

export interface LogStylePackage {
  schemaVersion: 1;
  id: string;
  version: string;
  name: string;
  license: string;
  minRendererVersion: string;
  tokens: Record<string, LogTokenValue>;
  templates: Record<string, string>;
  css: string;
  rules: LogStyleRule[];
  settingsSchema: Record<string, unknown>;
  assets: LogStyleAsset[];
}

export interface LogPublicStylePackage {
  schemaVersion: 1;
  version: string;
  name: string;
  license: string;
  minRendererVersion: string;
  tokens: Record<string, LogTokenValue>;
  templates: Record<string, string>;
  css: string;
  rules: LogStyleRule[];
  settingsSchema: Record<string, unknown>;
  assets: Array<{ assetKey: string; mime: string; sha256: string }>;
}

export type LogPublicMessageType = 'ic' | 'ooc' | 'narration' | 'dice' | 'bot' | 'image' | 'unknown';

export interface LogPublicMessageNode {
  id: string;
  kind: 'message';
  sectionId: string;
  messageType: LogPublicMessageType;
  /** release 内独立域哈希；绝不是真实角色/用户 ID。旧发布节点缺省。 */
  identityKey?: string;
  userKey?: string;
  /** 仅本条消息的模板覆盖，不含源消息 ID。 */
  templateOverride?: string;
  speaker: { key: string; name: string; color: string | null; avatarAssetKey: string | null; variantKey: string | null };
  bodyHtml: string;
  /** 发布时按正文图片序号投影，不公开源消息 ID。 */
  imageWidths?: Record<string, number>;
  plainText: string;
  createdAt: string | null;
  quoteNodeId: string | null;
  /** 由公开读取接口按引用目标的当前可见性填充；旧客户端忽略。 */
  quote?: LogQuotePresentation | null;
  /** 整条消息是站内嵌入（便签/线索）时的种类；正文已是展开内容。 */
  embed?: string | null;
  /** embed 为 worldClue 时按正文顺序列出各线索；旧发布节点没有该字段。 */
  clues?: LogWorldClue[] | null;
  /** 发布版不收录悄悄话；字段为结构兼容保留。 */
  whisper?: { targets: LogWhisperTarget[] } | null;
}

export interface LogPublicElementNode {
  id: string;
  kind: 'element';
  sectionId: string;
  type: LogElementType;
  templateId: string;
  content: Record<string, unknown>;
  style: Record<string, unknown>;
}

export type LogPublicNode = LogPublicMessageNode | LogPublicElementNode;

export interface LogPublicDocument {
  schemaVersion: 1;
  title: string;
  releaseId: string;
  privacyVersion: number;
  /** 发布顺序；章节模式下 title 为所属章节标题。 */
  sections: Array<{ id: string; title: string; chapterId?: string }>;
  /** 仅章节模式的发布版提供；旧发布版缺省时每个 section 即一章。 */
  chapters?: Array<{ id: string; title: string; sectionIds: string[] }>;
  page: LogPageSettings;
  style: LogPublicStylePackage | null;
  /** match 中的身份/用户/差分值均已按 release 哈希，规则 ID 也是公开投影。 */
  displayRules?: LogDisplayRule[];
}

export interface LogPublicItems {
  releaseId: string;
  privacyVersion: number;
  items: LogPublicNode[];
  nextCursor?: string | null;
}

export interface LogPublicSearchHit {
  nodeId: string;
  sectionId: string;
  excerpt: string;
}

export interface LogAppearancePatch {
  name?: string;
  color?: string;
  avatar_attachment_id?: string;
  variant_id?: string;
}

export interface LogAppearanceSelection {
  identity_id: string | null;
  shared_identity_id: string | null;
  variant_id: string | null;
  from_time: string;
  to_time: string;
}

export interface LogAppearancePreview {
  preview_id: string;
  expires_at: string;
  matched_count: number;
  allowed_count: number;
  denied_count: number;
  samples: Array<{ message_id: string; before: Record<string, unknown>; after: Record<string, unknown> }>;
}

export interface LogMutationJob {
  jobId: string;
  status: LogJobState;
  matched: number;
  succeeded: number;
  conflicted: number;
  denied: number;
  failed: number;
  itemsCursor?: string | null;
}

export interface LogMutationJobItem {
  messageId: string;
  status: 'success' | 'conflict' | 'denied' | 'failed';
  code?: string;
}
