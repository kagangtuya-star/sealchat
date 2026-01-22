import { defineStore } from 'pinia'
import { useChatStore } from './chat'

export type DisplayLayout = 'bubble' | 'compact'
export type DisplayPalette = 'day' | 'night'

export interface FavoriteHotkey {
  combo: string
  key: string
  ctrl?: boolean
  meta?: boolean
  alt?: boolean
  shift?: boolean
}

export interface ToolbarHotkeyConfig {
  enabled: boolean
  hotkey: FavoriteHotkey | null
}

export type ToolbarHotkeyKey =
  | 'icToggle'
  | 'whisper'
  | 'upload'
  | 'richMode'
  | 'broadcast'
  | 'emoji'
  | 'wideInput'
  | 'history'
  | 'diceTray'

export type TimestampFormat = 'relative' | 'time' | 'datetime' | 'datetimeSeconds'

// 自定义主题颜色配置
export interface CustomThemeColors {
  // 背景色
  bgSurface?: string        // 主背景
  bgElevated?: string       // 卡片/弹窗背景
  bgInput?: string          // 输入框背景
  bgHeader?: string         // 顶栏背景
  // 文字色
  textPrimary?: string      // 主文字
  textSecondary?: string    // 次要文字
  // 聊天区域
  chatIcBg?: string         // 场内消息背景
  chatOocBg?: string        // 场外消息背景
  chatStageBg?: string      // 聊天舞台背景
  chatPreviewBg?: string    // 预览区背景
  chatPreviewDot?: string   // 预览区圆点
  // 边框
  borderMute?: string       // 淡边框
  borderStrong?: string     // 强边框
  // 强调色
  primaryColor?: string     // 主题强调色
  primaryColorHover?: string
  // 术语高亮
  keywordBg?: string        // 术语高亮背景
  keywordBorder?: string    // 术语高亮下划线
}

export interface CustomTheme {
  id: string
  name: string
  colors: CustomThemeColors
  createdAt: number
  updatedAt: number
}

export interface DisplaySettings {
  layout: DisplayLayout
  palette: DisplayPalette
  showAvatar: boolean
  avatarSize: number            // 头像大小 (px)
  avatarBorderRadius: number    // 头像圆角 (0-50, 50为圆形)
  showInputPreview: boolean
  autoScrollTypingPreview: boolean
  mergeNeighbors: boolean
  alwaysShowTimestamp: boolean
  timestampFormat: TimestampFormat
  maxExportMessages: number
  maxExportConcurrency: number
  fontSize: number
  lineHeight: number
  letterSpacing: number
  bubbleGap: number
  compactBubbleGap: number
  paragraphSpacing: number
  messagePaddingX: number
  messagePaddingY: number
  sendShortcut: 'enter' | 'ctrlEnter'
  enableIcToggleHotkey: boolean
  favoriteChannelBarEnabled: boolean
  favoriteChannelIdsByWorld: Record<string, string[]>
  favoriteChannelHotkeysByWorld: Record<string, Record<string, FavoriteHotkey>>
  worldKeywordHighlightEnabled: boolean
  worldKeywordUnderlineOnly: boolean
  worldKeywordTooltipEnabled: boolean
  worldKeywordDeduplicateEnabled: boolean
  worldKeywordTooltipTextIndent: number  // 术语气泡多段首行缩进（em），0 为关闭
  worldKeywordQuickInputEnabled: boolean  // 术语快捷输入
  worldKeywordQuickInputTrigger: string   // 术语快捷输入触发字符，默认 /
  toolbarHotkeys: Record<ToolbarHotkeyKey, ToolbarHotkeyConfig>
  autoSwitchRoleOnIcOocToggle: boolean
  // 拖拽排序
  showDragIndicator: boolean  // 拖拽时显示蓝色指示线
  // 自定义主题
  customThemeEnabled: boolean
  customThemes: CustomTheme[]
  activeCustomThemeId: string | null
  // 右键菜单
  disableContextMenu: boolean
  // 输入区域自定义高度
  inputAreaHeight: number  // 0 means auto
  // 人物卡
  characterCardBadgeEnabled: boolean
  characterCardBadgeTemplateByWorld: Record<string, string>
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
const COMPACT_BUBBLE_GAP_DEFAULT = 4
const COMPACT_BUBBLE_GAP_MIN = 0
const COMPACT_BUBBLE_GAP_MAX = 24
const PARAGRAPH_SPACING_DEFAULT = 8
const PARAGRAPH_SPACING_MIN = 0
const PARAGRAPH_SPACING_MAX = 24
const MESSAGE_PADDING_X_DEFAULT = 18
const MESSAGE_PADDING_X_MIN = 8
const MESSAGE_PADDING_X_MAX = 48
const MESSAGE_PADDING_Y_DEFAULT = 14
const MESSAGE_PADDING_Y_MIN = 4
const MESSAGE_PADDING_Y_MAX = 32
const INPUT_AREA_HEIGHT_DEFAULT = 0  // 0 means auto (use default autosize)
const INPUT_AREA_HEIGHT_MIN = 80
const INPUT_AREA_HEIGHT_MAX = 600
export const INPUT_AREA_HEIGHT_LIMITS = {
  MIN: INPUT_AREA_HEIGHT_MIN,
  MAX: INPUT_AREA_HEIGHT_MAX,
}

// 头像样式常量
const AVATAR_SIZE_DEFAULT = 48
const AVATAR_SIZE_MIN = 32
const AVATAR_SIZE_MAX = 72
export const AVATAR_SIZE_LIMITS = {
  DEFAULT: AVATAR_SIZE_DEFAULT,
  MIN: AVATAR_SIZE_MIN,
  MAX: AVATAR_SIZE_MAX,
}
const AVATAR_BORDER_RADIUS_DEFAULT = 14 // 约等于 0.85rem
const AVATAR_BORDER_RADIUS_MIN = 0
const AVATAR_BORDER_RADIUS_MAX = 50 // 50% = 圆形
export const AVATAR_BORDER_RADIUS_LIMITS = {
  DEFAULT: AVATAR_BORDER_RADIUS_DEFAULT,
  MIN: AVATAR_BORDER_RADIUS_MIN,
  MAX: AVATAR_BORDER_RADIUS_MAX,
}
const normalizeInputAreaHeight = (value: unknown) => {
  const raw = coerceNumberInRange(
    value,
    INPUT_AREA_HEIGHT_DEFAULT,
    INPUT_AREA_HEIGHT_DEFAULT,
    INPUT_AREA_HEIGHT_MAX,
  )
  if (raw <= 0) return 0
  return Math.max(raw, INPUT_AREA_HEIGHT_MIN)
}
const KEYWORD_TOOLTIP_TEXT_INDENT_DEFAULT = 1  // 1em - 中文标准首行缩进
const KEYWORD_TOOLTIP_TEXT_INDENT_MIN = 0
const KEYWORD_TOOLTIP_TEXT_INDENT_MAX = 4
const SEND_SHORTCUT_DEFAULT: 'enter' | 'ctrlEnter' = 'enter'
const coerceSendShortcut = (value?: string): 'enter' | 'ctrlEnter' => (value === 'ctrlEnter' ? 'ctrlEnter' : 'enter')
const QUICK_INPUT_TRIGGER_DEFAULT = '/'
const coerceQuickInputTrigger = (value?: string): string => {
  if (typeof value === 'string' && value.length === 1) return value
  return QUICK_INPUT_TRIGGER_DEFAULT
}
const TIMESTAMP_FORMAT_VALUES: TimestampFormat[] = ['relative', 'time', 'datetime', 'datetimeSeconds']
const TIMESTAMP_FORMAT_DEFAULT: TimestampFormat = 'datetimeSeconds'
const coerceTimestampFormat = (value?: string): TimestampFormat => {
  if (typeof value === 'string') {
    const normalized = value.trim() as TimestampFormat
    if (TIMESTAMP_FORMAT_VALUES.includes(normalized)) {
      return normalized
    }
  }
  return TIMESTAMP_FORMAT_DEFAULT
}

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

const isPlainObject = (value: unknown): value is Record<string, any> =>
  !!value && typeof value === 'object' && !Array.isArray(value)

const composeHotkeyComboLabel = (key: string, flags: { ctrl?: boolean; meta?: boolean; alt?: boolean; shift?: boolean }) => {
  const parts: string[] = []
  if (flags.ctrl) parts.push('Ctrl')
  if (flags.meta) parts.push('Cmd')
  if (flags.alt) parts.push('Alt')
  if (flags.shift) parts.push('Shift')
  parts.push(key.length === 1 ? key.toUpperCase() : key)
  return parts.join('+')
}

const normalizeFavoriteHotkeyEntry = (value: any): FavoriteHotkey | null => {
  if (!isPlainObject(value)) return null
  const key = typeof value.key === 'string' ? value.key.trim() : ''
  if (!key) return null
  const flags = {
    ctrl: value.ctrl ? true : undefined,
    meta: value.meta ? true : undefined,
    alt: value.alt ? true : undefined,
    shift: value.shift ? true : undefined,
  }
  const combo =
    typeof value.combo === 'string' && value.combo.trim()
      ? value.combo.trim()
      : composeHotkeyComboLabel(key, flags)
  return {
    combo,
    key,
    ctrl: flags.ctrl,
    meta: flags.meta,
    alt: flags.alt,
    shift: flags.shift,
  }
}

const normalizeFavoriteHotkeyMap = (value: any): Record<string, Record<string, FavoriteHotkey>> => {
  if (!isPlainObject(value)) return {}
  const result: Record<string, Record<string, FavoriteHotkey>> = {}
  Object.entries(value).forEach(([worldId, entries]) => {
    if (!isPlainObject(entries)) {
      return
    }
    const normalizedWorld: Record<string, FavoriteHotkey> = {}
    Object.entries(entries as Record<string, unknown>).forEach(([channelId, hotkey]) => {
      const normalized = normalizeFavoriteHotkeyEntry(hotkey)
      if (normalized) {
        normalizedWorld[channelId] = normalized
      }
    })
    if (Object.keys(normalizedWorld).length) {
      result[worldId] = normalizedWorld
    }
  })
  return result
}

const cloneDeepHotkeys = (value: Record<string, Record<string, FavoriteHotkey>>): Record<string, Record<string, FavoriteHotkey>> => {
  const clone: Record<string, Record<string, FavoriteHotkey>> = {}
  Object.entries(value || {}).forEach(([worldId, entries]) => {
    clone[worldId] = { ...entries }
  })
  return clone
}

const WORLD_FALLBACK_KEY = '__global__'

const createDefaultToolbarHotkeys = (): Record<ToolbarHotkeyKey, ToolbarHotkeyConfig> => ({
  icToggle: {
    enabled: true,
    hotkey: { combo: 'Esc', key: 'Escape' },
  },
  whisper: {
    enabled: true,
    hotkey: { combo: 'Ctrl+W', key: 'W', ctrl: true },
  },
  upload: {
    enabled: true,
    hotkey: { combo: 'Ctrl+U', key: 'U', ctrl: true },
  },
  richMode: {
    enabled: true,
    hotkey: { combo: 'Ctrl+Shift+R', key: 'R', ctrl: true, shift: true },
  },
  broadcast: {
    enabled: true,
    hotkey: { combo: 'Ctrl+B', key: 'B', ctrl: true },
  },
  emoji: {
    enabled: true,
    hotkey: { combo: 'Ctrl+E', key: 'E', ctrl: true },
  },
  wideInput: {
    enabled: true,
    hotkey: { combo: 'Ctrl+L', key: 'L', ctrl: true },
  },
  history: {
    enabled: true,
    hotkey: { combo: 'Ctrl+H', key: 'H', ctrl: true },
  },
  diceTray: {
    enabled: true,
    hotkey: { combo: 'Ctrl+D', key: 'D', ctrl: true },
  },
})


export const createDefaultDisplaySettings = (): DisplaySettings => ({
  layout: 'compact',
  palette: 'day',
  showAvatar: true,
  avatarSize: AVATAR_SIZE_DEFAULT,
  avatarBorderRadius: AVATAR_BORDER_RADIUS_DEFAULT,
  showInputPreview: true,
  autoScrollTypingPreview: false,
  mergeNeighbors: true,
  alwaysShowTimestamp: false,
  timestampFormat: TIMESTAMP_FORMAT_DEFAULT,
  maxExportMessages: SLICE_LIMIT_DEFAULT,
  maxExportConcurrency: CONCURRENCY_DEFAULT,
  fontSize: FONT_SIZE_DEFAULT,
  lineHeight: LINE_HEIGHT_DEFAULT,
  letterSpacing: LETTER_SPACING_DEFAULT,
  bubbleGap: BUBBLE_GAP_DEFAULT,
  compactBubbleGap: COMPACT_BUBBLE_GAP_DEFAULT,
  paragraphSpacing: PARAGRAPH_SPACING_DEFAULT,
  messagePaddingX: MESSAGE_PADDING_X_DEFAULT,
  messagePaddingY: MESSAGE_PADDING_Y_DEFAULT,
  sendShortcut: SEND_SHORTCUT_DEFAULT,
  enableIcToggleHotkey: true,
  favoriteChannelBarEnabled: false,
  favoriteChannelIdsByWorld: {},
  favoriteChannelHotkeysByWorld: {},
  worldKeywordHighlightEnabled: true,
  worldKeywordUnderlineOnly: false,
  worldKeywordTooltipEnabled: true,
  worldKeywordDeduplicateEnabled: true,
  worldKeywordTooltipTextIndent: KEYWORD_TOOLTIP_TEXT_INDENT_DEFAULT,
  worldKeywordQuickInputEnabled: true,
  worldKeywordQuickInputTrigger: '/',
  toolbarHotkeys: createDefaultToolbarHotkeys(),
  autoSwitchRoleOnIcOocToggle: true,
  showDragIndicator: false,  // 默认隐藏拖拽指示线
  customThemeEnabled: false,
  customThemes: [],
  activeCustomThemeId: null,
  disableContextMenu: true,  // 默认禁用浏览器右键菜单
  inputAreaHeight: INPUT_AREA_HEIGHT_DEFAULT,
  characterCardBadgeEnabled: true,
  characterCardBadgeTemplateByWorld: {},
})
const defaultSettings = (): DisplaySettings => createDefaultDisplaySettings()

const normalizeToolbarHotkeyConfig = (value: any): ToolbarHotkeyConfig => {
  const enabled = typeof value?.enabled === 'boolean' ? value.enabled : true
  const hotkey = value?.hotkey ? normalizeFavoriteHotkeyEntry(value.hotkey) : null
  return { enabled, hotkey }
}

const normalizeToolbarHotkeys = (value: any): Record<ToolbarHotkeyKey, ToolbarHotkeyConfig> => {
  const defaults = createDefaultToolbarHotkeys()
  if (!value || typeof value !== 'object') {
    return defaults
  }
  const result: Record<string, ToolbarHotkeyConfig> = {}
  const keys: ToolbarHotkeyKey[] = [
    'icToggle',
    'whisper',
    'upload',
    'richMode',
    'broadcast',
    'emoji',
    'wideInput',
    'history',
    'diceTray',
  ]
  keys.forEach((key) => {
    result[key] = value[key] ? normalizeToolbarHotkeyConfig(value[key]) : defaults[key]
  })
  return result as Record<ToolbarHotkeyKey, ToolbarHotkeyConfig>
}

const normalizeCustomThemeColors = (value: any): CustomThemeColors => {
  if (!value || typeof value !== 'object') return {}
  const result: CustomThemeColors = {}
  const colorKeys: (keyof CustomThemeColors)[] = [
    'bgSurface', 'bgElevated', 'bgInput', 'bgHeader',
    'textPrimary', 'textSecondary',
    'chatIcBg', 'chatOocBg', 'chatStageBg', 'chatPreviewBg', 'chatPreviewDot',
    'borderMute', 'borderStrong',
    'primaryColor', 'primaryColorHover',
    'keywordBg', 'keywordBorder'
  ]
  colorKeys.forEach(key => {
    if (typeof value[key] === 'string' && value[key].trim()) {
      result[key] = value[key].trim()
    }
  })
  return result
}

const normalizeCustomTheme = (value: any): CustomTheme | null => {
  if (!value || typeof value !== 'object') return null
  const id = typeof value.id === 'string' ? value.id.trim() : ''
  const name = typeof value.name === 'string' ? value.name.trim() : ''
  if (!id || !name) return null
  return {
    id,
    name,
    colors: normalizeCustomThemeColors(value.colors),
    createdAt: typeof value.createdAt === 'number' ? value.createdAt : Date.now(),
    updatedAt: typeof value.updatedAt === 'number' ? value.updatedAt : Date.now(),
  }
}

const normalizeCustomThemes = (value: any): CustomTheme[] => {
  if (!Array.isArray(value)) return []
  const result: CustomTheme[] = []
  const seenIds = new Set<string>()
  for (const item of value) {
    const normalized = normalizeCustomTheme(item)
    if (normalized && !seenIds.has(normalized.id)) {
      result.push(normalized)
      seenIds.add(normalized.id)
    }
  }
  return result
}
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
    const favoriteChannelHotkeysByWorld = normalizeFavoriteHotkeyMap(
      (parsed as any)?.favoriteChannelHotkeysByWorld,
    )
    const toolbarHotkeys = normalizeToolbarHotkeys((parsed as any)?.toolbarHotkeys)
    if ((parsed as any)?.enableIcToggleHotkey === false && toolbarHotkeys.icToggle) {
      toolbarHotkeys.icToggle = {
        ...toolbarHotkeys.icToggle,
        enabled: false,
      }
    }
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
      avatarSize: coerceNumberInRange(
        (parsed as any)?.avatarSize,
        AVATAR_SIZE_DEFAULT,
        AVATAR_SIZE_MIN,
        AVATAR_SIZE_MAX,
      ),
      avatarBorderRadius: coerceNumberInRange(
        (parsed as any)?.avatarBorderRadius,
        AVATAR_BORDER_RADIUS_DEFAULT,
        AVATAR_BORDER_RADIUS_MIN,
        AVATAR_BORDER_RADIUS_MAX,
      ),
      showInputPreview: coerceBoolean(parsed.showInputPreview),
      autoScrollTypingPreview: coerceBoolean((parsed as any)?.autoScrollTypingPreview ?? false),
      mergeNeighbors: coerceBoolean(parsed.mergeNeighbors),
      alwaysShowTimestamp: coerceBoolean((parsed as any)?.alwaysShowTimestamp ?? false),
      timestampFormat: coerceTimestampFormat((parsed as any)?.timestampFormat),
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
      compactBubbleGap: coerceNumberInRange(
        parsed.compactBubbleGap,
        COMPACT_BUBBLE_GAP_DEFAULT,
        COMPACT_BUBBLE_GAP_MIN,
        COMPACT_BUBBLE_GAP_MAX,
      ),
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
      enableIcToggleHotkey: coerceBoolean((parsed as any)?.enableIcToggleHotkey ?? true),
      favoriteChannelBarEnabled: coerceBoolean(parsed.favoriteChannelBarEnabled),
      favoriteChannelIdsByWorld,
      favoriteChannelHotkeysByWorld,
      worldKeywordHighlightEnabled: coerceBoolean((parsed as any)?.worldKeywordHighlightEnabled ?? true),
      worldKeywordUnderlineOnly: coerceBoolean((parsed as any)?.worldKeywordUnderlineOnly ?? false),
      worldKeywordTooltipEnabled: coerceBoolean((parsed as any)?.worldKeywordTooltipEnabled ?? true),
      worldKeywordDeduplicateEnabled: coerceBoolean((parsed as any)?.worldKeywordDeduplicateEnabled ?? true),
      worldKeywordTooltipTextIndent: coerceFloatInRange(
        (parsed as any)?.worldKeywordTooltipTextIndent,
        KEYWORD_TOOLTIP_TEXT_INDENT_DEFAULT,
        KEYWORD_TOOLTIP_TEXT_INDENT_MIN,
        KEYWORD_TOOLTIP_TEXT_INDENT_MAX,
      ),
      worldKeywordQuickInputEnabled: coerceBoolean((parsed as any)?.worldKeywordQuickInputEnabled ?? true),
      worldKeywordQuickInputTrigger: coerceQuickInputTrigger((parsed as any)?.worldKeywordQuickInputTrigger),
      toolbarHotkeys,
      autoSwitchRoleOnIcOocToggle: coerceBoolean((parsed as any)?.autoSwitchRoleOnIcOocToggle ?? true),
      showDragIndicator: coerceBoolean((parsed as any)?.showDragIndicator ?? false),
      customThemeEnabled: coerceBoolean((parsed as any)?.customThemeEnabled ?? false),
      customThemes: normalizeCustomThemes((parsed as any)?.customThemes),
      activeCustomThemeId: typeof (parsed as any)?.activeCustomThemeId === 'string' ? (parsed as any).activeCustomThemeId : null,
      disableContextMenu: coerceBoolean((parsed as any)?.disableContextMenu ?? true),
      inputAreaHeight: normalizeInputAreaHeight((parsed as any)?.inputAreaHeight),
      characterCardBadgeEnabled: coerceBoolean((parsed as any)?.characterCardBadgeEnabled ?? true),
      characterCardBadgeTemplateByWorld: isPlainObject((parsed as any)?.characterCardBadgeTemplateByWorld)
        ? (parsed as any).characterCardBadgeTemplateByWorld
        : {},
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
  avatarSize:
    patch && Object.prototype.hasOwnProperty.call(patch, 'avatarSize')
      ? coerceNumberInRange((patch as any).avatarSize, AVATAR_SIZE_DEFAULT, AVATAR_SIZE_MIN, AVATAR_SIZE_MAX)
      : base.avatarSize,
  avatarBorderRadius:
    patch && Object.prototype.hasOwnProperty.call(patch, 'avatarBorderRadius')
      ? coerceNumberInRange((patch as any).avatarBorderRadius, AVATAR_BORDER_RADIUS_DEFAULT, AVATAR_BORDER_RADIUS_MIN, AVATAR_BORDER_RADIUS_MAX)
      : base.avatarBorderRadius,
  showInputPreview:
    patch && Object.prototype.hasOwnProperty.call(patch, 'showInputPreview')
      ? coerceBoolean(patch.showInputPreview)
      : base.showInputPreview,
  autoScrollTypingPreview:
    patch && Object.prototype.hasOwnProperty.call(patch, 'autoScrollTypingPreview')
      ? coerceBoolean((patch as any).autoScrollTypingPreview)
      : base.autoScrollTypingPreview,
  mergeNeighbors:
    patch && Object.prototype.hasOwnProperty.call(patch, 'mergeNeighbors')
      ? coerceBoolean(patch.mergeNeighbors)
      : base.mergeNeighbors,
  alwaysShowTimestamp:
    patch && Object.prototype.hasOwnProperty.call(patch, 'alwaysShowTimestamp')
      ? coerceBoolean((patch as any)?.alwaysShowTimestamp)
      : base.alwaysShowTimestamp,
  timestampFormat:
    patch && Object.prototype.hasOwnProperty.call(patch, 'timestampFormat')
      ? coerceTimestampFormat((patch as any)?.timestampFormat)
      : base.timestampFormat,
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
  compactBubbleGap:
    patch && Object.prototype.hasOwnProperty.call(patch, 'compactBubbleGap')
      ? coerceNumberInRange(patch.compactBubbleGap, COMPACT_BUBBLE_GAP_DEFAULT, COMPACT_BUBBLE_GAP_MIN, COMPACT_BUBBLE_GAP_MAX)
      : base.compactBubbleGap,
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
  enableIcToggleHotkey:
    patch && Object.prototype.hasOwnProperty.call(patch, 'enableIcToggleHotkey')
      ? coerceBoolean((patch as any).enableIcToggleHotkey)
      : base.enableIcToggleHotkey,
  favoriteChannelBarEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelBarEnabled')
      ? coerceBoolean(patch.favoriteChannelBarEnabled)
      : base.favoriteChannelBarEnabled,
  favoriteChannelIdsByWorld:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelIdsByWorld')
      ? normalizeFavoriteMap((patch as any).favoriteChannelIdsByWorld)
      : { ...base.favoriteChannelIdsByWorld },
  favoriteChannelHotkeysByWorld:
    patch && Object.prototype.hasOwnProperty.call(patch, 'favoriteChannelHotkeysByWorld')
      ? normalizeFavoriteHotkeyMap((patch as any).favoriteChannelHotkeysByWorld)
      : cloneDeepHotkeys(base.favoriteChannelHotkeysByWorld),
  worldKeywordHighlightEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordHighlightEnabled')
      ? coerceBoolean((patch as any).worldKeywordHighlightEnabled)
      : base.worldKeywordHighlightEnabled,
  worldKeywordUnderlineOnly:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordUnderlineOnly')
      ? coerceBoolean((patch as any).worldKeywordUnderlineOnly)
      : base.worldKeywordUnderlineOnly,
  worldKeywordTooltipEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordTooltipEnabled')
      ? coerceBoolean((patch as any).worldKeywordTooltipEnabled)
      : base.worldKeywordTooltipEnabled,
  worldKeywordDeduplicateEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordDeduplicateEnabled')
      ? coerceBoolean((patch as any).worldKeywordDeduplicateEnabled)
      : base.worldKeywordDeduplicateEnabled,
  worldKeywordTooltipTextIndent:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordTooltipTextIndent')
      ? coerceFloatInRange(
        (patch as any).worldKeywordTooltipTextIndent,
        KEYWORD_TOOLTIP_TEXT_INDENT_DEFAULT,
        KEYWORD_TOOLTIP_TEXT_INDENT_MIN,
        KEYWORD_TOOLTIP_TEXT_INDENT_MAX,
      )
      : base.worldKeywordTooltipTextIndent,
  worldKeywordQuickInputEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordQuickInputEnabled')
      ? coerceBoolean((patch as any).worldKeywordQuickInputEnabled)
      : base.worldKeywordQuickInputEnabled,
  worldKeywordQuickInputTrigger:
    patch && Object.prototype.hasOwnProperty.call(patch, 'worldKeywordQuickInputTrigger')
      ? coerceQuickInputTrigger((patch as any).worldKeywordQuickInputTrigger)
      : base.worldKeywordQuickInputTrigger,
  toolbarHotkeys:
    patch && Object.prototype.hasOwnProperty.call(patch, 'toolbarHotkeys')
      ? normalizeToolbarHotkeys((patch as any).toolbarHotkeys)
      : base.toolbarHotkeys,
  autoSwitchRoleOnIcOocToggle:
    patch && Object.prototype.hasOwnProperty.call(patch, 'autoSwitchRoleOnIcOocToggle')
      ? coerceBoolean((patch as any).autoSwitchRoleOnIcOocToggle)
      : base.autoSwitchRoleOnIcOocToggle,
  showDragIndicator:
    patch && Object.prototype.hasOwnProperty.call(patch, 'showDragIndicator')
      ? coerceBoolean((patch as any).showDragIndicator)
      : base.showDragIndicator,
  customThemeEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'customThemeEnabled')
      ? coerceBoolean((patch as any).customThemeEnabled)
      : base.customThemeEnabled,
  customThemes:
    patch && Object.prototype.hasOwnProperty.call(patch, 'customThemes')
      ? normalizeCustomThemes((patch as any).customThemes)
      : base.customThemes,
  activeCustomThemeId:
    patch && Object.prototype.hasOwnProperty.call(patch, 'activeCustomThemeId')
      ? (typeof (patch as any).activeCustomThemeId === 'string' ? (patch as any).activeCustomThemeId : null)
      : base.activeCustomThemeId,
  disableContextMenu:
    patch && Object.prototype.hasOwnProperty.call(patch, 'disableContextMenu')
      ? coerceBoolean((patch as any).disableContextMenu)
      : base.disableContextMenu,
  inputAreaHeight:
    patch && Object.prototype.hasOwnProperty.call(patch, 'inputAreaHeight')
      ? normalizeInputAreaHeight((patch as any).inputAreaHeight)
      : base.inputAreaHeight,
  characterCardBadgeEnabled:
    patch && Object.prototype.hasOwnProperty.call(patch, 'characterCardBadgeEnabled')
      ? coerceBoolean((patch as any).characterCardBadgeEnabled)
      : base.characterCardBadgeEnabled,
  characterCardBadgeTemplateByWorld:
    patch && Object.prototype.hasOwnProperty.call(patch, 'characterCardBadgeTemplateByWorld')
      ? (isPlainObject((patch as any).characterCardBadgeTemplateByWorld)
          ? (patch as any).characterCardBadgeTemplateByWorld
          : {})
      : { ...base.characterCardBadgeTemplateByWorld },
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
    getFavoriteHotkeyMap(worldId?: string) {
      const key = this.getCurrentWorldKey(worldId);
      return this.settings.favoriteChannelHotkeysByWorld[key] || {};
    },
    getFavoriteHotkey(channelId: string, worldId?: string) {
      const map = this.getFavoriteHotkeyMap(worldId);
      return map[channelId];
    },
    setFavoriteHotkey(
      channelId: string,
      hotkey: FavoriteHotkey | null | undefined,
      worldId?: string,
    ): { success: boolean; reason?: 'conflict' | 'invalid'; conflictChannelId?: string } {
      const id = typeof channelId === 'string' ? channelId.trim() : '';
      if (!id) {
        return { success: false, reason: 'invalid' };
      }
      const key = this.getCurrentWorldKey(worldId);
      const normalized = hotkey ? normalizeFavoriteHotkeyEntry(hotkey) : null;
      if (hotkey && !normalized) {
        return { success: false, reason: 'invalid' };
      }
      const existingWorld = this.settings.favoriteChannelHotkeysByWorld[key] || {};
      if (normalized) {
        const conflict = Object.entries(existingWorld).find(
          ([otherId, entry]) => otherId !== id && entry.combo === normalized.combo,
        );
        if (conflict) {
          return { success: false, reason: 'conflict', conflictChannelId: conflict[0] };
        }
      }
      const nextWorld: Record<string, FavoriteHotkey> = { ...existingWorld };
      if (normalized) {
        nextWorld[id] = normalized;
      } else {
        delete nextWorld[id];
      }
      if (Object.keys(nextWorld).length === 0) {
        const { [key]: _removed, ...rest } = this.settings.favoriteChannelHotkeysByWorld;
        this.settings.favoriteChannelHotkeysByWorld = rest;
      } else {
        this.settings.favoriteChannelHotkeysByWorld = {
          ...this.settings.favoriteChannelHotkeysByWorld,
          [key]: nextWorld,
        };
      }
      this.persist();
      return { success: true };
    },
    clearFavoriteHotkey(channelId: string, worldId?: string) {
      this.setFavoriteHotkey(channelId, null, worldId);
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
      this.clearFavoriteHotkey(id, worldId);
      this.persist();
    },
    reorderFavoriteChannels(nextOrder: string[], worldId?: string) {
      this.setFavoriteChannelIds(nextOrder, worldId);
    },
    pruneFavoriteHotkeys(allowedIds: string[], worldId?: string) {
      const key = this.getCurrentWorldKey(worldId);
      const current = this.settings.favoriteChannelHotkeysByWorld[key];
      if (!current) return;
      const allowed = new Set(allowedIds);
      const nextEntries = Object.entries(current).filter(([channelId]) => allowed.has(channelId));
      if (nextEntries.length === Object.keys(current).length) {
        return;
      }
      if (nextEntries.length === 0) {
        const { [key]: _removed, ...rest } = this.settings.favoriteChannelHotkeysByWorld;
        this.settings.favoriteChannelHotkeysByWorld = rest;
      } else {
        this.settings.favoriteChannelHotkeysByWorld = {
          ...this.settings.favoriteChannelHotkeysByWorld,
          [key]: Object.fromEntries(nextEntries),
        };
      }
      this.persist();
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
      this.pruneFavoriteHotkeys(filtered, worldId);
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
      const removeVar = (name: string) => {
        root.style.removeProperty(name)
      }
      setVar('--chat-font-size', `${effective.fontSize / 16}rem`)
      setVar('--chat-line-height', `${effective.lineHeight}`)
      setVar('--chat-letter-spacing', `${effective.letterSpacing}px`)
      setVar('--chat-bubble-gap', `${effective.bubbleGap}px`)
      setVar('--chat-compact-gap', `${effective.compactBubbleGap}px`)
      setVar('--chat-paragraph-spacing', `${effective.paragraphSpacing}px`)
      setVar('--chat-message-padding-x', `${effective.messagePaddingX}px`)
      setVar('--chat-message-padding-y', `${effective.messagePaddingY}px`)

      // Apply avatar style
      setVar('--chat-avatar-size', `${effective.avatarSize}px`)
      // Calculate border-radius: use percentage for values > 25 (approaching circle)
      const radiusValue = effective.avatarBorderRadius >= 50
        ? '50%'
        : `${effective.avatarBorderRadius}%`
      setVar('--chat-avatar-radius', radiusValue)

      // Apply custom theme colors
      const customColorVars = [
        '--sc-bg-surface', '--sc-bg-elevated', '--sc-bg-input', '--sc-bg-header',
        '--sc-text-primary', '--sc-text-secondary',
        '--chat-text-primary', '--chat-text-secondary',
        '--custom-chat-ic-bg', '--custom-chat-ooc-bg', '--custom-chat-stage-bg', '--custom-chat-preview-bg', '--custom-chat-preview-dot',
        '--sc-border-mute', '--sc-border-strong',
        '--primary-color', '--primary-color-hover'
      ]
      // Clear previous custom colors first
      customColorVars.forEach(v => removeVar(v))

      if (effective.customThemeEnabled && effective.activeCustomThemeId) {
        const activeTheme = effective.customThemes.find(t => t.id === effective.activeCustomThemeId)
        if (activeTheme?.colors) {
          const c = activeTheme.colors
          if (c.bgSurface) setVar('--sc-bg-surface', c.bgSurface)
          if (c.bgElevated) setVar('--sc-bg-elevated', c.bgElevated)
          if (c.bgInput) setVar('--sc-bg-input', c.bgInput)
          if (c.bgHeader) setVar('--sc-bg-header', c.bgHeader)
          if (c.textPrimary) {
            setVar('--sc-text-primary', c.textPrimary)
            // Also set --chat-text-primary for chat message content area
            setVar('--chat-text-primary', c.textPrimary)
          }
          if (c.textSecondary) {
            setVar('--sc-text-secondary', c.textSecondary)
            // Also set --chat-text-secondary for chat message content area
            setVar('--chat-text-secondary', c.textSecondary)
          }
          // Use --custom-* prefix for chat colors so they can override scoped class variables
          if (c.chatIcBg) setVar('--custom-chat-ic-bg', c.chatIcBg)
          if (c.chatOocBg) setVar('--custom-chat-ooc-bg', c.chatOocBg)
          const stageBg = c.chatStageBg || c.chatIcBg
          if (stageBg) setVar('--custom-chat-stage-bg', stageBg)
          if (c.chatPreviewBg) setVar('--custom-chat-preview-bg', c.chatPreviewBg)
          if (c.chatPreviewDot) setVar('--custom-chat-preview-dot', c.chatPreviewDot)
          if (c.borderMute) setVar('--sc-border-mute', c.borderMute)
          if (c.borderStrong) setVar('--sc-border-strong', c.borderStrong)
          if (c.primaryColor) setVar('--primary-color', c.primaryColor)
          if (c.primaryColorHover) setVar('--primary-color-hover', c.primaryColorHover)
          // Keyword highlight colors
          if (c.keywordBg) setVar('--custom-keyword-bg', c.keywordBg)
          if (c.keywordBorder) setVar('--custom-keyword-border', c.keywordBorder)
          // Mark custom theme active for CSS selectors
          root.dataset.customTheme = 'true'
        }
      } else {
        delete root.dataset.customTheme
      }
    },
    // Custom theme management
    getActiveCustomTheme(): CustomTheme | null {
      if (!this.settings.customThemeEnabled || !this.settings.activeCustomThemeId) return null
      return this.settings.customThemes.find(t => t.id === this.settings.activeCustomThemeId) || null
    },
    saveCustomTheme(theme: CustomTheme) {
      const normalized = normalizeCustomTheme(theme)
      if (!normalized) return
      const existingIndex = this.settings.customThemes.findIndex(t => t.id === normalized.id)
      if (existingIndex >= 0) {
        normalized.updatedAt = Date.now()
        this.settings.customThemes = [
          ...this.settings.customThemes.slice(0, existingIndex),
          normalized,
          ...this.settings.customThemes.slice(existingIndex + 1),
        ]
      } else {
        normalized.createdAt = Date.now()
        normalized.updatedAt = Date.now()
        this.settings.customThemes = [...this.settings.customThemes, normalized]
      }
      this.persist()
      this.applyTheme()
    },
    deleteCustomTheme(id: string) {
      const index = this.settings.customThemes.findIndex(t => t.id === id)
      if (index < 0) return
      this.settings.customThemes = [
        ...this.settings.customThemes.slice(0, index),
        ...this.settings.customThemes.slice(index + 1),
      ]
      if (this.settings.activeCustomThemeId === id) {
        this.settings.activeCustomThemeId = this.settings.customThemes[0]?.id || null
      }
      this.persist()
      this.applyTheme()
    },
    activateCustomTheme(id: string | null) {
      this.settings.activeCustomThemeId = id
      this.persist()
      this.applyTheme()
    },
    setCustomThemeEnabled(enabled: boolean) {
      this.settings.customThemeEnabled = enabled
      this.persist()
      this.applyTheme()
    },
  },
})
