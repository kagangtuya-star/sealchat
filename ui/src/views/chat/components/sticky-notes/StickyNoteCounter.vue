<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import type { StickyNote, CounterTypeData } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const typeData = computed<CounterTypeData>(() => {
  const parsed = stickyNoteStore.parseTypeData<CounterTypeData>(props.note)
  return parsed || { value: 0 }
})

const displayValue = computed(() => {
  const { value, max } = typeData.value
  if (max !== undefined && max > 0) {
    return `${value}/${max}`
  }
  return String(value)
})

const inputValue = ref('')

watch(() => typeData.value, (data) => {
  if (data.max !== undefined && data.max > 0) {
    inputValue.value = `${data.value}/${data.max}`
  } else {
    inputValue.value = String(data.value)
  }
}, { immediate: true })

let holdInterval: ReturnType<typeof setInterval> | null = null

function normalizeValue(value: number, max?: number): number {
  const upper = max !== undefined && max > 0 ? max : Infinity
  return Math.max(0, Math.min(upper, value))
}

function parseInput() {
  const raw = inputValue.value.trim()
  if (raw.includes('/')) {
    const [valPart, maxPart] = raw.split('/')
    const val = parseInt(valPart, 10)
    const max = parseInt(maxPart, 10)
    if (!isNaN(val)) {
      const nextMax = !isNaN(max) && max > 0 ? max : undefined
      const newData: CounterTypeData = { value: normalizeValue(val, nextMax) }
      if (!isNaN(max) && max > 0) {
        newData.max = max
      }
      stickyNoteStore.updateTypeData(props.note.id, newData)
    }
  } else {
    const val = parseInt(raw, 10)
    if (!isNaN(val)) {
      stickyNoteStore.updateTypeData(props.note.id, {
        value: normalizeValue(val, typeData.value.max),
        max: typeData.value.max
      })
    }
  }
}

function getStep(e: MouseEvent): number {
  if (e.shiftKey) return 10
  if (e.altKey) return 5
  return 1
}

function increment(e: MouseEvent) {
  const step = getStep(e)
  const newValue = normalizeValue(typeData.value.value + step, typeData.value.max)
  stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, value: newValue })
}

function decrement(e: MouseEvent) {
  const step = getStep(e)
  const newValue = normalizeValue(typeData.value.value - step, typeData.value.max)
  stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, value: newValue })
}

function startHold(direction: 'inc' | 'dec', e: MouseEvent) {
  if (holdInterval) return
  const step = getStep(e)
  holdInterval = setInterval(() => {
    const current = typeData.value.value
    const next = direction === 'inc' ? current + step : current - step
    const newValue = normalizeValue(next, typeData.value.max)
    stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, value: newValue })
  }, 100)
}

function stopHold() {
  if (holdInterval) {
    clearInterval(holdInterval)
    holdInterval = null
  }
}

onUnmounted(() => {
  stopHold()
})
</script>

<template>
  <div class="sticky-note-counter">
    <div class="sticky-note-counter__controls">
      <button
        class="sticky-note-counter__btn sticky-note-counter__btn--dec"
        @click="decrement"
        @mousedown="(e) => startHold('dec', e)"
        @mouseup="stopHold"
        @mouseleave="stopHold"
      >
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          <path d="M19 13H5v-2h14v2z"/>
        </svg>
      </button>

      <input
        v-model="inputValue"
        class="sticky-note-counter__value"
        @blur="parseInput"
        @keyup.enter="parseInput"
      />

      <button
        class="sticky-note-counter__btn sticky-note-counter__btn--inc"
        @click="increment"
        @mousedown="(e) => startHold('inc', e)"
        @mouseup="stopHold"
        @mouseleave="stopHold"
      >
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
        </svg>
      </button>
    </div>
    <div class="sticky-note-counter__hint">
      Shift: ±10 | Alt: ±5 | 长按快速调整
    </div>
  </div>
</template>

<style scoped>
.sticky-note-counter {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 16px;
}

.sticky-note-counter__controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sticky-note-counter__btn {
  width: 48px;
  height: 48px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(0, 0, 0, 0.7);
  transition: all 0.15s;
}

.sticky-note-counter__btn:hover {
  background: rgba(0, 0, 0, 0.2);
}

.sticky-note-counter__btn:active {
  transform: scale(0.95);
}

.sticky-note-counter__value {
  width: 120px;
  height: 48px;
  text-align: center;
  font-size: 24px;
  font-weight: bold;
  border: 2px solid rgba(0, 0, 0, 0.2);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.5);
  color: rgba(0, 0, 0, 0.85);
}

.sticky-note-counter__value:focus {
  outline: none;
  border-color: rgba(0, 0, 0, 0.4);
}

.sticky-note-counter__hint {
  margin-top: 12px;
  font-size: 11px;
  color: rgba(0, 0, 0, 0.5);
}
</style>
