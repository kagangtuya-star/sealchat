import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { useCharacterCardStore } from './characterCard';
import type { CharacterCard, CharacterCardData } from './characterCard';

export interface CharacterSheetWindow {
  id: string;
  cardId: string;
  cardName: string;
  channelId: string;
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
}

const TEMPLATE_STORAGE_KEY = 'sealchat_character_sheet_templates';
const WINDOWS_STORAGE_KEY = 'sealchat_character_sheet_windows';
const BUBBLE_POSITIONS_KEY = 'sealchat_sheet_bubble_positions';
const BUBBLE_SIZE = 56;
const MIN_WIDTH = 320;
const MIN_HEIGHT = 240;
const DEFAULT_WIDTH = 400;
const DEFAULT_HEIGHT = 480;
const VIEWPORT_PADDING = 16;
const BUBBLE_PERSIST_THROTTLE = 300;
const WINDOWS_PERSIST_THROTTLE = 300;
const ATTRS_SYNC_THROTTLE = 600;

let windowIdCounter = 0;

const generateWindowId = () => `sheet-${Date.now()}-${++windowIdCounter}`;

interface PersistedWindowState {
  id: string;
  cardId: string;
  cardName: string;
  channelId: string;
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

const DEFAULT_TEMPLATE_MARK = 'sealchat-default-template:v2';

const isLegacyDefaultTemplate = (template: string) => {
  if (!template) return false;
  if (template.includes(DEFAULT_TEMPLATE_MARK)) return false;
  if (template.includes('window.prompt')) return true;
  const hasShell =
    template.includes('id="content"') &&
    template.includes('sealchat.onUpdate(render)') &&
    template.includes('attrs-table') &&
    template.includes('card-header');
  const hasLegacyRoll =
    template.includes('data-roll=".ra {skill}"') ||
    template.includes('data-roll=\\".ra {skill}\\"');
  const hasPrompt = template.includes('window.prompt');
  return hasShell && (hasLegacyRoll || hasPrompt);
};

const normalizeTemplate = (_cardId: string | undefined, template: string) => {
  if (!template) return template;
  if (!isLegacyDefaultTemplate(template)) return template;
  return getDefaultTemplate();
};

const getDefaultTemplate = () => `<!DOCTYPE html>
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
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
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
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', { roll: { template: template, label: label || '', args: args || {} } });
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
            roll: {
              template: target.dataset.roll,
              label: target.dataset.label || target.innerText || '',
              args: args,
              rect: { top: rect.top, left: rect.left, width: rect.width, height: rect.height }
            }
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

export const useCharacterSheetStore = defineStore('characterSheet', () => {
  const windows = ref<Record<string, CharacterSheetWindow>>({});
  const activeWindowIds = ref<string[]>([]);
  const maxZIndex = ref(2000);
  const hasRestored = ref(false);
  const cardStore = useCharacterCardStore();

  const activeWindows = computed(() =>
    activeWindowIds.value.map(id => windows.value[id]).filter(Boolean)
  );

  const loadTemplates = (): Record<string, string> => {
    try {
      const raw = localStorage.getItem(TEMPLATE_STORAGE_KEY);
      const parsed = raw ? JSON.parse(raw) : {};
      let changed = false;
      for (const [cardId, template] of Object.entries(parsed)) {
        const normalized = normalizeTemplate(cardId, String(template || ''));
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

  const getTemplate = (cardId: string): string => {
    const templates = loadTemplates();
    const stored = templates[cardId];
    if (stored) {
      const normalized = normalizeTemplate(cardId, stored);
      if (normalized !== stored) {
        saveTemplate(cardId, normalized);
      }
      return normalized;
    }
    const fallback = getDefaultTemplate();
    const normalized = normalizeTemplate(cardId, fallback);
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
      states.push({
        id: win.id,
        cardId: win.cardId,
        cardName: win.cardName,
        channelId: win.channelId,
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
    windows.value = {};
    activeWindowIds.value = [];
    let nextMaxZ = maxZIndex.value;
    for (const state of states) {
      if (!state?.cardId) continue;
      const template = getTemplate(state.cardId);
      const clampedPos = clampBubbleCoords(state.bubbleX || 0, state.bubbleY || 0);
      const width = Math.max(MIN_WIDTH, state.width || DEFAULT_WIDTH);
      const height = Math.max(MIN_HEIGHT, state.height || DEFAULT_HEIGHT);
      windows.value[state.id] = {
        id: state.id,
        cardId: state.cardId,
        cardName: state.cardName || '人物卡',
        channelId: state.channelId || '',
        attrs: state.attrs || {},
        template,
        positionX: state.positionX ?? VIEWPORT_PADDING,
        positionY: state.positionY ?? VIEWPORT_PADDING,
        width,
        height,
        zIndex: state.zIndex || maxZIndex.value + 1,
        isMinimized: !!state.isMinimized,
        mode: state.mode === 'edit' ? 'edit' : 'view',
        bubbleX: clampedPos.x,
        bubbleY: clampedPos.y,
        avatarUrl: state.avatarUrl,
      };
      activeWindowIds.value.push(state.id);
      nextMaxZ = Math.max(nextMaxZ, windows.value[state.id].zIndex);
    }
    maxZIndex.value = nextMaxZ;
  };

  let attrsSyncTimer: Record<string, ReturnType<typeof setTimeout> | null> = {};

  const scheduleAttrsSync = (windowId: string) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId || !win.cardName) return;
    if (attrsSyncTimer[windowId]) {
      clearTimeout(attrsSyncTimer[windowId] as ReturnType<typeof setTimeout>);
    }
    attrsSyncTimer[windowId] = setTimeout(async () => {
      try {
        await cardStore.updateCard(win.channelId, win.cardName, win.attrs);
      } catch (e) {
        console.warn('Failed to sync character sheet attrs', e);
      }
    }, ATTRS_SYNC_THROTTLE);
  };

  const refreshWindowAttrs = async (windowId: string) => {
    const win = windows.value[windowId];
    if (!win || !win.channelId) return;
    try {
      const active = await cardStore.getActiveCard(win.channelId);
      if (!active || !active.attrs) return;
      if (active.name && active.name !== win.cardName) return;
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
    cardData?: CharacterCardData
  ): string => {
    restoreWindows();
    const existingId = activeWindowIds.value.find(
      id => windows.value[id]?.cardId === card.id
    );
    if (existingId) {
      const existing = windows.value[existingId];
      if (existing) {
        const normalized = normalizeTemplate(existing.cardId, existing.template);
        if (normalized !== existing.template) {
          existing.template = normalized;
        }
      }
      void refreshWindowAttrs(existingId);
      bringToFront(existingId);
      return existingId;
    }

    const windowId = generateWindowId();
    const offset = activeWindowIds.value.length * 30;
    const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1200;
    const viewportH = typeof window !== 'undefined' ? window.innerHeight : 800;

    const posX = Math.min(
      VIEWPORT_PADDING + offset,
      viewportW - DEFAULT_WIDTH - VIEWPORT_PADDING
    );
    const posY = Math.min(
      VIEWPORT_PADDING + offset,
      viewportH - DEFAULT_HEIGHT - VIEWPORT_PADDING
    );

    maxZIndex.value += 1;

    const savedBubblePos = loadBubblePositions()[card.id];
    const bubblePos = savedBubblePos
      ? clampBubbleCoords(savedBubblePos.x, savedBubblePos.y)
      : getDefaultBubblePosition(activeWindowIds.value.length);

    windows.value[windowId] = {
      id: windowId,
      cardId: card.id,
      cardName: card.name,
      channelId,
      attrs: cardData?.attrs || card.attrs || {},
      template: getTemplate(card.id),
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
    };
    activeWindowIds.value.push(windowId);
    schedulePersistWindows();

    return windowId;
  };

  const closeSheet = (windowId: string) => {
    const idx = activeWindowIds.value.indexOf(windowId);
    if (idx !== -1) {
      activeWindowIds.value.splice(idx, 1);
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
      win.attrs = attrs;
      schedulePersistWindows();
      scheduleAttrsSync(windowId);
    }
  };

  const updateTemplate = (windowId: string, template: string) => {
    const win = windows.value[windowId];
    if (win) {
      const normalized = normalizeTemplate(win.cardId, template);
      win.template = normalized;
      saveTemplate(win.cardId, normalized);
      schedulePersistWindows();
    }
  };

  const setMode = (windowId: string, mode: 'view' | 'edit') => {
    const win = windows.value[windowId];
    if (win) {
      win.mode = mode;
      schedulePersistWindows();
    }
  };

  const toggleMode = (windowId: string) => {
    const win = windows.value[windowId];
    if (win) {
      win.mode = win.mode === 'view' ? 'edit' : 'view';
      schedulePersistWindows();
    }
  };

  const reset = () => {
    windows.value = {};
    activeWindowIds.value = [];
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
    updateTemplate,
    normalizeTemplate,
    setMode,
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
