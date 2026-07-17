import type { TheaterMediaRef } from '@/types/theaterPresentation'

export type TheaterAppearanceAssetStatus = 'pending' | 'processing' | 'ready' | 'failed'
export interface TheaterAppearanceAsset {
  id: string
  channelId: string
  ownerUserId: string
  identityId: string
  variantId?: string
  purpose: 'portrait' | 'portrait-decoration' | 'dialogue-frame'
  status: TheaterAppearanceAssetStatus
  progress: number
  failureCode?: string
  failureMessage?: string
  media?: TheaterMediaRef
}

export const isTheaterAppearanceAssetProcessing = (asset?: TheaterAppearanceAsset | null) => (
  asset?.status === 'pending' || asset?.status === 'processing'
)

export const canApplyTheaterAppearanceAsset = (asset?: TheaterAppearanceAsset | null) => (
  asset?.status === 'ready' && Boolean(asset.media)
)

export const buildTheaterAppearanceAssetFields = (input: {
  identityId: string
  variantId?: string
  targetUserId?: string
  purpose: TheaterAppearanceAsset['purpose']
}) => ({
  purpose: input.purpose,
  identityId: input.identityId,
  ...(input.variantId ? { variantId: input.variantId } : {}),
  ...(input.targetUserId ? { targetUserId: input.targetUserId } : {}),
})

export const getTheaterAssetErrorCode = (error: any): string => (
  String(error?.response?.data?.error?.code || error?.response?.data?.code || error?.code || 'ASSET_UPLOAD_FAILED')
)
