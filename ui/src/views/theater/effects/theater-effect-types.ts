import { normalizeStageAudioRef, type StageAudioRef, type StageImageRef, type StageObject } from '../shared/stage-types'

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
export type TheaterEffectKind = 'media' | 'builtin' | 'web'

// Raw UTF-8 bytes of the authored HTML document.
export const THEATER_EFFECT_WEB_HTML_MAX_BYTES = 128 * 1024

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

// A complete HTML document authored by the user. It is untrusted active content and
// only ever runs inside the sandboxed TheaterEffectWebFrame host.
export interface TheaterEffectWebConfig {
  html: string
}

export type TheaterEffectAudioRef = StageAudioRef

export interface TheaterEffectConfig {
  version: 1
  kind: TheaterEffectKind
  keywords: string[]
  targetActorName: string | null
  durationMs: number
  fadeOut: boolean
  cooldownMs: number
  media: StageImageRef | null
  mediaLoopCount?: number
  audio: TheaterEffectAudioRef | null
  builtin: TheaterEffectBuiltinConfig
  web?: TheaterEffectWebConfig
}

const finiteRange = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
)

const text = (value: unknown, fallback = '', maximum = 512) => (
  typeof value === 'string' ? value.slice(0, maximum) : fallback
)

const theaterEffectWebTextEncoder = new TextEncoder()

export const theaterEffectWebHtmlBytes = (html: string) => theaterEffectWebTextEncoder.encode(html).length

const truncateTheaterEffectWebHtml = (html: string) => {
  if (theaterEffectWebHtmlBytes(html) <= THEATER_EFFECT_WEB_HTML_MAX_BYTES) return html
  const chunks: string[] = []
  let bytes = 0
  for (const character of html) {
    const size = theaterEffectWebTextEncoder.encode(character).length
    if (bytes + size > THEATER_EFFECT_WEB_HTML_MAX_BYTES) break
    chunks.push(character)
    bytes += size
  }
  return chunks.join('')
}

const DEFAULT_THEATER_EFFECT_WEB_HTML = `<!doctype html>
<html>
<head>
<style>
  body { display: grid; place-items: center; }
  h1 { margin: 0; color: #fff; font: 900 160px/1 sans-serif; text-shadow: 0 0 24px #38bdf8, 0 0 64px #6366f1; animation: pop 1.2s ease-out both; }
  @keyframes pop { from { opacity: 0; transform: scale(.6); } to { opacity: 1; transform: none; } }
</style>
</head>
<body>
  <h1>EFFECT</h1>
</body>
</html>
`

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
  fadeOut: false,
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
  ...(kind === 'web' ? { web: { html: DEFAULT_THEATER_EFFECT_WEB_HTML } } : {}),
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
  const mediaLoopCount = typeof value.mediaLoopCount === 'number'
    && Number.isInteger(value.mediaLoopCount)
    && value.mediaLoopCount > 0
    ? Math.min(65_535, value.mediaLoopCount)
    : undefined
  const audio = normalizeStageAudioRef(value.audio)
  const kind: TheaterEffectKind = value.kind === 'media' || value.kind === 'web' ? value.kind : 'builtin'
  const webInput = value.web && typeof value.web === 'object' ? value.web as Partial<TheaterEffectWebConfig> : null
  // Keep web code when switching kinds so switching back does not lose it.
  const webHtml = typeof webInput?.html === 'string' ? webInput.html : ''
  const web = kind === 'web' || webInput ? { html: truncateTheaterEffectWebHtml(webHtml) } : null

  return {
    version: 1,
    kind,
    keywords,
    targetActorName: typeof value.targetActorName === 'string' && value.targetActorName.trim()
      ? value.targetActorName.trim().slice(0, 512)
      : null,
    durationMs: Math.round(finiteRange(value.durationMs, fallback.durationMs, 300, 30_000)),
    fadeOut: value.fadeOut === true,
    cooldownMs: Math.round(finiteRange(value.cooldownMs, fallback.cooldownMs, 0, 300_000)),
    media,
    ...(mediaLoopCount ? { mediaLoopCount } : {}),
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
    ...(web ? { web } : {}),
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
