import { api } from '@/stores/_config'

export interface WorldKeywordItem {
  id: string
  worldId: string
  keyword: string
  category: string
  aliases: string[]
  matchMode: 'plain' | 'regex'
  description: string
  descriptionFormat?: 'plain' | 'rich'
  display: 'standard' | 'minimal' | 'inherit'
  sortOrder: number
  isEnabled: boolean
  createdAt: string
  updatedAt: string
  createdBy?: string
  updatedBy?: string
  matchedVia?: string
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
  descriptionFormat?: 'plain' | 'rich'
  display?: 'standard' | 'minimal' | 'inherit'
  sortOrder?: number
  isEnabled?: boolean
}

export interface WorldKeywordReorderItem {
  id: string
  sortOrder: number
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

export async function reorderWorldKeywords(worldId: string, items: WorldKeywordReorderItem[]) {
  const { data } = await api.post<{ updated: number }>(`/api/v1/worlds/${worldId}/keywords/reorder`, { items })
  return data.updated
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

export async function createWorldKeywordCategory(worldId: string, name: string): Promise<string> {
  const { data } = await api.post<{ name: string }>(`/api/v1/worlds/${worldId}/keywords/categories`, { name })
  return data.name
}

export async function renameWorldKeywordCategory(worldId: string, oldName: string, newName: string): Promise<{ updated: number; name: string }> {
  const { data } = await api.post<{ updated: number; name: string }>(`/api/v1/worlds/${worldId}/keywords/categories/rename`, { oldName, newName })
  return data
}

export async function deleteWorldKeywordCategory(worldId: string, name: string): Promise<number> {
  const { data } = await api.post<{ updated: number }>(`/api/v1/worlds/${worldId}/keywords/categories/delete`, { name })
  return data.updated
}

export async function fetchWorldKeywordCategoriesPublic(worldId: string): Promise<string[]> {
  const { data } = await api.get<{ categories: string[] }>(`/api/v1/public/worlds/${worldId}/keywords/categories`)
  return data.categories
}
