<script setup lang="ts">
import { h, type Component } from 'vue'
import { NButton, NDropdown, NIcon, NTooltip, type DropdownOption } from 'naive-ui'
import { ChevronDown, LetterT, Photo, Pin } from '@vicons/tabler'

type SceneFixedObjectType = 'text' | 'image'

const emit = defineEmits<{
  add: [type: SceneFixedObjectType]
}>()

const renderIcon = (icon: Component) => () => h(NIcon, null, { default: () => h(icon) })
const options: DropdownOption[] = [
  { key: 'text', label: '场景固定文字', icon: renderIcon(LetterT) },
  { key: 'image', label: '场景固定图片', icon: renderIcon(Photo) },
]

const addSelected = (key: string | number) => {
  if (key === 'text' || key === 'image') emit('add', key)
}
</script>

<template>
  <span class="theater-scene-fixed-trigger-group">
    <n-tooltip trigger="hover">
      <template #trigger>
        <n-button
          class="theater-scene-fixed-trigger theater-scene-fixed-trigger--primary"
          size="small"
          aria-label="添加场景固定组件"
          @click="emit('add', 'image')"
        >
          <n-icon><Pin /></n-icon>
        </n-button>
      </template>
      添加场景固定组件
    </n-tooltip>

    <n-dropdown trigger="click" :options="options" @select="addSelected">
      <n-button
        class="theater-scene-fixed-trigger theater-scene-fixed-trigger--menu"
        size="small"
        aria-label="选择场景固定组件"
      >
        <n-icon><ChevronDown /></n-icon>
      </n-button>
    </n-dropdown>
  </span>
</template>

<style scoped>
.theater-scene-fixed-trigger-group {
  display: inline-flex;
  flex: 0 0 auto;
}

.theater-scene-fixed-trigger {
  padding: 0;
  border-radius: 0;
}

.theater-scene-fixed-trigger--primary {
  width: 30px;
  border-radius: 3px 0 0 3px;
}

.theater-scene-fixed-trigger--menu {
  width: 18px;
  margin-left: -1px;
  border-radius: 0 3px 3px 0;
}
</style>
