import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.rain.medium', name: '中雨', description: '稳定、清晰的中等雨幕。', category: 'weather', color: '#acd5ff',
  count: 185, opacity: { min: 0.22, max: 0.62 }, size: { min: 10, max: 19 }, baseSpeed: 16,
  motion: { baseDirection: 90, windInfluence: 0.32 }, defaults: { intensity: 0.65, speed: 1.05, windDirection: 105, windStrength: 0.15, spread: 0.06 },
  shape: 'line', defaultOpacity: 0.82, controls: { windStrength: { max: 0.85 } },
})
