<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { NButton, NDropdown, NIcon, NInput, NInputNumber, NModal } from 'naive-ui'
import { GripVertical, Plus, Trash, X } from '@vicons/tabler'
import NSelect from '@/components/NSelect.vue'
import {
  createStageAtomicActionDescriptor,
  createStageSequenceStep,
  STAGE_SEQUENCE_MAX_STEPS,
} from '../shared/stage-actions'
import type {
  StageAtomicAction,
  StageObject,
  StageScene,
  StageSequenceAction,
  StageSequenceStep,
} from '../shared/stage-types'

const props = defineProps<{
  show: boolean
  componentName: string
  action: StageSequenceAction | null
  scenes: StageScene[]
  persistentObjects: Record<string, StageObject>
  activeSceneId: string
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
}>()

const actionTypeOptions: Array<{ label: string, value: StageAtomicAction['type'] }> = [
  { label: '发送消息', value: 'chat.send' },
  { label: '插入输入框', value: 'chat.insert' },
  { label: '切换场景', value: 'scene.apply' },
  { label: '切换组件显隐', value: 'object.toggle' },
]
const addActionOptions = actionTypeOptions.map(({ label, value }) => ({ label, key: value }))
const actionTypeLabel = (type: StageAtomicAction['type']) => (
  actionTypeOptions.find((option) => option.value === type)?.label || type
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
const sequenceDropdownMenuProps = () => ({
  class: 'theater-sequence-select-menu',
})
const sceneOptions = computed(() => props.scenes.map((scene) => ({ label: scene.name, value: scene.id })))
const allObjectOptions = computed(() => {
  const values = new Map<string, { label: string, value: string }>()
  props.scenes.forEach((scene) => Object.values(scene.state.sceneObjects).forEach((object) => {
    if (object.type !== 'group') values.set(object.id, { label: `${object.name} · ${scene.name}`, value: object.id })
  }))
  Object.values(props.persistentObjects).forEach((object) => {
    if (object.type !== 'group') values.set(object.id, { label: `${object.name} · 跨场景`, value: object.id })
  })
  return [...values.values()]
})

const objectOptions = (sceneId: string | null) => {
  const scene = sceneId ? props.scenes.find((item) => item.id === sceneId) : null
  const local = scene
    ? Object.values(scene.state.sceneObjects)
      .filter((object) => object.type !== 'group')
      .map((object) => ({ label: object.name, value: object.id }))
    : allObjectOptions.value
  const fixed = Object.values(props.persistentObjects)
    .filter((object) => object.type !== 'group')
    .map((object) => ({ label: `${object.name} · 跨场景`, value: object.id }))
  const values = new Map([...local, ...fixed].map((option) => [option.value, option]))
  return [...values.values()]
}

const firstObjectId = (sceneId: string | null) => objectOptions(sceneId)[0]?.value || ''

const addStep = (type: StageAtomicAction['type']) => {
  if (!props.action || props.action.payload.steps.length >= STAGE_SEQUENCE_MAX_STEPS) return
  const sceneId = props.activeSceneId || props.scenes[0]?.id || ''
  const step = createStageSequenceStep(sceneId, firstObjectId(sceneId))
  step.action = createStageAtomicActionDescriptor(type, sceneId, firstObjectId(sceneId))
  props.action.payload.steps.push(step)
}

const handleAddStepSelect = (key: string | number) => {
  const type = String(key) as StageAtomicAction['type']
  if (actionTypeOptions.some((option) => option.value === type)) addStep(type)
}

const removeStep = (stepId: string) => {
  if (!props.action) return
  const index = props.action.payload.steps.findIndex((step) => step.id === stepId)
  if (index >= 0) props.action.payload.steps.splice(index, 1)
}

const updateStepScene = (step: StageSequenceStep, sceneId: string) => {
  step.sceneId = sceneId || null
  if (step.action.type === 'scene.apply') step.action.payload.sceneId = sceneId
  if (step.action.type === 'object.toggle') {
    const options = objectOptions(step.sceneId)
    const objectId = step.action.payload.objectId
    if (options.length && !options.some((option) => option.value === objectId)) {
      step.action.payload.objectId = options[0].value
    }
  }
  openSelectKey.value = ''
}

const updateTiming = (step: StageSequenceStep, mode: 'after' | 'delay' | 'sync') => {
  step.timing = mode === 'delay' ? { mode, delayMs: 500 } : { mode }
  openSelectKey.value = ''
}

const updateStepObject = (step: StageSequenceStep, objectId: string) => {
  if (step.action.type !== 'object.toggle') return
  step.action.payload.objectId = objectId
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
  if (!props.action) return
  const rows = props.action.payload.steps
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
  const action = props.action
  const sourceId = draggingId.value
  const targetIndex = dropIndex.value
  draggingId.value = ''
  dropIndex.value = -1
  if (!action || !sourceId || targetIndex < 0) return
  const sourceIndex = action.payload.steps.findIndex((step) => step.id === sourceId)
  if (sourceIndex < 0) return
  const [step] = action.payload.steps.splice(sourceIndex, 1)
  const adjusted = targetIndex > sourceIndex ? targetIndex - 1 : targetIndex
  action.payload.steps.splice(Math.max(0, Math.min(adjusted, action.payload.steps.length)), 0, step)
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
  if (!props.action) return
  const index = props.action.payload.steps.findIndex((step) => step.id === stepId)
  const target = index + offset
  if (index < 0 || target < 0 || target >= props.action.payload.steps.length) return
  const [step] = props.action.payload.steps.splice(index, 1)
  props.action.payload.steps.splice(target, 0, step)
}

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', stopDragging)
  window.removeEventListener('pointercancel', stopDragging)
  if (dragFrame) cancelAnimationFrame(dragFrame)
})
</script>

<template>
  <n-modal :show="show" :mask-closable="true" @update:show="emit('update:show', $event)">
    <section class="theater-sequence-editor" role="dialog" aria-modal="true" aria-label="点击动作组合编辑器">
      <header class="theater-sequence-editor__header">
        <div>
          <strong>{{ componentName }} · 点击动作组合</strong>
          <small>修改自动保存</small>
        </div>
        <n-button text aria-label="关闭动作组合编辑器" @click="emit('update:show', false)"><n-icon><X /></n-icon></n-button>
      </header>

      <div v-if="action" class="theater-sequence-editor__body">
        <div class="theater-sequence-editor__name">
          <span>组合名称</span>
          <n-input v-model:value="action.payload.name" maxlength="128" placeholder="点击动作组合" />
          <n-dropdown :options="addActionOptions" trigger="click" :menu-props="sequenceDropdownMenuProps" @select="handleAddStepSelect">
            <n-button size="small" secondary :disabled="action.payload.steps.length >= STAGE_SEQUENCE_MAX_STEPS">
              <template #icon><n-icon><Plus /></n-icon></template>添加
            </n-button>
          </n-dropdown>
        </div>

        <div class="theater-sequence-grid theater-sequence-grid--heading">
          <span></span><span>操作内容 / 组件</span><span>动作类型</span><span>场景</span><span>时延</span><span></span>
        </div>
        <div class="theater-sequence-editor__rows">
          <div
            v-for="(step, index) in action.payload.steps"
            :key="step.id"
            :ref="element => setRowElement(step.id, element as Element | null)"
            class="theater-sequence-grid theater-sequence-row"
            :class="{
              'is-dragging': draggingId === step.id,
              'is-drop-before': draggingId && dropIndex === index,
              'is-drop-after': draggingId && dropIndex === action.payload.steps.length && index === action.payload.steps.length - 1,
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
            <n-select
              v-else-if="step.action.type === 'object.toggle'"
              :value="step.action.payload.objectId"
              :show="isSequenceSelectOpen('object', step.id)"
              :options="objectOptions(step.sceneId)"
              filterable
              :menu-props="sequenceSelectMenuProps"
              placeholder="选择组件"
              @update:show="updateSelectShow('object', step, $event)"
              @update:value="updateStepObject(step, $event)"
            />
            <span v-else class="theater-sequence-row__operation">切换至所选场景</span>

            <span class="theater-sequence-row__type">{{ actionTypeLabel(step.action.type) }}</span>

            <n-select
              :value="step.action.type === 'scene.apply' ? step.action.payload.sceneId : step.sceneId"
              :show="isSequenceSelectOpen('scene', step.id)"
              :options="sceneOptions"
              filterable
              :menu-props="sequenceSelectMenuProps"
              placeholder="当前场景"
              @update:show="updateSelectShow('scene', step, $event)"
              @update:value="updateStepScene(step, $event)"
            />

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
          <div v-if="!action.payload.steps.length" class="theater-sequence-editor__empty">暂无动作。添加后按顺序触发。</div>
        </div>
      </div>
    </section>
  </n-modal>
</template>

<style scoped>
.theater-sequence-editor { width: min(1120px, calc(100vw - 32px)); max-height: min(760px, calc(100vh - 32px)); overflow: visible; border: 1px solid rgba(148, 163, 184, .22); border-radius: 12px; color: var(--sc-text-primary, #f8fafc); background: rgba(15, 23, 42, .82); box-shadow: 0 24px 80px rgba(0, 0, 0, .42); backdrop-filter: blur(16px) saturate(120%); }
.theater-sequence-editor__header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border-bottom: 1px solid rgba(148, 163, 184, .18); }
.theater-sequence-editor__header div { min-width: 0; display: grid; gap: 3px; }
.theater-sequence-editor__header strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 15px; }
.theater-sequence-editor__header small { color: var(--sc-text-secondary, #a1a1aa); font-size: 10px; }
.theater-sequence-editor__body { min-height: 0; display: flex; flex-direction: column; padding: 14px; }
.theater-sequence-editor__name { display: grid; grid-template-columns: auto minmax(180px, 1fr) auto; align-items: center; gap: 10px; margin-bottom: 12px; }
.theater-sequence-editor__name span { color: var(--sc-text-secondary, #a1a1aa); font-size: 12px; }
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
