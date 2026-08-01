import type { ChannelIdentityVariant } from '@/types'
import {
  theaterPresentationPatchSchema,
  theaterPresentationSchema,
  type TheaterPresentation,
  type TheaterPresentationPatch,
} from '@/types/theaterPresentation'

const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

export const cloneChannelIdentityTheaterPresentation = (
  value?: TheaterPresentation | null,
): TheaterPresentation | null => {
  if (!value) return null
  const parsed = theaterPresentationSchema.safeParse(cloneJson(value))
  return parsed.success ? parsed.data : null
}

export const cloneChannelIdentityTheaterPresentationPatch = (
  value?: TheaterPresentationPatch | null,
): TheaterPresentationPatch => {
  if (!value) return {}
  const parsed = theaterPresentationPatchSchema.safeParse(cloneJson(value))
  return parsed.success ? parsed.data : {}
}

export const resolveChannelIdentityVariantTheaterPatch = (
  variant?: Pick<ChannelIdentityVariant, 'theaterPresentation' | 'appearance'> | null,
): TheaterPresentationPatch => {
  const value = variant?.theaterPresentation !== undefined
    ? variant.theaterPresentation
    : variant?.appearance?.theaterPresentation
  return value && typeof value === 'object'
    ? cloneChannelIdentityTheaterPresentationPatch(value as TheaterPresentationPatch)
    : {}
}
