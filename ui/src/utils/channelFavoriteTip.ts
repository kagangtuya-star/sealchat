export interface ChannelFavoriteTipState {
  everUsed: boolean
  toastJumpCounts: Record<string, number>
  shownRecommendations: string[]
}

const STORAGE_KEY = 'sealchat_channel_favorite_tip_v1'

const createDefaultState = (): ChannelFavoriteTipState => ({
  everUsed: false,
  toastJumpCounts: {},
  shownRecommendations: [],
})

const normalizeKeyPart = (value: unknown): string => String(value || '').trim()

export const buildChannelFavoriteTipKey = (worldId: string, channelId: string): string => {
  const world = normalizeKeyPart(worldId)
  const channel = normalizeKeyPart(channelId)
  return world && channel ? `${world}:${channel}` : ''
}

export const readChannelFavoriteTipState = (): ChannelFavoriteTipState => {
  if (typeof window === 'undefined') return createDefaultState()
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return createDefaultState()
    const parsed = JSON.parse(raw) as Partial<ChannelFavoriteTipState>
    const counts: Record<string, number> = {}
    if (parsed.toastJumpCounts && typeof parsed.toastJumpCounts === 'object') {
      Object.entries(parsed.toastJumpCounts).slice(0, 128).forEach(([key, value]) => {
        const count = Number(value)
        if (key && Number.isFinite(count) && count > 0) {
          counts[key] = Math.min(Math.trunc(count), 3)
        }
      })
    }
    const shown = Array.isArray(parsed.shownRecommendations)
      ? parsed.shownRecommendations
        .map(normalizeKeyPart)
        .filter(Boolean)
        .slice(0, 128)
      : []
    return {
      everUsed: parsed.everUsed === true,
      toastJumpCounts: counts,
      shownRecommendations: Array.from(new Set(shown)),
    }
  } catch {
    return createDefaultState()
  }
}

const writeChannelFavoriteTipState = (state: ChannelFavoriteTipState) => {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  } catch {
    // Storage may be unavailable in private/embed contexts; runtime behavior still works.
  }
}

export const markChannelFavoritesEverUsed = () => {
  const state = readChannelFavoriteTipState()
  if (state.everUsed) return
  state.everUsed = true
  writeChannelFavoriteTipState(state)
}

export const recordWorldMessageToastJump = (worldId: string, channelId: string): number => {
  const key = buildChannelFavoriteTipKey(worldId, channelId)
  if (!key) return 0
  const state = readChannelFavoriteTipState()
  const count = Math.min((state.toastJumpCounts[key] || 0) + 1, 3)
  state.toastJumpCounts[key] = count
  writeChannelFavoriteTipState(state)
  return count
}

export const hasShownChannelFavoriteRecommendation = (worldId: string, channelId: string): boolean => {
  const key = buildChannelFavoriteTipKey(worldId, channelId)
  return Boolean(key && readChannelFavoriteTipState().shownRecommendations.includes(key))
}

export const markChannelFavoriteRecommendationShown = (worldId: string, channelId: string) => {
  const key = buildChannelFavoriteTipKey(worldId, channelId)
  if (!key) return
  const state = readChannelFavoriteTipState()
  if (!state.shownRecommendations.includes(key)) {
    state.shownRecommendations = [...state.shownRecommendations, key].slice(-128)
    writeChannelFavoriteTipState(state)
  }
}

export const hasExistingFavoriteChannels = (value: unknown): boolean => {
  if (!value || typeof value !== 'object') return false
  return Object.values(value as Record<string, unknown>).some((ids) => Array.isArray(ids) && ids.length > 0)
}

