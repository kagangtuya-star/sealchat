import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.mist', name: '薄雾', description: '稀薄、缓慢横移的柔和雾气。', category: 'weather', color: '#dce6e8',
  count: 58, opacity: { min: 0.015, max: 0.07 }, size: { min: 38, max: 95 }, baseSpeed: 0.2,
  motion: { baseDirection: 0, baseWeight: 0.2, windInfluence: 1.1 }, defaults: { intensity: 0.35, speed: 0.45, windDirection: 0, windStrength: 0.08, spread: 0.62 },
  defaultOpacity: 0.42, controls: { spread: false },
})
