import type { TheaterCharacterOverlayTemplate } from '@/stores/channelCharacterSnapshot';

export type CharacterSnapshotTemplatePreset = 'shinobigami' | 'coc';

export const CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS: Record<CharacterSnapshotTemplatePreset, TheaterCharacterOverlayTemplate> = {
  shinobigami: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'stat_1784894687587_3', name: '器術', current: { path: '1-$忍神.damageSwitches.器术' }, min: { path: '0' }, max: { path: '1' }, barColor: '#e26b0a', textColor: '#f8fafc' },
      { id: 'stat_1784905856482_1', name: '体術', current: { path: '1-$忍神.damageSwitches.体术' }, max: { path: '1' }, barColor: '#76933c', textColor: '#f8fafc' },
      { id: 'stat_1784905861076_2', name: '忍術', current: { path: '1-$忍神.damageSwitches.忍术' }, max: { path: '1' }, barColor: '#c00000', textColor: '#f8fafc' },
      { id: 'stat_1784905870297_3', name: '謀術', current: { path: '1-$忍神.damageSwitches.谋术' }, max: { path: '1' }, barColor: '#948a54', textColor: '#f8fafc' },
      { id: 'stat_1784905871219_4', name: '戦術', current: { path: '1-$忍神.damageSwitches.战术' }, max: { path: '1' }, barColor: '#16365c', textColor: '#f8fafc' },
      { id: 'stat_1784905872110_5', name: '妖術', current: { path: '1-$忍神.damageSwitches.妖术' }, max: { path: '1' }, barColor: '#60497a', textColor: '#f8fafc' },
    ],
  },
  coc: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'stat_1784894687587_3', name: 'HP', current: { path: '生命值' }, min: { path: '0' }, max: { path: '生命值上限' }, barColor: '#B73F42', textColor: '#f8fafc' },
      { id: 'stat_1784905872110_5', name: 'MP', current: { path: '魔法值' }, max: { path: '意志/5' }, barColor: '#436C85', textColor: '#f8fafc' },
      { id: 'stat_1784912214206_2', name: 'SAN', current: { path: '理智' }, max: { path: '意志' }, barColor: '#DE9960', textColor: '#f8fafc' },
      { id: 'stat_1784912230676_3', name: '幸运', current: { path: '幸运' }, barColor: '#82B29B', textColor: '#f8fafc' },
    ],
  },
};

export const CHARACTER_SNAPSHOT_BADGE_TEMPLATE_PRESETS: Record<CharacterSnapshotTemplatePreset, string> = {
  shinobigami: 'HP{生命值} 损{损伤分野}',
  coc: 'HP{生命值} SAN{理智} 魔法{魔法值} 幸运{幸运}',
};

export const getCharacterSnapshotTemplatePreset = (sheetType: string): CharacterSnapshotTemplatePreset | null => {
  const normalized = sheetType.trim().toLowerCase();
  if (normalized === 'coc' || normalized === 'coc7') return 'coc';
  if (normalized === 'shinobigami' || sheetType.trim() === '忍神') return 'shinobigami';
  return null;
};
