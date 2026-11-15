<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NColorPicker,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpace,
  NSwitch,
  type SelectOption,
} from 'naive-ui'
import { ArrowDown, ArrowUp, Copy, Refresh, Trash } from '@vicons/tabler'
import {
  changeWorkbenchColumns,
  changeWorkbenchRows,
  minimumWorkbenchRows,
  setWorkbenchPanelRect,
  updateWorkbenchPanel,
  WORKBENCH_LIMITS,
  cloneWorkbenchLayout,
  reorderWorkbenchPanel,
} from './layout'
import { loadWorkbenchResources, type WorkbenchResourceOption } from './resources'
import { resolveWorkbenchWebUrl, WORKBENCH_SURFACE_LABELS, workbenchSourceChannelId } from './sources'
import type { WorkbenchLayout, WorkbenchRect, WorkbenchSource, WorkbenchSurfaceType } from './types'

const props = defineProps<{
  layout: WorkbenchLayout
  panelId: string
  worldId: string
  channelId: string
  channelOptions: SelectOption[]
}>()

const emit = defineEmits<{
  change: [layout: WorkbenchLayout]
  rejected: [message: string]
  duplicate: [id: string]
  remove: [id: string]
}>()

const panel = computed(() => props.layout.panels.find(item => item.id === props.panelId) || null)
const panelIndex = computed(() => props.layout.panels.findIndex(item => item.id === props.panelId))

const kindOptions: SelectOption[] = [
  ...Object.entries(WORKBENCH_SURFACE_LABELS).map(([surface, label]) => ({ label, value: `surface:${surface}` })),
  { label: '聊天（Embed Chat）', value: 'chat' },
  { label: '小剧场', value: 'theater' },
  { label: '外部网页', value: 'web' },
]
const sourceKind = computed(() => {
  const source = panel.value?.source
  if (!source) return ''
  return source.type === 'surface' ? `surface:${source.surface}` : source.type
})

const patchPanel = (patch: Parameters<typeof updateWorkbenchPanel>[2]) => {
  if (!panel.value) return
  emit('change', updateWorkbenchPanel(props.layout, panel.value.id, patch))
}
const patchSource = (patch: Partial<WorkbenchSource>) => {
  if (!panel.value) return
  const source: WorkbenchSource = { ...panel.value.source, ...patch }
  for (const key of Object.keys(source) as (keyof WorkbenchSource)[]) {
    if (source[key] === '' || source[key] === undefined) delete source[key]
  }
  patchPanel({ source })
}

const changeKind = (value: string) => {
  if (!panel.value || value === sourceKind.value) return
  const source: WorkbenchSource = value.startsWith('surface:')
    ? { type: 'surface', surface: value.slice('surface:'.length) as WorkbenchSurfaceType }
    : { type: value as WorkbenchSource['type'] }
  if (source.surface === 'clue-board') source.resourceId = 'main'
  patchPanel({ source })
}

// ---------------------------------------------------------------------------
// 资源目录（只读）：按 surface + 上下文频道加载，过期响应丢弃。

const resourceOptions = ref<WorkbenchResourceOption[]>([])
const resourceLoading = ref(false)
const resourceError = ref('')
const resourceLoadedKey = ref('')
let resourceEpoch = 0

const resourceContextKey = computed(() => {
  const source = panel.value?.source
  if (!source || source.type !== 'surface' || !source.surface) return ''
  return `${source.surface}\u0000${workbenchSourceChannelId(source, { worldId: props.worldId, channelId: props.channelId })}`
})

const loadResources = async () => {
  const key = resourceContextKey.value
  const source = panel.value?.source
  const epoch = ++resourceEpoch
  if (!key || !source?.surface) {
    resourceOptions.value = []
    resourceLoadedKey.value = ''
    return
  }
  resourceLoading.value = true
  resourceError.value = ''
  try {
    const options = await loadWorkbenchResources(source.surface, {
      worldId: props.worldId,
      channelId: workbenchSourceChannelId(source, { worldId: props.worldId, channelId: props.channelId }),
    })
    if (epoch !== resourceEpoch) return
    resourceOptions.value = options
    resourceLoadedKey.value = key
  } catch {
    if (epoch !== resourceEpoch) return
    resourceOptions.value = []
    resourceLoadedKey.value = ''
    resourceError.value = '资源列表加载失败，可直接输入资源 ID'
  } finally {
    if (epoch === resourceEpoch) resourceLoading.value = false
  }
}

watch(resourceContextKey, () => { void loadResources() }, { immediate: true })

const resourceSelectOptions = computed<SelectOption[]>(() => resourceOptions.value.map(option => ({
  label: option.hint ? `${option.label}（${option.hint}）` : option.label,
  value: option.value,
})))
const resourceMissing = computed(() => {
  const source = panel.value?.source
  if (!source?.resourceId || resourceLoadedKey.value !== resourceContextKey.value || resourceLoading.value) return false
  return !resourceOptions.value.some(option => option.value === source.resourceId)
})
const changeResource = (value: string | null) => {
  const option = resourceOptions.value.find(item => item.value === value)
  patchSource({ resourceId: (value || '').trim(), label: option?.label || '' })
}

const channelMissing = computed(() => {
  const id = panel.value?.source.channelId
  return !!id && !props.channelOptions.some(option => option.value === id)
})
const channelSelectOptions = computed<SelectOption[]>(() => [{ label: '当前布局频道', value: '' }, ...props.channelOptions])

// ---------------------------------------------------------------------------
// 网页

const webDraft = ref('')
watch(() => [props.panelId, panel.value?.source.url] as const, () => { webDraft.value = panel.value?.source.url || '' }, { immediate: true })
const webError = computed(() => {
  if (!webDraft.value.trim()) return ''
  return resolveWorkbenchWebUrl(webDraft.value, window.location.origin).reason
})
const commitWebUrl = () => {
  const value = webDraft.value.trim()
  if (value && webError.value) return
  patchSource({ url: value })
}

// ---------------------------------------------------------------------------
// 位置尺寸

const changeRect = (field: keyof WorkbenchRect, value: number | null) => {
  if (!panel.value || value === null) return
  const rect = { ...panel.value.rect, [field]: field === 'x' || field === 'y' ? value - 1 : value }
  const next = setWorkbenchPanelRect(props.layout, panel.value.id, rect)
  if (next) emit('change', next)
  else emit('rejected', '超出网格或与其他面板重叠')
}

// ---------------------------------------------------------------------------
// 网格与主题

const changeColumns = (value: 12 | 24) => emit('change', changeWorkbenchColumns(props.layout, value))
const changeRows = (value: number | null) => {
  if (value === null) return
  const next = changeWorkbenchRows(props.layout, value)
  if (next) emit('change', next)
  else emit('rejected', `行数不能小于面板占用的 ${minimumWorkbenchRows(props.layout)} 行`)
}
const changeGap = (value: number | null) => {
  if (value === null) return
  const next = cloneWorkbenchLayout(props.layout)
  next.grid.gap = Math.min(WORKBENCH_LIMITS.maxGap, Math.max(0, Math.round(value)))
  emit('change', next)
}
const changeSetting = (key: keyof WorkbenchLayout['settings'], value: string) => {
  const next = cloneWorkbenchLayout(props.layout)
  if (value) (next.settings as Record<string, string>)[key] = value
  else delete next.settings[key]
  emit('change', next)
}
const accentValue = computed(() => props.layout.settings.accent || null)
const changeAccent = (value: string | null) => {
  const normalized = (value || '').slice(0, 7)
  changeSetting('accent', /^#[0-9a-fA-F]{6}$/.test(normalized) ? normalized.toLowerCase() : '')
}
</script>

<template>
  <aside class="wb-inspector" data-sc-ui="workbench-inspector" aria-label="布局属性">
    <template v-if="panel">
      <header class="wb-inspector__header">
        <strong>面板属性</strong>
        <small>{{ panel.id }}</small>
      </header>
      <NForm label-placement="top" size="small" :show-feedback="false" class="wb-inspector__form">
        <NFormItem label="标题">
          <NInput
            :value="panel.title || ''"
            :maxlength="WORKBENCH_LIMITS.maxTitle"
            placeholder="留空时使用资源名称"
            clearable
            @update:value="value => patchPanel({ title: value.trim() })"
          />
        </NFormItem>
        <NFormItem label="内容类型">
          <NSelect :value="sourceKind" :options="kindOptions" @update:value="changeKind" />
        </NFormItem>

        <NFormItem v-if="panel.source.type !== 'web'" label="频道">
          <NSelect
            :value="panel.source.channelId || ''"
            :options="channelSelectOptions"
            filterable
            @update:value="(value: string) => patchSource({ channelId: value, ...(panel?.source.type === 'surface' && panel.source.surface !== 'clue-board' ? { resourceId: '', label: '' } : {}) })"
          />
        </NFormItem>
        <NAlert v-if="channelMissing" type="warning" :bordered="false" class="wb-inspector__alert">
          引用的频道不在当前世界或你无权访问，请重新选择。
        </NAlert>

        <template v-if="panel.source.type === 'surface' && panel.source.surface !== 'clue-board'">
          <NFormItem label="资源">
            <div class="wb-inspector__row">
              <NSelect
                :value="panel.source.resourceId || null"
                :options="resourceSelectOptions"
                :loading="resourceLoading"
                filterable
                tag
                clearable
                placeholder="选择或输入资源 ID"
                @update:value="changeResource"
              />
              <NButton quaternary circle size="small" aria-label="刷新资源列表" :loading="resourceLoading" @click="loadResources">
                <template #icon><NIcon><Refresh /></NIcon></template>
              </NButton>
            </div>
          </NFormItem>
          <NAlert v-if="resourceError" type="info" :bordered="false" class="wb-inspector__alert">{{ resourceError }}</NAlert>
          <NAlert v-else-if="!panel.source.resourceId" type="warning" :bordered="false" class="wb-inspector__alert">尚未绑定资源，玩家将看到“未绑定”提示。</NAlert>
          <NAlert v-else-if="resourceMissing" type="warning" :bordered="false" class="wb-inspector__alert">
            当前绑定“{{ panel.source.label || panel.source.resourceId }}”不在你可访问的资源列表中，可能已删除或来自其他世界，请重新绑定。
          </NAlert>
          <p v-if="panel.source.surface === 'character'" class="wb-inspector__hint">展示给全体成员时请选择“频道在线快照”；个人人物卡只有拥有者可见。</p>
        </template>

        <template v-if="panel.source.type === 'web'">
          <NFormItem label="网页地址">
            <NInput
              v-model:value="webDraft"
              placeholder="https://example.com"
              :status="webError ? 'error' : undefined"
              @blur="commitWebUrl"
              @keydown.enter.prevent="commitWebUrl"
            />
          </NFormItem>
          <p v-if="webError" class="wb-inspector__error" role="alert">{{ webError }}</p>
          <NFormItem label="展示方式">
            <NRadioGroup :value="panel.source.display || 'embed'" @update:value="(value: string) => patchSource({ display: value === 'link' ? 'link' : '' })">
              <NRadioButton value="embed">嵌入</NRadioButton>
              <NRadioButton value="link">链接卡片</NRadioButton>
            </NRadioGroup>
          </NFormItem>
          <p class="wb-inspector__hint">外部网页在沙箱中运行，不获得 SealChat 登录态或接口权限；拒绝嵌入的网站请使用链接卡片。</p>
        </template>

        <NFormItem label="外观">
          <NRadioGroup :value="panel.chrome || ''" @update:value="(value: string) => patchPanel({ chrome: value === 'minimal' ? 'minimal' : '' })">
            <NRadioButton value="">标准</NRadioButton>
            <NRadioButton value="minimal">极简</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem label="默认对玩家隐藏">
          <NSwitch :value="panel.hiddenByDefault === true" @update:value="(value: boolean) => patchPanel({ hiddenByDefault: value })" />
        </NFormItem>
        <NFormItem label="位置与尺寸（列 / 行 / 宽 / 高）">
          <div class="wb-inspector__rect">
            <NInputNumber :value="panel.rect.x + 1" :min="1" :max="layout.grid.columns" size="small" aria-label="起始列" @update:value="value => changeRect('x', value)" />
            <NInputNumber :value="panel.rect.y + 1" :min="1" :max="layout.grid.rows" size="small" aria-label="起始行" @update:value="value => changeRect('y', value)" />
            <NInputNumber :value="panel.rect.w" :min="1" :max="layout.grid.columns" size="small" aria-label="宽度" @update:value="value => changeRect('w', value)" />
            <NInputNumber :value="panel.rect.h" :min="1" :max="layout.grid.rows" size="small" aria-label="高度" @update:value="value => changeRect('h', value)" />
          </div>
        </NFormItem>
      </NForm>
      <NSpace class="wb-inspector__actions" size="small">
        <NButton size="small" :disabled="panelIndex <= 0" @click="emit('change', reorderWorkbenchPanel(layout, panel.id, -1))">
          <template #icon><NIcon><ArrowUp /></NIcon></template>
          前移
        </NButton>
        <NButton size="small" :disabled="panelIndex >= layout.panels.length - 1" @click="emit('change', reorderWorkbenchPanel(layout, panel.id, 1))">
          <template #icon><NIcon><ArrowDown /></NIcon></template>
          后移
        </NButton>
        <NButton size="small" @click="emit('duplicate', panel.id)">
          <template #icon><NIcon><Copy /></NIcon></template>
          复制
        </NButton>
        <NButton size="small" type="error" secondary @click="emit('remove', panel.id)">
          <template #icon><NIcon><Trash /></NIcon></template>
          删除
        </NButton>
      </NSpace>
      <p class="wb-inspector__hint">顺序决定移动端标签与堆叠顺序。画布中可用方向键移动、Shift+方向键调整尺寸。</p>
    </template>

    <template v-else>
      <header class="wb-inspector__header"><strong>布局设置</strong></header>
      <NForm label-placement="top" size="small" :show-feedback="false" class="wb-inspector__form">
        <NFormItem label="网格列数">
          <NRadioGroup :value="layout.grid.columns" @update:value="changeColumns">
            <NRadioButton :value="12">12 列</NRadioButton>
            <NRadioButton :value="24">24 列</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem label="网格行数">
          <NInputNumber :value="layout.grid.rows" :min="minimumWorkbenchRows(layout)" :max="WORKBENCH_LIMITS.maxRows" @update:value="changeRows" />
        </NFormItem>
        <NFormItem label="面板间距（px）">
          <NInputNumber :value="layout.grid.gap" :min="0" :max="WORKBENCH_LIMITS.maxGap" @update:value="changeGap" />
        </NFormItem>
        <NFormItem label="移动端展示">
          <NRadioGroup :value="layout.settings.mobileMode || ''" @update:value="(value: string) => changeSetting('mobileMode', value)">
            <NRadioButton value="">标签页</NRadioButton>
            <NRadioButton value="stack">纵向堆叠</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem label="面板风格">
          <NRadioGroup :value="layout.settings.panelStyle || ''" @update:value="(value: string) => changeSetting('panelStyle', value)">
            <NRadioButton value="">默认</NRadioButton>
            <NRadioButton value="flat">扁平</NRadioButton>
            <NRadioButton value="outlined">描边</NRadioButton>
            <NRadioButton value="glass">玻璃</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem label="极简操作区">
          <NRadioGroup :value="layout.settings.minimalActionsPlacement === 'outside' ? 'outside' : ''" @update:value="(value: string) => changeSetting('minimalActionsPlacement', value)">
            <NRadioButton value="">内部覆盖</NRadioButton>
            <NRadioButton value="outside">右上外展</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem label="强调色">
          <div class="wb-inspector__row">
            <NColorPicker :value="accentValue" :modes="['hex']" :show-alpha="false" @update:value="changeAccent" />
            <NButton size="small" quaternary :disabled="!layout.settings.accent" @click="changeSetting('accent', '')">跟随主题</NButton>
          </div>
        </NFormItem>
      </NForm>
      <p class="wb-inspector__hint">风格与强调色基于 --sc-* 主题变量，自动适配日间 / 夜间；更深度的样式可通过界面自定义包的 data-sc-ui="workbench*" hook 覆盖。</p>
    </template>
  </aside>
</template>

<style scoped>
.wb-inspector { display: flex; flex-direction: column; gap: 10px; min-height: 0; padding: 12px; overflow-y: auto; border-left: 1px solid var(--sc-border-mute, rgba(127, 127, 127, .2)); color: var(--sc-text-primary, inherit); background: var(--sc-bg-surface, #fff); }
.wb-inspector__header { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.wb-inspector__header small { color: var(--sc-text-secondary, #888); font-family: monospace; }
.wb-inspector__form { display: flex; flex-direction: column; gap: 10px; }
.wb-inspector__row { display: flex; width: 100%; align-items: center; gap: 6px; }
.wb-inspector__rect { display: grid; width: 100%; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; }
.wb-inspector__alert { font-size: 12px; }
.wb-inspector__hint { margin: 0; color: var(--sc-text-secondary, #888); font-size: 12px; line-height: 1.5; }
.wb-inspector__error { margin: -4px 0 0; color: var(--sc-color-error, #d03050); font-size: 12px; }
.wb-inspector__actions { flex-wrap: wrap; }
</style>
