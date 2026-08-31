import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam } from './effect-helpers'

export const fogEffect: SceneOverlayEffectDefinition = {
  id: 'weather.fog',
  name: '雾',
  description: '缓慢流动的低对比度雾层。',
  category: 'weather',
  defaultParams: { intensity: 0.7, speed: 1, color: '#d9e2e8' },
  controls: [
    { type: 'number', key: 'intensity', label: '浓度', min: 0.1, max: 1, step: 0.05 },
    { type: 'number', key: 'speed', label: '速度', min: 0.2, max: 2.5, step: 0.1 },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.55,
  buildRenderDescriptor(params) {
    const intensity = numberParam(params, 'intensity', 0.7, 0.1, 1)
    const speed = numberParam(params, 'speed', 1, 0.2, 2.5)
    return { renderer: 'color', config: {
      mode: 'fog',
      color: colorParam(params, 'color', '#d9e2e8'),
      secondaryColor: '#8796a1',
      strength: intensity,
      durationMs: 18_000 / speed,
    } }
  },
}
