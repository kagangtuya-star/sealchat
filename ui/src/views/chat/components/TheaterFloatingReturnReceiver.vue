<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useIFormStore } from '@/stores/iform';
import { useStickyNoteStore } from '@/stores/stickyNote';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useChannelCharacterSnapshotStore } from '@/stores/channelCharacterSnapshot';
import { buildInternalSurfaceResourceKey, parseInternalSurfaceLink } from '@/utils/internalSurfaceLink';
import {
  CHAT_FLOATING_TAKEOVER_ACK,
  isChatFloatingTakeoverRequest,
  type ChatFloatingTakeoverAck,
  type ChatFloatingTakeoverRequest,
} from '@/utils/theaterFloatingBridge';
import { openCharacterSheetRuntime } from './character-sheet/openCharacterSheetRuntime';

const chatStore = useChatStore();
const iformStore = useIFormStore();
const stickyNoteStore = useStickyNoteStore();
const characterCardStore = useCharacterCardStore();
const characterSheetStore = useCharacterSheetStore();
const snapshotStore = useChannelCharacterSnapshotStore();

const isCurrentChatContext = (worldId: string, channelId: string) => (
  String(chatStore.currentWorldId || '') === worldId
  && String(chatStore.curChannel?.id || '') === channelId
);

const openReturnedCharacter = async (resourceId: string, channelId: string, worldId: string) => {
  let card = characterCardStore.cards.find(item => item.id === resourceId);
  if (!card) {
    await characterCardStore.loadCards(channelId);
    if (!isCurrentChatContext(worldId, channelId)) return '';
    card = characterCardStore.cards.find(item => item.id === resourceId);
  }
  if (card) {
    return openCharacterSheetRuntime({
      card,
      channelId,
      worldId,
      reuse: true,
      isContextCurrent: () => isCurrentChatContext(worldId, channelId),
    });
  }

  await snapshotStore.initializeChannel(channelId);
  if (!isCurrentChatContext(worldId, channelId)) return '';
  const snapshotPrefix = `snapshot:${channelId}:`;
  let snapshot = snapshotStore.getChannelItems(channelId).find(item => (
    !!item.data.card
    && (item.sourceCardId === resourceId || `${snapshotPrefix}${item.identityId}` === resourceId)
  ));
  if (!snapshot) {
    await snapshotStore.refreshChannel(channelId);
    if (!isCurrentChatContext(worldId, channelId)) return '';
    snapshot = snapshotStore.getChannelItems(channelId).find(item => (
      !!item.data.card
      && (item.sourceCardId === resourceId || `${snapshotPrefix}${item.identityId}` === resourceId)
    ));
  }
  const snapshotCard = snapshot?.data.card;
  if (!snapshot || !snapshotCard) return '';
  const windowId = characterSheetStore.openSheet({
    id: `${snapshotPrefix}${snapshot.identityId}`,
    name: snapshotCard.name,
    sheetType: snapshotCard.sheetType,
    attrs: snapshotCard.attrs,
    channelId,
    userId: snapshot.userId,
  }, channelId, {
    name: snapshotCard.name,
    type: snapshotCard.sheetType,
    attrs: snapshotCard.attrs,
    avatarUrl: snapshot.data.identity.avatarAttachmentId || undefined,
    templateText: snapshotCard.templateText,
  }, {
    templateText: snapshotCard.templateText,
    readOnly: true,
    ephemeral: true,
    worldId,
  });
  characterSheetStore.setMode(windowId, 'view');
  return windowId;
};

const openReturnedWindow = async (request: ChatFloatingTakeoverRequest) => {
  const parsed = parseInternalSurfaceLink(request.resource.url);
  if (!parsed || buildInternalSurfaceResourceKey(parsed) !== request.resource.key) return false;
  if (!isCurrentChatContext(parsed.worldId, parsed.channelId)) return false;

  const presentation = request.resource.presentation;
  const x = request.clientX - request.offsetX;
  const y = request.clientY - request.offsetY;
  const width = presentation?.width;
  const height = presentation?.height;
  const minimized = presentation?.minimized === true;

  if (parsed.type === 'iform') {
    await iformStore.ensureForms(parsed.channelId);
    if (!isCurrentChatContext(parsed.worldId, parsed.channelId)) return false;
    if (!(iformStore.formsByChannel[parsed.channelId] || []).some(item => item.id === parsed.id)) return false;
    iformStore.openFloating(parsed.id, { x, y, width, height, minimized }, parsed.channelId);
    return true;
  }

  if (parsed.type === 'note') {
    if (!stickyNoteStore.notes[parsed.id]) {
      await stickyNoteStore.loadChannelNotes(parsed.channelId);
      if (!isCurrentChatContext(parsed.worldId, parsed.channelId)) return false;
    }
    if (!stickyNoteStore.notes[parsed.id]) return false;
    stickyNoteStore.openNote(parsed.id, {
      persistRemote: false,
      state: {
        positionX: x,
        positionY: y,
        width,
        height,
        minimized,
      },
    });
    return true;
  }

  const windowId = await openReturnedCharacter(parsed.id, parsed.channelId, parsed.worldId);
  if (!isCurrentChatContext(parsed.worldId, parsed.channelId)) return false;
  if (!windowId) return false;
  if (width !== undefined && height !== undefined) {
    characterSheetStore.updateSize(windowId, width, height);
  }
  if (minimized) {
    characterSheetStore.updateBubblePosition(windowId, x, y);
    characterSheetStore.minimizeSheet(windowId);
  } else {
    characterSheetStore.updatePosition(windowId, x, y);
  }
  return true;
};

const postAck = (requestId: string, accepted: boolean) => {
  const ack: ChatFloatingTakeoverAck = {
    type: CHAT_FLOATING_TAKEOVER_ACK,
    requestId,
    accepted,
  };
  window.parent.postMessage(ack, window.location.origin);
};

const handleMessage = async (event: MessageEvent<unknown>) => {
  if (window.parent === window) return;
  if (event.origin !== window.location.origin || event.source !== window.parent) return;
  if (!isChatFloatingTakeoverRequest(event.data)) return;
  let accepted = false;
  try {
    accepted = await openReturnedWindow(event.data);
    if (accepted) await nextTick();
  } catch (error) {
    console.warn('Failed to restore theater floating window to chat', error);
  }
  postAck(event.data.requestId, accepted);
};

onMounted(() => window.addEventListener('message', handleMessage));
onBeforeUnmount(() => window.removeEventListener('message', handleMessage));
</script>

<template></template>
