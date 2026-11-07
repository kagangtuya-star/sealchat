import type { CustomTheme, PaletteThemeBinding, PlatformTheme, ThemeSelectionMode } from './themeTypes'
export type { PaletteThemeBinding, PaletteThemeSource, PlatformTheme, ThemeSelectionMode } from './themeTypes'

export interface LegacyThemeSelectionSnapshot {
  customThemeEnabled?: boolean
  activeCustomThemeId?: string | null
}

export interface ThemeSelectionMigrationResult {
  themeSelectionMode: ThemeSelectionMode
  activePlatformThemeId: string | null
}

export interface ResolvedThemeSelection {
  source: 'platform' | 'personal' | 'none'
  resolvedMode: ThemeSelectionMode
  theme: PlatformTheme | CustomTheme | null
}

export interface ResolveEffectiveThemeSelectionInput {
  selectionMode: ThemeSelectionMode
  activePlatformThemeId?: string | null
  activePersonalThemeId?: string | null
  platformThemes: PlatformTheme[]
  defaultPlatformThemeId?: string | null
  personalThemes: CustomTheme[]
}

const normalizeId = (value: string | null | undefined): string => {
  if (typeof value !== 'string') return ''
  return value.trim()
}

export const migrateLegacyThemeSelection = (
  snapshot: LegacyThemeSelectionSnapshot,
): ThemeSelectionMigrationResult => {
  if (snapshot.customThemeEnabled && normalizeId(snapshot.activeCustomThemeId)) {
    return {
      themeSelectionMode: 'personal',
      activePlatformThemeId: null,
    }
  }
  return {
    themeSelectionMode: 'inherit',
    activePlatformThemeId: null,
  }
}

export const resolveEffectiveThemeSelection = (
  input: ResolveEffectiveThemeSelectionInput,
): ResolvedThemeSelection => {
  const platformThemes = Array.isArray(input.platformThemes) ? input.platformThemes : []
  const personalThemes = Array.isArray(input.personalThemes) ? input.personalThemes : []
  const defaultPlatformThemeId = normalizeId(input.defaultPlatformThemeId)
  const activePlatformThemeId = normalizeId(input.activePlatformThemeId)
  const activePersonalThemeId = normalizeId(input.activePersonalThemeId)

  const findPlatformTheme = (id: string) => platformThemes.find((item) => item.id === id) || null
  const findPersonalTheme = (id: string) => personalThemes.find((item) => item.id === id) || null

  if (input.selectionMode === 'none') {
    return {
      source: 'none',
      resolvedMode: 'none',
      theme: null,
    }
  }

  if (input.selectionMode === 'personal') {
    const personalTheme = activePersonalThemeId ? findPersonalTheme(activePersonalThemeId) : null
    if (personalTheme) {
      return {
        source: 'personal',
        resolvedMode: 'personal',
        theme: personalTheme,
      }
    }
  }

  if (input.selectionMode === 'platform') {
    const selectedPlatformTheme = activePlatformThemeId ? findPlatformTheme(activePlatformThemeId) : null
    if (selectedPlatformTheme) {
      return {
        source: 'platform',
        resolvedMode: 'platform',
        theme: selectedPlatformTheme,
      }
    }
  }

  const defaultPlatformTheme = defaultPlatformThemeId ? findPlatformTheme(defaultPlatformThemeId) : null
  if (defaultPlatformTheme) {
    return {
      source: 'platform',
      resolvedMode: 'inherit',
      theme: defaultPlatformTheme,
    }
  }

  return {
    source: 'none',
    resolvedMode: input.selectionMode === 'personal' ? 'inherit' : input.selectionMode === 'platform' ? 'inherit' : input.selectionMode,
    theme: null,
  }
}

export type ThemePalette = 'day' | 'night'

export interface LegacyPaletteThemeSnapshot {
  followSystemTheme?: boolean
  systemDayTheme?: unknown
  systemNightTheme?: unknown
  themeSelectionMode?: ThemeSelectionMode
  activePlatformThemeId?: string | null
  activeCustomThemeId?: string | null
}

export interface PaletteThemeBindings {
  dayThemeBinding: PaletteThemeBinding
  nightThemeBinding: PaletteThemeBinding
}

export interface PaletteDefaultPlatformThemeInput {
  palette: ThemePalette
  platformThemes: PlatformTheme[]
  defaultPlatformThemeId?: string | null
  defaultPlatformDayThemeId?: string | null
  defaultPlatformNightThemeId?: string | null
}

export interface ResolvePaletteThemeSelectionInput extends PaletteDefaultPlatformThemeInput {
  binding: PaletteThemeBinding
  personalThemes: CustomTheme[]
}

export const normalizePaletteThemeBinding = (value: unknown): PaletteThemeBinding => {
  if (!value || typeof value !== 'object') return { source: 'inherit', themeId: null }
  const source = (value as any).source
  const themeId = normalizeId((value as any).themeId) || null
  if ((source === 'platform' || source === 'personal') && themeId) {
    return { source, themeId }
  }
  if (source === 'none') {
    return { source: 'none', themeId: null }
  }
  return { source: 'inherit', themeId: null }
}

// 从旧版设置推导日间/夜间主题绑定：
// - 旧版“跟随系统”时使用 systemDayTheme/systemNightTheme（旧的“系统基础配色”视为跟随平台默认）；
// - 否则旧版单一主题选择同时作用于日间与夜间，保持原有观感。
export const migrateLegacyPaletteThemeBindings = (
  snapshot: LegacyPaletteThemeSnapshot,
): PaletteThemeBindings => {
  if (snapshot.followSystemTheme) {
    return {
      dayThemeBinding: normalizePaletteThemeBinding(snapshot.systemDayTheme),
      nightThemeBinding: normalizePaletteThemeBinding(snapshot.systemNightTheme),
    }
  }
  let binding: PaletteThemeBinding = { source: 'inherit', themeId: null }
  if (snapshot.themeSelectionMode === 'none') {
    binding = { source: 'none', themeId: null }
  } else if (snapshot.themeSelectionMode === 'platform') {
    binding = normalizePaletteThemeBinding({ source: 'platform', themeId: snapshot.activePlatformThemeId })
  } else if (snapshot.themeSelectionMode === 'personal') {
    binding = normalizePaletteThemeBinding({ source: 'personal', themeId: snapshot.activeCustomThemeId })
  }
  return {
    dayThemeBinding: { ...binding },
    nightThemeBinding: { ...binding },
  }
}

// 时段默认主题优先，未设置时回退到平台通用默认主题
export const resolvePaletteDefaultPlatformThemeId = (input: PaletteDefaultPlatformThemeInput): string => {
  const platformThemes = Array.isArray(input.platformThemes) ? input.platformThemes : []
  const candidates = [
    input.palette === 'day' ? input.defaultPlatformDayThemeId : input.defaultPlatformNightThemeId,
    input.defaultPlatformThemeId,
  ]
  for (const candidate of candidates) {
    const id = normalizeId(candidate)
    if (id && platformThemes.some((item) => item.id === id)) return id
  }
  return ''
}

export const resolvePaletteThemeSelection = (
  input: ResolvePaletteThemeSelectionInput,
): ResolvedThemeSelection => {
  const binding = normalizePaletteThemeBinding(input.binding)
  return resolveEffectiveThemeSelection({
    selectionMode: binding.source,
    activePlatformThemeId: binding.source === 'platform' ? binding.themeId : null,
    activePersonalThemeId: binding.source === 'personal' ? binding.themeId : null,
    platformThemes: input.platformThemes,
    defaultPlatformThemeId: resolvePaletteDefaultPlatformThemeId(input),
    personalThemes: input.personalThemes,
  })
}

export const describePaletteThemeSelection = (
  binding: PaletteThemeBinding,
  resolved: ResolvedThemeSelection,
): string => {
  if (binding.source === 'none' || resolved.resolvedMode === 'none') return '基础配色'
  const themeName = resolved.theme?.name
  if (!themeName) return '平台默认（未设置，使用基础配色）'
  if (resolved.resolvedMode === 'inherit') return `平台默认 · ${themeName}`
  return resolved.source === 'personal' ? `个人 · ${themeName}` : `平台 · ${themeName}`
}
