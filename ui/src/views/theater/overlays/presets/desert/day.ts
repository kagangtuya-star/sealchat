import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'desert.day', name: '白昼沙漠', description: '暖昼强光中少量沙尘漂移。', category: 'desert', tags: ['沙漠', '白昼', '炎热'], overlays: [item('environment.dust', 0.38, { intensity: 0.3, speed: 0.55, windDirection: 180, windStrength: 0.25, spread: 0.7 }), item('lighting.warm-day', 0.34)] })
