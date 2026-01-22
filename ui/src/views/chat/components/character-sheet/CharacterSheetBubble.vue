<template>
  <div
    class="character-sheet-bubble"
    :style="bubbleStyle"
    @pointerdown="handlePointerDown"
    @click="handleClick"
  >
    <img
      v-if="avatarUrl"
      :src="resolvedAvatarUrl"
      class="bubble-avatar"
      draggable="false"
    />
    <span v-else class="bubble-initial">{{ initial }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

const props = defineProps<{
  windowId: string;
  cardName: string;
  avatarUrl?: string;
  bubbleX: number;
  bubbleY: number;
  zIndex: number;
}>();

const emit = defineEmits<{
  restore: [];
}>();

const sheetStore = useCharacterSheetStore();

const isDragging = ref(false);
const hasMoved = ref(false);
const dragStart = ref({ x: 0, y: 0, bubbleX: 0, bubbleY: 0 });

const DRAG_THRESHOLD = 5;

const initial = computed(() => {
  const name = props.cardName || '';
  return name.charAt(0) || '?';
});

const resolvedAvatarUrl = computed(() => {
  return resolveAttachmentUrl(props.avatarUrl);
});

const bubbleStyle = computed(() => ({
  transform: `translate(${props.bubbleX}px, ${props.bubbleY}px)`,
  zIndex: props.zIndex,
}));

const handlePointerDown = (e: PointerEvent) => {
  e.preventDefault();
  isDragging.value = true;
  hasMoved.value = false;
  dragStart.value = {
    x: e.clientX,
    y: e.clientY,
    bubbleX: props.bubbleX,
    bubbleY: props.bubbleY,
  };
  document.addEventListener('pointermove', handlePointerMove);
  document.addEventListener('pointerup', handlePointerUp);
};

const handlePointerMove = (e: PointerEvent) => {
  if (!isDragging.value) return;
  const dx = e.clientX - dragStart.value.x;
  const dy = e.clientY - dragStart.value.y;
  if (Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD) {
    hasMoved.value = true;
  }
  sheetStore.updateBubblePosition(
    props.windowId,
    dragStart.value.bubbleX + dx,
    dragStart.value.bubbleY + dy
  );
};

const handlePointerUp = () => {
  isDragging.value = false;
  document.removeEventListener('pointermove', handlePointerMove);
  document.removeEventListener('pointerup', handlePointerUp);
};

const handleClick = () => {
  if (!hasMoved.value) {
    emit('restore');
  }
};

const handleResize = () => {
  sheetStore.clampAllBubbles();
};

onMounted(() => {
  window.addEventListener('resize', handleResize);
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize);
  handlePointerUp();
});
</script>

<style scoped>
.character-sheet-bubble {
  position: fixed;
  top: 0;
  left: 0;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--sc-bg-card, rgba(255, 255, 255, 0.98));
  border: 2px solid var(--sc-border-strong, rgba(100, 116, 139, 0.3));
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  cursor: pointer;
  user-select: none;
  touch-action: none;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  transition: box-shadow 0.15s ease, border-color 0.15s ease;
  pointer-events: auto;
}

.character-sheet-bubble:hover {
  border-color: var(--primary-color, #3388de);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.2);
}

.character-sheet-bubble:active {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.12);
}

.bubble-avatar {
  width: 100%;
  height: 100%;
  object-fit: cover;
  pointer-events: none;
}

.bubble-initial {
  font-size: 22px;
  font-weight: 600;
  color: var(--sc-text-primary, #1f2937);
  pointer-events: none;
}

:root[data-display-palette='night'] .character-sheet-bubble {
  background: var(--sc-bg-card, rgba(30, 41, 59, 0.98));
  border-color: rgba(148, 163, 184, 0.35);
}

:root[data-display-palette='night'] .bubble-initial {
  color: var(--sc-text-primary, #f1f5f9);
}
</style>
