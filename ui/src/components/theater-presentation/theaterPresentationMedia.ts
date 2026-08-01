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
