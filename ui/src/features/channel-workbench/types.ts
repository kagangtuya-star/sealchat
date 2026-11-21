import type { InternalSurfaceType } from '@/utils/internalSurfaceLink'

// 布局频道（工作台）协议类型，与 service/channel_workbench.go 保持一致。
// 布局只保存资源描述（resource descriptor），不保存生成后的 URL。

export const WORKBENCH_LAYOUT_VERSION = 1 as const
export const WORKBENCH_DISPLAY_MODE = 'workbench' as const

export type WorkbenchSurfaceType = InternalSurfaceType
export type WorkbenchSourceType = 'surface' | 'chat' | 'theater' | 'web'
export type WorkbenchMobileMode = '' | 'tabs' | 'stack'
export type WorkbenchPanelStyle = '' | 'flat' | 'outlined' | 'glass'
export type WorkbenchPanelChrome = '' | 'minimal'
export type WorkbenchMinimalActionsPlacement = '' | 'inside' | 'outside'
export type WorkbenchWebDisplay = '' | 'embed' | 'link'

export interface WorkbenchGrid {
  columns: 12 | 24
  rows: number
  gap: number
}

export interface WorkbenchSettings {
  mobileMode?: WorkbenchMobileMode
  panelStyle?: WorkbenchPanelStyle
  minimalActionsPlacement?: WorkbenchMinimalActionsPlacement
  accent?: string
}

export interface WorkbenchRect {
  x: number
  y: number
  w: number
  h: number
}

/** channelId 为空表示布局频道自身。 */
export interface WorkbenchSource {
  type: WorkbenchSourceType
  surface?: WorkbenchSurfaceType
  resourceId?: string
  channelId?: string
  label?: string
  url?: string
  display?: WorkbenchWebDisplay
}

export interface WorkbenchPanel {
  id: string
  title?: string
  rect: WorkbenchRect
  source: WorkbenchSource
  hiddenByDefault?: boolean
  chrome?: WorkbenchPanelChrome
}

export interface WorkbenchLayout {
  version: typeof WORKBENCH_LAYOUT_VERSION
  grid: WorkbenchGrid
  settings: WorkbenchSettings
  panels: WorkbenchPanel[]
}

export interface WorkbenchSpotlight {
  panelId: string
  by?: string
  at: number
}

export interface WorkbenchPresentation {
  version: number
  /** 只有管理者能拿到；玩家收到的布局已在服务端过滤掉隐藏面板。 */
  visibility?: Record<string, 'shown' | 'hidden'>
  spotlight?: WorkbenchSpotlight | null
}

export interface WorkbenchView {
  channelId: string
  worldId: string
  displayMode: string
  canManage: boolean
  layout: WorkbenchLayout
  layoutRevision: number
  presentation: WorkbenchPresentation
  presentationRevision: number
  updatedAt?: string
}

export type WorkbenchPresentationInput =
  | { action: 'visibility'; panelId: string; visibility: 'shown' | 'hidden' | 'default' }
  | { action: 'spotlight'; panelId: string; reveal?: boolean }
  | { action: 'clear-spotlight' }
  | { action: 'reset' }

export interface WorkbenchUpdatedEventPayload {
  channelId: string
  layoutRevision?: number
  presentationRevision?: number
  reason?: 'layout' | 'presentation' | string
  operatorId?: string
}
