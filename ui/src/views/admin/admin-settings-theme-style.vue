<script setup lang="ts">
import { cloneDeep } from 'lodash-es'
import { computed, onMounted, ref } from 'vue'
import { Photo as ImageIcon, X } from '@vicons/tabler'
import { NIcon, useMessage } from 'naive-ui'
import type { LoginBackgroundConfig, ServerConfig, ThemeManagementConfig } from '@/types'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { useImageCompressor } from '@/composables/useImageCompressor'
import { useLoginGlass } from '@/composables/useLoginGlass'
import type { CustomThemeColors, PlatformTheme } from '@/services/theme/themeTypes'
import { useDisplayStore } from '@/stores/display'
import { useUtilsStore } from '@/stores/utils'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'

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
const loginBgFileInputRef = ref<HTMLInputElement | null>(null)
const expandedNames = ref<string[]>([])
const loginBackground = ref<LoginBackgroundConfig>({})

const { compress: compressImage } = useImageCompressor()
const loginBgUploading = ref(false)

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
const isModified = computed(() =>
  JSON.stringify({
    themeManagement: model.value,
    loginBackground: loginBackground.value,
  }) !== originalSnapshot.value,
)

const normalizeThemeManagement = (value?: ThemeManagementConfig | null): ThemeManagementConfig => ({
  platformThemes: Array.isArray(value?.platformThemes) ? cloneDeep(value?.platformThemes || []) : [],
  defaultPlatformThemeId: value?.defaultPlatformThemeId || '',
})
const ensureLoginBackground = () => {
  if (!loginBackground.value) {
    loginBackground.value = {}
  }
  return loginBackground.value
}
const loginBgAttachmentId = computed({
  get: () => loginBackground.value?.attachmentId || '',
  set: (val: string) => {
    ensureLoginBackground().attachmentId = val
  },
})
const loginBgMode = computed({
  get: () => loginBackground.value?.mode || 'cover',
  set: (val: 'cover' | 'contain' | 'tile' | 'center') => {
    ensureLoginBackground().mode = val
  },
})
const loginBgOpacity = computed({
  get: () => loginBackground.value?.opacity ?? 30,
  set: (val: number) => {
    ensureLoginBackground().opacity = val
  },
})
const loginBgBlur = computed({
  get: () => loginBackground.value?.blur ?? 0,
  set: (val: number) => {
    ensureLoginBackground().blur = val
  },
})
const loginBgBrightness = computed({
  get: () => loginBackground.value?.brightness ?? 100,
  set: (val: number) => {
    ensureLoginBackground().brightness = val
  },
})
const loginBgOverlayColor = computed({
  get: () => loginBackground.value?.overlayColor || '',
  set: (val: string) => {
    ensureLoginBackground().overlayColor = val
  },
})
const loginBgOverlayOpacity = computed({
  get: () => loginBackground.value?.overlayOpacity ?? 0,
  set: (val: number) => {
    ensureLoginBackground().overlayOpacity = val
  },
})
const loginPanelAutoTint = computed({
  get: () => loginBackground.value?.panelAutoTint ?? true,
  set: (val: boolean) => {
    ensureLoginBackground().panelAutoTint = val
  },
})
const loginPanelTintColor = computed({
  get: () => loginBackground.value?.panelTintColor || '',
  set: (val: string) => {
    ensureLoginBackground().panelTintColor = val
  },
})
const loginPanelTintOpacity = computed({
  get: () => loginBackground.value?.panelTintOpacity ?? 72,
  set: (val: number) => {
    ensureLoginBackground().panelTintOpacity = val
  },
})
const loginPanelBlur = computed({
  get: () => loginBackground.value?.panelBlur ?? 14,
  set: (val: number) => {
    ensureLoginBackground().panelBlur = val
  },
})
const loginPanelSaturate = computed({
  get: () => loginBackground.value?.panelSaturate ?? 120,
  set: (val: number) => {
    ensureLoginBackground().panelSaturate = val
  },
})
const loginPanelContrast = computed({
  get: () => loginBackground.value?.panelContrast ?? 105,
  set: (val: number) => {
    ensureLoginBackground().panelContrast = val
  },
})
const loginPanelBorderOpacity = computed({
  get: () => loginBackground.value?.panelBorderOpacity ?? 18,
  set: (val: number) => {
    ensureLoginBackground().panelBorderOpacity = val
  },
})
const loginPanelShadowStrength = computed({
  get: () => loginBackground.value?.panelShadowStrength ?? 22,
  set: (val: number) => {
    ensureLoginBackground().panelShadowStrength = val
  },
})
const loginBgUrl = computed(() => {
  const id = loginBgAttachmentId.value
  if (!id) return ''
  return resolveAttachmentUrl(id.startsWith('id:') ? id : `id:${id}`)
})
const loginBgModeOptions = [
  { label: '铺满 (Cover)', value: 'cover' },
  { label: '适应 (Contain)', value: 'contain' },
  { label: '平铺 (Tile)', value: 'tile' },
  { label: '居中 (Center)', value: 'center' },
]
const loginBgPreviewStyle = computed(() => {
  if (!loginBgUrl.value) return {}
  const mode = loginBgMode.value
  let bgSize = 'cover'
  let bgRepeat = 'no-repeat'
  let bgPosition = 'center'
  switch (mode) {
    case 'contain':
      bgSize = 'contain'
      break
    case 'tile':
      bgSize = 'auto'
      bgRepeat = 'repeat'
      break
    case 'center':
      bgSize = 'auto'
      bgPosition = 'center'
      break
  }
  return {
    backgroundImage: `url(${loginBgUrl.value})`,
    backgroundSize: bgSize,
    backgroundRepeat: bgRepeat,
    backgroundPosition: bgPosition,
    opacity: loginBgOpacity.value / 100,
    filter: `blur(${loginBgBlur.value}px) brightness(${loginBgBrightness.value}%)`,
  }
})
const loginBgOverlayStyle = computed(() => {
  if (!loginBgOverlayColor.value || !loginBgOverlayOpacity.value) return null
  return {
    backgroundColor: loginBgOverlayColor.value,
    opacity: loginBgOverlayOpacity.value / 100,
  }
})
const { glassStyle: loginGlassStyle } = useLoginGlass({
  imageUrl: loginBgUrl,
  config: computed(() => loginBackground.value),
  enabled: computed(() => !!loginBgUrl.value),
  radius: '8px',
})
const loginGlassPreviewStyle = computed(() => ({
  ...loginGlassStyle.value,
  '--sc-glass-radius': '8px',
}))

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

const triggerLoginBgUpload = () => {
  loginBgFileInputRef.value?.click()
}
const clearLoginBg = () => {
  loginBgAttachmentId.value = ''
}
const handleLoginBgFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input?.files?.[0]
  if (!file) return
  input.value = ''

  const sizeLimit = utils.fileSizeLimit
  if (file.size > sizeLimit) {
    const limitMB = (sizeLimit / 1024 / 1024).toFixed(1)
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`)
    return
  }

  loginBgUploading.value = true
  try {
    const compressed = await compressImage(file, { maxWidth: 1920, maxHeight: 1080 })
    const result = await uploadImageAttachment(compressed, {
      channelId: 'login-background',
      skipCompression: true,
    })
    let attachId = result.attachmentId || ''
    if (attachId.startsWith('id:')) {
      attachId = attachId.slice(3)
    }
    loginBgAttachmentId.value = attachId
    message.success('背景图片上传成功')
    if (!expandedNames.value.includes('login-bg')) {
      expandedNames.value = [...expandedNames.value, 'login-bg']
    }
  } catch (error: any) {
    message.error(error?.message || '上传失败')
  } finally {
    loginBgUploading.value = false
  }
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
  loginBackground.value = cloneDeep(utils.config?.loginBackground || {})
  originalSnapshot.value = JSON.stringify({
    themeManagement: model.value,
    loginBackground: loginBackground.value,
  })
}

const save = async () => {
  saving.value = true
  try {
    if (!utils.config) {
      await utils.configGet()
    }
    const payload: ServerConfig = cloneDeep((utils.config || {}) as ServerConfig)
    payload.themeManagement = cloneDeep(model.value)
    payload.loginBackground = cloneDeep(loginBackground.value)
    await utils.configSet(payload)
    model.value = normalizeThemeManagement(payload.themeManagement)
    loginBackground.value = cloneDeep(payload.loginBackground || {})
    originalSnapshot.value = JSON.stringify({
      themeManagement: model.value,
      loginBackground: loginBackground.value,
    })
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
  <div class="admin-settings-scroll overflow-y-auto pr-2" style="max-height: 61vh; margin-top: 0;">
    <input
      ref="importFileInputRef"
      type="file"
      accept=".json,application/json"
      class="admin-theme-style__hidden-input"
      @change="handleImportFile"
    >
    <input
      ref="loginBgFileInputRef"
      type="file"
      accept="image/*"
      class="admin-theme-style__hidden-input"
      @change="handleLoginBgFileChange"
    >

    <n-form label-placement="left" label-width="120">
      <n-collapse v-model:expanded-names="expandedNames" class="settings-collapse">
        <n-collapse-item title="平台主题列表" name="platform-theme-list">
          <n-form-item label="默认主题">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ defaultPlatformThemeName || '未设置' }}</span>
          </n-form-item>
          <n-form-item label="平台主题数">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ platformThemes.length }}/50</span>
          </n-form-item>
          <n-form-item label="导入列表">
            <div class="flex flex-wrap items-center gap-2 w-full">
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
          </n-form-item>
          <n-form-item label="主题列表">
            <div class="theme-list-wrap">
              <n-empty v-if="platformThemes.length === 0" description="暂无平台主题，可通过 JSON 或个人主题导入" />
              <div v-else class="theme-list">
                <div v-for="theme in platformThemes" :key="theme.id" class="theme-list-item">
                  <div class="theme-list-item__main">
                    <div class="theme-list-item__title">
                      <span>{{ theme.name }}</span>
                      <n-tag v-if="model.defaultPlatformThemeId === theme.id" size="small" type="success" round>默认</n-tag>
                    </div>
                    <div class="theme-list-item__meta">
                      <span>更新时间：{{ formatTimestamp(theme.updatedAt) }}</span>
                    </div>
                  </div>
                  <div class="theme-list-item__actions">
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
          </n-form-item>
        </n-collapse-item>

        <n-collapse-item title="登录页背景" name="login-bg">
          <n-form-item label="背景图片">
            <div class="flex flex-wrap items-center gap-3 w-full">
              <div
                class="login-bg-no-option"
                :class="{ active: !loginBgAttachmentId }"
                @click="clearLoginBg"
              >
                <NIcon :component="X" :size="16" />
                <span>无</span>
              </div>
              <div v-if="loginBgUrl" class="login-bg-thumb-wrapper">
                <img :src="loginBgUrl" alt="登录背景" class="login-bg-thumb">
              </div>
              <n-button size="small" :loading="loginBgUploading" @click="triggerLoginBgUpload">
                <template #icon><NIcon :component="ImageIcon" /></template>
                {{ loginBgUrl ? '更换' : '上传' }}
              </n-button>
            </div>
          </n-form-item>

          <template v-if="loginBgAttachmentId">
            <n-form-item label="显示模式">
              <n-select v-model:value="loginBgMode" :options="loginBgModeOptions" class="settings-input-inline" />
            </n-form-item>
            <n-form-item label="透明度">
              <div class="login-bg-control-row">
                <n-slider class="login-bg-slider" v-model:value="loginBgOpacity" :min="0" :max="100" :step="1" :tooltip="false" />
                <span class="login-bg-value">{{ loginBgOpacity }}%</span>
              </div>
            </n-form-item>
            <n-form-item label="模糊度">
              <div class="login-bg-control-row">
                <n-slider class="login-bg-slider" v-model:value="loginBgBlur" :min="0" :max="20" :step="1" :tooltip="false" />
                <span class="login-bg-value">{{ loginBgBlur }}px</span>
              </div>
            </n-form-item>
            <n-form-item label="亮度">
              <div class="login-bg-control-row">
                <n-slider class="login-bg-slider" v-model:value="loginBgBrightness" :min="50" :max="150" :step="1" :tooltip="false" />
                <span class="login-bg-value">{{ loginBgBrightness }}%</span>
              </div>
            </n-form-item>
            <n-form-item label="叠加层颜色">
              <div class="flex flex-wrap items-center gap-2 w-full">
                <n-color-picker v-model:value="loginBgOverlayColor" :show-alpha="false" style="width: 100px;" />
                <n-button v-if="loginBgOverlayColor" size="tiny" quaternary @click="loginBgOverlayColor = ''">清除</n-button>
              </div>
            </n-form-item>
            <n-form-item v-if="loginBgOverlayColor" label="叠加层透明度">
              <div class="login-bg-control-row">
                <n-slider class="login-bg-slider" v-model:value="loginBgOverlayOpacity" :min="0" :max="100" :step="1" :tooltip="false" />
                <span class="login-bg-value">{{ loginBgOverlayOpacity }}%</span>
              </div>
            </n-form-item>
          </template>
        </n-collapse-item>

        <n-collapse-item v-if="loginBgAttachmentId" title="玻璃卡片" name="login-glass">
          <n-form-item label="自动色调">
            <n-switch v-model:value="loginPanelAutoTint" />
          </n-form-item>
          <n-form-item label="玻璃色">
            <div class="flex flex-wrap items-center gap-2 w-full">
              <n-color-picker v-model:value="loginPanelTintColor" :show-alpha="false" style="width: 100px;" />
              <n-button v-if="loginPanelTintColor" size="tiny" quaternary @click="loginPanelTintColor = ''">清除</n-button>
            </div>
          </n-form-item>
          <n-form-item label="玻璃透明度">
            <div class="login-bg-control-row">
              <n-slider class="login-bg-slider" v-model:value="loginPanelTintOpacity" :min="30" :max="95" :step="1" :tooltip="false" />
              <span class="login-bg-value">{{ loginPanelTintOpacity }}%</span>
            </div>
          </n-form-item>
          <n-form-item label="玻璃模糊">
            <div class="login-bg-control-row">
              <n-slider class="login-bg-slider" v-model:value="loginPanelBlur" :min="0" :max="30" :step="1" :tooltip="false" />
              <span class="login-bg-value">{{ loginPanelBlur }}px</span>
            </div>
          </n-form-item>
          <n-form-item label="饱和度">
            <div class="login-bg-control-row">
              <n-slider class="login-bg-slider" v-model:value="loginPanelSaturate" :min="80" :max="180" :step="1" :tooltip="false" />
              <span class="login-bg-value">{{ loginPanelSaturate }}%</span>
            </div>
          </n-form-item>
          <n-form-item label="对比度">
            <div class="login-bg-control-row">
              <n-slider class="login-bg-slider" v-model:value="loginPanelContrast" :min="90" :max="140" :step="1" :tooltip="false" />
              <span class="login-bg-value">{{ loginPanelContrast }}%</span>
            </div>
          </n-form-item>
          <n-form-item label="边框强度">
            <div class="login-bg-control-row">
              <n-slider class="login-bg-slider" v-model:value="loginPanelBorderOpacity" :min="0" :max="60" :step="1" :tooltip="false" />
              <span class="login-bg-value">{{ loginPanelBorderOpacity }}%</span>
            </div>
          </n-form-item>
          <n-form-item label="阴影强度">
            <div class="login-bg-control-row">
              <n-slider class="login-bg-slider" v-model:value="loginPanelShadowStrength" :min="0" :max="60" :step="1" :tooltip="false" />
              <span class="login-bg-value">{{ loginPanelShadowStrength }}%</span>
            </div>
          </n-form-item>
          <n-form-item label="预览">
            <div class="login-bg-preview">
              <div class="login-bg-preview-layer" :style="loginBgPreviewStyle"></div>
              <div v-if="loginBgOverlayStyle" class="login-bg-preview-overlay" :style="loginBgOverlayStyle"></div>
              <div class="login-bg-preview-card sc-glass-panel" :style="loginGlassPreviewStyle">
                <div class="login-bg-preview-input"></div>
                <div class="login-bg-preview-input"></div>
                <div class="login-bg-preview-btn"></div>
              </div>
            </div>
          </n-form-item>
        </n-collapse-item>
      </n-collapse>
    </n-form>
  </div>
</template>

<style scoped>
.admin-settings-scroll {
  overflow-x: hidden;
  overflow-y: scroll;
  scrollbar-gutter: stable;
}

.settings-collapse {
  width: 100%;
}

.admin-theme-style__toolbar-select {
  min-width: 240px;
  flex: 1 1 260px;
}

.admin-theme-style__hidden-input {
  display: none;
}

.settings-input-inline {
  max-width: 220px;
}

.theme-list-wrap,
.theme-list {
  width: 100%;
}

.theme-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.theme-list-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px;
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  background: var(--sc-bg-surface, rgba(255, 255, 255, 0.02));
}

.theme-list-item__main {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.theme-list-item__title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-weight: 600;
}

.theme-list-item__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 13px;
  color: var(--sc-text-secondary);
}

.theme-list-item__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.login-bg-no-option {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border: 2px dashed #d1d5db;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
  font-size: 12px;
  color: #9ca3af;
}

.login-bg-no-option:hover {
  border-color: #9ca3af;
  color: #6b7280;
}

.login-bg-no-option.active {
  border-color: #3b82f6;
  background-color: #eff6ff;
  color: #3b82f6;
}

.dark .login-bg-no-option {
  border-color: #4b5563;
  color: #6b7280;
}

.dark .login-bg-no-option:hover {
  border-color: #6b7280;
  color: #9ca3af;
}

.dark .login-bg-no-option.active {
  border-color: #3b82f6;
  background-color: #1e3a5f;
  color: #60a5fa;
}

.login-bg-thumb-wrapper {
  width: 48px;
  height: 48px;
  border-radius: 6px;
  overflow: hidden;
  border: 2px solid #3b82f6;
}

.login-bg-thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.login-bg-preview {
  position: relative;
  width: 240px;
  max-width: 100%;
  height: 160px;
  border-radius: 8px;
  overflow: hidden;
  background-color: #f3f4f6;
}

.dark .login-bg-preview {
  background-color: #1f2937;
}

.login-bg-preview-layer {
  position: absolute;
  inset: 0;
  z-index: 0;
}

.login-bg-preview-overlay {
  position: absolute;
  inset: 0;
  z-index: 1;
}

.login-bg-preview-card {
  position: absolute;
  inset: 18px 24px;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px;
}

.login-bg-preview-input {
  width: 100%;
  height: 18px;
  background: rgba(255, 255, 255, 0.65);
  border-radius: 4px;
}

.dark .login-bg-preview-input {
  background: rgba(15, 23, 42, 0.6);
}

.login-bg-preview-btn {
  width: 60%;
  height: 20px;
  background: rgba(59, 130, 246, 0.9);
  border-radius: 4px;
  margin-top: 4px;
}

.login-bg-control-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  width: 100%;
}

.login-bg-slider {
  flex: 1;
  min-width: 200px;
}

.login-bg-value {
  display: inline-block;
  width: 56px;
  margin-left: 8px;
  font-size: 12px;
  color: #6b7280;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.dark .login-bg-value {
  color: #9ca3af;
}

@media (max-width: 720px) {
  .theme-list-item {
    flex-direction: column;
    align-items: stretch;
  }

  .theme-list-item__actions {
    justify-content: flex-start;
  }
}
</style>
