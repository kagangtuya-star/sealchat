<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NDropdown, NIcon, NInput, NInputNumber, NModal, NSwitch } from 'naive-ui'
import { Click, Message, Plus, Trash, X } from '@vicons/tabler'
import NSelect from '@/components/NSelect.vue'
import TheaterSequenceStepsEditor, { stageSequenceActionTypeOptions } from '../stage/TheaterSequenceStepsEditor.vue'
import { STAGE_SEQUENCE_MAX_STEPS } from '../shared/stage-actions'
import type { StageObject, StageScene } from '../shared/stage-types'
import { cloneStageData } from '../stage/stage-editing'
import {
  createTheaterSequenceTrigger,
  isTheaterSequenceStepActionType,
  normalizeTheaterSequence,
  THEATER_SEQUENCE_MAX_COOLDOWN_MS,
  THEATER_SEQUENCE_MAX_HIT_INTERVAL,
  THEATER_SEQUENCE_MAX_LOOP_COUNT,
  THEATER_SEQUENCE_MAX_TRIGGERS,
  type TheaterSequence,
  type TheaterSequenceTriggerType,
} from './theater-sequence-types'

const props = defineProps<{
  show: boolean
  sequence: TheaterSequence | null
  scenes: StageScene[]
  persistentObjects: Record<string, StageObject>
  activeSceneId: string
  clickTargetOptions: Array<{ label: string, value: string }>
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  save: [sequence: TheaterSequence]
}>()

const draft = ref<TheaterSequence | null>(null)
const keywordDrafts = ref<Record<string, string>>({})
const stepsEditorRef = ref<InstanceType<typeof TheaterSequenceStepsEditor> | null>(null)

watch(() => [props.show, props.sequence] as const, ([show, sequence]) => {
  if (!show || !sequence) {
    if (!show) draft.value = null
    return
  }
  if (draft.value?.id === sequence.id) return
  draft.value = cloneStageData(sequence)
  keywordDrafts.value = Object.fromEntries(sequence.triggers.flatMap((trigger) => (
    trigger.type === 'message' ? [[trigger.id, trigger.keywords.join('\n')]] : []
  )))
}, { immediate: true })

const addStepOptions = stageSequenceActionTypeOptions
  .filter((option) => isTheaterSequenceStepActionType(option.value))
  .map(({ label, value }) => ({ label, key: value }))
const sequenceMenuProps = { class: 'theater-sequence-select-menu' }
const sequenceDropdownMenuProps = () => sequenceMenuProps

const handleAddStepSelect = (key: string | number) => {
  const type = String(key)
  if (isTheaterSequenceStepActionType(type)) void stepsEditorRef.value?.addStep(type)
}

const canAddTrigger = computed(() => Boolean(draft.value && draft.value.triggers.length < THEATER_SEQUENCE_MAX_TRIGGERS))

const addTrigger = (type: TheaterSequenceTriggerType) => {
  if (!draft.value || !canAddTrigger.value) return
  const objectId = props.clickTargetOptions[0]?.value || ''
  if (type === 'component.click' && !objectId) return
  const trigger = createTheaterSequenceTrigger(type, objectId)
  if (trigger.type === 'message') keywordDrafts.value = { ...keywordDrafts.value, [trigger.id]: '' }
  draft.value.triggers.push(trigger)
}

const removeTrigger = (triggerId: string) => {
  if (!draft.value) return
  draft.value.triggers = draft.value.triggers.filter((trigger) => trigger.id !== triggerId)
}

const updateKeywords = (triggerId: string, value: string) => {
  keywordDrafts.value = { ...keywordDrafts.value, [triggerId]: value }
  const trigger = draft.value?.triggers.find((item) => item.id === triggerId)
  if (trigger?.type === 'message') {
    trigger.keywords = value.split(/[\n,，]/).map((item) => item.trim()).filter(Boolean)
  }
}

const clickTargetSelectOptions = (objectId: string) => (
  props.clickTargetOptions.some((option) => option.value === objectId)
    ? props.clickTargetOptions
    : [...props.clickTargetOptions, { label: `已失效组件 · ${objectId}`, value: objectId }]
)

const close = () => emit('update:show', false)

const save = () => {
  const normalized = normalizeTheaterSequence(draft.value)
  if (!normalized) return
  emit('save', normalized)
  close()
}
</script>

<template>
  <n-modal :show="show" :mask-closable="false" @update:show="emit('update:show', $event)">
    <section class="theater-sequencer-editor" role="dialog" aria-modal="true" aria-label="序列编辑器">
      <header class="theater-sequencer-editor__header">
        <div>
          <strong>{{ draft?.name || '序列' }} · 序列</strong>
          <small>当前场景序列器 · 保存后生效</small>
        </div>
        <n-button text aria-label="关闭序列编辑器" @click="close"><n-icon><X /></n-icon></n-button>
      </header>

      <div v-if="draft" class="theater-sequencer-editor__body">
        <div class="theater-sequencer-editor__basic">
          <label>
            <span>序列名称</span>
            <n-input v-model:value="draft.name" maxlength="128" placeholder="新序列" />
          </label>
          <label>
            <span>启用</span>
            <n-switch v-model:value="draft.enabled" />
          </label>
          <label>
            <span>整体循环次数</span>
            <n-input-number v-model:value="draft.loopCount" :min="1" :max="THEATER_SEQUENCE_MAX_LOOP_COUNT" :precision="0" @blur="draft.loopCount = draft.loopCount || 1" />
          </label>
        </div>

        <section class="theater-sequencer-editor__section">
          <div class="theater-sequencer-editor__section-heading">
            <strong>触发条件 <small>任意一个满足即触发</small></strong>
            <div>
              <n-button size="small" secondary :disabled="!canAddTrigger" @click="addTrigger('message')">
                <template #icon><n-icon><Message /></n-icon></template>消息触发
              </n-button>
              <n-button size="small" secondary :disabled="!canAddTrigger || !clickTargetOptions.length" @click="addTrigger('component.click')">
                <template #icon><n-icon><Click /></n-icon></template>组件点击
              </n-button>
            </div>
          </div>
          <div class="theater-sequencer-trigger-grid theater-sequencer-trigger-grid--heading">
            <span>类型</span><span>条件</span><span>第 N 次开始</span><span>每 N 次</span><span>冷却（毫秒）</span><span></span>
          </div>
          <div class="theater-sequencer-editor__triggers">
            <div v-for="trigger in draft.triggers" :key="trigger.id" class="theater-sequencer-trigger-grid theater-sequencer-trigger">
              <span class="theater-sequencer-trigger__type">{{ trigger.type === 'message' ? '消息' : '组件点击' }}</span>
              <div v-if="trigger.type === 'message'" class="theater-sequencer-trigger__condition">
                <n-input
                  :value="keywordDrafts[trigger.id] ?? trigger.keywords.join('\n')"
                  type="textarea"
                  :autosize="{ minRows: 1, maxRows: 4 }"
                  placeholder="关键词，每行一个；命中任意关键词"
                  @update:value="updateKeywords(trigger.id, $event)"
                />
                <n-input
                  :value="trigger.targetActorName || ''"
                  clearable
                  placeholder="指定频道角色名，多个用英文 ; 分隔；留空表示全部"
                  @update:value="trigger.targetActorName = $event || null"
                />
              </div>
              <n-select
                v-else
                v-model:value="trigger.objectId"
                :options="clickTargetSelectOptions(trigger.objectId)"
                filterable
                :menu-props="sequenceMenuProps"
                placeholder="选择可交互组件"
              />
              <label class="theater-sequencer-trigger__field">
                <span>第 N 次开始</span>
                <n-input-number v-model:value="trigger.threshold" :min="1" :max="THEATER_SEQUENCE_MAX_HIT_INTERVAL" :precision="0" @blur="trigger.threshold = trigger.threshold || 1" />
              </label>
              <label class="theater-sequencer-trigger__field">
                <span>每 N 次</span>
                <n-input-number v-model:value="trigger.every" :min="1" :max="THEATER_SEQUENCE_MAX_HIT_INTERVAL" :precision="0" @blur="trigger.every = trigger.every || 1" />
              </label>
              <label class="theater-sequencer-trigger__field">
                <span>冷却（毫秒）</span>
                <n-input-number v-model:value="trigger.cooldownMs" :min="0" :max="THEATER_SEQUENCE_MAX_COOLDOWN_MS" :step="500" :precision="0" @blur="trigger.cooldownMs = trigger.cooldownMs || 0" />
              </label>
              <n-button text type="error" aria-label="删除触发条件" @click="removeTrigger(trigger.id)"><n-icon><Trash /></n-icon></n-button>
            </div>
            <div v-if="!draft.triggers.length" class="theater-sequencer-editor__empty">暂无触发条件，仅可手动测试。</div>
          </div>
        </section>

        <section class="theater-sequencer-editor__section theater-sequencer-editor__section--steps">
          <div class="theater-sequencer-editor__section-heading">
            <strong>执行步骤 <small>每轮按顺序执行，整体重复 {{ draft.loopCount || 1 }} 轮</small></strong>
            <n-dropdown :options="addStepOptions" trigger="click" :menu-props="sequenceDropdownMenuProps" @select="handleAddStepSelect">
              <n-button size="small" secondary :disabled="draft.steps.length >= STAGE_SEQUENCE_MAX_STEPS">
                <template #icon><n-icon><Plus /></n-icon></template>添加步骤
              </n-button>
            </n-dropdown>
          </div>
          <TheaterSequenceStepsEditor
            ref="stepsEditorRef"
            :steps="draft.steps"
            :scenes="scenes"
            :persistent-objects="persistentObjects"
            :active-scene-id="activeSceneId"
            empty-text="暂无步骤。可添加播放特效或切换场景。"
          />
        </section>
      </div>

      <footer class="theater-sequencer-editor__footer">
        <n-button size="small" @click="close">取消</n-button>
        <n-button size="small" type="primary" :disabled="!draft" @click="save">保存</n-button>
      </footer>
    </section>
  </n-modal>
</template>

<style scoped>
.theater-sequencer-editor { width: min(1120px, calc(100vw - 32px)); max-height: min(820px, calc(100vh - 32px)); display: flex; flex-direction: column; overflow: hidden; border: 1px solid rgba(148, 163, 184, .22); border-radius: 12px; color: var(--sc-text-primary, #f8fafc); background: rgba(15, 23, 42, .82); box-shadow: 0 24px 80px rgba(0, 0, 0, .42); backdrop-filter: blur(16px) saturate(120%); }
.theater-sequencer-editor__header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border-bottom: 1px solid rgba(148, 163, 184, .18); }
.theater-sequencer-editor__header div { min-width: 0; display: grid; gap: 3px; }
.theater-sequencer-editor__header strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 15px; }
.theater-sequencer-editor__header small, .theater-sequencer-editor small { color: var(--sc-text-secondary, #a1a1aa); font-size: 10px; font-weight: 400; }
.theater-sequencer-editor__body { min-height: 0; display: flex; flex: 1; flex-direction: column; gap: 14px; overflow: auto; padding: 14px; }
.theater-sequencer-editor__basic { display: grid; grid-template-columns: minmax(200px, 1fr) auto minmax(140px, 180px); align-items: end; gap: 12px; }
.theater-sequencer-editor__basic label { min-width: 0; display: grid; gap: 5px; }
.theater-sequencer-editor__basic span { color: var(--sc-text-secondary, #a1a1aa); font-size: 12px; }
.theater-sequencer-editor__section { display: flex; flex-direction: column; }
.theater-sequencer-editor__section-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.theater-sequencer-editor__section-heading strong { font-size: 13px; }
.theater-sequencer-editor__section-heading > div { display: flex; gap: 6px; }
.theater-sequencer-trigger-grid { display: grid; grid-template-columns: 72px minmax(240px, 2fr) minmax(100px, .6fr) minmax(100px, .6fr) minmax(120px, .7fr) 34px; align-items: center; gap: 8px; }
.theater-sequencer-trigger-grid--heading { padding: 0 8px 7px; color: var(--sc-text-secondary, #a1a1aa); font-size: 10px; }
.theater-sequencer-trigger { min-height: 52px; padding: 8px; border-bottom: 1px solid rgba(148, 163, 184, .12); background: rgba(15, 23, 42, .35); }
.theater-sequencer-trigger__type { font-size: 12px; }
.theater-sequencer-trigger__condition { min-width: 0; display: grid; gap: 6px; }
.theater-sequencer-trigger__field { min-width: 0; display: grid; gap: 4px; }
.theater-sequencer-trigger__field span { display: none; color: var(--sc-text-secondary, #a1a1aa); font-size: 10px; }
.theater-sequencer-editor__empty { min-height: 64px; display: grid; place-items: center; color: var(--sc-text-secondary, #a1a1aa); font-size: 12px; }
.theater-sequencer-editor__footer { display: flex; justify-content: flex-end; gap: 8px; padding: 10px 16px; border-top: 1px solid rgba(148, 163, 184, .18); }
@media (max-width: 820px) {
  .theater-sequencer-editor__basic { grid-template-columns: 1fr; align-items: stretch; }
  .theater-sequencer-trigger-grid--heading { display: none; }
  .theater-sequencer-trigger-grid { grid-template-columns: minmax(0, 1fr) 34px; }
  .theater-sequencer-trigger > :not(button:last-child) { grid-column: 1; }
  .theater-sequencer-trigger > button:last-child { grid-column: 2; grid-row: 1; }
  .theater-sequencer-trigger__field span { display: block; }
}
</style>
