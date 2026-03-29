<script setup lang="ts">
import { computed } from 'vue'
import { Mask, MoodHappy } from '@vicons/tabler'

interface Props {
  modelValue: 'ic' | 'ooc'
  disabled?: boolean
  compact?: boolean
}

interface Emits {
  (e: 'update:modelValue', value: 'ic' | 'ooc'): void
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  compact: false,
})
const emit = defineEmits<Emits>()

const isIc = computed(() => props.modelValue === 'ic')
const iconComponent = computed(() => (isIc.value ? Mask : MoodHappy))
const buttonType = computed(() => (isIc.value ? 'success' : 'info'))
const tooltipText = computed(() => (isIc.value ? '当前为场内模式，点击切换到场外' : '当前为场外模式，点击切换到场内'))
const buttonSize = computed(() => (props.compact ? 'medium' : 'small'))

const handleToggle = () => {
  if (props.disabled) return
  emit('update:modelValue', isIc.value ? 'ooc' : 'ic')
}
</script>

<template>
  <div class="ic-ooc-toggle">
    <n-tooltip trigger="hover">
      <template #trigger>
        <n-button
          circle
          :size="buttonSize"
          :type="buttonType"
          class="ic-ooc-toggle__button"
          :class="{ 'ic-ooc-toggle__button--compact': props.compact }"
          :quaternary="props.compact"
          :disabled="disabled"
          @click="handleToggle"
        >
          <template #icon>
            <n-icon :component="iconComponent" :size="props.compact ? 17 : 18" />
          </template>
        </n-button>
      </template>
      {{ tooltipText }}
    </n-tooltip>
  </div>
</template>

<style lang="scss" scoped>
.ic-ooc-toggle {
  display: inline-flex;
  align-items: center;
}

.ic-ooc-toggle__button {
  transition: transform 0.2s ease;
}

.ic-ooc-toggle__button--compact {
  border-radius: 999px;
}

.ic-ooc-toggle__button:not(:disabled):hover {
  transform: translateY(-1px);
}
</style>
