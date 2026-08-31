import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'indoor.library', name: '古老图书馆', description: '烛火穿过书库中的陈年浮尘。', category: 'indoor', tags: ['室内', '图书馆', '古老'], overlays: [item('environment.dust', 0.38, { intensity: 0.32, speed: 0.25, spread: 0.88 }), item('lighting.candlelight', 0.32)] })
