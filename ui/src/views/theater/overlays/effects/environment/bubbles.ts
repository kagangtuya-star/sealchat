import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.bubbles', name: '水下气泡', description: '大小不同、缓慢上浮的透明气泡。', category: 'environment', color: '#bdefff',
  count: 70, opacity: { min: 0.18, max: 0.62 }, size: { min: 1.5, max: 6.5 }, baseSpeed: 1.1,
  motion: { baseDirection: 270, windInfluence: 0.25 }, defaults: { intensity: 0.48, speed: 0.65, windDirection: 0, windStrength: 0.08, spread: 0.38 },
  defaultOpacity: 0.76, defaultBlendMode: 'screen', sizeAnimationSpeed: 0.2,
})
