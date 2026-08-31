import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'temple.holy', name: '圣洁神殿', description: '金白圣光粒子落入温暖神殿。', category: 'temple', tags: ['神殿', '圣光', '神圣'], overlays: [item('magic.holy', 0.88, { intensity: 0.58, speed: 0.5, windStrength: 0.05, spread: 0.6 }), item('lighting.warm-day', 0.32)] })
