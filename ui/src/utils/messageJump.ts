import { nextTick } from 'vue'
import { chatEvent } from '@/stores/chat'
import type { BattleReportJumpTarget } from '@/types'

interface MessageJumpChat {
  currentWorldId?: string | null
  curChannel?: { id?: string | null } | null
  switchWorld: (worldId: string, options?: { force?: boolean }) => Promise<unknown>
  channelSwitchTo: (channelId: string) => Promise<boolean>
}

export async function navigateToMessageTarget(
  chat: MessageJumpChat,
  target: Pick<BattleReportJumpTarget, 'worldId' | 'channelId'> & Partial<Pick<BattleReportJumpTarget, 'messageId' | 'createdAt' | 'displayOrder'>>,
): Promise<boolean> {
  const worldId = String(target?.worldId || '').trim()
  const channelId = String(target?.channelId || '').trim()
  const messageId = String(target?.messageId || '').trim()
  if (!worldId || !channelId) {
    return false
  }

  if (String(chat.currentWorldId || '').trim() !== worldId) {
    try {
      await chat.switchWorld(worldId, { force: true })
    } catch {
      throw new Error('无法访问该世界')
    }
  }

  if (String(chat.curChannel?.id || '').trim() !== channelId) {
    const switched = await chat.channelSwitchTo(channelId)
    if (!switched) {
      throw new Error('无法访问该频道')
    }
  }

  await nextTick()
  if (messageId) {
    chatEvent.emit('search-jump', {
      messageId,
      channelId,
      displayOrder: target.displayOrder,
      createdAt: target.createdAt,
    })
  }
  return true
}
