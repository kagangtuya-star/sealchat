<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { NAlert, NButton, NCard, NCheckbox, NCheckboxGroup, NDivider, NInput, NSpace, NTag, useDialog } from 'naive-ui';
import { api, urlBase } from '@/stores/_config';

type WebhookIntegrationItem = {
  id: string;
  channelId: string;
  name: string;
  source: string;
  botUserId: string;
  status: 'active' | 'revoked' | string;
  createdAt: number;
  createdBy: string;
  lastUsedAt: number;
  tokenTailFragment: string;
  capabilities: string[];
};

const props = defineProps<{
  channelId: string;
}>();

const dialog = useDialog();
const loading = ref(false);
const errorText = ref('');
const items = ref<WebhookIntegrationItem[]>([]);

const createName = ref('Foundry');
const createSource = ref('foundry');
const createCapabilities = ref<string[]>([
  'read_changes',
  'read_digest',
  'write_create',
  'write_update_own',
  'write_delete_own',
  'identity_upsert',
]);

const lastIssuedToken = ref<string>('');
const lastIssuedHint = ref<string>('');
const WEBHOOK_TOKEN_STORAGE_KEY = 'sealchat_webhook_token_cache';
const WEBHOOK_INTEGRATION_STORAGE_KEY = 'sealchat_webhook_integration_cache';

const hasChannel = computed(() => !!props.channelId && props.channelId.trim().length > 0);
const accessBaseUrl = computed(() => {
  if (/^https?:\/\//i.test(urlBase)) {
    return urlBase;
  }
  if (urlBase.startsWith('//')) {
    return `${window.location.protocol}${urlBase}`;
  }
  return `${window.location.origin}${urlBase.startsWith('/') ? '' : '/'}${urlBase}`;
});
const baseWebhookUrl = computed(() => `${accessBaseUrl.value}/api/v1/webhook/channels/${props.channelId}`);

const readTokenCache = (): Record<string, string> => {
  try {
    const raw = localStorage.getItem(WEBHOOK_TOKEN_STORAGE_KEY) || '{}';
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') {
      return parsed as Record<string, string>;
    }
  } catch {
    // ignore
  }
  return {};
};

const writeChannelTokenCache = (channelId: string, token: string) => {
  const normalizedChannelId = (channelId || '').trim();
  const normalizedToken = (token || '').trim();
  if (!normalizedChannelId || !normalizedToken) {
    return;
  }
  const next = readTokenCache();
  next[normalizedChannelId] = normalizedToken;
  try {
    localStorage.setItem(WEBHOOK_TOKEN_STORAGE_KEY, JSON.stringify(next));
  } catch {
    // ignore
  }
};

const writeChannelIntegrationCache = (channelId: string, integrationId: string) => {
  const normalizedChannelId = (channelId || '').trim();
  const normalizedIntegrationId = (integrationId || '').trim();
  if (!normalizedChannelId || !normalizedIntegrationId) {
    return;
  }
  try {
    const raw = localStorage.getItem(WEBHOOK_INTEGRATION_STORAGE_KEY) || '{}';
    const parsed = JSON.parse(raw);
    const next = parsed && typeof parsed === 'object' ? parsed as Record<string, string> : {};
    next[normalizedChannelId] = normalizedIntegrationId;
    localStorage.setItem(WEBHOOK_INTEGRATION_STORAGE_KEY, JSON.stringify(next));
  } catch {
    // ignore
  }
};

const formatTime = (ms?: number) => {
  if (!ms || ms <= 0) return '-';
  try {
    return new Date(ms).toLocaleString();
  } catch {
    return String(ms);
  }
};

const refresh = async () => {
  if (!hasChannel.value) return;
  loading.value = true;
  errorText.value = '';
  try {
    const resp = await api.get<{ items: WebhookIntegrationItem[] }>(`/api/v1/channels/${props.channelId}/webhook-integrations`);
    items.value = resp.data?.items || [];
  } catch (e: any) {
    errorText.value = e?.response?.data?.message || e?.message || '加载失败';
  } finally {
    loading.value = false;
  }
};

const copyText = async (text: string) => {
  const trimmed = (text || '').trim();
  if (!trimmed) return;
  try {
    await navigator.clipboard.writeText(trimmed);
  } catch {
    // 忽略：某些浏览器/非 https 环境可能失败
  }
};

const createIntegration = async () => {
  if (!hasChannel.value) return;
  lastIssuedToken.value = '';
  lastIssuedHint.value = '';
  loading.value = true;
  errorText.value = '';
  try {
    const resp = await api.post<{ item: WebhookIntegrationItem; token: string }>(`/api/v1/channels/${props.channelId}/webhook-integrations`, {
      name: createName.value,
      source: createSource.value,
      capabilities: createCapabilities.value,
    });
    lastIssuedToken.value = resp.data?.token || '';
    lastIssuedHint.value = resp.data?.item?.tokenTailFragment || '';
    writeChannelIntegrationCache(props.channelId, resp.data?.item?.id || '');
    writeChannelTokenCache(props.channelId, lastIssuedToken.value);
    await refresh();
    if (lastIssuedToken.value) await copyText(lastIssuedToken.value);
  } catch (e: any) {
    errorText.value = e?.response?.data?.message || e?.message || '创建失败';
  } finally {
    loading.value = false;
  }
};

const rotateToken = async (item: WebhookIntegrationItem) => {
  if (!hasChannel.value) return;
  dialog.warning({
    title: '轮换 Token',
    content: '轮换后旧 token 将立即失效，确认继续？',
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      lastIssuedToken.value = '';
      lastIssuedHint.value = '';
      loading.value = true;
      errorText.value = '';
      try {
        const resp = await api.post<{ token: string }>(`/api/v1/channels/${props.channelId}/webhook-integrations/${item.id}/rotate`, {});
        lastIssuedToken.value = resp.data?.token || '';
        lastIssuedHint.value = item.tokenTailFragment || '';
        writeChannelIntegrationCache(props.channelId, item.id);
        writeChannelTokenCache(props.channelId, lastIssuedToken.value);
        await refresh();
        if (lastIssuedToken.value) await copyText(lastIssuedToken.value);
      } catch (e: any) {
        errorText.value = e?.response?.data?.message || e?.message || '轮换失败';
      } finally {
        loading.value = false;
      }
    },
  });
};

const revokeIntegration = async (item: WebhookIntegrationItem) => {
  if (!hasChannel.value) return;
  dialog.warning({
    title: '撤销授权',
    content: '撤销后 token 将立即失效，确认继续？',
    positiveText: '确认撤销',
    negativeText: '取消',
    onPositiveClick: async () => {
      loading.value = true;
      errorText.value = '';
      try {
        await api.post(`/api/v1/channels/${props.channelId}/webhook-integrations/${item.id}/revoke`, {});
        await refresh();
      } catch (e: any) {
        errorText.value = e?.response?.data?.message || e?.message || '撤销失败';
      } finally {
        loading.value = false;
      }
    },
  });
};

watch(() => props.channelId, refresh, { immediate: true });
onMounted(refresh);
</script>

<template>
  <div class="p-3">
    <n-alert type="info" :bordered="false" class="mb-3">
      外部系统使用：
      <div class="mt-1 text-xs">
        GET：<code>{{ baseWebhookUrl }}/changes</code>
        <br />
        GET：<code>{{ baseWebhookUrl }}/digests</code>
        <br />
        POST：<code>{{ baseWebhookUrl }}/messages</code>
      </div>
    </n-alert>

    <n-alert v-if="errorText" type="error" :bordered="false" class="mb-3">
      {{ errorText }}
    </n-alert>

    <n-card title="创建授权" size="small" class="mb-3">
      <n-space vertical size="small">
        <n-input v-model:value="createName" placeholder="名称（例如 Foundry）" />
        <n-input v-model:value="createSource" placeholder="来源标识（例如 foundry）" />
        <div class="text-sm">能力</div>
        <n-checkbox-group v-model:value="createCapabilities">
          <n-space wrap>
            <n-checkbox value="read_changes">read_changes</n-checkbox>
            <n-checkbox value="read_digest">read_digest</n-checkbox>
            <n-checkbox value="write_create">write_create</n-checkbox>
            <n-checkbox value="write_update_own">write_update_own</n-checkbox>
            <n-checkbox value="write_delete_own">write_delete_own</n-checkbox>
            <n-checkbox value="identity_upsert">identity_upsert</n-checkbox>
          </n-space>
        </n-checkbox-group>
        <n-space justify="end">
          <n-button :loading="loading" type="primary" @click="createIntegration">创建并复制 token</n-button>
        </n-space>
        <n-alert v-if="lastIssuedToken" type="success" :bordered="false">
          token（已尝试复制到剪贴板，仅显示一次）：
          <div class="mt-1 break-all font-mono text-xs">{{ lastIssuedToken }}</div>
          <n-space class="mt-2" justify="end">
            <n-button size="small" @click="copyText(lastIssuedToken)">复制 token</n-button>
          </n-space>
        </n-alert>
      </n-space>
    </n-card>

    <n-card title="已创建授权" size="small">
      <n-space justify="space-between" align="center" class="mb-2">
        <div class="text-xs text-gray-500">当前频道：{{ channelId || '-' }}</div>
        <n-button size="small" :loading="loading" @click="refresh">刷新</n-button>
      </n-space>
      <n-divider class="my-2" />
      <div v-if="items.length === 0" class="text-sm text-gray-500">暂无授权</div>
      <div v-for="it in items" :key="it.id" class="mb-3">
        <div class="flex items-center justify-between">
          <div class="font-bold">
            {{ it.name }}
            <n-tag v-if="it.status !== 'active'" size="small" type="warning" class="ml-2">{{ it.status }}</n-tag>
          </div>
          <n-space>
            <n-button size="small" :disabled="it.status !== 'active' || loading" @click="rotateToken(it)">轮换</n-button>
            <n-button size="small" type="error" :disabled="it.status !== 'active' || loading" @click="revokeIntegration(it)">撤销</n-button>
          </n-space>
        </div>
        <div class="text-xs text-gray-500 mt-1">
          source: <code>{{ it.source }}</code> · token 尾号: <code>{{ it.tokenTailFragment || '-' }}</code>
        </div>
        <div class="text-xs text-gray-500">
          创建时间：{{ formatTime(it.createdAt) }} · 最近使用：{{ formatTime(it.lastUsedAt) }}
        </div>
        <n-space wrap class="mt-1">
          <n-tag v-for="cap in it.capabilities || []" :key="cap" size="small">{{ cap }}</n-tag>
        </n-space>
      </div>
    </n-card>
  </div>
</template>
