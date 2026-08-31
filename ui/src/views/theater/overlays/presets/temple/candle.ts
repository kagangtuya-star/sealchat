import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'temple.candle', name: '烛火神殿', description: '柔和烛光照出神殿中的古老浮尘。', category: 'temple', tags: ['神殿', '烛光', '室内'], overlays: [item('lighting.candlelight', 0.4), item('environment.dust', 0.32, { intensity: 0.28, speed: 0.28, spread: 0.86 })] })
