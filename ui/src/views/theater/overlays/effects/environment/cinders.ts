import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.cinders', name: '焦屑', description: '暗红、断续上扬的燃烧焦屑。', category: 'environment', color: '#9d3e20',
  count: 70, opacity: { min: 0.2, max: 0.78 }, size: { min: 1.2, max: 4.5 }, baseSpeed: 1.8,
  motion: { baseDirection: 280, windInfluence: 0.58 }, defaults: { intensity: 0.48, speed: 0.75, windDirection: 315, windStrength: 0.22, spread: 0.55 },
  shape: 'square', defaultOpacity: 0.76, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.7,
})
