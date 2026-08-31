import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.sandstorm', name: '沙尘暴', description: '主要由强风推动的高密度暖色沙尘。', category: 'weather', color: '#c49a58',
  count: 320, opacity: { min: 0.12, max: 0.58 }, size: { min: 0.7, max: 3.2 }, sizeControl: { min: 0.5, max: 2 }, baseSpeed: 8.5,
  motion: { baseDirection: 0, baseWeight: 0.15, windInfluence: 1.25 }, defaults: { intensity: 0.9, speed: 1.1, windDirection: 180, windStrength: 0.8, spread: 0.55 },
  defaultOpacity: 0.78, fpsLimit: 50,
})
