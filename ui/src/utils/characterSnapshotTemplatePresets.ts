import type { TheaterCharacterOverlayTemplate } from '@/stores/channelCharacterSnapshot';
import { normalizeCharacterSheetType } from '@/utils/characterSheetType';

export type CharacterSnapshotTemplatePreset = 'shinobigami' | 'coc' | 'daggerheart' | 'ta';

export const CHARACTER_SNAPSHOT_TEMPLATE_PRESET_LABELS: Record<CharacterSnapshotTemplatePreset, string> = {
  coc: 'COC',
  shinobigami: '忍神',
  daggerheart: '匕首之心',
  ta: '三角机构',
};

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
  daggerheart: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'daggerheart-hp', name: '生命', current: { path: '生命' }, min: { path: '0' }, max: { path: '生命上限' }, barColor: '#962f2a', textColor: '#f8fafc' },
      { id: 'daggerheart-stress', name: '压力', current: { path: '压力' }, max: { path: '压力上限' }, barColor: '#3d3a44', textColor: '#f8fafc' },
      { id: 'daggerheart-hope', name: '希望', current: { path: '希望' }, max: { path: '希望上限' }, barColor: '#b8862b', textColor: '#f8fafc' },
      { id: 'daggerheart-armor', name: '护甲', current: { path: '护甲' }, max: { path: '护甲上限' }, barColor: '#1f5a5c', textColor: '#f8fafc' },
    ],
  },
  ta: {
    version: 1,
    preferredColumns: 3,
    items: [
      { id: 'ta-attentiveness', name: '专注', current: { path: '专注' }, max: { path: '专注上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-empathy', name: '共情', current: { path: '共情' }, max: { path: '共情上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-presence', name: '气场', current: { path: '气场' }, max: { path: '气场上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-duplicity', name: '欺瞒', current: { path: '欺瞒' }, max: { path: '欺瞒上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-initiative', name: '主动', current: { path: '主动' }, max: { path: '主动上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-professionalism', name: '专业', current: { path: '专业' }, max: { path: '专业上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-dynamism', name: '活力', current: { path: '活力' }, max: { path: '活力上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-persistence', name: '坚毅', current: { path: '坚毅' }, max: { path: '坚毅上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-subtlety', name: '诡秘', current: { path: '诡秘' }, max: { path: '诡秘上限' }, barColor: '#d71937', textColor: '#f8fafc' },
    ],
  },
};

export const AVATAR_WORLD_STATE_OVERLAY_TEMPLATE_PRESETS: Record<CharacterSnapshotTemplatePreset, TheaterCharacterOverlayTemplate> = {
  shinobigami: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'shinobigami-instrument', name: '器術', current: { path: '器術' }, min: { value: 0 }, max: { value: 1 }, barColor: '#e26b0a', textColor: '#f8fafc' },
      { id: 'shinobigami-body', name: '体術', current: { path: '体術' }, min: { value: 0 }, max: { value: 1 }, barColor: '#76933c', textColor: '#f8fafc' },
      { id: 'shinobigami-ninja', name: '忍術', current: { path: '忍術' }, min: { value: 0 }, max: { value: 1 }, barColor: '#c00000', textColor: '#f8fafc' },
      { id: 'shinobigami-scheme', name: '謀術', current: { path: '謀術' }, min: { value: 0 }, max: { value: 1 }, barColor: '#948a54', textColor: '#f8fafc' },
      { id: 'shinobigami-battle', name: '戦術', current: { path: '戦術' }, min: { value: 0 }, max: { value: 1 }, barColor: '#16365c', textColor: '#f8fafc' },
      { id: 'shinobigami-demon', name: '妖術', current: { path: '妖術' }, min: { value: 0 }, max: { value: 1 }, barColor: '#60497a', textColor: '#f8fafc' },
    ],
  },
  coc: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'coc-hp', name: 'HP', current: { path: '生命值' }, min: { value: 0 }, max: { path: '生命值上限' }, barColor: '#B73F42', textColor: '#f8fafc' },
      { id: 'coc-mp', name: 'MP', current: { path: '魔法值' }, min: { value: 0 }, max: { path: '魔法值上限' }, barColor: '#436C85', textColor: '#f8fafc' },
      { id: 'coc-san', name: 'SAN', current: { path: '理智' }, min: { value: 0 }, max: { path: '理智上限' }, barColor: '#DE9960', textColor: '#f8fafc' },
      { id: 'coc-luck', name: '幸运', current: { path: '幸运' }, min: { value: 0 }, max: { path: '幸运上限' }, barColor: '#82B29B', textColor: '#f8fafc' },
    ],
  },
  daggerheart: {
    version: 1,
    preferredColumns: 2,
    items: [
      { id: 'daggerheart-hp', name: '生命', current: { path: '生命' }, min: { value: 0 }, max: { path: '生命上限' }, barColor: '#962f2a', textColor: '#f8fafc' },
      { id: 'daggerheart-stress', name: '压力', current: { path: '压力' }, min: { value: 0 }, max: { path: '压力上限' }, barColor: '#3d3a44', textColor: '#f8fafc' },
      { id: 'daggerheart-hope', name: '希望', current: { path: '希望' }, min: { value: 0 }, max: { path: '希望上限' }, barColor: '#b8862b', textColor: '#f8fafc' },
      { id: 'daggerheart-armor', name: '护甲', current: { path: '护甲' }, min: { value: 0 }, max: { path: '护甲上限' }, barColor: '#1f5a5c', textColor: '#f8fafc' },
    ],
  },
  ta: {
    version: 1,
    preferredColumns: 3,
    items: [
      { id: 'ta-attentiveness', name: '专注', current: { path: '专注' }, min: { value: 0 }, max: { path: '专注上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-empathy', name: '共情', current: { path: '共情' }, min: { value: 0 }, max: { path: '共情上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-presence', name: '气场', current: { path: '气场' }, min: { value: 0 }, max: { path: '气场上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-duplicity', name: '欺瞒', current: { path: '欺瞒' }, min: { value: 0 }, max: { path: '欺瞒上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-initiative', name: '主动', current: { path: '主动' }, min: { value: 0 }, max: { path: '主动上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-professionalism', name: '专业', current: { path: '专业' }, min: { value: 0 }, max: { path: '专业上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-dynamism', name: '活力', current: { path: '活力' }, min: { value: 0 }, max: { path: '活力上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-persistence', name: '坚毅', current: { path: '坚毅' }, min: { value: 0 }, max: { path: '坚毅上限' }, barColor: '#d71937', textColor: '#f8fafc' },
      { id: 'ta-subtlety', name: '诡秘', current: { path: '诡秘' }, min: { value: 0 }, max: { path: '诡秘上限' }, barColor: '#d71937', textColor: '#f8fafc' },
    ],
  },
};

export const CHARACTER_SNAPSHOT_BADGE_TEMPLATE_PRESETS: Record<CharacterSnapshotTemplatePreset, string> = {
  coc: 'HP{生命值} SAN{理智} 魔法{魔法值} 幸运{幸运}',
  shinobigami: "HP{生命值} 污损{污秽} {损伤分野}",
  daggerheart: '生命{生命}/{生命上限} 压力{压力}/{压力上限} 希望{希望}/{希望上限} 护甲{护甲}/{护甲上限}',
  ta: '嘉奖{嘉奖} 申诫{申诫}',
};

export const getCharacterSnapshotTemplatePreset = (sheetType: string): CharacterSnapshotTemplatePreset | null => {
  const normalized = normalizeCharacterSheetType(sheetType);
  if (normalized === 'coc7') return 'coc';
  if (normalized === 'shinobigami' || normalized === 'daggerheart' || normalized === 'ta') return normalized;
  return null;
};
