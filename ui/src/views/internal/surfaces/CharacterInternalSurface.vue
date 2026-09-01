<script lang="ts">
let characterSurfaceLoadQueue: Promise<void> = Promise.resolve();
</script>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import CharacterSheetManager from '@/views/chat/components/character-sheet/CharacterSheetManager.vue';
import { openCharacterSheetRuntime } from '@/views/chat/components/character-sheet/openCharacterSheetRuntime';

const props = defineProps<{
  resourceId: string;
  worldId: string;
  channelId: string;
}>();

const emit = defineEmits<{
  ready: [];
  unavailable: [message: string];
  error: [message: string];
}>();

const cardStore = useCharacterCardStore();
const sheetStore = useCharacterSheetStore();
const windowId = ref('');
let taskEpoch = 0;
let restoreContext: { channelId: string; cardId: string; cardName: string } | null = null;

const restoreActiveCard = async () => {
  const context = restoreContext;
  restoreContext = null;
  if (!context || cardStore.getActiveCardId(context.channelId) === context.cardId) return;
  try {
    const restored = await cardStore.tagCard(context.channelId, undefined, context.cardId);
    if (restored && context.cardName) {
      await cardStore.syncBotNicknameForCard(context.channelId, context.cardName, {
        reason: 'internal-character-surface-restore',
      });
    }
  } catch (error) {
    console.warn('Failed to restore character after closing internal surface', error);
  }
};

watch(
  () => [props.resourceId, props.worldId, props.channelId] as const,
  async ([resourceId, worldId, channelId]) => {
    const epoch = ++taskEpoch;
    if (windowId.value) {
      sheetStore.closeSheet(windowId.value);
      windowId.value = '';
    }
    await restoreActiveCard();
    if (epoch !== taskEpoch) return;
    try {
      characterSurfaceLoadQueue = characterSurfaceLoadQueue
        .catch(() => undefined)
        .then(() => cardStore.loadCards(channelId));
      await characterSurfaceLoadQueue;
      if (epoch !== taskEpoch) return;
      const card = cardStore.cards.find(item => item.id === resourceId);
      if (!card) {
        emit('unavailable', '人物卡不存在或当前人物卡 API 不可用');
        return;
      }
      if (!cardStore.isBotCharacterDisabled(channelId) && cardStore.getActiveCardId(channelId) !== card.id) {
        const previousCardId = cardStore.getActiveCardId(channelId);
        if (previousCardId) {
          restoreContext = {
            channelId,
            cardId: previousCardId,
            cardName: cardStore.getCardById(previousCardId)?.name || '',
          };
        }
        const switched = await cardStore.tagCard(channelId, undefined, card.id);
        if (!switched) throw new Error('切换人物卡失败');
        await cardStore.syncBotNicknameForCard(channelId, card.name, {
          reason: 'internal-character-surface',
        });
      }
      if (epoch !== taskEpoch) return;
      windowId.value = await openCharacterSheetRuntime({
        card,
        channelId,
        worldId,
        ephemeral: true,
        reuse: false,
      });
      if (epoch !== taskEpoch) {
        sheetStore.closeSheet(windowId.value);
        windowId.value = '';
        return;
      }
      emit('ready');
    } catch (error: any) {
      if (epoch !== taskEpoch) return;
      await restoreActiveCard();
      emit('error', error?.response?.data?.error || error?.message || '人物卡加载失败');
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  taskEpoch += 1;
  if (windowId.value) sheetStore.closeSheet(windowId.value);
  void restoreActiveCard();
});
</script>

<template>
  <CharacterSheetManager
    v-if="windowId"
    :window-id="windowId"
    standalone
  />
</template>
