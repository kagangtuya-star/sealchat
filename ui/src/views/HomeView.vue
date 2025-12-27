<script setup lang="tsx">
import { computed, ref, onMounted, watch } from 'vue';
import Chat from './chat/chat.vue'
import ChatHeader from './components/header.vue'
import ChatSidebar from './components/sidebar.vue'
import { useWindowSize } from '@vueuse/core'
import { useChatStore } from '@/stores/chat';
import { useRoute, useRouter } from 'vue-router';

const { width } = useWindowSize()
const chat = useChatStore();
const route = useRoute();
const router = useRouter();

const active = ref(false)
const isSidebarCollapsed = ref(false)

const isMobileViewport = computed(() => width.value < 700)
const computedCollapsed = computed(() => isMobileViewport.value || isSidebarCollapsed.value)
const collapsedWidth = computed(() => 0)

const toggleSidebar = () => {
  if (isMobileViewport.value) {
    active.value = true
    return
  }
  isSidebarCollapsed.value = !isSidebarCollapsed.value
}

onMounted(() => {
  const worldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  if (!worldId) {
    chat.ensureWorldReady();
  }
});

const handleDeepLink = async () => {
  const worldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  const channelId = typeof route.params.channelId === 'string' ? route.params.channelId.trim() : '';
  if (!worldId) return;
  try {
    await chat.ensureWorldReady();
    const currentWorldId = chat.currentWorldId ? String(chat.currentWorldId).trim() : '';
    const currentChannelId = chat.curChannel?.id ? String(chat.curChannel.id).trim() : '';
    if (worldId === currentWorldId) {
      if (channelId && channelId !== currentChannelId) {
        await chat.channelSwitchTo(channelId);
      }
      return;
    }
    await chat.switchWorld(worldId, { force: true });
    if (channelId) {
      await chat.channelSwitchTo(channelId);
    }
  } catch (error) {
    console.warn('[deep-link] switch failed', error);
  }
};

watch(
  () => [route.params.worldId, route.params.channelId],
  () => {
    void handleDeepLink();
  },
  { immediate: true },
);

const isHomeRoute = computed(() => route.name === 'home' || route.name === 'world-channel');

const isChannelInCurrentWorld = (channelId: string) => {
  if (!channelId) return false;
  if (chat.temporaryArchivedChannel?.id === channelId) return true;
  const stack = [...(chat.channelTree || [])];
  while (stack.length) {
    const node = stack.pop();
    if (!node) continue;
    if (node.id === channelId) return true;
    if (node.children?.length) {
      stack.push(...node.children);
    }
  }
  return false;
};

const syncUrlWithSelection = async () => {
  if (!isHomeRoute.value) return;
  const worldId = chat.currentWorldId ? String(chat.currentWorldId).trim() : '';
  if (!worldId) return;
  const rawChannelId = chat.curChannel?.id ? String(chat.curChannel.id).trim() : '';
  const channelId = isChannelInCurrentWorld(rawChannelId) ? rawChannelId : '';
  const routeWorldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  const routeChannelId = typeof route.params.channelId === 'string' ? route.params.channelId.trim() : '';
  if (routeWorldId === worldId && routeChannelId === channelId) return;
  const params: { worldId: string; channelId?: string } = { worldId };
  if (channelId) {
    params.channelId = channelId;
  }
  try {
    await router.replace({ name: 'world-channel', params });
  } catch (error) {
    console.warn('[deep-link] url sync failed', error);
  }
};

watch(
  () => [chat.currentWorldId, chat.curChannel?.id, chat.channelTree.length, route.name],
  () => {
    void syncUrlWithSelection();
  },
  { immediate: true },
);

</script>

<template>
  <main class="h-screen sc-app-shell">
    <n-layout-header class="sc-layout-header">
      <chat-header :sidebar-collapsed="computedCollapsed" @toggle-sidebar="toggleSidebar" />
    </n-layout-header>

    <n-layout class="sc-layout-root" has-sider position="absolute" style="margin-top: 3.5rem;">
      <n-layout-sider
        class="sc-layout-sider"
        collapse-mode="width"
        :collapsed="computedCollapsed"
        :collapsed-width="collapsedWidth"
        :native-scrollbar="false"
      >
        <ChatSidebar v-if="!isMobileViewport && !isSidebarCollapsed" />
      </n-layout-sider>

      <n-layout class="sc-layout-content">
        <Chat @drawer-show="active = true" />

        <n-drawer v-model:show="active" :width="'65%'" placement="left">
          <n-drawer-content closable body-content-style="padding: 0">
            <template #header>频道选择</template>
            <ChatSidebar />
          </n-drawer-content>
        </n-drawer>
      </n-layout>
    </n-layout>
  </main>
</template>

<style lang="scss">
.xxx {
  display: none;
}

@media (min-width: 1024px) {
  .xxx {
    display: block;
  }
}

</style>
