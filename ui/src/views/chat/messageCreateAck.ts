export const shouldApplyMessageCreateAck = (
  requestChannelId: string,
  currentChannelId: string,
  stillPending: boolean,
) => Boolean(requestChannelId && requestChannelId === currentChannelId && stillPending);
