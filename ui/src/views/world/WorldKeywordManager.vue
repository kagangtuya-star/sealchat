<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useWorldGlossaryStore } from '@/stores/worldGlossary'
import { useChatStore } from '@/stores/chat'
import { useUtilsStore } from '@/stores/utils'
import { useDialog, useMessage } from 'naive-ui'
import { triggerBlobDownload } from '@/utils/download'
import { clampTextWithImageTokens } from '@/utils/attachmentMarkdown'
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render'
import { convertPlainWithImagesToTiptap, convertTiptapToPlainWithImages } from '@/utils/keywordFormatConverter'
import { matchText } from '@/utils/pinyinMatch'
import type { WorldKeywordItem, WorldKeywordPayload } from '@/models/worldGlossary'
import { useBreakpoints } from '@vueuse/core'
import { ImageOutline } from '@vicons/ionicons5'
import KeywordDescriptionEditor from './KeywordDescriptionEditor.vue'
import KeywordRichEditor from './KeywordRichEditor.vue'

const DEFAULT_KEYWORD_MAX_LENGTH = 2000
type KeywordDisplayStyle = 'standard' | 'minimal' | 'inherit'
const glossary = useWorldGlossaryStore()
const chat = useChatStore()
const utils = useUtilsStore()
const message = useMessage()
const dialog = useDialog()
const breakpoints = useBreakpoints({ tablet: 768 })
const isMobileLayout = breakpoints.smaller('tablet')
const drawerWidth = computed(() => (isMobileLayout.value ? '100%' : 680))

const drawerVisible = computed({
  get: () => glossary.managerVisible,
  set: (value: boolean) => glossary.setManagerVisible(value),
})

const currentWorldId = computed(() => chat.currentWorldId)
const keywordItems = computed(() => {
  const worldId = currentWorldId.value
  if (!worldId) return []
  const page = glossary.pages[worldId]
  return page?.items || []
})
const filterValue = computed({
  get: () => glossary.searchQuery,
  set: (value: string) => glossary.setSearchQuery(value),
})

const getDescriptionPlainText = (item: WorldKeywordItem) => {
  if (!item?.description) return ''
  if (item.descriptionFormat === 'rich' && isTipTapJson(item.description)) {
    return tiptapJsonToPlainText(item.description)
  }
  return item.description
}

const getDescriptionPreview = (item: WorldKeywordItem) => {
  const text = getDescriptionPlainText(item)
  return text ? clampText(text) : ''
}

const filteredKeywords = computed(() => {
  let items = keywordItems.value
  
  // Filter by category first
  if (categoryFilter.value) {
    items = items.filter((item) => item.category === categoryFilter.value)
  }
  
  // Then filter by search query
  const q = filterValue.value.trim()
  if (!q) return items
  return items.filter((item) => {
    const description = getDescriptionPlainText(item)
    const targets = [item.keyword, ...(item.aliases || []), description]
    return targets.some((target) => matchText(q, target || ''))
  })
})

const PAGE_SIZE = 10
const selectedIds = ref<string[]>([])
const bulkDeleting = ref(false)
const bulkToggleState = ref<'enable' | 'disable' | null>(null)
const currentPage = ref(1)

const pagedKeywords = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return filteredKeywords.value.slice(start, start + PAGE_SIZE)
})

const visibleSelectionCount = computed(() =>
  pagedKeywords.value.filter((item) => selectedIds.value.includes(item.id)).length,
)

const isAllVisibleSelected = computed(
  () => pagedKeywords.value.length > 0 && visibleSelectionCount.value === pagedKeywords.value.length,
)

const isSelectionIndeterminate = computed(
  () => visibleSelectionCount.value > 0 && !isAllVisibleSelected.value,
)

const hasSelection = computed(() => selectedIds.value.length > 0)

const worldDetail = computed(() => {
  const worldId = currentWorldId.value
  if (!worldId) return null
  return chat.worldDetailMap[worldId] || null
})

const canEdit = computed(() => {
  const detail = worldDetail.value
  const role = detail?.memberRole
  const allowMemberEdit = detail?.world?.allowMemberEditKeywords ?? detail?.allowMemberEditKeywords ?? false
  return role === 'owner' || role === 'admin' || (allowMemberEdit && role === 'member')
})

const formModel = reactive({
  keyword: '',
  category: '',
  aliases: '',
  matchMode: 'plain' as 'plain' | 'regex',
  description: '',
  descriptionFormat: 'plain' as 'plain' | 'rich',
  display: 'inherit' as KeywordDisplayStyle,
  isEnabled: true,
})

// Description editor ref
const descriptionEditorRef = ref<InstanceType<typeof KeywordDescriptionEditor> | null>(null)
const richEditorRef = ref<InstanceType<typeof KeywordRichEditor> | null>(null)

// Computed for rich mode toggle
const handleModeSwitch = (toRich: boolean) => {
  const current = formModel.description || ''
  if (toRich) {
    if (!isTipTapJson(current)) {
      const json = convertPlainWithImagesToTiptap(current)
      formModel.description = JSON.stringify(json)
    }
    formModel.descriptionFormat = 'rich'
    return
  }
  if (isTipTapJson(current)) {
    formModel.description = convertTiptapToPlainWithImages(current)
  }
  formModel.descriptionFormat = 'plain'
}

const isRichMode = computed({
  get: () => formModel.descriptionFormat === 'rich',
  set: (value: boolean) => {
    const target = value ? 'rich' : 'plain'
    if (formModel.descriptionFormat === target) return
    handleModeSwitch(value)
  },
})

const categoryFilter = ref('')
const categoryOptions = ref<string[]>([])

// Export modal state
const exportModalVisible = ref(false)
const exportCategoryFilter = ref<string[]>([])

// Category management modal state
const categoryManagerVisible = ref(false)
const categoryStats = ref<Array<{ name: string; count: number }>>([])
const newCategoryName = ref('')

// Import category assignment
const importTargetCategory = ref('')

// Bulk modify category
const bulkCategoryModalVisible = ref(false)
const bulkTargetCategory = ref('')
const bulkDisplayModalVisible = ref(false)
const bulkTargetDisplay = ref<KeywordDisplayStyle>('inherit')

// Drag and drop state
const dragSourceId = ref<string | null>(null)
const dragTargetId = ref<string | null>(null)
const isReordering = ref(false)

const importText = reactive({ content: '' })

const isRegexMatch = computed({
  get: () => formModel.matchMode === 'regex',
  set: (value: boolean) => {
    formModel.matchMode = value ? 'regex' : 'plain'
  },
})

const displayOptions: Array<{ label: string; value: KeywordDisplayStyle }> = [
  { label: '跟随全局', value: 'inherit' },
  { label: '标准', value: 'standard' },
  { label: '极简下划线', value: 'minimal' },
]
const displayLabelMap: Record<KeywordDisplayStyle, string> = {
  inherit: '跟随全局',
  standard: '标准',
  minimal: '极简下划线',
}

const keywordMaxLength = computed(() => utils.config?.keywordMaxLength || DEFAULT_KEYWORD_MAX_LENGTH)

const clampText = (value: string) => value.slice(0, keywordMaxLength.value)
const clampDescription = (value: string) => clampTextWithImageTokens(value, keywordMaxLength.value)

const splitAliases = (value?: string | string[] | null) => {
  if (!value) return []
  const source = Array.isArray(value) ? value : String(value).split(/[，,;；\/、]/)
  return source
    .map((item) => clampText(String(item).trim()))
    .filter(Boolean)
}

const normalizePayloadEntry = (entry: any): WorldKeywordPayload | null => {
  if (!entry) return null
  const keyword = clampText(String(entry.keyword ?? '').trim())
  if (!keyword) return null
  const payload: WorldKeywordPayload = { keyword }
  if (entry.category) {
    payload.category = clampText(String(entry.category).trim())
  }
  const aliases = splitAliases(entry.aliases)
  if (aliases.length) {
    payload.aliases = aliases
  }
  const description = entry.description ?? entry.desc
  const descriptionFormat = entry.descriptionFormat === 'rich' ? 'rich' : entry.descriptionFormat === 'plain' ? 'plain' : undefined
  if (description) {
    if (descriptionFormat === 'rich') {
      const raw = String(description).trim()
      if (raw) {
        payload.description = raw
        payload.descriptionFormat = 'rich'
      }
    } else {
      const text = clampDescription(String(description).trim())
      if (text) payload.description = text
      if (descriptionFormat === 'plain') {
        payload.descriptionFormat = 'plain'
      }
    }
  }
  if (entry.matchMode === 'regex' || entry.matchMode === 'plain') {
    payload.matchMode = entry.matchMode
  }
  if (entry.display === 'minimal' || entry.display === 'standard' || entry.display === 'inherit') {
    payload.display = entry.display
  }
  if (typeof entry.isEnabled === 'boolean') {
    payload.isEnabled = entry.isEnabled
  }
  return payload
}

const parseStructuredImport = (raw: string): WorldKeywordPayload[] => {
  const trimmed = raw.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) {
      return parsed.map((item) => normalizePayloadEntry(item)).filter((item): item is WorldKeywordPayload => Boolean(item))
    }
  } catch (error) {
    // fallthrough to other formats
  }
  const lines = trimmed.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
  if (!lines.length) return []
  const firstLine = lines[0]
  const isMarkdownTable = firstLine.startsWith('|') && firstLine.includes('|')
  const headerKeywords = ['关键词', 'keyword']
  const isHeader = (value?: string | null) => {
    if (!value) return false
    const normalized = value.trim().toLowerCase()
    return headerKeywords.includes(normalized)
  }
  const rows: string[][] = []
  if (isMarkdownTable) {
    lines.forEach((line) => {
      if (!line.includes('|')) return
      const content = line.replace(/^\|/, '').replace(/\|$/, '').trim()
      if (!content) return
      const columns = content.split('|').map((col) => col.trim())
      if (!columns.length) return
      if (columns.every((col) => /^-+$/.test(col.replace(/:/g, '')))) return
      if (isHeader(columns[0])) return
      rows.push(columns)
    })
  } else {
    const delimiter = lines.some((line) => line.includes('|')) ? '|' : ','
    lines.forEach((line, index) => {
      const columns = line.split(delimiter).map((col) => col.trim())
      if (!columns.length) return
      if (index === 0 && isHeader(columns[0])) return
      rows.push(columns)
    })
  }
  return rows
    .map((columns) => {
      const keyword = clampText(columns[0] || '')
      const descriptionRaw = clampDescription(columns[1] || '')
      if (!keyword || !descriptionRaw) {
        return null
      }
      const entry: Partial<WorldKeywordPayload> = {
        keyword,
        description: descriptionRaw,
      }
      if (columns[2]) {
        const aliasList = splitAliases(columns[2])
        if (aliasList.length) entry.aliases = aliasList
      }
      return normalizePayloadEntry(entry)
    })
    .filter((item): item is WorldKeywordPayload => Boolean(item))
}

// Drag and drop handlers
function handleDragStart(e: DragEvent, item: WorldKeywordItem) {
  if (!canEdit.value) return
  dragSourceId.value = item.id
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', item.id)
  }
}

function handleDragEnter(item: WorldKeywordItem) {
  if (!dragSourceId.value || dragSourceId.value === item.id) return
  dragTargetId.value = item.id
}

function handleDragOver(e: DragEvent) {
  e.preventDefault()
}

function handleDragLeave() {
  // Optional: clear target on leave
}

async function handleDrop(targetItem: WorldKeywordItem) {
  const sourceId = dragSourceId.value
  const targetId = targetItem.id
  dragSourceId.value = null
  dragTargetId.value = null

  if (!sourceId || sourceId === targetId) return

  const worldId = currentWorldId.value
  if (!worldId) return

  const items = [...keywordItems.value]
  const sourceIndex = items.findIndex((item) => item.id === sourceId)
  const targetIndex = items.findIndex((item) => item.id === targetId)
  if (sourceIndex === -1 || targetIndex === -1) return

  // Reorder locally first
  const [removed] = items.splice(sourceIndex, 1)
  items.splice(targetIndex, 0, removed)

  // Calculate new sortOrders (descending, highest at top)
  const reorderItems = items.map((item, index) => ({
    id: item.id,
    sortOrder: items.length - index,
  }))

  isReordering.value = true
  try {
    await glossary.reorderKeywords(worldId, reorderItems)
  } catch (error) {
    message.error('排序失败')
  } finally {
    isReordering.value = false
  }
}

function handleDragEnd() {
  dragSourceId.value = null
  dragTargetId.value = null
}

function resetForm() {
  formModel.keyword = ''
  formModel.category = ''
  formModel.aliases = ''
  formModel.matchMode = 'plain'
  formModel.description = ''
  formModel.descriptionFormat = 'plain'
  formModel.display = 'inherit'
  formModel.isEnabled = true
}

function openCreate() {
  const worldId = currentWorldId.value
  if (!worldId) return
  resetForm()
  glossary.openEditor(worldId)
}

function openImportModal() {
  const worldId = currentWorldId.value
  if (!worldId) {
    message.warning('请选择一个世界')
    return
  }
  glossary.openImport(worldId)
}

function openEdit(item: any) {
  const worldId = currentWorldId.value
  if (!worldId) return
  formModel.keyword = clampText(item.keyword)
  formModel.category = item.category || ''
  formModel.aliases = (item.aliases || []).map((alias: string) => clampText(alias)).join(', ')
  formModel.matchMode = item.matchMode
  const descriptionFormat = item.descriptionFormat === 'rich' ? 'rich' : 'plain'
  formModel.description = descriptionFormat === 'rich' ? (item.description || '') : clampDescription(item.description || '')
  formModel.descriptionFormat = descriptionFormat
  formModel.display = item.display || 'inherit'
  formModel.isEnabled = item.isEnabled
  glossary.openEditor(worldId, item)
}

async function submitEditor() {
  const worldId = glossary.editorState.worldId || currentWorldId.value
  if (!worldId) return
  const keyword = clampText(formModel.keyword.trim())
  if (!keyword) {
    message.error('关键词不能为空')
    return
  }
  const aliases = formModel.aliases
    .split(',')
    .map((item: string) => clampText(item.trim()))
    .filter(Boolean)
  const rawDescription = formModel.description?.trim() || ''
  const description =
    rawDescription
      ? formModel.descriptionFormat === 'rich'
        ? rawDescription
        : clampDescription(rawDescription)
      : undefined
  const payload = {
    keyword,
    category: formModel.category?.trim() || undefined,
    aliases,
    matchMode: formModel.matchMode,
    description,
    descriptionFormat: formModel.descriptionFormat,
    display: formModel.display,
    isEnabled: formModel.isEnabled,
  }
  try {
    if (glossary.editorState.keyword) {
      await glossary.editKeyword(worldId, glossary.editorState.keyword.id, payload)
      message.success('已更新术语')
    } else {
      await glossary.createKeyword(worldId, payload)
      message.success('已创建术语')
    }
    glossary.closeEditor()
  } catch (error: any) {
    message.error(error?.message || '保存失败')
  }
}

async function handleDelete(itemId: string) {
  const worldId = currentWorldId.value
  if (!worldId) return
  await glossary.removeKeyword(worldId, itemId)
  message.success('已删除')
  selectedIds.value = selectedIds.value.filter((id) => id !== itemId)
}

async function handleToggle(item: WorldKeywordItem) {
  const worldId = currentWorldId.value
  if (!worldId) return
  await glossary.editKeyword(worldId, item.id, {
    keyword: item.keyword,
    category: item.category,
    aliases: item.aliases,
    matchMode: item.matchMode,
    description: item.description,
    descriptionFormat: item.descriptionFormat || 'plain',
    display: item.display,
    isEnabled: !item.isEnabled,
  })
}

async function handleExport(categoryFilters?: string[]) {
  const worldId = currentWorldId.value
  if (!worldId) return
  // If multiple categories, export each one and merge
  let items: WorldKeywordItem[] = []
  if (categoryFilters && categoryFilters.length > 0) {
    for (const cat of categoryFilters) {
      const catItems = await glossary.exportKeywords(worldId, cat)
      items.push(...catItems)
    }
  } else {
    items = await glossary.exportKeywords(worldId)
  }
  const blob = new Blob([JSON.stringify(items, null, 2)], { type: 'application/json' })
  const worldName = chat.worldMap[worldId]?.name || 'world'
  
  // Optimize filename based on categories
  let filename = `${worldName}-keywords`
  if (categoryFilters && categoryFilters.length > 0) {
    if (categoryFilters.length === 1) {
      filename = `${worldName}-${categoryFilters[0]}-keywords`
    } else {
      filename = `${worldName}-多分类-keywords`
    }
  }
  
  triggerBlobDownload(blob, `${filename}.json`)
  message.success('已导出词库')
  exportModalVisible.value = false
}

function openExportModal() {
  exportCategoryFilter.value = []
  exportModalVisible.value = true
}

async function openCategoryManager() {
  const worldId = currentWorldId.value
  if (!worldId) return
  // Compute category stats from current keywords
  const items = keywordItems.value
  const statsMap = new Map<string, number>()
  items.forEach((item) => {
    const cat = item.category || '(未分类)'
    statsMap.set(cat, (statsMap.get(cat) || 0) + 1)
  })
  categoryStats.value = Array.from(statsMap.entries())
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
  categoryManagerVisible.value = true
}

async function handleBulkRenameCategory(oldName: string, newName: string) {
  const worldId = currentWorldId.value
  if (!worldId || !newName.trim()) return
  const items = keywordItems.value.filter((item) => 
    (oldName === '(未分类)' ? !item.category : item.category === oldName)
  )
  for (const item of items) {
    await glossary.editKeyword(worldId, item.id, {
      keyword: item.keyword,
      category: newName.trim(),
      aliases: item.aliases,
      matchMode: item.matchMode,
      description: item.description,
      descriptionFormat: item.descriptionFormat || 'plain',
      display: item.display,
      isEnabled: item.isEnabled,
    })
  }
  message.success(`已将 ${items.length} 个术语的分类更新为 "${newName}"`)
  // Refresh categories
  categoryOptions.value = await glossary.fetchCategories(worldId)
  await openCategoryManager()
}

async function handleCreateCategory() {
  const worldId = currentWorldId.value
  if (!worldId || !newCategoryName.value.trim()) return
  
  // Simply add to categoryOptions if not exists
  const catName = newCategoryName.value.trim()
  if (!categoryOptions.value.includes(catName)) {
    categoryOptions.value.push(catName)
    categoryOptions.value.sort()
  }
  message.success(`已创建分类 "${catName}"`)
  newCategoryName.value = ''
  // Refresh stats
  await openCategoryManager()
}

async function handleBulkModifyCategory() {
  const worldId = currentWorldId.value
  if (!worldId || !selectedIds.value.length || !bulkTargetCategory.value.trim()) return
  
  const items = keywordItems.value.filter((item) => selectedIds.value.includes(item.id))
  for (const item of items) {
    await glossary.editKeyword(worldId, item.id, {
      keyword: item.keyword,
      category: bulkTargetCategory.value.trim(),
      aliases: item.aliases,
      matchMode: item.matchMode,
      description: item.description,
      descriptionFormat: item.descriptionFormat || 'plain',
      display: item.display,
      isEnabled: item.isEnabled,
    })
  }
  message.success(`已将 ${items.length} 个术语的分类修改为 "${bulkTargetCategory.value}"`)
  // Refresh categories
  categoryOptions.value = await glossary.fetchCategories(worldId)
  bulkCategoryModalVisible.value = false
  bulkTargetCategory.value = ''
  clearSelection()
}

const openBulkDisplayModal = () => {
  if (!hasSelection.value) return
  const first = keywordItems.value.find((item) => selectedIds.value.includes(item.id))
  bulkTargetDisplay.value = (first?.display || 'inherit') as KeywordDisplayStyle
  bulkDisplayModalVisible.value = true
}

async function handleBulkModifyDisplay() {
  const worldId = currentWorldId.value
  if (!worldId || !selectedIds.value.length) return
  try {
    await glossary.setKeywordDisplayBulk(worldId, [...selectedIds.value], bulkTargetDisplay.value)
    const label = displayLabelMap[bulkTargetDisplay.value] || displayLabelMap.inherit
    message.success(`已将 ${selectedIds.value.length} 个术语的显示状态修改为 "${label}"`)
    bulkDisplayModalVisible.value = false
    clearSelection()
  } catch (error: any) {
    message.error(error?.message || '批量更新失败')
  }
}


async function handleDeleteCategory(categoryName: string) {
  const worldId = currentWorldId.value
  if (!worldId) return
  
  const items = keywordItems.value.filter((item) => 
    (categoryName === '(未分类)' ? !item.category : item.category === categoryName)
  )
  
  if (items.length === 0) {
    // No keywords using this category, just remove from options
    categoryOptions.value = categoryOptions.value.filter(c => c !== categoryName)
    message.success(`已删除分类 "${categoryName}"`)
    await openCategoryManager()
    return
  }
  
  // Show dialog to handle keywords
  const d = dialog.warning({
    title: `删除分类 "${categoryName}"`,
    content: `该分类下有 ${items.length} 个术语。删除后这些术语将被设为"未分类"。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      d.loading = true
      // Set all keywords to uncategorized
      for (const item of items) {
        await glossary.editKeyword(worldId, item.id, {
          keyword: item.keyword,
          category: '',
          aliases: item.aliases,
          matchMode: item.matchMode,
          description: item.description,
          descriptionFormat: item.descriptionFormat || 'plain',
          display: item.display,
          isEnabled: item.isEnabled,
        })
      }
      // Remove from options
      categoryOptions.value = categoryOptions.value.filter(c => c !== categoryName)
      message.success(`已删除分类 "${categoryName}"，${items.length} 个术语已设为未分类`)
      await openCategoryManager()
    }
  })
}



async function handleImport(replace = false) {
  const worldId = glossary.importState.worldId || currentWorldId.value
  if (!worldId) return
  try {
    let payloads = parseStructuredImport(importText.content || '')
    if (!payloads.length) {
      message.error('未识别到可导入的数据，请检查格式')
      return
    }
    // Apply target category if specified
    if (importTargetCategory.value) {
      payloads = payloads.map((p) => ({ ...p, category: importTargetCategory.value }))
    }
    await glossary.importKeywords(worldId, payloads, replace)
    message.success('导入完成')
    // Refresh categories
    categoryOptions.value = await glossary.fetchCategories(worldId)
    importTargetCategory.value = ''
  } catch (error: any) {
    message.error(error?.message || '导入失败')
  }
}

// File upload import
const importFileInputRef = ref<HTMLInputElement | null>(null)

const triggerFileImport = () => {
  importFileInputRef.value?.click()
}

const handleFileImport = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input?.files?.[0]
  if (!file) return
  
  try {
    const text = await file.text()
    importText.content = text
    message.success(`已加载文件: ${file.name}`)
  } catch (error) {
    console.error(error)
    message.error('读取文件失败')
  } finally {
    if (input) {
      input.value = ''
    }
  }
}

const clearSelection = () => {
  selectedIds.value = []
}

const handleRowSelection = (keywordId: string, checked: boolean | undefined) => {
  const next = new Set(selectedIds.value)
  if (checked) {
    next.add(keywordId)
  } else {
    next.delete(keywordId)
  }
  selectedIds.value = Array.from(next)
}

const handleSelectAllVisible = (checked: boolean | undefined) => {
  const next = new Set(selectedIds.value)
  const shouldSelect = !!checked
  pagedKeywords.value.forEach((item) => {
    if (shouldSelect) {
      next.add(item.id)
    } else {
      next.delete(item.id)
    }
  })
  selectedIds.value = Array.from(next)
}

const handleBulkDelete = async () => {
  const worldId = currentWorldId.value
  if (!worldId || !selectedIds.value.length) {
    return
  }
  bulkDeleting.value = true
  try {
    await glossary.removeKeywordBulk(worldId, [...selectedIds.value])
    message.success(`已删除 ${selectedIds.value.length} 个术语`)
    clearSelection()
  } catch (error: any) {
    message.error(error?.message || '批量删除失败')
  } finally {
    bulkDeleting.value = false
  }
}

const handleBulkDeleteConfirm = () => {
  if (!canEdit.value || !hasSelection.value) {
    return
  }
  dialog.warning({
    title: '批量删除术语',
    content: `确认删除选中的 ${selectedIds.value.length} 个术语？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => handleBulkDelete(),
  })
}

const handleBulkToggle = async (enabled: boolean) => {
  const worldId = currentWorldId.value
  if (!worldId || !selectedIds.value.length) {
    return
  }
  bulkToggleState.value = enabled ? 'enable' : 'disable'
  try {
    await glossary.setKeywordEnabledBulk(worldId, [...selectedIds.value], enabled)
    message.success(`${enabled ? '已启用' : '已停用'} ${selectedIds.value.length} 个术语`)
    clearSelection()
  } catch (error: any) {
    message.error(error?.message || '批量更新失败')
  } finally {
    bulkToggleState.value = null
  }
}

watch(
  () => drawerVisible.value,
  async (visible) => {
    if (visible) {
      if (currentWorldId.value) {
        glossary.ensureKeywords(currentWorldId.value, { force: true })
        chat.worldDetail(currentWorldId.value)
        // Load categories
        try {
          categoryOptions.value = await glossary.fetchCategories(currentWorldId.value)
        } catch (e) {
          categoryOptions.value = []
        }
      }
      currentPage.value = 1
    } else {
      clearSelection()
      currentPage.value = 1
      categoryFilter.value = ''
    }
  },
)

watch(
  () => currentWorldId.value,
  (worldId) => {
    if (worldId && drawerVisible.value) {
      glossary.ensureKeywords(worldId, { force: true })
    }
    clearSelection()
    currentPage.value = 1
  },
)

onMounted(() => {
  if (currentWorldId.value) {
    glossary.ensureKeywords(currentWorldId.value)
  }
})

watch(keywordItems, (items) => {
  const validIds = new Set(items.map((item) => item.id))
  selectedIds.value = selectedIds.value.filter((id) => validIds.has(id))
})

watch(
  () => filteredKeywords.value.length,
  (length) => {
    const maxPage = Math.max(1, Math.ceil(Math.max(length, 1) / PAGE_SIZE))
    if (currentPage.value > maxPage) {
      currentPage.value = maxPage
    }
  },
)

watch(
  () => filterValue.value,
  () => {
    currentPage.value = 1
  },
)

watch(
  () => ({
    visible: glossary.editorState.visible,
    keyword: glossary.editorState.keyword,
    prefill: glossary.editorState.prefill,
  }),
  (state) => {
    if (!state.visible) {
      resetForm()
      return
    }
    if (state.keyword) {
      const keyword = state.keyword
      formModel.keyword = keyword.keyword
      formModel.category = keyword.category || ''
      formModel.aliases = (keyword.aliases || []).join(', ')
      formModel.matchMode = keyword.matchMode
      const descriptionFormat = keyword.descriptionFormat === 'rich' ? 'rich' : 'plain'
      formModel.description = descriptionFormat === 'rich' ? (keyword.description || '') : clampDescription(keyword.description || '')
      formModel.descriptionFormat = descriptionFormat
      formModel.display = keyword.display || 'inherit'
      formModel.isEnabled = keyword.isEnabled
    } else {
      resetForm()
    }
  },
)

watch(
  () => glossary.quickPrefill,
  (text) => {
    if (!text) return
    if (!glossary.editorState.visible || glossary.editorState.keyword) return
    formModel.keyword = text
    glossary.setQuickPrefill(null)
  },
)

const isEditing = computed(() => Boolean(glossary.editorState.keyword))
const editorVisible = computed({
  get: () => glossary.editorState.visible,
  set: (value: boolean) => {
    if (!value) glossary.closeEditor()
  },
})
const importVisible = computed({
  get: () => glossary.importState.visible,
  set: (value: boolean) => {
    if (!value) glossary.closeImport()
  },
})

watch(
  () => importVisible.value,
  (visible) => {
    if (!visible) {
      importText.content = ''
    }
  },
)

// Alias tags for tag-based input
const aliasesArray = computed({
  get: () => {
    if (!formModel.aliases) return []
    return formModel.aliases.split(',').map(s => s.trim()).filter(Boolean)
  },
  set: (value: string[]) => {
    formModel.aliases = value.join(', ')
  }
})

// Keyboard shortcuts
const handleEditorKeydown = (e: KeyboardEvent) => {
  if (!editorVisible.value) return
  
  // Ctrl+S or Cmd+S to save
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    submitEditor()
  }
  
  // Esc to close (only if not in input focus that handles its own Esc)
  if (e.key === 'Escape') {
    const active = document.activeElement
    const isInTextarea = active?.tagName === 'TEXTAREA'
    if (!isInTextarea) {
      glossary.closeEditor()
    }
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleEditorKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEditorKeydown)
})
</script>

<template>
  <n-drawer v-model:show="drawerVisible" :width="drawerWidth" placement="right" :mask-closable="true">
    <n-drawer-content>
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="drawerVisible = false">
              返回
            </n-button>
            <span>术语词库</span>
          </div>
          <div class="space-x-2 flex items-center">
            <n-button size="tiny" @click="currentWorldId && glossary.ensureKeywords(currentWorldId, { force: true })">刷新</n-button>
          </div>
        </div>
      </template>
      <div class="space-y-4">
        <div class="keyword-manager__filter-row">
          <n-input
            v-model:value="filterValue"
            placeholder="搜索关键词或描述"
            clearable
            size="small"
            style="flex: 1"
          />
          <n-select
            v-model:value="categoryFilter"
            placeholder="全部分类"
            clearable
            size="small"
            style="width: 140px"
            :options="[{ label: '全部分类', value: '' }, ...categoryOptions.map(c => ({ label: c, value: c }))]"
          />
        </div>
        <div v-if="canEdit" class="keyword-manager__toolbar">
          <div class="keyword-manager__selection">
            已选 {{ selectedIds.length }} / {{ filteredKeywords.length }}
            <n-button v-if="hasSelection" size="tiny" text class="ml-1" @click="clearSelection">
              清除选择
            </n-button>
          </div>
          <div class="keyword-manager__actions">
            <div class="keyword-manager__action-group keyword-manager__action-group--primary">
              <n-button size="tiny" type="primary" secondary :disabled="!canEdit || !currentWorldId" @click="openCreate">
                新建术语
              </n-button>
              <n-button size="tiny" tertiary :disabled="!canEdit || !currentWorldId" @click="openImportModal">
                导入
              </n-button>
              <n-button size="tiny" tertiary :disabled="!currentWorldId" @click="openExportModal">
                导出 JSON
              </n-button>
              <n-button size="tiny" tertiary :disabled="!canEdit || !currentWorldId" @click="openCategoryManager">
                分类管理
              </n-button>
            </div>
            <div class="keyword-manager__action-group keyword-manager__action-group--bulk">
              <n-button
                size="tiny"
                tertiary
                type="primary"
                :disabled="!hasSelection"
                :loading="bulkToggleState === 'enable'"
                @click="handleBulkToggle(true)"
              >
                批量启用
              </n-button>
              <n-button
                size="tiny"
                tertiary
                type="warning"
                :disabled="!hasSelection"
                :loading="bulkToggleState === 'disable'"
                @click="handleBulkToggle(false)"
              >
                批量停用
              </n-button>
              <n-button
                size="tiny"
                tertiary
                type="info"
                :disabled="!hasSelection"
                @click="bulkCategoryModalVisible = true"
              >
                修改分类
              </n-button>
              <n-button
                size="tiny"
                tertiary
                type="info"
                :disabled="!hasSelection"
                @click="openBulkDisplayModal"
              >
                修改显示
              </n-button>
              <n-button
                size="tiny"
                tertiary
                type="error"
                :loading="bulkDeleting"
                :disabled="!hasSelection"
                @click="handleBulkDeleteConfirm"
              >
                批量删除
              </n-button>
            </div>
          </div>
        </div>
        <n-alert v-if="!canEdit" type="info" title="仅可查看">
          该世界仅管理员可编辑术语，您当前没有编辑权限。
        </n-alert>
        <n-spin :show="glossary.loadingMap[currentWorldId || '']">
          <template v-if="!isMobileLayout">
            <n-table :single-line="false" size="small">
              <thead>
                <tr>
                  <th style="width: 42px">
                    <n-checkbox
                      :checked="isAllVisibleSelected"
                      :indeterminate="isSelectionIndeterminate"
                      :disabled="!canEdit || !pagedKeywords.length"
                      @update:checked="handleSelectAllVisible"
                    />
                  </th>
                  <th>关键词</th>
                  <th>分类</th>
                  <th>匹配</th>
                  <th>显示</th>
                  <th>状态</th>
                  <th style="width: 120px;">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in pagedKeywords"
                  :key="item.id"
                  :draggable="canEdit"
                  :class="{ 'keyword-drop-target': dragTargetId === item.id, 'keyword-dragging': dragSourceId === item.id }"
                  @dragstart="handleDragStart($event, item)"
                  @dragenter="handleDragEnter(item)"
                  @dragover="handleDragOver"
                  @dragleave="handleDragLeave"
                  @drop="handleDrop(item)"
                  @dragend="handleDragEnd"
                >
                  <td>
                    <n-checkbox
                      :checked="selectedIds.includes(item.id)"
                      :disabled="!canEdit"
                      @update:checked="(checked: boolean) => handleRowSelection(item.id, checked)"
                    />
                  </td>
                  <td>
                    <div class="font-medium">{{ item.keyword }}</div>
                    <div class="text-xs text-gray-500" v-if="item.aliases?.length">别名：{{ item.aliases.join(', ') }}</div>
                    <div class="text-xs text-gray-500" v-if="getDescriptionPreview(item)">{{ getDescriptionPreview(item) }}</div>
                  </td>
                  <td>
                    <n-tag v-if="item.category" size="small" :bordered="false">{{ item.category }}</n-tag>
                    <span v-else class="text-gray-400">-</span>
                  </td>
                  <td>{{ item.matchMode === 'regex' ? '正则' : '文本' }}</td>
                  <td>
                    {{
                      item.display === 'minimal'
                        ? '极简下划线'
                        : item.display === 'standard'
                          ? '标准'
                          : '跟随全局'
                    }}
                  </td>
                  <td>
                    <n-tag size="small" :type="item.isEnabled ? 'success' : 'default'">
                      {{ item.isEnabled ? '启用' : '关闭' }}
                    </n-tag>
                  </td>
                  <td>
                    <n-space size="small">
                      <n-button size="tiny" text :disabled="!canEdit" @click="openEdit(item)">编辑</n-button>
                      <n-button size="tiny" text :disabled="!canEdit" @click="handleToggle(item)">
                        {{ item.isEnabled ? '停用' : '启用' }}
                      </n-button>
                      <n-popconfirm v-if="canEdit" @positive-click="handleDelete(item.id)">
                        <template #trigger>
                          <n-button size="tiny" text type="error">删除</n-button>
                        </template>
                        确认删除该术语？
                      </n-popconfirm>
                    </n-space>
                  </td>
                </tr>
                <tr v-if="!filteredKeywords.length">
                  <td colspan="7" class="text-center text-gray-400">暂无数据</td>
                </tr>
              </tbody>
            </n-table>
          </template>
          <template v-else>
            <div class="keyword-mobile-simple-list">
              <div v-for="item in pagedKeywords" :key="item.id" class="keyword-mobile-simple-row">
                <div class="keyword-mobile-simple-main">
                  <n-checkbox
                    :checked="selectedIds.includes(item.id)"
                    :disabled="!canEdit"
                    @update:checked="(checked: boolean) => handleRowSelection(item.id, checked)"
                  />
                  <span class="keyword-mobile-simple-text">{{ item.keyword }}</span>
                </div>
                <div class="keyword-mobile-simple-actions">
                  <n-button size="tiny" text :disabled="!canEdit" @click="openEdit(item)">编辑</n-button>
                  <n-popconfirm v-if="canEdit" @positive-click="handleDelete(item.id)">
                    <template #trigger>
                      <n-button size="tiny" text type="error">删除</n-button>
                    </template>
                    确认删除该术语？
                  </n-popconfirm>
                </div>
              </div>
              <div v-if="!filteredKeywords.length" class="keyword-mobile-empty">暂无数据</div>
            </div>
          </template>
        </n-spin>
        <div class="keyword-manager__pagination" v-if="filteredKeywords.length > PAGE_SIZE">
          <n-pagination
            size="small"
            :item-count="filteredKeywords.length"
            :page-size="PAGE_SIZE"
            :page="currentPage"
            @update:page="currentPage = $event"
          />
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>

  <n-modal
    v-model:show="editorVisible"
    preset="card"
    :title="isEditing ? '编辑术语' : '新增术语'"
    class="keyword-editor-modal"
  >
    <n-form label-placement="top" class="keyword-editor-form" size="small">
      <div class="keyword-editor__row keyword-editor__row--compact">
        <n-form-item label="关键词" required class="keyword-editor__field keyword-editor__field--keyword" :show-feedback="false">
          <n-input v-model:value="formModel.keyword" placeholder="必填" :maxlength="keywordMaxLength" show-count />
        </n-form-item>
      </div>
      <div class="keyword-editor__row keyword-editor__row--compact">
        <n-form-item label="别名" class="keyword-editor__field keyword-editor__field--alias" :show-feedback="false">
          <n-dynamic-tags v-model:value="aliasesArray" :max="10" />
        </n-form-item>
      </div>
      <div class="keyword-editor__row keyword-editor__row--compact">
        <n-form-item label="分类" class="keyword-editor__field keyword-editor__field--category" :show-feedback="false">
          <n-select
            v-model:value="formModel.category"
            :options="categoryOptions.map(c => ({ label: c, value: c }))"
            placeholder="选择或输入分类（可选）"
            clearable
            filterable
            tag
          />
        </n-form-item>
      </div>
      <div class="keyword-editor__row keyword-editor__toggles">
        <div class="keyword-toggle">
          <span class="keyword-toggle__label">正则匹配</span>
          <n-switch v-model:value="isRegexMatch">
            <template #checked>正则</template>
            <template #unchecked>文本</template>
          </n-switch>
        </div>
        <div class="keyword-toggle">
          <span class="keyword-toggle__label">显示样式</span>
          <n-select
            v-model:value="formModel.display"
            :options="displayOptions"
            size="small"
            class="keyword-display-select"
          />
        </div>
        <div class="keyword-toggle">
          <span class="keyword-toggle__label">启用</span>
          <n-switch v-model:value="formModel.isEnabled">
            <template #checked>启用</template>
            <template #unchecked>停用</template>
          </n-switch>
        </div>
      </div>
      <div class="keyword-editor__row keyword-editor__description">
        <n-form-item path="description" :show-feedback="false">
          <template #label>
            <div class="keyword-description-label">
              <span>术语描述 / 详细说明</span>
              <div class="keyword-description-label__actions" @click.stop.prevent @mousedown.stop.prevent>
                <button
                  v-if="!isRichMode"
                  class="keyword-description-label__upload"
                  type="button"
                  @click="descriptionEditorRef?.triggerFileSelect()"
                  title="插入图片"
                >
                  <n-icon :component="ImageOutline" size="14" />
                </button>
                <n-tooltip trigger="hover" :delay="300">
                  <template #trigger>
                    <n-switch
                      v-model:value="isRichMode"
                      size="small"
                      class="keyword-rich-switch"
                    >
                      <template #checked>富文本</template>
                      <template #unchecked>纯文本</template>
                    </n-switch>
                  </template>
                  切换编辑模式（富文本支持格式化）
                </n-tooltip>
              </div>
            </div>
          </template>
          <KeywordDescriptionEditor
            v-if="!isRichMode"
            ref="descriptionEditorRef"
            v-model="formModel.description"
            :maxlength="keywordMaxLength"
          />
          <KeywordRichEditor
            v-else
            ref="richEditorRef"
            v-model="formModel.description"
            :maxlength="keywordMaxLength"
          />
        </n-form-item>
      </div>
    </n-form>
    <template #action>
      <div class="keyword-editor__action-row">
        <span class="keyboard-hint">Ctrl+S 保存 · Esc 取消</span>
        <n-space>
          <n-button @click="glossary.closeEditor()">取消</n-button>
          <n-button type="primary" @click="submitEditor">保存</n-button>
        </n-space>
      </div>
    </template>
  </n-modal>

  <n-modal v-model:show="importVisible" preset="card" title="导入术语" style="width: 520px">
    <n-alert type="info" class="mb-3">
      <p class="import-hint-title">支持以下格式：</p>
      <ul class="import-hint-list">
        <li>JSON 数组（推荐）：可直接粘贴导出的文件</li>
        <li>CSV：每行 “关键词,描述[,别名]”</li>
        <li>管道分隔：“关键词|描述[|别名]”</li>
        <li>Markdown 表格：前三列依次为关键词、描述、别名（别名可留空）</li>
      </ul>
      <p class="import-hint-desc">别名为可选项，可用逗号/顿号/分号分隔，留空则忽略。</p>
    </n-alert>
    <div class="import-file-upload mb-3">
      <n-button size="small" @click="triggerFileImport">
        从文件导入
      </n-button>
      <input
        ref="importFileInputRef"
        type="file"
        accept=".json,.csv,.txt"
        class="import-file-input"
        @change="handleFileImport"
      />
    </div>
    <n-input
      v-model:value="importText.content"
      type="textarea"
      :autosize="{ minRows: 8, maxRows: 16 }"
      placeholder='[\n  { "keyword": "阿瓦隆", "description": "古老之城" }\n]'
      class="import-textarea"
    />
    <div class="mt-3">
      <n-form-item label="导入到分类（可选）" :show-feedback="false">
        <n-select
          v-model:value="importTargetCategory"
          :options="[{ label: '保持原分类', value: '' }, ...categoryOptions.map(c => ({ label: c, value: c }))]"
          placeholder="保持原分类或指定目标分类"
          clearable
          filterable
          tag
        />
      </n-form-item>
    </div>
    <template #action>
      <n-space>
        <n-button text @click="glossary.closeImport()">取消</n-button>
        <n-button :loading="glossary.importState.processing" @click="handleImport(false)">追加</n-button>
        <n-button type="primary" :loading="glossary.importState.processing" @click="handleImport(true)">替换</n-button>
      </n-space>
    </template>
    <div v-if="glossary.importState.lastStats" class="text-xs text-gray-500 mt-2">
      导入结果：新增 {{ glossary.importState.lastStats.created }}，更新 {{ glossary.importState.lastStats.updated }}，跳过 {{ glossary.importState.lastStats.skipped }}
    </div>
  </n-modal>

  <!-- Export Modal -->
  <n-modal v-model:show="exportModalVisible" preset="card" title="导出术语" style="width: 420px">
    <n-form-item label="选择要导出的分类" :show-feedback="false">
      <n-select
        v-model:value="exportCategoryFilter"
        :options="categoryOptions.map(c => ({ label: c, value: c }))"
        placeholder="全部分类（留空导出全部）"
        multiple
        clearable
      />
    </n-form-item>
    <template #action>
      <n-space>
        <n-button text @click="exportModalVisible = false">取消</n-button>
        <n-button type="primary" @click="handleExport(exportCategoryFilter)">导出 JSON</n-button>
      </n-space>
    </template>
  </n-modal>

  <!-- Category Manager Modal -->
  <n-modal v-model:show="categoryManagerVisible" preset="card" title="分类管理" style="width: 520px">
    <div class="mb-4">
      <n-form-item label="创建新分类" :show-feedback="false">
        <div style="display: flex; gap: 8px">
          <n-input
            v-model:value="newCategoryName"
            placeholder="输入新分类名称"
            @keyup.enter="handleCreateCategory"
          />
          <n-button type="primary" :disabled="!newCategoryName.trim()" @click="handleCreateCategory">
            创建
          </n-button>
        </div>
      </n-form-item>
    </div>
    <n-divider style="margin: 12px 0" />
    <div class="category-manager__list">
      <div v-for="stat in categoryStats" :key="stat.name" class="category-manager__item">
        <div class="category-manager__info">
          <n-tag :bordered="false" size="small">{{ stat.name }}</n-tag>
          <span class="category-manager__count">{{ stat.count }} 个术语</span>
        </div>
        <div class="category-manager__actions">
          <n-popover trigger="click" placement="bottom">
            <template #trigger>
              <n-button size="tiny" tertiary>重命名</n-button>
            </template>
            <div style="width: 200px">
              <n-input
                :default-value="stat.name === '(未分类)' ? '' : stat.name"
                placeholder="新分类名"
                @keyup.enter="(e: KeyboardEvent) => { handleBulkRenameCategory(stat.name, (e.target as HTMLInputElement).value) }"
              />
              <div class="text-xs text-gray-400 mt-1">按回车确认</div>
            </div>
          </n-popover>
          <n-button size="tiny" tertiary type="error" @click="handleDeleteCategory(stat.name)">
            删除
          </n-button>
        </div>
      </div>
      <div v-if="!categoryStats.length" class="text-center text-gray-400 py-4">暂无分类</div>
    </div>
    <template #action>
      <n-button @click="categoryManagerVisible = false">关闭</n-button>
    </template>
  </n-modal>

  <!-- Bulk Modify Category Modal -->
  <n-modal v-model:show="bulkCategoryModalVisible" preset="card" title="批量修改分类" style="width: 420px">
    <div class="mb-2 text-sm text-gray-500">将为 {{ selectedIds.length }} 个术语修改分类</div>
    <n-form-item label="目标分类" :show-feedback="false">
      <n-select
        v-model:value="bulkTargetCategory"
        :options="categoryOptions.map(c => ({ label: c, value: c }))"
        placeholder="选择或输入目标分类"
        filterable
        tag
        clearable
      />
    </n-form-item>
    <template #action>
      <n-space>
        <n-button text @click="bulkCategoryModalVisible = false">取消</n-button>
        <n-button type="primary" :disabled="!bulkTargetCategory.trim()" @click="handleBulkModifyCategory">确认修改</n-button>
      </n-space>
    </template>
  </n-modal>

  <!-- Bulk Modify Display Modal -->
  <n-modal v-model:show="bulkDisplayModalVisible" preset="card" title="批量修改显示状态" style="width: 420px">
    <div class="mb-2 text-sm text-gray-500">将为 {{ selectedIds.length }} 个术语修改显示状态</div>
    <n-form-item label="显示状态" :show-feedback="false">
      <n-select
        v-model:value="bulkTargetDisplay"
        :options="displayOptions"
      />
    </n-form-item>
    <template #action>
      <n-space>
        <n-button text @click="bulkDisplayModalVisible = false">取消</n-button>
        <n-button type="primary" @click="handleBulkModifyDisplay">确认修改</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.keyword-drop-target {
  background-color: rgba(24, 160, 88, 0.15) !important;
  border-top: 2px solid var(--n-primary-color, #18a058) !important;
}

.keyword-dragging {
  opacity: 0.5;
}

tr[draggable="true"] {
  cursor: grab;
}

tr[draggable="true"]:active {
  cursor: grabbing;
}

.keyword-editor-form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  flex: 1;
  min-height: 0;
}

.keyword-editor-modal :deep(.n-modal) {
  height: calc(100vh - 48px);
  max-height: calc(100vh - 48px);
}

.keyword-editor-modal :deep(.n-modal-body-wrapper) {
  height: 100%;
  max-height: 100%;
  display: flex;
}

.keyword-editor-modal :deep(.n-card) {
  width: 600px;
  max-width: 92vw;
  height: 100%;
  max-height: 100%;
  display: flex;
  flex-direction: column;
}

.keyword-editor-modal :deep(.n-card__content) {
  flex: 1;
  display: flex;
  min-height: 0;
  overflow: hidden;
}

.keyword-editor__row {
  width: 100%;
}

.keyword-editor__row--compact :deep(.n-form-item) {
  margin-bottom: 0;
}

.keyword-editor__field :deep(.n-input) {
  width: 100%;
}

.keyword-editor__field--keyword :deep(.n-input) {
  font-size: 16px;
  font-weight: 600;
}

.keyword-editor__toggles {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: center;
}

.keyword-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 140px;
}

.keyword-display-select {
  min-width: 160px;
}

.keyword-toggle__label {
  font-size: 13px;
  color: #4b5563;
}

.keyword-editor__description :deep(.n-input) {
  font-size: 14px;
  line-height: 1.5;
}

.keyword-editor__description {
  flex: 0 0 auto;
  display: flex;
  min-height: 0;
  --desc-editor-height: 360px;
}

.keyword-editor__description :deep(.n-form-item) {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.keyword-editor__description :deep(.n-form-item-blank) {
  flex: 1;
  min-height: 0;
  height: var(--desc-editor-height);
  max-height: var(--desc-editor-height);
}

.keyword-editor__description :deep(.desc-editor) {
  height: 100%;
  max-height: 100%;
}

.keyword-description-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 0.5rem;
}

.keyword-description-label span {
  flex: 1;
}

.keyword-description-label__actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.keyword-rich-switch {
  font-size: 11px;
}

.keyword-description-label__upload {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  color: #64748b;
  transition: background 0.15s, color 0.15s;
}

.keyword-description-label__upload:hover {
  background: rgba(0, 0, 0, 0.06);
  color: #1e293b;
}

:root[data-display-palette='night'] .keyword-description-label__upload {
  color: rgba(248, 250, 252, 0.6);
}

:root[data-display-palette='night'] .keyword-description-label__upload:hover {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(248, 250, 252, 0.9);
}

:root[data-custom-theme='true'] .keyword-description-label__upload {
  color: var(--sc-text-secondary);
}

:root[data-custom-theme='true'] .keyword-description-label__upload:hover {
  background: var(--sc-bg-elevated);
  color: var(--sc-text-primary);
}

.keyword-manager__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem;
  font-size: 12px;
  color: #6b7280;
}

.keyword-manager__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: center;
}

.keyword-manager__action-group {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  align-items: center;
}

.keyword-mobile-simple-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.keyword-mobile-simple-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.65rem 0.2rem;
  border-bottom: 1px solid rgba(148, 163, 184, 0.3);
}

.keyword-mobile-simple-row:last-child {
  border-bottom: none;
}

:root[data-display-palette='night'] .keyword-mobile-simple-row {
  border-bottom-color: rgba(148, 163, 184, 0.2);
}

.keyword-mobile-simple-main {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex: 1;
  min-width: 0;
}

.keyword-mobile-simple-text {
  font-weight: 600;
  font-size: 14px;
  word-break: break-all;
  color: var(--sc-text-primary, #111827);
}

.keyword-mobile-simple-actions {
  display: flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.keyword-mobile-empty {
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
  padding: 0.5rem 0;
}

.keyword-manager__pagination {
  display: flex;
  justify-content: center;
  margin-top: 0.75rem;
}

.import-hint-title {
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.import-hint-list {
  margin: 0.25rem 0 0.4rem;
  padding-left: 1.1rem;
  font-size: 12px;
  color: #4b5563;
}

.import-hint-list li {
  list-style: disc;
  margin-bottom: 0.15rem;
}

.import-hint-desc {
  margin: 0;
  font-size: 12px;
  color: #4b5563;
}

.import-file-upload {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.import-file-input {
  display: none;
}

.import-textarea :deep(textarea) {
  max-height: 300px;
  overflow-y: auto !important;
}

@media (max-width: 767px) {
  .keyword-editor-form {
    gap: 0.5rem;
  }

  .keyword-manager__toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .keyword-manager__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .keyword-manager__action-group {
    width: 100%;
    justify-content: flex-start;
  }

  .keyword-manager__quick-actions {
    width: 100%;
  }

  .keyword-editor__toggles {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.4rem;
    align-items: center;
  }

  .keyword-toggle {
    min-width: 0;
    width: 100%;
    justify-content: space-between;
  }

  .keyword-toggle__label {
    font-size: 12px;
  }

  .keyword-editor__row--compact :deep(.n-form-item) {
    margin-bottom: 0.2rem;
  }

  .keyword-editor-modal :deep(.n-modal) {
    height: calc(100dvh - 12px);
    max-height: calc(100dvh - 12px);
  }

  .keyword-editor-modal :deep(.n-card) {
    width: 96vw;
    max-width: 96vw;
  }

  .keyword-editor-modal :deep(.n-card__content) {
    padding: 12px;
  }

  .keyword-editor__description :deep(.desc-editor__input) {
    min-height: 0;
  }

  .keyword-editor__description {
    --desc-editor-height: 220px;
  }
}

/* Filter row with search and category dropdown */
.keyword-manager__filter-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

/* Category Manager Styles */
.category-manager__list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.category-manager__item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
}

.category-manager__info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.category-manager__count {
  font-size: 13px;
  color: #6b7280;
}

.category-manager__actions {
  display: flex;
  gap: 6px;
}
</style>

<style scoped>
/* Action Row Styles */
.keyword-editor__action-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 1rem;
}

.keyboard-hint {
  font-size: 12px;
  color: #94a3b8;
}

/* Reduce action area padding to save space */
.keyword-editor-form :deep(.n-card__action) {
  padding-top: 12px !important;
  padding-bottom: 12px !important;
}

@media (max-width: 767px) {
  .keyword-editor__action-row {
    flex-direction: row;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  
  .keyboard-hint {
    display: none;
  }
  
  .keyword-editor-form :deep(.n-card__action) {
    padding: 8px 12px !important;
  }

  .keyword-editor__action-row :deep(.n-button) {
    height: 28px;
    padding: 0 10px;
    font-size: 12px;
  }
}
</style>
