<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { createLogPdfExport, describeLogError, fetchLogPdfCapability, isLogError, toLogError, type LogPdfCapability } from '@/services/log/logApi';
import { useChatStore } from '@/stores/chat';
import type { LogDraftController } from '../composables/useLogDraft';

// 后台 PDF 与浏览器打印并存：后台任务沿用现有导出任务（排队 → 生成 → 下载 / 失败重试），
// 浏览器打印交给工作台的现有打印流程，后台渲染器缺失时不影响它。
const props = defineProps<{ show: boolean; projectId: string; draftCtl: LogDraftController; title: string }>();
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
  (e: 'browser-print'): void;
}>();
const chat = useChatStore();
const toast = useMessage();

type TaskState = 'idle' | 'creating' | 'pending' | 'processing' | 'done' | 'failed';

const capability = ref<LogPdfCapability | null>(null);
const capabilityError = ref('');
const capabilityLoading = ref(false);
const mode = ref<'server' | 'browser'>('server');
const displayName = ref('');
const taskId = ref('');
const taskState = ref<TaskState>('idle');
const taskMessage = ref('');
const anchorBreaks = ref(0);
const anchorOversizes = ref(0);
const fileName = ref('');
const downloading = ref(false);
const errorText = ref('');

const storageKey = computed(() => `sealchat.log.pdfTask.${props.projectId}`);
const rememberTask = (id: string) => {
  try {
    if (id) localStorage.setItem(storageKey.value, id);
    else localStorage.removeItem(storageKey.value);
  } catch {
    // 浏览器存储不可用时只是不恢复上次任务。
  }
};
const recallTask = () => {
  try {
    return localStorage.getItem(storageKey.value) || '';
  } catch {
    return '';
  }
};

const serverAvailable = computed(() => !!capability.value?.available);
const isZip = computed(() => /\.zip$/i.test(fileName.value));
const running = computed(() => taskState.value === 'creating' || taskState.value === 'pending' || taskState.value === 'processing');
const stateText = computed(() => ({
  idle: '',
  creating: '正在创建任务…',
  pending: '排队中：任务已提交到服务器',
  processing: '服务器正在生成 PDF…',
  done: isZip.value ? '已完成（分段 PDF 压缩包）' : '已完成',
  failed: '生成失败',
}[taskState.value]));

let generation = 0;
let timer: ReturnType<typeof setTimeout> | null = null;
const stopPolling = () => {
  generation += 1;
  if (timer) clearTimeout(timer);
  timer = null;
};
onBeforeUnmount(stopPolling);

const loadCapability = async () => {
  capabilityLoading.value = true;
  capabilityError.value = '';
  try {
    capability.value = await fetchLogPdfCapability();
    if (!capability.value.available) mode.value = 'browser';
  } catch (error) {
    capability.value = null;
    capabilityError.value = describeLogError(error, '无法获取服务器 PDF 能力');
    mode.value = 'browser';
  } finally {
    capabilityLoading.value = false;
  }
};

const poll = (id: string) => {
  stopPolling();
  const run = generation;
  let failures = 0;
  const tick = async () => {
    if (run !== generation || !props.show) return;
    try {
      const state = await chat.getExportTaskStatus(id);
      if (run !== generation) return;
      failures = 0;
      errorText.value = '';
      taskMessage.value = state.message || '';
      fileName.value = state.file_name || '';
      if (state.status === 'done') {
        anchorBreaks.value = state.log_pdf_warnings?.ANCHOR_GROUP_BREAK ?? 0;
        anchorOversizes.value = state.log_pdf_warnings?.ANCHOR_GROUP_OVERSIZE ?? 0;
        taskState.value = 'done';
        return;
      }
      if (state.status === 'failed') {
        taskState.value = 'failed';
        return;
      }
      taskState.value = state.status === 'processing' ? 'processing' : 'pending';
      timer = setTimeout(() => { timer = null; void tick(); }, 2000);
    } catch (error) {
      if (run !== generation) return;
      const domain = toLogError(error);
      errorText.value = describeLogError(domain, '无法读取任务状态');
      if (isLogError(domain, 'NOT_FOUND', 'PERMISSION_DENIED')) {
        taskState.value = 'idle';
        taskId.value = '';
        rememberTask('');
        return;
      }
      errorText.value += '，将自动重试查询';
      failures = Math.min(failures + 1, 4);
      timer = setTimeout(() => { timer = null; void tick(); }, Math.min(2000 * 2 ** failures, 30000));
    }
  };
  void tick();
};

watch(() => [props.show, props.projectId] as const, ([show]) => {
  stopPolling();
  if (!show) return;
  errorText.value = '';
  displayName.value = '';
  void loadCapability();
  const last = recallTask();
  if (last) {
    taskId.value = last;
    taskState.value = 'pending';
    poll(last);
  } else {
    taskId.value = '';
    taskState.value = 'idle';
  }
}, { immediate: true });

const start = async () => {
  if (running.value || !serverAvailable.value) return;
  errorText.value = '';
  taskMessage.value = '';
  fileName.value = '';
  taskState.value = 'creating';
  const projectId = props.projectId;
  const name = displayName.value;
  try {
    const task = await props.draftCtl.runVersioned(async (expectedDraftVersion) => {
      const value = await createLogPdfExport(projectId, { expectedDraftVersion, format: 'pdf', displayName: name });
      return { value, draftVersion: null };
    });
    if (projectId !== props.projectId) return;
    taskId.value = task.task_id;
    rememberTask(task.task_id);
    taskState.value = 'pending';
    poll(task.task_id);
  } catch (error) {
    taskState.value = 'idle';
    errorText.value = describeLogError(error, '创建 PDF 任务失败');
  }
};

const retry = async () => {
  if (!taskId.value || running.value) return;
  errorText.value = '';
  taskState.value = 'creating';
  try {
    const task = await chat.retryExportTask(taskId.value);
    taskId.value = task.task_id;
    rememberTask(task.task_id);
    taskState.value = 'pending';
    taskMessage.value = '';
    poll(task.task_id);
  } catch (error) {
    taskState.value = 'failed';
    errorText.value = describeLogError(error, '重试失败');
  }
};

const download = async () => {
  if (!taskId.value || downloading.value) return;
  downloading.value = true;
  errorText.value = '';
  try {
    // PDF 可能较大，下载超时放宽到 10 分钟。
    const result = await chat.downloadExportResult(taskId.value, fileName.value || undefined, { timeout: 600000 });
    const url = URL.createObjectURL(result.blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = result.fileName;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    toast.success('PDF 已下载');
  } catch (error) {
    errorText.value = describeLogError(error, '下载失败');
  } finally {
    downloading.value = false;
  }
};

const reset = () => {
  stopPolling();
  rememberTask('');
  taskId.value = '';
  taskState.value = 'idle';
  taskMessage.value = '';
  fileName.value = '';
  errorText.value = '';
};

const browserPrint = () => {
  emit('browser-print');
  emit('update:show', false);
};
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="导出 PDF"
    style="width: min(560px, 94vw)"
    :content-style="{ maxHeight: '75vh', overflowY: 'auto' }"
    @update:show="(value: boolean) => emit('update:show', value)"
  >
    <div class="log-pdf">
      <n-spin :show="capabilityLoading" size="small">
        <n-radio-group v-model:value="mode" class="log-pdf__modes" name="log-pdf-mode">
          <label class="log-pdf__mode" :class="{ 'is-active': mode === 'server', 'is-disabled': !serverAvailable }">
            <n-radio value="server" :disabled="!serverAvailable" />
            <span class="log-pdf__mode-body">
              <span class="log-pdf__mode-title">后台生成 PDF（推荐）</span>
              <span class="log-pdf__mode-desc">适合长文档；任务在服务器运行，关闭页面也不影响生成。</span>
            </span>
          </label>
          <label class="log-pdf__mode" :class="{ 'is-active': mode === 'browser' }">
            <n-radio value="browser" />
            <span class="log-pdf__mode-body">
              <span class="log-pdf__mode-title">浏览器打印</span>
              <span class="log-pdf__mode-desc">适合短文档；继续使用现有打印能力，打印当前来源片段。</span>
            </span>
          </label>
        </n-radio-group>
      </n-spin>

      <n-alert v-if="capability && !capability.available && !capabilityLoading" type="warning" :show-icon="false" class="log-pdf__alert">
        <div>服务器未检测到 PDF 渲染器。</div>
        <div>Linux Server 推荐安装 chrome-headless-shell。</div>
        <div>仍可使用浏览器打印。</div>
        <div v-if="capability.reason" class="log-pdf__reason">{{ capability.probing ? '服务器正在检测 PDF 渲染器，请稍后再试。' : capability.reason }}</div>
      </n-alert>
      <n-alert v-else-if="capabilityError" type="warning" :show-icon="false" class="log-pdf__alert">
        {{ capabilityError }}。仍可使用浏览器打印。
      </n-alert>

      <template v-if="mode === 'server' && serverAvailable">
        <n-alert v-if="capability && !capability.mergerAvailable" type="info" :show-icon="false" class="log-pdf__alert">
          服务器缺少 PDF 合并工具（pdfunite / qpdf）。短 LOG 仍生成单个 PDF；超长 LOG 会按顺序输出分段 PDF 的 ZIP 压缩包。
        </n-alert>
        <n-form label-placement="top" :disabled="running">
          <n-form-item label="导出文件名称（可选）">
            <n-input v-model:value="displayName" :placeholder="title ? `默认使用 LOG 标题：${title}` : '默认使用 LOG 标题'" :maxlength="120" />
          </n-form-item>
        </n-form>
        <p class="log-pdf__hint">使用已保存的排版、样式与页面设置生成，与预览使用同一套排版器；页码在全文连续。</p>

        <div v-if="taskState !== 'idle'" class="log-pdf__task" role="status" :aria-busy="running">
          <n-spin v-if="running" size="small" />
          <span class="log-pdf__task-text" :class="`is-${taskState}`">{{ stateText }}</span>
        </div>
        <n-alert v-if="taskState === 'failed'" type="error" :show-icon="false" class="log-pdf__alert">
          {{ taskMessage || 'PDF 生成失败' }}
        </n-alert>
        <n-alert v-if="taskState === 'done' && (anchorBreaks || anchorOversizes)" type="warning" class="log-pdf__alert" title="PDF 分页提示">
          <div v-if="anchorBreaks">{{ anchorBreaks }} 组消息及其锚定元素因显式分页被拆开。</div>
          <div v-if="anchorOversizes">{{ anchorOversizes }} 组消息及其锚定元素超出一页，已分开排版。</div>
        </n-alert>
        <n-alert v-if="taskState === 'done' && isZip" type="info" :show-icon="false" class="log-pdf__alert">
          服务器缺少 PDF 合并工具，结果为按顺序编号的分段 PDF（页码连续）。安装 pdfunite 或 qpdf 后重新导出可得到单个 PDF。
        </n-alert>
      </template>

      <n-alert v-if="errorText" type="error" :show-icon="false" class="log-pdf__alert">{{ errorText }}</n-alert>
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">关闭</n-button>
        <template v-if="mode === 'browser'">
          <n-button type="primary" @click="browserPrint">打印当前片段</n-button>
        </template>
        <template v-else-if="serverAvailable">
          <n-button v-if="taskState === 'done' || taskState === 'failed'" @click="reset">新建任务</n-button>
          <n-button v-if="taskState === 'failed'" type="primary" @click="retry">重试</n-button>
          <n-button v-else-if="taskState === 'done'" type="primary" :loading="downloading" @click="download">下载</n-button>
          <n-button v-else type="primary" :loading="running" :disabled="running" @click="start">生成 PDF</n-button>
        </template>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.log-pdf__modes { display: grid; gap: 10px; width: 100%; }
.log-pdf__mode {
  display: flex; gap: 10px; align-items: flex-start; padding: 12px 14px; cursor: pointer;
  border: 1px solid var(--sc-border-mute, rgba(127, 127, 127, 0.25)); border-radius: 8px;
}
.log-pdf__mode.is-active { border-color: var(--primary-color, #2f6fde); }
.log-pdf__mode.is-disabled { cursor: not-allowed; opacity: 0.6; }
.log-pdf__mode-body { display: grid; gap: 2px; min-width: 0; }
.log-pdf__mode-title { font-weight: 600; }
.log-pdf__mode-desc { font-size: 13px; line-height: 1.6; color: var(--sc-text-secondary, #64748b); }
.log-pdf__alert { margin-top: 14px; line-height: 1.7; }
.log-pdf__reason { margin-top: 4px; font-size: 12px; opacity: 0.85; }
.log-pdf__hint { margin: 0; font-size: 13px; line-height: 1.7; color: var(--sc-text-secondary, #64748b); }
.log-pdf__task { display: flex; gap: 8px; align-items: center; margin-top: 14px; }
.log-pdf__task-text.is-done { color: var(--success-color, #18a058); }
.log-pdf__task-text.is-failed { color: var(--error-color, #d03050); }
:deep(.log-pdf__modes .n-form-item) { margin: 0; }
</style>
