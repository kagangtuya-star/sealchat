import type { StageSceneOverlayParams } from '../../../shared/stage-types'

import { numberParam } from '../effect-helpers'

export interface ParticleMotionDefaults {
  intensity: number
  speed: number
  windDirection: number
  windStrength: number
  spread: number
}

export interface ParticleMotionProfile {
  baseDirection: number
  baseWeight?: number
  windInfluence: number
}

const normalizedDegrees = (value: number) => ((value % 360) + 360) % 360
const vectorAt = (degrees: number) => {
  const radians = normalizedDegrees(degrees) * Math.PI / 180
  return { x: Math.cos(radians), y: Math.sin(radians) }
}

export const normalizeParticleMotionParams = (
  params: StageSceneOverlayParams,
  defaults: ParticleMotionDefaults,
): ParticleMotionDefaults => {
  const legacyWind = numberParam(params, 'wind', 0, -2, 2)
  const hasWindDirection = typeof params.windDirection === 'number'
  const hasWindStrength = typeof params.windStrength === 'number'
  return {
    intensity: numberParam(params, 'intensity', defaults.intensity, 0, 1),
    speed: numberParam(params, 'speed', defaults.speed, 0.1, 3),
    windDirection: numberParam(
      params,
      'windDirection',
      hasWindDirection ? defaults.windDirection : legacyWind < 0 ? 180 : legacyWind > 0 ? 0 : defaults.windDirection,
      0,
      360,
    ),
    windStrength: numberParam(
      params,
      'windStrength',
      hasWindStrength ? defaults.windStrength : legacyWind === 0 ? defaults.windStrength : Math.min(1, Math.abs(legacyWind) / 2),
      0,
      1,
    ),
    spread: numberParam(params, 'spread', defaults.spread, 0, 1),
  }
}

export const particleWindVector = (direction: number, strength: number) => {
  const vector = vectorAt(direction)
  return { x: vector.x * strength, y: vector.y * strength }
}

export const particleMotionDirection = (
  motion: ParticleMotionDefaults,
  profile: ParticleMotionProfile,
) => {
  const base = vectorAt(profile.baseDirection)
  const wind = particleWindVector(motion.windDirection, motion.windStrength * profile.windInfluence)
  const baseWeight = profile.baseWeight ?? 1
  return normalizedDegrees(Math.atan2(base.y * baseWeight + wind.y, base.x * baseWeight + wind.x) * 180 / Math.PI)
}
