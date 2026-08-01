import { api } from '@/stores/_config'
import type { CursorThemeConfig } from './cursorTypes'

export type CursorScope = 'platform' | 'world'

export interface UploadedCursorAsset {
  attachmentId: string
  width: number
  height: number
  animated: boolean
  mimeType: 'image/webp'
}

export const uploadCursorAsset = async (
  file: File,
  scope: CursorScope,
  worldId?: string,
  size = 32,
): Promise<UploadedCursorAsset> => {
  const form = new FormData()
  form.append('file', file)
  form.append('scope', scope)
  form.append('size', String(size))
  if (worldId) form.append('worldId', worldId)
  const response = await api.post('/api/v1/cursor-assets', form)
  return response.data as UploadedCursorAsset
}

export const cursorAttachmentIds = (config?: CursorThemeConfig | null): string[] =>
  Object.values(config?.slots || {})
    .filter((asset) => asset?.mode === 'custom' && asset.attachmentId)
    .map((asset) => String(asset?.attachmentId || '').replace(/^id:/, ''))
