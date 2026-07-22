export type TheaterAppearanceEditIntent = {
  channelId: string
  identityId: string
  mode: 'base' | 'variant'
  variantId?: string
  targetUserId?: string
  targetKind?: 'self' | 'user' | 'bot'
  targetLabel?: string
  targetAvatar?: string
  createdAt: number
}

const STORAGE_KEY = 'sealchat.theater.appearance-edit.intent.v1'
const INTENT_TTL_MS = 5 * 60 * 1000

const isRecord = (value: unknown): value is Record<string, unknown> => (
  typeof value === 'object' && value !== null
)

const parseIntent = (raw: string | null): TheaterAppearanceEditIntent | null => {
  if (!raw) return null
  try {
    const data = JSON.parse(raw) as unknown
    if (!isRecord(data)) return null
    const channelId = String(data.channelId || '').trim()
    const identityId = String(data.identityId || '').trim()
    const mode = data.mode === 'variant' ? 'variant' : data.mode === 'base' ? 'base' : ''
    const createdAt = Number(data.createdAt || 0)
    if (!channelId || !identityId || !mode || !Number.isFinite(createdAt) || createdAt <= 0) return null
    if (Date.now() - createdAt > INTENT_TTL_MS) return null
    return {
      channelId,
      identityId,
      mode,
      variantId: String(data.variantId || '').trim() || undefined,
      targetUserId: String(data.targetUserId || '').trim() || undefined,
      targetKind: data.targetKind === 'user' || data.targetKind === 'bot' || data.targetKind === 'self'
        ? data.targetKind
        : undefined,
      targetLabel: String(data.targetLabel || '').trim() || undefined,
      targetAvatar: String(data.targetAvatar || '').trim() || undefined,
      createdAt,
    }
  } catch {
    return null
  }
}

export const writeTheaterAppearanceEditIntent = (intent: Omit<TheaterAppearanceEditIntent, 'createdAt'>) => {
  if (typeof sessionStorage === 'undefined') return
  const payload: TheaterAppearanceEditIntent = {
    ...intent,
    channelId: String(intent.channelId || '').trim(),
    identityId: String(intent.identityId || '').trim(),
    createdAt: Date.now(),
  }
  if (!payload.channelId || !payload.identityId) return
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
  } catch {
    // Ignore storage failures; user can re-open the editor manually after entering theater.
  }
}

export const peekTheaterAppearanceEditIntent = (): TheaterAppearanceEditIntent | null => {
  if (typeof sessionStorage === 'undefined') return null
  try {
    const intent = parseIntent(sessionStorage.getItem(STORAGE_KEY))
    if (!intent) sessionStorage.removeItem(STORAGE_KEY)
    return intent
  } catch {
    return null
  }
}

export const consumeTheaterAppearanceEditIntent = (
  expectedChannelId?: string,
): TheaterAppearanceEditIntent | null => {
  const intent = peekTheaterAppearanceEditIntent()
  if (!intent) return null
  if (expectedChannelId && intent.channelId !== String(expectedChannelId).trim()) return null
  try {
    sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // no-op
  }
  return intent
}

export const clearTheaterAppearanceEditIntent = () => {
  if (typeof sessionStorage === 'undefined') return
  try {
    sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // no-op
  }
}
