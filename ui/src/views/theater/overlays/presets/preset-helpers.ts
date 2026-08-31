import type { StageSceneOverlayParams } from '../../shared/stage-types'
import type { SceneOverlayPresetDefinition, SceneOverlayPresetItem } from './scene-overlay-preset-types'

export const defineSceneOverlayPreset = (definition: SceneOverlayPresetDefinition) => definition

export const presetItem = (
  effectId: string,
  opacity: number,
  params?: StageSceneOverlayParams,
): SceneOverlayPresetItem => ({ effectId, opacity, ...(params ? { params } : {}) })
