import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'snow.light', name: '小雪原野', description: '稀疏小雪落在清冷原野。', category: 'snow', tags: ['雪地', '小雪', '冷色'], overlays: [item('weather.snow.light', 0.82, { intensity: 0.4, speed: 0.65, windDirection: 135, windStrength: 0.18, spread: 0.42 }), item('lighting.cold', 0.38)] })
