import type { CustomThemeColors } from './themeTypes'

export interface ThemeColorField {
  key: keyof CustomThemeColors
  label: string
  group: string
}

export const themeColorFields: ThemeColorField[] = [
  { key: 'bgSurface', label: '主背景', group: '背景' },
  { key: 'bgElevated', label: '卡片/弹窗', group: '背景' },
  { key: 'bgInput', label: '输入框背景', group: '背景' },
  { key: 'bgHeader', label: '顶栏背景', group: '背景' },
  { key: 'textPrimary', label: '主文字', group: '文字' },
  { key: 'textSecondary', label: '次要文字', group: '文字' },
  { key: 'chatIcBg', label: '场内气泡', group: '聊天区域' },
  { key: 'chatOocBg', label: '场外气泡', group: '聊天区域' },
  { key: 'chatStageBg', label: '聊天舞台', group: '聊天区域' },
  { key: 'chatPreviewBg', label: '预览背景', group: '聊天区域' },
  { key: 'chatPreviewDot', label: '预览圆点', group: '聊天区域' },
  { key: 'borderMute', label: '淡边框', group: '边框' },
  { key: 'borderStrong', label: '强边框', group: '边框' },
  { key: 'primaryColor', label: '主题色', group: '强调色' },
  { key: 'primaryColorHover', label: '悬停主题色', group: '强调色' },
  { key: 'keywordBg', label: '术语高亮背景', group: '术语高亮' },
  { key: 'keywordBorder', label: '术语下划线', group: '术语高亮' },
  { key: 'inlineCodeBg', label: '行内代码背景', group: '代码' },
  { key: 'inlineCodeFg', label: '行内代码文字', group: '代码' },
  { key: 'inlineCodeBorder', label: '行内代码边框', group: '代码' },
]
