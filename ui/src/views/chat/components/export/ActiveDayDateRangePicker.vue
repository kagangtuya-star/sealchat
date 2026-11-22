<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores/chat'
import { useCalendarActiveDays } from '@/composables/useCalendarActiveDays'

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
  (e: 'update:open', value: boolean): void
}

const props = withDefaults(defineProps<Props>(), {
  showPresets: true,
  placeholder: '选择时间范围，留空表示全部',
})
const emit = defineEmits<Emits>()

const chat = useChatStore()
const { locale } = useI18n()
const calendarHost = ref<HTMLElement | null>(null)
const calendarOpen = ref(false)
watch(calendarOpen, (open) => emit('update:open', open))
useCalendarActiveDays({
  root: calendarHost,
  enabled: () => calendarOpen.value && !!props.channelId,
  context: () => props.channelId || '',
  locale: () => locale.value,
  load: async (month, channelId) => (await chat.getChannelMessageActiveDays(channelId, month)).days || [],
})
const timePreset = ref<'none' | PresetValue | 'custom'>('none')
const isApplyingPreset = ref(false)

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
</script>

<template>
  <div class="active-day-date-range-picker">
    <Teleport to="body"><div ref="calendarHost" class="sc-export-calendar" /></Teleport>
    <n-date-picker
      v-model:value="innerValue"
      v-model:show="calendarOpen"
      type="datetimerange"
      clearable
      :to="calendarHost || undefined"
      :shortcuts="shortcuts"
      format="yyyy-MM-dd HH:mm:ss"
      :placeholder="placeholder"
      style="flex: 1"
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
/* 导出保留原有橙色视觉；叠层结构与 CSS 由公共能力提供。 */
.sc-export-calendar {
  --sc-calendar-active-day-background: var(--sc-export-active-day-background, linear-gradient(180deg, rgba(245, 158, 11, 0.26), rgba(249, 115, 22, 0.14)));
  --sc-calendar-active-day-border: var(--sc-export-active-day-border, inset 0 0 0 1px rgba(251, 191, 36, 0.28));
  --sc-calendar-active-day-dot: var(--sc-export-active-day-dot, linear-gradient(180deg, rgba(251, 191, 36, 0.8), rgba(249, 115, 22, 0.72)));
  --sc-calendar-active-day-dot-shadow: var(--sc-export-active-day-dot-shadow, 0 0 0 1px rgba(255, 237, 213, 0.34), 0 1px 2px rgba(194, 65, 12, 0.16));
}

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
