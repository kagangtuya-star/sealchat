<script setup lang="tsx">
import { computed, ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue';
import Chat from './chat/chat.vue'
import ChatHeader from './components/header.vue'
import ChatSidebar from './components/sidebar.vue'
import { useWindowSize } from '@vueuse/core'
import { useChatStore, chatEvent } from '@/stores/chat';
import { SIDEBAR_WIDTH_LIMITS, useDisplayStore } from '@/stores/display';
import { useRoute, useRouter } from 'vue-router';
import { useMessage } from 'naive-ui';
import { useEmailBindReminder, EmailBindPrompt } from '@/composables/useEmailBindReminder';

const { width } = useWindowSize()
const chat = useChatStore();
const display = useDisplayStore();
const route = useRoute();
const router = useRouter();
const message = useMessage();
const { showPrompt: showEmailPrompt, dismiss: dismissEmailPrompt } = useEmailBindReminder();

const handleEmailBind = () => {
  router.push({ name: 'profile' });
};

const handleEmailDismiss = async () => {
  await dismissEmailPrompt();
};

const active = ref(false)
const isSidebarCollapsed = ref(false)
const sidebarResizeMode = ref(false)

const isMobileViewport = computed(() => width.value < 700)
const computedCollapsed = computed(() => isMobileViewport.value || isSidebarCollapsed.value)
const collapsedWidth = computed(() => 0)
const sidebarResizeDragging = ref(false)
const sidebarResizePointerId = ref<number | null>(null)
const sidebarResizeStartX = ref(0)
const sidebarResizeStartWidth = ref(0)
const sidebarWidthPreview = ref<number | null>(null)

const computeSidebarMaxWidth = () => {
  const viewportWidth = Number(width.value) || 0
  if (viewportWidth <= 0) return SIDEBAR_WIDTH_LIMITS.MAX
  const maxByViewport = Math.floor(viewportWidth * 0.6)
  return Math.max(SIDEBAR_WIDTH_LIMITS.MIN, Math.min(SIDEBAR_WIDTH_LIMITS.MAX, maxByViewport))
}

const clampSidebarWidth = (value: number) => {
  const maxWidth = computeSidebarMaxWidth()
  return Math.min(maxWidth, Math.max(SIDEBAR_WIDTH_LIMITS.MIN, Math.round(value)))
}

const effectiveSidebarWidth = computed(() => {
  const base = sidebarWidthPreview.value ?? display.settings.sidebarWidth
  return clampSidebarWidth(base)
})

const toggleSidebar = () => {
  if (isMobileViewport.value) {
    active.value = true
    return
  }
  isSidebarCollapsed.value = !isSidebarCollapsed.value
}

const toggleSidebarResizeMode = () => {
  if (sidebarResizeMode.value) {
    finishSidebarResize()
    sidebarResizeMode.value = false
    return
  }
  if (isMobileViewport.value || isSidebarCollapsed.value) return
  sidebarResizeMode.value = true
}

const applySidebarWidth = (value: number) => {
  const next = clampSidebarWidth(value)
  if (display.settings.sidebarWidth === next) return
  display.updateSettings({ sidebarWidth: next })
}

const clearResizeCursorState = () => {
  if (typeof document === 'undefined') return
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}

const finishSidebarResize = (event?: PointerEvent) => {
  if (!sidebarResizeDragging.value) return
  if (
    event
    && sidebarResizePointerId.value !== null
    && event.pointerId !== sidebarResizePointerId.value
  ) {
    return
  }
  sidebarResizeDragging.value = false
  if (event) {
    try {
      (event.currentTarget as HTMLElement | null)?.releasePointerCapture?.(event.pointerId)
    } catch {
      // ignore capture failure
    }
  }
  const finalWidth = clampSidebarWidth(sidebarWidthPreview.value ?? display.settings.sidebarWidth)
  sidebarWidthPreview.value = null
  sidebarResizePointerId.value = null
  clearResizeCursorState()
  applySidebarWidth(finalWidth)
}

const handleSidebarResizePointerDown = (event: PointerEvent) => {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  if (!sidebarResizeMode.value) return
  if (isMobileViewport.value || isSidebarCollapsed.value) return
  sidebarResizeDragging.value = true
  sidebarResizePointerId.value = event.pointerId
  sidebarResizeStartX.value = event.clientX
  sidebarResizeStartWidth.value = effectiveSidebarWidth.value
  sidebarWidthPreview.value = effectiveSidebarWidth.value
  try {
    (event.currentTarget as HTMLElement | null)?.setPointerCapture?.(event.pointerId)
  } catch {
    // ignore capture failure
  }
  if (typeof document !== 'undefined') {
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }
}

const handleSidebarResizePointerMove = (event: PointerEvent) => {
  if (!sidebarResizeDragging.value) return
  if (sidebarResizePointerId.value !== event.pointerId) return
  const deltaX = event.clientX - sidebarResizeStartX.value
  sidebarWidthPreview.value = clampSidebarWidth(sidebarResizeStartWidth.value + deltaX)
}

const handleSidebarResizePointerUp = (event: PointerEvent) => {
  finishSidebarResize(event)
}

const handleSidebarResizePointerCancel = (event: PointerEvent) => {
  finishSidebarResize(event)
}

onMounted(() => {
  const worldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  if (!worldId) {
    chat.ensureWorldReady();
  }
});

let deepLinkTaskEpoch = 0;
let urlSyncTaskEpoch = 0;
let pendingInternalRouteSyncKey = '';

const buildRouteSyncKey = (worldId: string, channelId: string) => `${worldId}::${channelId}`;

const handleDeepLink = async () => {
  const worldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  const channelId = typeof route.params.channelId === 'string' ? route.params.channelId.trim() : '';
  if (!worldId) return;
  const routeKey = buildRouteSyncKey(worldId, channelId);
  if (pendingInternalRouteSyncKey && pendingInternalRouteSyncKey === routeKey) {
    pendingInternalRouteSyncKey = '';
    return;
  }
  const taskEpoch = ++deepLinkTaskEpoch;
  try {
    if (chat.isObserver) {
      chat.enableObserverMode(worldId, channelId);
      if (chat.connectState === 'connected') {
        await chat.initObserverSession();
        if (taskEpoch !== deepLinkTaskEpoch) {
          return;
        }
      }
      return;
    }
    await chat.ensureWorldReady();
    if (taskEpoch !== deepLinkTaskEpoch) {
      return;
    }
    const currentWorldId = chat.currentWorldId ? String(chat.currentWorldId).trim() : '';
    const currentChannelId = chat.curChannel?.id ? String(chat.curChannel.id).trim() : '';
    if (worldId === currentWorldId) {
      if (channelId && channelId !== currentChannelId) {
        await chat.channelSwitchTo(channelId);
        if (taskEpoch !== deepLinkTaskEpoch) {
          return;
        }
      }
      return;
    }
    await chat.switchWorld(worldId, { force: true });
    if (taskEpoch !== deepLinkTaskEpoch) {
      return;
    }
    if (channelId) {
      await chat.channelSwitchTo(channelId);
      if (taskEpoch !== deepLinkTaskEpoch) {
        return;
      }
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
  const taskEpoch = ++urlSyncTaskEpoch;
  if (!isHomeRoute.value) return;
  const worldId = chat.currentWorldId ? String(chat.currentWorldId).trim() : '';
  if (!worldId) return;
  const rawChannelId = chat.curChannel?.id ? String(chat.curChannel.id).trim() : '';
  const channelId = isChannelInCurrentWorld(rawChannelId) ? rawChannelId : '';
  const routeWorldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  const routeChannelId = typeof route.params.channelId === 'string' ? route.params.channelId.trim() : '';
  if (routeWorldId === worldId && routeChannelId === channelId) return;
  const routeKey = buildRouteSyncKey(worldId, channelId);
  const params: { worldId: string; channelId?: string } = { worldId };
  if (channelId) {
    params.channelId = channelId;
  }
  try {
    pendingInternalRouteSyncKey = routeKey;
    await router.replace({ name: 'world-channel', params });
    if (taskEpoch !== urlSyncTaskEpoch) {
      return;
    }
  } catch (error) {
    if (pendingInternalRouteSyncKey === routeKey) {
      pendingInternalRouteSyncKey = '';
    }
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

// 处理消息链接跳转 (msg query 参数)
const pendingMessageJump = ref<string | null>(null);

const handleMessageLinkJump = async () => {
  const msgParam = route.query.msg;
  const messageId = typeof msgParam === 'string' ? msgParam.trim() : '';
  if (!messageId) return;

  const worldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : '';
  const channelId = typeof route.params.channelId === 'string' ? route.params.channelId.trim() : '';
  if (!worldId || !channelId) return;

  // 标记待跳转的消息
  pendingMessageJump.value = messageId;

  // 确保世界和频道已切换
  try {
    if (chat.currentWorldId !== worldId) {
      await chat.switchWorld(worldId, { force: true });
    }
    if (chat.curChannel?.id !== channelId) {
      const switched = await chat.channelSwitchTo(channelId);
      if (!switched) {
        message.error('无法访问该频道');
        pendingMessageJump.value = null;
        return;
      }
    }

    // 等待 DOM 更新后触发跳转
    await nextTick();

    // 触发消息跳转事件
    chatEvent.emit('search-jump', {
      messageId,
      channelId,
    });

    // 清除 query 参数，避免刷新后重复跳转
    await router.replace({
      name: route.name as string,
      params: route.params,
      query: {},
    });
  } catch (error) {
    console.warn('[message-link] jump failed', error);
    message.error('跳转失败');
  } finally {
    pendingMessageJump.value = null;
  }
};

watch(
  () => route.query.msg,
  (newVal) => {
    if (newVal) {
      void handleMessageLinkJump();
    }
  },
  { immediate: true },
);

watch(
  () => computedCollapsed.value,
  (collapsed) => {
    if (collapsed) {
      finishSidebarResize()
      sidebarResizeMode.value = false
    }
  },
)

watch(
  () => width.value,
  () => {
    if (sidebarResizeDragging.value && sidebarWidthPreview.value !== null) {
      sidebarWidthPreview.value = clampSidebarWidth(sidebarWidthPreview.value)
    }
  },
)

onBeforeUnmount(() => {
  clearResizeCursorState()
})

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
        :width="effectiveSidebarWidth"
        :native-scrollbar="false"
      >
        <ChatSidebar
          v-if="!isMobileViewport && !isSidebarCollapsed"
          :sidebar-width-resize-available="!isMobileViewport && !isSidebarCollapsed"
          :sidebar-width-resize-mode="sidebarResizeMode"
          @toggle-sidebar-width-resize="toggleSidebarResizeMode"
        />
      </n-layout-sider>

      <div
        v-if="!computedCollapsed && sidebarResizeMode"
        class="sc-layout-sider-resize-handle"
        :class="{ 'is-dragging': sidebarResizeDragging }"
        role="separator"
        aria-orientation="vertical"
        aria-label="调整侧边栏宽度"
        tabindex="-1"
        @pointerdown="handleSidebarResizePointerDown"
        @pointermove="handleSidebarResizePointerMove"
        @pointerup="handleSidebarResizePointerUp"
        @pointercancel="handleSidebarResizePointerCancel"
      />

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

    <EmailBindPrompt
      v-model:show="showEmailPrompt"
      @bind="handleEmailBind"
      @dismiss="handleEmailDismiss"
      @skip="() => {}"
    />
  </main>
</template>

<style lang="scss">
.sc-layout-sider-resize-handle {
  position: relative;
  width: 8px;
  flex: 0 0 8px;
  cursor: col-resize;
  user-select: none;
  touch-action: none;
  background: transparent;
}

.sc-layout-sider-resize-handle::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 50%;
  width: 1px;
  transform: translateX(-50%);
  background: var(--sc-border-strong);
}

.sc-layout-sider-resize-handle:hover::before,
.sc-layout-sider-resize-handle.is-dragging::before {
  width: 2px;
  background: rgba(14, 165, 233, 0.65);
}
</style>
