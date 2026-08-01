export type {
  CursorAssetConfig,
  CursorMode,
  CursorSlot,
  CursorThemeConfig,
} from '@/types'

export const CURSOR_SLOTS = ['default', 'pointer', 'text', 'grab', 'grabbing', 'not-allowed'] as const

export const CURSOR_SLOT_LABELS: Record<(typeof CURSOR_SLOTS)[number], string> = {
  default: '普通浏览',
  pointer: '按钮与链接',
  text: '文本输入与选择',
  grab: '可拖动对象',
  grabbing: '正在拖动',
  'not-allowed': '禁用或不可操作',
}
