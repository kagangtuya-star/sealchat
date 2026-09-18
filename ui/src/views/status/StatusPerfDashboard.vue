<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import dayjs from 'dayjs';
import { useMessage } from 'naive-ui';
import { buildAuthorizedHeaders, urlBase } from '@/stores/_config';
import {
  useUtilsStore,
  type AdminPerfArtifact,
  type AdminPerfDurationStats,
  type AdminPerfMessagePipelineSummary,
  type AdminPerfSamplePoint,
  type AdminPerfState,
  type AdminPerfTopFunction,
} from '@/stores/utils';

type PerfRange = '15m' | '1h' | '6h' | '24h' | '7d' | 'custom';

const utils = useUtilsStore();
const message = useMessage();

const loading = ref(false);
const historyLoading = ref(false);
const cpuActionLoading = ref(false);
const pipelineLoading = ref(false);
const pipelineResetLoading = ref(false);
const traceActionLoading = ref(false);
const state = ref<AdminPerfState | null>(null);
const historyPoints = ref<AdminPerfSamplePoint[]>([]);
const artifacts = ref<AdminPerfArtifact[]>([]);
const topFunctions = ref<AdminPerfTopFunction[]>([]);
const range = ref<PerfRange>('1h');
const customRange = ref<[number, number] | null>(null);
const cpuDurationSec = ref<number | null>(null);
const traceDurationSec = ref<number | null>(30);
const pipelineWindowSec = ref(60);
const pipelineSummary = ref<AdminPerfMessagePipelineSummary | null>(null);
const refreshTimer = ref<number | null>(null);
const pipelineRefreshTimer = ref<number | null>(null);
const clockNow = ref(Date.now());

const rangeOptions = [
  { label: '近 15 分钟', value: '15m' },
  { label: '近 1 小时', value: '1h' },
  { label: '近 6 小时', value: '6h' },
  { label: '近 24 小时', value: '24h' },
  { label: '近 7 天', value: '7d' },
  { label: '自定义', value: 'custom' },
] as const;

const artifactColumns = [
  { title: '文件名', key: 'name' },
  { title: '类型', key: 'kind' },
  { title: '大小', key: 'sizeLabel' },
  { title: '更新时间', key: 'modifiedAtLabel' },
];

const topFunctionColumns = [
  { title: '函数', key: 'name' },
  { title: 'Flat', key: 'flatLabel' },
  { title: 'Flat %', key: 'flatPct' },
  { title: 'Cum', key: 'cumLabel' },
  { title: 'Cum %', key: 'cumPct' },
];

const pipelineWindowOptions = [
  { label: '30 秒', value: 30 },
  { label: '60 秒', value: 60 },
  { label: '5 分钟', value: 300 },
];

const durationColumns = [
  { title: '阶段', key: 'label' },
  { title: '样本数', key: 'countLabel' },
  { title: 'P50', key: 'p50Label' },
  { title: 'P95', key: 'p95Label' },
  { title: 'P99', key: 'p99Label' },
  { title: 'Max', key: 'maxLabel' },
];

const sparklineMetrics = [
  { key: 'goroutines', label: 'Goroutines', color: '#2563eb', format: (value: number) => formatNumber(value) },
  { key: 'heapAllocBytes', label: 'Heap Alloc', color: '#059669', format: (value: number) => formatBytes(value) },
  { key: 'heapInuseBytes', label: 'Heap Inuse', color: '#d97706', format: (value: number) => formatBytes(value) },
] as const;

const formatBytes = (value?: number) => {
  const size = value || 0;
  if (size <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let cursor = size;
  let unitIndex = 0;
  while (cursor >= 1024 && unitIndex < units.length - 1) {
    cursor /= 1024;
    unitIndex += 1;
  }
  const precision = unitIndex === 0 ? 0 : cursor >= 100 ? 0 : cursor >= 10 ? 1 : 2;
  return `${cursor.toFixed(precision)} ${units[unitIndex]}`;
};

const formatNumber = (value?: number) => new Intl.NumberFormat('zh-CN').format(value || 0);

const formatTimestamp = (value?: number) => {
  if (!value) return '未记录';
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss');
};

const formatDuration = (seconds?: number) => {
  const total = Math.max(0, Math.trunc(seconds || 0));
  const minutes = Math.floor(total / 60);
  const remain = total % 60;
  if (minutes <= 0) return `${remain} 秒`;
  return `${minutes} 分 ${remain} 秒`;
};

const formatLatency = (value?: number, count?: number) => {
  if (!count) return '—';
  const duration = value || 0;
  if (duration < 1) return `${duration.toFixed(2)} ms`;
  if (duration < 100) return `${duration.toFixed(1)} ms`;
  return `${duration.toFixed(duration < 1000 ? 1 : 0)} ms`;
};

const durationRow = (key: string, label: string, stats: AdminPerfDurationStats) => ({
  key,
  label,
  countLabel: formatNumber(stats.count),
  p50Label: formatLatency(stats.p50Ms, stats.count),
  p95Label: formatLatency(stats.p95Ms, stats.count),
  p99Label: formatLatency(stats.p99Ms, stats.count),
  maxLabel: formatLatency(stats.maxMs, stats.count),
});

const stateTagType = computed(() => {
  switch (state.value?.status) {
    case 'running':
      return 'success';
    case 'idle':
      return 'warning';
    case 'disabled':
      return 'default';
    default:
      return 'info';
  }
});

const latestSample = computed(() => state.value?.latest || null);

const statusCards = computed(() => {
  const latest = latestSample.value;
  return [
    {
      label: 'Goroutines',
      value: formatNumber(latest?.goroutines),
      hint: '轻量采样',
    },
    {
      label: 'Heap Alloc',
      value: formatBytes(latest?.heapAllocBytes),
      hint: '已分配堆内存',
    },
    {
      label: 'Heap Inuse',
      value: formatBytes(latest?.heapInuseBytes),
      hint: '正在使用的堆内存',
    },
    {
      label: 'GC 次数',
      value: formatNumber(latest?.gcCycles),
      hint: '进程累计 GC',
    },
  ];
});

const historySummary = computed(() => {
  const points = historyPoints.value;
  if (!points.length) {
    return {
      samples: '0',
      peakGoroutines: '0',
      peakHeapAlloc: '0 B',
      latestPauseMs: '0 ms',
    };
  }
  const peakGoroutines = Math.max(...points.map((item) => item.goroutines || 0));
  const peakHeapAlloc = Math.max(...points.map((item) => item.heapAllocBytes || 0));
  const latestPauseNs = points[points.length - 1]?.lastPauseNs || 0;
  return {
    samples: formatNumber(points.length),
    peakGoroutines: formatNumber(peakGoroutines),
    peakHeapAlloc: formatBytes(peakHeapAlloc),
    latestPauseMs: `${(latestPauseNs / 1_000_000).toFixed(latestPauseNs >= 100_000_000 ? 0 : 2)} ms`,
  };
});

const historyTable = computed(() => {
  return historyPoints.value
    .slice()
    .reverse()
    .slice(0, 20)
    .map((item) => ({
      timestamp: formatTimestamp(item.timestamp),
      goroutines: formatNumber(item.goroutines),
      heapAlloc: formatBytes(item.heapAllocBytes),
      heapInuse: formatBytes(item.heapInuseBytes),
      gcCycles: formatNumber(item.gcCycles),
      lastPause: `${(item.lastPauseNs / 1_000_000).toFixed(item.lastPauseNs >= 100_000_000 ? 0 : 2)} ms`,
    }));
});

const artifactRows = computed(() => {
  return artifacts.value.map((item) => ({
    ...item,
    sizeLabel: formatBytes(item.size),
    modifiedAtLabel: formatTimestamp(item.modifiedAt),
  }));
});

const topFunctionRows = computed(() => {
  return topFunctions.value.map((item) => ({
    ...item,
    flatLabel: `${formatNumber(item.flat)} ${item.unit || ''}`.trim(),
    cumLabel: `${formatNumber(item.cumulative)} ${item.unit || ''}`.trim(),
  }));
});

const buildSparklinePoints = (values: number[]) => {
  if (!values.length) return '';
  const width = 100;
  const height = 36;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  return values
    .map((value, index) => {
      const x = values.length === 1 ? width / 2 : (index / (values.length - 1)) * width;
      const y = height - ((value - min) / range) * height;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
};

const sparklinePanels = computed(() => {
  return sparklineMetrics.map((metric) => {
    const values = historyPoints.value.map((item) => Number(item[metric.key] || 0));
    const latest = values[values.length - 1] || 0;
    const peak = values.length ? Math.max(...values) : 0;
    const first = values[0] || 0;
    const delta = latest - first;
    return {
      key: metric.key,
      label: metric.label,
      color: metric.color,
      latest: metric.format(latest),
      peak: metric.format(peak),
      delta,
      deltaText: `${delta >= 0 ? '+' : ''}${metric.format(Math.abs(delta))}`,
      points: buildSparklinePoints(values),
    };
  });
});

const currentRangeParams = computed(() => {
  if (range.value === 'custom') {
    if (!customRange.value || customRange.value.length !== 2) {
      return null;
    }
    const [start, end] = customRange.value;
    if (!start || !end || start >= end) {
      return null;
    }
    return { start, end };
  }
  return { range: range.value };
});

const cpuSessionCountdown = computed(() => {
  const session = state.value?.cpuSession;
  if (!session?.active || !session.endsAt) return '未录制';
  const remainSeconds = Math.max(0, Math.ceil((session.endsAt - Date.now()) / 1000));
  return formatDuration(remainSeconds);
});

const traceSessionCountdown = computed(() => {
  const session = state.value?.traceSession;
  if (!session?.active || !session.endsAt) return '未录制';
  return formatDuration(Math.max(0, Math.ceil((session.endsAt - clockNow.value) / 1000)));
});

const pipelineRows = computed(() => (pipelineSummary.value?.stages || []).map((stage) => durationRow(stage.key, stage.label, stage)));

const digestRows = computed(() => {
  const digest = pipelineSummary.value?.digest;
  if (!digest) return [];
  return [
    durationRow('digest_total', 'Digest Total', digest.total),
    durationRow('visitor_upserts', 'Visitor Upserts', digest.visitorUpserts),
    durationRow('speaker_upserts', 'Speaker Upserts', digest.speakerUpserts),
  ];
});

const wsResponseRows = computed(() => {
  const ws = pipelineSummary.value?.ws;
  if (!ws) return [];
  return [
    durationRow('response_queue_wait', 'API Response Queue Wait', ws.responseQueueWait),
    durationRow('response_socket_write', 'API Response Socket Write', ws.responseSocketWrite),
  ];
});

const artifactDownloadUrl = (name: string) => {
  return `${urlBase}/api/v1/admin/perf/artifacts/${encodeURIComponent(name)}/download`;
};

const downloadArtifact = async (name: string) => {
  try {
    const resp = await fetch(artifactDownloadUrl(name), {
      credentials: 'include',
      headers: buildAuthorizedHeaders(),
    });
    if (!resp.ok) {
      throw new Error(`HTTP ${resp.status}`);
    }
    const blob = await resp.blob();
    const objectUrl = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = objectUrl;
    link.download = name;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(objectUrl);
  } catch (error: any) {
    console.error(error);
    message.error(error?.message || '下载性能检测文件失败');
  }
};

const refreshStatus = async () => {
  const resp = await utils.adminPerfStatus();
  state.value = resp.data.state;
  if (cpuDurationSec.value === null && resp.data.state?.cpuProfileDurationSec) {
    cpuDurationSec.value = resp.data.state.cpuProfileDurationSec;
  }
};

const refreshArtifacts = async () => {
  const resp = await utils.adminPerfArtifacts();
  artifacts.value = resp.data.items || [];
};

const refreshTopFunctions = async () => {
  const resp = await utils.adminPerfTopFunctions();
  topFunctions.value = resp.data.items || [];
};

const refreshHistory = async () => {
  const params = currentRangeParams.value;
  if (!params) {
    historyPoints.value = [];
    return;
  }
  historyLoading.value = true;
  try {
    const resp = await utils.adminPerfHistory(params as any);
    historyPoints.value = resp.data.points || [];
  } finally {
    historyLoading.value = false;
  }
};

const refreshPipeline = async () => {
  pipelineLoading.value = true;
  try {
    const resp = await utils.adminPerfMessagePipeline(pipelineWindowSec.value);
    pipelineSummary.value = resp.data.summary;
  } finally {
    pipelineLoading.value = false;
  }
};

const refreshAll = async () => {
  loading.value = true;
  try {
    await Promise.all([refreshStatus(), refreshArtifacts(), refreshHistory(), refreshTopFunctions()]);
  } catch (error: any) {
    console.error(error);
    message.error(error?.response?.data?.message || '获取性能检测状态失败');
  } finally {
    loading.value = false;
  }
};

const startCpuSession = async () => {
  cpuActionLoading.value = true;
  try {
    await utils.adminPerfStartCpuSession(cpuDurationSec.value || undefined);
    message.success('连续 CPU 录制已启动');
    await Promise.all([refreshStatus(), refreshArtifacts(), refreshTopFunctions()]);
  } catch (error: any) {
    console.error(error);
    message.error(error?.response?.data?.message || '启动连续 CPU 录制失败');
  } finally {
    cpuActionLoading.value = false;
  }
};

const stopCpuSession = async () => {
  cpuActionLoading.value = true;
  try {
    await utils.adminPerfStopCpuSession();
    message.success('连续 CPU 录制已停止');
    await Promise.all([refreshStatus(), refreshArtifacts(), refreshTopFunctions()]);
  } catch (error: any) {
    console.error(error);
    message.error(error?.response?.data?.message || '停止连续 CPU 录制失败');
  } finally {
    cpuActionLoading.value = false;
  }
};

const resetPipeline = async () => {
  pipelineResetLoading.value = true;
  try {
    await utils.adminPerfMessagePipelineReset();
    await refreshPipeline();
    message.success('消息链路诊断数据已清空');
  } catch (error: any) {
    console.error(error);
    message.error(error?.response?.data?.message || '清空消息链路诊断数据失败');
  } finally {
    pipelineResetLoading.value = false;
  }
};

const startTraceSession = async () => {
  traceActionLoading.value = true;
  try {
    await utils.adminPerfStartTraceSession(traceDurationSec.value || undefined);
    await Promise.all([refreshStatus(), refreshArtifacts()]);
    message.success('Runtime Trace 已启动');
  } catch (error: any) {
    console.error(error);
    message.error(error?.response?.data?.message || '启动 Runtime Trace 失败');
  } finally {
    traceActionLoading.value = false;
  }
};

const stopTraceSession = async () => {
  traceActionLoading.value = true;
  try {
    await utils.adminPerfStopTraceSession();
    await Promise.all([refreshStatus(), refreshArtifacts()]);
    message.success('Runtime Trace 已停止');
  } catch (error: any) {
    console.error(error);
    message.error(error?.response?.data?.message || '停止 Runtime Trace 失败');
  } finally {
    traceActionLoading.value = false;
  }
};

watch(currentRangeParams, () => {
  void refreshHistory();
}, { deep: true });

watch(pipelineWindowSec, () => {
  void refreshPipeline();
});

onMounted(() => {
	void refreshAll();
	void refreshPipeline();
  refreshTimer.value = window.setInterval(() => {
    void Promise.allSettled([refreshStatus(), refreshArtifacts(), refreshTopFunctions()]);
  }, 15_000);
	pipelineRefreshTimer.value = window.setInterval(() => {
	  clockNow.value = Date.now();
	  void refreshPipeline();
	}, 2_000);
});

onBeforeUnmount(() => {
  if (refreshTimer.value) {
    window.clearInterval(refreshTimer.value);
  }
	if (pipelineRefreshTimer.value) {
	  window.clearInterval(pipelineRefreshTimer.value);
	}
});
</script>

<template>
  <div class="perf-page">
    <n-page-header title="性能检测">
      <template #subtitle>
        状态页子路由，默认展示轻量采样、定期快照和本地 artifacts。
      </template>
      <template #extra>
        <n-space align="center">
          <n-button size="small" :loading="loading || historyLoading" @click="refreshAll">刷新</n-button>
          <n-button size="small" secondary tag="a" href="#/status">返回基础状态页</n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-alert v-if="state?.lastError" type="warning" :show-icon="false" class="perf-alert">
      最近错误：{{ state.lastError }}
    </n-alert>

    <n-grid cols="1 900:3" :x-gap="18" :y-gap="18">
      <n-grid-item>
        <n-card title="运行状态" size="small" class="perf-card">
          <div class="perf-stack">
            <div class="perf-row">
              <span class="perf-label">总开关</span>
              <n-tag :type="state?.enabled ? 'success' : 'default'">{{ state?.enabled ? '已开启' : '已关闭' }}</n-tag>
            </div>
            <div class="perf-row">
              <span class="perf-label">运行态</span>
              <n-tag :type="stateTagType">{{ state?.status || '未知' }}</n-tag>
            </div>
            <div class="perf-row">
              <span class="perf-label">输出目录</span>
              <span class="perf-value perf-value--mono">{{ state?.outputDir || './data/perf' }}</span>
            </div>
            <div class="perf-row">
              <span class="perf-label">最近采样</span>
              <span class="perf-value">{{ formatTimestamp(state?.lastSampleAt) }}</span>
            </div>
            <div class="perf-row">
              <span class="perf-label">轻量采样</span>
              <span class="perf-value">{{ formatDuration(state?.lightIntervalSec) }}</span>
            </div>
            <div class="perf-row">
              <span class="perf-label">定期快照</span>
              <span class="perf-value">{{ formatDuration(state?.snapshotIntervalSec) }}</span>
            </div>
            <div class="perf-row">
              <span class="perf-label">保留期</span>
              <span class="perf-value">{{ state?.retentionDays || 0 }} 天</span>
            </div>
          </div>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card title="连续 CPU 录制" size="small" class="perf-card">
          <div class="perf-stack">
            <div class="perf-row">
              <span class="perf-label">当前会话</span>
              <n-tag :type="state?.cpuSession?.active ? 'error' : 'default'">
                {{ state?.cpuSession?.active ? '录制中' : '空闲' }}
              </n-tag>
            </div>
            <div class="perf-row">
              <span class="perf-label">剩余时间</span>
              <span class="perf-value">{{ cpuSessionCountdown }}</span>
            </div>
            <div class="perf-row">
              <span class="perf-label">输出文件</span>
              <span class="perf-value perf-value--mono">{{ state?.cpuSession?.fileName || '未生成' }}</span>
            </div>
            <div class="perf-row">
              <span class="perf-label">默认时长</span>
              <n-input-number v-model:value="cpuDurationSec" :min="10" :precision="0" style="width: 140px">
                <template #suffix>秒</template>
              </n-input-number>
            </div>
            <n-space>
              <n-button
                type="error"
                :disabled="!state?.enabled || Boolean(state?.cpuSession?.active)"
                :loading="cpuActionLoading"
                @click="startCpuSession"
              >
                开始连续录制
              </n-button>
              <n-button
                secondary
                :disabled="!state?.cpuSession?.active"
                :loading="cpuActionLoading"
                @click="stopCpuSession"
              >
                停止录制
              </n-button>
            </n-space>
          </div>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card title="最近采样" size="small" class="perf-card">
          <div class="perf-metric-grid">
            <div v-for="card in statusCards" :key="card.label" class="perf-metric">
              <div class="perf-metric__label">{{ card.label }}</div>
              <div class="perf-metric__value">{{ card.value }}</div>
              <div class="perf-metric__hint">{{ card.hint }}</div>
            </div>
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <n-card title="轻量采样历史" size="small" class="perf-card">
      <template #header-extra>
        <n-space align="center">
          <n-select v-model:value="range" size="small" :options="rangeOptions" style="width: 140px" />
          <n-date-picker
            v-if="range === 'custom'"
            v-model:value="customRange"
            type="datetimerange"
            clearable
            size="small"
            style="width: 280px"
          />
        </n-space>
      </template>

      <n-spin :show="historyLoading">
        <div class="perf-summary-strip">
          <div class="perf-summary-item">
            <span class="perf-summary-item__label">样本数</span>
            <strong>{{ historySummary.samples }}</strong>
          </div>
          <div class="perf-summary-item">
            <span class="perf-summary-item__label">Goroutines 峰值</span>
            <strong>{{ historySummary.peakGoroutines }}</strong>
          </div>
          <div class="perf-summary-item">
            <span class="perf-summary-item__label">Heap Alloc 峰值</span>
            <strong>{{ historySummary.peakHeapAlloc }}</strong>
          </div>
          <div class="perf-summary-item">
            <span class="perf-summary-item__label">最近 GC 停顿</span>
            <strong>{{ historySummary.latestPauseMs }}</strong>
          </div>
        </div>

        <div v-if="sparklinePanels.some((item) => item.points)" class="perf-sparkline-grid">
          <div v-for="item in sparklinePanels" :key="item.key" class="perf-sparkline-card">
            <div class="perf-sparkline-card__header">
              <div>
                <div class="perf-sparkline-card__label">{{ item.label }}</div>
                <div class="perf-sparkline-card__value">{{ item.latest }}</div>
              </div>
              <div class="perf-sparkline-card__meta">
                <span>峰值 {{ item.peak }}</span>
                <span :class="item.delta >= 0 ? 'is-up' : 'is-down'">变化 {{ item.deltaText }}</span>
              </div>
            </div>
            <svg viewBox="0 0 100 36" preserveAspectRatio="none" class="perf-sparkline-card__svg">
              <polyline
                :points="item.points"
                fill="none"
                :stroke="item.color"
                stroke-width="2.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </div>
        </div>
      </n-spin>
    </n-card>

    <n-grid cols="1 960:3" :x-gap="18" :y-gap="18">
      <n-grid-item>
        <n-card title="采样明细" size="small" class="perf-card">
          <div class="perf-card__subcopy">按当前时间范围展示最近 20 条轻量采样记录。</div>
          <n-collapse class="perf-collapse" :default-expanded-names="[]">
            <n-collapse-item title="轻量采样明细列表" name="history-table">
              <div class="perf-table-wrap">
                <n-table v-if="historyTable.length" striped size="small" class="perf-table">
                  <thead>
                    <tr>
                      <th>时间</th>
                      <th>Goroutines</th>
                      <th>Heap Alloc</th>
                      <th>Heap Inuse</th>
                      <th>GC 次数</th>
                      <th>最近停顿</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="item in historyTable" :key="item.timestamp">
                      <td>{{ item.timestamp }}</td>
                      <td>{{ item.goroutines }}</td>
                      <td>{{ item.heapAlloc }}</td>
                      <td>{{ item.heapInuse }}</td>
                      <td>{{ item.gcCycles }}</td>
                      <td>{{ item.lastPause }}</td>
                    </tr>
                  </tbody>
                </n-table>
                <n-empty v-else description="当前时间范围内还没有采样数据" />
              </div>
            </n-collapse-item>
          </n-collapse>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card title="Artifacts" size="small" class="perf-card">
          <div class="perf-card__subcopy">样本文件不会展示；这里只列快照和 CPU 会话产物。</div>
          <n-collapse class="perf-collapse" :default-expanded-names="[]">
            <n-collapse-item title="Artifacts 列表" name="artifacts-table">
              <n-data-table
                :columns="artifactColumns"
                :data="artifactRows"
                :bordered="false"
                size="small"
                :pagination="{ pageSize: 8 }"
                :scroll-x="760"
              />
              <div v-if="artifactRows.length" class="perf-artifact-actions">
                <button
                  v-for="item in artifactRows.slice(0, 8)"
                  :key="item.name"
                  class="perf-download-link"
                  type="button"
                  @click="downloadArtifact(item.name)"
                >
                  下载 {{ item.name }}
                </button>
              </div>
              <n-empty v-else description="暂无快照或 CPU 会话文件" />
            </n-collapse-item>
          </n-collapse>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card title="CPU 热点函数 Top 10" size="small" class="perf-card">
          <div class="perf-card__subcopy">
            基于最近一份 CPU artifact 解析；优先使用连续录制，其次使用定期 CPU snapshot。
          </div>
          <n-collapse class="perf-collapse" :default-expanded-names="[]">
            <n-collapse-item title="CPU 热点函数 Top 10 列表" name="top-functions-table">
              <n-data-table
                v-if="topFunctionRows.length"
                :columns="topFunctionColumns"
                :data="topFunctionRows"
                :bordered="false"
                size="small"
                :pagination="false"
                :scroll-x="920"
              />
              <div v-if="topFunctionRows.length" class="perf-top-functions-source">
                来源：{{ topFunctionRows[0].source }}
              </div>
              <n-empty v-else description="暂无可解析的 CPU profile，先执行一次连续录制或等待定期 CPU snapshot 生成。" />
            </n-collapse-item>
          </n-collapse>
        </n-card>
      </n-grid-item>
    </n-grid>

    <n-card title="消息并发诊断" size="small" class="perf-card">
      <template #header-extra>
        <n-space align="center">
          <n-radio-group v-model:value="pipelineWindowSec" size="small">
            <n-radio-button v-for="option in pipelineWindowOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </n-radio-button>
          </n-radio-group>
          <n-button size="small" :loading="pipelineLoading" @click="refreshPipeline">刷新</n-button>
          <n-button size="small" secondary :loading="pipelineResetLoading" @click="resetPipeline">清空诊断数据</n-button>
        </n-space>
      </template>
      <div class="perf-card__subcopy">
        最近 {{ pipelineSummary?.windowSec || pipelineWindowSec }} 秒内采集 {{ formatNumber(pipelineSummary?.messageCount) }} 条 message.create；
        本轮自 {{ formatTimestamp(pipelineSummary?.sinceResetAt) }} 起。
      </div>

      <n-card title="消息发送链路" size="small" embedded class="perf-inner-card">
        <n-data-table
          :columns="durationColumns"
          :data="pipelineRows"
          :bordered="false"
          size="small"
          :pagination="false"
          :scroll-x="720"
        />
      </n-card>

      <n-grid cols="1 900:2" :x-gap="18" :y-gap="18" class="perf-diagnostic-grid">
        <n-grid-item>
          <n-card title="数据库连接池" size="small" embedded class="perf-inner-card">
            <div class="perf-metric-grid">
              <div class="perf-metric"><div class="perf-metric__label">Driver</div><div class="perf-metric__value perf-metric__value--small">{{ pipelineSummary?.db?.driver || '未连接' }}</div></div>
              <div class="perf-metric"><div class="perf-metric__label">Max Open</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.db?.maxOpenConnections) }}</div></div>
              <div class="perf-metric"><div class="perf-metric__label">Open</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.db?.openConnections) }}</div></div>
              <div class="perf-metric"><div class="perf-metric__label">In Use / Idle</div><div class="perf-metric__value perf-metric__value--small">{{ formatNumber(pipelineSummary?.db?.inUse) }} / {{ formatNumber(pipelineSummary?.db?.idle) }}</div></div>
              <div class="perf-metric"><div class="perf-metric__label">本轮 Wait Count</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.db?.waitCountDelta) }}</div></div>
              <div class="perf-metric"><div class="perf-metric__label">本轮 Wait Duration</div><div class="perf-metric__value perf-metric__value--small">{{ formatLatency(pipelineSummary?.db?.waitDurationMsDelta, 1) }}</div></div>
            </div>
            <div class="perf-card__subcopy perf-db-cumulative">
              累计 Wait Count {{ formatNumber(pipelineSummary?.db?.waitCount) }}，Wait Duration {{ formatLatency(pipelineSummary?.db?.waitDurationMs, 1) }}。
              这些值仅表示 database/sql 连接池等待，不等同于 SQLite write lock 等待。
            </div>
          </n-card>
        </n-grid-item>

        <n-grid-item>
          <n-card title="Digest 异步压力" size="small" embedded class="perf-inner-card">
            <div class="perf-summary-strip perf-summary-strip--five">
              <div class="perf-summary-item"><span class="perf-summary-item__label">Started</span><strong>{{ formatNumber(pipelineSummary?.digest?.started) }}</strong></div>
              <div class="perf-summary-item"><span class="perf-summary-item__label">Completed</span><strong>{{ formatNumber(pipelineSummary?.digest?.completed) }}</strong></div>
              <div class="perf-summary-item"><span class="perf-summary-item__label">Errors</span><strong>{{ formatNumber(pipelineSummary?.digest?.errors) }}</strong></div>
              <div class="perf-summary-item"><span class="perf-summary-item__label">In Flight</span><strong>{{ formatNumber(pipelineSummary?.digest?.inFlight) }}</strong></div>
              <div class="perf-summary-item"><span class="perf-summary-item__label">Peak In Flight</span><strong>{{ formatNumber(pipelineSummary?.digest?.peakInFlight) }}</strong></div>
            </div>
            <n-data-table :columns="durationColumns" :data="digestRows" :bordered="false" size="small" :pagination="false" :scroll-x="720" />
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-card title="WebSocket 出站诊断" size="small" embedded class="perf-inner-card perf-trace-card">
        <n-data-table :columns="durationColumns" :data="wsResponseRows" :bordered="false" size="small" :pagination="false" :scroll-x="720" />
        <div class="perf-card__subcopy perf-ws-copy">
	          Queue Wait 表示 message.create response 从进入 interactive queue 到 outboundWriter 取出的等待时间。<br>
	          Socket Write 表示 outboundWriter 实际执行 WebSocket WriteMessage 的耗时。<br>
	          Queue Depth Ahead 表示 message.create response 入 interactive queue 时排在它前面的 interactive frame 数量，为并发近似值。
        </div>
        <div class="perf-summary-strip perf-summary-strip--five">
          <div class="perf-summary-item"><span class="perf-summary-item__label">Queue Depth Ahead P50</span><strong>{{ formatNumber(pipelineSummary?.ws?.responseQueueDepth?.p50) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">P95</span><strong>{{ formatNumber(pipelineSummary?.ws?.responseQueueDepth?.p95) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">P99</span><strong>{{ formatNumber(pipelineSummary?.ws?.responseQueueDepth?.p99) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">Max</span><strong>{{ formatNumber(pipelineSummary?.ws?.responseQueueDepth?.max) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">Samples</span><strong>{{ formatNumber(pipelineSummary?.ws?.responseQueueDepth?.count) }}</strong></div>
        </div>
	        <div class="perf-card__subcopy perf-ws-copy">Reliable 出站组成（本轮累计）</div>
        <div class="perf-summary-strip perf-summary-strip--five">
          <div class="perf-summary-item"><span class="perf-summary-item__label">Total</span><strong>{{ formatNumber(pipelineSummary?.ws?.reliableEnqueuedTotal) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">Message Created</span><strong>{{ formatNumber(pipelineSummary?.ws?.reliableMessageCreated) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">Message Create Response</span><strong>{{ formatNumber(pipelineSummary?.ws?.reliableMessageCreateResponse) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">BOT Event</span><strong>{{ formatNumber(pipelineSummary?.ws?.reliableBotEvent) }}</strong></div>
          <div class="perf-summary-item"><span class="perf-summary-item__label">Other</span><strong>{{ formatNumber(pipelineSummary?.ws?.reliableOther) }}</strong></div>
        </div>
        <div class="perf-card__subcopy perf-ws-copy">
	          这些计数表示自上次清空诊断数据后成功进入 reliable 出站队列的 frame 数量。
        </div>
        <div class="perf-metric-grid perf-ws-counters">
          <div class="perf-metric"><div class="perf-metric__label">本轮累计 Reliable Queue Full</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.ws?.reliableQueueFull) }}</div></div>
          <div class="perf-metric"><div class="perf-metric__label">本轮累计 Response Queue Full</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.ws?.responseQueueFull) }}</div></div>
          <div class="perf-metric"><div class="perf-metric__label">Response Errors</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.ws?.responseErrors) }}</div></div>
          <div class="perf-metric"><div class="perf-metric__label">本轮累计 Coalesced Enqueued</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.ws?.coalescedEnqueued) }}</div></div>
          <div class="perf-metric"><div class="perf-metric__label">本轮累计 Coalesced Replaced</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.ws?.coalescedReplaced) }}</div></div>
          <div class="perf-metric"><div class="perf-metric__label">本轮累计 Coalesced Evicted</div><div class="perf-metric__value">{{ formatNumber(pipelineSummary?.ws?.coalescedEvicted) }}</div></div>
        </div>
        <div class="perf-card__subcopy perf-ws-copy">Coalesced Replaced 越高，说明跨频道派生通知合并实际生效。</div>
      </n-card>

      <n-card title="运行时 Trace" size="small" embedded class="perf-inner-card perf-trace-card">
        <div class="perf-card__subcopy">Runtime Trace 会记录 goroutine 调度、阻塞、syscall 等信息，仅建议在短时间压力测试中开启。</div>
        <div class="perf-trace-layout">
          <div class="perf-stack">
            <div class="perf-row"><span class="perf-label">当前状态</span><n-tag :type="state?.traceSession?.active ? 'error' : 'default'">{{ state?.traceSession?.active ? '录制中' : '空闲' }}</n-tag></div>
            <div class="perf-row"><span class="perf-label">剩余时间</span><span class="perf-value">{{ traceSessionCountdown }}</span></div>
            <div class="perf-row"><span class="perf-label">输出文件</span><span class="perf-value perf-value--mono">{{ state?.traceSession?.fileName || '未生成' }}</span></div>
          </div>
          <n-space align="center">
            <n-input-number v-model:value="traceDurationSec" :min="5" :max="120" :precision="0" style="width: 130px"><template #suffix>秒</template></n-input-number>
            <n-button type="error" :disabled="!state?.enabled || Boolean(state?.traceSession?.active)" :loading="traceActionLoading" @click="startTraceSession">开始 Trace</n-button>
            <n-button secondary :disabled="!state?.traceSession?.active" :loading="traceActionLoading" @click="stopTraceSession">停止 Trace</n-button>
          </n-space>
        </div>
      </n-card>
    </n-card>
  </div>
</template>

<style scoped lang="scss">
.perf-page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  height: 100vh;
  min-height: 100vh;
  padding: 1.25rem;
  box-sizing: border-box;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-gutter: stable;
  color: var(--sc-text-primary);
  background:
    radial-gradient(900px circle at 100% 0%, color-mix(in srgb, #dbeafe 75%, transparent) 0%, transparent 45%),
    radial-gradient(1000px circle at 0% 100%, color-mix(in srgb, #dcfce7 68%, transparent) 0%, transparent 50%),
    linear-gradient(180deg, color-mix(in srgb, var(--sc-bg-elevated) 38%, #f8fafc 62%) 0%, var(--sc-bg-surface) 100%);
}

.perf-page__hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.perf-alert {
  margin-top: -0.25rem;
}

.perf-card {
  border-radius: 1rem;
  border: 1px solid color-mix(in srgb, var(--sc-border-mute) 88%, transparent);
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--sc-bg-elevated) 82%, rgba(255, 255, 255, 0.5) 18%) 0%,
      color-mix(in srgb, var(--sc-bg-surface) 92%, rgba(255, 255, 255, 0.75) 8%) 100%
    );
  box-shadow: 0 14px 36px color-mix(in srgb, var(--sc-border-strong) 12%, transparent);
}

.perf-card__subcopy {
  margin-bottom: 0.6rem;
  font-size: 0.78rem;
  line-height: 1.55;
  color: var(--sc-text-secondary);
}

.perf-stack {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.perf-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.perf-label {
  color: var(--sc-text-secondary);
  font-size: 0.85rem;
}

.perf-value {
  text-align: right;
  font-weight: 500;
}

.perf-value--mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.8rem;
}

.perf-metric-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.9rem;
}

.perf-metric {
  padding: 0.85rem 0.9rem;
  border-radius: 0.9rem;
  background: color-mix(in srgb, var(--sc-bg-elevated) 86%, rgba(255, 255, 255, 0.65) 14%);
  border: 1px solid color-mix(in srgb, var(--sc-border-mute) 75%, transparent);
}

.perf-metric__label {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.perf-metric__value {
  margin-top: 0.35rem;
  font-size: 1.35rem;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.perf-metric__value--small {
  font-size: 1rem;
}

.perf-metric__hint {
  margin-top: 0.2rem;
  font-size: 0.72rem;
  color: var(--sc-text-secondary);
}

.perf-summary-strip {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.perf-summary-item {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  padding: 0.75rem 0.9rem;
  border-radius: 0.85rem;
  background: color-mix(in srgb, var(--sc-bg-elevated) 84%, rgba(255, 255, 255, 0.6) 16%);
}

.perf-summary-item__label {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.perf-inner-card {
  margin-top: 1rem;
}

.perf-diagnostic-grid {
  margin-top: 0.1rem;
}

.perf-summary-strip--five {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.perf-db-cumulative {
  margin-top: 0.8rem;
  margin-bottom: 0;
}

.perf-trace-card {
  margin-top: 1rem;
}

.perf-trace-layout {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) auto;
  align-items: end;
  gap: 1rem;
}

.perf-collapse {
  margin-top: 0.25rem;
}

.perf-table-wrap {
  overflow-x: auto;
}

.perf-sparkline-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.85rem;
  margin-bottom: 1rem;
}

.perf-sparkline-card {
  padding: 0.85rem 0.95rem;
  border-radius: 0.95rem;
  border: 1px solid color-mix(in srgb, var(--sc-border-mute) 76%, transparent);
  background: color-mix(in srgb, var(--sc-bg-elevated) 86%, rgba(255, 255, 255, 0.55) 14%);
}

.perf-sparkline-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.7rem;
}

.perf-sparkline-card__label {
  font-size: 0.74rem;
  color: var(--sc-text-secondary);
}

.perf-sparkline-card__value {
  margin-top: 0.15rem;
  font-size: 1rem;
  font-weight: 700;
}

.perf-sparkline-card__meta {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  text-align: right;
  font-size: 0.72rem;
  color: var(--sc-text-secondary);
}

.perf-sparkline-card__meta .is-up {
  color: #047857;
}

.perf-sparkline-card__meta .is-down {
  color: #b91c1c;
}

.perf-sparkline-card__svg {
  width: 100%;
  height: 54px;
  display: block;
}

.perf-table {
  margin-top: 0.2rem;
}

.perf-artifact-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.65rem 1rem;
  margin-top: 1rem;
}

.perf-download-link {
  font-size: 0.82rem;
  color: #2563eb;
  background: transparent;
  border: 0;
  padding: 0;
  cursor: pointer;
}

.perf-download-link:hover {
  text-decoration: underline;
}

.perf-top-functions-source {
  margin-top: 0.75rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

@media (max-width: 768px) {
  .perf-page {
    padding: 0.85rem;
  }

  .perf-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .perf-value {
    text-align: left;
  }

  .perf-metric-grid,
  .perf-summary-strip,
  .perf-sparkline-grid {
    grid-template-columns: 1fr;
  }

  .perf-trace-layout {
    grid-template-columns: 1fr;
  }
}
</style>
