<script setup lang="ts">
import { computed, h } from 'vue'
import { NButton, NDropdown, NIcon, NTooltip, type DropdownOption } from 'naive-ui'
import { Check, ChevronDown, Magnet } from '@vicons/tabler'

const props = defineProps<{
  snapEnabled: boolean
  displayGrid: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  toggleSnap: []
  toggleDisplayGrid: []
}>()

const renderCheck = (checked: boolean) => () => h(
  NIcon,
  { style: { opacity: checked ? 1 : 0 } },
  { default: () => h(Check) },
)

const options = computed<DropdownOption[]>(() => [
  { key: 'snap', label: '网格吸附', icon: renderCheck(props.snapEnabled) },
  { key: 'display-grid', label: '始终显示网格', icon: renderCheck(props.displayGrid) },
])

const selectGridOption = (key: string | number) => {
  if (key === 'snap') emit('toggleSnap')
  if (key === 'display-grid') emit('toggleDisplayGrid')
}

const theaterSecondaryMenuProps = () => ({ class: 'theater-secondary-surface' })
</script>

<template>
  <span class="theater-grid-toolbar">
    <n-tooltip trigger="hover">
      <template #trigger>
        <n-button
          class="theater-grid-trigger theater-grid-trigger--primary theater-grid-snap-tool"
          :class="{ 'is-active': snapEnabled }"
          size="small"
          :disabled="disabled"
          :aria-pressed="snapEnabled"
          aria-label="网格吸附"
          @click="emit('toggleSnap')"
        >
          <n-icon><Magnet /></n-icon>
        </n-button>
      </template>
      {{ snapEnabled ? '关闭网格吸附' : '网格吸附' }}
    </n-tooltip>

    <n-dropdown trigger="click" :options="options" :menu-props="theaterSecondaryMenuProps" @select="selectGridOption">
      <n-button
        class="theater-grid-trigger theater-grid-trigger--menu"
        size="small"
        :disabled="disabled"
        aria-label="选择网格选项"
      >
        <n-icon><ChevronDown /></n-icon>
      </n-button>
    </n-dropdown>
  </span>
</template>

<style scoped>
.theater-grid-toolbar {
  display: inline-flex;
  flex: 0 0 auto;
}

.theater-grid-trigger {
  padding: 0;
  border-radius: 0;
}

.theater-grid-trigger--primary {
  width: 30px;
  min-width: 30px;
  border-radius: 3px 0 0 3px;
}

.theater-grid-trigger--menu {
  width: 18px;
  min-width: 18px;
  margin-left: -1px;
  border-radius: 0 3px 3px 0;
}
</style>
