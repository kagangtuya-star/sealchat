<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NBadge, NButton, NDropdown, NEmpty, NIcon, NSpin, NTooltip, useDialog, useMessage, type DropdownOption, type SelectOption } from 'naive-ui'
import {
  ArrowBackUp,
  ArrowForwardUp,
  ArrowsMaximize,
  ArrowsMinimize,
  DeviceFloppy,
  DotsVertical,
  Eye,
  LayoutGrid,
  LayoutGridAdd,
  Link,
  Maximize,
  Minimize,
  Movie,
  Pencil,
  Presentation,
  Template,
  Wand,
  X,
} from '@vicons/tabler'
import TheaterFloatingHost from '@/views/theater/host/TheaterFloatingHost.vue'
import type { TheaterFloatingWindowSummary } from '@/views/theater/host/theater-floating-window'
import { openInternalSurfaceLink } from '@/utils/internalSurfaceLink'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import type { SChannel } from '@/types'
import WorkbenchPanelView from './WorkbenchPanel.vue'
import WorkbenchDesignCanvas from './WorkbenchDesignCanvas.vue'
import WorkbenchInspector from './WorkbenchInspector.vue'
import WorkbenchTransferModal from './WorkbenchTransferModal.vue'
import { useChannelWorkbench } from './useChannelWorkbench'
import { WorkbenchConflictError, workbenchErrorMessage } from './api'
import {
  addWorkbenchPanel,
  autoArrangeWorkbench,
  cloneWorkbenchLayout,
  duplicateWorkbenchPanel,
  filterWorkbenchForPlayer,
  isWorkbenchPanelHidden,
  layoutsEqual,
  nextWorkbenchPanelId,
  removeWorkbenchPanel,
  validateWorkbenchLayout,
} from './layout'
import { resolveWorkbenchSource, WORKBENCH_SURFACE_LABELS, workbenchPanelTitle, type WorkbenchResolvedSource } from './sources'
import type { WorkbenchImportResult } from './transfer'
import type { WorkbenchLayout, WorkbenchPanel, WorkbenchSource, WorkbenchSurfaceType } from './types'
import {
  emptyWorkbenchViewState,
  readWorkbenchDraft,
  readWorkbenchViewState,
  writeWorkbenchDraft,
  writeWorkbenchViewState,
  type WorkbenchDraft,
  type WorkbenchLocalViewState,
} from './viewState'
import { publishWorkbenchRuntimeState, resetWorkbenchRuntimeState } from './runtimeState'

// 布局频道宿主。状态分三层：
//   1. 布局定义（服务端，layoutRevision，管理者在设计模式编辑草稿后保存）；
//   2. GM 展示状态（服务端，presentationRevision：面板对玩家显隐、共享聚焦）；
//   3. 本地显示状态（localStorage：折叠、展开、移动端标签、已读聚焦、未保存草稿；浮窗由 Floating Host 自己持久化）。

const props = defineProps<{
  channel: SChannel
  worldId: string
  immersive: boolean
  isMobile: boolean
}>()

const emit = defineEmits<{
  'update:immersive': [value: boolean]
}>()

const chat = useChatStore()
const user = useUserStore()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()

const channelId = computed(() => String(props.channel?.id || '').trim())
const userId = computed(() => String(user.info?.id || '').trim())
const enabled = computed(() => !!channelId.value && !!props.worldId)
const { view, loading, error, reload, saveLayout, applyPresentation } = useChannelWorkbench(channelId, enabled)
const canManage = computed(() => view.value?.canManage === true)
const mode = ref<'view' | 'design' | 'preview'>('view')
const transferVisible = ref(false)

// ---------------------------------------------------------------------------
// 本地显示状态

const localView = ref<WorkbenchLocalViewState>(emptyWorkbenchViewState())
let localViewOwner = ''
watch([userId, channelId], ([uid, cid]) => {
  localViewOwner = `${uid}\u0000${cid}`
  localView.value = readWorkbenchViewState(uid, cid)
}, { immediate: true })
watch(localView, (state) => {
  if (localViewOwner !== `${userId.value}\u0000${channelId.value}`) return
  writeWorkbenchViewState(userId.value, channelId.value, state)
}, { deep: true })

// ---------------------------------------------------------------------------
// 设计草稿

const draft = ref<WorkbenchLayout | null>(null)
const draftBaseRevision = ref(0)
const undoStack = ref<WorkbenchLayout[]>([])
const redoStack = ref<WorkbenchLayout[]>([])
const selectedPanelId = ref('')
const saving = ref(false)
const recoverableDraft = ref<WorkbenchDraft | null>(null)
const HISTORY_LIMIT = 50
const DRAFT_PERSIST_DELAY = 400
let draftPersistTimer: number | null = null
let draftPersistTask: (() => void) | null = null

const dirty = computed(() => mode.value === 'design' && !!draft.value && !!view.value && !layoutsEqual(draft.value, view.value.layout))
const remoteNewer = computed(() => mode.value === 'design' && !!view.value && view.value.layoutRevision > draftBaseRevision.value)

const flushDraftPersist = () => {
  if (draftPersistTimer !== null) {
    window.clearTimeout(draftPersistTimer)
    draftPersistTimer = null
  }
  draftPersistTask?.()
  draftPersistTask = null
}

watch(draft, () => {
  if (mode.value !== 'design' || !draft.value) return
  // 捕获当前频道，延迟写入也不会落到切换后的频道上。
  const uid = userId.value
  const cid = channelId.value
  const snapshot: WorkbenchDraft | null = dirty.value
    ? { baseRevision: draftBaseRevision.value, savedAt: Date.now(), layout: cloneWorkbenchLayout(draft.value) }
    : null
  if (draftPersistTimer !== null) window.clearTimeout(draftPersistTimer)
  draftPersistTask = () => writeWorkbenchDraft(uid, cid, snapshot)
  draftPersistTimer = window.setTimeout(() => {
    draftPersistTimer = null
    draftPersistTask?.()
    draftPersistTask = null
  }, DRAFT_PERSIST_DELAY)
})

const resetDesign = () => {
  flushDraftPersist()
  draft.value = null
  undoStack.value = []
  redoStack.value = []
  selectedPanelId.value = ''
  recoverableDraft.value = null
}

const enterDesign = () => {
  if (!canManage.value || !view.value) return
  resetDesign()
  draft.value = cloneWorkbenchLayout(view.value.layout)
  draftBaseRevision.value = view.value.layoutRevision
  const stored = readWorkbenchDraft(userId.value, channelId.value)
  if (stored && !layoutsEqual(stored.layout, view.value.layout)) recoverableDraft.value = stored
  mode.value = 'design'
}

const applyDraft = (next: WorkbenchLayout) => {
  if (!draft.value || layoutsEqual(next, draft.value)) return
  undoStack.value = [...undoStack.value.slice(-(HISTORY_LIMIT - 1)), draft.value]
  redoStack.value = []
  draft.value = next
  if (selectedPanelId.value && !next.panels.some(panel => panel.id === selectedPanelId.value)) selectedPanelId.value = ''
}

const undo = () => {
  const previous = undoStack.value[undoStack.value.length - 1]
  if (!previous || !draft.value) return
  undoStack.value = undoStack.value.slice(0, -1)
  redoStack.value = [...redoStack.value, draft.value]
  draft.value = previous
}

const redo = () => {
  const next = redoStack.value[redoStack.value.length - 1]
  if (!next || !draft.value) return
  redoStack.value = redoStack.value.slice(0, -1)
  undoStack.value = [...undoStack.value, draft.value]
  draft.value = next
}

const recoverDraft = () => {
  const stored = recoverableDraft.value
  recoverableDraft.value = null
  if (!stored) return
  applyDraft(stored.layout)
  draftBaseRevision.value = Math.min(draftBaseRevision.value, stored.baseRevision || draftBaseRevision.value)
}

const discardRecoverableDraft = () => {
  recoverableDraft.value = null
  writeWorkbenchDraft(userId.value, channelId.value, null)
}

const leaveDesign = (discard: boolean) => {
  if (discard) {
    flushDraftPersist()
    writeWorkbenchDraft(userId.value, channelId.value, null)
  }
  resetDesign()
  mode.value = 'view'
}

const cancelDesign = () => {
  if (!dirty.value) {
    leaveDesign(true)
    return
  }
  dialog.warning({
    title: '放弃未保存的修改？',
    content: '草稿已暂存在本浏览器，下次进入设计时可以恢复；选择“放弃”会同时删除暂存。',
    positiveText: '放弃',
    negativeText: '保留草稿并退出',
    onPositiveClick: () => leaveDesign(true),
    onNegativeClick: () => leaveDesign(false),
  })
}

const persistLayout = async (baseRevision: number) => {
  const layout = draft.value
  if (!layout) return
  saving.value = true
  try {
    await saveLayout(layout, baseRevision)
    message.success('布局已保存')
    leaveDesign(true)
  } catch (err) {
    if (err instanceof WorkbenchConflictError) {
      const latestRevision = err.layoutRevision
      dialog.warning({
        title: '布局已被其他管理员更新',
        content: `服务端已是版本 ${latestRevision}。覆盖保存会替换对方的修改；也可以载入最新布局（放弃你的草稿）。`,
        positiveText: '覆盖保存',
        negativeText: '载入最新',
        onPositiveClick: () => { void persistLayout(latestRevision) },
        onNegativeClick: async () => {
          const latest = await reload()
          if (!latest || mode.value !== 'design') return
          undoStack.value = []
          redoStack.value = []
          draft.value = cloneWorkbenchLayout(latest.layout)
          draftBaseRevision.value = latest.layoutRevision
          writeWorkbenchDraft(userId.value, channelId.value, null)
        },
      })
      return
    }
    message.error(workbenchErrorMessage(err, '布局保存失败'))
  } finally {
    saving.value = false
  }
}

const save = () => {
  if (!draft.value) return
  const invalid = validateWorkbenchLayout(draft.value)
  if (invalid) {
    message.error(invalid)
    return
  }
  void persistLayout(draftBaseRevision.value)
}

const loadLatestIntoDraft = () => {
  if (!view.value) return
  dialog.warning({
    title: '载入最新布局？',
    content: '你的草稿修改会被丢弃。',
    positiveText: '载入最新',
    negativeText: '取消',
    onPositiveClick: () => {
      if (!view.value) return
      applyDraft(cloneWorkbenchLayout(view.value.layout))
      draftBaseRevision.value = view.value.layoutRevision
    },
  })
}

// 添加面板
const addOptions: DropdownOption[] = [
  ...(Object.entries(WORKBENCH_SURFACE_LABELS).map(([surface, label]) => ({ key: `surface:${surface}`, label }))),
  { type: 'divider', key: 'divider' },
  { key: 'chat', label: '聊天' },
  { key: 'theater', label: '小剧场' },
  { key: 'web', label: '外部网页' },
]

const addPanel = (key: string) => {
  if (!draft.value) return
  const source: WorkbenchSource = key.startsWith('surface:')
    ? { type: 'surface', surface: key.slice('surface:'.length) as WorkbenchSurfaceType }
    : { type: key as WorkbenchSource['type'] }
  if (source.surface === 'clue-board') source.resourceId = 'main'
  const id = nextWorkbenchPanelId(draft.value.panels.map(panel => panel.id))
  const next = addWorkbenchPanel(draft.value, { id, source, chrome: 'minimal' })
  if (!next) {
    message.warning('网格已满或面板数量已达上限')
    return
  }
  applyDraft(next)
  selectedPanelId.value = id
}

const duplicatePanel = (id: string) => {
  if (!draft.value) return
  const result = duplicateWorkbenchPanel(draft.value, id)
  if (!result) {
    message.warning('没有足够空间复制该面板')
    return
  }
  applyDraft(result.layout)
  selectedPanelId.value = result.panelId
}

const removePanel = (id: string) => {
  if (!draft.value) return
  applyDraft(removeWorkbenchPanel(draft.value, id))
}

const applyTransfer = (result: WorkbenchImportResult) => {
  if (mode.value !== 'design') enterDesign()
  if (!draft.value) return
  applyDraft(result.layout)
  selectedPanelId.value = ''
  const pending = Object.keys(designIssues.value).length
  message.info(pending ? `已应用到草稿，${pending} 个面板需要绑定资源，保存后生效` : '已应用到草稿，保存后生效')
}

// ---------------------------------------------------------------------------
// 频道与资源解析

const channelIndex = computed(() => {
  const map = new Map<string, string>()
  const walk = (items?: SChannel[]) => {
    for (const item of items || []) {
      if (!item?.id || (item as any).isPrivate) continue
      map.set(item.id, item.name || item.id)
      walk(item.children as SChannel[] | undefined)
    }
  }
  walk(chat.channelTree as SChannel[])
  return map
})
const channelOptions = computed<SelectOption[]>(() => [...channelIndex.value.entries()]
  .filter(([id]) => id !== channelId.value)
  .map(([id, name]) => ({ label: name, value: id })))
const sourceContext = computed(() => ({ worldId: props.worldId, channelId: channelId.value }))
const isChannelMissing = (panel: WorkbenchPanel) => {
  const target = panel.source.type === 'web' ? '' : (panel.source.channelId || '')
  return !!target && channelIndex.value.size > 0 && !channelIndex.value.has(target)
}

const displayLayout = computed<WorkbenchLayout | null>(() => {
  const current = view.value
  if (!current || current.channelId !== channelId.value) return null
  if (mode.value === 'design') return draft.value
  if (mode.value === 'preview') return filterWorkbenchForPlayer(current.layout, current.presentation)
  return current.layout
})
const panels = computed(() => displayLayout.value?.panels || [])
const resolvedSources = computed(() => {
  const map: Record<string, WorkbenchResolvedSource> = {}
  for (const panel of panels.value) map[panel.id] = resolveWorkbenchSource(panel, sourceContext.value)
  return map
})

const designIssues = computed(() => {
  const issues: Record<string, string> = {}
  if (!draft.value) return issues
  for (const panel of draft.value.panels) {
    const resolved = resolveWorkbenchSource(panel, sourceContext.value)
    if (isChannelMissing(panel)) issues[panel.id] = '频道不可用，需重新绑定'
    else if (resolved.kind === 'unbound' || resolved.kind === 'invalid') issues[panel.id] = resolved.reason
  }
  return issues
})
const selectNextIssue = () => {
  const ids = Object.keys(designIssues.value)
  if (!ids.length) return
  const index = ids.indexOf(selectedPanelId.value)
  selectedPanelId.value = ids[(index + 1) % ids.length]
}

// ---------------------------------------------------------------------------
// 个人浮窗（Floating Host，workbench 模式）

const floatingHostRef = ref<InstanceType<typeof TheaterFloatingHost> | null>(null)
const floatingSummaries = ref<TheaterFloatingWindowSummary[]>([])
const floatingKeyOf = (panel: WorkbenchPanel) => {
  const resolved = resolvedSources.value[panel.id]
  return resolved && 'floatingKey' in resolved ? resolved.floatingKey : ''
}
const poppedState = (panel: WorkbenchPanel): 'transient' | 'pinned' | null => {
  if (mode.value === 'design') return null
  const key = floatingKeyOf(panel)
  const summary = key ? floatingSummaries.value.find(item => item.key === key) : undefined
  if (!summary) return null
  return summary.transient ? 'transient' : 'pinned'
}

const popoutPanel = (panel: WorkbenchPanel, transient: boolean) => {
  const host = floatingHostRef.value
  const resolved = resolvedSources.value[panel.id]
  const title = workbenchPanelTitle(panel)
  let opened = false
  if (host && resolved?.kind === 'surface') {
    opened = host.openResource({ key: resolved.floatingKey, url: resolved.url, title, presentation: { transient } })
  } else if (host && resolved?.kind === 'chat') {
    opened = host.openCustomWindow({ source: 'chat', title, targetChannelId: resolved.targetChannelId, key: resolved.floatingKey, transient })
  } else if (host && resolved?.kind === 'web' && resolved.display === 'embed') {
    opened = host.openCustomWindow({ source: 'web', title, url: resolved.url, key: resolved.floatingKey, transient })
  }
  if (!opened) {
    message.warning('该面板不支持弹出为浮窗')
    return
  }
  if (localView.value.focusedPanelId === panel.id) localView.value.focusedPanelId = ''
}

const takeBackPanel = (panel: WorkbenchPanel) => {
  const key = floatingKeyOf(panel)
  const summary = floatingSummaries.value.find(item => item.key === key)
  if (summary) floatingHostRef.value?.closeWindow(summary.id)
}

const openPanelWindow = (panel: WorkbenchPanel) => {
  const resolved = resolvedSources.value[panel.id]
  if (!resolved) return
  if (resolved.kind === 'web') {
    window.open(resolved.url, '_blank', 'noopener,noreferrer')
    return
  }
  const url = resolved.kind === 'surface' ? resolved.url : resolved.kind === 'chat' || resolved.kind === 'theater' ? resolved.externalUrl : ''
  if (url && !openInternalSurfaceLink(url)) message.warning('浏览器阻止了新窗口')
}

// GM 收回或删除面板后，关闭对应的个人浮窗，避免已隐藏的内容继续显示。
watch([() => view.value, floatingSummaries, mode], () => {
  const current = view.value
  if (!current || current.channelId !== channelId.value || mode.value === 'design' || !floatingHostRef.value) return
  const keys = new Set(current.layout.panels
    .filter(panel => current.canManage || !isWorkbenchPanelHidden(panel, current.presentation))
    .map(panel => {
      const resolved = resolveWorkbenchSource(panel, sourceContext.value)
      return 'floatingKey' in resolved ? resolved.floatingKey : ''
    }))
  for (const summary of floatingSummaries.value) {
    if (!keys.has(summary.key)) floatingHostRef.value.closeWindow(summary.id)
  }
})

// ---------------------------------------------------------------------------
// 展开、聚焦、折叠

const spotlight = computed(() => view.value?.presentation.spotlight || null)
const spotlightPanel = computed(() => {
  const target = spotlight.value?.panelId
  return target ? panels.value.find(panel => panel.id === target) || null : null
})
const spotlightActive = computed(() => (
  mode.value !== 'design' && !!spotlightPanel.value && localView.value.dismissedSpotlightAt !== spotlight.value?.at
))
const focusedPanelId = computed(() => {
  if (mode.value === 'design') return ''
  if (spotlightActive.value && spotlightPanel.value && !poppedState(spotlightPanel.value)) return spotlightPanel.value.id
  const id = localView.value.focusedPanelId
  const panel = id ? panels.value.find(item => item.id === id) : undefined
  return panel && !poppedState(panel) ? panel.id : ''
})

const dismissSpotlight = () => {
  if (spotlight.value) localView.value.dismissedSpotlightAt = spotlight.value.at
}
const reopenSpotlight = () => { localView.value.dismissedSpotlightAt = 0 }

const toggleFocus = (panel: WorkbenchPanel) => {
  if (spotlightActive.value && spotlight.value?.panelId === panel.id) {
    dismissSpotlight()
    return
  }
  localView.value.focusedPanelId = focusedPanelId.value === panel.id ? '' : panel.id
}

const toggleCollapse = (panel: WorkbenchPanel) => {
  const collapsed = new Set(localView.value.collapsed)
  if (collapsed.has(panel.id)) collapsed.delete(panel.id)
  else collapsed.add(panel.id)
  localView.value.collapsed = [...collapsed]
}

// ---------------------------------------------------------------------------
// GM 展示状态

const presentationBusy = ref(false)
const runPresentation = async (input: Parameters<typeof applyPresentation>[0]) => {
  if (!view.value?.layoutRevision) {
    message.warning('请先保存布局')
    return
  }
  presentationBusy.value = true
  try {
    await applyPresentation(input)
  } catch (err) {
    message.error(workbenchErrorMessage(err, '展示状态更新失败'))
  } finally {
    presentationBusy.value = false
  }
}

const panelHiddenForPlayers = (panel: WorkbenchPanel) => canManage.value && isWorkbenchPanelHidden(panel, view.value?.presentation)

const toggleVisibility = (panel: WorkbenchPanel) => {
  const hidden = isWorkbenchPanelHidden(panel, view.value?.presentation)
  const target = hidden ? 'shown' : 'hidden'
  // 与默认值相同时清除覆盖，保持展示状态最小。
  const visibility = (target === 'hidden') === (panel.hiddenByDefault === true) ? 'default' : target
  void runPresentation({ action: 'visibility', panelId: panel.id, visibility })
}

const spotlightToAll = (panel: WorkbenchPanel) => {
  if (!isWorkbenchPanelHidden(panel, view.value?.presentation)) {
    void runPresentation({ action: 'spotlight', panelId: panel.id })
    return
  }
  dialog.info({
    title: '聚焦隐藏面板',
    content: '该面板当前对玩家隐藏，聚焦会同时对玩家显示它。',
    positiveText: '显示并聚焦',
    negativeText: '取消',
    onPositiveClick: () => { void runPresentation({ action: 'spotlight', panelId: panel.id, reveal: true }) },
  })
}

const clearSpotlight = () => { void runPresentation({ action: 'clear-spotlight' }) }
const resetPresentation = () => {
  dialog.warning({
    title: '重置展示状态？',
    content: '所有面板恢复默认显隐，并结束当前聚焦。',
    positiveText: '重置',
    negativeText: '取消',
    onPositiveClick: () => { void runPresentation({ action: 'reset' }) },
  })
}

// ---------------------------------------------------------------------------
// 移动端

const mobileMode = computed(() => (displayLayout.value?.settings.mobileMode === 'stack' ? 'stack' : 'tabs'))
const minimalActionsPlacement = computed(() => displayLayout.value?.settings.minimalActionsPlacement === 'outside' ? 'outside' : 'inside')
const layoutVariant = computed(() => (props.isMobile && mode.value !== 'design' ? mobileMode.value : 'grid'))
const mobileActiveId = computed(() => {
  if (focusedPanelId.value) return focusedPanelId.value
  const stored = localView.value.mobilePanelId
  return panels.value.some(panel => panel.id === stored) ? stored : panels.value[0]?.id || ''
})
const mountedTabs = reactive(new Set<string>())
watch([mobileActiveId, layoutVariant], ([id, variant]) => { if (id && variant === 'tabs') mountedTabs.add(id) }, { immediate: true })
watch([channelId, layoutVariant], () => {
  mountedTabs.clear()
  if (layoutVariant.value === 'tabs' && mobileActiveId.value) mountedTabs.add(mobileActiveId.value)
})
const selectMobilePanel = (id: string) => {
  localView.value.mobilePanelId = id
  if (localView.value.focusedPanelId && localView.value.focusedPanelId !== id) localView.value.focusedPanelId = ''
}
const panelMounted = (panel: WorkbenchPanel) => layoutVariant.value !== 'tabs' || mountedTabs.has(panel.id)

// ---------------------------------------------------------------------------
// 网格样式

const gridStyle = computed(() => {
  const layout = displayLayout.value
  if (!layout || layoutVariant.value !== 'grid') return {}
  return {
    gridTemplateColumns: `repeat(${layout.grid.columns}, minmax(0, 1fr))`,
    gridTemplateRows: `repeat(${layout.grid.rows}, minmax(0, 1fr))`,
    gap: `${layout.grid.gap}px`,
  }
})
const panelStyle = (panel: WorkbenchPanel) => {
  if (layoutVariant.value !== 'grid') return {}
  if (focusedPanelId.value === panel.id) return { gridColumn: '1 / -1', gridRow: '1 / -1', zIndex: 5 }
  return { gridColumn: `${panel.rect.x + 1} / span ${panel.rect.w}`, gridRow: `${panel.rect.y + 1} / span ${panel.rect.h}` }
}
const rootStyle = computed(() => {
  const accent = displayLayout.value?.settings.accent
  return accent ? { '--sc-workbench-accent': accent } : {}
})

// ---------------------------------------------------------------------------
// 沉浸与浏览器全屏

const browserFullscreen = ref(false)
const handleFullscreenChange = () => { browserFullscreen.value = !!document.fullscreenElement }
const setImmersive = (value: boolean) => {
  if (!value && document.fullscreenElement) void document.exitFullscreen().catch(() => undefined)
  emit('update:immersive', value)
}
const toggleBrowserFullscreen = async () => {
  try {
    if (document.fullscreenElement) {
      await document.exitFullscreen()
      return
    }
    // 全屏整个文档，浮窗层（Teleport 到 body）仍然可见。
    await document.documentElement.requestFullscreen()
    emit('update:immersive', true)
  } catch {
    message.warning('浏览器拒绝了全屏请求')
  }
}
const handleKeydown = (event: KeyboardEvent) => {
  if (event.key !== 'Escape' || event.defaultPrevented) return
  if (document.querySelector('.n-modal-container .n-modal, .n-dialog')) return
  if (focusedPanelId.value && !spotlightActive.value) {
    localView.value.focusedPanelId = ''
    return
  }
  if (props.immersive) setImmersive(false)
}

const enterTheater = async () => {
  if (!props.worldId || !channelId.value) return
  try {
    await router.push({ name: 'theater', query: { worldId: props.worldId, channelId: channelId.value, pip: '0' } })
  } catch (err) {
    message.error(err instanceof Error ? err.message : '进入小剧场失败')
  }
}

const moreOptions = computed<DropdownOption[]>(() => {
  const options: DropdownOption[] = [{ key: 'theater', label: '进入小剧场' }]
  if (canManage.value) {
    options.push({ key: 'transfer', label: '模板与导入导出' })
    if (spotlight.value) options.push({ key: 'clear-spotlight', label: '结束聚焦' })
    options.push({ key: 'reset-presentation', label: '重置展示状态' })
  }
  options.push({ key: 'fullscreen', label: browserFullscreen.value ? '退出浏览器全屏' : '浏览器全屏' })
  return options
})
const handleMore = (key: string) => {
  switch (key) {
    case 'theater': void enterTheater(); break
    case 'transfer': transferVisible.value = true; break
    case 'clear-spotlight': clearSpotlight(); break
    case 'reset-presentation': resetPresentation(); break
    case 'fullscreen': void toggleBrowserFullscreen(); break
  }
}

// ---------------------------------------------------------------------------
// 频道切换与界面自定义集成

watch(channelId, () => {
  resetDesign()
  mode.value = 'view'
  transferVisible.value = false
})
watch(canManage, (value) => {
  if (!value && mode.value !== 'view') {
    resetDesign()
    mode.value = 'view'
  }
})

watch(() => [channelId.value, mode.value, props.immersive, focusedPanelId.value, spotlightActive.value ? spotlight.value?.panelId || '' : ''] as const,
  ([cid, currentMode, immersive, focused, spotlighted]) => {
    publishWorkbenchRuntimeState({ active: true, channelId: cid, mode: currentMode, immersive, focusedPanelId: focused, spotlightPanelId: spotlighted })
    const root = document.documentElement.dataset
    root.scWorkbench = currentMode
    if (immersive) root.scImmersive = 'true'
    else delete root.scImmersive
  }, { immediate: true })

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  document.addEventListener('fullscreenchange', handleFullscreenChange)
})

onBeforeUnmount(() => {
  flushDraftPersist()
  window.removeEventListener('keydown', handleKeydown)
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  resetWorkbenchRuntimeState()
  delete document.documentElement.dataset.scWorkbench
  delete document.documentElement.dataset.scImmersive
})
</script>

<template>
  <div
    class="sc-workbench-root"
    data-sc-ui="workbench"
    :data-sc-workbench-mode="mode"
    :data-sc-workbench-layout="layoutVariant"
    :data-sc-panel-style="displayLayout?.settings.panelStyle || 'default'"
    :data-sc-minimal-actions-placement="minimalActionsPlacement"
    :data-sc-immersive="immersive ? 'true' : undefined"
    :style="rootStyle"
  >
    <header class="wb-toolbar" data-sc-ui="workbench-toolbar">
      <span class="wb-toolbar__title">
        <NIcon aria-hidden="true"><LayoutGrid /></NIcon>
        <strong>{{ channel.name }}</strong>
        <span v-if="mode === 'design'" class="wb-toolbar__tag">设计中{{ dirty ? ' · 未保存' : '' }}</span>
        <span v-else-if="mode === 'preview'" class="wb-toolbar__tag">玩家视角</span>
      </span>

      <template v-if="mode === 'design'">
        <span class="wb-toolbar__group">
          <NDropdown trigger="click" :options="addOptions" @select="addPanel">
            <NButton size="small" type="primary" secondary>
              <template #icon><NIcon><LayoutGridAdd /></NIcon></template>
              添加面板
            </NButton>
          </NDropdown>
          <NTooltip><template #trigger><NButton size="small" quaternary :disabled="!undoStack.length" aria-label="撤销" @click="undo"><template #icon><NIcon><ArrowBackUp /></NIcon></template></NButton></template>撤销</NTooltip>
          <NTooltip><template #trigger><NButton size="small" quaternary :disabled="!redoStack.length" aria-label="重做" @click="redo"><template #icon><NIcon><ArrowForwardUp /></NIcon></template></NButton></template>重做</NTooltip>
          <NTooltip><template #trigger><NButton size="small" quaternary aria-label="自动排列" @click="draft && applyDraft(autoArrangeWorkbench(draft))"><template #icon><NIcon><Wand /></NIcon></template></NButton></template>自动排列（紧凑填充空隙）</NTooltip>
          <NButton size="small" quaternary @click="transferVisible = true">
            <template #icon><NIcon><Template /></NIcon></template>
            模板 / 导入导出
          </NButton>
          <NBadge v-if="Object.keys(designIssues).length" :value="Object.keys(designIssues).length" type="warning">
            <NButton size="small" quaternary @click="selectNextIssue">
              <template #icon><NIcon><Link /></NIcon></template>
              待绑定
            </NButton>
          </NBadge>
        </span>
        <span class="wb-toolbar__group">
          <NButton size="small" @click="cancelDesign">取消</NButton>
          <NButton size="small" type="primary" :loading="saving" :disabled="!dirty" @click="save">
            <template #icon><NIcon><DeviceFloppy /></NIcon></template>
            保存
          </NButton>
        </span>
      </template>

      <template v-else>
        <span class="wb-toolbar__group">
          <NButton v-if="!isMobile" size="small" quaternary @click="enterTheater">
            <template #icon><NIcon><Movie /></NIcon></template>
            小剧场
          </NButton>
          <template v-if="canManage">
            <NButton size="small" :type="mode === 'preview' ? 'primary' : 'default'" :secondary="mode === 'preview'" :quaternary="mode !== 'preview'" :aria-pressed="mode === 'preview'" @click="mode = mode === 'preview' ? 'view' : 'preview'">
              <template #icon><NIcon><Eye /></NIcon></template>
              玩家视角
            </NButton>
            <NButton v-if="!isMobile" size="small" quaternary :disabled="!view" @click="enterDesign">
              <template #icon><NIcon><Pencil /></NIcon></template>
              设计
            </NButton>
          </template>
          <NTooltip>
            <template #trigger>
              <NButton size="small" quaternary :aria-pressed="immersive" :aria-label="immersive ? '退出沉浸模式' : '沉浸模式'" @click="setImmersive(!immersive)">
                <template #icon><NIcon><Minimize v-if="immersive" /><Maximize v-else /></NIcon></template>
                <span v-if="immersive">退出沉浸</span>
              </NButton>
            </template>
            {{ immersive ? '退出沉浸（Esc）' : '沉浸模式：隐藏导航与侧栏' }}
          </NTooltip>
          <NDropdown trigger="click" :options="moreOptions" placement="bottom-end" @select="handleMore">
            <NButton size="small" quaternary aria-label="更多">
              <template #icon><NIcon><DotsVertical /></NIcon></template>
            </NButton>
          </NDropdown>
        </span>
      </template>
    </header>

    <div v-if="spotlight && spotlightPanel && mode !== 'design'" class="wb-spotlight" data-sc-ui="workbench-spotlight" role="status">
      <NIcon aria-hidden="true"><Presentation /></NIcon>
      <span>GM 正在聚焦：<strong>{{ workbenchPanelTitle(spotlightPanel) }}</strong></span>
      <NButton v-if="spotlightActive" size="tiny" quaternary @click="dismissSpotlight">
        <template #icon><NIcon><ArrowsMinimize /></NIcon></template>
        退出聚焦
      </NButton>
      <NButton v-else size="tiny" quaternary @click="reopenSpotlight">
        <template #icon><NIcon><ArrowsMaximize /></NIcon></template>
        查看聚焦
      </NButton>
      <NButton v-if="canManage" size="tiny" type="primary" secondary :loading="presentationBusy" @click="clearSpotlight">
        <template #icon><NIcon><X /></NIcon></template>
        结束聚焦
      </NButton>
    </div>

    <NAlert v-if="mode === 'design' && recoverableDraft" type="info" :bordered="false" class="wb-notice">
      发现 {{ new Date(recoverableDraft.savedAt).toLocaleString() }} 暂存的未保存草稿。
      <NButton size="tiny" type="primary" secondary @click="recoverDraft">恢复</NButton>
      <NButton size="tiny" quaternary @click="discardRecoverableDraft">丢弃</NButton>
    </NAlert>
    <NAlert v-if="remoteNewer" type="warning" :bordered="false" class="wb-notice">
      其他管理员已把布局更新到版本 {{ view?.layoutRevision }}，保存时需要确认是否覆盖。
      <NButton size="tiny" quaternary @click="loadLatestIntoDraft">载入最新</NButton>
    </NAlert>

    <div class="wb-stage">
      <div v-if="loading && !displayLayout" class="wb-state"><NSpin size="small" /> 正在加载布局…</div>
      <div v-else-if="error && !displayLayout" class="wb-state" role="alert">
        <p>{{ error }}</p>
        <NButton size="small" @click="reload">重试</NButton>
      </div>

      <template v-else-if="displayLayout && mode === 'design'">
        <div class="wb-design-shell">
          <div class="wb-scroll">
            <WorkbenchDesignCanvas
              :layout="displayLayout"
              :selected-id="selectedPanelId"
              :issues="designIssues"
              @select="id => (selectedPanelId = id)"
              @change="applyDraft"
              @rejected="text => message.warning(text)"
              @remove="removePanel"
            />
          </div>
          <WorkbenchInspector
            class="wb-design-shell__inspector"
            :layout="displayLayout"
            :panel-id="selectedPanelId"
            :world-id="worldId"
            :channel-id="channelId"
            :channel-options="channelOptions"
            @change="applyDraft"
            @rejected="text => message.warning(text)"
            @duplicate="duplicatePanel"
            @remove="removePanel"
          />
        </div>
      </template>

      <template v-else-if="displayLayout">
        <div v-if="!panels.length" class="wb-state">
          <NEmpty :description="canManage ? '这个布局还没有面板' : 'GM 尚未配置布局，或当前没有对你显示的面板'">
            <template #extra>
              <span class="wb-state__actions">
                <template v-if="canManage && !isMobile">
                  <NButton size="small" type="primary" @click="enterDesign(); transferVisible = true">选择预设</NButton>
                  <NButton size="small" @click="enterDesign">开始设计</NButton>
                </template>
              </span>
            </template>
          </NEmpty>
        </div>
        <template v-else>
          <nav v-if="layoutVariant === 'tabs'" class="wb-tabs" aria-label="面板">
            <button
              v-for="panel in panels"
              :key="panel.id"
              type="button"
              class="wb-tabs__item"
              :class="{ 'is-active': panel.id === mobileActiveId }"
              :aria-current="panel.id === mobileActiveId ? 'page' : undefined"
              @click="selectMobilePanel(panel.id)"
            >
              {{ workbenchPanelTitle(panel) }}
            </button>
          </nav>
          <div class="wb-scroll" :class="{ 'is-focused': !!focusedPanelId, 'is-grid': layoutVariant === 'grid' }">
            <div class="wb-grid" :class="`wb-grid--${layoutVariant}`" :style="gridStyle">
              <WorkbenchPanelView
                v-for="panel in panels"
                v-show="layoutVariant !== 'tabs' || panel.id === mobileActiveId"
                :key="panel.id"
                class="sc-workbench-panel"
                :class="{ 'is-covered': !!focusedPanelId && focusedPanelId !== panel.id && layoutVariant === 'grid' }"
                :style="panelStyle(panel)"
                :panel="panel"
                :minimal-actions-placement="minimalActionsPlacement"
                :title="workbenchPanelTitle(panel)"
                :resolved="resolvedSources[panel.id]"
                :collapsed="localView.collapsed.includes(panel.id) && layoutVariant !== 'tabs'"
                :focused="focusedPanelId === panel.id"
                :spotlighted="spotlight?.panelId === panel.id"
                :popped-out="poppedState(panel)"
                :can-manage="canManage && mode === 'view'"
                :hidden-for-players="panelHiddenForPlayers(panel)"
                :missing-channel="isChannelMissing(panel)"
                :mounted="panelMounted(panel)"
                @toggle-collapse="toggleCollapse(panel)"
                @toggle-focus="toggleFocus(panel)"
                @popout="transient => popoutPanel(panel, transient)"
                @take-back="takeBackPanel(panel)"
                @open-window="openPanelWindow(panel)"
                @toggle-visibility="toggleVisibility(panel)"
                @spotlight="spotlightToAll(panel)"
              />
            </div>
          </div>
        </template>
      </template>
    </div>

    <TheaterFloatingHost
      ref="floatingHostRef"
      host-mode="workbench"
      :world-id="worldId"
      :channel-id="channelId"
      :dock-minimized-characters="isMobile"
      @windows-change="summaries => (floatingSummaries = summaries)"
    />
    <WorkbenchTransferModal
      v-model:show="transferVisible"
      :layout="draft || view?.layout || { version: 1, grid: { columns: 24, rows: 12, gap: 8 }, settings: {}, panels: [] }"
      :world-id="worldId"
      :channel-id="channelId"
      :channel-name="channel.name || ''"
      :can-import="canManage && !isMobile"
      @apply="applyTransfer"
    />
  </div>
</template>

<style scoped>
.sc-workbench-root { display: flex; height: 100%; min-height: 0; flex-direction: column; color: var(--sc-text-primary, inherit); background: var(--sc-workbench-canvas-bg, var(--sc-bg-page, var(--sc-bg-surface))); }
.wb-toolbar { display: flex; flex: none; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 12px; min-height: 44px; padding: 4px 10px; border-bottom: 1px solid var(--sc-border-mute, rgba(127, 127, 127, .2)); background: var(--sc-workbench-toolbar-bg, var(--sc-bg-header, var(--sc-bg-surface))); }
.wb-toolbar__title { display: inline-flex; min-width: 0; align-items: center; gap: 6px; }
.wb-toolbar__title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wb-toolbar__tag { padding: 1px 8px; border-radius: 999px; color: var(--sc-workbench-accent, var(--primary-color, #3b82f6)); background: color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 12%, transparent); font-size: 12px; white-space: nowrap; }
.wb-toolbar__group { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 4px; }
.wb-spotlight { display: flex; flex: none; flex-wrap: wrap; align-items: center; gap: 8px; padding: 4px 12px; border-bottom: 1px solid color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 35%, transparent); color: var(--sc-text-primary, inherit); background: color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 10%, var(--sc-bg-surface, transparent)); font-size: 13px; }
.wb-spotlight > :deep(.n-icon) { color: var(--sc-workbench-accent, var(--primary-color, #3b82f6)); }
.wb-notice { flex: none; border-radius: 0; }
.wb-notice :deep(.n-button) { margin-left: 6px; }
.wb-stage { position: relative; display: flex; flex: 1; min-height: 0; flex-direction: column; }
.wb-scroll { position: relative; flex: 1; min-height: 0; padding: var(--sc-workbench-padding, 10px); overflow: auto; scrollbar-width: none; -ms-overflow-style: none; }
.wb-scroll::-webkit-scrollbar { width: 0; height: 0; display: none; }
.sc-workbench-root[data-sc-minimal-actions-placement='outside'] .wb-scroll { padding: max(var(--sc-workbench-padding, 10px), 8px); }
.wb-scroll.is-focused,
.wb-scroll.is-grid { overflow: hidden; }
.wb-grid--grid { display: grid; height: 100%; min-height: 0; }
.wb-grid--tabs { height: 100%; }
.wb-grid--tabs > .sc-workbench-panel { height: 100%; }
.wb-grid--stack { display: flex; flex-direction: column; gap: 10px; }
.wb-grid--stack > .sc-workbench-panel { height: max(60vh, 320px); flex: none; }
.wb-grid--stack > .sc-workbench-panel.is-collapsed { height: auto; }
.sc-workbench-panel.is-covered { visibility: hidden; }
.wb-tabs { display: flex; flex: none; gap: 4px; padding: 6px 10px 0; overflow-x: auto; scrollbar-width: none; }
.wb-tabs::-webkit-scrollbar { display: none; }
.wb-tabs__item { flex: none; max-width: 40vw; padding: 5px 12px; overflow: hidden; border: 1px solid var(--sc-border-mute, rgba(127, 127, 127, .2)); border-radius: 999px; color: var(--sc-text-secondary, inherit); background: var(--sc-bg-surface, transparent); font-size: 13px; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.wb-tabs__item.is-active { border-color: var(--sc-workbench-accent, var(--primary-color, #3b82f6)); color: var(--sc-text-primary, inherit); background: color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 12%, var(--sc-bg-surface, transparent)); }
.wb-state { display: flex; flex: 1; flex-direction: column; align-items: center; justify-content: center; gap: 10px; padding: 24px; color: var(--sc-text-secondary, #888); text-align: center; }
.wb-state p { margin: 0; }
.wb-state__actions { display: inline-flex; flex-wrap: wrap; justify-content: center; gap: 8px; }
.wb-design-shell { display: grid; flex: 1; min-height: 0; grid-template-columns: minmax(0, 1fr) minmax(260px, 320px); }
.wb-design-shell > .wb-scroll { min-height: 0; }

/* 布局级面板风格；全部基于 --sc-* 令牌，日间 / 夜间自动适配。 */
.sc-workbench-root[data-sc-panel-style='flat'] :deep(.wb-panel:not(.is-minimal)) { border-color: transparent; box-shadow: none; }
.sc-workbench-root[data-sc-panel-style='outlined'] :deep(.wb-panel:not(.is-minimal)) { border-color: var(--sc-border-strong, rgba(127, 127, 127, .45)); box-shadow: none; }
.sc-workbench-root[data-sc-panel-style='outlined'] :deep(.wb-panel:not(.is-minimal) .wb-panel__header) { background: transparent; }
.sc-workbench-root[data-sc-panel-style='glass'] :deep(.wb-panel:not(.is-minimal)) { border-color: color-mix(in srgb, var(--sc-border-mute, #888) 60%, transparent); background: color-mix(in srgb, var(--sc-bg-surface, #fff) 72%, transparent); backdrop-filter: blur(12px) saturate(1.2); -webkit-backdrop-filter: blur(12px) saturate(1.2); }
.sc-workbench-root[data-sc-panel-style='glass'] :deep(.wb-panel:not(.is-minimal) .wb-panel__header) { background: transparent; }

@media (max-width: 699px) {
  .wb-toolbar { padding: 4px 8px; }
  .wb-scroll { padding: 8px; }
}
</style>
