import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.sparks', name: '火星', description: '快速飞散、短促明亮的火星。', category: 'environment', color: '#ffd25a',
  count: 58, opacity: { min: 0.35, max: 1 }, size: { min: 0.5, max: 2.2 }, baseSpeed: 5.2,
  motion: { baseDirection: 285, windInfluence: 0.5 }, defaults: { intensity: 0.48, speed: 1.1, windDirection: 320, windStrength: 0.2, spread: 0.65 },
  shape: 'square', defaultOpacity: 0.92, defaultBlendMode: 'screen', opacityAnimationSpeed: 1.6,
})
