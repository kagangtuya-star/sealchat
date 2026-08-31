import type { StageSceneOverlayBlendMode } from '../../../shared/stage-types'
import type { SceneOverlayEffectDefinition } from '../../scene-overlay-types'
import { colorParam } from '../effect-helpers'

interface ColorEffectSpec {
  id: string
  name: string
  description: string
  color: string
  secondaryColor?: string
  mode?: 'solid' | 'fog'
  defaultOpacity: number
  defaultBlendMode: StageSceneOverlayBlendMode
  durationMs?: number
}

export const createColorSceneOverlayEffect = (spec: ColorEffectSpec): SceneOverlayEffectDefinition => ({
  id: spec.id,
  name: spec.name,
  description: spec.description,
  category: 'lighting',
  defaultParams: {
    color: spec.color,
    ...(spec.secondaryColor ? { secondaryColor: spec.secondaryColor } : {}),
  },
  controls: [
    { type: 'color', key: 'color', label: '主色' },
    ...(spec.secondaryColor ? [{ type: 'color' as const, key: 'secondaryColor', label: '辅色' }] : []),
  ],
  defaultOpacity: spec.defaultOpacity,
  defaultBlendMode: spec.defaultBlendMode,
  buildRenderDescriptor(params) {
    return {
      renderer: 'color',
      config: {
        mode: spec.mode || 'solid',
        color: colorParam(params, 'color', spec.color),
        secondaryColor: spec.secondaryColor
          ? colorParam(params, 'secondaryColor', spec.secondaryColor)
          : spec.color,
        durationMs: spec.durationMs,
      },
    }
  },
})
