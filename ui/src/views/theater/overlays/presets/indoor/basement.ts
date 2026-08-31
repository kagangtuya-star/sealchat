import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'indoor.basement', name: '地下室', description: '几乎无光的地下室与缓慢积尘。', category: 'indoor', tags: ['室内', '地下室', '黑暗'], overlays: [item('lighting.moonless', 0.68), item('environment.dust', 0.28, { intensity: 0.24, speed: 0.25, spread: 0.85 })] })
