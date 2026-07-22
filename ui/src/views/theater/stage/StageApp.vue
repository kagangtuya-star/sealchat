<script setup lang="ts">
import Konva from 'konva'
import { Howl, Howler } from 'howler'
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NButtonGroup, NCheckbox, NColorPicker, NDropdown, NIcon, NInput, NInputNumber, NPopover, NRadio, NRadioGroup, NSelect, NSlider, NSwitch, NTooltip, useDialog, useMessage, type DropdownOption } from 'naive-ui'
import {
  ArrowBackUp,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  Archive,
  Bolt,
  Clipboard,
  Components,
  CloudDownload,
  Copy,
  Cut,
  Edit,
  Eye,
  EyeOff,
  FolderPlus,
  Focus,
  GripVertical,
  LayoutSidebarLeftExpand,
  LetterT,
  Lock,
  LockOpen,
  Message,
  Photo,
  Pencil,
  Plus,
  Select,
  Settings,
  Stars,
  Stack2,
  Trash,
  X,
} from '@vicons/tabler'
import { api, urlBase } from '@/stores/_config'
import { useAudioStudioStore } from '@/stores/audioStudio'
import { compressImage } from '@/composables/useImageCompressor'
import type { AudioAsset, AudioQuotaSummary } from '@/types/audio'
import {
  WORLD_UNIT_PX,
  type StageAction,
  type StageActionTriggeredPayload,
  type StageDrawing,
  type StageDrawingStyle,
  type StageDrawingTool,
  type StageImageRef,
  type StageObject,
  type StageObjectFit,
  type StagePointerTrace,
  type StagePointerTraceInput,
  type StageScene,
  type StageSurfaceFit,
  type StageSurfaceStyle,
  type StageSurfaceTarget,
} from '../shared/stage-types'
import { stageActionSchema, type ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { syncStageObjectHierarchy } from './stage-layering'
import {
  resolveTheaterStageMediaLocation,
  theaterResourceContentPath,
  theaterResourcePath as buildTheaterResourcePath,
  type TheaterStageMediaLocation,
} from './stage-media'
import StageDrawingToolbar, { type StageCanvasTool } from './StageDrawingToolbar.vue'
import StageSceneFixedToolbar from './StageSceneFixedToolbar.vue'
import StageTextEditor, { type StageTextEditorMode } from './StageTextEditor.vue'
import StageTextOverlay from './StageTextOverlay.vue'
import type { TheaterStageStore } from './StageStore'
import TheaterDialogueOverlay from '../dialogue/TheaterDialogueOverlay.vue'
import type { TheaterDialogueRuntime } from '../dialogue/theater-dialogue-runtime'
import type { TheaterEditorCommand, TheaterSection, TheaterSelection } from '@/components/theater-presentation/theaterPresentationEditorState'
import type { TheaterPresentation } from '@/types/theaterPresentation'
import TheaterPresentationPreview from '@/components/theater-presentation/TheaterPresentationPreview.vue'
import TheaterEffectOverlay from '../effects/TheaterEffectOverlay.vue'
import { TheaterEffectRuntime, type TheaterEffectPlayback } from '../effects/theater-effect-runtime'
import { isTheaterEffectObject, setTheaterEffectConfig, theaterEffectConfigFromObject } from '../effects/theater-effect-types'

const props = defineProps<{
  store: TheaterStageStore
  worldId: string
  channelId: string
  scopeType?: 'channel' | 'world'
  characterSnapshot: ChatCharactersSnapshotPayload
  chatBridgeOnline: boolean
  chatVisible: boolean
  syncReady: boolean
  syncing: boolean
  permissions: string[]
  dialogueRuntime: TheaterDialogueRuntime
  appearancePreview: {
    previewId: string
    draft: TheaterPresentation
    selection: TheaterSelection
    activeSection: TheaterSection
    previewName: string
    previewText: string
  } | null
  sceneDialogueEnabled: boolean
}>()
const emit = defineEmits<{
  actionTriggered: [payload: StageActionTriggeredPayload]
  pointerTrace: [trace: StagePointerTraceInput]
  selectCharacter: [identityId: string]
  selectCharacterVariant: [payload: { identityId: string, variantId: string | null }]
  toggleChat: []
  resetLayout: []
  exitTheater: []
  appearancePreviewCommand: [command: TheaterEditorCommand, transient?: boolean]
  appearancePreviewPhase: [phase: 'start' | 'end']
  preloadRequested: [sceneIds: string[]]
  sceneSwitchRequested: [sceneId: string]
  updateSceneDialogueEnabled: [enabled: boolean]
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const viewportRef = ref<HTMLDivElement | null>(null)
const viewportSize = ref({ width: 1, height: 1 })
const imageInputRef = ref<HTMLInputElement | null>(null)
const packageInputRef = ref<HTMLInputElement | null>(null)
const ccfoliaInputRef = ref<HTMLInputElement | null>(null)
const resourceError = ref('')
const resourceUploading = ref(false)
const scenePanelOpen = ref(false)
const inspectorPanelOpen = ref(false)
const layerPanelOpen = ref(false)
const effectPanelOpen = ref(false)
const assetPanelOpen = ref(false)
const effectEditingTarget = ref<'frame' | 'media'>('frame')
const toolbarColorsVisible = ref(false)
const MessageImageEditor = defineAsyncComponent(() => import('@/components/chat/MessageImageEditor.vue'))
const TheaterEffectPanel = defineAsyncComponent(() => import('../effects/TheaterEffectPanel.vue'))
const TheaterAssetManager = defineAsyncComponent(() => import('../effects/TheaterAssetManager.vue'))
const effectPlaybacks = ref<TheaterEffectPlayback[]>([])
const audioStudio = useAudioStudioStore()
const theaterAudioAssets = ref<AudioAsset[]>([])
const theaterAudioQuota = ref<AudioQuotaSummary | null>(null)
const theaterAudioLoading = ref(false)
const theaterAudioUploading = ref(false)
const theaterAudioError = ref('')
const theaterAudioPlayers = new Map<string, Howl>()
const theaterAudioBaseVolumes = new Map<string, number>()
const theaterAudioRetryIds = new Map<string, number>()
const theaterAudioSequences = new Map<string, number>()
const theaterAudioMasterVolumeKey = 'sealchat:theater-audio-volume:v1'
const previousHowlerVolumeValue = Howler.volume()
const previousHowlerVolume = typeof previousHowlerVolumeValue === 'number' ? previousHowlerVolumeValue : 1
Howler.volume(1)
const readTheaterAudioMasterVolume = () => {
  try {
    const stored = window.localStorage.getItem(theaterAudioMasterVolumeKey)
    if (stored === null) return 1
    const value = Number(stored)
    return Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : 1
  } catch {
    return 1
  }
}
const theaterAudioMasterVolume = ref(readTheaterAudioMasterVolume())
let theaterAudioRefreshTimer: number | null = null
const packageMessage = useMessage()
const stageMessage = useMessage()
const packageDialog = useDialog()
const stageDialog = useDialog()
const packageBusy = ref(false)
let packagePollTimer: number | null = null
let packagePollGeneration = 0

type TheaterPackageJob = {
  id: string
  type: 'export' | 'import' | 'import_ccfolia'
  status: 'pending' | 'running' | 'done' | 'failed'
  progress: number
  outputFileName?: string
  errorMessage?: string
  summary?: { scenes?: number, objects?: number, resources?: number, audioAssets?: number, animatedResources?: number, warnings?: string[] }
}

const canManagePackages = computed(() => props.syncReady && props.permissions.includes('stage.admin.restore'))
const packageMenuOptions = computed<DropdownOption[]>(() => [
  { label: packageBusy.value ? '任务处理中…' : '导出小剧场 ZIP', key: 'export', disabled: packageBusy.value },
  { label: '导入小剧场 ZIP', key: 'import', disabled: packageBusy.value },
  { label: '导入 CCFOLIA ZIP', key: 'import-ccfolia', disabled: packageBusy.value },
])

const theaterPackagePath = (suffix: string) => `api/v1/worlds/${encodeURIComponent(props.worldId)}/theater/packages/${suffix}`

const stopPackagePolling = () => {
  packagePollGeneration += 1
  if (packagePollTimer !== null) window.clearTimeout(packagePollTimer)
  packagePollTimer = null
}

const waitPackagePoll = (generation: number) => new Promise<boolean>((resolve) => {
  packagePollTimer = window.setTimeout(() => {
    packagePollTimer = null
    resolve(generation === packagePollGeneration)
  }, 1000)
})

const pollTheaterPackageJob = async (jobId: string) => {
  stopPackagePolling()
  const generation = packagePollGeneration
  while (generation === packagePollGeneration) {
    const response = await api.get<{ job: TheaterPackageJob }>(theaterPackagePath(`jobs/${encodeURIComponent(jobId)}`), { timeout: 30000 })
    const job = response.data.job
    if (job.status === 'done') return job
    if (job.status === 'failed') throw new Error(job.errorMessage || '小剧场任务失败')
    if (!await waitPackagePoll(generation)) throw new Error('小剧场任务已取消')
  }
  throw new Error('小剧场任务已取消')
}

const downloadTheaterPackage = (job: TheaterPackageJob) => {
  const anchor = document.createElement('a')
  anchor.href = `${urlBase}/${theaterPackagePath(`jobs/${encodeURIComponent(job.id)}/download`)}`
  anchor.download = job.outputFileName || 'theater-package.zip'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
}

const exportTheaterPackage = async () => {
  packageBusy.value = true
  try {
    const response = await api.post<{ job: TheaterPackageJob }>(theaterPackagePath('export'), { inputChannelId: props.channelId })
    packageMessage.info('小剧场导出任务已启动')
    const job = await pollTheaterPackageJob(response.data.job.id)
    downloadTheaterPackage(job)
    packageMessage.success('小剧场 ZIP 已生成')
  } catch (error) {
    packageMessage.error(theaterAudioErrorMessage(error, '小剧场导出失败'))
  } finally {
    packageBusy.value = false
  }
}

const importTheaterPackageFile = async (file: File) => {
  packageBusy.value = true
  try {
    const body = new FormData()
    body.append('file', file)
    body.append('inputChannelId', props.channelId)
    const response = await api.post<{ job: TheaterPackageJob }>(theaterPackagePath('import'), body, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
    })
    packageMessage.info('小剧场导入任务已启动')
    const job = await pollTheaterPackageJob(response.data.job.id)
    await fetchTheaterAudioAssets()
    const warnings = job.summary?.warnings?.filter(Boolean) || []
    packageMessage.success(`已追加导入 ${job.summary?.scenes ?? 0} 个场景、${job.summary?.objects ?? 0} 个组件`)
    if (warnings.length) packageMessage.warning(warnings.join('；'))
  } catch (error) {
    packageMessage.error(theaterAudioErrorMessage(error, '小剧场导入失败'))
  } finally {
    packageBusy.value = false
  }
}

const handlePackageInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  packageDialog.warning({
    title: '追加导入小剧场',
    content: `将“${file.name}”作为副本追加到当前世界。现有场景不会被覆盖。`,
    positiveText: '开始导入',
    negativeText: '取消',
    onPositiveClick: () => { void importTheaterPackageFile(file) },
  })
}

const importCCFOLIAPackageFile = async (file: File) => {
  packageBusy.value = true
  try {
    const body = new FormData()
    body.append('file', file)
    body.append('inputChannelId', props.channelId)
    const response = await api.post<{ job: TheaterPackageJob }>(theaterPackagePath('import/ccfolia'), body, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
    })
    packageMessage.info('CCFOLIA 导入任务已启动')
    const job = await pollTheaterPackageJob(response.data.job.id)
    const warnings = job.summary?.warnings?.filter(Boolean) || []
    packageMessage.success(`已导入 ${job.summary?.scenes ?? 0} 个场景、${job.summary?.objects ?? 0} 个组件、${job.summary?.resources ?? 0} 个资源`)
    if (warnings.length) packageMessage.warning(warnings.join('；'))
  } catch (error) {
    packageMessage.error(theaterAudioErrorMessage(error, 'CCFOLIA 导入失败'))
  } finally {
    packageBusy.value = false
  }
}

const handleCCFOLIAInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  packageDialog.warning({
    title: '导入 CCFOLIA 房间',
    content: `将“${file.name}”转换为小剧场场景并追加到当前世界。现有场景不会被覆盖。`,
    positiveText: '开始导入',
    negativeText: '取消',
    onPositiveClick: () => { void importCCFOLIAPackageFile(file) },
  })
}

const handlePackageMenuSelect = (key: string | number) => {
  if (!canManagePackages.value || packageBusy.value) return
  if (key === 'export') void exportTheaterPackage()
  if (key === 'import') packageInputRef.value?.click()
  if (key === 'import-ccfolia') ccfoliaInputRef.value?.click()
}

const unlockTheaterAudio = () => {
  if (!Howler.ctx) void Howler.volume()
  const context = Howler.ctx
  const resume = context?.state === 'suspended'
    ? context.resume().catch(() => undefined)
    : Promise.resolve()
  return resume.then(() => {
    theaterAudioRetryIds.forEach((soundId, key) => {
      const player = theaterAudioPlayers.get(key)
      if (!player) return
      theaterAudioRetryIds.delete(key)
      player.play(soundId)
    })
  })
}

const theaterAudioPath = (assetId = '') => {
  const base = `api/v1/worlds/${encodeURIComponent(props.worldId)}/channels/${encodeURIComponent(props.channelId)}/theater/audio-assets`
  return assetId ? `${base}/${encodeURIComponent(assetId)}` : base
}

const theaterAudioErrorMessage = (error: unknown, fallback: string) => {
  const value = error as { response?: { data?: string | { error?: { message?: string }, message?: string } }, message?: string }
  const data = value?.response?.data
  if (typeof data === 'string' && data.trim()) return data.trim()
  if (data && typeof data === 'object') return data.error?.message || data.message || value?.message || fallback
  return value?.message || fallback
}

const fetchTheaterAudioAssets = async () => {
  if (!props.worldId || !props.channelId) return
  theaterAudioLoading.value = true
  theaterAudioError.value = ''
  try {
    const response = await api.get<{ items?: AudioAsset[], quota?: AudioQuotaSummary }>(theaterAudioPath())
    theaterAudioAssets.value = response.data?.items || []
    theaterAudioQuota.value = response.data?.quota || null
    if (theaterAudioRefreshTimer !== null) window.clearTimeout(theaterAudioRefreshTimer)
    theaterAudioRefreshTimer = theaterAudioAssets.value.some((asset) => asset.transcodeStatus === 'pending')
      ? window.setTimeout(() => { void fetchTheaterAudioAssets() }, 2_000)
      : null
  } catch (error) {
    theaterAudioError.value = theaterAudioErrorMessage(error, '读取频道音频素材失败')
  } finally {
    theaterAudioLoading.value = false
  }
}

const uploadTheaterAudio = async (file: File, targetEffectId = '') => {
  if (!canUploadResources.value) return
  theaterAudioUploading.value = true
  theaterAudioError.value = ''
  try {
    const formData = new FormData()
    formData.append('file', file)
    const response = await api.post<{ item?: AudioAsset }>(theaterAudioPath(), formData, { headers: { 'Content-Type': 'multipart/form-data' } })
    const asset = response.data?.item
    const target = targetEffectId ? props.store.activeObjects.value[targetEffectId] : null
    if (asset && isTheaterEffectObject(target) && canEditAllObjects.value) {
      props.store.beginObjectEdit('上传并绑定特效音效')
      const config = theaterEffectConfigFromObject(target)
      config.audio = { assetId: asset.id, name: asset.name, volume: config.audio?.volume ?? 1 }
      setTheaterEffectConfig(target, config)
      props.store.commitObjectEdit()
    }
    await fetchTheaterAudioAssets()
  } catch (error) {
    theaterAudioError.value = theaterAudioErrorMessage(error, '上传音频素材失败')
  } finally {
    theaterAudioUploading.value = false
  }
}

const stopTheaterAudioPlayer = (key: string) => {
  const player = theaterAudioPlayers.get(key)
  if (!player) return
  player.stop()
  player.unload()
  theaterAudioPlayers.delete(key)
  theaterAudioBaseVolumes.delete(key)
  theaterAudioRetryIds.delete(key)
}

const theaterAudioFormatFromAsset = (asset?: AudioAsset) => {
  const source = asset?.objectKey || asset?.name || ''
  const extension = source.split(/[?#]/, 1)[0].match(/\.([a-z0-9]+)$/i)?.[1]?.toLowerCase()
  if (extension === 'mpeg') return 'mp3'
  if (extension === 'oga') return 'ogg'
  return extension || undefined
}

const theaterAudioFormat = async (assetId: string) => {
  const currentChannelAsset = theaterAudioAssets.value.find((item) => item.id === assetId)
  if (currentChannelAsset) return theaterAudioFormatFromAsset(currentChannelAsset)
  try {
    return theaterAudioFormatFromAsset(await audioStudio.fetchSingleAsset(assetId))
  } catch {
    return undefined
  }
}

const playTheaterAudioAsset = async (assetId: string, volume: number, key: string) => {
  const unlock = unlockTheaterAudio()
  const sequence = (theaterAudioSequences.get(key) || 0) + 1
  theaterAudioSequences.set(key, sequence)
  stopTheaterAudioPlayer(key)
  theaterAudioError.value = ''
  try {
    const src = await audioStudio.fetchPlayableStreamUrl(assetId)
    const format = await theaterAudioFormat(assetId)
    await unlock
    if (theaterAudioSequences.get(key) !== sequence) return
    const baseVolume = Math.max(0, Math.min(1, volume))
    const player = new Howl({
      src: [src],
      format,
      preload: true,
      volume: baseVolume * theaterAudioMasterVolume.value,
      onplay: () => {
        theaterAudioRetryIds.delete(key)
        if (theaterAudioError.value.startsWith('音频播放失败')) theaterAudioError.value = ''
      },
      onend: () => {
        if (theaterAudioPlayers.get(key) === player) stopTheaterAudioPlayer(key)
      },
      onloaderror: (_soundId, error) => {
        if (theaterAudioPlayers.get(key) !== player) return
        theaterAudioError.value = `音频加载失败（${String(error)}）`
        stopTheaterAudioPlayer(key)
      },
      onplayerror: (soundId, error) => {
        if (theaterAudioPlayers.get(key) !== player) return
        theaterAudioError.value = `音频播放失败（${String(error)}），点击页面后将重试`
        theaterAudioRetryIds.set(key, soundId)
      },
    })
    theaterAudioPlayers.set(key, player)
    theaterAudioBaseVolumes.set(key, baseVolume)
    player.play()
  } catch (error) {
    theaterAudioError.value = theaterAudioErrorMessage(error, '音频播放失败')
  }
}

const previewTheaterAudio = (asset: AudioAsset) => playTheaterAudioAsset(asset.id, 1, 'preview')
const deleteTheaterAudio = async (asset: AudioAsset) => {
  if (!canDeleteResources.value || !window.confirm(`删除音频素材“${asset.name}”？`)) return
  theaterAudioError.value = ''
  try {
    await api.delete(theaterAudioPath(asset.id))
    await fetchTheaterAudioAssets()
  } catch (error) {
    theaterAudioError.value = theaterAudioErrorMessage(error, '删除音频素材失败')
  }
}

const effectRuntime = new TheaterEffectRuntime({
  dialogueRuntime: props.dialogueRuntime,
  getObjects: () => Object.values(props.store.activeObjects.value),
  onStart: (playback) => {
    if (playback.config.audio?.assetId) {
      void playTheaterAudioAsset(playback.config.audio.assetId, playback.config.audio.volume, `effect:${playback.effectId}`)
    }
  },
})
const unsubscribeEffectRuntime = effectRuntime.subscribe((playbacks) => { effectPlaybacks.value = playbacks })
const theaterPopoverThemeOverrides = {
  color: 'color-mix(in srgb, var(--sc-bg-surface, #262626) 48%, transparent)',
  boxShadow: '0 14px 34px rgba(0, 0, 0, .2)',
}
const theaterSecondaryMenuProps = () => ({ class: 'theater-secondary-surface' })

const revealToolbarColors = () => { toolbarColorsVisible.value = true }
const hideToolbarColors = () => { toolbarColorsVisible.value = false }
const handleToolbarFocusOut = (event: FocusEvent) => {
  const toolbar = event.currentTarget as HTMLElement | null
  if (event.relatedTarget instanceof Node && toolbar?.contains(event.relatedTarget)) return
  hideToolbarColors()
}

type ImageTarget =
  | { kind: 'scene', target: 'background' | 'foreground' }
  | { kind: 'object', objectId: string }

const surfaceSettingRows: { target: StageSurfaceTarget, label: string }[] = [
  { target: 'background', label: '背景图片' },
  { target: 'foreground', label: '前景图片' },
]
const surfaceFitOptions: { value: StageSurfaceFit, label: string }[] = [
  { value: 'cover', label: '铺满' },
  { value: 'contain', label: '适应' },
  { value: 'fill', label: '拉伸' },
  { value: 'tile', label: '平铺' },
  { value: 'center', label: '居中' },
]
const surfaceStyle = (target: StageSurfaceTarget) => props.store.state.liveState.surfaceStyles[target]
const updateSurfaceFit = (target: StageSurfaceTarget, value: string | number | boolean) => {
  if (typeof value !== 'string' || !surfaceFitOptions.some((option) => option.value === value)) return
  props.store.patchSceneSurfaceStyle(target, { fit: value as StageSurfaceFit })
}
const updateSurfacePercentage = (target: StageSurfaceTarget, key: 'brightness' | 'opacity', value: number) => {
  props.store.patchSceneSurfaceStyle(target, { [key]: value / 100 })
}
const updateSurfaceOverlay = (target: StageSurfaceTarget, patch: Partial<StageSurfaceStyle['overlay']>) => {
  props.store.patchSceneSurfaceStyle(target, { overlay: patch })
}

interface TheaterResourceResponse {
  resource?: TheaterResource
}

interface TheaterResource {
  id?: string
  status?: string
  animated?: boolean
  playbackVariant?: string
  playbackMimeType?: string
  loopCount?: number | null
  processing?: { errorCode?: string }
}

const theaterResourceProcessingError = (code?: string) => {
  switch (code) {
    case 'MEDIA_PROCESSOR_UNAVAILABLE': return '动图处理不可用：服务器未配置 FFmpeg；GIF、Animated WebP、APNG 将尝试使用原文件'
    case 'MEDIA_LIMIT_EXCEEDED': return '动图尺寸、帧数或时长超过服务器限制'
    case 'IMAGE_DECODE_FAILED': return '图片文件损坏或编码不受支持'
    case 'MEDIA_TRANSCODE_FAILED': return '动图转换失败'
    case 'MEDIA_PROBE_FAILED': return '无法读取图片媒体信息'
    default: return code || '图片处理失败'
  }
}

const pendingImageTarget = ref<ImageTarget | null>(null)
const imageEditorTarget = ref<ImageTarget | null>(null)
const imageEditorFile = ref<File | null>(null)
const imageEditorVisible = ref(false)
const activeCanvasTool = ref<StageCanvasTool | null>(null)
const quickDeleteActive = ref(false)
const viewToolActive = ref(false)
const drawingStyle = ref<StageDrawingStyle>({
  stroke: '#f8fafc',
  strokeWidth: 4,
  opacity: 1,
  fill: null,
  dash: 'solid',
})
const drawingSmoothing = ref(0.35)
const drawingPolygonSides = ref(6)
const drawingStyleMemory = new Map<StageDrawingTool, StageDrawingStyle>()
const drawingDashOptions = [
  { label: '实线', value: 'solid' },
  { label: '虚线', value: 'dashed' },
  { label: '点线', value: 'dotted' },
]
const draggedLayerId = ref<string | null>(null)
type LayerDropPlacement = 'before' | 'inside' | 'after'
const layerDropTarget = ref<{ id: string | null, placement: LayerDropPlacement } | null>(null)
const workspaceRef = ref<HTMLDivElement | null>(null)
const hasPermission = (permission: string) => props.syncReady && props.permissions.includes(permission)
const canEditAllObjects = computed(() => hasPermission('stage.object.edit'))
const canEditDelegatedObjects = computed(() => hasPermission('stage.object.edit.delegated'))
const canSwitchScene = computed(() => hasPermission('stage.scene.switch'))
const canTriggerActions = computed(() => hasPermission('stage.action.trigger'))
const canUploadResources = computed(() => hasPermission('stage.resource.upload'))
const canDeleteResources = computed(() => hasPermission('stage.resource.delete'))
const referencedTheaterAudioAssetIds = computed(() => [...new Set(Object.values(props.store.activeObjects.value)
  .filter(isTheaterEffectObject)
  .map((object) => theaterEffectConfigFromObject(object).audio?.assetId)
  .filter((assetId): assetId is string => Boolean(assetId)))])
const isDrawingTool = (tool: StageCanvasTool | null): tool is StageDrawingTool => Boolean(tool && tool !== 'eraser')
const canEditObject = (object: StageObject | null | undefined) => Boolean(object) && (
  canEditAllObjects.value
  || (canEditDelegatedObjects.value && object!.editable && !object!.locked)
)

const confirmDelete = (title: string, content: string, onPositiveClick: () => void) => {
  let destroyDialog = () => {}
  const removeKeydownListener = () => window.removeEventListener('keydown', handleKeydown, true)
  const handleKeydown = (event: KeyboardEvent) => {
    if (event.isComposing || (event.key !== 'Enter' && event.key !== 'Escape')) return
    event.preventDefault()
    event.stopPropagation()
    removeKeydownListener()
    if (event.key === 'Enter') onPositiveClick()
    destroyDialog()
  }
  const dialogReactive = stageDialog.warning({
    title,
    content,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: () => {
      removeKeydownListener()
      onPositiveClick()
    },
    onNegativeClick: removeKeydownListener,
    onEsc: removeKeydownListener,
    onAfterEnter: () => window.addEventListener('keydown', handleKeydown, true),
    onAfterLeave: removeKeydownListener,
  })
  destroyDialog = () => dialogReactive.destroy()
}

const removeObjectsWithConfirm = (objectIds: string[]) => {
  const ids = objectIds.filter((id) => Boolean(props.store.activeObjects.value[id]))
  if (!ids.length || !canEditAllObjects.value) return false
  confirmDelete(
    '删除组件',
    ids.length > 1 ? `确定删除选中的 ${ids.length} 个组件？其子组件也会一并删除。` : '确定删除选中的组件？其子组件也会一并删除。',
    () => {
      props.store.removeObjects(ids)
      nextTick(updateTransformer)
    },
  )
  return true
}

const removeSelectedObjectsWithConfirm = () => removeObjectsWithConfirm([...props.store.selection.selectedIds])

const removeObjectActionWithConfirm = (objectId: string, actionId: string) => {
  confirmDelete('删除点击动作', '确定删除这个点击动作？', () => props.store.removeObjectAction(objectId, actionId))
}

const removeActiveSceneWithConfirm = () => {
  if (!canEditAllObjects.value || !canSwitchScene.value || props.store.scenes.value.length <= 1) return
  const scene = props.store.activeScene.value
  confirmDelete('删除场景', `确定删除场景“${scene.name}”？场景内组件也会一并删除。`, () => props.store.removeScene())
}

const sceneEditMode = ref(false)
const editingSceneId = ref<string | null>(null)
const editingSceneName = ref('')
const editingSceneSwitchText = ref('')

const beginSceneEdit = (scene: StageScene) => {
  if (!canEditAllObjects.value) return
  editingSceneId.value = scene.id
  editingSceneName.value = scene.name
  editingSceneSwitchText.value = scene.switchText
}

const closeSceneEditor = () => {
  editingSceneId.value = null
  editingSceneName.value = ''
  editingSceneSwitchText.value = ''
}

const toggleSceneEditMode = () => {
  sceneEditMode.value = !sceneEditMode.value
  closeSceneEditor()
}

watch(scenePanelOpen, (open) => {
  if (open) return
  sceneEditMode.value = false
  closeSceneEditor()
})

const handleSceneClick = (scene: StageScene) => {
  if (sceneEditMode.value) {
    beginSceneEdit(scene)
    return
  }
  if (canSwitchScene.value) emit('sceneSwitchRequested', scene.id)
}

const saveSceneDetails = () => {
  const sceneId = editingSceneId.value
  const name = editingSceneName.value.trim()
  if (!sceneId) return
  if (!name) {
    stageMessage.warning('场景名称不能为空')
    return
  }
  if (Array.from(name).length > 512) {
    stageMessage.warning('场景名称不能超过 512 个字符')
    return
  }
  if (Array.from(editingSceneSwitchText.value).length > 10_000) {
    stageMessage.warning('场景切换文本不能超过 10000 个字符')
    return
  }
  props.store.updateSceneDetails(sceneId, name, editingSceneSwitchText.value)
  closeSceneEditor()
}

const isEditableShortcutTarget = (target: EventTarget | null) => {
  const element = target instanceof HTMLElement ? target : null
  return Boolean(element?.closest('input, textarea, select, [contenteditable="true"]'))
}

const handleStageShortcut = (event: KeyboardEvent) => {
  if (
    event.isComposing
    || event.altKey
    || isEditableShortcutTarget(event.target)
    || imageEditorVisible.value
  ) return
  const key = event.key.toLowerCase()
  if (key === 'escape' && quickDeleteActive.value) {
    quickDeleteActive.value = false
    nextTick(updateTransformer)
    event.preventDefault()
    return
  }
  if (key === 'escape' && activeCanvasTool.value) {
    if (drawingSession) cancelDrawingSession()
    else activeCanvasTool.value = null
    nextTick(updateTransformer)
    event.preventDefault()
    return
  }
  if (key === 'escape' && props.store.selection.bulkMode) {
    if (props.store.selection.selectedIds.length) props.store.clearSelection()
    else props.store.setBulkSelectionMode(false)
    event.preventDefault()
    return
  }
  if (
    (key === 'delete' || key === 'backspace')
    && !event.ctrlKey
    && !event.metaKey
    && canEditAllObjects.value
  ) {
    if (removeSelectedObjectsWithConfirm()) {
      event.preventDefault()
      return
    }
  }
  if (!(event.ctrlKey || event.metaKey)) return
  let handled = false
  if (key === 'a' && props.store.selection.bulkMode && canEditAllObjects.value) {
    props.store.setSelectedObjectIds(Object.values(props.store.activeObjects.value)
      .filter((object) => object.visible)
      .map((object) => object.id))
    handled = true
  } else if (key === 'c') handled = props.store.copySelectedObject()
  else if (key === 'x' && canEditAllObjects.value) handled = props.store.cutSelectedObject()
  else if (key === 'v' && canEditAllObjects.value) handled = Boolean(props.store.pasteObject())
  else if (key === 'z' && !event.shiftKey && canEditAllObjects.value) handled = props.store.undo()
  if (handled) event.preventDefault()
}

const selectCanvasTool = (tool: StageCanvasTool) => {
  cancelDrawingSession()
  viewToolActive.value = false
  quickDeleteActive.value = false
  const previousTool = activeCanvasTool.value
  if (isDrawingTool(previousTool)) drawingStyleMemory.set(previousTool, { ...drawingStyle.value })
  if (previousTool === tool) {
    activeCanvasTool.value = null
    nextTick(updateTransformer)
    return
  }
  activeCanvasTool.value = tool
  props.store.setBulkSelectionMode(false)
  props.store.clearSelection()
  if (isDrawingTool(tool)) {
    drawingStyle.value = drawingStyleMemory.get(tool) || (tool === 'highlighter'
      ? { stroke: '#facc15', strokeWidth: 18, opacity: 0.32, fill: null, dash: 'solid' }
      : tool === 'pen'
        ? { stroke: '#f8fafc', strokeWidth: 4, opacity: 1, fill: null, dash: 'solid' }
        : { stroke: '#f8fafc', strokeWidth: 3, opacity: 1, fill: null, dash: 'solid' })
  }
  nextTick(updateTransformer)
}

const toggleQuickDeleteTool = () => {
  if (!canEditAllObjects.value) return
  cancelDrawingSession()
  viewToolActive.value = false
  activeCanvasTool.value = null
  quickDeleteActive.value = !quickDeleteActive.value
  props.store.setBulkSelectionMode(false)
  props.store.clearSelection()
  nextTick(updateTransformer)
}

const toggleViewTool = () => {
  cancelDrawingSession()
  finishPointerTrace()
  activeCanvasTool.value = null
  quickDeleteActive.value = false
  viewToolActive.value = !viewToolActive.value
  if (viewToolActive.value) {
    props.store.setBulkSelectionMode(false)
    props.store.clearSelection()
  }
  nextTick(() => {
    syncObjects()
    updateTransformer()
  })
}

const updateDrawingStyle = (style: StageDrawingStyle) => {
  drawingStyle.value = style
  if (isDrawingTool(activeCanvasTool.value)) drawingStyleMemory.set(activeCanvasTool.value, { ...style })
}

type PanelId = 'scene' | 'inspector' | 'layer' | 'effect' | 'asset'
interface PanelLayout {
  x: number
  y: number
  width: number
  height: number
}

const panelLayoutStorageKey = 'sealchat:theater-panel-layout:v1'
const panelTopInset = 58
const panelMinimums: Record<PanelId, { width: number, height: number }> = {
  scene: { width: 140, height: 180 },
  inspector: { width: 240, height: 240 },
  layer: { width: 280, height: 220 },
  effect: { width: 320, height: 320 },
  asset: { width: 320, height: 280 },
}
const readPanelLayouts = (): Partial<Record<PanelId, PanelLayout>> => {
  try {
    const value = JSON.parse(localStorage.getItem(panelLayoutStorageKey) || '{}')
    return value && typeof value === 'object' ? value : {}
  } catch {
    return {}
  }
}
const panelLayouts = ref<Partial<Record<PanelId, PanelLayout>>>(readPanelLayouts())
let panelResizeObserver: ResizeObserver | null = null
let draggingPanel: { id: PanelId, pointerX: number, pointerY: number, x: number, y: number } | null = null

const panelDefaultLayout = (id: PanelId): PanelLayout => {
  const workspace = workspaceRef.value
  const workspaceWidth = workspace?.clientWidth || 960
  const workspaceHeight = workspace?.clientHeight || 640
  const width = id === 'scene' ? 168 : id === 'inspector' ? 280 : id === 'effect' || id === 'asset' ? 340 : 300
  const height = Math.max(panelMinimums[id].height, workspaceHeight - panelTopInset - 12)
  return {
    x: id === 'scene' ? 12 : Math.max(12, workspaceWidth - width - 12),
    y: panelTopInset,
    width,
    height,
  }
}

const clampPanelLayout = (id: PanelId, layout: PanelLayout): PanelLayout => {
  const workspaceWidth = Math.max(1, workspaceRef.value?.clientWidth || 960)
  const workspaceHeight = Math.max(1, workspaceRef.value?.clientHeight || 640)
  const minimum = panelMinimums[id]
  const width = Math.min(workspaceWidth, Math.max(minimum.width, Number(layout.width) || minimum.width))
  const availableHeight = Math.max(1, workspaceHeight - panelTopInset)
  const height = Math.min(availableHeight, Math.max(minimum.height, Number(layout.height) || minimum.height))
  const minimumY = Math.min(panelTopInset, Math.max(0, workspaceHeight - height))
  const maximumY = Math.max(minimumY, workspaceHeight - height)
  return {
    x: Math.min(Math.max(0, Number(layout.x) || 0), Math.max(0, workspaceWidth - width)),
    y: Math.min(Math.max(minimumY, Number(layout.y) || minimumY), maximumY),
    width,
    height,
  }
}

const ensurePanelLayout = (id: PanelId) => {
  const next = clampPanelLayout(id, panelLayouts.value[id] || panelDefaultLayout(id))
  panelLayouts.value = { ...panelLayouts.value, [id]: next }
  return next
}

const persistPanelLayouts = () => {
  try {
    localStorage.setItem(panelLayoutStorageKey, JSON.stringify(panelLayouts.value))
  } catch {
    // Private browsing or storage policy may disable local persistence.
  }
}

const panelStyle = (id: PanelId) => {
  const layout = panelLayouts.value[id]
  if (!layout) return undefined
  return {
    left: `${layout.x}px`,
    top: `${layout.y}px`,
    width: `${layout.width}px`,
    height: `${layout.height}px`,
  }
}

const togglePanel = (id: PanelId) => {
  if (id === 'scene') scenePanelOpen.value = !scenePanelOpen.value
  else if (id === 'inspector') inspectorPanelOpen.value = !inspectorPanelOpen.value
  else if (id === 'layer') layerPanelOpen.value = !layerPanelOpen.value
  else if (id === 'effect') effectPanelOpen.value = !effectPanelOpen.value
  else assetPanelOpen.value = !assetPanelOpen.value
}

const resetWorkspaceLayout = async () => {
  panelLayouts.value = {}
  persistPanelLayouts()
  emit('resetLayout')
  await nextTick()
  const openPanels: [PanelId, boolean][] = [
    ['scene', scenePanelOpen.value],
    ['inspector', inspectorPanelOpen.value],
    ['layer', layerPanelOpen.value],
    ['effect', effectPanelOpen.value],
    ['asset', assetPanelOpen.value],
  ]
  openPanels.forEach(([id, open]) => {
    if (open) ensurePanelLayout(id)
  })
  observeOpenPanels()
}

const startPanelDrag = (id: PanelId, event: PointerEvent) => {
  if (event.button !== 0 || (event.target as HTMLElement).closest('button, input, textarea, select')) return
  const layout = ensurePanelLayout(id)
  const heading = event.currentTarget as HTMLElement
  draggingPanel = { id, pointerX: event.clientX, pointerY: event.clientY, x: layout.x, y: layout.y }
  heading.setPointerCapture(event.pointerId)
  event.preventDefault()
}

const movePanel = (event: PointerEvent) => {
  if (!draggingPanel) return
  const current = panelLayouts.value[draggingPanel.id] || panelDefaultLayout(draggingPanel.id)
  const next = clampPanelLayout(draggingPanel.id, {
    ...current,
    x: draggingPanel.x + event.clientX - draggingPanel.pointerX,
    y: draggingPanel.y + event.clientY - draggingPanel.pointerY,
  })
  panelLayouts.value = { ...panelLayouts.value, [draggingPanel.id]: next }
}

const stopPanelDrag = () => {
  if (!draggingPanel) return
  draggingPanel = null
  persistPanelLayouts()
}

const observeOpenPanels = () => {
  panelResizeObserver?.disconnect()
  workspaceRef.value?.querySelectorAll<HTMLElement>('.theater-floating-panel').forEach((element) => panelResizeObserver?.observe(element))
}

const clampOpenPanels = () => {
  const ids: PanelId[] = ['scene', 'inspector', 'layer', 'effect', 'asset']
  let changed = false
  const next = { ...panelLayouts.value }
  ids.forEach((id) => {
    if (!next[id]) return
    const clamped = clampPanelLayout(id, next[id]!)
    if (JSON.stringify(clamped) !== JSON.stringify(next[id])) {
      next[id] = clamped
      changed = true
    }
  })
  if (changed) {
    panelLayouts.value = next
    persistPanelLayouts()
  }
}

const activeChatCharacter = computed(() => props.characterSnapshot.characters.find((character) => (
  character.identityId === props.characterSnapshot.activeIdentityId
  || character.isActive
)) || null)
const chatCharacterOptions = computed(() => props.characterSnapshot.characters.map((character) => ({
  value: character.identityId,
  label: character.resolvedAppearance.displayName || character.displayName || character.identityId,
})))
const chatCharacterVariantOptions = computed(() => {
  const character = activeChatCharacter.value
  if (!character) return []
  return [
    { value: '', label: '基础外观' },
    ...character.variants
      .filter((variant) => variant.enabled)
      .map((variant) => ({
        value: variant.variantId,
        label: variant.keyword || variant.appearancePatch.displayName || variant.variantId,
      })),
  ]
})

const handleChatCharacterSelect = (identityId: string) => {
  if (identityId && identityId !== props.characterSnapshot.activeIdentityId) {
    emit('selectCharacter', identityId)
  }
}

const handleChatCharacterVariantSelect = (variantId: string | null) => {
  const character = activeChatCharacter.value
  if (!character) return
  emit('selectCharacterVariant', {
    identityId: character.identityId,
    variantId: variantId || null,
  })
}

const actionId = () => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `action-${value}`
}

let stage: Konva.Stage | null = null
let backgroundLayer: Konva.Layer | null = null
let worldLayer: Konva.Layer | null = null
let foregroundLayer: Konva.Layer | null = null
let interactionLayer: Konva.Layer | null = null
let backgroundCameraGroup: Konva.Group | null = null
let worldCameraGroup: Konva.Group | null = null
let foregroundCameraGroup: Konva.Group | null = null
let gridGroup: Konva.Group | null = null
let objectRoot: Konva.Group | null = null
let sceneMorphRoot: Konva.Group | null = null
let drawingDraftRoot: Konva.Group | null = null
let pointerTraceRoot: Konva.Group | null = null
let transformer: Konva.Transformer | null = null
let selectionRect: Konva.Rect | null = null
let quickDeleteOutline: Konva.Rect | null = null
let resizeObserver: ResizeObserver | null = null
let panning = false
let marqueeStart: { x: number, y: number } | null = null
let marqueeAdditive = false
let panPointer = { x: 0, y: 0 }
let panOrigin = { x: 0, y: 0 }
let gridSignature = ''

interface DrawingSession {
  tool: StageDrawingTool
  start: { x: number, y: number }
  current: { x: number, y: number }
  points: number[]
  shiftKey: boolean
  altKey: boolean
}

let drawingSession: DrawingSession | null = null

interface PointerTraceVisual {
  group: Konva.Group
  line: Konva.Line
  expiryTimer: number | null
}

interface PointerTraceSession {
  traceId: string
  identityId: string
  variantId: string | null
  pendingPoints: number[]
  lastPoint: { x: number, y: number }
  lastSentAt: number
}

const pointerTraceVisuals = new Map<string, PointerTraceVisual>()
const localPointerTraceIds = new Set<string>()
let pointerTraceSession: PointerTraceSession | null = null

const clearPointerTrace = (traceId: string) => {
  const visual = pointerTraceVisuals.get(traceId)
  if (!visual) return
  if (visual.expiryTimer !== null) window.clearTimeout(visual.expiryTimer)
  visual.group.destroy()
  pointerTraceVisuals.delete(traceId)
  localPointerTraceIds.delete(traceId)
  worldLayer?.batchDraw()
}

const keepPointerTrace = (traceId: string) => {
  const visual = pointerTraceVisuals.get(traceId)
  if (!visual) return
  if (visual.expiryTimer !== null) window.clearTimeout(visual.expiryTimer)
  visual.expiryTimer = window.setTimeout(() => clearPointerTrace(traceId), 5_000)
}

const appendPointerTraceVisual = (trace: StagePointerTrace) => {
  if (!pointerTraceRoot) return
  let visual = pointerTraceVisuals.get(trace.traceId)
  if (!visual) {
    const group = new Konva.Group({ listening: false })
    const line = new Konva.Line({
      points: trace.points,
      stroke: trace.color,
      strokeWidth: 5,
      opacity: 0.9,
      lineCap: 'round',
      lineJoin: 'round',
      listening: false,
    })
    const label = new Konva.Text({
      x: trace.points[0] + 8,
      y: trace.points[1] - 24,
      text: trace.displayName,
      fill: trace.color,
      fontSize: 14,
      fontStyle: 'bold',
      shadowColor: '#000000',
      shadowBlur: 3,
      shadowOpacity: 0.9,
      listening: false,
    })
    group.add(line, label)
    pointerTraceRoot.add(group)
    visual = { group, line, expiryTimer: null }
    pointerTraceVisuals.set(trace.traceId, visual)
  } else {
    visual.line.points([...visual.line.points(), ...trace.points])
  }
  keepPointerTrace(trace.traceId)
  worldLayer?.batchDraw()
}

const appendPointerTrace = (trace: StagePointerTrace) => {
  const local = localPointerTraceIds.has(trace.traceId)
  if (local) {
    keepPointerTrace(trace.traceId)
    return
  }
  appendPointerTraceVisual(trace)
}

const beginPointerTrace = (pointer: { x: number, y: number }) => {
  const character = activeChatCharacter.value
  if (!character) return
  const traceId = `pointer-${typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`}`
  pointerTraceSession = {
    traceId,
    identityId: character.identityId,
    variantId: character.activeVariantId || null,
    pendingPoints: [pointer.x, pointer.y],
    lastPoint: pointer,
    lastSentAt: 0,
  }
  localPointerTraceIds.add(traceId)
  appendPointerTraceVisual({
    traceId,
    displayName: character.resolvedAppearance.displayName || character.displayName || character.identityId,
    color: character.resolvedAppearance.color || character.color || '#38bdf8',
    points: [pointer.x, pointer.y],
    finished: false,
  })
}

const flushPointerTrace = (finished: boolean) => {
  const session = pointerTraceSession
  if (!session) return
  const points = session.pendingPoints.splice(0)
  if (!points.length) points.push(session.lastPoint.x, session.lastPoint.y)
  emit('pointerTrace', {
    traceId: session.traceId,
    identityId: session.identityId,
    variantId: session.variantId,
    points,
    finished,
  })
  session.lastSentAt = Date.now()
}

const continuePointerTrace = (pointer: { x: number, y: number }) => {
  const session = pointerTraceSession
  if (!session || Math.hypot(pointer.x - session.lastPoint.x, pointer.y - session.lastPoint.y) < 2) return
  session.lastPoint = pointer
  session.pendingPoints.push(pointer.x, pointer.y)
  appendPointerTraceVisual({
    traceId: session.traceId,
    displayName: '',
    color: '',
    points: [pointer.x, pointer.y],
    finished: false,
  })
  if (Date.now() - session.lastSentAt >= 50) flushPointerTrace(false)
}

const finishPointerTrace = () => {
  if (!pointerTraceSession) return
  flushPointerTrace(true)
  pointerTraceSession = null
}

const objectNodes = new Map<string, Konva.Group>()
const imageLoadVersions = new Map<string, number>()
type StageMediaSource = HTMLImageElement | HTMLVideoElement
const activeAnimatedMedia = new Set<StageMediaSource>()
const videoLoopStates = new WeakMap<HTMLVideoElement, { loopCount: number | null, completed: number }>()
let mediaAnimation: Konva.Animation | null = null
let multiDrag: {
  driverId: string
  driverStart: { x: number, y: number }
  nodes: Map<string, { node: Konva.Group, absolute: { x: number, y: number } }>
} | null = null

interface SurfaceSlot {
  group: Konva.Group
  base: Konva.Rect | null
  media: Konva.Shape
  directImage: Konva.Image
  overlay: Konva.Rect
  placeholder: Konva.Rect
  label: Konva.Text
  style: StageSurfaceStyle
  url: string
  version: number
  source: StageMediaSource | null
  ready: boolean
  debugDrawCount: number
}

let backgroundSlot: SurfaceSlot | null = null
let foregroundSlot: SurfaceSlot | null = null

const selectedObject = computed(() => {
  const id = props.store.state.selectedObjectId
  const object = id ? props.store.activeObjects.value[id] || null : null
  return isTheaterEffectObject(object) ? null : object
})
const selectedEffectObject = computed(() => {
  const id = props.store.state.selectedObjectId
  const object = id ? props.store.activeObjects.value[id] || null : null
  return isTheaterEffectObject(object) ? object : null
})
const beginEffectTransform = () => {
  if (!selectedEffectObject.value || !canEditAllObjects.value) return
  props.store.beginObjectEdit('变换特效')
}
const updateEffectTransform = (transform: StageObject['transform']) => {
  if (!selectedEffectObject.value || !canEditAllObjects.value) return
  selectedEffectObject.value.transform = transform
}
const endEffectTransform = () => props.store.commitObjectEdit()
const beginEffectMediaTransform = () => {
  if (!selectedEffectObject.value || !canEditAllObjects.value) return
  props.store.beginObjectEdit('移动特效媒体')
}
const updateEffectMediaTransform = (patch: { x: number, y: number }) => {
  const object = selectedEffectObject.value
  if (!object || !canEditAllObjects.value) return
  const config = theaterEffectConfigFromObject(object)
  config.builtin.mediaTransform.x = patch.x
  config.builtin.mediaTransform.y = patch.y
  setTheaterEffectConfig(object, config)
}
const endEffectMediaTransform = () => props.store.commitObjectEdit()
const stageObjects = props.store.activeObjects
const selectedObjects = props.store.selectedObjects
const selectedIdSet = computed(() => new Set(props.store.selection.selectedIds))
const isBatchSelection = computed(() => props.store.selection.bulkMode && selectedObjects.value.length > 1)
const batchMoveBlocked = computed(() => isBatchSelection.value && selectedObjects.value.some((object) => object.locked))

const toggleSelectedDrawingFill = (checked: boolean) => {
  const drawing = selectedObject.value?.drawing
  if (!drawing) return
  drawing.style.fill = checked ? drawing.style.fill || drawing.style.stroke : null
}

const selectedTextMode = computed<StageTextEditorMode>(() => (
  selectedObject.value?.metadata?.textEditorMode === 'rich' ? 'rich' : 'plain'
))

const updateSelectedText = (value: string) => {
  const object = selectedObject.value
  if (!object || object.type !== 'text') return
  object.text = value
}

const updateSelectedTextMode = (mode: StageTextEditorMode) => {
  const object = selectedObject.value
  if (!object || object.type !== 'text') return
  object.metadata = { ...object.metadata, textEditorMode: mode }
}

type BatchBooleanKey = 'visible' | 'interactive' | 'editable' | 'locked' | 'aspectRatioLocked'
const batchBooleanObjects = (_key: BatchBooleanKey) => selectedObjects.value
const batchBooleanChecked = (key: BatchBooleanKey) => batchBooleanObjects(key).length > 0
  && batchBooleanObjects(key).every((object) => object[key])
const batchBooleanIndeterminate = (key: BatchBooleanKey) => {
  const objects = batchBooleanObjects(key)
  const enabled = objects.filter((object) => object[key]).length
  return enabled > 0 && enabled < objects.length
}
const updateBatchBoolean = (key: BatchBooleanKey, checked: boolean) => {
  props.store.patchSelectedObjects({ [key]: checked })
  if (key === 'aspectRatioLocked') {
    nextTick(() => {
      syncObjects()
      updateTransformer()
    })
  }
}

const updateSelectedAspectRatioLocked = (checked: boolean) => {
  const object = selectedObject.value
  if (!object || object.aspectRatioLocked === checked) return
  props.store.beginObjectEdit('修改对象比例锁定')
  object.aspectRatioLocked = checked
  props.store.commitObjectEdit()
  nextTick(() => {
    syncObjects()
    updateTransformer()
  })
}

const updateSelectedDimension = (dimension: 'width' | 'height', value: number | null) => {
  const object = selectedObject.value
  if (!object || object.type === 'group' || value === null || !Number.isFinite(value)) return
  const nextValue = Math.max(0.5, value)
  const width = Math.max(0.5, object.transform.width)
  const height = Math.max(0.5, object.transform.height)
  const aspectRatio = width / height
  object.transform[dimension] = nextValue
  if (!object.aspectRatioLocked || !Number.isFinite(aspectRatio) || aspectRatio <= 0) return
  if (dimension === 'width') {
    object.transform.height = Number(Math.max(0.5, nextValue / aspectRatio).toFixed(6))
  } else {
    object.transform.width = Number(Math.max(0.5, nextValue * aspectRatio).toFixed(6))
  }
}

const updateSelectedLoopCount = (value: number | null) => {
  const image = selectedObject.value?.image
  if (!image?.animated) return
  if (value === null) {
    delete image.loopCount
    return
  }
  image.loopCount = Math.min(65_535, Math.max(1, Math.round(value)))
}

const updateSelectedScale = (dimension: 'scaleX' | 'scaleY', value: number | null) => {
  const object = selectedObject.value
  if (!object || object.type !== 'group' || value === null || !Number.isFinite(value)) return
  const nextValue = Math.min(100, Math.max(0.01, value))
  const scaleX = Math.max(0.01, object.transform.scaleX)
  const scaleY = Math.max(0.01, object.transform.scaleY)
  const aspectRatio = scaleX / scaleY
  object.transform[dimension] = nextValue
  if (!object.aspectRatioLocked || !Number.isFinite(aspectRatio) || aspectRatio <= 0) return
  if (dimension === 'scaleX') {
    object.transform.scaleY = Number(Math.min(100, Math.max(0.01, nextValue / aspectRatio)).toFixed(6))
  } else {
    object.transform.scaleX = Number(Math.min(100, Math.max(0.01, nextValue * aspectRatio)).toFixed(6))
  }
}

const rootObjectIds = (objectIds: string[]) => {
  const selected = new Set(objectIds)
  return objectIds.filter((id) => {
    let parentId = getObject(id)?.parentId || null
    while (parentId) {
      if (selected.has(parentId)) return false
      parentId = getObject(parentId)?.parentId || null
    }
    return true
  })
}

const selectedMovementRootIds = () => rootObjectIds(props.store.selection.selectedIds)

const parentOptions = computed(() => Object.values(props.store.activeObjects.value)
  .filter((object) => object.type === 'group'
    && object.id !== selectedObject.value?.id
    && (!selectedObject.value
      || props.store.isSceneFixedObject(object.id) === props.store.isSceneFixedObject(selectedObject.value.id)))
  .map((object) => ({ label: object.name, value: object.id })))

interface LayerRow {
  object: StageObject
  depth: number
}

const layerRows = computed<LayerRow[]>(() => {
  const objects = Object.values(props.store.activeObjects.value).filter((object) => !isTheaterEffectObject(object))
  const rows: LayerRow[] = []
  const append = (parentId: string | null, depth: number) => {
    objects
      .filter((object) => object.parentId === parentId)
      .sort((a, b) => b.transform.z - a.transform.z || b.transform.order - a.transform.order)
      .forEach((object) => {
        rows.push({ object, depth })
        append(object.id, depth + 1)
      })
  }
  append(null, 0)
  return rows
})

const layerPreviewUrls = ref<Record<string, string>>({})

const setLayerPreviewUrl = (objectId: string, url: string) => {
  if (layerPreviewUrls.value[objectId] === url) return
  layerPreviewUrls.value = { ...layerPreviewUrls.value, [objectId]: url }
}

const clearLayerPreviewUrl = (objectId: string) => {
  if (!layerPreviewUrls.value[objectId]) return
  const next = { ...layerPreviewUrls.value }
  delete next[objectId]
  layerPreviewUrls.value = next
}

const layerPreviewUrl = (object: StageObject) => {
  if (object.type !== 'image' || !object.image || object.image.mimeType?.startsWith('video/')) return null
  const cached = layerPreviewUrls.value[object.id]
  if (cached) return cached
  const location = resolveTheaterStageMedia(object.image)
  return location && !location.managed ? location.url : null
}

const layerPreviewIcon = (object: StageObject) => {
  if (object.type === 'group') return Components
  if (object.type === 'drawing') return Pencil
  if (object.type === 'text') return LetterT
  if (object.type === 'button') return Bolt
  return Photo
}

const toggleLayerObjectFlag = (object: StageObject, key: 'visible' | 'editable' | 'locked') => {
  if (!canEditAllObjects.value) return
  props.store.setObjectFlag(object.id, key, !object[key])
}

const getObject = (objectId: string) => props.store.activeObjects.value[objectId]

const objectIsDescendantOf = (objectId: string, ancestorId: string) => {
  let parentId = getObject(objectId)?.parentId || null
  while (parentId) {
    if (parentId === ancestorId) return true
    parentId = getObject(parentId)?.parentId || null
  }
  return false
}

interface MarqueeBounds {
  x: number
  y: number
  width: number
  height: number
}

const marqueeContains = (outer: MarqueeBounds, inner: MarqueeBounds) => {
  const epsilon = 0.01
  return inner.width > 0
    && inner.height > 0
    && inner.x >= outer.x - epsilon
    && inner.y >= outer.y - epsilon
    && inner.x + inner.width <= outer.x + outer.width + epsilon
    && inner.y + inner.height <= outer.y + outer.height + epsilon
}

const marqueeObjectBounds = (object: StageObject, node: Konva.Group, relativeTo: Konva.Stage) => {
  const target = object.type === 'group'
    ? node.findOne<Konva.Rect>('.theater-object-group-control-bounds')
    : node
  if (!target?.isVisible()) return null
  return target.getClientRect({
    relativeTo,
    skipShadow: true,
    skipStroke: object.type === 'group',
  })
}

const canvasSelectionTarget = (objectId: string) => {
  let target = getObject(objectId)
  let outerGroupId: string | null = null
  while (target?.parentId) {
    const parent = getObject(target.parentId)
    if (!parent || parent.type !== 'group') break
    outerGroupId = parent.id
    target = parent
  }
  if (!outerGroupId) return objectId
  const selectedId = props.store.state.selectedObjectId
  if (
    selectedId === objectId
    || selectedId === outerGroupId
    || (selectedId && objectIsDescendantOf(selectedId, outerGroupId))
  ) return objectId
  return outerGroupId
}

const addAction = (type: StageAction['type']) => {
  const object = selectedObject.value
  if (!object || !canEditAllObjects.value) return
  const action: StageAction = type === 'chat.send'
    ? { id: actionId(), type, payload: { content: '舞台消息' } }
    : type === 'chat.insert'
      ? { id: actionId(), type, payload: { content: '舞台台词' } }
      : type === 'scene.apply'
        ? { id: actionId(), type, payload: { sceneId: props.store.state.activeSceneId } }
        : { id: actionId(), type, payload: { objectId: object.id } }
  props.store.addObjectAction(object.id, action)
}

const triggerObjectActions = (object: StageObject) => {
  if (!canTriggerActions.value || !['image', 'button'].includes(object.type) || !object.interactive || !object.visible) return
  const pointer = worldCameraGroup?.getRelativePointerPosition()
  object.actions.forEach((action) => {
    const parsed = stageActionSchema.safeParse(action)
    if (!parsed.success) return
    emit('actionTriggered', {
      objectId: object.id,
      actionId: parsed.data.id,
      action: parsed.data,
      ...(pointer ? {
        pointer: {
          x: Number((pointer.x / WORLD_UNIT_PX).toFixed(6)),
          y: Number((pointer.y / WORLD_UNIT_PX).toFixed(6)),
        },
      } : {}),
    })
  })
}

const applyCamera = () => {
  if (!stage) return
  const position = {
    x: stage.width() / 2 + props.store.state.camera.x,
    y: stage.height() / 2 + props.store.state.camera.y,
  }
  const scale = { x: props.store.state.camera.zoom, y: props.store.state.camera.zoom }
  // Background fills viewport independently; world and foreground follow camera.
  backgroundCameraGroup?.position({ x: 0, y: 0 })
  backgroundCameraGroup?.scale({ x: 1, y: 1 })
  for (const group of [worldCameraGroup, foregroundCameraGroup]) {
    group?.position(position)
    group?.scale(scale)
  }
  backgroundLayer?.batchDraw()
  worldLayer?.batchDraw()
  foregroundLayer?.batchDraw()
  interactionLayer?.batchDraw()
}

const updateTransformer = () => {
  if (!transformer) return
  if (viewToolActive.value || activeCanvasTool.value || quickDeleteActive.value) {
    transformer.nodes([])
    transformer.visible(false)
    interactionLayer?.batchDraw()
    return
  }
  if (isBatchSelection.value) {
    const nodes = selectedMovementRootIds()
      .map((id) => objectNodes.get(id))
      .filter((node): node is Konva.Group => Boolean(node))
    transformer.nodes(nodes)
    transformer.padding(0)
    transformer.borderStrokeWidth(1)
    transformer.borderDash([])
    transformer.anchorSize(9)
    transformer.rotateAnchorOffset(50)
    transformer.keepRatio(false)
    transformer.enabledAnchors([])
    transformer.rotateEnabled(false)
    transformer.forceUpdate()
    interactionLayer?.batchDraw()
    return
  }
  const object = selectedObject.value
  const node = object && canEditObject(object) && !object.locked ? objectNodes.get(object.id) : null
  transformer.nodes(node ? [node] : [])
  transformer.visible(Boolean(node))
  const groupSelected = object?.type === 'group'
  const proportional = object?.aspectRatioLocked !== false
  transformer.padding(groupSelected ? 8 : 0)
  transformer.borderStrokeWidth(groupSelected ? 2 : 1)
  transformer.borderDash(groupSelected ? [6, 4] : [])
  transformer.anchorSize(groupSelected ? 11 : 9)
  transformer.rotateAnchorOffset(groupSelected ? 32 : 50)
  transformer.keepRatio(proportional)
  transformer.enabledAnchors(object?.locked ? [] : proportional
    ? ['top-left', 'top-right', 'bottom-left', 'bottom-right']
    : [
        'top-left', 'top-center', 'top-right',
        'middle-left', 'middle-right',
        'bottom-left', 'bottom-center', 'bottom-right',
      ])
  transformer.rotateEnabled(!object?.locked)
  transformer.forceUpdate()
  interactionLayer?.batchDraw()
}

const selectObject = (objectId: string | null, additive = false) => {
  if (viewToolActive.value) return
  if (objectId && !canEditObject(getObject(objectId))) return
  props.store.selectObject(objectId, additive)
  nextTick(updateTransformer)
}

const openObjectInspector = (objectId: string) => {
  if (!canEditObject(getObject(objectId))) return
  const keepBatchSelection = props.store.selection.bulkMode && selectedIdSet.value.has(objectId)
  if (!keepBatchSelection) selectObject(objectId)
  inspectorPanelOpen.value = true
}

const toggleBulkSelectionMode = () => {
  if (!canEditAllObjects.value) return
  cancelDrawingSession()
  activeCanvasTool.value = null
  quickDeleteActive.value = false
  props.store.setBulkSelectionMode(!props.store.selection.bulkMode)
  nextTick(() => {
    syncObjects()
    updateTransformer()
  })
}

const isVideoSource = (source: StageMediaSource): source is HTMLVideoElement => source instanceof HTMLVideoElement

const theaterMediaScope = () => ({
  urlBase: String(urlBase),
  worldId: props.worldId,
  channelId: props.channelId,
  scopeType: props.scopeType,
})

const resolveTheaterStageMedia = (imageRef: StageImageRef) => resolveTheaterStageMediaLocation(imageRef, theaterMediaScope())

const stageMediaDimensions = (source: StageMediaSource) => isVideoSource(source)
  ? { width: source.videoWidth, height: source.videoHeight }
  : { width: source.naturalWidth || source.width, height: source.naturalHeight || source.height }

const syncMediaAnimation = () => {
  if (!activeAnimatedMedia.size) {
    mediaAnimation?.stop()
    return
  }
  if (!mediaAnimation && backgroundLayer && worldLayer && foregroundLayer) {
    mediaAnimation = new Konva.Animation(() => {}, [backgroundLayer, worldLayer, foregroundLayer])
  }
  mediaAnimation?.start()
}

const stageMediaRequestControllers = new WeakMap<StageMediaSource, AbortController>()
const stageMediaObjectUrls = new WeakMap<StageMediaSource, string>()
const stageMediaBlobCache = new Map<string, Blob>()
const stageMediaBlobRequests = new Map<string, Promise<Blob>>()

const cacheStageMediaBlob = (url: string, blob: Blob) => {
  stageMediaBlobCache.delete(url)
  stageMediaBlobCache.set(url, blob)
  while (stageMediaBlobCache.size > 128) {
    const oldest = stageMediaBlobCache.keys().next().value
    if (typeof oldest !== 'string') break
    stageMediaBlobCache.delete(oldest)
  }
  return blob
}

const fetchStageMediaBlob = (url: string, force = false) => {
  if (!force) {
    const cached = stageMediaBlobCache.get(url)
    if (cached) return Promise.resolve(cached)
    const pending = stageMediaBlobRequests.get(url)
    if (pending) return pending
  }
  const requestUrl = force ? `${url}${url.includes('?') ? '&' : '?'}theaterRetry=${Date.now()}` : url
  const request = (async () => {
    const contentURL = new URL(requestUrl, window.location.href)
    contentURL.pathname = contentURL.pathname.replace(/\/content\/?$/, '/content-url')
    contentURL.search = ''
    const resolved = await api.get<{ url?: unknown }>(contentURL.toString())
    const directURL = typeof resolved.data?.url === 'string' ? resolved.data.url.trim() : ''
    if (directURL) {
      const response = await fetch(directURL, { credentials: 'omit' })
      if (!response.ok) throw new Error(`资源请求失败（HTTP ${response.status}）`)
      const blob = await response.blob()
      if (blob.size === 0) throw new Error('资源响应为空')
      return cacheStageMediaBlob(url, blob)
    }
    const response = await api.get<Blob>(requestUrl, { responseType: 'blob' })
    if (!(response.data instanceof Blob) || response.data.size === 0) throw new Error('资源响应为空')
    return cacheStageMediaBlob(url, response.data)
  })().finally(() => {
    if (stageMediaBlobRequests.get(url) === request) stageMediaBlobRequests.delete(url)
  })
  stageMediaBlobRequests.set(url, request)
  return request
}

const stageMediaErrorMessage = (error: unknown) => {
  const status = Number((error as { response?: { status?: number } })?.response?.status || 0)
  if (status === 401) return '资源鉴权失败，请重新登录'
  if (status === 403) return '没有读取此资源的权限'
  if (status === 404) return '资源不存在或不属于当前小剧场'
  return theaterAudioErrorMessage(error, '资源请求失败')
}

const theaterMediaDebug = (...args: unknown[]) => {
  const enabled = typeof window !== 'undefined'
    && ((window as any).__SC_DEBUG__ === true || window.localStorage.getItem('SC_DEBUG') === '1')
  if (enabled) {
    console.info('[theater-media]', ...args)
  }
}

const releaseStageMedia = (source: StageMediaSource | null | undefined) => {
  if (!source) return
  stageMediaRequestControllers.get(source)?.abort()
  stageMediaRequestControllers.delete(source)
  activeAnimatedMedia.delete(source)
  if (isVideoSource(source)) {
    source.pause()
    source.onloadedmetadata = null
    source.onerror = null
    source.onended = null
    videoLoopStates.delete(source)
    source.removeAttribute('src')
    source.load()
  } else {
    source.onload = null
    source.onerror = null
    source.removeAttribute('src')
  }
  const objectUrl = stageMediaObjectUrls.get(source)
  if (objectUrl) URL.revokeObjectURL(objectUrl)
  stageMediaObjectUrls.delete(source)
  syncMediaAnimation()
}

const loadStageMedia = (
  imageRef: StageImageRef,
  location: TheaterStageMediaLocation,
  onReady: (source: StageMediaSource) => void,
  onError: (message: string) => void,
) => {
  const source: StageMediaSource = imageRef.mimeType === 'video/webm'
    ? document.createElement('video')
    : new Image()
  const controller = new AbortController()
  stageMediaRequestControllers.set(source, controller)
  let authenticatedAttempt = 0

  // 舞台与 API 可能跨端口，显式携带凭据并保持 Canvas 可安全绘制。
  if (location.managed) source.crossOrigin = 'use-credentials'
  theaterMediaDebug('create', {
    source: isVideoSource(source) ? 'video' : 'image',
    managed: location.managed,
    url: location.url,
    resourceId: imageRef.resourceId,
    mimeType: imageRef.mimeType,
  })

  const assignSourceUrl = (sourceUrl: string) => {
    if (controller.signal.aborted) return
    source.src = sourceUrl
    if (isVideoSource(source)) source.load()
  }

  const loadAuthenticatedSource = (force = false) => {
    authenticatedAttempt += 1
    void fetchStageMediaBlob(location.url, force).then((blob) => {
      theaterMediaDebug('blob response', {
        size: blob.size,
        blobType: blob.type,
        url: location.url,
      })
      const previousUrl = stageMediaObjectUrls.get(source)
      if (previousUrl) URL.revokeObjectURL(previousUrl)
      const sourceUrl = URL.createObjectURL(blob)
      if (controller.signal.aborted) {
        URL.revokeObjectURL(sourceUrl)
        return
      }
      stageMediaObjectUrls.set(source, sourceUrl)
      assignSourceUrl(sourceUrl)
    }).catch((error) => {
      if (controller.signal.aborted) return
      theaterMediaDebug('blob error', error)
      if (authenticatedAttempt < 3) {
        window.setTimeout(() => {
          if (!controller.signal.aborted) loadAuthenticatedSource(true)
        }, authenticatedAttempt * 250)
        return
      }
      stageMediaRequestControllers.delete(source)
      onError(stageMediaErrorMessage(error))
    })
  }

  const handleSourceError = (decodeMessage: string) => {
    if (!location.managed) {
      onError(decodeMessage)
      return
    }
    if (authenticatedAttempt < 3) {
      loadAuthenticatedSource(true)
      return
    }
    onError(decodeMessage)
  }

  if (isVideoSource(source)) {
    source.muted = true
    source.loop = false
    source.autoplay = false
    source.playsInline = true
    source.preload = 'auto'
    source.onloadedmetadata = () => {
      theaterMediaDebug('video loadedmetadata', { width: source.videoWidth, height: source.videoHeight, url: location.url })
      stageMediaRequestControllers.delete(source)
      onReady(source)
    }
    source.onerror = () => {
      theaterMediaDebug('video error', { error: source.error, url: location.url })
      handleSourceError('浏览器无法解码此动图')
    }
  } else {
    source.onload = () => {
      theaterMediaDebug('image load', { width: source.naturalWidth, height: source.naturalHeight, url: location.url })
      stageMediaRequestControllers.delete(source)
      onReady(source)
    }
    source.onerror = () => {
      theaterMediaDebug('image error', { url: location.url })
      handleSourceError('浏览器无法解码此图片')
    }
  }

  if (location.managed) {
    loadAuthenticatedSource()
  } else {
    assignSourceUrl(location.url)
  }
  return source
}

type ScenePreloadStatus = 'loading' | 'ready' | 'error'
const scenePreloadStatus = ref<Record<string, ScenePreloadStatus>>({})
const scenePreloadPulse = ref<Record<string, boolean>>({})
const scenePreloadPulseTimers = new Map<string, number>()
const handledPreloadRequestIds = new Set<string>()

const pulseScenePreload = (sceneId: string) => {
  const existingTimer = scenePreloadPulseTimers.get(sceneId)
  if (existingTimer !== undefined) window.clearTimeout(existingTimer)
  scenePreloadPulse.value[sceneId] = false
  void nextTick(() => {
    scenePreloadPulse.value[sceneId] = true
    const timer = window.setTimeout(() => {
      scenePreloadPulse.value[sceneId] = false
      scenePreloadPulseTimers.delete(sceneId)
    }, 420)
    scenePreloadPulseTimers.set(sceneId, timer)
  })
}

const collectSceneMediaItems = (sceneId: string) => {
  const scene = props.store.state.scenes[sceneId]
  if (!scene) return []
  const refs: Array<{ key: string, imageRef: StageImageRef }> = []
  if (scene.state.background) refs.push({ key: 'surface:background', imageRef: scene.state.background })
  if (scene.state.foreground) refs.push({ key: 'surface:foreground', imageRef: scene.state.foreground })
  Object.values({ ...scene.state.sceneObjects, ...props.store.state.persistentObjects })
    .filter((object) => object.type === 'image' && Boolean(object.image))
    .forEach((object) => refs.push({ key: `object:${object.id}`, imageRef: object.image! }))
  return refs.flatMap(({ key, imageRef }) => {
    const location = resolveTheaterStageMedia(imageRef)
    return location ? [{ key, imageRef, location }] : []
  })
}

const collectSceneMedia = (sceneId: string) => {
  const unique = new Map<string, { imageRef: StageImageRef, location: TheaterStageMediaLocation }>()
  collectSceneMediaItems(sceneId).forEach(({ imageRef, location }) => unique.set(location.url, { imageRef, location }))
  return [...unique.values()]
}

const preloadStageMedia = ({ imageRef, location }: { imageRef: StageImageRef, location: TheaterStageMediaLocation }) => (
  new Promise<void>((resolve, reject) => {
    let source: StageMediaSource | null = null
    source = loadStageMedia(imageRef, location, (loadedSource) => {
      releaseStageMedia(loadedSource)
      resolve()
    }, (message) => {
      releaseStageMedia(source)
      reject(new Error(message))
    })
  })
)

const preloadSceneMedia = async (sceneId: string, pulseOnCompletion = false) => {
  if (!props.store.state.scenes[sceneId]) return
  scenePreloadStatus.value[sceneId] = 'loading'
  const queue = collectSceneMedia(sceneId)
  let cursor = 0
  let failed = false
  const worker = async () => {
    while (cursor < queue.length) {
      const item = queue[cursor++]
      try {
        await preloadStageMedia(item)
      } catch {
        failed = true
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(6, Math.max(1, queue.length)) }, worker))
  scenePreloadStatus.value[sceneId] = failed ? 'error' : 'ready'
  if (!failed && pulseOnCompletion) pulseScenePreload(sceneId)
}

const preloadScenes = async (sceneIds: string[], requestId = '') => {
  if (requestId) {
    if (handledPreloadRequestIds.has(requestId)) return
    handledPreloadRequestIds.add(requestId)
    if (handledPreloadRequestIds.size > 100) handledPreloadRequestIds.delete(handledPreloadRequestIds.values().next().value!)
  }
  const uniqueSceneIds = [...new Set(sceneIds)]
  const pulseOnCompletion = uniqueSceneIds.length > 1
  for (const sceneId of uniqueSceneIds) await preloadSceneMedia(sceneId, pulseOnCompletion)
}

const requestScenePreload = (sceneIds: string[]) => {
  const valid = [...new Set(sceneIds)].filter((sceneId) => Boolean(props.store.state.scenes[sceneId]))
  if (valid.length) emit('preloadRequested', valid)
}

interface SceneMediaBatch {
  sceneId: string
  expected: Map<string, string>
  settled: Set<string>
  reveals: Array<() => void>
  released: boolean
  timeout: number | null
}

let sceneMediaBatch: SceneMediaBatch | null = null
const sceneTransitionDurationMs = ref(400)
let sceneTransitionTimer: number | null = null

interface SceneMorphVisual {
  x: number
  y: number
  rotation: number
  scaleX: number
  scaleY: number
  opacity: number
  width: number
  height: number
}

interface SceneMorphItem {
  object: StageObject
  visual: SceneMorphVisual
  ghost: Konva.Group | null
}

interface SceneMorphSnapshot {
  sceneId: string
  previous: Map<string, SceneMorphItem>
  matches: Map<string, SceneMorphItem>
  targetAttrs: Map<string, SceneMorphVisual>
  backgroundGhost: Konva.Group | null
  textGhost: HTMLElement | null
  started: boolean
}

let sceneMorphSnapshot: SceneMorphSnapshot | null = null
let sceneMorphTweens: Konva.Tween[] = []
let sceneMorphDelayTimers: number[] = []
const sceneMorphTextHidden = ref(false)
const sceneMorphTextAnimating = ref(false)

const stageObjectTransitionKey = (object: StageObject) => {
  const value = object.metadata?.transitionKey
  return typeof value === 'string' && value.trim() ? value.trim() : ''
}

const stageObjectFallbackKey = (object: StageObject) => `${object.type}\u0000${object.name.trim().toLocaleLowerCase()}`

const uniqueObjectIndex = (objects: StageObject[], key: (object: StageObject) => string) => {
  const grouped = new Map<string, StageObject[]>()
  objects.forEach((object) => {
    const value = key(object)
    if (!value) return
    grouped.set(value, [...(grouped.get(value) || []), object])
  })
  return new Map(Array.from(grouped.entries())
    .filter(([, entries]) => entries.length === 1)
    .map(([value, entries]) => [value, entries[0]]))
}

const matchSceneMorphObjects = (previous: Map<string, SceneMorphItem>, next: StageObject[]) => {
  const matches = new Map<string, SceneMorphItem>()
  const used = new Set<string>()
  const previousObjects = Array.from(previous.values()).map((item) => item.object)
  const previousTransitionKeys = uniqueObjectIndex(previousObjects, stageObjectTransitionKey)
  const nextTransitionKeys = uniqueObjectIndex(next, stageObjectTransitionKey)

  next.forEach((object) => {
    const key = stageObjectTransitionKey(object)
    if (!key || nextTransitionKeys.get(key)?.id !== object.id) return
    const candidate = previousTransitionKeys.get(key)
    if (!candidate || used.has(candidate.id)) return
    matches.set(object.id, previous.get(candidate.id)!)
    used.add(candidate.id)
  })
  next.forEach((object) => {
    if (matches.has(object.id) || used.has(object.id) || !previous.has(object.id)) return
    matches.set(object.id, previous.get(object.id)!)
    used.add(object.id)
  })

  const unmatchedPrevious = previousObjects.filter((object) => !used.has(object.id))
  const unmatchedNext = next.filter((object) => !matches.has(object.id))
  const previousFallback = uniqueObjectIndex(unmatchedPrevious, stageObjectFallbackKey)
  const nextFallback = uniqueObjectIndex(unmatchedNext, stageObjectFallbackKey)
  unmatchedNext.forEach((object) => {
    const key = stageObjectFallbackKey(object)
    if (nextFallback.get(key)?.id !== object.id) return
    const candidate = previousFallback.get(key)
    if (!candidate || used.has(candidate.id)) return
    matches.set(object.id, previous.get(candidate.id)!)
    used.add(candidate.id)
  })
  return matches
}

const finishSceneMorph = () => {
  if (sceneTransitionTimer !== null) window.clearTimeout(sceneTransitionTimer)
  sceneTransitionTimer = null
  sceneMorphDelayTimers.forEach((timer) => window.clearTimeout(timer))
  sceneMorphDelayTimers = []
  sceneMorphTweens.forEach((tween) => tween.destroy())
  sceneMorphTweens = []
  const snapshot = sceneMorphSnapshot
  if (snapshot) {
    snapshot.targetAttrs.forEach((attrs, objectId) => objectNodes.get(objectId)?.setAttrs(attrs))
    snapshot.previous.forEach((item) => item.ghost?.destroy())
    snapshot.backgroundGhost?.destroy()
    snapshot.textGhost?.remove()
  }
  sceneMorphSnapshot = null
  sceneMorphTextHidden.value = false
  sceneMorphTextAnimating.value = false
  backgroundLayer?.batchDraw()
  worldLayer?.batchDraw()
  foregroundLayer?.batchDraw()
}

const prepareSceneMorph = (captureCurrent: boolean, sceneId: string) => {
  finishSceneMorph()
  const transition = props.store.state.scenes[sceneId]?.state.transition
  sceneTransitionDurationMs.value = transition?.type === 'crossfade' && transition.durationMs > 0
    ? Math.min(2_000, Math.max(150, transition.durationMs))
    : 400
  if (!captureCurrent || !sceneMorphRoot) return

  const previous = new Map<string, SceneMorphItem>()
  const sceneObjects = props.store.state.liveState.sceneObjects
  Object.values(sceneObjects).forEach((object) => {
    if (isTheaterEffectObject(object)) return
    const node = objectNodes.get(object.id)
    if (!node) return
    const root = !object.parentId || !sceneObjects[object.parentId]
    const ghost = root ? node.clone({ listening: false }) as Konva.Group : null
    if (ghost) sceneMorphRoot!.add(ghost)
    previous.set(object.id, {
      object: { ...object, transform: { ...object.transform }, metadata: { ...object.metadata } },
      visual: {
        x: node.x(),
        y: node.y(),
        rotation: node.rotation(),
        scaleX: node.scaleX(),
        scaleY: node.scaleY(),
        opacity: node.opacity(),
        width: Math.max(0.5, object.transform.width),
        height: Math.max(0.5, object.transform.height),
      },
      ghost,
    })
  })

  let backgroundGhost: Konva.Group | null = null
  backgroundLayer?.draw()
  const backgroundCanvas = backgroundLayer?.getCanvas()._canvas
  if (backgroundCanvas && stage) {
    const snapshotCanvas = document.createElement('canvas')
    snapshotCanvas.width = backgroundCanvas.width
    snapshotCanvas.height = backgroundCanvas.height
    snapshotCanvas.getContext('2d')?.drawImage(backgroundCanvas, 0, 0)
    backgroundGhost = new Konva.Group({ listening: false })
    backgroundGhost.add(new Konva.Image({
      image: snapshotCanvas,
      width: stage.width(),
      height: stage.height(),
      listening: false,
    }))
    backgroundLayer?.add(backgroundGhost)
  }

  const textOverlay = viewportRef.value?.querySelector<HTMLElement>('.theater-text-overlay')
  const textGhost = textOverlay?.cloneNode(true) as HTMLElement | undefined
  if (textGhost) {
    textGhost.style.pointerEvents = 'none'
    viewportRef.value?.append(textGhost)
  }
  sceneMorphTextHidden.value = true
  sceneMorphSnapshot = {
    sceneId,
    previous,
    matches: new Map(),
    targetAttrs: new Map(),
    backgroundGhost,
    textGhost: textGhost || null,
    started: false,
  }
}

const primeSceneMorphTargets = () => {
  const snapshot = sceneMorphSnapshot
  if (!snapshot || snapshot.started || snapshot.sceneId !== props.store.state.activeSceneId) return
  const sceneObjects = Object.values(props.store.state.liveState.sceneObjects)
    .filter((object) => !isTheaterEffectObject(object))
  snapshot.matches = matchSceneMorphObjects(snapshot.previous, sceneObjects)
  snapshot.targetAttrs.clear()
  sceneObjects.forEach((object) => {
    if (object.parentId && props.store.state.liveState.sceneObjects[object.parentId]) return
    const node = objectNodes.get(object.id)
    if (!node) return
    const target: SceneMorphVisual = {
      x: object.transform.x * WORLD_UNIT_PX,
      y: object.transform.y * WORLD_UNIT_PX,
      rotation: object.transform.rotation,
      scaleX: object.transform.scaleX,
      scaleY: object.transform.scaleY,
      opacity: 1,
      width: Math.max(0.5, object.transform.width),
      height: Math.max(0.5, object.transform.height),
    }
    snapshot.targetAttrs.set(object.id, target)
    const previous = snapshot.matches.get(object.id)
    node.setAttrs(previous
      ? {
          x: previous.visual.x,
          y: previous.visual.y,
          rotation: previous.visual.rotation,
          scaleX: previous.visual.scaleX * previous.visual.width / target.width,
          scaleY: previous.visual.scaleY * previous.visual.height / target.height,
          opacity: 0,
        }
      : { opacity: 0 })
  })
  backgroundLayer?.batchDraw()
  worldLayer?.batchDraw()
  foregroundLayer?.batchDraw()
}

const tweenSceneMorphNode = (node: Konva.Node, attrs: Record<string, number>, duration: number) => {
  const tween = new Konva.Tween({
    node,
    duration,
    easing: Konva.Easings.EaseInOut,
    ...attrs,
  })
  sceneMorphTweens.push(tween)
  tween.play()
}

const startSceneMorph = (sceneId: string) => {
  const snapshot = sceneMorphSnapshot
  if (!snapshot || snapshot.sceneId !== sceneId || snapshot.started) return
  primeSceneMorphTargets()
  snapshot.started = true
  const duration = sceneTransitionDurationMs.value / 1_000
  const matchedPrevious = new Set(snapshot.matches.values())
  snapshot.targetAttrs.forEach((target, objectId) => {
    const node = objectNodes.get(objectId)
    if (!node) return
    const previous = snapshot.matches.get(objectId)
    const coversScene = target.width >= props.store.state.liveState.fieldWidth * 0.9
      && target.height >= props.store.state.liveState.fieldHeight * 0.9
    if (previous || coversScene) node.opacity(1)
    tweenSceneMorphNode(node, {
      x: target.x,
      y: target.y,
      rotation: target.rotation,
      scaleX: target.scaleX,
      scaleY: target.scaleY,
      opacity: target.opacity,
    }, previous ? duration : duration * 0.6)
    if (!previous?.ghost) return
    tweenSceneMorphNode(previous.ghost, {
      x: target.x,
      y: target.y,
      rotation: target.rotation,
      scaleX: target.scaleX * target.width / previous.visual.width,
      scaleY: target.scaleY * target.height / previous.visual.height,
      opacity: 0,
    }, duration)
  })
  snapshot.previous.forEach((item) => {
    if (!item.ghost || matchedPrevious.has(item)) return
    const timer = window.setTimeout(() => {
      sceneMorphDelayTimers = sceneMorphDelayTimers.filter((value) => value !== timer)
      if (sceneMorphSnapshot !== snapshot) return
      tweenSceneMorphNode(item.ghost!, { opacity: 0 }, duration * 0.6)
    }, sceneTransitionDurationMs.value * 0.4)
    sceneMorphDelayTimers.push(timer)
  })
  if (snapshot.backgroundGhost) tweenSceneMorphNode(snapshot.backgroundGhost, { opacity: 0 }, duration)
  sceneMorphTextAnimating.value = true
  sceneMorphTextHidden.value = false
  snapshot.textGhost?.animate([{ opacity: 1 }, { opacity: 0 }], {
    duration: sceneTransitionDurationMs.value,
    easing: 'ease-in-out',
    fill: 'forwards',
  })
  sceneTransitionTimer = window.setTimeout(finishSceneMorph, sceneTransitionDurationMs.value + 50)
}

const beginSceneMediaBatch = (sceneId: string, captureCurrent = true) => {
  if (sceneMediaBatch && !sceneMediaBatch.released) releaseSceneMediaBatch(sceneMediaBatch)
  prepareSceneMorph(captureCurrent, sceneId)
  sceneMediaBatch = {
    sceneId,
    expected: new Map(collectSceneMediaItems(sceneId).map((item) => [item.key, item.location.url])),
    settled: new Set(),
    reveals: [],
    released: false,
    timeout: null,
  }
  const batch = sceneMediaBatch
  if (!batch.expected.size) {
    releaseSceneMediaBatch(batch)
    return
  }
  batch.timeout = window.setTimeout(() => releaseSceneMediaBatch(batch), 10_000)
}

const configureVideoLoop = (source: HTMLVideoElement, imageRef: StageImageRef, restart = false) => {
  const loopCount = Number.isInteger(imageRef.loopCount) && (imageRef.loopCount || 0) > 0
    ? Math.min(65_535, imageRef.loopCount!)
    : null
  const previous = videoLoopStates.get(source)
  if (!restart && previous?.loopCount === loopCount) return
  const state = { loopCount, completed: 0 }
  videoLoopStates.set(source, state)
  source.loop = loopCount === null
  source.onended = loopCount === null ? null : () => {
    state.completed += 1
    if (state.completed >= loopCount) {
      activeAnimatedMedia.delete(source)
      syncMediaAnimation()
      return
    }
    source.currentTime = 0
    void source.play().catch((error) => theaterMediaDebug('video replay error', stageMediaErrorMessage(error)))
  }
  if (previous && source.ended) {
    source.currentTime = 0
    activeAnimatedMedia.add(source)
    syncMediaAnimation()
    void source.play().catch((error) => theaterMediaDebug('video replay error', stageMediaErrorMessage(error)))
  }
}

const activateStageMedia = (source: StageMediaSource, imageRef: StageImageRef) => {
  if (isVideoSource(source)) {
    source.currentTime = 0
    configureVideoLoop(source, imageRef, true)
    activeAnimatedMedia.add(source)
    void source.play().catch((error) => theaterMediaDebug('video play error', stageMediaErrorMessage(error)))
  } else if (imageRef.animated) {
    activeAnimatedMedia.add(source)
  }
  syncMediaAnimation()
}

const releaseSceneMediaBatch = (batch: SceneMediaBatch) => {
  if (batch.released) return
  batch.released = true
  if (batch.timeout !== null) window.clearTimeout(batch.timeout)
  batch.timeout = null
  const reveals = batch.reveals.splice(0)
  requestAnimationFrame(() => {
    reveals.forEach((reveal) => reveal())
    if (sceneMediaBatch !== batch) return
    backgroundLayer?.draw()
    foregroundLayer?.draw()
    worldLayer?.draw()
    startSceneMorph(batch.sceneId)
  })
}

const settleSceneMedia = (key: string, url: string, reveal?: () => void) => {
  const batch = sceneMediaBatch
  if (!batch || batch.sceneId !== props.store.state.activeSceneId || batch.expected.get(key) !== url || batch.released) {
    reveal?.()
    return
  }
  if (reveal) batch.reveals.push(reveal)
  batch.settled.add(key)
  if (batch.settled.size >= batch.expected.size) releaseSceneMediaBatch(batch)
}

defineExpose({ preloadScenes, appendPointerTrace })

const setImageFit = (
  node: Konva.Image,
  source: StageMediaSource,
  width: number,
  height: number,
  fit: StageObjectFit,
) => {
  const dimensions = stageMediaDimensions(source)
  const sourceWidth = Math.max(1, dimensions.width)
  const sourceHeight = Math.max(1, dimensions.height)
  node.image(source)
  if (fit === 'fill') {
    node.position({ x: 0, y: 0 })
    node.size({ width, height })
    node.crop({ x: 0, y: 0, width: sourceWidth, height: sourceHeight })
    return
  }
  const sourceRatio = sourceWidth / sourceHeight
  const targetRatio = width / height
  const useWidth = fit === 'cover' ? sourceRatio < targetRatio : sourceRatio > targetRatio
  const renderedWidth = useWidth ? width : height * sourceRatio
  const renderedHeight = useWidth ? width / sourceRatio : height
  node.position({ x: (width - renderedWidth) / 2, y: (height - renderedHeight) / 2 })
  node.size({ width: renderedWidth, height: renderedHeight })
  node.crop({ x: 0, y: 0, width: sourceWidth, height: sourceHeight })
}

const objectImageFit = (object: StageObject): StageObjectFit => object.aspectRatioLocked ? 'contain' : 'fill'

const surfaceDrawRect = (
  source: StageMediaSource,
  width: number,
  height: number,
  fit: Exclude<StageSurfaceFit, 'tile'>,
  blurPx: number,
  zoom = 1,
) => {
  const dimensions = stageMediaDimensions(source)
  const sourceWidth = Math.max(1, dimensions.width)
  const sourceHeight = Math.max(1, dimensions.height)
  if (fit === 'fill') {
    const fillWidth = width + blurPx * 4
    const fillHeight = height + blurPx * 4
    const renderedWidth = fillWidth * zoom
    const renderedHeight = fillHeight * zoom
    return { x: (width - renderedWidth) / 2, y: (height - renderedHeight) / 2, width: renderedWidth, height: renderedHeight }
  }
  if (fit === 'center') {
    const renderedWidth = sourceWidth * zoom
    const renderedHeight = sourceHeight * zoom
    return { x: (width - renderedWidth) / 2, y: (height - renderedHeight) / 2, width: renderedWidth, height: renderedHeight }
  }
  const sourceRatio = sourceWidth / sourceHeight
  const targetRatio = width / height
  const useWidth = fit === 'cover' ? sourceRatio < targetRatio : sourceRatio > targetRatio
  let renderedWidth = (useWidth ? width : height * sourceRatio) * zoom
  let renderedHeight = (useWidth ? width / sourceRatio : height) * zoom
  let x = (width - renderedWidth) / 2
  let y = (height - renderedHeight) / 2
  if (fit === 'cover' && blurPx > 0) {
    const padding = blurPx * 2
    const scale = Math.max((width + padding * 2) / renderedWidth, (height + padding * 2) / renderedHeight)
    renderedWidth *= scale
    renderedHeight *= scale
    x = (width - renderedWidth) / 2
    y = (height - renderedHeight) / 2
  }
  return { x, y, width: renderedWidth, height: renderedHeight }
}

const drawSurfaceMedia = (slot: SurfaceSlot, context: Konva.Context) => {
  const source = slot.source
  if (!source) return
  if (slot.debugDrawCount < 2) {
    slot.debugDrawCount += 1
    theaterMediaDebug('surface draw', {
      count: slot.debugDrawCount,
      visible: slot.media.visible(),
      width: slot.placeholder.width(),
      height: slot.placeholder.height(),
      fit: slot.style.fit,
      sourceWidth: stageMediaDimensions(source).width,
      sourceHeight: stageMediaDimensions(source).height,
    })
  }
  const width = slot.placeholder.width()
  const height = slot.placeholder.height()
  const style = slot.style
  context.save()
  if (style.brightness !== 1 || style.blurPx > 0) {
    context.filter = `brightness(${style.brightness}) blur(${style.blurPx}px)`
  }
  context.imageSmoothingEnabled = true
  if (style.fit === 'tile') {
    const pattern = context.createPattern(source, 'repeat')
    if (pattern) {
      context.fillStyle = pattern
      if (style.zoom !== 1) {
        context.translate(width / 2, height / 2)
        context.scale(style.zoom, style.zoom)
        context.translate(-width / 2, -height / 2)
      }
      context.fillRect(0, 0, width, height)
    }
  } else {
    const rect = surfaceDrawRect(source, width, height, style.fit, style.blurPx, style.zoom)
    context.drawImage(source, 0, 0, stageMediaDimensions(source).width, stageMediaDimensions(source).height, rect.x, rect.y, rect.width, rect.height)
  }
  context.restore()
}

const createSurfaceSlot = (cameraGroup: Konva.Group, withBase: boolean, style: StageSurfaceStyle): SurfaceSlot => {
  const group = new Konva.Group()
  const base = withBase ? new Konva.Rect({ listening: false }) : null
  const directImage = new Konva.Image({ visible: false, listening: false })
  let slot: SurfaceSlot
  const media = new Konva.Shape({
    visible: false,
    listening: false,
    sceneFunc: (context) => drawSurfaceMedia(slot, context),
  })
  const overlay = new Konva.Rect({ visible: false, listening: false })
  const placeholder = new Konva.Rect({
    visible: false,
    fill: 'rgba(15, 23, 42, 0.78)',
    stroke: 'rgba(148, 163, 184, 0.52)',
    dash: [10, 7],
    listening: false,
  })
  const label = new Konva.Text({
    visible: false,
    align: 'center',
    verticalAlign: 'middle',
    fill: '#cbd5e1',
    fontSize: 18,
    listening: false,
  })
  cameraGroup.add(group)
  if (base) group.add(base)
  group.add(media, directImage, overlay, placeholder, label)
  slot = { group, base, media, directImage, overlay, placeholder, label, style, url: '', version: 0, source: null, ready: false, debugDrawCount: 0 }
  return slot
}

const useDirectSurfaceImage = (style: StageSurfaceStyle) => (
  style.fit !== 'tile'
)

const updateDirectSurfaceImage = (
  slot: SurfaceSlot,
  source: StageMediaSource | null,
  box: { width: number, height: number },
) => {
  theaterMediaDebug('direct image decision', {
    hasSource: Boolean(source),
    fit: slot.style.fit,
    brightness: slot.style.brightness,
    blurPx: slot.style.blurPx,
    isVideo: source ? isVideoSource(source) : false,
    useDirect: useDirectSurfaceImage(slot.style),
  })
  if (!source || !useDirectSurfaceImage(slot.style) || isVideoSource(source)) {
    slot.directImage.image(undefined)
    slot.directImage.visible(false)
    return
  }
  const dimensions = stageMediaDimensions(source)
  const rect = surfaceDrawRect(source, box.width, box.height, slot.style.fit as Exclude<StageSurfaceFit, 'tile'>, 0, slot.style.zoom)
  slot.directImage.image(source)
  slot.directImage.position({ x: rect.x, y: rect.y })
  slot.directImage.size({ width: rect.width, height: rect.height })
  slot.directImage.crop({ x: 0, y: 0, width: Math.max(1, dimensions.width), height: Math.max(1, dimensions.height) })
  slot.directImage.opacity(slot.style.opacity)
  const filters: Konva.Filter[] = []
  if (slot.style.brightness !== 1) {
    slot.directImage.brightness(slot.style.brightness - 1)
    filters.push(Konva.Filters.Brighten)
  } else {
    slot.directImage.brightness(0)
  }
  if (slot.style.blurPx > 0) {
    slot.directImage.blurRadius(slot.style.blurPx)
    filters.push(Konva.Filters.Blur)
  } else {
    slot.directImage.blurRadius(0)
  }
  slot.directImage.clearCache()
  slot.directImage.filters(filters)
  if (filters.length) slot.directImage.cache()
  slot.directImage.visible(true)
}

const updateSurfaceSlot = (
  slot: SurfaceSlot,
  imageRef: StageImageRef | null,
  box: { x: number, y: number, width: number, height: number },
  style: StageSurfaceStyle,
  loadingLabel: string,
  mediaKey: string,
) => {
  const renderedStyle = slot.style
  const applyStyle = (nextStyle: StageSurfaceStyle) => {
    slot.style = nextStyle
    slot.group.position({ x: box.x, y: box.y })
    slot.group.clip({ x: 0, y: 0, width: box.width, height: box.height })
    slot.base?.setAttrs({ width: box.width, height: box.height, fill: props.store.state.liveState.backgroundColor })
    slot.placeholder.setAttrs({ width: box.width, height: box.height })
    slot.label.setAttrs({ width: box.width, height: box.height })
    slot.media.setAttrs({ width: box.width, height: box.height, opacity: nextStyle.opacity })
    slot.directImage.setAttrs({ width: box.width, height: box.height, opacity: nextStyle.opacity })
    slot.overlay.setAttrs({
      width: box.width,
      height: box.height,
      fill: nextStyle.overlay.color,
      opacity: nextStyle.overlay.opacity * nextStyle.opacity,
    })
  }
  applyStyle(style)

  const location = imageRef ? resolveTheaterStageMedia(imageRef) : null
  const resolved = location?.url || null
  if (imageRef && location) theaterMediaDebug('surface resolve', {
    resourceId: imageRef.resourceId,
    sourceUrl: imageRef.url,
    resolvedUrl: location.url,
    managed: location.managed,
    box,
  })
  if (imageRef && !location) theaterMediaDebug('surface resolve rejected', {
    resourceId: imageRef.resourceId,
    sourceUrl: imageRef.url,
    scope: theaterMediaScope(),
  })
  if (!imageRef) {
    releaseStageMedia(slot.source)
    slot.url = ''
    slot.source = null
    slot.ready = false
    slot.media.visible(false)
    slot.directImage.visible(false)
    slot.overlay.visible(false)
    slot.placeholder.visible(false)
    slot.label.visible(false)
    return
  }
  if (!resolved) {
    releaseStageMedia(slot.source)
    slot.url = imageRef.url
    slot.source = null
    slot.ready = false
    slot.media.visible(false)
    slot.directImage.visible(false)
    slot.overlay.visible(false)
    slot.placeholder.visible(true)
    slot.label.text('图片地址被安全策略拒绝').visible(true)
    return
  }
  if (slot.url === resolved && slot.source && slot.ready) {
    if (isVideoSource(slot.source)) configureVideoLoop(slot.source, imageRef)
    updateDirectSurfaceImage(slot, slot.source, box)
    slot.media.visible(Boolean(slot.source) && !slot.directImage.visible())
    slot.overlay.visible(Boolean(slot.source) && style.overlay.enabled && style.overlay.opacity > 0)
    slot.group.getLayer()?.batchDraw()
    settleSceneMedia(mediaKey, resolved)
    return
  }
  if (slot.url === resolved && !slot.ready) {
    applyStyle(renderedStyle)
    return
  }
  const previousUrl = slot.url
  const previousSource = slot.source
  const previousReady = slot.ready
  slot.url = resolved
  slot.ready = false
  slot.version += 1
  const version = slot.version
  slot.placeholder.visible(false)
  slot.label.visible(false)
  if (previousSource) applyStyle(renderedStyle)
  let source: StageMediaSource | null = null
  source = loadStageMedia(imageRef, location!, (loadedSource) => {
    if (slot.version !== version || slot.url !== resolved) {
      releaseStageMedia(loadedSource)
      return
    }
    settleSceneMedia(mediaKey, resolved, () => {
      if (slot.version !== version || slot.url !== resolved) {
        releaseStageMedia(loadedSource)
        return
      }
      if (previousSource !== loadedSource) releaseStageMedia(previousSource)
      slot.source = loadedSource
      slot.ready = true
      applyStyle(style)
      activateStageMedia(loadedSource, imageRef)
      theaterMediaDebug('surface ready', {
        resourceId: imageRef.resourceId,
        width: stageMediaDimensions(loadedSource).width,
        height: stageMediaDimensions(loadedSource).height,
        visible: slot.media.visible(),
        box,
      })
      updateDirectSurfaceImage(slot, loadedSource, box)
      slot.media.visible(!slot.directImage.visible())
      slot.overlay.visible(slot.style.overlay.enabled && slot.style.overlay.opacity > 0)
      slot.placeholder.visible(false)
      slot.label.visible(false)
      theaterMediaDebug('surface visible', {
        resourceId: imageRef.resourceId,
        visible: slot.media.visible(),
        layer: Boolean(slot.group.getLayer()),
      })
      const layer = slot.group.getLayer()
      layer?.batchDraw()
      if (layer) {
        requestAnimationFrame(() => {
          if (!theaterMediaDebug) return
          try {
            const canvas = layer.getCanvas()._canvas
            const context = canvas.getContext('2d')
            const sampleAt = (x: number, y: number) => context
              ? Array.from(context.getImageData(Math.floor(canvas.width * x), Math.floor(canvas.height * y), 1, 1).data)
              : null
            theaterMediaDebug('surface pixels', {
              resourceId: imageRef.resourceId,
              canvas: { width: canvas.width, height: canvas.height },
              group: {
                position: slot.group.getAbsolutePosition(),
                clip: slot.group.clip(),
              },
              directImage: {
                visible: slot.directImage.visible(),
                position: slot.directImage.getAbsolutePosition(),
                size: slot.directImage.size(),
                hasImage: Boolean(slot.directImage.image()),
              },
              pixels: {
                topLeft: sampleAt(0.25, 0.25),
                center: sampleAt(0.5, 0.5),
                bottomRight: sampleAt(0.75, 0.75),
              },
            })
          } catch (error) {
            theaterMediaDebug('surface pixels error', { resourceId: imageRef.resourceId, error })
          }
        })
      }
    })
  }, (errorMessage) => {
    if (slot.version !== version || slot.url !== resolved) return
    settleSceneMedia(mediaKey, resolved)
    releaseStageMedia(source)
    slot.url = previousUrl
    slot.source = previousSource
    slot.ready = previousReady
    applyStyle(renderedStyle)
    if (previousSource) {
      updateDirectSurfaceImage(slot, previousSource, box)
      slot.media.visible(!slot.directImage.visible())
      slot.overlay.visible(slot.style.overlay.enabled && slot.style.overlay.opacity > 0)
      slot.placeholder.visible(false)
      slot.label.visible(false)
    } else {
      slot.media.visible(false)
      slot.overlay.visible(false)
      slot.placeholder.visible(true)
      slot.label.text(`${loadingLabel}加载失败：${errorMessage}`).visible(true)
    }
    theaterMediaDebug('surface error', { resourceId: imageRef.resourceId, errorMessage, box })
    slot.group.getLayer()?.batchDraw()
  })
}

const rebuildGrid = (fieldX: number, fieldY: number, fieldWidth: number, fieldHeight: number) => {
  if (!gridGroup) return
  const liveState = props.store.state.liveState
  const signature = [fieldWidth, fieldHeight, liveState.displayGrid, liveState.gridSize].join(':')
  if (signature === gridSignature) return
  gridSignature = signature
  gridGroup.destroyChildren()
  if (!liveState.displayGrid) return
  const step = Math.max(0.25, liveState.gridSize) * WORLD_UNIT_PX
  for (let x = fieldX; x <= fieldX + fieldWidth; x += step) {
    gridGroup.add(new Konva.Line({
      points: [x, fieldY, x, fieldY + fieldHeight],
      stroke: 'rgba(148, 163, 184, 0.12)',
      strokeWidth: 1,
      listening: false,
    }))
  }
  for (let y = fieldY; y <= fieldY + fieldHeight; y += step) {
    gridGroup.add(new Konva.Line({
      points: [fieldX, y, fieldX + fieldWidth, y],
      stroke: 'rgba(148, 163, 184, 0.12)',
      strokeWidth: 1,
      listening: false,
    }))
  }
}

const syncField = () => {
  if (!backgroundSlot || !foregroundSlot) return
  const liveState = props.store.state.liveState
  const width = liveState.fieldWidth * WORLD_UNIT_PX
  const height = liveState.fieldHeight * WORLD_UNIT_PX
  const box = { x: -width / 2, y: -height / 2, width, height }
  const viewportBox = { x: 0, y: 0, width: viewportSize.value.width, height: viewportSize.value.height }
  updateSurfaceSlot(backgroundSlot, liveState.background, viewportBox, liveState.surfaceStyles.background, '背景', 'surface:background')
  updateSurfaceSlot(foregroundSlot, liveState.foreground, box, liveState.surfaceStyles.foreground, '前景', 'surface:foreground')
  rebuildGrid(box.x, box.y, width, height)
  backgroundLayer?.batchDraw()
  worldLayer?.batchDraw()
  foregroundLayer?.batchDraw()
}

const drawingDash = (style: StageDrawingStyle) => style.dash === 'dashed'
  ? [style.strokeWidth * 3, style.strokeWidth * 2]
  : style.dash === 'dotted'
    ? [style.strokeWidth, style.strokeWidth * 1.8]
    : []

const createDrawingNode = (drawing: StageDrawing, width: number, height: number): Konva.Shape => {
  const style = drawing.style
  const common = {
    name: 'theater-object-drawing',
    stroke: style.stroke,
    strokeWidth: style.strokeWidth,
    opacity: style.opacity,
    dash: drawingDash(style),
    lineCap: 'round' as const,
    lineJoin: 'round' as const,
    hitStrokeWidth: Math.max(12, style.strokeWidth + 8),
  }
  if (drawing.tool === 'pen' || drawing.tool === 'highlighter') {
    const points = drawing.points || [0, 0, 1, 1]
    const mapped = points.map((point, index) => point * (index % 2 === 0 ? width : height))
    return new Konva.Line({
      ...common,
      points: mapped,
      tension: drawing.smoothing || 0,
      globalCompositeOperation: drawing.tool === 'highlighter' ? 'source-over' : undefined,
    })
  }
  if (drawing.tool === 'line' || drawing.tool === 'arrow') {
    const points = drawing.points || [0, 0.5, 1, 0.5]
    const mapped = points.map((point, index) => point * (index % 2 === 0 ? width : height))
    return drawing.tool === 'arrow'
      ? new Konva.Arrow({
          ...common,
          points: mapped,
          fill: style.stroke,
          pointerLength: Math.max(10, style.strokeWidth * 3),
          pointerWidth: Math.max(9, style.strokeWidth * 2.5),
        })
      : new Konva.Line({ ...common, points: mapped })
  }
  if (drawing.tool === 'ellipse') {
    return new Konva.Ellipse({
      ...common,
      x: width / 2,
      y: height / 2,
      radiusX: width / 2,
      radiusY: height / 2,
      fill: style.fill || undefined,
    })
  }
  if (drawing.tool === 'triangle' || drawing.tool === 'polygon') {
    return new Konva.RegularPolygon({
      ...common,
      x: width / 2,
      y: height / 2,
      sides: drawing.tool === 'triangle' ? 3 : drawing.sides || 6,
      radius: Math.max(1, Math.min(width, height) / 2),
      fill: style.fill || undefined,
    })
  }
  return new Konva.Rect({
    ...common,
    width,
    height,
    fill: style.fill || undefined,
    cornerRadius: Math.min(12, width / 5, height / 5),
  })
}

const drawingBounds = (session: DrawingSession) => {
  let end = { ...session.current }
  const delta = { x: end.x - session.start.x, y: end.y - session.start.y }
  if (session.shiftKey) {
    if (session.tool === 'line' || session.tool === 'arrow') {
      const length = Math.hypot(delta.x, delta.y)
      const angle = Math.round(Math.atan2(delta.y, delta.x) / (Math.PI / 4)) * (Math.PI / 4)
      end = { x: session.start.x + Math.cos(angle) * length, y: session.start.y + Math.sin(angle) * length }
    } else {
      const size = Math.max(Math.abs(delta.x), Math.abs(delta.y))
      end = {
        x: session.start.x + Math.sign(delta.x || 1) * size,
        y: session.start.y + Math.sign(delta.y || 1) * size,
      }
    }
  }
  const start = session.altKey
    ? { x: session.start.x - (end.x - session.start.x), y: session.start.y - (end.y - session.start.y) }
    : session.start
  const minimum = 12
  if (Math.abs(end.x - start.x) < minimum) end.x = start.x + Math.sign(end.x - start.x || 1) * minimum
  if (Math.abs(end.y - start.y) < minimum) end.y = start.y + Math.sign(end.y - start.y || 1) * minimum
  const x = Math.min(start.x, end.x)
  const y = Math.min(start.y, end.y)
  const width = Math.max(minimum, Math.abs(end.x - start.x))
  const height = Math.max(minimum, Math.abs(end.y - start.y))
  return { start, end, x, y, width, height }
}

const compactDrawingPoints = (points: number[]) => {
  const maximumPointCount = 1_000
  const pointCount = Math.floor(points.length / 2)
  if (pointCount <= maximumPointCount) return points
  const result: number[] = []
  for (let index = 0; index < maximumPointCount; index += 1) {
    const sourceIndex = Math.round(index * (pointCount - 1) / (maximumPointCount - 1)) * 2
    result.push(points[sourceIndex], points[sourceIndex + 1])
  }
  return result
}

const drawingResult = (session: DrawingSession) => {
  const style: StageDrawingStyle = {
    ...drawingStyle.value,
    fill: ['rectangle', 'ellipse', 'triangle', 'polygon'].includes(session.tool) ? drawingStyle.value.fill : null,
  }
  if (session.tool === 'pen' || session.tool === 'highlighter') {
    const sourcePoints = compactDrawingPoints(session.points)
    const xs = sourcePoints.filter((_, index) => index % 2 === 0)
    const ys = sourcePoints.filter((_, index) => index % 2 === 1)
    const padding = style.strokeWidth / 2
    const x = Math.min(...xs) - padding
    const y = Math.min(...ys) - padding
    const width = Math.max(12, Math.max(...xs) - Math.min(...xs) + padding * 2)
    const height = Math.max(12, Math.max(...ys) - Math.min(...ys) + padding * 2)
    const points = sourcePoints.map((point, index) => index % 2 === 0 ? (point - x) / width : (point - y) / height)
    return {
      drawing: { tool: session.tool, style, points, smoothing: drawingSmoothing.value } satisfies StageDrawing,
      transform: {
        x: (x + width / 2) / WORLD_UNIT_PX,
        y: (y + height / 2) / WORLD_UNIT_PX,
        width: width / WORLD_UNIT_PX,
        height: height / WORLD_UNIT_PX,
        rotation: 0,
      },
      preview: { x, y, width, height },
    }
  }
  const bounds = drawingBounds(session)
  const points = session.tool === 'line' || session.tool === 'arrow'
    ? [
        (bounds.start.x - bounds.x) / bounds.width,
        (bounds.start.y - bounds.y) / bounds.height,
        (bounds.end.x - bounds.x) / bounds.width,
        (bounds.end.y - bounds.y) / bounds.height,
      ]
    : undefined
  return {
    drawing: {
      tool: session.tool,
      style,
      ...(points ? { points } : {}),
      ...(session.tool === 'polygon' ? { sides: drawingPolygonSides.value } : {}),
    } satisfies StageDrawing,
    transform: {
      x: (bounds.x + bounds.width / 2) / WORLD_UNIT_PX,
      y: (bounds.y + bounds.height / 2) / WORLD_UNIT_PX,
      width: bounds.width / WORLD_UNIT_PX,
      height: bounds.height / WORLD_UNIT_PX,
      rotation: 0,
    },
    preview: bounds,
  }
}

const renderDrawingDraft = () => {
  if (!drawingDraftRoot || !drawingSession) return
  const result = drawingResult(drawingSession)
  drawingDraftRoot.destroyChildren()
  const group = new Konva.Group({ x: result.preview.x, y: result.preview.y, listening: false })
  group.add(createDrawingNode(result.drawing, result.preview.width, result.preview.height))
  drawingDraftRoot.add(group)
  worldLayer?.batchDraw()
}

const cancelDrawingSession = () => {
  drawingSession = null
  drawingDraftRoot?.destroyChildren()
  worldLayer?.batchDraw()
}

const releaseObjectMedia = (wrapper: Konva.Group) => {
  clearLayerPreviewUrl(String(wrapper.getAttr('stageObjectId') || ''))
  const image = wrapper.findOne<Konva.Image>('.theater-object-image')
  releaseStageMedia(image?.image() as StageMediaSource | undefined)
  image?.image(undefined)
}

const rebuildObjectContent = (wrapper: Konva.Group, object: StageObject) => {
  if (wrapper.getAttr('stageObjectType') && wrapper.getAttr('stageObjectType') !== object.type) {
    imageLoadVersions.set(object.id, (imageLoadVersions.get(object.id) || 0) + 1)
    wrapper.setAttr('stageImageUrl', '')
  }
  releaseObjectMedia(wrapper)
  wrapper.destroyChildren()
  wrapper.setAttr('stageObjectType', object.type)
  const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
  const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
  if (object.type === 'drawing' && object.drawing) {
    wrapper.setAttr('stageDrawingSignature', JSON.stringify(object.drawing))
    wrapper.add(createDrawingNode(object.drawing, width, height))
    return
  }
  if (object.type === 'text') {
    wrapper.add(new Konva.Rect({
      name: 'theater-object-content',
      width,
      height,
      fill: 'rgba(0, 0, 0, 0.001)',
      strokeEnabled: false,
    }))
    return
  }
  if (object.type === 'image') {
    wrapper.add(
      new Konva.Rect({
        name: 'theater-object-image-frame',
        width,
        height,
        listening: false,
      }),
      new Konva.Image({ image: undefined, name: 'theater-object-image', visible: false }),
      new Konva.Rect({
        name: 'theater-object-image-placeholder',
        width,
        height,
        fill: 'rgba(15, 23, 42, 0.82)',
        stroke: 'rgba(148, 163, 184, 0.62)',
        dash: [8, 6],
      }),
      new Konva.Text({
        name: 'theater-object-image-label',
        width,
        height,
        text: '未设置图片',
        align: 'center',
        verticalAlign: 'middle',
        fill: '#cbd5e1',
        fontSize: 14,
        padding: 10,
      }),
    )
    return
  }
  if (object.type === 'button') {
    wrapper.add(
      new Konva.Rect({
        name: 'theater-object-content',
        width,
        height,
        fill: object.fill,
        stroke: 'rgba(255, 255, 255, 0.7)',
        strokeWidth: 1,
        cornerRadius: 12,
        shadowColor: '#000000',
        shadowBlur: 18,
        shadowOpacity: 0.28,
      }),
      new Konva.Text({
        name: 'theater-object-button-label',
        text: object.text || object.name,
        width,
        height,
        align: 'center',
        verticalAlign: 'middle',
        fill: '#ffffff',
        fontSize: 20,
        fontStyle: 'bold',
        padding: 8,
      }),
    )
    return
  }
  if (object.type === 'group') {
    wrapper.add(new Konva.Rect({
      name: 'theater-object-group-control-bounds',
      visible: false,
      fill: 'rgba(0, 0, 0, 0)',
      strokeEnabled: false,
    }))
    wrapper.add(new Konva.Rect({
      name: 'theater-object-group-selection-outline',
      visible: false,
      listening: false,
      stroke: '#38bdf8',
      strokeWidth: 2,
      strokeScaleEnabled: false,
      dash: [6, 4],
    }))
    return
  }
  wrapper.add(new Konva.Rect({
    name: 'theater-object-content',
    width,
    height,
    fill: object.fill,
    stroke: 'rgba(255, 255, 255, 0.58)',
    strokeWidth: 1,
    cornerRadius: 14,
    shadowColor: '#000000',
    shadowBlur: 18,
    shadowOpacity: 0.28,
  }))
}

const createObjectNode = (object: StageObject) => {
  const wrapper = new Konva.Group({ id: `theater-object-${object.id}` })
  wrapper.setAttr('stageObjectId', object.id)
  rebuildObjectContent(wrapper, object)
  wrapper.on('pointerdown', (event) => {
    if (viewToolActive.value) return
    if (event.evt.button !== 0) return
    const current = getObject(object.id)
    if (quickDeleteActive.value) {
      if (!canEditAllObjects.value) return
      const targetId = canvasSelectionTarget(object.id)
      if (!getObject(targetId)) return
      event.cancelBubble = true
      quickDeleteOutline?.visible(false)
      removeObjectsWithConfirm([targetId])
      return
    }
    if (activeCanvasTool.value === 'eraser') {
      if (!canEditAllObjects.value || current?.type !== 'drawing') return
      event.cancelBubble = true
      props.store.selectObject(object.id)
      removeObjectsWithConfirm([object.id])
      return
    }
    if (isDrawingTool(activeCanvasTool.value)) {
      event.cancelBubble = false
      return
    }
    if (!canEditObject(current)) return
    const selectionId = canvasSelectionTarget(object.id)
    event.cancelBubble = selectionId === object.id
    const additive = event.evt.shiftKey || event.evt.ctrlKey || event.evt.metaKey
    if (
      props.store.selection.bulkMode
      && selectedIdSet.value.has(selectionId)
      && !additive
    ) return
    selectObject(selectionId, additive)
  })
  wrapper.on('dblclick dbltap', (event) => {
    if (viewToolActive.value || activeCanvasTool.value || quickDeleteActive.value) return
    if (!canEditObject(getObject(object.id))) return
    event.cancelBubble = true
    selectObject(object.id)
  })
  wrapper.on('click tap', () => {
    if (activeCanvasTool.value || quickDeleteActive.value) return
    const current = getObject(object.id)
    if (current) triggerObjectActions(current)
  })
  wrapper.on('contextmenu', (event) => {
    if (viewToolActive.value || activeCanvasTool.value || quickDeleteActive.value) return
    if (!canEditObject(getObject(object.id))) return
    event.evt.preventDefault()
    event.cancelBubble = true
    openObjectInspector(object.id)
  })
  wrapper.on('pointerenter pointermove', () => {
    if (!quickDeleteActive.value || !stage || !quickDeleteOutline) return
    const targetId = canvasSelectionTarget(object.id)
    const node = objectNodes.get(targetId)
    if (!node) return
    const box = node.getClientRect({ relativeTo: stage })
    quickDeleteOutline.setAttrs({
      x: box.x - 4,
      y: box.y - 4,
      width: box.width + 8,
      height: box.height + 8,
      visible: true,
    })
    interactionLayer?.batchDraw()
  })
  wrapper.on('pointerleave', () => {
    if (!quickDeleteActive.value) return
    quickDeleteOutline?.visible(false)
    interactionLayer?.batchDraw()
  })
  wrapper.on('dragstart', () => {
    if (!canEditObject(getObject(object.id))) return
    if (isBatchSelection.value && selectedIdSet.value.has(object.id)) {
      if (batchMoveBlocked.value) {
        wrapper.stopDrag()
        return
      }
      const rootIds = selectedMovementRootIds()
      if (!rootIds.includes(object.id)) {
        wrapper.stopDrag()
        return
      }
      const nodes = new Map<string, { node: Konva.Group, absolute: { x: number, y: number } }>()
      rootIds.forEach((id) => {
        const node = objectNodes.get(id)
        if (node) nodes.set(id, { node, absolute: node.absolutePosition() })
      })
      const driverStart = nodes.get(object.id)?.absolute
      if (!driverStart) return
      multiDrag = { driverId: object.id, driverStart, nodes }
      props.store.beginObjectEdit('批量移动对象')
      return
    }
    props.store.beginObjectEdit('移动对象')
  })
  wrapper.on('dragmove', () => {
    if (!multiDrag || multiDrag.driverId !== object.id) return
    const current = wrapper.absolutePosition()
    const delta = {
      x: current.x - multiDrag.driverStart.x,
      y: current.y - multiDrag.driverStart.y,
    }
    multiDrag.nodes.forEach(({ node, absolute }, id) => {
      if (id === object.id) return
      node.absolutePosition({ x: absolute.x + delta.x, y: absolute.y + delta.y })
    })
    updateTransformer()
  })
  wrapper.on('dragend', () => {
    if (multiDrag?.driverId === object.id) {
      const currentDrag = multiDrag
      multiDrag = null
      currentDrag.nodes.forEach(({ node }, id) => {
        const current = getObject(id)
        if (!current) return
        current.transform.x = Number((node.x() / WORLD_UNIT_PX).toFixed(6))
        current.transform.y = Number((node.y() / WORLD_UNIT_PX).toFixed(6))
      })
      props.store.commitObjectEdit()
      updateTransformer()
      return
    }
    const current = getObject(object.id)
    if (!canEditObject(current)) {
      props.store.cancelObjectEdit()
      return
    }
    current.transform.x = Number((wrapper.x() / WORLD_UNIT_PX).toFixed(6))
    current.transform.y = Number((wrapper.y() / WORLD_UNIT_PX).toFixed(6))
    props.store.commitObjectEdit()
  })
  wrapper.on('transformstart', () => {
    if (!canEditObject(getObject(object.id))) return
    props.store.beginObjectEdit('变换对象')
  })
  wrapper.on('transformend', () => {
    const current = getObject(object.id)
    if (!canEditObject(current)) {
      props.store.cancelObjectEdit()
      return
    }
    if (current.type === 'group') {
      current.transform.x = Number((wrapper.x() / WORLD_UNIT_PX).toFixed(6))
      current.transform.y = Number((wrapper.y() / WORLD_UNIT_PX).toFixed(6))
      current.transform.rotation = Number(wrapper.rotation().toFixed(6))
      current.transform.scaleX = Number(Math.min(100, Math.max(0.01, wrapper.scaleX())).toFixed(6))
      current.transform.scaleY = Number(Math.min(100, Math.max(0.01, wrapper.scaleY())).toFixed(6))
      props.store.commitObjectEdit()
      return
    }
    current.transform.width = Number((Math.max(12, current.transform.width * WORLD_UNIT_PX * wrapper.scaleX()) / WORLD_UNIT_PX).toFixed(6))
    current.transform.height = Number((Math.max(12, current.transform.height * WORLD_UNIT_PX * wrapper.scaleY()) / WORLD_UNIT_PX).toFixed(6))
    current.transform.rotation = Number(wrapper.rotation().toFixed(6))
    current.transform.x = Number((wrapper.x() / WORLD_UNIT_PX).toFixed(6))
    current.transform.y = Number((wrapper.y() / WORLD_UNIT_PX).toFixed(6))
    current.transform.scaleX = 1
    current.transform.scaleY = 1
    wrapper.scale({ x: 1, y: 1 })
    props.store.commitObjectEdit()
  })
  objectNodes.set(object.id, wrapper)
  return wrapper
}

const syncObjectImage = (wrapper: Konva.Group, object: StageObject, width: number, height: number) => {
  const frame = wrapper.findOne<Konva.Rect>('.theater-object-image-frame')
  const image = wrapper.findOne<Konva.Image>('.theater-object-image')
  const placeholder = wrapper.findOne<Konva.Rect>('.theater-object-image-placeholder')
  const label = wrapper.findOne<Konva.Text>('.theater-object-image-label')
  frame?.size({ width, height })
  placeholder?.size({ width, height })
  label?.size({ width, height })
  if (!image || !placeholder || !label) return
  const location = object.image ? resolveTheaterStageMedia(object.image) : null
  const resolved = location?.url || null
  if (!object.image) {
    clearLayerPreviewUrl(object.id)
    releaseStageMedia(image.image() as StageMediaSource | undefined)
    image.image(undefined)
    wrapper.setAttr('stageImageUrl', '')
    image.visible(false)
    placeholder.visible(true)
    label.text('未设置图片').visible(true)
    return
  }
  if (!resolved) {
    clearLayerPreviewUrl(object.id)
    releaseStageMedia(image.image() as StageMediaSource | undefined)
    image.image(undefined)
    wrapper.setAttr('stageImageUrl', object.image.url)
    image.visible(false)
    placeholder.visible(true)
    label.text('图片地址被安全策略拒绝').visible(true)
    return
  }
  const currentSource = image.image() as StageMediaSource | undefined
  if (wrapper.getAttr('stageImageUrl') === resolved && currentSource) {
    if (isVideoSource(currentSource)) configureVideoLoop(currentSource, object.image)
    setImageFit(image, currentSource, width, height, objectImageFit(object))
    settleSceneMedia(`object:${object.id}`, resolved)
    return
  }
  if (wrapper.getAttr('stageImageUrl') === resolved) return
  releaseStageMedia(currentSource)
  clearLayerPreviewUrl(object.id)
  image.image(undefined)
  wrapper.setAttr('stageImageUrl', resolved)
  const version = (imageLoadVersions.get(object.id) || 0) + 1
  imageLoadVersions.set(object.id, version)
  image.visible(false)
  placeholder.visible(false)
  label.visible(false)
  let source: StageMediaSource | null = null
  source = loadStageMedia(object.image, location!, (loadedSource) => {
    if (imageLoadVersions.get(object.id) !== version || wrapper.getAttr('stageImageUrl') !== resolved) {
      releaseStageMedia(loadedSource)
      return
    }
    settleSceneMedia(`object:${object.id}`, resolved, () => {
      if (imageLoadVersions.get(object.id) !== version || wrapper.getAttr('stageImageUrl') !== resolved) {
        releaseStageMedia(loadedSource)
        return
      }
      image.image(loadedSource)
      activateStageMedia(loadedSource, object.image!)
      setImageFit(
        image,
        loadedSource,
        frame?.width() || width,
        frame?.height() || height,
        objectImageFit(object),
      )
      image.visible(true)
      if (!isVideoSource(loadedSource)) {
        const previewUrl = stageMediaObjectUrls.get(loadedSource) || location!.url
        if (previewUrl) setLayerPreviewUrl(object.id, previewUrl)
      }
      placeholder.visible(false)
      label.visible(false)
      wrapper.getLayer()?.batchDraw()
    })
  }, (errorMessage) => {
    if (imageLoadVersions.get(object.id) !== version || wrapper.getAttr('stageImageUrl') !== resolved) return
    settleSceneMedia(`object:${object.id}`, resolved)
    releaseStageMedia(source)
    wrapper.setAttr('stageImageUrl', '')
    clearLayerPreviewUrl(object.id)
    image.visible(false)
    placeholder.visible(true)
    label.text(`图片加载失败：${errorMessage}`).visible(true)
    wrapper.getLayer()?.batchDraw()
  })
}

const updateObjectNode = (wrapper: Konva.Group, object: StageObject) => {
  if (
    wrapper.getAttr('stageObjectType') !== object.type
    || (object.type === 'drawing' && wrapper.getAttr('stageDrawingSignature') !== JSON.stringify(object.drawing))
  ) rebuildObjectContent(wrapper, object)
  const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
  const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
  const multiSelected = isBatchSelection.value && selectedIdSet.value.has(object.id)
  const selectedAncestor = multiSelected && !selectedMovementRootIds().includes(object.id)
  const groupedObjectDirectlySelected = !object.parentId
    || props.store.state.selectedObjectId === object.id
    || multiSelected
  wrapper.setAttrs({
    x: object.transform.x * WORLD_UNIT_PX,
    y: object.transform.y * WORLD_UNIT_PX,
    offsetX: width / 2,
    offsetY: height / 2,
    rotation: object.transform.rotation,
    scaleX: object.transform.scaleX,
    scaleY: object.transform.scaleY,
    visible: object.visible,
    draggable: !object.locked
      && !viewToolActive.value
      && !activeCanvasTool.value
      && !quickDeleteActive.value
      && canEditObject(object)
      && groupedObjectDirectlySelected
      && (!multiSelected || (!batchMoveBlocked.value && !selectedAncestor)),
    listening: (!viewToolActive.value && canEditObject(object))
      || (canTriggerActions.value && object.interactive && ['image', 'button'].includes(object.type)),
  })
  if (object.type === 'drawing') {
    return
  } else if (object.type === 'text') {
    wrapper.findOne<Konva.Rect>('.theater-object-content')?.setAttrs({
      width,
      height,
    })
  } else if (object.type === 'image') {
    syncObjectImage(wrapper, object, width, height)
  } else if (object.type === 'button') {
    wrapper.findOne<Konva.Rect>('.theater-object-content')?.setAttrs({ width, height, fill: object.fill })
    wrapper.findOne<Konva.Text>('.theater-object-button-label')?.setAttrs({
      text: object.text || object.name,
      width,
      height,
    })
  } else if (object.type !== 'group') {
    wrapper.findOne<Konva.Rect>('.theater-object-content')?.setAttrs({ width, height, fill: object.fill })
  }
}

const syncObjects = () => {
  if (!objectRoot) return
  const objects = Object.fromEntries(Object.entries(props.store.activeObjects.value)
    .filter(([, object]) => !isTheaterEffectObject(object)))
  for (const [objectId, node] of objectNodes) {
    if (objects[objectId]) continue
    imageLoadVersions.delete(objectId)
    releaseObjectMedia(node)
    node.destroy()
    objectNodes.delete(objectId)
  }
  for (const object of Object.values(objects)) {
    const node = objectNodes.get(object.id) || createObjectNode(object)
    updateObjectNode(node, object)
  }
  syncStageObjectHierarchy(objects, objectNodes, objectRoot)
  const groupControls: Array<{
    object: StageObject
    wrapper: Konva.Group
    controlBounds: Konva.Rect
    outline: Konva.Rect
  }> = []
  for (const object of Object.values(objects)) {
    if (object.type !== 'group') continue
    const wrapper = objectNodes.get(object.id)
    const controlBounds = wrapper?.findOne<Konva.Rect>('.theater-object-group-control-bounds')
    const outline = wrapper?.findOne<Konva.Rect>('.theater-object-group-selection-outline')
    if (!wrapper || !controlBounds || !outline) continue
    controlBounds.visible(false)
    outline.visible(false)
    groupControls.push({ object, wrapper, controlBounds, outline })
  }
  const selectedId = props.store.state.selectedObjectId
  for (const { object, wrapper, controlBounds, outline } of groupControls) {
    const bounds = wrapper.getClientRect({
      skipTransform: true,
      skipShadow: true,
      skipStroke: true,
    })
    if (bounds.width <= 0 || bounds.height <= 0) continue
    controlBounds.setAttrs({
      x: bounds.x,
      y: bounds.y,
      width: bounds.width,
      height: bounds.height,
      visible: true,
    })
    if (!selectedId || isBatchSelection.value || !objectIsDescendantOf(selectedId, object.id)) continue
    const padding = 8
    outline.setAttrs({
      x: bounds.x - padding,
      y: bounds.y - padding,
      width: bounds.width + padding * 2,
      height: bounds.height + padding * 2,
      visible: true,
      opacity: 0.65,
    })
  }
  worldLayer?.batchDraw()
  primeSceneMorphTargets()
  nextTick(updateTransformer)
}

const resizeStage = () => {
  const element = viewportRef.value
  if (!stage || !element) return
  const rect = element.getBoundingClientRect()
  viewportSize.value = { width: Math.max(1, rect.width), height: Math.max(1, rect.height) }
  stage.size(viewportSize.value)
  clampOpenPanels()
  applyCamera()
}

const handleWheel = (event: Konva.KonvaEventObject<WheelEvent>) => {
  if (!stage || !worldCameraGroup) return
  event.evt.preventDefault()
  const pointer = stage.getPointerPosition()
  if (!pointer) return
  const oldZoom = props.store.state.camera.zoom
  const worldPoint = {
    x: (pointer.x - worldCameraGroup.x()) / oldZoom,
    y: (pointer.y - worldCameraGroup.y()) / oldZoom,
  }
  const direction = event.evt.deltaY > 0 ? -1 : 1
  const zoom = Math.min(3, Math.max(0.2, direction > 0 ? oldZoom * 1.08 : oldZoom / 1.08))
  props.store.state.camera.zoom = zoom
  props.store.state.camera.x = pointer.x - stage.width() / 2 - worldPoint.x * zoom
  props.store.state.camera.y = pointer.y - stage.height() / 2 - worldPoint.y * zoom
}

const startPan = (event: Konva.KonvaEventObject<PointerEvent>) => {
  if (!stage) return
  if (viewToolActive.value) {
    if (event.evt.button === 2) {
      const pointer = worldCameraGroup?.getRelativePointerPosition()
      if (!pointer) return
      event.evt.preventDefault()
      beginPointerTrace(pointer)
      return
    }
    if (event.evt.button !== 0 || event.target !== stage) return
    event.evt.preventDefault()
    panning = true
    panPointer = { x: event.evt.clientX, y: event.evt.clientY }
    panOrigin = { x: props.store.state.camera.x, y: props.store.state.camera.y }
    return
  }
  if (quickDeleteActive.value) {
    quickDeleteOutline?.visible(false)
    interactionLayer?.batchDraw()
    return
  }
  if (activeCanvasTool.value === 'eraser') return
  if (isDrawingTool(activeCanvasTool.value) && canEditAllObjects.value && event.evt.button === 0) {
    const pointer = worldCameraGroup?.getRelativePointerPosition()
    if (!pointer) return
    event.evt.preventDefault()
    drawingSession = {
      tool: activeCanvasTool.value,
      start: pointer,
      current: pointer,
      points: [pointer.x, pointer.y],
      shiftKey: event.evt.shiftKey,
      altKey: event.evt.altKey,
    }
    renderDrawingDraft()
    return
  }
  if (event.evt.button !== 0 || event.target !== stage) return
  event.evt.preventDefault()
  if (props.store.selection.bulkMode && canEditAllObjects.value) {
    const pointer = stage.getPointerPosition()
    if (!pointer) return
    marqueeStart = pointer
    marqueeAdditive = event.evt.shiftKey || event.evt.ctrlKey || event.evt.metaKey
    selectionRect?.setAttrs({ x: pointer.x, y: pointer.y, width: 0, height: 0, visible: true })
    interactionLayer?.batchDraw()
    return
  }
  selectObject(null)
  panning = true
  panPointer = { x: event.evt.clientX, y: event.evt.clientY }
  panOrigin = { x: props.store.state.camera.x, y: props.store.state.camera.y }
}

const movePan = (event: Konva.KonvaEventObject<PointerEvent>) => {
  if (pointerTraceSession) {
    const pointer = worldCameraGroup?.getRelativePointerPosition()
    if (pointer) continuePointerTrace(pointer)
    return
  }
  if (drawingSession) {
    const pointer = worldCameraGroup?.getRelativePointerPosition()
    if (!pointer) return
    drawingSession.current = pointer
    drawingSession.shiftKey = event.evt.shiftKey
    drawingSession.altKey = event.evt.altKey
    if (drawingSession.tool === 'pen' || drawingSession.tool === 'highlighter') {
      const points = drawingSession.points
      const previous = { x: points[points.length - 2], y: points[points.length - 1] }
      if (Math.hypot(pointer.x - previous.x, pointer.y - previous.y) >= 2) points.push(pointer.x, pointer.y)
    }
    renderDrawingDraft()
    return
  }
  if (marqueeStart && stage && selectionRect) {
    const pointer = stage.getPointerPosition()
    if (!pointer) return
    selectionRect.setAttrs({
      x: Math.min(marqueeStart.x, pointer.x),
      y: Math.min(marqueeStart.y, pointer.y),
      width: Math.abs(pointer.x - marqueeStart.x),
      height: Math.abs(pointer.y - marqueeStart.y),
    })
    interactionLayer?.batchDraw()
    return
  }
  if (!panning) return
  props.store.state.camera.x = panOrigin.x + event.evt.clientX - panPointer.x
  props.store.state.camera.y = panOrigin.y + event.evt.clientY - panPointer.y
}

const stopPan = (event?: Konva.KonvaEventObject<PointerEvent>) => {
  if (pointerTraceSession) {
    finishPointerTrace()
    return
  }
  if (drawingSession) {
    if (event?.type === 'pointercancel') {
      cancelDrawingSession()
      return
    }
    const session = drawingSession
    if ((session.tool === 'pen' || session.tool === 'highlighter') && session.points.length === 2) {
      session.points.push(session.points[0] + 0.01, session.points[1] + 0.01)
    }
    const result = drawingResult(session)
    cancelDrawingSession()
    props.store.addDrawing(result.drawing, result.transform)
    return
  }
  panning = false
  if (!marqueeStart || !selectionRect || !stage) return
  const additive = marqueeAdditive
  const box = {
    x: selectionRect.x(),
    y: selectionRect.y(),
    width: selectionRect.width(),
    height: selectionRect.height(),
  }
  marqueeStart = null
  marqueeAdditive = false
  selectionRect.visible(false)
  interactionLayer?.batchDraw()
  if (event?.type === 'pointercancel') return
  if (Math.hypot(box.width, box.height) < 4) {
    if (!additive) props.store.clearSelection()
    return
  }
  const hits = Object.values(props.store.activeObjects.value)
    .filter((object) => object.visible && canEditObject(object))
    .filter((object) => {
      const node = objectNodes.get(object.id)
      if (!node?.isVisible()) return false
      const bounds = marqueeObjectBounds(object, node, stage!)
      return bounds ? marqueeContains(box, bounds) : false
    })
    .map((object) => object.id)
  const rootHits = rootObjectIds(hits)
  const next = rootObjectIds(additive
    ? [...props.store.selection.selectedIds, ...rootHits]
    : rootHits)
  const primaryHit = [...rootHits].reverse().find((id) => next.includes(id))
  const currentPrimary = props.store.state.selectedObjectId
  props.store.setSelectedObjectIds(
    next,
    primaryHit || (additive && currentPrimary && next.includes(currentPrimary) ? currentPrimary : null),
  )
}

const targetImageRef = (target: ImageTarget) => target.kind === 'scene'
  ? props.store.state.liveState[target.target]
  : props.store.activeObjects.value[target.objectId]?.image || null

const targetImageUrl = (target: ImageTarget) => targetImageRef(target)?.url || ''

const applyImageUrl = (
  target: ImageTarget,
  url: string,
  resourceId?: string,
  mimeType?: string,
  animated?: boolean,
  loopCount?: number,
  dimensions?: { width: number, height: number },
) => {
  if (target.kind === 'scene') return props.store.setSceneImage(target.target, url, resourceId, mimeType, animated, loopCount)
  return props.store.setObjectImage(target.objectId, url, resourceId, mimeType, animated, loopCount, dimensions)
}

const theaterResourcePath = (resourceId = '') => {
  return buildTheaterResourcePath(theaterMediaScope(), resourceId)
}

const waitForResource = async (resourceId: string) => {
  for (let attempt = 0; attempt < 240; attempt += 1) {
    const response = await api.get<TheaterResourceResponse>(theaterResourcePath(resourceId))
    const resource = response.data?.resource
    const status = resource?.status
    if (status === 'ready') return resource
    if (status === 'failed') {
      throw new Error(theaterResourceProcessingError(response.data?.resource?.processing?.errorCode))
    }
    await new Promise((resolve) => window.setTimeout(resolve, 500))
  }
  throw new Error('图片处理超时')
}

const supportedTheaterMedia = new Set(['image/jpeg', 'image/png', 'image/apng', 'image/webp', 'image/gif', 'video/webm'])

const normalizedFileType = (file: File) => {
  const declared = file.type.trim().toLowerCase()
  if (supportedTheaterMedia.has(declared)) return declared
  const extension = file.name.toLowerCase().match(/\.([a-z0-9]+)$/)?.[1]
  return extension === 'jpg' || extension === 'jpeg'
    ? 'image/jpeg'
    : extension === 'png'
      ? 'image/png'
      : extension === 'apng'
        ? 'image/apng'
        : extension === 'webp'
          ? 'image/webp'
          : extension === 'gif'
            ? 'image/gif'
            : extension === 'webm'
              ? 'video/webm'
              : declared
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

const prepareTheaterMedia = async (file: File) => {
  const mimeType = normalizedFileType(file)
  if (!supportedTheaterMedia.has(mimeType)) throw new Error('仅支持 PNG、APNG、JPEG、WebP、GIF、WebM')
  if ((mimeType === 'image/png' || mimeType === 'image/apng') && await isAnimatedPNG(file)) return file
  if (mimeType !== 'image/jpeg' && mimeType !== 'image/png') return file
  return compressImage(file, { mimeType: 'image/webp' })
}

const theaterMediaDimensions = (file: File): Promise<{ width: number, height: number } | undefined> => new Promise((resolve) => {
  const url = URL.createObjectURL(file)
  const finish = (width?: number, height?: number) => {
    URL.revokeObjectURL(url)
    resolve(width && height ? { width, height } : undefined)
  }
  if (normalizedFileType(file) === 'video/webm') {
    const video = document.createElement('video')
    video.preload = 'metadata'
    video.onloadedmetadata = () => finish(video.videoWidth, video.videoHeight)
    video.onerror = () => finish()
    video.src = url
    return
  }
  const image = new Image()
  image.onload = () => finish(image.naturalWidth, image.naturalHeight)
  image.onerror = () => finish()
  image.src = url
})

const uploadImage = async (file: File, target: ImageTarget) => {
  if (!canEditAllObjects.value || !canUploadResources.value) throw new Error('缺少小剧场资源编辑权限')
  if (!props.worldId || !props.channelId) throw new Error('缺少小剧场频道信息')
  resourceUploading.value = true
  resourceError.value = ''
  try {
    const prepared = await prepareTheaterMedia(file)
    const targetObject = target.kind === 'object' ? props.store.activeObjects.value[target.objectId] : null
    const targetEffectConfig = isTheaterEffectObject(targetObject) ? theaterEffectConfigFromObject(targetObject) : null
    const dimensions = targetEffectConfig?.kind === 'media' && !targetObject?.image && !targetEffectConfig.media
      ? await theaterMediaDimensions(prepared)
      : undefined
    const formData = new FormData()
    formData.append('file', prepared)
    formData.append('mediaKind', 'image')
    formData.append('clientResourceId', crypto.randomUUID?.() || `image-${Date.now()}-${Math.random().toString(16).slice(2)}`)
    const response = await api.post<TheaterResourceResponse>(theaterResourcePath(), formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    let resource = response.data?.resource
    const resourceId = resource?.id
    if (!resourceId) throw new Error('上传响应缺少资源 ID')
    if (resource?.status !== 'ready') resource = await waitForResource(resourceId)
    const variant = resource?.playbackVariant || 'original'
    const mimeType = resource?.playbackMimeType || prepared.type || normalizedFileType(prepared)
    const url = theaterResourceContentPath(theaterMediaScope(), resourceId, variant)
    if (!applyImageUrl(target, url, resourceId, mimeType, resource?.animated === true, resource?.loopCount || undefined, dimensions)) throw new Error('图片目标已失效')
  } catch (error) {
    resourceError.value = error instanceof Error ? error.message : '图片上传失败'
    throw error
  } finally {
    resourceUploading.value = false
  }
}

const requestImageUpload = (target: ImageTarget) => {
  pendingImageTarget.value = target
  imageInputRef.value?.click()
}

const handleImageInput = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const target = pendingImageTarget.value
  input.value = ''
  pendingImageTarget.value = null
  if (!file || !target) return
  try {
    await uploadImage(file, target)
  } catch {
    // Error shown in inspector.
  }
}

const clearImage = (target: ImageTarget) => {
  if (!canEditAllObjects.value) return
  applyImageUrl(target, '')
  resourceError.value = ''
}

const openImageEditor = async (target: ImageTarget) => {
  if (targetImageRef(target)?.animated) return
  const url = targetImageUrl(target)
  if (!url) return
  resourceUploading.value = true
  resourceError.value = ''
  try {
    const response = await api.get<Blob>(url, { responseType: 'blob' })
    const blob = response.data
    imageEditorFile.value = new File([blob], 'theater-image.webp', { type: blob.type || 'image/webp' })
    imageEditorTarget.value = target
    imageEditorVisible.value = true
  } catch (error) {
    resourceError.value = error instanceof Error ? error.message : '图片读取失败'
  } finally {
    resourceUploading.value = false
  }
}

const closeImageEditor = () => {
  imageEditorVisible.value = false
  imageEditorFile.value = null
  imageEditorTarget.value = null
}

const saveEditedImage = async (file: File) => {
  const target = imageEditorTarget.value
  if (!target) return
  imageEditorVisible.value = false
  try {
    await uploadImage(file, target)
    closeImageEditor()
  } catch {
    imageEditorVisible.value = true
  }
}

const handleCanvasDrop = async (event: DragEvent) => {
  if (!canEditAllObjects.value || !canUploadResources.value) return
  const file = Array.from(event.dataTransfer?.files || []).find((item) => supportedTheaterMedia.has(normalizedFileType(item)))
  const rect = viewportRef.value?.getBoundingClientRect()
  if (!file || !rect) return
  const object = props.store.addObject('image')
  object.transform.x = (event.clientX - rect.left - rect.width / 2 - props.store.state.camera.x) / props.store.state.camera.zoom / WORLD_UNIT_PX
  object.transform.y = (event.clientY - rect.top - rect.height / 2 - props.store.state.camera.y) / props.store.state.camera.zoom / WORLD_UNIT_PX
  try {
    await uploadImage(file, { kind: 'object', objectId: object.id })
  } catch {
    props.store.removeSelectedObject(false)
  }
}

const reparentObjectPreservingTransform = (objectId: string, parentId: string | null) => {
  const object = getObject(objectId)
  const node = objectNodes.get(objectId)
  const parentNode = parentId ? objectNodes.get(parentId) : objectRoot
  if (!object || !node || !parentNode || object.parentId === parentId) return false
  if (parentId) {
    let parent: StageObject | undefined = getObject(parentId)
    if (!parent || parent.type !== 'group') return false
    if (props.store.isSceneFixedObject(objectId) !== props.store.isSceneFixedObject(parentId)) return false
    while (parent) {
      if (parent.id === objectId) return false
      parent = parent.parentId ? getObject(parent.parentId) : undefined
    }
  }
  const absolutePosition = node.absolutePosition()
  const absoluteRotation = node.getAbsoluteRotation()
  const absoluteScale = node.getAbsoluteScale()
  node.moveTo(parentNode)
  const parentScale = parentNode.getAbsoluteScale()
  node.rotation(absoluteRotation - parentNode.getAbsoluteRotation())
  node.scale({
    x: absoluteScale.x / Math.max(0.000001, parentScale.x),
    y: absoluteScale.y / Math.max(0.000001, parentScale.y),
  })
  node.absolutePosition(absolutePosition)
  const changed = props.store.reparentObject(objectId, parentId, {
    x: Number((node.x() / WORLD_UNIT_PX).toFixed(6)),
    y: Number((node.y() / WORLD_UNIT_PX).toFixed(6)),
    rotation: Number(node.rotation().toFixed(6)),
    scaleX: Number(node.scaleX().toFixed(6)),
    scaleY: Number(node.scaleY().toFixed(6)),
  })
  if (!changed) syncObjects()
  return changed
}

const startLayerDrag = (event: DragEvent, objectId: string) => {
  if (!canEditAllObjects.value) return
  draggedLayerId.value = objectId
  layerDropTarget.value = null
  event.dataTransfer?.setData('application/x-sealchat-stage-object', objectId)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  props.store.beginObjectEdit('调整对象分组')
}

const finishLayerDrag = () => {
  draggedLayerId.value = null
  layerDropTarget.value = null
  props.store.commitObjectEdit()
}

const handleLayerDragOver = (event: DragEvent, targetId: string) => {
  const target = getObject(targetId)
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const ratio = (event.clientY - rect.top) / Math.max(1, rect.height)
  const placement: LayerDropPlacement = target?.type === 'group' && ratio >= 0.25 && ratio <= 0.75
    ? 'inside'
    : ratio < 0.5 ? 'before' : 'after'
  layerDropTarget.value = { id: targetId, placement }
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
}

const handleLayerDragLeave = (event: DragEvent, targetId: string | null) => {
  const currentTarget = event.currentTarget as HTMLElement
  if (event.relatedTarget && currentTarget.contains(event.relatedTarget as Node)) return
  if (layerDropTarget.value?.id === targetId) layerDropTarget.value = null
}

const handleLayerDrop = (event: DragEvent, targetId: string) => {
  if (!canEditAllObjects.value) return
  const objectId = draggedLayerId.value || event.dataTransfer?.getData('application/x-sealchat-stage-object')
  if (!objectId || objectId === targetId) return
  const target = getObject(targetId)
  const placement = layerDropTarget.value?.id === targetId ? layerDropTarget.value.placement : 'after'
  if (!target) return
  if (placement === 'inside' && target.type === 'group') {
    reparentObjectPreservingTransform(objectId, target.id)
    selectObject(objectId)
    return
  }
  const object = getObject(objectId)
  if (!object) return
  if (object.parentId !== target.parentId && !reparentObjectPreservingTransform(objectId, target.parentId)) return
  props.store.reorderObject(objectId, targetId, placement === 'before' ? 'before' : 'after')
  selectObject(objectId)
}

const handleRootLayerDrop = (event: DragEvent) => {
  if (!canEditAllObjects.value) return
  const objectId = draggedLayerId.value || event.dataTransfer?.getData('application/x-sealchat-stage-object')
  if (!objectId) return
  reparentObjectPreservingTransform(objectId, null)
  selectObject(objectId)
}

onMounted(() => {
  if (!containerRef.value) return
  document.addEventListener('pointerdown', unlockTheaterAudio, true)
  document.addEventListener('touchstart', unlockTheaterAudio, { passive: true, capture: true })
  document.addEventListener('keydown', unlockTheaterAudio, true)
  panelResizeObserver = new ResizeObserver((entries) => {
    entries.forEach((entry) => {
      const element = entry.target as HTMLElement
      const id = element.dataset.panelId as PanelId | undefined
      if (!id) return
      const current = panelLayouts.value[id] || panelDefaultLayout(id)
      const next = clampPanelLayout(id, { ...current, width: element.offsetWidth, height: element.offsetHeight })
      if (JSON.stringify(next) !== JSON.stringify(current)) {
        panelLayouts.value = { ...panelLayouts.value, [id]: next }
        persistPanelLayouts()
      }
    })
  })
  stage = new Konva.Stage({ container: containerRef.value, width: 1, height: 1 })
  backgroundLayer = new Konva.Layer({ listening: false })
  worldLayer = new Konva.Layer()
  foregroundLayer = new Konva.Layer({ listening: false })
  interactionLayer = new Konva.Layer()
  backgroundCameraGroup = new Konva.Group()
  worldCameraGroup = new Konva.Group()
  foregroundCameraGroup = new Konva.Group()
  gridGroup = new Konva.Group({ listening: false })
  objectRoot = new Konva.Group()
  drawingDraftRoot = new Konva.Group({ listening: false })
  pointerTraceRoot = new Konva.Group({ listening: false })
  transformer = new Konva.Transformer({
    rotateEnabled: true,
    keepRatio: false,
    shiftBehavior: 'none',
    centeredScaling: false,
    flipEnabled: false,
    borderStroke: '#38bdf8',
    anchorStroke: '#38bdf8',
    anchorFill: '#0f172a',
    anchorSize: 9,
  })
  transformer.on('contextmenu', (event) => {
    if (viewToolActive.value || activeCanvasTool.value || quickDeleteActive.value) return
    const selectedId = props.store.state.selectedObjectId
    if (!selectedId || !canEditObject(getObject(selectedId))) return
    event.evt.preventDefault()
    event.cancelBubble = true
    openObjectInspector(selectedId)
  })
  selectionRect = new Konva.Rect({
    visible: false,
    listening: false,
    fill: 'rgba(56, 189, 248, 0.12)',
    stroke: '#38bdf8',
    strokeWidth: 1,
    dash: [5, 4],
  })
  quickDeleteOutline = new Konva.Rect({
    visible: false,
    listening: false,
    stroke: '#ef4444',
    strokeWidth: 2,
    dash: [6, 4],
  })
  backgroundSlot = createSurfaceSlot(backgroundCameraGroup, true, props.store.state.liveState.surfaceStyles.background)
  foregroundSlot = createSurfaceSlot(foregroundCameraGroup, false, props.store.state.liveState.surfaceStyles.foreground)
  sceneMorphRoot = new Konva.Group({ listening: false })
  worldCameraGroup.add(gridGroup, objectRoot, sceneMorphRoot, drawingDraftRoot, pointerTraceRoot)
  backgroundLayer.add(backgroundCameraGroup)
  worldLayer.add(worldCameraGroup)
  foregroundLayer.add(foregroundCameraGroup)
  interactionLayer.add(selectionRect, quickDeleteOutline, transformer)
  stage.add(backgroundLayer, worldLayer, foregroundLayer, interactionLayer)
  backgroundLayer.getCanvas()._canvas.style.zIndex = '0'
  worldLayer.getCanvas()._canvas.style.zIndex = '1'
  foregroundLayer.getCanvas()._canvas.style.zIndex = '3'
  interactionLayer.getCanvas()._canvas.style.zIndex = '4'
  foregroundLayer.getCanvas()._canvas.style.pointerEvents = 'none'
  interactionLayer.getCanvas()._canvas.style.pointerEvents = 'none'
  stage.on('wheel', handleWheel)
  stage.on('pointerdown', startPan)
  stage.on('pointermove', movePan)
  stage.on('pointerup pointercancel', stopPan)
  stage.on('contextmenu', (event) => event.evt.preventDefault())
  resizeObserver = new ResizeObserver(resizeStage)
  resizeObserver.observe(viewportRef.value!)
  resizeStage()
  beginSceneMediaBatch(props.store.state.activeSceneId, false)
  syncField()
  syncObjects()
  window.addEventListener('pointermove', movePanel)
  window.addEventListener('pointerup', stopPanelDrag)
  window.addEventListener('pointercancel', stopPanelDrag)
  window.addEventListener('keydown', handleStageShortcut)
  void fetchTheaterAudioAssets()
})

watch(() => props.store.state.activeSceneId, (sceneId) => beginSceneMediaBatch(sceneId), { flush: 'sync' })
watch(() => props.store.state.liveState, () => {
  syncField()
  syncObjects()
  effectRuntime.reconcile()
}, { deep: true })
watch(() => props.store.state.persistentObjects, () => {
  syncObjects()
  effectRuntime.reconcile()
}, { deep: true })
watch(() => props.store.state.camera, applyCamera, { deep: true })
watch(activeCanvasTool, () => {
  syncObjects()
  updateTransformer()
})
watch(quickDeleteActive, (active) => {
  if (!active) quickDeleteOutline?.visible(false)
  syncObjects()
  updateTransformer()
})
watch(viewToolActive, () => {
  syncObjects()
  updateTransformer()
})
watch(() => [props.syncReady, ...props.permissions], () => {
  const object = selectedObject.value
  if (object && !canEditObject(object)) props.store.clearSelection()
  if (!canEditAllObjects.value && props.store.selection.bulkMode) props.store.setBulkSelectionMode(false)
  if (!canEditAllObjects.value) quickDeleteActive.value = false
  syncObjects()
})
watch(() => props.store.selection.selectedIds.slice(), () => {
  resourceError.value = ''
  syncObjects()
  updateTransformer()
})
watch([scenePanelOpen, inspectorPanelOpen, layerPanelOpen, effectPanelOpen, assetPanelOpen], async (open) => {
  await nextTick()
  const ids: PanelId[] = ['scene', 'inspector', 'layer', 'effect', 'asset']
  open.forEach((isOpen, index) => {
    if (isOpen) ensurePanelLayout(ids[index])
  })
  observeOpenPanels()
})
watch(() => [props.worldId, props.channelId], () => { void fetchTheaterAudioAssets() })
watch(theaterAudioMasterVolume, (volume) => {
  const normalized = Math.max(0, Math.min(1, volume))
  try {
    window.localStorage.setItem(theaterAudioMasterVolumeKey, String(normalized))
  } catch {
    // Playback remains available when browser storage is disabled.
  }
  theaterAudioPlayers.forEach((player, key) => {
    player.volume((theaterAudioBaseVolumes.get(key) ?? 1) * normalized)
  })
})

onBeforeUnmount(() => {
  stopPackagePolling()
  unsubscribeEffectRuntime()
  effectRuntime.dispose()
  theaterAudioSequences.clear()
  scenePreloadPulseTimers.forEach((timer) => window.clearTimeout(timer))
  scenePreloadPulseTimers.clear()
  if (theaterAudioRefreshTimer !== null) window.clearTimeout(theaterAudioRefreshTimer)
  Array.from(theaterAudioPlayers.keys()).forEach(stopTheaterAudioPlayer)
  Howler.volume(previousHowlerVolume)
  resizeObserver?.disconnect()
  panelResizeObserver?.disconnect()
  window.removeEventListener('pointermove', movePanel)
  window.removeEventListener('pointerup', stopPanelDrag)
  window.removeEventListener('pointercancel', stopPanelDrag)
  window.removeEventListener('keydown', handleStageShortcut)
  document.removeEventListener('pointerdown', unlockTheaterAudio, true)
  document.removeEventListener('touchstart', unlockTheaterAudio, true)
  document.removeEventListener('keydown', unlockTheaterAudio, true)
  props.store.commitObjectEdit()
  cancelDrawingSession()
  finishPointerTrace()
  Array.from(pointerTraceVisuals.keys()).forEach(clearPointerTrace)
  if (sceneMediaBatch && !sceneMediaBatch.released) releaseSceneMediaBatch(sceneMediaBatch)
  sceneMediaBatch = null
  finishSceneMorph()
  objectNodes.forEach(releaseObjectMedia)
  releaseStageMedia(backgroundSlot?.source)
  releaseStageMedia(foregroundSlot?.source)
  Array.from(activeAnimatedMedia).forEach(releaseStageMedia)
  mediaAnimation?.stop()
  mediaAnimation = null
  objectNodes.clear()
  imageLoadVersions.clear()
  stageMediaBlobCache.clear()
  props.store.setBulkSelectionMode(false)
  stage?.destroy()
  stage = null
  drawingDraftRoot = null
  pointerTraceRoot = null
  sceneMorphRoot = null
})
</script>

<template>
  <section class="theater-stage-app">
    <input ref="imageInputRef" class="theater-image-input" type="file" accept="image/png,image/apng,image/jpeg,image/webp,image/gif,video/webm,.apng,.webm" @change="handleImageInput">
    <input ref="packageInputRef" class="theater-image-input" type="file" accept=".zip,application/zip" @change="handlePackageInput">
    <input ref="ccfoliaInputRef" class="theater-image-input" type="file" accept=".zip,application/zip" @change="handleCCFOLIAInput">
    <header
      class="theater-stage-toolbar"
      :class="{ 'is-controls-visible': toolbarColorsVisible }"
      @pointerenter="revealToolbarColors"
      @pointerleave="hideToolbarColors"
      @focusin="revealToolbarColors"
      @focusout="handleToolbarFocusOut"
    >
      <n-tooltip trigger="hover">
        <template #trigger>
          <n-button class="theater-toolbar-exit" quaternary size="small" aria-label="退出小剧场" @click="emit('exitTheater')">
            <template #icon><n-icon><ArrowLeft /></n-icon></template>
          </n-button>
        </template>
        退出小剧场
      </n-tooltip>
      <n-dropdown v-if="canManagePackages" trigger="click" :options="packageMenuOptions" :menu-props="theaterSecondaryMenuProps" @select="handlePackageMenuSelect">
        <button class="theater-stage-title is-actionable" type="button" :title="`${store.activeScene.value.name} · 导入/导出`" :aria-busy="packageBusy">
          {{ store.activeScene.value.name }}
        </button>
      </n-dropdown>
      <div v-else class="theater-stage-title" :title="store.activeScene.value.name">{{ store.activeScene.value.name }}</div>
      <n-button-group class="theater-panel-switches" size="small">
        <n-tooltip v-if="canEditAllObjects || canSwitchScene" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': scenePanelOpen }" aria-label="切换场景面板" @click="togglePanel('scene')">
              <template #icon><n-icon><LayoutSidebarLeftExpand /></n-icon></template>
            </n-button>
          </template>
          场景
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects || canEditDelegatedObjects" trigger="hover">
          <template #trigger>
            <n-button
              :class="{ 'is-active': inspectorPanelOpen }"
              :aria-label="inspectorPanelOpen ? '隐藏组件编辑面板' : '显示组件编辑面板'"
              @click="togglePanel('inspector')"
            >
              <template #icon><n-icon><Components /></n-icon></template>
            </n-button>
          </template>
          {{ inspectorPanelOpen ? '隐藏组件编辑面板' : '显示组件编辑面板' }}
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects || canEditDelegatedObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': layerPanelOpen }" aria-label="切换图层与属性面板" @click="togglePanel('layer')">
              <template #icon><n-icon><Stack2 /></n-icon></template>
            </n-button>
          </template>
          图层与属性
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects || canEditDelegatedObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': effectPanelOpen }" aria-label="切换特效层面板" @click="togglePanel('effect')">
              <template #icon><n-icon><Stars /></n-icon></template>
            </n-button>
          </template>
          特效层
        </n-tooltip>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': assetPanelOpen }" aria-label="切换素材管理器" @click="togglePanel('asset')">
              <template #icon><n-icon><Archive /></n-icon></template>
            </n-button>
          </template>
          素材管理器
        </n-tooltip>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': chatVisible }" aria-label="切换聊天区" @click="emit('toggleChat')">
              <template #icon><n-icon><Message /></n-icon></template>
            </n-button>
          </template>
          {{ chatVisible ? '隐藏聊天' : '显示聊天' }}
        </n-tooltip>
      </n-button-group>
      <span
        class="theater-sync-status"
        :class="{ 'is-online': syncReady && !syncing, 'is-syncing': syncing }"
        :title="syncing ? '正在同步' : syncReady ? '后端同步已连接' : '后端同步未连接'"
      />
      <n-tooltip trigger="hover">
        <template #trigger>
          <n-button
            class="theater-view-tool"
            :class="{ 'is-active': viewToolActive }"
            :aria-pressed="viewToolActive"
            :aria-label="viewToolActive ? '关闭查看工具' : '打开查看工具'"
            @click="toggleViewTool"
          >
            <template #icon><n-icon><Eye /></n-icon></template>
          </n-button>
        </template>
        {{ viewToolActive ? '关闭查看工具' : '查看工具' }}
      </n-tooltip>
      <span v-if="canEditAllObjects" class="theater-toolbar-divider" />
      <StageDrawingToolbar
        v-if="canEditAllObjects"
        :tool="activeCanvasTool"
        :style="drawingStyle"
        :smoothing="drawingSmoothing"
        :sides="drawingPolygonSides"
        @select="selectCanvasTool"
        @update:style="updateDrawingStyle"
        @update:smoothing="drawingSmoothing = $event"
        @update:sides="drawingPolygonSides = $event"
      />
      <n-button-group v-if="canEditAllObjects" class="theater-stage-object-actions" size="small">
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('text')"><template #icon><n-icon><LetterT /></n-icon></template></n-button></template>添加文字</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('image')"><template #icon><n-icon><Photo /></n-icon></template></n-button></template>添加图片面板</n-tooltip>
      </n-button-group>
      <StageSceneFixedToolbar
        v-if="canEditAllObjects"
        @add="type => store.addObject(type, 'scene-fixed')"
      />
      <n-button-group v-if="canEditAllObjects" class="theater-stage-object-actions" size="small">
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('button')"><template #icon><n-icon><Bolt /></n-icon></template></n-button></template>添加动作按钮</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('group')"><template #icon><n-icon><FolderPlus /></n-icon></template></n-button></template>添加组</n-tooltip>
      </n-button-group>
      <span v-if="canEditAllObjects" class="theater-toolbar-divider" />
      <n-button-group v-if="canEditAllObjects" class="theater-stage-object-actions" size="small">
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canCopy.value" aria-label="复制组件" @click="store.copySelectedObject"><template #icon><n-icon><Copy /></n-icon></template></n-button></template>复制组件 Ctrl+C</n-tooltip>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button
              class="theater-bulk-select-tool"
              :class="{ 'is-active': store.selection.bulkMode }"
              aria-label="批量选择组件"
              @click="toggleBulkSelectionMode"
            >
              <template #icon><n-icon><Select /></n-icon></template>
            </n-button>
          </template>
          批量选择组件
        </n-tooltip>
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-button
              class="theater-quick-delete-tool"
              :class="{ 'is-active': quickDeleteActive }"
              :aria-pressed="quickDeleteActive"
              :aria-label="quickDeleteActive ? '退出快速删除组件' : '启用快速删除组件'"
              @click="toggleQuickDeleteTool"
            >
              <template #icon><n-icon><Trash /></n-icon></template>
            </n-button>
          </template>
          {{ quickDeleteActive ? '退出快速删除组件 Esc' : '快速删除组件' }}
        </n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canCut.value" aria-label="剪切组件" @click="store.cutSelectedObject"><template #icon><n-icon><Cut /></n-icon></template></n-button></template>剪切组件 Ctrl+X</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canPaste.value" aria-label="粘贴组件" @click="store.pasteObject"><template #icon><n-icon><Clipboard /></n-icon></template></n-button></template>粘贴组件 Ctrl+V</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canUndo.value" aria-label="撤回组件编辑" @click="store.undo"><template #icon><n-icon><ArrowBackUp /></n-icon></template></n-button></template>撤回 Ctrl+Z</n-tooltip>
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.selectedObjects.value.length" aria-label="删除所选组件" @click="removeSelectedObjectsWithConfirm"><template #icon><n-icon><Trash /></n-icon></template></n-button></template>删除所选组件 Del / Backspace</n-tooltip>
      </n-button-group>
      <n-tooltip trigger="hover">
        <template #trigger>
          <n-button class="theater-stage-reset-camera" size="small" quaternary aria-label="复位视角" @click="store.resetCamera">
            <template #icon><n-icon><Focus /></n-icon></template>
          </n-button>
        </template>
        复位视角
      </n-tooltip>
      <span class="theater-stage-zoom">{{ Math.round(store.state.camera.zoom * 100) }}%</span>
    </header>

    <div ref="workspaceRef" class="theater-stage-workspace">
      <div
        ref="viewportRef"
        class="theater-stage-viewport"
        :class="{ 'is-viewing': viewToolActive, 'is-drawing': activeCanvasTool && activeCanvasTool !== 'eraser', 'is-erasing': activeCanvasTool === 'eraser', 'is-quick-deleting': quickDeleteActive }"
        @dragover.prevent
        @drop.prevent="handleCanvasDrop"
      >
        <div ref="containerRef" class="theater-stage-canvas" />
        <StageTextOverlay
          :class="{
            'is-scene-morph-hidden': sceneMorphTextHidden,
            'is-scene-morph-active': sceneMorphTextAnimating,
          }"
          :style="{ '--theater-scene-transition-duration': `${sceneTransitionDurationMs}ms` }"
          :objects="stageObjects"
          :camera="store.state.camera"
          :viewport-width="viewportSize.width"
          :viewport-height="viewportSize.height"
        />
        <TheaterDialogueOverlay :runtime="dialogueRuntime" :character-snapshot="characterSnapshot" :world-id="worldId" :channel-id="channelId" />
        <TheaterEffectOverlay
          :playbacks="effectPlaybacks"
          :selected-object="selectedEffectObject"
          :editing="effectPanelOpen && canEditAllObjects"
          :editing-target="effectEditingTarget"
          @transform-start="beginEffectTransform"
          @transform-update="updateEffectTransform"
          @transform-end="endEffectTransform"
          @media-transform-start="beginEffectMediaTransform"
          @media-transform-update="updateEffectMediaTransform"
          @media-transform-end="endEffectMediaTransform"
        />
        <div v-if="appearancePreview" class="theater-appearance-preview-layer">
          <TheaterPresentationPreview
            :draft="appearancePreview.draft"
            :selection="appearancePreview.selection"
            :active-section="appearancePreview.activeSection"
            :preview-enabled="true"
            :preview-name="appearancePreview.previewName"
            :preview-text="appearancePreview.previewText"
            @dispatch="(command, options) => emit('appearancePreviewCommand', command, options?.transient)"
            @gesture-start="emit('appearancePreviewPhase', 'start')"
            @gesture-end="emit('appearancePreviewPhase', 'end')"
          />
        </div>
      </div>

      <aside v-if="scenePanelOpen" class="theater-floating-panel theater-scene-rail" data-panel-id="scene" :style="panelStyle('scene')">
        <div class="theater-panel-heading" @pointerdown="startPanelDrag('scene', $event)">
          <span>场景</span>
          <div class="theater-panel-heading__actions">
            <n-tooltip v-if="canSwitchScene" trigger="hover">
              <template #trigger>
                <n-button text size="tiny" aria-label="预加载全部场景" :loading="store.scenes.value.some((scene) => scenePreloadStatus[scene.id] === 'loading')" @click="requestScenePreload(store.scenes.value.map((scene) => scene.id))"><n-icon><CloudDownload /></n-icon></n-button>
              </template>
              预加载全部场景到所有设备
            </n-tooltip>
            <n-button v-if="canEditAllObjects && canSwitchScene" text size="tiny" aria-label="新建场景" :disabled="sceneEditMode" @click="store.addScene"><n-icon><Plus /></n-icon></n-button>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭场景面板" @click="scenePanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <div
          v-for="scene in store.scenes.value"
          :key="scene.id"
          class="theater-scene-row"
          :class="{ 'has-preload-pulse': scenePreloadPulse[scene.id] }"
        >
          <n-popover
            :show="sceneEditMode && editingSceneId === scene.id"
            trigger="manual"
            placement="right-start"
            :show-arrow="false"
            :theme-overrides="theaterPopoverThemeOverrides"
            class="theater-secondary-surface"
            :style="{ width: 'min(320px, calc(100vw - 24px))' }"
          >
            <template #trigger>
              <button
                class="theater-scene-card"
                :class="{ 'is-active': scene.id === store.state.activeSceneId, 'is-editing': editingSceneId === scene.id }"
                :disabled="sceneEditMode ? !canEditAllObjects : !canSwitchScene"
                @click="handleSceneClick(scene)"
              >
                <span class="theater-scene-card__title">{{ scene.name }}</span>
              </button>
            </template>
            <div class="theater-scene-editor">
              <strong>编辑场景</strong>
              <label>
                <span>名称</span>
                <n-input v-model:value="editingSceneName" size="small" maxlength="512" />
              </label>
              <label>
                <span>场景切换文本</span>
                <n-input v-model:value="editingSceneSwitchText" type="textarea" :autosize="{ minRows: 6, maxRows: 14 }" maxlength="10000" show-count />
              </label>
              <div class="theater-scene-editor__actions">
                <n-button size="small" @click="closeSceneEditor">取消</n-button>
                <n-button size="small" type="primary" :disabled="!editingSceneName.trim()" @click="saveSceneDetails">保存</n-button>
              </div>
            </div>
          </n-popover>
          <div v-if="canSwitchScene && !sceneEditMode" class="theater-scene-row__actions">
            <n-tooltip v-if="canSwitchScene && !sceneEditMode" trigger="hover">
              <template #trigger>
                <n-button class="theater-scene-preload" :class="{ 'is-ready-pulse': scenePreloadPulse[scene.id] }" text size="tiny" :type="scenePreloadStatus[scene.id] === 'ready' ? 'success' : scenePreloadStatus[scene.id] === 'error' ? 'error' : 'default'" :loading="scenePreloadStatus[scene.id] === 'loading'" :aria-label="`预加载场景 ${scene.name}`" @click="requestScenePreload([scene.id])"><n-icon><CloudDownload /></n-icon></n-button>
              </template>
              在所有设备预加载此场景
            </n-tooltip>
          </div>
        </div>
        <div v-if="canSwitchScene" class="theater-scene-actions">
          <n-button v-if="canEditAllObjects" size="tiny" quaternary :disabled="sceneEditMode" @click="store.duplicateScene"><template #icon><n-icon><Copy /></n-icon></template>复制</n-button>
          <n-button v-if="canEditAllObjects" size="tiny" quaternary :disabled="sceneEditMode || store.scenes.value.length <= 1" @click="removeActiveSceneWithConfirm"><template #icon><n-icon><Trash /></n-icon></template>删除</n-button>
          <n-button v-if="canEditAllObjects" size="tiny" quaternary :type="sceneEditMode ? 'primary' : 'default'" :aria-pressed="sceneEditMode" @click="toggleSceneEditMode"><template #icon><n-icon><Edit /></n-icon></template>编辑</n-button>
          <label class="theater-scene-dialogue-toggle">
            <span>台词</span>
            <n-switch size="small" :value="sceneDialogueEnabled" @update:value="emit('updateSceneDialogueEnabled', $event)" />
          </label>
        </div>
      </aside>

      <aside v-if="inspectorPanelOpen" class="theater-floating-panel theater-object-inspector" data-panel-id="inspector" :style="panelStyle('inspector')">
        <template v-if="isBatchSelection">
          <div class="theater-panel-heading" @pointerdown="startPanelDrag('inspector', $event)">
            <span>批量编辑</span>
            <div class="theater-panel-heading__actions">
              <small>{{ selectedObjects.length }} 个组件</small>
              <n-button class="theater-panel-close" text size="tiny" aria-label="关闭组件编辑面板" @click="inspectorPanelOpen = false"><n-icon><X /></n-icon></n-button>
            </div>
          </div>
          <div class="theater-inspector theater-batch-inspector">
            <div class="theater-batch-summary">
              <span>{{ selectedObjects.length }} 个组件</span>
              <n-button text size="tiny" @click="store.clearSelection">清除选择</n-button>
            </div>
            <div v-if="batchMoveBlocked" class="theater-batch-warning">
              选中组件包含锁定位置项
            </div>
            <div class="theater-object-editor__checks theater-batch-checks">
              <n-checkbox
                :checked="batchBooleanChecked('visible')"
                :indeterminate="batchBooleanIndeterminate('visible')"
                @update:checked="updateBatchBoolean('visible', $event)"
              >显示</n-checkbox>
              <n-checkbox
                :checked="batchBooleanChecked('interactive')"
                :indeterminate="batchBooleanIndeterminate('interactive')"
                @update:checked="updateBatchBoolean('interactive', $event)"
              >可交互</n-checkbox>
              <n-checkbox
                :checked="batchBooleanChecked('editable')"
                :indeterminate="batchBooleanIndeterminate('editable')"
                @update:checked="updateBatchBoolean('editable', $event)"
              >可编辑</n-checkbox>
              <n-checkbox
                :checked="batchBooleanChecked('locked')"
                :indeterminate="batchBooleanIndeterminate('locked')"
                @update:checked="updateBatchBoolean('locked', $event)"
              >锁定位置</n-checkbox>
              <n-checkbox
                :checked="batchBooleanChecked('aspectRatioLocked')"
                :indeterminate="batchBooleanIndeterminate('aspectRatioLocked')"
                @update:checked="updateBatchBoolean('aspectRatioLocked', $event)"
              >锁定比例</n-checkbox>
            </div>
            <n-button secondary type="error" @click="removeSelectedObjectsWithConfirm">
              <template #icon><n-icon><Trash /></n-icon></template>
              删除所选 {{ selectedObjects.length }} 个组件
            </n-button>
          </div>
        </template>
        <template v-else-if="selectedObject">
          <div class="theater-panel-heading" @pointerdown="startPanelDrag('inspector', $event)">
            <span>组件编辑</span>
            <div class="theater-panel-heading__actions">
              <small>{{ selectedObject.type }}</small>
              <n-button class="theater-panel-close" text size="tiny" aria-label="关闭组件编辑面板" @click="inspectorPanelOpen = false"><n-icon><X /></n-icon></n-button>
            </div>
          </div>
          <div
            class="theater-inspector"
            @focusin="store.beginObjectEdit('修改对象')"
            @focusout="store.commitObjectEdit"
          >
            <label>名称</label>
            <n-input v-model:value="selectedObject.name" size="small" />
            <template v-if="selectedObject.type === 'text'">
              <label>内容</label>
              <StageTextEditor
                :model-value="selectedObject.text || ''"
                :mode="selectedTextMode"
                :can-upload-images="canUploadResources"
                @update:model-value="updateSelectedText"
                @update:mode="updateSelectedTextMode"
              />
            </template>
            <template v-else-if="selectedObject.type === 'button'">
              <label>内容</label>
              <n-input v-model:value="selectedObject.text" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" />
            </template>
            <template v-if="selectedObject.type === 'image' && canEditAllObjects">
              <label>图片</label>
              <div class="theater-image-actions">
                <n-button size="small" :disabled="!canUploadResources" :loading="resourceUploading" @click="requestImageUpload({ kind: 'object', objectId: selectedObject.id })">
                  <template #icon><n-icon><Photo /></n-icon></template>上传替换
                </n-button>
                <n-tooltip trigger="hover">
                  <template #trigger>
                    <n-button size="small" :disabled="!canUploadResources || !selectedObject.image || selectedObject.image.animated || resourceUploading" aria-label="编辑图片" @click="openImageEditor({ kind: 'object', objectId: selectedObject.id })">
                      <template #icon><n-icon><Edit /></n-icon></template>
                    </n-button>
                  </template>
                  编辑图片
                </n-tooltip>
                <n-button size="small" quaternary type="error" :disabled="!selectedObject.image" @click="clearImage({ kind: 'object', objectId: selectedObject.id })">清除</n-button>
              </div>
            </template>
            <template v-if="selectedObject.type === 'drawing' && selectedObject.drawing">
              <label>描边</label>
              <n-color-picker v-model:value="selectedObject.drawing.style.stroke" :show-alpha="false" :modes="['hex']" size="small" />
              <label>粗细</label>
              <div class="theater-drawing-inspector-row">
                <n-slider v-model:value="selectedObject.drawing.style.strokeWidth" :min="1" :max="32" :step="1" />
                <span>{{ selectedObject.drawing.style.strokeWidth }} px</span>
              </div>
              <label>透明度</label>
              <div class="theater-drawing-inspector-row">
                <n-slider
                  :value="Math.round(selectedObject.drawing.style.opacity * 100)"
                  :min="5"
                  :max="100"
                  :step="5"
                  @update:value="selectedObject.drawing.style.opacity = $event / 100"
                />
                <span>{{ Math.round(selectedObject.drawing.style.opacity * 100) }}%</span>
              </div>
              <template v-if="selectedObject.drawing.tool !== 'pen' && selectedObject.drawing.tool !== 'highlighter'">
                <label>线型</label>
                <n-select v-model:value="selectedObject.drawing.style.dash" :options="drawingDashOptions" size="small" />
              </template>
              <template v-if="['rectangle', 'ellipse', 'triangle', 'polygon'].includes(selectedObject.drawing.tool)">
                <n-checkbox
                  :checked="selectedObject.drawing.style.fill !== null"
                  @update:checked="toggleSelectedDrawingFill"
                >填充</n-checkbox>
                <n-color-picker
                  v-if="selectedObject.drawing.style.fill !== null"
                  v-model:value="selectedObject.drawing.style.fill"
                  :show-alpha="false"
                  :modes="['hex']"
                  size="small"
                />
              </template>
              <template v-if="selectedObject.drawing.tool === 'pen' || selectedObject.drawing.tool === 'highlighter'">
                <label>平滑度</label>
                <n-slider v-model:value="selectedObject.drawing.smoothing" :min="0" :max="0.8" :step="0.05" />
              </template>
              <template v-if="selectedObject.drawing.tool === 'polygon'">
                <label>边数</label>
                <n-input-number v-model:value="selectedObject.drawing.sides" :min="5" :max="12" />
              </template>
            </template>
            <template v-if="!['text', 'image', 'group', 'drawing'].includes(selectedObject.type)">
              <label>颜色</label>
              <n-input v-model:value="selectedObject.fill" />
            </template>
            <div class="theater-object-editor__transform">
              <label>X</label><n-input-number v-model:value="selectedObject.transform.x" :precision="2" />
              <label>Y</label><n-input-number v-model:value="selectedObject.transform.y" :precision="2" />
              <template v-if="selectedObject.type === 'group'">
                <label>旋转</label><n-input-number v-model:value="selectedObject.transform.rotation" :precision="2" />
                <label>缩放 X</label><n-input-number :value="selectedObject.transform.scaleX" :min="0.01" :max="100" :step="0.1" :precision="2" @update:value="updateSelectedScale('scaleX', $event)" />
                <label>缩放 Y</label><n-input-number :value="selectedObject.transform.scaleY" :min="0.01" :max="100" :step="0.1" :precision="2" @update:value="updateSelectedScale('scaleY', $event)" />
              </template>
              <template v-else>
                <label>宽</label><n-input-number :value="selectedObject.transform.width" :min="0.5" :precision="2" @update:value="updateSelectedDimension('width', $event)" />
                <label>高</label><n-input-number :value="selectedObject.transform.height" :min="0.5" :precision="2" @update:value="updateSelectedDimension('height', $event)" />
                <template v-if="selectedObject.image?.animated">
                  <label>循环次数</label><n-input-number :value="selectedObject.image.loopCount ?? null" :min="1" :max="65535" :step="1" :precision="0" clearable placeholder="无限循环" @update:value="updateSelectedLoopCount" />
                </template>
              </template>
            </div>
            <div v-if="canEditAllObjects" class="theater-object-editor__checks">
              <n-checkbox v-model:checked="selectedObject.visible">显示</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.interactive">可交互</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.editable">可编辑</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.locked">锁定位置</n-checkbox>
              <n-checkbox
                :checked="selectedObject.aspectRatioLocked"
                @update:checked="updateSelectedAspectRatioLocked"
              >锁定比例</n-checkbox>
            </div>
            <template v-if="canEditAllObjects && (selectedObject.type === 'image' || selectedObject.type === 'button')">
              <label>点击动作</label>
              <div class="theater-action-add">
                <n-button size="tiny" @click="addAction('chat.send')">发送</n-button>
                <n-button size="tiny" @click="addAction('chat.insert')">插入</n-button>
                <n-button size="tiny" @click="addAction('scene.apply')">场景</n-button>
                <n-button size="tiny" @click="addAction('object.toggle')">显隐</n-button>
              </div>
              <div v-for="action in selectedObject.actions" :key="action.id" class="theater-action-row">
                <small>{{ action.type }}</small>
                <n-input v-if="action.type === 'chat.send' || action.type === 'chat.insert'" v-model:value="action.payload.content" size="tiny" maxlength="10000" />
                <n-select v-else-if="action.type === 'scene.apply'" v-model:value="action.payload.sceneId" :options="store.scenes.value.map((scene) => ({ label: scene.name, value: scene.id }))" size="tiny" />
                <n-select v-else v-model:value="action.payload.objectId" :options="Object.values(store.activeObjects.value).map((item) => ({ label: item.name, value: item.id }))" size="tiny" />
                <n-button text type="error" size="tiny" @click="removeObjectActionWithConfirm(selectedObject.id, action.id)">删除</n-button>
              </div>
            </template>
            <template v-if="canEditAllObjects">
              <label>父级</label>
              <n-select
                :value="selectedObject.parentId"
                :options="parentOptions"
                size="small"
                clearable
                placeholder="根层级"
                @update:value="reparentObjectPreservingTransform(selectedObject.id, $event || null)"
              />
              <div class="theater-inspector-actions">
                <n-button size="tiny" @click="store.moveOrder(selectedObject.id, 1)"><template #icon><n-icon><ArrowUp /></n-icon></template>上移</n-button>
                <n-button size="tiny" @click="store.moveOrder(selectedObject.id, -1)"><template #icon><n-icon><ArrowDown /></n-icon></template>下移</n-button>
                <n-button size="tiny" :disabled="!selectedObject.parentId" @click="reparentObjectPreservingTransform(selectedObject.id, null)"><template #icon><n-icon><ArrowBackUp /></n-icon></template>移出组</n-button>
              </div>
              <small v-if="resourceError" class="theater-resource-error">{{ resourceError }}</small>
              <n-button size="small" secondary type="error" @click="removeObjectsWithConfirm([selectedObject.id])"><template #icon><n-icon><Trash /></n-icon></template>删除组件</n-button>
            </template>
          </div>
        </template>
        <template v-else>
          <div class="theater-panel-heading" @pointerdown="startPanelDrag('inspector', $event)">
            <span>组件编辑</span>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭组件编辑面板" @click="inspectorPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
          <div class="theater-panel-empty">选择幕布或图层中的组件</div>
        </template>
      </aside>

      <aside v-if="layerPanelOpen" class="theater-floating-panel theater-layer-panel" data-panel-id="layer" :style="panelStyle('layer')">
        <div class="theater-panel-heading theater-layer-panel__top-heading" @pointerdown="startPanelDrag('layer', $event)">
          <span>图层与属性</span>
          <div class="theater-panel-heading__actions">
            <small>{{ layerRows.length }}</small>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭图层与属性面板" @click="layerPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <div v-if="canEditAllObjects" class="theater-media-settings">
          <template v-for="surface in surfaceSettingRows" :key="surface.target">
            <label>{{ surface.label }}</label>
            <div class="theater-image-actions">
              <n-button size="tiny" :disabled="!canUploadResources" :loading="resourceUploading" @click="requestImageUpload({ kind: 'scene', target: surface.target })"><template #icon><n-icon><Photo /></n-icon></template>上传</n-button>
              <n-button size="tiny" quaternary :disabled="!canUploadResources || !store.state.liveState[surface.target] || store.state.liveState[surface.target]?.animated" :aria-label="`编辑${surface.label}`" @click="openImageEditor({ kind: 'scene', target: surface.target })"><template #icon><n-icon><Edit /></n-icon></template></n-button>
              <n-popover :theme-overrides="theaterPopoverThemeOverrides" class="theater-secondary-surface" trigger="click" placement="right-start" :width="300" :show-arrow="false">
                <template #trigger>
                  <n-button size="tiny" quaternary :aria-label="`设置${surface.label}`"><template #icon><n-icon><Settings /></n-icon></template></n-button>
                </template>
                <div class="theater-surface-settings">
                  <div class="theater-surface-settings__heading">{{ surface.label }}设置</div>
                  <div class="theater-surface-settings__fit">
                    <span>填充方式</span>
                    <n-radio-group :value="surfaceStyle(surface.target).fit" size="small" @update:value="updateSurfaceFit(surface.target, $event)">
                      <n-radio v-for="option in surfaceFitOptions" :key="option.value" :value="option.value">{{ option.label }}</n-radio>
                    </n-radio-group>
                  </div>
                  <div class="theater-surface-settings__slider">
                    <span>放大</span>
                    <n-slider :value="Math.round(surfaceStyle(surface.target).zoom * 100)" :min="10" :max="500" :step="1" @update:value="store.patchSceneSurfaceStyle(surface.target, { zoom: $event / 100 })" />
                    <output>{{ Math.round(surfaceStyle(surface.target).zoom * 100) }}%</output>
                  </div>
                  <div class="theater-surface-settings__slider">
                    <span>透明度</span>
                    <n-slider :value="Math.round(surfaceStyle(surface.target).opacity * 100)" :min="0" :max="100" :step="1" @update:value="updateSurfacePercentage(surface.target, 'opacity', $event)" />
                    <output>{{ Math.round(surfaceStyle(surface.target).opacity * 100) }}%</output>
                  </div>
                  <div class="theater-surface-settings__slider">
                    <span>模糊</span>
                    <n-slider :value="surfaceStyle(surface.target).blurPx" :min="0" :max="40" :step="1" @update:value="store.patchSceneSurfaceStyle(surface.target, { blurPx: $event })" />
                    <output>{{ Math.round(surfaceStyle(surface.target).blurPx) }}px</output>
                  </div>
                  <div class="theater-surface-settings__slider">
                    <span>亮度</span>
                    <n-slider :value="Math.round(surfaceStyle(surface.target).brightness * 100)" :min="0" :max="200" :step="1" @update:value="updateSurfacePercentage(surface.target, 'brightness', $event)" />
                    <output>{{ Math.round(surfaceStyle(surface.target).brightness * 100) }}%</output>
                  </div>
                  <div class="theater-surface-settings__toggle">
                    <span>颜色叠加</span>
                    <n-switch :value="surfaceStyle(surface.target).overlay.enabled" size="small" @update:value="updateSurfaceOverlay(surface.target, { enabled: $event })" />
                  </div>
                  <div class="theater-surface-settings__overlay" :class="{ 'is-disabled': !surfaceStyle(surface.target).overlay.enabled }">
                    <span>叠加颜色</span>
                    <n-color-picker :value="surfaceStyle(surface.target).overlay.color" :show-alpha="false" :disabled="!surfaceStyle(surface.target).overlay.enabled" :modes="['hex']" size="small" @update:value="updateSurfaceOverlay(surface.target, { color: $event })" />
                  </div>
                  <div class="theater-surface-settings__slider" :class="{ 'is-disabled': !surfaceStyle(surface.target).overlay.enabled }">
                    <span>叠加透明度</span>
                    <n-slider :value="Math.round(surfaceStyle(surface.target).overlay.opacity * 100)" :disabled="!surfaceStyle(surface.target).overlay.enabled" :min="0" :max="100" :step="1" @update:value="updateSurfaceOverlay(surface.target, { opacity: $event / 100 })" />
                    <output>{{ Math.round(surfaceStyle(surface.target).overlay.opacity * 100) }}%</output>
                  </div>
                  <n-button class="theater-surface-settings__reset" text size="small" @click="store.resetSceneSurfaceStyle(surface.target)">重置为默认</n-button>
                </div>
              </n-popover>
              <n-button size="tiny" quaternary type="error" :disabled="!store.state.liveState[surface.target]" @click="clearImage({ kind: 'scene', target: surface.target })">清除</n-button>
            </div>
          </template>
          <small v-if="resourceError" class="theater-resource-error">{{ resourceError }}</small>
        </div>
        <div class="theater-panel-heading"><span>层级</span></div>
        <div class="theater-layer-list">
          <button
            v-if="canEditAllObjects"
            class="theater-layer-root-drop"
            :class="{ 'is-drop-target': layerDropTarget?.id === null }"
            @dragover.prevent="layerDropTarget = { id: null, placement: 'inside' }"
            @dragleave="handleLayerDragLeave($event, null)"
            @drop.prevent="handleRootLayerDrop"
          >
            根层级
          </button>
          <div
            v-for="row in layerRows"
            :key="row.object.id"
            class="theater-layer-row"
            :class="{
              'is-active': store.selection.selectedIds.includes(row.object.id),
              'is-hidden': !row.object.visible,
              'is-disabled': !canEditObject(row.object),
              'is-drop-before': layerDropTarget?.id === row.object.id && layerDropTarget.placement === 'before',
              'is-drop-inside': layerDropTarget?.id === row.object.id && layerDropTarget.placement === 'inside',
              'is-drop-after': layerDropTarget?.id === row.object.id && layerDropTarget.placement === 'after',
            }"
            :style="{ paddingLeft: `${10 + row.depth * 15}px` }"
            @dragover.prevent="handleLayerDragOver($event, row.object.id)"
            @dragleave="handleLayerDragLeave($event, row.object.id)"
            @drop.prevent="handleLayerDrop($event, row.object.id)"
          >
            <span
              class="theater-layer-row__grip"
              :draggable="canEditAllObjects"
              @dragstart.stop="startLayerDrag($event, row.object.id)"
              @dragend.stop="finishLayerDrag"
            >
              <n-icon><GripVertical /></n-icon>
            </span>
            <button
              type="button"
              class="theater-layer-row__select"
              :disabled="!canEditObject(row.object)"
              @click="selectObject(row.object.id, store.selection.bulkMode && ($event.shiftKey || $event.ctrlKey || $event.metaKey))"
              @dblclick.stop="openObjectInspector(row.object.id)"
            >
              <span class="theater-layer-row__preview" :style="{ '--layer-preview-color': row.object.fill }">
                <img
                  v-if="layerPreviewUrl(row.object)"
                  :src="layerPreviewUrl(row.object)!"
                  :alt="`${row.object.name} 预览`"
                  draggable="false"
                  loading="lazy"
                >
                <n-icon v-else :component="layerPreviewIcon(row.object)" />
              </span>
              <span class="theater-layer-row__name">{{ row.object.name }}</span>
              <small v-if="store.isSceneFixedObject(row.object.id)">场景固定</small>
            </button>
            <div v-if="canEditAllObjects" class="theater-layer-row__actions" @pointerdown.stop>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <button
                    type="button"
                    class="theater-layer-row__action"
                    :class="{ 'is-enabled': row.object.visible }"
                    :aria-pressed="row.object.visible"
                    :aria-label="row.object.visible ? `隐藏 ${row.object.name}` : `显示 ${row.object.name}`"
                    @click.stop="toggleLayerObjectFlag(row.object, 'visible')"
                  >
                    <n-icon :component="row.object.visible ? Eye : EyeOff" />
                  </button>
                </template>
                {{ row.object.visible ? '隐藏组件' : '显示组件' }}
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <button
                    type="button"
                    class="theater-layer-row__action"
                    :class="{ 'is-enabled': row.object.editable }"
                    :aria-pressed="row.object.editable"
                    :aria-label="row.object.editable ? `禁止授权用户编辑 ${row.object.name}` : `允许授权用户编辑 ${row.object.name}`"
                    @click.stop="toggleLayerObjectFlag(row.object, 'editable')"
                  >
                    <n-icon><Edit /></n-icon>
                  </button>
                </template>
                {{ row.object.editable ? '禁止授权用户编辑' : '允许授权用户编辑' }}
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <button
                    type="button"
                    class="theater-layer-row__action"
                    :class="{ 'is-enabled': row.object.locked }"
                    :aria-pressed="row.object.locked"
                    :aria-label="row.object.locked ? `解锁 ${row.object.name} 的位置` : `锁定 ${row.object.name} 的位置`"
                    @click.stop="toggleLayerObjectFlag(row.object, 'locked')"
                  >
                    <n-icon :component="row.object.locked ? Lock : LockOpen" />
                  </button>
                </template>
                {{ row.object.locked ? '解锁位置' : '锁定位置' }}
              </n-tooltip>
            </div>
          </div>
        </div>
      </aside>

      <aside v-if="effectPanelOpen" class="theater-floating-panel theater-effect-panel" data-panel-id="effect" :style="panelStyle('effect')">
        <div class="theater-panel-heading" @pointerdown="startPanelDrag('effect', $event)">
          <span>特效层</span>
          <div class="theater-panel-heading__actions">
            <small>{{ Object.values(store.activeObjects.value).filter(isTheaterEffectObject).length }}</small>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭特效层面板" @click="effectPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <TheaterEffectPanel
          :store="store"
          :runtime="effectRuntime"
          :can-edit="canEditAllObjects"
          :can-upload="canUploadResources"
          :editing-target="effectEditingTarget"
          :audio-assets="theaterAudioAssets"
          :audio-loading="theaterAudioLoading"
          :audio-uploading="theaterAudioUploading"
          :audio-error="theaterAudioError"
          @update:editing-target="effectEditingTarget = $event"
          @upload="objectId => requestImageUpload({ kind: 'object', objectId })"
          @upload-audio="(objectId, file) => uploadTheaterAudio(file, objectId)"
        />
      </aside>

      <aside v-if="assetPanelOpen" class="theater-floating-panel theater-asset-panel" data-panel-id="asset" :style="panelStyle('asset')">
        <div class="theater-panel-heading" @pointerdown="startPanelDrag('asset', $event)">
          <span>素材管理器</span>
          <div class="theater-panel-heading__actions">
            <small>{{ theaterAudioAssets.length }}</small>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭素材管理器" @click="assetPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <TheaterAssetManager
          :assets="theaterAudioAssets"
          :quota="theaterAudioQuota"
          :loading="theaterAudioLoading"
          :uploading="theaterAudioUploading"
          :error="theaterAudioError"
          :can-upload="canUploadResources"
          :can-delete="canDeleteResources"
          :referenced-asset-ids="referencedTheaterAudioAssetIds"
          :master-volume="theaterAudioMasterVolume"
          @update:master-volume="theaterAudioMasterVolume = $event"
          @refresh="fetchTheaterAudioAssets"
          @upload="uploadTheaterAudio"
          @preview="previewTheaterAudio"
          @delete="deleteTheaterAudio"
        />
      </aside>
    </div>

    <MessageImageEditor
      v-if="imageEditorVisible"
      :show="imageEditorVisible"
      :file="imageEditorFile"
      @update:show="value => { imageEditorVisible = value }"
      @cancel="closeImageEditor"
      @confirm="saveEditedImage"
    />
  </section>
</template>

<style scoped>
.theater-stage-app {
  --theater-accent: #3b82f6;
  --theater-panel: color-mix(in srgb, var(--sc-bg-surface, #262626) 48%, transparent);
  --theater-panel-muted: color-mix(in srgb, var(--sc-bg-layer, #3f3f46) 56%, transparent);
  --theater-border: var(--sc-border-strong, rgba(255, 255, 255, .16));
  position: relative; height: 100%; min-width: 0; display: flex; flex-direction: column;
  color: var(--sc-text-primary, #f4f4f5); background: var(--sc-bg-page, #141418);
}
/* 小剧场二级浮层半透明：优先级需压过 App.vue 自定义主题对 n-popover/n-dropdown 的实色 !important */
:global(.theater-secondary-surface),
:global(:root[data-custom-theme='true'] .theater-secondary-surface),
:global(:root[data-custom-theme='true'] .n-popover.theater-secondary-surface),
:global(:root[data-custom-theme='true'] .n-dropdown-menu.theater-secondary-surface),
:global(:root[data-custom-theme='true'] .n-base-select-menu.theater-secondary-surface) {
  border: 1px solid var(--sc-border-strong, rgba(255, 255, 255, .16)) !important;
  color: var(--sc-text-primary, #f4f4f5) !important;
  background: color-mix(in srgb, var(--sc-bg-surface, #262626) 48%, transparent) !important;
  box-shadow: 0 14px 34px rgba(0, 0, 0, .2) !important;
  backdrop-filter: blur(8px) saturate(110%) !important;
  -webkit-backdrop-filter: blur(8px) saturate(110%) !important;
}
:global(.n-popover.n-popover--raw:has(.theater-secondary-surface)),
:global(.n-popover.n-popover--raw:has(.theater-secondary-surface) > .n-popover__content),
:global(:root[data-custom-theme='true'] .n-popover.n-popover--raw:has(.theater-secondary-surface)),
:global(:root[data-custom-theme='true'] .n-popover.n-popover--raw:has(.theater-secondary-surface) > .n-popover__content) {
  --n-color: transparent !important;
  background: transparent !important;
  background-color: transparent !important;
  box-shadow: none !important;
}
:global(:root[data-custom-theme='true'] .n-dropdown-menu.theater-secondary-surface .n-dropdown-option),
:global(:root[data-custom-theme='true'] .n-base-select-menu.theater-secondary-surface .n-base-select-option) {
  --n-color: transparent !important;
  background-color: transparent !important;
}
:global(:root[data-custom-theme='true'] .n-dropdown-menu.theater-secondary-surface .n-dropdown-option:hover),
:global(:root[data-custom-theme='true'] .n-dropdown-menu.theater-secondary-surface .n-dropdown-option--pending),
:global(:root[data-custom-theme='true'] .n-base-select-menu.theater-secondary-surface .n-base-select-option--pending),
:global(:root[data-custom-theme='true'] .n-base-select-menu.theater-secondary-surface .n-base-select-option:hover) {
  background-color: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)) !important;
}
.theater-image-input { display: none; }
.theater-stage-toolbar {
  position: absolute; z-index: 20; top: 0; right: 0; left: 0; box-sizing: border-box;
  height: 46px; display: flex; align-items: center; gap: 7px; padding: 0 8px;
  overflow-x: auto; overflow-y: hidden; border-bottom: 1px solid transparent;
  background: transparent; box-shadow: none; scrollbar-width: none;
  transition: background-color .18s ease, border-color .18s ease, box-shadow .18s ease;
}
.theater-stage-toolbar.is-controls-visible {
  border-bottom-color: var(--sc-border-mute, rgba(255, 255, 255, .08));
  background: color-mix(in srgb, var(--sc-bg-header, #262626) 92%, transparent);
  box-shadow: 0 5px 18px rgba(0, 0, 0, .2);
  backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px);
}
.theater-stage-toolbar::-webkit-scrollbar { display: none; }
.theater-stage-toolbar :deep(.n-button) {
  transition: color .18s ease, background-color .18s ease, border-color .18s ease, box-shadow .18s ease;
}
.theater-stage-toolbar:not(.is-controls-visible) :deep(.n-button:not(:disabled)) {
  --n-color: transparent !important;
  --n-color-hover: transparent !important;
  --n-color-pressed: transparent !important;
  --n-color-focus: transparent !important;
  --n-border: 1px solid transparent !important;
  --n-border-hover: 1px solid transparent !important;
  --n-border-pressed: 1px solid transparent !important;
  --n-border-focus: 1px solid transparent !important;
  --n-text-color: rgba(255, 255, 255, .92) !important;
  --n-text-color-hover: #fff !important;
  --n-text-color-pressed: #fff !important;
  --n-text-color-focus: #fff !important;
  color: rgba(255, 255, 255, .92) !important;
  background: transparent !important;
  border-color: transparent !important;
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, .72));
}
.theater-stage-toolbar:not(.is-controls-visible) :deep(.n-button.is-active:not(:disabled)) {
  box-shadow: inset 0 -2px rgba(255, 255, 255, .82) !important;
}
.theater-toolbar-exit, .theater-bulk-select-tool, .theater-quick-delete-tool, .theater-panel-switches, .theater-stage-object-actions { flex: 0 0 auto; }
.theater-stage-title {
  width: 8em; flex: 0 0 8em; overflow: hidden; color: var(--sc-text-primary, #f4f4f5);
  font-size: 15px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap;
}
.theater-stage-title.is-actionable {
  height: 34px; padding: 0; border: 0; border-radius: 5px; background: transparent; text-align: left; cursor: pointer;
}
.theater-stage-title.is-actionable:hover, .theater-stage-title.is-actionable:focus-visible {
  color: #fff; text-decoration: underline; text-underline-offset: 4px; outline: none;
}
.theater-panel-switches :deep(.n-button), .theater-stage-object-actions :deep(.n-button) { width: 34px; padding: 0; }
.theater-bulk-select-tool.is-active, .theater-panel-switches :deep(.n-button.is-active) {
  color: #fff; background: var(--theater-accent); border-color: var(--theater-accent);
}
.theater-quick-delete-tool.is-active { color: #fff; background: #dc2626; border-color: #dc2626; }
.theater-toolbar-divider { width: 1px; height: 22px; flex: 0 0 1px; margin: 0 2px; background: var(--theater-border); }
.theater-sync-status { width: 7px; height: 7px; flex: 0 0 7px; border-radius: 50%; background: var(--sc-fg-muted, #71717a); }
.theater-sync-status.is-online { background: #22c55e; box-shadow: 0 0 0 3px rgba(34, 197, 94, .12); }
.theater-sync-status.is-syncing { background: #f59e0b; box-shadow: 0 0 0 3px rgba(245, 158, 11, .12); }
.theater-stage-character-bridge {
  width: 218px; flex: 0 0 218px; display: grid; grid-template-columns: 28px minmax(0, 1fr); align-items: center; gap: 6px;
  padding: 3px 6px; border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); border-radius: 6px;
  background: var(--theater-panel-muted);
}
.theater-stage-character-bridge.is-offline { opacity: .52; }
.theater-stage-character-bridge img, .theater-stage-character-bridge__placeholder { width: 28px; height: 28px; border-radius: 5px; object-fit: cover; }
.theater-stage-character-bridge__placeholder { display: grid; place-items: center; color: var(--sc-text-secondary, #b5b5c5); background: var(--sc-bg-input, #3f3f46); font-size: 11px; }
.theater-stage-character-bridge__selects { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 4px; }
.theater-stage-character-bridge small { display: none; }
.theater-stage-reset-camera { flex: 0 0 auto; }
.theater-stage-zoom { width: 38px; flex: 0 0 38px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; text-align: right; }
.theater-stage-workspace { position: relative; min-height: 0; flex: 1; overflow: hidden; }
.theater-stage-viewport { position: absolute; inset: 0; min-width: 0; min-height: 0; overflow: hidden; background: #343435; }
.theater-stage-viewport :deep(.theater-text-overlay.is-scene-morph-hidden) { opacity: 0; }
.theater-stage-viewport :deep(.theater-text-overlay.is-scene-morph-active) {
  transition: opacity var(--theater-scene-transition-duration, 400ms) ease-in-out;
}
.theater-appearance-preview-layer { position: absolute; z-index: 8; inset: 0; overflow: hidden; }
.theater-appearance-preview-layer :deep(.theater-preview) { min-height: 0; background: transparent; }
.theater-stage-viewport.is-drawing :deep(canvas) { cursor: crosshair !important; }
.theater-stage-viewport.is-viewing :deep(canvas) { cursor: grab !important; }
.theater-stage-viewport.is-erasing :deep(canvas) { cursor: cell !important; }
.theater-stage-viewport.is-quick-deleting :deep(canvas) { cursor: crosshair !important; }
.theater-stage-canvas { position: absolute; inset: 0; }
.theater-floating-panel {
  position: absolute; z-index: 10; box-sizing: border-box; display: flex; flex-direction: column; min-height: 0; overflow: hidden;
  border: 1px solid var(--theater-border); border-radius: 7px; background: var(--theater-panel);
  box-shadow: 0 14px 34px rgba(0, 0, 0, .2); backdrop-filter: blur(8px) saturate(110%); -webkit-backdrop-filter: blur(8px) saturate(110%);
  resize: both; max-width: 100%; max-height: 100%; animation: theater-panel-in .16s ease-out;
}
@keyframes theater-panel-in { from { opacity: 0; transform: translateY(-4px); } }
.theater-scene-rail { min-width: min(124px, 100%); min-height: min(160px, 100%); gap: 6px; padding: 6px; overflow-y: auto; }
.theater-object-inspector { min-width: min(240px, 100%); min-height: min(240px, 100%); overflow-y: auto; }
.theater-layer-panel { min-width: min(280px, 100%); min-height: min(220px, 100%); }
.theater-effect-panel { min-width: min(320px, 100%); min-height: min(320px, 100%); }
.theater-asset-panel { min-width: min(320px, 100%); min-height: min(280px, 100%); }
.theater-panel-heading {
  height: 32px; flex: 0 0 32px; display: flex; align-items: center; justify-content: space-between; padding: 0 8px;
  color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; font-weight: 700; cursor: move; user-select: none; touch-action: none;
}
.theater-panel-heading__actions { display: flex; align-items: center; gap: 3px; }
.theater-panel-heading small { font-weight: 400; }
.theater-panel-close { color: var(--sc-text-secondary, #b5b5c5); }
.theater-panel-empty { padding: 28px 16px; color: var(--sc-text-secondary, #b5b5c5); font-size: 12px; text-align: center; }
.theater-scene-row { position: relative; width: 100%; display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 2px; }
.theater-scene-row__actions { display: flex; align-items: center; gap: 2px; min-width: 0; }
.theater-scene-row__actions {
  position: absolute; right: 0; padding-left: 6px; opacity: 0; pointer-events: none;
  background: linear-gradient(90deg, transparent, var(--theater-panel) 10px); transition: opacity .14s ease;
}
.theater-scene-row:hover .theater-scene-row__actions, .theater-scene-row:has(button:focus-visible) .theater-scene-row__actions, .theater-scene-row.has-preload-pulse .theater-scene-row__actions { opacity: 1; pointer-events: auto; }
.theater-scene-row:hover .theater-scene-card, .theater-scene-row:has(button:focus-visible) .theater-scene-card, .theater-scene-row.has-preload-pulse .theater-scene-card { padding-right: 36px; }
.theater-scene-card {
  width: 100%; display: flex; align-items: center; min-height: 34px; padding: 7px 8px; border: 1px solid transparent; border-radius: 6px;
  color: var(--sc-text-secondary, #b5b5c5); background: transparent; font-size: 12px; line-height: 1.2; text-align: left; cursor: pointer;
  transition: color .14s ease, border-color .14s ease, background .14s ease;
}
.theater-scene-card:hover { color: var(--sc-text-primary, #f4f4f5); background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-scene-card.is-active { color: var(--sc-text-primary, #f4f4f5); border-color: color-mix(in srgb, var(--theater-accent) 70%, transparent); background: color-mix(in srgb, var(--theater-accent) 16%, transparent); }
.theater-scene-card.is-editing { outline: 1px solid var(--theater-accent); outline-offset: -2px; }
.theater-scene-card__title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-scene-preload { width: 28px; height: 28px; padding: 0; }
.theater-scene-preload.is-ready-pulse { animation: theater-scene-preload-ready .42s ease-out; }
@keyframes theater-scene-preload-ready {
  0%, 100% { transform: translateY(0) scale(1); }
  42% { transform: translateY(-4px) scale(1.14); }
  68% { transform: translateY(1px) scale(.96); }
}
@media (prefers-reduced-motion: reduce) { .theater-scene-preload.is-ready-pulse { animation: none; } }
.theater-scene-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 1px; margin-top: auto; }
.theater-scene-dialogue-toggle { display: flex; align-items: center; gap: 5px; margin-left: auto; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-scene-editor { display: grid; gap: 10px; }
.theater-scene-editor strong { font-size: 13px; }
.theater-scene-editor label { display: grid; gap: 5px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-scene-editor__actions { display: flex; justify-content: flex-end; gap: 6px; }
.theater-object-editor__transform { display: grid; grid-template-columns: auto minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 6px 8px; }
.theater-object-editor__transform label { color: var(--sc-text-secondary, #b5b5c5); font-size: 12px; }
.theater-object-editor__checks { display: flex; flex-wrap: wrap; gap: 10px 14px; padding-top: 2px; }
.theater-batch-inspector { gap: 12px; }
.theater-batch-summary { display: flex; align-items: center; justify-content: space-between; color: var(--sc-text-secondary, #b5b5c5); font-size: 12px; }
.theater-batch-warning { padding: 7px 8px; border: 1px solid color-mix(in srgb, #f59e0b 42%, transparent); border-radius: 6px; color: #fbbf24; background: color-mix(in srgb, #f59e0b 10%, transparent); font-size: 11px; }
.theater-batch-checks { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.theater-media-settings { display: grid; gap: 5px; padding: 9px; border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-media-settings label, .theater-inspector label { color: var(--sc-fg-muted, #71717a); font-size: 10px; }
.theater-image-actions { display: flex; align-items: center; gap: 4px; }
.theater-surface-settings { width: 100%; min-width: 0; max-width: 100%; box-sizing: border-box; display: grid; gap: 11px; overflow: hidden; }
.theater-surface-settings > * { min-width: 0; }
.theater-surface-settings__heading { color: var(--sc-text-primary, #f4f4f5); font-size: 13px; font-weight: 700; }
.theater-surface-settings__fit { display: grid; gap: 7px; }
.theater-surface-settings__fit > span, .theater-surface-settings__slider > span, .theater-surface-settings__toggle > span, .theater-surface-settings__overlay > span {
  color: var(--sc-text-secondary, #b5b5c5); font-size: 11px;
}
.theater-surface-settings__fit :deep(.n-radio-group) {
  width: 100%; min-width: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 7px 18px;
}
.theater-surface-settings__fit :deep(.n-radio) { min-width: 0; margin: 0; }
.theater-surface-settings__fit :deep(.n-radio__label) { min-width: 0; padding-left: 6px; font-size: 11px; }
.theater-surface-settings__slider { min-height: 24px; display: grid; grid-template-columns: 62px minmax(0, 1fr) 42px; align-items: center; gap: 8px; }
.theater-surface-settings__slider > *, .theater-surface-settings__toggle > *, .theater-surface-settings__overlay > * { min-width: 0; }
.theater-surface-settings__slider output { color: var(--sc-text-primary, #f4f4f5); font-size: 11px; font-variant-numeric: tabular-nums; text-align: right; }
.theater-surface-settings__toggle, .theater-surface-settings__overlay { display: grid; grid-template-columns: 86px minmax(0, 1fr); align-items: center; gap: 8px; }
.theater-surface-settings__overlay :deep(.n-color-picker) { width: 100%; min-width: 0; }
.theater-surface-settings .is-disabled { opacity: .48; }
.theater-surface-settings__reset { justify-self: start; color: var(--sc-text-secondary, #b5b5c5); }
.theater-resource-error { color: #f87171; font-size: 10px; line-height: 1.3; }
.theater-layer-list { min-height: 100px; flex: 1; overflow: auto; padding: 4px 0; }
.theater-layer-root-drop {
  width: calc(100% - 12px); height: 25px; margin: 2px 6px 5px; border: 1px dashed var(--sc-border-mute, rgba(255, 255, 255, .16));
  border-radius: 5px; color: var(--sc-fg-muted, #71717a); background: transparent; font-size: 10px; cursor: default;
}
.theater-layer-root-drop.is-drop-target { border-color: #38bdf8; color: #7dd3fc; background: rgba(56, 189, 248, .1); }
.theater-layer-row {
  position: relative; box-sizing: border-box; width: 100%; height: 38px; display: flex; align-items: center; gap: 5px;
  color: var(--sc-text-primary, #f4f4f5); background: transparent; font-size: 12px; text-align: left;
  transition: color .14s ease, background .14s ease;
}
.theater-layer-row:hover { background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-layer-row.is-active { color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--theater-accent) 18%, transparent); }
.theater-layer-row.is-disabled .theater-layer-row__select { cursor: default; }
.theater-layer-row.is-hidden .theater-layer-row__preview,
.theater-layer-row.is-hidden .theater-layer-row__name { opacity: .46; }
.theater-layer-row.is-drop-inside { outline: 1px solid #38bdf8; outline-offset: -2px; background: rgba(56, 189, 248, .12); }
.theater-layer-row.is-drop-before::before, .theater-layer-row.is-drop-after::after {
  position: absolute; right: 5px; left: 5px; height: 2px; border-radius: 1px; background: #38bdf8; content: '';
}
.theater-layer-row.is-drop-before::before { top: 0; }
.theater-layer-row.is-drop-after::after { bottom: 0; }
.theater-layer-row__grip { width: 16px; height: 100%; flex: 0 0 16px; display: grid; place-items: center; color: var(--sc-fg-muted, #71717a); font-size: 14px; cursor: grab; }
.theater-layer-row__grip:active { cursor: grabbing; }
.theater-layer-row__select {
  min-width: 0; height: 100%; flex: 1; display: flex; align-items: center; gap: 7px; padding: 0; border: 0;
  color: inherit; background: transparent; font: inherit; text-align: left; cursor: pointer;
}
.theater-layer-row__select:focus-visible { outline: 2px solid var(--theater-accent); outline-offset: -2px; }
.theater-layer-row__preview {
  --layer-preview-color: var(--sc-bg-input, #3f3f46);
  width: 26px; height: 26px; flex: 0 0 26px; display: grid; place-items: center; overflow: hidden;
  border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .12)); border-radius: 4px;
  color: var(--sc-text-secondary, #b5b5c5); background: color-mix(in srgb, var(--layer-preview-color) 38%, var(--sc-bg-input, #3f3f46));
  font-size: 15px; transition: opacity .14s ease;
}
.theater-layer-row__preview img { width: 100%; height: 100%; display: block; object-fit: cover; }
.theater-layer-row__name { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-layer-row small { flex: 0 0 auto; color: #eab308; font-size: 9px; }
.theater-layer-row__actions { height: 100%; flex: 0 0 auto; display: flex; align-items: center; gap: 1px; padding-right: 5px; }
.theater-layer-row__action {
  width: 24px; height: 24px; display: grid; place-items: center; padding: 0; border: 0; border-radius: 4px;
  color: var(--sc-fg-muted, #71717a); background: transparent; font-size: 14px; cursor: pointer;
  transition: color .14s ease, background .14s ease;
}
.theater-layer-row__action:hover { color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--sc-text-primary, #f4f4f5) 9%, transparent); }
.theater-layer-row__action.is-enabled { color: var(--sc-text-primary, #f4f4f5); }
.theater-layer-row__action:focus-visible { outline: 2px solid var(--theater-accent); outline-offset: 1px; }
.theater-inspector { display: grid; gap: 8px; padding: 10px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-drawing-inspector-row { display: grid; grid-template-columns: minmax(0, 1fr) 42px; align-items: center; gap: 8px; }
.theater-drawing-inspector-row span { color: var(--sc-fg-muted, #71717a); font-size: 10px; text-align: right; }
.theater-inspector-actions, .theater-action-add { display: flex; flex-wrap: wrap; gap: 4px; }
.theater-action-row { display: grid; gap: 4px; padding: 6px; border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); border-radius: 6px; }
.theater-action-row small { color: var(--sc-text-secondary, #b5b5c5); font-size: 9px; }
@media (max-width: 1100px) {
  .theater-stage-toolbar { gap: 5px; padding: 0 6px; }
  .theater-stage-character-bridge { width: 176px; flex-basis: 176px; }
}
@media (max-width: 720px) {
  .theater-stage-title { width: 6em; flex-basis: 6em; }
  .theater-stage-reset-camera { width: 34px; padding: 0; font-size: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .theater-stage-toolbar, .theater-stage-toolbar :deep(.n-button), .theater-floating-panel { transition: none; }
  .theater-floating-panel { animation: none; }
}
</style>
