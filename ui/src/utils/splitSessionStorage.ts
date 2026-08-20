export type SplitPaneId = 'A' | 'B'
export type SplitPaneMode = 'chat' | 'web'
export type SplitOperationTarget = 'follow' | SplitPaneId

export interface SplitSessionFilterState {
  icFilter: 'all' | 'ic' | 'ooc'
  showArchived: boolean
  roleIds: string[]
  whisperOnly: boolean
  fromTime: number | null
  toTime: number | null
}

export interface SplitSessionPaneSnapshot {
  mode: SplitPaneMode
  worldId: string
  channelId: string
  webUrl: string
  filterState: SplitSessionFilterState
  identityId: string
  identityVariantId: string
  searchPanelVisible: boolean
  stickyNoteVisible: boolean
  characterCardVisible: boolean
  audioStudioDrawerVisible: boolean
  embedPanelActive: boolean
}

export interface IcOocSplitEntryChannelState {
  mode: 'ic' | 'ooc'
  identityId: string
  identityVariantId: string
  filterState?: SplitSessionFilterState
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
  whisperOnly: false,
  fromTime: null,
  toTime: null,
})

export const createDefaultSplitSessionPaneSnapshot = (): SplitSessionPaneSnapshot => ({
  mode: 'chat',
  worldId: '',
  channelId: '',
  webUrl: '',
  filterState: createDefaultSplitSessionFilterState(),
  identityId: '',
  identityVariantId: '',
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

export const buildIcOocSplitScopeWorldId = (worldId: string): string => {
  const normalizedWorldId = worldId.trim()
  return normalizedWorldId ? `${normalizedWorldId}::preset:ic-ooc` : ''
}

export const createIcOocSplitSessionSnapshot = (
  scopeWorldId: string,
  worldId: string,
  channelId: string,
  layout: 'ic-left' | 'ooc-left' = 'ic-left',
): SplitSessionSnapshot => {
  const normalizedScopeWorldId = scopeWorldId.trim()
  const normalizedWorldId = worldId.trim()
  const normalizedChannelId = channelId.trim()
  const leftIcFilter = layout === 'ooc-left' ? 'ooc' : 'ic'
  const rightIcFilter = layout === 'ooc-left' ? 'ic' : 'ooc'
  const snapshot = createDefaultSplitSessionSnapshot(normalizedScopeWorldId)
  snapshot.updatedAt = Date.now()
  snapshot.panes = {
    A: {
      ...snapshot.panes.A,
      worldId: normalizedWorldId,
      channelId: normalizedChannelId,
      filterState: {
        ...snapshot.panes.A.filterState,
        icFilter: leftIcFilter,
      },
    },
    B: {
      ...snapshot.panes.B,
      worldId: normalizedWorldId,
      channelId: normalizedChannelId,
      filterState: {
        ...snapshot.panes.B.filterState,
        icFilter: rightIcFilter,
      },
    },
  }
  return snapshot
}

const isIcOocSplitPaneFilter = (value: unknown): value is 'ic' | 'ooc' => value === 'ic' || value === 'ooc'

export const resolveIcOocSplitSessionSnapshot = (
  scopeWorldId: string,
  worldId: string,
  channelId: string,
  layout: 'ic-left' | 'ooc-left' = 'ic-left',
  existingSnapshot?: SplitSessionSnapshot | null,
  entryChannelState?: IcOocSplitEntryChannelState | null,
): SplitSessionSnapshot => {
  const normalizedExisting = existingSnapshot
    ? normalizeSplitSessionSnapshot(scopeWorldId, existingSnapshot)
    : null
  const normalizedWorldId = worldId.trim()
  const normalizedChannelId = channelId.trim()
  const expectedAFilter = layout === 'ooc-left' ? 'ooc' : 'ic'
  const expectedBFilter = layout === 'ooc-left' ? 'ic' : 'ooc'
  const canReuseIdentityState =
    normalizedExisting
    && isIcOocSplitPaneFilter(normalizedExisting.panes.A.filterState.icFilter)
    && isIcOocSplitPaneFilter(normalizedExisting.panes.B.filterState.icFilter)
    && normalizedExisting.panes.A.filterState.icFilter !== normalizedExisting.panes.B.filterState.icFilter
    && normalizedExisting.panes.A.worldId === normalizedWorldId
    && normalizedExisting.panes.B.worldId === normalizedWorldId
    && normalizedExisting.panes.A.channelId === normalizedChannelId
    && normalizedExisting.panes.B.channelId === normalizedChannelId
  const snapshot: SplitSessionSnapshot = canReuseIdentityState
    ? {
      ...normalizedExisting,
      panes: {
        A: {
          ...normalizedExisting.panes.A,
          worldId: normalizedWorldId,
          channelId: normalizedChannelId,
          filterState: {
            ...normalizedExisting.panes.A.filterState,
            icFilter: expectedAFilter,
          },
        },
        B: {
          ...normalizedExisting.panes.B,
          worldId: normalizedWorldId,
          channelId: normalizedChannelId,
          filterState: {
            ...normalizedExisting.panes.B.filterState,
            icFilter: expectedBFilter,
          },
        },
      },
    }
    : createIcOocSplitSessionSnapshot(scopeWorldId, worldId, channelId, layout)
  snapshot.panes.A.filterState.icFilter = expectedAFilter
  snapshot.panes.B.filterState.icFilter = expectedBFilter
  if (entryChannelState) {
    const targetPaneId = entryChannelState.mode === expectedAFilter ? 'A' : 'B'
    const targetPane = snapshot.panes[targetPaneId]
    const normalizedEntryFilterState = normalizeFilterState(entryChannelState.filterState)
    targetPane.identityId = normalizeOptionalId(entryChannelState.identityId)
    targetPane.identityVariantId = targetPane.identityId
      ? normalizeOptionalId(entryChannelState.identityVariantId)
      : ''
    targetPane.filterState = {
      ...targetPane.filterState,
      showArchived: normalizedEntryFilterState.showArchived,
      roleIds: normalizedEntryFilterState.roleIds,
      whisperOnly: normalizedEntryFilterState.whisperOnly,
      fromTime: normalizedEntryFilterState.fromTime,
      toTime: normalizedEntryFilterState.toTime,
      icFilter: targetPane.filterState.icFilter,
    }
  }
  return snapshot
}

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
    whisperOnly: !!raw.whisperOnly,
    fromTime: typeof raw.fromTime === 'number' && Number.isFinite(raw.fromTime) ? raw.fromTime : null,
    toTime: typeof raw.toTime === 'number' && Number.isFinite(raw.toTime) ? raw.toTime : null,
  }
}

const normalizeOptionalId = (value: unknown): string => typeof value === 'string' ? value.trim() : ''

const normalizePaneSnapshot = (value: unknown): SplitSessionPaneSnapshot => {
  const raw = typeof value === 'object' && value !== null ? value as Partial<SplitSessionPaneSnapshot> : {}
  return {
    mode: raw.mode === 'web' ? 'web' : 'chat',
    worldId: typeof raw.worldId === 'string' ? raw.worldId : '',
    channelId: typeof raw.channelId === 'string' ? raw.channelId : '',
    webUrl: typeof raw.webUrl === 'string' ? raw.webUrl : '',
    filterState: normalizeFilterState(raw.filterState),
    identityId: normalizeOptionalId(raw.identityId),
    identityVariantId: normalizeOptionalId(raw.identityVariantId),
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
