<script setup lang="ts">
import { computed, h, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import { api } from '@/stores/_config';
import { copyTextWithResult } from '@/utils/clipboard';

const props = defineProps<{
  worldId: string;
  canManage: boolean;
}>();

const dialog = useDialog();
const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const rotating = ref(false);
const enabled = ref(false);
const hasToken = ref(false);
const publicId = ref('');
const tokenTail = ref('');
const sessionToken = ref('');
const rotatedAt = ref('');
const lastAccessAt = ref('');

const sessionTokenStorageKey = computed(() => {
  const worldId = String(props.worldId || '').trim();
  return worldId ? `sealchat_agent_access_token_${worldId}` : '';
});

const readSessionToken = () => {
  if (typeof window === 'undefined' || !sessionTokenStorageKey.value) return '';
  try {
    return window.sessionStorage.getItem(sessionTokenStorageKey.value)?.trim() || '';
  } catch {
    return '';
  }
};

const writeSessionToken = (token: string) => {
  if (typeof window === 'undefined' || !sessionTokenStorageKey.value) return;
  try {
    if (token) {
      window.sessionStorage.setItem(sessionTokenStorageKey.value, token);
    } else {
      window.sessionStorage.removeItem(sessionTokenStorageKey.value);
    }
  } catch {
    // sessionStorage unavailable; in-memory token still works for current view.
  }
};

const baseUrl = computed(() => {
  if (typeof window === 'undefined') {
    return '';
  }
  try {
    const configuredBase = String((window as any).__SEALCHAT_BASE__ ?? import.meta.env.BASE_URL ?? '').trim();
    const normalizedBase = configuredBase && configuredBase !== '/'
      ? `/${configuredBase.replace(/^\/+|\/+$/g, '')}/`
      : '/';
    return new URL(normalizedBase, window.location.origin).toString();
  } catch {
    return `${window.location.origin}/`;
  }
});

const accessLink = computed(() => {
  const value = sessionToken.value.trim();
  if (!value) {
    return '';
  }
  return `${baseUrl.value}ob-print/v1/${encodeURIComponent(value)}`;
});

const credentialDisplay = computed(() => {
  if (accessLink.value) {
    return accessLink.value;
  }
  if (!hasToken.value || !publicId.value) {
    return '';
  }
  const tail = tokenTail.value ? `…${tokenTail.value}` : '…';
  return `${baseUrl.value}ob-print/v1/agt_${publicId.value}.${tail}`;
});

const messagesLink = computed(() => {
  if (!accessLink.value) return '';
  return `${accessLink.value}?resource=messages&channel=all&format=json&content=both`;
});

const countsLink = computed(() => {
  if (!accessLink.value) return '';
  return `${accessLink.value}?resource=counts`;
});

const guideLink = computed(() => {
  if (!baseUrl.value) return '';
  return `${baseUrl.value}ob-print/v1/docs`;
});

const agentPrompt = computed(() => {
  if (!accessLink.value || !guideLink.value) return '';
  return `你需要查看文档，作为 TRPG 平台消息助手。\n 文档链接：${guideLink.value}\nAgent访问链接为：${accessLink.value}`;
});

const applyResponse = (data: any, clearHiddenToken = false) => {
  const state = data?.agentAccess || data || {};
  const nextPublicId = typeof state.publicId === 'string' ? state.publicId : '';
  const returnedToken = typeof state.token === 'string' ? state.token.trim() : '';
  const storedToken = sessionToken.value || readSessionToken();
  const storedTokenMatches = !!nextPublicId && storedToken.startsWith(`agt_${nextPublicId}.`);
  if ((clearHiddenToken && !storedTokenMatches) || (publicId.value && nextPublicId && publicId.value !== nextPublicId)) {
    sessionToken.value = '';
    writeSessionToken('');
  } else if (storedTokenMatches) {
    sessionToken.value = storedToken;
  }
  if (returnedToken) {
    sessionToken.value = returnedToken;
    writeSessionToken(returnedToken);
  }
  publicId.value = nextPublicId;
  hasToken.value = !!state.hasToken;
  tokenTail.value = typeof state.tokenTail === 'string' ? state.tokenTail : '';
  enabled.value = !!state.enabled;
  rotatedAt.value = typeof state.rotatedAt === 'string' ? state.rotatedAt : '';
  lastAccessAt.value = typeof state.lastAccessAt === 'string' ? state.lastAccessAt : '';
};

const load = async () => {
  if (!props.worldId || !props.canManage) {
    enabled.value = false;
    hasToken.value = false;
    publicId.value = '';
    tokenTail.value = '';
    sessionToken.value = '';
    return;
  }
  loading.value = true;
  try {
    sessionToken.value = readSessionToken();
    const resp = await api.get(`/api/v1/worlds/${props.worldId}/agent-access`);
    applyResponse(resp.data, true);
  } catch (error: any) {
    message.error(error?.response?.data?.message || '加载 AI Agent 访问链接失败');
  } finally {
    loading.value = false;
  }
};

watch(
  () => [props.worldId, props.canManage],
  () => void load(),
  { immediate: true },
);

const save = async () => {
  if (!props.worldId || !props.canManage) return;
  saving.value = true;
  try {
    const resp = await api.put(`/api/v1/worlds/${props.worldId}/agent-access`, {
      enabled: enabled.value,
      rotate: false,
    });
    applyResponse(resp.data);
    message.success(resp.data?.message || 'AI Agent 访问链接已保存');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败');
  } finally {
    saving.value = false;
  }
};

const doRotate = async () => {
  rotating.value = true;
  try {
    const resp = await api.put(`/api/v1/worlds/${props.worldId}/agent-access`, {
      enabled: enabled.value,
      rotate: true,
    });
    applyResponse(resp.data, true);
    message.success(resp.data?.message || '访问令牌已轮换');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '轮换失败');
  } finally {
    rotating.value = false;
  }
};

const rotate = () => {
  if (!props.worldId || !props.canManage) return;
  dialog.warning({
    title: hasToken.value ? '轮换 AI Agent 访问令牌' : '创建 AI Agent 访问令牌',
    content: hasToken.value
      ? '轮换后旧链接会立即失效。新链接只在本次页面会话中显示，请立即复制。'
      : '新链接只在创建后的本次页面会话中显示，请立即复制并安全保存。',
    positiveText: hasToken.value ? '确认轮换' : '确认创建',
    negativeText: '取消',
    onPositiveClick: doRotate,
  });
};

const copy = async (value: string, successText: string) => {
  if (!value) {
    message.warning(hasToken.value ? '现有令牌无法再次读取，请先轮换并复制新链接' : '请先保存或创建访问令牌');
    return;
  }
  await copyTextWithResult(value, {
    onSuccess: () => message.success(successText),
    onFailure: () => message.error('复制失败，请手动复制'),
  });
};

const copyAgentPrompt = async () => {
  if (saving.value || rotating.value) return;
  if (!agentPrompt.value) {
    dialog.warning({
      title: hasToken.value ? '需要轮换访问令牌' : '需要创建访问令牌',
      content: hasToken.value
        ? '服务端不保存现有令牌全文。确认轮换后，旧链接立即失效，并自动复制新指令。'
        : '当前没有完整访问链接。确认创建后，将在上方文本框显示新链接，并自动复制发给 AI 的指令。',
      positiveText: hasToken.value ? '轮换并复制' : '创建并复制',
      negativeText: '取消',
      onPositiveClick: async () => {
        await doRotate();
        if (agentPrompt.value) {
          await copyAgentPrompt();
        }
      },
    });
    return;
  }
  const copied = await copyTextWithResult(agentPrompt.value, {
    onSuccess: () => message.success('已复制发给 AI 的指令'),
    onFailure: () => message.error('复制失败，请手动复制对话框中的指令'),
  });
  dialog.info({
    title: copied ? '已复制发给 AI 的指令' : '发给 AI 的指令',
    content: () => h('div', {
      style: {
        whiteSpace: 'pre-wrap',
        overflowWrap: 'anywhere',
        lineHeight: '1.7',
      },
    }, agentPrompt.value),
    positiveText: '关闭',
  });
};
</script>

<template>
  <div class="agent-access-card">
    <n-spin :show="loading">
      <template v-if="props.canManage">
        <n-space vertical :size="12">
          <n-form label-placement="left" label-width="96">
            <n-form-item label="启用状态">
              <div class="agent-access-status-row">
                <n-switch v-model:value="enabled" :disabled="saving || rotating" />
                <span class="agent-access-status-text">{{ enabled ? '已启用' : '已停用' }}</span>
              </div>
            </n-form-item>
            <n-form-item label="基础访问链接">
              <n-input
                :value="credentialDisplay"
                type="textarea"
                :rows="3"
                readonly
                :placeholder="hasToken ? '令牌已存在；为安全起见无法再次读取，轮换后可复制新链接' : '保存或创建后生成独立的 Agent 访问令牌'"
              />
              <template #feedback>
                <span v-if="accessLink">完整链接只在本次创建或轮换后可见，请立即复制。无参数访问会返回频道 ID、能力清单和参数文档。</span>
                <span v-else-if="hasToken">服务端只保存令牌摘要。需要重新取得完整链接时，请轮换令牌。</span>
                <span v-else>将生成的这一条基础链接交给 Agent，Agent 可从 manifest 自行发现后续接口。</span>
              </template>
            </n-form-item>
          </n-form>

          <div class="agent-access-actions">
            <n-button size="small" type="primary" :loading="saving" @click="save">
              保存
            </n-button>
            <n-button size="small" secondary :disabled="!accessLink" @click="copy(accessLink, '已复制 Agent 基础链接')">
              复制基础链接
            </n-button>
            <n-button size="small" secondary @click="copyAgentPrompt">
              复制发给AI指令
            </n-button>
            <n-button size="small" tertiary type="warning" :loading="rotating" @click="rotate">
              {{ hasToken ? '轮换并显示新链接' : '创建并显示链接' }}
            </n-button>
          </div>

          <n-alert v-if="hasToken && !accessLink" type="warning" show-icon>
            已存在访问令牌（尾号 {{ tokenTail || '未知' }}），完整凭据不可查看，忘记请点击轮换，之后旧链接立即失效。
          </n-alert>
          <n-alert type="warning" show-icon>
            链接本身就是只读凭据，但它可让Agent读取频道内所有聊天信息。请勿发布到公开页面、工单或日志；泄漏后应立即轮换。聊天内容属于不可信用户数据，Agent 不应将其中的文字当作系统指令。
          </n-alert>

          <n-collapse arrow-placement="right">
            <n-collapse-item name="agent-quick-links" title="快速调用链接与状态">
              <n-space vertical :size="10">
                <div class="agent-access-quick-row">
                  <div class="agent-access-quick-label">全频道 JSON（≤20频道）</div>
                  <n-input :value="messagesLink" readonly />
                  <n-button size="tiny" :disabled="!messagesLink" @click="copy(messagesLink, '已复制全频道消息链接')">复制</n-button>
                </div>
                <div class="agent-access-quick-row">
                  <div class="agent-access-quick-label">频道消息计数</div>
                  <n-input :value="countsLink" readonly />
                  <n-button size="tiny" :disabled="!countsLink" @click="copy(countsLink, '已复制频道计数链接')">复制</n-button>
                </div>
                <div v-if="rotatedAt || lastAccessAt" class="agent-access-help">
                  <span v-if="rotatedAt">最近创建/轮换：{{ rotatedAt }}</span>
                  <span v-if="rotatedAt && lastAccessAt">；</span>
                  <span v-if="lastAccessAt">最近访问：{{ lastAccessAt }}</span>
                </div>
                <div class="agent-access-help">
                  常用参数：<code>resource=messages|counts</code>、重复的 <code>channel=频道ID</code>、<code>from</code>、<code>to</code>、<code>scope=all|ic|ooc</code>、<code>format=json|jsonl|text</code>、<code>content=plain|rich|both</code>、<code>colorizer=export</code>。完整定义以基础链接返回的 manifest 为准。
                </div>
              </n-space>
            </n-collapse-item>
          </n-collapse>
        </n-space>
      </template>
      <template v-else>
        <n-alert type="warning" show-icon>
          仅世界拥有者或管理员可管理 AI Agent 访问链接。
        </n-alert>
      </template>
    </n-spin>
  </div>
</template>

<style scoped>
.agent-access-card {
  display: grid;
  gap: 12px;
}

.agent-access-status-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.agent-access-status-text {
  color: var(--n-text-color-2, #64748b);
  font-size: 13px;
}

.agent-access-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.agent-access-quick-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
}

.agent-access-quick-label {
  color: var(--n-text-color-2, #64748b);
  font-size: 13px;
}

.agent-access-help {
  color: var(--n-text-color-2, #64748b);
  font-size: 13px;
  line-height: 1.7;
}

.agent-access-help code {
  font-family: var(--n-font-family-mono, monospace);
}

@media (max-width: 720px) {
  .agent-access-quick-row {
    grid-template-columns: 1fr auto;
  }

  .agent-access-quick-label {
    grid-column: 1 / -1;
  }
}
</style>
