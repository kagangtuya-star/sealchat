<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NIcon } from 'naive-ui'
import {
  Archive as ArchiveIcon,
  Download as DownloadIcon,
  DotsVertical as MoreIcon,
  Link as LinkIcon,
  LayoutBoardSplit as SplitIcon,
  MoodSmile as EmojiIcon,
  Palette,
  Photo as PhotoIcon,
  Star as StarIcon,
  Upload as UploadIcon,
  Users as UsersIcon,
  Id as CharacterCardIcon,
} from '@vicons/tabler'
import { DocumentTextOutline } from '@vicons/ionicons5'
import { MailOutline } from '@vicons/ionicons5'

interface FilterState {
  icFilter: 'all' | 'ic' | 'ooc'
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
  channelImagesActive?: boolean
  canImport?: boolean
  importActive?: boolean
  splitEnabled?: boolean
  splitActive?: boolean
  stickyNoteEnabled?: boolean
  stickyNoteActive?: boolean
  webhookEnabled?: boolean
  webhookActive?: boolean
  emailNotificationEnabled?: boolean
  emailNotificationActive?: boolean
  characterCardEnabled?: boolean
  characterCardActive?: boolean
}

interface Emits {
  (e: 'update:filters', filters: FilterState): void
  (e: 'open-archive'): void
  (e: 'open-export'): void
  (e: 'open-import'): void
  (e: 'open-identity-manager'): void
  (e: 'open-gallery'): void
  (e: 'open-display-settings'): void
  (e: 'open-favorites'): void
  (e: 'open-channel-images'): void
  (e: 'open-split'): void
  (e: 'toggle-sticky-note'): void
  (e: 'open-webhook'): void
  (e: 'open-email-notification'): void
  (e: 'open-character-card'): void
  (e: 'clear-filters'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

// Ref for measuring container width
const actionsContainerRef = ref<HTMLElement | null>(null)

// Number of visible buttons (dynamically calculated)
const visibleCount = ref(7)

// Define all action buttons
interface ActionButton {
  key: string
  label: string
  icon: any
  emitEvent: string
  activeKey: keyof Props
  condition?: () => boolean
}

const allActionButtons = computed<ActionButton[]>(() => {
  const buttons: ActionButton[] = [
    { key: 'display', label: '显示设置', icon: Palette, emitEvent: 'open-display-settings', activeKey: 'displayActive' },
    { key: 'identity', label: '角色管理', icon: UsersIcon, emitEvent: 'open-identity-manager', activeKey: 'identityActive' },
    { key: 'character-card', label: '人物卡', icon: CharacterCardIcon, emitEvent: 'open-character-card', activeKey: 'characterCardActive' },
    { key: 'export', label: '导出记录', icon: DownloadIcon, emitEvent: 'open-export', activeKey: 'exportActive' },
    { key: 'gallery', label: '表情资源', icon: EmojiIcon, emitEvent: 'open-gallery', activeKey: 'galleryActive' },
    { key: 'channel-images', label: '图片浏览', icon: PhotoIcon, emitEvent: 'open-channel-images', activeKey: 'channelImagesActive' },
    { key: 'favorites', label: '频道收藏', icon: StarIcon, emitEvent: 'open-favorites', activeKey: 'favoriteActive' },
  ]
  
  // Add import button if allowed (before 消息归档)
  if (props.canImport) {
    buttons.push({ key: 'import', label: '导入记录', icon: UploadIcon, emitEvent: 'open-import', activeKey: 'importActive' })
  }
  
  // 消息归档 always at the end
  buttons.push({ key: 'archive', label: '消息归档', icon: ArchiveIcon, emitEvent: 'open-archive', activeKey: 'archiveActive' })

  // 分屏入口（置于“消息归档”之后）
  if (props.splitEnabled !== false) {
    buttons.push({ key: 'split', label: '分屏', icon: SplitIcon, emitEvent: 'open-split', activeKey: 'splitActive' })
  }

  // 便签入口（置于“分屏”之后）
  if (props.stickyNoteEnabled !== false) {
    buttons.push({ key: 'sticky-note', label: '便签', icon: DocumentTextOutline, emitEvent: 'toggle-sticky-note', activeKey: 'stickyNoteActive' })
  }

  // Webhook 授权管理入口（通常在分屏模式下启用）
  if (props.webhookEnabled) {
    buttons.push({ key: 'webhook', label: 'Webhook', icon: LinkIcon, emitEvent: 'open-webhook', activeKey: 'webhookActive' })
  }

  // 邮件提醒入口（Webhook 下方）
  if (props.emailNotificationEnabled !== false) {
    buttons.push({ key: 'email-notification', label: '邮件提醒', icon: MailOutline, emitEvent: 'open-email-notification', activeKey: 'emailNotificationActive' })
  }
  
  return buttons
})

// Visible buttons (shown directly)
const visibleButtons = computed(() => {
  return allActionButtons.value.slice(0, visibleCount.value)
})

// Overflow buttons (shown in dropdown)
const overflowButtons = computed(() => {
  return allActionButtons.value.slice(visibleCount.value)
})

// Check if any overflow action is active
const hasActiveOverflowAction = computed(() => {
  return overflowButtons.value.some(btn => props[btn.activeKey])
})

// Show more button only if there are overflow buttons
const showMoreButton = computed(() => {
  return overflowButtons.value.length > 0
})

// Dropdown menu options for overflow buttons
const moreMenuOptions = computed(() => {
  return overflowButtons.value.map(btn => ({
    key: btn.key,
    label: btn.label,
    icon: () => h(NIcon, null, { default: () => h(btn.icon) }),
  }))
})

const handleMoreMenuSelect = (key: string) => {
  const button = allActionButtons.value.find(btn => btn.key === key)
  if (button) {
    emit(button.emitEvent as any)
  }
}

const handleButtonClick = (button: ActionButton) => {
  emit(button.emitEvent as any)
}

// Constants for button sizing (conservative estimates to avoid cutoff)
const BUTTON_BASE_WIDTH = 48 // icon + padding + border
const CHAR_WIDTH = 16 // approximate width per Chinese character
const BUTTON_GAP = 8 // gap between buttons (0.5rem)
const MORE_BUTTON_WIDTH = 75 // width of "更多" button
const MOBILE_BREAKPOINT = 768 // mobile breakpoint in px
const SAFETY_MARGIN = 10 // extra margin to prevent partial cutoff

// Check if current viewport is mobile
const isMobile = () => window.innerWidth <= MOBILE_BREAKPOINT

// Calculate button width based on label (with safety margin)
const getButtonWidth = (label: string) => {
  return BUTTON_BASE_WIDTH + label.length * CHAR_WIDTH + SAFETY_MARGIN
}

// Get all button widths (precomputed)
const allButtonWidths = computed(() => {
  return allActionButtons.value.map(btn => getButtonWidth(btn.label))
})

// Total width needed to display all buttons
const totalButtonsWidth = computed(() => {
  const widths = allButtonWidths.value
  return widths.reduce((sum, w) => sum + w, 0) + (widths.length - 1) * BUTTON_GAP
})

// Calculate how many buttons can fit
const calculateVisibleCount = () => {
  const totalButtons = allActionButtons.value.length
  
  // On mobile, show all buttons (CSS will handle wrapping)
  if (isMobile()) {
    visibleCount.value = totalButtons
    return
  }
  
  if (!actionsContainerRef.value) return
  
  const containerWidth = actionsContainerRef.value.offsetWidth
  const widths = allButtonWidths.value
  
  // Check if all buttons fit without "more" button
  if (totalButtonsWidth.value <= containerWidth) {
    visibleCount.value = totalButtons
    return
  }
  
  // Need to calculate how many fit with "more" button
  // Available width = container - more button - gap before more button
  const availableWidth = containerWidth - MORE_BUTTON_WIDTH - BUTTON_GAP
  
  let usedWidth = 0
  let count = 0
  
  for (let i = 0; i < totalButtons; i++) {
    const btnWidth = widths[i]
    const gapWidth = count > 0 ? BUTTON_GAP : 0
    const neededWidth = usedWidth + gapWidth + btnWidth
    
    if (neededWidth <= availableWidth) {
      usedWidth = neededWidth
      count++
    } else {
      break
    }
  }
  
  // Ensure at least 1 button is visible
  visibleCount.value = Math.max(count, 1)
}

// ResizeObserver for container
let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  nextTick(() => {
    calculateVisibleCount()
    
    // Setup ResizeObserver
    if (actionsContainerRef.value) {
      resizeObserver = new ResizeObserver(() => {
        calculateVisibleCount()
      })
      resizeObserver.observe(actionsContainerRef.value)
    }
  })
})

onBeforeUnmount(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
})

// Re-calculate when canImport changes (button list changes)
watch(
  () => [props.canImport, props.splitEnabled, props.stickyNoteEnabled, props.webhookEnabled, props.emailNotificationEnabled, props.characterCardEnabled],
  () => {
    nextTick(calculateVisibleCount)
  }
)

const roleSelectOptions = computed(() => {
  return props.roles.map(role => ({
    label: role.label || role.name || '未命名角色',
    value: role.id,
  }))
})

const activeFiltersCount = computed(() => {
  let count = 0
  if (props.filters.icFilter !== 'all') count++
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

const icFilterLabel = computed(() => {
  switch (props.filters.icFilter) {
    case 'ic': return '只看场内'
    case 'ooc': return '只看场外'
    default: return '全部消息'
  }
})

const cycleIcFilter = () => {
  const order: Array<'all' | 'ic' | 'ooc'> = ['all', 'ic', 'ooc']
  const idx = order.indexOf(props.filters.icFilter)
  const next = order[(idx + 1) % order.length]
  updateFilter('icFilter', next)
}
</script>

<template>
  <div class="action-ribbon">
    <!-- 筛选区域 -->
    <div class="ribbon-section ribbon-section--filters">
      <div class="filter-group">
        <n-button
          size="small"
          :type="filters.icFilter !== 'all' ? 'primary' : 'default'"
          :tertiary="filters.icFilter === 'all'"
          @click="cycleIcFilter"
        >
          {{ icFilterLabel }}
        </n-button>
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
    <div class="ribbon-section ribbon-section--actions" ref="actionsContainerRef">
      <div class="ribbon-actions-grid">
        <!-- 动态渲染可见按钮 -->
        <n-button
          v-for="button in visibleButtons"
          :key="button.key"
          type="tertiary"
          class="ribbon-action-button"
          :class="{ 'is-active': props[button.activeKey] }"
          @click="handleButtonClick(button)"
        >
          <template #icon>
            <n-icon :component="button.icon" />
          </template>
          {{ button.label }}
        </n-button>

        <!-- 更多功能 - 下拉菜单 (仅在有溢出按钮时显示) -->
        <n-dropdown
          v-if="showMoreButton"
          :options="moreMenuOptions"
          trigger="click"
          @select="handleMoreMenuSelect"
        >
          <n-button
            type="tertiary"
            class="ribbon-action-button ribbon-more-button"
            :class="{ 'is-active': hasActiveOverflowAction }"
          >
            <template #icon>
              <n-icon :component="MoreIcon" />
            </template>
            更多
          </n-button>
        </n-dropdown>
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
  flex-shrink: 0;
}

.ribbon-section--actions {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  justify-content: flex-start;
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
  flex-wrap: nowrap;
  gap: 0.5rem;
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

  .ribbon-section--actions {
    overflow: visible;
  }

  .ribbon-actions-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .ribbon-actions-grid :deep(.n-button) {
    width: 100%;
    justify-content: center;
  }

  .ribbon-section--summary {
    min-width: auto;
    justify-content: center;
  }
}
</style>
