export const MESSAGE_SOUND_MODE_VALUES = ['off', 'away', 'world-other-channel'] as const

export type MessageSoundMode = typeof MESSAGE_SOUND_MODE_VALUES[number]

export const MESSAGE_SOUND_MODE_LABELS: Record<MessageSoundMode, string> = {
  off: '关闭',
  away: '离页时',
  'world-other-channel': '其他频道',
}

interface ChannelTreeNode {
  id?: string
  children?: ChannelTreeNode[]
}

interface ShouldPlayMessageSoundInput {
  mode: MessageSoundMode
  isSelf: boolean
  isAppFocused: boolean
  messageChannelId: string
  currentChannelId: string
  currentWorldChannels: ChannelTreeNode[]
}

const isChannelInTree = (tree: ChannelTreeNode[], channelId: string): boolean => {
  for (const item of tree) {
    if (item?.id === channelId) {
      return true
    }
    if (Array.isArray(item?.children) && item.children.length > 0 && isChannelInTree(item.children, channelId)) {
      return true
    }
  }
  return false
}

export const shouldPlayMessageSound = ({
  mode,
  isSelf,
  isAppFocused,
  messageChannelId,
  currentChannelId,
  currentWorldChannels,
}: ShouldPlayMessageSoundInput): boolean => {
  if (isSelf || !messageChannelId) {
    return false
  }

  if (mode === 'off') {
    return false
  }

  if (mode === 'away') {
    return !isAppFocused && !!currentChannelId && messageChannelId === currentChannelId
  }

  if (!currentChannelId || messageChannelId === currentChannelId) {
    return false
  }

  return isChannelInTree(currentWorldChannels, messageChannelId)
}
