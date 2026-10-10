import { resolveTemplateValue } from '@/utils/characterCardTemplate';
import type { CharacterSnapshotNumericSource, TheaterCharacterStatTemplate } from '@/stores/channelCharacterSnapshot';

export const MAX_STAT_ICONS = 100;

export interface ResolvedCharacterStat {
  id: string;
  name: string;
  current: number | null;
  max: number | null;
  min: number | null;
  barColor?: string;
  textColor?: string;
  fillLeft: number;
  fillWidth: number;
  zeroLeft: number;
  displayMode: 'bar' | 'icon';
  iconType: 'text' | 'image';
  iconValue: string;
  valuePerIcon: number;
  subdivisions: number;
  iconCount: number;
  iconFills: number[];
}

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

export const resolveCharacterNumericSource = (source: CharacterSnapshotNumericSource | undefined, attrs: Record<string, any>): number | null => {
  if (!source) return null;
  const raw = 'value' in source ? source.value : resolveTemplateValue(attrs, source.path);
  if (raw === null || raw === undefined || raw === '') return null;
  const value = typeof raw === 'number' ? raw : Number(String(raw).trim());
  return Number.isFinite(value) ? value : null;
};

export const resolveCharacterStat = (
  template: TheaterCharacterStatTemplate,
  attrs: Record<string, any>,
  includeUnset = false,
): ResolvedCharacterStat | null => {
  const current = resolveCharacterNumericSource(template.current, attrs);
  const max = resolveCharacterNumericSource(template.max, attrs);
  const min = resolveCharacterNumericSource(template.min, attrs);
  if (current === null && !includeUnset) return null;
  const displayMode: ResolvedCharacterStat['displayMode'] = template.display?.mode === 'icon' ? 'icon' : 'bar';
  const configuredIconValue = String(template.display?.icon?.value || '').trim();
  const iconType: ResolvedCharacterStat['iconType'] = template.display?.icon?.type === 'image' && configuredIconValue ? 'image' : 'text';
  const iconValue = configuredIconValue || '❤️';
  const configuredValuePerIcon = Number(template.display?.valuePerIcon);
  const valuePerIcon = Number.isFinite(configuredValuePerIcon) && configuredValuePerIcon > 0 ? configuredValuePerIcon : 1;
  const configuredSubdivisions = Math.floor(Number(template.display?.subdivisions));
  const subdivisions = Number.isFinite(configuredSubdivisions) && configuredSubdivisions >= 1 ? configuredSubdivisions : 1;
  const base: ResolvedCharacterStat = {
    id: template.id, name: template.name, current, max, min,
    barColor: template.barColor, textColor: template.textColor,
    fillLeft: 0, fillWidth: 0, zeroLeft: 0, displayMode, iconType, iconValue,
    valuePerIcon, subdivisions, iconCount: 0, iconFills: [],
  };
  if (current === null) return base;
  const rangeMin = min ?? 0;
  if (displayMode === 'icon') {
    if (max === null || max <= rangeMin) return includeUnset ? base : null;
    const iconCount = Math.min(MAX_STAT_ICONS, Math.ceil((max - rangeMin) / valuePerIcon));
    const filledValue = clamp(current, rangeMin, max) - rangeMin;
    return { ...base, iconCount, iconFills: Array.from({ length: iconCount }, (_, index) => {
      const fill = clamp((filledValue - (index * valuePerIcon)) / valuePerIcon, 0, 1);
      return Math.min(1, Math.ceil(fill * subdivisions) / subdivisions);
    }) };
  }
  if (max === null || max <= rangeMin) return { ...base, max: null, fillWidth: 100 };
  const range = max - rangeMin;
  const currentRatio = clamp((current - rangeMin) / range, 0, 1);
  const zeroRatio = clamp((0 - rangeMin) / range, 0, 1);
  return { ...base, fillLeft: Math.min(currentRatio, zeroRatio) * 100,
    fillWidth: Math.abs(currentRatio - zeroRatio) * 100, zeroLeft: zeroRatio * 100 };
};
