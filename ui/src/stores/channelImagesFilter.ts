export type ChannelImagesIcModeFilter = 'all' | 'ic' | 'ooc'

export const CHANNEL_IMAGES_IC_MODE_FILTER_STORAGE_KEY = 'sealchat.channelImages.icModeFilter'

export const normalizeChannelImagesIcModeFilter = (value: unknown): ChannelImagesIcModeFilter => {
  return value === 'ic' || value === 'ooc' || value === 'all' ? value : 'all'
}

export const nextChannelImagesIcModeFilter = (value: unknown): ChannelImagesIcModeFilter => {
  switch (normalizeChannelImagesIcModeFilter(value)) {
    case 'ic':
      return 'ooc'
    case 'ooc':
      return 'all'
    default:
      return 'ic'
  }
}

const resolveDefaultStorage = (): Pick<Storage, 'getItem' | 'setItem'> | null => {
  if (typeof window === 'undefined') {
    return null
  }
  return window.localStorage
}

export const readChannelImagesIcModeFilter = (
  storage: Pick<Storage, 'getItem'> | null = resolveDefaultStorage(),
): ChannelImagesIcModeFilter => {
  try {
    return normalizeChannelImagesIcModeFilter(storage?.getItem(CHANNEL_IMAGES_IC_MODE_FILTER_STORAGE_KEY))
  } catch {
    return 'all'
  }
}

export const writeChannelImagesIcModeFilter = (
  storageOrValue: Pick<Storage, 'setItem'> | ChannelImagesIcModeFilter | null,
  maybeValue?: ChannelImagesIcModeFilter,
): void => {
  const storage = typeof storageOrValue === 'string' ? resolveDefaultStorage() : storageOrValue
  const value = typeof storageOrValue === 'string' ? storageOrValue : maybeValue
  try {
    storage?.setItem(CHANNEL_IMAGES_IC_MODE_FILTER_STORAGE_KEY, normalizeChannelImagesIcModeFilter(value))
  } catch {
    // Ignore storage failures in private mode or quota-limited environments.
  }
}
