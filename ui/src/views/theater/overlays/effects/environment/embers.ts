import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.embers', name: '余烬', description: '向上漂浮、忽明忽暗的暖红余烬。', category: 'environment', color: '#ff6b1a',
  count: 82, opacity: { min: 0.22, max: 0.96 }, size: { min: 0.9, max: 3.8 }, sizeControl: { min: 0.5, max: 2 }, baseSpeed: 2.7,
  motion: { baseDirection: 270, windInfluence: 0.45 }, defaults: { intensity: 0.55, speed: 0.85, windDirection: 300, windStrength: 0.15, spread: 0.42 },
  defaultOpacity: 0.88, defaultBlendMode: 'screen', opacityAnimationSpeed: 1.1, sizeAnimationSpeed: 0.5,
})
