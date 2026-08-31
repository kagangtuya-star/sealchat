import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'snow.normal', name: '雪原', description: '持续飘雪与开阔冷蓝雪原。', category: 'snow', tags: ['雪地', '飘雪', '原野'], overlays: [item('weather.snow', 0.9, { intensity: 0.65, speed: 0.9, windDirection: 145, windStrength: 0.28, spread: 0.48 }), item('lighting.cold', 0.42)] })
