<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDisplayStore, type CustomTheme, type CustomThemeColors } from '@/stores/display'
import { presetThemes, type PresetTheme } from '@/config/presetThemes'

interface Props {
  show: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const display = useDisplayStore()

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
  { key: 'chatIcBg', label: '场内消息', group: '聊天区域' },
  { key: 'chatOocBg', label: '场外消息', group: '聊天区域' },
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

// 预设主题选项
const presetOptions = computed(() => 
  presetThemes.map(p => ({ label: p.name, value: p.id, description: p.description }))
)

// 导入预设主题
const importPreset = (presetId: string) => {
  const preset = presetThemes.find(p => p.id === presetId)
  if (!preset) return
  
  // 检查是否已存在同名主题
  const existingTheme = themes.value.find(t => t.name === preset.name)
  const uniqueName = existingTheme 
    ? `${preset.name} ${Date.now().toString(36).slice(-4)}`
    : preset.name
  
  const theme: CustomTheme = {
    id: `preset_${Date.now()}`,
    name: uniqueName,
    colors: { ...preset.colors },
    createdAt: Date.now(),
    updatedAt: Date.now(),
  }
  
  // 先确保自定义主题功能已启用，这样后续的 applyTheme 才能生效
  if (!display.settings.customThemeEnabled) {
    display.setCustomThemeEnabled(true)
  }
  
  display.saveCustomTheme(theme)
  display.activateCustomTheme(theme.id)
}

const handleSave = () => {
  if (!themeName.value.trim()) return
  const id = editingThemeId.value || `theme_${Date.now()}`
  const theme: CustomTheme = {
    id,
    name: themeName.value.trim(),
    colors: { ...themeColors.value },
    createdAt: Date.now(),
    updatedAt: Date.now(),
  }
  display.saveCustomTheme(theme)
  // 自动激活新创建的主题
  if (editMode.value === 'create') {
    display.activateCustomTheme(id)
  }
  resetForm()
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

// 监听显示状态
watch(() => props.show, (visible) => {
  if (visible) {
    // 默认进入新建模式
    if (themes.value.length === 0) {
      startCreate()
    } else {
      resetForm()
    }
  }
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
  <n-drawer :show="props.show" :width="480" placement="right" @update:show="emit('update:show', $event)">
    <n-drawer-content closable title="自定义主题">
      <div class="custom-theme-panel">
        <!-- 主题列表 -->
        <section class="theme-section" v-if="themes.length > 0">
          <p class="section-title">已保存的主题</p>
          <div class="theme-list">
            <div
              v-for="theme in themes"
              :key="theme.id"
              class="theme-item"
              :class="{ 'is-active': activeThemeId === theme.id }"
              @click="handleActivate(theme.id)"
            >
              <div class="theme-item__info">
                <span class="theme-item__name">{{ theme.name }}</span>
                <n-tag v-if="activeThemeId === theme.id" size="small" type="success">当前</n-tag>
              </div>
              <div class="theme-item__actions">
                <n-button text size="small" @click.stop="exportTheme(theme)">导出</n-button>
                <n-button text size="small" @click.stop="startEdit(theme)">编辑</n-button>
                <n-button text size="small" type="error" @click.stop="handleDelete(theme.id)">删除</n-button>
              </div>
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

        <!-- 预设主题 -->
        <section class="theme-section">
          <p class="section-title">导入预设主题</p>
          <div class="preset-import">
            <n-select
              :options="presetOptions"
              placeholder="选择预设主题..."
              size="small"
              :render-label="(option: any) => option.label"
              @update:value="importPreset"
              :value="null"
              class="preset-select"
            />
            <p class="preset-hint">选择后将自动导入并激活该预设主题</p>
          </div>
        </section>

        <n-divider />

        <!-- 编辑/新建表单 -->
        <section class="theme-section">
          <div class="section-header">
            <p class="section-title">{{ editMode === 'edit' ? '编辑主题' : '新建主题' }}</p>
            <n-button v-if="editMode === 'edit'" text size="small" @click="startCreate">取消编辑</n-button>
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

.theme-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.theme-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.6rem 0.75rem;
  border-radius: 0.5rem;
  border: 1px solid var(--sc-border-mute);
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    border-color: var(--sc-border-strong);
    background: rgba(0, 0, 0, 0.02);
  }

  &.is-active {
    border-color: var(--primary-color, #3388de);
    background: rgba(51, 136, 222, 0.05);
  }
}

.theme-item__info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.theme-item__name {
  font-size: 0.875rem;
  font-weight: 500;
}

.theme-item__actions {
  display: flex;
  gap: 0.25rem;
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

:root[data-display-palette='night'] .theme-item:hover {
  background: rgba(255, 255, 255, 0.03);
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
</style>
