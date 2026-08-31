import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'volcanic.ash', name: '火山灰原野', description: '持续落灰遮蔽昏暗火山天光。', category: 'volcanic', tags: ['火山', '火山灰', '荒野'], overlays: [item('weather.ashfall', 0.78, { intensity: 0.68, speed: 0.75, windDirection: 150, windStrength: 0.35, spread: 0.62 }), item('lighting.dusk', 0.46)] })
