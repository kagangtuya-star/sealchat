import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'indoor.banquet-hall', name: '宴会厅', description: '明亮烛火与极少金色圣光点。', category: 'indoor', tags: ['室内', '宴会', '华丽'], overlays: [item('lighting.candlelight', 0.38), item('magic.holy', 0.22, { intensity: 0.12, speed: 0.35, windStrength: 0.03, spread: 0.55 })] })
