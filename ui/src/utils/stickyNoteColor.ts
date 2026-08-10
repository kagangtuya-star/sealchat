export const STICKY_NOTE_PRESET_COLORS = [
  'yellow',
  'pink',
  'green',
  'blue',
  'purple',
  'orange',
] as const

export type StickyNotePresetColor = typeof STICKY_NOTE_PRESET_COLORS[number]

const STICKY_NOTE_PRESET_COLOR_VALUES: Record<StickyNotePresetColor, string> = {
  yellow: '#fff9c4',
  pink: '#f8bbd9',
  green: '#c8e6c9',
  blue: '#bbdefb',
  purple: '#e1bee7',
  orange: '#ffe0b2',
}

function isHexDigit(value: string) {
  return /^[0-9a-f]$/i.test(value)
}

export function normalizeStickyNoteHexColor(value: string | undefined | null): string | null {
  const trimmed = String(value || '').trim().toLowerCase()
  const hex = trimmed.startsWith('#') ? trimmed.slice(1) : trimmed
  if (hex.length !== 3 && hex.length !== 6) return null
  if (!Array.from(hex).every(isHexDigit)) return null
  if (hex.length === 3) {
    return `#${hex.split('').map(char => `${char}${char}`).join('')}`
  }
  return `#${hex}`
}

export function isStickyNoteCustomColor(value: string | undefined | null): boolean {
  return normalizeStickyNoteHexColor(value) !== null
}

export function getStickyNoteColorValue(value: string | undefined | null): string {
  const color = String(value || '').trim().toLowerCase()
  if (Object.prototype.hasOwnProperty.call(STICKY_NOTE_PRESET_COLOR_VALUES, color)) {
    return STICKY_NOTE_PRESET_COLOR_VALUES[color as StickyNotePresetColor]
  }
  return normalizeStickyNoteHexColor(color) || STICKY_NOTE_PRESET_COLOR_VALUES.yellow
}

export function getStickyNoteSurfaceColor(value: string | undefined | null): string {
  const customColor = normalizeStickyNoteHexColor(value)
  if (!customColor) return getStickyNoteColorValue(value)

  const channels = [1, 3, 5].map(offset => {
    const channel = Number.parseInt(customColor.slice(offset, offset + 2), 16)
    return Math.round(channel * 0.3 + 255 * 0.7).toString(16).padStart(2, '0')
  })
  return `#${channels.join('')}`
}

export function getStickyNoteColorTheme(value: string | undefined | null): StickyNotePresetColor | 'custom' {
  const color = String(value || '').trim().toLowerCase()
  if (Object.prototype.hasOwnProperty.call(STICKY_NOTE_PRESET_COLOR_VALUES, color)) {
    return color as StickyNotePresetColor
  }
  return isStickyNoteCustomColor(color) ? 'custom' : 'yellow'
}
