import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'dungeon.damp', name: '潮湿地牢', description: '湿冷薄雾贴着地牢石面流动。', category: 'dungeon', tags: ['地牢', '潮湿', '薄雾'], overlays: [item('weather.mist', 0.38, { intensity: 0.42, speed: 0.28, windDirection: 0, windStrength: 0.08 }), item('lighting.cold', 0.46)] })
