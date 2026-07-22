export const WORLD_UNIT_PX = 24

export type StageObjectFit = 'fill' | 'cover' | 'contain'
export type StageSurfaceFit = StageObjectFit | 'tile' | 'center'
export type StageSurfaceTarget = 'background' | 'foreground'
export type StageObjectType = 'group' | 'drawing' | 'text' | 'image' | 'button' | 'character' | 'video' | 'effect'
export type StageDrawingTool = 'pen' | 'highlighter' | 'line' | 'arrow' | 'rectangle' | 'ellipse' | 'triangle' | 'polygon'
export type StageDrawingDash = 'solid' | 'dashed' | 'dotted'

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

export type StageAction =
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
    type: 'object.toggle'
    payload: {
      objectId: string
    }
  }

export interface StageActionTriggeredPayload {
  objectId: string
  actionId: string
  action: StageAction
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
  gridSize: number
  alignWithGrid: boolean
  sceneObjects: Record<string, StageObject>
  transition: {
    type: 'none' | 'crossfade'
    durationMs: number
  }
  serverState?: Record<string, unknown>
}

export type StageSceneState = StageLiveState

export interface StageScene {
  id: string
  name: string
  switchText: string
  order: number
  locked: boolean
  state: StageSceneState
}

export interface CameraState {
  x: number
  y: number
  zoom: number
}

export interface StageWorkspaceState {
  activeSceneId: string
  liveState: StageLiveState
  scenes: Record<string, StageScene>
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
