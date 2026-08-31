import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'city.sunset', name: '黄昏街道', description: '橙红夕阳照亮空气中的浮尘。', category: 'city', tags: ['城市', '夕阳', '黄昏'], overlays: [item('lighting.sunset', 0.4), item('environment.dust', 0.3, { intensity: 0.28, speed: 0.4, windDirection: 20, windStrength: 0.08 })] })
