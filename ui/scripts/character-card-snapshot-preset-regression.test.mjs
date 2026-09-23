import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const characterCardSource = readFileSync(new URL('../src/stores/characterCard.ts', import.meta.url), 'utf8');
const panelSource = readFileSync(new URL('../src/views/chat/components/CharacterCardPanel.vue', import.meta.url), 'utf8');

const tagCardSource = characterCardSource.slice(
  characterCardSource.indexOf('const tagCard = async'),
  characterCardSource.indexOf('// Unbind card from all channels'),
);

assert.ok(tagCardSource.includes('await getActiveCard(channelId)'));
assert.ok(!tagCardSource.includes('updatePreference'));
assert.ok(!characterCardSource.includes('applySnapshotTemplatePresetForCard'));
assert.match(panelSource, /const applySnapshotTemplatePreset = async/);
assert.match(panelSource, /theaterOverlayTemplateMode: 'custom'/);

console.log('character card snapshot preset regression checks passed');
