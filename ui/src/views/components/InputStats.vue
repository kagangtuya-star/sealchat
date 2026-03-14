<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/stores/_config'
import { useUserStore } from '@/stores/user'
import { useDisplayStore } from '@/stores/display'
import * as echarts from 'echarts/core'
import { LineChart, BarChart, ScatterChart } from 'echarts/charts'
import {
  TitleComponent, TooltipComponent, GridComponent,
  LegendComponent, DataZoomComponent
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  LineChart, BarChart, ScatterChart, TitleComponent, TooltipComponent,
  GridComponent, LegendComponent, DataZoomComponent, CanvasRenderer
])

const { t } = useI18n()
const user = useUserStore()
const display = useDisplayStore()
const emit = defineEmits<{ (e: 'close'): void }>()

const props = defineProps<{
  currentWorldId?: string
}>()

// === Theme ===
const isDark = computed(() => {
  if (display.settings.customThemeEnabled && display.settings.activeCustomThemeId) {
    // Custom theme active – read CSS variable to determine brightness
    return display.settings.palette === 'night'
  }
  return display.settings.palette === 'night'
})

// === State ===
const loading = ref(false)
const activeTab = ref<'all' | 'ic' | 'ooc'>('all')
const timeRange = ref<'all' | '7d' | '30d' | 'custom'>('30d')
const customStart = ref<number | null>(null)
const customEnd = ref<number | null>(null)

// World/channel filter
const filterMode = ref<'none' | 'include' | 'exclude'>('none')
const selectedWorldIds = ref<string[]>([])
const selectedChannelIds = ref<string[]>([])
const filterChannelMode = ref<'none' | 'include' | 'exclude'>('none')
const includeImported = ref(false)

// Session threshold
const sessionThreshold = ref(30)

// Session chart metric toggle: 'chars' or 'speed'
const sessionMetric = ref<'chars' | 'speed'>('chars')

// Session IC/OOC mode (independent of global activeTab)
const sessionIcMode = ref<'all' | 'ic' | 'ooc'>('all')

// === localStorage persistence ===
const PREFS_KEY = 'sc-input-stats-prefs'

function savePrefs() {
  try {
    const prefs = {
      sessionThreshold: sessionThreshold.value,
      sessionMetric: sessionMetric.value,
      sessionIcMode: sessionIcMode.value,
      filterChannelMode: filterChannelMode.value,
      selectedChannelIds: selectedChannelIds.value,
      includeImported: includeImported.value,
    }
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs))
  } catch { /* ignore */ }
}

function loadPrefs() {
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    if (!raw) return
    const prefs = JSON.parse(raw)
    if (prefs.sessionThreshold != null) sessionThreshold.value = prefs.sessionThreshold
    if (prefs.sessionMetric) sessionMetric.value = prefs.sessionMetric
    if (prefs.sessionIcMode) sessionIcMode.value = prefs.sessionIcMode
    if (prefs.filterChannelMode) filterChannelMode.value = prefs.filterChannelMode
    if (prefs.selectedChannelIds) selectedChannelIds.value = prefs.selectedChannelIds
    if (prefs.includeImported != null) includeImported.value = !!prefs.includeImported
  } catch { /* ignore */ }
}

// Hide zero-value axis toggle
const hideZeroAxis = ref(false)

// Data
const overview = ref<any>(null)
const worldStats = ref<any[]>([])
const channelStats = ref<Record<string, any[]>>({})
const timelineData = ref<any[]>([])
const sessionMessages = ref<any[]>([])
const expandedWorldId = ref<string | null>(null)

// Chart
const chartRef = ref<HTMLDivElement | null>(null)
let chartInstance: echarts.ECharts | null = null

// Session chart
const sessionChartRef = ref<HTMLDivElement | null>(null)
let sessionChartInstance: echarts.ECharts | null = null
let fetchAllRequestSeq = 0
let sessionDataRequestSeq = 0
let channelRequestSeq = 0

// Only show session analysis when a world is filtered
const isWorldFiltered = computed(() =>
  filterMode.value === 'include' && selectedWorldIds.value.length > 0
)

// === Computed ===
const icMode = computed(() => {
  if (activeTab.value === 'ic') return 'ic'
  if (activeTab.value === 'ooc') return 'ooc'
  return ''
})

const queryParams = computed(() => {
  const p: Record<string, string> = {}
  if (icMode.value) p.icMode = icMode.value
  if (includeImported.value) p.includeImported = 'true'

  if (timeRange.value === '7d') {
    p.start = String(Date.now() - 7 * 24 * 60 * 60 * 1000)
  } else if (timeRange.value === '30d') {
    p.start = String(Date.now() - 30 * 24 * 60 * 60 * 1000)
  } else if (timeRange.value === 'custom') {
    if (customStart.value) p.start = String(customStart.value)
    if (customEnd.value) p.end = String(customEnd.value)
  }

  if (filterMode.value === 'include' && selectedWorldIds.value.length > 0) {
    p.includeWorlds = selectedWorldIds.value.join(',')
  }
  if (filterMode.value === 'exclude' && selectedWorldIds.value.length > 0) {
    p.excludeWorlds = selectedWorldIds.value.join(',')
  }
  if (filterChannelMode.value === 'include' && selectedChannelIds.value.length > 0) {
    p.includeChannels = selectedChannelIds.value.join(',')
  }
  if (filterChannelMode.value === 'exclude' && selectedChannelIds.value.length > 0) {
    p.excludeChannels = selectedChannelIds.value.join(',')
  }

  return p
})

// === Session analysis (computed on frontend) ===
const sessions = computed(() => {
  const msgs = sessionMessages.value
  if (!msgs || msgs.length === 0) return []

  const thresholdMs = sessionThreshold.value * 60 * 1000
  const result: Array<{
    index: number
    startTime: string
    endTime: string
    duration: number
    totalChars: number
    totalMessages: number
    typingSpeed: number
  }> = []

  let sessionStart = 0
  let sessionChars = 0
  let sessionMsgs = 0

  for (let i = 0; i < msgs.length; i++) {
    const curTime = new Date(msgs[i].createdAt).getTime()

    if (i === 0) {
      sessionStart = curTime
      sessionChars = msgs[i].charCount || 0
      sessionMsgs = 1
      continue
    }

    const prevTime = new Date(msgs[i - 1].createdAt).getTime()
    const gap = curTime - prevTime

    if (gap > thresholdMs) {
      // End previous session
      const endTime = prevTime
      const durationMin = (endTime - sessionStart) / 60000
      result.push({
        index: result.length + 1,
        startTime: new Date(sessionStart).toLocaleString(),
        endTime: new Date(endTime).toLocaleString(),
        duration: Math.round(durationMin * 10) / 10,
        totalChars: sessionChars,
        totalMessages: sessionMsgs,
        typingSpeed: durationMin > 0 ? Math.round(sessionChars / durationMin * 10) / 10 : 0,
      })
      // Start new session
      sessionStart = curTime
      sessionChars = msgs[i].charCount || 0
      sessionMsgs = 1
    } else {
      sessionChars += msgs[i].charCount || 0
      sessionMsgs++
    }
  }

  // last session
  if (sessionMsgs > 0) {
    const endTime = new Date(msgs[msgs.length - 1].createdAt).getTime()
    const durationMin = (endTime - sessionStart) / 60000
    result.push({
      index: result.length + 1,
      startTime: new Date(sessionStart).toLocaleString(),
      endTime: new Date(endTime).toLocaleString(),
      duration: Math.round(durationMin * 10) / 10,
      totalChars: sessionChars,
      totalMessages: sessionMsgs,
      typingSpeed: durationMin > 0 ? Math.round(sessionChars / durationMin * 10) / 10 : 0,
    })
  }

  // Only keep sessions with at least 3 messages
  // Only keep sessions with at least 3 messages, then re-index
  const filtered = result.filter(s => s.totalMessages >= 3)
  filtered.forEach((s, i) => { s.index = i + 1 })
  return filtered
})

// === API calls ===
async function fetchAll() {
  const requestId = ++fetchAllRequestSeq
  const sessionRequestId = ++sessionDataRequestSeq
  loading.value = true
  const prevExpanded = expandedWorldId.value
  channelStats.value = {}
  try {
    const headers = { Authorization: user.token }
    const params = queryParams.value

    // Build session-specific params with its own icMode
    const sessionParams = { ...params }
    delete sessionParams.icMode
    if (sessionIcMode.value === 'ic') sessionParams.icMode = 'ic'
    else if (sessionIcMode.value === 'ooc') sessionParams.icMode = 'ooc'

    const [ovRes, wRes, tlRes, ssRes] = await Promise.all([
      api.get('api/v1/user/input-stats/overview', { headers, params }),
      api.get('api/v1/user/input-stats/by-world', { headers, params }),
      api.get('api/v1/user/input-stats/timeline', { headers, params }),
      api.get('api/v1/user/input-stats/sessions', { headers, params: sessionParams }),
    ])

    if (requestId !== fetchAllRequestSeq) {
      return
    }

    overview.value = ovRes.data
    worldStats.value = wRes.data || []
    timelineData.value = tlRes.data || []
    if (sessionRequestId === sessionDataRequestSeq) {
      sessionMessages.value = ssRes.data || []
    }

    // Preserve expanded channel list — re-fetch if a world was expanded
    if (prevExpanded) {
      try {
        const channelReqId = ++channelRequestSeq
        const chRes = await api.get('api/v1/user/input-stats/by-channel', {
          headers, params: { ...params, worldId: prevExpanded }
        })
        if (requestId !== fetchAllRequestSeq || channelReqId !== channelRequestSeq) {
          return
        }
        channelStats.value = { ...channelStats.value, [prevExpanded]: chRes.data || [] }
      } catch { /* keep old */ }
    }

    // If opened with currentWorldId and first fetch, auto-select the world
    if (props.currentWorldId && filterMode.value === 'include' && selectedWorldIds.value.length === 1) {
      const exists = worldStats.value.some((w: any) => w.worldId === props.currentWorldId)
      if (!exists && worldStats.value.length > 0) {
        filterMode.value = 'none'
        selectedWorldIds.value = []
      }
    }

    await nextTick()
    if (requestId !== fetchAllRequestSeq) {
      return
    }
    renderChart()
    renderSessionChart()
  } catch (err) {
    if (requestId === fetchAllRequestSeq) {
      console.error('fetch input stats failed', err)
    }
  } finally {
    if (requestId === fetchAllRequestSeq) {
      loading.value = false
    }
  }
}

// Fetch only session messages (for session IC/OOC changes)
async function fetchSessionMessages() {
  const requestId = ++sessionDataRequestSeq
  try {
    const headers = { Authorization: user.token }
    const params = { ...queryParams.value }
    delete params.icMode
    if (sessionIcMode.value === 'ic') params.icMode = 'ic'
    else if (sessionIcMode.value === 'ooc') params.icMode = 'ooc'

    const res = await api.get('api/v1/user/input-stats/sessions', { headers, params })
    if (requestId !== sessionDataRequestSeq) {
      return
    }
    sessionMessages.value = res.data || []
    await nextTick()
    if (requestId !== sessionDataRequestSeq) {
      return
    }
    renderSessionChart()
  } catch (err) {
    if (requestId === sessionDataRequestSeq) {
      console.error('fetch session messages failed', err)
    }
  }
}

async function fetchChannels(worldId: string) {
  if (channelStats.value[worldId]) {
    expandedWorldId.value = expandedWorldId.value === worldId ? null : worldId
    return
  }

  try {
    const requestId = ++channelRequestSeq
    const headers = { Authorization: user.token }
    const params = { ...queryParams.value, worldId }
    const res = await api.get('api/v1/user/input-stats/by-channel', { headers, params })
    if (requestId !== channelRequestSeq) {
      return
    }
    channelStats.value = { ...channelStats.value, [worldId]: res.data || [] }
    expandedWorldId.value = worldId
  } catch (err) {
    console.error('fetch channel stats failed', err)
  }
}

// === Chart helpers ===
function getCSSVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function getChartColors() {
  const dark = isDark.value
  return {
    textColor: getCSSVar('--sc-text-secondary') || (dark ? '#aaa' : '#666'),
    legendColor: getCSSVar('--sc-text-secondary') || (dark ? '#ccc' : '#333'),
    splitLineColor: dark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)',
    noDataColor: getCSSVar('--sc-text-secondary') || '#888',
  }
}

// === Chart ===
function ensureChartInstance() {
  if (!chartRef.value) return
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
  chartInstance = echarts.init(chartRef.value, isDark.value ? 'dark' : undefined)
}

function renderChart() {
  if (!chartRef.value) return
  ensureChartInstance()
  if (!chartInstance) return

  const colors = getChartColors()
  const rawData = timelineData.value
  if (!rawData || rawData.length === 0) {
    chartInstance.setOption({
      title: { text: t('inputStats.noData'), left: 'center', top: 'center', textStyle: { color: colors.noDataColor, fontSize: 14 } },
      xAxis: { show: false },
      yAxis: { show: false },
      series: [],
    })
    return
  }

  // Filter zero-value data points if hideZeroAxis is enabled
  const data = hideZeroAxis.value
    ? rawData.filter((d: any) => (d.totalChars || 0) > 0 || (d.totalMessages || 0) > 0)
    : rawData

  if (data.length === 0) {
    chartInstance.setOption({
      title: { text: t('inputStats.noData'), left: 'center', top: 'center', textStyle: { color: colors.noDataColor, fontSize: 14 } },
      xAxis: { show: false },
      yAxis: { show: false },
      series: [],
    })
    return
  }

  const dates = data.map((d: any) => d.date)
  const chars = data.map((d: any) => d.totalChars)
  const msgs = data.map((d: any) => d.totalMessages)

  chartInstance.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'cross' },
    },
    legend: {
      data: [t('inputStats.totalChars'), t('inputStats.totalMessages')],
      top: 0,
      textStyle: { color: colors.legendColor },
    },
    grid: {
      left: '3%', right: '4%', top: 36, bottom: 78, containLabel: true,
    },
    dataZoom: [
      { type: 'inside', start: 0, end: 100 },
      { type: 'slider', start: 0, end: 100, height: 18, bottom: 10, brushSelect: false },
    ],
    xAxis: {
      type: 'category',
      data: dates,
      axisLabel: { rotate: 30, margin: 14, hideOverlap: true, color: colors.textColor, fontSize: 10 },
    },
    yAxis: [
      {
        type: 'value',
        name: t('inputStats.chars'),
        axisLabel: { color: colors.textColor },
        splitLine: { lineStyle: { color: colors.splitLineColor } },
      },
      {
        type: 'value',
        name: t('inputStats.messages'),
        axisLabel: { color: colors.textColor },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: t('inputStats.totalChars'),
        type: 'line',
        smooth: true,
        data: chars,
        yAxisIndex: 0,
        itemStyle: { color: '#5b8ff9' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(91,143,249,0.35)' },
            { offset: 1, color: 'rgba(91,143,249,0.05)' },
          ]),
        },
      },
      {
        name: t('inputStats.totalMessages'),
        type: 'bar',
        data: msgs,
        yAxisIndex: 1,
        barMaxWidth: 16,
        itemStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(93,212,166,0.8)' },
            { offset: 1, color: 'rgba(93,212,166,0.2)' },
          ]),
          borderRadius: [4, 4, 0, 0],
        },
      },
    ],
  }, true)
}

// === Session chart ===
function ensureSessionChartInstance() {
  if (!sessionChartRef.value) return
  if (sessionChartInstance) {
    sessionChartInstance.dispose()
    sessionChartInstance = null
  }
  sessionChartInstance = echarts.init(sessionChartRef.value, isDark.value ? 'dark' : undefined)
}

function renderSessionChart() {
  if (!sessionChartRef.value || !isWorldFiltered.value) return
  ensureSessionChartInstance()
  if (!sessionChartInstance) return

  const colors = getChartColors()
  const data = sessions.value

  if (!data || data.length === 0) {
    sessionChartInstance.setOption({
      title: { text: t('inputStats.noData'), left: 'center', top: 'center', textStyle: { color: colors.noDataColor, fontSize: 14 } },
      xAxis: { show: false },
      yAxis: { show: false },
      series: [],
    })
    return
  }

  const isChars = sessionMetric.value === 'chars'
  const seriesColor = isChars ? '#5b8ff9' : '#5dd4a6'
  const gradientStart = isChars ? 'rgba(91,143,249,0.25)' : 'rgba(93,212,166,0.25)'
  const gradientEnd = isChars ? 'rgba(91,143,249,0.02)' : 'rgba(93,212,166,0.02)'

  // X axis: session labels (团次 #1, #2, #3...)
  const xLabels = data.map(s => `#${s.index}`)
  // Y axis: chars or typing speed
  const yValues = data.map(s => isChars ? s.totalChars : s.typingSpeed)
  // Store session data for tooltip
  const chartData = data.map((s, i) => ({
    value: yValues[i],
    session: s,
  }))

  const yAxisName = isChars
    ? t('inputStats.totalChars')
    : `${t('inputStats.typingSpeed')} (${t('inputStats.charsPerMin')})`

  sessionChartInstance.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: isDark.value ? 'rgba(30,30,35,0.95)' : 'rgba(255,255,255,0.96)',
      borderColor: isDark.value ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.08)',
      textStyle: { color: isDark.value ? '#e0e0e6' : '#333', fontSize: 12 },
      padding: [10, 14],
      formatter: (params: any) => {
        const s = params.data?.session
        if (!s) return ''
        return [
          `<div style="font-weight:700;color:${seriesColor};margin-bottom:4px">${t('inputStats.sessionNo')} #${s.index}</div>`,
          `<div style="font-size:11px;opacity:0.7;margin-bottom:6px">${s.startTime} ~ ${s.endTime}</div>`,
          `<div style="display:grid;grid-template-columns:auto auto;gap:2px 12px;font-size:12px">`,
          `<span style="opacity:0.6">⏱ ${t('inputStats.duration')}:</span><span>${formatDuration(s.duration)}</span>`,
          `<span style="opacity:0.6">✏ ${t('inputStats.totalChars')}:</span><span>${formatNum(s.totalChars)}</span>`,
          `<span style="opacity:0.6">💬 ${t('inputStats.totalMessages')}:</span><span>${s.totalMessages} ${t('inputStats.messages')}</span>`,
          `<span style="opacity:0.6">⚡ ${t('inputStats.typingSpeed')}:</span><span>${formatSpeed(s.typingSpeed)} ${t('inputStats.charsPerMin')}</span>`,
          `</div>`,
        ].join('')
      },
    },
    grid: {
      left: '3%', right: '4%', bottom: '6%', top: '12%', containLabel: true,
    },
    xAxis: {
      type: 'category',
      data: xLabels,
      axisLabel: { color: colors.textColor, fontSize: 11 },
      axisTick: { alignWithLabel: true },
    },
    yAxis: {
      type: 'value',
      name: yAxisName,
      nameTextStyle: { color: colors.textColor, fontSize: 11 },
      axisLabel: { color: colors.textColor },
      splitLine: { lineStyle: { color: colors.splitLineColor } },
    },
    series: [
      {
        name: yAxisName,
        type: 'line',
        smooth: true,
        data: chartData,
        symbolSize: 10,
        symbol: 'circle',
        lineStyle: {
          color: seriesColor,
          width: 2.5,
        },
        itemStyle: {
          color: seriesColor,
          borderColor: isDark.value ? '#1e1e23' : '#fff',
          borderWidth: 2,
        },
        emphasis: {
          itemStyle: {
            color: seriesColor,
            borderColor: '#fff',
            borderWidth: 3,
            shadowBlur: 8,
            shadowColor: seriesColor.replace(')', ',0.5)').replace('rgb', 'rgba'),
          },
          scale: 1.6,
        },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: gradientStart },
            { offset: 1, color: gradientEnd },
          ]),
        },
      },
    ],
  }, true)
}

function formatSpeed(v: number | undefined) {
  if (!v || !isFinite(v)) return '0'
  return v.toFixed(1)
}

function formatNum(v: number | undefined) {
  if (!v) return '0'
  if (v >= 10000) return (v / 10000).toFixed(1) + '万'
  return v.toLocaleString()
}

function formatDuration(min: number) {
  if (min < 1) return '< 1 ' + t('inputStats.minutes')
  if (min >= 60) {
    const h = Math.floor(min / 60)
    const m = Math.round(min % 60)
    return `${h}h ${m}m`
  }
  return `${Math.round(min)} ${t('inputStats.minutes')}`
}

// === Channel filter toggle ===
function getChannelFilterState(channelId: string): 'none' | 'include' | 'exclude' {
  if (!selectedChannelIds.value.includes(channelId)) return 'none'
  return filterChannelMode.value === 'include' ? 'include'
       : filterChannelMode.value === 'exclude' ? 'exclude'
       : 'none'
}

function toggleChannelFilter(channelId: string, mode: 'include' | 'exclude') {
  const currentState = getChannelFilterState(channelId)

  if (currentState === mode) {
    // Deselect: remove from list
    selectedChannelIds.value = selectedChannelIds.value.filter(id => id !== channelId)
    if (selectedChannelIds.value.length === 0) {
      filterChannelMode.value = 'none'
    }
  } else if (currentState === 'none' && filterChannelMode.value === mode) {
    // Same mode, add to list
    selectedChannelIds.value = [...selectedChannelIds.value, channelId]
  } else if (currentState === 'none' && (filterChannelMode.value === 'none' || filterChannelMode.value !== mode)) {
    // Switch to new mode — but preserve other channels if already in the same mode
    filterChannelMode.value = mode
    selectedChannelIds.value = [channelId]
  } else {
    // Was in opposite mode, switch: reset to just this channel
    filterChannelMode.value = mode
    selectedChannelIds.value = [channelId]
  }
  savePrefs()
  fetchAll()
}

function resetChannelFilter() {
  filterChannelMode.value = 'none'
  selectedChannelIds.value = []
  savePrefs()
  fetchAll()
}

// Channel filter note
const channelFilterNote = computed(() => {
  if (filterChannelMode.value === 'none' || selectedChannelIds.value.length === 0) return ''
  const count = selectedChannelIds.value.length
  if (filterChannelMode.value === 'include') {
    return `仅包含 ${count} 个频道`
  }
  return `排除 ${count} 个频道`
})

// === Lifecycle ===
onMounted(() => {
  // Load persisted prefs first
  loadPrefs()

  // Default filter based on context (overrides persisted prefs for world)
  if (props.currentWorldId) {
    filterMode.value = 'include'
    selectedWorldIds.value = [props.currentWorldId]
  }

  fetchAll()
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
  if (sessionChartInstance) {
    sessionChartInstance.dispose()
    sessionChartInstance = null
  }
})

function handleResize() {
  chartInstance?.resize()
  sessionChartInstance?.resize()
}

watch([activeTab, timeRange, customStart, customEnd], () => {
  fetchAll()
})

watch(includeImported, () => {
  savePrefs()
  fetchAll()
})

// Re-render chart when hideZeroAxis changes
watch(hideZeroAxis, () => {
  renderChart()
})

// Re-render session chart when threshold or metric changes, and persist
watch([sessionThreshold, sessionMetric], () => {
  savePrefs()
  nextTick(() => renderSessionChart())
})

// Re-fetch session messages when session IC/OOC changes
watch(sessionIcMode, () => {
  savePrefs()
  fetchSessionMessages()
})

// Re-render chart when theme changes
watch(isDark, () => {
  nextTick(() => {
    renderChart()
    renderSessionChart()
  })
})

// World filter options (built from worldStats)
const worldFilterOptions = computed(() =>
  worldStats.value.map((w: any) => ({
    label: w.worldName,
    value: w.worldId,
  }))
)

// Current world name for display
const currentWorldName = computed(() => {
  if (!props.currentWorldId) return ''
  const world = worldStats.value.find((w: any) => w.worldId === props.currentWorldId)
  return world?.worldName || ''
})
</script>

<template>
  <div class="input-stats-root pointer-events-auto" :class="{ 'is-light': !isDark }">
    <!-- Header -->
    <div class="stats-header">
      <button class="stats-back-btn" @click="emit('close')">← {{ t('inputStats.back') }}</button>
      <h2 class="stats-title">📊 {{ t('inputStats.title') }}</h2>
    </div>

    <!-- Controls -->
    <div class="stats-controls">
      <!-- Time range -->
      <div class="control-group">
        <n-button-group size="small">
          <n-button :type="timeRange === 'all' ? 'primary' : 'default'" @click="timeRange = 'all'">
            {{ t('inputStats.allTime') }}
          </n-button>
          <n-button :type="timeRange === '7d' ? 'primary' : 'default'" @click="timeRange = '7d'">
            {{ t('inputStats.last7Days') }}
          </n-button>
          <n-button :type="timeRange === '30d' ? 'primary' : 'default'" @click="timeRange = '30d'">
            {{ t('inputStats.last30Days') }}
          </n-button>
          <n-button :type="timeRange === 'custom' ? 'primary' : 'default'" @click="timeRange = 'custom'">
            {{ t('inputStats.custom') }}
          </n-button>
        </n-button-group>
        <div v-if="timeRange === 'custom'" class="custom-range">
          <n-date-picker
            v-model:value="customStart"
            type="datetime"
            :placeholder="t('inputStats.startTime')"
            size="small"
            clearable
          />
          <span class="range-sep">~</span>
          <n-date-picker
            v-model:value="customEnd"
            type="datetime"
            :placeholder="t('inputStats.endTime')"
            size="small"
            clearable
          />
        </div>
      </div>

      <!-- IC/OOC tab -->
      <div class="control-group">
        <n-button-group size="small">
          <n-button :type="activeTab === 'all' ? 'primary' : 'default'" @click="activeTab = 'all'">
            {{ t('inputStats.all') }}
          </n-button>
          <n-button :type="activeTab === 'ic' ? 'primary' : 'default'" @click="activeTab = 'ic'">
            {{ t('inputStats.ic') }}
          </n-button>
          <n-button :type="activeTab === 'ooc' ? 'primary' : 'default'" @click="activeTab = 'ooc'">
            {{ t('inputStats.ooc') }}
          </n-button>
        </n-button-group>
      </div>

      <!-- World/Channel filter (styled pill bar) -->
      <div class="control-group filter-group">
        <div class="filter-pill-bar">
          <n-select
            v-model:value="filterMode"
            size="small"
            :options="[
              { label: '不筛选世界', value: 'none' },
              { label: '仅包含世界', value: 'include' },
              { label: '排除世界', value: 'exclude' },
            ]"
            class="filter-mode-select"
          />
          <n-select
            v-if="filterMode !== 'none'"
            v-model:value="selectedWorldIds"
            size="small"
            multiple
            :options="worldFilterOptions"
            placeholder="选择世界"
            class="filter-world-select"
            max-tag-count="responsive"
          />
          <n-button v-if="filterMode !== 'none'" size="small" type="primary" round @click="fetchAll">
            应用
          </n-button>
        </div>
      </div>

      <div class="control-group">
        <label class="zero-toggle">
          <n-switch v-model:value="includeImported" size="small" />
          <span class="zero-toggle-label">{{ t('inputStats.includeImported') }}</span>
        </label>
      </div>

      <!-- Hide zero axis toggle -->
      <div class="control-group">
        <label class="zero-toggle">
          <n-switch v-model:value="hideZeroAxis" size="small" />
          <span class="zero-toggle-label">隐藏零值日期</span>
        </label>
      </div>
    </div>

    <!-- Loading -->
    <n-spin :show="loading">
      <!-- Overview -->
      <div class="overview-cards" v-if="overview">
        <div class="stat-card">
          <div class="stat-value">{{ formatNum(overview.totalChars) }}</div>
          <div class="stat-label">{{ t('inputStats.totalChars') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatNum(overview.totalMessages) }}</div>
          <div class="stat-label">{{ t('inputStats.totalMessages') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatSpeed(overview.typingSpeed) }}</div>
          <div class="stat-label">{{ t('inputStats.typingSpeed') }} ({{ t('inputStats.charsPerMin') }})</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatSpeed(overview.avgCharsPerMsg) }}</div>
          <div class="stat-label">{{ t('inputStats.avgCharsPerMsg') }}</div>
        </div>
      </div>

      <!-- Chart -->
      <div class="section">
        <h3 class="section-title">📈 {{ t('inputStats.dailyTrend') }}</h3>
        <div ref="chartRef" class="chart-container"></div>
      </div>

      <!-- World list -->
      <div class="section">
        <h3 class="section-title">🌍 {{ t('inputStats.worldStats') }}</h3>
        <div v-if="worldStats.length === 0" class="empty-hint">{{ t('inputStats.noData') }}</div>
        <div v-else class="world-list">
          <div
            v-for="w in worldStats"
            :key="w.worldId"
            class="world-item"
          >
            <div class="world-row" @click="fetchChannels(w.worldId)">
              <span class="world-name">{{ w.worldName }}</span>
              <span class="world-stat">{{ formatNum(w.totalChars) }} {{ t('inputStats.chars') }}</span>
              <span class="world-stat">{{ formatNum(w.totalMessages) }} {{ t('inputStats.messages') }}</span>
              <n-button size="tiny" quaternary type="info" @click.stop="fetchChannels(w.worldId)">
                {{ t('inputStats.viewChannels') }} {{ expandedWorldId === w.worldId ? '▲' : '▼' }}
              </n-button>
            </div>
            <!-- Channel drill-down -->
            <div v-if="expandedWorldId === w.worldId && channelStats[w.worldId]" class="channel-list">
              <!-- Channel filter note -->
              <div v-if="channelFilterNote" class="channel-filter-note">
                <span class="channel-filter-note-text">
                  {{ filterChannelMode === 'include' ? '🔵' : '🔴' }}
                  {{ channelFilterNote }}
                </span>
                <n-button size="tiny" quaternary type="warning" @click.stop="resetChannelFilter">
                  ↺ 重置
                </n-button>
              </div>
              <div class="channel-filter-hint" v-else>
                <span>+ 包含 / − 排除，可多选</span>
              </div>
              <div v-for="ch in channelStats[w.worldId]" :key="ch.channelId" class="channel-row" :class="{
                'channel-included': getChannelFilterState(ch.channelId) === 'include',
                'channel-excluded': getChannelFilterState(ch.channelId) === 'exclude',
              }">
                <span class="channel-name"># {{ ch.channelName }}</span>
                <span class="channel-stat">{{ formatNum(ch.totalChars) }} {{ t('inputStats.chars') }}</span>
                <span class="channel-stat">{{ formatNum(ch.totalMessages) }} {{ t('inputStats.messages') }}</span>
                <span class="channel-filter-btns">
                  <n-button
                    size="tiny"
                    :type="getChannelFilterState(ch.channelId) === 'include' ? 'primary' : 'default'"
                    @click.stop="toggleChannelFilter(ch.channelId, 'include')"
                    quaternary
                  >+</n-button>
                  <n-button
                    size="tiny"
                    :type="getChannelFilterState(ch.channelId) === 'exclude' ? 'error' : 'default'"
                    @click.stop="toggleChannelFilter(ch.channelId, 'exclude')"
                    quaternary
                  >−</n-button>
                </span>
              </div>
              <div v-if="channelStats[w.worldId].length === 0" class="empty-hint">{{ t('inputStats.noData') }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- Session analysis (only when world is filtered) -->
      <div class="section" v-if="isWorldFiltered">
        <h3 class="section-title">🎲 {{ t('inputStats.sessionAnalysis') }}</h3>
        <div class="session-controls">
          <div class="session-threshold">
            <span>{{ t('inputStats.sessionThreshold') }}: {{ sessionThreshold }}</span>
            <n-slider
              v-model:value="sessionThreshold"
              :min="5"
              :max="120"
              :step="5"
              :tooltip="true"
              style="width: 200px"
            />
          </div>
          <n-button-group size="small">
            <n-button
              :type="sessionMetric === 'chars' ? 'primary' : 'default'"
              @click="sessionMetric = 'chars'"
            >
              {{ t('inputStats.totalChars') }}
            </n-button>
            <n-button
              :type="sessionMetric === 'speed' ? 'primary' : 'default'"
              @click="sessionMetric = 'speed'"
            >
              {{ t('inputStats.typingSpeed') }}
            </n-button>
          </n-button-group>
          <n-button-group size="small">
            <n-button
              :type="sessionIcMode === 'all' ? 'primary' : 'default'"
              @click="sessionIcMode = 'all'"
            >
              {{ t('inputStats.all') }}
            </n-button>
            <n-button
              :type="sessionIcMode === 'ic' ? 'primary' : 'default'"
              @click="sessionIcMode = 'ic'"
            >
              {{ t('inputStats.ic') }}
            </n-button>
            <n-button
              :type="sessionIcMode === 'ooc' ? 'primary' : 'default'"
              @click="sessionIcMode = 'ooc'"
            >
              {{ t('inputStats.ooc') }}
            </n-button>
          </n-button-group>
        </div>
        <div v-if="sessions.length === 0" class="empty-hint">{{ t('inputStats.noData') }}</div>
        <div ref="sessionChartRef" v-else class="session-chart-container"></div>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
/* ===== Theme-adaptive root ===== */
.input-stats-root {
  background: var(--sc-bg-elevated, rgba(24, 24, 28, 0.96));
  border-radius: 12px;
  border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, 0.08));
  backdrop-filter: blur(20px);
  color: var(--sc-text-primary, #e0e0e6);
  max-width: 820px;
  width: 100%;
  max-height: 85vh;
  overflow-y: auto;
  padding: 0;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4), 0 0 0 1px var(--sc-border-mute, rgba(255,255,255,0.04));
  transition: background-color 0.25s ease, color 0.25s ease, border-color 0.25s ease;
}

.input-stats-root.is-light {
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1), 0 0 0 1px var(--sc-border-mute, rgba(0,0,0,0.06));
}

.input-stats-root::-webkit-scrollbar {
  width: 6px;
}
.input-stats-root::-webkit-scrollbar-thumb {
  background: var(--sc-scrollbar-thumb, rgba(255, 255, 255, 0.12));
  border-radius: 3px;
}

/* ===== Header ===== */
.stats-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 20px 8px;
  border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, 0.06));
  position: sticky;
  top: 0;
  background: var(--sc-bg-elevated, rgba(24, 24, 28, 0.98));
  z-index: 10;
  transition: background-color 0.25s ease, border-color 0.25s ease;
}
.stats-back-btn {
  background: none;
  border: none;
  color: #5b8ff9;
  cursor: pointer;
  font-size: 13px;
  padding: 4px 8px;
  border-radius: 6px;
  transition: background 0.15s;
}
.stats-back-btn:hover {
  background: rgba(91, 143, 249, 0.12);
}
.stats-title {
  font-size: 18px;
  font-weight: 700;
  margin: 0;
  letter-spacing: 0.5px;
}

/* ===== Controls ===== */
.stats-controls {
  padding: 12px 20px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, 0.06));
  transition: border-color 0.25s ease;
}
.control-group {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.custom-range {
  display: flex;
  align-items: center;
  gap: 4px;
}
.range-sep {
  color: var(--sc-text-secondary, #666);
  padding: 0 2px;
}

/* ===== World filter pill bar ===== */
.filter-group {
  margin-left: auto;
}

.filter-pill-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--sc-chip-bg, rgba(255, 255, 255, 0.06));
  border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, 0.08));
  border-radius: 20px;
  padding: 3px 6px;
  transition: background 0.2s, border-color 0.2s;
}

.filter-mode-select {
  min-width: 120px;
  max-width: 140px;
}

.filter-world-select {
  min-width: 140px;
  max-width: 280px;
}

.filter-pill-bar :deep(.n-base-selection) {
  border-radius: 14px !important;
}

.filter-pill-bar :deep(.n-base-selection .n-base-selection-label) {
  border-radius: 14px !important;
}

.filter-pill-bar :deep(.n-base-selection .n-base-selection__border),
.filter-pill-bar :deep(.n-base-selection .n-base-selection__state-border) {
  border-radius: 14px !important;
}

.filter-pill-bar :deep(.n-button) {
  border-radius: 14px !important;
}

/* ===== Zero toggle ===== */
.zero-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
.zero-toggle-label {
  font-size: 12px;
  color: var(--sc-text-secondary, #aaa);
  white-space: nowrap;
  user-select: none;
}

/* ===== Overview cards ===== */
.overview-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  padding: 16px 20px;
}
.stat-card {
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--sc-text-primary, #5b8ff9) 8%, transparent),
    color-mix(in srgb, var(--sc-text-primary, #5dd4a6) 4%, transparent)
  );
  border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, 0.06));
  border-radius: 10px;
  padding: 16px;
  text-align: center;
  transition: transform 0.15s, box-shadow 0.15s, background 0.25s, border-color 0.25s;
}
.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 16px rgba(91,143,249,0.15);
}
.stat-value {
  font-size: 24px;
  font-weight: 800;
  color: var(--sc-text-primary, #fff);
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}
.stat-label {
  font-size: 11px;
  color: var(--sc-text-secondary, #999);
  margin-top: 4px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

/* ===== Sections ===== */
.section {
  padding: 12px 20px 16px;
}
.section-title {
  font-size: 15px;
  font-weight: 600;
  margin: 0 0 10px;
  color: var(--sc-text-secondary, #ccc);
}

.chart-container {
  width: 100%;
  height: 320px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--sc-bg-surface, #000) 60%, transparent);
  transition: background 0.25s ease;
}

.empty-hint {
  color: var(--sc-text-secondary, #666);
  text-align: center;
  padding: 16px;
  font-size: 13px;
}

/* ===== World list ===== */
.world-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.world-item {
  border-radius: 8px;
  overflow: hidden;
}
.world-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  background: var(--sc-chip-bg, rgba(255, 255, 255, 0.03));
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s;
}
.world-row:hover {
  background: color-mix(in srgb, var(--sc-text-primary, #fff) 6%, transparent);
}
.world-name {
  font-weight: 600;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.world-stat, .channel-stat {
  font-size: 12px;
  color: var(--sc-text-secondary, #999);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.channel-list {
  padding: 4px 0 4px 24px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.channel-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 12px;
  border-radius: 6px;
  transition: background 0.1s;
}
.channel-row:hover {
  background: color-mix(in srgb, var(--sc-text-primary, #fff) 4%, transparent);
}
.channel-name {
  flex: 1;
  font-size: 13px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--sc-text-secondary, #b0b0c0);
}

.channel-filter-btns {
  display: flex;
  gap: 2px;
  margin-left: auto;
  flex-shrink: 0;
}

.channel-filter-note {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 12px;
  margin-bottom: 4px;
  border-radius: 6px;
  background: rgba(91, 143, 249, 0.06);
  font-size: 12px;
  color: var(--sc-text-secondary, #aaa);
}

.channel-filter-note-text {
  font-weight: 500;
}

.channel-filter-hint {
  padding: 2px 12px 4px;
  font-size: 11px;
  color: var(--sc-text-secondary, #777);
  opacity: 0.7;
}

.channel-row.channel-included {
  background: rgba(91, 143, 249, 0.1);
  border-left: 3px solid #5b8ff9;
}

.channel-row.channel-excluded {
  background: rgba(208, 48, 80, 0.08);
  border-left: 3px solid #d03050;
  opacity: 0.7;
}

/* ===== Session ===== */
.session-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 0 0 12px;
}

.session-threshold {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
  color: var(--sc-text-secondary, #aaa);
}

.session-chart-container {
  width: 100%;
  height: 260px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--sc-bg-surface, #000) 60%, transparent);
  transition: background 0.25s ease;
}
</style>
