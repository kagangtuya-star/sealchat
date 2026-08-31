import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'eastern.snow', name: '古寺飘雪', description: '冷蓝古寺前雪花安静飘落。', category: 'eastern', tags: ['东方', '古寺', '飘雪'], overlays: [item('weather.snow', 0.86, { intensity: 0.58, speed: 0.72, windDirection: 145, windStrength: 0.2, spread: 0.5 }), item('lighting.cold', 0.4)] })
