import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, horizontalDirection, numberParam, particleConfig } from './effect-helpers'

export const dustEffect: SceneOverlayEffectDefinition = {
  id: 'environment.dust',
  name: '浮尘',
  description: '微弱光线中的缓慢尘埃。',
  category: 'environment',
  defaultParams: { intensity: 1, speed: 1, wind: 0.25, size: 1, color: '#d8c49a' },
  controls: [
    { type: 'number', key: 'intensity', label: '强度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'wind', label: '风向', min: -2, max: 2, step: 0.1 },
    { type: 'number', key: 'size', label: '大小', min: 0.5, max: 2, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.62,
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.2, 2)
    const wind = numberParam(params, 'wind', 0.25, -2, 2)
    const size = numberParam(params, 'size', 1, 0.5, 2)
    return { renderer: 'particles', config: particleConfig({
      count: 72 * intensity,
      color: colorParam(params, 'color', '#d8c49a'),
      opacity: { min: 0.08, max: 0.38 },
      size: { min: 0.7 * size, max: 2.8 * size },
      speed: 0.7 * speed,
      direction: horizontalDirection(wind),
      random: true,
      opacityAnimationSpeed: 0.35,
    }) }
  },
}
