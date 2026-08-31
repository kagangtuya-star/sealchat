import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'ocean.underwater', name: '水下', description: '青蓝水下光线中气泡缓慢上浮。', category: 'ocean', tags: ['海洋', '水下', '气泡'], overlays: [item('environment.bubbles', 0.78, { intensity: 0.55, speed: 0.65, windStrength: 0.08, spread: 0.38 }), item('lighting.underwater', 0.52)] })
