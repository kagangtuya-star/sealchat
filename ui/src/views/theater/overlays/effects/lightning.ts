import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { colorParam, numberParam } from './effect-helpers'

export const lightningEffect: SceneOverlayEffectDefinition = {
  id: 'special.lightning',
  name: '闪电',
  description: '低成本随机全屏闪光。',
  category: 'special',
  defaultParams: { strength: 0.85, frequency: 0.2, color: '#e8f4ff' },
  controls: [
    { type: 'number', key: 'strength', label: '亮度', min: 0.05, max: 1, step: 0.05 },
    { type: 'number', key: 'frequency', label: '频率', min: 0.03, max: 2, step: 0.01, suffix: '次/秒' },
    { type: 'color', key: 'color', label: '颜色' },
  ],
  defaultBlendMode: 'screen',
  buildRenderDescriptor(params) {
    return { renderer: 'color', config: {
      mode: 'lightning',
      color: colorParam(params, 'color', '#e8f4ff'),
      strength: numberParam(params, 'strength', 0.85, 0.05, 1),
      frequency: numberParam(params, 'frequency', 0.2, 0.03, 2),
    } }
  },
}
