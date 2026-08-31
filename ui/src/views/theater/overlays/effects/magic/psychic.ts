import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.psychic', name: '精神波动微粒', description: '快速变亮、无规律飘移的粉紫精神微粒。', category: 'magic', color: '#e45cff',
  count: 88, opacity: { min: 0.05, max: 0.95 }, size: { min: 0.8, max: 5.5 }, baseSpeed: 1.6,
  motion: { baseDirection: 0, baseWeight: 0.08, windInfluence: 0.8 }, defaults: { intensity: 0.55, speed: 0.9, windDirection: 210, windStrength: 0.3, spread: 1 },
  defaultOpacity: 0.88, defaultBlendMode: 'screen', opacityAnimationSpeed: 1.5, sizeAnimationSpeed: 0.8,
})
