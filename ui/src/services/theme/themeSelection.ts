import type { CustomTheme, PlatformTheme, ThemeSelectionMode } from './themeTypes'
export type { PlatformTheme, ThemeSelectionMode } from './themeTypes'

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
