"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.writeSplitSessionSnapshot = exports.readSplitSessionSnapshot = exports.normalizeSplitSessionSnapshot = exports.isSplitPaneFilterRestored = exports.isSplitPaneLocationRestored = exports.resolveSplitSessionStorageKey = exports.createDefaultSplitSessionSnapshot = exports.createDefaultSplitSessionPaneSnapshot = exports.createDefaultSplitSessionFilterState = exports.SPLIT_SESSION_STORAGE_KEY_PREFIX = void 0;
exports.SPLIT_SESSION_STORAGE_KEY_PREFIX = 'sealchat.split.session.';
const createDefaultSplitSessionFilterState = () => ({
    icFilter: 'all',
    showArchived: false,
    roleIds: [],
});
exports.createDefaultSplitSessionFilterState = createDefaultSplitSessionFilterState;
const createDefaultSplitSessionPaneSnapshot = () => ({
    mode: 'chat',
    worldId: '',
    channelId: '',
    webUrl: '',
    filterState: (0, exports.createDefaultSplitSessionFilterState)(),
    searchPanelVisible: false,
    stickyNoteVisible: false,
    characterCardVisible: false,
    audioStudioDrawerVisible: false,
    embedPanelActive: false,
});
exports.createDefaultSplitSessionPaneSnapshot = createDefaultSplitSessionPaneSnapshot;
const createDefaultSplitSessionSnapshot = (scopeWorldId) => ({
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
        A: (0, exports.createDefaultSplitSessionPaneSnapshot)(),
        B: (0, exports.createDefaultSplitSessionPaneSnapshot)(),
    },
});
exports.createDefaultSplitSessionSnapshot = createDefaultSplitSessionSnapshot;
const resolveSplitSessionStorageKey = (scopeWorldId) => `${exports.SPLIT_SESSION_STORAGE_KEY_PREFIX}${scopeWorldId.trim()}`;
exports.resolveSplitSessionStorageKey = resolveSplitSessionStorageKey;
const clampSplitRatio = (ratio) => {
    const value = typeof ratio === 'number' ? ratio : Number(ratio);
    if (!Number.isFinite(value))
        return 0.5;
    return Math.min(0.85, Math.max(0.15, value));
};
const normalizePaneId = (value, fallback) => value === 'A' || value === 'B' ? value : fallback;
const normalizeOperationTarget = (value, fallback) => (value === 'A' || value === 'B' || value === 'follow' ? value : fallback);
const normalizeFilterState = (value) => {
    const raw = typeof value === 'object' && value !== null ? value : {};
    const roleIdsRaw = Array.isArray(raw.roleIds) ? raw.roleIds : [];
    return {
        icFilter: raw.icFilter === 'ic' || raw.icFilter === 'ooc' || raw.icFilter === 'all' ? raw.icFilter : 'all',
        showArchived: !!raw.showArchived,
        roleIds: roleIdsRaw.map((id) => String(id ?? '').trim()).filter(Boolean),
    };
};
const normalizePaneSnapshot = (value) => {
    const raw = typeof value === 'object' && value !== null ? value : {};
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
    };
};
const isSplitPaneLocationRestored = (expected, observed) => {
    if (expected.mode !== observed.mode)
        return false;
    if (expected.mode === 'web')
        return true;
    if ((expected.worldId || '') !== (observed.worldId || ''))
        return false;
    if ((expected.channelId || '').trim()) {
        if ((expected.channelId || '') !== (observed.channelId || ''))
            return false;
    }
    return true;
};
exports.isSplitPaneLocationRestored = isSplitPaneLocationRestored;
const isSplitPaneFilterRestored = (expected, observed) => {
    const observedFilter = observed.filterState;
    if (!observedFilter)
        return false;
    const expectedFilter = normalizeFilterState(expected.filterState);
    const actualFilter = normalizeFilterState(observedFilter);
    if (expectedFilter.icFilter !== actualFilter.icFilter)
        return false;
    if (expectedFilter.showArchived !== actualFilter.showArchived)
        return false;
    if (expectedFilter.roleIds.length !== actualFilter.roleIds.length)
        return false;
    const expectedRoleIds = [...expectedFilter.roleIds].sort();
    const actualRoleIds = [...actualFilter.roleIds].sort();
    for (let i = 0; i < expectedRoleIds.length; i += 1) {
        if (expectedRoleIds[i] !== actualRoleIds[i])
            return false;
    }
    return true;
};
exports.isSplitPaneFilterRestored = isSplitPaneFilterRestored;
const normalizeSplitSessionSnapshot = (scopeWorldId, value) => {
    const normalizedScopeWorldId = scopeWorldId.trim();
    if (!normalizedScopeWorldId)
        return null;
    if (typeof value !== 'object' || value === null)
        return null;
    const raw = value;
    if (raw.version !== 1)
        return null;
    const fallback = (0, exports.createDefaultSplitSessionSnapshot)(normalizedScopeWorldId);
    const shell = typeof raw.shell === 'object' && raw.shell !== null ? raw.shell : fallback.shell;
    const panes = typeof raw.panes === 'object' && raw.panes !== null ? raw.panes : fallback.panes;
    return {
        version: 1,
        scopeWorldId: normalizedScopeWorldId,
        updatedAt: typeof raw.updatedAt === 'number' && Number.isFinite(raw.updatedAt) ? raw.updatedAt : 0,
        shell: {
            activePaneId: normalizePaneId(shell.activePaneId, fallback.shell.activePaneId),
            operationTarget: normalizeOperationTarget(shell.operationTarget, fallback.shell.operationTarget),
            audioPlaybackTarget: normalizeOperationTarget(shell.audioPlaybackTarget, fallback.shell.audioPlaybackTarget),
            lockSameWorld: !!shell.lockSameWorld,
            notifyOwnerPaneId: shell.notifyOwnerPaneId === 'A' || shell.notifyOwnerPaneId === 'B'
                ? shell.notifyOwnerPaneId
                : null,
            webTargetPaneId: normalizePaneId(shell.webTargetPaneId, fallback.shell.webTargetPaneId),
            sidebarCollapsed: !!shell.sidebarCollapsed,
            splitRatio: clampSplitRatio(shell.splitRatio),
            actionRibbonVisible: !!shell.actionRibbonVisible,
        },
        panes: {
            A: normalizePaneSnapshot(panes.A),
            B: normalizePaneSnapshot(panes.B),
        },
    };
};
exports.normalizeSplitSessionSnapshot = normalizeSplitSessionSnapshot;
const resolveDefaultStorage = () => {
    if (typeof window === 'undefined')
        return null;
    return window.localStorage;
};
const readSplitSessionSnapshot = (scopeWorldId, storage = resolveDefaultStorage()) => {
    const normalizedScopeWorldId = scopeWorldId.trim();
    if (!normalizedScopeWorldId)
        return null;
    try {
        const raw = storage?.getItem((0, exports.resolveSplitSessionStorageKey)(normalizedScopeWorldId));
        if (!raw)
            return null;
        return (0, exports.normalizeSplitSessionSnapshot)(normalizedScopeWorldId, JSON.parse(raw));
    }
    catch {
        return null;
    }
};
exports.readSplitSessionSnapshot = readSplitSessionSnapshot;
const writeSplitSessionSnapshot = (scopeWorldId, snapshot, storage = resolveDefaultStorage()) => {
    const normalizedScopeWorldId = scopeWorldId.trim();
    if (!normalizedScopeWorldId)
        return false;
    const normalized = (0, exports.normalizeSplitSessionSnapshot)(normalizedScopeWorldId, snapshot);
    if (!normalized)
        return false;
    try {
        storage?.setItem((0, exports.resolveSplitSessionStorageKey)(normalizedScopeWorldId), JSON.stringify(normalized));
        return true;
    }
    catch {
        return false;
    }
};
exports.writeSplitSessionSnapshot = writeSplitSessionSnapshot;
