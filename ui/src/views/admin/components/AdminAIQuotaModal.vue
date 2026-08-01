<script setup lang="ts">
import { useUtilsStore } from '@/stores/utils'
import type { AdminAIQuotaDetail, AdminAIQuotaListResult, AIQuotaPolicyConfig, UserInfo } from '@/types'
import { Refresh, Search } from '@vicons/tabler'
import { NTag, useMessage, type DataTableColumns } from 'naive-ui'
import { computed, h, ref, watch } from 'vue'

const props = defineProps<{
  show: boolean;
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
}>()

const utils = useUtilsStore()
const message = useMessage()

const listLoading = ref(false)
const rows = ref<AdminAIQuotaDetail[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const keyword = ref('')
const selectedUserId = ref<string | null>(null)
const detailLoading = ref(false)
const detail = ref<AdminAIQuotaDetail | null>(null)
const saving = ref(false)
const deleting = ref(false)
const userSearchLoading = ref(false)
const userSearchValue = ref<string | null>(null)
const userOptions = ref<Array<{ label: string; value: string }>>([])
const draftPolicy = ref<AIQuotaPolicyConfig>({
  dailyLimit: null,
  monthlyLimit: null,
  lifetimeLimit: null,
})

let listSearchTimer: ReturnType<typeof setTimeout> | null = null
let userSearchTimer: ReturnType<typeof setTimeout> | null = null

const sourceLabelMap: Record<AdminAIQuotaDetail['source'], string> = {
  default: '默认规则',
  override: '单独覆盖',
}

const sourceTypeMap: Record<AdminAIQuotaDetail['source'], 'info' | 'warning'> = {
  default: 'info',
  override: 'warning',
}

const selectedSourceLabel = computed(() => (detail.value ? sourceLabelMap[detail.value.source] : '-'))

const calcUsagePercent = (used?: number | null, limit?: number | null) => {
  if (typeof used !== 'number' || Number.isNaN(used) || typeof limit !== 'number' || Number.isNaN(limit) || limit <= 0) {
    return null
  }
  return Math.min(100, Math.max(0, Number(((used / limit) * 100).toFixed(2))))
}

const quotaProgressItems = computed(() => {
  if (!detail.value) return []
  return [
    {
      key: 'daily',
      label: '日额度',
      used: detail.value.usage.dailySettled,
      limit: detail.value.effectivePolicy.dailyLimit ?? null,
      percent: calcUsagePercent(detail.value.usage.dailySettled, detail.value.effectivePolicy.dailyLimit ?? null),
    },
    {
      key: 'monthly',
      label: '月额度',
      used: detail.value.usage.monthlySettled,
      limit: detail.value.effectivePolicy.monthlyLimit ?? null,
      percent: calcUsagePercent(detail.value.usage.monthlySettled, detail.value.effectivePolicy.monthlyLimit ?? null),
    },
    {
      key: 'lifetime',
      label: '总额度',
      used: detail.value.usage.lifetimeSettled,
      limit: detail.value.effectivePolicy.lifetimeLimit ?? null,
      percent: calcUsagePercent(detail.value.usage.lifetimeSettled, detail.value.effectivePolicy.lifetimeLimit ?? null),
    },
  ]
})

const columns = computed<DataTableColumns<AdminAIQuotaDetail>>(() => [
  {
    title: '用户',
    key: 'username',
    minWidth: 220,
    render: (row) => h('div', { class: 'ai-quota-modal__user-cell' }, [
      h('strong', row.nickname || row.username || row.userId),
      h('span', row.username || row.userId),
      h('code', row.userId),
    ]),
  },
  {
    title: '当前来源',
    key: 'source',
    width: 108,
    render: (row) => h(
      NTag,
      { size: 'small', type: sourceTypeMap[row.source] },
      { default: () => sourceLabelMap[row.source] },
    ),
  },
  {
    title: '日额度',
    key: 'dailyLimit',
    width: 110,
    render: (row) => formatLimit(row.override?.dailyLimit),
  },
  {
    title: '月额度',
    key: 'monthlyLimit',
    width: 110,
    render: (row) => formatLimit(row.override?.monthlyLimit),
  },
  {
    title: '总额度',
    key: 'lifetimeLimit',
    width: 110,
    render: (row) => formatLimit(row.override?.lifetimeLimit),
  },
])

watch(
  () => props.show,
  (show) => {
    if (!show) return
    void refreshList()
    if (selectedUserId.value) {
      void loadDetail(selectedUserId.value)
    }
  },
)

const closeModal = () => {
  emit('update:show', false)
}

const extractErrorMessage = (error: any, fallback: string) => {
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || fallback
}

const formatAmount = (value?: number | null) => {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-'
  return value.toFixed(4)
}

const formatLimit = (value?: number | null) => {
  if (typeof value !== 'number') return '未设'
  return formatAmount(value)
}

const buildUserOption = (user: UserInfo) => {
  const nick = user.nick?.trim() || ''
  const username = user.username?.trim() || user.id
  const primary = nick || username || user.id
  return {
    value: user.id,
    label: `${primary} · ${username} · ${user.id}`,
  }
}

const resetDraftPolicy = (item: AdminAIQuotaDetail | null) => {
  draftPolicy.value = {
    dailyLimit: item?.override?.dailyLimit ?? null,
    monthlyLimit: item?.override?.monthlyLimit ?? null,
    lifetimeLimit: item?.override?.lifetimeLimit ?? null,
  }
}

const refreshList = async () => {
  listLoading.value = true
  try {
    const resp = await utils.adminAIQuotaList({
      page: page.value,
      pageSize: pageSize.value,
      query: keyword.value.trim() || undefined,
    })
    const data = resp.data as AdminAIQuotaListResult
    rows.value = data.items || []
    total.value = Number(data.total || 0)
    if (selectedUserId.value && rows.value.some((item) => item.userId === selectedUserId.value)) {
      return
    }
    if (!selectedUserId.value && rows.value.length) {
      selectedUserId.value = rows.value[0].userId
      void loadDetail(rows.value[0].userId)
    }
  } catch (error) {
    message.error(extractErrorMessage(error, '读取 AI 配额覆盖列表失败'))
  } finally {
    listLoading.value = false
  }
}

const loadDetail = async (userId: string) => {
  if (!userId) return
  selectedUserId.value = userId
  userSearchValue.value = userId
  detailLoading.value = true
  try {
    const resp = await utils.adminAIQuotaGet(userId)
    detail.value = resp.data as AdminAIQuotaDetail
    resetDraftPolicy(detail.value)
  } catch (error) {
    detail.value = null
    resetDraftPolicy(null)
    message.error(extractErrorMessage(error, '读取用户 AI 配额失败'))
  } finally {
    detailLoading.value = false
  }
}

const handleSearchInput = () => {
  if (listSearchTimer) clearTimeout(listSearchTimer)
  listSearchTimer = setTimeout(() => {
    page.value = 1
    void refreshList()
  }, 250)
}

const handlePageChange = (nextPage: number) => {
  page.value = nextPage
  void refreshList()
}

const handlePageSizeChange = (nextPageSize: number) => {
  pageSize.value = nextPageSize
  page.value = 1
  void refreshList()
}

const rowProps = (row: AdminAIQuotaDetail) => {
  return {
    style: 'cursor: var(--sc-cursor-pointer, pointer);',
    onClick: () => {
      void loadDetail(row.userId)
    },
  }
}

const searchUsers = async (keywordText: string) => {
  const query = keywordText.trim()
  if (!query) {
    userOptions.value = []
    return
  }
  userSearchLoading.value = true
  try {
    const resp = await utils.adminUserList({
      page: 1,
      pageSize: 20,
      keyword: query,
    })
    userOptions.value = (resp.data.items || []).map(buildUserOption)
  } catch (error) {
    message.error(extractErrorMessage(error, '搜索用户失败'))
  } finally {
    userSearchLoading.value = false
  }
}

const handleUserSearch = (keywordText: string) => {
  if (userSearchTimer) clearTimeout(userSearchTimer)
  userSearchTimer = setTimeout(() => {
    void searchUsers(keywordText)
  }, 250)
}

const handleUserSelect = (userId: string | null) => {
  if (!userId) return
  void loadDetail(userId)
}

const updateDraftLimit = (key: keyof AIQuotaPolicyConfig, value: number | null) => {
  draftPolicy.value = {
    ...draftPolicy.value,
    [key]: value,
  }
}

const saveOverride = async () => {
  if (!selectedUserId.value) {
    message.warning('请先选择用户')
    return
  }
  const payload: AIQuotaPolicyConfig = {
    dailyLimit: draftPolicy.value.dailyLimit ?? null,
    monthlyLimit: draftPolicy.value.monthlyLimit ?? null,
    lifetimeLimit: draftPolicy.value.lifetimeLimit ?? null,
  }
  if (payload.dailyLimit == null && payload.monthlyLimit == null && payload.lifetimeLimit == null) {
    message.warning('至少填写一个额度字段')
    return
  }
  saving.value = true
  try {
    const resp = await utils.adminAIQuotaUpsert(selectedUserId.value, payload)
    detail.value = resp.data as AdminAIQuotaDetail
    resetDraftPolicy(detail.value)
    message.success('AI 用户配额覆盖已保存')
    await refreshList()
  } catch (error) {
    message.error(extractErrorMessage(error, '保存 AI 用户配额失败'))
  } finally {
    saving.value = false
  }
}

const clearOverride = async () => {
  if (!selectedUserId.value) {
    message.warning('请先选择用户')
    return
  }
  deleting.value = true
  try {
    await utils.adminAIQuotaDelete(selectedUserId.value)
    message.success('AI 用户配额覆盖已删除')
    await loadDetail(selectedUserId.value)
    await refreshList()
  } catch (error) {
    message.error(extractErrorMessage(error, '删除 AI 用户配额覆盖失败'))
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="用户 AI 配额"
    class="ai-quota-modal sc-fluid-modal sc-fluid-modal--xwide"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <div class="ai-quota-modal__toolbar">
      <div class="ai-quota-modal__search">
        <n-input
          v-model:value="keyword"
          clearable
          placeholder="搜索已设置覆盖值的用户"
          @input="handleSearchInput"
          @clear="handleSearchInput"
        >
          <template #prefix>
            <n-icon :component="Search" />
          </template>
        </n-input>
        <n-button :loading="listLoading" @click="refreshList">
          <template #icon>
            <n-icon :component="Refresh" />
          </template>
          刷新
        </n-button>
      </div>

      <n-select
        v-model:value="userSearchValue"
        clearable
        filterable
        remote
        placeholder="搜索任意用户并载入详情"
        :options="userOptions"
        :loading="userSearchLoading"
        @search="handleUserSearch"
        @update:value="handleUserSelect"
      />
    </div>

    <div class="ai-quota-modal__layout">
      <section class="ai-quota-modal__list-card">
        <div class="ai-quota-modal__list-head">
          <div>
            <strong>覆盖列表</strong>
            <p>仅展示已设置单用户覆盖的用户</p>
          </div>
          <span>共 {{ total }} 条</span>
        </div>
        <div class="ai-quota-modal__table-scroll sc-modal-table-scroll">
          <n-data-table
            :columns="columns"
            :data="rows"
            :loading="listLoading"
            :pagination="false"
            :row-key="(row: AdminAIQuotaDetail) => row.userId"
            :row-props="rowProps"
            :max-height="420"
            :scroll-x="680"
            size="small"
          />
        </div>
        <div class="ai-quota-modal__pagination">
          <n-pagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[10, 20, 50]"
            show-size-picker
            :on-update:page="handlePageChange"
            :on-update:page-size="handlePageSizeChange"
          />
        </div>
      </section>

      <section class="ai-quota-modal__detail-card">
        <div class="ai-quota-modal__detail-head">
          <div>
            <strong>{{ detail?.nickname || detail?.username || detail?.userId || '未选择用户' }}</strong>
            <p>{{ detail?.username || '左侧选择已设覆盖用户，或右上搜索任意用户' }}</p>
          </div>
          <n-tag v-if="detail" size="small" :type="sourceTypeMap[detail.source]">
            {{ selectedSourceLabel }}
          </n-tag>
        </div>

        <n-skeleton v-if="detailLoading" text :repeat="8" />
        <template v-else-if="detail">
          <div class="ai-quota-modal__identity">
            <span class="ai-quota-modal__identity-id">ID: {{ detail.userId }}</span>
            <span class="ai-quota-modal__identity-side">当前策略：{{ selectedSourceLabel }}</span>
            <span class="ai-quota-modal__identity-side">预占中：{{ formatAmount(detail.usage.activeReserved) }}</span>
          </div>

          <div class="ai-quota-modal__progress-grid">
            <section
              v-for="item in quotaProgressItems"
              :key="item.key"
              class="ai-quota-modal__progress-card"
            >
              <div class="ai-quota-modal__progress-head">
                <strong>{{ item.label }}</strong>
                <span v-if="item.percent != null">{{ item.percent.toFixed(2) }}%</span>
                <span v-else>未设上限</span>
              </div>
              <div class="ai-quota-modal__progress-meta">
                <span>已用 {{ formatAmount(item.used) }}</span>
                <span>上限 {{ formatLimit(item.limit) }}</span>
              </div>
              <n-progress
                type="line"
                :percentage="item.percent ?? 0"
                :indicator-placement="'inside'"
                :processing="false"
                :show-indicator="item.percent != null"
                :height="14"
                :rail-color="'rgba(148, 163, 184, 0.18)'"
                :color="item.percent != null && item.percent >= 90 ? '#ef4444' : item.percent != null && item.percent >= 70 ? '#f59e0b' : '#38bdf8'"
              />
            </section>
          </div>

          <div class="ai-quota-modal__policy-grid">
            <section class="ai-quota-modal__policy-card">
              <strong>默认规则</strong>
              <span>日 {{ formatLimit(detail.defaultPolicy.dailyLimit) }}</span>
              <span>月 {{ formatLimit(detail.defaultPolicy.monthlyLimit) }}</span>
              <span>总 {{ formatLimit(detail.defaultPolicy.lifetimeLimit) }}</span>
            </section>
            <section class="ai-quota-modal__policy-card">
              <strong>当前覆盖</strong>
              <span>日 {{ formatLimit(detail.override?.dailyLimit) }}</span>
              <span>月 {{ formatLimit(detail.override?.monthlyLimit) }}</span>
              <span>总 {{ formatLimit(detail.override?.lifetimeLimit) }}</span>
            </section>
            <section class="ai-quota-modal__policy-card">
              <strong>生效规则</strong>
              <span>日 {{ formatLimit(detail.effectivePolicy.dailyLimit) }}</span>
              <span>月 {{ formatLimit(detail.effectivePolicy.monthlyLimit) }}</span>
              <span>总 {{ formatLimit(detail.effectivePolicy.lifetimeLimit) }}</span>
            </section>
          </div>

          <div class="ai-quota-modal__editor">
            <n-grid :cols="3" x-gap="12" responsive="screen">
              <n-gi>
                <n-form-item label="日额度">
                  <n-input-number
                    clearable
                    :value="draftPolicy.dailyLimit ?? null"
                    :min="0"
                    :precision="4"
                    placeholder="为空则不设"
                    @update:value="(value: number | null) => updateDraftLimit('dailyLimit', value)"
                  />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="月额度">
                  <n-input-number
                    clearable
                    :value="draftPolicy.monthlyLimit ?? null"
                    :min="0"
                    :precision="4"
                    placeholder="为空则不设"
                    @update:value="(value: number | null) => updateDraftLimit('monthlyLimit', value)"
                  />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="总额度">
                  <n-input-number
                    clearable
                    :value="draftPolicy.lifetimeLimit ?? null"
                    :min="0"
                    :precision="4"
                    placeholder="为空则不设"
                    @update:value="(value: number | null) => updateDraftLimit('lifetimeLimit', value)"
                  />
                </n-form-item>
              </n-gi>
            </n-grid>

            <div class="ai-quota-modal__editor-actions">
              <n-button type="primary" :loading="saving" @click="saveOverride">保存覆盖值</n-button>
              <n-button :disabled="!detail.override" :loading="deleting" @click="clearOverride">删除覆盖值</n-button>
            </div>
          </div>
        </template>
        <n-empty v-else class="ai-quota-modal__empty" description="左侧选择已设覆盖用户，或搜索任意用户开始设置" />
      </section>
    </div>

    <template #footer>
      <div class="ai-quota-modal__footer">
        <n-button @click="closeModal">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.ai-quota-modal__toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 360px);
  gap: 12px;
  margin-bottom: 12px;
}

.ai-quota-modal__search {
  display: flex;
  gap: 8px;
}

.ai-quota-modal__layout {
  display: flex;
  gap: 12px;
  align-items: stretch;
  flex-wrap: wrap;
}

.ai-quota-modal__layout > * {
  flex: 1 1 420px;
  min-width: 0;
}

.ai-quota-modal__list-card {
  min-width: 360px;
}

.ai-quota-modal__detail-card {
  min-width: 420px;
}

.ai-quota-modal__list-card,
.ai-quota-modal__detail-card {
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  padding: 12px;
  background: var(--n-card-color);
  min-height: 0;
  overflow: hidden;
}

.ai-quota-modal__list-head,
.ai-quota-modal__detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}

.ai-quota-modal__list-head p,
.ai-quota-modal__detail-head p {
  margin: 4px 0 0;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.ai-quota-modal__user-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ai-quota-modal__user-cell span,
.ai-quota-modal__user-cell code {
  color: var(--n-text-color-3);
  font-size: 12px;
}

.ai-quota-modal__pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.ai-quota-modal__table-scroll {
  min-width: 0;
}

.ai-quota-modal__identity {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 14px;
  margin-bottom: 14px;
}

.ai-quota-modal__identity-id,
.ai-quota-modal__identity-side {
  color: var(--n-text-color-3);
  font-size: 12px;
}

.ai-quota-modal__progress-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}

.ai-quota-modal__progress-card {
  padding: 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 10px;
  background: color-mix(in srgb, var(--n-card-color) 90%, var(--n-primary-color) 10%);
}

.ai-quota-modal__progress-head,
.ai-quota-modal__progress-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.ai-quota-modal__progress-meta {
  margin: 6px 0 10px;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.ai-quota-modal__policy-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-top: 14px;
}

.ai-quota-modal__policy-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 10px;
  background: color-mix(in srgb, var(--n-card-color) 88%, var(--n-primary-color) 12%);
}

.ai-quota-modal__policy-card span {
  color: var(--n-text-color-2);
  font-size: 12px;
}

.ai-quota-modal__editor {
  margin-top: 14px;
}

.ai-quota-modal__editor-actions,
.ai-quota-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.ai-quota-modal__empty {
  min-height: 320px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ai-quota-modal__empty :deep(.n-empty) {
  width: 100%;
}

.ai-quota-modal__empty :deep(.n-empty-description) {
  white-space: normal;
  word-break: break-word;
  text-align: center;
}

@media (max-width: 900px) {
  .ai-quota-modal__toolbar {
    grid-template-columns: 1fr;
  }

  .ai-quota-modal__progress-grid,
  .ai-quota-modal__policy-grid {
    grid-template-columns: 1fr;
  }
}
</style>
