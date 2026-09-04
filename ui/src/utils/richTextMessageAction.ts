import { createDiscreteApi } from 'naive-ui';
import type { Pinia } from 'pinia';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { isPrivateChatChannel } from './channelSendPermission';
import { MESSAGE_ACTION_DATA_ATTR } from './tiptap-message-action';

const WORLD_CONTEXT_ATTR = 'data-rich-message-world-id';
const CHANNEL_CONTEXT_ATTR = 'data-rich-message-channel-id';

let disposeInstalledRuntime: (() => void) | null = null;
const pendingActions = new WeakSet<HTMLElement>();

const readContext = (action: HTMLElement) => {
  const context = action.closest<HTMLElement>(`[${WORLD_CONTEXT_ATTR}], [${CHANNEL_CONTEXT_ATTR}]`);
  return {
    worldId: context?.getAttribute(WORLD_CONTEXT_ATTR)?.trim() || '',
    channelId: context?.getAttribute(CHANNEL_CONTEXT_ATTR)?.trim() || '',
  };
};

export const installRichTextMessageActionRuntime = ({ pinia }: { pinia: Pinia }) => {
  if (disposeInstalledRuntime || typeof document === 'undefined') {
    return disposeInstalledRuntime || (() => undefined);
  }

  const { message: notice } = createDiscreteApi(['message']);
  const chat = useChatStore(pinia);
  const user = useUserStore(pinia);

  const handleClick = async (event: MouseEvent) => {
    const target = event.target instanceof Element ? event.target : null;
    const action = target?.closest<HTMLElement>(`[${MESSAGE_ACTION_DATA_ATTR}="true"]`);
    if (!action || action.closest('.tiptap-editor') || pendingActions.has(action)) return;

    event.preventDefault();
    const content = action.getAttribute('data-message')?.trim() || '';
    if (!content) {
      notice.warning('消息按钮没有可发送内容');
      return;
    }
    if (content.length > 10000) {
      notice.error('消息过长，无法发送');
      return;
    }

    const currentWorldId = String(chat.currentWorldId || '').trim();
    const currentChannelId = String(chat.curChannel?.id || '').trim();
    const context = readContext(action);
    if (!currentWorldId || !currentChannelId
      || (context.worldId && context.worldId !== currentWorldId)
      || (context.channelId && context.channelId !== currentChannelId)) {
      notice.warning('当前页面没有有效频道上下文，无法发送');
      return;
    }
    if (chat.isObserver || chat.observerMode || !user.info.id) {
      notice.warning('当前用户不可发送消息');
      return;
    }
    if (chat.connectState !== 'connected') {
      notice.error('尚未连接，无法发送消息');
      return;
    }

    pendingActions.add(action);
    action.setAttribute('aria-disabled', 'true');
    try {
      const allowed = isPrivateChatChannel(chat.curChannel)
        || await chat.hasChannelPermission(currentChannelId, 'func_channel_text_send', user.info.id);
      if (!allowed) {
        notice.warning('当前频道不允许发送消息');
        return;
      }
      if (String(chat.currentWorldId || '').trim() !== currentWorldId
        || String(chat.curChannel?.id || '').trim() !== currentChannelId
        || chat.isObserver || chat.observerMode) {
        notice.warning('频道上下文已变化，消息未发送');
        return;
      }

      const identityId = chat.getActiveIdentityId(currentChannelId) || null;
      const identityVariantId = identityId
        ? chat.getActiveIdentityVariantId(currentChannelId, identityId)
        : undefined;
      const sent = await chat.messageCreate(
        content,
        undefined,
        undefined,
        undefined,
        identityId,
        undefined,
        [],
        undefined,
        undefined,
        identityVariantId,
        chat.icMode === 'ooc' ? 'ooc' : 'ic',
        currentChannelId,
      );
      if (!sent) throw new Error('message.create returned empty result');
    } catch (error) {
      notice.error(`发送失败：${error instanceof Error ? error.message : '未知错误'}`);
    } finally {
      pendingActions.delete(action);
      action.removeAttribute('aria-disabled');
    }
  };

  document.addEventListener('click', handleClick);
  disposeInstalledRuntime = () => {
    document.removeEventListener('click', handleClick);
    disposeInstalledRuntime = null;
  };
  return disposeInstalledRuntime;
};
