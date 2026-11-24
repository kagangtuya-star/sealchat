import { api, urlBase } from '@/stores/_config'
import type { StageImageRef } from '@/views/theater/shared/stage-types'
import { prepareTheaterMediaUpload } from '@/views/theater/stage/theater-media-upload'
import { normalizedTheaterMediaType, theaterResourceContentPath, theaterResourcePath, type TheaterStageMediaScope } from '@/views/theater/stage/stage-media'

interface TheaterResource {
  id?: string
  status?: string
  animated?: boolean
  mimeType?: string
  playbackVariant?: string
  playbackMimeType?: string
  loopCount?: number | null
  processing?: { errorCode?: string }
}

interface TheaterResourceResponse { resource?: TheaterResource }

const scopeFor = (worldId: string): TheaterStageMediaScope => ({ urlBase: String(urlBase), worldId, channelId: '', scopeType: 'world' })

const processingError = (code?: string) => ({
  MEDIA_PROCESSOR_UNAVAILABLE: '动图处理不可用：服务器未配置 FFmpeg',
  MEDIA_LIMIT_EXCEEDED: '动图尺寸、帧数或时长超过服务器限制',
  IMAGE_DECODE_FAILED: '图片文件损坏或编码不受支持',
  MEDIA_TRANSCODE_FAILED: '动图转换失败',
  MEDIA_PROBE_FAILED: '无法读取图片媒体信息',
}[code || ''] || code || '媒体处理失败')

const waitForResource = async (worldId: string, resourceId: string) => {
  const scope = scopeFor(worldId)
  for (let attempt = 0; attempt < 240; attempt += 1) {
    const { data } = await api.get<TheaterResourceResponse>(theaterResourcePath(scope, resourceId))
    if (data.resource?.status === 'ready') return data.resource
    if (data.resource?.status === 'failed') throw new Error(processingError(data.resource.processing?.errorCode))
    await new Promise((resolve) => window.setTimeout(resolve, 500))
  }
  throw new Error('媒体处理超时')
}

export const deleteWorldPresentationMedia = async (worldId: string, resourceId: string) => {
  if (resourceId.trim()) await api.delete(theaterResourcePath(scopeFor(worldId), resourceId))
}

export const uploadWorldPresentationMedia = async (worldId: string, file: File): Promise<StageImageRef> => {
  const scope = scopeFor(worldId)
  const prepared = await prepareTheaterMediaUpload(file)
  const formData = new FormData()
  formData.append('file', prepared)
  formData.append('mediaKind', 'image')
  formData.append('clientResourceId', crypto.randomUUID?.() || `world-effect-${Date.now()}-${Math.random().toString(16).slice(2)}`)
  let resourceId = ''
  try {
    const { data } = await api.post<TheaterResourceResponse>(theaterResourcePath(scope), formData, { headers: { 'Content-Type': 'multipart/form-data' } })
    let resource = data.resource
    resourceId = resource?.id?.trim() || ''
    if (!resourceId) throw new Error('上传响应缺少资源 ID')
    if (resource?.status !== 'ready') resource = await waitForResource(worldId, resourceId)
    const originalMimeType = resource?.mimeType?.trim().toLowerCase() || prepared.type || normalizedTheaterMediaType(prepared)
    const variant = resource?.playbackVariant || 'original'
    const mimeType = resource?.playbackMimeType || originalMimeType
    const resourceBase = urlBase.startsWith('//') ? `${window.location.protocol}${urlBase}` : urlBase
    return {
      resourceId,
      url: `${resourceBase.replace(/\/$/, '')}${theaterResourceContentPath(scope, resourceId, variant)}`,
      mimeType,
      ...(resource?.animated === true ? { animated: true } : {}),
      ...(resource?.loopCount && resource.loopCount > 0 ? { loopCount: resource.loopCount } : {}),
    }
  } catch (error) {
    if (resourceId) void deleteWorldPresentationMedia(worldId, resourceId).catch(() => undefined)
    throw error
  }
}
