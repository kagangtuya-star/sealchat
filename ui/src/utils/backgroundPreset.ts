import type { BackgroundPreset, ChannelBackgroundSettings } from '@/types';

const STORAGE_KEY_PREFIX = 'sealchat_bg_presets_';
const CATEGORIES_KEY_PREFIX = 'sealchat_bg_categories_';
const DEFAULT_SETTINGS: ChannelBackgroundSettings = {
  mode: 'cover',
  opacity: 30,
  blur: 0,
  brightness: 100,
  overlayColor: undefined,
  overlayOpacity: 0,
};

const normalizeSettings = (input: unknown): ChannelBackgroundSettings => {
  if (!input) return { ...DEFAULT_SETTINGS };
  if (typeof input === 'string') {
    try {
      return { ...DEFAULT_SETTINGS, ...(JSON.parse(input) as ChannelBackgroundSettings) };
    } catch {
      return { ...DEFAULT_SETTINGS };
    }
  }
  if (typeof input !== 'object') return { ...DEFAULT_SETTINGS };
  return { ...DEFAULT_SETTINGS, ...(input as ChannelBackgroundSettings) };
};

const normalizePreset = (input: unknown): BackgroundPreset | null => {
  if (!input || typeof input !== 'object') return null;
  const preset = input as BackgroundPreset;
  if (!preset.id || !preset.name || !preset.attachmentId) return null;
  return {
    id: String(preset.id),
    name: String(preset.name),
    category: typeof preset.category === 'string' ? preset.category : undefined,
    attachmentId: String(preset.attachmentId),
    thumbnailUrl: typeof preset.thumbnailUrl === 'string' ? preset.thumbnailUrl : undefined,
    settings: normalizeSettings((preset as { settings?: unknown }).settings),
    createdAt: typeof preset.createdAt === 'number' ? preset.createdAt : Date.now(),
  };
};

export function getStorageKey(channelId: string): string {
  return `${STORAGE_KEY_PREFIX}${channelId}`;
}

export function getCategoriesKey(channelId: string): string {
  return `${CATEGORIES_KEY_PREFIX}${channelId}`;
}

export function loadPresets(channelId: string): BackgroundPreset[] {
  try {
    const raw = localStorage.getItem(getStorageKey(channelId));
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map((item) => normalizePreset(item))
      .filter((item): item is BackgroundPreset => Boolean(item));
  } catch {
    return [];
  }
}

export function savePresets(channelId: string, presets: BackgroundPreset[]): void {
  localStorage.setItem(getStorageKey(channelId), JSON.stringify(presets));
}

export function loadCategories(channelId: string): string[] {
  try {
    const raw = localStorage.getItem(getCategoriesKey(channelId));
    if (!raw) return [];
    return JSON.parse(raw) as string[];
  } catch {
    return [];
  }
}

export function saveCategories(channelId: string, categories: string[]): void {
  localStorage.setItem(getCategoriesKey(channelId), JSON.stringify(categories));
}

export function generatePresetId(): string {
  return `preset_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
}

export function createPreset(
  attachmentId: string,
  settings: ChannelBackgroundSettings,
  name: string,
  category?: string,
  thumbnailUrl?: string
): BackgroundPreset {
  return {
    id: generatePresetId(),
    name,
    category,
    attachmentId,
    thumbnailUrl,
    settings: { ...settings },
    createdAt: Date.now(),
  };
}

export function addPreset(channelId: string, preset: BackgroundPreset): BackgroundPreset[] {
  const presets = loadPresets(channelId);
  presets.unshift(preset);
  savePresets(channelId, presets);
  return presets;
}

export function updatePreset(
  channelId: string,
  presetId: string,
  updates: Partial<Pick<BackgroundPreset, 'name' | 'category'>>
): BackgroundPreset[] {
  const presets = loadPresets(channelId);
  const idx = presets.findIndex((p) => p.id === presetId);
  if (idx !== -1) {
    presets[idx] = { ...presets[idx], ...updates };
    savePresets(channelId, presets);
  }
  return presets;
}

export function deletePreset(channelId: string, presetId: string): BackgroundPreset[] {
  const presets = loadPresets(channelId).filter((p) => p.id !== presetId);
  savePresets(channelId, presets);
  return presets;
}

export function addCategory(channelId: string, category: string): string[] {
  const categories = loadCategories(channelId);
  if (!categories.includes(category)) {
    categories.push(category);
    saveCategories(channelId, categories);
  }
  return categories;
}

export function deleteCategory(channelId: string, category: string): string[] {
  const categories = loadCategories(channelId).filter((c) => c !== category);
  saveCategories(channelId, categories);
  return categories;
}
