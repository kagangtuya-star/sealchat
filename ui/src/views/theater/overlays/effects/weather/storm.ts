import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.storm', name: '风暴雨幕', description: '受强风推动的高密度雨幕。', category: 'weather', color: '#8abce8',
  count: 340, opacity: { min: 0.28, max: 0.72 }, size: { min: 16, max: 30 }, baseSpeed: 25,
  motion: { baseDirection: 90, windInfluence: 0.48 }, defaults: { intensity: 0.95, speed: 1.35, windDirection: 150, windStrength: 0.6, spread: 0.12 },
  shape: 'line', defaultOpacity: 0.92, fpsLimit: 50,
})
