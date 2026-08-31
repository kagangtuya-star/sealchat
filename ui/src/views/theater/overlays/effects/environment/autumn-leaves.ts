import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.autumn-leaves', name: '秋叶', description: '暖红橙色、较干燥轻快的秋叶。', category: 'environment', color: '#c85d2b',
  count: 62, opacity: { min: 0.42, max: 0.94 }, size: { min: 4, max: 9.5 }, baseSpeed: 1.9,
  motion: { baseDirection: 90, windInfluence: 0.9 }, defaults: { intensity: 0.5, speed: 0.8, windDirection: 155, windStrength: 0.42, spread: 0.58 },
  shape: 'square', defaultOpacity: 0.86,
})
