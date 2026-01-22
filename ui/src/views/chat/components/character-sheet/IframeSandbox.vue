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
    rect?: { top: number; left: number; width: number; height: number };
    containerRect?: { top: number; left: number; width: number; height: number };
  };
  attrs?: Record<string, any>;
}

export interface SealChatEvent {
  type: 'SEALCHAT_EVENT';
  version: number;
  windowId: string;
  action: 'ROLL_DICE' | 'UPDATE_ATTRS';
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

const finalSrcDoc = computed(() => {
  return props.html;
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
