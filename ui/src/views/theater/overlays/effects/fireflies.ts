import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam, particleConfig } from './effect-helpers'

export const firefliesEffect: SceneOverlayEffectDefinition = {
  id: 'environment.fireflies',
  name: '萤火虫',
  description: '缓慢游动、明暗闪烁的暖色光点。',
  category: 'environment',
  defaultParams: { intensity: 1, speed: 1, size: 1, color: '#dfff70' },
  controls: [
    { type: 'number', key: 'intensity', label: '数量', min: 0.2, max: 2, step: 0.1 },
    { type: 'number', key: 'speed', label: '速度', min: 0.1, max: 2, step: 0.1 },
    { type: 'number', key: 'size', label: '大小', min: 0.5, max: 2.5, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.9,
  defaultBlendMode: 'screen',
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 1, 0.2, 2)
    const speed = numberParam(params, 'speed', 1, 0.1, 2)
    const size = numberParam(params, 'size', 1, 0.5, 2.5)
    return { renderer: 'particles', config: particleConfig({
      count: 42 * intensity,
      color: colorParam(params, 'color', '#dfff70'),
      opacity: { min: 0.08, max: 0.98 },
      size: { min: 1.2 * size, max: 3.2 * size },
      speed: 0.75 * speed,
      direction: 'none',
      random: true,
      opacityAnimationSpeed: 0.8,
      sizeAnimationSpeed: 0.35,
    }) }
  },
}
