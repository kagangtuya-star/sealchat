import { Howl } from 'howler';
import SoundMessageCreated from '@/assets/message.mp3';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useDisplayStore } from '@/stores/display';
import { usePushNotificationStore } from '@/stores/pushNotification';
import { useUserStore } from '@/stores/user';
import { shouldPlayMessageSound } from '@/utils/messageSoundMode';

type MessageEventPayload = {
  channel?: { id?: string };
  channelId?: string;
  channel_id?: string;
  message?: {
    user?: { id?: string };
    user_id?: string;
  };
  user?: { id?: string };
};

const normalizeChannelId = (event?: MessageEventPayload): string => String(
  event?.channel?.id || event?.channelId || event?.channel_id || '',
).trim();

const resolveMessageSenderId = (event?: MessageEventPayload): string => String(
  event?.message?.user?.id
    || event?.message?.user_id
    || event?.user?.id
    || '',
).trim();

export const installMessageSoundNotifier = (): (() => void) => {
  const chat = useChatStore();
  const display = useDisplayStore();
  const pushStore = usePushNotificationStore();
  const user = useUserStore();
  const sound = new Howl({
    src: [SoundMessageCreated],
    html5: true,
  });

  const shouldPlay = (event?: MessageEventPayload) => {
    const messageChannelId = normalizeChannelId(event);
    if (!messageChannelId) {
      return false;
    }
    const senderId = resolveMessageSenderId(event);
    const currentUserId = String(user.info?.id || '').trim();
    return shouldPlayMessageSound({
      mode: display.settings.messageSoundMode,
      isSelf: !!senderId && !!currentUserId && senderId === currentUserId,
      isAppFocused: chat.isAppFocused,
      messageChannelId,
      currentChannelId: String(chat.curChannel?.id || '').trim(),
      currentWorldChannels: chat.currentWorldChannels || chat.channelTree || [],
      embedNotifyOwnerEnabled: pushStore.embedNotifyOwnerEnabled,
    });
  };

  const handleMessageCreated = (event?: MessageEventPayload) => {
    if (shouldPlay(event)) {
      sound.play();
    }
  };

  const handleMessageCreatedNotice = (event?: MessageEventPayload) => {
    if (shouldPlay(event)) {
      sound.play();
    }
  };

  chatEvent.on('message-created', handleMessageCreated as any);
  chatEvent.on('message-created-notice', handleMessageCreatedNotice as any);

  return () => {
    chatEvent.off('message-created', handleMessageCreated as any);
    chatEvent.off('message-created-notice', handleMessageCreatedNotice as any);
    sound.unload();
  };
};
