<script lang="ts">
import type { StageAtomicAction } from '../shared/stage-types'

export const stageSequenceActionTypeOptions: Array<{ label: string, value: StageAtomicAction['type'] }> = [
  { label: '发送消息', value: 'chat.send' },
  { label: '随机表', value: 'chat.random-table' },
  { label: '插入输入框', value: 'chat.insert' },
  { label: '切换场景', value: 'scene.apply' },
  { label: '播放特效', value: 'effect.play' },
  { label: '线索', value: 'clue.execute' },
  { label: '切换组件显隐', value: 'object.toggle' },
]
</script>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { NButton, NIcon, NInput, NInputNumber } from 'naive-ui'
import { GripVertical, Trash } from '@vicons/tabler'
import NSelect from '@/components/NSelect.vue'
import {
  createStageAtomicActionDescriptor,
  createStageSequenceStep,
  STAGE_SEQUENCE_MAX_STEPS,
} from '../shared/stage-actions'
import {
  isStageActionTarget,
  type StageAtomicActionDescriptor,
  type StageObject,
  type StageScene,
  type StageSequenceStep,
} from '../shared/stage-types'
import { stageActionSchema, type ChatClueOptionsReadResult } from '../bridge/theater-bridge-protocol'
import { isTheaterEffectObject } from '../effects/theater-effect-types'

// Steps are edited in place, matching the existing auto-saving action editor contract.
const props = withDefaults(defineProps<{
  steps: StageSequenceStep[]
  scenes: StageScene[]
  persistentObjects: Record<string, StageObject>
  activeSceneId: string
  readClueOptions?: () => Promise<ChatClueOptionsReadResult>
  emptyText?: string
}>(), {
  emptyText: '暂无动作。添加后按顺序触发。',
})

const emit = defineEmits<{
  editRandomTable: [stepId: string]
  editClue: [stepId: string]
}>()

const actionTypeLabel = (type: StageAtomicAction['type']) => (
  type === 'object.trigger' ? '触发组件点击动作' : stageSequenceActionTypeOptions.find((option) => option.value === type)?.label || type
)
const isObjectTargetAction = (action: StageAtomicActionDescriptor): action is Extract<StageAtomicActionDescriptor, { type: 'object.toggle' | 'object.trigger' }> => (
  action.type === 'object.toggle' || action.type === 'object.trigger'
)
const timingOptions = [
  { label: '依次', value: 'after' },
  { label: '固定时延', value: 'delay' },
  { label: '同步', value: 'sync' },
]
const openSelectKey = ref('')
let openingSelectKey = ''
const sequenceSelectKey = (kind: string, stepId: string) => `${stepId}:${kind}`
const isSequenceSelectOpen = (kind: string, stepId: string) => openSelectKey.value === sequenceSelectKey(kind, stepId)
const updateSelectShow = (kind: string, step: StageSequenceStep, show: boolean) => {
  const key = sequenceSelectKey(kind, step.id)
  if (!show) {
    if (openingSelectKey !== key && openSelectKey.value === key) openSelectKey.value = ''
    return
  }
  openSelectKey.value = key
  openingSelectKey = key
  queueMicrotask(() => {
    if (openingSelectKey === key) openingSelectKey = ''
  })
}
const sequenceSelectMenuProps = {
  class: 'theater-sequence-select-menu',
}
const sceneOptions = computed(() => props.scenes.map((scene) => ({ label: scene.name, value: scene.id })))
const objectOptions = (sceneId: string | null) => {
  if (!sceneId) {
    return Object.values(props.persistentObjects)
      .filter((object) => object.type !== 'group')
      .map((object) => ({ label: `${object.name} · 跨场景`, value: object.id }))
  }
  const scene = props.scenes.find((item) => item.id === sceneId)
  return scene
    ? Object.values(scene.state.sceneObjects)
      .filter((object) => object.type !== 'group')
      .map((object) => ({ label: object.name, value: object.id }))
    : []
}

const isTriggerableObject = (object: StageObject) => (
  object.visible
  && object.interactive
  && isStageActionTarget(object.type)
  && object.actions.some((action) => stageActionSchema.safeParse(action).success)
)

const triggerObjectOptions = (sceneId: string | null) => {
  if (!sceneId) {
    return Object.values(props.persistentObjects)
      .filter(isTriggerableObject)
      .map((object) => ({ label: `${object.name} · 跨场景`, value: object.id }))
  }
  const scene = props.scenes.find((item) => item.id === sceneId)
  return scene
    ? Object.values(scene.state.sceneObjects)
      .filter(isTriggerableObject)
      .map((object) => ({ label: object.name, value: object.id }))
    : []
}

const objectTargetOptions = (type: StageAtomicAction['type'], sceneId: string | null) => (
  type === 'object.trigger' ? triggerObjectOptions(sceneId) : objectOptions(sceneId)
)

const effectOptions = (sceneId: string | null) => {
  if (!sceneId) {
    return Object.values(props.persistentObjects)
      .filter(isTheaterEffectObject)
      .map((object) => ({ label: `${object.name} · 跨场景`, value: object.id }))
  }
  const scene = props.scenes.find((item) => item.id === sceneId)
  return scene
    ? Object.values(scene.state.sceneObjects)
      .filter(isTheaterEffectObject)
      .map((object) => ({ label: object.name, value: object.id }))
    : []
}

const targetSceneOptions = (type: StageAtomicAction['type']) => {
  const persistentOptions = type === 'effect.play' ? effectOptions(null) : objectTargetOptions(type, null)
  return persistentOptions.length
    ? [...sceneOptions.value, { label: '跨场景', value: '' }]
    : sceneOptions.value
}

const objectSceneId = (objectId: string) => {
  if (props.persistentObjects[objectId]) return null
  return props.scenes.find((scene) => Boolean(scene.state.sceneObjects[objectId]))?.id || null
}

const addStep = async (type: StageAtomicAction['type']) => {
  const steps = props.steps
  if (steps.length >= STAGE_SEQUENCE_MAX_STEPS) return
  const sceneId = props.activeSceneId || props.scenes[0]?.id || ''
  const localTargetId = type === 'effect.play' ? effectOptions(sceneId)[0]?.value || '' : objectTargetOptions(type, sceneId)[0]?.value || ''
  const persistentTargetId = type === 'effect.play' ? effectOptions(null)[0]?.value || '' : objectTargetOptions(type, null)[0]?.value || ''
  const targetId = localTargetId || persistentTargetId
  const action = createStageAtomicActionDescriptor(type, sceneId, targetId)
  if ((type === 'effect.play' || isObjectTargetAction(action)) && !targetId) return
  if (type === 'clue.execute' && !props.readClueOptions) return
  const step = createStageSequenceStep(sceneId, targetId)
  step.action = action
  if (type === 'scene.apply') step.sceneId = sceneId || null
  else if (type === 'effect.play' || isObjectTargetAction(action)) step.sceneId = objectSceneId(targetId)
  else step.sceneId = null
  if (type === 'clue.execute' && props.readClueOptions) {
    let result: ChatClueOptionsReadResult
    try {
      result = await props.readClueOptions()
    } catch {
      return
    }
    if (props.steps !== steps) return
    if (steps.length >= STAGE_SEQUENCE_MAX_STEPS) return
    if (!result.ok || !result.clues.length) return
    step.action = createStageAtomicActionDescriptor(type, sceneId, result.clues[0].id)
  }
  steps.push(step)
  if (type === 'chat.random-table') emit('editRandomTable', step.id)
}

const removeStep = (stepId: string) => {
  const index = props.steps.findIndex((step) => step.id === stepId)
  if (index >= 0) props.steps.splice(index, 1)
}

const updateStepScene = (step: StageSequenceStep, sceneId: string) => {
  if (step.action.type === 'scene.apply') {
    step.sceneId = sceneId || null
    step.action.payload.sceneId = sceneId
    openSelectKey.value = ''
    return
  }
  if (!isObjectTargetAction(step.action) && step.action.type !== 'effect.play') {
    step.sceneId = null
    openSelectKey.value = ''
    return
  }
  const targetSceneId = sceneId || null
  const options = step.action.type === 'effect.play' ? effectOptions(targetSceneId) : objectTargetOptions(step.action.type, targetSceneId)
  if (!options.length) {
    openSelectKey.value = ''
    return
  }
  step.sceneId = targetSceneId
  if (isObjectTargetAction(step.action)) {
    const objectId = step.action.payload.objectId
    if (!options.some((option) => option.value === objectId)) {
      step.action.payload.objectId = options[0].value
    }
  } else {
    const effectId = step.action.payload.effectId
    if (!options.some((option) => option.value === effectId)) step.action.payload.effectId = options[0].value
  }
  openSelectKey.value = ''
}

const updateTiming = (step: StageSequenceStep, mode: 'after' | 'delay' | 'sync') => {
  step.timing = mode === 'delay' ? { mode, delayMs: 500 } : { mode }
  openSelectKey.value = ''
}

const updateStepObject = (step: StageSequenceStep, objectId: string) => {
  if (!isObjectTargetAction(step.action)) return
  step.action.payload.objectId = objectId
  step.sceneId = objectSceneId(objectId)
  openSelectKey.value = ''
}

const updateStepEffect = (step: StageSequenceStep, effectId: string) => {
  if (step.action.type !== 'effect.play') return
  step.action.payload.effectId = effectId
  step.sceneId = objectSceneId(effectId)
  openSelectKey.value = ''
}

const rowElements = new Map<string, HTMLElement>()
const draggingId = ref('')
const dropIndex = ref(-1)
let dragFrame = 0
let pendingY = 0

const setRowElement = (stepId: string, element: Element | null) => {
  if (element instanceof HTMLElement) rowElements.set(stepId, element)
  else rowElements.delete(stepId)
}

const updateDropIndex = () => {
  dragFrame = 0
  const rows = props.steps
    .map((step) => rowElements.get(step.id))
    .filter((element): element is HTMLElement => Boolean(element))
  let index = rows.length
  for (let current = 0; current < rows.length; current += 1) {
    const rect = rows[current].getBoundingClientRect()
    if (pendingY < rect.top + rect.height / 2) {
      index = current
      break
    }
  }
  dropIndex.value = index
}

const handlePointerMove = (event: PointerEvent) => {
  pendingY = event.clientY
  if (!dragFrame) dragFrame = requestAnimationFrame(updateDropIndex)
}

const stopDragging = () => {
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', stopDragging)
  window.removeEventListener('pointercancel', stopDragging)
  if (dragFrame) cancelAnimationFrame(dragFrame)
  dragFrame = 0
  const steps = props.steps
  const sourceId = draggingId.value
  const targetIndex = dropIndex.value
  draggingId.value = ''
  dropIndex.value = -1
  if (!sourceId || targetIndex < 0) return
  const sourceIndex = steps.findIndex((step) => step.id === sourceId)
  if (sourceIndex < 0) return
  const [step] = steps.splice(sourceIndex, 1)
  const adjusted = targetIndex > sourceIndex ? targetIndex - 1 : targetIndex
  steps.splice(Math.max(0, Math.min(adjusted, steps.length)), 0, step)
}

const startDragging = (stepId: string, event: PointerEvent) => {
  if (event.button !== 0) return
  event.preventDefault()
  draggingId.value = stepId
  pendingY = event.clientY
  updateDropIndex()
  window.addEventListener('pointermove', handlePointerMove)
  window.addEventListener('pointerup', stopDragging, { once: true })
  window.addEventListener('pointercancel', stopDragging, { once: true })
}

const moveStep = (stepId: string, offset: number) => {
  const index = props.steps.findIndex((step) => step.id === stepId)
  const target = index + offset
  if (index < 0 || target < 0 || target >= props.steps.length) return
  const [step] = props.steps.splice(index, 1)
  props.steps.splice(target, 0, step)
}

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', stopDragging)
  window.removeEventListener('pointercancel', stopDragging)
  if (dragFrame) cancelAnimationFrame(dragFrame)
})

defineExpose({ addStep })
</script>

<template>
  <div class="theater-sequence-grid theater-sequence-grid--heading">
    <span></span><span>操作内容 / 组件</span><span>动作类型</span><span>目标场景</span><span>时延</span><span></span>
  </div>
  <div class="theater-sequence-editor__rows">
    <div
      v-for="(step, index) in steps"
      :key="step.id"
      :ref="element => setRowElement(step.id, element as Element | null)"
      class="theater-sequence-grid theater-sequence-row"
      :class="{
        'is-dragging': draggingId === step.id,
        'is-drop-before': draggingId && dropIndex === index,
        'is-drop-after': draggingId && dropIndex === steps.length && index === steps.length - 1,
      }"
    >
      <button
        type="button"
        class="theater-sequence-row__handle"
        aria-label="拖动排序"
        @pointerdown="startDragging(step.id, $event)"
        @keydown.alt.up.prevent="moveStep(step.id, -1)"
        @keydown.alt.down.prevent="moveStep(step.id, 1)"
      ><n-icon><GripVertical /></n-icon></button>

      <n-input
        v-if="step.action.type === 'chat.send' || step.action.type === 'chat.insert'"
        v-model:value="step.action.payload.content"
        maxlength="10000"
        placeholder="输入文本"
      />
      <n-button v-else-if="step.action.type === 'chat.random-table'" secondary @click="emit('editRandomTable', step.id)">
        编辑随机表 · {{ step.action.payload.name }} · {{ step.action.payload.formula }} · {{ step.action.payload.entries.length }} 项
      </n-button>
      <n-select
        v-else-if="isObjectTargetAction(step.action)"
        :value="step.action.payload.objectId"
        :show="isSequenceSelectOpen('object', step.id)"
        :options="objectTargetOptions(step.action.type, step.sceneId)"
        filterable
        :menu-props="sequenceSelectMenuProps"
        placeholder="选择已配置点击动作的可交互组件"
        @update:show="updateSelectShow('object', step, $event)"
        @update:value="updateStepObject(step, $event)"
      />
      <n-select
        v-else-if="step.action.type === 'effect.play'"
        :value="step.action.payload.effectId"
        :show="isSequenceSelectOpen('effect', step.id)"
        :options="effectOptions(step.sceneId)"
        filterable
        :menu-props="sequenceSelectMenuProps"
        placeholder="选择特效"
        @update:show="updateSelectShow('effect', step, $event)"
        @update:value="updateStepEffect(step, $event)"
      />
      <n-button v-else-if="step.action.type === 'clue.execute'" secondary @click="emit('editClue', step.id)">
        编辑线索 · {{ step.action.payload.entries.length }} 条
      </n-button>
      <span v-else class="theater-sequence-row__operation">切换至所选场景</span>

      <span class="theater-sequence-row__type">{{ actionTypeLabel(step.action.type) }}</span>

      <n-select
        v-if="step.action.type === 'scene.apply'"
        :value="step.action.payload.sceneId"
        :show="isSequenceSelectOpen('scene', step.id)"
        :options="sceneOptions"
        filterable
        :menu-props="sequenceSelectMenuProps"
        placeholder="切换到场景"
        @update:show="updateSelectShow('scene', step, $event)"
        @update:value="updateStepScene(step, $event)"
      />
      <n-select
        v-else-if="isObjectTargetAction(step.action) || step.action.type === 'effect.play'"
        :value="step.sceneId || ''"
        :show="isSequenceSelectOpen('scene', step.id)"
        :options="targetSceneOptions(step.action.type)"
        filterable
        :menu-props="sequenceSelectMenuProps"
        placeholder="目标所属场景"
        @update:show="updateSelectShow('scene', step, $event)"
        @update:value="updateStepScene(step, $event)"
      />
      <span v-else class="theater-sequence-row__operation">无场景约束</span>

      <div class="theater-sequence-row__timing">
        <n-select
          :value="step.timing.mode"
          :show="isSequenceSelectOpen('timing', step.id)"
          :options="timingOptions"
          filterable
          :menu-props="sequenceSelectMenuProps"
          @update:show="updateSelectShow('timing', step, $event)"
          @update:value="updateTiming(step, $event as 'after' | 'delay' | 'sync')"
        />
        <n-input-number
          v-if="step.timing.mode === 'delay'"
          v-model:value="step.timing.delayMs"
          :min="0"
          :max="60000"
          :step="100"
          :precision="0"
          placeholder="毫秒"
        />
      </div>

      <n-button text type="error" aria-label="删除动作" @click="removeStep(step.id)"><n-icon><Trash /></n-icon></n-button>
    </div>
    <div v-if="!steps.length" class="theater-sequence-editor__empty">{{ emptyText }}</div>
  </div>
</template>

<style scoped>
.theater-sequence-grid { display: grid; grid-template-columns: 34px minmax(140px, .8fr) minmax(220px, 1.5fr) minmax(150px, 1fr) minmax(160px, .9fr) 34px; align-items: center; gap: 8px; }
.theater-sequence-grid--heading { padding: 0 8px 7px; color: var(--sc-text-secondary, #a1a1aa); font-size: 10px; }
.theater-sequence-editor__rows { min-height: 80px; overflow: auto; padding: 2px; }
.theater-sequence-row { position: relative; min-height: 58px; padding: 8px; border-bottom: 1px solid rgba(148, 163, 184, .12); background: rgba(15, 23, 42, .35); contain: layout paint; }
.theater-sequence-row.is-dragging { opacity: .42; }
.theater-sequence-row.is-drop-before::before, .theater-sequence-row.is-drop-after::after { position: absolute; right: 4px; left: 4px; height: 2px; content: ''; background: var(--theater-accent, #60a5fa); box-shadow: 0 0 8px color-mix(in srgb, var(--theater-accent, #60a5fa) 70%, transparent); }
.theater-sequence-row.is-drop-before::before { top: -1px; }
.theater-sequence-row.is-drop-after::after { bottom: -1px; }
.theater-sequence-row__handle { width: 30px; height: 36px; display: grid; place-items: center; border: 0; color: var(--sc-text-secondary, #a1a1aa); background: transparent; cursor: grab; touch-action: none; }
.theater-sequence-row__handle:active { cursor: grabbing; }
.theater-sequence-row__operation { color: var(--sc-text-secondary, #a1a1aa); font-size: 11px; }
.theater-sequence-row__type { color: var(--sc-text-primary, #f8fafc); font-size: 12px; }
.theater-sequence-row__timing { min-width: 0; display: grid; grid-template-columns: minmax(92px, 1fr) minmax(80px, .7fr); gap: 6px; }
.theater-sequence-editor__empty { min-height: 120px; display: grid; place-items: center; color: var(--sc-text-secondary, #a1a1aa); font-size: 12px; }
:global(.v-binder-follower-container:has(.n-select-menu.n-base-select-menu.theater-sequence-select-menu)),
:global(.v-binder-follower-container:has(.n-dropdown-menu.theater-sequence-select-menu)) {
  z-index: 10020 !important;
}
:global(.n-select-menu.n-base-select-menu.theater-sequence-select-menu),
:global(.n-dropdown-menu.theater-sequence-select-menu),
:global(:root[data-custom-theme='true'] .n-select-menu.n-base-select-menu.theater-sequence-select-menu) {
  --n-color: rgba(15, 23, 42, .76) !important;
  --n-option-text-color: #f8fafc !important;
  --n-option-text-color-active: #f8fafc !important;
  --n-option-text-color-pressed: #f8fafc !important;
  --n-option-check-color: var(--theater-accent, #60a5fa) !important;
  --n-option-color-active: rgba(96, 165, 250, .22) !important;
  --n-option-color-pending: rgba(96, 165, 250, .16) !important;
  --n-option-color-active-pending: rgba(96, 165, 250, .28) !important;
  border: 1px solid rgba(148, 163, 184, .24) !important;
  color: #f8fafc !important;
  background: rgba(15, 23, 42, .76) !important;
  box-shadow: 0 16px 42px rgba(0, 0, 0, .48) !important;
  backdrop-filter: blur(14px) saturate(120%);
  -webkit-backdrop-filter: blur(14px) saturate(120%);
}
:global(:root[data-custom-theme='true'] .n-select-menu.n-base-select-menu.theater-sequence-select-menu .n-base-select-option),
:global(.n-dropdown-menu.theater-sequence-select-menu .n-dropdown-option-body),
:global(.n-select-menu.n-base-select-menu.theater-sequence-select-menu .n-base-select-option) {
  color: #f8fafc !important;
  background-color: transparent !important;
}
:global(:root[data-custom-theme='true'] .n-select-menu.n-base-select-menu.theater-sequence-select-menu .n-base-select-option--pending),
:global(:root[data-custom-theme='true'] .n-select-menu.n-base-select-menu.theater-sequence-select-menu .n-base-select-option:hover),
:global(.n-dropdown-menu.theater-sequence-select-menu .n-dropdown-option-body:hover),
:global(.n-select-menu.n-base-select-menu.theater-sequence-select-menu .n-base-select-option--pending),
:global(.n-select-menu.n-base-select-menu.theater-sequence-select-menu .n-base-select-option:hover) {
  background-color: rgba(96, 165, 250, .16) !important;
}
@media (max-width: 820px) {
  .theater-sequence-grid--heading { display: none; }
  .theater-sequence-grid { grid-template-columns: 30px minmax(0, 1fr) 34px; }
  .theater-sequence-row > :not(.theater-sequence-row__handle):not(button:last-child) { grid-column: 2; }
  .theater-sequence-row > button:last-child { grid-column: 3; grid-row: 1; }
  .theater-sequence-row__timing { grid-template-columns: 1fr 1fr; }
}
</style>
