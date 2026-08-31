import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.smoke', name: '烟尘', description: '低透明度、大颗粒、明显受风的上升烟尘。', category: 'environment', color: '#6f7478',
  count: 88, opacity: { min: 0.025, max: 0.14 }, size: { min: 28, max: 78 }, baseSpeed: 0.75,
  motion: { baseDirection: 270, windInfluence: 0.8 }, defaults: { intensity: 0.5, speed: 0.6, windDirection: 280, windStrength: 0.2, spread: 0.72 },
  defaultOpacity: 0.64,
})
