import { nanoid } from 'nanoid'
import type { AvatarDecoration } from '@/types'

const cloneDecoration = (item: AvatarDecoration): AvatarDecoration => ({
  ...item,
  settings: item.settings ? { ...item.settings } : undefined,
})

const ensureDecorationId = (item: AvatarDecoration): AvatarDecoration => ({
  ...cloneDecoration(item),
  id: String(item.id || item.decorationId || nanoid()).trim(),
})

export const normalizeAvatarDecorations = (
  value?: AvatarDecoration[] | AvatarDecoration | null,
  legacyValue?: AvatarDecoration | null,
): AvatarDecoration[] => {
  const source = Array.isArray(value)
    ? value
    : value
      ? [value]
      : legacyValue
        ? [legacyValue]
        : []
  return source
    .filter(Boolean)
    .map(ensureDecorationId)
}

export const firstAvatarDecoration = (value?: AvatarDecoration[] | null): AvatarDecoration | null => {
  if (!Array.isArray(value) || value.length === 0) {
    return null
  }
  return ensureDecorationId(value[0])
}
