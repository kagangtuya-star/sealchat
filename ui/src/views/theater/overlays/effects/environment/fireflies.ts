import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'environment.fireflies', name: '萤火虫', description: '缓慢游动、柔和闪烁的暖色光点。', category: 'environment', color: '#dfff70',
  count: 38, opacity: { min: 0.06, max: 0.98 }, size: { min: 1.3, max: 3.3 }, sizeControl: { min: 0.5, max: 2.5 }, baseSpeed: 0.55,
  motion: { baseDirection: 270, baseWeight: 0.08, windInfluence: 0.25 }, defaults: { intensity: 0.42, speed: 0.45, windDirection: 20, windStrength: 0.08, spread: 0.95 },
  controls: { windDirection: false, windStrength: false }, defaultOpacity: 0.92, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.8, sizeAnimationSpeed: 0.35,
})
