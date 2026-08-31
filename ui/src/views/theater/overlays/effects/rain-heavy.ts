import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam, particleConfig, verticalDirection } from './effect-helpers'

export const rainHeavyEffect: SceneOverlayEffectDefinition = {
  id: 'weather.rain.heavy',
  name: '大雨',
  description: '密集、快速的暴雨线。',
  category: 'weather',
  defaultParams: { intensity: 1, speed: 1, wind: 0.35, color: '#9fcfff' },
  controls: [
    { type: 'number', key: 'intensity', label: '强度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.3, max: 2.5, step: 0.1 },
    { type: 'number', key: 'wind', label: '风向', min: -2, max: 2, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.88,
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.3, 2.5)
    const wind = numberParam(params, 'wind', 0.35, -2, 2)
    return { renderer: 'particles', config: particleConfig({
      count: 270 * intensity,
      color: colorParam(params, 'color', '#9fcfff'),
      opacity: { min: 0.28, max: 0.72 },
      size: { min: 13, max: 24 },
      speed: 22 * speed,
      direction: verticalDirection(wind),
      shape: 'line',
      straight: true,
      fpsLimit: 50,
    }) }
  },
}
