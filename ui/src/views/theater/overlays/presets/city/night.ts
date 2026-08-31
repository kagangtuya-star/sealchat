import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'city.night', name: '深夜街道', description: '深蓝夜色与近地极薄雾气。', category: 'city', tags: ['城市', '深夜', '街道'], overlays: [item('lighting.night', 0.58), item('weather.mist', 0.12, { intensity: 0.1, speed: 0.25, windStrength: 0.04 })] })
