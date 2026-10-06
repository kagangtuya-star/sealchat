// Media FX core: a renderer-agnostic, versioned description of image motion and
// static visual adjustments. Business code only stores MediaFxSpec; DOM / Konva /
// Canvas adapters translate it into concrete rendering primitives.
//
// Every value is a whitelisted enum or a clamped finite number. Never persist or
// execute caller-provided CSS filter strings, animation names or keyframes.

export const MEDIA_FX_VERSION = 1 as const

export const mediaFxMotionPresets = [
  'none',
  'shake',
  'shake-x',
  'shake-y',
  'breathe',
  'float',
  'drift',
  'zoom',
] as const

export type MediaFxMotionPreset = typeof mediaFxMotionPresets[number]

export interface MediaFxMotion {
  preset: MediaFxMotionPreset
  intensity: number
  durationMs: number
  loop: boolean
}

export interface MediaFxFilter {
  brightness: number
  contrast: number
  saturation: number
  grayscale: number
  sepia: number
  hueRotate: number
  blurPx: number
}

export interface MediaFxSpec {
  version: typeof MEDIA_FX_VERSION
  motion: MediaFxMotion
  filter: MediaFxFilter
}

export type MediaFxFilterKey = keyof MediaFxFilter

export interface MediaFxRange {
  min: number
  max: number
  step: number
  defaultValue: number
}

export const MEDIA_FX_FILTER_RANGES: Readonly<Record<MediaFxFilterKey, MediaFxRange>> = {
  brightness: { min: 0.25, max: 2, step: 0.01, defaultValue: 1 },
  contrast: { min: 0.25, max: 2, step: 0.01, defaultValue: 1 },
  saturation: { min: 0, max: 2, step: 0.01, defaultValue: 1 },
  grayscale: { min: 0, max: 1, step: 0.01, defaultValue: 0 },
  sepia: { min: 0, max: 1, step: 0.01, defaultValue: 0 },
  hueRotate: { min: -180, max: 180, step: 1, defaultValue: 0 },
  blurPx: { min: 0, max: 24, step: 0.1, defaultValue: 0 },
}

export const mediaFxFilterKeys = Object.keys(MEDIA_FX_FILTER_RANGES) as MediaFxFilterKey[]

export const MEDIA_FX_INTENSITY_RANGE: Readonly<MediaFxRange> = { min: 0, max: 1, step: 0.01, defaultValue: 0.5 }
export const MEDIA_FX_DURATION_RANGE: Readonly<MediaFxRange> = { min: 300, max: 20_000, step: 10, defaultValue: 4_000 }

// Values within this distance of the default are treated as "no change" so slider
// round-trips do not leave near-default noise in persisted data.
const FILTER_EPSILON = 0.001

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))

const finiteInRange = (value: unknown, range: MediaFxRange) => (
  typeof value === 'number' && Number.isFinite(value)
    ? clamp(value, range.min, range.max)
    : range.defaultValue
)

const roundTo = (value: number, digits: number) => {
  const factor = 10 ** digits
  return Math.round(value * factor) / factor
}

const isPlainObject = (input: unknown): input is Record<string, unknown> => {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return false
  const prototype = Object.getPrototypeOf(input)
  return prototype === Object.prototype || prototype === null
}

// ---------------------------------------------------------------------------
// Motion definitions
// ---------------------------------------------------------------------------

export type MediaFxEasing = 'linear' | 'ease-in-out'

// x / y are fractions of the animated surface size; scale is absolute.
export interface MediaFxMotionFrame {
  offset: number
  x: number
  y: number
  scale: number
}

export interface MediaFxMotionTrack {
  frames: MediaFxMotionFrame[]
  durationMs: number
  loop: boolean
  easing: MediaFxEasing
}

interface MediaFxMotionShape {
  defaultDurationMs: number
  easing: MediaFxEasing
  // amount is already scaled by intensity (0 ~ max for the preset).
  maxAmount: number
  frames: (amount: number) => Array<Omit<MediaFxMotionFrame, 'offset'>>
}

// Fixed pseudo-random path so shake is deterministic and identical across renderers.
const SHAKE_PATH: Array<[number, number]> = [
  [0, 0], [-0.9, 0.55], [0.75, -0.8], [-0.55, -0.35], [1, 0.45], [-0.8, 0.9], [0.45, -1], [0, 0],
]

const translateFrames = (path: Array<[number, number]>, amountX: number, amountY: number) => (
  path.map(([x, y]) => ({ x: x * amountX, y: y * amountY, scale: 1 }))
)

const mediaFxMotionShapes: Record<Exclude<MediaFxMotionPreset, 'none'>, MediaFxMotionShape> = {
  shake: {
    defaultDurationMs: 480,
    easing: 'linear',
    maxAmount: 0.024,
    frames: (amount) => translateFrames(SHAKE_PATH, amount, amount),
  },
  'shake-x': {
    defaultDurationMs: 420,
    easing: 'linear',
    maxAmount: 0.024,
    frames: (amount) => translateFrames(SHAKE_PATH, amount, 0),
  },
  'shake-y': {
    defaultDurationMs: 420,
    easing: 'linear',
    maxAmount: 0.024,
    frames: (amount) => translateFrames(SHAKE_PATH.map(([x, y]) => [y, x]), 0, amount),
  },
  breathe: {
    defaultDurationMs: 4_000,
    easing: 'ease-in-out',
    maxAmount: 0.05,
    frames: (amount) => [
      { x: 0, y: 0, scale: 1 },
      { x: 0, y: 0, scale: 1 + amount },
      { x: 0, y: 0, scale: 1 },
    ],
  },
  float: {
    defaultDurationMs: 4_000,
    easing: 'ease-in-out',
    maxAmount: 0.04,
    frames: (amount) => [
      { x: 0, y: 0, scale: 1 },
      { x: 0, y: -amount, scale: 1 },
      { x: 0, y: 0, scale: 1 },
    ],
  },
  drift: {
    defaultDurationMs: 12_000,
    easing: 'ease-in-out',
    maxAmount: 0.03,
    frames: (amount) => translateFrames([[0, 0], [1, -0.6], [0.2, -1], [-0.9, -0.4], [-0.4, 0.5], [0, 0]], amount, amount),
  },
  zoom: {
    defaultDurationMs: 8_000,
    easing: 'ease-in-out',
    maxAmount: 0.08,
    frames: (amount) => [
      { x: 0, y: 0, scale: 1 },
      { x: 0, y: 0, scale: 1 + amount },
      { x: 0, y: 0, scale: 1 },
    ],
  },
}

export const mediaFxMotionDefaultDuration = (preset: MediaFxMotionPreset) => (
  preset === 'none' ? MEDIA_FX_DURATION_RANGE.defaultValue : mediaFxMotionShapes[preset].defaultDurationMs
)

// Speed is a UI-only view of durationMs relative to the preset default.
export const mediaFxMotionSpeed = (motion: Pick<MediaFxMotion, 'preset' | 'durationMs'>) => (
  roundTo(mediaFxMotionDefaultDuration(motion.preset) / Math.max(1, motion.durationMs), 2)
)

export const mediaFxDurationForSpeed = (preset: MediaFxMotionPreset, speed: number) => {
  const safeSpeed = Number.isFinite(speed) && speed > 0 ? speed : 1
  return Math.round(clamp(mediaFxMotionDefaultDuration(preset) / safeSpeed, MEDIA_FX_DURATION_RANGE.min, MEDIA_FX_DURATION_RANGE.max))
}

export const resolveMediaFxMotionTrack = (motion: MediaFxMotion): MediaFxMotionTrack | null => {
  if (motion.preset === 'none' || motion.intensity <= 0) return null
  const shape = mediaFxMotionShapes[motion.preset]
  if (!shape) return null
  const points = shape.frames(shape.maxAmount * clamp(motion.intensity, 0, 1))
  const lastIndex = Math.max(1, points.length - 1)
  return {
    frames: points.map((point, index) => ({
      offset: roundTo(index / lastIndex, 4),
      x: roundTo(point.x, 5),
      y: roundTo(point.y, 5),
      scale: roundTo(point.scale, 5),
    })),
    durationMs: motion.durationMs,
    loop: motion.loop,
    easing: shape.easing,
  }
}

// Static renderer compensation for a centered surface that already covers its box.
export const resolveMediaFxMotionOverscanScale = (motion: MediaFxMotion): number => {
  const track = resolveMediaFxMotionTrack(motion)
  let overscan = 1
  for (const frame of track?.frames ?? []) {
    const safeScale = Math.max(frame.scale, 0.0001)
    overscan = Math.max(overscan, (1 + 2 * Math.abs(frame.x)) / safeScale, (1 + 2 * Math.abs(frame.y)) / safeScale)
  }
  // Keep four decimal places without rounding below the required coverage.
  const rounded = roundTo(overscan, 4)
  return rounded < overscan ? roundTo(rounded + 0.0001, 4) : rounded
}

// ---------------------------------------------------------------------------
// Defaults, normalization and emptiness
// ---------------------------------------------------------------------------

export const createDefaultMediaFxFilter = (): MediaFxFilter => ({
  brightness: 1,
  contrast: 1,
  saturation: 1,
  grayscale: 0,
  sepia: 0,
  hueRotate: 0,
  blurPx: 0,
})

export const createDefaultMediaFxMotion = (): MediaFxMotion => ({
  preset: 'none',
  intensity: MEDIA_FX_INTENSITY_RANGE.defaultValue,
  durationMs: MEDIA_FX_DURATION_RANGE.defaultValue,
  loop: true,
})

export const createDefaultMediaFxSpec = (): MediaFxSpec => ({
  version: MEDIA_FX_VERSION,
  motion: createDefaultMediaFxMotion(),
  filter: createDefaultMediaFxFilter(),
})

export const normalizeMediaFxFilter = (input: unknown): MediaFxFilter => {
  const value = isPlainObject(input) ? input : {}
  const filter = createDefaultMediaFxFilter()
  mediaFxFilterKeys.forEach((key) => {
    const range = MEDIA_FX_FILTER_RANGES[key]
    const normalized = roundTo(finiteInRange(value[key], range), key === 'hueRotate' ? 1 : 3)
    filter[key] = Math.abs(normalized - range.defaultValue) < FILTER_EPSILON ? range.defaultValue : normalized
  })
  return filter
}

export const normalizeMediaFxMotion = (input: unknown): MediaFxMotion => {
  const value = isPlainObject(input) ? input : {}
  const preset = mediaFxMotionPresets.includes(value.preset as MediaFxMotionPreset)
    ? value.preset as MediaFxMotionPreset
    : 'none'
  const fallbackDuration = mediaFxMotionDefaultDuration(preset)
  return {
    preset,
    intensity: roundTo(finiteInRange(value.intensity, MEDIA_FX_INTENSITY_RANGE), 3),
    durationMs: Math.round(typeof value.durationMs === 'number' && Number.isFinite(value.durationMs)
      ? clamp(value.durationMs, MEDIA_FX_DURATION_RANGE.min, MEDIA_FX_DURATION_RANGE.max)
      : fallbackDuration),
    loop: value.loop !== false,
  }
}

// Unknown fields and versions are dropped; the result only contains whitelisted keys.
export const normalizeMediaFxSpec = (input: unknown): MediaFxSpec => {
  const value = isPlainObject(input) ? input : {}
  if (value.version !== undefined && value.version !== MEDIA_FX_VERSION) return createDefaultMediaFxSpec()
  return {
    version: MEDIA_FX_VERSION,
    motion: normalizeMediaFxMotion(value.motion),
    filter: normalizeMediaFxFilter(value.filter),
  }
}

export const mediaFxFilterHasContent = (filter: MediaFxFilter) => mediaFxFilterKeys.some((key) => (
  Math.abs(filter[key] - MEDIA_FX_FILTER_RANGES[key].defaultValue) >= FILTER_EPSILON
))

export const mediaFxMotionHasContent = (motion: MediaFxMotion) => motion.preset !== 'none' && motion.intensity > 0

export const mediaFxHasContent = (spec: MediaFxSpec | null | undefined) => Boolean(
  spec && (mediaFxMotionHasContent(spec.motion) || mediaFxFilterHasContent(spec.filter)),
)

// Persistence helper: returns null when the spec carries no effect so callers can
// delete the field instead of storing an all-default object.
export const compactMediaFxSpec = (input: unknown): MediaFxSpec | null => {
  const spec = normalizeMediaFxSpec(input)
  return mediaFxHasContent(spec) ? spec : null
}

export const mediaFxSpecsEqual = (left: MediaFxSpec, right: MediaFxSpec) => (
  left.motion.preset === right.motion.preset
  && left.motion.intensity === right.motion.intensity
  && left.motion.durationMs === right.motion.durationMs
  && left.motion.loop === right.motion.loop
  && mediaFxFilterKeys.every((key) => left.filter[key] === right.filter[key])
)

// ---------------------------------------------------------------------------
// Filter presets: only fill MediaFxFilter values; never persisted as a preset id.
// ---------------------------------------------------------------------------

export interface MediaFxFilterPreset {
  id: string
  label: string
  filter: Partial<MediaFxFilter>
}

export const mediaFxFilterPresets: readonly MediaFxFilterPreset[] = [
  { id: 'normal', label: '正常', filter: {} },
  { id: 'morning', label: '朝晨', filter: { brightness: 1.08, contrast: 0.94, saturation: 1.06, sepia: 0.12, hueRotate: -6 } },
  { id: 'sunset', label: '夕阳', filter: { brightness: 0.98, contrast: 1.06, saturation: 1.25, sepia: 0.32, hueRotate: -12 } },
  { id: 'night', label: '夜晚', filter: { brightness: 0.6, contrast: 1.08, saturation: 0.5, hueRotate: 10 } },
  { id: 'horror', label: '恐怖', filter: { brightness: 0.72, contrast: 1.38, saturation: 0.4, grayscale: 0.2, sepia: 0.08, hueRotate: -10 } },
  { id: 'fog', label: '雾', filter: { brightness: 1.12, contrast: 0.7, saturation: 0.7, blurPx: 0.8 } },
  { id: 'dream', label: '梦境', filter: { brightness: 1.08, contrast: 0.86, saturation: 1.25, sepia: 0.1, hueRotate: -14, blurPx: 0.6 } },
  { id: 'underwater', label: '水下', filter: { brightness: 0.84, contrast: 0.9, saturation: 0.6, grayscale: 0.15, hueRotate: 30, blurPx: 0.6 } },
  { id: 'old-photo', label: '旧照片', filter: { brightness: 0.98, contrast: 0.88, saturation: 0.7, sepia: 0.72 } },
]

export const mediaFxFilterFromPreset = (presetId: string): MediaFxFilter | null => {
  const preset = mediaFxFilterPresets.find((item) => item.id === presetId)
  return preset ? normalizeMediaFxFilter({ ...createDefaultMediaFxFilter(), ...preset.filter }) : null
}

export const matchMediaFxFilterPreset = (filter: MediaFxFilter): string | null => {
  const preset = mediaFxFilterPresets.find((item) => {
    const candidate = mediaFxFilterFromPreset(item.id)
    return candidate && mediaFxFilterKeys.every((key) => candidate[key] === filter[key])
  })
  return preset?.id ?? null
}

// ---------------------------------------------------------------------------
// Shared render helpers
// ---------------------------------------------------------------------------

export interface MediaFxFilterCssOptions {
  // Multiplies blurPx, e.g. to match a scaled preview with an unscaled bake.
  blurScale?: number
}

// Builds a CSS / Canvas2D filter string from a normalized filter. Every token is
// produced here from clamped numbers, so the output never contains caller text.
export const mediaFxFilterToCss = (input: MediaFxFilter, options: MediaFxFilterCssOptions = {}) => {
  const filter = normalizeMediaFxFilter(input)
  const parts: string[] = []
  const blurScale = typeof options.blurScale === 'number' && Number.isFinite(options.blurScale) && options.blurScale > 0
    ? options.blurScale
    : 1
  if (filter.brightness !== 1) parts.push(`brightness(${filter.brightness})`)
  if (filter.contrast !== 1) parts.push(`contrast(${filter.contrast})`)
  if (filter.saturation !== 1) parts.push(`saturate(${filter.saturation})`)
  if (filter.grayscale !== 0) parts.push(`grayscale(${filter.grayscale})`)
  if (filter.sepia !== 0) parts.push(`sepia(${filter.sepia})`)
  if (filter.hueRotate !== 0) parts.push(`hue-rotate(${filter.hueRotate}deg)`)
  if (filter.blurPx !== 0) parts.push(`blur(${roundTo(filter.blurPx * blurScale, 3)}px)`)
  return parts.join(' ')
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

export interface MediaFxCapabilities {
  motion: boolean
  filters: boolean
  // The target renders animated media (animated image / video).
  animatedMedia: boolean
}

export type MediaFxRendererKind = 'dom' | 'konva' | 'bake'

// Animated media is excluded from live filters in v1 so the Konva static cache path
// never freezes a frame and all live surfaces share one rule. Bake never has motion.
export const resolveMediaFxCapabilities = (renderer: MediaFxRendererKind, animatedMedia = false): MediaFxCapabilities => ({
  motion: renderer !== 'bake',
  filters: !animatedMedia,
  animatedMedia,
})

export const prefersReducedMotion = () => (
  typeof window !== 'undefined'
  && typeof window.matchMedia === 'function'
  && window.matchMedia('(prefers-reduced-motion: reduce)').matches
)
