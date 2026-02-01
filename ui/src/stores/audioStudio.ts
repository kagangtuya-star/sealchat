import { defineStore } from 'pinia';
import { Howl, Howler } from 'howler';
import { nanoid } from 'nanoid';
import { api, urlBase } from './_config';
import { useUserStore } from './user';
import { useUtilsStore } from './utils';
import { useChatStore } from './chat';
import { audioDb, toCachedMeta } from '@/models/audio-cache';
import { ensurePinyinLoaded, matchText } from '@/utils/pinyinMatch';
import type {
  AudioAsset,
  AudioAssetMutationPayload,
  AudioAssetQueryParams,
  AudioAssetScope,
  AudioFolder,
  AudioFolderPayload,
  AudioScene,
  AudioSceneInput,
  AudioSceneTrack,
  AudioSearchFilters,
  AudioTrackType,
  AudioImportPreview,
  AudioImportResult,
  AudioPlaybackStatePayload,
  AudioTrackStatePayload,
  PaginatedResult,
  PlaylistMode,
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
  playlistFolderId: string | null;
  playlistMode: PlaylistMode | null;
  playlistAssetIds: string[];
  playlistIndex: number;
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
  importPreview: AudioImportPreview | null;
  importPreviewLoading: boolean;
  importLoading: boolean;
  importResult: AudioImportResult | null;
  importError: string | null;
  networkMode: 'normal' | 'constrained' | 'minimal';
  bufferMessage: string;
  isPlaying: boolean;
  loopEnabled: boolean;
  playbackRate: number;
  masterVolume: number;
  worldPlaybackEnabled: boolean;
  error: string | null;
  currentChannelId: string | null;
  currentWorldId: string | null;
  remoteState: AudioPlaybackStatePayload | null;
  isApplyingRemoteState: boolean;
  pendingSyncHandle: number | null;
  pendingRemotePlay: boolean;
  interactionListenerBound: boolean;
}

export const DEFAULT_TRACK_TYPES: AudioTrackType[] = ['music', 'ambience', 'sfx'];
if (typeof window !== 'undefined' && typeof Howler !== 'undefined') {
  // 增加音频池大小以支持更多并发播放
  const desiredPool = 30;
  if ((Howler as typeof Howler & { html5PoolSize?: number }).html5PoolSize < desiredPool) {
    (Howler as typeof Howler & { html5PoolSize?: number }).html5PoolSize = desiredPool;
  }
  // 用户首次交互时解锁音频上下文
  const unlockAudio = () => {
    if (Howler.ctx?.state === 'suspended') {
      Howler.ctx.resume().catch(() => {});
    }
    document.removeEventListener('click', unlockAudio);
    document.removeEventListener('touchstart', unlockAudio);
    document.removeEventListener('keydown', unlockAudio);
  };
  document.addEventListener('click', unlockAudio, { once: true });
  document.addEventListener('touchstart', unlockAudio, { once: true });
  document.addEventListener('keydown', unlockAudio, { once: true });
}
let progressTimer: number | null = null;
let transcodeTimer: number | null = null;
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
    loopEnabled: true,
    playbackRate: 1,
    howl: null,
    status: 'idle',
    progress: 0,
    buffered: 0,
    duration: 0,
    muted: false,
    solo: false,
    pendingSeek: null,
    playlistFolderId: null,
    playlistMode: null,
    playlistAssetIds: [],
    playlistIndex: 0,
  };
}

function startProgressWatcher(store: ReturnType<typeof useAudioStudioStore>) {
  if (typeof window === 'undefined') return;
  if (progressTimer) return;
  progressTimer = window.setInterval(() => {
    store.updateProgressFromPlayers();
  }, 500);
}

function startTranscodeWatcher(store: ReturnType<typeof useAudioStudioStore>) {
  if (typeof window === 'undefined') return;
  if (transcodeTimer) return;
  transcodeTimer = window.setInterval(() => {
    store.refreshTranscodeTasks();
  }, 3000);
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
      loopEnabled: runtime.loopEnabled ?? true,
      playbackRate: runtime.playbackRate ?? 1,
      playlistFolderId: runtime.playlistFolderId || null,
      playlistMode: runtime.playlistMode || null,
      playlistAssetIds: runtime.playlistAssetIds || [],
      playlistIndex: runtime.playlistIndex || 0,
    } as AudioSceneTrack;
  });
}

function stopProgressWatcher() {
  if (typeof window === 'undefined') return;
  if (!progressTimer) return;
  window.clearInterval(progressTimer);
  progressTimer = null;
}

function stopTranscodeWatcher() {
  if (typeof window === 'undefined') return;
  if (!transcodeTimer) return;
  window.clearInterval(transcodeTimer);
  transcodeTimer = null;
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
  if (filters.scope) {
    params.scope = filters.scope;
  }
  if (filters.worldId) {
    params.worldId = filters.worldId;
  }
  if (filters.includeCommon !== undefined) {
    params.includeCommon = filters.includeCommon;
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
      scope: undefined,
      worldId: null,
      includeCommon: true,
    },
    uploadTasks: [],
    importPreview: null,
    importPreviewLoading: false,
    importLoading: false,
    importResult: null,
    importError: null,
    networkMode: 'normal',
    bufferMessage: '',
    isPlaying: false,
    loopEnabled: false,
    playbackRate: 1,
    masterVolume: 1,
    worldPlaybackEnabled: false,
    error: null,
    currentChannelId: null,
    currentWorldId: null,
    remoteState: null,
    isApplyingRemoteState: false,
    pendingSyncHandle: null,
    pendingRemotePlay: false,
    interactionListenerBound: false,
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
      return this.canManageCurrentWorld;
    },

    isSystemAdmin(): boolean {
      const user = useUserStore();
      return Boolean(user.checkPerm?.('mod_admin'));
    },

    canManageCurrentWorld(state): boolean {
      const user = useUserStore();
      if (user.checkPerm?.('mod_admin')) return true;
      const utils = useUtilsStore();
      if (!utils.config?.audio?.allowWorldAudioWorkbench) return false;
      if (!state.currentWorldId) return false;
      const chat = useChatStore();
      const worldDetail = chat.worldDetailMap?.[state.currentWorldId];
      const memberRole = worldDetail?.memberRole;
      const ownerId = worldDetail?.world?.ownerId || chat.worldMap?.[state.currentWorldId]?.ownerId;
      return memberRole === 'owner' || memberRole === 'admin' || ownerId === user.info.id;
    },

    ffmpegAvailable(): boolean {
      const utils = useUtilsStore();
      return utils.config?.ffmpegAvailable === true;
    },

    importEnabled(): boolean {
      const utils = useUtilsStore();
      return utils.config?.audioImportEnabled === true;
    },

    hasAnyTrackPlaying(): boolean {
      return DEFAULT_TRACK_TYPES.some((type) => {
        const track = this.tracks[type];
        return track?.status === 'playing';
      });
    },
  },

  actions: {
    setCurrentWorld(worldId: string | null) {
      this.currentWorldId = worldId;
      if (worldId) {
        this.filters.worldId = worldId;
        this.filters.includeCommon = true;
        if (!this.isSystemAdmin) {
          this.filters.scope = 'world';
        }
      } else {
        this.filters.worldId = null;
        this.filters.includeCommon = true;
        if (!this.isSystemAdmin) {
          this.filters.scope = undefined;
        }
      }
    },

    canEditAsset(asset: AudioAsset): boolean {
      if (this.isSystemAdmin) return true;
      if (asset.scope === 'common') return false;
      if (!this.canManageCurrentWorld) return false;
      return asset.worldId === this.currentWorldId;
    },

    canDeleteAsset(asset: AudioAsset): boolean {
      return this.canEditAsset(asset);
    },

    setActiveChannel(channelId: string | null) {
      this.ensureInteractionListener();
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
      if (this.worldPlaybackEnabled) {
        return;
      }
      this.fetchPlaybackState(channelId);
    },

    setWorldPlaybackEnabled(enabled: boolean) {
      if (this.worldPlaybackEnabled === enabled) {
        return;
      }
      this.worldPlaybackEnabled = enabled;
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    ensureInteractionListener() {
      if (this.interactionListenerBound || typeof document === 'undefined') {
        return;
      }
      const handler = () => {
        this.tryResumeRemotePlayback();
      };
      document.addEventListener('pointerdown', handler, { capture: true });
      document.addEventListener('keydown', handler, { capture: true });
      this.interactionListenerBound = true;
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
      if (payload && typeof payload.worldPlaybackEnabled === 'boolean') {
        this.worldPlaybackEnabled = payload.worldPlaybackEnabled;
      }
      const allowWorld = !!payload?.worldPlaybackEnabled;
      if (!this.currentChannelId && !allowWorld) {
        return;
      }
      if (payload && !allowWorld && payload.channelId !== this.currentChannelId) {
        return;
      }
      const user = useUserStore();
      if (payload && payload.updatedBy && payload.updatedBy === user.info.id) {
        return;
      }

      console.log('[AudioSync] Received remote state:', {
        isPlaying: payload?.isPlaying,
        tracks: payload?.tracks?.map(t => ({
          type: t.type,
          assetId: t.assetId,
          isPlaying: t.isPlaying,
          loopEnabled: t.loopEnabled,
          playbackRate: t.playbackRate,
          muted: t.muted,
        })),
      });

      this.remoteState = payload;
      if (!payload?.isPlaying) {
        this.pendingRemotePlay = false;
      }
      this.isApplyingRemoteState = true;

      if (!payload) {
        // 清空所有轨道
        stopProgressWatcher();
        DEFAULT_TRACK_TYPES.forEach((type) => {
          const track = this.tracks[type];
          if (track?.howl) {
            try {
              track.howl.stop();
              track.howl.unload();
            } catch (e) {
              console.warn('cleanup howl failed', e);
            }
          }
          this.tracks[type] = createEmptyTrack(type);
        });
        this.isPlaying = false;
        this.isApplyingRemoteState = false;
        return;
      }

      try {
        this.loopEnabled = payload.loopEnabled ?? this.loopEnabled;
        this.playbackRate = payload.playbackRate || 1;
        this.currentSceneId = payload.sceneId || null;
        const trackStates = payload.tracks || [];

        // 逐轨道增量更新
        await Promise.all(
          DEFAULT_TRACK_TYPES.map(async (type) => {
            const incoming = trackStates.find((t) => t.type === type);
            const current = this.tracks[type];

            // 无远程状态 -> 清空轨道
            if (!incoming || !incoming.assetId) {
              if (current?.howl) {
                current.howl.stop();
                current.howl.unload();
              }
              this.tracks[type] = createEmptyTrack(type);
              return;
            }

            const targetPosition = typeof incoming.position === 'number' ? incoming.position : (payload.position ?? 0);
            // 后端尚未支持轨道级 isPlaying，回退到全局 isPlaying
            const trackIsPlaying = typeof incoming.isPlaying === 'boolean' ? incoming.isPlaying : payload.isPlaying;
            const shouldPlay = trackIsPlaying && !incoming.muted;

            console.log(`[AudioSync] Track ${type} - shouldPlay: ${shouldPlay} (isPlaying: ${trackIsPlaying}, muted: ${incoming.muted})`);

            // 资源相同 -> 仅更新状态
            if (current?.assetId === incoming.assetId && current.howl) {
              current.volume = typeof incoming.volume === 'number' ? incoming.volume : current.volume;
              current.muted = incoming.muted ?? current.muted;
              current.solo = incoming.solo ?? current.solo;
              current.fadeIn = incoming.fadeIn ?? current.fadeIn;
              current.fadeOut = incoming.fadeOut ?? current.fadeOut;
              current.loopEnabled = incoming.loopEnabled ?? current.loopEnabled ?? true;
              current.playbackRate = incoming.playbackRate ?? current.playbackRate ?? 1;
              current.playlistFolderId = incoming.playlistFolderId ?? null;
              current.playlistMode = incoming.playlistMode ?? null;
              current.playlistAssetIds = incoming.playlistAssetIds ?? [];
              current.playlistIndex = incoming.playlistIndex ?? 0;

              // 应用音量、倍速、循环（使用轨道级设置）
              current.howl.volume(current.muted ? 0 : current.volume);
              current.howl.rate(current.playbackRate);
              current.howl.loop(current.loopEnabled);

              // 同步播放状态（防止重复播放）
              const isCurrentlyPlaying = current.howl.playing();
              if (shouldPlay && !isCurrentlyPlaying) {
                if (targetPosition > 0) {
                  current.howl.seek(targetPosition);
                }
                // 尝试恢复 AudioContext
                if (Howler.ctx?.state === 'suspended') {
                  await Howler.ctx.resume();
                }
                try {
                  current.howl.play();
                  console.log(`[AudioSync] Track ${type} playing (same asset)`);
                } catch (e) {
                  console.warn(`[AudioSync] Autoplay blocked for track ${type}. Click anywhere to start.`, e);
                  current.status = 'paused';
                  this.pendingRemotePlay = true;
                }
              } else if (!shouldPlay && isCurrentlyPlaying) {
                current.howl.pause();
              } else if (shouldPlay && isCurrentlyPlaying) {
                // 已在播放，仅同步位置（如果差距较大）
                const currentPos = current.howl.seek() as number;
                if (Math.abs(currentPos - targetPosition) > 2) {
                  current.howl.seek(targetPosition);
                }
              }
              return;
            }

            // 资源不同 -> 重建轨道
            if (current?.howl) {
              try {
                current.howl.stop();
                current.howl.unload();
              } catch (e) {
                console.warn(`Failed to unload track ${type}`, e);
              }
            }

            const track = createEmptyTrack(type);
            track.volume = typeof incoming.volume === 'number' ? incoming.volume : 0.8;
            track.muted = incoming.muted ?? false;
            track.solo = incoming.solo ?? false;
            track.fadeIn = incoming.fadeIn ?? 2000;
            track.fadeOut = incoming.fadeOut ?? 2000;
            track.loopEnabled = incoming.loopEnabled ?? true;
            track.playbackRate = incoming.playbackRate ?? 1;
            track.playlistFolderId = incoming.playlistFolderId ?? null;
            track.playlistMode = incoming.playlistMode ?? null;
            track.playlistAssetIds = incoming.playlistAssetIds ?? [];
            track.playlistIndex = incoming.playlistIndex ?? 0;
            track.assetId = incoming.assetId;
            track.status = 'loading';

            this.tracks[type] = track;

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
            track.howl = this.createHowlInstance(track, asset, { initialSeek: targetPosition });
            track.status = 'ready';

            if (shouldPlay && track.howl) {
              // 使用轨道级设置
              track.howl.loop(track.loopEnabled);
              track.howl.rate(track.playbackRate);
              // 尝试恢复 AudioContext
              if (Howler.ctx?.state === 'suspended') {
                await Howler.ctx.resume();
              }
              try {
                track.howl.play();
                console.log(`[AudioSync] Track ${type} playing (new asset)`);
              } catch (e) {
                console.warn(`[AudioSync] Autoplay blocked for new track ${type}. Click anywhere to start.`, e);
                track.status = 'paused';
                this.pendingRemotePlay = true;
              }
            }
          }),
        );

        // 更新全局播放状态（基于是否有任何轨道在播放）
        this.isPlaying = payload.isPlaying;
        if (this.anyTrackPlaying()) {
          startProgressWatcher(this);
        } else {
          stopProgressWatcher();
        }
      } finally {
        this.isApplyingRemoteState = false;
      }
    },

    tryResumeRemotePlayback() {
      if (!this.pendingRemotePlay) {
        return;
      }
      if (!this.remoteState?.isPlaying) {
        this.pendingRemotePlay = false;
        return;
      }
      const trackStates = this.remoteState.tracks || [];
      DEFAULT_TRACK_TYPES.forEach((type) => {
        const incoming = trackStates.find((t) => t.type === type);
        const track = this.tracks[type];
        if (!incoming || !incoming.assetId || !track?.howl) {
          return;
        }
        const trackIsPlaying =
          typeof incoming.isPlaying === 'boolean' ? incoming.isPlaying : this.remoteState?.isPlaying;
        const shouldPlay = trackIsPlaying && !incoming.muted;
        if (!shouldPlay) {
          return;
        }
        if (!track.howl.playing()) {
          const targetPosition =
            typeof incoming.position === 'number' ? incoming.position : (this.remoteState?.position ?? 0);
          if (targetPosition > 0) {
            track.howl.seek(targetPosition);
          }
          if (Howler.ctx?.state === 'suspended') {
            Howler.ctx.resume().catch(() => {});
          }
          try {
            track.howl.play();
          } catch (err) {
            console.warn(`[AudioSync] Autoplay retry failed for track ${type}`, err);
          }
        }
      });
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

      console.log('[AudioSync] Sending state:', {
        isPlaying: payload.isPlaying,
        tracks: payload.tracks.map(t => ({
          type: t.type,
          assetId: t.assetId,
          isPlaying: t.isPlaying,
          loopEnabled: t.loopEnabled,
          playbackRate: t.playbackRate,
        })),
      });

      try {
        await api.post('/api/v1/audio/state', payload);
      } catch (err) {
        console.warn('同步音频状态失败', err);
      }
    },

    serializePlaybackState() {
      if (!this.currentChannelId) return null;
      // 使用实际播放状态而不是 this.isPlaying，避免状态不同步
      const actuallyPlaying = this.anyTrackPlaying();
      return {
        channelId: this.currentChannelId,
        sceneId: this.currentSceneId,
        tracks: this.buildTrackStatePayload(),
        isPlaying: actuallyPlaying,
        position: this.estimatePlaybackPosition(),
        loopEnabled: this.loopEnabled,
        playbackRate: this.playbackRate,
        worldPlaybackEnabled: this.worldPlaybackEnabled,
      };
    },

    buildTrackStatePayload(): AudioTrackStatePayload[] {
      return DEFAULT_TRACK_TYPES.map((type) => {
        const track = this.tracks[type] || createEmptyTrack(type);
        const trackIsPlaying = Boolean(
          track.assetId &&
            !track.muted &&
            (track.status === 'playing' || (this.isPlaying && track.status !== 'paused')),
        );
        const trackPosition = track.howl ? (track.howl.seek() as number) : 0;
        return {
          type,
          assetId: track.assetId,
          volume: track.volume,
          muted: track.muted,
          solo: track.solo,
          fadeIn: track.fadeIn,
          fadeOut: track.fadeOut,
          isPlaying: trackIsPlaying,
          position: typeof trackPosition === 'number' ? trackPosition : 0,
          loopEnabled: track.loopEnabled ?? true,
          playbackRate: track.playbackRate ?? 1,
          playlistFolderId: track.playlistFolderId || null,
          playlistMode: track.playlistMode || null,
          playlistAssetIds: track.playlistAssetIds || [],
          playlistIndex: track.playlistIndex || 0,
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
          scope: 'common',
          worldId: null,
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
        const params: Record<string, unknown> = {};
        if (!this.isSystemAdmin && !this.filters.worldId) {
          return;
        }
        if (this.filters.scope) {
          params.scope = this.filters.scope;
        }
        if (this.filters.worldId) {
          params.worldId = this.filters.worldId;
        }
        if (this.filters.includeCommon !== undefined) {
          params.includeCommon = this.filters.includeCommon;
        }
        const resp = await api.get('/api/v1/audio/folders', { params });
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
        const effectivePayload = { ...payload };
        if (!effectivePayload.scope) {
          if (this.filters.scope) {
            effectivePayload.scope = this.filters.scope;
          } else if (!this.isSystemAdmin) {
            effectivePayload.scope = 'world';
          } else {
            effectivePayload.scope = 'common';
          }
        }
        if (effectivePayload.scope === 'world' && !effectivePayload.worldId) {
          effectivePayload.worldId = this.filters.worldId ?? this.currentWorldId ?? undefined;
        }
        await api.post('/api/v1/audio/folders', effectivePayload);
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
          // 仅启动进度监控，不更新全局 isPlaying（由 playAll/playTrack 显式控制）
          if (!this.isApplyingRemoteState) {
            startProgressWatcher(this);
          }
          this.pendingRemotePlay = false;
        },
        onpause: () => {
          track.status = 'paused';
        },
        onstop: () => {
          track.status = 'ready';
        },
        onend: () => {
          track.status = 'ready';
          // 播放列表模式下自动播放下一曲
          if (track.playlistMode && track.playlistAssetIds?.length && !this.isApplyingRemoteState) {
            this.playNextInPlaylist(track.type);
            return;
          }
          // 仅当所有轨道都空闲时停止进度监控
          if (!this.isApplyingRemoteState && this.allTracksIdle()) {
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
          // 自动播放被阻止时，设置为暂停状态而非错误状态
          // 用户可以通过点击播放按钮手动触发
          console.warn('Play error (likely autoplay blocked):', err);
          track.status = 'paused';
          if (!this.canManage && this.remoteState?.isPlaying) {
            this.pendingRemotePlay = true;
          }
          // 尝试解锁音频上下文
          if (typeof Howler !== 'undefined' && Howler.ctx?.state === 'suspended') {
            Howler.ctx.resume().catch(() => {});
          }
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
          // 使用轨道级别的循环和倍速设置
          track.howl.loop(track.loopEnabled ?? true);
          track.howl.rate(track.playbackRate ?? 1);
          // 防止重复播放
          if (!track.howl.playing()) {
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

    seekTrack(type: AudioTrackType, position: number) {
      const track = this.tracks[type];
      if (!track) return;
      const target = Math.max(0, position);
      if (track.howl) {
        track.howl.seek(target);
      } else {
        track.pendingSeek = target;
      }
      this.updateProgressFromPlayers();
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    playTrack(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track?.howl || !track.assetId) return;
      // 防止重复播放：如果已经在播放，直接返回
      if (track.howl.playing()) return;
      // 使用轨道级别的循环和倍速设置
      track.howl.loop(track.loopEnabled ?? true);
      track.howl.rate(track.playbackRate ?? 1);
      track.howl.play();
      // 启动进度监控（如果未启动）
      startProgressWatcher(this);
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    pauseTrack(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track?.howl) return;
      if (track.howl.playing()) {
        track.howl.pause();
      }
      // 仅当所有轨道都空闲时才停止进度监控
      if (this.allTracksIdle()) {
        stopProgressWatcher();
      }
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    toggleTrackPlay(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track?.howl) return;
      if (track.howl.playing()) {
        this.pauseTrack(type);
      } else {
        this.playTrack(type);
      }
    },

    setTrackFadeIn(type: AudioTrackType, value: number) {
      const track = this.tracks[type];
      if (!track) return;
      track.fadeIn = value;
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    setTrackFadeOut(type: AudioTrackType, value: number) {
      const track = this.tracks[type];
      if (!track) return;
      track.fadeOut = value;
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    setTrackLoop(type: AudioTrackType, enabled: boolean) {
      const track = this.tracks[type];
      if (!track) return;
      track.loopEnabled = enabled;
      if (track.howl) {
        track.howl.loop(enabled);
      }
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    toggleTrackLoop(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track) return;
      this.setTrackLoop(type, !(track.loopEnabled ?? true));
    },

    setTrackPlaybackRate(type: AudioTrackType, rate: number) {
      const track = this.tracks[type];
      if (!track) return;
      track.playbackRate = rate;
      if (track.howl) {
        track.howl.rate(rate);
      }
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    setMasterVolume(volume: number) {
      this.masterVolume = Math.max(0, Math.min(1, volume));
      Howler.volume(this.masterVolume);
    },

    clearTrack(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track) return;
      if (track.howl) {
        track.howl.unload();
      }
      this.tracks[type] = createEmptyTrack(type);
      if (this.allTracksIdle()) {
        this.isPlaying = false;
        stopProgressWatcher();
      }
      if (this.canManage) {
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
        return !track || track.status === 'idle' || track.status === 'ready' || track.status === 'paused';
      });
    },

    anyTrackPlaying() {
      return DEFAULT_TRACK_TYPES.some((type) => {
        const track = this.tracks[type];
        return track?.howl?.playing();
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
      await this.fetchFolders();
      await this.fetchAssets({ pagination: { page: 1 } });
    },

    async searchAssetsLocally(keyword: string) {
      this.filters.query = keyword;
      if (!keyword.trim()) {
        this.filteredAssets = this.assets;
        return;
      }
      const normalizedKeyword = keyword.trim();
      const loadPromise = ensurePinyinLoaded();
      const lookup = this.folderPathLookup;
      const applyLocalFilter = () => {
        this.filteredAssets = this.assets.filter((asset) => {
          const folderPath = asset.folderId ? lookup[asset.folderId] ?? '' : '';
          const description = asset.description ?? '';
          const targets = [
            asset.name,
            asset.tags.join(' '),
            asset.createdBy,
            folderPath,
            description,
          ];
          return targets.some((target) => matchText(normalizedKeyword, target || ''));
        });
      };
      applyLocalFilter();
      void loadPromise.then((loaded) => {
        if (!loaded) return;
        if (this.filters.query !== keyword) return;
        applyLocalFilter();
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
      if ((payload.scope || payload.worldId !== undefined) && !this.isSystemAdmin) {
        throw new Error('无权限调整素材级别');
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

    async handleUpload(
      files: FileList | File[],
      options?: { scope?: AudioAssetScope; worldId?: string; folderId?: string | null }
    ) {
      if (!this.canManage && !this.canManageCurrentWorld) return;
      const uploadScope = options?.scope ?? (this.isSystemAdmin ? 'common' : 'world');
      const uploadWorldId = options?.worldId ?? (uploadScope === 'world' ? this.currentWorldId : null);
      const uploadFolderId = normalizeFolderId(options?.folderId) ?? null;
      const list = Array.from(files);
      const tasks: UploadTaskState[] = [];
      for (const file of list) {
        const task: UploadTaskState = {
          id: nanoid(),
          filename: file.name,
          size: file.size,
          progress: 0,
          status: 'pending',
          retryCount: 0,
          createdAt: Date.now(),
        };
        tasks.push(task);
        this.uploadTasks.push(task);
      }
      const concurrency = 2;
      const uploadTask = async (file: File, task: UploadTaskState) => {
        await this.uploadSingleFile(file, task, {
          scope: uploadScope,
          worldId: uploadWorldId ?? undefined,
          folderId: uploadFolderId ?? undefined,
        });
      };
      const queue = list.map((file, i) => ({ file, task: tasks[i] }));
      const running: Promise<void>[] = [];
      for (const item of queue) {
        const promise = uploadTask(item.file, item.task).finally(() => {
          const idx = running.indexOf(promise);
          if (idx >= 0) running.splice(idx, 1);
        });
        running.push(promise);
        if (running.length >= concurrency) {
          await Promise.race(running);
        }
      }
      await Promise.all(running);
      try {
        await this.fetchAssets();
      } catch (err) {
        console.warn('refresh assets after upload failed', err);
      }
    },

    async uploadSingleFile(
      file: File,
      task: UploadTaskState,
      options?: { scope?: AudioAssetScope; worldId?: string; folderId?: string }
    ) {
      const maxRetries = 2;
      const doUpload = async (): Promise<boolean> => {
        try {
          task.status = 'uploading';
          task.error = undefined;
          task.progress = 0;
          const formData = new FormData();
          formData.append('file', file);
          if (options?.scope) {
            formData.append('scope', options.scope);
          }
          if (options?.worldId) {
            formData.append('worldId', options.worldId);
          }
          if (options?.folderId) {
            formData.append('folderId', options.folderId);
          }
          const resp = await api.post('/api/v1/audio/assets/upload', formData, {
            headers: { 'Content-Type': 'multipart/form-data' },
          });
          const serverStatus = resp.data?.status;
          const assetId = resp.data?.item?.id;
          if (assetId) {
            task.assetId = assetId;
          }
          if (serverStatus === 'processing') {
            task.status = 'transcoding';
            startTranscodeWatcher(this);
          } else if (serverStatus === 'failed') {
            task.status = 'error';
            task.error = '转码失败';
          } else if (serverStatus) {
            task.status = 'success';
          } else {
            task.status = resp.data?.needsTranscode ? 'transcoding' : 'success';
          }
          task.progress = 100;
          return true;
        } catch (err: any) {
          const status = err?.response?.status;
          const isRetryable = !status || status >= 500 || status === 429;
          if (isRetryable && (task.retryCount ?? 0) < maxRetries) {
            task.retryCount = (task.retryCount ?? 0) + 1;
            task.progress = 0;
            await new Promise((r) => setTimeout(r, 1000 * task.retryCount!));
            return doUpload();
          }
          task.status = 'error';
          task.error = err?.response?.data?.message || err?.message || '上传失败';
          return false;
        }
      };
      await doUpload();
    },

    async previewImport() {
      if (!this.canManage) return null;
      if (!this.importEnabled) {
        this.importError = '未启用导入目录';
        return null;
      }
      this.importPreviewLoading = true;
      this.importError = null;
      this.importResult = null;
      try {
        const resp = await api.get('/api/v1/audio/assets/import/preview');
        this.importPreview = resp.data as AudioImportPreview;
        return this.importPreview;
      } catch (err: any) {
        this.importError = err?.response?.data?.message || err?.message || '读取导入目录失败';
        this.importPreview = null;
        return null;
      } finally {
        this.importPreviewLoading = false;
      }
    },

    async importFromDir(options: { all: boolean; paths?: string[]; scope?: AudioAssetScope; worldId?: string }) {
      if (!this.canManage) return null;
      if (!this.importEnabled) {
        this.importError = '未启用导入目录';
        return null;
      }
      this.importLoading = true;
      this.importError = null;
      this.importResult = null;
      try {
        const resp = await api.post('/api/v1/audio/assets/import', {
          all: options.all,
          paths: options.paths || [],
          scope: options.scope,
          worldId: options.worldId,
        });
        this.importResult = resp.data as AudioImportResult;
        try {
          await this.fetchAssets();
        } catch (err) {
          console.warn('refresh assets after import failed', err);
        }
        return this.importResult;
      } catch (err: any) {
        this.importError = err?.response?.data?.message || err?.message || '导入失败';
        return null;
      } finally {
        this.importLoading = false;
      }
    },

    async refreshTranscodeTasks() {
      const tasks = this.uploadTasks.filter((task) => task.status === 'transcoding' && task.assetId);
      if (!tasks.length) {
        stopTranscodeWatcher();
        return;
      }
      const results = await Promise.allSettled(tasks.map((task) => this.fetchSingleAsset(task.assetId!)));
      results.forEach((result, index) => {
        if (result.status !== 'fulfilled') return;
        const asset = result.value;
        const task = tasks[index];
        if (asset.transcodeStatus === 'ready') {
          task.status = 'success';
          task.progress = 100;
        } else if (asset.transcodeStatus === 'failed') {
          task.status = 'error';
          task.error = '转码失败';
        }
      });
    },

    removeUploadTask(taskId: string) {
      this.uploadTasks = this.uploadTasks.filter((task) => task.id !== taskId);
    },

    clearCompletedUploadTasks() {
      this.uploadTasks = this.uploadTasks.filter((task) => task.status !== 'success' && task.status !== 'error');
    },

    clearAllUploadTasks() {
      this.uploadTasks = [];
      stopTranscodeWatcher();
    },

    retryFailedUploadTask(taskId: string, file: File, options?: { scope?: AudioAssetScope; worldId?: string }) {
      const task = this.uploadTasks.find((t) => t.id === taskId);
      if (!task || task.status !== 'error') return;
      task.retryCount = 0;
      task.progress = 0;
      task.error = undefined;
      this.uploadSingleFile(file, task, options);
    },

    setNetworkMode(mode: AudioStudioState['networkMode']) {
      this.networkMode = mode;
    },

    setError(message: string | null) {
      this.error = message;
    },

    async setTrackPlaylistFolder(type: AudioTrackType, folderId: string | null) {
      const track = this.tracks[type];
      if (!track) return;
      track.playlistFolderId = folderId;
      if (!folderId) {
        track.playlistAssetIds = [];
        track.playlistIndex = 0;
        return;
      }
      try {
        const resp = await api.get('/api/v1/audio/assets', {
          params: { folderId, pageSize: 200 },
        });
        const raw = resp.data as PaginatedResult<AudioAsset> | AudioAsset[] | undefined;
        const items = Array.isArray(raw) ? raw : raw?.items || [];
        track.playlistAssetIds = items.map((a) => a.id);
        track.playlistIndex = 0;
        if (items.length && !track.assetId) {
          await this.assignAssetToTrack(type, items[0]);
        }
      } catch (err) {
        console.warn('fetch folder assets for playlist failed', err);
      }
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    setTrackPlaylistMode(type: AudioTrackType, mode: PlaylistMode | null) {
      const track = this.tracks[type];
      if (!track) return;
      track.playlistMode = mode;
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    async playNextInPlaylist(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track || !track.playlistAssetIds?.length) return;
      const mode = track.playlistMode;
      if (mode === 'single') {
        if (track.howl) {
          track.howl.seek(0);
          track.howl.play();
        }
        return;
      }
      let nextIndex = track.playlistIndex;
      if (mode === 'shuffle') {
        nextIndex = Math.floor(Math.random() * track.playlistAssetIds.length);
      } else {
        nextIndex = (track.playlistIndex + 1) % track.playlistAssetIds.length;
      }
      track.playlistIndex = nextIndex;
      const nextAssetId = track.playlistAssetIds[nextIndex];
      if (!nextAssetId) return;
      let asset = this.assets.find((a) => a.id === nextAssetId);
      if (!asset) {
        try {
          asset = await this.fetchSingleAsset(nextAssetId);
        } catch {
          return;
        }
      }
      await this.assignAssetToTrack(type, asset);
      if (this.isPlaying && track.howl && !track.muted) {
        track.howl.play();
      }
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    async playPrevInPlaylist(type: AudioTrackType) {
      const track = this.tracks[type];
      if (!track || !track.playlistAssetIds?.length) return;
      const mode = track.playlistMode;
      if (mode === 'single') {
        if (track.howl) {
          track.howl.seek(0);
        }
        return;
      }
      let prevIndex = track.playlistIndex;
      if (mode === 'shuffle') {
        prevIndex = Math.floor(Math.random() * track.playlistAssetIds.length);
      } else {
        prevIndex = (track.playlistIndex - 1 + track.playlistAssetIds.length) % track.playlistAssetIds.length;
      }
      track.playlistIndex = prevIndex;
      const prevAssetId = track.playlistAssetIds[prevIndex];
      if (!prevAssetId) return;
      let asset = this.assets.find((a) => a.id === prevAssetId);
      if (!asset) {
        try {
          asset = await this.fetchSingleAsset(prevAssetId);
        } catch {
          return;
        }
      }
      await this.assignAssetToTrack(type, asset);
      if (this.isPlaying && track.howl && !track.muted) {
        track.howl.play();
      }
      if (this.canManage) {
        this.queuePlaybackSync();
      }
    },

    getPlaylistModeLabel(mode: PlaylistMode | null): string {
      switch (mode) {
        case 'single':
          return '单曲循环';
        case 'sequential':
          return '顺序播放';
        case 'shuffle':
          return '随机播放';
        default:
          return '无';
      }
    },
  },
});
