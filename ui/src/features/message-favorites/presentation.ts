import dayjs from 'dayjs'
import type { Message } from '@satorijs/protocol'
import {
  renderMessageContent,
  resolveAvatarSource,
  resolveSenderColor,
  resolveSenderName,
} from '@/views/chat/components/message-image/messageImageSnapshot'
import type { MessageFavoriteBatchResult, MessageFavoriteItem, MessageFavoriteScope } from './api'

// 收藏卡片的展示模型。正文复用消息快照的安全渲染路径（tiptap/legacy/bot 命令 + DOMPurify），
// 不在收藏模块另写解析器。

export interface MessageFavoriteCardView {
  senderName: string
  senderColor?: string
  avatarUrl: string
  timeLabel: string
  timeTitle: string
  contentHtml: string
  isWhisper: boolean
  isOoc: boolean
  isEdited: boolean
}

export const buildFavoriteCardView = (message: Message, currentUserId: string): MessageFavoriteCardView => {
  const raw = message as any
  const createdAt = Number(raw?.createdAt || 0)
  const time = createdAt ? dayjs(createdAt) : null
  const sameYear = time ? time.year() === dayjs().year() : true
  return {
    senderName: resolveSenderName(message),
    senderColor: resolveSenderColor(message),
    avatarUrl: resolveAvatarSource(message),
    timeLabel: time ? time.format(sameYear ? 'MM-DD HH:mm' : 'YYYY-MM-DD HH:mm') : '',
    timeTitle: time ? time.format('YYYY-MM-DD HH:mm:ss') : '',
    contentHtml: renderMessageContent(message, { currentUserId }),
    isWhisper: Boolean(raw?.isWhisper),
    isOoc: String(raw?.icMode || 'ic').toLowerCase() === 'ooc',
    isEdited: Boolean(raw?.isEdited),
  }
}

export const canJumpToFavorite = (item: MessageFavoriteItem) => (
  item.status === 'available' && !!item.message && !!item.worldId && !!item.channelId && !!item.messageId
)

const REJECT_LABELS: Record<string, string> = {
  whisper_not_allowed: '悄悄话',
  not_visible: '无权查看',
  wrong_world: '不属于当前世界',
  not_found: '已删除或不存在',
}

export const describeFavoriteRejectReason = (reason: string) => REJECT_LABELS[reason] || '无法收藏'

export const favoriteScopeLabel = (scope: MessageFavoriteScope) => (scope === 'world' ? '世界收藏' : '个人收藏')

// 批量结果合并成一条反馈：区分成功、已存在和未收藏，避免连续弹多条通知。
export const summarizeFavoriteBatch = (scope: MessageFavoriteScope, result: MessageFavoriteBatchResult) => {
  const created = result.created.length
  const existing = result.existing.length
  const rejected = result.rejected.length
  const target = favoriteScopeLabel(scope)
  const parts: string[] = []
  if (created) parts.push(`已收藏 ${created} 条`)
  if (existing) parts.push(created || rejected ? `${existing} 条已在${target}中` : `已在${target}中`)
  if (rejected) {
    const counts = new Map<string, number>()
    for (const item of result.rejected) {
      const label = describeFavoriteRejectReason(item.reason)
      counts.set(label, (counts.get(label) || 0) + 1)
    }
    const reasons = Array.from(counts, ([label, count]) => (rejected > 1 ? `${label} ×${count}` : label)).join('、')
    parts.push(`${rejected} 条未收藏（${reasons}）`)
  }
  const type: 'success' | 'info' | 'warning' | 'error' = rejected
    ? (created || existing ? 'warning' : 'error')
    : (created ? 'success' : 'info')
  const prefix = created && !rejected ? `${target}：` : ''
  return { type, text: `${prefix}${parts.join('，')}` }
}
