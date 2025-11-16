<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useWorldStore } from '@/stores/world';
import { useRouter } from 'vue-router';
import { useUserStore } from '@/stores/user';
import { NCard, NSkeleton, NButton, NInput } from 'naive-ui';

const worldStore = useWorldStore();
const router = useRouter();
const userStore = useUserStore();
const keyword = ref('');

const goWorld = async (slug: string) => {
  if (!slug) return;
  await router.push(`/worlds/${slug}`);
};

const fetchData = async () => {
  await worldStore.fetchWorlds(keyword.value);
};

onMounted(fetchData);
</script>

<template>
  <div class="p-4 max-w-5xl mx-auto">
    <div class="flex items-center justify-between mb-4 gap-2">
      <div class="flex items-center gap-2">
        <n-input v-model:value="keyword" placeholder="搜索世界" clearable @keyup.enter="fetchData" />
        <n-button secondary @click="fetchData">搜索</n-button>
      </div>
      <n-button
        v-if="userStore.checkPerm('func_world_create')"
        type="primary"
        @click="$router.push('/worlds/new')"
      >
        创建世界
      </n-button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
      <template v-if="worldStore.loading">
        <n-skeleton v-for="i in 6" :key="i" height="120px" />
      </template>
      <template v-else>
        <n-card
          v-for="world in worldStore.list"
          :key="world.id"
          class="cursor-pointer"
          :title="world.name"
          @click="goWorld(world.slug)"
        >
          <div class="text-sm text-gray-500 line-clamp-2 mb-2">{{ world.description || '暂无简介' }}</div>
          <div class="text-xs text-gray-400">
            {{ world.visibility === 'private' ? '私密' : '公开' }} · 成员 {{ world.memberCount ?? '--' }}
          </div>
        </n-card>
      </template>
    </div>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
