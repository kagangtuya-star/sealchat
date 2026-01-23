import { defineStore } from 'pinia';
import type { GalleryCollection, GalleryItem } from '@/types';
import {
  createCollection as apiCreateCollection,
  deleteCollection as apiDeleteCollection,
  deleteItems as apiDeleteItems,
  fetchCollections as apiFetchCollections,
  fetchItems as apiFetchItems,
  searchGallery as apiSearchGallery,
  updateCollection as apiUpdateCollection,
  updateItem as apiUpdateItem,
  uploadItems as apiUploadItems,
  addEmojiToFavorites as apiAddEmojiToFavorites,
  addEmojiToReactions as apiAddEmojiToReactions,
  type GalleryCollectionPayload,
  type GalleryItemUploadPayload
} from '@/models/gallery';

interface CollectionCacheMeta {
  loadedAt: number;
}

interface CollectionStateEntry {
  items: GalleryCollection[];
  meta: CollectionCacheMeta;
}

interface ItemStateEntry {
  items: GalleryItem[];
  page: number;
  pageSize: number;
  total: number;
  loading: boolean;
}

interface GalleryState {
  collections: Record<string, CollectionStateEntry>;
  items: Record<string, ItemStateEntry>;
  uploading: boolean;
  initializing: boolean;
  searchResult: GalleryItem[];
  searchCollections: Record<string, GalleryCollection>;
  searchKeyword: string;
  searchRequestSeq: number;
  panelVisible: boolean;
  activeOwner: { type: 'user'; id: string } | null;
  activeCollectionId: string | null;
  emojiCollectionIds: string[];
  favoritesCollectionId: string | null;
  reactionCollectionId: string | null;
  emojiRemarkVisible: boolean;
  activeEmojiTabId: string | null;
}

const STORAGE_EMOJI_COLLECTION = 'sealchat.gallery.emojiCollection';
const DEFAULT_EMOJI_REMARK_VISIBLE = true;

export const COLLECTION_TYPE_EMOJI = 'emoji_favorites';
export const COLLECTION_TYPE_EMOJI_REACTION = 'emoji_reactions';

interface EmojiPreferencePayload {
  emojiCollectionIds: string[];
  emojiRemarkVisible: boolean;
  activeEmojiTabId: string | null;
}

function normalizeEmojiPreferencePayload(stored: string | null): EmojiPreferencePayload {
  if (!stored) {
    return { emojiCollectionIds: [], emojiRemarkVisible: DEFAULT_EMOJI_REMARK_VISIBLE, activeEmojiTabId: null };
  }
  try {
    const parsed = JSON.parse(stored);
    if (parsed && typeof parsed === 'object') {
      let emojiCollectionIds: string[] = [];
      if (Array.isArray(parsed.emojiCollectionIds)) {
        emojiCollectionIds = parsed.emojiCollectionIds.filter((id: unknown) => typeof id === 'string');
      } else if (typeof parsed.emojiCollectionId === 'string') {
        emojiCollectionIds = [parsed.emojiCollectionId];
      }
      const emojiRemarkVisible = typeof parsed.emojiRemarkVisible === 'boolean'
        ? parsed.emojiRemarkVisible
        : DEFAULT_EMOJI_REMARK_VISIBLE;
      const activeEmojiTabId = typeof parsed.activeEmojiTabId === 'string' ? parsed.activeEmojiTabId : null;
      return { emojiCollectionIds, emojiRemarkVisible, activeEmojiTabId };
    }
  } catch {
    // ignore and fallback to legacy string payload
  }
  return { emojiCollectionIds: [stored], emojiRemarkVisible: DEFAULT_EMOJI_REMARK_VISIBLE, activeEmojiTabId: null };
}

function ownerKey(ownerId: string) {
  return `user:${ownerId}`;
}

export const useGalleryStore = defineStore('gallery', {
  state: (): GalleryState => ({
    collections: {},
    items: {},
    uploading: false,
    initializing: false,
    searchResult: [],
    searchCollections: {},
    searchKeyword: '',
    searchRequestSeq: 0,
    panelVisible: false,
    activeOwner: null,
    activeCollectionId: null,
    emojiCollectionIds: [],
    favoritesCollectionId: null,
    reactionCollectionId: null,
    emojiRemarkVisible: DEFAULT_EMOJI_REMARK_VISIBLE,
    activeEmojiTabId: null
  }),

  getters: {
    getCollections: (state) => (ownerId: string) => {
      const key = ownerKey(ownerId);
      return state.collections[key]?.items ?? [];
    },
    getCollectionMeta: (state) => (ownerId: string) => {
      const key = ownerKey(ownerId);
      return state.collections[key]?.meta;
    },
    getItemsByCollection: (state) => (collectionId: string) => state.items[collectionId]?.items ?? [],
    getItemPagination: (state) => (collectionId: string) => {
      const entry = state.items[collectionId];
      if (!entry) return { page: 1, pageSize: 40, total: 0 };
      const { page, pageSize, total } = entry;
      return { page, pageSize, total };
    },
    isCollectionLoading: (state) => (collectionId: string) => state.items[collectionId]?.loading ?? false,
    isPanelVisible: (state) => state.panelVisible,
    isInitializing: (state) => state.initializing,
    emojiItems(state): GalleryItem[] {
      const result = new Map<string, GalleryItem>();
      const addItems = (collectionId: string | null) => {
        if (!collectionId) return;
        const items = state.items[collectionId]?.items;
        if (items) {
          for (const item of items) {
            result.set(item.id, item);
          }
        }
      };
      addItems(state.favoritesCollectionId);
      for (const id of state.emojiCollectionIds) {
        addItems(id);
      }
      return Array.from(result.values());
    },
    reactionEmojiItems(state): GalleryItem[] {
      if (!state.reactionCollectionId) return [];
      return state.items[state.reactionCollectionId]?.items ?? [];
    },
    allEmojiCollectionIds(state): string[] {
      const ids: string[] = [];
      if (state.favoritesCollectionId) {
        ids.push(state.favoritesCollectionId);
      }
      for (const id of state.emojiCollectionIds) {
        if (!ids.includes(id)) {
          ids.push(id);
        }
      }
      return ids;
    }
  },

  actions: {
    loadEmojiPreference(userId: string) {
      const key = `${STORAGE_EMOJI_COLLECTION}:${userId}`;
      const stored = localStorage.getItem(key);
      const preference = normalizeEmojiPreferencePayload(stored);
      this.emojiCollectionIds = preference.emojiCollectionIds;
      this.emojiRemarkVisible = preference.emojiRemarkVisible;
      this.activeEmojiTabId = preference.activeEmojiTabId;
    },

    persistEmojiPreference(userId: string) {
      const key = `${STORAGE_EMOJI_COLLECTION}:${userId}`;
      const payload: EmojiPreferencePayload = {
        emojiCollectionIds: this.emojiCollectionIds,
        emojiRemarkVisible: this.emojiRemarkVisible,
        activeEmojiTabId: this.activeEmojiTabId
      };
      localStorage.setItem(key, JSON.stringify(payload));
    },

    setActiveEmojiTab(tabId: string | null, userId: string) {
      this.activeEmojiTabId = tabId;
      this.persistEmojiPreference(userId);
    },

    setEmojiRemarkVisible(visible: boolean, userId: string) {
      this.emojiRemarkVisible = visible;
      this.persistEmojiPreference(userId);
    },

    async openPanel(ownerId: string) {
      this.activeOwner = { type: 'user', id: ownerId };
      this.panelVisible = true;
      this.initializing = true;
      try {
        // Force reload collections to ensure fresh data
        const collections = await this.loadCollections(ownerId, true);
        if (!collections.length) {
          this.activeCollectionId = null;
          return;
        }
        if (!this.activeCollectionId || !collections.some((col) => col.id === this.activeCollectionId)) {
          this.activeCollectionId = collections[0].id;
        }
        if (this.activeCollectionId) {
          await this.loadItems(this.activeCollectionId);
        }
      } finally {
        this.initializing = false;
      }
      // Load emoji collections in background
      for (const id of this.emojiCollectionIds) {
        if (id !== this.activeCollectionId) {
          void this.loadItems(id).catch(() => {});
        }
      }
    },

    closePanel() {
      this.panelVisible = false;
    },

    async setActiveCollection(collectionId: string | null) {
      this.activeCollectionId = collectionId;
      if (collectionId) {
        await this.loadItems(collectionId);
      }
    },

    linkEmojiCollection(collectionId: string, userId: string, link: boolean) {
      if (collectionId === this.reactionCollectionId) {
        return;
      }
      if (link) {
        if (!this.emojiCollectionIds.includes(collectionId)) {
          this.emojiCollectionIds.push(collectionId);
          void this.loadItems(collectionId);
        }
      } else {
        this.emojiCollectionIds = this.emojiCollectionIds.filter(id => id !== collectionId);
      }
      this.persistEmojiPreference(userId);
    },

    async ensureEmojiCollection(ownerId: string): Promise<string | null> {
      const collections = await this.loadCollections(ownerId, true);
      const existing = collections.find(c => c.collectionType === COLLECTION_TYPE_EMOJI);
      const reactionCollection = collections.find(c => c.collectionType === COLLECTION_TYPE_EMOJI_REACTION);

      if (existing) {
        this.favoritesCollectionId = existing.id;
        await this.loadItems(existing.id);
      }
      if (reactionCollection) {
        this.reactionCollectionId = reactionCollection.id;
        await this.loadItems(reactionCollection.id);
      }
      if (this.reactionCollectionId && this.emojiCollectionIds.includes(this.reactionCollectionId)) {
        this.emojiCollectionIds = this.emojiCollectionIds.filter(id => id !== this.reactionCollectionId);
        this.persistEmojiPreference(ownerId);
      }

      // Load linked collections in background
      for (const id of this.emojiCollectionIds) {
        void this.loadItems(id).catch(() => {});
      }

      return existing?.id ?? null;
    },

    async addEmoji(attachmentId: string, ownerId: string): Promise<void> {
      const resp = await apiAddEmojiToFavorites(attachmentId);
      const item = resp.data.item;
      // Refresh emoji collection data
      const collections = await this.loadCollections(ownerId, true);
      const emojiCol = collections.find(c => c.collectionType === COLLECTION_TYPE_EMOJI);
      if (emojiCol) {
        this.favoritesCollectionId = emojiCol.id;
        this.upsertItems(emojiCol.id, [item]);
      }
    },

    async addReactionEmoji(attachmentId: string, ownerId: string): Promise<void> {
      const resp = await apiAddEmojiToReactions(attachmentId);
      const item = resp.data.item;
      const collections = await this.loadCollections(ownerId, true);
      const reactionCol = collections.find(c => c.collectionType === COLLECTION_TYPE_EMOJI_REACTION);
      if (reactionCol) {
        this.reactionCollectionId = reactionCol.id;
        this.upsertItems(reactionCol.id, [item]);
      }
    },

    async loadCollections(ownerId: string, force = false) {
      const key = ownerKey(ownerId);
      const cache = this.collections[key];
      if (!force && cache && Date.now() - cache.meta.loadedAt < 60_000) {
        return cache.items;
      }
      const resp = await apiFetchCollections('user', ownerId);
      this.collections[key] = {
        items: resp.data.items,
        meta: { loadedAt: Date.now() }
      };
      return resp.data.items;
    },

    async createCollection(ownerId: string, payload: Omit<GalleryCollectionPayload, 'ownerType' | 'ownerId'>) {
      const resp = await apiCreateCollection({ ownerType: 'user', ownerId, ...payload });
      const key = ownerKey(ownerId);
      if (!this.collections[key]) {
        this.collections[key] = {
          items: [],
          meta: { loadedAt: Date.now() }
        };
      }
      this.collections[key].items.push(resp.data.item);
      return resp.data.item;
    },

    async updateCollection(ownerId: string, collectionId: string, payload: Partial<GalleryCollectionPayload>) {
      const resp = await apiUpdateCollection(collectionId, payload);
      const key = ownerKey(ownerId);
      const cache = this.collections[key];
      if (cache) {
        const idx = cache.items.findIndex((col) => col.id === collectionId);
        if (idx >= 0) {
          cache.items[idx] = resp.data.item;
        }
      }
      return resp.data.item;
    },

    async deleteCollection(ownerId: string, collectionId: string) {
      await apiDeleteCollection(collectionId);
      const key = ownerKey(ownerId);
      const cache = this.collections[key];
      if (cache) {
        cache.items = cache.items.filter((col) => col.id !== collectionId);
      }
      delete this.items[collectionId];
      if (this.activeCollectionId === collectionId) {
        const newActiveId = cache?.items?.[0]?.id ?? null;
        this.activeCollectionId = newActiveId;
        if (newActiveId) {
          void this.loadItems(newActiveId);
        }
      }
      if (this.emojiCollectionIds.includes(collectionId)) {
        this.emojiCollectionIds = this.emojiCollectionIds.filter(id => id !== collectionId);
      }
      if (this.favoritesCollectionId === collectionId) {
        this.favoritesCollectionId = null;
      }
      if (this.reactionCollectionId === collectionId) {
        this.reactionCollectionId = null;
      }
      this.persistEmojiPreference(ownerId);
    },

    async loadItems(collectionId: string, params: { page?: number; pageSize?: number; keyword?: string } = {}) {
      const entry = this.items[collectionId] ?? {
        items: [],
        page: params.page ?? 1,
        pageSize: params.pageSize ?? 40,
        total: 0,
        loading: false
      };
      entry.loading = true;
      this.items[collectionId] = entry;

      try {
        const resp = await apiFetchItems(collectionId, params);
        entry.items = resp.data.items;
        entry.page = resp.data.page;
        entry.pageSize = resp.data.pageSize;
        entry.total = resp.data.total;
        return resp.data.items;
      } finally {
        entry.loading = false;
      }
    },

    upsertItems(collectionId: string, items: GalleryItem[]) {
      const entry = this.items[collectionId] ?? {
        items: [],
        page: 1,
        pageSize: 40,
        total: 0,
        loading: false
      };
      const map = new Map(entry.items.map((item) => [item.id, item] as const));
      for (const item of items) {
        map.set(item.id, item);
      }
      entry.items = Array.from(map.values());
      entry.total = Math.max(entry.total, entry.items.length);
      this.items[collectionId] = entry;
    },

    removeItems(collectionId: string, ids: string[]) {
      const entry = this.items[collectionId];
      if (!entry) return;
      const idSet = new Set(ids);
      entry.items = entry.items.filter((item) => !idSet.has(item.id));
      entry.total = Math.max(0, entry.total - ids.length);
    },

    async upload(collectionId: string, payload: GalleryItemUploadPayload) {
      this.uploading = true;
      try {
        const resp = await apiUploadItems(payload);
        this.upsertItems(collectionId, resp.data.items);
        return resp.data.items;
      } finally {
        this.uploading = false;
      }
    },

    async updateItem(collectionId: string, itemId: string, payload: Partial<{ remark: string; collectionId: string; order: number }>) {
      const resp = await apiUpdateItem(itemId, payload);
      const item = resp.data.item;
      if (payload.collectionId && payload.collectionId !== collectionId) {
        this.removeItems(collectionId, [itemId]);
        this.upsertItems(payload.collectionId, [item]);
      } else {
        this.upsertItems(collectionId, [item]);
      }
      return item;
    },

    async deleteItems(collectionId: string, ids: string[]) {
      await apiDeleteItems(ids);
      this.removeItems(collectionId, ids);
    },

    async search(owner: { ownerType: 'user' | 'channel'; id: string } | null, keyword: string) {
      this.searchKeyword = keyword;
      const requestId = ++this.searchRequestSeq;
      if (!keyword) {
        this.searchResult = [];
        this.searchCollections = {};
        return;
      }

      const ownerType = owner?.ownerType ?? 'user';
      const ownerId = owner?.id;

      try {
        const resp = await apiSearchGallery({ keyword, ownerId, ownerType });
        if (requestId !== this.searchRequestSeq) {
          return;
        }
        this.searchResult = resp.data.items;
        this.searchCollections = resp.data.collections ?? {};
      } catch (error) {
        if (requestId !== this.searchRequestSeq) {
          return;
        }
        this.searchResult = [];
        this.searchCollections = {};
        throw error;
      }
    },

    clearSearch() {
      this.searchResult = [];
      this.searchCollections = {};
      this.searchKeyword = '';
    }
  }
});
