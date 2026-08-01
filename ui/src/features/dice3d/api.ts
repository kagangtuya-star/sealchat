import { api } from '@/stores/_config'
import type { Dice3DMemberProfile, Dice3DWorldConfig } from '@/types'

export const loadDice3DSettings = async (worldId: string) => {
  const [world, personal] = await Promise.all([
    api.get<{ config: Dice3DWorldConfig }>(`api/v1/worlds/${worldId}/dice3d`),
    api.get<{ profile: Dice3DMemberProfile, revision: number }>(`api/v1/worlds/${worldId}/dice3d/profile`),
  ])
  return { config: world.data.config, profile: personal.data.profile, revision: personal.data.revision }
}

export const saveDice3DWorldSettings = async (worldId: string, config: Dice3DWorldConfig) => {
  const response = await api.put<{ config: Dice3DWorldConfig }>(`api/v1/worlds/${worldId}/dice3d`, config)
  return response.data.config
}

export const saveDice3DProfile = async (worldId: string, profile: Dice3DMemberProfile) => {
  const response = await api.put<{ profile: Dice3DMemberProfile, revision: number }>(`api/v1/worlds/${worldId}/dice3d/profile`, profile)
  return response.data
}
