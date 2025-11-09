<script setup lang="ts">
import { computed } from 'vue'
import { Palette } from '@vicons/tabler'

interface FilterState {
  icOnly: boolean
  showArchived: boolean
  userIds: string[]
}

interface Member {
  id: string
  nick?: string
  name?: string
}

interface Props {
  filters: FilterState
  members: Member[]
  archiveActive?: boolean
  exportActive?: boolean
  identityActive?: boolean
  galleryActive?: boolean
  displayActive?: boolean
}

interface Emits {
  (e: 'update:filters', filters: FilterState): void
  (e: 'open-archive'): void
  (e: 'open-export'): void
  (e: 'open-identity-manager'): void
  (e: 'open-gallery'): void
  (e: 'open-display-settings'): void
  (e: 'clear-filters'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const memberOptions = computed(() => {
  return props.members.map(member => ({
    label: member.nick || member.name || '未知成员',
    value: member.id,
  }))
})

const activeFiltersCount = computed(() => {
  let count = 0
  if (props.filters.icOnly) count++
  if (props.filters.showArchived) count++
  if (props.filters.userIds.length > 0) count++
  return count
})

const updateFilter = (key: keyof FilterState, value: any) => {
  emit('update:filters', {
    ...props.filters,
    [key]: value,
  })
}

const clearAllFilters = () => {
  emit('clear-filters')
}
</script>

<template>
  <div class="action-ribbon">
    <!-- 筛选区域 -->
    <div class="ribbon-section ribbon-section--filters">
      <div class="filter-group">
        <n-switch
          :value="filters.icOnly"
          @update:value="updateFilter('icOnly', $event)"
          size="small"
        >
          <template #checked>只看场内</template>
          <template #unchecked>全部消息</template>
        </n-switch>
      </div>

      <div class="filter-group">
        <n-switch
          :value="filters.showArchived"
          @update:value="updateFilter('showArchived', $event)"
          size="small"
        >
          <template #checked>显示归档</template>
          <template #unchecked>隐藏归档</template>
        </n-switch>
      </div>

      <div class="filter-group">
        <n-select
          :value="filters.userIds"
          @update:value="updateFilter('userIds', $event)"
          :options="memberOptions"
          multiple
          placeholder="筛选用户"
          size="small"
          style="min-width: 120px"
          clearable
        />
      </div>
    </div>

    <!-- 功能入口区域 -->
    <div class="ribbon-section ribbon-section--actions">
      <n-button-group size="small">
        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.archiveActive }"
          @click="emit('open-archive')"
        >
          <template #icon>
            <n-icon component="ArchiveOutlined" />
          </template>
          消息归档
        </n-button>

        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.exportActive }"
          @click="emit('open-export')"
        >
          <template #icon>
            <n-icon component="DownloadOutlined" />
          </template>
          导出记录
        </n-button>

        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.identityActive }"
          @click="emit('open-identity-manager')"
        >
          <template #icon>
            <n-icon component="UserOutlined" />
          </template>
          角色管理
        </n-button>

        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.displayActive }"
          @click="emit('open-display-settings')"
        >
          <template #icon>
            <n-icon :component="Palette" />
          </template>
          显示模式
        </n-button>

        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.galleryActive }"
          @click="emit('open-gallery')"
        >
          <template #icon>
            <n-icon component="PictureOutlined" />
          </template>
          表情资源
        </n-button>
      </n-button-group>
    </div>

    <!-- 筛选摘要 -->
    <div class="ribbon-section ribbon-section--summary">
      <div v-if="activeFiltersCount > 0" class="filter-summary">
        <n-tag size="small" type="info">
          {{ activeFiltersCount }} 个筛选条件
        </n-tag>
        <n-button text size="tiny" @click="clearAllFilters">
          清除全部
        </n-button>
      </div>
      <div v-else class="filter-summary">
        <span class="text-gray-400 text-sm">无筛选条件</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.action-ribbon {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.75rem 1rem;
  background: rgba(248, 250, 252, 0.95);
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 0.75rem;
  backdrop-filter: blur(8px);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.08);
}

.ribbon-section {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.ribbon-section--filters {
  flex: 1;
}

.ribbon-section--actions {
  flex-shrink: 0;
}

.ribbon-section--summary {
  flex-shrink: 0;
  min-width: 120px;
  justify-content: flex-end;
}

.filter-group {
  display: flex;
  align-items: center;
}

.filter-summary {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.ribbon-action-button {
  transition: background-color 0.2s ease, color 0.2s ease;
  border-radius: 999px;
  padding: 0 0.85rem;
}

.ribbon-action-button.is-active {
  background-color: rgba(59, 130, 246, 0.15);
  color: #1d4ed8;
}

.ribbon-action-button.is-active :deep(.n-icon) {
  color: #2563eb;
}

@media (max-width: 768px) {
  .action-ribbon {
    flex-direction: column;
    align-items: stretch;
    gap: 0.75rem;
  }

  .ribbon-section {
    justify-content: center;
  }

  .ribbon-section--filters {
    flex-wrap: wrap;
  }

  .ribbon-section--summary {
    min-width: auto;
    justify-content: center;
  }
}
</style>
