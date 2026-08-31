import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'cave.dry', name: '干燥洞穴', description: '无光岩洞中漂浮着干燥石尘。', category: 'cave', tags: ['洞穴', '干燥', '黑暗'], overlays: [item('environment.dust', 0.4, { intensity: 0.38, speed: 0.28, spread: 0.85 }), item('lighting.moonless', 0.62)] })
