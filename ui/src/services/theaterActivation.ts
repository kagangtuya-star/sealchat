import { api } from '@/stores/_config'

const activationPath = (worldId: string) => `api/v1/worlds/${encodeURIComponent(worldId)}/theater/activate`

const activationRequired = (error: unknown) => (
  (error as { response?: { data?: { error?: { code?: unknown } } } })?.response?.data?.error?.code
  === 'THEATER_ACTIVATION_REQUIRED'
)

export const activateWorldTheater = (worldId: string, activationCode = '') => (
  api.post(activationPath(worldId), { activationCode })
)

export const isTheaterActivationRequired = activationRequired
