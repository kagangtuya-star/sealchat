import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam, particleConfig, verticalDirection } from './effect-helpers'

export const rainLightEffect: SceneOverlayEffectDefinition = {
  id: 'weather.rain.light',
  name: '小雨',
  description: '稀疏、轻快的细雨。',
  category: 'weather',
  defaultParams: { intensity: 1, speed: 1, wind: 0, color: '#b9dcff' },
  controls: [
    { type: 'number', key: 'intensity', label: '强度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.3, max: 2.5, step: 0.1 },
    { type: 'number', key: 'wind', label: '风向', min: -2, max: 2, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.72,
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.3, 2.5)
    const wind = numberParam(params, 'wind', 0, -2, 2)
    return { renderer: 'particles', config: particleConfig({
      count: 105 * intensity,
      color: colorParam(params, 'color', '#b9dcff'),
      opacity: { min: 0.18, max: 0.48 },
      size: { min: 8, max: 15 },
      speed: 12 * speed,
      direction: verticalDirection(wind),
      shape: 'line',
      straight: true,
    }) }
  },
}
