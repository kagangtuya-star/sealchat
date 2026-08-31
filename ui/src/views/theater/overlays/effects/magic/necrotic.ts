import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.necrotic', name: '死灵粒子', description: '阴冷、缓慢上升的幽绿死灵微粒。', category: 'magic', color: '#63b86a',
  count: 90, opacity: { min: 0.12, max: 0.78 }, size: { min: 0.8, max: 3.6 }, baseSpeed: 1.15,
  motion: { baseDirection: 270, windInfluence: 0.55 }, defaults: { intensity: 0.58, speed: 0.7, windDirection: 300, windStrength: 0.15, spread: 0.8 },
  defaultOpacity: 0.82, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.65,
})
