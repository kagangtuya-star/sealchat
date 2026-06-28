export interface SceneListRequestState {
  canManage: boolean;
  isSystemAdmin: boolean;
  currentWorldId: string | null;
  sceneFilters: {
    query?: string;
    tags?: string[];
    folderId?: string | null;
  };
  page: number;
  pageSize: number;
}

export function buildSceneListRequestParams(state: SceneListRequestState): Record<string, unknown> {
  const params: Record<string, unknown> = {
    ...state.sceneFilters,
    page: state.page,
    pageSize: state.pageSize,
  };
  if (!params.folderId) {
    delete params.folderId;
  }
  if (!params.query) {
    delete params.query;
  }
  if (state.currentWorldId) {
    params.scope = 'world';
    params.worldId = state.currentWorldId;
    params.includeCommon = false;
  }
  return params;
}

export function shouldAutoplayLoadedTrack(isPlaying: boolean, muted: boolean): boolean {
  return isPlaying && !muted;
}
