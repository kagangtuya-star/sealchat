import { computed, onBeforeUnmount, onMounted, reactive, readonly, watch } from 'vue';
import { clampBackgroundNumber, normalizeBackgroundPresentationSettings } from '@/utils/backgroundPresentation';
import { normalizeAttachmentId } from '@/composables/useAttachmentResolver';
import { worldGlassState } from '@/composables/useWorldGlassBackground';

export interface GlassBackgroundSettings {
  version: 1;
  enabled: boolean;
  attachmentId: string;
  mode: 'cover' | 'contain' | 'center' | 'tile';
  backgroundOpacity: number;
  backgroundBlur: number;
  backgroundBrightness: number;
  surfaceOpacity: number;
  glassBlur: number;
  saturation: number;
  overlayColor?: string;
  overlayOpacity: number;
}

const STORAGE_KEY = 'sealchat.glass-background.v1';
const defaults: GlassBackgroundSettings = {
  version: 1, enabled: false, attachmentId: '', mode: 'cover',
  backgroundOpacity: 100, backgroundBlur: 0, backgroundBrightness: 95,
  surfaceOpacity: 55, glassBlur: 12, saturation: 110,
  overlayColor: undefined, overlayOpacity: 0,
};

export function normalizeGlassBackgroundSettings(input: unknown): GlassBackgroundSettings {
  if (typeof input === 'string') {
    try { input = JSON.parse(input); } catch { input = null; }
  }
  const v = input && typeof input === 'object' ? input as Record<string, unknown> : {};
  if (v.version !== undefined && v.version !== 1) return { ...defaults };
  const presentation = normalizeBackgroundPresentationSettings(v);
  const number = (key: keyof GlassBackgroundSettings, min: number, max: number) =>
    clampBackgroundNumber(v[key], defaults[key] as number, min, max);
  const id = typeof v.attachmentId === 'string' ? normalizeAttachmentId(v.attachmentId) : '';
  return {
    version: 1, enabled: v.enabled === true,
    attachmentId: /^[\w-]+$/.test(id) ? id : '',
    mode: presentation.mode,
    backgroundOpacity: number('backgroundOpacity', 0, 100),
    backgroundBlur: number('backgroundBlur', 0, 40),
    backgroundBrightness: number('backgroundBrightness', 20, 150),
    surfaceOpacity: number('surfaceOpacity', 35, 95),
    glassBlur: number('glassBlur', 0, 32),
    saturation: number('saturation', 50, 180),
    overlayColor: presentation.overlayColor,
    overlayOpacity: presentation.overlayOpacity ?? 0,
  };
}

function readSettings() {
  try { return normalizeGlassBackgroundSettings(localStorage.getItem(STORAGE_KEY)); }
  catch { return { ...defaults }; }
}

const settings = reactive(readSettings());
const effectiveSettings = computed(() => {
  const remote = worldGlassState.value;
  return remote?.enabled && remote.preset
    ? normalizeGlassBackgroundSettings({ ...remote.preset.settings, attachmentId: remote.preset.attachmentId, enabled: true })
    : settings;
});
let pendingWrite: ReturnType<typeof setTimeout> | undefined;
function flush() {
  if (pendingWrite === undefined) return;
  clearTimeout(pendingWrite);
  pendingWrite = undefined;
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(settings)); } catch { /* Storage may be unavailable. */ }
}

function update(patch: Partial<GlassBackgroundSettings>) {
  Object.assign(settings, normalizeGlassBackgroundSettings({ ...settings, ...patch }));
  clearTimeout(pendingWrite);
  pendingWrite = setTimeout(flush, 180);
}

export function useGlassBackground() {
  const presentation = computed(() => ({
    mode: settings.mode, opacity: settings.backgroundOpacity, blur: settings.backgroundBlur,
    brightness: settings.backgroundBrightness, overlayColor: settings.overlayColor,
    overlayOpacity: settings.overlayOpacity,
  }));
  return {
    settings: readonly(settings), localSettings: readonly(settings), effectiveSettings, worldGlassState, presentation, update,
    clear: () => update({ attachmentId: '' }),
    // Reset only material parameters; keep the selected attachment and enabled state.
    reset: () => update({ ...defaults, attachmentId: settings.attachmentId, enabled: settings.enabled }),
  };
}

// Owned exclusively by the App-level renderer, never by panels or chat instances.
export function useGlassBackgroundRuntime() {
  const properties = ['surface-opacity', 'elevated-opacity', 'input-opacity', 'page-opacity', 'blur', 'saturation'];
  const clean = () => {
    delete document.documentElement.dataset.scGlassBackground;
    properties.forEach(name => document.documentElement.style.removeProperty(`--sc-glass-${name}`));
  };
  const stop = watch(effectiveSettings, (settings) => {
    if (typeof document === 'undefined') return;
    if (!settings.enabled) { clean(); return; }
    const root = document.documentElement;
    root.dataset.scGlassBackground = 'true';
    const values = [
      `${settings.surfaceOpacity}%`, `${Math.min(98, settings.surfaceOpacity + 12)}%`,
      `${Math.min(98, settings.surfaceOpacity + 8)}%`, `${Math.max(10, settings.surfaceOpacity - 45)}%`,
      `${settings.glassBlur}px`, `${settings.saturation}%`,
    ];
    properties.forEach((name, index) => root.style.setProperty(`--sc-glass-${name}`, values[index]));
  }, { immediate: true, deep: true });
  const onStorage = (event: StorageEvent) => {
    if (event.key !== STORAGE_KEY && event.key !== null) return;
    try { if (event.storageArea !== window.localStorage) return; } catch { return; }
    clearTimeout(pendingWrite);
    pendingWrite = undefined;
    Object.assign(settings, normalizeGlassBackgroundSettings(event.newValue));
  };
  onMounted(() => {
    window.addEventListener('storage', onStorage);
    window.addEventListener('pagehide', flush);
  });
  onBeforeUnmount(() => {
    stop(); flush(); clean();
    window.removeEventListener('storage', onStorage);
    window.removeEventListener('pagehide', flush);
  });
}
