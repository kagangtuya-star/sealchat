import { api } from '@/stores/_config'
import type { TheaterPresentation } from '@/types/theaterPresentation'

type ActorKey = { identityId: string; variantId?: string | null }
type CacheEntry = { revision: string; presentation: TheaterPresentation | null }

const cache = new Map<string, CacheEntry>()
const inFlight = new Map<string, Promise<void>>()

const keyOf = (channelId: string, actor: ActorKey) => (
  `${String(channelId).trim()}\u0000${String(actor.identityId).trim()}\u0000${String(actor.variantId || '').trim()}`
)

const request = async (channelId: string, actors: ActorKey[]) => {
  const response = await api.post<{ items: Array<{ identityId: string; variantId?: string | null; revision: string; presentation: TheaterPresentation | null }> }>(
    `api/v1/channels/${encodeURIComponent(channelId)}/theater-presentations/resolve`,
    { actors },
  )
  for (const item of response.data.items || []) {
    cache.set(keyOf(channelId, item), { revision: item.revision || '', presentation: item.presentation || null })
  }
}

export const useTheaterAppearanceCache = () => {
  const resolve = async (channelId: string, actor: ActorKey): Promise<CacheEntry | null> => {
    const normalizedChannelId = String(channelId).trim()
    const identityId = String(actor.identityId).trim()
    if (!normalizedChannelId || !identityId) return null
    const key = keyOf(normalizedChannelId, actor)
    const hit = cache.get(key)
    if (hit) return hit
    let task = inFlight.get(key)
    if (!task) {
      task = request(normalizedChannelId, [actor]).finally(() => inFlight.delete(key))
      inFlight.set(key, task)
    }
    await task
    return cache.get(key) || null
  }

  const invalidate = (channelId: string, actor?: ActorKey) => {
    if (actor) cache.delete(keyOf(channelId, actor))
    else {
      const prefix = `${String(channelId).trim()}\u0000`
      for (const key of cache.keys()) if (key.startsWith(prefix)) cache.delete(key)
    }
  }

  const clear = () => cache.clear()
  return { resolve, invalidate, clear }
}
