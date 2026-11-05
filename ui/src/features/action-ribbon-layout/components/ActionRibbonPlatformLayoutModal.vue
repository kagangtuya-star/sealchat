<script setup lang="ts">
// 平台“更多跑团功能”布局方案管理：左侧方案列表，右侧所见即所得编辑。
// 编辑结果写回调用方的草稿，随“主题与样式管理”保存进入平台配置（版本化、可回滚）。
import { computed, ref, watch } from 'vue'
import { cloneDeep } from 'lodash-es'
import { NIcon, useMessage } from 'naive-ui'
import { Copy, Download, Plus, Star, Trash, Upload } from '@vicons/tabler'
import ActionRibbonLayoutBoard from './ActionRibbonLayoutBoard.vue'
import { ACTION_RIBBON_KEYS } from '../actionRibbonCatalog'
import {
  MAX_ACTION_RIBBON_PRESETS,
  MAX_ACTION_RIBBON_PRESET_NAME,
  completeActionRibbonOrder,
  createActionRibbonPresetId,
  exportActionRibbonPreset,
  exportActionRibbonPresets,
  mergeEditedActionRibbonShape,
  normalizeActionRibbonLayoutConfig,
  parsePlatformActionRibbonLayoutFile,
  type ActionRibbonLayoutConfig,
  type ActionRibbonLayoutPreset,
} from '../actionRibbonLayout'
import { downloadJSON } from '@/services/ui-customization/transfer'

const props = defineProps<{ show: boolean; modelValue?: ActionRibbonLayoutConfig | null }>()
const emit = defineEmits<{
  'update:show': [value: boolean]
  'update:modelValue': [value: ActionRibbonLayoutConfig]
}>()

const message = useMessage()
const knownKeys = ACTION_RIBBON_KEYS as string[]
const SYSTEM_ID = ''

const draft = ref<ActionRibbonLayoutConfig>(normalizeActionRibbonLayoutConfig(null))
const selectedId = ref<string>(SYSTEM_ID)
const importInputRef = ref<HTMLInputElement | null>(null)

watch(() => props.show, (show) => {
  if (!show) return
  draft.value = normalizeActionRibbonLayoutConfig(cloneDeep(props.modelValue))
  selectedId.value = draft.value.defaultPresetId || draft.value.presets[0]?.id || SYSTEM_ID
}, { immediate: true })

const selectedPreset = computed(() => draft.value.presets.find(item => item.id === selectedId.value) || null)
const isDefault = (id: string) => draft.value.defaultPresetId === id
const defaultName = computed(() => draft.value.presets.find(item => isDefault(item.id))?.name || '系统默认')

const editorOrder = computed(() => completeActionRibbonOrder(selectedPreset.value?.order || [], knownKeys))
const editorHidden = computed(() => (selectedPreset.value?.hidden || []).filter(key => knownKeys.includes(key)))

const touch = (preset: ActionRibbonLayoutPreset) => {
  preset.updatedAt = Date.now()
}

const updateShape = (patch: { order?: string[]; hidden?: string[] }) => {
  const preset = selectedPreset.value
  if (!preset) return
  const next = mergeEditedActionRibbonShape(
    { order: patch.order ?? editorOrder.value, hidden: patch.hidden ?? editorHidden.value },
    { order: preset.order, hidden: preset.hidden },
    knownKeys,
  )
  preset.order = next.order
  preset.hidden = next.hidden
  touch(preset)
}

const uniqueName = (base: string) => {
  const names = new Set(draft.value.presets.map(item => item.name))
  const trimmed = Array.from(base.trim() || '新方案').slice(0, MAX_ACTION_RIBBON_PRESET_NAME - 4).join('')
  if (!names.has(trimmed)) return trimmed
  for (let i = 2; i < 100; i++) {
    const candidate = `${trimmed} ${i}`
    if (!names.has(candidate)) return candidate
  }
  return trimmed
}

const canAdd = computed(() => draft.value.presets.length < MAX_ACTION_RIBBON_PRESETS)

const addPreset = (seed: { name: string; order: string[]; hidden: string[] }) => {
  if (!canAdd.value) {
    message.warning(`最多保存 ${MAX_ACTION_RIBBON_PRESETS} 个方案`)
    return null
  }
  const preset: ActionRibbonLayoutPreset = {
    id: createActionRibbonPresetId(),
    name: uniqueName(seed.name),
    order: [...seed.order],
    hidden: [...seed.hidden],
    updatedAt: Date.now(),
  }
  draft.value.presets.push(preset)
  selectedId.value = preset.id
  return preset
}

const createPreset = () => {
  // 新方案从系统默认顺序开始，并把当前全部入口写入 order，方便后续拖拽。
  addPreset({ name: '新方案', order: [...knownKeys], hidden: [] })
}

const duplicateSelected = () => {
  const source = selectedPreset.value
  if (source) {
    addPreset({ name: `${source.name} 副本`, order: source.order, hidden: source.hidden })
  } else {
    addPreset({ name: '系统默认 副本', order: [...knownKeys], hidden: [] })
  }
}

const renameSelected = (value: string) => {
  const preset = selectedPreset.value
  if (!preset) return
  preset.name = Array.from(value).slice(0, MAX_ACTION_RIBBON_PRESET_NAME).join('')
  touch(preset)
}

const normalizeNameOnBlur = () => {
  const preset = selectedPreset.value
  if (preset && !preset.name.trim()) preset.name = uniqueName('未命名方案')
}

const deleteSelected = () => {
  const preset = selectedPreset.value
  if (!preset) return
  const index = draft.value.presets.findIndex(item => item.id === preset.id)
  draft.value.presets.splice(index, 1)
  if (isDefault(preset.id)) draft.value.defaultPresetId = ''
  selectedId.value = draft.value.presets[Math.min(index, draft.value.presets.length - 1)]?.id || SYSTEM_ID
}

const setDefault = (id: string) => {
  draft.value.defaultPresetId = id
}

const resetSelected = () => {
  const preset = selectedPreset.value
  if (!preset) return
  updateShape({ order: [...knownKeys], hidden: [] })
  message.success('已恢复为系统默认顺序，全部入口可见')
}

const exportSelected = () => {
  const preset = selectedPreset.value
  if (!preset) return
  downloadJSON(exportActionRibbonPreset(preset), `sealchat-ribbon-preset-${preset.name}.json`)
}

const exportAll = () => {
  downloadJSON(exportActionRibbonPresets(draft.value), 'sealchat-ribbon-presets.json')
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const result = parsePlatformActionRibbonLayoutFile(await file.text())
    if (result.kind === 'preset') {
      if (addPreset(result.preset)) message.success(`已导入方案「${result.preset.name}」`)
      return
    }
    // 方案集合按 id 合并：同 id 覆盖（便于备份恢复后世界引用仍有效），其余追加。
    let added = 0
    let updated = 0
    for (const item of result.config.presets) {
      const existing = draft.value.presets.find(preset => preset.id === item.id)
      if (existing) {
        Object.assign(existing, { name: item.name, order: item.order, hidden: item.hidden, updatedAt: Date.now() })
        updated++
      } else if (draft.value.presets.length < MAX_ACTION_RIBBON_PRESETS) {
        draft.value.presets.push({ ...item, updatedAt: Date.now() })
        added++
      }
    }
    if (!result.config.defaultPresetId) {
      draft.value.defaultPresetId = ''
    } else if (draft.value.presets.some(item => item.id === result.config.defaultPresetId)) {
      draft.value.defaultPresetId = result.config.defaultPresetId
    }
    message.success(`已导入 ${added} 个方案${updated ? `，更新 ${updated} 个同名 ID 方案` : ''}`)
  } catch (error: any) {
    message.error(error?.message || '导入失败')
  }
}

const apply = () => {
  normalizeNameOnBlur()
  emit('update:modelValue', normalizeActionRibbonLayoutConfig(cloneDeep(draft.value)))
  emit('update:show', false)
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="更多跑团功能显示排序"
    class="sc-fluid-modal sc-fluid-modal--wide"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <input ref="importInputRef" type="file" accept=".json,application/json" hidden @change="handleImportFile">
    <div class="arl-platform">
      <aside class="arl-platform__side" aria-label="布局方案">
        <div class="arl-platform__side-head">
          <span>布局方案 {{ draft.presets.length }}/{{ MAX_ACTION_RIBBON_PRESETS }}</span>
        </div>
        <div class="arl-platform__list" role="listbox" aria-label="布局方案列表">
          <button
            type="button"
            role="option"
            class="arl-preset"
            :class="{ 'is-selected': selectedId === SYSTEM_ID }"
            :aria-selected="selectedId === SYSTEM_ID"
            @click="selectedId = SYSTEM_ID"
          >
            <span class="arl-preset__name">系统默认</span>
            <n-icon v-if="isDefault(SYSTEM_ID)" class="arl-preset__star" :component="Star" title="平台默认" />
          </button>
          <button
            v-for="preset in draft.presets"
            :key="preset.id"
            type="button"
            role="option"
            class="arl-preset"
            :class="{ 'is-selected': selectedId === preset.id }"
            :aria-selected="selectedId === preset.id"
            @click="selectedId = preset.id"
          >
            <span class="arl-preset__name">{{ preset.name || '未命名方案' }}</span>
            <n-icon v-if="isDefault(preset.id)" class="arl-preset__star" :component="Star" title="平台默认" />
          </button>
        </div>
        <div class="arl-platform__side-actions">
          <n-button size="small" secondary :disabled="!canAdd" @click="createPreset">
            <template #icon><n-icon :component="Plus" /></template>
            新建
          </n-button>
          <n-button size="small" secondary @click="importInputRef?.click()">
            <template #icon><n-icon :component="Upload" /></template>
            导入
          </n-button>
          <n-button size="small" quaternary :disabled="!draft.presets.length" @click="exportAll">导出全部</n-button>
        </div>
      </aside>

      <section class="arl-platform__main">
        <header class="arl-platform__head">
          <div v-if="selectedPreset" class="arl-platform__title">
            <n-input
              :value="selectedPreset.name"
              size="small"
              :maxlength="MAX_ACTION_RIBBON_PRESET_NAME"
              placeholder="方案名称"
              class="arl-platform__name-input"
              aria-label="方案名称"
              @update:value="renameSelected"
              @blur="normalizeNameOnBlur"
            />
            <n-tag v-if="isDefault(selectedPreset.id)" size="small" type="success" round :bordered="false">平台默认</n-tag>
          </div>
          <div v-else class="arl-platform__title">
            <strong>系统默认</strong>
            <n-tag v-if="isDefault(SYSTEM_ID)" size="small" type="success" round :bordered="false">平台默认</n-tag>
            <span class="arl-platform__muted">内置顺序，全部入口可见；不可编辑，可复制后修改。</span>
          </div>
          <div class="arl-platform__actions">
            <n-button v-if="!isDefault(selectedId)" size="small" secondary @click="setDefault(selectedId)">
              <template #icon><n-icon :component="Star" /></template>
              设为平台默认
            </n-button>
            <n-button size="small" secondary :disabled="!canAdd" @click="duplicateSelected">
              <template #icon><n-icon :component="Copy" /></template>
              复制
            </n-button>
            <template v-if="selectedPreset">
              <n-button size="small" secondary @click="exportSelected">
                <template #icon><n-icon :component="Download" /></template>
                导出
              </n-button>
              <n-popconfirm @positive-click="resetSelected">
                <template #trigger>
                  <n-button size="small" secondary>恢复默认</n-button>
                </template>
                将此方案恢复为系统默认顺序并显示全部入口？
              </n-popconfirm>
              <n-popconfirm @positive-click="deleteSelected">
                <template #trigger>
                  <n-button size="small" secondary type="error">
                    <template #icon><n-icon :component="Trash" /></template>
                    删除
                  </n-button>
                </template>
                删除后，引用此方案的世界会自动回退到平台默认方案{{ isDefault(selectedPreset.id) ? '；平台默认将改为系统默认' : '' }}。
              </n-popconfirm>
            </template>
          </div>
        </header>

        <ActionRibbonLayoutBoard
          :order="editorOrder"
          :hidden="editorHidden"
          :readonly="!selectedPreset"
          @update:order="updateShape({ order: $event })"
          @update:hidden="updateShape({ hidden: $event })"
        />

        <p class="arl-platform__note">
          顺序即聊天中的阅读顺序：宽屏时前几项直接显示在工具栏，其余收进「更多」；窄屏显示全部。
          带条件说明的入口只在条件满足时出现，配置无法让无权限或不可用的功能显示。
          当前平台默认：<strong>{{ defaultName }}</strong>，世界可选择跟随、引用某个方案或自定义。
        </p>
      </section>
    </div>

    <template #footer>
      <div class="arl-platform__footer">
        <span class="arl-platform__muted">应用后，请点击页面上的保存按钮使配置生效。</span>
        <n-space>
          <n-button @click="emit('update:show', false)">取消</n-button>
          <n-button type="primary" @click="apply">应用</n-button>
        </n-space>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.arl-platform {
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  gap: 16px;
  min-height: 360px;
}

.arl-platform__side {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  padding-right: 16px;
  border-right: 1px solid var(--sc-border-mute);
}

.arl-platform__side-head,
.arl-platform__muted {
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.arl-platform__list {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 4px;
  max-height: 420px;
  overflow-y: auto;
}

.arl-preset {
  display: flex;
  flex: none;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 8px 10px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: var(--sc-text-primary);
  font-size: 13px;
  text-align: left;
  cursor: var(--sc-cursor-pointer, pointer);
}

.arl-preset:hover {
  background: var(--sc-chip-bg);
}

.arl-preset.is-selected {
  border-color: var(--sc-border-strong);
  background: var(--sc-bg-active, var(--sc-chip-bg));
  font-weight: 600;
}

.arl-preset__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.arl-preset__star {
  flex: none;
  color: var(--sc-text-secondary);
}

.arl-platform__side-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.arl-platform__main {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.arl-platform__head {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.arl-platform__title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.arl-platform__name-input {
  max-width: 260px;
}

.arl-platform__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.arl-platform__note {
  margin: 0;
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.7;
}

.arl-platform__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

@media (max-width: 720px) {
  .arl-platform {
    grid-template-columns: minmax(0, 1fr);
    min-height: 0;
  }

  .arl-platform__side {
    padding: 0 0 12px;
    border-right: 0;
    border-bottom: 1px solid var(--sc-border-mute);
  }

  .arl-platform__list {
    flex-direction: row;
    max-height: none;
    overflow-x: auto;
    overflow-y: hidden;
  }

  .arl-preset {
    max-width: 180px;
  }
}
</style>
