<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { StickyNote, SliderTypeData } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const typeData = computed<SliderTypeData>(() => {
  const parsed = stickyNoteStore.parseTypeData<SliderTypeData>(props.note)
  return parsed || { value: 50, min: 0, max: 100, step: 1 }
})

const localValue = ref(typeData.value.value)
const showSettings = ref(false)
const settingsMin = ref(typeData.value.min)
const settingsMax = ref(typeData.value.max)
const settingsStep = ref(typeData.value.step)

watch(() => typeData.value, (data) => {
  localValue.value = data.value
  settingsMin.value = data.min
  settingsMax.value = data.max
  settingsStep.value = data.step
}, { immediate: true })

const percentage = computed(() => {
  const { min, max, value } = typeData.value
  const range = max - min
  if (!Number.isFinite(range) || range <= 0) return 0
  return ((value - min) / range) * 100
})

function updateValue(newValue: number) {
  const { min, max, step } = typeData.value
  const safeStep = Number.isFinite(step) && step > 0 ? step : 1
  const clamped = Math.max(min, Math.min(max, newValue))
  const stepped = Math.round((clamped - min) / safeStep) * safeStep + min
  stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, value: stepped })
}

function handleSliderInput(e: Event) {
  const target = e.target as HTMLInputElement
  updateValue(parseFloat(target.value))
}

function handleInputChange() {
  updateValue(localValue.value)
}

function saveSettings() {
  stickyNoteStore.updateTypeData(props.note.id, {
    value: Math.max(settingsMin.value, Math.min(settingsMax.value, typeData.value.value)),
    min: settingsMin.value,
    max: settingsMax.value,
    step: settingsStep.value
  })
  showSettings.value = false
}
</script>

<template>
  <div class="sticky-note-slider">
    <div class="sticky-note-slider__main">
      <input
        type="range"
        :min="typeData.min"
        :max="typeData.max"
        :step="typeData.step"
        :value="typeData.value"
        class="sticky-note-slider__range"
        @input="handleSliderInput"
      />

      <div class="sticky-note-slider__value-row">
        <span class="sticky-note-slider__min">{{ typeData.min }}</span>
        <input
          v-model.number="localValue"
          type="number"
          class="sticky-note-slider__value-input"
          :min="typeData.min"
          :max="typeData.max"
          :step="typeData.step"
          @change="handleInputChange"
        />
        <span class="sticky-note-slider__max">{{ typeData.max }}</span>
      </div>

      <div class="sticky-note-slider__bar">
        <div
          class="sticky-note-slider__fill"
          :style="{ width: `${percentage}%` }"
        ></div>
      </div>
    </div>

    <div
      class="sticky-note-slider__settings-trigger"
      @mouseenter="showSettings = true"
      @mouseleave="showSettings = false"
    >
      <div class="sticky-note-slider__settings-hint">设置</div>

      <div v-if="showSettings" class="sticky-note-slider__settings-panel">
        <div class="sticky-note-slider__settings-row">
          <label>最小值</label>
          <input v-model.number="settingsMin" type="number" />
        </div>
        <div class="sticky-note-slider__settings-row">
          <label>最大值</label>
          <input v-model.number="settingsMax" type="number" />
        </div>
        <div class="sticky-note-slider__settings-row">
          <label>步进</label>
          <input v-model.number="settingsStep" type="number" min="0.01" step="0.01" />
        </div>
        <button class="sticky-note-slider__settings-save" @click="saveSettings">
          保存
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sticky-note-slider {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px;
}

.sticky-note-slider__main {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
}

.sticky-note-slider__range {
  width: 100%;
  height: 8px;
  -webkit-appearance: none;
  appearance: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  outline: none;
  cursor: pointer;
}

.sticky-note-slider__range::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 20px;
  height: 20px;
  background: rgba(0, 0, 0, 0.6);
  border-radius: 50%;
  cursor: pointer;
}

.sticky-note-slider__range::-moz-range-thumb {
  width: 20px;
  height: 20px;
  background: rgba(0, 0, 0, 0.6);
  border-radius: 50%;
  cursor: pointer;
  border: none;
}

.sticky-note-slider__value-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.sticky-note-slider__min,
.sticky-note-slider__max {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.5);
  min-width: 30px;
}

.sticky-note-slider__max {
  text-align: right;
}

.sticky-note-slider__value-input {
  width: 80px;
  padding: 8px;
  text-align: center;
  font-size: 18px;
  font-weight: bold;
  border: 2px solid rgba(0, 0, 0, 0.2);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.5);
  color: rgba(0, 0, 0, 0.85);
}

.sticky-note-slider__value-input:focus {
  outline: none;
  border-color: rgba(0, 0, 0, 0.4);
}

.sticky-note-slider__bar {
  height: 8px;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  overflow: hidden;
}

.sticky-note-slider__fill {
  height: 100%;
  background: rgba(0, 0, 0, 0.4);
  border-radius: 4px;
  transition: width 0.1s;
}

.sticky-note-slider__settings-trigger {
  position: relative;
  height: 24px;
  margin-top: 8px;
}

.sticky-note-slider__settings-hint {
  text-align: center;
  font-size: 11px;
  color: rgba(0, 0, 0, 0.4);
  cursor: pointer;
}

.sticky-note-slider__settings-panel {
  position: absolute;
  bottom: 100%;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid rgba(0, 0, 0, 0.2);
  border-radius: 8px;
  padding: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  z-index: 10;
  min-width: 160px;
}

.sticky-note-slider__settings-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  gap: 8px;
}

.sticky-note-slider__settings-row label {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-slider__settings-row input {
  width: 70px;
  padding: 4px 8px;
  border: 1px solid rgba(0, 0, 0, 0.2);
  border-radius: 4px;
  font-size: 12px;
}

.sticky-note-slider__settings-save {
  width: 100%;
  padding: 6px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-slider__settings-save:hover {
  background: rgba(0, 0, 0, 0.15);
}
</style>
