import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'dungeon.arcane', name: '魔法地下城', description: '活跃奥术粒子照亮蓝紫暮色。', category: 'dungeon', tags: ['地牢', '奥术', '魔法'], overlays: [item('magic.arcane', 0.82, { intensity: 0.58, speed: 0.9, windStrength: 0.18, spread: 0.75 }), item('lighting.twilight', 0.46)] })
