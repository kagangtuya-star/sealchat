<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import dayjs from 'dayjs';
import { useMessage } from 'naive-ui';
import { api } from '@/stores/_config';
import { useUserStore } from '@/stores/user';

type RangeOption = '1h' | '24h' | '7d';

type ChartMetricKey =
  | 'concurrentConnections'
  | 'onlineUsers'
  | 'messagesPerMinute'
  | 'attachmentCount'
  | 'attachmentBytes';

interface StatusSummary {
  timestamp: number;
  concurrentConnections: number;
  onlineUsers: number;
  messagesPerMinute: number;
  registeredUsers: number;
  worldCount: number;
  channelCount: number;
  privateChannelCount: number;
  messageCount: number;
  attachmentCount: number;
  attachmentBytes: number;
  intervalSeconds: number;
  retentionDays: number;
}

interface StatusHistoryPoint {
  timestamp: number;
  concurrentConnections: number;
  onlineUsers: number;
  messagesPerMinute: number;
  registeredUsers: number;
  worldCount: number;
  channelCount: number;
  privateChannelCount: number;
  messageCount: number;
  attachmentCount: number;
  attachmentBytes: number;
}

const user = useUserStore();
const message = useMessage();

const summary = ref<StatusSummary | null>(null);
const historyPoints = ref<StatusHistoryPoint[]>([]);
const loading = ref(false);
const historyLoading = ref(false);
const selectedRange = ref<RangeOption>('1h');
const refreshTimer = ref<number | null>(null);

const rangeOptions = [
  { label: '近 1 小时', value: '1h' },
  { label: '近 24 小时', value: '24h' },
  { label: '近 7 天', value: '7d' },
];

const numberFormatter = new Intl.NumberFormat('zh-CN');
const formatNumber = (value?: number) => numberFormatter.format(value || 0);
const formatBytes = (value?: number) => {
  const size = value || 0;
  if (size <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let cursor = size;
  let unitIndex = 0;
  while (cursor >= 1024 && unitIndex < units.length - 1) {
    cursor /= 1024;
    unitIndex += 1;
  }
  const precision = unitIndex === 0 ? 0 : cursor >= 100 ? 0 : cursor >= 10 ? 1 : 2;
  return `${cursor.toFixed(precision)} ${units[unitIndex]}`;
};

const chartMetrics: { key: ChartMetricKey; label: string; color: string; format: (value: number) => string }[] = [
  { key: 'concurrentConnections', label: '并发连接', color: '#2563eb', format: formatNumber },
  { key: 'onlineUsers', label: '在线用户', color: '#059669', format: formatNumber },
  { key: 'messagesPerMinute', label: '消息/分钟', color: '#f97316', format: formatNumber },
  { key: 'attachmentCount', label: '附件数量', color: '#0ea5e9', format: formatNumber },
  { key: 'attachmentBytes', label: '附件总大小', color: '#ca8a04', format: formatBytes },
];

const lastUpdatedText = computed(() => {
  if (!summary.value?.timestamp) {
    return '尚无数据';
  }
  return dayjs(summary.value.timestamp).format('YYYY-MM-DD HH:mm:ss');
});

const summaryCards = computed(() => {
  if (!summary.value) {
    return [];
  }
  const data = summary.value;
  return [
    { label: '并发连接', value: formatNumber(data.concurrentConnections), hint: '当前活跃 WebSocket 数' },
    { label: '在线用户', value: formatNumber(data.onlineUsers), hint: '120 秒内仍活跃的用户' },
    { label: '消息 / 分钟', value: formatNumber(data.messagesPerMinute), hint: '最近一分钟的消息吞吐' },
    { label: '注册用户', value: formatNumber(data.registeredUsers), hint: '未被禁用的账户数量' },
    { label: '世界总数', value: formatNumber(data.worldCount), hint: '状态正常的世界' },
    { label: '公共频道', value: formatNumber(data.channelCount), hint: '状态正常的公共频道' },
    { label: '私聊频道', value: formatNumber(data.privateChannelCount), hint: '状态正常的私聊频道' },
    { label: '消息总数', value: formatNumber(data.messageCount), hint: '未被删除的历史消息' },
    { label: '附件数量', value: formatNumber(data.attachmentCount), hint: '附件目录内文件数量' },
    { label: '附件总大小', value: formatBytes(data.attachmentBytes), hint: '附件目录占用空间' },
  ];
});

const chartWidth = 680;
const chartHeight = 240;

const chartSeries = computed(() => {
  const points = historyPoints.value;
  const count = points.length;
  const width = chartWidth;
  const height = chartHeight;
  return chartMetrics.map((metric) => {
    let maxValue = 0;
    points.forEach((point) => {
      maxValue = Math.max(maxValue, point[metric.key] || 0);
    });
    if (maxValue <= 0) {
      maxValue = 1;
    }
    const coords = points.map((point, index) => {
      const value = point[metric.key] || 0;
      const x = count <= 1 ? width : (index / (count - 1)) * width;
      const normalized = Math.min(value / maxValue, 1);
      const y = height - normalized * height;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    });
    const ticks = [
      { value: maxValue, label: metric.format(maxValue) },
      { value: maxValue / 2, label: metric.format(Math.round(maxValue / 2)) },
      { value: 0, label: metric.format(0) },
    ];
    return {
      ...metric,
      path: coords.join(' '),
      ticks,
      maxValue,
    };
  });
});

const chartTicksX = computed(() => {
  const points = historyPoints.value;
  if (!points.length) {
    return [];
  }
  if (points.length === 1) {
    return [{ x: 0, label: dayjs(points[0].timestamp).format('HH:mm') }];
  }
  const indexes = [0, Math.floor((points.length - 1) / 2), points.length - 1];
  const unique = Array.from(new Set(indexes));
  return unique.map((idx) => {
    const x = (idx / (points.length - 1)) * chartWidth;
    return { x, label: dayjs(points[idx].timestamp).format('HH:mm') };
  });
});

const historyTableData = computed(() => {
  const list = historyPoints.value.slice(-10);
  return list.reverse();
});

const isHistoryEmpty = computed(() => historyPoints.value.length === 0);

const fetchSummary = async () => {
  loading.value = true;
  try {
    const resp = await api.get('api/v1/status', {
      headers: { Authorization: user.token },
    });
    summary.value = resp.data as StatusSummary;
  } catch (err) {
    console.error(err);
    message.error('获取状态失败');
  } finally {
    loading.value = false;
  }
};

const fetchHistory = async () => {
  historyLoading.value = true;
  try {
    const resp = await api.get('api/v1/status/history', {
      headers: { Authorization: user.token },
      params: { range: selectedRange.value },
    });
    const payload = resp.data as { points: StatusHistoryPoint[] };
    historyPoints.value = payload.points || [];
  } catch (err) {
    console.error(err);
    message.error('获取历史数据失败');
  } finally {
    historyLoading.value = false;
  }
};

const refreshAll = async () => {
  await Promise.all([fetchSummary(), fetchHistory()]);
};

watch(selectedRange, () => {
  fetchHistory();
});

onMounted(() => {
  refreshAll();
  refreshTimer.value = window.setInterval(fetchSummary, 60_000);
});

onBeforeUnmount(() => {
  if (refreshTimer.value) {
    window.clearInterval(refreshTimer.value);
  }
});
</script>

<template>
  <div class="status-page">
    <n-page-header title="服务状态监控">
      <template #subtitle>
        最近刷新：{{ lastUpdatedText }}
      </template>
      <template #extra>
        <n-space>
          <n-select v-model:value="selectedRange" size="small" :options="rangeOptions" />
          <n-button size="small" :loading="loading || historyLoading" @click="refreshAll">刷新</n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-spin :show="loading">
      <n-grid cols="1 768:2 1160:3" :x-gap="18" :y-gap="18">
        <n-grid-item v-for="card in summaryCards" :key="card.label">
          <n-card class="status-card" size="small">
            <div class="status-card__label">{{ card.label }}</div>
            <div class="status-card__value">{{ card.value }}</div>
            <div class="status-card__hint">{{ card.hint }}</div>
          </n-card>
        </n-grid-item>
      </n-grid>
      <div v-if="!summaryCards.length" class="status-empty" role="status">暂无数据，正在等待第一次采样 ...</div>
    </n-spin>

    <n-card class="status-chart-card" title="实时曲线" size="small">
      <n-spin :show="historyLoading">
        <div v-if="!isHistoryEmpty" class="chart-wrapper">
          <div v-for="series in chartSeries" :key="series.key" class="chart-wrapper__series">
            <div class="chart-series-header">
              <span class="chart-series-dot" :style="{ backgroundColor: series.color }"></span>
              <span class="chart-series-title">{{ series.label }}</span>
            </div>
            <svg :width="chartWidth" :height="chartHeight" role="img">
              <g class="chart-grid">
                <line v-for="tick in series.ticks" :key="`${series.key}-grid-${tick.label}`" x1="0"
                  :y1="chartHeight - (tick.value / series.maxValue) * chartHeight" :x2="chartWidth"
                  :y2="chartHeight - (tick.value / series.maxValue) * chartHeight" />
              </g>
              <g class="chart-polylines">
                <polyline :points="series.path" :stroke="series.color" />
              </g>
              <g class="chart-y-ticks">
                <text v-for="tick in series.ticks" :key="`${series.key}-tick-${tick.label}`" x="0"
                  :y="chartHeight - (tick.value / series.maxValue) * chartHeight - 4">
                  {{ tick.label }}
                </text>
              </g>
              <g class="chart-x-ticks">
                <text v-for="tick in chartTicksX" :key="`${series.key}-time-${tick.label}`" :x="tick.x" :y="chartHeight + 20">
                  {{ tick.label }}
                </text>
              </g>
            </svg>
          </div>
        </div>
        <div v-else class="status-empty" role="status">暂无历史数据</div>
      </n-spin>
    </n-card>

    <n-card class="status-history-card" title="历史记录" size="small">
      <n-spin :show="historyLoading">
        <n-table size="small" v-if="historyTableData.length">
          <thead>
            <tr>
              <th>采样时间</th>
              <th>并发连接</th>
              <th>在线用户</th>
              <th>消息 / 分钟</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in historyTableData" :key="item.timestamp">
              <td>{{ dayjs(item.timestamp).format('MM-DD HH:mm') }}</td>
              <td>{{ formatNumber(item.concurrentConnections) }}</td>
              <td>{{ formatNumber(item.onlineUsers) }}</td>
              <td>{{ formatNumber(item.messagesPerMinute) }}</td>
            </tr>
          </tbody>
        </n-table>
        <div v-else class="status-empty" role="status">暂无历史数据</div>
      </n-spin>
    </n-card>
  </div>
</template>

<style scoped lang="scss">
.status-page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1.25rem;
  color: var(--sc-text-primary);
  background-color: var(--sc-bg-surface);
  background-image:
    radial-gradient(1200px circle at 0% -20%, color-mix(in srgb, var(--sc-bg-elevated) 60%, transparent) 0%, transparent 55%),
    linear-gradient(180deg, color-mix(in srgb, var(--sc-bg-header, var(--sc-bg-surface)) 70%, transparent) 0%, var(--sc-bg-surface) 45%, color-mix(in srgb, var(--sc-bg-elevated) 40%, var(--sc-bg-surface) 60%) 100%);
  height: 100vh;
  box-sizing: border-box;
  overflow-y: auto;
}

.status-card {
  border-radius: 1rem;
  border: 1px solid var(--sc-border-mute);
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--sc-bg-elevated) 85%, var(--sc-bg-surface) 15%) 0%,
    color-mix(in srgb, var(--sc-bg-elevated) 55%, var(--sc-bg-surface) 45%) 100%
  );
  box-shadow: 0 18px 30px color-mix(in srgb, var(--sc-border-strong) 18%, transparent);
}

.status-card__label {
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.status-card__value {
  font-size: 1.8rem;
  font-weight: 600;
  margin-top: 0.25rem;
  color: var(--sc-text-primary);
}

.status-card__hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.status-chart-card {
  border-radius: 1rem;
  border: 1px solid var(--sc-border-mute);
  background: var(--sc-bg-elevated);
}

.chart-wrapper {
  overflow-x: auto;
  padding: 0.5rem 0.25rem 0.25rem 0;
}

.chart-wrapper__series {
  margin-bottom: 1.5rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--sc-border-mute);
}

.chart-wrapper__series:last-child {
  border-bottom: none;
  margin-bottom: 0;
  padding-bottom: 0;
}

.chart-series-header {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin-bottom: 0.35rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.chart-series-dot {
  width: 0.65rem;
  height: 0.65rem;
  border-radius: 999px;
  display: inline-block;
}

svg {
  width: 100%;
  max-width: 100%;
}

.chart-grid line {
  stroke: color-mix(in srgb, var(--sc-border-mute) 65%, transparent);
  stroke-dasharray: 4 6;
}

.chart-polylines polyline {
  fill: none;
  stroke-width: 2;
}

.chart-y-ticks text,
.chart-x-ticks text {
  font-size: 0.75rem;
  fill: var(--sc-text-secondary);
}

.status-empty {
  text-align: center;
  padding: 1.5rem 0;
  color: var(--sc-text-secondary);
}

.status-history-card {
  border-radius: 1rem;
  border: 1px solid var(--sc-border-mute);
  background: var(--sc-bg-elevated);
}

.status-history-card table {
  width: 100%;
  border-collapse: collapse;
}

.status-history-card th,
.status-history-card td {
  padding: 0.35rem 0.5rem;
  text-align: left;
  border-bottom: 1px solid var(--sc-border-mute);
}

.status-history-card thead {
  background-color: color-mix(in srgb, var(--sc-bg-elevated) 75%, var(--sc-border-mute) 25%);
}
</style>
