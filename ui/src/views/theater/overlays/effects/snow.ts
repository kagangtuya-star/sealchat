import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam, particleConfig, verticalDirection } from './effect-helpers'

export const snowEffect: SceneOverlayEffectDefinition = {
  id: 'weather.snow',
  name: '飘雪',
  description: '缓慢飘落、大小不一的雪花。',
  category: 'weather',
  defaultParams: { intensity: 1, speed: 1, wind: 0, size: 1, color: '#ffffff' },
  controls: [
    { type: 'number', key: 'intensity', label: '强度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'wind', label: '风向', min: -2, max: 2, step: 0.1 },
    { type: 'number', key: 'size', label: '大小', min: 0.4, max: 2.5, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.9,
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.2, 2)
    const wind = numberParam(params, 'wind', 0, -2, 2)
    const size = numberParam(params, 'size', 1, 0.4, 2.5)
    return { renderer: 'particles', config: particleConfig({
      count: 120 * intensity,
      color: colorParam(params, 'color', '#ffffff'),
      opacity: { min: 0.34, max: 0.86 },
      size: { min: 1.2 * size, max: 4.5 * size },
      speed: 2.2 * speed,
      direction: verticalDirection(wind),
      random: true,
      opacityAnimationSpeed: 0.35,
    }) }
  },
}
