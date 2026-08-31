import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'city.drizzle', name: '毛毛雨街道', description: '细碎雨点覆盖阴冷街景。', category: 'city', tags: ['城市', '细雨', '天气'], overlays: [item('weather.drizzle', 0.62, { intensity: 0.38, speed: 0.65, windDirection: 115, windStrength: 0.12, spread: 0.5 }), item('lighting.overcast', 0.34)] })
