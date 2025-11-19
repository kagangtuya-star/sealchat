import { defineStore } from 'pinia'
import { useChatStore } from './chat'

export type DisplayLayout = 'bubble' | 'compact'
export type DisplayPalette = 'day' | 'night'

export interface DisplaySettings {
  layout: DisplayLayout
  palette: DisplayPalette
  showAvatar: boolean
  showInputPreview: boolean
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
  sendShortcut: 'enter' | 'ctrlEnter'
  favoriteChannelBarEnabled: boolean
  favoriteChannelIdsByWorld: Record<string, string[]>
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
const SEND_SHORTCUT_DEFAULT: 'enter' | 'ctrlEnter' = 'enter'
const coerceSendShortcut = (value?: string): 'enter' | 'ctrlEnter' => (value === 'ctrlEnter' ? 'ctrlEnter' : 'enter')

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

const normalizeFavoriteMap = (value: any): Record<string, string[]> => {
  if (!value || typeof value !== 'object') return {}
  const result: Record<string, string[]> = {}
  Object.entries(value as Record<string, unknown>).forEach(([key, ids]) => {
    const normalized = normalizeFavoriteIds(ids)
    if (normalized.length) {
      result[key] = normalized.slice(0, FAVORITE_CHANNEL_LIMIT)
    }
  })
  return result
}

const WORLD_FALLBACK_KEY = '__global__'

export const createDefaultDisplaySettings = (): DisplaySettings => ({
  layout: 'compact',
  palette: 'day',
  showAvatar: true,
  showInputPreview: true,
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
  sendShortcut: SEND_SHORTCUT_DEFAULT,
  favoriteChannelBarEnabled: false,
  favoriteChannelIdsByWorld: {},
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
    const favoriteChannelIdsByWorld = normalizeFavoriteMap((parsed as any)?.favoriteChannelIdsByWorld)
    if (Object.keys(favoriteChannelIdsByWorld).length === 0 && Array.isArray((parsed as any)?.favoriteChannelIds)) {
      const legacyIds = normalizeFavoriteIds((parsed as any)?.favoriteChannelIds)
      if (legacyIds.length) {
        favoriteChannelIdsByWorld[WORLD_FALLBACK_KEY] = legacyIds.slice(0, FAVORITE_CHANNEL_LIMIT)
      }
    }
    return {
      layout: coerceLayout(parsed.layout),
      palette: coercePalette(parsed.palette),
      showAvatar: coerceBoolean(parsed.showAvatar),
      showInputPreview: coerceBoolean(parsed.showInputPreview),
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
      sendShortcut: coerceSendShortcut((parsed as any)?.sendShortcut),
      favoriteChannelBarEnabled: coerceBoolean(parsed.favoriteChannelBarEnabled),
      favoriteChannelIdsByWorld,
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
  showInputPreview:
    patch && Object.prototype.hasOwnProperty.call(patch, 'showInputPreview')
      ? coerceBoolean(patch.showInputPreview)
      : base.showInputPreview,
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
  sendShortcut:
    patch && Object.prototype.hasOwnProperty.call(patch, 'sendShortcut')
      ? coerceSendShortcut((patch as any).sendShortcut)
      : base.sendShortcut,
  favoriteChannelBarEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelBarEnabled')
      ? coerceBoolean(patch.favoriteChannelBarEnabled)
      : base.favoriteChannelBarEnabled,
  favoriteChannelIdsByWorld:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelIdsByWorld')
      ? normalizeFavoriteMap((patch as any).favoriteChannelIdsByWorld)
      : { ...base.favoriteChannelIdsByWorld },
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
  },
  actions: {
    getCurrentWorldKey(worldId?: string) {
      const chat = useChatStore();
      const key = worldId || chat.currentWorldId || WORLD_FALLBACK_KEY;
      return key;
    },
    getFavoriteChannelIds(worldId?: string) {
      const key = this.getCurrentWorldKey(worldId);
      return this.settings.favoriteChannelIdsByWorld[key] || [];
    },
    setFavoriteChannelIds(ids: string[], worldId?: string) {
      const key = this.getCurrentWorldKey(worldId);
      const normalized = normalizeFavoriteIds(ids).slice(0, FAVORITE_CHANNEL_LIMIT);
      const current = this.settings.favoriteChannelIdsByWorld[key] || [];
      if (normalized.length === current.length && normalized.every((id, index) => id === current[index])) {
        return;
      }
      this.settings.favoriteChannelIdsByWorld = {
        ...this.settings.favoriteChannelIdsByWorld,
        [key]: normalized,
      };
      this.persist();
    },
    addFavoriteChannel(channelId: string, worldId?: string) {
      const id = typeof channelId === 'string' ? channelId.trim() : '';
      if (!id) return;
      const key = this.getCurrentWorldKey(worldId);
      const current = this.settings.favoriteChannelIdsByWorld[key] || [];
      if (current.includes(id) || current.length >= FAVORITE_CHANNEL_LIMIT) return;
      this.settings.favoriteChannelIdsByWorld = {
        ...this.settings.favoriteChannelIdsByWorld,
        [key]: [...current, id],
      };
      this.persist();
    },
    removeFavoriteChannel(channelId: string, worldId?: string) {
      const id = typeof channelId === 'string' ? channelId.trim() : '';
      if (!id) return;
      const key = this.getCurrentWorldKey(worldId);
      const current = this.settings.favoriteChannelIdsByWorld[key] || [];
      const next = current.filter(existing => existing !== id);
      this.settings.favoriteChannelIdsByWorld = {
        ...this.settings.favoriteChannelIdsByWorld,
        [key]: next,
      };
      this.persist();
    },
    reorderFavoriteChannels(nextOrder: string[], worldId?: string) {
      this.setFavoriteChannelIds(nextOrder, worldId);
    },
    syncFavoritesWithChannels(availableIds: string[], worldId?: string) {
      const key = this.getCurrentWorldKey(worldId);
      const current = this.settings.favoriteChannelIdsByWorld[key] || [];
      if (!current.length) return;
      if (!Array.isArray(availableIds) || !availableIds.length) {
        this.settings.favoriteChannelIdsByWorld = {
          ...this.settings.favoriteChannelIdsByWorld,
          [key]: current,
        };
        return;
      }
      const availableSet = new Set(availableIds);
      const filtered = current.filter(id => availableSet.has(id));
      if (filtered.length === current.length) return;
      this.settings.favoriteChannelIdsByWorld = {
        ...this.settings.favoriteChannelIdsByWorld,
        [key]: filtered,
      };
      this.persist();
    },
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
