import type { StageSceneOverlayMediaRef } from '../../shared/stage-types'
import type { SceneOverlayRenderer } from '../scene-overlay-types'

interface MediaRendererConfig {
  media?: StageSceneOverlayMediaRef
  fit?: 'cover' | 'contain' | 'fill'
  positionX?: number
  positionY?: number
  playbackRate?: number
}

interface MountedMedia {
  element: HTMLImageElement | HTMLVideoElement
  sourceKey: string
  dispose(): void
}

const finiteRange = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
)

const normalizeConfig = (input: unknown): MediaRendererConfig => {
  const value = input && typeof input === 'object' ? input as MediaRendererConfig : {}
  const fit = value.fit === 'contain' || value.fit === 'fill' ? value.fit : 'cover'
  return {
    media: value.media,
    fit,
    positionX: finiteRange(value.positionX, 50, 0, 100),
    positionY: finiteRange(value.positionY, 50, 0, 100),
    playbackRate: finiteRange(value.playbackRate, 1, 0.25, 4),
  }
}

const applyElementStyle = (element: HTMLImageElement | HTMLVideoElement, config: MediaRendererConfig) => {
  element.style.width = '100%'
  element.style.height = '100%'
  element.style.display = 'block'
  element.style.pointerEvents = 'none'
  element.style.objectFit = config.fit || 'cover'
  element.style.objectPosition = `${config.positionX}% ${config.positionY}%`
  if (element instanceof HTMLVideoElement) {
    const playbackRate = config.playbackRate || 1
    element.defaultPlaybackRate = playbackRate
    element.playbackRate = playbackRate
  }
}

export const mediaSceneOverlayRenderer: SceneOverlayRenderer = {
  type: 'media',
  mount(host, initialConfig, context) {
    host.style.position = 'absolute'
    host.style.inset = '0'
    host.style.overflow = 'hidden'
    host.style.pointerEvents = 'none'
    let mounted: MountedMedia | null = null
    let destroyed = false

    const disposeMounted = () => {
      mounted?.dispose()
      mounted = null
    }

    const mountMedia = (config: MediaRendererConfig, sourceKey: string, url: string, videoSource: boolean) => {
      const element = videoSource ? document.createElement('video') : document.createElement('img')
      let warned = false
      const warnLoadFailure = () => {
        if (warned || destroyed) return
        warned = true
        console.warn(`Scene overlay media failed to load: ${sourceKey}`)
      }
      const handleMetadata = () => {
        if (element instanceof HTMLVideoElement) void element.play().catch(() => undefined)
      }
      element.addEventListener('error', warnLoadFailure)
      if (element instanceof HTMLVideoElement) {
        element.autoplay = true
        element.loop = true
        element.muted = true
        element.defaultMuted = true
        element.playsInline = true
        element.preload = 'auto'
        element.addEventListener('loadedmetadata', handleMetadata)
      }
      applyElementStyle(element, config)
      element.src = url
      host.append(element)
      if (element instanceof HTMLVideoElement) void element.play().catch(() => undefined)
      mounted = {
        element,
        sourceKey,
        dispose() {
          element.removeEventListener('error', warnLoadFailure)
          if (element instanceof HTMLVideoElement) {
            element.removeEventListener('loadedmetadata', handleMetadata)
            element.pause()
            element.removeAttribute('src')
            element.load()
          } else {
            element.removeAttribute('src')
          }
          element.remove()
        },
      }
    }

    const apply = (input: unknown) => {
      if (destroyed) return
      const config = normalizeConfig(input)
      const media = config.media
      const resourceId = media?.resourceId?.trim() || ''
      if (!resourceId) {
        disposeMounted()
        return
      }
      const variant = media?.variant?.trim() || 'original'
      const mimeType = media?.mimeType?.trim().toLowerCase() || ''
      const videoSource = mimeType === 'video/webm'
      const sourceKey = `${resourceId}\n${variant}\n${mimeType}`
      if (mounted?.sourceKey === sourceKey) {
        applyElementStyle(mounted.element, config)
        return
      }
      const url = context.resolveResourceUrl(resourceId, variant)
      disposeMounted()
      if (!url) return
      mountMedia(config, sourceKey, url, videoSource)
    }

    apply(initialConfig)
    return {
      update: apply,
      destroy() {
        destroyed = true
        disposeMounted()
      },
    }
  },
}
