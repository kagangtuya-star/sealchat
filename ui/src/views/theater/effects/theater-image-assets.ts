export const THEATER_IMAGE_ASSET_DRAG_TYPE = 'application/x-sealchat-theater-image-asset'

export interface TheaterImageAssetResource {
  id: string
  status: string
  mimeType: string
  playbackVariant?: string
  playbackMimeType?: string
  animated?: boolean
  loopCount?: number | null
  width?: number
  height?: number
}

export interface TheaterImageAsset {
  id: string
  name: string
  resourceId: string
  createdAt: string
  updatedAt: string
  resource: TheaterImageAssetResource
  url: string
}
