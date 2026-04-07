import { defineStore } from 'pinia'
import type { AnnouncementPayload, AnnouncementReminderScope } from '@/models/announcement'
import {
  ackWorldAnnouncement,
  createLobbyAnnouncement,
  createWorldAnnouncement,
  deleteLobbyAnnouncement,
  deleteWorldAnnouncement,
  fetchLobbyAnnouncements,
  fetchLobbyPendingAnnouncement,
  fetchWorldAnnouncements,
  fetchWorldPendingAnnouncement,
  markLobbyAnnouncementPopup,
  markWorldAnnouncementPopup,
  updateLobbyAnnouncement,
  updateWorldAnnouncement,
} from '@/models/announcement'

export const useAnnouncementStore = defineStore('announcement', () => {
  async function fetchWorldList(worldId: string, params?: { page?: number; pageSize?: number; includeAll?: boolean; includeArchived?: boolean }) {
    return fetchWorldAnnouncements(worldId, params)
  }

  async function fetchLobbyList(params?: { page?: number; pageSize?: number; includeAll?: boolean; includeArchived?: boolean; showInTicker?: boolean }) {
    return fetchLobbyAnnouncements(params)
  }

  async function createWorld(worldId: string, payload: AnnouncementPayload) {
    return createWorldAnnouncement(worldId, payload)
  }

  async function updateWorld(worldId: string, announcementId: string, payload: AnnouncementPayload) {
    return updateWorldAnnouncement(worldId, announcementId, payload)
  }

  async function removeWorld(worldId: string, announcementId: string) {
    return deleteWorldAnnouncement(worldId, announcementId)
  }

  async function createLobby(payload: AnnouncementPayload) {
    return createLobbyAnnouncement(payload)
  }

  async function updateLobby(announcementId: string, payload: AnnouncementPayload) {
    return updateLobbyAnnouncement(announcementId, payload)
  }

  async function removeLobby(announcementId: string) {
    return deleteLobbyAnnouncement(announcementId)
  }

  async function fetchWorldPending(worldId: string) {
    return fetchWorldPendingAnnouncement(worldId)
  }

  async function fetchLobbyPending(params?: { reminderScope?: AnnouncementReminderScope }) {
    return fetchLobbyPendingAnnouncement(params)
  }

  async function markWorldPopup(worldId: string, announcementId: string) {
    return markWorldAnnouncementPopup(worldId, announcementId)
  }

  async function markLobbyPopup(announcementId: string) {
    return markLobbyAnnouncementPopup(announcementId)
  }

  async function ackWorld(worldId: string, announcementId: string) {
    return ackWorldAnnouncement(worldId, announcementId)
  }

  return {
    fetchWorldList,
    fetchLobbyList,
    createWorld,
    updateWorld,
    removeWorld,
    createLobby,
    updateLobby,
    removeLobby,
    fetchWorldPending,
    fetchLobbyPending,
    markWorldPopup,
    markLobbyPopup,
    ackWorld,
  }
})
