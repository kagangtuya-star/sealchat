import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.sleet', name: '雨夹雪', description: '短促冰雨与湿雪混合的冷雨幕。', category: 'weather', color: '#d8efff',
  count: 205, opacity: { min: 0.2, max: 0.66 }, size: { min: 2, max: 8 }, baseSpeed: 9,
  motion: { baseDirection: 90, windInfluence: 0.52 }, defaults: { intensity: 0.7, speed: 1, windDirection: 120, windStrength: 0.25, spread: 0.22 },
  shape: 'line', defaultOpacity: 0.82,
})
