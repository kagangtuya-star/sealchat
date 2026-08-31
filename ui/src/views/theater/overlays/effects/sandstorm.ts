import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, horizontalDirection, numberParam, particleConfig } from './effect-helpers'

export const sandstormEffect: SceneOverlayEffectDefinition = {
  id: 'weather.sandstorm',
  name: '沙尘',
  description: '高速横向移动的密集沙粒。',
  category: 'weather',
  defaultParams: { intensity: 1, speed: 1, wind: 1, size: 1, color: '#c49a58' },
  controls: [
    { type: 'number', key: 'intensity', label: '强度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.2, max: 2.5, step: 0.1 },
    { type: 'number', key: 'wind', label: '风向', min: -2, max: 2, step: 0.1 },
    { type: 'number', key: 'size', label: '粒径', min: 0.5, max: 2, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.76,
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.2, 2.5)
    const wind = numberParam(params, 'wind', 1, -2, 2)
    const size = numberParam(params, 'size', 1, 0.5, 2)
    return { renderer: 'particles', config: particleConfig({
      count: 310 * intensity,
      color: colorParam(params, 'color', '#c49a58'),
      opacity: { min: 0.12, max: 0.58 },
      size: { min: 0.7 * size, max: 3 * size },
      speed: 9 * speed,
      direction: horizontalDirection(wind),
      straight: true,
      fpsLimit: 50,
    }) }
  },
}
