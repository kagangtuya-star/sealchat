import { isMobileBrowserRuntime } from './windowFocusState';
import { resolvePublicLinkBase, type PublicLinkConfig } from './publicLinkBase';

export const INTERNAL_SURFACE_TYPES = ['iform', 'note', 'character', 'clue', 'clue-board', 'theater'] as const;

export type InternalSurfaceType = typeof INTERNAL_SURFACE_TYPES[number];

export interface InternalSurfaceLinkParams {
  type: InternalSurfaceType;
  id: string;
  worldId: string;
  channelId: string;
  /** 小剧场的局部展示选项，不参与资源身份。 */
  transparent?: boolean;
  full?: boolean;
}

export const buildInternalSurfaceResourceKey = (params: InternalSurfaceLinkParams): string => (
  `${params.type}:${params.id}:${params.worldId}:${params.channelId}`
);

export const resolveInternalSurfaceLinkBase = (
  config?: PublicLinkConfig | null,
): string | undefined => resolvePublicLinkBase(config);

export function generateInternalSurfaceLink(
  params: InternalSurfaceLinkParams,
  options?: { base?: string },
): string {
  const explicitBase = (options?.base || '').trim();
  let prefix = '';
  if (explicitBase) {
    prefix = `${explicitBase.replace(/\/+$/, '')}/#`;
  } else if (typeof window !== 'undefined') {
    const documentUrl = new URL(window.location.href.split('#', 1)[0]);
    if (window.self !== window.top) {
      documentUrl.searchParams.set(
        '__sc_surface',
        buildInternalSurfaceResourceKey(params),
      );
    }
    prefix = `${documentUrl.toString()}#`;
  }
  const search = new URLSearchParams({
    world: params.worldId,
    channel: params.channelId,
  });
  if (params.type === 'theater') {
    search.set('transparent', params.transparent === true ? '1' : '0');
    search.set('full', params.full === true ? '1' : '0');
  }
  return `${prefix}/internal/${encodeURIComponent(params.type)}/${encodeURIComponent(params.id)}?${search.toString()}`;
}

export interface InternalSurfacePopoutOptions {
  width?: number;
  height?: number;
}

const normalizePopupDimension = (value: number | undefined, fallback: number, min: number, max: number) => {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return fallback;
  return Math.min(max, Math.max(min, Math.round(numeric)));
};

export function openInternalSurfaceLink(
  link: string,
  options?: InternalSurfacePopoutOptions,
): Window | null {
  if (typeof window === 'undefined' || !link) return null;

  let opened: Window | null;
  if (isMobileBrowserRuntime()) {
    opened = window.open(link, '_blank');
  } else {
    const width = normalizePopupDimension(options?.width, 960, 320, 1920);
    const height = normalizePopupDimension(options?.height, 720, 240, 1200);
    const features = [
      'resizable=yes',
      'scrollbars=yes',
      `width=${width}`,
      `height=${height}`,
    ];
    if (window.screen?.availWidth && window.screen?.availHeight) {
      features.push(
        `left=${Math.max(0, Math.round((window.screen.availWidth - width) / 2))}`,
        `top=${Math.max(0, Math.round((window.screen.availHeight - height) / 2))}`,
      );
    }
    opened = window.open(link, '_blank', features.join(','));
  }

  if (opened) {
    try {
      opened.opener = null;
    } catch {
      // Cross-origin WindowProxy may reject opener assignment.
    }
  }
  return opened;
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
    return {
      type: type as InternalSurfaceType, id, worldId, channelId,
      ...(type === 'theater' ? { transparent: search.get('transparent') === '1', full: search.get('full') === '1' } : {}),
    };
  } catch {
    return null;
  }
}
