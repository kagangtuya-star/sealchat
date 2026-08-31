import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.rain.light', name: '小雨', description: '稀疏、细短的轻雨。', category: 'weather', color: '#b9dcff',
  count: 105, opacity: { min: 0.16, max: 0.46 }, size: { min: 7, max: 14 }, baseSpeed: 11,
  motion: { baseDirection: 90, windInfluence: 0.25 }, defaults: { intensity: 0.45, speed: 1, windDirection: 100, windStrength: 0.08, spread: 0.04 },
  shape: 'line', defaultOpacity: 0.72, controls: { windStrength: { max: 0.8 } },
})
