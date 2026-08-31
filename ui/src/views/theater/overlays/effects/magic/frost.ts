import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.frost', name: '寒霜魔力', description: '锐利、晶亮、缓慢扩散的冰蓝碎光。', category: 'magic', color: '#a5e8ff',
  count: 92, opacity: { min: 0.18, max: 0.92 }, size: { min: 0.8, max: 3.5 }, baseSpeed: 1.15,
  motion: { baseDirection: 315, baseWeight: 0.18, windInfluence: 0.65 }, defaults: { intensity: 0.56, speed: 0.65, windDirection: 160, windStrength: 0.25, spread: 0.72 },
  shape: 'star', defaultOpacity: 0.88, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.5,
})
