import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'desert.sunset', name: '沙漠黄昏', description: '夕阳将缓慢风沙染成金红色。', category: 'desert', tags: ['沙漠', '夕阳', '风沙'], overlays: [item('environment.dust', 0.46, { intensity: 0.38, speed: 0.65, windDirection: 180, windStrength: 0.32 }), item('lighting.sunset', 0.5)] })
