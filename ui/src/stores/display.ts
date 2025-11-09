import { defineStore } from 'pinia'

export type DisplayLayout = 'bubble' | 'compact'
export type DisplayPalette = 'day' | 'night'

export interface DisplaySettings {
  layout: DisplayLayout
  palette: DisplayPalette
  showAvatar: boolean
  mergeNeighbors: boolean
}

const STORAGE_KEY = 'sealchat_display_settings'

const coerceLayout = (value?: string): DisplayLayout => (value === 'compact' ? 'compact' : 'bubble')
const coercePalette = (value?: string): DisplayPalette => (value === 'night' ? 'night' : 'day')
const coerceBoolean = (value: any): boolean => value !== false

const defaultSettings = (): DisplaySettings => ({
  layout: 'bubble',
  palette: 'day',
  showAvatar: true,
  mergeNeighbors: true,
})

const loadSettings = (): DisplaySettings => {
  if (typeof window === 'undefined') {
    return defaultSettings()
  }
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return defaultSettings()
    }
    const parsed = JSON.parse(raw) as Partial<DisplaySettings>
    return {
      layout: coerceLayout(parsed.layout),
      palette: coercePalette(parsed.palette),
      showAvatar: coerceBoolean(parsed.showAvatar),
      mergeNeighbors: coerceBoolean(parsed.mergeNeighbors),
    }
  } catch (error) {
    console.warn('加载显示模式设置失败，使用默认值', error)
    return defaultSettings()
  }
}

const normalizeWith = (base: DisplaySettings, patch?: Partial<DisplaySettings>): DisplaySettings => ({
  layout: patch && patch.layout ? coerceLayout(patch.layout) : base.layout,
  palette: patch && patch.palette ? coercePalette(patch.palette) : base.palette,
  showAvatar:
    patch && Object.prototype.hasOwnProperty.call(patch, 'showAvatar')
      ? coerceBoolean(patch.showAvatar)
      : base.showAvatar,
  mergeNeighbors:
    patch && Object.prototype.hasOwnProperty.call(patch, 'mergeNeighbors')
      ? coerceBoolean(patch.mergeNeighbors)
      : base.mergeNeighbors,
})

export const useDisplayStore = defineStore('display', {
  state: () => ({
    settings: loadSettings(),
  }),
  getters: {
    layout: (state) => state.settings.layout,
    palette: (state) => state.settings.palette,
    showAvatar: (state) => state.settings.showAvatar,
  },
  actions: {
    updateSettings(patch: Partial<DisplaySettings>) {
      this.settings = normalizeWith(this.settings, patch)
      this.persist()
      this.applyTheme()
    },
    reset() {
      this.settings = defaultSettings()
      this.persist()
      this.applyTheme()
    },
    persist() {
      if (typeof window === 'undefined') return
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(this.settings))
      } catch (error) {
        console.warn('显示模式设置写入失败', error)
      }
    },
    applyTheme(target?: DisplaySettings) {
      if (typeof document === 'undefined') return
      const effective = target || this.settings
      const root = document.documentElement
      root.dataset.displayPalette = effective.palette
      root.dataset.displayLayout = effective.layout
    },
  },
})
