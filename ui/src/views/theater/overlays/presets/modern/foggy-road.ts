import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'modern.foggy-road', name: '雾中公路', description: '高浓度冷雾压低公路能见度。', category: 'modern', tags: ['现代', '公路', '浓雾'], overlays: [item('weather.fog', 0.68, { intensity: 0.76, speed: 0.28, windDirection: 0, windStrength: 0.12 }), item('lighting.cold', 0.44)] })
