<script setup lang="ts">
// 发布管理：发布前确保草稿已保存并检查孤立元素/模板问题；发布期间暂停自动保存，
// 用幂等 clientOpId 提交并轮询任务；完成后以“版本 +1”采纳新工程版本。构建失败不影响旧发布版。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import {
  describeLogError,
  fetchLogReleaseJob,
  isLogError,
  LogDomainError,
  queueLogRelease,
  withdrawLogPublication,
} from '@/services/log/logApi';
import { newClientOpId } from '@/services/log/logMessageCommands';
import type { AnchorState } from '@/services/log/logRenderer';
import type { FrameLayoutResult } from '@/services/log/pagination/logFrameHost';
import type { LogProjectContext, LogReleaseJob, LogSection } from '@/types/log';
import type { LogDraftController } from '../composables/useLogDraft';
import { chapterViews } from '../logChapterOps';

const props = defineProps<{
  context: LogProjectContext;
  slug: string;
  draftCtl: LogDraftController;
  sections: LogSection[];
  loadedCount: number;
  complete: boolean;
  currentIsFragment: boolean;
  anchorStates: Map<string, AnchorState> | null;
  layout: FrameLayoutResult | null;
  templateIssues: string[];
  sessionExclusions: Array<{ sectionId: string; ids: string[]; label: string }>;
}>();
const chapterCount = computed(() => chapterViews(props.draftCtl.draft.value).length);
const emit = defineEmits<{
  (e: 'undo-exclusion', index: number): void;
  (e: 'locate-element', id: string): void;
  (e: 'reload'): void;
}>();

const toast = useMessage();
const dialog = useDialog();
const caps = computed(() => props.context.capabilities);
const STORE_KEY = computed(() => `sealchat.log.release.${props.context.projectId}`);
const publishedReleaseId = ref<string | null>(props.context.publishedReleaseId);

const orphans = computed(() => Array.from(props.anchorStates?.entries() || []).filter(([, s]) => s === 'orphaned').map(([id]) => id));
const serverBadElements = ref<string[]>([]);
const viewUrl = computed(() => {
  const slug = props.context.management?.viewSlug;
  if (!slug) return '';
  return `${location.origin}${location.pathname}#/log/views/${slug}`;
});

// ---- 发布任务 ----
interface PendingRelease { clientOpId: string; expected: number; jobId?: string }
const readPending = (): PendingRelease | null => {
  try {
    const raw = sessionStorage.getItem(STORE_KEY.value);
    const v = raw ? JSON.parse(raw) : null;
    return v && typeof v.clientOpId === 'string' && typeof v.expected === 'number' ? v : null;
  } catch {
    return null;
  }
};
const writePending = (v: PendingRelease | null) => {
  try {
    if (v) sessionStorage.setItem(STORE_KEY.value, JSON.stringify(v));
    else sessionStorage.removeItem(STORE_KEY.value);
  } catch {
    // 存储不可用时仍可在本页内完成
  }
};

const job = ref<LogReleaseJob | null>(null);
const publishing = ref(false);
const publishError = ref('');
let disposed = false;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

const pollUntilFinished = async (jobId: string): Promise<LogReleaseJob> => {
  let failures = 0;
  for (;;) {
    if (disposed) throw new LogDomainError('ABORTED', '页面已离开');
    try {
      const j = await fetchLogReleaseJob(props.context.projectId, jobId);
      job.value = j;
      failures = 0;
      if (j.status === 'done' || j.status === 'failed') return j;
    } catch (error) {
      failures += 1;
      if (isLogError(error, 'NOT_FOUND', 'PERMISSION_DENIED') || failures > 10) throw error;
    }
    await sleep(1200);
  }
};

const finishJob = (j: LogReleaseJob) => {
  writePending(null);
  if (j.status === 'done') {
    publishedReleaseId.value = j.releaseId || publishedReleaseId.value;
    serverBadElements.value = [];
    toast.success('发布完成');
    return;
  }
  const code = j.error?.code || 'BUILD_FAILED';
  throw new LogDomainError(code, j.error?.message || '构建失败，旧发布版不受影响');
};

const publish = async () => {
  if (orphans.value.length) {
    publishError.value = `${props.currentIsFragment ? '当前片段' : '本章'}有锚点不存在的元素，请先重新定位或删除`;
    return;
  }
  publishing.value = true;
  publishError.value = '';
  serverBadElements.value = [];
  job.value = null;
  props.draftCtl.pause();
  try {
    await props.draftCtl.runVersioned(async (expected) => {
      const prev = readPending();
      const pending: PendingRelease = prev && prev.expected === expected && !prev.jobId ? prev : { clientOpId: newClientOpId('release'), expected };
      writePending(pending);
      const queued = await queueLogRelease(props.context.projectId, expected, pending.clientOpId);
      writePending({ ...pending, jobId: queued.jobId });
      const j = await pollUntilFinished(queued.jobId);
      finishJob(j);
      return { value: j, draftVersion: expected + 1 };
    });
  } catch (error) {
    if (isLogError(error, 'INVALID_TEMPLATE')) {
      const ids = (error as LogDomainError).details?.elementIds;
      serverBadElements.value = Array.isArray(ids) ? ids.filter((x): x is string => typeof x === 'string') : [];
    }
    if (!isLogError(error, 'ABORTED')) publishError.value = describeLogError(error, '发布失败');
    if (isLogError(error, 'VERSION_CONFLICT') || /版本/.test(publishError.value)) {
      publishError.value += '（发布期间工程被修改，请重新加载后再发布）';
    }
  } finally {
    publishing.value = false;
    props.draftCtl.resume();
  }
};

/** 刷新后恢复仍在进行的发布任务：暂停自动保存直到结束，完成后按 expected+1 采纳。 */
const resumePending = async () => {
  const pending = readPending();
  if (!pending?.jobId) return;
  publishing.value = true;
  props.draftCtl.pause();
  try {
    const j = await pollUntilFinished(pending.jobId);
    finishJob(j);
    props.draftCtl.adoptVersion(pending.expected, pending.expected + 1);
  } catch (error) {
    writePending(null);
    if (!isLogError(error, 'ABORTED')) publishError.value = describeLogError(error, '发布失败');
  } finally {
    publishing.value = false;
    props.draftCtl.resume();
  }
};

const confirmPublish = () => {
  dialog.info({
    title: '发布',
    content: `将以当前已保存的草稿（v${props.draftCtl.draftVersion.value}）生成固定发布版。发布期间请不要编辑；之后对原消息或草稿的修改不会进入本次发布版。`,
    positiveText: '发布',
    negativeText: '取消',
    onPositiveClick: () => {
      void publish();
    },
  });
};

// ---- 下架 ----
const withdrawing = ref(false);
const withdraw = () => {
  dialog.warning({
    title: '下架发布',
    content: '下架后公开链接立即不可访问；当前发布快照会删除，草稿保留，可随时重新发布。',
    positiveText: '下架',
    negativeText: '取消',
    onPositiveClick: async () => {
      withdrawing.value = true;
      try {
        await props.draftCtl.runVersioned(async (expected) => {
          await withdrawLogPublication(props.context.projectId, expected);
          return { value: null, draftVersion: expected + 1 };
        });
        publishedReleaseId.value = null;
        toast.success('已下架');
      } catch (error) {
        toast.error(describeLogError(error, '下架失败'));
      } finally {
        withdrawing.value = false;
      }
    },
  });
};

const copyLink = async () => {
  if (!viewUrl.value) return;
  try {
    await navigator.clipboard.writeText(viewUrl.value);
    toast.success('已复制展示链接');
  } catch {
    toast.info(viewUrl.value);
  }
};

const progressPct = computed(() => {
  const p = job.value?.progress;
  return p && p.total ? Math.min(100, Math.round((p.done / p.total) * 100)) : 0;
});

onMounted(() => {
  void resumePending();
});
onBeforeUnmount(() => {
  disposed = true;
});
</script>

<template>
  <div class="log-pub">
    <section>
      <div class="log-pub__title">发布状态</div>
      <div v-if="publishedReleaseId" class="log-pub__line">
        <n-tag size="small" type="success">已发布</n-tag>
        <template v-if="viewUrl">
          <a :href="viewUrl" target="_blank" rel="noopener noreferrer">查看展示</a>
          <n-button size="tiny" @click="copyLink">复制链接</n-button>
        </template>
        <span v-else class="log-pub__hint">展示链接由世界管理者提供</span>
      </div>
      <div v-else class="log-pub__line"><n-tag size="small">未发布</n-tag><span class="log-pub__hint">发布前展示链接会显示“尚未发布”。</span></div>
      <p class="log-pub__hint">{{ currentIsFragment ? '工具栏的“片段发布效果”只预览当前来源片段；公开章节会按编排串联全部片段。' : '工具栏的“发布效果”开关可预览公开读者看到的版式（不显示设计控件）。' }}</p>
    </section>

    <section>
      <div class="log-pub__title">发布前检查<template v-if="currentIsFragment">（当前片段）</template></div>
      <ul class="log-pub__checks">
        <li>章节 {{ chapterCount }} 个<template v-if="chapterCount !== sections.length">（来源片段 {{ sections.length }} 个）</template>；当前{{ currentIsFragment ? '片段' : '章' }}已加载 {{ loadedCount }} 条{{ complete ? '（完整）' : '（未完整加载，发布以服务器全量为准）' }}</li>
        <li :class="{ 'is-bad': draftCtl.status.value !== 'saved' }">草稿：{{ { saved: '已保存', dirty: '有未保存修改（发布前会先保存）', saving: '保存中', error: '保存失败', conflict: '冲突，需重新加载' }[draftCtl.status.value] }}</li>
        <li :class="{ 'is-bad': orphans.length }">
          孤立元素：{{ orphans.length ? `${orphans.length} 个（必须处理）` : `无（当前${currentIsFragment ? '片段' : '章'}）` }}
          <a v-for="id in orphans" :key="id" href="#" class="log-pub__link" @click.prevent="emit('locate-element', id)">处理</a>
        </li>
        <li :class="{ 'is-warn': templateIssues.length }">模板问题：{{ templateIssues.length || '无' }}</li>
        <li :class="{ 'is-warn': layout?.missingAssets }">资源：{{ layout?.missingAssets ? `${layout.missingAssets} 个缺失` : '正常' }}</li>
        <li v-if="layout">排版警告：{{ layout.warnings.length || '无' }}</li>
        <li v-for="w in layout?.warnings.filter(w => w.code.startsWith('ANCHOR_GROUP_'))" :key="w.nodeId || w.code" class="is-warn">
          消息 {{ w.nodeId }} 与锚定元素{{ w.code === 'ANCHOR_GROUP_BREAK' ? '遇到模板分页规则' : '合计超过一页' }}，已按原顺序分别排版。
        </li>
      </ul>
      <p v-if="currentIsFragment" class="log-pub__hint">这里的元素、模板、资源与排版检查来自当前片段预览；正式发布时服务端仍会校验全部章节和来源片段。</p>
      <div v-if="sessionExclusions.length" class="log-pub__excl">
        <div class="log-pub__subtitle">本次会话排除的消息</div>
        <div v-for="(x, i) in sessionExclusions" :key="i" class="log-pub__line">
          <span>{{ x.label }}</span>
          <n-button size="tiny" :disabled="!caps.manageSources" @click="emit('undo-exclusion', i)">撤销排除</n-button>
        </div>
      </div>
    </section>

    <section v-if="caps.publish">
      <n-button type="primary" :loading="publishing" :disabled="publishing || draftCtl.status.value === 'conflict'" @click="confirmPublish">发布当前草稿</n-button>
      <div v-if="job" class="log-pub__job" role="status" aria-live="polite">
        {{ { queued: '排队中', running: '构建中', done: '已完成', failed: '失败' }[job.status] }}
        <n-progress type="line" :percentage="progressPct" :status="job.status === 'failed' ? 'error' : job.status === 'done' ? 'success' : 'default'" />
        <span class="log-pub__hint">{{ job.progress.done }}/{{ job.progress.total }}</span>
      </div>
      <n-alert v-if="publishError" type="error" :show-icon="false" class="log-pub__err">
        {{ publishError }}
        <div v-if="serverBadElements.length">
          需要处理的元素：
          <a v-for="id in serverBadElements" :key="id" href="#" class="log-pub__link" @click.prevent="emit('locate-element', id)">{{ id.slice(0, 8) }}</a>
        </div>
        <n-button v-if="draftCtl.status.value === 'conflict'" size="tiny" style="margin-top: 6px" @click="emit('reload')">重新加载</n-button>
      </n-alert>
    </section>
    <n-alert v-else type="info" :show-icon="false">只有世界管理者可以发布和下架。</n-alert>

    <section v-if="caps.publish">
      <div class="log-pub__title">发布管理</div>
      <n-button v-if="publishedReleaseId" size="small" type="error" secondary :loading="withdrawing" @click="withdraw">下架当前发布</n-button>
      <p class="log-pub__hint">更换展示链接、启用/停用排版器请在世界详情的“LOG 排版与发布”中操作。</p>
    </section>
  </div>
</template>

<style scoped>
.log-pub {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 8px 0 16px;
}

.log-pub > section + section {
  padding-top: 14px;
  border-top: 1px solid var(--sc-border-mute, #e5e5e5);
}

.log-pub__title {
  font-weight: 600;
  font-size: 12px;
  letter-spacing: 0.02em;
  color: var(--sc-text-secondary, #666);
  margin-bottom: 8px;
}

.log-pub__subtitle {
  font-weight: 600;
  font-size: 12px;
  margin: 6px 0 4px;
}

.log-pub__line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  padding: 2px 0;
}

.log-pub__hint {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  margin: 4px 0 0;
}

.log-pub__checks {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.8;
}

.log-pub__checks .is-bad {
  color: #d03050;
}

.log-pub__checks .is-warn {
  color: #b7791f;
}

.log-pub__link {
  margin-left: 6px;
}

.log-pub__job {
  margin-top: 8px;
  font-size: 12px;
}

.log-pub__err {
  margin-top: 8px;
}
</style>
