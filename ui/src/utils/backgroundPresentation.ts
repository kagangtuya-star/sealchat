import type { CSSProperties } from 'vue';
import type { ChannelBackgroundSettings } from '@/types';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

export const DEFAULT_BACKGROUND_PRESENTATION: ChannelBackgroundSettings = {
  mode: 'cover', opacity: 30, blur: 0, brightness: 100,
  overlayColor: undefined, overlayOpacity: 0,
};

export function clampBackgroundNumber(value: unknown, fallback: number, min: number, max: number): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.min(max, Math.max(min, value)) : fallback;
}

export function normalizeBackgroundPresentationSettings(input: unknown): ChannelBackgroundSettings {
  if (typeof input === 'string') {
    try { input = JSON.parse(input); } catch { input = null; }
  }
  const value = input && typeof input === 'object' ? input as Record<string, unknown> : {};
  return {
    mode: value.mode === 'contain' || value.mode === 'center' || value.mode === 'tile' ? value.mode : 'cover',
    opacity: clampBackgroundNumber(value.opacity, 30, 0, 100),
    blur: clampBackgroundNumber(value.blur, 0, 0, 40),
    brightness: clampBackgroundNumber(value.brightness, 100, 0, 200),
    overlayColor: typeof value.overlayColor === 'string' ? value.overlayColor : undefined,
    overlayOpacity: clampBackgroundNumber(value.overlayOpacity, 0, 0, 100),
  };
}

export function buildBackgroundImageStyle(attachmentId: string | undefined, input: unknown): CSSProperties | null {
  if (!attachmentId) return null;
  const settings = normalizeBackgroundPresentationSettings(input);
  return {
    backgroundImage: `url(${JSON.stringify(resolveAttachmentUrl(attachmentId))})`,
    backgroundSize: settings.mode === 'center' || settings.mode === 'tile' ? 'auto' : settings.mode,
    backgroundRepeat: settings.mode === 'tile' ? 'repeat' : 'no-repeat',
    backgroundPosition: 'center',
    opacity: settings.opacity / 100,
    filter: `blur(${settings.blur}px) brightness(${settings.brightness}%)`,
  };
}

export function buildBackgroundOverlayStyle(input: unknown): CSSProperties | null {
  const settings = normalizeBackgroundPresentationSettings(input);
  if (!settings.overlayColor || !settings.overlayOpacity) return null;
  return { backgroundColor: settings.overlayColor, opacity: settings.overlayOpacity / 100 };
}
