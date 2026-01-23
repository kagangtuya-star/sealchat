import { normalizeAttachmentId, resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { getEmojiUrl, getFallbackUrl, shouldUseEmojiText } from '@/utils/twemoji';

export type EmojiRenderInfo = {
  src: string;
  fallback?: string;
  isCustom: boolean;
  asText?: boolean;
};

export const isCustomEmojiValue = (value: string): boolean => {
  const trimmed = value?.trim() || '';
  return trimmed.startsWith('id:');
};

export const normalizeCustomEmojiValue = (value: string): string => {
  const trimmed = value?.trim() || '';
  if (!trimmed) return '';
  if (trimmed.startsWith('id:')) {
    return `id:${normalizeAttachmentId(trimmed)}`;
  }
  return trimmed;
};

export const buildEmojiRenderInfo = (value: string): EmojiRenderInfo => {
  const normalized = normalizeCustomEmojiValue(value);
  if (isCustomEmojiValue(normalized)) {
    return {
      src: resolveAttachmentUrl(normalized),
      isCustom: true,
    };
  }
  if (shouldUseEmojiText(normalized)) {
    return {
      src: '',
      fallback: '',
      isCustom: false,
      asText: true,
    };
  }
  return {
    src: getEmojiUrl(normalized),
    fallback: getFallbackUrl(normalized),
    isCustom: false,
  };
};
