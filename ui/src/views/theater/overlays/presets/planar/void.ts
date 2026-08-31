import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'planar.void', name: '虚空', description: '近黑位面中仅有反常虚空碎屑。', category: 'planar', tags: ['位面', '虚空', '黑暗'], overlays: [item('magic.void', 0.92, { intensity: 0.7, speed: 0.9, windStrength: 0.4, spread: 1 }), item('lighting.moonless', 0.76)] })
