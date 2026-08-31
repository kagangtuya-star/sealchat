import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.snow', name: '飘雪', description: '大小不一、随风飘落的雪花。', category: 'weather', color: '#ffffff',
  count: 125, opacity: { min: 0.32, max: 0.88 }, size: { min: 1.3, max: 4.8 }, sizeControl: { min: 0.4, max: 2.5 }, baseSpeed: 1.8,
  motion: { baseDirection: 90, windInfluence: 0.75 }, defaults: { intensity: 0.62, speed: 0.9, windDirection: 135, windStrength: 0.25, spread: 0.48 },
  defaultOpacity: 0.9, opacityAnimationSpeed: 0.35,
})
