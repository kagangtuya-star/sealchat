import type { SceneOverlayEffectDefinition } from '../scene-overlay-types'
import { numberParam } from './effect-helpers'

const fitOptions = ['cover', 'contain', 'fill'] as const

export const customMediaEffect: SceneOverlayEffectDefinition = {
  id: 'custom.media',
  name: '自定义素材',
  description: '使用素材管理器中的图片或动态素材。',
  category: 'special',
  catalog: false,
  defaultParams: {
    fit: 'cover',
    positionX: 50,
    positionY: 50,
    playbackRate: 1,
  },
  controls: [
    {
      type: 'select',
      key: 'fit',
      label: '显示模式',
      options: [
        { label: '铺满', value: 'cover' },
        { label: '完整显示', value: 'contain' },
        { label: '拉伸', value: 'fill' },
      ],
    },
    { type: 'number', key: 'positionX', label: '水平位置', min: 0, max: 100, step: 1, suffix: '%' },
    { type: 'number', key: 'positionY', label: '垂直位置', min: 0, max: 100, step: 1, suffix: '%' },
    { type: 'number', key: 'playbackRate', label: '播放速度', min: 0.25, max: 4, step: 0.25, suffix: 'x' },
  ],
  buildRenderDescriptor(params, _context, binding) {
    const fit = typeof params.fit === 'string' && fitOptions.includes(params.fit as typeof fitOptions[number])
      ? params.fit
      : 'cover'
    return {
      renderer: 'media',
      config: {
        media: binding.media,
        fit,
        positionX: numberParam(params, 'positionX', 50, 0, 100),
        positionY: numberParam(params, 'positionY', 50, 0, 100),
        playbackRate: numberParam(params, 'playbackRate', 1, 0.25, 4),
      },
    }
  },
}

export default customMediaEffect
