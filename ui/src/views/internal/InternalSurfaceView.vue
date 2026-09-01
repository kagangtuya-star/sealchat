<script setup lang="ts">
import { computed, onErrorCaptured, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { NResult, NSpin } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import { getInternalSurfaceComponent } from './internalSurfaceRegistry';

const route = useRoute();
const chat = useChatStore();

const contextReady = ref(false);
const resourceReady = ref(false);
const errorTitle = ref('');
const errorDescription = ref('');
let taskEpoch = 0;
let contextQueue: Promise<void> = Promise.resolve();

const normalizeRouteValue = (value: unknown) => (
  typeof value === 'string' ? value.trim() : ''
);

const type = computed(() => normalizeRouteValue(route.params.type));
const id = computed(() => normalizeRouteValue(route.params.id));
const worldId = computed(() => normalizeRouteValue(route.query.world));
const channelId = computed(() => normalizeRouteValue(route.query.channel));
const surfaceComponent = computed(() => getInternalSurfaceComponent(type.value));
const surfaceKey = computed(() => `${type.value}:${id.value}:${worldId.value}:${channelId.value}`);

const setError = (title: string, description: string) => {
  errorTitle.value = title;
  errorDescription.value = description;
  resourceReady.value = false;
};

const initializeContext = async (epoch: number) => {
  if (!surfaceComponent.value) {
    setError('不支持的内部窗口类型', type.value || '未提供 type');
    return;
  }
  if (!id.value || !worldId.value || !channelId.value) {
    setError('缺少运行上下文', 'URL 必须包含资源 id、world 和 channel');
    return;
  }

  const targetWorldId = worldId.value;
  const targetChannelId = channelId.value;
  try {
    await chat.ensureWorldReady();
    if (epoch !== taskEpoch) return;
    if (String(chat.currentWorldId || '') !== targetWorldId) {
      await chat.switchWorld(targetWorldId, { force: true, autoSwitch: false });
      if (epoch !== taskEpoch) return;
    }
    if (String(chat.curChannel?.id || '') !== targetChannelId) {
      const switched = await chat.channelSwitchTo(targetChannelId);
      if (epoch !== taskEpoch) return;
      if (!switched) throw new Error('无法访问指定频道');
    }
    if (
      String(chat.currentWorldId || '') !== targetWorldId
      || String(chat.curChannel?.id || '') !== targetChannelId
    ) {
      throw new Error('世界或频道上下文初始化失败');
    }
    contextReady.value = true;
  } catch (error: any) {
    if (epoch !== taskEpoch) return;
    setError('无法建立运行上下文', error?.response?.data?.error || error?.message || '世界或频道不可用');
  }
};

const queueContextInitialization = () => {
  const epoch = ++taskEpoch;
  contextReady.value = false;
  resourceReady.value = false;
  errorTitle.value = '';
  errorDescription.value = '';
  contextQueue = contextQueue
    .catch(() => undefined)
    .then(() => initializeContext(epoch));
};

watch(() => route.fullPath, queueContextInitialization, { immediate: true });

onErrorCaptured((error) => {
  setError('内部窗口加载失败', error instanceof Error ? error.message : String(error));
  return false;
});
</script>

<template>
  <main class="internal-surface">
    <n-result
      v-if="errorTitle"
      status="error"
      :title="errorTitle"
      :description="errorDescription"
    />
    <template v-else>
      <component
        v-if="contextReady && surfaceComponent"
        :is="surfaceComponent"
        :key="surfaceKey"
        :resource-id="id"
        :world-id="worldId"
        :channel-id="channelId"
        @ready="resourceReady = true"
        @unavailable="setError('资源不可用', $event)"
        @error="setError('资源加载失败', $event)"
      />
      <div v-if="!contextReady || !resourceReady" class="internal-surface__loading">
        <n-spin size="large" />
      </div>
    </template>
  </main>
</template>

<style scoped>
.internal-surface {
  position: relative;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: var(--sc-bg-surface, #fff);
}

.internal-surface :deep(.n-result) {
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.internal-surface__loading {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: grid;
  place-items: center;
  background: var(--sc-bg-surface, #fff);
}
</style>
