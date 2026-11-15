import { sanitizeWorkbenchLayout } from './layout'
import type { WorkbenchLayout } from './types'

// 三类状态的第三类：每个用户在本浏览器的本地显示状态（折叠、展开、移动端标签、已读聚焦、未保存草稿）。
// 布局定义与 GM 展示状态在服务端；这里的任何内容都不会影响其他成员。

const VIEW_PREFIX = 'sealchat:workbench-view:v1:'
const DRAFT_PREFIX = 'sealchat:workbench-draft:v1:'
const MAX_IDS = 64

export interface WorkbenchLocalViewState {
  collapsed: string[]
  focusedPanelId: string
  mobilePanelId: string
  /** 已在本地退出的 GM 聚焦（按 spotlight.at 识别，新的聚焦会再次显示）。 */
  dismissedSpotlightAt: number
}

export interface WorkbenchDraft {
  baseRevision: number
  savedAt: number
  layout: WorkbenchLayout
}

const storage = () => {
  try { return typeof window === 'undefined' ? null : window.localStorage } catch { return null }
}

const scopeKey = (userId: string, channelId: string) => `${encodeURIComponent(userId)}:${encodeURIComponent(channelId)}`

export const emptyWorkbenchViewState = (): WorkbenchLocalViewState => ({
  collapsed: [],
  focusedPanelId: '',
  mobilePanelId: '',
  dismissedSpotlightAt: 0,
})

export const readWorkbenchViewState = (userId: string, channelId: string): WorkbenchLocalViewState => {
  const fallback = emptyWorkbenchViewState()
  if (!userId || !channelId) return fallback
  try {
    const raw = JSON.parse(storage()?.getItem(VIEW_PREFIX + scopeKey(userId, channelId)) || 'null')
    if (!raw || typeof raw !== 'object') return fallback
    const ids = (value: unknown) => (Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string' && !!id).slice(0, MAX_IDS) : [])
    return {
      collapsed: ids(raw.collapsed),
      focusedPanelId: typeof raw.focusedPanelId === 'string' ? raw.focusedPanelId : '',
      mobilePanelId: typeof raw.mobilePanelId === 'string' ? raw.mobilePanelId : '',
      dismissedSpotlightAt: Number.isFinite(raw.dismissedSpotlightAt) ? Number(raw.dismissedSpotlightAt) : 0,
    }
  } catch {
    return fallback
  }
}

export const writeWorkbenchViewState = (userId: string, channelId: string, state: WorkbenchLocalViewState) => {
  if (!userId || !channelId) return
  try {
    storage()?.setItem(VIEW_PREFIX + scopeKey(userId, channelId), JSON.stringify({ ...state, collapsed: state.collapsed.slice(0, MAX_IDS) }))
  } catch {
    // 隐私模式或存储策略可能禁用持久化；本地显示状态丢失不影响功能。
  }
}

export const readWorkbenchDraft = (userId: string, channelId: string): WorkbenchDraft | null => {
  if (!userId || !channelId) return null
  try {
    const raw = JSON.parse(storage()?.getItem(DRAFT_PREFIX + scopeKey(userId, channelId)) || 'null')
    if (!raw || typeof raw !== 'object') return null
    return {
      baseRevision: Number(raw.baseRevision) || 0,
      savedAt: Number(raw.savedAt) || 0,
      layout: sanitizeWorkbenchLayout(raw.layout).layout,
    }
  } catch {
    return null
  }
}

export const writeWorkbenchDraft = (userId: string, channelId: string, draft: WorkbenchDraft | null) => {
  if (!userId || !channelId) return
  try {
    const key = DRAFT_PREFIX + scopeKey(userId, channelId)
    if (draft) storage()?.setItem(key, JSON.stringify(draft))
    else storage()?.removeItem(key)
  } catch {
    // 同上。
  }
}
