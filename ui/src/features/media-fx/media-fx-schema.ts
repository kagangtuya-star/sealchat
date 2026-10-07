import { z } from 'zod'

import {
  createDefaultMediaFxAdvanced,
  LEGACY_MEDIA_FX_VERSION,
  MEDIA_FX_ADVANCED_RANGES,
  MEDIA_FX_DURATION_RANGE,
  MEDIA_FX_FILTER_RANGES,
  MEDIA_FX_INTENSITY_RANGE,
  MEDIA_FX_VERSION,
  mediaFxMotionPresets,
  type MediaFxRange,
  type MediaFxSpec,
} from './media-fx'

// Strict validation for MediaFxSpec embedded in persisted documents. Ranges come from
// the core constants so the schema and normalizeMediaFxSpec can never drift apart;
// keep protocol.validateMediaFx in sync with the same bounds.
const rangeNumber = (range: Readonly<MediaFxRange>) => z.number().finite().min(range.min).max(range.max)

const mediaFxMotionSchema = z.strictObject({
  preset: z.enum(mediaFxMotionPresets),
  intensity: rangeNumber(MEDIA_FX_INTENSITY_RANGE),
  durationMs: rangeNumber(MEDIA_FX_DURATION_RANGE).int(),
  loop: z.boolean(),
})

const mediaFxFilterSchema = z.strictObject({
  brightness: rangeNumber(MEDIA_FX_FILTER_RANGES.brightness),
  contrast: rangeNumber(MEDIA_FX_FILTER_RANGES.contrast),
  saturation: rangeNumber(MEDIA_FX_FILTER_RANGES.saturation),
  grayscale: rangeNumber(MEDIA_FX_FILTER_RANGES.grayscale),
  sepia: rangeNumber(MEDIA_FX_FILTER_RANGES.sepia),
  hueRotate: rangeNumber(MEDIA_FX_FILTER_RANGES.hueRotate),
  blurPx: rangeNumber(MEDIA_FX_FILTER_RANGES.blurPx),
})

const mediaFxAdvancedSchema = z.strictObject({
  pixelate: rangeNumber(MEDIA_FX_ADVANCED_RANGES.pixelate),
  rgbSplit: rangeNumber(MEDIA_FX_ADVANCED_RANGES.rgbSplit),
  scanline: rangeNumber(MEDIA_FX_ADVANCED_RANGES.scanline),
})

export const mediaFxV2SpecSchema = z.strictObject({
  version: z.literal(MEDIA_FX_VERSION),
  motion: mediaFxMotionSchema,
  filter: mediaFxFilterSchema,
  advanced: mediaFxAdvancedSchema,
}) satisfies z.ZodType<MediaFxSpec>

// Historical v1 documents (TheaterPresentation v3, stage surfaces, bridge payloads)
// stay strict: a v1 spec carrying `advanced` or any unknown field is rejected.
export const mediaFxV1SpecSchema = z.strictObject({
  version: z.literal(LEGACY_MEDIA_FX_VERSION),
  motion: mediaFxMotionSchema,
  filter: mediaFxFilterSchema,
})

// Accepts strict v2, or strict v1 upgraded losslessly to the canonical v2 shape.
export const mediaFxSpecSchema = z.union([
  mediaFxV2SpecSchema,
  mediaFxV1SpecSchema.transform((spec): MediaFxSpec => ({
    version: MEDIA_FX_VERSION,
    motion: spec.motion,
    filter: spec.filter,
    advanced: createDefaultMediaFxAdvanced(),
  })),
])
