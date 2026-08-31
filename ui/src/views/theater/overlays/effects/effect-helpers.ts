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

interface ParticleConfigInput {
  count: number
  color: string
  opacity: { min: number, max: number }
  size: { min: number, max: number }
  speed: number
  direction: number
  spread: number
  shape?: 'circle' | 'line' | 'square' | 'star'
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
      value: Math.max(0, Math.round(input.count)),
      density: { enable: true, width: 1280, height: 720 },
    },
    shape: { type: input.shape || 'circle' },
    paint: {
      color: { value: input.color },
      fill: { enable: input.shape !== 'line' },
      ...(input.shape === 'line' ? { stroke: { width: 1 } } : {}),
    },
    ...(input.shape === 'line' ? { rotate: { path: true } } : {}),
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
      angle: { offset: 0, value: Math.max(1, input.spread * 100) },
      random: false,
      straight: input.spread <= 0.02,
      outModes: { default: 'out' },
    },
  },
})
