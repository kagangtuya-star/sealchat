<script setup lang="ts">
import { computed, h, type Component } from 'vue'
import { NButton, NDropdown, NIcon, NTooltip, type DropdownOption } from 'naive-ui'
import { Check, ChevronDown, Copy } from '@vicons/tabler'
import type { StageCopyMode } from './stage-editing'

const props = defineProps<{
  mode: StageCopyMode
  disabled?: boolean
}>()

const emit = defineEmits<{
  copy: []
  selectMode: [mode: StageCopyMode]
}>()

const renderIcon = (icon: Component) => () => h(NIcon, null, { default: () => h(icon) })
const options = computed<DropdownOption[]>(() => [
  { key: 'in-place', label: '原位复制', icon: renderIcon(props.mode === 'in-place' ? Check : Copy) },
  { key: 'offset', label: '偏移复制', icon: renderIcon(props.mode === 'offset' ? Check : Copy) },
])
const activeModeLabel = computed(() => props.mode === 'in-place' ? '原位复制' : '偏移复制')

const copySelected = () => emit('copy')
const selectCopyMode = (key: string | number) => {
  if (key === 'in-place' || key === 'offset') emit('selectMode', key)
}
const theaterSecondaryMenuProps = () => ({ class: 'theater-secondary-surface' })
</script>

<template>
  <span class="theater-copy-trigger-group">
    <n-tooltip trigger="hover">
      <template #trigger>
        <n-button
          class="theater-copy-trigger theater-copy-trigger--primary"
          size="small"
          :disabled="disabled"
          :aria-label="`${activeModeLabel}所选组件`"
          @click="copySelected"
        >
          <n-icon><Copy /></n-icon>
        </n-button>
      </template>
      {{ activeModeLabel }}所选组件 Ctrl+C
    </n-tooltip>

    <n-dropdown trigger="click" :options="options" :menu-props="theaterSecondaryMenuProps" @select="selectCopyMode">
      <n-button
        class="theater-copy-trigger theater-copy-trigger--menu"
        size="small"
        :disabled="disabled"
        aria-label="选择复制方式"
      >
        <n-icon><ChevronDown /></n-icon>
      </n-button>
    </n-dropdown>
  </span>
</template>

<style scoped>
.theater-copy-trigger-group {
  display: inline-flex;
  flex: 0 0 auto;
}

.theater-copy-trigger {
  padding: 0;
  border-radius: 0;
}

.theater-copy-trigger--primary {
  width: 30px;
  border-radius: 3px 0 0 3px;
}

.theater-copy-trigger--menu {
  width: 18px;
  margin-left: -1px;
  border-radius: 0 3px 3px 0;
}
</style>
