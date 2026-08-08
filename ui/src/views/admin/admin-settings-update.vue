<script setup lang="ts">
import { useUtilsStore } from '@/stores/utils';
import type { AdminUpdateOverview, AdminUpdateRelease, UpdateChannel } from '@/types';
import dayjs from 'dayjs';
import { useDialog, useMessage } from 'naive-ui';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

const utils = useUtilsStore();
const message = useMessage();
const dialog = useDialog();
const overview = ref<AdminUpdateOverview | null>(null);
const channel = ref<UpdateChannel>('stable');
const loading = ref(false);
const applying = ref(false);
const reconnecting = ref(false);
const errorText = ref('');
let pollTimer: ReturnType<typeof setTimeout> | null = null;
let terminalNotice = '';
let announceTerminal = false;
let reconnectStartedAt = 0;

const selectedRelease = computed<AdminUpdateRelease | undefined>(() => {
  if (!overview.value) return undefined;
  return channel.value === 'stable' ? overview.value.stable : overview.value.test;
});

const escapeHtml = (value: string) => value
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;');

const renderMarkdownInline = (value: string) => {
  const codeSpans: string[] = [];
  let rendered = escapeHtml(value).replace(/`([^`]+)`/g, (_match, code: string) => {
    const index = codeSpans.push(`<code>${code}</code>`) - 1;
    return `\u0000CODE${index}\u0000`;
  });
  rendered = rendered
    .replace(/!\[([^\]]*)\]\((https?:\/\/[^\s)]+)\)/g, '<img src="$2" alt="$1" loading="lazy" />')
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/__([^_]+)__/g, '<strong>$1</strong>')
    .replace(/~~([^~]+)~~/g, '<del>$1</del>')
    .replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>');
  return rendered.replace(/\u0000CODE(\d+)\u0000/g, (_match, index: string) => codeSpans[Number(index)] || '');
};

const renderMarkdown = (value: string) => {
  const lines = (value || '').replace(/\r\n?/g, '\n').split('\n');
  const output: string[] = [];
  let listType: 'ul' | 'ol' | '' = '';
  let paragraph: string[] = [];
  let codeFence = false;
  let codeLines: string[] = [];

  const closeList = () => {
    if (!listType) return;
    output.push(`</${listType}>`);
    listType = '';
  };
  const flushParagraph = () => {
    if (!paragraph.length) return;
    output.push(`<p>${renderMarkdownInline(paragraph.join(' '))}</p>`);
    paragraph = [];
  };
  const openList = (type: 'ul' | 'ol') => {
    flushParagraph();
    if (listType === type) return;
    closeList();
    listType = type;
    output.push(`<${type}>`);
  };

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    if (line.trimStart().startsWith('```')) {
      flushParagraph();
      closeList();
      if (codeFence) {
        output.push(`<pre><code>${escapeHtml(codeLines.join('\n'))}</code></pre>`);
        codeLines = [];
      }
      codeFence = !codeFence;
      continue;
    }
    if (codeFence) {
      codeLines.push(rawLine);
      continue;
    }
    if (!line.trim()) {
      flushParagraph();
      closeList();
      continue;
    }

    const heading = /^(#{1,4})\s+(.+)$/.exec(line);
    if (heading) {
      flushParagraph();
      closeList();
      const level = heading[1].length;
      output.push(`<h${level}>${renderMarkdownInline(heading[2])}</h${level}>`);
      continue;
    }
    const unordered = /^\s*[-*+]\s+(.+)$/.exec(line);
    if (unordered) {
      openList('ul');
      output.push(`<li>${renderMarkdownInline(unordered[1])}</li>`);
      continue;
    }
    const ordered = /^\s*\d+[.)]\s+(.+)$/.exec(line);
    if (ordered) {
      openList('ol');
      output.push(`<li>${renderMarkdownInline(ordered[1])}</li>`);
      continue;
    }
    const quote = /^\s*>\s?(.*)$/.exec(line);
    if (quote) {
      flushParagraph();
      closeList();
      output.push(`<blockquote>${renderMarkdownInline(quote[1])}</blockquote>`);
      continue;
    }
    if (/^\s*([-*_])(?:\s*\1){2,}\s*$/.test(line)) {
      flushParagraph();
      closeList();
      output.push('<hr />');
      continue;
    }
    closeList();
    paragraph.push(line.trim());
  }

  if (codeFence && codeLines.length) {
    output.push(`<pre><code>${escapeHtml(codeLines.join('\n'))}</code></pre>`);
  }
  flushParagraph();
  closeList();
  return output.join('');
};

const selectedReleaseBodyHtml = computed(() => renderMarkdown(selectedRelease.value?.body || ''));

const jobActive = computed(() => {
  const status = overview.value?.job?.status || '';
  return [
    'preparing',
    'downloading_updater',
    'downloading_package',
    'verifying',
    'preparing_restart',
    'restarting',
  ].includes(status);
});

const canApply = computed(() => {
  const release = selectedRelease.value;
  return !!overview.value?.supported && !!release?.asset && !release.isCurrent && !jobActive.value && !applying.value;
});

const formatTime = (value?: number) => value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '未知';
const formatSize = (value?: number) => {
  if (!value) return '未知';
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
};

const stopPolling = () => {
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
};

const schedulePoll = (delay = 1500) => {
  stopPolling();
  pollTimer = setTimeout(() => void fetchStatus(true), delay);
};

const handleTerminalJob = () => {
  const job = overview.value?.job;
  if (!job || (job.status !== 'succeeded' && job.status !== 'failed')) return;
  const noticeKey = `${job.status}:${job.finishedAt}:${job.targetVersion}`;
  if (terminalNotice === noticeKey) return;
  terminalNotice = noticeKey;
  if (job.status === 'succeeded') {
    message.success(`已更新至 ${job.targetVersion}`);
  } else {
    message.error(job.error || '更新失败');
  }
};

const fetchStatus = async (silent = false) => {
  if (!silent) loading.value = true;
  if (!silent) errorText.value = '';
  try {
    const resp = await utils.adminUpdateStatus();
    overview.value = resp.data;
    if (silent) errorText.value = '';
    reconnecting.value = false;
    reconnectStartedAt = 0;
    if (jobActive.value) announceTerminal = true;
    if (announceTerminal) handleTerminalJob();
    if (jobActive.value) schedulePoll(1500);
  } catch (error: any) {
    if (silent && jobActive.value) {
      reconnecting.value = true;
      if (!reconnectStartedAt) reconnectStartedAt = Date.now();
      if (Date.now() - reconnectStartedAt >= 90_000) {
        errorText.value = '服务未在 90 秒内恢复。请检查二进制目录中的 Sealupd 日志和旧版本备份。';
        schedulePoll(10_000);
      } else {
        schedulePoll(2200);
      }
    } else {
      errorText.value = error?.response?.data?.message || '获取更新状态失败';
    }
  } finally {
    if (!silent) loading.value = false;
  }
};

const checkUpdates = async (notify = true) => {
  loading.value = true;
  errorText.value = '';
  try {
    const resp = await utils.adminUpdateCheck();
    overview.value = resp.data;
    if (notify) message.success('版本信息已刷新');
  } catch (error: any) {
    errorText.value = error?.response?.data?.message || '检查更新失败';
  } finally {
    loading.value = false;
  }
};

const executeUpdate = async () => {
  const release = selectedRelease.value;
  if (!release?.asset) return;
  applying.value = true;
  errorText.value = '';
  try {
    const resp = await utils.adminUpdateApply({
      channel: channel.value,
      expectedReleaseId: release.releaseId,
      expectedAssetId: release.asset.id,
    });
    if (overview.value) overview.value.job = resp.data;
    announceTerminal = true;
    message.info('更新任务已启动，服务随后会自动重启');
    schedulePoll(1000);
  } catch (error: any) {
    const text = error?.response?.data?.message || '发起更新失败';
    errorText.value = text;
    if (error?.response?.status === 409) {
      message.warning('发布内容已变化，请重新检查后确认');
      await checkUpdates();
    }
  } finally {
    applying.value = false;
  }
};

const confirmUpdate = () => {
  const release = selectedRelease.value;
  if (!release?.asset) return;
  dialog.warning({
    title: channel.value === 'test' ? '确认更新到测试版本' : '确认更新',
    content: `将从 ${overview.value?.currentVersion || '未知版本'} 更新到 ${release.version}。下载约 ${formatSize(release.asset.size)}，服务会短暂中断并自动重启。`,
    positiveText: '下载并更新',
    negativeText: '取消',
    onPositiveClick: executeUpdate,
  });
};

onMounted(async () => {
  await fetchStatus();
  if (jobActive.value) {
    schedulePoll();
  } else if (overview.value?.supported && !overview.value?.stable && !overview.value?.test) {
    await checkUpdates(false);
  }
});

onBeforeUnmount(stopPolling);
</script>

<template>
  <div class="update-settings-scroll">
    <div class="update-header">
      <div>
        <h3>版本检测与更新</h3>
        <p>当前版本：<code>{{ overview?.currentVersion || '未写入构建版本' }}</code></p>
      </div>
      <n-button :loading="loading" :disabled="jobActive" @click="() => checkUpdates()">重新检查</n-button>
    </div>

    <n-alert v-if="errorText" type="error" class="mb-3">{{ errorText }}</n-alert>
    <n-alert v-if="overview && !overview.supported" type="warning" class="mb-3">
      {{ overview.unsupportedReason }}（{{ overview.platform }}）
    </n-alert>

    <div class="channel-grid">
      <button
        type="button"
        class="channel-card"
        :class="{ active: channel === 'stable' }"
        @click="channel = 'stable'"
      >
        <span class="channel-title">正式通道</span>
        <span>稳定版本，适合正常部署</span>
        <code>{{ overview?.stable?.version || '尚未获取' }}</code>
      </button>
      <button
        type="button"
        class="channel-card test"
        :class="{ active: channel === 'test' }"
        @click="channel = 'test'"
      >
        <span class="channel-title">测试通道</span>
        <span>滚动构建，追踪最新功能与修复</span>
        <code>{{ overview?.test?.version || '尚未获取' }}</code>
      </button>
    </div>

    <n-alert v-if="channel === 'test'" type="warning" class="mt-3">
      测试版本使用滚动发布页；版本和发布时间取自当前平台资产，不使用固定 tag 的发布时间。
    </n-alert>

    <n-spin :show="loading">
      <section v-if="selectedRelease" class="release-panel">
        <div class="release-heading">
          <div>
            <div class="release-version">
              <span>{{ selectedRelease.version }}</span>
              <n-tag v-if="selectedRelease.isCurrent" type="success" size="small">当前版本</n-tag>
              <n-tag v-else :type="channel === 'test' ? 'warning' : 'info'" size="small">可安装</n-tag>
            </div>
            <p>{{ selectedRelease.name || selectedRelease.tag }}</p>
          </div>
          <div class="release-heading-actions">
            <a :href="selectedRelease.htmlUrl" target="_blank" rel="noreferrer">打开发布页</a>
            <n-button type="primary" :disabled="!canApply" :loading="applying" @click="confirmUpdate">
              {{ selectedRelease.isCurrent ? '已是当前版本' : '下载并更新' }}
            </n-button>
          </div>
        </div>

        <n-collapse class="release-notes">
          <n-collapse-item title="查看更新内容" name="notes">
            <div v-if="selectedRelease.body" class="release-body release-markdown" v-html="selectedReleaseBodyHtml"></div>
            <n-empty v-else size="small" description="此版本未提供更新说明" />
          </n-collapse-item>
        </n-collapse>

        <dl class="release-meta">
          <div><dt>发布时间</dt><dd>{{ formatTime(selectedRelease.publishedAt) }}</dd></div>
          <div><dt>目标平台</dt><dd>{{ selectedRelease.platformLabel }}</dd></div>
          <div><dt>更新包</dt><dd>{{ selectedRelease.asset?.name || '无匹配资产' }}</dd></div>
          <div><dt>下载大小</dt><dd>{{ formatSize(selectedRelease.asset?.size) }}</dd></div>
        </dl>

        <div class="update-actions">
          <span v-if="overview?.lastCheckedAt" class="check-time">检查于 {{ formatTime(overview.lastCheckedAt) }}</span>
        </div>
      </section>
    </n-spin>

    <section v-if="overview?.job" class="job-panel">
      <div class="job-title">
        <span>{{ overview.job.message || '更新任务' }}</span>
        <n-tag :type="overview.job.status === 'failed' ? 'error' : overview.job.status === 'succeeded' ? 'success' : 'info'" size="small">
          {{ overview.job.status }}
        </n-tag>
      </div>
      <n-progress type="line" :percentage="overview.job.progress || 0" :status="overview.job.status === 'failed' ? 'error' : overview.job.status === 'succeeded' ? 'success' : 'default'" />
      <p v-if="reconnecting" class="reconnecting">服务正在重启，等待重新连接……</p>
      <p v-if="overview.job.error" class="job-error">{{ overview.job.error }}</p>
      <p class="job-detail">{{ overview.job.previousVersion }} → {{ overview.job.targetVersion }}</p>
    </section>
  </div>
</template>

<style scoped>
.update-settings-scroll {
  box-sizing: border-box;
  height: 61vh;
  max-height: 61vh;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: scroll;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  padding: 4px 8px 24px 2px;
}

.update-settings-scroll::-webkit-scrollbar {
  width: 8px;
}

.update-settings-scroll::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.45);
  border-radius: 999px;
}

.update-header,
.release-heading,
.update-actions,
.job-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.release-heading-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.update-header h3 {
  margin: 0 0 4px;
  font-size: 18px;
}

.update-header p,
.release-heading p,
.job-panel p {
  margin: 0;
  color: var(--n-text-color-3, #71717a);
}

.channel-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.channel-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px;
  text-align: left;
  color: inherit;
  background: color-mix(in srgb, var(--n-color, transparent) 92%, #3b82f6 8%);
  border: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.28));
  border-radius: 10px;
  cursor: pointer;
}

.channel-card:hover,
.channel-card.active {
  border-color: #3b82f6;
  box-shadow: 0 0 0 1px color-mix(in srgb, #3b82f6 45%, transparent);
}

.channel-card.test.active {
  border-color: #f59e0b;
  box-shadow: 0 0 0 1px color-mix(in srgb, #f59e0b 45%, transparent);
}

.channel-title {
  font-size: 16px;
  font-weight: 600;
}

.release-panel,
.job-panel {
  margin-top: 16px;
  padding: 18px;
  border: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.28));
  border-radius: 10px;
}

.release-version {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 20px;
  font-weight: 650;
}

.release-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 24px;
  margin: 18px 0;
}

.release-notes {
  margin-top: 16px;
  padding: 0 2px;
}

.release-meta div {
  min-width: 0;
}

.release-meta dt {
  color: var(--n-text-color-3, #71717a);
  font-size: 12px;
}

.release-meta dd {
  margin: 3px 0 0;
  overflow-wrap: anywhere;
}

.release-body {
  max-height: 240px;
  overflow: auto;
  white-space: pre-wrap;
  line-height: 1.6;
}

.release-markdown :deep(h1),
.release-markdown :deep(h2),
.release-markdown :deep(h3),
.release-markdown :deep(h4) {
  margin: 1rem 0 0.45rem;
  line-height: 1.3;
  font-weight: 650;
}

.release-markdown :deep(h1) { font-size: 1.35rem; }
.release-markdown :deep(h2) { font-size: 1.2rem; }
.release-markdown :deep(h3) { font-size: 1.08rem; }
.release-markdown :deep(h4) { font-size: 1rem; }

.release-markdown :deep(p) {
  margin: 0.55rem 0;
}

.release-markdown :deep(ul),
.release-markdown :deep(ol) {
  margin: 0.55rem 0;
  padding-left: 1.4rem;
}

.release-markdown :deep(li) {
  margin: 0.25rem 0;
}

.release-markdown :deep(code) {
  padding: 0.12rem 0.35rem;
  border-radius: 4px;
  background: rgba(128, 128, 128, 0.18);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.release-markdown :deep(pre) {
  overflow-x: auto;
  padding: 12px;
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.28);
}

.release-markdown :deep(pre code) {
  padding: 0;
  background: transparent;
}

.release-markdown :deep(blockquote) {
  margin: 0.7rem 0;
  padding: 0.25rem 0.8rem;
  border-left: 3px solid #3b82f6;
  color: var(--n-text-color-3, #71717a);
}

.release-markdown :deep(a) {
  color: #3b82f6;
  text-decoration: none;
}

.release-markdown :deep(a:hover) {
  text-decoration: underline;
}

.release-markdown :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0.7rem 0;
  border-radius: 8px;
}

.release-markdown :deep(hr) {
  margin: 1rem 0;
  border: 0;
  border-top: 1px solid rgba(128, 128, 128, 0.3);
}

.update-actions {
  margin-top: 16px;
}

.check-time,
.job-detail,
.reconnecting {
  color: var(--n-text-color-3, #71717a);
  font-size: 12px;
}

.job-panel {
  background: color-mix(in srgb, var(--n-color, transparent) 94%, #3b82f6 6%);
}

.job-title {
  margin-bottom: 12px;
  font-weight: 600;
}

.job-error {
  margin-top: 10px !important;
  color: #ef4444 !important;
}

@media (max-width: 720px) {
  .channel-grid,
  .release-meta {
    grid-template-columns: 1fr;
  }

  .update-header,
  .release-heading,
  .update-actions {
    align-items: flex-start;
    flex-direction: column;
  }

  .release-heading-actions {
    width: 100%;
    justify-content: space-between;
  }

  .update-settings-scroll {
    height: 68vh;
    max-height: 68vh;
  }
}
</style>
