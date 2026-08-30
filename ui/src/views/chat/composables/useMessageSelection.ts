import { computed, onBeforeUnmount, ref, type Ref } from 'vue';
import dayjs from 'dayjs';
import { nanoid } from 'nanoid';
import type { Message } from '@satorijs/protocol';
import { chatEvent, useChatStore } from '@/stores/chat';
import { copyTextWithFallback } from '@/utils/clipboard';
import { dialogAskConfirm } from '@/utils/dialog';

interface MessageSelectionOptions {
  chat: ReturnType<typeof useChatStore>;
  rows: Ref<Message[]>;
  pinnedRows: Ref<Message[]>;
  message: any;
  dialog: any;
}

interface MessageForwardPayload {
  sourceChannelId?: string;
  sourceWorldId?: string;
  messageIds?: string[];
  messages?: any[];
}

export const useMessageSelection = ({
  chat,
  rows,
  pinnedRows,
  message,
  dialog,
}: MessageSelectionOptions) => {
  const forwardDialogVisible = ref(false);
  const forwardDialogSourceChannelId = ref('');
  const forwardDialogSourceWorldId = ref('');
  const forwardDialogMessageIds = ref<string[]>([]);
  const forwardDialogMessages = ref<any[]>([]);

  const allMessageIds = computed(() => {
    const ids = new Set<string>();
    for (const row of rows.value) {
      if (row.id) ids.add(row.id);
    }
    for (const row of pinnedRows.value) {
      if (row.id) ids.add(row.id);
    }
    return Array.from(ids);
  });

  const getMultiSelectedMessages = () => {
    if (!chat.multiSelect?.selectedIds.size) return [];
    const selected = Array.from(chat.multiSelect.selectedIds);
    return rows.value.filter((row): row is Message & { id: string } => {
      const id = row.id;
      return typeof id === 'string' && id.length > 0 && selected.includes(id);
    });
  };

  const getMultiSelectedMessageIdsInDisplayOrder = () => {
    const selected = chat.multiSelect?.selectedIds;
    if (!selected?.size) return [];
    return rows.value
      .map((row) => row.id)
      .filter((id): id is string => typeof id === 'string' && id.length > 0 && selected.has(id));
  };

  const openMessageForwardDialog = (payload: MessageForwardPayload) => {
    const channelId = String(payload.sourceChannelId || chat.curChannel?.id || '').trim();
    const ids = Array.from(new Set((payload.messageIds || []).map((id) => String(id || '').trim()).filter(Boolean)));
    if (!channelId || ids.length === 0) {
      message.warning('请先选择消息');
      return;
    }
    forwardDialogSourceChannelId.value = channelId;
    forwardDialogSourceWorldId.value = String(payload.sourceWorldId || chat.currentWorldId || '').trim();
    forwardDialogMessageIds.value = ids;
    forwardDialogMessages.value = Array.isArray(payload.messages) ? payload.messages : [];
    forwardDialogVisible.value = true;
  };

  const handleMessageForwardOpen = (payload?: any) => {
    openMessageForwardDialog(payload || {});
  };
  chatEvent.on('message-forward-open' as any, handleMessageForwardOpen as any);
  onBeforeUnmount(() => {
    chatEvent.off('message-forward-open' as any, handleMessageForwardOpen as any);
  });

  const handleMultiSelectForward = () => {
    const messageIds = getMultiSelectedMessageIdsInDisplayOrder();
    if (!messageIds.length) {
      message.warning('请先选择消息');
      return;
    }
    const selected = getMultiSelectedMessages();
    openMessageForwardDialog({
      sourceChannelId: chat.curChannel?.id || '',
      sourceWorldId: chat.currentWorldId,
      messageIds,
      messages: selected,
    });
  };

  const handleMessageForwardSuccess = () => {
    if (chat.multiSelect?.active) chat.exitMultiSelectMode();
  };

  const handleMultiSelectCopy = async () => {
    const messages = getMultiSelectedMessages();
    if (!messages.length) {
      message.warning('请先选择消息');
      return;
    }
    const text = messages.map((msg) => {
      const time = msg.createdAt ? dayjs(msg.createdAt).format('YYYY-MM-DD HH:mm:ss') : '';
      const name = (msg as any).sender_member_name || (msg as any).identity?.displayName || (msg as any).member?.nick || (msg as any).user?.name || '未知';
      const content = typeof msg.content === 'string' ? msg.content.replace(/<[^>]*>/g, '') : '';
      return `[${time}] ${name}: ${content}`;
    }).join('\n');
    const copied = await copyTextWithFallback(text);
    if (copied) {
      message.success(`已复制 ${messages.length} 条消息`);
      chat.exitMultiSelectMode();
    } else {
      message.error('复制失败');
    }
  };

  const handleMultiSelectArchive = async () => {
    const ids = Array.from(chat.multiSelect?.selectedIds || []);
    if (!ids.length) {
      message.warning('请先选择消息');
      return;
    }
    const confirmed = await dialogAskConfirm(
      dialog,
      '批量归档',
      `确定归档选中的 ${ids.length} 条消息吗？归档后可在归档管理中查看或恢复。`,
    );
    if (!confirmed) return;
    try {
      await chat.archiveMessages(ids);
      message.success(`已归档 ${ids.length} 条消息`);
      chat.exitMultiSelectMode();
    } catch (e) {
      message.error('归档失败');
    }
  };

  const handleMultiSelectDelete = async () => {
    const ids = Array.from(chat.multiSelect?.selectedIds || []);
    if (!ids.length) {
      message.warning('请先选择消息');
      return;
    }
    const channelId = chat.curChannel?.id;
    if (!channelId) {
      message.error('当前频道不可用');
      return;
    }
    dialog.warning({
      title: '批量删除',
      content: `确定要删除选中的 ${ids.length} 条消息吗？此操作不可撤销。`,
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          await chat.removeMessages(ids);
          message.success(`已删除 ${ids.length} 条消息`);
          chat.exitMultiSelectMode();
        } catch (e) {
          message.error('删除失败');
        }
      },
    });
  };

  const handleMultiSelectCopyImage = async () => {
    const messages = getMultiSelectedMessages();
    if (!messages.length) {
      message.warning('请先选择消息');
      return;
    }
    try {
      const html2canvas = (await import('html2canvas')).default;
      const messageEls: HTMLElement[] = [];
      for (const msg of messages) {
        const el = msg.id ? document.getElementById(msg.id) : null;
        if (el) messageEls.push(el);
      }
      if (!messageEls.length) {
        message.error('未找到消息元素');
        return;
      }
      const rootStyles = getComputedStyle(document.documentElement);
      const bgColor = rootStyles.getPropertyValue('--sc-bg-base')?.trim()
        || rootStyles.getPropertyValue('--chat-bg')?.trim()
        || getComputedStyle(document.body).backgroundColor
        || '#ffffff';
      const canvases: HTMLCanvasElement[] = [];
      for (const el of messageEls) {
        const canvas = await html2canvas(el, {
          backgroundColor: bgColor,
          scale: 2,
          useCORS: true,
          allowTaint: true,
          logging: false,
          onclone: (_clonedDoc, clonedEl) => {
            clonedEl.classList.remove('chat-item--multiselect', 'chat-item--selected');
            const checkbox = clonedEl.querySelector('.chat-item__select-checkbox');
            if (checkbox) checkbox.remove();
          },
        });
        canvases.push(canvas);
      }
      const totalHeight = canvases.reduce((sum, canvas) => sum + canvas.height, 0);
      const maxWidth = Math.max(...canvases.map((canvas) => canvas.width));
      const padding = 16 * 2;
      const combinedCanvas = document.createElement('canvas');
      combinedCanvas.width = maxWidth + padding * 2;
      combinedCanvas.height = totalHeight + padding * 2;
      const ctx = combinedCanvas.getContext('2d')!;
      ctx.fillStyle = bgColor;
      ctx.fillRect(0, 0, combinedCanvas.width, combinedCanvas.height);
      let y = padding;
      for (const canvas of canvases) {
        ctx.drawImage(canvas, padding, y);
        y += canvas.height;
      }
      combinedCanvas.toBlob(async (blob) => {
        if (!blob) return;
        try {
          await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
          message.success('已复制为图片');
          chat.exitMultiSelectMode();
        } catch (e) {
          message.error('复制图片失败');
        }
      }, 'image/png');
    } catch (e) {
      console.error(e);
      message.error('生成图片失败');
    }
  };

  const handleMultiSelectMoveToBottom = async () => {
    const messages = getMultiSelectedMessages();
    if (!messages.length) {
      message.warning('请先选择消息');
      return;
    }
    const channelId = chat.curChannel?.id;
    if (!channelId) {
      message.error('当前频道不可用');
      return;
    }
    const messageIds = messages.map((msg) => msg.id).filter((id): id is string => Boolean(id));
    if (!messageIds.length) {
      message.warning('请先选择消息');
      return;
    }
    const confirmed = await dialogAskConfirm(
      dialog,
      '批量置底',
      `确定将选中的 ${messageIds.length} 条消息置底吗？原有相对顺序会保持不变。`,
    );
    if (!confirmed) return;
    try {
      await chat.messageReorderBatch(channelId, { messageIds, clientOpId: nanoid() });
      message.success(`已置底 ${messageIds.length} 条消息`);
      chat.exitMultiSelectMode();
    } catch (error) {
      message.error((error as Error)?.message || '置底失败');
    }
  };

  const handleMultiSelectRelocate = () => {
    const messageIds = getMultiSelectedMessageIdsInDisplayOrder();
    if (!messageIds.length) {
      message.warning('请先选择消息');
      return;
    }
    chat.startMultiSelectRelocate(messageIds);
  };

  const handleCancelMultiSelectRelocate = () => chat.cancelMultiSelectRelocate();

  const handleRelocateTargetPick = (messageId: string) => {
    const relocate = chat.multiSelect?.relocate;
    if (!relocate?.active) return;
    const targetId = String(messageId || '').trim();
    if (!targetId) return;
    if (relocate.sourceMessageIds.includes(targetId)) {
      message.warning('不能定位到已选消息内部');
      return;
    }
    const channelId = chat.curChannel?.id;
    if (!channelId) {
      message.error('当前频道不可用');
      return;
    }
    chat.setMultiSelectRelocateTarget(targetId);
    const targetMessage = rows.value.find((row) => row.id === targetId);
    const raw = typeof targetMessage?.content === 'string' ? targetMessage.content.replace(/<[^>]*>/g, '').trim() : '';
    const targetSummary = raw ? (raw.length > 32 ? `${raw.slice(0, 32)}...` : raw) : '该消息';
    const messageIds = relocate.sourceMessageIds.slice();
    dialog.warning({
      title: '批量重定位',
      content: `确定将选中的 ${messageIds.length} 条消息移动到“${targetSummary}”下方吗？`,
      positiveText: '移动',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          await chat.messageRelocateBatch(channelId, {
            messageIds,
            targetMessageId: targetId,
            placement: 'after',
            clientOpId: nanoid(),
          });
          message.success(`已重定位 ${messageIds.length} 条消息`);
          chat.exitMultiSelectMode();
        } catch (error) {
          message.error((error as Error)?.message || '重定位失败');
        }
      },
    });
  };

  const handleMultiSelectAll = () => {
    const allIds = rows.value.map((row) => row.id).filter((id): id is string => Boolean(id));
    chat.selectMessagesByIds(allIds);
    message.info(`已选中 ${allIds.length} 条消息`);
  };

  return {
    allMessageIds,
    forwardDialogVisible,
    forwardDialogSourceChannelId,
    forwardDialogSourceWorldId,
    forwardDialogMessageIds,
    forwardDialogMessages,
    handleMultiSelectForward,
    handleMessageForwardSuccess,
    handleMultiSelectCopy,
    handleMultiSelectArchive,
    handleMultiSelectDelete,
    handleMultiSelectCopyImage,
    handleMultiSelectMoveToBottom,
    handleMultiSelectRelocate,
    handleCancelMultiSelectRelocate,
    handleRelocateTargetPick,
    handleMultiSelectAll,
  };
};
