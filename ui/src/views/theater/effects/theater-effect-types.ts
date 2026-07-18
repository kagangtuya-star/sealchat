import type { StageImageRef, StageObject } from '../shared/stage-types'

export const THEATER_EFFECT_DESIGN_WIDTH = 1920
export const THEATER_EFFECT_DESIGN_HEIGHT = 1080

export const theaterBuiltinEffectThemes = [
  'brush',
  'cyber',
  'cinematic',
  'impact',
  'glitch',
  'neon',
  'cleave',
  'eclipse',
] as const

export type TheaterBuiltinEffectTheme = typeof theaterBuiltinEffectThemes[number]
export type TheaterEffectKind = 'media' | 'builtin'

export interface TheaterEffectMediaTransform {
  x: number
  y: number
  scale: number
  rotation: number
  mirror: boolean
}

export interface TheaterEffectBuiltinConfig {
  theme: TheaterBuiltinEffectTheme
  format: 'popout' | 'boxed'
  text: string
  subText: string
  accentColor: string
  mainTextColor: string
  subTextColor: string
  dimIntensity: number
  shakeIntensity: number
  mediaTransform: TheaterEffectMediaTransform
}

export interface TheaterEffectAudioRef {
  assetId: string
  name: string
  volume: number
}

export interface TheaterEffectConfig {
  version: 1
  kind: TheaterEffectKind
  keywords: string[]
  targetActorName: string | null
  durationMs: number
  cooldownMs: number
  media: StageImageRef | null
  audio: TheaterEffectAudioRef | null
  builtin: TheaterEffectBuiltinConfig
}

const finiteRange = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
)

const text = (value: unknown, fallback = '', maximum = 512) => (
  typeof value === 'string' ? value.slice(0, maximum) : fallback
)

const color = (value: unknown, fallback: string) => {
  const normalized = text(value, '', 64).trim()
  return normalized || fallback
}

export const createDefaultTheaterEffectConfig = (kind: TheaterEffectKind = 'builtin'): TheaterEffectConfig => ({
  version: 1,
  kind,
  keywords: [],
  targetActorName: null,
  durationMs: 3500,
  cooldownMs: 0,
  media: null,
  audio: null,
  builtin: {
    theme: 'brush',
    format: 'popout',
    text: 'CRITICAL HIT',
    subText: '',
    accentColor: '#e61c34',
    mainTextColor: '#ffffff',
    subTextColor: '#000000',
    dimIntensity: 70,
    shakeIntensity: 0,
    mediaTransform: {
      x: 0,
      y: 0,
      scale: 1,
      rotation: 0,
      mirror: false,
    },
  },
})

export const normalizeTheaterEffectConfig = (input: unknown): TheaterEffectConfig => {
  const fallback = createDefaultTheaterEffectConfig()
  const value = input && typeof input === 'object' ? input as Partial<TheaterEffectConfig> : {}
  const builtin = value.builtin && typeof value.builtin === 'object' ? value.builtin as Partial<TheaterEffectBuiltinConfig> : {}
  const mediaTransform = builtin.mediaTransform && typeof builtin.mediaTransform === 'object'
    ? builtin.mediaTransform as Partial<TheaterEffectMediaTransform>
    : {}
  const theme = theaterBuiltinEffectThemes.includes(builtin.theme as TheaterBuiltinEffectTheme)
    ? builtin.theme as TheaterBuiltinEffectTheme
    : fallback.builtin.theme
  const keywords = Array.isArray(value.keywords)
    ? [...new Set(value.keywords
      .filter((item): item is string => typeof item === 'string')
      .map((item) => item.trim())
      .filter(Boolean))].slice(0, 32)
    : []
  const media = value.media && typeof value.media === 'object' && typeof value.media.url === 'string'
    ? value.media as StageImageRef
    : null
  const rawAudio = value.audio && typeof value.audio === 'object'
    ? value.audio as Partial<TheaterEffectAudioRef>
    : null
  const audio = rawAudio && typeof rawAudio.assetId === 'string' && rawAudio.assetId.trim()
    ? {
        assetId: rawAudio.assetId.trim().slice(0, 256),
        name: text(rawAudio.name, '', 512),
        volume: finiteRange(rawAudio.volume, 1, 0, 1),
      }
    : null

  return {
    version: 1,
    kind: value.kind === 'media' ? 'media' : 'builtin',
    keywords,
    targetActorName: typeof value.targetActorName === 'string' && value.targetActorName.trim()
      ? value.targetActorName.trim().slice(0, 512)
      : null,
    durationMs: Math.round(finiteRange(value.durationMs, fallback.durationMs, 300, 30_000)),
    cooldownMs: Math.round(finiteRange(value.cooldownMs, fallback.cooldownMs, 0, 300_000)),
    media,
    audio,
    builtin: {
      theme,
      format: builtin.format === 'boxed' ? 'boxed' : 'popout',
      text: text(builtin.text, fallback.builtin.text, 512),
      subText: text(builtin.subText, '', 512),
      accentColor: color(builtin.accentColor, fallback.builtin.accentColor),
      mainTextColor: color(builtin.mainTextColor, fallback.builtin.mainTextColor),
      subTextColor: color(builtin.subTextColor, fallback.builtin.subTextColor),
      dimIntensity: finiteRange(builtin.dimIntensity, fallback.builtin.dimIntensity, 0, 100),
      shakeIntensity: finiteRange(builtin.shakeIntensity, fallback.builtin.shakeIntensity, 0, 10),
      mediaTransform: {
        x: finiteRange(mediaTransform.x, 0, -1920, 1920),
        y: finiteRange(mediaTransform.y, 0, -1080, 1080),
        scale: finiteRange(mediaTransform.scale, 1, 0.1, 5),
        rotation: finiteRange(mediaTransform.rotation, 0, -360, 360),
        mirror: mediaTransform.mirror === true,
      },
    },
  }
}

export type TheaterEffectObject = StageObject & { type: 'effect' }

export const isTheaterEffectObject = (object: StageObject | null | undefined): object is TheaterEffectObject => (
  object?.type === 'effect'
)

export const theaterEffectConfigFromObject = (object: StageObject): TheaterEffectConfig => (
  normalizeTheaterEffectConfig(object.content?.effect)
)

export const setTheaterEffectConfig = (object: StageObject, config: TheaterEffectConfig) => {
  object.content = {
    ...object.content,
    effect: normalizeTheaterEffectConfig(config),
  }
}
