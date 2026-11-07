import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { useChatStore } from './chat';
import {
  getCharacterSheetTemplateIntegration,
  matchCharacterSheetProbeResponse,
  normalizeCharacterSheetType,
} from '@/utils/characterSheetType';

export type CharacterSheetIntegrationStatus = 'unknown' | 'checking' | 'available' | 'missing' | 'unavailable';

export const useCharacterSheetIntegrationStore = defineStore('characterSheetIntegration', () => {
  const chatStore = useChatStore();
  const statuses = reactive(new Map<string, CharacterSheetIntegrationStatus>());
  const pending = new Map<string, Promise<CharacterSheetIntegrationStatus>>();

  const resolveContext = (
    sheetType: string,
    templateHtml: string,
    channelId = chatStore.curChannel?.id || '',
  ) => {
    const channel = chatStore.curChannel?.id === channelId
      ? chatStore.curChannel
      : chatStore.findChannelById(channelId);
    const primaryBotId = String(channel?.primaryBotId || '').trim();
    const privateBotId = channel?.friendInfo?.userInfo?.is_bot === true
      ? String(channel.friendInfo.userInfo.id || '').trim()
      : '';
    const botId = primaryBotId || privateBotId;
    const normalizedSheetType = normalizeCharacterSheetType(sheetType);
    const integration = getCharacterSheetTemplateIntegration(templateHtml, normalizedSheetType);
    const integrationKey = integration ? JSON.stringify(integration) : '';
    return {
      channelId,
      botId,
      sheetType: normalizedSheetType,
      integration,
      key: JSON.stringify([channelId, botId, normalizedSheetType, integrationKey]),
    };
  };

  const getCacheKey = (sheetType: string, templateHtml: string, channelId?: string) => (
    resolveContext(sheetType, templateHtml, channelId).key
  );

  const getStatus = (
    sheetType: string,
    templateHtml: string,
    channelId?: string,
  ): CharacterSheetIntegrationStatus => (
    statuses.get(getCacheKey(sheetType, templateHtml, channelId)) || 'unknown'
  );

  const check = (
    sheetType: string,
    templateHtml: string,
    channelId?: string,
    force = false,
  ): Promise<CharacterSheetIntegrationStatus> => {
    const context = resolveContext(sheetType, templateHtml, channelId);
    const { integration, key } = context;
    if (!integration) return Promise.resolve('unknown');
    const inFlight = pending.get(key);
    if (inFlight) return inFlight;
    const cached = statuses.get(key);
    if (!force && cached) return Promise.resolve(cached);

    statuses.set(key, 'checking');
    const request = (async (): Promise<CharacterSheetIntegrationStatus> => {
      let status: CharacterSheetIntegrationStatus = 'unavailable';
      try {
        if (context.channelId && context.botId && chatStore.connectState === 'connected' && !chatStore.observerMode) {
          const result = await chatStore.botInteract(context.channelId, integration.probe.command, {
            timeoutMs: integration.probe.timeoutMs ?? 5000,
          });
          if (result?.ok === true && typeof result.content === 'string' && result.content.trim()) {
            status = matchCharacterSheetProbeResponse(integration.probe, result.content) ? 'available' : 'missing';
          }
        }
      } catch {
        status = 'unavailable';
      }
      if (resolveContext(context.sheetType, templateHtml, context.channelId).key !== key) {
        status = 'unavailable';
      }
      statuses.set(key, status);
      return status;
    })();
    pending.set(key, request);
    void request.finally(() => pending.delete(key));
    return request;
  };

  return { getCacheKey, getStatus, check };
});
