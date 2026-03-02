<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useDisplayStore } from '@/stores/display'
import { buildGlobalFontFamilyStack, createFontAssetId, sanitizeFontFamilyName } from '@/services/font/fontUtils'
import { listFontAssetMeta, deleteFontAssetById, isFontAssetCacheAvailable, saveFontAsset } from '@/services/font/fontCache'
import { isLocalFontApiAvailable, loadFontFromFile, loadFontFromUrl, queryLocalFontCandidates, restoreCachedFontById } from '@/services/font/fontLoader'
import type { LocalFontCandidate } from '@/services/font/fontLoader'
import type { FontAssetMeta, FontSourceType, ImportedFontPayload } from '@/services/font/types'

interface Props {
  show: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const message = useMessage()
const display = useDisplayStore()

interface LocalFontOption {
  label: string
  value: string
  aliases: string[]
}

const sourceMode = ref<FontSourceType>('system')
const localFontOptions = ref<LocalFontOption[]>([])
const loadingLocalFonts = ref(false)
const selectedLocalFamily = ref('')
const manualFamily = ref('')
const localFontAliasLookup = ref<Record<string, string>>({})
const enhancedCoverageEnabled = ref(false)
const uploadFamily = ref('')
const urlValue = ref('')
const urlFamily = ref('')
const importedDraft = ref<ImportedFontPayload | null>(null)
const selectedCachedAssetId = ref<string | null>(null)
const cachedAssets = ref<FontAssetMeta[]>([])
const processing = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)

const localFontAvailable = computed(() => isLocalFontApiAvailable())
const cacheAvailable = computed(() => isFontAssetCacheAvailable())

const selectedCachedAsset = computed(() =>
  cachedAssets.value.find(asset => asset.id === selectedCachedAssetId.value) || null,
)

const currentDisplayFamily = computed(() => sanitizeFontFamilyName(display.settings.globalFontFamily))
const normalizeLookupKey = (value: string): string => sanitizeFontFamilyName(value).toLowerCase()

const rebuildLocalAliasLookup = (options: LocalFontOption[]) => {
  const lookup: Record<string, string> = {}
  for (const option of options) {
    const family = sanitizeFontFamilyName(option.value)
    if (!family) continue
    for (const rawName of [option.label, option.value, ...option.aliases]) {
      const key = normalizeLookupKey(rawName || '')
      if (!key || lookup[key]) continue
      lookup[key] = family
    }
  }
  localFontAliasLookup.value = lookup
}

const buildLocalFontOptions = (candidates: LocalFontCandidate[]): LocalFontOption[] => {
  const labelCount = new Map<string, number>()
  for (const item of candidates) {
    const count = labelCount.get(item.displayName) || 0
    labelCount.set(item.displayName, count + 1)
  }
  const options = candidates.map((item) => {
    const label = (labelCount.get(item.displayName) || 0) > 1
      ? `${item.displayName}（${item.family}）`
      : item.displayName
    return {
      label,
      value: item.family,
      aliases: item.aliases,
    }
  })
  rebuildLocalAliasLookup(options)
  return options
}

const resolveManualFamilyInput = async (value: string): Promise<string> => {
  const normalized = sanitizeFontFamilyName(value)
  if (!normalized) return ''
  const key = normalizeLookupKey(normalized)
  const mapped = localFontAliasLookup.value[key]
  if (mapped) return mapped
  if (!localFontAvailable.value) return normalized
  try {
    const options = buildLocalFontOptions(await queryLocalFontCandidates())
    localFontOptions.value = options
    const refreshedMapped = localFontAliasLookup.value[key]
    return refreshedMapped || normalized
  } catch {
    return normalized
  }
}

const resolvedManualPreviewFamily = computed(() => {
  const normalized = sanitizeFontFamilyName(manualFamily.value)
  if (!normalized) return ''
  const mapped = localFontAliasLookup.value[normalizeLookupKey(normalized)]
  return mapped || normalized
})

const previewFamily = computed(() => {
  if (sourceMode.value === 'system') {
    return sanitizeFontFamilyName(selectedLocalFamily.value) || currentDisplayFamily.value
  }
  if (sourceMode.value === 'manual') {
    return sanitizeFontFamilyName(resolvedManualPreviewFamily.value) || currentDisplayFamily.value
  }
  if (importedDraft.value) {
    return sanitizeFontFamilyName(importedDraft.value.family) || currentDisplayFamily.value
  }
  if (selectedCachedAsset.value) {
    return sanitizeFontFamilyName(selectedCachedAsset.value.family) || currentDisplayFamily.value
  }
  return currentDisplayFamily.value
})

const previewStyle = computed(() => ({
  '--preview-font-family': buildGlobalFontFamilyStack(previewFamily.value),
}))

const refreshCachedAssets = async () => {
  try {
    cachedAssets.value = await listFontAssetMeta()
  } catch (error) {
    console.warn('加载字体缓存列表失败', error)
    cachedAssets.value = []
  }
}

const setupDraftFromCurrent = async () => {
  const currentSource = display.settings.globalFontSourceType
  sourceMode.value = currentSource === 'default' ? 'system' : currentSource
  selectedLocalFamily.value = currentDisplayFamily.value
  manualFamily.value = currentDisplayFamily.value
  uploadFamily.value = currentDisplayFamily.value
  urlFamily.value = currentDisplayFamily.value
  urlValue.value = ''
  importedDraft.value = null
  enhancedCoverageEnabled.value = !!display.settings.fontEnhancedCoverageEnabled
  selectedCachedAssetId.value = display.settings.globalFontAssetId || null
  await refreshCachedAssets()
}

watch(
  () => props.show,
  async (visible) => {
    if (!visible) return
    await setupDraftFromCurrent()
  },
)

const handleLoadLocalFonts = async () => {
  if (!localFontAvailable.value) {
    message.warning('浏览器不支持读取本地字体列表，请改用手动输入或导入字体')
    sourceMode.value = 'manual'
    return
  }
  loadingLocalFonts.value = true
  try {
    const options = buildLocalFontOptions(await queryLocalFontCandidates())
    localFontOptions.value = options
    if (!selectedLocalFamily.value && options.length > 0) {
      selectedLocalFamily.value = options[0].value
    }
    if (options.length === 0) {
      message.warning('未读取到可用系统字体，请改用手动输入')
      sourceMode.value = 'manual'
      return
    }
    message.success(`已读取 ${options.length} 个本地字体`)
  } catch (error: any) {
    message.error(error?.message || '读取本地字体失败')
    sourceMode.value = 'manual'
  } finally {
    loadingLocalFonts.value = false
  }
}

const triggerFileSelect = () => {
  fileInputRef.value?.click()
}

const handleFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  processing.value = true
  try {
    const loaded = await loadFontFromFile(file, uploadFamily.value)
    importedDraft.value = loaded
    sourceMode.value = 'upload'
    uploadFamily.value = loaded.family
    selectedCachedAssetId.value = null
    message.success(`字体已加载：${loaded.family}`)
  } catch (error: any) {
    message.error(error?.message || '字体文件加载失败')
  } finally {
    input.value = ''
    processing.value = false
  }
}

const handleImportFromUrl = async () => {
  processing.value = true
  try {
    const loaded = await loadFontFromUrl(urlValue.value, urlFamily.value)
    importedDraft.value = loaded
    sourceMode.value = 'url'
    urlFamily.value = loaded.family
    selectedCachedAssetId.value = null
    message.success(`字体已加载：${loaded.family}`)
  } catch (error: any) {
    message.error(error?.message || 'URL 字体加载失败')
  } finally {
    processing.value = false
  }
}

const handleUseCachedAsset = async (asset: FontAssetMeta) => {
  processing.value = true
  try {
    await restoreCachedFontById(asset.id)
    sourceMode.value = asset.sourceType
    selectedCachedAssetId.value = asset.id
    importedDraft.value = null
    if (asset.sourceType === 'upload') {
      uploadFamily.value = asset.family
    } else {
      urlFamily.value = asset.family
    }
    message.success(`已选中缓存字体：${asset.family}`)
  } catch (error: any) {
    message.error(error?.message || '读取缓存字体失败')
  } finally {
    processing.value = false
  }
}

const handleDeleteCachedAsset = async (asset: FontAssetMeta) => {
  try {
    await deleteFontAssetById(asset.id)
    if (selectedCachedAssetId.value === asset.id) {
      selectedCachedAssetId.value = null
    }
    if (display.settings.globalFontAssetId === asset.id) {
      display.setGlobalFont({
        family: '',
        sourceType: 'default',
        assetId: null,
      })
      message.warning('当前生效字体缓存已删除，已回退到默认字体链')
    }
    await refreshCachedAssets()
  } catch (error: any) {
    message.error(error?.message || '删除缓存字体失败')
  }
}

const resolveSubmitPayload = async (): Promise<{ family: string; sourceType: FontSourceType; assetId: string | null } | null> => {
  if (sourceMode.value === 'system') {
    const family = sanitizeFontFamilyName(selectedLocalFamily.value)
    return {
      family,
      sourceType: family ? 'system' : 'default',
      assetId: null,
    }
  }
  if (sourceMode.value === 'manual') {
    const family = await resolveManualFamilyInput(manualFamily.value)
    return {
      family,
      sourceType: family ? 'manual' : 'default',
      assetId: null,
    }
  }
  if (sourceMode.value === 'upload' || sourceMode.value === 'url') {
    if (importedDraft.value) {
      if (!cacheAvailable.value) {
        return {
          family: sanitizeFontFamilyName(importedDraft.value.family),
          sourceType: 'manual',
          assetId: null,
        }
      }
      const saved = await saveFontAsset({
        id: createFontAssetId(),
        family: sanitizeFontFamilyName(importedDraft.value.family),
        sourceType: importedDraft.value.sourceType,
        mime: importedDraft.value.mime,
        size: importedDraft.value.size,
        blob: importedDraft.value.blob,
        sourceUrl: importedDraft.value.sourceUrl,
      })
      if (saved.evictedIds.length > 0) {
        message.info(`已自动清理 ${saved.evictedIds.length} 个较旧缓存字体`)
      }
      await refreshCachedAssets()
      selectedCachedAssetId.value = saved.saved.id
      return {
        family: saved.saved.family,
        sourceType: saved.saved.sourceType,
        assetId: saved.saved.id,
      }
    }
    if (selectedCachedAsset.value) {
      await restoreCachedFontById(selectedCachedAsset.value.id)
      return {
        family: selectedCachedAsset.value.family,
        sourceType: selectedCachedAsset.value.sourceType,
        assetId: selectedCachedAsset.value.id,
      }
    }
    message.warning('请先导入字体文件或选择一个已缓存字体')
    return null
  }
  return {
    family: '',
    sourceType: 'default',
    assetId: null,
  }
}

const handleKeep = async () => {
  processing.value = true
  try {
    const payload = await resolveSubmitPayload()
    if (!payload) return
    display.updateSettings({
      fontEnhancedCoverageEnabled: enhancedCoverageEnabled.value,
    })
    display.setGlobalFont({
      family: payload.family,
      sourceType: payload.sourceType,
      assetId: payload.assetId,
    })
    emit('update:show', false)
  } catch (error: any) {
    message.error(error?.message || '字体设置保存失败')
  } finally {
    processing.value = false
  }
}

const handleCancel = () => {
  emit('update:show', false)
}

const handleRestoreDefault = () => {
  display.updateSettings({
    fontEnhancedCoverageEnabled: enhancedCoverageEnabled.value,
  })
  display.setGlobalFont({
    family: '',
    sourceType: 'default',
    assetId: null,
  })
  message.success('已恢复默认字体链')
  emit('update:show', false)
}
</script>

<template>
  <n-modal
    class="font-settings-panel"
    preset="card"
    :show="props.show"
    title="字体设置"
    :style="{ width: 'min(760px, 94vw)' }"
    @update:show="emit('update:show', $event)"
  >
    <div class="font-settings-content">
      <section class="font-settings-section">
        <header>
          <p class="section-title">选择方式</p>
          <p class="section-desc">系统字体读取失败时，可改用手动输入或导入字体</p>
        </header>
        <n-radio-group v-model:value="sourceMode" size="small" class="source-mode-group">
          <n-radio-button value="system">系统字体</n-radio-button>
          <n-radio-button value="manual">手动输入</n-radio-button>
          <n-radio-button value="upload">上传字体</n-radio-button>
          <n-radio-button value="url">URL 导入</n-radio-button>
        </n-radio-group>
      </section>

      <section class="font-settings-section">
        <header>
          <p class="section-title">增强模式（可选）</p>
          <p class="section-desc">默认关闭。开启后使用高优先级 CSS 强制覆盖更多组件文本，可能影响极少数图标字体样式。</p>
        </header>
        <n-switch v-model:value="enhancedCoverageEnabled">
          <template #checked>增强覆盖已开启</template>
          <template #unchecked>增强覆盖已关闭</template>
        </n-switch>
      </section>

      <section v-if="sourceMode === 'system'" class="font-settings-section">
        <header>
          <p class="section-title">系统字体列表</p>
          <p class="section-desc">基于浏览器 API 读取当前设备可用字体</p>
        </header>
        <div class="inline-row">
          <n-button
            secondary
            size="small"
            :disabled="!localFontAvailable || loadingLocalFonts"
            :loading="loadingLocalFonts"
            @click="handleLoadLocalFonts"
          >
            读取本地字体
          </n-button>
          <span v-if="!localFontAvailable" class="tip-text">当前浏览器不支持该 API</span>
        </div>
        <n-select
          v-model:value="selectedLocalFamily"
          filterable
          clearable
          :options="localFontOptions"
          placeholder="读取后选择字体"
        />
      </section>

      <section v-if="sourceMode === 'manual'" class="font-settings-section">
        <header>
          <p class="section-title">手动输入字体名</p>
          <p class="section-desc">输入已安装的字体名称，例如：思源黑体</p>
        </header>
        <n-input v-model:value="manualFamily" placeholder="请输入字体名称" />
      </section>

      <section v-if="sourceMode === 'upload'" class="font-settings-section">
        <header>
          <p class="section-title">上传字体文件</p>
          <p class="section-desc">支持 ttf/otf/woff/woff2，保存后会进入本地缓存（最多 3 个）</p>
        </header>
        <div class="upload-row">
          <n-input v-model:value="uploadFamily" placeholder="可选：自定义字体名称" />
          <n-button secondary size="small" :disabled="processing" @click="triggerFileSelect">选择字体文件</n-button>
        </div>
        <input
          ref="fileInputRef"
          class="native-file-input"
          type="file"
          accept=".ttf,.otf,.woff,.woff2,.ttc,.otc"
          @change="handleFileChange"
        />
      </section>

      <section v-if="sourceMode === 'url'" class="font-settings-section">
        <header>
          <p class="section-title">URL 导入字体</p>
          <p class="section-desc">若跨域失败，请先下载字体后改用“上传字体”</p>
        </header>
        <div class="url-grid">
          <n-input v-model:value="urlValue" placeholder="请输入字体 URL" />
          <n-input v-model:value="urlFamily" placeholder="可选：自定义字体名称" />
          <n-button secondary size="small" :disabled="processing" @click="handleImportFromUrl">导入并预览</n-button>
        </div>
      </section>

      <section class="font-settings-section">
        <header>
          <p class="section-title">已缓存字体文件（{{ cachedAssets.length }}/3）</p>
          <p class="section-desc">仅上传与 URL 导入的字体会出现在这里</p>
        </header>
        <div v-if="cachedAssets.length === 0" class="empty-cache">
          暂无缓存字体
        </div>
        <div v-else class="cache-list">
          <div v-for="asset in cachedAssets" :key="asset.id" class="cache-row">
            <div>
              <p class="cache-name">{{ asset.family }}</p>
              <p class="cache-meta">{{ Math.round(asset.size / 1024) }} KB · {{ asset.sourceType === 'upload' ? '上传' : 'URL' }}</p>
            </div>
            <n-space size="small">
              <n-button size="tiny" secondary @click="handleUseCachedAsset(asset)">使用</n-button>
              <n-button size="tiny" quaternary @click="handleDeleteCachedAsset(asset)">删除</n-button>
            </n-space>
          </div>
        </div>
      </section>

      <section class="font-settings-section">
        <header>
          <p class="section-title">预览</p>
          <p class="section-desc">保持后应用到全局正文字体（代码块等宽字体不受影响）</p>
        </header>
        <div class="preview-box" :style="previewStyle">
          <p class="preview-title">SealChat 全局字体预览</p>
          <p class="preview-body">
            中文测试：阿瓦隆勇者穿越黑森林。<br>
            English Test: The quick brown fox jumps over the lazy dog.<br>
            数字与符号：0123456789 !@#$%^&*()_+[]{}，。；：！？<br>
            当前候选：{{ previewFamily || '系统默认字体链' }}
          </p>
        </div>
      </section>

      <n-space justify="end" align="center" class="font-settings-footer">
        <n-space size="small">
          <n-button tertiary size="small" :disabled="processing" @click="handleRestoreDefault">恢复默认字体</n-button>
          <n-button quaternary size="small" @click="handleCancel">取消</n-button>
          <n-button type="primary" size="small" :loading="processing" @click="handleKeep">保持</n-button>
        </n-space>
      </n-space>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
.font-settings-panel :deep(.n-card) {
  background-color: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-strong);
  color: var(--sc-text-primary);
}

.font-settings-content {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-width: 0;
}

.font-settings-section {
  min-width: 0;
}

.font-settings-section header {
  margin-bottom: 0.5rem;
}

.section-title {
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.section-desc {
  margin-top: 0.15rem;
  font-size: 0.78rem;
  color: var(--sc-text-secondary);
}

.inline-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
  flex-wrap: wrap;
  min-width: 0;
}

.tip-text {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.source-mode-group {
  width: 100%;
  max-width: 100%;
  display: flex;
  flex-wrap: nowrap;
  overflow: hidden;
  padding-bottom: 2px;
}

.source-mode-group :deep(.n-radio-group__splitor) {
  display: none;
}

.source-mode-group :deep(.n-radio-button) {
  flex: 1 1 0;
  min-width: 0;
}

.source-mode-group :deep(.n-radio-button__state-border) {
  border-radius: 8px;
}

.source-mode-group :deep(.n-radio__label) {
  justify-content: center;
  font-size: 0.76rem;
  padding-inline: 0.45rem;
  white-space: nowrap;
}

.upload-row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
  min-width: 0;
}

.url-grid {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 0.75rem;
  min-width: 0;
}

.native-file-input {
  display: none;
}

.empty-cache {
  padding: 0.75rem;
  border: 1px dashed var(--sc-border-mute);
  border-radius: 0.5rem;
  color: var(--sc-text-secondary);
  font-size: 0.78rem;
}

.cache-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.cache-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.6rem 0.7rem;
  border-radius: 0.5rem;
  border: 1px solid var(--sc-border-mute);
  background-color: var(--sc-bg-surface);
  min-width: 0;
}

.cache-name {
  font-size: 0.82rem;
  color: var(--sc-text-primary);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.cache-meta {
  margin-top: 0.1rem;
  font-size: 0.72rem;
  color: var(--sc-text-secondary);
}

.preview-box {
  padding: 0.9rem;
  border-radius: 0.75rem;
  border: 1px solid var(--sc-border-mute);
  background: var(--sc-bg-surface);
  font-family: var(--preview-font-family);
}

.preview-title {
  font-size: 0.95rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
}

.preview-body {
  font-size: 0.86rem;
  line-height: 1.7;
  color: var(--sc-text-secondary);
}

.font-settings-footer {
  margin-top: 0.4rem;
  flex-wrap: wrap;
}

.font-settings-footer :deep(.n-space) {
  flex-wrap: wrap;
}

.font-settings-panel :deep(.n-button) {
  max-width: 100%;
}

.font-settings-panel :deep(.n-button__content) {
  white-space: normal;
  overflow-wrap: anywhere;
}

@media (max-width: 760px) {
  .upload-row {
    grid-template-columns: 1fr;
  }

  .url-grid {
    grid-template-columns: 1fr;
  }
}
</style>
