<script setup lang="ts">
import type { WorldInviteModel, WorldModel } from '@/stores/world';
import { api } from '@/stores/_config';
import { useUserStore } from '@/stores/user';
import { useWorldStore } from '@/stores/world';
import { useChatStore } from '@/stores/chat';
import { NCard, NButton, NSkeleton, useMessage } from 'naive-ui';
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

const route = useRoute();
const router = useRouter();
const message = useMessage();
const userStore = useUserStore();
const worldStore = useWorldStore();
const chatStore = useChatStore();

const loading = ref(true);
const joinLoading = ref(false);
const summary = ref<{ invite: WorldInviteModel; world: WorldModel } | null>(null);
const loadError = ref('');

const code = computed(() => String(route.params.code || '').trim());

const fetchSummary = async () => {
  if (!code.value) return;
  loading.value = true;
  loadError.value = '';
  summary.value = null;
  try {
    const resp = await api.get<{ invite: WorldInviteModel; world: WorldModel }>(`/api/v1/invites/${code.value}`);
    summary.value = resp.data;
  } catch (err) {
    loadError.value = '邀请不存在或已失效';
  } finally {
    loading.value = false;
  }
};

const handleJoin = async () => {
  if (!code.value || !summary.value) {
    return;
  }
  if (!userStore.token) {
    router.push({ name: 'user-signin', query: { redirect: router.currentRoute.value.fullPath } });
    return;
  }
  joinLoading.value = true;
  try {
    await api.post(`/api/v1/invites/${code.value}/accept`);
    message.success('已加入世界');
    const targetSlug = summary.value.world.slug || summary.value.world.id;
    await worldStore.fetchWorlds();
    const joinedWorld = await worldStore.fetchWorldBySlug(targetSlug);
    await chatStore.ensureWorldSession(joinedWorld?.id || summary.value.world.id);
    router.push(`/worlds/${targetSlug}`);
  } catch (err) {
    message.error('加入失败，邀请可能已失效');
  } finally {
    joinLoading.value = false;
  }
};

onMounted(fetchSummary);
watch(() => route.params.code, fetchSummary);
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-4">
    <n-card class="w-full max-w-lg">
      <template #header>世界邀请</template>

      <div v-if="loading">
        <n-skeleton height="160px" />
      </div>
      <div v-else-if="loadError" class="text-center text-gray-500 py-10">
        {{ loadError }}
      </div>
      <div v-else-if="summary" class="space-y-4">
        <div class="space-y-1">
          <div class="text-sm text-gray-500">受邀加入的世界</div>
          <div class="text-2xl font-semibold">{{ summary.world.name }}</div>
          <div class="text-sm text-gray-600">{{ summary.world.description || '暂无介绍' }}</div>
        </div>

        <div class="text-sm text-gray-500">
          可见性：{{ summary.world.visibility === 'private' ? '私密' : '公开' }} ·
          加入策略：{{ summary.world.joinPolicy === 'approval' ? '需审批' : summary.world.joinPolicy === 'invite_only' ? '仅邀请' : '开放' }}
        </div>

        <div class="flex flex-col gap-2">
          <n-button type="primary" :loading="joinLoading" @click="handleJoin">加入世界</n-button>
          <n-button v-if="!userStore.token" tertiary @click="$router.push('/user/signin')">登录后加入</n-button>
        </div>
      </div>
    </n-card>
  </div>
</template>

