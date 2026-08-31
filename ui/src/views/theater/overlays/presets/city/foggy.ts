import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'city.foggy', name: '雾中街道', description: '浓雾吞没冷蓝街道轮廓。', category: 'city', tags: ['城市', '浓雾', '冷色'], overlays: [item('weather.fog', 0.62, { intensity: 0.72, speed: 0.32, windDirection: 15, windStrength: 0.12 }), item('lighting.cold', 0.34)] })
