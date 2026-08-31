import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.rain.heavy', name: '暴雨', description: '密集、高速的长雨线。', category: 'weather', color: '#9fcfff',
  count: 300, opacity: { min: 0.3, max: 0.76 }, size: { min: 14, max: 27 }, baseSpeed: 23,
  motion: { baseDirection: 90, windInfluence: 0.4 }, defaults: { intensity: 0.85, speed: 1.2, windDirection: 110, windStrength: 0.25, spread: 0.08 },
  shape: 'line', defaultOpacity: 0.9, controls: { windStrength: { max: 0.9 } }, fpsLimit: 50,
})
