export const WORLD_UNIT_PX = 24

export type StageObjectFit = 'fill' | 'cover' | 'contain'
export type StageObjectType = 'group' | 'shape' | 'text' | 'image' | 'button' | 'character' | 'video'

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

export interface StageImageRef {
  resourceId: string
  url: string
  alt?: string
}

export interface StageObjectTransform {
  x: number
  y: number
  width: number
  height: number
  rotation: number
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
  sizeLocked: boolean
  interactive: boolean
  editable: boolean
  fill: string
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
