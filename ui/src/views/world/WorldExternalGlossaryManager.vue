<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { useWorldExternalGlossaryStore } from '@/stores/worldExternalGlossary'
import { matchText } from '@/utils/pinyinMatch'

const chat = useChatStore()
const user = useUserStore()
const store = useWorldExternalGlossaryStore()
const message = useMessage()

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
  <n-drawer v-model:show="drawerVisible" :width="640" placement="right">
    <n-drawer-content title="本世界外挂术语">
      <div class="world-external-glossary">
        <div class="world-external-glossary__toolbar">
          <n-input v-model:value="searchQuery" size="small" clearable placeholder="搜索术语库名称或简介" />
          <n-button size="small" @click="refresh">刷新</n-button>
          <n-button size="small" :disabled="!canManage || !selectedIds.length" @click="handleBulkToggle(true)">批量启用</n-button>
          <n-button size="small" :disabled="!canManage || !selectedIds.length" @click="handleBulkToggle(false)">批量停用</n-button>
        </div>

        <div class="world-external-glossary__table-wrap">
          <table class="world-external-glossary__table">
            <thead>
              <tr>
                <th class="w-10">
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
                <td><input v-model="selectedIds" type="checkbox" :value="item.id"></td>
                <td>
                  <div class="world-external-glossary__name">{{ item.name }}</div>
                  <div class="world-external-glossary__desc">{{ item.description || '无简介' }}</div>
                </td>
                <td>{{ item.termCount }}</td>
                <td>
                  <n-tag size="small" :type="item.isEnabled ? 'success' : 'warning'">
                    {{ item.isEnabled ? '平台启用' : '平台停用' }}
                  </n-tag>
                </td>
                <td>
                  <n-tag size="small" :type="item.isBound ? 'info' : 'default'">
                    {{ item.isBound ? '已启用' : '未启用' }}
                  </n-tag>
                </td>
                <td>
                  <n-button
                    size="tiny"
                    text
                    :disabled="!canManage || (!item.isEnabled && !item.isBound)"
                    @click="handleToggle(item.id, item.isBound)"
                  >
                    {{ item.isBound ? '停用' : '启用' }}
                  </n-button>
                </td>
              </tr>
              <tr v-if="!filteredItems.length">
                <td colspan="6" class="world-external-glossary__empty">暂无可用外挂术语库</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
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
}

.world-external-glossary__table-wrap {
  overflow: auto;
  min-height: 0;
  flex: 1;
}

.world-external-glossary__table {
  width: 100%;
  border-collapse: collapse;
}

.world-external-glossary__table th,
.world-external-glossary__table td {
  padding: 10px 12px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.1);
  vertical-align: top;
  text-align: left;
}

.world-external-glossary__name {
  font-weight: 600;
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
</style>
