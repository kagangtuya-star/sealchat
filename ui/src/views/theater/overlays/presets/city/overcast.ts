import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'city.overcast', name: '阴天街道', description: '低饱和阴天与极薄街雾。', category: 'city', tags: ['城市', '阴天', '街道'], overlays: [item('lighting.overcast', 0.36), item('weather.mist', 0.14, { intensity: 0.12, speed: 0.3, windStrength: 0.06 })] })
