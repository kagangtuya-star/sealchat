import twemoji from '@twemoji/api';

const CDN_BASE =
  import.meta.env.VITE_TWEMOJI_CDN_BASE ||
  'https://cdn.jsdelivr.net/gh/twitter/twemoji@master/assets/';
const FAILURE_LIMIT = 2;
const failureCounts = new Map<string, number>();
const textFallbackEmojis = new Set<string>();

function normalizeUrl(url: string): string {
  if (typeof window === 'undefined') return url;
  try {
    return new URL(url, window.location.href).toString();
  } catch {
    return url;
  }
}

export function noteEmojiLoadFailure(url?: string | null, emoji?: string | null): boolean {
  if (!url) return false;
  const normalized = normalizeUrl(url);
  const nextCount = (failureCounts.get(normalized) || 0) + 1;
  failureCounts.set(normalized, nextCount);
  const blocked = nextCount >= FAILURE_LIMIT;
  if (blocked && emoji) {
    textFallbackEmojis.add(emoji);
  }
  return blocked;
}

function getBaseUrl(): string {
  return CDN_BASE;
}

export function getEmojiUrl(emoji: string): string {
  const codePoint = twemoji.convert.toCodePoint(emoji);
  return `${getBaseUrl()}svg/${codePoint}.svg`;
}

export function getFallbackUrl(emoji: string): string {
  return '';
}

export function parseEmoji(text: string): string {
  return twemoji.parse(text, {
    folder: 'svg',
    ext: '.svg',
    base: getBaseUrl(),
  });
}

export const twemojiClass = 'twemoji-img';

export function shouldUseEmojiText(emoji: string): boolean {
  return textFallbackEmojis.has(emoji);
}
