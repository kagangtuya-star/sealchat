<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NCheckbox, NInputNumber, NSelect, NSwitch } from 'naive-ui'

import {
  normalizeStageEntranceConfig,
  type StageEntrancePreset,
} from '../shared/stage-types'
import {
  resolveImageObjectPreset,
  type TheaterImageObjectPreset,
} from './theater-image-folder-preset'

const props = defineProps<{
  preset?: TheaterImageObjectPreset
  inheritedPreset?: TheaterImageObjectPreset
  mode?: 'folder' | 'asset'
}>()

const emit = defineEmits<{
  save: [preset: TheaterImageObjectPreset | null]
  clear: []
}>()

const assetMode = computed(() => props.mode === 'asset')
const width = ref<number | null>(null)
const height = ref<number | null>(null)
const visible = ref(true)
const interactive = ref(true)
const editable = ref(false)
const locked = ref(false)
const aspectRatioLocked = ref(true)
const entrancePreset = ref<StageEntrancePreset>('none')
const entranceDurationMs = ref(400)
const widthOverridden = ref(false)
const heightOverridden = ref(false)
const visibleOverridden = ref(false)
const interactiveOverridden = ref(false)
const editableOverridden = ref(false)
const lockedOverridden = ref(false)
const aspectRatioLockedOverridden = ref(false)
const entranceOverridden = ref(false)

const entranceOptions: Array<{ label: string, value: StageEntrancePreset }> = [
  { label: '无', value: 'none' },
  { label: '淡入淡出', value: 'fade' },
  { label: '滑入滑出', value: 'slide' },
  { label: '缩放', value: 'zoom' },
  { label: '遮罩', value: 'mask' },
]

watch([() => props.preset, () => props.inheritedPreset, () => props.mode], ([preset]) => {
  const resolved = resolveImageObjectPreset(assetMode.value ? props.inheritedPreset : undefined, preset)
  const entrance = normalizeStageEntranceConfig(resolved?.entrance)
  width.value = resolved?.width ?? null
  height.value = resolved?.height ?? null
  visible.value = resolved?.visible ?? true
  interactive.value = resolved?.interactive ?? true
  editable.value = resolved?.editable ?? false
  locked.value = resolved?.locked ?? false
  aspectRatioLocked.value = resolved?.aspectRatioLocked ?? true
  entrancePreset.value = entrance.preset
  entranceDurationMs.value = entrance.durationMs
  widthOverridden.value = preset?.width !== undefined
  heightOverridden.value = preset?.height !== undefined
  visibleOverridden.value = preset?.visible !== undefined
  interactiveOverridden.value = preset?.interactive !== undefined
  editableOverridden.value = preset?.editable !== undefined
  lockedOverridden.value = preset?.locked !== undefined
  aspectRatioLockedOverridden.value = preset?.aspectRatioLocked !== undefined
  entranceOverridden.value = preset?.entrance !== undefined
}, { immediate: true })

const inherited = () => resolveImageObjectPreset(props.inheritedPreset, undefined)
watch(widthOverridden, (enabled) => {
  if (assetMode.value && !enabled) width.value = inherited()?.width ?? null
})
watch(heightOverridden, (enabled) => {
  if (assetMode.value && !enabled) height.value = inherited()?.height ?? null
})
watch(visibleOverridden, (enabled) => {
  if (assetMode.value && !enabled) visible.value = inherited()?.visible ?? true
})
watch(interactiveOverridden, (enabled) => {
  if (assetMode.value && !enabled) interactive.value = inherited()?.interactive ?? true
})
watch(editableOverridden, (enabled) => {
  if (assetMode.value && !enabled) editable.value = inherited()?.editable ?? false
})
watch(lockedOverridden, (enabled) => {
  if (assetMode.value && !enabled) locked.value = inherited()?.locked ?? false
})
watch(aspectRatioLockedOverridden, (enabled) => {
  if (assetMode.value && !enabled) aspectRatioLocked.value = inherited()?.aspectRatioLocked ?? true
})
watch(entranceOverridden, (enabled) => {
  if (!assetMode.value || enabled) return
  const value = normalizeStageEntranceConfig(inherited()?.entrance)
  entrancePreset.value = value.preset
  entranceDurationMs.value = value.durationMs
})

const save = () => {
  const entrance = normalizeStageEntranceConfig({ preset: entrancePreset.value, durationMs: entranceDurationMs.value })
  if (!assetMode.value) {
    emit('save', {
      version: 1,
      ...(typeof width.value === 'number' ? { width: width.value } : {}),
      ...(typeof height.value === 'number' ? { height: height.value } : {}),
      visible: visible.value,
      interactive: interactive.value,
      editable: editable.value,
      locked: locked.value,
      aspectRatioLocked: aspectRatioLocked.value,
      entrance,
    })
    return
  }
  const preset: TheaterImageObjectPreset = { version: 1 }
  if (widthOverridden.value && typeof width.value === 'number') preset.width = width.value
  if (heightOverridden.value && typeof height.value === 'number') preset.height = height.value
  if (visibleOverridden.value) preset.visible = visible.value
  if (interactiveOverridden.value) preset.interactive = interactive.value
  if (editableOverridden.value) preset.editable = editable.value
  if (lockedOverridden.value) preset.locked = locked.value
  if (aspectRatioLockedOverridden.value) preset.aspectRatioLocked = aspectRatioLocked.value
  if (entranceOverridden.value) preset.entrance = entrance
  emit('save', Object.keys(preset).length > 1 ? preset : null)
}
</script>

<template>
  <div class="theater-image-folder-preset-editor">
    <header>
      <strong>{{ assetMode ? '素材预设' : '图片文件夹预设' }}</strong>
      <small v-if="assetMode">未覆盖属性继承当前文件夹预设；未分类素材使用原始默认值</small>
      <small v-else>仅影响以后从此文件夹拖出的组件</small>
    </header>
    <div class="theater-image-folder-preset-editor__dimensions">
      <label><span><n-checkbox v-if="assetMode" v-model:checked="widthOverridden" size="small">覆盖</n-checkbox> 宽</span><n-input-number v-model:value="width" :disabled="assetMode && !widthOverridden" :min="0.5" :max="10000" :precision="2" clearable /></label>
      <label><span><n-checkbox v-if="assetMode" v-model:checked="heightOverridden" size="small">覆盖</n-checkbox> 高</span><n-input-number v-model:value="height" :disabled="assetMode && !heightOverridden" :min="0.5" :max="10000" :precision="2" clearable /></label>
    </div>
    <div class="theater-image-folder-preset-editor__switches">
      <label><span><n-checkbox v-if="assetMode" v-model:checked="visibleOverridden" size="small" />显示</span><n-switch v-model:value="visible" :disabled="assetMode && !visibleOverridden" size="small" /></label>
      <label><span><n-checkbox v-if="assetMode" v-model:checked="interactiveOverridden" size="small" />可交互</span><n-switch v-model:value="interactive" :disabled="assetMode && !interactiveOverridden" size="small" /></label>
      <label><span><n-checkbox v-if="assetMode" v-model:checked="editableOverridden" size="small" />可编辑</span><n-switch v-model:value="editable" :disabled="assetMode && !editableOverridden" size="small" /></label>
      <label><span><n-checkbox v-if="assetMode" v-model:checked="lockedOverridden" size="small" />锁定位置</span><n-switch v-model:value="locked" :disabled="assetMode && !lockedOverridden" size="small" /></label>
      <label><span><n-checkbox v-if="assetMode" v-model:checked="aspectRatioLockedOverridden" size="small" />锁定比例</span><n-switch v-model:value="aspectRatioLocked" :disabled="assetMode && !aspectRatioLockedOverridden" size="small" /></label>
    </div>
    <label class="theater-image-folder-preset-editor__field"><span><n-checkbox v-if="assetMode" v-model:checked="entranceOverridden" size="small">覆盖</n-checkbox> 显隐动画</span><n-select v-model:value="entrancePreset" :disabled="assetMode && !entranceOverridden" :options="entranceOptions" size="small" /></label>
    <label class="theater-image-folder-preset-editor__field"><span>动画时长</span><n-input-number v-model:value="entranceDurationMs" :disabled="assetMode && !entranceOverridden" :min="150" :max="5000" :step="50" size="small"><template #suffix>ms</template></n-input-number></label>
    <small class="theater-image-folder-preset-editor__hint">动态图片继续遵循现有 runtime 显隐行为。</small>
    <footer>
      <n-button size="small" quaternary type="error" :disabled="!preset" @click="emit('clear')">{{ assetMode ? '全部恢复继承' : '清除预设' }}</n-button>
      <n-button size="small" type="primary" @click="save">保存</n-button>
    </footer>
  </div>
</template>

<style scoped>
.theater-image-folder-preset-editor { width: 278px; display: grid; gap: 11px; color: var(--sc-text-primary); }
.theater-image-folder-preset-editor header { display: grid; gap: 2px; }
.theater-image-folder-preset-editor header small, .theater-image-folder-preset-editor__hint { color: var(--sc-text-secondary); font-size: 10px; }
.theater-image-folder-preset-editor__dimensions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.theater-image-folder-preset-editor__dimensions label, .theater-image-folder-preset-editor__field { display: grid; gap: 4px; font-size: 11px; }
.theater-image-folder-preset-editor__switches { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 12px; }
.theater-image-folder-preset-editor__switches label { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 11px; }
.theater-image-folder-preset-editor footer { display: flex; justify-content: space-between; gap: 8px; padding-top: 2px; border-top: 1px solid var(--theater-border); }
</style>
