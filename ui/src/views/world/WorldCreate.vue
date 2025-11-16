<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useWorldStore } from '@/stores/world';
import { api } from '@/stores/_config';
import { useMessage } from 'naive-ui';

const form = reactive({
  name: '',
  description: '',
  visibility: 'public',
  joinPolicy: 'open',
});
const loading = ref(false);
const router = useRouter();
const worldStore = useWorldStore();
const message = useMessage();

const handleSubmit = async () => {
  if (!form.name.trim()) {
    message.error('请输入世界名称');
    return;
  }
  loading.value = true;
  try {
    const resp = await api.post<{ world: any }>('/api/v1/worlds', form);
    await worldStore.fetchWorlds();
    const slug = resp.data?.world?.slug;
    message.success('世界创建成功');
    if (slug) {
      router.push(`/worlds/${slug}`);
    } else {
      router.push('/worlds');
    }
  } finally {
    loading.value = false;
  }
};
</script>

<template>
  <div class="max-w-xl mx-auto p-4">
    <h2 class="text-2xl font-semibold mb-4">创建新世界</h2>
    <form class="space-y-4" @submit.prevent="handleSubmit">
      <div>
        <label class="block text-sm font-medium text-gray-600 mb-1">名称</label>
        <input
          v-model="form.name"
          class="sc-input"
          placeholder="请输入世界名称"
          required
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-600 mb-1">简介</label>
        <textarea
          v-model="form.description"
          class="sc-input"
          rows="4"
          placeholder="介绍一下这个世界"
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-600 mb-1">可见性</label>
        <select v-model="form.visibility" class="sc-input">
          <option value="public">公开</option>
          <option value="private">私密</option>
        </select>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-600 mb-1">加入策略</label>
        <select v-model="form.joinPolicy" class="sc-input">
          <option value="open">开放加入</option>
          <option value="approval">需审批</option>
          <option value="invite_only">仅邀请</option>
        </select>
      </div>
      <div class="flex gap-2">
        <button type="submit" class="sc-btn sc-btn-primary" :disabled="loading">
          {{ loading ? '创建中...' : '创建' }}
        </button>
        <button type="button" class="sc-btn" @click="$router.back()">取消</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.sc-input {
  width: 100%;
  border: 1px solid rgba(148, 163, 184, 0.6);
  border-radius: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: rgba(255, 255, 255, 0.85);
  color: inherit;
}
.sc-btn {
  border-radius: 0.5rem;
  padding: 0.4rem 1rem;
  border: 1px solid rgba(148, 163, 184, 0.6);
}
.sc-btn-primary {
  background: linear-gradient(135deg, #0284c7, #0ea5e9);
  color: white;
  border: none;
}
</style>
