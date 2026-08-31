<script setup lang="ts">
import Konva from 'konva'
import { Howl, Howler } from 'howler'
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { NBadge, NButton, NButtonGroup, NCheckbox, NColorPicker, NDropdown, NIcon, NInput, NInputNumber, NModal, NPopover, NProgress, NRadio, NRadioGroup, NSelect, NSlider, NSwitch, NTooltip, useDialog, useMessage, type DropdownOption } from 'naive-ui'
import {
  ArrowBackUp,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  AspectRatio,
  Archive,
  Bolt,
  BoltOff,
  Clipboard,
  ChevronDown,
  ChevronRight,
  Components,
  CloudDownload,
  CloudRain,
  Copy,
  Cut,
  Dots,
  Edit,
  Eye,
  EyeOff,
  Filter,
  Folder,
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
  Pin,
  Plus,
  PlayerPlay,
  Search,
  Select,
  Settings,
  Stars,
  Stack2,
  Trash,
  Upload,
  X,
} from '@vicons/tabler'
import { api, urlBase } from '@/stores/_config'
import { getUploadTimeoutMs } from '@/utils/uploadTimeout'
import { useAudioStudioStore } from '@/stores/audioStudio'
import { compressImage } from '@/composables/useImageCompressor'
import type { AudioAsset, AudioQuotaSummary } from '@/types/audio'
import {
  WORLD_UNIT_PX,
  STAGE_ACTION_DELAY_STEP_MS,
  STAGE_ACTION_MAX_DELAY_MS,
  STAGE_ENTRANCE_MAX_DURATION_MS,
  createDefaultStageActionSchedule,
  createDefaultStageImageAnnotation,
  normalizeStageImageAnnotation,
  normalizeStageAudioRef,
  normalizeStageMusicSnapshot,
  normalizeStageEntranceConfig,
  normalizeStageSceneTransition,
  stageSceneTransitionTypes,
  type StageEntranceConfig,
  type StageEntrancePlayback,
  type StageEntrancePreset,
  isStageActionTarget,
  type StageAction,
  type StageActionTriggeredPayload,
  type StageAudioRef,
  type StageDrawing,
  type StageDrawingStyle,
  type StageDrawingTool,
  type StageImageRef,
  type StageImageAnnotation,
  type StageMusicPlaylistMode,
  type StageMusicTrackType,
  type StageObject,
  type StageObjectFit,
  type StagePointerTrace,
  type StagePointerTraceInput,
  type SceneFolder,
  type StageScene,
  type StageSceneTransition,
  type StageSceneTransitionPhase,
  type StageSceneTransitionType,
  type StageSurfaceFit,
  type StageSurfaceStyle,
  type StageSurfaceTarget,
} from '../shared/stage-types'
import { stageActionSchema, type ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { syncStageObjectHierarchy } from './stage-layering'
import { compareStageLayersBottomToTop, compareStageLayersTopToBottom } from './stage-layer-order'
import { buildStageLayerRows, stageLayerSelectionExpansionIds } from './stage-layer-tree'
import { stageSelectionRootIds } from './stage-selection'
import { stageSceneTransitionKeyframes, stageSceneTransitionOptions } from './stage-scene-transition'
import { snapStageResizeBox, type StageGridResizeSession } from './stage-grid-resize'
import {
  resolveTheaterStageMediaLocation,
  theaterResourceContentPath,
  theaterResourcePath as buildTheaterResourcePath,
  type TheaterStageMediaLocation,
} from './stage-media'
import StageDrawingToolbar, { type StageCanvasTool } from './StageDrawingToolbar.vue'
import StageCopyToolbar from './StageCopyToolbar.vue'
import StageGridToolbar from './StageGridToolbar.vue'
import StageSceneFixedToolbar from './StageSceneFixedToolbar.vue'
import { cloneStageData, type StageCopyMode } from './stage-editing'
import StageTextEditor, { type StageTextEditorMode } from './StageTextEditor.vue'
import StageTextOverlay from './StageTextOverlay.vue'
import StageImageAnnotationEditor from './StageImageAnnotationEditor.vue'
import TheaterActionSequenceEditor from './TheaterActionSequenceEditor.vue'
import TheaterRandomTableEditor from './TheaterRandomTableEditor.vue'
import type { TheaterStageStore } from './StageStore'
import { createStageSequenceAction, isStageSequenceAction } from '../shared/stage-actions'
import { resolveTheaterReducedMotion } from '../shared/theater-reduced-motion'
import TheaterDialogueOverlay from '../dialogue/TheaterDialogueOverlay.vue'
import TheaterCharacterStatsOverlay from './TheaterCharacterStatsOverlay.vue'
import type { TheaterDialogueRuntime } from '../dialogue/theater-dialogue-runtime'
import type { TheaterChatBridgeStatus } from '../bridge/TheaterHostBridge'
import type { TheaterEditorCommand, TheaterSection, TheaterSelection } from '@/components/theater-presentation/theaterPresentationEditorState'
import type { TheaterPresentation } from '@/types/theaterPresentation'
import TheaterPresentationPreview from '@/components/theater-presentation/TheaterPresentationPreview.vue'
import TheaterEffectOverlay from '../effects/TheaterEffectOverlay.vue'
import SceneOverlayStageHost from '../overlays/SceneOverlayStageHost.vue'
import { TheaterEffectRuntime, type TheaterEffectPlayback } from '../effects/theater-effect-runtime'
import { isTheaterEffectObject, setTheaterEffectConfig, theaterEffectConfigFromObject } from '../effects/theater-effect-types'
import {
  emptyTheaterPanelOrganizer,
  type TheaterPanelDomain,
  type TheaterPanelFolder,
  type TheaterPanelOrganizerSnapshot,
} from '../effects/theater-panel-organizer'
import {
  applyImageObjectPreset,
  resolveImageObjectPreset,
  type TheaterImageFolderPreset,
  type TheaterImageObjectPreset,
} from '../effects/theater-image-folder-preset'
import { THEATER_IMAGE_ASSET_DRAG_TYPE, type TheaterImageAsset } from '../effects/theater-image-assets'

const sceneOverlayImageFolderName = '场景叠加'

const props = defineProps<{
  store: TheaterStageStore
  worldId: string
  channelId: string
  scopeType?: 'channel' | 'world'
  characterSnapshot: ChatCharactersSnapshotPayload
  chatBridgeOnline: boolean
  chatBridgeStatus: TheaterChatBridgeStatus
  chatVisible: boolean
  syncReady: boolean
  syncing: boolean
  permissions: string[]
  constructionSceneId: string | null
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
  sceneAudioEnabled: boolean
  syncBeforeOrganizerWrite: () => Promise<void>
}>()
const emit = defineEmits<{
  actionTriggered: [payload: StageActionTriggeredPayload]
  pointerTrace: [trace: StagePointerTraceInput]
  selectCharacter: [identityId: string]
  selectCharacterVariant: [payload: { identityId: string, variantId: string | null }]
  openCharacterCard: [identityId: string]
  toggleChat: []
  disconnectChatBridge: []
  reconnectChatBridge: []
  resetLayout: []
  exitTheater: []
  appearancePreviewCommand: [command: TheaterEditorCommand, transient?: boolean]
  appearancePreviewPhase: [phase: 'start' | 'end']
  preloadRequested: [sceneIds: string[]]
  sceneSwitchRequested: [sceneId: string]
  constructionSceneChangeRequested: [sceneId: string | null]
  sceneMusicRecordRequested: [sceneId: string]
  sceneMusicClearRequested: [sceneId: string]
  updateSceneDialogueEnabled: [enabled: boolean]
  updateSceneAudioEnabled: [enabled: boolean]
}>()

const chatBridgeStatusLabel = computed(() => {
  switch (props.chatBridgeStatus) {
    case 'connected': return '已连接'
    case 'connecting': return '连接中'
    case 'reconnecting': return '重连中'
    case 'manual-disconnected': return '已手动断开'
    case 'error': return '连接异常'
    default: return '未连接'
  }
})

const stageActionDescriptions: Record<StageAction['type'], string> = {
  'chat.send': '发送消息',
  'chat.random-table': '随机表',
  'chat.insert': '插入输入框',
  'scene.apply': '切换场景',
  'effect.play': '触发特效',
  'object.toggle': '显隐切换',
  'action.sequence': '组合动作',
}

const containerRef = ref<HTMLDivElement | null>(null)
const viewportRef = ref<HTMLDivElement | null>(null)
const sceneVisualRef = ref<HTMLDivElement | null>(null)
const sceneMorphContainerRef = ref<HTMLDivElement | null>(null)
const viewportSize = ref({ width: 1, height: 1 })
const selectionQuickBar = reactive({ visible: false, left: 0, top: 0 })
const imageInputRef = ref<HTMLInputElement | null>(null)
const sceneAudioInputRef = ref<HTMLInputElement | null>(null)
const packageInputRef = ref<HTMLInputElement | null>(null)
const ccfoliaInputRef = ref<HTMLInputElement | null>(null)
const resourceError = ref('')
const resourceUploading = ref(false)
const scenePanelOpen = ref(false)
const inspectorPanelOpen = ref(false)
const imageAnnotationEditorObjectId = ref<string | null>(null)
const imageAnnotationEditorVisible = ref(false)
const imageAnnotationOverlayRef = ref<HTMLDivElement | null>(null)
const imageAnnotationOverlay = reactive({
  visible: false,
  objectId: '',
  left: 0,
  top: 0,
  pointerX: 0,
  pointerY: 0,
  annotation: createDefaultStageImageAnnotation(),
})
let imageAnnotationTimer: number | null = null
let imageAnnotationPendingObjectId = ''
const layerPanelOpen = ref(false)
const effectPanelOpen = ref(false)
const overlayPanelOpen = ref(false)
const assetPanelOpen = ref(false)
const effectEditingTarget = ref<'frame' | 'media'>('frame')
const toolbarColorsVisible = ref(false)
const MessageImageEditor = defineAsyncComponent(() => import('@/components/chat/MessageImageEditor.vue'))
const TheaterEffectPanel = defineAsyncComponent(() => import('../effects/TheaterEffectPanel.vue'))
const SceneOverlayManagerPanel = defineAsyncComponent(() => import('../overlays/SceneOverlayManagerPanel.vue'))
const TheaterAssetManager = defineAsyncComponent(() => import('../effects/TheaterAssetManager.vue'))
const effectPlaybacks = ref<TheaterEffectPlayback[]>([])
const audioStudio = useAudioStudioStore()
const theaterAudioAssets = ref<AudioAsset[]>([])
const theaterAudioQuota = ref<AudioQuotaSummary | null>(null)
const theaterAudioLoading = ref(false)
const theaterAudioUploading = ref(false)
const theaterAudioError = ref('')
const theaterImageAssets = ref<TheaterImageAsset[]>([])
const theaterImageLoading = ref(false)
const theaterImageUploading = ref(false)
const theaterImageError = ref('')
let theaterImageFetchGeneration = 0
let theaterImageUploadGeneration = 0
const theaterPanelOrganizer = ref<TheaterPanelOrganizerSnapshot>(emptyTheaterPanelOrganizer())
const duplicatingScene = ref(false)
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
const packageProgressVisible = ref(false)
const packageProgressJob = ref<TheaterPackageJob | null>(null)
const packageProgressError = ref('')
const packageDisplayedProgress = ref(0)
const packageDisplayedDone = ref(0)
const packageTargetProgress = ref(0)
const packageTargetDone = ref(0)
let packageDisplayAnimation = 0
let packageDisplayLoopRunning = false
let packageDisplayLoopAnimation = 0
let packagePollTimer: number | null = null
let packagePollGeneration = 0

type TheaterPackageJob = {
  id: string
  type: 'export' | 'export_effects' | 'import' | 'import_ccfolia'
  status: 'pending' | 'running' | 'done' | 'failed'
  progress: number
  progressDone?: number
  progressTotal?: number
  progressStage?: string
  outputFileName?: string
  errorMessage?: string
  summary?: { packageKind?: 'theater' | 'effects', effects?: number, scenes?: number, objects?: number, resources?: number, audioAssets?: number, animatedResources?: number, warnings?: string[] }
}

const canManagePackages = computed(() => props.syncReady && props.permissions.includes('stage.admin.restore'))
const packageMenuOptions = computed<DropdownOption[]>(() => [
  { label: packageBusy.value ? '任务处理中…' : '导出小剧场 ZIP', key: 'export', disabled: packageBusy.value },
  { label: '导出特效包 ZIP', key: 'export-effects', disabled: packageBusy.value },
  { label: '导入小剧场 ZIP', key: 'import', disabled: packageBusy.value },
  { label: '导入 CCFOLIA ZIP', key: 'import-ccfolia', disabled: packageBusy.value },
])

const theaterPackagePath = (suffix: string) => `api/v1/worlds/${encodeURIComponent(props.worldId)}/theater/packages/${suffix}`

type TheaterRequestScope = {
  worldId: string
  channelId: string
  scopeType?: 'channel' | 'world'
}

const captureTheaterRequestScope = (): TheaterRequestScope => ({
  worldId: props.worldId,
  channelId: props.channelId,
  scopeType: props.scopeType,
})

const isCurrentTheaterRequestScope = (scope: TheaterRequestScope) => (
  scope.worldId === props.worldId
  && scope.channelId === props.channelId
  && scope.scopeType === props.scopeType
)

const theaterEditorStatePath = (objectId = '') => {
  const base = props.scopeType === 'world'
    ? `api/v1/worlds/${encodeURIComponent(props.worldId)}/theater/editor-state/groups`
    : `api/v1/worlds/${encodeURIComponent(props.worldId)}/channels/${encodeURIComponent(props.channelId)}/theater/editor-state/groups`
  return objectId ? `${base}/${encodeURIComponent(objectId)}` : base
}

const theaterPanelOrganizerPath = (suffix = '', scope = captureTheaterRequestScope()) => {
  const base = scope.scopeType === 'world'
    ? `api/v1/worlds/${encodeURIComponent(scope.worldId)}/theater/panel-organizer`
    : `api/v1/worlds/${encodeURIComponent(scope.worldId)}/channels/${encodeURIComponent(scope.channelId)}/theater/panel-organizer`
  return suffix ? `${base}/${suffix}` : base
}

const theaterImageAssetPath = (assetId = '', scope = captureTheaterRequestScope()) => {
  const base = scope.scopeType === 'world'
    ? `api/v1/worlds/${encodeURIComponent(scope.worldId)}/theater/image-assets`
    : `api/v1/worlds/${encodeURIComponent(scope.worldId)}/channels/${encodeURIComponent(scope.channelId)}/theater/image-assets`
  return assetId ? `${base}/${encodeURIComponent(assetId)}` : base
}

const stopPackagePolling = () => {
  packagePollGeneration += 1
  if (packagePollTimer !== null) window.clearTimeout(packagePollTimer)
  packagePollTimer = null
}

const waitPackagePoll = (generation: number) => new Promise<boolean>((resolve) => {
  packagePollTimer = window.setTimeout(() => {
    packagePollTimer = null
    resolve(generation === packagePollGeneration)
  }, 300)
})

const setPackageProgressTarget = (job: TheaterPackageJob) => {
  packageTargetProgress.value = Math.max(0, Math.min(1, job.progress || 0))
  packageTargetDone.value = Math.max(0, job.progressDone || 0)
  if (packageDisplayLoopRunning && packageDisplayLoopAnimation === packageDisplayAnimation) return
  packageDisplayLoopRunning = true
  const animation = packageDisplayAnimation
  packageDisplayLoopAnimation = animation
  const run = async () => {
    while (animation === packageDisplayAnimation && (packageDisplayedDone.value < packageTargetDone.value || packageDisplayedProgress.value < packageTargetProgress.value - 0.001)) {
      if (packageDisplayedDone.value < packageTargetDone.value) packageDisplayedDone.value += 1
      const delta = packageTargetProgress.value - packageDisplayedProgress.value
      packageDisplayedProgress.value += Math.abs(delta) < 0.01 ? delta : delta * 0.12
      await new Promise(resolve => window.setTimeout(resolve, 35))
    }
    if (animation === packageDisplayAnimation) {
      packageDisplayedDone.value = packageTargetDone.value
      packageDisplayedProgress.value = packageTargetProgress.value
    }
    if (packageDisplayLoopAnimation === animation) packageDisplayLoopRunning = false
  }
  void run()
}

const waitPackageProgressDisplayed = async () => {
  while (packageDisplayedDone.value < packageTargetDone.value || packageDisplayedProgress.value < packageTargetProgress.value - 0.001) {
    await new Promise(resolve => window.setTimeout(resolve, 35))
  }
}

const pollTheaterPackageJob = async (jobId: string) => {
  stopPackagePolling()
  const generation = packagePollGeneration
  while (generation === packagePollGeneration) {
    const response = await api.get<{ job: TheaterPackageJob }>(theaterPackagePath(`jobs/${encodeURIComponent(jobId)}`), { timeout: 30000 })
    const job = response.data.job
    packageProgressJob.value = job
    setPackageProgressTarget(job)
    if (job.status === 'failed') packageProgressError.value = job.errorMessage || '小剧场任务失败'
    if (job.status === 'done') {
      await waitPackageProgressDisplayed()
      return job
    }
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

const exportTheaterPackage = async (kind: 'theater' | 'effects' = 'theater') => {
  packageBusy.value = true
  packageProgressError.value = ''
  packageProgressVisible.value = true
  packageDisplayAnimation += 1
  packageDisplayLoopRunning = false
  packageDisplayedProgress.value = 0
  packageDisplayedDone.value = 0
  packageTargetProgress.value = 0
  packageTargetDone.value = 0
  try {
    const response = await api.post<{ job: TheaterPackageJob }>(theaterPackagePath(kind === 'effects' ? 'export/effects' : 'export'), { inputChannelId: props.channelId })
    packageProgressJob.value = response.data.job
    packageMessage.info(kind === 'effects' ? '特效包导出任务已启动' : '小剧场导出任务已启动')
    const job = await pollTheaterPackageJob(response.data.job.id)
    downloadTheaterPackage(job)
    packageMessage.success(kind === 'effects' ? '特效包 ZIP 已生成' : '小剧场 ZIP 已生成')
  } catch (error) {
    packageProgressError.value = theaterAudioErrorMessage(error, kind === 'effects' ? '特效包导出失败' : '小剧场导出失败')
    packageMessage.error(theaterAudioErrorMessage(error, kind === 'effects' ? '特效包导出失败' : '小剧场导出失败'))
  } finally {
    packageBusy.value = false
  }
}

const importTheaterPackageFile = async (file: File) => {
  packageBusy.value = true
  packageProgressError.value = ''
  packageProgressVisible.value = true
  packageDisplayAnimation += 1
  packageDisplayLoopRunning = false
  packageDisplayedProgress.value = 0
  packageDisplayedDone.value = 0
  packageTargetProgress.value = 0
  packageTargetDone.value = 0
  try {
    const body = new FormData()
    body.append('file', file)
    body.append('inputChannelId', props.channelId)
    const response = await api.post<{ job: TheaterPackageJob }>(theaterPackagePath('import'), body, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
    })
    packageProgressJob.value = response.data.job
    packageMessage.info('ZIP 导入任务已启动')
    const job = await pollTheaterPackageJob(response.data.job.id)
    await fetchTheaterAudioAssets()
    await fetchTheaterPanelOrganizer()
    const warnings = job.summary?.warnings?.filter(Boolean) || []
    packageMessage.success(job.summary?.packageKind === 'effects'
      ? `已导入 ${job.summary?.effects ?? job.summary?.objects ?? 0} 个特效`
      : `已追加导入 ${job.summary?.scenes ?? 0} 个场景、${job.summary?.objects ?? 0} 个组件`)
    if (warnings.length) packageMessage.warning(warnings.join('；'))
  } catch (error) {
    packageProgressError.value = theaterAudioErrorMessage(error, 'ZIP 导入失败')
    packageMessage.error(theaterAudioErrorMessage(error, 'ZIP 导入失败'))
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
    title: '导入小剧场或特效包',
    content: `将“${file.name}”导入当前世界。小剧场包追加场景；特效包导入全部特效，其中原场景特效放入当前场景。现有场景不会被覆盖。`,
    positiveText: '开始导入',
    negativeText: '取消',
    onPositiveClick: () => { void importTheaterPackageFile(file) },
  })
}

const importCCFOLIAPackageFile = async (file: File) => {
  packageBusy.value = true
  packageProgressError.value = ''
  packageProgressVisible.value = true
  packageDisplayAnimation += 1
  packageDisplayLoopRunning = false
  packageDisplayedProgress.value = 0
  packageDisplayedDone.value = 0
  packageTargetProgress.value = 0
  packageTargetDone.value = 0
  try {
    const body = new FormData()
    body.append('file', file)
    body.append('inputChannelId', props.channelId)
    const response = await api.post<{ job: TheaterPackageJob }>(theaterPackagePath('import/ccfolia'), body, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
    })
    packageProgressJob.value = response.data.job
    packageMessage.info('CCFOLIA 导入任务已启动')
    const job = await pollTheaterPackageJob(response.data.job.id)
    const warnings = job.summary?.warnings?.filter(Boolean) || []
    packageMessage.success(`已导入 ${job.summary?.scenes ?? 0} 个场景、${job.summary?.objects ?? 0} 个组件、${job.summary?.resources ?? 0} 个资源`)
    if (warnings.length) packageMessage.warning(warnings.join('；'))
  } catch (error) {
    packageProgressError.value = theaterAudioErrorMessage(error, 'CCFOLIA 导入失败')
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
  if (key === 'export-effects') void exportTheaterPackage('effects')
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

const withTheaterImageAssetURL = (asset: Omit<TheaterImageAsset, 'url'>, scope = captureTheaterRequestScope()): TheaterImageAsset => {
  const resourceBase = urlBase.startsWith('//') ? `${window.location.protocol}${urlBase}` : urlBase
  const variant = asset.resource.playbackVariant || 'original'
  return {
    ...asset,
    url: `${resourceBase.replace(/\/$/, '')}${theaterResourceContentPath(theaterMediaScope(scope), asset.resourceId, variant)}`,
  }
}

const fetchTheaterImageAssets = async () => {
  const generation = ++theaterImageFetchGeneration
  const scope = captureTheaterRequestScope()
  if (!props.worldId || (props.scopeType !== 'world' && !props.channelId) || !props.syncReady) {
    theaterImageAssets.value = []
    theaterImageLoading.value = false
    return
  }
  theaterImageLoading.value = true
  try {
    const response = await api.get<{ items?: Omit<TheaterImageAsset, 'url'>[] }>(theaterImageAssetPath('', scope))
    if (generation !== theaterImageFetchGeneration || !isCurrentTheaterRequestScope(scope)) return
    theaterImageAssets.value = (response.data?.items || []).map((asset) => withTheaterImageAssetURL(asset, scope))
    theaterImageError.value = ''
  } catch (error) {
    if (generation !== theaterImageFetchGeneration || !isCurrentTheaterRequestScope(scope)) return
    theaterImageError.value = theaterAudioErrorMessage(error, '读取图片素材失败')
  } finally {
    if (generation === theaterImageFetchGeneration) theaterImageLoading.value = false
  }
}

const renameTheaterImageAsset = async (assetId: string, name: string) => {
  try {
    await api.patch(theaterImageAssetPath(assetId), { name })
    await fetchTheaterImageAssets()
  } catch (error) {
    theaterImageError.value = theaterAudioErrorMessage(error, '重命名图片素材失败')
  }
}

const updateTheaterImageAssetPreset = async (assetId: string, preset: TheaterImageObjectPreset | null) => {
  if (!canEditAllObjects.value) return
  try {
    await api.patch(theaterImageAssetPath(assetId), { preset })
    await fetchTheaterImageAssets()
  } catch (error) {
    theaterImageError.value = theaterAudioErrorMessage(error, preset ? '保存素材预设失败' : '清除素材预设失败')
  }
}

const deleteTheaterImageAsset = async (asset: TheaterImageAsset) => {
  if (!canDeleteResources.value || !window.confirm(`删除图片素材“${asset.name}”？舞台上的图片组件不会受影响。`)) return
  try {
    await api.delete(theaterImageAssetPath(asset.id))
    await Promise.all([fetchTheaterImageAssets(), fetchTheaterPanelOrganizer()])
  } catch (error) {
    theaterImageError.value = theaterAudioErrorMessage(error, '删除图片素材失败')
  }
}

const deleteTheaterImageAssetsBatch = async (assets: TheaterImageAsset[]) => {
  if (!canDeleteResources.value || !assets.length || !window.confirm(`删除选中的 ${assets.length} 个图片素材？舞台上的图片组件不会受影响。`)) return
  const results = await Promise.allSettled(assets.map((asset) => api.delete(theaterImageAssetPath(asset.id))))
  await Promise.all([fetchTheaterImageAssets(), fetchTheaterPanelOrganizer()])
  const failed = results.filter((result) => result.status === 'rejected')
  if (failed.length) theaterImageError.value = `${assets.length - failed.length} 个删除成功，${failed.length} 个删除失败`
}

const fetchTheaterPanelOrganizer = async () => {
  if (!props.worldId || (props.scopeType !== 'world' && !props.channelId) || !props.syncReady) {
    theaterPanelOrganizer.value = emptyTheaterPanelOrganizer()
    return
  }
  try {
    const response = await api.get<TheaterPanelOrganizerSnapshot>(theaterPanelOrganizerPath())
    theaterPanelOrganizer.value = {
      folders: response.data?.folders || [],
      items: response.data?.items || [],
    }
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, '读取面板文件夹失败'))
  }
}

const createTheaterPanelFolder = async (domain: TheaterPanelDomain, done?: (folder: TheaterPanelFolder | null) => void) => {
  try {
    const response = await api.post<{ folder?: TheaterPanelFolder }>(theaterPanelOrganizerPath('folders'), { domain, name: '' })
    await fetchTheaterPanelOrganizer()
    done?.(response.data?.folder || null)
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, '创建文件夹失败'))
    done?.(null)
  }
}

const renameTheaterPanelFolder = async (folderId: string, name: string) => {
  try {
    await api.patch(theaterPanelOrganizerPath(`folders/${encodeURIComponent(folderId)}`), { name })
    await fetchTheaterPanelOrganizer()
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, '重命名文件夹失败'))
  }
}

const updateTheaterImageFolderPreset = async (folderId: string, preset: TheaterImageFolderPreset | null) => {
  if (!canEditAllObjects.value) return
  try {
    await api.patch(theaterPanelOrganizerPath(`folders/${encodeURIComponent(folderId)}`), { preset })
    await fetchTheaterPanelOrganizer()
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, preset ? '保存图片文件夹预设失败' : '清除图片文件夹预设失败'))
  }
}

const deleteTheaterPanelFolder = async (folderId: string) => {
  try {
    await api.delete(theaterPanelOrganizerPath(`folders/${encodeURIComponent(folderId)}`))
    await fetchTheaterPanelOrganizer()
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, '删除文件夹失败'))
  }
}

const setTheaterPanelFolderCollapsed = async (folderId: string, collapsed: boolean) => {
  const folder = theaterPanelOrganizer.value.folders.find((item) => item.id === folderId)
  if (folder) folder.collapsed = collapsed
  try {
    await api.put(theaterPanelOrganizerPath(`folders/${encodeURIComponent(folderId)}/state`), { collapsed })
  } catch (error) {
    if (folder) folder.collapsed = !collapsed
    stageMessage.error(theaterAudioErrorMessage(error, '保存折叠状态失败'))
  }
}

const reorderTheaterPanelFolders = async (domain: TheaterPanelDomain, folderIds: string[]) => {
  try {
    await api.put(theaterPanelOrganizerPath('folder-order'), { domain, folderIds })
    await fetchTheaterPanelOrganizer()
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, '调整文件夹顺序失败'))
  }
}

const reorderTheaterPanelItems = async (domain: TheaterPanelDomain, folderId: string, targetIds: string[]) => {
  try {
    await api.put(theaterPanelOrganizerPath('item-order'), { domain, folderId, targetIds })
    await fetchTheaterPanelOrganizer()
  } catch (error) {
    stageMessage.error(theaterAudioErrorMessage(error, '整理项目失败'))
  }
}

const duplicateScene = async (sourceSceneId?: string, activateDuplicate = true) => {
  if (duplicatingScene.value) return
  duplicatingScene.value = true
  const sourceItems = new Map(theaterPanelOrganizer.value.items
    .filter((item) => item.domain === 'effect')
    .map((item) => [item.targetId, item]))
  const result = props.store.duplicateScene(sourceSceneId, activateDuplicate)
  const copiedItems = [...result.objectIdMap.entries()].flatMap(([sourceId, targetId]) => {
    const source = sourceItems.get(sourceId)
    return source ? [{ ...source, id: `pending-${targetId}`, targetId }] : []
  })
  theaterPanelOrganizer.value.items.push(...copiedItems)
  try {
    await props.syncBeforeOrganizerWrite()
    const byFolder = new Map<string, typeof copiedItems>()
    copiedItems.forEach((item) => {
      const items = byFolder.get(item.folderId || '') || []
      items.push(item)
      byFolder.set(item.folderId || '', items)
    })
    await Promise.all([...byFolder.entries()].map(([folderId, items]) => api.put(
      theaterPanelOrganizerPath('item-order'),
      {
        domain: 'effect',
        folderId,
        targetIds: items.sort((left, right) => left.sortOrder - right.sortOrder).map((item) => item.targetId),
      },
    )))
    await fetchTheaterPanelOrganizer()
  } catch (error) {
    await fetchTheaterPanelOrganizer()
    stageMessage.error(theaterAudioErrorMessage(error, '复制场景特效文件夹失败'))
  } finally {
    duplicatingScene.value = false
  }
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

const uploadTheaterAudio = async (file: File, targetEffectId = ''): Promise<AudioAsset | null> => {
  if (!canUploadResources.value) return null
  theaterAudioUploading.value = true
  theaterAudioError.value = ''
  try {
    const formData = new FormData()
    formData.append('file', file)
    const response = await api.post<{ item?: AudioAsset }>(theaterAudioPath(), formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: getUploadTimeoutMs(),
    })
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
    return asset || null
  } catch (error) {
    theaterAudioError.value = theaterAudioErrorMessage(error, '上传音频素材失败')
    return null
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
let lastSceneAudioTriggerId = ''
const playSceneAudio = (assetId: string, volume: number, triggerId: string) => {
  if (!assetId || !triggerId || triggerId === lastSceneAudioTriggerId) return false
  lastSceneAudioTriggerId = triggerId
  void playTheaterAudioAsset(assetId, volume, 'scene-switch')
  return true
}
const deleteTheaterAudio = async (asset: AudioAsset) => {
  if (!canDeleteResources.value || !window.confirm(`删除音频素材“${asset.name}”？`)) return
  theaterAudioError.value = ''
	try {
		await api.delete(theaterAudioPath(asset.id))
		await Promise.all([fetchTheaterAudioAssets(), fetchTheaterPanelOrganizer()])
  } catch (error) {
    theaterAudioError.value = theaterAudioErrorMessage(error, '删除音频素材失败')
  }
}

const deleteTheaterAudioBatch = async (assets: AudioAsset[]) => {
  if (!canDeleteResources.value || !assets.length || !window.confirm(`删除选中的 ${assets.length} 条音频素材？`)) return
  theaterAudioError.value = ''
  let failed = 0
  for (const asset of assets) {
    try {
      await api.delete(theaterAudioPath(asset.id))
    } catch {
      failed += 1
    }
  }
  await Promise.all([fetchTheaterAudioAssets(), fetchTheaterPanelOrganizer()])
  if (failed) stageMessage.warning(`${failed} 条音频删除失败；被引用素材不会删除`)
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
const copyMode = ref<StageCopyMode>('offset')
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
const gridSnapPreviewActive = ref(false)
type LayerDropPlacement = 'before' | 'inside' | 'after'
const layerDropTarget = ref<{ id: string | null, placement: LayerDropPlacement } | null>(null)
const layerListRef = ref<HTMLDivElement | null>(null)
let layerDragSession: {
  objectId: string
  pointerId: number
  clientX: number
  clientY: number
  ghost: HTMLElement
} | null = null
let layerDragFrame: number | null = null
let layerExpandTimer: number | null = null
let layerExpandTargetId: string | null = null
let layerHierarchyMovePending = false
let layerHierarchyUpdatedObjectIds = new Set<string>()
const workspaceRef = ref<HTMLDivElement | null>(null)
const hasPermission = (permission: string) => props.syncReady && props.permissions.includes(permission)
const canEditAllObjects = computed(() => hasPermission('stage.object.edit'))
const canEditDelegatedObjects = computed(() => hasPermission('stage.object.edit.delegated'))
const canSwitchScene = computed(() => hasPermission('stage.scene.switch'))
const canTriggerActions = computed(() => hasPermission('stage.action.trigger'))
const canUploadResources = computed(() => hasPermission('stage.resource.upload'))
const canDeleteResources = computed(() => hasPermission('stage.resource.delete'))
const canManageResources = computed(() => canUploadResources.value || canDeleteResources.value)
const sceneOverlayImageFolder = computed(() => theaterPanelOrganizer.value.folders.find(
  (folder) => folder.domain === 'image' && folder.name.trim() === sceneOverlayImageFolderName,
))
const sceneOverlayImageAssets = computed(() => {
  const folderId = sceneOverlayImageFolder.value?.id
  if (!folderId) return []
  const itemOrder = new Map(theaterPanelOrganizer.value.items
    .filter((item) => item.domain === 'image' && item.folderId === folderId)
    .map((item) => [item.targetId, item.sortOrder]))
  return theaterImageAssets.value
    .filter((asset) => itemOrder.has(asset.id))
    .sort((left, right) => (itemOrder.get(left.id) ?? Number.MAX_SAFE_INTEGER)
      - (itemOrder.get(right.id) ?? Number.MAX_SAFE_INTEGER)
      || left.name.localeCompare(right.name))
})
const referencedTheaterAudioAssetIds = computed(() => [...new Set([
  ...Object.values(props.store.state.scenes).flatMap((scene) => [
    scene.state.switchAudio?.assetId,
    ...(scene.state.musicSnapshot?.tracks.flatMap((track) => [
      track.asset?.assetId,
      ...track.playlist.map((asset) => asset.assetId),
    ]) || []),
    ...Object.values(scene.state.sceneObjects)
      .filter(isTheaterEffectObject)
      .map((object) => theaterEffectConfigFromObject(object).audio?.assetId),
  ]),
  ...Object.values(props.store.state.persistentObjects)
    .filter(isTheaterEffectObject)
    .map((object) => theaterEffectConfigFromObject(object).audio?.assetId),
].filter((assetId): assetId is string => Boolean(assetId)))])
const effectActionOptions = computed(() => Object.values(props.store.activeObjects.value)
  .filter(isTheaterEffectObject)
  .sort(compareStageLayersBottomToTop)
  .map((object) => ({ label: object.name, value: object.id })))
const isDrawingTool = (tool: StageCanvasTool | null): tool is StageDrawingTool => Boolean(tool && tool !== 'eraser')
const canEditObject = (object: StageObject | null | undefined) => Boolean(object) && (
  canEditAllObjects.value
  || (canEditDelegatedObjects.value && object!.editable)
)
const canDragObject = (object: StageObject | null | undefined) => Boolean(
  object
  && !object.locked
  && canEditObject(object),
)
const hasConfiguredObjectAction = (object: StageObject) => object.actions.some(
  action => stageActionSchema.safeParse(action).success,
)
const canInteractObject = (object: StageObject | null | undefined) => Boolean(
  object
  && canTriggerActions.value
  && object.visible
  && object.interactive
  && hasConfiguredObjectAction(object)
  && isStageActionTarget(object.type),
)

const imageAnnotationForObject = (object: StageObject | null | undefined) => (
  object?.type === 'image' ? normalizeStageImageAnnotation(object.annotation || object.content?.annotation) : undefined
)
const canShowImageAnnotation = (object: StageObject | null | undefined) => {
  const annotation = imageAnnotationForObject(object)
  return Boolean(object?.visible && object.interactive && annotation?.enabled && annotation.text.trim())
}
const imageAnnotationEditorObject = computed(() => {
  const object = imageAnnotationEditorObjectId.value
    ? props.store.activeObjects.value[imageAnnotationEditorObjectId.value]
    : null
  return object?.type === 'image' ? object : null
})
const imageAnnotationOverlayStyle = computed(() => ({
  left: `${imageAnnotationOverlay.left}px`,
  top: `${imageAnnotationOverlay.top}px`,
  color: imageAnnotationOverlay.annotation.textColor,
  background: annotationRgba(
    imageAnnotationOverlay.annotation.backgroundColor,
    imageAnnotationOverlay.annotation.backgroundOpacity,
  ),
  fontSize: `${imageAnnotationOverlay.annotation.fontSize}px`,
  maxWidth: `${Math.max(80, Math.min(imageAnnotationOverlay.annotation.maxWidth, viewportSize.value.width - 16))}px`,
}))

const annotationRgba = (color: string, opacity: number) => {
  const value = color.replace('#', '')
  const red = Number.parseInt(value.slice(0, 2), 16)
  const green = Number.parseInt(value.slice(2, 4), 16)
  const blue = Number.parseInt(value.slice(4, 6), 16)
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`
}

const clearImageAnnotationTimer = () => {
  if (imageAnnotationTimer !== null) window.clearTimeout(imageAnnotationTimer)
  imageAnnotationTimer = null
  imageAnnotationPendingObjectId = ''
}

const hideImageAnnotation = (objectId?: string) => {
  clearImageAnnotationTimer()
  if (objectId && imageAnnotationOverlay.objectId !== objectId) return
  imageAnnotationOverlay.visible = false
  imageAnnotationOverlay.objectId = ''
}

const positionImageAnnotation = async () => {
  await nextTick()
  if (!imageAnnotationOverlay.visible || !viewportRef.value || !imageAnnotationOverlayRef.value) return
  const viewport = viewportRef.value.getBoundingClientRect()
  const overlay = imageAnnotationOverlayRef.value.getBoundingClientRect()
  const gap = 12
  const x = imageAnnotationOverlay.pointerX
  const y = imageAnnotationOverlay.pointerY
  const placement = imageAnnotationOverlay.annotation.placement
  let resolved = placement
  if (resolved === 'auto') {
    if (x + gap + overlay.width <= viewport.width) resolved = 'right'
    else if (x - gap - overlay.width >= 0) resolved = 'left'
    else if (y + gap + overlay.height <= viewport.height) resolved = 'bottom'
    else resolved = 'top'
  }
  let left = x + gap
  let top = y - overlay.height / 2
  if (resolved === 'left') left = x - gap - overlay.width
  else if (resolved === 'top') {
    left = x - overlay.width / 2
    top = y - gap - overlay.height
  } else if (resolved === 'bottom') {
    left = x - overlay.width / 2
    top = y + gap
  }
  imageAnnotationOverlay.left = Math.max(8, Math.min(viewport.width - overlay.width - 8, left))
  imageAnnotationOverlay.top = Math.max(8, Math.min(viewport.height - overlay.height - 8, top))
}

const updateImageAnnotationHover = (objectId: string, event: Konva.KonvaEventObject<MouseEvent | PointerEvent>) => {
  const object = getObject(objectId)
  if (!canShowImageAnnotation(object) || !viewportRef.value) {
    hideImageAnnotation(objectId)
    return
  }
  const annotation = imageAnnotationForObject(object)!
  const viewport = viewportRef.value.getBoundingClientRect()
  const updatePointer = () => {
    imageAnnotationOverlay.pointerX = event.evt.clientX - viewport.left
    imageAnnotationOverlay.pointerY = event.evt.clientY - viewport.top
  }
  updatePointer()
  if (imageAnnotationOverlay.visible && imageAnnotationOverlay.objectId === objectId) {
    void positionImageAnnotation()
    return
  }
  if (imageAnnotationTimer !== null && imageAnnotationPendingObjectId === objectId) return
  clearImageAnnotationTimer()
  imageAnnotationPendingObjectId = objectId
  imageAnnotationTimer = window.setTimeout(() => {
    imageAnnotationTimer = null
    imageAnnotationPendingObjectId = ''
    if (!canShowImageAnnotation(getObject(objectId))) return
    imageAnnotationOverlay.objectId = objectId
    imageAnnotationOverlay.annotation = annotation
    imageAnnotationOverlay.visible = true
    void positionImageAnnotation()
  }, annotation.delayMs)
}

const openImageAnnotationEditor = (objectId: string) => {
  const object = getObject(objectId)
  if (object?.type !== 'image' || !canEditObject(object)) return
  hideImageAnnotation()
  imageAnnotationEditorObjectId.value = objectId
  imageAnnotationEditorVisible.value = true
}

const closeImageAnnotationEditor = () => {
  imageAnnotationEditorVisible.value = false
  imageAnnotationEditorObjectId.value = null
}

const saveImageAnnotation = (annotation: StageImageAnnotation) => {
  const object = imageAnnotationEditorObject.value
  if (!object || !canEditObject(object)) return
  props.store.beginObjectEdit('修改图片标注')
  object.annotation = normalizeStageImageAnnotation(annotation)
  props.store.commitObjectEdit()
  closeImageAnnotationEditor()
}

type PanelId = 'scene' | 'inspector' | 'layer' | 'effect' | 'overlay' | 'asset'

const canOpenPanel = (id: PanelId) => {
  if (id === 'scene') return canEditAllObjects.value || canSwitchScene.value
  if (id === 'inspector') return canEditAllObjects.value || canEditDelegatedObjects.value
  if (id === 'asset') return canManageResources.value
  return canEditAllObjects.value
}

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

const objectHierarchyDepth = (objectId: string) => {
  let depth = 0
  let object = props.store.activeObjects.value[objectId]
  const visited = new Set<string>()
  while (object?.parentId && !visited.has(object.parentId)) {
    visited.add(object.parentId)
    depth += 1
    object = props.store.activeObjects.value[object.parentId]
  }
  return depth
}

const dissolveGroupsAndRemoveObjects = (objectIds: string[]) => {
  const objects = props.store.activeObjects.value
  const selectedIds = [...new Set(objectIds)].filter((id) => Boolean(objects[id]))
  const groupIds = selectedIds
    .filter((id) => objects[id]?.type === 'group')
    .sort((left, right) => objectHierarchyDepth(right) - objectHierarchyDepth(left))
  const componentIds = selectedIds.filter((id) => objects[id]?.type !== 'group')
  props.store.beginObjectEdit(groupIds.length ? '解散组' : '删除组件')
  try {
    groupIds.forEach((groupId) => {
      const group = props.store.activeObjects.value[groupId]
      if (!group) return
      const childIds = Object.values(props.store.activeObjects.value)
        .filter((object) => object.parentId === groupId)
        .map((object) => object.id)
      childIds.forEach((childId) => {
        if (!reparentObjectPreservingTransform(childId, group.parentId)) {
          throw new Error('解散组时无法保留成员位置')
        }
      })
      props.store.removeObjects([groupId])
    })
    if (componentIds.length) props.store.removeObjects(componentIds)
    props.store.commitObjectEdit()
    nextTick(updateTransformer)
  } catch (error) {
    props.store.cancelObjectEdit()
    stageMessage.error(error instanceof Error ? error.message : '解散组失败')
  }
}

const removeObjectsWithConfirm = (objectIds: string[]) => {
  const ids = objectIds.filter((id) => Boolean(props.store.activeObjects.value[id]))
  if (!ids.length || !canEditAllObjects.value) return false
  const groupCount = ids.filter((id) => props.store.activeObjects.value[id]?.type === 'group').length
  confirmDelete(
    groupCount ? '解散组' : '删除组件',
    groupCount
      ? `确定解散选中的 ${groupCount} 个组？组成员会保留；另外选中的组件仍会删除。`
      : ids.length > 1 ? `确定删除选中的 ${ids.length} 个组件？` : '确定删除选中的组件？',
    () => dissolveGroupsAndRemoveObjects(ids),
  )
  return true
}

const removeGroupTreeWithConfirm = (groupId: string) => {
  const group = props.store.activeObjects.value[groupId]
  if (!group || group.type !== 'group' || !canEditAllObjects.value) return
  confirmDelete('删除组及成员', `确定删除组“${group.name}”及其全部成员？`, () => {
    props.store.removeObjects([groupId])
    nextTick(updateTransformer)
  })
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

type SceneBatchMode = 'copy' | 'delete' | 'construction'
const sceneBatchMode = ref<SceneBatchMode | null>(null)
const sceneBatchSelectedIds = ref<string[]>([])
const lastConstructionSceneId = ref<string | null>(null)
let sceneBatchLongPressTimer: number | null = null
let sceneBatchLongPressTriggered = false

const clearSceneBatchLongPress = () => {
  if (sceneBatchLongPressTimer !== null) window.clearTimeout(sceneBatchLongPressTimer)
  sceneBatchLongPressTimer = null
}

const exitSceneBatchMode = () => {
  clearSceneBatchLongPress()
  sceneBatchMode.value = null
  sceneBatchSelectedIds.value = []
}

const enterSceneBatchMode = (mode: SceneBatchMode) => {
  closeSceneEditor()
  sceneEditMode.value = false
  sceneBatchMode.value = mode
  const constructionSceneId = props.constructionSceneId || lastConstructionSceneId.value
  sceneBatchSelectedIds.value = mode === 'construction' && constructionSceneId && props.store.state.scenes[constructionSceneId]
    ? [constructionSceneId]
    : []
}

const isSceneBatchSelected = (sceneId: string) => sceneBatchSelectedIds.value.includes(sceneId)

const setSceneBatchSelected = (sceneId: string, selected: boolean) => {
  if (sceneBatchMode.value === 'construction') {
    sceneBatchSelectedIds.value = selected ? [sceneId] : []
    if (selected) lastConstructionSceneId.value = sceneId
    return
  }
  const selectedIds = new Set(sceneBatchSelectedIds.value)
  if (selected) selectedIds.add(sceneId)
  else selectedIds.delete(sceneId)
  sceneBatchSelectedIds.value = [...selectedIds]
}

const toggleSceneBatchSelected = (sceneId: string) => setSceneBatchSelected(sceneId, !isSceneBatchSelected(sceneId))

const startSceneBatchLongPress = (event: PointerEvent, mode: SceneBatchMode) => {
  if (!canEditAllObjects.value || sceneEditMode.value || event.button !== 0) return
  clearSceneBatchLongPress()
  sceneBatchLongPressTriggered = false
  sceneBatchLongPressTimer = window.setTimeout(() => {
    sceneBatchLongPressTimer = null
    sceneBatchLongPressTriggered = true
    enterSceneBatchMode(mode)
  }, 550)
}

const finishSceneBatchLongPress = () => clearSceneBatchLongPress()

const duplicateSelectedScenes = async () => {
  const sceneIds = sceneBatchSelectedIds.value.filter((sceneId) => Boolean(props.store.state.scenes[sceneId]))
  if (!sceneIds.length) {
    stageMessage.warning('请先选择要复制的场景')
    return
  }
  for (const sceneId of sceneIds) await duplicateScene(sceneId, false)
  stageMessage.success(`已复制 ${sceneIds.length} 个场景`)
  exitSceneBatchMode()
}

const removeSelectedScenesWithConfirm = () => {
  const sceneIds = sceneBatchSelectedIds.value.filter((sceneId) => Boolean(props.store.state.scenes[sceneId]))
  if (!sceneIds.length) {
    stageMessage.warning('请先选择要删除的场景')
    return
  }
  if (sceneIds.length >= props.store.scenes.value.length) {
    stageMessage.warning('至少保留一个场景，请取消选择一项')
    return
  }
  confirmDelete('批量删除场景', `确定逐一删除选中的 ${sceneIds.length} 个场景？场景内组件也会一并删除。`, () => {
    sceneIds.forEach((sceneId) => props.store.removeScene(sceneId))
    exitSceneBatchMode()
  })
}

const applyConstructionScene = () => {
  const sceneId = sceneBatchSelectedIds.value[0] || null
  if (sceneId) lastConstructionSceneId.value = sceneId
  emit('constructionSceneChangeRequested', sceneId)
  exitSceneBatchMode()
}

const handleSceneActionClick = (mode: SceneBatchMode) => {
  if (sceneBatchLongPressTriggered) {
    sceneBatchLongPressTriggered = false
    return
  }
  if (sceneBatchMode.value === mode) {
    if (mode === 'copy') void duplicateSelectedScenes()
    else if (mode === 'delete') removeSelectedScenesWithConfirm()
    else applyConstructionScene()
    return
  }
  if (sceneBatchMode.value) {
    enterSceneBatchMode(mode)
    return
  }
  if (mode === 'copy') void duplicateScene()
  else if (mode === 'delete') removeActiveSceneWithConfirm()
  else enterSceneBatchMode('construction')
}

const sceneEditMode = ref(false)
const editingSceneId = ref<string | null>(null)
const editingSceneName = ref('')
const editingSceneSwitchText = ref('')
const editingSceneSwitchAudio = ref<StageAudioRef | null>(null)
const editingSceneTransition = ref<StageSceneTransition>(normalizeStageSceneTransition(null))
const sceneMusicPreviewVisible = ref(false)
const editingSceneMusicSnapshot = computed(() => normalizeStageMusicSnapshot(
  editingSceneId.value ? props.store.state.scenes[editingSceneId.value]?.state.musicSnapshot : null,
))
const sceneMusicTrackLabels: Record<StageMusicTrackType, string> = {
  music: '音乐',
  ambience: '环境',
  sfx: '音效',
}
const sceneMusicPlaylistModeLabels: Record<StageMusicPlaylistMode, string> = {
  single: '单曲',
  sequential: '顺序',
  shuffle: '随机',
}
const sceneMusicSummary = computed(() => {
  const snapshot = editingSceneMusicSnapshot.value
  if (!snapshot) return '未记录'
  const itemCount = snapshot.tracks.reduce((total, track) => total + Math.max(track.playlist.length, track.asset ? 1 : 0), 0)
  const trackCount = snapshot.tracks.filter((track) => track.asset || track.playlist.length).length
  return `已记录 · ${trackCount}轨 · ${itemCount}首`
})
const sceneSwitchAudioOptions = computed(() => {
  const options = theaterAudioAssets.value
    .filter((asset) => !asset.transcodeStatus || asset.transcodeStatus === 'ready')
    .map((asset) => ({ label: asset.name, value: asset.id }))
  const selected = editingSceneSwitchAudio.value
  if (selected && !options.some((option) => option.value === selected.assetId)) {
    options.unshift({ label: selected.name || selected.assetId, value: selected.assetId })
  }
  return options
})
const sceneTransitionTypeOptions = stageSceneTransitionTypes.map((type) => ({
  value: type,
  label: ({
    none: '无',
    fade: '淡入淡出',
    slide: '滑动',
    dissolve: '溶解',
    zoom: '缩放',
    mask: '遮罩',
    flip: '翻转',
    blur: '模糊',
    rotate: '旋转',
    curtain: '黑幕开合',
  } satisfies Record<StageSceneTransitionType, string>)[type],
}))
const sceneListRef = ref<HTMLDivElement | null>(null)
const collapsedSceneFolders = ref(new Set<string>())
const sceneFolderCollapseKey = (folderId: string) => `folder:${folderId}`
const uncategorizedSceneFolderCollapseKey = 'virtual:uncategorized'
type SceneFolderDialogMode = 'create' | 'rename'
const sceneFolderDialogVisible = ref(false)
const sceneFolderDialogMode = ref<SceneFolderDialogMode>('create')
const sceneFolderDialogFolderId = ref<string | null>(null)
const sceneFolderNameDraft = ref('')
type SceneListEntry =
  | { kind: 'scene', key: string, scene: StageScene, nested: boolean }
  | { kind: 'folder', key: string, folder: SceneFolder, scenes: StageScene[], collapsed: boolean }
  | { kind: 'uncategorized', key: string, scenes: StageScene[], collapsed: boolean }
const sceneListEntries = computed<SceneListEntry[]>(() => {
  const folders = props.store.state.sceneFolders || []
  const scenes = props.store.scenes.value
  const folderIDs = new Set(folders.map((folder) => folder.id))
  const grouped = new Map<string, StageScene[]>()
  const uncategorized: StageScene[] = []
  scenes.forEach((scene) => {
    const folderId = scene.folderId && folderIDs.has(scene.folderId) ? scene.folderId : ''
    if (!folderId) uncategorized.push(scene)
    else {
      const bucket = grouped.get(folderId)
      if (bucket) bucket.push(scene)
      else grouped.set(folderId, [scene])
    }
  })
  const entries: SceneListEntry[] = []
  folders.forEach((folder) => {
    const scenesInFolder = grouped.get(folder.id) || []
    const collapsed = collapsedSceneFolders.value.has(sceneFolderCollapseKey(folder.id))
    entries.push({ kind: 'folder', key: `folder:${folder.id}`, folder, scenes: scenesInFolder, collapsed })
    if (!collapsed) scenesInFolder.forEach((scene) => entries.push({ kind: 'scene', key: scene.id, scene, nested: true }))
  })
  const uncategorizedCollapsed = collapsedSceneFolders.value.has(uncategorizedSceneFolderCollapseKey)
  entries.push({ kind: 'uncategorized', key: 'virtual-folder:uncategorized', scenes: uncategorized, collapsed: uncategorizedCollapsed })
  if (!uncategorizedCollapsed) uncategorized.forEach((scene) => entries.push({ kind: 'scene', key: scene.id, scene, nested: true }))
  return entries
})
const draggedSceneId = ref<string | null>(null)
type SceneDropPlacement = 'before' | 'after'
const sceneDropTarget = ref<{ id: string, placement: SceneDropPlacement } | null>(null)
let sceneDragSession: {
  sceneId: string
  pointerId: number
  clientX: number
  clientY: number
  ghost: HTMLElement
} | null = null
let sceneDragFrame: number | null = null

const beginSceneEdit = (scene: StageScene) => {
  if (!canEditAllObjects.value) return
  editingSceneId.value = scene.id
  editingSceneName.value = scene.name
  editingSceneSwitchText.value = scene.switchText
  editingSceneSwitchAudio.value = normalizeStageAudioRef(scene.state.switchAudio)
  editingSceneTransition.value = normalizeStageSceneTransition(scene.state.transition)
  sceneMusicPreviewVisible.value = false
}

const closeSceneEditor = () => {
  editingSceneId.value = null
  editingSceneName.value = ''
  editingSceneSwitchText.value = ''
  editingSceneSwitchAudio.value = null
  editingSceneTransition.value = normalizeStageSceneTransition(null)
  sceneMusicPreviewVisible.value = false
}

const updateEditingSceneTransition = (
  direction: 'enter' | 'exit',
  patch: Partial<StageSceneTransitionPhase>,
) => {
  const current = editingSceneTransition.value
  if (patch.type === 'curtain') {
    editingSceneTransition.value = {
      curtain: current.curtain,
      enter: { ...current.enter, type: 'curtain' },
      exit: { ...current.exit, type: 'curtain' },
    }
    return
  }
  const phase = { ...current[direction], ...patch }
  editingSceneTransition.value = direction === 'enter'
    ? { curtain: current.curtain, enter: phase, exit: { ...current.exit } }
    : { curtain: current.curtain, enter: { ...current.enter }, exit: phase }
}

const updateEditingSceneTransitionDuration = (direction: 'enter' | 'exit', value: number | null) => {
  if (value === null || !Number.isFinite(value)) return
  updateEditingSceneTransition(direction, { durationMs: value })
}

const updateEditingSceneSwitchAudio = (assetId: string | null) => {
  const asset = theaterAudioAssets.value.find((item) => item.id === assetId)
  editingSceneSwitchAudio.value = asset
    ? { assetId: asset.id, name: asset.name, volume: editingSceneSwitchAudio.value?.volume ?? 1 }
    : null
}

const requestSceneAudioUpload = () => sceneAudioInputRef.value?.click()

const handleSceneAudioInput = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const asset = await uploadTheaterAudio(file)
  if (asset) editingSceneSwitchAudio.value = { assetId: asset.id, name: asset.name, volume: 1 }
}

const previewEditingSceneSwitchAudio = () => {
  const audio = editingSceneSwitchAudio.value
  if (audio) void playTheaterAudioAsset(audio.assetId, audio.volume, 'preview')
}

const toggleSceneEditMode = () => {
  if (!sceneEditMode.value) exitSceneBatchMode()
  sceneEditMode.value = !sceneEditMode.value
  closeSceneEditor()
}

watch(scenePanelOpen, (open) => {
  if (open) return
  exitSceneBatchMode()
  sceneEditMode.value = false
  closeSceneEditor()
})

const handleSceneClick = (scene: StageScene) => {
  if (sceneBatchMode.value) {
    toggleSceneBatchSelected(scene.id)
    return
  }
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
  props.store.updateSceneTransition(sceneId, editingSceneTransition.value)
  props.store.updateSceneSwitchAudio(sceneId, editingSceneSwitchAudio.value)
  closeSceneEditor()
}

const setSceneDropTarget = (target: typeof sceneDropTarget.value) => {
  if (sceneDropTarget.value?.id === target?.id && sceneDropTarget.value?.placement === target?.placement) return
  sceneDropTarget.value = target
}

const updateSceneDropTarget = (clientX: number, clientY: number) => {
  const session = sceneDragSession
  const list = sceneListRef.value
  if (!session || !list) return
  const element = document.elementFromPoint(clientX, clientY) as HTMLElement | null
  const row = element?.closest<HTMLElement>('.theater-scene-row')
  const targetId = row?.dataset.sceneId
  if (!row || !targetId || !list.contains(row) || targetId === session.sceneId) {
    setSceneDropTarget(null)
    return
  }
  const rect = row.getBoundingClientRect()
  setSceneDropTarget({ id: targetId, placement: clientY < rect.top + rect.height / 2 ? 'before' : 'after' })
}

const runSceneDragFrame = () => {
  sceneDragFrame = null
  const session = sceneDragSession
  if (!session) return
  session.ghost.style.transform = `translate3d(${session.clientX + 12}px, ${session.clientY + 12}px, 0)`
  updateSceneDropTarget(session.clientX, session.clientY)
  const list = sceneListRef.value
  if (!list) return
  const rect = list.getBoundingClientRect()
  const edge = Math.min(44, rect.height / 4)
  const topDistance = session.clientY - rect.top
  const bottomDistance = rect.bottom - session.clientY
  const speed = topDistance >= 0 && topDistance < edge
    ? -Math.ceil((edge - topDistance) / 4)
    : bottomDistance >= 0 && bottomDistance < edge
      ? Math.ceil((edge - bottomDistance) / 4)
      : 0
  if (speed) {
    list.scrollTop += speed
    sceneDragFrame = window.requestAnimationFrame(runSceneDragFrame)
  }
}

const scheduleSceneDragFrame = () => {
  if (sceneDragFrame === null) sceneDragFrame = window.requestAnimationFrame(runSceneDragFrame)
}

const startScenePointerDrag = (event: PointerEvent, sceneId: string) => {
  if (!canEditAllObjects.value || !sceneEditMode.value || event.button !== 0 || sceneDragSession) return
  const grip = event.currentTarget as HTMLElement
  const row = grip.closest<HTMLElement>('.theater-scene-row')
  if (!row) return
  event.preventDefault()
  grip.setPointerCapture(event.pointerId)
  const rect = row.getBoundingClientRect()
  const ghost = row.cloneNode(true) as HTMLElement
  ghost.classList.add('is-drag-preview')
  ghost.setAttribute('aria-hidden', 'true')
  ghost.style.width = `${rect.width}px`
  document.body.appendChild(ghost)
  sceneDragSession = { sceneId, pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, ghost }
  draggedSceneId.value = sceneId
  setSceneDropTarget(null)
  scheduleSceneDragFrame()
}

const moveScenePointerDrag = (event: PointerEvent) => {
  if (!sceneDragSession || event.pointerId !== sceneDragSession.pointerId) return
  sceneDragSession.clientX = event.clientX
  sceneDragSession.clientY = event.clientY
  scheduleSceneDragFrame()
}

const finishScenePointerDrag = (event: PointerEvent, cancelled = false) => {
  const session = sceneDragSession
  if (!session || event.pointerId !== session.pointerId) return
  if (!cancelled) updateSceneDropTarget(event.clientX, event.clientY)
  const dropTarget = sceneDropTarget.value
  sceneDragSession = null
  const grip = event.currentTarget as HTMLElement
  if (grip.hasPointerCapture(event.pointerId)) grip.releasePointerCapture(event.pointerId)
  if (sceneDragFrame !== null) window.cancelAnimationFrame(sceneDragFrame)
  sceneDragFrame = null
  session.ghost.remove()
  draggedSceneId.value = null
  setSceneDropTarget(null)
  if (!cancelled && dropTarget) props.store.reorderScenes(session.sceneId, dropTarget.id, dropTarget.placement)
}

const toggleSceneFolder = (folderId: string) => {
  const next = new Set(collapsedSceneFolders.value)
  if (next.has(folderId)) next.delete(folderId)
  else next.add(folderId)
  collapsedSceneFolders.value = next
}

const openSceneFolderDialog = (mode: SceneFolderDialogMode, folderId: string | null = null) => {
  if (!canEditAllObjects.value) return
  const folder = mode === 'rename' && folderId
    ? props.store.state.sceneFolders.find((item) => item.id === folderId)
    : null
  if (mode === 'rename' && !folder) return
  sceneFolderDialogMode.value = mode
  sceneFolderDialogFolderId.value = folder?.id || null
  sceneFolderNameDraft.value = folder?.name || ''
  sceneFolderDialogVisible.value = true
}

const createSceneFolder = () => openSceneFolderDialog('create')

const closeSceneFolderDialog = () => {
  sceneFolderDialogVisible.value = false
}

const submitSceneFolderDialog = () => {
  const name = sceneFolderNameDraft.value.trim()
  if (!name) {
    stageMessage.warning('文件夹名称不能为空、不能重复或过长')
    return
  }
  if (sceneFolderDialogMode.value === 'create') {
    if (props.store.state.sceneFolders.length >= 200) {
      stageMessage.warning('场景文件夹最多创建 200 个')
      return
    }
    const folder = props.store.createSceneFolder(name)
    if (!folder) {
      stageMessage.warning('文件夹名称不能为空、不能重复或过长')
      return
    }
    const next = new Set(collapsedSceneFolders.value)
    next.delete(sceneFolderCollapseKey(folder.id))
    collapsedSceneFolders.value = next
    closeSceneFolderDialog()
    return
  }
  const folderId = sceneFolderDialogFolderId.value
  const folder = folderId ? props.store.state.sceneFolders.find((item) => item.id === folderId) : null
  if (!folder) {
    closeSceneFolderDialog()
    return
  }
  if (name === folder.name) {
    closeSceneFolderDialog()
    return
  }
  if (!props.store.renameSceneFolder(folder.id, name)) {
    stageMessage.warning('文件夹名称不能为空、不能重复或过长')
    return
  }
  closeSceneFolderDialog()
}

const handleSceneFolderDialogKeydown = (event: KeyboardEvent) => {
  if (event.isComposing || event.key !== 'Enter') return
  event.preventDefault()
  submitSceneFolderDialog()
}

const sceneFolderMenuOptions: DropdownOption[] = [
  { label: '重命名', key: 'rename' },
  { label: '删除', key: 'delete' },
]

const handleSceneFolderMenu = (key: string | number, folderId: string) => {
  const folder = props.store.state.sceneFolders.find((item) => item.id === folderId)
  if (!folder) return
  if (key === 'rename') {
    openSceneFolderDialog('rename', folder.id)
    return
  }
  if (key === 'delete') {
    confirmDelete('删除文件夹', `删除文件夹“${folder.name}”不会删除其中的场景，场景将移动到未分类。`, () => {
      props.store.deleteSceneFolder(folderId)
      const collapseKey = sceneFolderCollapseKey(folderId)
      collapsedSceneFolders.value = new Set([...collapsedSceneFolders.value].filter((id) => id !== collapseKey))
    })
  }
}

const sceneMoveOptions = (scene: StageScene): DropdownOption[] => [
  { label: '移动到未分类', key: 'move:', disabled: !scene.folderId },
  ...props.store.state.sceneFolders.map((folder) => ({
    label: `移动到：${folder.name}`,
    key: `move:${folder.id}`,
    disabled: scene.folderId === folder.id,
  })),
]

const moveSceneFromMenu = (key: string | number, sceneId: string) => {
  const value = String(key)
  if (!value.startsWith('move:')) return
  const folderId = value.slice('move:'.length) || null
  props.store.moveSceneToFolder(sceneId, folderId)
}

const isEditableShortcutTarget = (target: EventTarget | null) => {
  const element = target instanceof HTMLElement ? target : null
  return Boolean(element?.closest('input, textarea, select, [contenteditable="true"]'))
}

const copySelectedObjects = () => props.store.copySelectedObjects(copyMode.value)

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
      .filter((object) => object.visible && object.type !== 'group')
      .map((object) => object.id))
    handled = true
  } else if (key === 'c') handled = copySelectedObjects()
  else if (key === 'x' && canEditAllObjects.value) handled = props.store.cutSelectedObjects()
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
  overlay: { width: 520, height: 360 },
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
const frontPanelId = ref<PanelId | null>(null)
let panelResizeObserver: ResizeObserver | null = null
let draggingPanel: { id: PanelId, pointerX: number, pointerY: number, x: number, y: number } | null = null

const panelDefaultLayout = (id: PanelId): PanelLayout => {
  const workspace = workspaceRef.value
  const workspaceWidth = workspace?.clientWidth || 960
  const workspaceHeight = workspace?.clientHeight || 640
  const width = id === 'scene' ? 168 : id === 'inspector' ? 280 : id === 'overlay' ? 680 : id === 'effect' || id === 'asset' ? 340 : 300
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
    zIndex: frontPanelId.value === id ? '10001' : '10000',
  }
}

const sceneEditorPlacement = computed<'right-start' | 'left-start' | 'bottom-start'>(() => {
  // Keep side placement only when editor width plus popover gap fits beside scene panel.
  const layout = panelLayouts.value.scene || panelDefaultLayout('scene')
  const workspaceLeft = workspaceRef.value?.getBoundingClientRect().left || 0
  const viewportWidth = Math.max(
    1,
    document.documentElement.clientWidth || 0,
    window.innerWidth || 0,
    viewportSize.value.width,
  )
  const editorWidth = Math.min(320, Math.max(1, viewportWidth - 24))
  const panelLeft = workspaceLeft + layout.x
  const panelRight = panelLeft + layout.width
  const popoverGap = 8
  if (viewportWidth - panelRight >= editorWidth + popoverGap) return 'right-start'
  if (panelLeft >= editorWidth + popoverGap) return 'left-start'
  return 'bottom-start'
})

const bringPanelToFront = (id: PanelId) => {
  frontPanelId.value = id
}

const openOverlayPanel = () => {
  if (!canOpenPanel('overlay')) return
  overlayPanelOpen.value = true
  bringPanelToFront('overlay')
}

const togglePanel = (id: PanelId) => {
  if (!canOpenPanel(id)) return
  if (id === 'scene') scenePanelOpen.value = !scenePanelOpen.value
  else if (id === 'inspector') inspectorPanelOpen.value = !inspectorPanelOpen.value
  else if (id === 'layer') layerPanelOpen.value = !layerPanelOpen.value
  else if (id === 'effect') effectPanelOpen.value = !effectPanelOpen.value
  else if (id === 'overlay') overlayPanelOpen.value = !overlayPanelOpen.value
  else assetPanelOpen.value = !assetPanelOpen.value

  const isOpen = id === 'scene'
    ? scenePanelOpen.value
    : id === 'inspector'
      ? inspectorPanelOpen.value
      : id === 'layer'
        ? layerPanelOpen.value
        : id === 'effect'
          ? effectPanelOpen.value
          : id === 'overlay'
            ? overlayPanelOpen.value
            : assetPanelOpen.value
  if (isOpen) bringPanelToFront(id)
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
    ['overlay', overlayPanelOpen.value],
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
  const ids: PanelId[] = ['scene', 'inspector', 'layer', 'effect', 'overlay', 'asset']
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
let worldOverlayLayer: Konva.Layer | null = null
let foregroundLayer: Konva.Layer | null = null
let gridTopLayer: Konva.Layer | null = null
let interactionLayer: Konva.Layer | null = null
let backgroundCameraGroup: Konva.Group | null = null
let worldCameraGroup: Konva.Group | null = null
let worldOverlayCameraGroup: Konva.Group | null = null
let foregroundCameraGroup: Konva.Group | null = null
let gridTopCameraGroup: Konva.Group | null = null
let gridGroup: Konva.Group | null = null
let objectRoot: Konva.Group | null = null
const objectRootLayers = new Map<string, { layer: Konva.Layer; camera: Konva.Group }>()
const rootStackingOrder = ref<Record<string, number>>({})
const OBJECT_ROOT_LAYER_Z_BASE = 100
const WORLD_OVERLAY_LAYER_Z = 8990
const GRID_TOP_LAYER_Z = 9990
let sceneMorphStage: Konva.Stage | null = null
const sceneMorphLayers = new Map<string, { layer: Konva.Layer; camera: Konva.Group; root: Konva.Group }>()
const sceneMorphTextCameras = new Map<string, HTMLDivElement>()
let drawingDraftRoot: Konva.Group | null = null
let pointerTraceRoot: Konva.Group | null = null
let transformer: Konva.Transformer | null = null
let selectionRect: Konva.Rect | null = null
let selectionGroupHitArea: Konva.Rect | null = null
let quickDeleteOutline: Konva.Rect | null = null
let resizeObserver: ResizeObserver | null = null
let panning = false
let panCandidate: {
  pointerId: number
  objectId: string | null
} | null = null
let suppressedObjectClickId: string | null = null
let marqueeStart: { x: number, y: number } | null = null
let marqueeAdditive = false
let panPointer = { x: 0, y: 0 }
let panOrigin = { x: 0, y: 0 }
let gridSignature = ''
let gridCoverage: { minX: number, maxX: number, minY: number, maxY: number } | null = null
let gridSyncFrame: number | null = null
let gridNodeSnapSession: {
  node: Konva.Node
  camera: Konva.Group
  offsetX: number
  offsetY: number
  lockedX: number | null
  lockedY: number | null
} | null = null
let gridResizeSession: StageGridResizeSession | null = null

const gridSnapEnabled = computed(() => props.store.state.liveState.alignWithGrid)
const gridDisplayEnabled = computed(() => props.store.state.liveState.displayGrid)
const gridOnTopEnabled = computed(() => props.store.state.liveState.gridOnTop)
const toggleGridSnap = () => {
  if (!canEditAllObjects.value) return
  props.store.state.liveState.alignWithGrid = !props.store.state.liveState.alignWithGrid
}
const toggleGridDisplay = () => {
  if (!canEditAllObjects.value) return
  props.store.state.liveState.displayGrid = !props.store.state.liveState.displayGrid
}
const toggleGridOnTop = () => {
  if (!canEditAllObjects.value) return
  props.store.state.liveState.gridOnTop = !props.store.state.liveState.gridOnTop
}

const setGridSnapPreview = (active: boolean) => {
  if (!active) gridNodeSnapSession = null
  if (gridSnapPreviewActive.value === active) return
  gridSnapPreviewActive.value = active
  scheduleGridSync()
}

const finishGridSnapPreview = () => {
  if (gridSnapPreviewActive.value) setGridSnapPreview(false)
}

const stageGridStep = () => Math.max(0.25, props.store.state.liveState.gridSize) * WORLD_UNIT_PX

const snapStageCoordinate = (value: number, axis: 'x' | 'y') => {
  const liveState = props.store.state.liveState
  if (!liveState.alignWithGrid) return value
  const fieldSize = axis === 'x' ? liveState.fieldWidth : liveState.fieldHeight
  const origin = -fieldSize * WORLD_UNIT_PX / 2
  const step = stageGridStep()
  return origin + Math.round((value - origin) / step) * step
}

const snapStagePosition = (position: { x: number, y: number }) => ({
  x: snapStageCoordinate(position.x, 'x'),
  y: snapStageCoordinate(position.y, 'y'),
})

const snapDrawingPoint = (tool: StageDrawingTool, position: { x: number, y: number }) => (
  tool === 'pen' || tool === 'highlighter' ? position : snapStagePosition(position)
)

const cameraForNode = (node: Konva.Node) => {
  let ancestor = node.getParent()
  while (ancestor && ancestor.getParent() && !(ancestor.getParent() instanceof Konva.Layer)) {
    ancestor = ancestor.getParent()
  }
  return ancestor instanceof Konva.Group ? ancestor : worldCameraGroup
}

const beginNodeGridSnap = (node: Konva.Node) => {
  const cameraGroup = cameraForNode(node)
  if (!gridSnapEnabled.value || !cameraGroup) {
    gridNodeSnapSession = null
    return
  }
  const zoom = Math.max(0.01, cameraGroup.scaleX())
  const camera = cameraGroup.absolutePosition()
  const current = node.absolutePosition()
  const origin = { x: (current.x - camera.x) / zoom, y: (current.y - camera.y) / zoom }
  const bounds = node.getClientRect({
    relativeTo: cameraGroup,
    skipStroke: true,
    skipShadow: true,
  })
  gridNodeSnapSession = {
    node,
    camera: cameraGroup,
    offsetX: bounds.x - origin.x,
    offsetY: bounds.y - origin.y,
    lockedX: null,
    lockedY: null,
  }
}

const snapStageCoordinateWithHysteresis = (
  value: number,
  axis: 'x' | 'y',
  locked: number | null,
) => locked !== null && Math.abs(value - locked) <= stageGridStep() * 0.6
  ? locked
  : snapStageCoordinate(value, axis)

const snapNodeToGrid = (node: Konva.Node) => {
  if (!gridSnapEnabled.value) return { x: 0, y: 0 }
  if (gridNodeSnapSession?.node !== node) beginNodeGridSnap(node)
  const session = gridNodeSnapSession
  if (!session) return { x: 0, y: 0 }
  const zoom = Math.max(0.01, session.camera.scaleX())
  const camera = session.camera.absolutePosition()
  const current = node.absolutePosition()
  const bounds = {
    x: (current.x - camera.x) / zoom + session.offsetX,
    y: (current.y - camera.y) / zoom + session.offsetY,
  }
  const snapped = {
    x: snapStageCoordinateWithHysteresis(bounds.x, 'x', session.lockedX),
    y: snapStageCoordinateWithHysteresis(bounds.y, 'y', session.lockedY),
  }
  session.lockedX = snapped.x
  session.lockedY = snapped.y
  const correction = {
    x: (snapped.x - bounds.x) * zoom,
    y: (snapped.y - bounds.y) * zoom,
  }
  if (correction.x || correction.y) {
    node.absolutePosition({ x: current.x + correction.x, y: current.y + correction.y })
  }
  return correction
}

const beginGridResize = () => {
  gridResizeSession = null
  if (!gridSnapEnabled.value || !transformer || !worldCameraGroup || isBatchSelection.value) return
  const object = selectedObject.value
  const anchor = transformer.getActiveAnchor()
  if (
    !object
    || object.type === 'group'
    || object.type === 'effect'
    || object.aspectRatioLocked
    || !anchor
    || anchor === 'rotater'
  ) return
  const node = objectNodes.get(object.id)
  const baseBoxWidth = Math.abs(transformer.width())
  const baseBoxHeight = Math.abs(transformer.height())
  if (!node || baseBoxWidth <= 0 || baseBoxHeight <= 0) return
  const liveState = props.store.state.liveState
  const zoom = Math.max(0.01, worldCameraGroup.scaleX())
  const camera = worldCameraGroup.absolutePosition()
  gridResizeSession = {
    anchor,
    gridSize: liveState.gridSize,
    baseWidth: Math.max(0.5, object.transform.width * Math.abs(node.scaleX())),
    baseHeight: Math.max(0.5, object.transform.height * Math.abs(node.scaleY())),
    baseBoxWidth,
    baseBoxHeight,
    gridOriginX: camera.x - liveState.fieldWidth * WORLD_UNIT_PX * zoom / 2,
    gridOriginY: camera.y - liveState.fieldHeight * WORLD_UNIT_PX * zoom / 2,
    gridStepPx: stageGridStep() * zoom,
    lockedWidthCells: null,
    lockedHeightCells: null,
  }
  setGridSnapPreview(true)
}

const finishGridResize = () => {
  if (!gridResizeSession) return
  gridResizeSession = null
  setGridSnapPreview(false)
}

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
  drawWorldLayers()
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
  drawWorldLayers()
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
const objectEntranceTweens = new Map<string, Konva.Tween>()
const pendingObjectEntrances = new Set<string>()
type StageMediaSource = HTMLImageElement | HTMLVideoElement
const activeAnimatedMedia = new Set<StageMediaSource>()
const videoLoopStates = new WeakMap<HTMLVideoElement, { loopCount: number | null, completed: number }>()
let mediaAnimation: Konva.Animation | null = null
let multiDrag: {
  driverId: string
  driverStart: { x: number, y: number }
  nodes: Map<string, { node: Konva.Group, absolute: { x: number, y: number } }>
} | null = null
let selectionGroupDrag: {
  start: { x: number, y: number }
  nodes: Map<string, { node: Konva.Group, absolute: { x: number, y: number } }>
} | null = null
let batchTransformRootIds: string[] | null = null

const drawWorldLayers = (immediate = false) => {
  if (immediate) {
    worldLayer?.draw()
    objectRootLayers.forEach(({ layer }) => layer.draw())
    worldOverlayLayer?.draw()
    gridTopLayer?.draw()
    return
  }
  worldLayer?.batchDraw()
  objectRootLayers.forEach(({ layer }) => layer.batchDraw())
  worldOverlayLayer?.batchDraw()
  gridTopLayer?.batchDraw()
}

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
  directImageSource: StageMediaSource | null
  directImageSignature: string
  debugDrawCount: number
}

let backgroundSlot: SurfaceSlot | null = null
let foregroundSlot: SurfaceSlot | null = null

const selectedObject = computed(() => {
  const id = props.store.state.selectedObjectId
  const object = id ? props.store.activeObjects.value[id] || null : null
  return isTheaterEffectObject(object) || !canEditObject(object) ? null : object
})

const isStaticImageObject = (object: StageObject | null | undefined): object is StageObject & { type: 'image' } => (
  object?.type === 'image'
  && Boolean(object.image)
  && object.image?.animated !== true
  && !object.image?.mimeType?.startsWith('video/')
)

const isEntranceConfigurableObject = (object: StageObject | null | undefined): object is StageObject => (
  object?.type === 'text' || object?.type === 'image'
)

const supportsStageEntrance = (object: StageObject | null | undefined): object is StageObject => (
  object?.type === 'text' || isStaticImageObject(object)
)

const entranceConfigFor = (object: StageObject): StageEntranceConfig => normalizeStageEntranceConfig(object.metadata?.entrance)

const textEntrancePlaybacks = reactive<Record<string, StageEntrancePlayback>>({})
const textEntranceTimers = new Map<string, number>()
const departingStageObjects = reactive<Record<string, StageObject>>({})
const networkVisibilityTargets = new Map<string, boolean>()
const playedVisibilityTriggerIds = new Set<string>()
const playedVisibilityTriggerOrder: string[] = []
let textEntranceToken = 0

const finishDepartingObject = (objectId: string) => {
  if (!departingStageObjects[objectId]) return
  delete departingStageObjects[objectId]
  void nextTick(() => syncObjects())
}

const playTextTransition = (object: StageObject, direction: 'enter' | 'exit', force = false) => {
  if (object.type !== 'text' || (direction === 'enter' && !object.visible)) return
  const config = entranceConfigFor(object)
  if (!force && config.preset === 'none') return
  const previousTimer = textEntranceTimers.get(object.id)
  if (previousTimer !== undefined) window.clearTimeout(previousTimer)
  const token = ++textEntranceToken
  textEntrancePlaybacks[object.id] = { ...config, direction, token }
  textEntranceTimers.delete(object.id)
  if (direction === 'enter') {
    finishDepartingObject(object.id)
    return
  }
  const timer = window.setTimeout(() => {
    textEntranceTimers.delete(object.id)
    if (textEntrancePlaybacks[object.id]?.token === token) delete textEntrancePlaybacks[object.id]
    finishDepartingObject(object.id)
  }, config.durationMs)
  textEntranceTimers.set(object.id, timer)
}

const finishObjectTransition = (object: StageObject, node: Konva.Group) => {
  objectEntranceTweens.get(object.id)?.destroy()
  objectEntranceTweens.delete(object.id)
  node.setAttrs({
    x: object.transform.x * WORLD_UNIT_PX,
    y: object.transform.y * WORLD_UNIT_PX,
    rotation: object.transform.rotation,
    scaleX: object.transform.scaleX,
    scaleY: object.transform.scaleY,
    opacity: 1,
  })
  node.setAttr('clipX', undefined)
  node.setAttr('clipY', undefined)
  node.setAttr('clipWidth', undefined)
  node.setAttr('clipHeight', undefined)
  node.visible(object.visible)
}

const playObjectTransition = (object: StageObject, node: Konva.Group, direction: 'enter' | 'exit', force = false) => {
  if (!isStaticImageObject(object) || (direction === 'enter' && !object.visible)) return
  const config = entranceConfigFor(object)
  if (!force && config.preset === 'none') return
  const image = node.findOne<Konva.Image>('.theater-object-image')
  if (direction === 'enter' && !image?.image()) {
    pendingObjectEntrances.add(object.id)
    return
  }
  pendingObjectEntrances.delete(object.id)
  finishObjectTransition(object, node)
  if (direction === 'enter') finishDepartingObject(object.id)
  if (config.preset === 'none') {
    node.visible(object.visible)
    return
  }
  const target = {
    x: object.transform.x * WORLD_UNIT_PX,
    y: object.transform.y * WORLD_UNIT_PX,
    rotation: object.transform.rotation,
    scaleX: object.transform.scaleX,
    scaleY: object.transform.scaleY,
    opacity: 1,
  }
  const duration = config.durationMs / 1_000
  const attrs: Record<string, number> = direction === 'enter' ? { ...target } : {}
  if (config.preset === 'fade') {
    node.opacity(direction === 'enter' ? 0 : 1)
    attrs.opacity = direction === 'enter' ? 1 : 0
  } else if (config.preset === 'slide') {
    if (direction === 'enter') {
      node.setAttrs({ y: target.y + WORLD_UNIT_PX * 0.75, opacity: 0 })
      attrs.y = target.y
      attrs.opacity = 1
    } else {
      attrs.y = target.y + WORLD_UNIT_PX * 0.75
      attrs.opacity = 0
    }
  } else if (config.preset === 'zoom') {
    if (direction === 'enter') {
      node.setAttrs({ scaleX: target.scaleX * 0.92, scaleY: target.scaleY * 0.92, opacity: 0 })
      attrs.scaleX = target.scaleX
      attrs.scaleY = target.scaleY
      attrs.opacity = 1
    } else {
      attrs.scaleX = target.scaleX * 0.92
      attrs.scaleY = target.scaleY * 0.92
      attrs.opacity = 0
    }
  } else if (config.preset === 'mask') {
    const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
    const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
    node.setAttrs({ clipX: 0, clipY: 0, clipWidth: direction === 'enter' ? 0.01 : width, clipHeight: height })
    attrs.clipWidth = direction === 'enter' ? width : 0.01
  }
  const tween = new Konva.Tween({
    node,
    duration,
    easing: Konva.Easings.EaseOut,
    ...attrs,
    onFinish: () => {
      finishObjectTransition(object, node)
      finishDepartingObject(object.id)
    },
    onUpdate: () => drawWorldLayers(),
  })
  objectEntranceTweens.set(object.id, tween)
  node.visible(true)
  drawWorldLayers()
  tween.play()
}

const playObjectEntrance = (object: StageObject, node: Konva.Group, force = false) => playObjectTransition(object, node, 'enter', force)

const playStageObjectTransition = (
  object: StageObject,
  node: Konva.Group,
  direction: 'enter' | 'exit',
  force = false,
) => {
  if (object.type === 'text') playTextTransition(object, direction, force)
  else playObjectTransition(object, node, direction, force)
}

const playStageObjectEntrance = (object: StageObject, node: Konva.Group, force = false) => (
  playStageObjectTransition(object, node, 'enter', force)
)

const playVisibilityTransitions = (changes: Array<{ objectId: string, visible: boolean }>, triggerId: string) => {
  const normalizedTriggerId = triggerId.trim()
  if (!normalizedTriggerId || playedVisibilityTriggerIds.has(normalizedTriggerId)) return
  playedVisibilityTriggerIds.add(normalizedTriggerId)
  playedVisibilityTriggerOrder.push(normalizedTriggerId)
  if (playedVisibilityTriggerOrder.length > 256) {
    playedVisibilityTriggerIds.delete(playedVisibilityTriggerOrder.shift()!)
  }
  changes.forEach(({ objectId, visible }) => {
    const object = props.store.activeObjects.value[objectId]
    if (visible && object && departingStageObjects[objectId]) {
      networkVisibilityTargets.delete(objectId)
      const node = objectNodes.get(objectId)
      if (object.type === 'text') playTextTransition(object, 'enter')
      else if (node) playStageObjectTransition(object, node, 'enter')
      return
    }
    if (!object || object.visible === visible) return
    if (visible) {
      networkVisibilityTargets.set(objectId, true)
      return
    }
    if (!supportsStageEntrance(object) || entranceConfigFor(object).preset === 'none') return
    const node = objectNodes.get(objectId)
    if (object.type !== 'text' && !node) return
    networkVisibilityTargets.set(objectId, false)
    const departing = cloneStageData(object)
    departing.visible = false
    departingStageObjects[objectId] = departing
    if (object.type === 'text') playTextTransition(departing, 'exit')
    else playStageObjectTransition(departing, node!, 'exit')
  })
}

const selectedEntranceConfig = computed(() => {
  const object = selectedObject.value
  return isEntranceConfigurableObject(object) ? entranceConfigFor(object) : normalizeStageEntranceConfig(null)
})
const selectedObjectSupportsEntrance = computed(() => isEntranceConfigurableObject(selectedObject.value))
const selectedObjectCanPreviewEntrance = computed(() => supportsStageEntrance(selectedObject.value))
const canEditSelectedTransform = computed(() => Boolean(selectedObject.value) && (
  canEditAllObjects.value || !selectedObject.value!.locked
))
const entrancePresetOptions: Array<{ label: string, value: StageEntrancePreset }> = [
  { label: '无', value: 'none' },
  { label: '淡入', value: 'fade' },
  { label: '滑入', value: 'slide' },
  { label: '缩放', value: 'zoom' },
  { label: '遮罩揭示', value: 'mask' },
]

const updateSelectedEntrance = (patch: Partial<StageEntranceConfig>) => {
  const object = selectedObject.value
  if (!isEntranceConfigurableObject(object)) return
  object.metadata = { ...object.metadata, entrance: normalizeStageEntranceConfig({ ...entranceConfigFor(object), ...patch }) }
}

const updateSelectedEntrancePreset = (value: string) => updateSelectedEntrance({ preset: value as StageEntrancePreset })
const updateSelectedEntranceDuration = (value: number | null) => {
  if (value !== null) updateSelectedEntrance({ durationMs: value })
}

const previewSelectedEntrance = () => {
  const object = selectedObject.value
  if (!supportsStageEntrance(object)) return
  const node = objectNodes.get(object.id)
  if (!node) return
  window.requestAnimationFrame(() => {
    window.requestAnimationFrame(() => {
      if (selectedObject.value?.id === object.id && objectNodes.get(object.id) === node) {
        playStageObjectEntrance(object, node, true)
      }
    })
  })
}
const sequenceEditorActionId = ref('')
const sequenceEditorVisible = computed({
  get: () => Boolean(sequenceEditorActionId.value && selectedObject.value),
  set: (value) => { if (!value) sequenceEditorActionId.value = '' },
})
const editingSequenceAction = computed(() => {
  const action = selectedObject.value?.actions.find((item) => item.id === sequenceEditorActionId.value)
  return action && isStageSequenceAction(action) ? action : null
})
const openSequenceEditor = (actionId: string) => {
  const object = selectedObject.value
  if (!object || !canEditAllObjects.value) return
  object.interactive = true
  sequenceEditorActionId.value = actionId
}
const randomTableEditorActionId = ref('')
const randomTableEditorVisible = computed({
  get: () => Boolean(randomTableEditorActionId.value && selectedObject.value),
  set: (value) => { if (!value) randomTableEditorActionId.value = '' },
})
const editingRandomTableAction = computed(() => {
  const action = selectedObject.value?.actions.find((item) => item.id === randomTableEditorActionId.value)
  return action?.type === 'chat.random-table' ? action : null
})
const openRandomTableEditor = (actionId: string) => {
  const object = selectedObject.value
  const action = object?.actions.find((item) => item.id === actionId)
  if (!object || action?.type !== 'chat.random-table' || !canEditAllObjects.value) return
  object.interactive = true
  randomTableEditorActionId.value = actionId
}
const saveRandomTable = (payload: Extract<StageAction, { type: 'chat.random-table' }>['payload']) => {
  const object = selectedObject.value
  const action = object?.actions.find((item) => item.id === randomTableEditorActionId.value)
  if (!object || action?.type !== 'chat.random-table') return
  props.store.beginObjectEdit('编辑随机表')
  action.payload = payload
  object.interactive = true
  props.store.commitObjectEdit()
}
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
const stageObjects = computed<Record<string, StageObject>>(() => ({
  ...Object.fromEntries(Object.entries(props.store.activeObjects.value).filter(([objectId, object]) => (
    networkVisibilityTargets.get(objectId) !== false || !object.visible || Boolean(departingStageObjects[objectId])
  ))),
  ...departingStageObjects,
}))
const selectedObjects = props.store.selectedObjects
const selectedIdSet = computed(() => new Set(props.store.selection.selectedIds))
const isBatchSelection = computed(() => props.store.selection.bulkMode && selectedObjects.value.length > 1)
const batchMoveBlocked = computed(() => isBatchSelection.value && props.store.selectionGroup.value.lockedIds.length > 0)

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
const batchBooleanObjects = (key: BatchBooleanKey) => selectedObjects.value
  .filter((object) => object.type !== 'group' || (key !== 'interactive' && key !== 'editable'))
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
const toggleBatchQuickFlag = (key: BatchBooleanKey) => {
  const objects = batchBooleanObjects(key)
  if (!objects.length) return
  const next = !objects.every((object) => object[key])
  props.store.patchSelectedObjects({ [key]: next })
  void nextTick(() => {
    syncObjects()
    updateTransformer()
  })
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

const selectedMovementRootIds = () => props.store.selectionGroup.value.rootIds

const parentOptions = computed(() => Object.values(props.store.activeObjects.value)
  .filter((object) => object.type === 'group'
    && object.id !== selectedObject.value?.id
    && (!selectedObject.value || props.store.canSetParent(selectedObject.value.id, object.id)))
  .map((object) => ({ label: object.name, value: object.id })))

const persistedCollapsedGroupIds = ref<Set<string>>(new Set())
const temporaryExpandedGroupIds = ref<Set<string>>(new Set())
const layerFilterOpen = ref(false)
const layerNameFilter = ref('')
const layerHiddenOnly = ref(false)
const layerSceneFixedOnly = ref(false)
let editorStateLoadVersion = 0

const layerObjects = computed(() => Object.values(props.store.activeObjects.value)
  .filter((object) => !isTheaterEffectObject(object)))
const normalizedLayerNameFilter = computed(() => layerNameFilter.value.trim().toLocaleLowerCase())
const layerFilterActive = computed(() => Boolean(
  normalizedLayerNameFilter.value || layerHiddenOnly.value || layerSceneFixedOnly.value,
))
const layerFilterActiveCount = computed(() => Number(Boolean(normalizedLayerNameFilter.value))
  + Number(layerHiddenOnly.value)
  + Number(layerSceneFixedOnly.value))
const layerFilterMatchIds = computed(() => {
  const name = normalizedLayerNameFilter.value
  return new Set(layerObjects.value
    .filter((object) => (!name || object.name.toLocaleLowerCase().includes(name))
      && (!layerHiddenOnly.value || !object.visible)
      && (!layerSceneFixedOnly.value || props.store.isSceneFixedObject(object.id)))
    .map((object) => object.id))
})
const layerFilterContextGroupIds = computed(() => {
  if (!layerFilterActive.value) return new Set<string>()
  const groups = new Set<string>()
  const objects = props.store.activeObjects.value
  layerFilterMatchIds.value.forEach((objectId) => {
    let parentId = objects[objectId]?.parentId
    while (parentId) {
      const parent = objects[parentId]
      if (!parent) break
      if (parent.type === 'group') groups.add(parent.id)
      parentId = parent.parentId
    }
  })
  return groups
})
const layerExpandedGroupIds = computed(() => new Set([
  ...temporaryExpandedGroupIds.value,
  ...layerFilterContextGroupIds.value,
]))
const layerRows = computed(() => {
  const rows = buildStageLayerRows(
    layerObjects.value,
    persistedCollapsedGroupIds.value,
    layerExpandedGroupIds.value,
  )
  if (!layerFilterActive.value) return rows
  const includedIds = new Set([
    ...layerFilterMatchIds.value,
    ...layerFilterContextGroupIds.value,
  ])
  return rows.filter((row) => includedIds.has(row.object.id))
})
const canReorderLayerRows = computed(() => canEditAllObjects.value && !layerFilterActive.value)

const toggleLayerFilterPanel = () => {
  layerFilterOpen.value = !layerFilterOpen.value
}

const clearLayerFilters = () => {
  layerNameFilter.value = ''
  layerHiddenOnly.value = false
  layerSceneFixedOnly.value = false
}

watch(layerFilterActive, (active) => {
  if (active) layerFilterOpen.value = true
})

const selectedLayerExpansionIds = computed(() => stageLayerSelectionExpansionIds(
  props.store.activeObjects.value,
  props.store.state.selectedObjectId,
))

watch(() => [...selectedLayerExpansionIds.value].sort().join('\u0000'), () => {
  temporaryExpandedGroupIds.value = new Set(selectedLayerExpansionIds.value)
}, { immediate: true })

const loadTheaterGroupEditorState = async () => {
  const version = ++editorStateLoadVersion
  if (!props.worldId || (props.scopeType !== 'world' && !props.channelId) || !canEditAllObjects.value) {
    persistedCollapsedGroupIds.value = new Set()
    return
  }
  try {
    const response = await api.get<{ state?: { collapsedGroupIds?: unknown } }>(theaterEditorStatePath())
    if (version !== editorStateLoadVersion) return
    const collapsedGroupIds = response.data?.state?.collapsedGroupIds
    const ids = Array.isArray(collapsedGroupIds)
      ? collapsedGroupIds.filter((id): id is string => typeof id === 'string' && Boolean(id))
      : []
    persistedCollapsedGroupIds.value = new Set(ids)
  } catch (error) {
    if (version !== editorStateLoadVersion) return
    persistedCollapsedGroupIds.value = new Set()
    stageMessage.warning(theaterAudioErrorMessage(error, '读取图层折叠状态失败'))
  }
}

const isLayerGroupCollapsed = (object: StageObject) => object.type === 'group'
  && persistedCollapsedGroupIds.value.has(object.id)
  && !layerExpandedGroupIds.value.has(object.id)

const toggleLayerGroupCollapsed = async (object: StageObject) => {
  if (object.type !== 'group' || !canEditAllObjects.value) return
  const previousPersisted = new Set(persistedCollapsedGroupIds.value)
  const previousTemporary = new Set(temporaryExpandedGroupIds.value)
  const collapsed = !isLayerGroupCollapsed(object)
  const nextPersisted = new Set(previousPersisted)
  if (collapsed) nextPersisted.add(object.id)
  else nextPersisted.delete(object.id)
  const nextTemporary = new Set(previousTemporary)
  nextTemporary.delete(object.id)
  persistedCollapsedGroupIds.value = nextPersisted
  temporaryExpandedGroupIds.value = nextTemporary
  try {
    await api.put(theaterEditorStatePath(object.id), { collapsed })
  } catch (error) {
    persistedCollapsedGroupIds.value = previousPersisted
    temporaryExpandedGroupIds.value = previousTemporary
    stageMessage.error(theaterAudioErrorMessage(error, '保存图层折叠状态失败'))
  }
}

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
  const object = getObject(objectId)
  return object && object.type !== 'group' ? object.id : null
}

const editableCanvasSelectionTarget = (objectId: string) => {
  const targetId = canvasSelectionTarget(objectId)
  return targetId && canEditObject(getObject(targetId)) ? targetId : null
}

const addAction = (type: StageAction['type']) => {
  const object = selectedObject.value
  if (!object || !canEditAllObjects.value) return
  object.interactive = true
  if (type === 'action.sequence') {
    const sequence = createStageSequenceAction(props.store.state.activeSceneId, object.id)
    props.store.addObjectAction(object.id, sequence)
    sequenceEditorActionId.value = sequence.id
    return
  }
  const action: StageAction = type === 'chat.send'
    ? { id: actionId(), type, schedule: createDefaultStageActionSchedule(), payload: { content: '舞台消息' } }
    : type === 'chat.random-table'
      ? {
          id: actionId(),
          type,
          schedule: createDefaultStageActionSchedule(),
          payload: {
            name: '随机表',
            formula: '1d6',
            entries: Array.from({ length: 6 }, (_, index) => ({
              min: index + 1,
              max: index + 1,
              text: `结果${index + 1}`,
            })),
          },
        }
      : type === 'chat.insert'
        ? { id: actionId(), type, schedule: createDefaultStageActionSchedule(), payload: { content: '舞台台词' } }
        : type === 'scene.apply'
          ? { id: actionId(), type, schedule: createDefaultStageActionSchedule(), payload: { sceneId: props.store.state.activeSceneId } }
          : type === 'effect.play'
            ? { id: actionId(), type, schedule: createDefaultStageActionSchedule(), payload: { effectId: effectActionOptions.value[0]?.value || '' } }
            : { id: actionId(), type, schedule: createDefaultStageActionSchedule(), payload: { objectId: object.id } }
  if (action.type === 'effect.play' && !action.payload.effectId) return
  if (!props.store.addObjectAction(object.id, action)) return
  if (action.type === 'chat.random-table') randomTableEditorActionId.value = action.id
}

const actionDelaySeconds = (milliseconds: number | undefined) => (
  typeof milliseconds === 'number' && Number.isFinite(milliseconds) ? milliseconds / 1_000 : 0
)
const updateActionDelaySeconds = (actionId: string, value: number | null) => {
  const object = selectedObject.value
  if (!object) return
  props.store.setObjectActionSchedule(object.id, actionId, { delayMs: (value ?? 0) * 1_000 })
}

const actionRowElements = new Map<string, HTMLElement>()
const draggingActionId = ref('')
const actionDropIndex = ref(-1)
let actionDragFrame: number | null = null
let actionDragY = 0
let actionDragSession: { pointerId: number, objectId: string, actionId: string } | null = null

const setActionRowElement = (actionId: string, element: Element | null) => {
  if (element instanceof HTMLElement) actionRowElements.set(actionId, element)
  else actionRowElements.delete(actionId)
}

const updateActionDropIndex = () => {
  actionDragFrame = null
  if (!actionDragSession) return
  const object = props.store.activeObjects.value[actionDragSession.objectId]
  if (!object) return
  const rows = object.actions
    .map((action) => actionRowElements.get(action.id))
    .filter((element): element is HTMLElement => Boolean(element))
  let index = rows.length
  for (let current = 0; current < rows.length; current += 1) {
    const rect = rows[current].getBoundingClientRect()
    if (actionDragY < rect.top + rect.height / 2) {
      index = current
      break
    }
  }
  actionDropIndex.value = index
}

const startActionPointerDrag = (event: PointerEvent, actionId: string) => {
  const object = selectedObject.value
  if (!object || event.button !== 0 || actionDragSession) return
  const handle = event.currentTarget as HTMLElement
  event.preventDefault()
  handle.setPointerCapture(event.pointerId)
  actionDragSession = { pointerId: event.pointerId, objectId: object.id, actionId }
  draggingActionId.value = actionId
  actionDragY = event.clientY
  updateActionDropIndex()
}

const moveActionPointerDrag = (event: PointerEvent) => {
  if (!actionDragSession || actionDragSession.pointerId !== event.pointerId) return
  actionDragY = event.clientY
  if (actionDragFrame === null) actionDragFrame = window.requestAnimationFrame(updateActionDropIndex)
}

const finishActionPointerDrag = (event: PointerEvent, cancelled = false) => {
  const session = actionDragSession
  if (!session || session.pointerId !== event.pointerId) return
  if (actionDragFrame !== null) {
    window.cancelAnimationFrame(actionDragFrame)
    updateActionDropIndex()
  }
  const targetIndex = actionDropIndex.value
  const handle = event.currentTarget as HTMLElement
  actionDragSession = null
  draggingActionId.value = ''
  actionDropIndex.value = -1
  if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId)
  if (cancelled || targetIndex < 0) return
  const object = props.store.activeObjects.value[session.objectId]
  const sourceIndex = object?.actions.findIndex((action) => action.id === session.actionId) ?? -1
  if (sourceIndex < 0) return
  const adjustedIndex = targetIndex > sourceIndex ? targetIndex - 1 : targetIndex
  props.store.moveObjectAction(session.objectId, session.actionId, adjustedIndex)
}

const moveActionByKeyboard = (actionId: string, offset: -1 | 1) => {
  const object = selectedObject.value
  if (!object) return
  const sourceIndex = object.actions.findIndex((action) => action.id === actionId)
  if (sourceIndex < 0) return
  props.store.moveObjectAction(object.id, actionId, sourceIndex + offset)
}

const triggerObjectActions = (object: StageObject) => {
  if (!canInteractObject(object)) return
  const pointer = worldCameraGroup?.getRelativePointerPosition()
  const execution = {
    id: actionId(),
    mode: object.metadata.actionExecutionMode === 'sequential' ? 'sequential' as const : 'parallel' as const,
    total: object.actions.length,
  }
  object.actions.forEach((action, index) => {
    const parsed = stageActionSchema.safeParse(action)
    if (!parsed.success) return
    emit('actionTriggered', {
      objectId: object.id,
      actionId: parsed.data.id,
      action: parsed.data,
      execution: { ...execution, index },
      ...(pointer ? {
        pointer: {
          x: Number((pointer.x / WORLD_UNIT_PX).toFixed(6)),
          y: Number((pointer.y / WORLD_UNIT_PX).toFixed(6)),
        },
      } : {}),
    })
  })
}

const triggerSingleObjectAction = (object: StageObject | null | undefined, action: StageAction) => {
  if (!object) return
  if (!canTriggerActions.value) {
    stageMessage.warning('没有执行舞台动作的权限')
    return
  }
  if (!object.visible) {
    stageMessage.warning('隐藏组件不能执行动作')
    return
  }
  if (!object.interactive) {
    stageMessage.warning('请先开启组件的成员交互')
    return
  }
  if (!isStageActionTarget(object.type)) {
    stageMessage.warning('当前组件类型不支持点击动作')
    return
  }
  const parsed = stageActionSchema.safeParse(action)
  if (!parsed.success) {
    stageMessage.warning('动作配置无效，无法执行')
    return
  }
  emit('actionTriggered', {
    objectId: object.id,
    actionId: parsed.data.id,
    direct: true,
    action: parsed.data,
    execution: {
      id: actionId(),
      mode: 'parallel',
      total: 1,
      index: 0,
    },
  })
}

const objectNodeIntersectsStagePoint = (node: Konva.Group, point: Konva.Vector2d) => {
  if (!stage || !node.isVisible()) return false
  const bounds = node.getClientRect({ relativeTo: stage, skipShadow: true })
  if (
    point.x < bounds.x
    || point.y < bounds.y
    || point.x > bounds.x + bounds.width
    || point.y > bounds.y + bounds.height
  ) return false
  return node.find<Konva.Shape>('Shape').some(shape => shape.intersects(point))
}

const resolveObjectActionTarget = (point: Konva.Vector2d) => [...objectNodes.entries()]
  .map(([objectId, node]) => ({ object: getObject(objectId), node }))
  .filter((entry): entry is { object: StageObject, node: Konva.Group } => canInteractObject(entry.object))
  .sort((left, right) => right.node.getAbsoluteZIndex() - left.node.getAbsoluteZIndex())
  .find(({ node }) => objectNodeIntersectsStagePoint(node, point))
  ?.object || null

const applyCamera = () => {
  if (!stage) return
  const position = {
    x: stage.width() / 2 + props.store.state.camera.x,
    y: stage.height() / 2 + props.store.state.camera.y,
  }
  const scale = { x: props.store.state.camera.zoom, y: props.store.state.camera.zoom }
  // Background fills viewport independently; world, foreground, and top grid follow camera.
  backgroundCameraGroup?.position({ x: 0, y: 0 })
  backgroundCameraGroup?.scale({ x: 1, y: 1 })
  for (const group of [worldCameraGroup, worldOverlayCameraGroup, foregroundCameraGroup, gridTopCameraGroup]) {
    group?.position(position)
    group?.scale(scale)
  }
  sceneMorphLayers.forEach(({ camera }) => {
    camera.position(position)
    camera.scale(scale)
  })
  sceneMorphTextCameras.forEach((camera) => {
    camera.style.transform = `translate(${position.x}px, ${position.y}px) scale(${scale.x})`
  })
  objectRootLayers.forEach(({ camera }) => {
    camera.position(position)
    camera.scale(scale)
  })
  // Background camera stays fixed; camera movement does not invalidate its canvas.
  drawWorldLayers()
  foregroundLayer?.batchDraw()
  interactionLayer?.batchDraw()
  sceneMorphLayers.forEach(({ layer }) => layer.batchDraw())
}

const selectedMovementNodes = () => selectedMovementRootIds()
  .map((id) => ({ id, node: objectNodes.get(id) }))
  .filter((entry): entry is { id: string, node: Konva.Group } => Boolean(entry.node))

const syncSelectionGroupHitArea = (nodes: Konva.Group[]) => {
  if (!selectionGroupHitArea || !stage || !nodes.length) {
    selectionGroupHitArea?.setAttrs({ visible: false, listening: false, draggable: false })
    selectionQuickBar.visible = false
    return
  }
  const boxes = nodes.map((node) => node.getClientRect({
    relativeTo: stage!,
    skipShadow: true,
  }))
  const left = Math.min(...boxes.map((box) => box.x))
  const top = Math.min(...boxes.map((box) => box.y))
  const right = Math.max(...boxes.map((box) => box.x + box.width))
  const bottom = Math.max(...boxes.map((box) => box.y + box.height))
  const padding = 8
  const center = (left + right) / 2
  const barWidth = 154
  const barHeight = 36
  const preferredTop = top - barHeight - 8
  const horizontalMargin = barWidth / 2 + 4
  selectionQuickBar.left = Math.min(
    Math.max(center, horizontalMargin),
    Math.max(horizontalMargin, viewportSize.value.width - horizontalMargin),
  )
  selectionQuickBar.top = preferredTop >= 4 ? preferredTop : Math.min(bottom + 8, Math.max(4, viewportSize.value.height - barHeight - 4))
  selectionQuickBar.visible = true
  selectionGroupHitArea.setAttrs({
    x: left - padding,
    y: top - padding,
    width: Math.max(1, right - left + padding * 2),
    height: Math.max(1, bottom - top + padding * 2),
    visible: true,
    listening: !batchMoveBlocked.value,
    draggable: !batchMoveBlocked.value,
  })
}

const applyObjectNodeTransform = (node: Konva.Group, object: StageObject) => {
  if (object.type === 'group') {
    object.transform.x = Number((node.x() / WORLD_UNIT_PX).toFixed(6))
    object.transform.y = Number((node.y() / WORLD_UNIT_PX).toFixed(6))
    object.transform.rotation = Number(node.rotation().toFixed(6))
    object.transform.scaleX = Number(Math.min(100, Math.max(0.01, node.scaleX())).toFixed(6))
    object.transform.scaleY = Number(Math.min(100, Math.max(0.01, node.scaleY())).toFixed(6))
    return
  }
  object.transform.width = Number((Math.max(12, object.transform.width * WORLD_UNIT_PX * node.scaleX()) / WORLD_UNIT_PX).toFixed(6))
  object.transform.height = Number((Math.max(12, object.transform.height * WORLD_UNIT_PX * node.scaleY()) / WORLD_UNIT_PX).toFixed(6))
  object.transform.rotation = Number(node.rotation().toFixed(6))
  object.transform.x = Number((node.x() / WORLD_UNIT_PX).toFixed(6))
  object.transform.y = Number((node.y() / WORLD_UNIT_PX).toFixed(6))
  object.transform.scaleX = 1
  object.transform.scaleY = 1
  node.scale({ x: 1, y: 1 })
  updateObjectNode(node, object)
}

const updateTransformer = () => {
  if (!transformer) return
  if (viewToolActive.value || activeCanvasTool.value || quickDeleteActive.value) {
    transformer.nodes([])
    transformer.visible(false)
    selectionGroupHitArea?.setAttrs({ visible: false, listening: false, draggable: false })
    selectionQuickBar.visible = false
    interactionLayer?.batchDraw()
    return
  }
  if (isBatchSelection.value) {
    const nodes = selectedMovementNodes().map(({ node }) => node)
    const proportional = batchBooleanObjects('aspectRatioLocked').some((object) => object.aspectRatioLocked)
    transformer.nodes(nodes)
    transformer.ignoreStroke(selectedObjects.value.some((object) => object.type === 'drawing'))
    transformer.visible(Boolean(nodes.length))
    transformer.padding(8)
    transformer.borderStrokeWidth(2)
    transformer.borderDash([6, 4])
    transformer.anchorSize(11)
    transformer.rotateAnchorOffset(32)
    transformer.keepRatio(proportional)
    transformer.enabledAnchors(batchMoveBlocked.value ? [] : proportional
      ? ['top-left', 'top-right', 'bottom-left', 'bottom-right']
      : [
          'top-left', 'top-center', 'top-right',
          'middle-left', 'middle-right',
          'bottom-left', 'bottom-center', 'bottom-right',
        ])
    transformer.rotateEnabled(!batchMoveBlocked.value)
    transformer.forceUpdate()
    syncSelectionGroupHitArea(nodes)
    interactionLayer?.batchDraw()
    return
  }
  selectionGroupHitArea?.setAttrs({ visible: false, listening: false, draggable: false })
  selectionQuickBar.visible = false
  const object = selectedObject.value
  const node = object && canEditObject(object) && !object.locked ? objectNodes.get(object.id) : null
  transformer.nodes(node ? [node] : [])
  transformer.ignoreStroke(object?.type === 'drawing')
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

const selectLayerObject = (objectId: string, additive = false) => {
  selectObject(objectId, additive)
  if (props.store.state.selectedObjectId !== objectId) return
  temporaryExpandedGroupIds.value = new Set(stageLayerSelectionExpansionIds(
    props.store.activeObjects.value,
    objectId,
  ))
}

const openObjectInspector = (objectId: string) => {
  if (!canEditObject(getObject(objectId))) return
  const keepBatchSelection = props.store.selection.bulkMode && selectedIdSet.value.has(objectId)
  if (!keepBatchSelection) selectObject(objectId)
  if (getObject(objectId)?.type === 'group') {
    temporaryExpandedGroupIds.value = new Set(stageLayerSelectionExpansionIds(props.store.activeObjects.value, objectId))
  }
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

const theaterMediaScope = (scope = captureTheaterRequestScope()) => ({
  urlBase: String(urlBase),
  worldId: scope.worldId,
  channelId: scope.channelId,
  scopeType: scope.scopeType,
})

const resolveTheaterResourceUrl = (resourceId: string, variant = 'original') => {
  const normalizedResourceId = resourceId.trim()
  if (!normalizedResourceId) return ''
  const resourceBase = urlBase.startsWith('//') ? `${window.location.protocol}${urlBase}` : urlBase
  return `${resourceBase.replace(/\/$/, '')}${theaterResourceContentPath(theaterMediaScope(), normalizedResourceId, variant || 'original')}`
}

const resolveTheaterStageMedia = (imageRef: StageImageRef) => resolveTheaterStageMediaLocation(imageRef, theaterMediaScope())

const stageMediaDimensions = (source: StageMediaSource) => isVideoSource(source)
  ? { width: source.videoWidth, height: source.videoHeight }
  : { width: source.naturalWidth || source.width, height: source.naturalHeight || source.height }

const syncMediaAnimation = () => {
  if (!activeAnimatedMedia.size) {
    mediaAnimation?.stop()
    return
  }
  if (!mediaAnimation && backgroundLayer && worldLayer && worldOverlayLayer && foregroundLayer && gridTopLayer) {
    mediaAnimation = new Konva.Animation(() => {}, [
      backgroundLayer,
      worldLayer,
      ...[...objectRootLayers.values()].map(({ layer }) => layer),
      worldOverlayLayer,
      foregroundLayer,
      gridTopLayer,
    ])
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
    source.onloadeddata = null
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
    source.onloadeddata = () => {
      source.pause()
      source.currentTime = 0
      theaterMediaDebug('video loadeddata', { width: source.videoWidth, height: source.videoHeight, url: location.url })
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
      void source.decode().catch(() => undefined).then(() => {
        if (controller.signal.aborted) return
        stageMediaRequestControllers.delete(source)
        onReady(source)
      })
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
  const refs: Array<{ key: string, imageRef: StageImageRef, blocksSceneReveal: boolean }> = []
  if (scene.state.background) refs.push({ key: 'surface:background', imageRef: scene.state.background, blocksSceneReveal: true })
  if (scene.state.foreground) refs.push({ key: 'surface:foreground', imageRef: scene.state.foreground, blocksSceneReveal: true })
  Object.values({ ...scene.state.sceneObjects, ...props.store.state.persistentObjects })
    .filter((object) => object.type === 'image' && Boolean(object.image))
    .forEach((object) => refs.push({ key: `object:${object.id}`, imageRef: object.image!, blocksSceneReveal: true }))
  scene.state.sceneOverlays.forEach((binding) => {
    const media = binding.media
    if (!media?.resourceId) return
    refs.push({
      key: `overlay:${binding.id}`,
      imageRef: {
        resourceId: media.resourceId,
        url: resolveTheaterResourceUrl(media.resourceId, media.variant),
        mimeType: media.mimeType,
        animated: media.animated,
        loopCount: media.loopCount,
      },
      blocksSceneReveal: false,
    })
  })
  return refs.flatMap(({ key, imageRef, blocksSceneReveal }) => {
    const location = resolveTheaterStageMedia(imageRef)
    return location ? [{ key, imageRef, location, blocksSceneReveal }] : []
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
  activations: Array<() => void>
  released: boolean
  ready: boolean
  timeout: number | null
}

let sceneMediaBatch: SceneMediaBatch | null = null
let sceneTransitionTimer: number | null = null

interface SceneMorphVisual {
  x: number
  y: number
  rotation: number
  scaleX: number
  scaleY: number
  opacity: number
  visible: boolean
  width: number
  height: number
}

interface SceneMorphItem {
  object: StageObject
  visual: SceneMorphVisual
  ghost: Konva.Group | null
  textGhost: HTMLElement | null
}

interface SceneMorphSnapshot {
  sceneId: string
  previous: Map<string, SceneMorphItem>
  matches: Map<string, SceneMorphItem>
  targetAttrs: Map<string, SceneMorphVisual>
  targetGhosts: Map<string, Konva.Group>
  targetTextGhosts: Map<string, HTMLElement>
  targetTextElements: Map<string, HTMLElement>
  animations: Animation[]
  backgroundGhost: Konva.Group | null
  textGhosts: HTMLElement[]
  generation: number
  started: boolean
  completed: boolean
}

let sceneMorphSnapshot: SceneMorphSnapshot | null = null
let sceneMorphTweens: Konva.Tween[] = []
let sceneMorphDelayTimers: number[] = []
let pendingSceneEntrances: { sceneId: string, objectIds: Set<string> } | null = null
const pendingTextEntranceIds = ref<string[]>([])
const sceneMorphTextHidden = ref(false)
const sceneMorphTextAnimating = ref(false)

interface SceneCompositeTransition {
  sceneId: string
  overlay: HTMLDivElement | null
  curtainOverlay: HTMLDivElement | null
  enter: StageSceneTransitionPhase
  exit: StageSceneTransitionPhase
  animations: Animation[]
  generation: number
  started: boolean
  completed: boolean
}

let sceneCompositeTransition: SceneCompositeTransition | null = null
let sceneTransitionGeneration = 0

const resetSceneVisualStyle = () => {
  const visual = sceneVisualRef.value
  if (!visual) return
  visual.style.removeProperty('visibility')
  visual.style.removeProperty('opacity')
  visual.style.removeProperty('transform')
  visual.style.removeProperty('filter')
  visual.style.removeProperty('clip-path')
}

const captureSceneTransitionOverlay = () => {
  if (!stage || !viewportRef.value) return null
  const interactionWasVisible = interactionLayer?.visible() !== false
  interactionLayer?.hide()
  interactionLayer?.draw()
  const snapshot = stage.toCanvas({
    width: stage.width(),
    height: stage.height(),
    pixelRatio: Math.min(2, window.devicePixelRatio || 1),
  })
  if (interactionWasVisible) interactionLayer?.show()
  interactionLayer?.draw()
  snapshot.style.width = '100%'
  snapshot.style.height = '100%'
  const overlay = document.createElement('div')
  overlay.className = 'theater-scene-transition-overlay'
  overlay.append(snapshot)
  const textOverlay = viewportRef.value.querySelector<HTMLElement>('.theater-scene-visual .theater-text-overlay')
  if (textOverlay) overlay.append(textOverlay.cloneNode(true))
  // Keep previous scene beneath incoming scene. Its exit animation becomes a true
  // backdrop instead of flattening local objects above target scene layers.
  viewportRef.value.insertBefore(overlay, sceneVisualRef.value)
  return overlay
}

const createSceneCurtainOverlay = () => {
  if (!viewportRef.value) return null
  const overlay = document.createElement('div')
  overlay.className = 'theater-scene-curtain-overlay'
  const left = document.createElement('i')
  left.className = 'theater-scene-curtain-panel is-left'
  const right = document.createElement('i')
  right.className = 'theater-scene-curtain-panel is-right'
  overlay.append(left, right)
  viewportRef.value.append(overlay)
  return overlay
}

const clearSceneCompositeTransition = () => {
  const transition = sceneCompositeTransition
  if (!transition) {
    resetSceneVisualStyle()
    return
  }
  transition.animations.forEach((animation) => animation.cancel())
  transition.overlay?.remove()
  transition.curtainOverlay?.remove()
  sceneCompositeTransition = null
  resetSceneVisualStyle()
}

const stageTextVisualElement = (objectId: string) => Array.from(
  sceneVisualRef.value?.querySelectorAll<HTMLElement>('[data-stage-object-id]') || [],
).find((element) => element.dataset.stageObjectId === objectId) || null

const sceneMorphRootFor = (rootId: string) => {
  const existing = sceneMorphLayers.get(rootId)
  if (existing) return existing.root
  if (!sceneMorphStage) return null
  const layer = new Konva.Layer({ listening: false })
  const camera = new Konva.Group({ listening: false })
  const root = new Konva.Group({ listening: false })
  camera.position(worldCameraGroup?.position() || { x: 0, y: 0 })
  camera.scale(worldCameraGroup?.scale() || { x: 1, y: 1 })
  camera.add(root)
  layer.add(camera)
  sceneMorphStage.add(layer)
  layer.getCanvas()._canvas.style.zIndex = String((rootStackingOrder.value[rootId] ?? 101) - 1)
  sceneMorphLayers.set(rootId, { layer, camera, root })
  return root
}

const sceneMorphTextCameraFor = (rootId: string) => {
  const existing = sceneMorphTextCameras.get(rootId)
  if (existing) return existing
  if (!sceneMorphContainerRef.value) return null
  const camera = document.createElement('div')
  camera.className = 'theater-scene-morph-text-camera'
  camera.style.zIndex = String(rootStackingOrder.value[rootId] ?? 101)
  const position = worldCameraGroup?.position() || { x: 0, y: 0 }
  const scale = worldCameraGroup?.scale() || { x: 1, y: 1 }
  camera.style.transform = `translate(${position.x}px, ${position.y}px) scale(${scale.x})`
  sceneMorphContainerRef.value.append(camera)
  sceneMorphTextCameras.set(rootId, camera)
  return camera
}

const stageObjectTransitionKey = (object: StageObject) => {
  const value = object.metadata?.transitionKey
  return typeof value === 'string' && value.trim() ? value.trim() : ''
}

const stageObjectVisualFallbackKey = (object: StageObject) => {
  if (object.image) {
    const resourceId = object.image.resourceId?.trim()
    const url = object.image.url?.trim()
    const identity = resourceId || url
    return identity ? `${object.type}\u0000media\u0000${identity}` : ''
  }
  if (object.type === 'text' && typeof object.text === 'string') {
    return `${object.type}\u0000text\u0000${object.text}`
  }
  if (object.type === 'drawing' && object.drawing) {
    return `${object.type}\u0000drawing\u0000${JSON.stringify(object.drawing)}`
  }
  if (object.type === 'button' && typeof object.text === 'string') {
    return `${object.type}\u0000button\u0000${object.text}\u0000${object.fill}`
  }
  return ''
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

  const matchUniqueFallback = (keyFor: (object: StageObject) => string) => {
    const unmatchedPrevious = previousObjects.filter((object) => !used.has(object.id))
    const unmatchedNext = next.filter((object) => !matches.has(object.id))
    const previousFallback = uniqueObjectIndex(unmatchedPrevious, keyFor)
    const nextFallback = uniqueObjectIndex(unmatchedNext, keyFor)
    unmatchedNext.forEach((object) => {
      const key = keyFor(object)
      if (!key || nextFallback.get(key)?.id !== object.id) return
      const candidate = previousFallback.get(key)
      if (!candidate || used.has(candidate.id)) return
      matches.set(object.id, previous.get(candidate.id)!)
      used.add(candidate.id)
    })
  }

  // Legacy and imported scenes may predate transitionKey. Match only visual
  // identities that are unique in both scenes, avoiding arbitrary pairings when
  // the same resource or content is used by multiple components.
  matchUniqueFallback(stageObjectVisualFallbackKey)
  matchUniqueFallback(stageObjectFallbackKey)
  return matches
}

const queueSceneEntrances = (sceneId: string, includePersistent = false) => {
  const scene = props.store.state.scenes[sceneId]
  if (!scene) {
    pendingSceneEntrances = null
    pendingTextEntranceIds.value = []
    return
  }
  const objects = Object.values({
    ...scene.state.sceneObjects,
    ...(includePersistent ? props.store.state.persistentObjects : {}),
  }).filter((object) => (
    supportsStageEntrance(object)
    && object.visible
    && entranceConfigFor(object).preset !== 'none'
  ))
  pendingSceneEntrances = {
    sceneId,
    objectIds: new Set(objects.map((object) => object.id)),
  }
  pendingTextEntranceIds.value = objects.filter((object) => object.type === 'text').map((object) => object.id)
}

const hasPendingSceneEntrance = (objectId: string) => pendingSceneEntrances?.objectIds.has(objectId) === true

const playPendingSceneEntrances = (sceneId: string) => {
  if (pendingSceneEntrances?.sceneId !== sceneId) return
  if (!props.syncReady) return
  if (sceneMorphSnapshot?.sceneId === sceneId) return
  if (sceneMediaBatch?.sceneId === sceneId && !sceneMediaBatch.ready) return
  const objectIds = pendingSceneEntrances.objectIds
  pendingSceneEntrances = null
  pendingTextEntranceIds.value = []
  objectIds.forEach((objectId) => {
    const object = props.store.activeObjects.value[objectId]
    const node = objectNodes.get(objectId)
    if (!object || !node) return
    if (!supportsStageEntrance(object) || entranceConfigFor(object).preset === 'none') {
      finishObjectTransition(object, node)
      return
    }
    if (object.type === 'image' && object.image) {
      const source = node.findOne<Konva.Image>('.theater-object-image')?.image() as StageMediaSource | undefined
      if (source) activateStageMedia(source, object.image)
    }
    playStageObjectEntrance(object, node)
  })
}

const finishSceneMorph = () => {
  if (sceneTransitionTimer !== null) window.clearTimeout(sceneTransitionTimer)
  sceneTransitionTimer = null
  sceneMorphDelayTimers.forEach((timer) => window.clearTimeout(timer))
  sceneMorphDelayTimers = []
  sceneMorphTweens.forEach((tween) => tween.destroy())
  sceneMorphTweens = []
  const snapshot = sceneMorphSnapshot
  const composite = sceneCompositeTransition
  if (snapshot) {
    snapshot.animations.forEach((animation) => animation.cancel())
    snapshot.targetAttrs.forEach((attrs, objectId) => objectNodes.get(objectId)?.setAttrs(attrs))
    snapshot.targetTextElements.forEach((element) => element.style.removeProperty('visibility'))
    snapshot.previous.forEach((item) => item.ghost?.destroy())
    snapshot.targetGhosts.forEach((ghost) => ghost.destroy())
    snapshot.backgroundGhost?.destroy()
    snapshot.textGhosts.forEach((ghost) => ghost.remove())
  }
  sceneMorphLayers.forEach(({ layer }) => layer.destroy())
  sceneMorphLayers.clear()
  sceneMorphTextCameras.forEach((camera) => camera.remove())
  sceneMorphTextCameras.clear()
  sceneMorphSnapshot = null
  sceneMorphTextHidden.value = false
  sceneMorphTextAnimating.value = false
  clearSceneCompositeTransition()
  backgroundLayer?.batchDraw()
  drawWorldLayers()
  foregroundLayer?.batchDraw()
  sceneMorphLayers.forEach(({ layer }) => layer.batchDraw())
  if (snapshot) playPendingSceneEntrances(snapshot.sceneId)
  else if (composite?.started) playPendingSceneEntrances(composite.sceneId)
}

const finishSceneTransitionWhenReady = () => {
  const transition = sceneCompositeTransition
  if (!transition?.completed) return
  const snapshot = sceneMorphSnapshot
  if (snapshot?.generation === transition.generation && !snapshot.completed) return
  finishSceneMorph()
}

const prepareSceneMorph = (captureCurrent: boolean, sceneId: string, previousSceneId = '') => {
  finishSceneMorph()
  const nextTransition = normalizeStageSceneTransition(props.store.state.scenes[sceneId]?.state.transition)
  const previousTransition = normalizeStageSceneTransition(props.store.state.scenes[previousSceneId]?.state.transition)
  const generation = ++sceneTransitionGeneration
  const usesCurtain = captureCurrent && (nextTransition.enter.type === 'curtain' || previousTransition.exit.type === 'curtain')
  sceneCompositeTransition = {
    sceneId,
    overlay: captureCurrent ? captureSceneTransitionOverlay() : null,
    curtainOverlay: usesCurtain ? createSceneCurtainOverlay() : null,
    enter: nextTransition.enter,
    exit: previousTransition.exit,
    animations: [],
    generation,
    started: false,
    completed: false,
  }
  if (captureCurrent && sceneMorphStage) {
    const previousSceneObjects = props.store.state.scenes[previousSceneId]?.state.sceneObjects
      || props.store.state.liveState.sceneObjects
    const previous = new Map<string, SceneMorphItem>()
    Object.values(previousSceneObjects).forEach((object) => {
      if (isTheaterEffectObject(object)) return
      const node = objectNodes.get(object.id)
      if (!node) return
      const root = !object.parentId || !previousSceneObjects[object.parentId]
      if (!root) return
      const ghost = root ? node.clone({ listening: false }) as Konva.Group : null
      const textElement = root ? stageTextVisualElement(object.id) : null
      const textGhost = textElement ? textElement.cloneNode(true) as HTMLElement : null
      previous.set(object.id, {
        object: { ...object, transform: { ...object.transform }, metadata: { ...object.metadata } },
        visual: {
          x: node.x(),
          y: node.y(),
          rotation: node.rotation(),
          scaleX: node.scaleX(),
          scaleY: node.scaleY(),
          opacity: node.opacity(),
          visible: node.visible(),
          width: Math.max(0.5, object.transform.width),
          height: Math.max(0.5, object.transform.height),
        },
        ghost,
        textGhost,
      })
    })
    sceneMorphSnapshot = {
      sceneId,
      previous,
      matches: new Map(),
      targetAttrs: new Map(),
      targetGhosts: new Map(),
      targetTextGhosts: new Map(),
      targetTextElements: new Map(),
      animations: [],
      backgroundGhost: null,
      textGhosts: [],
      generation,
      started: false,
      completed: false,
    }
    sceneMorphLayers.forEach(({ layer }) => layer.draw())
  }
  if (captureCurrent && sceneVisualRef.value) sceneVisualRef.value.style.visibility = 'hidden'
}

const primeSceneMorphTargets = () => {
  const snapshot = sceneMorphSnapshot
  if (!snapshot || snapshot.started || snapshot.sceneId !== props.store.state.activeSceneId) return
  const sceneObjects = Object.values(props.store.state.liveState.sceneObjects)
    .filter((object) => !isTheaterEffectObject(object))
    .filter((object) => !object.parentId || !props.store.state.liveState.sceneObjects[object.parentId])
  snapshot.matches = matchSceneMorphObjects(snapshot.previous, sceneObjects)
  snapshot.targetAttrs.clear()
  snapshot.targetGhosts.forEach((ghost) => ghost.destroy())
  snapshot.targetGhosts.clear()
  const previousTargetTextGhosts = new Set(snapshot.targetTextGhosts.values())
  snapshot.targetTextGhosts.forEach((ghost) => ghost.remove())
  snapshot.textGhosts = snapshot.textGhosts.filter((ghost) => !previousTargetTextGhosts.has(ghost))
  snapshot.targetTextGhosts.clear()
  snapshot.targetTextElements.forEach((element) => element.style.removeProperty('visibility'))
  snapshot.targetTextElements.clear()
  sceneObjects.forEach((object) => {
    const node = objectNodes.get(object.id)
    if (!node) return
    if (hasPendingSceneEntrance(object.id)) {
      snapshot.matches.delete(object.id)
      node.setAttrs({ visible: false, opacity: 1 })
      return
    }
    if (!object.visible) {
      snapshot.matches.delete(object.id)
      return
    }
    const target: SceneMorphVisual = {
      x: object.transform.x * WORLD_UNIT_PX,
      y: object.transform.y * WORLD_UNIT_PX,
      rotation: object.transform.rotation,
      scaleX: object.transform.scaleX,
      scaleY: object.transform.scaleY,
      opacity: 1,
      visible: object.visible,
      width: Math.max(0.5, object.transform.width),
      height: Math.max(0.5, object.transform.height),
    }
    const previous = snapshot.matches.get(object.id)
    if (!previous?.ghost) {
      snapshot.matches.delete(object.id)
      return
    }
    const morphRoot = sceneMorphRootFor(object.id)
    if (!morphRoot) {
      snapshot.matches.delete(object.id)
      return
    }
    morphRoot.add(previous.ghost)
    if (previous.textGhost) {
      const previousTextCamera = sceneMorphTextCameraFor(object.id)
      if (previousTextCamera) {
        previousTextCamera.append(previous.textGhost)
        snapshot.textGhosts.push(previous.textGhost)
      }
    }
    const targetGhost = node.clone({ listening: false }) as Konva.Group
    targetGhost.setAttrs({
      x: previous.visual.x,
      y: previous.visual.y,
      rotation: previous.visual.rotation,
      scaleX: previous.visual.scaleX * previous.visual.width / target.width,
      scaleY: previous.visual.scaleY * previous.visual.height / target.height,
      opacity: 0,
      visible: true,
    })
    morphRoot.add(targetGhost)
    snapshot.targetGhosts.set(object.id, targetGhost)
    const targetTextElement = previous.textGhost ? stageTextVisualElement(object.id) : null
    const textCamera = targetTextElement ? sceneMorphTextCameraFor(object.id) : null
    if (targetTextElement && textCamera) {
      const targetTextGhost = targetTextElement.cloneNode(true) as HTMLElement
      textCamera.append(targetTextGhost)
      targetTextElement.style.visibility = 'hidden'
      snapshot.targetTextGhosts.set(object.id, targetTextGhost)
      snapshot.targetTextElements.set(object.id, targetTextElement)
      snapshot.textGhosts.push(targetTextGhost)
    }
    snapshot.targetAttrs.set(object.id, target)
    node.visible(false)
  })
  backgroundLayer?.batchDraw()
  drawWorldLayers()
  foregroundLayer?.batchDraw()
  sceneMorphLayers.forEach(({ layer }) => layer.batchDraw())
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

const sceneMorphTextFrame = (object: StageObject, opacity: number): Keyframe => ({
  left: `${object.transform.x * WORLD_UNIT_PX}px`,
  top: `${object.transform.y * WORLD_UNIT_PX}px`,
  width: `${Math.max(0.5, object.transform.width) * WORLD_UNIT_PX}px`,
  height: `${Math.max(0.5, object.transform.height) * WORLD_UNIT_PX}px`,
  transform: `translate(-50%, -50%) rotate(${object.transform.rotation}deg) scale(${object.transform.scaleX}, ${object.transform.scaleY})`,
  opacity,
})

const startSharedElementMorph = (sceneId: string, durationMs: number, reducedMotion: boolean) => {
  const snapshot = sceneMorphSnapshot
  if (!snapshot || snapshot.sceneId !== sceneId || snapshot.started) return
  snapshot.started = true
  if (reducedMotion) {
    snapshot.previous.forEach((item) => {
      item.ghost?.hide()
      if (item.textGhost) item.textGhost.style.visibility = 'hidden'
    })
    snapshot.completed = true
    sceneMorphLayers.forEach(({ layer }) => layer.draw())
    finishSceneTransitionWhenReady()
    return
  }
  snapshot.started = false
  primeSceneMorphTargets()
  snapshot.started = true
  const duration = Math.max(20, durationMs) / 1_000
  const matchedPrevious = new Set(snapshot.matches.values())
  snapshot.previous.forEach((item) => {
    if (matchedPrevious.has(item)) return
    item.ghost?.destroy()
    item.ghost = null
    item.textGhost?.remove()
    item.textGhost = null
  })
  snapshot.targetAttrs.forEach((target, objectId) => {
    const previous = snapshot.matches.get(objectId)
    const targetGhost = snapshot.targetGhosts.get(objectId)
    if (!previous?.ghost || !targetGhost) return
    tweenSceneMorphNode(previous.ghost, {
      x: target.x,
      y: target.y,
      rotation: target.rotation,
      scaleX: target.scaleX * target.width / previous.visual.width,
      scaleY: target.scaleY * target.height / previous.visual.height,
      opacity: 0,
    }, duration)
    tweenSceneMorphNode(targetGhost, {
      x: target.x,
      y: target.y,
      rotation: target.rotation,
      scaleX: target.scaleX,
      scaleY: target.scaleY,
      opacity: target.opacity,
    }, duration)
    const targetObject = props.store.state.liveState.sceneObjects[objectId]
    const targetTextGhost = snapshot.targetTextGhosts.get(objectId)
    if (previous.textGhost && targetTextGhost && targetObject) {
      const options: KeyframeAnimationOptions = {
        duration: Math.max(20, durationMs),
        easing: 'ease-in-out',
        fill: 'both',
      }
      snapshot.animations.push(
        previous.textGhost.animate([
          sceneMorphTextFrame(previous.object, previous.visual.opacity),
          sceneMorphTextFrame(targetObject, 0),
        ], options),
        targetTextGhost.animate([
          sceneMorphTextFrame(previous.object, 0),
          sceneMorphTextFrame(targetObject, 1),
        ], options),
      )
    }
  })
  if (!snapshot.targetAttrs.size) {
    snapshot.completed = true
    sceneMorphLayers.forEach(({ layer }) => layer.draw())
    finishSceneTransitionWhenReady()
    return
  }
  const timer = window.setTimeout(() => {
    sceneMorphDelayTimers = sceneMorphDelayTimers.filter((value) => value !== timer)
    if (sceneMorphSnapshot !== snapshot) return
    snapshot.completed = true
    finishSceneTransitionWhenReady()
  }, Math.max(20, durationMs) + 20)
  sceneMorphDelayTimers.push(timer)
}

const startSceneMorph = (sceneId: string) => {
  const transition = sceneCompositeTransition
  const visual = sceneVisualRef.value
  if (!transition || transition.sceneId !== sceneId || transition.started || !visual) return
  transition.started = true
  const batch = sceneMediaBatch
  if (batch?.sceneId === sceneId) batch.activations.splice(0).forEach((activate) => activate())
  const reducedMotion = resolveTheaterReducedMotion()
  const curtainOverlay = transition.curtainOverlay
  const curtainPanels = curtainOverlay
    ? Array.from(curtainOverlay.querySelectorAll<HTMLElement>('.theater-scene-curtain-panel'))
    : []
  const usesCurtain = curtainPanels.length === 2
  const morphDurationMs = Math.max(transition.enter.durationMs, transition.exit.durationMs)
  const enterFrames = reducedMotion.effectiveReducedMotion ? [] : stageSceneTransitionKeyframes(transition.enter, 'enter')
  const exitFrames = reducedMotion.effectiveReducedMotion ? [] : stageSceneTransitionKeyframes(transition.exit, 'exit')
  if (usesCurtain && !reducedMotion.effectiveReducedMotion) {
    const closeDurationMs = transition.exit.durationMs
    const openDurationMs = transition.enter.durationMs
    const closeAnimations = curtainPanels.map((panel) => panel.animate([
      { transform: panel.classList.contains('is-left') ? 'translateX(-100%)' : 'translateX(100%)' },
      { transform: 'translateX(0)' },
    ], {
      duration: closeDurationMs,
      easing: 'cubic-bezier(.22, 1, .36, 1)',
      fill: 'forwards',
    }))
    transition.animations.push(...closeAnimations)
    const generation = transition.generation
    void Promise.all(closeAnimations.map((animation) => animation.finished.catch(() => undefined))).then(() => {
      if (sceneCompositeTransition?.generation !== generation || sceneCompositeTransition !== transition) return
      transition.overlay?.remove()
      transition.overlay = null
      visual.style.visibility = 'visible'
      backgroundLayer?.draw()
      foregroundLayer?.draw()
      drawWorldLayers(true)
      startSharedElementMorph(sceneId, openDurationMs, false)
      const openAnimations = curtainPanels.map((panel) => panel.animate([
        { transform: 'translateX(0)' },
        { transform: panel.classList.contains('is-left') ? 'translateX(-100%)' : 'translateX(100%)' },
      ], {
        duration: openDurationMs,
        easing: 'cubic-bezier(.65, 0, .35, 1)',
        fill: 'forwards',
      }))
      transition.animations.push(...openAnimations)
      void Promise.all(openAnimations.map((animation) => animation.finished.catch(() => undefined))).then(() => {
        if (sceneCompositeTransition?.generation !== generation || sceneCompositeTransition !== transition) return
        transition.completed = true
        finishSceneTransitionWhenReady()
      })
    })
    sceneTransitionTimer = window.setTimeout(() => {
      if (sceneCompositeTransition?.generation === generation) {
        finishSceneMorph()
      }
    }, closeDurationMs + openDurationMs + 200)
    return
  }
  startSharedElementMorph(sceneId, morphDurationMs, reducedMotion.effectiveReducedMotion)
  if (enterFrames.length) transition.animations.push(visual.animate(enterFrames, stageSceneTransitionOptions(transition.enter)))
  if (transition.overlay && exitFrames.length) {
    transition.animations.push(transition.overlay.animate(exitFrames, stageSceneTransitionOptions(transition.exit)))
  } else {
    transition.overlay?.remove()
    transition.overlay = null
  }
  visual.style.visibility = 'visible'
  backgroundLayer?.draw()
  foregroundLayer?.draw()
  drawWorldLayers(true)
  if (!transition.animations.length) {
    const generation = transition.generation
    requestAnimationFrame(() => {
      if (sceneCompositeTransition !== transition) return
      transition.completed = true
      finishSceneTransitionWhenReady()
    })
    sceneTransitionTimer = window.setTimeout(() => {
      if (sceneCompositeTransition?.generation === generation) finishSceneMorph()
    }, morphDurationMs + 150)
    return
  }
  const generation = transition.generation
  void Promise.all(transition.animations.map((animation) => animation.finished.catch(() => undefined))).then(() => {
    if (sceneCompositeTransition?.generation === generation) {
      transition.completed = true
      finishSceneTransitionWhenReady()
    }
  })
  sceneTransitionTimer = window.setTimeout(() => {
    if (sceneCompositeTransition?.generation === generation) {
      finishSceneMorph()
    }
  }, Math.max(transition.enter.durationMs, transition.exit.durationMs) + 150)
}

let preparedSceneId = ''

const beginSceneMediaBatch = (sceneId: string, captureCurrent = true, previousSceneId = '') => {
  if (sceneId === preparedSceneId) {
    playPendingSceneEntrances(sceneId)
    return
  }
  preparedSceneId = sceneId
  if (sceneMediaBatch && !sceneMediaBatch.released) releaseSceneMediaBatch(sceneMediaBatch)
  pendingSceneEntrances = null
  pendingTextEntranceIds.value = []
  prepareSceneMorph(captureCurrent, sceneId, previousSceneId)
  queueSceneEntrances(sceneId, !captureCurrent)
  sceneMediaBatch = {
    sceneId,
    expected: new Map(collectSceneMediaItems(sceneId).filter((item) => item.blocksSceneReveal).map((item) => [item.key, item.location.url])),
    settled: new Set(),
    reveals: [],
    activations: [],
    released: false,
    ready: false,
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
    batch.ready = true
    backgroundLayer?.draw()
    foregroundLayer?.draw()
    drawWorldLayers(true)
    requestAnimationFrame(() => {
      if (sceneMediaBatch === batch) startSceneMorph(batch.sceneId)
    })
  })
}

const settleSceneMedia = (key: string, url: string, reveal?: () => void, activate?: () => void) => {
  const batch = sceneMediaBatch
  if (!batch || batch.sceneId !== props.store.state.activeSceneId || batch.expected.get(key) !== url || batch.released) {
    reveal?.()
    if (reveal) activate?.()
    return
  }
  if (reveal) batch.reveals.push(reveal)
  if (activate) batch.activations.push(activate)
  batch.settled.add(key)
  if (batch.settled.size >= batch.expected.size) releaseSceneMediaBatch(batch)
}

const playEffect = (effectId: string, triggerId = '') => effectRuntime.play(effectId, triggerId)

defineExpose({ preloadScenes, appendPointerTrace, playEffect, playSceneAudio, playVisibilityTransitions })

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
  slot = {
    group,
    base,
    media,
    directImage,
    overlay,
    placeholder,
    label,
    style,
    url: '',
    version: 0,
    source: null,
    ready: false,
    directImageSource: null,
    directImageSignature: '',
    debugDrawCount: 0,
  }
  return slot
}

const useDirectSurfaceImage = (style: StageSurfaceStyle) => (
  style.fit !== 'tile'
)

const clearDirectSurfaceImage = (slot: SurfaceSlot) => {
  if (slot.directImageSource || slot.directImage.image()) slot.directImage.clearCache()
  slot.directImage.image(undefined)
  slot.directImage.visible(false)
  slot.directImageSource = null
  slot.directImageSignature = ''
}

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
    clearDirectSurfaceImage(slot)
    return
  }
  const dimensions = stageMediaDimensions(source)
  const rect = surfaceDrawRect(source, box.width, box.height, slot.style.fit as Exclude<StageSurfaceFit, 'tile'>, 0, slot.style.zoom)
  const signature = [
    stageMediaObjectUrls.get(source) || '',
    dimensions.width,
    dimensions.height,
    box.width,
    box.height,
    rect.x,
    rect.y,
    rect.width,
    rect.height,
    slot.style.fit,
    slot.style.zoom,
    slot.style.brightness,
    slot.style.blurPx,
  ].join(':')
  if (
    slot.directImageSource === source
    && slot.directImageSignature === signature
    && slot.directImage.image() === source
  ) {
    slot.directImage.opacity(slot.style.opacity)
    slot.directImage.visible(true)
    return
  }
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
  slot.directImageSource = source
  slot.directImageSignature = signature
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
    clearDirectSurfaceImage(slot)
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
    clearDirectSurfaceImage(slot)
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
    settleSceneMedia(mediaKey, resolved, undefined, () => activateStageMedia(slot.source!, imageRef))
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
    }, () => activateStageMedia(loadedSource, imageRef))
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

const syncGridLayer = () => {
  if (!gridGroup) return false
  const target = props.store.state.liveState.gridOnTop ? gridTopCameraGroup : worldCameraGroup
  if (!target || gridGroup.getParent() === target) return false
  const previousLayer = gridGroup.getLayer()
  gridGroup.moveTo(target)
  previousLayer?.batchDraw()
  target.getLayer()?.batchDraw()
  return true
}

const rebuildGrid = (fieldX: number, fieldY: number, fieldWidth: number, fieldHeight: number) => {
  if (!gridGroup) return false
  const liveState = props.store.state.liveState
  const camera = props.store.state.camera
  const gridVisible = liveState.displayGrid || gridSnapPreviewActive.value
  const baseSignature = [
    fieldX,
    fieldY,
    fieldWidth,
    fieldHeight,
    gridVisible,
    liveState.gridSize,
  ].join(':')
  if (!gridVisible) {
    const changed = gridSignature !== baseSignature || gridGroup.children.length > 0
    if (changed) gridGroup.destroyChildren()
    gridSignature = baseSignature
    gridCoverage = null
    return changed
  }

  const zoom = Math.max(0.01, camera.zoom)
  const snapStep = Math.max(0.25, liveState.gridSize) * WORLD_UNIT_PX
  // Keep rendered lines legible and bounded while preserving the finer snap interval.
  const gridLineStepMultiplier = Math.max(1, Math.ceil(8 / (snapStep * zoom)))
  const step = snapStep * gridLineStepMultiplier
  const signature = `${baseSignature}:${gridLineStepMultiplier}`
  const visibleLeft = (-viewportSize.value.width / 2 - camera.x) / zoom
  const visibleRight = (viewportSize.value.width / 2 - camera.x) / zoom
  const visibleTop = (-viewportSize.value.height / 2 - camera.y) / zoom
  const visibleBottom = (viewportSize.value.height / 2 - camera.y) / zoom
  const visibleWidth = Math.max(step, visibleRight - visibleLeft)
  const visibleHeight = Math.max(step, visibleBottom - visibleTop)
  // Keep overscan so small pans do not allocate and destroy grid nodes every frame.
  const paddingX = Math.max(step * 2, visibleWidth * 0.5)
  const paddingY = Math.max(step * 2, visibleHeight * 0.5)
  const minX = visibleLeft - paddingX
  const maxX = visibleRight + paddingX
  const minY = visibleTop - paddingY
  const maxY = visibleBottom + paddingY
  const covered = Boolean(
    gridCoverage
    && visibleLeft >= gridCoverage.minX
    && visibleRight <= gridCoverage.maxX
    && visibleTop >= gridCoverage.minY
    && visibleBottom <= gridCoverage.maxY,
  )
  if (gridSignature === signature && covered) return false

  gridSignature = signature
  gridGroup.destroyChildren()
  // Draw across current viewport plus overscan, not only inside finite field rectangle.
  // Grid phase still follows field origin, so snapping and lines stay aligned.
  const firstX = fieldX + Math.floor((minX - fieldX) / step) * step
  const firstY = fieldY + Math.floor((minY - fieldY) / step) * step
  for (let x = firstX; x <= maxX; x += step) {
    gridGroup.add(new Konva.Line({
      points: [x, minY, x, maxY],
      stroke: 'rgba(148, 163, 184, 0.12)',
      strokeWidth: 1,
      listening: false,
    }))
  }
  for (let y = firstY; y <= maxY; y += step) {
    gridGroup.add(new Konva.Line({
      points: [minX, y, maxX, y],
      stroke: 'rgba(148, 163, 184, 0.12)',
      strokeWidth: 1,
      listening: false,
    }))
  }
  gridCoverage = { minX, maxX, minY, maxY }
  return true
}

const syncGrid = () => {
  if (!gridGroup) return
  const moved = syncGridLayer()
  const liveState = props.store.state.liveState
  const width = liveState.fieldWidth * WORLD_UNIT_PX
  const height = liveState.fieldHeight * WORLD_UNIT_PX
  const box = { x: -width / 2, y: -height / 2, width, height }
  if (rebuildGrid(box.x, box.y, width, height) || moved) {
    worldLayer?.batchDraw()
    gridTopLayer?.batchDraw()
  }
}

const scheduleGridSync = () => {
  if (!gridGroup) return
  const liveState = props.store.state.liveState
  const hasGridNodes = Boolean(gridGroup?.children.length)
  if (!liveState.displayGrid && !gridSnapPreviewActive.value && !hasGridNodes) return
  if (gridSyncFrame !== null) return
  gridSyncFrame = window.requestAnimationFrame(() => {
    gridSyncFrame = null
    syncGrid()
  })
}

const syncSurfaceSlots = () => {
  if (!backgroundSlot || !foregroundSlot) return
  const liveState = props.store.state.liveState
  const width = liveState.fieldWidth * WORLD_UNIT_PX
  const height = liveState.fieldHeight * WORLD_UNIT_PX
  const box = { x: -width / 2, y: -height / 2, width, height }
  const viewportBox = { x: 0, y: 0, width: viewportSize.value.width, height: viewportSize.value.height }
  updateSurfaceSlot(backgroundSlot, liveState.background, viewportBox, liveState.surfaceStyles.background, '背景', 'surface:background')
  updateSurfaceSlot(foregroundSlot, liveState.foreground, box, liveState.surfaceStyles.foreground, '前景', 'surface:foreground')
  backgroundLayer?.batchDraw()
  foregroundLayer?.batchDraw()
}

const syncField = () => {
  syncSurfaceSlots()
  syncGrid()
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
    // Keep line width constant while parent node is resized.
    strokeScaleEnabled: false,
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
    // RegularPolygon is circularly inscribed; non-uniform stretch is applied via scale.
    const size = Math.max(1, Math.min(width, height))
    return new Konva.RegularPolygon({
      ...common,
      x: width / 2,
      y: height / 2,
      sides: drawing.tool === 'triangle' ? 3 : drawing.sides || 6,
      radius: size / 2,
      scaleX: width / size,
      scaleY: height / size,
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
  const lineLike = session.tool === 'line' || session.tool === 'arrow'
  if (session.shiftKey) {
    if (lineLike) {
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
  if (lineLike && session.shiftKey) end = snapStagePosition(end)
  const start = session.altKey
    ? { x: session.start.x - (end.x - session.start.x), y: session.start.y - (end.y - session.start.y) }
    : session.start
  const minimum = gridSnapEnabled.value ? Math.max(12, stageGridStep()) : 12
  if (!lineLike) {
    if (Math.abs(end.x - start.x) < minimum) end.x = start.x + Math.sign(end.x - start.x || 1) * minimum
    if (Math.abs(end.y - start.y) < minimum) end.y = start.y + Math.sign(end.y - start.y || 1) * minimum
  }
  const deltaX = Math.abs(end.x - start.x)
  const deltaY = Math.abs(end.y - start.y)
  const width = Math.max(minimum, deltaX)
  const height = Math.max(minimum, deltaY)
  const x = lineLike && deltaX < minimum
    ? (start.x + end.x - width) / 2
    : Math.min(start.x, end.x)
  const y = lineLike && deltaY < minimum
    ? (start.y + end.y - height) / 2
    : Math.min(start.y, end.y)
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
  drawWorldLayers()
}

const cancelDrawingSession = () => {
  const hadSession = Boolean(drawingSession)
  drawingSession = null
  drawingDraftRoot?.destroyChildren()
  if (hadSession) setGridSnapPreview(false)
  drawWorldLayers()
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
    wrapper.setAttr('stageDrawingWidth', width)
    wrapper.setAttr('stageDrawingHeight', height)
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
      listening: false,
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
    const current = getObject(object.id)
    if (viewToolActive.value) {
      // Editable unlocked objects remain movable in view mode instead of starting a canvas pan.
      if (canDragObject(current)) event.cancelBubble = true
      return
    }
    if (event.evt.button !== 0) return
    if (current?.type === 'group') return
    if (quickDeleteActive.value) {
      if (!canEditAllObjects.value) return
      const targetId = canvasSelectionTarget(object.id)
      if (!targetId || !getObject(targetId)) return
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
    const selectionId = editableCanvasSelectionTarget(object.id)
    if (!selectionId) return
    const lockedPanSurface = Boolean(current?.locked && selectionId === object.id)
    event.cancelBubble = selectionId === object.id && !lockedPanSurface
    if (lockedPanSurface) return
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
    if (getObject(object.id)?.type === 'group') return
    const selectionId = editableCanvasSelectionTarget(object.id)
    if (!selectionId) return
    event.cancelBubble = true
    selectObject(selectionId)
  })
  wrapper.on('click tap', (event) => {
    if ('button' in event.evt && event.evt.button !== 0) return
    if (activeCanvasTool.value || quickDeleteActive.value) return
    if (suppressedObjectClickId === object.id) {
      suppressedObjectClickId = null
      event.cancelBubble = true
      return
    }
    const current = getObject(object.id)
    if (current?.type === 'group') return
    if (!viewToolActive.value && current?.locked) {
      const selectionId = editableCanvasSelectionTarget(object.id)
      if (selectionId) {
        const additive = event.evt.shiftKey || event.evt.ctrlKey || event.evt.metaKey
        selectObject(selectionId, additive)
      }
    }
    const pointer = stage?.getPointerPosition()
    const actionTarget = pointer ? resolveObjectActionTarget(pointer) : null
    if (actionTarget) triggerObjectActions(actionTarget)
  })
  wrapper.on('contextmenu', (event) => {
    if (viewToolActive.value || activeCanvasTool.value || quickDeleteActive.value) return
    if (getObject(object.id)?.type === 'group') return
    const selectionId = editableCanvasSelectionTarget(object.id)
    if (!selectionId) return
    event.evt.preventDefault()
    event.cancelBubble = true
    openObjectInspector(selectionId)
  })
  wrapper.on('pointerenter pointermove', () => {
    if (!quickDeleteActive.value || !stage || !quickDeleteOutline) return
    const targetId = canvasSelectionTarget(object.id)
    if (!targetId) return
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
      beginNodeGridSnap(wrapper)
      setGridSnapPreview(gridSnapEnabled.value)
      props.store.beginObjectEdit('批量移动对象')
      return
    }
    beginNodeGridSnap(wrapper)
    setGridSnapPreview(gridSnapEnabled.value)
    props.store.beginObjectEdit('移动对象')
  })
  wrapper.on('dragmove', () => {
    if (!multiDrag || multiDrag.driverId !== object.id) {
      snapNodeToGrid(wrapper)
      return
    }
    const current = wrapper.absolutePosition()
    const delta = {
      x: current.x - multiDrag.driverStart.x,
      y: current.y - multiDrag.driverStart.y,
    }
    multiDrag.nodes.forEach(({ node, absolute }, id) => {
      if (id === object.id) return
      node.absolutePosition({ x: absolute.x + delta.x, y: absolute.y + delta.y })
    })
    if (gridSnapEnabled.value) {
      const correction = snapNodeToGrid(wrapper)
      multiDrag.nodes.forEach(({ node }) => {
        if (node === wrapper) return
        const position = node.absolutePosition()
        node.absolutePosition({ x: position.x + correction.x, y: position.y + correction.y })
      })
    }
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
      setGridSnapPreview(false)
      updateTransformer()
      return
    }
    const current = getObject(object.id)
    if (!canEditObject(current)) {
      props.store.cancelObjectEdit()
      setGridSnapPreview(false)
      return
    }
    if (gridSnapEnabled.value) {
      snapNodeToGrid(wrapper)
    }
    current.transform.x = Number((wrapper.x() / WORLD_UNIT_PX).toFixed(6))
    current.transform.y = Number((wrapper.y() / WORLD_UNIT_PX).toFixed(6))
    props.store.commitObjectEdit()
    setGridSnapPreview(false)
  })
  wrapper.on('dragcancel', finishGridSnapPreview)
  wrapper.on('transformstart', () => {
    if (!canEditObject(getObject(object.id))) return
    if (isBatchSelection.value && props.store.selectionGroup.value.rootIds.includes(object.id)) return
    props.store.beginObjectEdit('变换对象')
  })
  wrapper.on('transformend', () => {
    if (isBatchSelection.value && props.store.selectionGroup.value.rootIds.includes(object.id)) return
    const current = getObject(object.id)
    if (!canEditObject(current)) {
      props.store.cancelObjectEdit()
      return
    }
    applyObjectNodeTransform(wrapper, current)
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
    settleSceneMedia(`object:${object.id}`, resolved, undefined, () => {
      if (!hasPendingSceneEntrance(object.id)) activateStageMedia(currentSource, object.image!)
    })
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
      if (pendingObjectEntrances.has(object.id)) playObjectEntrance(object, wrapper)
      wrapper.getLayer()?.batchDraw()
    }, () => {
      if (!hasPendingSceneEntrance(object.id)) activateStageMedia(loadedSource, object.image!)
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
  const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
  const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
  // Drawings bake non-group transforms into width/height (scale reset to 1).
  // Content must be rebuilt whenever pixel size changes, not only when style/points change;
  // otherwise free-aspect stretch snaps back to the original geometry after transformend.
  const drawingNeedsRebuild = object.type === 'drawing' && (
    wrapper.getAttr('stageDrawingSignature') !== JSON.stringify(object.drawing)
    || wrapper.getAttr('stageDrawingWidth') !== width
    || wrapper.getAttr('stageDrawingHeight') !== height
  )
  if (
    wrapper.getAttr('stageObjectType') !== object.type
    || drawingNeedsRebuild
  ) rebuildObjectContent(wrapper, object)
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
    visible: props.syncReady
      && !hasPendingSceneEntrance(object.id)
      && (object.visible || objectEntranceTweens.has(object.id)),
    draggable: canDragObject(object)
      && !activeCanvasTool.value
      && !quickDeleteActive.value
      && (viewToolActive.value || groupedObjectDirectlySelected)
      && (!multiSelected || (!batchMoveBlocked.value && !selectedAncestor)),
    listening: object.type === 'group'
      ? true
      : (!viewToolActive.value && Boolean(editableCanvasSelectionTarget(object.id)))
        || (viewToolActive.value && canDragObject(object))
        || canInteractObject(object)
        || canShowImageAnnotation(object),
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

const canvasStageObjects = () => Object.fromEntries(Object.entries(stageObjects.value)
  .filter(([, object]) => !isTheaterEffectObject(object)))

const syncObjectRootLayers = (objects: Record<string, StageObject>) => {
  if (!stage || !worldCameraGroup) return
  const roots = Object.values(objects)
    .filter((object) => !object.parentId || !objects[object.parentId])
    .sort(compareStageLayersBottomToTop)
  const rootIds = new Set(roots.map((object) => object.id))
  let changed = false

  objectRootLayers.forEach((entry, objectId) => {
    if (rootIds.has(objectId)) return
    entry.layer.destroy()
    objectRootLayers.delete(objectId)
    changed = true
  })

  const stackingOrder: Record<string, number> = {}
  roots.forEach((object, index) => {
    const canvasZIndex = OBJECT_ROOT_LAYER_Z_BASE + index * 2
    stackingOrder[object.id] = canvasZIndex + 1
    let entry = objectRootLayers.get(object.id)
    if (!entry) {
      const layer = new Konva.Layer()
      const camera = new Konva.Group()
      layer.add(camera)
      stage!.add(layer)
      entry = { layer, camera }
      objectRootLayers.set(object.id, entry)
      changed = true
    }
    entry.camera.position(worldCameraGroup.position())
    entry.camera.scale(worldCameraGroup.scale())
    entry.layer.getCanvas()._canvas.style.zIndex = String(canvasZIndex)
    const node = objectNodes.get(object.id)
    if (node && node.getParent() !== entry.camera) node.moveTo(entry.camera)
  })
  backgroundLayer?.moveToTop()
  worldLayer?.moveToTop()
  roots.forEach((object) => objectRootLayers.get(object.id)?.layer.moveToTop())
  worldOverlayLayer?.moveToTop()
  foregroundLayer?.moveToTop()
  gridTopLayer?.moveToTop()
  interactionLayer?.moveToTop()
  rootStackingOrder.value = stackingOrder
  if (changed) {
    mediaAnimation?.stop()
    mediaAnimation = null
    syncMediaAnimation()
  }
}

const syncGroupControls = (objects: Record<string, StageObject>) => {
  const selectedId = props.store.state.selectedObjectId
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
    controlBounds.listening(false)
    outline.visible(false)
    groupControls.push({ object, wrapper, controlBounds, outline })
  }
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
      listening: selectedId === object.id
        && !isBatchSelection.value
        && canEditObject(object)
        && !object.locked,
    })
    if (selectedId === object.id && !isBatchSelection.value) controlBounds.moveToTop()
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
}

const syncLayerHierarchy = () => {
  if (!objectRoot) return
  const objects = canvasStageObjects()
  layerHierarchyUpdatedObjectIds.forEach((objectId) => {
    const object = objects[objectId]
    const node = objectNodes.get(objectId)
    if (object && node) updateObjectNode(node, object)
  })
  syncStageObjectHierarchy(objects, objectNodes, objectRoot)
  syncObjectRootLayers(objects)
  syncGroupControls(objects)
  drawWorldLayers()
  nextTick(updateTransformer)
}

const syncObjects = () => {
  if (!objectRoot) return
  const objects = canvasStageObjects()
  for (const [objectId, node] of objectNodes) {
    if (objects[objectId]) continue
    objectEntranceTweens.get(objectId)?.destroy()
    objectEntranceTweens.delete(objectId)
    pendingObjectEntrances.delete(objectId)
    const textEntranceTimer = textEntranceTimers.get(objectId)
    if (textEntranceTimer !== undefined) window.clearTimeout(textEntranceTimer)
    textEntranceTimers.delete(objectId)
    delete textEntrancePlaybacks[objectId]
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
  syncObjectRootLayers(objects)
  syncGroupControls(objects)
  drawWorldLayers()
  primeSceneMorphTargets()
  nextTick(updateTransformer)
}

const resizeStage = () => {
  const element = viewportRef.value
  if (!stage || !element) return
  const rect = element.getBoundingClientRect()
  const nextViewportSize = { width: Math.max(1, rect.width), height: Math.max(1, rect.height) }
  if (
    viewportSize.value.width === nextViewportSize.width
    && viewportSize.value.height === nextViewportSize.height
  ) return
  viewportSize.value = nextViewportSize
  stage.size(viewportSize.value)
  sceneMorphStage?.size(viewportSize.value)
  syncField()
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

const PAN_DRAG_THRESHOLD = 5

const stageObjectIdFromTarget = (target: Konva.Node) => {
  let node: Konva.Node | null = target
  while (node && node !== stage) {
    const objectId = node.getAttr('stageObjectId')
    if (typeof objectId === 'string' && objectId) return objectId
    node = node.getParent()
  }
  return null
}

const handleStageImageAnnotationHover = (event: Konva.KonvaEventObject<PointerEvent>) => {
  const objectId = stageObjectIdFromTarget(event.target)
  if (!objectId || !canShowImageAnnotation(getObject(objectId))) {
    hideImageAnnotation()
    return
  }
  updateImageAnnotationHover(objectId, event)
}

const beginPanCandidate = (event: Konva.KonvaEventObject<PointerEvent>, objectId: string | null) => {
  panning = false
  panCandidate = { pointerId: event.evt.pointerId, objectId }
  panPointer = { x: event.evt.clientX, y: event.evt.clientY }
  panOrigin = { x: props.store.state.camera.x, y: props.store.state.camera.y }
}

const startPan = (event: Konva.KonvaEventObject<PointerEvent>) => {
  if (!stage) return
  const objectId = stageObjectIdFromTarget(event.target)
  if (viewToolActive.value) {
    if (event.evt.button === 2) {
      const pointer = worldCameraGroup?.getRelativePointerPosition()
      if (!pointer) return
      event.evt.preventDefault()
      beginPointerTrace(pointer)
      return
    }
    if (event.evt.button !== 0) return
    beginPanCandidate(event, objectId)
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
    const drawingPointer = snapDrawingPoint(activeCanvasTool.value, pointer)
    event.evt.preventDefault()
    drawingSession = {
      tool: activeCanvasTool.value,
      start: drawingPointer,
      current: drawingPointer,
      points: [drawingPointer.x, drawingPointer.y],
      shiftKey: event.evt.shiftKey,
      altKey: event.evt.altKey,
    }
    setGridSnapPreview(gridSnapEnabled.value)
    renderDrawingDraft()
    return
  }
  const hitObject = objectId ? getObject(objectId) : null
  if (event.evt.button !== 0 || (event.target !== stage && !hitObject?.locked)) return
  if (event.target === stage && props.store.selection.bulkMode && canEditAllObjects.value) {
    event.evt.preventDefault()
    const pointer = stage.getPointerPosition()
    if (!pointer) return
    marqueeStart = pointer
    marqueeAdditive = event.evt.shiftKey || event.evt.ctrlKey || event.evt.metaKey
    selectionRect?.setAttrs({ x: pointer.x, y: pointer.y, width: 0, height: 0, visible: true })
    interactionLayer?.batchDraw()
    return
  }
  if (event.target === stage) selectObject(null)
  beginPanCandidate(event, objectId)
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
    const drawingPointer = snapDrawingPoint(drawingSession.tool, pointer)
    drawingSession.current = drawingPointer
    drawingSession.shiftKey = event.evt.shiftKey
    drawingSession.altKey = event.evt.altKey
    if (drawingSession.tool === 'pen' || drawingSession.tool === 'highlighter') {
      const points = drawingSession.points
      const previous = { x: points[points.length - 2], y: points[points.length - 1] }
      if (Math.hypot(drawingPointer.x - previous.x, drawingPointer.y - previous.y) >= 2) points.push(drawingPointer.x, drawingPointer.y)
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
  if (!panCandidate || event.evt.pointerId !== panCandidate.pointerId) return
  const deltaX = event.evt.clientX - panPointer.x
  const deltaY = event.evt.clientY - panPointer.y
  if (!panning) {
    if (Math.hypot(deltaX, deltaY) < PAN_DRAG_THRESHOLD) return
    panning = true
  }
  event.evt.preventDefault()
  props.store.state.camera.x = panOrigin.x + deltaX
  props.store.state.camera.y = panOrigin.y + deltaY
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
  if (panCandidate) {
    if (event && event.evt.pointerId !== panCandidate.pointerId) return
    const candidate = panCandidate
    const didPan = panning
    panCandidate = null
    panning = false
    if (didPan && candidate.objectId) {
      suppressedObjectClickId = candidate.objectId
      window.setTimeout(() => {
        if (suppressedObjectClickId === candidate.objectId) suppressedObjectClickId = null
      }, 0)
    }
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
    .filter((object) => object.type !== 'group' && object.visible && canEditObject(object))
    .filter((object) => {
      const node = objectNodes.get(object.id)
      if (!node?.isVisible()) return false
      const bounds = marqueeObjectBounds(object, node, stage!)
      return bounds ? marqueeContains(box, bounds) : false
    })
    .map((object) => object.id)
  const rootHits = stageSelectionRootIds(props.store.activeObjects.value, hits)
  const next = stageSelectionRootIds(props.store.activeObjects.value, additive
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

const canUploadImageTarget = (target: ImageTarget) => {
  if (canEditAllObjects.value && canUploadResources.value) return true
  if (target.kind !== 'object') return false
  const object = props.store.activeObjects.value[target.objectId]
  return object?.type === 'image' && canEditObject(object)
}

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

const theaterResourcePath = (resourceId = '', scope = captureTheaterRequestScope()) => {
  return buildTheaterResourcePath(theaterMediaScope(scope), resourceId)
}

const waitForResource = async (resourceId: string, scope = captureTheaterRequestScope()) => {
  for (let attempt = 0; attempt < 240; attempt += 1) {
    const response = await api.get<TheaterResourceResponse>(theaterResourcePath(resourceId, scope))
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

const uploadTheaterImageResource = async (file: File, targetObjectId = '', scope = captureTheaterRequestScope()) => {
  if (!scope.worldId || !scope.channelId) throw new Error('缺少小剧场频道信息')
  const prepared = await prepareTheaterMedia(file)
  const formData = new FormData()
  formData.append('file', prepared)
  formData.append('mediaKind', 'image')
  formData.append('clientResourceId', crypto.randomUUID?.() || `image-${Date.now()}-${Math.random().toString(16).slice(2)}`)
  if (!canUploadResources.value && targetObjectId) formData.append('targetObjectId', targetObjectId)
  const response = await api.post<TheaterResourceResponse>(theaterResourcePath('', scope), formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  let resource = response.data?.resource
  const resourceId = resource?.id
  if (!resourceId) throw new Error('上传响应缺少资源 ID')
  if (resource?.status !== 'ready') resource = await waitForResource(resourceId, scope)
  const variant = resource?.playbackVariant || 'original'
  const mimeType = resource?.playbackMimeType || prepared.type || normalizedFileType(prepared)
  const resourceBase = urlBase.startsWith('//') ? `${window.location.protocol}${urlBase}` : urlBase
  return {
    resource,
    resourceId,
    prepared,
    url: `${resourceBase.replace(/\/$/, '')}${theaterResourceContentPath(theaterMediaScope(scope), resourceId, variant)}`,
    mimeType,
  }
}

const uploadImage = async (file: File, target: ImageTarget) => {
  if (!canUploadImageTarget(target)) throw new Error('缺少小剧场图片编辑权限')
  if (!props.worldId || !props.channelId) throw new Error('缺少小剧场频道信息')
  resourceUploading.value = true
  resourceError.value = ''
  try {
    const targetObject = target.kind === 'object' ? props.store.activeObjects.value[target.objectId] : null
    const targetEffectConfig = isTheaterEffectObject(targetObject) ? theaterEffectConfigFromObject(targetObject) : null
    const uploaded = await uploadTheaterImageResource(file, target.kind === 'object' ? target.objectId : '')
    const dimensions = (
      targetObject?.type === 'image' && !targetObject.image
    ) || (
      targetEffectConfig?.kind === 'media' && !targetObject?.image && !targetEffectConfig.media
    )
      ? await theaterMediaDimensions(uploaded.prepared)
      : undefined
    if (!applyImageUrl(target, uploaded.url, uploaded.resourceId, uploaded.mimeType, uploaded.resource?.animated === true, uploaded.resource?.loopCount || undefined, dimensions)) throw new Error('图片目标已失效')
  } catch (error) {
    resourceError.value = error instanceof Error ? error.message : '图片上传失败'
    throw error
  } finally {
    resourceUploading.value = false
  }
}

const uploadTheaterImageAssets = async (files: File[], folderId: string) => {
  if (!canUploadResources.value || !files.length) return
  const generation = ++theaterImageUploadGeneration
  const scope = captureTheaterRequestScope()
  const finishUpload = () => {
    if (generation === theaterImageUploadGeneration) theaterImageUploading.value = false
  }
  theaterImageUploading.value = true
  theaterImageError.value = ''
  let succeeded = 0
  const errors: string[] = []
  const createdIds: string[] = []
  const imageItems = new Map(theaterPanelOrganizer.value.items
    .filter((item) => item.domain === 'image')
    .map((item) => [item.targetId, item]))
  const existingIds = theaterImageAssets.value
    .filter((asset) => (imageItems.get(asset.id)?.folderId || '') === folderId)
    .sort((left, right) => {
      const leftOrder = imageItems.get(left.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
      const rightOrder = imageItems.get(right.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
      return leftOrder - rightOrder || left.name.localeCompare(right.name)
    })
    .map((asset) => asset.id)
  for (const file of files) {
    if (!isCurrentTheaterRequestScope(scope)) {
      finishUpload()
      return
    }
    try {
      const uploaded = await uploadTheaterImageResource(file, '', scope)
      if (!isCurrentTheaterRequestScope(scope)) {
        finishUpload()
        return
      }
      const name = file.name.replace(/\.[^.]+$/, '').trim() || '未命名图片'
      const response = await api.post<{ item?: { id?: string } }>(theaterImageAssetPath('', scope), { resourceId: uploaded.resourceId, name })
      if (!isCurrentTheaterRequestScope(scope)) {
        finishUpload()
        return
      }
      if (response.data?.item?.id) createdIds.push(response.data.item.id)
      succeeded += 1
    } catch (error) {
      if (!isCurrentTheaterRequestScope(scope)) {
        finishUpload()
        return
      }
      errors.push(error instanceof Error ? error.message : '图片素材导入失败')
    }
  }
  let organizerError = ''
  if (!isCurrentTheaterRequestScope(scope)) {
    finishUpload()
    return
  }
  if (createdIds.length) {
    try {
      await api.put(theaterPanelOrganizerPath('item-order', scope), {
        domain: 'image',
        folderId,
        targetIds: [...existingIds, ...createdIds],
      })
      if (!isCurrentTheaterRequestScope(scope)) {
        finishUpload()
        return
      }
    } catch (error) {
      organizerError = theaterAudioErrorMessage(error, '上传成功，但整理到当前文件夹失败')
    }
  }
  if (!isCurrentTheaterRequestScope(scope)) {
    finishUpload()
    return
  }
  await Promise.all([fetchTheaterImageAssets(), fetchTheaterPanelOrganizer()])
  const messages: string[] = []
  if (errors.length) messages.push(files.length === 1 ? errors[0] : `${succeeded} 个成功，${errors.length} 个失败：${errors[0]}${errors.length > 1 ? ' 等' : ''}`)
  if (organizerError) messages.push(organizerError)
  if (messages.length) theaterImageError.value = messages.join('；')
  finishUpload()
}

const uploadSceneOverlayMedia = async (files: File[]) => {
  if (!canUploadResources.value || !files.length) return
  theaterImageError.value = ''
  let folder = sceneOverlayImageFolder.value
  if (!folder) {
    const scope = captureTheaterRequestScope()
    let createError: unknown
    try {
      const response = await api.post<{ folder?: TheaterPanelFolder }>(
        theaterPanelOrganizerPath('folders', scope),
        { domain: 'image', name: sceneOverlayImageFolderName },
      )
      if (!isCurrentTheaterRequestScope(scope)) return
      folder = response.data?.folder
    } catch (error) {
      createError = error
    }
    if (!folder) {
      await fetchTheaterPanelOrganizer()
      if (!isCurrentTheaterRequestScope(scope)) return
      folder = sceneOverlayImageFolder.value
    }
    if (!folder) {
      theaterImageError.value = theaterAudioErrorMessage(createError, `创建“${sceneOverlayImageFolderName}”文件夹失败`)
      return
    }
  }
  await uploadTheaterImageAssets(files, folder.id)
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
  if (target.kind === 'object' && !canEditObject(props.store.activeObjects.value[target.objectId])) return
  if (target.kind === 'scene' && !canEditAllObjects.value) return
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

const placeCanvasDropObject = (object: StageObject, event: DragEvent, offsetIndex = 0) => {
  const rect = viewportRef.value?.getBoundingClientRect()
  if (!rect) return false
  const { x: cameraX, y: cameraY, zoom } = props.store.state.camera
  const step = 24 / zoom / WORLD_UNIT_PX
  object.transform.x = (event.clientX - rect.left - rect.width / 2 - cameraX) / zoom / WORLD_UNIT_PX + offsetIndex * step
  object.transform.y = (event.clientY - rect.top - rect.height / 2 - cameraY) / zoom / WORLD_UNIT_PX + offsetIndex * step
  if (gridSnapEnabled.value) {
    const width = Math.max(0.5, object.transform.width) * WORLD_UNIT_PX
    const height = Math.max(0.5, object.transform.height) * WORLD_UNIT_PX
    const center = { x: object.transform.x * WORLD_UNIT_PX, y: object.transform.y * WORLD_UNIT_PX }
    const topLeft = { x: center.x - width / 2, y: center.y - height / 2 }
    const snapped = snapStagePosition(topLeft)
    object.transform.x = (center.x + snapped.x - topLeft.x) / WORLD_UNIT_PX
    object.transform.y = (center.y + snapped.y - topLeft.y) / WORLD_UNIT_PX
  }
  return true
}

const handleCanvasDrop = async (event: DragEvent) => {
  const imageAssetId = event.dataTransfer?.getData(THEATER_IMAGE_ASSET_DRAG_TYPE)?.trim() || ''
  if (imageAssetId) {
    if (!canEditAllObjects.value) return
    let asset = theaterImageAssets.value.find((item) => item.id === imageAssetId)
    if (!asset) {
      await fetchTheaterImageAssets()
      asset = theaterImageAssets.value.find((item) => item.id === imageAssetId)
    }
    if (!asset || asset.resource.status !== 'ready') {
      theaterImageError.value = '图片素材不存在或资源不可用'
      return
    }
    const object = props.store.addObject('image')
    object.name = asset.name
    const dimensions = Number.isFinite(asset.resource.width) && Number.isFinite(asset.resource.height)
      && (asset.resource.width || 0) > 0 && (asset.resource.height || 0) > 0
      ? { width: asset.resource.width!, height: asset.resource.height! }
      : undefined
    if (!props.store.setObjectImage(
      object.id,
      asset.url,
      asset.resourceId,
      asset.resource.playbackMimeType || asset.resource.mimeType,
      asset.resource.animated === true,
      asset.resource.loopCount || undefined,
      dimensions,
    )) {
      props.store.removeObjects([object.id], false)
      theaterImageError.value = '图片组件创建失败'
      return
    }
    const organizerItem = theaterPanelOrganizer.value.items.find((item) => item.domain === 'image' && item.targetId === asset.id)
    const folderPreset = organizerItem?.folderId
      ? theaterPanelOrganizer.value.folders.find((folder) => folder.domain === 'image' && folder.id === organizerItem.folderId)?.preset
      : undefined
    const resolvedPreset = resolveImageObjectPreset(folderPreset, asset.preset)
    applyImageObjectPreset(object, resolvedPreset)
    if (!placeCanvasDropObject(object, event)) {
      props.store.removeObjects([object.id], false)
      return
    }
    props.store.setSelectedObjectIds([object.id], object.id)
    theaterImageError.value = ''
    return
  }
  if (!canEditAllObjects.value || !canUploadResources.value) return
  const files = Array.from(event.dataTransfer?.files || [])
    .filter((item) => supportedTheaterMedia.has(normalizedFileType(item)))
  if (!files.length) return

  const createdIds: string[] = []
  const errors: string[] = []

  for (let i = 0; i < files.length; i += 1) {
    const object = props.store.addObject('image')
    if (!placeCanvasDropObject(object, event, i)) {
      props.store.removeObjects([object.id], false)
      continue
    }
    try {
      await uploadImage(files[i], { kind: 'object', objectId: object.id })
      createdIds.push(object.id)
    } catch (error) {
      props.store.removeObjects([object.id], false)
      errors.push(error instanceof Error ? error.message : '图片上传失败')
    }
  }

  if (createdIds.length) {
    props.store.setSelectedObjectIds(createdIds, createdIds[createdIds.length - 1])
  }
  if (errors.length) {
    resourceError.value = files.length === 1
      ? errors[0]
      : `${createdIds.length} 个成功，${errors.length} 个失败：${errors[0]}${errors.length > 1 ? ` 等` : ''}`
  } else {
    resourceError.value = ''
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

const moveLayerObjectPreservingTransform = (
  objectId: string,
  parentId: string | null,
  targetId: string | null = null,
  placement: 'before' | 'after' = 'after',
) => {
  const object = getObject(objectId)
  const node = objectNodes.get(objectId)
  const parentNode = parentId ? objectNodes.get(parentId) : objectRoot
  if (!object || !node || !parentNode || !props.store.canSetParent(objectId, parentId)) return false
  if (object.parentId !== parentId) {
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
  }
  layerHierarchyMovePending = true
  layerHierarchyUpdatedObjectIds = new Set([...props.store.selection.selectedIds, objectId])
  const changed = props.store.moveObject(objectId, parentId, {
    x: Number((node.x() / WORLD_UNIT_PX).toFixed(6)),
    y: Number((node.y() / WORLD_UNIT_PX).toFixed(6)),
    rotation: Number(node.rotation().toFixed(6)),
    scaleX: Number(node.scaleX().toFixed(6)),
    scaleY: Number(node.scaleY().toFixed(6)),
  }, targetId, placement)
  void nextTick(() => {
    layerHierarchyMovePending = false
    layerHierarchyUpdatedObjectIds.clear()
  })
  if (!changed) {
    layerHierarchyMovePending = false
    layerHierarchyUpdatedObjectIds.clear()
    syncObjects()
  }
  return changed
}

const setLayerDropTarget = (target: typeof layerDropTarget.value) => {
  if (layerDropTarget.value?.id === target?.id && layerDropTarget.value?.placement === target?.placement) return
  layerDropTarget.value = target
}

const clearLayerExpandTimer = () => {
  if (layerExpandTimer !== null) window.clearTimeout(layerExpandTimer)
  layerExpandTimer = null
  layerExpandTargetId = null
}

const scheduleLayerGroupExpand = (target: StageObject | undefined, placement: LayerDropPlacement) => {
  const targetId = placement === 'inside' && target?.type === 'group' && isLayerGroupCollapsed(target) ? target.id : null
  if (targetId === layerExpandTargetId) return
  clearLayerExpandTimer()
  if (!targetId) return
  layerExpandTargetId = targetId
  layerExpandTimer = window.setTimeout(() => {
    temporaryExpandedGroupIds.value = new Set([...temporaryExpandedGroupIds.value, targetId])
    clearLayerExpandTimer()
  }, 350)
}

const updateLayerDropTarget = (clientX: number, clientY: number) => {
  const session = layerDragSession
  const list = layerListRef.value
  if (!session || !list) return
  const element = document.elementFromPoint(clientX, clientY) as HTMLElement | null
  if (!element || !list.contains(element)) {
    setLayerDropTarget(null)
    scheduleLayerGroupExpand(undefined, 'after')
    return
  }
  const rootDrop = element.closest<HTMLElement>('.theater-layer-root-drop')
  if (rootDrop) {
    setLayerDropTarget(props.store.canSetParent(session.objectId, null) ? { id: null, placement: 'inside' } : null)
    scheduleLayerGroupExpand(undefined, 'after')
    return
  }
  const row = element.closest<HTMLElement>('.theater-layer-row')
  const targetId = row?.dataset.objectId
  const target = targetId ? getObject(targetId) : undefined
  if (!row || !target || target.id === session.objectId) {
    setLayerDropTarget(null)
    scheduleLayerGroupExpand(undefined, 'after')
    return
  }
  const rect = row.getBoundingClientRect()
  const ratio = (clientY - rect.top) / Math.max(1, rect.height)
  const placement: LayerDropPlacement = target.type === 'group' && ratio >= 0.25 && ratio <= 0.75
    ? 'inside'
    : ratio < 0.5 ? 'before' : 'after'
  const parentId = placement === 'inside' ? target.id : target.parentId
  const valid = props.store.canSetParent(session.objectId, parentId)
  setLayerDropTarget(valid ? { id: target.id, placement } : null)
  scheduleLayerGroupExpand(target, valid ? placement : 'after')
}

const runLayerDragFrame = () => {
  layerDragFrame = null
  const session = layerDragSession
  if (!session) return
  session.ghost.style.transform = `translate3d(${session.clientX + 12}px, ${session.clientY + 12}px, 0)`
  updateLayerDropTarget(session.clientX, session.clientY)
  const list = layerListRef.value
  if (!list) return
  const rect = list.getBoundingClientRect()
  const edge = Math.min(44, rect.height / 4)
  const topDistance = session.clientY - rect.top
  const bottomDistance = rect.bottom - session.clientY
  const speed = topDistance >= 0 && topDistance < edge
    ? -Math.ceil((edge - topDistance) / 4)
    : bottomDistance >= 0 && bottomDistance < edge
      ? Math.ceil((edge - bottomDistance) / 4)
      : 0
  if (speed) {
    list.scrollTop += speed
    layerDragFrame = window.requestAnimationFrame(runLayerDragFrame)
  }
}

const scheduleLayerDragFrame = () => {
  if (layerDragFrame === null) layerDragFrame = window.requestAnimationFrame(runLayerDragFrame)
}

const startLayerPointerDrag = (event: PointerEvent, objectId: string) => {
  if (!canReorderLayerRows.value || event.button !== 0 || layerDragSession) return
  const grip = event.currentTarget as HTMLElement
  const row = grip.closest<HTMLElement>('.theater-layer-row')
  if (!row) return
  event.preventDefault()
  grip.setPointerCapture(event.pointerId)
  const rect = row.getBoundingClientRect()
  const ghost = row.cloneNode(true) as HTMLElement
  ghost.classList.add('is-drag-preview')
  ghost.setAttribute('aria-hidden', 'true')
  ghost.style.width = `${rect.width}px`
  document.body.appendChild(ghost)
  layerDragSession = { objectId, pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, ghost }
  draggedLayerId.value = objectId
  setLayerDropTarget(null)
  scheduleLayerDragFrame()
}

const moveLayerPointerDrag = (event: PointerEvent) => {
  if (!layerDragSession || event.pointerId !== layerDragSession.pointerId) return
  layerDragSession.clientX = event.clientX
  layerDragSession.clientY = event.clientY
  scheduleLayerDragFrame()
}

const applyLayerDrop = (objectId: string, dropTarget: typeof layerDropTarget.value) => {
  if (!dropTarget) return
  const previousPositions = new Map(Array.from(layerListRef.value?.querySelectorAll<HTMLElement>('.theater-layer-row') || [])
    .map((row) => [row.dataset.objectId || '', row.getBoundingClientRect().top]))
  const animateRows = () => {
    void nextTick(() => {
      layerListRef.value?.querySelectorAll<HTMLElement>('.theater-layer-row').forEach((row) => {
        const previousTop = previousPositions.get(row.dataset.objectId || '')
        if (previousTop === undefined) return
        const delta = previousTop - row.getBoundingClientRect().top
        if (!delta) return
        row.style.transition = 'none'
        row.style.transform = `translateY(${delta}px)`
        window.requestAnimationFrame(() => {
          row.style.transition = 'transform 120ms ease-out, color .14s ease, background .14s ease'
          row.style.transform = ''
          window.setTimeout(() => { row.style.transition = '' }, 140)
        })
      })
    })
  }
  if (dropTarget.id === null) {
    moveLayerObjectPreservingTransform(objectId, null)
    animateRows()
    return
  }
  const target = getObject(dropTarget.id)
  if (!target) return
  if (dropTarget.placement === 'inside' && target.type === 'group') {
    const topChild = Object.values(props.store.activeObjects.value)
      .filter((object) => object.parentId === target.id && object.id !== objectId)
      .sort(compareStageLayersTopToBottom)[0]
    if (!moveLayerObjectPreservingTransform(objectId, target.id, topChild?.id || null, 'before')) {
      stageMessage.warning('组内不能混合场景固定组件与当前场景组件')
    }
    animateRows()
    return
  }
  moveLayerObjectPreservingTransform(
    objectId,
    target.parentId,
    target.id,
    dropTarget.placement === 'before' ? 'before' : 'after',
  )
  animateRows()
}

const finishLayerPointerDrag = (event: PointerEvent, cancelled = false) => {
  const session = layerDragSession
  if (!session || event.pointerId !== session.pointerId) return
  if (!cancelled) updateLayerDropTarget(event.clientX, event.clientY)
  const dropTarget = layerDropTarget.value
  layerDragSession = null
  const grip = event.currentTarget as HTMLElement
  if (grip.hasPointerCapture(event.pointerId)) grip.releasePointerCapture(event.pointerId)
  if (layerDragFrame !== null) window.cancelAnimationFrame(layerDragFrame)
  layerDragFrame = null
  clearLayerExpandTimer()
  session.ghost.remove()
  draggedLayerId.value = null
  setLayerDropTarget(null)
  if (!cancelled) applyLayerDrop(session.objectId, dropTarget)
}

onMounted(() => {
  if (!containerRef.value || !sceneMorphContainerRef.value) return
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
  sceneMorphStage = new Konva.Stage({ container: sceneMorphContainerRef.value, width: 1, height: 1, listening: false })
  backgroundLayer = new Konva.Layer({ listening: false })
  worldLayer = new Konva.Layer()
  worldOverlayLayer = new Konva.Layer({ listening: false })
  foregroundLayer = new Konva.Layer({ listening: false })
  gridTopLayer = new Konva.Layer({ listening: false })
  interactionLayer = new Konva.Layer()
  backgroundCameraGroup = new Konva.Group()
  worldCameraGroup = new Konva.Group()
  worldOverlayCameraGroup = new Konva.Group()
  foregroundCameraGroup = new Konva.Group()
  gridTopCameraGroup = new Konva.Group()
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
    boundBoxFunc: (_oldBox, newBox) => gridResizeSession
      ? snapStageResizeBox(newBox, gridResizeSession)
      : newBox,
  })
  transformer.on('transformstart', () => {
    beginGridResize()
    if (!isBatchSelection.value || batchMoveBlocked.value) return
    const rootIds = [...selectedMovementRootIds()]
    if (!rootIds.length) return
    batchTransformRootIds = rootIds
    selectionGroupHitArea?.setAttrs({ visible: false, listening: false, draggable: false })
    props.store.beginObjectEdit('批量变换对象')
  })
  transformer.on('transform', () => {
    if (!batchTransformRootIds) return
    syncSelectionGroupHitArea(selectedMovementNodes().map(({ node }) => node))
    selectionGroupHitArea?.setAttrs({ visible: false, listening: false, draggable: false })
  })
  transformer.on('transformend', () => {
    finishGridResize()
    const rootIds = batchTransformRootIds
    batchTransformRootIds = null
    if (!rootIds) return
    let valid = true
    rootIds.forEach((id) => {
      const object = getObject(id)
      const node = objectNodes.get(id)
      if (!object || !node || !canEditObject(object)) {
        valid = false
        return
      }
      applyObjectNodeTransform(node, object)
    })
    if (valid) props.store.commitObjectEdit()
    else props.store.cancelObjectEdit()
    void nextTick(() => {
      syncObjects()
      updateTransformer()
    })
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
  selectionGroupHitArea = new Konva.Rect({
    visible: false,
    listening: false,
    draggable: false,
    fill: 'rgba(56, 189, 248, 0.001)',
  })
  selectionGroupHitArea.on('pointerdown', (event) => {
    if (!stage || !selectionGroupHitArea || !transformer) return
    const pointer = stage.getPointerPosition()
    if (!pointer) return
    selectionGroupHitArea.listening(false)
    transformer.listening(false)
    const target = stage.getIntersection(pointer)
    selectionGroupHitArea.listening(true)
    transformer.listening(true)
    const objectId = target ? stageObjectIdFromTarget(target) : null
    const selectionId = objectId ? editableCanvasSelectionTarget(objectId) : null
    const additive = event.evt.shiftKey || event.evt.ctrlKey || event.evt.metaKey
    if (!selectionId || (selectedIdSet.value.has(selectionId) && !additive)) return
    event.cancelBubble = true
    selectionGroupHitArea.draggable(false)
    selectionGroupHitArea.stopDrag()
    selectObject(selectionId, additive)
  })
  selectionGroupHitArea.on('dragstart', (event) => {
    if (!isBatchSelection.value || batchMoveBlocked.value) {
      selectionGroupHitArea?.stopDrag()
      return
    }
    event.cancelBubble = true
    const nodes = new Map<string, { node: Konva.Group, absolute: { x: number, y: number } }>()
    selectedMovementNodes().forEach(({ id, node }) => {
      nodes.set(id, { node, absolute: node.absolutePosition() })
    })
    selectionGroupDrag = {
      start: selectionGroupHitArea!.position(),
      nodes,
    }
    const anchor = nodes.values().next().value as { node: Konva.Group, absolute: { x: number, y: number } } | undefined
    if (anchor) beginNodeGridSnap(anchor.node)
    setGridSnapPreview(gridSnapEnabled.value)
    props.store.beginObjectEdit('批量移动对象')
  })
  selectionGroupHitArea.on('dragmove', (event) => {
    if (!selectionGroupDrag) return
    event.cancelBubble = true
    const current = selectionGroupHitArea!.position()
    const delta = {
      x: current.x - selectionGroupDrag.start.x,
      y: current.y - selectionGroupDrag.start.y,
    }
    selectionGroupDrag.nodes.forEach(({ node, absolute }) => {
      node.absolutePosition({ x: absolute.x + delta.x, y: absolute.y + delta.y })
    })
    if (gridSnapEnabled.value) {
      const anchor = selectionGroupDrag.nodes.values().next().value as { node: Konva.Group, absolute: { x: number, y: number } } | undefined
      if (!anchor) return
      const correction = snapNodeToGrid(anchor.node)
      selectionGroupDrag.nodes.forEach(({ node }) => {
        if (node === anchor.node) return
        const position = node.absolutePosition()
        node.absolutePosition({ x: position.x + correction.x, y: position.y + correction.y })
      })
    }
    transformer?.forceUpdate()
    syncSelectionGroupHitArea([...selectionGroupDrag.nodes.values()].map(({ node }) => node))
    interactionLayer?.batchDraw()
  })
  selectionGroupHitArea.on('dragend', (event) => {
    if (!selectionGroupDrag) return
    event.cancelBubble = true
    const currentDrag = selectionGroupDrag
    selectionGroupDrag = null
    currentDrag.nodes.forEach(({ node }, id) => {
      const object = getObject(id)
      if (!object) return
      object.transform.x = Number((node.x() / WORLD_UNIT_PX).toFixed(6))
      object.transform.y = Number((node.y() / WORLD_UNIT_PX).toFixed(6))
    })
    props.store.commitObjectEdit()
    setGridSnapPreview(false)
    void nextTick(() => {
      syncObjects()
      updateTransformer()
    })
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
  worldCameraGroup.add(gridGroup, objectRoot)
  worldOverlayCameraGroup.add(drawingDraftRoot, pointerTraceRoot)
  gridTopLayer.add(gridTopCameraGroup)
  backgroundLayer.add(backgroundCameraGroup)
  worldLayer.add(worldCameraGroup)
  worldOverlayLayer.add(worldOverlayCameraGroup)
  foregroundLayer.add(foregroundCameraGroup)
  interactionLayer.add(selectionRect, quickDeleteOutline, selectionGroupHitArea, transformer)
  stage.add(backgroundLayer, worldLayer, worldOverlayLayer, foregroundLayer, gridTopLayer, interactionLayer)
  backgroundLayer.getCanvas()._canvas.style.zIndex = '0'
  worldLayer.getCanvas()._canvas.style.zIndex = '10'
  worldOverlayLayer.getCanvas()._canvas.style.zIndex = String(WORLD_OVERLAY_LAYER_Z)
  foregroundLayer.getCanvas()._canvas.style.zIndex = '9000'
  gridTopLayer.getCanvas()._canvas.style.zIndex = String(GRID_TOP_LAYER_Z)
  interactionLayer.getCanvas()._canvas.style.zIndex = '10000'
  worldOverlayLayer.getCanvas()._canvas.style.pointerEvents = 'none'
  foregroundLayer.getCanvas()._canvas.style.pointerEvents = 'none'
  gridTopLayer.getCanvas()._canvas.style.pointerEvents = 'none'
  interactionLayer.getCanvas()._canvas.style.pointerEvents = 'none'
  stage.on('wheel', handleWheel)
  stage.on('pointerdown', startPan)
  stage.on('pointermove', movePan)
  stage.on('pointermove', handleStageImageAnnotationHover)
  stage.on('pointerup pointercancel', stopPan)
  stage.on('pointerleave', () => hideImageAnnotation())
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
  if (canManageResources.value) void fetchTheaterAudioAssets()
})

watch(() => props.store.state.activeSceneId, (sceneId, previousSceneId) => {
  hideImageAnnotation()
  closeImageAnnotationEditor()
  effectRuntime.invalidateCurrentMessage()
  beginSceneMediaBatch(sceneId, true, previousSceneId)
}, { flush: 'sync' })
watch(
  () => Object.fromEntries(Object.values(props.store.activeObjects.value).map((object) => [object.id, object.visible])),
  (next, previous) => {
    if (!props.syncReady) return
    networkVisibilityTargets.forEach((visible, objectId) => {
      if (!visible && next[objectId] === undefined) networkVisibilityTargets.delete(objectId)
    })
    Object.entries(next).forEach(([objectId, visible]) => {
      const object = props.store.activeObjects.value[objectId]
      const previousVisible = previous?.[objectId]
      if (object?.type !== 'text' || previousVisible === visible || pendingSceneEntrances) return
      const networkTarget = networkVisibilityTargets.get(objectId)
      if (networkTarget === visible) {
        networkVisibilityTargets.delete(objectId)
        if (!visible && departingStageObjects[objectId]) return
      }
      if (visible) playTextTransition(object, 'enter')
      else if (previousVisible === true) playTextTransition(object, 'exit')
    })
    void nextTick(() => {
      Object.entries(next).forEach(([objectId, visible]) => {
        const object = props.store.activeObjects.value[objectId]
        const node = objectNodes.get(objectId)
        const previousVisible = previous?.[objectId]
        if (!object || object.type === 'text' || !node || previousVisible === visible || pendingSceneEntrances) return
        const networkTarget = networkVisibilityTargets.get(objectId)
        if (networkTarget === visible) {
          networkVisibilityTargets.delete(objectId)
          if (!visible && departingStageObjects[objectId]) return
        }
        if (visible) playStageObjectEntrance(object, node)
        else if (previousVisible === true) playStageObjectTransition(object, node, 'exit')
      })
    })
  },
  { flush: 'sync' },
)
watch(() => props.store.state.liveState.sceneObjects, () => {
  if (layerHierarchyMovePending) syncLayerHierarchy()
  else syncObjects()
  effectRuntime.reconcile()
}, { deep: true })
watch(() => ({
  background: props.store.state.liveState.background,
  foreground: props.store.state.liveState.foreground,
  surfaceStyles: props.store.state.liveState.surfaceStyles,
  backgroundColor: props.store.state.liveState.backgroundColor,
  fieldWidth: props.store.state.liveState.fieldWidth,
  fieldHeight: props.store.state.liveState.fieldHeight,
  fieldObjectFit: props.store.state.liveState.fieldObjectFit,
}), syncSurfaceSlots, { deep: true })
watch(() => ({
  fieldWidth: props.store.state.liveState.fieldWidth,
  fieldHeight: props.store.state.liveState.fieldHeight,
  displayGrid: props.store.state.liveState.displayGrid,
  gridOnTop: props.store.state.liveState.gridOnTop,
  gridSize: props.store.state.liveState.gridSize,
  alignWithGrid: props.store.state.liveState.alignWithGrid,
}), scheduleGridSync, { deep: true })
watch(() => props.store.state.persistentObjects, () => {
  if (layerHierarchyMovePending) syncLayerHierarchy()
  else syncObjects()
  effectRuntime.reconcile()
}, { deep: true })
watch(() => props.store.state.camera, () => {
  applyCamera()
  scheduleGridSync()
  updateTransformer()
}, { deep: true })
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
watch(() => props.syncReady, (ready, wasReady) => {
  if (ready && !wasReady) beginSceneMediaBatch(props.store.state.activeSceneId, false)
})
watch(() => [props.syncReady, ...props.permissions], () => {
  const selectedId = props.store.state.selectedObjectId
  const object = selectedId ? props.store.activeObjects.value[selectedId] : null
  if (object && !canEditObject(object)) props.store.clearSelection()
  if (!canEditAllObjects.value && props.store.selection.bulkMode) props.store.setBulkSelectionMode(false)
  if (!canEditAllObjects.value) exitSceneBatchMode()
  if (!canEditAllObjects.value) quickDeleteActive.value = false
  if (!canOpenPanel('scene')) scenePanelOpen.value = false
  if (!canOpenPanel('inspector')) inspectorPanelOpen.value = false
  if (!canOpenPanel('layer')) layerPanelOpen.value = false
  if (!canOpenPanel('effect')) effectPanelOpen.value = false
  if (!canOpenPanel('overlay')) overlayPanelOpen.value = false
  if (!canOpenPanel('asset')) {
    assetPanelOpen.value = false
    theaterAudioAssets.value = []
    theaterAudioQuota.value = null
    theaterImageAssets.value = []
  } else {
    void Promise.all([fetchTheaterAudioAssets(), fetchTheaterImageAssets()])
  }
  syncObjects()
  if (props.syncReady) playPendingSceneEntrances(props.store.state.activeSceneId)
})
watch(
  () => [props.worldId, props.channelId, props.scopeType, canEditAllObjects.value] as const,
  () => { void loadTheaterGroupEditorState() },
  { immediate: true },
)
watch(
  () => [props.worldId, props.channelId, props.scopeType, props.syncReady] as const,
  () => { void fetchTheaterPanelOrganizer() },
  { immediate: true },
)
watch([effectPanelOpen, overlayPanelOpen, assetPanelOpen], ([effectOpen, overlayOpen, assetOpen]) => {
  if (effectOpen || overlayOpen || assetOpen) void fetchTheaterPanelOrganizer()
  if (overlayOpen || assetOpen) void fetchTheaterImageAssets()
})
watch(() => props.store.selection.selectedIds.slice(), () => {
  resourceError.value = ''
  if (layerHierarchyMovePending) syncLayerHierarchy()
  else syncObjects()
  updateTransformer()
})
watch([scenePanelOpen, inspectorPanelOpen, layerPanelOpen, effectPanelOpen, overlayPanelOpen, assetPanelOpen], async (open) => {
  await nextTick()
  const ids: PanelId[] = ['scene', 'inspector', 'layer', 'effect', 'overlay', 'asset']
  open.forEach((isOpen, index) => {
    if (isOpen) ensurePanelLayout(ids[index])
  })
  observeOpenPanels()
})
watch(() => [props.worldId, props.channelId], () => {
  if (canManageResources.value) void Promise.all([fetchTheaterAudioAssets(), fetchTheaterImageAssets()])
})
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
  hideImageAnnotation()
  if (actionDragFrame !== null) window.cancelAnimationFrame(actionDragFrame)
  actionDragFrame = null
  actionDragSession = null
  actionRowElements.clear()
  if (layerDragFrame !== null) window.cancelAnimationFrame(layerDragFrame)
  if (sceneDragFrame !== null) window.cancelAnimationFrame(sceneDragFrame)
  clearLayerExpandTimer()
  layerDragSession?.ghost.remove()
  layerDragSession = null
  sceneDragSession?.ghost.remove()
  sceneDragSession = null
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
  finishGridSnapPreview()
  if (gridSyncFrame !== null) window.cancelAnimationFrame(gridSyncFrame)
  gridSyncFrame = null
  cancelDrawingSession()
  finishPointerTrace()
  Array.from(pointerTraceVisuals.keys()).forEach(clearPointerTrace)
  if (sceneMediaBatch && !sceneMediaBatch.released) releaseSceneMediaBatch(sceneMediaBatch)
  sceneMediaBatch = null
  pendingSceneEntrances = null
  pendingTextEntranceIds.value = []
  finishSceneMorph()
  objectNodes.forEach(releaseObjectMedia)
  releaseStageMedia(backgroundSlot?.source)
  releaseStageMedia(foregroundSlot?.source)
  Array.from(activeAnimatedMedia).forEach(releaseStageMedia)
  mediaAnimation?.stop()
  mediaAnimation = null
  objectEntranceTweens.forEach((tween) => tween.destroy())
  objectEntranceTweens.clear()
  pendingObjectEntrances.clear()
  textEntranceTimers.forEach((timer) => window.clearTimeout(timer))
  textEntranceTimers.clear()
  Object.keys(textEntrancePlaybacks).forEach((objectId) => delete textEntrancePlaybacks[objectId])
  Object.keys(departingStageObjects).forEach((objectId) => delete departingStageObjects[objectId])
  networkVisibilityTargets.clear()
  playedVisibilityTriggerIds.clear()
  playedVisibilityTriggerOrder.length = 0
  objectNodes.clear()
  imageLoadVersions.clear()
  stageMediaBlobCache.clear()
  props.store.setBulkSelectionMode(false)
  sceneMorphTextCameras.forEach((camera) => camera.remove())
  sceneMorphTextCameras.clear()
  sceneMorphLayers.forEach(({ layer }) => layer.destroy())
  sceneMorphLayers.clear()
  sceneMorphStage?.destroy()
  sceneMorphStage = null
  stage?.destroy()
  stage = null
  objectRootLayers.clear()
  rootStackingOrder.value = {}
  gridTopLayer = null
  gridTopCameraGroup = null
  gridGroup = null
  worldOverlayLayer = null
  worldOverlayCameraGroup = null
  drawingDraftRoot = null
  pointerTraceRoot = null
})
</script>

<template>
  <section class="theater-stage-app">
    <input ref="imageInputRef" class="theater-image-input" type="file" accept="image/png,image/apng,image/jpeg,image/webp,image/gif,video/webm,.apng,.webm" @change="handleImageInput">
    <input ref="sceneAudioInputRef" class="theater-image-input" type="file" accept="audio/ogg,audio/mpeg,audio/wav,.ogg,.mp3,.wav" @change="handleSceneAudioInput">
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
        <n-tooltip v-if="canEditAllObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': layerPanelOpen }" aria-label="切换图层与属性面板" @click="togglePanel('layer')">
              <template #icon><n-icon><Stack2 /></n-icon></template>
            </n-button>
          </template>
          图层与属性
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': effectPanelOpen }" aria-label="切换特效层面板" @click="togglePanel('effect')">
              <template #icon><n-icon><Stars /></n-icon></template>
            </n-button>
          </template>
          特效层
        </n-tooltip>
        <n-tooltip v-if="canEditAllObjects" trigger="hover">
          <template #trigger>
            <n-button :class="{ 'is-active': overlayPanelOpen }" aria-label="切换场景叠加管理面板" @click="togglePanel('overlay')">
              <template #icon><n-icon><CloudRain /></n-icon></template>
            </n-button>
          </template>
          场景叠加
        </n-tooltip>
        <n-tooltip v-if="canManageResources" trigger="hover">
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
      <n-popover
        trigger="click"
        placement="bottom-start"
        :show-arrow="false"
        :width="220"
        :theme-overrides="theaterPopoverThemeOverrides"
        class="theater-secondary-surface"
      >
        <template #trigger>
          <n-button
            class="theater-bridge-status"
            :class="{
              'is-connected': chatBridgeStatus === 'connected',
              'is-reconnecting': chatBridgeStatus === 'connecting' || chatBridgeStatus === 'reconnecting',
              'is-manual-disconnected': chatBridgeStatus === 'manual-disconnected',
              'is-error': chatBridgeStatus === 'error' || chatBridgeStatus === 'disconnected',
            }"
            quaternary
            circle
            size="tiny"
            :aria-label="`聊天桥接：${chatBridgeStatusLabel}`"
            :title="`聊天桥接：${chatBridgeStatusLabel}`"
          >
            <template #icon><n-icon><component :is="chatBridgeStatus === 'manual-disconnected' ? BoltOff : Bolt" /></n-icon></template>
          </n-button>
        </template>
        <div class="theater-bridge-popover">
          <div class="theater-bridge-popover__heading">聊天桥接</div>
          <div class="theater-bridge-popover__status">
            <span
              class="theater-bridge-popover__dot"
              :class="{
                'is-connected': chatBridgeStatus === 'connected',
                'is-reconnecting': chatBridgeStatus === 'connecting' || chatBridgeStatus === 'reconnecting',
                'is-manual-disconnected': chatBridgeStatus === 'manual-disconnected',
                'is-error': chatBridgeStatus === 'error' || chatBridgeStatus === 'disconnected',
              }"
            />
            {{ chatBridgeStatusLabel }}
          </div>
          <div class="theater-bridge-popover__actions">
            <n-button
              v-if="chatBridgeStatus === 'connected'"
              size="tiny"
              secondary
              @click="emit('disconnectChatBridge')"
            >断开桥接</n-button>
            <n-button
              v-else
              size="tiny"
              type="primary"
              :loading="chatBridgeStatus === 'connecting' || chatBridgeStatus === 'reconnecting'"
              :disabled="chatBridgeStatus === 'connecting' || chatBridgeStatus === 'reconnecting'"
              @click="emit('reconnectChatBridge')"
            >重连桥接</n-button>
          </div>
          <div class="theater-bridge-popover__sync">
            舞台同步：{{ syncing ? '同步中' : syncReady ? '已连接' : '未连接' }}
          </div>
        </div>
      </n-popover>
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
        <n-tooltip trigger="hover"><template #trigger><n-button @click="store.addObject('group')"><template #icon><n-icon><FolderPlus /></n-icon></template></n-button></template>添加组</n-tooltip>
      </n-button-group>
      <span v-if="canEditAllObjects" class="theater-toolbar-divider" />
      <n-button-group v-if="canEditAllObjects" class="theater-stage-object-actions" size="small">
        <StageCopyToolbar
          :mode="copyMode"
          :disabled="!store.canCopy.value"
          @copy="copySelectedObjects"
          @select-mode="copyMode = $event"
        />
        <StageGridToolbar
          :snap-enabled="gridSnapEnabled"
          :display-grid="gridDisplayEnabled"
          :grid-on-top="gridOnTopEnabled"
          :disabled="!canEditAllObjects"
          @toggle-snap="toggleGridSnap"
          @toggle-display-grid="toggleGridDisplay"
          @toggle-grid-on-top="toggleGridOnTop"
        />
        <n-tooltip trigger="hover">
          <template #trigger>
            <n-badge
              class="theater-bulk-select-badge"
              :value="store.selectionGroup.value.memberIds.length"
              :max="99"
              :show="store.selection.bulkMode && store.selectionGroup.value.memberIds.length > 0"
            >
              <n-button
                class="theater-bulk-select-tool"
                :class="{ 'is-active': store.selection.bulkMode }"
                :aria-label="store.selection.bulkMode ? `批量选择组件，已选 ${store.selectionGroup.value.memberIds.length} 个` : '批量选择组件'"
                @click="toggleBulkSelectionMode"
              >
                <template #icon><n-icon><Select /></n-icon></template>
              </n-button>
            </n-badge>
          </template>
          {{ store.selection.bulkMode ? `已选 ${store.selectionGroup.value.memberIds.length} 个组件` : '批量选择组件' }}
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
        <n-tooltip trigger="hover"><template #trigger><n-button :disabled="!store.canCut.value" aria-label="剪切所选组件" @click="store.cutSelectedObjects"><template #icon><n-icon><Cut /></n-icon></template></n-button></template>剪切所选组件 Ctrl+X</n-tooltip>
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
        <div ref="sceneVisualRef" class="theater-scene-visual">
          <div ref="containerRef" class="theater-stage-canvas" />
          <SceneOverlayStageHost
            :scene-id="store.state.activeSceneId"
            :overlays="store.state.liveState.sceneOverlays"
            :resolve-resource-url="resolveTheaterResourceUrl"
          />
          <StageTextOverlay
            :objects="stageObjects"
            :camera="store.state.camera"
            :viewport-width="viewportSize.width"
            :viewport-height="viewportSize.height"
            :entrance-playbacks="textEntrancePlaybacks"
            :hidden-object-ids="pendingTextEntranceIds"
            :stacking-order="rootStackingOrder"
          />
          <div
            v-if="imageAnnotationOverlay.visible"
            ref="imageAnnotationOverlayRef"
            class="theater-image-annotation"
            :class="`is-${imageAnnotationOverlay.annotation.style}`"
            :style="imageAnnotationOverlayStyle"
          >{{ imageAnnotationOverlay.annotation.text }}</div>
          <div ref="sceneMorphContainerRef" class="theater-scene-morph-overlay" />
        </div>
        <div
          v-if="isBatchSelection && selectionQuickBar.visible"
          class="theater-selection-quick-bar"
          :style="{ left: `${selectionQuickBar.left}px`, top: `${selectionQuickBar.top}px` }"
          @pointerdown.stop
        >
          <n-tooltip trigger="hover"><template #trigger><n-button :class="{ 'is-active': batchBooleanChecked('visible'), 'is-mixed': batchBooleanIndeterminate('visible') }" :disabled="!batchBooleanObjects('visible').length" aria-label="批量显示或隐藏" @click.stop="toggleBatchQuickFlag('visible')"><template #icon><n-icon><component :is="batchBooleanChecked('visible') ? Eye : EyeOff" /></n-icon></template></n-button></template>{{ batchBooleanChecked('visible') ? '隐藏所选组件' : '显示所选组件' }}</n-tooltip>
          <n-tooltip trigger="hover"><template #trigger><n-button :class="{ 'is-active': batchBooleanChecked('locked'), 'is-mixed': batchBooleanIndeterminate('locked') }" :disabled="!batchBooleanObjects('locked').length" aria-label="批量锁定或解锁位置" @click.stop="toggleBatchQuickFlag('locked')"><template #icon><n-icon><component :is="batchBooleanChecked('locked') ? Lock : LockOpen" /></n-icon></template></n-button></template>{{ batchBooleanChecked('locked') ? '解锁位置' : '锁定位置' }}</n-tooltip>
          <n-tooltip trigger="hover"><template #trigger><n-button :class="{ 'is-active': batchBooleanChecked('aspectRatioLocked'), 'is-mixed': batchBooleanIndeterminate('aspectRatioLocked') }" :disabled="!batchBooleanObjects('aspectRatioLocked').length" aria-label="批量锁定或解锁比例" @click.stop="toggleBatchQuickFlag('aspectRatioLocked')"><template #icon><n-icon><AspectRatio /></n-icon></template></n-button></template>{{ batchBooleanChecked('aspectRatioLocked') ? '解锁比例' : '锁定比例' }}</n-tooltip>
          <n-tooltip trigger="hover"><template #trigger><n-button :class="{ 'is-active': batchBooleanChecked('editable'), 'is-mixed': batchBooleanIndeterminate('editable') }" :disabled="!batchBooleanObjects('editable').length" aria-label="批量设置可编辑" @click.stop="toggleBatchQuickFlag('editable')"><template #icon><n-icon><Edit /></n-icon></template></n-button></template>{{ batchBooleanChecked('editable') ? '取消可编辑' : '设为可编辑' }}</n-tooltip>
          <n-tooltip trigger="hover"><template #trigger><n-button :class="{ 'is-active': batchBooleanChecked('interactive'), 'is-mixed': batchBooleanIndeterminate('interactive') }" :disabled="!batchBooleanObjects('interactive').length" aria-label="批量设置可交互" @click.stop="toggleBatchQuickFlag('interactive')"><template #icon><n-icon><component :is="batchBooleanChecked('interactive') ? Bolt : BoltOff" /></n-icon></template></n-button></template>{{ batchBooleanChecked('interactive') ? '取消可交互' : '设为可交互' }}</n-tooltip>
        </div>
          <TheaterCharacterStatsOverlay
          :world-id="worldId"
          :channel-id="channelId"
          @open-character-card="emit('openCharacterCard', $event)"
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

      <aside v-if="scenePanelOpen && canOpenPanel('scene')" class="theater-floating-panel theater-scene-rail" data-panel-id="scene" :style="panelStyle('scene')" @pointerdown.capture="bringPanelToFront('scene')" @focusin="bringPanelToFront('scene')">
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
            <n-button v-if="canEditAllObjects" text size="tiny" aria-label="新建场景文件夹" :disabled="sceneEditMode" @click="createSceneFolder"><n-icon><FolderPlus /></n-icon></n-button>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭场景面板" @click="scenePanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <div ref="sceneListRef" class="theater-scene-list">
          <template v-for="entry in sceneListEntries" :key="entry.key">
          <div
            v-if="entry.kind === 'folder' || entry.kind === 'uncategorized'"
            class="theater-scene-folder"
            :class="{ 'is-collapsed': entry.collapsed, 'is-virtual': entry.kind === 'uncategorized' }"
          >
            <button type="button" class="theater-scene-folder__main" @click="toggleSceneFolder(entry.kind === 'folder' ? sceneFolderCollapseKey(entry.folder.id) : uncategorizedSceneFolderCollapseKey)">
              <n-icon :component="entry.collapsed ? ChevronRight : ChevronDown" />
              <n-icon><Folder /></n-icon>
              <strong>{{ entry.kind === 'folder' ? entry.folder.name : '未分类' }}</strong>
              <small>{{ entry.scenes.length }}</small>
            </button>
            <n-dropdown v-if="entry.kind === 'folder' && canEditAllObjects" trigger="click" :options="sceneFolderMenuOptions" :menu-props="theaterSecondaryMenuProps" @select="handleSceneFolderMenu($event, entry.folder.id)">
              <n-button quaternary circle size="tiny" aria-label="文件夹操作" @click.stop><template #icon><n-icon><Dots /></n-icon></template></n-button>
            </n-dropdown>
          </div>
          <div
            v-else
            :data-scene-id="entry.scene.id"
            class="theater-scene-row"
            :class="{
              'has-preload-pulse': scenePreloadPulse[entry.scene.id],
              'is-nested': entry.nested,
              'is-edit-mode': sceneEditMode,
              'is-batch-mode': sceneBatchMode,
              'has-scene-move-actions': canEditAllObjects,
              'is-dragging': draggedSceneId === entry.scene.id,
              'is-drop-before': sceneDropTarget?.id === entry.scene.id && sceneDropTarget.placement === 'before',
              'is-drop-after': sceneDropTarget?.id === entry.scene.id && sceneDropTarget.placement === 'after',
            }"
          >
            <n-checkbox
              v-if="sceneBatchMode && canEditAllObjects"
              class="theater-scene-row__select"
              :checked="isSceneBatchSelected(entry.scene.id)"
              :aria-label="`选择场景 ${entry.scene.name}`"
              @click.stop
              @update:checked="setSceneBatchSelected(entry.scene.id, $event)"
            />
            <span
              v-if="sceneEditMode && canEditAllObjects"
              class="theater-scene-row__grip"
              aria-label="拖动调整场景顺序"
              role="button"
              tabindex="0"
              @pointerdown.stop="startScenePointerDrag($event, entry.scene.id)"
              @pointermove.stop="moveScenePointerDrag"
              @pointerup.stop="finishScenePointerDrag($event)"
              @pointercancel.stop="finishScenePointerDrag($event, true)"
              @lostpointercapture.stop="finishScenePointerDrag($event, true)"
            >
              <n-icon><GripVertical /></n-icon>
            </span>
            <n-popover
            :show="sceneEditMode && editingSceneId === entry.scene.id"
            trigger="manual"
            :placement="sceneEditorPlacement"
            :show-arrow="false"
            :theme-overrides="theaterPopoverThemeOverrides"
            class="theater-secondary-surface theater-scene-editor-popover"
            scrollable
          >
            <template #trigger>
              <button
                class="theater-scene-card"
                :class="{ 'is-active': entry.scene.id === store.state.activeSceneId, 'is-editing': editingSceneId === entry.scene.id, 'is-selected': isSceneBatchSelected(entry.scene.id), 'is-construction-selected': sceneBatchMode === 'construction' && isSceneBatchSelected(entry.scene.id) }"
                :aria-pressed="sceneBatchMode ? isSceneBatchSelected(entry.scene.id) : undefined"
                :disabled="sceneEditMode || sceneBatchMode ? !canEditAllObjects : !canSwitchScene"
                @click="handleSceneClick(entry.scene)"
              >
                <span class="theater-scene-card__title">{{ entry.scene.name }}</span>
                <n-icon v-if="constructionSceneId === entry.scene.id" class="theater-scene-card__construction" aria-label="施工模式锁定场景"><Lock /></n-icon>
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
                <n-input v-model:value="editingSceneSwitchText" type="textarea" :autosize="{ minRows: 4, maxRows: 10 }" maxlength="10000" show-count />
              </label>
              <label>
                <span>场景切换音效</span>
                <div class="theater-scene-editor__audio">
                  <n-select
                    :value="editingSceneSwitchAudio?.assetId || null"
                    :options="sceneSwitchAudioOptions"
                    :loading="theaterAudioLoading"
                    size="small"
                    clearable
                    filterable
                    placeholder="从频道素材选择"
                    :menu-props="theaterSecondaryMenuProps"
                    @update:value="updateEditingSceneSwitchAudio"
                  />
                  <n-button size="small" secondary :disabled="!canUploadResources" :loading="theaterAudioUploading" aria-label="上传场景切换音效" @click="requestSceneAudioUpload">
                    <template #icon><n-icon><Upload /></n-icon></template>
                  </n-button>
                  <n-button size="small" secondary :disabled="!editingSceneSwitchAudio" aria-label="试听场景切换音效" @click="previewEditingSceneSwitchAudio">
                    <template #icon><n-icon><PlayerPlay /></n-icon></template>
                  </n-button>
                </div>
              </label>
              <label>
                <span>场景音乐</span>
                <div class="theater-scene-editor__music">
                  <span class="theater-scene-editor__music-summary">{{ sceneMusicSummary }}</span>
                  <n-popover
                    :show="sceneMusicPreviewVisible && Boolean(editingSceneMusicSnapshot)"
                    trigger="manual"
                    placement="right-start"
                    :show-arrow="false"
                    class="theater-scene-music-popover"
                  >
                    <template #trigger>
                      <n-button size="tiny" :disabled="!editingSceneMusicSnapshot" @click="sceneMusicPreviewVisible = !sceneMusicPreviewVisible">查看</n-button>
                    </template>
                    <div v-if="editingSceneMusicSnapshot" class="theater-scene-music-preview">
                      <strong>场景音乐播放列表</strong>
                      <section v-for="track in editingSceneMusicSnapshot.tracks" :key="track.type">
                        <header>
                          <span>{{ sceneMusicTrackLabels[track.type] }}</span>
                          <small>{{ track.playlistMode ? sceneMusicPlaylistModeLabels[track.playlistMode] : '单曲' }} · 音量 {{ Math.round(track.volume * 100) }}%</small>
                        </header>
                        <div class="theater-scene-music-preview__current">当前：{{ track.asset?.name || '未设置' }}</div>
                        <ol v-if="track.playlist.length">
                          <li v-for="(asset, index) in track.playlist" :key="`${track.type}-${asset.assetId}-${index}`" :class="{ 'is-current': index === track.playlistIndex }">
                            {{ asset.name || asset.assetId }}
                          </li>
                        </ol>
                        <small v-else>无播放列表</small>
                      </section>
                    </div>
                  </n-popover>
                  <n-button size="tiny" :disabled="!chatBridgeOnline" @click="editingSceneId && emit('sceneMusicRecordRequested', editingSceneId)">记录</n-button>
                  <n-button size="tiny" :disabled="!editingSceneMusicSnapshot" @click="editingSceneId && emit('sceneMusicClearRequested', editingSceneId)">清空</n-button>
                </div>
              </label>
              <div class="theater-scene-editor__transition">
                <span>退出动画</span>
                <n-select
                  :value="editingSceneTransition.exit.type"
                  :options="sceneTransitionTypeOptions"
                  size="small"
                  :menu-props="theaterSecondaryMenuProps"
                  @update:value="updateEditingSceneTransition('exit', { type: $event })"
                />
                <n-input-number
                  :value="editingSceneTransition.exit.durationMs"
                  :min="20"
                  :max="5000"
                  :show-button="false"
                  :precision="0"
                  size="small"
                  @update:value="updateEditingSceneTransitionDuration('exit', $event)"
                ><template #suffix>ms</template></n-input-number>
              </div>
              <div class="theater-scene-editor__transition">
                <span>进入动画</span>
                <n-select
                  :value="editingSceneTransition.enter.type"
                  :options="sceneTransitionTypeOptions"
                  size="small"
                  :menu-props="theaterSecondaryMenuProps"
                  @update:value="updateEditingSceneTransition('enter', { type: $event })"
                />
                <n-input-number
                  :value="editingSceneTransition.enter.durationMs"
                  :min="20"
                  :max="5000"
                  :show-button="false"
                  :precision="0"
                  size="small"
                  @update:value="updateEditingSceneTransitionDuration('enter', $event)"
                ><template #suffix>ms</template></n-input-number>
              </div>
              <small
                v-if="editingSceneTransition.enter.type === 'curtain' || editingSceneTransition.exit.type === 'curtain'"
                class="theater-scene-editor__transition-hint"
              ></small>
              <div class="theater-scene-editor__actions">
                <n-button size="small" @click="closeSceneEditor">取消</n-button>
                <n-button size="small" type="primary" :disabled="!editingSceneName.trim()" @click="saveSceneDetails">保存</n-button>
              </div>
            </div>
            </n-popover>
            <div v-if="(canSwitchScene || canEditAllObjects) && !sceneEditMode && !sceneBatchMode" class="theater-scene-row__actions">
              <n-dropdown v-if="canEditAllObjects" trigger="click" :options="sceneMoveOptions(entry.scene)" :menu-props="theaterSecondaryMenuProps" @select="moveSceneFromMenu($event, entry.scene.id)">
                <n-button quaternary circle size="tiny" aria-label="移动场景到文件夹" @click.stop><template #icon><n-icon><Dots /></n-icon></template></n-button>
              </n-dropdown>
              <n-tooltip v-if="canSwitchScene && !sceneEditMode" trigger="hover">
                <template #trigger>
                  <n-button class="theater-scene-preload" :class="{ 'is-ready-pulse': scenePreloadPulse[entry.scene.id] }" text size="tiny" :type="scenePreloadStatus[entry.scene.id] === 'ready' ? 'success' : scenePreloadStatus[entry.scene.id] === 'error' ? 'error' : 'default'" :loading="scenePreloadStatus[entry.scene.id] === 'loading'" :aria-label="`预加载场景 ${entry.scene.name}`" @click="requestScenePreload([entry.scene.id])"><n-icon><CloudDownload /></n-icon></n-button>
                </template>
                在所有设备预加载此场景
              </n-tooltip>
            </div>
          </div>
          </template>
        </div>
        <div v-if="canSwitchScene || canEditAllObjects" class="theater-scene-actions">
          <n-tooltip v-if="canEditAllObjects" trigger="hover">
            <template #trigger>
              <n-button size="tiny" quaternary :type="sceneBatchMode === 'copy' ? 'error' : 'default'" :disabled="sceneEditMode || duplicatingScene" :loading="duplicatingScene" @pointerdown="startSceneBatchLongPress($event, 'copy')" @pointerup="finishSceneBatchLongPress" @pointercancel="finishSceneBatchLongPress" @click="handleSceneActionClick('copy')"><template #icon><n-icon><Copy /></n-icon></template>{{ sceneBatchMode === 'copy' ? `复制 ${sceneBatchSelectedIds.length || ''}` : '复制' }}</n-button>
            </template>
            长按进入批量复制；批量模式下再次点击复制选中场景
          </n-tooltip>
          <n-tooltip v-if="canEditAllObjects" trigger="hover">
            <template #trigger>
              <n-button size="tiny" quaternary :type="sceneBatchMode === 'delete' ? 'error' : 'default'" :disabled="sceneEditMode || store.scenes.value.length <= 1" @pointerdown="startSceneBatchLongPress($event, 'delete')" @pointerup="finishSceneBatchLongPress" @pointercancel="finishSceneBatchLongPress" @click="handleSceneActionClick('delete')"><template #icon><n-icon><Trash /></n-icon></template>{{ sceneBatchMode === 'delete' ? `删除 ${sceneBatchSelectedIds.length || ''}` : '删除' }}</n-button>
            </template>
            长按进入批量删除；批量模式下再次点击删除选中场景
          </n-tooltip>
          <n-button v-if="sceneBatchMode && sceneBatchMode !== 'construction'" size="tiny" quaternary @click="exitSceneBatchMode"><template #icon><n-icon><X /></n-icon></template>取消</n-button>
          <n-button v-if="canEditAllObjects" size="tiny" quaternary :disabled="Boolean(sceneBatchMode)" :type="sceneEditMode ? 'primary' : 'default'" :aria-pressed="sceneEditMode" @click="toggleSceneEditMode"><template #icon><n-icon><Edit /></n-icon></template>编辑</n-button>
          <n-tooltip v-if="canEditAllObjects" trigger="hover">
            <template #trigger>
              <n-button
                size="tiny"
                quaternary
                :type="constructionSceneId || sceneBatchMode === 'construction' ? 'primary' : 'default'"
                :disabled="sceneEditMode || Boolean(sceneBatchMode && sceneBatchMode !== 'construction')"
                :aria-pressed="Boolean(constructionSceneId)"
                aria-label="施工模式（锁定场景）"
                @click="handleSceneActionClick('construction')"
              >
                <template #icon><n-icon><Lock /></n-icon></template>
                {{ sceneBatchMode === 'construction' && constructionSceneId && !sceneBatchSelectedIds.length ? '解除' : '锁定' }}
              </n-button>
            </template>
            施工模式：选择锁定场景后，除管理员外，其他成员进入小剧场时将不再跟随场景切换。
          </n-tooltip>
          <div class="theater-scene-playback-toggles">
            <n-tooltip trigger="hover">
              <template #trigger>
                <label class="theater-scene-playback-toggle">
                  <span>音效</span>
                  <n-switch size="small" :value="sceneAudioEnabled" @update:value="emit('updateSceneAudioEnabled', $event)" />
                </label>
              </template>
              切换场景时向所有在线舞台播放已配置音效
            </n-tooltip>
            <label class="theater-scene-playback-toggle">
              <span>台词</span>
              <n-switch size="small" :value="sceneDialogueEnabled" @update:value="emit('updateSceneDialogueEnabled', $event)" />
            </label>
          </div>
        </div>
      </aside>

      <aside v-if="inspectorPanelOpen && canOpenPanel('inspector')" class="theater-floating-panel theater-object-inspector" data-panel-id="inspector" :style="panelStyle('inspector')" @pointerdown.capture="bringPanelToFront('inspector')" @focusin="bringPanelToFront('inspector')">
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
                :disabled="!batchBooleanObjects('interactive').length"
                @update:checked="updateBatchBoolean('interactive', $event)"
              >可交互</n-checkbox>
              <n-checkbox
                :checked="batchBooleanChecked('editable')"
                :indeterminate="batchBooleanIndeterminate('editable')"
                :disabled="!batchBooleanObjects('editable').length"
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
            <template v-if="selectedObject.type === 'image' && canEditObject(selectedObject)">
              <label>图片</label>
              <div class="theater-image-actions">
                <n-button size="small" :disabled="!canUploadImageTarget({ kind: 'object', objectId: selectedObject.id })" :loading="resourceUploading" @click="requestImageUpload({ kind: 'object', objectId: selectedObject.id })">
                  <template #icon><n-icon><Photo /></n-icon></template>上传替换
                </n-button>
                <n-tooltip trigger="hover">
                  <template #trigger>
                    <n-button size="small" :disabled="!canUploadImageTarget({ kind: 'object', objectId: selectedObject.id }) || !selectedObject.image || selectedObject.image.animated || resourceUploading" aria-label="编辑图片" @click="openImageEditor({ kind: 'object', objectId: selectedObject.id })">
                      <template #icon><n-icon><Edit /></n-icon></template>
                    </n-button>
                  </template>
                  编辑图片
                </n-tooltip>
                <n-button size="small" quaternary type="error" :disabled="!selectedObject.image" @click="clearImage({ kind: 'object', objectId: selectedObject.id })">清除</n-button>
              </div>
              <label>图片标注</label>
              <div class="theater-image-annotation-entry-row">
                <n-button class="theater-image-annotation-entry" size="small" secondary @click="openImageAnnotationEditor(selectedObject.id)">
                  <template #icon><n-icon><Settings /></n-icon></template>
                  设置悬停文字标注
                </n-button>
              </div>
            </template>
            <template v-if="selectedObject.type === 'image' && selectedObjectSupportsEntrance && canEditObject(selectedObject)">
              <label>显隐动画</label>
              <div class="theater-entrance-editor">
                <n-select
                  :value="selectedEntranceConfig.preset"
                  :options="entrancePresetOptions"
                  size="small"
                  :menu-props="theaterSecondaryMenuProps"
                  @update:value="updateSelectedEntrancePreset"
                />
                <n-button
                  size="small"
                  :disabled="selectedEntranceConfig.preset === 'none' || !selectedObject.visible || !selectedObjectCanPreviewEntrance"
                  @click="previewSelectedEntrance"
                >
                  <template #icon><n-icon><Stars /></n-icon></template>
                  预览
                </n-button>
              </div>
              <label>动画时长</label>
              <n-input-number
                :value="selectedEntranceConfig.durationMs"
                :min="150"
                :max="STAGE_ENTRANCE_MAX_DURATION_MS"
                :step="50"
                :precision="0"
                size="small"
                @update:value="updateSelectedEntranceDuration"
              >
                <template #suffix>ms</template>
              </n-input-number>
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
                <n-select v-model:value="selectedObject.drawing.style.dash" :options="drawingDashOptions" size="small" filterable :menu-props="theaterSecondaryMenuProps" />
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
              <label>X</label><n-input-number v-model:value="selectedObject.transform.x" :precision="2" :disabled="!canEditSelectedTransform" />
              <label>Y</label><n-input-number v-model:value="selectedObject.transform.y" :precision="2" :disabled="!canEditSelectedTransform" />
              <template v-if="selectedObject.type === 'group'">
                <label>旋转</label><n-input-number v-model:value="selectedObject.transform.rotation" :precision="2" />
                <label>缩放 X</label><n-input-number :value="selectedObject.transform.scaleX" :min="0.01" :max="100" :step="0.1" :precision="2" @update:value="updateSelectedScale('scaleX', $event)" />
                <label>缩放 Y</label><n-input-number :value="selectedObject.transform.scaleY" :min="0.01" :max="100" :step="0.1" :precision="2" @update:value="updateSelectedScale('scaleY', $event)" />
              </template>
              <template v-else>
                <label>宽</label><n-input-number :value="selectedObject.transform.width" :min="0.5" :precision="2" :disabled="!canEditSelectedTransform" @update:value="updateSelectedDimension('width', $event)" />
                <label>高</label><n-input-number :value="selectedObject.transform.height" :min="0.5" :precision="2" :disabled="!canEditSelectedTransform" @update:value="updateSelectedDimension('height', $event)" />
                <template v-if="selectedObject.image?.animated">
                  <label>循环次数</label><n-input-number :value="selectedObject.image.loopCount ?? null" :min="1" :max="65535" :step="1" :precision="0" clearable placeholder="无限循环" @update:value="updateSelectedLoopCount" />
                </template>
              </template>
            </div>
            <div v-if="canEditAllObjects || (selectedObject.type === 'image' && canEditObject(selectedObject))" class="theater-object-editor__checks">
              <n-checkbox v-if="canEditAllObjects" v-model:checked="selectedObject.visible">显示</n-checkbox>
              <n-checkbox v-if="canEditAllObjects && selectedObject.type !== 'group'" v-model:checked="selectedObject.interactive">可交互</n-checkbox>
              <n-checkbox v-if="canEditAllObjects && selectedObject.type !== 'group'" v-model:checked="selectedObject.editable">可编辑</n-checkbox>
              <n-checkbox
                v-if="canEditAllObjects && selectedObject.type === 'group'"
                :checked="store.isSceneFixedObject(selectedObject.id)"
                disabled
              >跨场景</n-checkbox>
              <n-checkbox v-model:checked="selectedObject.locked">锁定位置</n-checkbox>
              <n-checkbox
                :checked="selectedObject.aspectRatioLocked"
                @update:checked="updateSelectedAspectRatioLocked"
              >锁定比例</n-checkbox>
            </div>
            <template v-if="canEditAllObjects && isStageActionTarget(selectedObject.type)">
              <label>点击动作</label>
              <label class="theater-action-execution-mode">
                <span>执行方式</span>
                <n-switch
                  size="small"
                  :value="selectedObject.metadata.actionExecutionMode !== 'sequential'"
                  @update:value="selectedObject.metadata.actionExecutionMode = $event ? 'parallel' : 'sequential'"
                >
                  <template #checked>同步播放</template>
                  <template #unchecked>依次播放</template>
                </n-switch>
              </label>
              <div class="theater-action-add">
                <n-button size="tiny" @click="addAction('chat.send')">发送</n-button>
                <n-button size="tiny" @click="addAction('chat.random-table')">随机表</n-button>
                <n-button size="tiny" @click="addAction('chat.insert')">插入</n-button>
                <n-button size="tiny" @click="addAction('scene.apply')">场景</n-button>
                <n-button size="tiny" :disabled="!effectActionOptions.length" @click="addAction('effect.play')">特效</n-button>
                <n-button size="tiny" @click="addAction('object.toggle')">显隐</n-button>
                <n-button size="tiny" @click="addAction('action.sequence')">组合</n-button>
              </div>
              <div
                v-for="(action, actionIndex) in selectedObject.actions"
                :key="action.id"
                :ref="element => setActionRowElement(action.id, element as Element | null)"
                class="theater-action-row"
                :class="{
                  'is-dragging': draggingActionId === action.id,
                  'is-drop-before': draggingActionId && actionDropIndex === actionIndex,
                  'is-drop-after': draggingActionId && actionDropIndex === selectedObject.actions.length && actionIndex === selectedObject.actions.length - 1,
                }"
              >
                <button
                  type="button"
                  class="theater-action-row__handle"
                  title="拖动排序"
                  aria-label="拖动调整动作顺序"
                  @pointerdown.stop="startActionPointerDrag($event, action.id)"
                  @pointermove.stop="moveActionPointerDrag"
                  @pointerup.stop="finishActionPointerDrag($event)"
                  @pointercancel.stop="finishActionPointerDrag($event, true)"
                  @lostpointercapture.stop="finishActionPointerDrag($event, true)"
                  @keydown.alt.up.prevent="moveActionByKeyboard(action.id, -1)"
                  @keydown.alt.down.prevent="moveActionByKeyboard(action.id, 1)"
                ><n-icon><GripVertical /></n-icon></button>
                <small>{{ action.type }} {{ stageActionDescriptions[action.type] }}</small>
                <div
                  class="theater-action-row__controls"
                  :class="{ 'is-sequential': selectedObject.metadata.actionExecutionMode === 'sequential' }"
                >
                  <n-input v-if="action.type === 'chat.send' || action.type === 'chat.insert'" v-model:value="action.payload.content" class="theater-action-row__target" size="tiny" maxlength="10000" />
                  <div v-else-if="action.type === 'chat.random-table'" class="theater-action-row__target theater-random-table-actions">
                    <n-button size="tiny" secondary @click="openRandomTableEditor(action.id)">
                      编辑 · {{ action.payload.name }} · {{ action.payload.formula }} · {{ action.payload.entries.length }} 项
                    </n-button>
                    <n-tooltip>
                      <template #trigger>
                        <n-button
                          size="tiny"
                          secondary
                          circle
                          aria-label="掷骰并发送随机表结果"
                          @click="triggerSingleObjectAction(selectedObject, action)"
                        ><template #icon><n-icon><PlayerPlay /></n-icon></template></n-button>
                      </template>
                      掷骰并发送结果
                    </n-tooltip>
                  </div>
                  <n-select v-else-if="action.type === 'scene.apply'" v-model:value="action.payload.sceneId" class="theater-action-row__target" :options="store.scenes.value.map((scene) => ({ label: scene.name, value: scene.id }))" size="tiny" filterable :menu-props="theaterSecondaryMenuProps" />
                  <n-select v-else-if="action.type === 'effect.play'" v-model:value="action.payload.effectId" class="theater-action-row__target" :options="effectActionOptions" size="tiny" filterable :menu-props="theaterSecondaryMenuProps" />
                  <n-select v-else-if="action.type === 'object.toggle'" v-model:value="action.payload.objectId" class="theater-action-row__target" :options="Object.values(store.activeObjects.value).map((item) => ({ label: item.name, value: item.id }))" size="tiny" filterable :menu-props="theaterSecondaryMenuProps" />
                  <n-button v-else class="theater-action-row__target" size="tiny" secondary @click="openSequenceEditor(action.id)">编辑组合 · {{ action.payload.steps.length }} 项</n-button>
                  <n-input-number
                    v-if="selectedObject.metadata.actionExecutionMode === 'sequential'"
                    :value="actionDelaySeconds(action.schedule?.delayMs)"
                    class="theater-action-row__timing"
                    size="tiny"
                    :min="0"
                    :max="STAGE_ACTION_MAX_DELAY_MS / 1_000"
                    :step="STAGE_ACTION_DELAY_STEP_MS / 1_000"
                    :precision="1"
                    :show-button="false"
                    placeholder="间隔"
                    title="间隔时延"
                    aria-label="动作间隔时延（秒）"
                    @update:value="updateActionDelaySeconds(action.id, $event)"
                  ><template #suffix>s</template></n-input-number>
                  <n-button text type="error" size="tiny" aria-label="删除动作" @click="removeObjectActionWithConfirm(selectedObject.id, action.id)"><n-icon><Trash /></n-icon></n-button>
                </div>
              </div>
            </template>
            <template v-if="canEditAllObjects">
              <label>父级</label>
              <n-select
                :value="selectedObject.parentId"
                :options="parentOptions"
                size="small"
                filterable
                :menu-props="theaterSecondaryMenuProps"
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
              <n-button size="small" secondary :type="selectedObject.type === 'group' ? 'warning' : 'error'" @click="removeObjectsWithConfirm([selectedObject.id])"><template #icon><n-icon><Trash /></n-icon></template>{{ selectedObject.type === 'group' ? '解散组' : '删除组件' }}</n-button>
              <n-button v-if="selectedObject.type === 'group'" size="small" secondary type="error" @click="removeGroupTreeWithConfirm(selectedObject.id)"><template #icon><n-icon><Trash /></n-icon></template>删除组及成员</n-button>
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

      <aside v-if="layerPanelOpen && canOpenPanel('layer')" class="theater-floating-panel theater-layer-panel" data-panel-id="layer" :style="panelStyle('layer')" @pointerdown.capture="bringPanelToFront('layer')" @focusin="bringPanelToFront('layer')">
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
          <div class="theater-scene-overlay-settings-row">
            <label>场景叠加效果</label>
            <div class="theater-image-actions">
              <small>{{ store.state.liveState.sceneOverlays.length }} 个 · {{ store.state.liveState.sceneOverlays.filter(item => item.enabled).length }} 已启用</small>
              <n-button size="tiny" quaternary aria-label="配置场景叠加效果" @click="openOverlayPanel"><template #icon><n-icon><Settings /></n-icon></template></n-button>
            </div>
          </div>
          <small v-if="resourceError" class="theater-resource-error">{{ resourceError }}</small>
        </div>
        <div class="theater-panel-heading theater-layer-list-heading">
          <span>层级</span>
          <div class="theater-panel-heading__actions">
            <n-tooltip trigger="hover">
              <template #trigger>
                <button
                  type="button"
                  class="theater-layer-filter-toggle"
                  :class="{ 'is-active': layerFilterOpen || layerFilterActive }"
                  :aria-expanded="layerFilterOpen"
                  :aria-label="layerFilterOpen ? '收起图层筛选' : '展开图层筛选'"
                  @click="toggleLayerFilterPanel"
                >
                  <n-icon :component="Filter" />
                  <small v-if="layerFilterActiveCount">{{ layerFilterActiveCount }}</small>
                </button>
              </template>
              {{ layerFilterOpen ? '收起筛选' : '筛选组件' }}
            </n-tooltip>
          </div>
        </div>
        <div v-if="layerFilterOpen" class="theater-layer-filter" @pointerdown.stop>
          <n-input
            v-model:value="layerNameFilter"
            class="theater-layer-filter__search"
            clearable
            size="small"
            placeholder="搜索组件名称"
          >
            <template #prefix><n-icon :component="Search" /></template>
          </n-input>
          <n-tooltip trigger="hover">
            <template #trigger>
              <button
                type="button"
                class="theater-layer-filter__toggle"
                :class="{ 'is-active': layerHiddenOnly }"
                :aria-pressed="layerHiddenOnly"
                aria-label="仅隐藏组件"
                @click="layerHiddenOnly = !layerHiddenOnly"
              ><n-icon :component="EyeOff" /></button>
            </template>
            仅隐藏
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <button
                type="button"
                class="theater-layer-filter__toggle"
                :class="{ 'is-active': layerSceneFixedOnly }"
                :aria-pressed="layerSceneFixedOnly"
                aria-label="仅场景固定组件"
                @click="layerSceneFixedOnly = !layerSceneFixedOnly"
              ><n-icon :component="Pin" /></button>
            </template>
            仅场景固定
          </n-tooltip>
          <n-tooltip v-if="layerFilterActive" trigger="hover">
            <template #trigger>
              <button type="button" class="theater-layer-filter__clear" aria-label="清除图层筛选" @click="clearLayerFilters"><n-icon><X /></n-icon></button>
            </template>
            清除筛选
          </n-tooltip>
        </div>
        <div ref="layerListRef" class="theater-layer-list">
          <div
            v-if="canReorderLayerRows"
            class="theater-layer-root-drop"
            :class="{ 'is-drop-target': layerDropTarget?.id === null }"
            aria-hidden="true"
          ></div>
          <div v-if="layerFilterActive && !layerRows.length" class="theater-layer-list__empty">未找到匹配组件</div>
          <div v-else-if="!layerFilterActive && !layerRows.length" class="theater-layer-list__empty">暂无组件</div>
          <div
            v-for="row in layerRows"
            :key="row.object.id"
            :data-object-id="row.object.id"
            class="theater-layer-row"
            :class="{
              'is-active': store.selection.selectedIds.includes(row.object.id),
              'is-hidden': !row.object.visible,
              'is-filter-context': layerFilterContextGroupIds.has(row.object.id) && !layerFilterMatchIds.has(row.object.id),
              'is-disabled': !canEditObject(row.object),
              'is-dragging': draggedLayerId === row.object.id,
              'is-drop-before': layerDropTarget?.id === row.object.id && layerDropTarget.placement === 'before',
              'is-drop-inside': layerDropTarget?.id === row.object.id && layerDropTarget.placement === 'inside',
              'is-drop-after': layerDropTarget?.id === row.object.id && layerDropTarget.placement === 'after',
            }"
            :style="{ paddingLeft: `${10 + row.depth * 15}px` }"
          >
            <span
              class="theater-layer-row__grip"
              :class="{ 'is-disabled': !canReorderLayerRows }"
              @pointerdown.stop="startLayerPointerDrag($event, row.object.id)"
              @pointermove.stop="moveLayerPointerDrag"
              @pointerup.stop="finishLayerPointerDrag($event)"
              @pointercancel.stop="finishLayerPointerDrag($event, true)"
              @lostpointercapture.stop="finishLayerPointerDrag($event, true)"
            >
              <n-icon><GripVertical /></n-icon>
            </span>
            <button
              type="button"
              class="theater-layer-row__select"
              :disabled="!canEditObject(row.object)"
              @click="selectLayerObject(row.object.id, store.selection.bulkMode && ($event.shiftKey || $event.ctrlKey || $event.metaKey))"
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
              <small v-if="store.isSceneFixedObject(row.object.id)">{{ row.object.type === 'group' ? '跨场景' : '场景固定' }}</small>
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
              <n-tooltip v-if="row.object.type !== 'group'" trigger="hover">
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
              <n-tooltip v-if="row.object.type === 'group' && !layerFilterActive" trigger="hover">
                <template #trigger>
                  <button
                    type="button"
                    class="theater-layer-row__action"
                    :class="{ 'is-enabled': !isLayerGroupCollapsed(row.object) }"
                    :aria-expanded="!isLayerGroupCollapsed(row.object)"
                    :aria-label="isLayerGroupCollapsed(row.object) ? `展开 ${row.object.name}` : `折叠 ${row.object.name}`"
                    @click.stop="toggleLayerGroupCollapsed(row.object)"
                  >
                    <n-icon :component="isLayerGroupCollapsed(row.object) ? ChevronRight : ChevronDown" />
                  </button>
                </template>
                {{ isLayerGroupCollapsed(row.object) ? '展开组' : '折叠组' }}
              </n-tooltip>
            </div>
          </div>
        </div>
      </aside>

      <aside v-if="effectPanelOpen && canOpenPanel('effect')" class="theater-floating-panel theater-effect-panel" data-panel-id="effect" :style="panelStyle('effect')" @pointerdown.capture="bringPanelToFront('effect')" @focusin="bringPanelToFront('effect')">
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
          :organizer-folders="theaterPanelOrganizer.folders"
          :organizer-items="theaterPanelOrganizer.items"
          @update:editing-target="effectEditingTarget = $event"
          @upload="objectId => requestImageUpload({ kind: 'object', objectId })"
          @upload-audio="(objectId, file) => uploadTheaterAudio(file, objectId)"
          @create-folder="done => createTheaterPanelFolder('effect', done)"
          @rename-folder="renameTheaterPanelFolder"
          @delete-folder="deleteTheaterPanelFolder"
          @collapse-folder="setTheaterPanelFolderCollapsed"
          @reorder-folders="folderIds => reorderTheaterPanelFolders('effect', folderIds)"
          @reorder-items="(folderId, targetIds) => reorderTheaterPanelItems('effect', folderId, targetIds)"
        />
      </aside>

      <aside v-if="overlayPanelOpen && canOpenPanel('overlay')" class="theater-floating-panel theater-overlay-panel" data-panel-id="overlay" :style="panelStyle('overlay')" @pointerdown.capture="bringPanelToFront('overlay')" @focusin="bringPanelToFront('overlay')">
        <div class="theater-panel-heading" @pointerdown="startPanelDrag('overlay', $event)">
          <span>场景叠加管理</span>
          <div class="theater-panel-heading__actions">
            <small>{{ store.state.liveState.sceneOverlays.length }}</small>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭场景叠加管理面板" @click="overlayPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <SceneOverlayManagerPanel
          :store="store"
          :can-edit="canEditAllObjects"
          :image-assets="sceneOverlayImageAssets"
          :image-loading="theaterImageLoading"
          :image-uploading="theaterImageUploading"
          :image-error="theaterImageError"
          :can-upload-media="canUploadResources"
          :can-edit-media="canUploadResources || canDeleteResources"
          :can-delete-media="canDeleteResources"
          @upload-media="uploadSceneOverlayMedia"
          @rename-media="renameTheaterImageAsset"
          @delete-media="deleteTheaterImageAsset"
        />
      </aside>

      <aside v-if="assetPanelOpen && canOpenPanel('asset')" class="theater-floating-panel theater-asset-panel" data-panel-id="asset" :style="panelStyle('asset')" @pointerdown.capture="bringPanelToFront('asset')" @focusin="bringPanelToFront('asset')">
        <div class="theater-panel-heading" @pointerdown="startPanelDrag('asset', $event)">
          <span>素材管理器</span>
          <div class="theater-panel-heading__actions">
            <small>{{ theaterImageAssets.length + theaterAudioAssets.length }}</small>
            <n-button class="theater-panel-close" text size="tiny" aria-label="关闭素材管理器" @click="assetPanelOpen = false"><n-icon><X /></n-icon></n-button>
          </div>
        </div>
        <TheaterAssetManager
          :assets="theaterAudioAssets"
          :image-assets="theaterImageAssets"
          :image-loading="theaterImageLoading"
          :image-uploading="theaterImageUploading"
          :image-error="theaterImageError"
          :quota="theaterAudioQuota"
          :loading="theaterAudioLoading"
          :uploading="theaterAudioUploading"
          :error="theaterAudioError"
          :can-upload="canUploadResources"
          :can-delete="canDeleteResources"
          :can-edit-objects="canEditAllObjects"
          :referenced-asset-ids="referencedTheaterAudioAssetIds"
          :master-volume="theaterAudioMasterVolume"
          :organizer-folders="theaterPanelOrganizer.folders"
          :organizer-items="theaterPanelOrganizer.items"
          @update:master-volume="theaterAudioMasterVolume = $event"
          @refresh="fetchTheaterAudioAssets"
          @upload="uploadTheaterAudio"
          @preview="previewTheaterAudio"
          @delete="deleteTheaterAudio"
          @delete-batch="deleteTheaterAudioBatch"
          @refresh-images="fetchTheaterImageAssets"
          @upload-images="uploadTheaterImageAssets"
          @rename-image="renameTheaterImageAsset"
          @delete-image="deleteTheaterImageAsset"
          @delete-image-batch="deleteTheaterImageAssetsBatch"
          @create-image-folder="done => createTheaterPanelFolder('image', done)"
          @update-image-folder-preset="updateTheaterImageFolderPreset"
          @update-image-asset-preset="updateTheaterImageAssetPreset"
          @reorder-image-folders="folderIds => reorderTheaterPanelFolders('image', folderIds)"
          @reorder-image-items="(folderId, targetIds) => reorderTheaterPanelItems('image', folderId, targetIds)"
          @create-folder="done => createTheaterPanelFolder('audio', done)"
          @rename-folder="renameTheaterPanelFolder"
          @delete-folder="deleteTheaterPanelFolder"
          @collapse-folder="setTheaterPanelFolderCollapsed"
          @reorder-folders="folderIds => reorderTheaterPanelFolders('audio', folderIds)"
          @reorder-items="(folderId, targetIds) => reorderTheaterPanelItems('audio', folderId, targetIds)"
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
    <StageImageAnnotationEditor
      v-if="imageAnnotationEditorObject"
      :show="imageAnnotationEditorVisible"
      :value="imageAnnotationForObject(imageAnnotationEditorObject)"
      :object-name="imageAnnotationEditorObject.name"
      @close="closeImageAnnotationEditor"
      @save="saveImageAnnotation"
    />
    <TheaterActionSequenceEditor
      v-model:show="sequenceEditorVisible"
      :component-name="selectedObject?.name || ''"
      :action="editingSequenceAction"
      :scenes="store.scenes.value"
      :persistent-objects="store.state.persistentObjects"
      :active-scene-id="store.state.activeSceneId"
    />
    <TheaterRandomTableEditor
      v-model:show="randomTableEditorVisible"
      :component-name="selectedObject?.name || ''"
      :action="editingRandomTableAction"
      @save="saveRandomTable"
    />
    <n-modal
      :show="sceneFolderDialogVisible"
      :mask-closable="false"
      :close-on-esc="true"
      @update:show="value => { if (!value) closeSceneFolderDialog() }"
    >
      <div class="theater-scene-folder-dialog" role="dialog" aria-modal="true" :aria-label="sceneFolderDialogMode === 'create' ? '新建场景文件夹' : '重命名场景文件夹'">
        <header class="theater-scene-folder-dialog__header">
          <div>
            <strong>{{ sceneFolderDialogMode === 'create' ? '新建场景文件夹' : '重命名场景文件夹' }}</strong>
            <small>文件夹仅用于整理场景列表</small>
          </div>
          <n-button text aria-label="关闭文件夹弹窗" @click="closeSceneFolderDialog"><n-icon><X /></n-icon></n-button>
        </header>
        <div class="theater-scene-folder-dialog__body">
          <n-input
            v-model:value="sceneFolderNameDraft"
            autofocus
            maxlength="128"
            show-count
            placeholder="输入文件夹名称"
            @keydown="handleSceneFolderDialogKeydown"
          />
        </div>
        <footer class="theater-scene-folder-dialog__footer">
          <n-button @click="closeSceneFolderDialog">取消</n-button>
          <n-button type="primary" @click="submitSceneFolderDialog">确定</n-button>
        </footer>
      </div>
    </n-modal>
    <n-modal v-model:show="packageProgressVisible" :mask-closable="false" :closable="false" preset="card" title="小剧场导入进度" class="theater-package-progress-modal">
      <div class="theater-package-progress">
        <n-progress type="line" :percentage="Math.round(packageDisplayedProgress * 100)" :status="packageProgressJob?.status === 'failed' ? 'error' : packageProgressJob?.status === 'done' ? 'success' : 'default'" />
        <div class="theater-package-progress-meta">
          <span>{{ packageProgressJob?.progressStage || '等待任务开始' }}</span>
          <span v-if="packageProgressJob?.progressTotal">{{ packageDisplayedDone }} / {{ packageProgressJob.progressTotal }}</span>
        </div>
        <p v-if="packageProgressError" class="theater-package-progress-error">{{ packageProgressError }}</p>
        <n-button v-if="packageProgressError || packageProgressJob?.status === 'done' || packageProgressJob?.status === 'failed'" type="primary" block @click="packageProgressVisible = false">关闭</n-button>
      </div>
    </n-modal>
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
:global(.theater-scene-editor-popover) {
  box-sizing: border-box;
  width: min(320px, calc(100vw - 24px));
  max-width: calc(100vw - 24px);
  max-height: min(640px, calc(100vh - 24px));
  max-height: min(640px, calc(100dvh - 24px));
}
:global(body:has(.theater-stage-app) .v-binder-follower-container) {
  z-index: 10002 !important;
}
:global(body:has(.theater-stage-app) .n-modal-container) {
  z-index: 10004 !important;
}
.theater-image-input { display: none; }
.theater-scene-folder-dialog {
  width: min(420px, calc(100vw - 32px));
  overflow: hidden;
  border: 1px solid var(--sc-border-strong, rgba(255, 255, 255, .16));
  border-radius: 7px;
  color: var(--sc-text-primary, #f4f4f5);
  background: color-mix(in srgb, var(--sc-bg-surface, #262626) 48%, transparent);
  box-shadow: 0 14px 34px rgba(0, 0, 0, .2);
  backdrop-filter: blur(8px) saturate(110%);
  -webkit-backdrop-filter: blur(8px) saturate(110%);
}
.theater-scene-folder-dialog__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08));
}
.theater-scene-folder-dialog__header div { min-width: 0; display: grid; gap: 3px; }
.theater-scene-folder-dialog__header strong { font-size: 15px; }
.theater-scene-folder-dialog__header small { color: var(--sc-text-secondary, #b5b5c5); font-size: 10px; }
.theater-scene-folder-dialog__body { padding: 16px; }
.theater-scene-folder-dialog__footer { display: flex; justify-content: flex-end; gap: 8px; padding: 0 16px 14px; }
.theater-package-progress-modal {
  width: min(460px, calc(100vw - 32px));
  background: color-mix(in srgb, var(--sc-bg-surface, #262626) 72%, transparent) !important;
  border: 1px solid var(--sc-border-strong, rgba(255, 255, 255, .16));
  backdrop-filter: blur(12px) saturate(120%);
  -webkit-backdrop-filter: blur(12px) saturate(120%);
}
.theater-package-progress { display: grid; gap: 12px; }
.theater-package-progress-meta { display: flex; justify-content: space-between; color: var(--sc-text-secondary, rgba(255, 255, 255, .68)); font-size: 12px; }
.theater-package-progress-error { margin: 0; color: var(--sc-color-error, #ef4444); white-space: pre-wrap; word-break: break-word; }
.theater-stage-toolbar {
  position: absolute; z-index: 10000; top: 0; right: 0; left: 0; box-sizing: border-box;
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
.theater-toolbar-exit, .theater-grid-snap-tool, .theater-bulk-select-tool, .theater-quick-delete-tool, .theater-panel-switches, .theater-stage-object-actions { flex: 0 0 auto; }
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
.theater-stage-object-actions :deep(.theater-copy-trigger--primary),
.theater-stage-object-actions :deep(.theater-scene-fixed-trigger--primary),
.theater-stage-object-actions :deep(.theater-grid-trigger--primary),
.theater-stage-object-actions :deep(.theater-drawing-trigger--primary) {
  --n-width: 30px !important;
  --n-padding: 0 !important;
  width: 30px;
  min-width: 30px;
}
.theater-stage-object-actions :deep(.theater-copy-trigger--menu),
.theater-stage-object-actions :deep(.theater-scene-fixed-trigger--menu),
.theater-stage-object-actions :deep(.theater-grid-trigger--menu),
.theater-stage-object-actions :deep(.theater-drawing-trigger--menu) {
  --n-width: 18px !important;
  --n-padding: 0 !important;
  width: 18px;
  min-width: 18px;
}
.theater-bulk-select-badge { display: inline-flex; }
.theater-grid-snap-tool.is-active, .theater-bulk-select-tool.is-active, .theater-panel-switches :deep(.n-button.is-active) {
  color: #fff; background: var(--theater-accent); border-color: var(--theater-accent);
}
.theater-quick-delete-tool.is-active { color: #fff; background: #dc2626; border-color: #dc2626; }
.theater-toolbar-divider { width: 1px; height: 22px; flex: 0 0 1px; margin: 0 2px; background: var(--theater-border); }
.theater-bridge-status { width: 28px; min-width: 28px; height: 28px; flex: 0 0 28px; padding: 0; color: var(--sc-fg-muted, #71717a); }
.theater-bridge-status.is-connected { color: #22c55e; }
.theater-bridge-status.is-reconnecting { color: #f59e0b; }
.theater-bridge-status.is-manual-disconnected { color: var(--sc-fg-muted, #71717a); }
.theater-bridge-status.is-error { color: #ef4444; }
.theater-bridge-popover { display: grid; gap: 8px; min-width: 184px; font-size: 12px; }
.theater-bridge-popover__heading { color: var(--sc-text-primary, #f4f4f5); font-weight: 600; }
.theater-bridge-popover__status { display: flex; align-items: center; gap: 7px; color: var(--sc-text-secondary, rgba(255, 255, 255, .72)); }
.theater-bridge-popover__dot { width: 7px; height: 7px; flex: 0 0 7px; border-radius: 50%; background: var(--sc-fg-muted, #71717a); }
.theater-bridge-popover__dot.is-connected { background: #22c55e; box-shadow: 0 0 0 3px rgba(34, 197, 94, .12); }
.theater-bridge-popover__dot.is-reconnecting { background: #f59e0b; box-shadow: 0 0 0 3px rgba(245, 158, 11, .12); }
.theater-bridge-popover__dot.is-error { background: #ef4444; box-shadow: 0 0 0 3px rgba(239, 68, 68, .12); }
.theater-bridge-popover__actions { display: flex; justify-content: flex-end; }
.theater-bridge-popover__sync { padding-top: 2px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); color: var(--sc-text-muted, rgba(255, 255, 255, .52)); }
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
.theater-stage-viewport { position: absolute; inset: 0; min-width: 0; min-height: 0; overflow: hidden; isolation: isolate; background: #343435; touch-action: none; }
.theater-scene-visual { position: absolute; z-index: 0; inset: 0; overflow: hidden; transform-origin: center; will-change: opacity, transform, filter, clip-path; }
.theater-stage-viewport :global(.theater-scene-transition-overlay) { position: absolute; z-index: 0; inset: 0; overflow: hidden; pointer-events: none; transform-origin: center; will-change: opacity, transform, filter, clip-path; }
.theater-stage-viewport :global(.theater-scene-transition-overlay > canvas) { position: absolute; inset: 0; width: 100%; height: 100%; }
.theater-scene-morph-overlay { position: absolute; inset: 0; overflow: hidden; pointer-events: none; }
.theater-scene-morph-overlay :deep(.konvajs-content), .theater-scene-morph-overlay :deep(canvas) { position: absolute !important; inset: 0; pointer-events: none !important; }
.theater-scene-morph-overlay :global(.theater-scene-morph-text-camera) { position: absolute; top: 0; left: 0; width: 0; height: 0; transform-origin: 0 0; pointer-events: none; }
.theater-stage-viewport :global(.theater-scene-curtain-overlay) { position: absolute; z-index: 3; inset: 0; display: flex; overflow: hidden; pointer-events: none; }
.theater-stage-viewport :global(.theater-scene-curtain-panel) { width: 50%; height: 100%; display: block; background: #000; will-change: transform; }
.theater-stage-viewport :global(.theater-scene-curtain-panel.is-left) { transform: translateX(-100%); }
.theater-stage-viewport :global(.theater-scene-curtain-panel.is-right) { transform: translateX(100%); }
.theater-appearance-preview-layer { position: absolute; z-index: 9500; inset: 0; overflow: hidden; }
.theater-appearance-preview-layer :deep(.theater-preview) { min-height: 0; background: transparent; }
.theater-stage-viewport.is-drawing :deep(canvas) { cursor: crosshair !important; }
.theater-stage-viewport.is-viewing :deep(canvas) { cursor: grab !important; }
.theater-stage-viewport.is-erasing :deep(canvas) { cursor: cell !important; }
.theater-stage-viewport.is-quick-deleting :deep(canvas) { cursor: crosshair !important; }
.theater-stage-canvas { position: absolute; inset: 0; }
.theater-image-annotation {
  position: absolute; z-index: 10001; box-sizing: border-box; padding: 9px 11px; white-space: pre-wrap; overflow-wrap: anywhere;
  border: 1px solid rgba(255, 255, 255, .24); border-radius: 8px; line-height: 1.45; pointer-events: none;
  box-shadow: 0 8px 24px rgba(0, 0, 0, .34); backdrop-filter: blur(8px); -webkit-backdrop-filter: blur(8px);
}
.theater-image-annotation.is-bubble { border-radius: 12px; }
.theater-image-annotation.is-tag { padding: 5px 8px; border-radius: 4px; font-weight: 650; line-height: 1.25; }
.theater-image-annotation.is-floating { border-color: rgba(255, 255, 255, .16); border-radius: 6px; }
.theater-image-annotation.is-footer { padding: 7px 10px; border-radius: 3px; }
.theater-image-annotation-entry-row { min-width: 0; padding-right: 3px; }
.theater-image-annotation-entry { box-sizing: border-box; width: 100%; max-width: 100%; justify-content: flex-start; }
.theater-selection-quick-bar {
  position: absolute; z-index: 10000; display: inline-flex; gap: 2px; padding: 3px;
  border: 1px solid var(--theater-border); border-radius: 6px; background: rgba(24, 24, 27, .92);
  box-shadow: 0 6px 16px rgba(0, 0, 0, .28); transform: translateX(-50%); pointer-events: auto;
}
.theater-selection-quick-bar :deep(.n-button) { width: 28px; height: 28px; padding: 0; }
.theater-selection-quick-bar :deep(.n-button.is-active:not(:disabled)) { color: #fff; background: var(--theater-accent); border-color: var(--theater-accent); }
.theater-selection-quick-bar :deep(.n-button.is-mixed:not(:disabled)) { color: #fbbf24; }
.theater-floating-panel {
  position: absolute; z-index: 10000; box-sizing: border-box; display: flex; flex-direction: column; min-height: 0; overflow: hidden;
  border: 1px solid var(--theater-border); border-radius: 7px; background: var(--theater-panel);
  box-shadow: 0 14px 34px rgba(0, 0, 0, .2); backdrop-filter: blur(8px) saturate(110%); -webkit-backdrop-filter: blur(8px) saturate(110%);
  resize: both; max-width: 100%; max-height: 100%; animation: theater-panel-in .16s ease-out;
}
@keyframes theater-panel-in { from { opacity: 0; transform: translateY(-4px); } }
.theater-scene-rail { min-width: min(124px, 100%); min-height: min(160px, 100%); gap: 0; padding: 0; overflow: hidden; }
.theater-scene-list { flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 0 6px 6px; }
.theater-object-inspector { min-width: min(240px, 100%); min-height: min(240px, 100%); overflow: hidden; }
.theater-object-inspector > .theater-inspector { flex: 1 1 auto; min-height: 0; overflow-y: auto; }
.theater-layer-panel { min-width: min(280px, 100%); min-height: min(220px, 100%); }
.theater-effect-panel { min-width: min(320px, 100%); min-height: min(320px, 100%); }
.theater-overlay-panel { min-width: min(520px, 100%); min-height: min(360px, 100%); }
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
.theater-scene-row.is-edit-mode, .theater-scene-row.is-batch-mode { grid-template-columns: 16px minmax(0, 1fr); }
.theater-scene-row__select { width: 16px; display: grid; place-items: center; justify-self: center; }
.theater-scene-row__actions { display: flex; align-items: center; gap: 2px; min-width: 0; }
.theater-scene-row__actions {
  position: absolute; right: 0; padding-left: 6px; opacity: 0; pointer-events: none;
  background: linear-gradient(90deg, transparent, var(--theater-panel) 10px); transition: opacity .14s ease;
}
.theater-scene-row:hover .theater-scene-row__actions, .theater-scene-row:has(button:focus-visible) .theater-scene-row__actions, .theater-scene-row.has-preload-pulse .theater-scene-row__actions { opacity: 1; pointer-events: auto; }
.theater-scene-row:hover .theater-scene-card, .theater-scene-row:has(button:focus-visible) .theater-scene-card, .theater-scene-row.has-preload-pulse .theater-scene-card { padding-right: 36px; }
.theater-scene-row.has-scene-move-actions:hover .theater-scene-card, .theater-scene-row.has-scene-move-actions:has(button:focus-visible) .theater-scene-card, .theater-scene-row.has-scene-move-actions.has-preload-pulse .theater-scene-card { padding-right: 66px; }
.theater-scene-row.is-dragging { opacity: .36; }
.theater-scene-row.is-drag-preview {
  position: fixed; z-index: 10003; top: 0; left: 0; pointer-events: none; opacity: .92;
  border: 1px solid color-mix(in srgb, var(--theater-accent, #38bdf8) 58%, transparent); border-radius: 5px;
  background: var(--theater-panel); box-shadow: 0 10px 24px rgba(0, 0, 0, .26); will-change: transform;
}
.theater-scene-row.is-drop-before::before, .theater-scene-row.is-drop-after::after {
  position: absolute; z-index: 1; right: 5px; left: 5px; height: 2px; border-radius: 1px; background: #38bdf8; content: '';
}
.theater-scene-row.is-drop-before::before { top: 0; }
.theater-scene-row.is-drop-after::after { bottom: 0; }
.theater-scene-row__grip { width: 16px; height: 100%; display: grid; place-items: center; color: var(--sc-fg-muted, #71717a); font-size: 14px; cursor: grab; touch-action: none; user-select: none; }
.theater-scene-row__grip:active { cursor: grabbing; }
.theater-scene-folder { display: flex; align-items: center; gap: 2px; min-height: 32px; padding: 1px 2px; color: var(--sc-text-secondary, #b5b5c5); }
.theater-scene-folder__main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 5px; padding: 6px 6px; border: 0; border-radius: 5px; color: inherit; background: transparent; font-size: 11px; text-align: left; cursor: pointer; }
.theater-scene-folder__main:hover { color: var(--sc-text-primary, #f4f4f5); background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-scene-folder__main strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-scene-folder__main small { margin-left: auto; color: var(--sc-fg-muted, #71717a); font-size: 10px; }
.theater-scene-folder.is-virtual { margin-top: 3px; border-top: 1px solid color-mix(in srgb, var(--theater-border) 70%, transparent); }
.theater-scene-row.is-nested .theater-scene-card { padding-left: 24px; }
.theater-scene-card {
  width: 100%; display: flex; align-items: center; min-height: 34px; padding: 7px 8px; border: 1px solid transparent; border-radius: 6px;
  color: var(--sc-text-secondary, #b5b5c5); background: transparent; font-size: 12px; line-height: 1.2; text-align: left; cursor: pointer;
  transition: color .14s ease, border-color .14s ease, background .14s ease;
}
.theater-scene-card__construction { flex: 0 0 auto; margin-left: auto; color: var(--theater-accent, #38bdf8); font-size: 13px; }
.theater-scene-card:hover { color: var(--sc-text-primary, #f4f4f5); background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-scene-card.is-active { color: var(--sc-text-primary, #f4f4f5); border-color: color-mix(in srgb, var(--theater-accent) 70%, transparent); background: color-mix(in srgb, var(--theater-accent) 16%, transparent); }
.theater-scene-card.is-selected { border-color: color-mix(in srgb, #ef4444 74%, transparent); background: color-mix(in srgb, #ef4444 16%, transparent); }
.theater-scene-card.is-construction-selected { border-color: color-mix(in srgb, var(--theater-accent) 74%, transparent); background: color-mix(in srgb, var(--theater-accent) 16%, transparent); }
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
.theater-scene-actions { flex: 0 0 auto; display: flex; flex-wrap: wrap; align-items: center; gap: 1px; padding: 6px; border-top: 1px solid var(--theater-border); background: var(--theater-panel); }
.theater-scene-playback-toggles { display: flex; align-items: center; gap: 8px; margin-left: auto; }
.theater-scene-playback-toggle { display: flex; align-items: center; gap: 5px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-scene-editor { display: grid; gap: 10px; }
.theater-scene-editor strong { font-size: 13px; }
.theater-scene-editor label { display: grid; gap: 5px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-scene-editor__audio { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 6px; }
.theater-scene-editor__music { display: grid; grid-template-columns: minmax(0, 1fr) auto auto auto; align-items: center; gap: 5px; }
.theater-scene-editor__music-summary { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 5px 8px; border: 1px solid var(--theater-border); border-radius: 4px; color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--theater-panel) 78%, transparent); }
:global(.theater-scene-music-popover) { background: rgba(28, 31, 40, .86) !important; backdrop-filter: blur(14px); border: 1px solid rgba(255, 255, 255, .12); }
.theater-scene-music-preview { width: 300px; max-height: min(520px, calc(100vh - 32px)); overflow: auto; display: grid; gap: 10px; color: var(--sc-text-primary, #f4f4f5); }
.theater-scene-music-preview > strong { font-size: 13px; }
.theater-scene-music-preview section { display: grid; gap: 5px; padding-top: 8px; border-top: 1px solid rgba(255, 255, 255, .1); }
.theater-scene-music-preview header { display: flex; justify-content: space-between; gap: 8px; }
.theater-scene-music-preview header small, .theater-scene-music-preview section > small { color: var(--sc-text-secondary, #b5b5c5); }
.theater-scene-music-preview__current { font-size: 12px; }
.theater-scene-music-preview ol { max-height: 150px; overflow: auto; margin: 0; padding-left: 22px; font-size: 11px; color: var(--sc-text-secondary, #b5b5c5); }
.theater-scene-music-preview li.is-current { color: var(--sc-primary, #63a8ff); }
.theater-scene-editor__transition { display: grid; grid-template-columns: 58px minmax(0, 1fr) 104px; align-items: center; gap: 6px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-scene-editor__transition-hint { color: var(--sc-fg-muted, #71717a); font-size: 10px; line-height: 1.4; }
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
.theater-scene-overlay-settings-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding-top: 3px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-scene-overlay-settings-row .theater-image-actions { min-width: 0; }
.theater-scene-overlay-settings-row small { overflow: hidden; color: var(--sc-text-secondary); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.theater-entrance-editor { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 6px; }
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
.theater-layer-list-heading { border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-layer-filter { display: flex; align-items: center; gap: 4px; padding: 6px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-layer-filter__search { min-width: 0; flex: 1; }
.theater-layer-filter__toggle, .theater-layer-filter__clear, .theater-layer-filter-toggle {
  position: relative; width: 26px; height: 26px; display: grid; place-items: center; flex: 0 0 26px; padding: 0;
  border: 0; border-radius: 5px; color: var(--sc-fg-muted, #71717a); background: transparent; cursor: pointer;
}
.theater-layer-filter__toggle:hover, .theater-layer-filter__clear:hover, .theater-layer-filter-toggle:hover { color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--sc-text-primary, #f4f4f5) 9%, transparent); }
.theater-layer-filter__toggle.is-active, .theater-layer-filter-toggle.is-active { color: #7dd3fc; background: rgba(56, 189, 248, .14); }
.theater-layer-filter-toggle small { position: absolute; top: -3px; right: -3px; min-width: 13px; padding: 0 3px; border-radius: 8px; color: #082f49; background: #7dd3fc; font-size: 9px; line-height: 13px; }
.theater-layer-filter__toggle:focus-visible, .theater-layer-filter__clear:focus-visible, .theater-layer-filter-toggle:focus-visible { outline: 2px solid var(--theater-accent); outline-offset: 1px; }
.theater-layer-list { min-height: 100px; flex: 1; overflow: auto; padding: 4px 0; }
.theater-layer-root-drop {
  position: relative; height: 8px; margin: 0 6px;
}
.theater-layer-root-drop::after { position: absolute; right: 0; bottom: 2px; left: 0; height: 2px; border-radius: 1px; background: #38bdf8; content: ''; opacity: 0; transition: opacity .12s ease; }
.theater-layer-root-drop.is-drop-target::after { opacity: 1; }
.theater-layer-list__empty { display: grid; min-height: 82px; place-items: center; padding: 12px; color: var(--sc-fg-muted, #71717a); font-size: 11px; text-align: center; }
.theater-layer-row {
  position: relative; box-sizing: border-box; width: 100%; height: 38px; display: flex; align-items: center; gap: 5px;
  color: var(--sc-text-primary, #f4f4f5); background: transparent; font-size: 12px; text-align: left;
  transition: color .14s ease, background .14s ease;
}
.theater-layer-row:hover { background: var(--sc-sidebar-hover, rgba(255, 255, 255, .08)); }
.theater-layer-row.is-active { color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--theater-accent) 18%, transparent); }
.theater-layer-row.is-filter-context:not(.is-active) { opacity: .66; }
.theater-layer-row.is-dragging { opacity: .36; }
.theater-layer-row.is-drag-preview {
  position: fixed; z-index: 10003; top: 0; left: 0; pointer-events: none; opacity: .92;
  border: 1px solid color-mix(in srgb, var(--theater-accent, #38bdf8) 58%, transparent); border-radius: 5px;
  background: var(--sc-bg-panel, #26262b); box-shadow: 0 8px 24px rgba(0, 0, 0, .28); will-change: transform;
}
.theater-layer-row.is-disabled .theater-layer-row__select { cursor: default; }
.theater-layer-row.is-hidden .theater-layer-row__preview,
.theater-layer-row.is-hidden .theater-layer-row__name { opacity: .46; }
.theater-layer-row.is-drop-inside { outline: 1px solid #38bdf8; outline-offset: -2px; background: rgba(56, 189, 248, .12); }
.theater-layer-row.is-drop-before::before, .theater-layer-row.is-drop-after::after {
  position: absolute; right: 5px; left: 5px; height: 2px; border-radius: 1px; background: #38bdf8; content: '';
}
.theater-layer-row.is-drop-before::before { top: 0; }
.theater-layer-row.is-drop-after::after { bottom: 0; }
.theater-layer-row__grip { width: 16px; height: 100%; flex: 0 0 16px; display: grid; place-items: center; color: var(--sc-fg-muted, #71717a); font-size: 14px; cursor: grab; touch-action: none; user-select: none; }
.theater-layer-row__grip:active { cursor: grabbing; }
.theater-layer-row__grip.is-disabled, .theater-layer-row__grip.is-disabled:active { cursor: default; opacity: .48; }
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
.theater-action-execution-mode { display: flex; align-items: center; justify-content: space-between; gap: 8px; color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-action-row { position: relative; display: grid; grid-template-columns: 18px minmax(0, 1fr); column-gap: 4px; padding: 6px; border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); border-radius: 6px; }
.theater-action-row.is-dragging { opacity: .42; }
.theater-action-row.is-drop-before::before, .theater-action-row.is-drop-after::after { position: absolute; right: 4px; left: 4px; z-index: 1; height: 2px; content: ''; background: var(--theater-accent, #60a5fa); box-shadow: 0 0 7px color-mix(in srgb, var(--theater-accent, #60a5fa) 65%, transparent); }
.theater-action-row.is-drop-before::before { top: -2px; }
.theater-action-row.is-drop-after::after { bottom: -2px; }
.theater-action-row__handle { grid-row: 1 / span 2; align-self: stretch; width: 18px; display: grid; place-items: center; padding: 0; border: 0; color: var(--sc-fg-muted, #71717a); background: transparent; cursor: grab; touch-action: none; }
.theater-action-row__handle:active { cursor: grabbing; }
.theater-action-row__handle:focus-visible { outline: 2px solid var(--theater-accent); outline-offset: 1px; }
.theater-action-row small { min-width: 0; margin-bottom: 4px; overflow: hidden; color: var(--sc-text-secondary, #b5b5c5); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.theater-action-row__controls { min-width: 0; display: grid; grid-template-columns: minmax(72px, 1fr) 24px; align-items: center; gap: 4px; }
.theater-action-row__controls.is-sequential { grid-template-columns: minmax(72px, 1fr) 62px 24px; }
.theater-action-row__target, .theater-action-row__timing { min-width: 0; width: 100%; }
.theater-random-table-actions { display: grid; grid-template-columns: minmax(0, 1fr) 24px; gap: 4px; }
.theater-random-table-actions > .n-button:first-child { min-width: 0; overflow: hidden; }
.theater-action-row__timing :deep(.n-input__input-el) { padding-right: 0; }
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
