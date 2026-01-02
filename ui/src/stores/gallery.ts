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
  emojiCollectionId: string | null;
}

const STORAGE_EMOJI_COLLECTION = 'sealchat.gallery.emojiCollection';

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
    emojiCollectionId: null
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
      if (!state.emojiCollectionId) return [];
      return state.items[state.emojiCollectionId]?.items ?? [];
    },
    emojiCollection(state): GalleryCollection | null {
      if (!state.activeOwner || !state.emojiCollectionId) return null;
      const list = state.collections[ownerKey(state.activeOwner.id)]?.items ?? [];
      return list.find((item) => item.id === state.emojiCollectionId) ?? null;
    }
  },

  actions: {
    loadEmojiPreference(userId: string) {
      const key = `${STORAGE_EMOJI_COLLECTION}:${userId}`;
      const stored = localStorage.getItem(key);
      this.emojiCollectionId = stored || null;
    },

    persistEmojiPreference(userId: string) {
      const key = `${STORAGE_EMOJI_COLLECTION}:${userId}`;
      if (this.emojiCollectionId) {
        localStorage.setItem(key, this.emojiCollectionId);
      } else {
        localStorage.removeItem(key);
      }
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
      if (this.emojiCollectionId && this.emojiCollectionId !== this.activeCollectionId) {
        // Load emoji collection in the background so panel init isn't blocked.
        void this.loadItems(this.emojiCollectionId).catch(() => {});
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

    linkEmojiCollection(collectionId: string | null, userId: string) {
      this.emojiCollectionId = collectionId;
      this.persistEmojiPreference(userId);
      if (collectionId) {
        void this.loadItems(collectionId);
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
      if (this.emojiCollectionId === collectionId) {
        this.emojiCollectionId = null;
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
