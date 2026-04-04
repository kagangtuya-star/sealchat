<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useBreakpoints } from '@vueuse/core'
import { useMessage } from 'naive-ui'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { useWorldExternalGlossaryStore } from '@/stores/worldExternalGlossary'
import { matchText } from '@/utils/pinyinMatch'

const chat = useChatStore()
const user = useUserStore()
const store = useWorldExternalGlossaryStore()
const message = useMessage()
const breakpoints = useBreakpoints({ tablet: 768 })
const isMobileLayout = breakpoints.smaller('tablet')
const drawerWidth = computed(() => (isMobileLayout.value ? '100%' : 640))

const searchQuery = ref('')
const selectedIds = ref<string[]>([])

const drawerVisible = computed({
  get: () => store.managerVisible,
  set: (value: boolean) => store.setManagerVisible(value),
})

const currentWorldId = computed(() => chat.currentWorldId)
const items = computed(() => store.currentLibraries)
const filteredItems = computed(() => {
  const query = searchQuery.value.trim()
  if (!query) return items.value
  return items.value.filter((item) => matchText(query, `${item.name} ${item.description}`))
})

const canManage = computed(() => {
  if (user.checkPerm?.('mod_admin')) return true
  const worldId = currentWorldId.value
  if (!worldId) return false
  const detail = chat.worldDetailMap[worldId]
  return detail?.memberRole === 'owner' || detail?.memberRole === 'admin'
})

async function refresh() {
  if (!currentWorldId.value) return
  await store.ensureLibraries(currentWorldId.value, { force: true })
}

async function handleToggle(libraryId: string, enabled: boolean) {
  const worldId = currentWorldId.value
  if (!worldId) return
  try {
    if (enabled) {
      await store.disableLibrary(worldId, libraryId)
      message.success('已停用外挂术语库')
    } else {
      await store.enableLibrary(worldId, libraryId)
      message.success('已启用外挂术语库')
    }
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '更新外挂术语库失败')
  }
}

async function handleBulkToggle(enabled: boolean) {
  const worldId = currentWorldId.value
  if (!worldId || !selectedIds.value.length) return
  try {
    if (enabled) {
      await store.bulkEnable(worldId, [...selectedIds.value])
      message.success('已批量启用外挂术语库')
    } else {
      await store.bulkDisable(worldId, [...selectedIds.value])
      message.success('已批量停用外挂术语库')
    }
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '批量更新外挂术语库失败')
  }
}

function onSelectAll(event: Event) {
  const target = event.target as HTMLInputElement | null
  selectedIds.value = target?.checked ? filteredItems.value.map((item) => item.id) : []
}

watch(drawerVisible, async (visible) => {
  if (!visible || !currentWorldId.value) return
  await refresh()
})

watch(currentWorldId, async (worldId) => {
  if (!worldId || !drawerVisible.value) return
  await refresh()
})

onMounted(async () => {
  if (drawerVisible.value && currentWorldId.value) {
    await refresh()
  }
})
</script>

<template>
  <n-drawer
    v-model:show="drawerVisible"
    :width="drawerWidth"
    placement="right"
    :mask-closable="true"
    :close-on-esc="true"
    class="world-external-glossary-drawer"
  >
    <n-drawer-content>
      <template #header>
        <div class="world-external-glossary__header">
          <div class="world-external-glossary__title">
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="drawerVisible = false">
              返回
            </n-button>
            <span>本世界外挂术语</span>
          </div>
          <n-button size="tiny" quaternary @click="refresh">刷新</n-button>
        </div>
      </template>

      <div class="world-external-glossary">
        <div class="world-external-glossary__toolbar">
          <n-input v-model:value="searchQuery" size="small" clearable placeholder="搜索术语库名称或简介" />
          <n-button size="small" :disabled="!canManage || !selectedIds.length" @click="handleBulkToggle(true)">批量启用</n-button>
          <n-button size="small" :disabled="!canManage || !selectedIds.length" @click="handleBulkToggle(false)">批量停用</n-button>
        </div>

        <div v-if="isMobileLayout" class="world-external-glossary__mobile-list">
          <article
            v-for="item in filteredItems"
            :key="item.id"
            class="world-external-glossary__card"
          >
            <div class="world-external-glossary__card-top">
              <label class="world-external-glossary__card-check">
                <input v-model="selectedIds" type="checkbox" :value="item.id">
                <span>选择</span>
              </label>
              <n-button
                size="tiny"
                quaternary
                class="world-external-glossary__card-action"
                :disabled="!canManage || (!item.isEnabled && !item.isBound)"
                @click="handleToggle(item.id, item.isBound)"
              >
                {{ item.isBound ? '停用' : '启用' }}
              </n-button>
            </div>

            <div class="world-external-glossary__card-main">
              <div class="world-external-glossary__name">{{ item.name }}</div>
              <div class="world-external-glossary__desc">{{ item.description || '无简介' }}</div>
            </div>

            <div class="world-external-glossary__card-stats">
              <div class="world-external-glossary__metric">
                <span class="world-external-glossary__metric-label">术语数</span>
                <strong class="world-external-glossary__metric-value">{{ item.termCount }}</strong>
              </div>
              <div class="world-external-glossary__status-group">
                <div class="world-external-glossary__status-row">
                  <span class="world-external-glossary__status-label">平台状态</span>
                  <n-tag size="small" :type="item.isEnabled ? 'success' : 'warning'" :bordered="false">
                    {{ item.isEnabled ? '平台启用' : '平台停用' }}
                  </n-tag>
                </div>
                <div class="world-external-glossary__status-row">
                  <span class="world-external-glossary__status-label">世界状态</span>
                  <n-tag size="small" :type="item.isBound ? 'info' : 'default'" :bordered="false">
                    {{ item.isBound ? '已启用' : '未启用' }}
                  </n-tag>
                </div>
              </div>
            </div>
          </article>

          <div v-if="!filteredItems.length" class="world-external-glossary__mobile-empty">
            暂无可用外挂术语库
          </div>
        </div>

        <div v-else class="world-external-glossary__table-wrap">
          <table class="world-external-glossary__table">
            <thead>
              <tr>
                <th class="world-external-glossary__col world-external-glossary__col--select">
                  <input
                    type="checkbox"
                    :checked="filteredItems.length > 0 && selectedIds.length === filteredItems.length"
                    @change="onSelectAll"
                  >
                </th>
                <th>术语库</th>
                <th>术语数</th>
                <th>平台状态</th>
                <th>世界状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredItems" :key="item.id">
                <td
                  class="world-external-glossary__cell world-external-glossary__cell--select"
                  data-label="选择"
                >
                  <input v-model="selectedIds" type="checkbox" :value="item.id">
                </td>
                <td
                  class="world-external-glossary__cell world-external-glossary__cell--library"
                  data-label="术语库"
                >
                  <div class="world-external-glossary__name">{{ item.name }}</div>
                  <div class="world-external-glossary__desc">{{ item.description || '无简介' }}</div>
                </td>
                <td class="world-external-glossary__cell" data-label="术语数">{{ item.termCount }}</td>
                <td class="world-external-glossary__cell" data-label="平台状态">
                  <n-tag size="small" :type="item.isEnabled ? 'success' : 'warning'" :bordered="false">
                    {{ item.isEnabled ? '平台启用' : '平台停用' }}
                  </n-tag>
                </td>
                <td class="world-external-glossary__cell" data-label="世界状态">
                  <n-tag size="small" :type="item.isBound ? 'info' : 'default'" :bordered="false">
                    {{ item.isBound ? '已启用' : '未启用' }}
                  </n-tag>
                </td>
                <td class="world-external-glossary__cell world-external-glossary__cell--action" data-label="操作">
                  <n-button
                    size="tiny"
                    quaternary
                    :disabled="!canManage || (!item.isEnabled && !item.isBound)"
                    @click="handleToggle(item.id, item.isBound)"
                  >
                    {{ item.isBound ? '停用' : '启用' }}
                  </n-button>
                </td>
              </tr>
              <tr v-if="!filteredItems.length">
                <td colspan="6" class="world-external-glossary__empty" data-label="">暂无可用外挂术语库</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.world-external-glossary-drawer :deep(.n-drawer-body-content-wrapper) {
  min-width: 0;
}

.world-external-glossary__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.world-external-glossary__title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-weight: 600;
}

.world-external-glossary__title span {
  font-size: 1.05rem;
  letter-spacing: 0.01em;
}

.world-external-glossary {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
}

.world-external-glossary__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  padding-bottom: 4px;
}

.world-external-glossary__toolbar :deep(.n-input) {
  flex: 1 1 220px;
  min-width: min(100%, 220px);
}

.world-external-glossary__toolbar :deep(.n-input-wrapper) {
  border-radius: 12px;
}

.world-external-glossary__mobile-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.world-external-glossary__card {
  border-radius: 18px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(255, 255, 255, 0.015)),
    rgba(15, 23, 42, 0.34);
  box-shadow: 0 12px 30px rgba(2, 6, 23, 0.16);
  padding: 14px;
}

.world-external-glossary__card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.world-external-glossary__card-check {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text-color-3);
  font-size: 12px;
}

.world-external-glossary__card-action {
  flex-shrink: 0;
}

.world-external-glossary__card-main {
  margin-top: 12px;
}

.world-external-glossary__card-stats {
  margin-top: 14px;
  display: grid;
  gap: 10px;
}

.world-external-glossary__metric {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.26);
}

.world-external-glossary__metric-label,
.world-external-glossary__status-label {
  color: var(--text-color-3);
  font-size: 12px;
}

.world-external-glossary__metric-value {
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--text-color-1);
}

.world-external-glossary__status-group {
  display: grid;
  gap: 8px;
}

.world-external-glossary__status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.2);
}

.world-external-glossary__mobile-empty {
  padding: 28px 12px;
  border-radius: 16px;
  border: 1px dashed rgba(148, 163, 184, 0.18);
  text-align: center;
  color: var(--text-color-3);
}

.world-external-glossary__table-wrap {
  overflow: auto;
  min-height: 0;
  flex: 1;
}

.world-external-glossary__table {
  width: 100%;
  min-width: 560px;
  border-collapse: collapse;
}

.world-external-glossary__table th,
.world-external-glossary__table td {
  padding: 10px 12px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.1);
  vertical-align: top;
  text-align: left;
}

.world-external-glossary__col--select {
  width: 52px;
}

.world-external-glossary__name {
  font-weight: 600;
  line-height: 1.4;
  font-size: 0.98rem;
}

.world-external-glossary__desc {
  margin-top: 4px;
  color: var(--text-color-3);
  font-size: 12px;
  white-space: pre-wrap;
}

.world-external-glossary__empty {
  color: var(--text-color-3);
  text-align: center;
}

@media (max-width: 767px) {
  .world-external-glossary__header {
    align-items: flex-start;
  }

  .world-external-glossary__title {
    gap: 6px;
  }

  .world-external-glossary__title span {
    font-size: 1.2rem;
    font-weight: 700;
  }

  .world-external-glossary__toolbar {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .world-external-glossary__toolbar > :first-child {
    grid-column: 1 / -1;
  }

  .world-external-glossary__toolbar :deep(.n-button) {
    width: 100%;
  }
}
</style>
