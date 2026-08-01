import { resolveStageImageUrl, type StageImageRef } from '../shared/stage-types'

export interface TheaterStageMediaScope {
  urlBase: string
  worldId: string
  channelId: string
  scopeType?: 'channel' | 'world'
}

export interface TheaterStageMediaLocation {
  url: string
  managed: boolean
}

const managedTheaterContentPath = /\/api\/v1\/worlds\/[^/]+(?:\/channels\/[^/]+)?\/theater\/resources\/[^/]+(?:\/variants\/([^/]+))?\/content\/?$/

const decodedPathSegment = (value: string | undefined) => {
  if (!value) return ''
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

export const theaterResourcePath = (scope: TheaterStageMediaScope, resourceId = '') => {
  const base = scope.scopeType === 'world'
    ? `api/v1/worlds/${encodeURIComponent(scope.worldId)}/theater/resources`
    : `api/v1/worlds/${encodeURIComponent(scope.worldId)}/channels/${encodeURIComponent(scope.channelId)}/theater/resources`
  return resourceId ? `${base}/${encodeURIComponent(resourceId)}` : base
}

export const theaterResourceContentPath = (scope: TheaterStageMediaScope, resourceId: string, variant = '') => {
  const base = theaterResourcePath(scope, resourceId)
  const suffix = variant ? `/variants/${encodeURIComponent(variant)}/content` : '/content'
  return `/${base}${suffix}`
}

export const resolveTheaterStageMediaLocation = (
  imageRef: StageImageRef,
  scope: TheaterStageMediaScope,
  baseHref = typeof window !== 'undefined' ? window.location.href : 'https://sealchat.invalid/',
): TheaterStageMediaLocation | null => {
  const normalized = imageRef.url.trim()
  if (!normalized) return null

  let candidate = normalized
  let managed = false
  try {
    const parsed = new URL(normalized, baseHref)
    const match = parsed.pathname.match(managedTheaterContentPath)
    if (match && imageRef.resourceId.trim()) {
      candidate = `${scope.urlBase.replace(/\/$/, '')}${theaterResourceContentPath(scope, imageRef.resourceId, decodedPathSegment(match[1]))}`
      managed = true
    } else if (normalized.startsWith('/api/')) {
      candidate = `${scope.urlBase.replace(/\/$/, '')}${normalized}`
    }
  } catch {
    return null
  }

  const url = resolveStageImageUrl(candidate, baseHref)
  return url ? { url, managed } : null
}
