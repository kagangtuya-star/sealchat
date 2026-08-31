import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'ocean.coast-day', name: '晴朗海岸', description: '暖日海岸中少量金色光尘闪动。', category: 'ocean', tags: ['海洋', '海岸', '晴天'], overlays: [item('lighting.warm-day', 0.28), item('environment.pollen', 0.18, { intensity: 0.1, speed: 0.45, windDirection: 180, windStrength: 0.2 })] })
