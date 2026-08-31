import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.dust', name: '浮尘', description: '光线中缓慢漂移的细尘。', category: 'environment', color: '#d8c49a',
  count: 72, opacity: { min: 0.06, max: 0.32 }, size: { min: 0.6, max: 2.5 }, sizeControl: { min: 0.5, max: 2 }, baseSpeed: 0.55,
  motion: { baseDirection: 0, baseWeight: 0.2, windInfluence: 0.7 }, defaults: { intensity: 0.35, speed: 0.45, windDirection: 25, windStrength: 0.12, spread: 0.75 },
  defaultOpacity: 0.58, opacityAnimationSpeed: 0.3,
})
