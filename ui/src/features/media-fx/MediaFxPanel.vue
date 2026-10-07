<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NCheckbox, NSlider } from 'naive-ui'

import {
  createDefaultMediaFxAdvanced,
  createDefaultMediaFxFilter,
  createDefaultMediaFxSpec,
  MEDIA_FX_ADVANCED_RANGES,
  MEDIA_FX_FILTER_RANGES,
  MEDIA_FX_INTENSITY_RANGE,
  matchMediaFxAdvancedPreset,
  matchMediaFxFilterPreset,
  mediaFxAdvancedFromPreset,
  mediaFxAdvancedHasContent,
  mediaFxAdvancedPresets,
  mediaFxDurationForSpeed,
  mediaFxFilterFromPreset,
  mediaFxFilterHasContent,
  mediaFxFilterPresets,
  mediaFxHasContent,
  mediaFxMotionSpeed,
  normalizeMediaFxSpec,
  type MediaFxAdvanced,
  type MediaFxAdvancedKey,
  type MediaFxCapabilities,
  type MediaFxFilter,
  type MediaFxFilterKey,
  type MediaFxMotion,
  type MediaFxMotionPreset,
  type MediaFxSpec,
} from './media-fx'

// Shared editor for MediaFxSpec. It knows nothing about stage objects, chat messages
// or stores: callers own persistence and decide capabilities per render target.
const props = withDefaults(defineProps<{
  modelValue: MediaFxSpec | null | undefined
  mode?: 'live' | 'bake'
  capabilities?: Partial<MediaFxCapabilities>
  disabled?: boolean
  // Shows the local "pause preview" toggle; the paused state is never persisted.
  previewControl?: boolean
  previewPaused?: boolean
}>(), {
  mode: 'live',
  capabilities: () => ({}),
  disabled: false,
  previewControl: false,
  previewPaused: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: MediaFxSpec]
  'update:previewPaused': [value: boolean]
  'edit-start': []
  'edit-end': []
  reset: []
}>()

const spec = computed(() => normalizeMediaFxSpec(props.modelValue))
const isAnimatedMedia = computed(() => props.capabilities.animatedMedia === true)
const filtersAvailable = computed(() => props.capabilities.filters !== false && !isAnimatedMedia.value)
const motionVisible = computed(() => props.mode === 'live' && props.capabilities.motion !== false)
const activeFilterPreset = computed(() => matchMediaFxFilterPreset(spec.value.filter))
const filterChanged = computed(() => mediaFxFilterHasContent(spec.value.filter))
const anyChanged = computed(() => mediaFxHasContent(spec.value))
const advancedChanged = computed(() => mediaFxAdvancedHasContent(spec.value.advanced))
const advancedAvailable = computed(() => props.capabilities.advanced === true && !isAnimatedMedia.value)
// Consumers without advanced rendering never see the section unless the data already
// carries advanced values (e.g. imported); it is then shown read-only and kept as is.
const advancedVisible = computed(() => advancedAvailable.value || advancedChanged.value)
const activeAdvancedPreset = computed(() => matchMediaFxAdvancedPreset(spec.value.advanced))

const filterRows: Array<{ key: MediaFxFilterKey, label: string }> = [
  { key: 'brightness', label: '亮度' },
  { key: 'contrast', label: '对比度' },
  { key: 'saturation', label: '饱和度' },
  { key: 'grayscale', label: '灰度' },
  { key: 'sepia', label: '褐色' },
  { key: 'hueRotate', label: '色相' },
  { key: 'blurPx', label: '模糊' },
]

const advancedRows: Array<{ key: MediaFxAdvancedKey, label: string }> = [
  { key: 'pixelate', label: '像素化' },
  { key: 'rgbSplit', label: '色差' },
  { key: 'scanline', label: '扫描线' },
]

const motionOptions: Array<{ value: MediaFxMotionPreset, label: string }> = [
  { value: 'none', label: '正常' },
  { value: 'shake', label: '震动' },
  { value: 'shake-x', label: '横震' },
  { value: 'shake-y', label: '纵震' },
  { value: 'breathe', label: '呼吸' },
  { value: 'float', label: '漂浮' },
  { value: 'drift', label: '漂移' },
  { value: 'zoom', label: '缩放' },
]

const SPEED_MIN = 0.25
const SPEED_MAX = 3

const formatFilterValue = (key: MediaFxFilterKey, value: number) => {
  if (key === 'hueRotate') return `${Math.round(value)}°`
  if (key === 'blurPx') return `${value.toFixed(1)}px`
  return `${Math.round(value * 100)}%`
}

const emitSpec = (next: MediaFxSpec) => {
  if (props.disabled) return
  emit('update:modelValue', normalizeMediaFxSpec(next))
}

const updateFilter = (patch: Partial<MediaFxFilter>) => {
  if (!filtersAvailable.value) return
  emitSpec({ ...spec.value, filter: { ...spec.value.filter, ...patch } })
}

const updateFilterValue = (key: MediaFxFilterKey, value: number | number[]) => {
  updateFilter({ [key]: Array.isArray(value) ? value[0] : value })
}

const resetFilterValue = (key: MediaFxFilterKey) => updateFilter({ [key]: MEDIA_FX_FILTER_RANGES[key].defaultValue })

const applyFilterPreset = (presetId: string) => {
  const filter = mediaFxFilterFromPreset(presetId)
  if (filter) updateFilter(filter)
}

const resetFilter = () => updateFilter(createDefaultMediaFxFilter())

const updateAdvanced = (patch: Partial<MediaFxAdvanced>) => {
  if (!advancedAvailable.value) return
  emitSpec({ ...spec.value, advanced: { ...spec.value.advanced, ...patch } })
}

const updateAdvancedValue = (key: MediaFxAdvancedKey, value: number | number[]) => {
  updateAdvanced({ [key]: Array.isArray(value) ? value[0] : value })
}

const resetAdvancedValue = (key: MediaFxAdvancedKey) => updateAdvanced({ [key]: MEDIA_FX_ADVANCED_RANGES[key].defaultValue })

const applyAdvancedPreset = (presetId: string) => {
  const advanced = mediaFxAdvancedFromPreset(presetId)
  if (advanced) updateAdvanced(advanced)
}

const resetAdvanced = () => updateAdvanced(createDefaultMediaFxAdvanced())

const updateMotion = (patch: Partial<MediaFxMotion>) => {
  emitSpec({ ...spec.value, motion: { ...spec.value.motion, ...patch } })
}

const selectMotionPreset = (preset: MediaFxMotionPreset) => {
  if (preset === spec.value.motion.preset) return
  // Keep the user's relative speed when switching between presets.
  const speed = spec.value.motion.preset === 'none' ? 1 : mediaFxMotionSpeed(spec.value.motion)
  updateMotion({ preset, durationMs: mediaFxDurationForSpeed(preset, speed) })
}

const motionSpeed = computed(() => Math.min(SPEED_MAX, Math.max(SPEED_MIN, mediaFxMotionSpeed(spec.value.motion))))

const updateMotionIntensity = (value: number | number[]) => updateMotion({ intensity: Array.isArray(value) ? value[0] : value })
const updateMotionSpeed = (value: number | number[]) => {
  const speed = Array.isArray(value) ? value[0] : value
  updateMotion({ durationMs: mediaFxDurationForSpeed(spec.value.motion.preset, speed) })
}

const resetAll = () => {
  emitSpec(createDefaultMediaFxSpec())
  emit('reset')
}
</script>

<template>
  <div class="media-fx-panel" :class="{ 'is-disabled': disabled }">
    <section class="media-fx-panel__section">
      <header class="media-fx-panel__heading">
        <span>风格预设</span>
      </header>
      <div class="media-fx-panel__chips" role="group" aria-label="图像风格预设">
        <button
          v-for="preset in mediaFxFilterPresets"
          :key="preset.id"
          type="button"
          class="media-fx-panel__chip"
          :class="{ 'is-active': activeFilterPreset === preset.id }"
          :disabled="disabled || !filtersAvailable"
          :aria-pressed="activeFilterPreset === preset.id"
          @click="applyFilterPreset(preset.id)"
        >{{ preset.label }}</button>
      </div>
    </section>

    <section class="media-fx-panel__section">
      <header class="media-fx-panel__heading">
        <span>图像调整</span>
        <n-button text size="tiny" :disabled="disabled || !filtersAvailable || !filterChanged" @click="resetFilter">重置图像调整</n-button>
      </header>
      <p v-if="isAnimatedMedia" class="media-fx-panel__hint">动态媒体暂不支持实时图像滤镜</p>
      <div class="media-fx-panel__rows">
        <div v-for="row in filterRows" :key="row.key" class="media-fx-panel__row">
          <span class="media-fx-panel__label" title="双击恢复默认" @dblclick="resetFilterValue(row.key)">{{ row.label }}</span>
          <n-slider
            :value="spec.filter[row.key]"
            :min="MEDIA_FX_FILTER_RANGES[row.key].min"
            :max="MEDIA_FX_FILTER_RANGES[row.key].max"
            :step="MEDIA_FX_FILTER_RANGES[row.key].step"
            :tooltip="false"
            :disabled="disabled || !filtersAvailable"
            @dragstart="emit('edit-start')"
            @dragend="emit('edit-end')"
            @update:value="updateFilterValue(row.key, $event)"
          />
          <span class="media-fx-panel__value">{{ formatFilterValue(row.key, spec.filter[row.key]) }}</span>
          <button
            type="button"
            class="media-fx-panel__reset"
            :class="{ 'is-hidden': spec.filter[row.key] === MEDIA_FX_FILTER_RANGES[row.key].defaultValue }"
            :disabled="disabled || !filtersAvailable"
            :aria-label="`恢复${row.label}默认值`"
            title="恢复默认"
            @click="resetFilterValue(row.key)"
          >↺</button>
        </div>
      </div>
    </section>

    <section v-if="advancedVisible" class="media-fx-panel__section">
      <header class="media-fx-panel__heading">
        <span>高级效果</span>
        <n-button text size="tiny" :disabled="disabled || !advancedAvailable || !advancedChanged" @click="resetAdvanced">重置高级效果</n-button>
      </header>
      <p v-if="!advancedAvailable" class="media-fx-panel__hint">当前媒体暂不支持高级效果</p>
      <div class="media-fx-panel__chips" role="group" aria-label="高级效果预设">
        <button
          v-for="preset in mediaFxAdvancedPresets"
          :key="preset.id"
          type="button"
          class="media-fx-panel__chip"
          :class="{ 'is-active': activeAdvancedPreset === preset.id }"
          :disabled="disabled || !advancedAvailable"
          :aria-pressed="activeAdvancedPreset === preset.id"
          @click="applyAdvancedPreset(preset.id)"
        >{{ preset.label }}</button>
      </div>
      <div class="media-fx-panel__rows">
        <div v-for="row in advancedRows" :key="row.key" class="media-fx-panel__row">
          <span class="media-fx-panel__label" title="双击恢复默认" @dblclick="resetAdvancedValue(row.key)">{{ row.label }}</span>
          <n-slider
            :value="spec.advanced[row.key]"
            :min="MEDIA_FX_ADVANCED_RANGES[row.key].min"
            :max="MEDIA_FX_ADVANCED_RANGES[row.key].max"
            :step="MEDIA_FX_ADVANCED_RANGES[row.key].step"
            :tooltip="false"
            :disabled="disabled || !advancedAvailable"
            @dragstart="emit('edit-start')"
            @dragend="emit('edit-end')"
            @update:value="updateAdvancedValue(row.key, $event)"
          />
          <span class="media-fx-panel__value">{{ Math.round(spec.advanced[row.key] * 100) }}%</span>
          <button
            type="button"
            class="media-fx-panel__reset"
            :class="{ 'is-hidden': spec.advanced[row.key] === MEDIA_FX_ADVANCED_RANGES[row.key].defaultValue }"
            :disabled="disabled || !advancedAvailable"
            :aria-label="`恢复${row.label}默认值`"
            title="恢复默认"
            @click="resetAdvancedValue(row.key)"
          >↺</button>
        </div>
      </div>
    </section>

    <section v-if="motionVisible" class="media-fx-panel__section">
      <header class="media-fx-panel__heading">
        <span>图像动效</span>
        <n-button
          v-if="previewControl"
          text
          size="tiny"
          :disabled="spec.motion.preset === 'none'"
          @click="emit('update:previewPaused', !previewPaused)"
        >{{ previewPaused ? '继续预览' : '暂停预览' }}</n-button>
      </header>
      <div class="media-fx-panel__chips" role="group" aria-label="图像动效">
        <button
          v-for="option in motionOptions"
          :key="option.value"
          type="button"
          class="media-fx-panel__chip"
          :class="{ 'is-active': spec.motion.preset === option.value }"
          :disabled="disabled"
          :aria-pressed="spec.motion.preset === option.value"
          @click="selectMotionPreset(option.value)"
        >{{ option.label }}</button>
      </div>
      <div v-if="spec.motion.preset !== 'none'" class="media-fx-panel__rows">
        <div class="media-fx-panel__row">
          <span class="media-fx-panel__label">强度</span>
          <n-slider
            :value="spec.motion.intensity"
            :min="MEDIA_FX_INTENSITY_RANGE.min"
            :max="MEDIA_FX_INTENSITY_RANGE.max"
            :step="MEDIA_FX_INTENSITY_RANGE.step"
            :tooltip="false"
            :disabled="disabled"
            @dragstart="emit('edit-start')"
            @dragend="emit('edit-end')"
            @update:value="updateMotionIntensity"
          />
          <span class="media-fx-panel__value">{{ Math.round(spec.motion.intensity * 100) }}%</span>
          <span />
        </div>
        <div class="media-fx-panel__row">
          <span class="media-fx-panel__label">速度</span>
          <n-slider
            :value="motionSpeed"
            :min="SPEED_MIN"
            :max="SPEED_MAX"
            :step="0.05"
            :tooltip="false"
            :disabled="disabled"
            @dragstart="emit('edit-start')"
            @dragend="emit('edit-end')"
            @update:value="updateMotionSpeed"
          />
          <span class="media-fx-panel__value">{{ motionSpeed.toFixed(2) }}×</span>
          <span />
        </div>
        <n-checkbox
          size="small"
          class="media-fx-panel__loop"
          :checked="spec.motion.loop"
          :disabled="disabled"
          @update:checked="updateMotion({ loop: $event })"
        >循环播放</n-checkbox>
      </div>
    </section>

    <footer class="media-fx-panel__footer">
      <n-button size="tiny" secondary :disabled="disabled || !anyChanged" @click="resetAll">全部恢复正常</n-button>
    </footer>
  </div>
</template>

<style scoped>
.media-fx-panel {
  --media-fx-accent: var(--primary-color, #3388de);
  --media-fx-line: color-mix(in srgb, currentColor 18%, transparent);
  display: grid;
  gap: 10px;
  min-width: 0;
  font-size: 12px;
  color: inherit;
}
.media-fx-panel.is-disabled { opacity: .6; }
.media-fx-panel__section { display: grid; gap: 6px; min-width: 0; }
.media-fx-panel__heading { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-weight: 600; opacity: .85; }
.media-fx-panel__hint { margin: 0; font-size: 11px; opacity: .7; }
.media-fx-panel__chips { display: grid; grid-template-columns: repeat(auto-fill, minmax(52px, 1fr)); gap: 4px; }
.media-fx-panel__chip {
  min-width: 0;
  padding: 4px 6px;
  overflow: hidden;
  border: 1px solid var(--media-fx-line);
  border-radius: 6px;
  color: inherit;
  background: transparent;
  font: inherit;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}
.media-fx-panel__chip:hover:not(:disabled) { border-color: color-mix(in srgb, var(--media-fx-accent) 60%, transparent); }
.media-fx-panel__chip.is-active { border-color: var(--media-fx-accent); background: color-mix(in srgb, var(--media-fx-accent) 18%, transparent); }
.media-fx-panel__chip:disabled { cursor: not-allowed; opacity: .5; }
.media-fx-panel__rows { display: grid; gap: 2px; }
.media-fx-panel__row { display: grid; grid-template-columns: 3.4em minmax(0, 1fr) 3.6em 18px; align-items: center; gap: 6px; min-width: 0; }
.media-fx-panel__label { overflow: hidden; opacity: .75; white-space: nowrap; user-select: none; }
.media-fx-panel__value { font-variant-numeric: tabular-nums; opacity: .75; text-align: right; white-space: nowrap; }
.media-fx-panel__reset { width: 18px; height: 18px; padding: 0; border: 0; border-radius: 4px; color: inherit; background: transparent; line-height: 18px; opacity: .7; cursor: pointer; }
.media-fx-panel__reset:hover:not(:disabled) { opacity: 1; background: var(--media-fx-line); }
.media-fx-panel__reset.is-hidden { visibility: hidden; }
.media-fx-panel__loop { margin-top: 2px; }
.media-fx-panel__footer { display: flex; justify-content: flex-end; }
</style>
