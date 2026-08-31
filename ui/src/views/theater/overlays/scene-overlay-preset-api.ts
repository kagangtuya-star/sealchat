import { api } from '@/stores/_config'
import type {
  StageSceneOverlayBlendMode,
  StageSceneOverlayLayer,
  StageSceneOverlayMediaRef,
  StageSceneOverlayParamValue,
} from '../shared/stage-types'

export interface TheaterSceneOverlayPresetItem {
  effectId: string
  name?: string
  enabled?: boolean
  opacity?: number
  blendMode?: StageSceneOverlayBlendMode
  layer?: StageSceneOverlayLayer
  media?: StageSceneOverlayMediaRef
  params?: Record<string, StageSceneOverlayParamValue>
}

export interface TheaterSceneOverlayPreset {
  id: string
  name: string
  description: string
  tags: string[]
  overlays: TheaterSceneOverlayPresetItem[]
  revision: number
  createdAt?: string
  updatedAt?: string
}

export interface TheaterSceneOverlayPresetScope {
  worldId: string
  channelId: string
  scopeType?: 'channel' | 'world'
}

const basePath = (scope: TheaterSceneOverlayPresetScope) => scope.scopeType === 'world'
  ? `api/v1/worlds/${encodeURIComponent(scope.worldId)}/theater/scene-overlay-presets`
  : `api/v1/worlds/${encodeURIComponent(scope.worldId)}/channels/${encodeURIComponent(scope.channelId)}/theater/scene-overlay-presets`

export const listTheaterSceneOverlayPresets = async (scope: TheaterSceneOverlayPresetScope) => {
  const response = await api.get<{ presets: TheaterSceneOverlayPreset[] }>(basePath(scope))
  return response.data.presets || []
}

export const createTheaterSceneOverlayPreset = async (scope: TheaterSceneOverlayPresetScope, input: Omit<TheaterSceneOverlayPreset, 'id' | 'revision' | 'createdAt' | 'updatedAt'>) => {
  const response = await api.post<{ preset: TheaterSceneOverlayPreset }>(basePath(scope), input)
  return response.data.preset
}

export const updateTheaterSceneOverlayPreset = async (scope: TheaterSceneOverlayPresetScope, id: string, patch: Partial<Omit<TheaterSceneOverlayPreset, 'id'>> & { revision: number }) => {
  const response = await api.patch<{ preset: TheaterSceneOverlayPreset }>(`${basePath(scope)}/${encodeURIComponent(id)}`, patch)
  return response.data.preset
}

export const deleteTheaterSceneOverlayPreset = async (scope: TheaterSceneOverlayPresetScope, id: string) => {
  await api.delete(`${basePath(scope)}/${encodeURIComponent(id)}`)
}
