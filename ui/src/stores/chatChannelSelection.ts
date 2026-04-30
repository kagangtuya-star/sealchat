type ChannelLike = {
  id?: string;
  type?: number;
  isPrivate?: boolean;
  status?: string;
  children?: ChannelLike[];
};

export const isEnterableChannel = (channel?: ChannelLike | null) => {
  if (!channel?.id) {
    return false;
  }
  return channel.type === 3 || channel.isPrivate === true;
};

export const findChannelByIdFromTree = <T extends ChannelLike>(nodes: T[] = [], channelId: string): T | null => {
  if (!channelId) {
    return null;
  }
  for (const node of nodes) {
    if (!node) {
      continue;
    }
    if (node.id === channelId) {
      return node;
    }
    const found = findChannelByIdFromTree((node.children || []) as T[], channelId);
    if (found) {
      return found;
    }
  }
  return null;
};

export const isDeletedChannelForAccess = (channel?: ChannelLike | null) => (
  String(channel?.status || '').trim().toLowerCase() === 'deleted'
);

export const findFirstEnterableChannel = <T extends ChannelLike>(nodes: T[] = []): T | null => {
  for (const node of nodes) {
    if (!node) {
      continue;
    }
    if (isEnterableChannel(node)) {
      return node;
    }
    const found = findFirstEnterableChannel((node.children || []) as T[]);
    if (found) {
      return found;
    }
  }
  return null;
};

export const findFirstEnterableChannelExcept = <T extends ChannelLike>(
  nodes: T[] = [],
  excludedChannelId = '',
): T | null => {
  for (const node of nodes) {
    if (!node) {
      continue;
    }
    if (node.id !== excludedChannelId && isEnterableChannel(node)) {
      return node;
    }
    const found = findFirstEnterableChannelExcept((node.children || []) as T[], excludedChannelId);
    if (found) {
      return found;
    }
  }
  return null;
};

export const resolveDeletedChannelFallbackId = (options: {
  deletedChannelId?: string;
  currentChannelId?: string;
  channelTree?: ChannelLike[];
}) => {
  const deletedChannelId = String(options.deletedChannelId || '').trim();
  const currentChannelId = String(options.currentChannelId || '').trim();
  if (!deletedChannelId || deletedChannelId !== currentChannelId) {
    return '';
  }
  return findFirstEnterableChannelExcept(options.channelTree || [], deletedChannelId)?.id || '';
};

export const shouldRenderChannelSidebarList = (options: {
  currentWorldId?: string;
  channelTree?: ChannelLike[];
  channelTreeReady?: Record<string, boolean>;
}) => {
  if ((options.channelTree || []).length > 0) {
    return true;
  }
  const currentWorldId = String(options.currentWorldId || '').trim();
  return !!(currentWorldId && options.channelTreeReady?.[currentWorldId]);
};
