import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'ocean.deep-sea', name: '深海', description: '近黑深海中仅有少量气泡与微弱青蓝。', category: 'ocean', tags: ['海洋', '深海', '黑暗'], overlays: [item('environment.bubbles', 0.36, { intensity: 0.18, speed: 0.5, spread: 0.3 }), item('lighting.moonless', 0.62), item('lighting.underwater', 0.2)] })
