import { api } from '@/stores/_config'

export interface WorldKeywordItem {
  id: string
  worldId: string
  keyword: string
  category: string
  aliases: string[]
  matchMode: 'plain' | 'regex'
  description: string
  display: 'standard' | 'minimal' | 'inherit'
  isEnabled: boolean
  createdAt: string
  updatedAt: string
  createdBy?: string
  updatedBy?: string
}

export interface WorldKeywordListResponse {
  items: WorldKeywordItem[]
  total: number
  page: number
  pageSize: number
}

export interface WorldKeywordPayload {
  keyword: string
  category?: string
  aliases?: string[]
  matchMode?: 'plain' | 'regex'
  description?: string
  display?: 'standard' | 'minimal' | 'inherit'
  isEnabled?: boolean
}

export interface WorldKeywordImportPayload {
  items: WorldKeywordPayload[]
  replace?: boolean
}

export async function fetchWorldKeywords(worldId: string, params?: { page?: number; pageSize?: number; q?: string; includeDisabled?: boolean }) {
  if (!worldId) throw new Error('worldId is required')
  const { data } = await api.get<WorldKeywordListResponse>(`/api/v1/worlds/${worldId}/keywords`, { params })
  return data
}

export async function fetchWorldKeywordsPublic(worldId: string, params?: { page?: number; pageSize?: number; q?: string; category?: string }) {
  if (!worldId) throw new Error('worldId is required')
  const { data } = await api.get<WorldKeywordListResponse>(`/api/v1/public/worlds/${worldId}/keywords`, { params })
  return data
}

export async function createWorldKeyword(worldId: string, payload: WorldKeywordPayload) {
  const { data } = await api.post<{ item: WorldKeywordItem }>(`/api/v1/worlds/${worldId}/keywords`, payload)
  return data.item
}

export async function updateWorldKeyword(worldId: string, keywordId: string, payload: WorldKeywordPayload) {
  const { data } = await api.patch<{ item: WorldKeywordItem }>(`/api/v1/worlds/${worldId}/keywords/${keywordId}`, payload)
  return data.item
}

export async function deleteWorldKeyword(worldId: string, keywordId: string) {
  await api.delete(`/api/v1/worlds/${worldId}/keywords/${keywordId}`)
}

export async function bulkDeleteWorldKeywords(worldId: string, ids: string[]) {
  const { data } = await api.post<{ deleted: number }>(`/api/v1/worlds/${worldId}/keywords/bulk-delete`, { ids })
  return data.deleted
}

export async function importWorldKeywords(worldId: string, payload: WorldKeywordImportPayload) {
  const { data } = await api.post<{ stats: { created: number; updated: number; skipped: number } }>(`/api/v1/worlds/${worldId}/keywords/import`, payload)
  return data.stats
}

export async function exportWorldKeywords(worldId: string, category?: string) {
  const params = category ? { category } : undefined
  const { data } = await api.get<{ items: WorldKeywordItem[] }>(`/api/v1/worlds/${worldId}/keywords/export`, { params })
  return data.items
}

export async function fetchWorldKeywordCategories(worldId: string): Promise<string[]> {
  const { data } = await api.get<{ categories: string[] }>(`/api/v1/worlds/${worldId}/keywords/categories`)
  return data.categories
}

export async function fetchWorldKeywordCategoriesPublic(worldId: string): Promise<string[]> {
  const { data } = await api.get<{ categories: string[] }>(`/api/v1/public/worlds/${worldId}/keywords/categories`)
  return data.categories
}
