import { defineStore } from 'pinia';
import { api, urlBase } from './_config';
import { getUploadTimeoutMs } from '@/utils/uploadTimeout';
import type {
  AudioAsset,
  AudioDeleteResult,
  AudioFolder,
  AudioPlayableStreamResponse,
} from '@/types/audio';

export interface AudioS3LibrarySettings {
  enabled: boolean;
  prefix: string;
  available: boolean;
  bucket: string;
  canConfigure: boolean;
}

export interface AudioS3BrowsePrefix {
  name: string;
  prefix: string;
}

export interface AudioS3BrowseResult {
  current: string;
  parent: string;
  prefixes: AudioS3BrowsePrefix[];
}

export interface AudioS3Asset extends AudioAsset {
  source: 's3';
  etag?: string;
  contentType?: string;
}

export interface AudioS3Folder extends AudioFolder {
  source: 's3';
  prefix: string;
}

interface AudioS3ListResult {
  items: AudioS3Asset[];
  page: number;
  pageSize: number;
  total: number;
}

interface AudioS3LibraryState {
  settings: AudioS3LibrarySettings;
  settingsLoaded: boolean;
  settingsLoading: boolean;
  assets: AudioS3Asset[];
  selectableAssets: AudioS3Asset[];
  folders: AudioS3Folder[];
  assetsLoading: boolean;
  foldersLoading: boolean;
  selectedFolderId: string | null;
  selectedAssetId: string | null;
  query: string;
  sortBy: 'name' | 'updatedAt' | 'size';
  sortOrder: 'asc' | 'desc';
  page: number;
  pageSize: number;
  total: number;
}

const defaultSettings = (): AudioS3LibrarySettings => ({
  enabled: false,
  prefix: '',
  available: false,
  bucket: '',
  canConfigure: false,
});

function normalizeAsset(asset: AudioS3Asset): AudioS3Asset {
  return {
    ...asset,
    source: 's3',
    tags: Array.isArray(asset.tags) ? asset.tags : [],
    duration: Number(asset.duration || 0),
    bitrate: Number(asset.bitrate || 0),
  };
}

function upsertAsset(list: AudioS3Asset[], asset: AudioS3Asset) {
  const index = list.findIndex((item) => item.id === asset.id);
  if (index < 0) return [asset, ...list];
  const next = [...list];
  next[index] = asset;
  return next;
}

export const useAudioS3LibraryStore = defineStore('audioS3Library', {
  state: (): AudioS3LibraryState => ({
    settings: defaultSettings(),
    settingsLoaded: false,
    settingsLoading: false,
    assets: [],
    selectableAssets: [],
    folders: [],
    assetsLoading: false,
    foldersLoading: false,
    selectedFolderId: null,
    selectedAssetId: null,
    query: '',
    sortBy: 'name',
    sortOrder: 'asc',
    page: 1,
    pageSize: 20,
    total: 0,
  }),

  getters: {
    enabled(state): boolean {
      return state.settings.enabled && state.settings.available;
    },

    selectedAsset(state): AudioS3Asset | null {
      if (!state.selectedAssetId) return null;
      return state.assets.find((item) => item.id === state.selectedAssetId)
        || state.selectableAssets.find((item) => item.id === state.selectedAssetId)
        || null;
    },

    folderPathLookup(state): Record<string, string> {
      const lookup: Record<string, string> = {};
      const walk = (items: AudioS3Folder[]) => {
        for (const folder of items) {
          lookup[folder.id] = folder.path || folder.prefix;
          if (folder.children?.length) walk(folder.children as AudioS3Folder[]);
        }
      };
      walk(state.folders);
      return lookup;
    },
  },

  actions: {
    async ensureSettings(force = false) {
      if (this.settingsLoaded && !force) return this.settings;
      this.settingsLoading = true;
      try {
        const resp = await api.get<AudioS3LibrarySettings>('/api/v1/audio/s3-library/settings');
        this.settings = {
          ...defaultSettings(),
          ...(resp.data || {}),
        };
        this.settingsLoaded = true;
        return this.settings;
      } finally {
        this.settingsLoading = false;
      }
    },

    async saveSettings(payload: { enabled: boolean; prefix: string }) {
      this.settingsLoading = true;
      try {
        const resp = await api.put<AudioS3LibrarySettings>('/api/v1/audio/s3-library/settings', payload);
        this.settings = {
          ...defaultSettings(),
          ...(resp.data || {}),
        };
        this.settingsLoaded = true;
        this.selectedFolderId = null;
        this.selectedAssetId = null;
        this.page = 1;
        this.assets = [];
        this.selectableAssets = [];
        this.folders = [];
        if (this.enabled) {
          await Promise.all([this.fetchFolders(), this.fetchAssets(), this.fetchSelectableAssets()]);
        }
        return this.settings;
      } finally {
        this.settingsLoading = false;
      }
    },

    async browse(prefix = '') {
      const resp = await api.get<AudioS3BrowseResult>('/api/v1/audio/s3-library/browse', {
        params: { prefix },
      });
      return resp.data;
    },

    async fetchAssets(options?: {
      page?: number;
      pageSize?: number;
      folderId?: string | null;
      query?: string;
      recursive?: boolean;
      silent?: boolean;
    }) {
      if (!this.enabled) {
        this.assets = [];
        this.total = 0;
        return [] as AudioS3Asset[];
      }
      if (!options?.silent) this.assetsLoading = true;
      try {
        if (options?.page != null) this.page = options.page;
        if (options?.pageSize != null) this.pageSize = options.pageSize;
        if (options?.folderId !== undefined) this.selectedFolderId = options.folderId;
        if (options?.query !== undefined) this.query = options.query;
        const recursive = options?.recursive ?? !this.selectedFolderId;
        const resp = await api.get<AudioS3ListResult>('/api/v1/audio/s3-library/assets', {
          params: {
            page: this.page,
            pageSize: this.pageSize,
            folderId: this.selectedFolderId || undefined,
            query: this.query.trim() || undefined,
            recursive,
            sortBy: this.sortBy,
            sortOrder: this.sortOrder,
          },
        });
        const items = (resp.data?.items || []).map(normalizeAsset);
        this.assets = items;
        this.page = resp.data?.page || this.page;
        this.pageSize = resp.data?.pageSize || this.pageSize;
        this.total = Number(resp.data?.total || 0);
        if (!this.selectedAssetId || !items.some((item) => item.id === this.selectedAssetId)) {
          this.selectedAssetId = items[0]?.id ?? null;
        }
        return items;
      } finally {
        if (!options?.silent) this.assetsLoading = false;
      }
    },

    async fetchSelectableAssets(maxItems = 5000) {
      if (!this.enabled) {
        this.selectableAssets = [];
        return [] as AudioS3Asset[];
      }
      const pageSize = 500;
      let page = 1;
      let total = 0;
      const result: AudioS3Asset[] = [];
      do {
        const resp = await api.get<AudioS3ListResult>('/api/v1/audio/s3-library/assets', {
          params: {
            page,
            pageSize,
            recursive: true,
            sortBy: 'name',
            sortOrder: 'asc',
          },
        });
        const items = (resp.data?.items || []).map(normalizeAsset);
        result.push(...items);
        total = Number(resp.data?.total || items.length);
        page += 1;
        if (!items.length) break;
      } while (result.length < total && result.length < maxItems);
      this.selectableAssets = result.slice(0, maxItems);
      return this.selectableAssets;
    },

    async fetchAssetsByFolder(folderId: string, maxItems = 5000) {
      if (!folderId) return [] as AudioS3Asset[];
      const pageSize = 500;
      let page = 1;
      let total = 0;
      const result: AudioS3Asset[] = [];
      do {
        const resp = await api.get<AudioS3ListResult>('/api/v1/audio/s3-library/assets', {
          params: {
            page,
            pageSize,
            folderId,
            recursive: false,
            sortBy: 'name',
            sortOrder: 'asc',
          },
        });
        const items = (resp.data?.items || []).map(normalizeAsset);
        result.push(...items);
        total = Number(resp.data?.total || items.length);
        page += 1;
        if (!items.length) break;
      } while (result.length < total && result.length < maxItems);
      return result.slice(0, maxItems);
    },

    async fetchFolders() {
      if (!this.enabled) {
        this.folders = [];
        return [] as AudioS3Folder[];
      }
      this.foldersLoading = true;
      try {
        const resp = await api.get<{ items: AudioS3Folder[] }>('/api/v1/audio/s3-library/folders');
        this.folders = (resp.data?.items || []) as AudioS3Folder[];
        return this.folders;
      } finally {
        this.foldersLoading = false;
      }
    },

    async refresh() {
      await Promise.all([this.fetchFolders(), this.fetchAssets(), this.fetchSelectableAssets()]);
    },

    async setFolder(folderId: string | null) {
      this.selectedFolderId = folderId;
      this.page = 1;
      await this.fetchAssets({ page: 1, folderId });
    },

    async setPage(page: number) {
      if (page <= 0) return;
      await this.fetchAssets({ page });
    },

    async setPageSize(pageSize: number) {
      if (pageSize <= 0) return;
      await this.fetchAssets({ page: 1, pageSize });
    },

    async setQuery(query: string) {
      this.query = query;
      await this.fetchAssets({ page: 1, query });
    },

    async setSort(sortBy: AudioS3LibraryState['sortBy']) {
      if (this.sortBy === sortBy) {
        this.sortOrder = this.sortOrder === 'asc' ? 'desc' : 'asc';
      } else {
        this.sortBy = sortBy;
        this.sortOrder = 'asc';
      }
      await this.fetchAssets({ page: 1 });
    },

    setSelectedAsset(assetId: string | null) {
      this.selectedAssetId = assetId;
    },

    async fetchAsset(assetId: string) {
      const resp = await api.get<AudioS3Asset>(`/api/v1/audio/s3-library/assets/${assetId}`);
      const item = normalizeAsset(resp.data);
      this.assets = upsertAsset(this.assets, item);
      this.selectableAssets = upsertAsset(this.selectableAssets, item);
      return item;
    },

    async fetchPlayableStreamUrl(assetId: string) {
      const resp = await api.post<AudioPlayableStreamResponse>(
        `/api/v1/audio/s3-library/assets/${assetId}/play-token`,
      );
      return resp.data;
    },

    buildRawStreamUrl(assetId: string) {
      return `${urlBase}/api/v1/audio/s3-library/stream/${assetId}`;
    },

    async uploadFiles(files: FileList | File[], folderId?: string | null) {
      const list = Array.from(files);
      const uploaded: AudioS3Asset[] = [];
      for (const file of list) {
        const formData = new FormData();
        formData.append('file', file);
        if (folderId) formData.append('folderId', folderId);
        const resp = await api.post<{ item: AudioS3Asset }>(
          '/api/v1/audio/s3-library/assets/upload',
          formData,
          {
            headers: { 'Content-Type': 'multipart/form-data' },
            timeout: getUploadTimeoutMs(),
          },
        );
        if (resp.data?.item) uploaded.push(normalizeAsset(resp.data.item));
      }
      if (uploaded.length) {
        for (const item of uploaded) {
          this.assets = upsertAsset(this.assets, item);
          this.selectableAssets = upsertAsset(this.selectableAssets, item);
        }
        await this.refresh();
      }
      return uploaded;
    },

    async createFolder(payload: { name: string; parentId?: string | null }) {
      const resp = await api.post<{ item: AudioS3Folder }>('/api/v1/audio/s3-library/folders', payload);
      await this.fetchFolders();
      return resp.data?.item || null;
    },

    async updateFolder(folderId: string, payload: { name?: string; parentId?: string | null }) {
      const resp = await api.patch<{ item: AudioS3Folder }>(
        `/api/v1/audio/s3-library/folders/${folderId}`,
        {
          ...payload,
          parentId: payload.parentId ?? '',
        },
      );
      await this.refresh();
      return resp.data?.item || null;
    },

    async deleteFolder(folderId: string, forceDetach = false) {
      const resp = await api.delete<AudioDeleteResult>(`/api/v1/audio/s3-library/folders/${folderId}`, {
        params: forceDetach ? { forceDetach: true } : undefined,
      });
      if (this.selectedFolderId === folderId) this.selectedFolderId = null;
      await this.refresh();
      return resp.data;
    },

    async updateAsset(assetId: string, payload: { name?: string; folderId?: string | null }) {
      const resp = await api.patch<{ item: AudioS3Asset }>(
        `/api/v1/audio/s3-library/assets/${assetId}`,
        {
          ...payload,
          folderId: payload.folderId ?? '',
        },
      );
      const item = normalizeAsset(resp.data.item);
      this.assets = this.assets.filter((asset) => asset.id !== assetId);
      this.selectableAssets = this.selectableAssets.filter((asset) => asset.id !== assetId);
      this.assets = upsertAsset(this.assets, item);
      this.selectableAssets = upsertAsset(this.selectableAssets, item);
      this.selectedAssetId = item.id;
      await Promise.all([this.fetchAssets({ silent: true }), this.fetchFolders()]);
      return item;
    },

    async deleteAsset(assetId: string, forceDetach = false) {
      const resp = await api.delete<AudioDeleteResult>(`/api/v1/audio/s3-library/assets/${assetId}`, {
        params: forceDetach ? { forceDetach: true } : undefined,
      });
      this.assets = this.assets.filter((item) => item.id !== assetId);
      this.selectableAssets = this.selectableAssets.filter((item) => item.id !== assetId);
      if (this.selectedAssetId === assetId) this.selectedAssetId = this.assets[0]?.id ?? null;
      await this.fetchAssets({ silent: true });
      return resp.data;
    },
  },
});
