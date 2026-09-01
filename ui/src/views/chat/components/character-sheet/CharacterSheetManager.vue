<template>
  <Teleport to="body">
    <div class="character-sheet-overlay" data-sc-font-surface="true" v-if="managedWindowIds.length > 0">
      <CharacterSheetWindow
        v-for="windowId in expandedWindowIds"
        :key="windowId"
        :window-id="windowId"
        :standalone="standalone"
        @roll-request="handleRollRequest"
      />
      <CharacterSheetBubble
        v-if="!standalone"
        v-for="win in minimizedWindows"
        :key="`bubble-${win.id}`"
        :window-id="win.id"
        :card-name="win.cardName"
        :avatar-url="win.avatarUrl"
        :bubble-x="win.bubbleX"
        :bubble-y="win.bubbleY"
        :z-index="win.zIndex"
        @restore="sheetStore.restoreSheet(win.id)"
      />
    </div>
    <DiceRollPopover
      v-model:visible="popoverVisible"
      :label="pendingRoll?.label"
      :template="pendingRoll?.template || ''"
      :args="pendingRoll?.args"
      :target-rect="pendingRoll?.rect"
      :container-rect="pendingRoll?.containerRect"
      @confirm="executeRoll"
      @cancel="popoverVisible = false"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useChatStore } from '@/stores/chat';
import CharacterSheetWindow from './CharacterSheetWindow.vue';
import CharacterSheetBubble from './CharacterSheetBubble.vue';
import DiceRollPopover from './DiceRollPopover.vue';
import type { SealChatEventPayload } from './IframeSandbox.vue';
import { buildRollExpression } from './rollExpression';

const sheetStore = useCharacterSheetStore();
const chatStore = useChatStore();

const props = defineProps<{
  windowId?: string;
  standalone?: boolean;
}>();

const popoverVisible = ref(false);
const pendingRoll = ref<SealChatEventPayload['roll'] | null>(null);

const managedWindowIds = computed(() => (
  props.windowId
    ? sheetStore.activeWindowIds.filter(id => id === props.windowId)
    : sheetStore.activeWindowIds
));

const minimizedWindows = computed(() =>
  sheetStore.activeWindows.filter(w => managedWindowIds.value.includes(w.id) && w.isMinimized)
);

const expandedWindowIds = computed(() =>
  managedWindowIds.value.filter(id => !sheetStore.windows[id]?.isMinimized)
);

const executeRollDirectly = (payload: SealChatEventPayload['roll']) => {
  const expression = buildRollExpression(payload?.template || '', payload?.args);
  executeRoll(expression);
};

const handleRollRequest = (payload: SealChatEventPayload['roll']) => {
  if (!payload) return;
  if (payload.dispatchMode === 'template') {
    executeRollDirectly(payload);
    return;
  }
  pendingRoll.value = payload;
  popoverVisible.value = true;
};

const executeRoll = (expression: string) => {
  if (!expression.trim()) return;
  chatStore.messageCreate(expression);
  popoverVisible.value = false;
  pendingRoll.value = null;
};

let pollTimer: ReturnType<typeof setInterval> | null = null;
let pollInFlight = false;
const POLL_INTERVAL = 10000;

const startPolling = () => {
  if (pollTimer) return;
  pollTimer = setInterval(async () => {
    if (pollInFlight || managedWindowIds.value.length === 0) return;
    pollInFlight = true;
    try {
      if (props.windowId) {
        await sheetStore.refreshWindowAttrs(props.windowId);
      } else {
        await sheetStore.refreshAllWindows();
      }
    } finally {
      pollInFlight = false;
    }
  }, POLL_INTERVAL);
};

const stopPolling = () => {
  if (!pollTimer) return;
  clearInterval(pollTimer);
  pollTimer = null;
};

onMounted(() => {
  if (managedWindowIds.value.length > 0) {
    if (props.windowId) void sheetStore.refreshWindowAttrs(props.windowId);
    else void sheetStore.refreshAllWindows();
    startPolling();
  }
});

watch(
  () => chatStore.curChannel?.id,
  () => {
    if (managedWindowIds.value.length > 0) {
      if (props.windowId) void sheetStore.refreshWindowAttrs(props.windowId);
      else void sheetStore.refreshAllWindows();
    }
  }
);

watch(
  () => managedWindowIds.value.length,
  (len) => {
    if (len > 0) {
      startPolling();
    } else {
      stopPolling();
    }
  }
);

onBeforeUnmount(() => {
  stopPolling();
});
</script>

<style scoped>
.character-sheet-overlay {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 1900;
}
</style>
