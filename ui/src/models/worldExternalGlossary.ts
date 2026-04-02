import { api } from '@/stores/_config'

export interface WorldExternalGlossaryLibraryItem {
  id: string
  name: string
  description: string
  isEnabled: boolean
  isBound: boolean
  sortOrder: number
  termCount: number
}

export async function fetchWorldExternalGlossaries(worldId: string) {
  const { data } = await api.get<{ items: WorldExternalGlossaryLibraryItem[]; total: number }>(`/api/v1/worlds/${worldId}/external-glossaries`)
  return data
}

export async function enableWorldExternalGlossary(worldId: string, libraryId: string) {
  await api.post(`/api/v1/worlds/${worldId}/external-glossaries/${libraryId}/enable`)
}

export async function disableWorldExternalGlossary(worldId: string, libraryId: string) {
  await api.post(`/api/v1/worlds/${worldId}/external-glossaries/${libraryId}/disable`)
}

export async function bulkEnableWorldExternalGlossaries(worldId: string, libraryIds: string[]) {
  const { data } = await api.post<{ updated: number }>(`/api/v1/worlds/${worldId}/external-glossaries/bulk-enable`, { libraryIds })
  return data.updated
}

export async function bulkDisableWorldExternalGlossaries(worldId: string, libraryIds: string[]) {
  const { data } = await api.post<{ updated: number }>(`/api/v1/worlds/${worldId}/external-glossaries/bulk-disable`, { libraryIds })
  return data.updated
}
