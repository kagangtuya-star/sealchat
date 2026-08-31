import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'eastern.lantern-night', name: '灯火夜市', description: '暖烛色灯火照出夜市中的少量浮尘。', category: 'eastern', tags: ['东方', '夜市', '灯火'], overlays: [item('lighting.candlelight', 0.44), item('environment.dust', 0.22, { intensity: 0.16, speed: 0.32, windStrength: 0.08 })] })
