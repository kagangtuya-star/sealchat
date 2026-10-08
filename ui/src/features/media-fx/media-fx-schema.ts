import { z } from 'zod'

import {
  createDefaultMediaFxAdvanced,
  createDefaultMediaFxTemporal,
  LEGACY_MEDIA_FX_VERSION,
  MEDIA_FX_ADVANCED_RANGES,
  MEDIA_FX_DURATION_RANGE,
  MEDIA_FX_FILTER_RANGES,
  MEDIA_FX_INTENSITY_RANGE,
  MEDIA_FX_TEMPORAL_RANGES,
  MEDIA_FX_V2_VERSION,
  MEDIA_FX_V3_VERSION,
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

// v2 advanced: exactly the three effects v2 knew. A v2 spec carrying any v3-only
// effect is not a valid v2 document and is rejected by the strict object.
const mediaFxV2AdvancedSchema = z.strictObject({
  pixelate: rangeNumber(MEDIA_FX_ADVANCED_RANGES.pixelate),
  rgbSplit: rangeNumber(MEDIA_FX_ADVANCED_RANGES.rgbSplit),
  scanline: rangeNumber(MEDIA_FX_ADVANCED_RANGES.scanline),
})

const mediaFxAdvancedSchema = z.strictObject({
  ...mediaFxV2AdvancedSchema.shape,
  vignette: rangeNumber(MEDIA_FX_ADVANCED_RANGES.vignette),
  grain: rangeNumber(MEDIA_FX_ADVANCED_RANGES.grain),
  posterize: rangeNumber(MEDIA_FX_ADVANCED_RANGES.posterize),
  negative: rangeNumber(MEDIA_FX_ADVANCED_RANGES.negative),
  sharpen: rangeNumber(MEDIA_FX_ADVANCED_RANGES.sharpen),
  edge: rangeNumber(MEDIA_FX_ADVANCED_RANGES.edge),
})

// v4 temporal: every field is required, unknown fields (seeds, uniforms, shaders) are
// rejected. Renderer parameters are derived from these values, never stored.
const mediaFxTemporalSchema = z.strictObject({
  grain: rangeNumber(MEDIA_FX_TEMPORAL_RANGES.grain),
  flicker: rangeNumber(MEDIA_FX_TEMPORAL_RANGES.flicker),
  glitch: rangeNumber(MEDIA_FX_TEMPORAL_RANGES.glitch),
  scanlineRoll: rangeNumber(MEDIA_FX_TEMPORAL_RANGES.scanlineRoll),
  speed: rangeNumber(MEDIA_FX_TEMPORAL_RANGES.speed),
})

export const mediaFxV4SpecSchema = z.strictObject({
  version: z.literal(MEDIA_FX_VERSION),
  motion: mediaFxMotionSchema,
  filter: mediaFxFilterSchema,
  advanced: mediaFxAdvancedSchema,
  temporal: mediaFxTemporalSchema,
}) satisfies z.ZodType<MediaFxSpec>

// v3 knew all nine advanced effects but no temporal; a v3 spec carrying `temporal`
// is rejected by the strict object.
export const mediaFxV3SpecSchema = z.strictObject({
  version: z.literal(MEDIA_FX_V3_VERSION),
  motion: mediaFxMotionSchema,
  filter: mediaFxFilterSchema,
  advanced: mediaFxAdvancedSchema,
})

export const mediaFxV2SpecSchema = z.strictObject({
  version: z.literal(MEDIA_FX_V2_VERSION),
  motion: mediaFxMotionSchema,
  filter: mediaFxFilterSchema,
  advanced: mediaFxV2AdvancedSchema,
})

// Historical v1 documents (TheaterPresentation v3, stage surfaces, bridge payloads)
// stay strict: a v1 spec carrying `advanced` or any unknown field is rejected.
export const mediaFxV1SpecSchema = z.strictObject({
  version: z.literal(LEGACY_MEDIA_FX_VERSION),
  motion: mediaFxMotionSchema,
  filter: mediaFxFilterSchema,
})

// Accepts strict v4, or strict v1 / v2 / v3 upgraded losslessly to the canonical v4
// shape (effects the old version did not know are off).
export const mediaFxSpecSchema = z.union([
  mediaFxV4SpecSchema,
  mediaFxV3SpecSchema.transform((spec): MediaFxSpec => ({
    version: MEDIA_FX_VERSION,
    motion: spec.motion,
    filter: spec.filter,
    advanced: spec.advanced,
    temporal: createDefaultMediaFxTemporal(),
  })),
  mediaFxV2SpecSchema.transform((spec): MediaFxSpec => ({
    version: MEDIA_FX_VERSION,
    motion: spec.motion,
    filter: spec.filter,
    advanced: { ...createDefaultMediaFxAdvanced(), ...spec.advanced },
    temporal: createDefaultMediaFxTemporal(),
  })),
  mediaFxV1SpecSchema.transform((spec): MediaFxSpec => ({
    version: MEDIA_FX_VERSION,
    motion: spec.motion,
    filter: spec.filter,
    advanced: createDefaultMediaFxAdvanced(),
    temporal: createDefaultMediaFxTemporal(),
  })),
])
