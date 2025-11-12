import Dexie, { type Table } from 'dexie';
import { toRaw } from 'vue';
import type { AudioAsset } from '@/types/audio';

export interface CachedAudioAssetMeta {
  id: string;
  name: string;
  folderId: string | null;
  tags: string[];
  creator: string;
  duration: number;
  updatedAt: number;
  folderPath: string;
  description: string;
  searchIndex: string;
}

class AudioStudioDexie extends Dexie {
  public assets!: Table<CachedAudioAssetMeta>;

  constructor() {
    super('sealchatAudioStudio');
    this.version(1).stores({
      assets: '&id, folderId, searchIndex, updatedAt'
    });
    this.version(2)
      .stores({
        assets: '&id, folderId, searchIndex, updatedAt, folderPath'
      })
      .upgrade((tx) => {
        return tx.table('assets').toCollection().modify((meta) => {
          (meta as CachedAudioAssetMeta).folderPath = meta.folderPath || '';
          (meta as CachedAudioAssetMeta).description = meta.description || '';
        });
      });
  }
}

export const audioDb = new AudioStudioDexie();

export function toCachedMeta(asset: AudioAsset, folderPath = ''): CachedAudioAssetMeta {
  const rawTags = normalizeTags(asset.tags);
  const description = (asset.description ?? '').toString();
  return {
    id: asset.id,
    name: asset.name,
    folderId: asset.folderId,
    tags: rawTags,
    creator: asset.createdBy,
    duration: asset.duration,
    updatedAt: new Date(asset.updatedAt).getTime(),
    folderPath,
    description,
    searchIndex: `${asset.name} ${rawTags.join(' ')} ${asset.createdBy} ${folderPath} ${description}`.toLowerCase(),
  };
}

function normalizeTags(tags: string[] | undefined | null): string[] {
  if (!tags) return [];
  const raw = toRaw(tags);
  return Array.isArray(raw) ? raw.map((tag) => String(tag)) : [];
}
