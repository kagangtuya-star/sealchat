import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.feathers', name: '羽毛', description: '大而轻、缓慢盘旋下落的羽毛。', category: 'environment', color: '#f4f1e8',
  count: 34, opacity: { min: 0.35, max: 0.82 }, size: { min: 5, max: 12 }, baseSpeed: 0.8,
  motion: { baseDirection: 90, windInfluence: 0.95 }, defaults: { intensity: 0.34, speed: 0.5, windDirection: 145, windStrength: 0.28, spread: 0.82 },
  shape: 'line', defaultOpacity: 0.78,
})
