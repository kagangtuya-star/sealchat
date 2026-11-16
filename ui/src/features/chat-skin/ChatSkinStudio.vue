<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useWindowSize } from '@vueuse/core'
import { useDisplayStore } from '@/stores/display'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import {
  CHAT_SKIN_VAR_NAME,
  cloneChatSkin,
  createDefaultChatSkinLayer,
  createEmptyChatSkin,
  createEmptyChatSkinSurface,
  buildChatSkinStudioResult,
  hasChatSkinConfiguration,
  normalizeChatSkin,
  normalizeChatSkinPatch,
  resolveChatSkinWithPatch,
  sanitizeChatSkinCss,
  type ChatSkin,
  type ChatSkinLayout,
  type ChatSkinPatch,
  type ChatSkinSurface,
  type ChatSkinVariantMode,
} from './chatSkinSchema'
import { buildChatSkinRenderWithAssets } from './chatSkinRuntime'
import { BUILTIN_CHAT_SKIN_PRESETS, deriveCompactSurfaceFromBubble, type ChatSkinPreset } from './chatSkinPresets'
import { loadChatSkinLocalPresets, removeChatSkinLocalPreset, saveChatSkinLocalPreset } from './chatSkinLocalPresets'
import {
  buildChatSkinExportFile,
  collectChatSkinDocumentAttachmentIds,
  exportChatSkinDocument,
  exportChatSkinPatch,
  importChatSkin,
  importChatSkinPatch,
  parseChatSkinExportFile,
  pickReferencedChatSkinAssets,
  type ChatSkinAssetPayload,
} from './chatSkinTransfer'
import ChatSkinLayers from './ChatSkinLayers.vue'
import ChatSkinPreview from './editor/ChatSkinPreview.vue'
import ChatSkinStyleEditor from './editor/ChatSkinStyleEditor.vue'
import ChatSkinLayerEditor from './editor/ChatSkinLayerEditor.vue'

// 聊天装扮工作室：与头像装饰编辑器分离。左侧为真实消息组件的实时预览，右侧为分组编辑。
// 基础模式编辑频道角色的完整装扮；差分模式在基础装扮上编辑，保存时只提交与基础不同的部分。
// 本组件不直接写服务端：应用时把结果交给调用方，通过专用接口保存。

export type { ChatSkinVariantMode } from './chatSkinSchema'

export interface ChatSkinStudioApply {
  skin?: ChatSkin | null
  patch?: ChatSkinPatch | null
}

const props = withDefaults(defineProps<{
  show: boolean
  mode: 'base' | 'variant'
  name: string
  color?: string
  avatarSrc?: string
  avatarAttachmentId?: string
  baseSkin: Record<string, any> | null | undefined
  variantPatch?: Record<string, any> | null
  variantLabel?: string
  readonly?: boolean
  readonlyReason?: string
  applying?: boolean
  peerPresets?: ChatSkinPreset[]
  uploadImage: (file: File) => Promise<string>
  exportAssets: (attachmentIds: string[]) => Promise<{ assets: ChatSkinAssetPayload[]; keyByAttachmentId: Map<string, string> }>
  importAssets: (assets: ChatSkinAssetPayload[]) => Promise<Map<string, string>>
}>(), {
  color: '',
  avatarSrc: '',
  avatarAttachmentId: '',
  variantPatch: null,
  variantLabel: '',
  readonly: false,
  readonlyReason: '',
  applying: false,
  peerPresets: () => [],
})

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'apply', value: ChatSkinStudioApply): void
}>()

const message = useMessage()
const dialog = useDialog()
const display = useDisplayStore()
const { width: windowWidth } = useWindowSize()
const isNarrow = computed(() => windowWidth.value < 820)

type Tab = 'style' | 'layers' | 'presets' | 'advanced'
const tab = ref<Tab>('style')
const layout = ref<ChatSkinLayout>('bubble')
const styleTarget = ref<'day' | 'night'>('day')
const selectedLayerId = ref('')
const draft = ref<ChatSkin>(createEmptyChatSkin())
const variantMode = ref<ChatSkinVariantMode>('inherit')
const initialSignature = ref('')
const previewCollapsed = ref(false)
const uploading = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)
const importInputRef = ref<HTMLInputElement | null>(null)
const uploadTarget = ref<'layer' | 'background'>('layer')
const presetName = ref('')
const localPresets = ref<ChatSkinPreset[]>([])

const normalizedBase = computed(() => normalizeChatSkin(props.baseSkin))

const reset = () => {
  layout.value = display.layout === 'compact' ? 'compact' : 'bubble'
  styleTarget.value = display.palette === 'night' ? 'night' : 'day'
  tab.value = 'style'
  selectedLayerId.value = ''
  if (props.mode === 'variant') {
    const patch = normalizeChatSkinPatch(props.variantPatch)
    variantMode.value = patch ? patch.mode : 'inherit'
    draft.value = resolveChatSkinWithPatch(normalizedBase.value, patch)
      || cloneChatSkin(normalizedBase.value)
      || createEmptyChatSkin()
  } else {
    variantMode.value = 'inherit'
    draft.value = cloneChatSkin(normalizedBase.value) || createEmptyChatSkin()
  }
  if (!draft.value[layout.value]) styleTarget.value = 'day'
  initialSignature.value = signature.value
  localPresets.value = loadChatSkinLocalPresets()
}

watch(() => props.show, (visible) => {
  if (visible) reset()
}, { immediate: true })

const editable = computed(() => (
  !props.readonly && (props.mode === 'base' || variantMode.value === 'merge' || variantMode.value === 'replace')
))

const surface = computed<ChatSkinSurface | null>(() => draft.value[layout.value])
const styleForEditor = computed(() => {
  const current = surface.value
  if (!current) return null
  if (styleTarget.value === 'night') {
    if (!current.night) current.night = {}
    return current.night
  }
  return current.style
})

const normalizedDraft = computed(() => normalizeChatSkin(draft.value))
const previewSkin = computed<ChatSkin | null>(() => {
  if (props.mode === 'variant') {
    if (variantMode.value === 'inherit') return normalizedBase.value
    if (variantMode.value === 'none') return null
  }
  return normalizedDraft.value
})

const result = computed<ChatSkinStudioApply>(() => buildChatSkinStudioResult(
  props.mode, variantMode.value, normalizedBase.value, normalizedDraft.value,
))

const signature = computed(() => JSON.stringify(result.value))
const dirty = computed(() => signature.value !== initialSignature.value)
const encodedBytes = computed(() => new TextEncoder().encode(signature.value).length)
const tooLarge = computed(() => encodedBytes.value > 16 * 1024)

const layerCount = computed(() => surface.value?.layers.length || 0)

// ---------------------------------------------------------------------------
// 表面 / 布局
// ---------------------------------------------------------------------------

const createSurface = (source: 'blank' | 'derive' | 'copy') => {
  if (!editable.value) return
  if (source === 'derive') {
    draft.value[layout.value] = deriveCompactSurfaceFromBubble(draft.value.bubble)
  } else if (source === 'copy') {
    const other = layout.value === 'bubble' ? draft.value.compact : draft.value.bubble
    draft.value[layout.value] = other ? JSON.parse(JSON.stringify(other)) : createEmptyChatSkinSurface()
  } else {
    draft.value[layout.value] = createEmptyChatSkinSurface()
  }
}
const removeSurface = () => {
  if (!editable.value) return
  draft.value[layout.value] = null
}
const otherLayoutLabel = computed(() => (layout.value === 'bubble' ? '紧凑' : '气泡'))
const otherSurfaceExists = computed(() => !!(layout.value === 'bubble' ? draft.value.compact : draft.value.bubble))

const setVariantMode = (mode: ChatSkinVariantMode) => {
  if (props.readonly) return
  if (mode === 'replace' && variantMode.value !== 'replace' && !hasChatSkinConfiguration(normalizedDraft.value)) {
    draft.value = cloneChatSkin(normalizedBase.value) || createEmptyChatSkin()
  }
  if (mode === 'merge' && (variantMode.value === 'inherit' || variantMode.value === 'none')) {
    draft.value = cloneChatSkin(normalizedBase.value) || createEmptyChatSkin()
  }
  variantMode.value = mode
}

// ---------------------------------------------------------------------------
// 图层拖动
// ---------------------------------------------------------------------------

let dragOrigin: { id: string; x: number; y: number } | null = null
const findLayer = (id: string) => surface.value?.layers.find(layer => layer.id === id) || null
const handleDragStart = (id: string) => {
  const layer = findLayer(id)
  dragOrigin = layer ? { id, x: layer.x, y: layer.y } : null
}
const clampOffset = (value: number) => Math.max(-240, Math.min(240, Math.round(value)))
const handleDragMove = (id: string, dx: number, dy: number) => {
  if (!dragOrigin || dragOrigin.id !== id) return
  const layer = findLayer(id)
  if (!layer) return
  layer.x = clampOffset(dragOrigin.x + dx)
  layer.y = clampOffset(dragOrigin.y + dy)
}
const handleDragEnd = () => {
  dragOrigin = null
}

// ---------------------------------------------------------------------------
// 上传
// ---------------------------------------------------------------------------

const pickImage = (target: 'layer' | 'background') => {
  if (!editable.value || uploading.value) return
  uploadTarget.value = target
  fileInputRef.value?.click()
}

const readImageAspect = (file: File) => new Promise<{ aspect: number; width: number }>((resolve) => {
  const url = URL.createObjectURL(file)
  const image = new Image()
  image.onload = () => {
    const width = image.naturalWidth || 1
    resolve({ aspect: (image.naturalHeight || width) / width, width })
    URL.revokeObjectURL(url)
  }
  image.onerror = () => {
    resolve({ aspect: 1, width: 64 })
    URL.revokeObjectURL(url)
  }
  image.src = url
})

const handleFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!file.type.startsWith('image/')) {
    message.warning('请选择图片文件')
    return
  }
  const current = surface.value
  if (!current) return
  uploading.value = true
  try {
    const target = uploadTarget.value
    const { aspect, width } = await readImageAspect(file)
    const attachmentId = (await props.uploadImage(file)).replace(/^id:/, '')
    if (!attachmentId) throw new Error('上传失败')
    const ref = `id:${attachmentId}`
    if (target === 'background') {
      const style = styleForEditor.value
      if (!style) return
      if (!style.background) {
        style.background = { mode: 'color', color: '#ffffff', color2: '#f1f5f9', angle: 135, imageRef: ref, imageFit: 'cover', imagePosition: 'center' }
      } else {
        style.background.imageRef = ref
      }
    } else {
      const layer = createDefaultChatSkinLayer(ref, {
        label: file.name.replace(/\.[^.]+$/, '').slice(0, 32),
        size: Math.max(16, Math.min(96, Math.round(width))),
        aspect: Math.round(Math.max(0.05, Math.min(20, aspect)) * 1000) / 1000,
      })
      current.layers.push(layer)
      selectedLayerId.value = layer.id
      tab.value = 'layers'
    }
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '图片上传失败')
  } finally {
    uploading.value = false
  }
}

const resolveThumb = (ref: string) => (ref.startsWith('id:') ? resolveAttachmentUrl(ref) : '')
const backgroundImageUrl = computed(() => {
  const ref = styleForEditor.value?.background?.imageRef || ''
  return ref ? resolveThumb(ref) : ''
})

// ---------------------------------------------------------------------------
// 预设
// ---------------------------------------------------------------------------

const presetGroups = computed(() => [
  { key: 'builtin', label: '内置预设', items: BUILTIN_CHAT_SKIN_PRESETS },
  { key: 'mine', label: '我的预设（仅本机）', items: localPresets.value },
  { key: 'identity', label: '其他角色', items: props.peerPresets },
].filter(group => group.key === 'mine' || group.items.length > 0))

const presetRender = (preset: ChatSkinPreset) => buildChatSkinRenderWithAssets(preset.skin, {
  layout: layout.value,
  tone: 'ic',
  isSelf: false,
  palette: display.palette,
  ignoreScope: true,
})
const presetSurfaceStyle = (preset: ChatSkinPreset) => {
  const render = presetRender(preset)
  if (!render) return undefined
  return layout.value === 'bubble' ? render.contentStyle : { ...render.rootStyle, ...render.contentStyle }
}

const applyPreset = (preset: ChatSkinPreset) => {
  if (props.readonly) return
  const next = cloneChatSkin(preset.skin)
  if (!next) return
  const apply = () => {
    if (props.mode === 'variant' && (variantMode.value === 'inherit' || variantMode.value === 'none')) {
      variantMode.value = 'replace'
    }
    draft.value = next
    selectedLayerId.value = ''
    message.success(`已套用「${preset.name}」，可继续调整`)
  }
  if (hasChatSkinConfiguration(normalizedDraft.value) && editable.value) {
    dialog.warning({
      title: '套用预设',
      content: '将用预设替换当前编辑中的装扮，是否继续？',
      positiveText: '套用',
      negativeText: '取消',
      onPositiveClick: apply,
    })
  } else {
    apply()
  }
}

const saveAsPreset = () => {
  const skin = normalizedDraft.value
  if (!skin || !hasChatSkinConfiguration(skin)) {
    message.warning('当前没有可保存的装扮')
    return
  }
  const next = saveChatSkinLocalPreset(presetName.value || props.name || '我的装扮', skin)
  if (!next) {
    message.error('保存失败：浏览器存储不可用或空间不足')
    return
  }
  localPresets.value = next
  presetName.value = ''
  message.success('已保存到我的预设')
}

const removePreset = (preset: ChatSkinPreset) => {
  localPresets.value = removeChatSkinLocalPreset(preset.id)
}

// ---------------------------------------------------------------------------
// 导入 / 导出 / 复制
// ---------------------------------------------------------------------------

const exporting = ref(false)
const importing = ref(false)

const downloadJson = (filename: string, data: unknown) => {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

const exportFile = async () => {
  const skin = previewSkin.value
  if (!skin) {
    message.warning('当前没有可导出的装扮')
    return
  }
  exporting.value = true
  try {
    const patch = props.mode === 'variant' ? result.value.patch || null : null
    const ids = collectChatSkinDocumentAttachmentIds(skin, patch?.skin)
    const { assets, keyByAttachmentId } = await props.exportAssets(ids)
    const skinDoc = exportChatSkinDocument(skin, keyByAttachmentId)
    const patchDoc = patch ? exportChatSkinPatch(patch, keyByAttachmentId) : null
    const file = buildChatSkinExportFile({
      name: skin.meta.name || props.name,
      skin: skinDoc,
      patch: patchDoc as Record<string, unknown> | null,
      assets: pickReferencedChatSkinAssets(assets, skinDoc, patchDoc),
    })
    const safeName = (props.name || 'chat-skin').replace(/[\\/:*?"<>|\s]+/g, '_').slice(0, 40)
    downloadJson(`${safeName}.chat-skin.json`, file)
    if (ids.length > keyByAttachmentId.size) {
      message.warning('部分图片素材无法导出，对应图层已从文件中省略')
    }
  } catch (error: any) {
    message.error(error?.message || '导出失败')
  } finally {
    exporting.value = false
  }
}

const loadSkinIntoDraft = (skin: ChatSkin | null, label: string) => {
  if (!skin) {
    message.warning('文件中没有可用的装扮')
    return
  }
  if (props.mode === 'variant' && (variantMode.value === 'inherit' || variantMode.value === 'none')) {
    variantMode.value = 'replace'
  }
  draft.value = skin
  selectedLayerId.value = ''
  message.success(label)
}

const importFromText = async (text: string, keepAttachmentRefs: boolean) => {
  const file = parseChatSkinExportFile(text)
  const assetMap = file.assets.length ? await props.importAssets(file.assets) : new Map<string, string>()
  let skin = file.skin ? importChatSkin(file.skin, assetMap, { keepAttachmentRefs }) : null
  if (!skin && file.patch) {
    skin = resolveChatSkinWithPatch(normalizedBase.value, importChatSkinPatch(file.patch, assetMap, { keepAttachmentRefs }))
  }
  return skin
}

const pickImportFile = () => {
  if (props.readonly || importing.value) return
  importInputRef.value?.click()
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  importing.value = true
  try {
    const skin = await importFromText(await file.text(), false)
    loadSkinIntoDraft(skin, '已导入装扮，确认无误后点击应用')
  } catch (error: any) {
    message.error(error?.message || '导入失败')
  } finally {
    importing.value = false
  }
}

const copyJson = async () => {
  const skin = previewSkin.value
  if (!skin) {
    message.warning('当前没有可复制的装扮')
    return
  }
  try {
    await navigator.clipboard.writeText(JSON.stringify(skin))
    message.success('已复制装扮配置（图片以附件引用表示，适合在本站内粘贴）')
  } catch {
    message.error('无法写入剪贴板')
  }
}

const pasteJson = async () => {
  if (props.readonly) return
  try {
    const text = await navigator.clipboard.readText()
    importing.value = true
    const skin = await importFromText(text, true)
    loadSkinIntoDraft(skin, '已粘贴装扮配置')
  } catch (error: any) {
    message.error(error?.message || '无法读取剪贴板中的装扮')
  } finally {
    importing.value = false
  }
}

const moreOptions = computed(() => [
  { label: '导出装扮文件', key: 'export', disabled: !previewSkin.value || exporting.value },
  { label: '导入装扮文件', key: 'import', disabled: props.readonly },
  { label: '复制配置', key: 'copy', disabled: !previewSkin.value },
  { label: '粘贴配置', key: 'paste', disabled: props.readonly },
  { type: 'divider', key: 'd1' },
  { label: '撤销全部修改', key: 'revert', disabled: !dirty.value },
  ...(props.mode === 'base' ? [{ label: '清空装扮', key: 'clear', disabled: props.readonly || !hasChatSkinConfiguration(normalizedDraft.value) }] : []),
])

const handleMore = (key: string) => {
  switch (key) {
    case 'export': void exportFile(); break
    case 'import': pickImportFile(); break
    case 'copy': void copyJson(); break
    case 'paste': void pasteJson(); break
    case 'revert': reset(); break
    case 'clear':
      draft.value = createEmptyChatSkin()
      selectedLayerId.value = ''
      break
  }
}

// ---------------------------------------------------------------------------
// 高级
// ---------------------------------------------------------------------------

const ensureAdvanced = () => {
  const current = surface.value
  if (!current) return null
  if (!current.advanced) current.advanced = { vars: {}, surfaceCss: '', nameCss: '', textCss: '' }
  return current.advanced
}
const advancedVarRows = computed(() => Object.entries(surface.value?.advanced?.vars || {}))
const newVarName = ref('--chat-')
const newVarValue = ref('')
const addVar = () => {
  const name = newVarName.value.trim()
  const value = newVarValue.value.trim()
  if (!CHAT_SKIN_VAR_NAME.test(name)) {
    message.warning('变量名需以 --chat-、--quote-、--message- 或 --sc-skin- 开头')
    return
  }
  if (!value || sanitizeChatSkinCss(`color: ${value}`) === '') {
    message.warning('变量值不被允许（不能包含 url()、花括号等）')
    return
  }
  const advanced = ensureAdvanced()
  if (!advanced) return
  advanced.vars = { ...advanced.vars, [name]: value }
  newVarName.value = '--chat-'
  newVarValue.value = ''
}
const removeVar = (name: string) => {
  const advanced = surface.value?.advanced
  if (!advanced) return
  const next = { ...advanced.vars }
  delete next[name]
  advanced.vars = next
}
const cssFields = [
  { key: 'surfaceCss', label: '表面声明', placeholder: 'letter-spacing: .02em;\noutline: 1px solid #fff4;' },
  { key: 'textCss', label: '正文声明', placeholder: 'line-height: 1.8;' },
  { key: 'nameCss', label: '名字声明', placeholder: 'letter-spacing: .1em;' },
] as const
const cssValue = (key: typeof cssFields[number]['key']) => surface.value?.advanced?.[key] || ''
const setCss = (key: typeof cssFields[number]['key'], value: string) => {
  const advanced = ensureAdvanced()
  if (advanced) advanced[key] = value.slice(0, 600)
}
const cssRejected = (key: typeof cssFields[number]['key']) => {
  const raw = cssValue(key).trim()
  if (!raw) return false
  const kept = sanitizeChatSkinCss(raw)
  const count = (css: string) => css.split(';').map(part => part.trim()).filter(Boolean).length
  return count(kept) < count(raw)
}

// ---------------------------------------------------------------------------
// 关闭 / 应用
// ---------------------------------------------------------------------------

const close = () => {
  if (props.applying) return
  if (!dirty.value) {
    emit('update:show', false)
    return
  }
  dialog.warning({
    title: '放弃修改？',
    content: '聊天装扮的修改尚未应用，关闭后将丢失。',
    positiveText: '放弃',
    negativeText: '继续编辑',
    onPositiveClick: () => emit('update:show', false),
  })
}

const apply = () => {
  if (props.readonly || props.applying || tooLarge.value) return
  emit('apply', JSON.parse(JSON.stringify(result.value)))
}

/** 应用成功后由调用方调用，刷新「未修改」基线。 */
const markSaved = () => {
  initialSignature.value = signature.value
}
defineExpose({ markSaved })

const variantModeOptions: Array<{ value: ChatSkinVariantMode; label: string; hint: string }> = [
  { value: 'inherit', label: '跟随基础', hint: '使用角色的基础装扮' },
  { value: 'merge', label: '局部覆写', hint: '只保存与基础不同的部分，基础改动会继续生效' },
  { value: 'replace', label: '独立装扮', hint: '此差分使用一套完整的独立装扮' },
  { value: 'none', label: '不显示', hint: '此差分发言时不显示聊天装扮' },
]
const variantModeHint = computed(() => variantModeOptions.find(item => item.value === variantMode.value)?.hint || '')

const subtitle = computed(() => (
  props.mode === 'variant'
    ? `差分「${props.variantLabel || '未命名'}」 · 基于角色基础装扮`
    : '频道角色基础装扮 · 仅影响之后发送的消息'
))
</script>

<template>
  <n-modal :show="show" :mask-closable="false" :auto-focus="false" @update:show="$event ? emit('update:show', true) : close()">
    <div class="csk-studio" data-testid="chat-skin-studio">
      <header class="csk-studio__header">
        <div class="csk-studio__heading">
          <div class="csk-studio__title">聊天装扮 · {{ name || '角色' }}</div>
          <div class="csk-studio__subtitle">{{ subtitle }}</div>
        </div>
        <div class="csk-studio__header-actions">
          <n-radio-group v-model:value="layout" size="small">
            <n-radio-button value="bubble">气泡</n-radio-button>
            <n-radio-button value="compact">紧凑</n-radio-button>
          </n-radio-group>
          <n-dropdown trigger="click" :options="moreOptions" @select="handleMore">
            <n-button size="small" :loading="exporting || importing">更多</n-button>
          </n-dropdown>
        </div>
      </header>

      <n-alert v-if="readonly" type="info" :show-icon="false" class="csk-studio__alert">
        {{ readonlyReason || '当前装扮只读。' }}
      </n-alert>

      <div v-if="mode === 'variant'" class="csk-studio__variant">
        <n-radio-group :value="variantMode" size="small" :disabled="readonly" @update:value="setVariantMode">
          <n-radio-button v-for="option in variantModeOptions" :key="option.value" :value="option.value">{{ option.label }}</n-radio-button>
        </n-radio-group>
        <span class="csk-studio__hint">{{ variantModeHint }}</span>
      </div>

      <div class="csk-studio__workspace" :class="{ 'is-narrow': isNarrow }">
        <section class="csk-studio__preview" :class="{ 'is-collapsed': isNarrow && previewCollapsed }">
          <div class="csk-studio__preview-bar">
            <span>实时预览 · 当前主题{{ display.palette === 'night' ? '（夜间）' : '（日间）' }}</span>
            <n-button v-if="isNarrow" size="tiny" quaternary @click="previewCollapsed = !previewCollapsed">
              {{ previewCollapsed ? '展开' : '收起' }}
            </n-button>
          </div>
          <ChatSkinPreview
            v-show="!(isNarrow && previewCollapsed)"
            class="csk-studio__preview-body"
            :skin="previewSkin"
            :layout="layout"
            :name="name || '角色名'"
            :color="color"
            :avatar-src="avatarSrc"
            :avatar-attachment-id="avatarAttachmentId"
            :selected-layer-id="selectedLayerId"
            :draggable="editable && tab === 'layers'"
            @select-layer="selectedLayerId = $event"
            @drag-start="handleDragStart"
            @drag-move="handleDragMove"
            @drag-end="handleDragEnd"
          />
        </section>

        <section class="csk-studio__editor">
          <n-tabs v-model:value="tab" type="line" size="small" class="csk-studio__tabs">
            <n-tab name="style">样式</n-tab>
            <n-tab name="layers">图层{{ layerCount ? ` ${layerCount}` : '' }}</n-tab>
            <n-tab name="presets">预设</n-tab>
            <n-tab name="advanced">高级</n-tab>
          </n-tabs>

          <div class="csk-studio__pane">
            <template v-if="tab === 'presets'">
              <div v-for="group in presetGroups" :key="group.key" class="csk-presets">
                <div class="csk-presets__title">{{ group.label }}</div>
                <div v-if="group.key === 'mine'" class="csk-presets__save">
                  <n-input v-model:value="presetName" size="small" maxlength="40" placeholder="预设名称" />
                  <n-button size="small" secondary :disabled="!hasChatSkinConfiguration(normalizedDraft)" @click="saveAsPreset">保存当前</n-button>
                </div>
                <div v-if="group.items.length" class="csk-presets__grid">
                  <div v-for="preset in group.items" :key="preset.id" class="csk-preset">
                    <button type="button" class="csk-preset__card" :disabled="readonly" @click="applyPreset(preset)">
                      <span class="csk-preset__stage" :class="[`csk-preset__stage--${layout}`, `chat--palette-${display.palette}`]">
                        <span class="csk-preset__surface" :style="presetSurfaceStyle(preset)">
                          <ChatSkinLayers v-if="presetRender(preset)?.layers.length" :layers="presetRender(preset)!.layers" />
                          <span class="csk-preset__name" :style="presetRender(preset)?.nameStyle">{{ preset.name }}</span>
                          <span class="csk-preset__text">示例文字</span>
                        </span>
                      </span>
                      <span class="csk-preset__desc">{{ preset.description || preset.name }}</span>
                    </button>
                    <n-button v-if="preset.source === 'mine'" size="tiny" quaternary class="csk-preset__remove" @click="removePreset(preset)">删除</n-button>
                  </div>
                </div>
                <div v-else class="csk-studio__hint">还没有保存的预设。</div>
              </div>
            </template>

            <template v-else-if="!surface">
              <div class="csk-empty">
                <div class="csk-empty__title">{{ layout === 'bubble' ? '气泡' : '紧凑' }}布局尚未设置装扮</div>
                <div class="csk-studio__hint">未设置时该布局保持主题原样。两种布局分别设计，互不影响。</div>
                <div class="csk-empty__actions">
                  <n-button size="small" type="primary" :disabled="!editable" @click="createSurface('blank')">从空白开始</n-button>
                  <n-button v-if="layout === 'compact' && draft.bubble" size="small" secondary :disabled="!editable" @click="createSurface('derive')">从气泡设计派生</n-button>
                  <n-button v-if="otherSurfaceExists" size="small" secondary :disabled="!editable" @click="createSurface('copy')">复制{{ otherLayoutLabel }}设置</n-button>
                  <n-button size="small" tertiary @click="tab = 'presets'">浏览预设</n-button>
                </div>
              </div>
            </template>

            <template v-else-if="tab === 'style'">
              <div class="csk-studio__target">
                <n-radio-group v-model:value="styleTarget" size="small">
                  <n-radio-button value="day">日间</n-radio-button>
                  <n-radio-button value="night">夜间覆写</n-radio-button>
                </n-radio-group>
                <span v-if="styleTarget === 'night' && display.palette !== 'night'" class="csk-studio__hint">夜间覆写在夜间主题下生效，当前预览为日间主题。</span>
                <span v-else-if="styleTarget === 'night'" class="csk-studio__hint">未开启的分组沿用日间设置。</span>
              </div>
              <ChatSkinStyleEditor
                v-if="styleForEditor"
                :key="`${layout}-${styleTarget}`"
                :style="styleForEditor"
                :layout="layout"
                :night="styleTarget === 'night'"
                :day-style="surface.style"
                :background-image-url="backgroundImageUrl"
                :uploading="uploading && uploadTarget === 'background'"
                :readonly="!editable"
                @pick-background-image="pickImage('background')"
              />
            </template>

            <template v-else-if="tab === 'layers'">
              <ChatSkinLayerEditor
                v-model:selected-id="selectedLayerId"
                :layers="surface.layers"
                :resolve-thumb="resolveThumb"
                :uploading="uploading && uploadTarget === 'layer'"
                :readonly="!editable"
                @upload-layer="pickImage('layer')"
              />
              <div class="csk-studio__hint csk-studio__hint--block">在左侧预览的第一条消息上可直接拖动图层。</div>
            </template>

            <template v-else>
              <div class="csk-adv" :class="{ 'is-readonly': !editable }">
                <div class="csk-adv__row">
                  <div>
                    <div class="csk-adv__title">显示范围</div>
                    <div class="csk-studio__hint">场外消息默认保持主题原样。</div>
                  </div>
                  <n-radio-group v-model:value="draft.scope" size="small">
                    <n-radio-button value="ic">仅场内</n-radio-button>
                    <n-radio-button value="all">场内与场外</n-radio-button>
                  </n-radio-group>
                </div>
                <div class="csk-adv__row">
                  <div>
                    <div class="csk-adv__title">启用此布局装扮</div>
                    <div class="csk-studio__hint">关闭后保留设置，但{{ layout === 'bubble' ? '气泡' : '紧凑' }}布局显示为主题原样。</div>
                  </div>
                  <n-switch v-model:value="surface.enabled" size="small" />
                </div>
                <div v-if="layout === 'bubble'" class="csk-adv__row">
                  <div>
                    <div class="csk-adv__title">自己发言时镜像</div>
                    <div class="csk-studio__hint">气泡在右侧时，左右方向的装饰与色条随之翻转。</div>
                  </div>
                  <n-switch v-model:value="surface.mirrorOnSelf" size="small" />
                </div>
                <div class="csk-adv__row">
                  <div>
                    <div class="csk-adv__title">为突出的装饰预留空间</div>
                    <div class="csk-studio__hint">避免伸出上下边缘的图层压到相邻消息。</div>
                  </div>
                  <n-switch v-model:value="surface.reserveSpace" size="small" />
                </div>
                <div class="csk-adv__block">
                  <div class="csk-adv__title">主题变量</div>
                  <div class="csk-studio__hint">只能覆写 --chat-*、--quote-*、--message-*、--sc-skin-* 变量，作用于本条消息内部。</div>
                  <div v-for="[varName, varValue] in advancedVarRows" :key="varName" class="csk-adv__var">
                    <code>{{ varName }}</code>
                    <span class="csk-adv__var-value">{{ varValue }}</span>
                    <n-button size="tiny" quaternary @click="removeVar(varName)">移除</n-button>
                  </div>
                  <div class="csk-adv__var-new">
                    <n-input v-model:value="newVarName" size="small" placeholder="--chat-..." />
                    <n-input v-model:value="newVarValue" size="small" placeholder="值" @keyup.enter="addVar" />
                    <n-button size="small" secondary @click="addVar">添加</n-button>
                  </div>
                </div>
                <div v-for="field in cssFields" :key="field.key" class="csk-adv__block">
                  <div class="csk-adv__title">{{ field.label }}</div>
                  <n-input
                    type="textarea"
                    size="small"
                    :value="cssValue(field.key)"
                    :placeholder="field.placeholder"
                    :autosize="{ minRows: 2, maxRows: 5 }"
                    maxlength="600"
                    @update:value="setCss(field.key, $event)"
                  />
                  <div v-if="cssRejected(field.key)" class="csk-adv__warn">部分声明不被允许（定位、变换、外部资源等），已在预览与保存时忽略。</div>
                </div>
                <div class="csk-studio__hint">只接受 CSS 声明，不支持选择器、HTML 或脚本；稳定的 DOM 约定见开发文档 doc/chat-skin.md。</div>
                <n-button size="small" tertiary type="error" :disabled="!editable" @click="removeSurface">删除此布局的装扮</n-button>
              </div>
            </template>
          </div>
        </section>
      </div>

      <footer class="csk-studio__footer">
        <span class="csk-studio__hint" :class="{ 'csk-studio__hint--danger': tooLarge }">
          {{ tooLarge ? `装扮配置过大（${(encodedBytes / 1024).toFixed(1)} KB，上限 16 KB）` : (dirty ? '有未应用的修改' : '已是最新') }}
        </span>
        <div class="csk-studio__footer-actions">
          <n-button :disabled="applying" @click="close">{{ dirty ? '取消' : '关闭' }}</n-button>
          <n-button type="primary" :loading="applying" :disabled="readonly || !dirty || tooLarge" @click="apply">应用</n-button>
        </div>
      </footer>

      <input ref="fileInputRef" class="csk-studio__file" type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml" @change="handleFile">
      <input ref="importInputRef" class="csk-studio__file" type="file" accept="application/json,.json" @change="handleImportFile">
    </div>
  </n-modal>
</template>

<style scoped>
.csk-studio { width: min(1120px, calc(100vw - 32px)); height: min(820px, calc(100dvh - 32px)); display: flex; flex-direction: column; overflow: hidden; color: var(--sc-text-primary, #0f172a); background: var(--sc-bg-elevated, #fff); border: 1px solid var(--sc-border-strong, rgba(15, 23, 42, .15)); border-radius: 8px; box-shadow: 0 18px 50px rgba(0, 0, 0, .24); }
.csk-studio__header, .csk-studio__footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px; flex-wrap: wrap; }
.csk-studio__header { border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); }
.csk-studio__footer { border-top: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); }
.csk-studio__heading { min-width: 0; }
.csk-studio__title { font-size: 16px; font-weight: 700; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.csk-studio__subtitle, .csk-studio__hint { font-size: 12px; color: var(--sc-text-secondary, #64748b); }
.csk-studio__hint--block { display: block; margin-top: 8px; }
.csk-studio__hint--danger { color: var(--error-color, #dc2626); }
.csk-studio__header-actions, .csk-studio__footer-actions { display: flex; align-items: center; gap: 8px; }
.csk-studio__alert { margin: 8px 16px 0; }
.csk-studio__variant { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 16px; border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); }
.csk-studio__workspace { flex: 1 1 auto; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) minmax(320px, 400px); overflow: hidden; }
.csk-studio__preview { min-width: 0; min-height: 0; display: flex; flex-direction: column; border-right: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); }
.csk-studio__preview-bar { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 6px 12px; font-size: 12px; color: var(--sc-text-secondary, #64748b); border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .18)); }
.csk-studio__preview-body { flex: 1 1 auto; }
.csk-studio__editor { min-width: 0; min-height: 0; display: flex; flex-direction: column; }
.csk-studio__tabs { padding: 0 12px; flex: none; }
.csk-studio__pane { flex: 1 1 auto; min-height: 0; overflow-y: auto; overflow-x: hidden; padding: 12px; }
.csk-studio__target { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 10px; }
.csk-studio__file { display: none; }
.csk-studio__workspace.is-narrow { grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr); }
.csk-studio__workspace.is-narrow .csk-studio__preview { border-right: 0; border-bottom: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); max-height: 42dvh; }
.csk-empty { display: flex; flex-direction: column; gap: 10px; padding: 24px 8px; text-align: center; align-items: center; }
.csk-empty__title { font-size: 14px; font-weight: 600; }
.csk-empty__actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
.csk-presets { margin-bottom: 14px; }
.csk-presets__title { font-size: 13px; font-weight: 600; margin-bottom: 8px; }
.csk-presets__save { display: flex; gap: 8px; margin-bottom: 8px; }
.csk-presets__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 8px; }
.csk-preset { position: relative; min-width: 0; }
.csk-preset__card { width: 100%; display: flex; flex-direction: column; gap: 6px; padding: 8px; border-radius: 8px; border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .3)); background: none; color: inherit; cursor: pointer; text-align: left; }
.csk-preset__card:hover:not(:disabled) { border-color: var(--primary-color, #6366f1); }
.csk-preset__stage { display: flex; align-items: center; justify-content: center; height: 76px; padding: 14px 10px; overflow: hidden; border-radius: 6px; background: var(--sc-bg-chat, rgba(148, 163, 184, .1)); }
.csk-preset__stage--compact { justify-content: stretch; align-items: center; }
.csk-preset__surface { position: relative; isolation: isolate; display: flex; flex-direction: column; gap: 2px; padding: calc(var(--chat-message-padding-y, 8px) * .8) calc(var(--chat-message-padding-x, 12px) * .95); border-radius: 10px; background: var(--sc-bg-elevated, #fff); font-size: 12px; max-width: 100%; }
.csk-preset__stage--compact .csk-preset__surface { width: 100%; border-radius: 4px; flex-direction: row; gap: 6px; background: transparent; padding: 4px 6px; }
.csk-preset__name { font-weight: 600; font-size: 11px; white-space: nowrap; }
.csk-preset__text { white-space: nowrap; }
.csk-preset__desc { font-size: 11px; color: var(--sc-text-secondary, #64748b); display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.csk-preset__remove { position: absolute; top: 4px; right: 4px; }
.csk-adv { display: flex; flex-direction: column; gap: 12px; }
.csk-adv.is-readonly { pointer-events: none; opacity: .6; }
.csk-adv__row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.csk-adv__block { display: flex; flex-direction: column; gap: 6px; }
.csk-adv__title { font-size: 13px; font-weight: 600; }
.csk-adv__var { display: flex; align-items: center; gap: 8px; font-size: 12px; min-width: 0; }
.csk-adv__var code { flex: none; }
.csk-adv__var-value { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.csk-adv__var-new { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto; gap: 6px; }
.csk-adv__warn { font-size: 12px; color: var(--warning-color, #d97706); }
@media (max-width: 820px) {
  .csk-studio { width: 100vw; height: 100dvh; max-height: 100dvh; border-radius: 0; }
  .csk-studio__header, .csk-studio__footer { padding: 10px 12px; }
  .csk-studio__variant { padding: 8px 12px; }
}
</style>
