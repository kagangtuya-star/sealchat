<script setup lang="ts">
import { computed, ref } from 'vue'
import { NButton, NDropdown, NIcon, NInput, NModal } from 'naive-ui'
import { Plus, X } from '@vicons/tabler'
import TheaterRandomTableEditor from './TheaterRandomTableEditor.vue'
import TheaterClueActionEditor from './TheaterClueActionEditor.vue'
import TheaterSequenceStepsEditor, { stageSequenceActionTypeOptions } from './TheaterSequenceStepsEditor.vue'
import { STAGE_SEQUENCE_MAX_STEPS } from '../shared/stage-actions'
import type {
  StageAtomicAction,
  StageObject,
  StageScene,
  StageSequenceAction,
} from '../shared/stage-types'
import type { ChatClueAccessReadResult, ChatClueOptionsReadResult } from '../bridge/theater-bridge-protocol'

const props = defineProps<{
  show: boolean
  componentName: string
  action: StageSequenceAction | null
  scenes: StageScene[]
  persistentObjects: Record<string, StageObject>
  activeSceneId: string
  readClueOptions?: () => Promise<ChatClueOptionsReadResult>
  readClueAccess?: (clueId: string) => Promise<ChatClueAccessReadResult>
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
}>()

const addActionOptions = stageSequenceActionTypeOptions.map(({ label, value }) => ({ label, key: value }))
const sequenceDropdownMenuProps = () => ({
  class: 'theater-sequence-select-menu',
})
const stepsEditorRef = ref<InstanceType<typeof TheaterSequenceStepsEditor> | null>(null)

const handleAddStepSelect = (key: string | number) => {
  const type = String(key) as StageAtomicAction['type']
  if (stageSequenceActionTypeOptions.some((option) => option.value === type)) void stepsEditorRef.value?.addStep(type)
}

const randomTableStepId = ref('')
const randomTableEditorVisible = computed({
  get: () => Boolean(randomTableStepId.value && props.action),
  set: (value) => { if (!value) randomTableStepId.value = '' },
})
const editingRandomTableAction = computed(() => {
  const step = props.action?.payload.steps.find((item) => item.id === randomTableStepId.value)
  return step?.action.type === 'chat.random-table' ? step.action : null
})
const openRandomTableEditor = (stepId: string) => {
  randomTableStepId.value = stepId
}
const saveRandomTable = (payload: Extract<StageAtomicAction, { type: 'chat.random-table' }>['payload']) => {
  const action = editingRandomTableAction.value
  if (action) action.payload = payload
}
const clueStepId = ref('')
const clueEditorVisible = computed({
  get: () => Boolean(clueStepId.value && props.action),
  set: value => { if (!value) clueStepId.value = '' },
})
const editingClueAction = computed(() => {
  const step = props.action?.payload.steps.find(item => item.id === clueStepId.value)
  return step?.action.type === 'clue.execute' ? step.action : null
})
const openClueEditor = (stepId: string) => { clueStepId.value = stepId }
const saveClue = (payload: Extract<StageAtomicAction, { type: 'clue.execute' }>['payload']) => {
  const step = props.action?.payload.steps.find(item => item.id === clueStepId.value)
  if (step?.action.type === 'clue.execute') step.action.payload = payload
}
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

        <TheaterSequenceStepsEditor
          ref="stepsEditorRef"
          :steps="action.payload.steps"
          :scenes="scenes"
          :persistent-objects="persistentObjects"
          :active-scene-id="activeSceneId"
          :read-clue-options="readClueOptions"
          @edit-random-table="openRandomTableEditor"
          @edit-clue="openClueEditor"
        />
      </div>
    </section>
  </n-modal>
  <TheaterRandomTableEditor
    v-model:show="randomTableEditorVisible"
    :component-name="componentName"
    :action="editingRandomTableAction"
    @save="saveRandomTable"
  />
  <TheaterClueActionEditor
    v-model:show="clueEditorVisible"
    :component-name="componentName"
    :action="editingClueAction"
    :read-options="readClueOptions"
    :read-access="readClueAccess"
    @save="saveClue"
  />
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
</style>
