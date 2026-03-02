export const FONT_ASSET_LIMIT = 3

export const DEFAULT_SANS_FONT_STACK =
  '"PingFang SC", "Microsoft YaHei", "Noto Sans SC", "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'

export const DEFAULT_MONO_FONT_STACK =
  'ui-monospace, SFMono-Regular, "SF Mono", Consolas, "Liberation Mono", Menlo, monospace'

const GENERIC_FONT_FAMILIES = new Set([
  'serif',
  'sans-serif',
  'monospace',
  'cursive',
  'fantasy',
  'system-ui',
  'emoji',
  'math',
  'fangsong',
  'ui-serif',
  'ui-sans-serif',
  'ui-monospace',
  'ui-rounded',
])

const MAX_FONT_FAMILY_LENGTH = 120

export const sanitizeFontFamilyName = (value: unknown): string => {
  if (typeof value !== 'string') return ''
  const normalized = value.trim().replace(/\s+/g, ' ')
  if (!normalized) return ''
  return normalized.slice(0, MAX_FONT_FAMILY_LENGTH)
}

export const quoteFontFamilyName = (family: string): string => {
  const normalized = sanitizeFontFamilyName(family)
  if (!normalized) return ''
  const lower = normalized.toLowerCase()
  if (GENERIC_FONT_FAMILIES.has(lower)) return lower
  if (
    (normalized.startsWith('"') && normalized.endsWith('"'))
    || (normalized.startsWith('\'') && normalized.endsWith('\''))
  ) {
    return normalized
  }
  const escaped = normalized.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
  return `"${escaped}"`
}

export const buildGlobalFontFamilyStack = (preferredFamily?: string): string => {
  const preferred = quoteFontFamilyName(preferredFamily || '')
  if (!preferred) return DEFAULT_SANS_FONT_STACK
  return `${preferred}, ${DEFAULT_SANS_FONT_STACK}`
}

export const inferFontFamilyFromFilename = (filename: string): string => {
  const base = (filename || '').replace(/\.[^/.]+$/u, '').replace(/[_-]+/g, ' ')
  return sanitizeFontFamilyName(base)
}

export const createFontAssetId = (): string =>
  `font_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
