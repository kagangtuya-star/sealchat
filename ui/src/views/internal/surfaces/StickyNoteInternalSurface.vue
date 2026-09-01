<script lang="ts">
let stickyNoteSurfaceLoadQueue: Promise<void> = Promise.resolve();
</script>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, watch } from 'vue';
import { chatEvent } from '@/stores/chat';
import { useStickyNoteStore } from '@/stores/stickyNote';
import StickyNote from '@/views/chat/components/StickyNote.vue';

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

const stickyNoteStore = useStickyNoteStore();
let taskEpoch = 0;
let resourceReady = false;

const handleEvent = (event: any) => {
  if (event?.type?.startsWith('sticky-note-')) {
    stickyNoteStore.handleStickyNoteEvent(event);
  }
};

watch(
  () => [props.resourceId, props.channelId] as const,
  async ([resourceId, channelId]) => {
    const epoch = ++taskEpoch;
    resourceReady = false;
    try {
      stickyNoteSurfaceLoadQueue = stickyNoteSurfaceLoadQueue
        .catch(() => undefined)
        .then(() => stickyNoteStore.loadChannelNotes(channelId));
      await stickyNoteSurfaceLoadQueue;
      if (epoch !== taskEpoch) return;
      if (!stickyNoteStore.notes[resourceId]) {
        emit('unavailable', '便签不存在或当前用户不可见');
        return;
      }
      resourceReady = true;
      emit('ready');
    } catch (error: any) {
      if (epoch !== taskEpoch) return;
      emit('error', error?.response?.data?.error || error?.message || '便签加载失败');
    }
  },
  { immediate: true },
);

watch(
  () => stickyNoteStore.notes[props.resourceId],
  (note, previousNote) => {
    if (!resourceReady || !previousNote || note) return;
    resourceReady = false;
    emit('unavailable', '便签已删除或当前用户不可见');
  },
);

onMounted(() => {
  chatEvent.on('sticky-note-created', handleEvent);
  chatEvent.on('sticky-note-updated', handleEvent);
  chatEvent.on('sticky-note-deleted', handleEvent);
  chatEvent.on('sticky-note-pushed', handleEvent);
});

onBeforeUnmount(() => {
  taskEpoch += 1;
  chatEvent.off('sticky-note-created', handleEvent);
  chatEvent.off('sticky-note-updated', handleEvent);
  chatEvent.off('sticky-note-deleted', handleEvent);
  chatEvent.off('sticky-note-pushed', handleEvent);
});
</script>

<template>
  <StickyNote
    v-if="stickyNoteStore.notes[resourceId]"
    :note-id="resourceId"
    internal-surface
  />
</template>
