import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.holy', name: '圣光粒子', description: '缓慢降下、明亮温暖的金白光点。', category: 'magic', color: '#fff1a8',
  count: 80, opacity: { min: 0.16, max: 0.96 }, size: { min: 1, max: 4.2 }, baseSpeed: 0.9,
  motion: { baseDirection: 90, windInfluence: 0.3 }, defaults: { intensity: 0.52, speed: 0.55, windDirection: 120, windStrength: 0.08, spread: 0.65 },
  defaultOpacity: 0.9, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.55,
})
