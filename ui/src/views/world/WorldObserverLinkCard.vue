<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import { copyTextWithResult } from '@/utils/clipboard';

const props = defineProps<{
  worldId: string;
  canManage: boolean;
}>();

const chat = useChatStore();
const message = useMessage();
const PRINT_LINK_OPTIONS_STORAGE_KEY = 'observerPrintLinkOptions';

const loading = ref(false);
const saving = ref(false);
const printOptionsVisible = ref(false);
const printChannelOptionsLoading = ref(false);
const form = ref({
  slug: '',
  enabled: false,
});
const printOptions = ref({
  channelId: '',
  messageScope: 0,
  showArchived: true,
  showTimestamp: true,
  showColorCode: false,
});
const messageScopeOptions = [
  { label: '都显示', value: 0 },
  { label: '只场外', value: 1 },
  { label: '只场内', value: 2 },
];
const printChannelOptions = computed(() => {
  const tree = (props.worldId && chat.channelTreeByWorld?.[props.worldId]) || [];
  const result: Array<{ label: string; value: string }> = [];
  const walk = (nodes: any[], depth = 0) => {
    nodes.forEach((node) => {
      if (!node?.id) {
        return;
      }
      const indent = depth ? `${'· '.repeat(depth)}` : '';
      result.push({ label: `${indent}${node.name || node.id}`, value: node.id });
      if (Array.isArray(node.children) && node.children.length > 0) {
        walk(node.children, depth + 1);
      }
    });
  };
  walk(tree);
  return result;
});

const buildPrintLink = (slug: string, options?: typeof printOptions.value) => {
  const normalized = slug.trim().toLowerCase();
  if (!normalized) {
    return '';
  }
  const params = new URLSearchParams();
  const current = options || printOptions.value;
  const channelId = current.channelId.trim();
  if (channelId) {
    params.set('channel_id', channelId);
  }
  params.set('message_scope', String(current.messageScope));
  params.set('show_archived', current.showArchived ? '1' : '0');
  params.set('show_timestamp', current.showTimestamp ? '1' : '0');
  params.set('show_color_code', current.showColorCode ? '1' : '0');
  return `${baseUrl.value}ob-print/${encodeURIComponent(normalized)}?${params.toString()}`;
};

const readPrintOptionsStorage = () => {
  if (typeof window === 'undefined') {
    return null;
  }
  try {
    const raw = localStorage.getItem(`${PRINT_LINK_OPTIONS_STORAGE_KEY}:${props.worldId || 'global'}`);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw);
    return {
      channelId: typeof parsed?.channelId === 'string' ? parsed.channelId : '',
      messageScope: [0, 1, 2].includes(Number(parsed?.messageScope)) ? Number(parsed.messageScope) : 0,
      showArchived: parsed?.showArchived !== false,
      showTimestamp: parsed?.showTimestamp !== false,
      showColorCode: parsed?.showColorCode === true,
    };
  } catch {
    return null;
  }
};

const writePrintOptionsStorage = (value: typeof printOptions.value) => {
  if (typeof window === 'undefined') {
    return;
  }
  try {
    localStorage.setItem(`${PRINT_LINK_OPTIONS_STORAGE_KEY}:${props.worldId || 'global'}`, JSON.stringify(value));
  } catch {
    // noop
  }
};

const restorePrintOptions = () => {
  const stored = readPrintOptionsStorage();
  printOptions.value = stored || {
    channelId: '',
    messageScope: 0,
    showArchived: true,
    showTimestamp: true,
    showColorCode: false,
  };
};

const normalizedSlug = computed(() => form.value.slug.trim().toLowerCase());
const baseUrl = computed(() => {
  if (typeof window === 'undefined') {
    return '';
  }
  try {
    return new URL(import.meta.env.BASE_URL || '/', window.location.origin).toString();
  } catch {
    return `${window.location.origin}/`;
  }
});
const fullLink = computed(() => {
  if (!normalizedSlug.value) {
    return '';
  }
  return `${baseUrl.value}#/ob/${encodeURIComponent(normalizedSlug.value)}`;
});

const printLink = computed(() => {
  return buildPrintLink(normalizedSlug.value);
});

const loadObserverLink = async () => {
  if (!props.worldId || !props.canManage) {
    form.value.slug = '';
    form.value.enabled = false;
    return;
  }
  loading.value = true;
  try {
    const resp = await chat.worldObserverLinkGet(props.worldId);
    form.value.slug = typeof resp?.slug === 'string' ? resp.slug : '';
    form.value.enabled = !!resp?.enabled;
  } catch (error: any) {
    message.error(error?.response?.data?.message || '加载 OB 旁观链接失败');
  } finally {
    loading.value = false;
  }
};

const ensurePrintChannelOptionsLoaded = async () => {
  if (!props.worldId || !props.canManage) {
    return;
  }
  if ((chat.channelTreeByWorld?.[props.worldId] || []).length > 0) {
    return;
  }
  printChannelOptionsLoading.value = true;
  try {
    await chat.channelList(props.worldId, true);
  } catch (error: any) {
    message.error(error?.message || error?.response?.data?.message || '加载频道列表失败');
  } finally {
    printChannelOptionsLoading.value = false;
  }
};

watch(
  () => [props.worldId, props.canManage],
  () => {
    restorePrintOptions();
    void loadObserverLink();
  },
  { immediate: true },
);

watch(
  () => printOptions.value,
  (value) => {
    writePrintOptionsStorage(value);
  },
  { deep: true },
);

const randomSegment = (length: number) => {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  const size = Math.max(1, length);
  let output = '';
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    const bytes = new Uint8Array(size);
    crypto.getRandomValues(bytes);
    for (let i = 0; i < size; i += 1) {
      output += chars[bytes[i] % chars.length];
    }
    return output;
  }
  for (let i = 0; i < size; i += 1) {
    output += chars[Math.floor(Math.random() * chars.length)];
  }
  return output;
};

const generateSlug = () => {
  form.value.slug = `ob-${randomSegment(8)}`;
};

const save = async () => {
  if (!props.worldId || !props.canManage) {
    return;
  }
  const slug = normalizedSlug.value;
  if (form.value.enabled && !slug) {
    message.warning('启用前请先填写 OB 链接标识');
    return;
  }
  saving.value = true;
  try {
    const resp = await chat.worldObserverLinkUpdate(props.worldId, {
      slug,
      enabled: form.value.enabled,
    });
    const observerLink = resp?.observerLink || {};
    form.value.slug = typeof observerLink.slug === 'string' ? observerLink.slug : slug;
    form.value.enabled = !!observerLink.enabled;
    message.success('OB 旁观链接已保存');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败');
  } finally {
    saving.value = false;
  }
};

const copyText = async (value: string, successText: string) => {
  if (!value) {
    message.warning('请先填写并保存 OB 链接标识');
    return;
  }
  await copyTextWithResult(value, {
    onSuccess: () => {
      message.success(successText);
    },
    onFailure: () => {
      message.error('复制失败，请手动复制');
    },
  });
};

const copyLink = async () => copyText(fullLink.value, '已复制 OB 旁观链接');

const copyPrintLink = async () => copyText(printLink.value, '已复制打印链接');

const openPrintOptions = () => {
  if (!normalizedSlug.value) {
    message.warning('请先填写并保存 OB 链接标识');
    return;
  }
  void ensurePrintChannelOptionsLoaded();
  printOptionsVisible.value = true;
};

const resetPrintOptions = () => {
  printOptions.value = {
    channelId: '',
    messageScope: 0,
    showArchived: true,
    showTimestamp: true,
    showColorCode: false,
  };
};

const generatePrintLink = async () => {
  await copyText(printLink.value, '已生成并复制打印链接');
};
</script>

<template>
  <div class="observer-link-card">
    <n-spin :show="loading">
      <template v-if="props.canManage">
        <n-space vertical :size="12">
          <n-form label-placement="left" label-width="86">
            <n-form-item label="链接标识">
              <n-input
                v-model:value="form.slug"
                :disabled="saving"
                placeholder="例如 ob-demo-01（4-32位小写字母/数字/-/_）"
              />
            </n-form-item>
            <n-form-item label="启用状态">
              <div class="observer-link-status-row">
                <n-switch v-model:value="form.enabled" :disabled="saving" />
                <span class="observer-link-status-text">{{ form.enabled ? '已启用' : '已停用' }}</span>
              </div>
            </n-form-item>
          </n-form>
          <n-input
            :value="fullLink"
            readonly
            placeholder="保存后可复制完整链接"
          />
          <n-input
            :value="printLink"
            readonly
            placeholder="保存后可复制打印链接"
          />
          <div class="observer-link-actions">
            <n-button size="small" tertiary @click="generateSlug">随机生成</n-button>
            <n-button size="small" secondary :disabled="!normalizedSlug" @click="copyLink">
              复制旁观链接
            </n-button>
            <n-button size="small" secondary :disabled="!normalizedSlug" @click="copyPrintLink">
              复制打印链接
            </n-button>
            <n-button size="small" secondary :disabled="!normalizedSlug" @click="openPrintOptions">
              打印链接选项
            </n-button>
            <n-button size="small" type="primary" :loading="saving" @click="save">保存</n-button>
          </div>
          <n-alert type="info" show-icon>
            外部用户可通过该链接免登录进入旁观模式；支持 public/private/unlisted 世界分享。
          </n-alert>
          <n-alert type="info" show-icon>
            打印链接会直接返回可抓取的频道文本快照；如需固定频道，可在链接后追加 <code>channel_id=频道ID</code>，并可用 <code>show_timestamp</code>、<code>show_color_code</code> 控制时间与颜色代码显示。
          </n-alert>
        </n-space>
      </template>
      <template v-else>
        <n-alert type="warning" show-icon>
          仅世界拥有者或管理员可管理 OB 旁观链接。
        </n-alert>
      </template>
    </n-spin>

    <n-modal
      :show="printOptionsVisible"
      preset="card"
      title="打印链接选项"
      style="width: 560px; max-width: 96vw"
      :auto-focus="false"
      @update:show="printOptionsVisible = $event"
    >
      <n-space vertical :size="12">
        <n-alert type="info" :show-icon="false">
          这里的配置会保存在当前浏览器本地，下次打开世界管理页时自动恢复。
        </n-alert>
        <n-form :model="printOptions" label-placement="left" label-width="108">
          <n-form-item label="频道选择">
            <n-select
              v-model:value="printOptions.channelId"
              clearable
              filterable
              :loading="printChannelOptionsLoading"
              :options="printChannelOptions"
              placeholder="留空则使用默认 OB 入口频道"
            />
            <template #feedback>
              选择当前世界中的目标频道；若留空，则使用默认 OB 入口频道。
            </template>
          </n-form-item>
          <n-form-item label="场内外消息">
            <n-select
              v-model:value="printOptions.messageScope"
              :options="messageScopeOptions"
            />
          </n-form-item>
          <n-form-item label="显示归档">
            <n-switch v-model:value="printOptions.showArchived" />
          </n-form-item>
          <n-form-item label="显示时间">
            <n-switch v-model:value="printOptions.showTimestamp" />
          </n-form-item>
          <n-form-item label="显示颜色代码">
            <n-switch v-model:value="printOptions.showColorCode" />
          </n-form-item>
          <n-form-item label="生成结果">
            <n-input
              :value="printLink"
              type="textarea"
              :rows="4"
              readonly
            />
          </n-form-item>
        </n-form>
        <div class="observer-link-actions observer-link-actions--end">
          <n-button @click="resetPrintOptions">恢复默认</n-button>
          <n-button @click="printOptionsVisible = false">关闭</n-button>
          <n-button type="primary" :disabled="!normalizedSlug" @click="generatePrintLink">
            一键生成并复制
          </n-button>
        </div>
      </n-space>
    </n-modal>
  </div>
</template>

<style scoped>
.observer-link-card {
  display: grid;
  gap: 12px;
}

.observer-link-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.observer-link-actions--end {
  justify-content: flex-end;
}

.observer-link-status-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.observer-link-status-text {
  color: var(--n-text-color-2, #64748b);
  font-size: 13px;
}
</style>
