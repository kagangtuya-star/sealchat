import { defineStore } from 'pinia'

export type DisplayLayout = 'bubble' | 'compact'
export type DisplayPalette = 'day' | 'night'

export interface DisplaySettings {
  layout: DisplayLayout
  palette: DisplayPalette
  showAvatar: boolean
  mergeNeighbors: boolean
  maxExportMessages: number
  maxExportConcurrency: number
  fontSize: number
  lineHeight: number
  letterSpacing: number
  bubbleGap: number
  paragraphSpacing: number
  messagePaddingX: number
  messagePaddingY: number
  favoriteChannelBarEnabled: boolean
  favoriteChannelIds: string[]
}

export const FAVORITE_CHANNEL_LIMIT = 4

const STORAGE_KEY = 'sealchat_display_settings'

const SLICE_LIMIT_DEFAULT = 5000
const SLICE_LIMIT_MIN = 1000
const SLICE_LIMIT_MAX = 20000
const CONCURRENCY_DEFAULT = 2
const CONCURRENCY_MIN = 1
const CONCURRENCY_MAX = 8

const FONT_SIZE_DEFAULT = 15
const FONT_SIZE_MIN = 12
const FONT_SIZE_MAX = 22
const LINE_HEIGHT_DEFAULT = 1.6
const LINE_HEIGHT_MIN = 1.2
const LINE_HEIGHT_MAX = 2
const LETTER_SPACING_DEFAULT = 0
const LETTER_SPACING_MIN = -1
const LETTER_SPACING_MAX = 2
const BUBBLE_GAP_DEFAULT = 12
const BUBBLE_GAP_MIN = 4
const BUBBLE_GAP_MAX = 48
const PARAGRAPH_SPACING_DEFAULT = 8
const PARAGRAPH_SPACING_MIN = 0
const PARAGRAPH_SPACING_MAX = 24
const MESSAGE_PADDING_X_DEFAULT = 18
const MESSAGE_PADDING_X_MIN = 8
const MESSAGE_PADDING_X_MAX = 48
const MESSAGE_PADDING_Y_DEFAULT = 14
const MESSAGE_PADDING_Y_MIN = 4
const MESSAGE_PADDING_Y_MAX = 32

const coerceLayout = (value?: string): DisplayLayout => (value === 'compact' ? 'compact' : 'bubble')
const coercePalette = (value?: string): DisplayPalette => (value === 'night' ? 'night' : 'day')
const coerceBoolean = (value: any): boolean => value !== false
const coerceNumberInRange = (value: any, fallback: number, min: number, max: number): number => {
  const num = Number(value)
  if (!Number.isFinite(num)) return fallback
  if (num < min) return min
  if (num > max) return max
  return Math.round(num)
}
const coerceFloatInRange = (value: any, fallback: number, min: number, max: number): number => {
  const num = Number(value)
  if (!Number.isFinite(num)) return fallback
  if (num < min) return min
  if (num > max) return max
  return num
}
const normalizeFavoriteIds = (value: any): string[] => {
  if (!Array.isArray(value)) return []
  const normalized: string[] = []
  const seen = new Set<string>()
  for (const entry of value) {
    let id = ''
    if (typeof entry === 'string') {
      id = entry.trim()
    } else if (entry != null && typeof entry.toString === 'function') {
      id = String(entry).trim()
    }
    if (!id || seen.has(id)) {
      continue
    }
    normalized.push(id)
    seen.add(id)
    if (normalized.length >= FAVORITE_CHANNEL_LIMIT) break
  }
  return normalized
}

export const createDefaultDisplaySettings = (): DisplaySettings => ({
  layout: 'bubble',
  palette: 'day',
  showAvatar: true,
  mergeNeighbors: true,
  maxExportMessages: SLICE_LIMIT_DEFAULT,
  maxExportConcurrency: CONCURRENCY_DEFAULT,
  fontSize: FONT_SIZE_DEFAULT,
  lineHeight: LINE_HEIGHT_DEFAULT,
  letterSpacing: LETTER_SPACING_DEFAULT,
  bubbleGap: BUBBLE_GAP_DEFAULT,
  paragraphSpacing: PARAGRAPH_SPACING_DEFAULT,
  messagePaddingX: MESSAGE_PADDING_X_DEFAULT,
  messagePaddingY: MESSAGE_PADDING_Y_DEFAULT,
  favoriteChannelBarEnabled: false,
  favoriteChannelIds: [],
})
const defaultSettings = (): DisplaySettings => createDefaultDisplaySettings()

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
      maxExportMessages: coerceNumberInRange(
        parsed.maxExportMessages,
        SLICE_LIMIT_DEFAULT,
        SLICE_LIMIT_MIN,
        SLICE_LIMIT_MAX,
      ),
      maxExportConcurrency: coerceNumberInRange(
        parsed.maxExportConcurrency,
        CONCURRENCY_DEFAULT,
        CONCURRENCY_MIN,
        CONCURRENCY_MAX,
      ),
      fontSize: coerceNumberInRange(parsed.fontSize, FONT_SIZE_DEFAULT, FONT_SIZE_MIN, FONT_SIZE_MAX),
      lineHeight: coerceFloatInRange(parsed.lineHeight, LINE_HEIGHT_DEFAULT, LINE_HEIGHT_MIN, LINE_HEIGHT_MAX),
      letterSpacing: coerceFloatInRange(
        parsed.letterSpacing,
        LETTER_SPACING_DEFAULT,
        LETTER_SPACING_MIN,
        LETTER_SPACING_MAX,
      ),
      bubbleGap: coerceNumberInRange(parsed.bubbleGap, BUBBLE_GAP_DEFAULT, BUBBLE_GAP_MIN, BUBBLE_GAP_MAX),
      paragraphSpacing: coerceNumberInRange(
        parsed.paragraphSpacing,
        PARAGRAPH_SPACING_DEFAULT,
        PARAGRAPH_SPACING_MIN,
        PARAGRAPH_SPACING_MAX,
      ),
      messagePaddingX: coerceNumberInRange(
        parsed.messagePaddingX,
        MESSAGE_PADDING_X_DEFAULT,
        MESSAGE_PADDING_X_MIN,
        MESSAGE_PADDING_X_MAX,
      ),
      messagePaddingY: coerceNumberInRange(
        parsed.messagePaddingY,
        MESSAGE_PADDING_Y_DEFAULT,
        MESSAGE_PADDING_Y_MIN,
        MESSAGE_PADDING_Y_MAX,
      ),
      favoriteChannelBarEnabled: coerceBoolean(parsed.favoriteChannelBarEnabled),
      favoriteChannelIds: normalizeFavoriteIds(parsed.favoriteChannelIds),
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
  maxExportMessages:
    patch && Object.prototype.hasOwnProperty.call(patch, 'maxExportMessages')
      ? coerceNumberInRange(patch.maxExportMessages, SLICE_LIMIT_DEFAULT, SLICE_LIMIT_MIN, SLICE_LIMIT_MAX)
      : base.maxExportMessages,
  maxExportConcurrency:
    patch && Object.prototype.hasOwnProperty.call(patch, 'maxExportConcurrency')
      ? coerceNumberInRange(
          patch.maxExportConcurrency,
          CONCURRENCY_DEFAULT,
          CONCURRENCY_MIN,
          CONCURRENCY_MAX,
        )
      : base.maxExportConcurrency,
  fontSize:
    patch && Object.prototype.hasOwnProperty.call(patch, 'fontSize')
      ? coerceNumberInRange(patch.fontSize, FONT_SIZE_DEFAULT, FONT_SIZE_MIN, FONT_SIZE_MAX)
      : base.fontSize,
  lineHeight:
    patch && Object.prototype.hasOwnProperty.call(patch, 'lineHeight')
      ? coerceFloatInRange(patch.lineHeight, LINE_HEIGHT_DEFAULT, LINE_HEIGHT_MIN, LINE_HEIGHT_MAX)
      : base.lineHeight,
  letterSpacing:
    patch && Object.prototype.hasOwnProperty.call(patch, 'letterSpacing')
      ? coerceFloatInRange(
          patch.letterSpacing,
          LETTER_SPACING_DEFAULT,
          LETTER_SPACING_MIN,
          LETTER_SPACING_MAX,
        )
      : base.letterSpacing,
  bubbleGap:
    patch && Object.prototype.hasOwnProperty.call(patch, 'bubbleGap')
      ? coerceNumberInRange(patch.bubbleGap, BUBBLE_GAP_DEFAULT, BUBBLE_GAP_MIN, BUBBLE_GAP_MAX)
      : base.bubbleGap,
  paragraphSpacing:
    patch && Object.prototype.hasOwnProperty.call(patch, 'paragraphSpacing')
      ? coerceNumberInRange(
          patch.paragraphSpacing,
          PARAGRAPH_SPACING_DEFAULT,
          PARAGRAPH_SPACING_MIN,
          PARAGRAPH_SPACING_MAX,
        )
      : base.paragraphSpacing,
  messagePaddingX:
    patch && Object.prototype.hasOwnProperty.call(patch, 'messagePaddingX')
      ? coerceNumberInRange(
          patch.messagePaddingX,
          MESSAGE_PADDING_X_DEFAULT,
          MESSAGE_PADDING_X_MIN,
          MESSAGE_PADDING_X_MAX,
        )
      : base.messagePaddingX,
  messagePaddingY:
    patch && Object.prototype.hasOwnProperty.call(patch, 'messagePaddingY')
      ? coerceNumberInRange(
          patch.messagePaddingY,
          MESSAGE_PADDING_Y_DEFAULT,
          MESSAGE_PADDING_Y_MIN,
          MESSAGE_PADDING_Y_MAX,
        )
      : base.messagePaddingY,
  favoriteChannelBarEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelBarEnabled')
      ? coerceBoolean(patch.favoriteChannelBarEnabled)
      : base.favoriteChannelBarEnabled,
  favoriteChannelIds:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelIds')
      ? normalizeFavoriteIds(patch.favoriteChannelIds)
      : base.favoriteChannelIds.slice(),
})

export const useDisplayStore = defineStore('display', {
  state: () => ({
    settings: loadSettings(),
  }),
  getters: {
    layout: (state) => state.settings.layout,
    palette: (state) => state.settings.palette,
    showAvatar: (state) => state.settings.showAvatar,
    favoriteBarEnabled: (state) => state.settings.favoriteChannelBarEnabled,
    favoriteChannelIds: (state) => state.settings.favoriteChannelIds,
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
    setFavoriteBarEnabled(enabled: boolean) {
      const normalized = !!enabled
      if (this.settings.favoriteChannelBarEnabled === normalized) return
      this.settings.favoriteChannelBarEnabled = normalized
      this.persist()
    },
    setFavoriteChannelIds(ids: string[]) {
      const normalized = normalizeFavoriteIds(ids)
      const current = this.settings.favoriteChannelIds
      if (normalized.length === current.length && normalized.every((id, index) => id === current[index])) {
        return
      }
      this.settings.favoriteChannelIds = normalized
      this.persist()
    },
    addFavoriteChannel(channelId: string) {
      const id = typeof channelId === 'string' ? channelId.trim() : ''
      if (!id || this.settings.favoriteChannelIds.includes(id)) return
      if (this.settings.favoriteChannelIds.length >= FAVORITE_CHANNEL_LIMIT) return
      this.settings.favoriteChannelIds = [...this.settings.favoriteChannelIds, id]
      this.persist()
    },
    removeFavoriteChannel(channelId: string) {
      const id = typeof channelId === 'string' ? channelId.trim() : ''
      if (!id) return
      const next = this.settings.favoriteChannelIds.filter(existing => existing !== id)
      this.setFavoriteChannelIds(next)
    },
    reorderFavoriteChannels(nextOrder: string[]) {
      this.setFavoriteChannelIds(nextOrder)
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
      const setVar = (name: string, value: string) => {
        root.style.setProperty(name, value)
      }
      setVar('--chat-font-size', `${effective.fontSize / 16}rem`)
      setVar('--chat-line-height', `${effective.lineHeight}`)
      setVar('--chat-letter-spacing', `${effective.letterSpacing}px`)
      setVar('--chat-bubble-gap', `${effective.bubbleGap}px`)
      setVar('--chat-paragraph-spacing', `${effective.paragraphSpacing}px`)
      setVar('--chat-message-padding-x', `${effective.messagePaddingX}px`)
      setVar('--chat-message-padding-y', `${effective.messagePaddingY}px`)
    },
  },
})
