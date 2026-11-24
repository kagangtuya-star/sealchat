<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NSwitch, useDialog } from 'naive-ui'
import { Edit, PlayerPlay, Plus, Search, Trash } from '@vicons/tabler'
import { isStageActionTarget, type StageObject, type StageScene } from '../shared/stage-types'
import TheaterSequenceEditor from './TheaterSequenceEditor.vue'
import {
  createDefaultTheaterSequence,
  normalizeTheaterSequences,
  THEATER_SEQUENCE_MAX_COUNT,
  type TheaterSequence,
} from './theater-sequence-types'

const props = defineProps<{
  scopeId: string
  sequences: TheaterSequence[]
  commitSequences: (next: TheaterSequence[]) => void
  scene?: {
    scenes: StageScene[]
    objects: Record<string, StageObject>
    persistentObjects: Record<string, StageObject>
  }
  worldEffects?: StageObject[]
  canEdit: boolean
  canTest: boolean
}>()

const emit = defineEmits<{
  test: [sequenceId: string]
}>()

const dialog = useDialog()
const search = ref('')
const editorOpen = ref(false)
const editingSequence = ref<TheaterSequence | null>(null)
const filteredSequences = computed(() => {
  const keyword = search.value.trim().toLocaleLowerCase()
  return keyword
    ? props.sequences.filter((sequence) => sequence.name.toLocaleLowerCase().includes(keyword))
    : props.sequences
})
const clickTargetOptions = computed(() => Object.values(props.scene?.objects ?? {})
  .filter((object) => object.visible && object.interactive && isStageActionTarget(object.type))
  .map((object) => ({
    label: `${object.name}${props.scene?.persistentObjects[object.id] ? ' · 跨场景' : ''}`,
    value: object.id,
  })))

// Editing is scoped to the scene that was active when the editor opened.
watch(() => props.scopeId, () => {
  editorOpen.value = false
  editingSequence.value = null
})

const commit = (next: TheaterSequence[]) => {
  if (props.canEdit) props.commitSequences(normalizeTheaterSequences(next))
}

const hasPlayableSteps = (sequence: TheaterSequence) => sequence.steps.some((step) => (
  props.scene || (step.action.type === 'effect.play' && !step.sceneId)
))

const triggerSummary = (sequence: TheaterSequence) => {
  if (!sequence.triggers.length) return '仅手动测试'
  return sequence.triggers.map((trigger) => {
    if (trigger.type === 'message') {
      const keywords = trigger.keywords.length ? trigger.keywords.slice(0, 3).join('/') : '未设关键词'
      return `消息「${keywords}${trigger.keywords.length > 3 ? '…' : ''}」`
    }
    return props.scene ? `点击「${props.scene.objects[trigger.objectId]?.name || '已失效组件'}」` : '组件点击（需要小剧场）'
  }).join(' 或 ')
}

const openEditor = (sequence: TheaterSequence) => {
  if (!props.canEdit) return
  editingSequence.value = sequence
  editorOpen.value = true
}

const openSequence = (id: string) => {
  const sequence = props.sequences.find((item) => item.id === id)
  if (!sequence) return
  search.value = ''
  openEditor(sequence)
}

const createSequence = () => {
  if (props.sequences.length >= THEATER_SEQUENCE_MAX_COUNT) return
  openEditor(createDefaultTheaterSequence())
}

const saveSequence = (sequence: TheaterSequence) => {
  const current = props.sequences
  const index = current.findIndex((item) => item.id === sequence.id)
  commit(index >= 0
    ? current.map((item) => (item.id === sequence.id ? sequence : item))
    : [...current, sequence])
}

const setEnabled = (sequence: TheaterSequence, enabled: boolean) => {
  commit(props.sequences.map((item) => (item.id === sequence.id ? { ...item, enabled } : item)))
}

const removeSequence = (sequence: TheaterSequence) => {
  if (!props.canEdit) return
  const scopeId = props.scopeId
  dialog.warning({
    title: '删除序列',
    content: `确定删除序列“${sequence.name}”？`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: () => {
      if (props.scopeId !== scopeId) return
      commit(props.sequences.filter((item) => item.id !== sequence.id))
    },
  })
}

defineExpose({ openSequence })
</script>

<template>
  <div class="theater-sequence-panel">
    <div class="theater-sequence-panel__toolbar">
      <n-input v-model:value="search" size="small" clearable placeholder="搜索序列名称">
        <template #prefix><n-icon><Search /></n-icon></template>
      </n-input>
      <n-button size="small" type="primary" secondary :disabled="!canEdit || sequences.length >= THEATER_SEQUENCE_MAX_COUNT" @click="createSequence">
        <template #icon><n-icon><Plus /></n-icon></template>新建序列
      </n-button>
    </div>

    <div class="theater-sequence-panel__list">
      <div v-if="!sequences.length" class="theater-sequence-panel__empty">{{ scene ? '当前场景没有序列。新建序列后可用消息关键词或组件点击触发特效与场景切换。' : '当前世界没有序列。' }}</div>
      <div v-else-if="!filteredSequences.length" class="theater-sequence-panel__empty">没有匹配的序列</div>
      <article
        v-for="sequence in filteredSequences"
        :key="sequence.id"
        class="theater-sequence-panel__row"
        :class="{ 'is-disabled': !sequence.enabled }"
      >
        <n-switch :value="sequence.enabled" size="small" :disabled="!canEdit" @update:value="setEnabled(sequence, $event)" />
        <button class="theater-sequence-panel__select" type="button" :disabled="!canEdit" @click="openEditor(sequence)">
          <strong>{{ sequence.name }}</strong>
          <span :title="triggerSummary(sequence)">{{ triggerSummary(sequence) }}</span>
          <small>{{ sequence.steps.length }} 步{{ sequence.loopCount > 1 ? ` · 循环 ${sequence.loopCount} 轮` : '' }}</small>
        </button>
        <div class="theater-sequence-panel__actions">
          <n-button text size="tiny" aria-label="测试序列" title="测试序列" :disabled="!canTest || !sequence.enabled || !hasPlayableSteps(sequence)" @click="emit('test', sequence.id)"><n-icon><PlayerPlay /></n-icon></n-button>
          <n-button text size="tiny" aria-label="编辑序列" title="编辑序列" :disabled="!canEdit" @click="openEditor(sequence)"><n-icon><Edit /></n-icon></n-button>
          <n-button text size="tiny" type="error" aria-label="删除序列" title="删除序列" :disabled="!canEdit" @click="removeSequence(sequence)"><n-icon><Trash /></n-icon></n-button>
          <slot name="row-actions" :sequence="sequence" />
        </div>
      </article>
    </div>

    <TheaterSequenceEditor
      v-model:show="editorOpen"
      :sequence="editingSequence"
      :scenes="scene?.scenes ?? []"
      :persistent-objects="scene?.persistentObjects ?? {}"
      :active-scene-id="scene ? scopeId : ''"
      :world-effects="worldEffects"
      :click-target-options="clickTargetOptions"
      @save="saveSequence"
    />
  </div>
</template>

<style scoped>
.theater-sequence-panel { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.theater-sequence-panel__toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 6px; padding: 7px 9px; border-bottom: 1px solid var(--theater-border); }
.theater-sequence-panel__list { min-height: 0; flex: 1; overflow: auto; }
.theater-sequence-panel__empty { padding: 36px 14px; color: var(--sc-text-secondary); font-size: 11px; text-align: center; }
.theater-sequence-panel__row { min-height: 54px; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 7px; padding: 5px 7px; border-bottom: 1px solid color-mix(in srgb, var(--theater-border) 68%, transparent); }
.theater-sequence-panel__row:hover { background: color-mix(in srgb, var(--theater-accent) 14%, transparent); }
.theater-sequence-panel__row.is-disabled { opacity: .55; }
.theater-sequence-panel__select { min-width: 0; display: grid; gap: 2px; border: 0; padding: 3px 0; color: inherit; background: transparent; text-align: left; cursor: pointer; }
.theater-sequence-panel__select:disabled { cursor: default; }
.theater-sequence-panel__select strong, .theater-sequence-panel__select span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-sequence-panel__select strong { font-size: 11px; }
.theater-sequence-panel__select span, .theater-sequence-panel__select small { color: var(--sc-text-secondary); font-size: 9px; }
.theater-sequence-panel__actions { display: flex; align-items: center; gap: 1px; }
.theater-sequence-panel__actions :deep(.n-button) { width: 24px; height: 24px; padding: 0; }
</style>
