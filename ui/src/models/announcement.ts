import { api } from '@/stores/_config'

export type AnnouncementScopeType = 'world' | 'lobby'
export type AnnouncementStatus = 'draft' | 'published' | 'archived'
export type AnnouncementPopupMode = 'none' | 'once_per_version' | 'every_entry'
export type AnnouncementContentFormat = 'plain' | 'rich'
export type AnnouncementReminderScope = 'lobby_only' | 'site_wide'

export interface AnnouncementItem {
  id: string
  createdAt: string
  updatedAt: string
  scopeType: AnnouncementScopeType
  scopeId: string
  title: string
  content: string
  contentFormat: AnnouncementContentFormat
  status: AnnouncementStatus
  isPinned: boolean
  pinOrder: number
  popupMode: AnnouncementPopupMode
  reminderScope: AnnouncementReminderScope
  requireAck: boolean
  version: number
  publishedAt?: string | null
  createdBy: string
  updatedBy: string
  creatorName?: string
  updaterName?: string
  lastSeenVersion: number
  ackVersion: number
  ackCount: number
  isAcked: boolean
  needsAck: boolean
  canEdit: boolean
}

export interface AnnouncementListResponse {
  items: AnnouncementItem[]
  total: number
  page: number
  pageSize: number
}

export interface AnnouncementPayload {
  title: string
  content: string
  contentFormat: AnnouncementContentFormat
  status: AnnouncementStatus
  isPinned: boolean
  pinOrder: number
  popupMode: AnnouncementPopupMode
  reminderScope: AnnouncementReminderScope
  requireAck: boolean
}

export async function fetchWorldAnnouncements(worldId: string, params?: { page?: number; pageSize?: number; includeAll?: boolean; includeArchived?: boolean }) {
  const { data } = await api.get<AnnouncementListResponse>(`/api/v1/worlds/${worldId}/announcements`, { params })
  return data
}

export async function createWorldAnnouncement(worldId: string, payload: AnnouncementPayload) {
  const { data } = await api.post<{ item: AnnouncementItem }>(`/api/v1/worlds/${worldId}/announcements`, payload)
  return data.item
}

export async function updateWorldAnnouncement(worldId: string, announcementId: string, payload: AnnouncementPayload) {
  const { data } = await api.patch<{ item: AnnouncementItem }>(`/api/v1/worlds/${worldId}/announcements/${announcementId}`, payload)
  return data.item
}

export async function deleteWorldAnnouncement(worldId: string, announcementId: string) {
  await api.delete(`/api/v1/worlds/${worldId}/announcements/${announcementId}`)
}

export async function fetchWorldPendingAnnouncement(worldId: string) {
  const { data } = await api.get<{ item: AnnouncementItem | null }>(`/api/v1/worlds/${worldId}/announcements/pending-popup`)
  return data.item
}

export async function markWorldAnnouncementPopup(worldId: string, announcementId: string) {
  const { data } = await api.post<{ item: AnnouncementItem }>(`/api/v1/worlds/${worldId}/announcements/${announcementId}/mark-popup`)
  return data.item
}

export async function ackWorldAnnouncement(worldId: string, announcementId: string) {
  const { data } = await api.post<{ item: AnnouncementItem }>(`/api/v1/worlds/${worldId}/announcements/${announcementId}/ack`)
  return data.item
}

export async function fetchLobbyAnnouncements(params?: { page?: number; pageSize?: number; includeAll?: boolean; includeArchived?: boolean }) {
  const { data } = await api.get<AnnouncementListResponse>('/api/v1/lobby-announcements', { params })
  return data
}

export async function fetchLobbyPendingAnnouncement(params?: { reminderScope?: AnnouncementReminderScope }) {
  const { data } = await api.get<{ item: AnnouncementItem | null }>('/api/v1/lobby-announcements/pending-popup', { params })
  return data.item
}

export async function markLobbyAnnouncementPopup(announcementId: string) {
  const { data } = await api.post<{ item: AnnouncementItem }>(`/api/v1/lobby-announcements/${announcementId}/mark-popup`)
  return data.item
}

export async function createLobbyAnnouncement(payload: AnnouncementPayload) {
  const { data } = await api.post<{ item: AnnouncementItem }>('/api/v1/admin/lobby-announcements', payload)
  return data.item
}

export async function updateLobbyAnnouncement(announcementId: string, payload: AnnouncementPayload) {
  const { data } = await api.patch<{ item: AnnouncementItem }>(`/api/v1/admin/lobby-announcements/${announcementId}`, payload)
  return data.item
}

export async function deleteLobbyAnnouncement(announcementId: string) {
  await api.delete(`/api/v1/admin/lobby-announcements/${announcementId}`)
}
