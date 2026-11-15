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
  /** 布局频道的临时弹出窗口：不持久化，刷新或切换频道后消失。 */
  transient?: boolean
}

/** key/transient 只由布局频道使用：固定 key 让同一面板只对应一个浮窗。 */
export type TheaterFloatingCustomWindowInput = ({
  source: 'web'
  title: string
  url: string
  width?: number
  height?: number
} | {
  source: 'chat'
  title: string
  targetChannelId: string
}) & { key?: string; transient?: boolean }

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

/** 浮窗 / 布局频道共用的 Embed Chat 地址：普通 embed 端点，不接管音频，不携带桥接能力。 */
export const buildEmbedChatWindowUrl = (worldId: string, channelId: string, floatingChat = true) => {
  const url = new URL(window.location.href)
  const params = new URLSearchParams({ worldId, channelId, scopeWorldId: worldId, viewport: 'mobile', audioOwner: '0', toolbar: '1' })
  if (floatingChat) params.set('floatingChat', '1')
  url.hash = `/embed?${params}`
  return url.toString()
}
