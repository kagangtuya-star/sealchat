import type { ChannelIdentityVariant, GalleryItem } from '@/types';

// 输入框上方上下文快捷提示：只做纯字符串匹配，供 typing hot path 调用。

export const COMPOSER_HINT_WINDOW_BEFORE = 64;
export const COMPOSER_HINT_WINDOW_AFTER = 32;
export const EMOJI_INPUT_HINT_LIMIT = 6;
export const VARIANT_INPUT_HINT_LIMIT = 5;
// note 只作为辅助匹配，过长的说明文字不参与，避免偶然命中
const VARIANT_NOTE_MAX_LENGTH = 8;
const INLINE_MARKER_PATTERN = /\[\[[^\]]*\]\]/g;

export interface EmojiHintIndexEntry {
  id: string;
  item: GalleryItem;
  keyword: string;
}

interface HintWindow {
  text: string;
  cursor: number;
}

export const buildEmojiHintIndex = (items: readonly GalleryItem[]): EmojiHintIndexEntry[] => {
  const seen = new Set<string>();
  const index: EmojiHintIndexEntry[] = [];
  for (const item of items) {
    const keyword = item?.remark?.trim();
    if (!keyword || !item.id || !item.attachmentId || seen.has(item.id)) continue;
    seen.add(item.id);
    index.push({ id: item.id, item, keyword });
  }
  return index;
};

const resolveHintWindow = (text: string, cursor?: number | null): HintWindow | null => {
  if (!text) return null;
  const length = text.length;
  const pos = typeof cursor === 'number' && Number.isFinite(cursor)
    ? Math.max(0, Math.min(length, Math.floor(cursor)))
    : length;
  const start = Math.max(0, pos - COMPOSER_HINT_WINDOW_BEFORE);
  const end = Math.min(length, pos + COMPOSER_HINT_WINDOW_AFTER);
  // 先遮蔽完整内联标记再取窗口，避免跨窗口边界的 [[图片:xxx]] 被误命中
  const slice = text.replace(INLINE_MARKER_PATTERN, (match) => ' '.repeat(match.length)).slice(start, end);
  if (!slice.trim()) return null;
  return { text: slice, cursor: pos - start };
};

// 返回关键词在窗口内距光标最近的一次出现的距离；未命中返回 -1
const nearestDistance = (scope: HintWindow, keyword: string): number => {
  let best = -1;
  let from = 0;
  while (from <= scope.text.length - keyword.length) {
    const start = scope.text.indexOf(keyword, from);
    if (start < 0) break;
    const end = start + keyword.length;
    const distance = scope.cursor < start ? start - scope.cursor : scope.cursor > end ? scope.cursor - end : 0;
    if (best < 0 || distance < best) best = distance;
    if (best === 0 || start >= scope.cursor) break;
    from = start + 1;
  }
  return best;
};

export const resolveEmojiInputHints = (options: {
  text: string;
  cursor?: number | null;
  index: readonly EmojiHintIndexEntry[];
  usageMap?: Record<string, number> | null;
  limit?: number;
}): GalleryItem[] => {
  const limit = options.limit ?? EMOJI_INPUT_HINT_LIMIT;
  if (limit <= 0 || !options.index.length) return [];
  const scope = resolveHintWindow(options.text, options.cursor);
  if (!scope) return [];
  const usageMap = options.usageMap || {};
  const matches: Array<{ entry: EmojiHintIndexEntry; distance: number; usedAt: number }> = [];
  const seen = new Set<string>();
  for (const entry of options.index) {
    if (seen.has(entry.id) || entry.keyword.length > scope.text.length) continue;
    const distance = nearestDistance(scope, entry.keyword);
    if (distance < 0) continue;
    seen.add(entry.id);
    matches.push({ entry, distance, usedAt: usageMap[entry.id] || 0 });
  }
  matches.sort((a, b) => (
    a.distance - b.distance
    || b.usedAt - a.usedAt
    || b.entry.keyword.length - a.entry.keyword.length
  ));
  return matches.slice(0, limit).map((match) => match.entry.item);
};

export const resolveIdentityVariantHints = (options: {
  text: string;
  cursor?: number | null;
  variants: readonly ChannelIdentityVariant[];
  limit?: number;
}): ChannelIdentityVariant[] => {
  const limit = options.limit ?? VARIANT_INPUT_HINT_LIMIT;
  if (limit <= 0 || !options.variants.length) return [];
  const scope = resolveHintWindow(options.text, options.cursor);
  if (!scope) return [];
  const matches: Array<{ variant: ChannelIdentityVariant; tier: number; distance: number; length: number; order: number }> = [];
  options.variants.forEach((variant, order) => {
    if (!variant?.id || variant.enabled === false) return;
    const candidates: Array<[string, number]> = [
      [String(variant.keyword || '').trim(), 0],
      [String(variant.displayName || '').trim(), 1],
    ];
    const note = String(variant.note || '').trim();
    if (note.length <= VARIANT_NOTE_MAX_LENGTH) candidates.push([note, 2]);
    for (const [keyword, tier] of candidates) {
      if (!keyword) continue;
      const distance = nearestDistance(scope, keyword);
      if (distance < 0) continue;
      matches.push({ variant, tier, distance, length: keyword.length, order });
      return;
    }
  });
  matches.sort((a, b) => (
    a.tier - b.tier
    || a.distance - b.distance
    || b.length - a.length
    || a.order - b.order
  ));
  return matches.slice(0, limit).map((match) => match.variant);
};
