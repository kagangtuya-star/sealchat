import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.ashfall', name: '火山灰', description: '缓慢坠落、明暗不一的火山灰。', category: 'weather', color: '#77726f',
  count: 165, opacity: { min: 0.18, max: 0.65 }, size: { min: 0.8, max: 4.2 }, baseSpeed: 1.8,
  motion: { baseDirection: 90, windInfluence: 0.7 }, defaults: { intensity: 0.65, speed: 0.8, windDirection: 145, windStrength: 0.35, spread: 0.6 },
  defaultOpacity: 0.72, opacityAnimationSpeed: 0.2,
})
