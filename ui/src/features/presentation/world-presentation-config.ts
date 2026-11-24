import {
  createDefaultTheaterEffectConfig,
  normalizeTheaterEffectConfig,
  type TheaterEffectKind,
} from '@/views/theater/effects/theater-effect-types'
import { normalizeTheaterSequences, type TheaterSequence } from '@/views/theater/sequences/theater-sequence-types'
import { isSafeStageImageUrl, setStageObjectMediaFx, type StageImageRef, type StageObject } from '@/views/theater/shared/stage-types'

export const WORLD_PRESENTATION_MAX_EFFECTS = 64

export interface WorldPresentationConfig {
  version: 1
  effects: StageObject[]
  sequences: TheaterSequence[]
}

const record = (value: unknown): Record<string, unknown> => (
  value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
)

export const createWorldEffect = (id: string, kind: TheaterEffectKind, order: number): StageObject => ({
  id,
  type: 'effect',
  name: kind === 'media' ? '新建媒体特效' : kind === 'web' ? '新建网页特效' : '新建内置特效',
  transform: { x: 960, y: 540, width: 1600, height: 900, rotation: 0, scaleX: 1, scaleY: 1, z: 0, order },
  parentId: null,
  coordinateSpace: 'world',
  viewportX: 0.5,
  viewportY: 0.5,
  visible: true,
  locked: false,
  aspectRatioLocked: false,
  interactive: false,
  editable: false,
  fill: '#60a5fa',
  content: { effect: createDefaultTheaterEffectConfig(kind) },
  metadata: {},
  actions: [],
})

const normalizeMedia = (input: unknown, fallbackId: string): StageImageRef | null => {
  const media = record(input)
  const url = typeof media.url === 'string' ? media.url.trim() : ''
  if (!url || url.length > 8192 || !isSafeStageImageUrl(url)) return null
  return {
    url,
    resourceId: typeof media.resourceId === 'string' ? media.resourceId.trim().slice(0, 128) || fallbackId : fallbackId,
    ...(typeof media.alt === 'string' ? { alt: media.alt.slice(0, 512) } : {}),
    ...(typeof media.mimeType === 'string' ? { mimeType: media.mimeType.trim().toLowerCase().slice(0, 128) } : {}),
    ...(media.animated === true ? { animated: true } : {}),
    ...(typeof media.loopCount === 'number' && Number.isInteger(media.loopCount) && media.loopCount > 0
      ? { loopCount: Math.min(65_535, media.loopCount) }
      : {}),
  }
}

export const normalizeWorldPresentationConfig = (input: unknown): WorldPresentationConfig => {
  const config = record(input)
  const effects: StageObject[] = []
  const seen = new Set<string>()
  for (const raw of Array.isArray(config.effects) ? config.effects : []) {
    const candidate = record(raw)
    const id = typeof candidate.id === 'string' ? Array.from(candidate.id.trim()).slice(0, 128).join('') : ''
    if (!id || candidate.type !== 'effect' || seen.has(id)) continue
    const effect = normalizeTheaterEffectConfig(record(candidate.content).effect)
    const object = createWorldEffect(id, effect.kind, effects.length)
    effect.media = normalizeMedia(effect.media, id)
    object.image = normalizeMedia(candidate.image, id) || undefined
    object.content = { effect }
    const transform = record(candidate.transform)
    for (const key of Object.keys(object.transform) as Array<keyof StageObject['transform']>) {
      const value = transform[key]
      if (typeof value === 'number' && Number.isFinite(value) &&
        (!['width', 'height', 'scaleX', 'scaleY'].includes(key) || value > 0)) {
        object.transform[key] = Math.max(-10_000, Math.min(10_000, value))
      }
    }
    if (typeof candidate.name === 'string') object.name = candidate.name.slice(0, 512)
    object.visible = candidate.visible !== false
    if (record(candidate.metadata).mediaFx !== undefined) setStageObjectMediaFx(object, record(candidate.metadata).mediaFx)
    effects.push(object)
    seen.add(id)
    if (effects.length >= WORLD_PRESENTATION_MAX_EFFECTS) break
  }
  return { version: 1, effects, sequences: normalizeTheaterSequences(config.sequences) }
}
