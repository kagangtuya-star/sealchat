import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.blizzard', name: '暴雪', description: '高速、密集、横向偏移明显的暴雪。', category: 'weather', color: '#eef8ff',
  count: 285, opacity: { min: 0.3, max: 0.9 }, size: { min: 1.2, max: 4.2 }, baseSpeed: 4.6,
  motion: { baseDirection: 90, windInfluence: 0.85 }, defaults: { intensity: 1, speed: 1.4, windDirection: 165, windStrength: 0.7, spread: 0.4 },
  defaultOpacity: 0.94, fpsLimit: 50,
})
