import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.leaves', name: '落叶', description: '受重力与风共同影响的绿色落叶。', category: 'environment', color: '#769447',
  count: 68, opacity: { min: 0.38, max: 0.9 }, size: { min: 3.5, max: 8.5 }, baseSpeed: 1.7,
  motion: { baseDirection: 90, windInfluence: 0.8 }, defaults: { intensity: 0.48, speed: 0.75, windDirection: 160, windStrength: 0.35, spread: 0.5 },
  shape: 'square', defaultOpacity: 0.82,
})
