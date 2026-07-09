import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
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

const isEphemeralWindowState = (state?: { cardId?: string; readOnly?: boolean }) => (
  !!state?.readOnly || isOnlinePreviewCardId(state?.cardId)
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
const DEFAULT_TEMPLATE_MARK_COC = 'sealchat-default-template:v3-coc7th';

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

const isLegacyDefaultTemplate = (template: string) => {
  if (!template) return false;
  if (template.includes(DEFAULT_TEMPLATE_MARK) || template.includes(DEFAULT_TEMPLATE_MARK_COC)) return false;
  // 旧版 COC 默认模板（sealchat-default-template:v2-coc-dark）→ 自动升级为新版
  if (template.includes('sealchat-default-template:v2-coc-dark')) return true;
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

const normalizeTemplate = (_cardId: string | undefined, template: string, sheetType?: string) => {
  if (!template) return template;
  if (!isLegacyDefaultTemplate(template)) return template;
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

const getCocDefaultTemplate = () => `<!DOCTYPE html>
<!-- ${DEFAULT_TEMPLATE_MARK_COC} -->
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=yes">
  <!-- 加载字体与图标 -->
  <link rel="preconnect" href="https://fonts.googleapis.com" crossorigin>
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="preconnect" href="https://cdnjs.cloudflare.com" crossorigin>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Noto+Serif:wght@400;700&family=Noto+Serif+SC:wght@400;700&family=ZCOOL+KuaiLe&display=swap" onerror="this.href='https://fonts.yite.net/css2?family=Noto+Serif:wght@400;700&family=Noto+Serif+SC:wght@400;700&family=ZCOOL+KuaiLe&display=swap'">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/7.0.0/css/all.min.css" onerror="this.href='https://cdnjs.yite.net/ajax/libs/font-awesome/7.0.0/css/all.min.css'">
  <!-- 平台字库声明 -->
  <script type="application/sealchat-fonts+json">
    {
      "version": 1,
      "global": false,
      "fonts": [
        {
          "key": "platform",
          "platformFontId": "在此处填写平台字体ID可以覆盖默认显示字体", 
          "cssVar": "--font-platform"
        }
      ]
    }
  </script>
  <style>
    :root {
      --c-bg: #0a0c12;
      --c-bg-40: rgba(10, 12, 18, 0.4);
      --c-bg-80: rgba(10, 12, 18, 0.8);
      --c-card-bg: #151b25;
      --c-text-main: #d1d7e0;
      --c-text-dim: #7a8499;
      --c-text-edit: #f0f8ff;
      --c-text-empty: rgba(177, 177, 177, 0.6);
      --c-accent: #a8c7fa;
      --c-accent-20: rgba(168, 199, 250, 0.2);
      --c-health: #e88d9d;
      --c-magic: #b19cd9;
      --c-sanity: #e6d3a7;
      --c-armor: #008b8b;
      --c-border: #2a3142;
      --c-hover: #1a2130;
      --c-favorite: gold;
      
      --font-serif: "Noto Serif SC", "Noto Serif", "Songti SC", "SimSun", "Georgia", serif;
      --font-gothic: var(--font-platform, ""), "ZCOOL KuaiLe", var(--font-serif);
    }

    button { all: unset; box-sizing: border-box; cursor: pointer; touch-action: manipulation; }
    button:focus-visible {
      outline: none;
      border-radius: 3px;
      box-shadow: 0 0 0 2px var(--c-accent-20);
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }

    html {
      -webkit-overflow-scrolling: touch;
      overflow-y: auto;
      touch-action: pan-y;
    }

    body {
      min-height: 100vh;
      font-family: var(--font-serif);
      background: var(--c-bg);
      color: var(--c-text-main);
      padding: 12px;
      font-size: 18px;
      line-height: 1.4;
      letter-spacing: 1px;
      background-image: 
        radial-gradient(5px at 12% 30%, rgba(255, 255, 255, 0.95) 0px, rgba(255, 255, 255, 0.7) 1px, rgba(255, 255, 255, 0) 5px),
        radial-gradient(4px at 72% 45%, rgba(255, 255, 220, 0.9) 0px, rgba(255, 255, 200, 0.6) 1px, rgba(255, 255, 200, 0) 4px),
        radial-gradient(6px at 5% 88%, rgba(230, 240, 255, 0.95) 0px, rgba(200, 220, 255, 0.7) 2px, rgba(200, 220, 255, 0) 6px),
        radial-gradient(4px at 33% 60%, rgba(255, 255, 255, 0.9) 0px, rgba(255, 255, 255, 0.6) 1px, rgba(255, 255, 255, 0) 4px),
        radial-gradient(5px at 80% 12%, rgba(210, 230, 255, 0.95) 0px, rgba(180, 210, 250, 0.7) 1.5px, rgba(180, 210, 250, 0) 5px),
        radial-gradient(6px at 95% 70%, rgba(255, 240, 210, 0.95) 0px, rgba(255, 220, 180, 0.6) 2px, rgba(255, 220, 180, 0) 6px),
        radial-gradient(4px at 42% 92%, rgba(255, 255, 255, 0.9) 0px, rgba(255, 255, 255, 0.6) 1px, rgba(255, 255, 255, 0) 4px),
        radial-gradient(5px at 60% 25%, rgba(220, 230, 255, 0.95) 0px, rgba(200, 215, 250, 0.7) 1.5px, rgba(200, 215, 250, 0) 5px),
        radial-gradient(4px at 10% 50%, rgba(255, 250, 220, 0.9) 0px, rgba(255, 240, 200, 0.6) 1px, rgba(255, 240, 200, 0) 4px),
        radial-gradient(6px at 25% 75%, rgba(255, 255, 255, 0.95) 0px, rgba(240, 240, 255, 0.7) 2px, rgba(240, 240, 255, 0) 6px),
        radial-gradient(4px at 48% 18%, rgba(230, 245, 255, 0.9) 0px, rgba(210, 230, 250, 0.6) 1px, rgba(210, 230, 250, 0) 4px),
        radial-gradient(5px at 88% 35%, rgba(255, 255, 240, 0.95) 0px, rgba(255, 245, 220, 0.7) 1.5px, rgba(255, 245, 220, 0) 5px),

        radial-gradient(circle at 10% 20%, rgba(100, 150, 250, 0.1) 0%, transparent 30%),
        radial-gradient(circle at 90% 80%, rgba(80, 120, 200, 0.1) 0%, transparent 35%),
        linear-gradient(to bottom, transparent 80%, rgba(20, 30, 50, 0.3) 100%);
      overflow-x: hidden;
      touch-action: pan-y;
      padding-bottom: 20px;
    }

    body::before {
      content: "";
      position: fixed;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background-image: 
        radial-gradient(5px at 3% 12%, rgba(255, 255, 255, 0.5) 0px, rgba(255, 255, 255, 0.3) 1px, rgba(255, 255, 255, 0) 5px),
        radial-gradient(4px at 48% 77%, rgba(220, 230, 255, 0.5) 0px, rgba(200, 215, 240, 0.3) 1px, rgba(200, 215, 240, 0) 4px),
        radial-gradient(6px at 22% 34%, rgba(255, 240, 200, 0.5) 0px, rgba(240, 220, 180, 0.3) 2px, rgba(240, 220, 180, 0) 6px),
        radial-gradient(4px at 91% 55%, rgba(210, 230, 255, 0.5) 0px, rgba(190, 210, 240, 0.3) 1px, rgba(190, 210, 240, 0) 4px),
        radial-gradient(5px at 66% 8%, rgba(255, 255, 220, 0.5) 0px, rgba(235, 235, 200, 0.3) 1.5px, rgba(235, 235, 200, 0) 5px),
        radial-gradient(4px at 35% 43%, rgba(255, 255, 255, 0.5) 0px, rgba(255, 255, 255, 0.3) 1px, rgba(255, 255, 255, 0) 4px),
        radial-gradient(6px at 74% 61%, rgba(200, 220, 255, 0.5) 0px, rgba(180, 200, 240, 0.3) 2px, rgba(180, 200, 240, 0) 6px),
        radial-gradient(4px at 8% 94%, rgba(230, 240, 255, 0.5) 0px, rgba(210, 220, 250, 0.3) 1px, rgba(210, 220, 250, 0) 4px),
        radial-gradient(5px at 53% 19%, rgba(255, 250, 210, 0.5) 0px, rgba(240, 230, 190, 0.3) 1.5px, rgba(240, 230, 190, 0) 5px),
        radial-gradient(4px at 87% 38%, rgba(255, 255, 255, 0.5) 0px, rgba(255, 255, 255, 0.3) 1px, rgba(255, 255, 255, 0) 4px),
        radial-gradient(5px at 17% 68%, rgba(180, 210, 250, 0.5) 0px, rgba(160, 190, 230, 0.3) 1.5px, rgba(160, 190, 230, 0) 5px),
        radial-gradient(4px at 43% 27%, rgba(255, 245, 215, 0.5) 0px, rgba(235, 225, 195, 0.3) 1px, rgba(235, 225, 195, 0) 4px),

        radial-gradient(circle at 30% 60%, rgba(70, 130, 200, 0.05) 0%, transparent 40%),
        radial-gradient(circle at 70% 15%, rgba(100, 160, 220, 0.05) 0%, transparent 35%);
      pointer-events: none;
      z-index: -1;
    }

    /* 滚动条 */
    ::-webkit-scrollbar {
      width: 6px;
      height: 6px;
    }

    ::-webkit-scrollbar-track {
      background: transparent;
    }

    ::-webkit-scrollbar-thumb {
      background: rgba(168, 199, 250, 0.4);
      border-radius: 4px;
      transition: background 0.15s ease;
    }

    ::-webkit-scrollbar-thumb:hover {
      background: rgba(168, 199, 250, 0.6);
    }

    ::-webkit-scrollbar-thumb:active {
      background: rgba(168, 199, 250, 0.8);
    }

     /* 加载动画 */
    #loadingIndicator {
      position: fixed;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background: var(--c-bg);
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      z-index: 1000;
      font-family: var(--font-gothic);
      color: var(--c-accent);
      text-align: center;
    }

    .loading-icon {
      width: 100px;
      height: 100px;
      position: relative;
      margin-bottom: 20px;
    }

    .loading-icon::before,
    .loading-icon::after {
      content: '';
      position: absolute;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      border-radius: 50%;
      border: 3px solid transparent;
    }

    .loading-icon::before {
      border-top: 3px solid var(--c-accent);
      border-right: 3px solid var(--c-accent);
      animation: spin 1.5s linear infinite;
    }

    .loading-icon::after {
      border-bottom: 3px solid var(--c-magic);
      border-left: 3px solid var(--c-magic);
      animation: spinReverse 1s linear infinite;
    }

    @keyframes spin {
      0% { transform: rotate(0deg); }
      100% { transform: rotate(360deg); }
    }

    @keyframes spinReverse {
      0% { transform: rotate(0deg); }
      100% { transform: rotate(-360deg); }
    }

    .loading-empty-msg {
      padding: 50px;
      text-align: center;
      color: var(--c-text-dim);
      font-family: var(--font-gothic);
    }
    
    .loading-empty-msg::before {
      content: "❄";
      display: block;
      font-size: 80px;
      margin-bottom: 10px;
    }

    /* 数值编辑框 */
    .editable-value.empty {
      color: var(--c-text-empty); 
    }

    input.inline-editor {
      box-sizing: border-box;
      max-width: 100%;
      width: 100%;
      height: 100%;
      text-align: center;
      background: var(--c-bg-80);
      border: 1px solid var(--c-accent);
      color: var(--c-text-edit);
      font-family: var(--font-serif);
      font-size: 13px;
      padding: 2px 4px;
      border-radius: 3px;
    }

    input.inline-editor:focus {
      outline: none;
      box-shadow: 0 0 10px var(--c-accent-20);
    }

    /* 文本编辑框 */
    .editable-textarea {
      position: relative;
      cursor: pointer;
      border-radius: 3px;
      display: flex;  
      flex-direction: column;
      font-family: var(--font-gothic);
      font-size: 14px;
      overflow: hidden;
      transition: background 0.15s ease;
    }

    .editable-textarea-content {
      display: block;
      width: 100%;
      height: 100%;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
      padding: 2px 4px;
      box-sizing: border-box;
      white-space: pre-wrap;
      word-break: break-all;
      text-align: left;
    }

    .editable-textarea::after {
      content: '';
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      bottom: 0;
      border: 1px dashed var(--c-accent);
      border-radius: 3px;
      opacity: 0;
      pointer-events: none;
      transition: opacity 0.15s ease;
    }

    .editable-textarea:hover {
      background: var(--c-hover);
    }

    .editable-textarea:hover::after {
      opacity: 1;
    }
    
    .editable-textarea.empty .editable-textarea-content {
      color: var(--c-text-empty);
      font-style: italic;
    }

    .editable-textarea.h60 { height: 60px;}
    .editable-textarea.h100 { height: 100px;}
    .editable-textarea.h160 { height: 160px;}
    .editable-textarea.h300 { height: 300px;}

    .editable-textarea.inblock {
      margin-top: 6px;
      margin-bottom: 6px;
      margin-left: 9px;
      margin-right: 9px;
    }

    .editable-textarea.inblock-top {
      margin-top: 2px;
      margin-bottom: 6px;
      margin-left: 9px;
      margin-right: 9px;
    }

    textarea.text-editor {
      height: 100%;
      width: 100%;
      background: var(--c-bg-80);
      border: 1px solid var(--c-accent);
      color: var(--c-text-edit);
      font-family: var(--font-serif);
      font-size: 13px;
      padding: 2px 4px;
      border-radius: 3px;
      resize: none;
    }
    
    textarea.text-editor:focus {
      outline: none;
      box-shadow: 0 0 10px var(--c-accent-20);
    }

    /* 主体容器 */
    .sheet {
      max-width: 700px;
      min-width: 0;
      width: 100%;
      margin: 0 auto;
      background: var(--c-card-bg);
      border: 2px solid var(--c-border);        
      box-shadow: 
        0 10px 30px -5px rgba(0, 0, 0, 0.8),      
        0 0 0 1px var(--c-accent-20) inset;  
      position: relative;
      padding: 0 10px;
      border-radius: 16px;
      overflow: hidden;
      scroll-behavior: smooth;
    }

    .sheet::before {
      content: "";
      position: absolute;
      top: 10px;
      left: 10px;
      right: 10px;
      bottom: 10px;
      border: 1px solid var(--c-border); 
      pointer-events: none;
      border-radius: 12px;
    }

    /* 标题 */
    .sheet__title {
      position: relative;
      font-family: var(--font-gothic);
      color: var(--c-accent);
      font-size: 17px;
      padding: 8px 12px;
      text-transform: uppercase;
      letter-spacing: 2px;
      border-left: 1px solid var(--c-border); 
      border-right: 1px solid var(--c-border); 
      background-image: linear-gradient(to right, rgba(15, 18, 26, 0.8), rgba(42, 49, 66, 0.5), rgba(15, 18, 26, 0.8)); 
      flex-shrink: 0;
    }

    .sheet__title::before {
      content: "";
      position: absolute;
      top: -1px;
      left: 20%;
      width: 60%;
      height: 1px;
      background: linear-gradient(to right, transparent, var(--c-accent), transparent);
      pointer-events: none;
    }

    .sheet__title::after {
      content: "";
      position: absolute;
      bottom: -1px;
      left: 20%;
      width: 60%;
      height: 1px;
      background: linear-gradient(to right, transparent, var(--c-accent), transparent);
      pointer-events: none;
    }

    .sheet__title.left { border-right: none; }
    .sheet__title.right { border-left: none; }

    .sheet__title-icon {
      font-family: "Font Awesome 7 Free";
      font-weight: 400;
      font-size: 14px;
      margin-right: 8px;
      opacity: 0.7;
      display: inline-block;
    }
    .sheet__title-icon::before {
      content: "\\f2dc"; 
    }

    .sheet__title.small {
      font-size: 15px;
      padding: 8px 12px;
      background: linear-gradient(to right, rgba(15, 18, 26, 0.6), rgba(42, 49, 66, 0.3), rgba(15, 18, 26, 0.6));
    }

    .sheet__title.small::after {
      background: none;
    }

    /* 基本信息区域 */
    .sheet__header {
      display: flex;
      align-items: center;
      padding: 20px 20px 10px 20px;
      position: relative;
      border-bottom: 1px solid var(--c-border);
    }

    .sheet__avatar {
      width: 80px;
      height: 80px;
      border-radius: 50%;
      background: linear-gradient(135deg, var(--c-bg), var(--c-card-bg));
      border: 1px solid var(--c-border);
      display: flex;
      align-items: center;
      justify-content: center;
      margin-right: 20px;
      overflow: hidden;
      font-size: 36px;
      color: var(--c-text-dim);
      flex-shrink: 0;
      box-shadow: 0 0 10px var(--c-accent-20);
      position: relative;
    }

    .sheet__avatar::before {
      content: "";
      position: absolute;
      top: -2px; left: -2px; right: -2px; bottom: -2px;
      background: linear-gradient(45deg, transparent, var(--c-accent), transparent);
      border-radius: 50%;
      z-index: -1;
      opacity: 0.5;
    }

    .sheet__avatar img {
      width: 100%; height: 100%; object-fit: cover;
    }

    .sheet__info {
      flex: 1;
      min-width: 0;
    }

    .sheet__name-row {
      font-family: var(--font-gothic);
      display: flex;
      justify-content: space-between;
      align-items: center;
      width: 100%;
    }

    .sheet__name {
      font-size: 30px;
      font-weight: bold;
      color: var(--c-text-main);
      letter-spacing: 2px;
      margin-bottom: 5px;
      text-shadow: 0 2px 10px var(--c-accent-20);
      position: relative;
      display: inline-block;
    }

    .sheet__name::after {
      content: "";
      position: absolute;
      bottom: -5px;
      left: 0;
      width: 100%;
      height: 1px;
      background: linear-gradient(to right, transparent, var(--c-accent), transparent);
    }

    .sheet__era-toggle {   
      display: inline-block;                                        
      font-size: 18px;    
      text-align: center;  
      color: var(--c-text-main);
      cursor: pointer;
      padding: 2px 12px;
      border-radius: 12px;
      transition: background 0.15s ease, border-color 0.15s ease, text-shadow 0.15s ease; 
      border: 1px solid transparent;  
    }

    .sheet__era-toggle:hover {
      background: var(--c-hover);
      border-color: var(--c-accent-20);
      text-shadow: 0 0 4px currentColor;
    }

    .sheet__basic {
      display: flex;
      flex-wrap: wrap;  
      gap: 0px 12px;
      margin-top: 8px;
      font-family: var(--font-gothic);
      font-size: 14px;
      color: var(--c-text-dim);
    }

    .sheet__basic-item {
      display: flex;
      align-items: center;
      gap: 4px;
      min-width: 0;  
    }

    .sheet__basic-label {
      color: var(--c-text-dim);
      flex-shrink: 0;
    }

    .sheet__basic-value { 
      display: inline-block;         
      height: 26px;                                            
      min-width: 50px;         
      white-space: nowrap;        
      overflow: hidden;          
      text-overflow: ellipsis; 
      text-align: center;  
      color: var(--c-text-main);
      cursor: pointer;
      padding: 2px 4px;
      border-radius: 3px;
      transition: text-shadow 0.15s ease;
    }

    .sheet__basic-value:hover {
      text-shadow: 0 0 4px currentColor;
    }

    .sheet__appearance {
      margin-top: 10px;
      padding: 0;
      display: flex;
      flex-direction: column;
    }

    .sheet__appearance-label {
      font-family: var(--font-gothic);
      font-size: 14px;
      color: var(--c-accent);
      display: block;
    }

    /* 属性与状态区域 */
    .sheet__status-grid {
      display: grid;
      grid-template-columns: 1fr 1fr 1fr;
      gap: 1px;
      background: var(--c-border);
      border-top: 1px solid var(--c-border);
      border-bottom: 1px solid var(--c-border);
    }

    .sheet__status-item {
      background: var(--c-card-bg);
      padding: 8px 4px 5px 4px;
      text-align: center;
    }

    .sheet__status-label-row {
      height: 24px;
      font-family: var(--font-gothic);
      display: flex;
      justify-content: center;
      align-items: center;
      gap: 4px;
    }

    .sheet__status-label {
      font-size: 14px;
      color: var(--c-text-dim);
      display: block;
    }

    .sheet__status-toggle {
      display: inline-block;   
      font-size: 12px;
      color: var(--c-text-main);
      background-color: rgba(var(--c-state-base), 0.06);
      cursor: pointer;
      padding: 2px 8px;
      border: 1px solid transparent;  
      border-radius: 12px;
      transition: border-color 0.15s ease, text-shadow 0.15s ease;
    }

    .sheet__status-toggle:hover {
      border-color: var(--c-accent-20);
      text-shadow: 0 0 4px rgba(var(--c-state-base), 0.6);
    }
    
    .sheet__status-toggle[data-attr="健康状态"][data-value="健康"] { --c-state-base: 0, 255, 0; }
    .sheet__status-toggle[data-attr="健康状态"][data-value="轻伤"] { --c-state-base: 160, 255, 0; }
    .sheet__status-toggle[data-attr="健康状态"][data-value="重伤"] { --c-state-base: 255, 255, 0; }
    .sheet__status-toggle[data-attr="健康状态"][data-value="昏迷"] { --c-state-base: 255, 165, 0; }
    .sheet__status-toggle[data-attr="健康状态"][data-value="濒死"] { --c-state-base: 255, 69, 0; }
    .sheet__status-toggle[data-attr="健康状态"][data-value="死亡"] { --c-state-base: 255, 0, 0; }

    .sheet__status-toggle[data-attr="精神状态"][data-value="神志清醒"] { --c-state-base: 0, 255, 0; }
    .sheet__status-toggle[data-attr="精神状态"][data-value="临时性疯狂"] { --c-state-base: 255, 255, 0; }
    .sheet__status-toggle[data-attr="精神状态"][data-value="不定性疯狂"] { --c-state-base: 255, 165, 0; }
    .sheet__status-toggle[data-attr="精神状态"][data-value="永久性疯狂"] { --c-state-base: 255, 0, 0; }

    .sheet__status-row {
      height: 28px;
      font-family: var(--font-gothic);
      display: flex;
      align-items: baseline;
      justify-content: center;
      white-space: nowrap;
    }

    .sheet__status-val {
      display: inline-block;   
      font-size: 20px;
      font-weight: bold;
      cursor: pointer;
      transition: text-shadow 0.15s ease;
    }

    .sheet__status-val:hover {
      text-shadow: 0 0 8px currentColor;
    }

    .st-hp .sheet__status-val { color: var(--c-health); }
    .st-mp .sheet__status-val { color: var(--c-magic); }
    .st-san .sheet__status-val { color: var(--c-sanity); }

    .sheet__status-val::after {
      font-family: "Font Awesome 7 Free";   
      font-weight: 900;
      font-size: 14px;        
      margin-left: 4px;
    }
    .sheet__status-val[data-editing="1"]::after {
      display: none;
    }

    .st-hp .sheet__status-val::after { content: "\\f004"; }
    .st-mp .sheet__status-val::after { content: "\\e2ca"; }
    .st-san .sheet__status-val::after { content: "\\f5dc"; }

    .sheet__status-max {
      display: inline-block; 
      font-size: 16px;
      color: var(--c-text-dim);
      cursor: pointer;
      transition: color 0.15s ease;
    }

    .sheet__status-max::before {
      content: "/";
      color: var(--c-text-dim);
      margin-left: 2px;
      margin-right: 2px;
    }

    .sheet__status-max:hover {
      color: var(--c-text-main);
    }

    .sheet__status-desc-icon {
      font-family: "Font Awesome 7 Free";
      font-weight: 400;
      color: var(--c-text-dim);
      opacity: 0.7;
      font-size: 12px;
      cursor: help;
      margin-left: 2px;
      transition: opacity 0.15s ease;
    }

    .sheet__status-desc-icon::before {
      content: "\\f29c";
    }

    .sheet__status-desc-icon:hover {
      opacity: 1;
    }
    
    .sheet__combat-grid {
      display: grid;
      grid-template-columns: repeat(6, 1fr);  
      gap: 1px;
      background: var(--c-border);
      border-bottom: 1px solid var(--c-border);
    }
    
    .sheet__combat-item {
      background: var(--c-card-bg);
      padding: 8px 4px 5px 4px;
      text-align: center;
    }

    .sheet__combat-label {
      font-family: var(--font-gothic);
      font-size: 14px;
      color: var(--c-text-dim);
      display: block;
    }
    
    .sheet__combat-val {
      height: 24px;
      display: inline-block;  
      vertical-align: middle;
      font-family: var(--font-gothic);
      font-size: 16px;
      color: var(--c-text-main);
      transition: text-shadow 0.15s ease;
    }

    .st-ar .sheet__combat-val {
      display: inline-block; 
      font-weight: bold;
      cursor: pointer;
    }

    .st-ar .sheet__combat-val:hover {
      text-shadow: 0 0 8px currentColor;
    }

    .st-ar .sheet__combat-val { color: var(--c-armor); }

    .sheet__combat-val::after {
      font-family: "Font Awesome 7 Free"; 
      font-weight: 900;  
      font-size: 14px;        
      margin-left: 4px;
    }
    .sheet__combat-val[data-editing="1"]::after {
      display: none;
    }

    .st-ar .sheet__combat-val::after { content: "\\f3ed"; }

    .sheet__char-radar-extra-grid {
      display: grid;
      grid-template-columns: 1fr clamp(0px, 50%, 225px); 
      gap: 1px;
      background: var(--c-border);
      border-bottom: 1px solid var(--c-border);
      grid-template-areas: 
        "char-grid char-radar"
        "extranote char-radar";
    }

    .sheet__char-grid {
      grid-area: char-grid;
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 1px;
      background: var(--c-border);
    }
    
    .sheet__char-item {
      background: var(--c-card-bg);
      padding: 10px 12px;                   
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .sheet__char-label {
      font-family: var(--font-gothic);
      color: var(--c-text-dim);
      font-size: 16px;     
      cursor: pointer;
      display: flex;
      align-items: center;
      transition: color 0.15s ease;       
      flex-shrink: 0; 
    }
    
    .sheet__char-label:hover { 
      color: var(--c-accent); 
    }
    
    .sheet__char-val {
      height: 30px;
      display: inline-block;  
      font-family: var(--font-gothic);
      font-size: 20px;    
      font-weight: bold;
      color: var(--c-text-main);
      cursor: pointer;   
      text-align: right;
      transition: text-shadow 0.15s ease;
    }
    
    .sheet__char-val:hover { 
      text-shadow: 0 0 8px currentColor;
    }
    
    .sheet__radar-item {
      grid-area: char-radar;
      background: var(--c-card-bg);
      padding: 8px;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: flex-start;
    }

    #canvasCharRadar {
      width: 100%;
      height: auto;
      aspect-ratio: 1/1;
      display: block;
    }

    .sheet__char-total {
      margin-top: 8px;
      font-family: var(--font-gothic);
      font-size: 13px;
      color: var(--c-text-dim);
      text-align: center;
    }

    .sheet__extranote {
      grid-area: extranote;
      display: flex;
      flex-direction: column;
      padding: 8px 12px;
      background: var(--c-card-bg);
    }

    .sheet__extranote-label {
      font-family: var(--font-gothic);
      font-size: 14px;
      color: var(--c-accent);
      display: flex;
      align-items: center;
    }

    @media (max-width: 580px) {
      .sheet__char-radar-extra-grid {
        grid-template-areas: 
          "char-grid char-radar"
          "extranote extranote";
      }
      .sheet__char-grid {
        grid-template-columns: repeat(2, 1fr);
      }
      .sheet__char-item:last-child {
        grid-column: span 2;
      }
      .sheet__credit-row {
        flex-direction: column;
        gap: 8px;
        padding: 8px 12px 8px 12px;
      }
      .editable-textarea.media-h130 {
        height: 130px;
      }
    }

    /* 职业选择区域 */
    .sheet__job {
      padding: 12px 20px;
      border-top: 1px solid var(--c-border);
      border-bottom: 1px solid var(--c-border);
      background:  var(--c-card-bg);
    }

    .sheet__job-select {
      width: 100%;
      background: var(--c-bg-80);
      border: 1px solid var(--c-border);
      color: var(--c-text-empty);
      font-family: var(--font-gothic);
      font-size: 14px;
      padding: 8px 12px;
      border-radius: 5px;
      cursor: pointer;
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }

    .sheet__job-select.selected {
      color: var(--c-text-main);
    }

    .sheet__job-select optgroup {
      color: var(--c-text-dim); 
    }

    .sheet__job-select option {
      color: var(--c-text-main); 
    }

    .sheet__job-select:hover {
      border-color: var(--c-accent);
    }

    .sheet__job-select:focus {
      outline: none;
      border-color: var(--c-accent);
      box-shadow: 0 0 10px var(--c-accent-20);
    }

    .sheet__job-detail-grid {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      row-gap: 5px;
      column-gap: 20px;
      margin-top: 10px;
    }

    .sheet__job-detail-item {
      display: flex;
      flex-direction: column;
      font-family: var(--font-gothic);
    }

    .sheet__job-detail-label {
      color: var(--c-text-dim);
      font-size: 14px;
      margin-bottom: 3px;
    }

    .sheet__job-detail-value {
      color: var(--c-text-main);
      font-size: 14px;
      white-space: pre-line;
    }

    .sheet__job-desc {
      margin-top: 10px;
      padding: 4px 10px;
      background: var(--c-hover);
      border-left: 2px solid var(--c-accent);
      font-size: 13px;
      font-family: var(--font-gothic);
      color: var(--c-text-dim);
      max-height: 95px;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
      white-space: pre-line;
    }

    /* 技能区域 */
    .sheet__skill-info {
      display: flex;
      align-items: center;
      padding: 10px 20px;
      border-top: 1px solid var(--c-border);
      font-family: var(--font-gothic);
      font-size: 13px;
    }

    .sheet__skill-points-box {
      flex: 1;
      display: flex;
      flex-wrap: wrap;
      gap: 0px 12px;
    }

    .sheet__skill-points-item {
      display: flex;
      align-items: center;
      gap: 4px;
    }

    .sheet__skill-points-label {
      color: var(--c-text-dim);
    }

    .sheet__skill-points-value {
      color: var(--c-text-main);
    }

    .sheet__skill-lock {
      display: inline-flex;
      align-items: center;
      color: var(--c-text-dim);
      font-size: 14px;
      cursor: pointer;
      flex-shrink: 0; 
      margin-left: 6px;
      transition: text-shadow 0.15s ease;
    }

    .sheet__skill-lock:hover{
      text-shadow: 0 0 8px currentColor;
    }

    .sheet__skill-lock::before {
      font-family: "Font Awesome 7 Free";
      font-weight: 900;
      font-size: 16px;
      margin-right: 4px;
    }

    .sheet__skill-lock[data-value="false"] { color: var(--c-text-dim); }
    .sheet__skill-lock[data-value="true"] { color: var(--c-accent); }
    .sheet__skill-lock[data-value="false"]::before { content: "\\f09c"; }
    .sheet__skill-lock[data-value="true"]::before { content: "\\f023"; }

    .sheet__skill-search {
      display: flex;
      align-items: baseline;
      margin-left: 20px;
      margin-right: 20px;
      margin-bottom: 10px;
      padding: 1px 12px 5px 12px; 
      background: var(--c-bg-80);
      border: 1px solid var(--c-border);
      border-radius: 5px;
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }

    .sheet__skill-search:hover {
      border-color: var(--c-accent);
    }

    .sheet__skill-search:focus-within {
      border-color: var(--c-accent);
      box-shadow: 0 0 10px var(--c-accent-20);
    }

    .sheet__skill-search-icon {
      order: 0;
      flex-shrink: 0;
      margin-right: 8px; 
      transition: color 0.15s ease;
    }

    .sheet__skill-search-icon::before {
      font-family: "Font Awesome 7 Free";
      font-weight: 900;
      content: "\\f002"; 
      color: var(--c-text-dim);
      font-size: 14px;
    }

    .sheet__skill-search-input:not(:placeholder-shown) + .sheet__skill-search-icon::before {
      color: var(--c-accent);
    }

    .sheet__skill-search-input {
      order: 1;
      flex: 1;
      background: transparent;
      border: none;
      color: var(--c-text-main);
      font-family: var(--font-gothic);
      font-size: 14px;
    }

    .sheet__skill-search-input:focus {
      outline: none;
    }

    .sheet__skill-search-input::placeholder {
      color: var(--c-text-empty);
    }

    .sheet__skill-tabs {
      display: flex;
      flex-wrap: wrap;
      gap: 2px;
      border-bottom: 1px solid var(--c-border);
      background: var(--c-card-bg);
      padding: 0 8px;
    }

    .sheet__skill-tab {
      background: var(--c-card-bg);
      border: 1px solid var(--c-border);
      border-bottom: none;
      color: var(--c-text-dim);
      padding: 8px 12px;
      cursor: pointer;
      font-family: var(--font-gothic);
      font-size: 14px;
      transition: color 0.15s ease;
      border-top-left-radius: 5px;
      border-top-right-radius: 5px;
    }

    .sheet__skill-tab[data-category="收藏夹"]::before {
      font-family: "Font Awesome 7 Free";
      font-weight: 900;
      content: "\\f005"; 
      font-size: inherit;
    }

    .sheet__skill-tab:hover {
      color: var(--c-accent);
    }

    .sheet__skill-tab.active {
      color: var(--c-accent);
      border-bottom-color: var(--c-card-bg);
      margin-bottom: -1px;
      position: relative;
    }

    .sheet__skill-tab[data-category="收藏夹"].active::before {
      color: var(--c-favorite);
    }

    .sheet__skill-panels {
      background: var(--c-card-bg);
      border: 1px solid var(--c-border);
      border-top: none;
    }

    .sheet__skill-panel {
      display: none;
      height: 400px;
      background: var(--c-card-bg);
      flex-direction: column;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
      scrollbar-gutter: stable both-edges;
    }

    .sheet__skill-panel.active {
      display: flex;
    }

    .sheet__skill-item {
      display: flex;
      flex-direction: column;
      padding: 8px 10px 1px 10px;
      gap: 2px;
      border-bottom: 1px solid var(--c-hover);
      font-family: var(--font-gothic);
      transition: background 0.15s ease;
    }

    .sheet__skill-item.filter-hidden {
      display: none;
    }

    .sheet__skill-item:hover {
      background: var(--c-hover);
    }

    .sheet__skill-item:last-child {
        border-bottom: none;
    }

    .sheet__skill-label {
      display: flex;
      align-items: center;
      width: 100%;
      gap: 4px;
    }

    .sheet__skill-name {
      cursor: pointer;
      color: var(--c-text-main);
      font-size: 14px;
      transition: color 0.15s ease;
    }

    .sheet__skill-name:hover {
      color: var(--c-accent);
    }

    .sheet__skill-favorite {
      background: none;
      border: none;
      cursor: pointer;
      font-size: 14px;
      padding: 0 4px;
      transition: color 0.15s ease;
      font-family: "Font Awesome 7 Free";
      font-weight: 900;
    }

    .sheet__skill-favorite::before {
      content: "\\f005"; 
      color: var(--c-text-dim);
    }

    .sheet__skill-favorite:hover::before {
      color: var(--c-accent);
    }

    .sheet__skill-favorite.active::before {
      color: var(--c-favorite);
    }

    .sheet__skill-values-row {
      display: flex;
      gap: 5px;
      width: 100%;
    }

    .sheet__skill-value-col {
      flex: 1;
      text-align: center;
    }

    .sheet__skill-value-label {
      color: var(--c-text-dim);
      display: block;
      font-size: 13px;
    }

    .sheet__skill-value {
      height: 24px;
      font-family: var(--font-gothic);
      font-size: 16px;
      color: var(--c-text-main);
      text-align: center;
      display: inline-block;
      cursor: pointer;
      transition: text-shadow 0.15s ease;
    }

    .sheet__skill-value-col:not(.val-base):not(.val-half) .sheet__skill-value:hover { text-shadow: 0 0 8px currentColor; }

    .val-base .sheet__skill-value { color: var(--c-text-dim); }
    .val-interest .sheet__skill-value { color: var(--c-sanity); }
    .val-occupation .sheet__skill-value { color: var(--c-magic); }
    .val-growth .sheet__skill-value { color: var(--c-accent); }
    .val-total .sheet__skill-value { font-weight: bold; }
    .val-half .sheet__skill-value { font-size: 13px; }

    /* 武器区域 */
    .sheet__weapons {
      background: var(--c-card-bg);
      border-bottom: 1px solid var(--c-border);
      border-top: 1px solid var(--c-border);
      overflow-x: auto;
      -webkit-overflow-scrolling: touch;
    }

    .sheet__weapons-table {
      width: auto; 
      min-width: 670px;
      table-layout: fixed;
      border-collapse: collapse;
      font-family: var(--font-gothic);
      font-size: 14px;
    }

    .sheet__weapons-table th:nth-child(1),
    .sheet__weapons-table td:nth-child(1) { width: 20%; }  
    .sheet__weapons-table th:nth-child(2),
    .sheet__weapons-table td:nth-child(2) { width: 15%; }  
    .sheet__weapons-table th:nth-child(3),
    .sheet__weapons-table td:nth-child(3) { width: 15%; } 
    .sheet__weapons-table th:nth-child(4),
    .sheet__weapons-table td:nth-child(4) { width: 10%; } 
    .sheet__weapons-table th:nth-child(5),
    .sheet__weapons-table td:nth-child(5) { width: 10%; } 
    .sheet__weapons-table th:nth-child(6),
    .sheet__weapons-table td:nth-child(6) { width: 10%; } 
    .sheet__weapons-table th:nth-child(7),
    .sheet__weapons-table td:nth-child(7) { width: 10%; } 
    .sheet__weapons-table th:nth-child(8),
    .sheet__weapons-table td:nth-child(8) { width: 10%; } 

    .sheet__weapons-table th:first-child,
    .sheet__weapons-table td:first-child { padding-left: 12px; }
    .sheet__weapons-table th:last-child,
    .sheet__weapons-table td:last-child { padding-right: 12px; }

    .sheet__weapons-header {
      height: 34px;
      background: var(--c-bg-40);
      color: var(--c-text-dim);
      border-bottom: 1px solid var(--c-hover);
    }

    .sheet__weapons-row {
      height: 38px;
      background: var(--c-card-bg);
      color: var(--c-text-main);
      border-bottom: 1px solid var(--c-hover);
    }

    .sheet__weapon-item {
      padding: 4px;
      font-weight: normal;
      text-align: center;
      vertical-align: middle;
      white-space: nowrap;
    }

    .sheet__weapon-select {
      width: 100%;
      min-width: 120px;
      background: var(--c-bg-80);
      border: 1px solid var(--c-border);
      color: var(--c-text-empty);
      font-family: var(--font-gothic);
      font-size: 14px;
      padding: 4px;
      border-radius: 3px;
      cursor: pointer;
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }

    .sheet__weapon-select.selected {
      color: var(--c-text-main);
    }

    .sheet__weapon-select optgroup {
      color: var(--c-text-dim); 
    }

    .sheet__weapon-select option {
      color: var(--c-text-main); 
    }

    .sheet__weapon-select:hover {
      border-color: var(--c-accent);
    }

    .sheet__weapon-select:focus {
      outline: none;
      border-color: var(--c-accent);
      box-shadow: 0 0 6px var(--c-accent-20);
    }

    .sheet__weapon-skill.clickable {
      text-align: center;
      cursor: pointer;
      color: var(--c-text-main);
      transition: color 0.15s;
    }

    .sheet__weapon-skill.clickable:hover {
      color: var(--c-accent);
    }

    .sheet__weapon-damage {
      background: transparent;
      font-size: 14px;
      text-align: center;
      cursor: pointer;
      transition: text-shadow 0.15s ease;
    }

    .sheet__weapon-damage[tabindex="-1"] {
      cursor: default;
    }

    .sheet__weapon-damage:hover {
      text-shadow: 0 0 4px currentColor;
    }

    .sheet__weapon-damage[tabindex="-1"]:hover {
      text-shadow: none;
    }


    .sheet__weapons-note {
      padding: 6px 12px;
      color: var(--c-text-dim);
      font-size: 13px;
      font-family: var(--font-gothic);
    }

    /* 资产与随身物品区域 */
    .sheet__assets-background-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 1px;
      background: var(--c-border);
      border-bottom: 1px solid var(--c-border);
    }

    .sheet__assets-background-left {
      display: flex;
      flex-direction: column;
      background: var(--c-border);
      gap: 1px;
      height: 100%;
    }

    .sheet__assets {
      background: var(--c-card-bg);
      display: flex;
      flex-direction: column;
      
    }
    
    .sheet__credit-row {
      display: flex;
      padding: 10px 12px 0px 12px;               
      font-family: var(--font-gothic);
      font-size: 14px;
      border-top: 1px solid var(--c-border);
    }

    .sheet__credit-col {
      flex: 1;                
      display: flex; 
      align-items: center;               
    }

    .sheet__credit-label {
      color: var(--c-text-dim);
      white-space: nowrap;
    }

    .sheet__credit-value {                                      
      margin-left: 4px;    
      white-space: nowrap;
      text-align: center;  
      color: var(--c-text-main);
    }

    .sheet__items {
      background: var(--c-card-bg);
      display: flex;
      flex-direction: column;
      height: 100%;
    }

    .sheet__items-border {
      border-top: 1px solid var(--c-border);
    }

    /* 背景故事区域 */
    .sheet__assets-background-right {
      background: var(--c-card-bg);
      display: flex;
      flex-direction: column;
      min-width: 0;
    }

    .sheet__background {
      display: flex;
      flex-direction: column;
      width: 100%;
      min-width: 0;
      padding-top: 10px;
      gap: 5px;
      border-top: 1px solid var(--c-border);
    }

    .sheet__background-item {
      display: flex;
      flex-direction: column;
      width: 100%;
      min-width: 0;
      font-family: var(--font-gothic);
      font-size: 14px;
    }

    .sheet__background-label {
      padding: 0px 12px;
      color: var(--c-text-dim);
    }

    .sheet__background-item:has(.editable-textarea) .sheet__background-label {
      color: var(--c-accent);
    }

    .sheet__background-value { 
      display: block;         
      height: 26px;
      min-width: 0;                                                
      white-space: nowrap;        
      overflow: hidden;          
      text-overflow: ellipsis; 
      color: var(--c-text-main);
      cursor: pointer;
      padding: 2px 4px;
      margin-left: 12px;
      margin-right: 12px;
      border-radius: 3px;
      transition: background 0.15s ease, text-shadow 0.15s ease;
    }

    .sheet__background-value:hover {
      background: var(--c-hover);
      text-shadow: 0 0 4px currentColor;
    }

    /* 神话与经历区域 */
    .sheet__myth-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 1px;                     
      background: var(--c-border);
    }

    .sheet__myth-item {
      display: flex;
      flex-direction: column;
      background: var(--c-card-bg);
      border-bottom: 1px solid var(--c-border);
    }

    /* 其他属性区域 */
    .sheet__other-attrs {
      display: flex;
      flex-wrap: wrap;
      gap: 6px 12px;
      padding: 8px 12px;
      margin-bottom: 10px;
      background: var(--c-card-bg);
      border-top: 1px solid var(--c-border);
      font-family: var(--font-gothic);
      font-size: 13px;
      height: 120px;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
      scrollbar-gutter: stable both-edges;
      align-content: flex-start; 
    }

    .sheet__other-attr-item {
      display: flex;
      background: var(--c-hover);
      border-radius: 12px;
      gap: 2px;
      padding: 4px 8px;
      max-width: 100%;         
      flex-wrap: wrap;  
      line-height: 1.1;      
    }

    .sheet__other-attr-key {
      color: var(--c-text-dim);
      white-space: nowrap;    
      flex-shrink: 0;          
    }

    .sheet__other-attr-value {
      color: var(--c-text-main);
      word-break: break-all;   
      flex: 1;                
    }

    /* 掷骰弹窗 */
    .modal__overlay {
      position: fixed;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background: rgba(0, 0, 0, 0.75);
      display: none;
      justify-content: center;
      align-items: center;
      z-index: 1000;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
    }
    .modal__overlay.active {
      display: flex;
    }

    .modal__roll {
      background: var(--c-card-bg);
      border: 1px solid var(--c-border);
      border-radius: 8px;
      width: 90%;
      max-width: 340px;
      box-shadow: 0 10px 30px rgba(0, 0, 0, 0.8);
      padding: 16px;
    }
    .modal__choice {
      background: var(--c-card-bg);
      border: 1px solid var(--c-border);
      border-radius: 8px;
      width: 90%;
      max-width: 280px;
      box-shadow: 0 10px 30px rgba(0, 0, 0, 0.8);
      padding: 16px;
      text-align: center;
    }

    .modal__header {
      text-align: center;
      border-bottom: 1px solid var(--c-border);
      padding-bottom: 10px;
      margin-bottom: 12px;
    }
    .modal__head-name {
      font-family: var(--font-gothic);
      font-size: 18px;
      font-weight: bold;
      color: var(--c-text-main);
    }
    .modal__head-val {
      font-family: var(--font-gothic);
      color: var(--c-accent);
      font-size: 24px;
      display: block;
      margin-top: 2px;
    }

    .modal__body   { margin-bottom: 12px; }
    .modal__footer { display: flex; gap: 8px; }

    .modal__row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 8px;
      gap: 8px;
      font-family: var(--font-gothic);
      font-size: 14px;
      color: var(--c-text-dim);
    }
    .modal__row-label { flex-shrink: 0; }

    .modal__group {
      display: flex;
      gap: 4px;
    }

    .modal__num {
      display: flex;
      align-items: center;
      gap: 2px;
    }
    .modal__num-btn {
      width: 28px;
      height: 28px;
      border: 1px solid var(--c-border);
      border-radius: 4px;
      background: var(--c-bg-40);
      color: var(--c-text-dim);
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      font-family: var(--font-gothic);
      font-size: 14px;
      transition: border-color 0.15s ease, color 0.15s ease;
    }
    .modal__num-btn:hover {
      border-color: var(--c-accent);
      color: var(--c-accent);
    }

    .modal__num-input {
      width: 44px;
      height: 28px;
      text-align: center;
      background: var(--c-bg-80);
      border: 1px solid var(--c-border);
      border-radius: 3px;
      color: var(--c-text-main);
      font-family: var(--font-gothic);
      font-size: 14px;
    }
    .modal__num-input:focus {
      outline: none;
    }
    .modal__expr-input {
      flex: 1;
      height: 28px;
      margin-left: 12px;
      background: var(--c-bg-80);
      border: 1px solid var(--c-border);
      border-radius: 3px;
      color: var(--c-text-main);
      font-family: var(--font-gothic);
      font-size: 14px;
      padding: 2px 8px;
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }
    .modal__expr-input:hover {
      border-color: var(--c-accent);
    }
    .modal__expr-input:focus {
      outline: none;
      border-color: var(--c-accent);
      box-shadow: 0 0 10px var(--c-accent-20);
    }

    .modal__toggle {
      padding: 6px 12px;
      border: 1px solid var(--c-border);
      border-radius: 6px;
      background: var(--c-bg-40);
      color: var(--c-text-dim);
      cursor: pointer;
      font-family: var(--font-gothic);
      font-size: 14px;
      transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;
    }
    .modal__toggle:hover {
      border-color: var(--c-accent);
      color: var(--c-accent);
    }
    .modal__toggle[data-active="true"] {
      background: var(--c-accent-20);
      border-color: var(--c-accent);
      color: var(--c-accent);
    }

    .modal__submit {
      flex: 1;
      padding: 8px;
      background: var(--c-accent-20);
      border: 1px solid var(--c-accent);
      border-radius: 6px;
      color: var(--c-accent);
      cursor: pointer;
      font-family: var(--font-gothic);
      font-size: 14px;
      font-weight: bold;
      text-align: center;
      transition: background 0.15s ease;
    }
    .modal__submit:hover {
      background: rgba(168, 199, 250, 0.35);
    }

    .modal__cancel {
      padding: 8px 16px;
      background: transparent;
      border: 1px solid var(--c-border);
      border-radius: 6px;
      color: var(--c-text-dim);
      cursor: pointer;
      font-family: var(--font-gothic);
      font-size: 14px;
      transition: border-color 0.15s ease, color 0.15s ease;
    }
    .modal__cancel:hover {
      border-color: var(--c-accent);
      color: var(--c-accent);
    }

    .modal__actions {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 8px;
      margin-bottom: 10px;
    }
    .modal__action-btn {
      padding: 10px 8px;
      border: 1px solid var(--c-border);
      border-radius: 6px;
      background: var(--c-bg-40);
      color: var(--c-text-dim);
      cursor: pointer;
      font-family: var(--font-gothic);
      font-size: 14px;
      transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;
    }
    .modal__action-btn:hover {
      border-color: var(--c-accent);
      background: var(--c-accent-20);
      color: var(--c-accent);
    }

  </style>
</head>

<body>
  <!-- 加载动画 -->
  <div id="loadingIndicator">
    <div class="loading-icon"></div>
    灵魂之灯长明<br>正在连接数据...请稍等
  </div>

  <!-- 主内容容器 -->
  <div id="content"></div>

  <!-- 技能掷骰弹窗 -->
  <div id="skillRollModal" class="modal__overlay">
    <div class="modal__roll">
      <div class="modal__header">
        <div class="modal__head-name" id="skillRollTitle">技能检定</div>
        <span class="modal__head-val" id="skillRollValue">50</span>
      </div>
      <div class="modal__body">
        <div class="modal__row">
          <button id="btnRollHidden" class="modal__toggle" data-active="false">暗骰</button>
          <div class="modal__group">
            <button id="btnRollBonus" class="modal__toggle" data-active="false">奖励骰</button>
            <button id="btnRollPenalty" class="modal__toggle" data-active="false">惩罚骰</button>
          </div>
        </div>
        <div id="rowRollBpCount" class="modal__row" style="display:none;">
          <span class="modal__row-label">奖励/惩罚骰个数</span>
          <div class="modal__num">
            <button id="btnRollBpDec" class="modal__num-btn">−</button>
            <input type="text" id="inputRollBpCount" value="1" readonly class="modal__num-input">
            <button id="btnRollBpInc" class="modal__num-btn">+</button>
          </div>
        </div>
        <div class="modal__row">
          <span class="modal__row-label">修正类型</span>
          <div class="modal__group">
            <button id="btnRollModSkill" class="modal__toggle" data-active="false">技能修正</button>
            <button id="btnRollModDice" class="modal__toggle" data-active="false">骰点修正</button>
          </div>
        </div>
        <div id="rowRollMod" class="modal__row" style="display:none;">
          <span class="modal__row-label">修正值</span>
          <input type="text" id="inputRollMod" placeholder="例如 +1D4 或 -2" class="modal__expr-input">
        </div>
        <div class="modal__row">
          <span class="modal__row-label">检定次数</span>
          <div class="modal__num">
            <button id="btnRollCountDec" class="modal__num-btn">−</button>
            <input type="text" id="inputRollCount" value="1" readonly class="modal__num-input">
            <button id="btnRollCountInc" class="modal__num-btn">+</button>
          </div>
        </div>
      </div>
      <div class="modal__footer">
        <button class="modal__submit" id="btnSkillRollSubmit">检定</button>
        <button class="modal__cancel" id="btnSkillRollCancel">取消</button>
      </div>
    </div>
  </div>

  <!-- 伤害掷骰弹窗 -->
  <div id="damageRollModal" class="modal__overlay">
    <div class="modal__roll">
      <div class="modal__header">
        <div class="modal__head-name" id="damageRollTitle">伤害掷骰</div>
        <span class="modal__head-val" id="damageRollExpression">1D6</span>
      </div>
      <div class="modal__body">
        <div class="modal__row">
          <button id="btnDamageRollHidden" class="modal__toggle" data-active="false">暗骰</button>
          <button id="btnDamageRollMod" class="modal__toggle" data-active="false">伤害修正</button>
        </div>
        <div id="rowDamageRollMod" class="modal__row" style="display:none;">
          <span class="modal__row-label">修正值</span>
          <input type="text" id="inputDamageRollMod" placeholder="例如 +1D4 或 -2" class="modal__expr-input">
        </div>
        <div class="modal__row">
          <span class="modal__row-label">掷骰次数</span>
          <div class="modal__num">
            <button id="btnDamageRollCountDec" class="modal__num-btn">−</button>
            <input type="text" id="inputDamageRollCount" value="1" readonly class="modal__num-input">
            <button id="btnDamageRollCountInc" class="modal__num-btn">+</button>
          </div>
        </div>
      </div>
      <div class="modal__footer">
        <button class="modal__submit" id="btnDamageRollSubmit">掷骰</button>
        <button class="modal__cancel" id="btnDamageRollCancel">取消</button>
      </div>
    </div>
  </div>

    <!-- 武器选择弹窗 -->
  <div id="weaponChoiceModal" class="modal__overlay">
    <div class="modal__choice">
      <div class="modal__header">
        <div class="modal__head-name">选择掷骰类型</div>
      </div>
      <div class="modal__actions">
        <button class="modal__action-btn" id="btnWeaponAttack">攻击掷骰</button>
        <button class="modal__action-btn" id="btnWeaponDamage">伤害掷骰</button>
      </div>
      <button class="modal__cancel" id="btnWeaponCancel">取消</button>
    </div>
  </div>

  <script>
    // ==================== 常量定义 ====================
    const CHAR_KEYS = ['力量', '体质', '体型', '敏捷', '外貌', '教育', '智力', '意志', '幸运'];
    const STATUS_KEYS = ['生命值', '生命值上限', '理智', '理智上限', '魔法值', '魔法值上限', '护甲'];
    const CHAR_INFO_KEY = '$SEALCHAT_人物信息';
    const TEXT_FIELDS = ['年龄', '性别', '故乡', '住址', '外貌描述',
                         '备注', '职业', '资产说明', '随身物品',
                         '思想与信念', '性格特点', '重要之人', '意义非凡之地',
                         '宝贵之物', '伤口和疤痕', '恐惧症和狂躁症', '个人详细描述',
                         '神话物品', '特质与能力', '法术', '第三类接触', '调查员经历', '调查员伙伴'
                        ];     
    const TOGGLE_FIELDS = {
      '时代': ['1920s', '现代', '维多利亚时代', '未知'],
      '健康状态': ['健康', '轻伤', '重伤', '昏迷', '濒死', '死亡'],
      '精神状态': ['神志清醒', '临时性疯狂', '不定性疯狂', '永久性疯狂'],
      '技能总值锁定': ['false', 'true'],
    };

    const CHAR_DESCRIPTIONS = {
      '力量': '力量 STR\\n0：衰弱，没法站起来甚至端起一杯茶。\\n15：弱者，虚弱。\\n50：普通人水平。\\n90：你见过的力气最大的人。\\n99：世界水平(奥赛举重冠军)，人类极限。\\n140：超越人类之力(例如大猩猩或马)。\\n200+：怪物之力(例如格拉基)。',
      '体质': '体质 CON\\n0：死亡。\\n1：体弱多病，易病难愈，可能在没有帮助的情况下无法自理。\\n15：身体虚弱，易突发疾病，易感到疼痛。\\n50：普通人水平。\\n90：不惧寒冷，强壮而精神。\\n99：钢铁之躯。能够承受巨大的疼痛，人类极限。\\n140：超越人类之体格(大象)。\\n200+：怪物之体，免疫大部分地球疾病(例如尼约格萨)。',
      '体型': '体型 SIZ\\n1：一个婴儿(1~12磅)。\\n15：孩童，或身短体瘦(矮人)(33磅/15kg)。\\n65：普通人类体型(中等身高和体重)(170磅/75kg)。\\n80：非常高，强健的体格或非常胖(240磅/110kg)。\\n99：某方面已经是超大号了(330磅/150kg)。\\n150：马或牛(960磅/436kg)。\\n180：记录中最重的人类(1400磅/634kg)。\\n200+：1920磅/872kg(例如昌格纳·方庚)。\\n注意：有些人类的体型可以超过99。',
      '敏捷': '敏捷 DEX\\n0：没有协助无法移动。\\n15：缓慢，笨拙，无法行动自如。\\n50：普通人水平。\\n90：高速而灵活，可以达成超凡的技艺(例如杂技演员，伟大的舞者)。\\n99：世界级运动员，人类极限。\\n120：超越人类之速(例如虎)。\\n200+：闪电之速，可以在人类反应过来之前完成一系列动作。',
      '外貌': '外貌 APP\\n0：十分难看，他人会对你报以恐惧、厌恶和怜悯。\\n15：挫，估计是因为受伤事故或先天如此。\\n50：普通人水平。\\n90：你见过的最漂亮的人，有着天然的吸引力。\\n99：魅力和酷的巅峰(超级名模或世界影星)，人类极限。\\n注意：外貌通常只有人类使用，且超过99无意义。',
      '教育': '教育 EDU\\n0：新生儿。\\n15：任何方面都没有受过教育。\\n60：高中毕业。\\n70：大学毕业(本科学位)。\\n80：研究生毕业(硕士学位)。\\n90：博士学位，教授。\\n96：某研究领域的世界级权威。\\n99：人类极限。',
      '智力': '智力 INT / 灵感 idea\\n0：没有智商，无法理解周遭的世界。\\n15：学得很慢，只能理解最常用的数字，或阅读学前教育级别的书。\\n50：普通人水平。\\n90：超凡之脑，可以理解多门语言或定理。\\n99：天才(爱因斯坦、达芬奇、特斯拉等等)，人类极限。\\n140：超越人类之智(例如远古者)。\\n210+：怪物之智，可以理解并操作多重次元(例如伟大的克苏鲁)。',
      '意志': '意志 POW\\n0：弱者的心，没有意志力，没有魔法潜能。\\n15：意志力弱，经常成为高智力或高意志人士的人偶或玩物。\\n50：普通人水平。\\n90：坚强的心，对沟通不可视之物和魔法有着高潜质。\\n100：钢铁之心，与灵能领域和不可视世界有着强烈的链接。\\n140：超越人类，基本上是异界存在(例如伊格)。\\n210+：怪物的魔法潜质和力量，超越凡人之理解力(例如伟大之克苏鲁)。\\n注意：人类的意志可以超过100，但那是极端特例。',
      '幸运': '幸运 LUCK\\n决定了调查员的命运，常用于修改掷骰结果。'
    };
    const ADJUSTMENT_DESCRIPTIONS = '年龄对属性的调整表：\\n15-19岁：STR和SIZ合计减5点，EDU减５点，决定幸运值时可以骰2次取更高值。\\n20-39岁：对EDU进行１次增强检定。\\n40-49岁：对EDU进行２次增强检定，STR和CON和DEX合计减5点，APP减5点。\\n50-59岁：对EDU进行３次增强检定，STR和CON和DEX合计减10点，APP减10点。\\n60-69岁：对EDU进行４次增强检定，STR和CON和DEX合计减20点，APP减15点。\\n70-79岁：对EDU进行４次增强检定，STR和CON和DEX合计减40点，APP减20点。\\n80-89岁：对EDU进行４次增强检定，STR和CON和DEX合计减80点，APP减25点。\\n进行EDU增强检定时，掷骰D100，若结果大于你当前的EDU，则EDU增加1D10点（不能高于99）';

    const JOB_DATA = [
      { name: '会计师', desc: '会计师可能在企业工作或作为自由会计师，为个体经营者和企业客户担任顾问。他们是优秀的研究者，既勤奋又关注细节，能够通过仔细分析个人和企业交易记录、财务报表和其他记录支援其他调查员。', credit: [30, 70], Skills: ['会计', '法律', '图书馆使用', '聆听', '说服', '侦查'], SkillPoint: '教育*4', SkillExt: '任选两项个人或时代特长作为本职技能' },
      { name: '杂技演员', desc: '杂技演员可能是参加各级比赛（甚至奥运会）的业余运动员，也可能是专业的演员，在马戏团、嘉年华、歌舞团之类的地方作为娱乐业从业者工作。', credit: [9, 20], Skills: ['攀爬', '闪避', '跳跃', '投掷', '侦查', '游泳'], SkillPoint: '教育*2+敏捷*2', SkillExt: '任选两项个人或时代特长作为本职技能' },
      { name: '演员-戏剧演员', desc: '一般指舞台剧演员和电影演员。许多演员有相当深厚的文化素养，认为自己才是“正统”的，倾向于轻视电影业的商业活动。直到 20 世纪后期电影业的地位提高，电影演员的薪酬增加，这种情况才发生改变。电影业和电影明星一直是世界人民关注的焦点。许多明星一夜成名，在媒体的聚光灯下过着光鲜亮丽的生活。在 1920 年代，虽然全国都有大型剧院，美国的戏剧中心仍然是纽约城。英国的情况与之相近，戏剧的中心在伦敦，其他的剧团则在各郡作巡回演出。巡回剧团乘火车旅行，演出内容既包括新编剧目，也包括莎士比亚和其他人的传统剧目。有些剧团也会花时间去国外采风，通常是去加拿大、夏威夷、澳大利亚和欧洲大陆。20 年代后期出现了有声电影，不少默片时代的明星难以适应有声电影的冲击，挥舞手臂的夸张扮演从此让位给了细致入微的角色特写。这段时间前期的明星包括约翰·加菲尔德和弗兰西斯·布什曼，后期则是贾莱·库珀和琼·克劳馥。', credit: [9, 40], Skills: ['艺术与手艺(表演)', '乔装', '格斗', '历史'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '演员-电影演员', desc: '一般指舞台剧演员和电影演员。许多演员有相当深厚的文化素养，认为自己才是“正统”的，倾向于轻视电影业的商业活动。直到 20 世纪后期电影业的地位提高，电影演员的薪酬增加，这种情况才发生改变。电影业和电影明星一直是世界人民关注的焦点。许多明星一夜成名，在媒体的聚光灯下过着光鲜亮丽的生活。在 1920 年代，虽然全国都有大型剧院，美国的戏剧中心仍然是纽约城。英国的情况与之相近，戏剧的中心在伦敦，其他的剧团则在各郡作巡回演出。巡回剧团乘火车旅行，演出内容既包括新编剧目，也包括莎士比亚和其他人的传统剧目。有些剧团也会花时间去国外采风，通常是去加拿大、夏威夷、澳大利亚和欧洲大陆。20 年代后期出现了有声电影，不少默片时代的明星难以适应有声电影的冲击，挥舞手臂的夸张扮演从此让位给了细致入微的角色特写。这段时间前期的明星包括约翰·加菲尔德和弗兰西斯·布什曼，后期则是贾莱·库珀和琼·克劳馥。', credit: [20, 90], Skills: ['艺术与手艺(表演)', '乔装', '汽车驾驶', '心理学'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '事务所侦探', desc: '世界上有许多著名的侦探机构，其中最著名的是平克顿和伯恩斯调查局（后来合并成一家公司）。这样的公司一般有两类工作人员：安保人员和调查人员。', credit: [20, 45], Skills: ['格斗(斗殴)', '射击', '法律', '图书馆使用', '心理学', '潜行', '追踪'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '精神病医生(古典)', desc: '1920 年代，“精神病医生”这个词专用来称呼治疗精神失常的医生（也就是早期的精神科医生）。精神分析在当时的美国鲜为人知，而且它的基本内容都是性生活和如厕训练之类令大众不齿的东西。精神病学，一种正规的从行为主义发展来的医学理论则要普及得多。精神病医生、精神科医生和神经科医生还经常爆发激烈的论战。', credit: [10, 60], Skills: ['法律', '聆听', '医学', '外语', '精神分析', '心理学', '科学(生物学、化学)'], SkillPoint: '教育*4' },
      { name: '动物训练师', desc: '动物训练师可能在电影工作室、巡回马戏团、马厩工作或自由工作。不管是训练导盲犬、狮子钻火圈，他们工作时基本要独自一人长时间近距离地照看这些动物。动物训练师可以像对人一样对动物使用「心理学」技能。', credit: [10, 40], Skills: ['跳跃', '聆听', '博物学', '动物驯养', '科学(动物学)', '潜行', '追踪'], SkillPoint: '教育*2+Max(外貌,意志)*2', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '文物学家(原作向)', desc: '文物学家也许是调查员可以从事的最具有洛夫克拉夫特风格的职业：那些历久弥新的的卓越作品、湮没在古代传说中的神奇力量，总能使他们乐在其中。独立的收入使文物学家能够研究古旧晦涩的文物，或者根据自己的兴趣爱好集中探寻特别的种类。他们通常有着欣赏的眼光、敏锐的头脑，和讽刺无知、自大、贪婪者的愚蠢时尖酸刻薄的幽默。', credit: [30, 70], Skills: ['估价', '艺术与手艺', '历史', '图书馆使用', '外语', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '古董商', desc: '古董商通常自己开店，从自己所在的地方转卖物品，或继续扩展业务范围，通过倒卖物品到城市商店赚取利润。', credit: [30, 50], Skills: ['会计', '估价', '汽车驾驶', '历史', '图书馆使用', '导航'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '考古学家(原作向)', desc: '考古学家研究探索历史的痕迹。主要来说，是对人类历史相关的物质资料进行各种鉴识、检查、分析。这项工作包含辛苦细致的研究，更不必提情愿亲自下斗铲土的决心。\\n在 1920 年代，成功的考古学家会被当成著名冒险家与探险家，名利双收。有人运用科学方法考古，不过更多的人对付老祖宗的秘密时喜好暴力破解的办法，甚至祭出炸药，这种碉堡了的办法现代人可是很难看得惯的。', credit: [10, 40], Skills: ['估价', '考古学', '历史', '外语', '图书馆使用', '侦查', '机械维修', '导航', '科学(任一)'], SkillPoint: '教育*4' },
      { name: '建筑师', desc: '建筑师掌握设计和营造建筑的知识，不论是个人房屋的改造还是造价数百万美元的地标工程。建筑师与项目经理紧密合作，负责监督施工全程。建筑师必须了解当地的规划法律，健康和安全法规，和基础的公众安全原则。\\n他们既可以在大公司工作，也可以自由工作，这在很大程度上取决于信誉。在 1920年代，许多人尝试在自家或小办公室单干。不过他们苦心创造的宏伟设计很少能卖得出去。', credit: [30, 70], Skills: ['会计', '艺术与手艺(技术制图)', '法律', '母语', '说服', '心理学', '科学(数学)'], SkillPoint: '教育*4', SkillExt: '计算机使用/图书馆使用选择其中一项作为本职技能' },
      { name: '艺术家', desc: '艺术家在这里可以是画家，雕塑家等等。他们有时沉浸于自己虚幻的想象当中，有时又沐浴在激发热情和理解的灵感之下。不论是否天资优秀，艺术家的内心必须足够强大，这样才能战胜生涯起步时的障碍和挑剔的眼光，并且在自己小有名气以后继续努力。有些艺术家对物质生活是否丰富并不在乎，而有些则有着强烈的创业倾向。', credit: [9, 50], Skills: ['艺术与手艺', '外语', '心理学', '侦查'], SkillPoint: '教育*2+Max(敏捷,意志)*2', SkillExt: '历史/博物学选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '精神病院看护', desc: '尽管少数富有的人会选择私人疗养院，大多精神病患者最终会被安置到州县设置的定点医院。这些地方除了医生护士以外，还会有一支看护队伍。选聘看护的时候，力量和体格往往比医学知识更被看重。', credit: [8, 20], Skills: ['闪避', '格斗(斗殴)', '急救', '聆听', '心理学', '潜行'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '运动员', desc: '运动员可能效力于职业的棒球、足球、板球或者篮球队伍。这支队伍也许是大联盟队伍，有着稳定工资，参加的比赛万人瞩目；或者是众多小联盟队伍之一，尤其是在1920 年代的棒球界。这些队伍往往寄于大联盟队伍篱下，各方面都受其管理，工资更是刚够运动员糊口又不至于让他们跳槽的水平。\\n成功的运动员在自己的专业领域会拥有相当的声誉——现今尤其如此，在世界各地都能看到体育明星和电影明星并肩站在红地毯上的场景。', credit: [9, 70], Skills: ['攀爬', '跳跃', '格斗(斗殴)', '骑术', '游泳', '投掷'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '作家(原作向)', desc: '作家不同于记者，他们用文字定义和探讨人们的社会生活，尤其是人们的情感变化。他们的劳动通常孤立而又自我中心：虽然以前写作是个能稳拿工资的行当，但如今只靠写作发大财的人屈指可数。\\n作家的工作习惯相差极大。通常作家们会花费数月乃至数年的时间调查取材，为新书的创作做准备；然后闭门谢客，投入紧张的创作。', credit: [9, 30], Skills: ['艺术与手艺(写作)', '历史', '图书馆使用', '外语', '母语', '心理学'], SkillPoint: '教育*4', SkillExt: '博物学/神秘学选择其中一项、任选一项个人或时代特长作为本职技能' },
      { name: '酒保', desc: '酒保虽然不一定是酒吧的掌柜，却一定是所有客人的朋友。对客人们的好声气，一部分来说是出于他们的职业或者业务，而更多的来说则是达到目的的一种手段。\\n1920 年代，由于禁酒令的存在，酒保变成了非法的职业；但是遍地开花的黑酒吧又不能没有酒保，结果就是酒保仍然不愁找不到活干。', credit: [8, 25], Skills: ['会计', '格斗(斗殴)', '聆听', '心理学', '侦查'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '猎人', desc: '猎人是优秀的追踪者和狩猎者，通常靠为富裕的客户捕猎为生。绝大多数猎人会对地球上某一个部分的情况烂熟于心，比如加拿大森林、非洲草原等等。有些人可能从事盗猎活动，例如为私人收藏家捕捉珍稀动物，或者贩卖受保护的动物和违反道德的动物制品，如兽皮、象牙之类——虽然 1920 年代大多数国家这些活动都不算违法。\\n尽管“王牌猎人”是最典型的类型，不过在加拿大育空的深山老林里打驼鹿和熊为生的土著人也可以算是猎人。', credit: [20, 50], Skills: ['射击', '博物学', '导航', '科学(生物学,植物学)', '潜行', '追踪'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '外语/生存选择其中一项、聆听/侦查选择其中一项作为本职技能' },
      { name: '书商', desc: '书商可能拥有自己的店面或者利基（小众）邮购服务，也可能辗转全国甚至海外专门经销书籍。许多人拥有富有的，能提供利润丰厚又稀罕的工作的固定客户。', credit: [20, 40], Skills: ['会计', '估价', '汽车驾驶', '历史', '图书馆使用', '母语', '外语'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '赏金猎人', desc: '赏金猎人捉拿罪犯并将他们交给正义去审判。最常见的情况是受保释人的委托去缉捕逃狱者。赏金猎人们为了自己的猎物可以不择手段，几乎不会考虑其他人的正当权益之类细枝末节的东西。\\n非法闯入、威胁、肢体暴力，都是赏金猎人屡试不爽的秘技。现在这些秘技还包括了电话窃听、黑客操作和其他的秘密监控。', credit: [9, 30], Skills: ['汽车驾驶', '法律', '心理学', '追踪', '潜行'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '电子学/电气维修选择其中一项、格斗/射击选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '拳击手、摔跤手', desc: '拳击手和摔跤手各分为职业和业余两种。\\n职业拳击手和职业摔角手的活动由外部利益支持的赞助人安排，并有合同约束。他们还要进行全日制的工作和训练。\\n业余拳击的竞赛种类非常丰富，同时它也是那些想成为职业拳手的人的训练场。不过也有业余和准职业的选手靠参加黑市拳击赛谋生，举办这些比赛的通常是本地的黑社会或者是从中渔利的庄家。', credit: [9, 60], Skills: ['闪避', '格斗(斗殴)', '恐吓', '跳跃', '心理学', '侦查'], SkillPoint: '教育*2+力量*2', SkillExt: '任选两项个人或时代特长作为本职技能' },
      { name: '管家、男仆、女仆', desc: '管家、男仆、女仆都属于作为仆人被雇佣的服务业人员。\\n管家通常为一个大家庭打理家事。传统上，他负责的范围包括厨房、酒窖和储藏室，在所有仆人中位置最高。一般男管家还要负责管理其他的男仆（女管家反之）。更多的职责则听候主人差遣。\\n男仆和女仆则为主人提供贴身服务，包括管理主人的服装、准备浴室和担任私人助理。助理的工作则包括安排旅行日程、整理日记、家庭理财等。\\n(信用范围取决于雇主家的社会地位和信用等级)', credit: [9, 40], Skills: ['艺术与手艺', '急救', '聆听', '外语', '心理学', '侦查'], SkillPoint: '教育*4', SkillExt: '会计/估价选择其中一项作为本职技能' },
      { name: '神职人员', desc: '神职人员通常担任一个教区的牧师，或是经过分配外出传教，尤其是去国外（见传教士）。不同的教会工作的侧重点和组织结构各不相同，如天主教会的牧师可能上升到主教、大主教和红衣主教，而一个卫理公会的牧师则会升职到教区主管和主教。\\n许多神职人员都接受忏悔（不仅仅是天主教）。虽然不能透露忏悔的内容，但是要怎样利用它们就全凭他们自己了。\\n有些教职人员在教堂接受医生、律师、学者的专业培训。这样的调查员应该选择最符合自己工作的职业模板。', credit: [9, 60], Skills: ['会计', '历史', '图书馆使用', '聆听', '外语', '心理学'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '计算机程序员、工程师(现代)', desc: '计算机程序员通常是设计、编写、测试、调试和维护计算机程序源代码的职业。他们精通从形式逻辑到系统平台（程序运行环境）的各种知识，可能是自由工作者，也可能供职于软件开发部门。\\n计算机技术人员负责计算机系统和网络的开发和维护工作，经常与其他人员（如项目经理）合作来保证系统的完整稳定和正常提供所需功能。类似的职业还包括数据库管理员、系统管理员、网络管理员、多媒体开发人员、软件工程师、网络管理员等。\\n计算机黑客则利用计算机和计算机网络为手段，进行干扰或破坏以达成政治目的（有时被称为“政治黑客”）或获取非法利益。达成目标的手段主要是非法入侵计算机和其他用户帐户，目的则可能包括篡改网页、人肉搜索、盗取身份信息、垃圾邮件炸弹、拒绝服务攻击等等。', credit: [10, 70], Skills: ['计算机使用', '电气维修', '电子学', '图书馆使用', '科学(数学)', '侦查'], SkillPoint: '教育*4', SkillExt: '任选两项个人或时代特长作为本职技能' },
      { name: '黑客(现代)', desc: '计算机程序员通常是设计、编写、测试、调试和维护计算机程序源代码的职业。他们精通从形式逻辑到系统平台（程序运行环境）的各种知识，可能是自由工作者，也可能供职于软件开发部门。\\n计算机技术人员负责计算机系统和网络的开发和维护工作，经常与其他人员（如项目经理）合作来保证系统的完整稳定和正常提供所需功能。类似的职业还包括数据库管理员、系统管理员、网络管理员、多媒体开发人员、软件工程师、网络管理员等。\\n计算机黑客则利用计算机和计算机网络为手段，进行干扰或破坏以达成政治目的（有时被称为“政治黑客”）或获取非法利益。达成目标的手段主要是非法入侵计算机和其他用户帐户，目的则可能包括篡改网页、人肉搜索、盗取身份信息、垃圾邮件炸弹、拒绝服务攻击等等。', credit: [10, 70], Skills: ['计算机使用', '电气维修', '电子学', '图书馆使用', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '牛仔', desc: '牛仔在西部的牧区和牧场工作。有些人拥有自己的牧场，更多的则是在各处打工为生。想赚大钱的牛仔会去冒着丢胳膊少腿乃至送命的危险参加牛仔巡回赛，通过旅行获取名誉。\\n在 1920 年代，一些牛仔能在好莱坞找到西部片替身演员和群众演员的工作，例如怀特·厄普就曾为西部电影担任顾问。在现代，有些牧场也对想要体验一把牛仔生活的游客开放。', credit: [9, 20], Skills: ['闪避', '跳跃', '骑术', '生存', '投掷', '追踪'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '格斗/射击选择其中一项、急救/博物学选择其中一项作为本职技能' },
      { name: '工匠', desc: '工匠也可能被人叫做师傅或大师，是擅长对各种材料进行手工加工的人。通常都是才能出众的人，有的凭借自己的艺术作品出名，有的则会服务于自己的社区。\\n可能的行当包括：家具、珠宝、钟表、陶艺、锻造、纺织、书法、裁缝、木匠、书籍装裱、玩具制造、彩色玻璃吹制等等。', credit: [10, 40], Skills: ['会计', '机械维修', '博物学', '侦查'], SkillPoint: '教育*2+敏捷*2', SkillExt: '艺术与手艺选择其中两项、任选两项个人或时代特长作为本职技能' },
      { name: '罪犯-刺客', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [30, 60], Skills: ['乔装', '电气维修', '格斗', '射击', '锁匠', '机械维修', '潜行', '心理学'], SkillPoint: '教育*2+Max(敏捷,力量)*2' },
      { name: '罪犯-银行劫匪', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [5, 75], Skills: ['汽车驾驶', '格斗', '射击', '恐吓', '锁匠', '操作重型机械'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '电气维修/机械维修选择其中一项、任选一项个人或时代特长作为本职技能' },
      { name: '罪犯-打手、暴徒', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [5, 30], Skills: ['汽车驾驶', '格斗', '射击', '心理学', '潜行', '侦查'], SkillPoint: '教育*2+力量*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '罪犯-窃贼', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [5, 40], Skills: ['估价', '攀爬', '聆听', '锁匠', '妙手', '潜行', '侦查'], SkillPoint: '教育*2+敏捷*2', SkillExt: '电气维修/机械维修选择其中一项作为本职技能' },
      { name: '罪犯-欺诈师', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [10, 65], Skills: ['估价', '艺术与手艺(表演)', '聆听', '心理学', '妙手'], SkillPoint: '教育*2+外貌*2', SkillExt: '法律/外语选择其中一项、任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '罪犯-独行罪犯', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [5, 65], Skills: ['估价', '潜行', '心理学', '侦查'], SkillPoint: '教育*2+Max(敏捷,外貌)*2', SkillExt: '艺术与手艺(表演)/乔装选择其中一项、格斗/射击选择其中一项、锁匠/机械维修选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '罪犯-女飞贼(古典)', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [10, 80], Skills: ['艺术与手艺', '汽车驾驶', '聆听', '潜行'], SkillPoint: '教育*2+外貌*2', SkillExt: '格斗(斗殴)/射击(手枪)选择其中一项、锁匠/机械维修选择其中一项、任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '罪犯-赃物贩子', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。\\n女飞贼是名为专业大盗的女人。大部分都是独立行动，也有对自己的男伴言听计从的时候。不过这也不一定，实际上情况可能完全相反，她完全可以在干了某一票以后就卷走所有现金和皮草溜之大吉。', credit: [20, 40], Skills: ['会计', '估价', '艺术与手艺(伪造)', '历史', '图书馆使用', '侦查'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '罪犯-赝造者、伪造者', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [20, 60], Skills: ['会计', '估价', '艺术与手艺(伪造)', '历史', '图书馆使用', '侦查', '妙手'], SkillPoint: '教育*4', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '罪犯-走私者', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [20, 60], Skills: ['射击', '聆听', '导航', '心理学', '妙手', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '汽车驾驶/驾驶(飞行器或船)选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '罪犯-混混', desc: '罪犯的体格和相貌形形色色，有些是纯粹碰运气伺机行事，比如扒手和暴徒；有些则组成分工明确，会详细调查并制定计划的犯罪组织。后者包括银行劫匪、飞贼、赝造者和诈骗者。\\n罪犯可能为别人工作，后者通常是“匪帮”或罪犯家族；也可能单打独斗，如果成功的报酬值得去费力冒险，才会和别人搭伙。自由犯罪者则往往被称为抢劫犯、响马贼和江洋大盗。\\n打手、暴徒都是犯罪组织的兵卒。他们被犯罪组织豢养，不过团伙上层出事的时候，倒霉的往往是他们这些喽啰。对于他们来说，嘴紧和忠心属于职业道德。\\n欺诈师通常都是油嘴滑舌的人物。他们或单独或集体出没在富裕的人家和社区周边，诈取他们来之不易的钱财。许多骗局复杂精妙，诈骗团伙会倾巢出动乃至租用建筑；有些则不需要这么麻烦，只要一个骗子几分钟就能搞定。\\n赃物贩子，顾名思义是买卖偷抢来的财产，通常是收购赃物并转手卖给其他罪犯或（无意中）守法的顾客。主要来说，他们是小偷和买家的中间人，有时也会从交易中收取提成；不过更常见的还是以极低的价格直接收购赃物。\\n赝造者是地下世界的艺术家，专门从事伪造官方文件、契约、转让书，并提供伪造的签名。初学者只能做做小贼的假身份证，而顶级的赝造者连印假币的铸模都能做。杀手是地下世界的冷血夺命者。这是一项严谨的活计，他们从外地受雇杀人，接近目标，果断下手，又迅速离开。杀手通常很难融入社会，因为很多杀手行为总是很刻板，其他人很容易以为他们不近人情。但是另一方面，他们也会结婚生子，在其他方面和普通人没有什么不同。\\n走私一直是一个有利可图的高风险行当。走私者往往有一个合法的表面职业，比如船长、飞行员或商人，以掩盖他们非法运输的行为。\\n街头混混一般都是些小年轻，弄不好还在寻觅加入真正黑帮的契机。不过他们的本事也就限于偷车，盗窃商店货物，抢钱或者夜盗。', credit: [3, 10], Skills: ['攀爬', '格斗', '射击', '跳跃', '妙手', '潜行', '投掷'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '教团首领', desc: '美国的新兴宗教层出不穷。直到现在，也还有从新英格兰超验主义到“天父的儿女”等等许多种类。教团首领有的创立了严格的教条并且对信徒推行，另一些则仅仅是垂涎于信徒的金钱和权势。\\n在 1920 年代，各种诱惑性的新兴宗教团体纷纷涌现。有些采取基督教的形式，有些则混杂了东方的神秘主义和神秘学的仪式。美国西海岸的人对这些教团屡见不鲜，不过其他形式的教团全国各地都存在。在美国南部的“圣经带”，就有许多巡回帐篷演出圣歌、舞蹈，推行信仰复兴。其他国家也是一样，只要有需要信仰的人，就会有新兴宗教团体。', credit: [30, 60], Skills: ['会计', '神秘学', '心理学', '侦查'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '除魅师(现代)', desc: '除魅师的工作是说服（或者强迫）一个人放弃自己的信仰或是对宗教团体、社会团体的忠心。他们一般受雇于深陷教团之类组织的人的亲属，任务就是解救对方（通常靠绑架），并通过心理学手段使他们割断与原来教团的联系（“控制”）。\\n也有不那么激烈的除魅师，他们的工作对象则是那些自愿离开教团的人，为他们完全地退出教团进行有效的指导。', credit: [20, 50], Skills: ['汽车驾驶', '历史', '神秘学', '心理学', '潜行'], SkillPoint: '教育*4', SkillExt: '格斗(斗殴)/射击选择其中一项、任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能\\n经过 KP 允许，可以包含「催眠」技能' },
      { name: '设计师', desc: '设计师的工作包括许多方面，从时装到家具或是其他任何东西。他们自由工作，为设计工作室和企业设计产品、流程、法律、游戏、图像等等。\\n调查员特定的设计方向也会影响他们对专业技能的选择，如果需要的话要进行调整。', credit: [20, 60], Skills: ['会计', '艺术与手艺(摄影)', '艺术与手艺', '机械维修', '心理学', '侦查'], SkillPoint: '教育*4', SkillExt: '计算机使用/图书馆使用选择其中一项、任选一项个人或时代特长作为本职技能' },
      { name: '业余艺术爱好者(原作向)', desc: '业余艺术爱好者靠经济自立、遗产继承、信托基金或者其他各种来源保障自己的生活开支，没有必要自己工作。如果经济条件足够好，他们甚至可以雇佣专业的经济顾问来打理自己的产业。\\n他们可能有很高的学历，但不一定是真才实学；优越的经济条件使得他们性情古怪，口无遮拦。\\n在 1920 年代，这些人可能会被时人称为“摩登女郎”或者“公子哥儿”，当然想当一个社交“名流”其实并不要求他有多有钱。换作现代，“时髦”则是恰如其分的形容词。\\n业余艺术爱好者有着大把的时间考虑如何变得潇洒世故，不过花这些时间去做别的事可是违背他们的天性和兴致。', credit: [50, 99], Skills: ['射击', '外语', '骑术', '艺术与手艺'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、三项个人或时代特长作为本职技能' },
      { name: '潜水员', desc: '潜水员可能在军队、执法机构或海绵采集、海上救援、环境保护甚至水下寻宝的民间机构工作。', credit: [9, 30], Skills: ['潜水', '急救', '机械维修', '驾驶(船)', '科学(生物)', '侦查', '游泳'], SkillPoint: '教育*2+敏捷*2' },
      { name: '医生(原作向)', desc: '医生这里可能是指全科医生、外科医生、其他专科医生或者独立医学研究员。除去个人的目标以外，救死扶伤、获得财富和荣誉、提升公众的理性意识和科学素养也常常是医生的理想。\\n农村和小城镇的卫生院是全科医生的舞台，而大城市的各大医院则是高手如云，集聚了众多专攻病理学、毒理学、整形外科、脑外科等领域的专家。有些医生也可能担任全职或兼职的法医，进行尸检，并为市、县、州级执法机构出具检验报告。\\n在美国，行医资格由各州认证，大多要求最少两年的正规医学院校学习经历。不过这个规定还是比较晚近的，在 1920 年代很多年长的医生尽管没受过任何正规专业教育，仍然可以获得医师执照。', credit: [30, 80], Skills: ['急救', '医学', '外语(拉丁语)', '心理学', '科学(生物学、药学)'], SkillPoint: '教育*4', SkillExt: '任选两项学术专长作为本职技能' },
      { name: '流浪者', desc: '相对于那些因贫困而苦恼的人，流浪者选择四处漂泊的生活，可能是出于社会、哲学、经济的原因，或只是渴望摆脱社会的约束。\\n流浪汉需要工作，有时几天或几个月，但他们应对问题时往往选择流动和孤立，而不是舒适和亲近。在美国，这种情况尤其常见，只要旅行本身没有什么危险，就会有人选择漂泊为生。', credit: [0, 5], Skills: ['攀爬', '跳跃', '聆听', '导航', '潜行'], SkillPoint: '教育*2+Max(外貌,敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '司机-私人司机', desc: '专职司机可能为企业、个人工作，也可能拥有自己的出租车或货车。\\n出租车司机可能属于大大小小的出租车公司，也可能靠自己的车和证件运营（在美国，需要出租车牌照）。出租车公司负责为出租车司机登记车辆并分配调度，方便司机自由揽客。出租车上必须统一安装计价器，并由出租车协会进行定期检查。通常司机还要通过警方的背景调查，获得特殊的驾驶许可证。\\n私人司机则是直接受雇于个人或企业，或者是专门提供连人带车的私人司机业务的中介机构。', credit: [10, 40], Skills: ['汽车驾驶', '聆听', '机械维修', '导航', '侦查'], SkillPoint: '教育*2+敏捷*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '司机-司机', desc: '专职司机可能为企业、个人工作，也可能拥有自己的出租车或货车。\\n出租车司机可能属于大大小小的出租车公司，也可能靠自己的车和证件运营（在美国，需要出租车牌照）。出租车公司负责为出租车司机登记车辆并分配调度，方便司机自由揽客。出租车上必须统一安装计价器，并由出租车协会进行定期检查。通常司机还要通过警方的背景调查，获得特殊的驾驶许可证。\\n私人司机则是直接受雇于个人或企业，或者是专门提供连人带车的私人司机业务的中介机构。', credit: [9, 20], Skills: ['会计', '汽车驾驶', '电气维修', '话术', '机械维修', '导航', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '司机-出租车司机', desc: '专职司机可能为企业、个人工作，也可能拥有自己的出租车或货车。\\n出租车司机可能属于大大小小的出租车公司，也可能靠自己的车和证件运营（在美国，需要出租车牌照）。出租车公司负责为出租车司机登记车辆并分配调度，方便司机自由揽客。出租车上必须统一安装计价器，并由出租车协会进行定期检查。通常司机还要通过警方的背景调查，获得特殊的驾驶许可证。\\n私人司机则是直接受雇于个人或企业，或者是专门提供连人带车的私人司机业务的中介机构。', credit: [9, 30], Skills: ['会计', '汽车驾驶', '电气维修', '话术', '机械维修', '导航', '侦查'], SkillPoint: '教育*2+敏捷*2', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '编辑', desc: '编辑的工作包括审核记者的稿件，撰写报刊社论，应对各种突发事件、到了截稿时间要催稿，编辑工作只好偶尔为之啦。大型报社的编辑数量众多，包括比起新闻编辑更多参与业务运营的主编。其他编辑专门负责时尚、体育或者其他板块。许多小报可能就只有一个编辑，他甚至有可能就是报社的业主或者唯一的全职员工。', credit: [10, 30], Skills: ['会计', '历史', '母语', '心理学', '侦查'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '政府官员', desc: '以民选方式选举出来的政府官员享有与他们的职位相符的声望。小城市的市长和城镇的镇长之类，他们的影响力基本出不了城镇的范围，而且这样的职务基本上是兼职的，报酬也很少。大城市的市长，工资就相当可观了，而且还能把自己的城市管理得像小王国一样，影响力和权力比所在州的州长还要大。\\n州议会的众参两院议员是相当有面子的职位，尤其是在商界和本州的其他业界。州长负责全州的事务，是联系各州和国家的纽带。\\n联邦政府拥有最高等级的影响力。众议院议员由各州按本州人口所占比重选派的共400 余名议员组成，任期为两年。参议院则是不论各州大小，每州选派两名议员到花生屯任职。任期长达六年，人数不超过一百，所以参议员更是权倾一方，许多年长的议员能够享受总统级的待遇。\\n在英国，下议院议员由选举产生，任期四到五年；上议院议员则不由选举产生，是世袭制或由君主指任。', credit: [50, 90], Skills: ['取悦', '历史', '恐吓', '话术', '聆听', '母语', '说服', '心理学'], SkillPoint: '教育*2+外貌*2' },
      { name: '工程师', desc: '工程师精通机械和电气设备，可能在民间或军工企业工作，也可能是个发明家。他们擅长应用科学、数学知识和丰富的创造思维，解决各种技术问题。', credit: [30, 60], Skills: ['艺术与手艺(技术制图)', '电气维修', '图书馆使用', '机械维修', '操作重型机械', '科学(工程学、物理学)'], SkillPoint: '教育*4', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '艺人', desc: '艺人包括小丑、歌手、舞蹈演员、喜剧演员、杂耍艺人、魔术师，各种以在人前表演谋生的人。他们乐于向更多的人表现自己的能力，并期待观众回报的掌声。\\n在 1920 年代，这一职业并不受人尊重。不过 1920 年代好莱坞明星的高薪彻底改变了很多人的想法，现在这个职业背景已经通常被视作是优势了。', credit: [9, 70], Skills: ['艺术与手艺', '乔装', '聆听', '心理学'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '探险家(古典)', desc: '在 20 世纪早期，这世界还有许多地区尚未有人涉足，而探索这些地方正是探险家的工作。这种令人兴奋不已的生活方式，其经济来源则是科学界的赞助、私人的捐赠、博物馆的委托和报纸杂志图书电影的版权等等。\\n黑非洲的大部分仍然不为人知，同样的地方还包括了南美的马托格罗索高原，澳大利亚的大沙沙漠，撒哈拉和阿拉伯沙漠，和亚洲的茫茫戈壁。尽管南北极点已经被探险家征服了，但周围很大部分的地区仍然是未知的。', credit: [55, 80], Skills: ['射击', '历史', '跳跃', '博物学', '导航', '外语', '生存'], SkillPoint: '教育*2+Max(外貌,敏捷,力量)*2', SkillExt: '攀爬/游泳选择其中一项作为本职技能' },
      { name: '农民', desc: '农民可能自己拥有土地，自己从事农牧业，也可能是受雇在农场工作。农业劳动繁重而枯燥，特别适合那些喜欢户外体力劳动的人。\\n1920 年代是美国城镇人口超过农村人口的首个十年。从这时起一直到现在，自耕农民都在受到规模化农业企业和剧烈波动的农产品市场的双重冲击。', credit: [9, 30], Skills: ['艺术与手艺(耕作)', '汽车驾驶', '机械维修', '博物学', '操作重型机械', '追踪'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '联邦探员', desc: '联邦执法机构和特工种类各异。有些身着制服，比如美国司法部的人员；另外一些则穿便服，工作内容也类似警探，比如联邦调查局的人员。', credit: [20, 40], Skills: ['汽车驾驶', '格斗(斗殴)', '射击', '法律', '说服', '潜行', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '消防员', desc: '消防员是公职人员，通常为所管辖的社区服务。他们夜以继日地工作，或者连续几天的倒班工作，吃住包括娱乐活动都要局限在消防局里。消防员的管理结构类似军队，职位包括中尉、上尉和局长等等。', credit: [9, 30], Skills: ['攀爬', '闪避', '汽车驾驶', '急救', '跳跃', '机械维修', '操作重型机械', '投掷'], SkillPoint: '教育*2+Max(敏捷,力量)*2' },
      { name: '驻外记者', desc: '驻外记者是新闻界的精英人才。他们拿着固定工资，靠报销单环游全世界。在 1920年代，驻外记者通常供职于大型报社、广播电台、或者国家级通讯社。当代的驻外记者也可能自由撰稿或者为电视台、网络通讯社和国际新闻通讯社工作。\\n这个职业的工作内容五花八门，经常能激动人心。不过自然灾害、政治动荡和战争也会成为驻外记者报道的主要内容，工作也不总是一帆风顺。', credit: [10, 40], Skills: ['历史', '外语', '母语', '聆听'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '法医', desc: '法医是一个高度专门化的职业，大多数法医为市、县或州执法机构工作。工作内容包括尸体解剖，推定死因，并为公诉人提供建议。法医也常常会在刑事审判中出庭提供证言。', credit: [40, 60], Skills: ['外语(拉丁语)', '图书馆使用', '医学', '说服', '科学(生物学、司法科学、药学)', '侦查'], SkillPoint: '教育*4' },
      { name: '赌徒', desc: '赌徒是罪犯世界里最花哨的一群人。他们衣着光鲜，不论朴实还是华丽都魅力四射。不论是靠赛马、纸牌游戏还是其他赌博方式，他们总是要凭自己的运气过活。\\n老练的赌徒会频繁地光顾犯罪组织开设的地下赌场。少数赌场高手可能经常参加漫长而又一掷千金的豪赌，甚至可能有外部利益集团作为后台。\\n低级的赌徒则出入于狭窄的小巷，在骰子房耍弄灌铅的骰子，或者是挤坐在阴暗的台球室里。', credit: [8, 50], Skills: ['会计', '艺术与手艺(表演)', '聆听', '心理学', '妙手', '侦查'], SkillPoint: '教育*2+Max(外貌,敏捷)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '黑帮-黑帮老大', desc: '黑帮可能是整个城市、一部分城市的大佬，也可能只是给这些大佬打工的马仔。马仔们通常有自己的保护范围，比如监管非法运输和收取保护费等等。老板总管业务，负责交易，并要就各种各样的问题给马仔们拿主意。更重要的是，老板可以各种高人一等，只要能找到马仔或者小弟去干的事，他基本是不肯污了自己的手去做的。\\n黑社会在 1920 年代上升为突出的社会问题。本来仅限于在本地收收保护费和管管赌场的外国裔黑帮，不约而同地发现了贩卖私酒带来的巨大利润。没过多久，他们就掌控了城市的大片区域，并在街上和其他黑帮火并。虽然大部分黑帮是按来源的民族划分——如爱尔兰裔、意大利裔、非洲裔和犹太裔，黑帮的成员仍然可能是任何民族。如今，贩毒则取代其他，成为多数黑帮中来钱最快的犯罪门路。和 1920 年代前辈的工作方法类似，现在的黑帮老大也需要大量的小弟来负责保卫、推广、在街道里推行自己的业务。\\n除去贩私酒和贩毒以外，卖淫、保护、赌博、腐败等等都是这些犯罪组织的业务范围。', credit: [60, 95], Skills: ['格斗', '射击', '法律', '聆听', '心理学', '侦查'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '黑帮-马仔', desc: '黑帮可能是整个城市、一部分城市的大佬，也可能只是给这些大佬打工的马仔。马仔们通常有自己的保护范围，比如监管非法运输和收取保护费等等。老板总管业务，负责交易，并要就各种各样的问题给马仔们拿主意。更重要的是，老板可以各种高人一等，只要能找到马仔或者小弟去干的事，他基本是不肯污了自己的手去做的。\\n黑社会在 1920 年代上升为突出的社会问题。本来仅限于在本地收收保护费和管管赌场的外国裔黑帮，不约而同地发现了贩卖私酒带来的巨大利润。没过多久，他们就掌控了城市的大片区域，并在街上和其他黑帮火并。虽然大部分黑帮是按来源的民族划分——如爱尔兰裔、意大利裔、非洲裔和犹太裔，黑帮的成员仍然可能是任何民族。如今，贩毒则取代其他，成为多数黑帮中来钱最快的犯罪门路。和 1920 年代前辈的工作方法类似，现在的黑帮老大也需要大量的小弟来负责保卫、推广、在街道里推行自己的业务。\\n除去贩私酒和贩毒以外，卖淫、保护、赌博、腐败等等都是这些犯罪组织的业务范围。', credit: [9, 20], Skills: ['汽车驾驶', '格斗', '射击', '心理学'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '绅士、淑女', desc: '绅士淑女指的是有良好的教养品行、举止彬彬有礼的人。通常用来称呼上流社会（通过继承或津贴）拥有相当财富的人。\\n在上世纪 20 年代，这样的人至少要有一个仆人（管家、男仆、女仆、私人司机），还要有城市或乡村的宅第。家庭的富有并不重要，因为家庭的社会地位往往比财产更被上流社会所看重。', credit: [40, 90], Skills: ['艺术与手艺', '射击(步霰)', '历史', '外语', '导航', '骑术'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '游民', desc: '游民只有少数的人愿意去当，虽然失业的人、醉倒在阴沟里的醉鬼到处都是。和流浪者只会在必需时才工作不同，游民的工作本身就是流浪。\\n他们不断地坐火车旅行，从一个城市辗转到另一个城市，他们是身无分文的诗人、漂泊者，铁路上的探索者、冒险者和盗贼。但是铁路上的生活一样充满危险。且不说穷困潦倒、无家可归，还要面对来自警察、周围居民和铁路员工的敌意。另外在深夜中跳车并不是一件容易的事，在跳车的时候被车厢夹断过手脚的人可是不可胜数。', credit: [0, 5], Skills: ['艺术与手艺', '攀爬', '跳跃', '聆听', '导航', '潜行'], SkillPoint: '教育*2+Max(外貌,敏捷)*2', SkillExt: '锁匠/妙手选择其中一项、任选一项个人或时代特长作为本职技能' },
      { name: '勤杂护工', desc: '勤杂护工在医院的工作包括倒垃圾、打扫房间、运送病人，还有一些其他乱七八糟的工作。总之对他们的要求不比对看门人多多少。', credit: [6, 15], Skills: ['电气维修', '格斗(斗殴)', '急救', '聆听', '机械维修', '心理学', '潜行'], SkillPoint: '教育*2+力量*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '记者（原作向）-调查记者', desc: '记者用文字对当天的新闻事件进行报导与评论，一天之内就要完成一个作家一周的工作量。他们通常为报纸、杂志、广播电台、电视台或者新闻网站撰稿。\\n优秀的调查记者在报道事件的同时，即使面对丑恶，也能保持自身的清廉正直。恶心的记者则被现实所压倒，最终丧失自己的节操，肆意操纵文字歪曲真相。通讯记者则是新闻传媒行业大军中的一员，不管是自由撰稿或是在报社、杂志社、新闻网站、通讯社工作。大部分记者从事实地工作，包括走访见证人、查看记录、收集叙述。有些记者被安排专门追踪警界、体育界或商界的热点新闻，其他人则是负责社会事件乃至园艺俱乐部之类的事情。\\n通讯记者都会携带记者证，不过记者证除了各通讯社（主要是报社）用来识别自己的雇员以外没有太大的作用。实际上记者的工作内容更像私家侦探，有时为了获得第一手消息也难免使点嘴上花招。', credit: [9, 30], Skills: ['艺术与手艺(摄影)', '历史', '图书馆使用', '母语', '心理学'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '记者（原作向）-通讯记者', desc: '记者用文字对当天的新闻事件进行报导与评论，一天之内就要完成一个作家一周的工作量。他们通常为报纸、杂志、广播电台、电视台或者新闻网站撰稿。\\n优秀的调查记者在报道事件的同时，即使面对丑恶，也能保持自身的清廉正直。恶心的记者则被现实所压倒，最终丧失自己的节操，肆意操纵文字歪曲真相。通讯记者则是新闻传媒行业大军中的一员，不管是自由撰稿或是在报社、杂志社、新闻网站、通讯社工作。大部分记者从事实地工作，包括走访见证人、查看记录、收集叙述。有些记者被安排专门追踪警界、体育界或商界的热点新闻，其他人则是负责社会事件乃至园艺俱乐部之类的事情。\\n通讯记者都会携带记者证，不过记者证除了各通讯社（主要是报社）用来识别自己的雇员以外没有太大的作用。实际上记者的工作内容更像私家侦探，有时为了获得第一手消息也难免使点嘴上花招。', credit: [9, 30], Skills: ['艺术与手艺(表演)', '历史', '聆听', '母语', '心理学', '潜行', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '法官', desc: '法官是主持审判全过程的人，可能单独工作或是和同事组成合议庭。一般是推选或任命制，工作年限也分定期和终身。有的人是初出茅庐就当了法官，而其余的绝大多数，不论是在联邦最高法院还是遥远西部小镇的法官，其实至少都是经过注册的律师。', credit: [50, 80], Skills: ['历史', '恐吓', '法律', '图书馆使用', '聆听', '母语', '说服', '心理学'], SkillPoint: '教育*4' },
      { name: '实验室助理', desc: '实验室助理在科研环境中工作，在首席科学家的监督下进行实验和行政工作。研究内容可能依首席科学家的研究学科而变化。但基本都包括取样、测试、记录和分析数据、调整和进行实验、制备标本和样品、管理实验室的日常工作，和保护工作人员的健康与安全。', credit: [10, 30], Skills: ['电气维修', '外语', '科学(化学)', '科学', '科学', '侦查'], SkillPoint: '教育*4', SkillExt: '计算机使用/图书馆使用选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '工人-非熟练工人', desc: '工人这一大类职业包括工厂工人、纺织工人、码头工人、养路工人、矿工、建筑工人等等。工人分为两种类型：熟练工和非熟练工。普通的工人虽然技术不熟练，但是仍然长于使用电动工具、起重机和其他工厂设备。', credit: [9, 30], Skills: ['汽车驾驶', '电气维修', '格斗', '急救', '机械维修', '操作重型机械', '投掷'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '工人-伐木工', desc: '工人这一大类职业包括工厂工人、纺织工人、码头工人、养路工人、矿工、建筑工人等等。工人分为两种类型：熟练工和非熟练工。普通的工人虽然技术不熟练，但是仍然长于使用电动工具、起重机和其他工厂设备。', credit: [9, 30], Skills: ['攀爬', '闪避', '格斗(链锯)', '急救', '跳跃', '机械维修', '投掷'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '博物学/科学(生物学或植物学)选择其中一项作为本职技能' },
      { name: '工人-矿工', desc: '工人这一大类职业包括工厂工人、纺织工人、码头工人、养路工人、矿工、建筑工人等等。工人分为两种类型：熟练工和非熟练工。普通的工人虽然技术不熟练，但是仍然长于使用电动工具、起重机和其他工厂设备。', credit: [9, 30], Skills: ['攀爬', '科学(地质学)', '跳跃', '机械维修', '操作重型机械', '潜行', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '律师', desc: '律师或法律顾问精通他们所在地区的法律，擅长把抽象的法学理论知识联系起来，为客户解决法律方面的疑难，担任辩护代理、法律顾问的工作，为客户提供解决办法。可能受托处理个人案件、接受法院指定，也可能专门为某个富裕客户或公司服务。\\n在美国，“律师”一词一般只指辩护律师。在英国，“律师”一词则包括高级律师、初级律师还有一些执法机构。\\n假如碰上好客户的话，律师自己也可以一战成名，少数律师还能以自己在政治经济方面的获益吸引媒体的关注。', credit: [30, 80], Skills: ['会计', '法律', '图书馆使用', '心理学'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '图书馆管理员(原作向)', desc: '图书馆管理员在公共机构和图书馆工作，负责管理图书目录和书库，并处理图书借阅等。在现代，图书馆管理员还要负责管理视听资料、电子书库。\\n一些大公司可能聘用图书馆管理员管理书库，偶尔还会有富有的图书藏家招收他们管理自己的私人藏书。', credit: [9, 35], Skills: ['会计', '图书馆使用', '外语', '母语'], SkillPoint: '教育*4', SkillExt: '任选四项个人或时代特长/学术专长作为本职技能' },
      { name: '技师', desc: '技师包括所有需要专业训练和作为学徒或实习生工作经验的职业，例如木匠、石匠、管道工、电工、设备安装工人、机修工人等等这些需要技术资质的职业。通常这些工人有自己的工会组织，会和承包人和雇主争取自己的权益。', credit: [9, 40], Skills: ['艺术与手艺(木匠、焊接、管道工)', '攀爬', '汽车驾驶', '电气维修', '机械维修', '操作重型机械'], SkillPoint: '教育*4', SkillExt: '任选两项个人或时代特长作为本职技能' },
      { name: '军官', desc: '军官有严格的等级，许多等级还需要高等教育学历。各国武装部队都建立了人才培养系统，其中包括大学教育。在美国，许多大学开设军校生训练项目，可以让学员同时接受文化教育和军事训练。毕业的学员可以授以陆军或海军少尉军衔，并分派到各驻地。他们通常会为国家服役四年，之后可以退役复员。许多人有专门的任命，作为医生、律师和工程师工作。\\n寻求军旅生涯的人会为进入西点军校和美国海军军官学校这样的著名军校而努力，拥有这些名校学历很容易得到其他军官的尊敬。离开学校以后，许多军官也会选择接受飞行训练等特殊训练。\\n富有经验，特别值得提升的士兵会被破例提拔为一级准尉。虽然在名义上位列最末，获得这一军衔所需要的时间和经验意味着他们远比普通的初中级军官更受尊敬。绝大多数军衔是终身荣誉，退役多年的军官仍然可以自称上尉或者将军。', credit: [20, 70], Skills: ['会计', '射击', '导航', '急救', '心理学'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '传教士', desc: '传教士云游到世界的各个角落，传播神的旨意，在文明的地方拯救“不幸的原始人”和“迷途的灵魂”。他们可能属于天主教、新教、伊斯兰教或者其他信仰系统，比如后期圣徒教会（摩门教）在欧美就有专门的传道所。\\n有的传教士只凭自己的意志独立行动，有的则可能有教会以外的组织支持。\\n基督教、伊斯兰教的传教者，佛教、印度教的法师，在全世界各个时代都能遇到。', credit: [0, 30], Skills: ['艺术与手艺', '急救', '机械维修', '医学', '博物学'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '登山家', desc: '登山家一般都是利用业余时间和假期的运动员，只有少数攀登著名高山的人才会去寻找财力和设备的赞助。\\n19 世纪登山运动开始兴起，到了 1920 年代，所有美洲和阿尔卑斯地区的主要山峰都被一一征服。经过与西藏人的冗长谈判之后，外国登山队终于获准进入喜马拉雅山的高峰地区。作为世界上最后未被征服的高峰，对珠峰的进军经常被电台和报纸报道。不过 1921、1922、1924 年的三次远征都没能达到峰顶，还造成了 13 人死亡。\\n到了现代，登山可以是休闲运动或职业选择。如果是后者，则工作内容包括教练、向导、运动员或救生员等。', credit: [30, 60], Skills: ['攀爬', '急救', '跳跃', '聆听', '导航', '外语', '生存', '追踪'], SkillPoint: '教育*2+Max(敏捷,力量)*2' },
      { name: '博物馆管理员', desc: '博物馆管理员可能负责大学或其他公共机构的大型设施，也可能负责小一些的博物馆，往往对本地的地质或者其他的内容颇有研究。', credit: [10, 30], Skills: ['会计', '估价', '考古学', '历史', '图书馆使用', '神秘学', '外语', '侦查'], SkillPoint: '教育*4' },
      { name: '音乐家', desc: '音乐家可能加入乐团、乐队或者独奏，演奏的乐器则可以是任何你能想象的种类。音乐家想出人头地十分困难，签约发布唱片就更难了。所以绝大多数音乐家都贫穷又无人关注，只靠街头卖艺勉强维持生计。少数幸运儿可以找到固定工作，比如在酒吧、宾馆或者市交响乐团弹钢琴。对更少的人来说，在正确的时间出现在正确的地点，再加上一点点天赋，就能获得巨大的成功和可观的财富。\\n1920 年代是爵士乐的年代，众多的音乐家在美国各地的大中城市、城镇里的爵士乐队和交响乐队工作。少数音乐家住在芝加哥和纽约之类的大城市并在那里打拼，而大部分的人靠巴士、汽车或者火车过着旅行生活。', credit: [9, 30], Skills: ['艺术与手艺', '聆听', '心理学'], SkillPoint: '教育*2+Max(意志,敏捷)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、四项个人或时代特长作为本职技能' },
      { name: '护士', desc: '护士是专业的医疗助理，通常在医院和疗养院之类的地方工作，或者和全科医生一起合作。一般来说，护士会协助健康人或病人进行保健或康复活动（或者临终关怀），虽然其他人若是有足够的力量、意志或者知识，完全不需要护士帮助的康复也是可能的。', credit: [9, 30], Skills: ['急救', '聆听', '医学', '心理学', '科学(生物学、化学)', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '神秘学家', desc: '神秘学家是钻研深奥秘密和神秘魔法的人。他们对超自然能力深信不疑，并竭尽所能靠他们的能力去了解这些东西。许多人对不同神秘哲学和魔法理论的知识面都相当广泛，有些甚至相信自己专注研究三十年真的成为了魔法师——到底是真是假就交由 KP来决断了。\\n需要指出的是，神秘学家熟知的基本上是“表面的魔法”——克苏鲁神话魔法的秘密对他们仍然是未知的，或者不过是古书上描述那些诱人的线索而已。', credit: [9, 65], Skills: ['人类学', '历史', '图书馆使用', '神秘学', '外语', '科学(天文学)'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能\\n 经过 KP 允许，可以包含「克苏鲁神话」技能(建议一开始限制在 10% 以内)' },
      { name: '旅行家', desc: '旅行家爱好户外，他们一年中大部分时间都呆在户外，并且一出门就是相当长的时间；通常有相当的捕鱼和狩猎技术，能在最恶劣的环境之中幸存下来。擅长的技术可能包括登山、捕鱼、滑雪、皮划艇、攀登和露营。\\n旅行家可能在国家公园或素质拓展中心做野外向导和护林员，也可能是有其他经济来源能让他们不用工作就能以这种方式生活，说不定还可能是一个隐士，只有在需要的时候才会回到文明社会。', credit: [5, 20], Skills: ['射击', '急救', '聆听', '博物学', '导航', '侦查', '生存', '追踪'], SkillPoint: '教育*2+Max(敏捷,力量)*2' },
      { name: '超心理学家', desc: '超心理学家从不打算欣赏超常现象。相反，他们试图去观察，记录并研究这些实例。被叫做“捉鬼人”的他们利用技术手段来获取某人或某地点的超自然活动的证据，当然比起收集到实在的证据，更多的时候他们是在揭穿假冒和误认的超常现象。\\n一些超心理学家专门研究特定的现象，例如超感官、心灵致动、闹鬼等等。\\n名牌大学是没有超心理学学位的。这个领域成就的评判标准完全是基于个人声誉，所以一般有相近学科学历比如物理学、心理学和医学的人会比较有说服力。\\n选择研究这个的人往往对不可视的神秘力量抱有相当的同情态度，并希望其他的科学家也能满意地点头肯定。这就表现出了一种既相信又怀疑的奇异的叠加态——恐怕超心理学家自己也难解决这个问题。一个对观察实验证明不感兴趣的人是个神秘学家而不是个科学家。', credit: [9, 30], Skills: ['人类学', '艺术与手艺(摄影)', '历史', '图书馆使用', '神秘学', '外语', '心理学'], SkillPoint: '教育*4', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '药剂师', desc: '药剂师的管理一直以来都比医生更严格。所有的药剂师都要在各州注册，注册的条件则是高中毕业并至少在药学院学习三年。他们可能在医院或者药房工作，也可能自己开药房。', credit: [35, 75], Skills: ['会计', '急救', '外语(拉丁语)', '图书馆使用', '心理学', '科学(药学、化学)'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '摄影师-摄影师', desc: '摄影师大部分是自由工作者，可能制作广告电影或者在照相馆做肖像拍摄。其他一些摄像师则在报纸、电视和电影产业工作。\\n摄影作为一种艺术形式已经产生相当长的时间了，精英的摄影师可以从艺术、新闻报道、野生动物保护等多种角度出发创作他们的作品。不管是哪种立意，他们都能获得名誉和报酬。\\n摄影记者本质上就是拿照相机，为拍摄的照片写配文的记者。在 1920 年代，新闻短片走上历史舞台。笨重的 35mm 摄像装备走遍全球各地，搜寻有价值的新闻轶事、体育赛事和泳装选美比赛。新闻片制作人员一般分为三类：一类是画面中的记者，另两个人则负责摄像和灯光等等。新闻中的声音则是在新闻稿完成以后在录音棚中录入完成的。', credit: [9, 30], Skills: ['艺术与手艺(摄影)', '心理学', '科学(化学)', '潜行', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '摄影师-摄影记者', desc: '摄影师大部分是自由工作者，可能制作广告电影或者在照相馆做肖像拍摄。其他一些摄像师则在报纸、电视和电影产业工作。\\n摄影作为一种艺术形式已经产生相当长的时间了，精英的摄影师可以从艺术、新闻报道、野生动物保护等多种角度出发创作他们的作品。不管是哪种立意，他们都能获得名誉和报酬。\\n摄影记者本质上就是拿照相机，为拍摄的照片写配文的记者。在 1920 年代，新闻短片走上历史舞台。笨重的 35mm 摄像装备走遍全球各地，搜寻有价值的新闻轶事、体育赛事和泳装选美比赛。新闻片制作人员一般分为三类：一类是画面中的记者，另两个人则负责摄像和灯光等等。新闻中的声音则是在新闻稿完成以后在录音棚中录入完成的。', credit: [10, 30], Skills: ['艺术与手艺(摄影)', '攀爬', '外语', '心理学', '科学(化学)'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '飞行员-飞行员', desc: '飞行员可以在美国邮政这样的企业工作，也可以在大大小小的民航公司做飞行人员。美国 1926 年之前没有对飞行员的职业要求，1926 年航空商业法案通过之后才要求有执照。这个时代的多数飞行员从事嘉年华表演、特技飞行表演、乘飞机游玩或是小机场的空中的士等服务。\\n也有飞行员在部队服现役。许多特技飞行员是在服役期间学会的驾驶飞机，有时仍然会被军队委派任务。', credit: [20, 70], Skills: ['电气维修', '机械维修', '导航', '操作重型机械', '驾驶(飞行器)', '科学(天文学)'], SkillPoint: '教育*2+敏捷*2', SkillExt: '任选两项个人或时代特长作为本职技能' },
      { name: '飞行员-特技飞行员(古典)', desc: '特技飞行员在嘉年华工作或者为大胆的消费者进行休闲飞行服务。参加有组织的飞行表演赛，不论是固定路线还是越野赛，往往都可以增加自己的知名度。在 1920 年代，好莱坞常常使用特技飞行员，飞机制造商也会录用一些飞行员为新机作测试。许多特技飞行员是在一次大战中掌握的飞行技术，所以许多人仍然在陆海空军或海岸警卫队服役；年轻的飞行员则基本上是在和平时期接受的训练或是自学成才。\\n参加过一战的王牌飞行员“现在”还活跃在公众视野中的包括：埃迪·里肯巴克，现在在克莱斯勒公司工作；汤米·希区柯克，“现在”是马球赛场的明星；里德·兰迪斯，美国职棒大联盟执行长凯纳索·蒙顿·兰迪斯的儿子。', credit: [30, 60], Skills: ['会计', '电气维修', '聆听', '机械维修', '导航', '驾驶(飞行器)', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项个人或时代特长作为本职技能' },
      { name: '警探', desc: '便衣警察的工作是检查犯罪现场、收集证据、询问证人以解决凶杀、盗窃等重大案件。他们在现场办案中往往与穿制服的巡警密切合作。\\n警探可能指挥他的下属进行详尽的调查，但是很难有机会集中精力对付单独一个事件，在美国他们很可能要同时处理数十乃至上百的案件。警探工作最关键的部分是通过梳理证词、重建现场情况，摒弃伪证，从而收集足够逮捕嫌疑人的证据，进而促成成功的刑事审判。警探和检察官的职责是分开的，这样可以保证证据在审判之前被独立地对待。\\n尽管现在警探通常会参加警察学校课程并获得学位、参加特殊训练或公务员培训，他们最主要的经验还是来源于担任基层警官或者普通巡警时的工作经历。巡警则属于市、城镇、县治安部门或州、地区的警察机关。他们工作时可能步行、驾驶巡逻车，或者干脆坐办公室。', credit: [20, 50], Skills: ['射击', '法律', '聆听', '心理学', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '艺术与手艺(表演)/乔装选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '巡警', desc: '便衣警察的工作是检查犯罪现场、收集证据、询问证人以解决凶杀、盗窃等重大案件。他们在现场办案中往往与穿制服的巡警密切合作。\\n警探可能指挥他的下属进行详尽的调查，但是很难有机会集中精力对付单独一个事件，在美国他们很可能要同时处理数十乃至上百的案件。警探工作最关键的部分是通过梳理证词、重建现场情况，摒弃伪证，从而收集足够逮捕嫌疑人的证据，进而促成成功的刑事审判。警探和检察官的职责是分开的，这样可以保证证据在审判之前被独立地对待。\\n尽管现在警探通常会参加警察学校课程并获得学位、参加特殊训练或公务员培训，他们最主要的经验还是来源于担任基层警官或者普通巡警时的工作经历。巡警则属于市、城镇、县治安部门或州、地区的警察机关。他们工作时可能步行、驾驶巡逻车，或者干脆坐办公室。', credit: [9, 30], Skills: ['格斗(斗殴)', '射击', '急救', '法律', '心理学', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '汽车驾驶/骑术选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '私家侦探', desc: '私家侦探通常在警察不出手的地方活跃，包括收集证据为客户准备民事诉讼，追查跑路的配偶或生意伙伴，或者代理刑事案件的私人辩护。他们和任何专业人员一样，私家侦探从不顾及自己的私人情感，只要付钱，不管是有罪还是无罪的一方的委托他们都乐得接受。\\n私家侦探过去可能在警察队伍里工作，利用以前的业务关系为现在工作谋求优势；然而事实并非总是如此。在许多地方私家侦探必须持证上岗，假如被发现有违法行为，就会撤销执照——侦探生涯也就到此为止。', credit: [9, 30], Skills: ['艺术与手艺(摄影)', '乔装', '法律', '图书馆使用', '心理学', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '教授(原作向)', desc: '教授是受聘于高等院校的学者。大公司也可能聘请他们以开展学术研究与产品开发。独立的学者也靠开办业余课程作为经济来源。\\n最重要的一点，这一职业代表了 PhD（博士）的荣誉称号，意味着可以在世界各地的大学任终身教职。教授的专长是教学和专业研究，往往在自己的专业领域内有着可圈可点的学术成就。', credit: [20, 70], Skills: ['图书馆使用', '外语', '母语', '心理学'], SkillPoint: '教育*4', SkillExt: '任选四项个人或时代特长/学术专长作为本职技能' },
      { name: '淘金客', desc: '淘金客一直是美国西部的特色，即便在加利福尼亚淘金热和内华达康姆斯塔克发现金银矿的日子早已经过去的现在。他们无休止地在山间漫游，寻找能使自己一夜暴富的矿脉。而且现在发现石油和发现金子一样给力', credit: [0, 10], Skills: ['攀爬', '急救', '历史', '机械维修', '导航', '科学(地质学)', '侦查'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项个人或时代特长作为本职技能' },  // 修正：去掉末尾的“长”
      { name: '性工作者', desc: '性工作者根据场合、背景和教养，从超级值钱的应召小姐到牛郎再到站街女都有可能。往往入这一行都是权宜之计，许多人梦想有朝一日能够脱身。少数人能够自己接客，不过绝大多数人基本都是被只认钱不认人的老鸨和皮条客逼迫着工作。', credit: [5, 50], Skills: ['艺术与手艺', '闪避', '心理学', '妙手', '潜行'], SkillPoint: '教育*2+外貌*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '精神病学家', desc: '精神病学家是在现代专门从事精神失常诊断和治疗的医生。精神病学家掌握着精神药理学的治疗方法，使用精神类药物的资质，还能整理脑电图并对其进行计算机分析。\\n在十九二十世纪之交，精神分析理论刚刚产生，试图解释一些现在认为实际上是生物学范畴的现象。所以，精神分析学家们努力寻求获得自己的医疗证书。与此同时，各种不同的精神失常诊断与治疗理论开始起步。到 1930 年代，任何一个医生都可以以精神病学家的身份进入美国医学协会名录中了。', credit: [30, 80], Skills: ['外语', '聆听', '医学', '说服', '精神分析', '心理学', '科学(生物学、化学)'], SkillPoint: '教育*4' },
      { name: '心理学家、心理分析学家', desc: '心理学家虽然也经常被叫做心理治疗师和心理咨询师，不过这些工作都只是心理学的分支。其他的还有为企业和政府提供顾问的组织管理心理学家，进行研究并在学校教授心理学的学术型心理学家等等。\\n临床心理学家可能实际接触病人，并且运用各种可能的心理治疗方法。注意心理学家和专业的精神病学家的区别，后者本质上还是医生。\\n在 1920 年代，对人类行为的研究还是一个新兴的领域，主要的理论还是弗洛伊德心理学。', credit: [10, 40], Skills: ['会计', '图书馆使用', '聆听', '说服', '精神分析', '心理学'], SkillPoint: '教育*4', SkillExt: '任选两项个人或时代特长/学术专长作为本职技能' },
      { name: '研究员', desc: '学术界的研究课题不计其数，尤其是在天文学、物理学和其他理论领域。私人或企业也雇用了成千上万的研究员，重点在化学、制药和工程领域。石油公司则会聘用专业的地质学家，不一而足。研究员大部分的时间都在室内工作和写作，不过有的则会经常外出考察。\\n考察研究员通常经验丰富，思想独立又足智多谋，可能受雇于私人或者为大学进行学术研究。\\n石油公司会派出地质学家探索潜在的油田，人类学家则是调查地球被人遗忘角落的原始部落，考古学家则竭数年之力挖掘沙漠丛林之中的宝藏，还要和工人与地方政府打交道。', credit: [9, 30], Skills: ['历史', '图书馆使用', '外语', '侦查'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)、三项学术专长作为本职技能' },
      { name: '海员-军舰海员', desc: '海员包括军舰海员和商船海员。\\n新入伍的海员像陆军的同行一样，开始时需要接受基本的训练。这之后他们获得军衔，并被分配到各镇守府。虽然很多海员担任像水手长副手和司炉工（管理引擎）这样的传统角色，但是也有很多经过专门训练的机械师、无线电操作员、通风管理员之类。海员们最高的军衔是士官长，达到了这个军衔连高级将领都要礼让三分。在美国，海员通常要服四年的现役和后两年的后备役，即在国家发布动员令时有应召服役的义务。民用船海员可能在渔船、客船，或运输原油或商品的运输船上工作。在美国，客船活跃在东西海岸和五大湖，运送渔民和游客。目前佛罗里达州在墨西哥湾和大西洋海岸拥有最多数量的客船。\\n在禁酒令期间，许多客船船长发现把急切想喝酒的顾客运到 3 海里外，外国船只允许卖酒的地方是一桩赚钱的买卖。当然走私酒也报酬丰厚，但是危险就高多了。', credit: [9, 30], Skills: ['格斗', '射击', '急救', '导航', '驾驶(船)', '生存(海洋)', '游泳'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '电工/机械维修选择其中一项作为本职技能' },  // 修正：去掉多余的“长”
      { name: '海员-民用船海员', desc: '海员包括军舰海员和商船海员。\\n新入伍的海员像陆军的同行一样，开始时需要接受基本的训练。这之后他们获得军衔，并被分配到各镇守府。虽然很多海员担任像水手长副手和司炉工（管理引擎）这样的传统角色，但是也有很多经过专门训练的机械师、无线电操作员、通风管理员之类。海员们最高的军衔是士官长，达到了这个军衔连高级将领都要礼让三分。在美国，海员通常要服四年的现役和后两年的后备役，即在国家发布动员令时有应召服役的义务。民用船海员可能在渔船、客船，或运输原油或商品的运输船上工作。在美国，客船活跃在东西海岸和五大湖，运送渔民和游客。目前佛罗里达州在墨西哥湾和大西洋海岸拥有最多数量的客船。\\n在禁酒令期间，许多客船船长发现把急切想喝酒的顾客运到 3 海里外，外国船只允许卖酒的地方是一桩赚钱的买卖。当然走私酒也报酬丰厚，但是危险就高多了。', credit: [20, 40], Skills: ['急救', '机械维修', '博物学', '导航', '驾驶(船)', '侦查', '游泳'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '推销员', desc: '推销员是商务工作的必需一环，他们的工作就是推广和销售公司的产品或服务。大部分推销员的时间要用来旅行、开会、和客户应酬（在报销限额之内）。有些则主要坐办公室用电话联系潜在客户，还有的会在各社区巡回，挨家挨户上门推销。\\n1920 年代是企业家的年代，旅行推销成了一种日常生活方式。这些人有些直接现货交易，有些通过托销交易，当然不管黑猫白猫，拿到订单的才是好猫，推销员要用强烈的销售策略才能说得客户，至于价钱就不是他们考虑的范围了。有些推销员在固定的地区工作，有些则可以自由漫游，寻找任何地方可能出现的商机。如果是上门推销，那商品可能就是刷子、吸尘器或者百科全书之类的各种物件了。', credit: [9, 40], Skills: ['会计', '汽车驾驶', '聆听', '心理学'], SkillPoint: '教育*2+外貌*2', SkillExt: '潜行/妙手选择其中一项、任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '科学家', desc: '科学家是在追求知识的过程中挖掘真理的人。如果想要利用科学知识制造有用的物品，需要的是工程师；而如果想要扩展“可能”这个概念的范围，那就是科学家的工作了。\\n科学家们通常在企业和大学工作，以进行他们的研究。\\n虽然主攻一个科学领域，但是真正称职的科学家一般也能达到通晓其他数个科学领域的水平。他们对自己的母语也能使用自如，学历也相当高，甚至拥有博士学位。', credit: [9, 50], Skills: ['外语', '母语', '侦查'], SkillPoint: '教育*4', SkillExt: '计算机使用/图书馆使用选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)、三项学术专长作为本职技能' },
      { name: '秘书', desc: '秘书的范围是从高薪的私人管理助理到普通的打字员。这份工作的重点在于以自己各种沟通协调能力，支持主管和经理人员。\\n因为身处企业流程的中心，许多秘书比老板对企业的内部运作和经营还要熟悉。\\n在 1920 年代，秘书工作主要是通信工作，例如听写打印信件，整理文档系统，并为老板安排会议时间。有的情况下，秘书还会负责老板的生活，比如安排假期、为老板和家人置办礼物，还有保护老板的安全。', credit: [9, 30], Skills: ['会计', '艺术与手艺(打字、速记)', '母语', '心理学'], SkillPoint: '教育*2+Max(敏捷,外貌)*2', SkillExt: '计算机使用/图书馆使用选择其中一项、任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '店老板', desc: '店老板经营小店、市场摊位或者是小饭馆。这种店往往都是小本自营，不过也有为其他东家照顾生意的。不少店是家族式管理，工作人员大部分都有亲属关系，其他的雇员即便有也很少。\\n在 1920 年代，还有不少的老板娘开起了自己的理发店和帽店。', credit: [20, 40], Skills: ['会计', '电气维修', '聆听', '机械维修', '心理学', '侦查'], SkillPoint: '教育*2+Max(敏捷,外貌)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '士兵、海军陆战队士兵', desc: '士兵指的是从列兵到士官长（美国军衔制）的统称。尽管名义上比起最新的次级少尉还低，即便高级军官也往往对他们给予尊重。在美国，标准的服役期限是六年，包括四年现役和两年后备役。\\n所有应征人员首先要在“训练营（新兵连）”接受基本训练，在训练营中新兵将学习如何行军、射击和敬礼。结束训练营的训练后，大部分的新兵会分配到步兵营，虽然也有分配到炮兵营和坦克营的。少部分会接受非战斗的训练，例如通风系统、机械装备、文职甚至军官接待。海军陆战队名义上属于海军，但是和陆军士兵在背景、训练方式和技能方面都很相近。', credit: [9, 30], Skills: ['闪避', '格斗', '射击', '潜行', '生存'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '急救/机械维修/外语选择其中两项、攀爬/游泳选择其中一项作为本职技能' },
      { name: '间谍', desc: '间谍为国家和组织的情报部门卖命。他们能以从大使到厨房清洁工的任何职业身份作为掩饰，刺探他们所需的情报。有些间谍数年如一日的持续着卧底工作，另一些穿个马甲就换一个身份。在本国委任的间谍通常会去往外国工作。\\n间谍除了情报收集和反情报收集的主要工作，也会被委派其他任务，例如招募新间谍和国家批准的暗杀等。', credit: [20, 60], Skills: ['射击', '聆听', '外语', '心理学', '妙手', '潜行'], SkillPoint: '教育*2+Max(敏捷,外貌)*2', SkillExt: '艺术与手艺(表演)/乔装选择其中一项、任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '学生、实习生', desc: '学生可能在大学或学院学习，实习生则是正在接受宝贵的入职培训，获得最低报酬的公司员工。', credit: [5, 10], Skills: ['图书馆使用', '聆听'], SkillPoint: '教育*4', SkillExt: '母语/外语选择其中一项、任选两项个人或时代特长、三项学术专长作为本职技能' },
      { name: '替身演员', desc: '替身演员在电影和电视剧工业中活跃，专门模拟坠楼、车祸等灾难场景。他们通常会接受格斗技巧和舞台格斗的训练。任何的替身特技表演都是有风险的，所以健康和安全是这个工作的核心元素。\\n在现代，替身演员基本都是工会的成员，想要加入工会，他们必须有证明自己能力的证书（例如高级驾驶执照，潜水执照等等）。而且，所有的特技场景还要有特技总监负责指导动作。但是在 1920 年代，这些演员组织、行业规范根本就没有成型，所以事故率和死亡率居高不下。', credit: [10, 50], Skills: ['攀爬', '闪避', '格斗', '急救', '跳跃', '游泳', '骑术'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '潜水/汽车驾驶/驾驶(飞行器或船)选择其中两项、电气维修/机械维修选择其中一项作为本职技能' },
      { name: '部落成员', desc: '至少，从家族忠诚来说，部落文化无处不在。在部落中，亲属关系和传统习俗的首要地位是不言而喻的。一个部落通常来说是一个相对较小的群体。相比起法律和个人权利，部落更加依据个人荣耀而裁定行为。崇拜，复仇，嘉奖，以及荣耀——所有的一切都是部落成员个人所有的，而如果领袖或是仇敌被视为有荣耀的人，那么他们个人也必然在某种程度上十分有名。在这样的环境下，放逐是有着实际的效用在的。', credit: [0, 15], Skills: ['攀爬', '聆听', '博物学', '神秘学', '侦查', '游泳', '生存'], SkillPoint: '教育*2+Max(敏捷,力量)*2', SkillExt: '格斗/投掷选择其中一项作为本职技能' },
      { name: '殡葬师', desc: '殡葬师又叫殡葬业者或葬礼主持人，是负责运行丧葬仪式的职业。工作也包含土葬和火化等内容。在葬礼上，殡葬师要进行防腐、裹衣、入殓、遗体美容等等工作。\\n殡葬师的执照由各州发放。他们可能自己拥有殡仪馆，或者在别人的殡仪馆工作。', credit: [20, 40], Skills: ['会计', '汽车驾驶', '历史', '神秘学', '心理学', '科学(生物学、化学)'], SkillPoint: '教育*4', SkillExt: '任选一项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '工会活动家', desc: '工会活动家是组织者、领导者，有时也是空想者或者别有用心的抗议者，通常是工人的伙伴、老板的对头。各行各业都有工会，不论是码头工人、建筑工人、矿工还是演员。\\n在 20 世纪早期，工会活动家所在的工会面临着诸多危险。大企业想要毁掉它，政治家在支持它和谴责它之间摇摆不定，社会主义者和共产主义者试图影响它，还有犯罪组织试图夺取它。', credit: [5, 50], Skills: ['会计', '格斗(斗殴)', '法律', '聆听', '操作重型机械', '心理学'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)作为本职技能' },
      { name: '服务生', desc: '服务生在酒店、酒吧或者其他餐饮业场所服务顾客。通常薪酬很低，不过通过对顾客良好服务，可以得到他们给的小费。\\n在禁酒令时期，售酒场所的服务员是非法职业。不过犯罪组织把控的地下酒吧中仍然存在许多工作机会', credit: [9, 20], Skills: ['会计', '艺术与手艺', '闪避', '聆听', '心理学'], SkillPoint: '教育*2+Max(敏捷,外貌)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、一项个人或时代特长作为本职技能' },
      { name: '白领工人-职员/主管', desc: '白领工人可能是从最低等级的白领职员到中层或高层的管理人员。所属单位则可能从小型中型的本地企业直到大型的国家级甚至跨国公司。\\n职员被扣工资是家常便饭，工作也往往单调乏味。不过如果在工作中展现出了天分，那也会被看上，将来会得到提拔。中高层管理人员的工资比较高，当然责任也比较重，要负责管理公司的日常事务。虽然未婚的白领并不少见，但很多管理人员还是很顾家，家里一般会有配偶和孩子——家庭通常是他们的期望。', credit: [9, 20], Skills: ['会计', '外语', '法律', '聆听'], SkillPoint: '教育*4', SkillExt: '图书馆使用/计算机使用选择其中一项、任选一项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '白领工人-中层、高层管理人员', desc: '白领工人可能是从最低等级的白领职员到中层或高层的管理人员。所属单位则可能从小型中型的本地企业直到大型的国家级甚至跨国公司。\\n职员被扣工资是家常便饭，工作也往往单调乏味。不过如果在工作中展现出了天分，那也会被看上，将来会得到提拔。中高层管理人员的工资比较高，当然责任也比较重，要负责管理公司的日常事务。虽然未婚的白领并不少见，但很多管理人员还是很顾家，家里一般会有配偶和孩子——家庭通常是他们的期望。', credit: [20, 80], Skills: ['会计', '外语', '法律', '心理学'], SkillPoint: '教育*4', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、两项个人或时代特长作为本职技能' },
      { name: '狂热者', desc: '热情而有动力、鄙视安逸的生活，狂热者们为人类更好的生活或者为人类中最精华部分的利益而躁动不安。一些狂热者通过暴力推进他们的信仰，但是并不能说采取和平方式的就比他们好说话，他们每个人都梦想着为自己的理想辩护。', credit: [0, 30], Skills: ['历史', '心理学', '潜行'], SkillPoint: '教育*2+Max(外貌,意志)*2', SkillExt: '任选两项社交技能(取悦/话术/恐吓/说服)、三项个人或时代特长作为本职技能' },
      { name: '饲养员', desc: '饲养员负责动物的喂养和看护，场地管理员和服务员管理其他杂务。通常饲养员会专门照看某一种动物，可以对动物使用「医学」技能。', credit: [9, 40], Skills: ['动物驯养', '会计', '闪避', '急救', '博物学', '医学', '科学(药学、动物学)'], SkillPoint: '教育*4' }
    ];
    JOB_DATA.sort((a, b) => a.name.localeCompare(b.name, 'zh'));
    // 构建职业映射
    const JOB_MAP = Object.fromEntries(JOB_DATA.map(job => [job.name, job]));

    const SKILL_CATEGORIES = {
      '社交': ['信用评级','取悦', '话术', '恐吓', '说服', '心理学', '母语', '外语'],
      '探索': ['估价', '乔装', '潜行', '追踪', '侦查', '聆听', '读唇', '图书馆使用', '生存'],
      '运动': ['攀爬', '跳跃', '骑术', '游泳', '潜水'],
      '技艺': ['艺术与手艺', '妙手', '锁匠', '电气维修', '机械维修', '计算机使用', '导航', '汽车驾驶', '驾驶', '动物驯养', '操作重型机械'],
      '战斗': ['格斗', '射击', '闪避', '投掷', '爆破', '炮术'],
      '医疗': ['急救', '医学', '精神分析', '催眠'],
      '知识': ['会计', '法律', '历史', '考古学', '博物学', '人类学', '神秘学', '电子学', '科学', '克苏鲁神话']
    };
    const SKILL_SUBCATEGORIES = {
      '外语': ['汉语', '英语', '西班牙语', '印地语', '阿拉伯语', '法语', '葡萄牙语', '俄语', '孟加拉语', '德语', '日语', '韩语', '越南语', '泰语', '意大利语', '土耳其语', '波斯语', '波兰语', '荷兰语', '泰米尔语', '马来语', '他加禄语', '希伯来语', '拉丁语', '挪威语', '丹麦语'],
      '生存': ['沙漠', '海洋', '极地'],
      '艺术与手艺': ['表演', '美术', '伪造', '摄影', '打字', '速记', '技术制图', '耕作', '木匠', '焊接', '管道工', '写作', '音乐', '舞蹈', '厨艺', '书法', '裁缝', '理发', '制陶', '雕塑'],
      '驾驶': ['飞行器', '船'],
      '格斗': ['斗殴', '斧', '链锯', '连枷', '绞索', '矛', '剑', '鞭'],
      '射击': ['弓', '手枪', '重武器', '火焰喷射器', '机枪', '步霰', '冲锋枪'],
      '科学': ['天文学', '生物学', '植物学', '化学', '密码学', '工程学', '司法科学', '地质学', '数学', '气象学', '药学', '物理学', '动物学'],
    };
    const SKILL_DESCRIPTIONS = {
      // 社交类技能
      "信用评级": "衡量调查员表现出来的富裕程度以及经济上的自信度。钱是敲门砖；如果调查员尝试用他的经济地位来达成某个目标，那么也许使用信用评级会比较合适。",
      "取悦": "允许通过许多形式来使用，包括肉体魅力诱惑、奉承或是单纯令人感到温暖的人格魅力。取悦可以被用于迫使某人进行特定的行动，但所要求的行为不应与该人的日常举止完全相反。",
      "话术": "特别限定于言语上的哄骗，欺骗以及误导，例如迷惑一名保安来让你进入一间俱乐部，让某人在一张他还没有读的文件上签字，误导警察看向另一边。话术的效果是暂时性的，经过一段时间后，对方会意识到自己被欺骗了。",
      "恐吓": "可以以许多形式使用，包括武力威慑、心理操控、以及威胁。这通常被用来使某人害怕，并迫使其进行某种特定的行为。",
      "说服": "使用说服来通过一场有理有据的论述、争辩以及讨论让目标相信一个确切的想法，概念，或者信仰。说服并不一定需要涉及真实的内容。一场成功的说服将花费不少的时间。",
      "心理学": "一项对所有人来说都很通用的察觉技能，允许使用者研究个人并且形成对于其他某人动机和人格的了解。",
      "母语": "在婴儿期或者童年早期，大多数人使用单一一门语言。所选择作为母语的语言起始值自动地等同于调查员的EDU属性。",
      "外语": "一个人可以了解任何数量的语言，这意味着使用者可以理解、说、读、写一门非母语语言。",

      // 探索类技能
      "估价": "用来估计某种物品的价值，包括质量，使用的材料以及工艺。相关的，技能使用者可以准确地辨认出物品的年龄，评估它的历史关联性以及发现赝品。",
      "乔装": "使用在当调查员想要演出除自己外的别人时。使用者改变了态度，习惯，或声音来进行一个乔装，以另一个人或者另一类人的形象出现。",
      "潜行": "安静地移动或者躲藏的技巧，不惊扰到那些可能在听或看的人们。潜行也意味着调查员可以维持长时间的警觉与冷静来使自己保持静止和隐秘。",
      "追踪": "一名调查员可以凭借追踪技能来通过土壤上的脚印，或是物体通过植被时留下的印记来追踪别人，或者是交通工具以及地球上的动物。",
      "侦查": "这项技能允许使用者发现密门或者秘密隔间，注意到隐藏的闯入者，发现并不明显的线索，发现重新涂过漆的汽车，意识到埋伏，注意到鼓出的口袋，或者任何类似的事情。",
      "聆听": "衡量一名调查员理解声音的能力，包括偶然听到的对话，一扇关着的门后的轻声嘀咕，以及咖啡厅里的私语。",
      "读唇": "允许好奇的调查员听懂一段交谈对话，而不需要听见对方说了什么。能看到对方的视线是必须的。",
      "图书馆使用": "使一名调查员能在图书馆找到一些信息，例如特定的一本书，新闻或者参考书，搜集文件或者查阅资料库，假设需要的东西确实在那里的话。图书馆使用同样适用于网络资料检索。",
      "生存": "提供专业的如何在极端环境下生存的知识和技巧，内容包括狩猎的知识、搭建住所、可能遇到的危险的知识等，取决于所处的地理位置。例如在沙漠中或者极地环境，也包括海洋上或者荒野。",
      "沙漠": "提供在沙漠环境下生存的知识和技巧，包括寻找水源、应对极端温度、辨识危险动植物等。",
      "海洋": "提供在海洋或海面上生存的知识和技巧，包括漂流、捕鱼、海水淡化、应对海洋生物等。",
      "极地": "提供在极寒环境下生存的知识和技巧，包括搭建雪屋、防寒、在冰雪中获取食物和水源等。",

      // 运动类技能
      "攀爬": "允许一名角色借助或者不借助绳索或者登山工具进行爬树、墙以及其他垂直表面。同样包括用绳索下降。",
      "跳跃": "如果成功，调查员可以在垂直方向上跳起或跳下，或者从一个站立点或起步点水平向外跳。当坠落时，跳跃可以被用来降低可能造成的坠落伤害。",
      "骑术": "这项技能被用于给坐在鞍上驾驭马，驴子或者骡子，以及获得对这些骑乘动物、骑乘工具的基础照料知识，以及如何在疾驰中或困难地形上操纵坐骑。",
      "游泳": "有能力在水或者其他液体中漂浮以及移动。只有在遭遇危险的时候需要进行游泳技能检定。",
      "潜水": "使用者接受过在深海游泳的使用以及维持潜水设备的训练，水下导航，合适的下潜配重，以及应对紧急情况的方法。",

      // 技艺类技能
      "艺术与手艺": "进行各种艺术或手工创作，可以用来谋生或表达创意，通常需要工具和时间。一个成功的检定可能可以提供一个物品的相关信息，例如这个物品在何时以及哪里被制造，与之相关的一些历史或者技艺。",
      "表演": "表演者受到过戏剧或电影演技的训练，使你能适应一个人物角色、记住剧本，以及使用舞台、电影化妆来改变他们的外貌。",
      "美术": "艺术家在艺术绘画上十分熟练，同样在用铅笔、彩色蜡笔、粉笔的素描上十分熟练。甚至可以快速素描出准确的印象、物体或人物。",
      "伪造": "熟练于细节，使用者可以制作高质量的伪造文档，使它以某人的笔迹写成，制作官僚作风的形式或许可，或者进行卷册的复制。",
      "摄影": "同时包括静止以及运动摄影。这项技能允许某人拍摄清晰的照片，恰当地修饰照片，并且强化半掩的细节。当进行偷拍或者对细节进行捕捉的时候需要进行摄影技能检定。摄影也允许调查员判断照片的真伪，以及拍摄的角度和位置。",
      "写作": "创作小说、诗歌、剧本等文学作品。",
      "音乐": "演奏乐器、作曲、指挥或音乐理论。",
      "舞蹈": "表演各种舞蹈，包括社交舞、芭蕾、现代舞。",
      "厨艺": "烹饪美食、辨别食材、制作特定菜肴。",
      "书法": "以美观的字体书写、用于正式文书或伪造签名。",
      "裁缝": "从事服装裁剪、缝制、修补等工艺。",
      "理发": "剪发、剃须、造型，可作为乔装的一部分。",
      "制陶": "制作陶器、瓷器。",
      "雕塑": "雕刻石材、木材，塑造粘土等三维艺术品。",
      "妙手": "允许在视觉上隐藏物体，例如利用残骸、衣物或其他制造错觉的物品。妙手包括偷窃、卡牌魔术、以及秘密使用设备。",
      "锁匠": "可以打开车门、短路电线来发动汽车、用铁撬撬开图书馆的窗子、解决中国机关箱、以及穿过常规的商用警报系统。使用者还可以修锁、制作钥匙或是借助其它工具开锁。",
      "电气维修": "使调查员能够修理或者改装电气设备，例如自动点火装置，电动机，保险丝盒，以及防盗自动警铃。",
      "机械维修": "允许调查员修理一个破损的机器或者制造一个新的。基础的木工手艺和管道项目也可以执行，制作物品也同样可以。",
      "计算机使用": "允许调查员用各种不同的电脑语言进行编程；恢复或者分析隐藏的数据；解除被加了保护的系统；探索一个复杂的网络；或者发现别人的骇入、后门程序、病毒。",
      "导航": "允许使用者在早上或者晚上，在暴风雨或者晴朗天气中认清自己的路。有着更高技能的人将对天文表图和工具，以及卫星定位装置十分熟悉。",
      "汽车驾驶": "任何有着这项技能的人都可以驾驶一辆汽车或者轻型卡车，进行常规的移动，并且处理机动车的一般毛病。",
      "驾驶": "相当于水上或者空中的汽车驾驶，这是驾驶飞行器或水上交通工具的技巧。",
      "飞行器": "理解以及足以操作飞行器类的载具。当进行任何的降落时，即使是在最好的环境下，也必须进行一个驾驶检定。",
      "船": "理解在风、暴风雨以及潮流下操纵小型摩托艇以及轮船的机理，并且可以读懂潮流以及风的流向，以此来得到暗礁以及将要逼近的暴风雨的情报。",
      "动物驯养": "命令以及训练已驯化动物去完成一些简单任务的技能。这个技能最常用于狗上，但也包括鸟、猫、猴子以及其他。",
      "操作重型机械": "当驾驶以及操纵一辆坦克，反铲挖掘机，蒸汽挖土机或者其他巨型建造机械时需要这个技能。",

      // 战斗类技能
      "格斗": "指的是一名角色在近距离战斗上的技能。包括空手格斗以及任何人都可以捡起并使用的基础武器，例如棍棒，小刀，以及许多临时武器。",
      "斗殴": "包括空手格斗以及任何人都可以捡起并使用的基础武器，例如棍棒（例如板球棒或者棒球棍）、小刀、以及许多临时武器。",
      "斧": "当使用大型的木斧时使用这个技能。短柄小斧则可以用基础的斗殴技能。",
      "链锯": "使用链锯进行攻击。",
      "连枷": "索连棍、钉锤、以及相似的中世纪兵器。",
      "绞索": "任何长度的材料被用于绞死对方。需要受害者进行一个战技检定来逃脱，否则就要遭受每轮1D6的伤害。",
      "矛": "长枪或者鱼叉。如果投掷，使用投掷技能。",
      "剑": "所有的长度超过两英尺的利刃。",
      "鞭": "套索以及鞭子。",
      "射击": "包括了各种形式的火器，也包括了弓箭和弩。",
      "弓": "用来使用弓以及弩，包括从中世纪的长弓到现代，高性能的复合弓。",
      "手枪": "用来使用所有的类似于手枪的火器，进行非连续的射击。",
      "重武器": "用于使用枪榴弹发射器、反坦克火箭炮等等。",
      "火焰喷射器": "喷射出一连串点燃的可燃烧液体或者气体的武器。可以被操作者携带或者架设在交通工具上。",
      "机枪": "用两脚架或者三脚架架设的进行连续射击的武器。",
      "步霰": "可以用于射击任何类型的步枪（无论是杠杆作用，手动栓式或者半自动的）或者霰弹枪。",
      "冲锋枪": "使用任何全自动手枪或者冲锋枪开火时，使用这个技能。同样也用于突击步枪的全自动模式。",
      "闪避": "允许调查员本能地闪避攻击，投掷过来的投射物以及诸如此类的。如果一次攻击可以被看见，一名角色可以尝试闪避开它。",
      "投掷": "当需要用物体击中目标或者用物件的正确部分击中目标时，使用投掷技能。",
      "爆破": "熟练于安全使用爆破，包括设置以及拆除炸药。地雷以及相似的设备被设计得容易设置但是相对较为困难地进行除去或拆除。",
      "炮术": "呈现出对一些形式的军事训练和经历。使用者具有在战争中操作战地武器的经验，可以操作超过个人武器射击距离的武器。",

      // 医疗类技能
      "急救": "有能力可以提供紧急的医疗处理。这可能包括：对摔断了的腿用夹板进行处理，止血，处理烧伤，对一名溺水的受害者进行复苏处理，包扎以及清理伤口等等。急救不能用来治疗疾病。",
      "医学": "使用者可以诊断并治疗事故、创伤、疾病、毒药等，并且可以提供相关药品信息或者公共健康建议。",
      "精神分析": "指的是广泛的情感上的治疗。短期强化的精神分析可以恢复一名调查员患者的理智值。成功使用这项技能将允许角色在短期内克服恐惧症状，或者看穿幻觉。",
      "催眠": "使用者可以在一名自愿并经历过高度暗示、放松的目标身上引出出神似的状态，并且可能回忆起忘却的记忆。对于那些遭受精神创伤的人，催眠可以用来减轻其恐惧或者躁狂。",

      // 知识类技能
      "会计": "使你理解会计工作的流程以及一个企业或者个人的金融职务。通过检查账簿，你可以了解资金的流动情况与渠道，也能发现做假账的员工、对资金的偷偷挪用、对行贿或者敲诈的款项支付，以及经济状况是否比表面陈述的更好或者更差。",
      "法律": "代表你对相关法律、早期事件、法庭辩术或者法院程序了解的可能性。一个法律上的专家可能会获得巨大的奖励以及政治事务所。",
      "历史": "让一名调查员能够记住一个国家，城市，区域或者个人及其相关的重要情报。一个成功的检定可以用来帮助辨认先祖所熟悉的工具，科技，或者想法。",
      "考古学": "允许从过去的文化中鉴定一件古董的年代以及辨别它，以及用来发现赝品。获得建立以及开掘一个挖掘遗址的专业知识，以及辨认已消失的人类语言形式。",
      "博物学": "指对在自然环境中的植物以及动物生命的研究。它可以一般地对物种，栖息地进行辨认，并且可以辨认踪迹、足迹以及叫声。",
      "人类学": "使用者能够通过观察来辨认和理解一个人的生活方式。如果技能使用者持续观察一个其他的文化一段时间，或者在有正确资料环境下工作，那么他可以对文化方式以及道德习惯进行简单的预测。",
      "神秘学": "使用者可以识别出神秘学道具，用语和概念，以及民间传统，并且可以辨认魔法书以及神秘学记号。",
      "电子学": "用来发现并对电子设备的故障进行维修。允许制作简单的电子设备。",
      "科学": "科学专业上的理论和实践的能力，拥有这个技能的人接受过一定程度的正式的教育或者训练。",
      "天文学": "使用者可以知道在某个特定的日子或者一天早晚某个时间时哪颗恒星或者行星位于正上方，何时彗星和流星雨会出现，以及重要的恒星的名字。",
      "生物学": "关于生命和存活的有机物的学科，包括细胞学、生态学、基因学、组织学、微观生物学、生理学等等。",
      "植物学": "关于植物生命的研究，包括物种分类、结构、生长、繁殖、化学特性、进化原理、疾病，以及显微研究。",
      "化学": "有关物质组成，温度的影响，能量，以及作用于其上的压力的研究，也包括物质如何互相影响。",
      "密码学": "关于由其他人发展出来的用于隐藏对话或者信息内容用的暗码或者密语的研究。这项技能使使用者能够辨认，创造或破译暗码。",
      "工程学": "将科学发现利用起来进行实际应用，例如机器、结构、以及材料。",
      "司法科学": "对于证据的分析和检定的研究。通常与犯罪现场调查和实验室工作相联系。",
      "地质学": "用来决定大致的岩层年龄，辨认出化石的类型，区分矿物和水晶，确定合理的采矿和挖掘地址，评估土地，预测火山活动、地震、雪崩。",
      "数学": "对于数字和逻辑的研究，包括数学理论、应用以及理论上的解决方法设计和推演发展。",
      "气象学": "关于大气的科学研究，包括天气系统和形态，以及大气现象。",
      "药学": "关于化学复合物以及它们的在有机生命体上的效果的研究。包括药物的配方、创造以及施用。",
      "物理学": "使使用者能够理论上了解压力、材料、运动、磁力、电力、光学、辐射和相关的现象。",
      "动物学": "对专门联系到动物王国的生物学的研究，包括仍存在以及灭绝动物的生态结构，进化，分类，行为习性，以及分布。",
      "克苏鲁神话": "反应了对非人类（洛夫克拉夫特的）克苏鲁神话的了解。这个技能并不像学术技能一样建立在知识的积累之上。相反地，它代表了克苏鲁神话向人类思想的打开以及同化。"
    };
    // 构建技能映射
    const SKILLS = {
      byCategory: {},
      byKey: {}
    };
    Object.entries(SKILL_CATEGORIES).forEach(([category, skillNames]) => {
      SKILLS.byCategory[category] = [];
      skillNames.forEach(skillName => {
        const subSkills = SKILL_SUBCATEGORIES[skillName] || [];
        if (subSkills.length > 0) {
          subSkills.forEach(subSkill => {
            const key = subSkill;
            const displayName = \`\${skillName}（\${subSkill}）\`;
            const description = SKILL_DESCRIPTIONS[subSkill] || SKILL_DESCRIPTIONS[skillName] || '';
            const skillObj = { key, displayName, baseSkill: skillName, subSkill, description };
            SKILLS.byCategory[category].push(skillObj);
            SKILLS.byKey[key] = skillObj;
          });
          if (!SKILLS.byKey[skillName]) {
            SKILLS.byKey[skillName] = {
              key: skillName, displayName: skillName, baseSkill: skillName,
              subSkill: null, description: SKILL_DESCRIPTIONS[skillName] || ''
            };
          }
        } else {
          const key = skillName;
          const displayName = skillName;
          const description = SKILL_DESCRIPTIONS[skillName] || '';
          const skillObj = { key, displayName, baseSkill: skillName, subSkill: null, description };
          SKILLS.byCategory[category].push(skillObj);
          SKILLS.byKey[key] = skillObj;
        }
      });
    });

    const SKILL_BASE_VALUE = {
      // 常规技能项
      '会计': 5, '动物驯养': 5, '人类学': 1, '估价': 5, '考古学': 1,
      '炮术': 1, '取悦': 15, '攀爬': 20, '计算机使用': 5, '信用评级': 0,
      '克苏鲁神话': 0, '爆破': 1, '乔装': 5, '潜水': 1,
      '汽车驾驶': 20, '电气维修': 10, '电子学': 1, '话术': 5, '急救': 30,
      '历史': 5, '催眠': 1, '恐吓': 15, '跳跃': 20,
      '法律': 5, '图书馆使用': 20, '聆听': 20, '锁匠': 1, '机械维修': 10,
      '医学': 1, '博物学': 10, '导航': 10, '神秘学': 5, '操作重型机械': 1,
      '说服': 10, '精神分析': 1, '心理学': 10, '读唇': 1, '骑术': 5,
      '妙手': 10, '侦查': 25, '潜行': 20, '游泳': 20, '投掷': 20,
      '追踪': 10,

      // 专精技能大类
      '生存': 10,
      '艺术与手艺': 5,
      '格斗': 5,
      '射击': 10,
      '驾驶': 1,
      '科学': 1,
      '外语': 1,

      // 格斗技能子项
      '斧': 15, '斗殴': 25, '链锯': 10, '连枷': 10, '绞索': 15, '矛': 20, '剑': 20, '鞭': 5,
      // 射击技能子项
      '弓': 15, '手枪': 20, '重武器': 10, '火焰喷射器': 10, '机枪': 10, '步霰': 25, '冲锋枪': 15,
      // 科学技能子项
      '数学': 10,
      
      // 属性依赖技能项
      '闪避': (attrs) => Math.floor((attrs['敏捷'] || 0) / 2),
      '母语': (attrs) => attrs['教育'] || 0
    };
    // 构建属性依赖映射
    const SKILLS_DEPEND_ON_ATTR = {
      '敏捷': ['闪避'],
      '教育': ['母语']
    };

    const WEAPON_DATA = {
      '常规武器': {
        items: [
          { name: '徒手格斗', skill: '格斗（斗殴）', damage: '1D3+DB', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '', malfunction: '', era: '' },
          { name: '弓箭', skill: '射击（弓）', damage: '1D6+半DB', penetration: '', range: '30码', rateOfFire: '1', ammoCapacity: '1', price: '$7/$75', malfunction: 97, era: '1920s,现代' },
          { name: '黄铜指虎', skill: '格斗（斗殴）', damage: '1D3+1+DB', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$1/$10', malfunction: '', era: '1920s,现代' },
          { name: '牛鞭', skill: '格斗（鞭）', damage: '1D3+半DB', penetration: '', range: '3码', rateOfFire: '1', ammoCapacity: '', price: '$5/$50', malfunction: '', era: '1920s' },
          { name: '燃烧的火把', skill: '格斗（斗殴）', damage: '1D6+燃烧', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$0.05/$0.5', malfunction: '', era: '1920s,现代' },
          { name: '链锯*', skill: '格斗（链锯）', damage: '2D8', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '-/$300', malfunction: 95, era: '现代', note: '难以被用作武器，大失败的概率加倍。使用链锯时大失败的情况非常糟糕，可能导致链锯冲向使用者的头肩，或者向下切到腿脚，从而对使用者造成2D8点伤害，链锯造成的重伤会使伤者随机失去一条肢体。' },
          { name: '包革金属棒（大头棍、护身棒）', skill: '格斗（斗殴）', damage: '1D8+DB', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$2/$15', malfunction: '', era: '1920s,现代' },
          { name: '大型棍棒（棒球棒、板球棒、拨火棍）', skill: '格斗（斗殴）', damage: '1D8+DB', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$3/$35', malfunction: '', era: '1920s,现代' },
          { name: '小型棍棒（警棍）', skill: '格斗（斗殴）', damage: '1D6+DB', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$3/$35', malfunction: '', era: '1920s,现代' },
          { name: '弩', skill: '射击（弓）', damage: '1D8+2', penetration: '贯穿', range: '50码', rateOfFire: '1/2', ammoCapacity: '1', price: '$10/$100', malfunction: 96, era: '1920s,现代' },
          { name: '绞索*', skill: '格斗（绞索）', damage: '1D6+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$0.5/$3', malfunction: '', era: '1920s,现代', note: '受害者需要使用战技来逃脱，否则每轮受到1D6点伤害。只对人类和类似生物有效。' },
          { name: '手斧/镰刀', skill: '格斗（斧）', damage: '1D6+1+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$3/$9', malfunction: '', era: '1920s,现代' },
          { name: '大型刀具（弯刀等）', skill: '格斗（斗殴）', damage: '1D8+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$4/$50', malfunction: '', era: '1920s,现代' },
          { name: '中型刀具（切肉刀等）', skill: '格斗（斗殴）', damage: '1D4+2+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$2/$15', malfunction: '', era: '1920s,现代' },
          { name: '小型刀具（折叠刀等）', skill: '格斗（斗殴）', damage: '1D4+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$2/$6', malfunction: '', era: '1920s,现代' },
          { name: '220V通电导线', skill: '格斗（斗殴）', damage: '2D8+眩晕', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '', malfunction: 95, era: '现代' },
          { name: '催泪喷雾*', skill: '格斗（斗殴）', damage: '晕眩', penetration: '', range: '2码', rateOfFire: '1', ammoCapacity: '25次喷射', price: '-/$10', malfunction: '', era: '1920s,现代', note: '这种武器不使用抵近射击规则；目标须通过极难难度的DEX检定以避免暂时失明。只对人类和类似生物有效。' },
          { name: '双节棍', skill: '格斗（连枷）', damage: '1D8+DB', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$1/$10', malfunction: '', era: '1920s,现代' },
          { name: '投石', skill: '投掷', damage: '1D4+半DB', penetration: '', range: 'STR/5码', rateOfFire: '1', ammoCapacity: '', price: '', malfunction: '', era: '1920s,现代' },
          { name: '手里剑', skill: '投掷', damage: '1D3+半DB', penetration: '贯穿', range: 'STR/5码', rateOfFire: '2', ammoCapacity: '一次性', price: '$0.5/$3', malfunction: 100, era: '1920s,现代' },
          { name: '矛(骑枪)', skill: '格斗（矛）', damage: '1D8+1', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$25/$150', malfunction: '', era: '1920s,现代' },
          { name: '投矛', skill: '投掷', damage: '1D8+半DB', penetration: '贯穿', range: 'STR/5码', rateOfFire: '1', ammoCapacity: '', price: '$1/$25', malfunction: '', era: '稀有' },
          { name: '大型剑(马刀)', skill: '格斗（剑）', damage: '1D8+1+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$30/$75', malfunction: '', era: '1920s,现代' },
          { name: '中型剑(长剑、重剑)', skill: '格斗（剑）', damage: '1D6+1+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$15/$100', malfunction: '', era: '1920s,现代' },
          { name: '轻型剑(花剑、剑杖)', skill: '格斗（剑）', damage: '1D6+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$25/$100', malfunction: '', era: '1920s,现代' },
          { name: '电击器*', skill: '格斗（斗殴）', damage: '1D3+晕眩', penetration: '', range: '接触', rateOfFire: '1', ammoCapacity: '多种', price: '-/$200', malfunction: 97, era: '现代', note: '仅对体格2及以下的目标有效，晕眩的目标在1D6回合内无法行动（或由KP决定）。' },
          { name: '泰瑟枪*', skill: '射击（手枪）', damage: '1D3+晕眩', penetration: '', range: '5码', rateOfFire: '1', ammoCapacity: '3', price: '-/$400', malfunction: 95, era: '现代', note: '仅对体格2及以下的目标有效，晕眩的目标在1D6回合内无法行动（或由KP决定）。' },
          { name: '战斗回力镖', skill: '投掷', damage: '1D8+半DB', penetration: '', range: 'STR/5码', rateOfFire: '1', ammoCapacity: '', price: '$2/$4', malfunction: '', era: '稀有' },
          { name: '伐木斧', skill: '格斗（斧）', damage: '1D8+2+DB', penetration: '贯穿', range: '接触', rateOfFire: '1', ammoCapacity: '', price: '$5/$10', malfunction: '', era: '1920s,现代' },
        ]
      },

      '手枪*': {
        note: '如果每轮射击多于1次，则每次掷骰都承受一颗惩罚骰。括号内的数字表示每轮能够进行的最大射击次数。',
        items: [
          { name: '燧发手枪', skill: '射击（手枪）', damage: '1D6+1', penetration: '贯穿', range: '10码', rateOfFire: '1/4', ammoCapacity: '1', price: '$30/$300', malfunction: 95, era: '稀有' },
          { name: '.22口径自动手枪', skill: '射击（手枪）', damage: '1D6', penetration: '贯穿', range: '10码', rateOfFire: '1(3)', ammoCapacity: '6', price: '$25/$190', malfunction: 100, era: '1920s,现代' },
          { name: '.25德林杰手枪(单管)', skill: '射击（手枪）', damage: '1D6', penetration: '贯穿', range: '3码', rateOfFire: '1', ammoCapacity: '1', price: '$12/$55', malfunction: 100, era: '1920s' },
          { name: '.32/7.65mm左轮手枪', skill: '射击（手枪）', damage: '1D8', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '6', price: '$15/$200', malfunction: 100, era: '1920s,现代' },
          { name: '.32/7.65mm自动手枪', skill: '射击（手枪）', damage: '1D8', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '8', price: '$20/$350', malfunction: 99, era: '1920s,现代' },
          { name: '.357马格南左轮手枪', skill: '射击（手枪）', damage: '1D8+1D4', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '6', price: '-/$425', malfunction: 100, era: '现代' },
          { name: '.38/9mm左轮手枪', skill: '射击（手枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '6', price: '$25/$200', malfunction: 100, era: '1920s,现代' },
          { name: '.38/9mm自动手枪', skill: '射击（手枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '8', price: '$30/$375', malfunction: 99, era: '1920s,现代' },
          { name: '贝瑞塔M9', skill: '射击（手枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '15', price: '-/$500', malfunction: 98, era: '现代' },
          { name: '9mm格洛克17', skill: '射击（手枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '17', price: '-/$500', malfunction: 98, era: '现代' },
          { name: '9mm鲁格P08', skill: '射击（手枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '8', price: '$75/$600', malfunction: 99, era: '1920s,现代' },
          { name: '.41左轮手枪', skill: '射击（手枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '8', price: '$30/-', malfunction: 100, era: '1920s,稀有' },
          { name: '.44马格南左轮手枪', skill: '射击（手枪）', damage: '1D10+1D4+2', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '6', price: '-/$475', malfunction: 100, era: '现代' },
          { name: '.45左轮手枪', skill: '射击（手枪）', damage: '1D10+2', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '6', price: '$30/$300', malfunction: 100, era: '1920s,现代' },
          { name: '.45自动手枪', skill: '射击（手枪）', damage: '1D10+2', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '7', price: '$40/$375', malfunction: 100, era: '1920s,现代' },
          { name: 'IMI 沙漠之鹰', skill: '射击（手枪）', damage: '1D10+1D6+3', penetration: '贯穿', range: '15码', rateOfFire: '1(3)', ammoCapacity: '7', price: '-/$650', malfunction: 94, era: '现代' },
        ]
      },

      '步枪*': {
        note: '大多数步枪每轮射击1次。重新装填需要时间，无法在一轮内装填并击发。',
        items: [
          { name: '.58春田燧发步枪', skill: '射击（步霰）', damage: '1D10+4', penetration: '贯穿', range: '60码', rateOfFire: '1/4', ammoCapacity: '1', price: '$25/$350', malfunction: 95, era: '稀有' },
          { name: '.22栓动步枪', skill: '射击（步霰）', damage: '1D6+1', penetration: '贯穿', range: '30码', rateOfFire: '1', ammoCapacity: '6', price: '$13/$70', malfunction: 99, era: '1920s,现代' },
          { name: '.30杠杆步枪', skill: '射击（步霰）', damage: '2D6', penetration: '贯穿', range: '50码', rateOfFire: '1', ammoCapacity: '6', price: '$19/$150', malfunction: 98, era: '1920s,现代' },
          { name: '.45马蒂尼-亨利步枪', skill: '射击（步霰）', damage: '1D8+1D6+3', penetration: '贯穿', range: '80码', rateOfFire: '1/3', ammoCapacity: '1', price: '$20/$200', malfunction: 100, era: '1920s' },
          { name: '莫兰上校的气动步枪*', skill: '射击（步枪）', damage: '2D6+1', penetration: '贯穿', range: '20码', rateOfFire: '1/3', ammoCapacity: '1', price: '$200/$-', malfunction: 88, era: '1920s', note: '依靠压缩空气发射，而非火药，因而较为安静。' },
          { name: '加兰德M1、M2步枪', skill: '射击（步霰）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '1', ammoCapacity: '8', price: '-/$400', malfunction: 100, era: '二战及以后' },
          { name: 'SKS半自动步枪', skill: '射击（步霰）', damage: '2D6+1', penetration: '贯穿', range: '90码', rateOfFire: '1(2)', ammoCapacity: '10', price: '-/$500', malfunction: 97, era: '现代' },
          { name: '.303李-恩菲尔德步枪', skill: '射击（步霰）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '1', ammoCapacity: '10', price: '$50/$300', malfunction: 100, era: '1920s,现代' },
          { name: '.30-06(7.62mm)栓动步枪', skill: '射击（步霰）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '1', ammoCapacity: '5', price: '$75/$175', malfunction: 100, era: '1920s,现代' },
          { name: '.30-06(7.62mm)半自动步枪', skill: '射击（步霰）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '1', ammoCapacity: '5', price: '-/$275', malfunction: 100, era: '现代' },
          { name: '.444马林杠杆步枪', skill: '射击（步霰）', damage: '2D8+4', penetration: '贯穿', range: '110码', rateOfFire: '1', ammoCapacity: '5', price: '-/$400', malfunction: 98, era: '现代' },
          { name: '双管猎象枪', skill: '射击（步霰）', damage: '3D6+4', penetration: '贯穿', range: '100码', rateOfFire: '1或2', ammoCapacity: '2', price: '$400/$1800', malfunction: 100, era: '1920s,现代' },
        ]
      },

      '霰弹枪*': {
        note: '根据距离（射程）不同，伤害被分为三档，标注为“近程/中程/远程”。此外还可以装填独头弹，10号伤害1D10+7，12号伤害1D10+6，16号伤害1D10+5，20号伤害1D10+4，基础射程50码，可以贯穿。',
        items: [
          { name: '20号双管霰弹枪', skill: '射击（步霰）', damage: '2D6/1D6/1D3', penetration: '', range: '10/20/50码', rateOfFire: '1或2', ammoCapacity: '2', price: '$35/稀有', malfunction: 100, era: '1920s' },
          { name: '16号双管霰弹枪', skill: '射击（步霰）', damage: '2D6+2/1D6+1/1D4', penetration: '', range: '10/20/50码', rateOfFire: '1或2', ammoCapacity: '2', price: '$40/稀有', malfunction: 100, era: '1920s' },
          { name: '12号双管霰弹枪', skill: '射击（步霰）', damage: '4D6/2D6/1D6', penetration: '', range: '10/20/50码', rateOfFire: '1或2', ammoCapacity: '2', price: '$40/$200', malfunction: 100, era: '1920s,现代' },
          { name: '12号泵动式霰弹枪', skill: '射击（步霰）', damage: '4D6/2D6/1D6', penetration: '', range: '10/20/50码', rateOfFire: '1', ammoCapacity: '5', price: '$45/$100', malfunction: 100, era: '现代' },
          { name: '12号半自动霰弹枪', skill: '射击（步霰）', damage: '4D6/2D6/1D6', penetration: '', range: '10/20/50码', rateOfFire: '1(2)', ammoCapacity: '5', price: '$45/$100', malfunction: 100, era: '现代' },
          { name: '12号双管霰弹枪（锯短枪管）', skill: '射击（步霰）', damage: '4D6/1D6', penetration: '', range: '5/10码', rateOfFire: '1或2', ammoCapacity: '2', price: 'N/A', malfunction: 100, era: '1920s' },
          { name: '10号双管霰弹枪', skill: '射击（步霰）', damage: '4D6+2/2D6+1/1D4', penetration: '', range: '10/20/50码', rateOfFire: '1或2', ammoCapacity: '2', price: '稀有', malfunction: 100, era: '1920s稀有' },
          { name: '12号伯奈利M3霰弹枪（折叠枪托）', skill: '射击（步霰）', damage: '4D6/2D6/1D6', penetration: '', range: '10/20/50码', rateOfFire: '1(2)', ammoCapacity: '7', price: '-/$895', malfunction: 100, era: '现代' },
          { name: '12号SPAS霰弹枪（折叠枪托）', skill: '射击（步霰）', damage: '4D6/2D6/1D6', penetration: '', range: '10/20/50码', rateOfFire: '1', ammoCapacity: '8', price: '-/$600', malfunction: 98, era: '现代' },
        ]
      },

      '突击步枪*': {
        note: '可以在单发、短点射和全自动模式之间切换。单发模式下使用步霰技能，短点射或全自动模式下使用冲锋枪技能。',
        items: [
          { name: 'AK-47或AKM', skill: '射击（步霰）', damage: '2D6+1', penetration: '贯穿', range: '100码', rateOfFire: '1(2)或全自动', ammoCapacity: '30', price: '-/$200', malfunction: 100, era: '现代' },
          { name: 'AK-74', skill: '射击（步霰）', damage: '2D6+1', penetration: '贯穿', range: '110码', rateOfFire: '1(2)或全自动', ammoCapacity: '30', price: '-/$1000', malfunction: 97, era: '现代' },
          { name: '巴雷特M82反器材步枪', skill: '射击（步霰）', damage: '2D10+1D8+6', penetration: '贯穿', range: '250码', rateOfFire: '1', ammoCapacity: '11', price: '-/$3000', malfunction: 96, era: '现代' },
          { name: 'FN FAL 突击步枪', skill: '射击（步霰）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '1(2)或3发点射', ammoCapacity: '20', price: '-/$1500', malfunction: 97, era: '现代' },
          { name: '加利尔突击步枪', skill: '射击（步霰）', damage: '2D6', penetration: '贯穿', range: '110码', rateOfFire: '1(2)或全自动', ammoCapacity: '20', price: '-/$2000', malfunction: 98, era: '现代' },
          { name: 'M16A2', skill: '射击（步霰）', damage: '2D6', penetration: '贯穿', range: '110码', rateOfFire: '1(2)或3发点射', ammoCapacity: '30', price: 'N/A', malfunction: 97, era: '现代' },
          { name: 'M4', skill: '射击（步霰）', damage: '2D6', penetration: '贯穿', range: '90码', rateOfFire: '1或3发点射', ammoCapacity: '30', price: 'N/A', malfunction: 97, era: '现代' },
          { name: '斯太尔AUG', skill: '射击（步霰）', damage: '2D6', penetration: '贯穿', range: '110码', rateOfFire: '1(2)或全自动', ammoCapacity: '30', price: '-/$1100', malfunction: 99, era: '现代' },
          { name: '贝雷塔M70/90', skill: '射击（步霰）', damage: '2D6', penetration: '贯穿', range: '110码', rateOfFire: '1或全自动', ammoCapacity: '30', price: '-/$2800', malfunction: 99, era: '现代' },
        ]
      },

      '冲锋枪': {
        items: [
          { name: '贝格曼MP181/MP2811', skill: '射击（冲锋枪）', damage: '1D10', penetration: '贯穿', range: '20码', rateOfFire: '1(2)或全自动', ammoCapacity: '20/30/32', price: '$1000/$20000', malfunction: 96, era: '1920s' },
          { name: 'H&K MP5', skill: '射击（冲锋枪）', damage: '1D10', penetration: '贯穿', range: '20码', rateOfFire: '1(2)或全自动', ammoCapacity: '15/30', price: 'N/A', malfunction: 97, era: '现代' },
          { name: 'MAC-11', skill: '射击（冲锋枪）', damage: '1D10', penetration: '贯穿', range: '15码', rateOfFire: '1(3)或全自动', ammoCapacity: '32', price: '-/$750', malfunction: 96, era: '现代' },
          { name: '蝎式冲锋枪', skill: '射击（冲锋枪）', damage: '1D8', penetration: '贯穿', range: '15码', rateOfFire: '1(3)或全自动', ammoCapacity: '20', price: 'N/A', malfunction: 96, era: '现代' },
          { name: '汤普森冲锋枪', skill: '射击（冲锋枪）', damage: '1D10+2', penetration: '贯穿', range: '20码', rateOfFire: '1或全自动', ammoCapacity: '20/30/50', price: '$200+/$1600', malfunction: 96, era: '1920s' },
          { name: '乌兹冲锋枪', skill: '射击（冲锋枪）', damage: '1D10', penetration: '贯穿', range: '20码', rateOfFire: '1(2)或全自动', ammoCapacity: '32', price: '-/$1000', malfunction: 98, era: '现代' },
        ]
      },

      '机枪': {
        items: [
          { name: '1882年式手摇加特林', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '100码', rateOfFire: '全自动', ammoCapacity: '200', price: '$2000/$14000', malfunction: 96, era: '1920s,稀有' },
          { name: 'M1918勃朗宁自动步枪', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '90码', rateOfFire: '1(2)或全自动', ammoCapacity: '20', price: '$800/$1500', malfunction: 100, era: '1920s' },
          { name: '勃朗宁M1917A1 (.30-06/7.62mm)', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '150码', rateOfFire: '全自动', ammoCapacity: '250', price: '$3000/$3万', malfunction: 96, era: '1920s' },
          { name: '布伦轻机枪', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '1或全自动', ammoCapacity: '30/100', price: '$3000/$5万', malfunction: 96, era: '1920s' },
          { name: '刘易斯MK.I型机枪', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '全自动', ammoCapacity: '47/97', price: '$3000/$2万', malfunction: 96, era: '1920s' },
          { name: '转管速射机枪*(7.62mm)', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '200码', rateOfFire: '全自动', ammoCapacity: '4000', price: 'N/A', malfunction: 98, era: '现代', note: '加特林式重机枪，通常被安装在直升机上。若要不经安装直接使用这种武器，需要使用者的体格至少为2。' },
          { name: 'FN米尼米机枪(5.56mm)', skill: '射击（机枪）', damage: '2D6', penetration: '贯穿', range: '110码', rateOfFire: '全自动', ammoCapacity: '30/200', price: 'N/A', malfunction: 99, era: '现代' },
          { name: '维克斯.303机枪', skill: '射击（机枪）', damage: '2D6+4', penetration: '贯穿', range: '110码', rateOfFire: '全自动', ammoCapacity: '250', price: 'N/A', malfunction: 99, era: '1920s' },
        ]
      },

      '爆炸物、重武器和其他武器': {
        items: [
          { name: '莫洛托夫鸡尾酒', skill: '投掷', damage: '2D6+燃烧', penetration: '贯穿', range: 'STR/5码', rateOfFire: '1/2', ammoCapacity: '一次性', price: 'N/A', malfunction: 95, era: '1920s,现代' },
          { name: '信号枪', skill: '射击（手枪）', damage: '1D10+1D3+燃烧', penetration: '贯穿', range: '10码', rateOfFire: '1/2', ammoCapacity: '1', price: '$15/$75', malfunction: 100, era: '1920s,现代' },
          { name: 'M79榴弹发射器', skill: '射击（重武器）', damage: '3D10/2码', penetration: '贯穿', range: '20码', rateOfFire: '1/3', ammoCapacity: '1', price: '', malfunction: 99, era: '现代' },
          { name: '炸药棒*', skill: '投掷', damage: '4D10/3码', penetration: '贯穿', range: 'STR/5码', rateOfFire: '1/2', ammoCapacity: '一次性', price: '$2/$5', malfunction: 99, era: '1920s,现代', note: '对3码内的目标造成4D10点伤害，超过3码且在6码之内的目标造成2D10点伤害，超过6码且在9码之内的目标造成1D10点伤害。' },
          { name: '雷管', skill: '电气维修', damage: '2D10/1码', penetration: '贯穿', range: 'N/A', rateOfFire: 'N/A', ammoCapacity: '一次性', price: '每盒$1/$20', malfunction: 100, era: '1920s,现代' },
          { name: '管状土制炸弹', skill: '爆破', damage: '1D10/3码', penetration: '贯穿', range: '即地', rateOfFire: '一次性', ammoCapacity: '一次性', price: 'N/A', malfunction: 95, era: '现代' },
          { name: '塑胶炸弹(C-4)100g', skill: '爆破', damage: '6D10/3码', penetration: '贯穿', range: '即地', rateOfFire: '一次性', ammoCapacity: '一次性', price: 'N/A', malfunction: 99, era: '现代' },
          { name: '手榴弹*', skill: '投掷', damage: '4D10/3码', penetration: '贯穿', range: 'STR/5码', rateOfFire: '1/2', ammoCapacity: '一次性', price: 'N/A', malfunction: 99, era: '1920s,现代', note: '对3码内的角色造成4D10点伤害，超过3码且在6码之内的角色造成2D10点伤害，超过6码且在9码之内的角色造成1D10点伤害。' },
          { name: '81mm迫击炮', skill: '炮术', damage: '6D10/6码', penetration: '贯穿', range: '500码', rateOfFire: '1', ammoCapacity: '独立装弹', price: 'N/A', malfunction: 100, era: '现代' },
          { name: '75mm野战炮', skill: '炮术', damage: '10D10/2码', penetration: '贯穿', range: '500码', rateOfFire: '1/4', ammoCapacity: '独立装弹', price: '$1500/-', malfunction: 99, era: '1920s,现代' },
          { name: '120mm坦克炮', skill: '炮术', damage: '15D10/2码', penetration: '贯穿', range: '2000码', rateOfFire: '1', ammoCapacity: '独立装弹', price: 'N/A', malfunction: 100, era: '现代' },
          { name: '5英寸(127mm)舰载炮', skill: '炮术', damage: '12D10/4码', penetration: '贯穿', range: '3000码', rateOfFire: '1', ammoCapacity: '自动上弹', price: 'N/A', malfunction: 98, era: '现代' },
          { name: '反步兵地雷', skill: '爆破', damage: '4D10/5码', penetration: '贯穿', range: '即地', rateOfFire: '布置', ammoCapacity: '一次性', price: 'N/A', malfunction: 99, era: '现代' },
          { name: '阔剑地雷*', skill: '爆破', damage: '6D6/20码', penetration: '贯穿', range: '即地', rateOfFire: '布置', ammoCapacity: '一次性', price: 'N/A', malfunction: 99, era: '现代', note: '这种武器的弹道是密集的射束流，杀伤范围为120度。' },
          { name: '火焰喷射器', skill: '射击（火焰喷射器）', damage: '2D6+燃烧', penetration: '贯穿', range: '25码', rateOfFire: '1', ammoCapacity: '至少10', price: 'N/A', malfunction: 93, era: '1920s,现代' },
          { name: 'M72反坦克火箭筒', skill: '射击（重武器）', damage: '8D10/1码', penetration: '贯穿', range: '150码', rateOfFire: '1', ammoCapacity: '1', price: 'N/A', malfunction: 98, era: '现代' },
        ]
      },
    };
    // 构建武器映射
    const WEAPON_MAP = Object.fromEntries(
      Object.entries(WEAPON_DATA).flatMap(([type, group]) =>
        group.items.map(weapon => [weapon.name, { weapon, type }])
      )
    );

    const ASSET_DATA = {
      '1920s': {
        unit: '$',
        levels: [
          { name: '身无分文', crRange: [0, 0], cash: '0.5', assets: '—', spending: '0.5' },
          { name: '贫穷', crRange: [1, 9], cash: 'CR * 1', assets: 'CR * 10', spending: '2' },
          { name: '标准', crRange: [10, 49], cash: 'CR * 2', assets: 'CR * 50', spending: '10' },
          { name: '小康', crRange: [50, 89], cash: 'CR * 5', assets: 'CR * 500', spending: '50' },
          { name: '富裕', crRange: [90, 98], cash: 'CR * 20', assets: 'CR * 2000', spending: '250' },
          { name: '富豪', crRange: [99, 99], cash: '50000', assets: '5M+', spending: '5000' }
        ]
      },
      '现代': {
        unit: '$',
        levels: [
          { name: '身无分文', crRange: [0, 0], cash: '10', assets: '—', spending: '10' },
          { name: '贫穷', crRange: [1, 9], cash: 'CR * 20', assets: 'CR * 200', spending: '40' },
          { name: '标准', crRange: [10, 49], cash: 'CR * 40', assets: 'CR * 1000', spending: '200' },
          { name: '小康', crRange: [50, 89], cash: 'CR * 100', assets: 'CR * 10000', spending: '1000' },
          { name: '富裕', crRange: [90, 98], cash: 'CR * 200', assets: 'CR * 40000', spending: '5000' },
          { name: '富豪', crRange: [99, 99], cash: '1M', assets: '100M+', spending: '100000' }
        ]
      },
      '维多利亚': {
        unit: '£',
        note: '12便士=1先令；20先令=1英镑(￡)',
        levels: [
          { name: '身无分文', crRange: [0, 0], cash: '5先令', assets: '—', spending: '5便士' },
          { name: '贫穷', crRange: [1, 9], cash: 'CR * 1', assets: 'CR * 10', spending: '20先令' },
          { name: '标准', crRange: [10, 49], cash: 'CR * 10', assets: 'CR * 20', spending: '2磅10先令' },
          { name: '小康', crRange: [50, 89], cash: 'CR * 12', assets: 'CR * 200', spending: '15' },
          { name: '富裕', crRange: [90, 98], cash: 'CR * 50', assets: 'CR * 500', spending: '50' },
          { name: '富豪', crRange: [99, 99], cash: '150000', assets: '300000+', spending: '250' }
        ]
      }
    };
    // 构建CR区间映射
    Object.values(ASSET_DATA).forEach(table => {
      const crRanges = table.levels.map(l => l.crRange);
      table.minCr = Math.min(...crRanges.map(r => r[0]));
      table.maxCr = Math.max(...crRanges.map(r => r[1]));
    });

    const CUSTOM_DATA_KEY = '$SEALCHAT_自定义数据';
    // 首次使用时自动推送的自定义数据结构
    const CUSTOM_DATA_TEMPLATE = {
      tip: '这是跟随人物卡数据的自定义职业/技能/武器，其中带有 _template: true 的条目会被自动忽略。请根据需要自行仿照样例增减或修改这些条目。',
      职业: [
        { name: JOB_DATA[0].name, desc: JOB_DATA[0].desc, credit: JOB_DATA[0].credit,
          Skills: JOB_DATA[0].Skills, SkillPoint: JOB_DATA[0].SkillPoint, SkillExt: JOB_DATA[0].SkillExt, _template: true }
      ],
      武器: [
        { name: WEAPON_DATA['常规武器'].items[0].name, skill: WEAPON_DATA['常规武器'].items[0].skill,
          damage: WEAPON_DATA['常规武器'].items[0].damage, penetration: WEAPON_DATA['常规武器'].items[0].penetration,
          range: WEAPON_DATA['常规武器'].items[0].range, rateOfFire: WEAPON_DATA['常规武器'].items[0].rateOfFire,
          ammoCapacity: WEAPON_DATA['常规武器'].items[0].ammoCapacity, price: WEAPON_DATA['常规武器'].items[0].price,
          malfunction: WEAPON_DATA['常规武器'].items[0].malfunction, era: WEAPON_DATA['常规武器'].items[0].era,
          note: '', _template: true }
      ],
      技能: [
        { name: SKILLS.byCategory[Object.keys(SKILL_CATEGORIES)[0]][0].key,
          baseValue: SKILL_BASE_VALUE[SKILLS.byCategory[Object.keys(SKILL_CATEGORIES)[0]][0].key] || 0,
          category: Object.keys(SKILL_CATEGORIES)[0],
          description: SKILLS.byCategory[Object.keys(SKILL_CATEGORIES)[0]][0].description, _template: true }
      ]
    };

    // ==================== 工具函数 ====================
    // 转义html与英文引号
    function escapeHtml(text) {
      if (text == null) return '';
      const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
        '\`': '&#96;'
      };
      return toString(text).replace(/[&<>"'\`]/g, m => map[m]);
    }

    function toNumber(val, nanFallback = 0) {
      if (typeof val === 'number') return Math.trunc(val);
      if (typeof val === 'string') {
        const num = Number(val.trim());
        return isNaN(num) ? nanFallback : Math.trunc(num);
      }
      return nanFallback;
    }
    function toString(val) {
      return val == null ? '' : String(val);
    }

    // 防抖函数
    function debounce(func, delay) {
      let timer;
      return function(...args) {
        clearTimeout(timer);
        timer = setTimeout(() => func.apply(this, args), delay);
      };
    }

    // 计算伤害加值
    function calculateDamageBonus(str, siz) {
      const sum = str + siz;
      if (sum < 65) return '-2';
      if (sum < 85) return '-1';
      if (sum < 125) return '0';
      if (sum < 165) return '+1D4';
      if (sum < 205) return '+1D6';
      const db = Math.floor((sum - 205) / 80) + 2;
      return \`+\${db}D6\`;
    }

    // 计算体格
    function calculateBuild(str, siz) {
      const sum = str + siz;
      if (sum < 65) return '-2';
      if (sum < 85) return '-1';
      if (sum < 125) return '0';
      if (sum < 165) return '+1';
      if (sum < 205) return '+2';
      const build = Math.floor((sum - 205) / 80) + 3;
      return \`+\${build}\`;
    }

    // 计算移动速度
    function calculateMove(age, dex, str, siz) {
      const base = (dex > siz && str > siz) ? 9 : (dex < siz && str < siz) ? 7 : 8;
      const agePenalty = age >= 80 ? 5 : age >= 70 ? 4 : age >= 60 ? 3 : age >= 50 ? 2 : age >= 40 ? 1 : 0;
      return Math.max(1, base - agePenalty);
    }

    // 归一化技能key
    function normalizeSkillKey(key) {
      if (typeof key !== 'string') return key;
      const colonIndex = key.indexOf(':');
      if (colonIndex === -1) return key;
      const baseSkill = key.slice(0, colonIndex).trim();
      const subSkill = key.slice(colonIndex + 1).trim();
      if (SKILL_SUBCATEGORIES[baseSkill] && SKILL_SUBCATEGORIES[baseSkill].includes(subSkill)) {
        return subSkill;
      }
      return key; 
    }

    // 计算职业技能点
    function calculateSkillPoint(expr, attrs) {
      if (!expr) return 0;
      try {
        const evalStr = expr
          .replace(/×/g, '*')
          .replace(/÷/g, '/')
          .replace(/力量/g, attrs['力量'] || 0)
          .replace(/体质/g, attrs['体质'] || 0)
          .replace(/体型/g, attrs['体型'] || 0)
          .replace(/敏捷/g, attrs['敏捷'] || 0)
          .replace(/外貌/g, attrs['外貌'] || 0)
          .replace(/智力/g, attrs['智力'] || 0)
          .replace(/意志/g, attrs['意志'] || 0)
          .replace(/教育/g, attrs['教育'] || 0)
          .replace(/Max/g, 'Math.max');
        return toNumber(new Function('return ' + evalStr)());
      } catch (e) {
        return 0;
      }
    }

    // 获取技能基础值
    function getSkillBaseValue(skillName, subSkill, attrs) {
      try {
        if (subSkill && SKILL_BASE_VALUE[subSkill] !== undefined) {
          const subDef = SKILL_BASE_VALUE[subSkill];
          return typeof subDef === 'function' ? subDef(attrs) : subDef;
        }

        const def = SKILL_BASE_VALUE[skillName];
        if (def === undefined) return 0; 
        return typeof def === 'function' ? def(attrs) : def;
      } catch (e) {
        return 0;
      }
    }

    // 解析武器伤害
    function parseWeaponDamage(displayText) {
      if (!displayText) return '';
      let expr = displayText.trim();

      if (expr.startsWith('&(') && expr.endsWith(')')) {
        expr = expr.slice(2, -1);
      }
      expr = expr.replace(/半DB/g, '(DB/2)');
      expr = expr.replace(/\\/\\d+码.*$/, '');
      expr = expr.replace(/\\/$/, '');
      return \`&(\${expr})\`;
    }

    // 获取生活水平
    function getLivingLevel(era, credit) {
      const defaultLevel = { name: '未知', cash: '', assets: '', spending: '' };
      const table = ASSET_DATA[era];
      if (!table) {
        return {  livingLevel: defaultLevel, matchedCr: credit, unit: '', note: '' };
      }
      const unit = table.unit || '';
      const note = table.note || '';

      let level = table.levels.find(l => credit >= l.crRange[0] && credit <= l.crRange[1]);
      let matchedCr = credit;
      if (!level) {
        matchedCr = Math.min(table.maxCr, Math.max(table.minCr, credit));
        level = table.levels.find(l => matchedCr >= l.crRange[0] && matchedCr <= l.crRange[1]);
      }
      return { livingLevel: level || defaultLevel, matchedCr, unit, note };
    } 

    // 计算资产表达式
    function calculateAssetValue(expr, credit) {
      if (!expr) return '';
      if (expr.toUpperCase().includes('CR')) {
        const evalExpr = expr.replace(/CR/gi, credit).replace(/×/g, '*').replace(/÷/g, '/');
        try {
          const num = toNumber(new Function('return ' + evalExpr)(), undefined);
          if (num !== undefined) return num;
        } catch (e) {
        }
      }
      return expr;
    }

    // 自定义数据合并
    function applyCustomData(customData) {
      if (!customData || typeof customData !== 'object') return;

      if (Array.isArray(customData.职业)) {
        customData.职业.forEach(job => {
          if (!job || !job.name || job._template || JOB_MAP[job.name]) return;
          job._custom = true;
          JOB_DATA.push(job);
          JOB_MAP[job.name] = job;
        });
      }

      if (Array.isArray(customData.武器)) {
        if (!WEAPON_DATA['自定义武器']) {
          WEAPON_DATA['自定义武器'] = { items: [] };
        }
        customData.武器.forEach(weapon => {
          if (!weapon || !weapon.name || weapon._template || WEAPON_MAP[weapon.name]) return;
          WEAPON_DATA['自定义武器'].items.push(weapon);
          WEAPON_MAP[weapon.name] = { weapon, type: '自定义武器' };
        });
      }

      if (Array.isArray(customData.技能)) {
        customData.技能.forEach(skill => {
          if (!skill || !skill.name || skill._template || SKILLS.byKey[skill.name]) return;

          const skillObj = {
            key: skill.name,
            displayName: skill.name,
            baseSkill: skill.name,
            subSkill: null,
            description: skill.description || ''
          };

          SKILLS.byKey[skill.name] = skillObj;
          if (skill.baseValue !== undefined) {
            SKILL_BASE_VALUE[skill.name] = toNumber(skill.baseValue);
          }

          const cat = (skill.category && SKILL_CATEGORIES[skill.category])
            ? skill.category
            : Object.keys(SKILL_CATEGORIES)[0];
          if (!SKILLS.byCategory[cat]) SKILLS.byCategory[cat] = [];
          SKILLS.byCategory[cat].push(skillObj);
        });
      }
    }

    // 字体加载检测
    function fontFamily(cssVar) {
      const raw = getComputedStyle(document.documentElement).getPropertyValue(cssVar).trim();
      const names = raw.split(',');
      for (let i = 0; i < names.length; i++) {
        const name = names[i].replace(/['"]/g, '').trim();
        if (name) return name;
      }
      return '';
    }
    function loadFont(cssVar, opts) {
      opts = opts || {};
      const family = fontFamily(cssVar);
      if (!family) return Promise.resolve(true);
      if (!document.fonts) return Promise.resolve(false);

      return Promise.race([
        document.fonts.load('1em ' + family).then(function() { return true; }),
        new Promise(function(r) { setTimeout(function() { r(false); }, opts.timeout || 8000); })
      ]);
    }
    function isFontAvailable(cssVar) {
      if (!document.fonts) return true;
      const family = fontFamily(cssVar);
      return family ? document.fonts.check('1em ' + family) : true;
    }

    // 切换加载动画
    function toggleLoading(show) {
      const el = document.getElementById('loadingIndicator');
      if (el) el.style.display = show ? 'flex' : 'none';
    }
    function hideLoadingWhenReady() {
      if (!document.fonts) { toggleLoading(false); return; }

      const done = Promise.all([
        loadFont('--font-gothic'),
        loadFont('--font-serif')
      ]);
      const cap = new Promise(function(r) { setTimeout(r, 5000); });
      Promise.race([done, cap]).finally(function() { toggleLoading(false); });
    }

    // 绘制属性雷达图
    function drawRadar() {
      const canvas = document.getElementById('canvasCharRadar');
      if (!canvas) return;
      const ch = state.character;
      if (!isFontAvailable('--font-gothic')) {
        if (!state.radarPending) {
          state.radarPending = true;
          loadFont('--font-gothic').finally(function() {
            state.radarPending = false;
            drawRadar();
          });
        }
        return;
      }

      const containerWidth = canvas.clientWidth;
      const containerHeight = canvas.clientHeight;
      const size = Math.min(containerWidth, containerHeight);
      if (size <= 0) return;

      const dpr = window.devicePixelRatio || 1;
      canvas.width = size * dpr;
      canvas.height = size * dpr;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.scale(dpr, dpr);

      const centerX = size / 2;
      const centerY = size / 2;
      const radius = size * 0.36;
      ctx.clearRect(0, 0, size, size);

      const dimensions = CHAR_KEYS.slice(0, 8);
      const count = dimensions.length;
      const angles = Array.from({ length: count }, (_, i) => (i * 2 * Math.PI / count) - Math.PI / 2);
      const cosAngles = angles.map(Math.cos);
      const sinAngles = angles.map(Math.sin);

      // 绘制网格
      ctx.strokeStyle = 'rgba(168, 199, 250, 0.2)';
      ctx.fillStyle = 'rgba(168, 199, 250, 0.05)';
      ctx.lineWidth = 1 / dpr;

      for (let level = 1; level <= 5; level++) {
        const r = radius * level / 5;
        ctx.beginPath();
        for (let i = 0; i < count; i++) {
          const x = centerX + r * cosAngles[i];
          const y = centerY + r * sinAngles[i];
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.closePath();
        ctx.stroke();
        if (level === 5) ctx.fill();
      }

      // 绘制从中心到顶点的射线
      ctx.strokeStyle = 'rgba(168, 199, 250, 0.3)';
      ctx.lineWidth = 1 / dpr;
      for (let i = 0; i < count; i++) {
        const x = centerX + radius * cosAngles[i];
        const y = centerY + radius * sinAngles[i];
        ctx.moveTo(centerX, centerY);
        ctx.lineTo(x, y);
      }
      ctx.stroke();

      // 处理数值
      const processedValues = dimensions.map(dim => Math.max(0, ch[dim] ?? 0));
      const maxVal = Math.max(...processedValues, 100); 

      // 绘制雷达区域
      ctx.fillStyle = 'rgba(168, 199, 250, 0.3)';
      ctx.strokeStyle = '#a8c7fa';
      ctx.lineWidth = 2 / dpr;

      ctx.beginPath();
      for (let i = 0; i < count; i++) {
        const r = radius * (processedValues[i] / maxVal);
        const x = centerX + r * cosAngles[i];
        const y = centerY + r * sinAngles[i];
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.closePath();
      ctx.fill();
      ctx.stroke();

      // 绘制标签
      ctx.fillStyle = '#7a8499';
      const fontSize = Math.min(14, size * 0.065);
      ctx.font = \`\${fontSize}px "\${fontFamily('--font-gothic')}"\`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';

      const labelOffset = radius + Math.min(20, size * 0.075);
      for (let i = 0; i < count; i++) {
        const x = centerX + labelOffset * cosAngles[i];
        const y = centerY + labelOffset * sinAngles[i];
        ctx.fillText(dimensions[i], x, y);
      }
    }

    // 在title显示溢出的内容
    function updateTitlesForOverflow(selectors) {
      const selectorList = Array.isArray(selectors) ? selectors : [selectors];
      selectorList.forEach(selector => {
        const elements = document.querySelectorAll(selector);
        elements.forEach(el => {
          if (el.dataset.editing === '1') return;
          const fullText = el.dataset.value || el.textContent || '';
          if (el.scrollWidth > el.clientWidth) {
            el.setAttribute('title', fullText); 
          } else {
            el.removeAttribute('title');       
          }
        });
      });
    }

    // 滚动条位置记忆与恢复
    function saveScrollPositions() {
      document.querySelectorAll('[scroll-memory-id]').forEach(el => {
        const id = el.getAttribute('scroll-memory-id');
        if (id && el.offsetParent !== null) {
          const scrollHeight = el.scrollHeight - el.clientHeight;
          const scrollWidth = el.scrollWidth - el.clientWidth;
          state.scrollPositions[id] = {
              topRatio: scrollHeight > 0 ? el.scrollTop / scrollHeight : 0,
              leftRatio: scrollWidth > 0 ? el.scrollLeft / scrollWidth : 0
          };
        }
      });
    }
    function restoreScrollPositions() {
      if (!state.scrollPositions) return;
      requestAnimationFrame(() => {
        document.querySelectorAll('[scroll-memory-id]').forEach(el => {
          if (el.offsetParent === null) return;
          const id = el.getAttribute('scroll-memory-id');
          if (id && state.scrollPositions[id]) {
            const scrollHeight = el.scrollHeight - el.clientHeight;
            const scrollWidth = el.scrollWidth - el.clientWidth;
            el.scrollTop = state.scrollPositions[id].topRatio * scrollHeight;
            el.scrollLeft = state.scrollPositions[id].leftRatio * scrollWidth;
          }
        });
      });
    }

    // 技能搜索结果过滤
    function filterSkillsByKeyword() {
      const keyword = (state.searchKeyword || '').trim().toLowerCase();
      document.querySelectorAll('.sheet__skill-item').forEach(el => {
        const nameEl = el.querySelector('.sheet__skill-name');
        if (nameEl) {
          const text = nameEl.dataset.rollTarget.toLowerCase();
          el.classList.toggle('filter-hidden', keyword !== '' && !text.includes(keyword));
        }
      });
    }

    // ==================== 通信桥接 ====================
    const state = {
      windowId: null,
      rollDispatchMode: 'template',
      activeSkillCategory: null,
      character: {},
      charInfo: {},
      radarPending: false,
      scrollPositions: {},
      searchKeyword: ''
    };

    function postEvent(action, payload) {
      if (!state.windowId) return;
      window.parent.postMessage({
        type: 'SEALCHAT_EVENT',
        version: 1,
        windowId: state.windowId,
        action: action,
        payload: payload
      }, '*');
    }

    window.sealchat = {
      onUpdate: function(cb) {
        window.addEventListener('message', e => {
          if (e.source !== window.parent) return;
          if (e.data && e.data.type === 'SEALCHAT_UPDATE') {
            state.windowId = e.data.payload.windowId;
            cb(e.data.payload);
          }
        });
      },
      setRollDispatchMode: function(mode) {
        state.rollDispatchMode = mode === 'template' ? 'template' : 'default';
      },
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', {
          roll: {
            template: template,
            label: label || '',
            args: args || {},
            dispatchMode: state.rollDispatchMode
          }
        });
      },
      updateAttrs: function(attrs) {
        postEvent('UPDATE_ATTRS', { attrs: attrs });
      }
    };

    // ==================== 渲染辅助函数 ====================
    function renderHeader(avatarUrl, name, foundInfo) {
      const renderId = 1;
      const age = foundInfo['年龄'] || '';
      const gender = foundInfo['性别'] || '';
      const hometown = foundInfo['故乡'] || '';
      const address = foundInfo['住址'] || '';
      const appearance = foundInfo['外貌描述'] || '';
      const era = foundInfo['时代'] || '1920s';   

      return \`
        <div class="sheet__header">
          <div class="sheet__avatar">\${avatarUrl ? \`<img src="\${escapeHtml(avatarUrl)}">\` : (name || '?').charAt(0)}</div>
          <div class="sheet__info">
            <div class="sheet__name-row">
              <div class="sheet__name">\${escapeHtml(name || '未命名')}</div>
              <button type="button" class="sheet__era-toggle toggle-value" data-attr="时代" data-value="\${escapeHtml(era)}">\${escapeHtml(era)}</button>
            </div>

            <div class="sheet__basic">
              <div class="sheet__basic-item">
                <span class="sheet__basic-label">年龄</span>
                <button type="button" class="sheet__basic-value editable-value \${age ? '' : 'empty'}" data-attr="年龄" data-value="\${escapeHtml(age)}">\${escapeHtml(age || '未设定')}</button>
              </div>
              <div class="sheet__basic-item">
                <span class="sheet__basic-label">性别</span>
                <button type="button" class="sheet__basic-value editable-value \${gender ? '' : 'empty'}" data-attr="性别" data-value="\${escapeHtml(gender)}">\${escapeHtml(gender || '未设定')}</button>
              </div>
              <div class="sheet__basic-item">
                <span class="sheet__basic-label">故乡</span>
                <button type="button" class="sheet__basic-value editable-value \${hometown ? '' : 'empty'}" data-attr="故乡" data-value="\${escapeHtml(hometown)}">\${escapeHtml(hometown || '未设定')}</button>
              </div>
              <div class="sheet__basic-item">
                <span class="sheet__basic-label">住址</span>
                <button type="button" class="sheet__basic-value editable-value \${address ? '' : 'empty'}" data-attr="住址" data-value="\${escapeHtml(address)}">\${escapeHtml(address || '未设定')}</button>
              </div>
            </div>

            <div class="sheet__appearance">
              <span class="sheet__appearance-label">外貌描述</span>
              <button type="button" class="editable-textarea h60 \${appearance ? '' : 'empty'}" data-attr="外貌描述" data-value="\${escapeHtml(appearance)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000}">\${escapeHtml(appearance || '点击添加外貌描述...')}</span></button>
            </div>
          </div>
        </div>
      \`;
    }

    function renderCharAndStatus(foundChar, foundStatus, foundInfo, cthulhuMythValue) {
      const renderId = 2;
      const strVal = foundChar['力量'] || 0;
      const conVal = foundChar['体质'] || 0;
      const sizVal = foundChar['体型'] || 0;
      const dexVal = foundChar['敏捷'] || 0;
      const powVal = foundChar['意志'] || 0;

      const defaultHPMax = Math.floor((conVal + sizVal) / 10);
      const defaultMPMax = Math.floor(powVal / 5);
      const defaultSANMax = 99 - cthulhuMythValue;

      const hpMax = foundStatus['生命值上限'] ?? defaultHPMax;
      const mpMax = foundStatus['魔法值上限'] ?? defaultMPMax;
      const sanMax = foundStatus['理智上限'] ?? defaultSANMax;

      const hpVal = foundStatus['生命值'] ?? hpMax;
      const mpVal = foundStatus['魔法值'] ?? mpMax;
      const sanVal = foundStatus['理智'] ?? sanMax;

      const hpDesc = \`默认上限: (体质+体型)÷10 = \${conVal + sizVal}÷10 = \${defaultHPMax}\`;
      const mpDesc = \`默认上限: 意志÷5 = \${powVal}÷5 = \${defaultMPMax}\`;
      const sanDesc = \`默认上限: 99-克苏鲁神话 = 99-\${cthulhuMythValue} = \${defaultSANMax}\`;

      const hpState = foundInfo['健康状态'] || '健康'; 
      const sanState = foundInfo['精神状态'] || '神志清醒';

      const ageValue = toNumber(foundInfo['年龄'], 25);
      const damageBonus = calculateDamageBonus(strVal, sizVal);
      const build = calculateBuild(strVal, sizVal);
      const move = calculateMove(ageValue, dexVal, strVal, sizVal);
      const magicRecovery = Math.ceil(powVal / 100);
      const armorVal = foundStatus['护甲'] || 0;

      const CharCount = CHAR_KEYS.slice(0, 8).reduce((sum, key) => sum + (foundChar[key] || 0), 0);
      const extraNote = foundInfo['备注'] || '';

      return \`
        <div class="sheet__title"><span class="sheet__title-icon"></span>属性</div>
        <div class="sheet__status-grid">
          <div class="sheet__status-item st-hp">
            <div class="sheet__status-label-row">
              <span class="sheet__status-label">生命</span>
              <button type="button" class="sheet__status-toggle toggle-value" data-attr="健康状态" data-value="\${escapeHtml(hpState)}">\${escapeHtml(hpState)}</button>
            </div>
            <div class="sheet__status-row">
              <button type="button" class="sheet__status-val editable-value" data-attr="生命值" data-value="\${hpVal}">\${hpVal}</button>
              <button type="button" class="sheet__status-max editable-value" title="\${escapeHtml(hpDesc)}" data-attr="生命值上限" data-value="\${hpMax}">\${hpMax}</button>
            </div>
          </div>
          <div class="sheet__status-item st-mp">
            <div class="sheet__status-label-row">
              <span class="sheet__status-label">魔力</span>
            </div>
            <div class="sheet__status-row">
              <button type="button" class="sheet__status-val editable-value" data-attr="魔法值" data-value="\${mpVal}">\${mpVal}</button>
              <button type="button" class="sheet__status-max editable-value" title="\${escapeHtml(mpDesc)}" data-attr="魔法值上限" data-value="\${mpMax}">\${mpMax}</button>
            </div>
          </div>
          <div class="sheet__status-item st-san">
            <div class="sheet__status-label-row">
              <span class="sheet__status-label">理智</span>
              <button type="button" class="sheet__status-toggle toggle-value" data-attr="精神状态" data-value="\${escapeHtml(sanState)}">\${escapeHtml(sanState)}</button>
            </div>
            <div class="sheet__status-row">
              <button type="button" class="sheet__status-val editable-value" data-attr="理智" data-value="\${sanVal}">\${sanVal}</button>
              <button type="button" class="sheet__status-max editable-value" title="\${escapeHtml(sanDesc)}" data-attr="理智上限" data-value="\${sanMax}">\${sanMax}</button>
            </div>
          </div>
        </div>

        <div class="sheet__combat-grid">
          <div class="sheet__combat-item">
            <span class="sheet__combat-label">伤害加值</span>
            <span class="sheet__combat-val">\${damageBonus}</span>
          </div>
          <div class="sheet__combat-item">
            <span class="sheet__combat-label">体格</span>
            <span class="sheet__combat-val">\${build}</span>
          </div>
          <div class="sheet__combat-item">
            <span class="sheet__combat-label">移动速度</span>
            <span class="sheet__combat-val">\${move}</span>
          </div>
          <div class="sheet__combat-item">
            <span class="sheet__combat-label">每轮移动</span>
            <span class="sheet__combat-val">\${move * 5}码</span>
          </div>
          <div class="sheet__combat-item">
            <span class="sheet__combat-label">魔力恢复</span>
            <span class="sheet__combat-val">\${magicRecovery}/h</span>
          </div>
          <div class="sheet__combat-item st-ar">
            <span class="sheet__combat-label">护甲</span>
            <button type="button" class="sheet__combat-val editable-value" data-attr="护甲" data-value="\${armorVal}">\${armorVal}</button>
          </div>
        </div>

        <div class="sheet__char-radar-extra-grid">
          <div class="sheet__char-grid">
            \${CHAR_KEYS.map(k => {
              const val = foundChar[k] || 0;
              const desc = CHAR_DESCRIPTIONS[k] || k;
              return \`
                <div class="sheet__char-item">
                  <button type="button" class="sheet__char-label" title="\${escapeHtml(desc)}" data-roll-target="\${k}" data-roll-value="\${val}">\${k}</button>
                  <button type="button" class="sheet__char-val editable-value" data-attr="\${k}" data-value="\${val}">\${val}</button>
                </div>
              \`;
            }).join('')}
          </div>
          <div class="sheet__radar-item">
            <canvas id="canvasCharRadar"></canvas>
            <div class="sheet__char-total">属性总值(除幸运): <span>\${CharCount}</span></div>
          </div>
          <div class="sheet__extranote">
            <span class="sheet__extranote-label">备注<span class="sheet__status-desc-icon" title="\${escapeHtml(ADJUSTMENT_DESCRIPTIONS)}"></span></span>
            <button type="button" class="editable-textarea h60 \${extraNote ? '' : 'empty'}" data-attr="备注" data-value="\${escapeHtml(extraNote)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000}">\${escapeHtml(extraNote || '点击添加备注...')}</span></button>
          </div>
        </div>
      \`;
    }

    function renderJobSelect(currentJob) {
      const renderId = 3;
      const creditArr = Array.isArray(currentJob.credit) ? currentJob.credit : [];
      const creditRange = creditArr.length === 2 ? \`\${creditArr[0]} - \${creditArr[1]}\` : '未知';
      const skillsArr = Array.isArray(currentJob.Skills) ? currentJob.Skills : [];
      const Skills = skillsArr.length ? skillsArr.join('、') : '未知';
      const SkillPoint = currentJob.SkillPoint || '未知';
      const SkillExt = currentJob.SkillExt || '无';
      const jobDesc = currentJob.desc || '请从上方下拉菜单中选择一个职业';

      return \`
        <div class="sheet__title"><span class="sheet__title-icon"></span>职业</div>
        <div class="sheet__job">
          <select class="sheet__job-select \${currentJob.name ? 'selected' : ''}" data-attr="职业">
            <option value="">请选择职业...</option>
            \${JOB_DATA.map(job => {
                const selected = (currentJob && job.name === currentJob.name) ? 'selected' : '';
                return \`<option value="\${escapeHtml(job.name)}" \${selected}>\${escapeHtml(job.name)}</option>\`;
              }).join('')}
          </select>

          <div class="sheet__job-detail-grid">
            <div class="sheet__job-detail-item">
              <span class="sheet__job-detail-label">信用范围</span>
              <span class="sheet__job-detail-value">\${escapeHtml(creditRange)}</span>
            </div>
            <div class="sheet__job-detail-item">
              <span class="sheet__job-detail-label">本职技能点</span>
              <span class="sheet__job-detail-value">\${escapeHtml(SkillPoint)}</span>
            </div>
            <div class="sheet__job-detail-item">
              <span class="sheet__job-detail-label">本职技能</span>
              <span class="sheet__job-detail-value">\${escapeHtml(Skills)}</span>
            </div>
            <div class="sheet__job-detail-item">
              <span class="sheet__job-detail-label">备注</span>
              <span class="sheet__job-detail-value">\${escapeHtml(SkillExt)}</span>
            </div>
          </div>

          <div class="sheet__job-desc" scroll-memory-id="\${renderId * 1000}">\${escapeHtml(jobDesc)}</div>
        </div>
      \`;
    }

    function renderSkills(currentJob, foundChar, foundSkills) {
      const renderId = 4;
      const skillGrowthMap = (state.charInfo['技能成长'] && typeof state.charInfo['技能成长'] === 'object') ? state.charInfo['技能成长'] : {};
      const skillOccupationMap = (state.charInfo['技能职业'] && typeof state.charInfo['技能职业'] === 'object') ? state.charInfo['技能职业'] : {};
      const skillInterestMap = (state.charInfo['技能兴趣'] && typeof state.charInfo['技能兴趣'] === 'object') ? state.charInfo['技能兴趣'] : {};

      // 计算技能点
      let occupationSkillPoint = 0;
      if (currentJob && currentJob.SkillPoint) {
        occupationSkillPoint = calculateSkillPoint(currentJob.SkillPoint, foundChar);
      }
      const interestSkillPoint = (foundChar['智力'] || 0) * 2;
      const totalGrowth = Object.values(skillGrowthMap).reduce((s, v) => s + (v || 0), 0);
      const totalOccupation = Object.values(skillOccupationMap).reduce((s, v) => s + (v || 0), 0);
      const totalInterest = Object.values(skillInterestMap).reduce((s, v) => s + (v || 0), 0);

      // 选项卡列表
      const categories = ['收藏夹', ...Object.keys(SKILL_CATEGORIES)];
      if (!state.activeSkillCategory || !categories.includes(state.activeSkillCategory)) {
        state.activeSkillCategory = categories[1];
      }

      // 技能项辅助函数
      const generateSkillItem = (skillObj, isFavoriteCat) => {
        const { key: skillKey, displayName, baseSkill, subSkill, description } = skillObj;
        const baseValue = getSkillBaseValue(baseSkill, subSkill, foundChar);
        const growthVal = skillGrowthMap[skillKey] || 0;
        const occupationVal = skillOccupationMap[skillKey] || 0;
        const interestVal = skillInterestMap[skillKey] || 0;
        const totalVal = foundSkills[skillKey] ?? (baseValue + growthVal + occupationVal + interestVal);
        const halfVal = Math.floor(totalVal / 2);
        const fifthVal = Math.floor(totalVal / 5);
        const isFavorite = isFavoriteCat || (Array.isArray(state.charInfo['收藏夹']) && state.charInfo['收藏夹'].includes(skillKey));
        const favoriteActive = isFavorite ? 'active' : '';

        return \`
          <div class="sheet__skill-item">
            <div class="sheet__skill-label">
                <button type="button" class="sheet__skill-name" data-roll-target="\${escapeHtml(displayName)}" data-roll-value="\${totalVal}" title="\${escapeHtml(description)}">\${escapeHtml(displayName)}</button>
                <button type="button" class="sheet__skill-favorite \${favoriteActive}" data-skill="\${escapeHtml(skillKey)}" title="\${isFavorite ? '取消收藏' : '收藏'}"></button>
            </div>
            <div class="sheet__skill-values-row">
              <div class="sheet__skill-value-col val-base">
                <span class="sheet__skill-value-label">初始</span>
                <span class="sheet__skill-value">\${baseValue}</span>
              </div>
              <div class="sheet__skill-value-col val-interest">
                <span class="sheet__skill-value-label">兴趣</span>
                <button type="button" class="sheet__skill-value editable-value" data-attr="\${escapeHtml(skillKey)}_兴趣" data-value="\${interestVal}">\${interestVal}</button>
              </div>
              <div class="sheet__skill-value-col val-occupation">
                <span class="sheet__skill-value-label val-occupation">职业</span>
                <button type="button" class="sheet__skill-value editable-value" data-attr="\${escapeHtml(skillKey)}_职业" data-value="\${occupationVal}">\${occupationVal}</button>
              </div>
              <div class="sheet__skill-value-col val-growth">
                <span class="sheet__skill-value-label">成长</span>
                <button type="button" class="sheet__skill-value editable-value" data-attr="\${escapeHtml(skillKey)}_成长" data-value="\${growthVal}">\${growthVal}</button>
              </div>
              <div class="sheet__skill-value-col val-total">
                <span class="sheet__skill-value-label">总值</span>
                <button type="button" class="sheet__skill-value editable-value" data-attr="\${escapeHtml(skillKey)}" data-value="\${totalVal}" data-existed="\${foundSkills[skillKey] !== undefined}">\${totalVal}</button>
              </div>
              <div class="sheet__skill-value-col val-half">
                <span class="sheet__skill-value-label">½ / ⅕</span>
                <span class="sheet__skill-value">\${halfVal} / \${fifthVal}</span>
              </div> 
            </div>
          </div>
        \`;
      };

      return \`
        <div class="sheet__title"><span class="sheet__title-icon"></span>技能</div>
        <div class="sheet__skill-info">
          <div class="sheet__skill-points-box">
            <div class="sheet__skill-points-item">
              <span class="sheet__skill-points-label">兴趣技能点</span>
              <span class="sheet__skill-points-value">\${interestSkillPoint}</span>
            </div>
            <div class="sheet__skill-points-item">
              <span class="sheet__skill-points-label">本职技能点</span>
              <span class="sheet__skill-points-value">\${occupationSkillPoint}</span>
            </div>
            <div class="sheet__skill-points-item">
              <span class="sheet__skill-points-label">已加兴趣</span>
              <span class="sheet__skill-points-value">\${totalInterest}</span>
            </div>
            <div class="sheet__skill-points-item">
              <span class="sheet__skill-points-label">已加职业</span>
              <span class="sheet__skill-points-value">\${totalOccupation}</span>
            </div>
            <div class="sheet__skill-points-item">
              <span class="sheet__skill-points-label">已加成长</span>
              <span class="sheet__skill-points-value">\${totalGrowth}</span>
            </div>
          </div>

          <button type="button" class="sheet__skill-lock toggle-value" data-attr="技能总值锁定" data-value="\${state.charInfo['技能总值锁定'] === 'true' ? 'true' : 'false'}" title="锁定后，技能总值不会因成长/兴趣/职业点数或属性等的修改而发生间接变动">总值锁定</button>
        </div>

        <label class="sheet__skill-search">
          <input type="text" class="sheet__skill-search-input" placeholder="搜索技能..." value="\${escapeHtml(state.searchKeyword || '')}">
          <span class="sheet__skill-search-icon"></span>
        </label>

        <div class="sheet__skill-tabs">
          \${categories.map(cat => {
            const active = cat === state.activeSkillCategory ? 'active' : '';
            const displayText = cat === '收藏夹' ? '' : cat;
            return \`<button class="sheet__skill-tab \${active}" data-category="\${cat}">\${displayText}</button>\`;
          }).join('')}
        </div>

        <div class="sheet__skill-panels">
          \${categories.map((cat, index) => {
            const active = cat === state.activeSkillCategory ? 'active' : '';
            const isFavoriteCat = cat === '收藏夹';
            const favArr = state.charInfo['收藏夹'];
            const skillObjs = isFavoriteCat ? (Array.isArray(favArr) ? favArr.map(key => SKILLS.byKey[key]).filter(Boolean) : []) : (SKILLS.byCategory[cat] || []);
            if (isFavoriteCat && skillObjs.length === 0) {
              return \`
                <div class="sheet__skill-panel \${active}" data-category="\${cat}" scroll-memory-id="\${renderId * 1000 + 999}">
                  <div class="loading-empty-msg" >暂无收藏技能</div>
                </div>
              \`;
            }

            return \`
              <div class="sheet__skill-panel \${active}" data-category="\${cat}" scroll-memory-id="\${renderId * 1000 + (isFavoriteCat ? 999 : index)}">
                 \${skillObjs.map(skillObj => generateSkillItem(skillObj, isFavoriteCat)).join('')}
              </div>
            \`;
          }).join('')}
        </div>
      \`;
    }

    function renderWeapons(weapons) {
      const renderId = 5;
      const noteSet = new Set();
      const wpList = Array.isArray(weapons) ? weapons : [];
      const rowCount = Math.max(5, ...wpList.map((wp, i) => wp && wp.name ? i + 2 : 0));

      const generateWeaponItem = (idx) => {
        const wp = (wpList[idx]) || { name: '', damage: '', dmg: '' };
        const savedName = wp.name || '';
        const displayDamage = wp.damage || (wp.dmg ? wp.dmg.replace(/^&\\(|\\)$/g, '') : '');

        const info = WEAPON_MAP[savedName] || {};
        const weapon = info.weapon || {};
        if (weapon.note) noteSet.add(\`\${weapon.name}：\${weapon.note}\`);
        const group = WEAPON_DATA[info.type];
        if (group && group.note) noteSet.add(\`\${info.type}：\${group.note}\`);

        const skill = weapon.skill || '';
        const penetration = weapon.penetration || '';
        const range = weapon.range || '';
        const rateOfFire = weapon.rateOfFire || '';
        const ammoCapacity = weapon.ammoCapacity || '';
        const malfunction = weapon.malfunction || '';

        return \`
          <tr class="sheet__weapons-row" data-weapon-idx="\${idx}">
            <td class="sheet__weapon-item">
              <select class="sheet__weapon-select \${savedName ? 'selected' : ''}" data-attr="武器选择" data-value="\${escapeHtml(savedName)}">
                <option value="">请选择武器...</option>
                \${Object.entries(WEAPON_DATA).filter(([, group]) => group.items.length).map(([type, group]) => \`
                  <optgroup label="\${escapeHtml(type)}">
                    \${group.items.map(item => {
                      const selected = (savedName === item.name) ? 'selected' : '';
                      return \`<option value="\${escapeHtml(item.name)}" \${selected}>\${escapeHtml(item.name)}</option>\`;
                    }).join('')}
                  </optgroup>
                \`).join('')}
              </select>
            </td>
            <td class="sheet__weapon-item"><button type="button" class="sheet__weapon-skill \${skill ? 'clickable' : ''}" data-roll-skill="\${escapeHtml(skill)}" data-roll-damage="\${escapeHtml(displayDamage)}" \${savedName ? '' : ' tabindex="-1"'}>\${escapeHtml(skill)}</button></td>
            <td class="sheet__weapon-item"><button type="button" class="sheet__weapon-damage editable-value" data-attr="武器伤害" data-value="\${escapeHtml(displayDamage)}" \${savedName ? '' : ' tabindex="-1"'}>\${escapeHtml(displayDamage)}</button></td>
            <td class="sheet__weapon-item">\${escapeHtml(penetration)}</td>
            <td class="sheet__weapon-item">\${escapeHtml(range)}</td>
            <td class="sheet__weapon-item">\${escapeHtml(rateOfFire)}</td>
            <td class="sheet__weapon-item">\${escapeHtml(ammoCapacity)}</td>
            <td class="sheet__weapon-item">\${escapeHtml(malfunction)}</td>
          </tr>
        \`;
      };

      return \`
        <div class="sheet__title"><span class="sheet__title-icon"></span>武器</div>
        <div class="sheet__weapons" scroll-memory-id="\${renderId * 1000}">
          <table class="sheet__weapons-table">
            <thead>
              <tr class="sheet__weapons-header">
                <th class="sheet__weapon-item">名称</th>
                <th class="sheet__weapon-item">技能</th>
                <th class="sheet__weapon-item">伤害</th>
                <th class="sheet__weapon-item">贯穿</th>
                <th class="sheet__weapon-item">射程</th>
                <th class="sheet__weapon-item">每轮</th>
                <th class="sheet__weapon-item">装弹</th>
                <th class="sheet__weapon-item">故障值</th>
              </tr>
            </thead>
            <tbody>
              \${Array.from({ length: rowCount }, (_, i) => generateWeaponItem(i)).join('')}
            </tbody>
          </table>
          <div class="sheet__weapons-note">* 星号代表有额外说明<span class="sheet__status-desc-icon" title="\${escapeHtml(Array.from(noteSet).join('\\n'))}"></span></div>
        </div>
      \`;
    }
    
    function renderAssetsAndBackground(foundInfo, credit) {
      const renderId = 6;
      const era = foundInfo['时代'] || '1920s';
      const { livingLevel, matchedCr, unit, note } = getLivingLevel(era, credit);
      let infoTitle = \`\${era}\${unit ? \` (\${unit})\` : ''}\\n现金：\${calculateAssetValue(livingLevel.cash, matchedCr)}\\n其他资产：\${calculateAssetValue(livingLevel.assets, matchedCr)}\\n消费水平：\${calculateAssetValue(livingLevel.spending, matchedCr)}\`;
      if (note) {
        infoTitle += \`\\n\${note}\`;
      }

      const assetDesc = foundInfo['资产说明'] || '';
      const items = foundInfo['随身物品'] || '';
      const thought = foundInfo['思想与信念'] || '';
      const personality = foundInfo['性格特点'] || '';
      const importantPeople = foundInfo['重要之人'] || '';
      const meaningfulPlace = foundInfo['意义非凡之地'] || '';
      const preciousItem = foundInfo['宝贵之物'] || '';
      const wound = foundInfo['伤口和疤痕'] || '';
      const phobia = foundInfo['恐惧症和狂躁症'] || '';
      const personalDetail = foundInfo['个人详细描述'] || '';
      
      return \`
        <div class="sheet__assets-background-grid">
          <div class="sheet__assets-background-left">
            <div class="sheet__assets">
              <div class="sheet__title left"><span class="sheet__title-icon"></span>资产情况</div>
              <div class="sheet__credit-row">
                <div class="sheet__credit-col">
                  <span class="sheet__credit-label">信用评级</span>
                  <span class="sheet__credit-value">\${credit}</span>
                </div>
                <div class="sheet__credit-col">
                  <span class="sheet__credit-label">生活水平</span>
                  <span class="sheet__status-desc-icon" title="\${escapeHtml(infoTitle)}"></span>
                  <span class="sheet__credit-value">\${escapeHtml(livingLevel.name)}</span>
                </div>
              </div>
              <button type="button" class="editable-textarea h100 inblock-top \${assetDesc ? '' : 'empty'}" data-attr="资产说明" data-value="\${escapeHtml(assetDesc)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000}">\${escapeHtml(assetDesc || '点击添加资产说明...')}</span></button>
            </div>
   
            <div class="sheet__items">
              <div class="sheet__title left"><span class="sheet__title-icon"></span>随身物品</div>
              <span class="sheet__items-border"></span>
              <button type="button" class="editable-textarea h300 inblock \${items ? '' : 'empty'}" data-attr="随身物品" data-value="\${escapeHtml(items)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 1}">\${escapeHtml(items || '点击添加随身物品...')}</span></button>
            </div>
          </div>

          <div class="sheet__assets-background-right">
            <div class="sheet__title right"><span class="sheet__title-icon"></span>背景故事</div>
            <div class="sheet__background">
              <div class="sheet__background-item">
                <span class="sheet__background-label">思想与信念</span>
                <button type="button" class="sheet__background-value editable-value \${thought ? '' : 'empty'}" data-attr="思想与信念" data-value="\${escapeHtml(thought)}">\${escapeHtml(thought || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">性格特点</span>
                <button type="button" class="sheet__background-value editable-value \${personality ? '' : 'empty'}" data-attr="性格特点" data-value="\${escapeHtml(personality)}">\${escapeHtml(personality || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">重要之人</span>
                <button type="button" class="sheet__background-value editable-value \${importantPeople ? '' : 'empty'}" data-attr="重要之人" data-value="\${escapeHtml(importantPeople)}">\${escapeHtml(importantPeople || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">意义非凡之地</span>
                <button type="button" class="sheet__background-value editable-value \${meaningfulPlace ? '' : 'empty'}" data-attr="意义非凡之地" data-value="\${escapeHtml(meaningfulPlace)}">\${escapeHtml(meaningfulPlace || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">宝贵之物</span>
                <button type="button" class="sheet__background-value editable-value \${preciousItem ? '' : 'empty'}" data-attr="宝贵之物" data-value="\${escapeHtml(preciousItem)}">\${escapeHtml(preciousItem || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">伤口和疤痕</span>
                <button type="button" class="sheet__background-value editable-value \${wound ? '' : 'empty'}" data-attr="伤口和疤痕" data-value="\${escapeHtml(wound)}">\${escapeHtml(wound || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">恐惧症和狂躁症</span>
                <button type="button" class="sheet__background-value editable-value \${phobia ? '' : 'empty'}" data-attr="恐惧症和狂躁症" data-value="\${escapeHtml(phobia)}">\${escapeHtml(phobia || '未设定')}</button>
              </div>
              <div class="sheet__background-item">
                <span class="sheet__background-label">个人详细描述</span>
                <button type="button" class="editable-textarea h100 media-h130 inblock-top \${personalDetail ? '' : 'empty'}" data-attr="个人详细描述" data-value="\${escapeHtml(personalDetail)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 2}">\${escapeHtml(personalDetail || '点击添加个人详细描述...')}</span></button>
              </div>
            </div>
          </div>
        </div>
      \`;
    }

    function renderMythAndExperience(foundInfo) {
      const renderId = 7;
      const mythItems = foundInfo['神话物品'] || '';
      const traits = foundInfo['特质与能力'] || '';
      const spells = foundInfo['法术'] || '';
      const contact = foundInfo['第三类接触'] || '';
      const exp = foundInfo['调查员经历'] || '';
      const partners = foundInfo['调查员伙伴'] || '';

      return \`
        <div class="sheet__myth-grid">
          <div class="sheet__myth-item">
            <div class="sheet__title small left"><span class="sheet__title-icon"></span>神话物品</div>
            <span class="sheet__items-border"></span>
            <button type="button" class="editable-textarea h160 inblock \${mythItems ? '' : 'empty'}" data-attr="神话物品" data-value="\${escapeHtml(mythItems)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000}">\${escapeHtml(mythItems || '点击添加神话物品...')}</span></button>
          </div>
          <div class="sheet__myth-item">
            <div class="sheet__title small right"><span class="sheet__title-icon"></span>特质与能力</div>
            <span class="sheet__items-border"></span>
            <button type="button" class="editable-textarea h160 inblock \${traits ? '' : 'empty'}" data-attr="特质与能力" data-value="\${escapeHtml(traits)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 1}">\${escapeHtml(traits || '点击添加特质与能力...')}</span></button>
          </div>
        </div>
        <div class="sheet__myth-grid">
          <div class="sheet__myth-item">
            <div class="sheet__title small left"><span class="sheet__title-icon"></span>法术</div>
            <span class="sheet__items-border"></span>
            <button type="button" class="editable-textarea h160 inblock \${spells ? '' : 'empty'}" data-attr="法术" data-value="\${escapeHtml(spells)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 2}">\${escapeHtml(spells || '点击添加法术...')}</span></button>
          </div>
          <div class="sheet__myth-item">
            <div class="sheet__title small right"><span class="sheet__title-icon"></span>第三类接触</div>
            <span class="sheet__items-border"></span>
            <button type="button" class="editable-textarea h160 inblock \${contact ? '' : 'empty'}" data-attr="第三类接触" data-value="\${escapeHtml(contact)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 3}">\${escapeHtml(contact || '点击添加第三类接触记录...')}</span></button>
          </div>
        </div>
        <div class="sheet__myth-grid">
          <div class="sheet__myth-item">
            <div class="sheet__title small left"><span class="sheet__title-icon"></span>调查员经历</div>
            <span class="sheet__items-border"></span>
            <button type="button" class="editable-textarea h160 inblock \${exp ? '' : 'empty'}" data-attr="调查员经历" data-value="\${escapeHtml(exp)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 4}">\${escapeHtml(exp || '点击添加调查员经历...')}</span></button>
          </div>
          <div class="sheet__myth-item">
            <div class="sheet__title small right"><span class="sheet__title-icon"></span>调查员伙伴</div>
            <span class="sheet__items-border"></span>
            <button type="button" class="editable-textarea h160 inblock \${partners ? '' : 'empty'}" data-attr="调查员伙伴" data-value="\${escapeHtml(partners)}"><span class="editable-textarea-content" scroll-memory-id="\${renderId * 1000 + 5}">\${escapeHtml(partners || '点击添加调查员伙伴...')}</span></button>
          </div>
        </div>
      \`;
    }

    function renderOtherAttrs(otherAttrs) {
      const renderId = 8;
      return \`
        <div class="sheet__title"><span class="sheet__title-icon"></span>其他属性</div>
        <div class="sheet__other-attrs" scroll-memory-id="\${renderId * 1000}">
          \${Object.entries(otherAttrs).map(([key, val]) => \`
            <div class="sheet__other-attr-item">
              <span class="sheet__other-attr-key">\${escapeHtml(key)}:</span>
              <span class="sheet__other-attr-value">\${escapeHtml(val)}</span>
            </div>
          \`).join('')}
        </div>
      \`;
    }

    // ==================== 渲染主函数 ====================
    function render(data) {
      saveScrollPositions();
      const contentEl = document.getElementById('content');
      if (!data || !data.attrs || Object.keys(data.attrs).length === 0) {
        contentEl.innerHTML = '<div class="loading-empty-msg">灵魂之灯尚未点燃<br>等待调查员数据录入...</div>';
        return;
      }
      const { attrs: rawAttrs, avatarUrl = '', name = '未命名' } = data;
      const foundInfo = (rawAttrs[CHAR_INFO_KEY] && typeof rawAttrs[CHAR_INFO_KEY] === 'object')
        ? { ...rawAttrs[CHAR_INFO_KEY] } : {};

      applyCustomData(rawAttrs[CUSTOM_DATA_KEY]);
      if (!(CUSTOM_DATA_KEY in rawAttrs)) {
        sealchat.updateAttrs({ [CUSTOM_DATA_KEY]: CUSTOM_DATA_TEMPLATE });
      }

      const foundChar = {}, foundStatus = {}, foundSkills = {}, otherAttrs = {};
      for (const [key, val] of Object.entries(rawAttrs)) {
        if (key === CHAR_INFO_KEY || key === CUSTOM_DATA_KEY) continue;
        if (CHAR_KEYS.includes(key))      { foundChar[key]   = toNumber(val); continue; }
        if (STATUS_KEYS.includes(key))    { foundStatus[key] = toNumber(val); continue; }
        if (SKILLS.byKey[key] !== undefined) { foundSkills[key] = toNumber(val); continue; }
        const norm = normalizeSkillKey(key);
        if (SKILLS.byKey[norm] !== undefined) { foundSkills[norm] = toNumber(val); continue; }
        otherAttrs[key] = toString(val);
      }
      for (const k of Object.keys(otherAttrs)) { if (/^dmg\\d+$/.test(k) || k === '$ver') delete otherAttrs[k]; }

      state.character = foundChar;
      state.charInfo = foundInfo;

      const currentJob = JOB_MAP[foundInfo['职业']] || {};
      const weapons = foundInfo['武器'] || [];
      const cthulhu = foundSkills['克苏鲁神话'] || 0;
      const credit  = foundSkills['信用评级'] || 0;

      let html = '<div class="sheet">';
        html += renderHeader(avatarUrl, name, foundInfo);
        html += renderCharAndStatus(foundChar, foundStatus, foundInfo, cthulhu);
        html += renderJobSelect(currentJob);
        html += renderSkills(currentJob, foundChar, foundSkills);
        html += renderWeapons(weapons);
        html += renderAssetsAndBackground(foundInfo, credit);
        html += renderMythAndExperience(foundInfo);
        html += renderOtherAttrs(otherAttrs);
      html += '</div>';

      contentEl.innerHTML = html;
      drawRadar();
      updateTitlesForOverflow(['.sheet__basic-value', '.sheet__background-value']);
      filterSkillsByKeyword();
      restoreScrollPositions();
    }

    // ==================== 交互函数 ====================
    // 单行输入框策略
    function getEditorStrategy(attrKey) {
      // 技能加值
      const skillSuffix = ['_兴趣', '_职业', '_成长'].find(s => attrKey.endsWith(s));
      const suffixStruct = { '_兴趣': '技能兴趣', '_职业': '技能职业', '_成长': '技能成长' };
      if (skillSuffix) {
        const structKey = suffixStruct[skillSuffix];
        return {
          inputType: 'number',
          parseValue: (raw) => toNumber(raw, undefined),
          getContext: (target) => {
            const skillKey = attrKey.slice(0, -skillSuffix.length);
            const totalEl  = target.closest('.sheet__skill-item')?.querySelector('.val-total .sheet__skill-value');
            return {
              skillKey,
              currentTotal: toNumber(totalEl?.dataset.value || totalEl?.textContent),
              isPersisted:  totalEl?.dataset.existed === 'true',
              structKey
            };
          },
          buildPatch: (finalVal, ctx) => {
            const raw = state.charInfo[ctx.structKey];
            const groupMap = (raw && typeof raw === 'object') ? { ...raw } : {};
            const oldVal   = groupMap[ctx.skillKey] || 0;
            if (finalVal === 0) delete groupMap[ctx.skillKey];
            else groupMap[ctx.skillKey] = finalVal;
            const patch = { [CHAR_INFO_KEY]: { ...state.charInfo, [ctx.structKey]: groupMap } };
            if (state.charInfo['技能总值锁定'] === 'true') {
              if (!ctx.isPersisted) patch[ctx.skillKey] = ctx.currentTotal;
            } else {
              patch[ctx.skillKey] = ctx.currentTotal + (finalVal - oldVal);
            }
            return patch;
          }
        };
      }

      // 角色属性
      if (CHAR_KEYS.includes(attrKey)) {
        return {
          inputType: 'number',
          parseValue: (raw) => toNumber(raw, undefined),
          getContext: () => {
            const depSkills = (SKILLS_DEPEND_ON_ATTR[attrKey] || []).reduce((arr, skillKey) => {
              const skill = SKILLS.byKey[skillKey];
              if (!skill) return arr;
              const totalEl = document.querySelector(
                \`.sheet__skill-name[data-roll-target="\${CSS.escape(skill.displayName)}"]\`
              )?.closest('.sheet__skill-item')?.querySelector('.val-total .sheet__skill-value');
              arr.push({
                skillKey,
                baseSkill: skill.baseSkill,
                subSkill:  skill.subSkill,
                currentTotal: toNumber(totalEl?.dataset.value || totalEl?.textContent),
                isPersisted:  totalEl?.dataset.existed === 'true'
              });
              return arr;
            }, []);
            return { depSkills };
          },
          buildPatch: (finalVal, ctx) => {
            const patch    = { [attrKey]: finalVal };
            const newChar  = { ...state.character, [attrKey]: finalVal };
            ctx.depSkills.forEach(ds => {
              const oldBase = getSkillBaseValue(ds.baseSkill, ds.subSkill, state.character);
              const newBase = getSkillBaseValue(ds.baseSkill, ds.subSkill, newChar);
              const delta   = newBase - oldBase;
              if (state.charInfo['技能总值锁定'] === 'true') {
                if (!ds.isPersisted) patch[ds.skillKey] = ds.currentTotal;
              } else {
                patch[ds.skillKey] = ds.currentTotal + delta;
              }
            });
            return patch;
          }
        };
      }

      // 武器伤害
      if (attrKey === '武器伤害') {
        return {
          inputType: 'text',
          parseValue: toString,
          getContext: (target) => {
            const row = target.closest('[data-weapon-idx]');
            return { idx: toNumber(row?.dataset.weaponIdx) };
          },
          buildPatch: (finalVal, ctx) => {
            const weapons = Array.isArray(state.charInfo['武器']) ? [...state.charInfo['武器']] : [];
            while (weapons.length <= ctx.idx) weapons.push({ name: '', damage: '', dmg: '' });
            weapons[ctx.idx] = { ...weapons[ctx.idx], damage: finalVal, dmg: parseWeaponDamage(finalVal) };
            const patch = { [CHAR_INFO_KEY]: { ...state.charInfo, '武器': weapons } };
            weapons.forEach((wp, i) => { if (wp.dmg) patch['dmg' + (i + 1)] = wp.dmg; });
            return patch;
          }
        };
      }

      // 人物信息
      if (TEXT_FIELDS.includes(attrKey)) {
        return {
          inputType: 'text',
          parseValue: toString,
          buildPatch: finalVal => ({ [CHAR_INFO_KEY]: { ...state.charInfo, [attrKey]: finalVal } })
        };
      }

      // 其余数值
      return {
        inputType: 'number',
        parseValue: (raw) => toNumber(raw, undefined),
        buildPatch: finalVal => ({ [attrKey]: finalVal })
      };
    }

    // 单行输入框
    function openInlineEditor(target) {
      const attrKey = target.dataset.attr;
      if (!attrKey || target.dataset.editing === '1') return;
      if (document.querySelector('[data-editing="1"]')) return;

      const strategy    = getEditorStrategy(attrKey);
      const currentVal  = target.dataset.value || '';
      const originalText = target.textContent;

      const input = document.createElement('input');
      input.className = 'inline-editor';
      input.name  = attrKey;
      input.type  = strategy.inputType;
      input.value = currentVal;

      const ctx = strategy.getContext ? strategy.getContext(target) : null;

      if (strategy.inputType === 'number') {
        input.style.width = '56px';  
        input.style.minWidth = 'auto';
        input.style.textAlign = 'center';
      } else if (target.matches('.sheet__background-value')) {
        input.style.textAlign = 'left';
      } else {
        const targetStyle = getComputedStyle(target);
        input.style.textAlign = targetStyle.textAlign || 'left';
        const measure = (useTargetFont) => {
          const s = useTargetFont ? targetStyle : getComputedStyle(input);
          const c = document.createElement('canvas');
          const ctx2 = c.getContext('2d');
          ctx2.font = s.font;
          const w = ctx2.measureText(input.value || input.placeholder || '').width + 12;
          input.style.width = Math.max(42, Math.min(w, 280)) + 'px';
        };
        measure(true);
        input.addEventListener('input', () => measure(false));
      }

      target.textContent = '';
      target.appendChild(input);
      target.dataset.editing = '1';
      input.focus();
      input.select();

      let closed = false;

      const cleanup = () => {
        input.removeEventListener('keydown', onKeydown);
        input.removeEventListener('blur', onBlur);
      };

      const commit = () => {
        if (closed || target.dataset.editing !== '1') return;
        closed = true;
        cleanup();
        const rawVal  = input.value.trim();
        const finalVal = strategy.parseValue(rawVal);
        if (finalVal === undefined) { closed = false; cancel(); return; }
        target.textContent = String(finalVal);
        target.dataset.value = String(finalVal);
        target.dataset.editing = '';
        sealchat.updateAttrs(strategy.buildPatch(finalVal, ctx));
      };

      const cancel = () => {
        if (closed) return;
        closed = true;
        cleanup();
        target.textContent = originalText;
        target.dataset.editing = '';
      };

      const onKeydown = e => {
        if (e.key === 'Enter') { e.preventDefault(); commit(); }
        else if (e.key === 'Escape') { e.preventDefault(); cancel(); }
      };
      const onBlur = () => { if (!closed) commit(); };

      input.addEventListener('keydown', onKeydown);
      input.addEventListener('blur', onBlur);
    }

    // 多行文本输入框
    function openTextEditor(target) {
      const attrKey = target.dataset.attr;
      if (!attrKey || target.dataset.editing === '1') return;
      if (document.querySelector('[data-editing="1"]')) return;
      const currentVal = target.dataset.value || '';
      const originalHTML = target.innerHTML;
      const originalContent = target.querySelector('.editable-textarea-content');
      const scrollMemoryId = originalContent?.getAttribute('scroll-memory-id') || '';
      const emptyDisplayText = !currentVal ? (originalContent?.textContent || target.textContent || '') : '';
      const editor = document.createElement('textarea');
      editor.className = 'text-editor';
      editor.name = attrKey; 
      editor.value = currentVal;
      target.innerHTML = '';
      target.appendChild(editor);
      target.dataset.editing = '1';
      editor.focus();

      let closed = false;
      const cleanup = () => {
        editor.removeEventListener('keydown', keydownHandler);
        editor.removeEventListener('blur', blurHandler);
      };

      const commit = () => {
        if (target.dataset.editing !== '1' || closed) return;
        closed = true;
        cleanup();
        const val = editor.value;
        const patch = { [CHAR_INFO_KEY]: { ...state.charInfo, [attrKey]: val } };
        target.dataset.value = val;
        target.dataset.editing = '';
        target.classList.toggle('empty', !val);
        target.innerHTML = '<span class="editable-textarea-content"' +
          (scrollMemoryId ? ' scroll-memory-id="' + scrollMemoryId + '"' : '') +
          '>' + escapeHtml(val || emptyDisplayText) + '</span>';
        sealchat.updateAttrs(patch);
      };
      const cancel = () => {
        if (closed) return;
        closed = true;
        cleanup();
        target.innerHTML = originalHTML;
        target.dataset.editing = '';
      };

      const keydownHandler = (e) => {
        if (e.key === 'Escape') {
          e.preventDefault();
          cancel();
        } else if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
          e.preventDefault();
          commit();
        }
      };
      const blurHandler = () => {
        if (!closed) commit();
      };

      editor.addEventListener('keydown', keydownHandler);
      editor.addEventListener('blur', blurHandler);
      editor.addEventListener('click', e => e.stopPropagation());
    }

    // 处理循环切换型属性
    function toggleValueHandler(target) {
      const attrKey = target.dataset.attr;
      if (!attrKey || !(attrKey in TOGGLE_FIELDS)) return;
      const currentVal = target.dataset.value;
      const values = TOGGLE_FIELDS[attrKey];
      const nextVal = values.includes(currentVal) ? values[(values.indexOf(currentVal) + 1) % values.length] : values[0];
      sealchat.updateAttrs({ [CHAR_INFO_KEY]: { ...state.charInfo, [attrKey]: nextVal } });
    }

    // 切换技能选项卡
    function skillTabHandler(target) {
      const category = target.dataset.category;
      if (!category) return;
      saveScrollPositions(); 
      state.activeSkillCategory = category;
      document.querySelectorAll('.sheet__skill-tab').forEach(tab => tab.classList.remove('active'));
      target.classList.add('active');
      document.querySelectorAll('.sheet__skill-panel').forEach(panel => {
        panel.classList.toggle('active', panel.dataset.category === category);
      });
      restoreScrollPositions(); 
    }

    // 切换技能收藏
    function skillFavoriteHandler(target) {
      const skillKey = target.dataset.skill;
      if (!skillKey) return;
      const arr = state.charInfo['收藏夹'];
      const list = Array.isArray(arr) ? [...arr] : [];
      const newList = list.includes(skillKey) ? list.filter(s => s !== skillKey) : [...list, skillKey];
      sealchat.updateAttrs({ [CHAR_INFO_KEY]: { ...state.charInfo, ['收藏夹']: newList } });
    }

    // 职业选择
    function selectJobHandler(target) {
      const attrKey = target.dataset.attr;
      if (!attrKey) return;
      sealchat.updateAttrs({ [CHAR_INFO_KEY]: { ...state.charInfo, [attrKey]: target.value } });
    }

    // 武器选择
    function selectWeaponHandler(target) {
      const row = target.closest('[data-weapon-idx]');
      if (!row) return;
      const idx = toNumber(row.dataset.weaponIdx);
      const weapons = Array.isArray(state.charInfo['武器']) ? [...state.charInfo['武器']] : [];
      while (weapons.length <= idx) weapons.push({ name: '', damage: '', dmg: '' });
      const damage = WEAPON_MAP[target.value]?.weapon?.damage ?? '';
      weapons[idx] = { name: target.value, damage, dmg: parseWeaponDamage(damage) };
      const patch = { [CHAR_INFO_KEY]: { ...state.charInfo, '武器': weapons } };
      weapons.forEach((wp, i) => { if (wp.dmg) patch['dmg' + (i + 1)] = wp.dmg; });
      sealchat.updateAttrs(patch);
    }

    // 技能掷骰弹窗
    function openSkillModal(name, val) {
      document.getElementById('skillRollModal').dataset.rollSkill = name;
      document.getElementById('skillRollTitle').textContent = name + '检定';
      document.getElementById('skillRollValue').textContent = val;
      resetToggles('btnRollHidden', 'btnRollBonus', 'btnRollPenalty', 'btnRollModSkill', 'btnRollModDice');
      hideRows('rowRollBpCount', 'rowRollMod');
      setInputs({ inputRollBpCount: '1', inputRollMod: '', inputRollCount: '1' });
      focusModal(document.getElementById('skillRollModal'));
    }

    function performSkillRoll() {
      const skill = document.getElementById('skillRollModal').dataset.rollSkill;
      if (!skill) return;
      const hidden = document.getElementById('btnRollHidden').dataset.active === 'true';
      const bonusActive = document.getElementById('btnRollBonus').dataset.active === 'true';
      const penaltyActive = document.getElementById('btnRollPenalty').dataset.active === 'true';
      const bpCount = toNumber(document.getElementById('inputRollBpCount').value, 1);
      const count = toNumber(document.getElementById('inputRollCount').value, 1);
      const modSkillActive = document.getElementById('btnRollModSkill').dataset.active === 'true';
      const modDiceActive = document.getElementById('btnRollModDice').dataset.active === 'true';
      const modExpr = document.getElementById('inputRollMod').value.trim();

      const baseCmd = hidden ? '.rah' : '.ra';
      let bpPart = '';
      if (bonusActive) bpPart = 'b' + bpCount;
      else if (penaltyActive) bpPart = 'p' + bpCount;

      let modPart = '';
      if ((modSkillActive || modDiceActive) && modExpr) {
        modPart = /^[+\\-]/.test(modExpr) ? modExpr : '+' + modExpr;
      }

      let skillPart = skill;
      if (modSkillActive && modPart) {
        skillPart += modPart;
      }

      const parts = [baseCmd];
      if (count > 1) parts.push(count + '#');
      if (bpPart) parts.push(bpPart);
      if (modDiceActive && modPart) parts.push(modPart);
      parts.push(skillPart);

      const template = parts.join(' ');
      const label = skill + '检定';

      window.sealchat.roll(template, label, {});
      closeModal('skillRollModal');
    }

    // 伤害掷骰弹窗
    function openDamageModal(idx, damageDisplay) {
      document.getElementById('damageRollModal').dataset.weaponIdx = idx;
      document.getElementById('damageRollExpression').textContent = damageDisplay || '—';
      resetToggles('btnDamageRollHidden', 'btnDamageRollMod');
      hideRows('rowDamageRollMod');
      setInputs({ inputDamageRollCount: '1', inputDamageRollMod: '' });
      focusModal(document.getElementById('damageRollModal'));
    }

    function performDamageRoll() {
      const idx = toNumber(document.getElementById('damageRollModal').dataset.weaponIdx);
      const hidden = document.getElementById('btnDamageRollHidden').dataset.active === 'true';
      const count = toNumber(document.getElementById('inputDamageRollCount').value, 1);
      const modExpr = document.getElementById('inputDamageRollMod').value.trim();

      let damagePart = 'dmg' + (idx + 1);
      if (modExpr) {
        const mod = /^[+\\-]/.test(modExpr) ? modExpr : '+' + modExpr;
        damagePart += mod;
      }

      const baseCmd = hidden ? '.rh' : '.r';
      const parts = [baseCmd];
      if (count > 1) parts.push(count + '#');
      parts.push(damagePart);

      const template = parts.join(' ');
      const label = '伤害掷骰';

      window.sealchat.roll(template, label, {});
      closeModal('damageRollModal');
    }

    // 武器选择弹窗
    function openWeaponModal(idx, skill, damage) {
      const modal = document.getElementById('weaponChoiceModal');
      if (!modal) return;
      modal.dataset.weaponIdx = idx;
      modal.dataset.weaponSkill = skill;
      modal.dataset.weaponDamage = damage;
      focusModal(modal);
    }

    // 弹窗焦点陷阱
    function trapModalFocus(e) {
      if (e.key !== 'Tab') return;
      const activeModal = document.querySelector('.modal__overlay.active');
      if (!activeModal) return;
      const focusable = activeModal.querySelectorAll(
        'button:not([tabindex="-1"]), input:not([tabindex="-1"]), [tabindex]:not([tabindex="-1"])'
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!activeModal.contains(document.activeElement)) {
        e.preventDefault();
        first.focus();
        return;
      }
      if (e.shiftKey) {
        if (document.activeElement === first) { e.preventDefault(); last.focus(); }
      } else {
        if (document.activeElement === last) { e.preventDefault(); first.focus(); }
      }
    }

    function resetToggles(...ids) {
      ids.forEach(id => { document.getElementById(id).dataset.active = 'false'; });
    }
    function hideRows(...ids) {
      ids.forEach(id => { document.getElementById(id).style.display = 'none'; });
    }
    function setInputs(obj) {
      Object.entries(obj).forEach(([id, val]) => { document.getElementById(id).value = val; });
    }

    function focusModal(modalEl) {
      if (!modalEl) return;
      modalEl.classList.add('active');
      requestAnimationFrame(() => {
        const first = modalEl.querySelector('button:not([tabindex="-1"]), input:not([tabindex="-1"])');
        if (first) first.focus();
      });
    }
    function closeModal(id) {
      const modal = document.getElementById(id);
      if (!modal) return;
      modal.classList.remove('active');
    }

    // ==================== 事件委托 ====================
    document.addEventListener('click', e => { 
      const target = e.target;

      const weaponEl = target.closest('.sheet__weapon-skill.clickable');
      if (weaponEl) {
        const skill = weaponEl.dataset.rollSkill;
        if (!skill) return;
        const row = weaponEl.closest('[data-weapon-idx]');
        openWeaponModal(toNumber(row?.dataset.weaponIdx), skill, weaponEl.dataset.rollDamage || '');
        return;
      }

      const rollEl = target.closest('[data-roll-target]');
      if (rollEl) {
        const name = rollEl.dataset.rollTarget;
        if (!name) return;
        openSkillModal(name, toNumber(rollEl.dataset.rollValue));
        return;
      }

      const editableTextarea = target.closest('.editable-textarea');
      if (editableTextarea) {
        openTextEditor(editableTextarea);
        return;
      }
      if (target.matches('.editable-value')) {
        openInlineEditor(target);
        return;
      }

      if (target.matches('.toggle-value')) {
        toggleValueHandler(target);
        return;
      }

      if (target.matches('.sheet__skill-tab')) {
        skillTabHandler(target);
        return;
      }

      if (target.matches('.sheet__skill-favorite')) {
        skillFavoriteHandler(target);
        return;
      }
    });

    document.addEventListener('change', e => { 
      const target = e.target
      if (target.matches('.sheet__job-select')) {
        selectJobHandler(target);
        return;
      }
      if (target.matches('.sheet__weapon-select')) {
        selectWeaponHandler(target);
        return;
      }
    });

    const debouncedFilterSkills = debounce(filterSkillsByKeyword, 150);
    document.addEventListener('input', e => { 
      const target = e.target
      if (target.matches('.sheet__skill-search-input')) {
        state.searchKeyword = target.value;
        debouncedFilterSkills();
        return;
      }
    });

    // 窗口尺寸变化
    const debouncedResize = debounce(() => {
      drawRadar();
      updateTitlesForOverflow(['.sheet__basic-value', '.sheet__background-value']);
    }, 150);
    window.addEventListener('resize', debouncedResize);

    // 掷骰事件配置表
    const MODAL_ACTIONS = {
      btnSkillRollSubmit: performSkillRoll,
      btnSkillRollCancel: () => closeModal('skillRollModal'),
      btnDamageRollSubmit: performDamageRoll,
      btnDamageRollCancel: () => closeModal('damageRollModal'),
      btnWeaponCancel: () => closeModal('weaponChoiceModal'),
    };

    const MODAL_TOGGLES = {
      btnRollHidden: {},
      btnDamageRollHidden: {},
      btnDamageRollMod: { row: 'rowDamageRollMod' },
      btnRollBonus: { row: 'rowRollBpCount', exclusive: 'btnRollPenalty' },
      btnRollPenalty: { row: 'rowRollBpCount', exclusive: 'btnRollBonus' },
      btnRollModSkill: { row: 'rowRollMod', exclusive: 'btnRollModDice' },
      btnRollModDice: { row: 'rowRollMod', exclusive: 'btnRollModSkill' },
    };

    const MODAL_COUNTERS = {
      btnRollBpDec: { input: 'inputRollBpCount', delta: -1, min: 1, max: 10 },
      btnRollBpInc: { input: 'inputRollBpCount', delta: 1, min: 1, max: 10 },
      btnRollCountDec: { input: 'inputRollCount', delta: -1, min: 1, max: 10 },
      btnRollCountInc: { input: 'inputRollCount', delta: 1, min: 1, max: 10 },
      btnDamageRollCountDec: { input: 'inputDamageRollCount', delta: -1, min: 1, max: 10 },
      btnDamageRollCountInc: { input: 'inputDamageRollCount', delta: 1, min: 1, max: 10 },
    };

    document.addEventListener('click', e => {
      const target = e.target;

      if (target.matches('.modal__overlay')) {
        closeModal(target.id);
        return; 
      }

      if (MODAL_ACTIONS[target.id]) { 
        MODAL_ACTIONS[target.id](); 
        return; 
      }

      const toggleCfg = MODAL_TOGGLES[target.id];
      if (toggleCfg) {
        const active = target.dataset.active !== 'true';
        target.dataset.active = active ? 'true' : 'false';
        if (toggleCfg.row) {
          const rowEl = document.getElementById(toggleCfg.row);
          if (rowEl) rowEl.style.display = active ? 'flex' : 'none';
        }
        if (toggleCfg.exclusive && active) {
          const exclEl = document.getElementById(toggleCfg.exclusive);
          if (exclEl) exclEl.dataset.active = 'false';
        }
        return;
      }

      const cnt = MODAL_COUNTERS[target.id];
      if (cnt) {
        const input = document.getElementById(cnt.input);
        if (!input) return;
        let v = toNumber(input.value, cnt.min);
        v = Math.min(Math.max(v + cnt.delta, cnt.min), cnt.max);
        input.value = v;
        return;
      }

      if (target.id === 'btnWeaponAttack') {
        const modal = document.getElementById('weaponChoiceModal');
        if (!modal) return;
        const skill = modal.dataset.weaponSkill;
        if (skill) {
          closeModal('weaponChoiceModal');
          const rollEl = document.querySelector(\`.sheet__skill-name[data-roll-target="\${CSS.escape(skill)}"]\`);
          openSkillModal(skill, toNumber(rollEl?.dataset.rollValue));
        }
        return;
      }

      if (target.id === 'btnWeaponDamage') {
        const modal = document.getElementById('weaponChoiceModal');
        if (!modal) return;
        const idx = toNumber(modal.dataset.weaponIdx);
        closeModal('weaponChoiceModal');
        openDamageModal(idx, modal.dataset.weaponDamage || '');
        return;
      }
    });

    // 弹窗焦点陷阱
    document.addEventListener('keydown', trapModalFocus);

    // ==================== 启动 ====================
    window.sealchat.onUpdate(data => {
      hideLoadingWhenReady();
      render(data);
    });
  </script>
</body>
</html>
`;

const getShinobigamiDefaultTemplate = () => shinobigamiTemplateHtml.trim();

const getDefaultTemplate = (sheetType?: string) => (
  isShinobigamiSheetType(sheetType)
    ? getShinobigamiDefaultTemplate()
    : (isCocSheetType(sheetType) ? getCocDefaultTemplate() : getGenericDefaultTemplate())
);

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
      win.template = normalized;
      win.templateMode = binding?.mode;
      win.templateId = binding?.templateId || undefined;
      saveTemplate(win.cardId, normalized);
      schedulePersistWindows();
    } catch (e) {
      console.warn('Failed to sync character sheet template from cloud', e);
    }
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
      worldId?: string;
    }
  ): string => {
    restoreWindows();
    const existingId = activeWindowIds.value.find(
      id => windows.value[id]?.cardId === card.id
    );
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
        }

        existing.mode = 'view';

        if (existing.isMinimized) {
          existing.isMinimized = false;
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

    const clampedInitialPos = clampWindowCoords(
      VIEWPORT_PADDING + offset,
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
    const initialTemplate = normalizeTemplate(
      card.id,
      templateMeta?.templateText || getTemplate(card.id, resolvedSheetType),
      resolvedSheetType,
    );
    windows.value[windowId] = {
      id: windowId,
      cardId: card.id,
      cardName: card.name,
      channelId,
      worldId: templateMeta?.worldId,
      readOnly: !!templateMeta?.readOnly,
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
