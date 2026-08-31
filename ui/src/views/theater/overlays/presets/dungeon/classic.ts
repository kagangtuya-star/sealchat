import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'dungeon.classic', name: '普通地牢', description: '冷蓝石墙与静滞积尘。', category: 'dungeon', tags: ['地牢', '冷色', '经典'], overlays: [item('lighting.cold', 0.42), item('environment.dust', 0.3, { intensity: 0.26, speed: 0.25, spread: 0.82 })] })
