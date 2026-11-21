// 频道角色聊天装扮（Chat Skin）数据契约。
//
// 本文件是装扮 JSON 的唯一前端权威：频道角色、角色差分、消息快照、导入导出与预设
// 全部使用同一份结构，并在任何渲染 / 保存前经过 normalizeChatSkin 规范化。
// 规范化同时是安全边界：颜色、枚举、数值、资源引用与高级 CSS 均在此处白名单化，
// 渲染层只消费规范化结果，绝不直接把原始 JSON 写进 DOM。
//
// 语义约定：
// - 任何「未出现」的字段都表示继承当前主题，不存在固定的白底默认值；
// - 一个样式分组（如 border）一旦出现，组内字段补齐为完整值；
// - 气泡（bubble）与紧凑（compact）是两份独立表面，互不回退。

export const CHAT_SKIN_VERSION = 1
export const CHAT_SKIN_MAX_LAYERS = 8
export const CHAT_SKIN_MAX_ADVANCED_CSS = 600
export const CHAT_SKIN_MAX_VARS = 24
export const CHAT_SKIN_MAX_LAYER_AXIS = 480

export const CHAT_SKIN_LAYOUTS = ['bubble', 'compact'] as const
export type ChatSkinLayout = (typeof CHAT_SKIN_LAYOUTS)[number]

export type ChatSkinScope = 'all' | 'ic'
export const CHAT_SKIN_ANCHORS = [
  'top-left', 'top', 'top-right',
  'left', 'center', 'right',
  'bottom-left', 'bottom', 'bottom-right',
] as const
export type ChatSkinAnchor = (typeof CHAT_SKIN_ANCHORS)[number]
export const CHAT_SKIN_FONTS = ['inherit', 'sans', 'serif', 'kai', 'mono', 'rounded'] as const
export type ChatSkinFont = (typeof CHAT_SKIN_FONTS)[number]
export const CHAT_SKIN_BORDER_STYLES = ['solid', 'dashed', 'dotted', 'double'] as const
export type ChatSkinBorderStyle = (typeof CHAT_SKIN_BORDER_STYLES)[number]
export const CHAT_SKIN_IMAGE_FITS = ['cover', 'contain', 'tile', 'stretch'] as const
export type ChatSkinImageFit = (typeof CHAT_SKIN_IMAGE_FITS)[number]
export const CHAT_SKIN_IMAGE_POSITIONS = ['center', 'top', 'bottom', 'left', 'right'] as const
export type ChatSkinImagePosition = (typeof CHAT_SKIN_IMAGE_POSITIONS)[number]
export const CHAT_SKIN_BLENDS = ['normal', 'multiply', 'screen', 'overlay', 'soft-light'] as const
export type ChatSkinBlend = (typeof CHAT_SKIN_BLENDS)[number]
export const CHAT_SKIN_ACCENT_SIDES = ['left', 'right', 'top', 'bottom'] as const
export type ChatSkinAccentSide = (typeof CHAT_SKIN_ACCENT_SIDES)[number]
export const CHAT_SKIN_BACKGROUND_MODES = ['color', 'gradient', 'none'] as const
export type ChatSkinBackgroundMode = (typeof CHAT_SKIN_BACKGROUND_MODES)[number]
export const CHAT_SKIN_NAME_WEIGHTS = ['normal', 'bold', 'black'] as const
export type ChatSkinNameWeight = (typeof CHAT_SKIN_NAME_WEIGHTS)[number]

export interface ChatSkinBackground {
  mode: ChatSkinBackgroundMode
  color: string
  color2: string
  angle: number
  imageRef: string
  imageFit: ChatSkinImageFit
  imagePosition: ChatSkinImagePosition
}

export interface ChatSkinBorder {
  width: number
  style: ChatSkinBorderStyle
  color: string
}

export interface ChatSkinRadius {
  tl: number
  tr: number
  br: number
  bl: number
}

export interface ChatSkinShadow {
  mode: 'none' | 'custom'
  color: string
  blur: number
  x: number
  y: number
  spread: number
}

export interface ChatSkinPadding {
  x: number
  y: number
}

export interface ChatSkinSize {
  minWidth: number
  minHeight: number
  /** 0 表示沿用主题最大宽度。 */
  maxWidth: number
}

export interface ChatSkinSpacing {
  top: number
  bottom: number
}

export interface ChatSkinAccent {
  color: string
  width: number
  side: ChatSkinAccentSide
}

export interface ChatSkinText {
  color?: string
  font?: ChatSkinFont
  scale?: number
  shadow?: string
}

export interface ChatSkinName {
  color?: string
  font?: ChatSkinFont
  weight?: ChatSkinNameWeight
  italic?: boolean
  chip?: string
  shadow?: string
}

export interface ChatSkinStyle {
  background?: ChatSkinBackground
  border?: ChatSkinBorder
  radius?: ChatSkinRadius
  shadow?: ChatSkinShadow
  padding?: ChatSkinPadding
  size?: ChatSkinSize
  spacing?: ChatSkinSpacing
  accent?: ChatSkinAccent
  blur?: number
  text?: ChatSkinText
  name?: ChatSkinName
}

export type ChatSkinStyleKey = keyof ChatSkinStyle

export interface ChatSkinLayer {
  id: string
  label: string
  enabled: boolean
  /** `id:<附件ID>` 引用附件资源，`builtin:<名称>` 引用内置纹样。 */
  ref: string
  /** 仅内置纹样可着色；空字符串表示使用纹样原色。 */
  tint: string
  anchor: ChatSkinAnchor
  x: number
  y: number
  size: number
  /** 高宽比 height / width，用于在图片加载前预留空间。 */
  aspect: number
  rotate: number
  opacity: number
  z: 'front' | 'back'
  flipX: boolean
  flipY: boolean
  /** 自己的气泡位于右侧时是否随气泡水平镜像。 */
  mirror: boolean
  blend: ChatSkinBlend
}

export interface ChatSkinAdvanced {
  vars: Record<string, string>
  surfaceCss: string
  nameCss: string
  textCss: string
}

export interface ChatSkinSurface {
  enabled: boolean
  style: ChatSkinStyle
  /** 夜间配色覆写；按样式分组整体覆盖 style。 */
  night: ChatSkinStyle | null
  layers: ChatSkinLayer[]
  mirrorOnSelf: boolean
  reserveSpace: boolean
  advanced: ChatSkinAdvanced | null
}

export interface ChatSkinMeta {
  name: string
  presetId: string
}

export interface ChatSkin {
  version: typeof CHAT_SKIN_VERSION
  scope: ChatSkinScope
  bubble: ChatSkinSurface | null
  compact: ChatSkinSurface | null
  meta: ChatSkinMeta
}

/** 角色差分对基础装扮的局部覆写。 */
export interface ChatSkinPatch {
  /**
   * merge：在基础装扮上深合并（对象递归合并、数组整体替换、null 删除字段）；
   * replace：完全替换为 skin；none：此差分不显示任何聊天装扮。
   */
  mode: 'merge' | 'replace' | 'none'
  skin: Record<string, unknown> | null
}

// ---------------------------------------------------------------------------
// 基础校验工具
// ---------------------------------------------------------------------------

type UnknownRecord = Record<string, unknown>

export const isPlainRecord = (value: unknown): value is UnknownRecord => (
  !!value && typeof value === 'object' && !Array.isArray(value)
)

const clampNumber = (value: unknown, min: number, max: number, fallback: number, precision = 2) => {
  const num = typeof value === 'number' ? value : typeof value === 'string' && value.trim() ? Number(value) : NaN
  if (!Number.isFinite(num)) return fallback
  const factor = 10 ** precision
  return Math.min(max, Math.max(min, Math.round(num * factor) / factor))
}

const pickEnum = <T extends string>(value: unknown, allowed: readonly T[], fallback: T): T => (
  typeof value === 'string' && (allowed as readonly string[]).includes(value) ? value as T : fallback
)

const HEX_COLOR = /^#([0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$/i

/** 只接受十六进制颜色与 transparent，统一为小写 #rrggbb / #rrggbbaa。 */
export const normalizeChatSkinColor = (value: unknown): string => {
  if (typeof value !== 'string') return ''
  const raw = value.trim().toLowerCase()
  if (raw === 'transparent') return 'transparent'
  if (!HEX_COLOR.test(raw)) return ''
  const hex = raw.slice(1)
  if (hex.length === 3 || hex.length === 4) {
    return `#${hex.split('').map((char) => char + char).join('')}`.replace(/^#([0-9a-f]{6})ff$/, '#$1')
  }
  return `#${hex}`.replace(/^#([0-9a-f]{6})ff$/, '#$1')
}

const ATTACHMENT_REF = /^id:[A-Za-z0-9_-]{1,96}$/
const BUILTIN_REF = /^builtin:[a-z0-9-]{1,40}$/

/** 资源引用只允许附件引用或内置纹样，任何 URL / data URL 都会被丢弃。 */
export const normalizeChatSkinRef = (value: unknown): string => {
  if (typeof value !== 'string') return ''
  const raw = value.trim()
  if (BUILTIN_REF.test(raw)) return raw
  if (ATTACHMENT_REF.test(raw)) return raw
  // 兼容裸附件 ID。
  if (/^[A-Za-z0-9_-]{6,96}$/.test(raw)) return `id:${raw}`
  return ''
}

export const isChatSkinAttachmentRef = (ref: string) => ATTACHMENT_REF.test(ref)

const normalizeText = (value: unknown, max: number) => (
  typeof value === 'string' ? value.replace(/[\u0000-\u001f<>]/g, '').trim().slice(0, max) : ''
)

// ---------------------------------------------------------------------------
// 受控高级 CSS
// ---------------------------------------------------------------------------

/** 高级 CSS 只能写声明，属性必须在白名单内；不能定位、变换或加载外部资源。 */
export const CHAT_SKIN_ADVANCED_CSS_PROPERTIES = new Set([
  'background', 'background-color', 'background-image', 'background-size', 'background-position',
  'background-repeat', 'background-blend-mode',
  'border', 'border-color', 'border-style', 'border-width', 'border-radius',
  'border-top', 'border-right', 'border-bottom', 'border-left',
  'box-shadow', 'outline', 'outline-offset', 'filter', 'backdrop-filter', 'opacity', 'mix-blend-mode',
  'color', 'text-shadow', 'letter-spacing', 'word-spacing', 'line-height', 'font-style',
  'font-weight', 'font-variant', 'text-decoration', 'text-decoration-color', 'text-decoration-style',
  'text-transform', 'text-align', 'padding', 'padding-top', 'padding-right', 'padding-bottom',
  'padding-left', 'min-height', 'min-width',
])

const FORBIDDEN_CSS_VALUE = /url\s*\(|expression\s*\(|javascript:|@import|[<>{}\\]|\/\*|image-set|image\s*\(|-moz-binding|behavior\s*:/i

const isSafeCssValue = (value: string) => value.length > 0 && value.length <= 200 && !FORBIDDEN_CSS_VALUE.test(value)

/** 解析 `prop: value; ...`，只保留白名单属性，返回规范化后的声明串。 */
export const sanitizeChatSkinCss = (value: unknown): string => {
  if (typeof value !== 'string') return ''
  const declarations: string[] = []
  for (const chunk of value.slice(0, CHAT_SKIN_MAX_ADVANCED_CSS * 2).split(';')) {
    const index = chunk.indexOf(':')
    if (index <= 0) continue
    const property = chunk.slice(0, index).trim().toLowerCase()
    const declValue = chunk.slice(index + 1).replace(/!important/gi, '').trim()
    if (!CHAT_SKIN_ADVANCED_CSS_PROPERTIES.has(property) || !isSafeCssValue(declValue)) continue
    declarations.push(`${property}: ${declValue}`)
  }
  let result = ''
  for (const decl of declarations) {
    const next = result ? `${result}; ${decl}` : decl
    if (next.length > CHAT_SKIN_MAX_ADVANCED_CSS) break
    result = next
  }
  return result
}

/** 高级变量只能覆写聊天作用域内的变量，变量作用于单条消息。 */
export const CHAT_SKIN_VAR_NAME = /^--(chat|quote|message|sc-skin)-[a-z0-9-]{1,48}$/

export const sanitizeChatSkinVars = (value: unknown): Record<string, string> => {
  if (!isPlainRecord(value)) return {}
  const result: Record<string, string> = {}
  let count = 0
  for (const [rawKey, rawValue] of Object.entries(value)) {
    if (count >= CHAT_SKIN_MAX_VARS) break
    const key = rawKey.trim().toLowerCase()
    if (!CHAT_SKIN_VAR_NAME.test(key) || typeof rawValue !== 'string') continue
    const cssValue = rawValue.replace(/!important/gi, '').trim()
    if (!isSafeCssValue(cssValue) || cssValue.includes(';')) continue
    result[key] = cssValue
    count += 1
  }
  return result
}

/** 将 `prop: value` 串拆成 Vue 内联样式对象（输入已经过 sanitizeChatSkinCss）。 */
export const parseChatSkinCssDeclarations = (css: string): Record<string, string> => {
  const result: Record<string, string> = {}
  for (const chunk of sanitizeChatSkinCss(css).split(';')) {
    const index = chunk.indexOf(':')
    if (index <= 0) continue
    const property = chunk.slice(0, index).trim()
    result[property] = constrainChatSkinCssGeometry(property, chunk.slice(index + 1).trim())
  }
  return result
}

/** 只约束会参与消息布局的长度；阴影、滤镜等视觉声明保持原样。 */
export const constrainChatSkinCssGeometry = (property: string, value: string): string => {
  if (property === 'min-width') return `min(100%, 480px, max(0px, ${value}))`
  if (property === 'min-height') return `clamp(0px, ${value}, 240px)`
  if (property === 'line-height' && value !== 'normal') {
    const length = /^[\d.]+$/.test(value) ? `${value}em` : value
    return `clamp(0px, ${length}, 96px)`
  }
  if (property === 'padding' || property.startsWith('padding-') || property === 'border-width') {
    // 空格只分隔顶层简写值，保留 calc()/var() 内部的空格。
    const parts = value.match(/(?:[^\s()]+|\((?:[^()]|\([^()]*\))*\))+/g) || []
    const limit = 48
    return parts.map((part) => `clamp(0px, ${part}, ${limit}px)`).join(' ')
  }
  if (/^border(?:-(?:top|right|bottom|left))?$/.test(property)) {
    return value.replace(/(?:\d*\.)?\d+(?:px|em|rem|vw|vh|vmin|vmax)\b/gi, (length) => `clamp(0px, ${length}, 48px)`)
  }
  return value
}

// ---------------------------------------------------------------------------
// 规范化
// ---------------------------------------------------------------------------

export const createDefaultChatSkinBackground = (): ChatSkinBackground => ({
  mode: 'color',
  color: '#ffffff',
  color2: '#f1f5f9',
  angle: 135,
  imageRef: '',
  imageFit: 'cover',
  imagePosition: 'center',
})

const normalizeBackground = (raw: unknown): ChatSkinBackground | undefined => {
  if (!isPlainRecord(raw)) return undefined
  const base = createDefaultChatSkinBackground()
  const mode = pickEnum(raw.mode, CHAT_SKIN_BACKGROUND_MODES, base.mode)
  return {
    mode,
    color: normalizeChatSkinColor(raw.color) || base.color,
    color2: normalizeChatSkinColor(raw.color2) || base.color2,
    angle: clampNumber(raw.angle, 0, 360, base.angle, 0),
    imageRef: normalizeChatSkinRef(raw.imageRef),
    imageFit: pickEnum(raw.imageFit, CHAT_SKIN_IMAGE_FITS, base.imageFit),
    imagePosition: pickEnum(raw.imagePosition, CHAT_SKIN_IMAGE_POSITIONS, base.imagePosition),
  }
}

const normalizeBorder = (raw: unknown): ChatSkinBorder | undefined => {
  if (!isPlainRecord(raw)) return undefined
  return {
    width: clampNumber(raw.width, 0, 8, 1, 1),
    style: pickEnum(raw.style, CHAT_SKIN_BORDER_STYLES, 'solid'),
    color: normalizeChatSkinColor(raw.color) || '#94a3b8',
  }
}

const normalizeRadius = (raw: unknown): ChatSkinRadius | undefined => {
  if (typeof raw === 'number') {
    const value = clampNumber(raw, 0, 48, 12, 0)
    return { tl: value, tr: value, br: value, bl: value }
  }
  if (!isPlainRecord(raw)) return undefined
  return {
    tl: clampNumber(raw.tl, 0, 48, 12, 0),
    tr: clampNumber(raw.tr, 0, 48, 12, 0),
    br: clampNumber(raw.br, 0, 48, 12, 0),
    bl: clampNumber(raw.bl, 0, 48, 12, 0),
  }
}

const normalizeShadow = (raw: unknown): ChatSkinShadow | undefined => {
  if (!isPlainRecord(raw)) return undefined
  return {
    mode: raw.mode === 'none' ? 'none' : 'custom',
    color: normalizeChatSkinColor(raw.color) || '#0f172a29',
    blur: clampNumber(raw.blur, 0, 48, 12, 0),
    x: clampNumber(raw.x, -24, 24, 0, 0),
    y: clampNumber(raw.y, -24, 24, 4, 0),
    spread: clampNumber(raw.spread, -12, 12, 0, 0),
  }
}

const normalizePadding = (raw: unknown): ChatSkinPadding | undefined => {
  if (!isPlainRecord(raw)) return undefined
  return {
    x: clampNumber(raw.x, 0, 48, 14, 0),
    y: clampNumber(raw.y, 0, 40, 10, 0),
  }
}

const normalizeSize = (raw: unknown): ChatSkinSize | undefined => {
  if (!isPlainRecord(raw)) return undefined
  return {
    minWidth: clampNumber(raw.minWidth, 0, 480, 0, 0),
    minHeight: clampNumber(raw.minHeight, 0, 240, 0, 0),
    maxWidth: clampNumber(raw.maxWidth, 0, 960, 0, 0),
  }
}

const normalizeSpacing = (raw: unknown): ChatSkinSpacing | undefined => {
  if (!isPlainRecord(raw)) return undefined
  return {
    top: clampNumber(raw.top, 0, 64, 0, 0),
    bottom: clampNumber(raw.bottom, 0, 64, 0, 0),
  }
}

const normalizeAccent = (raw: unknown): ChatSkinAccent | undefined => {
  if (!isPlainRecord(raw)) return undefined
  return {
    color: normalizeChatSkinColor(raw.color) || '#6366f1',
    width: clampNumber(raw.width, 1, 12, 3, 0),
    side: pickEnum(raw.side, CHAT_SKIN_ACCENT_SIDES, 'left'),
  }
}

const normalizeTextStyle = (raw: unknown): ChatSkinText | undefined => {
  if (!isPlainRecord(raw)) return undefined
  const result: ChatSkinText = {}
  const color = normalizeChatSkinColor(raw.color)
  if (color) result.color = color
  if (typeof raw.font === 'string' && (CHAT_SKIN_FONTS as readonly string[]).includes(raw.font) && raw.font !== 'inherit') {
    result.font = raw.font as ChatSkinFont
  }
  if (raw.scale !== undefined && raw.scale !== null) {
    const scale = clampNumber(raw.scale, 0.8, 1.4, 1, 2)
    if (scale !== 1) result.scale = scale
  }
  const shadow = normalizeChatSkinColor(raw.shadow)
  if (shadow) result.shadow = shadow
  return Object.keys(result).length ? result : undefined
}

const normalizeNameStyle = (raw: unknown): ChatSkinName | undefined => {
  if (!isPlainRecord(raw)) return undefined
  const result: ChatSkinName = {}
  const color = normalizeChatSkinColor(raw.color)
  if (color) result.color = color
  if (typeof raw.font === 'string' && (CHAT_SKIN_FONTS as readonly string[]).includes(raw.font) && raw.font !== 'inherit') {
    result.font = raw.font as ChatSkinFont
  }
  if (typeof raw.weight === 'string' && (CHAT_SKIN_NAME_WEIGHTS as readonly string[]).includes(raw.weight)) {
    result.weight = raw.weight as ChatSkinNameWeight
  }
  if (raw.italic === true) result.italic = true
  const chip = normalizeChatSkinColor(raw.chip)
  if (chip) result.chip = chip
  const shadow = normalizeChatSkinColor(raw.shadow)
  if (shadow) result.shadow = shadow
  return Object.keys(result).length ? result : undefined
}

export const normalizeChatSkinStyle = (raw: unknown): ChatSkinStyle => {
  if (!isPlainRecord(raw)) return {}
  const style: ChatSkinStyle = {}
  const background = normalizeBackground(raw.background)
  if (background) style.background = background
  const border = normalizeBorder(raw.border)
  if (border) style.border = border
  const radius = normalizeRadius(raw.radius)
  if (radius) style.radius = radius
  const shadow = normalizeShadow(raw.shadow)
  if (shadow) style.shadow = shadow
  const padding = normalizePadding(raw.padding)
  if (padding) style.padding = padding
  const size = normalizeSize(raw.size)
  if (size) style.size = size
  const spacing = normalizeSpacing(raw.spacing)
  if (spacing) style.spacing = spacing
  const accent = normalizeAccent(raw.accent)
  if (accent) style.accent = accent
  if (raw.blur !== undefined && raw.blur !== null) {
    const blur = clampNumber(raw.blur, 0, 20, 0, 0)
    if (blur > 0) style.blur = blur
  }
  const text = normalizeTextStyle(raw.text)
  if (text) style.text = text
  const name = normalizeNameStyle(raw.name)
  if (name) style.name = name
  return style
}

let layerSeed = 0
export const createChatSkinLayerId = () => {
  layerSeed = (layerSeed + 1) % 1_000_000
  return `layer-${Date.now().toString(36)}-${layerSeed.toString(36)}`
}

export const createDefaultChatSkinLayer = (ref = '', overrides: Partial<ChatSkinLayer> = {}): ChatSkinLayer => ({
  id: createChatSkinLayerId(),
  label: '',
  enabled: true,
  ref,
  tint: '',
  anchor: 'top-left',
  x: 0,
  y: 0,
  size: 40,
  aspect: 1,
  rotate: 0,
  opacity: 1,
  z: 'front',
  flipX: false,
  flipY: false,
  mirror: true,
  blend: 'normal',
  ...overrides,
})

export const normalizeChatSkinLayer = (raw: unknown, index = 0): ChatSkinLayer | null => {
  if (!isPlainRecord(raw)) return null
  const ref = normalizeChatSkinRef(raw.ref)
  if (!ref) return null
  const id = typeof raw.id === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(raw.id) ? raw.id : `layer-${index + 1}`
  const aspect = clampNumber(raw.aspect, 0.05, 20, 1, 3)
  const size = Math.min(clampNumber(raw.size, 8, 480, 40, 3), Math.floor(CHAT_SKIN_MAX_LAYER_AXIS / aspect * 1000) / 1000)
  return {
    id,
    label: normalizeText(raw.label, 32),
    enabled: raw.enabled !== false,
    ref,
    tint: ref.startsWith('builtin:') ? normalizeChatSkinColor(raw.tint) : '',
    anchor: pickEnum(raw.anchor, CHAT_SKIN_ANCHORS, 'top-left'),
    x: clampNumber(raw.x, -240, 240, 0, 0),
    y: clampNumber(raw.y, -240, 240, 0, 0),
    size,
    aspect,
    rotate: clampNumber(raw.rotate, -180, 180, 0, 0),
    opacity: clampNumber(raw.opacity, 0, 1, 1, 2),
    z: raw.z === 'back' ? 'back' : 'front',
    flipX: raw.flipX === true,
    flipY: raw.flipY === true,
    mirror: raw.mirror !== false,
    blend: pickEnum(raw.blend, CHAT_SKIN_BLENDS, 'normal'),
  }
}

const normalizeAdvanced = (raw: unknown): ChatSkinAdvanced | null => {
  if (!isPlainRecord(raw)) return null
  const advanced: ChatSkinAdvanced = {
    vars: sanitizeChatSkinVars(raw.vars),
    surfaceCss: sanitizeChatSkinCss(raw.surfaceCss),
    nameCss: sanitizeChatSkinCss(raw.nameCss),
    textCss: sanitizeChatSkinCss(raw.textCss),
  }
  if (!Object.keys(advanced.vars).length && !advanced.surfaceCss && !advanced.nameCss && !advanced.textCss) {
    return null
  }
  return advanced
}

export const createEmptyChatSkinSurface = (): ChatSkinSurface => ({
  enabled: true,
  style: {},
  night: null,
  layers: [],
  mirrorOnSelf: true,
  reserveSpace: true,
  advanced: null,
})

export const normalizeChatSkinSurface = (raw: unknown): ChatSkinSurface | null => {
  if (!isPlainRecord(raw)) return null
  const layers: ChatSkinLayer[] = []
  const seen = new Set<string>()
  if (Array.isArray(raw.layers)) {
    raw.layers.forEach((item, index) => {
      if (layers.length >= CHAT_SKIN_MAX_LAYERS) return
      const layer = normalizeChatSkinLayer(item, index)
      if (!layer) return
      if (seen.has(layer.id)) layer.id = `${layer.id}-${index}`
      seen.add(layer.id)
      layers.push(layer)
    })
  }
  const night = isPlainRecord(raw.night) ? normalizeChatSkinStyle(raw.night) : null
  return {
    enabled: raw.enabled !== false,
    style: normalizeChatSkinStyle(raw.style),
    night: night && Object.keys(night).length ? night : null,
    layers,
    mirrorOnSelf: raw.mirrorOnSelf !== false,
    reserveSpace: raw.reserveSpace !== false,
    advanced: normalizeAdvanced(raw.advanced),
  }
}

export const isChatSkinSurfaceEffective = (surface: ChatSkinSurface | null | undefined) => (
  !!surface
  && surface.enabled
  && (
    Object.keys(surface.style).length > 0
    || !!surface.night
    || surface.layers.some((layer) => layer.enabled)
    || !!surface.advanced
  )
)

export const createEmptyChatSkin = (): ChatSkin => ({
  version: CHAT_SKIN_VERSION,
  scope: 'ic',
  bubble: null,
  compact: null,
  meta: { name: '', presetId: '' },
})

/**
 * 规范化任意来源（服务端、消息快照、导入文件、预设）的装扮。
 * 没有任何有效表面时返回 null，调用方据此保持「无装扮」的旧行为。
 */
export const normalizeChatSkin = (raw: unknown): ChatSkin | null => {
  if (!isPlainRecord(raw)) return null
  const bubble = normalizeChatSkinSurface(raw.bubble)
  const compact = normalizeChatSkinSurface(raw.compact)
  if (!bubble && !compact) return null
  const meta = isPlainRecord(raw.meta) ? raw.meta : {}
  return {
    version: CHAT_SKIN_VERSION,
    scope: raw.scope === 'all' ? 'all' : 'ic',
    bubble,
    compact,
    meta: {
      name: normalizeText(meta.name, 40),
      presetId: typeof meta.presetId === 'string' && /^[a-z0-9:_-]{1,64}$/i.test(meta.presetId) ? meta.presetId : '',
    },
  }
}

export const isChatSkinEffective = (skin: ChatSkin | null | undefined) => (
  !!skin && (isChatSkinSurfaceEffective(skin.bubble) || isChatSkinSurfaceEffective(skin.compact))
)

/** 存在配置与当前可见是两件事；关闭布局或全部图层仍需保存。 */
export const hasChatSkinConfiguration = (skin: ChatSkin | null | undefined) => (
  !!skin && (!!skin.bubble || !!skin.compact)
)

export type ChatSkinVariantMode = 'inherit' | 'merge' | 'replace' | 'none'

export const buildChatSkinStudioResult = (mode: 'base' | 'variant', variantMode: ChatSkinVariantMode, base: ChatSkin | null, draft: unknown): { skin?: ChatSkin | null; patch?: ChatSkinPatch | null } => {
  const skin = normalizeChatSkin(draft)
  if (mode === 'base') return { skin }
  if (variantMode === 'inherit') return { patch: null }
  if (variantMode === 'none') return { patch: { mode: 'none', skin: null } }
  if (variantMode === 'replace') return { patch: skin ? { mode: 'replace', skin: skin as unknown as UnknownRecord } : { mode: 'none', skin: null } }
  return { patch: diffChatSkinAgainstBase(base, skin) }
}

export const cloneChatSkin = (skin: ChatSkin | null | undefined): ChatSkin | null => (
  skin ? normalizeChatSkin(JSON.parse(JSON.stringify(skin))) : null
)

// ---------------------------------------------------------------------------
// 角色差分覆写
// ---------------------------------------------------------------------------

export const normalizeChatSkinPatch = (raw: unknown): ChatSkinPatch | null => {
  if (!isPlainRecord(raw)) return null
  const mode = raw.mode === 'replace' || raw.mode === 'none' ? raw.mode : 'merge'
  if (mode === 'none') return { mode, skin: null }
  if (!isPlainRecord(raw.skin)) return mode === 'replace' ? { mode: 'none', skin: null } : null
  return { mode, skin: JSON.parse(JSON.stringify(raw.skin)) as Record<string, unknown> }
}

/** 与服务端 protocol.MergeChatSkinDocument 保持同一语义：对象递归合并、数组整体替换、null 删除。 */
export const deepMergeChatSkinDocument = (base: unknown, patch: unknown): unknown => {
  if (!isPlainRecord(patch)) return patch === undefined ? base : patch
  const result: UnknownRecord = isPlainRecord(base) ? { ...base } : {}
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) {
      delete result[key]
      continue
    }
    if (isPlainRecord(value)) {
      result[key] = deepMergeChatSkinDocument(result[key], value)
      continue
    }
    result[key] = Array.isArray(value) ? JSON.parse(JSON.stringify(value)) : value
  }
  return result
}

/** 基础装扮 + 差分覆写 → 最终装扮；与发送时写入消息快照的结果一致。 */
export const resolveChatSkinWithPatch = (
  base: unknown,
  patch: ChatSkinPatch | null | undefined,
): ChatSkin | null => {
  const normalizedBase = normalizeChatSkin(base)
  if (!patch) return normalizedBase
  if (patch.mode === 'none') return null
  if (patch.mode === 'replace') return normalizeChatSkin(patch.skin)
  if (!patch.skin) return normalizedBase
  return normalizeChatSkin(deepMergeChatSkinDocument(normalizedBase ?? {}, patch.skin))
}

/** 编辑器在差分模式下生成的覆写：只记录与基础装扮不同的分组。 */
export const diffChatSkinAgainstBase = (base: ChatSkin | null, target: ChatSkin | null): ChatSkinPatch | null => {
  if (!target) return base ? { mode: 'none', skin: null } : null
  if (!base) return { mode: 'replace', skin: JSON.parse(JSON.stringify(target)) }
  const diff = diffRecords(base as unknown as UnknownRecord, target as unknown as UnknownRecord)
  if (!diff || !Object.keys(diff).length) return null
  return { mode: 'merge', skin: diff }
}

const diffRecords = (base: UnknownRecord, target: UnknownRecord): UnknownRecord | null => {
  const result: UnknownRecord = {}
  const keys = new Set([...Object.keys(base), ...Object.keys(target)])
  for (const key of keys) {
    const left = base[key]
    const right = target[key]
    if (right === undefined) {
      if (left !== undefined) result[key] = null
      continue
    }
    if (isPlainRecord(left) && isPlainRecord(right)) {
      const nested = diffRecords(left, right)
      if (nested && Object.keys(nested).length) result[key] = nested
      continue
    }
    if (JSON.stringify(left) !== JSON.stringify(right)) {
      result[key] = right === null ? null : JSON.parse(JSON.stringify(right))
    }
  }
  return result
}

// ---------------------------------------------------------------------------
// 资源引用
// ---------------------------------------------------------------------------

/** 收集装扮中所有附件引用（不含内置纹样），用于导出与权限校验。 */
export const collectChatSkinAttachmentRefs = (skin: ChatSkin | null | undefined): string[] => {
  if (!skin) return []
  const refs = new Set<string>()
  const visitStyle = (style: ChatSkinStyle | null | undefined) => {
    const ref = style?.background?.imageRef || ''
    if (isChatSkinAttachmentRef(ref)) refs.add(ref)
  }
  for (const surface of [skin.bubble, skin.compact]) {
    if (!surface) continue
    visitStyle(surface.style)
    visitStyle(surface.night)
    surface.layers.forEach((layer) => {
      if (isChatSkinAttachmentRef(layer.ref)) refs.add(layer.ref)
    })
  }
  return [...refs]
}

/** 将装扮内的资源引用按映射替换；映射不到的附件引用会被移除（图层删除、背景图清空）。 */
export const mapChatSkinRefs = (
  skin: ChatSkin | null | undefined,
  mapRef: (ref: string) => string,
): ChatSkin | null => {
  const next = cloneChatSkin(skin)
  if (!next) return null
  const mapStyle = (style: ChatSkinStyle | null | undefined) => {
    if (style?.background?.imageRef && !style.background.imageRef.startsWith('builtin:')) {
      style.background.imageRef = mapRef(style.background.imageRef) || ''
    }
  }
  for (const surface of [next.bubble, next.compact]) {
    if (!surface) continue
    mapStyle(surface.style)
    mapStyle(surface.night)
    surface.layers = surface.layers
      .map((layer) => (layer.ref.startsWith('builtin:') ? layer : { ...layer, ref: mapRef(layer.ref) || '' }))
      .filter((layer) => !!layer.ref)
  }
  return next
}

/** 读取消息快照中的装扮（兼容 identity.chatSkin 与 sender_chat_skin 两种载荷）。 */
export const resolveMessageChatSkinSnapshot = (message: unknown): ChatSkin | null => {
  if (!isPlainRecord(message)) return null
  const identity = isPlainRecord(message.identity) ? message.identity : null
  const raw = identity && identity.chatSkin !== undefined ? identity.chatSkin : message.sender_chat_skin
  return normalizeChatSkin(raw)
}

/** undefined 沿用快照；null 明确清除。调用方负责选择当前消息的编辑预览。 */
export const resolvePreviewChatSkin = (message: unknown, previewSkin: unknown, enabled = true): ChatSkin | null => (
  enabled ? (previewSkin === undefined ? resolveMessageChatSkinSnapshot(message) : normalizeChatSkin(previewSkin)) : null
)

/** 读取差分上的装扮覆写（优先顶层 chatSkin，兼容 appearance.chatSkin）。 */
export const resolveVariantChatSkinPatch = (variant: unknown): ChatSkinPatch | null => {
  if (!isPlainRecord(variant)) return null
  if (variant.chatSkin !== undefined) return normalizeChatSkinPatch(variant.chatSkin)
  const appearance = isPlainRecord(variant.appearance) ? variant.appearance : null
  return appearance ? normalizeChatSkinPatch(appearance.chatSkin) : null
}
