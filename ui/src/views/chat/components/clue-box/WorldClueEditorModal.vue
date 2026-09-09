<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { NButton, NColorPicker, NForm, NFormItem, NIcon, NInput, NInputNumber, NModal, NSelect, NSlider, NSwitch, useMessage } from 'naive-ui'
import { DeviceFloppy, X } from '@vicons/tabler'
import { urlBase } from '@/stores/_config'
import { chatEvent } from '@/stores/chat'
import { useWorldClueStore, type WorldClueDetail, type WorldCluePresentation } from '@/stores/worldClue'
import RichTextEditor from '@/components/rich-text/RichTextEditor.vue'
import WorldClueContentView from '@/components/world-clue/WorldClueContentView.vue'
import WorldClueDistributionPanel from './WorldClueDistributionPanel.vue'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'

const props = defineProps<{ show: boolean; worldId: string; channelId: string; clue?: WorldClueDetail | null; canManage: boolean }>()
const emit = defineEmits<{ (event: 'update:show', value: boolean): void; (event: 'saved', clue: WorldClueDetail): void }>()
const store = useWorldClueStore()
const message = useMessage()
const saving = ref(false)
const workingClue = ref<WorldClueDetail | null>(null)
const privatePreview = ref<{
  userId: string
  name: string
  format: 'plain' | 'tiptap'
  content: string
} | null>(null)
const autosaveReady = ref(false)
const savedSnapshot = ref('')
const saveFailed = ref(false)
const remoteConflict = ref(false)
const pendingRemoteRevision = ref(0)
const distributionPanel = ref<InstanceType<typeof WorldClueDistributionPanel> | null>(null)
let autosaveTimer: ReturnType<typeof setTimeout> | null = null
let saveFlight: Promise<boolean> | null = null
let session = 0
let sessionWorldId = ''
let sessionChanged = false
const closing = ref(false)
const publishing = ref(false)
const rich = ref(false)
const managerRich = ref(false)
const imageUploading = ref(false)
const imageFileInput = ref<HTMLInputElement | null>(null)
const imagePreviewUrl = ref('')
const selectedImageName = ref('')
const backgroundMediaUploading = ref(false)
const backgroundMediaFileInput = ref<HTMLInputElement | null>(null)
const backgroundMediaPreviewUrl = ref('')
const selectedBackgroundMediaName = ref('')
const selectedBackgroundMediaIsVideo = ref(false)
const backgroundVideoFallbackUrl = ref('')
const activeTab = ref<'content' | 'distribution' | 'manager'>('content')
const lockSessionId = ref('')
const ownedLockFields = ref<Set<string>>(new Set())
const acquiringLockFields = ref<Set<string>>(new Set())
const pendingReleaseLockFields = ref<Set<string>>(new Set())
const lockClock = ref(Date.now())
let lockRenewTimer: ReturnType<typeof setInterval> | null = null
const contentEditorRef = ref<InstanceType<typeof RichTextEditor> | null>(null)
const managerEditorRef = ref<InstanceType<typeof RichTextEditor> | null>(null)
const emptyTiptapDocument = '{"type":"doc","content":[]}'
const mediaPlacementOptions = [{ label: '左侧', value: 'left' }, { label: '右侧', value: 'right' }, { label: '上方', value: 'top' }, { label: '下方', value: 'bottom' }]
const objectFitOptions = [{ label: '完整显示', value: 'contain' }, { label: '裁切填充', value: 'cover' }]
const enterAnimationOptions = [{ label: '无', value: 'none' }, { label: '淡入', value: 'fade' }, { label: '上浮淡入', value: 'fade-up' }, { label: '缩放进入', value: 'scale' }, { label: '淡入缩放', value: 'fade-scale' }, { label: '从左滑入', value: 'slide-left' }, { label: '从右滑入', value: 'slide-right' }]
const exitAnimationOptions = [{ label: '无', value: 'none' }, { label: '淡出', value: 'fade' }, { label: '上浮淡出', value: 'fade-up' }, { label: '缩放退出', value: 'scale' }, { label: '淡出缩放', value: 'fade-scale' }, { label: '向左滑出', value: 'slide-left' }, { label: '向右滑出', value: 'slide-right' }]
const backgroundMediaModeOptions = [{ label: '铺满', value: 'cover' }, { label: '适应', value: 'contain' }, { label: '平铺', value: 'tile' }, { label: '居中', value: 'center' }]

const defaultPresentation = (): WorldCluePresentation => ({
  version: 1, mediaPlacement: 'left', mediaRatio: .42, objectFit: 'contain', mediaType: 'image',
  enterAnimation: 'fade-scale', exitAnimation: 'fade', animationDurationMs: 280,
  titleColor: '#ffffff',
  backgroundColorEnabled: false, backgroundColor: '#000000', backgroundColorOpacity: 45,
  backgroundMediaEnabled: false, backgroundMediaAttachmentId: '', backgroundMediaUrl: '', backgroundMediaType: 'image', backgroundMediaMode: 'cover',
  backgroundMediaOpacity: 100, backgroundMediaBlur: 0, backgroundMediaBrightness: 100,
})
const form = reactive({
  title: '', kind: 'text' as 'text' | 'image' | 'iframe', contentFormat: 'plain' as 'plain' | 'tiptap', content: '',
  imageAttachmentId: '', imageUrl: '', embedUrl: '', defaultAccess: 'view' as 'none' | 'view',
  managerNoteFormat: 'plain' as 'plain' | 'tiptap', managerNote: '', presentation: defaultPresentation(),
})

const formSnapshot = computed(() => JSON.stringify({
  title: form.title, kind: form.kind, contentFormat: form.contentFormat, content: form.content,
  imageAttachmentId: form.imageAttachmentId, imageUrl: form.imageUrl, embedUrl: form.embedUrl,
  presentation: form.presentation,
  ...(props.canManage ? { defaultAccess: form.defaultAccess, managerNoteFormat: form.managerNoteFormat, managerNote: form.managerNote } : {}),
}))
const dirty = computed(() => formSnapshot.value !== savedSnapshot.value)
function isValidHTTPURL(value: string, required = false) {
  const trimmed = value.trim()
  if (!trimmed) return !required
  try {
    const parsed = new URL(trimmed)
    return !!parsed.host && (parsed.protocol === 'http:' || parsed.protocol === 'https:')
  } catch {
    return false
  }
}
function backgroundMediaNeedsSource() {
  return form.presentation.backgroundMediaEnabled
    && !form.presentation.backgroundMediaAttachmentId.trim()
    && !form.presentation.backgroundMediaUrl.trim()
}
function saveValidationMessage() {
  if (form.kind === 'iframe' && !isValidHTTPURL(form.embedUrl, true)) return '请输入有效的网页 URL'
  if (form.kind === 'image' && form.imageUrl.trim() && !isValidHTTPURL(form.imageUrl)) return '请输入有效的图片 URL'
  if (backgroundMediaNeedsSource()) return '请选择背景媒体或关闭媒体覆盖'
  return ''
}
function clearAutosaveTimer() {
  if (autosaveTimer) clearTimeout(autosaveTimer)
  autosaveTimer = null
}
function scheduleAutosave() {
  clearAutosaveTimer()
  if (!autosaveReady.value || remoteConflict.value) return
  if (dirty.value && !saving.value && pendingRemoteRevision.value > (workingClue.value?.revision || 0)) {
    enterRemoteConflict()
    return
  }
  if (!autosaveReady.value || !dirty.value || saveFailed.value || imageUploading.value || backgroundMediaUploading.value || saveValidationMessage()) return
  autosaveTimer = setTimeout(() => { void flushAutosave(false) }, 700)
}
watch(formSnapshot, scheduleAutosave, { flush: 'sync' })
watch([imageUploading, backgroundMediaUploading], scheduleAutosave)

const lockKey = (worldId: string, clueId: string) => `${worldId}:${clueId}`
function getFieldLock(field: string) {
  lockClock.value
  const clueId = workingClue.value?.id
  if (!clueId) return undefined
  return (store.editLocksByClue[lockKey(props.worldId, clueId)] || []).find(lock => lock.field === field && lock.expireAt > Date.now())
}
function ownsFieldLock(field: string) {
  const lock = getFieldLock(field)
  return ownedLockFields.value.has(field) && !!lock && lock.sessionId === lockSessionId.value
}
function isFieldEditable(field: string) {
  return !workingClue.value?.id || ownsFieldLock(field)
}
function isFieldLockedByOther(field: string) {
  const lock = getFieldLock(field)
  return !!lock && !ownsFieldLock(field)
}
function fieldLockOwnerName(field: string) {
  const lock = getFieldLock(field)
  return lock?.user?.nick || lock?.user?.name || lock?.userId || '其他用户'
}
function clearLockRenewTimer() {
  if (lockRenewTimer) clearInterval(lockRenewTimer)
  lockRenewTimer = null
}
function ensureLockRenewTimer() {
  if (lockRenewTimer || !workingClue.value?.id) return
  lockRenewTimer = setInterval(() => { void renewOwnedLocks() }, 4000)
}
async function renewOwnedLocks() {
  lockClock.value = Date.now()
  const clueId = workingClue.value?.id
  const worldId = props.worldId
  const sessionId = lockSessionId.value
  const currentSession = session
  if (!clueId || !props.show) {
    clearLockRenewTimer()
    return
  }
  if (!ownedLockFields.value.size) return
  const fields = [...ownedLockFields.value]
  await Promise.all(fields.map(async field => {
    try {
      const result = await store.acquireEditLock(worldId, clueId, field, sessionId)
      if (session !== currentSession || !props.show || props.worldId !== worldId || workingClue.value?.id !== clueId || lockSessionId.value !== sessionId) return
      if (!result.ok && result.conflict) {
        const next = new Set(ownedLockFields.value)
        next.delete(field)
        ownedLockFields.value = next
        message.warning(`${fieldLockOwnerName(field)} 正在编辑该字段`)
      }
    } catch {
      // A transient network failure leaves the lease to expire naturally.
    }
  }))
  if (session !== currentSession || !props.show || props.worldId !== worldId || workingClue.value?.id !== clueId || lockSessionId.value !== sessionId) return
}
async function beginFieldEdit(field: string) {
  const clueId = workingClue.value?.id
  if (!clueId) return true
  const pending = new Set(pendingReleaseLockFields.value)
  pending.delete(field)
  pendingReleaseLockFields.value = pending
  if (ownsFieldLock(field)) return true
  if (acquiringLockFields.value.has(field)) return false
  const worldId = props.worldId
  const sessionId = lockSessionId.value
  const currentSession = session
  acquiringLockFields.value = new Set(acquiringLockFields.value).add(field)
  let acquired = false
  try {
    const result = await store.acquireEditLock(worldId, clueId, field, sessionId)
    if (!result.ok) return false
    if (session !== currentSession || !props.show || !autosaveReady.value || props.worldId !== worldId || workingClue.value?.id !== clueId || lockSessionId.value !== sessionId) {
      await store.releaseEditLock(worldId, clueId, field, sessionId).catch(() => false)
      return false
    }
    acquired = true
    ownedLockFields.value = new Set(ownedLockFields.value).add(field)
    ensureLockRenewTimer()
    if (pendingReleaseLockFields.value.has(field)) {
      const pending = new Set(pendingReleaseLockFields.value)
      pending.delete(field)
      pendingReleaseLockFields.value = pending
      void finishFieldEdit(field)
    }
    return true
  } catch (error: any) {
    message.error(error?.response?.data?.message || '获取编辑权失败')
    return false
  } finally {
    const next = new Set(acquiringLockFields.value)
    next.delete(field)
    acquiringLockFields.value = next
    if (!acquired) {
      const pending = new Set(pendingReleaseLockFields.value)
      pending.delete(field)
      pendingReleaseLockFields.value = pending
    }
  }
}
async function releaseFieldLock(field: string, options?: { worldId?: string; clueId?: string; sessionId?: string }) {
  const worldId = options?.worldId ?? props.worldId
  const clueId = options ? options.clueId : workingClue.value?.id
  const sessionId = options?.sessionId ?? lockSessionId.value
  const owned = ownedLockFields.value.has(field)
  const next = new Set(ownedLockFields.value)
  next.delete(field)
  ownedLockFields.value = next
  if (!clueId || !owned) return
  try { await store.releaseEditLock(worldId, clueId, field, sessionId) } catch { /* best effort */ }
}
async function finishFieldEdit(field: string) {
  if (!ownedLockFields.value.has(field)) {
    if (acquiringLockFields.value.has(field)) {
      pendingReleaseLockFields.value = new Set(pendingReleaseLockFields.value).add(field)
    }
    return
  }
  const lockContext = { worldId: props.worldId, clueId: workingClue.value?.id, sessionId: lockSessionId.value }
  const editSession = session
  clearAutosaveTimer()
  try {
    await flushAutosave(false)
  } finally {
    if (session === editSession) {
      await releaseFieldLock(field, lockContext)
    } else if (lockContext.clueId) {
      await store.releaseEditLock(lockContext.worldId, lockContext.clueId, field, lockContext.sessionId).catch(() => false)
    }
  }
}
async function releaseAllFieldLocks(options?: { worldId?: string; clueId?: string; sessionId?: string }) {
  const worldId = options?.worldId ?? props.worldId
  const fields = [...ownedLockFields.value]
  const sessionId = options?.sessionId ?? lockSessionId.value
  clearLockRenewTimer()
  ownedLockFields.value = new Set()
  pendingReleaseLockFields.value = new Set()
  const clueId = options ? options.clueId : workingClue.value?.id
  if (!clueId) return
  await Promise.all(fields.map(field => store.releaseEditLock(worldId, clueId, field, sessionId).catch(() => false)))
}
function fieldInputLockClass(field: string) {
  return { 'clue-field--locked': isFieldLockedByOther(field), 'clue-field--acquiring': acquiringLockFields.value.has(field) }
}
function richOverlayActive(editor: InstanceType<typeof RichTextEditor> | null) {
  return !!editor?.hasOpenOverlay?.() || !!editor?.hasRecentOverlayInteraction?.(500)
}
function finishRichField(field: string, editor: InstanceType<typeof RichTextEditor> | null) {
  nextTick(() => {
    if (richOverlayActive(editor)) {
      window.setTimeout(() => {
        if (!richOverlayActive(editor)) void finishFieldEdit(field)
      }, 600)
      return
    }
    void finishFieldEdit(field)
  })
}
function gateRichTextPointer(event: PointerEvent, field: string) {
  if (isFieldEditable(field)) return
  event.preventDefault()
  event.stopPropagation()
  void beginFieldEdit(field).then(acquired => {
    if (acquired) nextTick(() => field === 'content' ? contentEditorRef.value?.focus() : managerEditorRef.value?.focus())
  })
}
function gateRichTextKeydown(event: KeyboardEvent, field: string) {
  if (isFieldEditable(field)) return
  event.preventDefault()
  event.stopPropagation()
  void beginFieldEdit(field).then(acquired => {
    if (acquired) nextTick(() => field === 'content' ? contentEditorRef.value?.focus() : managerEditorRef.value?.focus())
  })
}

function clearImagePreview() {
  if (imagePreviewUrl.value) URL.revokeObjectURL(imagePreviewUrl.value)
  imagePreviewUrl.value = ''
}

function clearBackgroundMediaPreview() {
  if (backgroundMediaPreviewUrl.value) URL.revokeObjectURL(backgroundMediaPreviewUrl.value)
  backgroundMediaPreviewUrl.value = ''
  selectedBackgroundMediaIsVideo.value = false
  backgroundVideoFallbackUrl.value = ''
}

function reset() {
  autosaveReady.value = false
  clearAutosaveTimer()
  session += 1
  lockSessionId.value = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  ownedLockFields.value = new Set()
  acquiringLockFields.value = new Set()
  pendingReleaseLockFields.value = new Set()
  clearLockRenewTimer()
  sessionWorldId = props.worldId
  sessionChanged = false
  saveFailed.value = false
  remoteConflict.value = false
  pendingRemoteRevision.value = 0
  clearImagePreview()
  clearBackgroundMediaPreview()
  privatePreview.value = null
  selectedImageName.value = ''
  selectedBackgroundMediaName.value = ''
  activeTab.value = 'content'
  hydrateFromClue(props.clue || null)
  if (workingClue.value?.id) {
    ensureLockRenewTimer()
    void store.loadEditLocks(props.worldId, workingClue.value.id).catch(() => undefined)
  }
}

function hydrateFromClue(clue: WorldClueDetail | null) {
  autosaveReady.value = false
  clearAutosaveTimer()
  workingClue.value = clue
  form.title = clue?.title || ''
  form.kind = clue?.kind || 'text'
  form.contentFormat = clue?.contentFormat || 'plain'
  form.content = clue?.content || ''
  form.imageAttachmentId = clue?.imageAttachmentId || ''
  form.imageUrl = clue?.imageUrl || ''
  form.embedUrl = clue?.embedUrl || ''
  form.defaultAccess = clue?.defaultAccess || 'view'
  form.managerNoteFormat = clue?.managerNoteFormat || 'plain'
  form.managerNote = clue?.managerNote || ''
  form.presentation = { ...defaultPresentation(), ...(clue?.presentation || {}) }
  rich.value = form.contentFormat === 'tiptap'
  managerRich.value = form.managerNoteFormat === 'tiptap'
  savedSnapshot.value = formSnapshot.value
  autosaveReady.value = true
}

function enterRemoteConflict() {
  const alreadyConflicted = remoteConflict.value
  remoteConflict.value = true
  saveFailed.value = false
  clearAutosaveTimer()
  if (!alreadyConflicted) message.warning('线索已被其他人更新，当前修改尚未同步')
}

async function handleRemoteRevision(revision: number) {
  const clue = workingClue.value
  if (!props.show || !autosaveReady.value || props.worldId !== sessionWorldId || !clue || revision <= clue.revision || remoteConflict.value) return
  pendingRemoteRevision.value = Math.max(pendingRemoteRevision.value, revision)
  if (saving.value) return
  if (dirty.value || imageUploading.value || backgroundMediaUploading.value) {
    enterRemoteConflict()
    return
  }
  const currentSession = session
  const worldId = props.worldId
  const isCurrent = () => session === currentSession && props.show && autosaveReady.value && props.worldId === worldId && workingClue.value?.id === clue.id
  try {
    const latest = await store.fetchDetail(worldId, clue.id, false)
    if (!isCurrent() || remoteConflict.value || latest.revision <= workingClue.value!.revision) return
    if (saving.value) {
      pendingRemoteRevision.value = Math.max(pendingRemoteRevision.value, latest.revision)
      return
    }
    if (dirty.value || imageUploading.value || backgroundMediaUploading.value) {
      enterRemoteConflict()
      return
    }
    // A slower fetch must not replace a newer revision announced meanwhile.
    if (latest.revision < pendingRemoteRevision.value) return
    pendingRemoteRevision.value = 0
    clearImagePreview()
    clearBackgroundMediaPreview()
    selectedImageName.value = ''
    selectedBackgroundMediaName.value = ''
    hydrateFromClue(latest)
  } catch {
    // Keep the known remote revision so subsequent edits cannot overwrite it.
  }
}

type ClueRevisionPayload = { worldId?: string; clueId?: string; action?: string; revision?: number }
function handleClueChanged(event: { worldClue?: ClueRevisionPayload; argv?: { options?: ClueRevisionPayload; Options?: ClueRevisionPayload } }) {
	const payload = event?.worldClue || event?.argv?.options || event?.argv?.Options
	if (payload?.worldId !== props.worldId || payload?.clueId !== workingClue.value?.id) return
	if (payload.action === 'edit-lock') {
		void store.loadEditLocks(props.worldId, workingClue.value.id).catch(() => undefined)
		return
	}
	if (!(Number(payload?.revision) > 0)) return
	void handleRemoteRevision(Number(payload.revision))
}

function handleVisibilityChange() {
  if (!props.show) return
  if (document.visibilityState === 'hidden') {
    void releaseAllFieldLocks()
  } else if (workingClue.value?.id) {
    ensureLockRenewTimer()
  }
}
onMounted(() => {
  chatEvent.on('world-clue-changed' as any, handleClueChanged)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

watch(() => props.show, value => {
  if (value) {
    reset()
  } else {
    autosaveReady.value = false
    clearAutosaveTimer()
    const lockContext = { worldId: props.worldId, clueId: workingClue.value?.id, sessionId: lockSessionId.value }
    void (async () => {
      if (distributionPanel.value) await distributionPanel.value.closePrivate()
      await releaseAllFieldLocks(lockContext)
    })().catch(() => undefined)
    clearImagePreview()
    clearBackgroundMediaPreview()
  }
}, { immediate: true })
onBeforeUnmount(() => {
  chatEvent.off('world-clue-changed' as any, handleClueChanged)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  autosaveReady.value = false
  session += 1
  clearAutosaveTimer()
  clearLockRenewTimer()
  void releaseAllFieldLocks()
  clearImagePreview()
  clearBackgroundMediaPreview()
})
function setRich(value: boolean) {
  rich.value = value
  form.contentFormat = value ? 'tiptap' : 'plain'
  form.content = value ? emptyTiptapDocument : ''
}
function setManagerRich(value: boolean) {
  managerRich.value = value
  form.managerNoteFormat = value ? 'tiptap' : 'plain'
  form.managerNote = value ? emptyTiptapDocument : ''
}

const preview = computed<WorldClueDetail>(() => ({
  ...(workingClue.value || { id: '', worldId: props.worldId, effectiveAccess: 'edit', status: 'draft', revision: 1, publishSeq: 0, orderIndex: 0, hasPrivateContent: false, unread: false, creatorId: '', updatedAt: Date.now() }),
  title: form.title || '未命名线索', kind: form.kind, contentFormat: form.contentFormat,
  content: form.contentFormat === 'tiptap' && !form.content.trim() ? emptyTiptapDocument : form.content,
  imageAttachmentId: imagePreviewUrl.value ? '' : form.imageAttachmentId,
  imageUrl: imagePreviewUrl.value || form.imageUrl,
  embedUrl: form.embedUrl, presentation: form.presentation,
  privateContentFormat: privatePreview.value?.format ?? workingClue.value?.privateContentFormat,
  privateContent: privatePreview.value ? privatePreview.value.content : (workingClue.value?.privateContent || ''),
}))

const backgroundPreviewUrl = computed(() => {
  if (backgroundMediaPreviewUrl.value) return backgroundMediaPreviewUrl.value
  if (form.presentation.backgroundMediaAttachmentId && workingClue.value?.id) {
    return `${urlBase}/api/v1/worlds/${encodeURIComponent(props.worldId)}/clues/${encodeURIComponent(workingClue.value.id)}/background-media?v=${encodeURIComponent(form.presentation.backgroundMediaAttachmentId)}`
  }
  return form.presentation.backgroundMediaUrl.trim()
})
const backgroundPreviewIsVideo = computed(() => {
  if (selectedBackgroundMediaIsVideo.value || form.presentation.backgroundMediaType === 'video') return true
  if (backgroundVideoFallbackUrl.value === backgroundPreviewUrl.value) return true
  try {
    return /\.(mp4|webm|ogg|ogv|mov|m4v)$/i.test(new URL(backgroundPreviewUrl.value).pathname)
  } catch {
    return false
  }
})
const backgroundPreviewIsTiled = computed(() => form.presentation.backgroundMediaMode === 'tile' && !backgroundPreviewIsVideo.value)
const backgroundPreviewFit = computed(() => {
  if (form.presentation.backgroundMediaMode === 'center') return backgroundPreviewIsVideo.value ? 'contain' : 'none'
  if (form.presentation.backgroundMediaMode === 'tile') return 'cover'
  return form.presentation.backgroundMediaMode
})
const backgroundPreviewMediaStyle = computed(() => ({
  opacity: form.presentation.backgroundMediaOpacity / 100,
  filter: `blur(${form.presentation.backgroundMediaBlur}px) brightness(${form.presentation.backgroundMediaBrightness}%)`,
  inset: `${-form.presentation.backgroundMediaBlur * 2}px`,
}))
const backgroundPreviewTileStyle = computed(() => ({ backgroundImage: `url(${JSON.stringify(backgroundPreviewUrl.value)})` }))
const backgroundPreviewColorStyle = computed(() => ({
  backgroundColor: form.presentation.backgroundColor,
  opacity: form.presentation.backgroundColorOpacity / 100,
}))
watch(backgroundPreviewUrl, () => { backgroundVideoFallbackUrl.value = '' })

function useBackgroundPreviewVideo() {
  backgroundVideoFallbackUrl.value = backgroundPreviewUrl.value
}

async function uploadImage(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  const previousMediaType = form.presentation.mediaType
  clearImagePreview()
  imagePreviewUrl.value = URL.createObjectURL(file)
  selectedImageName.value = file.name
  form.presentation.mediaType = file.type.startsWith('video/') ? 'video' : 'image'
  imageUploading.value = true
  try {
    const result = await uploadImageAttachment(file, { channelId: props.channelId, rootId: props.worldId, rootIdType: 'world_clue', skipCompression: true })
    form.imageAttachmentId = result.attachmentId.replace(/^id:/, '')
    form.imageUrl = ''
  } catch (error: any) {
    clearImagePreview()
    selectedImageName.value = ''
    form.presentation.mediaType = previousMediaType
    message.error(error?.message || '图片上传失败')
  }
  finally { imageUploading.value = false; (event.target as HTMLInputElement).value = '' }
}

async function uploadBackgroundMedia(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  clearBackgroundMediaPreview()
  backgroundMediaPreviewUrl.value = URL.createObjectURL(file)
  selectedBackgroundMediaName.value = file.name
  selectedBackgroundMediaIsVideo.value = file.type.startsWith('video/')
  backgroundMediaUploading.value = true
  try {
    const result = await uploadImageAttachment(file, {
      channelId: props.channelId,
      rootId: props.worldId,
      rootIdType: 'world_clue',
      skipCompression: true,
    })
    form.presentation.backgroundMediaAttachmentId = result.attachmentId.replace(/^id:/, '')
    form.presentation.backgroundMediaUrl = ''
    form.presentation.backgroundMediaType = selectedBackgroundMediaIsVideo.value ? 'video' : 'image'
    form.presentation.backgroundMediaEnabled = true
  } catch (error: any) {
    clearBackgroundMediaPreview()
    selectedBackgroundMediaName.value = ''
    message.error(error?.message || '背景媒体上传失败')
  } finally {
    backgroundMediaUploading.value = false
    input.value = ''
  }
}

function clearBackgroundMedia() {
  clearBackgroundMediaPreview()
  selectedBackgroundMediaName.value = ''
  form.presentation.backgroundMediaAttachmentId = ''
  form.presentation.backgroundMediaUrl = ''
  form.presentation.backgroundMediaEnabled = false
  if (backgroundMediaFileInput.value) backgroundMediaFileInput.value.value = ''
}

function useExternalImage() {
  form.imageAttachmentId = ''
  selectedImageName.value = ''
  clearImagePreview()
}

async function flushAutosave(manual = true): Promise<boolean> {
  // Field edit locks prevent same-field concurrent editing.
  // Revision conflict handling remains the fallback for cross-field concurrent updates.
  clearAutosaveTimer()
  if (remoteConflict.value) return false
  if (saveFlight) {
    const saved = await saveFlight
    if (!saved && !remoteConflict.value && manual && !form.title.trim()) message.warning('请输入标题')
    return saved && !remoteConflict.value
  }
  if (!autosaveReady.value || props.worldId !== sessionWorldId) return false
  if (imageUploading.value || backgroundMediaUploading.value) {
    if (manual) message.warning('请等待媒体上传完成')
    return false
  }
  if (!dirty.value) return true
  const validationMessage = saveValidationMessage()
  if (validationMessage) {
    if (manual) message.warning(validationMessage)
    return false
  }
  if (!form.title.trim()) {
    if (manual) message.warning('请输入标题')
    return false
  }
  if (!manual && saveFailed.value) return false
  const currentSession = session
  saving.value = true
  saveFailed.value = false
  saveFlight = (async () => {
    try {
      while (dirty.value && !remoteConflict.value && autosaveReady.value && session === currentSession && props.worldId === sessionWorldId) {
        if (imageUploading.value || backgroundMediaUploading.value) return false
        const validationMessage = saveValidationMessage()
        if (validationMessage) {
          if (manual) message.warning(validationMessage)
          return false
        }
        if (!form.title.trim()) { if (manual) message.warning('请输入标题'); return false }
        const snapshot = formSnapshot.value
        const payload = JSON.parse(snapshot)
        if (payload.contentFormat === 'tiptap' && !payload.content.trim()) payload.content = emptyTiptapDocument
        if (payload.managerNoteFormat === 'tiptap' && !payload.managerNote.trim()) payload.managerNote = emptyTiptapDocument
        const saved = await store.saveClue(sessionWorldId, workingClue.value?.id || null, {
          ...payload,
          ...(workingClue.value ? { expectedRevision: workingClue.value.revision } : {}),
        })
        if (session !== currentSession || !autosaveReady.value || props.worldId !== sessionWorldId) return false
        const clueWasCreated = !workingClue.value?.id && !!saved.id
        workingClue.value = saved
        if (clueWasCreated) ensureLockRenewTimer()
        savedSnapshot.value = snapshot
        sessionChanged = true
        if (clueWasCreated) void store.loadEditLocks(sessionWorldId, saved.id).catch(() => undefined)
        if (pendingRemoteRevision.value > saved.revision) break
        pendingRemoteRevision.value = 0
      }
      return !dirty.value
    } catch (error: any) {
      if (session !== currentSession || !autosaveReady.value || props.worldId !== sessionWorldId) return false
      if (error?.response?.status === 409) {
        enterRemoteConflict()
        return false
      }
      saveFailed.value = true
      clearAutosaveTimer()
      message.error(error?.response?.data?.message || '保存失败，请刷新后重试')
      return false
    } finally {
      saving.value = false
      if (session === currentSession && pendingRemoteRevision.value > 0) {
        await handleRemoteRevision(pendingRemoteRevision.value)
      }
      saveFlight = null
    }
  })()
  const saved = await saveFlight
  return saved && !remoteConflict.value
}

async function requestClose() {
  if (closing.value || publishing.value) return
  closing.value = true
  try {
    if (!remoteConflict.value && !await flushAutosave() && !remoteConflict.value) return
    if (distributionPanel.value && !await distributionPanel.value.flushPrivate()) return
    if (distributionPanel.value) await distributionPanel.value.closePrivate()
    await releaseAllFieldLocks()
    if (!remoteConflict.value && workingClue.value && sessionChanged) emit('saved', workingClue.value)
    emit('update:show', false)
  } finally { closing.value = false }
}

async function selectTab(tab: typeof activeTab.value) {
  if (closing.value || publishing.value) return
  if (!remoteConflict.value && !await flushAutosave()) return
  if (distributionPanel.value && !await distributionPanel.value.flushPrivate()) return
  if (distributionPanel.value) await distributionPanel.value.closePrivate()
  await releaseAllFieldLocks()
  activeTab.value = tab
  if (tab === 'content' && workingClue.value?.id) ensureLockRenewTimer()
}

async function publishOrPresent() {
  if (remoteConflict.value) { message.warning('线索已有新版本，请关闭后重新打开再操作'); return }
  if (publishing.value || closing.value) return
  publishing.value = true
  try {
    if (!await flushAutosave()) return
    if (distributionPanel.value && !await distributionPanel.value.flushPrivate()) return
    if (remoteConflict.value) { message.warning('线索已有新版本，请关闭后重新打开再操作'); return }
    if (!workingClue.value) return
    const wasPublished = workingClue.value.status === 'published'
    const saved = await store.publish(sessionWorldId, workingClue.value.id, workingClue.value.publishSeq)
    workingClue.value = saved
    await releaseAllFieldLocks()
    emit('saved', saved)
    emit('update:show', false)
    message.success(wasPublished ? '已再次揭示' : '已揭示')
  } catch (error: any) { message.error(error?.response?.data?.message || '揭示失败') }
  finally { publishing.value = false }
}

async function unpublish() {
  if (remoteConflict.value) { message.warning('线索已有新版本，请关闭后重新打开再操作'); return }
  if (publishing.value || closing.value) return
  publishing.value = true
  try {
    if (!await flushAutosave()) return
    if (distributionPanel.value && !await distributionPanel.value.flushPrivate()) return
    if (remoteConflict.value) { message.warning('线索已有新版本，请关闭后重新打开再操作'); return }
    if (!workingClue.value) return
    const saved = await store.unpublish(sessionWorldId, workingClue.value.id, workingClue.value.publishSeq)
    workingClue.value = saved
    await releaseAllFieldLocks()
    emit('saved', saved)
    emit('update:show', false)
    message.success('已收回，恢复为未揭示状态')
  } catch (error: any) { message.error(error?.response?.data?.message || '收回失败') }
  finally { publishing.value = false }
}

function handleDistributionRevealed(saved: WorldClueDetail) {
  if (!workingClue.value || workingClue.value.worldId !== props.worldId || workingClue.value.id !== saved.id) return
  workingClue.value = { ...workingClue.value, ...saved }
  sessionChanged = true
}
</script>

<template>
  <NModal :show="show" :mask-closable="false" @update:show="value => { if (!value) void requestClose() }">
    <div class="clue-editor-shell">
      <header class="clue-editor__header">
        <strong>线索编辑</strong>
        <div class="clue-editor__header-actions">
          <small>{{ remoteConflict ? '已有新版本' : saving ? '保存中' : saveFailed ? '保存失败' : dirty ? '未保存' : workingClue ? '已保存' : '' }}</small>
          <NButton circle quaternary size="small" title="保存" aria-label="保存" :type="dirty ? 'primary' : 'default'" :loading="saving" :disabled="publishing || closing || remoteConflict" @click="flushAutosave()"><template #icon><NIcon><DeviceFloppy /></NIcon></template></NButton>
          <NButton circle quaternary size="small" title="关闭" aria-label="关闭" :disabled="publishing || closing" @click="requestClose">
            <template #icon><NIcon><X /></NIcon></template>
          </NButton>
        </div>
      </header>

      <nav class="clue-editor__tabs" role="tablist" aria-label="线索编辑页面">
        <button type="button" role="tab" :aria-selected="activeTab === 'content'" :class="{ 'is-active': activeTab === 'content' }" @click="selectTab('content')">内容与展示</button>
        <button v-if="canManage" type="button" role="tab" :aria-selected="activeTab === 'distribution'" :class="{ 'is-active': activeTab === 'distribution' }" @click="selectTab('distribution')">分发</button>
        <button v-if="canManage" type="button" role="tab" :aria-selected="activeTab === 'manager'" :class="{ 'is-active': activeTab === 'manager' }" @click="selectTab('manager')">管理备注</button>
      </nav>

      <main class="clue-editor__body" :inert="closing || publishing">
        <div v-if="activeTab === 'content'" class="clue-editor__workspace">
          <section class="clue-editor__panel clue-editor__edit-panel">
            <NForm class="clue-editor__form" label-placement="top">
              <div class="clue-editor__setting-grid clue-editor__setting-grid--title">
                <NFormItem label="标题" :class="fieldInputLockClass('title')">
                  <NInput v-model:value="form.title" maxlength="255" :readonly="!isFieldEditable('title')" @focus="beginFieldEdit('title')" @blur="finishFieldEdit('title')" />
                  <small v-if="isFieldLockedByOther('title')" class="clue-field-lock">🔒 {{ fieldLockOwnerName('title') }} 正在编辑</small>
                  <small v-else-if="acquiringLockFields.has('title')" class="clue-field-lock clue-field-lock--pending">正在获取编辑权…</small>
                </NFormItem>
                <NFormItem label="标题颜色"><NColorPicker v-model:value="form.presentation.titleColor" :show-alpha="false" /></NFormItem>
              </div>
              <div class="clue-editor__setting-grid clue-editor__setting-grid--two">
                <NFormItem label="类型"><NSelect v-model:value="form.kind" :options="[{label:'文本',value:'text'},{label:'图片',value:'image'},{label:'交互网页',value:'iframe'}]" /></NFormItem>
                <NFormItem label="富文本"><NSwitch :value="rich" @update:value="setRich" /></NFormItem>
              </div>
              <NFormItem class="clue-editor__field--full" label="正文" :class="fieldInputLockClass('content')">
                <div v-if="rich" class="clue-rich-lock-gate" @pointerdown.capture="gateRichTextPointer($event, 'content')" @keydown.capture="gateRichTextKeydown($event, 'content')">
                  <RichTextEditor ref="contentEditorRef" v-model="form.content" :maxlength="50000" min-height="320px" @focus="beginFieldEdit('content')" @blur="finishRichField('content', contentEditorRef)" />
                </div>
                <NInput v-else v-model:value="form.content" type="textarea" :autosize="{ minRows: 8, maxRows: 18 }" :readonly="!isFieldEditable('content')" @focus="beginFieldEdit('content')" @blur="finishFieldEdit('content')" />
                <small v-if="isFieldLockedByOther('content')" class="clue-field-lock">🔒 {{ fieldLockOwnerName('content') }} 正在编辑</small>
                <small v-else-if="acquiringLockFields.has('content')" class="clue-field-lock clue-field-lock--pending">正在获取编辑权…</small>
              </NFormItem>
              <NFormItem v-if="form.kind === 'iframe'" class="clue-editor__field--full" label="网页 URL" :class="fieldInputLockClass('embedUrl')">
                <NInput v-model:value="form.embedUrl" placeholder="https://" :readonly="!isFieldEditable('embedUrl')" @focus="beginFieldEdit('embedUrl')" @blur="finishFieldEdit('embedUrl')" />
                <small v-if="isFieldLockedByOther('embedUrl')" class="clue-field-lock">🔒 {{ fieldLockOwnerName('embedUrl') }} 正在编辑</small>
              </NFormItem>
              <details v-if="form.kind === 'image'" class="clue-editor__options">
                <summary>图片附件</summary>
                <div class="clue-editor__setting-grid">
                  <NFormItem class="clue-editor__field--full" label="SealChat 媒体附件">
                    <div class="clue-editor__upload-control">
                      <NButton secondary :loading="imageUploading" :disabled="imageUploading" @click="imageFileInput?.click()">选择媒体</NButton>
                      <span>{{ selectedImageName || (form.imageAttachmentId ? '已关联媒体附件' : '未选择媒体') }}</span>
                      <input ref="imageFileInput" type="file" accept="image/*,video/webm,.webm" :disabled="imageUploading" @change="uploadImage" />
                    </div>
                  </NFormItem>
                  <NFormItem class="clue-editor__field--full" label="或外部图片 URL" :class="fieldInputLockClass('imageUrl')">
                    <NInput v-model:value="form.imageUrl" placeholder="https://" :readonly="!isFieldEditable('imageUrl')" @focus="beginFieldEdit('imageUrl')" @blur="finishFieldEdit('imageUrl')" @update:value="useExternalImage" />
                    <small v-if="isFieldLockedByOther('imageUrl')" class="clue-field-lock">🔒 {{ fieldLockOwnerName('imageUrl') }} 正在编辑</small>
                  </NFormItem>
                </div>
              </details>
              <details class="clue-editor__options">
                <summary>媒体设置</summary>
                <div class="clue-editor__setting-grid clue-editor__setting-grid--three">
                  <NFormItem label="媒体位置"><NSelect v-model:value="form.presentation.mediaPlacement" :options="mediaPlacementOptions" /></NFormItem>
                  <NFormItem label="媒体适应"><NSelect v-model:value="form.presentation.objectFit" :options="objectFitOptions" /></NFormItem>
                  <NFormItem label="媒体比例"><NInputNumber v-model:value="form.presentation.mediaRatio" :min=".2" :max=".8" :step=".05" /></NFormItem>
                </div>
              </details>
              <details class="clue-editor__options">
                <summary>动画设置</summary>
                <div class="clue-editor__setting-grid clue-editor__setting-grid--three">
                  <NFormItem label="进入动画"><NSelect v-model:value="form.presentation.enterAnimation" :options="enterAnimationOptions" /></NFormItem>
                  <NFormItem label="退出动画"><NSelect v-model:value="form.presentation.exitAnimation" :options="exitAnimationOptions" /></NFormItem>
                  <NFormItem label="时长 (ms)"><NInputNumber v-model:value="form.presentation.animationDurationMs" :min="0" :max="1000" /></NFormItem>
                </div>
              </details>
              <details class="clue-editor__options">
                <summary>背景设置</summary>
                <div class="clue-editor__setting-grid clue-editor__setting-grid--three">
                  <NFormItem label="媒体覆盖"><NSwitch v-model:value="form.presentation.backgroundMediaEnabled" /></NFormItem>
                  <template v-if="form.presentation.backgroundMediaEnabled">
                    <NFormItem class="clue-editor__field--full" label="背景媒体">
                      <div class="clue-editor__upload-control">
                        <NButton secondary :loading="backgroundMediaUploading" :disabled="backgroundMediaUploading" @click="backgroundMediaFileInput?.click()">选择文件</NButton>
                        <span>{{ selectedBackgroundMediaName || (form.presentation.backgroundMediaAttachmentId || form.presentation.backgroundMediaUrl ? '已关联背景媒体' : '未选择文件') }}</span>
                        <NButton v-if="backgroundMediaPreviewUrl || form.presentation.backgroundMediaAttachmentId || form.presentation.backgroundMediaUrl" quaternary size="small" :disabled="backgroundMediaUploading" @click="clearBackgroundMedia">移除</NButton>
                        <input ref="backgroundMediaFileInput" type="file" accept="image/*,video/mp4,video/webm,video/ogg,video/quicktime,video/x-m4v,.ogv,.m4v" :disabled="backgroundMediaUploading" @change="uploadBackgroundMedia" />
                      </div>
                    </NFormItem>
                    <NFormItem label="展示模式"><NSelect v-model:value="form.presentation.backgroundMediaMode" :options="backgroundMediaModeOptions" /></NFormItem>
                    <NFormItem :label="`透明度 ${form.presentation.backgroundMediaOpacity}%`"><NSlider v-model:value="form.presentation.backgroundMediaOpacity" :min="0" :max="100" :step="1" /></NFormItem>
                    <NFormItem :label="`模糊 ${form.presentation.backgroundMediaBlur}px`"><NSlider v-model:value="form.presentation.backgroundMediaBlur" :min="0" :max="20" :step="1" /></NFormItem>
                    <NFormItem :label="`亮度 ${form.presentation.backgroundMediaBrightness}%`"><NSlider v-model:value="form.presentation.backgroundMediaBrightness" :min="50" :max="150" :step="1" /></NFormItem>
                  </template>
                  <NFormItem class="clue-editor__field--full" label="纯色覆盖"><NSwitch v-model:value="form.presentation.backgroundColorEnabled" /></NFormItem>
                  <template v-if="form.presentation.backgroundColorEnabled">
                    <NFormItem label="颜色"><NColorPicker v-model:value="form.presentation.backgroundColor" :show-alpha="false" /></NFormItem>
                    <NFormItem :label="`透明度 ${form.presentation.backgroundColorOpacity}%`"><NSlider v-model:value="form.presentation.backgroundColorOpacity" :min="0" :max="100" :step="1" /></NFormItem>
                  </template>
                </div>
              </details>
            </NForm>
          </section>
          <aside class="clue-editor__panel clue-editor__preview-panel">
            <div class="clue-editor__section-title">实时预览</div>
            <div class="clue-editor__preview">
              <div v-if="form.presentation.backgroundMediaEnabled && backgroundPreviewUrl" class="clue-editor__preview-background" :style="backgroundPreviewMediaStyle" aria-hidden="true">
                <div v-if="backgroundPreviewIsTiled" class="clue-editor__preview-background-tile" :style="backgroundPreviewTileStyle" />
                <video v-else-if="backgroundPreviewIsVideo" :src="backgroundPreviewUrl" :style="{ objectFit: backgroundPreviewFit }" autoplay muted loop playsinline />
                <img v-else :src="backgroundPreviewUrl" :style="{ objectFit: backgroundPreviewFit }" alt="" @error="useBackgroundPreviewVideo" />
              </div>
              <div v-if="form.presentation.backgroundColorEnabled" class="clue-editor__preview-background-color" :style="backgroundPreviewColorStyle" aria-hidden="true" />
              <div class="clue-editor__preview-content">
                <header class="clue-editor__preview-title">
                  <span>线索</span>
                  <h2 :style="{ color: form.presentation.titleColor }">{{ preview.title }}</h2>
                </header>
                <WorldClueContentView
                  :clue="preview"
                  :private-label="privatePreview ? `仅 ${privatePreview.name} 可见` : '仅你可见'"
                />
              </div>
            </div>
          </aside>
        </div>

        <WorldClueDistributionPanel v-else-if="activeTab === 'distribution' && canManage" ref="distributionPanel" :world-id="worldId" :clue="workingClue" :default-access="form.defaultAccess" :publishing="publishing" :lock-session-id="lockSessionId" @update:default-access="form.defaultAccess = $event" @private-preview="privatePreview = $event" @publish="publishOrPresent" @unpublish="unpublish" @revealed="handleDistributionRevealed" />

        <section v-else-if="activeTab === 'manager' && canManage" class="clue-editor__panel clue-editor__section-panel clue-editor__manager-panel" :class="fieldInputLockClass('managerNote')">
          <div class="clue-editor__section-title">管理备注</div>
          <div class="clue-editor__editor-toolbar">
            <strong>备注内容</strong>
            <div class="clue-editor__format-toggle">
              <span>内容格式</span>
              <NSwitch :value="managerRich" @update:value="setManagerRich"><template #checked>富文本</template><template #unchecked>纯文本</template></NSwitch>
            </div>
          </div>
          <div v-if="managerRich" class="clue-rich-lock-gate" @pointerdown.capture="gateRichTextPointer($event, 'managerNote')" @keydown.capture="gateRichTextKeydown($event, 'managerNote')">
            <RichTextEditor ref="managerEditorRef" v-model="form.managerNote" :maxlength="50000" min-height="360px" @focus="beginFieldEdit('managerNote')" @blur="finishRichField('managerNote', managerEditorRef)" />
          </div>
          <NInput v-else v-model:value="form.managerNote" type="textarea" :autosize="{ minRows: 12 }" :readonly="!isFieldEditable('managerNote')" @focus="beginFieldEdit('managerNote')" @blur="finishFieldEdit('managerNote')" />
          <small v-if="isFieldLockedByOther('managerNote')" class="clue-field-lock">🔒 {{ fieldLockOwnerName('managerNote') }} 正在编辑</small>
        </section>
      </main>
    </div>
  </NModal>
</template>

<style scoped>
.clue-field-lock { display: block; flex: 0 0 100%; width: 100%; box-sizing: border-box; margin-top: 5px; color: var(--n-warning-color); font-size: 11px; }
.clue-field-lock--pending { color: var(--sc-text-secondary); }
.clue-field--locked :deep(.n-form-item-blank), .clue-field--acquiring :deep(.n-form-item-blank) { flex-direction: column; align-items: stretch; }
.clue-rich-lock-gate { position: relative; width: 100%; }
.clue-field--locked :deep(.n-input), .clue-field--locked :deep(.rich-text-editor) { border-color: color-mix(in srgb, var(--n-warning-color) 45%, var(--sc-border-mute)); }
.clue-editor-shell { display: flex; width: min(1180px, calc(100vw - 48px)); height: min(820px, calc(100dvh - 48px)); max-height: min(820px, calc(100dvh - 48px)); flex-direction: column; overflow: hidden; color: var(--sc-text-primary); border: 1px solid var(--sc-border-strong); border-radius: 8px; background: color-mix(in srgb, var(--sc-bg-surface) 96%, transparent); box-shadow: 0 18px 50px #0004; backdrop-filter: blur(14px); }
.clue-editor__header { display: flex; flex: none; align-items: center; justify-content: space-between; min-height: 56px; padding: 0 14px 0 20px; border-bottom: 1px solid var(--sc-border-mute); }
.clue-editor__header strong { font-size: 16px; }
.clue-editor__header-actions { display: flex; align-items: center; gap: 6px; }
.clue-editor__header-actions small { color: var(--sc-text-secondary); font-size: 11px; }
.clue-editor__tabs { display: flex; flex: none; gap: 4px; padding: 8px 16px; border-bottom: 1px solid var(--sc-border-mute); }
.clue-editor__tabs button { min-height: 34px; padding: 6px 12px; color: var(--sc-text-secondary); font: inherit; border: 1px solid transparent; border-radius: 5px; background: transparent; cursor: pointer; }
.clue-editor__tabs button:hover { color: var(--sc-text-primary); background: color-mix(in srgb, var(--primary-color, #3388de) 8%, transparent); }
.clue-editor__tabs button.is-active { color: var(--primary-color, #3388de); border-color: color-mix(in srgb, var(--primary-color, #3388de) 38%, transparent); background: color-mix(in srgb, var(--primary-color, #3388de) 12%, transparent); }
.clue-editor__body { min-width: 0; min-height: 0; flex: 1; overflow: auto; padding: 20px; }
.clue-editor__workspace { display: grid; grid-template-columns: minmax(0, 11fr) minmax(320px, 9fr); align-items: start; gap: 20px; }
.clue-editor__panel { min-width: 0; border: 1px solid var(--sc-border-mute); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-elevated) 92%, transparent); }
.clue-editor__edit-panel, .clue-editor__section-panel { padding: 18px; }
.clue-editor__form { display: grid; min-width: 0; gap: 4px; }
.clue-editor__field--full { min-width: 0; }
.clue-editor__setting-grid { display: grid; min-width: 0; gap: 12px; }
.clue-editor__setting-grid--two { grid-template-columns: minmax(0, 1fr) minmax(120px, .45fr); }
.clue-editor__setting-grid--title { grid-template-columns: minmax(0, 1fr) minmax(150px, .34fr); }
.clue-editor__setting-grid--three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.clue-editor__setting-grid :deep(.n-form-item) { min-width: 0; }
.clue-editor__setting-grid .clue-editor__field--full { grid-column: 1 / -1; }
.clue-editor__setting-grid :deep(.n-form-item-label) { white-space: nowrap; }
.clue-editor__setting-grid :deep(.n-input-number) { width: 100%; }
.clue-editor__upload-control { display: flex; width: 100%; min-width: 0; align-items: center; gap: 10px; padding: 10px 12px; border: 1px dashed var(--sc-border-mute); border-radius: 5px; background: color-mix(in srgb, var(--primary-color, #3388de) 3%, transparent); }
.clue-editor__upload-control span { min-width: 0; overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.clue-editor__upload-control input { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.clue-editor__options { min-width: 0; margin-bottom: 12px; border: 1px solid var(--sc-border-mute); border-radius: 5px; background: color-mix(in srgb, var(--sc-bg-elevated) 88%, transparent); }
.clue-editor__options summary { padding: 9px 12px; color: var(--sc-text-secondary); font-size: 12px; font-weight: 600; cursor: pointer; user-select: none; }
.clue-editor__options[open] summary { color: var(--sc-text-primary); border-bottom: 1px solid var(--sc-border-mute); }
.clue-editor__options .clue-editor__setting-grid { padding: 12px 12px 0; }
.clue-editor__preview-panel { position: sticky; top: 0; overflow: hidden; }
.clue-editor__section-title { padding: 12px 14px; color: var(--sc-text-secondary); font-size: 12px; font-weight: 600; border-bottom: 1px solid var(--sc-border-mute); }
.clue-editor__section-panel > .clue-editor__section-title { margin: -18px -18px 18px; }
.clue-editor__section-panel { box-sizing: border-box; width: min(980px, 100%); margin: 0 auto; }
.clue-editor__preview { position: relative; min-height: 320px; overflow: hidden; padding: 18px; }
.clue-editor__preview-background { position: absolute; z-index: 0; overflow: hidden; pointer-events: none; }
.clue-editor__preview-background > img, .clue-editor__preview-background > video, .clue-editor__preview-background-tile { display: block; width: 100%; height: 100%; pointer-events: none; }
.clue-editor__preview-background > img, .clue-editor__preview-background > video { object-position: center; }
.clue-editor__preview-background-tile { background-position: center; background-repeat: repeat; }
.clue-editor__preview-background-color { position: absolute; inset: 0; z-index: 1; pointer-events: none; }
.clue-editor__preview-content { position: relative; z-index: 2; }
.clue-editor__preview-title { min-width: 0; margin-bottom: 16px; }
.clue-editor__preview-title span { color: rgb(255 255 255 / 72%); font-size: 10px; font-weight: 600; text-shadow: 0 1px 10px #000c; }
.clue-editor__preview-title h2 { margin: 5px 0 0; font-family: Georgia, 'Times New Roman', serif; font-size: clamp(24px, 2.8vw, 38px); font-weight: 500; letter-spacing: 0; line-height: 1.2; overflow-wrap: anywhere; text-shadow: 0 2px 18px #000b; }
.clue-editor__editor-toolbar { display: flex; min-height: 34px; align-items: center; justify-content: space-between; gap: 16px; padding-bottom: 10px; border-bottom: 1px solid var(--sc-border-mute); }
.clue-editor__editor-toolbar strong { font-size: 13px; }
.clue-editor__format-toggle { display: flex; flex: none; align-items: center; gap: 9px; color: var(--sc-text-secondary); font-size: 12px; }
.clue-editor__editor-actions { display: flex; justify-content: flex-end; }
.clue-editor__manager-panel { display: flex; flex-direction: column; gap: 14px; }
.clue-editor-shell :deep(.n-input), .clue-editor-shell :deep(.n-base-selection) { --n-box-shadow-hover: none !important; --n-box-shadow-focus: none !important; --n-box-shadow-active: none !important; --n-box-shadow-hover-warning: none !important; --n-box-shadow-focus-warning: none !important; --n-box-shadow-active-warning: none !important; --n-box-shadow-hover-error: none !important; --n-box-shadow-focus-error: none !important; --n-box-shadow-active-error: none !important; }
@media (max-width: 900px) { .clue-editor__workspace { grid-template-columns: 1fr; } .clue-editor__preview-panel { position: static; } .clue-editor__setting-grid--three { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 760px) { .clue-editor-shell { width: 100vw; height: 100dvh; max-height: 100dvh; border-width: 0; border-radius: 0; } .clue-editor__header { min-height: 52px; padding-left: 14px; } .clue-editor__tabs { flex-wrap: wrap; gap: 3px; padding: 6px 10px; } .clue-editor__tabs button { min-height: 32px; padding: 5px 8px; font-size: 12px; } .clue-editor__body { padding: 12px; overflow-x: hidden; } .clue-editor__edit-panel, .clue-editor__section-panel { padding: 12px; } .clue-editor__section-panel > .clue-editor__section-title { margin: -12px -12px 14px; } .clue-editor__setting-grid--two, .clue-editor__setting-grid--three, .clue-editor__setting-grid--title { grid-template-columns: 1fr; } .clue-editor__editor-toolbar { align-items: flex-start; } .clue-editor__format-toggle { flex-direction: column; align-items: flex-end; gap: 4px; } .clue-editor__preview { min-height: 220px; padding: 12px; } }
</style>
