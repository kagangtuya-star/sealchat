import { z } from 'zod'

import {
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

export const mediaFxSpecSchema = z.strictObject({
  version: z.literal(MEDIA_FX_VERSION),
  motion: z.strictObject({
    preset: z.enum(mediaFxMotionPresets),
    intensity: rangeNumber(MEDIA_FX_INTENSITY_RANGE),
    durationMs: rangeNumber(MEDIA_FX_DURATION_RANGE).int(),
    loop: z.boolean(),
  }),
  filter: z.strictObject({
    brightness: rangeNumber(MEDIA_FX_FILTER_RANGES.brightness),
    contrast: rangeNumber(MEDIA_FX_FILTER_RANGES.contrast),
    saturation: rangeNumber(MEDIA_FX_FILTER_RANGES.saturation),
    grayscale: rangeNumber(MEDIA_FX_FILTER_RANGES.grayscale),
    sepia: rangeNumber(MEDIA_FX_FILTER_RANGES.sepia),
    hueRotate: rangeNumber(MEDIA_FX_FILTER_RANGES.hueRotate),
    blurPx: rangeNumber(MEDIA_FX_FILTER_RANGES.blurPx),
  }),
}) satisfies z.ZodType<MediaFxSpec>
