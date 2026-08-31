import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'desert.wind', name: '风沙', description: '高风力浮尘横扫明亮沙地。', category: 'desert', tags: ['沙漠', '风沙', '大风'], overlays: [item('environment.dust', 0.62, { intensity: 0.58, speed: 1.15, windDirection: 185, windStrength: 0.75, spread: 0.48 }), item('lighting.warm-day', 0.32)] })
