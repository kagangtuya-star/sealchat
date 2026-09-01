import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import cocTemplateHtml from '../../../doc/template/sealchat-default-template-v3-coc7th.html?raw';
import shinobigamiTemplateHtml from '../../../doc/template/sealchat-shinobigami-template-v1.html?raw';
import { useCharacterCardStore } from './characterCard';
import { useCharacterCardTemplateStore, type CharacterCardTemplateMode } from './characterCardTemplate';
import { useChatStore } from './chat';
import { useDisplayStore } from './display';
import type { CharacterCard, CharacterCardData } from './characterCard';

export interface CharacterSheetWindow {
  id: string;
  cardId: string;
  cardName: string;
  channelId: string;
  worldId?: string;
  readOnly?: boolean;
  ephemeral?: boolean;
  sheetType?: string;
  attrs: Record<string, any>;
  template: string;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
  zIndex: number;
  isMinimized: boolean;
  mode: 'view' | 'edit';
  bubbleX: number;
  bubbleY: number;
  avatarUrl?: string;
  templateMode?: CharacterCardTemplateMode;
  templateId?: string;
  syncState: CharacterSheetSyncState;
  hasLocalEditsInLock: boolean;
  hasSavedAfterEditEnd: boolean;
  pendingRemoteAttrs?: Record<string, any>;
}

type CharacterSheetSyncState = 'normal' | 'editing_locked' | 'resume_pending';

const TEMPLATE_STORAGE_KEY = 'sealchat_character_sheet_templates';
const WINDOWS_STORAGE_KEY = 'sealchat_character_sheet_windows';
const BUBBLE_POSITIONS_KEY = 'sealchat_sheet_bubble_positions';
const BUBBLE_SIZE = 56;
const MIN_WIDTH = 320;
const MIN_HEIGHT = 240;
const DEFAULT_WIDTH = 480;
const DEFAULT_HEIGHT = 560;
const VIEWPORT_PADDING = 16;
const BUBBLE_PERSIST_THROTTLE = 300;
const WINDOWS_PERSIST_THROTTLE = 300;
const ATTRS_SYNC_THROTTLE = 600;

const isOnlinePreviewCardId = (cardId?: string) => String(cardId || '').startsWith('online:');

const isEphemeralWindowState = (state?: { cardId?: string; readOnly?: boolean; ephemeral?: boolean }) => (
  !!state?.readOnly || !!state?.ephemeral || isOnlinePreviewCardId(state?.cardId)
);

const isAttrsEqual = (a: Record<string, any>, b: Record<string, any>) => {
  try {
    return JSON.stringify(a || {}) === JSON.stringify(b || {});
  } catch {
    return false;
  }
};

let windowIdCounter = 0;

const generateWindowId = () => `sheet-${Date.now()}-${++windowIdCounter}`;

interface PersistedWindowState {
  id: string;
  cardId: string;
  cardName: string;
  channelId: string;
  worldId?: string;
  readOnly?: boolean;
  sheetType?: string;
  attrs: Record<string, any>;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
  zIndex: number;
  isMinimized: boolean;
  mode: 'view' | 'edit';
  bubbleX: number;
  bubbleY: number;
  avatarUrl?: string;
  templateMode?: CharacterCardTemplateMode;
  templateId?: string;
}

const loadWindowStates = (): PersistedWindowState[] => {
  try {
    const raw = localStorage.getItem(WINDOWS_STORAGE_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

const saveWindowStates = (states: PersistedWindowState[]) => {
  try {
    localStorage.setItem(WINDOWS_STORAGE_KEY, JSON.stringify(states));
  } catch (e) {
    console.warn('Failed to save character sheet windows', e);
  }
};

const clearWindowStates = () => {
  try {
    localStorage.removeItem(WINDOWS_STORAGE_KEY);
  } catch (e) {
    console.warn('Failed to clear character sheet windows', e);
  }
};

const loadBubblePositions = (): Record<string, { x: number; y: number }> => {
  try {
    const raw = localStorage.getItem(BUBBLE_POSITIONS_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
};

const saveBubblePositions = (positions: Record<string, { x: number; y: number }>) => {
  try {
    localStorage.setItem(BUBBLE_POSITIONS_KEY, JSON.stringify(positions));
  } catch (e) {
    console.warn('Failed to save bubble positions', e);
  }
};

const getDefaultBubblePosition = (index: number): { x: number; y: number } => {
  const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1200;
  return {
    x: viewportW - BUBBLE_SIZE - VIEWPORT_PADDING,
    y: VIEWPORT_PADDING + index * (BUBBLE_SIZE + 8),
  };
};

const clampBubbleCoords = (x: number, y: number): { x: number; y: number } => {
  const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1200;
  const viewportH = typeof window !== 'undefined' ? window.innerHeight : 800;
  return {
    x: Math.max(0, Math.min(x, viewportW - BUBBLE_SIZE)),
    y: Math.max(0, Math.min(y, viewportH - BUBBLE_SIZE)),
  };
};

const clampWindowCoords = (
  x: number,
  y: number,
  width: number,
  height: number,
): { x: number; y: number } => {
  const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1200;
  const viewportH = typeof window !== 'undefined' ? window.innerHeight : 800;
  const maxX = Math.max(VIEWPORT_PADDING, viewportW - width - VIEWPORT_PADDING);
  const maxY = Math.max(VIEWPORT_PADDING, viewportH - height - VIEWPORT_PADDING);
  return {
    x: Math.min(Math.max(x, VIEWPORT_PADDING), maxX),
    y: Math.min(Math.max(y, VIEWPORT_PADDING), maxY),
  };
};

const DEFAULT_TEMPLATE_MARK = 'sealchat-default-template:v2';
export const LEGACY_TEMPLATE_MARKERS = {
  coc: ['sealchat-default-template:v2-coc-dark', 'sealchat-default-template:v2-coc7th'],
  shinobigami: ['sealchat-shinobigami-template:v1'],
};

const isCocSheetType = (value?: string) => {
  const normalized = (value || '').trim().toLowerCase();
  if (!normalized) return false;
  if (normalized === 'coc') return true;
  return normalized.startsWith('coc');
};

const isShinobigamiSheetType = (value?: string) => {
  const normalized = (value || '').trim().toLowerCase();
  if (!normalized) return false;
  return normalized === 'shinobigami' || normalized === '忍神' || normalized.startsWith('shinobigami');
};

export const isLegacyDefaultTemplate = (template: string, sheetType?: string) => {
  if (!template) return false;
  const normalizedSheetType = (sheetType || '').trim().toLowerCase();
  if (!normalizedSheetType) return false;
  if (isCocSheetType(sheetType)) {
    return LEGACY_TEMPLATE_MARKERS.coc.some(marker => template.includes(marker));
  }
  if (isShinobigamiSheetType(sheetType)) {
    return LEGACY_TEMPLATE_MARKERS.shinobigami.some(marker => template.includes(marker));
  }
  return false;
};

export const normalizeTemplate = (_cardId: string | undefined, template: string, sheetType?: string) => {
  if (!template) return template;
  if (!isLegacyDefaultTemplate(template, sheetType)) return template;
  return getDefaultTemplate(sheetType);
};

const getGenericDefaultTemplate = () => `<!DOCTYPE html>
<!-- ${DEFAULT_TEMPLATE_MARK} -->
<html>
<head>
  <meta charset="UTF-8">
  <style>
    :root {
      --text-primary: #1f2937;
      --text-secondary: #6b7280;
      --bg-hover: #f3f4f6;
      --bg-header: #f9fafb;
      --bg-body: #ffffff;
      --border-color: #e5e7eb;
      --scrollbar-track: rgba(0, 0, 0, 0.04);
      --scrollbar-thumb: rgba(100, 116, 139, 0.4);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --text-primary: #f1f5f9;
        --text-secondary: #94a3b8;
        --bg-hover: rgba(255,255,255,0.05);
        --bg-header: rgba(30,41,59,0.6);
        --bg-body: #0f172a;
        --border-color: rgba(148,163,184,0.2);
        --scrollbar-track: rgba(15, 23, 42, 0.8);
        --scrollbar-thumb: rgba(148, 163, 184, 0.5);
      }
    }
    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
      scrollbar-width: thin;
      scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);
    }
    *::-webkit-scrollbar { width: 6px; height: 6px; }
    *::-webkit-scrollbar-track { background: var(--scrollbar-track); }
    *::-webkit-scrollbar-thumb {
      background: var(--scrollbar-thumb);
      border-radius: 999px;
    }
    body {
      font-family: var(--sc-font-family, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif);
      padding: 16px;
      background: var(--bg-body);
      color: var(--text-primary);
      font-size: 14px;
      line-height: 1.6;
    }
    .card-header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
    .card-avatar {
      width: 48px; height: 48px; border-radius: 50%;
      background: var(--bg-header); color: var(--text-secondary);
      display: flex; align-items: center; justify-content: center;
      font-size: 20px; font-weight: 600; overflow: hidden; flex-shrink: 0;
    }
    .card-avatar img { width: 100%; height: 100%; object-fit: cover; }
    .card-name { font-size: 18px; font-weight: 600; }
    .attrs-table { width: 100%; border-collapse: collapse; }
    .attrs-table th, .attrs-table td {
      padding: 8px 12px; text-align: left;
      border-bottom: 1px solid var(--border-color);
    }
    .attrs-table th { background: var(--bg-header); font-weight: 500; width: 40%; }
    .attrs-table tr:hover { background: var(--bg-hover); }
    .attrs-table th[data-roll] { cursor: pointer; color: #3b82f6; }
    .attrs-table th[data-roll]:hover { text-decoration: underline; }
    .attrs-table td[data-attr] { cursor: pointer; }
    .attrs-table td[data-attr]:hover { background: var(--bg-hover); }
    .attrs-table td.is-editing { background: var(--bg-hover); }
    .inline-editor {
      width: 100%;
      border: 1px solid var(--border-color);
      border-radius: 6px;
      padding: 4px 6px;
      font: inherit;
      color: var(--text-primary);
      background: var(--bg-body);
      outline: none;
    }
    .empty { color: var(--text-secondary); font-style: italic; padding: 20px; text-align: center; }
  </style>
</head>
<body>
  <div id="content"></div>
  <script>
    var _windowId = null;
    var _rollDispatchMode = 'default';
    function normalizeRollDispatchMode(mode) {
      return mode === 'template' ? 'template' : 'default';
    }
    function withRollDispatchMode(roll) {
      return Object.assign({}, roll || {}, { dispatchMode: _rollDispatchMode });
    }
    function escapeHtml(text) {
      var div = document.createElement('div');
      div.textContent = text;
      return div.innerHTML;
    }
    function postEvent(action, payload) {
      if (!_windowId) return;
      window.parent.postMessage({
        type: 'SEALCHAT_EVENT',
        version: 1,
        windowId: _windowId,
        action: action,
        payload: payload
      }, '*');
    }
    window.sealchat = {
      onUpdate: function(cb) {
        window.addEventListener('message', function(e) {
          if (e.source !== window.parent) return;
          if (e.data && e.data.type === 'SEALCHAT_UPDATE') {
            _windowId = e.data.payload.windowId;
            cb(e.data.payload);
          }
        });
      },
      setRollDispatchMode: function(mode) {
        _rollDispatchMode = normalizeRollDispatchMode(mode);
      },
      setRollMode: function(mode) {
        _rollDispatchMode = normalizeRollDispatchMode(mode);
      },
      // 示例：启用模板内直发掷骰（跳过默认掷骰窗口）
      // window.sealchat.setRollDispatchMode('template');
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', {
          roll: withRollDispatchMode({ template: template, label: label || '', args: args || {} })
        });
      },
      updateAttrs: function(attrs) {
        postEvent('UPDATE_ATTRS', { attrs: attrs });
      }
    };
    function render(data) {
      var el = document.getElementById('content');
      if (!data || !data.attrs || Object.keys(data.attrs).length === 0) {
        el.innerHTML = '<div class="empty">暂无属性数据</div>';
        return;
      }
      var avatarHtml = '';
      if (data.avatarUrl) {
        avatarHtml = '<img src="' + escapeHtml(data.avatarUrl) + '" alt="" />';
      } else {
        avatarHtml = escapeHtml((data.name || '?').charAt(0));
      }
      var html = '<div class="card-header">';
      html += '<div class="card-avatar">' + avatarHtml + '</div>';
      html += '<div class="card-name">' + escapeHtml(data.name || '未命名') + '</div>';
      html += '</div>';
      html += '<table class="attrs-table"><tbody>';
      for (var key in data.attrs) {
        if (data.attrs.hasOwnProperty(key)) {
          var val = data.attrs[key];
          var isNumeric = typeof val === 'number' || (typeof val === 'string' && /^-?\\d+(?:\\.\\d+)?$/.test(val));
          if (typeof val === 'object') val = JSON.stringify(val);
          var thAttr = '';
          var tdAttr = '';
          if (isNumeric) {
            thAttr = ' data-roll=".ra {skill}" data-label="' + escapeHtml(key) + '" data-skill="' + escapeHtml(key) + '"';
            tdAttr = ' data-attr="' + escapeHtml(key) + '" data-value="' + escapeHtml(String(val)) + '"';
          }
          html += '<tr><th' + thAttr + '>' + escapeHtml(key) + '</th><td' + tdAttr + '>' + escapeHtml(String(val)) + '</td></tr>';
        }
      }
      html += '</tbody></table>';
      el.innerHTML = html;
    }
    function openInlineEditor(cell) {
      if (!cell || cell.dataset.editing === '1') return;
      var attrKey = cell.dataset.attr;
      var currentValue = cell.dataset.value || '';
      var input = document.createElement('input');
      input.type = 'number';
      input.step = 'any';
      input.value = currentValue;
      input.className = 'inline-editor';
      cell.textContent = '';
      cell.appendChild(input);
      cell.dataset.editing = '1';
      cell.classList.add('is-editing');
      input.focus();
      input.select();

      var commit = function() {
        var nextRaw = String(input.value || '').trim();
        var nextNumber = Number(nextRaw);
        if (!nextRaw || isNaN(nextNumber)) {
          cancel();
          return;
        }
        cell.textContent = String(nextNumber);
        cell.dataset.value = String(nextNumber);
        cell.dataset.editing = '';
        cell.classList.remove('is-editing');
        var patch = {};
        patch[attrKey] = nextNumber;
        postEvent('UPDATE_ATTRS', { attrs: patch });
      };

      var cancel = function() {
        cell.textContent = currentValue;
        cell.dataset.editing = '';
        cell.classList.remove('is-editing');
      };

      input.addEventListener('keydown', function(ev) {
        if (ev.key === 'Enter') {
          ev.preventDefault();
          commit();
        } else if (ev.key === 'Escape') {
          ev.preventDefault();
          cancel();
        }
      });
      input.addEventListener('blur', function() {
        commit();
      });
      input.addEventListener('click', function(ev) { ev.stopPropagation(); });
      input.addEventListener('pointerdown', function(ev) { ev.stopPropagation(); });
    }

    document.addEventListener('click', function(e) {
      var target = e.target;
      while (target && target !== document.body) {
        if (target.dataset && target.dataset.attr) {
          openInlineEditor(target);
          return;
        }
        if (target.dataset && target.dataset.roll) {
          var rect = target.getBoundingClientRect();
          var args = {};
          if (target.dataset.skill) {
            args = { skill: target.dataset.skill };
          }
          postEvent('ROLL_DICE', {
            roll: withRollDispatchMode({
              template: target.dataset.roll,
              label: target.dataset.label || target.innerText || '',
              args: args,
              rect: { top: rect.top, left: rect.left, width: rect.width, height: rect.height }
            })
          });
          return;
        }
        target = target.parentElement;
      }
    });
    sealchat.onUpdate(render);
  </script>
</body>
</html>`;

const getShinobigamiDefaultTemplate = () => shinobigamiTemplateHtml.trim();

const getDefaultTemplate = (sheetType?: string) => (
  isShinobigamiSheetType(sheetType)
    ? getShinobigamiDefaultTemplate()
    : (isCocSheetType(sheetType) ? cocTemplateHtml.trim() : getGenericDefaultTemplate())
);

export const resolveInitialSheetTemplate = (
  cardId: string,
  sheetType: string,
  templateMeta: { readOnly?: boolean; templateText?: string } | undefined,
  managedTemplateContent: string | undefined,
  getTemplate: (cardId: string, sheetType?: string) => string,
  getDefault: (sheetType?: string) => string = getDefaultTemplate,
) => {
  const source = templateMeta?.readOnly && !templateMeta.templateText
    ? getDefault(sheetType)
    : templateMeta?.templateText || managedTemplateContent || getTemplate(cardId, sheetType);
  return normalizeTemplate(cardId, source, sheetType);
};

export const useCharacterSheetStore = defineStore('characterSheet', () => {
  const windows = ref<Record<string, CharacterSheetWindow>>({});
  const activeWindowIds = ref<string[]>([]);
  const maxZIndex = ref(2000);
  const hasRestored = ref(false);
  const cardStore = useCharacterCardStore();
  const templateStore = useCharacterCardTemplateStore();
  const chatStore = useChatStore();
  const displayStore = useDisplayStore();

  interface ApplyManagedTemplateOptions {
    syncWorldLocalBadgeTemplate?: boolean;
  }

  const resolveSheetTypeByCardId = (cardId?: string) => {
    if (!cardId) return '';
    return cardStore.getCardById(cardId)?.sheetType || '';
  };

  const resolveWorldBadgeTemplate = (worldId: string) => {
    if (!worldId) return '';
    const world = (chatStore as any).worldMap?.[worldId];
    const fromMap = typeof world?.characterCardBadgeTemplate === 'string' ? world.characterCardBadgeTemplate.trim() : '';
    if (fromMap) return fromMap;
    const fromDetail = (chatStore as any).worldDetailMap?.[worldId]?.world?.characterCardBadgeTemplate;
    if (typeof fromDetail === 'string' && fromDetail.trim()) {
      return fromDetail.trim();
    }
    return '';
  };

  const syncWorldLocalBadgeTemplate = (
    worldId: string,
    defaultBadgeTemplate: string | undefined,
    enabled: boolean,
  ) => {
    if (!enabled || !worldId) return;
    if (resolveWorldBadgeTemplate(worldId)) return;
    const normalized = String(defaultBadgeTemplate || '').trim();
    if (!normalized) return;
    const current = displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId];
    if ((current || '').trim() === normalized) return;
    displayStore.updateSettings({
      characterCardBadgeTemplateByWorld: {
        ...displayStore.settings.characterCardBadgeTemplateByWorld,
        [worldId]: normalized,
      },
    });
  };

  const activeWindows = computed(() =>
    activeWindowIds.value.map(id => windows.value[id]).filter(Boolean)
  );

  const normalizeSyncState = (win: CharacterSheetWindow) => {
    if (!win.syncState) win.syncState = 'normal';
    if (typeof win.hasLocalEditsInLock !== 'boolean') win.hasLocalEditsInLock = false;
    if (typeof win.hasSavedAfterEditEnd !== 'boolean') win.hasSavedAfterEditEnd = false;
  };

  const loadTemplates = (): Record<string, string> => {
    try {
      const raw = localStorage.getItem(TEMPLATE_STORAGE_KEY);
      const parsed = raw ? JSON.parse(raw) : {};
      let changed = false;
      for (const [cardId, template] of Object.entries(parsed)) {
        const sheetType = resolveSheetTypeByCardId(cardId);
        const normalized = normalizeTemplate(cardId, String(template || ''), sheetType);
        if (normalized !== template) {
          parsed[cardId] = normalized;
          changed = true;
        }
      }
      if (changed) {
        try {
          localStorage.setItem(TEMPLATE_STORAGE_KEY, JSON.stringify(parsed));
        } catch (e) {
          console.warn('Failed to migrate character sheet templates', e);
        }
      }
      return parsed;
    } catch {
      return {};
    }
  };

  const saveTemplate = (cardId: string, template: string) => {
    try {
      const templates = loadTemplates();
      templates[cardId] = template;
      localStorage.setItem(TEMPLATE_STORAGE_KEY, JSON.stringify(templates));
    } catch (e) {
      console.warn('Failed to save character sheet template', e);
    }
  };

  const getTemplate = (cardId: string, sheetType?: string): string => {
    const templates = loadTemplates();
    const stored = templates[cardId];
    const resolvedSheetType = sheetType || resolveSheetTypeByCardId(cardId);
    if (stored) {
      const normalized = normalizeTemplate(cardId, stored, resolvedSheetType);
      if (normalized !== stored) {
        saveTemplate(cardId, normalized);
      }
      return normalized;
    }
    const fallback = getDefaultTemplate(resolvedSheetType);
    const normalized = normalizeTemplate(cardId, fallback, resolvedSheetType);
    if (normalized !== fallback) {
      saveTemplate(cardId, normalized);
    }
    return normalized;
  };

  let windowsPersistTimer: ReturnType<typeof setTimeout> | null = null;

  const persistWindows = () => {
    const states: PersistedWindowState[] = [];
    for (const id of activeWindowIds.value) {
      const win = windows.value[id];
      if (!win) continue;
      if (isEphemeralWindowState(win)) continue;
      states.push({
        id: win.id,
        cardId: win.cardId,
        cardName: win.cardName,
        channelId: win.channelId,
        worldId: win.worldId,
        readOnly: !!win.readOnly,
        sheetType: win.sheetType,
        attrs: win.attrs,
        positionX: win.positionX,
        positionY: win.positionY,
        width: win.width,
        height: win.height,
        zIndex: win.zIndex,
        isMinimized: win.isMinimized,
        mode: win.mode,
        bubbleX: win.bubbleX,
        bubbleY: win.bubbleY,
        avatarUrl: win.avatarUrl,
        templateMode: win.templateMode,
        templateId: win.templateId,
      });
    }
    saveWindowStates(states);
  };

  const schedulePersistWindows = () => {
    if (typeof window === 'undefined') return;
    if (windowsPersistTimer) clearTimeout(windowsPersistTimer);
    windowsPersistTimer = setTimeout(() => {
      persistWindows();
    }, WINDOWS_PERSIST_THROTTLE);
  };

  const restoreWindows = () => {
    if (typeof window === 'undefined' || hasRestored.value) return;
    hasRestored.value = true;
    const states = loadWindowStates();
    if (!states.length) return;
    const sanitizedStates = states.filter(state => !isEphemeralWindowState(state));
    if (sanitizedStates.length !== states.length) {
      saveWindowStates(sanitizedStates);
    }
    windows.value = {};
    activeWindowIds.value = [];
    let nextMaxZ = maxZIndex.value;
    for (const state of sanitizedStates) {
      if (!state?.cardId) continue;
      const resolvedSheetType = state.sheetType || resolveSheetTypeByCardId(state.cardId);
      const template = getTemplate(state.cardId, resolvedSheetType);
      const clampedPos = clampBubbleCoords(state.bubbleX || 0, state.bubbleY || 0);
      const width = Math.max(MIN_WIDTH, state.width || DEFAULT_WIDTH);
      const height = Math.max(MIN_HEIGHT, state.height || DEFAULT_HEIGHT);
      const clampedWindowPos = clampWindowCoords(
        state.positionX ?? VIEWPORT_PADDING,
        state.positionY ?? VIEWPORT_PADDING,
        width,
        height,
      );
      windows.value[state.id] = {
        id: state.id,
        cardId: state.cardId,
        cardName: state.cardName || '人物卡',
        channelId: state.channelId || '',
        worldId: state.worldId || undefined,
        readOnly: !!state.readOnly,
        sheetType: resolvedSheetType || undefined,
        attrs: state.attrs || {},
        template,
        positionX: clampedWindowPos.x,
        positionY: clampedWindowPos.y,
        width,
        height,
        zIndex: state.zIndex || maxZIndex.value + 1,
        isMinimized: !!state.isMinimized,
        mode: state.mode === 'edit' ? 'edit' : 'view',
        bubbleX: clampedPos.x,
        bubbleY: clampedPos.y,
        avatarUrl: state.avatarUrl,
        templateMode: state.templateMode,
        templateId: state.templateId,
        syncState: 'normal',
        hasLocalEditsInLock: false,
        hasSavedAfterEditEnd: false,
        pendingRemoteAttrs: undefined,
      };
      activeWindowIds.value.push(state.id);
      nextMaxZ = Math.max(nextMaxZ, windows.value[state.id].zIndex);
    }
    maxZIndex.value = nextMaxZ;
    for (const id of activeWindowIds.value) {
      void syncWindowTemplateFromCloud(id);
    }
  };

  const syncWindowTemplateFromCloud = async (windowId: string) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId || !win.cardId) return;
    if (win.readOnly) return;
    try {
      await templateStore.ensureTemplatesLoaded({ worldId: win.worldId || undefined });
      await templateStore.ensureBindingsLoaded(win.channelId);
      const fallback = normalizeTemplate(
        win.cardId,
        win.template || getTemplate(win.cardId, win.sheetType),
        win.sheetType,
      );
      const binding = await templateStore.ensureCardBinding({
        channelId: win.channelId,
        externalCardId: win.cardId,
        cardName: win.cardName,
        sheetType: win.sheetType || '',
        fallbackTemplate: fallback,
      });
      const resolved = templateStore.resolveCardTemplate(win.channelId, win.cardId, win.sheetType, fallback);
      const normalized = normalizeTemplate(win.cardId, resolved, win.sheetType);
      if (normalized !== resolved && binding) {
        try {
          if (binding.mode === 'managed' && binding.templateId) {
            const template = templateStore.getTemplateById(binding.templateId);
            if (template && !template.readonly) {
              await templateStore.updateTemplate(binding.templateId, { content: normalized });
            }
          } else if (binding.mode === 'detached') {
            await templateStore.bindCardToDetachedTemplate({
              channelId: win.channelId,
              externalCardId: win.cardId,
              cardName: win.cardName,
              sheetType: win.sheetType || '',
              templateSnapshot: normalized,
            });
          }
        } catch (e) {
          console.warn('Failed to persist legacy character template upgrade', e);
        }
      }
      win.template = normalized;
      win.templateMode = binding?.mode;
      win.templateId = binding?.templateId || undefined;
      saveTemplate(win.cardId, normalized);
      schedulePersistWindows();
    } catch (e) {
      console.warn('Failed to sync character sheet template from cloud', e);
    }
  };

  const syncOpenWindowsUsingTemplate = async (templateId: string, excludeWindowId?: string) => {
    if (!templateId) return;
    const windowIds = activeWindowIds.value.filter((windowId) => {
      if (windowId === excludeWindowId) return false;
      const win = windows.value[windowId];
      if (!win || win.readOnly || win.templateMode !== 'managed') return false;
      const binding = templateStore.getBinding(win.channelId, win.cardId);
      return win.templateId === templateId || binding?.templateId === templateId;
    });
    await Promise.all(windowIds.map(windowId => syncWindowTemplateFromCloud(windowId)));
  };

  const applyManagedTemplate = async (windowId: string, templateId: string, options?: ApplyManagedTemplateOptions) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId || !win.cardId || !templateId) return null;
    if (win.readOnly) return null;
    await templateStore.ensureTemplatesLoaded({ worldId: win.worldId || undefined });
    const template = templateStore.getTemplateById(templateId);
    if (!template) {
      throw new Error('模板不存在');
    }
    const binding = await templateStore.bindCardToTemplate({
      channelId: win.channelId,
      externalCardId: win.cardId,
      cardName: win.cardName,
      sheetType: win.sheetType || template.sheetType,
      templateId,
    });
    const normalized = normalizeTemplate(win.cardId, template.content, win.sheetType || template.sheetType);
    if (normalized !== template.content && !template.readonly) {
      try {
        await templateStore.updateTemplate(templateId, { content: normalized });
      } catch (e) {
        console.warn('Failed to persist managed character template upgrade', e);
      }
    }
    win.template = normalized;
    win.templateMode = 'managed';
    win.templateId = templateId;
    syncWorldLocalBadgeTemplate(
      win.worldId || '',
      template.defaultBadgeTemplate,
      !!options?.syncWorldLocalBadgeTemplate,
    );
    saveTemplate(win.cardId, normalized);
    schedulePersistWindows();
    return binding;
  };

  const applyDetachedTemplate = async (windowId: string, templateText?: string) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId || !win.cardId) return null;
    if (win.readOnly) return null;
    const normalized = normalizeTemplate(win.cardId, templateText ?? win.template, win.sheetType);
    const binding = await templateStore.bindCardToDetachedTemplate({
      channelId: win.channelId,
      externalCardId: win.cardId,
      cardName: win.cardName,
      sheetType: win.sheetType || '',
      templateSnapshot: normalized,
    });
    win.template = normalized;
    win.templateMode = 'detached';
    win.templateId = undefined;
    saveTemplate(win.cardId, normalized);
    schedulePersistWindows();
    return binding;
  };

  const saveCurrentTemplateAsNew = async (windowId: string, name: string) => {
    const win = windows.value[windowId];
    if (!win) return null;
    if (win.readOnly) return null;
    const trimmedName = name.trim();
    if (!trimmedName) {
      throw new Error('模板名称不能为空');
    }
    await templateStore.ensureTemplatesLoaded();
    const created = await templateStore.createTemplate({
      name: trimmedName,
      sheetType: win.sheetType || '',
      content: win.template,
    });
    if (!created?.id) {
      throw new Error('创建模板失败');
    }
    await applyManagedTemplate(windowId, created.id);
    return created;
  };

  const syncCurrentTemplateToTemplate = async (windowId: string, templateId: string) => {
    const win = windows.value[windowId];
    if (!win || !templateId) return null;
    if (win.readOnly) return null;
    await templateStore.updateTemplate(templateId, {
      content: win.template,
    });
    await applyManagedTemplate(windowId, templateId);
    await syncOpenWindowsUsingTemplate(templateId, windowId);
    return templateStore.getTemplateById(templateId);
  };

  let attrsSyncTimer: Record<string, ReturnType<typeof setTimeout> | null> = {};

  const beginEditLock = (windowId: string) => {
    const win = windows.value[windowId];
    if (!win) return;
    normalizeSyncState(win);
    if (win.syncState === 'normal') {
      win.hasLocalEditsInLock = false;
      win.hasSavedAfterEditEnd = false;
      win.pendingRemoteAttrs = undefined;
    }
    win.syncState = 'editing_locked';
    schedulePersistWindows();
  };

  const endEditLock = (windowId: string) => {
    const win = windows.value[windowId];
    if (!win) return;
    normalizeSyncState(win);
    if (!win.hasLocalEditsInLock) {
      win.syncState = 'normal';
      win.hasSavedAfterEditEnd = false;
      win.pendingRemoteAttrs = undefined;
      schedulePersistWindows();
      return;
    }
    if (win.hasSavedAfterEditEnd) {
      win.hasLocalEditsInLock = false;
      win.pendingRemoteAttrs = undefined;
      win.syncState = 'normal';
      schedulePersistWindows();
      return;
    }
    win.syncState = 'resume_pending';
    schedulePersistWindows();
  };

  const scheduleAttrsSync = (windowId: string) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId || !win.cardName) return;
    normalizeSyncState(win);
    if (attrsSyncTimer[windowId]) {
      clearTimeout(attrsSyncTimer[windowId] as ReturnType<typeof setTimeout>);
    }
    attrsSyncTimer[windowId] = setTimeout(async () => {
      try {
        const latest = windows.value[windowId];
        if (!latest) return;
        const ok = await cardStore.updateCard(latest.channelId, latest.cardName, latest.attrs);
        if (ok) {
          latest.hasSavedAfterEditEnd = true;
          if (latest.syncState === 'resume_pending') {
            latest.hasLocalEditsInLock = false;
            latest.pendingRemoteAttrs = undefined;
            latest.syncState = 'normal';
          }
          schedulePersistWindows();
        }
      } catch (e) {
        console.warn('Failed to sync character sheet attrs', e);
      }
    }, ATTRS_SYNC_THROTTLE);
  };

  const refreshWindowAttrs = async (windowId: string) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId) return;
    if (win.readOnly) return;
    normalizeSyncState(win);
    try {
      const active = await cardStore.getActiveCard(win.channelId);
      if (!active || !active.attrs) return;
      if (active.name && active.name !== win.cardName) return;
      if (win.syncState !== 'normal') {
        win.pendingRemoteAttrs = active.attrs;
        return;
      }
      if (isAttrsEqual(win.attrs, active.attrs)) return;
      win.attrs = active.attrs;
      schedulePersistWindows();
    } catch (e) {
      console.warn('Failed to refresh character sheet attrs', e);
    }
  };

  const refreshAllWindows = async () => {
    const ids = [...activeWindowIds.value];
    for (const id of ids) {
      await refreshWindowAttrs(id);
    }
  };

  const openSheet = (
    card: CharacterCard,
    channelId: string,
    cardData?: CharacterCardData,
    templateMeta?: {
      templateMode?: CharacterCardTemplateMode;
      templateId?: string;
      templateText?: string;
      readOnly?: boolean;
      ephemeral?: boolean;
      reuse?: boolean;
      worldId?: string;
      placement?: 'right';
    }
  ): string => {
    restoreWindows();
    const managedTemplate = templateMeta?.templateMode === 'managed' && templateMeta.templateId
      ? templateStore.getTemplateById(templateMeta.templateId)
      : null;
    const existingId = templateMeta?.reuse === false
      ? undefined
      : activeWindowIds.value.find(id => windows.value[id]?.cardId === card.id);
    if (existingId) {
      const existing = windows.value[existingId];
      const resolvedSheetType = (cardData?.type || card.sheetType || '').trim();
      if (existing) {
        normalizeSyncState(existing);
        existing.cardName = card.name || existing.cardName;
        if (templateMeta?.worldId !== undefined) {
          existing.worldId = templateMeta.worldId || undefined;
        }
        if (resolvedSheetType && !existing.sheetType) {
          existing.sheetType = resolvedSheetType;
        }
        if (cardData?.avatarUrl !== undefined) {
          existing.avatarUrl = cardData.avatarUrl;
        }
        if (templateMeta?.readOnly) {
          existing.attrs = cardData?.attrs || card.attrs || existing.attrs;
          if (resolvedSheetType) {
            existing.sheetType = resolvedSheetType;
          }
        }
        const normalized = normalizeTemplate(existing.cardId, existing.template, existing.sheetType);
        if (normalized !== existing.template) {
          existing.template = normalized;
        }
        if (templateMeta?.templateMode) {
          existing.templateMode = templateMeta.templateMode;
        }
        if (templateMeta?.templateId !== undefined) {
          existing.templateId = templateMeta.templateId || undefined;
        }
        if (templateMeta?.readOnly !== undefined) {
          existing.readOnly = templateMeta.readOnly;
          existing.mode = 'view';
        }
        if (templateMeta?.templateText) {
          existing.template = normalizeTemplate(existing.cardId, templateMeta.templateText, existing.sheetType);
        } else if (managedTemplate?.content) {
          existing.template = normalizeTemplate(existing.cardId, managedTemplate.content, existing.sheetType);
        }

        existing.mode = 'view';

        if (existing.isMinimized) {
          existing.isMinimized = false;
        }

        if (templateMeta?.placement === 'right') {
          existing.positionX = (typeof window !== 'undefined' ? window.innerWidth : 1200)
            - Math.max(MIN_WIDTH, existing.width || DEFAULT_WIDTH)
            - VIEWPORT_PADDING;
          existing.positionY = VIEWPORT_PADDING;
        }

        const clampedPos = clampWindowCoords(
          existing.positionX,
          existing.positionY,
          Math.max(MIN_WIDTH, existing.width || DEFAULT_WIDTH),
          Math.max(MIN_HEIGHT, existing.height || DEFAULT_HEIGHT),
        );
        if (clampedPos.x !== existing.positionX || clampedPos.y !== existing.positionY) {
          existing.positionX = clampedPos.x;
          existing.positionY = clampedPos.y;
        }
      }
      if (!existing?.readOnly) {
        void syncWindowTemplateFromCloud(existingId);
        void refreshWindowAttrs(existingId);
      }
      bringToFront(existingId);
      schedulePersistWindows();
      return existingId;
    }

    const windowId = generateWindowId();
    const offset = activeWindowIds.value.length * 30;

    const preferredInitialX = templateMeta?.placement === 'right'
      ? (typeof window !== 'undefined' ? window.innerWidth : 1200) - DEFAULT_WIDTH - VIEWPORT_PADDING
      : VIEWPORT_PADDING + offset;
    const clampedInitialPos = clampWindowCoords(
      preferredInitialX,
      VIEWPORT_PADDING + offset,
      DEFAULT_WIDTH,
      DEFAULT_HEIGHT,
    );
    const posX = clampedInitialPos.x;
    const posY = clampedInitialPos.y;

    maxZIndex.value += 1;

    const savedBubblePos = loadBubblePositions()[card.id];
    const bubblePos = savedBubblePos
      ? clampBubbleCoords(savedBubblePos.x, savedBubblePos.y)
      : getDefaultBubblePosition(activeWindowIds.value.length);

    const resolvedSheetType = (cardData?.type || card.sheetType || '').trim();
    const initialTemplate = resolveInitialSheetTemplate(
      card.id,
      resolvedSheetType,
      templateMeta,
      managedTemplate?.content,
      getTemplate,
    );
    windows.value[windowId] = {
      id: windowId,
      cardId: card.id,
      cardName: card.name,
      channelId,
      worldId: templateMeta?.worldId,
      readOnly: !!templateMeta?.readOnly,
      ephemeral: !!templateMeta?.ephemeral,
      sheetType: resolvedSheetType || undefined,
      attrs: cardData?.attrs || card.attrs || {},
      template: initialTemplate,
      positionX: posX,
      positionY: posY,
      width: DEFAULT_WIDTH,
      height: DEFAULT_HEIGHT,
      zIndex: maxZIndex.value,
      isMinimized: false,
      mode: 'view',
      bubbleX: bubblePos.x,
      bubbleY: bubblePos.y,
      avatarUrl: cardData?.avatarUrl,
      templateMode: templateMeta?.templateMode,
      templateId: templateMeta?.templateId,
      syncState: 'normal',
      hasLocalEditsInLock: false,
      hasSavedAfterEditEnd: false,
      pendingRemoteAttrs: undefined,
    };
    activeWindowIds.value.push(windowId);
    schedulePersistWindows();
    if (!windows.value[windowId].readOnly) {
      void syncWindowTemplateFromCloud(windowId);
    }

    return windowId;
  };

  const closeSheet = (windowId: string) => {
    const idx = activeWindowIds.value.indexOf(windowId);
    if (idx !== -1) {
      activeWindowIds.value.splice(idx, 1);
    }
    if (attrsSyncTimer[windowId]) {
      clearTimeout(attrsSyncTimer[windowId] as ReturnType<typeof setTimeout>);
      delete attrsSyncTimer[windowId];
    }
    delete windows.value[windowId];
    schedulePersistWindows();
  };

  const bringToFront = (windowId: string) => {
    const win = windows.value[windowId];
    if (!win) return;
    maxZIndex.value += 1;
    win.zIndex = maxZIndex.value;
    schedulePersistWindows();
  };

  const minimizeSheet = (windowId: string) => {
    const win = windows.value[windowId];
    if (win) {
      win.isMinimized = true;
      schedulePersistWindows();
    }
  };

  const restoreSheet = (windowId: string) => {
    const win = windows.value[windowId];
    if (win) {
      win.isMinimized = false;
      bringToFront(windowId);
      schedulePersistWindows();
    }
  };

  const updatePosition = (windowId: string, x: number, y: number) => {
    const win = windows.value[windowId];
    if (win) {
      win.positionX = x;
      win.positionY = y;
      schedulePersistWindows();
    }
  };

  const updateSize = (windowId: string, w: number, h: number) => {
    const win = windows.value[windowId];
    if (win) {
      win.width = Math.max(MIN_WIDTH, w);
      win.height = Math.max(MIN_HEIGHT, h);
      schedulePersistWindows();
    }
  };

  const updateAttrs = (windowId: string, attrs: Record<string, any>) => {
    const win = windows.value[windowId];
    if (win) {
      if (win.readOnly) return;
      normalizeSyncState(win);
      win.attrs = attrs;
      if (win.syncState !== 'normal') {
        win.hasLocalEditsInLock = true;
        win.hasSavedAfterEditEnd = false;
      }
      schedulePersistWindows();
      scheduleAttrsSync(windowId);
    }
  };

  const updateTemplate = (windowId: string, template: string) => {
    const win = windows.value[windowId];
    if (win) {
      if (win.readOnly) return;
      const normalized = normalizeTemplate(win.cardId, template, win.sheetType);
      win.template = normalized;
      win.templateMode = 'detached';
      win.templateId = undefined;
      saveTemplate(win.cardId, normalized);
      void applyDetachedTemplate(windowId, normalized);
      schedulePersistWindows();
    }
  };

  const updateReadOnlyWindowData = (
    windowId: string,
    payload: {
      cardName?: string;
      sheetType?: string;
      attrs?: Record<string, any>;
      avatarUrl?: string;
      templateText?: string;
    },
  ) => {
    const win = windows.value[windowId];
    if (!win || !win.readOnly) return;
    if (payload.cardName) {
      win.cardName = payload.cardName;
    }
    if (payload.sheetType) {
      win.sheetType = payload.sheetType;
    }
    if (payload.attrs && typeof payload.attrs === 'object') {
      win.attrs = payload.attrs;
    }
    if (payload.avatarUrl !== undefined) {
      win.avatarUrl = payload.avatarUrl;
    }
    if (payload.templateText) {
      win.template = normalizeTemplate(win.cardId, payload.templateText, payload.sheetType || win.sheetType);
    }
    schedulePersistWindows();
  };

  const setMode = (windowId: string, mode: 'view' | 'edit') => {
    const win = windows.value[windowId];
    if (win) {
      if (win.readOnly && mode === 'edit') return;
      win.mode = mode;
      schedulePersistWindows();
    }
  };

  const updateCardAvatar = (cardId: string, avatarUrl?: string) => {
    if (!cardId) return;
    let changed = false;
    activeWindowIds.value.forEach((windowId) => {
      const win = windows.value[windowId];
      if (!win || win.cardId !== cardId) return;
      if ((win.avatarUrl || '') === (avatarUrl || '')) return;
      win.avatarUrl = avatarUrl;
      changed = true;
    });
    if (changed) {
      schedulePersistWindows();
    }
  };

  const toggleMode = (windowId: string) => {
    const win = windows.value[windowId];
    if (win) {
      if (win.readOnly) return;
      win.mode = win.mode === 'view' ? 'edit' : 'view';
      schedulePersistWindows();
    }
  };

  const reset = () => {
    windows.value = {};
    activeWindowIds.value = [];
    for (const timer of Object.values(attrsSyncTimer)) {
      if (timer) clearTimeout(timer as ReturnType<typeof setTimeout>);
    }
    attrsSyncTimer = {};
    clearWindowStates();
  };

  let bubblePersistTimer: ReturnType<typeof setTimeout> | null = null;

  const updateBubblePosition = (windowId: string, x: number, y: number) => {
    const win = windows.value[windowId];
    if (!win) return;
    const clamped = clampBubbleCoords(x, y);
    win.bubbleX = clamped.x;
    win.bubbleY = clamped.y;
    if (bubblePersistTimer) clearTimeout(bubblePersistTimer);
    bubblePersistTimer = setTimeout(() => {
      persistBubblePositions();
    }, BUBBLE_PERSIST_THROTTLE);
    schedulePersistWindows();
  };

  const persistBubblePositions = () => {
    const positions: Record<string, { x: number; y: number }> = {};
    for (const id of activeWindowIds.value) {
      const win = windows.value[id];
      if (win) {
        positions[win.cardId] = { x: win.bubbleX, y: win.bubbleY };
      }
    }
    saveBubblePositions(positions);
  };

  const clampAllBubbles = () => {
    for (const id of activeWindowIds.value) {
      const win = windows.value[id];
      if (win) {
        const clamped = clampBubbleCoords(win.bubbleX, win.bubbleY);
        win.bubbleX = clamped.x;
        win.bubbleY = clamped.y;
      }
    }
    schedulePersistWindows();
  };

  restoreWindows();

  return {
    windows,
    activeWindowIds,
    activeWindows,
    maxZIndex,
    openSheet,
    closeSheet,
    bringToFront,
    minimizeSheet,
    restoreSheet,
    updatePosition,
    updateSize,
    updateAttrs,
    beginEditLock,
    endEditLock,
    updateTemplate,
    updateReadOnlyWindowData,
    applyManagedTemplate,
    applyDetachedTemplate,
    saveCurrentTemplateAsNew,
    syncCurrentTemplateToTemplate,
    syncWindowTemplateFromCloud,
    normalizeTemplate,
    setMode,
    updateCardAvatar,
    toggleMode,
    getTemplate,
    getDefaultTemplate,
    reset,
    updateBubblePosition,
    clampAllBubbles,
    restoreWindows,
    refreshWindowAttrs,
    refreshAllWindows,
  };
});
