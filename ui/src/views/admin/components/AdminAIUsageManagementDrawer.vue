<script setup lang="ts">
import { useUtilsStore } from '@/stores/utils'
import { speechAPI } from '@/features/tts/api'
import type { AdminAIUsageLogItem, AdminAIUsageLogListResult, AIConfig } from '@/types'
import { Refresh, Search, Trash } from '@vicons/tabler'
import { NTag, useDialog, useMessage, type DataTableColumns } from 'naive-ui'
import { computed, h, ref, watch } from 'vue'

const props = defineProps<{
  show: boolean;
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
  (e: 'open-quota-management'): void;
}>()

const utils = useUtilsStore()
const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const quotaKind = ref<'text' | 'speech'>('text')
let requestRevision = 0
const rows = ref<AdminAIUsageLogItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const query = ref('')
const featureKey = ref<string | null>(null)
const providerId = ref<string | null>(null)
const modelName = ref('')
const status = ref<string | null>(null)
const timeRange = ref<[number, number] | null>(null)
const cleanupLoading = ref(false)
const cleanupDays = ref<number | null>(30)
const configMeta = ref<AIConfig | null>(null)

let searchTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.show,
  (show) => {
    if (!show) return
    void loadMeta()
    void refresh()
  },
)

const statusOptions = [
  { label: '全部状态', value: null },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'error' },
]

const featureOptions = computed(() => {
  if (quotaKind.value === 'speech') return [
    { label: '全部语音操作', value: null },
    ...['message_synthesis', 'audition', 'design', 'clone'].map(value => ({ label: value, value })),
  ]
  const items = Object.keys(configMeta.value?.features || {})
  return [
    { label: '全部功能', value: null },
    ...items.map((item) => ({ label: item, value: item })),
  ]
})

const providerOptions = computed(() => {
  const items = configMeta.value?.providers || []
  return [
    { label: '全部 Provider', value: null },
    ...items.map((item) => ({ label: `${item.name} (${item.id})`, value: item.id })),
  ]
})

const columns = computed<DataTableColumns<AdminAIUsageLogItem>>(() => [
  {
    title: '用户',
    key: 'usernameSnapshot',
    minWidth: 220,
    render: (row) => h('div', { class: 'admin-ai-usage__user-cell' }, [
      h('strong', row.nicknameSnapshot || row.usernameSnapshot || '-'),
      h('span', { class: 'admin-ai-usage__subtle' }, `ID: ${row.userId}`),
    ]),
  },
  {
    title: '功能',
    key: 'featureKey',
    minWidth: 120,
    render: (row) => h('div', { class: 'admin-ai-usage__model-cell' }, [
      h('strong', row.featureKey),
    ]),
  },
  {
    title: '模型',
    key: 'model',
    minWidth: 220,
    render: (row) => h('div', { class: 'admin-ai-usage__model-cell' }, [
      h('span', { class: 'admin-ai-usage__subtle' }, row.providerId),
      h('span', row.model),
    ]),
  },
  {
    title: '状态',
    key: 'status',
    width: 92,
    render: (row) => h(
      NTag,
      { size: 'small', type: row.status === 'success' ? 'success' : 'error' },
      { default: () => row.status === 'success' ? '成功' : row.status },
    ),
  },
  ...(quotaKind.value === 'speech' ? [
    { title: '计费字符 / 创建次数', key: 'billingUnits', width: 150 },
    { title: '确认单价', key: 'unitPrice', width: 100 },
  ] : [
    { title: '输入', key: 'promptTokens', width: 88 },
    { title: '输出', key: 'completionTokens', width: 88 },
    { title: '缓存', key: 'cacheTokens', width: 88 },
  ]),
  {
    title: '消耗',
    key: 'totalCost',
    width: 96,
    render: (row) => formatAmount(row.totalCost),
  },
  {
    title: '耗时',
    key: 'latencyMs',
    width: 92,
    render: (row) => `${row.latencyMs} ms`,
  },
  {
    title: '调用时间',
    key: 'startedAt',
    width: 170,
    render: (row) => formatDateTime(row.startedAt || row.finishedAt),
  },
])

const closeDrawer = () => {
  emit('update:show', false)
}

const extractErrorMessage = (error: any, fallback: string) => {
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || fallback
}

const formatAmount = (value?: number | null) => {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-'
  return value.toFixed(4)
}

const formatDateTime = (value?: string | null) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

const buildParams = () => {
  const params: Record<string, string | number | undefined> = {
    page: page.value,
    pageSize: pageSize.value,
    query: query.value.trim() || undefined,
    featureKey: featureKey.value || undefined,
    providerId: providerId.value || undefined,
    model: modelName.value.trim() || undefined,
    status: status.value || undefined,
  }
  if (timeRange.value) {
    params.start = timeRange.value[0]
    params.end = timeRange.value[1]
  }
  return params
}

const loadMeta = async () => {
  try {
    const resp = await utils.adminAIConfigGet()
    configMeta.value = resp.data?.config || null
    if (typeof configMeta.value?.logRetentionDays === 'number' && configMeta.value.logRetentionDays > 0) {
      cleanupDays.value = configMeta.value.logRetentionDays
    }
  } catch (error) {
    message.error(extractErrorMessage(error, '读取 AI 配置失败'))
  }
}

const refresh = async () => {
  const revision = ++requestRevision
  loading.value = true
  try {
    const resp = quotaKind.value === 'speech' ? await speechAPI.logs(buildParams()) : await utils.adminAIUsageLogs(buildParams())
    if (revision !== requestRevision) return
    const data = resp.data as AdminAIUsageLogListResult
    rows.value = data.items || []
    total.value = Number(data.total || 0)
  } catch (error) {
    message.error(extractErrorMessage(error, '读取 AI 调用日志失败'))
  } finally {
    if (revision === requestRevision) loading.value = false
  }
}

const handleSearchInput = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    void refresh()
  }, 250)
}

const applyFilters = () => {
  page.value = 1
  void refresh()
}

const resetFilters = () => {
  query.value = ''
  featureKey.value = null
  providerId.value = null
  modelName.value = ''
  status.value = null
  timeRange.value = null
  page.value = 1
  void refresh()
}

const handlePageChange = (nextPage: number) => {
  page.value = nextPage
  void refresh()
}

const handlePageSizeChange = (nextPageSize: number) => {
  pageSize.value = nextPageSize
  page.value = 1
  void refresh()
}

const executeCleanup = async () => {
  cleanupLoading.value = true
  try {
    const resp = quotaKind.value === 'speech' ? await speechAPI.cleanupLogs(cleanupDays.value ?? undefined) : await utils.adminAIUsageLogsCleanup({
      retentionDays: cleanupDays.value ?? undefined,
    })
    message.success(`已清理 ${resp.data?.affectedRows || 0} 条日志`)
    await refresh()
  } catch (error) {
    message.error(extractErrorMessage(error, '清理 AI 调用日志失败'))
  } finally {
    cleanupLoading.value = false
  }
}

const confirmCleanup = () => {
  dialog.warning({
    title: '立即清理 AI 调用日志',
    content: `将按保留 ${cleanupDays.value || 30} 天规则立即清理旧日志。账本与已扣额度不会回退。`,
    positiveText: '立即清理',
    negativeText: '取消',
    onPositiveClick: async () => {
      await executeCleanup()
    },
  })
}
</script>

<template>
  <n-drawer
    :show="show"
    :width="'min(1380px, 96vw)'"
    placement="right"
    class="admin-ai-usage-drawer"
    @update:show="emit('update:show', $event)"
  >
    <n-drawer-content closable body-content-style="padding: 0;">
      <template #header>
        <div class="admin-ai-usage__header">
          <div>
            <strong>AI 调用日志与清理</strong>
            <p>仅统计平台内置 AI 调用；日志清理不回滚用户额度。</p>
          </div>
          <div class="admin-ai-usage__header-actions">
            <n-button tertiary @click="emit('open-quota-management')">用户配额管理</n-button>
            <n-button quaternary @click="closeDrawer">退出</n-button>
          </div>
        </div>
      </template>

      <div class="admin-ai-usage__body">
        <div class="admin-ai-usage__toolbar">
          <div class="admin-ai-usage__toolbar-main">
            <n-input
              v-model:value="query"
              clearable
              placeholder="搜索用户名 / 用户ID / 昵称"
              @input="handleSearchInput"
              @clear="handleSearchInput"
            >
              <template #prefix>
                <n-icon :component="Search" />
              </template>
            </n-input>
            <n-select v-model:value="featureKey" :options="featureOptions" @update:value="applyFilters" />
            <n-select v-model:value="providerId" :options="providerOptions" @update:value="applyFilters" />
            <n-select v-model:value="status" :options="statusOptions" @update:value="applyFilters" />
            <n-input
              v-model:value="modelName"
              clearable
              placeholder="按 model 过滤"
              @input="handleSearchInput"
              @clear="handleSearchInput"
            />
            <n-date-picker
              v-model:value="timeRange"
              clearable
              type="daterange"
              :actions="['clear', 'confirm']"
              @update:value="applyFilters"
            />
          </div>

          <div class="admin-ai-usage__toolbar-side">
            <n-input-number v-model:value="cleanupDays" :min="1" :precision="0" placeholder="保留天数" />
            <n-button :loading="loading" @click="refresh">
              <template #icon>
                <n-icon :component="Refresh" />
              </template>
              刷新
            </n-button>
            <n-button :loading="cleanupLoading" type="error" @click="confirmCleanup">
              <template #icon>
                <n-icon :component="Trash" />
              </template>
              立即清理
            </n-button>
            <n-button tertiary @click="resetFilters">重置筛选</n-button>
          </div>
        </div>

        <div class="admin-ai-usage__summary">
          <n-select v-model:value="quotaKind" style="width: 160px" :options="[{ label: '文本金额日志', value: 'text' }, { label: '语音金额日志', value: 'speech' }]" @update:value="resetFilters" />
          <n-tag size="small" type="info">当前保留 {{ cleanupDays || 30 }} 天</n-tag>
          <span>共 {{ total }} 条日志</span>
        </div>

        <div class="admin-ai-usage__table">
          <n-data-table
            :columns="columns"
            :data="rows"
            :loading="loading"
            :pagination="false"
            :row-key="(row: AdminAIUsageLogItem) => row.id"
            :max-height="620"
            :scroll-x="1200"
            size="small"
          />
        </div>

        <div class="admin-ai-usage__pagination">
          <n-pagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[20, 50, 100]"
            show-size-picker
            :on-update:page="handlePageChange"
            :on-update:page-size="handlePageSizeChange"
          />
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.admin-ai-usage__header {
  width: 100%;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.admin-ai-usage__header p {
  margin: 4px 0 0;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.admin-ai-usage__header-actions {
  display: flex;
  gap: 8px;
}

.admin-ai-usage__body {
  height: calc(100vh - 96px);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.admin-ai-usage__toolbar {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.admin-ai-usage__toolbar-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}

.admin-ai-usage__toolbar-main > * {
  flex: 0 1 auto;
}

.admin-ai-usage__toolbar-main > :first-child {
  flex: 1 1 260px;
  min-width: 220px;
}

.admin-ai-usage__toolbar-main :deep(.n-base-selection),
.admin-ai-usage__toolbar-main :deep(.n-date-picker),
.admin-ai-usage__toolbar-main :deep(.n-input) {
  min-width: 0;
}

.admin-ai-usage__toolbar-main > :nth-child(2),
.admin-ai-usage__toolbar-main > :nth-child(3),
.admin-ai-usage__toolbar-main > :nth-child(5) {
  flex: 1 1 180px;
  min-width: 180px;
}

.admin-ai-usage__toolbar-main > :nth-child(4) {
  flex: 0 1 140px;
  min-width: 140px;
}

.admin-ai-usage__toolbar-main > :last-child {
  flex: 1 1 280px;
  min-width: 260px;
}

.admin-ai-usage__toolbar-side {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.admin-ai-usage__toolbar-side :deep(.n-input-number) {
  width: 140px;
}

.admin-ai-usage__summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.admin-ai-usage__table {
  flex: 1;
  min-height: 0;
}

.admin-ai-usage__pagination {
  display: flex;
  justify-content: flex-end;
}

.admin-ai-usage__user-cell,
.admin-ai-usage__model-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.admin-ai-usage__user-cell span,
.admin-ai-usage__model-cell span {
  color: var(--n-text-color-3);
  font-size: 12px;
}

.admin-ai-usage__subtle {
  color: var(--n-text-color-3);
  font-size: 12px;
}

.admin-ai-usage-drawer :deep(.n-drawer-content) {
  overflow: hidden;
}

@media (max-width: 1100px) {
  .admin-ai-usage__toolbar-side {
    justify-content: flex-start;
  }
}

@media (max-width: 768px) {
  .admin-ai-usage__body {
    height: calc(100vh - 88px);
    padding: 12px;
  }

  .admin-ai-usage__toolbar-main {
    flex-direction: column;
    align-items: stretch;
  }

  .admin-ai-usage__toolbar-main > *,
  .admin-ai-usage__toolbar-main > :first-child,
  .admin-ai-usage__toolbar-main > :nth-child(2),
  .admin-ai-usage__toolbar-main > :nth-child(3),
  .admin-ai-usage__toolbar-main > :nth-child(4),
  .admin-ai-usage__toolbar-main > :nth-child(5),
  .admin-ai-usage__toolbar-main > :last-child {
    flex: 1 1 auto;
    min-width: 0;
  }

  .admin-ai-usage__header,
  .admin-ai-usage__summary {
    flex-direction: column;
    align-items: flex-start;
  }

  .admin-ai-usage__toolbar-side {
    justify-content: flex-start;
  }
}
</style>
