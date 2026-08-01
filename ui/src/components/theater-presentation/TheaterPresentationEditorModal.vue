<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ArrowBackUp, ArrowForwardUp, Photo, Plus, Template, X } from '@vicons/tabler'
import { compressImage } from '@/composables/useImageCompressor'
import { useTheaterPresentationEditor } from '@/composables/useTheaterPresentationEditor'
import {
  getTheaterAssetErrorCode,
  uploadTheaterAppearanceAsset,
  waitForTheaterAppearanceAsset,
  type TheaterAppearanceAsset,
} from '@/composables/useTheaterAppearanceAssets'
import {
  MAX_THEATER_PORTRAIT_DECORATIONS,
  theaterPresentationPatchSchema,
  theaterPresentationSchema,
  type TheaterPresentation,
  type TheaterPresentationPatch,
  type WorldTheaterPresentationTemplate,
  type WorldTheaterPresentationTemplateSection,
} from '@/types/theaterPresentation'
import {
  createTheaterPresentationEditorState,
  createTheaterVisualLayer,
  type TheaterEditorCommand,
} from './theaterPresentationEditorState'
import TheaterPresentationInspector from './TheaterPresentationInspector.vue'
import TheaterPresentationPreview from './TheaterPresentationPreview.vue'

const props = withDefaults(defineProps<{
  show: boolean
  mode: 'base' | 'variant'
  presentation?: TheaterPresentation | null
  base?: TheaterPresentation | null
  patch?: TheaterPresentationPatch | null
  channelId: string
  identityId: string
  variantId?: string
  targetUserId?: string
  previewName?: string
  worldTemplate?: WorldTheaterPresentationTemplate | null
  canSetWorldTemplate?: boolean
  worldTemplateSaving?: boolean
}>(), {
  presentation: null,
  base: null,
  patch: null,
  variantId: '',
  targetUserId: '',
  previewName: '角色名',
  worldTemplate: null,
  canSetWorldTemplate: false,
  worldTemplateSaving: false,
})
const emit = defineEmits<{
  'update:show': [show: boolean]
  apply: [value: TheaterPresentation | TheaterPresentationPatch]
  setWorldTemplate: [sections: WorldTheaterPresentationTemplateSection[], presentation: TheaterPresentation]
}>()

const editor = useTheaterPresentationEditor({
  mode: props.mode,
  presentation: props.presentation,
  base: props.base,
  patch: props.patch,
  worldTemplate: props.worldTemplate,
})
const activeTab = ref<'portrait' | 'speaker' | 'content' | 'decorations' | 'dialogue'>('portrait')
const previewEnabled = ref(true)
const templatePopoverVisible = ref(false)
const templateSections = ref<WorldTheaterPresentationTemplateSection[]>(['portrait', 'speaker', 'content', 'dialogue'])
const externalPreview = typeof window !== 'undefined'
  && window.parent !== window
  && new URLSearchParams(window.location.hash.split('?')[1] || '').get('mode') === 'theater'
const fileInput = ref<HTMLInputElement | null>(null)
const uploadPurpose = ref<TheaterAppearanceAsset['purpose']>('portrait')
const uploadAsset = ref<TheaterAppearanceAsset | null>(null)
const uploadErrorCode = ref('')
let uploadGeneration = 0
let previewId = ''
let previewStarted = false

const postPreviewMessage = (type: 'start' | 'update' | 'stop') => {
  if (!externalPreview || !previewId) return
  window.parent.postMessage({
    type: `sealchat.theater.appearance-preview.${type}`,
    previewId,
    draft: type === 'stop' ? undefined : editor.draft.value,
    selection: type === 'stop' ? undefined : editor.selection.value,
    activeSection: type === 'stop' ? undefined : activeTab.value === 'decorations' ? 'decorations' : activeTab.value,
    previewName: props.previewName || '角色名',
    previewText: '夜色正好，我们该出发了。',
  }, window.location.origin)
}

const stopExternalPreview = () => {
  if (!previewStarted) return
  postPreviewMessage('stop')
  previewStarted = false
}

watch(() => props.show, (show) => {
  uploadGeneration += 1
  if (!show) return
  previewId = `appearance-preview-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  previewStarted = false
  editor.state.value = createTheaterPresentationEditorState({
    mode: props.mode,
    presentation: props.presentation,
    base: props.base,
    patch: props.patch,
    worldTemplate: props.worldTemplate,
  })
  activeTab.value = 'portrait'
  previewEnabled.value = true
  uploadAsset.value = null
  uploadErrorCode.value = ''
}, { immediate: true })

watch(() => props.worldTemplate, (template) => {
  editor.state.value = {
    ...editor.state.value,
    worldTemplate: JSON.parse(JSON.stringify(template || {})) as WorldTheaterPresentationTemplate,
  }
}, { deep: true })

watch(
  () => [props.show, previewEnabled.value, editor.revision.value, props.previewName] as const,
  ([show, enabled]) => {
    if (!externalPreview) return
    if (!show || !enabled) {
      stopExternalPreview()
      return
    }
    postPreviewMessage(previewStarted ? 'update' : 'start')
    previewStarted = true
  },
  { flush: 'post' },
)

const handlePreviewCommand = (event: MessageEvent) => {
  if (!externalPreview || event.origin !== window.location.origin || event.source !== window.parent) return
  const data = event.data as Record<string, unknown> | null
  if (!data || data.type !== 'sealchat.theater.appearance-preview.command' || data.previewId !== previewId) return
  if (data.phase === 'start') editor.beginTransaction()
  if (data.phase === 'end') editor.commitTransaction()
  if (data.command && typeof data.command === 'object') {
    editor.dispatch(data.command as TheaterEditorCommand, { transient: data.transient === true })
  }
}

onMounted(() => window.addEventListener('message', handlePreviewCommand))
onBeforeUnmount(() => {
  stopExternalPreview()
  window.removeEventListener('message', handlePreviewCommand)
})

const uploading = computed(() => uploadAsset.value?.status === 'pending' || uploadAsset.value?.status === 'processing')
const canApply = computed(() => !uploading.value)
const canUpload = computed(() => Boolean(props.channelId && props.identityId))

const selectTab = (tab: 'portrait' | 'speaker' | 'content' | 'decorations' | 'dialogue') => {
  activeTab.value = tab
  if (tab === 'portrait') editor.dispatch({ type: 'select', target: { kind: 'portrait' } })
  if (tab === 'speaker') editor.dispatch({ type: 'select', target: { kind: 'speaker' } })
  if (tab === 'content') editor.dispatch({ type: 'select', target: { kind: 'content' } })
  if (tab === 'decorations') {
    const first = editor.draft.value.portraitDecorations[0]
    editor.dispatch({ type: 'select', target: first ? { kind: 'decoration', id: first.id } : { kind: 'portrait' } })
  }
  if (tab === 'dialogue') editor.dispatch({ type: 'select', target: { kind: 'dialogue' } })
}

const triggerUpload = (purpose: TheaterAppearanceAsset['purpose']) => {
  if (!canUpload.value || uploading.value) return
  uploadPurpose.value = purpose
  fileInput.value?.click()
}

const isAnimatedPNG = async (file: File) => {
  let offset = 8
  while (offset + 12 <= file.size) {
    const header = new Uint8Array(await file.slice(offset, offset + 8).arrayBuffer())
    if (header.length < 8) return false
    const length = new DataView(header.buffer, header.byteOffset, header.byteLength).getUint32(0)
    const type = String.fromCharCode(...header.slice(4, 8))
    if (type === 'acTL') return true
    if (type === 'IDAT' || type === 'IEND') return false
    offset += 12 + length
  }
  return false
}

const isAnimatedWebP = async (file: File) => {
  const data = new Uint8Array(await file.slice(0, 21).arrayBuffer())
  if (data.length < 21) return false
  const chunk = String.fromCharCode(...data.slice(12, 16))
  return chunk === 'VP8X' && (data[20] & 0x02) !== 0
}

const appearanceFileType = (file: File) => {
  const declared = file.type.trim().toLowerCase()
  if (declared) return declared
  const extension = file.name.toLowerCase().match(/\.([a-z0-9]+)$/)?.[1]
  if (extension === 'jpg' || extension === 'jpeg') return 'image/jpeg'
  if (extension === 'png') return 'image/png'
  if (extension === 'webp') return 'image/webp'
  if (extension === 'gif') return 'image/gif'
  if (extension === 'webm') return 'video/webm'
  return ''
}

const prepareAppearanceFile = async (file: File) => {
  const mimeType = appearanceFileType(file)
  if (mimeType === 'video/webm' || mimeType === 'image/gif') return file
  if (mimeType !== 'image/png' && mimeType !== 'image/jpeg' && mimeType !== 'image/webp') return file
  if (mimeType === 'image/png' && await isAnimatedPNG(file)) return file
  if (mimeType === 'image/webp' && await isAnimatedWebP(file)) return file
  return compressImage(file, { maxWidth: 1920, maxHeight: 1920, mimeType: 'image/webp' })
}

const applyReadyMedia = (asset: TheaterAppearanceAsset) => {
  if (!asset.media) return
  if (asset.purpose === 'portrait') {
    editor.dispatch({ type: 'set-media', target: { kind: 'portrait' }, media: asset.media })
    return
  }
  if (asset.purpose === 'dialogue-frame') {
    editor.dispatch({ type: 'set-media', target: { kind: 'dialogue-frame' }, media: asset.media })
    editor.dispatch({ type: 'select', target: { kind: 'dialogue' } })
    return
  }
  const layer = createTheaterVisualLayer(asset.media, 'portrait')
  editor.dispatch({ type: 'add-decoration', layer })
}

const handleFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !canUpload.value) return
  uploadErrorCode.value = ''
  try {
    const generation = uploadGeneration
    const preparedFile = await prepareAppearanceFile(file)
    const initial = await uploadTheaterAppearanceAsset({
      channelId: props.channelId,
      identityId: props.identityId,
      variantId: props.variantId || undefined,
      targetUserId: props.targetUserId || undefined,
      purpose: uploadPurpose.value,
      file: preparedFile,
    })
    const ready = await waitForTheaterAppearanceAsset(props.channelId, initial, (asset) => {
      if (generation === uploadGeneration && props.show) uploadAsset.value = asset
    })
    if (generation !== uploadGeneration || !props.show) return
    if (ready.status === 'ready') applyReadyMedia(ready)
    if (ready.status === 'failed') uploadErrorCode.value = ready.failureCode || 'ASSET_PROCESSING_FAILED'
  } catch (error) {
    uploadErrorCode.value = getTheaterAssetErrorCode(error)
    uploadAsset.value = null
  }
}

const dispatch = (command: TheaterEditorCommand, options?: { transient?: boolean }) => editor.dispatch(command, options)
const close = () => {
  stopExternalPreview()
  emit('update:show', false)
}
const apply = () => {
  if (!canApply.value) return
  const result = props.mode === 'variant'
    ? theaterPresentationPatchSchema.parse(editor.result.value)
    : theaterPresentationSchema.parse(editor.result.value)
  emit('apply', result)
  close()
}
const setWorldTemplate = () => {
  if (!templateSections.value.length || props.worldTemplateSaving) return
  emit('setWorldTemplate', [...templateSections.value], theaterPresentationSchema.parse(editor.draft.value))
  templatePopoverVisible.value = false
}
</script>

<template>
  <n-modal :show="show" :mask-closable="false" :auto-focus="false" @update:show="emit('update:show', $event)">
    <div class="theater-editor-modal" :class="{ 'is-external-preview': externalPreview }" data-testid="theater-presentation-editor">
      <header class="theater-editor-modal__header">
        <div>
          <div class="theater-editor-modal__title">小剧场演出外观</div>
          <div class="theater-editor-modal__subtitle">{{ mode === 'variant' ? '差分覆盖' : '频道角色基础外观' }}</div>
        </div>
        <div class="theater-editor-modal__header-actions">
          <n-popover v-if="canSetWorldTemplate" v-model:show="templatePopoverVisible" trigger="click" placement="bottom-end">
            <template #trigger>
              <n-button size="small" secondary :loading="worldTemplateSaving">
                <template #icon><n-icon><Template /></n-icon></template>
                覆盖世界默认
              </n-button>
            </template>
            <div class="theater-editor-modal__template-popover">
              <div class="theater-editor-modal__template-title">选择覆盖部分</div>
              <n-checkbox-group v-model:value="templateSections">
                <n-space vertical size="small">
                  <n-checkbox value="portrait">立绘配置（不含图片）</n-checkbox>
                  <n-checkbox value="speaker">昵称</n-checkbox>
                  <n-checkbox value="content">聊天内容</n-checkbox>
                  <n-checkbox value="dialogue">对话框设定（含图片）</n-checkbox>
                </n-space>
              </n-checkbox-group>
              <n-button type="primary" size="small" :disabled="!templateSections.length" @click="setWorldTemplate">设为世界默认</n-button>
            </div>
          </n-popover>
          <n-tooltip><template #trigger><n-switch v-model:value="previewEnabled" size="small" /></template>编辑预览</n-tooltip>
          <n-tooltip><template #trigger><n-button circle quaternary :disabled="!editor.history.value.past.length" @click="editor.undo"><template #icon><n-icon><ArrowBackUp /></n-icon></template></n-button></template>撤销</n-tooltip>
          <n-tooltip><template #trigger><n-button circle quaternary :disabled="!editor.history.value.future.length" @click="editor.redo"><template #icon><n-icon><ArrowForwardUp /></n-icon></template></n-button></template>重做</n-tooltip>
          <n-tooltip><template #trigger><n-button circle quaternary @click="close"><template #icon><n-icon><X /></n-icon></template></n-button></template>关闭</n-tooltip>
        </div>
      </header>

      <div class="theater-editor-modal__toolbar">
        <n-tabs :value="activeTab" type="segment" size="small" @update:value="selectTab">
          <n-tab name="portrait">立绘</n-tab>
          <n-tab name="speaker">昵称</n-tab>
          <n-tab name="content">聊天内容</n-tab>
          <n-tab name="decorations">立绘装饰</n-tab>
          <n-tab name="dialogue">对话框</n-tab>
        </n-tabs>
      </div>

      <div class="theater-editor-modal__workspace">
        <main class="theater-editor-modal__preview">
          <TheaterPresentationPreview
            :draft="editor.draft.value"
            :selection="editor.selection.value"
            :active-section="activeTab === 'decorations' ? 'decorations' : activeTab"
            :preview-enabled="previewEnabled"
            :preview-name="previewName"
            @dispatch="dispatch"
            @gesture-start="editor.beginTransaction"
            @gesture-end="editor.commitTransaction"
          />
          <div class="theater-editor-modal__asset-row">
            <n-button v-if="activeTab === 'portrait'" size="small" :disabled="!canUpload || uploading" @click="triggerUpload('portrait')"><template #icon><n-icon><Photo /></n-icon></template>上传立绘</n-button>
            <n-button v-if="activeTab === 'decorations'" size="small" :disabled="!canUpload || uploading || editor.draft.value.portraitDecorations.length >= MAX_THEATER_PORTRAIT_DECORATIONS" @click="triggerUpload('portrait-decoration')"><template #icon><n-icon><Plus /></n-icon></template>添加装饰</n-button>
            <n-button v-if="activeTab === 'dialogue'" size="small" :disabled="!canUpload || uploading" @click="triggerUpload('dialogue-frame')"><template #icon><n-icon><Photo /></n-icon></template>上传对话框</n-button>
            <span v-if="!canUpload" class="theater-editor-modal__hint">先保存频道角色，才能上传演出资源</span>
            <span v-else-if="uploading" class="theater-editor-modal__progress">处理中 {{ Math.round((uploadAsset?.progress || 0) * 100) }}%</span>
            <span v-else-if="uploadErrorCode" class="theater-editor-modal__error">{{ uploadErrorCode }}</span>
            <span v-else-if="uploadAsset?.status === 'ready'" class="theater-editor-modal__ready">资源已就绪</span>
          </div>
          <input ref="fileInput" class="theater-editor-modal__file" type="file" accept="image/png,image/jpeg,image/webp,image/gif,video/webm" @change="handleFile">
        </main>

        <aside class="theater-editor-modal__inspector">
          <TheaterPresentationInspector
            :draft="editor.draft.value"
            :selection="editor.selection.value"
            :mode="mode"
            :section-modes="editor.sectionModes.value"
            @dispatch="dispatch"
            @transaction-start="editor.beginTransaction"
            @transaction-end="editor.commitTransaction"
          />
        </aside>
      </div>

      <footer class="theater-editor-modal__footer">
        <n-button @click="close">取消</n-button>
        <n-button type="primary" :disabled="!canApply" @click="apply">应用</n-button>
      </footer>
    </div>
  </n-modal>
</template>

<style scoped>
.theater-editor-modal { width: min(1180px, calc(100vw - 32px)); max-height: calc(100vh - 32px); display: flex; flex-direction: column; overflow: hidden; color: var(--sc-text-primary, #0f172a); background: var(--sc-bg-elevated, #fff); border: 1px solid var(--sc-border-strong, rgba(15,23,42,.15)); border-radius: 6px; box-shadow: 0 18px 50px rgba(0,0,0,.24); }
.theater-editor-modal__header, .theater-editor-modal__toolbar, .theater-editor-modal__footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); }
.theater-editor-modal__title { font-size: 16px; font-weight: 700; }
.theater-editor-modal__subtitle, .theater-editor-modal__hint { color: var(--sc-text-secondary, #64748b); font-size: 12px; }
.theater-editor-modal__header-actions, .theater-editor-modal__asset-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.theater-editor-modal__template-popover { display: flex; min-width: 210px; flex-direction: column; gap: 12px; }
.theater-editor-modal__template-title { font-size: 13px; font-weight: 600; }
.theater-editor-modal__toolbar :deep(.n-tabs) { width: 100%; }
.theater-editor-modal__workspace { display: grid; grid-template-columns: minmax(0, 1fr) 290px; min-height: 0; overflow: hidden; }
.theater-editor-modal.is-external-preview { width: min(460px, calc(100vw - 16px)); height: calc(100dvh - 16px); max-height: calc(100dvh - 16px); }
.theater-editor-modal.is-external-preview .theater-editor-modal__workspace { grid-template-columns: 1fr; overflow-y: auto; }
.theater-editor-modal.is-external-preview .theater-editor-modal__preview { min-height: auto; }
.theater-editor-modal.is-external-preview .theater-editor-modal__preview :deep(.theater-preview-wrap) { display: none; }
.theater-editor-modal.is-external-preview .theater-editor-modal__inspector { border-left: 0; border-top: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); overflow: visible; }
.theater-editor-modal__preview { min-width: 0; min-height: 0; overflow: hidden; background: #15171b; display: grid; grid-template-rows: minmax(0, 1fr) auto; }
.theater-editor-modal__preview :deep(.theater-preview-wrap) { min-height: 0; }
.theater-editor-modal__preview :deep(.theater-preview) { min-height: 0; }
.theater-editor-modal__inspector { min-width: 0; overflow-y: auto; padding: 14px; border-left: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); }
.theater-editor-modal__asset-row { min-height: 48px; padding: 8px 14px; color: rgba(255,255,255,.78); border-top: 1px solid rgba(255,255,255,.08); }
.theater-editor-modal__progress { color: #93c5fd; }.theater-editor-modal__ready { color: #86efac; }.theater-editor-modal__error { color: #fca5a5; font-family: monospace; }
.theater-editor-modal__file { display: none; }
.theater-editor-modal__footer { justify-content: flex-end; border-top: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); border-bottom: 0; }
@media (max-width: 760px) {
  .theater-editor-modal { width: 100vw; height: 100dvh; max-height: 100dvh; border-radius: 0; }
  .theater-editor-modal__header, .theater-editor-modal__toolbar, .theater-editor-modal__footer { padding: 10px 12px; }
  .theater-editor-modal__toolbar { align-items: stretch; flex-direction: column; }
  .theater-editor-modal__toolbar :deep(.n-tabs) { width: 100%; }
  .theater-editor-modal__workspace { grid-template-columns: 1fr; overflow-y: auto; }
  .theater-editor-modal__preview { overflow: visible; }
  .theater-editor-modal__inspector { border-left: 0; border-top: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); overflow: visible; }
}
</style>
