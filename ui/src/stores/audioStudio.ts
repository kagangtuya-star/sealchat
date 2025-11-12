import { defineStore } from 'pinia';
import { Howl, Howler } from 'howler';
import { nanoid } from 'nanoid';
import { api, urlBase } from './_config';
import { useUserStore } from './user';
import { audioDb, toCachedMeta } from '@/models/audio-cache';
import type {
  AudioAsset,
  AudioFolder,
  AudioScene,
  AudioSceneTrack,
  AudioSearchFilters,
  AudioTrackType,
  AudioPlaybackStatePayload,
  AudioTrackStatePayload,
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
  currentSceneId: string | null;
  tracks: Record<AudioTrackType, TrackRuntime>;
  assets: AudioAsset[];
  filteredAssets: AudioAsset[];
  assetsLoading: boolean;
  folders: AudioFolder[];
  folderPathLookup: Record<string, string>;
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

const DEFAULT_TRACK_TYPES: AudioTrackType[] = ['music', 'ambience', 'sfx'];
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

function normalizeFolderId(input: string | null | undefined): string | null {
  if (input === undefined || input === null) return null;
  const trimmed = String(input).trim();
  if (!trimmed || trimmed === 'undefined' || trimmed === 'null') {
    return null;
  }
  return trimmed;
}

export const useAudioStudioStore = defineStore('audioStudio', {
  state: (): AudioStudioState => ({
    drawerVisible: false,
    initialized: false,
    activeTab: 'player',
    scenes: [],
    scenesLoading: false,
    currentSceneId: null,
    tracks: DEFAULT_TRACK_TYPES.reduce((acc, type) => {
      acc[type] = createEmptyTrack(type);
      return acc;
    }, {} as Record<AudioTrackType, TrackRuntime>),
    assets: [],
    filteredAssets: [],
    assetsLoading: false,
    folders: [],
    folderPathLookup: {},
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

    async fetchScenes() {
      try {
        this.scenesLoading = true;
        const resp = await api.get('/api/v1/audio/scenes');
        this.scenes = resp.data?.items || [];
      } catch (err) {
        console.error('fetchScenes failed', err);
        this.error = '无法加载音频场景';
      } finally {
        this.scenesLoading = false;
      }
    },

    async fetchAssets(payload?: Partial<AudioSearchFilters>) {
      this.assetsLoading = true;
      try {
        const params: Record<string, unknown> = { ...this.filters, ...(payload || {}) };
        const normalizedFolderId = normalizeFolderId(params.folderId as string | null | undefined);
        if (normalizedFolderId) {
          params.folderId = normalizedFolderId;
        } else {
          delete params.folderId;
        }
        const resp = await api.get('/api/v1/audio/assets', { params });
        this.assets = resp.data?.items || [];
        this.filteredAssets = this.assets;
        await this.persistAssetsToCache();
      } catch (err) {
        console.warn('fetchAssets failed, fallback to cache', err);
        const query = (payload?.query ?? this.filters.query ?? '').trim().toLowerCase();
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
      } finally {
        this.assetsLoading = false;
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

    selectTab(tab: AudioStudioState['activeTab']) {
      if (!this.canManage && tab !== 'player') {
        this.activeTab = 'player';
        return;
      }
      this.activeTab = tab;
    },

    async applyScene(sceneId: string) {
      const scene = this.scenes.find((item) => item.id === sceneId);
      if (!scene) return;
      this.currentSceneId = sceneId;
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const trackMeta = scene.tracks.find((t) => t.type === type) || createEmptyTrack(type);
        this.assignTrack(type, trackMeta);
      });
      if (this.isPlaying) {
        await this.playAll();
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
      const nextFolderId =
        filters.folderId !== undefined ? filters.folderId : this.filters.folderId;
      const mergedFilters: AudioSearchFilters = {
        ...this.filters,
        ...filters,
        folderId: normalizeFolderId(nextFolderId) ?? null,
      };
      this.filters = mergedFilters;
      await this.fetchAssets();
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
          await this.fetchAssets({ query: keyword });
        } catch (err) {
          console.warn('远程搜索失败', err);
        }
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
