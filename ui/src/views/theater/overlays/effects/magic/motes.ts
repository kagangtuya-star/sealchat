import { createParticleSceneOverlayEffect } from '../shared/particle-effect'

export default createParticleSceneOverlayEffect({
  id: 'magic.motes', name: '魔法微粒', description: '柔和、缓慢闪烁的紫蓝魔法光点。', category: 'magic', color: '#9d8cff',
  count: 70, opacity: { min: 0.1, max: 0.88 }, size: { min: 0.8, max: 3.2 }, baseSpeed: 0.7,
  motion: { baseDirection: 275, baseWeight: 0.18, windInfluence: 0.45 }, defaults: { intensity: 0.5, speed: 0.55, windDirection: 20, windStrength: 0.1, spread: 0.9 },
  defaultOpacity: 0.84, defaultBlendMode: 'screen', opacityAnimationSpeed: 0.75,
})
