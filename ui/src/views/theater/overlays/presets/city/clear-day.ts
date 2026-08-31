import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'city.clear-day', name: '晴朗白昼', description: '暖日光与极轻浮尘构成的清朗街景。', category: 'city', tags: ['城市', '晴天', '白昼'], overlays: [item('lighting.warm-day', 0.2), item('environment.dust', 0.16, { intensity: 0.1, speed: 0.35 })] })
