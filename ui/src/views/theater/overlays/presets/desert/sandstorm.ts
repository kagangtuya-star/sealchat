import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'desert.sandstorm', name: '沙尘暴', description: '高密度强风沙尘遮蔽昏暗天光。', category: 'desert', tags: ['沙漠', '沙尘暴', '天气'], overlays: [item('weather.sandstorm', 0.86, { intensity: 0.9, speed: 1.2, windDirection: 185, windStrength: 0.9, spread: 0.55 }), item('lighting.dusk', 0.48)] })
