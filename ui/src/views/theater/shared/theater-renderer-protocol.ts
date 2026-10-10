import { z } from 'zod'
import type { CameraState, StageSequenceTiming } from './stage-types'

export const THEATER_RENDERER_VERSION = 1
export const theaterRendererScopeSchema = z.strictObject({
  worldId: z.string().min(1).max(100),
  scopeType: z.enum(['world', 'channel']),
  channelId: z.string().max(100).optional(),
  inputChannelId: z.string().max(100).optional(),
}).superRefine((scope, ctx) => {
  if ((scope.scopeType === 'world' && scope.channelId) || (scope.scopeType === 'channel' && !scope.channelId)) {
    ctx.addIssue({ code: 'custom', message: 'Invalid Theater room scope' })
  }
})
export type TheaterRendererScope = z.infer<typeof theaterRendererScopeSchema>
export const theaterRendererCommandSchema = z.strictObject({
  version: z.literal(THEATER_RENDERER_VERSION),
  requestId: z.string().min(1).max(128),
  rendererId: z.string().min(1).max(128),
  scope: theaterRendererScopeSchema,
  sceneId: z.string().max(128),
  expectedRevision: z.number().int().nonnegative(),
  expiresAt: z.number().int().positive(),
  operation: z.enum(['capture', 'get', 'camera_set', 'fit_scene', 'focus_object', 'select_objects', 'clear_selection', 'execute', 'cancel']),
  payload: z.unknown(),
})
export type TheaterRendererCommand = z.infer<typeof theaterRendererCommandSchema>
export interface TheaterRendererView {
  width: number
  height: number
  camera: CameraState
  selectedObjectIds: string[]
}
export interface TheaterExecutionPlan {
  id: string
  delayMs: number
  timing: StageSequenceTiming
  children?: TheaterExecutionPlan[]
  stepId?: string
  confirm?: boolean
}
export interface TheaterCaptureResult {
  captureId: string
  rendererId: string
  sceneId: string
  revision: number
  status: 'complete' | 'partial'
  width: number
  height: number
  mimeType: string
  coordinateMapping: {
    worldUnitPx: 24
    anchor: 'center'
    worldOriginPx: { x: number, y: number }
    pixelsPerWorldUnit: number
    camera: CameraState
    crop: { x: number, y: number, width: number, height: number }
    effectDesignSize: { width: 1920, height: 1080 }
  }
  includedLayers: string[]
  missingLayers: string[]
  unsupportedObjects: string[]
  warnings: string[]
  data: string
}
export const sameRendererScope = (a: TheaterRendererScope, b: TheaterRendererScope) => (
  a.worldId === b.worldId && a.scopeType === b.scopeType && (a.channelId || '') === (b.channelId || '')
)
