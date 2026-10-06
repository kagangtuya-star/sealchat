import { resolveMediaFxCapabilities } from '../../features/media-fx/media-fx'
import type { MediaFxDirectiveValue } from '../../features/media-fx/media-fx-dom'
import type { TheaterMediaRef } from '../../types/theaterPresentation'

export type TheaterMediaCandidate = { kind: 'image' | 'video'; attachmentId: string }

export const resolveTheaterMediaCandidates = (
  media: TheaterMediaRef,
  options: { preferStatic?: boolean; supportsVideo?: boolean } = {},
): TheaterMediaCandidate[] => {
  const fallback = media.fallbackAttachmentId
    ? [{ kind: 'image' as const, attachmentId: media.fallbackAttachmentId }]
    : []
  if (media.mimeType === 'video/webm') {
    const primary = { kind: 'video' as const, attachmentId: media.resourceAttachmentId }
    if (options.preferStatic || options.supportsVideo === false) return [...fallback, primary]
    return [primary, ...fallback]
  }
  return [{ kind: 'image', attachmentId: media.resourceAttachmentId }, ...fallback]
}

export const isAnimatedTheaterMedia = (media: TheaterMediaRef | null | undefined) => Boolean(media && media.kind !== 'static_image')

// v-media-fx binding for one theater visual layer. Animated media keeps the Media FX v1
// capability rule (motion only, no live filters); reduced motion stops motion but keeps
// static filters. The directive must sit on a wrapper inside the layout element so
// WAAPI motion never replaces the layer's own transform.
export const resolveTheaterMediaFxBinding = (
  spec: unknown,
  media: TheaterMediaRef | null | undefined,
  reducedMotion: boolean,
): MediaFxDirectiveValue => ({
  spec: spec ?? null,
  filters: resolveMediaFxCapabilities('dom', isAnimatedTheaterMedia(media)).filters,
  reducedMotion,
})
