export type FontSourceType = 'default' | 'system' | 'manual' | 'upload' | 'url'

export interface FontAssetRecord {
  id: string
  family: string
  sourceType: Extract<FontSourceType, 'upload' | 'url'>
  mime: string
  size: number
  blob: Blob
  createdAt: number
  updatedAt: number
  sourceUrl?: string
}

export interface FontAssetMeta {
  id: string
  family: string
  sourceType: Extract<FontSourceType, 'upload' | 'url'>
  mime: string
  size: number
  createdAt: number
  updatedAt: number
  sourceUrl?: string
}

export interface FontAssetSaveResult {
  saved: FontAssetMeta
  evictedIds: string[]
}

export interface ImportedFontPayload {
  family: string
  cssFontFamily: string
  sourceType: Extract<FontSourceType, 'upload' | 'url'>
  blob: Blob
  mime: string
  size: number
  sourceUrl?: string
}
