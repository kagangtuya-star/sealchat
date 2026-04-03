type RGB = { r: number; g: number; b: number }
type RGBA = RGB & { a: number }

export type IdentityMetaKind = 'remark' | 'badge'
export type IdentityMetaStyleMode = 'disabled' | 'normal' | 'adjusted' | 'fallback'

export interface ResolveIdentityMetaStyleOptions {
  enabled: boolean
  kind: IdentityMetaKind
  identityColor?: string | null
  backgroundColor?: string | null
}

export interface ResolveIdentityMetaStyleResult {
  mode: IdentityMetaStyleMode
  contrastRatio: number
  style: Record<string, string>
}

const DAY_BACKGROUND: RGB = { r: 248, g: 250, b: 252 }
const NIGHT_BACKGROUND: RGB = { r: 15, g: 23, b: 42 }
const DAY_ACCENT: RGB = { r: 37, g: 99, b: 235 }
const NIGHT_ACCENT: RGB = { r: 147, g: 197, b: 253 }
const WHITE: RGB = { r: 255, g: 255, b: 255 }
const BLACK: RGB = { r: 15, g: 23, b: 42 }
const TRANSPARENT_THRESHOLD = 0.08
const TARGET_TEXT_CONTRAST = 4.5

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))

const roundChannel = (value: number) => clamp(Math.round(value), 0, 255)

const mix = (from: RGB, to: RGB, amount: number): RGB => {
  const t = clamp(amount, 0, 1)
  return {
    r: roundChannel(from.r + (to.r - from.r) * t),
    g: roundChannel(from.g + (to.g - from.g) * t),
    b: roundChannel(from.b + (to.b - from.b) * t),
  }
}

const rgbaToString = ({ r, g, b, a }: RGBA) => `rgba(${r}, ${g}, ${b}, ${a.toFixed(3)})`
const rgbToString = ({ r, g, b }: RGB) => `rgb(${r}, ${g}, ${b})`
const toHex = ({ r, g, b }: RGB) => `#${[r, g, b].map((value) => value.toString(16).padStart(2, '0')).join('')}`

const parseHex = (value: string): RGBA | null => {
  const raw = value.trim().replace('#', '')
  if (raw.length === 3) {
    const r = Number.parseInt(raw[0] + raw[0], 16)
    const g = Number.parseInt(raw[1] + raw[1], 16)
    const b = Number.parseInt(raw[2] + raw[2], 16)
    if ([r, g, b].some(Number.isNaN)) return null
    return { r, g, b, a: 1 }
  }
  if (raw.length === 6 || raw.length === 8) {
    const r = Number.parseInt(raw.slice(0, 2), 16)
    const g = Number.parseInt(raw.slice(2, 4), 16)
    const b = Number.parseInt(raw.slice(4, 6), 16)
    const a = raw.length === 8 ? Number.parseInt(raw.slice(6, 8), 16) / 255 : 1
    if ([r, g, b, a].some(Number.isNaN)) return null
    return { r, g, b, a }
  }
  return null
}

const parseRgb = (value: string): RGBA | null => {
  const match = value.trim().match(/^rgba?\((.+)\)$/i)
  if (!match) return null
  const parts = match[1].split(',').map(part => part.trim())
  if (parts.length < 3) return null
  const r = Number.parseFloat(parts[0])
  const g = Number.parseFloat(parts[1])
  const b = Number.parseFloat(parts[2])
  const a = parts.length > 3 ? Number.parseFloat(parts[3]) : 1
  if ([r, g, b, a].some(Number.isNaN)) return null
  return {
    r: roundChannel(r),
    g: roundChannel(g),
    b: roundChannel(b),
    a: clamp(a, 0, 1),
  }
}

const parseColor = (value?: string | null): RGBA | null => {
  if (!value) return null
  if (value.startsWith('#')) return parseHex(value)
  if (value.startsWith('rgb')) return parseRgb(value)
  return null
}

const composite = (foreground: RGBA, background: RGB): RGB => {
  if (foreground.a >= 1) {
    return { r: foreground.r, g: foreground.g, b: foreground.b }
  }
  return {
    r: roundChannel((foreground.r * foreground.a) + background.r * (1 - foreground.a)),
    g: roundChannel((foreground.g * foreground.a) + background.g * (1 - foreground.a)),
    b: roundChannel((foreground.b * foreground.a) + background.b * (1 - foreground.a)),
  }
}

const luminance = (color: RGB) => {
  const normalize = (value: number) => {
    const channel = value / 255
    return channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  }
  const r = normalize(color.r)
  const g = normalize(color.g)
  const b = normalize(color.b)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

const contrastRatio = (a: RGB, b: RGB) => {
  const light = Math.max(luminance(a), luminance(b))
  const dark = Math.min(luminance(a), luminance(b))
  return (light + 0.05) / (dark + 0.05)
}

const resolvePaletteFallback = (): RGB => {
  if (typeof document === 'undefined') {
    return DAY_BACKGROUND
  }
  const palette = document.documentElement?.dataset?.displayPalette === 'night' ? 'night' : 'day'
  return palette === 'night' ? NIGHT_BACKGROUND : DAY_BACKGROUND
}

const readRootColor = (name: string): RGBA | null => {
  if (typeof document === 'undefined') return null
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return parseColor(value)
}

const resolveBaseBackground = (): RGB => {
  const rootBg = readRootColor('--sc-bg-surface') || readRootColor('--sc-bg-elevated')
  if (rootBg) {
    return composite(rootBg, resolvePaletteFallback())
  }
  return resolvePaletteFallback()
}

const resolveIdentityBaseColor = (identityColor?: string | null): RGB => {
  const parsed = parseColor(identityColor)
  if (parsed) {
    return composite(parsed, resolveBaseBackground())
  }
  return resolvePaletteFallback() === NIGHT_BACKGROUND ? NIGHT_ACCENT : DAY_ACCENT
}

const buildDisabledStyle = (kind: IdentityMetaKind, identity: RGB): ResolveIdentityMetaStyleResult => {
  const backgroundAlpha = kind === 'badge' ? 0.11 : 0.08
  const borderAlpha = kind === 'badge' ? 0.2 : 0.16
  return {
    mode: 'disabled',
    contrastRatio: 0,
    style: {
      backgroundColor: rgbaToString({ ...identity, a: backgroundAlpha }),
      color: toHex(identity),
      borderColor: rgbaToString({ ...identity, a: borderAlpha }),
    },
  }
}

const createFill = (background: RGB, identity: RGB, amount: number, surfaceLift: number) => {
  const tinted = mix(background, identity, amount)
  return mix(tinted, WHITE, surfaceLift)
}

const findAccessibleText = (identity: RGB, fill: RGB) => {
  const identityContrast = contrastRatio(identity, fill)
  if (identityContrast >= TARGET_TEXT_CONTRAST) {
    return { mode: 'normal' as const, text: identity, contrast: identityContrast }
  }

  const steps = [0.14, 0.28, 0.42, 0.56, 0.68, 0.8]
  let best: { text: RGB; contrast: number; distance: number } | null = null
  for (const step of steps) {
    for (const candidate of [mix(identity, BLACK, step), mix(identity, WHITE, step)]) {
      const nextContrast = contrastRatio(candidate, fill)
      if (nextContrast < TARGET_TEXT_CONTRAST) continue
      if (!best || step < best.distance) {
        best = { text: candidate, contrast: nextContrast, distance: step }
      }
    }
    if (best) {
      return { mode: 'adjusted' as const, text: best.text, contrast: best.contrast }
    }
  }

  const darkContrast = contrastRatio(BLACK, fill)
  const lightContrast = contrastRatio(WHITE, fill)
  return darkContrast >= lightContrast
    ? { mode: 'fallback' as const, text: BLACK, contrast: darkContrast }
    : { mode: 'fallback' as const, text: WHITE, contrast: lightContrast }
}

const ensureFallbackFillContrast = (text: RGB, fill: RGB) => {
  let current = fill
  let currentContrast = contrastRatio(text, current)
  if (currentContrast >= TARGET_TEXT_CONTRAST) {
    return { fill: current, contrast: currentContrast }
  }
  const direction = text === WHITE ? BLACK : WHITE
  for (const step of [0.1, 0.18, 0.26, 0.34, 0.42, 0.5]) {
    const candidate = mix(current, direction, step)
    currentContrast = contrastRatio(text, candidate)
    if (currentContrast >= TARGET_TEXT_CONTRAST) {
      return { fill: candidate, contrast: currentContrast }
    }
    current = candidate
  }
  return { fill: current, contrast: currentContrast }
}

export const resolveIdentityMetaStyle = ({
  enabled,
  kind,
  identityColor,
  backgroundColor,
}: ResolveIdentityMetaStyleOptions): ResolveIdentityMetaStyleResult => {
  const backgroundParsed = parseColor(backgroundColor)
  const baseBackground = backgroundParsed
    ? composite(backgroundParsed, resolveBaseBackground())
    : resolveBaseBackground()
  const identity = resolveIdentityBaseColor(identityColor)

  if (!enabled) {
    return buildDisabledStyle(kind, identity)
  }

  const baseContrast = contrastRatio(identity, baseBackground)
  const preserveFactor = baseContrast >= TARGET_TEXT_CONTRAST ? 0.42 : 1
  const fillAmount = (kind === 'badge' ? 0.1 : 0.08) * preserveFactor
  const surfaceLift = (kind === 'badge' ? 0.01 : 0.03) * preserveFactor
  let fill = createFill(baseBackground, identity, fillAmount, surfaceLift)
  let { mode, text, contrast } = findAccessibleText(identity, fill)

  if (mode === 'fallback') {
    const fallback = ensureFallbackFillContrast(text, fill)
    fill = fallback.fill
    contrast = fallback.contrast
  }

  const borderBase = mix(identity, text, 0.42)
  const border = mode === 'normal' ? mix(borderBase, fill, 0.12) : mix(borderBase, text, 0.08)
  const shadow = text === WHITE
    ? '0 1px 1px rgba(15, 23, 42, 0.24)'
    : '0 1px 1px rgba(255, 255, 255, 0.12)'

  return {
    mode,
    contrastRatio: Number(contrast.toFixed(2)),
    style: {
      backgroundColor: rgbToString(fill),
      color: toHex(text),
      borderColor: rgbToString(border),
      boxShadow: `inset 0 0 0 1px ${rgbToString(border)}`,
      textShadow: mode === 'normal' ? 'none' : shadow,
    },
  }
}

const resolvePointBackground = (element: HTMLElement, fallback: RGB) => {
  if (typeof document === 'undefined' || typeof document.elementsFromPoint !== 'function') {
    return null
  }
  const rect = element.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) {
    return null
  }
  const x = clamp(rect.left + Math.min(rect.width * 0.5, Math.max(rect.width - 2, 1)), 1, window.innerWidth - 1)
  const y = clamp(rect.top + Math.min(rect.height * 0.5, Math.max(rect.height - 2, 1)), 1, window.innerHeight - 1)
  const stack = document.elementsFromPoint(x, y)
  for (const item of stack) {
    if (!(item instanceof HTMLElement)) continue
    if (item === element || element.contains(item)) continue
    const parsed = parseColor(getComputedStyle(item).backgroundColor)
    if (!parsed || parsed.a <= TRANSPARENT_THRESHOLD) continue
    return composite(parsed, fallback)
  }
  return null
}

export const resolveIdentityMetaHostBackground = (element?: HTMLElement | null) => {
  const baseFallback = resolveBaseBackground()
  if (!element || typeof window === 'undefined') {
    return rgbToString(baseFallback)
  }

  let current: HTMLElement | null = element
  while (current) {
    const parsed = parseColor(getComputedStyle(current).backgroundColor)
    if (parsed && parsed.a > TRANSPARENT_THRESHOLD) {
      return rgbToString(composite(parsed, baseFallback))
    }
    current = current.parentElement
  }

  const stackBackground = resolvePointBackground(element, baseFallback)
  if (stackBackground) {
    return rgbToString(stackBackground)
  }

  return rgbToString(baseFallback)
}
