import { api } from '@/stores/_config'
import { useCharacterCardStore } from '@/stores/characterCard'
import { useChannelCharacterSnapshotStore } from '@/stores/channelCharacterSnapshot'
import type { WorkbenchSurfaceType } from './types'

// 设计模式的资源目录：只读。列表接口直接读取，不调用会改写运行时窗口状态的 Store 加载器
// （例如 iform.ensureForms 会清理/接管共享窗口）。

export interface WorkbenchResourceOption {
  value: string
  label: string
  hint?: string
}

export interface WorkbenchResourceContext {
  worldId: string
  channelId: string
  characterSource?: WorkbenchCharacterSource
}

export type WorkbenchCharacterSource = 'snapshot' | 'personal'

const items = (data: any): any[] => (Array.isArray(data?.items) ? data.items : [])
const label = (value: unknown, fallback: string) => (typeof value === 'string' && value.trim() ? value.trim() : fallback)

const loadCharacters = async (context: WorkbenchResourceContext): Promise<WorkbenchResourceOption[]> => {
  if (context.characterSource === 'personal') {
    const cards = useCharacterCardStore()
    // 人物卡列表来自所选频道的角色卡 API，不能复用其他频道留下的全局列表。
    await cards.loadCardList(context.channelId, { throwOnError: true })
    if (cards.isBotCharacterDisabled(context.channelId)) {
      throw new Error(cards.getCharacterApiDisabledReason(context.channelId) || '当前频道的人物卡 API 不可用')
    }
    return cards.cardList.map(card => ({ value: card.id, label: label(card.name, card.id), hint: '仅自己可见' }))
  }

  const snapshots = useChannelCharacterSnapshotStore()
  await snapshots.refreshChannel(context.channelId)
  return snapshots.getChannelItems(context.channelId)
    .filter(item => !!item.data.card)
    .sort((a, b) => (b.lastSeenAt || b.sourceUpdatedAt || 0) - (a.lastSeenAt || a.sourceUpdatedAt || 0))
    .map(item => {
      const name = label(item.data?.card?.name, '') || label(item.data?.identity?.displayName, item.identityId)
      const identity = label(item.data.identity.displayName, item.identityId)
      const timestamp = item.lastSeenAt || item.sourceUpdatedAt || 0
      const updatedAt = new Date(timestamp < 1_000_000_000_000 ? timestamp * 1000 : timestamp)
      const updatedLabel = timestamp && !Number.isNaN(updatedAt.getTime()) ? updatedAt.toLocaleString() : '未知'
      return { value: `snapshot:${context.channelId}:${item.identityId}`, label: name, hint: `${identity} · 最近同步 ${updatedLabel}` }
    })
}

/** 返回可绑定资源；接口失败时抛错，由调用方提示并允许手动输入资源 ID。 */
export const loadWorkbenchResources = async (surface: WorkbenchSurfaceType, context: WorkbenchResourceContext): Promise<WorkbenchResourceOption[]> => {
  const channel = encodeURIComponent(context.channelId)
  const world = encodeURIComponent(context.worldId)
  switch (surface) {
    case 'clue-board':
      return [{ value: 'main', label: '世界线索画板' }]
    case 'iform': {
      const { data } = await api.get(`api/v1/channels/${channel}/iforms`)
      return items(data).map(item => ({ value: String(item.id), label: label(item.name, String(item.id)) }))
    }
    case 'note': {
      const { data } = await api.get(`api/v1/channels/${channel}/sticky-notes`)
      return items(data).map(item => ({ value: String(item.id), label: label(item.title, '未命名便签'), hint: item.isPublic ? '公开' : '私有' }))
    }
    case 'clue': {
      const { data } = await api.get(`api/v1/worlds/${world}/clues`)
      return items(data).map(item => ({ value: String(item.id), label: label(item.title, '未命名线索') }))
    }
    case 'character':
      return loadCharacters(context)
    default:
      return []
  }
}
