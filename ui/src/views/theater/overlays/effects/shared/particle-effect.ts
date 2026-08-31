import type { StageSceneOverlayBlendMode, StageSceneOverlayParams } from '../../../shared/stage-types'
import type { SceneOverlayCategory, SceneOverlayEffectDefinition } from '../../scene-overlay-types'
import { colorParam, numberParam, particleConfig } from '../effect-helpers'
import { particleMotionControls, type ParticleMotionControlOverrides } from './particle-controls'
import {
  normalizeParticleMotionParams,
  particleMotionDirection,
  type ParticleMotionDefaults,
  type ParticleMotionProfile,
} from './particle-motion'

interface ParticleEffectSpec {
  id: string
  name: string
  description: string
  category: Extract<SceneOverlayCategory, 'weather' | 'environment' | 'magic'>
  color: string
  count: number
  opacity: { min: number, max: number }
  size: { min: number, max: number }
  sizeControl?: { min: number, max: number }
  baseSpeed: number
  motion: ParticleMotionProfile
  defaults: ParticleMotionDefaults
  controls?: ParticleMotionControlOverrides
  shape?: 'circle' | 'line' | 'square' | 'star'
  opacityAnimationSpeed?: number
  sizeAnimationSpeed?: number
  defaultOpacity?: number
  defaultBlendMode?: StageSceneOverlayBlendMode
  fpsLimit?: number
}

export const createParticleSceneOverlayEffect = (spec: ParticleEffectSpec): SceneOverlayEffectDefinition => ({
  id: spec.id,
  name: spec.name,
  description: spec.description,
  category: spec.category,
  defaultParams: { ...spec.defaults, ...(spec.sizeControl ? { size: 1 } : {}), color: spec.color },
  controls: [
    ...particleMotionControls(spec.controls),
    ...(spec.sizeControl ? [{ type: 'number' as const, key: 'size', label: '大小', min: spec.sizeControl.min, max: spec.sizeControl.max, step: 0.1 }] : []),
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: spec.defaultOpacity ?? 0.8,
  defaultBlendMode: spec.defaultBlendMode,
  buildRenderDescriptor(params: StageSceneOverlayParams) {
    const motion = normalizeParticleMotionParams(params, spec.defaults)
    const sizeScale = spec.sizeControl ? numberParam(params, 'size', 1, spec.sizeControl.min, spec.sizeControl.max) : 1
    return {
      renderer: 'particles',
      config: particleConfig({
        count: spec.count * motion.intensity,
        color: colorParam(params, 'color', spec.color),
        opacity: spec.opacity,
        size: { min: spec.size.min * sizeScale, max: spec.size.max * sizeScale },
        speed: spec.baseSpeed * motion.speed,
        direction: particleMotionDirection(motion, spec.motion),
        spread: motion.spread,
        shape: spec.shape,
        opacityAnimationSpeed: spec.opacityAnimationSpeed,
        sizeAnimationSpeed: spec.sizeAnimationSpeed,
        fpsLimit: spec.fpsLimit,
      }),
    }
  },
})
