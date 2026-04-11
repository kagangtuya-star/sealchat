import type { SChannel } from '../types';

type ChannelLike = Pick<Partial<SChannel>, 'id' | 'type' | 'isPrivate'> & {
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
