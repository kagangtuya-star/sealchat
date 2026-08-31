import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'modern.rainy-night', name: '现代雨夜', description: '中雨穿过深蓝现代夜景。', category: 'modern', tags: ['现代', '雨夜', '城市'], overlays: [item('weather.rain.medium', 0.86, { intensity: 0.68, speed: 1.1, windDirection: 125, windStrength: 0.22, spread: 0.06 }), item('lighting.night', 0.58)] })
