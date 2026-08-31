import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'horror.dark-night', name: '恐怖夜晚', description: '无月黑暗与浓雾吞没环境细节。', category: 'horror', tags: ['恐怖', '黑夜', '浓雾'], overlays: [item('lighting.moonless', 0.72), item('weather.fog', 0.6, { intensity: 0.62, speed: 0.22, windStrength: 0.08 })] })
