<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import type { StickyNote, TimerTypeData } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const typeData = computed<TimerTypeData>(() => {
  const parsed = stickyNoteStore.parseTypeData<TimerTypeData>(props.note)
  return parsed || { startTime: 0, baseValue: 0, direction: 'up', running: false, resetValue: 0 }
})

const draftData = ref<TimerTypeData>({ ...typeData.value })
const isInteracting = ref(false)
const displayTime = ref('00:00:00')
let intervalId: ReturnType<typeof setInterval> | null = null
let holdStartTimer: ReturnType<typeof setTimeout> | null = null
let holdInterval: ReturnType<typeof setInterval> | null = null
let holdDelta = 0
let holdStarted = false
let suppressNextClick = false
let suppressClickTimer: ReturnType<typeof setTimeout> | null = null

function formatTime(seconds: number): string {
  const abs = Math.abs(seconds)
  const h = Math.floor(abs / 3600)
  const m = Math.floor((abs % 3600) / 60)
  const s = abs % 60
  const sign = seconds < 0 ? '-' : ''
  return `${sign}${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

function calculateCurrentValue(data: TimerTypeData = draftData.value): number {
  const { startTime, baseValue, direction, running } = data
  if (!running) return baseValue
  const elapsed = Math.floor((Date.now() - startTime) / 1000)
  return direction === 'up' ? baseValue + elapsed : baseValue - elapsed
}

function updateDisplay() {
  displayTime.value = formatTime(calculateCurrentValue())
}

function commitDraft() {
  stickyNoteStore.updateTypeData(props.note.id, { ...draftData.value })
  isInteracting.value = false
}

function toggleTimer() {
  const { running } = draftData.value
  if (running) {
    const currentValue = calculateCurrentValue()
    draftData.value = {
      ...draftData.value,
      running: false,
      baseValue: currentValue,
      startTime: 0
    }
  } else {
    draftData.value = {
      ...draftData.value,
      running: true,
      startTime: Date.now()
    }
  }
  updateDisplay()
  commitDraft()
}

function resetTimer() {
  const { resetValue } = draftData.value
  draftData.value = {
    ...draftData.value,
    running: false,
    baseValue: resetValue,
    startTime: 0
  }
  updateDisplay()
  commitDraft()
}

function setDirection(dir: 'up' | 'down') {
  const currentValue = draftData.value.running ? calculateCurrentValue() : draftData.value.baseValue
  draftData.value = {
    ...draftData.value,
    direction: dir,
    baseValue: currentValue,
    startTime: draftData.value.running ? Date.now() : 0
  }
  updateDisplay()
  commitDraft()
}

function adjustTime(delta: number) {
  const currentValue = draftData.value.running ? calculateCurrentValue() : draftData.value.baseValue
  draftData.value = {
    ...draftData.value,
    baseValue: currentValue + delta,
    startTime: draftData.value.running ? Date.now() : 0
  }
  updateDisplay()
}

function handleAdjustClick(delta: number) {
  if (suppressNextClick) {
    suppressNextClick = false
    if (suppressClickTimer) {
      clearTimeout(suppressClickTimer)
      suppressClickTimer = null
    }
    return
  }
  adjustTime(delta)
  commitDraft()
}

function startAdjustHold(delta: number, e: PointerEvent) {
  if (e.pointerType === 'mouse' && e.button !== 0) return
  stopAdjustHold()
  isInteracting.value = true
  holdDelta = delta
  holdStarted = false
  suppressNextClick = false
  const target = e.currentTarget as HTMLButtonElement | null
  target?.setPointerCapture?.(e.pointerId)
  holdStartTimer = setTimeout(() => {
    holdStarted = true
    adjustTime(holdDelta)
    holdInterval = setInterval(() => adjustTime(holdDelta), 100)
  }, 350)
}

function stopAdjustHold() {
  if (holdStartTimer) {
    clearTimeout(holdStartTimer)
    holdStartTimer = null
  }
  if (holdInterval) {
    clearInterval(holdInterval)
    holdInterval = null
  }
  if (holdStarted) {
    suppressNextClick = true
    if (suppressClickTimer) clearTimeout(suppressClickTimer)
    suppressClickTimer = setTimeout(() => {
      suppressNextClick = false
      suppressClickTimer = null
    }, 500)
    commitDraft()
  }
  holdStarted = false
}

function setResetValue() {
  const currentValue = draftData.value.running ? calculateCurrentValue() : draftData.value.baseValue
  draftData.value = {
    ...draftData.value,
    resetValue: currentValue
  }
  commitDraft()
}

watch(() => typeData.value, (data) => {
  if (!isInteracting.value) {
    draftData.value = { ...data }
  }
  updateDisplay()
}, { immediate: true })

onMounted(() => {
  updateDisplay()
  intervalId = setInterval(updateDisplay, 1000)
})

onUnmounted(() => {
  if (intervalId) clearInterval(intervalId)
  stopAdjustHold()
  if (suppressClickTimer) clearTimeout(suppressClickTimer)
})
</script>

<template>
  <div class="sticky-note-timer">
    <div class="sticky-note-timer__display">
      {{ displayTime }}
    </div>

    <div class="sticky-note-timer__controls">
      <button
        class="sticky-note-timer__btn sticky-note-timer__btn--primary"
        @click="toggleTimer"
      >
        {{ typeData.running ? '暂停' : '开始' }}
      </button>
      <button
        class="sticky-note-timer__btn"
        @click="resetTimer"
      >
        重置
      </button>
    </div>

    <div class="sticky-note-timer__direction">
      <button
        class="sticky-note-timer__dir-btn"
        :class="{ 'sticky-note-timer__dir-btn--active': typeData.direction === 'up' }"
        @click="setDirection('up')"
      >
        ↑ 正计时
      </button>
      <button
        class="sticky-note-timer__dir-btn"
        :class="{ 'sticky-note-timer__dir-btn--active': typeData.direction === 'down' }"
        @click="setDirection('down')"
      >
        ↓ 倒计时
      </button>
    </div>

    <div class="sticky-note-timer__adjust">
      <button
        class="sticky-note-timer__adj-btn"
        @click="handleAdjustClick(-60)"
        @pointerdown="startAdjustHold(-60, $event)"
        @pointerup="stopAdjustHold"
        @pointercancel="stopAdjustHold"
        @contextmenu.prevent
      >-1m</button>
      <button
        class="sticky-note-timer__adj-btn"
        @click="handleAdjustClick(-10)"
        @pointerdown="startAdjustHold(-10, $event)"
        @pointerup="stopAdjustHold"
        @pointercancel="stopAdjustHold"
        @contextmenu.prevent
      >-10s</button>
      <button
        class="sticky-note-timer__adj-btn"
        @click="handleAdjustClick(10)"
        @pointerdown="startAdjustHold(10, $event)"
        @pointerup="stopAdjustHold"
        @pointercancel="stopAdjustHold"
        @contextmenu.prevent
      >+10s</button>
      <button
        class="sticky-note-timer__adj-btn"
        @click="handleAdjustClick(60)"
        @pointerdown="startAdjustHold(60, $event)"
        @pointerup="stopAdjustHold"
        @pointercancel="stopAdjustHold"
        @contextmenu.prevent
      >+1m</button>
    </div>

    <div class="sticky-note-timer__footer">
      <button class="sticky-note-timer__set-reset" @click="setResetValue">
        设为重置值
      </button>
    </div>
  </div>
</template>

<style scoped>
.sticky-note-timer {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 16px;
  gap: 12px;
}

.sticky-note-timer__display {
  font-size: 32px;
  font-weight: bold;
  font-family: 'Courier New', monospace;
  color: rgba(0, 0, 0, 0.85);
  letter-spacing: 2px;
}

.sticky-note-timer__controls {
  display: flex;
  gap: 8px;
}

.sticky-note-timer__btn {
  padding: 8px 16px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  color: rgba(0, 0, 0, 0.7);
  transition: background 0.15s;
}

.sticky-note-timer__btn:hover {
  background: rgba(0, 0, 0, 0.15);
}

.sticky-note-timer__btn--primary {
  background: rgba(0, 0, 0, 0.2);
  font-weight: bold;
}

.sticky-note-timer__btn--primary:hover {
  background: rgba(0, 0, 0, 0.25);
}

.sticky-note-timer__direction {
  display: flex;
  gap: 4px;
}

.sticky-note-timer__dir-btn {
  padding: 6px 12px;
  border: 1px solid rgba(0, 0, 0, 0.2);
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.6);
}

.sticky-note-timer__dir-btn--active {
  background: rgba(0, 0, 0, 0.1);
  border-color: rgba(0, 0, 0, 0.3);
  color: rgba(0, 0, 0, 0.8);
}

.sticky-note-timer__adjust {
  display: flex;
  gap: 4px;
}

.sticky-note-timer__adj-btn {
  padding: 4px 8px;
  border: none;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 4px;
  cursor: pointer;
  font-size: 11px;
  color: rgba(0, 0, 0, 0.6);
  touch-action: manipulation;
  user-select: none;
}

.sticky-note-timer__adj-btn:hover {
  background: rgba(0, 0, 0, 0.12);
}

.sticky-note-timer__footer {
  margin-top: 4px;
}

.sticky-note-timer__set-reset {
  padding: 4px 8px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 11px;
  color: rgba(0, 0, 0, 0.4);
  text-decoration: underline;
}

.sticky-note-timer__set-reset:hover {
  color: rgba(0, 0, 0, 0.6);
}
</style>
