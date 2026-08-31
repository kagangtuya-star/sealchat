import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'graveyard.night', name: '午夜墓地', description: '浓雾在银蓝月光下掩没墓碑。', category: 'graveyard', tags: ['墓地', '午夜', '浓雾'], overlays: [item('weather.fog', 0.58, { intensity: 0.6, speed: 0.25, windStrength: 0.1 }), item('lighting.moonlight', 0.42)] })
