type DisplaySettingsDraftManagedElsewhereKeys =
  | 'themeSelectionMode'
  | 'activePlatformThemeId'
  | 'customThemeEnabled'
  | 'customThemes'
  | 'activeCustomThemeId'

type DisplaySettingsLike = object

export type DisplaySettingsDraftSavePayload<T extends DisplaySettingsLike> = Omit<T, DisplaySettingsDraftManagedElsewhereKeys>

const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

// Keep modal draft aligned with full store snapshot so closing modal never writes stale defaults back.
export const cloneDisplaySettingsDraftSnapshot = <T extends DisplaySettingsLike>(source: T): T => cloneJson(source)

export const syncDisplaySettingsDraft = <T extends DisplaySettingsLike>(target: T, source: T) => {
  Object.assign(target, cloneDisplaySettingsDraftSnapshot(source))
}

export const buildDisplaySettingsDraftSavePayload = <T extends DisplaySettingsLike>(value: T): DisplaySettingsDraftSavePayload<T> => {
  const snapshot = cloneDisplaySettingsDraftSnapshot(value) as any
  delete snapshot.themeSelectionMode
  delete snapshot.activePlatformThemeId
  delete snapshot.customThemeEnabled
  delete snapshot.customThemes
  delete snapshot.activeCustomThemeId
  return snapshot as DisplaySettingsDraftSavePayload<T>
}
