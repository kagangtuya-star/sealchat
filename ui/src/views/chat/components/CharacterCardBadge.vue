<script setup lang="ts">
import { computed } from 'vue';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useDisplayStore } from '@/stores/display';
import { useChatStore } from '@/stores/chat';
import { messageVisibilityScopeMatches } from '@/stores/displayAvatarVisibility';
import {
  renderCardTemplate,
  getWorldCardTemplate,
  hasRenderableBadgeData,
} from '@/utils/characterCardTemplate';
import { resolveIdentityMetaStyle } from '@/utils/identityMetaContrast';

const props = defineProps<{
  identityId?: string;
  identityColor?: string;
  messageTone?: 'ic' | 'ooc' | 'archived';
  hostBackgroundColor?: string;
}>();

const cardStore = useCharacterCardStore();
const displayStore = useDisplayStore();
const chatStore = useChatStore();

const badgeEntry = computed(() => {
  const channelId = chatStore.curChannel?.id || '';
  const identityId = props.identityId || '';
  return cardStore.getBadgeByIdentity(channelId, identityId);
});

const worldTemplate = computed(() => {
  const worldId = chatStore.currentWorldId;
  const world = chatStore.currentWorld;
  const template = typeof world?.characterCardBadgeTemplate === 'string' ? world.characterCardBadgeTemplate.trim() : '';
  if (template) return template;
  const fromDetail = (chatStore as any).worldDetailMap?.[worldId]?.world?.characterCardBadgeTemplate;
  if (typeof fromDetail === 'string' && fromDetail.trim()) {
    return fromDetail.trim();
  }
  return '';
});

const template = computed(() => {
  const worldId = chatStore.currentWorldId;
  if (worldTemplate.value) return worldTemplate.value;
  return badgeEntry.value?.template
    || displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId]
    || getWorldCardTemplate(worldId);
});

const resolvedAttrs = computed<Record<string, any> | undefined>(() => {
  if (badgeEntry.value?.attrs) return badgeEntry.value.attrs;
  const channelId = chatStore.curChannel?.id || '';
  const activeIdentityId = chatStore.getActiveIdentityId(channelId);
  const activeBoundCardId = props.identityId ? (cardStore.getBoundCardId(props.identityId) || '') : '';
  if (channelId && activeIdentityId && props.identityId === activeIdentityId && activeBoundCardId) {
    return cardStore.activeCards[channelId]?.attrs;
  }
  return undefined;
});

const renderedContent = computed(() => {
  if (!resolvedAttrs.value) return '';
  return renderCardTemplate(template.value, resolvedAttrs.value);
});

const isVisible = computed(() => {
  return displayStore.settings.characterCardBadgeEnabled
    && !cardStore.isNarratorIdentity(chatStore.curChannel?.id || '', props.identityId || '')
    && messageVisibilityScopeMatches(
      displayStore.settings.characterCardBadgeVisibilityScope,
      props.messageTone,
    )
    && hasRenderableBadgeData(template.value, resolvedAttrs.value)
    && !!renderedContent.value;
});

const badgeStyle = computed(() => resolveIdentityMetaStyle({
  enabled: displayStore.settings.characterCardBadgeAutoContrastEnabled,
  kind: 'badge',
  identityColor: props.identityColor,
  backgroundColor: props.hostBackgroundColor,
}).style);
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
  font-size: 0.68em;
  line-height: 1.2;
  padding: 0.08em 0.36em;
  border-radius: 6px;
  border: 1px solid rgba(128, 128, 128, 0.2);
  margin-left: 0.5em;
  vertical-align: middle;
  white-space: nowrap;
}
</style>
