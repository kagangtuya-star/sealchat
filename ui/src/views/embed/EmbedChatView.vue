<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import Chat from '@/views/chat/chat.vue';
import { useChatStore } from '@/stores/chat';

type ChannelOption = { label: string; value: string };

const route = useRoute();
const chat = useChatStore();

const paneId = computed(() => (typeof route.query.paneId === 'string' ? route.query.paneId : ''));
const initialWorldId = computed(() => (typeof route.query.worldId === 'string' ? route.query.worldId : ''));
const initialChannelId = computed(() => (typeof route.query.channelId === 'string' ? route.query.channelId : ''));

const flattenChannelOptions = (): ChannelOption[] => {
  const result: ChannelOption[] = [];
  const walk = (items: any[], depth: number) => {
    if (!Array.isArray(items)) return;
    items.forEach((item) => {
      if (!item || !item.id) return;
      const prefix = depth > 0 ? `${'—'.repeat(Math.min(depth, 3))} ` : '';
      const name = typeof item.name === 'string' ? item.name : String(item.name || '');
      result.push({ value: String(item.id), label: `${prefix}${name}`.trim() });
      if (Array.isArray(item.children) && item.children.length > 0) {
        walk(item.children, depth + 1);
      }
    });
  };
  walk(chat.channelTree as any[], 0);
  return result;
};

const postState = (type: 'sealchat.embed.ready' | 'sealchat.embed.state') => {
  if (typeof window === 'undefined') return;
  if (!paneId.value) return;
  if (window.parent === window) return;
  window.parent.postMessage(
    {
      type,
      paneId: paneId.value,
      worldId: chat.currentWorldId || '',
      currentChannelId: chat.curChannel?.id ? String(chat.curChannel.id) : '',
      channelOptions: flattenChannelOptions(),
    },
    window.location.origin,
  );
};

const handleMessage = async (event: MessageEvent) => {
  if (event.origin !== window.location.origin) return;
  const data = event.data as any;
  if (!data || typeof data !== 'object') return;
  if (data.type === 'sealchat.embed.setChannel') {
    if (data.paneId && data.paneId !== paneId.value) return;
    const channelId = typeof data.channelId === 'string' ? data.channelId : '';
    if (!channelId) return;
    try {
      await chat.channelSwitchTo(channelId);
      postState('sealchat.embed.state');
    } catch (e) {
      console.warn('[embed] channelSwitchTo failed', e);
    }
  }
};

const postFocus = (focused: boolean) => {
  if (!paneId.value) return;
  if (window.parent === window) return;
  window.parent.postMessage(
    { type: 'sealchat.embed.focus', paneId: paneId.value, focused },
    window.location.origin,
  );
};

const handleFocus = () => postFocus(true);
const handleBlur = () => postFocus(false);

const initializing = ref(false);

const initialize = async () => {
  if (initializing.value) return;
  initializing.value = true;
  try {
    await chat.ensureWorldReady();
    if (initialWorldId.value) {
      chat.setCurrentWorld(initialWorldId.value);
    }
    await chat.channelList(chat.currentWorldId, true);
    if (initialChannelId.value) {
      await chat.channelSwitchTo(initialChannelId.value);
    }
    postState('sealchat.embed.ready');
  } finally {
    initializing.value = false;
  }
};

watch(
  () => chat.curChannel?.id,
  () => {
    postState('sealchat.embed.state');
  },
);

onMounted(() => {
  initialize();
  window.addEventListener('message', handleMessage);
  window.addEventListener('focus', handleFocus);
  window.addEventListener('blur', handleBlur);
  postFocus(document.hasFocus());
});

onBeforeUnmount(() => {
  window.removeEventListener('message', handleMessage);
  window.removeEventListener('focus', handleFocus);
  window.removeEventListener('blur', handleBlur);
});
</script>

<template>
  <div class="sc-embed-root">
    <Chat />
  </div>
</template>

<style scoped>
.sc-embed-root {
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}
</style>

