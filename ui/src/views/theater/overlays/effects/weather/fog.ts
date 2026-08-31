import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.fog', name: '浓雾', description: '厚重、低对比度的流动雾层。', category: 'weather', color: '#cbd5d8',
  count: 105, opacity: { min: 0.025, max: 0.11 }, size: { min: 62, max: 155 }, baseSpeed: 0.16,
  motion: { baseDirection: 0, baseWeight: 0.18, windInfluence: 1.2 }, defaults: { intensity: 0.72, speed: 0.35, windDirection: 15, windStrength: 0.16, spread: 0.72 },
  defaultOpacity: 0.58, controls: { spread: false },
})
