import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'modern.blackout', name: '停电', description: '无月黑暗伴随周期性断电脉冲。', category: 'modern', tags: ['现代', '停电', '黑暗'], overlays: [item('lighting.moonless', 0.78), item('special.blackout', 0.82, { strength: 0.88, frequency: 0.1 })] })
