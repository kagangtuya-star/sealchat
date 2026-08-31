import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.petals', name: '花瓣', description: '轻柔旋落、飘散明显的粉色花瓣。', category: 'environment', color: '#f2a7bd',
  count: 72, opacity: { min: 0.35, max: 0.9 }, size: { min: 2.5, max: 6.5 }, baseSpeed: 1.2,
  motion: { baseDirection: 90, windInfluence: 0.9 }, defaults: { intensity: 0.48, speed: 0.65, windDirection: 150, windStrength: 0.25, spread: 0.68 },
  shape: 'square', defaultOpacity: 0.86, opacityAnimationSpeed: 0.25,
})
