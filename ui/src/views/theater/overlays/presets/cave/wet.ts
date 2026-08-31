import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'cave.wet', name: '湿润洞穴', description: '冷光与凝滞薄雾塑造潮湿岩洞。', category: 'cave', tags: ['洞穴', '潮湿', '冷色'], overlays: [item('weather.mist', 0.4, { intensity: 0.42, speed: 0.25, windStrength: 0.06 }), item('lighting.cold', 0.48)] })
