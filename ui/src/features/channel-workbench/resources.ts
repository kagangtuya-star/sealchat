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
}

const items = (data: any): any[] => (Array.isArray(data?.items) ? data.items : [])
const label = (value: unknown, fallback: string) => (typeof value === 'string' && value.trim() ? value.trim() : fallback)

const loadCharacters = async (context: WorkbenchResourceContext): Promise<WorkbenchResourceOption[]> => {
  const snapshots = useChannelCharacterSnapshotStore()
  const cards = useCharacterCardStore()
  const [snapshotResult] = await Promise.allSettled([
    snapshots.refreshChannel(context.channelId),
    cards.cardList.length ? Promise.resolve() : cards.loadCardList(context.channelId),
  ])
  const options: WorkbenchResourceOption[] = []
  if (snapshotResult.status === 'fulfilled') {
    for (const item of snapshots.getChannelItems(context.channelId)) {
      const name = label(item.data?.card?.name, '') || label(item.data?.identity?.displayName, item.identityId)
      // 频道快照对频道成员可读（只读），适合角色卡墙展示其他玩家的人物卡。
      options.push({ value: `snapshot:${context.channelId}:${item.identityId}`, label: name, hint: '频道在线快照（成员可见，只读）' })
    }
  }
  for (const card of cards.cardList) {
    options.push({ value: card.id, label: label(card.name, card.id), hint: '我的人物卡（仅自己可见）' })
  }
  return options
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
