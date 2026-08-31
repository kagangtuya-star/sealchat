import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam } from './effect-helpers'

export const duskEffect: SceneOverlayEffectDefinition = {
  id: 'lighting.dusk',
  name: '黄昏',
  description: '暖橙与深棕组成的黄昏色调。',
  category: 'lighting',
  defaultParams: { color: '#b85f2c', secondaryColor: '#4a241f' },
  controls: [
    { type: 'color', key: 'color', label: '暖色' },
    { type: 'color', key: 'secondaryColor', label: '暗部' },
  ],
  defaultOpacity: 0.42,
  defaultBlendMode: 'soft-light',
  buildRenderDescriptor(params) {
    return { renderer: 'color', config: {
      mode: 'fog',
      color: colorParam(params, 'color', '#b85f2c'),
      secondaryColor: colorParam(params, 'secondaryColor', '#4a241f'),
      durationMs: 32_000,
    } }
  },
}
