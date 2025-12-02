<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import dayjs from 'dayjs'
import { useDialog, useMessage, type UploadCustomRequestOptions } from 'naive-ui'
import { useWorldKeywordStore } from '@/stores/worldKeywords'
import type { WorldKeyword } from '@/types/world'

interface Props {
  worldId: string
  visible: boolean
  canEdit: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

const keywordStore = useWorldKeywordStore()
const message = useMessage()
const dialog = useDialog()

const formVisible = ref(false)
const formLoading = ref(false)
const exporting = ref(false)
const importVisible = ref(false)
const importLoading = ref(false)
const batchDeleting = ref(false)
const importForm = reactive({
  content: '',
})
const editing = ref<WorldKeyword | null>(null)
const form = reactive({
  keyword: '',
  description: '',
})
const searchKeyword = ref('')
const selectedIds = ref<string[]>([])

const keywordList = computed(() => {
  if (!props.worldId) return []
  return keywordStore.keywords(props.worldId) || []
})

const filteredKeywordList = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return keywordList.value
  return keywordList.value.filter((item) => {
    const kw = item.keyword.toLowerCase()
    const desc = item.description?.toLowerCase() || ''
    return kw.includes(keyword) || desc.includes(keyword)
  })
})

const loading = computed(() => {
  if (!props.worldId) return false
  return keywordStore.loadingMap[props.worldId] ?? false
})

const hasSelection = computed(() => selectedIds.value.length > 0)
const selectedCount = computed(() => selectedIds.value.length)
const isAllSelected = computed(() => {
  if (!filteredKeywordList.value.length) return false
  return filteredKeywordList.value.every((item) => selectedIds.value.includes(item.id))
})
const isIndeterminate = computed(() => hasSelection.value && !isAllSelected.value)

const clearSelection = () => {
  selectedIds.value = []
}

const removeFromSelection = (keywordId: string) => {
  selectedIds.value = selectedIds.value.filter((id) => id !== keywordId)
}

const toggleSelect = (keywordId: string, checked: boolean) => {
  if (!checked) {
    removeFromSelection(keywordId)
    return
  }
  if (!selectedIds.value.includes(keywordId)) {
    selectedIds.value = [...selectedIds.value, keywordId]
  }
}

const toggleSelectAll = (checked: boolean) => {
  if (!checked) {
    clearSelection()
    return
  }
  selectedIds.value = filteredKeywordList.value.map((item) => item.id)
}

watch(
  () => props.visible,
  (visible) => {
    if (visible && props.worldId) {
      void keywordStore.ensure(props.worldId)
    } else if (!visible) {
      clearSelection()
    }
  },
  { immediate: false },
)

watch(
  () => props.worldId,
  () => {
    clearSelection()
  },
)

watch(keywordList, (list) => {
  if (!list?.length) {
    clearSelection()
    return
  }
  const allowed = new Set(list.map((item) => item.id))
  selectedIds.value = selectedIds.value.filter((id) => allowed.has(id))
})

const resetForm = (payload?: WorldKeyword | null, overrides?: { keyword?: string; description?: string }) => {
  editing.value = payload ?? null
  form.keyword = overrides?.keyword ?? payload?.keyword ?? ''
  form.description = overrides?.description ?? payload?.description ?? ''
  formVisible.value = true
}

const handleSubmit = async () => {
  if (!props.worldId) return
  if (!form.keyword.trim() || !form.description.trim()) {
    message.warning('请填写完整关键词与描述')
    return
  }
  formLoading.value = true
  try {
    if (editing.value) {
      await keywordStore.updateKeyword(props.worldId, editing.value.id, {
        keyword: form.keyword,
        description: form.description,
      })
      message.success('已更新关键词')
    } else {
      await keywordStore.createKeyword(props.worldId, {
        keyword: form.keyword,
        description: form.description,
      })
      message.success('已创建关键词')
    }
    formVisible.value = false
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败')
  } finally {
    formLoading.value = false
  }
}

const handleDelete = (item: WorldKeyword) => {
  if (!props.canEdit) return
  dialog.warning({
    title: '删除关键词',
    content: `确定要删除「${item.keyword}」吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await keywordStore.deleteKeyword(props.worldId, item.id)
        removeFromSelection(item.id)
        message.success('已删除关键词')
      } catch (error: any) {
        message.error(error?.response?.data?.message || '删除失败')
      }
    },
  })
}

const handleBatchDelete = () => {
  if (!props.canEdit || !props.worldId || !selectedIds.value.length) return
  const ids = [...selectedIds.value]
  dialog.warning({
    title: '批量删除关键词',
    content: `确认要删除选中的 ${ids.length} 个关键词吗？该操作不可恢复。`,
    positiveText: '批量删除',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: async () => {
      batchDeleting.value = true
      try {
        await keywordStore.deleteKeywords(props.worldId, ids)
        message.success(`已删除 ${ids.length} 个关键词`)
        clearSelection()
      } catch (error: any) {
        message.error(error?.message || '批量删除失败，请重试')
      } finally {
        batchDeleting.value = false
      }
    },
  })
}

const handleExport = async () => {
  if (!props.worldId) return
  exporting.value = true
  try {
    await keywordStore.exportKeywords(props.worldId)
  } catch (error: any) {
    message.error(error?.response?.data?.message || '导出失败')
  } finally {
    exporting.value = false
  }
}

const handleImport = async () => {
  if (!props.worldId) return
  if (!importForm.content.trim()) {
    message.warning('请粘贴或上传需要导入的内容')
    return
  }
  importLoading.value = true
  try {
    const stats = await keywordStore.importKeywords(props.worldId, importForm.content)
    message.success(`导入完成：新增 ${stats?.created ?? 0}，更新 ${stats?.updated ?? 0}，跳过 ${stats?.skipped ?? 0}`)
    importVisible.value = false
    importForm.content = ''
  } catch (error: any) {
    message.error(error?.response?.data?.message || '导入失败')
  } finally {
    importLoading.value = false
  }
}

const handleImportUpload: UploadCustomRequestOptions = (options) => {
  const file = options.file.file as File | undefined
  if (!file) {
    options.onError?.(new Error('未选择文件'))
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    importForm.content = String(reader.result || '')
    options.onFinish?.()
  }
  reader.onerror = () => {
    message.error('读取文件失败')
    options.onError?.(new Error('读取失败'))
  }
  reader.readAsText(file, 'utf-8')
}

const close = () => emit('update:visible', false)
const canEdit = computed(() => props.canEdit)

const formatTime = (value?: string) => {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm')
}

const normalizePresetKeyword = (value?: string) => {
  if (!value) {
    return ''
  }
  return value.replace(/\s+/g, ' ').trim().slice(0, 32)
}

const openCreateForm = (preset?: string) => {
  const keyword = normalizePresetKeyword(preset)
  resetForm(null, { keyword })
}

const openEditForm = (payload?: WorldKeyword | null) => {
  if (!payload) return
  resetForm(payload)
}

defineExpose({
  openCreateForm,
  openEditForm,
})
</script>

<template>
  <n-modal
    :show="props.visible"
    preset="dialog"
    class="world-keyword-manager"
    title="关键词管理"
    :style="{ width: 'min(860px, 96vw)' }"
    @update:show="close"
  >
    <div class="toolbar">
      <div class="toolbar__row">
        <div class="toolbar__actions">
          <n-button type="primary" @click="resetForm()" :disabled="!canEdit">新增关键词</n-button>
          <n-button secondary :disabled="!canEdit" @click="importVisible = true">导入</n-button>
          <n-button secondary @click="handleExport" :loading="exporting">导出 JSON</n-button>
          <n-button
            type="error"
            secondary
            :disabled="!canEdit || !hasSelection"
            :loading="batchDeleting"
            @click="handleBatchDelete"
          >
            批量删除<span v-if="hasSelection">（{{ selectedCount }}）</span>
          </n-button>
        </div>
        <n-input
          v-model:value="searchKeyword"
          size="small"
          clearable
          placeholder="搜索关键词或描述"
          class="toolbar__search"
        />
      </div>
      <n-alert v-if="!canEdit" type="info" :show-icon="false">
        仅世界管理员或成员可维护列表；旁观者可查看但无法编辑。
      </n-alert>
    </div>
    <n-spin :show="loading">
      <n-empty v-if="!filteredKeywordList.length && !loading" description="暂无关键词" />
      <n-table v-else :single-line="false" size="small">
        <thead>
          <tr>
            <th style="width: 48px">
              <n-checkbox
                :checked="isAllSelected"
                :indeterminate="isIndeterminate"
                :disabled="!canEdit || !filteredKeywordList.length"
                @update:checked="toggleSelectAll"
              />
            </th>
            <th style="width: 180px">关键词</th>
            <th>描述</th>
            <th style="width: 180px">更新</th>
            <th style="width: 120px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in filteredKeywordList" :key="item.id">
            <td>
              <n-checkbox
                :checked="selectedIds.includes(item.id)"
                :disabled="!canEdit"
                @update:checked="(val) => toggleSelect(item.id, val)"
              />
            </td>
            <td class="keyword-cell">
              <strong>{{ item.keyword }}</strong>
            </td>
            <td>{{ item.description }}</td>
            <td>
              <div class="meta-text">
                <p>{{ formatTime(item.updatedAt) }}</p>
                <p class="meta-id" v-if="item.updatedBy || item.updatedByName">由 {{ item.updatedByName || item.updatedBy }}</p>
              </div>
            </td>
            <td>
              <n-space size="small" justify="center">
                <n-button text size="small" @click="resetForm(item)" :disabled="!canEdit">编辑</n-button>
                <n-button text size="small" type="error" @click="handleDelete(item)" :disabled="!canEdit">删除</n-button>
              </n-space>
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-spin>
    <template #action>
      <n-space>
        <n-button @click="close">关闭</n-button>
      </n-space>
    </template>
  </n-modal>

  <n-modal
    :show="formVisible"
    preset="dialog"
    :mask-closable="false"
    :title="editing ? '编辑关键词' : '新增关键词'"
    style="max-width: 520px"
    @update:show="formVisible = $event"
  >
    <n-form label-placement="left" label-width="80">
      <n-form-item label="关键词" :feedback="`${form.keyword.length}/32`">
        <n-input v-model:value="form.keyword" :maxlength="32" placeholder="请输入关键词" />
      </n-form-item>
      <n-form-item label="描述" :feedback="`${form.description.length}/200`">
        <n-input
          v-model:value="form.description"
          type="textarea"
          :maxlength="200"
          rows="4"
          placeholder="在此填写关键词说明"
        />
      </n-form-item>
    </n-form>
    <template #action>
      <n-space>
        <n-button quaternary @click="formVisible = false">取消</n-button>
        <n-button type="primary" :loading="formLoading" @click="handleSubmit">保存</n-button>
      </n-space>
    </template>
  </n-modal>

  <n-modal
    :show="importVisible"
    preset="dialog"
    style="max-width: 640px"
    title="导入关键词"
    :mask-closable="false"
    @update:show="importVisible = $event"
  >
    <n-space vertical size="large">
      <n-upload
        accept=".json,.txt,.csv"
        :show-file-list="false"
        :max="1"
        :custom-request="handleImportUpload"
      >
        <n-button secondary>上传文件（JSON/CSV/表格）</n-button>
      </n-upload>
      <n-input
        v-model:value="importForm.content"
        type="textarea"
        rows="10"
        placeholder="粘贴导出的 JSON，或每行以逗号/制表符分隔的“关键词,描述”"
      />
      <n-alert type="info" :show-icon="false">
        支持：<br />
        1）系统导出的 JSON（包含 keywords 字段）；<br />
        2）每行 “关键词,描述” 或 “关键词&lt;tab&gt;描述” 的表格文本；<br />
        3）使用中文逗号、竖线等分隔符的简表。重复关键词会覆盖描述。
      </n-alert>
    </n-space>
    <template #action>
      <n-space>
        <n-button quaternary @click="importVisible = false">取消</n-button>
        <n-button type="primary" :loading="importLoading" @click="handleImport">开始导入</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.world-keyword-manager :deep(.n-modal__content) {
  max-height: 70vh;
  overflow: auto;
}

.toolbar {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 12px;
}

.toolbar__row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.toolbar__actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.toolbar__search {
  min-width: 200px;
  flex: 1;
}

.keyword-cell {
  font-size: 15px;
}

.meta-text {
  font-size: 12px;
  color: #6b7280;
}

.meta-id {
  margin-top: 2px;
}
</style>
watch(
  () => props.canEdit,
  (val) => {
    if (!val) {
      clearSelection()
    }
  },
)
