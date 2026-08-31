import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam } from './effect-helpers'

export const colorWashEffect: SceneOverlayEffectDefinition = {
  id: 'lighting.color-wash',
  name: '自定义色调',
  description: '使用实例透明度控制强度的纯色叠加。',
  category: 'lighting',
  defaultParams: { color: '#5b7cfa' },
  controls: [
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultOpacity: 0.3,
  defaultBlendMode: 'soft-light',
  buildRenderDescriptor(params) {
    return { renderer: 'color', config: {
      mode: 'solid',
      color: colorParam(params, 'color', '#5b7cfa'),
    } }
  },
}
