<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NCard, NInput, NModal, NRadioButton, NRadioGroup, NSpace, NTabPane, NTabs, useMessage } from 'naive-ui'
import { WorkbenchLayoutError } from './layout'
import {
  buildWorkbenchExport,
  parseWorkbenchImport,
  WORKBENCH_PRESETS,
  workbenchExportFilename,
  type WorkbenchExportType,
  type WorkbenchImportResult,
} from './transfer'
import type { WorkbenchLayout } from './types'

// 预设、导入、导出共用同一份 sealchat.workbench 文档格式。应用结果只进入设计草稿，
// 由管理者确认保存；缺失的资源在设计模式中逐个重新绑定，导入本身不会失败。

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  layout: WorkbenchLayout
  worldId: string
  channelId: string
  channelName: string
  canImport: boolean
}>()

const emit = defineEmits<{ apply: [result: WorkbenchImportResult] }>()

const message = useMessage()
const tab = ref<'presets' | 'import' | 'export'>('presets')
const exportType = ref<WorkbenchExportType>('template')
const importText = ref('')
const importResult = ref<WorkbenchImportResult | null>(null)
const importError = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

watch(show, (visible) => {
  if (!visible) return
  tab.value = props.canImport ? 'presets' : 'export'
  importText.value = ''
  importResult.value = null
  importError.value = ''
})

const exportDocument = computed(() => buildWorkbenchExport(props.layout, exportType.value, {
  name: props.channelName,
  worldId: props.worldId,
  channelId: props.channelId,
  channelName: props.channelName,
}))
const exportJson = computed(() => JSON.stringify(exportDocument.value, null, 2))

const download = () => {
  const blob = new Blob([exportJson.value], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = workbenchExportFilename(props.channelName, exportType.value)
  anchor.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 0)
}

const copy = async () => {
  try {
    await navigator.clipboard.writeText(exportJson.value)
    message.success('已复制到剪贴板')
  } catch {
    message.error('复制失败，请使用下载')
  }
}

const parse = (text: string) => {
  importError.value = ''
  importResult.value = null
  if (!text.trim()) return
  try {
    importResult.value = parseWorkbenchImport(text, props.worldId)
  } catch (error) {
    importError.value = error instanceof WorkbenchLayoutError ? error.message : '无法解析导入文件'
  }
}

const MAX_IMPORT_BYTES = 512 * 1024
const handleFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (file.size > MAX_IMPORT_BYTES) {
    importError.value = '文件过大（上限 512KB）'
    return
  }
  importText.value = await file.text()
  parse(importText.value)
}

const bindingCount = (result: WorkbenchImportResult) => result.layout.panels.filter(panel => (
  (panel.source.type === 'surface' && panel.source.surface !== 'clue-board' && !panel.source.resourceId)
  || (panel.source.type === 'web' && !panel.source.url)
)).length

const applyResult = (result: WorkbenchImportResult) => {
  emit('apply', result)
  show.value = false
}

const applyPreset = (index: number) => {
  applyResult(parseWorkbenchImport(WORKBENCH_PRESETS[index].document, props.worldId))
}
</script>

<template>
  <NModal v-model:show="show" preset="card" title="布局模板与导入导出" class="wb-transfer" style="width: min(720px, calc(100vw - 32px));" :bordered="false">
    <NTabs v-model:value="tab" type="line" animated>
      <NTabPane v-if="canImport" name="presets" tab="预设">
        <div class="wb-transfer__presets">
          <NCard v-for="(preset, index) in WORKBENCH_PRESETS" :key="preset.id" size="small" :title="preset.name" embedded>
            <p>{{ preset.description }}</p>
            <template #action>
              <NButton size="small" type="primary" secondary @click="applyPreset(index)">应用到草稿</NButton>
            </template>
          </NCard>
        </div>
      </NTabPane>
      <NTabPane v-if="canImport" name="import" tab="导入">
        <NSpace vertical>
          <NSpace>
            <NButton size="small" @click="fileInput?.click()">选择文件</NButton>
            <input ref="fileInput" type="file" accept=".json,application/json" hidden @change="handleFile">
          </NSpace>
          <NInput
            v-model:value="importText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 10 }"
            placeholder="或粘贴 sealchat.workbench JSON"
            @update:value="parse"
          />
          <NAlert v-if="importError" type="error" :bordered="false">{{ importError }}</NAlert>
          <template v-if="importResult">
            <NAlert type="info" :bordered="false" :title="importResult.exportType === 'template' ? '布局模板' : '完整设计'">
              {{ importResult.name ? `「${importResult.name}」 · ` : '' }}{{ importResult.layout.panels.length }} 个面板，{{ importResult.layout.grid.columns }} 列 × {{ importResult.layout.grid.rows }} 行。
              <template v-if="importResult.exportType === 'template'">模板不含资源绑定，应用后在设计模式中为面板选择资源。</template>
              <template v-else-if="importResult.foreignWorld">该设计来自其他世界，频道与资源绑定可能需要重新选择。</template>
              <template v-if="bindingCount(importResult)">共有 {{ bindingCount(importResult) }} 个面板需要绑定。</template>
            </NAlert>
            <NAlert v-if="importResult.warnings.length" type="warning" :bordered="false" title="导入提示">
              <ul class="wb-transfer__warnings">
                <li v-for="(warning, index) in importResult.warnings" :key="index">{{ warning }}</li>
              </ul>
            </NAlert>
            <NButton type="primary" @click="applyResult(importResult)">应用到草稿</NButton>
          </template>
        </NSpace>
      </NTabPane>
      <NTabPane name="export" tab="导出">
        <NSpace vertical>
          <NRadioGroup v-model:value="exportType">
            <NRadioButton value="template">布局模板（不含资源绑定）</NRadioButton>
            <NRadioButton value="design">完整设计（含资源描述）</NRadioButton>
          </NRadioGroup>
          <p class="wb-transfer__hint">导出只包含资源描述（类型、资源 ID、频道 ID、网页地址），不包含生成的内部链接或任何登录凭据。</p>
          <NSpace>
            <NButton type="primary" @click="download">下载 JSON</NButton>
            <NButton @click="copy">复制</NButton>
          </NSpace>
        </NSpace>
      </NTabPane>
    </NTabs>
  </NModal>
</template>

<style scoped>
.wb-transfer__presets { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 10px; }
.wb-transfer__presets p { margin: 0; color: var(--sc-text-secondary, #888); font-size: 13px; }
.wb-transfer__warnings { margin: 0; padding-left: 18px; }
.wb-transfer__hint { margin: 0; color: var(--sc-text-secondary, #888); font-size: 12px; }
</style>
