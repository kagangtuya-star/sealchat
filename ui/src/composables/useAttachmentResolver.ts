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

export const normalizeAttachmentId = (value: string) => {
  if (!value) return '';
  return value.startsWith('id:') ? value.slice(3) : value;
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
      if (external) {
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

export const resolveAttachmentUrl = (value?: string) => {
  const raw = (value || '').trim();
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
  if (cached) {
    return cached;
  }
  void ensureAttachmentMeta(normalized);
  return `${urlBase}/api/v1/attachment/${normalized}`;
};
