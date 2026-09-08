<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { NBadge, NButton, NButtonGroup, NEmpty, NIcon, NInput, NSpace, useDialog, useMessage } from 'naive-ui'
import { ChevronLeft, Copy, Edit, ExternalLink, FileText, Folder, GridDots, GripVertical, List, MessagePlus, Photo, Pin, Pinned, Plus, Presentation, Search, Star, Trash, World, X } from '@vicons/tabler'
import { api, urlBase } from '@/stores/_config'
import { chatEvent } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { useWorldClueStore, type WorldClueDetail, type WorldClueSummary, type WorldClueUserState } from '@/stores/worldClue'
import { generateWorldClueEmbedLink } from '@/utils/worldClueEmbedLink'
import { copyTextWithFallback } from '@/utils/clipboard'
import WorldClueEditorModal from './WorldClueEditorModal.vue'
import WorldClueRosterPopover from './WorldClueRosterPopover.vue'

const props = defineProps<{ worldId: string; channelId: string; canManage?: boolean }>()
const store = useWorldClueStore()
const user = useUserStore()
const message = useMessage()
const dialog = useDialog()
const keyword = ref('')
const viewScope = ref<'shared' | 'personal'>('shared')
const selectedFolderId = ref('')
const editorVisible = ref(false)
const editingClue = ref<WorldClueDetail | null>(null)
const dragging = ref<{ type: 'clue' | 'folder'; id: string } | null>(null)
const dragOverTarget = ref('')
const inlineFolderMode = ref<'create' | 'rename' | null>(null)
const editingFolderId = ref('')
const folderNameDraft = ref('')
const folderSubmitting = ref(false)
const inlineEditSession = ref(0)
const submittedEditSession = ref(0)
const inlineInputRef = ref<{ focus: () => void } | null>(null)
const mediaFallback = reactive<Partial<Record<string, 'video' | 'failed'>>>({})
const favoriteOverrides = reactive<Partial<Record<string, boolean>>>({})
const favoriteSubmitting = reactive<Partial<Record<string, boolean>>>({})
const panelWasDragged = ref(false)
const tabDismissed = ref(false)
const preference = reactive({ x: 0, y: 80, width: 460, height: 720, expanded: true, pinned: true, layout: 'grid' as 'grid' | 'list', mediaPreview: false })
let searchTimer: ReturnType<typeof setTimeout> | null = null
let pointerCleanup: (() => void) | null = null

const panelMargin = 8
const panelMinWidth = 360
const panelMinHeight = 320

const preferenceKey = computed(() => `sealchat.world-clue-box.v1:${user.info?.id || 'anonymous'}:${props.worldId}`)
const scopedFolders = computed(() => store.folders.filter(folder => folder.scope === viewScope.value))
const visibleFolders = computed(() => scopedFolders.value
  .filter(folder => (folder.parentId || '') === selectedFolderId.value)
  .sort((a, b) => a.orderIndex - b.orderIndex))
const currentFolder = computed(() => scopedFolders.value.find(folder => folder.id === selectedFolderId.value))
const parentFolderId = computed(() => currentFolder.value?.parentId || '')
const siblingFolders = computed(() => scopedFolders.value
  .filter(folder => (folder.parentId || '') === parentFolderId.value)
  .sort((a, b) => a.orderIndex - b.orderIndex))
const folderTabs = computed(() => selectedFolderId.value
  ? [...siblingFolders.value, ...visibleFolders.value]
  : visibleFolders.value)
const canManageCurrentScope = computed(() => viewScope.value === 'personal' || !!props.canManage)
const canReorderCurrentScope = computed(() => canManageCurrentScope.value && !keyword.value.trim())
const currentFolderItems = computed(() => store.summaries
  .filter(item => viewScope.value === 'shared'
    ? (item.sharedFolderId || '') === selectedFolderId.value
    : (item.userState?.personalFolderId || '') === selectedFolderId.value)
  .sort((a, b) => viewScope.value === 'shared' ? a.orderIndex - b.orderIndex : (a.userState?.personalOrder || 0) - (b.userState?.personalOrder || 0)))
function isFavorite(summary: WorldClueSummary) {
  return favoriteOverrides[summary.id] ?? (summary.userState?.favorite === true)
}
const visibleItems = computed(() => viewScope.value === 'personal'
  ? currentFolderItems.value.filter(isFavorite)
  : currentFolderItems.value)

function readPreference() {
  try {
    const value = JSON.parse(localStorage.getItem(preferenceKey.value) || '{}')
    preference.width = Number(value.width) || 460
    preference.height = Number(value.height) || Math.min(720, window.innerHeight - 96)
    preference.x = Number.isFinite(Number(value.x)) ? Number(value.x) : window.innerWidth - preference.width - 12
    preference.y = Number(value.y) || 80
    preference.expanded = value.expanded !== false
    preference.pinned = value.pinned !== false
    preference.layout = value.layout === 'list' ? 'list' : 'grid'
    preference.mediaPreview = value.mediaPreview === true
  } catch { /* ignore invalid local state */ }
  clampPanelGeometry()
}
function clampPanelGeometry() {
  if (window.innerWidth <= 680) return
  const maxWidth = Math.max(panelMinWidth, window.innerWidth - panelMargin * 2)
  const maxHeight = Math.max(panelMinHeight, window.innerHeight - panelMargin * 2)
  preference.width = Math.max(panelMinWidth, Math.min(maxWidth, preference.width))
  preference.height = Math.max(panelMinHeight, Math.min(maxHeight, preference.height))
  preference.x = Math.max(panelMargin, Math.min(window.innerWidth - preference.width - panelMargin, preference.x))
  preference.y = Math.max(panelMargin, Math.min(window.innerHeight - preference.height - panelMargin, preference.y))
}
function savePreference() { try { localStorage.setItem(preferenceKey.value, JSON.stringify(preference)) } catch { /* ignore */ } }
watch(preference, savePreference, { deep: true })
watch(() => store.uiVisible, (visible, wasVisible) => {
  if (visible) {
    tabDismissed.value = false
    preference.expanded = true
  } else if (wasVisible && preference.expanded) {
    tabDismissed.value = true
  }
})
watch(scopedFolders, folders => {
  if (selectedFolderId.value && !folders.some(folder => folder.id === selectedFolderId.value)) selectedFolderId.value = ''
})
watch(() => [props.worldId, props.channelId, props.canManage] as const, ([worldId, channelId, canManage], previous) => {
  readPreference()
  if (previous?.[1] && previous[1] !== channelId && !preference.pinned) close()
  if (worldId) {
    void store.loadWorld(worldId)
    if (canManage) void store.loadRoster(worldId).catch(() => undefined)
  }
}, { immediate: true })
watch(keyword, value => { if (searchTimer) clearTimeout(searchTimer); searchTimer = setTimeout(() => void store.loadWorld(props.worldId, value), 220) })
onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer)
  pointerCleanup?.()
  chatEvent.off('world-clue-published' as any, handlePublished as any)
  chatEvent.off('world-clue-edit' as any, handleEdit as any)
  editRequest += 1
  window.removeEventListener('resize', clampPanelGeometry)
})

function close() { preference.expanded = false; store.setVisible(false) }
function openFromTab() {
  if (panelWasDragged.value) return
  store.setVisible(true)
  preference.expanded = true
}
function handlePublished(event: any) {
  const payload = event?.worldClue || event?.argv?.options || event?.argv?.Options || {}
  if (String(payload.worldId || '') === props.worldId) tabDismissed.value = false
}
async function openClue(summary: WorldClueSummary) {
  try {
    const clue = await store.fetchDetail(props.worldId, summary.id)
    chatEvent.emit('world-clue-open' as any, { worldId: props.worldId, clueId: clue.id })
  } catch { message.warning('线索不可用') }
}
async function editClue(summary?: WorldClueSummary) {
  if (editorVisible.value) return
  editingClue.value = summary ? await store.fetchDetail(props.worldId, summary.id, false) : null
  editorVisible.value = true
}
let editRequest = 0
async function handleEdit(payload: { worldId?: string; clueId?: string }) {
  if (payload?.worldId !== props.worldId || editorVisible.value) return
  const clueId = String(payload.clueId || '').trim()
  if (!clueId) return
  const worldId = props.worldId, request = ++editRequest
  try {
    const clue = await store.fetchDetail(worldId, clueId, false)
    if (request !== editRequest || props.worldId !== worldId || editorVisible.value) return
    if (clue.effectiveAccess !== 'edit') { message.warning('没有编辑此线索的权限'); return }
    editingClue.value = clue
    editorVisible.value = true
  } catch { message.warning('线索不可用或没有编辑权限') }
}
async function publish(summary: WorldClueSummary) {
  dialog.warning({
    title: summary.status === 'published' ? '确认再次揭示' : '确认揭示线索',
    content: summary.status === 'published'
      ? `将再次向当前可见的世界成员揭示「${summary.title}」。`
      : `将向世界成员揭示「${summary.title}」；明确设置为“不可见”的成员除外。`,
    positiveText: summary.status === 'published' ? '再次揭示' : '揭示',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const saved = await store.publish(props.worldId, summary.id, summary.publishSeq)
        if (editingClue.value?.id === saved.id) editingClue.value = saved
        message.success(summary.status === 'published' ? '已再次揭示' : '已揭示')
      } catch (error: any) {
        message.error(error?.response?.data?.message || '揭示失败')
        return false
      }
    },
  })
}
function deleteClue(summary: WorldClueSummary) {
  dialog.warning({
    title: '删除线索',
    content: `确定删除「${summary.title}」吗？删除后成员将无法继续查看此线索。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.removeClue(props.worldId, summary.id)
        if (editingClue.value?.id === summary.id) {
          editingClue.value = null
          editorVisible.value = false
        }
        message.success('线索已删除')
      } catch (error: any) {
        message.error(error?.response?.data?.message || '线索删除失败')
        return false
      }
    },
  })
}
function handleEditorSaved(saved: WorldClueDetail) {
  if (!editingClue.value || editingClue.value.id === saved.id) editingClue.value = saved
  void store.loadWorld(props.worldId, keyword.value)
}
async function copyLink(summary: WorldClueSummary) {
  await copyTextWithFallback(generateWorldClueEmbedLink({ worldId: props.worldId, channelId: props.channelId, clueId: summary.id }))
  message.success('嵌入链接已复制')
}
async function insertLink(summary: WorldClueSummary) {
  const link = generateWorldClueEmbedLink({ worldId: props.worldId, channelId: props.channelId, clueId: summary.id })
  chatEvent.emit('world-clue-insert-link' as any, { link })
}
async function toggleFavorite(summary: WorldClueSummary) {
  if (favoriteSubmitting[summary.id]) return
  const favorite = !isFavorite(summary)
  favoriteOverrides[summary.id] = favorite
  favoriteSubmitting[summary.id] = true
  try {
    const response = await api.patch(`api/v1/worlds/${props.worldId}/clues/${summary.id}/user-state`, { favorite })
    const userState = response.data?.item as WorldClueUserState | undefined
    if (userState) summary.userState = userState
    delete favoriteOverrides[summary.id]
  } catch (error: any) {
    delete favoriteOverrides[summary.id]
    message.error(error?.response?.data?.message || '收藏状态更新失败')
  } finally { delete favoriteSubmitting[summary.id] }
}
async function moveToFolder(summary: WorldClueSummary, folderId: string) {
  if (viewScope.value === 'shared') {
    await store.saveClue(props.worldId, summary.id, { expectedRevision: summary.revision, sharedFolderId: folderId })
  } else {
    await api.patch(`api/v1/worlds/${props.worldId}/clues/${summary.id}/user-state`, { personalFolderId: folderId })
  }
  await store.loadWorld(props.worldId, keyword.value)
}
function startDrag(event: DragEvent, type: 'clue' | 'folder', id: string) {
  dragging.value = { type, id }
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', id)
  }
}
function endDrag() { dragging.value = null; dragOverTarget.value = '' }
function reorderedIDs(ids: string[], sourceId: string, targetId: string) {
  const next = ids.filter(id => id !== sourceId)
  const targetIndex = next.indexOf(targetId)
  next.splice(targetIndex < 0 ? next.length : targetIndex, 0, sourceId)
  return next
}
function reorderedClueIDs(sourceId: string, targetId: string) {
  const visibleOrder = reorderedIDs(visibleItems.value.map(item => item.id), sourceId, targetId)
  if (viewScope.value === 'shared') return visibleOrder
  const visibleSet = new Set(visibleOrder)
  let visibleIndex = 0
  return currentFolderItems.value.map(item => visibleSet.has(item.id) ? visibleOrder[visibleIndex++] : item.id)
}
async function dropClue(targetId: string) {
  const source = dragging.value
  endDrag()
  if (!canReorderCurrentScope.value || source?.type !== 'clue' || source.id === targetId) return
  try {
    await api.post(`api/v1/worlds/${props.worldId}/clues/reorder`, {
      scope: viewScope.value, folderId: selectedFolderId.value,
      orderedIds: reorderedClueIDs(source.id, targetId),
    })
    await store.loadWorld(props.worldId, keyword.value)
  } catch (error: any) { message.error(error?.response?.data?.message || '线索排序失败') }
}
async function reorderFolder(sourceId: string, targetId: string) {
  if (!canReorderCurrentScope.value || sourceId === targetId) return
  const source = scopedFolders.value.find(folder => folder.id === sourceId)
  const target = scopedFolders.value.find(folder => folder.id === targetId)
  const parentId = target?.parentId || ''
  if (!source || !target || (source.parentId || '') !== parentId) return
  const siblingIds = scopedFolders.value
    .filter(folder => (folder.parentId || '') === parentId)
    .sort((a, b) => a.orderIndex - b.orderIndex)
    .map(folder => folder.id)
  try {
    await api.post(`api/v1/worlds/${props.worldId}/clue-folders/reorder`, {
      scope: viewScope.value, folderId: parentId,
      orderedIds: reorderedIDs(siblingIds, sourceId, targetId),
    })
    await store.loadWorld(props.worldId, keyword.value)
  } catch (error: any) { message.error(error?.response?.data?.message || '文件夹排序失败') }
}
function canAcceptDrop(targetFolderId: string, allowFolderReorder = false) {
  if (!canReorderCurrentScope.value || !dragging.value) return false
  if (dragging.value.type === 'clue') return targetFolderId !== selectedFolderId.value
  if (!allowFolderReorder || dragging.value.id === targetFolderId) return false
  const source = scopedFolders.value.find(folder => folder.id === dragging.value?.id)
  const target = scopedFolders.value.find(folder => folder.id === targetFolderId)
  return !!source && !!target && (source.parentId || '') === (target.parentId || '')
}
function markDropTarget(event: DragEvent, key: string, targetFolderId: string, allowFolderReorder = false) {
  if (!canAcceptDrop(targetFolderId, allowFolderReorder)) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  dragOverTarget.value = key
}
async function dropOnFolder(targetFolderId: string, reorderTargetId = '') {
  const source = dragging.value
  endDrag()
  if (!source || !canReorderCurrentScope.value) return
  if (source.type === 'folder') {
    if (reorderTargetId) await reorderFolder(source.id, reorderTargetId)
    return
  }
  if (targetFolderId === selectedFolderId.value) return
  const summary = currentFolderItems.value.find(item => item.id === source.id)
  if (!summary) return
  try { await moveToFolder(summary, targetFolderId) }
  catch (error: any) { message.error(error?.response?.data?.message || '线索移动失败') }
}
function focusInlineInput() { void nextTick(() => inlineInputRef.value?.focus()) }
function beginCreateFolder() {
  if (folderSubmitting.value) return
  inlineEditSession.value += 1
  inlineFolderMode.value = 'create'
  editingFolderId.value = ''
  folderNameDraft.value = ''
  focusInlineInput()
}
function beginRenameFolder(folderId: string, name: string) {
  if (folderSubmitting.value) return
  inlineEditSession.value += 1
  inlineFolderMode.value = 'rename'
  editingFolderId.value = folderId
  folderNameDraft.value = name
  focusInlineInput()
}
function cancelInlineFolderEdit() {
  if (folderSubmitting.value) return
  inlineFolderMode.value = null
  editingFolderId.value = ''
  folderNameDraft.value = ''
}
function switchScope(scope: 'shared' | 'personal') {
  if (folderSubmitting.value) return
  cancelInlineFolderEdit()
  viewScope.value = scope
  selectedFolderId.value = ''
}
async function submitInlineFolderEdit() {
  const session = inlineEditSession.value
  if (!inlineFolderMode.value || folderSubmitting.value || submittedEditSession.value === session) return
  const mode = inlineFolderMode.value
  const worldId = props.worldId
  const scope = viewScope.value
  const parentId = selectedFolderId.value
  const folderId = editingFolderId.value
  const name = folderNameDraft.value.trim()
  if (!name) { cancelInlineFolderEdit(); return }
  submittedEditSession.value = session
  folderSubmitting.value = true
  try {
    if (mode === 'create') {
      const lastOrder = visibleFolders.value.reduce((max, folder) => Math.max(max, folder.orderIndex), -1)
      await api.post(`api/v1/worlds/${worldId}/clue-folders`, {
        scope, parentId, name, orderIndex: lastOrder + 1,
      })
    } else {
      await api.patch(`api/v1/worlds/${worldId}/clue-folders/${folderId}`, { name })
    }
    inlineFolderMode.value = null
    editingFolderId.value = ''
    folderNameDraft.value = ''
    if (props.worldId === worldId) await store.loadWorld(worldId, keyword.value)
  } catch (error: any) {
    submittedEditSession.value = 0
    message.error(error?.response?.data?.message || (mode === 'create' ? '文件夹创建失败' : '文件夹重命名失败'))
    focusInlineInput()
  } finally { folderSubmitting.value = false }
}
function handleInlineFolderKeydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Enter') { event.preventDefault(); void submitInlineFolderEdit() }
  if (event.key === 'Escape') { event.preventDefault(); cancelInlineFolderEdit() }
}
function deleteFolder(folderId: string) {
  dialog.warning({ title: '删除文件夹', content: '线索不会被删除，将移动到上一级目录。', positiveText: '删除', negativeText: '取消', onPositiveClick: async () => {
    try {
      await api.delete(`api/v1/worlds/${props.worldId}/clue-folders/${folderId}`)
      if (selectedFolderId.value === folderId) selectedFolderId.value = parentFolderId.value
      await store.loadWorld(props.worldId, keyword.value)
    } catch (error: any) { message.error(error?.response?.data?.message || '文件夹删除失败'); return false }
  } })
}
function clueKindIcon(summary: WorldClueSummary) {
  if (summary.kind === 'image') return Photo
  if (summary.kind === 'iframe') return World
  return FileText
}
function clueMediaUrl(summary: WorldClueSummary) {
  return summary.imageAttachmentId
    ? `${urlBase}/api/v1/worlds/${encodeURIComponent(props.worldId)}/clues/${encodeURIComponent(summary.id)}/image?v=${encodeURIComponent(summary.imageAttachmentId)}`
    : summary.imageUrl || ''
}
function mediaFallbackKey(summary: WorldClueSummary) { return `${summary.id}:${summary.imageAttachmentId || summary.imageUrl || ''}` }
function mediaState(summary: WorldClueSummary): 'image' | 'video' | 'failed' | 'placeholder' {
  if (summary.kind !== 'image' || !clueMediaUrl(summary)) return 'placeholder'
  return mediaFallback[mediaFallbackKey(summary)] || 'image'
}
function useVideoFallback(summary: WorldClueSummary) { mediaFallback[mediaFallbackKey(summary)] = 'video' }
function useMediaPlaceholder(summary: WorldClueSummary) { mediaFallback[mediaFallbackKey(summary)] = 'failed' }
function startResize(event: PointerEvent, edge: 'left' | 'right' | 'bottom') {
  event.preventDefault()
  event.stopPropagation()
  const startX = event.clientX, startY = event.clientY
  const startLeft = preference.x, startWidth = preference.width, startHeight = preference.height
  const move = (next: PointerEvent) => {
    if (edge === 'left') {
      const right = startLeft + startWidth
      preference.x = Math.max(panelMargin, Math.min(right - panelMinWidth, startLeft + next.clientX - startX))
      preference.width = right - preference.x
    } else if (edge === 'right') {
      preference.width = Math.max(panelMinWidth, Math.min(window.innerWidth - panelMargin - startLeft, startWidth + next.clientX - startX))
    } else {
      preference.height = Math.max(panelMinHeight, Math.min(window.innerHeight - panelMargin - preference.y, startHeight + next.clientY - startY))
    }
  }
  pointerCleanup?.()
  const up = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); pointerCleanup = null }
  pointerCleanup = up
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', up)
}
function startMove(event: PointerEvent) {
  const startX = event.clientX, startY = event.clientY, originX = preference.x, originY = preference.y
  const movingPanel = store.uiVisible && preference.expanded
  if (movingPanel) event.preventDefault()
  const target = event.currentTarget as HTMLElement | null
  target?.setPointerCapture?.(event.pointerId)
  panelWasDragged.value = false
  const move = (next: PointerEvent) => {
    if (Math.abs(next.clientX - startX) > 3 || Math.abs(next.clientY - startY) > 3) {
      panelWasDragged.value = true
      next.preventDefault()
    }
    if (movingPanel) {
      preference.x = Math.max(panelMargin, Math.min(window.innerWidth - preference.width - panelMargin, originX + next.clientX - startX))
      preference.y = Math.max(panelMargin, Math.min(window.innerHeight - preference.height - panelMargin, originY + next.clientY - startY))
    } else {
      preference.y = Math.max(panelMargin, Math.min(window.innerHeight - 120, originY + next.clientY - startY))
    }
  }
  pointerCleanup?.()
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
    pointerCleanup = null
  }
  pointerCleanup = up
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', up); window.addEventListener('pointercancel', up)
}
onMounted(() => {
  readPreference()
  chatEvent.on('world-clue-published' as any, handlePublished as any)
  chatEvent.on('world-clue-edit' as any, handleEdit as any)
  window.addEventListener('resize', clampPanelGeometry)
})
</script>

<template>
  <button v-if="!tabDismissed && (!store.uiVisible || !preference.expanded)" class="clue-box-tab" type="button" :style="{ top: `${preference.y}px` }" @pointerdown="startMove" @click="openFromTab">
    <NBadge :value="store.unreadCount" :max="99">线索箱</NBadge>
  </button>
  <aside v-else-if="store.uiVisible && preference.expanded" class="clue-box" :style="{ left: `${preference.x}px`, top: `${preference.y}px`, width: `${preference.width}px`, height: `${preference.height}px` }">
    <div class="clue-box__resize clue-box__resize--left" @pointerdown="startResize($event, 'left')" />
    <div class="clue-box__resize clue-box__resize--right" @pointerdown="startResize($event, 'right')" />
    <div class="clue-box__resize clue-box__resize--bottom" @pointerdown="startResize($event, 'bottom')" />
    <header class="clue-box__header" @pointerdown="startMove">
      <div><strong>线索箱</strong><small>{{ store.unreadCount ? `${store.unreadCount} 条未读` : '世界资料' }}</small></div>
      <NSpace @pointerdown.stop><WorldClueRosterPopover v-if="canManage" :world-id="worldId" /><NButton circle quaternary size="small" :title="preference.pinned ? '取消固定' : '固定面板'" @click.stop="preference.pinned = !preference.pinned"><template #icon><NIcon><component :is="preference.pinned ? Pinned : Pin" /></NIcon></template></NButton><NButton v-if="canManage" circle quaternary size="small" title="新建线索" @click.stop="editClue()"><template #icon><NIcon><Plus /></NIcon></template></NButton><NButton circle quaternary size="small" title="关闭" @click.stop="close"><template #icon><NIcon><X /></NIcon></template></NButton></NSpace>
    </header>
    <div class="clue-box__tools">
      <NInput v-model:value="keyword" clearable placeholder="搜索线索"><template #prefix><NIcon><Search /></NIcon></template></NInput>
      <NButtonGroup><NButton :type="viewScope === 'shared' ? 'primary' : 'default'" :disabled="folderSubmitting" @click="switchScope('shared')">世界目录</NButton><NButton :type="viewScope === 'personal' ? 'primary' : 'default'" :disabled="folderSubmitting" @click="switchScope('personal')">我的收藏</NButton></NButtonGroup>
      <div class="clue-box__view-row">
        <div class="clue-box__folder-tabs">
          <div
            v-if="selectedFolderId" class="clue-box__folder-tab clue-box__folder-tab--back"
            :class="{ 'clue-box__folder-tab--drag-over': dragOverTarget === 'parent' }"
            title="返回上一级" @dragover="markDropTarget($event, 'parent', parentFolderId)" @dragleave="dragOverTarget = ''" @drop.prevent="dropOnFolder(parentFolderId)"
          >
            <button type="button" @click="selectedFolderId = parentFolderId"><NIcon><ChevronLeft /></NIcon></button>
          </div>
          <div v-if="!selectedFolderId" class="clue-box__folder-tab clue-box__folder-tab--current"><NIcon><Folder /></NIcon><span>根目录</span></div>
          <div
            v-for="folder in folderTabs" :key="folder.id" class="clue-box__folder-tab"
            :class="{
              'clue-box__folder-tab--current': folder.id === selectedFolderId,
              'clue-box__folder-tab--child': !!selectedFolderId && (folder.parentId || '') === selectedFolderId,
              'clue-box__folder-tab--drag-over': dragOverTarget === `folder:${folder.id}`,
            }"
            :draggable="canReorderCurrentScope && !(inlineFolderMode === 'rename' && editingFolderId === folder.id)"
            @dragstart="startDrag($event, 'folder', folder.id)" @dragend="endDrag"
            @dragover="markDropTarget($event, `folder:${folder.id}`, folder.id, true)" @dragleave="dragOverTarget = ''" @drop.prevent="dropOnFolder(folder.id, folder.id)"
          >
            <NInput
              v-if="inlineFolderMode === 'rename' && editingFolderId === folder.id" ref="inlineInputRef"
              v-model:value="folderNameDraft" class="clue-box__folder-input" size="tiny" :disabled="folderSubmitting"
              @keydown="handleInlineFolderKeydown" @blur="submitInlineFolderEdit"
            />
            <template v-else>
              <NIcon v-if="canReorderCurrentScope" class="clue-box__drag"><GripVertical /></NIcon>
              <button type="button" @click="selectedFolderId = folder.id">{{ folder.name }}</button>
              <span v-if="canManageCurrentScope" class="clue-box__folder-actions">
                <NButton text size="tiny" title="重命名文件夹" @click.stop="beginRenameFolder(folder.id, folder.name)"><template #icon><NIcon><Edit /></NIcon></template></NButton>
                <NButton text size="tiny" title="删除文件夹" @click.stop="deleteFolder(folder.id)"><template #icon><NIcon><Trash /></NIcon></template></NButton>
              </span>
            </template>
          </div>
          <div v-if="inlineFolderMode === 'create'" class="clue-box__folder-tab clue-box__folder-tab--editing">
            <NInput
              ref="inlineInputRef" v-model:value="folderNameDraft" class="clue-box__folder-input" size="tiny"
              placeholder="新文件夹" :disabled="folderSubmitting" @keydown="handleInlineFolderKeydown" @blur="submitInlineFolderEdit"
            />
          </div>
          <NButton v-else-if="canManageCurrentScope" class="clue-box__folder-add" circle quaternary size="small" title="新建文件夹" @click="beginCreateFolder"><template #icon><NIcon><Plus /></NIcon></template></NButton>
        </div>
        <div class="clue-box__display-tools">
          <NButton circle quaternary :title="preference.layout === 'grid' ? '列表' : '网格'" @click="preference.layout = preference.layout === 'grid' ? 'list' : 'grid'"><template #icon><NIcon><component :is="preference.layout === 'grid' ? List : GridDots" /></NIcon></template></NButton>
          <NButton circle quaternary :type="preference.mediaPreview ? 'primary' : 'default'" :title="preference.mediaPreview ? '显示内容' : '显示媒体'" @click="preference.mediaPreview = !preference.mediaPreview"><template #icon><NIcon><Photo /></NIcon></template></NButton>
        </div>
      </div>
    </div>
    <div v-if="visibleItems.length" class="clue-box__items" :class="`clue-box__items--${preference.layout}`">
      <article
        v-for="item in visibleItems" :key="item.id" class="clue-box__item" :class="{ 'clue-box__item--unread': item.unread }"
        :draggable="canReorderCurrentScope" @dragstart="startDrag($event, 'clue', item.id)" @dragend="endDrag"
        @dragover.prevent @drop.prevent="dropClue(item.id)" @dblclick="openClue(item)"
      >
        <div v-if="preference.mediaPreview" class="clue-box__media">
          <img
            v-if="mediaState(item) === 'image'" :src="clueMediaUrl(item)" :alt="item.title" loading="lazy"
            :referrerpolicy="item.imageAttachmentId ? undefined : 'no-referrer'" @error="useVideoFallback(item)"
          />
          <video v-else-if="mediaState(item) === 'video'" :src="clueMediaUrl(item)" muted playsinline preload="metadata" @error="useMediaPlaceholder(item)" />
          <div v-else class="clue-box__media-placeholder">
            <NIcon><component :is="item.kind === 'iframe' ? World : item.kind === 'image' ? Photo : FileText" /></NIcon>
            <span>{{ item.kind === 'iframe' ? item.embedDomain || '网页线索' : item.kind === 'image' ? '媒体不可预览' : '文档线索' }}</span>
          </div>
        </div>
        <div class="clue-box__item-heading">
          <NIcon><component :is="clueKindIcon(item)" /></NIcon><strong>{{ item.title }}</strong><span v-if="item.status === 'draft'">草稿</span>
          <NButton
            class="clue-box__favorite" :class="{ 'clue-box__favorite--active': isFavorite(item) }"
            circle quaternary size="tiny" :type="isFavorite(item) ? 'primary' : 'default'" :loading="favoriteSubmitting[item.id] === true"
            :aria-pressed="isFavorite(item)" :title="isFavorite(item) ? '取消收藏' : '收藏'" @click.stop="toggleFavorite(item)" @dblclick.stop
          ><template #icon><NIcon><Star /></NIcon></template></NButton>
        </div>
        <p v-if="!preference.mediaPreview">{{ item.contentText || (item.kind === 'iframe' ? item.embedDomain : '暂无摘要') }}</p>
        <div class="clue-box__item-actions">
          <NButton quaternary size="tiny" title="打开" @click.stop="openClue(item)" @dblclick.stop><template #icon><NIcon><ExternalLink /></NIcon></template>打开</NButton>
          <NButton v-if="canManage || item.effectiveAccess === 'edit'" quaternary size="tiny" title="编辑" @click.stop="editClue(item)" @dblclick.stop><template #icon><NIcon><Edit /></NIcon></template>编辑</NButton>
          <NButton v-if="canManage" quaternary size="tiny" :title="item.status === 'published' ? '再次揭示' : '揭示'" @click.stop="publish(item)" @dblclick.stop><template #icon><NIcon><Presentation /></NIcon></template>{{ item.status === 'published' ? '再次揭示' : '揭示' }}</NButton>
          <NButton v-if="canManage" circle quaternary size="tiny" type="error" title="删除" aria-label="删除" @click.stop="deleteClue(item)" @dblclick.stop><template #icon><NIcon><Trash /></NIcon></template></NButton>
          <NButton circle quaternary size="tiny" title="复制链接" @click.stop="copyLink(item)" @dblclick.stop><template #icon><NIcon><Copy /></NIcon></template></NButton>
          <NButton circle quaternary size="tiny" title="插入输入框" @click.stop="insertLink(item)" @dblclick.stop><template #icon><NIcon><MessagePlus /></NIcon></template></NButton>
        </div>
      </article>
    </div>
    <NEmpty v-else class="clue-box__empty" description="这里还没有可见线索" />
  </aside>
  <WorldClueEditorModal v-model:show="editorVisible" :world-id="worldId" :channel-id="channelId" :clue="editingClue" :can-manage="!!canManage" @saved="handleEditorSaved" />
</template>

<style scoped>
.clue-box { position: fixed; z-index: 999; display: flex; min-width: 360px; min-height: 320px; max-width: calc(100vw - 16px); max-height: calc(100dvh - 16px); flex-direction: column; overflow: hidden; color: var(--sc-text-primary); border: 1px solid var(--sc-border-strong); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-surface) 96%, transparent); box-shadow: 0 18px 50px #0004; backdrop-filter: blur(14px); }
.clue-box__resize { position: absolute; z-index: 2; touch-action: none; }
.clue-box__resize--left { inset: 0 auto 0 0; width: 7px; cursor: ew-resize; }
.clue-box__resize--right { inset: 0 0 0 auto; width: 7px; cursor: ew-resize; }
.clue-box__resize--bottom { inset: auto 0 0; height: 7px; cursor: ns-resize; }
.clue-box__header { display: flex; align-items: center; justify-content: space-between; padding: 13px 14px 11px 17px; border-bottom: 1px solid var(--sc-border-mute); cursor: move; }
.clue-box__header strong, .clue-box__header small { display: block; }
.clue-box__header small { margin-top: 2px; color: var(--sc-text-secondary); font-size: 11px; }
.clue-box__tools { display: grid; gap: 9px; padding: 12px 14px; }
.clue-box__view-row { display: flex; min-width: 0; align-items: center; gap: 7px; }
.clue-box__folder-tabs { display: flex; min-width: 0; flex: 1; align-items: center; gap: 5px; overflow-x: auto; scrollbar-width: thin; }
.clue-box__folder-tab { display: inline-flex; height: 32px; min-width: max-content; max-width: 190px; flex: 0 0 auto; align-items: center; gap: 5px; padding: 0 8px; color: var(--sc-text-secondary); border: 1px solid var(--sc-border-mute); border-radius: 5px; background: transparent; transition: border-color .14s ease, background-color .14s ease, color .14s ease; }
.clue-box__folder-tab:hover { color: var(--sc-text-primary); border-color: var(--sc-border-strong); background: color-mix(in srgb, var(--sc-bg-elevated) 78%, transparent); }
.clue-box__folder-tab--current { color: var(--sc-text-primary); border-color: color-mix(in srgb, var(--primary-color, #3388de) 42%, var(--sc-border-mute)); background: color-mix(in srgb, var(--primary-color, #3388de) 9%, var(--sc-bg-surface)); }
.clue-box__folder-tab:not(.clue-box__folder-tab--child) + .clue-box__folder-tab--child { position: relative; margin-left: 7px; }
.clue-box__folder-tab:not(.clue-box__folder-tab--child) + .clue-box__folder-tab--child::before { position: absolute; top: 6px; bottom: 6px; left: -7px; width: 1px; background: var(--sc-border-mute); content: ''; }
.clue-box__folder-tab--drag-over { color: var(--sc-text-primary); border-color: var(--primary-color, #3388de); background: color-mix(in srgb, var(--primary-color, #3388de) 14%, var(--sc-bg-surface)); }
.clue-box__folder-tab--back { padding: 0 5px; }
.clue-box__folder-tab > button { max-width: 112px; overflow: hidden; padding: 0; color: inherit; text-overflow: ellipsis; white-space: nowrap; border: 0; background: transparent; cursor: pointer; }
.clue-box__folder-tab--back > button { display: grid; width: 20px; height: 24px; place-items: center; }
.clue-box__folder-tab > span:not(.clue-box__folder-actions) { max-width: 112px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.clue-box__folder-actions { display: inline-flex; max-width: none; gap: 2px; opacity: 0; transition: opacity .14s ease; }
.clue-box__folder-tab:hover .clue-box__folder-actions, .clue-box__folder-tab:focus-within .clue-box__folder-actions { opacity: 1; }
.clue-box__folder-input { width: 128px; }
.clue-box__folder-tab--editing { padding: 0 4px; }
.clue-box__folder-add { flex: 0 0 auto; }
.clue-box__display-tools { display: flex; flex: 0 0 auto; gap: 3px; padding-left: 7px; border-left: 1px solid var(--sc-border-mute); }
.clue-box__items { min-height: 0; overflow: auto; padding: 0 14px 14px; }
.clue-box__items--grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
.clue-box__items--list { display: grid; gap: 7px; }
.clue-box__drag { color: var(--sc-text-secondary); cursor: grab; }
.clue-box__item { min-width: 0; overflow: hidden; padding: 12px; color: var(--sc-text-primary); border: 1px solid var(--sc-border-mute); border-radius: 5px; background: color-mix(in srgb, var(--sc-bg-elevated) 94%, var(--primary-color, #3388de) 2%); transition: border-color .14s ease, background-color .14s ease, transform .14s ease; }
.clue-box__item:hover { border-color: color-mix(in srgb, var(--primary-color, #3388de) 34%, var(--sc-border-mute)); background: color-mix(in srgb, var(--sc-bg-elevated) 90%, var(--primary-color, #3388de) 5%); transform: translateY(-1px); }
.clue-box__item--unread { border-left: 3px solid var(--primary-color, #3388de); }
.clue-box__item-heading { display: grid; grid-template-columns: 18px minmax(0, 1fr) auto 24px; align-items: center; gap: 5px; }
.clue-box__item-heading strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.clue-box__item-heading span { color: var(--sc-text-secondary); font-size: 10px; }
.clue-box__item p { display: -webkit-box; min-height: 38px; margin: 7px 0; overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; line-height: 1.45; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.clue-box__favorite { width: 26px; height: 26px; color: var(--sc-text-secondary); box-shadow: inset 0 0 0 1px var(--sc-border-mute); }
.clue-box__favorite--active { color: var(--primary-color, #3388de); background: color-mix(in srgb, var(--primary-color, #3388de) 18%, var(--sc-bg-surface)) !important; box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--primary-color, #3388de) 65%, var(--sc-border-strong)); }
.clue-box__favorite--active :deep(svg) { fill: currentColor !important; stroke-width: 2.2; }
.clue-box__media { display: grid; width: calc(100% + 24px); aspect-ratio: 16 / 9; margin: -12px -12px 11px; overflow: hidden; place-items: center; border-bottom: 1px solid var(--sc-border-mute); background: color-mix(in srgb, var(--sc-bg-surface) 86%, #000 14%); }
.clue-box__media img, .clue-box__media video { display: block; width: 100%; height: 100%; object-fit: cover; }
.clue-box__media-placeholder { display: grid; max-width: 90%; place-items: center; gap: 5px; color: var(--sc-text-secondary); font-size: 11px; text-align: center; }
.clue-box__media-placeholder .n-icon { font-size: 24px; }
.clue-box__item-actions { display: flex; min-height: 30px; flex-wrap: wrap; align-items: center; gap: 3px; margin: 10px -4px -5px; padding-top: 8px; border-top: 1px solid var(--sc-border-mute); }
.clue-box__empty { margin: auto; }
.clue-box-tab { position: fixed; right: 0; z-index: 999; width: 34px; min-height: 108px; padding: 28px 7px 10px; border: 1px solid var(--sc-border-strong); border-right: 0; border-radius: 5px 0 0 5px; color: var(--sc-text-primary); background: var(--sc-bg-elevated); writing-mode: vertical-rl; cursor: pointer; touch-action: none; user-select: none; }
.clue-box-tab :deep(.n-badge-sup) { top: -18px; right: 50%; transform: translateX(50%); writing-mode: horizontal-tb; }
.clue-box :deep(.n-input) { --n-box-shadow-hover: none !important; --n-box-shadow-focus: none !important; --n-box-shadow-active: none !important; --n-box-shadow-hover-warning: none !important; --n-box-shadow-focus-warning: none !important; --n-box-shadow-active-warning: none !important; --n-box-shadow-hover-error: none !important; --n-box-shadow-focus-error: none !important; --n-box-shadow-active-error: none !important; }
@media (max-width: 680px) { .clue-box { inset: 0 !important; width: 100% !important; height: 100dvh !important; min-width: 0; min-height: 0; max-width: none; max-height: none; border: 0; border-radius: 0; } .clue-box__resize { display: none; } .clue-box__folder-actions { opacity: 1; } .clue-box__items--grid { grid-template-columns: 1fr; } }
</style>
