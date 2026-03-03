<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useChatStore } from '@/stores/chat';

const props = defineProps<{
  worldId: string;
  canManage: boolean;
}>();

const chat = useChatStore();
const message = useMessage();

const loading = ref(false);
const saving = ref(false);
const form = ref({
  slug: '',
  enabled: false,
});

const normalizedSlug = computed(() => form.value.slug.trim().toLowerCase());
const fullLink = computed(() => {
  if (!normalizedSlug.value) {
    return '';
  }
  const origin = typeof window !== 'undefined' ? window.location.origin : '';
  return `${origin}/#/ob/${encodeURIComponent(normalizedSlug.value)}`;
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

watch(
  () => [props.worldId, props.canManage],
  () => {
    void loadObserverLink();
  },
  { immediate: true },
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

const copyLink = async () => {
  if (!fullLink.value) {
    message.warning('请先填写并保存 OB 链接标识');
    return;
  }
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(fullLink.value);
    } else {
      const textarea = document.createElement('textarea');
      textarea.value = fullLink.value;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.focus();
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
    }
    message.success('已复制 OB 旁观链接');
  } catch {
    message.error('复制失败，请手动复制');
  }
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
          <div class="observer-link-actions">
            <n-button size="small" tertiary @click="generateSlug">随机生成</n-button>
            <n-button size="small" secondary :disabled="!normalizedSlug" @click="copyLink">
              复制链接
            </n-button>
            <n-button size="small" type="primary" :loading="saving" @click="save">保存</n-button>
          </div>
          <n-alert type="info" show-icon>
            外部用户可通过该链接免登录进入旁观模式；支持 public/private/unlisted 世界分享。
          </n-alert>
        </n-space>
      </template>
      <template v-else>
        <n-alert type="warning" show-icon>
          仅世界拥有者或管理员可管理 OB 旁观链接。
        </n-alert>
      </template>
    </n-spin>
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
