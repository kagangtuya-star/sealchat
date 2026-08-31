import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'desert.night', name: '沙漠夜晚', description: '清冷月光下仅有极少沙尘。', category: 'desert', tags: ['沙漠', '夜晚', '月光'], overlays: [item('environment.dust', 0.14, { intensity: 0.1, speed: 0.35, windStrength: 0.08 }), item('lighting.moonlight', 0.42)] })
