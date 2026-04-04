type ChannelTreeNode = {
  id?: string;
  children?: ChannelTreeNode[] | null;
};

type UnreadCountMap = Record<string, number>;

interface MessageNoticeUnreadArgs {
  channelId: string;
  currentChannelId?: string;
  unreadCountMap: UnreadCountMap;
  channelTree?: ChannelTreeNode[] | null;
  channelTreePrivate?: ChannelTreeNode[] | null;
}

const channelExistsInTree = (items: ChannelTreeNode[] | null | undefined, channelId: string): boolean => {
  if (!Array.isArray(items) || !channelId) {
    return false;
  }
  for (const item of items) {
    if (!item) {
      continue;
    }
    if (item.id === channelId) {
      return true;
    }
    if (channelExistsInTree(item.children || [], channelId)) {
      return true;
    }
  }
  return false;
};

export const resolveNextUnreadCountForMessageNotice = ({
  channelId,
  currentChannelId,
  unreadCountMap,
  channelTree,
  channelTreePrivate,
}: MessageNoticeUnreadArgs): number | null => {
  if (!channelId || currentChannelId === channelId) {
    return null;
  }
  const exists =
    channelExistsInTree(channelTree, channelId) || channelExistsInTree(channelTreePrivate, channelId);
  if (!exists) {
    return null;
  }
  return (unreadCountMap[channelId] || 0) + 1;
};

export const nextUnreadCountMapForMessageNotice = (args: MessageNoticeUnreadArgs): UnreadCountMap => {
  const nextCount = resolveNextUnreadCountForMessageNotice(args);
  if (nextCount === null) {
    return args.unreadCountMap;
  }
  return {
    ...args.unreadCountMap,
    [args.channelId]: nextCount,
  };
};
