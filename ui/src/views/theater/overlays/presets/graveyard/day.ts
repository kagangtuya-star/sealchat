import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'graveyard.day', name: '阴郁墓地', description: '阴天薄雾笼罩寂静墓碑。', category: 'graveyard', tags: ['墓地', '阴天', '薄雾'], overlays: [item('weather.mist', 0.34, { intensity: 0.35, speed: 0.25 }), item('lighting.overcast', 0.44)] })
