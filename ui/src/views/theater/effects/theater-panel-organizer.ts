export type TheaterPanelDomain = 'audio' | 'effect'

export interface TheaterPanelFolder {
  id: string
  roomId: string
  domain: TheaterPanelDomain
  name: string
  sortOrder: number
  collapsed: boolean
}

export interface TheaterPanelItem {
  id: string
  roomId: string
  domain: TheaterPanelDomain
  targetId: string
  folderId?: string
  sortOrder: number
}

export interface TheaterPanelOrganizerSnapshot {
  folders: TheaterPanelFolder[]
  items: TheaterPanelItem[]
}

export const emptyTheaterPanelOrganizer = (): TheaterPanelOrganizerSnapshot => ({ folders: [], items: [] })
