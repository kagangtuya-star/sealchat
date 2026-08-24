import { computed, reactive, ref, type Ref } from 'vue';
import { urlBase } from '@/stores/_config';
import { useAIStore, isUserAISettingsRequiredMessage } from '@/stores/ai';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import {
  clearAIPolishSlot,
  createAIPolishDockState,
  finishAIPolishTaskError,
  finishAIPolishTaskSuccess,
  findNextIdleAIPolishSlot,
  readCurrentInputIntoSlot,
  setActiveAIPolishSlot,
  setAIPolishSlotViewMode,
  prepareAIPolishTask,
  toggleAIPolishDockMinimized,
  type AIPolishDockState,
} from '@/services/ai/ai-polish-dock';

interface ChatAIPolishOptions {
  aiStore: ReturnType<typeof useAIStore>;
  chat: ReturnType<typeof useChatStore>;
  utils: ReturnType<typeof useUtilsStore>;
  message: any;
  dialog: any;
  inputMode: Ref<'plain' | 'rich'>;
  textToSend: Ref<string>;
  textInputRef: Ref<any>;
  resolveInput: () => string;
  buildRichContentFromPlain: (text: string) => unknown;
}

export const useChatAIPolish = ({
  aiStore,
  chat,
  utils,
  message,
  dialog,
  inputMode,
  textToSend,
  textInputRef,
  resolveInput,
  buildRichContentFromPlain,
}: ChatAIPolishOptions) => {
  const aiPolishDockVisible = ref(false);
  const aiPolishDockState = reactive(createAIPolishDockState()) as AIPolishDockState;
  const aiPolishActiveSlot = computed(() => aiPolishDockState.slots[aiPolishDockState.activeSlotIndex]);
  const aiPolishAnyLoading = computed(() => aiPolishDockState.slots.some((slot) => slot.status === 'loading'));
  const aiPolishFaviconHref = computed(() => {
    const faviconAttachmentId = utils.config?.faviconAttachmentId?.trim() || '';
    const normalized = faviconAttachmentId.startsWith('id:') ? faviconAttachmentId.slice(3) : faviconAttachmentId;
    if (normalized) {
      return `${urlBase}/api/v1/attachment/${encodeURIComponent(normalized)}?v=${encodeURIComponent(normalized)}`;
    }
    return `${urlBase}/favicon.ico?v=default`;
  });

  const showSettingsRequiredDialog = (errMsg: string) => {
    if (!isUserAISettingsRequiredMessage(errMsg)) {
      return false;
    }
    dialog.warning({
      title: '需要配置个人 API',
      content: '当前功能仅允许用户自定义调用。请先前往个人设置中的 AI 设置，配置个人 API 后再使用。',
      positiveText: '前往配置',
      negativeText: '取消',
      onPositiveClick: () => {
        chatEvent.emit('open-user-profile', { openAISettings: true } as any);
      },
    });
    return true;
  };

  const runAIPolishTask = async (input: string, preferredSlotIndex?: number) => {
    const { slotIndex, requestId } = prepareAIPolishTask(aiPolishDockState, input, preferredSlotIndex);
    try {
      const resp = await aiStore.runTask('polish', {
        worldId: chat.currentWorldId ? String(chat.currentWorldId) : '',
        channelId: chat.curChannel?.id || '',
        input,
        source: aiStore.currentSource,
      });
      finishAIPolishTaskSuccess(aiPolishDockState, slotIndex, requestId, String(resp.data?.result || ''));
    } catch (error: any) {
      const errMsg = error?.response?.data?.message || error?.message || '润色失败';
      finishAIPolishTaskError(aiPolishDockState, slotIndex, requestId, errMsg);
      if (showSettingsRequiredDialog(errMsg)) {
        return;
      }
      message.error(errMsg);
    }
  };

  const runAIPolish = async () => {
    const input = resolveInput();
    if (!input) {
      message.error('请输入需要润色的内容');
      return;
    }
    aiPolishDockVisible.value = true;
    const activeSlot = aiPolishDockState.slots[aiPolishDockState.activeSlotIndex];
    if (activeSlot && activeSlot.status !== 'idle' && activeSlot.sourceText.trim()) {
      const nextIdleIndex = findNextIdleAIPolishSlot(aiPolishDockState);
      if (nextIdleIndex < 0) {
        message.warning('5 个润色槽都已占用，请先清空一个槽位或切换后重用');
        return;
      }
    }
    await runAIPolishTask(input);
  };

  const applyAIPolishResult = () => {
    const resultText = aiPolishActiveSlot.value?.resultText || '';
    if (inputMode.value === 'rich') {
      textToSend.value = JSON.stringify(buildRichContentFromPlain(resultText));
    } else {
      textToSend.value = resultText;
    }
    textInputRef.value?.focus?.();
  };

  const retryCurrentAIPolishTask = async () => {
    const sourceText = aiPolishActiveSlot.value?.sourceText?.trim() || '';
    if (!sourceText) {
      message.error('当前槽位没有可重试的原文');
      return;
    }
    await runAIPolishTask(sourceText, aiPolishDockState.activeSlotIndex);
  };

  const readCurrentInputIntoAIPolishSlot = () => {
    const input = resolveInput();
    if (!input) {
      message.error('当前输入框没有可读取内容');
      return;
    }
    aiPolishDockVisible.value = true;
    readCurrentInputIntoSlot(aiPolishDockState, aiPolishDockState.activeSlotIndex, input);
  };

  const updateActiveAIPolishResultText = (value: string) => {
    const slot = aiPolishActiveSlot.value;
    if (slot) slot.resultText = value;
  };

  const updateActiveAIPolishSourceText = (value: string) => {
    const slot = aiPolishActiveSlot.value;
    if (slot) slot.sourceText = value;
  };

  const updateActiveAIPolishViewMode = (viewMode: 'edit' | 'diff') => {
    setAIPolishSlotViewMode(aiPolishDockState, aiPolishDockState.activeSlotIndex, viewMode);
  };

  const clearCurrentAIPolishSlot = () => {
    if (aiPolishActiveSlot.value?.status === 'loading') {
      message.warning('当前槽位仍在处理中，暂不能清空');
      return;
    }
    clearAIPolishSlot(aiPolishDockState, aiPolishDockState.activeSlotIndex);
  };

  const closeAIPolishDock = () => {
    aiPolishDockVisible.value = false;
  };

  return {
    aiPolishDockVisible,
    aiPolishDockState,
    aiPolishActiveSlot,
    aiPolishAnyLoading,
    aiPolishFaviconHref,
    runAIPolish,
    applyAIPolishResult,
    retryCurrentAIPolishTask,
    readCurrentInputIntoAIPolishSlot,
    updateActiveAIPolishResultText,
    updateActiveAIPolishSourceText,
    updateActiveAIPolishViewMode,
    clearCurrentAIPolishSlot,
    closeAIPolishDock,
    setActiveAIPolishSlot,
    toggleAIPolishDockMinimized,
  };
};
