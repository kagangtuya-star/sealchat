import type { TheaterCharacterOverlayTemplate } from '@/stores/channelCharacterSnapshot';

export type CharacterSnapshotTemplatePreset = 'shinobigami' | 'coc';

export const CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS: Record<CharacterSnapshotTemplatePreset, TheaterCharacterOverlayTemplate> = {
  shinobigami: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'shinobigami-instrument', name: '器術', current: { path: '1-$忍神.damageSwitches.器术' }, min: { path: '0' }, max: { path: '1' }, barColor: '#e26b0a', textColor: '#f8fafc' },
      { id: 'shinobigami-body', name: '体術', current: { path: '1-$忍神.damageSwitches.体术' }, max: { path: '1' }, barColor: '#76933c', textColor: '#f8fafc' },
      { id: 'shinobigami-ninja', name: '忍術', current: { path: '1-$忍神.damageSwitches.忍术' }, max: { path: '1' }, barColor: '#c00000', textColor: '#f8fafc' },
      { id: 'shinobigami-scheme', name: '謀術', current: { path: '1-$忍神.damageSwitches.谋术' }, max: { path: '1' }, barColor: '#948a54', textColor: '#f8fafc' },
      { id: 'shinobigami-battle', name: '戦術', current: { path: '1-$忍神.damageSwitches.战术' }, max: { path: '1' }, barColor: '#16365c', textColor: '#f8fafc' },
      { id: 'shinobigami-demon', name: '妖術', current: { path: '1-$忍神.damageSwitches.妖术' }, max: { path: '1' }, barColor: '#60497a', textColor: '#f8fafc' },
    ],
  },
  coc: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'coc-hp', name: 'HP', current: { path: '生命值' }, min: { path: '0' }, max: { path: '生命值上限' }, barColor: '#B73F42', textColor: '#f8fafc' },
      { id: 'coc-mp', name: 'MP', current: { path: '魔法值' }, max: { path: '意志/5' }, barColor: '#436C85', textColor: '#f8fafc' },
      { id: 'coc-san', name: 'SAN', current: { path: '理智' }, max: { path: '意志' }, barColor: '#DE9960', textColor: '#f8fafc' },
      { id: 'coc-luck', name: '幸运', current: { path: '幸运' }, barColor: '#82B29B', textColor: '#f8fafc' },
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
