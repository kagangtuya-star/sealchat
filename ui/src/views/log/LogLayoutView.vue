<script setup lang="ts">
// 设计入口：解析 slug → 能力与草稿。slug 不是权限凭据，服务端每次重新判定。
import { computed, ref, shallowRef, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { describeLogError, isLogError, resolveLogLayout } from '@/services/log/logApi';
import type { LogProjectContext } from '@/types/log';
import LogWorkbench from './components/LogWorkbench.vue';
import type { FailedElementMove } from './logDropOps';

const route = useRoute();
const router = useRouter();
const slug = computed(() => String(route.params.slug || ''));
const context = shallowRef<LogProjectContext | null>(null);
const state = ref<'loading' | 'ready' | 'forbidden' | 'unavailable' | 'unauthenticated' | 'error'>('loading');
const errorText = ref('');
const reloadKey = ref(0);
const failedMove = shallowRef<FailedElementMove | null>(null);
let seq = 0;

const load = async () => {
  const id = ++seq;
  const s = slug.value;
  if (!/^[a-z0-9]{4,64}$/.test(s)) {
    state.value = 'unavailable';
    return;
  }
  state.value = 'loading';
  try {
    const ctx = await resolveLogLayout(s);
    if (id !== seq) return;
    context.value = ctx;
    reloadKey.value += 1;
    state.value = 'ready';
  } catch (error) {
    if (id !== seq) return;
    context.value = null;
    errorText.value = describeLogError(error, '加载失败');
    if (isLogError(error, 'UNAUTHENTICATED')) state.value = 'unauthenticated';
    else if (isLogError(error, 'PERMISSION_DENIED')) state.value = 'forbidden';
    else if (isLogError(error, 'NOT_FOUND')) state.value = 'unavailable';
    else state.value = 'error';
  }
};

watch(slug, () => { failedMove.value = null; void load(); }, { immediate: true });

const goSignIn = () => router.replace({ name: 'user-signin', query: { redirect: `/log/layout/${slug.value}` } });
const goHome = () => router.push({ name: 'world-lobby' });

/** 冲突后重新加载：获取最新工程并以服务端版本重建工作台。 */
const reloadContext = async () => {
  await load();
};
</script>

<template>
  <div class="log-layout-view">
    <LogWorkbench
      v-if="state === 'ready' && context"
      :key="`${context.projectId}:${reloadKey}`"
      :context="context"
      :slug="slug"
      v-model:failed-move="failedMove"
      @reload="reloadContext"
    />
    <div v-else class="log-layout-view__state">
      <n-spin v-if="state === 'loading'" size="large" />
      <n-result v-else-if="state === 'forbidden'" status="403" title="无权进入 LOG 排版器" :description="errorText || '需要世界管理者身份，或管理者开启了成员访问；也可能你无法读取全部来源频道。'">
        <template #footer><n-button @click="goHome">返回世界大厅</n-button></template>
      </n-result>
      <n-result v-else-if="state === 'unavailable'" status="404" title="排版入口不可用" description="链接无效、已更换或排版器已停用。请向世界管理者索取新的设计链接。">
        <template #footer><n-button @click="goHome">返回世界大厅</n-button></template>
      </n-result>
      <n-result v-else-if="state === 'unauthenticated'" status="info" title="请先登录" description="设计入口需要登录；登录后会回到此页面。">
        <template #footer><n-button type="primary" @click="goSignIn">去登录</n-button></template>
      </n-result>
      <n-result v-else status="error" title="加载失败" :description="errorText">
        <template #footer><n-button @click="load">重试</n-button></template>
      </n-result>
    </div>
  </div>
</template>

<style scoped>
.log-layout-view {
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  background: var(--sc-bg-surface, #f5f5f4);
}

.log-layout-view__state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}
</style>
