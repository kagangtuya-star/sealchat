<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { CSSProperties } from 'vue';
import dayjs from 'dayjs';
import { useMessage } from 'naive-ui';
import { api } from '@/stores/_config';
import { useUserStore } from '@/stores/user';
import StatusMetricPanel from './components/StatusMetricPanel.vue';
import {
  buildStatusHistoryParams,
  createMetricHistoryStore,
  formatStatusBytes,
  formatStatusNumber,
  statusFilterOptions,
  statusMetricList,
  toggleExpandedMetricKey,
  type StatusFilterMode,
  type StatusHistoryPoint,
  type StatusHistoryQuery,
  type StatusMetricKey,
} from './status-history';

interface StatusSummary {
  timestamp: number;
  concurrentConnections: number;
  wsAuthedConnections?: number;
  wsPreAuthConnections?: number;
  wsTotalConnections?: number;
  wsGuestConnections?: number;
  wsObserverConnections?: number;
  wsAuthenticatedUsers?: number;
  onlineUsers: number;
  messagesPerMinute: number;
  registeredUsers: number;
  worldCount: number;
  channelCount: number;
  privateChannelCount: number;
  messageCount: number;
  messageCountIc: number;
  messageCountOoc: number;
  messageCharCount: number;
  messageCharCountIc: number;
  messageCharCountOoc: number;
  attachmentCount: number;
  attachmentBytes: number;
  attachmentImageCount?: number;
  attachmentImageBytes?: number;
  attachmentFontCount?: number;
  attachmentFontBytes?: number;
  intervalSeconds: number;
  retentionDays: number;
}

type StatusCard = {
  label: string;
  value: string;
  hint: string;
  variant?: 'default' | 'metric';
  compactValue?: boolean;
  breakdowns?: { label: string; value: string }[];
};

const user = useUserStore();
const message = useMessage();

const summary = ref<StatusSummary | null>(null);
const historyPoints = ref<StatusHistoryPoint[]>([]);
const loading = ref(false);
const historyLoading = ref(false);
const refreshTimer = ref<number | null>(null);
const historyFilterMode = ref<StatusFilterMode>('1h');
const customRange = ref<[number, number] | null>(null);
const expandedMetricKey = ref<StatusMetricKey | null>(null);
let historyRequestSeq = 0;

const lastUpdatedText = computed(() => {
  if (!summary.value?.timestamp) {
    return '尚无数据';
  }
  return dayjs(summary.value.timestamp).format('YYYY-MM-DD HH:mm:ss');
});

const currentHistoryQuery = computed<StatusHistoryQuery | null>(() => {
  if (historyFilterMode.value === 'custom') {
    if (!customRange.value || customRange.value.length !== 2) {
      return null;
    }
    const [start, end] = customRange.value;
    if (!start || !end || start >= end) {
      return null;
    }
    return {
      mode: 'custom',
      start,
      end,
    };
  }

  return { mode: historyFilterMode.value };
});

const historyStore = createMetricHistoryStore(async (query) => {
  const resp = await api.get('api/v1/status/history', {
    headers: { Authorization: user.token },
    params: buildStatusHistoryParams(query),
  });
  const payload = resp.data as { points: StatusHistoryPoint[] };
  return { range: query.mode, points: payload.points || [] };
});

const summaryCards = computed<StatusCard[]>(() => {
  if (!summary.value) {
    return [];
  }
  const data = summary.value;
  return [
    { label: '并发连接', value: formatStatusNumber(data.concurrentConnections), hint: '采样值：已鉴权 WebSocket 连接数' },
    { label: 'WS 总连接', value: formatStatusNumber(data.wsTotalConnections), hint: '实时值：已鉴权 + 鉴权前连接' },
    { label: 'WS 已鉴权', value: formatStatusNumber(data.wsAuthedConnections), hint: '实时值：已完成 IDENTIFY 的连接' },
    { label: 'WS 鉴权前', value: formatStatusNumber(data.wsPreAuthConnections), hint: '实时值：已升级但未 IDENTIFY 的连接' },
    { label: 'Guest 连接', value: formatStatusNumber(data.wsGuestConnections), hint: '实时值：空 token 访客连接数' },
    { label: '观察者连接', value: formatStatusNumber(data.wsObserverConnections), hint: '实时值：observer 模式连接数' },
    { label: 'WS 鉴权用户', value: formatStatusNumber(data.wsAuthenticatedUsers), hint: '实时值：存在鉴权连接的唯一用户数' },
    { label: '在线用户', value: formatStatusNumber(data.onlineUsers), hint: '120 秒内仍活跃的用户' },
    { label: '消息 / 分钟', value: formatStatusNumber(data.messagesPerMinute), hint: '最近一分钟的消息吞吐' },
    { label: '注册用户', value: formatStatusNumber(data.registeredUsers), hint: '未被禁用的账户数量' },
    { label: '世界总数', value: formatStatusNumber(data.worldCount), hint: '状态正常的世界' },
    { label: '公共频道', value: formatStatusNumber(data.channelCount), hint: '状态正常的公共频道' },
    { label: '私聊频道', value: formatStatusNumber(data.privateChannelCount), hint: '状态正常的私聊频道' },
    {
      label: '消息总数',
      value: formatStatusNumber(data.messageCount),
      hint: '未被删除的历史消息',
      variant: 'metric',
      breakdowns: [
        { label: '内', value: formatStatusNumber(data.messageCountIc) },
        { label: '外', value: formatStatusNumber(data.messageCountOoc) },
      ],
    },
    {
      label: '消息总字数',
      value: formatStatusNumber(data.messageCharCount),
      hint: '未被删除历史消息的字符总数',
      variant: 'metric',
      compactValue: true,
      breakdowns: [
        { label: '内', value: formatStatusNumber(data.messageCharCountIc) },
        { label: '外', value: formatStatusNumber(data.messageCharCountOoc) },
      ],
    },
    {
      label: '附件数量',
      value: formatStatusNumber(data.attachmentCount),
      hint: '正式图片附件与平台字体资源数量',
      variant: 'metric',
      breakdowns: [
        { label: '图', value: formatStatusNumber(data.attachmentImageCount) },
        { label: '字', value: formatStatusNumber(data.attachmentFontCount) },
      ],
    },
    {
      label: '附件总大小',
      value: formatStatusBytes(data.attachmentBytes),
      hint: '正式图片附件与平台字体资源大小',
      variant: 'metric',
      compactValue: true,
      breakdowns: [
        { label: '图', value: formatStatusBytes(data.attachmentImageBytes) },
        { label: '字', value: formatStatusBytes(data.attachmentFontBytes) },
      ],
    },
  ];
});

const metricPanels = computed(() => statusMetricList.map((metric) => ({
  metric,
  points: historyPoints.value,
  loading: historyLoading.value,
})));

const expandedPanel = computed(() => {
  if (!expandedMetricKey.value) {
    return null;
  }
  return metricPanels.value.find((panel) => panel.metric.key === expandedMetricKey.value) || null;
});

const customRangeText = computed(() => {
  if (!customRange.value?.length) {
    return '未设置';
  }
  return `${dayjs(customRange.value[0]).format('YYYY-MM-DD HH:mm')} ~ ${dayjs(customRange.value[1]).format('YYYY-MM-DD HH:mm')}`;
});

const expandedOverlayPanelStyle: CSSProperties = {
  position: 'fixed',
  inset: '0',
  width: '100vw',
  maxWidth: '100vw',
  height: '100vh',
  minHeight: '100vh',
};

const expandedOverlayStyle: CSSProperties = {
  position: 'fixed',
  inset: '0',
  zIndex: '5000',
  display: 'flex',
  alignItems: 'stretch',
  justifyContent: 'stretch',
  padding: '0',
  overflowY: 'auto',
};

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

const loadHistory = async (forceRefresh = false) => {
  const query = currentHistoryQuery.value;
  if (!query) {
    historyPoints.value = [];
    return;
  }
  const requestId = ++historyRequestSeq;
  historyLoading.value = true;
  try {
    if (forceRefresh) {
      historyStore.clearQuery(query);
    }
    const payload = await historyStore.getQueryData(query);
    if (requestId !== historyRequestSeq) {
      return;
    }
    historyPoints.value = payload.points || [];
  } catch (err) {
    if (requestId === historyRequestSeq) {
      console.error(err);
      message.error('获取历史数据失败');
    }
  } finally {
    if (requestId === historyRequestSeq) {
      historyLoading.value = false;
    }
  }
};

const refreshAll = async () => {
  historyStore.clearAll();
  await Promise.allSettled([fetchSummary(), loadHistory(true)]);
};

const closeExpandedPanel = () => {
  expandedMetricKey.value = null;
};

const handleExpandedPanelKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && expandedMetricKey.value) {
    closeExpandedPanel();
  }
};

watch(expandedMetricKey, (value) => {
  document.body.classList.toggle('status-history-overlay-open', Boolean(value));
});

onMounted(() => {
  void refreshAll();
  refreshTimer.value = window.setInterval(fetchSummary, 60_000);
  window.addEventListener('keydown', handleExpandedPanelKeydown);
});

watch(currentHistoryQuery, (query, prevQuery) => {
  if (!query) {
    historyPoints.value = [];
    historyLoading.value = false;
    return;
  }
  const prevKey = prevQuery ? JSON.stringify(prevQuery) : '';
  const nextKey = JSON.stringify(query);
  if (prevKey === nextKey) {
    return;
  }
  void loadHistory();
}, { deep: true });

onBeforeUnmount(() => {
  if (refreshTimer.value) {
    window.clearInterval(refreshTimer.value);
  }
  document.body.classList.remove('status-history-overlay-open');
  window.removeEventListener('keydown', handleExpandedPanelKeydown);
});
</script>

<template>
  <div class="status-page">
    <n-page-header title="服务状态监控">
      <template #subtitle>
        最近刷新：{{ lastUpdatedText }}
      </template>
      <template #extra>
        <n-space align="center">
          <n-button size="small" secondary tag="a" href="#/status/perf">性能检测</n-button>
          <n-button size="small" :loading="loading || historyLoading" @click="refreshAll">刷新</n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-spin :show="loading">
      <n-grid cols="1 768:2 1160:3" :x-gap="18" :y-gap="18">
        <n-grid-item v-for="card in summaryCards" :key="card.label">
          <n-card
            class="status-card"
            :class="{
              'status-card--metric': card.variant === 'metric',
              'status-card--metric-compact': card.compactValue,
            }"
            size="small"
          >
            <div v-if="card.variant === 'metric'" class="status-card__metric">
              <div class="status-card__label">{{ card.label }}</div>
              <div
                class="status-card__value status-card__value--metric"
                :class="{ 'status-card__value--compact': card.compactValue }"
              >
                {{ card.value }}
              </div>
              <div class="status-card__metric-footer">
                <div class="status-card__hint status-card__hint--metric">{{ card.hint }}</div>
                <div v-if="card.breakdowns?.length" class="status-card__mini-stats" aria-label="场内场外拆分">
                  <div
                    v-for="item in card.breakdowns"
                    :key="`${card.label}-${item.label}`"
                    class="status-card__mini-stat"
                  >
                    <span class="status-card__mini-stat-label">{{ item.label }}</span>
                    <span class="status-card__mini-stat-value">{{ item.value }}</span>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="status-card__main">
              <div class="status-card__label">{{ card.label }}</div>
              <div class="status-card__value">{{ card.value }}</div>
              <div class="status-card__hint">{{ card.hint }}</div>
            </div>
          </n-card>
        </n-grid-item>
      </n-grid>
      <div v-if="!summaryCards.length" class="status-empty" role="status">暂无数据，正在等待第一次采样 ...</div>
    </n-spin>

    <section class="status-history-section">
      <div class="status-history-section__header">
        <div class="status-history-section__copy">
          <h2>历史指标曲线</h2>
          <p>点击标题右侧的 + 可放大查看。</p>
          <p class="status-history-section__link-line">
            需要排查 CPU / 内存占用时，可前往
            <a href="#/status/perf">性能检测页</a>
            查看轻量采样、pprof 快照和连续 CPU 录制。
          </p>
          <p v-if="historyFilterMode === 'custom'" class="status-history-section__custom-label">当前自定义区间：{{ customRangeText }}</p>
        </div>
        <div class="status-history-section__controls">
          <n-select
            v-model:value="historyFilterMode"
            size="small"
            :options="statusFilterOptions"
            class="status-history-section__select"
          />
          <n-date-picker
            v-if="historyFilterMode === 'custom'"
            v-model:value="customRange"
            type="datetimerange"
            clearable
            size="small"
            class="status-history-section__picker"
          />
        </div>
      </div>

      <transition-group name="status-history" tag="div" class="status-history-section__grid">
        <div
          v-for="panel in metricPanels"
          :key="panel.metric.key"
          class="status-history-card-item"
        >
          <StatusMetricPanel
            :metric="panel.metric"
            :points="panel.points"
            :loading="panel.loading"
            @toggle-expand="expandedMetricKey = toggleExpandedMetricKey(expandedMetricKey, panel.metric.key)"
          />
        </div>
      </transition-group>
    </section>

    <teleport to="body">
      <transition name="status-overlay">
        <div
          v-if="expandedPanel"
          class="status-history-overlay"
          :style="expandedOverlayStyle"
          @click.self="closeExpandedPanel"
        >
          <div class="status-history-overlay__panel" :style="expandedOverlayPanelStyle">
            <StatusMetricPanel
              :key="`${expandedPanel.metric.key}-overlay`"
              :metric="expandedPanel.metric"
              :points="expandedPanel.points"
              :loading="expandedPanel.loading"
              :expanded="true"
              :overlay="true"
              @toggle-expand="closeExpandedPanel"
            />
          </div>
        </div>
      </transition>
    </teleport>
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

.status-page__hint {
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.status-page a {
  color: #2563eb;
}

.status-page a:hover {
  text-decoration: underline;
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

.status-card :deep(.n-card__content) {
  height: 100%;
}

.status-card__main {
  min-width: 0;
}

.status-card__metric {
  display: flex;
  flex-direction: column;
  min-height: 9.75rem;
  gap: 0.65rem;
}

.status-card--metric {
  border-color: color-mix(in srgb, var(--sc-border-mute) 82%, transparent);
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--sc-bg-elevated) 82%, var(--sc-bg-surface) 18%) 0%,
      color-mix(in srgb, var(--sc-bg-elevated) 64%, var(--sc-bg-surface) 36%) 100%
    );
  box-shadow:
    inset 0 1px 0 color-mix(in srgb, var(--sc-text-primary) 7%, transparent),
    0 10px 24px color-mix(in srgb, var(--sc-border-strong) 10%, transparent);
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

.status-history-section__link-line {
  margin-top: 0.2rem;
  color: var(--sc-text-secondary);
}

.status-card__value--metric {
  margin-top: 0.1rem;
  font-size: 1.8rem;
  line-height: 1.02;
  letter-spacing: -0.03em;
  font-variant-numeric: tabular-nums lining-nums;
  white-space: nowrap;
}

.status-card__value--compact {
  font-size: 1.68rem;
  letter-spacing: -0.045em;
}

.status-card__metric-footer {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 0.9rem 1.2rem;
  margin-top: auto;
  padding-top: 0.9rem;
  border-top: 1px solid color-mix(in srgb, var(--sc-border-mute) 76%, transparent);
}

.status-card__hint--metric {
  max-width: 14rem;
  line-height: 1.45;
}

.status-card__mini-stats {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.45rem 0.6rem;
}

.status-card__mini-stat {
  display: inline-flex;
  align-items: center;
  gap: 0.42rem;
  min-height: 1.9rem;
  padding: 0.18rem 0.58rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--sc-border-strong) 34%, transparent);
  background: color-mix(in srgb, var(--sc-bg-surface) 34%, transparent);
  box-shadow: inset 0 1px 0 color-mix(in srgb, var(--sc-text-primary) 5%, transparent);
  font-variant-numeric: tabular-nums lining-nums;
}

.status-card__mini-stat-label {
  font-size: 0.72rem;
  font-weight: 600;
  color: color-mix(in srgb, var(--sc-text-secondary) 88%, transparent);
}

.status-card__mini-stat-value {
  font-size: 0.82rem;
  font-weight: 600;
  color: color-mix(in srgb, var(--sc-text-primary) 84%, var(--sc-text-secondary) 16%);
}

.status-history-section {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
}

.status-history-section__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.status-history-section__copy {
  min-width: 0;
}

.status-history-section__header h2 {
  margin: 0;
  font-size: 1.02rem;
}

.status-history-section__header p {
  margin: 0.2rem 0 0;
  color: var(--sc-text-secondary);
  font-size: 0.82rem;
}

.status-history-section__custom-label {
  font-variant-numeric: tabular-nums;
}

.status-history-section__controls {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.status-history-section__select {
  width: 132px;
}

.status-history-section__picker {
  width: 360px;
  max-width: 100%;
}

.status-history-section__grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 1rem;
  width: 100%;
}

.status-history-card-item {
  min-width: 0;
}

.status-history-move,
.status-history-enter-active,
.status-history-leave-active {
  transition:
    transform 0.38s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.28s ease;
}

.status-history-enter-from,
.status-history-leave-to {
  opacity: 0;
}

.status-overlay-enter-active,
.status-overlay-leave-active {
  transition:
    opacity 0.24s ease,
    transform 0.34s cubic-bezier(0.22, 1, 0.36, 1);
}

.status-overlay-enter-from,
.status-overlay-leave-to {
  opacity: 0;
}

.status-overlay-enter-active .status-history-overlay__panel,
.status-overlay-leave-active .status-history-overlay__panel {
  transition:
    transform 0.34s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.24s ease;
}

.status-overlay-enter-from .status-history-overlay__panel,
.status-overlay-leave-to .status-history-overlay__panel {
  opacity: 0;
  transform: translateY(18px) scale(0.972);
}

.status-empty {
  text-align: center;
  padding: 1.5rem 0;
  color: var(--sc-text-secondary);
}

.status-history-overlay {
  background: color-mix(in srgb, var(--sc-bg-surface) 52%, rgba(2, 6, 23, 0.78));
  backdrop-filter: blur(10px);
}

.status-history-overlay__panel {
  display: flex;
  align-items: stretch;
  justify-content: stretch;
  min-width: 0;
  min-height: 100vh;
}

:global(body.status-history-overlay-open) {
  overflow: hidden;
}

@media (max-width: 1320px) {
  .status-history-section__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .status-card__metric-footer {
    flex-direction: column;
    align-items: flex-start;
  }

  .status-card__hint--metric {
    max-width: none;
  }

  .status-card__mini-stats {
    justify-content: flex-start;
  }

  .status-history-section__header {
    flex-direction: column;
  }

  .status-history-section__controls {
    width: 100%;
    justify-content: stretch;
  }

  .status-history-section__select,
  .status-history-section__picker {
    width: 100%;
  }

  .status-history-section__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
