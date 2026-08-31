import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'cave.crystal', name: '水晶洞穴', description: '魔法微光在蓝紫水晶间漂移。', category: 'cave', tags: ['洞穴', '水晶', '魔法'], overlays: [item('magic.motes', 0.78, { intensity: 0.5, speed: 0.5, spread: 0.9 }), item('lighting.twilight', 0.44)] })
