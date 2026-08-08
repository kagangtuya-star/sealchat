<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NIcon } from 'naive-ui'
import { Plus, Trash, X } from '@vicons/tabler'
import type { StageAction } from '../shared/stage-types'
import {
  normalizeStageRandomTablePayload,
  STAGE_RANDOM_TABLE_MAX_ENTRIES,
} from '../shared/stage-actions'

type RandomTableAction = Extract<StageAction, { type: 'chat.random-table' }>
type RandomTablePayload = RandomTableAction['payload']
type RandomTableEditableAction = Pick<RandomTableAction, 'payload'> & { id?: string }
type DraftEntry = { min: number | '', max: number | '', text: string }

const props = defineProps<{
  show: boolean
  componentName: string
  action: RandomTableEditableAction | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  save: [payload: RandomTablePayload]
}>()

const name = ref('')
const formula = ref('1d6')
const entries = ref<DraftEntry[]>([])
const validationError = ref('')

const resetDraft = () => {
  const payload = props.action?.payload
  name.value = payload?.name || ''
  formula.value = payload?.formula || '1d6'
  entries.value = payload?.entries.map((entry) => ({ ...entry })) || []
  validationError.value = ''
}

watch(() => [props.show, props.action?.id] as const, ([show]) => {
  if (show) resetDraft()
})

const addEntry = () => {
  if (entries.value.length >= STAGE_RANDOM_TABLE_MAX_ENTRIES) return
  const previous = entries.value[entries.value.length - 1]
  const minimum = typeof previous?.max === 'number' ? previous.max + 1 : 1
  entries.value.push({ min: minimum, max: minimum, text: '' })
}

const removeEntry = (index: number) => entries.value.splice(index, 1)

const close = () => emit('update:show', false)

const save = () => {
  const payload = normalizeStageRandomTablePayload({
    name: name.value,
    formula: formula.value,
    entries: entries.value,
  })
  if (!payload) {
    validationError.value = '配置无效：检查名称、骰式、结果文本和区间；区间不能倒置或重叠。'
    return
  }
  emit('save', payload)
  close()
}
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="random-table-editor-modal">
      <section class="random-table-editor" role="dialog" aria-modal="true" aria-label="随机表编辑器" @pointerdown.stop @click.stop @keydown.stop>
      <header class="random-table-editor__header">
        <div>
          <strong>{{ componentName }} · 随机表</strong>
          <small>掷骰后发送匹配结果到当前聊天频道</small>
        </div>
        <n-button text aria-label="关闭随机表编辑器" @click="close"><n-icon><X /></n-icon></n-button>
      </header>

      <div class="random-table-editor__body">
        <label>
          <span>名称</span>
          <input v-model="name" class="random-table-editor__input" maxlength="128" placeholder="输入随机表名称" />
        </label>
        <label>
          <span>自定义骰式</span>
          <div class="random-table-editor__formula">
            <input v-model="formula" class="random-table-editor__input" maxlength="128" placeholder="例如 1D6、2D10+3" />
            <small>支持 NdM、NdM+K、NdM-K；允许空格与大小写。</small>
          </div>
        </label>

        <div class="random-table-editor__entries-header">
          <strong>结果条目</strong>
          <n-button size="small" secondary :disabled="entries.length >= STAGE_RANDOM_TABLE_MAX_ENTRIES" @click="addEntry">
            <template #icon><n-icon><Plus /></n-icon></template>添加条目
          </n-button>
        </div>
        <div class="random-table-editor__entries">
          <div v-for="(entry, index) in entries" :key="index" class="random-table-editor__entry">
            <input v-model.number="entry.min" class="random-table-editor__input" type="number" step="1" placeholder="最小" aria-label="区间最小值" />
            <span>至</span>
            <input v-model.number="entry.max" class="random-table-editor__input" type="number" step="1" placeholder="最大" aria-label="区间最大值" />
            <input v-model="entry.text" class="random-table-editor__input" maxlength="10000" placeholder="输入命中结果" />
            <n-button text type="error" aria-label="删除条目" @click="removeEntry(index)"><n-icon><Trash /></n-icon></n-button>
          </div>
          <div v-if="!entries.length" class="random-table-editor__empty">至少添加一个结果条目。</div>
        </div>

        <p v-if="validationError" class="random-table-editor__error" role="alert">{{ validationError }}</p>
      </div>

      <footer class="random-table-editor__footer">
        <n-button @click="close">取消</n-button>
        <n-button type="primary" @click="save">保存</n-button>
      </footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.random-table-editor-modal { --theater-accent: #3b82f6; --theater-panel: color-mix(in srgb, var(--sc-bg-surface, #262626) 48%, transparent); --theater-panel-muted: color-mix(in srgb, var(--sc-bg-layer, #3f3f46) 56%, transparent); --theater-border: var(--sc-border-strong, rgba(255, 255, 255, .16)); position: fixed; z-index: 10005; inset: 0; display: grid; place-items: center; padding: 16px; pointer-events: auto; background: rgba(0, 0, 0, .24); }
.random-table-editor { width: min(920px, calc(100vw - 32px)); max-height: min(760px, calc(100vh - 32px)); display: flex; flex-direction: column; border: 1px solid var(--theater-border); border-radius: 7px; color: var(--sc-text-primary, #f4f4f5); background: var(--theater-panel); box-shadow: 0 14px 34px rgba(0, 0, 0, .2); backdrop-filter: blur(8px) saturate(110%); -webkit-backdrop-filter: blur(8px) saturate(110%); }
.random-table-editor__header, .random-table-editor__footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 16px; }
.random-table-editor__header { border-bottom: 1px solid rgba(148, 163, 184, .18); }
.random-table-editor__header div { min-width: 0; display: grid; gap: 3px; }
.random-table-editor__header small, .random-table-editor label > span { color: var(--sc-text-secondary, #a1a1aa); font-size: 11px; }
.random-table-editor__body { min-height: 0; overflow: auto; display: grid; gap: 12px; padding: 16px; }
.random-table-editor label { display: grid; grid-template-columns: 72px minmax(0, 1fr); align-items: center; gap: 10px; }
.random-table-editor__formula { min-width: 0; display: grid; gap: 4px; }
.random-table-editor__formula small { color: var(--sc-text-secondary, #a1a1aa); font-size: 10px; }
.random-table-editor__input { min-width: 0; width: 100%; height: 34px; padding: 0 11px; border: 1px solid var(--theater-border); border-radius: 4px; outline: none; color: var(--sc-text-primary, #f4f4f5); background: var(--theater-panel-muted); font: inherit; cursor: text; pointer-events: auto; user-select: text; transition: border-color .15s, background-color .15s, box-shadow .15s; }
.random-table-editor__input:hover { border-color: rgba(148, 163, 184, .48); }
.random-table-editor__input:focus { border-color: var(--theater-accent); background: color-mix(in srgb, var(--theater-panel-muted) 88%, var(--sc-bg-layer, #3f3f46)); box-shadow: 0 0 0 2px color-mix(in srgb, var(--theater-accent) 22%, transparent); }
.random-table-editor__input::placeholder { color: var(--sc-text-secondary, #a1a1aa); opacity: .72; }
.random-table-editor__entries-header { display: flex; align-items: center; justify-content: space-between; margin-top: 4px; }
.random-table-editor__entries { min-height: 96px; overflow: auto; display: grid; gap: 6px; }
.random-table-editor__entry { display: grid; grid-template-columns: 110px auto 110px minmax(180px, 1fr) 34px; align-items: center; gap: 6px; }
.random-table-editor__entry > span { color: var(--sc-text-secondary, #a1a1aa); font-size: 11px; }
.random-table-editor__empty { min-height: 96px; display: grid; place-items: center; color: var(--sc-text-secondary, #a1a1aa); font-size: 12px; }
.random-table-editor__error { margin: 0; color: var(--n-color-error, #ef4444); font-size: 12px; }
.random-table-editor__footer { justify-content: flex-end; border-top: 1px solid rgba(148, 163, 184, .18); }
@media (max-width: 680px) {
  .random-table-editor label { grid-template-columns: 1fr; }
  .random-table-editor__entry { grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) 34px; }
  .random-table-editor__entry > .random-table-editor__input:last-of-type { grid-column: 1 / -1; grid-row: 2; }
}
</style>
