<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useExternalGlossaryStore } from '@/stores/externalGlossary'
import { triggerBlobDownload } from '@/utils/download'
import type { ExternalGlossaryLibraryItem, ExternalGlossaryTermItem } from '@/models/externalGlossary'
import type { KeywordCategoryInfo, WorldKeywordPayload } from '@/models/worldGlossary'
import { clampTextWithImageTokens } from '@/utils/attachmentMarkdown'
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render'
import { convertPlainWithImagesToTiptap, convertTiptapToPlainWithImages } from '@/utils/keywordFormatConverter'
import { matchText } from '@/utils/pinyinMatch'
import KeywordDescriptionEditor from '@/views/world/KeywordDescriptionEditor.vue'
import KeywordRichEditor from '@/views/world/KeywordRichEditor.vue'

const store = useExternalGlossaryStore()
const message = useMessage()
const dialog = useDialog()

const DEFAULT_KEYWORD_MAX_LENGTH = 2000
const PAGE_SIZE = 10
type KeywordDisplayStyle = 'standard' | 'minimal' | 'inherit'

const libraryQuery = ref('')
const termQuery = ref('')
const termCategoryFilter = ref<string | null>(null)
const currentTermPage = ref(1)
const selectedLibraryIds = ref<string[]>([])
const selectedTermIds = ref<string[]>([])

const libraryModalVisible = ref(false)
const libraryEditingId = ref('')
const editorVisible = ref(false)
const importLibraryVisible = ref(false)
const importTermsVisible = ref(false)
const categoryVisible = ref(false)
const categoryPriorityVisible = ref(false)
const categoryPrioritySaving = ref(false)
const categoryPriorityDragSource = ref<string | null>(null)
const categoryPriorityDragTarget = ref<string | null>(null)
const termModalVisible = ref(false)
const termEditingId = ref('')
const termImportReplace = ref(false)
const libraryImportReplace = ref(false)
const libraryImportFileInputRef = ref<HTMLInputElement | null>(null)

const libraryForm = reactive({
  name: '',
  description: '',
  isEnabled: true,
  sortOrder: 0,
})

const termForm = reactive({
  keyword: '',
  category: '',
  aliases: [] as string[],
  matchMode: 'plain' as 'plain' | 'regex',
  description: '',
  descriptionFormat: 'plain' as 'plain' | 'rich',
  display: 'inherit' as KeywordDisplayStyle,
  isEnabled: true,
  sortOrder: 0,
})

const libraryImportForm = reactive({
  name: '',
  description: '',
  isEnabled: true,
  sortOrder: 0,
  content: '',
})

const termImportForm = reactive({
  content: '',
})

const categoryDraft = ref('')
const categoryPriorityDrafts = reactive<Record<string, number>>({})
const categoryPriorityItems = ref<KeywordCategoryInfo[]>([])

const selectedLibrary = computed(() => {
  const libraryId = store.activeLibraryId
  return (store.libraryPage?.items || []).find((item) => item.id === libraryId) || null
})

const libraryItems = computed(() => store.libraryPage?.items || [])
const filteredLibraries = computed(() => {
  const query = libraryQuery.value.trim()
  if (!query) return libraryItems.value
  return libraryItems.value.filter((item) => matchText(query, `${item.name} ${item.description}`))
})

const categoryOptions = computed(() => (selectedLibrary.value ? store.categoriesMap[selectedLibrary.value.id] || [] : []))
const termItems = computed(() => store.currentTerms)
const filteredTerms = computed(() => {
  let items = termItems.value
  if (termCategoryFilter.value) {
    items = items.filter((item) => item.category === termCategoryFilter.value)
  }
  const query = termQuery.value.trim()
  if (!query) return items
  return items.filter((item) => {
    const description = getDescriptionPlainText(item)
    return [item.keyword, ...(item.aliases || []), item.category, description].some((value) => matchText(query, value || ''))
  })
})
const pagedTerms = computed(() => {
  const start = (currentTermPage.value - 1) * PAGE_SIZE
  return filteredTerms.value.slice(start, start + PAGE_SIZE)
})
const visibleSelectedTermCount = computed(() =>
  pagedTerms.value.filter((item) => selectedTermIds.value.includes(item.id)).length,
)
const isAllVisibleTermsSelected = computed(
  () => pagedTerms.value.length > 0 && visibleSelectedTermCount.value === pagedTerms.value.length,
)
const isTermSelectionIndeterminate = computed(
  () => visibleSelectedTermCount.value > 0 && !isAllVisibleTermsSelected.value,
)

const keywordMaxLength = computed(() => DEFAULT_KEYWORD_MAX_LENGTH)

const clampText = (value = '') => value.slice(0, keywordMaxLength.value)
const clampDescription = (value = '') => clampTextWithImageTokens(value, keywordMaxLength.value)
const resolveErrorMessage = (error: any, fallback: string) => error?.response?.data?.message || error?.message || fallback

const displayOptions: Array<{ label: string; value: KeywordDisplayStyle }> = [
  { label: '跟随全局', value: 'inherit' },
  { label: '标准', value: 'standard' },
  { label: '极简下划线', value: 'minimal' },
]

const getDescriptionPlainText = (item: { description?: string; descriptionFormat?: 'plain' | 'rich' }) => {
  if (!item?.description) return ''
  if (item.descriptionFormat === 'rich' && isTipTapJson(item.description)) {
    return tiptapJsonToPlainText(item.description)
  }
  return item.description
}

const isRichMode = computed({
  get: () => termForm.descriptionFormat === 'rich',
  set: (value: boolean) => {
    if (value && termForm.descriptionFormat !== 'rich') {
      const current = termForm.description || ''
      termForm.description = isTipTapJson(current) ? current : JSON.stringify(convertPlainWithImagesToTiptap(current))
      termForm.descriptionFormat = 'rich'
      return
    }
    if (!value && termForm.descriptionFormat !== 'plain') {
      termForm.description = isTipTapJson(termForm.description)
        ? convertTiptapToPlainWithImages(termForm.description)
        : termForm.description
      termForm.descriptionFormat = 'plain'
    }
  },
})

function resetLibraryForm(item?: ExternalGlossaryLibraryItem | null) {
  libraryEditingId.value = item?.id || ''
  libraryForm.name = item?.name || ''
  libraryForm.description = item?.description || ''
  libraryForm.isEnabled = item?.isEnabled ?? true
  libraryForm.sortOrder = item?.sortOrder || 0
}

function resetTermForm(item?: ExternalGlossaryTermItem | null) {
  termEditingId.value = item?.id || ''
  termForm.keyword = item?.keyword || ''
  termForm.category = item?.category || ''
  termForm.aliases = [...(item?.aliases || [])]
  termForm.matchMode = item?.matchMode || 'plain'
  termForm.description = item?.description || ''
  termForm.descriptionFormat = item?.descriptionFormat === 'rich' ? 'rich' : 'plain'
  termForm.display = item?.display || 'inherit'
  termForm.isEnabled = item?.isEnabled ?? true
  termForm.sortOrder = item?.sortOrder || 0
}

function buildTermPayload(): WorldKeywordPayload {
  return {
    keyword: clampText(termForm.keyword.trim()),
    category: clampText(termForm.category.trim()),
    aliases: termForm.aliases.map((alias) => clampText(alias.trim())).filter(Boolean),
    matchMode: termForm.matchMode,
    description: termForm.descriptionFormat === 'rich'
      ? termForm.description
      : clampDescription(termForm.description.trim()),
    descriptionFormat: termForm.descriptionFormat,
    display: termForm.display,
    isEnabled: termForm.isEnabled,
    sortOrder: termForm.sortOrder,
  }
}

function parseImportContent(raw: string): WorldKeywordPayload[] {
  const trimmed = raw.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) {
      return parsed
        .map((item) => normalizeImportEntry(item))
        .filter((item): item is WorldKeywordPayload => Boolean(item))
    }
  } catch {
    // ignore
  }
  return trimmed
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [keyword, description = '', aliases = '', category = ''] = line.split(/[|,]/).map((part) => part.trim())
      return normalizeImportEntry({
        keyword,
        description,
        aliases: aliases ? aliases.split(/[，,;；/、]/).map((item) => item.trim()) : [],
        category,
      })
    })
    .filter((item): item is WorldKeywordPayload => Boolean(item))
}

function normalizeImportEntry(entry: any): WorldKeywordPayload | null {
  if (!entry) return null
  const keyword = clampText(String(entry.keyword ?? '').trim())
  if (!keyword) return null
  const descriptionFormat = entry.descriptionFormat === 'rich' ? 'rich' : 'plain'
  return {
    keyword,
    category: clampText(String(entry.category ?? '').trim()),
    aliases: Array.isArray(entry.aliases)
      ? entry.aliases.map((item: string) => clampText(String(item).trim())).filter(Boolean)
      : [],
    matchMode: entry.matchMode === 'regex' ? 'regex' : 'plain',
    description: descriptionFormat === 'rich'
      ? String(entry.description ?? '').trim()
      : clampDescription(String(entry.description ?? '').trim()),
    descriptionFormat,
    display: entry.display === 'minimal' || entry.display === 'standard' || entry.display === 'inherit'
      ? entry.display
      : 'inherit',
    isEnabled: typeof entry.isEnabled === 'boolean' ? entry.isEnabled : true,
    sortOrder: Number.isFinite(Number(entry.sortOrder)) ? Number(entry.sortOrder) : undefined,
  }
}

function normalizeLibraryImportMeta(entry: any) {
  return {
    name: String(entry?.name ?? '').trim(),
    description: String(entry?.description ?? '').trim(),
    isEnabled: typeof entry?.isEnabled === 'boolean' ? entry.isEnabled : true,
    sortOrder: Number.isFinite(Number(entry?.sortOrder)) ? Number(entry.sortOrder) : 0,
  }
}

function parseLibraryImportContent(raw: string) {
  const trimmed = raw.trim()
  if (!trimmed) {
    return {
      library: null as ReturnType<typeof normalizeLibraryImportMeta> | null,
      items: [] as WorldKeywordPayload[],
    }
  }
  try {
    const parsed = JSON.parse(trimmed)
    if (parsed && !Array.isArray(parsed) && Array.isArray(parsed.items)) {
      return {
        library: parsed.library ? normalizeLibraryImportMeta(parsed.library) : null,
        items: parsed.items
          .map((item: any) => normalizeImportEntry(item))
          .filter((item: WorldKeywordPayload | null): item is WorldKeywordPayload => Boolean(item)),
      }
    }
  } catch {
    // ignore
  }
  return {
    library: null,
    items: parseImportContent(raw),
  }
}

async function ensureSelectedLibraryResources() {
  const library = selectedLibrary.value
  if (!library) return
  await Promise.all([
    store.ensureTerms(library.id, { force: true }),
    store.ensureCategories(library.id, { force: true }),
    store.ensureCategoryInfos(library.id, { force: true }),
  ])
}

async function refreshAll() {
  await store.ensureLibraries({ force: true })
  await ensureSelectedLibraryResources()
}

function handleSelectLibrary(libraryId: string) {
  store.setActiveLibrary(libraryId)
}

async function handleToggleLibrary(item: ExternalGlossaryLibraryItem) {
  try {
    await store.editLibrary(item.id, {
      name: item.name,
      description: item.description,
      isEnabled: !item.isEnabled,
      sortOrder: item.sortOrder,
    })
    message.success(item.isEnabled ? '术语库已停用' : '术语库已启用')
  } catch (error) {
    message.error(resolveErrorMessage(error, '更新术语库失败'))
  }
}

async function handleSaveLibrary() {
  try {
    const payload = {
      name: libraryForm.name.trim(),
      description: libraryForm.description.trim(),
      isEnabled: libraryForm.isEnabled,
      sortOrder: libraryForm.sortOrder,
    }
    if (!payload.name) {
      message.warning('术语库名称不能为空')
      return
    }
    if (libraryEditingId.value) {
      await store.editLibrary(libraryEditingId.value, payload)
      message.success('术语库已更新')
    } else {
      const item = await store.createLibrary(payload)
      store.setActiveLibrary(item.id)
      message.success('术语库已创建')
    }
    libraryModalVisible.value = false
  } catch (error) {
    message.error(resolveErrorMessage(error, '保存术语库失败'))
  }
}

function openCreateLibrary() {
  resetLibraryForm(null)
  libraryModalVisible.value = true
}

function openEditLibrary(item: ExternalGlossaryLibraryItem) {
  resetLibraryForm(item)
  libraryModalVisible.value = true
}

async function openLibraryEditor(item: ExternalGlossaryLibraryItem) {
  try {
    handleSelectLibrary(item.id)
    editorVisible.value = true
    await ensureSelectedLibraryResources()
  } catch (error) {
    message.error(resolveErrorMessage(error, '打开术语库编辑器失败'))
  }
}

function handleDeleteLibrary(item: ExternalGlossaryLibraryItem) {
  dialog.warning({
    title: '删除术语库',
    content: `确认删除“${item.name}”？已绑定世界会同步失效。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.removeLibrary(item.id)
        message.success('术语库已删除')
      } catch (error) {
        message.error(resolveErrorMessage(error, '删除术语库失败'))
      }
    },
  })
}

async function handleBulkLibraryState(enabled: boolean) {
  const items = libraryItems.value.filter((item) => selectedLibraryIds.value.includes(item.id))
  if (!items.length) return
  try {
    await Promise.all(items.map((item) => store.editLibrary(item.id, {
      name: item.name,
      description: item.description,
      isEnabled: enabled,
      sortOrder: item.sortOrder,
    })))
    message.success(enabled ? '已批量启用术语库' : '已批量停用术语库')
  } catch (error) {
    message.error(resolveErrorMessage(error, '批量更新术语库失败'))
  }
}

function handleBulkDeleteLibraries() {
  if (!selectedLibraryIds.value.length) return
  dialog.warning({
    title: '批量删除术语库',
    content: `确认删除已选 ${selectedLibraryIds.value.length} 个术语库？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.removeLibraries([...selectedLibraryIds.value])
        selectedLibraryIds.value = []
        message.success('已批量删除术语库')
      } catch (error) {
        message.error(resolveErrorMessage(error, '批量删除术语库失败'))
      }
    },
  })
}

async function handleExportLibrary(item: ExternalGlossaryLibraryItem) {
  try {
    const payload = await store.exportLibrary(item.id)
    triggerBlobDownload(
      new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' }),
      `external-glossary-library-${item.name || item.id}.json`,
    )
  } catch (error) {
    message.error(resolveErrorMessage(error, '导出整库失败'))
  }
}

async function handleImportLibrary() {
  try {
    const parsed = parseLibraryImportContent(libraryImportForm.content)
    const importedLibrary = parsed.library
    const items = parsed.items
    const name = libraryImportForm.name.trim() || importedLibrary?.name || ''
    if (!name) {
      message.warning('术语库名称不能为空')
      return
    }
    const result = await store.importLibrary({
      library: {
        name,
        description: libraryImportForm.description.trim() || importedLibrary?.description || '',
        isEnabled: importedLibrary && !libraryImportForm.name.trim() ? importedLibrary.isEnabled : libraryImportForm.isEnabled,
        sortOrder: importedLibrary && !libraryImportForm.name.trim() ? importedLibrary.sortOrder : libraryImportForm.sortOrder,
      },
      items,
      replace: libraryImportReplace.value,
    })
    store.setActiveLibrary(result.item.id)
    importLibraryVisible.value = false
    libraryImportForm.content = ''
    message.success(`整库导入完成，新增 ${result.stats.created}，更新 ${result.stats.updated}，跳过 ${result.stats.skipped}`)
  } catch (error) {
    message.error(resolveErrorMessage(error, '整库导入失败'))
  }
}

function triggerImportLibraryFile() {
  libraryImportFileInputRef.value?.click()
}

async function handleImportLibraryFile(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (!file) return
  try {
    const text = await file.text()
    const parsed = parseLibraryImportContent(text)
    libraryImportForm.content = parsed.library ? JSON.stringify(parsed.items, null, 2) : text
    if (parsed.library) {
      libraryImportForm.name = parsed.library.name
      libraryImportForm.description = parsed.library.description
      libraryImportForm.isEnabled = parsed.library.isEnabled
      libraryImportForm.sortOrder = parsed.library.sortOrder
    }
    message.success(`已加载文件：${file.name}`)
  } catch (error) {
    console.error(error)
    message.error('读取 JSON 文件失败')
  } finally {
    if (input) {
      input.value = ''
    }
  }
}

async function handleToggleTerm(item: ExternalGlossaryTermItem) {
  const library = selectedLibrary.value
  if (!library) return
  try {
    await store.editTerm(library.id, item.id, {
      keyword: item.keyword,
      category: item.category,
      aliases: item.aliases,
      matchMode: item.matchMode,
      description: item.description,
      descriptionFormat: item.descriptionFormat,
      display: item.display,
      isEnabled: !item.isEnabled,
      sortOrder: item.sortOrder,
    })
    message.success(item.isEnabled ? '术语已停用' : '术语已启用')
  } catch (error) {
    message.error(resolveErrorMessage(error, '更新术语失败'))
  }
}

function openCreateTerm() {
  resetTermForm(null)
  termModalVisible.value = true
}

function openEditTerm(item: ExternalGlossaryTermItem) {
  resetTermForm(item)
  termModalVisible.value = true
}

async function handleSaveTerm() {
  const library = selectedLibrary.value
  if (!library) return
  try {
    const payload = buildTermPayload()
    if (!payload.keyword?.trim()) {
      message.warning('关键词不能为空')
      return
    }
    if (termEditingId.value) {
      await store.editTerm(library.id, termEditingId.value, payload)
      message.success('术语已更新')
    } else {
      await store.createTerm(library.id, payload)
      message.success('术语已创建')
    }
    termModalVisible.value = false
  } catch (error) {
    message.error(resolveErrorMessage(error, '保存术语失败'))
  }
}

function handleDeleteTerm(item: ExternalGlossaryTermItem) {
  const library = selectedLibrary.value
  if (!library) return
  dialog.warning({
    title: '删除术语',
    content: `确认删除“${item.keyword}”？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.removeTerm(library.id, item.id)
        message.success('术语已删除')
      } catch (error) {
        message.error(resolveErrorMessage(error, '删除术语失败'))
      }
    },
  })
}

async function handleBulkTermsEnabled(enabled: boolean) {
  const library = selectedLibrary.value
  if (!library || !selectedTermIds.value.length) return
  try {
    await store.setTermsEnabled(library.id, [...selectedTermIds.value], enabled)
    message.success(enabled ? '已批量启用术语' : '已批量停用术语')
  } catch (error) {
    message.error(resolveErrorMessage(error, '批量更新术语失败'))
  }
}

async function handleBulkTermsDisplay(display: KeywordDisplayStyle) {
  const library = selectedLibrary.value
  if (!library || !selectedTermIds.value.length) return
  try {
    await store.setTermsDisplay(library.id, [...selectedTermIds.value], display)
    message.success('已批量更新显示模式')
  } catch (error) {
    message.error(resolveErrorMessage(error, '批量更新显示模式失败'))
  }
}

function handleBulkDeleteTerms() {
  const library = selectedLibrary.value
  if (!library || !selectedTermIds.value.length) return
  dialog.warning({
    title: '批量删除术语',
    content: `确认删除已选 ${selectedTermIds.value.length} 条术语？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.removeTerms(library.id, [...selectedTermIds.value])
        selectedTermIds.value = []
        message.success('已批量删除术语')
      } catch (error) {
        message.error(resolveErrorMessage(error, '批量删除术语失败'))
      }
    },
  })
}

async function handleImportTerms() {
  const library = selectedLibrary.value
  if (!library) return
  try {
    const items = parseImportContent(termImportForm.content)
    const stats = await store.importTerms(library.id, items, termImportReplace.value)
    importTermsVisible.value = false
    termImportForm.content = ''
    message.success(`术语导入完成，新增 ${stats.created}，更新 ${stats.updated}，跳过 ${stats.skipped}`)
  } catch (error) {
    message.error(resolveErrorMessage(error, '导入术语失败'))
  }
}

async function handleExportTerms() {
  const library = selectedLibrary.value
  if (!library) return
  try {
    const items = await store.exportTerms(library.id, termCategoryFilter.value ?? undefined)
    triggerBlobDownload(
      new Blob([JSON.stringify(items, null, 2)], { type: 'application/json;charset=utf-8' }),
      `external-glossary-terms-${library.name || library.id}.json`,
    )
  } catch (error) {
    message.error(resolveErrorMessage(error, '导出术语失败'))
  }
}

async function handleAddCategory() {
  const library = selectedLibrary.value
  if (!library || !categoryDraft.value.trim()) return
  try {
    await store.addCategory(library.id, categoryDraft.value.trim())
    categoryDraft.value = ''
    message.success('分类已创建')
  } catch (error) {
    message.error(resolveErrorMessage(error, '创建分类失败'))
  }
}

function toggleSelectAllLibraries(checked: boolean) {
  selectedLibraryIds.value = checked ? filteredLibraries.value.map((item) => item.id) : []
}

function toggleSelectAllTerms(checked: boolean) {
  const next = new Set(selectedTermIds.value)
  pagedTerms.value.forEach((item) => {
    if (checked) {
      next.add(item.id)
    } else {
      next.delete(item.id)
    }
  })
  selectedTermIds.value = Array.from(next)
}

function onLibrarySelectAll(event: Event) {
  const target = event.target as HTMLInputElement | null
  toggleSelectAllLibraries(Boolean(target?.checked))
}

function handleSelectAllVisibleTerms(checked: boolean | undefined) {
  toggleSelectAllTerms(Boolean(checked))
}

function handleRowTermSelection(termId: string, checked: boolean | undefined) {
  const next = new Set(selectedTermIds.value)
  if (checked) {
    next.add(termId)
  } else {
    next.delete(termId)
  }
  selectedTermIds.value = Array.from(next)
}

async function handleRenameCategory(category: string) {
  const library = selectedLibrary.value
  if (!library) return
  const nextName = window.prompt('输入新的分类名称', category)
  if (!nextName || nextName.trim() === category) return
  try {
    await store.renameCategory(library.id, category, nextName.trim())
    message.success('分类已重命名')
  } catch (error) {
    message.error(resolveErrorMessage(error, '重命名分类失败'))
  }
}

async function handleDeleteCategory(category: string) {
  const library = selectedLibrary.value
  if (!library) return
  try {
    await store.removeCategory(library.id, category)
    message.success('分类已删除')
  } catch (error) {
    message.error(resolveErrorMessage(error, '删除分类失败'))
  }
}

async function openCategoryPriorityEditor() {
  const library = selectedLibrary.value
  if (!library) return
  try {
    const items = await store.ensureCategoryInfos(library.id, { force: true })
    Object.keys(categoryPriorityDrafts).forEach((key) => delete categoryPriorityDrafts[key])
    items.forEach((item) => {
      categoryPriorityDrafts[item.name] = item.priority || 0
    })
    categoryPriorityItems.value = [...items]
    categoryPriorityVisible.value = true
  } catch (error) {
    message.error(resolveErrorMessage(error, '加载分类优先级失败'))
  }
}

async function handleSaveCategoryPriority(category: string) {
  const library = selectedLibrary.value
  if (!library) return
  try {
    await store.setCategoryPriority(library.id, category, Number(categoryPriorityDrafts[category] || 0))
    const latest = await store.ensureCategoryInfos(library.id, { force: true })
    categoryPriorityItems.value = [...latest]
    message.success('分类优先级已更新')
  } catch (error) {
    message.error(resolveErrorMessage(error, '更新分类优先级失败'))
  }
}

function applyCategoryPriorityOrder(items: KeywordCategoryInfo[]) {
  const total = items.length
  items.forEach((item, index) => {
    categoryPriorityDrafts[item.name] = total - index
  })
}

function handleCategoryPriorityDragStart(item: KeywordCategoryInfo) {
  if (categoryPrioritySaving.value) return
  categoryPriorityDragSource.value = item.name
}

function handleCategoryPriorityDragEnter(item: KeywordCategoryInfo) {
  if (categoryPrioritySaving.value || !categoryPriorityDragSource.value || categoryPriorityDragSource.value === item.name) return
  categoryPriorityDragTarget.value = item.name
}

function handleCategoryPriorityDragOver(event: DragEvent) {
  if (categoryPrioritySaving.value) return
  event.preventDefault()
}

function handleCategoryPriorityDragLeave() {
  categoryPriorityDragTarget.value = null
}

async function handleCategoryPriorityDrop(item: KeywordCategoryInfo) {
  const library = selectedLibrary.value
  const sourceName = categoryPriorityDragSource.value
  if (!library || categoryPrioritySaving.value || !sourceName || sourceName === item.name) return
  const items = [...categoryPriorityItems.value]
  const fromIndex = items.findIndex((entry) => entry.name === sourceName)
  const toIndex = items.findIndex((entry) => entry.name === item.name)
  if (fromIndex < 0 || toIndex < 0) return
  const [moved] = items.splice(fromIndex, 1)
  items.splice(toIndex, 0, moved)
  applyCategoryPriorityOrder(items)
  categoryPriorityItems.value = items.map((entry) => ({ ...entry, priority: categoryPriorityDrafts[entry.name] || 0 }))
  categoryPrioritySaving.value = true
  try {
    await store.setCategoryPriorities(library.id, items.map((entry) => ({
      name: entry.name,
      priority: categoryPriorityDrafts[entry.name] || 0,
    })))
    const latest = await store.ensureCategoryInfos(library.id, { force: true })
    categoryPriorityItems.value = [...latest]
    message.success('已按拖拽顺序更新分类优先级')
  } catch (error) {
    message.error(resolveErrorMessage(error, '拖拽更新分类优先级失败'))
    const latest = await store.ensureCategoryInfos(library.id, { force: true })
    categoryPriorityItems.value = [...latest]
  } finally {
    categoryPrioritySaving.value = false
    categoryPriorityDragSource.value = null
    categoryPriorityDragTarget.value = null
  }
}

function handleCategoryPriorityDragEnd() {
  categoryPriorityDragSource.value = null
  categoryPriorityDragTarget.value = null
}

watch(() => store.activeLibraryId, () => {
  selectedTermIds.value = []
  termCategoryFilter.value = null
  termQuery.value = ''
  currentTermPage.value = 1
})

watch(termItems, (items) => {
  const validIds = new Set(items.map((item) => item.id))
  selectedTermIds.value = selectedTermIds.value.filter((id) => validIds.has(id))
})

watch(
  () => filteredTerms.value.length,
  (length) => {
    const maxPage = Math.max(1, Math.ceil(Math.max(length, 1) / PAGE_SIZE))
    if (currentTermPage.value > maxPage) {
      currentTermPage.value = maxPage
    }
  },
)

watch(
  () => [termQuery.value, termCategoryFilter.value],
  () => {
    currentTermPage.value = 1
  },
)

onMounted(async () => {
  await store.ensureLibraries({ force: true })
})
</script>

<template>
  <div class="external-glossary-admin">
    <div class="external-glossary-admin__toolbar">
      <n-input v-model:value="libraryQuery" size="small" clearable placeholder="搜索术语库名称或简介" />
      <n-button size="small" @click="refreshAll">刷新</n-button>
      <n-button size="small" type="primary" @click="openCreateLibrary">新建术语库</n-button>
      <n-button size="small" secondary @click="importLibraryVisible = true">导入整库</n-button>
      <n-button size="small" :disabled="!selectedLibraryIds.length" @click="handleBulkLibraryState(true)">批量启用</n-button>
      <n-button size="small" :disabled="!selectedLibraryIds.length" @click="handleBulkLibraryState(false)">批量停用</n-button>
      <n-button size="small" tertiary type="error" :disabled="!selectedLibraryIds.length" @click="handleBulkDeleteLibraries">批量删除</n-button>
    </div>

    <div class="external-glossary-admin__content">
      <section class="external-glossary-admin__panel external-glossary-admin__panel--libraries">
        <div class="external-glossary-admin__panel-header">
          <strong>外挂术语库</strong>
          <span class="external-glossary-admin__count">{{ filteredLibraries.length }} / {{ libraryItems.length }}</span>
        </div>
        <n-spin :show="store.libraryLoading">
          <div class="external-glossary-admin__table-wrap">
            <table class="external-glossary-admin__table">
              <thead>
                <tr>
                  <th class="w-10">
                  <input
                    type="checkbox"
                    :checked="filteredLibraries.length > 0 && selectedLibraryIds.length === filteredLibraries.length"
                    @change="onLibrarySelectAll"
                  >
                  </th>
                  <th>术语库</th>
                  <th>术语数</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in filteredLibraries"
                  :key="item.id"
                  :class="{ 'is-active': item.id === store.activeLibraryId }"
                  @click="handleSelectLibrary(item.id)"
                >
                  <td>
                    <input v-model="selectedLibraryIds" type="checkbox" :value="item.id" @click.stop>
                  </td>
                  <td>
                    <div class="external-glossary-admin__library-name">{{ item.name }}</div>
                    <div class="external-glossary-admin__library-desc">{{ item.description || '无简介' }}</div>
                  </td>
                  <td>{{ item.termCount }}</td>
                  <td>
                    <n-tag size="small" :type="item.isEnabled ? 'success' : 'default'">
                      {{ item.isEnabled ? '启用' : '停用' }}
                    </n-tag>
                  </td>
                  <td>
                    <n-space size="small">
                      <n-button size="tiny" text type="primary" @click.stop="openLibraryEditor(item)">编辑</n-button>
                      <n-button size="tiny" text @click.stop="openEditLibrary(item)">设置</n-button>
                      <n-button size="tiny" text @click.stop="handleToggleLibrary(item)">
                        {{ item.isEnabled ? '停用' : '启用' }}
                      </n-button>
                      <n-button size="tiny" text @click.stop="handleExportLibrary(item)">导出</n-button>
                      <n-button size="tiny" text type="error" @click.stop="handleDeleteLibrary(item)">删除</n-button>
                    </n-space>
                  </td>
                </tr>
                <tr v-if="!filteredLibraries.length">
                  <td colspan="5" class="external-glossary-admin__empty">暂无术语库</td>
                </tr>
              </tbody>
            </table>
          </div>
        </n-spin>
      </section>
    </div>

    <n-modal v-model:show="libraryModalVisible" preset="card" :title="libraryEditingId ? '编辑术语库' : '新建术语库'" class="external-glossary-admin__modal">
      <n-form label-placement="top">
        <n-form-item label="名称" required>
          <n-input v-model:value="libraryForm.name" maxlength="120" show-count />
        </n-form-item>
        <n-form-item label="简介">
          <n-input v-model:value="libraryForm.description" type="textarea" maxlength="500" show-count />
        </n-form-item>
        <div class="external-glossary-admin__modal-row">
          <n-form-item label="排序" class="external-glossary-admin__modal-field">
            <n-input-number v-model:value="libraryForm.sortOrder" style="width: 100%" />
          </n-form-item>
          <n-form-item label="启用" class="external-glossary-admin__modal-field">
            <n-switch v-model:value="libraryForm.isEnabled" />
          </n-form-item>
        </div>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="libraryModalVisible = false">取消</n-button>
          <n-button type="primary" @click="handleSaveLibrary">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-drawer
      v-model:show="editorVisible"
      :width="'100%'"
      placement="right"
      class="external-glossary-admin__drawer"
    >
      <n-drawer-content closable>
        <template #header>
          <div class="external-glossary-admin__drawer-header">
            <span class="external-glossary-admin__drawer-title">
              {{ selectedLibrary ? `编辑外挂术语库：${selectedLibrary.name}` : '编辑外挂术语库' }}
            </span>
            <n-button size="small" quaternary @click="editorVisible = false">退出</n-button>
          </div>
        </template>
        <template v-if="selectedLibrary">
          <div class="external-glossary-admin__drawer-body">
            <section class="external-glossary-admin__panel external-glossary-admin__panel--editor">
              <div class="external-glossary-admin__panel-header">
                <div>
                  <strong>{{ selectedLibrary.name }}</strong>
                  <div class="external-glossary-admin__library-desc">{{ selectedLibrary.description || '当前术语库暂无简介' }}</div>
                </div>
                <n-space size="small">
                  <n-button size="small" @click="categoryVisible = true">分类</n-button>
                  <n-button size="small" @click="openCategoryPriorityEditor">分类优先级</n-button>
                  <n-button size="small" @click="handleExportTerms">导出术语</n-button>
                  <n-button size="small" type="primary" @click="openCreateTerm">新建术语</n-button>
                  <n-button size="small" secondary @click="importTermsVisible = true">导入术语</n-button>
                </n-space>
              </div>

              <div class="external-glossary-admin__toolbar external-glossary-admin__toolbar--sub">
                <n-input v-model:value="termQuery" size="small" clearable placeholder="搜索关键词、别名、分类或描述" />
                <n-select
                  v-model:value="termCategoryFilter"
                  size="small"
                  clearable
                  placeholder="全部分类"
                  :options="categoryOptions.map(item => ({ label: item, value: item }))"
                />
                <n-button size="small" :disabled="!selectedTermIds.length" @click="handleBulkTermsEnabled(true)">批量启用</n-button>
                <n-button size="small" :disabled="!selectedTermIds.length" @click="handleBulkTermsEnabled(false)">批量停用</n-button>
                <n-button size="small" :disabled="!selectedTermIds.length" @click="handleBulkTermsDisplay('inherit')">跟随全局</n-button>
                <n-button size="small" :disabled="!selectedTermIds.length" @click="handleBulkTermsDisplay('standard')">标准</n-button>
                <n-button size="small" :disabled="!selectedTermIds.length" @click="handleBulkTermsDisplay('minimal')">极简</n-button>
                <n-button size="small" tertiary type="error" :disabled="!selectedTermIds.length" @click="handleBulkDeleteTerms">批量删除</n-button>
              </div>

              <n-spin :show="store.termLoadingMap[selectedLibrary.id]" class="external-glossary-admin__table-spin">
                <div class="external-glossary-admin__table-wrap">
                  <table class="external-glossary-admin__table">
                    <thead>
                      <tr>
                        <th class="w-10">
                          <n-checkbox
                            :checked="isAllVisibleTermsSelected"
                            :indeterminate="isTermSelectionIndeterminate"
                            :disabled="!pagedTerms.length"
                            @update:checked="handleSelectAllVisibleTerms"
                          />
                        </th>
                        <th>关键词</th>
                        <th>分类</th>
                        <th>描述</th>
                        <th>显示</th>
                        <th>状态</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="item in pagedTerms" :key="item.id">
                        <td>
                          <n-checkbox
                            :checked="selectedTermIds.includes(item.id)"
                            @update:checked="(checked: boolean) => handleRowTermSelection(item.id, checked)"
                          />
                        </td>
                        <td>
                          <div class="external-glossary-admin__library-name">{{ item.keyword }}</div>
                          <div v-if="item.aliases?.length" class="external-glossary-admin__library-desc">
                            别名：{{ item.aliases.join(' / ') }}
                          </div>
                        </td>
                        <td>{{ item.category || '未分类' }}</td>
                        <td>{{ getDescriptionPlainText(item) || '无描述' }}</td>
                        <td>{{ item.display || 'inherit' }}</td>
                        <td>
                          <n-tag size="small" :type="item.isEnabled ? 'success' : 'default'">
                            {{ item.isEnabled ? '启用' : '停用' }}
                          </n-tag>
                        </td>
                        <td>
                          <n-space size="small">
                            <n-button size="tiny" text @click="openEditTerm(item)">编辑</n-button>
                            <n-button size="tiny" text @click="handleToggleTerm(item)">
                              {{ item.isEnabled ? '停用' : '启用' }}
                            </n-button>
                            <n-button size="tiny" text type="error" @click="handleDeleteTerm(item)">删除</n-button>
                          </n-space>
                        </td>
                      </tr>
                      <tr v-if="!filteredTerms.length">
                        <td colspan="7" class="external-glossary-admin__empty">当前术语库暂无术语</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </n-spin>
            </section>

            <div v-if="filteredTerms.length > PAGE_SIZE" class="external-glossary-admin__pagination">
              <n-pagination
                size="small"
                :item-count="filteredTerms.length"
                :page-size="PAGE_SIZE"
                :page="currentTermPage"
                @update:page="currentTermPage = $event"
              />
            </div>
          </div>
        </template>
        <n-empty v-else description="请选择要编辑的外挂术语库" class="external-glossary-admin__placeholder" />
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="termModalVisible" preset="card" :title="termEditingId ? '编辑术语' : '新建术语'" class="external-glossary-admin__modal external-glossary-admin__modal--wide">
      <n-form label-placement="top" size="small">
        <n-form-item label="关键词" required>
          <n-input v-model:value="termForm.keyword" maxlength="120" show-count />
        </n-form-item>
        <div class="external-glossary-admin__modal-row">
          <n-form-item label="分类" class="external-glossary-admin__modal-field">
            <n-input v-model:value="termForm.category" />
          </n-form-item>
          <n-form-item label="匹配模式" class="external-glossary-admin__modal-field">
            <n-select
              v-model:value="termForm.matchMode"
              :options="[
                { label: '普通文本', value: 'plain' },
                { label: '正则表达式', value: 'regex' },
              ]"
            />
          </n-form-item>
        </div>
        <n-form-item label="别名">
          <n-dynamic-tags v-model:value="termForm.aliases" :max="10" />
        </n-form-item>
        <div class="external-glossary-admin__modal-row">
          <n-form-item label="显示模式" class="external-glossary-admin__modal-field">
            <n-select v-model:value="termForm.display" :options="displayOptions" />
          </n-form-item>
          <n-form-item label="排序" class="external-glossary-admin__modal-field">
            <n-input-number v-model:value="termForm.sortOrder" style="width: 100%" />
          </n-form-item>
        </div>
        <div class="external-glossary-admin__modal-row">
          <n-form-item label="富文本描述" class="external-glossary-admin__modal-field">
            <n-switch v-model:value="isRichMode" />
          </n-form-item>
          <n-form-item label="启用" class="external-glossary-admin__modal-field">
            <n-switch v-model:value="termForm.isEnabled" />
          </n-form-item>
        </div>
        <n-form-item label="描述">
          <KeywordRichEditor
            v-if="isRichMode"
            v-model="termForm.description"
            placeholder="输入术语描述"
            :max-length="keywordMaxLength"
            class="external-glossary-admin__editor"
          />
          <KeywordDescriptionEditor
            v-else
            v-model="termForm.description"
            placeholder="输入术语描述"
            :max-length="keywordMaxLength"
            class="external-glossary-admin__editor"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="termModalVisible = false">取消</n-button>
          <n-button type="primary" @click="handleSaveTerm">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="importLibraryVisible" preset="card" title="导入外挂术语库" class="external-glossary-admin__modal external-glossary-admin__modal--wide">
      <n-form label-placement="top">
        <n-form-item label="术语库名称" required>
          <n-input v-model:value="libraryImportForm.name" />
        </n-form-item>
        <n-form-item label="简介">
          <n-input v-model:value="libraryImportForm.description" type="textarea" />
        </n-form-item>
        <div class="external-glossary-admin__modal-row">
          <n-form-item label="排序" class="external-glossary-admin__modal-field">
            <n-input-number v-model:value="libraryImportForm.sortOrder" style="width: 100%" />
          </n-form-item>
          <n-form-item label="启用" class="external-glossary-admin__modal-field">
            <n-switch v-model:value="libraryImportForm.isEnabled" />
          </n-form-item>
        </div>
        <div class="external-glossary-admin__import-upload">
          <n-button size="small" @click="triggerImportLibraryFile">上传 JSON 文件</n-button>
          <input
            ref="libraryImportFileInputRef"
            type="file"
            accept=".json,application/json"
            class="external-glossary-admin__import-file-input"
            @change="handleImportLibraryFile"
          >
        </div>
        <n-form-item label="术语内容">
          <n-input
            v-model:value="libraryImportForm.content"
            type="textarea"
            :autosize="{ minRows: 10, maxRows: 16 }"
            placeholder="支持整库导出 JSON、术语 JSON 数组，或每行 keyword|description|aliases|category"
          />
        </n-form-item>
        <n-checkbox v-model:checked="libraryImportReplace">同关键词允许覆盖更新</n-checkbox>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="importLibraryVisible = false">取消</n-button>
          <n-button type="primary" @click="handleImportLibrary">导入</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="importTermsVisible" preset="card" title="导入术语" class="external-glossary-admin__modal external-glossary-admin__modal--wide">
      <n-form label-placement="top">
        <n-form-item label="术语内容">
          <n-input
            v-model:value="termImportForm.content"
            type="textarea"
            :autosize="{ minRows: 10, maxRows: 16 }"
            placeholder="支持 JSON 数组，或每行 keyword|description|aliases|category"
          />
        </n-form-item>
        <n-checkbox v-model:checked="termImportReplace">同关键词允许覆盖更新</n-checkbox>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="importTermsVisible = false">取消</n-button>
          <n-button type="primary" @click="handleImportTerms">导入</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="categoryVisible" preset="card" title="分类管理" class="external-glossary-admin__modal">
      <n-space vertical>
        <div class="external-glossary-admin__toolbar external-glossary-admin__toolbar--sub">
          <n-input v-model:value="categoryDraft" size="small" placeholder="新分类名称" />
          <n-button size="small" type="primary" @click="handleAddCategory">新增</n-button>
        </div>
        <div v-for="category in categoryOptions" :key="category" class="external-glossary-admin__category-row">
          <span>{{ category }}</span>
          <n-space size="small">
            <n-button size="tiny" text @click="handleRenameCategory(category)">重命名</n-button>
            <n-button size="tiny" text type="error" @click="handleDeleteCategory(category)">删除</n-button>
          </n-space>
        </div>
        <n-empty v-if="!categoryOptions.length" description="暂无分类" />
      </n-space>
    </n-modal>

    <n-modal v-model:show="categoryPriorityVisible" preset="card" title="分类优先级" class="external-glossary-admin__modal">
      <n-space vertical>
        <div class="external-glossary-admin__category-tip">拖动排序后会立即保存，最上方优先级最高。</div>
        <div
          v-for="category in categoryPriorityItems"
          :key="category.name"
          class="external-glossary-admin__category-row"
          :draggable="!categoryPrioritySaving"
          :class="{ 'is-active': categoryPriorityDragTarget === category.name }"
          @dragstart="handleCategoryPriorityDragStart(category)"
          @dragenter="handleCategoryPriorityDragEnter(category)"
          @dragover="handleCategoryPriorityDragOver"
          @dragleave="handleCategoryPriorityDragLeave"
          @drop="handleCategoryPriorityDrop(category)"
          @dragend="handleCategoryPriorityDragEnd"
        >
          <div class="external-glossary-admin__category-meta">
            <div class="external-glossary-admin__category-name">
              <span class="external-glossary-admin__category-drag">::</span>
              <span>{{ category.name }}</span>
            </div>
            <span class="external-glossary-admin__category-count">{{ category.count }} 个术语</span>
          </div>
          <n-space size="small">
            <n-input-number
              v-model:value="categoryPriorityDrafts[category.name]"
              size="small"
              style="width: 100px"
              :disabled="categoryPrioritySaving"
            />
            <n-button size="tiny" text type="primary" :loading="categoryPrioritySaving" @click="handleSaveCategoryPriority(category.name)">保存</n-button>
          </n-space>
        </div>
        <n-empty v-if="!categoryPriorityItems.length" description="暂无可配置分类" />
      </n-space>
    </n-modal>
  </div>
</template>

<style scoped>
.external-glossary-admin__drawer :deep(.n-drawer-body-content-wrapper) {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  min-height: 0;
}

.external-glossary-admin {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  min-height: 0;
}

.external-glossary-admin__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.external-glossary-admin__toolbar--sub {
  padding: 8px 0;
}

.external-glossary-admin__content {
  flex: 1;
  min-height: 0;
}

.external-glossary-admin__panel {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.04);
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.external-glossary-admin__panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
}

.external-glossary-admin__count {
  color: var(--text-color-3);
  font-size: 12px;
}

.external-glossary-admin__table-wrap {
  overflow: auto;
  min-height: 0;
  flex: 1;
}

.external-glossary-admin__table {
  width: 100%;
  border-collapse: collapse;
}

.external-glossary-admin__table th,
.external-glossary-admin__table td {
  padding: 10px 12px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.1);
  vertical-align: top;
  text-align: left;
  font-size: 13px;
}

.external-glossary-admin__table tbody tr {
  cursor: pointer;
}

.external-glossary-admin__table tbody tr:hover,
.external-glossary-admin__table tbody tr.is-active {
  background: rgba(148, 163, 184, 0.08);
}

.external-glossary-admin__library-name {
  font-weight: 600;
  line-height: 1.4;
}

.external-glossary-admin__library-desc {
  margin-top: 4px;
  color: var(--text-color-3);
  font-size: 12px;
  white-space: pre-wrap;
}

.external-glossary-admin__empty,
.external-glossary-admin__placeholder {
  color: var(--text-color-3);
}

.external-glossary-admin__placeholder {
  margin: auto;
}

.external-glossary-admin__modal {
  width: min(680px, calc(100vw - 32px));
}

.external-glossary-admin__modal--wide {
  width: min(860px, calc(100vw - 32px));
}

.external-glossary-admin__modal-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.external-glossary-admin__modal-field {
  min-width: 0;
}

.external-glossary-admin__editor {
  width: 100%;
}

.external-glossary-admin__drawer-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 0;
}

.external-glossary-admin__table-spin {
  display: flex;
  flex: 1;
  min-height: 0;
}

.external-glossary-admin__table-spin :deep(.n-spin-content) {
  display: flex;
  flex: 1;
  min-height: 0;
}

.external-glossary-admin__drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.external-glossary-admin__drawer-title {
  font-size: 16px;
  font-weight: 600;
  line-height: 1.4;
}

.external-glossary-admin__panel--editor {
  flex: 1;
  min-height: 0;
}

.external-glossary-admin__import-upload {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.external-glossary-admin__import-file-input {
  display: none;
}

.external-glossary-admin__category-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.1);
  cursor: grab;
}

.external-glossary-admin__category-row.is-active {
  background: rgba(148, 163, 184, 0.08);
}

.external-glossary-admin__category-row:active {
  cursor: grabbing;
}

.external-glossary-admin__category-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.external-glossary-admin__category-name {
  display: flex;
  align-items: center;
  gap: 8px;
}

.external-glossary-admin__category-tip {
  color: var(--text-color-3);
  font-size: 12px;
}

.external-glossary-admin__category-drag {
  color: var(--text-color-3);
  font-size: 12px;
  letter-spacing: 1px;
  user-select: none;
}

.external-glossary-admin__category-count {
  color: var(--text-color-3);
  font-size: 12px;
}

.external-glossary-admin__pagination {
  display: flex;
  justify-content: center;
  padding-bottom: 4px;
}

@media (max-width: 640px) {
  .external-glossary-admin__modal-row {
    grid-template-columns: 1fr;
  }

  .external-glossary-admin__table th,
  .external-glossary-admin__table td {
    padding: 8px;
  }
}
</style>
