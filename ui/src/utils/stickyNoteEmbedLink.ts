export interface StickyNoteEmbedLinkParams {
  worldId: string;
  channelId: string;
  noteId: string;
}

export interface ParsedSingleStickyNoteEmbedLink extends StickyNoteEmbedLinkParams {
  rawLink: string;
}

const STICKY_NOTE_LINK_EXACT_REGEX = /^https?:\/\/[^\s<>"']*#\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\?([^\s#]+)$/;

const normalizeInput = (value: string) => value.replace(/&amp;/gi, '&').trim();

const resolveLinkBase = (base?: string): string => {
  const trimmed = (base || '').trim();
  if (trimmed) {
    return trimmed.replace(/\/+$/, '');
  }
  if (typeof window === 'undefined') {
    return '';
  }
  return window.location.origin;
};

export function generateStickyNoteEmbedLink(
  params: StickyNoteEmbedLinkParams,
  options?: { base?: string },
): string {
  const base = resolveLinkBase(options?.base);
  const search = new URLSearchParams({
    snote: params.noteId,
  });
  return `${base}/#/${params.worldId}/${params.channelId}?${search.toString()}`;
}

export function parseStickyNoteEmbedLink(url: string): StickyNoteEmbedLinkParams | null {
  if (!url || typeof url !== 'string') {
    return null;
  }
  const normalized = normalizeInput(url);
  const match = normalized.match(STICKY_NOTE_LINK_EXACT_REGEX);
  if (!match) {
    return null;
  }
  const [, worldId, channelId, queryString] = match;
  if (!worldId || !channelId || !queryString) {
    return null;
  }
  const search = new URLSearchParams(queryString);
  const noteId = (search.get('snote') || '').trim();
  if (!noteId) {
    return null;
  }
  return {
    worldId,
    channelId,
    noteId,
  };
}

export function isStickyNoteEmbedLink(url: string): boolean {
  return parseStickyNoteEmbedLink(url) !== null;
}

export function parseSingleStickyNoteEmbedLinkText(text: string): ParsedSingleStickyNoteEmbedLink | null {
  if (!text || typeof text !== 'string') {
    return null;
  }
  const normalized = normalizeInput(text).replace(/\u00a0/g, ' ').trim();
  if (!normalized || /\s/.test(normalized)) {
    return null;
  }
  const parsed = parseStickyNoteEmbedLink(normalized);
  if (!parsed) {
    return null;
  }
  return {
    ...parsed,
    rawLink: normalized,
  };
}
