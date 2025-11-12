import { defineStore } from 'pinia';
import { Howl, Howler } from 'howler';
import { nanoid } from 'nanoid';
import { api, urlBase } from './_config';
import { useUserStore } from './user';
import { audioDb, toCachedMeta } from '@/models/audio-cache';
import type {
  AudioAsset,
  AudioAssetMutationPayload,
  AudioAssetQueryParams,
  AudioFolder,
  AudioFolderPayload,
  AudioScene,
  AudioSceneInput,
  AudioSceneTrack,
  AudioSearchFilters,
  AudioTrackType,
  AudioPlaybackStatePayload,
  AudioTrackStatePayload,
  PaginatedResult,
  UploadTaskState,
} from '@/types/audio';

export interface TrackRuntime extends AudioSceneTrack {
  id: string;
  asset?: AudioAsset | null;
  howl?: Howl | null;
  status: 'idle' | 'loading' | 'ready' | 'playing' | 'paused' | 'error';
  progress: number;
  buffered: number;
  duration: number;
  muted: boolean;
  solo: boolean;
  error?: string;
  pendingSeek?: number | null;
}

interface AudioStudioState {
  drawerVisible: boolean;
  initialized: boolean;
  activeTab: 'player' | 'playlist' | 'library';
  scenes: AudioScene[];
  scenesLoading: boolean;
  sceneFilters: {
    query: string;
    tags: string[];
    folderId: string | null;
  };
  scenePagination: PaginationState;
  selectedSceneId: string | null;
  currentSceneId: string | null;
  tracks: Record<AudioTrackType, TrackRuntime>;
  assets: AudioAsset[];
  filteredAssets: AudioAsset[];
  assetsLoading: boolean;
  assetPagination: PaginationState;
  selectedAssetId: string | null;
  assetMutationLoading: boolean;
  assetBulkLoading: boolean;
  folders: AudioFolder[];
  folderPathLookup: Record<string, string>;
  folderActionLoading: boolean;
  filters: AudioSearchFilters;
  uploadTasks: UploadTaskState[];
  networkMode: 'normal' | 'constrained' | 'minimal';
  bufferMessage: string;
  isPlaying: boolean;
  loopEnabled: boolean;
  playbackRate: number;
  error: string | null;
  currentChannelId: string | null;
  remoteState: AudioPlaybackStatePayload | null;
  isApplyingRemoteState: boolean;
  pendingSyncHandle: number | null;
}

export const DEFAULT_TRACK_TYPES: AudioTrackType[] = ['music', 'ambience', 'sfx'];
if (typeof window !== 'undefined' && typeof Howler !== 'undefined') {
  const desiredPool = 18;
  if ((Howler as typeof Howler & { html5PoolSize?: number }).html5PoolSize < desiredPool) {
    (Howler as typeof Howler & { html5PoolSize?: number }).html5PoolSize = desiredPool;
  }
}
let progressTimer: number | null = null;
const SYNC_DEBOUNCE_MS = 300;

function createEmptyTrack(type: AudioTrackType): TrackRuntime {
  return {
    id: nanoid(),
    type,
    assetId: null,
    asset: null,
    volume: 0.8,
    fadeIn: 2000,
    fadeOut: 2000,
    howl: null,
    status: 'idle',
    progress: 0,
    buffered: 0,
    duration: 0,
    muted: false,
    solo: false,
    pendingSeek: null,
  };
}

function startProgressWatcher(store: ReturnType<typeof useAudioStudioStore>) {
  if (typeof window === 'undefined') return;
  if (progressTimer) return;
  progressTimer = window.setInterval(() => {
    store.updateProgressFromPlayers();
  }, 500);
}

function serializeRuntimeTracks(tracks: Record<AudioTrackType, TrackRuntime>): AudioSceneTrack[] {
  return DEFAULT_TRACK_TYPES.map((type) => {
    const runtime = tracks[type] || createEmptyTrack(type);
    return {
      type,
      assetId: runtime.assetId || null,
      volume: typeof runtime.volume === 'number' ? runtime.volume : 0.8,
      fadeIn: runtime.fadeIn ?? 2000,
      fadeOut: runtime.fadeOut ?? 2000,
    } as AudioSceneTrack;
  });
}

function stopProgressWatcher() {
  if (typeof window === 'undefined') return;
  if (!progressTimer) return;
  window.clearInterval(progressTimer);
  progressTimer = null;
}

function buildFolderPathLookup(folders: AudioFolder[]): Record<string, string> {
  const lookup: Record<string, string> = {};
  const walk = (items: AudioFolder[], parentPath: string) => {
    items.forEach((folder) => {
      const path = folder.path || (parentPath ? `${parentPath}/${folder.name}` : folder.name);
      lookup[folder.id] = path;
      if (folder.children?.length) {
        walk(folder.children, path);
      }
    });
  };
  walk(folders, '');
  return lookup;
}

interface TrackMutationOptions {
  force?: boolean;
  initialSeek?: number;
}

interface PaginationState {
  page: number;
  pageSize: number;
  total: number;
}

interface FetchAssetsOptions {
  filters?: Partial<AudioSearchFilters>;
  pagination?: Partial<PaginationState>;
  silent?: boolean;
}

function normalizeFolderId(input: string | null | undefined): string | null {
  if (input === undefined || input === null) return null;
  const trimmed = String(input).trim();
  if (!trimmed || trimmed === 'undefined' || trimmed === 'null') {
    return null;
  }
  return trimmed;
}

function buildAssetQueryParams(filters: AudioSearchFilters, pagination: PaginationState): AudioAssetQueryParams {
  const params: AudioAssetQueryParams = {
    page: pagination.page,
    pageSize: pagination.pageSize,
  };
  const query = filters.query?.trim();
  if (query) {
    params.query = query;
  }
  if (filters.tags?.length) {
    params.tags = filters.tags;
  }
  const normalizedFolderId = normalizeFolderId(filters.folderId);
  if (normalizedFolderId) {
    params.folderId = normalizedFolderId;
  }
  if (filters.creatorIds?.length) {
    params.creatorIds = filters.creatorIds;
  }
  if (filters.durationRange && filters.durationRange.length === 2) {
    params.durationMin = filters.durationRange[0];
    params.durationMax = filters.durationRange[1];
  }
  if (filters.hasSceneOnly) {
    params.hasSceneOnly = true;
  }
  return params;
}

export const useAudioStudioStore = defineStore('audioStudio', {
  state: (): AudioStudioState => ({
    drawerVisible: false,
    initialized: false,
    activeTab: 'player',
    scenes: [],
    scenesLoading: false,
    sceneFilters: {
      query: '',
      tags: [],
      folderId: null,
    },
    scenePagination: { page: 1, pageSize: 10, total: 0 },
    selectedSceneId: null,
    currentSceneId: null,
    tracks: DEFAULT_TRACK_TYPES.reduce((acc, type) => {
      acc[type] = createEmptyTrack(type);
      return acc;
    }, {} as Record<AudioTrackType, TrackRuntime>),
    assets: [],
    filteredAssets: [],
    assetsLoading: false,
    assetPagination: { page: 1, pageSize: 20, total: 0 },
    selectedAssetId: null,
    assetMutationLoading: false,
    assetBulkLoading: false,
    folders: [],
    folderPathLookup: {},
    folderActionLoading: false,
    filters: {
      query: '',
      tags: [],
      folderId: null,
      creatorIds: [],
      durationRange: null,
      hasSceneOnly: false,
    },
    uploadTasks: [],
    networkMode: 'normal',
    bufferMessage: '',
    isPlaying: false,
    loopEnabled: false,
    playbackRate: 1,
    error: null,
    currentChannelId: null,
    remoteState: null,
    isApplyingRemoteState: false,
    pendingSyncHandle: null,
  }),

  getters: {
    currentScene(state): AudioScene | null {
      return state.scenes.find((scene) => scene.id === state.currentSceneId) || null;
    },

    selectedScene(state): AudioScene | null {
      if (!state.selectedSceneId) return null;
      return state.scenes.find((scene) => scene.id === state.selectedSceneId) || null;
    },

    selectedAsset(state): AudioAsset | null {
      if (!state.selectedAssetId) return null;
      return state.filteredAssets.find((asset) => asset.id === state.selectedAssetId) || null;
    },

    canManage(): boolean {
      const user = useUserStore();
      return Boolean(user.checkPerm?.('mod_admin'));
    },
  },

  actions: {
    setActiveChannel(channelId: string | null) {
      if (typeof window !== 'undefined' && this.pendingSyncHandle) {
        window.clearTimeout(this.pendingSyncHandle);
        this.pendingSyncHandle = null;
      }
      if (!this.canManage && this.activeTab !== 'player') {
        this.activeTab = 'player';
      }
      if (this.currentChannelId === channelId) {
        return;
      }
      this.currentChannelId = channelId;
      if (!channelId) {
        this.remoteState = null;
        return;
      }
      this.fetchPlaybackState(channelId);
    },

    async fetchPlaybackState(channelId: string) {
      if (!channelId) return;
      try {
        const resp = await api.get('/api/v1/audio/state', { params: { channelId } });
        await this.applyRemotePlayback(resp.data?.state || null);
      } catch (err) {
        console.warn('fetchPlaybackState failed', err);
      }
    },

    async applyRemotePlayback(payload: AudioPlaybackStatePayload | null) {
      if (!this.currentChannelId) {
        return;
      }
      if (payload && payload.channelId !== this.currentChannelId) {
        return;
      }
      const user = useUserStore();
      if (payload && payload.updatedBy && payload.updatedBy === user.info.id) {
        return;
      }
      this.remoteState = payload;
      if (!payload) {
        this.isApplyingRemoteState = true;
        try {
          await this.pauseAll({ force: true });
        } finally {
          this.isApplyingRemoteState = false;
        }
        return;
      }
      this.isApplyingRemoteState = true;
      const targetPosition = typeof payload.position === 'number' ? payload.position : 0;
      try {
        this.loopEnabled = payload.loopEnabled ?? this.loopEnabled;
        this.playbackRate = payload.playbackRate || 1;
        this.currentSceneId = payload.sceneId || null;
        const trackStates = payload.tracks || [];
        await Promise.all(
          DEFAULT_TRACK_TYPES.map(async (type) => {
            const incoming = trackStates.find((t) => t.type === type);
            if (!incoming || !incoming.assetId) {
              this.tracks[type] = createEmptyTrack(type);
              return;
            }
            let track = this.tracks[type];
            if (!track) {
              track = createEmptyTrack(type);
              this.tracks[type] = track;
            }
            track.volume = typeof incoming.volume === 'number' ? incoming.volume : track.volume;
            track.muted = incoming.muted ?? false;
            track.solo = incoming.solo ?? false;
            track.fadeIn = incoming.fadeIn ?? track.fadeIn;
            track.fadeOut = incoming.fadeOut ?? track.fadeOut;
            track.pendingSeek = targetPosition;
            track.status = 'loading';
            if (track.howl) {
              track.howl.unload();
              track.howl = null;
            }
            let asset = this.assets.find((item) => item.id === incoming.assetId) || null;
            if (!asset) {
              try {
                asset = await this.fetchSingleAsset(incoming.assetId);
              } catch (err) {
                console.warn('fetch asset failed', err);
                track.status = 'error';
                track.error = '资源加载失败';
                return;
              }
            }
            track.asset = asset;
            track.assetId = asset.id;
            track.howl = this.createHowlInstance(track, asset, { initialSeek: targetPosition });
            track.status = payload.isPlaying ? 'playing' : 'ready';
          }),
        );
        if (payload.isPlaying) {
          await this.playAll({ force: true });
        } else {
          await this.pauseAll({ force: true });
          await this.seekToSeconds(targetPosition, { force: true });
        }
      } finally {
        this.isApplyingRemoteState = false;
      }
    },

    queuePlaybackSync() {
      if (!this.canManage || this.isApplyingRemoteState || !this.currentChannelId) {
        return;
      }
      if (typeof window === 'undefined') {
        void this.commitPlaybackSync();
        return;
      }
      if (this.pendingSyncHandle) {
        window.clearTimeout(this.pendingSyncHandle);
      }
      this.pendingSyncHandle = window.setTimeout(() => {
        this.pendingSyncHandle = null;
        void this.commitPlaybackSync();
      }, SYNC_DEBOUNCE_MS);
    },

    async commitPlaybackSync() {
      if (!this.canManage || this.isApplyingRemoteState || !this.currentChannelId) {
        return;
      }
      const payload = this.serializePlaybackState();
      if (!payload) return;
      try {
        await api.post('/api/v1/audio/state', payload);
      } catch (err) {
        console.warn('同步音频状态失败', err);
      }
    },

    serializePlaybackState() {
      if (!this.currentChannelId) return null;
      return {
        channelId: this.currentChannelId,
        sceneId: this.currentSceneId,
        tracks: this.buildTrackStatePayload(),
        isPlaying: this.isPlaying,
        position: this.estimatePlaybackPosition(),
        loopEnabled: this.loopEnabled,
        playbackRate: this.playbackRate,
      };
    },

    buildTrackStatePayload(): AudioTrackStatePayload[] {
      return DEFAULT_TRACK_TYPES.map((type) => {
        const track = this.tracks[type] || createEmptyTrack(type);
        return {
          type,
          assetId: track.assetId,
          volume: track.volume,
          muted: track.muted,
          solo: track.solo,
          fadeIn: track.fadeIn,
          fadeOut: track.fadeOut,
        };
      });
    },

    estimatePlaybackPosition() {
      const candidates = Object.values(this.tracks || {});
      for (const track of candidates) {
        if (track?.howl) {
          const value = track.howl.seek();
          if (typeof value === 'number' && value >= 0) {
            return value;
          }
        }
      }
      return 0;
    },

    async toggleDrawer(next?: boolean) {
      const target = typeof next === 'boolean' ? next : !this.drawerVisible;
      this.drawerVisible = target;
      if (target) {
        await this.ensureInitialized();
      }
    },

    async ensureInitialized() {
      if (this.initialized) return;
      await Promise.all([this.fetchScenes(), this.fetchFolders()]);
      await this.fetchAssets();
      this.initialized = true;
      if (!this.currentSceneId && this.scenes.length) {
        this.applyScene(this.scenes[0].id);
      }
    },

    async fetchScenes(filters?: Partial<AudioStudioState['sceneFilters']>) {
      try {
        this.scenesLoading = true;
        if (filters) {
          this.sceneFilters = {
            ...this.sceneFilters,
            ...filters,
            folderId: normalizeFolderId(filters.folderId) ?? null,
          };
        }
        if (filters && filters.query !== undefined) {
          this.scenePagination.page = 1;
        }
        const params: Record<string, unknown> = {
          ...this.sceneFilters,
          page: this.scenePagination.page,
          pageSize: this.scenePagination.pageSize,
        };
        if (!params.folderId) {
          delete params.folderId;
        }
        if (!params.query) {
          delete params.query;
        }
        if (!this.canManage) {
          params.channelScope = this.currentChannelId || undefined;
        }
        const resp = await api.get('/api/v1/audio/scenes', { params });
        const raw = resp.data as PaginatedResult<AudioScene> | AudioScene[] | undefined;
        const items = Array.isArray(raw) ? raw : raw?.items || [];
        this.scenes = items;
        if (!Array.isArray(raw) && raw) {
          this.scenePagination = {
            page: raw.page ?? this.scenePagination.page,
            pageSize: raw.pageSize ?? this.scenePagination.pageSize,
            total: raw.total ?? items.length,
          };
        } else {
          this.scenePagination = {
            ...this.scenePagination,
            total: items.length,
          };
        }
        if (!this.selectedSceneId && items.length) {
          this.selectedSceneId = items[0].id;
        } else if (this.selectedSceneId && !items.some((scene) => scene.id === this.selectedSceneId)) {
          this.selectedSceneId = items[0]?.id ?? null;
        }
      } catch (err) {
        console.error('fetchScenes failed', err);
        this.error = '无法加载音频场景';
      } finally {
        this.scenesLoading = false;
      }
    },

    setScenePage(page: number) {
      if (page <= 0) return;
      this.scenePagination.page = page;
      this.fetchScenes();
    },

    setScenePageSize(pageSize: number) {
      if (pageSize <= 0) return;
      this.scenePagination.pageSize = pageSize;
      this.scenePagination.page = 1;
      this.fetchScenes();
    },

    setSelectedScene(sceneId: string | null) {
      this.selectedSceneId = sceneId;
    },


    async createSceneFromCurrentTracks(payload: Omit<AudioSceneInput, 'tracks'> & { autoPlayAfterSave?: boolean }) {
      if (!this.canManage) {
        throw new Error('无权限创建播放列表');
      }
      const scenePayload: AudioSceneInput = {
        name: payload.name,
        description: payload.description,
        tags: payload.tags || [],
        tracks: serializeRuntimeTracks(this.tracks),
        folderId: normalizeFolderId(payload.folderId) ?? null,
        channelScope: payload.channelScope ?? this.currentChannelId ?? null,
        order: payload.order,
      };
      const resp = await api.post('/api/v1/audio/scenes', scenePayload);
      const created = resp.data?.item as AudioScene | undefined;
      if (created) {
        this.scenes.unshift(created);
        this.scenePagination.total += 1;
        this.selectedSceneId = created.id;
        await this.fetchScenes();
      }
      if (payload.autoPlayAfterSave && created) {
        await this.applyScene(created.id, { autoPlay: true });
      }
      return created;
    },

    async updateScene(sceneId: string, payload: Partial<AudioSceneInput>) {
      if (!this.canManage || !sceneId) return null;
      const existing = this.scenes.find((scene) => scene.id === sceneId);
      const normalized: AudioSceneInput = {
        name: payload.name || existing?.name || '无标题',
        description: payload.description,
        tags: payload.tags,
        tracks: payload.tracks || [],
        folderId: normalizeFolderId(payload.folderId) ?? null,
        channelScope: payload.channelScope ?? existing?.channelScope ?? null,
        order: payload.order ?? existing?.order,
      };
      if (!normalized.tracks.length) {
        normalized.tracks = existing ? existing.tracks : serializeRuntimeTracks(this.tracks);
      }
      const resp = await api.patch(`/api/v1/audio/scenes/${sceneId}`, normalized);
      const updated = resp.data?.item as AudioScene | undefined;
      if (updated) {
        const index = this.scenes.findIndex((scene) => scene.id === sceneId);
        if (index >= 0) {
          this.scenes[index] = updated;
        } else {
          this.scenes.unshift(updated);
        }
        if (this.currentSceneId === sceneId) {
          this.applyScene(sceneId, { skipSync: true });
          this.queuePlaybackSync();
        }
        await this.fetchScenes();
      }
      return updated;
    },

    async deleteScenes(sceneIds: string[]) {
      if (!this.canManage || !sceneIds.length) return { success: 0, failed: 0 };
      let success = 0;
      for (const id of sceneIds) {
        try {
          await api.delete(`/api/v1/audio/scenes/${id}`);
          success += 1;
          this.scenes = this.scenes.filter((scene) => scene.id !== id);
        } catch (err) {
          console.error('delete scene failed', err);
        }
      }
      if (success) {
        this.scenePagination.total = Math.max(0, this.scenePagination.total - success);
        await this.fetchScenes();
      }
      if (sceneIds.includes(this.currentSceneId || '')) {
        this.currentSceneId = this.scenes[0]?.id ?? null;
      }
      return { success, failed: sceneIds.length - success };
    },

    async fetchAssets(options?: FetchAssetsOptions) {
      if (!options?.silent) {
        this.assetsLoading = true;
      }
      try {
        const mergedFilters: AudioSearchFilters = {
          ...this.filters,
          ...(options?.filters || {}),
        };
        mergedFilters.folderId = normalizeFolderId(mergedFilters.folderId) ?? null;
        this.filters = mergedFilters;

        const pagination: PaginationState = {
          ...this.assetPagination,
          ...(options?.pagination || {}),
        };
        const params = buildAssetQueryParams(mergedFilters, pagination);
        const resp = await api.get('/api/v1/audio/assets', { params });
        const raw = resp.data as PaginatedResult<AudioAsset> | AudioAsset[] | undefined;
        const items = Array.isArray(raw) ? raw : raw?.items || [];
        const page = !Array.isArray(raw) && raw?.page ? raw.page : pagination.page;
        const pageSize = !Array.isArray(raw) && raw?.pageSize ? raw.pageSize : pagination.pageSize;
        const total = !Array.isArray(raw) && typeof raw?.total === 'number' ? raw.total : items.length;
        this.assetPagination = {
          page,
          pageSize,
          total,
        };
        this.assets = items;
        this.filteredAssets = items;
        if (!this.selectedAssetId && items.length) {
          this.selectedAssetId = items[0].id;
        } else if (this.selectedAssetId && !items.some((asset) => asset.id === this.selectedAssetId)) {
          this.selectedAssetId = items[0]?.id ?? null;
        }
        await this.persistAssetsToCache();
      } catch (err) {
        console.warn('fetchAssets failed, fallback to cache', err);
        const query = (this.filters.query ?? '').trim().toLowerCase();
        const cached = query
          ? await audioDb.assets.where('searchIndex').startsWith(query).toArray()
          : await audioDb.assets.orderBy('updatedAt').reverse().toArray();
        const fallback = cached.map((meta) => ({
          id: meta.id,
          name: meta.name,
          folderId: meta.folderId,
          tags: meta.tags,
          createdBy: meta.creator,
          duration: meta.duration,
          updatedAt: new Date(meta.updatedAt).toISOString(),
          updatedBy: meta.creator,
          size: 0,
          bitrate: 0,
          storageType: 'local',
          objectKey: '',
          visibility: 'public',
          createdAt: new Date(meta.updatedAt).toISOString(),
          description: meta.description,
        } as AudioAsset));
        this.assets = fallback;
        this.filteredAssets = fallback;
        this.assetPagination = {
          ...this.assetPagination,
          page: 1,
          total: fallback.length,
        };
        if (!fallback.some((asset) => asset.id === this.selectedAssetId)) {
          this.selectedAssetId = fallback[0]?.id ?? null;
        }
      } finally {
        if (!options?.silent) {
          this.assetsLoading = false;
        }
      }
    },

    async fetchFolders() {
      try {
        const resp = await api.get('/api/v1/audio/folders');
        this.folders = resp.data?.items || [];
        this.folderPathLookup = buildFolderPathLookup(this.folders);
        await this.refreshLocalCacheWithFolderPaths();
      } catch (err) {
        console.error('fetchFolders failed', err);
      }
    },

    async createFolder(payload: AudioFolderPayload) {
      this.folderActionLoading = true;
      try {
        await api.post('/api/v1/audio/folders', payload);
        await this.fetchFolders();
      } catch (err) {
        console.error('createFolder failed', err);
        throw err;
      } finally {
        this.folderActionLoading = false;
      }
    },

    async updateFolder(folderId: string, payload: Partial<AudioFolderPayload>) {
      if (!folderId) return;
      this.folderActionLoading = true;
      try {
        await api.patch(`/api/v1/audio/folders/${folderId}`, payload);
        await this.fetchFolders();
      } catch (err) {
        console.error('updateFolder failed', err);
        throw err;
      } finally {
        this.folderActionLoading = false;
      }
    },

    async deleteFolder(folderId: string) {
      if (!folderId) return;
      this.folderActionLoading = true;
      try {
        await api.delete(`/api/v1/audio/folders/${folderId}`);
        if (this.filters.folderId === folderId) {
          this.filters.folderId = null;
        }
        await this.fetchFolders();
        await this.fetchAssets({ pagination: { page: 1 } });
      } catch (err) {
        console.error('deleteFolder failed', err);
        throw err;
      } finally {
        this.folderActionLoading = false;
      }
    },

    selectTab(tab: AudioStudioState['activeTab']) {
      if (!this.canManage && tab !== 'player') {
        this.activeTab = 'player';
        return;
      }
      this.activeTab = tab;
    },

    async applyScene(sceneId: string | null, options?: { autoPlay?: boolean; force?: boolean; skipSync?: boolean }) {
      if (!sceneId) return;
      const scene = this.scenes.find((item) => item.id === sceneId);
      if (!scene) return;
      this.currentSceneId = sceneId;
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const trackMeta = scene.tracks.find((t) => t.type === type) || createEmptyTrack(type);
        this.assignTrack(type, trackMeta);
      });
      if (options?.autoPlay ?? this.isPlaying) {
        await this.playAll({ force: options?.force });
      } else {
        this.pauseAll({ force: true });
      }
      if (!options?.force && !options?.skipSync) {
        this.queuePlaybackSync();
      }
    },

    assignTrack(type: AudioTrackType, payload: AudioSceneTrack, options?: TrackMutationOptions) {
      if (!options?.force && !this.canManage) {
        return;
      }
      const prev = this.tracks[type];
      if (prev?.howl) {
        prev.howl.unload();
      }
      this.tracks[type] = {
        ...createEmptyTrack(type),
        ...payload,
        id: prev?.id || nanoid(),
        status: payload.assetId ? 'loading' : 'idle',
        pendingSeek: options?.initialSeek ?? null,
      };
      if (payload.assetId) {
        this.loadTrackAsset(type, payload.assetId, options);
      }
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    async assignAssetToTrack(type: AudioTrackType, asset: AudioAsset, options?: TrackMutationOptions) {
      const track = this.tracks[type];
      if (!track) return;
      if (!options?.force && !this.canManage) {
        return;
      }
      if (!track) return;
      if (track.howl) {
        track.howl.unload();
      }
      track.assetId = asset.id;
      track.asset = asset;
      track.status = 'loading';
      track.pendingSeek = options?.initialSeek ?? track.pendingSeek ?? null;
      track.howl = this.createHowlInstance(track, asset, { initialSeek: track.pendingSeek ?? undefined });
      track.status = 'ready';
      if (this.isPlaying && track.howl && !track.muted) {
        track.howl.play();
      }
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    async loadTrackAsset(type: AudioTrackType, assetId: string, options?: TrackMutationOptions) {
      const track = this.tracks[type];
      if (!track) return;
      if (!options?.force && !this.canManage) {
        // 非管理端仅在远端同步时传 force
        return;
      }
      try {
        track.status = 'loading';
        const asset = this.assets.find((item) => item.id === assetId) || (await this.fetchSingleAsset(assetId));
        track.asset = asset;
        track.assetId = asset.id;
        track.pendingSeek = options?.initialSeek ?? track.pendingSeek ?? null;
        track.howl = this.createHowlInstance(track, asset, { initialSeek: track.pendingSeek ?? undefined });
        track.status = 'ready';
      } catch (err) {
        console.error('loadTrackAsset error', err);
        track.status = 'error';
        track.error = '资源加载失败';
      }
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    async fetchSingleAsset(assetId: string) {
      const resp = await api.get(`/api/v1/audio/assets/${assetId}`);
      const asset = resp.data as AudioAsset;
      this.assets = [...this.assets.filter((item) => item.id !== asset.id), asset];
      await audioDb.assets.put(toCachedMeta(asset));
      return asset;
    },

    buildStreamUrl(assetId: string) {
      return `${urlBase}/api/v1/audio/stream/${assetId}`;
    },

    createHowlInstance(track: TrackRuntime, asset: AudioAsset, options?: { initialSeek?: number }) {
      const src = this.buildStreamUrl(asset.id);
      const howl = new Howl({
        src: [src],
        html5: true,
        preload: false,
        volume: track.volume,
        onplay: () => {
          track.status = 'playing';
          this.isPlaying = true;
          startProgressWatcher(this);
        },
        onpause: () => {
          track.status = 'paused';
        },
        onstop: () => {
          track.status = 'ready';
        },
        onend: () => {
          track.status = 'ready';
          if (this.allTracksIdle()) {
            this.isPlaying = false;
            stopProgressWatcher();
          }
        },
        onload: () => {
          track.duration = howl.duration();
          const targetSeek =
            typeof options?.initialSeek === 'number' ? options.initialSeek : track.pendingSeek ?? 0;
          if (targetSeek && targetSeek > 0 && !Number.isNaN(targetSeek)) {
            const maxDuration = howl.duration() || targetSeek;
            howl.seek(Math.min(targetSeek, maxDuration));
          }
          track.pendingSeek = null;
        },
        onloaderror: (_, err) => {
          track.status = 'error';
          track.error = String(err);
        },
        onplayerror: (_, err) => {
          track.status = 'error';
          track.error = String(err);
        },
      });
      return howl;
    },

    async playAll(options?: { force?: boolean }) {
      if (!options?.force && !this.canManage) {
        return;
      }
      this.isPlaying = true;
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        if (track?.howl && track.assetId && !track.muted) {
          track.howl.loop(this.loopEnabled);
          track.howl.rate(this.playbackRate);
          const alreadyPlaying = track.howl.playing();
          if (!alreadyPlaying) {
            track.howl.play();
          }
        }
      });
      startProgressWatcher(this);
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    pauseAll(options?: { force?: boolean }) {
      if (!options?.force && !this.canManage) {
        return;
      }
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        if (track?.howl && track.howl.playing()) {
          track.howl.pause();
        }
      });
      this.isPlaying = false;
      stopProgressWatcher();
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    togglePlay() {
      if (!this.canManage) return;
      if (this.isPlaying) {
        this.pauseAll();
      } else {
        this.playAll();
      }
    },

    seekAll(deltaSeconds: number, options?: { force?: boolean }) {
      if (!options?.force && !this.canManage) {
        return;
      }
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        if (!track?.howl) return;
        const current = track.howl.seek() as number;
        track.howl.seek(Math.max(0, current + deltaSeconds));
      });
      this.updateProgressFromPlayers();
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    seekToSeconds(position: number, options?: { force?: boolean }) {
      const target = Math.max(0, position);
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        if (!track) return;
        if (track.howl) {
          track.howl.seek(target);
        } else {
          track.pendingSeek = target;
        }
      });
      this.updateProgressFromPlayers();
      if (!options?.force && this.canManage) {
        this.queuePlaybackSync();
      }
    },

    setTrackVolume(type: AudioTrackType, value: number) {
      const track = this.tracks[type];
      if (!track) return;
      track.volume = value;
      this.applyEffectiveVolume(type);
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    toggleTrackMute(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track) return;
      track.muted = !track.muted;
      track.solo = false;
      this.applyEffectiveVolume(type);
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    toggleTrackSolo(type: AudioTrackType) {
      const target = this.tracks[type];
      if (!target) return;
      const nextState = !target.solo;
      DEFAULT_TRACK_TYPES.forEach((key) => {
        const track = this.tracks[key];
        if (!track) return;
        track.solo = key === type ? nextState : false;
        track.muted = nextState ? key !== type : track.muted;
        this.applyEffectiveVolume(key);
      });
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    applyEffectiveVolume(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track?.howl) return;
      const effectiveVolume = track.muted ? 0 : track.volume;
      track.howl.volume(effectiveVolume);
    },

    setPlaybackRate(rate: number, options?: { force?: boolean }) {
      if (!options?.force && !this.canManage) return;
      this.playbackRate = rate;
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        track?.howl?.rate(rate);
      });
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    toggleLoop(options?: { force?: boolean }) {
      if (!options?.force && !this.canManage) return;
      this.loopEnabled = !this.loopEnabled;
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        if (track?.howl) {
          track.howl.loop(this.loopEnabled);
        }
      });
      if (!options?.force) {
        this.queuePlaybackSync();
      }
    },

    updateProgressFromPlayers() {
      let anyBuffering = false;
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const track = this.tracks[type];
        if (!track?.howl) return;
        const duration = track.howl.duration();
        if (duration > 0) {
          track.progress = (track.howl.seek() as number) / duration;
        }
        const sound = (track.howl as any)?._sounds?.[0]?._node as HTMLAudioElement | undefined;
        if (sound && sound.buffered.length) {
          const end = sound.buffered.end(sound.buffered.length - 1);
          track.buffered = Math.min(1, end / duration);
          anyBuffering = track.buffered < 1;
        }
      });
      this.bufferMessage = anyBuffering ? '正在边下边播' : '已缓存全部音频';
    },

    allTracksIdle() {
      return DEFAULT_TRACK_TYPES.every((type) => {
        const track = this.tracks[type];
        return !track || track.status === 'idle' || track.status === 'ready';
      });
    },

    async applyFilters(filters: Partial<AudioSearchFilters>) {
      const mergedFilters: AudioSearchFilters = {
        ...this.filters,
        ...filters,
      };
      mergedFilters.folderId = normalizeFolderId(mergedFilters.folderId) ?? null;
      this.filters = mergedFilters;
      this.assetPagination.page = 1;
      await this.fetchAssets({ pagination: { page: 1 } });
    },

    async searchAssetsLocally(keyword: string) {
      this.filters.query = keyword;
      if (!keyword.trim()) {
        this.filteredAssets = this.assets;
        return;
      }
      const lower = keyword.toLowerCase();
      const lookup = this.folderPathLookup;
      this.filteredAssets = this.assets.filter((asset) => {
        const folderPath = asset.folderId ? lookup[asset.folderId] ?? '' : '';
        const description = asset.description ?? '';
        const joined = `${asset.name} ${asset.tags.join(' ')} ${asset.createdBy} ${folderPath} ${description}`.toLowerCase();
        return joined.includes(lower);
      });
      if (this.filteredAssets.length === 0) {
        try {
          await this.fetchAssets({ filters: { query: keyword }, pagination: { page: 1 } });
        } catch (err) {
          console.warn('远程搜索失败', err);
        }
      }
    },

    async setAssetPage(page: number) {
      if (page <= 0) return;
      this.assetPagination.page = page;
      await this.fetchAssets({ pagination: { page } });
    },

    async setAssetPageSize(pageSize: number) {
      if (pageSize <= 0) return;
      this.assetPagination.pageSize = pageSize;
      this.assetPagination.page = 1;
      await this.fetchAssets({ pagination: { page: 1, pageSize } });
    },

    setSelectedAsset(assetId: string | null) {
      this.selectedAssetId = assetId;
    },

    upsertAssetLocally(asset: AudioAsset) {
      const updateList = (list: AudioAsset[]) => {
        const index = list.findIndex((item) => item.id === asset.id);
        if (index >= 0) {
          list[index] = { ...list[index], ...asset };
        } else {
          list.unshift(asset);
        }
      };
      updateList(this.assets);
      updateList(this.filteredAssets);
      if (!this.selectedAssetId) {
        this.selectedAssetId = asset.id;
      }
    },

    removeAssetLocally(assetId: string) {
      const filterList = (list: AudioAsset[]) => list.filter((item) => item.id !== assetId);
      this.assets = filterList(this.assets);
      this.filteredAssets = filterList(this.filteredAssets);
      if (this.selectedAssetId === assetId) {
        this.selectedAssetId = this.filteredAssets[0]?.id ?? null;
      }
    },

    async updateAssetMeta(assetId: string, payload: AudioAssetMutationPayload) {
      if (!assetId) return;
      this.assetMutationLoading = true;
      try {
        const resp = await api.patch(`/api/v1/audio/assets/${assetId}`, payload);
        const updated = resp.data as AudioAsset | undefined;
        if (updated) {
          this.upsertAssetLocally(updated);
        } else {
          const existing = this.assets.find((item) => item.id === assetId);
          if (existing) {
            this.upsertAssetLocally({ ...existing, ...payload });
          }
        }
        await this.persistAssetsToCache();
        await this.fetchAssets({ pagination: { page: this.assetPagination.page }, silent: true });
      } catch (err) {
        console.error('updateAssetMeta failed', err);
        throw err;
      } finally {
        this.assetMutationLoading = false;
      }
    },

    async deleteAsset(assetId: string) {
      if (!assetId) return;
      this.assetMutationLoading = true;
      try {
        await api.delete(`/api/v1/audio/assets/${assetId}`);
        this.removeAssetLocally(assetId);
        this.assetPagination.total = Math.max(0, this.assetPagination.total - 1);
        const nextPage = this.filteredAssets.length
          ? this.assetPagination.page
          : Math.max(1, this.assetPagination.page - 1);
        await this.fetchAssets({ pagination: { page: nextPage }, silent: false });
      } catch (err) {
        console.error('deleteAsset failed', err);
        throw err;
      } finally {
        this.assetMutationLoading = false;
      }
    },

    async batchUpdateAssets(assetIds: string[], payload: AudioAssetMutationPayload) {
      if (!assetIds?.length) {
        return { success: 0, failed: 0 };
      }
      this.assetBulkLoading = true;
      try {
        const tasks = assetIds.map((id) => api.patch(`/api/v1/audio/assets/${id}`, payload));
        const results = await Promise.allSettled(tasks);
        let success = 0;
        results.forEach((result, index) => {
          if (result.status === 'fulfilled') {
            success += 1;
            const updated = result.value.data as AudioAsset | undefined;
            if (updated) {
              this.upsertAssetLocally(updated);
            } else {
              const existing = this.assets.find((item) => item.id === assetIds[index]);
              if (existing) {
                this.upsertAssetLocally({ ...existing, ...payload });
              }
            }
          }
        });
        if (success) {
          await this.persistAssetsToCache();
          await this.fetchAssets({ pagination: { page: this.assetPagination.page }, silent: true });
        }
        return { success, failed: assetIds.length - success };
      } finally {
        this.assetBulkLoading = false;
      }
    },

    async batchDeleteAssets(assetIds: string[]) {
      if (!assetIds?.length) {
        return { success: 0, failed: 0 };
      }
      this.assetBulkLoading = true;
      try {
        const tasks = assetIds.map((id) => api.delete(`/api/v1/audio/assets/${id}`));
        const results = await Promise.allSettled(tasks);
        let success = 0;
        results.forEach((result, index) => {
          if (result.status === 'fulfilled') {
            success += 1;
            this.removeAssetLocally(assetIds[index]);
          }
        });
        if (success) {
          this.assetPagination.total = Math.max(0, this.assetPagination.total - success);
        }
        await this.fetchAssets({ pagination: { page: this.assetPagination.page }, silent: false });
        return { success, failed: assetIds.length - success };
      } finally {
        this.assetBulkLoading = false;
      }
    },

    async persistAssetsToCache() {
      if (!this.assets.length) return;
      const lookup = this.folderPathLookup;
      try {
        const metas = this.assets.map((asset) => {
          const folderPath = asset.folderId ? lookup[asset.folderId] ?? '' : '';
          return toCachedMeta(asset, folderPath);
        });
        await audioDb.assets.bulkPut(metas);
      } catch (cacheErr) {
        console.warn('audio cache write skipped', cacheErr);
      }
    },

    async refreshLocalCacheWithFolderPaths() {
      await this.persistAssetsToCache();
    },

    async handleUpload(files: FileList | File[]) {
      if (!this.canManage) return;
      const list = Array.from(files);
      for (const file of list) {
        const task: UploadTaskState = {
          id: nanoid(),
          filename: file.name,
          size: file.size,
          progress: 0,
          status: 'pending',
        };
        this.uploadTasks.push(task);
        await this.uploadSingleFile(file, task);
      }
      try {
        await this.fetchAssets();
      } catch (err) {
        console.warn('refresh assets after upload failed', err);
      }
    },

    async uploadSingleFile(file: File, task: UploadTaskState) {
      try {
        task.status = 'uploading';
        const formData = new FormData();
        formData.append('file', file);
        const resp = await api.post('/api/v1/audio/assets/upload', formData, {
          headers: { 'Content-Type': 'multipart/form-data' },
          onUploadProgress: (e) => {
            if (e.total) {
              task.progress = Math.round((e.loaded / e.total) * 100);
            }
          },
        });
        task.status = resp.data?.needsTranscode ? 'transcoding' : 'success';
        task.progress = 100;
      } catch (err: any) {
        task.status = 'error';
        task.error = err?.response?.data?.message || '上传失败';
      }
    },

    removeUploadTask(taskId: string) {
      this.uploadTasks = this.uploadTasks.filter((task) => task.id !== taskId);
    },

    setNetworkMode(mode: AudioStudioState['networkMode']) {
      this.networkMode = mode;
    },

    setError(message: string | null) {
      this.error = message;
    },
  },
});
