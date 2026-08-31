export const WORLD_UNIT_PX = 24

export type StageObjectFit = 'fill' | 'cover' | 'contain'
export type StageSurfaceFit = StageObjectFit | 'tile' | 'center'
export type StageSurfaceTarget = 'background' | 'foreground'
export type StageObjectType = 'group' | 'drawing' | 'text' | 'image' | 'button' | 'character' | 'video' | 'effect'
export type StageDrawingTool = 'pen' | 'highlighter' | 'line' | 'arrow' | 'rectangle' | 'ellipse' | 'triangle' | 'polygon'
export type StageDrawingDash = 'solid' | 'dashed' | 'dotted'

export interface StageAudioRef {
  assetId: string
  name: string
  volume: number
}

export type StageMusicTrackType = 'music' | 'ambience' | 'sfx'
export type StageMusicPlaylistMode = 'single' | 'sequential' | 'shuffle'

export interface StageMusicAssetRef {
  assetId: string
  name: string
}

export interface StageMusicTrackSnapshot {
  type: StageMusicTrackType
  asset: StageMusicAssetRef | null
  volume: number
  fadeIn: number
  fadeOut: number
  loopEnabled: boolean
  playbackRate: number
  playlistMode: StageMusicPlaylistMode | null
  playlist: StageMusicAssetRef[]
  playlistIndex: number
}

export interface StageMusicSnapshot {
  version: 1
  tracks: StageMusicTrackSnapshot[]
}

const stageMusicTrackTypes: StageMusicTrackType[] = ['music', 'ambience', 'sfx']
const stageMusicPlaylistModes: StageMusicPlaylistMode[] = ['single', 'sequential', 'shuffle']

const normalizeStageMusicAssetRef = (input: unknown): StageMusicAssetRef | null => {
  if (!input || typeof input !== 'object') return null
  const value = input as Partial<StageMusicAssetRef>
  const assetId = typeof value.assetId === 'string' ? value.assetId.trim().slice(0, 256) : ''
  if (!assetId) return null
  return {
    assetId,
    name: typeof value.name === 'string' ? Array.from(value.name).slice(0, 128).join('') : '',
  }
}

export const normalizeStageMusicSnapshot = (input: unknown): StageMusicSnapshot | null => {
  if (!input || typeof input !== 'object') return null
  const value = input as Partial<StageMusicSnapshot>
  if (value.version !== 1 || !Array.isArray(value.tracks)) return null
  const tracks = stageMusicTrackTypes.map((type) => {
    const raw = value.tracks!.find((track) => track && typeof track === 'object' && (track as StageMusicTrackSnapshot).type === type)
    const track = raw && typeof raw === 'object' ? raw as Partial<StageMusicTrackSnapshot> : {}
    const playlist = Array.isArray(track.playlist)
      ? track.playlist.map(normalizeStageMusicAssetRef).filter((asset): asset is StageMusicAssetRef => Boolean(asset)).slice(0, 64)
      : []
    const playlistIndex = typeof track.playlistIndex === 'number' && Number.isFinite(track.playlistIndex)
      ? Math.max(0, Math.min(Math.round(track.playlistIndex), Math.max(0, playlist.length - 1)))
      : 0
    return {
      type,
      asset: normalizeStageMusicAssetRef(track.asset),
      volume: typeof track.volume === 'number' && Number.isFinite(track.volume) ? Math.min(1, Math.max(0, track.volume)) : 0.8,
      fadeIn: typeof track.fadeIn === 'number' && Number.isFinite(track.fadeIn) ? Math.min(60_000, Math.max(0, Math.round(track.fadeIn))) : 2_000,
      fadeOut: typeof track.fadeOut === 'number' && Number.isFinite(track.fadeOut) ? Math.min(60_000, Math.max(0, Math.round(track.fadeOut))) : 2_000,
      loopEnabled: track.loopEnabled !== false,
      playbackRate: typeof track.playbackRate === 'number' && Number.isFinite(track.playbackRate)
        ? Math.min(4, Math.max(0.25, track.playbackRate))
        : 1,
      playlistMode: stageMusicPlaylistModes.includes(track.playlistMode as StageMusicPlaylistMode)
        ? track.playlistMode as StageMusicPlaylistMode
        : null,
      playlist,
      playlistIndex,
    }
  })
  return tracks.some((track) => track.asset || track.playlist.length) ? { version: 1, tracks } : null
}

export const stageMusicSnapshotHasContent = (snapshot: StageMusicSnapshot | null | undefined) => (
  Boolean(snapshot?.tracks.some((track) => track.asset || track.playlist.length))
)

export const normalizeStageAudioRef = (input: unknown): StageAudioRef | null => {
  if (!input || typeof input !== 'object') return null
  const value = input as Partial<StageAudioRef>
  const assetId = typeof value.assetId === 'string' ? value.assetId.trim().slice(0, 256) : ''
  if (!assetId) return null
  const volume = typeof value.volume === 'number' && Number.isFinite(value.volume)
    ? Math.min(1, Math.max(0, value.volume))
    : 1
  return {
    assetId,
    name: typeof value.name === 'string' ? Array.from(value.name).slice(0, 512).join('') : '',
    volume,
  }
}

export const isStageActionTarget = (type: StageObjectType) => (
  type === 'drawing' || type === 'text' || type === 'image' || type === 'button'
)

export interface StageDrawingStyle {
  stroke: string
  strokeWidth: number
  opacity: number
  fill: string | null
  dash: StageDrawingDash
}

export interface StageDrawing {
  tool: StageDrawingTool
  style: StageDrawingStyle
  points?: number[]
  sides?: number
  smoothing?: number
}

export const STAGE_ACTION_MAX_DELAY_MS = 10_000
export const STAGE_ACTION_DELAY_STEP_MS = 100

export interface StageActionSchedule {
  delayMs: number
}

export const createDefaultStageActionSchedule = (): StageActionSchedule => ({
  delayMs: 0,
})

const normalizeStageActionScheduleValue = (value: unknown) => {
  const numeric = typeof value === 'number' && Number.isFinite(value) ? value : 0
  const clamped = Math.min(STAGE_ACTION_MAX_DELAY_MS, Math.max(0, numeric))
  return Math.round(clamped / STAGE_ACTION_DELAY_STEP_MS) * STAGE_ACTION_DELAY_STEP_MS
}

export const normalizeStageActionSchedule = (input: unknown): StageActionSchedule => {
  const value = input && typeof input === 'object' ? input as Partial<StageActionSchedule> : {}
  return {
    delayMs: normalizeStageActionScheduleValue(value.delayMs),
  }
}

type StageAtomicActionData =
  | {
    id: string
    type: 'chat.send'
    payload: {
      content: string
      channelId?: string
      characterId?: string
    }
  }
  | {
    id: string
    type: 'chat.random-table'
    payload: {
      name: string
      formula: string
      entries: Array<{
        min: number
        max: number
        text: string
      }>
    }
  }
  | {
    id: string
    type: 'chat.insert'
    payload: {
      content: string
    }
  }
  | {
    id: string
    type: 'scene.apply'
    payload: {
      sceneId: string
    }
  }
  | {
    id: string
    type: 'effect.play'
    payload: {
      effectId: string
    }
  }
  | {
    id: string
    type: 'object.toggle'
    payload: {
      objectId: string
    }
  }

export type StageAtomicAction = StageAtomicActionData & {
  schedule: StageActionSchedule
}

export type StageAtomicActionDescriptor = StageAtomicActionData extends infer Action
  ? Action extends StageAtomicActionData ? Omit<Action, 'id'> : never
  : never

export type StageSequenceTiming =
  | { mode: 'after' }
  | { mode: 'delay', delayMs: number }
  | { mode: 'sync' }

export interface StageSequenceStep {
  id: string
  sceneId: string | null
  timing: StageSequenceTiming
  action: StageAtomicActionDescriptor
}

export interface StageSequenceAction {
  id: string
  type: 'action.sequence'
  schedule: StageActionSchedule
  payload: {
    version: 1
    name: string
    steps: StageSequenceStep[]
  }
}

export type StageAction = StageAtomicAction | StageSequenceAction

export interface StageActionTriggeredPayload {
  objectId: string
  actionId: string
  stepId?: string
  direct?: true
  action: StageAction
  execution?: {
    id: string
    mode: 'parallel' | 'sequential'
    index: number
    total: number
  }
  pointer?: {
    x: number
    y: number
  }
}

export interface StagePointerTraceInput {
  traceId: string
  identityId: string
  variantId: string | null
  points: number[]
  finished: boolean
}

export interface StagePointerTrace {
  traceId: string
  displayName: string
  color: string
  points: number[]
  finished: boolean
}

export interface StageImageRef {
  resourceId: string
  url: string
  alt?: string
  mimeType?: string
  animated?: boolean
  loopCount?: number
}

export const stageImageAnnotationStyles = ['card', 'bubble', 'tag', 'floating', 'footer'] as const
export type StageImageAnnotationStyle = typeof stageImageAnnotationStyles[number]
export const stageImageAnnotationPlacements = ['auto', 'top', 'right', 'bottom', 'left'] as const
export type StageImageAnnotationPlacement = typeof stageImageAnnotationPlacements[number]

export interface StageImageAnnotation {
  version: 1
  enabled: boolean
  text: string
  style: StageImageAnnotationStyle
  placement: StageImageAnnotationPlacement
  fontSize: number
  textColor: string
  backgroundColor: string
  backgroundOpacity: number
  maxWidth: number
  delayMs: number
}

export const createDefaultStageImageAnnotation = (): StageImageAnnotation => ({
  version: 1,
  enabled: true,
  text: '',
  style: 'floating',
  placement: 'auto',
  fontSize: 14,
  textColor: '#ffffff',
  backgroundColor: '#111827',
  backgroundOpacity: 0.65,
  maxWidth: 300,
  delayMs: 100,
})

const stageAnnotationColorPattern = /^#[0-9a-fA-F]{6}$/

export const normalizeStageImageAnnotation = (input: unknown): StageImageAnnotation | undefined => {
  if (!input || typeof input !== 'object') return undefined
  const value = input as Partial<StageImageAnnotation>
  const fallback = createDefaultStageImageAnnotation()
  const numeric = (candidate: unknown, defaultValue: number, min: number, max: number) => (
    typeof candidate === 'number' && Number.isFinite(candidate)
      ? Math.min(max, Math.max(min, candidate))
      : defaultValue
  )
  return {
    version: 1,
    enabled: value.enabled !== false,
    text: typeof value.text === 'string' ? value.text.slice(0, 2_000) : '',
    style: stageImageAnnotationStyles.includes(value.style as StageImageAnnotationStyle)
      ? value.style as StageImageAnnotationStyle
      : fallback.style,
    placement: stageImageAnnotationPlacements.includes(value.placement as StageImageAnnotationPlacement)
      ? value.placement as StageImageAnnotationPlacement
      : fallback.placement,
    fontSize: Math.round(numeric(value.fontSize, fallback.fontSize, 10, 36)),
    textColor: typeof value.textColor === 'string' && stageAnnotationColorPattern.test(value.textColor)
      ? value.textColor.toLowerCase()
      : fallback.textColor,
    backgroundColor: typeof value.backgroundColor === 'string' && stageAnnotationColorPattern.test(value.backgroundColor)
      ? value.backgroundColor.toLowerCase()
      : fallback.backgroundColor,
    backgroundOpacity: numeric(value.backgroundOpacity, fallback.backgroundOpacity, 0, 1),
    maxWidth: Math.round(numeric(value.maxWidth, fallback.maxWidth, 120, 480)),
    delayMs: Math.round(numeric(value.delayMs, fallback.delayMs, 0, 1_000)),
  }
}

export const stageSceneTransitionTypes = [
  'none',
  'fade',
  'slide',
  'dissolve',
  'zoom',
  'mask',
  'flip',
  'blur',
  'rotate',
  'curtain',
] as const
export type StageSceneTransitionType = typeof stageSceneTransitionTypes[number]

export interface StageSceneTransitionPhase {
  type: StageSceneTransitionType
  durationMs: number
}

export interface StageSceneTransition {
  curtain: boolean
  enter: StageSceneTransitionPhase
  exit: StageSceneTransitionPhase
}

export const STAGE_SCENE_TRANSITION_MIN_DURATION_MS = 20
export const STAGE_SCENE_TRANSITION_MAX_DURATION_MS = 5_000

export const createDefaultStageSceneTransition = (): StageSceneTransition => ({
  curtain: true,
  enter: { type: 'none', durationMs: 400 },
  exit: { type: 'none', durationMs: 400 },
})

const normalizeStageSceneTransitionPhase = (input: unknown, fallback: StageSceneTransitionPhase): StageSceneTransitionPhase => {
  const value = input && typeof input === 'object' ? input as Partial<StageSceneTransitionPhase> : {}
  const durationMs = typeof value.durationMs === 'number' && Number.isFinite(value.durationMs)
    ? Math.round(Math.min(STAGE_SCENE_TRANSITION_MAX_DURATION_MS, Math.max(STAGE_SCENE_TRANSITION_MIN_DURATION_MS, value.durationMs)))
    : fallback.durationMs
  return {
    type: stageSceneTransitionTypes.includes(value.type as StageSceneTransitionType)
      ? value.type as StageSceneTransitionType
      : fallback.type,
    durationMs,
  }
}

export const normalizeStageSceneTransition = (input: unknown): StageSceneTransition => {
  const fallback = createDefaultStageSceneTransition()
  const value = input && typeof input === 'object'
    ? input as Partial<StageSceneTransition> & { type?: string, durationMs?: number }
    : {}
  if (value.type === 'crossfade') {
    const phase = normalizeStageSceneTransitionPhase({ type: 'fade', durationMs: value.durationMs }, fallback.enter)
    return { curtain: value.curtain !== false, enter: phase, exit: { ...phase } }
  }
  if (value.type === 'none') return fallback
  return {
    curtain: value.curtain !== false,
    enter: normalizeStageSceneTransitionPhase(value.enter, fallback.enter),
    exit: normalizeStageSceneTransitionPhase(value.exit, fallback.exit),
  }
}

export const stageSceneOverlayBlendModes = [
  'normal',
  'multiply',
  'screen',
  'overlay',
  'darken',
  'lighten',
  'color-dodge',
  'color-burn',
  'hard-light',
  'soft-light',
] as const

export type StageSceneOverlayBlendMode = typeof stageSceneOverlayBlendModes[number]

export const stageSceneOverlayLayers = ['belowCharacters', 'aboveCharacters'] as const

export type StageSceneOverlayLayer = typeof stageSceneOverlayLayers[number]
export type StageSceneOverlayParamValue = string | number | boolean | null
export type StageSceneOverlayParams = Record<string, StageSceneOverlayParamValue>

export interface StageSceneOverlayMediaRef {
  resourceId: string
  variant?: string
  mimeType?: string
  animated?: boolean
  loopCount?: number
}

export interface StageSceneOverlayBinding {
  version: 1
  id: string
  effectId: string
  name: string
  enabled: boolean
  opacity: number
  blendMode: StageSceneOverlayBlendMode
  layer: StageSceneOverlayLayer
  media?: StageSceneOverlayMediaRef
  params: StageSceneOverlayParams
}

const truncateSceneOverlayText = (value: string, maximum: number) => Array.from(value).slice(0, maximum).join('')

const normalizeStageSceneOverlayParams = (input: unknown): StageSceneOverlayParams => {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return {}
  const prototype = Object.getPrototypeOf(input)
  if (prototype !== Object.prototype && prototype !== null) return {}
  const params: StageSceneOverlayParams = {}
  Object.entries(input as Record<string, unknown>).slice(0, 64).forEach(([rawKey, value]) => {
    const key = truncateSceneOverlayText(rawKey.trim(), 64)
    if (!key) return
    if (typeof value === 'string') params[key] = truncateSceneOverlayText(value, 512)
    else if (typeof value === 'number' && Number.isFinite(value)) params[key] = value
    else if (typeof value === 'boolean' || value === null) params[key] = value
  })
  return params
}

const normalizeStageSceneOverlayMedia = (input: unknown): StageSceneOverlayMediaRef | undefined => {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return undefined
  const prototype = Object.getPrototypeOf(input)
  if (prototype !== Object.prototype && prototype !== null) return undefined
  const value = input as Partial<StageSceneOverlayMediaRef>
  const resourceId = typeof value.resourceId === 'string' ? value.resourceId.trim() : ''
  if (!resourceId || Array.from(resourceId).length > 128) return undefined
  const variant = typeof value.variant === 'string'
    ? truncateSceneOverlayText(value.variant.trim(), 64) || 'original'
    : 'original'
  const mimeType = typeof value.mimeType === 'string'
    ? truncateSceneOverlayText(value.mimeType.trim(), 128)
    : ''
  const loopCount = typeof value.loopCount === 'number'
    && Number.isFinite(value.loopCount)
    && Number.isInteger(value.loopCount)
    && value.loopCount > 0
    ? Math.min(65_535, value.loopCount)
    : undefined
  return {
    resourceId,
    variant,
    ...(mimeType ? { mimeType } : {}),
    ...(typeof value.animated === 'boolean' ? { animated: value.animated } : {}),
    ...(loopCount ? { loopCount } : {}),
  }
}

export const normalizeStageSceneOverlays = (input: unknown): StageSceneOverlayBinding[] => {
  if (!Array.isArray(input)) return []
  return input.flatMap((candidate) => {
    if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) return []
    const value = candidate as Partial<StageSceneOverlayBinding>
    const id = typeof value.id === 'string' ? truncateSceneOverlayText(value.id.trim(), 128) : ''
    const effectId = typeof value.effectId === 'string' ? truncateSceneOverlayText(value.effectId.trim(), 128) : ''
    if (!id || !effectId) return []
    const name = typeof value.name === 'string'
      ? truncateSceneOverlayText(value.name, 128)
      : effectId
    return [{
      version: 1 as const,
      id,
      effectId,
      name,
      enabled: value.enabled !== false,
      opacity: typeof value.opacity === 'number' && Number.isFinite(value.opacity)
        ? Math.min(1, Math.max(0, value.opacity))
        : 1,
      blendMode: stageSceneOverlayBlendModes.includes(value.blendMode as StageSceneOverlayBlendMode)
        ? value.blendMode as StageSceneOverlayBlendMode
        : 'normal',
      layer: stageSceneOverlayLayers.includes(value.layer as StageSceneOverlayLayer)
        ? value.layer as StageSceneOverlayLayer
        : 'aboveCharacters',
      media: normalizeStageSceneOverlayMedia(value.media),
      params: normalizeStageSceneOverlayParams(value.params),
    }]
  }).slice(0, 32)
}

export const stageEntrancePresets = ['none', 'fade', 'slide', 'zoom', 'mask'] as const
export type StageEntrancePreset = typeof stageEntrancePresets[number]

export interface StageEntranceConfig {
  preset: StageEntrancePreset
  durationMs: number
}

export interface StageEntrancePlayback extends StageEntranceConfig {
  direction: 'enter' | 'exit'
  token: number
}

export const STAGE_ENTRANCE_MIN_DURATION_MS = 150
export const STAGE_ENTRANCE_MAX_DURATION_MS = 5_000

export const createDefaultStageEntranceConfig = (): StageEntranceConfig => ({
  preset: 'none',
  durationMs: 400,
})

export const normalizeStageEntranceConfig = (input: unknown): StageEntranceConfig => {
  const fallback = createDefaultStageEntranceConfig()
  const value = input && typeof input === 'object' ? input as Partial<StageEntranceConfig> : {}
  const durationMs = typeof value.durationMs === 'number' && Number.isFinite(value.durationMs)
    ? Math.round(Math.min(STAGE_ENTRANCE_MAX_DURATION_MS, Math.max(STAGE_ENTRANCE_MIN_DURATION_MS, value.durationMs)))
    : fallback.durationMs
  return {
    preset: stageEntrancePresets.includes(value.preset as StageEntrancePreset)
      ? value.preset as StageEntrancePreset
      : fallback.preset,
    durationMs,
  }
}

export interface StageSurfaceStyle {
  brightness: number
  blurPx: number
  opacity: number
  zoom: number
  fit: StageSurfaceFit
  overlay: {
    enabled: boolean
    color: string
    opacity: number
  }
}

export type StageSurfaceStylePatch = Partial<Omit<StageSurfaceStyle, 'overlay'>> & {
  overlay?: Partial<StageSurfaceStyle['overlay']>
}

export const createDefaultStageSurfaceStyle = (fit: StageSurfaceFit = 'cover', overrides: Partial<StageSurfaceStyle> = {}): StageSurfaceStyle => ({
  brightness: 1,
  blurPx: 0,
  opacity: 1,
  zoom: 1,
  fit,
  overlay: {
    enabled: false,
    color: '#000000',
    opacity: 0.4,
  },
  ...overrides,
})

const finiteRange = (value: unknown, fallback: number, min: number, max: number) => (
  typeof value === 'number' && Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback
)

export const normalizeStageSurfaceStyle = (
  input: unknown,
  fallbackFit: StageSurfaceFit = 'cover',
  defaults: Partial<StageSurfaceStyle> = {},
): StageSurfaceStyle => {
  const base = createDefaultStageSurfaceStyle(fallbackFit, defaults)
  const value = input && typeof input === 'object' ? input as Partial<StageSurfaceStyle> : {}
  const overlay: Partial<StageSurfaceStyle['overlay']> = value.overlay && typeof value.overlay === 'object'
    ? value.overlay
    : {}
  const fits: StageSurfaceFit[] = ['fill', 'cover', 'contain', 'tile', 'center']
  return {
    brightness: finiteRange(value.brightness, base.brightness, 0, 2),
    blurPx: finiteRange(value.blurPx, base.blurPx, 0, 40),
    opacity: finiteRange(value.opacity, base.opacity, 0, 1),
    zoom: finiteRange(value.zoom, base.zoom, 0.1, 5),
    fit: value.fit && fits.includes(value.fit) ? value.fit : fallbackFit,
    overlay: {
      enabled: overlay.enabled === true,
      color: typeof overlay.color === 'string' && overlay.color.trim() && overlay.color.length <= 64
        ? overlay.color.trim()
        : '#000000',
      opacity: finiteRange(overlay.opacity, base.overlay.opacity, 0, 1),
    },
  }
}

export interface StageObjectTransform {
  x: number
  y: number
  width: number
  height: number
  rotation: number
  scaleX: number
  scaleY: number
  z: number
  order: number
}

export interface StageObject {
  id: string
  parentId: string | null
  type: StageObjectType
  name: string
  transform: StageObjectTransform
  visible: boolean
  locked: boolean
  aspectRatioLocked: boolean
  interactive: boolean
  editable: boolean
  fill: string
  drawing?: StageDrawing
  text?: string
  image?: StageImageRef
  annotation?: StageImageAnnotation
  content?: Record<string, unknown>
  ownerUserId?: string | null
  characterIdentityId?: string | null
  actions: StageAction[]
  metadata: Record<string, unknown>
}

export interface StageLiveState {
  background: StageImageRef | null
  foreground: StageImageRef | null
  surfaceStyles: Record<StageSurfaceTarget, StageSurfaceStyle>
  backgroundColor: string
  fieldWidth: number
  fieldHeight: number
  fieldObjectFit: StageObjectFit
  displayGrid: boolean
  gridOnTop: boolean
  gridSize: number
  alignWithGrid: boolean
  sceneObjects: Record<string, StageObject>
  transition: StageSceneTransition
  switchAudio: StageAudioRef | null
  musicSnapshot: StageMusicSnapshot | null
  sceneOverlays: StageSceneOverlayBinding[]
  serverState?: Record<string, unknown>
}

export type StageSceneState = StageLiveState

export interface StageScene {
  id: string
  name: string
  switchText: string
  order: number
  locked: boolean
  folderId?: string
  state: StageSceneState
}

export interface SceneFolder {
  id: string
  name: string
}

export type StageObjectScope = 'scene' | 'scene-fixed'

export interface CameraState {
  x: number
  y: number
  zoom: number
}

export interface StageWorkspaceState {
  activeSceneId: string
  liveState: StageLiveState
  scenes: Record<string, StageScene>
  sceneFolders: SceneFolder[]
  persistentObjects: Record<string, StageObject>
  camera: CameraState
  selectedObjectId: string | null
}

const explicitSchemePattern = /^[a-zA-Z][a-zA-Z\d+.-]*:/

export const isSafeStageImageUrl = (value: string) => {
  const input = value.trim()
  if (!input) return false
  if (explicitSchemePattern.test(input)) {
    try {
      const protocol = new URL(input).protocol
      return protocol === 'http:' || protocol === 'https:'
    } catch {
      return false
    }
  }
  if (input.startsWith('//')) {
    try {
      const protocol = new URL(input, 'https://sealchat.invalid').protocol
      return protocol === 'http:' || protocol === 'https:'
    } catch {
      return false
    }
  }
  try {
    const resolved = new URL(input, 'https://sealchat.invalid/')
    return resolved.origin === 'https://sealchat.invalid'
  } catch {
    return false
  }
}

export const resolveStageImageUrl = (value: string, baseUrl?: string) => {
  if (!isSafeStageImageUrl(value)) return null
  try {
    const base = baseUrl || (typeof window !== 'undefined' ? window.location.href : 'https://sealchat.invalid/')
    const resolved = new URL(value.trim(), base)
    return resolved.protocol === 'http:' || resolved.protocol === 'https:' ? resolved.href : null
  } catch {
    return null
  }
}
