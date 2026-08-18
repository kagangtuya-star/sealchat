import { api } from '@/stores/_config';
import type { AudioAsset } from '@/types/audio';
import type {
  AudioLibraryAsset,
  AudioLibraryListResult,
  AudioLibraryPrefix,
  AudioLibrarySettings,
} from '@/types/audio-library';

export type AudioLibraryAssetRecord = AudioLibraryAsset | AudioAsset;

export interface AudioLibraryAssetMutation {
  targetPrefix?: string;
  expectedEtag?: string;
  contentType?: string;
}

export interface AudioLibraryProvider {
  readonly mode: 'database' | 's3';
  listFolders(prefix?: string, cursor?: string, limit?: number): Promise<AudioLibraryListResult & { prefixes: AudioLibraryPrefix[] }>;
  listAssets(prefix?: string, cursor?: string, limit?: number): Promise<AudioLibraryListResult & { items: AudioLibraryAsset[] }>;
  resolveAssets(refs: string[]): Promise<AudioLibraryAssetRecord[]>;
  uploadAsset(file: Blob, prefix?: string): Promise<AudioLibraryAssetRecord>;
  createFolder(prefix: string): Promise<AudioLibraryPrefix>;
  renameAsset(ref: string, name: string, options?: AudioLibraryAssetMutation): Promise<AudioLibraryAssetRecord>;
  moveAsset(ref: string, targetPrefix: string, name?: string, expectedEtag?: string): Promise<AudioLibraryAssetRecord>;
  deleteAsset(ref: string, options?: { expectedEtag?: string; forceDetach?: boolean }): Promise<void>;
  deleteFolder(prefix: string): Promise<void>;
  getPlayToken(ref: string): Promise<{ streamUrl: string; expiresAt: number }>;
}

export class S3AudioLibraryProvider implements AudioLibraryProvider {
  readonly mode = 's3' as const;
  constructor(private readonly worldId?: string | null) {}
  async listFolders(prefix = '', cursor = '', limit = 100) {
    const { data } = await api.get('/api/v1/audio/library/s3/prefixes', { params: { worldId: this.worldId || undefined, prefix, cursor, limit } });
    return data as AudioLibraryListResult & { prefixes: AudioLibraryPrefix[] };
  }
  async listAssets(prefix = '', cursor = '', limit = 100) {
    const { data } = await api.get('/api/v1/audio/library/assets', { params: { worldId: this.worldId || undefined, prefix, cursor, limit } });
    return data as AudioLibraryListResult & { items: AudioLibraryAsset[] };
  }
  async resolveAssets(refs: string[]) {
    const { data } = await api.post('/api/v1/audio/library/assets/resolve', { worldId: this.worldId || undefined, refs });
    return (data?.items || []) as AudioLibraryAsset[];
  }
  async uploadAsset(file: Blob, prefix = '') {
    const form = new FormData();
    form.append('file', file);
    form.append('prefix', prefix);
    const { data } = await api.post('/api/v1/audio/library/upload', form, { params: { worldId: this.worldId || undefined }, headers: { 'Content-Type': 'multipart/form-data' } });
    return data.item as AudioLibraryAsset;
  }
  async createFolder(prefix: string) {
    const { data } = await api.post('/api/v1/audio/library/folders', { prefix }, { params: { worldId: this.worldId || undefined } });
    return data.item as AudioLibraryPrefix;
  }
  async renameAsset(ref: string, name: string, options: AudioLibraryAssetMutation = {}) {
    const { data } = await api.patch('/api/v1/audio/library/assets', { ref, name, ...options }, { params: { worldId: this.worldId || undefined } });
    return data.item as AudioLibraryAsset;
  }
  async moveAsset(ref: string, targetPrefix: string, name?: string, expectedEtag?: string) {
    const { data } = await api.patch('/api/v1/audio/library/assets', { ref, targetPrefix, name, expectedEtag }, { params: { worldId: this.worldId || undefined } });
    return data.item as AudioLibraryAsset;
  }
  async deleteAsset(ref: string, options: { expectedEtag?: string; forceDetach?: boolean } = {}) {
    await api.delete('/api/v1/audio/library/assets', { params: { worldId: this.worldId || undefined }, data: { ref, ...options } });
  }
  async deleteFolder(prefix: string) {
    await api.delete('/api/v1/audio/library/folders', { params: { worldId: this.worldId || undefined }, data: { prefix } });
  }
  async getPlayToken(ref: string) {
    const { data } = await api.post('/api/v1/audio/library/play-token', { worldId: this.worldId || undefined, ref });
    return data as { streamUrl: string; expiresAt: number };
  }
}

export class DatabaseAudioLibraryProvider implements AudioLibraryProvider {
  readonly mode = 'database' as const;
  async listFolders() {
    const { data } = await api.get('/api/v1/audio/folders');
    return { prefixes: [], items: data?.items || [], isTruncated: false } as AudioLibraryListResult & { prefixes: AudioLibraryPrefix[] };
  }
  async listAssets(_prefix = '', _cursor = '', limit = 200) {
    const { data } = await api.get('/api/v1/audio/assets', { params: { page: 1, pageSize: limit } });
    return { ...(data || {}), items: data?.items || [], prefixes: [] } as AudioLibraryListResult & { items: AudioLibraryAsset[] };
  }
  async resolveAssets(refs: string[]) {
    const items = await Promise.all(refs.map(async (ref) => {
      const { data } = await api.get(`/api/v1/audio/assets/${encodeURIComponent(ref)}`);
      return data as AudioAsset;
    }));
    return items;
  }
  async uploadAsset(file: Blob) {
    const form = new FormData();
    form.append('file', file);
    const { data } = await api.post('/api/v1/audio/assets/upload', form, { headers: { 'Content-Type': 'multipart/form-data' } });
    return data.item as AudioAsset;
  }
  async createFolder(prefix: string) {
    const { data } = await api.post('/api/v1/audio/folders', { name: prefix });
    return data.item as AudioLibraryPrefix;
  }
  async renameAsset(ref: string, name: string, options: AudioLibraryAssetMutation = {}) {
    const { data } = await api.patch(`/api/v1/audio/assets/${encodeURIComponent(ref)}`, { name, ...options });
    return data as AudioAsset;
  }
  async moveAsset(ref: string, targetPrefix: string, name?: string) {
    const { data } = await api.patch(`/api/v1/audio/assets/${encodeURIComponent(ref)}`, { name, folderId: targetPrefix });
    return data as AudioAsset;
  }
  async deleteAsset(ref: string, options: { forceDetach?: boolean } = {}) {
    await api.delete(`/api/v1/audio/assets/${encodeURIComponent(ref)}`, { params: options });
  }
  async deleteFolder(prefix: string) {
    await api.delete(`/api/v1/audio/folders/${encodeURIComponent(prefix)}`);
  }
  async getPlayToken(ref: string) {
    const { data } = await api.post(`/api/v1/audio/assets/${encodeURIComponent(ref)}/play-token`);
    return data as { streamUrl: string; expiresAt: number };
  }
}

export function createAudioLibraryProvider(settings: AudioLibrarySettings, worldId?: string | null): AudioLibraryProvider {
  return settings.mode === 's3' ? new S3AudioLibraryProvider(worldId) : new DatabaseAudioLibraryProvider();
}
