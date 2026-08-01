<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { cloneDeep } from 'lodash-es'
import { useMessage } from 'naive-ui'
import { fetchAttachmentFileById, resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { uploadCursorAsset, type CursorScope } from '@/services/cursor/cursorApi'
import { normalizeCursorTheme } from '@/services/cursor/cursorRuntime'
import { exportCursorThemePackage, importCursorThemePackage } from '@/services/cursor/cursorTransfer'
import { CURSOR_SLOT_LABELS, CURSOR_SLOTS } from '@/services/cursor/cursorTypes'
import type { CursorMode, CursorSlot, CursorThemeConfig } from '@/services/cursor/cursorTypes'

const props = defineProps<{ show: boolean; modelValue?: CursorThemeConfig | null; scope: CursorScope; worldId?: string }>()
const emit = defineEmits<{ 'update:show': [value: boolean]; 'update:modelValue': [value: CursorThemeConfig] }>()
const message = useMessage()
const draft = ref<CursorThemeConfig>(normalizeCursorTheme(props.modelValue, props.scope))
const uploadingSlot = ref<CursorSlot | null>(null)
const activeUploadSlot = ref<CursorSlot | null>(null)
const uploadInputRef = ref<HTMLInputElement | null>(null)
const importInputRef = ref<HTMLInputElement | null>(null)
const importing = ref(false)
const exporting = ref(false)

watch(() => props.show, (show) => {
  if (show) draft.value = normalizeCursorTheme(cloneDeep(props.modelValue), props.scope)
})

const modeOptions = computed(() => {
  const options: Array<{ label: string; value: CursorMode }> = []
  if (props.scope === 'world') options.push({ label: '继承平台', value: 'inherit' })
  options.push({ label: '浏览器默认', value: 'browser' }, { label: '自定义图片', value: 'custom' })
  return options
})

const assetFor = (slot: CursorSlot) => {
  if (!draft.value.slots[slot]) draft.value.slots[slot] = { mode: props.scope === 'world' ? 'inherit' : 'browser' }
  return draft.value.slots[slot]!
}
const previewUrl = (slot: CursorSlot) => {
  const id = assetFor(slot).attachmentId?.replace(/^id:/, '')
  return id ? resolveAttachmentUrl(`id:${id}`) : ''
}
const previewCursor = (slot: CursorSlot) => {
  const asset = assetFor(slot)
  const url = previewUrl(slot)
  if (!url || asset.mode !== 'custom') return slot
  const hotspotX = Math.max(0, Math.min((asset.width || 128) - 1, asset.hotspotX || 0))
  const hotspotY = Math.max(0, Math.min((asset.height || 128) - 1, asset.hotspotY || 0))
  return `url("${url}") ${hotspotX} ${hotspotY}, ${slot}`
}
const cursorSizeFor = (slot: CursorSlot) => assetFor(slot).size || Math.max(assetFor(slot).width || 0, assetFor(slot).height || 0) || 32
const triggerUpload = (slot: CursorSlot) => {
  activeUploadSlot.value = slot
  uploadInputRef.value?.click()
}
const handleUpload = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const slot = activeUploadSlot.value
  input.value = ''
  if (!file || !slot) return
  uploadingSlot.value = slot
  try {
    const size = assetFor(slot).attachmentId ? cursorSizeFor(slot) : 32
    const uploaded = await uploadCursorAsset(file, props.scope, props.worldId, size)
    draft.value.slots[slot] = { mode: 'custom', attachmentId: uploaded.attachmentId, hotspotX: 0, hotspotY: 0, width: uploaded.width, height: uploaded.height, size, animated: uploaded.animated }
    message.success('鼠标图片已转换为 WebP')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '上传失败')
  } finally {
    uploadingSlot.value = null
    activeUploadSlot.value = null
  }
}
const resizeAsset = async (slot: CursorSlot) => {
  const asset = assetFor(slot)
  if (!asset.attachmentId) return
  const size = cursorSizeFor(slot)
  uploadingSlot.value = slot
  try {
    const sourceFile = await fetchAttachmentFileById(asset.attachmentId, `${slot}.webp`)
    if (!sourceFile) throw new Error('无法读取当前鼠标图片')
    const sourceWidth = Math.max(1, asset.width || size)
    const sourceHeight = Math.max(1, asset.height || size)
    const uploaded = await uploadCursorAsset(
      sourceFile,
      props.scope,
      props.worldId,
      size,
    )
    asset.attachmentId = uploaded.attachmentId
    asset.hotspotX = Math.round((asset.hotspotX || 0) * uploaded.width / sourceWidth)
    asset.hotspotY = Math.round((asset.hotspotY || 0) * uploaded.height / sourceHeight)
    asset.width = uploaded.width
    asset.height = uploaded.height
    asset.size = size
    asset.animated = uploaded.animated
    message.success(`指针已调整为 ${size}px`)
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '调整尺寸失败')
  } finally {
    uploadingSlot.value = null
  }
}
const clearAsset = (slot: CursorSlot) => {
  draft.value.slots[slot] = { mode: props.scope === 'world' ? 'inherit' : 'browser' }
}
const handleModeChange = (slot: CursorSlot, mode: CursorMode) => {
  const current = assetFor(slot)
  draft.value.slots[slot] = mode === 'custom'
    ? { ...current, mode, size: current.attachmentId ? cursorSizeFor(slot) : 32 }
    : { mode }
}
const handleImport = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  importing.value = true
  try {
    draft.value = await importCursorThemePackage(file, props.scope, (assetFile, size) => uploadCursorAsset(assetFile, props.scope, props.worldId, size))
    message.success('鼠标样式已导入；应用后仍需保存设置')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '导入失败')
  } finally {
    importing.value = false
  }
}
const handleExport = async () => {
  exporting.value = true
  try {
    const blob = await exportCursorThemePackage(draft.value, props.scope)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `sealchat-${props.scope}-cursor-theme.zip`
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
    message.success('鼠标样式已导出')
  } catch (error: any) {
    message.error(error?.message || '导出失败')
  } finally {
    exporting.value = false
  }
}
const apply = () => {
  emit('update:modelValue', cloneDeep(draft.value))
  emit('update:show', false)
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="网页鼠标样式"
    class="cursor-theme-modal sc-fluid-modal sc-fluid-modal--xwide"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <input ref="uploadInputRef" type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden @change="handleUpload">
    <input ref="importInputRef" type="file" accept=".zip,application/zip" hidden @change="handleImport">
    <div class="cursor-theme-toolbar">
      <div class="cursor-theme-toolbar__copy">
        <strong>{{ scope === 'world' ? '世界级鼠标样式' : '平台级鼠标样式' }}</strong>
        <span>最多六种。图片保留透明通道并转换 WebP；默认 32px，最长边等比适配。</span>
      </div>
      <n-space class="cursor-theme-toolbar__actions">
        <n-button size="small" :loading="importing" @click="importInputRef?.click()">导入 ZIP</n-button>
        <n-button size="small" :loading="exporting" @click="handleExport">导出 ZIP</n-button>
      </n-space>
    </div>
    <div class="cursor-theme-scroll">
      <div class="cursor-theme-grid">
        <section v-for="slot in CURSOR_SLOTS" :key="slot" class="cursor-theme-card">
          <header class="cursor-theme-card__header">
            <div class="cursor-theme-card__title">
              <strong>{{ CURSOR_SLOT_LABELS[slot] }}</strong>
              <code>{{ slot }}</code>
            </div>
            <n-select :value="assetFor(slot).mode" :options="modeOptions" class="cursor-theme-card__mode" @update:value="handleModeChange(slot, $event)" />
          </header>

          <div v-if="assetFor(slot).mode === 'custom'" class="cursor-theme-card__body">
            <div class="cursor-theme-preview-column">
              <div class="cursor-theme-image-preview">
                <img v-if="previewUrl(slot)" :src="previewUrl(slot)" :alt="slot">
                <span v-else>尚未上传</span>
              </div>
              <span v-if="assetFor(slot).width" class="cursor-theme-card__meta">
                {{ assetFor(slot).width }}×{{ assetFor(slot).height }}{{ assetFor(slot).animated ? ' · 动图' : '' }}
              </span>
            </div>

            <div class="cursor-theme-controls">
              <div class="cursor-theme-controls__buttons">
                <n-button size="small" type="primary" secondary :loading="uploadingSlot === slot" @click="triggerUpload(slot)">
                  {{ assetFor(slot).attachmentId ? '更换图片' : '上传图片' }}
                </n-button>
                <n-button v-if="assetFor(slot).attachmentId" size="small" quaternary @click="clearAsset(slot)">清除</n-button>
              </div>
              <div class="cursor-theme-size">
                <label>实际尺寸</label>
                <n-slider v-model:value="assetFor(slot).size" :min="16" :max="128" :step="1" :disabled="!assetFor(slot).attachmentId" />
                <n-input-number v-model:value="assetFor(slot).size" :min="16" :max="128" size="small" :disabled="!assetFor(slot).attachmentId" />
                <span>px</span>
                <n-button size="small" :disabled="!assetFor(slot).attachmentId || cursorSizeFor(slot) === Math.max(assetFor(slot).width || 0, assetFor(slot).height || 0)" :loading="uploadingSlot === slot" @click="resizeAsset(slot)">
                  应用尺寸
                </n-button>
              </div>
              <div class="cursor-theme-hotspot">
                <label>热点 X</label>
                <n-input-number v-model:value="assetFor(slot).hotspotX" :min="0" :max="Math.max(0, (assetFor(slot).width || 128) - 1)" size="small" />
                <label>Y</label>
                <n-input-number v-model:value="assetFor(slot).hotspotY" :min="0" :max="Math.max(0, (assetFor(slot).height || 128) - 1)" size="small" />
              </div>
              <div class="cursor-theme-live-preview" :style="{ cursor: previewCursor(slot) }">
                移动鼠标到这里预览热点
              </div>
            </div>
          </div>
          <div v-else class="cursor-theme-card__fallback">
            {{ assetFor(slot).mode === 'inherit' ? '进入世界时继承平台配置。' : '使用浏览器原生鼠标样式。' }}
          </div>
        </section>
      </div>
    </div>
    <template #footer>
      <div class="cursor-theme-footer">
        <span>点击“应用”后，还需保存外层平台或世界设置。</span>
        <n-space><n-button @click="emit('update:show', false)">取消</n-button><n-button type="primary" @click="apply">应用</n-button></n-space>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.cursor-theme-toolbar { display: flex; justify-content: space-between; gap: 20px; align-items: center; margin-bottom: 16px; }
.cursor-theme-toolbar__copy { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.cursor-theme-toolbar__copy span, .cursor-theme-footer > span { color: var(--sc-text-secondary); font-size: 13px; }
.cursor-theme-toolbar__actions { flex: none; }
.cursor-theme-scroll { min-width: 0; max-height: min(68vh, 760px); overflow: auto; padding: 2px 6px 2px 2px; }
.cursor-theme-grid { min-width: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.cursor-theme-card { min-width: 0; padding: 16px; border: 1px solid var(--sc-border-muted, rgba(128,128,128,.22)); border-radius: 10px; background: var(--sc-bg-surface, rgba(128,128,128,.04)); }
.cursor-theme-card__header { min-width: 0; display: flex; justify-content: space-between; align-items: center; gap: 14px; }
.cursor-theme-card__title { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.cursor-theme-card__title code, .cursor-theme-card__meta { color: var(--sc-text-secondary); font-size: 12px; }
.cursor-theme-card__mode { width: 160px; flex: none; }
.cursor-theme-card__body { min-width: 0; display: grid; grid-template-columns: 118px minmax(0, 1fr); gap: 16px; margin-top: 14px; }
.cursor-theme-preview-column { min-width: 0; display: flex; flex-direction: column; align-items: center; gap: 6px; }
.cursor-theme-image-preview { width: 104px; height: 104px; display: grid; place-items: center; overflow: hidden; border: 1px solid var(--sc-border-muted, rgba(128,128,128,.3)); border-radius: 8px; color: var(--sc-text-secondary); font-size: 12px; background-color: #fff; background-image: linear-gradient(45deg, #ddd 25%, transparent 25%), linear-gradient(-45deg, #ddd 25%, transparent 25%), linear-gradient(45deg, transparent 75%, #ddd 75%), linear-gradient(-45deg, transparent 75%, #ddd 75%); background-size: 16px 16px; background-position: 0 0, 0 8px, 8px -8px, -8px 0; }
.cursor-theme-image-preview img { display: block; max-width: 96px; max-height: 96px; object-fit: contain; image-rendering: auto; }
.cursor-theme-controls { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
.cursor-theme-controls__buttons, .cursor-theme-hotspot { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.cursor-theme-size { display: grid; grid-template-columns: auto minmax(90px, 1fr) 82px auto auto; align-items: center; gap: 8px; }
.cursor-theme-size label, .cursor-theme-hotspot label, .cursor-theme-size > span { color: var(--sc-text-secondary); font-size: 12px; }
.cursor-theme-size :deep(.n-input-number) { width: 82px; }
.cursor-theme-hotspot :deep(.n-input-number) { width: 80px; }
.cursor-theme-live-preview { min-height: 42px; display: grid; place-items: center; padding: 8px; border: 1px dashed var(--sc-border-muted, rgba(128,128,128,.35)); border-radius: 7px; color: var(--sc-text-secondary); font-size: 12px; user-select: none; }
.cursor-theme-card__fallback { min-height: 88px; display: grid; place-items: center; margin-top: 14px; padding: 14px; border-radius: 8px; background: var(--sc-bg-input, rgba(128,128,128,.06)); color: var(--sc-text-secondary); font-size: 13px; text-align: center; }
.cursor-theme-footer { min-width: 0; display: flex; justify-content: space-between; align-items: center; gap: 16px; }
@media (max-width: 900px) { .cursor-theme-grid { grid-template-columns: 1fr; } }
@media (max-width: 640px) { .cursor-theme-toolbar, .cursor-theme-footer { align-items: stretch; flex-direction: column; } .cursor-theme-toolbar__actions { align-self: flex-start; } .cursor-theme-card__header { align-items: stretch; flex-direction: column; } .cursor-theme-card__mode { width: 100%; } .cursor-theme-card__body { grid-template-columns: 1fr; } .cursor-theme-size { grid-template-columns: auto minmax(80px, 1fr) 76px auto; } .cursor-theme-size .n-button { grid-column: 1 / -1; } }
</style>
