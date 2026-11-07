// LOG REST 适配层：所有新接口的 `{data}` / `{error}` 包裹只在这里拆一次。
import axiosFactory, { isAxiosError, type AxiosResponse } from 'axios';
import { api, urlBase } from '@/stores/_config';
import type { ChannelIdentityManageCandidatesResponse } from '@/types';
import type {
  LogDesignMessage,
  LogDraft,
  LogMessageWindow,
  LogMutationJob,
  LogMutationJobItem,
  LogProjectContext,
  LogPublicDocument,
  LogPublicItems,
  LogPublicSearchHit,
  LogReleaseJob,
  LogStylePackage,
  LogStyleSummary,
} from '@/types/log';

export type LogErrorCode =
  | 'INVALID_INPUT' | 'UNAUTHENTICATED' | 'PERMISSION_DENIED' | 'NOT_FOUND'
  | 'VERSION_CONFLICT' | 'SOURCE_CHANGED' | 'SLUG_TAKEN' | 'TOO_LARGE'
  | 'INVALID_TEMPLATE' | 'RATE_LIMITED' | 'INTERNAL_ERROR' | 'PREVIEW_EXPIRED'
  | 'INVALID_PATCH' | 'INVALID_IDENTITY' | 'NETWORK_ERROR' | 'COMMAND_FAILED'
  | 'PDF_UNAVAILABLE' | 'PDF_EXPORT_FAILED';

/** 前端统一的 LOG 领域错误；REST 与 WS 命令错误都转换成它。 */
export class LogDomainError extends Error {
  constructor(
    readonly code: LogErrorCode | string,
    message: string,
    readonly status = 0,
    readonly details: Record<string, unknown> = {},
  ) {
    super(message);
    this.name = 'LogDomainError';
  }
}

export const isLogError = (error: unknown, ...codes: string[]): error is LogDomainError =>
  error instanceof LogDomainError && (codes.length === 0 || codes.includes(error.code));

const fallbackMessages: Record<string, string> = {
  UNAUTHENTICATED: '请先登录',
  PERMISSION_DENIED: '没有权限执行此操作',
  NOT_FOUND: '内容不存在或已失效',
  VERSION_CONFLICT: '内容已被其他人修改，请重新加载',
  SOURCE_CHANGED: '来源消息已变化，需要重新加载',
  SLUG_TAKEN: '入口名称已被使用',
  TOO_LARGE: '内容超过大小限制',
  INVALID_TEMPLATE: '样式、模板或元素无效',
  RATE_LIMITED: '请求过于频繁，请稍后再试',
  NETWORK_ERROR: '网络连接失败',
};

export const describeLogError = (error: unknown, fallback = '操作失败') => {
  if (error instanceof LogDomainError) {
    return error.message || fallbackMessages[error.code] || fallback;
  }
  return error instanceof Error && error.message ? error.message : fallback;
};

export const toLogError = (error: unknown): LogDomainError => {
  if (error instanceof LogDomainError) return error;
  if (isAxiosError(error)) {
    const status = error.response?.status || 0;
    const body = error.response?.data as { error?: { code?: string; message?: string; details?: Record<string, unknown> } } | undefined;
    const domain = body && typeof body === 'object' ? body.error : undefined;
    if (domain?.code) {
      return new LogDomainError(domain.code, domain.message || fallbackMessages[domain.code] || '操作失败', status, domain.details || {});
    }
    if (!error.response) return new LogDomainError('NETWORK_ERROR', fallbackMessages.NETWORK_ERROR);
    const code = status === 401 ? 'UNAUTHENTICATED' : status === 403 ? 'PERMISSION_DENIED' : status === 404 ? 'NOT_FOUND' : 'INTERNAL_ERROR';
    return new LogDomainError(code, fallbackMessages[code] || `请求失败（${status}）`, status);
  }
  return new LogDomainError('INTERNAL_ERROR', error instanceof Error ? error.message : '操作失败');
};

const unwrap = async <T>(request: Promise<AxiosResponse<{ data: T }>>): Promise<T> => {
  try {
    const resp = await request;
    return resp.data?.data as T;
  } catch (error) {
    throw toLogError(error);
  }
};

const seg = (value: string) => encodeURIComponent(value);

// ---- 工程与设计 ----

export const fetchWorldLogProject = (worldId: string) =>
  unwrap<LogProjectContext | null>(api.get(`api/v1/worlds/${seg(worldId)}/log-project`));

export interface LogProjectUpdateInput {
  expectedVersion: number | null;
  enabled: boolean;
  layoutSlug?: string;
  viewSlug?: string;
}
export const updateWorldLogProject = (worldId: string, input: LogProjectUpdateInput) =>
  unwrap<LogProjectContext>(api.put(`api/v1/worlds/${seg(worldId)}/log-project`, input));

export const resolveLogLayout = (slug: string) =>
  unwrap<LogProjectContext>(api.get(`api/v1/log/layout/${seg(slug)}`));

export const saveLogDraft = (projectId: string, expectedVersion: number, draft: LogDraft) =>
  unwrap<{ draftVersion: number; draft: LogDraft }>(api.put(`api/v1/log/projects/${seg(projectId)}/draft`, { expectedVersion, draft }));

export interface LogStandardExportInput {
  expectedDraftVersion: number;
  format: 'txt' | 'docx' | 'html' | 'json';
  displayName: string;
  includeImages: boolean;
  includeDiceCommands: boolean;
  withoutTimestamp: boolean;
  withoutOocParentheses: boolean;
  mergeMessages: boolean;
  autoCorrectPunctuation: boolean;
  textBbcodeColorize: boolean;
}

export const createLogStandardExport = (projectId: string, input: LogStandardExportInput) =>
  unwrap<{ task_id: string; status: string; message: string; requested_at: number }>(
    api.post(`api/v1/log/projects/${seg(projectId)}/export`, input),
  );

/** 后台 PDF 只需要版本与文件名；排版、样式与分页全部来自已保存草稿。 */
export interface LogPdfExportInput {
  expectedDraftVersion: number;
  format: 'pdf';
  displayName: string;
}

export const createLogPdfExport = (projectId: string, input: LogPdfExportInput) =>
  unwrap<{ task_id: string; status: string; message: string; requested_at: number }>(
    api.post(`api/v1/log/projects/${seg(projectId)}/export`, input),
  );

/** 服务器后台 PDF 能力；不包含任何外部工具路径。 */
export interface LogPdfCapability {
  enabled: boolean;
  available: boolean;
  probing: boolean;
  runtimeKind?: string;
  mergerAvailable: boolean;
  reason?: string;
}

export const fetchLogPdfCapability = () =>
  unwrap<LogPdfCapability>(api.get('api/v1/log/pdf-capability'));

export const fetchLogSectionMessages = (projectId: string, sectionId: string, cursor: string | null, limit = 100, signal?: AbortSignal) =>
  unwrap<LogMessageWindow>(api.get(`api/v1/log/projects/${seg(projectId)}/sections/${seg(sectionId)}/messages`, {
    params: cursor ? { cursor, limit } : { limit },
    signal,
  }));

export interface LogSourceRangeCheck {
  count: number;
  latestTime: string | null;
  draftVersion: number;
  sourceVersion: number;
}

export const checkLogSourceRange = (projectId: string, sectionId: string, expectedVersion: number) =>
  unwrap<LogSourceRangeCheck>(api.get(`api/v1/log/projects/${seg(projectId)}/sections/${seg(sectionId)}/range-check`, {
    params: { expectedVersion },
  }));

export const fetchLogMessageContext = (projectId: string, sectionId: string, messageId: string, signal?: AbortSignal) =>
  unwrap<{ items: LogDesignMessage[]; sourceVersion: number }>(
    api.get(`api/v1/log/projects/${seg(projectId)}/sections/${seg(sectionId)}/message-context/${seg(messageId)}`, { signal }),
  );

export const updateLogExclusions = (projectId: string, sectionId: string, expectedVersion: number, messageIds: string[], excluded: boolean) =>
  unwrap<{ draftVersion: number }>(api.put(`api/v1/log/projects/${seg(projectId)}/sections/${seg(sectionId)}/exclusions`, { expectedVersion, messageIds, excluded }));

// ---- 发布 ----

export const queueLogRelease = (projectId: string, expectedVersion: number, clientOpId: string) =>
  unwrap<{ jobId: string; status: 'queued' }>(api.post(`api/v1/log/projects/${seg(projectId)}/releases`, { expectedVersion, clientOpId }));

export const fetchLogReleaseJob = (projectId: string, jobId: string) =>
  unwrap<LogReleaseJob>(api.get(`api/v1/log/projects/${seg(projectId)}/jobs/${seg(jobId)}`));

export const withdrawLogPublication = async (projectId: string, expectedVersion: number) => {
  try {
    await api.delete(`api/v1/log/projects/${seg(projectId)}/publication`, { data: { expectedVersion } });
  } catch (error) {
    throw toLogError(error);
  }
};

// ---- 样式包 ----

export const fetchLogStyles = (cursor: string | null = null, limit = 50) =>
  unwrap<{ items: LogStyleSummary[]; nextCursor: string | null }>(api.get('api/v1/log/styles', { params: cursor ? { cursor, limit } : { limit } }));

export const fetchLogStylePackage = (id: string, version: string) =>
  unwrap<LogStylePackage>(api.get(`api/v1/log/styles/${seg(id)}/versions/${seg(version)}`));

export const forkLogStyle = (id: string, version: string, name: string) =>
  unwrap<LogStylePackage>(api.post(`api/v1/log/styles/${seg(id)}/fork`, { version, name }));

/** 新建私有样式的首个版本：服务端分配包 ID 与 1.0.0，新资源只能是自己上传的附件。 */
export const createLogStyle = (pkg: LogStylePackage) =>
  unwrap<LogStylePackage>(api.post('api/v1/log/styles', { package: pkg }));

export const deleteLogStyle = (id: string) =>
  unwrap<{ deleted: boolean }>(api.delete(`api/v1/log/styles/${seg(id)}`));

export const saveLogStyleVersion = (id: string, baseVersion: string, pkg: LogStylePackage) =>
  unwrap<LogStylePackage>(api.post(`api/v1/log/styles/${seg(id)}/versions`, { baseVersion, package: pkg }));

export const importLogStyleZip = (file: File) => {
  const form = new FormData();
  form.append('file', file);
  return unwrap<{ stylePackageId: string; version: string; warnings: string[] }>(
    api.post('api/v1/log/styles/import', form, { timeout: 60000 }),
  );
};

/** 导出接口返回 ZIP 原始字节，不使用 JSON 包裹。 */
export const exportLogStyleZip = async (id: string, version: string): Promise<Blob> => {
  try {
    const resp = await api.get(`api/v1/log/styles/${seg(id)}/versions/${seg(version)}/export`, { responseType: 'blob', timeout: 60000 });
    return resp.data as Blob;
  } catch (error) {
    if (isAxiosError(error) && error.response?.data instanceof Blob) {
      try {
        const parsed = JSON.parse(await error.response.data.text());
        if (parsed?.error?.code) {
          throw new LogDomainError(parsed.error.code, parsed.error.message || '导出失败', error.response.status);
        }
      } catch (inner) {
        if (inner instanceof LogDomainError) throw inner;
      }
    }
    throw toLogError(error);
  }
};

/** 设计态资源：样式包资源与聊天附件都经已认证接口取字节，交给隔离 iframe。 */
export const fetchLogStyleAssetBytes = async (id: string, version: string, assetId: string, signal?: AbortSignal) => {
  const resp = await api.get(`api/v1/log/styles/${seg(id)}/versions/${seg(version)}/assets/${seg(assetId)}`, { responseType: 'arraybuffer', signal, timeout: 30000 });
  return { data: resp.data as ArrayBuffer, mime: String(resp.headers['content-type'] || '') };
};

export const fetchAttachmentBytes = async (attachmentId: string, signal?: AbortSignal) => {
  const resp = await api.get(`api/v1/attachment/${seg(attachmentId)}?proxy=1`, { responseType: 'arraybuffer', signal, timeout: 30000 });
  return { data: resp.data as ArrayBuffer, mime: String(resp.headers['content-type'] || '') };
};

/** LOG 工程素材：经工程权限读取已保存草稿中登记的图片，不直接使用附件地址。 */
export const fetchLogProjectAssetBytes = async (projectId: string, assetId: string, signal?: AbortSignal) => {
  const resp = await api.get(`api/v1/log/projects/${seg(projectId)}/assets/${seg(assetId)}`, { responseType: 'arraybuffer', signal, timeout: 30000 });
  return { data: resp.data as ArrayBuffer, mime: String(resp.headers['content-type'] || '') };
};

export const fetchLogBodyAssetBytes = async (projectId: string, context: string, signal?: AbortSignal) => {
  const [sectionId, messageId, assetKey, extra] = context.split('/');
  if (extra !== undefined || !/^[A-Za-z0-9_-]{1,100}$/.test(sectionId || '') || !/^[A-Za-z0-9_-]{1,100}$/.test(messageId || '') || !/^[a-f0-9]{32}$/.test(assetKey || '')) {
    throw new LogDomainError('INVALID_INPUT', '资源标识无效');
  }
  const resp = await api.get(`api/v1/log/projects/${seg(projectId)}/sections/${seg(sectionId)}/messages/${seg(messageId)}/assets/${seg(assetKey)}`, { responseType: 'arraybuffer', signal, timeout: 30000 });
  return { data: resp.data as ArrayBuffer, mime: String(resp.headers['content-type'] || '') };
};

// ---- 消息领域任务读取（外观批量） ----

export const fetchMutationJob = (jobId: string) =>
  unwrap<LogMutationJob>(api.get(`api/v1/message-mutation-jobs/${seg(jobId)}`));

export const fetchMutationJobItems = (jobId: string, cursor: string | null, limit = 100) =>
  unwrap<{ items: LogMutationJobItem[]; nextCursor: string | null }>(
    api.get(`api/v1/message-mutation-jobs/${seg(jobId)}/items`, { params: cursor ? { cursor, limit } : { limit } }),
  );

// ---- 世界内频道与身份（只读，供来源与身份选择） ----

export interface LogIdentityOption {
  id: string;
  displayName: string;
  color: string;
  avatarAttachmentId: string;
  sharedIdentityId?: string;
  userId: string;
}
export interface LogIdentityVariantOption {
  id: string;
  identityId: string;
  label: string;
  displayName?: string;
  avatarAttachmentId?: string;
}

/** 显式频道的代管候选：只取首屏，更多用户通过服务端关键字搜索定位。 */
export const fetchIdentityManageCandidatesForLog = async (channelId: string, keyword = ''): Promise<ChannelIdentityManageCandidatesResponse> => {
  try {
    const normalizedKeyword = keyword.trim();
    const response = await api.get<ChannelIdentityManageCandidatesResponse>(`api/v1/channels/${seg(channelId)}/identity-manage-candidates`, {
      params: { page: 1, pageSize: 20, ...(normalizedKeyword ? { keyword: normalizedKeyword } : {}) },
    });
    return { ...response.data, items: response.data.items || [] };
  } catch (error) {
    throw toLogError(error);
  }
};

export const fetchChannelIdentitiesForLog = async (channelId: string, targetUserId?: string) => {
  try {
    const params: Record<string, string> = { channelId };
    if (targetUserId) params.targetUserId = targetUserId;
    const [identities, variants] = await Promise.all([
      api.get<{ items?: LogIdentityOption[] }>('api/v1/channel-identities', { params }),
      api.get<{ items?: Array<{ id: string; identityId: string; keyword?: string; note?: string; displayName?: string; selectorEmoji?: string; avatarAttachmentId?: string }> }>('api/v1/channel-identity-variants', { params }),
    ]);
    return {
      identities: (identities.data?.items || []).filter((item) => item && typeof item.id === 'string'),
      variants: (variants.data?.items || []).map((v) => ({
        id: v.id,
        identityId: v.identityId,
        displayName: v.displayName,
        avatarAttachmentId: v.avatarAttachmentId,
        label: [v.selectorEmoji, v.displayName || v.keyword || v.note || v.id].filter(Boolean).join(' '),
      })) as LogIdentityVariantOption[],
    };
  } catch (error) {
    throw toLogError(error);
  }
};

// ---- 匿名展示：独立客户端，不带登录凭据 ----

const publicClient = axiosFactory.create({
  baseURL: urlBase + '/',
  withCredentials: false,
  timeout: 15000,
  responseType: 'json',
});

const unwrapPublic = async <T>(request: Promise<AxiosResponse<{ data: T }>>): Promise<T> => {
  try {
    const resp = await request;
    return resp.data?.data as T;
  } catch (error) {
    throw toLogError(error);
  }
};

export const fetchPublicLog = (slug: string) =>
  unwrapPublic<LogPublicDocument>(publicClient.get(`api/v1/public/log/${seg(slug)}`));

export const fetchPublicLogItems = (slug: string, sectionId: string, cursor: string | null, limit = 100, signal?: AbortSignal) =>
  unwrapPublic<LogPublicItems>(publicClient.get(`api/v1/public/log/${seg(slug)}/sections/${seg(sectionId)}/items`, {
    params: cursor ? { cursor, limit } : { limit },
    signal,
  }));

export const fetchPublicLogContext = (slug: string, sectionId: string, nodeId: string, signal?: AbortSignal) =>
  unwrapPublic<LogPublicItems>(publicClient.get(`api/v1/public/log/${seg(slug)}/sections/${seg(sectionId)}/context/${seg(nodeId)}`, { signal }));

/** 章节模式发布版：按发布顺序读取一个视觉章节内全部来源片段的节点。 */
export const fetchPublicLogChapterItems = (slug: string, chapterId: string, cursor: string | null, limit = 100, signal?: AbortSignal) =>
  unwrapPublic<LogPublicItems>(publicClient.get(`api/v1/public/log/${seg(slug)}/chapters/${seg(chapterId)}/items`, {
    params: cursor ? { cursor, limit } : { limit },
    signal,
  }));

export const fetchPublicLogChapterContext = (slug: string, chapterId: string, nodeId: string, signal?: AbortSignal) =>
  unwrapPublic<LogPublicItems>(publicClient.get(`api/v1/public/log/${seg(slug)}/chapters/${seg(chapterId)}/context/${seg(nodeId)}`, { signal }));

export const searchPublicLog = (slug: string, q: string, cursor: string | null, limit = 20) =>
  unwrapPublic<{ items: LogPublicSearchHit[]; nextCursor: string | null }>(publicClient.get(`api/v1/public/log/${seg(slug)}/search`, {
    params: cursor ? { q, cursor, limit } : { q, limit },
  }));

/** 公开资源路径由后端生成（/api/v1/public/log/<slug>/assets/<key>）；只接受该前缀。 */
export const fetchPublicAssetBytes = async (path: string, signal?: AbortSignal) => {
  const normalized = path.replace(/^\/+/, '');
  if (!/^api\/v1\/public\/log\/[a-z0-9]{4,64}\/assets\/[a-f0-9]{32}$/.test(normalized)) {
    throw new LogDomainError('INVALID_INPUT', '资源地址无效');
  }
  const resp = await publicClient.get(normalized, { responseType: 'arraybuffer', signal, timeout: 30000 });
  return { data: resp.data as ArrayBuffer, mime: String(resp.headers['content-type'] || '') };
};

export const publicAssetPath = (slug: string, assetKey: string) => `/api/v1/public/log/${seg(slug)}/assets/${seg(assetKey)}`;
