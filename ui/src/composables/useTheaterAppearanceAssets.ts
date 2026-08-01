import { api } from '@/stores/_config'
import {
  buildTheaterAppearanceAssetFields,
  type TheaterAppearanceAsset,
} from '@/components/theater-presentation/theaterAppearanceAssetState'

export type { TheaterAppearanceAsset, TheaterAppearanceAssetStatus } from '@/components/theater-presentation/theaterAppearanceAssetState'
export { buildTheaterAppearanceAssetFields, canApplyTheaterAppearanceAsset, getTheaterAssetErrorCode, isTheaterAppearanceAssetProcessing } from '@/components/theater-presentation/theaterAppearanceAssetState'

const unwrapAsset = (data: any): TheaterAppearanceAsset => data?.asset || data?.item || data

export const uploadTheaterAppearanceAsset = async (input: {
  channelId: string
  identityId: string
  variantId?: string
  targetUserId?: string
  purpose: TheaterAppearanceAsset['purpose']
  file: File
}): Promise<TheaterAppearanceAsset> => {
  const form = new FormData()
  form.append('file', input.file)
  Object.entries(buildTheaterAppearanceAssetFields(input)).forEach(([key, value]) => form.append(key, value))
  const response = await api.post(`api/v1/channels/${input.channelId}/theater-appearance-assets`, form)
  return unwrapAsset(response.data)
}

export const importTheaterAppearanceAsset = async (input: {
  channelId: string
  identityId: string
  variantId?: string
  targetUserId?: string
  purpose: TheaterAppearanceAsset['purpose']
  attachmentId: string
}): Promise<TheaterAppearanceAsset> => {
  const response = await api.post(`api/v1/channels/${input.channelId}/theater-appearance-assets/import`, {
    attachmentId: input.attachmentId,
    identityId: input.identityId,
    variantId: input.variantId,
    targetUserId: input.targetUserId,
    purpose: input.purpose,
  })
  return unwrapAsset(response.data)
}

export const getTheaterAppearanceAsset = async (channelId: string, assetId: string): Promise<TheaterAppearanceAsset> => {
  const response = await api.get(`api/v1/channels/${channelId}/theater-appearance-assets/${assetId}`)
  return unwrapAsset(response.data)
}

export const waitForTheaterAppearanceAsset = async (
  channelId: string,
  initial: TheaterAppearanceAsset,
  onUpdate: (asset: TheaterAppearanceAsset) => void,
): Promise<TheaterAppearanceAsset> => {
  let current = initial
  onUpdate(current)
  while (current.status === 'pending' || current.status === 'processing') {
    await new Promise((resolve) => window.setTimeout(resolve, 700))
    current = await getTheaterAppearanceAsset(channelId, current.id)
    onUpdate(current)
  }
  return current
}

export const deleteTheaterAppearanceAsset = async (channelId: string, assetId: string) => {
  await api.delete(`api/v1/channels/${channelId}/theater-appearance-assets/${assetId}`)
}
