import type { InternalSurfaceType } from '@/utils/internalSurfaceLink'

export type TheaterFloatingWindowSource = 'internal' | 'web' | 'chat'

export interface TheaterFloatingWindowSummary {
  id: string
  key: string
  title: string
  source: TheaterFloatingWindowSource
  resourceType?: InternalSurfaceType
  targetChannelId?: string
  width: number
  height: number
  url?: string
  hidden: boolean
  minimized: boolean
  zIndex: number
}

export type TheaterFloatingCustomWindowInput = {
  source: 'web'
  title: string
  url: string
  width?: number
  height?: number
} | {
  source: 'chat'
  title: string
  targetChannelId: string
}

export type TheaterFloatingCustomWindowUpdate = {
  source: 'web'
  id: string
  title: string
  url: string
  width?: number
  height?: number
} | {
  source: 'chat'
  id: string
  title: string
  targetChannelId: string
}

export type TheaterFloatingWindowAction =
  | { type: 'focus' | 'toggle-hidden' | 'toggle-minimized' | 'close'; id: string }
  | { type: 'close-all' }
  | { type: 'add-web'; title: string; url: string; width?: number; height?: number }
  | { type: 'add-chat'; channelId: string }
  | { type: 'update-web'; id: string; title: string; url: string; width?: number; height?: number }
  | { type: 'update-chat'; id: string; channelId: string }
