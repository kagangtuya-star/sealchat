export type AudioTrackType = 'music' | 'ambience' | 'sfx';

export interface AudioAsset {
  id: string;
  name: string;
  folderId: string | null;
  size: number;
  duration: number;
  bitrate: number;
  storageType: 'local' | 's3';
  objectKey: string;
  description?: string;
  tags: string[];
  visibility: 'public' | 'restricted';
  createdBy: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AudioFolder {
  id: string;
  parentId: string | null;
  name: string;
  path: string;
  children?: AudioFolder[];
}

export interface AudioSceneTrack {
  type: AudioTrackType;
  assetId: string | null;
  volume: number;
  fadeIn: number;
  fadeOut: number;
}

export interface AudioScene {
  id: string;
  name: string;
  description?: string;
  tracks: AudioSceneTrack[];
  tags: string[];
  order: number;
  channelScope?: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface AudioSearchFilters {
  query: string;
  tags: string[];
  folderId: string | null;
  creatorIds: string[];
  durationRange: [number, number] | null;
  hasSceneOnly?: boolean;
}

export interface UploadTaskState {
  id: string;
  filename: string;
  size: number;
  progress: number;
  status: 'pending' | 'uploading' | 'transcoding' | 'success' | 'error';
  error?: string;
}

export interface AudioTrackStatePayload {
  type: AudioTrackType;
  assetId: string | null;
  volume: number;
  muted: boolean;
  solo: boolean;
  fadeIn: number;
  fadeOut: number;
}

export interface AudioPlaybackStatePayload {
  channelId: string;
  sceneId: string | null;
  tracks: AudioTrackStatePayload[];
  isPlaying: boolean;
  position: number;
  loopEnabled: boolean;
  playbackRate: number;
  updatedBy?: string;
  updatedAt?: string;
}
