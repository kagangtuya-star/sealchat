import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.steam', name: '蒸汽', description: '快速上升、扩散明显的白色蒸汽。', category: 'environment', color: '#e6eeee',
  count: 72, opacity: { min: 0.025, max: 0.13 }, size: { min: 24, max: 68 }, baseSpeed: 1.1,
  motion: { baseDirection: 270, windInfluence: 0.65 }, defaults: { intensity: 0.5, speed: 0.8, windDirection: 300, windStrength: 0.16, spread: 0.78 },
  defaultOpacity: 0.6, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.18,
})
