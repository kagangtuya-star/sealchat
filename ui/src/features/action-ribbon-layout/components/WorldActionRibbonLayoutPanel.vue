<script setup lang="ts">
// 世界级“更多跑团功能”布局：跟随平台默认 / 引用平台方案 / 在基础方案上差异覆写。
// 保存到世界的轻量 JSON 字段；平台方案按 id 引用，平台修改后世界自动同步。
import { computed, onMounted, ref, watch } from 'vue'
import { NIcon, useMessage } from 'naive-ui'
import { AlertTriangle, ArrowBackUp, Download, Upload } from '@vicons/tabler'
import ActionRibbonLayoutBoard from './ActionRibbonLayoutBoard.vue'
import { ACTION_RIBBON_KEYS } from '../actionRibbonCatalog'
import {
  buildWorldCustomLayout,
  completeActionRibbonOrder,
  countWorldOverrides,
  exportWorldActionRibbonLayout,
  findActionRibbonPreset,
  normalizeActionRibbonLayoutConfig,
  normalizeWorldActionRibbonLayout,
  parseWorldActionRibbonLayoutFile,
  resolveActionRibbonLayout,
  type ActionRibbonLayoutShape,
  type WorldActionRibbonLayout,
  type WorldActionRibbonLayoutMode,
} from '../actionRibbonLayout'
import { downloadJSON } from '@/services/ui-customization/transfer'
import { useChatStore } from '@/stores/chat'
import { useUtilsStore } from '@/stores/utils'

const props = defineProps<{ worldId: string; worldName?: string }>()

const chat = useChatStore()
const utils = useUtilsStore()
const message = useMessage()
const knownKeys = ACTION_RIBBON_KEYS as string[]

const platform = computed(() => normalizeActionRibbonLayoutConfig(utils.config?.actionRibbonLayout))
// 以 store 中的世界为准：本页保存和其他管理员保存（world-updated 广播）都会更新它。
const storedLayout = computed(() => normalizeWorldActionRibbonLayout(
  (chat.worldDetailMap[props.worldId]?.world || chat.worldMap[props.worldId])?.actionRibbonLayout,
))
const saved = ref<WorldActionRibbonLayout>(storedLayout.value)

const mode = ref<WorldActionRibbonLayoutMode>('inherit')
/** preset 模式下引用的方案。 */
const presetId = ref('')
/** custom 模式下的基础方案（空为平台默认）。 */
const basePresetId = ref('')
/** custom 模式下编辑中的最终形态（已补全的已知入口）。 */
const customShape = ref<ActionRibbonLayoutShape>({ order: [...knownKeys], hidden: [] })
const saving = ref(false)
const importInputRef = ref<HTMLInputElement | null>(null)

const toEditorShape = (shape: ActionRibbonLayoutShape): ActionRibbonLayoutShape => ({
  order: completeActionRibbonOrder(shape.order, knownKeys),
  hidden: shape.hidden.filter(key => knownKeys.includes(key)),
})

const baseShapeFor = (id: string) => toEditorShape(resolveActionRibbonLayout(platform.value, { mode: 'custom', presetId: id }))

const loadDraft = (layout: WorldActionRibbonLayout) => {
  mode.value = layout.mode
  presetId.value = layout.mode === 'preset' ? layout.presetId || '' : ''
  basePresetId.value = layout.mode === 'custom' ? layout.presetId || '' : ''
  customShape.value = toEditorShape(resolveActionRibbonLayout(platform.value, layout))
}

loadDraft(saved.value)

// 外部更新到达时只在没有未保存修改的情况下刷新草稿，避免覆盖正在进行的编辑。
watch(() => JSON.stringify(storedLayout.value), () => {
  const keepDraft = dirty.value
  saved.value = storedLayout.value
  if (!keepDraft) loadDraft(saved.value)
})

// 平台配置可能稍后才加载完成；草稿未改动时随之刷新。
watch(platform, () => {
  if (!dirty.value) loadDraft(saved.value)
})

onMounted(() => {
  if (!utils.config) void utils.configGet().catch(() => undefined)
})

const draftLayout = computed<WorldActionRibbonLayout>(() => {
  if (mode.value === 'preset' && presetId.value) {
    return normalizeWorldActionRibbonLayout({ mode: 'preset', presetId: presetId.value })
  }
  if (mode.value === 'custom') {
    return buildWorldCustomLayout(basePresetId.value, baseShapeFor(basePresetId.value), customShape.value, knownKeys, saved.value)
  }
  return normalizeWorldActionRibbonLayout({ mode: 'inherit' })
})

const dirty = computed(() => JSON.stringify(draftLayout.value) !== JSON.stringify(saved.value))
const resolved = computed(() => resolveActionRibbonLayout(platform.value, draftLayout.value))
const previewShape = computed(() => (mode.value === 'custom' ? customShape.value : toEditorShape(resolved.value)))
const customBaseShape = computed(() => baseShapeFor(basePresetId.value))
const overrides = computed(() => countWorldOverrides(draftLayout.value))

const platformDefaultName = computed(() => resolved.value.platformDefault?.name || '系统默认')
const presetOptions = computed(() => {
  const options = platform.value.presets.map(item => ({
    label: item.id === platform.value.defaultPresetId ? `${item.name}（平台默认）` : item.name,
    value: item.id,
  }))
  const missing = (mode.value === 'preset' ? presetId.value : basePresetId.value)
  if (missing && !findActionRibbonPreset(platform.value, missing)) {
    options.push({ label: '（已删除的方案）', value: missing })
  }
  return options
})
const baseOptions = computed(() => [{ label: `平台默认（${platformDefaultName.value}）`, value: '' }, ...presetOptions.value])

const modeCards: Array<{ value: WorldActionRibbonLayoutMode; title: string; desc: string }> = [
  { value: 'inherit', title: '跟随平台默认', desc: '平台调整默认方案后自动同步' },
  { value: 'preset', title: '使用平台方案', desc: '固定引用一个平台方案，方案更新后同步' },
  { value: 'custom', title: '本世界自定义', desc: '在某个方案基础上调整排序与显隐' },
]

const selectMode = (next: WorldActionRibbonLayoutMode) => {
  if (next === mode.value) return
  if (next === 'preset' && !presetId.value) {
    presetId.value = resolved.value.basePreset?.id || platform.value.presets[0]?.id || ''
    if (!presetId.value) {
      message.warning('平台尚未提供布局方案，请联系平台管理员或选择自定义')
      return
    }
  }
  if (next === 'custom') {
    // 以当前看到的效果为起点，保证切换瞬间预览不跳变；跟随平台默认时基础方案也跟随平台默认。
    basePresetId.value = mode.value === 'preset' && !resolved.value.missingPresetId ? presetId.value : ''
    customShape.value = toEditorShape(resolved.value)
  }
  mode.value = next
}

const changeBasePreset = (id: string) => {
  // 把现有覆写平移到新的基础方案上，而不是丢弃。
  const current = buildWorldCustomLayout(basePresetId.value, customBaseShape.value, customShape.value, knownKeys, saved.value)
  basePresetId.value = id
  customShape.value = toEditorShape(resolveActionRibbonLayout(platform.value, { ...current, presetId: id }))
}

const clearOverrides = () => {
  customShape.value = { ...customBaseShape.value }
}

const statusText = computed(() => {
  const base = resolved.value.basePreset?.name || '系统默认'
  if (mode.value === 'inherit') return `跟随平台默认，当前为「${platformDefaultName.value}」`
  if (mode.value === 'preset') return `引用平台方案「${base}」`
  const parts: string[] = []
  if (overrides.value.reordered) parts.push('排序')
  if (overrides.value.visibility) parts.push(`${overrides.value.visibility} 项显隐`)
  return parts.length ? `基于「${base}」，世界已覆写：${parts.join('、')}` : `基于「${base}」，暂无覆写`
})

const save = async () => {
  const id = props.worldId
  const layout = draftLayout.value
  if (!id) return
  saving.value = true
  try {
    const data = await chat.worldUpdate(id, { actionRibbonLayout: layout })
    if (id !== props.worldId) return
    saved.value = normalizeWorldActionRibbonLayout(data?.world?.actionRibbonLayout ?? layout)
    loadDraft(saved.value)
    message.success('已保存世界入口布局')
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const discard = () => loadDraft(saved.value)

const restoreInherit = () => {
  mode.value = 'inherit'
}

const handleExport = () => {
  const name = resolved.value.basePreset?.name
  downloadJSON(exportWorldActionRibbonLayout(draftLayout.value, name), `sealchat-ribbon-world-${props.worldName || props.worldId}.json`)
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const { layout, presetName } = parseWorldActionRibbonLayoutFile(await file.text())
    loadDraft(layout)
    const missing = resolveActionRibbonLayout(platform.value, layout).missingPresetId
    if (missing) {
      message.warning(`文件引用的平台方案${presetName ? `「${presetName}」` : ''}在本平台不存在，将回退到平台默认方案`)
    } else {
      message.success('已导入到草稿，确认无误后点击保存')
    }
  } catch (error: any) {
    message.error(error?.message || '导入失败')
  }
}
</script>

<template>
  <div class="arl-world">
    <input ref="importInputRef" type="file" accept=".json,application/json" hidden @change="handleImportFile">

    <div class="arl-world__modes" role="radiogroup" aria-label="布局来源">
      <button
        v-for="card in modeCards"
        :key="card.value"
        type="button"
        role="radio"
        class="arl-world__mode"
        :class="{ 'is-active': mode === card.value }"
        :aria-checked="mode === card.value"
        @click="selectMode(card.value)"
      >
        <span class="arl-world__mode-title">{{ card.title }}</span>
        <span class="arl-world__mode-desc">{{ card.desc }}</span>
      </button>
    </div>

    <div class="arl-world__controls">
      <div v-if="mode === 'preset'" class="arl-world__field">
        <span class="arl-world__label">平台方案</span>
        <n-select v-model:value="presetId" size="small" :options="presetOptions" class="arl-world__select" />
      </div>
      <div v-else-if="mode === 'custom'" class="arl-world__field">
        <span class="arl-world__label">基础方案</span>
        <n-select :value="basePresetId" size="small" :options="baseOptions" class="arl-world__select" @update:value="changeBasePreset" />
        <n-button size="small" quaternary :disabled="!overrides.reordered && !overrides.visibility" @click="clearOverrides">
          <template #icon><n-icon :component="ArrowBackUp" /></template>
          清除覆写
        </n-button>
      </div>
      <div class="arl-world__status">
        <n-tag size="small" round :bordered="false" :type="mode === 'custom' ? 'warning' : mode === 'preset' ? 'info' : 'default'">
          {{ mode === 'inherit' ? '继承平台' : mode === 'preset' ? '引用方案' : '世界自定义' }}
        </n-tag>
        <span>{{ statusText }}</span>
        <n-tag v-if="dirty" size="small" round :bordered="false" type="warning">未保存</n-tag>
      </div>
      <div v-if="resolved.missingPresetId" class="arl-world__warning" role="status">
        <n-icon :component="AlertTriangle" />
        <span>引用的平台方案已被删除，当前已回退到平台默认「{{ platformDefaultName }}」。可重新选择方案或改为跟随平台默认。</span>
      </div>
    </div>

    <ActionRibbonLayoutBoard
      :order="previewShape.order"
      :hidden="previewShape.hidden"
      :readonly="mode !== 'custom'"
      :base-hidden="mode === 'custom' ? customBaseShape.hidden : null"
      @update:order="customShape = { ...customShape, order: $event }"
      @update:hidden="customShape = { ...customShape, hidden: $event }"
    />
    <p class="arl-world__note">
      <template v-if="mode !== 'custom'">当前为只读预览；如需调整，请选择「本世界自定义」。</template>
      带条件说明的入口只在条件满足时出现；此处配置只影响显示与顺序，不会开放成员无权使用的功能。
    </p>

    <footer class="arl-world__footer">
      <div class="arl-world__footer-group">
        <n-button size="small" secondary @click="importInputRef?.click()">
          <template #icon><n-icon :component="Upload" /></template>
          导入
        </n-button>
        <n-button size="small" secondary @click="handleExport">
          <template #icon><n-icon :component="Download" /></template>
          导出
        </n-button>
        <n-button v-if="mode !== 'inherit'" size="small" quaternary @click="restoreInherit">恢复跟随平台</n-button>
      </div>
      <div class="arl-world__footer-group">
        <n-button size="small" :disabled="!dirty || saving" @click="discard">放弃修改</n-button>
        <n-button size="small" type="primary" :disabled="!dirty" :loading="saving" @click="save">保存</n-button>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.arl-world {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}

.arl-world__modes {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.arl-world__mode {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  padding: 10px 12px;
  border: 1px solid var(--sc-border-strong);
  border-radius: 10px;
  background: transparent;
  color: var(--sc-text-primary);
  text-align: left;
  cursor: var(--sc-cursor-pointer, pointer);
}

.arl-world__mode:hover {
  background: var(--sc-chip-bg);
}

.arl-world__mode.is-active {
  border-color: var(--sc-text-primary);
  background: var(--sc-chip-bg);
  box-shadow: inset 0 0 0 1px var(--sc-text-primary);
}

.arl-world__mode-title {
  font-size: 14px;
  font-weight: 600;
}

.arl-world__mode-desc,
.arl-world__label,
.arl-world__note,
.arl-world__status {
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.arl-world__controls {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.arl-world__field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.arl-world__select {
  width: 260px;
  max-width: 100%;
}

.arl-world__status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.arl-world__warning {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid var(--sc-border-strong);
  border-radius: 8px;
  background: var(--sc-chip-bg);
  color: var(--sc-text-primary);
  font-size: 12px;
  line-height: 1.6;
}

.arl-world__warning .n-icon {
  flex: none;
  margin-top: 2px;
}

.arl-world__note {
  margin: 0;
}

.arl-world__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--sc-border-mute);
}

.arl-world__footer-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 640px) {
  .arl-world__modes {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
