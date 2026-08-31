import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'forest.autumn', name: '秋日森林', description: '橙红夕照中大量秋叶斜落。', category: 'forest', tags: ['森林', '秋天', '落叶'], overlays: [item('environment.autumn-leaves', 0.82, { intensity: 0.62, speed: 0.85, windDirection: 155, windStrength: 0.42, spread: 0.58 }), item('lighting.sunset', 0.4)] })
