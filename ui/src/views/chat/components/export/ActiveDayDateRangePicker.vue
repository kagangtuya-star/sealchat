<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useChatStore } from '@/stores/chat'
import {
  applyCalendarActiveDayHighlights,
  buildPanelMonthActiveDayMap,
  collectPanelRelatedMonthKeys,
  parsePanelMonthKey,
} from './calendarActiveDayHighlight'

type DateRangeValue = [number, number] | null
type PresetValue = '1d' | '7d' | '30d'

interface Props {
  channelId?: string
  modelValue: DateRangeValue
  showPresets?: boolean
  placeholder?: string
}

interface Emits {
  (e: 'update:modelValue', value: DateRangeValue): void
}

const props = withDefaults(defineProps<Props>(), {
  showPresets: true,
  placeholder: '选择时间范围，留空表示全部',
})
const emit = defineEmits<Emits>()

const chat = useChatStore()
const activeDaysByMonth = ref<Record<string, string[]>>({})
const loadingMonths = ref<Set<string>>(new Set())
const timePreset = ref<'none' | PresetValue | 'custom'>('none')
const isApplyingPreset = ref(false)
let calendarMonthObserver: MutationObserver | null = null

const activeDaySetByMonth = computed(() => buildPanelMonthActiveDayMap(activeDaysByMonth.value))
const innerValue = computed<DateRangeValue>({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const shortcuts = {
  今天: (): [number, number] => {
    const now = new Date()
    const start = new Date(now)
    start.setHours(0, 0, 0, 0)
    const end = new Date(now)
    end.setHours(23, 59, 59, 999)
    return [start.getTime(), end.getTime()]
  },
  昨天: (): [number, number] => {
    const now = new Date()
    const start = new Date(now)
    start.setDate(start.getDate() - 1)
    start.setHours(0, 0, 0, 0)
    const end = new Date(start)
    end.setHours(23, 59, 59, 999)
    return [start.getTime(), end.getTime()]
  },
  最近七天: (): [number, number] => {
    const end = Date.now()
    return [end - 7 * 24 * 60 * 60 * 1000, end]
  },
}

const timePresets = [
  { label: '一天内', value: '1d' },
  { label: '一周内', value: '7d' },
  { label: '一月内', value: '30d' },
]

const clearCalendarHighlights = () => {
  const panel = document.querySelector<HTMLElement>('.n-date-panel')
  if (!panel) return
  applyCalendarActiveDayHighlights({ panel, activeDaySetByMonth: {} })
}

const stopCalendarMonthObserver = () => {
  if (!calendarMonthObserver) return
  calendarMonthObserver.disconnect()
  calendarMonthObserver = null
}

const syncCalendarHighlights = () => {
  const panel = document.querySelector<HTMLElement>('.n-date-panel')
  if (!panel) {
    clearCalendarHighlights()
    return
  }
  applyCalendarActiveDayHighlights({
    panel,
    activeDaySetByMonth: activeDaySetByMonth.value,
  })
}

const collectVisibleCalendarMonths = () => {
  const panel = document.querySelector<HTMLElement>('.n-date-panel')
  if (!panel) return [] as string[]
  return Array.from(panel.querySelectorAll<HTMLElement>('.n-date-panel-calendar .n-date-panel-month__month-year'))
    .map((node) => parsePanelMonthKey(node.textContent || ''))
    .filter((value, index, list) => !!value && list.indexOf(value) === index)
}

const ensureCalendarMonthLoaded = async (month: string) => {
  const normalizedMonth = String(month || '').trim()
  if (!props.channelId || !normalizedMonth) return
  if (activeDaysByMonth.value[normalizedMonth] || loadingMonths.value.has(normalizedMonth)) return
  const loading = new Set(loadingMonths.value)
  loading.add(normalizedMonth)
  loadingMonths.value = loading
  try {
    const resp = await chat.getChannelMessageActiveDays(props.channelId, normalizedMonth)
    activeDaysByMonth.value = {
      ...activeDaysByMonth.value,
      [normalizedMonth]: (resp.days || []).slice().sort(),
    }
  } catch (error) {
    console.warn('加载日历活跃日期失败', error)
  } finally {
    const nextLoading = new Set(loadingMonths.value)
    nextLoading.delete(normalizedMonth)
    loadingMonths.value = nextLoading
  }
}

const ensureVisibleCalendarMonthsLoaded = async () => {
  if (!props.channelId) return
  const panel = document.querySelector<HTMLElement>('.n-date-panel')
  if (!panel) return
  const monthKeys = new Set<string>()
  Array.from(panel.querySelectorAll<HTMLElement>('.n-date-panel-calendar')).forEach((calendar) => {
    const monthKey = parsePanelMonthKey(
      calendar.querySelector<HTMLElement>('.n-date-panel-month__month-year')?.textContent || ''
    )
    if (!monthKey) return
    const cellCount = calendar.querySelectorAll('[data-n-date].n-date-panel-date').length
    collectPanelRelatedMonthKeys(monthKey, cellCount).forEach((relatedMonth) => monthKeys.add(relatedMonth))
  })
  if (!monthKeys.size) {
    collectVisibleCalendarMonths().forEach((month) => monthKeys.add(month))
  }
  if (!monthKeys.size) return
  await Promise.all(Array.from(monthKeys).map((month) => ensureCalendarMonthLoaded(month)))
}

const startCalendarMonthObserver = () => {
  stopCalendarMonthObserver()
  const panel = document.querySelector<HTMLElement>('.n-date-panel')
  if (!panel) return
  calendarMonthObserver = new MutationObserver(() => {
    syncCalendarHighlightsAfterViewportChange()
  })
  panel.querySelectorAll('.n-date-panel-month__month-year').forEach((node) => {
    calendarMonthObserver?.observe(node, {
      childList: true,
      characterData: true,
      subtree: true,
    })
  })
}

const syncCalendarHighlightsAfterViewportChange = () => {
  void nextTick(async () => {
    await ensureVisibleCalendarMonthsLoaded()
    syncCalendarHighlights()
    startCalendarMonthObserver()
  })
}

const handleCalendarShowUpdate = (show: boolean) => {
  if (!show) {
    stopCalendarMonthObserver()
    clearCalendarHighlights()
    return
  }
  syncCalendarHighlightsAfterViewportChange()
}

const applyPresetRange = (preset: PresetValue) => {
  isApplyingPreset.value = true
  const end = Date.now()
  const day = 24 * 60 * 60 * 1000
  const start = preset === '1d' ? end - day : preset === '7d' ? end - 7 * day : end - 30 * day
  innerValue.value = [start, end]
  timePreset.value = preset
  void nextTick(() => {
    isApplyingPreset.value = false
  })
}

const handleClearPreset = () => {
  innerValue.value = null
  timePreset.value = 'none'
}

const resetCalendarHighlightState = () => {
  activeDaysByMonth.value = {}
  loadingMonths.value = new Set()
  stopCalendarMonthObserver()
  clearCalendarHighlights()
}

watch(
  () => props.modelValue,
  (newVal, oldVal) => {
    if (isApplyingPreset.value) return
    if (!newVal && oldVal) {
      timePreset.value = 'none'
      return
    }
    if (newVal && timePreset.value !== 'custom') {
      timePreset.value = 'custom'
    }
  },
)

watch(
  () => props.channelId,
  () => resetCalendarHighlightState(),
)

watch(
  () => activeDaysByMonth.value,
  () => {
    if (document.querySelector('.n-date-panel')) {
      void nextTick(syncCalendarHighlights)
    }
  },
  { deep: true },
)

onBeforeUnmount(() => resetCalendarHighlightState())
</script>

<template>
  <div class="active-day-date-range-picker">
    <n-date-picker
      v-model:value="innerValue"
      type="datetimerange"
      clearable
      to="body"
      :shortcuts="shortcuts"
      format="yyyy-MM-dd HH:mm:ss"
      :placeholder="placeholder"
      style="flex: 1"
      @update:show="handleCalendarShowUpdate"
    />
    <div v-if="showPresets" class="preset-group">
      <n-button-group size="small">
        <n-button
          v-for="item in timePresets"
          :key="item.value"
          :type="timePreset === item.value ? 'primary' : 'default'"
          @click="applyPresetRange(item.value as PresetValue)"
        >
          {{ item.label }}
        </n-button>
      </n-button-group>
      <n-button v-if="timePreset !== 'none'" text size="small" @click="handleClearPreset">
        清除
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.active-day-date-range-picker {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.preset-group {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
