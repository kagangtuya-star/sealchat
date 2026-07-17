import { z } from 'zod'

export const THEATER_PRESENTATION_SCHEMA_VERSION = 2 as const
export const MAX_THEATER_PORTRAIT_DECORATIONS = 16

export const theaterMediaKindSchema = z.enum(['static_image', 'animated_image', 'video'])
export const theaterObjectFitSchema = z.literal('cover')
export const theaterLayerSpaceSchema = z.enum(['viewport', 'portrait', 'dialogue'])
export const theaterBlendModeSchema = z.enum(['normal', 'multiply', 'screen', 'overlay'])

export const theaterTransformSchema = z.strictObject({
  x: z.number().finite().min(-1).max(2),
  y: z.number().finite().min(-1).max(2),
  width: z.number().finite().min(0.01).max(3),
  height: z.number().finite().min(0.01).max(3),
  rotation: z.number().finite().min(-180).max(180),
  opacity: z.number().finite().min(0).max(1),
  zIndex: z.number().int().min(-100).max(100),
})

export const theaterMediaRefSchema = z.strictObject({
  assetId: z.string().min(1).max(128),
  resourceAttachmentId: z.string().min(1).max(128),
  fallbackAttachmentId: z.string().min(1).max(128).optional(),
  mimeType: z.enum(['image/png', 'image/webp', 'video/webm']),
  kind: theaterMediaKindSchema,
  width: z.number().int().min(1).max(4096),
  height: z.number().int().min(1).max(4096),
  durationMs: z.number().int().min(0).max(60_000).optional(),
}).superRefine((media, context) => {
  const mediaMatchesKind = media.kind === 'video'
    ? media.mimeType === 'video/webm'
    : media.kind === 'animated_image'
      ? media.mimeType === 'image/webp' || media.mimeType === 'video/webm'
      : media.mimeType === 'image/png' || media.mimeType === 'image/webp'
  if (!mediaMatchesKind) {
    context.addIssue({ code: 'custom', path: ['mimeType'], message: 'mimeType does not match media kind' })
  }
})

export const theaterVisualLayerSchema = z.strictObject({
  id: z.string().min(1).max(128),
  enabled: z.boolean(),
  media: theaterMediaRefSchema,
  space: theaterLayerSpaceSchema,
  transform: theaterTransformSchema,
  fit: theaterObjectFitSchema,
  playbackRate: z.number().finite().min(0.25).max(4),
  blendMode: theaterBlendModeSchema,
})

export const theaterSpacingSchema = z.strictObject({
  top: z.number().finite().min(0).max(1),
  right: z.number().finite().min(0).max(1),
  bottom: z.number().finite().min(0).max(1),
  left: z.number().finite().min(0).max(1),
})

export const theaterTextLayerSchema = z.strictObject({
  enabled: z.boolean(),
  transform: theaterTransformSchema,
  fontScale: z.number().finite().min(0.25).max(4).default(1),
})

export const theaterDialogueStyleSchema = z.strictObject({
  transform: theaterTransformSchema,
  frame: theaterVisualLayerSchema.nullable(),
  speaker: theaterTextLayerSchema,
  content: theaterTextLayerSchema,
  padding: theaterSpacingSchema,
  nameGap: z.number().finite().min(0).max(1),
  textAlign: z.enum(['left', 'center', 'right']),
}).superRefine((dialogue, context) => {
  if (dialogue.frame && dialogue.frame.space !== 'dialogue') {
    context.addIssue({ code: 'custom', path: ['frame', 'space'], message: 'dialogue frame must use dialogue space' })
  }
})

const portraitDecorationsSchema = z.array(theaterVisualLayerSchema)
  .max(MAX_THEATER_PORTRAIT_DECORATIONS)
  .superRefine((layers, context) => {
    const ids = new Set<string>()
    layers.forEach((layer, index) => {
      if (layer.space !== 'portrait') {
        context.addIssue({ code: 'custom', path: [index, 'space'], message: 'portrait decoration must use portrait space' })
      }
      if (ids.has(layer.id)) {
        context.addIssue({ code: 'custom', path: [index, 'id'], message: 'portrait decoration id must be unique' })
      }
      ids.add(layer.id)
    })
  })

export const theaterPresentationSchema = z.strictObject({
  schemaVersion: z.literal(THEATER_PRESENTATION_SCHEMA_VERSION),
  portrait: theaterVisualLayerSchema.nullable(),
  portraitDecorations: portraitDecorationsSchema,
  dialogue: theaterDialogueStyleSchema,
}).superRefine((presentation, context) => {
  if (presentation.portrait && presentation.portrait.space !== 'viewport') {
    context.addIssue({ code: 'custom', path: ['portrait', 'space'], message: 'portrait must use viewport space' })
  }
})

export const theaterPresentationPatchSchema = z.strictObject({
  portrait: theaterVisualLayerSchema.nullable().optional(),
  portraitDecorations: portraitDecorationsSchema.nullable().optional(),
  dialogue: theaterDialogueStyleSchema.nullable().optional(),
}).superRefine((patch, context) => {
  if (patch.portrait && patch.portrait.space !== 'viewport') {
    context.addIssue({ code: 'custom', path: ['portrait', 'space'], message: 'portrait must use viewport space' })
  }
})

export type TheaterMediaKind = z.infer<typeof theaterMediaKindSchema>
export type TheaterObjectFit = z.infer<typeof theaterObjectFitSchema>
export type TheaterLayerSpace = z.infer<typeof theaterLayerSpaceSchema>
export type TheaterTransform = z.infer<typeof theaterTransformSchema>
export type TheaterMediaRef = z.infer<typeof theaterMediaRefSchema>
export type TheaterVisualLayer = z.infer<typeof theaterVisualLayerSchema>
export type TheaterTextLayer = z.infer<typeof theaterTextLayerSchema>
export type TheaterDialogueStyle = z.infer<typeof theaterDialogueStyleSchema>
export type TheaterPresentation = z.infer<typeof theaterPresentationSchema>
export type TheaterPresentationPatch = z.infer<typeof theaterPresentationPatchSchema>
export type ResolvedTheaterPresentation = TheaterPresentation

export const createDefaultTheaterTransform = (): TheaterTransform => ({
  x: 0,
  y: 0,
  width: 1,
  height: 1,
  rotation: 0,
  opacity: 1,
  zIndex: 0,
})

export const createDefaultTheaterDialogueStyle = (): TheaterDialogueStyle => ({
  transform: {
    x: 0.02,
    y: 0.69,
    width: 0.96,
    height: 0.28,
    rotation: 0,
    opacity: 1,
    zIndex: 0,
  },
  frame: null,
  speaker: {
    enabled: true,
    transform: { x: 0.08, y: 0.12, width: 0.34, height: 0.12, rotation: 0, opacity: 1, zIndex: 2 },
    fontScale: 1,
  },
  content: {
    enabled: true,
    transform: { x: 0.08, y: 0.30, width: 0.84, height: 0.56, rotation: 0, opacity: 1, zIndex: 2 },
    fontScale: 1,
  },
  padding: { top: 0.16, right: 0.08, bottom: 0.12, left: 0.08 },
  nameGap: 0.04,
  textAlign: 'left',
})

export const createDefaultTheaterPresentation = (): TheaterPresentation => ({
  schemaVersion: THEATER_PRESENTATION_SCHEMA_VERSION,
  portrait: null,
  portraitDecorations: [],
  dialogue: createDefaultTheaterDialogueStyle(),
})

export const normalizeTheaterPresentation = (input: unknown): TheaterPresentation => {
  if (!input || typeof input !== 'object') return createDefaultTheaterPresentation()
  const value = input as Partial<TheaterPresentation>
  if (value.schemaVersion !== THEATER_PRESENTATION_SCHEMA_VERSION) return createDefaultTheaterPresentation()
  return theaterPresentationSchema.parse({
    schemaVersion: THEATER_PRESENTATION_SCHEMA_VERSION,
    portrait: value.portrait ?? null,
    portraitDecorations: value.portraitDecorations ?? [],
    dialogue: value.dialogue ?? createDefaultTheaterDialogueStyle(),
  })
}

export const validateTheaterPresentation = (input: unknown) => theaterPresentationSchema.safeParse(input)

export const resolveTheaterPresentation = (
  base: TheaterPresentation | null | undefined,
  patch?: TheaterPresentationPatch | null,
): ResolvedTheaterPresentation => {
  const resolved = structuredClone(base ? normalizeTheaterPresentation(base) : createDefaultTheaterPresentation())
  if (!patch) return resolved
  const parsedPatch = theaterPresentationPatchSchema.safeParse(patch)
  if (!parsedPatch.success) return resolved
  const validatedPatch = parsedPatch.data
  if (validatedPatch.portrait !== undefined) resolved.portrait = structuredClone(validatedPatch.portrait)
  if (validatedPatch.portraitDecorations !== undefined) {
    resolved.portraitDecorations = structuredClone(validatedPatch.portraitDecorations ?? [])
  }
  if (validatedPatch.dialogue !== undefined) {
    resolved.dialogue = structuredClone(validatedPatch.dialogue ?? createDefaultTheaterDialogueStyle())
  }
  return theaterPresentationSchema.parse(resolved)
}

const clampFinite = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
)

export const normalizeTheaterTransform = (
  input: Partial<TheaterTransform> | null | undefined,
  fallback: TheaterTransform = createDefaultTheaterTransform(),
): TheaterTransform => ({
  x: clampFinite(input?.x, fallback.x, -1, 2),
  y: clampFinite(input?.y, fallback.y, -1, 2),
  width: clampFinite(input?.width, fallback.width, 0.01, 3),
  height: clampFinite(input?.height, fallback.height, 0.01, 3),
  rotation: clampFinite(input?.rotation, fallback.rotation, -180, 180),
  opacity: clampFinite(input?.opacity, fallback.opacity, 0, 1),
  zIndex: Math.round(clampFinite(input?.zIndex, fallback.zIndex, -100, 100)),
})

export interface TheaterTransformStyle {
  position: 'absolute'
  left: string
  top: string
  width: string
  height: string
  transform: string
  transformOrigin: 'center center'
  opacity: string
  zIndex: string
}

const formatPercentage = (value: number) => `${Number((value * 100).toFixed(6))}%`

export const resolveTheaterTransformStyle = (input: TheaterTransform): TheaterTransformStyle => {
  const transform = normalizeTheaterTransform(input)
  return {
    position: 'absolute',
    left: formatPercentage(transform.x),
    top: formatPercentage(transform.y),
    width: formatPercentage(transform.width),
    height: formatPercentage(transform.height),
    transform: `rotate(${transform.rotation}deg)`,
    transformOrigin: 'center center',
    opacity: String(transform.opacity),
    zIndex: String(transform.zIndex),
  }
}
