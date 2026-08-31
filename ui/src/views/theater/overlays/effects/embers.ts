import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam, particleConfig, verticalDirection } from './effect-helpers'

export const embersEffect: SceneOverlayEffectDefinition = {
  id: 'environment.embers',
  name: '余烬 / 火星',
  description: '向上漂浮并闪烁的火星。',
  category: 'environment',
  defaultParams: { intensity: 1, speed: 1, wind: 0.2, size: 1, color: '#ff7a1a' },
  controls: [
    { type: 'number', key: 'intensity', label: '强度', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.2, max: 2.5, step: 0.1 },
    { type: 'number', key: 'wind', label: '风向', min: -2, max: 2, step: 0.1 },
    { type: 'number', key: 'size', label: '大小', min: 0.5, max: 2, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.86,
  defaultBlendMode: 'screen',
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.2, 2.5)
    const wind = numberParam(params, 'wind', 0.2, -2, 2)
    const size = numberParam(params, 'size', 1, 0.5, 2)
    return { renderer: 'particles', config: particleConfig({
      count: 82 * intensity,
      color: colorParam(params, 'color', '#ff7a1a'),
      opacity: { min: 0.24, max: 0.95 },
      size: { min: 1 * size, max: 3.8 * size },
      speed: 2.8 * speed,
      direction: verticalDirection(wind, false),
      random: true,
      opacityAnimationSpeed: 1.1,
      sizeAnimationSpeed: 0.5,
    }) }
  },
}
