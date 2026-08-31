import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.drizzle', name: '毛毛雨', description: '细小、缓慢且分散的雨滴。', category: 'weather', color: '#c9e4f5',
  count: 145, opacity: { min: 0.08, max: 0.3 }, size: { min: 0.5, max: 1.4 }, baseSpeed: 3.2,
  motion: { baseDirection: 90, windInfluence: 0.45 }, defaults: { intensity: 0.38, speed: 0.7, windDirection: 100, windStrength: 0.1, spread: 0.45 },
  defaultOpacity: 0.58,
})
