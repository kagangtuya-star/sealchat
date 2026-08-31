import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'forest.petals', name: '花林', description: '暖日花林中粉色花瓣轻柔旋落。', category: 'forest', tags: ['森林', '花瓣', '春日'], overlays: [item('environment.petals', 0.82, { intensity: 0.58, speed: 0.65, windDirection: 150, windStrength: 0.25, spread: 0.7 }), item('lighting.warm-day', 0.24)] })
