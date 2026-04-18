import type { AudioAsset } from '../types/audio';

export interface AudioAssetCollectionsState {
  assets: AudioAsset[];
  filteredAssets: AudioAsset[];
  trackSelectableAssets: AudioAsset[];
  selectedAssetId: string | null;
}

function upsertAsset(list: AudioAsset[], asset: AudioAsset) {
  const index = list.findIndex((item) => item.id === asset.id);
  if (index >= 0) {
    const next = list.slice();
    next[index] = { ...next[index], ...asset };
    return next;
  }
  return [asset, ...list];
}

export function upsertAudioAssetCollections(
  state: AudioAssetCollectionsState,
  asset: AudioAsset,
): AudioAssetCollectionsState {
  return {
    assets: upsertAsset(state.assets, asset),
    filteredAssets: upsertAsset(state.filteredAssets, asset),
    trackSelectableAssets: upsertAsset(state.trackSelectableAssets, asset),
    selectedAssetId: state.selectedAssetId || asset.id,
  };
}
