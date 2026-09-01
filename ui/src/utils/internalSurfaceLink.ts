export const INTERNAL_SURFACE_TYPES = ['iform', 'note', 'character'] as const;

export type InternalSurfaceType = typeof INTERNAL_SURFACE_TYPES[number];

export interface InternalSurfaceLinkParams {
  type: InternalSurfaceType;
  id: string;
  worldId: string;
  channelId: string;
}

interface InternalSurfaceLinkConfig {
  domain?: string | null;
  webUrl?: string | null;
}

export const resolveInternalSurfaceLinkBase = (
  config?: InternalSurfaceLinkConfig | null,
): string | undefined => {
  const domain = config?.domain?.trim() || '';
  if (!domain) return undefined;
  let base = domain;
  if (!/^(https?:)?\/\//i.test(base)) {
    const protocol = typeof window === 'undefined' ? 'https:' : window.location.protocol;
    base = `${protocol}//${base}`;
  }
  const webUrl = config?.webUrl?.trim() || '';
  if (webUrl) {
    base = `${base.replace(/\/+$/, '')}/${webUrl.replace(/^\/+/, '')}`;
  }
  return base;
};

const resolveLinkBase = (base?: string): string => {
  const trimmed = (base || '').trim();
  if (trimmed) return trimmed.replace(/\/+$/, '');
  if (typeof window === 'undefined') return '';
  return window.location.href.split('#', 1)[0].replace(/\/+$/, '');
};

export function generateInternalSurfaceLink(
  params: InternalSurfaceLinkParams,
  options?: { base?: string },
): string {
  const base = resolveLinkBase(options?.base);
  const search = new URLSearchParams({
    world: params.worldId,
    channel: params.channelId,
  });
  return `${base}/#/internal/${encodeURIComponent(params.type)}/${encodeURIComponent(params.id)}?${search.toString()}`;
}

export function parseInternalSurfaceLink(value: string): InternalSurfaceLinkParams | null {
  if (!value || typeof value !== 'string') return null;
  const normalized = value.replace(/&amp;/gi, '&').trim();
  const hashIndex = normalized.indexOf('#');
  const hash = hashIndex >= 0 ? normalized.slice(hashIndex + 1) : normalized;
  const match = hash.match(/^\/internal\/([^/?#]+)\/([^/?#]+)\?([^#]+)$/);
  if (!match) return null;
  try {
    const type = decodeURIComponent(match[1] || '');
    if (!INTERNAL_SURFACE_TYPES.includes(type as InternalSurfaceType)) return null;
    const id = decodeURIComponent(match[2] || '').trim();
    const search = new URLSearchParams(match[3]);
    const worldId = (search.get('world') || '').trim();
    const channelId = (search.get('channel') || '').trim();
    if (!id || !worldId || !channelId) return null;
    return { type: type as InternalSurfaceType, id, worldId, channelId };
  } catch {
    return null;
  }
}
