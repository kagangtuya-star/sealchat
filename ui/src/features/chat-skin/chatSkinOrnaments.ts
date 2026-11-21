// 内置装饰纹样：随前端代码分发的单色 SVG，以 `builtin:<id>` 引用，渲染时作为遮罩并按图层着色。
// 配置与消息快照中只保存引用名，不包含任何图片内容。

import type { ChatSkinResolvedRef } from './chatSkinRender'
import catEar from './ornaments/cat-ear.svg?url'
import bunnyEar from './ornaments/bunny-ear.svg?url'
import bow from './ornaments/bow.svg?url'
import star from './ornaments/star.svg?url'
import sparkle from './ornaments/sparkle.svg?url'
import heart from './ornaments/heart.svg?url'
import flower from './ornaments/flower.svg?url'
import paw from './ornaments/paw.svg?url'
import crown from './ornaments/crown.svg?url'
import leaf from './ornaments/leaf.svg?url'
import moon from './ornaments/moon.svg?url'
import cloud from './ornaments/cloud.svg?url'
import seal from './ornaments/seal.svg?url'
import corner from './ornaments/corner.svg?url'
import scallop from './ornaments/scallop.svg?url'
import wave from './ornaments/wave.svg?url'
import tail from './ornaments/tail.svg?url'
import dots from './ornaments/dots.svg?url'

export type ChatSkinOrnamentGroup = 'shape' | 'edge' | 'stamp'

export interface ChatSkinOrnament {
  id: string
  label: string
  group: ChatSkinOrnamentGroup
  src: string
  aspect: number
  defaultTint: string
  /** 新建图层时的建议尺寸（px）。 */
  size: number
}

export const CHAT_SKIN_ORNAMENTS: ChatSkinOrnament[] = [
  { id: 'cat-ear', label: '猫耳', group: 'shape', src: catEar, aspect: 1, defaultTint: '#f4a6b8', size: 26 },
  { id: 'bunny-ear', label: '兔耳', group: 'shape', src: bunnyEar, aspect: 2.4, defaultTint: '#f8c8d4', size: 18 },
  { id: 'bow', label: '蝴蝶结', group: 'shape', src: bow, aspect: 0.583, defaultTint: '#e85d75', size: 34 },
  { id: 'star', label: '星星', group: 'shape', src: star, aspect: 1, defaultTint: '#f5c542', size: 18 },
  { id: 'sparkle', label: '闪光', group: 'shape', src: sparkle, aspect: 1, defaultTint: '#ffd76a', size: 16 },
  { id: 'heart', label: '爱心', group: 'shape', src: heart, aspect: 0.9, defaultTint: '#ef5b7b', size: 18 },
  { id: 'flower', label: '花朵', group: 'shape', src: flower, aspect: 1, defaultTint: '#f29cb7', size: 22 },
  { id: 'paw', label: '肉垫', group: 'shape', src: paw, aspect: 1, defaultTint: '#c58b6b', size: 20 },
  { id: 'crown', label: '王冠', group: 'shape', src: crown, aspect: 0.667, defaultTint: '#e3b341', size: 28 },
  { id: 'leaf', label: '叶片', group: 'shape', src: leaf, aspect: 1, defaultTint: '#6dbb7a', size: 22 },
  { id: 'moon', label: '月牙', group: 'shape', src: moon, aspect: 1, defaultTint: '#f2d27a', size: 20 },
  { id: 'cloud', label: '云朵', group: 'shape', src: cloud, aspect: 0.583, defaultTint: '#cfe3f7', size: 36 },
  { id: 'tail', label: '气泡尾巴', group: 'edge', src: tail, aspect: 1, defaultTint: '#ffffff', size: 14 },
  { id: 'corner', label: '角花', group: 'edge', src: corner, aspect: 1, defaultTint: '#b08d57', size: 26 },
  { id: 'scallop', label: '蕾丝边', group: 'edge', src: scallop, aspect: 0.12, defaultTint: '#ffffff', size: 120 },
  { id: 'wave', label: '波浪线', group: 'edge', src: wave, aspect: 0.1, defaultTint: '#7aa7d9', size: 120 },
  { id: 'dots', label: '渐隐圆点', group: 'edge', src: dots, aspect: 0.25, defaultTint: '#94a3b8', size: 40 },
  { id: 'seal', label: '图章', group: 'stamp', src: seal, aspect: 1, defaultTint: '#c0392b', size: 34 },
]

const ornamentMap = new Map(CHAT_SKIN_ORNAMENTS.map((item) => [item.id, item]))

export const findChatSkinOrnament = (ref: string) => (
  ref.startsWith('builtin:') ? ornamentMap.get(ref.slice('builtin:'.length)) || null : null
)

/** 统一的资源引用解析：内置纹样 → 遮罩；附件 → 普通图片。 */
export const createChatSkinRefResolver = (
  resolveAttachment: (ref: string) => string,
) => (ref: string): ChatSkinResolvedRef | null => {
  if (!ref) return null
  if (ref.startsWith('builtin:')) {
    const ornament = findChatSkinOrnament(ref)
    return ornament ? { src: ornament.src, mask: true, defaultTint: ornament.defaultTint } : null
  }
  if (!ref.startsWith('id:')) return null
  const src = resolveAttachment(ref)
  return src ? { src, mask: false } : null
}
