import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.poison', name: '毒性微粒', description: '黏滞、低速扩散的酸绿毒性光点。', category: 'magic', color: '#85d32f',
  count: 100, opacity: { min: 0.12, max: 0.78 }, size: { min: 1.2, max: 4.8 }, baseSpeed: 0.75,
  motion: { baseDirection: 270, baseWeight: 0.16, windInfluence: 0.65 }, defaults: { intensity: 0.6, speed: 0.55, windDirection: 10, windStrength: 0.18, spread: 0.88 },
  defaultOpacity: 0.82, defaultBlendMode: 'screen', sizeAnimationSpeed: 0.3,
})
