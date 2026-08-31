import type { ISourceOptions } from '@tsparticles/engine'

import type { StageSceneOverlayParams } from '../../shared/stage-types'

export const numberParam = (
  params: StageSceneOverlayParams,
  key: string,
  fallback: number,
  minimum: number,
  maximum: number,
) => {
  const value = params[key]
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
}

export const colorParam = (params: StageSceneOverlayParams, key: string, fallback: string) => {
  const value = params[key]
  return typeof value === 'string' && /^#[0-9a-f]{6}$/i.test(value) ? value : fallback
}

type ParticleDirection = 'top' | 'top-right' | 'right' | 'bottom-right' | 'bottom' | 'bottom-left' | 'left' | 'top-left' | 'none'

interface ParticleConfigInput {
  count: number
  color: string
  opacity: { min: number, max: number }
  size: { min: number, max: number }
  speed: number
  direction: ParticleDirection
  shape?: 'circle' | 'line' | 'square' | 'star'
  straight?: boolean
  random?: boolean
  opacityAnimationSpeed?: number
  sizeAnimationSpeed?: number
  fpsLimit?: number
}

export const particleConfig = (input: ParticleConfigInput): ISourceOptions => ({
  fullScreen: { enable: false },
  detectRetina: true,
  fpsLimit: input.fpsLimit || 45,
  background: { opacity: 0 },
  interactivity: { events: {} },
  particles: {
    number: {
      value: Math.round(input.count),
      density: { enable: true, width: 1280, height: 720 },
    },
    color: { value: input.color },
    shape: { type: input.shape || 'circle' },
    opacity: {
      value: input.opacity,
      animation: input.opacityAnimationSpeed
        ? { enable: true, speed: input.opacityAnimationSpeed, sync: false }
        : { enable: false },
    },
    size: {
      value: input.size,
      animation: input.sizeAnimationSpeed
        ? { enable: true, speed: input.sizeAnimationSpeed, sync: false }
        : { enable: false },
    },
    move: {
      enable: true,
      speed: input.speed,
      direction: input.direction,
      random: input.random === true,
      straight: input.straight === true,
      outModes: { default: 'out' },
    },
  },
})

export const verticalDirection = (wind: number, downward = true): ParticleDirection => {
  if (downward) return wind < -0.15 ? 'bottom-left' : wind > 0.15 ? 'bottom-right' : 'bottom'
  return wind < -0.15 ? 'top-left' : wind > 0.15 ? 'top-right' : 'top'
}

export const horizontalDirection = (wind: number): ParticleDirection => wind < 0 ? 'left' : 'right'
