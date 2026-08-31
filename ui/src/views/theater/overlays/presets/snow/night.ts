import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'snow.night', name: '雪夜', description: '银蓝月光映亮安静飘雪。', category: 'snow', tags: ['雪地', '夜晚', '月光'], overlays: [item('weather.snow', 0.82, { intensity: 0.55, speed: 0.75, windDirection: 135, windStrength: 0.2, spread: 0.52 }), item('lighting.moonlight', 0.44)] })
