import type { CharacterCard } from '@/stores/characterCard';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterCardAvatarStore } from '@/stores/characterCardAvatar';
import { useCharacterCardTemplateStore } from '@/stores/characterCardTemplate';
import { useCharacterSheetStore } from '@/stores/characterSheet';

interface OpenCharacterSheetRuntimeOptions {
  card: CharacterCard;
  channelId: string;
  worldId?: string;
  ephemeral?: boolean;
  reuse?: boolean;
}

export async function openCharacterSheetRuntime({
  card,
  channelId,
  worldId,
  ephemeral,
  reuse,
}: OpenCharacterSheetRuntimeOptions): Promise<string> {
  const cardStore = useCharacterCardStore();
  const avatarStore = useCharacterCardAvatarStore();
  const templateStore = useCharacterCardTemplateStore();
  const sheetStore = useCharacterSheetStore();

  const openFallback = () => sheetStore.openSheet(card, channelId, {
    name: card.name,
    type: card.sheetType,
    attrs: card.attrs || {},
    avatarUrl: avatarStore.resolveCardAvatar(card.id, channelId) || undefined,
  }, { worldId, ephemeral, reuse });

  if (cardStore.isBotCharacterDisabled(channelId)) {
    return openFallback();
  }

  try {
    await avatarStore.ensureBindingsLoaded(channelId);
    let cardData = cardStore.activeCards[channelId];
    const shouldRefreshActiveCard = !cardData || cardStore.getActiveCardId(channelId) === card.id;
    if (shouldRefreshActiveCard) {
      await cardStore.getActiveCard(channelId);
      cardData = cardStore.activeCards[channelId];
    }
    const effectiveCardData = cardStore.getActiveCardId(channelId) === card.id ? cardData : undefined;
    await templateStore.ensureTemplatesLoaded({ worldId });
    await templateStore.ensureBindingsLoaded(channelId);
    const resolvedSheetType = (effectiveCardData?.type || card.sheetType || '').trim();
    const fallbackTemplate = sheetStore.getTemplate(card.id, resolvedSheetType);
    const ensured = await templateStore.ensureCardBinding({
      channelId,
      externalCardId: card.id,
      cardName: card.name,
      sheetType: resolvedSheetType,
      fallbackTemplate,
    });
    const binding = templateStore.getBinding(channelId, card.id) || ensured;
    if (binding?.mode) {
      card.templateMode = binding.mode;
      card.templateId = binding.templateId || undefined;
      card.templateSnapshot = binding.templateSnapshot || undefined;
    }
    const managedTemplateContent = binding?.mode === 'managed' && binding.templateId
      ? templateStore.getTemplateById(binding.templateId)?.content
      : undefined;
    const avatarUrl = avatarStore.resolveCardAvatar(card.id, channelId, effectiveCardData?.avatarUrl || '');
    return sheetStore.openSheet(card, channelId, {
      name: effectiveCardData?.name || card.name,
      type: effectiveCardData?.type || card.sheetType,
      attrs: effectiveCardData?.attrs || card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    }, {
      templateMode: binding?.mode,
      templateId: binding?.templateId || undefined,
      templateText: binding?.mode === 'detached' ? binding.templateSnapshot : managedTemplateContent,
      worldId,
      ephemeral,
      reuse,
    });
  } catch (error) {
    console.warn('Failed to open character preview', error);
    return openFallback();
  }
}
