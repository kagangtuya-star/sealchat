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
const emit = defineEmits(['close']);

const list = computed(() => props.items || []);
const updateItem = computed(() => list.value.find((item) => item?.type === 'system.update'));
const otherItems = computed(() => list.value.filter((item) => item?.type !== 'system.update'));
const hasUpdateItem = computed(() => !!updateItem.value);

const utils = useUtilsStore();
const updateStatus = ref<UpdateStatus | null>(null);
const updateLoading = ref(false);
const updateError = ref('');

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
</script>

<template>
  <div class="absolute justify-center items-center flex w-full h-full pointer-events-none z-10">
    <div class="pointer-events-auto min-w-[20rem] max-w-[38rem] w-[92vw] sm:w-[32rem] bg-white dark:bg-zinc-900 border border-slate-200 dark:border-zinc-700 rounded-xl shadow-xl overflow-hidden">
      <div class="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-zinc-800">
        <div>
          <div class="text-sm font-semibold text-slate-900 dark:text-slate-100">更新提示</div>
          <div class="text-xs text-slate-500 dark:text-slate-400">仅平台管理员可见</div>
        </div>
        <button
          type="button"
          class="sc-notif-close text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-100"
          aria-label="关闭通知"
          @click="emit('close')"
        >
          ×
        </button>
      </div>
      <div v-if="!list.length" class="p-4 text-sm text-slate-500 dark:text-slate-400">
        暂无通知
      </div>
      <div v-else class="p-4">
        <div
          v-if="hasUpdateItem"
          class="rounded-lg border border-slate-200 dark:border-zinc-700 bg-slate-50 dark:bg-zinc-950/40 p-4"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="text-sm font-semibold text-slate-900 dark:text-slate-100">系统更新提醒</div>
              <div class="text-xs text-slate-500 dark:text-slate-400 mt-1">
                {{ updateTitle }}
              </div>
              <div v-if="releaseName" class="text-xs text-slate-500 dark:text-slate-400 mt-1">
                {{ releaseName }}
              </div>
            </div>
            <span class="sc-update-badge">有新版本</span>
          </div>
          <div class="text-xs text-slate-500 dark:text-slate-400 mt-2">
            发布时间：{{ updatePublishedAtText }}
          </div>
          <div v-if="releaseLink" class="text-xs mt-2">
            <a :href="releaseLink" target="_blank" rel="noreferrer" class="text-blue-600 dark:text-blue-400 hover:underline">查看发布页</a>
          </div>
          <div class="mt-3">
            <div v-if="updateLoading" class="text-xs text-slate-500 dark:text-slate-400">
              正在加载更新内容...
            </div>
            <div v-else-if="updateError" class="text-xs text-red-500">{{ updateError }}</div>
            <div v-else-if="updateBodyRaw" class="text-sm sc-update-body" v-html="updateBodyHtml"></div>
            <div v-else class="text-xs text-slate-500 dark:text-slate-400">暂无更新内容</div>
          </div>
        </div>

        <div v-if="otherItems.length" class="mt-4">
          <div class="text-xs text-slate-500 dark:text-slate-400 mb-2">其他通知</div>
          <div v-for="i in otherItems" :key="i.id" class="py-2 border-b border-slate-100 dark:border-zinc-800 last:border-b-0">
            <div class="text-xs text-slate-500 dark:text-slate-400">类型: {{ i.type }}</div>
            <div class="text-xs text-slate-500 dark:text-slate-400">时间: {{ i.createdAt }}</div>
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
  transition: background-color 0.2s ease, color 0.2s ease;
}

.sc-notif-close:hover {
  background: rgba(15, 23, 42, 0.08);
}

:global(.dark) .sc-notif-close:hover {
  background: rgba(148, 163, 184, 0.12);
}

.sc-update-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(249, 115, 22, 0.12);
  color: #f97316;
  border: 1px solid rgba(249, 115, 22, 0.35);
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
