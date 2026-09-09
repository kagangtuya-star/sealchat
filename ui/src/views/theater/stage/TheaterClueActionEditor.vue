<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NSelect } from 'naive-ui'
import { GripVertical, Plus, Trash, X } from '@vicons/tabler'
import type { StageAction, StageClueAccessMode, StageClueActionEntry, StageClueActionTarget } from '../shared/stage-types'
import { normalizeStageClueExecutePayload, STAGE_CLUE_MAX_ENTRIES } from '../shared/stage-actions'
import type { ChatClueAccessReadResult, ChatClueOptionsReadResult } from '../bridge/theater-bridge-protocol'

type ClueAction = Extract<StageAction, { type: 'clue.execute' }>
type EditableAction = Pick<ClueAction, 'payload'> & { id?: string }
type ClueOptions = Extract<ChatClueOptionsReadResult, { ok: true }>
type ClueAccess = Extract<ChatClueAccessReadResult, { ok: true }>
type ClueAccessItem = ClueAccess['items'][number]
type DraftEntry = StageClueActionEntry

const props = defineProps<{
  show: boolean
  componentName: string
  action: EditableAction | null
  readOptions?: () => Promise<ChatClueOptionsReadResult>
  readAccess?: (clueId: string) => Promise<ChatClueAccessReadResult>
}>()
const emit = defineEmits<{
  'update:show': [value: boolean]
  save: [payload: ClueAction['payload']]
}>()

const options = ref<ClueOptions | null>(null)
const search = ref('')
const entries = ref<DraftEntry[]>([])
const validationError = ref('')
const loading = ref(false)
const accessEntryId = ref('')
const accessItems = ref<Record<string, ClueAccessItem>>({})
const accessLoading = ref(false)
let accessRequestEpoch = 0
const rowElements = new Map<string, HTMLElement>()
const draggingId = ref('')
const dropIndex = ref(-1)
let dragFrame = 0
let dragY = 0

const clueById = computed(() => new Map((options.value?.clues || []).map(clue => [clue.id, clue])))
const filteredClues = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  return (options.value?.clues || []).filter(clue => !keyword || clue.title.toLowerCase().includes(keyword))
})
const clueOptions = computed(() => filteredClues.value.map(clue => ({
  label: `${clue.title} · ${clue.kind}${clue.sharedFolderId ? ` · ${clue.sharedFolderId}` : ''}`,
  value: clue.id,
})))
const accessOptions: Array<{ label: string, value: StageClueAccessMode }> = [
  { label: '保持', value: 'keep' },
  { label: '继承', value: 'inherit' },
  { label: '不可见', value: 'none' },
  { label: '查看', value: 'view' },
  { label: '编辑', value: 'edit' },
]
const clueSelectMenuProps = { class: 'theater-clue-select-menu' }
const accessEntry = computed(() => entries.value.find(entry => entry.id === accessEntryId.value) || null)
const accessSummary = (entry: DraftEntry) => {
  const names = entry.targets
    .filter(target => target.access !== 'keep')
    .map(target => {
      const member = options.value?.roster.find(item => item.userId === target.userId)
      const name = member?.nickname || member?.username || target.userId
      return `${name} · ${target.access}`
    })
  return names.length <= 2 ? names.join('、') || '保持当前权限' : `已配置 ${names.length} 人`
}
const displayClue = (clueId: string) => clueById.value.get(clueId)?.title || clueId || '选择线索'

const resetDraft = () => {
  accessRequestEpoch += 1
  options.value = null
  entries.value = props.action?.payload.entries.map(entry => ({
    ...entry,
    targets: entry.targets.map(target => ({ ...target })),
  })) || []
  validationError.value = ''
  search.value = ''
  accessEntryId.value = ''
  accessItems.value = {}
  accessLoading.value = false
}

const loadOptions = async () => {
  if (!props.readOptions) return
  loading.value = true
  try {
    const result = await props.readOptions()
    if (result.ok) options.value = result
    else validationError.value = result.error.message
  } catch (error) {
    validationError.value = error instanceof Error ? error.message : '线索读取失败'
  } finally {
    loading.value = false
  }
}

watch(() => [props.show, props.action?.id] as const, ([show]) => {
  if (!show) return
  resetDraft()
  void loadOptions()
})

const addEntry = () => {
  if (entries.value.length >= STAGE_CLUE_MAX_ENTRIES) return
  const clueId = options.value?.clues[0]?.id || ''
  entries.value.push({ id: `entry-${Date.now()}-${entries.value.length}`, clueId, targets: [], present: true, confirm: false })
}
const removeEntry = (id: string) => { entries.value = entries.value.filter(entry => entry.id !== id) }
const setRowElement = (id: string, element: Element | null) => {
  if (element instanceof HTMLElement) rowElements.set(id, element)
  else rowElements.delete(id)
}
const updateDropIndex = () => {
  dragFrame = 0
  const rows = entries.value.map(entry => rowElements.get(entry.id)).filter((row): row is HTMLElement => Boolean(row))
  let index = rows.length
  for (let current = 0; current < rows.length; current += 1) {
    const rect = rows[current].getBoundingClientRect()
    if (dragY < rect.top + rect.height / 2) { index = current; break }
  }
  dropIndex.value = index
}
const moveDrag = (event: PointerEvent) => {
  dragY = event.clientY
  if (!dragFrame) dragFrame = requestAnimationFrame(updateDropIndex)
}
const stopDrag = () => {
  window.removeEventListener('pointermove', moveDrag)
  window.removeEventListener('pointerup', stopDrag)
  window.removeEventListener('pointercancel', stopDrag)
  if (dragFrame) cancelAnimationFrame(dragFrame)
  dragFrame = 0
  const sourceId = draggingId.value
  const target = dropIndex.value
  draggingId.value = ''
  dropIndex.value = -1
  const source = entries.value.findIndex(entry => entry.id === sourceId)
  if (source < 0 || target < 0) return
  const [entry] = entries.value.splice(source, 1)
  const adjusted = target > source ? target - 1 : target
  entries.value.splice(Math.max(0, Math.min(adjusted, entries.value.length)), 0, entry)
}
const startDrag = (id: string, event: PointerEvent) => {
  if (event.button !== 0) return
  event.preventDefault()
  draggingId.value = id
  dragY = event.clientY
  updateDropIndex()
  window.addEventListener('pointermove', moveDrag)
  window.addEventListener('pointerup', stopDrag, { once: true })
  window.addEventListener('pointercancel', stopDrag, { once: true })
}
const moveByKeyboard = (id: string, offset: -1 | 1) => {
  const index = entries.value.findIndex(entry => entry.id === id)
  const target = index + offset
  if (index < 0 || target < 0 || target >= entries.value.length) return
  const [entry] = entries.value.splice(index, 1)
  entries.value.splice(target, 0, entry)
}

const targetAccess = (entry: DraftEntry, userId: string) => entry.targets.find(target => target.userId === userId)?.access || 'keep'
const setTargetAccess = (entry: DraftEntry, userId: string, access: StageClueAccessMode) => {
  const index = entry.targets.findIndex(target => target.userId === userId)
  if (access === 'keep') {
    if (index >= 0) entry.targets.splice(index, 1)
    return
  }
  const target: StageClueActionTarget = { userId, access }
  if (index >= 0) entry.targets[index] = target
  else entry.targets.push(target)
}
const openAccess = async (entry: DraftEntry) => {
  const requestEpoch = ++accessRequestEpoch
  accessEntryId.value = entry.id
  accessItems.value = {}
  accessLoading.value = false
  if (!props.readAccess || !entry.clueId) return
  accessLoading.value = true
  try {
    const result = await props.readAccess(entry.clueId)
    if (result.ok && accessEntryId.value === entry.id && requestEpoch === accessRequestEpoch) {
      accessItems.value = Object.fromEntries(result.items.map(item => [item.userId, item]))
    } else if (!result.ok && accessEntryId.value === entry.id && requestEpoch === accessRequestEpoch) {
      validationError.value = result.error.message
    }
  } catch (error) {
    if (accessEntryId.value === entry.id && requestEpoch === accessRequestEpoch) {
      validationError.value = error instanceof Error ? error.message : '权限读取失败'
    }
  } finally {
    if (requestEpoch === accessRequestEpoch) accessLoading.value = false
  }
}
const closeAccess = () => {
  accessRequestEpoch += 1
  accessEntryId.value = ''
}
const save = () => {
  const payload = normalizeStageClueExecutePayload({ version: 1, entries: entries.value })
  if (!payload) { validationError.value = '配置无效：至少添加一条线索，且线索与条目标识不能为空。'; return }
  emit('save', payload)
  emit('update:show', false)
}
const close = () => emit('update:show', false)
onBeforeUnmount(() => {
  window.removeEventListener('pointermove', moveDrag)
  window.removeEventListener('pointerup', stopDrag)
  window.removeEventListener('pointercancel', stopDrag)
  if (dragFrame) cancelAnimationFrame(dragFrame)
})
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="clue-editor-modal">
      <section class="clue-editor" role="dialog" aria-modal="true" aria-label="线索动作编辑器" @pointerdown.stop @click.stop @keydown.stop>
        <header class="clue-editor__header"><div><strong>{{ componentName }} · 线索</strong><small>按条目顺序逐条执行</small></div><n-button text aria-label="关闭线索编辑器" @click="close"><n-icon><X /></n-icon></n-button></header>
        <div class="clue-editor__body">
          <div class="clue-editor__toolbar"><n-input v-model:value="search" clearable placeholder="搜索线索（按名称）" /><n-button size="small" secondary :disabled="entries.length >= STAGE_CLUE_MAX_ENTRIES" @click="addEntry"><template #icon><n-icon><Plus /></n-icon></template>添加线索</n-button></div>
          <div v-if="loading" class="clue-editor__empty">正在读取线索…</div>
          <div v-else class="clue-editor__rows">
            <article v-for="(entry, index) in entries" :key="entry.id" :ref="element => setRowElement(entry.id, element as Element | null)" class="clue-editor__row" :class="{ 'is-dragging': draggingId === entry.id, 'is-drop-before': draggingId && dropIndex === index, 'is-drop-after': draggingId && dropIndex === entries.length && index === entries.length - 1 }">
              <button type="button" class="clue-editor__handle" aria-label="拖动排序" @pointerdown="startDrag(entry.id, $event)" @keydown.alt.up.prevent="moveByKeyboard(entry.id, -1)" @keydown.alt.down.prevent="moveByKeyboard(entry.id, 1)"><n-icon><GripVertical /></n-icon></button>
              <div class="clue-editor__select-wrap"><n-select v-model:value="entry.clueId" size="small" :options="clueOptions" filterable :menu-props="clueSelectMenuProps" placeholder="选择线索" /><small>{{ displayClue(entry.clueId) }}</small></div>
              <button type="button" class="clue-editor__access" @click="openAccess(entry)"><strong>{{ accessSummary(entry) }}</strong><small>{{ entry.targets.filter(target => target.access !== 'keep').length ? '权限配置' : '未配置权限' }}</small></button>
              <label class="clue-editor__toggle"><input v-model="entry.present" type="checkbox" />同时展示</label>
              <label class="clue-editor__toggle"><input v-model="entry.confirm" type="checkbox" />二次确认</label>
              <n-button text type="error" aria-label="删除线索" @click="removeEntry(entry.id)"><n-icon><Trash /></n-icon></n-button>
            </article>
            <div v-if="!entries.length" class="clue-editor__empty">至少添加一条线索。</div>
          </div>
          <p v-if="validationError" class="clue-editor__error" role="alert">{{ validationError }}</p>
        </div>
        <footer class="clue-editor__footer"><n-button secondary :disabled="entries.length >= STAGE_CLUE_MAX_ENTRIES" @click="addEntry"><template #icon><n-icon><Plus /></n-icon></template>添加线索</n-button><span></span><div><n-button @click="close">取消</n-button><n-button type="primary" @click="save">保存</n-button></div></footer>
      </section>
    </div>
    <div v-if="accessEntry" class="clue-access-modal" @pointerdown.stop @click.stop>
      <section class="clue-access-dialog" role="dialog" aria-modal="true" aria-label="权限分配">
        <header class="clue-access-dialog__header">
          <div><strong>权限分配</strong><small>{{ displayClue(accessEntry.clueId) }}</small></div>
          <n-button text aria-label="关闭权限分配" @click="closeAccess"><n-icon><X /></n-icon></n-button>
        </header>
        <div class="clue-access">
          <small v-if="accessLoading">正在读取当前权限…</small>
          <div v-for="member in options?.roster || []" :key="member.userId" class="clue-access__row">
            <span>{{ member.nickname || member.username || member.userId }}<small v-if="accessItems[member.userId]">当前 {{ accessItems[member.userId].effectiveAccess }}</small></span>
            <n-select size="small" :value="targetAccess(accessEntry, member.userId)" :options="accessOptions" :menu-props="clueSelectMenuProps" @update:value="value => setTargetAccess(accessEntry!, member.userId, value as StageClueAccessMode)" />
          </div>
        </div>
        <footer class="clue-access-dialog__footer"><n-button type="primary" @click="closeAccess">完成</n-button></footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.clue-editor-modal, .clue-access-modal { --theater-accent: #3b82f6; --theater-panel: color-mix(in srgb, var(--sc-bg-surface, #262626) 48%, transparent); --theater-panel-muted: color-mix(in srgb, var(--sc-bg-layer, #3f3f46) 56%, transparent); --theater-border: var(--sc-border-strong, rgba(255, 255, 255, .16)); }
.clue-editor-modal { position: fixed; z-index: 10005; inset: 0; display: grid; place-items: center; padding: 16px; pointer-events: auto; background: rgba(0, 0, 0, .24); }
.clue-editor { width: min(980px, calc(100vw - 32px)); max-height: min(760px, calc(100vh - 32px)); display: flex; flex-direction: column; border: 1px solid var(--theater-border); border-radius: 7px; color: var(--sc-text-primary, #f4f4f5); background: var(--theater-panel); box-shadow: 0 14px 34px rgba(0, 0, 0, .2); backdrop-filter: blur(8px) saturate(110%); -webkit-backdrop-filter: blur(8px) saturate(110%); }
.clue-editor__header, .clue-editor__footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 16px; }.clue-editor__header { border-bottom: 1px solid rgba(148, 163, 184, .18); }.clue-editor__header div { display: grid; gap: 3px; }.clue-editor__header small { color: var(--sc-text-secondary, #a1a1aa); font-size: 11px; }.clue-editor__body { min-height: 0; overflow: auto; display: grid; gap: 12px; padding: 16px; }.clue-editor__toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; }.clue-editor__rows { display: grid; gap: 6px; }.clue-editor__row { position: relative; display: grid; grid-template-columns: 28px minmax(180px, 1.4fr) minmax(150px, 1fr) auto auto 28px; align-items: center; gap: 7px; padding: 8px; border: 1px solid var(--theater-border); border-radius: 5px; background: var(--theater-panel-muted); }.clue-editor__row.is-dragging { opacity: .42; }.clue-editor__row.is-drop-before::before, .clue-editor__row.is-drop-after::after { position: absolute; right: 4px; left: 4px; height: 2px; content: ''; background: var(--theater-accent); }.clue-editor__row.is-drop-before::before { top: -2px; }.clue-editor__row.is-drop-after::after { bottom: -2px; }.clue-editor__handle { width: 24px; height: 34px; display: grid; place-items: center; border: 0; color: var(--sc-text-secondary); background: transparent; cursor: grab; touch-action: none; }.clue-editor__select-wrap { min-width: 0; display: grid; gap: 3px; }.clue-editor__select { min-width: 0; height: 32px; padding: 0 8px; border: 1px solid var(--theater-border); border-radius: 4px; color: var(--sc-text-primary); background: var(--theater-panel-muted); }.clue-editor__select-wrap small, .clue-editor__access small { overflow: hidden; color: var(--sc-text-secondary); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }.clue-editor__access { min-width: 0; display: grid; gap: 3px; padding: 4px; border: 1px solid transparent; color: var(--sc-text-primary); background: transparent; text-align: left; cursor: pointer; }.clue-editor__access:hover { border-color: var(--theater-accent); }.clue-editor__access strong { overflow: hidden; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }.clue-editor__toggle { display: inline-flex; align-items: center; gap: 4px; color: var(--sc-text-secondary); font-size: 11px; white-space: nowrap; }.clue-editor__empty { min-height: 110px; display: grid; place-items: center; color: var(--sc-text-secondary); font-size: 12px; }.clue-editor__error { margin: 0; color: var(--n-color-error, #ef4444); font-size: 12px; }.clue-editor__footer { justify-content: flex-end; border-top: 1px solid rgba(148, 163, 184, .18); }
.clue-access-modal { position: fixed; z-index: 10010; inset: 0; display: grid; place-items: center; padding: 16px; background: rgba(0, 0, 0, .42); backdrop-filter: blur(6px) saturate(110%); -webkit-backdrop-filter: blur(6px) saturate(110%); }
.clue-access-dialog { width: min(560px, calc(100vw - 32px)); max-height: min(680px, calc(100vh - 32px)); display: flex; flex-direction: column; overflow: hidden; border: 1px solid var(--theater-border); border-radius: 7px; color: var(--sc-text-primary, #f4f4f5); background: var(--theater-panel); box-shadow: 0 14px 34px rgba(0, 0, 0, .28); backdrop-filter: blur(8px) saturate(110%); -webkit-backdrop-filter: blur(8px) saturate(110%); }
.clue-access-dialog__header, .clue-access-dialog__footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 16px; }.clue-access-dialog__header { border-bottom: 1px solid rgba(148, 163, 184, .18); }.clue-access-dialog__header div { min-width: 0; display: grid; gap: 3px; }.clue-access-dialog__header small { overflow: hidden; color: var(--sc-text-secondary, #a1a1aa); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }.clue-access-dialog__footer { justify-content: flex-end; border-top: 1px solid rgba(148, 163, 184, .18); }.clue-access { display: grid; gap: 8px; max-height: 60vh; overflow: auto; padding: 16px; }.clue-access__row { display: grid; grid-template-columns: minmax(0, 1fr) 132px; align-items: center; gap: 8px; }.clue-access__row > span { min-width: 0; display: grid; gap: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.clue-access__row > span small { color: var(--sc-text-secondary); font-size: 10px; }
:global(.v-binder-follower-container:has(.n-select-menu.n-base-select-menu.theater-clue-select-menu)) { z-index: 10020 !important; }
:global(.n-select-menu.n-base-select-menu.theater-clue-select-menu) { --n-color: rgba(15, 23, 42, .94) !important; --n-option-text-color: #f8fafc !important; --n-option-text-color-active: #f8fafc !important; --n-option-text-color-pressed: #f8fafc !important; --n-option-check-color: var(--theater-accent, #60a5fa) !important; --n-option-color-active: rgba(96, 165, 250, .22) !important; --n-option-color-pending: rgba(96, 165, 250, .16) !important; --n-option-color-active-pending: rgba(96, 165, 250, .28) !important; border: 1px solid rgba(148, 163, 184, .24) !important; color: #f8fafc !important; background: rgba(15, 23, 42, .94) !important; box-shadow: 0 16px 42px rgba(0, 0, 0, .48) !important; backdrop-filter: blur(14px) saturate(120%); -webkit-backdrop-filter: blur(14px) saturate(120%); }
:global(.n-select-menu.n-base-select-menu.theater-clue-select-menu .n-base-select-option) { color: #f8fafc !important; background-color: transparent !important; }
:global(.n-select-menu.n-base-select-menu.theater-clue-select-menu .n-base-select-option--pending), :global(.n-select-menu.n-base-select-menu.theater-clue-select-menu .n-base-select-option:hover) { background-color: rgba(96, 165, 250, .16) !important; }
@media (max-width: 760px) { .clue-editor__row { grid-template-columns: 26px minmax(0, 1fr) 28px; }.clue-editor__row > .clue-editor__access, .clue-editor__row > .clue-editor__toggle { grid-column: 2; }.clue-editor__row > :last-child { grid-column: 3; grid-row: 1; }.clue-editor__toolbar { grid-template-columns: 1fr; } }
</style>
