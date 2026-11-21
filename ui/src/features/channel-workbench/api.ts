import { api } from '@/stores/_config'
import { sanitizeWorkbenchLayout } from './layout'
import type { WorkbenchLayout, WorkbenchPresentation, WorkbenchPresentationInput, WorkbenchView } from './types'

// 布局频道 REST 契约的唯一归一化边界（api/channel_workbench.go）。

const encode = (channelId: string) => encodeURIComponent(channelId)

export class WorkbenchConflictError extends Error {
  constructor(readonly layoutRevision: number) {
    super('布局已被其他管理员更新')
  }
}

export const workbenchErrorMessage = (error: unknown, fallback: string) => {
  const data = (error as any)?.response?.data
  if (data && typeof data.message === 'string' && data.message) return data.message
  if (error instanceof Error && error.message && !(error as any).response) return error.message
  return fallback
}

const normalizePresentation = (value: unknown): WorkbenchPresentation => {
  const raw = value && typeof value === 'object' ? value as Record<string, any> : {}
  const visibility: Record<string, 'shown' | 'hidden'> = {}
  if (raw.visibility && typeof raw.visibility === 'object') {
    for (const [panelId, state] of Object.entries(raw.visibility)) {
      if (state === 'shown' || state === 'hidden') visibility[panelId] = state
    }
  }
  const spotlight = raw.spotlight && typeof raw.spotlight.panelId === 'string' && raw.spotlight.panelId
    ? { panelId: raw.spotlight.panelId, by: typeof raw.spotlight.by === 'string' ? raw.spotlight.by : undefined, at: Number(raw.spotlight.at) || 0 }
    : null
  return { version: Number(raw.version) || 1, visibility, spotlight }
}

const normalizeView = (value: unknown): WorkbenchView => {
  const raw = value && typeof value === 'object' ? value as Record<string, any> : {}
  // 服务端已严格校验；这里再走一遍宽松归一化，保证前端拿到的结构稳定。
  const { layout } = sanitizeWorkbenchLayout(raw.layout || {})
  return {
    channelId: String(raw.channelId || ''),
    worldId: String(raw.worldId || ''),
    displayMode: String(raw.displayMode || ''),
    canManage: raw.canManage === true,
    layout,
    layoutRevision: Number(raw.layoutRevision) || 0,
    presentation: normalizePresentation(raw.presentation),
    presentationRevision: Number(raw.presentationRevision) || 0,
    updatedAt: typeof raw.updatedAt === 'string' ? raw.updatedAt : undefined,
  }
}

export const fetchWorkbench = async (channelId: string, signal?: AbortSignal) => {
  const { data } = await api.get(`api/v1/channels/${encode(channelId)}/workbench`, { signal })
  return normalizeView(data)
}

export const saveWorkbenchLayout = async (channelId: string, layout: WorkbenchLayout, baseRevision: number) => {
  try {
    const { data } = await api.put(`api/v1/channels/${encode(channelId)}/workbench/layout`, { layout, baseRevision })
    return normalizeView(data)
  } catch (error) {
    const response = (error as any)?.response
    if (response?.status === 409) throw new WorkbenchConflictError(Number(response.data?.layoutRevision) || 0)
    throw error
  }
}

export const setChannelDisplayMode = async (channelId: string, displayMode: '' | 'workbench') => {
  const { data } = await api.put(`api/v1/channels/${encode(channelId)}/workbench/mode`, { displayMode })
  return { channelId: String(data?.channelId || channelId), displayMode: String(data?.displayMode || '') }
}

export const applyWorkbenchPresentation = async (channelId: string, input: WorkbenchPresentationInput) => {
  const { data } = await api.post(`api/v1/channels/${encode(channelId)}/workbench/presentation`, input)
  return { presentationRevision: Number(data?.presentationRevision) || 0 }
}
