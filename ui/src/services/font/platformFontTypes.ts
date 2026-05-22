export type PlatformFontStatus = 'processing' | 'ready' | 'failed' | 'disabled'
export type PlatformFontDeliveryMode = 'single' | 'subset'

export interface PlatformFontAsset {
  id: string
  displayName: string
  family: string
  weight: string
  style: string
  status: PlatformFontStatus
  deliveryMode: PlatformFontDeliveryMode
  originalStorageType?: string
  originalObjectKey?: string
  subsetStorageType?: string
  subsetObjectKey?: string
  manifestStorageType?: string
  manifestObjectKey?: string
  previewText?: string
  sourceFileName?: string
  sourceMimeType?: string
  sourceSize?: number
  subsetCount?: number
  lastError?: string
  createdBy?: string
  updatedBy?: string
  lastPublishedAt?: string
  createdAt?: string
  updatedAt?: string
}

export interface PlatformFontListResponse {
  items: PlatformFontAsset[]
  total?: number
  page?: number
  pageSize?: number
}

export interface PlatformFontSubsetManifest {
  mode?: string
  entry?: string
  cssUrl?: string
  cssName?: string
  fontUrls?: string[]
  fontFiles?: string[]
  chunks?: Array<{
    name: string
    url: string
    unicodeRange?: string
    mimeType?: string
  }>
}

export interface PlatformFontSubsetPackagePayload {
  manifest: PlatformFontSubsetManifest
  files: Array<{
    name: string
    blob: Blob
    contentType?: string
  }>
}

export interface PlatformFontSplitCapability {
  available: boolean
  version?: string
  reason?: string
  wasmAssetName?: string
}

export interface PlatformFontCreatePayload {
  file: File
  displayName?: string
  family?: string
  weight?: string
  style?: string
  previewText?: string
}

export interface PlatformFontUpdatePayload {
  displayName?: string
  family?: string
  weight?: string
  style?: string
  previewText?: string
  status?: PlatformFontStatus
  deliveryMode?: PlatformFontDeliveryMode
  lastError?: string
}
