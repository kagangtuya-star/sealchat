export type SplitPaneId = 'A' | 'B'
export type SplitPaneMode = 'chat' | 'web'
export type SplitOperationTarget = 'follow' | SplitPaneId

export interface SplitSessionFilterState {
  icFilter: 'all' | 'ic' | 'ooc'
  showArchived: boolean
  roleIds: string[]
}

export interface SplitSessionPaneSnapshot {
  mode: SplitPaneMode
  worldId: string
  channelId: string
  webUrl: string
  filterState: SplitSessionFilterState
  searchPanelVisible: boolean
  stickyNoteVisible: boolean
  characterCardVisible: boolean
  audioStudioDrawerVisible: boolean
  embedPanelActive: boolean
}

export interface SplitSessionSnapshot {
  version: 1
  scopeWorldId: string
  updatedAt: number
  shell: {
    activePaneId: SplitPaneId
    operationTarget: SplitOperationTarget
    audioPlaybackTarget: SplitOperationTarget
    lockSameWorld: boolean
    notifyOwnerPaneId: SplitPaneId | null
    webTargetPaneId: SplitPaneId
    sidebarCollapsed: boolean
    splitRatio: number
    actionRibbonVisible: boolean
  }
  panes: {
    A: SplitSessionPaneSnapshot
    B: SplitSessionPaneSnapshot
  }
}

export const SPLIT_SESSION_STORAGE_KEY_PREFIX = 'sealchat.split.session.'

export const createDefaultSplitSessionFilterState = (): SplitSessionFilterState => ({
  icFilter: 'all',
  showArchived: false,
  roleIds: [],
})

export const createDefaultSplitSessionPaneSnapshot = (): SplitSessionPaneSnapshot => ({
  mode: 'chat',
  worldId: '',
  channelId: '',
  webUrl: '',
  filterState: createDefaultSplitSessionFilterState(),
  searchPanelVisible: false,
  stickyNoteVisible: false,
  characterCardVisible: false,
  audioStudioDrawerVisible: false,
  embedPanelActive: false,
})

export const createDefaultSplitSessionSnapshot = (scopeWorldId: string): SplitSessionSnapshot => ({
  version: 1,
  scopeWorldId,
  updatedAt: 0,
  shell: {
    activePaneId: 'A',
    operationTarget: 'follow',
    audioPlaybackTarget: 'A',
    lockSameWorld: false,
    notifyOwnerPaneId: null,
    webTargetPaneId: 'A',
    sidebarCollapsed: false,
    splitRatio: 0.5,
    actionRibbonVisible: false,
  },
  panes: {
    A: createDefaultSplitSessionPaneSnapshot(),
    B: createDefaultSplitSessionPaneSnapshot(),
  },
})

export interface SplitSessionPaneRestoreObservedState {
  mode: SplitPaneMode
  worldId: string
  channelId: string
  filterState?: SplitSessionFilterState
}

export const resolveSplitSessionStorageKey = (scopeWorldId: string): string => `${SPLIT_SESSION_STORAGE_KEY_PREFIX}${scopeWorldId.trim()}`

const clampSplitRatio = (ratio: unknown): number => {
  const value = typeof ratio === 'number' ? ratio : Number(ratio)
  if (!Number.isFinite(value)) return 0.5
  return Math.min(0.85, Math.max(0.15, value))
}

const normalizePaneId = (value: unknown, fallback: SplitPaneId): SplitPaneId => value === 'A' || value === 'B' ? value : fallback

const normalizeOperationTarget = (value: unknown, fallback: SplitOperationTarget): SplitOperationTarget => (
  value === 'A' || value === 'B' || value === 'follow' ? value : fallback
)

const normalizeFilterState = (value: unknown): SplitSessionFilterState => {
  const raw = typeof value === 'object' && value !== null ? value as Partial<SplitSessionFilterState> : {}
  const roleIdsRaw = Array.isArray(raw.roleIds) ? raw.roleIds : []
  return {
    icFilter: raw.icFilter === 'ic' || raw.icFilter === 'ooc' || raw.icFilter === 'all' ? raw.icFilter : 'all',
    showArchived: !!raw.showArchived,
    roleIds: roleIdsRaw.map((id) => String(id ?? '').trim()).filter(Boolean),
  }
}

const normalizePaneSnapshot = (value: unknown): SplitSessionPaneSnapshot => {
  const raw = typeof value === 'object' && value !== null ? value as Partial<SplitSessionPaneSnapshot> : {}
  return {
    mode: raw.mode === 'web' ? 'web' : 'chat',
    worldId: typeof raw.worldId === 'string' ? raw.worldId : '',
    channelId: typeof raw.channelId === 'string' ? raw.channelId : '',
    webUrl: typeof raw.webUrl === 'string' ? raw.webUrl : '',
    filterState: normalizeFilterState(raw.filterState),
    searchPanelVisible: !!raw.searchPanelVisible,
    stickyNoteVisible: !!raw.stickyNoteVisible,
    characterCardVisible: !!raw.characterCardVisible,
    audioStudioDrawerVisible: !!raw.audioStudioDrawerVisible,
    embedPanelActive: !!raw.embedPanelActive,
  }
}

export const isSplitPaneLocationRestored = (
  expected: SplitSessionPaneSnapshot,
  observed: SplitSessionPaneRestoreObservedState,
): boolean => {
  if (expected.mode !== observed.mode) return false
  if (expected.mode === 'web') return true
  if ((expected.worldId || '') !== (observed.worldId || '')) return false
  if ((expected.channelId || '').trim()) {
    if ((expected.channelId || '') !== (observed.channelId || '')) return false
  }
  return true
}

export const isSplitPaneFilterRestored = (
  expected: SplitSessionPaneSnapshot,
  observed: SplitSessionPaneRestoreObservedState,
): boolean => {
  const observedFilter = observed.filterState
  if (!observedFilter) return false
  const expectedFilter = normalizeFilterState(expected.filterState)
  const actualFilter = normalizeFilterState(observedFilter)
  if (expectedFilter.icFilter !== actualFilter.icFilter) return false
  if (expectedFilter.showArchived !== actualFilter.showArchived) return false
  if (expectedFilter.roleIds.length !== actualFilter.roleIds.length) return false
  const expectedRoleIds = [...expectedFilter.roleIds].sort()
  const actualRoleIds = [...actualFilter.roleIds].sort()
  for (let i = 0; i < expectedRoleIds.length; i += 1) {
    if (expectedRoleIds[i] !== actualRoleIds[i]) return false
  }
  return true
}

export const normalizeSplitSessionSnapshot = (scopeWorldId: string, value: unknown): SplitSessionSnapshot | null => {
  const normalizedScopeWorldId = scopeWorldId.trim()
  if (!normalizedScopeWorldId) return null
  if (typeof value !== 'object' || value === null) return null
  const raw = value as Partial<SplitSessionSnapshot>
  if (raw.version !== 1) return null
  const fallback = createDefaultSplitSessionSnapshot(normalizedScopeWorldId)
  const shell = typeof raw.shell === 'object' && raw.shell !== null ? raw.shell : fallback.shell
  const panes = typeof raw.panes === 'object' && raw.panes !== null ? raw.panes : fallback.panes
  return {
    version: 1,
    scopeWorldId: normalizedScopeWorldId,
    updatedAt: typeof raw.updatedAt === 'number' && Number.isFinite(raw.updatedAt) ? raw.updatedAt : 0,
    shell: {
      activePaneId: normalizePaneId((shell as any).activePaneId, fallback.shell.activePaneId),
      operationTarget: normalizeOperationTarget((shell as any).operationTarget, fallback.shell.operationTarget),
      audioPlaybackTarget: normalizeOperationTarget((shell as any).audioPlaybackTarget, fallback.shell.audioPlaybackTarget),
      lockSameWorld: !!(shell as any).lockSameWorld,
      notifyOwnerPaneId: (shell as any).notifyOwnerPaneId === 'A' || (shell as any).notifyOwnerPaneId === 'B'
        ? (shell as any).notifyOwnerPaneId
        : null,
      webTargetPaneId: normalizePaneId((shell as any).webTargetPaneId, fallback.shell.webTargetPaneId),
      sidebarCollapsed: !!(shell as any).sidebarCollapsed,
      splitRatio: clampSplitRatio((shell as any).splitRatio),
      actionRibbonVisible: !!(shell as any).actionRibbonVisible,
    },
    panes: {
      A: normalizePaneSnapshot((panes as any).A),
      B: normalizePaneSnapshot((panes as any).B),
    },
  }
}

type ReadStorage = Pick<Storage, 'getItem'> | null | undefined
type WriteStorage = Pick<Storage, 'setItem'> | null | undefined

const resolveDefaultStorage = (): Storage | null => {
  if (typeof window === 'undefined') return null
  return window.localStorage
}

export const readSplitSessionSnapshot = (
  scopeWorldId: string,
  storage: ReadStorage = resolveDefaultStorage(),
): SplitSessionSnapshot | null => {
  const normalizedScopeWorldId = scopeWorldId.trim()
  if (!normalizedScopeWorldId) return null
  try {
    const raw = storage?.getItem(resolveSplitSessionStorageKey(normalizedScopeWorldId))
    if (!raw) return null
    return normalizeSplitSessionSnapshot(normalizedScopeWorldId, JSON.parse(raw))
  } catch {
    return null
  }
}

export const writeSplitSessionSnapshot = (
  scopeWorldId: string,
  snapshot: SplitSessionSnapshot,
  storage: WriteStorage = resolveDefaultStorage(),
): boolean => {
  const normalizedScopeWorldId = scopeWorldId.trim()
  if (!normalizedScopeWorldId) return false
  const normalized = normalizeSplitSessionSnapshot(normalizedScopeWorldId, snapshot)
  if (!normalized) return false
  try {
    storage?.setItem(resolveSplitSessionStorageKey(normalizedScopeWorldId), JSON.stringify(normalized))
    return true
  } catch {
    return false
  }
}
