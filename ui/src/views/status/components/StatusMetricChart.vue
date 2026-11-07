<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import dayjs from 'dayjs';
import { useDisplayStore } from '@/stores/display';
import { useECharts } from '@/composables/useECharts';
import * as echarts from 'echarts/core';
import { LineChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, DataZoomComponent, LegendComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { StatusHistoryPoint, StatusMetricDefinition } from '../status-history';

echarts.use([LineChart, GridComponent, TooltipComponent, DataZoomComponent, LegendComponent, CanvasRenderer]);

const props = defineProps<{
  metric: StatusMetricDefinition;
  points: StatusHistoryPoint[];
  expanded?: boolean;
  overlay?: boolean;
}>();

const display = useDisplayStore();
const chartRef = ref<HTMLDivElement | null>(null);
const resizeObservers: ResizeObserver[] = [];
let resizeTimers: number[] = [];

const isDark = computed(() => {
  if (display.settings.customThemeEnabled && display.settings.activeCustomThemeId) {
    return display.palette === 'night';
  }
  return display.palette === 'night';
});

const chart = useECharts(chartRef, isDark, echarts);

const getCSSVar = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

const chartColors = computed(() => ({
  textColor: getCSSVar('--sc-text-secondary') || (isDark.value ? '#b1b6c6' : '#667085'),
  splitLineColor: isDark.value ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.08)',
  areaEndColor: isDark.value ? 'rgba(255,255,255,0.02)' : 'rgba(255,255,255,0.01)',
}));

const overlayChartStyle = computed(() => (props.overlay ? {
  minHeight: 'calc(100vh - 112px)',
  height: 'calc(100vh - 112px)',
} : undefined));

const timeLabelFormat = computed(() => {
  const first = props.points[0]?.timestamp || 0;
  const last = props.points[props.points.length - 1]?.timestamp || 0;
  return last - first > 24 * 60 * 60 * 1000 ? 'MM-DD HH:mm' : 'HH:mm';
});

const chartOption = computed(() => {
  if (!props.points.length) {
    return {
      title: {
        text: '暂无历史数据',
        left: 'center',
        top: 'center',
        textStyle: {
          color: chartColors.value.textColor,
          fontSize: 14,
          fontWeight: 500,
        },
      },
      xAxis: { show: false },
      yAxis: { show: false },
      series: [],
    };
  }

  const labels = props.points.map((point) => dayjs(point.timestamp).format(timeLabelFormat.value));
  const seriesDefs = props.metric.series?.length
    ? props.metric.series
    : [{ key: props.metric.key, label: props.metric.label, color: props.metric.color }];
  const seriesColors = seriesDefs.map((item) => item.color);

  return {
    backgroundColor: 'transparent',
    color: seriesColors,
    legend: seriesDefs.length > 1 ? {
      top: 0,
      right: '3%',
      textStyle: {
        color: chartColors.value.textColor,
        fontSize: 11,
      },
      itemWidth: 16,
      itemHeight: 8,
    } : undefined,
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'line',
        lineStyle: {
          color: props.metric.color,
          opacity: 0.35,
        },
      },
      valueFormatter: (value: number) => props.metric.format(Number(value || 0)),
    },
    grid: {
      left: '3%',
      right: '3%',
      top: props.metric.relativeToStart ? 58 : seriesDefs.length > 1 ? 34 : 18,
      bottom: props.points.length > 24 ? 72 : 28,
      containLabel: true,
    },
    dataZoom: props.points.length > 24 ? [
      { type: 'inside', start: 0, end: 100 },
      { type: 'slider', start: 0, end: 100, height: 18, bottom: 8, brushSelect: false },
    ] : [],
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: labels,
      axisLabel: {
        color: chartColors.value.textColor,
        hideOverlap: true,
        fontSize: 11,
      },
      axisLine: {
        lineStyle: {
          color: chartColors.value.splitLineColor,
        },
      },
    },
    yAxis: {
      type: 'value',
      name: props.metric.relativeToStart ? '相对变化（%）' : undefined,
      nameTextStyle: { color: chartColors.value.textColor },
      scale: Boolean(props.metric.scale),
      axisLabel: {
        color: chartColors.value.textColor,
        formatter: (value: number) => props.metric.relativeToStart
          ? `${value.toLocaleString('zh-CN', { maximumSignificantDigits: 3 })}%`
          : props.metric.format(value),
      },
      splitLine: {
        lineStyle: {
          color: chartColors.value.splitLineColor,
        },
      },
    },
    series: seriesDefs.map((item) => {
      const baselineIndex = props.metric.relativeToStart
        ? props.points.findIndex((point) => Number.isFinite(point[item.key]) && Number(point[item.key]) > 0)
        : -1;
      const baseline = props.points[baselineIndex]?.[item.key] ?? 0;
      return {
        name: item.label,
        type: 'line',
        smooth: !props.metric.relativeToStart,
        showSymbol: false,
        data: props.points.map((point, index) => {
          if (!props.metric.relativeToStart) {
            return point[item.key] || 0;
          }
          const value = point[item.key];
          if (typeof value !== 'number' || !Number.isFinite(value)) {
            return null;
          }
          if (baseline === 0) {
            return value === 0 ? 0 : null;
          }
          return index < baselineIndex ? null : (value - baseline) / baseline * 100;
        }),
        tooltip: props.metric.relativeToStart ? {
          valueFormatter: (_value: unknown, dataIndex: number) => props.metric.format(Number(props.points[dataIndex]?.[item.key] || 0)),
        } : undefined,
        lineStyle: {
          color: item.color,
          width: 2,
        },
        itemStyle: {
          color: item.color,
        },
        areaStyle: props.metric.relativeToStart ? undefined : {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: `${item.color}44` },
            { offset: 1, color: chartColors.value.areaEndColor },
          ]),
        },
      };
    }),
  };
});

const renderChart = () => {
  const instance = chart.ensureInstance();
  if (!instance) {
    return;
  }
  instance.setOption(chartOption.value, true);
};

const clearResizeTimers = () => {
  resizeTimers.forEach((timer) => window.clearTimeout(timer));
  resizeTimers = [];
};

const scheduleResize = () => {
  clearResizeTimers();
  chart.resize();
  void nextTick(() => {
    chart.resize();
    requestAnimationFrame(() => chart.resize());
  });
  [120, 260, 420].forEach((delay) => {
    const timer = window.setTimeout(() => {
      chart.resize();
    }, delay);
    resizeTimers.push(timer);
  });
};

onMounted(() => {
  void nextTick(renderChart);

  if (typeof ResizeObserver !== 'undefined' && chartRef.value) {
    const targets = [chartRef.value, chartRef.value.parentElement, chartRef.value.closest('.status-metric-panel')].filter(Boolean) as Element[];
    targets.forEach((target) => {
      const observer = new ResizeObserver(() => {
        scheduleResize();
      });
      observer.observe(target);
      resizeObservers.push(observer);
    });
  }

  scheduleResize();
});

watch(chartOption, () => {
  void nextTick(renderChart);
}, { deep: true });

watch(isDark, () => {
  void nextTick(renderChart);
});

watch(() => props.expanded, () => {
  scheduleResize();
});

onBeforeUnmount(() => {
  resizeObservers.forEach((observer) => observer.disconnect());
  resizeObservers.length = 0;
  clearResizeTimers();
});
</script>

<template>
  <div
    ref="chartRef"
    class="status-metric-chart"
    :class="{
      'status-metric-chart--expanded': expanded,
      'status-metric-chart--overlay': overlay,
    }"
    :style="overlayChartStyle"
  ></div>
</template>

<style scoped lang="scss">
.status-metric-chart {
  width: 100%;
  min-height: 248px;
  height: 248px;
  transition: height 0.34s cubic-bezier(0.22, 1, 0.36, 1);
}

.status-metric-chart--expanded {
  min-height: 340px;
  height: 340px;
}

.status-metric-chart--overlay {
  min-height: calc(100vh - 112px);
  height: calc(100vh - 112px);
}
</style>
