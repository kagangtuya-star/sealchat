import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.soot', name: '黑灰', description: '稀疏落下的黑色燃烧残留物。', category: 'environment', color: '#34383a',
  count: 95, opacity: { min: 0.18, max: 0.58 }, size: { min: 0.7, max: 3.2 }, baseSpeed: 1.25,
  motion: { baseDirection: 90, windInfluence: 0.7 }, defaults: { intensity: 0.46, speed: 0.65, windDirection: 155, windStrength: 0.28, spread: 0.62 },
  defaultOpacity: 0.62,
})
