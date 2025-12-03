<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useWorldGlossaryStore } from '@/stores/worldGlossary'
import { useChatStore } from '@/stores/chat'
import { useDialog, useMessage } from 'naive-ui'
import { triggerBlobDownload } from '@/utils/download'
import type { WorldKeywordItem, WorldKeywordPayload } from '@/models/worldGlossary'
import { useBreakpoints } from '@vueuse/core'

const KEYWORD_MAX_LENGTH = 500
const glossary = useWorldGlossaryStore()
const chat = useChatStore()
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

const filteredKeywords = computed(() => {
  const q = filterValue.value.trim().toLowerCase()
  if (!q) return keywordItems.value
  return keywordItems.value.filter((item) => {
    const haystack = [item.keyword, ...(item.aliases || []), item.description || ''].join(' ').toLowerCase()
    return haystack.includes(q)
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
  return role === 'owner' || role === 'admin'
})

const formModel = reactive({
  keyword: '',
  aliases: '',
  matchMode: 'plain' as 'plain' | 'regex',
  description: '',
  display: 'standard' as 'standard' | 'minimal',
  isEnabled: true,
})

const importText = reactive({ content: '' })

const isRegexMatch = computed({
  get: () => formModel.matchMode === 'regex',
  set: (value: boolean) => {
    formModel.matchMode = value ? 'regex' : 'plain'
  },
})

const isMinimalDisplay = computed({
  get: () => formModel.display === 'minimal',
  set: (value: boolean) => {
    formModel.display = value ? 'minimal' : 'standard'
  },
})

const clampText = (value: string) => value.slice(0, KEYWORD_MAX_LENGTH)

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
  const aliases = splitAliases(entry.aliases)
  if (aliases.length) {
    payload.aliases = aliases
  }
  const description = entry.description ?? entry.desc
  if (description) {
    const text = clampText(String(description).trim())
    if (text) payload.description = text
  }
  if (entry.matchMode === 'regex' || entry.matchMode === 'plain') {
    payload.matchMode = entry.matchMode
  }
  if (entry.display === 'minimal' || entry.display === 'standard') {
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
      const descriptionRaw = clampText(columns[1] || '')
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

function resetForm() {
  formModel.keyword = ''
  formModel.aliases = ''
  formModel.matchMode = 'plain'
  formModel.description = ''
  formModel.display = 'standard'
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
  formModel.aliases = (item.aliases || []).map((alias) => clampText(alias)).join(', ')
  formModel.matchMode = item.matchMode
  formModel.description = clampText(item.description || '')
  formModel.display = item.display
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
    .map((item) => clampText(item.trim()))
    .filter(Boolean)
  const payload = {
    keyword,
    aliases,
    matchMode: formModel.matchMode,
    description: formModel.description?.trim() ? clampText(formModel.description.trim()) : undefined,
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
    aliases: item.aliases,
    matchMode: item.matchMode,
    description: item.description,
    display: item.display,
    isEnabled: !item.isEnabled,
  })
}

async function handleExport() {
  const worldId = currentWorldId.value
  if (!worldId) return
  const items = await glossary.exportKeywords(worldId)
  const blob = new Blob([JSON.stringify(items, null, 2)], { type: 'application/json' })
  const worldName = chat.worldMap[worldId]?.name || 'world'
  triggerBlobDownload(blob, `${worldName}-keywords.json`)
  message.success('已导出词库')
}

async function handleImport(replace = false) {
  const worldId = glossary.importState.worldId || currentWorldId.value
  if (!worldId) return
  try {
    const payloads = parseStructuredImport(importText.content || '')
    if (!payloads.length) {
      message.error('未识别到可导入的数据，请检查格式')
      return
    }
    await glossary.importKeywords(worldId, payloads, replace)
    message.success('导入完成')
  } catch (error: any) {
    message.error(error?.message || '导入失败')
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
  (visible) => {
    if (visible) {
      if (currentWorldId.value) {
        glossary.ensureKeywords(currentWorldId.value, { force: true })
        chat.worldDetail(currentWorldId.value)
      }
      currentPage.value = 1
    } else {
      clearSelection()
      currentPage.value = 1
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
        formModel.aliases = (keyword.aliases || []).join(', ')
        formModel.matchMode = keyword.matchMode
        formModel.description = keyword.description
        formModel.display = keyword.display
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
</script>

<template>
  <n-drawer v-model:show="drawerVisible" :width="drawerWidth" placement="right" :mask-closable="true">
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
      <n-input
        v-model:value="filterValue"
        placeholder="搜索关键词或描述"
        clearable
        size="small"
      />
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
            <n-button size="tiny" tertiary :disabled="!currentWorldId" @click="handleExport">
              导出 JSON
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
                <th>匹配</th>
                <th>显示</th>
                <th>状态</th>
                <th style="width: 120px;">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in pagedKeywords" :key="item.id">
                <td>
                  <n-checkbox
                    :checked="selectedIds.includes(item.id)"
                    :disabled="!canEdit"
                    @update:checked="(checked) => handleRowSelection(item.id, checked)"
                  />
                </td>
                <td>
                  <div class="font-medium">{{ item.keyword }}</div>
                  <div class="text-xs text-gray-500" v-if="item.aliases?.length">别名：{{ item.aliases.join(', ') }}</div>
                  <div class="text-xs text-gray-500" v-if="item.description">{{ item.description }}</div>
                </td>
                <td>{{ item.matchMode === 'regex' ? '正则' : '文本' }}</td>
                <td>{{ item.display === 'minimal' ? '极简下划线' : '标准' }}</td>
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
                <td colspan="6" class="text-center text-gray-400">暂无数据</td>
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
                  @update:checked="(checked) => handleRowSelection(item.id, checked)"
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
  </n-drawer>

  <n-modal v-model:show="editorVisible" preset="card" :title="isEditing ? '编辑术语' : '新增术语'" style="width: 520px">
    <n-form label-placement="top" class="keyword-editor-form" size="small">
      <div class="keyword-editor__row keyword-editor__row--compact">
        <n-form-item label="关键词" required class="keyword-editor__field keyword-editor__field--keyword" :show-feedback="false">
          <n-input v-model:value="formModel.keyword" placeholder="必填" :maxlength="KEYWORD_MAX_LENGTH" show-count />
        </n-form-item>
      </div>
      <div class="keyword-editor__row keyword-editor__row--compact">
        <n-form-item label="别名（逗号分隔）" class="keyword-editor__field keyword-editor__field--alias" :show-feedback="false">
          <n-input v-model:value="formModel.aliases" placeholder="可选" :maxlength="KEYWORD_MAX_LENGTH" />
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
          <span class="keyword-toggle__label">极简样式</span>
          <n-switch v-model:value="isMinimalDisplay">
            <template #checked>极简</template>
            <template #unchecked>标准</template>
          </n-switch>
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
        <n-form-item label="术语描述 / 详细说明" path="description" :show-feedback="false">
          <n-input
            v-model:value="formModel.description"
            type="textarea"
            :autosize="{ minRows: 8, maxRows: 14 }"
            :maxlength="KEYWORD_MAX_LENGTH"
            show-count
            placeholder="用于聊天中的提示和解释"
          />
        </n-form-item>
      </div>
    </n-form>
    <template #action>
      <n-space>
        <n-button @click="glossary.closeEditor()">取消</n-button>
        <n-button type="primary" @click="submitEditor">保存</n-button>
      </n-space>
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
    <n-input
      v-model:value="importText.content"
      type="textarea"
      :autosize="{ minRows: 8 }"
      placeholder='[\n  { "keyword": "阿瓦隆", "description": "古老之城" }\n]'
    />
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
</template>

<style scoped>
.keyword-editor-form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
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

.keyword-toggle__label {
  font-size: 13px;
  color: #4b5563;
}

.keyword-editor__description :deep(.n-input) {
  font-size: 14px;
  line-height: 1.5;
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

@media (max-width: 767px) {
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
    flex-direction: column;
    align-items: flex-start;
  }

  .keyword-editor__row--compact :deep(.n-form-item) {
    margin-bottom: 0.35rem;
  }
}
</style>
.keyword-editor__field--alias :deep(.n-input) {
  font-size: 14px;
}
