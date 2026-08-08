<script lang="tsx" setup>
import { computed, ref, watch } from 'vue';
import dayjs from 'dayjs';
import { useUtilsStore } from '@/stores/utils';

interface UpdateStatus {
  currentVersion?: string;
  latestTag?: string;
  latestName?: string;
  latestBody?: string;
  latestPublishedAt?: number;
  latestHtmlUrl?: string;
  lastCheckedAt?: number;
  hasUpdate?: boolean;
}

const props = withDefaults(defineProps<{ items?: any[]; visible?: boolean }>(), {
  items: () => [],
  visible: false,
});
const emit = defineEmits<{
  (event: 'close'): void;
  (event: 'open-update-settings'): void;
}>();

const list = computed(() => props.items || []);
const updateItem = computed(() => list.value.find((item) => item?.type === 'system.update'));
const otherItems = computed(() => list.value.filter((item) => item?.type !== 'system.update'));
const hasUpdateItem = computed(() => !!updateItem.value);

const utils = useUtilsStore();
const updateStatus = ref<UpdateStatus | null>(null);
const updateLoading = ref(false);
const updateError = ref('');
const updateBodyExpanded = ref(true);

const fetchUpdateStatus = async () => {
  if (!hasUpdateItem.value) {
    updateStatus.value = null;
    updateError.value = '';
    return;
  }
  updateLoading.value = true;
  updateError.value = '';
  try {
    const resp = await utils.adminUpdateStatus();
    updateStatus.value = resp.data as UpdateStatus;
  } catch (err) {
    updateError.value = '获取更新内容失败';
  } finally {
    updateLoading.value = false;
  }
};

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      fetchUpdateStatus();
    }
  },
  { immediate: true },
);

watch(hasUpdateItem, (value) => {
  if (value && props.visible) {
    fetchUpdateStatus();
  }
});

const escapeHtml = (text: string) => {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
};

const formatInline = (text: string) => {
  let result = escapeHtml(text);
  result = result.replace(/`([^`]+)`/g, '<code>$1</code>');
  result = result.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  result = result.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  result = result.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  result = result.replace(/!\[([^\]]*)\]\((https?:\/\/[^\s)]+)\)/g, '<img src="$2" alt="$1" />');
  return result;
};

const renderMarkdown = (text: string) => {
  const lines = (text || '').split(/\r?\n/);
  let html = '';
  let inList = false;
  lines.forEach((raw) => {
    const line = raw.trimEnd();
    if (line.startsWith('- ') || line.startsWith('* ')) {
      if (!inList) {
        html += '<ul>';
        inList = true;
      }
      html += `<li>${formatInline(line.slice(2).trim())}</li>`;
      return;
    }
    if (inList) {
      html += '</ul>';
      inList = false;
    }
    if (line.startsWith('### ')) {
      html += `<h3>${formatInline(line.slice(4).trim())}</h3>`;
      return;
    }
    if (line.startsWith('## ')) {
      html += `<h2>${formatInline(line.slice(3).trim())}</h2>`;
      return;
    }
    if (line.startsWith('# ')) {
      html += `<h1>${formatInline(line.slice(2).trim())}</h1>`;
      return;
    }
    if (line === '') {
      html += '<br />';
      return;
    }
    html += `<p>${formatInline(line)}</p>`;
  });
  if (inList) {
    html += '</ul>';
  }
  return html;
};

const updateTitle = computed(() => {
  if (updateStatus.value?.latestTag) {
    return `发现新版本 ${updateStatus.value.latestTag}`;
  }
  return updateItem.value?.title || '发现新版本';
});

const releaseName = computed(() => updateStatus.value?.latestName || updateItem.value?.brief || '');
const releaseLink = computed(() => updateStatus.value?.latestHtmlUrl || updateItem.value?.locPostId || '');
const updatePublishedAtText = computed(() => {
  const ts = updateStatus.value?.latestPublishedAt;
  if (ts) {
    return dayjs(ts).format('YYYY-MM-DD HH:mm:ss');
  }
  return updateItem.value?.createdAt || '未知';
});
const updateBodyRaw = computed(() => (updateStatus.value?.latestBody || '').trim());
const updateBodyHtml = computed(() => renderMarkdown(updateBodyRaw.value));
const toggleUpdateBody = () => {
  updateBodyExpanded.value = !updateBodyExpanded.value;
};

watch(updateBodyRaw, (next, prev) => {
  if (next && next !== prev) {
    updateBodyExpanded.value = false;
  }
});
</script>

<template>
  <div class="absolute justify-center items-center flex w-full h-full pointer-events-none z-10">
    <div
      class="sc-notif-card pointer-events-auto min-w-[20rem] max-w-[38rem] w-[92vw] sm:w-[32rem] border rounded-xl shadow-xl overflow-hidden flex flex-col"
    >
      <div class="sc-notif-header flex items-center justify-between px-4 py-3 border-b">
        <div>
          <div class="text-sm font-semibold">更新提示</div>
          <div class="text-xs sc-notif-muted">仅平台管理员可见</div>
        </div>
        <button
          type="button"
          class="sc-notif-close"
          aria-label="关闭通知"
          @click="emit('close')"
        >
          ×
        </button>
      </div>
      <div v-if="!list.length" class="sc-notif-body p-4 text-sm sc-notif-muted">
        暂无通知
      </div>
      <div v-else class="sc-notif-body p-4">
        <div
          v-if="hasUpdateItem"
          class="sc-update-panel rounded-lg border p-4"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="text-sm font-semibold">系统更新提醒</div>
              <div class="text-xs sc-notif-muted mt-1">
                {{ updateTitle }}
              </div>
              <div v-if="releaseName" class="text-xs sc-notif-muted mt-1">
                {{ releaseName }}
              </div>
            </div>
            <span class="sc-update-badge">有新版本</span>
          </div>
          <div class="text-xs sc-notif-muted mt-2">
            发布时间：{{ updatePublishedAtText }}
          </div>
          <div v-if="releaseLink" class="text-xs mt-2">
            <a :href="releaseLink" target="_blank" rel="noreferrer" class="sc-notif-link hover:underline">查看发布页</a>
          </div>
          <button
            type="button"
            class="sc-notif-update-button mt-3"
            @click="emit('open-update-settings')"
          >
            前往平台管理下载更新
          </button>
          <div class="mt-3">
            <div v-if="updateLoading" class="text-xs sc-notif-muted">
              正在加载更新内容...
            </div>
            <div v-else-if="updateError" class="text-xs text-red-500">{{ updateError }}</div>
            <div v-else-if="updateBodyRaw" class="flex flex-col gap-2">
              <button
                type="button"
                class="text-xs sc-notif-link hover:underline self-start"
                @click="toggleUpdateBody"
              >
                {{ updateBodyExpanded ? '收起更新内容' : '展开更新内容' }}
              </button>
              <div
                class="text-sm sc-update-body"
                :class="{ 'is-collapsed': !updateBodyExpanded }"
                v-html="updateBodyHtml"
              ></div>
            </div>
            <div v-else class="text-xs sc-notif-muted">暂无更新内容</div>
          </div>
        </div>

        <div v-if="otherItems.length" class="mt-4">
          <div class="text-xs sc-notif-muted mb-2">其他通知</div>
          <div v-for="i in otherItems" :key="i.id" class="py-2 border-b sc-notif-divider last:border-b-0">
            <div class="text-xs sc-notif-muted">类型: {{ i.type }}</div>
            <div class="text-xs sc-notif-muted">时间: {{ i.createdAt }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sc-notif-close {
  width: 28px;
  height: 28px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  color: rgba(100, 116, 139, 0.9);
  transition: background-color 0.2s ease, color 0.2s ease;
}

.sc-notif-close:hover {
  background: rgba(15, 23, 42, 0.08);
  color: rgba(30, 41, 59, 0.95);
}

.sc-update-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(249, 115, 22, 0.12);
  color: #f97316;
  border: 1px solid rgba(249, 115, 22, 0.35);
}

.sc-update-body.is-collapsed {
  max-height: 8rem;
  overflow: hidden;
}

.sc-notif-card {
  max-height: calc(100vh - 2rem);
  background: #ffffff;
  color: #0f172a;
  border-color: rgba(148, 163, 184, 0.5);
}

.sc-notif-header,
.sc-notif-divider {
  border-color: rgba(226, 232, 240, 0.9);
}

.sc-notif-body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.5) transparent;
}

.sc-notif-body::-webkit-scrollbar {
  width: 4px;
}

.sc-notif-body::-webkit-scrollbar-track {
  background: transparent;
}

.sc-notif-body::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.5);
  border-radius: 999px;
}

.sc-notif-muted {
  color: rgba(100, 116, 139, 0.95);
}

.sc-notif-link {
  color: #2563eb;
}

.sc-notif-update-button {
  align-self: flex-start;
  padding: 5px 10px;
  border: 1px solid rgba(37, 99, 235, 0.35);
  border-radius: 6px;
  color: #2563eb;
  background: rgba(37, 99, 235, 0.08);
  font-size: 12px;
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.sc-notif-update-button:hover {
  border-color: rgba(37, 99, 235, 0.55);
  background: rgba(37, 99, 235, 0.14);
}

.sc-update-panel {
  background: rgba(248, 250, 252, 0.9);
  border-color: rgba(226, 232, 240, 0.9);
}

:global(:root[data-display-palette='night'] .sc-notif-card) {
  background: rgba(24, 24, 27, 0.95);
  color: rgba(248, 250, 252, 0.95);
  border-color: rgba(51, 65, 85, 0.8);
}

:global(:root[data-display-palette='night'] .sc-notif-header),
:global(:root[data-display-palette='night'] .sc-notif-divider) {
  border-color: rgba(51, 65, 85, 0.85);
}

:global(:root[data-display-palette='night'] .sc-notif-muted) {
  color: rgba(148, 163, 184, 0.9);
}

:global(:root[data-display-palette='night'] .sc-notif-link) {
  color: rgba(147, 197, 253, 0.95);
}

:global(:root[data-display-palette='night'] .sc-notif-body) {
  scrollbar-color: rgba(100, 116, 139, 0.6) transparent;
}

:global(:root[data-display-palette='night'] .sc-notif-body::-webkit-scrollbar-thumb) {
  background: rgba(100, 116, 139, 0.6);
}

:global(:root[data-display-palette='night'] .sc-notif-close) {
  color: rgba(226, 232, 240, 0.88);
}

:global(:root[data-display-palette='night'] .sc-notif-close:hover) {
  background: rgba(148, 163, 184, 0.2);
  color: rgba(248, 250, 252, 0.98);
}

:global(:root[data-display-palette='night'] .sc-update-panel) {
  background: rgba(15, 23, 42, 0.6);
  border-color: rgba(51, 65, 85, 0.85);
}

:global(:root[data-custom-theme='true'] .sc-notif-card) {
  background: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  border-color: var(--sc-border-strong, rgba(148, 163, 184, 0.5));
}

:global(:root[data-custom-theme='true'] .sc-notif-header),
:global(:root[data-custom-theme='true'] .sc-notif-divider) {
  border-color: var(--sc-border-mute, rgba(226, 232, 240, 0.9));
}

:global(:root[data-custom-theme='true'] .sc-notif-muted) {
  color: var(--sc-text-secondary, rgba(100, 116, 139, 0.95));
}

:global(:root[data-custom-theme='true'] .sc-notif-link) {
  color: var(--primary-color, #2563eb);
}

:global(:root[data-custom-theme='true'] .sc-notif-update-button) {
  border-color: var(--primary-color, rgba(37, 99, 235, 0.35));
  color: var(--primary-color, #2563eb);
  background: color-mix(in srgb, var(--primary-color, #2563eb) 10%, transparent);
}

:global(:root[data-custom-theme='true'] .sc-notif-body) {
  scrollbar-color: var(--sc-border-mute, rgba(148, 163, 184, 0.5)) transparent;
}

:global(:root[data-custom-theme='true'] .sc-notif-body::-webkit-scrollbar-thumb) {
  background: var(--sc-border-mute, rgba(148, 163, 184, 0.5));
}

:global(:root[data-custom-theme='true'] .sc-update-panel) {
  background: var(--sc-bg-surface, rgba(248, 250, 252, 0.9));
  border-color: var(--sc-border-mute, rgba(226, 232, 240, 0.9));
}

.sc-update-body :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 8px;
  margin-top: 0.25rem;
}

.sc-update-body :deep(h1),
.sc-update-body :deep(h2),
.sc-update-body :deep(h3) {
  font-weight: 600;
  margin: 0.6rem 0 0.4rem;
}

.sc-update-body :deep(ul) {
  list-style: disc;
  padding-left: 1.25rem;
  margin: 0.4rem 0;
}
</style>
