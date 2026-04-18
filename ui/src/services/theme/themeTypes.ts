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
