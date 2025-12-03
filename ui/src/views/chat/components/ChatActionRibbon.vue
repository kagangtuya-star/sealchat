<script setup lang="ts">
import { computed } from 'vue'
import {
  Archive as ArchiveIcon,
  Download as DownloadIcon,
  MoodSmile as EmojiIcon,
  Palette,
  Star as StarIcon,
  Users as UsersIcon,
} from '@vicons/tabler'

interface FilterState {
  icOnly: boolean
  showArchived: boolean
  roleIds: string[]
}

interface RoleOption {
  id: string
  label?: string
  name?: string
}

interface Props {
  filters: FilterState
  roles: RoleOption[]
  archiveActive?: boolean
  exportActive?: boolean
  identityActive?: boolean
  galleryActive?: boolean
  displayActive?: boolean
  favoriteActive?: boolean
}

interface Emits {
  (e: 'update:filters', filters: FilterState): void
  (e: 'open-archive'): void
  (e: 'open-export'): void
  (e: 'open-identity-manager'): void
  (e: 'open-gallery'): void
  (e: 'open-display-settings'): void
  (e: 'open-favorites'): void
  (e: 'clear-filters'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const roleSelectOptions = computed(() => {
  return props.roles.map(role => ({
    label: role.label || role.name || '未命名角色',
    value: role.id,
  }))
})

const activeFiltersCount = computed(() => {
  let count = 0
  if (props.filters.icOnly) count++
  if (props.filters.showArchived) count++
  if (props.filters.roleIds.length > 0) count++
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
          :value="filters.roleIds"
          @update:value="updateFilter('roleIds', $event)"
          :options="roleSelectOptions"
          multiple
          placeholder="筛选角色"
          size="small"
          style="min-width: 120px"
          clearable
        />
      </div>
    </div>

    <!-- 功能入口区域 -->
    <div class="ribbon-section ribbon-section--actions">
      <div class="ribbon-actions-grid">
        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.archiveActive }"
          @click="emit('open-archive')"
        >
          <template #icon>
            <n-icon :component="ArchiveIcon" />
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
            <n-icon :component="DownloadIcon" />
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
            <n-icon :component="UsersIcon" />
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
          :class="{ 'is-active': props.favoriteActive }"
          @click="emit('open-favorites')"
        >
          <template #icon>
            <n-icon :component="StarIcon" />
          </template>
          频道收藏
        </n-button>

        <n-button
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props.galleryActive }"
          @click="emit('open-gallery')"
        >
          <template #icon>
            <n-icon :component="EmojiIcon" />
          </template>
          表情资源
        </n-button>

      </div>
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
  padding: 0.9rem 1.1rem;
  background: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-strong);
  border-radius: 0.75rem;
  color: var(--sc-text-primary);
  box-shadow: 0 12px 24px rgba(15, 23, 42, 0.06);
  transition: background-color 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
}

:root[data-display-palette='night'] .action-ribbon {
  box-shadow: 0 14px 32px rgba(0, 0, 0, 0.55);
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
  width: 100%;
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
  color: var(--sc-text-secondary);
}

.ribbon-action-button {
  transition: background-color 0.2s ease, color 0.2s ease;
  border-radius: 999px;
  padding: 0 0.85rem;
  color: var(--sc-text-primary);
  border: 1px solid transparent;
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  background-color: transparent;
}

.ribbon-action-button:hover {
  background-color: var(--sc-chip-bg);
}

.ribbon-actions-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

@media (max-width: 1200px) {
  .ribbon-actions-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.5rem;
  }

  .ribbon-actions-grid :deep(.n-button) {
    width: 100%;
    justify-content: center;
  }
}

:root[data-display-palette='night'] .ribbon-action-button:hover {
  background-color: rgba(244, 244, 245, 0.08);
}

.ribbon-action-button.is-active {
  background-color: rgba(59, 130, 246, 0.18);
  color: #1d4ed8;
  border-color: rgba(37, 99, 235, 0.35);
}

.ribbon-action-button.is-active :deep(.n-icon) {
  color: #2563eb;
}

:root[data-display-palette='night'] .ribbon-action-button.is-active {
  background-color: rgba(96, 165, 250, 0.25);
  color: #cfe0ff;
  border-color: rgba(147, 197, 253, 0.45);
}

:root[data-display-palette='night'] .ribbon-action-button.is-active :deep(.n-icon) {
  color: #e0edff;
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
