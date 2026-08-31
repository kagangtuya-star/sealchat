import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.nature', name: '自然灵光', description: '柔和、蓬勃、向上生长感的翠绿灵光。', category: 'magic', color: '#77d96a',
  count: 78, opacity: { min: 0.1, max: 0.86 }, size: { min: 1, max: 4.2 }, baseSpeed: 0.8,
  motion: { baseDirection: 270, windInfluence: 0.48 }, defaults: { intensity: 0.48, speed: 0.55, windDirection: 20, windStrength: 0.12, spread: 0.82 },
  defaultOpacity: 0.82, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.5, sizeAnimationSpeed: 0.3,
})
