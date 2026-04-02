import { api } from '@/stores/_config'
import type { WorldKeywordPayload, WorldKeywordReorderItem } from './worldGlossary'

export interface ExternalGlossaryLibraryItem {
  id: string
  name: string
  description: string
  isEnabled: boolean
  sortOrder: number
  createdBy?: string
  updatedBy?: string
  createdAt: string
  updatedAt: string
  termCount: number
}

export interface ExternalGlossaryTermItem {
  id: string
  libraryId: string
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
}

export interface ExternalGlossaryLibraryListResponse {
  items: ExternalGlossaryLibraryItem[]
  total: number
  page: number
  pageSize: number
}

export interface ExternalGlossaryTermListResponse {
  items: ExternalGlossaryTermItem[]
  total: number
  page: number
  pageSize: number
}

export interface ExternalGlossaryLibraryPayload {
  name: string
  description?: string
  isEnabled?: boolean
  sortOrder?: number
}

export interface ExternalGlossaryLibraryImportPayload {
  library: ExternalGlossaryLibraryPayload
  items: WorldKeywordPayload[]
  replace?: boolean
}

export interface ExternalGlossaryLibraryExportPayload {
  library: ExternalGlossaryLibraryItem
  categories: string[]
  items: ExternalGlossaryTermItem[]
}

export async function fetchExternalGlossaryLibraries(params?: {
  page?: number
  pageSize?: number
  q?: string
  includeDisabled?: boolean
}) {
  const { data } = await api.get<ExternalGlossaryLibraryListResponse>('/api/v1/admin/external-glossaries', { params })
  return data
}

export async function createExternalGlossaryLibrary(payload: ExternalGlossaryLibraryPayload) {
  const { data } = await api.post<{ item: ExternalGlossaryLibraryItem }>('/api/v1/admin/external-glossaries', payload)
  return data.item
}

export async function updateExternalGlossaryLibrary(libraryId: string, payload: ExternalGlossaryLibraryPayload) {
  const { data } = await api.patch<{ item: ExternalGlossaryLibraryItem }>(`/api/v1/admin/external-glossaries/${libraryId}`, payload)
  return data.item
}

export async function deleteExternalGlossaryLibrary(libraryId: string) {
  await api.delete(`/api/v1/admin/external-glossaries/${libraryId}`)
}

export async function bulkDeleteExternalGlossaryLibraries(ids: string[]) {
  const { data } = await api.post<{ deleted: number }>('/api/v1/admin/external-glossaries/bulk-delete', { ids })
  return data.deleted
}

export async function reorderExternalGlossaryLibraries(items: WorldKeywordReorderItem[]) {
  const { data } = await api.post<{ updated: number }>('/api/v1/admin/external-glossaries/reorder', { items })
  return data.updated
}

export async function importExternalGlossaryLibrary(payload: ExternalGlossaryLibraryImportPayload) {
  const { data } = await api.post<{ item: ExternalGlossaryLibraryItem; stats: { created: number; updated: number; skipped: number } }>(
    '/api/v1/admin/external-glossaries/import',
    payload,
  )
  return data
}

export async function exportExternalGlossaryLibrary(libraryId: string) {
  const { data } = await api.get<ExternalGlossaryLibraryExportPayload>(`/api/v1/admin/external-glossaries/${libraryId}/export`)
  return data
}

export async function fetchExternalGlossaryTerms(libraryId: string, params?: {
  page?: number
  pageSize?: number
  q?: string
  category?: string
  includeDisabled?: boolean
}) {
  const { data } = await api.get<ExternalGlossaryTermListResponse>(`/api/v1/admin/external-glossaries/${libraryId}/terms`, { params })
  return data
}

export async function createExternalGlossaryTerm(libraryId: string, payload: WorldKeywordPayload) {
  const { data } = await api.post<{ item: ExternalGlossaryTermItem }>(`/api/v1/admin/external-glossaries/${libraryId}/terms`, payload)
  return data.item
}

export async function updateExternalGlossaryTerm(libraryId: string, termId: string, payload: WorldKeywordPayload) {
  const { data } = await api.patch<{ item: ExternalGlossaryTermItem }>(`/api/v1/admin/external-glossaries/${libraryId}/terms/${termId}`, payload)
  return data.item
}

export async function deleteExternalGlossaryTerm(libraryId: string, termId: string) {
  await api.delete(`/api/v1/admin/external-glossaries/${libraryId}/terms/${termId}`)
}

export async function bulkDeleteExternalGlossaryTerms(libraryId: string, ids: string[]) {
  const { data } = await api.post<{ deleted: number }>(`/api/v1/admin/external-glossaries/${libraryId}/terms/bulk-delete`, { ids })
  return data.deleted
}

export async function reorderExternalGlossaryTerms(libraryId: string, items: WorldKeywordReorderItem[]) {
  const { data } = await api.post<{ updated: number }>(`/api/v1/admin/external-glossaries/${libraryId}/terms/reorder`, { items })
  return data.updated
}

export async function importExternalGlossaryTerms(libraryId: string, payload: { items: WorldKeywordPayload[]; replace?: boolean }) {
  const { data } = await api.post<{ stats: { created: number; updated: number; skipped: number } }>(
    `/api/v1/admin/external-glossaries/${libraryId}/terms/import`,
    payload,
  )
  return data.stats
}

export async function exportExternalGlossaryTerms(libraryId: string, category?: string) {
  const params = category ? { category } : undefined
  const { data } = await api.get<{ items: ExternalGlossaryTermItem[] }>(`/api/v1/admin/external-glossaries/${libraryId}/terms/export`, { params })
  return data.items
}

export async function fetchExternalGlossaryCategories(libraryId: string) {
  const { data } = await api.get<{ categories: string[] }>(`/api/v1/admin/external-glossaries/${libraryId}/categories`)
  return data.categories
}

export async function createExternalGlossaryCategory(libraryId: string, name: string) {
  const { data } = await api.post<{ name: string }>(`/api/v1/admin/external-glossaries/${libraryId}/categories`, { name })
  return data.name
}

export async function renameExternalGlossaryCategory(libraryId: string, oldName: string, newName: string) {
  const { data } = await api.post<{ updated: number; name: string }>(`/api/v1/admin/external-glossaries/${libraryId}/categories/rename`, { oldName, newName })
  return data
}

export async function deleteExternalGlossaryCategory(libraryId: string, name: string) {
  const { data } = await api.post<{ updated: number }>(`/api/v1/admin/external-glossaries/${libraryId}/categories/delete`, { name })
  return data.updated
}
