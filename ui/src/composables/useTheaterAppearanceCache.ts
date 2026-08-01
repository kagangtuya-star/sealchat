import { api } from '@/stores/_config'
import type { TheaterPresentation } from '@/types/theaterPresentation'

type ActorKey = { identityId: string; variantId?: string | null }
type CacheEntry = { revision: string; presentation: TheaterPresentation | null }
type ResolvedActor = ActorKey & {
  sourceChannelId: string
  requestedVariantId?: string | null
  revision: string
  presentation: TheaterPresentation | null
}

const cache = new Map<string, CacheEntry>()
const inFlight = new Map<string, Promise<void>>()

const keyOf = (worldId: string, channelId: string, actor: ActorKey) => (
  `${String(worldId).trim()}\u0000${String(channelId).trim()}\u0000${String(actor.identityId).trim()}\u0000${String(actor.variantId || '').trim()}`
)

const request = async (worldId: string, channelId: string, actors: ActorKey[]) => {
  const response = await api.post<{ items: ResolvedActor[] }>(
    `api/v1/worlds/${encodeURIComponent(worldId)}/theater-presentations/resolve`,
    { actors: actors.map(actor => ({ channelId, ...actor })) },
  )
  for (const actor of actors) cache.set(keyOf(worldId, channelId, actor), { revision: '', presentation: null })
  for (const item of response.data.items || []) {
    cache.set(keyOf(worldId, item.sourceChannelId, {
      identityId: item.identityId,
      variantId: item.requestedVariantId ?? item.variantId,
    }), { revision: item.revision || '', presentation: item.presentation || null })
  }
}

export const useTheaterAppearanceCache = () => {
  const resolve = async (worldId: string, channelId: string, actor: ActorKey): Promise<CacheEntry | null> => {
    const normalizedWorldId = String(worldId).trim()
    const normalizedChannelId = String(channelId).trim()
    const identityId = String(actor.identityId).trim()
    if (!normalizedWorldId || !normalizedChannelId || !identityId) return null
    const key = keyOf(normalizedWorldId, normalizedChannelId, actor)
    const hit = cache.get(key)
    if (hit) return hit
    let task = inFlight.get(key)
    if (!task) {
      task = request(normalizedWorldId, normalizedChannelId, [actor]).finally(() => inFlight.delete(key))
      inFlight.set(key, task)
    }
    await task
    return cache.get(key) || null
  }

  const invalidate = (worldId: string, channelId: string, actor?: ActorKey) => {
    if (actor) cache.delete(keyOf(worldId, channelId, actor))
    else {
      const prefix = `${String(worldId).trim()}\u0000${String(channelId).trim()}\u0000`
      for (const key of cache.keys()) if (key.startsWith(prefix)) cache.delete(key)
    }
  }

  const clear = () => cache.clear()
  return { resolve, invalidate, clear }
}
