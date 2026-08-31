import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam } from './effect-helpers'

export const nightEffect: SceneOverlayEffectDefinition = {
  id: 'lighting.night',
  name: '夜间',
  description: '深蓝冷色调，压低场景暖色。',
  category: 'lighting',
  defaultParams: { color: '#102b58', secondaryColor: '#050b1c' },
  controls: [
    { type: 'color', key: 'color', label: '主色' },
    { type: 'color', key: 'secondaryColor', label: '暗部' },
  ],
  defaultOpacity: 0.58,
  defaultBlendMode: 'multiply',
  buildRenderDescriptor(params) {
    return { renderer: 'color', config: {
      mode: 'solid',
      color: colorParam(params, 'color', '#102b58'),
      secondaryColor: colorParam(params, 'secondaryColor', '#050b1c'),
    } }
  },
}
