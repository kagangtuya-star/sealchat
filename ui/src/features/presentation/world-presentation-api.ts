import { api } from '@/stores/_config'
import { normalizeWorldPresentationConfig, type WorldPresentationConfig } from './world-presentation-config'

interface WorldPresentationResponse {
  config: unknown
  revision: string
}

const settingsPath = (worldId: string) => `api/v1/worlds/${encodeURIComponent(worldId)}/presentation`

const normalizeResponse = ({ config, revision }: WorldPresentationResponse) => ({
  config: normalizeWorldPresentationConfig(config),
  revision,
})

export const readWorldPresentation = async (worldId: string, signal: AbortSignal) => (
  normalizeResponse((await api.get<WorldPresentationResponse>(settingsPath(worldId), { signal })).data)
)

export const saveWorldPresentation = async (worldId: string, config: WorldPresentationConfig, revision: string, signal: AbortSignal) => (
  normalizeResponse((await api.put<WorldPresentationResponse>(settingsPath(worldId), { config, revision }, { signal })).data)
)
