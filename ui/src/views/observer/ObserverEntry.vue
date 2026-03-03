<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useChatStore } from '@/stores/chat';
import HomeView from '@/views/HomeView.vue';

const route = useRoute();
const router = useRouter();
const chat = useChatStore();

const slug = computed(() => {
  const raw = typeof route.params.slug === 'string' ? route.params.slug : '';
  return raw.trim();
});

const status = ref<'pending' | 'processing' | 'ready' | 'invalid' | 'error'>('pending');
const errorMessage = ref('');
const lastResolvedSlug = ref('');

const resolveAndEnter = async () => {
  const targetSlug = slug.value;
  if (targetSlug && lastResolvedSlug.value === targetSlug && status.value === 'ready') {
    return;
  }
  if (!slug.value) {
    chat.disableObserverMode();
    status.value = 'invalid';
    errorMessage.value = '缺少旁观链接标识';
    return;
  }
  status.value = 'processing';
  try {
    const resp = await chat.resolveObserverLink(slug.value);
    const worldId = typeof resp?.worldId === 'string' ? resp.worldId.trim() : '';
    const channelId = typeof resp?.channelId === 'string' ? resp.channelId.trim() : '';
    if (!worldId) {
      status.value = 'invalid';
      errorMessage.value = '旁观链接无效或已关闭';
      return;
    }
    chat.enableObserverMode(worldId, channelId, targetSlug);
    await chat.ensureConnectionReady();
    lastResolvedSlug.value = targetSlug;
    status.value = 'ready';
  } catch (error: any) {
    chat.disableObserverMode();
    const code = error?.response?.status;
    if (code === 404) {
      status.value = 'invalid';
      errorMessage.value = error?.response?.data?.message || '旁观链接无效或已关闭';
      return;
    }
    status.value = 'error';
    errorMessage.value = error?.response?.data?.message || '进入旁观模式失败，请稍后重试';
  }
};

const goHome = () => {
  chat.disableObserverMode();
  router.replace({ name: 'home' });
};

watch(
  () => slug.value,
  () => {
    status.value = 'pending';
    errorMessage.value = '';
    void resolveAndEnter();
  },
  { immediate: true },
);
</script>

<template>
  <HomeView v-if="status === 'ready'" />
  <div v-if="status !== 'ready'" class="observer-entry-page">
    <n-card class="observer-entry-card" title="OB 旁观入口">
      <n-spin :show="status === 'processing'">
        <template v-if="status === 'pending' || status === 'processing'">
          <p>正在解析旁观链接...</p>
        </template>
        <template v-else-if="status === 'invalid'">
          <n-alert type="warning" title="链接不可用">{{ errorMessage }}</n-alert>
          <div class="observer-entry-actions">
            <n-button type="primary" @click="goHome">返回首页</n-button>
          </div>
        </template>
        <template v-else>
          <n-alert type="error" title="访问失败">{{ errorMessage }}</n-alert>
          <div class="observer-entry-actions">
            <n-button type="primary" @click="goHome">返回首页</n-button>
          </div>
        </template>
      </n-spin>
    </n-card>
  </div>
</template>

<style scoped>
.observer-entry-page {
  min-height: 100vh;
  padding: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--sc-bg-body, #f8fafc);
}

.observer-entry-card {
  width: min(520px, 100%);
}

.observer-entry-actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}
</style>
