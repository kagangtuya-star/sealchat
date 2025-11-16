<script setup lang="tsx">
import { computed, ref, watch, onMounted } from 'vue';
import Chat from './chat/chat.vue'
import ChatHeader from './components/header.vue'
import ChatSidebar from './components/sidebar.vue'
import { useWindowSize } from '@vueuse/core'
import { useWorldStore } from '@/stores/world';
import { useChatStore } from '@/stores/chat';

const worldStore = useWorldStore();
const chatStore = useChatStore();

const { width } = useWindowSize()

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
  worldStore.fetchWorlds().catch(() => {});
});

watch(
  () => worldStore.currentWorldId,
  async (newId, oldId) => {
    if (newId && newId !== oldId) {
      try {
        await chatStore.ensureWorldSession(newId);
      } catch (err) {
        console.warn('世界切换重连失败', err);
      }
    }
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
