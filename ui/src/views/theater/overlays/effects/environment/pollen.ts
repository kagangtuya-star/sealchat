import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.pollen', name: '花粉 / 光尘', description: '稀疏、发光感明显的金色微尘。', category: 'environment', color: '#f8e58c',
  count: 60, opacity: { min: 0.08, max: 0.75 }, size: { min: 0.7, max: 2.4 }, baseSpeed: 0.45,
  motion: { baseDirection: 0, baseWeight: 0.12, windInfluence: 0.6 }, defaults: { intensity: 0.36, speed: 0.4, windDirection: 20, windStrength: 0.1, spread: 0.9 },
  defaultOpacity: 0.76, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.55,
})
