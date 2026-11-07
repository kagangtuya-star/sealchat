<template>
  <div class="iframe-sandbox">
    <iframe
      ref="iframeRef"
      :srcdoc="finalSrcDoc"
      :title="`人物卡: ${props.data.name || '未命名'}`"
      sandbox="allow-scripts"
      class="iframe-sandbox__frame"
      @load="handleLoad"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue';
import {
  buildCharacterSheetFontCssMessage,
  parseCharacterSheetFontManifest,
  resolveCharacterSheetFontCss,
} from '@/services/font/characterSheetTemplateFontRuntime';

export interface SealChatEventPayload {
  roll?: {
    template: string;
    label?: string;
    args?: Record<string, any>;
    dispatchMode?: 'default' | 'template';
    rect?: { top: number; left: number; width: number; height: number };
    containerRect?: { top: number; left: number; width: number; height: number };
  };
  attrs?: Record<string, any>;
  requestId?: string;
}

export interface SealChatEvent {
  type: 'SEALCHAT_EVENT';
  version: number;
  windowId: string;
  action: 'ROLL_DICE' | 'UPDATE_ATTRS' | 'EDIT_START' | 'EDIT_END' | 'FLUSH_ATTRS';
  payload: SealChatEventPayload;
}

const props = defineProps<{
  html: string;
  data: { name: string; attrs: Record<string, any>; avatarUrl?: string };
  windowId: string;
  flushAttrs?: () => Promise<void>;
}>();

const MOBILE_PAN_STYLE_MARKER = 'data-sealchat-mobile-pan="1"';
const MOBILE_PAN_STYLE = `<style ${MOBILE_PAN_STYLE_MARKER}>
@media (max-width: 767px) {
  html,
  body {
    overflow-x: auto !important;
    touch-action: none !important;
    -webkit-overflow-scrolling: touch;
  }
}
</style>`;

const MOBILE_PAN_HOOK_MARKER = 'data-sealchat-mobile-pan-hook="1"';
const MOBILE_PAN_HOOK_SCRIPT = `<script ${MOBILE_PAN_HOOK_MARKER}>
(function () {
  if (!window.matchMedia || !window.matchMedia('(max-width: 767px)').matches) return;
  var startX = 0;
  var startY = 0;
  var startLeft = 0;
  var startTop = 0;
  var horizontalScroller = null;
  var verticalScroller = null;
  var tracking = false;
  var moved = false;
  var lastTapTarget = null;
  var lastTapTime = 0;

  function findScroller(target, axis) {
    var node = target instanceof Element ? target : null;
    while (node && node !== document.documentElement) {
      if (axis === 'x' && node.scrollWidth > node.clientWidth + 1) return node;
      if (axis === 'y' && node.scrollHeight > node.clientHeight + 1) return node;
      node = node.parentElement;
    }
    var root = document.scrollingElement || document.documentElement || document.body;
    if (axis === 'x' && root.scrollWidth > root.clientWidth + 1) return root;
    if (axis === 'y' && root.scrollHeight > root.clientHeight + 1) return root;
    return null;
  }

  function isEditable(target) {
    return target instanceof Element && !!target.closest('input, textarea, select, [contenteditable="true"]');
  }

  document.addEventListener('touchstart', function (event) {
    if (event.touches.length !== 1 || isEditable(event.target)) return;
    var touch = event.touches[0];
    horizontalScroller = findScroller(event.target, 'x');
    verticalScroller = findScroller(event.target, 'y');
    tracking = true;
    moved = false;
    startX = touch.clientX;
    startY = touch.clientY;
    startLeft = horizontalScroller ? horizontalScroller.scrollLeft : 0;
    startTop = verticalScroller ? verticalScroller.scrollTop : 0;
  }, { passive: true });

  document.addEventListener('touchmove', function (event) {
    if (!tracking || event.touches.length !== 1) return;
    var touch = event.touches[0];
    var dx = touch.clientX - startX;
    var dy = touch.clientY - startY;
    if (!moved && Math.abs(dx) < 4 && Math.abs(dy) < 4) return;
    moved = true;
    event.preventDefault();
    if (horizontalScroller) horizontalScroller.scrollLeft = startLeft - dx;
    if (verticalScroller) verticalScroller.scrollTop = startTop - dy;
  }, { passive: false });

  function stopPan() {
    tracking = false;
    moved = false;
    horizontalScroller = null;
    verticalScroller = null;
  }

  document.addEventListener('touchend', function (event) {
    var target = event.target;
    var now = Date.now();
    if (!moved && target instanceof Element && !isEditable(target) && !target.closest('[data-tool-kind]')) {
      if (target === lastTapTarget && now - lastTapTime <= 360) {
        lastTapTarget = null;
        lastTapTime = 0;
        target.dispatchEvent(new MouseEvent('dblclick', {
          bubbles: true,
          cancelable: true,
          view: window
        }));
      } else {
        lastTapTarget = target;
        lastTapTime = now;
      }
    } else if (moved) {
      lastTapTarget = null;
      lastTapTime = 0;
    }
    stopPan();
  }, { passive: true });
  document.addEventListener('touchcancel', stopPan, { passive: true });
})();
<\/script>`;

const emit = defineEmits<{
  iframeEvent: [event: SealChatEvent];
}>();

const iframeRef = ref<HTMLIFrameElement | null>(null);
const fontSyncTimer = ref<ReturnType<typeof setTimeout> | null>(null);
const fontSyncSeq = ref(0);
const latestVisibleText = ref('');

const EDIT_HOOK_MARKER = 'data-sealchat-edit-hook="1"';
const EDIT_HOOK_SCRIPT = `<script ${EDIT_HOOK_MARKER}>
(function () {
  var _windowId = '';
  var editing = false;
  var endTimer = null;
  function isEditable(el) {
    if (!el || !(el instanceof Element)) return false;
    if (el instanceof HTMLInputElement) {
      return !el.disabled && !el.readOnly && el.type !== 'hidden';
    }
    if (el instanceof HTMLTextAreaElement) return !el.disabled && !el.readOnly;
    if (el instanceof HTMLSelectElement) return !el.disabled;
    return !!el.isContentEditable;
  }
  function post(action) {
    if (!_windowId) return;
    try {
      window.parent.postMessage({
        type: 'SEALCHAT_EVENT',
        version: 1,
        windowId: _windowId,
        action: action,
        payload: {}
      }, '*');
    } catch (e) {}
  }
  function markEditStart() {
    if (endTimer) {
      clearTimeout(endTimer);
      endTimer = null;
    }
    if (editing) return;
    editing = true;
    post('EDIT_START');
  }
  function checkEditEnd() {
    var active = document.activeElement;
    if (!isEditable(active) && editing) {
      editing = false;
      post('EDIT_END');
    }
  }
  document.addEventListener('focusin', function (ev) {
    if (isEditable(ev.target)) {
      markEditStart();
    }
  }, true);
  document.addEventListener('focusout', function () {
    if (endTimer) clearTimeout(endTimer);
    endTimer = setTimeout(checkEditEnd, 0);
  }, true);
  window.addEventListener('blur', function () {
    if (endTimer) clearTimeout(endTimer);
    endTimer = setTimeout(checkEditEnd, 0);
  });
  window.addEventListener('message', function (e) {
    if (e.source !== window.parent) return;
    var data = e.data;
    if (data && data.type === 'SEALCHAT_UPDATE' && data.payload && typeof data.payload.windowId === 'string') {
      _windowId = data.payload.windowId;
    }
  });
})();
<\/script>`;

const SYNC_HOOK_MARKER = 'data-sealchat-sync-hook="1"';
const SYNC_HOOK_SCRIPT = `<script ${SYNC_HOOK_MARKER}>
(function () {
  var windowId = '';
  var sequence = 0;
  var pending = new Map();
  window.sealchat = window.sealchat || {};
  window.sealchat.flushAttrs = function () {
    if (!windowId) return Promise.reject(new Error('人物卡尚未连接宿主'));
    return new Promise(function (resolve, reject) {
      var requestId = String(++sequence);
      var timer = setTimeout(function () {
        pending.delete(requestId);
        reject(new Error('人物卡属性同步超时，请重试'));
      }, 30000);
      pending.set(requestId, { resolve: resolve, reject: reject, timer: timer });
      window.parent.postMessage({
        type: 'SEALCHAT_EVENT', version: 1, windowId: windowId,
        action: 'FLUSH_ATTRS', payload: { requestId: requestId }
      }, '*');
    });
  };
  window.addEventListener('message', function (e) {
    if (e.source !== window.parent) return;
    var data = e.data;
    if (data && data.type === 'SEALCHAT_UPDATE' && data.payload) {
      windowId = data.payload.windowId;
    }
    if (!data || data.type !== 'SEALCHAT_ATTRS_FLUSHED' || data.windowId !== windowId) return;
    var request = pending.get(data.requestId);
    if (!request) return;
    pending.delete(data.requestId);
    clearTimeout(request.timer);
    if (data.ok === true) request.resolve();
    else request.reject(new Error(data.error || '人物卡属性同步失败'));
  });
  window.addEventListener('pagehide', function () {
    pending.forEach(function (request) {
      clearTimeout(request.timer);
      request.reject(new Error('人物卡窗口已关闭'));
    });
    pending.clear();
  });
})();
<\/script>`;

const FONT_HOOK_MARKER = 'data-sealchat-font-hook="1"';
const FONT_HOOK_SCRIPT = `<script ${FONT_HOOK_MARKER}>
(function () {
  var _windowId = '';
  var timer = null;
  var lastText = '';
  function collectText() {
    return (document.body && document.body.innerText) || '';
  }
  function postText() {
    if (!_windowId) return;
    var text = collectText();
    if (text === lastText) return;
    lastText = text;
    try {
      window.parent.postMessage({
        type: 'SEALCHAT_FONT_TEXT',
        version: 1,
        windowId: _windowId,
        payload: { text: text }
      }, '*');
    } catch (e) {}
  }
  function schedulePostText() {
    if (timer) clearTimeout(timer);
    timer = setTimeout(postText, 80);
  }
  function applyCss(cssText) {
    var style = document.querySelector('style[data-sealchat-font-runtime]');
    if (!style) {
      style = document.createElement('style');
      style.setAttribute('data-sealchat-font-runtime', '1');
      document.head.appendChild(style);
    }
    style.textContent = String(cssText || '');
  }
  window.addEventListener('message', function (e) {
    if (e.source !== window.parent) return;
    var data = e.data;
    if (data && data.type === 'SEALCHAT_UPDATE' && data.payload && typeof data.payload.windowId === 'string') {
      _windowId = data.payload.windowId;
      schedulePostText();
      return;
    }
    if (data && data.type === 'SEALCHAT_FONT_CSS' && data.payload) {
      applyCss(data.payload.cssText);
    }
  });
  if (typeof MutationObserver === 'function') {
    var observer = new MutationObserver(schedulePostText);
    observer.observe(document.documentElement || document, { childList: true, subtree: true, characterData: true });
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', schedulePostText);
  } else {
    schedulePostText();
  }
})();
<\/script>`;

const hasTemplateFonts = computed(() => parseCharacterSheetFontManifest(props.html).fonts.length > 0);

const finalSrcDoc = computed(() => {
  let html = props.html || '';
  if (!html.includes(MOBILE_PAN_STYLE_MARKER)) {
    if (/<\/head>/i.test(html)) {
      html = html.replace(/<\/head>/i, `${MOBILE_PAN_STYLE}</head>`);
    } else {
      html = `${MOBILE_PAN_STYLE}\n${html}`;
    }
  }
  if (!html.includes(MOBILE_PAN_HOOK_MARKER)) {
    if (/<\/body>/i.test(html)) {
      html = html.replace(/<\/body>/i, `${MOBILE_PAN_HOOK_SCRIPT}</body>`);
    } else {
      html = `${html}\n${MOBILE_PAN_HOOK_SCRIPT}`;
    }
  }
  if (!html.includes(EDIT_HOOK_MARKER)) {
    if (/<\/body>/i.test(html)) {
      html = html.replace(/<\/body>/i, `${EDIT_HOOK_SCRIPT}</body>`);
    } else {
      html = `${html}\n${EDIT_HOOK_SCRIPT}`;
    }
  }
  if (!html.includes(SYNC_HOOK_MARKER)) {
    if (/<\/body>/i.test(html)) {
      html = html.replace(/<\/body>/i, `${SYNC_HOOK_SCRIPT}</body>`);
    } else {
      html = `${html}\n${SYNC_HOOK_SCRIPT}`;
    }
  }
  if (hasTemplateFonts.value && !html.includes(FONT_HOOK_MARKER)) {
    if (/<\/body>/i.test(html)) {
      html = html.replace(/<\/body>/i, `${FONT_HOOK_SCRIPT}</body>`);
    } else {
      html = `${html}\n${FONT_HOOK_SCRIPT}`;
    }
  }
  return html;
});

const postData = () => {
  if (!iframeRef.value?.contentWindow) return;
  try {
    const payload = JSON.parse(JSON.stringify(props.data));
    payload.windowId = props.windowId;
    iframeRef.value.contentWindow.postMessage(
      { type: 'SEALCHAT_UPDATE', payload },
      '*'
    );
  } catch (e) {
    console.warn('Failed to post data to iframe', e);
  }
};

let frameGeneration = 0;
const handleLoad = () => {
  frameGeneration += 1;
  postData();
  scheduleFontSync(latestVisibleText.value);
};

const postFontCss = (cssText: string) => {
  if (!iframeRef.value?.contentWindow) return;
  iframeRef.value.contentWindow.postMessage(
    buildCharacterSheetFontCssMessage(cssText),
    '*',
  );
};

const syncCharacterSheetFonts = async (visibleText: string) => {
  if (!hasTemplateFonts.value) {
    postFontCss('');
    return;
  }
  const seq = fontSyncSeq.value + 1;
  fontSyncSeq.value = seq;
  try {
    const cssText = await resolveCharacterSheetFontCss(props.html, visibleText);
    if (seq !== fontSyncSeq.value) return;
    postFontCss(cssText);
  } catch (error) {
    console.warn('人物卡模板字体加载失败', error);
  }
};

const scheduleFontSync = (visibleText: string) => {
  latestVisibleText.value = visibleText || '';
  if (fontSyncTimer.value) {
    clearTimeout(fontSyncTimer.value);
  }
  fontSyncTimer.value = setTimeout(() => {
    void syncCharacterSheetFonts(latestVisibleText.value);
  }, 80);
};

const handleMessage = async (e: MessageEvent) => {
  if (!iframeRef.value?.contentWindow) return;
  if (e.source !== iframeRef.value.contentWindow) return;
  if (e.data?.type === 'SEALCHAT_FONT_TEXT' && e.data?.windowId === props.windowId) {
    scheduleFontSync(String(e.data?.payload?.text || ''));
    return;
  }
  if (e.data?.type !== 'SEALCHAT_EVENT') return;
  if (e.data?.windowId !== props.windowId) return;
  const incoming = e.data as SealChatEvent;
  if (incoming.action === 'FLUSH_ATTRS') {
    const requestId = incoming.payload?.requestId;
    if (typeof requestId !== 'string' || !requestId || requestId.length > 100) return;
    const source = iframeRef.value.contentWindow;
    const generation = frameGeneration;
    const windowId = props.windowId;
    let error = '';
    try {
      if (!props.flushAttrs) throw new Error('当前预览不支持人物卡属性同步');
      await props.flushAttrs();
    } catch (e) {
      error = e instanceof Error && e.message ? e.message : '人物卡属性同步失败';
    }
    if (generation !== frameGeneration || iframeRef.value?.contentWindow !== source
      || props.windowId !== windowId) return;
    source.postMessage({ type: 'SEALCHAT_ATTRS_FLUSHED', windowId, requestId, ok: !error, error }, '*');
    return;
  }
  const roll = incoming.payload?.roll;
  if (roll) {
    const frameRect = iframeRef.value.getBoundingClientRect();
    const containerRect = {
      top: frameRect.top,
      left: frameRect.left,
      width: frameRect.width,
      height: frameRect.height,
    };
    const nextRoll = roll.rect
      ? {
          ...roll,
          rect: {
            top: roll.rect.top + frameRect.top,
            left: roll.rect.left + frameRect.left,
            width: roll.rect.width,
            height: roll.rect.height,
          },
          containerRect,
        }
      : { ...roll, containerRect };
    emit('iframeEvent', { ...incoming, payload: { ...incoming.payload, roll: nextRoll } });
    return;
  }
  emit('iframeEvent', incoming);
};

watch(
  () => props.data,
  () => {
    postData();
  },
  { deep: true }
);

watch(
  () => props.html,
  () => {
    frameGeneration += 1;
    scheduleFontSync(latestVisibleText.value);
  },
);

onMounted(() => {
  window.addEventListener('message', handleMessage);
  if (iframeRef.value?.contentDocument?.readyState === 'complete') {
    postData();
  }
});

onBeforeUnmount(() => {
  frameGeneration += 1;
  window.removeEventListener('message', handleMessage);
  if (fontSyncTimer.value) {
    clearTimeout(fontSyncTimer.value);
  }
});
</script>

<style scoped>
.iframe-sandbox {
  width: 100%;
  height: 100%;
  overflow: hidden;
}

.iframe-sandbox__frame {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
  background: transparent;
}
</style>
