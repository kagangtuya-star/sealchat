import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.arcane', name: '奥术粒子', description: '高速、锐利、规律脉动的蓝紫奥术粒子。', category: 'magic', color: '#725cff',
  count: 110, opacity: { min: 0.2, max: 1 }, size: { min: 0.7, max: 2.8 }, baseSpeed: 2.1,
  motion: { baseDirection: 315, baseWeight: 0.25, windInfluence: 0.5 }, defaults: { intensity: 0.62, speed: 1, windDirection: 45, windStrength: 0.2, spread: 0.78 },
  shape: 'star', defaultOpacity: 0.9, defaultBlendMode: 'screen', opacityAnimationSpeed: 1.15, sizeAnimationSpeed: 0.45,
})
