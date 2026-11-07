export interface CustomThemeColors {
  bgSurface?: string
  bgElevated?: string
  bgInput?: string
  bgHeader?: string
  textPrimary?: string
  textSecondary?: string
  chatIcBg?: string
  chatOocBg?: string
  chatStageBg?: string
  chatPreviewBg?: string
  chatPreviewDot?: string
  borderMute?: string
  borderStrong?: string
  primaryColor?: string
  primaryColorHover?: string
  actionRibbonHoverText?: string
  keywordBg?: string
  keywordBorder?: string
  inlineCodeBg?: string
  inlineCodeFg?: string
  inlineCodeBorder?: string
}

export interface CustomTheme {
  id: string
  name: string
  colors: CustomThemeColors
  createdAt: number
  updatedAt: number
}

export interface PlatformTheme {
  id: string
  name: string
  colors: CustomThemeColors
  createdAt: number
  updatedAt: number
}

export type ThemeSelectionMode = 'inherit' | 'platform' | 'personal' | 'none'

// 日间/夜间各自绑定的主题来源；inherit 表示跟随平台对应时段的默认主题
export type PaletteThemeSource = 'inherit' | 'platform' | 'personal' | 'none'

export interface PaletteThemeBinding {
  source: PaletteThemeSource
  themeId: string | null
}
