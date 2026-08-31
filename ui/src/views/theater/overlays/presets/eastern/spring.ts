import { defineSceneOverlayPreset as preset, presetItem as item } from '../preset-helpers'
export default preset({ id: 'eastern.spring', name: '春日花林', description: '暖日下粉色花瓣掠过东方花林。', category: 'eastern', tags: ['东方', '春日', '花瓣'], overlays: [item('environment.petals', 0.82, { intensity: 0.6, speed: 0.62, windDirection: 150, windStrength: 0.25, spread: 0.7 }), item('lighting.warm-day', 0.28)] })
