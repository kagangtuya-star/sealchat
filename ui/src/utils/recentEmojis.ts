import { api } from '@/stores/_config';
import { normalizeCustomEmojiValue } from '@/utils/emojiRender';

const STORAGE_KEY = 'sealchat:recent-emojis';
const PREF_KEY = 'recent_emojis_v1';
const MAX_RECENT = 16;
const DEFAULT_EMOJIS = ['👍', '❤️', '😆', '😮', '😢', '😠'];
const SAVE_DEBOUNCE_MS = 800;

export type RecentEmojiItem = {
  value: string;
  kind: 'unicode' | 'custom';
};

interface RecentEmojisPayloadV1 {
  emojis: RecentEmojiItem[];
  lastUpdated: number;
}

let cache: RecentEmojiItem[] | null = null;
let cacheUpdatedAt = 0;
let loaded = false;
let loadPromise: Promise<RecentEmojiItem[]> | null = null;
let saveTimer: number | null = null;
const listeners = new Set<() => void>();

const notify = () => {
  listeners.forEach((listener) => listener());
};

const normalizeItem = (input: unknown): RecentEmojiItem | null => {
  if (typeof input === 'string') {
    const normalized = normalizeCustomEmojiValue(input);
    if (!normalized) return null;
    return {
      value: normalized,
      kind: normalized.startsWith('id:') ? 'custom' : 'unicode',
    };
  }
  if (input && typeof input === 'object') {
    const value = typeof (input as any).value === 'string' ? normalizeCustomEmojiValue((input as any).value) : '';
    if (!value) return null;
    const kind = (input as any).kind === 'custom' || value.startsWith('id:') ? 'custom' : 'unicode';
    return { value, kind };
  }
  return null;
};

const trimAndDedup = (items: RecentEmojiItem[]) => {
  const seen = new Set<string>();
  const result: RecentEmojiItem[] = [];
  for (const item of items) {
    if (!item?.value) continue;
    if (seen.has(item.value)) continue;
    seen.add(item.value);
    result.push(item);
    if (result.length >= MAX_RECENT) break;
  }
  return result;
};

const parsePayload = (raw: string | null): { items: RecentEmojiItem[]; lastUpdated: number } => {
  if (!raw) return { items: [], lastUpdated: 0 };
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return {
        items: trimAndDedup(parsed.map(normalizeItem).filter((item): item is RecentEmojiItem => !!item)),
        lastUpdated: 0,
      };
    }
    if (parsed && typeof parsed === 'object') {
      const list = (parsed as RecentEmojisPayloadV1).emojis;
      if (Array.isArray(list)) {
        return {
          items: trimAndDedup(list.map(normalizeItem).filter((item): item is RecentEmojiItem => !!item)),
          lastUpdated: Number((parsed as RecentEmojisPayloadV1).lastUpdated) || 0,
        };
      }
    }
  } catch {
    // ignore parse error
  }
  return { items: [], lastUpdated: 0 };
};

const buildPayload = (items: RecentEmojiItem[], updatedAt = Date.now()): RecentEmojisPayloadV1 => ({
  emojis: items,
  lastUpdated: updatedAt,
});

const saveLocal = (items: RecentEmojiItem[], updatedAt?: number) => {
  try {
    const payload = buildPayload(items, updatedAt);
    cacheUpdatedAt = payload.lastUpdated;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch {
    // ignore storage errors
  }
};

const scheduleRemoteSave = (items: RecentEmojiItem[]) => {
  if (typeof window === 'undefined') return;
  if (saveTimer) {
    window.clearTimeout(saveTimer);
  }
  saveTimer = window.setTimeout(async () => {
    const payload = JSON.stringify(buildPayload(items));
    try {
      await api.post('api/v1/user/preferences', { key: PREF_KEY, value: payload });
    } catch {
      // ignore sync errors
    }
  }, SAVE_DEBOUNCE_MS);
};

const setCache = (items: RecentEmojiItem[], updatedAt: number) => {
  cache = items;
  cacheUpdatedAt = updatedAt;
  return cache;
};

const ensureCache = () => {
  if (cache) return cache;
  const stored = parsePayload(localStorage.getItem(STORAGE_KEY));
  const initialItems = stored.items.length
    ? stored.items
    : DEFAULT_EMOJIS.map((value) => ({ value, kind: 'unicode' as const }));
  cache = initialItems;
  cacheUpdatedAt = stored.lastUpdated;
  return cache;
};

export const subscribeRecentEmojis = (listener: () => void) => {
  listeners.add(listener);
  return () => listeners.delete(listener);
};

export const loadRecentEmojis = async (force = false): Promise<RecentEmojiItem[]> => {
  if (loaded && !force) {
    return ensureCache();
  }
  if (loadPromise) {
    return loadPromise;
  }
  loadPromise = (async () => {
    const localState = parsePayload(localStorage.getItem(STORAGE_KEY));
    const localItems = localState.items;
    const fallbackItems = localItems.length ? localItems : ensureCache();
    try {
      const resp = await api.get<{ key: string; value: string; exists: boolean }>('api/v1/user/preferences', {
        params: { key: PREF_KEY },
      });
      const remoteState = resp.data?.exists ? parsePayload(resp.data?.value || '') : { items: [], lastUpdated: 0 };
      if (remoteState.items.length > 0 || localItems.length > 0) {
        const pickRemote = remoteState.items.length > 0 && remoteState.lastUpdated >= localState.lastUpdated;
        const selected = pickRemote
          ? remoteState
          : { items: localItems.length ? localItems : fallbackItems, lastUpdated: localState.lastUpdated || cacheUpdatedAt };
        setCache(selected.items, selected.lastUpdated);
        saveLocal(selected.items, selected.lastUpdated);
        if (!pickRemote && localItems.length > 0) {
          scheduleRemoteSave(selected.items);
        }
        loaded = true;
        notify();
        return selected.items;
      }
    } catch {
      // ignore remote errors
    }
    loaded = true;
    return ensureCache();
  })();
  const result = await loadPromise;
  loadPromise = null;
  return result;
};

export const getRecentEmojis = (): RecentEmojiItem[] => {
  return ensureCache();
};

export const addRecentEmoji = (value: string): void => {
  const item = normalizeItem(value);
  if (!item) return;
  const current = ensureCache();
  const filtered = current.filter((entry) => entry.value !== item.value);
  const updated = trimAndDedup([item, ...filtered]);
  cache = updated;
  saveLocal(updated);
  scheduleRemoteSave(updated);
  notify();
};

export const getQuickEmojis = (): RecentEmojiItem[] => {
  return getRecentEmojis().slice(0, 6);
};
