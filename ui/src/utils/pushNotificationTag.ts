export const buildPushNotificationTag = (_channelId: string, messageId?: string): string | undefined => {
  const normalizedMessageId = String(messageId || '').trim();
  if (!normalizedMessageId) {
    return undefined;
  }
  return `sealchat-message-${normalizedMessageId}`;
};
