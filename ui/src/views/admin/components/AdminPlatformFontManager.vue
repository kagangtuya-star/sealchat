<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import {
  createAdminPlatformFont,
  deleteAdminPlatformFont,
  listAdminPlatformFonts,
  updateAdminPlatformFont,
  uploadAdminPlatformFontSubsetPackage,
} from '@/services/font/platformFontApi'
import { inferFontFamilyFromFilename, sanitizeFontFamilyName } from '@/services/font/fontUtils'
import {
  getPlatformFontSplitCapability,
  splitPlatformFontFile,
} from '@/services/font/platformFontSplitter'
import type {
  PlatformFontAsset,
  PlatformFontSplitCapability,
} from '@/services/font/platformFontTypes'

const message = useMessage()

const loading = ref(false)
const uploading = ref(false)
const splitting = ref(false)
const probingCapability = ref(false)
const items = ref<PlatformFontAsset[]>([])
const query = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)
const draftDisplayName = ref('')
const draftFamily = ref('')
const draftWeight = ref('400')
const draftStyle = ref<'normal' | 'italic'>('normal')
const draftPreviewText = ref('永字八法')
const selectedFile = ref<File | null>(null)
const splitCapability = ref<PlatformFontSplitCapability>({
  available: false,
  reason: '正在检测分割运行时',
})

const readyCount = computed(() => items.value.filter((item) => item.status === 'ready').length)
const buildPreviewFontFamily = (family?: string) => {
  const normalized = sanitizeFontFamilyName(family || '')
  return normalized ? `"${normalized}", inherit` : undefined
}

const resetDraft = () => {
  selectedFile.value = null
  draftDisplayName.value = ''
  draftFamily.value = ''
  draftWeight.value = '400'
  draftStyle.value = 'normal'
  draftPreviewText.value = '永字八法'
}

const reload = async () => {
  loading.value = true
  try {
    const resp = await listAdminPlatformFonts({
      query: query.value.trim() || undefined,
      includeDisabled: true,
      page: 1,
      pageSize: 200,
    })
    items.value = resp.items || []
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '加载平台字体失败')
  } finally {
    loading.value = false
  }
}

const refreshSplitCapability = async () => {
  probingCapability.value = true
  try {
    splitCapability.value = await getPlatformFontSplitCapability()
  } catch (error: any) {
    splitCapability.value = {
      available: false,
      reason: error?.message || '检测字体分割运行时失败',
    }
  } finally {
    probingCapability.value = false
  }
}

const triggerFileSelect = () => {
  fileInputRef.value?.click()
}

const handleFileChange = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] || null
  input.value = ''
  selectedFile.value = file
  if (!file) return
  if (!draftDisplayName.value.trim()) {
    draftDisplayName.value = file.name.replace(/\.[^/.]+$/u, '')
  }
  if (!draftFamily.value.trim()) {
    draftFamily.value = inferFontFamilyFromFilename(file.name)
  }
}

const buildCreatePayload = () => {
  if (!selectedFile.value) {
    throw new Error('先选择字体文件')
  }
  return {
    file: selectedFile.value,
    displayName: draftDisplayName.value.trim() || selectedFile.value.name,
    family: sanitizeFontFamilyName(draftFamily.value) || inferFontFamilyFromFilename(selectedFile.value.name),
    weight: draftWeight.value.trim() || '400',
    style: draftStyle.value,
    previewText: draftPreviewText.value.trim() || '永字八法',
  }
}

const handleCreate = async () => {
  let payload: ReturnType<typeof buildCreatePayload>
  try {
    payload = buildCreatePayload()
  } catch (error: any) {
    message.warning(error?.message || '先选择字体文件')
    return
  }
  uploading.value = true
  try {
    await createAdminPlatformFont(payload)
    resetDraft()
    message.success('平台字体上传成功')
    await reload()
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '平台字体上传失败')
  } finally {
    uploading.value = false
  }
}

const handleSplitAndPublish = async () => {
  let payload: ReturnType<typeof buildCreatePayload>
  try {
    payload = buildCreatePayload()
  } catch (error: any) {
    message.warning(error?.message || '先选择字体文件')
    return
  }
  if (!splitCapability.value.available) {
    message.warning(splitCapability.value.reason || '当前环境暂不可用字体分割')
    return
  }

  uploading.value = true
  splitting.value = true
  try {
    const created = await createAdminPlatformFont(payload)
    const subsetPackage = await splitPlatformFontFile({
      file: payload.file,
      family: payload.family,
      weight: payload.weight,
      style: payload.style,
    })
    await uploadAdminPlatformFontSubsetPackage(created.id, subsetPackage)
    resetDraft()
    message.success('平台字体已分割并发布')
    await reload()
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '平台字体分割发布失败')
  } finally {
    uploading.value = false
    splitting.value = false
  }
}

const toggleStatus = async (item: PlatformFontAsset) => {
  const nextStatus = item.status === 'disabled' ? 'ready' : 'disabled'
  try {
    const updated = await updateAdminPlatformFont(item.id, { status: nextStatus })
    const idx = items.value.findIndex((row) => row.id === item.id)
    if (idx >= 0) {
      items.value[idx] = updated
    }
    message.success(nextStatus === 'ready' ? '字体已启用' : '字体已停用')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '更新字体状态失败')
  }
}

const handleDelete = async (item: PlatformFontAsset) => {
  try {
    await deleteAdminPlatformFont(item.id)
    items.value = items.value.filter((row) => row.id !== item.id)
    message.success('平台字体已删除')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '删除平台字体失败')
  }
}

onMounted(() => {
  void reload()
  void refreshSplitCapability()
})
</script>

<template>
  <div class="platform-font-manager">
    <input
      ref="fileInputRef"
      type="file"
      accept=".ttf,.otf,.woff,.woff2"
      class="platform-font-manager__hidden-input"
      @change="handleFileChange"
    >

    <div class="platform-font-manager__toolbar">
      <div>
        <p class="platform-font-manager__title">平台字体资源</p>
        <p class="platform-font-manager__desc">
          独立于附件系统存储，供富文本与全局 UI 共享使用。普通访问只按需加载；字体分割仅在管理员主动点击后懒加载执行，并从主程序同目录的 bin/cn-font-split/ 读取运行时。
        </p>
      </div>
      <div class="platform-font-manager__toolbar-actions">
        <n-button secondary size="small" :loading="probingCapability" @click="refreshSplitCapability">检测分割器</n-button>
        <n-button secondary size="small" :loading="loading" @click="reload">刷新</n-button>
      </div>
    </div>

    <div class="platform-font-manager__summary">
      <n-tag type="success" size="small">可用 {{ readyCount }}</n-tag>
      <n-tag size="small">总数 {{ items.length }}</n-tag>
      <n-tag size="small" :type="splitCapability.available ? 'success' : 'warning'">
        分割器 {{ splitCapability.available ? `可用 ${splitCapability.version || ''}`.trim() : '不可用' }}
      </n-tag>
    </div>

    <div class="platform-font-manager__capability" :class="{ 'platform-font-manager__capability--ok': splitCapability.available }">
      <strong>实验性前端分割</strong>
      <span>
        {{ splitCapability.available
          ? `检测到 wasm 运行时${splitCapability.version ? `（${splitCapability.version}）` : ''}，可在当前页面执行分割后上传。`
          : splitCapability.reason || '当前环境未就绪，将继续保留单文件上传路径。' }}
      </span>
    </div>

    <div class="platform-font-manager__upload-card">
      <div class="platform-font-manager__upload-grid">
        <n-input v-model:value="draftDisplayName" placeholder="显示名称，例如 思源黑体平台版" />
        <n-input v-model:value="draftFamily" placeholder="字体族名，例如 Source Han Sans SC" />
        <n-input v-model:value="draftWeight" placeholder="字重，例如 400 / 700" />
        <n-select
          v-model:value="draftStyle"
          :options="[
            { label: '正常 normal', value: 'normal' },
            { label: '斜体 italic', value: 'italic' },
          ]"
        />
        <n-input v-model:value="draftPreviewText" placeholder="预览文案" />
        <div class="platform-font-manager__upload-actions">
          <n-button secondary :disabled="uploading" @click="triggerFileSelect">选择字体文件</n-button>
          <span class="platform-font-manager__file-name">{{ selectedFile?.name || '未选择文件' }}</span>
          <n-button type="primary" :loading="uploading && !splitting" :disabled="!selectedFile" @click="handleCreate">
            上传单文件
          </n-button>
          <n-button
            secondary
            type="warning"
            :loading="splitting"
            :disabled="!selectedFile || !splitCapability.available || uploading"
            @click="handleSplitAndPublish"
          >
            分割并发布
          </n-button>
        </div>
      </div>
    </div>

    <n-input
      v-model:value="query"
      clearable
      placeholder="按显示名、字体族名或源文件名筛选"
      @keydown.enter.prevent="reload"
    />

    <div class="platform-font-manager__list">
      <n-empty v-if="!loading && items.length === 0" description="暂无平台字体" />
      <div v-for="item in items" :key="item.id" class="platform-font-manager__item">
        <div class="platform-font-manager__item-main">
          <div class="platform-font-manager__item-head">
            <strong>{{ item.displayName || item.family }}</strong>
            <div class="platform-font-manager__tags">
              <n-tag size="small" :type="item.status === 'ready' ? 'success' : item.status === 'failed' ? 'error' : item.status === 'disabled' ? 'warning' : 'default'">
                {{ item.status }}
              </n-tag>
              <n-tag size="small">{{ item.deliveryMode || 'single' }}</n-tag>
              <n-tag size="small">{{ item.weight || '400' }}</n-tag>
              <n-tag size="small">{{ item.style || 'normal' }}</n-tag>
            </div>
          </div>
          <p class="platform-font-manager__family">{{ item.family }}</p>
          <p class="platform-font-manager__preview" :style="{ fontFamily: buildPreviewFontFamily(item.family) }">
            {{ item.previewText || '永字八法' }}
          </p>
          <p class="platform-font-manager__meta">
            {{ item.sourceFileName || '未知文件' }} · {{ item.sourceMimeType || '未知类型' }} · {{ item.sourceSize || 0 }} B · 分片数 {{ item.subsetCount || 0 }}
          </p>
          <p v-if="item.lastError" class="platform-font-manager__error">{{ item.lastError }}</p>
        </div>
        <div class="platform-font-manager__item-actions">
          <n-button size="small" secondary @click="toggleStatus(item)">
            {{ item.status === 'disabled' ? '启用' : '停用' }}
          </n-button>
          <n-button size="small" tertiary type="error" @click="handleDelete(item)">删除</n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.platform-font-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.platform-font-manager__hidden-input {
  display: none;
}

.platform-font-manager__toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.platform-font-manager__toolbar-actions {
  display: flex;
  gap: 8px;
}

.platform-font-manager__title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}

.platform-font-manager__desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.platform-font-manager__summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.platform-font-manager__capability {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid color-mix(in srgb, var(--sc-border-strong) 82%, #d97706 18%);
  border-radius: 10px;
  background: color-mix(in srgb, var(--sc-bg-elevated) 88%, #f59e0b 12%);
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.platform-font-manager__capability--ok {
  border-color: color-mix(in srgb, var(--sc-border-strong) 78%, #16a34a 22%);
  background: color-mix(in srgb, var(--sc-bg-elevated) 90%, #16a34a 10%);
}

.platform-font-manager__upload-card {
  padding: 12px;
  border: 1px solid var(--sc-border-strong);
  border-radius: 10px;
  background: var(--sc-bg-elevated);
}

.platform-font-manager__upload-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
}

.platform-font-manager__upload-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  grid-column: 1 / -1;
}

.platform-font-manager__file-name {
  min-width: 180px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.platform-font-manager__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.platform-font-manager__item {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border-radius: 12px;
  border: 1px solid var(--sc-border-strong);
  background: var(--sc-bg-elevated);
}

.platform-font-manager__item-main {
  min-width: 0;
  flex: 1;
}

.platform-font-manager__item-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.platform-font-manager__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.platform-font-manager__family,
.platform-font-manager__meta,
.platform-font-manager__error {
  margin: 6px 0 0;
  font-size: 12px;
}

.platform-font-manager__family {
  color: var(--sc-text-secondary);
}

.platform-font-manager__preview {
  margin: 8px 0 0;
  font-size: 20px;
  line-height: 1.4;
}

.platform-font-manager__meta {
  color: var(--sc-text-secondary);
}

.platform-font-manager__error {
  color: #dc2626;
}

.platform-font-manager__item-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

@media (max-width: 900px) {
  .platform-font-manager__toolbar,
  .platform-font-manager__item {
    flex-direction: column;
  }

  .platform-font-manager__item-actions {
    flex-direction: row;
  }
}
</style>
