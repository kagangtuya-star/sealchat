<script setup lang="tsx">
import { ref, watch } from 'vue';
import Chat from './chat/chat.vue'
import ChatHeader from './components/header.vue'
import ChatSidebar from './components/sidebar.vue'
import { useWindowSize } from '@vueuse/core'

const { width } = useWindowSize()

const active = ref(false)
</script>

<template>
  <main class="h-screen">
    <n-layout-header bordered>
      <chat-header />
    </n-layout-header>

    <n-layout has-sider position="absolute" style="margin-top: 3.5rem;">
      <n-layout-sider :collapsed-width="0" :collapsed="width < 700" content-style="" :native-scrollbar="false" bordered>
        <ChatSidebar v-if="width >= 700" />
      </n-layout-sider>

      <n-layout>
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
