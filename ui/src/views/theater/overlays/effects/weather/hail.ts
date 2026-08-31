import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'weather.hail', name: '冰雹', description: '快速坠落、颗粒清晰的冰雹。', category: 'weather', color: '#f2fbff',
  count: 120, opacity: { min: 0.48, max: 0.95 }, size: { min: 2, max: 5.5 }, baseSpeed: 12,
  motion: { baseDirection: 90, windInfluence: 0.28 }, defaults: { intensity: 0.6, speed: 1.1, windDirection: 105, windStrength: 0.12, spread: 0.08 },
  defaultOpacity: 0.9,
})
