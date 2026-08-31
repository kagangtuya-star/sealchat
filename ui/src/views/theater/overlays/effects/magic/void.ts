import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.void', name: '虚空微粒', description: '暗紫、稀疏、运动反常的虚空碎屑。', category: 'magic', color: '#4b2678',
  count: 75, opacity: { min: 0.08, max: 0.7 }, size: { min: 1, max: 5.2 }, baseSpeed: 1.35,
  motion: { baseDirection: 225, baseWeight: 0.25, windInfluence: 0.7 }, defaults: { intensity: 0.55, speed: 0.75, windDirection: 30, windStrength: 0.28, spread: 0.96 },
  shape: 'square', defaultOpacity: 0.86, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.9, sizeAnimationSpeed: 0.6,
})
