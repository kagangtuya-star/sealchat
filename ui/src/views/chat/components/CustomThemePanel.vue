<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useWindowSize } from '@vueuse/core'
import { useMessage } from 'naive-ui'
import { useDisplayStore, type CustomTheme, type CustomThemeColors } from '@/stores/display'
import { presetThemes, dayBaseTheme, nightBaseTheme } from '@/config/presetThemes'
import ThemeLivePreviewFloating from './ThemeLivePreviewFloating.vue'

interface Props {
  show: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const message = useMessage()
const display = useDisplayStore()
const livePreviewFloatingVisible = ref(false)
const livePreviewBaseColors = ref<CustomThemeColors | null>(null)
const selectedImportSource = ref<string | null>(null)
const selectedThemeId = ref<string | null>(null)

// 响应式 drawer 宽度
const { width: windowWidth } = useWindowSize()
const MOBILE_BREAKPOINT = 600
const DRAWER_WIDTH_DESKTOP = 480
const drawerWidth = computed(() => {
  // 移动端使用窗口宽度，但不超过桌面宽度
  if (windowWidth.value <= MOBILE_BREAKPOINT) {
    return Math.min(windowWidth.value, DRAWER_WIDTH_DESKTOP)
  }
  return DRAWER_WIDTH_DESKTOP
})


// 编辑模式：新建 or 编辑现有
const editMode = ref<'create' | 'edit'>('create')
const editingThemeId = ref<string | null>(null)

// 表单数据
const themeName = ref('')
const themeColors = ref<CustomThemeColors>({})

// 颜色配置项定义
const colorFields: { key: keyof CustomThemeColors; label: string; group: string }[] = [
  // 背景
  { key: 'bgSurface', label: '主背景', group: '背景' },
  { key: 'bgElevated', label: '卡片/弹窗', group: '背景' },
  { key: 'bgInput', label: '输入框', group: '背景' },
  { key: 'bgHeader', label: '顶栏', group: '背景' },
  // 文字
  { key: 'textPrimary', label: '主文字', group: '文字' },
  { key: 'textSecondary', label: '次要文字', group: '文字' },
  // 聊天
  { key: 'chatIcBg', label: '气泡（场内）', group: '气泡颜色' },
  { key: 'chatOocBg', label: '气泡（场外）', group: '气泡颜色' },
  { key: 'chatStageBg', label: '聊天舞台', group: '聊天区域' },
  { key: 'chatPreviewBg', label: '预览背景', group: '聊天区域' },
  { key: 'chatPreviewDot', label: '预览圆点', group: '聊天区域' },
  // 边框
  { key: 'borderMute', label: '淡边框', group: '边框' },
  { key: 'borderStrong', label: '强边框', group: '边框' },
  // 强调色
  { key: 'primaryColor', label: '主题色', group: '强调色' },
  { key: 'primaryColorHover', label: '悬停色', group: '强调色' },
  // 术语高亮
  { key: 'keywordBg', label: '高亮背景', group: '术语高亮' },
  { key: 'keywordBorder', label: '下划线色', group: '术语高亮' },
]

const colorGroups = computed(() => {
  const groups: Record<string, typeof colorFields> = {}
  colorFields.forEach(f => {
    if (!groups[f.group]) groups[f.group] = []
    groups[f.group].push(f)
  })
  return groups
})

// 主题列表
const themes = computed(() => display.settings.customThemes)
const activeThemeId = computed(() => display.settings.activeCustomThemeId)
const selectedSavedTheme = computed(() => {
  if (!selectedThemeId.value) return null
  return themes.value.find(t => t.id === selectedThemeId.value) || null
})
const savedThemeOptions = computed(() =>
  themes.value.map((theme) => ({
    label: activeThemeId.value === theme.id ? `${theme.name}（当前）` : theme.name,
    value: theme.id,
  })),
)

// 初始化表单
const resetForm = () => {
  themeName.value = ''
  themeColors.value = {}
  editMode.value = 'create'
  editingThemeId.value = null
}

const startCreate = () => {
  resetForm()
  themeName.value = `自定义主题 ${themes.value.length + 1}`
}

const startEdit = (theme: CustomTheme) => {
  editMode.value = 'edit'
  editingThemeId.value = theme.id
  themeName.value = theme.name
  themeColors.value = { ...theme.colors }
}

const importThemeOptions = computed(() => {
  const presetChildren = presetThemes.map(preset => ({
    label: preset.name,
    value: `preset:${preset.id}`,
  }))
  const savedChildren = themes.value.map(theme => ({
    label: activeThemeId.value === theme.id ? `${theme.name}（当前）` : theme.name,
    value: `saved:${theme.id}`,
  }))

  const options: Array<{ label: string; type: 'group'; key: string; children: Array<{ label: string; value: string }> }> = []
  if (presetChildren.length > 0) {
    options.push({
      label: '预设主题',
      type: 'group',
      key: 'preset-group',
      children: presetChildren,
    })
  }
  if (savedChildren.length > 0) {
    options.push({
      label: '已保存主题',
      type: 'group',
      key: 'saved-group',
      children: savedChildren,
    })
  }
  return options
})

const buildUniqueThemeName = (baseName: string) => {
  const normalized = baseName.trim()
  const fallback = `自定义主题 ${themes.value.length + 1}`
  const sourceName = normalized || fallback
  const hasSameName = themes.value.some(theme => theme.name === sourceName)
  if (!hasSameName) return sourceName
  return `${sourceName} ${Date.now().toString(36).slice(-4)}`
}

const fillEditorByTemplate = (sourceName: string, sourceColors: CustomThemeColors) => {
  editMode.value = 'create'
  editingThemeId.value = null
  themeName.value = buildUniqueThemeName(sourceName)
  themeColors.value = { ...sourceColors }
  selectedImportSource.value = null
}

const importThemeFromSource = (sourceValue: string | null) => {
  if (typeof sourceValue !== 'string' || !sourceValue.trim()) {
    selectedImportSource.value = null
    return
  }

  const separatorIndex = sourceValue.indexOf(':')
  if (separatorIndex <= 0 || separatorIndex >= sourceValue.length - 1) {
    selectedImportSource.value = null
    return
  }

  const sourceType = sourceValue.slice(0, separatorIndex)
  const sourceId = sourceValue.slice(separatorIndex + 1)

  if (sourceType === 'preset') {
    const preset = presetThemes.find(p => p.id === sourceId)
    if (!preset) {
      selectedImportSource.value = null
      return
    }
    fillEditorByTemplate(preset.name, preset.colors)
    return
  }

  if (sourceType === 'saved') {
    const savedTheme = themes.value.find(t => t.id === sourceId)
    if (!savedTheme) {
      selectedImportSource.value = null
      return
    }
    fillEditorByTemplate(savedTheme.name, savedTheme.colors)
    return
  }

  selectedImportSource.value = null
}

const handleThemeSelect = (themeId: string | null) => {
  selectedThemeId.value = themeId
  if (!themeId) return
  handleActivate(themeId)
}

const handleEditSelectedTheme = () => {
  if (!selectedSavedTheme.value) return
  startEdit(selectedSavedTheme.value)
}

const handleExportSelectedTheme = () => {
  if (!selectedSavedTheme.value) return
  exportTheme(selectedSavedTheme.value)
}

const handleDeleteSelectedTheme = () => {
  if (!selectedSavedTheme.value) return
  handleDelete(selectedSavedTheme.value.id)
}

const saveCurrentTheme = (options?: { keepEditing?: boolean }) => {
  const keepEditing = options?.keepEditing === true
  let name = themeName.value.trim()
  if (!name) {
    if (!keepEditing) return
    const fallbackName = `自定义主题 ${themes.value.length + 1}`
    name = fallbackName
    themeName.value = fallbackName
  }

  const now = Date.now()
  const id = editingThemeId.value || `theme_${now}`
  const existingTheme = themes.value.find(t => t.id === id)

  const theme: CustomTheme = {
    id,
    name,
    colors: { ...themeColors.value },
    createdAt: existingTheme?.createdAt ?? now,
    updatedAt: now,
  }

  if (!display.settings.customThemeEnabled) {
    display.setCustomThemeEnabled(true)
  }

  display.saveCustomTheme(theme)
  display.activateCustomTheme(id)
  message.success(existingTheme ? `主题已更新：${name}` : `主题已保存：${name}`)

  if (keepEditing) {
    editMode.value = 'edit'
    editingThemeId.value = id
    return
  }

  resetForm()
}

const handleSave = () => {
  saveCurrentTheme({ keepEditing: false })
}

const handleSaveFromLivePreviewFloating = () => {
  saveCurrentTheme({ keepEditing: true })
}

const handleDelete = (id: string) => {
  display.deleteCustomTheme(id)
}

const handleActivate = (id: string) => {
  display.activateCustomTheme(id)
}

const handleClose = () => {
  emit('update:show', false)
}

const stopLivePreviewSafely = () => {
  clearPreviewThemeVars()
  if (typeof display.applyTheme === 'function') {
    display.applyTheme()
  }
}

const previewCssVars = [
  '--sc-bg-surface', '--sc-bg-elevated', '--sc-bg-input', '--sc-bg-header',
  '--sc-text-primary', '--sc-text-secondary',
  '--chat-text-primary', '--chat-text-secondary',
  '--custom-chat-ic-bg', '--custom-chat-ooc-bg', '--custom-chat-stage-bg', '--custom-chat-preview-bg', '--custom-chat-preview-dot',
  '--sc-border-mute', '--sc-border-strong',
  '--primary-color', '--primary-color-hover',
  '--custom-keyword-bg', '--custom-keyword-border',
]

const clearPreviewThemeVars = () => {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  previewCssVars.forEach((name) => {
    root.style.removeProperty(name)
  })
  delete root.dataset.customTheme
}

const applyPreviewColorsToRoot = (colors: CustomThemeColors) => {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const setVar = (name: string, value?: string) => {
    if (!value) return
    root.style.setProperty(name, value)
  }

  clearPreviewThemeVars()

  setVar('--sc-bg-surface', colors.bgSurface)
  setVar('--sc-bg-elevated', colors.bgElevated)
  setVar('--sc-bg-input', colors.bgInput)
  setVar('--sc-bg-header', colors.bgHeader)

  setVar('--sc-text-primary', colors.textPrimary)
  setVar('--chat-text-primary', colors.textPrimary)
  setVar('--sc-text-secondary', colors.textSecondary)
  setVar('--chat-text-secondary', colors.textSecondary)

  setVar('--custom-chat-ic-bg', colors.chatIcBg)
  setVar('--custom-chat-ooc-bg', colors.chatOocBg)
  setVar('--custom-chat-stage-bg', colors.chatStageBg || colors.chatIcBg)
  setVar('--custom-chat-preview-bg', colors.chatPreviewBg)
  setVar('--custom-chat-preview-dot', colors.chatPreviewDot)

  setVar('--sc-border-mute', colors.borderMute)
  setVar('--sc-border-strong', colors.borderStrong)

  setVar('--primary-color', colors.primaryColor)
  setVar('--primary-color-hover', colors.primaryColorHover)

  setVar('--custom-keyword-bg', colors.keywordBg)
  setVar('--custom-keyword-border', colors.keywordBorder)

  root.dataset.customTheme = 'true'
}

const previewColorVarMap: Record<keyof CustomThemeColors, string[]> = {
  bgSurface: ['--sc-bg-surface'],
  bgElevated: ['--sc-bg-elevated'],
  bgInput: ['--sc-bg-input'],
  bgHeader: ['--sc-bg-header'],
  textPrimary: ['--sc-text-primary'],
  textSecondary: ['--sc-text-secondary'],
  chatIcBg: ['--custom-chat-ic-bg', '--chat-ic-bg'],
  chatOocBg: ['--custom-chat-ooc-bg', '--chat-ooc-bg'],
  chatStageBg: ['--custom-chat-stage-bg', '--chat-stage-bg'],
  chatPreviewBg: ['--custom-chat-preview-bg', '--chat-preview-bg'],
  chatPreviewDot: ['--custom-chat-preview-dot', '--chat-preview-dot'],
  borderMute: ['--sc-border-mute'],
  borderStrong: ['--sc-border-strong'],
  primaryColor: ['--primary-color'],
  primaryColorHover: ['--primary-color-hover'],
  keywordBg: ['--custom-keyword-bg'],
  keywordBorder: ['--custom-keyword-border'],
}

const readCurrentThemeColorsFromCss = (): CustomThemeColors => {
  if (typeof document === 'undefined') return {}
  if (typeof window === 'undefined' || typeof window.getComputedStyle !== 'function') return {}
  const style = window.getComputedStyle(document.documentElement)
  const result: CustomThemeColors = {}

  const colorKeys = Object.keys(previewColorVarMap) as Array<keyof CustomThemeColors>
  colorKeys.forEach((key) => {
    const candidates = previewColorVarMap[key]
    const value = candidates
      .map(v => style.getPropertyValue(v).trim())
      .find(v => !!v)
    if (value) {
      result[key] = value
    }
  })

  return result
}

const getPaletteBaseColors = (): CustomThemeColors => {
  return display.settings.palette === 'night'
    ? { ...nightBaseTheme.colors }
    : { ...dayBaseTheme.colors }
}

const buildLivePreviewColors = (): CustomThemeColors => {
  const base = livePreviewBaseColors.value || getPaletteBaseColors()
  return {
    ...base,
    ...themeColors.value,
  }
}

const handleOpenLivePreview = () => {
  try {
    const currentCssColors = readCurrentThemeColorsFromCss()
    const activeTheme = typeof display.getActiveCustomTheme === 'function'
      ? display.getActiveCustomTheme()
      : null
    livePreviewBaseColors.value = {
      ...getPaletteBaseColors(),
      ...(activeTheme?.colors || {}),
      ...currentCssColors,
    }
    livePreviewFloatingVisible.value = true
    applyPreviewColorsToRoot(buildLivePreviewColors())
  } catch (error) {
    console.error('[CustomThemePanel] 开启实时预览失败', error)
    livePreviewFloatingVisible.value = false
    livePreviewBaseColors.value = null
  }
}

const handleCloseLivePreview = () => {
  try {
    livePreviewFloatingVisible.value = false
    livePreviewBaseColors.value = null
    stopLivePreviewSafely()
  } catch (error) {
    console.error('[CustomThemePanel] 关闭实时预览失败', error)
  }
}

const toggleLivePreview = () => {
  if (livePreviewFloatingVisible.value) {
    handleCloseLivePreview()
    return
  }
  handleOpenLivePreview()
}

const handleLivePreviewFloatingShowUpdate = (visible: boolean) => {
  if (!visible) {
    handleCloseLivePreview()
  }
}

const handleLivePreviewFloatingColorUpdate = (payload: { key: keyof CustomThemeColors; value: string | null }) => {
  updateColor(payload.key, payload.value)
}

// 监听显示状态
watch(() => props.show, (visible) => {
  if (visible) {
    selectedThemeId.value = activeThemeId.value || themes.value[0]?.id || null
    // 默认进入新建模式
    if (themes.value.length === 0) {
      startCreate()
    } else {
      resetForm()
    }
  }
})

watch(
  () => [activeThemeId.value, themes.value.map(theme => theme.id).join(',')],
  () => {
    if (activeThemeId.value) {
      selectedThemeId.value = activeThemeId.value
      return
    }
    const fallbackId = themes.value[0]?.id || null
    selectedThemeId.value = fallbackId
  },
  { immediate: true },
)

watch(
  () => themeColors.value,
  () => {
    if (!livePreviewFloatingVisible.value) return
    applyPreviewColorsToRoot(buildLivePreviewColors())
  },
  { deep: true },
)

onBeforeUnmount(() => {
  livePreviewBaseColors.value = null
  stopLivePreviewSafely()
})

const updateColor = (key: keyof CustomThemeColors, value: string | null) => {
  if (value) {
    themeColors.value[key] = value
  } else {
    delete themeColors.value[key]
  }
}

const getColorValue = (key: keyof CustomThemeColors): string | null => {
  return themeColors.value[key] || null
}

// JSON 导出主题
const exportTheme = (theme: CustomTheme) => {
  const exportData = {
    name: theme.name,
    colors: theme.colors,
    exportedAt: new Date().toISOString(),
    version: '1.0',
  }
  const json = JSON.stringify(exportData, null, 2)
  const blob = new Blob([json], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `sealchat-theme-${theme.name.replace(/[^a-zA-Z0-9\u4e00-\u9fa5]/g, '_')}.json`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// JSON 导入主题
const importFileInput = ref<HTMLInputElement | null>(null)
const importError = ref<string | null>(null)

const triggerImport = () => {
  importFileInput.value?.click()
}

const handleImportFile = (event: Event) => {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  
  importError.value = null
  
  const reader = new FileReader()
  reader.onload = (e) => {
    try {
      const content = e.target?.result as string
      const data = JSON.parse(content)
      
      // 验证必需字段
      if (!data.name || typeof data.name !== 'string') {
        importError.value = '无效的主题文件：缺少 name 字段'
        return
      }
      if (!data.colors || typeof data.colors !== 'object') {
        importError.value = '无效的主题文件：缺少 colors 字段'
        return
      }
      
      // 创建新主题
      const existingTheme = themes.value.find(t => t.name === data.name)
      const uniqueName = existingTheme 
        ? `${data.name} ${Date.now().toString(36).slice(-4)}`
        : data.name
      
      const theme: CustomTheme = {
        id: `imported_${Date.now()}`,
        name: uniqueName,
        colors: { ...data.colors },
        createdAt: Date.now(),
        updatedAt: Date.now(),
      }
      
      // 确保自定义主题功能已启用
      if (!display.settings.customThemeEnabled) {
        display.setCustomThemeEnabled(true)
      }
      
      display.saveCustomTheme(theme)
      display.activateCustomTheme(theme.id)
      
      // 重置文件输入
      target.value = ''
    } catch (err) {
      importError.value = '无效的 JSON 文件'
      console.error('Import theme error:', err)
    }
  }
  reader.readAsText(file)
}
</script>

<template>
  <n-drawer :show="props.show" :width="drawerWidth" placement="right" @update:show="emit('update:show', $event)">
    <n-drawer-content closable title="自定义主题">
      <div class="custom-theme-panel">
        <!-- 主题选择 -->
        <section class="theme-section" v-if="themes.length > 0">
          <p class="section-title">已保存的主题</p>
          <div class="theme-selector-row">
            <n-select
              v-model:value="selectedThemeId"
              :options="savedThemeOptions"
              filterable
              clearable
              placeholder="搜索并选择已保存主题..."
              size="small"
              class="theme-select"
              @update:value="handleThemeSelect"
            />
            <div class="theme-selector-actions">
              <n-button text size="small" :disabled="!selectedSavedTheme" @click="handleExportSelectedTheme">导出</n-button>
              <n-button text size="small" :disabled="!selectedSavedTheme" @click="handleEditSelectedTheme">编辑</n-button>
              <n-button text size="small" type="error" :disabled="!selectedSavedTheme" @click="handleDeleteSelectedTheme">删除</n-button>
            </div>
          </div>
        </section>

        <n-divider v-if="themes.length > 0" />

        <!-- 导入/导出 JSON -->
        <section class="theme-section">
          <p class="section-title">导入/导出</p>
          <div class="import-export-section">
            <input
              ref="importFileInput"
              type="file"
              accept=".json,application/json"
              style="display: none"
              @change="handleImportFile"
            />
            <n-button size="small" @click="triggerImport">从 JSON 文件导入</n-button>
            <n-text v-if="importError" type="error" class="import-error">{{ importError }}</n-text>
          </div>
        </section>

        <n-divider />

        <!-- 导入主题模板 -->
        <section class="theme-section">
          <p class="section-title">导入主题模板</p>
          <div class="preset-import">
            <n-select
              v-model:value="selectedImportSource"
              :options="importThemeOptions"
              filterable
              clearable
              placeholder="搜索并选择主题模板..."
              size="small"
              @update:value="importThemeFromSource"
              class="preset-select"
            />
            <p class="preset-hint">支持预设主题和已保存主题，选择后仅填充编辑器，确认保存后才写入主题列表</p>
          </div>
        </section>

        <n-divider />

        <!-- 编辑/新建表单 -->
        <section class="theme-section">
          <div class="section-header">
            <p class="section-title">{{ editMode === 'edit' ? '编辑主题' : '新建主题' }}</p>
            <n-button v-if="editMode === 'edit'" text size="small" @click="startCreate">取消编辑</n-button>
          </div>

          <div class="theme-live-preview-row">
            <n-button
              size="small"
              secondary
              :type="livePreviewFloatingVisible ? 'warning' : 'primary'"
              @click="toggleLivePreview"
            >
              {{ livePreviewFloatingVisible ? '关闭实时预览' : '开启实时预览' }}
            </n-button>
            <span class="theme-live-preview-hint">
              {{ livePreviewFloatingVisible ? '预览悬浮窗已开启，可持续对照当前调色效果' : '点击开启后，颜色修改将实时渲染到当前页面' }}
            </span>
          </div>

          <n-form label-placement="left" label-width="80">
            <n-form-item label="主题名称">
              <n-input v-model:value="themeName" placeholder="输入主题名称" maxlength="32" show-count />
            </n-form-item>
          </n-form>

          <div class="color-groups">
            <div v-for="(fields, groupName) in colorGroups" :key="groupName" class="color-group">
              <p class="color-group__title">{{ groupName }}</p>
              <div class="color-group__items">
                <div v-for="field in fields" :key="field.key" class="color-item">
                  <span class="color-item__label">{{ field.label }}</span>
                  <div class="color-item__picker">
                    <n-color-picker
                      :value="themeColors[field.key] || undefined"
                      :show-alpha="true"
                      size="small"
                      :modes="['hex', 'rgb', 'hsl']"
                      :default-value="'#808080'"
                      :show-preview="true"
                      :actions="['confirm']"
                      :swatches="['#ffffff', '#f8fafc', '#1b1b20', '#2a282a', '#3F3F46', '#FBFDF7', '#3388de', '#2563eb', '#10b981', '#f59e0b', '#ef4444']"
                      @update:value="(v: string | null) => updateColor(field.key, v)"
                    >
                    <template #label>
                        <div 
                          class="color-swatch-trigger" 
                          :class="{ 'color-swatch-trigger--empty': !themeColors[field.key] }"
                          :style="{ backgroundColor: themeColors[field.key] || 'transparent' }"
                        ></div>
                      </template>
                    </n-color-picker>
                    <n-button
                      v-if="themeColors[field.key]"
                      text
                      size="tiny"
                      @click="updateColor(field.key, null)"
                    >
                      清除
                    </n-button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <n-button type="primary" block :disabled="!themeName.trim()" @click="handleSave">
            {{ editMode === 'edit' ? '保存修改' : '创建主题' }}
          </n-button>
        </section>
      </div>

      <template #footer>
        <n-button @click="handleClose">关闭</n-button>
      </template>
    </n-drawer-content>
  </n-drawer>
  <ThemeLivePreviewFloating
    :show="livePreviewFloatingVisible"
    :color-fields="colorFields"
    :theme-colors="themeColors"
    @update:show="handleLivePreviewFloatingShowUpdate"
    @update:theme-color="handleLivePreviewFloatingColorUpdate"
    @save-theme="handleSaveFromLivePreviewFloating"
  />
</template>

<style scoped lang="scss">
.custom-theme-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.theme-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.section-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.theme-selector-row {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.theme-select {
  width: 100%;
}

.theme-selector-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.25rem;
}

.theme-live-preview-row {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}

.theme-live-preview-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.preset-import {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.preset-select {
  max-width: 100%;
}

.preset-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  margin: 0;
}

.import-export-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: flex-start;
}

.import-error {
  font-size: 0.75rem;
  margin-top: 0.25rem;
}

.color-groups {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  margin-bottom: 1rem;
}

.color-group__title {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--sc-text-secondary);
  margin-bottom: 0.5rem;
}

.color-group__items {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.color-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.35rem 0;
}

.color-item__label {
  font-size: 0.85rem;
  color: var(--sc-text-primary);
}

.color-item__picker {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.color-swatch-trigger {
  width: 36px;
  height: 24px;
  border-radius: 4px;
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.15));
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.1);

  &:hover {
    border-color: var(--sc-border-strong, rgba(0, 0, 0, 0.3));
    box-shadow: 0 0 0 2px rgba(51, 136, 222, 0.2);
  }

  &--empty {
    border-style: dashed;
    background: repeating-linear-gradient(
      45deg,
      transparent,
      transparent 3px,
      rgba(128, 128, 128, 0.1) 3px,
      rgba(128, 128, 128, 0.1) 6px
    ) !important;
  }
}

/* Minimal clean scrollbar for custom theme panel */
.custom-theme-panel {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.3) transparent;

  &::-webkit-scrollbar {
    width: 4px;
  }

  &::-webkit-scrollbar-track {
    background: transparent;
  }

  &::-webkit-scrollbar-thumb {
    background: rgba(128, 128, 128, 0.3);
    border-radius: 2px;

    &:hover {
      background: rgba(128, 128, 128, 0.5);
    }
  }
}

/* Drawer content minimal scrollbar */
:deep(.n-drawer-body-content-wrapper) {
  scrollbar-width: thin;
  scrollbar-color: rgba(128, 128, 128, 0.3) transparent;

  &::-webkit-scrollbar {
    width: 4px;
  }

  &::-webkit-scrollbar-track {
    background: transparent;
  }

  &::-webkit-scrollbar-thumb {
    background: rgba(128, 128, 128, 0.3);
    border-radius: 2px;
  }
}

/* ========== 移动端响应式设计 ========== */
@media (max-width: 600px) {
  .custom-theme-panel {
    gap: 0.75rem;
  }

  .theme-section {
    gap: 0.5rem;
  }

  .section-title {
    font-size: 0.85rem;
  }

  .theme-selector-actions {
    width: 100%;
    justify-content: flex-end;
    gap: 0.5rem;
  }

  .color-item {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
  }

  .color-item__picker {
    width: 100%;
    justify-content: flex-start;
  }

  .color-swatch-trigger {
    width: 48px;
    height: 32px;
  }

  .import-export-section {
    width: 100%;
  }

  .import-export-section .n-button {
    width: 100%;
  }

  .preset-select {
    width: 100%;
  }

  .theme-live-preview-row .n-button {
    width: 100%;
  }

  /* 更大的触摸目标 */
  .theme-selector-actions .n-button {
    padding: 0.35rem 0.5rem;
    font-size: 0.8rem;
  }

  /* 颜色分组更紧凑 */
  .color-groups {
    gap: 0.75rem;
  }

  .color-group__items {
    gap: 0.35rem;
  }
}
</style>
