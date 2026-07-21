import { z } from 'zod'

export const THEATER_PRESENTATION_SCHEMA_VERSION = 2 as const
export const MAX_THEATER_PORTRAIT_DECORATIONS = 16

export const theaterMediaKindSchema = z.enum(['static_image', 'animated_image', 'video'])
export const theaterObjectFitSchema = z.literal('cover')
export const theaterLayerSpaceSchema = z.enum(['viewport', 'portrait', 'dialogue'])
export const theaterBlendModeSchema = z.enum(['normal', 'multiply', 'screen', 'overlay'])
export const theaterColorSchema = z.string().regex(/^#[0-9A-Fa-f]{6}$/)

export const theaterTransformSchema = z.strictObject({
  x: z.number().finite().min(-1).max(2),
  y: z.number().finite().min(-1).max(2),
  width: z.number().finite().min(0.01).max(3),
  height: z.number().finite().min(0.01).max(3),
  rotation: z.number().finite().min(-180).max(180),
  opacity: z.number().finite().min(0).max(1),
  zIndex: z.number().int().min(-100).max(100),
})

const theaterTextTransformSchema = theaterTransformSchema.extend({
  y: z.number().finite().max(2),
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

export const theaterVisualStyleSchema = z.strictObject({
  enabled: z.boolean(),
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
  transform: theaterTextTransformSchema,
  fontScale: z.number().finite().min(0.25).max(4).default(1),
})

const theaterSpeakerTextLayerSchema = theaterTextLayerSchema.extend({
  fontScale: z.number().finite().min(0.25).max(4).default(0.85),
})

const theaterContentTextLayerSchema = theaterTextLayerSchema.extend({
  fontScale: z.number().finite().min(0.25).max(4).default(1.2),
})

export const theaterNarrationStyleSchema = z.strictObject({
  enabled: z.boolean(),
  backdropColor: theaterColorSchema,
  backdropOpacity: z.number().finite().min(0).max(1),
})

export const theaterDialogueStyleSchema = z.strictObject({
  transform: theaterTransformSchema,
  frame: theaterVisualLayerSchema.nullable(),
  speaker: theaterSpeakerTextLayerSchema,
  content: theaterContentTextLayerSchema,
  padding: theaterSpacingSchema,
  nameGap: z.number().finite().min(0).max(1),
  textAlign: z.enum(['left', 'center', 'right']),
  contentColor: theaterColorSchema.default('#F4F4F5'),
  charactersPerSecond: z.number().finite().min(1).max(60).default(10),
}).superRefine((dialogue, context) => {
  if (dialogue.frame && dialogue.frame.space !== 'dialogue') {
    context.addIssue({ code: 'custom', path: ['frame', 'space'], message: 'dialogue frame must use dialogue space' })
  }
})

export const theaterDialogueBoxTemplateSchema = z.strictObject({
  transform: theaterTransformSchema,
  frame: theaterVisualLayerSchema.nullable(),
  padding: theaterSpacingSchema,
  nameGap: z.number().finite().min(0).max(1),
  textAlign: z.enum(['left', 'center', 'right']),
  contentColor: theaterColorSchema,
  charactersPerSecond: z.number().finite().min(1).max(60),
})

export const worldTheaterPresentationTemplateSchema = z.strictObject({
  portrait: theaterVisualStyleSchema.optional(),
  speaker: theaterTextLayerSchema.optional(),
  content: theaterTextLayerSchema.optional(),
  dialogue: theaterDialogueBoxTemplateSchema.optional(),
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
  narration: theaterNarrationStyleSchema.default({
    enabled: false,
    backdropColor: '#000000',
    backdropOpacity: 1,
  }),
}).superRefine((presentation, context) => {
  if (presentation.portrait && presentation.portrait.space !== 'viewport') {
    context.addIssue({ code: 'custom', path: ['portrait', 'space'], message: 'portrait must use viewport space' })
  }
})

export const theaterPresentationPatchSchema = z.strictObject({
  portrait: theaterVisualLayerSchema.nullable().optional(),
  portraitDecorations: portraitDecorationsSchema.nullable().optional(),
  dialogue: theaterDialogueStyleSchema.nullable().optional(),
  narration: theaterNarrationStyleSchema.nullable().optional(),
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
export type TheaterVisualStyle = z.infer<typeof theaterVisualStyleSchema>
export type TheaterTextLayer = z.infer<typeof theaterTextLayerSchema>
export type TheaterNarrationStyle = z.infer<typeof theaterNarrationStyleSchema>
export type TheaterDialogueStyle = z.infer<typeof theaterDialogueStyleSchema>
export type TheaterPresentation = z.infer<typeof theaterPresentationSchema>
export type TheaterPresentationPatch = z.infer<typeof theaterPresentationPatchSchema>
export type TheaterDialogueBoxTemplate = z.infer<typeof theaterDialogueBoxTemplateSchema>
export type WorldTheaterPresentationTemplate = z.infer<typeof worldTheaterPresentationTemplateSchema>
export type WorldTheaterPresentationTemplateSection = 'portrait' | 'speaker' | 'content' | 'dialogue'
export type ResolvedTheaterPresentation = TheaterPresentation

const cloneTheaterJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

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
    x: 0.05,
    y: 0.69,
    width: 0.9,
    height: 0.28,
    rotation: 0,
    opacity: 1,
    zIndex: 0,
  },
  frame: null,
  speaker: {
    enabled: true,
    transform: { x: 0.025, y: 0.065, width: 0.34, height: 0.12, rotation: 0, opacity: 1, zIndex: 2 },
    fontScale: 0.85,
  },
  content: {
    enabled: true,
    transform: { x: 0.025, y: 0.28, width: 0.95, height: 0.68, rotation: 0, opacity: 1, zIndex: 2 },
    fontScale: 1.2,
  },
  padding: { top: 0.16, right: 0.08, bottom: 0.12, left: 0.08 },
  nameGap: 0.04,
  textAlign: 'left',
  contentColor: '#F4F4F5',
  charactersPerSecond: 10,
})

export const createDefaultTheaterNarrationStyle = (): TheaterNarrationStyle => ({
  enabled: false,
  backdropColor: '#000000',
  backdropOpacity: 1,
})

export const createDefaultTheaterPresentation = (): TheaterPresentation => ({
  schemaVersion: THEATER_PRESENTATION_SCHEMA_VERSION,
  portrait: null,
  portraitDecorations: [],
  dialogue: createDefaultTheaterDialogueStyle(),
  narration: createDefaultTheaterNarrationStyle(),
})

export const theaterVisualStyleFromLayer = (layer: TheaterVisualLayer | null): TheaterVisualStyle | undefined => (
  layer
    ? cloneTheaterJson({
        enabled: layer.enabled,
        transform: layer.transform,
        fit: layer.fit,
        playbackRate: layer.playbackRate,
        blendMode: layer.blendMode,
      })
    : undefined
)

export const theaterDialogueBoxTemplateFromStyle = (dialogue: TheaterDialogueStyle): TheaterDialogueBoxTemplate => ({
  transform: cloneTheaterJson(dialogue.transform),
  frame: cloneTheaterJson(dialogue.frame),
  padding: cloneTheaterJson(dialogue.padding),
  nameGap: dialogue.nameGap,
  textAlign: dialogue.textAlign,
  contentColor: dialogue.contentColor,
  charactersPerSecond: dialogue.charactersPerSecond,
})

const applyTheaterVisualStyle = (layer: TheaterVisualLayer | null, style?: TheaterVisualStyle | null) => {
  if (!layer || !style) return
  layer.enabled = style.enabled
  layer.transform = cloneTheaterJson(style.transform)
  layer.fit = style.fit
  layer.playbackRate = style.playbackRate
  layer.blendMode = style.blendMode
}

export const applyWorldTheaterPresentationTemplate = (
  presentation: TheaterPresentation,
  template?: WorldTheaterPresentationTemplate | null,
): TheaterPresentation => {
  const next = normalizeTheaterPresentation(cloneTheaterJson(presentation))
  if (!template) return next
  applyTheaterVisualStyle(next.portrait, template.portrait)
  if (template.speaker) next.dialogue.speaker = cloneTheaterJson(template.speaker)
  if (template.content) next.dialogue.content = cloneTheaterJson(template.content)
  if (template.dialogue) {
    next.dialogue.transform = cloneTheaterJson(template.dialogue.transform)
    next.dialogue.frame = cloneTheaterJson(template.dialogue.frame)
    next.dialogue.padding = cloneTheaterJson(template.dialogue.padding)
    next.dialogue.nameGap = template.dialogue.nameGap
    next.dialogue.textAlign = template.dialogue.textAlign
    next.dialogue.contentColor = template.dialogue.contentColor
    next.dialogue.charactersPerSecond = template.dialogue.charactersPerSecond
  }
  return normalizeTheaterPresentation(next)
}

export const mergeWorldTheaterPresentationTemplate = (
  current: WorldTheaterPresentationTemplate | null | undefined,
  presentation: TheaterPresentation,
  sections: WorldTheaterPresentationTemplateSection[],
): WorldTheaterPresentationTemplate => {
  const next = worldTheaterPresentationTemplateSchema.parse(current || {})
  if (sections.includes('portrait')) {
    const portrait = theaterVisualStyleFromLayer(presentation.portrait)
    if (portrait) next.portrait = portrait
    else delete next.portrait
  }
  if (sections.includes('speaker')) next.speaker = cloneTheaterJson(presentation.dialogue.speaker)
  if (sections.includes('content')) next.content = cloneTheaterJson(presentation.dialogue.content)
  if (sections.includes('dialogue')) next.dialogue = theaterDialogueBoxTemplateFromStyle(presentation.dialogue)
  return worldTheaterPresentationTemplateSchema.parse(next)
}

const matchesTransform = (
  transform: TheaterTransform,
  expected: Pick<TheaterTransform, 'x' | 'y' | 'width' | 'height'>,
) => transform.x === expected.x
  && transform.y === expected.y
  && transform.width === expected.width
  && transform.height === expected.height

const migrateLegacyDefaultDialogue = (dialogue: TheaterDialogueStyle): TheaterDialogueStyle => {
  const migrated = structuredClone(dialogue)
  const legacyOuter = matchesTransform(migrated.transform, { x: 0.02, y: 0.69, width: 0.96, height: 0.28 })
  const firstRevisionOuter = matchesTransform(migrated.transform, { x: 0.1, y: 0.69, width: 0.8, height: 0.28 })
  const secondRevisionOuter = matchesTransform(migrated.transform, { x: 0.05, y: 0.69, width: 0.9, height: 0.28 })
  if (!legacyOuter && !firstRevisionOuter && !secondRevisionOuter) return migrated

  migrated.transform.x = 0.05
  migrated.transform.width = 0.9
  const previousSpeaker = legacyOuter
    ? { x: 0.08, y: 0.12, width: 0.34, height: 0.12 }
    : { x: 0.075, y: 0.12, width: 0.34, height: 0.12 }
  if (
    matchesTransform(migrated.speaker.transform, previousSpeaker)
    && migrated.speaker.fontScale === (secondRevisionOuter ? 0.85 : 1)
  ) {
    migrated.speaker.transform = {
      ...migrated.speaker.transform,
      x: 0.025,
      y: 0.065,
      width: 0.34,
      height: 0.12,
    }
    migrated.speaker.fontScale = 0.85
  }
  if (
    matchesTransform(migrated.content.transform, legacyOuter
      ? { x: 0.08, y: 0.3, width: 0.84, height: 0.56 }
      : { x: 0.075, y: 0.3, width: 0.85, height: 0.56 })
    && (legacyOuter ? migrated.content.fontScale === 1 : migrated.content.fontScale === 1.2)
  ) {
    migrated.content.transform = {
      ...migrated.content.transform,
      x: 0.025,
      y: 0.28,
      width: 0.95,
      height: 0.68,
    }
    migrated.content.fontScale = 1.2
  }
  return migrated
}

export const normalizeTheaterPresentation = (input: unknown): TheaterPresentation => {
  if (!input || typeof input !== 'object') return createDefaultTheaterPresentation()
  const value = input as Partial<TheaterPresentation>
  if (value.schemaVersion !== THEATER_PRESENTATION_SCHEMA_VERSION) return createDefaultTheaterPresentation()
  const normalized = theaterPresentationSchema.parse({
    schemaVersion: THEATER_PRESENTATION_SCHEMA_VERSION,
    portrait: value.portrait ?? null,
    portraitDecorations: value.portraitDecorations ?? [],
    dialogue: value.dialogue ?? createDefaultTheaterDialogueStyle(),
    narration: value.narration ?? createDefaultTheaterNarrationStyle(),
  })
  normalized.dialogue = migrateLegacyDefaultDialogue(normalized.dialogue)
  return normalized
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
  if (validatedPatch.narration !== undefined) {
    resolved.narration = structuredClone(validatedPatch.narration ?? createDefaultTheaterNarrationStyle())
  }
  return normalizeTheaterPresentation(resolved)
}

const clampFinite = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
)

export const resolveTheaterBackdropColor = (color: string, opacity: number) => {
  const normalized = theaterColorSchema.safeParse(color)
  const hex = normalized.success ? normalized.data.slice(1) : '000000'
  const red = Number.parseInt(hex.slice(0, 2), 16)
  const green = Number.parseInt(hex.slice(2, 4), 16)
  const blue = Number.parseInt(hex.slice(4, 6), 16)
  const alpha = clampFinite(opacity, 1, 0, 1)
  return 'rgba(' + red + ', ' + green + ', ' + blue + ', ' + alpha + ')'
}

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

export const normalizeTheaterTextTransform = (
  input: Partial<TheaterTransform> | null | undefined,
  fallback: TheaterTransform = createDefaultTheaterTransform(),
): TheaterTransform => ({
  ...normalizeTheaterTransform(input, fallback),
  y: typeof input?.y === 'number' && Number.isFinite(input.y)
    ? Math.min(2, input.y)
    : fallback.y,
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

const resolveNormalizedTheaterTransformStyle = (transform: TheaterTransform): TheaterTransformStyle => {
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

export const resolveTheaterTransformStyle = (input: TheaterTransform): TheaterTransformStyle => (
  resolveNormalizedTheaterTransformStyle(normalizeTheaterTransform(input))
)

export const resolveTheaterTextTransformStyle = (input: TheaterTransform): TheaterTransformStyle => (
  resolveNormalizedTheaterTransformStyle(normalizeTheaterTextTransform(input))
)
