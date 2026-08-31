import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.spores', name: '孢子', description: '潮湿环境中缓慢漂浮的微绿孢子。', category: 'environment', color: '#9fbf72',
  count: 78, opacity: { min: 0.1, max: 0.62 }, size: { min: 1, max: 3.4 }, baseSpeed: 0.5,
  motion: { baseDirection: 280, baseWeight: 0.15, windInfluence: 0.7 }, defaults: { intensity: 0.45, speed: 0.42, windDirection: 10, windStrength: 0.12, spread: 0.88 },
  defaultOpacity: 0.72, opacityAnimationSpeed: 0.4,
})
