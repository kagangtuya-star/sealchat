<script setup lang="ts">
import { onMounted, ref, computed, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useWorldStore } from '@/stores/world';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { api } from '@/stores/_config';
import { NButton, NCard, NSkeleton, NTag } from 'naive-ui';
import type { Channel } from '@satorijs/protocol';

const route = useRoute();
const router = useRouter();
const worldStore = useWorldStore();
const chatStore = useChatStore();
const userStore = useUserStore();
const channels = ref<Channel[]>([]);
const loading = ref(false);

const slug = computed(() => String(route.params.slug || ''));

const loadWorld = async () => {
  loading.value = true;
  try {
    const world = await worldStore.fetchWorldBySlug(slug.value);
    const targetWorldId = world?.id || worldStore.currentWorldId;
    await chatStore.ensureWorldSession(targetWorldId);
    if (!targetWorldId) {
      channels.value = [];
      return;
    }
    const resp = await api.get<{ channels: Channel[] }>(`/api/v1/worlds/${targetWorldId}/channels`);
    channels.value = resp.data.channels || [];
  } finally {
    loading.value = false;
  }
};

const enterChannel = async (ch: Channel) => {
  await chatStore.channelSwitchTo(ch.id);
  router.push('/');
};

const goManage = () => {
  if (worldStore.currentWorld?.slug) {
    router.push(`/worlds/${worldStore.currentWorld.slug}/manage`);
  }
};

onMounted(loadWorld);
watch(() => route.params.slug, loadWorld);
</script>

<template>
  <div class="max-w-4xl mx-auto p-4">
    <n-card v-if="worldStore.currentWorld" :title="worldStore.currentWorld.name">
      <p class="text-gray-600 mb-2">{{ worldStore.currentWorld.description }}</p>
      <div class="flex gap-2 text-sm text-gray-500">
        <n-tag size="small" type="info">成员 {{ worldStore.currentWorld.memberCount ?? '--' }}</n-tag>
        <n-tag size="small" type="success">
          {{ worldStore.currentWorld.visibility === 'private' ? '私密世界' : '公开世界' }}
        </n-tag>
      </div>
      <div class="mt-3 flex gap-2">
        <n-button size="small" secondary @click="$router.push('/worlds')">返回大厅</n-button>
        <n-button
          v-if="userStore.checkPerm('func_world_manage')"
          size="small"
          type="primary"
          @click="goManage"
        >
          管理世界
        </n-button>
      </div>
    </n-card>

    <div class="mt-6">
      <h3 class="text-lg font-semibold mb-3">频道列表</h3>
      <div class="space-y-2">
        <template v-if="loading">
          <n-skeleton v-for="i in 3" :key="i" height="64px" />
        </template>
        <template v-else>
          <n-card v-for="channel in channels" :key="channel.id" class="flex items-center justify-between">
            <div>
              <div class="font-medium">{{ channel.name }}</div>
              <div class="text-xs text-gray-500">{{ channel.permType === 'public' ? '公开' : '非公开' }}</div>
            </div>
            <n-button size="small" type="primary" @click="enterChannel(channel)">进入</n-button>
          </n-card>
        </template>
      </div>
    </div>
  </div>
</template>
