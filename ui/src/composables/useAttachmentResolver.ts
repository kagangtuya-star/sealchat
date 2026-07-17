import { reactive } from 'vue';
import { api, urlBase } from '@/stores/_config';

export interface AttachmentMeta {
  id: string;
  filename?: string;
  size?: number;
  hash?: string;
  mimeType?: string;
  isAnimated?: boolean;
  storageType?: string;
  objectKey?: string;
  externalUrl?: string;
  publicUrl?: string;
}

const attachmentMetaStore = reactive<Record<string, AttachmentMeta>>({});
const attachmentUrlStore = reactive<Record<string, string>>({});
const pendingMetaFetch = new Set<string>();

const ATTACHMENT_PATH_PATTERN = /(.*?api\/v1\/attachment\/)([^/?#]+)(.*)/i;

const stripAttachmentPathIdPrefix = (value: string) => {
  const match = value.match(ATTACHMENT_PATH_PATTERN);
  if (!match) {
    return value;
  }
  const [, prefix, rawId, suffix] = match;
  if (!rawId.startsWith('id:')) {
    return value;
  }
  return `${prefix}${rawId.slice(3)}${suffix}`;
};

export const normalizeAttachmentId = (value: string) => {
  const raw = (value || '').trim();
  if (!raw) return '';
  if (raw.startsWith('id:')) {
    return raw.slice(3);
  }
  const sanitizedPath = stripAttachmentPathIdPrefix(raw);
  const match = sanitizedPath.match(ATTACHMENT_PATH_PATTERN);
  if (match) {
    return match[2];
  }
  return raw;
};

const ensureAttachmentMeta = async (normalized: string) => {
  if (!normalized || pendingMetaFetch.has(normalized) || attachmentMetaStore[normalized]) {
    return;
  }
  pendingMetaFetch.add(normalized);
  try {
    const resp = await api.get<{ item: AttachmentMeta }>(`api/v1/attachment/${normalized}/meta`);
    const meta = resp.data?.item;
    if (meta) {
      attachmentMetaStore[normalized] = meta;
      const external = meta.externalUrl || meta.publicUrl;
      if (external && /^(https?:|blob:|data:|\/\/|\/)/i.test(external)) {
        attachmentUrlStore[normalized] = external;
      }
    }
  } catch (error) {
    console.warn('获取附件信息失败', error);
  } finally {
    pendingMetaFetch.delete(normalized);
  }
};

export const fetchAttachmentMetaById = async (attachmentId: string): Promise<AttachmentMeta | null> => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized) {
    return null;
  }
  if (attachmentMetaStore[normalized]) {
    return attachmentMetaStore[normalized];
  }
  await ensureAttachmentMeta(normalized);
  return attachmentMetaStore[normalized] || null;
};

export const fetchAttachmentFileById = async (attachmentId: string, fallbackName?: string): Promise<File | null> => {
  const normalized = normalizeAttachmentId(attachmentId);
  if (!normalized) {
    return null;
  }

  const meta = await fetchAttachmentMetaById(normalized);
  const resp = await api.get<Blob>(`api/v1/attachment/${normalized}`, {
    responseType: 'blob',
  });
  const blob = resp.data;
  if (!blob) {
    return null;
  }

  const fileName = String(meta?.filename || fallbackName || `attachment-${normalized}`).trim() || `attachment-${normalized}`;
  const fileType = blob.type || meta?.mimeType || 'application/octet-stream';
  return new File([blob], fileName, {
    type: fileType,
    lastModified: Date.now(),
  });
};

export const resolveAttachmentUrl = (value?: string) => {
  const raw = stripAttachmentPathIdPrefix((value || '').trim());
  if (!raw) {
    return '';
  }
  if (/^(https?:|blob:|data:|\/\/)/i.test(raw)) {
    return raw;
  }
  if (raw.startsWith('/')) {
    return `${urlBase}${raw}`;
  }
  if (raw.includes('/')) {
    return `${urlBase}/${raw}`;
  }
  const normalized = normalizeAttachmentId(raw);
  if (!normalized) {
    return '';
  }
  const cached = attachmentUrlStore[normalized];
  if (cached && /^(https?:|blob:|data:|\/\/|\/)/i.test(cached)) {
    return cached;
  }
  void ensureAttachmentMeta(normalized);
  return `${urlBase}/api/v1/attachment/${normalized}`;
};
