<script lang="ts">
let characterSurfaceLoadQueue: Promise<void> = Promise.resolve();
</script>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { NButton, NResult } from 'naive-ui';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useChannelCharacterSnapshotStore } from '@/stores/channelCharacterSnapshot';
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
const snapshotStore = useChannelCharacterSnapshotStore();
const windowId = ref('');
const snapshotBinding = ref<{ channelId: string; identityId: string; worldId: string } | null>(null);
const snapshotUnavailable = ref('');
const snapshotRefreshing = ref(false);
let taskEpoch = 0;
let restoreContext: { channelId: string; cardId: string; cardName: string } | null = null;

const findSnapshotByResourceId = (channelId: string, resourceId: string) => {
  const snapshotPrefix = `snapshot:${channelId}:`;
  return snapshotStore.getChannelItems(channelId).find((item) => (
    !!item.data.card
    && (
      item.sourceCardId === resourceId
      || `${snapshotPrefix}${item.identityId}` === resourceId
    )
  )) || null;
};

const openSnapshotCard = (snapshot: NonNullable<ReturnType<typeof findSnapshotByResourceId>>, channelId: string, worldId: string) => {
  const card = snapshot.data.card;
  if (!card) return '';
  const openedWindowId = sheetStore.openSheet({
    id: `snapshot:${channelId}:${snapshot.identityId}`,
    name: card.name,
    sheetType: card.sheetType,
    attrs: card.attrs,
    channelId,
    userId: snapshot.userId,
  }, channelId, {
    name: card.name,
    type: card.sheetType,
    attrs: card.attrs,
    avatarUrl: card.avatarAttachmentId || snapshot.data.identity.avatarAttachmentId || undefined,
    templateText: card.templateText,
  }, {
    templateText: card.templateText,
    readOnly: true,
    ephemeral: true,
    worldId,
    reuse: false,
  });
  sheetStore.setMode(openedWindowId, 'view');
  return openedWindowId;
};

const loadSnapshotByResourceId = async (channelId: string, resourceId: string) => {
  // 每次打开都重新检查来源频道权限，不用旧缓存掩盖撤回或失活。
  await snapshotStore.refreshChannel(channelId);
  return findSnapshotByResourceId(channelId, resourceId);
};

const closeWindow = () => {
  if (windowId.value) sheetStore.closeSheet(windowId.value);
  windowId.value = '';
};

watch([snapshotBinding, () => {
  const binding = snapshotBinding.value;
  return binding ? snapshotStore.getSnapshot(binding.channelId, binding.identityId) : null;
}] as const, ([binding, snapshot]) => {
  if (!binding) return;
  const card = snapshot?.data.card;
  if (!snapshot || !card) {
    closeWindow();
    snapshotUnavailable.value = '该身份的角色卡快照不存在、已撤回或已失活，请刷新重试或重新绑定。';
  } else {
    snapshotUnavailable.value = '';
    if (windowId.value) {
      sheetStore.updateReadOnlyWindowData(windowId.value, {
        cardName: card.name,
        sheetType: card.sheetType,
        attrs: card.attrs,
        avatarUrl: card.avatarAttachmentId || snapshot.data.identity.avatarAttachmentId || '',
        templateText: card.templateText || '',
      });
    } else {
      windowId.value = openSnapshotCard(snapshot, binding.channelId, binding.worldId);
    }
  }
  // 快照失效由本组件呈现，保持挂载以便同一身份重新同步后恢复。
  emit('ready');
});

const refreshSnapshot = async () => {
  const binding = snapshotBinding.value;
  const epoch = taskEpoch;
  if (!binding || snapshotRefreshing.value) return;
  snapshotRefreshing.value = true;
  try {
    await snapshotStore.refreshChannel(binding.channelId);
  } catch {
    if (epoch !== taskEpoch) return;
    closeWindow();
    snapshotUnavailable.value = '角色卡快照加载失败，请确认来源频道的访问权限后重试。';
  } finally {
    if (epoch === taskEpoch) snapshotRefreshing.value = false;
  }
};

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
    snapshotBinding.value = null;
    snapshotUnavailable.value = '';
    snapshotRefreshing.value = false;
    closeWindow();
    await restoreActiveCard();
    if (epoch !== taskEpoch) return;
    try {
      if (resourceId.startsWith('snapshot:')) {
        const prefix = `snapshot:${channelId}:`;
        if (!resourceId.startsWith(prefix) || resourceId.length === prefix.length) {
          emit('unavailable', '角色卡快照与来源频道不匹配，请重新绑定。');
          return;
        }
        // 明确的快照资源直接读取数据库，不依赖个人人物卡 API，也不切换当前人物卡。
        await snapshotStore.refreshChannel(channelId);
        if (epoch !== taskEpoch) return;
        snapshotBinding.value = { channelId, identityId: resourceId.slice(prefix.length), worldId };
        return;
      }
      characterSurfaceLoadQueue = characterSurfaceLoadQueue
        .catch(() => undefined)
        .then(() => cardStore.loadCards(channelId));
      await characterSurfaceLoadQueue;
      if (epoch !== taskEpoch) return;
      const card = cardStore.cards.find(item => item.id === resourceId);
      if (!card) {
        const snapshot = await loadSnapshotByResourceId(channelId, resourceId);
        if (epoch !== taskEpoch) return;
        if (snapshot) {
          snapshotBinding.value = { channelId, identityId: snapshot.identityId, worldId };
          return;
        }
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
      const openedWindowId = await openCharacterSheetRuntime({
        card,
        channelId,
        worldId,
        ephemeral: true,
        reuse: false,
        isContextCurrent: () => epoch === taskEpoch,
      });
      if (epoch !== taskEpoch) {
        if (openedWindowId) sheetStore.closeSheet(openedWindowId);
        return;
      }
      windowId.value = openedWindowId;
      emit('ready');
    } catch (error: any) {
      if (epoch !== taskEpoch) return;
      await restoreActiveCard();
      if (epoch !== taskEpoch) return;
      emit('error', error?.response?.data?.error || error?.message || '人物卡加载失败');
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  taskEpoch += 1;
  snapshotBinding.value = null;
  closeWindow();
  void restoreActiveCard();
});
</script>

<template>
  <NResult v-if="snapshotUnavailable" status="warning" title="角色卡快照不可用" :description="snapshotUnavailable">
    <template #footer>
      <NButton :loading="snapshotRefreshing" @click="refreshSnapshot">刷新快照</NButton>
    </template>
  </NResult>
  <CharacterSheetManager
    v-else-if="windowId"
    :window-id="windowId"
    standalone
  />
</template>
