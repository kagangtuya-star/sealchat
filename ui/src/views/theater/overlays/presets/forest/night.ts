import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'forest.night', name: '夜间森林', description: '银蓝月光下萤火虫缓慢闪烁。', category: 'forest', tags: ['森林', '夜晚', '萤火虫'], overlays: [item('lighting.moonlight', 0.38), item('environment.fireflies', 0.86, { intensity: 0.46, speed: 0.4, spread: 0.96 })] })
