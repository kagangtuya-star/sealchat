import type { FavoriteHotkey } from '@/stores/display'

const MODIFIER_LABELS: Record<'ctrl' | 'meta' | 'alt' | 'shift', string> = {
  ctrl: 'Ctrl',
  meta: 'Cmd',
  alt: 'Alt',
  shift: 'Shift',
}

const SPECIAL_KEY_LABELS: Record<string, string> = {
  ' ': 'Space',
  Space: 'Space',
  Shift: 'Shift',
  Control: 'Ctrl',
  Alt: 'Alt',
  Meta: 'Cmd',
  Enter: 'Enter',
  Escape: 'Esc',
  ArrowUp: 'ArrowUp',
  ArrowDown: 'ArrowDown',
  ArrowLeft: 'ArrowLeft',
  ArrowRight: 'ArrowRight',
  Tab: 'Tab',
  Backspace: 'Backspace',
  Delete: 'Delete',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
}

const isModifierKey = (key: string) => {
  const normalized = key.toLowerCase()
  return normalized === 'shift' || normalized === 'control' || normalized === 'alt' || normalized === 'meta'
}

const normalizeBaseKey = (key: string): string => {
  if (!key) return ''
  if (SPECIAL_KEY_LABELS[key]) {
    return key
  }
  if (key.length === 1) {
    return key.toUpperCase()
  }
  return key
}

const formatKeyLabel = (key: string): string => {
  if (SPECIAL_KEY_LABELS[key]) {
    return SPECIAL_KEY_LABELS[key]
  }
  if (key.length === 1) {
    return key.toUpperCase()
  }
  return key
}

export const formatHotkeyCombo = (descriptor?: FavoriteHotkey | null): string => {
  if (!descriptor) return ''
  const parts: string[] = []
  if (isModifierKey(descriptor.key)) {
    if (descriptor.ctrl) parts.push(MODIFIER_LABELS.ctrl)
    if (descriptor.meta) parts.push(MODIFIER_LABELS.meta)
    if (descriptor.alt) parts.push(MODIFIER_LABELS.alt)
    if (descriptor.shift) parts.push(MODIFIER_LABELS.shift)
    if (!parts.length) parts.push(formatKeyLabel(descriptor.key))
    return parts.join('+')
  }
  if (descriptor.ctrl && descriptor.key !== 'Control') parts.push(MODIFIER_LABELS.ctrl)
  if (descriptor.meta && descriptor.key !== 'Meta') parts.push(MODIFIER_LABELS.meta)
  if (descriptor.alt && descriptor.key !== 'Alt') parts.push(MODIFIER_LABELS.alt)
  if (descriptor.shift && descriptor.key !== 'Shift') parts.push(MODIFIER_LABELS.shift)
  parts.push(formatKeyLabel(descriptor.key))
  return parts.join('+')
}

export const captureHotkeyFromEvent = (event: KeyboardEvent): FavoriteHotkey | null => {
  if (!event) return null
  const hasPrimaryModifier = event.ctrlKey || event.metaKey || event.altKey
  if (!hasPrimaryModifier) return null
  const key = normalizeBaseKey(event.key)
  if (!key || isModifierKey(key)) {
    return null
  }
  return {
    key,
    ctrl: event.ctrlKey || undefined,
    meta: event.metaKey || undefined,
    alt: event.altKey || undefined,
    shift: event.shiftKey || undefined,
    combo: '',
  }
}

export const enrichHotkeyCombo = (hotkey: FavoriteHotkey | null): FavoriteHotkey | null => {
  if (!hotkey) return null
  if (hotkey.combo && hotkey.combo.length > 0) {
    return hotkey
  }
  return {
    ...hotkey,
    combo: formatHotkeyCombo(hotkey),
  }
}

export const isHotkeyMatchingEvent = (event: KeyboardEvent, hotkey?: FavoriteHotkey | null): boolean => {
  if (!event || !hotkey) return false
  const key = normalizeBaseKey(event.key)
  if (!key) return false
  return (
    key === hotkey.key &&
    Boolean(hotkey.ctrl) === Boolean(event.ctrlKey) &&
    Boolean(hotkey.meta) === Boolean(event.metaKey) &&
    Boolean(hotkey.alt) === Boolean(event.altKey) &&
    Boolean(hotkey.shift) === Boolean(event.shiftKey)
  )
}

export const buildHotkeyDescriptor = (event: KeyboardEvent): FavoriteHotkey | null => {
  const captured = captureHotkeyFromEvent(event)
  if (!captured) return null
  return enrichHotkeyCombo({ ...captured })
}

export const buildHoldHotkeyDescriptor = (event: KeyboardEvent): FavoriteHotkey | null => {
  if (!event) return null
  const key = normalizeBaseKey(event.key)
  if (!key) return null
  const hasModifier = event.ctrlKey || event.metaKey || event.altKey || event.shiftKey || isModifierKey(key)
  if (!hasModifier) return null
  return enrichHotkeyCombo({
    key,
    ctrl: event.ctrlKey || undefined,
    meta: event.metaKey || undefined,
    alt: event.altKey || undefined,
    shift: event.shiftKey || undefined,
    combo: '',
  })
}

export const isHotkeyReleaseEvent = (event: KeyboardEvent, hotkey?: FavoriteHotkey | null): boolean => {
  if (!event || !hotkey) return false
  const key = normalizeBaseKey(event.key)
  if (key === hotkey.key) return true
  if (key === 'Control' && hotkey.ctrl) return true
  if (key === 'Meta' && hotkey.meta) return true
  if (key === 'Alt' && hotkey.alt) return true
  if (key === 'Shift' && hotkey.shift) return true
  return false
}
