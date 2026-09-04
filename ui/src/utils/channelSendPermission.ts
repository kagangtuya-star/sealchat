export const isPrivateChatChannel = (channel: unknown): boolean => {
  if (!channel || typeof channel !== 'object') return false;
  const value = channel as Record<string, any>;
  if (value.isPrivate || value.friendInfo) return true;
  if (typeof value.permType === 'string' && value.permType.toLowerCase() === 'private') return true;
  return typeof value.type === 'number' && value.type === 3;
};
