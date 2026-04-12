<script setup lang="ts">
import { cloneDeep } from 'lodash-es'
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import type { ServerConfig, ThemeManagementConfig } from '@/types'
import type { CustomThemeColors, PlatformTheme } from '@/services/theme/themeTypes'
import { useDisplayStore } from '@/stores/display'
import { useUtilsStore } from '@/stores/utils'

type AdminThemeStyleExpose = {
  save: () => Promise<void>
  isModified: () => boolean
}

type ThemeImportPayload = {
  name: string
  colors: CustomThemeColors
}

const message = useMessage()
const display = useDisplayStore()
const utils = useUtilsStore()

const model = ref<ThemeManagementConfig>({
  platformThemes: [],
  defaultPlatformThemeId: '',
})
const originalSnapshot = ref('')
const saving = ref(false)
const importFileInputRef = ref<HTMLInputElement | null>(null)
const selectedPersonalThemeId = ref<string | null>(null)
const expandedNames = ref<string[]>(['platform-theme-list'])

const platformThemes = computed(() => model.value.platformThemes || [])
const personalThemeOptions = computed(() =>
  display.settings.customThemes.map((theme) => ({
    label: theme.name,
    value: theme.id,
  })),
)
const defaultPlatformThemeName = computed(() => {
  if (!model.value.defaultPlatformThemeId) return ''
  return platformThemes.value.find((item) => item.id === model.value.defaultPlatformThemeId)?.name || ''
})
const isModified = computed(() => JSON.stringify(model.value) !== originalSnapshot.value)

const normalizeThemeManagement = (value?: ThemeManagementConfig | null): ThemeManagementConfig => ({
  platformThemes: Array.isArray(value?.platformThemes) ? cloneDeep(value?.platformThemes || []) : [],
  defaultPlatformThemeId: value?.defaultPlatformThemeId || '',
})

const buildUniqueThemeName = (rawName: string) => {
  const baseName = rawName.trim() || `平台主题 ${platformThemes.value.length + 1}`
  const existingNames = new Set(platformThemes.value.map((item) => item.name.trim()))
  if (!existingNames.has(baseName)) return baseName
  let index = 2
  let nextName = `${baseName} ${index}`
  while (existingNames.has(nextName)) {
    index += 1
    nextName = `${baseName} ${index}`
  }
  return nextName
}

const appendPlatformTheme = (payload: ThemeImportPayload) => {
  if (platformThemes.value.length >= 50) {
    message.warning('平台主题数量不能超过 50 个')
    return false
  }
  const now = Date.now()
  const nextTheme: PlatformTheme = {
    id: `platform-theme-${now}`,
    name: buildUniqueThemeName(payload.name),
    colors: { ...payload.colors },
    createdAt: now,
    updatedAt: now,
  }
  model.value = {
    platformThemes: [...platformThemes.value, nextTheme],
    defaultPlatformThemeId: model.value.defaultPlatformThemeId || '',
  }
  return true
}

const exportPlatformTheme = (theme: PlatformTheme) => {
  const exportData = {
    name: theme.name,
    colors: theme.colors,
    exportedAt: new Date().toISOString(),
    version: '1.0',
  }
  const blob = new Blob([JSON.stringify(exportData, null, 2)], {
    type: 'application/json;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `sealchat-theme-${theme.name.replace(/[^a-zA-Z0-9\u4e00-\u9fa5]/g, '_')}.json`
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

const formatTimestamp = (value?: number) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

const readImportedTheme = async (file: File): Promise<ThemeImportPayload> => {
  const rawText = await file.text()
  let parsed: any
  try {
    parsed = JSON.parse(rawText)
  } catch {
    throw new Error('主题文件不是有效的 JSON')
  }

  const name = typeof parsed?.name === 'string' ? parsed.name.trim() : ''
  if (!name) {
    throw new Error('主题文件缺少 name 字段')
  }
  if (!parsed?.colors || typeof parsed.colors !== 'object' || Array.isArray(parsed.colors)) {
    throw new Error('主题文件缺少有效的 colors 配置')
  }

  return {
    name,
    colors: cloneDeep(parsed.colors) as CustomThemeColors,
  }
}

const handleTriggerImport = () => {
  importFileInputRef.value?.click()
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const payload = await readImportedTheme(file)
    if (!appendPlatformTheme(payload)) return
    message.success(`已导入平台主题：${payload.name}`)
  } catch (error: any) {
    message.error(error?.message || '导入主题失败')
  } finally {
    input.value = ''
  }
}

const handleImportPersonalTheme = () => {
  const selected = display.settings.customThemes.find((item) => item.id === selectedPersonalThemeId.value) || null
  if (!selected) {
    message.warning('先选择一个个人主题')
    return
  }
  if (!appendPlatformTheme({
    name: selected.name,
    colors: selected.colors,
  })) return
  message.success(`已导入个人主题：${selected.name}`)
}

const handleDeleteTheme = (themeId: string) => {
  const nextThemes = platformThemes.value.filter((item) => item.id !== themeId)
  model.value = {
    platformThemes: nextThemes,
    defaultPlatformThemeId: model.value.defaultPlatformThemeId === themeId ? '' : model.value.defaultPlatformThemeId || '',
  }
}

const handleSetDefault = (themeId: string) => {
  model.value = {
    ...model.value,
    defaultPlatformThemeId: themeId,
  }
}

const resetFromConfig = async () => {
  if (!utils.config) {
    await utils.configGet()
  }
  model.value = normalizeThemeManagement(utils.config?.themeManagement)
  originalSnapshot.value = JSON.stringify(model.value)
}

const save = async () => {
  saving.value = true
  try {
    if (!utils.config) {
      await utils.configGet()
    }
    const payload: ServerConfig = cloneDeep((utils.config || {}) as ServerConfig)
    payload.themeManagement = cloneDeep(model.value)
    await utils.configSet(payload)
    model.value = normalizeThemeManagement(payload.themeManagement)
    originalSnapshot.value = JSON.stringify(model.value)
    message.success('主题与样式管理已保存')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await resetFromConfig()
})

defineExpose<AdminThemeStyleExpose>({
  save,
  isModified: () => isModified.value,
})
</script>

<template>
  <div class="admin-theme-style">
    <n-collapse v-model:expanded-names="expandedNames" class="settings-fold">
      <n-collapse-item name="platform-theme-list" class="settings-fold__item">
        <template #header>
          <div class="settings-fold__header">
            <div class="settings-fold__header-main">
              <div class="settings-fold__title">平台主题列表</div>
              <div class="settings-fold__desc">管理成员可选主题的导入/导出列表。</div>
            </div>
          </div>
        </template>
        <template #header-extra>
          <div class="settings-fold__header-extra">
            <span class="settings-fold__metric">
              <label>默认主题</label>
              <strong>{{ defaultPlatformThemeName || '未设置' }}</strong>
            </span>
            <span class="settings-fold__metric">
              <label>平台主题数</label>
              <strong>{{ platformThemes.length }}/50</strong>
            </span>
          </div>
        </template>

        <div class="settings-fold__body">
          <div class="admin-theme-style__toolbar">
            <n-button secondary :loading="saving" @click="handleTriggerImport">导入 JSON</n-button>
            <n-select
              v-model:value="selectedPersonalThemeId"
              :options="personalThemeOptions"
              clearable
              placeholder="从个人主题导入"
              class="admin-theme-style__toolbar-select"
            />
            <n-button secondary :disabled="!selectedPersonalThemeId" @click="handleImportPersonalTheme">导入个人主题</n-button>
          </div>
          <input
            ref="importFileInputRef"
            type="file"
            accept=".json,application/json"
            class="admin-theme-style__hidden-input"
            @change="handleImportFile"
          />

          <div class="admin-theme-style__list-section">
            <n-empty v-if="platformThemes.length === 0" description="暂无平台主题，可通过 JSON 或个人主题导入" />

            <div v-else class="admin-theme-style__list">
              <div
                v-for="theme in platformThemes"
                :key="theme.id"
                class="admin-theme-style__item"
              >
                <div class="admin-theme-style__item-main">
                  <div class="admin-theme-style__item-title">
                    <span>{{ theme.name }}</span>
                    <n-tag v-if="model.defaultPlatformThemeId === theme.id" size="small" type="success" round>默认</n-tag>
                  </div>
                  <div class="admin-theme-style__item-meta">
                    <span>更新时间：{{ formatTimestamp(theme.updatedAt) }}</span>
                    <span>颜色项：{{ Object.keys(theme.colors || {}).length }}</span>
                  </div>
                </div>
                <div class="admin-theme-style__item-actions">
                  <n-button
                    size="small"
                    secondary
                    :disabled="model.defaultPlatformThemeId === theme.id"
                    @click="handleSetDefault(theme.id)"
                  >
                    设为默认
                  </n-button>
                  <n-button size="small" secondary @click="exportPlatformTheme(theme)">导出</n-button>
                  <n-popconfirm @positive-click="handleDeleteTheme(theme.id)">
                    <template #trigger>
                      <n-button size="small" secondary type="error">删除</n-button>
                    </template>
                    删除后若当前主题为默认主题，将自动清空默认主题。
                  </n-popconfirm>
                </div>
              </div>
            </div>
          </div>
        </div>
      </n-collapse-item>
    </n-collapse>
  </div>
</template>

<style scoped>
.admin-theme-style {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.settings-fold {
  flex: 1;
  min-height: 0;
  width: 100%;
  overflow: hidden;
}

.settings-fold :deep(.n-collapse-item) {
  margin: 0;
  border: 1px solid var(--sc-border-mute);
  border-radius: 14px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.02), rgba(255, 255, 255, 0.01));
  overflow: hidden;
}

.settings-fold :deep(.n-collapse-item__header) {
  padding: 18px 20px;
  align-items: center;
  gap: 12px;
  min-height: 88px;
}

.settings-fold :deep(.n-collapse-item__header-main) {
  display: flex;
  align-items: center;
  min-height: 52px;
}

.settings-fold :deep(.n-collapse-item__header-extra) {
  display: flex;
  align-items: center;
  min-height: 52px;
}

.settings-fold :deep(.n-collapse-item-arrow) {
  margin-top: 0;
}

.settings-fold :deep(.n-collapse-item__content-wrapper) {
  overflow: hidden;
}

.settings-fold :deep(.n-collapse-item__content-inner) {
  padding: 0;
  min-height: 0;
}

.settings-fold__header {
  min-width: 0;
}

.settings-fold__header-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  justify-content: center;
}

.settings-fold__title {
  font-size: 16px;
  font-weight: 700;
  line-height: 1.3;
}

.settings-fold__desc,
.admin-theme-style__item-meta {
  color: var(--sc-text-secondary);
  font-size: 13px;
  line-height: 1.45;
}

.settings-fold__header-extra {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.settings-fold__metric {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.settings-fold__metric label {
  opacity: 0.72;
}

.settings-fold__metric strong {
  color: var(--sc-text-primary);
  font-size: 13px;
  font-weight: 600;
}

.settings-fold__body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 0 20px 20px;
  min-height: 0;
  max-height: calc(78vh - 140px);
}

.admin-theme-style__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.admin-theme-style__toolbar-select {
  min-width: 240px;
  flex: 1 1 260px;
}

.admin-theme-style__hidden-input {
  display: none;
}

.admin-theme-style__list-section {
  flex: 1;
  min-height: 0;
  border-top: 1px solid var(--sc-border-mute);
  padding-top: 16px;
  overflow-y: auto;
  padding-right: 4px;
  scrollbar-gutter: stable;
}

.admin-theme-style__list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.admin-theme-style__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px;
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  background: var(--sc-bg-surface);
}

.admin-theme-style__item-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.admin-theme-style__item-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.admin-theme-style__item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.admin-theme-style__item-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

@media (max-width: 720px) {
  .settings-fold :deep(.n-collapse-item__header),
  .admin-theme-style__item {
    flex-direction: column;
    align-items: stretch;
  }

  .settings-fold__body {
    max-height: calc(85vh - 170px);
  }

  .settings-fold__header-extra {
    justify-content: flex-start;
  }

  .admin-theme-style__list-section {
    padding-right: 0;
  }

  .admin-theme-style__item-actions {
    justify-content: flex-start;
  }
}
</style>
