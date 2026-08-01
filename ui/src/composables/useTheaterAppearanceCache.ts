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
const versions = new Map<string, number>()

const keyOf = (worldId: string, channelId: string, actor: ActorKey) => (
  `${String(worldId).trim()}\u0000${String(channelId).trim()}\u0000${String(actor.identityId).trim()}\u0000${String(actor.variantId || '').trim()}`
)

const request = async (worldId: string, channelId: string, actors: ActorKey[]) => {
  const requestVersions = new Map(actors.map(actor => {
    const key = keyOf(worldId, channelId, actor)
    return [key, versions.get(key) || 0] as const
  }))
  const response = await api.post<{ items: ResolvedActor[] }>(
    `api/v1/worlds/${encodeURIComponent(worldId)}/theater-presentations/resolve`,
    { actors: actors.map(actor => ({ channelId, ...actor })) },
  )
  for (const actor of actors) {
    const key = keyOf(worldId, channelId, actor)
    if ((versions.get(key) || 0) === requestVersions.get(key)) {
      cache.set(key, { revision: '', presentation: null })
    }
  }
  for (const item of response.data.items || []) {
    const key = keyOf(worldId, item.sourceChannelId, {
      identityId: item.identityId,
      variantId: item.requestedVariantId ?? item.variantId,
    })
    if ((versions.get(key) || 0) === requestVersions.get(key)) {
      cache.set(key, { revision: item.revision || '', presentation: item.presentation || null })
    }
  }
}

const invalidateKey = (key: string) => {
  versions.set(key, (versions.get(key) || 0) + 1)
  cache.delete(key)
  inFlight.delete(key)
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
      const requestTask = request(normalizedWorldId, normalizedChannelId, [actor])
      let trackedTask: Promise<void>
      trackedTask = requestTask.finally(() => {
        if (inFlight.get(key) === trackedTask) inFlight.delete(key)
      })
      task = trackedTask
      inFlight.set(key, task)
    }
    await task
    return cache.get(key) || null
  }

  const invalidate = (worldId: string, channelId: string, actor?: ActorKey) => {
    if (actor) invalidateKey(keyOf(worldId, channelId, actor))
    else {
      const prefix = `${String(worldId).trim()}\u0000${String(channelId).trim()}\u0000`
      const keys = new Set([...cache.keys(), ...inFlight.keys()])
      for (const key of keys) if (key.startsWith(prefix)) invalidateKey(key)
    }
  }

  const invalidateChannel = (channelId: string) => {
    const normalizedChannelId = String(channelId).trim()
    if (!normalizedChannelId) return
    const keys = new Set([...cache.keys(), ...inFlight.keys()])
    for (const key of keys) {
      if (key.split('\u0000')[1] === normalizedChannelId) invalidateKey(key)
    }
  }

  const clear = () => {
    const keys = new Set([...cache.keys(), ...inFlight.keys()])
    for (const key of keys) invalidateKey(key)
  }
  return { resolve, invalidate, invalidateChannel, clear }
}
