import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'temple.abandoned', name: '废弃神殿', description: '阴天冷光穿过废弃神殿的厚尘。', category: 'temple', tags: ['神殿', '废弃', '阴天'], overlays: [item('environment.dust', 0.48, { intensity: 0.44, speed: 0.25, spread: 0.9 }), item('lighting.overcast', 0.46)] })
