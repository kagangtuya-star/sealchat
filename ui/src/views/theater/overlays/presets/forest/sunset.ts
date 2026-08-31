import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'forest.sunset', name: '黄昏森林', description: '黄昏暖光下零散落叶随风而下。', category: 'forest', tags: ['森林', '黄昏', '落叶'], overlays: [item('lighting.dusk', 0.42), item('environment.leaves', 0.62, { intensity: 0.42, speed: 0.7, windDirection: 160, windStrength: 0.3, spread: 0.52 })] })
