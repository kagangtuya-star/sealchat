import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.snow.light', name: '小雪', description: '稀疏、缓慢飘落的雪花。', category: 'weather', color: '#ffffff',
  count: 70, opacity: { min: 0.26, max: 0.72 }, size: { min: 1, max: 3.2 }, baseSpeed: 1.2,
  motion: { baseDirection: 90, windInfluence: 0.65 }, defaults: { intensity: 0.4, speed: 0.7, windDirection: 120, windStrength: 0.14, spread: 0.4 },
  defaultOpacity: 0.82, opacityAnimationSpeed: 0.25,
})
