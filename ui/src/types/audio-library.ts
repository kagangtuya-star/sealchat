import type { AudioLibraryMode } from '@/types/audio';

export interface AudioLibrarySettings {
  mode: AudioLibraryMode;
  prefix: string;
  selectorDepth: number;
  sourceId: string;
  s3Available: boolean;
  bucketLabel: string;
  canConfigure: boolean;
  version: number;
}

export interface AudioLibraryPrefix {
  ref: string;
  name: string;
  prefix: string;
}

export interface AudioLibraryAsset {
  ref: string;
  name: string;
  key: string;
  parentPrefix: string;
  size: number;
  lastModified: string;
  etag: string;
  storageClass?: string;
  extension: string;
  contentType?: string;
}

export interface AudioLibraryListResult {
  items?: AudioLibraryAsset[];
  prefixes?: AudioLibraryPrefix[];
  nextCursor?: string;
  isTruncated: boolean;
}

export interface AudioLibraryCapabilities {
  tags: boolean;
  description: boolean;
  duration: boolean;
  creator: boolean;
  worldScope: boolean;
  visibility: boolean;
  manualSort: boolean;
  fullTextSearch: boolean;
  globalSort: boolean;
}

export const databaseAudioLibraryCapabilities: AudioLibraryCapabilities = {
  tags: true,
  description: true,
  duration: true,
  creator: true,
  worldScope: true,
  visibility: true,
  manualSort: true,
  fullTextSearch: true,
  globalSort: true,
};

export const s3AudioLibraryCapabilities: AudioLibraryCapabilities = {
  tags: false,
  description: false,
  duration: false,
  creator: false,
  worldScope: false,
  visibility: false,
  manualSort: false,
  fullTextSearch: false,
  globalSort: false,
};
