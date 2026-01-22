<script setup lang="ts">
import { computed } from 'vue';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useDisplayStore } from '@/stores/display';
import { useChatStore } from '@/stores/chat';
import { renderCardTemplate, getWorldCardTemplate } from '@/utils/characterCardTemplate';

const props = defineProps<{
  identityId?: string;
  identityColor?: string;
}>();

const cardStore = useCharacterCardStore();
const displayStore = useDisplayStore();
const chatStore = useChatStore();

const card = computed(() => {
  if (!props.identityId) return null;
  const cardId = cardStore.getBoundCardId(props.identityId);
  if (!cardId) return null;
  return cardStore.getCardById(cardId);
});

const template = computed(() => {
  const worldId = chatStore.currentWorldId;
  return displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId] ?? getWorldCardTemplate(worldId);
});

const renderedContent = computed(() => {
  if (!card.value || !card.value.attrs) return '';
  return renderCardTemplate(template.value, card.value.attrs);
});

const isVisible = computed(() => {
  return displayStore.settings.characterCardBadgeEnabled && !!renderedContent.value;
});

const badgeStyle = computed(() => {
  if (!props.identityColor) return {};
  return {
    backgroundColor: `${props.identityColor}15`,
    color: props.identityColor,
    borderColor: `${props.identityColor}40`,
  };
});
</script>

<template>
  <span
    v-if="isVisible"
    class="character-card-badge"
    :style="badgeStyle"
    v-html="renderedContent"
  ></span>
</template>

<style scoped>
.character-card-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3em;
  font-size: 0.75em;
  line-height: 1.2;
  padding: 0.1em 0.4em;
  border-radius: 4px;
  border: 1px solid rgba(128, 128, 128, 0.2);
  margin-left: 0.5em;
  vertical-align: middle;
  white-space: nowrap;
}
</style>
