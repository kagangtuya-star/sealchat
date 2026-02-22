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
}

export interface SealChatEvent {
  type: 'SEALCHAT_EVENT';
  version: number;
  windowId: string;
  action: 'ROLL_DICE' | 'UPDATE_ATTRS' | 'EDIT_START' | 'EDIT_END';
  payload: SealChatEventPayload;
}

const props = defineProps<{
  html: string;
  data: { name: string; attrs: Record<string, any>; avatarUrl?: string };
  windowId: string;
}>();

const emit = defineEmits<{
  iframeEvent: [event: SealChatEvent];
}>();

const iframeRef = ref<HTMLIFrameElement | null>(null);

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

const finalSrcDoc = computed(() => {
  const html = props.html || '';
  if (html.includes(EDIT_HOOK_MARKER)) return html;
  if (/<\/body>/i.test(html)) {
    return html.replace(/<\/body>/i, `${EDIT_HOOK_SCRIPT}</body>`);
  }
  return `${html}\n${EDIT_HOOK_SCRIPT}`;
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

const handleLoad = () => {
  postData();
};

const handleMessage = (e: MessageEvent) => {
  if (!iframeRef.value?.contentWindow) return;
  if (e.source !== iframeRef.value.contentWindow) return;
  if (e.data?.type !== 'SEALCHAT_EVENT') return;
  if (e.data?.windowId !== props.windowId) return;
  const incoming = e.data as SealChatEvent;
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

onMounted(() => {
  window.addEventListener('message', handleMessage);
  if (iframeRef.value?.contentDocument?.readyState === 'complete') {
    postData();
  }
});

onBeforeUnmount(() => {
  window.removeEventListener('message', handleMessage);
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
