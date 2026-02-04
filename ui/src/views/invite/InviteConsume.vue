<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router';
import { onMounted, ref, computed } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { useMessage } from 'naive-ui';

const route = useRoute();
const router = useRouter();
const chat = useChatStore();
const user = useUserStore();
const message = useMessage();

const slug = computed(() => (route.params.slug as string) || (route.query.invite as string) || '');
const status = ref<'pending' | 'processing' | 'success' | 'error' | 'alreadyJoined' | 'invalid'>('pending');
const errorMessage = ref('');
const worldName = ref('');
const worldId = ref('');
const displayWorldName = computed(() => worldName.value || '该世界');

const processInvite = async () => {
  if (!slug.value) {
    status.value = 'error';
    errorMessage.value = '缺少邀请码';
    return;
  }
  if (!user.token) {
    router.replace({ name: 'user-signin', query: { redirect: route.fullPath } });
    return;
  }
  status.value = 'processing';
  try {
    await chat.ensureConnectionReady();
    const resp = await chat.consumeWorldInvite(slug.value);
    const respWorldId = resp.world?.id;
    const respWorldName = resp.world?.name || '目标世界';
    const alreadyJoined = !!resp.already_joined;
    if (respWorldId) {
      worldId.value = respWorldId;
      worldName.value = respWorldName;
    }
    if (alreadyJoined && respWorldId) {
      status.value = 'alreadyJoined';
      errorMessage.value = '';
      return;
    }
    if (respWorldId) {
      await chat.switchWorld(respWorldId, { force: true });
      status.value = 'success';
      message.success('已加入世界');
      try {
        await router.replace({ name: 'world-channel', params: { worldId: respWorldId } });
      } catch (err) {
        console.warn('router replace failed', err);
      }
      if (router.currentRoute.value.name !== 'world-channel') {
        window.location.hash = `#/${respWorldId}`;
      }
    } else {
      status.value = 'error';
      errorMessage.value = '加入失败，世界信息缺失';
    }
  } catch (e: any) {
    const msg = e?.response?.data?.message || '加入失败';
    if (msg.includes('邀请链接无效或已过期')) {
      status.value = 'invalid';
      errorMessage.value = '邀请链接无效或已过期';
      return;
    }
    status.value = 'error';
    errorMessage.value = msg;
  }
};

const gotoWorld = async () => {
  if (!worldId.value) {
    goBack();
    return;
  }
  try {
    await chat.switchWorld(worldId.value, { force: true });
    await router.replace({ name: 'world-channel', params: { worldId: worldId.value } });
  } catch (err) {
    message.error('跳转失败，请稍后重试');
  }
};

const goBack = () => {
  const canBack = window.history.length > 1;
  if (canBack) {
    router.back();
  } else {
    router.replace({ name: 'home' }).catch(() => {
      window.location.hash = '#/';
    });
  }
};

onMounted(processInvite);
</script>

<template>
  <div class="w-full h-full flex items-center justify-center p-6">
    <div class="text-center space-y-4">
      <n-spin :show="status === 'processing'">
        <template v-if="status === 'pending' || status === 'processing'">
          <p>正在验证邀请链接...</p>
        </template>
        <template v-else-if="status === 'success'">
          <p>邀请成功，正在跳转...</p>
        </template>
        <template v-else-if="status === 'alreadyJoined'">
          <n-alert type="info" title="您已在此世界">
            您已经加入了「{{ displayWorldName }}」。
          </n-alert>
          <div class="flex justify-center gap-3 pt-2">
            <n-button type="primary" @click="gotoWorld" :disabled="!worldId">进入世界</n-button>
            <n-button tertiary @click="goBack">返回</n-button>
          </div>
        </template>
        <template v-else-if="status === 'invalid'">
          <n-alert type="error" title="邀请链接无效或已过期">{{ errorMessage }}</n-alert>
          <div class="flex justify-center gap-3 pt-2">
            <n-button @click="goBack">返回</n-button>
          </div>
        </template>
        <template v-else>
          <n-alert type="error" title="加入失败">{{ errorMessage }}</n-alert>
          <div class="flex justify-center gap-3 pt-2">
            <n-button @click="goBack">返回</n-button>
          </div>
        </template>
      </n-spin>
    </div>
  </div>
</template>
