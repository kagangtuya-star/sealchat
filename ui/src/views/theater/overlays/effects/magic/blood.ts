import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.blood', name: '血色微粒', description: '沉重、缓慢下坠的深红魔力微粒。', category: 'magic', color: '#b51f32',
  count: 85, opacity: { min: 0.22, max: 0.9 }, size: { min: 1.2, max: 4.3 }, baseSpeed: 1.15,
  motion: { baseDirection: 90, windInfluence: 0.38 }, defaults: { intensity: 0.55, speed: 0.7, windDirection: 130, windStrength: 0.12, spread: 0.5 },
  defaultOpacity: 0.86, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.6,
})
