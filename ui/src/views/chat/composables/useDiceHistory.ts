import { ref, computed } from 'vue';

export type DiceHistoryItem = {
  id: string;
  expr: string;
  favorite: boolean;
  timestamp: number;
};

export const HISTORY_KEY = 'sealchat:dice-history';
export const HISTORY_DISPLAY_LIMIT = 4;
export const HISTORY_STORAGE_LIMIT = 12;

const historyItems = ref<DiceHistoryItem[]>([]);
let initialized = false;

const sortByTimestampDesc = (a: DiceHistoryItem, b: DiceHistoryItem) => b.timestamp - a.timestamp;
const isClient = typeof window !== 'undefined';

const persistHistory = () => {
  if (!isClient) return;
  try {
    window.localStorage.setItem(HISTORY_KEY, JSON.stringify(historyItems.value));
  } catch (error) {
    console.warn('无法保存骰子历史', error);
  }
};

const loadHistory = () => {
  if (!isClient || initialized) {
    return;
  }
  initialized = true;
  try {
    const raw = window.localStorage.getItem(HISTORY_KEY);
    if (!raw) return;
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return;
    historyItems.value = parsed
      .filter((item: any) => typeof item?.expr === 'string')
      .map((item: any): DiceHistoryItem => ({
        id: item.id || `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        expr: item.expr,
        favorite: !!item.favorite,
        timestamp: typeof item.timestamp === 'number' ? item.timestamp : Date.now(),
      }))
      .sort(sortByTimestampDesc);
  } catch (error) {
    console.warn('无法加载骰子历史', error);
  }
};

const pruneHistory = () => {
  const favorites = historyItems.value.filter((item) => item.favorite).sort(sortByTimestampDesc);
  const nonFavorites = historyItems.value.filter((item) => !item.favorite).sort(sortByTimestampDesc);
  const allowedNonFavorites = Math.max(0, HISTORY_STORAGE_LIMIT - favorites.length);
  const keptNonFavorites = nonFavorites.slice(0, allowedNonFavorites);
  historyItems.value = [...favorites, ...keptNonFavorites].sort(sortByTimestampDesc);
};

const recordHistory = (expr: string) => {
  if (!expr) return;
  loadHistory();
  const trimmed = expr.trim();
  if (!trimmed) return;
  const existingIndex = historyItems.value.findIndex((item) => item.expr === trimmed);
  const favorite = existingIndex !== -1 ? historyItems.value[existingIndex].favorite : false;
  if (existingIndex !== -1) {
    historyItems.value.splice(existingIndex, 1);
  }
  const newItem: DiceHistoryItem = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    expr: trimmed,
    favorite,
    timestamp: Date.now(),
  };
  historyItems.value = [newItem, ...historyItems.value];
  pruneHistory();
  persistHistory();
};

const toggleFavorite = (itemId: string) => {
  loadHistory();
  historyItems.value = historyItems.value.map((item) =>
    item.id === itemId ? { ...item, favorite: !item.favorite, timestamp: item.timestamp } : item,
  );
  pruneHistory();
  persistHistory();
};

const displayedHistory = computed(() => {
  const favorites = historyItems.value.filter((item) => item.favorite).sort(sortByTimestampDesc);
  const nonFavorites = historyItems.value.filter((item) => !item.favorite).sort(sortByTimestampDesc);
  if (!favorites.length) {
    return nonFavorites.slice(0, HISTORY_DISPLAY_LIMIT);
  }
  if (!nonFavorites.length) {
    return favorites.slice(0, HISTORY_DISPLAY_LIMIT);
  }
  const reservedRecentSlots = Math.min(1, nonFavorites.length);
  const maxFavorites = Math.max(0, HISTORY_DISPLAY_LIMIT - reservedRecentSlots);
  const limitedFavorites = favorites.slice(0, maxFavorites);
  const remainingSlots = Math.max(0, HISTORY_DISPLAY_LIMIT - limitedFavorites.length);
  const limitedRecents = nonFavorites.slice(0, remainingSlots);
  return [...limitedFavorites, ...limitedRecents];
});

const hasHistory = computed(() => displayedHistory.value.length > 0);

export const useDiceHistory = () => {
  loadHistory();
  return {
    historyItems,
    displayedHistory,
    hasHistory,
    recordHistory,
    toggleFavorite,
  };
};

export const recordDiceHistory = recordHistory;
