export type AudioTrackType = 'music' | 'ambience' | 'sfx';
export type AudioAssetScope = 'common' | 'world';

export interface AudioAsset {
  id: string;
  name: string;
  folderId: string | null;
  size: number;
  duration: number;
  bitrate: number;
  storageType: 'local' | 's3';
  objectKey: string;
  transcodeStatus?: 'pending' | 'ready' | 'failed';
  description?: string;
  tags: string[];
  visibility: 'public' | 'restricted';
  createdBy: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
  lastAccessedAt?: string | null;
  accessCount?: number;
  sortOrder?: number;
  manualSorted?: boolean;
  scope: AudioAssetScope;
  worldId?: string | null;
}

export interface AudioAssetUsageSummary {
  sceneRefCount: number;
  playbackStateRefCount: number;
  sceneNames?: string[];
  playbackScopeLabels?: string[];
  referenced: boolean;
}

export interface AudioPlayableStreamResponse {
  streamUrl: string;
  expiresAt: number;
}

export interface AudioQuotaSummary {
  limited: boolean;
  quotaBytes: number | null;
  usedBytes: number;
  remainingBytes: number | null;
  usagePercent: number | null;
  source: 'default' | 'override' | 'admin-unlimited';
}

export interface AudioAssetListResult {
  items: AudioAsset[];
  page: number;
  pageSize: number;
  total: number;
  quota?: AudioQuotaSummary | null;
}

export interface AdminAudioFilterOption {
  label: string;
  value: string;
}

export interface AdminAudioAssetItem extends AudioAsset {
  worldName?: string;
  creatorName?: string;
  accessCount: number;
  usageSummary: AudioAssetUsageSummary;
  safeToDelete: boolean;
}

export interface AdminAudioAssetListResult {
  items: AdminAudioAssetItem[];
  page: number;
  pageSize: number;
  total: number;
  worldOptions: AdminAudioFilterOption[];
  creatorOptions: AdminAudioFilterOption[];
}

export interface AudioManageAssetListResult extends AdminAudioAssetListResult {
  quota?: AudioQuotaSummary | null;
}

export interface AdminAudioQuotaItem {
  userId: string;
  username: string;
  nickname: string;
  hasOverride: boolean;
  quotaMB: number;
  usedBytes: number;
  limited: boolean;
  quotaBytes: number | null;
  remainingBytes: number | null;
  usagePercent: number | null;
  source: 'default' | 'override' | 'admin-unlimited';
  updatedBy?: string;
}

export interface AdminAudioQuotaListResult {
  items: AdminAudioQuotaItem[];
  page: number;
  pageSize: number;
  total: number;
}

export interface AdminAudioCleanupPreview {
  thresholdBefore: string;
  totalCandidates: number;
  safeCandidates: number;
  referencedSkipped: number;
  directDeleteCandidates?: number;
  detachThenDeleteCandidates?: number;
  items: AdminAudioAssetItem[];
}

export interface AudioDeleteImpact {
  detachedSceneCount: number;
  detachedPlaybackStateCount: number;
  sceneNames?: string[];
  playbackScopeLabels?: string[];
}

export interface AudioDeleteResult {
  message?: string;
  impact?: AudioDeleteImpact | null;
}

export interface AudioDeleteConflictPayload {
  message?: string;
  usage?: AudioAssetUsageSummary;
}

export interface AudioBulkDeleteFailure {
  assetId: string;
  reason: string;
  usageSummary?: AudioAssetUsageSummary;
}

export interface AudioBulkDeleteResult {
  successIds: string[];
  failed: AudioBulkDeleteFailure[];
  successCount: number;
  failedCount: number;
  detachedSceneCount?: number;
  detachedPlaybackStateCount?: number;
  detachedReferencedAssetCount?: number;
  playbackScopeLabels?: string[];
}

export interface AudioAssetBatchDeleteSummary {
  success: number;
  failed: number;
  failures: AudioBulkDeleteFailure[];
}

export interface AudioAssetMutationPayload {
  name?: string;
  description?: string;
  tags?: string[];
  visibility?: 'public' | 'restricted';
  folderId?: string | null;
  scope?: AudioAssetScope;
  worldId?: string | null;
}

export interface AudioAssetQueryParams extends Partial<AudioSearchFilters> {
  page?: number;
  pageSize?: number;
  durationMin?: number;
  durationMax?: number;
}

export interface PaginatedResult<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

export interface AudioFolder {
  id: string;
  parentId: string | null;
  name: string;
  path: string;
  children?: AudioFolder[];
  scope: AudioAssetScope;
  worldId?: string | null;
  createdBy?: string;
  updatedBy?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface AudioFolderPayload {
  name: string;
  parentId?: string | null;
  scope?: AudioAssetScope;
  worldId?: string | null;
}

export type PlaylistMode = 'single' | 'sequential' | 'shuffle';

export interface AudioSceneTrack {
  type: AudioTrackType;
  assetId: string | null;
  volume: number;
  fadeIn: number;
  fadeOut: number;
  loopEnabled?: boolean;
  playbackRate?: number;
  playlistFolderId?: string | null;
  playlistMode?: PlaylistMode | null;
  playlistAssetIds?: string[];
  playlistIndex?: number;
}

export interface AudioScene {
  id: string;
  name: string;
  description?: string;
  tracks: AudioSceneTrack[];
  tags: string[];
  order: number;
  channelScope?: string | null;
  folderId?: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  scope: AudioAssetScope;
  worldId?: string | null;
}

export interface AudioSceneInput {
  name: string;
  description?: string;
  tags?: string[];
  tracks: AudioSceneTrack[];
  order?: number;
  channelScope?: string | null;
  folderId?: string | null;
  scope?: AudioAssetScope;
  worldId?: string | null;
}

export interface AudioSearchFilters {
  query: string;
  tags: string[];
  folderId: string | null;
  creatorIds: string[];
  durationRange: [number, number] | null;
  hasSceneOnly?: boolean;
  sortBy?: 'name' | 'scope' | 'duration' | 'updatedAt';
  sortOrder?: 'asc' | 'desc';
  manualSort?: boolean;
  scope?: AudioAssetScope;
  worldId?: string | null;
  includeCommon?: boolean;
}

export interface UploadTaskState {
  id: string;
  assetId?: string;
  filename: string;
  size: number;
  progress: number;
  status: 'pending' | 'uploading' | 'transcoding' | 'success' | 'error';
  error?: string;
  retryCount?: number;
  createdAt?: number;
}

export interface AudioImportPreviewItem {
  path: string;
  name: string;
  size: number;
  modTime: number;
  mimeType?: string;
  valid: boolean;
  reason?: string;
}

export interface AudioImportPreview {
  items: AudioImportPreviewItem[];
  total: number;
  valid: number;
  invalid: number;
}

export interface AudioImportDirectoryNode {
  path: string;
  name: string;
  children?: AudioImportDirectoryNode[];
}

export interface AudioImportBrowseResult extends AudioImportPreview {
  tree: AudioImportDirectoryNode[];
  currentPath: string;
}

export interface AudioImportResultItem {
  path: string;
  name?: string;
  assetId?: string;
  error?: string;
  reason?: string;
  warning?: string;
}

export interface AudioImportResult {
  imported: AudioImportResultItem[];
  failed: AudioImportResultItem[];
  skipped: AudioImportResultItem[];
}

export interface AudioImportJobStatus {
  jobId: string;
  status: 'pending' | 'running' | 'done' | 'failed';
  directory: string;
  totalFiles: number;
  processedFiles: number;
  importedCount: number;
  skippedCount: number;
  failedCount: number;
  errorMessage?: string;
  percentage: number;
  imported: AudioImportResultItem[];
  failed: AudioImportResultItem[];
  skipped: AudioImportResultItem[];
  startedAt?: string;
  finishedAt?: string;
}

export interface AudioTrackStatePayload {
  type: AudioTrackType;
  assetId: string | null;
  volume: number;
  muted: boolean;
  solo: boolean;
  fadeIn: number;
  fadeOut: number;
  isPlaying: boolean;
  position: number;
  loopEnabled: boolean;
  playbackRate: number;
  playlistFolderId?: string | null;
  playlistMode?: PlaylistMode | null;
  playlistAssetIds?: string[];
  playlistIndex?: number;
}

export interface AudioPlaybackStatePayload {
  channelId: string;
  sceneId: string | null;
  tracks: AudioTrackStatePayload[];
  isPlaying: boolean;
  position: number;
  basePositionSec?: number;
  capturedAtMs?: number;
  loopEnabled: boolean;
  playbackRate: number;
  worldPlaybackEnabled?: boolean;
  revision?: number;
  updatedBy?: string;
  updatedAt?: number | string;
  scopeType?: 'channel' | 'world' | string;
  scopeId?: string;
}
