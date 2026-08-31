<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NColorPicker, NInput, NInputNumber, NSelect, NSlider, NSwitch } from 'naive-ui'

import type { TheaterImageAsset } from '../effects/theater-image-assets'
import {
  normalizeStageSceneOverlays,
  stageSceneOverlayBlendModes,
  type StageSceneOverlayBinding,
  type StageSceneOverlayBlendMode,
  type StageSceneOverlayParamValue,
} from '../shared/stage-types'
import type { SceneOverlayControl, SceneOverlayEffectDefinition } from './scene-overlay-types'

const props = defineProps<{
  binding: StageSceneOverlayBinding
  definition?: SceneOverlayEffectDefinition
  mediaAsset?: TheaterImageAsset
  canEdit: boolean
}>()

const emit = defineEmits<{
  update: [binding: StageSceneOverlayBinding]
  remove: []
  replaceMedia: []
}>()

const blendModeLabels: Record<StageSceneOverlayBlendMode, string> = {
  normal: '正常',
  multiply: '正片叠底',
  screen: '滤色',
  overlay: '叠加',
  darken: '变暗',
  lighten: '变亮',
  'color-dodge': '颜色减淡',
  'color-burn': '颜色加深',
  'hard-light': '强光',
  'soft-light': '柔光',
}
const blendModeOptions = stageSceneOverlayBlendModes.map((value) => ({ label: blendModeLabels[value], value }))

const patchBinding = (patch: Partial<StageSceneOverlayBinding>) => {
  if (!props.canEdit) return
  const normalized = normalizeStageSceneOverlays([{ ...props.binding, ...patch }])[0]
  if (normalized) emit('update', normalized)
}

const numberControlValue = (control: Extract<SceneOverlayControl, { type: 'number' | 'angle' }>) => {
  const value = props.binding.params[control.key]
  const fallback = props.definition?.defaultParams[control.key]
  return typeof value === 'number' ? value : typeof fallback === 'number' ? fallback : control.min
}

const controlValue = (control: SceneOverlayControl) => {
  const value = props.binding.params[control.key]
  return value === undefined ? props.definition?.defaultParams[control.key] ?? null : value
}

const normalizeControlValue = (control: SceneOverlayControl, value: StageSceneOverlayParamValue) => {
  if (control.type === 'number' || control.type === 'angle') {
    const numeric = typeof value === 'number' && Number.isFinite(value) ? value : numberControlValue(control)
    const stepped = control.min + Math.round((numeric - control.min) / control.step) * control.step
    return Number(Math.min(control.max, Math.max(control.min, stepped)).toFixed(6))
  }
  if (control.type === 'boolean') return value === true
  if (control.type === 'select') {
    const selected = typeof value === 'string' && control.options.some((option) => option.value === value)
      ? value
      : control.options[0]?.value || ''
    return selected
  }
  const fallback = props.definition?.defaultParams[control.key]
  return typeof value === 'string' && /^#[0-9a-f]{6}$/i.test(value)
    ? value
    : typeof fallback === 'string' ? fallback : '#ffffff'
}

const updateParam = (control: SceneOverlayControl, value: StageSceneOverlayParamValue) => {
  patchBinding({
    params: {
      ...props.binding.params,
      [control.key]: normalizeControlValue(control, value),
    },
  })
}

const title = computed(() => props.definition?.name || '未知效果')
const mediaTypeLabel = computed(() => props.binding.media?.animated === true || props.binding.media?.mimeType === 'video/webm'
  ? '动态素材'
  : '静态素材')
</script>

<template>
  <aside class="scene-overlay-inspector">
    <div class="scene-overlay-inspector__title">
      <strong>{{ title }}</strong>
      <small>{{ binding.effectId }}</small>
    </div>

    <div class="scene-overlay-inspector__form">
      <label>名称</label>
      <n-input :value="binding.name" size="small" maxlength="128" :disabled="!canEdit" @update:value="patchBinding({ name: $event })" />

      <label>启用</label>
      <n-switch :value="binding.enabled" size="small" :disabled="!canEdit" @update:value="patchBinding({ enabled: $event })" />

      <label>透明度</label>
      <div class="scene-overlay-inspector__slider">
        <n-slider :value="binding.opacity" :min="0" :max="1" :step="0.01" :disabled="!canEdit" @update:value="patchBinding({ opacity: $event })" />
        <output>{{ Math.round(binding.opacity * 100) }}%</output>
      </div>

      <label>混合模式</label>
      <n-select :value="binding.blendMode" :options="blendModeOptions" size="small" :disabled="!canEdit" @update:value="patchBinding({ blendMode: $event })" />

      <label>覆盖角色</label>
      <n-switch :value="binding.layer === 'aboveCharacters'" size="small" :disabled="!canEdit" @update:value="patchBinding({ layer: $event ? 'aboveCharacters' : 'belowCharacters' })" />
    </div>

    <div v-if="binding.media" class="scene-overlay-inspector__parameters">
      <h3>素材</h3>
      <div class="scene-overlay-inspector__form">
        <label>当前素材</label>
        <strong class="scene-overlay-inspector__media-name">{{ mediaAsset?.name || '资源已绑定' }}</strong>
        <label>类型</label>
        <span class="scene-overlay-inspector__media-type">{{ mediaTypeLabel }}</span>
        <span />
        <n-button size="small" secondary :disabled="!canEdit" @click="emit('replaceMedia')">更换素材</n-button>
      </div>
    </div>

    <div v-if="definition" class="scene-overlay-inspector__parameters">
      <h3>效果参数</h3>
      <div class="scene-overlay-inspector__form">
        <template v-for="control in definition.controls" :key="control.key">
          <label>{{ control.label }}</label>
          <div v-if="control.type === 'number'" class="scene-overlay-inspector__number">
            <n-input-number
              :value="numberControlValue(control)"
              :min="control.min"
              :max="control.max"
              :step="control.step"
              :disabled="!canEdit"
              size="small"
              @update:value="$event !== null && updateParam(control, $event)"
            />
            <small v-if="control.suffix">{{ control.suffix }}</small>
          </div>
          <div v-else-if="control.type === 'angle'" class="scene-overlay-inspector__angle">
            <div class="scene-overlay-inspector__angle-inputs">
              <n-slider
                :value="numberControlValue(control)"
                :min="control.min"
                :max="control.max"
                :step="control.step"
                :disabled="!canEdit"
                @update:value="updateParam(control, $event)"
              />
              <n-input-number
                :value="numberControlValue(control)"
                :min="control.min"
                :max="control.max"
                :step="control.step"
                :disabled="!canEdit"
                size="small"
                @update:value="$event !== null && updateParam(control, $event)"
              >
                <template #suffix>°</template>
              </n-input-number>
            </div>
            <small>0° →　90° ↓　180° ←　270° ↑</small>
          </div>
          <n-color-picker
            v-else-if="control.type === 'color'"
            :value="String(controlValue(control))"
            :show-alpha="false"
            :modes="['hex']"
            :disabled="!canEdit"
            size="small"
            @update:value="updateParam(control, $event)"
          />
          <n-switch
            v-else-if="control.type === 'boolean'"
            :value="controlValue(control) === true"
            :disabled="!canEdit"
            size="small"
            @update:value="updateParam(control, $event)"
          />
          <n-select
            v-else
            :value="String(controlValue(control))"
            :options="control.options"
            :disabled="!canEdit"
            size="small"
            @update:value="updateParam(control, $event)"
          />
        </template>
      </div>
    </div>
    <p v-else class="scene-overlay-inspector__unknown">此效果定义在当前版本不可用，配置数据已保留。</p>

    <n-button class="scene-overlay-inspector__remove" size="small" type="error" secondary :disabled="!canEdit" @click="emit('remove')">删除效果</n-button>
  </aside>
</template>

<style scoped>
.scene-overlay-inspector { min-width: 0; display: flex; flex-direction: column; overflow: auto; border-left: 1px solid var(--theater-border); }
.scene-overlay-inspector__title { display: grid; gap: 2px; padding: 9px 10px; border-bottom: 1px solid var(--theater-border); }
.scene-overlay-inspector__title strong { font-size: 12px; }
.scene-overlay-inspector__title small { overflow: hidden; color: var(--sc-text-secondary); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-inspector__form { display: grid; grid-template-columns: 82px minmax(0, 1fr); align-items: center; gap: 8px; padding: 10px; }
.scene-overlay-inspector__form > label { color: var(--sc-text-secondary); font-size: 11px; }
.scene-overlay-inspector__slider { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr) 38px; align-items: center; gap: 6px; }
.scene-overlay-inspector__slider output { color: var(--sc-text-secondary); font-size: 10px; text-align: right; }
.scene-overlay-inspector__parameters { border-top: 1px solid var(--theater-border); }
.scene-overlay-inspector__parameters h3 { margin: 9px 10px 0; color: var(--sc-text-secondary); font-size: 11px; }
.scene-overlay-inspector__number { min-width: 0; display: flex; align-items: center; gap: 5px; }
.scene-overlay-inspector__number :deep(.n-input-number) { min-width: 0; flex: 1; }
.scene-overlay-inspector__number small { color: var(--sc-text-secondary); font-size: 9px; white-space: nowrap; }
.scene-overlay-inspector__angle { min-width: 0; display: grid; gap: 3px; }
.scene-overlay-inspector__angle-inputs { min-width: 0; display: grid; grid-template-columns: minmax(80px, 1fr) 92px; align-items: center; gap: 7px; }
.scene-overlay-inspector__angle small { color: var(--sc-text-secondary); font-size: 9px; white-space: nowrap; }
.scene-overlay-inspector__media-name { overflow: hidden; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-inspector__media-type { color: var(--sc-text-secondary); font-size: 10px; }
.scene-overlay-inspector__unknown { margin: 10px; color: #fbbf24; font-size: 11px; line-height: 1.5; }
.scene-overlay-inspector__remove { align-self: flex-end; margin: auto 10px 10px; }
</style>
